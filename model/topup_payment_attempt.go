package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	PaymentMethodBepusdt = "bepusdt"
	PaymentMethodOkpay   = "okpay"

	PaymentProviderBepusdt = "bepusdt"
	PaymentProviderOkpay   = "okpay"
)

const (
	TopUpPaymentAttemptCreated      = "created"
	TopUpPaymentAttemptLaunched     = "launched"
	TopUpPaymentAttemptLaunchFailed = "launch_failed"
	TopUpPaymentAttemptSucceeded    = "succeeded"
)

var (
	ErrTopUpPaymentAttemptNotFound      = errors.New("topup payment attempt not found")
	ErrTopUpPaymentAttemptMismatch      = errors.New("topup payment attempt mismatch")
	ErrTopUpPaymentAttemptStatusInvalid = errors.New("topup payment attempt status invalid")
)

// TopUpPaymentAttempt is an append-only gateway checkout snapshot. A retry
// creates another row; it never rewrites the snapshot used by an older signed
// callback.
type TopUpPaymentAttempt struct {
	Id               int    `json:"id"`
	TopUpId          int    `json:"topup_id" gorm:"index;not null"`
	TradeNo          string `json:"trade_no" gorm:"type:varchar(255);index;not null"`
	PaymentProvider  string `json:"payment_provider" gorm:"type:varchar(50);index:idx_topup_attempt_provider_order,priority:1;not null"`
	PaymentMethod    string `json:"payment_method" gorm:"type:varchar(50);not null"`
	ProviderOrderId  string `json:"provider_order_id" gorm:"type:varchar(255);index:idx_topup_attempt_provider_order,priority:2"`
	ProviderAmount   string `json:"provider_amount" gorm:"type:varchar(64);not null"`
	ProviderCurrency string `json:"provider_currency" gorm:"type:varchar(32);not null"`
	Status           string `json:"status" gorm:"type:varchar(32);index;not null"`
	FailureReason    string `json:"-" gorm:"type:varchar(255)"`
	CreateTime       int64  `json:"create_time" gorm:"index"`
	UpdateTime       int64  `json:"update_time"`
}

func normalizeTopUpPaymentSnapshot(provider, method, amount, currency string) (string, string, string, string, error) {
	provider = strings.TrimSpace(provider)
	method = strings.TrimSpace(method)
	amount = strings.TrimSpace(amount)
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if provider == "" || method == "" || amount == "" || currency == "" {
		return "", "", "", "", ErrTopUpPaymentAttemptMismatch
	}
	parsed, err := decimal.NewFromString(amount)
	if err != nil || parsed.IsNegative() {
		return "", "", "", "", ErrTopUpPaymentAttemptMismatch
	}
	return provider, method, amount, currency, nil
}

// CreateTopUpPaymentAttempt appends a checkout snapshot before contacting the
// provider. Failed launches remain auditable and callback-eligible so a signed
// late success cannot orphan a payable provider session.
func CreateTopUpPaymentAttempt(tradeNo, provider, method, amount, currency string) (*TopUpPaymentAttempt, error) {
	tradeNo = strings.TrimSpace(tradeNo)
	provider, method, amount, currency, err := normalizeTopUpPaymentSnapshot(provider, method, amount, currency)
	if err != nil || tradeNo == "" {
		return nil, ErrTopUpPaymentAttemptMismatch
	}

	attempt := &TopUpPaymentAttempt{}
	err = DB.Transaction(func(tx *gorm.DB) error {
		var topUp TopUp
		if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).First(&topUp).Error; err != nil {
			return ErrTopUpNotFound
		}
		if topUp.Status == common.TopUpStatusSuccess {
			return ErrTopUpStatusInvalid
		}
		switch topUp.Status {
		case common.TopUpStatusPending:
		case common.TopUpStatusFailed, common.TopUpStatusExpired:
			topUp.Status = common.TopUpStatusPending
			if err := tx.Model(&topUp).Update("status", common.TopUpStatusPending).Error; err != nil {
				return err
			}
		default:
			return ErrTopUpStatusInvalid
		}
		now := common.GetTimestamp()
		attempt = &TopUpPaymentAttempt{
			TopUpId:          topUp.Id,
			TradeNo:          topUp.TradeNo,
			PaymentProvider:  provider,
			PaymentMethod:    method,
			ProviderAmount:   amount,
			ProviderCurrency: currency,
			Status:           TopUpPaymentAttemptCreated,
			CreateTime:       now,
			UpdateTime:       now,
		}
		return tx.Create(attempt).Error
	})
	if err != nil {
		return nil, err
	}
	return attempt, nil
}

func MarkTopUpPaymentAttemptLaunched(attemptId int, providerOrderId string) error {
	providerOrderId = strings.TrimSpace(providerOrderId)
	if attemptId <= 0 {
		return ErrTopUpPaymentAttemptNotFound
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var attempt TopUpPaymentAttempt
		if err := lockForUpdate(tx).First(&attempt, attemptId).Error; err != nil {
			return ErrTopUpPaymentAttemptNotFound
		}
		if attempt.Status != TopUpPaymentAttemptCreated {
			if attempt.ProviderOrderId == providerOrderId {
				switch attempt.Status {
				case TopUpPaymentAttemptLaunched, TopUpPaymentAttemptLaunchFailed, TopUpPaymentAttemptSucceeded:
					return nil
				}
			}
			return ErrTopUpPaymentAttemptStatusInvalid
		}
		if providerOrderId != "" {
			var count int64
			if err := tx.Model(&TopUpPaymentAttempt{}).
				Where("payment_provider = ? AND provider_order_id = ? AND id <> ?", attempt.PaymentProvider, providerOrderId, attempt.Id).
				Count(&count).Error; err != nil {
				return err
			}
			if count != 0 {
				return ErrTopUpPaymentAttemptMismatch
			}
		}
		return tx.Model(&attempt).Updates(map[string]interface{}{
			"provider_order_id": providerOrderId,
			"status":            TopUpPaymentAttemptLaunched,
			"update_time":       common.GetTimestamp(),
		}).Error
	})
}

func MarkTopUpPaymentAttemptLaunchFailed(attemptId int, reason string) error {
	if attemptId <= 0 {
		return ErrTopUpPaymentAttemptNotFound
	}
	reason = strings.TrimSpace(reason)
	if len(reason) > 255 {
		reason = reason[:255]
	}
	result := DB.Model(&TopUpPaymentAttempt{}).
		Where("id = ? AND status = ?", attemptId, TopUpPaymentAttemptCreated).
		Updates(map[string]interface{}{
			"status":         TopUpPaymentAttemptLaunchFailed,
			"failure_reason": reason,
			"update_time":    common.GetTimestamp(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrTopUpPaymentAttemptStatusInvalid
	}
	return nil
}

// ResolveTopUpPaymentAttempt finds the exact provider attempt. Providers that
// do not issue an order id (EPay) resolve to the newest callback-eligible
// snapshot for the signed merchant trade number.
func ResolveTopUpPaymentAttempt(provider, tradeNo, providerOrderId string) (*TopUpPaymentAttempt, error) {
	provider = strings.TrimSpace(provider)
	tradeNo = strings.TrimSpace(tradeNo)
	providerOrderId = strings.TrimSpace(providerOrderId)
	if provider == "" || (tradeNo == "" && providerOrderId == "") {
		return nil, ErrTopUpPaymentAttemptMismatch
	}

	query := DB.Where("payment_provider = ? AND status IN ?", provider, []string{
		TopUpPaymentAttemptLaunched,
		TopUpPaymentAttemptLaunchFailed,
		TopUpPaymentAttemptSucceeded,
	})
	if providerOrderId != "" {
		query = query.Where("provider_order_id = ?", providerOrderId)
	}
	if tradeNo != "" {
		query = query.Where("trade_no = ?", tradeNo)
	}
	var attempts []TopUpPaymentAttempt
	if err := query.Order("id DESC").Limit(2).Find(&attempts).Error; err != nil {
		return nil, err
	}
	if len(attempts) == 0 && providerOrderId != "" && tradeNo != "" {
		if err := DB.Where("payment_provider = ? AND trade_no = ? AND provider_order_id = ? AND status IN ?", provider, tradeNo, "", []string{TopUpPaymentAttemptCreated, TopUpPaymentAttemptLaunchFailed}).Order("id DESC").Limit(2).Find(&attempts).Error; err != nil {
			return nil, err
		}
		if len(attempts) > 1 {
			return nil, ErrTopUpPaymentAttemptMismatch
		}
	}
	if len(attempts) == 0 {
		return nil, ErrTopUpPaymentAttemptNotFound
	}
	if providerOrderId != "" && len(attempts) != 1 {
		return nil, ErrTopUpPaymentAttemptMismatch
	}
	return &attempts[0], nil
}

// BindTopUpPaymentAttemptProviderOrder claims an initially-unbound attempt
// after a signed callback proves that the provider created a session despite
// an ambiguous launch failure. A different provider order can never rebind it.
func BindTopUpPaymentAttemptProviderOrder(attemptId int, providerOrderId string) error {
	providerOrderId = strings.TrimSpace(providerOrderId)
	if attemptId <= 0 || providerOrderId == "" {
		return ErrTopUpPaymentAttemptMismatch
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var attempt TopUpPaymentAttempt
		if err := lockForUpdate(tx).First(&attempt, attemptId).Error; err != nil {
			return ErrTopUpPaymentAttemptNotFound
		}
		if attempt.ProviderOrderId == providerOrderId {
			return nil
		}
		if attempt.ProviderOrderId != "" {
			return ErrTopUpPaymentAttemptMismatch
		}
		var count int64
		if err := tx.Model(&TopUpPaymentAttempt{}).Where("payment_provider = ? AND provider_order_id = ? AND id <> ?", attempt.PaymentProvider, providerOrderId, attempt.Id).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrTopUpPaymentAttemptMismatch
		}
		updates := map[string]interface{}{"provider_order_id": providerOrderId, "update_time": common.GetTimestamp()}
		if attempt.Status == TopUpPaymentAttemptCreated {
			updates["status"] = TopUpPaymentAttemptLaunched
		}
		return tx.Model(&attempt).Updates(updates).Error
	})
}

func ValidateTopUpPaymentAttemptSnapshot(attempt *TopUpPaymentAttempt, provider, providerOrderId, amount, currency string, tolerance decimal.Decimal) error {
	if attempt == nil || attempt.Id <= 0 || attempt.PaymentProvider != strings.TrimSpace(provider) {
		return ErrTopUpPaymentAttemptMismatch
	}
	providerOrderId = strings.TrimSpace(providerOrderId)
	if attempt.ProviderOrderId != "" && providerOrderId != attempt.ProviderOrderId {
		return fmt.Errorf("provider order mismatch: %w", ErrTopUpPaymentAttemptMismatch)
	}
	actualAmount, err := decimal.NewFromString(strings.TrimSpace(amount))
	if err != nil {
		return fmt.Errorf("invalid callback amount: %w", ErrTopUpPaymentAttemptMismatch)
	}
	expectedAmount, err := decimal.NewFromString(attempt.ProviderAmount)
	if err != nil {
		return fmt.Errorf("invalid stored amount: %w", ErrTopUpPaymentAttemptMismatch)
	}
	if tolerance.IsNegative() {
		tolerance = decimal.Zero
	}
	if actualAmount.Sub(expectedAmount).Abs().GreaterThan(tolerance) {
		return fmt.Errorf("amount mismatch: expected=%s actual=%s: %w", expectedAmount.String(), actualAmount.String(), ErrTopUpPaymentAttemptMismatch)
	}
	if strings.ToUpper(strings.TrimSpace(currency)) != attempt.ProviderCurrency {
		return fmt.Errorf("currency mismatch: expected=%s actual=%s: %w", attempt.ProviderCurrency, currency, ErrTopUpPaymentAttemptMismatch)
	}
	return nil
}

func HasTopUpPaymentAttempts(tradeNo, provider string) (bool, error) {
	var count int64
	err := DB.Model(&TopUpPaymentAttempt{}).
		Where("trade_no = ? AND payment_provider = ?", strings.TrimSpace(tradeNo), strings.TrimSpace(provider)).
		Count(&count).Error
	return count != 0, err
}

// AllowLegacyTopUpCallback intentionally bounds fallback to recent pre-attempt
// orders. New orders always have attempts; old callbacks age out with the
// existing 30-day topup query window.
func AllowLegacyTopUpCallback(topUp *TopUp, provider string) bool {
	return topUp != nil &&
		topUp.PaymentProvider == strings.TrimSpace(provider) &&
		topUp.CreateTime >= topUpQueryCutoff()
}

func completeTopUpPaymentAttempt(attemptId int, tradeNo, provider, method, callerIp string, legacy bool, userUpdates map[string]interface{}) (bool, error) {
	tradeNo = strings.TrimSpace(tradeNo)
	provider = strings.TrimSpace(provider)
	method = strings.TrimSpace(method)
	if tradeNo == "" || provider == "" {
		return false, ErrTopUpPaymentAttemptMismatch
	}

	var topUp TopUp
	var quotaToAdd int
	alreadyDone := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).First(&topUp).Error; err != nil {
			return ErrTopUpNotFound
		}

		var attempt TopUpPaymentAttempt
		if !legacy {
			if err := lockForUpdate(tx).First(&attempt, attemptId).Error; err != nil {
				return ErrTopUpPaymentAttemptNotFound
			}
			if attempt.TopUpId != topUp.Id || attempt.TradeNo != topUp.TradeNo || attempt.PaymentProvider != provider {
				return ErrTopUpPaymentAttemptMismatch
			}
			switch attempt.Status {
			case TopUpPaymentAttemptLaunched, TopUpPaymentAttemptLaunchFailed, TopUpPaymentAttemptSucceeded:
			default:
				return ErrTopUpPaymentAttemptStatusInvalid
			}
		} else if !AllowLegacyTopUpCallback(&topUp, provider) {
			return ErrTopUpPaymentAttemptMismatch
		}

		if topUp.Status == common.TopUpStatusSuccess {
			alreadyDone = true
			return nil
		}
		switch topUp.Status {
		case common.TopUpStatusPending, common.TopUpStatusFailed, common.TopUpStatusExpired:
		default:
			return ErrTopUpStatusInvalid
		}

		if topUp.CreditedQuota > 0 {
			quotaToAdd = topUp.CreditedQuota
		} else {
			var err error
			quotaToAdd, err = common.QuotaFromDecimalStrict(
				decimal.NewFromInt(topUp.Amount).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
			)
			if err != nil || quotaToAdd <= 0 {
				return ErrInvalidTopUpQuota
			}
		}
		topUp.PaymentProvider = provider
		if method != "" {
			topUp.PaymentMethod = method
		}
		if !legacy {
			topUp.ProviderOrderId = attempt.ProviderOrderId
			topUp.ProviderAmount = attempt.ProviderAmount
			topUp.ProviderCurrency = attempt.ProviderCurrency
		}
		if topUp.RequestIP == "" {
			topUp.RequestIP = strings.TrimSpace(callerIp)
		}
		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(&topUp).Error; err != nil {
			return err
		}
		if err := creditTopUpQuota(tx, topUp.UserId, quotaToAdd, userUpdates); err != nil {
			return err
		}
		if err := recordTopUpPromoUsageTx(tx, &topUp, false); err != nil {
			return err
		}
		if err := CreateInvoiceRecordFromTopUpTx(tx, &topUp); err != nil {
			return err
		}
		if err := CreateAffiliateRewardsForPaymentTx(tx, topUp.UserId, AffiliateSourceTopUp, topUp.TradeNo, topUpAffiliateSourceQuota(&topUp, quotaToAdd)); err != nil {
			return err
		}
		if !legacy {
			if err := tx.Model(&attempt).Updates(map[string]interface{}{
				"status":      TopUpPaymentAttemptSucceeded,
				"update_time": common.GetTimestamp(),
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	if alreadyDone {
		return true, nil
	}
	syncCreditUserQuotaCache(topUp.UserId, quotaToAdd, provider+" topup")
	RecordTopupLog(topUp.UserId, fmt.Sprintf("使用%s充值成功，充值金额: %v，支付金额：%.2f", provider, logger.LogQuota(quotaToAdd), topUp.Money), callerIp, topUp.PaymentMethod, provider)
	return false, nil
}

func CompleteTopUpPaymentAttempt(attemptId int, tradeNo, provider, method, callerIp string) (bool, error) {
	return completeTopUpPaymentAttempt(attemptId, tradeNo, provider, method, callerIp, false, nil)
}

func CompleteStripeTopUpPaymentAttempt(attemptId int, tradeNo, customerId, callerIp string) (bool, error) {
	updates := map[string]interface{}{}
	if strings.TrimSpace(customerId) != "" {
		updates["stripe_customer"] = strings.TrimSpace(customerId)
	}
	return completeTopUpPaymentAttempt(attemptId, tradeNo, PaymentProviderStripe, PaymentMethodStripe, callerIp, false, updates)
}

func CompleteCreemTopUpPaymentAttempt(attemptId int, tradeNo, customerEmail, callerIp string) (bool, error) {
	var updates map[string]interface{}
	if customerEmail = strings.TrimSpace(customerEmail); customerEmail != "" {
		updates = map[string]interface{}{
			"email": gorm.Expr("CASE WHEN COALESCE(email, '') = '' THEN ? ELSE email END", customerEmail),
		}
	}
	return completeTopUpPaymentAttempt(attemptId, tradeNo, PaymentProviderCreem, PaymentMethodCreem, callerIp, false, updates)
}

func CompleteLegacyTopUpPayment(tradeNo, provider, method, callerIp string) (bool, error) {
	return completeTopUpPaymentAttempt(0, tradeNo, provider, method, callerIp, true, nil)
}
