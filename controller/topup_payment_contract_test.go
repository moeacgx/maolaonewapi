package controller

import (
	"crypto/md5"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

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
