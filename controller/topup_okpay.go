package controller

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/extension"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/tidwall/gjson"
)

type OkpayPayRequest struct {
	Amount    int64                `json:"amount"`
	PromoCode string               `json:"promo_code"`
	Invoice   model.InvoiceRequest `json:"invoice"`
}

type OkpayAmountRequest struct {
	Amount    int64                `json:"amount"`
	PromoCode string               `json:"promo_code"`
	Invoice   model.InvoiceRequest `json:"invoice"`
}

type okpayPaymentAmount struct {
	FiatAmount     float64
	CoinAmount     float64
	Rate           float64
	RateSource     string
	AutoRateFailed bool
	Coin           string
}

type okpayRateQuote struct {
	RawRate        float64 `json:"raw_rate"`
	AdjustedRate   float64 `json:"adjusted_rate"`
	Source         string  `json:"source"`
	Tier           int     `json:"tier,omitempty"`
	Side           string  `json:"side,omitempty"`
	Adjustment     float64 `json:"adjustment"`
	AdjustmentType string  `json:"adjustment_type"`
}

type okpayPaymentLinkResult struct {
	ProviderOrderId string
	PaymentUrl      string
	Amount          string
	PaymentAmount   okpayPaymentAmount
}

type okpayRateCacheEntry struct {
	rate      float64
	source    string
	configKey string
	expiresAt time.Time
}

type okpaySignPair struct {
	Key   string
	Value string
}

var (
	okpayRateCacheMu sync.Mutex
	okpayRateCache   okpayRateCacheEntry
)

const (
	okpayRateCacheTTL              = 5 * time.Minute
	okpayRateSourceCoinGecko       = "coingecko"
	okpayRateSourceOkxAlipayTier   = "okx-alipay-tier"
	okpayRateSourceOkxAlipayModule = extension.OkxAlipayRateSourceID
	okpayAdjustmentTypeAbsolute    = "absolute"
	okpayAdjustmentTypePercent     = "percent"
	okpayDefaultCoinGeckoRateUrl   = "https://api.coingecko.com/api/v3/simple/price?ids=tether&vs_currencies=cny&include_last_updated_at=true"
)

var okpayCallbackSignatureOrder = []string{
	"code", "data[order_id]", "data[unique_id]", "data[pay_user_id]", "data[amount]",
	"data[coin]", "data[status]", "data[type]", "id", "status",
}

func generateOkpaySignature(params map[string]string, merchantToken string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" && !strings.EqualFold(key, "sign") {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	pairs := make([]okpaySignPair, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, okpaySignPair{Key: key, Value: params[key]})
	}
	return generateOkpaySignatureFromPairs(pairs, merchantToken)
}

func generateOkpaySignatureFromPairs(pairs []okpaySignPair, merchantToken string) string {
	parts := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		key := strings.TrimSpace(pair.Key)
		value := strings.TrimSpace(pair.Value)
		if key != "" && value != "" && !strings.EqualFold(key, "sign") {
			parts = append(parts, key+"="+value)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	digest := md5.Sum([]byte(strings.Join(parts, "&") + "&token=" + strings.TrimSpace(merchantToken)))
	return strings.ToUpper(fmt.Sprintf("%x", digest))
}

func generateOkpayCallbackOrderedSignature(values url.Values, token string) string {
	pairs := make([]okpaySignPair, 0, len(okpayCallbackSignatureOrder))
	for _, key := range okpayCallbackSignatureOrder {
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			pairs = append(pairs, okpaySignPair{Key: key, Value: value})
		}
	}
	return generateOkpaySignatureFromPairs(pairs, token)
}

func okpayOrderedPairsCoverValues(pairs []okpaySignPair, values url.Values) bool {
	covered := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		if key, value := strings.TrimSpace(pair.Key), strings.TrimSpace(pair.Value); key != "" && value != "" && !strings.EqualFold(key, "sign") {
			covered[key] = value
		}
	}
	for key := range values {
		value := strings.TrimSpace(values.Get(key))
		if value != "" && !strings.EqualFold(key, "sign") && covered[key] != value {
			return false
		}
	}
	return true
}

func verifyOkpayCallbackSignature(values url.Values, token string, rawPairs ...[]okpaySignPair) bool {
	actual := strings.TrimSpace(values.Get("sign"))
	if actual == "" {
		return false
	}
	if expected := generateOkpayCallbackOrderedSignature(values, token); expected != "" && strings.EqualFold(expected, actual) {
		return true
	}
	if len(rawPairs) != 0 && okpayOrderedPairsCoverValues(rawPairs[0], values) {
		if expected := generateOkpaySignatureFromPairs(rawPairs[0], token); expected != "" && strings.EqualFold(expected, actual) {
			return true
		}
	}
	params := make(map[string]string, len(values))
	for key := range values {
		if value := strings.TrimSpace(values.Get(key)); value != "" && !strings.EqualFold(key, "sign") {
			params[key] = value
		}
	}
	return strings.EqualFold(generateOkpaySignature(params, token), actual)
}

func parseOkpayCallbackOrderedPairs(body []byte) []okpaySignPair {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil
	}
	pairs := make([]okpaySignPair, 0)
	if strings.HasPrefix(trimmed, "{") {
		if !gjson.Valid(trimmed) {
			return nil
		}
		root := gjson.Parse(trimmed)
		if !root.IsObject() {
			return nil
		}
		flattenOkpayJSONSignPairs("", root, &pairs)
		return pairs
	}
	for _, item := range strings.Split(trimmed, "&") {
		parts := strings.SplitN(item, "=", 2)
		key, err := url.QueryUnescape(parts[0])
		if err != nil {
			key = parts[0]
		}
		value := ""
		if len(parts) == 2 {
			value, err = url.QueryUnescape(parts[1])
			if err != nil {
				value = parts[1]
			}
		}
		pairs = append(pairs, okpaySignPair{Key: key, Value: value})
	}
	return pairs
}

func flattenOkpayJSONSignPairs(prefix string, value gjson.Result, pairs *[]okpaySignPair) {
	if value.IsObject() {
		value.ForEach(func(key, child gjson.Result) bool {
			childKey := key.String()
			if prefix != "" {
				childKey = prefix + "[" + childKey + "]"
			}
			flattenOkpayJSONSignPairs(childKey, child, pairs)
			return true
		})
		return
	}
	if value.IsArray() {
		index := 0
		value.ForEach(func(_, child gjson.Result) bool {
			flattenOkpayJSONSignPairs(fmt.Sprintf("%s[%d]", prefix, index), child, pairs)
			index++
			return true
		})
		return
	}
	*pairs = append(*pairs, okpaySignPair{Key: prefix, Value: value.String()})
}

func mergeOkpayCallbackValues(dst, src url.Values) {
	for key, items := range src {
		if strings.TrimSpace(key) != "" && len(items) != 0 {
			dst[key] = append([]string(nil), items...)
		}
	}
}

func parseOkpayJSONCallbackValues(body []byte) (url.Values, error) {
	var payload map[string]json.RawMessage
	if err := common.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	values := url.Values{}
	for key, raw := range payload {
		if key == "data" && common.GetJsonType(raw) == "object" {
			var data map[string]json.RawMessage
			if err := common.Unmarshal(raw, &data); err != nil {
				return nil, err
			}
			for dataKey, dataRaw := range data {
				if text := strings.TrimSpace(common.JsonRawMessageToString(dataRaw)); text != "" {
					values.Set("data["+dataKey+"]", text)
				}
			}
			continue
		}
		if text := strings.TrimSpace(common.JsonRawMessageToString(raw)); text != "" {
			values.Set(key, text)
		}
	}
	return values, nil
}

func parseOkpayCallbackValues(c *gin.Context) (url.Values, []byte, error) {
	values := url.Values{}
	mergeOkpayCallbackValues(values, c.Request.URL.Query())
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil, nil, err
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if strings.Contains(contentType, "multipart/form-data") {
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			return nil, body, err
		}
		mergeOkpayCallbackValues(values, c.Request.PostForm)
		if c.Request.MultipartForm != nil {
			mergeOkpayCallbackValues(values, c.Request.MultipartForm.Value)
		}
		return values, body, nil
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return values, body, nil
	}
	if trimmed[0] == '{' {
		jsonValues, err := parseOkpayJSONCallbackValues(trimmed)
		if err != nil {
			return nil, body, err
		}
		mergeOkpayCallbackValues(values, jsonValues)
		return values, body, nil
	}
	form, err := url.ParseQuery(string(trimmed))
	if err != nil {
		return nil, body, err
	}
	mergeOkpayCallbackValues(values, form)
	return values, body, nil
}

func getOkpayCallbackValue(values url.Values, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(values.Get(key)); value != "" {
			return value
		}
	}
	return ""
}

func isOkpayCallbackSuccess(requestStatus, paymentStatus string) bool {
	requestStatus = strings.TrimSpace(requestStatus)
	paymentStatus = strings.TrimSpace(paymentStatus)
	requestOK := requestStatus == "" || requestStatus == "1" || strings.EqualFold(requestStatus, "success")
	if paymentStatus == "" {
		return requestStatus == "1" || strings.EqualFold(requestStatus, "success")
	}
	return requestOK && (paymentStatus == "1" || strings.EqualFold(paymentStatus, "success") || strings.EqualFold(paymentStatus, "paid"))
}

func getOkpayFiatPayMoney(amount int64, group string) float64 {
	displayAmount := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		displayAmount = displayAmount.Div(decimal.NewFromFloat(common.QuotaPerUnit))
	}
	groupRatio := common.GetTopupGroupRatio(group)
	if groupRatio == 0 {
		groupRatio = 1
	}
	discount := topUpAmountDiscount(amount, model.InvoiceRequest{})
	return displayAmount.Mul(decimal.NewFromFloat(setting.OkpayExchangeRate)).Mul(decimal.NewFromFloat(groupRatio)).Mul(decimal.NewFromFloat(discount)).InexactFloat64()
}

func getOkpayFiatPayMoneyWithInvoice(amount int64, group string, invoice model.InvoiceRequest) float64 {
	displayAmount := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		displayAmount = displayAmount.Div(decimal.NewFromFloat(common.QuotaPerUnit))
	}
	groupRatio := common.GetTopupGroupRatio(group)
	if groupRatio == 0 {
		groupRatio = 1
	}
	return displayAmount.Mul(decimal.NewFromFloat(setting.OkpayExchangeRate)).Mul(decimal.NewFromFloat(groupRatio)).Mul(decimal.NewFromFloat(topUpAmountDiscount(amount, invoice))).InexactFloat64()
}

func getOkpayPayMoney(amount int64, group string) float64 {
	return getOkpayFiatPayMoney(amount, group)
}

func getOkpayCoin() string {
	coin := strings.ToUpper(strings.TrimSpace(setting.OkpayCoin))
	if coin == "" {
		return "USDT"
	}
	return coin
}

func getOkpayFallbackUsdtCnyRate() float64 {
	if setting.OkpayUsdtCnyRate > 0 && !math.IsNaN(setting.OkpayUsdtCnyRate) && !math.IsInf(setting.OkpayUsdtCnyRate, 0) {
		return setting.OkpayUsdtCnyRate
	}
	if setting.OkpayExchangeRate > 0 && !math.IsNaN(setting.OkpayExchangeRate) && !math.IsInf(setting.OkpayExchangeRate, 0) {
		return setting.OkpayExchangeRate
	}
	return 1
}

func normalizeOkpayRateSource() string {
	source := strings.TrimSpace(setting.OkpayRateSource)
	if strings.EqualFold(source, okpayRateSourceOkxAlipayModule) {
		return okpayRateSourceOkxAlipayModule
	}
	if strings.EqualFold(source, okpayRateSourceOkxAlipayTier) {
		return okpayRateSourceOkxAlipayTier
	}
	return okpayRateSourceCoinGecko
}

func normalizeOkpayAdjustmentType() string {
	if strings.EqualFold(strings.TrimSpace(setting.OkpayRateAdjustmentType), okpayAdjustmentTypePercent) {
		return okpayAdjustmentTypePercent
	}
	return okpayAdjustmentTypeAbsolute
}

func okpayRateCacheKey() string {
	source := normalizeOkpayRateSource()
	if source == okpayRateSourceOkxAlipayModule {
		return source + "|" + extension.OkxAlipayRateConfigCacheKey()
	}
	return fmt.Sprintf("%s|%s|%s|%d|%s|%.8f", source, setting.OkpayRateApiUrl, setting.OkpayOkxSide, setting.OkpayOkxTier, normalizeOkpayAdjustmentType(), setting.OkpayRateAdjustmentValue)
}

func applyOkpayRateAdjustment(rate float64) (float64, error) {
	if rate <= 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return 0, errors.New("invalid raw rate")
	}
	adjusted := decimal.NewFromFloat(rate)
	if normalizeOkpayAdjustmentType() == okpayAdjustmentTypePercent {
		adjusted = adjusted.Mul(decimal.NewFromInt(100).Add(decimal.NewFromFloat(setting.OkpayRateAdjustmentValue))).Div(decimal.NewFromInt(100))
	} else {
		adjusted = adjusted.Add(decimal.NewFromFloat(setting.OkpayRateAdjustmentValue))
	}
	value := adjusted.InexactFloat64()
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("adjusted rate must be positive")
	}
	return value, nil
}

func parseOkpayRateFromBody(body []byte) (float64, error) {
	result := gjson.GetBytes(body, "tether.cny")
	if !result.Exists() || result.Float() <= 0 {
		return 0, errors.New("missing tether cny price")
	}
	return result.Float(), nil
}

func parseOkpayOkxAlipayTierRateFromBody(body []byte, side string, tier int) (float64, error) {
	if tier <= 0 {
		tier = 3
	}
	path := fmt.Sprintf("data.%s.%d.price", side, tier-1)
	result := gjson.GetBytes(body, path)
	if !result.Exists() || result.Float() <= 0 {
		return 0, errors.New("missing okx tier price")
	}
	return result.Float(), nil
}

func fetchOkpayUsdtCnyRateQuote() (okpayRateQuote, error) {
	source := normalizeOkpayRateSource()
	if source == okpayRateSourceOkxAlipayModule {
		quote, err := extension.FetchEnabledOkxAlipayRateQuote()
		if err != nil {
			return okpayRateQuote{}, err
		}
		return okpayRateQuote{
			RawRate:        quote.RawRate,
			AdjustedRate:   quote.AdjustedRate,
			Source:         quote.Source,
			Tier:           quote.Tier,
			Side:           quote.Side,
			Adjustment:     quote.AdjustmentValue,
			AdjustmentType: quote.AdjustmentType,
		}, nil
	}
	rateURL := strings.TrimSpace(setting.OkpayRateApiUrl)
	side := ""
	tier := 0
	if source == okpayRateSourceOkxAlipayTier {
		side = strings.ToLower(strings.TrimSpace(setting.OkpayOkxSide))
		if side != "sell" {
			side = "buy"
		}
		tier = setting.OkpayOkxTier
		if tier <= 0 {
			tier = 3
		}
		if rateURL == "" || rateURL == okpayDefaultCoinGeckoRateUrl {
			rateURL = fmt.Sprintf("https://www.okx.com/v3/c2c/tradingOrders/books?quoteCurrency=CNY&baseCurrency=USDT&side=%s&paymentMethod=aliPay", side)
		}
	}
	if rateURL == "" {
		return okpayRateQuote{}, errors.New("rate API URL is empty")
	}
	req, err := http.NewRequest(http.MethodGet, rateURL, nil)
	if err != nil {
		return okpayRateQuote{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := (&http.Client{Timeout: 8 * time.Second}).Do(req)
	if err != nil {
		return okpayRateQuote{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return okpayRateQuote{}, err
	}
	if resp.StatusCode/100 != 2 {
		return okpayRateQuote{}, fmt.Errorf("rate API HTTP %d", resp.StatusCode)
	}
	var rawRate float64
	if source == okpayRateSourceOkxAlipayTier {
		rawRate, err = parseOkpayOkxAlipayTierRateFromBody(body, side, tier)
	} else {
		rawRate, err = parseOkpayRateFromBody(body)
	}
	if err != nil {
		return okpayRateQuote{}, err
	}
	adjusted, err := applyOkpayRateAdjustment(rawRate)
	if err != nil {
		return okpayRateQuote{}, err
	}
	return okpayRateQuote{RawRate: rawRate, AdjustedRate: adjusted, Source: source, Tier: tier, Side: side, Adjustment: setting.OkpayRateAdjustmentValue, AdjustmentType: normalizeOkpayAdjustmentType()}, nil
}

func fetchOkpayUsdtCnyRate() (float64, string, error) {
	quote, err := fetchOkpayUsdtCnyRateQuote()
	if err != nil {
		return 0, "", err
	}
	return quote.AdjustedRate, quote.Source, nil
}

func getOkpayUsdtCnyRate() (float64, string, bool) {
	fallback := getOkpayFallbackUsdtCnyRate()
	if !setting.OkpayAutoExchangeEnabled {
		return fallback, "fallback", false
	}
	now := time.Now()
	cacheKey := okpayRateCacheKey()
	okpayRateCacheMu.Lock()
	if okpayRateCache.rate > 0 && okpayRateCache.configKey == cacheKey && now.Before(okpayRateCache.expiresAt) {
		cached := okpayRateCache
		okpayRateCacheMu.Unlock()
		return cached.rate, cached.source, false
	}
	okpayRateCacheMu.Unlock()
	rate, source, err := fetchOkpayUsdtCnyRate()
	if err != nil {
		return fallback, "fallback", true
	}
	okpayRateCacheMu.Lock()
	okpayRateCache = okpayRateCacheEntry{rate: rate, source: source, configKey: cacheKey, expiresAt: now.Add(okpayRateCacheTTL)}
	okpayRateCacheMu.Unlock()
	return rate, source, false
}

func getOkpayPaymentAmountFromFiat(fiat float64) okpayPaymentAmount {
	coin := getOkpayCoin()
	if coin != "USDT" {
		return okpayPaymentAmount{FiatAmount: fiat, CoinAmount: fiat, Rate: 1, RateSource: "coin", Coin: coin}
	}
	rate, source, failed := getOkpayUsdtCnyRate()
	coinAmount := decimal.NewFromFloat(fiat).Div(decimal.NewFromFloat(rate)).Round(8).InexactFloat64()
	return okpayPaymentAmount{FiatAmount: fiat, CoinAmount: coinAmount, Rate: rate, RateSource: source, AutoRateFailed: failed, Coin: coin}
}

func PreviewOkpayRate(c *gin.Context) {
	quote, err := fetchOkpayUsdtCnyRateQuote()
	if err != nil {
		common.ApiErrorMsg(c, "获取 OKPay 汇率失败: "+err.Error())
		return
	}
	common.ApiSuccess(c, gin.H{
		"raw_rate": strconv.FormatFloat(quote.RawRate, 'f', -1, 64), "adjusted_rate": strconv.FormatFloat(quote.AdjustedRate, 'f', -1, 64),
		"source": quote.Source, "side": quote.Side, "tier": quote.Tier,
		"adjustment_type": quote.AdjustmentType, "adjustment": strconv.FormatFloat(quote.Adjustment, 'f', -1, 64),
	})
}
func getOkpayMinTopup() int64 {
	minimum := int64(setting.OkpayMinTopUp)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		minimum = decimal.NewFromInt(minimum).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
	}
	return minimum
}

func RequestOkpayAmount(c *gin.Context) {
	var req OkpayAmountRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Amount < getOkpayMinTopup() {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	userId := c.GetInt("id")
	if rejectInvalidTopUpQuota(c, userId, req.Amount) {
		return
	}
	group, err := model.GetUserGroup(userId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	businessPayMoney := getOkpayFiatPayMoneyWithInvoice(req.Amount, group, req.Invoice)
	discount, err := calculateTopUpPromoCodeDiscount(req.PromoCode, req.Invoice, businessPayMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if discount != nil {
		businessPayMoney = discount.PaidAmount
	}
	invoiceAmounts, err := buildInvoicePaymentPreviewAmounts(req.Invoice, model.PaymentProviderOkpay, businessPayMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	payment := getOkpayPaymentAmountFromFiat(invoiceAmounts.TotalPayment)
	amount := decimal.NewFromFloat(payment.CoinAmount).StringFixed(8)
	response := gin.H{"message": "success", "data": amount, "amount": amount, "coin": payment.Coin, "fiat_amount": decimal.NewFromFloat(invoiceAmounts.TotalPayment).StringFixed(2), "fiat_currency": "CNY", "rate": strconv.FormatFloat(payment.Rate, 'f', -1, 64), "rate_source": payment.RateSource, "auto_rate_failed": payment.AutoRateFailed}
	if discount != nil {
		response["discount"] = discount
	}
	addInvoiceFieldsToResponse(response, invoiceAmounts)
	c.JSON(http.StatusOK, response)
}

func createOkpayPaymentLink(c *gin.Context, tradeNo string, payment okpayPaymentAmount, name, callbackURL, redirectURL string) (*okpayPaymentLinkResult, error) {
	amount := decimal.NewFromFloat(payment.CoinAmount).StringFixed(8)
	payload := map[string]string{"unique_id": tradeNo, "amount": amount, "return_url": redirectURL, "callback_url": callbackURL, "coin": payment.Coin, "name": strings.TrimSpace(name), "id": setting.OkpayMerchantId}
	payload["sign"] = generateOkpaySignature(payload, setting.OkpayMerchantToken)
	form := url.Values{}
	for key, value := range payload {
		form.Set(key, value)
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, strings.TrimRight(setting.OkpayGatewayUrl, "/")+"/payLink", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("OKPay HTTP %d", resp.StatusCode)
	}
	payURL := strings.TrimSpace(gjson.GetBytes(body, "data.pay_url").String())
	providerOrder := strings.TrimSpace(gjson.GetBytes(body, "data.order_id").String())
	if payURL == "" {
		payURL = strings.TrimSpace(gjson.GetBytes(body, "data.0.pay_url").String())
	}
	if providerOrder == "" {
		providerOrder = strings.TrimSpace(gjson.GetBytes(body, "data.0.order_id").String())
	}
	if payURL == "" || providerOrder == "" {
		return nil, errors.New("OKPay response missing pay_url or order_id")
	}
	return &okpayPaymentLinkResult{ProviderOrderId: providerOrder, PaymentUrl: payURL, Amount: amount, PaymentAmount: payment}, nil
}

func RequestOkpayPay(c *gin.Context) {
	var req OkpayPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Amount < getOkpayMinTopup() {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	userId := c.GetInt("id")
	if rejectInvalidTopUpQuota(c, userId, req.Amount) {
		return
	}
	creditedQuota, _ := validateTopUpQuota(req.Amount)
	group, err := model.GetUserGroup(userId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	originalPayMoney := getOkpayFiatPayMoneyWithInvoice(req.Amount, group, req.Invoice)
	businessPayMoney := originalPayMoney
	discount, err := calculateTopUpPromoCodeDiscount(req.PromoCode, req.Invoice, businessPayMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if discount != nil {
		businessPayMoney = discount.PaidAmount
	}
	invoiceAmounts, err := buildInvoicePaymentAmounts(req.Invoice, model.PaymentProviderOkpay, businessPayMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	totalPayMoney := invoiceAmounts.TotalPayment
	tradeNo := fmt.Sprintf("USR%dNO%s%d", userId, common.GetRandomString(6), time.Now().Unix())
	topUp := &model.TopUp{UserId: userId, Amount: normalizeTopUpAmountForStorage(req.Amount), Money: totalPayMoney, CreditedQuota: creditedQuota, AffiliateSourceQuota: creditedQuota, TradeNo: tradeNo, PaymentMethod: model.PaymentMethodOkpay, PaymentProvider: model.PaymentProviderOkpay, RequestIP: c.ClientIP(), CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending}
	model.ApplyPromoCodeResultToTopUp(topUp, discount)
	applyInvoiceToTopUp(topUp, invoiceAmounts, originalPayMoney, businessPayMoney, true)
	if err := topUp.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	if totalPayMoney < 0.01 {
		completed, quotaToAdd, _, err := model.CompleteFreeTopUp(tradeNo, model.PaymentProviderOkpay)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		c.JSON(http.StatusOK, freeTopUpResponse(completed, quotaToAdd, discount))
		return
	}
	payment := getOkpayPaymentAmountFromFiat(totalPayMoney)
	providerAmount := decimal.NewFromFloat(payment.CoinAmount).StringFixed(8)
	attempt, err := model.CreateTopUpPaymentAttempt(tradeNo, model.PaymentProviderOkpay, model.PaymentMethodOkpay, providerAmount, payment.Coin)
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderOkpay, common.TopUpStatusFailed)
		common.ApiError(c, err)
		return
	}
	link, err := createOkpayPaymentLink(c, tradeNo, payment, "TopUp-"+tradeNo, service.GetCallbackAddress()+"/api/okpay/notify", paymentReturnPath("/usage-logs"))
	if err != nil {
		_ = model.MarkTopUpPaymentAttemptLaunchFailed(attempt.Id, err.Error())
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderOkpay, common.TopUpStatusFailed)
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	if err := model.MarkTopUpPaymentAttemptLaunched(attempt.Id, link.ProviderOrderId); err != nil {
		_ = model.MarkTopUpPaymentAttemptLaunchFailed(attempt.Id, err.Error())
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderOkpay, common.TopUpStatusFailed)
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{"payment_url": link.PaymentUrl, "trade_no": tradeNo, "provider_order_id": link.ProviderOrderId, "amount": link.Amount, "coin": payment.Coin, "fiat_amount": decimal.NewFromFloat(totalPayMoney).StringFixed(2), "fiat_currency": "CNY", "rate": strconv.FormatFloat(payment.Rate, 'f', -1, 64), "rate_source": payment.RateSource, "auto_rate_failed": payment.AutoRateFailed, "invoice": invoiceAmounts}})
}

func writeOkpayCallbackStatus(c *gin.Context, success bool) {
	status := "fail"
	if success {
		status = "success"
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(fmt.Sprintf(`{"status":"%s"}`, status)))
}

func OkpayNotify(c *gin.Context) {
	if !isOkpayWebhookEnabled() {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	values, body, err := parseOkpayCallbackValues(c)
	if err != nil || len(values) == 0 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	orderedSource := body
	if len(bytes.TrimSpace(orderedSource)) == 0 {
		orderedSource = []byte(c.Request.URL.RawQuery)
	}
	if !verifyOkpayCallbackSignature(values, setting.OkpayMerchantToken, parseOkpayCallbackOrderedPairs(orderedSource)) {
		writeOkpayCallbackStatus(c, false)
		return
	}
	merchantId := strings.TrimSpace(values.Get("id"))
	if merchantId == "" || merchantId != strings.TrimSpace(setting.OkpayMerchantId) {
		writeOkpayCallbackStatus(c, false)
		return
	}
	paymentStatus := strings.TrimSpace(values.Get("data[status]"))
	if !isOkpayCallbackSuccess(values.Get("status"), paymentStatus) {
		writeOkpayCallbackStatus(c, true)
		return
	}
	tradeNo := strings.TrimSpace(values.Get("data[unique_id]"))
	providerOrder := strings.TrimSpace(values.Get("data[order_id]"))
	amount := strings.TrimSpace(values.Get("data[amount]"))
	currency := strings.ToUpper(strings.TrimSpace(values.Get("data[coin]")))
	if providerOrder == "" || amount == "" || currency == "" {
		writeOkpayCallbackStatus(c, false)
		return
	}
	if tradeNo == "" {
		if order := model.GetSubscriptionOrderByProviderOrderId(model.PaymentProviderOkpay, providerOrder); order != nil {
			tradeNo = order.TradeNo
		} else {
			resolved, err := model.ResolveTopUpPaymentAttempt(model.PaymentProviderOkpay, "", providerOrder)
			if err != nil {
				writeOkpayCallbackStatus(c, false)
				return
			}
			tradeNo = resolved.TradeNo
		}
	}
	if order := model.GetSubscriptionOrderByTradeNo(tradeNo); order != nil {
		if order.PaymentProvider != model.PaymentProviderOkpay {
			writeOkpayCallbackStatus(c, false)
			return
		}
		if strings.TrimSpace(order.ProviderOrderId) == "" || strings.TrimSpace(order.ProviderAmount) == "" || strings.TrimSpace(order.ProviderCurrency) == "" {
			writeOkpayCallbackStatus(c, false)
			return
		}
		snapshot := &model.TopUpPaymentAttempt{Id: 1, PaymentProvider: model.PaymentProviderOkpay, ProviderOrderId: order.ProviderOrderId, ProviderAmount: order.ProviderAmount, ProviderCurrency: strings.ToUpper(order.ProviderCurrency)}
		if err := model.ValidateTopUpPaymentAttemptSnapshot(snapshot, model.PaymentProviderOkpay, providerOrder, amount, currency, decimal.Zero); err != nil {
			writeOkpayCallbackStatus(c, false)
			return
		}
		if err := model.CompleteSubscriptionOrder(tradeNo, common.GetJsonString(values), model.PaymentProviderOkpay, model.PaymentMethodOkpay); err != nil {
			writeOkpayCallbackStatus(c, false)
			return
		}
		writeOkpayCallbackStatus(c, true)
		return
	}
	if topUp := model.GetTopUpByTradeNo(tradeNo); topUp == nil {
		writeOkpayCallbackStatus(c, false)
		return
	}
	attempt, err := model.ResolveTopUpPaymentAttempt(model.PaymentProviderOkpay, tradeNo, providerOrder)
	if err != nil {
		writeOkpayCallbackStatus(c, false)
		return
	}
	if err := model.ValidateTopUpPaymentAttemptSnapshot(attempt, model.PaymentProviderOkpay, providerOrder, amount, currency, decimal.Zero); err != nil {
		writeOkpayCallbackStatus(c, false)
		return
	}
	if err := model.BindTopUpPaymentAttemptProviderOrder(attempt.Id, providerOrder); err != nil {
		writeOkpayCallbackStatus(c, false)
		return
	}
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	if _, err := model.CompleteTopUpPaymentAttempt(attempt.Id, tradeNo, model.PaymentProviderOkpay, model.PaymentMethodOkpay, c.ClientIP()); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("OKPay 充值处理失败 trade_no=%s provider_order_id=%s error=%q", tradeNo, providerOrder, err.Error()))
		writeOkpayCallbackStatus(c, false)
		return
	}
	writeOkpayCallbackStatus(c, true)
}
