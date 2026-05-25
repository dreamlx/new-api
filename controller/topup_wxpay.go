package controller

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	payment "github.com/QuantumNous/new-api/service/payment"
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
//
// The default delegates to payment.NewWechatPayServiceFromSettings so the HTTP
// path and the background sweep construct the
// provider the same way. HTTP paths still hard-require a non-nil verifier;
// the wrapper below enforces that invariant which the notify handler relies
// on (the sweep tolerates a nil verifier because Close doesn't decrypt).
var wechatPayServiceProvider = func() (payment.WechatPayService, error) {
	svc, err := payment.NewWechatPayServiceFromSettings()
	if err != nil {
		return nil, err
	}
	if setting.GetWechatPayVerifier() == nil {
		return nil, fmt.Errorf("wechat pay verifier not available")
	}
	return svc, nil
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

	payAmountCents := payment.MoneyToCents(payMoney)
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

	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount := decimal.NewFromInt(amount)
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		amount = dAmount.Div(dQuotaPerUnit).IntPart()
	}

	now := time.Now().Unix()
	expireAt := now + wxpayOrderExpireSeconds
	topUp := &model.TopUp{
		UserId:          userId,
		Amount:          amount,
		Money:           payMoney,
		TradeNo:         tradeNo,
		PaymentMethod:   PaymentMethodWxpay,
		PaymentProvider: model.PaymentProviderWxpay,
		CreateTime:      now,
		Status:          common.TopUpStatusPending,
		PayAmountCents:  payAmountCents,
		ExpireTime:      expireAt,
	}
	// Insert the local order BEFORE calling NativePrepay so a failed Insert
	// does not leave an orphan order on WeChat's side. WeChat will not have
	// any record of this trade_no until Prepay succeeds — and if Prepay then
	// fails, the local pending row is harmless (the expiry sweep will close
	// it without ever asking WeChat).
	if err := topUp.Insert(); err != nil {
		log.Printf("wxpay topup insert failed: %v (trade_no=%s)", err, tradeNo)
		c.JSON(200, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	// Pass the same TTL upstream so a payment arriving after our local sweep
	// is also rejected by WeChat, instead of WeChat keeping its default
	// 2-hour acceptance window.
	codeURL, err := svc.NativePrepay(c.Request.Context(), tradeNo, description, payAmountCents, notifyURL, time.Unix(expireAt, 0))
	if err != nil {
		log.Printf("wxpay NativePrepay failed: %v", err)
		c.JSON(200, gin.H{"message": "error", "data": "拉起支付失败"})
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

	if topUp.PayAmountCents <= 0 {
		// Direct-integration orders ALWAYS set PayAmountCents > 0 at insert time
		// (see RequestWxpay). A zero here means data corruption or a row from
		// a different channel that should never have reached this notify path.
		reason := fmt.Sprintf("pay_amount_cents=0 on wxpay order tradeNo=%s — refusing to accept notify", tradeNo)
		log.Printf("wxpay notify: %s", reason)
		_ = model.SetTopUpAnomaly(model.DB, tradeNo, reason)
		wxpayFail(c, http.StatusBadRequest, "invalid order amount")
		return
	}
	if result.AmountTotal != topUp.PayAmountCents {
		reason := fmt.Sprintf("amount mismatch: notify=%d expected=%d", result.AmountTotal, topUp.PayAmountCents)
		log.Printf("wxpay notify: %s tradeNo=%s", reason, tradeNo)
		_ = model.SetTopUpAnomaly(model.DB, tradeNo, reason)
		wxpayFail(c, http.StatusBadRequest, "amount mismatch")
		return
	}
	if result.Raw != "" {
		if err := model.SetTopUpCallbackRaw(model.DB, tradeNo, result.Raw); err != nil {
			log.Printf("wxpay notify: persist callback raw failed tradeNo=%s err=%v", tradeNo, err)
		}
	}

	granted, err := finalizeTopUpSuccess(topUp, result.TransactionId, result.PaidAt, "使用微信")
	if err != nil {
		// Transaction rolled back: status NOT flipped, quota NOT changed.
		// Let WeChat retry by responding FAIL. Safe because next delivery
		// will re-enter the same conditional UPDATE.
		log.Printf("wxpay notify: finalize transaction failed tradeNo=%s err=%v", tradeNo, err)
		wxpayFail(c, http.StatusInternalServerError, "db error")
		return
	}
	if !granted {
		log.Printf("wxpay notify: idempotent skip tradeNo=%s (already completed)", tradeNo)
	}

	wxpaySuccess(c)
}
