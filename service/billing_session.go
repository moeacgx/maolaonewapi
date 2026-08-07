package service

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// BillingSession — 统一计费会话
// ---------------------------------------------------------------------------

// BillingSession 封装单次请求的预扣费/结算/退款生命周期。
// 实现 relaycommon.BillingSettler 接口。
type BillingSession struct {
	relayInfo                 *relaycommon.RelayInfo
	funding                   FundingSource
	preConsumedQuota          int  // 实际预扣额度（信任用户可能为 0）
	tokenConsumed             int  // 令牌额度实际扣减量
	extraReserved             int  // 发送前补充预扣的额度（订阅退款时需要单独回滚）
	trusted                   bool // 是否命中信任额度旁路
	fundingSettled            bool // funding.Settle 已成功，资金来源已提交
	settled                   bool // Settle 全部完成（资金 + 令牌）
	refunded                  bool // Refund 已调用
	settleRequested           bool
	settleTarget              int
	compensationBase          string
	reserveAttemptSequence    int
	pendingTokenCompensations []tokenQuotaCompensationState
	compensationRetryRunning  bool
	mu                        sync.Mutex
}

type tokenQuotaCompensationState struct {
	operationKey string
	quota        int
}

const tokenQuotaCompensationAttempts = 3

// Settle 根据实际消耗额度进行结算。
// 资金来源和令牌额度分两步提交：若资金来源已提交但令牌调整失败，
// 会标记 fundingSettled 防止 Refund 对已提交的资金来源执行退款。
func (s *BillingSession) Settle(actualQuota int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.settleLocked(actualQuota)
}

func (s *BillingSession) settleLocked(actualQuota int) error {
	if actualQuota < 0 {
		return fmt.Errorf("billing settlement quota must be non-negative: %d", actualQuota)
	}
	if s.settleRequested && s.settleTarget != actualQuota {
		return fmt.Errorf("billing settlement target conflict: existing=%d requested=%d", s.settleTarget, actualQuota)
	}
	if s.settled {
		return nil
	}
	if s.refunded {
		return errors.New("billing session already refunded")
	}
	s.settleRequested = true
	s.settleTarget = actualQuota
	if err := s.retryPendingTokenCompensationsLocked(tokenQuotaCompensationAttempts); err != nil {
		s.schedulePendingTokenCompensationRetryLocked()
		return fmt.Errorf("complete pending token quota compensation before settlement: %w", err)
	}
	delta := actualQuota - s.preConsumedQuota
	if delta == 0 {
		s.settled = true
		return nil
	}
	// 1) 调整资金来源（仅在尚未提交时执行，防止重复调用）
	if !s.fundingSettled {
		if err := s.funding.Settle(delta); err != nil {
			return err
		}
		s.fundingSettled = true
	}
	// 2) 调整令牌额度
	var tokenErr error
	if !s.relayInfo.IsPlayground && !s.relayInfo.SkipTokenQuota {
		if delta > 0 {
			tokenErr = model.DecreaseTokenQuota(s.relayInfo.TokenId, s.relayInfo.TokenKey, delta)
		} else {
			tokenErr = model.IncreaseTokenQuota(s.relayInfo.TokenId, s.relayInfo.TokenKey, -delta)
		}
		if tokenErr != nil {
			// 资金来源已提交，令牌调整失败只能记录日志；fundingSettled 会阻止 Refund 误退资金。
			common.SysLog(fmt.Sprintf("error adjusting token quota after funding settled (userId=%d, tokenId=%d, delta=%d): %s",
				s.relayInfo.UserId, s.relayInfo.TokenId, delta, tokenErr.Error()))
			return tokenErr
		}
	}
	// 3) 更新 relayInfo 上的订阅 PostDelta（用于日志）
	if s.funding.Source() == BillingSourceSubscription {
		s.relayInfo.SubscriptionPostDelta += int64(delta)
	}
	s.settled = true
	return nil
}

// Refund 退还所有预扣费，幂等安全，异步执行。
func (s *BillingSession) Refund(c *gin.Context) {
	s.mu.Lock()
	if s.settleRequested {
		// Realtime 已经根据成功 usage 进入结算，不得再切换为全额退款。
		// 待补偿令牌操作由幂等 Worker 继续处理。
		s.schedulePendingTokenCompensationRetryLocked()
		s.mu.Unlock()
		return
	}
	if s.settled || s.refunded || !s.needsRefundLocked() {
		s.mu.Unlock()
		return
	}
	s.refunded = true
	s.schedulePendingTokenCompensationRetryLocked()
	tokenId := s.relayInfo.TokenId
	tokenKey := s.relayInfo.TokenKey
	skipTokenQuota := s.relayInfo.IsPlayground || s.relayInfo.SkipTokenQuota
	tokenConsumed := s.tokenConsumed
	extraReserved := s.extraReserved
	subscriptionId := s.relayInfo.SubscriptionId
	funding := s.funding
	s.mu.Unlock()

	logger.LogInfo(c, fmt.Sprintf("用户 %d 请求失败, 返还预扣费（token_quota=%s, funding=%s）",
		s.relayInfo.UserId,
		logger.FormatQuota(tokenConsumed),
		funding.Source(),
	))

	gopool.Go(func() {
		// 1) 退还资金来源
		if err := funding.Refund(); err != nil {
			common.SysLog("error refunding billing source: " + err.Error())
		}
		if extraReserved > 0 && funding.Source() == BillingSourceSubscription && subscriptionId > 0 {
			if err := model.PostConsumeUserSubscriptionDelta(subscriptionId, -int64(extraReserved)); err != nil {
				common.SysLog("error refunding subscription extra reserved quota: " + err.Error())
			}
		}
		// 2) 退还令牌额度
		if tokenConsumed > 0 && !skipTokenQuota {
			if err := model.IncreaseTokenQuota(tokenId, tokenKey, tokenConsumed); err != nil {
				common.SysLog("error refunding token quota: " + err.Error())
			}
		}
	})
}

// NeedsRefund 返回是否存在需要退还的预扣状态。
func (s *BillingSession) NeedsRefund() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.needsRefundLocked()
}

func (s *BillingSession) HasPendingTokenCompensation() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pendingTokenCompensations) > 0
}

func (s *BillingSession) needsRefundLocked() bool {
	if s.settled || s.refunded || s.fundingSettled || s.settleRequested {
		// fundingSettled 时资金来源已提交结算，不能再退预扣费
		return false
	}
	if len(s.pendingTokenCompensations) > 0 {
		return true
	}
	if s.tokenConsumed > 0 {
		return true
	}
	// 订阅可能在 tokenConsumed=0 时仍预扣了额度
	if sub, ok := s.funding.(*SubscriptionFunding); ok && sub.preConsumed > 0 {
		return true
	}
	return false
}

// GetPreConsumedQuota 返回实际预扣的额度。
func (s *BillingSession) GetPreConsumedQuota() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.preConsumedQuota
}

func (s *BillingSession) Reserve(targetQuota int) error {
	return s.reserve(targetQuota, false)
}

// ReserveRealtime 强制把信任旁路转换为累计预留，避免长连接最终一次性透支。
func (s *BillingSession) ReserveRealtime(targetQuota int) error {
	return s.reserve(targetQuota, true)
}

func (s *BillingSession) reserve(targetQuota int, forceTrustedReservation bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.settled || s.refunded || (!forceTrustedReservation && s.trusted) || targetQuota <= s.preConsumedQuota {
		return nil
	}
	if s.settleRequested {
		return errors.New("billing settlement already started")
	}

	delta := targetQuota - s.preConsumedQuota
	if delta <= 0 {
		return nil
	}

	s.reserveAttemptSequence++
	reservePhase := fmt.Sprintf("reserve:%s:%d", s.funding.Source(), s.reserveAttemptSequence)
	tokenReservation, err := s.reserveToken(delta)
	if err != nil {
		return err
	}
	if err := s.reserveFunding(delta); err != nil {
		compensationErr, resolved := s.compensateTokenReservationLocked(
			tokenReservation,
			s.tokenCompensationOperationKeyLocked(reservePhase, targetQuota),
			delta,
			tokenQuotaCompensationAttempts,
		)
		if !resolved {
			s.schedulePendingTokenCompensationRetryLocked()
		}
		return s.newFundingError(err, compensationErr, resolved)
	}
	if tokenReservation != nil {
		tokenReservation.Commit()
	}

	s.trusted = false
	s.preConsumedQuota += delta
	s.tokenConsumed += delta
	s.extraReserved += delta
	s.syncRelayInfo()
	return nil
}

// ---------------------------------------------------------------------------
// PreConsume — 统一预扣费入口（含信任额度旁路）
// ---------------------------------------------------------------------------

// preConsume 执行预扣费：信任检查 -> 令牌预扣 -> 资金来源预扣。
// 资金来源失败时，批量预留在刷盘前抵消，已落库预留使用持久幂等操作补偿。
func (s *BillingSession) preConsume(c *gin.Context, quota int) *types.NewAPIError {
	effectiveQuota := quota

	// ---- 信任额度旁路 ----
	if s.shouldTrust(c) {
		s.trusted = true
		effectiveQuota = 0
		logger.LogInfo(c, fmt.Sprintf("用户 %d 额度充足, 信任且不需要预扣费 (funding=%s)", s.relayInfo.UserId, s.funding.Source()))
	} else if effectiveQuota > 0 {
		logger.LogInfo(c, fmt.Sprintf("用户 %d 需要预扣费 %s (funding=%s)", s.relayInfo.UserId, logger.FormatQuota(effectiveQuota), s.funding.Source()))
	}

	// ---- 1) 预扣令牌额度 ----
	var tokenReservation *model.TokenQuotaReservation
	if effectiveQuota > 0 {
		var err error
		tokenReservation, err = beginTokenQuotaReservation(s.relayInfo, effectiveQuota)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		if tokenReservation != nil {
			s.tokenConsumed = effectiveQuota
		}
	}

	// ---- 2) 预扣资金来源 ----
	if err := s.funding.PreConsume(effectiveQuota); err != nil {
		compensationResolved := true
		var compensationErr error
		if tokenReservation != nil {
			compensationErr, compensationResolved = s.compensateTokenReservationLocked(
				tokenReservation,
				s.tokenCompensationOperationKeyLocked("initial:"+s.funding.Source(), effectiveQuota),
				effectiveQuota,
				tokenQuotaCompensationAttempts,
			)
			s.tokenConsumed = 0
			if !compensationResolved {
				s.schedulePendingTokenCompensationRetryLocked()
			}
		}
		return s.newFundingError(err, compensationErr, compensationResolved)
	}
	if tokenReservation != nil {
		tokenReservation.Commit()
	}

	s.preConsumedQuota = effectiveQuota

	// ---- 同步 RelayInfo 兼容字段 ----
	s.syncRelayInfo()

	return nil
}

func (s *BillingSession) reserveFunding(delta int) error {
	switch funding := s.funding.(type) {
	case *WalletFunding:
		if err := model.DecreaseUserQuota(funding.userId, delta, false); err != nil {
			return err
		}
		funding.consumed += delta
		return nil
	case *SubscriptionFunding:
		if err := model.PostConsumeUserSubscriptionDelta(funding.subscriptionId, int64(delta)); err != nil {
			return err
		}
		return nil
	default:
		return types.NewError(fmt.Errorf("unsupported funding source: %s", s.funding.Source()), types.ErrorCodeUpdateDataError, types.ErrOptionWithSkipRetry())
	}
}

func (s *BillingSession) reserveToken(delta int) (*model.TokenQuotaReservation, error) {
	if delta <= 0 || s.relayInfo.IsPlayground || s.relayInfo.SkipTokenQuota {
		return nil, nil
	}
	reservation, err := beginTokenQuotaReservation(s.relayInfo, delta)
	if err != nil {
		return nil, types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	return reservation, nil
}

func newSubscriptionFundingError(err error) *types.NewAPIError {
	if errors.Is(err, model.ErrNoActiveSubscription) || errors.Is(err, model.ErrSubscriptionQuotaInsufficient) {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("订阅额度不足或未配置订阅: %w", err),
			types.ErrorCodeInsufficientUserQuota,
			http.StatusForbidden,
			types.ErrOptionWithSkipRetry(),
			types.ErrOptionWithNoRecordErrorLog(),
		)
	}
	return NewUserQuotaUpdateError(err)
}

func (s *BillingSession) newFundingError(fundingErr error, compensationErr error, compensationResolved bool) *types.NewAPIError {
	combinedErr := errors.Join(fundingErr, compensationErr)
	if !compensationResolved {
		return NewUserQuotaUpdateError(combinedErr)
	}
	if s.funding.Source() == BillingSourceSubscription {
		return newSubscriptionFundingError(combinedErr)
	}
	return NewUserQuotaUpdateError(combinedErr)
}

func (s *BillingSession) tokenCompensationOperationKeyLocked(phase string, targetQuota int) string {
	if s.compensationBase == "" {
		requestId := strings.TrimSpace(s.relayInfo.RequestId)
		sessionId := common.GetUUID()
		if requestId == "" {
			s.compensationBase = sessionId
		} else {
			s.compensationBase = requestId + ":" + sessionId
		}
	}
	return fmt.Sprintf("billing:%s:%s:%d", s.compensationBase, phase, targetQuota)
}

func (s *BillingSession) rememberPendingTokenCompensationLocked(operationKey string, quota int) {
	for _, pending := range s.pendingTokenCompensations {
		if pending.operationKey == operationKey {
			return
		}
	}
	s.pendingTokenCompensations = append(s.pendingTokenCompensations, tokenQuotaCompensationState{
		operationKey: operationKey,
		quota:        quota,
	})
}

func (s *BillingSession) forgetPendingTokenCompensationLocked(operationKey string) {
	for i, pending := range s.pendingTokenCompensations {
		if pending.operationKey != operationKey {
			continue
		}
		s.pendingTokenCompensations = append(s.pendingTokenCompensations[:i], s.pendingTokenCompensations[i+1:]...)
		return
	}
}

func (s *BillingSession) compensateTokenReservationLocked(
	reservation *model.TokenQuotaReservation,
	operationKey string,
	quota int,
	attempts int,
) (error, bool) {
	if reservation == nil || quota <= 0 {
		return nil, true
	}
	if attempts <= 0 {
		attempts = 1
	}
	var failures error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := reservation.Compensate(operationKey); err == nil {
			s.forgetPendingTokenCompensationLocked(operationKey)
			return failures, true
		} else {
			failures = errors.Join(failures, err)
		}
		if attempt < attempts-1 {
			time.Sleep(time.Duration(25*(attempt+1)) * time.Millisecond)
		}
	}
	s.rememberPendingTokenCompensationLocked(operationKey, quota)
	return failures, false
}

func (s *BillingSession) compensateTokenQuotaLocked(operationKey string, quota int, attempts int) (error, bool) {
	if quota <= 0 || s.relayInfo.IsPlayground || s.relayInfo.SkipTokenQuota {
		return nil, true
	}
	if attempts <= 0 {
		attempts = 1
	}
	var failures error
	for attempt := 0; attempt < attempts; attempt++ {
		err := model.ApplyTokenQuotaCompensation(
			operationKey,
			s.relayInfo.TokenId,
			s.relayInfo.TokenKey,
			quota,
		)
		if err == nil {
			s.forgetPendingTokenCompensationLocked(operationKey)
			if failures != nil {
				common.SysLog(fmt.Sprintf("token quota compensation recovered (userId=%d, tokenId=%d, quota=%d): %s",
					s.relayInfo.UserId, s.relayInfo.TokenId, quota, failures.Error()))
			}
			return failures, true
		}
		failures = errors.Join(failures, err)
		if attempt < attempts-1 {
			time.Sleep(time.Duration(25*(attempt+1)) * time.Millisecond)
		}
	}
	s.rememberPendingTokenCompensationLocked(operationKey, quota)
	return failures, false
}

func (s *BillingSession) retryPendingTokenCompensationsLocked(attempts int) error {
	pending := append([]tokenQuotaCompensationState(nil), s.pendingTokenCompensations...)
	var unresolved error
	for _, item := range pending {
		failures, resolved := s.compensateTokenQuotaLocked(item.operationKey, item.quota, attempts)
		if !resolved {
			unresolved = errors.Join(unresolved, failures)
		}
	}
	return unresolved
}

func (s *BillingSession) schedulePendingTokenCompensationRetryLocked() {
	if s.compensationRetryRunning || len(s.pendingTokenCompensations) == 0 {
		return
	}
	s.compensationRetryRunning = true
	gopool.Go(func() {
		for attempt := 0; attempt < 8; attempt++ {
			time.Sleep(time.Duration(min(attempt+1, 5)) * time.Second)
			s.mu.Lock()
			err := s.retryPendingTokenCompensationsLocked(1)
			completed := len(s.pendingTokenCompensations) == 0
			if completed || attempt == 7 {
				s.compensationRetryRunning = false
			}
			s.mu.Unlock()
			if completed {
				return
			}
			if err != nil {
				common.SysLog("token quota compensation remains pending: " + err.Error())
			}
		}
	})
}

func compensateRelayTokenQuota(
	relayInfo *relaycommon.RelayInfo,
	reservation *model.TokenQuotaReservation,
	phase string,
	targetQuota int,
	quota int,
) error {
	if relayInfo == nil || quota <= 0 || relayInfo.IsPlayground || relayInfo.SkipTokenQuota {
		return nil
	}
	compensationBase := strings.TrimSpace(relayInfo.RequestId)
	if compensationBase != "" {
		compensationBase += ":"
	}
	compensationBase += common.GetUUID()
	session := &BillingSession{relayInfo: relayInfo, compensationBase: compensationBase}
	operationKey := session.tokenCompensationOperationKeyLocked(phase, targetQuota)
	var compensationErr error
	var resolved bool
	if reservation != nil {
		compensationErr, resolved = session.compensateTokenReservationLocked(
			reservation,
			operationKey,
			quota,
			tokenQuotaCompensationAttempts,
		)
	} else {
		compensationErr, resolved = session.compensateTokenQuotaLocked(
			operationKey,
			quota,
			tokenQuotaCompensationAttempts,
		)
	}
	if !resolved {
		session.schedulePendingTokenCompensationRetryLocked()
	}
	return compensationErr
}

// shouldTrust 统一信任额度检查，适用于钱包和订阅。
func (s *BillingSession) shouldTrust(c *gin.Context) bool {
	// 异步任务（ForcePreConsume=true）必须预扣全额，不允许信任旁路
	if s.relayInfo.ForcePreConsume {
		return false
	}

	trustQuota := common.GetTrustQuota()
	if trustQuota <= 0 {
		return false
	}

	// 检查令牌是否充足
	tokenTrusted := s.relayInfo.TokenUnlimited
	if !tokenTrusted {
		tokenQuota := c.GetInt("token_quota")
		tokenTrusted = tokenQuota > trustQuota
	}
	if !tokenTrusted {
		return false
	}

	switch s.funding.Source() {
	case BillingSourceWallet:
		return s.relayInfo.UserQuota > trustQuota
	case BillingSourceSubscription:
		// 订阅不能启用信任旁路。原因：
		// 1. PreConsumeUserSubscription 要求 amount>0 来创建预扣记录并锁定订阅
		// 2. SubscriptionFunding.PreConsume 忽略参数，始终用 s.amount 预扣
		// 3. 若信任旁路将 effectiveQuota 设为 0，会导致 preConsumedQuota 与实际订阅预扣不一致
		return false
	default:
		return false
	}
}

// syncRelayInfo 将 BillingSession 的状态同步到 RelayInfo 的兼容字段上。
func (s *BillingSession) syncRelayInfo() {
	info := s.relayInfo
	info.FinalPreConsumedQuota = s.preConsumedQuota
	info.BillingSource = s.funding.Source()

	if sub, ok := s.funding.(*SubscriptionFunding); ok {
		info.SubscriptionId = sub.subscriptionId
		info.SubscriptionPreConsumed = sub.preConsumed + int64(s.extraReserved)
		info.SubscriptionPostDelta = 0
		info.SubscriptionAmountTotal = sub.AmountTotal
		info.SubscriptionAmountUsedAfterPreConsume = sub.AmountUsedAfter + int64(s.extraReserved)
		info.SubscriptionPlanId = sub.PlanId
		info.SubscriptionPlanTitle = sub.PlanTitle
	} else {
		info.SubscriptionId = 0
		info.SubscriptionPreConsumed = 0
	}
}

// ---------------------------------------------------------------------------
// NewBillingSession 工厂 — 根据计费偏好创建会话并处理回退
// ---------------------------------------------------------------------------

// NewBillingSession 根据用户计费偏好创建 BillingSession，处理 subscription_first / wallet_first 的回退。
func NewBillingSession(c *gin.Context, relayInfo *relaycommon.RelayInfo, preConsumedQuota int) (*BillingSession, *types.NewAPIError) {
	if relayInfo == nil {
		return nil, types.NewError(fmt.Errorf("relayInfo is nil"), types.ErrorCodeInvalidRequest, types.ErrOptionWithSkipRetry())
	}

	pref := common.NormalizeBillingPreference(relayInfo.UserSetting.BillingPreference)

	// 钱包路径需要先检查用户额度
	tryWallet := func() (*BillingSession, *types.NewAPIError) {
		userQuota, err := model.GetUserQuotaWithContext(requestContextOrBackground(c), relayInfo.UserId, false)
		if err != nil {
			return nil, newUserQuotaQueryError(err)
		}
		if userQuota <= 0 {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("用户额度不足, 剩余额度: %s", logger.FormatQuota(userQuota)),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		if userQuota-preConsumedQuota < 0 {
			return nil, types.NewErrorWithStatusCode(
				fmt.Errorf("预扣费额度失败, 用户剩余额度: %s, 需要预扣费额度: %s", logger.FormatQuota(userQuota), logger.FormatQuota(preConsumedQuota)),
				types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
				types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		relayInfo.UserQuota = userQuota

		session := &BillingSession{
			relayInfo: relayInfo,
			funding:   &WalletFunding{userId: relayInfo.UserId},
		}
		if apiErr := session.preConsume(c, preConsumedQuota); apiErr != nil {
			if session.HasPendingTokenCompensation() {
				return session, apiErr
			}
			return nil, apiErr
		}
		return session, nil
	}

	trySubscription := func() (*BillingSession, *types.NewAPIError) {
		subConsume := int64(preConsumedQuota)
		if subConsume <= 0 {
			subConsume = 1
		}
		session := &BillingSession{
			relayInfo: relayInfo,
			funding: &SubscriptionFunding{
				requestId: relayInfo.RequestId,
				userId:    relayInfo.UserId,
				modelName: relayInfo.OriginModelName,
				amount:    subConsume,
			},
		}
		// 必须传 subConsume 而非 preConsumedQuota，保证 SubscriptionFunding.amount、
		// preConsume 参数和 FinalPreConsumedQuota 三者一致，避免订阅多扣费。
		if apiErr := session.preConsume(c, int(subConsume)); apiErr != nil {
			if session.HasPendingTokenCompensation() {
				return session, apiErr
			}
			return nil, apiErr
		}
		return session, nil
	}

	switch pref {
	case "subscription_only":
		return trySubscription()
	case "wallet_only":
		return tryWallet()
	case "wallet_first":
		session, err := tryWallet()
		if err != nil {
			if err.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
				return trySubscription()
			}
			return session, err
		}
		return session, nil
	case "subscription_first":
		fallthrough
	default:
		hasSub, subCheckErr := model.HasActiveUserSubscription(relayInfo.UserId)
		if subCheckErr != nil {
			return nil, newSubscriptionQueryError(subCheckErr)
		}
		if !hasSub {
			return tryWallet()
		}
		session, apiErr := trySubscription()
		if apiErr != nil {
			if apiErr.GetErrorCode() == types.ErrorCodeInsufficientUserQuota {
				return tryWallet()
			}
			return session, apiErr
		}
		return session, nil
	}
}
