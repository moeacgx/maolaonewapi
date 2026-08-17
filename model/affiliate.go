package model

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	AffiliateSourceTopUp        = "topup"
	AffiliateSourceSubscription = "subscription"
	AffiliateSourceRedemption   = "redemption"
)

const (
	AffiliateRecordStatusPending     = "pending"
	AffiliateRecordStatusAvailable   = "available"
	AffiliateRecordStatusConfiscated = "confiscated"
)

const (
	AffiliatePayoutMethodUSDT   = "usdt"
	AffiliatePayoutMethodAlipay = "alipay"
	AffiliatePayoutMethodWechat = "wechat"
)

const (
	AffiliateWithdrawalStatusPending  = "pending"
	AffiliateWithdrawalStatusApproved = "approved"
	AffiliateWithdrawalStatusPaid     = "paid"
	AffiliateWithdrawalStatusRejected = "rejected"
)

type AffiliateRecord struct {
	Id                int    `json:"id"`
	UserId            int    `json:"user_id" gorm:"index"`
	InviteeId         int    `json:"invitee_id" gorm:"index"`
	Level             int    `json:"level" gorm:"index;uniqueIndex:idx_affiliate_record_source,priority:3"`
	SourceType        string `json:"source_type" gorm:"type:varchar(32);index;uniqueIndex:idx_affiliate_record_source,priority:1"`
	SourceId          string `json:"source_id" gorm:"type:varchar(255);index;uniqueIndex:idx_affiliate_record_source,priority:2"`
	SourceQuota       int    `json:"source_quota"`
	RewardQuota       int    `json:"reward_quota"`
	Ratio             int    `json:"ratio"`
	Status            string `json:"status" gorm:"type:varchar(32);index"`
	BalanceAfterQuota int    `json:"balance_after_quota"`
	AvailableTime     int64  `json:"available_time" gorm:"index"`
	SettledTime       int64  `json:"settled_time"`
	CreatedAt         int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt         int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type AffiliateBalance struct {
	Id               int   `json:"id"`
	UserId           int   `json:"user_id" gorm:"uniqueIndex"`
	PendingQuota     int   `json:"pending_quota"`
	AvailableQuota   int   `json:"available_quota"`
	FrozenQuota      int   `json:"frozen_quota"`
	RiskFrozenQuota  int   `json:"risk_frozen_quota"`
	ConfiscatedQuota int   `json:"confiscated_quota"`
	WithdrawnQuota   int   `json:"withdrawn_quota"`
	TransferredQuota int   `json:"transferred_quota"`
	TotalQuota       int   `json:"total_quota"`
	CreatedAt        int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt        int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

func (balance *AffiliateBalance) normalizeTotalQuotaFloor() bool {
	if balance == nil {
		return false
	}
	minTotal := balance.PendingQuota + balance.AvailableQuota + balance.FrozenQuota + balance.RiskFrozenQuota + balance.WithdrawnQuota + balance.TransferredQuota
	if balance.TotalQuota >= minTotal {
		return false
	}
	balance.TotalQuota = minTotal
	return true
}

type AffiliatePayoutAccount struct {
	Id            int    `json:"id"`
	UserId        int    `json:"user_id" gorm:"uniqueIndex"`
	UsdtAddress   string `json:"usdt_address" gorm:"type:varchar(255)"`
	UsdtChain     string `json:"usdt_chain" gorm:"type:varchar(32)"`
	AlipayAccount string `json:"alipay_account" gorm:"type:varchar(255)"`
	AlipayName    string `json:"alipay_name" gorm:"type:varchar(255)"`
	AlipayQrPath  string `json:"alipay_qr_path" gorm:"type:varchar(255)"`
	WechatAccount string `json:"wechat_account" gorm:"type:varchar(255)"`
	WechatName    string `json:"wechat_name" gorm:"type:varchar(255)"`
	WechatQrPath  string `json:"wechat_qr_path" gorm:"type:varchar(255)"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type AffiliateWithdrawal struct {
	Id              int     `json:"id"`
	UserId          int     `json:"user_id" gorm:"index"`
	Quota           int     `json:"quota"`
	DisplayAmount   float64 `json:"display_amount"`
	DisplayCurrency string  `json:"display_currency" gorm:"type:varchar(32)"`
	Method          string  `json:"method" gorm:"type:varchar(32);index"`
	PayoutSnapshot  string  `json:"payout_snapshot" gorm:"type:text"`
	Status          string  `json:"status" gorm:"type:varchar(32);index"`
	AdminId         int     `json:"admin_id"`
	AdminRemark     string  `json:"admin_remark" gorm:"type:varchar(500)"`
	ApprovedTime    int64   `json:"approved_time"`
	PaidTime        int64   `json:"paid_time"`
	RejectedTime    int64   `json:"rejected_time"`
	CreatedAt       int64   `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt       int64   `json:"updated_at" gorm:"autoUpdateTime"`
}

type AffiliateLeaderboardItem struct {
	Rank            int    `json:"rank"`
	UserId          int    `json:"user_id"`
	Username        string `json:"username"`
	DisplayName     string `json:"display_name"`
	MaskedName      string `json:"masked_name"`
	InviteCount     int    `json:"invite_count"`
	CommissionQuota int    `json:"commission_quota"`
}

type AffiliateSourceDetail struct {
	SourceType      string  `json:"source_type"`
	Title           string  `json:"title"`
	PlanId          int     `json:"plan_id,omitempty"`
	PlanTitle       string  `json:"plan_title,omitempty"`
	RedemptionId    int     `json:"redemption_id,omitempty"`
	RedemptionName  string  `json:"redemption_name,omitempty"`
	OriginalAmount  float64 `json:"original_amount,omitempty"`
	DiscountAmount  float64 `json:"discount_amount,omitempty"`
	PaidAmount      float64 `json:"paid_amount,omitempty"`
	PromoCode       string  `json:"promo_code,omitempty"`
	PaymentProvider string  `json:"payment_provider,omitempty"`
	PaymentMethod   string  `json:"payment_method,omitempty"`
	Quota           int     `json:"quota,omitempty"`
}

type AffiliateRecordWithDetail struct {
	AffiliateRecord
	Invitee AffiliateUserInfo      `json:"invitee"`
	Detail  *AffiliateSourceDetail `json:"detail,omitempty"`
}

type AffiliateUserInfo struct {
	Id          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	MaskedName  string `json:"masked_name"`
	Status      int    `json:"status"`
	CreatedAt   int64  `json:"created_at"`
}

type AffiliateInvitationItem struct {
	Invitee         AffiliateUserInfo `json:"invitee"`
	TopUpCount      int               `json:"topup_count"`
	TopUpQuota      int               `json:"topup_quota"`
	LastTopUpTime   int64             `json:"last_topup_time"`
	CommissionQuota int               `json:"commission_quota"`
}

type AffiliateAdminUserInfo struct {
	Id          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	AffCode     string `json:"aff_code"`
	Status      int    `json:"status"`
	InviterId   int    `json:"inviter_id"`
	CreatedAt   int64  `json:"created_at"`
}

type AffiliateAdminInvitationItem struct {
	InviterId        int     `json:"inviter_id"`
	InviterUsername  string  `json:"inviter_username"`
	InviterName      string  `json:"inviter_name"`
	InviterEmail     string  `json:"inviter_email"`
	InviterAffCode   string  `json:"inviter_aff_code"`
	InviteeId        int     `json:"invitee_id"`
	InviteeUsername  string  `json:"invitee_username"`
	InviteeName      string  `json:"invitee_name"`
	InviteeEmail     string  `json:"invitee_email"`
	InviteeStatus    int     `json:"invitee_status"`
	InviteeCreatedAt int64   `json:"invitee_created_at"`
	TopUpCount       int     `json:"topup_count"`
	TopUpQuota       int     `json:"topup_quota"`
	RechargeAmount   float64 `json:"recharge_amount"`
	LastTopUpTime    int64   `json:"last_topup_time"`
	CommissionQuota  int     `json:"commission_quota"`
}

type AffiliateAdminInvitationSummary struct {
	MatchedInviterCount int              `json:"matched_inviter_count"`
	MatchedInviteeCount int              `json:"matched_invitee_count"`
	TopUpCount          int              `json:"topup_count"`
	TopUpQuota          int              `json:"topup_quota"`
	RechargeAmount      float64          `json:"recharge_amount"`
	Balance             AffiliateBalance `json:"balance"`
}

type AffiliateAdminRecordWithDetail struct {
	AffiliateRecord
	Inviter AffiliateAdminUserInfo `json:"inviter"`
	Invitee AffiliateAdminUserInfo `json:"invitee"`
	Detail  *AffiliateSourceDetail `json:"detail,omitempty"`
}

type AffiliateInviterBindResult struct {
	UserId            int    `json:"user_id"`
	Username          string `json:"username"`
	DisplayName       string `json:"display_name"`
	InviterId         int    `json:"inviter_id"`
	InviterUsername   string `json:"inviter_username"`
	InviterAffCode    string `json:"inviter_aff_code"`
	PreviousInviterId int    `json:"previous_inviter_id"`
	Updated           bool   `json:"updated"`
}

type AffiliateInviterUnbindResult struct {
	UserId            int    `json:"user_id"`
	Username          string `json:"username"`
	DisplayName       string `json:"display_name"`
	PreviousInviterId int    `json:"previous_inviter_id"`
	Updated           bool   `json:"updated"`
}

func isAffiliateSourceEnabled(sourceType string) bool {
	affiliateSetting := setting.GetAffiliateSetting()
	switch sourceType {
	case AffiliateSourceTopUp:
		return affiliateSetting.TriggerTopupEnabled
	case AffiliateSourceSubscription:
		return affiliateSetting.TriggerSubscriptionEnabled
	case AffiliateSourceRedemption:
		return affiliateSetting.TriggerTopupEnabled && !affiliateSetting.FilterRedemptionTopupEnabled
	default:
		return false
	}
}

func normalizeAffiliatePayoutMethod(method string) string {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case AffiliatePayoutMethodUSDT:
		return AffiliatePayoutMethodUSDT
	case AffiliatePayoutMethodAlipay:
		return AffiliatePayoutMethodAlipay
	case AffiliatePayoutMethodWechat:
		return AffiliatePayoutMethodWechat
	default:
		return ""
	}
}

func NormalizeAffiliatePayoutMethods(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = setting.DefaultAffiliatePayoutMethods
	}
	seen := make(map[string]bool, 3)
	methods := make([]string, 0, 3)
	for _, item := range strings.Split(raw, ",") {
		method := normalizeAffiliatePayoutMethod(item)
		if method == "" || seen[method] {
			continue
		}
		seen[method] = true
		methods = append(methods, method)
	}
	return methods
}

func isAffiliatePayoutMethodEnabled(method string) bool {
	method = normalizeAffiliatePayoutMethod(method)
	if method == "" {
		return false
	}
	for _, enabledMethod := range NormalizeAffiliatePayoutMethods(setting.GetAffiliateSetting().PayoutMethods) {
		if enabledMethod == method {
			return true
		}
	}
	return false
}

func getAffiliateBalanceForUpdateTx(tx *gorm.DB, userId int) (*AffiliateBalance, error) {
	if tx == nil {
		return nil, errors.New("tx is nil")
	}
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}

	// Every affiliate ledger mutation locks rows in the same order: user,
	// balance, withdrawal, then reward record. The user lock also protects the
	// legacy aff_quota/aff_history mirror updated when the balance is saved.
	var legacy User
	if err := lockForUpdate(tx).
		Select("id", "aff_quota", "aff_history").
		Where("id = ?", userId).
		First(&legacy).Error; err != nil {
		return nil, err
	}

	balance := &AffiliateBalance{}
	err := lockForUpdate(tx).Where("user_id = ?", userId).First(balance).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		available := legacy.AffQuota
		if available < 0 {
			available = 0
		}
		total := legacy.AffHistoryQuota
		if total < available {
			total = available
		}
		seed := &AffiliateBalance{
			UserId:           userId,
			AvailableQuota:   available,
			TransferredQuota: total - available,
			TotalQuota:       total,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}},
			DoNothing: true,
		}).Create(seed).Error; err != nil {
			return nil, err
		}
		balance = &AffiliateBalance{}
		err = lockForUpdate(tx).Where("user_id = ?", userId).First(balance).Error
	}
	if err != nil {
		return nil, err
	}
	if balance.normalizeTotalQuotaFloor() {
		if err := saveAffiliateBalanceTx(tx, balance); err != nil {
			return nil, err
		}
	}
	return balance, nil
}

func syncLegacyAffiliateBalanceTx(tx *gorm.DB, balance *AffiliateBalance) error {
	if tx == nil || balance == nil || balance.UserId <= 0 {
		return errors.New("invalid affiliate balance")
	}
	return tx.Model(&User{}).Where("id = ?", balance.UserId).Updates(map[string]interface{}{
		"aff_quota":   balance.AvailableQuota,
		"aff_history": balance.TotalQuota,
	}).Error
}

func saveAffiliateBalanceTx(tx *gorm.DB, balance *AffiliateBalance) error {
	if err := tx.Save(balance).Error; err != nil {
		return err
	}
	return syncLegacyAffiliateBalanceTx(tx, balance)
}

func GetAffiliateBalance(userId int) (*AffiliateBalance, error) {
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	var balance *AffiliateBalance
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		balance, err = getAffiliateBalanceForUpdateTx(tx, userId)
		if err != nil {
			return err
		}
		return syncLegacyAffiliateBalanceTx(tx, balance)
	})
	return balance, err
}

func CreateAffiliateRewardsForPayment(inviteeId int, sourceType string, sourceId string, sourceQuota int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return CreateAffiliateRewardsForPaymentTx(tx, inviteeId, sourceType, sourceId, sourceQuota)
	})
}

// CreateAffiliateRewardsForPaymentTx adds affiliate rewards to an existing
// payment settlement transaction. Callers must pass only the purchased
// business entitlement; invoice fees and other pass-through charges are not
// commissionable.
func CreateAffiliateRewardsForPaymentTx(tx *gorm.DB, inviteeId int, sourceType string, sourceId string, sourceQuota int) error {
	if tx == nil {
		return errors.New("tx is nil")
	}
	sourceId = strings.TrimSpace(sourceId)
	if inviteeId <= 0 || sourceId == "" || sourceQuota <= 0 {
		return nil
	}
	if !isAffiliateSourceEnabled(sourceType) {
		return nil
	}

	var invitee User
	if err := lockForUpdate(tx).Select("id", "inviter_id", "created_at").Where("id = ?", inviteeId).First(&invitee).Error; err != nil {
		return err
	}
	if invitee.InviterId <= 0 {
		return nil
	}
	var parent User
	if err := lockForUpdate(tx).Select("id", "inviter_id").Where("id = ?", invitee.InviterId).First(&parent).Error; err != nil {
		return err
	}

	affiliateSetting := setting.GetAffiliateSetting()

	firstLevelAllowed, err := queryAffiliateUserCanInviteForUpdateWithDB(tx, parent.Id, affiliateSetting)
	if err != nil {
		return err
	}
	if !firstLevelAllowed {
		return nil
	}

	inviteeEligible, err := checkInviteeEligibility(tx, inviteeId, invitee.CreatedAt, affiliateSetting)
	if err != nil {
		return err
	}
	if !inviteeEligible {
		return nil
	}

	if affiliateSetting.FirstLevelEnabled && affiliateSetting.FirstLevelRatio > 0 {
		if err := createAffiliateRewardRecordTx(tx, parent.Id, inviteeId, 1, sourceType, sourceId, sourceQuota, affiliateSetting.FirstLevelRatio); err != nil {
			return err
		}
	}

	if !affiliateSetting.SecondLevelEnabled || affiliateSetting.SecondLevelRatio <= 0 {
		return nil
	}

	if parent.InviterId <= 0 {
		return nil
	}
	secondLevelAllowed, err := queryAffiliateUserCanInviteForUpdateWithDB(tx, parent.InviterId, affiliateSetting)
	if err != nil {
		return err
	}
	if !secondLevelAllowed {
		return nil
	}
	return createAffiliateRewardRecordTx(tx, parent.InviterId, inviteeId, 2, sourceType, sourceId, sourceQuota, affiliateSetting.SecondLevelRatio)
}

func createAffiliateRewardRecordTx(tx *gorm.DB, userId int, inviteeId int, level int, sourceType string, sourceId string, sourceQuota int, ratio int) error {
	if userId <= 0 || rewardRatioInvalid(ratio) {
		return nil
	}
	rewardQuota := sourceQuota * ratio / 100
	if rewardQuota <= 0 {
		return nil
	}

	var existing AffiliateRecord
	err := tx.Where("source_type = ? AND source_id = ? AND user_id = ? AND level = ?", sourceType, sourceId, userId, level).
		First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	balance, err := getAffiliateBalanceForUpdateTx(tx, userId)
	if err != nil {
		return err
	}
	now := common.GetTimestamp()
	record := &AffiliateRecord{
		UserId:        userId,
		InviteeId:     inviteeId,
		Level:         level,
		SourceType:    sourceType,
		SourceId:      sourceId,
		SourceQuota:   sourceQuota,
		RewardQuota:   rewardQuota,
		Ratio:         ratio,
		Status:        AffiliateRecordStatusPending,
		AvailableTime: now + setting.GetAffiliateSetting().SettlementDelaySeconds,
	}
	createResult := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "source_type"},
			{Name: "source_id"},
			{Name: "level"},
		},
		DoNothing: true,
	}).Create(record)
	if createResult.Error != nil {
		return createResult.Error
	}
	if createResult.RowsAffected == 0 {
		return nil
	}

	balance.PendingQuota += rewardQuota
	balance.TotalQuota += rewardQuota
	record.BalanceAfterQuota = affiliateBalanceSnapshotQuota(balance)
	if err := saveAffiliateBalanceTx(tx, balance); err != nil {
		return err
	}
	return tx.Model(record).Update("balance_after_quota", record.BalanceAfterQuota).Error
}

func rewardRatioInvalid(ratio int) bool {
	return ratio <= 0 || ratio > 100
}

func checkInviteeEligibility(tx *gorm.DB, inviteeId int, inviteeCreatedAt int64, s *setting.AffiliateSetting) (bool, error) {
	if s.InviteeMinAccountAgeDays <= 0 && s.InviteeMinRechargeAmount <= 0 {
		return true, nil
	}

	if s.InviteeMinAccountAgeDays > 0 {
		requiredAge := int64(s.InviteeMinAccountAgeDays) * 86400
		if common.GetTimestamp()-inviteeCreatedAt < requiredAge {
			return false, nil
		}
	}

	if s.InviteeMinRechargeAmount > 0 {
		totalRecharge, err := getUserTotalRechargeAmountWithDB(tx, inviteeId)
		if err != nil {
			return false, err
		}
		if totalRecharge < float64(s.InviteeMinRechargeAmount) {
			return false, nil
		}
	}

	return true, nil
}

func affiliateTopUpPaidAmountCNY(topUp *TopUp) decimal.Decimal {
	if topUp == nil {
		return decimal.Zero
	}
	if topUp.PaidAmountCNY > 0 {
		return decimal.NewFromFloat(topUp.PaidAmountCNY).Round(2)
	}
	paidAmount := invoiceOrderPaidAmount(topUp.Money, topUp.ActualMoney, topUp.PromoCodeId)
	if paidAmount <= 0 {
		return decimal.Zero
	}
	provider := invoiceOrderPaymentProvider(topUp.PaymentProvider, topUp.PaymentMethod)
	return decimal.NewFromFloat(invoiceOrderAmountCNY(paidAmount, provider)).Round(2)
}

func getUserTotalRechargeAmountWithDB(db *gorm.DB, userId int) (float64, error) {
	if db == nil || userId <= 0 {
		return 0, nil
	}
	var topUps []TopUp
	if err := db.Select("money", "actual_money", "paid_amount_cny", "promo_code_id", "payment_provider", "payment_method").
		Where("user_id = ? AND status = ?", userId, common.TopUpStatusSuccess).
		Find(&topUps).Error; err != nil {
		return 0, err
	}
	total := decimal.Zero
	for i := range topUps {
		total = total.Add(affiliateTopUpPaidAmountCNY(&topUps[i]))
	}
	return total.Round(2).InexactFloat64(), nil
}

// GetUserTotalRechargeAmount returns successful business payments normalized
// to CNY. Immutable PaidAmountCNY snapshots win; legacy rows use provider-aware
// conversion of ActualMoney/Money and never include invoice fees.
func GetUserTotalRechargeAmount(userId int) (float64, error) {
	return getUserTotalRechargeAmountWithDB(DB, userId)
}

func SettleMatureAffiliateRecords(userId int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return settleMatureAffiliateRecordsTx(tx, userId)
	})
}

func settleMatureAffiliateRecordsTx(tx *gorm.DB, userId int) error {
	now := common.GetTimestamp()
	matureUsers := tx.Model(&AffiliateRecord{}).
		Distinct("user_id").
		Where("status = ? AND available_time <= ?", AffiliateRecordStatusPending, now)
	if userId > 0 {
		matureUsers = matureUsers.Where("user_id = ?", userId)
	}
	var userIds []int
	if err := matureUsers.Order("user_id ASC").Pluck("user_id", &userIds).Error; err != nil {
		return err
	}
	if len(userIds) == 0 {
		return nil
	}
	sort.Ints(userIds)

	balances := make(map[int]*AffiliateBalance, len(userIds))
	for _, uid := range userIds {
		balance, err := getAffiliateBalanceForUpdateTx(tx, uid)
		if err != nil {
			return err
		}
		balances[uid] = balance
	}

	var records []AffiliateRecord
	if err := lockForUpdate(tx).
		Where("status = ? AND available_time <= ? AND user_id IN ?", AffiliateRecordStatusPending, now, userIds).
		Order("user_id ASC, id ASC").
		Find(&records).Error; err != nil {
		return err
	}
	quotaByUser := make(map[int]int, len(userIds))
	for _, record := range records {
		result := tx.Model(&AffiliateRecord{}).
			Where("id = ? AND status = ?", record.Id, AffiliateRecordStatusPending).
			Updates(map[string]interface{}{
				"status":       AffiliateRecordStatusAvailable,
				"settled_time": now,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			quotaByUser[record.UserId] += record.RewardQuota
		}
	}

	for _, uid := range userIds {
		quota := quotaByUser[uid]
		if quota == 0 {
			continue
		}
		balance := balances[uid]
		if balance.PendingQuota < quota {
			return errors.New("affiliate pending balance invariant violated")
		}
		balance.PendingQuota -= quota
		frozen, err := queryAffiliateUserAssetsFrozenTx(tx, uid)
		if err != nil {
			return err
		}
		if frozen {
			balance.RiskFrozenQuota += quota
		} else {
			balance.AvailableQuota += quota
		}
		if err := saveAffiliateBalanceTx(tx, balance); err != nil {
			return err
		}
	}
	return nil
}

func GetAffiliateRecords(userId int, status string, pageInfo *common.PageInfo) ([]*AffiliateRecord, int64, error) {
	if err := SettleMatureAffiliateRecords(userId); err != nil {
		return nil, 0, err
	}
	query := DB.Model(&AffiliateRecord{}).Where("user_id = ?", userId)
	if status = strings.TrimSpace(status); status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var records []*AffiliateRecord
	err := query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&records).Error
	return records, total, err
}

func GetAffiliateRecordsWithDetails(userId int, status string, pageInfo *common.PageInfo) ([]*AffiliateRecordWithDetail, int64, error) {
	records, total, err := GetAffiliateRecords(userId, status, pageInfo)
	if err != nil {
		return nil, 0, err
	}
	items := make([]*AffiliateRecordWithDetail, 0, len(records))
	inviteeIds := make([]int, 0, len(records))
	for _, record := range records {
		if record == nil {
			continue
		}
		items = append(items, &AffiliateRecordWithDetail{AffiliateRecord: *record})
		inviteeIds = append(inviteeIds, record.InviteeId)
	}
	if len(items) == 0 {
		return items, total, nil
	}
	if err := attachAffiliateSourceDetails(items); err != nil {
		return nil, 0, err
	}
	usersById, err := getAffiliateUsersByIds(uniqueInts(inviteeIds))
	if err != nil {
		return nil, 0, err
	}
	for _, item := range items {
		if item == nil {
			continue
		}
		item.Invitee = usersById[item.InviteeId]
	}
	return items, total, nil
}

func GetAffiliateInvitations(userId int, pageInfo *common.PageInfo) ([]*AffiliateInvitationItem, int64, error) {
	query := DB.Model(&User{}).Where("inviter_id = ?", userId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var invitees []User
	if err := query.
		Select("id", "username", "display_name", "status", "created_at").
		Order("id desc").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&invitees).Error; err != nil {
		return nil, 0, err
	}
	if len(invitees) == 0 {
		return []*AffiliateInvitationItem{}, total, nil
	}

	inviteeIds := make([]int, 0, len(invitees))
	for _, invitee := range invitees {
		inviteeIds = append(inviteeIds, invitee.Id)
	}

	topupByInvitee, err := getAffiliateTopUpAggByInviteeIds(inviteeIds)
	if err != nil {
		return nil, 0, err
	}
	commissionByInvitee, err := getAffiliateCommissionQuotaByInviteeIds(inviteeIds, userId)
	if err != nil {
		return nil, 0, err
	}

	items := make([]*AffiliateInvitationItem, 0, len(invitees))
	for _, invitee := range invitees {
		topup := topupByInvitee[invitee.Id]
		items = append(items, &AffiliateInvitationItem{
			Invitee:         publicAffiliateUserInfo(invitee),
			TopUpCount:      topup.TopUpCount,
			TopUpQuota:      topup.TopUpQuota,
			LastTopUpTime:   topup.LastTopUpTime,
			CommissionQuota: commissionByInvitee[invitee.Id],
		})
	}
	return items, total, nil
}

func buildAdminAffiliateInvitationQuery(keyword string) (*gorm.DB, error) {
	query := DB.Model(&User{}).Where("inviter_id > 0")
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return query, nil
	}
	userIds, err := findAffiliateAdminMatchedUserIds(keyword)
	if err != nil {
		return nil, err
	}
	if len(userIds) == 0 {
		return DB.Model(&User{}).Where("1 = 0"), nil
	}
	return query.Where("id IN ? OR inviter_id IN ?", userIds, userIds), nil
}

func GetAdminAffiliateInvitations(keyword string, pageInfo *common.PageInfo) ([]*AffiliateAdminInvitationItem, int64, error) {
	query, err := buildAdminAffiliateInvitationQuery(keyword)
	if err != nil {
		return nil, 0, err
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var invitees []User
	if err := query.
		Select("id", "username", "display_name", "email", "aff_code", "status", "inviter_id", "created_at").
		Order("id desc").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&invitees).Error; err != nil {
		return nil, 0, err
	}
	if len(invitees) == 0 {
		return []*AffiliateAdminInvitationItem{}, total, nil
	}

	inviteeIds := make([]int, 0, len(invitees))
	inviterIds := make([]int, 0, len(invitees))
	for _, invitee := range invitees {
		inviteeIds = append(inviteeIds, invitee.Id)
		inviterIds = append(inviterIds, invitee.InviterId)
	}

	usersById, err := getAffiliateAdminUsersByIds(uniqueInts(append(inviteeIds, inviterIds...)))
	if err != nil {
		return nil, 0, err
	}

	topupByInvitee, err := getAffiliateTopUpAggByInviteeIds(inviteeIds)
	if err != nil {
		return nil, 0, err
	}
	commissionByInvitee, err := getAffiliateCommissionQuotaByInviteeIds(inviteeIds, 0)
	if err != nil {
		return nil, 0, err
	}

	items := make([]*AffiliateAdminInvitationItem, 0, len(invitees))
	for _, invitee := range invitees {
		inviter := usersById[invitee.InviterId]
		topup := topupByInvitee[invitee.Id]
		items = append(items, &AffiliateAdminInvitationItem{
			InviterId:        inviter.Id,
			InviterUsername:  inviter.Username,
			InviterName:      inviter.DisplayName,
			InviterEmail:     inviter.Email,
			InviterAffCode:   inviter.AffCode,
			InviteeId:        invitee.Id,
			InviteeUsername:  invitee.Username,
			InviteeName:      invitee.DisplayName,
			InviteeEmail:     invitee.Email,
			InviteeStatus:    invitee.Status,
			InviteeCreatedAt: invitee.CreatedAt,
			TopUpCount:       topup.TopUpCount,
			TopUpQuota:       topup.TopUpQuota,
			LastTopUpTime:    topup.LastTopUpTime,
			CommissionQuota:  commissionByInvitee[invitee.Id],
			RechargeAmount:   topup.RechargeAmount,
		})
	}
	return items, total, nil
}

type affiliateAdminTopUpAgg struct {
	TopUpCount     int
	TopUpQuota     int
	RechargeAmount float64
	LastTopUpTime  int64
}

func getAffiliateTopUpAggByInviteeIds(inviteeIds []int) (map[int]affiliateAdminTopUpAgg, error) {
	topupByInvitee := make(map[int]affiliateAdminTopUpAgg)
	inviteeIds = uniqueInts(inviteeIds)
	if len(inviteeIds) == 0 {
		return topupByInvitee, nil
	}
	var topups []TopUp
	if err := DB.Select("user_id", "amount", "money", "actual_money", "promo_code_id", "affiliate_source_quota", "trade_no", "complete_time").
		Where("user_id IN ? AND status = ?", inviteeIds, common.TopUpStatusSuccess).
		Find(&topups).Error; err != nil {
		return nil, err
	}
	legacyTradeNos := make([]string, 0)
	for _, topup := range topups {
		if topup.AffiliateSourceQuota <= 0 {
			legacyTradeNos = append(legacyTradeNos, topup.TradeNo)
		}
	}
	recordSourceQuotaByTradeNo, err := getAffiliateRecordSourceQuotaByTopUpTradeNos(legacyTradeNos)
	if err != nil {
		return nil, err
	}
	for _, topup := range topups {
		row := topupByInvitee[topup.UserId]
		row.TopUpCount++
		row.TopUpQuota += affiliateAdminTopUpQuota(&topup, recordSourceQuotaByTradeNo[topup.TradeNo])
		row.RechargeAmount += affiliateTopUpRechargeAmount(&topup)
		if topup.CompleteTime > row.LastTopUpTime {
			row.LastTopUpTime = topup.CompleteTime
		}
		topupByInvitee[topup.UserId] = row
	}
	return topupByInvitee, nil
}

func getAffiliateRecordSourceQuotaByTopUpTradeNos(tradeNos []string) (map[string]int, error) {
	sourceQuotaByTradeNo := make(map[string]int)
	tradeNos = uniqueStrings(tradeNos)
	if len(tradeNos) == 0 {
		return sourceQuotaByTradeNo, nil
	}
	type sourceQuotaRow struct {
		SourceId    string
		SourceQuota int
	}
	var rows []sourceQuotaRow
	if err := DB.Model(&AffiliateRecord{}).
		Select("source_id, MAX(source_quota) AS source_quota").
		Where("source_type = ? AND source_id IN ? AND source_quota > 0", AffiliateSourceTopUp, tradeNos).
		Group("source_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		sourceQuotaByTradeNo[row.SourceId] = row.SourceQuota
	}
	return sourceQuotaByTradeNo, nil
}

func affiliateAdminTopUpQuota(topup *TopUp, recordSourceQuota int) int {
	if topup == nil {
		return 0
	}
	if topup.AffiliateSourceQuota > 0 {
		return topup.AffiliateSourceQuota
	}
	if recordSourceQuota > 0 {
		return recordSourceQuota
	}
	return int(topup.Amount)
}

func affiliateTopUpRechargeAmount(topup *TopUp) float64 {
	if topup == nil {
		return 0
	}
	if topup.PromoCodeId > 0 {
		if topup.ActualMoney > 0 {
			return topup.ActualMoney
		}
		return 0
	}
	if topup.ActualMoney > 0 {
		return topup.ActualMoney
	}
	return topup.Money
}

type affiliateAdminInvitationRow struct {
	Id        int
	InviterId int
}

func GetAdminAffiliateInvitationSummary(keyword string) (*AffiliateAdminInvitationSummary, error) {
	query, err := buildAdminAffiliateInvitationQuery(keyword)
	if err != nil {
		return nil, err
	}
	var rows []affiliateAdminInvitationRow
	if err := query.Select("id", "inviter_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	summary := &AffiliateAdminInvitationSummary{MatchedInviteeCount: len(rows)}
	if len(rows) == 0 {
		return summary, nil
	}
	inviteeIds := make([]int, 0, len(rows))
	inviterIds := make([]int, 0, len(rows))
	for _, row := range rows {
		inviteeIds = append(inviteeIds, row.Id)
		inviterIds = append(inviterIds, row.InviterId)
	}
	uniqueInviterIds := uniqueInts(inviterIds)
	summary.MatchedInviterCount = len(uniqueInviterIds)
	topupByInvitee, err := getAffiliateTopUpAggByInviteeIds(inviteeIds)
	if err != nil {
		return nil, err
	}
	for _, inviteeId := range uniqueInts(inviteeIds) {
		topup := topupByInvitee[inviteeId]
		summary.TopUpCount += topup.TopUpCount
		summary.TopUpQuota += topup.TopUpQuota
		summary.RechargeAmount += topup.RechargeAmount
	}
	balancesByUser, err := getAffiliateBalancesByUserIds(uniqueInviterIds)
	if err != nil {
		return nil, err
	}
	for _, inviterId := range uniqueInviterIds {
		balance := balancesByUser[inviterId]
		summary.Balance.PendingQuota += balance.PendingQuota
		summary.Balance.AvailableQuota += balance.AvailableQuota
		summary.Balance.FrozenQuota += balance.FrozenQuota
		summary.Balance.RiskFrozenQuota += balance.RiskFrozenQuota
		summary.Balance.ConfiscatedQuota += balance.ConfiscatedQuota
		summary.Balance.WithdrawnQuota += balance.WithdrawnQuota
		summary.Balance.TransferredQuota += balance.TransferredQuota
		summary.Balance.TotalQuota += balance.TotalQuota
	}
	return summary, nil
}

func getAffiliateCommissionQuotaByInviteeIds(inviteeIds []int, userId int) (map[int]int, error) {
	commissionByInvitee := make(map[int]int)
	inviteeIds = uniqueInts(inviteeIds)
	if len(inviteeIds) == 0 {
		return commissionByInvitee, nil
	}
	type commissionAggRow struct {
		InviteeId       int
		CommissionQuota int
	}
	var commissionRows []commissionAggRow
	query := DB.Model(&AffiliateRecord{}).
		Select("invitee_id, COALESCE(SUM(reward_quota), 0) AS commission_quota").
		Where("invitee_id IN ?", inviteeIds)
	if userId > 0 {
		query = query.Where("user_id = ?", userId)
	}
	if err := query.Group("invitee_id").Scan(&commissionRows).Error; err != nil {
		return nil, err
	}
	for _, row := range commissionRows {
		commissionByInvitee[row.InviteeId] = row.CommissionQuota
	}
	return commissionByInvitee, nil
}

func GetAdminAffiliateRecordsWithDetails(sourceType string, status string, keyword string, pageInfo *common.PageInfo) ([]*AffiliateAdminRecordWithDetail, int64, error) {
	if err := SettleMatureAffiliateRecords(0); err != nil {
		return nil, 0, err
	}

	query := DB.Model(&AffiliateRecord{})
	if sourceType = strings.TrimSpace(sourceType); sourceType != "" {
		query = query.Where("source_type = ?", sourceType)
	}
	if status = strings.TrimSpace(status); status != "" {
		query = query.Where("status = ?", status)
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		userIds, err := findAffiliateAdminMatchedUserIds(keyword)
		if err != nil {
			return nil, 0, err
		}
		if len(userIds) == 0 {
			return []*AffiliateAdminRecordWithDetail{}, 0, nil
		}
		query = query.Where("user_id IN ? OR invitee_id IN ?", userIds, userIds)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var records []*AffiliateRecord
	if err := query.Order("id desc").
		Limit(pageInfo.GetPageSize()).
		Offset(pageInfo.GetStartIdx()).
		Find(&records).Error; err != nil {
		return nil, 0, err
	}
	if len(records) == 0 {
		return []*AffiliateAdminRecordWithDetail{}, total, nil
	}

	userIds := make([]int, 0, len(records)*2)
	detailItems := make([]*AffiliateRecordWithDetail, 0, len(records))
	for _, record := range records {
		if record == nil {
			continue
		}
		userIds = append(userIds, record.UserId, record.InviteeId)
		detailItems = append(detailItems, &AffiliateRecordWithDetail{AffiliateRecord: *record})
	}
	if err := attachAffiliateSourceDetails(detailItems); err != nil {
		return nil, 0, err
	}
	usersById, err := getAffiliateAdminUsersByIds(uniqueInts(userIds))
	if err != nil {
		return nil, 0, err
	}

	items := make([]*AffiliateAdminRecordWithDetail, 0, len(detailItems))
	for _, item := range detailItems {
		if item == nil {
			continue
		}
		items = append(items, &AffiliateAdminRecordWithDetail{
			AffiliateRecord: item.AffiliateRecord,
			Inviter:         usersById[item.UserId],
			Invitee:         usersById[item.InviteeId],
			Detail:          item.Detail,
		})
	}
	return items, total, nil
}

func findAffiliateAdminMatchedUserIds(keyword string) ([]int, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return []int{}, nil
	}
	numericKeyword := strings.TrimPrefix(keyword, "#")
	query := DB.Model(&User{}).Select("id")
	likeCondition := "username LIKE ? OR email LIKE ? OR display_name LIKE ? OR aff_code LIKE ?"
	likeArgs := []interface{}{"%" + keyword + "%", "%" + keyword + "%", "%" + keyword + "%", "%" + keyword + "%"}
	if keywordInt, err := strconv.Atoi(numericKeyword); err == nil && keywordInt > 0 {
		likeCondition = "id = ? OR " + likeCondition
		likeArgs = append([]interface{}{keywordInt}, likeArgs...)
	}
	var ids []int
	if err := query.Where("("+likeCondition+")", likeArgs...).Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	return uniqueInts(ids), nil
}

func getAffiliateAdminUsersByIds(userIds []int) (map[int]AffiliateAdminUserInfo, error) {
	usersById := make(map[int]AffiliateAdminUserInfo, len(userIds))
	userIds = uniqueInts(userIds)
	if len(userIds) == 0 {
		return usersById, nil
	}
	var users []User
	if err := DB.Select("id", "username", "display_name", "email", "aff_code", "status", "inviter_id", "created_at").
		Where("id IN ?", userIds).
		Find(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		usersById[user.Id] = AffiliateAdminUserInfo{
			Id:          user.Id,
			Username:    user.Username,
			DisplayName: user.DisplayName,
			Email:       user.Email,
			AffCode:     user.AffCode,
			Status:      user.Status,
			InviterId:   user.InviterId,
			CreatedAt:   user.CreatedAt,
		}
	}
	return usersById, nil
}

func publicAffiliateUserInfo(user User) AffiliateUserInfo {
	name := strings.TrimSpace(user.DisplayName)
	if name == "" {
		name = user.Username
	}
	return AffiliateUserInfo{
		Id:         user.Id,
		MaskedName: maskAffiliatePublicName(name, user.Id),
		Status:     user.Status,
		CreatedAt:  user.CreatedAt,
	}
}

func getAffiliateUsersByIds(userIds []int) (map[int]AffiliateUserInfo, error) {
	usersById := make(map[int]AffiliateUserInfo, len(userIds))
	userIds = uniqueInts(userIds)
	if len(userIds) == 0 {
		return usersById, nil
	}
	var users []User
	if err := DB.Select("id", "username", "display_name", "status", "created_at").
		Where("id IN ?", userIds).
		Find(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		usersById[user.Id] = publicAffiliateUserInfo(user)
	}
	return usersById, nil
}

func attachAffiliateSourceDetails(records []*AffiliateRecordWithDetail) error {
	topupSourceIds := make([]string, 0)
	subscriptionSourceIds := make([]string, 0)
	redemptionIds := make([]int, 0)
	redemptionSourceIdsById := make(map[int][]string)

	for _, record := range records {
		if record == nil {
			continue
		}
		switch record.SourceType {
		case AffiliateSourceTopUp:
			topupSourceIds = append(topupSourceIds, record.SourceId)
		case AffiliateSourceSubscription:
			subscriptionSourceIds = append(subscriptionSourceIds, record.SourceId)
		case AffiliateSourceRedemption:
			redemptionId, _, ok := parseAffiliateRedemptionSourceId(record.SourceId)
			if ok {
				redemptionIds = append(redemptionIds, redemptionId)
				redemptionSourceIdsById[redemptionId] = append(redemptionSourceIdsById[redemptionId], record.SourceId)
			}
		}
	}

	details := make(map[string]*AffiliateSourceDetail)
	if len(topupSourceIds) > 0 {
		var topups []TopUp
		if err := DB.Where("trade_no IN ?", uniqueStrings(topupSourceIds)).Find(&topups).Error; err != nil {
			return err
		}
		for _, topup := range topups {
			original, discount, paid := affiliateMoneySnapshot(topup.OriginalMoney, topup.DiscountMoney, topup.ActualMoney, topup.Money)
			details[affiliateSourceDetailKey(AffiliateSourceTopUp, topup.TradeNo)] = &AffiliateSourceDetail{
				SourceType:      AffiliateSourceTopUp,
				Title:           "余额充值",
				OriginalAmount:  original,
				DiscountAmount:  discount,
				PaidAmount:      paid,
				PromoCode:       topup.PromoCode,
				PaymentProvider: topup.PaymentProvider,
				PaymentMethod:   topup.PaymentMethod,
				Quota:           int(topup.Amount),
			}
		}
	}
	if len(subscriptionSourceIds) > 0 {
		var orders []SubscriptionOrder
		if err := DB.Where("trade_no IN ?", uniqueStrings(subscriptionSourceIds)).Find(&orders).Error; err != nil {
			return err
		}
		planIds := make([]int, 0, len(orders))
		for _, order := range orders {
			if order.PlanId > 0 {
				planIds = append(planIds, order.PlanId)
			}
		}
		plans := make(map[int]SubscriptionPlan)
		if len(planIds) > 0 {
			var planRows []SubscriptionPlan
			if err := DB.Select("id", "title").Where("id IN ?", uniqueInts(planIds)).Find(&planRows).Error; err != nil {
				return err
			}
			for _, plan := range planRows {
				plans[plan.Id] = plan
			}
		}
		for _, order := range orders {
			planTitle := ""
			if plan, ok := plans[order.PlanId]; ok {
				planTitle = plan.Title
			}
			title := "订阅"
			if planTitle != "" {
				title = "订阅：" + planTitle
			}
			original, discount, paid := affiliateMoneySnapshot(order.OriginalMoney, order.DiscountMoney, order.ActualMoney, order.Money)
			details[affiliateSourceDetailKey(AffiliateSourceSubscription, order.TradeNo)] = &AffiliateSourceDetail{
				SourceType:      AffiliateSourceSubscription,
				Title:           title,
				PlanId:          order.PlanId,
				PlanTitle:       planTitle,
				OriginalAmount:  original,
				DiscountAmount:  discount,
				PaidAmount:      paid,
				PromoCode:       order.PromoCode,
				PaymentProvider: order.PaymentProvider,
				PaymentMethod:   order.PaymentMethod,
			}
		}
	}
	if len(redemptionIds) > 0 {
		var redemptions []Redemption
		if err := DB.Select("id", "name", "quota").Where("id IN ?", uniqueInts(redemptionIds)).Find(&redemptions).Error; err != nil {
			return err
		}
		for _, redemption := range redemptions {
			title := "兑换码兑换"
			if redemption.Name != "" {
				title += "：" + redemption.Name
			}
			for _, sourceId := range redemptionSourceIdsById[redemption.Id] {
				details[affiliateSourceDetailKey(AffiliateSourceRedemption, sourceId)] = &AffiliateSourceDetail{
					SourceType:     AffiliateSourceRedemption,
					Title:          title,
					RedemptionId:   redemption.Id,
					RedemptionName: redemption.Name,
					Quota:          redemption.Quota,
				}
			}
		}
	}

	for _, record := range records {
		if record == nil {
			continue
		}
		detail := details[affiliateSourceDetailKey(record.SourceType, record.SourceId)]
		if detail == nil {
			detail = fallbackAffiliateSourceDetail(record)
		}
		if record.SourceQuota > 0 && (record.SourceType == AffiliateSourceTopUp || record.SourceType == AffiliateSourceSubscription) {
			detailCopy := *detail
			detailCopy.Quota = record.SourceQuota
			detail = &detailCopy
		}
		record.Detail = detail
	}
	return nil
}

func affiliateSourceDetailKey(sourceType string, sourceId string) string {
	return sourceType + "\x00" + sourceId
}

func affiliateMoneySnapshot(originalAmount float64, discountAmount float64, paidAmount float64, fallbackPaidAmount float64) (float64, float64, float64) {
	if originalAmount <= 0 {
		if paidAmount > 0 || discountAmount > 0 {
			originalAmount = paidAmount + discountAmount
		} else if fallbackPaidAmount > 0 {
			originalAmount = fallbackPaidAmount
		}
	}
	if paidAmount <= 0 && discountAmount <= 0 && fallbackPaidAmount > 0 {
		paidAmount = fallbackPaidAmount
	}
	if discountAmount <= 0 && originalAmount > 0 && paidAmount > 0 && originalAmount > paidAmount {
		discountAmount = originalAmount - paidAmount
	}
	return originalAmount, discountAmount, paidAmount
}

func fallbackAffiliateSourceDetail(record *AffiliateRecordWithDetail) *AffiliateSourceDetail {
	detail := &AffiliateSourceDetail{SourceType: record.SourceType}
	switch record.SourceType {
	case AffiliateSourceTopUp:
		detail.Title = "余额充值"
	case AffiliateSourceSubscription:
		detail.Title = "订阅"
	case AffiliateSourceRedemption:
		detail.Title = "兑换码兑换"
		detail.Quota = record.SourceQuota
	default:
		detail.Title = record.SourceType
	}
	return detail
}

func parseAffiliateRedemptionSourceId(sourceId string) (int, int, bool) {
	var redemptionId int
	var userId int
	n, err := fmt.Sscanf(sourceId, "redemption-%d-user-%d", &redemptionId, &userId)
	return redemptionId, userId, err == nil && n == 2 && redemptionId > 0 && userId > 0
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
}

func uniqueInts(values []int) []int {
	seen := make(map[int]bool, len(values))
	unique := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
}

func BindUserInviterByAffCode(userId int, userIdentifier string, affCode string, force bool) (*AffiliateInviterBindResult, error) {
	affCode = normalizeAffiliateBindAffCode(affCode)
	if affCode == "" {
		return nil, errors.New("邀请代码不能为空")
	}
	var result *AffiliateInviterBindResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		invitee, err := findAffiliateBindUserTx(tx, userId, userIdentifier)
		if err != nil {
			return err
		}
		inviter, err := findAffiliateInviterByAffCodeTx(tx, affCode)
		if err != nil {
			return err
		}
		if invitee.Id == inviter.Id {
			return errors.New("不能绑定自己为邀请人")
		}
		if invitee.InviterId == inviter.Id {
			result = buildAffiliateBindResult(invitee, inviter, invitee.InviterId, false)
			return nil
		}
		if invitee.InviterId > 0 && !force {
			return errors.New("该用户已有邀请人，如需改绑请开启强制覆盖")
		}
		if err := ensureNoAffiliateInviteCycleTx(tx, invitee.Id, inviter.Id); err != nil {
			return err
		}

		previousInviterId := invitee.InviterId
		if err := tx.Model(&User{}).Where("id = ?", invitee.Id).Update("inviter_id", inviter.Id).Error; err != nil {
			return err
		}
		if previousInviterId > 0 {
			if err := tx.Model(&User{}).
				Where("id = ? AND aff_count > 0", previousInviterId).
				Update("aff_count", gorm.Expr("aff_count - ?", 1)).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&User{}).Where("id = ?", inviter.Id).Update("aff_count", gorm.Expr("aff_count + ?", 1)).Error; err != nil {
			return err
		}
		result = buildAffiliateBindResult(invitee, inviter, previousInviterId, true)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result != nil && result.Updated {
		_ = invalidateUserCache(result.UserId)
		if result.InviterId > 0 {
			_ = invalidateUserCache(result.InviterId)
		}
		if result.PreviousInviterId > 0 {
			_ = invalidateUserCache(result.PreviousInviterId)
		}
	}
	return result, nil
}

func UnbindUserInviter(userId int, userIdentifier string) (*AffiliateInviterUnbindResult, error) {
	var result *AffiliateInviterUnbindResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		invitee, err := findAffiliateBindUserTx(tx, userId, userIdentifier)
		if err != nil {
			return err
		}
		previousInviterId := invitee.InviterId
		result = &AffiliateInviterUnbindResult{
			UserId:            invitee.Id,
			Username:          invitee.Username,
			DisplayName:       invitee.DisplayName,
			PreviousInviterId: previousInviterId,
			Updated:           false,
		}
		if previousInviterId <= 0 {
			return nil
		}
		if err := tx.Model(&User{}).Where("id = ?", invitee.Id).Update("inviter_id", 0).Error; err != nil {
			return err
		}
		if err := tx.Model(&User{}).
			Where("id = ? AND aff_count > 0", previousInviterId).
			Update("aff_count", gorm.Expr("aff_count - ?", 1)).Error; err != nil {
			return err
		}
		result.Updated = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result != nil && result.Updated {
		_ = invalidateUserCache(result.UserId)
		if result.PreviousInviterId > 0 {
			_ = invalidateUserCache(result.PreviousInviterId)
		}
	}
	return result, nil
}

func findAffiliateInviterByAffCodeTx(tx *gorm.DB, affCode string) (*User, error) {
	var users []User
	if err := lockForUpdate(tx).
		Select("id", "username", "display_name", "aff_code", "inviter_id", "aff_count").
		Where("aff_code = ?", affCode).
		Limit(2).
		Find(&users).Error; err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, errors.New("邀请代码对应的用户不存在")
	}
	if len(users) > 1 {
		return nil, errors.New("邀请代码存在冲突，请先检查用户的邀请码配置")
	}
	if isAffiliateUserInviteCodeBlockedWithDB(tx, users[0].Id) {
		return nil, errors.New("该邀请人的邀请码已被风控废除")
	}
	return &users[0], nil
}

func normalizeAffiliateBindAffCode(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "aff=") {
		idx := strings.Index(raw, "aff=")
		raw = raw[idx+len("aff="):]
		for _, separator := range []string{"&", "#", "?", " "} {
			if pos := strings.Index(raw, separator); pos >= 0 {
				raw = raw[:pos]
			}
		}
	}
	return strings.TrimSpace(raw)
}

func findAffiliateBindUserTx(tx *gorm.DB, userId int, userIdentifier string) (*User, error) {
	query := lockForUpdate(tx).
		Select("id", "username", "display_name", "email", "inviter_id")
	if userId > 0 {
		user := &User{}
		if err := query.Where("id = ?", userId).First(user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("被绑定用户不存在")
			}
			return nil, err
		}
		return user, nil
	}
	userIdentifier = strings.TrimSpace(userIdentifier)
	if userIdentifier == "" {
		return nil, errors.New("被绑定用户不能为空")
	}
	if parsedId, err := strconv.Atoi(userIdentifier); err == nil && parsedId > 0 {
		query = query.Where("id = ? OR username = ? OR email = ? OR display_name = ?", parsedId, userIdentifier, userIdentifier, userIdentifier)
	} else {
		query = query.Where("username = ? OR email = ? OR display_name = ?", userIdentifier, userIdentifier, userIdentifier)
	}
	var users []User
	if err := query.Limit(2).Find(&users).Error; err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, errors.New("被绑定用户不存在")
	}
	if len(users) > 1 {
		return nil, errors.New("匹配到多个用户，请先搜索并选择具体用户")
	}
	return &users[0], nil
}

func ensureNoAffiliateInviteCycleTx(tx *gorm.DB, inviteeId int, inviterId int) error {
	visited := make(map[int]bool)
	currentId := inviterId
	for depth := 0; currentId > 0 && depth < 64; depth++ {
		if currentId == inviteeId || visited[currentId] {
			return errors.New("不能形成循环邀请关系")
		}
		visited[currentId] = true
		parent := &User{}
		if err := lockForUpdate(tx).Select("id", "inviter_id").Where("id = ?", currentId).First(parent).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("邀请链用户不存在")
			}
			return err
		}
		currentId = parent.InviterId
	}
	if currentId > 0 {
		return errors.New("邀请链层级过深")
	}
	return nil
}

func buildAffiliateBindResult(invitee *User, inviter *User, previousInviterId int, updated bool) *AffiliateInviterBindResult {
	return &AffiliateInviterBindResult{
		UserId:            invitee.Id,
		Username:          invitee.Username,
		DisplayName:       invitee.DisplayName,
		InviterId:         inviter.Id,
		InviterUsername:   inviter.Username,
		InviterAffCode:    inviter.AffCode,
		PreviousInviterId: previousInviterId,
		Updated:           updated,
	}
}

func GetAffiliateWithdrawals(userId int, pageInfo *common.PageInfo) ([]*AffiliateWithdrawal, int64, error) {
	query := DB.Model(&AffiliateWithdrawal{}).Where("user_id = ?", userId)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var withdrawals []*AffiliateWithdrawal
	err := query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&withdrawals).Error
	return withdrawals, total, err
}

func GetAllAffiliateWithdrawals(status string, pageInfo *common.PageInfo) ([]*AffiliateWithdrawal, int64, error) {
	query := DB.Model(&AffiliateWithdrawal{})
	if status = strings.TrimSpace(status); status != "" {
		query = query.Where("status = ?", status)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var withdrawals []*AffiliateWithdrawal
	err := query.Order("id desc").Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Find(&withdrawals).Error
	return withdrawals, total, err
}

func GetAffiliatePayoutAccount(userId int) (*AffiliatePayoutAccount, error) {
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	account := &AffiliatePayoutAccount{}
	err := DB.Where("user_id = ?", userId).First(account).Error
	if err == nil {
		if account.UsdtChain == "" {
			account.UsdtChain = setting.GetAffiliateSetting().UsdtChain
		}
		return account, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return &AffiliatePayoutAccount{
		UserId:    userId,
		UsdtChain: setting.GetAffiliateSetting().UsdtChain,
	}, nil
}

func SaveAffiliatePayoutAccount(account *AffiliatePayoutAccount) error {
	if account == nil || account.UserId <= 0 {
		return errors.New("invalid payout account")
	}
	account.UsdtAddress = strings.TrimSpace(account.UsdtAddress)
	account.AlipayAccount = strings.TrimSpace(account.AlipayAccount)
	account.AlipayName = strings.TrimSpace(account.AlipayName)
	account.AlipayQrPath = strings.TrimSpace(account.AlipayQrPath)
	account.WechatAccount = strings.TrimSpace(account.WechatAccount)
	account.WechatName = strings.TrimSpace(account.WechatName)
	account.WechatQrPath = strings.TrimSpace(account.WechatQrPath)
	account.UsdtChain = strings.TrimSpace(account.UsdtChain)
	if account.UsdtChain == "" {
		account.UsdtChain = setting.GetAffiliateSetting().UsdtChain
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		existing := &AffiliatePayoutAccount{}
		err := lockForUpdate(tx).Where("user_id = ?", account.UserId).First(existing).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return tx.Create(account).Error
			}
			return err
		}
		account.Id = existing.Id
		account.CreatedAt = existing.CreatedAt
		if account.AlipayQrPath == "" {
			account.AlipayQrPath = existing.AlipayQrPath
		}
		if account.WechatQrPath == "" {
			account.WechatQrPath = existing.WechatQrPath
		}
		return tx.Save(account).Error
	})
}

func SetAffiliatePayoutQrPath(userId int, method string, qrPath string) (*AffiliatePayoutAccount, error) {
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	method = normalizeAffiliatePayoutMethod(method)
	if method != AffiliatePayoutMethodAlipay && method != AffiliatePayoutMethodWechat {
		return nil, errors.New("invalid payout qr method")
	}
	qrPath = strings.TrimSpace(qrPath)

	var saved *AffiliatePayoutAccount
	err := DB.Transaction(func(tx *gorm.DB) error {
		account := &AffiliatePayoutAccount{}
		err := lockForUpdate(tx).Where("user_id = ?", userId).First(account).Error
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			account = &AffiliatePayoutAccount{
				UserId:    userId,
				UsdtChain: setting.GetAffiliateSetting().UsdtChain,
			}
		}
		if account.UsdtChain == "" {
			account.UsdtChain = setting.GetAffiliateSetting().UsdtChain
		}
		if method == AffiliatePayoutMethodAlipay {
			account.AlipayQrPath = qrPath
		} else {
			account.WechatQrPath = qrPath
		}
		if account.Id == 0 {
			if err := tx.Create(account).Error; err != nil {
				return err
			}
		} else if err := tx.Save(account).Error; err != nil {
			return err
		}
		saved = account
		return nil
	})
	return saved, err
}

func CreateAffiliateWithdrawal(userId int, method string, quota int) (*AffiliateWithdrawal, error) {
	if quota <= 0 {
		return nil, errors.New("提现额度必须大于 0")
	}
	if minAmount := setting.GetAffiliateSetting().MinWithdrawalAmount; minAmount > 0 {
		minimumQuota := decimal.NewFromInt(int64(minAmount))
		if operation_setting.GetQuotaDisplayType() != operation_setting.QuotaDisplayTypeTokens {
			rate := operation_setting.GetUsdToCurrencyRate(operation_setting.USDExchangeRate)
			if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) ||
				common.QuotaPerUnit <= 0 || math.IsNaN(common.QuotaPerUnit) || math.IsInf(common.QuotaPerUnit, 0) {
				return nil, errors.New("提现最低金额配置错误")
			}
			minimumQuota = minimumQuota.
				Div(decimal.NewFromFloat(rate)).
				Mul(decimal.NewFromFloat(common.QuotaPerUnit)).
				Floor()
		}
		minQuota, err := common.QuotaFromDecimalStrict(minimumQuota)
		if err != nil || minQuota <= 0 {
			return nil, errors.New("提现最低金额配置超出系统可表示范围")
		}
		if quota < minQuota {
			return nil, fmt.Errorf("提现金额不能小于 %d", minAmount)
		}
	}
	method = normalizeAffiliatePayoutMethod(method)
	if method == "" {
		return nil, errors.New("无效的提现方式")
	}
	if !isAffiliatePayoutMethodEnabled(method) {
		return nil, errors.New("当前提现方式未开放")
	}
	if err := SettleMatureAffiliateRecords(userId); err != nil {
		return nil, err
	}

	var withdrawal *AffiliateWithdrawal
	err := DB.Transaction(func(tx *gorm.DB) error {
		balance, err := getAffiliateBalanceForUpdateTx(tx, userId)
		if err != nil {
			return err
		}
		frozen, err := queryAffiliateUserAssetsFrozenTx(tx, userId)
		if err != nil {
			return err
		}
		if frozen {
			return errors.New("返佣资产已被冻结，暂不能提现")
		}
		if balance.AvailableQuota < quota {
			return errors.New("可提现额度不足")
		}

		snapshot, err := buildAffiliatePayoutSnapshotTx(tx, userId, method)
		if err != nil {
			return err
		}
		withdrawal = &AffiliateWithdrawal{
			UserId:          userId,
			Quota:           quota,
			DisplayAmount:   float64(quota) / common.QuotaPerUnit,
			DisplayCurrency: "USD",
			Method:          method,
			PayoutSnapshot:  snapshot,
			Status:          AffiliateWithdrawalStatusPending,
		}
		if err := tx.Create(withdrawal).Error; err != nil {
			return err
		}

		balance.AvailableQuota -= quota
		balance.FrozenQuota += quota
		return saveAffiliateBalanceTx(tx, balance)
	})
	return withdrawal, err
}

func buildAffiliatePayoutSnapshotTx(tx *gorm.DB, userId int, method string) (string, error) {
	var account AffiliatePayoutAccount
	err := tx.Where("user_id = ?", userId).First(&account).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	if account.UsdtChain == "" {
		account.UsdtChain = setting.GetAffiliateSetting().UsdtChain
	}
	snapshot := map[string]interface{}{
		"method": method,
	}
	switch method {
	case AffiliatePayoutMethodUSDT:
		if strings.TrimSpace(account.UsdtAddress) == "" {
			return "", errors.New("请先填写 USDT 收款地址")
		}
		snapshot["usdt_address"] = account.UsdtAddress
		snapshot["usdt_chain"] = account.UsdtChain
	case AffiliatePayoutMethodAlipay:
		if strings.TrimSpace(account.AlipayAccount) == "" && strings.TrimSpace(account.AlipayQrPath) == "" {
			return "", errors.New("请先填写支付宝账号或上传支付宝收款码")
		}
		snapshot["alipay_account"] = account.AlipayAccount
		snapshot["alipay_name"] = account.AlipayName
		snapshot["alipay_qr_path"] = account.AlipayQrPath
	case AffiliatePayoutMethodWechat:
		if strings.TrimSpace(account.WechatAccount) == "" && strings.TrimSpace(account.WechatQrPath) == "" {
			return "", errors.New("请先填写微信账号或上传微信收款码")
		}
		snapshot["wechat_account"] = account.WechatAccount
		snapshot["wechat_name"] = account.WechatName
		snapshot["wechat_qr_path"] = account.WechatQrPath
	}
	bytes, err := common.Marshal(snapshot)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

func ApproveAffiliateWithdrawal(withdrawalId int, adminId int, remark string) error {
	return updateAffiliateWithdrawalStatus(withdrawalId, adminId, remark, AffiliateWithdrawalStatusApproved)
}

func RejectAffiliateWithdrawal(withdrawalId int, adminId int, remark string) error {
	return updateAffiliateWithdrawalStatus(withdrawalId, adminId, remark, AffiliateWithdrawalStatusRejected)
}

func MarkAffiliateWithdrawalPaid(withdrawalId int, adminId int, remark string) error {
	return updateAffiliateWithdrawalStatus(withdrawalId, adminId, remark, AffiliateWithdrawalStatusPaid)
}

func updateAffiliateWithdrawalStatus(withdrawalId int, adminId int, remark string, targetStatus string) error {
	if withdrawalId <= 0 {
		return errors.New("invalid withdrawal id")
	}
	now := common.GetTimestamp()
	return DB.Transaction(func(tx *gorm.DB) error {
		var owner struct {
			UserId int
		}
		if err := tx.Model(&AffiliateWithdrawal{}).
			Select("user_id").
			Where("id = ?", withdrawalId).
			Take(&owner).Error; err != nil {
			return err
		}

		balance, err := getAffiliateBalanceForUpdateTx(tx, owner.UserId)
		if err != nil {
			return err
		}
		withdrawal := &AffiliateWithdrawal{}
		if err := lockForUpdate(tx).Where("id = ?", withdrawalId).First(withdrawal).Error; err != nil {
			return err
		}
		if withdrawal.UserId != owner.UserId {
			return errors.New("affiliate withdrawal owner changed")
		}
		if withdrawal.Status == targetStatus {
			return nil
		}
		if withdrawal.Status == AffiliateWithdrawalStatusPaid || withdrawal.Status == AffiliateWithdrawalStatusRejected {
			return errors.New("提现申请已完结")
		}
		if targetStatus == AffiliateWithdrawalStatusApproved && withdrawal.Status != AffiliateWithdrawalStatusPending {
			return errors.New("提现申请状态不允许审核")
		}

		updates := map[string]interface{}{
			"admin_id":     adminId,
			"admin_remark": strings.TrimSpace(remark),
			"status":       targetStatus,
		}
		switch targetStatus {
		case AffiliateWithdrawalStatusApproved:
			updates["approved_time"] = now
		case AffiliateWithdrawalStatusRejected:
			if balance.FrozenQuota < withdrawal.Quota {
				return errors.New("affiliate frozen balance invariant violated")
			}
			updates["rejected_time"] = now
			balance.FrozenQuota -= withdrawal.Quota
			balance.AvailableQuota += withdrawal.Quota
		case AffiliateWithdrawalStatusPaid:
			if balance.FrozenQuota < withdrawal.Quota {
				return errors.New("affiliate frozen balance invariant violated")
			}
			updates["paid_time"] = now
			balance.FrozenQuota -= withdrawal.Quota
			balance.WithdrawnQuota += withdrawal.Quota
		default:
			return errors.New("无效的提现状态")
		}

		result := tx.Model(&AffiliateWithdrawal{}).
			Where("id = ? AND status = ?", withdrawal.Id, withdrawal.Status).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("提现申请状态已变更")
		}
		return saveAffiliateBalanceTx(tx, balance)
	})
}

func TransferAffiliateQuotaToBalance(userId int, quota int) error {
	if quota <= 0 {
		return errors.New("转入额度必须大于 0")
	}
	if err := SettleMatureAffiliateRecords(userId); err != nil {
		return err
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		balance, err := getAffiliateBalanceForUpdateTx(tx, userId)
		if err != nil {
			return err
		}
		frozen, err := queryAffiliateUserAssetsFrozenTx(tx, userId)
		if err != nil {
			return err
		}
		if frozen {
			return errors.New("返佣资产已被冻结，暂不能转入余额")
		}
		if balance.AvailableQuota < quota {
			return errors.New("可转入额度不足")
		}
		if err := creditTopUpQuota(tx, userId, quota, nil); err != nil {
			return err
		}
		balance.AvailableQuota -= quota
		balance.TransferredQuota += quota
		return saveAffiliateBalanceTx(tx, balance)
	})
	if err != nil {
		return err
	}
	syncCreditUserQuotaCache(userId, quota, "affiliate transfer")
	return nil
}

func affiliateLeaderboardPeriodStart(period string) int64 {
	now := time.Now()
	year, month, day := now.Date()
	location := now.Location()
	switch strings.ToLower(strings.TrimSpace(period)) {
	case "day":
		return time.Date(year, month, day, 0, 0, 0, 0, location).Unix()
	case "week":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start := time.Date(year, month, day, 0, 0, 0, 0, location).
			AddDate(0, 0, -(weekday - 1))
		return start.Unix()
	default:
		return time.Date(year, month, 1, 0, 0, 0, 0, location).Unix()
	}
}

func normalizeAffiliateLeaderboardSort(sortBy string) string {
	switch strings.ToLower(strings.TrimSpace(sortBy)) {
	case "invite", "invites", "invite_count", "invite_count_desc":
		return "invites"
	default:
		return "commission"
	}
}

func normalizeAffiliateLeaderboardMetric(metric string) string {
	switch strings.ToLower(strings.TrimSpace(metric)) {
	case "invite", "invites", "invite_count", "invite_count_desc":
		return "invites"
	case "commission", "reward", "reward_quota", "commission_quota":
		return "commission"
	default:
		return ""
	}
}

func maskAffiliatePublicName(name string, userId int) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Sprintf("User #%d", userId)
	}
	runes := []rune(name)
	switch len(runes) {
	case 0:
		return fmt.Sprintf("User #%d", userId)
	case 1:
		return string(runes[0]) + "*"
	case 2:
		return string(runes[0]) + "*"
	case 3:
		return string(runes[0]) + "*" + string(runes[2])
	case 4, 5, 6:
		return string(runes[:2]) + "***" + string(runes[len(runes)-1:])
	default:
		return string(runes[:3]) + "***" + string(runes[len(runes)-3:])
	}
}

func GetAffiliateLeaderboard(period string, limit int, sortBy string) ([]AffiliateLeaderboardItem, error) {
	return GetAffiliateLeaderboardByMetric(period, limit, sortBy, "")
}

func GetAffiliateLeaderboardByMetric(period string, limit int, sortBy string, metric string) ([]AffiliateLeaderboardItem, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	items, err := buildAffiliateLeaderboardByMetric(period, sortBy, metric)
	if err != nil {
		return nil, err
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func GetAffiliateLeaderboardByMetricPage(period string, page int, pageSize int, sortBy string, metric string) ([]AffiliateLeaderboardItem, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = common.ItemsPerPage
	}
	if pageSize > 100 {
		pageSize = 100
	}
	items, err := buildAffiliateLeaderboardByMetric(period, sortBy, metric)
	if err != nil {
		return nil, 0, err
	}
	total := len(items)
	start := (page - 1) * pageSize
	if start >= total {
		return []AffiliateLeaderboardItem{}, total, nil
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return items[start:end], total, nil
}

func buildAffiliateLeaderboardByMetric(period string, sortBy string, metric string) ([]AffiliateLeaderboardItem, error) {
	startTime := affiliateLeaderboardPeriodStart(period)
	normalizedSort := normalizeAffiliateLeaderboardSort(sortBy)
	normalizedMetric := normalizeAffiliateLeaderboardMetric(metric)

	type inviteRow struct {
		UserId      int
		InviteCount int
	}
	type commissionRow struct {
		UserId          int
		CommissionQuota int
	}

	var inviteRows []inviteRow
	if normalizedMetric == "" || normalizedMetric == "invites" {
		if err := DB.Model(&User{}).
			Select("inviter_id AS user_id, COUNT(*) AS invite_count").
			Where("inviter_id > 0 AND created_at >= ?", startTime).
			Group("inviter_id").
			Scan(&inviteRows).Error; err != nil {
			return nil, err
		}
	}

	var commissionRows []commissionRow
	if normalizedMetric == "" || normalizedMetric == "commission" {
		if err := DB.Model(&AffiliateRecord{}).
			Select("user_id, COALESCE(SUM(reward_quota), 0) AS commission_quota").
			Where("created_at >= ?", startTime).
			Group("user_id").
			Scan(&commissionRows).Error; err != nil {
			return nil, err
		}
	}

	itemMap := make(map[int]*AffiliateLeaderboardItem)
	for _, row := range inviteRows {
		if row.UserId <= 0 {
			continue
		}
		item := itemMap[row.UserId]
		if item == nil {
			item = &AffiliateLeaderboardItem{UserId: row.UserId}
			itemMap[row.UserId] = item
		}
		item.InviteCount = row.InviteCount
	}
	for _, row := range commissionRows {
		if row.UserId <= 0 {
			continue
		}
		item := itemMap[row.UserId]
		if item == nil {
			item = &AffiliateLeaderboardItem{UserId: row.UserId}
			itemMap[row.UserId] = item
		}
		item.CommissionQuota = row.CommissionQuota
	}

	if len(itemMap) == 0 {
		return []AffiliateLeaderboardItem{}, nil
	}

	userIds := make([]int, 0, len(itemMap))
	for userId := range itemMap {
		userIds = append(userIds, userId)
	}
	var users []User
	if err := DB.Select("id", "username", "display_name").Where("id IN ?", userIds).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		if item := itemMap[user.Id]; item != nil {
			name := user.DisplayName
			if strings.TrimSpace(name) == "" {
				name = user.Username
			}
			item.MaskedName = maskAffiliatePublicName(name, user.Id)
			item.Username = ""
			item.DisplayName = ""
		}
	}

	items := make([]AffiliateLeaderboardItem, 0, len(itemMap))
	for _, item := range itemMap {
		items = append(items, *item)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if normalizedSort == "invites" {
			if items[i].InviteCount != items[j].InviteCount {
				return items[i].InviteCount > items[j].InviteCount
			}
			if items[i].CommissionQuota != items[j].CommissionQuota {
				return items[i].CommissionQuota > items[j].CommissionQuota
			}
			return items[i].UserId < items[j].UserId
		}
		if items[i].CommissionQuota != items[j].CommissionQuota {
			return items[i].CommissionQuota > items[j].CommissionQuota
		}
		if items[i].InviteCount != items[j].InviteCount {
			return items[i].InviteCount > items[j].InviteCount
		}
		return items[i].UserId < items[j].UserId
	})
	for i := range items {
		items[i].Rank = i + 1
	}
	return items, nil
}
