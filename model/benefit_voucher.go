package model

import (
	"errors"
	"fmt"
	"math/big"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
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

	BenefitLedgerTypePreConsume  = "pre_consume"
	BenefitLedgerTypeSettleDelta = "settle_delta"
	BenefitLedgerTypeRefund      = "refund"
	BenefitLedgerTypeVoid        = "void"
	BenefitLedgerTypeExpire      = "expire"

	BenefitTerminateModeUnused = "unused"
	BenefitTerminateModeAll    = "all"
)

var (
	ErrBenefitActivityOverlap       = errors.New("同一分组的福利活动时间区间重叠")
	ErrBenefitActivityNotDraft      = errors.New("福利活动已发布，关键字段不可修改")
	ErrBenefitActivityNotClaimable  = errors.New("福利活动当前不可领取")
	ErrBenefitAlreadyClaimed        = errors.New("用户已领取该福利活动")
	ErrBenefitSoldOut               = errors.New("福利活动额度已领完")
	ErrBenefitClaimIneligible       = errors.New("用户不符合福利活动领取条件")
	ErrBenefitActivityTransition    = errors.New("福利活动状态转换无效")
	ErrBenefitActivityTerminateMode = errors.New("福利活动终止模式无效")
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

type BenefitActivityReport struct {
	TotalQuota         int64 `json:"total_quota"`
	UndistributedQuota int64 `json:"undistributed_quota"`
	DistributedQuota   int64 `json:"distributed_quota"`
	UsedQuota          int64 `json:"used_quota"`
	ExpiredUnusedQuota int64 `json:"expired_unused_quota"`
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

func ListBenefitActivitiesForUser(userID int, now int64) ([]BenefitActivityUserView, error) {
	if userID <= 0 {
		return nil, errors.New("用户 ID 无效")
	}
	var activities []BenefitActivity
	if err := DB.Where("status IN ?", []string{BenefitActivityStatusPublished, BenefitActivityStatusPaused, BenefitActivityStatusEnded, BenefitActivityStatusTerminated}).Order("starts_at DESC, id DESC").Find(&activities).Error; err != nil {
		return nil, err
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
		if err := expireBenefitActivityRecordsTx(DB, activity.Id, now); err != nil {
			return nil, err
		}
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
	return views, nil
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
	var vouchers []BenefitUserVoucher
	err := DB.Where("user_id = ?", userID).Order("id DESC").Find(&vouchers).Error
	return vouchers, err
}

func VoidBenefitVoucher(voucherID, operatorID int, reason string, now int64) error {
	reason = strings.TrimSpace(reason)
	if voucherID <= 0 || operatorID <= 0 || reason == "" {
		return errors.New("单券作废参数无效")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var voucher BenefitUserVoucher
		if err := lockForUpdate(tx).Where("id = ?", voucherID).First(&voucher).Error; err != nil {
			return err
		}
		if voucher.Status == BenefitVoucherStatusVoided {
			return nil
		}
		remaining := voucher.RemainingQuota
		if err := tx.Model(&voucher).Updates(map[string]interface{}{
			"remaining_quota": 0, "status": BenefitVoucherStatusVoided,
			"voided_at": now, "void_reason": reason, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&BenefitVoucherLedger{
			ActivityId: voucher.ActivityId, VoucherId: voucher.Id, UserId: voucher.UserId,
			RequestId: fmt.Sprintf("admin-void:%d:%d", operatorID, now), Type: BenefitLedgerTypeVoid,
			QuotaDelta: -remaining, BalanceAfter: 0,
			Metadata: common.MapToJsonStr(map[string]interface{}{"operator_id": operatorID, "reason": reason}), CreatedAt: now,
		}).Error
	})
}

func benefitPaidAmountCents(db *gorm.DB, userID int) (int64, error) {
	amount, err := getUserTotalRechargeAmountWithDB(db, userID)
	if err != nil {
		return 0, err
	}
	return decimal.NewFromFloat(amount).Mul(decimal.NewFromInt(100)).Round(0).IntPart(), nil
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
	result.PaidAmountCents, _ = benefitPaidAmountCents(tx, userID)
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
	if activity.TotalAmountCents <= 0 || activity.TotalQuota <= 0 || activity.TotalCount <= 0 {
		return errors.New("福利活动总预算、总额度和总份数必须大于 0")
	}
	if activity.ClaimPaidThresholdCents < 0 || activity.PersonalValidSeconds <= 0 {
		return errors.New("福利活动领取门槛不能为负且个人有效期必须大于 0")
	}
	if activity.StartsAt <= 0 || activity.EndsAt <= activity.StartsAt {
		return errors.New("福利活动开始时间必须早于结束时间")
	}
	_, err := SplitBenefitShares(BenefitShareSplitInput{
		Mode:             activity.AmountMode,
		TotalAmountCents: activity.TotalAmountCents,
		TotalCount:       activity.TotalCount,
		FixedAmountCents: activity.FixedAmountCents,
		MinAmountCents:   activity.MinAmountCents,
		MaxAmountCents:   activity.MaxAmountCents,
		QuotaPerCent:     1,
	})
	return err
}

func prepareBenefitAllocations(activity *BenefitActivity) ([]BenefitShareAllocation, error) {
	allocations, err := SplitBenefitShares(BenefitShareSplitInput{
		Mode:             activity.AmountMode,
		TotalAmountCents: activity.TotalAmountCents,
		TotalCount:       activity.TotalCount,
		FixedAmountCents: activity.FixedAmountCents,
		MinAmountCents:   activity.MinAmountCents,
		MaxAmountCents:   activity.MaxAmountCents,
		QuotaPerCent:     1,
	})
	if err != nil {
		return nil, err
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
	if err := lockForUpdate(tx).Where("id = ?", activityID).First(&activity).Error; err != nil {
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
			}
		}
		var vouchers []BenefitUserVoucher
		if err := tx.Where("activity_id = ?", activityID).Find(&vouchers).Error; err != nil {
			return err
		}
		for _, voucher := range vouchers {
			report.DistributedQuota += voucher.OriginalQuota
			report.UsedQuota += voucher.UsedQuota
			if voucher.Status == BenefitVoucherStatusExpired || voucher.Status == BenefitVoucherStatusVoided {
				report.ExpiredUnusedQuota += voucher.RemainingQuota
			}
		}
		return nil
	})
	return report, err
}
