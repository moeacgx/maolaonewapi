package service

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/model"
)

// ---------------------------------------------------------------------------
// FundingSource — 资金来源接口（钱包 or 订阅）
// ---------------------------------------------------------------------------

// FundingSource 抽象了预扣费的资金来源。
type FundingSource interface {
	// Source 返回资金来源标识："wallet" 或 "subscription"
	Source() string
	// PreConsume 从该资金来源预扣 amount 额度
	PreConsume(amount int) error
	// Settle 根据差额调整资金来源（正数补扣，负数退还）
	Settle(delta int) error
	// Refund 退还所有预扣费
	Refund() error
}

const BillingSourceBenefitVoucher = "benefit_voucher"

var ErrInsufficientBenefitVoucher = errors.New("benefit voucher quota insufficient")

type CompositeFunding struct {
	sources  []FundingSource
	consumed []int
}

type BenefitVoucherFunding struct {
	requestID string
	userID    int
	groupID   int
	now       func() int64
	consumed  int64
	reserved  int64
	voucherID int
}

func NewBenefitVoucherFunding(requestID string, userID, groupID int) *BenefitVoucherFunding {
	return &BenefitVoucherFunding{requestID: requestID, userID: userID, groupID: groupID, now: func() int64 { return time.Now().Unix() }}
}

func (f *BenefitVoucherFunding) Source() string { return BillingSourceBenefitVoucher }

func (f *BenefitVoucherFunding) PreConsume(amount int) error {
	reservation, err := model.ReserveBenefitVoucherQuota(f.requestID, f.userID, f.groupID, int64(amount), f.now())
	if err != nil {
		if errors.Is(err, model.ErrBenefitVoucherUnavailable) {
			return ErrInsufficientBenefitVoucher
		}
		return err
	}
	f.reserved = reservation.Reserved
	f.consumed = reservation.Reserved
	f.voucherID = reservation.VoucherID
	return nil
}

func (f *BenefitVoucherFunding) Capacity() int {
	if f == nil || f.userID <= 0 || f.groupID <= 0 {
		return 0
	}
	quota, err := model.GetBenefitVoucherAvailableQuota(f.userID, f.groupID, f.now())
	if err != nil || quota <= 0 {
		return 0
	}
	maxInt := int64(^uint(0) >> 1)
	if quota > maxInt {
		return int(maxInt)
	}
	return int(quota)
}

func (f *BenefitVoucherFunding) AdditionalCapacity() int { return f.Capacity() }

func (f *BenefitVoucherFunding) PreConsumedAmount() int { return int(f.consumed) }

func (f *BenefitVoucherFunding) Settle(delta int) error {
	if err := model.SettleBenefitVoucherQuota(f.requestID, int64(delta), f.now()); err != nil {
		return err
	}
	f.consumed += int64(delta)
	return nil
}

func (f *BenefitVoucherFunding) Refund() error {
	if f == nil || f.reserved <= 0 {
		return nil
	}
	err := model.RefundBenefitVoucherQuota(f.requestID, f.now())
	if err == nil {
		f.reserved = 0
		f.consumed = 0
	}
	return err
}

type fundingCapacity interface {
	Capacity() int
}

type fundingAdditionalCapacity interface {
	AdditionalCapacity() int
}

type fundingConsumed interface {
	PreConsumedAmount() int
}

func NewCompositeFunding(sources ...FundingSource) *CompositeFunding {
	filtered := make([]FundingSource, 0, len(sources))
	for _, source := range sources {
		if source != nil {
			filtered = append(filtered, source)
		}
	}
	return &CompositeFunding{sources: filtered, consumed: make([]int, len(filtered))}
}

func (f *CompositeFunding) Source() string { return BillingSourceBenefitVoucher }

func (f *CompositeFunding) PreConsume(amount int) error {
	if amount <= 0 {
		return nil
	}
	remaining := amount
	for index, source := range f.sources {
		if remaining <= 0 {
			break
		}
		attempt := remaining
		if capacity, ok := source.(fundingCapacity); ok && capacity.Capacity() >= 0 && capacity.Capacity() < attempt {
			attempt = capacity.Capacity()
		}
		if attempt <= 0 {
			continue
		}
		if err := source.PreConsume(attempt); err != nil {
			if errors.Is(err, ErrInsufficientBenefitVoucher) || errors.Is(err, ErrInsufficientWalletQuota) || strings.Contains(err.Error(), "no active subscription") || strings.Contains(err.Error(), "subscription quota insufficient") {
				continue
			}
			for rollback := index - 1; rollback >= 0; rollback-- {
				if f.consumed[rollback] > 0 {
					_ = f.sources[rollback].Refund()
				}
			}
			return fmt.Errorf("benefit composite funding %s failed: %w", source.Source(), err)
		}
		consumed := attempt
		if measured, ok := source.(fundingConsumed); ok {
			consumed = measured.PreConsumedAmount()
		}
		if consumed < 0 || consumed > attempt {
			return errors.New("benefit composite funding returned invalid reservation")
		}
		f.consumed[index] = consumed
		remaining -= consumed
	}
	if remaining > 0 {
		for index := len(f.sources) - 1; index >= 0; index-- {
			if f.consumed[index] > 0 {
				_ = f.sources[index].Refund()
			}
		}
		return errors.New("benefit composite funding insufficient")
	}
	return nil
}

func (f *CompositeFunding) Settle(delta int) error {
	if delta == 0 {
		for index, source := range f.sources {
			if f.consumed[index] <= 0 || source.Source() != BillingSourceBenefitVoucher {
				continue
			}
			if err := source.Settle(0); err != nil {
				return err
			}
		}
		return nil
	}
	if delta > 0 {
		adjustments := make([]int, len(f.sources))
		remaining := delta
		for index, source := range f.sources {
			if remaining <= 0 {
				break
			}
			capacity := remaining
			if available, ok := source.(fundingAdditionalCapacity); ok {
				capacity = available.AdditionalCapacity()
				if capacity < 0 {
					capacity = 0
				}
				if capacity > remaining {
					capacity = remaining
				}
			}
			adjustments[index] = capacity
			remaining -= capacity
		}
		if remaining > 0 {
			return errors.New("benefit composite funding settlement capacity insufficient")
		}
		for index, source := range f.sources {
			if adjustments[index] == 0 {
				continue
			}
			if err := source.Settle(adjustments[index]); err != nil {
				return err
			}
			f.consumed[index] += adjustments[index]
		}
		return nil
	}
	remaining := -delta
	for index := len(f.sources) - 1; index >= 0 && remaining > 0; index-- {
		refund := f.consumed[index]
		if refund > remaining {
			refund = remaining
		}
		if refund == 0 {
			continue
		}
		if err := f.sources[index].Settle(-refund); err != nil {
			return err
		}
		f.consumed[index] -= refund
		remaining -= refund
	}
	if remaining > 0 {
		return errors.New("benefit composite funding refund exceeds reservation")
	}
	return nil
}

func (f *CompositeFunding) Refund() error {
	var firstErr error
	for index := len(f.sources) - 1; index >= 0; index-- {
		if f.consumed[index] <= 0 {
			continue
		}
		if err := f.sources[index].Refund(); err != nil && firstErr == nil {
			firstErr = err
		}
		f.consumed[index] = 0
	}
	return firstErr
}

// ---------------------------------------------------------------------------
// WalletFunding — 钱包资金来源实现
// ---------------------------------------------------------------------------

// ErrInsufficientWalletQuota 钱包原子预扣失败（余额不足），未发生任何扣减。
// BillingSession 据此映射为 ErrorCodeInsufficientUserQuota，
// 使 wallet_first 等计费偏好可以回退到订阅。
var ErrInsufficientWalletQuota = errors.New("wallet quota insufficient")

type WalletFunding struct {
	userId   int
	consumed int // 实际预扣的用户额度
}

func (w *WalletFunding) Source() string { return BillingSourceWallet }

func (w *WalletFunding) PreConsume(amount int) error {
	if amount <= 0 {
		return nil
	}
	reserved, err := model.TryReserveUserQuota(w.userId, amount)
	if err != nil {
		return err
	}
	if !reserved {
		return ErrInsufficientWalletQuota
	}
	w.consumed = amount
	return nil
}

func (w *WalletFunding) Capacity() int {
	if w == nil || w.userId <= 0 {
		return 0
	}
	quota, err := model.GetUserQuota(w.userId, false)
	if err != nil || quota <= 0 {
		return 0
	}
	maxInt := int64(^uint(0) >> 1)
	if quota > maxInt {
		return int(maxInt)
	}
	return int(quota)
}

func (w *WalletFunding) AdditionalCapacity() int { return w.Capacity() }

func (w *WalletFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		return model.DecreaseUserQuota(w.userId, delta, false)
	}
	return model.IncreaseUserQuota(w.userId, -delta, false)
}

func (w *WalletFunding) Refund() error {
	if w.consumed <= 0 {
		return nil
	}
	// IncreaseUserQuota 是 quota += N 的非幂等操作，不能重试，否则会多退额度。
	// 订阅的 RefundSubscriptionPreConsume 有 requestId 幂等保护所以可以重试。
	return model.IncreaseUserQuota(w.userId, w.consumed, false)
}

// ---------------------------------------------------------------------------
// SubscriptionFunding — 订阅资金来源实现
// ---------------------------------------------------------------------------

type SubscriptionFunding struct {
	requestId      string
	userId         int
	modelName      string
	amount         int64 // 预扣的订阅额度（subConsume）
	subscriptionId int
	preConsumed    int64
	// 以下字段在 PreConsume 成功后填充，供 RelayInfo 同步使用
	AmountTotal     int64
	AmountUsedAfter int64
	PlanId          int
	PlanTitle       string
}

func (s *SubscriptionFunding) Source() string { return BillingSourceSubscription }

func (s *SubscriptionFunding) PreConsume(amount int) error {
	if amount > 0 {
		s.amount = int64(amount)
	}
	return s.preConsumeAmount(s.amount)
}

func (s *SubscriptionFunding) Capacity() int {
	if s == nil {
		return 0
	}
	available, err := model.GetUserSubscriptionAvailableQuota(s.userId)
	if err != nil || available <= 0 {
		return 0
	}
	maxInt := int64(^uint(0) >> 1)
	if available > maxInt {
		return int(maxInt)
	}
	return int(available)
}

func (s *SubscriptionFunding) AdditionalCapacity() int { return s.Capacity() }

func (s *SubscriptionFunding) preConsumeAmount(amount int64) error {
	res, err := model.PreConsumeUserSubscription(s.requestId, s.userId, s.modelName, 0, amount)
	if err != nil {
		return err
	}
	s.subscriptionId = res.UserSubscriptionId
	s.preConsumed = res.PreConsumed
	s.AmountTotal = res.AmountTotal
	s.AmountUsedAfter = res.AmountUsedAfter
	// 获取订阅计划信息
	if planInfo, err := model.GetSubscriptionPlanInfoByUserSubscriptionId(res.UserSubscriptionId); err == nil && planInfo != nil {
		s.PlanId = planInfo.PlanId
		s.PlanTitle = planInfo.PlanTitle
	}
	return nil
}

func (s *SubscriptionFunding) Settle(delta int) error {
	if delta == 0 {
		return nil
	}
	return model.PostConsumeUserSubscriptionDelta(s.subscriptionId, int64(delta))
}

func (s *SubscriptionFunding) Refund() error {
	if s.preConsumed <= 0 {
		return nil
	}
	return refundWithRetry(func() error {
		return model.RefundSubscriptionPreConsume(s.requestId)
	})
}

// refundWithRetry 尝试多次执行退款操作以提高成功率，只能用于基于事务的退款函数！！！！！！
// try to refund with retries, only for refund functions based on transactions!!!
func refundWithRetry(fn func() error) error {
	if fn == nil {
		return nil
	}
	const maxAttempts = 3
	var lastErr error
	for i := 0; i < maxAttempts; i++ {
		if err := fn(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if i < maxAttempts-1 {
			time.Sleep(time.Duration(200*(i+1)) * time.Millisecond)
		}
	}
	return lastErr
}
