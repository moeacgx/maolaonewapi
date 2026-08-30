package controller

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type benefitActivityTransitionRequest struct {
	Now int64 `json:"now"`
}

type benefitTerminateRequest struct {
	Mode    string `json:"mode"`
	Confirm bool   `json:"confirm"`
	Reason  string `json:"reason"`
	Now     int64  `json:"now"`
}

func benefitNow(requested int64) int64 {
	if requested > 0 {
		return requested
	}
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
	common.ApiSuccess(c, activities)
}

func GetBenefitVouchers(c *gin.Context) {
	vouchers, err := model.ListBenefitVouchersForUser(c.GetInt("id"), common.GetTimestamp())
	if err != nil {
		benefitApiError(c, err)
		return
	}
	common.ApiSuccess(c, vouchers)
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
	common.ApiSuccess(c, voucher)
}

func GetBenefitAdminActivities(c *gin.Context) {
	page := common.GetPageQuery(c)
	activities, total, err := model.ListBenefitActivitiesForAdmin(page.GetStartIdx(), page.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	page.SetTotal(int(total))
	page.SetItems(activities)
	common.ApiSuccess(c, page)
}

func CreateBenefitAdminActivity(c *gin.Context) {
	var activity model.BenefitActivity
	if err := common.DecodeJson(c.Request.Body, &activity); err != nil {
		common.ApiErrorMsg(c, "福利活动参数格式错误")
		return
	}
	if err := model.CreateBenefitActivity(&activity, c.GetInt("id"), common.GetTimestamp()); err != nil {
		benefitApiError(c, err)
		return
	}
	recordManageAudit(c, "benefit.activity.create", map[string]interface{}{"id": activity.Id, "name": activity.Name})
	common.ApiSuccess(c, activity)
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
	common.ApiSuccess(c, activity)
}

func UpdateBenefitAdminActivity(c *gin.Context) {
	activityID, err := benefitPathID(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	var activity model.BenefitActivity
	if err := common.DecodeJson(c.Request.Body, &activity); err != nil {
		common.ApiErrorMsg(c, "福利活动参数格式错误")
		return
	}
	activity.Id = activityID
	var updated *model.BenefitActivity
	if stored, getErr := model.GetBenefitActivityForAdmin(activityID); getErr != nil {
		benefitApiError(c, getErr)
		return
	} else if stored.Status == model.BenefitActivityStatusDraft {
		err = model.UpdateBenefitActivityDraft(&activity, c.GetInt("id"), common.GetTimestamp())
		updated = &activity
	} else {
		updated, err = model.UpdateBenefitActivityMetadata(activityID, activity.Name, activity.Description, c.GetInt("id"), common.GetTimestamp())
	}
	if err != nil {
		benefitApiError(c, err)
		return
	}
	recordManageAudit(c, "benefit.activity.update", map[string]interface{}{"id": activityID})
	common.ApiSuccess(c, updated)
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
	common.ApiSuccess(c, activity)
}

func transitionBenefitAdminActivity(c *gin.Context, target string) {
	activityID, err := benefitPathID(c)
	if err != nil {
		common.ApiErrorMsg(c, err.Error())
		return
	}
	var request benefitActivityTransitionRequest
	if c.Request.Body != nil {
		_ = common.DecodeJson(c.Request.Body, &request)
	}
	activity, err := model.TransitionBenefitActivity(activityID, c.GetInt("id"), target, benefitNow(request.Now))
	if err != nil {
		benefitApiError(c, err)
		return
	}
	recordManageAudit(c, "benefit.activity."+target, map[string]interface{}{"id": activityID})
	common.ApiSuccess(c, activity)
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
	if err := model.TerminateBenefitActivity(activityID, c.GetInt("id"), request.Mode, request.Reason, benefitNow(request.Now)); err != nil {
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
	common.ApiSuccess(c, vouchers)
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
