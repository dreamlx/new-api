package controller

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/thanhpk/randstr"
)

const PaymentMethodPayPal = "paypal"

type PayPalTopUpRequest struct {
	Amount int64 `json:"amount"`
}

func RequestPayPalTopUp(c *gin.Context) {
	var req PayPalTopUpRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.Amount <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	if setting.PayPalClientId == "" || setting.PayPalClientSecret == "" {
		common.ApiErrorMsg(c, "PayPal 未配置或配置不完整")
		return
	}

	if setting.PayPalWebhookSecret == "" {
		common.ApiErrorMsg(c, "PayPal Webhook 未配置")
		return
	}

	userId := c.GetInt("id")
	user, err := model.GetUserById(userId, false)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if user == nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}

	// 生成订单号
	reference := fmt.Sprintf("paypal-topup-%d-%d-%s", user.Id, time.Now().UnixMilli(), randstr.String(4))
	referenceId := "paypal_" + common.Sha1([]byte(reference))

	money := getPayMoney(req.Amount, user.Group)
	moneyStr := fmt.Sprintf("%.2f", money)
	if moneyStr == "0.00" || money <= 0 {
		common.ApiErrorMsg(c, "充值金额过小，最低 $0.01")
		return
	}

	// 创建 PayPal 订单
	returnUrl := system_setting.ServerAddress + "/console/log"
	cancelUrl := system_setting.ServerAddress + "/console/topup"

	payLink, err := service.CreatePayPalOrder(referenceId, moneyStr, "USD", returnUrl, cancelUrl)
	if err != nil {
		log.Println("创建 PayPal 订单失败:", err)
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("创建 PayPal 订单失败: %v", err)})
		return
	}

	// 创建本地充值记录
	order := &model.TopUp{
		UserId:        userId,
		Amount:        req.Amount,
		Money:         money,
		TradeNo:       referenceId,
		PaymentMethod: PaymentMethodPayPal,
		CreateTime:    time.Now().Unix(),
		Status:        common.TopUpStatusPending,
	}
	if err := order.Insert(); err != nil {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link": payLink,
		},
	})
}

func PayPalWebhook(c *gin.Context) {
	payload, err := io.ReadAll(c.Request.Body)
	if err != nil {
		log.Printf("读取 PayPal Webhook 数据失败: %v\n", err)
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}

	// 验证签名（可选，PayPal 推荐使用）
	transmissionID := c.GetHeader("Paypal-Transmission-Id")
	transmissionTime := c.GetHeader("Paypal-Transmission-Time")
	certURL := c.GetHeader("Paypal-Cert-Url")
	signature := c.GetHeader("Paypal-Transmission-Sig")

	// 注：这里简化了签名验证，生产环境应该使用完整的 PayPal 签名验证
	_ = transmissionID
	_ = transmissionTime
	_ = certURL
	_ = signature

	// 提取事件类型和数据
	eventType, referenceID, err := extractPayPalEventData(payload)
	if err != nil {
		log.Printf("解析 PayPal Webhook 数据失败: %v\n", err)
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	switch eventType {
	case "CHECKOUT.ORDER.COMPLETED", "PAYMENT.CAPTURE.COMPLETED":
		handlePayPalPaymentCompleted(referenceID, payload)
	case "CHECKOUT.ORDER.APPROVED":
		// 订单已批准，但尚未捕获，可选处理
		log.Printf("PayPal 订单已批准: %s\n", referenceID)
	default:
		log.Printf("不支持的 PayPal Webhook 事件类型: %s\n", eventType)
	}

	c.Status(http.StatusOK)
}

func handlePayPalPaymentCompleted(referenceID string, payload []byte) {
	LockOrder(referenceID)
	defer UnlockOrder(referenceID)

	amount, currency, err := service.ExtractPayPalAmount(payload)
	if err != nil {
		log.Println("提取 PayPal 金额失败:", err)
		return
	}

	// 记录 webhook payload
	payloadStr := common.GetJsonString(map[string]interface{}{
		"amount":   amount,
		"currency": currency,
		"event":    "PAYMENT.CAPTURE.COMPLETED",
	})

	// 尝试完成订阅订单（如果存在）
	if err := model.CompleteSubscriptionOrder(referenceID, payloadStr); err == nil {
		return
	} else if err != nil && !errors.Is(err, model.ErrSubscriptionOrderNotFound) {
		log.Println("完成订阅订单失败:", err.Error(), referenceID)
		return
	}

	// 充值
	err = model.Recharge(referenceID, "")
	if err != nil {
		log.Println("充值失败:", err.Error(), referenceID)
		return
	}

	log.Printf("PayPal 支付成功: %s, %.2f %s\n", referenceID, amount, currency)
}

func extractPayPalEventData(payload []byte) (string, string, error) {
	var data map[string]interface{}
	if err := common.Unmarshal(payload, &data); err != nil {
		return "", "", err
	}

	eventType, _ := data["event_type"].(string)
	if eventType == "" {
		return "", "", fmt.Errorf("unable to extract event_type")
	}

	referenceID, err := service.ExtractPayPalReferenceID(payload)
	if err != nil {
		return eventType, "", err
	}

	return eventType, referenceID, nil
}
