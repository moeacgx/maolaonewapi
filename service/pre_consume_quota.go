package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/bytedance/gopkg/util/gopool"
	"github.com/gin-gonic/gin"
)

func requestContextOrBackground(c *gin.Context) context.Context {
	if c == nil || c.Request == nil {
		return context.Background()
	}
	return c.Request.Context()
}

func ReturnPreConsumedQuota(c *gin.Context, relayInfo *relaycommon.RelayInfo) {
	if relayInfo.FinalPreConsumedQuota != 0 {
		logger.LogInfo(c, fmt.Sprintf("用户 %d 请求失败, 返还预扣费额度 %s", relayInfo.UserId, logger.FormatQuota(relayInfo.FinalPreConsumedQuota)))
		gopool.Go(func() {
			relayInfoCopy := *relayInfo

			err := PostConsumeQuota(&relayInfoCopy, -relayInfoCopy.FinalPreConsumedQuota, 0, false)
			if err != nil {
				common.SysLog("error return pre-consumed quota: " + err.Error())
			}
		})
	}
}

func NewUserQuotaQueryError(err error) *types.NewAPIError {
	if errors.Is(err, model.ErrUserQuotaCacheSync) {
		return types.NewErrorWithStatusCode(err, types.ErrorCodeQueryDataError, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry())
	}
	return types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
}

func newUserQuotaQueryError(err error) *types.NewAPIError {
	return NewUserQuotaQueryError(err)
}

func newSubscriptionQueryError(err error) *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		err,
		types.ErrorCodeQueryDataError,
		http.StatusServiceUnavailable,
		types.ErrOptionWithSkipRetry(),
	)
}

// NewUserQuotaUpdateError 将预上游的本地钱包写入故障标记为临时不可用。
// 这类错误不能归责渠道，也不能在同一次请求内跨渠道重试。
func NewUserQuotaUpdateError(err error) *types.NewAPIError {
	return types.NewErrorWithStatusCode(
		err, types.ErrorCodeUpdateDataError, http.StatusServiceUnavailable, types.ErrOptionWithSkipRetry(),
	)
}

// NewRealtimeQuotaError 保留已有额度错误分类，并把未知的本地预留错误标记为可重试的 503。
func NewRealtimeQuotaError(err error) *types.NewAPIError {
	var apiErr *types.NewAPIError
	if errors.As(err, &apiErr) {
		return apiErr
	}
	if errors.Is(err, model.ErrUserQuotaCacheSync) {
		return NewUserQuotaQueryError(err)
	}
	return NewUserQuotaUpdateError(err)
}

// PreConsumeQuota checks if the user has enough quota to pre-consume.
// It returns the pre-consumed quota if successful, or an error if not.
func PreConsumeQuota(c *gin.Context, preConsumedQuota int, relayInfo *relaycommon.RelayInfo) *types.NewAPIError {
	userQuota, err := model.GetUserQuotaWithContext(requestContextOrBackground(c), relayInfo.UserId, false)
	if err != nil {
		return newUserQuotaQueryError(err)
	}
	if userQuota <= 0 {
		return types.NewErrorWithStatusCode(fmt.Errorf("用户额度不足, 剩余额度: %s", logger.FormatQuota(userQuota)), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	if userQuota-preConsumedQuota < 0 {
		return types.NewErrorWithStatusCode(fmt.Errorf("预扣费额度失败, 用户剩余额度: %s, 需要预扣费额度: %s", logger.FormatQuota(userQuota), logger.FormatQuota(preConsumedQuota)), types.ErrorCodeInsufficientUserQuota, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}

	trustQuota := common.GetTrustQuota()

	relayInfo.UserQuota = userQuota
	if userQuota > trustQuota {
		// 用户额度充足，判断令牌额度是否充足
		if !relayInfo.TokenUnlimited {
			// 非无限令牌，判断令牌额度是否充足
			tokenQuota := c.GetInt("token_quota")
			if tokenQuota > trustQuota {
				// 令牌额度充足，信任令牌
				preConsumedQuota = 0
				logger.LogInfo(c, fmt.Sprintf("用户 %d 剩余额度 %s 且令牌 %d 额度 %d 充足, 信任且不需要预扣费", relayInfo.UserId, logger.FormatQuota(userQuota), relayInfo.TokenId, tokenQuota))
			}
		} else {
			// in this case, we do not pre-consume quota
			// because the user has enough quota
			preConsumedQuota = 0
			logger.LogInfo(c, fmt.Sprintf("用户 %d 额度充足且为无限额度令牌, 信任且不需要预扣费", relayInfo.UserId))
		}
	}

	if preConsumedQuota > 0 {
		tokenReservation, err := beginTokenQuotaReservation(relayInfo, preConsumedQuota)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodePreConsumeTokenQuotaFailed, http.StatusForbidden, types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
		}
		err = model.DecreaseUserQuota(relayInfo.UserId, preConsumedQuota, false)
		if err != nil {
			compensationErr := compensateRelayTokenQuota(
				relayInfo,
				tokenReservation,
				"legacy-initial",
				preConsumedQuota,
				preConsumedQuota,
			)
			return NewUserQuotaUpdateError(errors.Join(err, compensationErr))
		}
		if tokenReservation != nil {
			tokenReservation.Commit()
		}
		logger.LogInfo(c, fmt.Sprintf("用户 %d 预扣费 %s, 预扣费后剩余额度: %s", relayInfo.UserId, logger.FormatQuota(preConsumedQuota), logger.FormatQuota(userQuota-preConsumedQuota)))
	}
	relayInfo.FinalPreConsumedQuota = preConsumedQuota
	return nil
}
