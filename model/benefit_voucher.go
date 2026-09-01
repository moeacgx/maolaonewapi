package model

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
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

	BenefitLedgerTypePreConsume       = "pre_consume"
	BenefitLedgerTypeSettleDelta      = "settle_delta"
	BenefitLedgerTypeSettleRollback   = "settle_rollback"
	BenefitLedgerTypeRefundAdditional = "refund_additional"
	BenefitLedgerTypeRefund           = "refund"
	BenefitLedgerTypeVoid             = "void"
	BenefitLedgerTypeExpire           = "expire"

	BenefitTerminateModeUnused = "unused"
	BenefitTerminateModeAll    = "all"
)

var (
	ErrBenefitActivityOverlap       = errors.New("同一分组的福利活动时间区间重叠")
	ErrBenefitActivityNotDraft      = errors.New("福利活动已发布，关键字段不可修改")
	ErrBenefitActivityNotClaimable  = errors.New("福利活动当前不可领取")
	ErrBenefitAlreadyClaimed        = errors.New("用户已领取该福利活动")
	ErrBenefitSoldOut               = errors.New("福利活动额度已领完")
	ErrBenefitVoucherUnavailable    = errors.New("福利券不可用")
	ErrBenefitClaimIneligible       = errors.New("用户不符合福利活动领取条件")
	ErrBenefitActivityTransition    = errors.New("福利活动状态转换无效")
	ErrBenefitActivityTerminateMode = errors.New("福利活动终止模式无效")
	ErrBenefitVoucherForbidden      = errors.New("无权查看该福利券流水")
)

const (
	BenefitClaimReasonIneligible = "ineligible"
	BenefitClaimReasonClaimed    = "claimed"
	BenefitClaimReasonSoldOut    = "sold_out"
	BenefitClaimReasonInactive   = "inactive"
	BenefitClaimReasonNotStarted = "not_started"
	BenefitClaimReasonEnded      = "ended"
)

// BenefitActivity 保存福利活动配置和发布时的分组快照。
type BenefitActivity struct {
	Id                int    `json:"id"`
	Name              string `json:"name" gorm:"size:128;not null"`
	Description       string `json:"description" gorm:"type:text"`
	GroupId           int    `json:"group_id" gorm:"index:idx_benefit_activity_group_status_time,priority:1"`
	GroupCodeSnapshot string `json:"group_code_snapshot" gorm:"size:64;not null"`
	GroupNameSnapshot string `json:"group_name_snapshot" gorm:"size:128;not null"`
	Status            string `json:"status" gorm:"size:24;index:idx_benefit_activity_group_status_time,priority:2"`
	AmountMode        string `json:"amount_mode" gorm:"size:16;not null"`
	// *_Cents 仅是内部 0.01 元精度存储，管理接口统一暴露元金额。
	TotalAmountCents          int64  `json:"total_amount_cents"`
	TotalQuota                int64  `json:"total_quota"`
	FixedQuota                int64  `json:"fixed_quota"`
	MinQuota                  int64  `json:"min_quota"`
	MaxQuota                  int64  `json:"max_quota"`
	AmountDisplayTypeSnapshot string `json:"amount_display_type_snapshot" gorm:"size:16"`
	AmountDisplayRateSnapshot string `json:"amount_display_rate_snapshot" gorm:"size:64"`
	QuotaPerUnitSnapshot      string `json:"quota_per_unit_snapshot" gorm:"size:64"`
	TotalCount                int    `json:"total_count"`
	FixedAmountCents          int64  `json:"fixed_amount_cents"`
	MinAmountCents            int64  `json:"min_amount_cents"`
	MaxAmountCents            int64  `json:"max_amount_cents"`
	ClaimPaidThresholdCents   int64  `json:"claim_paid_threshold_cents"`
	// PersonalValidSeconds 仅用于数据库内部秒级计算；管理/用户 API 通过 personal_valid_hours 暴露。
	PersonalValidSeconds int64          `json:"-"`
	StartsAt             int64          `json:"starts_at" gorm:"index:idx_benefit_activity_group_status_time,priority:3"`
	EndsAt               int64          `json:"ends_at" gorm:"index:idx_benefit_activity_group_status_time,priority:4"`
	PublishedAt          int64          `json:"published_at"`
	CreatedBy            int            `json:"created_by"`
	UpdatedBy            int            `json:"updated_by"`
	TerminatedBy         int            `json:"terminated_by"`
	TerminatedAt         int64          `json:"terminated_at"`
	TerminateMode        string         `json:"terminate_mode" gorm:"size:32"`
	TerminateReason      string         `json:"terminate_reason" gorm:"type:text"`
	CreatedAt            int64          `json:"created_at"`
	UpdatedAt            int64          `json:"updated_at"`
	DeletedAt            gorm.DeletedAt `json:"-" gorm:"index"`
}

func (BenefitActivity) TableName() string { return "benefit_activities" }

// BenefitActivityShare 是活动发布时预先确定的一份额度。
type BenefitActivityShare struct {
	Id         int `json:"id"`
	ActivityId int `json:"activity_id" gorm:"index:idx_benefit_share_activity_status,priority:1"`
	// AmountCents 保存用于拆分的精确 0.01 元份额金额。
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
	ActivityName        string `json:"activity_name" gorm:"-:all"`
	GroupNameSnapshot   string `json:"group_name_snapshot" gorm:"-:all"`
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
	TotalQuota       int64
	FixedQuota       int64
	MinQuota         int64
	MaxQuota         int64
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

func benefitFormatYuanAmount(amountCents int64) string {
	return decimal.NewFromInt(amountCents).Div(decimal.NewFromInt(100)).StringFixed(2)
}

func benefitAllocation(amountCents, quotaPerCent int64) (BenefitShareAllocation, error) {
	quota, err := benefitMultiplyPositive(amountCents, quotaPerCent)
	if err != nil {
		return BenefitShareAllocation{}, err
	}
	return BenefitShareAllocation{AmountCents: amountCents, Quota: quota}, nil
}

// SplitBenefitShares 按 0.01 元粒度拆分活动预算，确保每份和总额均无浮点误差。
func SplitBenefitShares(input BenefitShareSplitInput) ([]BenefitShareAllocation, error) {
	if input.TotalQuota > 0 && (input.FixedQuota > 0 || input.MinQuota > 0 || input.MaxQuota > 0) {
		return splitBenefitQuotaShares(input)
	}
	if input.TotalAmountCents <= 0 || input.TotalCount <= 0 || input.QuotaPerCent <= 0 {
		return nil, errors.New("福利券总预算、总份数和额度单位必须大于 0")
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
			return nil, fmt.Errorf("固定面额 %s 元乘 %d 份必须等于总预算 %s 元", benefitFormatYuanAmount(input.FixedAmountCents), input.TotalCount, benefitFormatYuanAmount(input.TotalAmountCents))
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
			return nil, fmt.Errorf("随机面额参数无效：需满足 %s 元 <= 总预算 <= %s 元", benefitFormatYuanAmount(minimumTotal), benefitFormatYuanAmount(maximumTotal))
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

func splitBenefitQuotaShares(input BenefitShareSplitInput) ([]BenefitShareAllocation, error) {
	if input.TotalQuota <= 0 || input.TotalCount <= 0 {
		return nil, errors.New("福利券总额度和总份数必须大于 0")
	}
	count := int64(input.TotalCount)
	allocations := make([]BenefitShareAllocation, input.TotalCount)
	randomIntn := input.RandomIntn
	if randomIntn == nil {
		randomIntn = rand.IntN
	}
	switch input.Mode {
	case BenefitAmountModeFixed:
		if input.FixedQuota <= 0 || input.FixedQuota > (1<<63-1)/count || input.FixedQuota*count != input.TotalQuota {
			return nil, errors.New("固定面额额度乘总份数必须等于总额度")
		}
		for i := range allocations {
			allocations[i].Quota = input.FixedQuota
		}
	case BenefitAmountModeRandom:
		if input.MinQuota <= 0 || input.MaxQuota < input.MinQuota || input.MinQuota > (1<<63-1)/count || input.MaxQuota > (1<<63-1)/count {
			return nil, errors.New("随机面额额度范围无效")
		}
		minimumTotal := input.MinQuota * count
		maximumTotal := input.MaxQuota * count
		if input.TotalQuota < minimumTotal || input.TotalQuota > maximumTotal {
			return nil, errors.New("随机面额额度不在总额度范围内")
		}
		remaining := input.TotalQuota - minimumTotal
		capacity := input.MaxQuota - input.MinQuota
		for i := range allocations {
			remainingSlots := int64(input.TotalCount - i - 1)
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
				if span >= int64(^uint(0)>>1) {
					return nil, errors.New("随机面额额度范围过大")
				}
				addition += int64(randomIntn(int(span + 1)))
			}
			allocations[i].Quota = input.MinQuota + addition
			remaining -= addition
		}
		if remaining != 0 {
			return nil, errors.New("随机面额额度拆分后仍有未分配额度")
		}
		for i := len(allocations) - 1; i > 0; i-- {
			j := randomIntn(i + 1)
			allocations[i], allocations[j] = allocations[j], allocations[i]
		}
	default:
		return nil, fmt.Errorf("不支持的福利券面额模式: %s", input.Mode)
	}
	return allocations, nil
}

type BenefitActivityReport struct {
	TotalQuota         int64 `json:"total_quota"`
	UndistributedQuota int64 `json:"undistributed_quota"`
	DistributedQuota   int64 `json:"distributed_quota"`
	UsedQuota          int64 `json:"used_quota"`
	ExpiredUnusedQuota int64 `json:"expired_unused_quota"`
	TotalCount         int   `json:"total_count"`
	DistributedCount   int   `json:"distributed_count"`
	UsedCount          int   `json:"used_count"`
	ExpiredCount       int   `json:"expired_count"`
}

type BenefitVoucherReservation struct {
	VoucherID    int
	ActivityID   int
	UserID       int
	Reserved     int64
	BalanceAfter int64
}

func GetBenefitVoucherAvailableQuota(userID, groupID int, now int64) (int64, error) {
	if userID <= 0 || groupID <= 0 {
		return 0, errors.New("福利券查询参数无效")
	}
	if DB == nil || !DB.Migrator().HasTable(&BenefitUserVoucher{}) || !DB.Migrator().HasTable(&BenefitActivity{}) {
		return 0, nil
	}
	var voucher BenefitUserVoucher
	err := DB.Joins("JOIN benefit_activities ON benefit_activities.id = benefit_user_vouchers.activity_id").
		Where("benefit_user_vouchers.user_id = ? AND benefit_activities.group_id = ?", userID, groupID).
		Where("benefit_user_vouchers.status = ? AND benefit_user_vouchers.remaining_quota > ? AND benefit_user_vouchers.expires_at > ?", BenefitVoucherStatusActive, 0, now).
		Where("(benefit_activities.status IN ? OR (benefit_activities.status = ? AND benefit_activities.terminate_mode = ?)) AND benefit_activities.starts_at <= ? AND benefit_activities.ends_at > ?", []string{BenefitActivityStatusPublished, BenefitActivityStatusPaused}, BenefitActivityStatusTerminated, BenefitTerminateModeUnused, now, now).
		Order("benefit_user_vouchers.expires_at ASC, benefit_user_vouchers.id ASC").First(&voucher).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return voucher.RemainingQuota, nil
}

func ReserveBenefitVoucherQuota(requestID string, userID, groupID int, amount int64, now int64) (*BenefitVoucherReservation, error) {
	if strings.TrimSpace(requestID) == "" || userID <= 0 || groupID <= 0 || amount <= 0 {
		return nil, errors.New("福利券预扣参数无效")
	}
	if DB == nil || !DB.Migrator().HasTable(&BenefitUserVoucher{}) || !DB.Migrator().HasTable(&BenefitActivity{}) {
		return nil, ErrBenefitVoucherUnavailable
	}
	var reservation *BenefitVoucherReservation
	err := DB.Transaction(func(tx *gorm.DB) error {
		var existing BenefitVoucherLedger
		if err := lockForUpdate(tx).Where("request_id = ? AND type = ? AND user_id = ?", requestID, BenefitLedgerTypePreConsume, userID).First(&existing).Error; err == nil {
			reserved := -existing.QuotaDelta
			if amount <= reserved {
				reservation = &BenefitVoucherReservation{VoucherID: existing.VoucherId, ActivityID: existing.ActivityId, UserID: existing.UserId, Reserved: reserved, BalanceAfter: existing.BalanceAfter}
				return nil
			}
			additional := amount - reserved
			var voucher BenefitUserVoucher
			if err := lockForUpdate(tx).Where("id = ?", existing.VoucherId).First(&voucher).Error; err != nil {
				return err
			}
			var activity BenefitActivity
			if err := tx.Where("id = ?", voucher.ActivityId).First(&activity).Error; err != nil {
				return err
			}
			if !(slices.Contains([]string{BenefitActivityStatusPublished, BenefitActivityStatusPaused}, activity.Status) || (activity.Status == BenefitActivityStatusTerminated && activity.TerminateMode == BenefitTerminateModeUnused)) || now < activity.StartsAt || now >= activity.EndsAt {
				return ErrBenefitVoucherUnavailable
			}
			if voucher.Status != BenefitVoucherStatusActive || voucher.RemainingQuota < additional || voucher.ExpiresAt <= now {
				return ErrBenefitVoucherUnavailable
			}
			balanceAfter := voucher.RemainingQuota - additional
			status := BenefitVoucherStatusActive
			if balanceAfter == 0 {
				status = BenefitVoucherStatusExhausted
			}
			if err := tx.Model(&voucher).Updates(map[string]interface{}{"remaining_quota": balanceAfter, "status": status, "updated_at": now}).Error; err != nil {
				return err
			}
			if err := tx.Model(&existing).Updates(map[string]interface{}{"quota_delta": -amount, "balance_after": balanceAfter}).Error; err != nil {
				return err
			}
			reservation = &BenefitVoucherReservation{VoucherID: voucher.Id, ActivityID: voucher.ActivityId, UserID: voucher.UserId, Reserved: amount, BalanceAfter: balanceAfter}
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var voucher BenefitUserVoucher
		query := lockForUpdate(tx).Joins("JOIN benefit_activities ON benefit_activities.id = benefit_user_vouchers.activity_id").
			Where("benefit_user_vouchers.user_id = ? AND benefit_activities.group_id = ?", userID, groupID).
			Where("benefit_user_vouchers.status = ? AND benefit_user_vouchers.remaining_quota > ? AND benefit_user_vouchers.expires_at > ?", BenefitVoucherStatusActive, 0, now).
			Where("(benefit_activities.status IN ? OR (benefit_activities.status = ? AND benefit_activities.terminate_mode = ?)) AND benefit_activities.starts_at <= ? AND benefit_activities.ends_at > ?", []string{BenefitActivityStatusPublished, BenefitActivityStatusPaused}, BenefitActivityStatusTerminated, BenefitTerminateModeUnused, now, now).
			Order("benefit_user_vouchers.expires_at ASC, benefit_user_vouchers.id ASC")
		if err := query.First(&voucher).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBenefitVoucherUnavailable
			}
			return err
		}
		reserved := amount
		if voucher.RemainingQuota < reserved {
			reserved = voucher.RemainingQuota
		}
		balanceAfter := voucher.RemainingQuota - reserved
		status := BenefitVoucherStatusActive
		if balanceAfter == 0 {
			status = BenefitVoucherStatusExhausted
		}
		if err := tx.Model(&voucher).Updates(map[string]interface{}{"remaining_quota": balanceAfter, "status": status, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Create(&BenefitVoucherLedger{
			ActivityId: voucher.ActivityId, VoucherId: voucher.Id, UserId: userID,
			RequestId: requestID, Type: BenefitLedgerTypePreConsume,
			QuotaDelta: -reserved, BalanceAfter: balanceAfter, CreatedAt: now,
		}).Error; err != nil {
			return err
		}
		reservation = &BenefitVoucherReservation{VoucherID: voucher.Id, ActivityID: voucher.ActivityId, UserID: userID, Reserved: reserved, BalanceAfter: balanceAfter}
		return nil
	})
	return reservation, err
}

// RefundBenefitVoucherAdditional 释放后续 Reserve 追加的额度，保持原请求预扣流水幂等。
func RefundBenefitVoucherAdditional(requestID string, amount int64, now int64) error {
	if strings.TrimSpace(requestID) == "" || amount <= 0 {
		return errors.New("福利券追加预扣退款参数无效")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var pre BenefitVoucherLedger
		if err := lockForUpdate(tx).Where("request_id = ? AND type = ?", requestID, BenefitLedgerTypePreConsume).First(&pre).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		var voucher BenefitUserVoucher
		if err := lockForUpdate(tx).Where("id = ?", pre.VoucherId).First(&voucher).Error; err != nil {
			return err
		}
		var refund BenefitVoucherLedger
		refundErr := tx.Where("request_id = ? AND type = ?", requestID, BenefitLedgerTypeRefundAdditional).First(&refund).Error
		if refundErr != nil && !errors.Is(refundErr, gorm.ErrRecordNotFound) {
			return refundErr
		}
		reserved := -pre.QuotaDelta
		if refundErr == nil {
			var metadata struct {
				ReservedAfter int64 `json:"reserved_after"`
			}
			if common.UnmarshalJsonStr(refund.Metadata, &metadata) == nil && metadata.ReservedAfter == reserved {
				return nil
			}
		}
		if amount > reserved {
			return errors.New("福利券追加预扣退款超过预扣额度")
		}
		balanceAfter := voucher.RemainingQuota + amount
		if err := tx.Model(&voucher).Updates(map[string]interface{}{"remaining_quota": balanceAfter, "status": BenefitVoucherStatusActive, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.Model(&pre).Updates(map[string]interface{}{"quota_delta": -(reserved - amount), "balance_after": balanceAfter}).Error; err != nil {
			return err
		}
		if errors.Is(refundErr, gorm.ErrRecordNotFound) {
			return tx.Create(&BenefitVoucherLedger{
				ActivityId: pre.ActivityId, VoucherId: pre.VoucherId, UserId: pre.UserId,
				RequestId: requestID, Type: BenefitLedgerTypeRefundAdditional,
				QuotaDelta: amount, BalanceAfter: balanceAfter, CreatedAt: now,
				Metadata: common.MapToJsonStr(map[string]interface{}{"reserved_after": reserved - amount}),
			}).Error
		}
		return tx.Model(&refund).Updates(map[string]interface{}{
			"quota_delta": gorm.Expr("quota_delta + ?", amount), "balance_after": balanceAfter,
			"metadata": common.MapToJsonStr(map[string]interface{}{"reserved_after": reserved - amount}), "created_at": now,
		}).Error
	})
}

func SettleBenefitVoucherQuota(requestID string, delta int64, now int64) error {
	if strings.TrimSpace(requestID) == "" {
		return errors.New("福利券结算请求 ID 不能为空")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var pre BenefitVoucherLedger
		if err := lockForUpdate(tx).Where("request_id = ? AND type = ?", requestID, BenefitLedgerTypePreConsume).First(&pre).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		var settled BenefitVoucherLedger
		if err := tx.Where("request_id = ? AND type = ?", requestID, BenefitLedgerTypeSettleDelta).First(&settled).Error; err == nil {
			if settled.QuotaDelta != -delta {
				return errors.New("福利券重复结算额度与原结算不一致")
			}
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var voucher BenefitUserVoucher
		if err := lockForUpdate(tx).Where("id = ?", pre.VoucherId).First(&voucher).Error; err != nil {
			return err
		}
		reserved := -pre.QuotaDelta
		actual := reserved + delta
		if actual < 0 {
			return errors.New("福利券实际结算额度不能为负")
		}
		balanceAfter := voucher.RemainingQuota
		if delta > 0 {
			if voucher.RemainingQuota < delta {
				return errors.New("福利券余额不足以补扣")
			}
			balanceAfter -= delta
		} else if delta < 0 {
			balanceAfter += -delta
		}
		status := BenefitVoucherStatusActive
		if balanceAfter == 0 {
			status = BenefitVoucherStatusExhausted
		}
		if voucher.Status == BenefitVoucherStatusVoided || voucher.Status == BenefitVoucherStatusExpired {
			return errors.New("福利券已失效")
		}
		if err := tx.Model(&voucher).Updates(map[string]interface{}{
			"remaining_quota": balanceAfter, "used_quota": gorm.Expr("used_quota + ?", actual),
			"status": status, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&BenefitVoucherLedger{
			ActivityId: pre.ActivityId, VoucherId: pre.VoucherId, UserId: pre.UserId,
			RequestId: requestID, Type: BenefitLedgerTypeSettleDelta,
			QuotaDelta: -delta, BalanceAfter: balanceAfter, CreatedAt: now,
		}).Error
	})
}

// RollbackBenefitVoucherSettlement 撤销一次成功的结算操作。
// 使用独立流水类型和原请求键，既保留原结算记录又保证补偿幂等。
func RollbackBenefitVoucherSettlement(requestID string, delta int64, now int64) error {
	if strings.TrimSpace(requestID) == "" {
		return errors.New("福利券结算补偿请求 ID 不能为空")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var pre BenefitVoucherLedger
		if err := lockForUpdate(tx).Where("request_id = ? AND type = ?", requestID, BenefitLedgerTypePreConsume).First(&pre).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		var settled BenefitVoucherLedger
		if err := tx.Where("request_id = ? AND type = ?", requestID, BenefitLedgerTypeSettleDelta).First(&settled).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		if settled.QuotaDelta != -delta {
			return errors.New("福利券结算补偿额度与原结算不一致")
		}
		var existing BenefitVoucherLedger
		if err := tx.Where("request_id = ? AND type = ?", requestID, BenefitLedgerTypeSettleRollback).First(&existing).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var voucher BenefitUserVoucher
		if err := lockForUpdate(tx).Where("id = ?", pre.VoucherId).First(&voucher).Error; err != nil {
			return err
		}
		reserved := -pre.QuotaDelta
		actual := reserved + delta
		if actual < 0 || voucher.UsedQuota < actual {
			return errors.New("福利券结算补偿额度无效")
		}
		balanceAfter := voucher.RemainingQuota + delta
		if balanceAfter < 0 {
			return errors.New("福利券结算补偿后余额不能为负")
		}
		status := BenefitVoucherStatusActive
		if balanceAfter == 0 {
			status = BenefitVoucherStatusExhausted
		}
		if err := tx.Model(&voucher).Updates(map[string]interface{}{
			"remaining_quota": balanceAfter,
			"used_quota":      gorm.Expr("used_quota - ?", actual),
			"status":          status,
			"updated_at":      now,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&BenefitVoucherLedger{
			ActivityId: pre.ActivityId, VoucherId: pre.VoucherId, UserId: pre.UserId,
			RequestId: requestID, Type: BenefitLedgerTypeSettleRollback,
			QuotaDelta: delta, BalanceAfter: balanceAfter,
			Metadata: common.MapToJsonStr(map[string]interface{}{"original_request_id": requestID}), CreatedAt: now,
		}).Error
	})
}

func RefundBenefitVoucherQuota(requestID string, now int64) error {
	if strings.TrimSpace(requestID) == "" {
		return errors.New("福利券退款请求 ID 不能为空")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var pre BenefitVoucherLedger
		if err := lockForUpdate(tx).Where("request_id = ? AND type = ?", requestID, BenefitLedgerTypePreConsume).First(&pre).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		var existing BenefitVoucherLedger
		if err := tx.Where("request_id = ? AND type = ?", requestID, BenefitLedgerTypeRefund).First(&existing).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var settled BenefitVoucherLedger
		if err := tx.Where("request_id = ? AND type = ?", requestID, BenefitLedgerTypeSettleDelta).First(&settled).Error; err == nil {
			var rollback BenefitVoucherLedger
			if err := tx.Where("request_id = ? AND type = ?", requestID, BenefitLedgerTypeSettleRollback).First(&rollback).Error; errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			} else if err != nil {
				return err
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var voucher BenefitUserVoucher
		if err := lockForUpdate(tx).Where("id = ?", pre.VoucherId).First(&voucher).Error; err != nil {
			return err
		}
		if voucher.Status == BenefitVoucherStatusVoided || voucher.Status == BenefitVoucherStatusExpired {
			// 终态券不能因请求退款重新激活；保留零变更流水用于审计。
			return tx.Create(&BenefitVoucherLedger{
				ActivityId: pre.ActivityId, VoucherId: pre.VoucherId, UserId: pre.UserId,
				RequestId: requestID, Type: BenefitLedgerTypeRefund,
				QuotaDelta: 0, BalanceAfter: voucher.RemainingQuota, CreatedAt: now,
				Metadata: common.MapToJsonStr(map[string]interface{}{
					"not_restored": true, "terminal_status": voucher.Status,
				}),
			}).Error
		}
		refund := -pre.QuotaDelta
		balanceAfter := voucher.RemainingQuota + refund
		status := BenefitVoucherStatusActive
		if voucher.UsedQuota > 0 && balanceAfter == 0 {
			status = BenefitVoucherStatusExhausted
		}
		if err := tx.Model(&voucher).Updates(map[string]interface{}{"remaining_quota": balanceAfter, "status": status, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Create(&BenefitVoucherLedger{
			ActivityId: pre.ActivityId, VoucherId: pre.VoucherId, UserId: pre.UserId,
			RequestId: requestID, Type: BenefitLedgerTypeRefund,
			QuotaDelta: refund, BalanceAfter: balanceAfter, CreatedAt: now,
		}).Error
	})
}

type BenefitClaimEligibility struct {
	Eligible        bool   `json:"eligible"`
	Reason          string `json:"reason,omitempty"`
	PaidAmountCents int64  `json:"paid_amount_cents"`
	HasClaimed      bool   `json:"has_claimed"`
	SoldOut         bool   `json:"sold_out"`
}

type BenefitActivityUserView struct {
	BenefitActivity
	Eligible                   bool                `json:"eligible"`
	EligibilityReason          string              `json:"eligibility_reason,omitempty"`
	HasClaimed                 bool                `json:"has_claimed"`
	ClaimedVoucher             *BenefitUserVoucher `json:"claimed_voucher,omitempty"`
	SingleUserConcurrencyLimit int                 `json:"single_user_concurrency_limit"`
	RemainingCount             int                 `json:"remaining_count"`
}

func ListBenefitActivitiesForAdmin(offset, limit int) ([]BenefitActivity, int64, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var total int64
	if err := DB.Model(&BenefitActivity{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var activities []BenefitActivity
	err := DB.Order("id DESC").Offset(offset).Limit(limit).Find(&activities).Error
	return activities, total, err
}

func GetBenefitActivityForAdmin(activityID int) (*BenefitActivity, error) {
	var activity BenefitActivity
	if err := DB.Where("id = ?", activityID).First(&activity).Error; err != nil {
		return nil, err
	}
	return &activity, nil
}

// DeleteBenefitActivitiesByIDs 软删除已结束/已终止的历史活动，保留关联券、份额和流水。
// 进行中的活动必须先暂停、结束或终止，避免删除后仍有可领取或扣费记录。
func DeleteBenefitActivitiesByIDs(ids []int, now int64) (result BatchDeleteResult, err error) {
	if len(ids) == 0 {
		return result, errors.New("活动 ID 不能为空")
	}
	uniqueIDs := make([]int, 0, len(ids))
	seenIDs := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if _, exists := seenIDs[id]; exists {
			continue
		}
		seenIDs[id] = struct{}{}
		uniqueIDs = append(uniqueIDs, id)
	}
	result.DeletedIds = make([]int, 0, len(uniqueIDs))
	result.Skipped = make([]BatchDeleteSkipped, 0)
	err = DB.Transaction(func(tx *gorm.DB) error {
		var activities []BenefitActivity
		if err := lockForUpdate(tx).Where("id IN ?", uniqueIDs).Find(&activities).Error; err != nil {
			return err
		}
		found := make(map[int]struct{}, len(activities))
		for i := range activities {
			activity := &activities[i]
			found[activity.Id] = struct{}{}
			if activity.Status == BenefitActivityStatusDraft {
				var claimDataCount int64
				if err := tx.Model(&BenefitUserVoucher{}).Where("activity_id = ?", activity.Id).Count(&claimDataCount).Error; err != nil {
					return err
				}
				if claimDataCount > 0 {
					result.Skipped = append(result.Skipped, BatchDeleteSkipped{Id: activity.Id, Reason: "has_claim_data"})
					continue
				}
				var shareCount int64
				if err := tx.Model(&BenefitActivityShare{}).Where("activity_id = ?", activity.Id).Count(&shareCount).Error; err != nil {
					return err
				}
				if shareCount > 0 {
					result.Skipped = append(result.Skipped, BatchDeleteSkipped{Id: activity.Id, Reason: "has_claim_data"})
					continue
				}
			}
			if activity.Status == BenefitActivityStatusEnded || activity.Status == BenefitActivityStatusTerminated {
				if err := expireBenefitActivityRecordsTx(tx, activity.Id, now); err != nil {
					return err
				}
			}
			if (activity.Status == BenefitActivityStatusPublished || activity.Status == BenefitActivityStatusPaused) && activity.EndsAt > 0 && activity.EndsAt <= now {
				if err := expireBenefitActivitySharesTx(tx, activity.Id); err != nil {
					return err
				}
				if err := tx.Model(activity).Updates(map[string]interface{}{"status": BenefitActivityStatusEnded, "updated_at": now}).Error; err != nil {
					return err
				}
				activity.Status = BenefitActivityStatusEnded
			}
			if activity.Status != BenefitActivityStatusDraft && activity.Status != BenefitActivityStatusEnded && activity.Status != BenefitActivityStatusTerminated {
				result.Skipped = append(result.Skipped, BatchDeleteSkipped{Id: activity.Id, Reason: "not_deletable"})
				continue
			}
			if activity.Status == BenefitActivityStatusEnded || (activity.Status == BenefitActivityStatusTerminated && activity.TerminateMode == BenefitTerminateModeUnused) {
				var activeVoucherCount int64
				if err := tx.Model(&BenefitUserVoucher{}).Where("activity_id = ? AND status = ? AND remaining_quota > ?", activity.Id, BenefitVoucherStatusActive, 0).Count(&activeVoucherCount).Error; err != nil {
					return err
				}
				if activeVoucherCount > 0 {
					result.Skipped = append(result.Skipped, BatchDeleteSkipped{Id: activity.Id, Reason: "active_voucher"})
					continue
				}
			}
			if err := tx.Delete(activity).Error; err != nil {
				return err
			}
			result.DeletedIds = append(result.DeletedIds, activity.Id)
		}
		for _, id := range uniqueIDs {
			if _, ok := found[id]; !ok {
				result.Skipped = append(result.Skipped, BatchDeleteSkipped{Id: id, Reason: "not_found"})
			}
		}
		return nil
	})
	return result, err
}

func ListBenefitActivitiesForUser(userID int, now int64) ([]BenefitActivityUserView, error) {
	if userID <= 0 {
		return nil, errors.New("用户 ID 无效")
	}
	var activities []BenefitActivity
	if err := DB.Where("status IN ?", []string{BenefitActivityStatusPublished, BenefitActivityStatusPaused, BenefitActivityStatusEnded, BenefitActivityStatusTerminated}).Order("starts_at DESC, id DESC").Find(&activities).Error; err != nil {
		return nil, err
	}
	for index := range activities {
		activity := &activities[index]
		if err := expireBenefitActivityRecordsTx(DB, activity.Id, now); err != nil {
			return nil, err
		}
		if (activity.Status == BenefitActivityStatusPublished || activity.Status == BenefitActivityStatusPaused) && activity.EndsAt <= now {
			activity.Status = BenefitActivityStatusEnded
		}
	}
	var vouchers []BenefitUserVoucher
	if err := DB.Where("user_id = ?", userID).Order("id DESC").Find(&vouchers).Error; err != nil {
		return nil, err
	}
	voucherByActivity := make(map[int]*BenefitUserVoucher, len(vouchers))
	for index := range vouchers {
		voucher := vouchers[index]
		voucherByActivity[voucher.ActivityId] = &voucher
	}
	views := make([]BenefitActivityUserView, 0, len(activities))
	for index := range activities {
		activity := activities[index]
		view := BenefitActivityUserView{BenefitActivity: activity}
		if voucher := voucherByActivity[activity.Id]; voucher != nil {
			view.HasClaimed = true
			view.ClaimedVoucher = voucher
			view.EligibilityReason = BenefitClaimReasonClaimed
		} else {
			eligibility, err := GetBenefitClaimEligibility(userID, &activity, now)
			if err != nil {
				return nil, err
			}
			view.Eligible = eligibility.Eligible
			view.EligibilityReason = eligibility.Reason
		}
		var group Group
		if err := DB.Select("single_user_concurrency_limit").Where("id = ?", activity.GroupId).First(&group).Error; err == nil {
			view.SingleUserConcurrencyLimit = group.SingleUserConcurrencyLimit
		}
		views = append(views, view)
	}
	activityIDs := make([]int, 0, len(activities))
	for _, activity := range activities {
		activityIDs = append(activityIDs, activity.Id)
	}
	remainingCounts, err := getBenefitRemainingShareCounts(activityIDs)
	if err != nil {
		return nil, err
	}
	for i := range views {
		if views[i].Status == BenefitActivityStatusPublished && views[i].StartsAt <= now && views[i].EndsAt > now {
			views[i].RemainingCount = remainingCounts[views[i].Id]
		}
	}
	return views, nil
}

func getBenefitRemainingShareCounts(activityIDs []int) (map[int]int, error) {
	counts := make(map[int]int, len(activityIDs))
	if len(activityIDs) == 0 {
		return counts, nil
	}
	type countRow struct {
		ActivityId     int   `gorm:"column:activity_id"`
		RemainingCount int64 `gorm:"column:remaining_count"`
	}
	var rows []countRow
	if err := DB.Model(&BenefitActivityShare{}).
		Select("activity_id, COUNT(*) AS remaining_count").
		Where("activity_id IN ? AND status = ?", activityIDs, BenefitShareStatusAvailable).
		Group("activity_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.RemainingCount > 0 {
			counts[row.ActivityId] = int(row.RemainingCount)
		}
	}
	return counts, nil
}

func ListBenefitVouchersForUser(userID int, now int64) ([]BenefitUserVoucher, error) {
	if userID <= 0 {
		return nil, errors.New("用户 ID 无效")
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		var vouchers []BenefitUserVoucher
		if err := tx.Where("user_id = ?", userID).Find(&vouchers).Error; err != nil {
			return err
		}
		for index := range vouchers {
			voucher := &vouchers[index]
			if err := expireBenefitActivityRecordsTx(tx, voucher.ActivityId, now); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	type voucherViewRow struct {
		BenefitUserVoucher
		ActivityName      string `gorm:"column:activity_name"`
		GroupNameSnapshot string `gorm:"column:group_name_snapshot"`
	}
	var rows []voucherViewRow
	err := DB.Unscoped().Table("benefit_user_vouchers AS v").
		Select("v.*, a.name AS activity_name, a.group_name_snapshot AS group_name_snapshot").
		Joins("LEFT JOIN benefit_activities AS a ON a.id = v.activity_id").
		Where("v.user_id = ?", userID).Order("v.id DESC").Scan(&rows).Error
	vouchers := make([]BenefitUserVoucher, len(rows))
	for i := range rows {
		vouchers[i] = rows[i].BenefitUserVoucher
		vouchers[i].ActivityName = rows[i].ActivityName
		vouchers[i].GroupNameSnapshot = rows[i].GroupNameSnapshot
	}
	return vouchers, err
}

type BenefitVoucherListFilter struct {
	Keyword string
	Status  string
}

type BenefitVoucherAdminView struct {
	BenefitUserVoucher
	ActivityName      string `json:"activity_name"`
	GroupNameSnapshot string `json:"group_name_snapshot"`
	Username          string `json:"username"`
}

func ListBenefitVouchersForAdmin(activityID int, filter BenefitVoucherListFilter, offset, limit int) ([]BenefitVoucherAdminView, int64, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	base := DB.Unscoped().Table("benefit_user_vouchers AS v").
		Joins("LEFT JOIN benefit_activities AS a ON a.id = v.activity_id").
		Joins("LEFT JOIN users AS u ON u.id = v.user_id")
	if activityID > 0 {
		base = base.Where("v.activity_id = ?", activityID)
	}
	if filter.Status != "" {
		base = base.Where("v.status = ?", filter.Status)
	}
	if keyword := strings.TrimSpace(filter.Keyword); keyword != "" {
		like := "%" + strings.ToLower(keyword) + "%"
		base = base.Where("LOWER(u.username) LIKE ? OR LOWER(a.name) LIKE ? OR LOWER(a.group_name_snapshot) LIKE ?", like, like, like)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	views := make([]BenefitVoucherAdminView, 0)
	err := base.Select("v.*, a.name AS activity_name, a.group_name_snapshot AS group_name_snapshot, u.username AS username").
		Order("v.id DESC").Offset(offset).Limit(limit).Scan(&views).Error
	return views, total, err
}

func GetBenefitVoucherLedgerForUser(voucherID, userID int) ([]BenefitVoucherLedger, error) {
	if voucherID <= 0 || userID <= 0 {
		return nil, errors.New("福利券流水查询参数无效")
	}
	var voucher BenefitUserVoucher
	if err := DB.Where("id = ?", voucherID).First(&voucher).Error; err != nil {
		return nil, err
	}
	if voucher.UserId != userID {
		return nil, ErrBenefitVoucherForbidden
	}
	var ledger []BenefitVoucherLedger
	if err := DB.Where("voucher_id = ? AND user_id = ?", voucherID, userID).Order("id ASC").Find(&ledger).Error; err != nil {
		return nil, err
	}
	return ledger, nil
}

func VoidBenefitVouchers(ids []int, operatorID int, reason string, now int64) (result BenefitVoucherBatchResult, err error) {
	if len(ids) == 0 || operatorID <= 0 || strings.TrimSpace(reason) == "" {
		return result, errors.New("批量作废参数无效")
	}
	uniqueIDs := make([]int, 0, len(ids))
	seen := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			return result, errors.New("福利券 ID 必须为正整数")
		}
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			uniqueIDs = append(uniqueIDs, id)
		}
	}
	result.UpdatedIds = make([]int, 0, len(uniqueIDs))
	result.Skipped = make([]BatchDeleteSkipped, 0)
	err = DB.Transaction(func(tx *gorm.DB) error {
		var vouchers []BenefitUserVoucher
		if err := lockForUpdate(tx).Where("id IN ?", uniqueIDs).Find(&vouchers).Error; err != nil {
			return err
		}
		byID := make(map[int]*BenefitUserVoucher, len(vouchers))
		for i := range vouchers {
			byID[vouchers[i].Id] = &vouchers[i]
		}
		for _, id := range uniqueIDs {
			voucher, ok := byID[id]
			if !ok {
				result.Skipped = append(result.Skipped, BatchDeleteSkipped{Id: id, Reason: "not_found"})
				continue
			}
			if voucher.Status != BenefitVoucherStatusActive || voucher.RemainingQuota <= 0 {
				result.Skipped = append(result.Skipped, BatchDeleteSkipped{Id: id, Reason: "not_active"})
				continue
			}
			remaining := voucher.RemainingQuota
			if err := tx.Model(voucher).Updates(map[string]interface{}{
				"remaining_quota": 0, "status": BenefitVoucherStatusVoided,
				"voided_at": now, "void_reason": reason, "updated_at": now,
			}).Error; err != nil {
				return err
			}
			if err := tx.Create(&BenefitVoucherLedger{
				ActivityId: voucher.ActivityId, VoucherId: voucher.Id, UserId: voucher.UserId,
				RequestId: fmt.Sprintf("admin-void:%d:%d:%d", operatorID, now, voucher.Id),
				Type:      BenefitLedgerTypeVoid, QuotaDelta: -remaining, BalanceAfter: 0,
				Metadata: common.MapToJsonStr(map[string]interface{}{"operator_id": operatorID, "reason": reason}), CreatedAt: now,
			}).Error; err != nil {
				return err
			}
			result.UpdatedIds = append(result.UpdatedIds, id)
		}
		return nil
	})
	return result, err
}

func VoidBenefitVoucher(voucherID, operatorID int, reason string, now int64) error {
	reason = strings.TrimSpace(reason)
	if voucherID <= 0 || operatorID <= 0 || reason == "" {
		return errors.New("单券作废参数无效")
	}
	result, err := VoidBenefitVouchers([]int{voucherID}, operatorID, reason, now)
	if err != nil {
		return err
	}
	if len(result.UpdatedIds) == 1 || (len(result.Skipped) == 1 && result.Skipped[0].Reason == "not_active") {
		return nil
	}
	return gorm.ErrRecordNotFound
}

func LinkBenefitLedgerLogID(requestID string, logID int) error {
	if strings.TrimSpace(requestID) == "" || logID <= 0 || DB == nil || !DB.Migrator().HasTable(&BenefitVoucherLedger{}) {
		return nil
	}
	return DB.Model(&BenefitVoucherLedger{}).Where("request_id = ? AND log_id = 0", requestID).Update("log_id", logID).Error
}

func benefitPaidAmountCents(db *gorm.DB, userID int) (int64, error) {
	amount, err := getUserTotalRechargeAmountWithDB(db, userID)
	if err != nil {
		return 0, err
	}
	return decimal.NewFromFloat(amount).Mul(decimal.NewFromInt(100)).Round(0).IntPart(), nil
}

// BenefitAmountCNYToQuota 将按 0.01 元精度保存的活动金额换算为计费使用的内部 quota。
func BenefitAmountCNYToQuota(amountMinorUnits int64) (int64, error) {
	if amountMinorUnits <= 0 {
		return 0, errors.New("福利活动金额必须大于 0")
	}
	rate := operation_setting.USDExchangeRate
	if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0, errors.New("人民币兑美元汇率配置无效")
	}
	if common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
		return 0, errors.New("额度单位配置无效")
	}
	amountCNY := decimal.NewFromInt(amountMinorUnits).Div(decimal.NewFromInt(100))
	quota := amountCNY.
		Div(decimal.NewFromFloat(rate)).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	converted, err := common.WalletQuotaFromDecimalStrict(quota)
	if err != nil || converted <= 0 {
		return 0, errors.New("福利活动金额换算后的额度无效")
	}
	return converted, nil
}

func getBenefitClaimEligibilityTx(tx *gorm.DB, userID int, activity *BenefitActivity, now int64) (BenefitClaimEligibility, error) {
	result := BenefitClaimEligibility{}
	if activity == nil {
		return result, errors.New("福利活动不能为空")
	}
	if activity.Status != BenefitActivityStatusPublished {
		result.Reason = BenefitClaimReasonInactive
		return result, nil
	}
	if now < activity.StartsAt {
		result.Reason = BenefitClaimReasonNotStarted
		return result, nil
	}
	if now >= activity.EndsAt {
		result.Reason = BenefitClaimReasonEnded
		return result, nil
	}
	var user User
	if err := tx.Select("id", "created_at").Where("id = ?", userID).First(&user).Error; err != nil {
		return result, err
	}
	paidAmountCents, paidErr := benefitPaidAmountCents(tx, userID)
	if paidErr != nil {
		return result, paidErr
	}
	result.PaidAmountCents = paidAmountCents
	if user.CreatedAt > 0 && now-user.CreatedAt < int64((30*time.Minute).Seconds()) {
		result.Reason = BenefitClaimReasonIneligible
		return result, nil
	}
	var claimedCount int64
	if err := tx.Model(&BenefitUserVoucher{}).Where("activity_id = ? AND user_id = ?", activity.Id, userID).Count(&claimedCount).Error; err != nil {
		return result, err
	}
	if claimedCount > 0 {
		result.HasClaimed = true
		result.Reason = BenefitClaimReasonClaimed
		return result, nil
	}
	if result.PaidAmountCents < activity.ClaimPaidThresholdCents {
		result.Reason = BenefitClaimReasonIneligible
		return result, nil
	}
	var availableCount int64
	if err := tx.Model(&BenefitActivityShare{}).Where("activity_id = ? AND status = ?", activity.Id, BenefitShareStatusAvailable).Count(&availableCount).Error; err != nil {
		return result, err
	}
	if availableCount == 0 {
		result.SoldOut = true
		result.Reason = BenefitClaimReasonSoldOut
		return result, nil
	}
	result.Eligible = true
	return result, nil
}

func GetBenefitClaimEligibility(userID int, activity *BenefitActivity, now int64) (BenefitClaimEligibility, error) {
	return getBenefitClaimEligibilityTx(DB, userID, activity, now)
}

func ClaimBenefitActivity(activityID, userID int, now int64) (*BenefitUserVoucher, error) {
	var voucher BenefitUserVoucher
	err := DB.Transaction(func(tx *gorm.DB) error {
		var activity BenefitActivity
		if err := lockForUpdate(tx).Where("id = ?", activityID).First(&activity).Error; err != nil {
			return err
		}
		if err := expireBenefitActivityRecordsTx(tx, activityID, now); err != nil {
			return err
		}
		if activity.Status != BenefitActivityStatusPublished || now < activity.StartsAt || now >= activity.EndsAt {
			return ErrBenefitActivityNotClaimable
		}
		eligibility, err := getBenefitClaimEligibilityTx(tx, userID, &activity, now)
		if err != nil {
			return err
		}
		if eligibility.HasClaimed {
			return ErrBenefitAlreadyClaimed
		}
		if !eligibility.Eligible {
			if eligibility.SoldOut {
				return ErrBenefitSoldOut
			}
			return ErrBenefitClaimIneligible
		}
		var shares []BenefitActivityShare
		if err := lockForUpdate(tx).Where("activity_id = ? AND status = ?", activityID, BenefitShareStatusAvailable).Find(&shares).Error; err != nil {
			return err
		}
		if len(shares) == 0 {
			return ErrBenefitSoldOut
		}
		share := shares[rand.IntN(len(shares))]
		expiresAt := now + activity.PersonalValidSeconds
		if activity.EndsAt < expiresAt {
			expiresAt = activity.EndsAt
		}
		voucher = BenefitUserVoucher{
			ActivityId: activity.Id, ShareId: share.Id, UserId: userID,
			OriginalAmountCents: share.AmountCents, OriginalQuota: share.Quota,
			RemainingQuota: share.Quota, Status: BenefitVoucherStatusActive,
			ClaimedAt: now, ExpiresAt: expiresAt, CreatedAt: now, UpdatedAt: now,
		}
		if err := tx.Create(&voucher).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return ErrBenefitAlreadyClaimed
			}
			return err
		}
		updated := tx.Model(&BenefitActivityShare{}).Where("id = ? AND status = ?", share.Id, BenefitShareStatusAvailable).Updates(map[string]interface{}{
			"status": BenefitShareStatusClaimed, "claimed_by_user_id": userID,
			"claimed_voucher_id": voucher.Id, "claimed_at": now,
		})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrBenefitSoldOut
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &voucher, nil
}

func validateBenefitActivity(activity *BenefitActivity) error {
	if activity == nil {
		return errors.New("福利活动不能为空")
	}
	activity.Name = strings.TrimSpace(activity.Name)
	activity.Description = strings.TrimSpace(activity.Description)
	if activity.Name == "" || len([]rune(activity.Name)) > 128 {
		return errors.New("福利活动名称长度必须在 1 到 128 个字符之间")
	}
	if activity.GroupId <= 0 {
		return errors.New("福利活动必须绑定有效分组")
	}
	if activity.TotalQuota <= 0 || activity.TotalCount <= 0 || (activity.FixedQuota <= 0 && activity.MinQuota <= 0 && activity.MaxQuota <= 0 && activity.TotalAmountCents <= 0) {
		return errors.New("福利活动总预算、总额度和总份数必须大于 0")
	}
	if activity.ClaimPaidThresholdCents < 0 || activity.PersonalValidSeconds <= 0 {
		return errors.New("福利活动领取门槛不能为负且个人有效期必须大于 0")
	}
	if activity.StartsAt <= 0 || activity.EndsAt <= activity.StartsAt {
		return errors.New("福利活动开始时间必须早于结束时间")
	}
	input := BenefitShareSplitInput{
		Mode:             activity.AmountMode,
		TotalAmountCents: activity.TotalAmountCents,
		TotalCount:       activity.TotalCount,
		FixedAmountCents: activity.FixedAmountCents,
		MinAmountCents:   activity.MinAmountCents,
		MaxAmountCents:   activity.MaxAmountCents,
		QuotaPerCent:     1,
		TotalQuota:       activity.TotalQuota,
		FixedQuota:       activity.FixedQuota,
		MinQuota:         activity.MinQuota,
		MaxQuota:         activity.MaxQuota,
	}
	if activity.FixedQuota > 0 || activity.MinQuota > 0 || activity.MaxQuota > 0 {
		input.TotalAmountCents = 0
		input.QuotaPerCent = 0
	}
	_, err := SplitBenefitShares(input)
	return err
}

func prepareBenefitAllocations(activity *BenefitActivity) ([]BenefitShareAllocation, error) {
	input := BenefitShareSplitInput{
		Mode:             activity.AmountMode,
		TotalAmountCents: activity.TotalAmountCents,
		TotalCount:       activity.TotalCount,
		FixedAmountCents: activity.FixedAmountCents,
		MinAmountCents:   activity.MinAmountCents,
		MaxAmountCents:   activity.MaxAmountCents,
		QuotaPerCent:     1,
		TotalQuota:       activity.TotalQuota,
		FixedQuota:       activity.FixedQuota,
		MinQuota:         activity.MinQuota,
		MaxQuota:         activity.MaxQuota,
	}
	if activity.FixedQuota > 0 || activity.MinQuota > 0 || activity.MaxQuota > 0 {
		input.TotalAmountCents = 0
		input.QuotaPerCent = 0
	}
	allocations, err := SplitBenefitShares(input)
	if err != nil {
		return nil, err
	}
	if activity.FixedQuota > 0 || activity.MinQuota > 0 || activity.MaxQuota > 0 {
		return allocations, nil
	}
	totalAmount := big.NewInt(activity.TotalAmountCents)
	totalQuota := big.NewInt(activity.TotalQuota)
	var allocatedQuota int64
	for index := range allocations {
		product := new(big.Int).Mul(big.NewInt(allocations[index].AmountCents), totalQuota)
		quota := new(big.Int).Quo(product, totalAmount)
		if !quota.IsInt64() {
			return nil, errors.New("福利券份额额度计算溢出")
		}
		allocations[index].Quota = quota.Int64()
		allocatedQuota += allocations[index].Quota
	}
	remaining := activity.TotalQuota - allocatedQuota
	for index := int64(0); index < remaining; index++ {
		allocations[int(index%int64(len(allocations)))].Quota++
	}
	return allocations, nil
}

func benefitActivityOverlapTx(tx *gorm.DB, activity *BenefitActivity) (bool, error) {
	var count int64
	err := tx.Model(&BenefitActivity{}).
		Where("group_id = ? AND id <> ?", activity.GroupId, activity.Id).
		Where("status IN ?", []string{BenefitActivityStatusPublished, BenefitActivityStatusPaused}).
		Where("starts_at < ? AND ends_at > ?", activity.EndsAt, activity.StartsAt).
		Count(&count).Error
	return count > 0, err
}

func CreateBenefitActivity(activity *BenefitActivity, operatorID int, now int64) error {
	if err := validateBenefitActivity(activity); err != nil {
		return err
	}
	var group Group
	if err := DB.Where("id = ? AND status = ?", activity.GroupId, GroupStatusActive).First(&group).Error; err != nil {
		return err
	}
	activity.Status = BenefitActivityStatusDraft
	activity.GroupCodeSnapshot = group.Code
	activity.GroupNameSnapshot = group.Name
	activity.CreatedBy = operatorID
	activity.UpdatedBy = operatorID
	activity.CreatedAt = now
	activity.UpdatedAt = now
	return DB.Create(activity).Error
}

func UpdateBenefitActivityDraft(activity *BenefitActivity, operatorID int, now int64) error {
	if activity == nil || activity.Id <= 0 {
		return errors.New("福利活动 ID 无效")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var stored BenefitActivity
		if err := lockForUpdate(tx).Where("id = ?", activity.Id).First(&stored).Error; err != nil {
			return err
		}
		if stored.Status != BenefitActivityStatusDraft {
			return ErrBenefitActivityNotDraft
		}
		candidate := *activity
		candidate.Status = stored.Status
		candidate.CreatedBy = stored.CreatedBy
		candidate.CreatedAt = stored.CreatedAt
		candidate.UpdatedBy = operatorID
		candidate.UpdatedAt = now
		if err := validateBenefitActivity(&candidate); err != nil {
			return err
		}
		var group Group
		if err := tx.Where("id = ? AND status = ?", candidate.GroupId, GroupStatusActive).First(&group).Error; err != nil {
			return err
		}
		candidate.GroupCodeSnapshot = group.Code
		candidate.GroupNameSnapshot = group.Name
		if err := tx.Save(&candidate).Error; err != nil {
			return err
		}
		*activity = candidate
		return nil
	})
}

func UpdateBenefitActivityMetadata(activityID int, name, description string, operatorID int, now int64) (*BenefitActivity, error) {
	name = strings.TrimSpace(name)
	if activityID <= 0 || name == "" || len([]rune(name)) > 128 {
		return nil, errors.New("福利活动名称长度必须在 1 到 128 个字符之间")
	}
	var activity BenefitActivity
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ?", activityID).First(&activity).Error; err != nil {
			return err
		}
		if activity.Status == BenefitActivityStatusDraft {
			return ErrBenefitActivityNotDraft
		}
		activity.Name = name
		activity.Description = strings.TrimSpace(description)
		activity.UpdatedBy = operatorID
		activity.UpdatedAt = now
		return tx.Save(&activity).Error
	})
	return &activity, err
}

func PublishBenefitActivity(activityID, operatorID int, now int64) (*BenefitActivity, error) {
	var published BenefitActivity
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ?", activityID).First(&published).Error; err != nil {
			return err
		}
		if published.Status != BenefitActivityStatusDraft {
			return ErrBenefitActivityNotDraft
		}
		if err := validateBenefitActivity(&published); err != nil {
			return err
		}
		overlap, err := benefitActivityOverlapTx(tx, &published)
		if err != nil {
			return err
		}
		if overlap {
			return ErrBenefitActivityOverlap
		}
		allocations, err := prepareBenefitAllocations(&published)
		if err != nil {
			return err
		}
		shares := make([]BenefitActivityShare, len(allocations))
		for index, allocation := range allocations {
			shares[index] = BenefitActivityShare{
				ActivityId: published.Id, AmountCents: allocation.AmountCents,
				Quota: allocation.Quota, Status: BenefitShareStatusAvailable, CreatedAt: now,
			}
		}
		if err := tx.Create(&shares).Error; err != nil {
			return err
		}
		published.Status = BenefitActivityStatusPublished
		published.PublishedAt = now
		published.UpdatedBy = operatorID
		published.UpdatedAt = now
		return tx.Save(&published).Error
	})
	return &published, err
}

func TransitionBenefitActivity(activityID, operatorID int, targetStatus string, now int64) (*BenefitActivity, error) {
	var activity BenefitActivity
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("id = ?", activityID).First(&activity).Error; err != nil {
			return err
		}
		switch targetStatus {
		case BenefitActivityStatusPaused:
			if activity.Status != BenefitActivityStatusPublished {
				return ErrBenefitActivityTransition
			}
		case BenefitActivityStatusPublished:
			if activity.Status != BenefitActivityStatusPaused {
				return ErrBenefitActivityTransition
			}
			overlap, err := benefitActivityOverlapTx(tx, &activity)
			if err != nil {
				return err
			}
			if overlap {
				return ErrBenefitActivityOverlap
			}
		case BenefitActivityStatusEnded:
			if activity.Status != BenefitActivityStatusPublished && activity.Status != BenefitActivityStatusPaused {
				return ErrBenefitActivityTransition
			}
			activity.EndsAt = now
		default:
			return ErrBenefitActivityTransition
		}
		activity.Status = targetStatus
		activity.UpdatedBy = operatorID
		activity.UpdatedAt = now
		if err := tx.Save(&activity).Error; err != nil {
			return err
		}
		if targetStatus == BenefitActivityStatusEnded {
			return expireBenefitActivitySharesTx(tx, activity.Id)
		}
		return nil
	})
	return &activity, err
}

func TerminateBenefitActivity(activityID, operatorID int, mode, reason string, now int64) error {
	reason = strings.TrimSpace(reason)
	if mode != BenefitTerminateModeUnused && mode != BenefitTerminateModeAll {
		return ErrBenefitActivityTerminateMode
	}
	if reason == "" {
		return errors.New("福利活动终止原因不能为空")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var activity BenefitActivity
		if err := lockForUpdate(tx).Where("id = ?", activityID).First(&activity).Error; err != nil {
			return err
		}
		if activity.Status != BenefitActivityStatusPublished && activity.Status != BenefitActivityStatusPaused {
			return ErrBenefitActivityTransition
		}
		if err := tx.Model(&BenefitActivityShare{}).
			Where("activity_id = ? AND status = ?", activityID, BenefitShareStatusAvailable).
			Update("status", BenefitShareStatusVoided).Error; err != nil {
			return err
		}
		if mode == BenefitTerminateModeAll {
			var vouchers []BenefitUserVoucher
			if err := lockForUpdate(tx).Where("activity_id = ? AND status IN ?", activityID, []string{BenefitVoucherStatusActive, BenefitVoucherStatusExhausted}).Find(&vouchers).Error; err != nil {
				return err
			}
			for index := range vouchers {
				voucher := &vouchers[index]
				remaining := voucher.RemainingQuota
				if err := tx.Model(voucher).Updates(map[string]interface{}{
					"remaining_quota": 0, "status": BenefitVoucherStatusVoided,
					"voided_at": now, "void_reason": reason, "updated_at": now,
				}).Error; err != nil {
					return err
				}
				ledger := BenefitVoucherLedger{
					ActivityId: activityID, VoucherId: voucher.Id, UserId: voucher.UserId,
					RequestId: fmt.Sprintf("void:%d", now), Type: BenefitLedgerTypeVoid,
					QuotaDelta: -remaining, BalanceAfter: 0,
					Metadata: common.MapToJsonStr(map[string]interface{}{"reason": reason, "mode": mode}), CreatedAt: now,
				}
				if err := tx.Create(&ledger).Error; err != nil {
					return err
				}
			}
		}
		activity.Status = BenefitActivityStatusTerminated
		activity.TerminatedBy = operatorID
		activity.TerminatedAt = now
		activity.TerminateMode = mode
		activity.TerminateReason = reason
		activity.UpdatedBy = operatorID
		activity.UpdatedAt = now
		return tx.Save(&activity).Error
	})
}

func expireBenefitActivityRecordsTx(tx *gorm.DB, activityID int, now int64) error {
	var activity BenefitActivity
	if err := lockForUpdate(tx).Unscoped().Where("id = ?", activityID).First(&activity).Error; err != nil {
		return err
	}
	if (activity.Status == BenefitActivityStatusPublished || activity.Status == BenefitActivityStatusPaused) && activity.EndsAt <= now {
		activity.Status = BenefitActivityStatusEnded
		activity.UpdatedAt = now
		if err := tx.Save(&activity).Error; err != nil {
			return err
		}
		if err := expireBenefitActivitySharesTx(tx, activityID); err != nil {
			return err
		}
	}
	var vouchers []BenefitUserVoucher
	if err := lockForUpdate(tx).
		Where("activity_id = ? AND status = ? AND expires_at <= ?", activityID, BenefitVoucherStatusActive, now).
		Find(&vouchers).Error; err != nil {
		return err
	}
	for index := range vouchers {
		voucher := &vouchers[index]
		if err := tx.Model(voucher).Updates(map[string]interface{}{"status": BenefitVoucherStatusExpired, "updated_at": now}).Error; err != nil {
			return err
		}
		ledger := BenefitVoucherLedger{
			ActivityId: activityID, VoucherId: voucher.Id, UserId: voucher.UserId,
			RequestId: fmt.Sprintf("expire:%d", voucher.ExpiresAt), Type: BenefitLedgerTypeExpire,
			QuotaDelta: -voucher.RemainingQuota, BalanceAfter: 0, CreatedAt: now,
		}
		if err := tx.Create(&ledger).Error; err != nil {
			return err
		}
	}
	return nil
}

func expireBenefitActivitySharesTx(tx *gorm.DB, activityID int) error {
	return tx.Model(&BenefitActivityShare{}).
		Where("activity_id = ? AND status = ?", activityID, BenefitShareStatusAvailable).
		Update("status", BenefitShareStatusExpired).Error
}

func GetBenefitActivityReport(activityID int, now int64) (*BenefitActivityReport, error) {
	report := &BenefitActivityReport{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := expireBenefitActivityRecordsTx(tx, activityID, now); err != nil {
			return err
		}
		var activity BenefitActivity
		if err := tx.Where("id = ?", activityID).First(&activity).Error; err != nil {
			return err
		}
		report.TotalQuota = activity.TotalQuota
		report.TotalCount = activity.TotalCount
		var shares []BenefitActivityShare
		if err := tx.Where("activity_id = ?", activityID).Find(&shares).Error; err != nil {
			return err
		}
		for _, share := range shares {
			switch share.Status {
			case BenefitShareStatusAvailable:
				report.UndistributedQuota += share.Quota
			case BenefitShareStatusExpired, BenefitShareStatusVoided:
				report.ExpiredUnusedQuota += share.Quota
				if share.Status == BenefitShareStatusExpired {
					report.ExpiredCount++
				}
			}
		}
		var vouchers []BenefitUserVoucher
		if err := tx.Where("activity_id = ?", activityID).Find(&vouchers).Error; err != nil {
			return err
		}
		for _, voucher := range vouchers {
			report.DistributedQuota += voucher.OriginalQuota
			report.DistributedCount++
			report.UsedQuota += voucher.UsedQuota
			if voucher.UsedQuota > 0 {
				report.UsedCount++
			}
			if voucher.Status == BenefitVoucherStatusExpired {
				report.ExpiredCount++
			}
			if voucher.Status == BenefitVoucherStatusExpired || voucher.Status == BenefitVoucherStatusVoided {
				unused := voucher.OriginalQuota - voucher.UsedQuota
				if unused > 0 {
					report.ExpiredUnusedQuota += unused
				}
			}
		}
		return nil
	})
	return report, err
}
