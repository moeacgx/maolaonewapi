package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTopUpInvoiceControllerTest(t *testing.T) {
	t.Helper()

	initModelListColumnNames(t)
	db := setupSubscriptionPaymentControllerTestDB(t)
	require.NoError(t, db.Create(&model.User{
		Id:       901,
		Username: common.GetRandomString(8),
		Status:   common.UserStatusEnabled,
		Group:    "default",
		AffCode:  common.GetRandomString(8),
	}).Error)

	originalInvoiceEnabled := model.InvoiceEnabled
	originalInvoiceDiscountDisabled := model.InvoiceDiscountDisabled
	originalInvoiceTypes := model.InvoiceTypes
	originalInvoiceFeeRules := model.InvoiceFeeRules
	originalPrice := operation_setting.Price
	originalMinTopUp := operation_setting.MinTopUp
	originalPayMethods := operation_setting.PayMethods
	originalPayAddress := operation_setting.PayAddress
	originalEpayID := operation_setting.EpayId
	originalEpayKey := operation_setting.EpayKey

	model.InvoiceEnabled = true
	model.InvoiceDiscountDisabled = false
	model.InvoiceTypes = `["personal","company"]`
	model.InvoiceFeeRules = `[{"min":0,"max":500,"type":"fixed","value":50}]`
	operation_setting.Price = 1
	operation_setting.MinTopUp = 1
	operation_setting.PayMethods = []map[string]string{{"type": "alipay", "name": "支付宝"}}
	operation_setting.PayAddress = "https://pay.example.com"
	operation_setting.EpayId = "epay_id"
	operation_setting.EpayKey = "epay_key"

	t.Cleanup(func() {
		model.InvoiceEnabled = originalInvoiceEnabled
		model.InvoiceDiscountDisabled = originalInvoiceDiscountDisabled
		model.InvoiceTypes = originalInvoiceTypes
		model.InvoiceFeeRules = originalInvoiceFeeRules
		operation_setting.Price = originalPrice
		operation_setting.MinTopUp = originalMinTopUp
		operation_setting.PayMethods = originalPayMethods
		operation_setting.PayAddress = originalPayAddress
		operation_setting.EpayId = originalEpayID
		operation_setting.EpayKey = originalEpayKey
	})
}

func TestTopUpRequestAmount_PreviewsInvoiceFeeWithoutTitle(t *testing.T) {
	setupTopUpInvoiceControllerTest(t)

	ctx, recorder := newSubscriptionPaymentContext(t, AmountRequest{
		Amount: 100,
		Invoice: model.InvoiceRequest{
			Required: true,
			Type:     model.InvoiceTypePersonal,
		},
	}, 901)
	RequestAmount(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Message         string  `json:"message"`
		Data            string  `json:"data"`
		InvoiceRequired bool    `json:"invoice_required"`
		InvoiceFee      float64 `json:"invoice_fee"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "success", response.Message)
	assert.Equal(t, "150.00", response.Data)
	assert.True(t, response.InvoiceRequired)
	assert.InDelta(t, 50, response.InvoiceFee, 0.000001)

	var count int64
	require.NoError(t, model.DB.Model(&model.TopUp{}).Count(&count).Error)
	assert.EqualValues(t, 0, count)
}

func TestTopUpRequestEpay_RequiresInvoiceTitleForOrder(t *testing.T) {
	setupTopUpInvoiceControllerTest(t)

	ctx, recorder := newSubscriptionPaymentContext(t, EpayRequest{
		Amount:        100,
		PaymentMethod: "alipay",
		Invoice: model.InvoiceRequest{
			Required: true,
			Type:     model.InvoiceTypePersonal,
		},
	}, 901)
	RequestEpay(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), "请填写发票抬头")

	var count int64
	require.NoError(t, model.DB.Model(&model.TopUp{}).Count(&count).Error)
	assert.EqualValues(t, 0, count)
}

func TestTopUpRequestAmount_DisablesConfiguredDiscountsForInvoice(t *testing.T) {
	setupTopUpInvoiceControllerTest(t)

	originalDiscounts := operation_setting.GetPaymentSetting().AmountDiscount
	operation_setting.GetPaymentSetting().AmountDiscount = map[int]float64{100: 0.5}
	common.OptionMapRWMutex.Lock()
	if common.OptionMap == nil {
		common.OptionMap = make(map[string]string)
	}
	common.OptionMapRWMutex.Unlock()
	require.NoError(t, model.DB.AutoMigrate(&model.Option{}))
	require.NoError(t, model.UpdateOption("InvoiceDiscountDisabled", "true"))
	t.Cleanup(func() {
		operation_setting.GetPaymentSetting().AmountDiscount = originalDiscounts
	})

	ctx, recorder := newSubscriptionPaymentContext(t, AmountRequest{
		Amount:    100,
		PromoCode: "ignored-for-invoice",
		Invoice: model.InvoiceRequest{
			Required: true,
			Type:     model.InvoiceTypePersonal,
		},
	}, 901)
	RequestAmount(ctx)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Message         string                         `json:"message"`
		Data            string                         `json:"data"`
		Discount        *model.PromoCodeDiscountResult `json:"discount"`
		InvoiceRequired bool                           `json:"invoice_required"`
		InvoiceFee      float64                        `json:"invoice_fee"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	assert.Equal(t, "success", response.Message)
	assert.Equal(t, "150.00", response.Data)
	assert.Nil(t, response.Discount)
	assert.True(t, response.InvoiceRequired)
	assert.InDelta(t, 50, response.InvoiceFee, 0.000001)
}

func TestRetryStripePromotionCodesAllowedUsesOrderSnapshot(t *testing.T) {
	assert.True(t, retryStripePromotionCodesAllowed(&model.TopUp{}))
	assert.False(t, retryStripePromotionCodesAllowed(&model.TopUp{
		InvoiceRequired:         true,
		InvoiceDiscountDisabled: true,
	}))
	assert.False(t, retryStripePromotionCodesAllowed(nil))
}

func TestInvoicePaymentAmountsRejectFullDiscountOrder(t *testing.T) {
	_, err := buildInvoicePaymentAmounts(
		model.InvoiceRequest{Required: true, Type: model.InvoiceTypePersonal, Title: "测试个人"},
		model.PaymentProviderEpay,
		0,
	)
	require.ErrorContains(t, err, "零金额订单不能申请发票")
}
