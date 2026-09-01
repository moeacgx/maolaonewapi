package model

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	PromoCodeDiscountTypePercent = "percent"
	PromoCodeDiscountTypeFixed   = "fixed"
)

const (
	PromoCodeTargetTopUp        = "topup"
	PromoCodeTargetSubscription = "subscription"
)
const (
	promoReservationStatusReserved = "reserved"
	promoReservationStatusSettled  = "settled"
	promoReservationStatusReleased = "released"
)

const promoReservationTTLSeconds int64 = topUpQueryWindowSeconds

const (
	promoCodeLegacyCodeIndex = "idx_promo_codes_code"
	promoCodeUniqueIndex     = "idx_promo_codes_code_deleted_id"
)

type PromoCode struct {
	Id                       int    `json:"id"`
	UserId                   int    `json:"user_id"`
	Name                     string `json:"name" gorm:"index"`
	Code                     string `json:"code" gorm:"type:varchar(64);uniqueIndex:idx_promo_codes_code_deleted_id,priority:1"`
	DeletedId                int    `json:"-" gorm:"not null;default:0;uniqueIndex:idx_promo_codes_code_deleted_id,priority:2"`
	Status                   int    `json:"status" gorm:"default:1"`
	DiscountType             string `json:"discount_type" gorm:"type:varchar(16)"`
	DiscountValue            int64  `json:"discount_value" gorm:"type:bigint;not null;default:0"`
	AppliesToTopup           bool   `json:"applies_to_topup" gorm:"default:false"`
	AppliesToAllSubscription bool   `json:"applies_to_all_subscription" gorm:"default:false"`
	SubscriptionPlanIds      string `json:"subscription_plan_ids" gorm:"type:text"`
	MaxRedeemCount           int    `json:"max_redeem_count" gorm:"default:0"`
	RedeemedCount            int    `json:"redeemed_count" gorm:"default:0"`
	ReservedCount            int    `json:"reserved_count" gorm:"default:0"`

	CreatedTime int64          `json:"created_time" gorm:"bigint"`
	UpdatedTime int64          `json:"updated_time" gorm:"bigint"`
	ExpiredTime int64          `json:"expired_time" gorm:"bigint"`
	DeletedAt   gorm.DeletedAt `gorm:"index"`
}

type PromoCodeUsage struct {
	Id             int     `json:"id"`
	PromoCodeId    int     `json:"promo_code_id" gorm:"index;uniqueIndex:idx_promo_usage_order,priority:1"`
	UserId         int     `json:"user_id" gorm:"index"`
	OrderType      string  `json:"order_type" gorm:"type:varchar(32);index"`
	OrderNo        string  `json:"order_no" gorm:"type:varchar(255);index;uniqueIndex:idx_promo_usage_order,priority:2"`
	OriginalAmount float64 `json:"original_amount"`
	DiscountAmount float64 `json:"discount_amount"`
	PaidAmount     float64 `json:"paid_amount"`
	CreatedTime    int64   `json:"created_time" gorm:"bigint"`
}

type PromoCodeReservation struct {
	Id          int    `json:"id"`
	PromoCodeId int    `json:"promo_code_id" gorm:"index;uniqueIndex:idx_promo_reservation_order,priority:1"`
	OrderType   string `json:"order_type" gorm:"type:varchar(32);index;uniqueIndex:idx_promo_reservation_order,priority:2"`
	OrderNo     string `json:"order_no" gorm:"type:varchar(255);index;uniqueIndex:idx_promo_reservation_order,priority:3"`
	Status      string `json:"status" gorm:"type:varchar(16);index"`
	CreatedTime int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime int64  `json:"updated_time" gorm:"bigint"`
}

type PromoCodeDiscountResult struct {
	PromoCodeId     int     `json:"promo_code_id"`
	Code            string  `json:"code"`
	Name            string  `json:"name"`
	DiscountType    string  `json:"discount_type"`
	DiscountValue   int64   `json:"discount_value"`
	OriginalAmount  float64 `json:"original_amount"`
	DiscountAmount  float64 `json:"discount_amount"`
	PaidAmount      float64 `json:"paid_amount"`
	ActualPaidQuota int64   `json:"actual_paid_quota"`
}

func normalizePromoCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func (promo *PromoCode) normalize() {
	promo.Code = normalizePromoCode(promo.Code)
	promo.Name = strings.TrimSpace(promo.Name)
	promo.DiscountType = strings.ToLower(strings.TrimSpace(promo.DiscountType))
	if promo.DiscountType == "" {
		promo.DiscountType = PromoCodeDiscountTypePercent
	}
	promo.SubscriptionPlanIds = normalizePromoSubscriptionPlanIds(promo.SubscriptionPlanIds)
	if promo.Status == 0 {
		promo.Status = common.RedemptionCodeStatusEnabled
	}
	now := common.GetTimestamp()
	if promo.CreatedTime == 0 {
		promo.CreatedTime = now
	}
	promo.UpdatedTime = now
}

func normalizePromoSubscriptionPlanIds(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	seen := map[int]struct{}{}
	ids := make([]string, 0)
	for _, item := range strings.Split(raw, ",") {
		id, err := strconv.Atoi(strings.TrimSpace(item))
		if err != nil || id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, strconv.Itoa(id))
	}
	return strings.Join(ids, ",")
}

func promoPlanIdIncluded(raw string, planId int) bool {
	if planId <= 0 {
		return false
	}
	for _, item := range strings.Split(raw, ",") {
		id, err := strconv.Atoi(strings.TrimSpace(item))
		if err == nil && id == planId {
			return true
		}
	}
	return false
}

func validatePromoCode(promo *PromoCode) error {
	if promo == nil {
		return errors.New("优惠码为空")
	}
	promo.normalize()
	if promo.Code == "" {
		return errors.New("优惠码不能为空")
	}
	if promo.Name == "" {
		return errors.New("优惠码名称不能为空")
	}
	if promo.DiscountType != PromoCodeDiscountTypePercent && promo.DiscountType != PromoCodeDiscountTypeFixed {
		return errors.New("无效的优惠类型")
	}
	if promo.DiscountValue <= 0 {
		return errors.New("优惠值必须大于 0")
	}
	if promo.DiscountType == PromoCodeDiscountTypePercent && promo.DiscountValue > 100 {
		return errors.New("优惠百分比不能超过 100")
	}
	if !promo.AppliesToTopup && !promo.AppliesToAllSubscription && promo.SubscriptionPlanIds == "" {
		return errors.New("优惠码必须至少指定一个适用范围")
	}
	if promo.MaxRedeemCount < 0 {
		return errors.New("最大使用次数不能为负数")
	}
	if promo.ExpiredTime != 0 && promo.ExpiredTime < common.GetTimestamp() {
		return errors.New("过期时间不能早于当前时间")
	}
	return nil
}

func migratePromoCodeDeletionKey(db *gorm.DB) error {
	if db == nil {
		return errors.New("数据库连接为空")
	}
	if !db.Migrator().HasColumn(&PromoCode{}, "DeletedId") {
		if err := db.Migrator().AddColumn(&PromoCode{}, "DeletedId"); err != nil {
			return fmt.Errorf("添加优惠码删除标识失败: %w", err)
		}
	}
	if err := db.Unscoped().Model(&PromoCode{}).
		Where("deleted_at IS NOT NULL AND deleted_id = ?", 0).
		UpdateColumn("deleted_id", gorm.Expr("id")).Error; err != nil {
		return fmt.Errorf("回填优惠码删除标识失败: %w", err)
	}
	if !db.Migrator().HasIndex(&PromoCode{}, promoCodeUniqueIndex) {
		if err := db.Migrator().CreateIndex(&PromoCode{}, promoCodeUniqueIndex); err != nil {
			return fmt.Errorf("创建优惠码组合唯一索引失败: %w", err)
		}
	}
	if err := dropPromoCodeLegacyUniqueKey(db); err != nil {
		return fmt.Errorf("删除优惠码旧唯一键失败: %w", err)
	}
	return nil
}

func dropPromoCodeLegacyUniqueKey(db *gorm.DB) error {
	migrator := db.Migrator()
	// PostgreSQL 的旧版本可能把 uniqueIndex 落成同名 UNIQUE CONSTRAINT。
	// 约束的支撑索引不能直接 DROP INDEX，必须先删除约束。
	if db.Dialector.Name() == "postgres" && migrator.HasConstraint(&PromoCode{}, promoCodeLegacyCodeIndex) {
		if err := migrator.DropConstraint(&PromoCode{}, promoCodeLegacyCodeIndex); err != nil {
			return fmt.Errorf("删除 PostgreSQL 唯一约束失败: %w", err)
		}
	}
	if migrator.HasIndex(&PromoCode{}, promoCodeLegacyCodeIndex) {
		if err := migrator.DropIndex(&PromoCode{}, promoCodeLegacyCodeIndex); err != nil {
			return fmt.Errorf("删除唯一索引失败: %w", err)
		}
	}
	return nil
}

// preparePromoCodeForWriteTx 兼容旧版本只写 deleted_at、deleted_id 仍为 0 的记录。
// 活动冲突仍被拒绝；历史软删除冲突则补齐 deleted_id，释放活动唯一键。
func preparePromoCodeForWriteTx(tx *gorm.DB, code string, currentId int) error {
	var existing PromoCode
	query := lockForUpdate(tx).Unscoped().Where("code = ? AND deleted_id = ?", code, 0)
	if currentId > 0 {
		query = query.Where("id <> ?", currentId)
	}
	err := query.First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !existing.DeletedAt.Valid {
		return errors.New("优惠码已存在")
	}
	return tx.Unscoped().Model(&PromoCode{}).
		Where("id = ? AND deleted_id = ?", existing.Id, 0).
		UpdateColumn("deleted_id", existing.Id).Error
}

func (promo *PromoCode) Insert() error {
	promo.RedeemedCount = 0
	promo.ReservedCount = 0
	if err := validatePromoCode(promo); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := preparePromoCodeForWriteTx(tx, promo.Code, 0); err != nil {
			return err
		}
		return tx.Create(promo).Error
	})
}

func (promo *PromoCode) Update() error {
	if err := validatePromoCode(promo); err != nil {
		return err
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var current PromoCode
		if err := lockForUpdate(tx).Where("id = ?", promo.Id).First(&current).Error; err != nil {
			return err
		}
		if err := reclaimExpiredPromoReservationsTx(tx, &current, common.GetTimestamp()); err != nil {
			return err
		}
		if promo.MaxRedeemCount > 0 && promo.MaxRedeemCount < current.RedeemedCount+current.ReservedCount {
			return errors.New("使用次数不能小于已使用及已预留次数")
		}
		if err := preparePromoCodeForWriteTx(tx, promo.Code, promo.Id); err != nil {
			return err
		}
		return tx.Model(&current).Select(
			"name",
			"code",
			"status",
			"discount_type",
			"discount_value",
			"applies_to_topup",
			"applies_to_all_subscription",
			"subscription_plan_ids",
			"max_redeem_count",
			"updated_time",
			"expired_time",
		).Updates(promo).Error
	})
}

func GetPromoCodeById(id int) (*PromoCode, error) {
	if id <= 0 {
		return nil, errors.New("id 为空")
	}
	promo := &PromoCode{}
	return promo, DB.Where("id = ?", id).First(promo).Error
}

func GetAllPromoCodes(startIdx int, num int) (promoCodes []*PromoCode, total int64, err error) {
	tx := DB.Model(&PromoCode{})
	if err = tx.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err = tx.Order("id desc").Limit(num).Offset(startIdx).Find(&promoCodes).Error
	return promoCodes, total, err
}

func SearchPromoCodes(keyword string, startIdx int, num int) (promoCodes []*PromoCode, total int64, err error) {
	keyword = strings.TrimSpace(keyword)
	query := DB.Model(&PromoCode{})
	if keyword != "" {
		normalized := normalizePromoCode(keyword)
		if id, convErr := strconv.Atoi(keyword); convErr == nil {
			query = query.Where("id = ? OR code LIKE ? OR name LIKE ?", id, normalized+"%", keyword+"%")
		} else {
			query = query.Where("code LIKE ? OR name LIKE ?", normalized+"%", keyword+"%")
		}
	}
	if err = query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err = query.Order("id desc").Limit(num).Offset(startIdx).Find(&promoCodes).Error
	return promoCodes, total, err
}

func DeletePromoCodeById(id int) error {
	if id <= 0 {
		return errors.New("id 为空")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		var promo PromoCode
		err := lockForUpdate(tx).Where("id = ?", id).First(&promo).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := tx.Model(&promo).UpdateColumn("deleted_id", promo.Id).Error; err != nil {
			return err
		}
		return tx.Delete(&promo).Error
	})
}

// DeletePromoCodesByIDs 批量软删除优惠码，逐条写入 deleted_id 以保持历史唯一键契约。
func DeletePromoCodesByIDs(ids []int) ([]int, error) {
	if len(ids) == 0 {
		return nil, errors.New("优惠码 ID 不能为空")
	}
	deleted := make([]int, 0, len(ids))
	err := DB.Transaction(func(tx *gorm.DB) error {
		var promos []PromoCode
		if err := lockForUpdate(tx).Where("id IN ?", ids).Find(&promos).Error; err != nil {
			return err
		}
		for i := range promos {
			promo := &promos[i]
			if err := tx.Model(promo).UpdateColumn("deleted_id", promo.Id).Error; err != nil {
				return err
			}
			if err := tx.Delete(promo).Error; err != nil {
				return err
			}
			deleted = append(deleted, promo.Id)
		}
		return nil
	})
	return deleted, err
}

func DeleteInvalidPromoCodes(now int64) ([]int, error) {
	if now <= 0 {
		now = common.GetTimestamp()
	}
	var ids []int
	err := DB.Transaction(func(tx *gorm.DB) error {
		var promos []PromoCode
		if err := lockForUpdate(tx).Where("status IN ? OR (status = ? AND expired_time != 0 AND expired_time <= ?)", []int{common.RedemptionCodeStatusUsed, common.RedemptionCodeStatusDisabled}, common.RedemptionCodeStatusEnabled, now).Find(&promos).Error; err != nil {
			return err
		}
		for i := range promos {
			promo := &promos[i]
			if err := tx.Model(promo).UpdateColumn("deleted_id", promo.Id).Error; err != nil {
				return err
			}
			if err := tx.Delete(promo).Error; err != nil {
				return err
			}
			ids = append(ids, promo.Id)
		}
		return nil
	})
	return ids, err
}

func promoAppliesToTarget(promo *PromoCode, target string, planId int) bool {
	switch target {
	case PromoCodeTargetTopUp:
		return promo.AppliesToTopup
	case PromoCodeTargetSubscription:
		return promo.AppliesToAllSubscription || promoPlanIdIncluded(promo.SubscriptionPlanIds, planId)
	default:
		return false
	}
}

func CalculatePromoCodeDiscount(code string, target string, planId int, originalAmount float64) (*PromoCodeDiscountResult, error) {
	var discount *PromoCodeDiscountResult
	err := DB.Transaction(func(tx *gorm.DB) error {
		var err error
		discount, err = calculatePromoCodeDiscountTx(tx, code, target, planId, originalAmount)
		return err
	})
	return discount, err
}

func calculatePromoCodeDiscountTx(tx *gorm.DB, code string, target string, planId int, originalAmount float64) (*PromoCodeDiscountResult, error) {
	code = normalizePromoCode(code)
	if code == "" {
		return nil, nil
	}
	if tx == nil {
		tx = DB
	}
	if originalAmount < 0 {
		return nil, errors.New("原始金额不能为负数")
	}
	var promo PromoCode
	if err := lockForUpdate(tx).Where("code = ?", code).First(&promo).Error; err != nil {
		return nil, errors.New("无效的优惠码")
	}
	if err := reclaimExpiredPromoReservationsTx(tx, &promo, common.GetTimestamp()); err != nil {
		return nil, err
	}
	if promo.Status != common.RedemptionCodeStatusEnabled {
		return nil, errors.New("优惠码不可用")
	}
	if promo.ExpiredTime != 0 && promo.ExpiredTime < common.GetTimestamp() {
		return nil, errors.New("优惠码已过期")
	}
	if promo.MaxRedeemCount > 0 && promo.RedeemedCount+promo.ReservedCount >= promo.MaxRedeemCount {
		return nil, errors.New("优惠码已达使用次数上限")
	}
	if !promoAppliesToTarget(&promo, target, planId) {
		return nil, errors.New("优惠码不适用于当前订单")
	}
	discount := decimal.Zero
	original := decimal.NewFromFloat(originalAmount)
	switch promo.DiscountType {
	case PromoCodeDiscountTypePercent:
		discount = original.Mul(decimal.NewFromInt(promo.DiscountValue)).Div(decimal.NewFromInt(100))
	case PromoCodeDiscountTypeFixed:
		discount = decimal.NewFromInt(promo.DiscountValue).Div(decimal.NewFromFloat(common.QuotaPerUnit))
	default:
		return nil, errors.New("无效的优惠类型")
	}
	if discount.LessThan(decimal.Zero) {
		discount = decimal.Zero
	}
	if discount.GreaterThan(original) {
		discount = original
	}
	discount = discount.Round(2)
	paid := original.Sub(discount).Round(2)
	if paid.LessThan(decimal.Zero) {
		paid = decimal.Zero
	}
	paidFloat := paid.InexactFloat64()
	actualPaidQuota, err := common.WalletQuotaFromDecimalStrict(paid.Mul(decimal.NewFromFloat(common.QuotaPerUnit)))
	if err != nil {
		return nil, errors.New("优惠码折后额度超出系统可表示范围")
	}
	return &PromoCodeDiscountResult{
		PromoCodeId:     promo.Id,
		Code:            promo.Code,
		Name:            promo.Name,
		DiscountType:    promo.DiscountType,
		DiscountValue:   promo.DiscountValue,
		OriginalAmount:  original.Round(2).InexactFloat64(),
		DiscountAmount:  discount.InexactFloat64(),
		PaidAmount:      paidFloat,
		ActualPaidQuota: actualPaidQuota,
	}, nil
}

func ApplyPromoCodeResultToTopUp(topUp *TopUp, discount *PromoCodeDiscountResult) {
	if topUp == nil || discount == nil {
		return
	}
	topUp.OriginalMoney = discount.OriginalAmount
	topUp.DiscountMoney = discount.DiscountAmount
	topUp.ActualMoney = discount.PaidAmount
	topUp.PromoCodeId = discount.PromoCodeId
	topUp.PromoCode = discount.Code
	topUp.Money = discount.PaidAmount
	topUp.AffiliateSourceQuota = discount.ActualPaidQuota
}

func ApplyPromoCodeResultToStripeTopUp(topUp *TopUp, discount *PromoCodeDiscountResult, rechargeMoney float64) {
	if topUp == nil || discount == nil {
		return
	}
	ApplyPromoCodeResultToTopUp(topUp, discount)
	topUp.Money = rechargeMoney
}

func ApplyPromoCodeResultToSubscriptionOrder(order *SubscriptionOrder, discount *PromoCodeDiscountResult) {
	if order == nil || discount == nil {
		return
	}
	order.OriginalMoney = discount.OriginalAmount
	order.DiscountMoney = discount.DiscountAmount
	order.ActualMoney = discount.PaidAmount
	order.PromoCodeId = discount.PromoCodeId
	order.PromoCode = discount.Code
	order.Money = discount.PaidAmount
	order.AffiliateSourceQuota = discount.ActualPaidQuota
}

func topUpAffiliateSourceQuota[T walletQuotaValue](topUp *TopUp, fallbackQuota T) int64 {
	if topUp != nil && topUp.AffiliateSourceQuota > 0 {
		return topUp.AffiliateSourceQuota
	}
	return int64(fallbackQuota)
}

func TopUpAffiliateSourceQuota[T walletQuotaValue](topUp *TopUp, fallbackQuota T) int64 {
	return topUpAffiliateSourceQuota(topUp, fallbackQuota)
}

func promoReservationQuery(tx *gorm.DB, promoCodeId int, orderType string, orderNo string) *gorm.DB {
	return tx.Where("promo_code_id = ? AND order_type = ? AND order_no = ?", promoCodeId, orderType, orderNo)
}

func promoReservationCallbackEligibleTx(tx *gorm.DB, reservation *PromoCodeReservation) (bool, error) {
	if tx == nil || reservation == nil {
		return false, nil
	}
	switch reservation.OrderType {
	case PromoCodeTargetTopUp:
		var topUp TopUp
		if err := tx.Where("trade_no = ?", reservation.OrderNo).First(&topUp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
		if topUp.CreateTime < topUpQueryCutoff() {
			return false, nil
		}
		var attemptCount int64
		if err := tx.Model(&TopUpPaymentAttempt{}).
			Where("top_up_id = ? AND create_time >= ? AND status IN ?", topUp.Id, topUpQueryCutoff(), []string{
				TopUpPaymentAttemptCreated,
				TopUpPaymentAttemptLaunched,
				TopUpPaymentAttemptLaunchFailed,
				TopUpPaymentAttemptSucceeded,
			}).Count(&attemptCount).Error; err != nil {
			return false, err
		}
		return attemptCount > 0 || strings.TrimSpace(topUp.ProviderOrderId) != "", nil
	case PromoCodeTargetSubscription:
		var order SubscriptionOrder
		if err := tx.Where("trade_no = ?", reservation.OrderNo).First(&order).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
		if order.CreateTime < topUpQueryCutoff() {
			return false, nil
		}
		if strings.TrimSpace(order.ProviderOrderId) != "" {
			return true, nil
		}
		return order.PaymentProvider == PaymentProviderEpay &&
			strings.TrimSpace(order.ProviderAmount) != "" &&
			strings.TrimSpace(order.ProviderCurrency) != "", nil
	default:
		return false, nil
	}
}

func reclaimExpiredPromoReservationsTx(tx *gorm.DB, promo *PromoCode, now int64) error {
	if tx == nil || promo == nil || promo.Id <= 0 || promo.ReservedCount <= 0 {
		return nil
	}
	var candidates []PromoCodeReservation
	if err := tx.Where("promo_code_id = ? AND status = ? AND updated_time < ?", promo.Id, promoReservationStatusReserved, now-promoReservationTTLSeconds).
		Find(&candidates).Error; err != nil {
		return err
	}
	reclaimIds := make([]int, 0, len(candidates))
	for i := range candidates {
		eligible, err := promoReservationCallbackEligibleTx(tx, &candidates[i])
		if err != nil {
			return err
		}
		if !eligible {
			reclaimIds = append(reclaimIds, candidates[i].Id)
		}
	}
	if len(reclaimIds) == 0 {
		return nil
	}
	result := tx.Model(&PromoCodeReservation{}).
		Where("id IN ? AND status = ?", reclaimIds, promoReservationStatusReserved).
		Updates(map[string]interface{}{
			"status":       promoReservationStatusReleased,
			"updated_time": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return nil
	}
	reclaimed := result.RowsAffected
	if reclaimed > int64(promo.ReservedCount) {
		reclaimed = int64(promo.ReservedCount)
	}
	if err := tx.Unscoped().Model(&PromoCode{}).Where("id = ?", promo.Id).Updates(map[string]interface{}{
		"reserved_count": gorm.Expr("CASE WHEN reserved_count >= ? THEN reserved_count - ? ELSE 0 END", reclaimed, reclaimed),
		"updated_time":   now,
	}).Error; err != nil {
		return err
	}
	promo.ReservedCount -= int(reclaimed)
	return nil
}

func reservePromoCodeForOrderTx(tx *gorm.DB, promoCodeId int, orderType string, orderNo string, planId int) error {
	orderNo = strings.TrimSpace(orderNo)
	if tx == nil || promoCodeId <= 0 || orderNo == "" {
		return nil
	}

	var promo PromoCode
	if err := lockForUpdate(tx).Where("id = ?", promoCodeId).First(&promo).Error; err != nil {
		return err
	}
	now := common.GetTimestamp()
	if err := reclaimExpiredPromoReservationsTx(tx, &promo, now); err != nil {
		return err
	}

	var existing PromoCodeReservation
	err := promoReservationQuery(tx, promoCodeId, orderType, orderNo).First(&existing).Error
	if err == nil && existing.Status == promoReservationStatusReserved {
		return nil
	}
	if err == nil && existing.Status == promoReservationStatusSettled {
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if promo.Status != common.RedemptionCodeStatusEnabled {
		return errors.New("优惠码不可用")
	}
	if promo.ExpiredTime != 0 && promo.ExpiredTime < now {
		return errors.New("优惠码已过期")
	}
	if !promoAppliesToTarget(&promo, orderType, planId) {
		return errors.New("优惠码不适用于当前订单")
	}

	result := tx.Model(&PromoCode{}).
		Where("id = ? AND status = ? AND (expired_time = 0 OR expired_time >= ?) AND (max_redeem_count = 0 OR redeemed_count + reserved_count < max_redeem_count)",
			promoCodeId, common.RedemptionCodeStatusEnabled, now).
		Updates(map[string]interface{}{
			"reserved_count": gorm.Expr("reserved_count + ?", 1),
			"updated_time":   now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("优惠码已达使用次数上限")
	}

	if existing.Id > 0 {
		return tx.Model(&existing).Updates(map[string]interface{}{
			"status":       promoReservationStatusReserved,
			"updated_time": now,
		}).Error
	}
	return tx.Create(&PromoCodeReservation{
		PromoCodeId: promoCodeId,
		OrderType:   orderType,
		OrderNo:     orderNo,
		Status:      promoReservationStatusReserved,
		CreatedTime: now,
		UpdatedTime: now,
	}).Error
}

func releasePromoCodeReservationTx(tx *gorm.DB, promoCodeId int, orderType string, orderNo string) error {
	if tx == nil || promoCodeId <= 0 || strings.TrimSpace(orderNo) == "" {
		return nil
	}
	var promo PromoCode
	if err := lockForUpdate(tx).Unscoped().Where("id = ?", promoCodeId).First(&promo).Error; err != nil {
		return err
	}
	var reservation PromoCodeReservation
	err := promoReservationQuery(tx, promoCodeId, orderType, orderNo).First(&reservation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if reservation.Status != promoReservationStatusReserved {
		return nil
	}
	callbackEligible, err := promoReservationCallbackEligibleTx(tx, &reservation)
	if err != nil {
		return err
	}
	if callbackEligible {
		return nil
	}
	now := common.GetTimestamp()
	if err := tx.Unscoped().Model(&PromoCode{}).Where("id = ? AND reserved_count > 0", promoCodeId).Updates(map[string]interface{}{
		"reserved_count": gorm.Expr("reserved_count - ?", 1),
		"updated_time":   now,
	}).Error; err != nil {
		return err
	}
	return tx.Model(&reservation).Updates(map[string]interface{}{
		"status":       promoReservationStatusReleased,
		"updated_time": now,
	}).Error
}

func promoCodeStatusAfterRedemption(currentStatus int, maxRedeemCount int, redeemedCount int, increment int) int {
	if increment > 0 && maxRedeemCount > 0 && redeemedCount+increment >= maxRedeemCount {
		return common.RedemptionCodeStatusUsed
	}
	return currentStatus
}

func promoCodeSettlementUpdates(promo *PromoCode, now int64, hasActiveReservation bool) map[string]interface{} {
	updates := map[string]interface{}{
		"redeemed_count": gorm.Expr("redeemed_count + ?", 1),
		"updated_time":   now,
		"status": promoCodeStatusAfterRedemption(
			promo.Status,
			promo.MaxRedeemCount,
			promo.RedeemedCount,
			1,
		),
	}
	if hasActiveReservation {
		updates["reserved_count"] = gorm.Expr("CASE WHEN reserved_count > 0 THEN reserved_count - 1 ELSE 0 END")
	}
	return updates
}

func recordPromoCodeUsageTx(tx *gorm.DB, promoCodeId int, userId int, orderType string, orderNo string, originalAmount float64, discountAmount float64, paidAmount float64, enforceLimit bool) error {
	if tx == nil || promoCodeId <= 0 || userId <= 0 || strings.TrimSpace(orderNo) == "" {
		return nil
	}
	var existing PromoCodeUsage
	err := tx.Where("promo_code_id = ? AND order_no = ?", promoCodeId, orderNo).First(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	var promo PromoCode
	if err := lockForUpdate(tx).Unscoped().Where("id = ?", promoCodeId).First(&promo).Error; err != nil {
		if !enforceLimit && errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	now := common.GetTimestamp()
	if err := reclaimExpiredPromoReservationsTx(tx, &promo, now); err != nil {
		return err
	}

	var reservation PromoCodeReservation
	reservationErr := promoReservationQuery(tx, promoCodeId, orderType, orderNo).First(&reservation).Error
	if reservationErr != nil && !errors.Is(reservationErr, gorm.ErrRecordNotFound) {
		return reservationErr
	}
	hasActiveReservation := reservationErr == nil && reservation.Status == promoReservationStatusReserved

	updates := promoCodeSettlementUpdates(&promo, now, hasActiveReservation)
	capacityQuery := tx.Unscoped().Model(&PromoCode{}).Where("id = ?", promoCodeId)
	if enforceLimit && !hasActiveReservation {
		capacityQuery = capacityQuery.Where("max_redeem_count = 0 OR redeemed_count + reserved_count < max_redeem_count")
	}
	auditOverCapacity := !enforceLimit && !hasActiveReservation && promo.MaxRedeemCount > 0 && promo.RedeemedCount+promo.ReservedCount >= promo.MaxRedeemCount
	result := capacityQuery.Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("优惠码已达使用次数上限")
	}

	usage := &PromoCodeUsage{
		PromoCodeId:    promoCodeId,
		UserId:         userId,
		OrderType:      orderType,
		OrderNo:        orderNo,
		OriginalAmount: originalAmount,
		DiscountAmount: discountAmount,
		PaidAmount:     paidAmount,
		CreatedTime:    now,
	}
	if err := tx.Create(usage).Error; err != nil {
		return err
	}
	if reservationErr == nil {
		if err := tx.Model(&reservation).Updates(map[string]interface{}{
			"status":       promoReservationStatusSettled,
			"updated_time": now,
		}).Error; err != nil {
			return err
		}
	} else if err := tx.Create(&PromoCodeReservation{
		PromoCodeId: promoCodeId,
		OrderType:   orderType,
		OrderNo:     orderNo,
		Status:      promoReservationStatusSettled,
		CreatedTime: now,
		UpdatedTime: now,
	}).Error; err != nil {
		return err
	}
	if auditOverCapacity {
		common.SysError(fmt.Sprintf(
			"paid promo settlement exceeded capacity: promo_id=%d user_id=%d order_type=%s",
			promo.Id,
			userId,
			strings.ToLower(strings.TrimSpace(orderType)),
		))
	}
	return nil
}

func recordTopUpPromoUsageTx(tx *gorm.DB, topUp *TopUp, enforceLimit bool) error {
	if topUp == nil || topUp.PromoCodeId <= 0 {
		return nil
	}
	return recordPromoCodeUsageTx(tx, topUp.PromoCodeId, topUp.UserId, PromoCodeTargetTopUp, topUp.TradeNo, topUp.OriginalMoney, topUp.DiscountMoney, topUp.ActualMoney, enforceLimit)
}

func RecordTopUpPromoUsageForPayment(topUp *TopUp) error {
	if topUp == nil || topUp.PromoCodeId <= 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return recordTopUpPromoUsageTx(tx, topUp, false)
	})
}

func recordSubscriptionPromoUsageTx(tx *gorm.DB, order *SubscriptionOrder, enforceLimit bool) error {
	if order == nil || order.PromoCodeId <= 0 {
		return nil
	}
	return recordPromoCodeUsageTx(tx, order.PromoCodeId, order.UserId, PromoCodeTargetSubscription, order.TradeNo, order.OriginalMoney, order.DiscountMoney, order.ActualMoney, enforceLimit)
}

func formatPromoAppliedLog(code string, discount float64) string {
	if strings.TrimSpace(code) == "" || discount <= 0 {
		return ""
	}
	return fmt.Sprintf("，优惠码: %s，优惠金额: %.2f", code, discount)
}
