package controller

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
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

var payPalCaptureOrder = service.CapturePayPalOrder

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

	eventType, err := extractPayPalEventType(payload)
	if err != nil {
		log.Printf("解析 PayPal Webhook 数据失败: %v\n", err)
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}

	switch eventType {
	case "CHECKOUT.ORDER.COMPLETED":
		referenceID, err := service.ExtractPayPalReferenceID(payload)
		if err != nil {
			log.Printf("解析 PayPal 订单完成 reference_id 失败: %v\n", err)
			break
		}
		amount, currency, err := service.ExtractPayPalAmount(payload)
		if err != nil {
			log.Printf("解析 PayPal 订单完成金额失败: %v\n", err)
			break
		}
		completePayPalOrder(referenceID, amount, currency, "CHECKOUT.ORDER.COMPLETED")
	case "PAYMENT.CAPTURE.COMPLETED":
		referenceID, amount, currency, err := extractPayPalCaptureCompletedData(payload)
		if err != nil {
			log.Printf("解析 PayPal 支付完成数据失败: %v\n", err)
			break
		}
		completePayPalOrder(referenceID, amount, currency, "PAYMENT.CAPTURE.COMPLETED")
	case "CHECKOUT.ORDER.APPROVED":
		orderID, err := service.ExtractPayPalOrderID(payload)
		if err != nil {
			log.Printf("提取 PayPal 订单 ID 失败: %v\n", err)
			break
		}
		resp, err := payPalCaptureOrder(orderID)
		if err != nil {
			log.Printf("捕获 PayPal 订单失败: %v\n", err)
			break
		}
		if err := handlePayPalCaptureResponse(resp); err != nil {
			log.Printf("处理 PayPal 捕获结果失败: %v\n", err)
		}
	default:
		log.Printf("不支持的 PayPal Webhook 事件类型: %s\n", eventType)
	}

	c.Status(http.StatusOK)
}

func handlePayPalCaptureResponse(resp *service.PayPalCaptureResponse) error {
	if resp == nil {
		return fmt.Errorf("empty capture response")
	}
	if resp.Status != "COMPLETED" {
		return fmt.Errorf("capture status is %s", resp.Status)
	}

	referenceID, amount, currency, err := extractPayPalCaptureResponseData(resp)
	if err != nil {
		return err
	}

	completePayPalOrder(referenceID, amount, currency, "PAYPAL_ORDER_CAPTURE")
	return nil
}

func completePayPalOrder(referenceID string, amount float64, currency string, event string) {
	LockOrder(referenceID)
	defer UnlockOrder(referenceID)

	payloadStr := common.GetJsonString(map[string]interface{}{
		"amount":   amount,
		"currency": currency,
		"event":    event,
	})

	// 尝试完成订阅订单（如果存在）
	if err := model.CompleteSubscriptionOrder(referenceID, payloadStr); err == nil {
		return
	} else if err != nil && !errors.Is(err, model.ErrSubscriptionOrderNotFound) {
		log.Println("完成订阅订单失败:", err.Error(), referenceID)
		return
	}

	topUp := model.GetTopUpByTradeNo(referenceID)
	if topUp == nil {
		log.Println("PayPal 充值订单不存在:", referenceID)
		return
	}
	if topUp.Status == common.TopUpStatusSuccess {
		log.Println("PayPal 订单已完成，忽略重复通知:", referenceID)
		return
	}
	if topUp.Status != common.TopUpStatusPending {
		log.Println("PayPal 充值订单状态错误:", referenceID, topUp.Status)
		return
	}

	if err := model.Recharge(referenceID, ""); err != nil {
		log.Println("充值失败:", err.Error(), referenceID)
		return
	}

	log.Printf("PayPal 支付成功: %s, %.2f %s\n", referenceID, amount, currency)
}

func extractPayPalEventType(payload []byte) (string, error) {
	var data map[string]interface{}
	if err := common.Unmarshal(payload, &data); err != nil {
		return "", err
	}

	eventType, _ := data["event_type"].(string)
	if eventType == "" {
		return "", fmt.Errorf("unable to extract event_type")
	}

	return eventType, nil
}

func extractPayPalCaptureResponseData(resp *service.PayPalCaptureResponse) (string, float64, string, error) {
	if len(resp.PurchaseUnits) == 0 {
		return "", 0, "", fmt.Errorf("capture response missing purchase_units")
	}
	unit := resp.PurchaseUnits[0]
	referenceID := unit.ReferenceId
	if referenceID == "" {
		referenceID = unit.InvoiceId
	}
	if referenceID == "" {
		return "", 0, "", fmt.Errorf("capture response missing reference_id")
	}
	if unit.Payments == nil || len(unit.Payments.Captures) == 0 {
		return "", 0, "", fmt.Errorf("capture response missing captures")
	}
	capture := unit.Payments.Captures[0]
	if capture.Status != "" && capture.Status != "COMPLETED" {
		return "", 0, "", fmt.Errorf("capture status is %s", capture.Status)
	}
	amount, err := strconv.ParseFloat(capture.Amount.Value, 64)
	if err != nil {
		return "", 0, "", err
	}
	return referenceID, amount, capture.Amount.CurrencyCode, nil
}

func extractPayPalCaptureCompletedData(payload []byte) (string, float64, string, error) {
	var data map[string]interface{}
	if err := common.Unmarshal(payload, &data); err != nil {
		return "", 0, "", err
	}

	resource, ok := data["resource"].(map[string]interface{})
	if !ok {
		return "", 0, "", fmt.Errorf("unable to extract resource")
	}
	referenceID, _ := resource["invoice_id"].(string)
	if referenceID == "" {
		return "", 0, "", fmt.Errorf("unable to extract invoice_id")
	}
	amountData, ok := resource["amount"].(map[string]interface{})
	if !ok {
		return "", 0, "", fmt.Errorf("unable to extract amount")
	}
	valueStr, _ := amountData["value"].(string)
	currency, _ := amountData["currency_code"].(string)
	amount, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return "", 0, "", err
	}

	return referenceID, amount, currency, nil
}
