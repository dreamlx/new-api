package controller

import (
	"context"
	"fmt"
	"log"
	"net/http"
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

const (
	PaymentMethodWxpay = "wxpay"

	wxpayOrderExpireSeconds = 30 * 60

	wxpayDescriptionFormat = "TopUp Order #%s"

	wxpayNotifyPath = "/api/user/wxpay/notify"

	// WeChat Pay APIv3 acknowledgement codes per
	// https://pay.weixin.qq.com/wiki/doc/apiv3/wechatpay/wechatpay4_1.shtml
	wxpayNotifyOK   = "SUCCESS"
	wxpayNotifyFail = "FAIL"
)

// wechatPayServiceProvider returns a WechatPayService instance. Tests override
// this var to inject a mock without touching the SDK or setting state.
var wechatPayServiceProvider = func() (service.WechatPayService, error) {
	client := setting.GetWechatPayClient()
	if client == nil {
		return nil, fmt.Errorf("wechat pay client not configured")
	}
	verifier := setting.GetWechatPayVerifier()
	if verifier == nil {
		return nil, fmt.Errorf("wechat pay verifier not available")
	}
	return service.NewRealWechatPayService(client, setting.WxpayMchId, setting.WxpayAppId, setting.WxpayApiV3Key, verifier), nil
}

type WxpayPayRequest struct {
	Amount int64 `json:"amount"`
}

// RequestWxpay creates a WeChat Pay Native (QR) order and returns the code_url.
//
// POST /api/user/wxpay/pay  {"amount": <int64>}
// Response: {"message":"success","data":{"code_url":"weixin://...","trade_no":"USRxxx"}}
func RequestWxpay(c *gin.Context) {
	if !setting.WxpayEnabled {
		c.JSON(200, gin.H{"message": "error", "data": "微信支付未启用"})
		return
	}

	var req WxpayPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	minTopUp := int64(setting.WxpayMinTopUp)
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dMin := decimal.NewFromInt(minTopUp)
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		minTopUp = dMin.Mul(dQuotaPerUnit).IntPart()
	}
	if req.Amount < minTopUp {
		c.JSON(200, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", minTopUp)})
		return
	}

	userId := c.GetInt("id")
	group, err := model.GetUserGroup(userId, true)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}

	payMoney := getPayMoney(req.Amount, group)
	if payMoney < 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	payAmountCents := common.MoneyToCents(payMoney)
	if payAmountCents <= 0 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	svc, err := wechatPayServiceProvider()
	if err != nil || svc == nil {
		log.Printf("wxpay service unavailable: %v", err)
		c.JSON(200, gin.H{"message": "error", "data": "支付服务暂不可用"})
		return
	}

	tradeNo := fmt.Sprintf("USR%dNO%s%d", userId, common.GetRandomString(6), time.Now().Unix())
	notifyURL := service.GetCallbackAddress() + wxpayNotifyPath
	description := fmt.Sprintf(wxpayDescriptionFormat, tradeNo)

	codeURL, err := svc.NativePrepay(c.Request.Context(), tradeNo, description, payAmountCents, notifyURL)
	if err != nil {
		log.Printf("wxpay NativePrepay failed: %v", err)
		c.JSON(200, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount := decimal.NewFromInt(amount)
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		amount = dAmount.Div(dQuotaPerUnit).IntPart()
	}

	now := time.Now().Unix()
	topUp := &model.TopUp{
		UserId:         userId,
		Amount:         amount,
		Money:          payMoney,
		TradeNo:        tradeNo,
		PaymentMethod:  PaymentMethodWxpay,
		CreateTime:     now,
		Status:         common.TopUpStatusPending,
		PayAmountCents: payAmountCents,
		Currency:       "CNY",
		ExpireTime:     now + wxpayOrderExpireSeconds,
	}
	if err := topUp.Insert(); err != nil {
		log.Printf("wxpay topup insert failed: %v (trade_no=%s)", err, tradeNo)
		c.JSON(200, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	c.JSON(200, gin.H{
		"message": "success",
		"data": gin.H{
			"code_url": codeURL,
			"trade_no": tradeNo,
		},
	})
}

func wxpayFail(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"code": wxpayNotifyFail, "message": message})
}

func wxpaySuccess(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": wxpayNotifyOK, "message": "成功"})
}

// WxpayNotify handles asynchronous WeChat Pay APIv3 payment notifications.
//
// POST /api/user/wxpay/notify (application/json, signed + AES-GCM encrypted)
//
// The SDK's notify.Handler verifies the WeChat-Pay-Signature header and
// decrypts the ciphertext into a payments.Transaction in one call.
//
// Validation order (all must pass before granting quota):
//  1. Service available
//  2. Signature verify + AES-GCM decrypt (SDK)
//  3. out_trade_no present
//  4. trade_state == SUCCESS (anything else: ack-and-skip to stop retries)
//  5. TopUp row exists
//  6. amount_total (cents) equals TopUp.PayAmountCents
//
// Idempotency: in-memory LockOrder + DB conditional update via
// CompleteTopUpByCondition (multi-replica safe). Quota is granted only when
// RowsAffected == 1.
func WxpayNotify(c *gin.Context) {
	svc, err := wechatPayServiceProvider()
	if err != nil || svc == nil {
		log.Printf("wxpay notify: service unavailable: %v", err)
		wxpayFail(c, http.StatusInternalServerError, "service unavailable")
		return
	}

	ctx := context.Background()
	result, err := svc.DecryptNotification(ctx, c.Request)
	if err != nil || result == nil {
		log.Printf("wxpay notify: decrypt failed: %v", err)
		wxpayFail(c, http.StatusUnauthorized, "decrypt failed")
		return
	}

	tradeNo := result.OutTradeNo
	if tradeNo == "" {
		log.Printf("wxpay notify: missing out_trade_no")
		wxpayFail(c, http.StatusBadRequest, "missing out_trade_no")
		return
	}

	// Non-SUCCESS states (NOTPAY/USERPAYING/CLOSED/...) are well-formed
	// notifications we do not act on. Acknowledge with SUCCESS to stop retries.
	if result.TradeState != "SUCCESS" {
		log.Printf("wxpay notify: non-success trade_state=%s tradeNo=%s", result.TradeState, tradeNo)
		wxpaySuccess(c)
		return
	}

	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil {
		log.Printf("wxpay notify: top-up not found tradeNo=%s", tradeNo)
		wxpayFail(c, http.StatusNotFound, "topup not found")
		return
	}

	if topUp.PayAmountCents > 0 && result.AmountTotal != topUp.PayAmountCents {
		reason := fmt.Sprintf("amount mismatch: notify=%d expected=%d", result.AmountTotal, topUp.PayAmountCents)
		log.Printf("wxpay notify: %s tradeNo=%s", reason, tradeNo)
		_ = model.SetTopUpAnomaly(model.DB, tradeNo, reason)
		wxpayFail(c, http.StatusBadRequest, "amount mismatch")
		return
	}

	rowsAffected, err := model.CompleteTopUpByCondition(model.DB, tradeNo, result.TransactionId, result.PaidAt)
	if err != nil {
		log.Printf("wxpay notify: CompleteTopUpByCondition failed tradeNo=%s err=%v", tradeNo, err)
		wxpayFail(c, http.StatusInternalServerError, "db error")
		return
	}
	if rowsAffected == 0 {
		log.Printf("wxpay notify: idempotent skip tradeNo=%s (already completed)", tradeNo)
		wxpaySuccess(c)
		return
	}

	dAmount := decimal.NewFromInt(topUp.Amount)
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	quotaToAdd := int(dAmount.Mul(dQuotaPerUnit).IntPart())
	if quotaToAdd > 0 {
		if err := model.IncreaseUserQuota(topUp.UserId, quotaToAdd, true); err != nil {
			// Quota grant failed AFTER status flip; ack SUCCESS so WeChat does not
			// retry-storm, and surface the inconsistency through logs for ops.
			log.Printf("wxpay notify: IncreaseUserQuota failed tradeNo=%s userId=%d err=%v",
				tradeNo, topUp.UserId, err)
		} else {
			model.RecordLog(topUp.UserId, model.LogTypeTopup,
				fmt.Sprintf("使用微信在线充值成功，充值金额：%s，订单号：%s",
					logger.LogQuota(quotaToAdd), tradeNo))
		}
	}

	wxpaySuccess(c)
}
