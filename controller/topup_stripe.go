package controller

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/checkout/session"
	"github.com/stripe/stripe-go/v81/webhook"
	"github.com/thanhpk/randstr"
)

var stripeAdaptor = &StripeAdaptor{}

// StripePayRequest represents a payment request for Stripe checkout.
type StripePayRequest struct {
	Amount        int64                `json:"amount"`
	PaymentMethod string               `json:"payment_method"`
	SuccessURL    string               `json:"success_url,omitempty"`
	CancelURL     string               `json:"cancel_url,omitempty"`
	PromoCode     string               `json:"promo_code"`
	Invoice       model.InvoiceRequest `json:"invoice"`
}

type StripeAdaptor struct {
}
type stripeCheckoutResult struct {
	Id  string
	URL string
}

func (*StripeAdaptor) RequestAmount(c *gin.Context, req *StripePayRequest) {
	if req.Amount < getStripeMinTopup() || req.Amount > 10000 {
		common.ApiErrorMsg(c, "充值数量无效")
		return
	}
	userId := c.GetInt("id")
	group, err := model.GetUserGroup(userId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if rejectInvalidCreditedQuota(c, userId, getStripeCreditedQuota(req.Amount, group)) {
		return
	}
	businessPayMoney := getStripePayMoneyWithInvoice(float64(req.Amount), group, req.Invoice)
	discount, err := calculateTopUpPromoCodeDiscount(req.PromoCode, req.Invoice, businessPayMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if discount != nil {
		businessPayMoney = discount.PaidAmount
	}
	invoiceAmounts, err := buildInvoicePaymentPreviewAmounts(req.Invoice, model.PaymentProviderStripe, businessPayMoney)
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

func (*StripeAdaptor) RequestPay(c *gin.Context, req *StripePayRequest) {
	if req.PaymentMethod != model.PaymentMethodStripe || req.Amount < getStripeMinTopup() || req.Amount > 10000 {
		common.ApiErrorMsg(c, "充值数量或支付渠道无效")
		return
	}
	if (req.SuccessURL != "" && common.ValidateRedirectURL(req.SuccessURL) != nil) || (req.CancelURL != "" && common.ValidateRedirectURL(req.CancelURL) != nil) {
		c.JSON(http.StatusBadRequest, gin.H{"message": "重定向URL不在可信任域名列表中", "data": ""})
		return
	}
	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil || user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}
	creditedQuota, err := validateCreditedQuota(getStripeCreditedQuota(req.Amount, user.Group))
	if err != nil || model.ValidateTopUpQuotaCapacity(userId, creditedQuota) != nil {
		common.ApiErrorMsg(c, "充值额度超出限制")
		return
	}
	originalPayMoney := getStripePayMoneyWithInvoice(float64(req.Amount), user.Group, req.Invoice)
	businessPayMoney := originalPayMoney
	discount, err := calculateTopUpPromoCodeDiscount(req.PromoCode, req.Invoice, businessPayMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if discount != nil {
		businessPayMoney = discount.PaidAmount
	}
	invoiceAmounts, err := buildInvoicePaymentAmounts(req.Invoice, model.PaymentProviderStripe, businessPayMoney)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	totalPayMoney := invoiceAmounts.TotalPayment
	reference := fmt.Sprintf("new-api-ref-%d-%d-%s", user.Id, time.Now().UnixMilli(), randstr.String(4))
	tradeNo := "ref_" + common.Sha1([]byte(reference))
	topUp := &model.TopUp{UserId: userId, Amount: req.Amount, Money: totalPayMoney, CreditedQuota: creditedQuota, AffiliateSourceQuota: creditedQuota, TradeNo: tradeNo, PaymentMethod: model.PaymentMethodStripe, PaymentProvider: model.PaymentProviderStripe, RequestIP: c.ClientIP(), CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending}
	model.ApplyPromoCodeResultToTopUp(topUp, discount)
	applyInvoiceToTopUp(topUp, invoiceAmounts, originalPayMoney, businessPayMoney, true)
	if err := topUp.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	if totalPayMoney < 0.01 {
		completed, quotaToAdd, _, err := model.CompleteFreeTopUp(tradeNo, model.PaymentProviderStripe)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		c.JSON(http.StatusOK, freeTopUpResponse(completed, quotaToAdd, discount))
		return
	}
	payMoney := decimal.NewFromFloat(totalPayMoney).Round(2)
	amountMinor := payMoney.Mul(decimal.NewFromInt(100)).Round(0).StringFixed(0)
	attempt, err := model.CreateTopUpPaymentAttempt(tradeNo, model.PaymentProviderStripe, model.PaymentMethodStripe, amountMinor, "USD")
	if err != nil {
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderStripe, common.TopUpStatusFailed)
		common.ApiError(c, err)
		return
	}
	allowPromotions := setting.StripePromotionCodesEnabled && !model.ShouldDisableInvoiceDiscount(req.Invoice)
	checkout, err := genStripeLink(tradeNo, user.StripeCustomer, user.Email, req.Amount, payMoney.InexactFloat64(), req.SuccessURL, req.CancelURL, allowPromotions)
	if err != nil {
		_ = model.MarkTopUpPaymentAttemptLaunchFailed(attempt.Id, err.Error())
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderStripe, common.TopUpStatusFailed)
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	if err := model.MarkTopUpPaymentAttemptLaunched(attempt.Id, checkout.Id); err != nil {
		_ = model.MarkTopUpPaymentAttemptLaunchFailed(attempt.Id, err.Error())
		_ = model.UpdatePendingTopUpStatus(tradeNo, model.PaymentProviderStripe, common.TopUpStatusFailed)
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{"pay_link": checkout.URL, "invoice": invoiceAmounts}})
}
func retryStripePromotionCodesAllowed(topUp *model.TopUp) bool {
	return topUp != nil && !topUp.InvoiceDiscountDisabled
}

func retryStripeTopUpPayment(c *gin.Context, topUp *model.TopUp) {
	user, err := model.GetUserById(topUp.UserId, false)
	if err != nil || user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}
	payMoney := decimal.NewFromFloat(topUp.Money).Round(2)
	amountMinor := payMoney.Mul(decimal.NewFromInt(100)).Round(0).StringFixed(0)
	attempt, err := model.CreateTopUpPaymentAttempt(topUp.TradeNo, model.PaymentProviderStripe, model.PaymentMethodStripe, amountMinor, "USD")
	if err != nil {
		common.ApiError(c, err)
		return
	}
	checkout, err := genStripeLink(topUp.TradeNo, user.StripeCustomer, user.Email, topUp.Amount, payMoney.InexactFloat64(), "", "", setting.StripePromotionCodesEnabled && retryStripePromotionCodesAllowed(topUp))
	if err != nil {
		_ = model.MarkTopUpPaymentAttemptLaunchFailed(attempt.Id, err.Error())
		_ = model.UpdatePendingTopUpStatus(topUp.TradeNo, model.PaymentProviderStripe, common.TopUpStatusFailed)
		common.ApiErrorMsg(c, "拉起支付失败")
		return
	}
	if err := model.MarkTopUpPaymentAttemptLaunched(attempt.Id, checkout.Id); err != nil {
		common.ApiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success", "data": gin.H{"pay_link": checkout.URL}})
}

func RequestStripeAmount(c *gin.Context) {
	var req StripePayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	stripeAdaptor.RequestAmount(c, &req)
}

func RequestStripePay(c *gin.Context) {
	var req StripePayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	stripeAdaptor.RequestPay(c, &req)
}

func StripeWebhook(c *gin.Context) {
	ctx := c.Request.Context()
	if !isStripeWebhookEnabled() {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe webhook 读取请求体失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}

	signature := c.GetHeader("Stripe-Signature")
	logger.LogInfo(ctx, fmt.Sprintf("Stripe webhook 收到请求 path=%q client_ip=%s signature=%q body=%q", c.Request.RequestURI, c.ClientIP(), signature, string(payload)))
	event, err := webhook.ConstructEventWithOptions(payload, signature, setting.StripeWebhookSecret, webhook.ConstructEventOptions{
		IgnoreAPIVersionMismatch: true,
	})

	if err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe webhook 验签失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	callerIp := c.ClientIP()
	logger.LogInfo(ctx, fmt.Sprintf("Stripe webhook 验签成功 event_type=%s client_ip=%s path=%q", string(event.Type), callerIp, c.Request.RequestURI))
	switch event.Type {
	case stripe.EventTypeCheckoutSessionCompleted:
		sessionCompleted(ctx, event, callerIp)
	case stripe.EventTypeCheckoutSessionExpired:
		sessionExpired(ctx, event)
	case stripe.EventTypeCheckoutSessionAsyncPaymentSucceeded:
		sessionAsyncPaymentSucceeded(ctx, event, callerIp)
	case stripe.EventTypeCheckoutSessionAsyncPaymentFailed:
		sessionAsyncPaymentFailed(ctx, event, callerIp)
	default:
		logger.LogInfo(ctx, fmt.Sprintf("Stripe webhook 忽略事件 event_type=%s client_ip=%s", string(event.Type), callerIp))
	}

	c.Status(http.StatusOK)
}

func sessionCompleted(ctx context.Context, event stripe.Event, callerIp string) {
	customerId := event.GetObjectValue("customer")
	referenceId := event.GetObjectValue("client_reference_id")
	status := event.GetObjectValue("status")
	if "complete" != status {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe checkout.completed 状态异常，忽略处理 trade_no=%s status=%s client_ip=%s", referenceId, status, callerIp))
		return
	}

	paymentStatus := event.GetObjectValue("payment_status")
	if paymentStatus != "paid" {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe Checkout 支付未完成，等待异步结果 trade_no=%s payment_status=%s client_ip=%s", referenceId, paymentStatus, callerIp))
		return
	}

	fulfillOrder(ctx, event, referenceId, customerId, callerIp)
}

// sessionAsyncPaymentSucceeded handles delayed payment methods (bank transfer, SEPA, etc.)
// that confirm payment after the checkout session completes.
func sessionAsyncPaymentSucceeded(ctx context.Context, event stripe.Event, callerIp string) {
	customerId := event.GetObjectValue("customer")
	referenceId := event.GetObjectValue("client_reference_id")
	logger.LogInfo(ctx, fmt.Sprintf("Stripe 异步支付成功 trade_no=%s client_ip=%s", referenceId, callerIp))

	fulfillOrder(ctx, event, referenceId, customerId, callerIp)
}

// sessionAsyncPaymentFailed marks orders as failed when delayed payment methods
// ultimately fail (e.g. bank transfer not received, SEPA rejected).
func sessionAsyncPaymentFailed(ctx context.Context, event stripe.Event, callerIp string) {
	referenceId := event.GetObjectValue("client_reference_id")
	logger.LogWarn(ctx, fmt.Sprintf("Stripe 异步支付失败 trade_no=%s client_ip=%s", referenceId, callerIp))

	if len(referenceId) == 0 {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 异步支付失败事件缺少订单号 client_ip=%s", callerIp))
		return
	}

	err := model.UpdatePendingTopUpStatus(referenceId, model.PaymentProviderStripe, common.TopUpStatusFailed)
	switch {
	case errors.Is(err, model.ErrTopUpNotFound):
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 异步支付失败但本地订单不存在 trade_no=%s client_ip=%s", referenceId, callerIp))
	case errors.Is(err, model.ErrPaymentMethodMismatch):
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 异步支付失败但订单支付网关不匹配 trade_no=%s client_ip=%s", referenceId, callerIp))
	case errors.Is(err, model.ErrTopUpStatusInvalid):
		logger.LogInfo(ctx, fmt.Sprintf("Stripe 异步支付失败但订单已离开 pending，忽略处理 trade_no=%s client_ip=%s", referenceId, callerIp))
	case err != nil:
		logger.LogError(ctx, fmt.Sprintf("Stripe 标记充值订单失败状态失败 trade_no=%s client_ip=%s error=%q", referenceId, callerIp, err.Error()))
	default:
		logger.LogInfo(ctx, fmt.Sprintf("Stripe 充值订单已标记为失败 trade_no=%s client_ip=%s", referenceId, callerIp))
	}
}

// fulfillOrder is the shared logic for crediting quota after payment is confirmed.
func fulfillOrder(ctx context.Context, event stripe.Event, referenceId string, customerId string, callerIp string) {
	if len(referenceId) == 0 {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 完成订单时缺少订单号 client_ip=%s", callerIp))
		return
	}

	LockOrder(referenceId)
	defer UnlockOrder(referenceId)
	payload := map[string]any{
		"customer":     customerId,
		"amount_total": event.GetObjectValue("amount_total"),
		"currency":     strings.ToUpper(event.GetObjectValue("currency")),
		"event_type":   string(event.Type),
	}
	if order := model.GetSubscriptionOrderByTradeNo(referenceId); order != nil {
		sessionId := strings.TrimSpace(event.GetObjectValue("id"))
		amountTotal := strings.TrimSpace(event.GetObjectValue("amount_total"))
		currency := strings.TrimSpace(event.GetObjectValue("currency"))
		if !validateSubscriptionProviderSnapshot(order, model.PaymentProviderStripe, sessionId, amountTotal, currency, true) {
			logger.LogWarn(ctx, fmt.Sprintf("Stripe 订阅回调快照不匹配 trade_no=%s", referenceId))
			return
		}
		if err := model.CompleteSubscriptionOrder(referenceId, common.GetJsonString(payload), model.PaymentProviderStripe, ""); err != nil {
			logger.LogError(ctx, fmt.Sprintf("Stripe 订阅订单处理失败 trade_no=%s error=%q", referenceId, err.Error()))
		}
		return
	}

	sessionId := strings.TrimSpace(event.GetObjectValue("id"))
	amountTotal := strings.TrimSpace(event.GetObjectValue("amount_total"))
	currency := strings.ToUpper(strings.TrimSpace(event.GetObjectValue("currency")))
	attempt, err := model.ResolveTopUpPaymentAttempt(model.PaymentProviderStripe, referenceId, sessionId)
	if err == nil {
		if err := model.ValidateTopUpPaymentAttemptSnapshot(attempt, model.PaymentProviderStripe, sessionId, amountTotal, currency, decimal.Zero); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("Stripe 回调快照不匹配 trade_no=%s session_id=%s error=%q", referenceId, sessionId, err.Error()))
			return
		}
		if err := model.BindTopUpPaymentAttemptProviderOrder(attempt.Id, sessionId); err != nil {
			logger.LogWarn(ctx, fmt.Sprintf("Stripe 回调会话绑定失败 trade_no=%s session_id=%s error=%q", referenceId, sessionId, err.Error()))
			return
		}
		_, err = model.CompleteStripeTopUpPaymentAttempt(attempt.Id, referenceId, customerId, callerIp)
	} else if errors.Is(err, model.ErrTopUpPaymentAttemptNotFound) {
		topUp := model.GetTopUpByTradeNo(referenceId)
		expectedMinor := decimal.Zero
		if topUp != nil {
			expectedMinor = decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromInt(100)).Round(0)
			if strings.TrimSpace(topUp.ProviderOrderId) != "" && topUp.ProviderOrderId != sessionId {
				return
			}
			if strings.TrimSpace(topUp.ProviderAmount) != "" {
				var expectedErr error
				expectedMinor, expectedErr = decimal.NewFromString(strings.TrimSpace(topUp.ProviderAmount))
				if expectedErr != nil {
					return
				}
			}
		}
		actualMinor, parseErr := decimal.NewFromString(amountTotal)
		if !model.AllowLegacyTopUpCallback(topUp, model.PaymentProviderStripe) || (topUp.ProviderCurrency != "" && !strings.EqualFold(topUp.ProviderCurrency, "USD")) || parseErr != nil || !actualMinor.Equal(expectedMinor) || currency != "USD" {
			return
		}
		err = model.Recharge(referenceId, customerId, callerIp)
	}
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe 充值处理失败 trade_no=%s session_id=%s error=%q", referenceId, sessionId, err.Error()))
		return
	}
	total, _ := strconv.ParseFloat(amountTotal, 64)
	logger.LogInfo(ctx, fmt.Sprintf("Stripe 充值成功 trade_no=%s amount_total=%.2f currency=%s event_type=%s client_ip=%s", referenceId, total/100, currency, string(event.Type), callerIp))
}

func sessionExpired(ctx context.Context, event stripe.Event) {
	referenceId := event.GetObjectValue("client_reference_id")
	status := event.GetObjectValue("status")
	if "expired" != status {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe checkout.expired 状态异常，忽略处理 trade_no=%s status=%s", referenceId, status))
		return
	}

	if len(referenceId) == 0 {
		logger.LogWarn(ctx, "Stripe checkout.expired 缺少订单号")
		return
	}

	// Subscription order expiration
	LockOrder(referenceId)
	defer UnlockOrder(referenceId)
	if err := model.ExpireSubscriptionOrder(referenceId, model.PaymentProviderStripe); err == nil {
		logger.LogInfo(ctx, fmt.Sprintf("Stripe 订阅订单已过期 trade_no=%s", referenceId))
		return
	} else if err != nil && !errors.Is(err, model.ErrSubscriptionOrderNotFound) {
		logger.LogError(ctx, fmt.Sprintf("Stripe 订阅订单过期处理失败 trade_no=%s error=%q", referenceId, err.Error()))
		return
	}

	err := model.UpdatePendingTopUpStatus(referenceId, model.PaymentProviderStripe, common.TopUpStatusExpired)
	if errors.Is(err, model.ErrTopUpNotFound) {
		logger.LogWarn(ctx, fmt.Sprintf("Stripe 充值订单不存在，无法标记过期 trade_no=%s", referenceId))
		return
	}
	if err != nil {
		logger.LogError(ctx, fmt.Sprintf("Stripe 充值订单过期处理失败 trade_no=%s error=%q", referenceId, err.Error()))
		return
	}

	logger.LogInfo(ctx, fmt.Sprintf("Stripe 充值订单已过期 trade_no=%s", referenceId))
}

// genStripeLink generates a Stripe Checkout session URL for payment.
// It creates a new checkout session with the specified parameters and returns the payment URL.
//
// Parameters:
//   - referenceId: unique reference identifier for the transaction
//   - customerId: existing Stripe customer ID (empty string if new customer)
//   - email: customer email address for new customer creation
//   - amount: quantity of units to purchase
//   - successURL: custom URL to redirect after successful payment (empty for default)
//   - cancelURL: custom URL to redirect when payment is canceled (empty for default)
//
// Returns the checkout session URL or an error if the session creation fails.
func genStripeLink(referenceId, customerId, email string, amount int64, payMoney float64, successURL, cancelURL string, allowPromotionCodes ...bool) (*stripeCheckoutResult, error) {
	if !strings.HasPrefix(setting.StripeApiSecret, "sk_") && !strings.HasPrefix(setting.StripeApiSecret, "rk_") {
		return nil, fmt.Errorf("无效的Stripe API密钥")
	}
	unitAmount := decimal.NewFromFloat(payMoney).Mul(decimal.NewFromInt(100)).Round(0).IntPart()
	if unitAmount <= 0 {
		return nil, errors.New("无效的支付金额")
	}
	stripe.Key = setting.StripeApiSecret
	if successURL == "" {
		successURL = paymentReturnPath("/usage-logs")
	}
	if cancelURL == "" {
		cancelURL = paymentReturnPath("/wallet")
	}
	params := &stripe.CheckoutSessionParams{
		ClientReferenceID: stripe.String(referenceId), SuccessURL: stripe.String(successURL), CancelURL: stripe.String(cancelURL),
		LineItems: []*stripe.CheckoutSessionLineItemParams{{
			Quantity: stripe.Int64(1),
			PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
				Currency: stripe.String("usd"), UnitAmount: stripe.Int64(unitAmount),
				ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{Name: stripe.String(fmt.Sprintf("Recharge %d credits", amount))},
			},
		}},
		Mode: stripe.String(string(stripe.CheckoutSessionModePayment)), AllowPromotionCodes: stripe.Bool(len(allowPromotionCodes) > 0 && allowPromotionCodes[0]),
	}
	if customerId == "" {
		if email != "" {
			params.CustomerEmail = stripe.String(email)
		}
		params.CustomerCreation = stripe.String(string(stripe.CheckoutSessionCustomerCreationAlways))
	} else {
		params.Customer = stripe.String(customerId)
	}
	result, err := session.New(params)
	if err != nil {
		return nil, err
	}
	if result.ID == "" || result.URL == "" {
		return nil, errors.New("Stripe 未返回 Checkout Session")
	}
	return &stripeCheckoutResult{Id: result.ID, URL: result.URL}, nil
}

func GetChargedAmount(count float64, user model.User) float64 {
	topUpGroupRatio := common.GetTopupGroupRatio(user.Group)
	if topUpGroupRatio == 0 {
		topUpGroupRatio = 1
	}

	return count * topUpGroupRatio
}

func getStripeCreditedQuota(amount int64, group string) decimal.Decimal {
	topUpGroupRatio := common.GetTopupGroupRatio(group)
	if topUpGroupRatio == 0 {
		topUpGroupRatio = 1
	}
	return decimal.NewFromInt(amount).
		Mul(decimal.NewFromFloat(topUpGroupRatio)).
		Mul(decimal.NewFromFloat(common.QuotaPerUnit))
}

func getStripePayMoney(amount float64, group string) float64 {
	return getStripePayMoneyWithInvoice(amount, group, model.InvoiceRequest{})
}

func getStripePayMoneyWithInvoice(amount float64, group string, invoice model.InvoiceRequest) float64 {
	originalAmount := amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		amount /= common.QuotaPerUnit
	}
	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}
	return amount * setting.StripeUnitPrice * topupGroupRatio * topUpAmountDiscount(int64(originalAmount), invoice)
}

func getStripeMinTopup() int64 {
	minTopup := setting.StripeMinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		minTopup = minTopup * int(common.QuotaPerUnit)
	}
	return int64(minTopup)
}
