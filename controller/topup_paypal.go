package controller

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
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
	returnUrl := system_setting.ServerAddress + "/api/paypal/return"
	cancelUrl := system_setting.ServerAddress + "/console/topup"
	log.Printf("PayPal 创建订单开始: user_id=%d trade_no=%s amount=%d money=%s currency=USD return_url=%s cancel_url=%s\n", userId, referenceId, req.Amount, moneyStr, returnUrl, cancelUrl)

	payLink, err := service.CreatePayPalOrder(referenceId, moneyStr, "USD", returnUrl, cancelUrl)
	if err != nil {
		log.Printf("创建 PayPal 订单失败: user_id=%d trade_no=%s err=%v\n", userId, referenceId, err)
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": fmt.Sprintf("创建 PayPal 订单失败: %v", err)})
		return
	}
	log.Printf("PayPal 平台订单创建成功: user_id=%d trade_no=%s\n", userId, referenceId)

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
		log.Printf("创建 PayPal 本地充值订单失败: user_id=%d trade_no=%s err=%v\n", userId, referenceId, err)
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	log.Printf("PayPal 本地充值订单创建成功: user_id=%d trade_no=%s topup_id=%d status=%s\n", userId, referenceId, order.Id, order.Status)

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
	log.Printf("PayPal webhook 收到事件: event=%s payload_size=%d\n", eventType, len(payload))

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
		log.Printf("PayPal webhook 订单完成: event=%s trade_no=%s amount=%.2f currency=%s\n", eventType, referenceID, amount, currency)
		if err := completePayPalOrder(referenceID, amount, currency, "CHECKOUT.ORDER.COMPLETED"); err != nil {
			log.Printf("完成 PayPal 订单失败: event=%s trade_no=%s err=%v\n", eventType, referenceID, err)
		}
	case "PAYMENT.CAPTURE.COMPLETED":
		referenceID, amount, currency, err := extractPayPalCaptureCompletedData(payload)
		if err != nil {
			log.Printf("解析 PayPal 支付完成数据失败: %v\n", err)
			break
		}
		log.Printf("PayPal webhook 捕获完成: event=%s trade_no=%s amount=%.2f currency=%s\n", eventType, referenceID, amount, currency)
		if err := completePayPalOrder(referenceID, amount, currency, "PAYMENT.CAPTURE.COMPLETED"); err != nil {
			log.Printf("完成 PayPal 订单失败: event=%s trade_no=%s err=%v\n", eventType, referenceID, err)
		}
	case "CHECKOUT.ORDER.APPROVED":
		orderID, err := service.ExtractPayPalOrderID(payload)
		if err != nil {
			log.Printf("提取 PayPal 订单 ID 失败: %v\n", err)
			break
		}
		log.Printf("PayPal webhook 订单已批准，开始捕获: event=%s paypal_order_id=%s\n", eventType, orderID)
		resp, err := payPalCaptureOrder(orderID)
		if err != nil {
			log.Printf("捕获 PayPal 订单失败: event=%s paypal_order_id=%s err=%v\n", eventType, orderID, err)
			break
		}
		if err := handlePayPalCaptureResponse(resp); err != nil {
			log.Printf("处理 PayPal 捕获结果失败: event=%s paypal_order_id=%s err=%v\n", eventType, orderID, err)
		}
	default:
		log.Printf("不支持的 PayPal Webhook 事件类型: %s\n", eventType)
	}

	c.Status(http.StatusOK)
}

func PayPalReturn(c *gin.Context) {
	orderID := c.Query("token")
	payerID := c.Query("PayerID")
	if orderID == "" {
		log.Printf("PayPal return 缺少 token: payer_id=%s\n", payerID)
		redirectPayPalReturn(c, "fail")
		return
	}
	log.Printf("PayPal return 收到回跳: paypal_order_id=%s payer_id=%s\n", orderID, payerID)

	resp, err := payPalCaptureOrder(orderID)
	if err != nil {
		log.Printf("PayPal return 捕获订单失败: paypal_order_id=%s err=%v\n", orderID, err)
		if strings.Contains(err.Error(), "ORDER_ALREADY_CAPTURED") {
			redirectPayPalReturn(c, "success")
			return
		}
		redirectPayPalReturn(c, "pending")
		return
	}
	if err := handlePayPalCaptureResponse(resp); err != nil {
		log.Printf("PayPal return 处理捕获结果失败: paypal_order_id=%s err=%v\n", orderID, err)
		redirectPayPalReturn(c, "pending")
		return
	}

	redirectPayPalReturn(c, "success")
}

func redirectPayPalReturn(c *gin.Context, status string) {
	target := system_setting.ServerAddress + "/console/topup?pay=" + status + "&show_history=true"
	log.Printf("PayPal return 重定向: status=%s target=%s\n", status, target)
	c.Redirect(http.StatusFound, target)
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
	log.Printf("PayPal capture 响应解析成功: paypal_order_id=%s status=%s trade_no=%s amount=%.2f currency=%s\n", resp.Id, resp.Status, referenceID, amount, currency)

	return completePayPalOrder(referenceID, amount, currency, "PAYPAL_ORDER_CAPTURE")
}

func completePayPalOrder(referenceID string, amount float64, currency string, event string) error {
	LockOrder(referenceID)
	defer UnlockOrder(referenceID)
	log.Printf("PayPal 本地订单完成开始: trade_no=%s event=%s amount=%.2f currency=%s\n", referenceID, event, amount, currency)

	payloadStr := common.GetJsonString(map[string]interface{}{
		"amount":   amount,
		"currency": currency,
		"event":    event,
	})

	// 尝试完成订阅订单（如果存在）
	if err := model.CompleteSubscriptionOrder(referenceID, payloadStr); err == nil {
		log.Printf("PayPal 订阅订单完成成功: trade_no=%s event=%s\n", referenceID, event)
		return nil
	} else if err != nil && !errors.Is(err, model.ErrSubscriptionOrderNotFound) {
		log.Printf("完成订阅订单失败: trade_no=%s event=%s err=%v\n", referenceID, event, err)
		return err
	}

	topUp := model.GetTopUpByTradeNo(referenceID)
	if topUp == nil {
		err := fmt.Errorf("PayPal 充值订单不存在: %s", referenceID)
		log.Println(err)
		return err
	}
	log.Printf("PayPal 本地充值订单命中: trade_no=%s topup_id=%d user_id=%d status=%s\n", referenceID, topUp.Id, topUp.UserId, topUp.Status)
	if topUp.Status == common.TopUpStatusSuccess {
		log.Printf("PayPal 订单已完成，忽略重复通知: trade_no=%s event=%s\n", referenceID, event)
		return nil
	}
	if topUp.Status != common.TopUpStatusPending {
		err := fmt.Errorf("PayPal 充值订单状态错误: %s %s", referenceID, topUp.Status)
		log.Println(err)
		return err
	}

	if err := model.Recharge(referenceID, ""); err != nil {
		log.Println("充值失败:", err.Error(), referenceID)
		return err
	}

	log.Printf("PayPal 支付成功，本地充值完成: trade_no=%s topup_id=%d user_id=%d amount=%.2f currency=%s event=%s\n", referenceID, topUp.Id, topUp.UserId, amount, currency, event)
	return nil
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
	// PayPal overwrites reference_id with "DEFAULT" for single-unit orders; invoice_id is preserved.
	referenceID := unit.InvoiceId
	if referenceID == "" {
		referenceID = unit.ReferenceId
	}
	if referenceID == "" || referenceID == "DEFAULT" {
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
