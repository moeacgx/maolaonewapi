package controller

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

type BepusdtPayRequest struct {
	Amount    int64                `json:"amount"`
	TradeType string               `json:"trade_type"`
	PromoCode string               `json:"promo_code"`
	Invoice   model.InvoiceRequest `json:"invoice"`
}

type SubscriptionBepusdtPayRequest struct {
	PlanId    int                  `json:"plan_id"`
	TradeType string               `json:"trade_type"`
	PromoCode string               `json:"promo_code"`
	Invoice   model.InvoiceRequest `json:"invoice"`
}

func SubscriptionRequestBepusdtPay(c *gin.Context) {
	if !requirePaymentCompliance(c) {
		return
	}
	var req SubscriptionBepusdtPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 || !isValidBepusdtTradeType(req.TradeType) {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !plan.Enabled {
		common.ApiErrorMsg(c, "套餐未启用")
		return
	}
	planPriceUSD, err := model.SubscriptionPlanPriceUSD(plan)
	if err != nil || planPriceUSD < 0.01 {
		common.ApiErrorMsg(c, "套餐金额过低")
		return
	}
	userId := c.GetInt("id")
	if plan.MaxPurchasePerUser > 0 {
		count, err := model.CountActiveUserSubscriptionsByPlan(userId, plan.Id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if count >= int64(plan.MaxPurchasePerUser) {
			common.ApiErrorMsg(c, "已达到该套餐购买上限")
			return
		}
	}
	discount, err := calculateSubscriptionPromoCodeDiscount(req.PromoCode, req.Invoice, plan.Id, planPriceUSD)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	paidUSD := planPriceUSD
	if discount != nil {
		paidUSD = discount.PaidAmount
	}
	payMoney, err := getSubscriptionBepusdtPayMoney(plan, paidUSD)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	bepusdtDiscount, err := convertSubscriptionDiscountToBepusdtMoney(plan, discount)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if bepusdtDiscount != nil {
		payMoney = bepusdtDiscount.PaidAmount
	}
	invoiceAmounts, err := buildInvoicePaymentAmounts(req.Invoice, model.PaymentProviderBepusdt, payMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	totalPayMoney := payMoney
	if invoiceAmounts.Required {
		totalPayMoney = invoiceAmounts.TotalPayment
	}
	tradeNo := fmt.Sprintf("BEPUSDT_SUBUSR%dNO%s%d", userId, common.GetRandomString(6), time.Now().Unix())
	businessQuota, err := subscriptionPaidQuotaFromUSD(paidUSD)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	order := &model.SubscriptionOrder{UserId: userId, PlanId: plan.Id, Money: totalPayMoney, TradeNo: tradeNo, PaymentMethod: model.PaymentMethodBepusdt, PaymentProvider: model.PaymentProviderBepusdt, RequestIP: c.ClientIP(), ProviderAmount: decimal.NewFromFloat(totalPayMoney).Round(2).StringFixed(2), ProviderCurrency: model.SubscriptionCurrencyCNY, CreateTime: time.Now().Unix(), Status: common.TopUpStatusPending}
	if discount == nil {
		order.AffiliateSourceQuota = businessQuota
	}
	model.ApplyPromoCodeResultToSubscriptionOrder(order, bepusdtDiscount)
	applyInvoiceToSubscriptionOrder(order, invoiceAmounts, payMoney, payMoney, businessQuota)
	if err := order.Insert(); err != nil {
		common.ApiErrorMsg(c, "创建订单失败")
		return
	}
	if totalPayMoney < 0.01 {
		if err := model.CompleteFreeSubscriptionOrder(tradeNo, model.PaymentProviderBepusdt); err != nil {
			common.ApiError(c, err)
			return
		}
		common.ApiSuccess(c, gin.H{"completed": true, "trade_no": tradeNo})
		return
	}
	callback := service.GetCallbackAddress() + "/api/bepusdt/notify"
	result, err := createBepusdtTransactionDetails(c, tradeNo, totalPayMoney, req.TradeType, callback, paymentReturnPath("/console/topup"))
	if err != nil {
		_ = model.ExpireSubscriptionOrder(tradeNo, model.PaymentProviderBepusdt)
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	if err := model.UpdateSubscriptionOrderProviderSnapshot(tradeNo, model.PaymentProviderBepusdt, result.Data.TradeId, result.Data.Amount, model.SubscriptionCurrencyCNY); err != nil {
		_ = model.ExpireSubscriptionOrder(tradeNo, model.PaymentProviderBepusdt)
		common.ApiErrorMsg(c, "保存支付订单失败")
		return
	}
	common.ApiSuccess(c, gin.H{"payment_url": result.Data.PaymentUrl, "trade_no": tradeNo, "invoice": invoiceAmounts})
}

type BepusdtAmountRequest struct {
	Amount    int64                `json:"amount"`
	PromoCode string               `json:"promo_code"`
	Invoice   model.InvoiceRequest `json:"invoice"`
}

type bepusdtCreateTransactionResp struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	Data       struct {
		Fiat           string `json:"fiat"`
		TradeType      string `json:"trade_type"`
		TradeId        string `json:"trade_id"`
		OrderId        string `json:"order_id"`
		Amount         string `json:"amount"`
		ActualAmount   string `json:"actual_amount"`
		Token          string `json:"token"`
		ExpirationTime int    `json:"expiration_time"`
		Status         int    `json:"status"`
		PaymentUrl     string `json:"payment_url"`
	} `json:"data"`
}

type bepusdtNotifyPayload struct {
	TradeId            string      `json:"trade_id"`
	OrderId            string      `json:"order_id"`
	Amount             interface{} `json:"amount"`
	ActualAmount       interface{} `json:"actual_amount"`
	Token              string      `json:"token"`
	BlockTransactionId string      `json:"block_transaction_id"`
	Signature          string      `json:"signature"`
	Status             int         `json:"status"`
}

type bepusdtParsedNotify struct {
	Payload bepusdtNotifyPayload
	Params  map[string]string
	Body    []byte
}

func generateBepusdtSignature(params map[string]string, authToken string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if key == "signature" || value == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var signed strings.Builder
	for i, key := range keys {
		if i != 0 {
			signed.WriteByte('&')
		}
		signed.WriteString(key)
		signed.WriteByte('=')
		signed.WriteString(params[key])
	}
	signed.WriteString(authToken)
	digest := md5.Sum([]byte(signed.String()))
	return fmt.Sprintf("%x", digest)
}

func verifyBepusdtNotifyParamsSignature(params map[string]string, authToken string) bool {
	actual := strings.TrimSpace(params["signature"])
	return actual != "" && strings.EqualFold(generateBepusdtSignature(params, authToken), actual)
}

func verifyBepusdtNotifySignature(payload *bepusdtNotifyPayload, authToken string) bool {
	if payload == nil {
		return false
	}
	params := map[string]string{"status": strconv.Itoa(payload.Status)}
	if payload.TradeId != "" {
		params["trade_id"] = payload.TradeId
	}
	if payload.OrderId != "" {
		params["order_id"] = payload.OrderId
	}
	if payload.Amount != nil {
		params["amount"] = fmt.Sprintf("%v", payload.Amount)
	}
	if payload.ActualAmount != nil {
		params["actual_amount"] = fmt.Sprintf("%v", payload.ActualAmount)
	}
	if payload.Token != "" {
		params["token"] = payload.Token
	}
	if payload.BlockTransactionId != "" {
		params["block_transaction_id"] = payload.BlockTransactionId
	}
	params["signature"] = payload.Signature
	return verifyBepusdtNotifyParamsSignature(params, authToken)
}

func bepusdtPayloadFromParams(params map[string]string) bepusdtNotifyPayload {
	status, _ := strconv.Atoi(strings.TrimSpace(params["status"]))
	return bepusdtNotifyPayload{
		TradeId:            strings.TrimSpace(params["trade_id"]),
		OrderId:            strings.TrimSpace(params["order_id"]),
		Amount:             strings.TrimSpace(params["amount"]),
		ActualAmount:       strings.TrimSpace(params["actual_amount"]),
		Token:              strings.TrimSpace(params["token"]),
		BlockTransactionId: strings.TrimSpace(params["block_transaction_id"]),
		Signature:          strings.TrimSpace(params["signature"]),
		Status:             status,
	}
}

func parseBepusdtJSONNotify(body []byte) (bepusdtParsedNotify, error) {
	var raw map[string]json.RawMessage
	if err := common.Unmarshal(body, &raw); err != nil {
		return bepusdtParsedNotify{}, err
	}
	params := make(map[string]string, len(raw))
	for key, value := range raw {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		text := strings.TrimSpace(common.JsonRawMessageToString(value))
		if text != "" {
			params[key] = text
		}
	}
	return bepusdtParsedNotify{Payload: bepusdtPayloadFromParams(params), Params: params, Body: body}, nil
}

func bepusdtParamsFromValues(values url.Values) map[string]string {
	params := make(map[string]string, len(values))
	for key := range values {
		key = strings.TrimSpace(key)
		value := strings.TrimSpace(values.Get(key))
		if key != "" && value != "" {
			params[key] = value
		}
	}
	return params
}

func parseBepusdtNotifyPayload(c *gin.Context) (bepusdtParsedNotify, error) {
	values := c.Request.URL.Query()
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return bepusdtParsedNotify{}, err
	}
	c.Request.Body = io.NopCloser(bytes.NewReader(body))

	contentType := strings.ToLower(c.GetHeader("Content-Type"))
	if strings.Contains(contentType, "multipart/form-data") {
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			return bepusdtParsedNotify{Body: body}, err
		}
		for key, items := range c.Request.PostForm {
			values[key] = items
		}
		if c.Request.MultipartForm != nil {
			for key, items := range c.Request.MultipartForm.Value {
				values[key] = items
			}
		}
	} else if trimmed := bytes.TrimSpace(body); len(trimmed) != 0 {
		if trimmed[0] == '{' {
			parsed, err := parseBepusdtJSONNotify(trimmed)
			if err != nil {
				return bepusdtParsedNotify{Body: body}, err
			}
			for key, value := range bepusdtParamsFromValues(values) {
				if _, exists := parsed.Params[key]; !exists {
					parsed.Params[key] = value
				}
			}
			parsed.Payload = bepusdtPayloadFromParams(parsed.Params)
			parsed.Body = body
			c.Request.Body = io.NopCloser(bytes.NewReader(body))
			return parsed, nil
		}
		bodyValues, err := url.ParseQuery(string(trimmed))
		if err != nil {
			return bepusdtParsedNotify{Body: body}, err
		}
		for key, items := range bodyValues {
			values[key] = items
		}
	}
	params := bepusdtParamsFromValues(values)
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	return bepusdtParsedNotify{Payload: bepusdtPayloadFromParams(params), Params: params, Body: body}, nil
}

func getBepusdtPayMoney(amount int64, group string) float64 {
	return getBepusdtPayMoneyWithInvoice(amount, group, model.InvoiceRequest{})
}

func getBepusdtPayMoneyWithInvoice(amount int64, group string, invoice model.InvoiceRequest) float64 {
	displayAmount := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		displayAmount = displayAmount.Div(decimal.NewFromFloat(common.QuotaPerUnit))
	}
	groupRatio := common.GetTopupGroupRatio(group)
	if groupRatio == 0 {
		groupRatio = 1
	}
	return displayAmount.
		Mul(decimal.NewFromFloat(setting.BepusdtUnitPrice)).
		Mul(decimal.NewFromFloat(groupRatio)).
		Mul(decimal.NewFromFloat(topUpAmountDiscount(amount, invoice))).
		InexactFloat64()
}

func getBepusdtPayMoneyFromUSD(amount float64) float64 {
	return decimal.NewFromFloat(amount).Mul(decimal.NewFromFloat(setting.BepusdtUnitPrice)).Round(2).InexactFloat64()
}

func getBepusdtMinTopup() int64 {
	minimum := int64(setting.BepusdtMinTopUp)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		minimum = decimal.NewFromInt(minimum).Mul(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
	}
	return minimum
}

func isValidBepusdtTradeType(tradeType string) bool {
	tradeType = strings.TrimSpace(tradeType)
	for _, chain := range setting.GetBepusdtChains() {
		if strings.TrimSpace(chain.TradeType) == tradeType {
			return true
		}
	}
	return false
}

func RequestBepusdtAmount(c *gin.Context) {
	var req BepusdtAmountRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Amount < getBepusdtMinTopup() {
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
	businessPayMoney := getBepusdtPayMoneyWithInvoice(req.Amount, group, req.Invoice)
	discount, err := calculateTopUpPromoCodeDiscount(req.PromoCode, req.Invoice, businessPayMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if discount != nil {
		businessPayMoney = discount.PaidAmount
	}
	invoiceAmounts, err := buildInvoicePaymentPreviewAmounts(req.Invoice, model.PaymentProviderBepusdt, businessPayMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response := gin.H{"message": "success", "data": decimal.NewFromFloat(invoiceAmounts.TotalPayment).StringFixed(2)}
	if discount != nil {
		response["discount"] = discount
	}
	addInvoiceFieldsToResponse(response, invoiceAmounts)
	c.JSON(http.StatusOK, response)
}

func RequestBepusdtPay(c *gin.Context) {
	var req BepusdtPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Amount < getBepusdtMinTopup() || !isValidBepusdtTradeType(req.TradeType) {
		common.ApiErrorMsg(c, "充值数量或支付链无效")
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
	originalPayMoney := getBepusdtPayMoneyWithInvoice(req.Amount, group, req.Invoice)
	businessPayMoney := originalPayMoney
	discount, err := calculateTopUpPromoCodeDiscount(req.PromoCode, req.Invoice, businessPayMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if discount != nil {
		businessPayMoney = discount.PaidAmount
	}
	invoiceAmounts, err := buildInvoicePaymentAmounts(req.Invoice, model.PaymentProviderBepusdt, businessPayMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	totalPayMoney := invoiceAmounts.TotalPayment
	tradeNo := fmt.Sprintf("USR%dNO%s%d", userId, common.GetRandomString(6), time.Now().Unix())
	topUp := &model.TopUp{
		UserId: userId, Amount: normalizeTopUpAmountForStorage(req.Amount), Money: totalPayMoney,
		CreditedQuota: creditedQuota, AffiliateSourceQuota: creditedQuota,
		TradeNo: tradeNo, PaymentMethod: model.PaymentMethodBepusdt, PaymentProvider: model.PaymentProviderBepusdt,
		RequestIP: c.ClientIP(), CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending,
	}
	model.ApplyPromoCodeResultToTopUp(topUp, discount)
	applyInvoiceToTopUp(topUp, invoiceAmounts, originalPayMoney, businessPayMoney, true)
	if err := topUp.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	if totalPayMoney < 0.01 {
		completed, quotaToAdd, _, err := model.CompleteFreeTopUp(tradeNo, model.PaymentProviderBepusdt)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		c.JSON(http.StatusOK, freeTopUpResponse(completed, quotaToAdd, discount))
		return
	}
	payMoney := decimal.NewFromFloat(totalPayMoney).Round(2)
	attempt, err := model.CreateTopUpPaymentAttempt(tradeNo, model.PaymentProviderBepusdt, model.PaymentMethodBepusdt, payMoney.StringFixed(2), "CNY")
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderBepusdt, common.TopUpStatusFailed)
		common.ApiError(c, err)
		return
	}
	callback := service.GetCallbackAddress() + "/api/bepusdt/notify"
	result, err := createBepusdtTransactionDetails(c, tradeNo, payMoney.InexactFloat64(), req.TradeType, callback, paymentReturnPath("/usage-logs"))
	if err != nil {
		_ = model.MarkTopUpPaymentAttemptLaunchFailed(attempt.Id, err.Error())
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderBepusdt, common.TopUpStatusFailed)
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	if err := model.MarkTopUpPaymentAttemptLaunched(attempt.Id, result.Data.TradeId); err != nil {
		_ = model.MarkTopUpPaymentAttemptLaunchFailed(attempt.Id, err.Error())
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderBepusdt, common.TopUpStatusFailed)
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{"payment_url": result.Data.PaymentUrl, "trade_no": tradeNo, "invoice": invoiceAmounts}})
}

func createBepusdtTransactionDetails(c *gin.Context, orderId string, amountCNY float64, tradeType, notifyUrl, redirectUrl string, names ...string) (*bepusdtCreateTransactionResp, error) {
	name := "TopUp-" + orderId
	if len(names) != 0 && strings.TrimSpace(names[0]) != "" {
		name = strings.TrimSpace(names[0])
	}
	amountText := strconv.FormatFloat(amountCNY, 'f', -1, 64)
	params := map[string]string{
		"order_id": orderId, "amount": amountText, "fiat": "CNY", "trade_type": tradeType,
		"notify_url": notifyUrl, "redirect_url": redirectUrl, "name": name,
		"timeout": strconv.Itoa(setting.BepusdtTimeout),
	}
	params["signature"] = generateBepusdtSignature(params, setting.BepusdtAuthToken)
	body, err := common.Marshal(map[string]interface{}{
		"order_id": orderId, "amount": amountCNY, "fiat": "CNY", "trade_type": tradeType,
		"notify_url": notifyUrl, "redirect_url": redirectUrl, "name": name,
		"timeout": setting.BepusdtTimeout, "signature": params["signature"],
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, strings.TrimRight(setting.BepusdtApiUrl, "/")+"/api/v1/order/create-transaction", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("bepusdt API HTTP %d", resp.StatusCode)
	}
	var result bepusdtCreateTransactionResp
	if err := common.Unmarshal(responseBody, &result); err != nil {
		return nil, err
	}
	if result.StatusCode != http.StatusOK || result.Data.PaymentUrl == "" || result.Data.TradeId == "" {
		return nil, fmt.Errorf("bepusdt API error: %s", result.Message)
	}
	if result.Data.OrderId != orderId || !strings.EqualFold(result.Data.Fiat, "CNY") || result.Data.TradeType != tradeType {
		return nil, errors.New("bepusdt response order, currency, or chain mismatch")
	}
	actual, err := decimal.NewFromString(strings.TrimSpace(result.Data.Amount))
	if err != nil || actual.Sub(decimal.NewFromFloat(amountCNY)).Abs().GreaterThan(decimal.NewFromFloat(0.01)) {
		return nil, errors.New("bepusdt response amount mismatch")
	}
	return &result, nil
}

func createBepusdtTransaction(c *gin.Context, orderId string, amountCNY float64, tradeType, notifyUrl, redirectUrl string, names ...string) (string, error) {
	result, err := createBepusdtTransactionDetails(c, orderId, amountCNY, tradeType, notifyUrl, redirectUrl, names...)
	if err != nil {
		return "", err
	}
	return result.Data.PaymentUrl, nil
}

func BepusdtNotify(c *gin.Context) {
	if !isBepusdtWebhookEnabled() {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	parsed, err := parseBepusdtNotifyPayload(c)
	if err != nil || len(parsed.Params) == 0 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if !verifyBepusdtNotifyParamsSignature(parsed.Params, setting.BepusdtAuthToken) {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}
	payload := parsed.Payload
	switch payload.Status {
	case 2:
		handleBepusdtPaymentSuccess(c, &payload)
	case 3:
		tradeNo := strings.TrimSpace(payload.OrderId)
		if tradeNo != "" {
			if order := model.GetSubscriptionOrderByTradeNo(tradeNo); order != nil && order.PaymentProvider == model.PaymentProviderBepusdt {
				_ = model.ExpireSubscriptionOrder(tradeNo, model.PaymentProviderBepusdt)
			} else {
				_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderBepusdt, common.TopUpStatusExpired)
			}
		}
		_, _ = c.Writer.Write([]byte("ok"))
	default:
		_, _ = c.Writer.Write([]byte("ok"))
	}
}

func handleBepusdtPaymentSuccess(c *gin.Context, payload *bepusdtNotifyPayload) {
	tradeNo := strings.TrimSpace(payload.OrderId)
	tradeId := strings.TrimSpace(payload.TradeId)
	amount := strings.TrimSpace(fmt.Sprintf("%v", payload.Amount))
	if tradeNo == "" || tradeId == "" || amount == "" {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	if topUp := model.GetTopUpByTradeNo(tradeNo); topUp == nil {
		if order := model.GetSubscriptionOrderByTradeNo(tradeNo); order != nil && order.PaymentProvider == model.PaymentProviderBepusdt {
			if model.AllowLegacyBepusdtSubscriptionPaymentSnapshotBinding(order) {
				expected := decimal.NewFromFloat(order.Money)
				actual, parseErr := decimal.NewFromString(amount)
				if parseErr != nil || actual.Sub(expected).Abs().GreaterThan(decimal.NewFromFloat(0.01)) {
					c.AbortWithStatus(http.StatusBadRequest)
					return
				}
				if err := model.UpdateSubscriptionOrderProviderSnapshot(tradeNo, model.PaymentProviderBepusdt, tradeId, amount, model.SubscriptionCurrencyCNY); err != nil {
					c.AbortWithStatus(http.StatusBadRequest)
					return
				}
				order = model.GetSubscriptionOrderByTradeNo(tradeNo)
			}
			if order == nil || strings.TrimSpace(order.ProviderOrderId) == "" || order.ProviderOrderId != tradeId || strings.TrimSpace(order.ProviderAmount) == "" || !strings.EqualFold(order.ProviderCurrency, model.SubscriptionCurrencyCNY) {
				c.AbortWithStatus(http.StatusBadRequest)
				return
			}
			expected, parseErr := decimal.NewFromString(strings.TrimSpace(order.ProviderAmount))
			actual, actualErr := decimal.NewFromString(amount)
			if parseErr != nil || actualErr != nil || !actual.Round(2).Equal(expected.Round(2)) {
				c.AbortWithStatus(http.StatusBadRequest)
				return
			}
			if err := model.CompleteSubscriptionOrder(tradeNo, common.GetJsonString(payload), model.PaymentProviderBepusdt, model.PaymentMethodBepusdt); err != nil {
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}
			_, _ = c.Writer.Write([]byte("ok"))
			return
		}
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	attempt, err := model.ResolveTopUpPaymentAttempt(model.PaymentProviderBepusdt, tradeNo, tradeId)
	if err == nil {
		if err := model.ValidateTopUpPaymentAttemptSnapshot(attempt, model.PaymentProviderBepusdt, tradeId, amount, "CNY", decimal.NewFromFloat(0.01)); err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		if err := model.BindTopUpPaymentAttemptProviderOrder(attempt.Id, tradeId); err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		_, err = model.CompleteTopUpPaymentAttempt(attempt.Id, tradeNo, model.PaymentProviderBepusdt, model.PaymentMethodBepusdt, c.ClientIP())
	} else if errors.Is(err, model.ErrTopUpPaymentAttemptNotFound) {
		topUp := model.GetTopUpByTradeNo(tradeNo)
		if !model.AllowLegacyTopUpCallback(topUp, model.PaymentProviderBepusdt) {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		expected := decimal.NewFromFloat(topUp.Money)
		if strings.TrimSpace(topUp.ProviderOrderId) != "" && topUp.ProviderOrderId != tradeId {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(topUp.ProviderAmount) != "" {
			var expectedErr error
			expected, expectedErr = decimal.NewFromString(strings.TrimSpace(topUp.ProviderAmount))
			if expectedErr != nil {
				c.AbortWithStatus(http.StatusBadRequest)
				return
			}
		}
		actual, parseErr := decimal.NewFromString(amount)
		if parseErr != nil || (topUp.ProviderCurrency != "" && !strings.EqualFold(topUp.ProviderCurrency, "CNY")) || actual.Sub(expected).Abs().GreaterThan(decimal.NewFromFloat(0.01)) {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		_, err = model.CompleteLegacyBepusdtTopUpPayment(tradeNo, tradeId, amount, c.ClientIP())
	}
	if errors.Is(err, model.ErrTopUpPaymentAttemptMismatch) {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Bepusdt 充值处理失败 trade_no=%s trade_id=%s error=%q", tradeNo, tradeId, err.Error()))
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	_, _ = c.Writer.Write([]byte("ok"))
}
