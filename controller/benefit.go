package controller

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type benefitActivityRequest struct {
	Name                 string          `json:"name"`
	Description          string          `json:"description"`
	GroupID              int             `json:"group_id"`
	AmountMode           string          `json:"amount_mode"`
	TotalAmount          decimal.Decimal `json:"total_amount"`
	TotalCount           int             `json:"total_count"`
	FixedAmount          decimal.Decimal `json:"fixed_amount"`
	MinAmount            decimal.Decimal `json:"min_amount"`
	MaxAmount            decimal.Decimal `json:"max_amount"`
	ClaimPaidThreshold   decimal.Decimal `json:"claim_paid_threshold"`
	PersonalValidSeconds int64           `json:"personal_valid_seconds"`
	StartsAt             int64           `json:"starts_at"`
	EndsAt               int64           `json:"ends_at"`
}

type benefitActivityResponse struct {
	model.BenefitActivity
	TotalAmount        float64 `json:"total_amount"`
	FixedAmount        float64 `json:"fixed_amount"`
	MinAmount          float64 `json:"min_amount"`
	MaxAmount          float64 `json:"max_amount"`
	ClaimPaidThreshold float64 `json:"claim_paid_threshold"`
}

type benefitActivityUserResponse struct {
	model.BenefitActivityUserView
	TotalAmount        float64 `json:"total_amount"`
	FixedAmount        float64 `json:"fixed_amount"`
	MinAmount          float64 `json:"min_amount"`
	MaxAmount          float64 `json:"max_amount"`
	ClaimPaidThreshold float64 `json:"claim_paid_threshold"`
}

type benefitVoucherResponse struct {
	model.BenefitUserVoucher
	OriginalAmount float64 `json:"original_amount"`
}

func newBenefitActivityResponse(activity *model.BenefitActivity) *benefitActivityResponse {
	if activity == nil {
		return nil
	}
	return &benefitActivityResponse{
		BenefitActivity:    *activity,
		TotalAmount:        float64(activity.TotalAmountCents) / 100,
		FixedAmount:        float64(activity.FixedAmountCents) / 100,
		MinAmount:          float64(activity.MinAmountCents) / 100,
		MaxAmount:          float64(activity.MaxAmountCents) / 100,
		ClaimPaidThreshold: float64(activity.ClaimPaidThresholdCents) / 100,
	}
}

func newBenefitActivityResponses(activities []model.BenefitActivity) []*benefitActivityResponse {
	responses := make([]*benefitActivityResponse, len(activities))
	for index := range activities {
		responses[index] = newBenefitActivityResponse(&activities[index])
	}
	return responses
}

func newBenefitActivityUserResponses(activities []model.BenefitActivityUserView) []*benefitActivityUserResponse {
	responses := make([]*benefitActivityUserResponse, len(activities))
	for index := range activities {
		activity := &activities[index]
		responses[index] = &benefitActivityUserResponse{
			BenefitActivityUserView: *activity,
			TotalAmount:             float64(activity.TotalAmountCents) / 100,
			FixedAmount:             float64(activity.FixedAmountCents) / 100,
			MinAmount:               float64(activity.MinAmountCents) / 100,
			MaxAmount:               float64(activity.MaxAmountCents) / 100,
			ClaimPaidThreshold:      float64(activity.ClaimPaidThresholdCents) / 100,
		}
	}
	return responses
}

func newBenefitVoucherResponse(voucher *model.BenefitUserVoucher) *benefitVoucherResponse {
	if voucher == nil {
		return nil
	}
	return &benefitVoucherResponse{
		BenefitUserVoucher: *voucher,
		OriginalAmount:     float64(voucher.OriginalAmountCents) / 100,
	}
}

func newBenefitVoucherResponses(vouchers []model.BenefitUserVoucher) []*benefitVoucherResponse {
	responses := make([]*benefitVoucherResponse, len(vouchers))
	for index := range vouchers {
		responses[index] = newBenefitVoucherResponse(&vouchers[index])
	}
	return responses
}

func benefitAmountYuanToMinorUnits(amount decimal.Decimal, allowZero bool) (int64, error) {
	if amount.IsNegative() || (!allowZero && amount.IsZero()) {
		return 0, errors.New("金额必须大于 0")
	}
	scaled := amount.Mul(decimal.NewFromInt(100))
	if !scaled.IsInteger() {
		return 0, errors.New("金额最多只能保留两位小数")
	}
	max := decimal.NewFromInt(int64(^uint64(0) >> 1))
	if scaled.GreaterThan(max) {
		return 0, errors.New("金额超出系统可表示范围")
	}
	return scaled.IntPart(), nil
}

func (request *benefitActivityRequest) toModel() (*model.BenefitActivity, error) {
	if request == nil {
		return nil, errors.New("福利活动参数不能为空")
	}
	totalAmount, err := benefitAmountYuanToMinorUnits(request.TotalAmount, false)
	if err != nil {
		return nil, fmt.Errorf("总预算无效：%w", err)
	}
	fixedAmount, err := benefitAmountYuanToMinorUnits(request.FixedAmount, request.AmountMode == model.BenefitAmountModeRandom)
	if err != nil {
		return nil, fmt.Errorf("固定面额无效：%w", err)
	}
	minAmount, err := benefitAmountYuanToMinorUnits(request.MinAmount, request.AmountMode == model.BenefitAmountModeFixed)
	if err != nil {
		return nil, fmt.Errorf("最小面额无效：%w", err)
	}
	maxAmount, err := benefitAmountYuanToMinorUnits(request.MaxAmount, request.AmountMode == model.BenefitAmountModeFixed)
	if err != nil {
		return nil, fmt.Errorf("最大面额无效：%w", err)
	}
	threshold, err := benefitAmountYuanToMinorUnits(request.ClaimPaidThreshold, true)
	if err != nil {
		return nil, fmt.Errorf("实付门槛无效：%w", err)
	}
	totalQuota, err := model.BenefitAmountCNYToQuota(totalAmount)
	if err != nil {
		return nil, err
	}
	return &model.BenefitActivity{
		Name: request.Name, Description: request.Description, GroupId: request.GroupID,
		AmountMode: request.AmountMode, TotalAmountCents: totalAmount,
		TotalQuota: totalQuota, TotalCount: request.TotalCount,
		FixedAmountCents: fixedAmount, MinAmountCents: minAmount,
		MaxAmountCents: maxAmount, ClaimPaidThresholdCents: threshold,
		PersonalValidSeconds: request.PersonalValidSeconds,
		StartsAt:             request.StartsAt, EndsAt: request.EndsAt,
	}, nil
}

type benefitTerminateRequest struct {
	Mode    string `json:"mode"`
	Confirm bool   `json:"confirm"`
	Reason  string `json:"reason"`
}

func benefitNow() int64 {
	return common.GetTimestamp()
}

func benefitPathID(c *gin.Context) (int, error) {
	id, err := strconv.Atoi(strings.TrimSpace(c.Param("id")))
	if err != nil || id <= 0 {
		return 0, errors.New("福利活动 ID 无效")
	}
	return id, nil
}

func benefitApiError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, model.ErrBenefitClaimIneligible):
		common.ApiErrorMsg(c, "不符合领取条件")
	case errors.Is(err, model.ErrBenefitAlreadyClaimed):
		common.ApiErrorMsg(c, "每个活动只能领取一次")
	case errors.Is(err, model.ErrBenefitSoldOut):
		common.ApiErrorMsg(c, "已领完")
	case errors.Is(err, model.ErrBenefitActivityNotClaimable):
		common.ApiErrorMsg(c, "活动当前不可领取")
	case errors.Is(err, gorm.ErrRecordNotFound):
		common.ApiErrorMsg(c, "福利活动不存在")
	default:
		common.ApiError(c, err)
	}
}

func GetBenefitActivities(c *gin.Context) {
	activities, err := model.ListBenefitActivitiesForUser(c.GetInt("id"), common.GetTimestamp())
	if err != nil {
		benefitApiError(c, err)
		return
	}
	common.ApiSuccess(c, newBenefitActivityUserResponses(activities))
}

func GetBenefitVouchers(c *gin.Context) {
	vouchers, err := model.ListBenefitVouchersForUser(c.GetInt("id"), common.GetTimestamp())
	if err != nil {
		benefitApiError(c, err)
		return
	}
	common.ApiSuccess(c, newBenefitVoucherResponses(vouchers))
}

func ClaimBenefitActivity(c *gin.Context) {
	activityID, err := benefitPathID(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	voucher, err := model.ClaimBenefitActivity(activityID, c.GetInt("id"), common.GetTimestamp())
	if err != nil {
		benefitApiError(c, err)
		return
	}
	common.ApiSuccess(c, newBenefitVoucherResponse(voucher))
}

func GetBenefitAdminActivities(c *gin.Context) {
	page := common.GetPageQuery(c)
	activities, total, err := model.ListBenefitActivitiesForAdmin(page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	page.SetTotal(int(total))
	page.SetItems(newBenefitActivityResponses(activities))
	common.ApiSuccess(c, page)
}

func CreateBenefitAdminActivity(c *gin.Context) {
	var request benefitActivityRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "福利活动参数格式错误")
		return
	}
	activity, err := request.toModel()
	if err != nil {
		benefitApiError(c, err)
		return
	}
	if err := model.CreateBenefitActivity(activity, c.GetInt("id"), common.GetTimestamp()); err != nil {
		benefitApiError(c, err)
		return
	}
	recordManageAudit(c, "benefit.activity.create", map[string]interface{}{"id": activity.Id, "name": activity.Name})
	common.ApiSuccess(c, newBenefitActivityResponse(activity))
}

func GetBenefitAdminActivity(c *gin.Context) {
	activityID, err := benefitPathID(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	activity, err := model.GetBenefitActivityForAdmin(activityID)
	if err != nil {
		benefitApiError(c, err)
		return
	}
	common.ApiSuccess(c, newBenefitActivityResponse(activity))
}

func UpdateBenefitAdminActivity(c *gin.Context) {
	activityID, err := benefitPathID(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	var request benefitActivityRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "福利活动参数格式错误")
		return
	}
	stored, getErr := model.GetBenefitActivityForAdmin(activityID)
	if getErr != nil {
		benefitApiError(c, getErr)
		return
	}
	var updated *model.BenefitActivity
	if stored.Status == model.BenefitActivityStatusDraft {
		activity, convertErr := request.toModel()
		if convertErr != nil {
			benefitApiError(c, convertErr)
			return
		}
		activity.Id = activityID
		err = model.UpdateBenefitActivityDraft(activity, c.GetInt("id"), common.GetTimestamp())
		updated = activity
	} else {
		updated, err = model.UpdateBenefitActivityMetadata(activityID, request.Name, request.Description, c.GetInt("id"), common.GetTimestamp())
	}
	if err != nil {
		benefitApiError(c, err)
		return
	}
	recordManageAudit(c, "benefit.activity.update", map[string]interface{}{"id": activityID})
	common.ApiSuccess(c, newBenefitActivityResponse(updated))
}

func PublishBenefitAdminActivity(c *gin.Context) {
	activityID, err := benefitPathID(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	activity, err := model.PublishBenefitActivity(activityID, c.GetInt("id"), common.GetTimestamp())
	if err != nil {
		benefitApiError(c, err)
		return
	}
	recordManageAudit(c, "benefit.activity.publish", map[string]interface{}{"id": activityID})
	common.ApiSuccess(c, newBenefitActivityResponse(activity))
}

func transitionBenefitAdminActivity(c *gin.Context, target string) {
	activityID, err := benefitPathID(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	activity, err := model.TransitionBenefitActivity(activityID, c.GetInt("id"), target, benefitNow())
	if err != nil {
		benefitApiError(c, err)
		return
	}
	recordManageAudit(c, "benefit.activity."+target, map[string]interface{}{"id": activityID})
	common.ApiSuccess(c, newBenefitActivityResponse(activity))
}

func PauseBenefitAdminActivity(c *gin.Context) {
	transitionBenefitAdminActivity(c, model.BenefitActivityStatusPaused)
}
func ResumeBenefitAdminActivity(c *gin.Context) {
	transitionBenefitAdminActivity(c, model.BenefitActivityStatusPublished)
}
func EndBenefitAdminActivity(c *gin.Context) {
	transitionBenefitAdminActivity(c, model.BenefitActivityStatusEnded)
}

func TerminateBenefitAdminActivity(c *gin.Context) {
	activityID, err := benefitPathID(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	var request benefitTerminateRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorMsg(c, "福利活动终止参数格式错误")
		return
	}
	if !request.Confirm {
		common.ApiErrorMsg(c, "强制作废需要二次确认")
		return
	}
	if err := model.TerminateBenefitActivity(activityID, c.GetInt("id"), request.Mode, request.Reason, benefitNow()); err != nil {
		benefitApiError(c, err)
		return
	}
	recordManageAudit(c, "benefit.activity.terminate", map[string]interface{}{"id": activityID, "mode": request.Mode, "reason": request.Reason})
	common.ApiSuccess(c, nil)
}

func GetBenefitAdminReport(c *gin.Context) {
	activityID, err := benefitPathID(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	report, err := model.GetBenefitActivityReport(activityID, common.GetTimestamp())
	if err != nil {
		benefitApiError(c, err)
		return
	}
	common.ApiSuccess(c, report)
}

func GetBenefitAdminVouchers(c *gin.Context) {
	activityID, err := benefitPathID(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	var vouchers []model.BenefitUserVoucher
	if err := model.DB.Where("activity_id = ?", activityID).Order("id DESC").Find(&vouchers).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, newBenefitVoucherResponses(vouchers))
}

func GetBenefitAdminVoucherLedger(c *gin.Context) {
	voucherID, err := benefitPathID(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	var ledger []model.BenefitVoucherLedger
	if err := model.DB.Where("voucher_id = ?", voucherID).Order("id ASC").Find(&ledger).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, ledger)
}

type benefitVoucherVoidRequest struct {
	Confirm bool   `json:"confirm"`
	Reason  string `json:"reason"`
}

func VoidBenefitAdminVoucher(c *gin.Context) {
	voucherID, err := benefitPathID(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	var request benefitVoucherVoidRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || !request.Confirm || strings.TrimSpace(request.Reason) == "" {
		common.ApiErrorMsg(c, "单券作废需要二次确认和原因")
		return
	}
	if err := model.VoidBenefitVoucher(voucherID, c.GetInt("id"), request.Reason, common.GetTimestamp()); err != nil {
		benefitApiError(c, err)
		return
	}
	recordManageAudit(c, "benefit.voucher.void", map[string]interface{}{"id": voucherID, "reason": request.Reason})
	common.ApiSuccess(c, nil)
}
