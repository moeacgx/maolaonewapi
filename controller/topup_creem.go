package controller

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/thanhpk/randstr"
)

const CreemSignatureHeader = "creem-signature"

var creemAdaptor = &CreemAdaptor{}

// 生成HMAC-SHA256签名
func generateCreemSignature(payload string, secret string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(payload))
	return hex.EncodeToString(h.Sum(nil))
}

// 验证Creem webhook签名
func verifyCreemSignature(payload string, signature string, secret string) bool {
	if secret == "" {
		logger.LogWarn(context.Background(), fmt.Sprintf("Creem webhook secret 未配置 test_mode=%t signature=%q body=%q", setting.CreemTestMode, signature, payload))
		if setting.CreemTestMode {
			logger.LogInfo(context.Background(), fmt.Sprintf("Creem webhook 验签已跳过 reason=test_mode signature=%q body=%q", signature, payload))
			return true
		}
		return false
	}

	expectedSignature := generateCreemSignature(payload, secret)
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

type CreemPayRequest struct {
	ProductId     string               `json:"product_id"`
	PaymentMethod string               `json:"payment_method"`
	PromoCode     string               `json:"promo_code"`
	Invoice       model.InvoiceRequest `json:"invoice"`
}

type CreemProduct struct {
	ProductId string  `json:"productId"`
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	Currency  string  `json:"currency"`
	Quota     int64   `json:"quota"`
}

type CreemAdaptor struct {
}

func (*CreemAdaptor) RequestPay(c *gin.Context, req *CreemPayRequest) {
	if req.PaymentMethod != model.PaymentMethodCreem {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "不支持的支付渠道"})
		return
	}

	if req.ProductId == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "请选择产品"})
		return
	}
	if strings.TrimSpace(req.PromoCode) != "" {
		common.ApiErrorMsg(c, "Creem 固定产品暂不支持优惠码，请选择其他支付方式")
		return
	}
	if req.Invoice.Required {
		common.ApiErrorMsg(c, "Creem 固定产品暂不支持开发票，请选择其他支付方式")
		return
	}

	// 解析产品列表
	var products []CreemProduct
	err := common.UnmarshalJsonStr(setting.CreemProducts, &products)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 产品配置解析失败 user_id=%d error=%q", c.GetInt("id"), err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "产品配置错误"})
		return
	}

	// 查找对应的产品
	var selectedProduct *CreemProduct
	for _, product := range products {
		if product.ProductId == req.ProductId {
			selectedProduct = &product
			break
		}
	}

	if selectedProduct == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "产品不存在"})
		return
	}
	providerCurrency := strings.ToUpper(strings.TrimSpace(selectedProduct.Currency))
	providerAmount := decimal.NewFromFloat(selectedProduct.Price).Mul(decimal.NewFromInt(100)).Round(0).StringFixed(0)
	if providerCurrency == "" || selectedProduct.Price <= 0 {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "产品支付快照配置错误"})
		return
	}

	id := c.GetInt("id")
	if rejectInvalidCreditedQuota(c, id, decimal.NewFromInt(selectedProduct.Quota)) {
		return
	}
	creditedQuota, _ := validateCreditedQuota(decimal.NewFromInt(selectedProduct.Quota))

	user, err := model.GetUserById(id, false)
	if err != nil || user == nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "用户不存在"})
		return
	}

	// 生成唯一的订单引用ID
	reference := fmt.Sprintf("creem-api-ref-%d-%d-%s", user.Id, time.Now().UnixMilli(), randstr.String(4))
	referenceId := "ref_" + common.Sha1([]byte(reference))

	// 先创建订单记录，使用产品配置的金额和充值额度
	topUp := &model.TopUp{
		UserId:               id,
		Amount:               selectedProduct.Quota, // 充值额度
		Money:                selectedProduct.Price, // 支付金额
		CreditedQuota:        creditedQuota,
		AffiliateSourceQuota: creditedQuota,
		TradeNo:              referenceId,
		PaymentMethod:        model.PaymentMethodCreem,
		PaymentProvider:      model.PaymentProviderCreem,
		RequestIP:            c.ClientIP(),
		CreateTime:           time.Now().Unix(),
		Status:               common.TopUpStatusPending,
	}
	err = topUp.Insert()
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 创建充值订单失败 user_id=%d trade_no=%s product_id=%s error=%q", id, referenceId, selectedProduct.ProductId, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	attempt, err := model.CreateTopUpPaymentAttempt(referenceId, model.PaymentProviderCreem, model.PaymentMethodCreem, providerAmount, providerCurrency)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 创建支付尝试失败 user_id=%d trade_no=%s error=%q", id, referenceId, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建支付订单失败"})
		return
	}

	// 创建支付链接，传入用户邮箱
	checkoutUrl, providerOrderId, err := genCreemLink(c.Request.Context(), referenceId, selectedProduct, user.Email, user.Username)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 创建支付链接失败 user_id=%d trade_no=%s product_id=%s error=%q", id, referenceId, selectedProduct.ProductId, err.Error()))
		_ = model.MarkTopUpPaymentAttemptLaunchFailed(attempt.Id, err.Error())
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	if err := model.MarkTopUpPaymentAttemptLaunched(attempt.Id, providerOrderId); err != nil {
		_ = model.MarkTopUpPaymentAttemptLaunchFailed(attempt.Id, err.Error())
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 保存第三方订单快照失败 user_id=%d trade_no=%s provider_order_id=%s error=%q", id, referenceId, providerOrderId, err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "保存支付订单失败"})
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 充值订单创建成功 user_id=%d trade_no=%s product_id=%s product_name=%q quota=%d money=%.2f provider_order_id=%s", id, referenceId, selectedProduct.ProductId, selectedProduct.Name, selectedProduct.Quota, selectedProduct.Price, providerOrderId))

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"checkout_url":      checkoutUrl,
			"order_id":          referenceId,
			"provider_order_id": providerOrderId,
		},
	})
}

func RequestCreemPay(c *gin.Context) {
	var req CreemPayRequest

	// 读取body内容用于打印，同时保留原始数据供后续使用
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 支付请求读取失败 error=%q", err.Error()))
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "read query error"})
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 支付请求已收到 user_id=%d body=%q", c.GetInt("id"), string(bodyBytes)))

	// 重新设置body供后续的ShouldBindJSON使用
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	err = c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	creemAdaptor.RequestPay(c, &req)
}

// 新的Creem Webhook结构体，匹配实际的webhook数据格式
type CreemWebhookEvent struct {
	Id        string `json:"id"`
	EventType string `json:"eventType"`
	CreatedAt int64  `json:"created_at"`
	Object    struct {
		Id        string `json:"id"`
		Object    string `json:"object"`
		RequestId string `json:"request_id"`
		Order     struct {
			Object      string `json:"object"`
			Id          string `json:"id"`
			Customer    string `json:"customer"`
			Product     string `json:"product"`
			Amount      int    `json:"amount"`
			Currency    string `json:"currency"`
			SubTotal    int    `json:"sub_total"`
			TaxAmount   int    `json:"tax_amount"`
			AmountDue   int    `json:"amount_due"`
			AmountPaid  int    `json:"amount_paid"`
			Status      string `json:"status"`
			Type        string `json:"type"`
			Transaction string `json:"transaction"`
			CreatedAt   string `json:"created_at"`
			UpdatedAt   string `json:"updated_at"`
			Mode        string `json:"mode"`
		} `json:"order"`
		Product struct {
			Id                string  `json:"id"`
			Object            string  `json:"object"`
			Name              string  `json:"name"`
			Description       string  `json:"description"`
			Price             int     `json:"price"`
			Currency          string  `json:"currency"`
			BillingType       string  `json:"billing_type"`
			BillingPeriod     string  `json:"billing_period"`
			Status            string  `json:"status"`
			TaxMode           string  `json:"tax_mode"`
			TaxCategory       string  `json:"tax_category"`
			DefaultSuccessUrl *string `json:"default_success_url"`
			CreatedAt         string  `json:"created_at"`
			UpdatedAt         string  `json:"updated_at"`
			Mode              string  `json:"mode"`
		} `json:"product"`
		Units    int `json:"units"`
		Customer struct {
			Id        string `json:"id"`
			Object    string `json:"object"`
			Email     string `json:"email"`
			Name      string `json:"name"`
			Country   string `json:"country"`
			CreatedAt string `json:"created_at"`
			UpdatedAt string `json:"updated_at"`
			Mode      string `json:"mode"`
		} `json:"customer"`
		Status   string            `json:"status"`
		Metadata map[string]string `json:"metadata"`
		Mode     string            `json:"mode"`
	} `json:"object"`
}

func CreemWebhook(c *gin.Context) {
	if !isCreemWebhookEnabled() {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem webhook 被拒绝 reason=webhook_disabled path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))
		c.AbortWithStatus(http.StatusForbidden)
		return
	}

	// 读取body内容用于打印，同时保留原始数据供后续使用
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem webhook 读取请求体失败 path=%q client_ip=%s error=%q", c.Request.RequestURI, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	// 获取签名头
	signature := c.GetHeader(CreemSignatureHeader)
	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem webhook 收到请求 path=%q client_ip=%s signature=%q body=%q", c.Request.RequestURI, c.ClientIP(), signature, string(bodyBytes)))
	if signature == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem webhook 缺少签名 path=%q client_ip=%s body=%q", c.Request.RequestURI, c.ClientIP(), string(bodyBytes)))
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	// 验证签名
	if !verifyCreemSignature(string(bodyBytes), signature, setting.CreemWebhookSecret) {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem webhook 验签失败 path=%q client_ip=%s signature=%q body=%q", c.Request.RequestURI, c.ClientIP(), signature, string(bodyBytes)))
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem webhook 验签成功 path=%q client_ip=%s", c.Request.RequestURI, c.ClientIP()))

	// 重新设置body供后续的ShouldBindJSON使用
	c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// 解析新格式的webhook数据
	var webhookEvent CreemWebhookEvent
	if err := c.ShouldBindJSON(&webhookEvent); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem webhook 解析失败 path=%q client_ip=%s error=%q body=%q", c.Request.RequestURI, c.ClientIP(), err.Error(), string(bodyBytes)))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem webhook 解析成功 event_type=%s event_id=%s request_id=%s order_id=%s order_status=%s", webhookEvent.EventType, webhookEvent.Id, webhookEvent.Object.RequestId, webhookEvent.Object.Order.Id, webhookEvent.Object.Order.Status))

	// 根据事件类型处理不同的webhook
	switch webhookEvent.EventType {
	case "checkout.completed":
		handleCheckoutCompleted(c, &webhookEvent)
	default:
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem webhook 忽略事件 event_type=%s event_id=%s", webhookEvent.EventType, webhookEvent.Id))
		c.Status(http.StatusOK)
	}
}

// 处理支付完成事件
func handleCheckoutCompleted(c *gin.Context, event *CreemWebhookEvent) {
	// 验证订单状态
	if event.Object.Order.Status != "paid" {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 订单状态未支付，忽略处理 request_id=%s order_id=%s order_status=%s", event.Object.RequestId, event.Object.Order.Id, event.Object.Order.Status))
		c.Status(http.StatusOK)
		return
	}

	// 获取引用ID（这是我们创建订单时传递的request_id）
	referenceId := event.Object.RequestId
	if referenceId == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem webhook 缺少 request_id event_id=%s order_id=%s", event.Id, event.Object.Order.Id))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	order := model.GetSubscriptionOrderByTradeNo(referenceId)
	if order != nil && !validateSubscriptionProviderSnapshot(
		order,
		model.PaymentProviderCreem,
		event.Object.Id,
		fmt.Sprintf("%d", event.Object.Order.AmountPaid),
		event.Object.Order.Currency,
		true,
	) {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 订阅回调快照不匹配 trade_no=%s checkout_id=%s order_id=%s", referenceId, event.Object.Id, event.Object.Order.Id))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if err := model.CompleteSubscriptionOrder(referenceId, common.GetJsonString(event), model.PaymentProviderCreem, ""); err == nil {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 订阅订单处理成功 trade_no=%s creem_order_id=%s", referenceId, event.Object.Order.Id))
		c.Status(http.StatusOK)
		return
	} else if err != nil && !errors.Is(err, model.ErrSubscriptionOrderNotFound) {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 订阅订单处理失败 trade_no=%s creem_order_id=%s error=%q", referenceId, event.Object.Order.Id, err.Error()))
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	// 验证订单类型，目前只处理一次性付款（充值）
	if event.Object.Order.Type != "onetime" {
		logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 暂不支持该订单类型，忽略处理 request_id=%s creem_order_id=%s order_type=%s", referenceId, event.Object.Order.Id, event.Object.Order.Type))
		c.Status(http.StatusOK)
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 支付完成回调 trade_no=%s creem_order_id=%s amount_paid=%d currency=%s product_name=%q customer_email=%q customer_name=%q", referenceId, event.Object.Order.Id, event.Object.Order.AmountPaid, event.Object.Order.Currency, event.Object.Product.Name, event.Object.Customer.Email, event.Object.Customer.Name))

	providerOrderId := strings.TrimSpace(event.Object.Id)
	providerAmount := fmt.Sprintf("%d", event.Object.Order.AmountPaid)
	providerCurrency := strings.TrimSpace(event.Object.Order.Currency)
	if providerOrderId == "" || providerCurrency == "" {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 充值回调快照不完整 trade_no=%s checkout_id=%s", referenceId, providerOrderId))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	attempt, err := model.ResolveTopUpPaymentAttempt(model.PaymentProviderCreem, referenceId, providerOrderId)
	if err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 充值回调支付尝试不存在 trade_no=%s checkout_id=%s error=%q", referenceId, providerOrderId, err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if err := model.ValidateTopUpPaymentAttemptSnapshot(attempt, model.PaymentProviderCreem, providerOrderId, providerAmount, providerCurrency, decimal.Zero); err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 充值回调快照不匹配 trade_no=%s checkout_id=%s error=%q", referenceId, providerOrderId, err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if err := model.BindTopUpPaymentAttemptProviderOrder(attempt.Id, providerOrderId); err != nil {
		logger.LogWarn(c.Request.Context(), fmt.Sprintf("Creem 充值回调订单绑定失败 trade_no=%s checkout_id=%s error=%q", referenceId, providerOrderId, err.Error()))
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	if _, err := model.CompleteCreemTopUpPaymentAttempt(attempt.Id, referenceId, event.Object.Customer.Email, c.ClientIP()); err != nil {
		logger.LogError(c.Request.Context(), fmt.Sprintf("Creem 充值处理失败 trade_no=%s checkout_id=%s client_ip=%s error=%q", referenceId, providerOrderId, c.ClientIP(), err.Error()))
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}

	logger.LogInfo(c.Request.Context(), fmt.Sprintf("Creem 充值成功 trade_no=%s checkout_id=%s client_ip=%s", referenceId, providerOrderId, c.ClientIP()))
	c.Status(http.StatusOK)
}

type CreemCheckoutRequest struct {
	ProductId string `json:"product_id"`
	RequestId string `json:"request_id"`
	Customer  struct {
		Email string `json:"email"`
	} `json:"customer"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

type CreemCheckoutResponse struct {
	CheckoutUrl string `json:"checkout_url"`
	Id          string `json:"id"`
}

func genCreemLink(ctx context.Context, referenceId string, product *CreemProduct, email string, username string) (string, string, error) {
	if setting.CreemApiKey == "" {
		return "", "", fmt.Errorf("未配置Creem API密钥")
	}

	// 根据测试模式选择 API 端点
	apiUrl := "https://api.creem.io/v1/checkouts"
	if setting.CreemTestMode {
		apiUrl = "https://test-api.creem.io/v1/checkouts"
		logger.LogInfo(ctx, fmt.Sprintf("Creem 使用测试环境 api_url=%s", apiUrl))
	}

	// 构建请求数据，确保包含用户邮箱
	requestData := CreemCheckoutRequest{
		ProductId: product.ProductId,
		RequestId: referenceId, // 这个作为订单ID传递给Creem
		Customer: struct {
			Email string `json:"email"`
		}{
			Email: email, // 用户邮箱会在支付页面预填充
		},
		Metadata: map[string]string{
			"username":     username,
			"reference_id": referenceId,
			"product_name": product.Name,
			"quota":        fmt.Sprintf("%d", product.Quota),
		},
	}

	// 序列化请求数据
	jsonData, err := common.Marshal(requestData)
	if err != nil {
		return "", "", fmt.Errorf("序列化请求数据失败: %v", err)
	}

	// 创建 HTTP 请求
	req, err := http.NewRequest("POST", apiUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", "", fmt.Errorf("创建HTTP请求失败: %v", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", setting.CreemApiKey)

	logger.LogInfo(ctx, fmt.Sprintf("Creem 支付请求已发送 api_url=%s product_id=%s email=%q trade_no=%s", apiUrl, product.ProductId, email, referenceId))

	// 发送请求
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("发送HTTP请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("读取响应失败: %v", err)
	}

	logger.LogInfo(ctx, fmt.Sprintf("Creem API 响应已收到 trade_no=%s status_code=%d body=%q", referenceId, resp.StatusCode, string(body)))

	// 检查响应状态
	if resp.StatusCode/100 != 2 {
		return "", "", fmt.Errorf("Creem API http status %d ", resp.StatusCode)
	}
	// 解析响应
	var checkoutResp CreemCheckoutResponse
	err = common.Unmarshal(body, &checkoutResp)
	if err != nil {
		return "", "", fmt.Errorf("解析响应失败: %v", err)
	}

	if checkoutResp.CheckoutUrl == "" {
		return "", "", fmt.Errorf("Creem API resp no checkout url ")
	}

	logger.LogInfo(ctx, fmt.Sprintf("Creem 支付链接创建成功 trade_no=%s response_id=%s checkout_url=%q", referenceId, checkoutResp.Id, checkoutResp.CheckoutUrl))
	return checkoutResp.CheckoutUrl, checkoutResp.Id, nil
}
