package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupInvoiceOrderControllerTest(t *testing.T) {
	t.Helper()
	db := setupSubscriptionPaymentControllerTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.InvoiceRecord{}, &model.InvoiceOrderLink{}))

	originalEnabled := model.InvoiceEnabled
	originalRules := model.InvoiceFeeRules
	originalTypes := model.InvoiceTypes
	originalKinds := model.InvoiceKinds
	originalPrice := operation_setting.Price
	originalQuotaPerUnit := common.QuotaPerUnit
	model.InvoiceEnabled = true
	model.InvoiceFeeRules = `[{"min":0,"type":"percent","value":10}]`
	model.InvoiceTypes = `["personal","company"]`
	model.InvoiceKinds = `["normal","special"]`
	operation_setting.Price = 7
	common.QuotaPerUnit = 500000
	t.Cleanup(func() {
		model.InvoiceEnabled = originalEnabled
		model.InvoiceFeeRules = originalRules
		model.InvoiceTypes = originalTypes
		model.InvoiceKinds = originalKinds
		operation_setting.Price = originalPrice
		common.QuotaPerUnit = originalQuotaPerUnit
	})
}

func invoiceOrderControllerContext(t *testing.T, body string, userId int) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/user/invoice/request", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set("id", userId)
	return ctx, recorder
}

func TestApplyInvoiceOrdersUsesAuthenticatedUserAndForcesRequired(t *testing.T) {
	setupInvoiceOrderControllerTest(t)
	model.InvoiceFeeRules = `[{"min":0,"type":"fixed","value":0}]`
	require.NoError(t, model.DB.Create(&model.User{Id: 1201, Username: "invoice-controller", Quota: 1_000_000}).Error)
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId:          1201,
		TradeNo:         "TOP-CONTROLLER",
		Money:           70,
		ActualMoney:     70,
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		Status:          common.TopUpStatusSuccess,
		CreateTime:      now - 30,
		CompleteTime:    now - 20,
	}).Error)

	ctx, recorder := invoiceOrderControllerContext(t, `{
		"orders":[{"source_type":"topup","source_id":"TOP-CONTROLLER"}],
		"invoice":{"type":"company","kind":"special","title":"控制器测试公司","tax_no":"91310000CTRL"}
	}`, 1201)
	ApplyInvoiceOrders(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			SourceType  string  `json:"source_type"`
			BaseAmount  float64 `json:"base_amount"`
			FeeAmount   float64 `json:"fee_amount"`
			TotalAmount float64 `json:"total_amount"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.True(t, response.Success)
	assert.Equal(t, model.InvoiceSourceCombined, response.Data.SourceType)
	assert.Equal(t, 70.0, response.Data.BaseAmount)
	assert.Equal(t, 0.0, response.Data.FeeAmount)
	assert.Equal(t, 70.0, response.Data.TotalAmount)

	var saved model.TopUp
	require.NoError(t, model.DB.Where("trade_no = ?", "TOP-CONTROLLER").First(&saved).Error)
	assert.True(t, saved.InvoiceRequired)
	assert.Equal(t, model.InvoiceKindSpecial, saved.InvoiceKind)
}

func TestApplyInvoiceOrdersRejectsPositiveFeeBalancePayment(t *testing.T) {
	setupInvoiceOrderControllerTest(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 1205, Username: "invoice-balance-disabled", Quota: 1_000_000}).Error)
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId: 1205, TradeNo: "TOP-BALANCE-DISABLED", Money: 70, ActualMoney: 70,
		PaymentMethod: "alipay", PaymentProvider: model.PaymentProviderEpay,
		Status: common.TopUpStatusSuccess, CreateTime: now - 30, CompleteTime: now - 20,
	}).Error)

	ctx, recorder := invoiceOrderControllerContext(t, `{
		"orders":[{"source_type":"topup","source_id":"TOP-BALANCE-DISABLED"}],
		"invoice":{"type":"personal","kind":"normal","title":"余额禁用测试"}
	}`, 1205)
	ApplyInvoiceOrders(ctx)

	assert.Contains(t, recorder.Body.String(), "不支持余额支付")
	var invoiceCount int64
	require.NoError(t, model.DB.Model(&model.InvoiceRecord{}).Where("user_id = ?", 1205).Count(&invoiceCount).Error)
	assert.Zero(t, invoiceCount)
}

func TestPreviewInvoiceOrdersReturnsFrontendFieldNames(t *testing.T) {
	setupInvoiceOrderControllerTest(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 1202, Username: "invoice-preview", Quota: 1_000_000}).Error)
	now := time.Now().Unix()
	require.NoError(t, model.DB.Create(&model.TopUp{
		UserId:          1202,
		TradeNo:         "TOP-PREVIEW",
		Money:           70,
		ActualMoney:     70,
		PaymentMethod:   "alipay",
		PaymentProvider: model.PaymentProviderEpay,
		Status:          common.TopUpStatusSuccess,
		CreateTime:      now - 30,
		CompleteTime:    now - 20,
	}).Error)

	ctx, recorder := invoiceOrderControllerContext(t, `{
		"orders":[{"source_type":"topup","source_id":"TOP-PREVIEW"}],
		"invoice":{"required":true,"type":"personal"}
	}`, 1202)
	PreviewInvoiceOrders(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `"order_amount":70`)
	assert.Contains(t, recorder.Body.String(), `"invoice_fee":7`)
	assert.Contains(t, recorder.Body.String(), `"invoice_total_amount":77`)
}

func TestInvoiceListResponsesSeparateUserAndAdminRemarks(t *testing.T) {
	setupInvoiceOrderControllerTest(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 1203, Username: "invoice-response", Quota: 1_000_000}).Error)
	require.NoError(t, model.DB.Create(&model.InvoiceRecord{
		UserId:       1203,
		SourceType:   model.InvoiceSourceCombined,
		SourceId:     "INV-RESPONSE-SHAPE",
		Status:       model.InvoiceStatusClosed,
		AdminRemark:  "仅管理员可见",
		CancelReason: "用户可见的取消原因",
	}).Error)

	userRecorder := httptest.NewRecorder()
	userContext, _ := gin.CreateTestContext(userRecorder)
	userContext.Request = httptest.NewRequest(http.MethodGet, "/api/user/invoice?page=1&page_size=10", nil)
	userContext.Set("id", 1203)
	GetUserInvoices(userContext)
	require.Equal(t, http.StatusOK, userRecorder.Code)
	assert.NotContains(t, userRecorder.Body.String(), "admin_remark")
	assert.NotContains(t, userRecorder.Body.String(), "仅管理员可见")
	assert.Contains(t, userRecorder.Body.String(), `"cancel_reason":"用户可见的取消原因"`)

	adminRecorder := httptest.NewRecorder()
	adminContext, _ := gin.CreateTestContext(adminRecorder)
	adminContext.Request = httptest.NewRequest(http.MethodGet, "/api/admin/invoice?page=1&page_size=10", nil)
	AdminListInvoices(adminContext)
	require.Equal(t, http.StatusOK, adminRecorder.Code)
	assert.Contains(t, adminRecorder.Body.String(), `"admin_remark":"仅管理员可见"`)
	assert.Contains(t, adminRecorder.Body.String(), `"cancel_reason":"用户可见的取消原因"`)
}

func TestAdminManualRefundNoteSaveAndSerializationRoundTrip(t *testing.T) {
	setupInvoiceOrderControllerTest(t)
	require.NoError(t, model.DB.Create(&model.User{Id: 1204, Username: "invoice-refund-note", Quota: 1_000_000}).Error)
	record := model.InvoiceRecord{
		UserId:          1204,
		SourceType:      model.InvoiceSourceCombined,
		SourceId:        "INV-MANUAL-REFUND-NOTE",
		PaymentProvider: model.PaymentProviderEpay,
		PaymentStatus:   model.InvoicePaymentStatusManualRefundRequired,
		Status:          model.InvoiceStatusClosed,
		CancelReason:    "用户取消待支付申请",
	}
	require.NoError(t, model.DB.Create(&record).Error)

	for range 2 {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "/api/admin/invoice/"+jsonIntForTest(record.Id), bytes.NewBufferString(`{"status":"closed","admin_remark":"人工退款备注"}`))
		ctx.Request.Header.Set("Content-Type", "application/json")
		ctx.Params = gin.Params{{Key: "id", Value: jsonIntForTest(record.Id)}}
		AdminUpdateInvoice(ctx)
		require.Equal(t, http.StatusOK, recorder.Code)
		assert.Contains(t, recorder.Body.String(), `"success":true`)
	}

	var saved model.InvoiceRecord
	require.NoError(t, model.DB.First(&saved, record.Id).Error)
	assert.Equal(t, model.InvoicePaymentStatusManualRefundRequired, saved.PaymentStatus)
	assert.Equal(t, model.InvoiceStatusClosed, saved.Status)
	assert.Equal(t, "人工退款备注", saved.AdminRemark)
	assert.Equal(t, "用户取消待支付申请", saved.CancelReason)

	userRecorder := httptest.NewRecorder()
	userContext, _ := gin.CreateTestContext(userRecorder)
	userContext.Request = httptest.NewRequest(http.MethodGet, "/api/user/invoice?page=1&page_size=10", nil)
	userContext.Set("id", 1204)
	GetUserInvoices(userContext)
	require.Equal(t, http.StatusOK, userRecorder.Code)
	assert.NotContains(t, userRecorder.Body.String(), "admin_remark")
	assert.NotContains(t, userRecorder.Body.String(), "人工退款备注")
	assert.Contains(t, userRecorder.Body.String(), `"payment_status":"manual_refund_required"`)
	assert.Contains(t, userRecorder.Body.String(), `"cancel_reason":"用户取消待支付申请"`)

	adminRecorder := httptest.NewRecorder()
	adminContext, _ := gin.CreateTestContext(adminRecorder)
	adminContext.Request = httptest.NewRequest(http.MethodGet, "/api/admin/invoice?page=1&page_size=10", nil)
	AdminListInvoices(adminContext)
	require.Equal(t, http.StatusOK, adminRecorder.Code)
	assert.Contains(t, adminRecorder.Body.String(), `"payment_status":"manual_refund_required"`)
	assert.Contains(t, adminRecorder.Body.String(), `"admin_remark":"人工退款备注"`)
}
