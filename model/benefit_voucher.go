package model

import (
	"errors"
	"fmt"
	"math/rand/v2"
)

const (
	BenefitActivityStatusDraft      = "draft"
	BenefitActivityStatusPublished  = "published"
	BenefitActivityStatusPaused     = "paused"
	BenefitActivityStatusEnded      = "ended"
	BenefitActivityStatusTerminated = "terminated"

	BenefitAmountModeFixed  = "fixed"
	BenefitAmountModeRandom = "random"

	BenefitShareStatusAvailable = "available"
	BenefitShareStatusClaimed   = "claimed"
	BenefitShareStatusVoided    = "voided"
	BenefitShareStatusExpired   = "expired"

	BenefitVoucherStatusActive    = "active"
	BenefitVoucherStatusExhausted = "exhausted"
	BenefitVoucherStatusExpired   = "expired"
	BenefitVoucherStatusVoided    = "voided"

	BenefitLedgerTypePreConsume  = "pre_consume"
	BenefitLedgerTypeSettleDelta = "settle_delta"
	BenefitLedgerTypeRefund      = "refund"
	BenefitLedgerTypeVoid        = "void"
	BenefitLedgerTypeExpire      = "expire"
)

// BenefitActivity 保存福利活动配置和发布时的分组快照。
type BenefitActivity struct {
	Id                      int    `json:"id"`
	Name                    string `json:"name" gorm:"size:128;not null"`
	Description             string `json:"description" gorm:"type:text"`
	GroupId                 int    `json:"group_id" gorm:"index:idx_benefit_activity_group_status_time,priority:1"`
	GroupCodeSnapshot       string `json:"group_code_snapshot" gorm:"size:64;not null"`
	GroupNameSnapshot       string `json:"group_name_snapshot" gorm:"size:128;not null"`
	Status                  string `json:"status" gorm:"size:24;index:idx_benefit_activity_group_status_time,priority:2"`
	AmountMode              string `json:"amount_mode" gorm:"size:16;not null"`
	TotalAmountCents        int64  `json:"total_amount_cents"`
	TotalQuota              int64  `json:"total_quota"`
	TotalCount              int    `json:"total_count"`
	FixedAmountCents        int64  `json:"fixed_amount_cents"`
	MinAmountCents          int64  `json:"min_amount_cents"`
	MaxAmountCents          int64  `json:"max_amount_cents"`
	ClaimPaidThresholdCents int64  `json:"claim_paid_threshold_cents"`
	PersonalValidSeconds    int64  `json:"personal_valid_seconds"`
	StartsAt                int64  `json:"starts_at" gorm:"index:idx_benefit_activity_group_status_time,priority:3"`
	EndsAt                  int64  `json:"ends_at" gorm:"index:idx_benefit_activity_group_status_time,priority:4"`
	PublishedAt             int64  `json:"published_at"`
	CreatedBy               int    `json:"created_by"`
	UpdatedBy               int    `json:"updated_by"`
	TerminatedBy            int    `json:"terminated_by"`
	TerminatedAt            int64  `json:"terminated_at"`
	TerminateMode           string `json:"terminate_mode" gorm:"size:32"`
	TerminateReason         string `json:"terminate_reason" gorm:"type:text"`
	CreatedAt               int64  `json:"created_at"`
	UpdatedAt               int64  `json:"updated_at"`
}

func (BenefitActivity) TableName() string { return "benefit_activities" }

// BenefitActivityShare 是活动发布时预先确定的一份额度。
type BenefitActivityShare struct {
	Id               int    `json:"id"`
	ActivityId       int    `json:"activity_id" gorm:"index:idx_benefit_share_activity_status,priority:1"`
	AmountCents      int64  `json:"amount_cents"`
	Quota            int64  `json:"quota"`
	Status           string `json:"status" gorm:"size:24;index:idx_benefit_share_activity_status,priority:2"`
	ClaimedByUserId  int    `json:"claimed_by_user_id" gorm:"index"`
	ClaimedVoucherId int    `json:"claimed_voucher_id" gorm:"index"`
	ClaimedAt        int64  `json:"claimed_at"`
	CreatedAt        int64  `json:"created_at"`
}

func (BenefitActivityShare) TableName() string { return "benefit_activity_shares" }

// BenefitUserVoucher 保存用户领取后的独立福利余额。
type BenefitUserVoucher struct {
	Id                  int    `json:"id"`
	ActivityId          int    `json:"activity_id" gorm:"uniqueIndex:idx_benefit_voucher_activity_user,priority:1;index"`
	ShareId             int    `json:"share_id" gorm:"uniqueIndex"`
	UserId              int    `json:"user_id" gorm:"uniqueIndex:idx_benefit_voucher_activity_user,priority:2;index"`
	OriginalAmountCents int64  `json:"original_amount_cents"`
	OriginalQuota       int64  `json:"original_quota"`
	RemainingQuota      int64  `json:"remaining_quota"`
	UsedQuota           int64  `json:"used_quota"`
	Status              string `json:"status" gorm:"size:24;index"`
	ClaimedAt           int64  `json:"claimed_at"`
	ExpiresAt           int64  `json:"expires_at" gorm:"index"`
	VoidedAt            int64  `json:"voided_at"`
	VoidReason          string `json:"void_reason" gorm:"type:text"`
	CreatedAt           int64  `json:"created_at"`
	UpdatedAt           int64  `json:"updated_at"`
}

func (BenefitUserVoucher) TableName() string { return "benefit_user_vouchers" }

// BenefitVoucherLedger 保存券余额变化及请求/使用日志关联。
type BenefitVoucherLedger struct {
	Id           int    `json:"id"`
	ActivityId   int    `json:"activity_id" gorm:"index"`
	VoucherId    int    `json:"voucher_id" gorm:"uniqueIndex:idx_benefit_ledger_request_type,priority:1;index"`
	UserId       int    `json:"user_id" gorm:"index"`
	RequestId    string `json:"request_id" gorm:"size:64;uniqueIndex:idx_benefit_ledger_request_type,priority:2"`
	LogId        int    `json:"log_id" gorm:"index"`
	Type         string `json:"type" gorm:"size:24;uniqueIndex:idx_benefit_ledger_request_type,priority:3"`
	QuotaDelta   int64  `json:"quota_delta"`
	BalanceAfter int64  `json:"balance_after"`
	Metadata     string `json:"metadata" gorm:"type:text"`
	CreatedAt    int64  `json:"created_at"`
}

func (BenefitVoucherLedger) TableName() string { return "benefit_voucher_ledger" }

type BenefitShareSplitInput struct {
	Mode             string
	TotalAmountCents int64
	TotalCount       int
	FixedAmountCents int64
	MinAmountCents   int64
	MaxAmountCents   int64
	QuotaPerCent     int64
	RandomIntn       func(max int) int
}

type BenefitShareAllocation struct {
	AmountCents int64
	Quota       int64
}

func benefitMultiplyPositive(left, right int64) (int64, error) {
	if left <= 0 || right <= 0 {
		return 0, errors.New("福利券金额和数量必须大于 0")
	}
	if left > (1<<63-1)/right {
		return 0, errors.New("福利券金额计算溢出")
	}
	return left * right, nil
}

func benefitAllocation(amountCents, quotaPerCent int64) (BenefitShareAllocation, error) {
	quota, err := benefitMultiplyPositive(amountCents, quotaPerCent)
	if err != nil {
		return BenefitShareAllocation{}, err
	}
	return BenefitShareAllocation{AmountCents: amountCents, Quota: quota}, nil
}

// SplitBenefitShares 以整数分拆分活动预算，确保每份和总额均无浮点误差。
func SplitBenefitShares(input BenefitShareSplitInput) ([]BenefitShareAllocation, error) {
	if input.TotalAmountCents <= 0 || input.TotalCount <= 0 || input.QuotaPerCent <= 0 {
		return nil, errors.New("福利券总预算、总份数和每分额度必须大于 0")
	}
	count := int64(input.TotalCount)
	allocations := make([]BenefitShareAllocation, input.TotalCount)
	switch input.Mode {
	case BenefitAmountModeFixed:
		total, err := benefitMultiplyPositive(input.FixedAmountCents, count)
		if err != nil {
			return nil, err
		}
		if total != input.TotalAmountCents {
			return nil, fmt.Errorf("固定面额 %d 分乘 %d 份必须等于总预算 %d 分", input.FixedAmountCents, input.TotalCount, input.TotalAmountCents)
		}
		allocation, err := benefitAllocation(input.FixedAmountCents, input.QuotaPerCent)
		if err != nil {
			return nil, err
		}
		for index := range allocations {
			allocations[index] = allocation
		}
		return allocations, nil
	case BenefitAmountModeRandom:
		minimumTotal, err := benefitMultiplyPositive(input.MinAmountCents, count)
		if err != nil {
			return nil, err
		}
		maximumTotal, err := benefitMultiplyPositive(input.MaxAmountCents, count)
		if err != nil {
			return nil, err
		}
		if input.MaxAmountCents < input.MinAmountCents || input.TotalAmountCents < minimumTotal || input.TotalAmountCents > maximumTotal {
			return nil, fmt.Errorf("随机面额参数无效：需满足 %d 分 <= 总预算 <= %d 分", minimumTotal, maximumTotal)
		}

		randomIntn := input.RandomIntn
		if randomIntn == nil {
			randomIntn = rand.IntN
		}
		remaining := input.TotalAmountCents - minimumTotal
		capacity := input.MaxAmountCents - input.MinAmountCents
		maxInt := int64(^uint(0) >> 1)
		for index := range allocations {
			remainingSlots := int64(input.TotalCount - index - 1)
			minimumAddition := remaining - remainingSlots*capacity
			if minimumAddition < 0 {
				minimumAddition = 0
			}
			maximumAddition := capacity
			if remaining < maximumAddition {
				maximumAddition = remaining
			}
			addition := minimumAddition
			span := maximumAddition - minimumAddition
			if span > 0 {
				if span >= maxInt {
					return nil, errors.New("随机面额范围过大")
				}
				addition += int64(randomIntn(int(span + 1)))
			}
			allocation, err := benefitAllocation(input.MinAmountCents+addition, input.QuotaPerCent)
			if err != nil {
				return nil, err
			}
			allocations[index] = allocation
			remaining -= addition
		}
		if remaining != 0 {
			return nil, errors.New("随机面额拆分后仍有未分配预算")
		}
		for index := len(allocations) - 1; index > 0; index-- {
			swapIndex := randomIntn(index + 1)
			allocations[index], allocations[swapIndex] = allocations[swapIndex], allocations[index]
		}
		return allocations, nil
	default:
		return nil, fmt.Errorf("不支持的福利券面额模式: %s", input.Mode)
	}
}
