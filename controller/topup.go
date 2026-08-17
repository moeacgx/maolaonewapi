package controller

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
)

func GetTopUpInfo(c *gin.Context) {
	complianceConfirmed := operation_setting.IsPaymentComplianceConfirmed()
	payMethods := operation_setting.PayMethods
	if !complianceConfirmed {
		payMethods = []map[string]string{}
	}
	if isStripeTopUpEnabled() {
		payMethods = appendPaymentMethod(payMethods, map[string]string{"name": "Stripe", "type": model.PaymentMethodStripe, "color": "#635BFF", "min_topup": strconv.Itoa(setting.StripeMinTopUp)})
	}
	enableWaffoPancake := isWaffoPancakeTopUpEnabled()
	if enableWaffoPancake {
		payMethods = appendPaymentMethod(payMethods, map[string]string{"name": "Waffo Pancake", "type": model.PaymentMethodWaffoPancake, "color": "#F97316", "min_topup": strconv.Itoa(setting.WaffoPancakeMinTopUp)})
	}
	enableWaffo := isWaffoTopUpEnabled()
	if enableWaffo {
		payMethods = appendPaymentMethod(payMethods, map[string]string{"name": "Waffo (Global Payment)", "type": model.PaymentMethodWaffo, "color": "#3B82F6", "min_topup": strconv.Itoa(setting.WaffoMinTopUp)})
	}
	if isBepusdtTopUpEnabled() {
		payMethods = appendPaymentMethod(payMethods, map[string]string{"name": "USDT", "type": model.PaymentMethodBepusdt, "color": "#26A17B", "min_topup": strconv.Itoa(setting.BepusdtMinTopUp)})
	}
	if isOkpayTopUpEnabled() {
		payMethods = appendPaymentMethod(payMethods, map[string]string{"name": "OKPay", "type": model.PaymentMethodOkpay, "color": "#4F46E5", "min_topup": strconv.Itoa(setting.OkpayMinTopUp)})
	}
	data := gin.H{
		"enable_online_topup": isEpayTopUpEnabled(), "enable_stripe_topup": isStripeTopUpEnabled(),
		"enable_creem_topup": isCreemTopUpEnabled(), "enable_waffo_topup": enableWaffo,
		"enable_waffo_pancake_topup": enableWaffoPancake, "enable_bepusdt_topup": isBepusdtTopUpEnabled(),
		"enable_okpay_topup": isOkpayTopUpEnabled(), "enable_redemption": complianceConfirmed,
		"payment_compliance_confirmed":     complianceConfirmed,
		"payment_compliance_terms_version": operation_setting.CurrentComplianceTermsVersion,
		"waffo_pay_methods": func() interface{} {
			if enableWaffo {
				return setting.GetWaffoPayMethods()
			}
			return nil
		}(),
		"creem_products": setting.CreemProducts, "bepusdt_chains": setting.GetBepusdtChains(),
		"bepusdt_min_topup": setting.BepusdtMinTopUp, "okpay_min_topup": setting.OkpayMinTopUp,
		"pay_methods": payMethods, "min_topup": operation_setting.MinTopUp,
		"stripe_min_topup": setting.StripeMinTopUp, "waffo_min_topup": setting.WaffoMinTopUp,
		"waffo_pancake_min_topup": setting.WaffoPancakeMinTopUp,
		"amount_options":          operation_setting.GetPaymentSetting().AmountOptions,
		"discount":                operation_setting.GetPaymentSetting().AmountDiscount, "topup_link": common.TopUpLink,
		"invoice": model.InvoiceConfigSnapshot(),
	}
	common.ApiSuccess(c, data)
}

// normalizeTopUpAmountForStorage keeps Amount in the business quota unit when
// the UI is configured to submit token quantities.
func normalizeTopUpAmountForStorage(amount int64) int64 {
	if operation_setting.GetQuotaDisplayType() != operation_setting.QuotaDisplayTypeTokens {
		return amount
	}
	return decimal.NewFromInt(amount).Div(decimal.NewFromFloat(common.QuotaPerUnit)).IntPart()
}

func appendPaymentMethod(methods []map[string]string, method map[string]string) []map[string]string {
	for _, existing := range methods {
		if existing["type"] == method["type"] {
			return methods
		}
	}
	return append(methods, method)
}

type EpayRequest struct {
	Amount        int64                `json:"amount"`
	PaymentMethod string               `json:"payment_method"`
	PromoCode     string               `json:"promo_code"`
	Invoice       model.InvoiceRequest `json:"invoice"`
}

type AmountRequest struct {
	Amount    int64                `json:"amount"`
	PromoCode string               `json:"promo_code"`
	Invoice   model.InvoiceRequest `json:"invoice"`
}
type RetryTopUpPaymentRequest struct {
	TradeNo   string `json:"trade_no"`
	TradeType string `json:"trade_type"`
}

func GetEpayClient() *epay.Client {
	if operation_setting.PayAddress == "" || operation_setting.EpayId == "" || operation_setting.EpayKey == "" {
		return nil
	}
	withUrl, err := epay.NewClient(&epay.Config{
		PartnerID: operation_setting.EpayId,
		Key:       operation_setting.EpayKey,
	}, operation_setting.PayAddress)
	if err != nil {
		return nil
	}
	return withUrl
}

func topUpAmountDiscount(amount int64, invoice model.InvoiceRequest) float64 {
	if model.ShouldDisableInvoiceDiscount(invoice) {
		return 1
	}
	if discount, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok && discount > 0 {
		return discount
	}
	return 1
}

func calculateTopUpPromoCodeDiscount(promoCode string, invoice model.InvoiceRequest, payMoney float64) (*model.PromoCodeDiscountResult, error) {
	if model.ShouldDisableInvoiceDiscount(invoice) {
		return nil, nil
	}
	return model.CalculatePromoCodeDiscount(promoCode, model.PromoCodeTargetTopUp, 0, payMoney)
}

func getPayMoney(amount int64, group string) float64 {
	return getPayMoneyWithInvoice(amount, group, model.InvoiceRequest{})
}

func getPayMoneyWithInvoice(amount int64, group string, invoice model.InvoiceRequest) float64 {
	dAmount := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount = dAmount.Div(decimal.NewFromFloat(common.QuotaPerUnit))
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}

	return dAmount.
		Mul(decimal.NewFromFloat(operation_setting.Price)).
		Mul(decimal.NewFromFloat(topupGroupRatio)).
		Mul(decimal.NewFromFloat(topUpAmountDiscount(amount, invoice))).
		InexactFloat64()
}

func getMinTopup() int64 {
	minTopup := operation_setting.MinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dMinTopup := decimal.NewFromInt(int64(minTopup))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		minTopup = common.QuotaFromDecimal(dMinTopup.Mul(dQuotaPerUnit))
	}
	return int64(minTopup)
}

func getTopUpQuota(amount int64) (int, error) {
	quota := decimal.NewFromInt(amount)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		quota = decimal.NewFromInt(quota.Div(quotaPerUnit).IntPart()).Mul(quotaPerUnit)
	} else {
		quota = quota.Mul(decimal.NewFromFloat(common.QuotaPerUnit))
	}
	return common.QuotaFromDecimalStrict(quota)
}

func getMaxTopUpAmount() int64 {
	if common.QuotaPerUnit <= 0 {
		return 0
	}
	quotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	maxStoredAmount := decimal.NewFromInt(common.MaxQuota - 1).
		Div(quotaPerUnit).
		Floor()
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		return maxStoredAmount.Add(decimal.NewFromInt(1)).
			Mul(quotaPerUnit).
			Ceil().
			Sub(decimal.NewFromInt(1)).
			IntPart()
	}
	return maxStoredAmount.IntPart()
}

func validateCreditedQuota(quota decimal.Decimal) (int, error) {
	value, err := common.QuotaFromDecimalStrict(quota)
	if err != nil {
		return 0, errors.New("充值额度超出系统可表示范围")
	}
	if value <= 0 {
		return 0, errors.New("充值额度必须大于 0")
	}
	return value, nil
}

func validateTopUpQuota(amount int64) (int, error) {
	quota, err := getTopUpQuota(amount)
	if err == nil && quota > 0 {
		return quota, nil
	}
	maxAmount := getMaxTopUpAmount()
	if maxAmount > 0 && amount > maxAmount {
		return 0, fmt.Errorf("单笔充值数量不能大于 %d", maxAmount)
	}
	return 0, errors.New("充值数量无效")
}

func rejectInvalidCreditedQuota(c *gin.Context, userId int, quota decimal.Decimal) bool {
	creditedQuota, err := validateCreditedQuota(quota)
	if err == nil {
		err = model.ValidateTopUpQuotaCapacity(userId, creditedQuota)
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return true
	}
	return false
}

func rejectInvalidTopUpQuota(c *gin.Context, userId int, amount int64) bool {
	creditedQuota, err := validateTopUpQuota(amount)
	if err == nil {
		err = model.ValidateTopUpQuotaCapacity(userId, creditedQuota)
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": err.Error()})
		return true
	}
	return false
}

func freeTopUpResponse(topUp *model.TopUp, quotaToAdd int, discount *model.PromoCodeDiscountResult) gin.H {
	data := gin.H{"completed": true, "trade_no": topUp.TradeNo, "quota_to_add": quotaToAdd}
	if discount != nil {
		data["discount"] = discount
	}
	return gin.H{"message": "success", "data": data, "completed": true}
}

func RequestEpay(c *gin.Context) {
	var req EpayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Amount < getMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	userId := c.GetInt("id")
	if rejectInvalidTopUpQuota(c, userId, req.Amount) {
		return
	}
	creditedQuota, _ := validateTopUpQuota(req.Amount)
	group, err := model.GetUserGroup(userId, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	originalPayMoney := getPayMoneyWithInvoice(req.Amount, group, req.Invoice)
	businessPayMoney := originalPayMoney
	discount, err := calculateTopUpPromoCodeDiscount(req.PromoCode, req.Invoice, businessPayMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if discount != nil {
		businessPayMoney = discount.PaidAmount
	}
	if businessPayMoney < 0 || !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "充值金额或支付方式无效"})
		return
	}
	invoiceAmounts, err := buildInvoicePaymentAmounts(req.Invoice, model.PaymentProviderEpay, businessPayMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	totalPayMoney := invoiceAmounts.TotalPayment
	client := GetEpayClient()
	if client == nil && totalPayMoney >= 0.01 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "当前管理员未配置支付信息"})
		return
	}

	tradeNo := fmt.Sprintf("USR%dNO%s%d", userId, common.GetRandomString(6), time.Now().Unix())
	topUp := &model.TopUp{
		UserId: userId, Amount: normalizeTopUpAmountForStorage(req.Amount), Money: totalPayMoney,
		CreditedQuota: creditedQuota, AffiliateSourceQuota: creditedQuota,
		TradeNo: tradeNo, PaymentMethod: req.PaymentMethod, PaymentProvider: model.PaymentProviderEpay,
		RequestIP: c.ClientIP(), CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending,
	}
	model.ApplyPromoCodeResultToTopUp(topUp, discount)
	applyInvoiceToTopUp(topUp, invoiceAmounts, originalPayMoney, businessPayMoney, true)
	if err := topUp.Insert(); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	if totalPayMoney < 0.01 {
		completedTopUp, quotaToAdd, _, err := model.CompleteFreeTopUp(tradeNo, model.PaymentProviderEpay)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		c.JSON(http.StatusOK, freeTopUpResponse(completedTopUp, quotaToAdd, discount))
		return
	}

	payMoney := decimal.NewFromFloat(totalPayMoney).Round(2)
	attempt, err := model.CreateTopUpPaymentAttempt(tradeNo, model.PaymentProviderEpay, req.PaymentMethod, payMoney.StringFixed(2), "CNY")
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderEpay, common.TopUpStatusFailed)
		common.ApiError(c, err)
		return
	}
	returnURL, _ := url.Parse(paymentReturnPath("/usage-logs"))
	notifyURL, _ := url.Parse(service.GetCallbackAddress() + "/api/user/epay/notify")
	uri, params, err := client.Purchase(&epay.PurchaseArgs{Type: req.PaymentMethod, ServiceTradeNo: tradeNo, Name: fmt.Sprintf("TUC%d", req.Amount), Money: payMoney.StringFixed(2), Device: epay.PC, NotifyUrl: notifyURL, ReturnUrl: returnURL})
	if err != nil {
		_ = model.MarkTopUpPaymentAttemptLaunchFailed(attempt.Id, err.Error())
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderEpay, common.TopUpStatusFailed)
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	if err := model.MarkTopUpPaymentAttemptLaunched(attempt.Id, ""); err != nil {
		_ = model.MarkTopUpPaymentAttemptLaunchFailed(attempt.Id, err.Error())
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderEpay, common.TopUpStatusFailed)
		common.ApiError(c, err)
		return
	}
	response := gin.H{"message": "success", "data": params, "url": uri}
	addInvoiceFieldsToResponse(response, invoiceAmounts)
	c.JSON(http.StatusOK, response)
}

func retryEpayTopUpPayment(c *gin.Context, topUp *model.TopUp) {
	if !operation_setting.ContainsPayMethod(topUp.PaymentMethod) {
		common.ApiErrorMsg(c, "支付方式不存在")
		return
	}
	client := GetEpayClient()
	if client == nil {
		common.ApiErrorMsg(c, "当前管理员未配置支付信息")
		return
	}
	amount := decimal.NewFromFloat(topUp.Money).Round(2).StringFixed(2)
	attempt, err := model.CreateTopUpPaymentAttempt(topUp.TradeNo, model.PaymentProviderEpay, topUp.PaymentMethod, amount, "CNY")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	returnURL, _ := url.Parse(paymentReturnPath("/usage-logs"))
	notifyURL, _ := url.Parse(service.GetCallbackAddress() + "/api/user/epay/notify")
	uri, params, err := client.Purchase(&epay.PurchaseArgs{Type: topUp.PaymentMethod, ServiceTradeNo: topUp.TradeNo, Name: fmt.Sprintf("TUC%d", topUp.Amount), Money: amount, Device: epay.PC, NotifyUrl: notifyURL, ReturnUrl: returnURL})
	if err != nil {
		_ = model.MarkTopUpPaymentAttemptLaunchFailed(attempt.Id, err.Error())
		_ = model.UpdatePendingTopUpStatus(topUp.TradeNo, model.PaymentProviderEpay, common.TopUpStatusFailed)
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	if err := model.MarkTopUpPaymentAttemptLaunched(attempt.Id, ""); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": params, "url": uri})
}

func retryBepusdtTopUpPayment(c *gin.Context, topUp *model.TopUp, requestedTradeType string) {
	tradeType := strings.TrimSpace(requestedTradeType)
	if tradeType == "" && len(setting.GetBepusdtChains()) != 0 {
		tradeType = setting.GetBepusdtChains()[0].TradeType
	}
	if !isValidBepusdtTradeType(tradeType) {
		common.ApiErrorMsg(c, "支付链无效")
		return
	}
	amount := decimal.NewFromFloat(topUp.Money).Round(2)
	attempt, err := model.CreateTopUpPaymentAttempt(topUp.TradeNo, model.PaymentProviderBepusdt, model.PaymentMethodBepusdt, amount.StringFixed(2), "CNY")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	result, err := createBepusdtTransactionDetails(c, topUp.TradeNo, amount.InexactFloat64(), tradeType, service.GetCallbackAddress()+"/api/bepusdt/notify", paymentReturnPath("/usage-logs"))
	if err != nil {
		_ = model.MarkTopUpPaymentAttemptLaunchFailed(attempt.Id, err.Error())
		_ = model.UpdatePendingTopUpStatus(topUp.TradeNo, model.PaymentProviderBepusdt, common.TopUpStatusFailed)
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	if err := model.MarkTopUpPaymentAttemptLaunched(attempt.Id, result.Data.TradeId); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{"payment_url": result.Data.PaymentUrl, "trade_no": topUp.TradeNo}})
}

func retryOkpayTopUpPayment(c *gin.Context, topUp *model.TopUp) {
	payment := getOkpayPaymentAmountFromFiat(topUp.Money)
	amount := decimal.NewFromFloat(payment.CoinAmount).StringFixed(8)
	attempt, err := model.CreateTopUpPaymentAttempt(topUp.TradeNo, model.PaymentProviderOkpay, model.PaymentMethodOkpay, amount, payment.Coin)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	link, err := createOkpayPaymentLink(c, topUp.TradeNo, payment, "TopUp-"+topUp.TradeNo, service.GetCallbackAddress()+"/api/okpay/notify", paymentReturnPath("/usage-logs"))
	if err != nil {
		_ = model.MarkTopUpPaymentAttemptLaunchFailed(attempt.Id, err.Error())
		_ = model.UpdatePendingTopUpStatus(topUp.TradeNo, model.PaymentProviderOkpay, common.TopUpStatusFailed)
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	if err := model.MarkTopUpPaymentAttemptLaunched(attempt.Id, link.ProviderOrderId); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{"payment_url": link.PaymentUrl, "trade_no": topUp.TradeNo, "provider_order_id": link.ProviderOrderId, "amount": link.Amount, "coin": payment.Coin}})
}

func RetryTopUpPayment(c *gin.Context) {
	var req RetryTopUpPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}
	tradeNo := strings.TrimSpace(req.TradeNo)
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)
	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil || topUp.UserId != c.GetInt("id") {
		common.ApiErrorMsg(c, "充值订单不存在")
		return
	}
	if topUp.Status == common.TopUpStatusSuccess || topUp.Money < 0.01 {
		common.ApiErrorMsg(c, "订单不可重新支付")
		return
	}
	switch topUp.PaymentProvider {
	case model.PaymentProviderEpay:
		retryEpayTopUpPayment(c, topUp)
	case model.PaymentProviderStripe:
		retryStripeTopUpPayment(c, topUp)
	case model.PaymentProviderBepusdt:
		retryBepusdtTopUpPayment(c, topUp, req.TradeType)
	case model.PaymentProviderOkpay:
		retryOkpayTopUpPayment(c, topUp)
	default:
		common.ApiErrorMsg(c, "该支付渠道暂不支持重新支付")
	}
}

// tradeNo lock
var orderLocks sync.Map
var createLock sync.Mutex

// refCountedMutex 带引用计数的互斥锁，确保最后一个使用者才从 map 中删除
type refCountedMutex struct {
	mu       sync.Mutex
	refCount int
}

// LockOrder 尝试对给定订单号加锁
func LockOrder(tradeNo string) {
	createLock.Lock()
	var rcm *refCountedMutex
	if v, ok := orderLocks.Load(tradeNo); ok {
		rcm = v.(*refCountedMutex)
	} else {
		rcm = &refCountedMutex{}
		orderLocks.Store(tradeNo, rcm)
	}
	rcm.refCount++
	createLock.Unlock()
	rcm.mu.Lock()
}

// UnlockOrder 释放给定订单号的锁
func UnlockOrder(tradeNo string) {
	v, ok := orderLocks.Load(tradeNo)
	if !ok {
		return
	}
	rcm := v.(*refCountedMutex)
	rcm.mu.Unlock()

	createLock.Lock()
	rcm.refCount--
	if rcm.refCount == 0 {
		orderLocks.Delete(tradeNo)
	}
	createLock.Unlock()
}

func EpayNotify(c *gin.Context) {
	if !isEpayWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	var params map[string]string

	if c.Request.Method == "POST" {
		// POST 请求：从 POST body 解析参数
		if err := c.Request.ParseForm(); err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook POST 表单解析失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		params = lo.Reduce(lo.Keys(c.Request.PostForm), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.PostForm.Get(t)
			return r
		}, map[string]string{})
	} else {
		// GET 请求：从 URL Query 解析参数
		params = lo.Reduce(lo.Keys(c.Request.URL.Query()), func(r map[string]string, t string, i int) map[string]string {
			r[t] = c.Request.URL.Query().Get(t)
			return r
		}, map[string]string{})
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 收到请求 path=%q client_ip=%s method=%s params=%q", c.Request.RequestURI, c.ClientIP(), c.Request.Method, common.GetJsonString(params)))

	if len(params) == 0 {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 参数为空 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	client := GetEpayClient()
	if client == nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 client 未初始化 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		_, err := c.Writer.Write([]byte("fail"))
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		}
		return
	}
	verifyInfo, err := client.Verify(params)
	if err != nil || !verifyInfo.VerifyStatus {
		if _, writeErr := c.Writer.Write([]byte("fail")); writeErr != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), writeErr.Error()))
		}
		if err != nil {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签失败 path=%q client_ip=%s verify_error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		} else {
			logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签失败 path=%q client_ip=%s verify_status=false", c.Request.RequestURI, c.ClientIP()))
		}
		return
	}
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 验签成功 trade_no=%s callback_type=%s trade_status=%s client_ip=%s verify_info=%q", verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.TradeStatus, c.ClientIP(), common.GetJsonString(verifyInfo)))

	if verifyInfo.TradeStatus == epay.StatusTradeSuccess {
		tradeNo := verifyInfo.ServiceTradeNo
		callbackAmount := strings.TrimSpace(params["money"])
		if callbackAmount == "" {
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		LockOrder(tradeNo)
		defer UnlockOrder(tradeNo)
		attempt, attemptErr := model.ResolveTopUpPaymentAttempt(model.PaymentProviderEpay, tradeNo, "")
		var alreadyDone bool
		if attemptErr == nil {
			if err := model.ValidateTopUpPaymentAttemptSnapshot(attempt, model.PaymentProviderEpay, "", callbackAmount, "CNY", decimal.Zero); err != nil {
				logger.LogWarn(c.Request.Context(), fmt.Sprintf("易支付 回调金额不匹配 trade_no=%s error=%q", tradeNo, err.Error()))
				_, _ = c.Writer.Write([]byte("fail"))
				return
			}
			alreadyDone, err = model.CompleteTopUpPaymentAttempt(attempt.Id, tradeNo, model.PaymentProviderEpay, verifyInfo.Type, c.ClientIP())
		} else if errors.Is(attemptErr, model.ErrTopUpPaymentAttemptNotFound) {
			topUp := model.GetTopUpByTradeNo(tradeNo)
			if topUp == nil {
				_, _ = c.Writer.Write([]byte("fail"))
				return
			}
			expected := decimal.NewFromFloat(topUp.Money).Round(2)
			if strings.TrimSpace(topUp.ProviderAmount) != "" {
				var expectedErr error
				expected, expectedErr = decimal.NewFromString(strings.TrimSpace(topUp.ProviderAmount))
				if expectedErr != nil {
					_, _ = c.Writer.Write([]byte("fail"))
					return
				}
			}
			actual, parseErr := decimal.NewFromString(callbackAmount)
			if !model.AllowLegacyTopUpCallback(topUp, model.PaymentProviderEpay) || (topUp.ProviderCurrency != "" && !strings.EqualFold(topUp.ProviderCurrency, "CNY")) || parseErr != nil || !actual.Equal(expected) {
				_, _ = c.Writer.Write([]byte("fail"))
				return
			}
			alreadyDone, err = model.RechargeEpay(tradeNo, verifyInfo.Type, c.ClientIP())
		} else {
			err = attemptErr
		}
		if err != nil {
			logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 充值处理失败 trade_no=%s client_ip=%s error=%q", tradeNo, c.ClientIP(), err.Error()))
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		if alreadyDone {
			logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 重复回调幂等忽略 trade_no=%s", tradeNo))
		}
	} else {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("易支付 webhook 忽略事件 trade_no=%s callback_type=%s trade_status=%s client_ip=%s verify_info=%q", verifyInfo.ServiceTradeNo, verifyInfo.Type, verifyInfo.TradeStatus, c.ClientIP(), common.GetJsonString(verifyInfo)))
	}
	if _, writeErr := c.Writer.Write([]byte("success")); writeErr != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("易支付 webhook 响应写入失败 trade_no=%s client_ip=%s error=%q", verifyInfo.ServiceTradeNo, c.ClientIP(), writeErr.Error()))
	}
}

func RequestAmount(c *gin.Context) {
	var req AmountRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if req.Amount < getMinTopup() {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}
	userId := c.GetInt("id")
	if rejectInvalidTopUpQuota(c, userId, req.Amount) {
		return
	}
	group, err := model.GetUserGroup(userId, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	businessPayMoney := getPayMoneyWithInvoice(req.Amount, group, req.Invoice)
	discount, err := calculateTopUpPromoCodeDiscount(req.PromoCode, req.Invoice, businessPayMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if discount != nil {
		businessPayMoney = discount.PaidAmount
	}
	if businessPayMoney < 0 {
		common.ApiErrorMsg(c, "充值金额无效")
		return
	}
	invoiceAmounts, err := buildInvoicePaymentPreviewAmounts(req.Invoice, model.PaymentProviderEpay, businessPayMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	response := gin.H{"message": "success", "data": strconv.FormatFloat(invoiceAmounts.TotalPayment, 'f', 2, 64)}
	if discount != nil {
		response["discount"] = discount
	}
	addInvoiceFieldsToResponse(response, invoiceAmounts)
	c.JSON(http.StatusOK, response)
}

func GetUserTopUps(c *gin.Context) {
	userId := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchUserTopUps(userId, keyword, pageInfo)
	} else {
		topups, total, err = model.GetUserTopUps(userId, pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

// GetAllTopUps 管理员获取全平台充值记录
func GetAllTopUps(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	keyword := c.Query("keyword")

	var (
		topups []*model.TopUp
		total  int64
		err    error
	)
	if keyword != "" {
		topups, total, err = model.SearchAllTopUps(keyword, pageInfo)
	} else {
		topups, total, err = model.GetAllTopUps(pageInfo)
	}
	if err != nil {
		common.ApiError(c, err)
		return
	}

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(topups)
	common.ApiSuccess(c, pageInfo)
}

type AdminCompleteTopupRequest struct {
	TradeNo string `json:"trade_no"`
}

// AdminCompleteTopUp 管理员补单接口
func AdminCompleteTopUp(c *gin.Context) {
	var req AdminCompleteTopupRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.TradeNo == "" {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	// 订单级互斥，防止并发补单
	LockOrder(req.TradeNo)
	defer UnlockOrder(req.TradeNo)

	if err := model.ManualCompleteTopUp(req.TradeNo, c.ClientIP()); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
