package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
)

const (
	PayPalSandboxURL = "https://api-m.sandbox.paypal.com"
	PayPalLiveURL    = "https://api-m.paypal.com"
)

var payPalBaseURLOverride string
var payPalHTTPClient = &http.Client{}

func getPayPalURL() string {
	if payPalBaseURLOverride != "" {
		return strings.TrimRight(payPalBaseURLOverride, "/")
	}
	if setting.PayPalMode == "live" {
		return PayPalLiveURL
	}
	return PayPalSandboxURL
}

// PayPalCreateOrderRequest 创建订单请求
type PayPalCreateOrderRequest struct {
	Intent             string                    `json:"intent"`
	PurchaseUnits      []PayPalPurchaseUnit      `json:"purchase_units"`
	PaymentSource      *PayPalPaymentSource      `json:"payment_source,omitempty"`
	ApplicationContext *PayPalApplicationContext `json:"application_context,omitempty"`
}

type PayPalPurchaseUnit struct {
	ReferenceId string       `json:"reference_id"`
	InvoiceId   string       `json:"invoice_id,omitempty"`
	Amount      PayPalAmount `json:"amount"`
	Description string       `json:"description,omitempty"`
}

type PayPalAmount struct {
	CurrencyCode string `json:"currency_code"`
	Value        string `json:"value"`
}

type PayPalPaymentSource struct {
	Paypal *PayPalPayerInfo `json:"paypal,omitempty"`
}

type PayPalPayerInfo struct {
	ExperienceContext *PayPalExperienceContext `json:"experience_context,omitempty"`
}

type PayPalExperienceContext struct {
	ReturnUrl  string `json:"return_url"`
	CancelUrl  string `json:"cancel_url"`
	UserAction string `json:"user_action,omitempty"`
}

type PayPalApplicationContext struct {
	BrandName  string `json:"brand_name,omitempty"`
	UserAction string `json:"user_action,omitempty"`
	ReturnUrl  string `json:"return_url"`
	CancelUrl  string `json:"cancel_url"`
	NoticeUrl  string `json:"notify_url,omitempty"`
	LocaleCode string `json:"locale_code,omitempty"`
}

// PayPalCreateOrderResponse 创建订单响应
type PayPalCreateOrderResponse struct {
	Id      string              `json:"id"`
	Status  string              `json:"status"`
	Links   []PayPalLink        `json:"links"`
	Details []PayPalErrorDetail `json:"details,omitempty"`
	Message string              `json:"message,omitempty"`
}

type PayPalLink struct {
	Rel    string `json:"rel"`
	Href   string `json:"href"`
	Method string `json:"method,omitempty"`
}

type PayPalErrorDetail struct {
	Issue       string `json:"issue"`
	Description string `json:"description"`
}

// CreatePayPalOrder 创建 PayPal 订单
func CreatePayPalOrder(referenceId, amount, currency, returnUrl, cancelUrl string) (string, error) {
	if setting.PayPalClientId == "" || setting.PayPalClientSecret == "" {
		return "", fmt.Errorf("PayPal 配置不完整")
	}

	// 获取 access token
	token, err := getPayPalAccessToken()
	if err != nil {
		return "", fmt.Errorf("获取 PayPal token 失败: %w", err)
	}

	// 创建订单（使用 payment_source，不能同时使用已废弃的 application_context）
	req := PayPalCreateOrderRequest{
		Intent: "CAPTURE",
		PurchaseUnits: []PayPalPurchaseUnit{
			{
				ReferenceId: referenceId,
				InvoiceId:   referenceId,
				Amount: PayPalAmount{
					CurrencyCode: currency,
					Value:        amount,
				},
				Description: fmt.Sprintf("Recharge Order: %s", referenceId),
			},
		},
		PaymentSource: &PayPalPaymentSource{
			Paypal: &PayPalPayerInfo{
				ExperienceContext: &PayPalExperienceContext{
					ReturnUrl:  returnUrl,
					CancelUrl:  cancelUrl,
					UserAction: "PAY_NOW",
				},
			},
		},
	}

	reqBody, err := common.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("序列化请求失败: %w", err)
	}

	url := fmt.Sprintf("%s/v2/checkout/orders", getPayPalURL())
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("创建 HTTP 请求失败: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	httpResp, err := payPalHTTPClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("发送请求失败: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	var resp PayPalCreateOrderResponse
	if err := common.Unmarshal(respBody, &resp); err != nil {
		return "", fmt.Errorf("解析响应失败: %w", err)
	}

	// PayPal 返回 201 (CREATED) 或 200 (PAYER_ACTION_REQUIRED) 均为成功
	if httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusOK {
		if len(resp.Details) > 0 {
			return "", fmt.Errorf("PayPal API 错误: %s - %s", resp.Details[0].Issue, resp.Details[0].Description)
		}
		if resp.Message != "" {
			return "", fmt.Errorf("PayPal API 错误: %s", resp.Message)
		}
		return "", fmt.Errorf("PayPal API 错误 (HTTP %d): %s", httpResp.StatusCode, string(respBody))
	}

	// payment_source 模式返回 "payer-action"，基础模式返回 "approve"
	for _, link := range resp.Links {
		if link.Rel == "payer-action" || link.Rel == "approve" {
			return link.Href, nil
		}
	}

	return "", fmt.Errorf("未找到支付链接")
}

// getPayPalAccessToken 获取 PayPal access token
func getPayPalAccessToken() (string, error) {
	url := fmt.Sprintf("%s/v1/oauth2/token", getPayPalURL())

	// 构建 basic auth
	auth := fmt.Sprintf("%s:%s", setting.PayPalClientId, setting.PayPalClientSecret)
	encodedAuth := base64.StdEncoding.EncodeToString([]byte(auth))

	httpReq, err := http.NewRequest("POST", url, strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	httpReq.Header.Set("Authorization", fmt.Sprintf("Basic %s", encodedAuth))
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	httpResp, err := payPalHTTPClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	var tokenResp map[string]interface{}
	if err := common.Unmarshal(respBody, &tokenResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %w (响应: %s)", err, string(respBody))
	}

	if token, ok := tokenResp["access_token"].(string); ok {
		return token, nil
	}

	// PayPal 返回了错误，输出完整响应帮助诊断
	return "", fmt.Errorf("未获取到 token。响应状态: %d, 响应内容: %s", httpResp.StatusCode, string(respBody))
}

// CapturePayPalOrder 捕获 PayPal 订单（完成支付）
type PayPalCaptureResponse struct {
	Id            string               `json:"id"`
	Status        string               `json:"status"`
	PurchaseUnits []PayPalCapturedUnit `json:"purchase_units,omitempty"`
	Message       string               `json:"message,omitempty"`
	Details       []PayPalErrorDetail  `json:"details,omitempty"`
}

type PayPalCapturedUnit struct {
	ReferenceId string                  `json:"reference_id"`
	InvoiceId   string                  `json:"invoice_id,omitempty"`
	Payments    *PayPalCapturedPayments `json:"payments,omitempty"`
}

type PayPalCapturedPayments struct {
	Captures []PayPalCapture `json:"captures,omitempty"`
}

type PayPalCapture struct {
	Id     string       `json:"id"`
	Status string       `json:"status"`
	Amount PayPalAmount `json:"amount"`
}

func CapturePayPalOrder(orderID string) (*PayPalCaptureResponse, error) {
	if setting.PayPalClientId == "" || setting.PayPalClientSecret == "" {
		return nil, fmt.Errorf("PayPal 配置不完整")
	}

	token, err := getPayPalAccessToken()
	if err != nil {
		return nil, fmt.Errorf("获取 PayPal token 失败: %w", err)
	}

	url := fmt.Sprintf("%s/v2/checkout/orders/%s/capture", getPayPalURL(), orderID)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader([]byte("{}")))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	httpReq.Header.Set("Prefer", "return=representation")

	httpResp, err := payPalHTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var resp PayPalCaptureResponse
	if err := common.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if httpResp.StatusCode != http.StatusCreated && httpResp.StatusCode != http.StatusOK {
		if len(resp.Details) > 0 {
			return nil, fmt.Errorf("PayPal capture HTTP %d: issue=%s desc=%s", httpResp.StatusCode, resp.Details[0].Issue, resp.Details[0].Description)
		}
		return nil, fmt.Errorf("PayPal capture HTTP %d: message=%s body=%s", httpResp.StatusCode, resp.Message, truncateRespBody(respBody))
	}

	return &resp, nil
}

func truncateRespBody(body []byte) string {
	const maxLen = 300
	if len(body) <= maxLen {
		return string(body)
	}
	return string(body[:maxLen]) + "...(truncated)"
}

// GetPayPalOrder fetches the current state of a PayPal order via GET /v2/checkout/orders/{id}
func GetPayPalOrder(orderID string) (*PayPalCaptureResponse, error) {
	if setting.PayPalClientId == "" || setting.PayPalClientSecret == "" {
		return nil, fmt.Errorf("PayPal 配置不完整")
	}
	token, err := getPayPalAccessToken()
	if err != nil {
		return nil, fmt.Errorf("获取 PayPal token 失败: %w", err)
	}
	url := fmt.Sprintf("%s/v2/checkout/orders/%s", getPayPalURL(), orderID)
	httpReq, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := payPalHTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var resp PayPalCaptureResponse
	if err := common.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		if len(resp.Details) > 0 {
			return nil, fmt.Errorf("PayPal get order HTTP %d: %s - %s", httpResp.StatusCode, resp.Details[0].Issue, resp.Details[0].Description)
		}
		return nil, fmt.Errorf("PayPal get order HTTP %d: %s", httpResp.StatusCode, truncateRespBody(respBody))
	}
	return &resp, nil
}

// VerifyPayPalWebhookSignature verifies PayPal webhook signatures through
// PayPal's official verify-webhook-signature endpoint. PayPalWebhookSecret is
// used as the configured webhook ID for that API.
func VerifyPayPalWebhookSignature(transmissionID, transmissionTime, certURL string, payload []byte, signature string) bool {
	if setting.PayPalWebhookSecret == "" ||
		transmissionID == "" ||
		transmissionTime == "" ||
		certURL == "" ||
		signature == "" ||
		len(payload) == 0 {
		return false
	}

	token, err := getPayPalAccessToken()
	if err != nil {
		common.SysError("paypal webhook signature token failed: " + err.Error())
		return false
	}

	body, err := common.Marshal(map[string]interface{}{
		"transmission_id":   transmissionID,
		"transmission_time": transmissionTime,
		"cert_url":          certURL,
		"auth_algo":         "SHA256withRSA",
		"transmission_sig":  signature,
		"webhook_id":        setting.PayPalWebhookSecret,
		"webhook_event":     json.RawMessage(payload),
	})
	if err != nil {
		common.SysError("paypal webhook signature payload marshal failed: " + err.Error())
		return false
	}

	url := fmt.Sprintf("%s/v1/notifications/verify-webhook-signature", getPayPalURL())
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		common.SysError("paypal webhook signature request create failed: " + err.Error())
		return false
	}
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := payPalHTTPClient.Do(httpReq)
	if err != nil {
		common.SysError("paypal webhook signature request failed: " + err.Error())
		return false
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		common.SysError("paypal webhook signature response read failed: " + err.Error())
		return false
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		common.SysError(fmt.Sprintf("paypal webhook signature HTTP %d: %s", httpResp.StatusCode, truncateRespBody(respBody)))
		return false
	}

	var resp struct {
		VerificationStatus string `json:"verification_status"`
	}
	if err := common.Unmarshal(respBody, &resp); err != nil {
		common.SysError("paypal webhook signature response parse failed: " + err.Error())
		return false
	}
	return strings.EqualFold(resp.VerificationStatus, "SUCCESS")
}

// ExtractPayPalOrderID 从 webhook payload 中提取订单 ID
func ExtractPayPalOrderID(payload []byte) (string, error) {
	var data map[string]interface{}
	if err := common.Unmarshal(payload, &data); err != nil {
		return "", err
	}

	// PayPal webhook 事件结构
	if resource, ok := data["resource"].(map[string]interface{}); ok {
		if id, ok := resource["id"].(string); ok {
			return id, nil
		}
	}

	return "", fmt.Errorf("unable to extract order id from webhook")
}

// ExtractPayPalReferenceID 从 webhook payload 中提取 reference ID
func ExtractPayPalReferenceID(payload []byte) (string, error) {
	var data map[string]interface{}
	if err := common.Unmarshal(payload, &data); err != nil {
		return "", err
	}

	if resource, ok := data["resource"].(map[string]interface{}); ok {
		if purchaseUnits, ok := resource["purchase_units"].([]interface{}); ok && len(purchaseUnits) > 0 {
			if pu, ok := purchaseUnits[0].(map[string]interface{}); ok {
				// PayPal overwrites reference_id with "DEFAULT" for single-unit orders; invoice_id is preserved.
				if invoiceId, ok := pu["invoice_id"].(string); ok && invoiceId != "" {
					return invoiceId, nil
				}
				if refId, ok := pu["reference_id"].(string); ok && refId != "" && refId != "DEFAULT" {
					return refId, nil
				}
			}
		}
	}

	return "", fmt.Errorf("unable to extract reference id from webhook")
}

// ExtractPayPalAmount 从 webhook payload 中提取金额和币种
func ExtractPayPalAmount(payload []byte) (float64, string, error) {
	var data map[string]interface{}
	if err := common.Unmarshal(payload, &data); err != nil {
		return 0, "", err
	}

	if resource, ok := data["resource"].(map[string]interface{}); ok {
		if purchaseUnits, ok := resource["purchase_units"].([]interface{}); ok && len(purchaseUnits) > 0 {
			if pu, ok := purchaseUnits[0].(map[string]interface{}); ok {
				if payments, ok := pu["payments"].(map[string]interface{}); ok {
					if captures, ok := payments["captures"].([]interface{}); ok && len(captures) > 0 {
						if capture, ok := captures[0].(map[string]interface{}); ok {
							if amount, ok := capture["amount"].(map[string]interface{}); ok {
								if valueStr, ok := amount["value"].(string); ok {
									if currency, ok := amount["currency_code"].(string); ok {
										value, err := strconv.ParseFloat(valueStr, 64)
										return value, currency, err
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return 0, "", fmt.Errorf("unable to extract amount from webhook")
}
