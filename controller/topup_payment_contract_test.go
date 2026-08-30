package controller

import (
	"context"
	"crypto/md5"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/extension"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/stripe/stripe-go/v81"
)

func setupOkxAlipayRateModuleForOkpayTest(t *testing.T, serverURL string, optionOverrides map[string]string) *extension.Manager {
	t.Helper()

	fetchSetting := system_setting.GetFetchSetting()
	originalFetchSetting := *fetchSetting
	fetchSetting.EnableSSRFProtection = false
	t.Cleanup(func() {
		*fetchSetting = originalFetchSetting
	})
	if service.GetSSRFProtectedHTTPClient() == nil {
		service.InitHttpClient()
	}

	rootDir := t.TempDir()
	moduleDir := filepath.Join(rootDir, extension.OkxAlipayRateModuleID)
	require.NoError(t, os.MkdirAll(moduleDir, 0o755))
	manifest := extension.Manifest{
		ID:      extension.OkxAlipayRateModuleID,
		Name:    "OKX 支付宝汇率",
		Version: "0.3.0",
		Runtime: extension.Runtime{Type: extension.RuntimeTypeStatic, StaticDir: "public"},
		Permissions: extension.PermissionConfig{
			Roles: []string{"root"},
		},
	}
	manifestBytes, err := common.Marshal(manifest)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "manifest.json"), manifestBytes, 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(moduleDir, "public"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "public", "index.html"), []byte("ok"), 0o644))
	manager := extension.NewManager(rootDir)
	require.NoError(t, manager.Scan())
	_, err = manager.SetEnabled(extension.OkxAlipayRateModuleID, true)
	require.NoError(t, err)

	originalManager := extension.DefaultManager
	extension.DefaultManager = manager
	t.Cleanup(func() {
		extension.DefaultManager = originalManager
	})

	previousOptions := map[string]string{}
	optionExisted := map[string]bool{}
	optionKeys := []string{
		extension.OkxAlipayRateOptionRateAPIURL,
		extension.OkxAlipayRateOptionSide,
		extension.OkxAlipayRateOptionTier,
		extension.OkxAlipayRateOptionAdjustmentType,
		extension.OkxAlipayRateOptionAdjustmentValue,
	}
	options := map[string]string{
		extension.OkxAlipayRateOptionRateAPIURL:      serverURL,
		extension.OkxAlipayRateOptionSide:            "buy",
		extension.OkxAlipayRateOptionTier:            "3",
		extension.OkxAlipayRateOptionAdjustmentType:  extension.OkxAlipayRateAdjustmentTypeAbsolute,
		extension.OkxAlipayRateOptionAdjustmentValue: "-0.2",
	}
	for key, value := range optionOverrides {
		options[key] = value
	}
	common.OptionMapRWMutex.Lock()
	optionMapWasNil := common.OptionMap == nil
	if optionMapWasNil {
		common.OptionMap = map[string]string{}
	}
	for _, key := range optionKeys {
		previousOptions[key], optionExisted[key] = common.OptionMap[key]
	}
	for key, value := range options {
		common.OptionMap[key] = value
	}
	common.OptionMapRWMutex.Unlock()
	t.Cleanup(func() {
		common.OptionMapRWMutex.Lock()
		defer common.OptionMapRWMutex.Unlock()
		if optionMapWasNil {
			common.OptionMap = nil
			return
		}
		for _, key := range optionKeys {
			if optionExisted[key] {
				common.OptionMap[key] = previousOptions[key]
			} else {
				delete(common.OptionMap, key)
			}
		}
	})

	return manager
}

func TestStripeFulfillOrderAcceptsDiscountedCheckoutSubtotal(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	user := model.User{Id: 1301, Username: "stripe-discount", Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, db.Create(&user).Error)
	topUp := model.TopUp{
		UserId:          user.Id,
		Amount:          10,
		Money:           10,
		CreditedQuota:   777,
		TradeNo:         "STRIPE_DISCOUNT_SUBTOTAL",
		PaymentMethod:   model.PaymentMethodStripe,
		PaymentProvider: model.PaymentProviderStripe,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())
	attempt, err := model.CreateTopUpPaymentAttempt(topUp.TradeNo, model.PaymentProviderStripe, model.PaymentMethodStripe, "1000", "USD")
	require.NoError(t, err)
	require.NoError(t, model.MarkTopUpPaymentAttemptLaunched(attempt.Id, "cs_discounted"))

	event := stripe.Event{Data: &stripe.EventData{Object: map[string]interface{}{
		"id":                  "cs_discounted",
		"amount_total":        "900",
		"amount_subtotal":     "1000",
		"currency":            "usd",
		"client_reference_id": topUp.TradeNo,
		"customer":            "cus_discounted",
	}}}

	fulfillOrder(context.Background(), event, topUp.TradeNo, "cus_discounted", "203.0.113.5")

	var savedTopUp model.TopUp
	require.NoError(t, db.Where("trade_no = ?", topUp.TradeNo).First(&savedTopUp).Error)
	require.Equal(t, common.TopUpStatusSuccess, savedTopUp.Status)
	require.Equal(t, "cs_discounted", savedTopUp.ProviderOrderId)
	var savedUser model.User
	require.NoError(t, db.First(&savedUser, user.Id).Error)
	require.Equal(t, int64(777), savedUser.Quota)
}

func TestBepusdtJSONCallbackPreservesNumericTextForSignature(t *testing.T) {
	token := "callback-secret"
	params := map[string]string{
		"trade_id": "provider-1", "order_id": "local-1", "amount": "10.00",
		"actual_amount": "1.23450000", "status": "2",
	}
	body := fmt.Sprintf(`{"trade_id":"provider-1","order_id":"local-1","amount":10.00,"actual_amount":1.23450000,"status":2,"signature":"%s"}`, generateBepusdtSignature(params, token))
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/bepusdt/notify", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	parsed, err := parseBepusdtNotifyPayload(ctx)

	require.NoError(t, err)
	require.Equal(t, "10.00", parsed.Params["amount"])
	require.Equal(t, "1.23450000", parsed.Params["actual_amount"])
	require.True(t, verifyBepusdtNotifyParamsSignature(parsed.Params, token))
}

func TestOkpayCallbackMatchesDocumentFieldOrderAndRawNumbers(t *testing.T) {
	body := `{"code":200,"data":{"order_id":"ac7b86615fdb137576ae35879f7ed844","unique_id":"BWIN-20250922152023LDVNSyxLQko","pay_user_id":7238234930,"amount":"6.00000000","coin":"USDT","status":1,"type":"deposit"},"id":1,"status":"success","sign":"95BE540FB7D1996770E2B4CDBC6F184D"}`
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/okpay/notify", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	values, raw, err := parseOkpayCallbackValues(ctx)

	require.NoError(t, err)
	require.Equal(t, "7238234930", values.Get("data[pay_user_id]"))
	require.Equal(t, "6.00000000", values.Get("data[amount]"))
	require.True(t, verifyOkpayCallbackSignature(values, "123456", parseOkpayCallbackOrderedPairs(raw)))
}

func TestOkpayRawOrderSignatureRejectsUnsignedMergedField(t *testing.T) {
	const token = "123456"
	const raw = "status=success&code=200&data[unique_id]=trade-no&id=1"
	digest := md5.Sum([]byte(raw + "&token=" + token))
	values, err := url.ParseQuery(raw + "&sign=" + strings.ToUpper(fmt.Sprintf("%x", digest)))
	require.NoError(t, err)
	values.Set("data[amount]", "10.00000000")

	require.False(t, verifyOkpayCallbackSignature(values, token, parseOkpayCallbackOrderedPairs([]byte(raw))))
}

func TestBepusdtTradeTypeMustBeConfigured(t *testing.T) {
	original := setting.BepusdtChains
	t.Cleanup(func() { setting.BepusdtChains = original })
	setting.BepusdtChains = `[{"name":"TRC20","trade_type":"usdt.trc20"}]`
	require.True(t, isValidBepusdtTradeType("usdt.trc20"))
	require.False(t, isValidBepusdtTradeType("usdt.bep20"))
}

func TestOkpayRateQuoteUsesConfiguredOkxTierAndAdjustment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"buy":[{"price":"6.70"},{"price":"6.80"},{"price":"6.90"}]}}`))
	}))
	t.Cleanup(server.Close)
	originalSource, originalURL, originalSide := setting.OkpayRateSource, setting.OkpayRateApiUrl, setting.OkpayOkxSide
	originalTier, originalType, originalAdjustment := setting.OkpayOkxTier, setting.OkpayRateAdjustmentType, setting.OkpayRateAdjustmentValue
	t.Cleanup(func() {
		setting.OkpayRateSource, setting.OkpayRateApiUrl, setting.OkpayOkxSide = originalSource, originalURL, originalSide
		setting.OkpayOkxTier, setting.OkpayRateAdjustmentType, setting.OkpayRateAdjustmentValue = originalTier, originalType, originalAdjustment
	})
	setting.OkpayRateSource, setting.OkpayRateApiUrl, setting.OkpayOkxSide = okpayRateSourceOkxAlipayTier, server.URL, "buy"
	setting.OkpayOkxTier, setting.OkpayRateAdjustmentType, setting.OkpayRateAdjustmentValue = 2, okpayAdjustmentTypeAbsolute, -0.2

	quote, err := fetchOkpayUsdtCnyRateQuote()
	require.NoError(t, err)
	require.InDelta(t, 6.8, quote.RawRate, 0.000001)
	require.InDelta(t, 6.6, quote.AdjustedRate, 0.000001)
	require.Equal(t, 2, quote.Tier)
}

func TestOkpayRateQuoteUsesOkxAlipayRateModuleConfig(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"buy":[{"price":"6.70"},{"price":"6.80"},{"price":"6.90"}]}}`))
	}))
	t.Cleanup(server.Close)
	setupOkxAlipayRateModuleForOkpayTest(t, server.URL, nil)
	originalSource := setting.OkpayRateSource
	originalURL := setting.OkpayRateApiUrl
	originalTier := setting.OkpayOkxTier
	originalAdjustment := setting.OkpayRateAdjustmentValue
	t.Cleanup(func() {
		setting.OkpayRateSource = originalSource
		setting.OkpayRateApiUrl = originalURL
		setting.OkpayOkxTier = originalTier
		setting.OkpayRateAdjustmentValue = originalAdjustment
	})
	setting.OkpayRateSource = extension.OkxAlipayRateSourceID
	setting.OkpayRateApiUrl = "https://api.coingecko.example/unused"
	setting.OkpayOkxTier = 1
	setting.OkpayRateAdjustmentValue = 9.9

	quote, err := fetchOkpayUsdtCnyRateQuote()
	require.NoError(t, err)
	require.Equal(t, extension.OkxAlipayRateSourceID, quote.Source)
	require.InDelta(t, 6.9, quote.RawRate, 0.000001)
	require.InDelta(t, 6.7, quote.AdjustedRate, 0.000001)
	require.Equal(t, 3, quote.Tier)
}

func TestOkpayRateCacheRespectsOkxAlipayModuleDisabledState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"buy":[{"price":"6.70"},{"price":"6.80"},{"price":"6.90"}]}}`))
	}))
	t.Cleanup(server.Close)
	manager := setupOkxAlipayRateModuleForOkpayTest(t, server.URL, map[string]string{
		extension.OkxAlipayRateOptionTier:            "2",
		extension.OkxAlipayRateOptionAdjustmentValue: "0",
	})

	originalSource := setting.OkpayRateSource
	originalAutoExchange := setting.OkpayAutoExchangeEnabled
	originalUsdtCnyRate := setting.OkpayUsdtCnyRate
	originalExchangeRate := setting.OkpayExchangeRate
	t.Cleanup(func() {
		setting.OkpayRateSource = originalSource
		setting.OkpayAutoExchangeEnabled = originalAutoExchange
		setting.OkpayUsdtCnyRate = originalUsdtCnyRate
		setting.OkpayExchangeRate = originalExchangeRate
		resetOkpayRateCacheForTest()
	})
	resetOkpayRateCacheForTest()
	setting.OkpayRateSource = extension.OkxAlipayRateSourceID
	setting.OkpayAutoExchangeEnabled = true
	setting.OkpayUsdtCnyRate = 7.77
	setting.OkpayExchangeRate = 7.66

	rate, source, failed := getOkpayUsdtCnyRate()
	require.False(t, failed)
	require.Equal(t, extension.OkxAlipayRateSourceID, source)
	require.InDelta(t, 6.8, rate, 0.000001)

	_, err := manager.SetEnabled(extension.OkxAlipayRateModuleID, false)
	require.NoError(t, err)

	rate, source, failed = getOkpayUsdtCnyRate()
	require.True(t, failed)
	require.Equal(t, "fallback", source)
	require.InDelta(t, 7.77, rate, 0.000001)
}

func TestCreemTopUpSnapshotRejectsUnderpaymentAndCheckoutMismatch(t *testing.T) {
	attempt := &model.TopUpPaymentAttempt{
		Id: 1, PaymentProvider: model.PaymentProviderCreem,
		ProviderOrderId: "checkout-1", ProviderAmount: "1234", ProviderCurrency: "USD",
	}
	require.NoError(t, model.ValidateTopUpPaymentAttemptSnapshot(attempt, model.PaymentProviderCreem, "checkout-1", "1234", "USD", decimal.Zero))
	require.ErrorIs(t, model.ValidateTopUpPaymentAttemptSnapshot(attempt, model.PaymentProviderCreem, "checkout-1", "1233", "USD", decimal.Zero), model.ErrTopUpPaymentAttemptMismatch)
	require.ErrorIs(t, model.ValidateTopUpPaymentAttemptSnapshot(attempt, model.PaymentProviderCreem, "checkout-2", "1234", "USD", decimal.Zero), model.ErrTopUpPaymentAttemptMismatch)
}

func TestWaffoTopUpSnapshotRejectsUnderpaymentAndAcquiringOrderMismatch(t *testing.T) {
	attempt := &model.TopUpPaymentAttempt{
		Id: 2, PaymentProvider: model.PaymentProviderWaffo,
		ProviderOrderId: "acquiring-1", ProviderAmount: "12.34", ProviderCurrency: "USD",
	}
	require.NoError(t, model.ValidateTopUpPaymentAttemptSnapshot(attempt, model.PaymentProviderWaffo, "acquiring-1", "12.34", "usd", decimal.Zero))
	require.ErrorIs(t, model.ValidateTopUpPaymentAttemptSnapshot(attempt, model.PaymentProviderWaffo, "acquiring-1", "12.33", "USD", decimal.Zero), model.ErrTopUpPaymentAttemptMismatch)
	require.ErrorIs(t, model.ValidateTopUpPaymentAttemptSnapshot(attempt, model.PaymentProviderWaffo, "acquiring-2", "12.34", "USD", decimal.Zero), model.ErrTopUpPaymentAttemptMismatch)
}

func TestBepusdtLegacyTopUpPersistsProviderAuditSnapshot(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	user := model.User{Id: 1201, Username: "bep-legacy-audit", Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, db.Create(&user).Error)
	topUp := model.TopUp{
		UserId:          user.Id,
		Amount:          1,
		Money:           7.2,
		TradeNo:         "BEP_LEGACY_AUDIT",
		PaymentMethod:   model.PaymentMethodBepusdt,
		PaymentProvider: model.PaymentProviderBepusdt,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/bepusdt/notify", nil)
	handleBepusdtPaymentSuccess(ctx, &bepusdtNotifyPayload{
		OrderId: topUp.TradeNo,
		TradeId: "bep-provider-audit",
		Amount:  "7.20",
		Status:  2,
	})

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "ok", recorder.Body.String())
	require.NoError(t, db.First(&topUp, topUp.Id).Error)
	require.Equal(t, common.TopUpStatusSuccess, topUp.Status)
	require.Equal(t, "bep-provider-audit", topUp.ProviderOrderId)
	require.Equal(t, "7.20", topUp.ProviderAmount)
	require.Equal(t, "CNY", topUp.ProviderCurrency)
}

func TestBepusdtLegacyTopUpCannotBypassExistingAttempt(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	user := model.User{Id: 1202, Username: "bep-attempt-guard", Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, db.Create(&user).Error)
	topUp := model.TopUp{
		UserId:          user.Id,
		Amount:          1,
		Money:           7.2,
		TradeNo:         "BEP_ATTEMPT_GUARD",
		PaymentMethod:   model.PaymentMethodBepusdt,
		PaymentProvider: model.PaymentProviderBepusdt,
		CreateTime:      common.GetTimestamp(),
		Status:          common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())
	attempt, err := model.CreateTopUpPaymentAttempt(topUp.TradeNo, model.PaymentProviderBepusdt, model.PaymentMethodBepusdt, "7.20", "CNY")
	require.NoError(t, err)
	require.NoError(t, model.MarkTopUpPaymentAttemptLaunched(attempt.Id, "bep-provider-expected"))

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/bepusdt/notify", nil)
	handleBepusdtPaymentSuccess(ctx, &bepusdtNotifyPayload{
		OrderId: topUp.TradeNo,
		TradeId: "bep-provider-mismatch",
		Amount:  "7.20",
		Status:  2,
	})

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.NoError(t, db.First(&topUp, topUp.Id).Error)
	require.Equal(t, common.TopUpStatusPending, topUp.Status)
	require.Empty(t, topUp.ProviderOrderId)
	require.NoError(t, db.First(attempt, attempt.Id).Error)
	require.Equal(t, model.TopUpPaymentAttemptLaunched, attempt.Status)
}

func TestBepusdtLegacyTopUpRejectsMismatchedProviderOrder(t *testing.T) {
	db := setupSubscriptionPaymentControllerTestDB(t)
	user := model.User{Id: 1203, Username: "bep-provider-guard", Status: common.UserStatusEnabled, Group: "default"}
	require.NoError(t, db.Create(&user).Error)
	topUp := model.TopUp{
		UserId:           user.Id,
		Amount:           1,
		Money:            7.2,
		TradeNo:          "BEP_PROVIDER_GUARD",
		PaymentMethod:    model.PaymentMethodBepusdt,
		PaymentProvider:  model.PaymentProviderBepusdt,
		ProviderOrderId:  "bep-provider-original",
		ProviderAmount:   "7.20",
		ProviderCurrency: "CNY",
		CreateTime:       common.GetTimestamp(),
		Status:           common.TopUpStatusPending,
	}
	require.NoError(t, topUp.Insert())

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/bepusdt/notify", nil)
	handleBepusdtPaymentSuccess(ctx, &bepusdtNotifyPayload{
		OrderId: topUp.TradeNo,
		TradeId: "bep-provider-other",
		Amount:  "7.20",
		Status:  2,
	})

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.NoError(t, db.First(&topUp, topUp.Id).Error)
	require.Equal(t, common.TopUpStatusPending, topUp.Status)
	require.Equal(t, "bep-provider-original", topUp.ProviderOrderId)
}
