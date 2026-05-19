package controller

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"github.com/smartwalle/alipay/v3"
)

const (
	// PaymentMethodAlipay identifies the Alipay direct-integration channel.
	PaymentMethodAlipay = "alipay"

	// alipayOrderExpireSeconds is the order time-to-live (30 minutes).
	alipayOrderExpireSeconds = 30 * 60

	// alipaySubjectFormat is the order subject shown to the user on Alipay.
	alipaySubjectFormat = "TopUp Order #%s"

	alipayNotifyPath = "/api/user/alipay/notify"
	alipayReturnPath = "/api/user/alipay/return"
)

// alipayServiceProvider returns an AlipayService instance. It is exposed as a
// package-level variable so tests can inject a mock implementation without
// touching the underlying SDK or setting state.
//
// The default delegates to service.NewAlipayServiceFromSettings so the HTTP
// path and the background sweep (service/topup_expiry.go) construct the
// provider the same way.
var alipayServiceProvider = service.NewAlipayServiceFromSettings

// AlipayPayRequest is the JSON body for POST /api/user/alipay/pay.
type AlipayPayRequest struct {
	// Amount is the quantity of units to purchase (same semantic as Stripe/PayPal).
	Amount int64 `json:"amount"`
}

// RequestAlipay creates an Alipay order and returns the PC payment URL.
//
// POST /api/user/alipay/pay
//
//	{"amount": <int64>}
//
// Response:
//
//	{"message":"success","data":{"pay_link":"https://...","trade_no":"USRxxNOyy"}}
func RequestAlipay(c *gin.Context) {
	if !setting.AlipayEnabled {
		c.JSON(200, gin.H{"message": "error", "data": "支付宝支付未启用"})
		return
	}

	var req AlipayPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	minTopUp := int64(setting.AlipayMinTopUp)
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
		// Treat group lookup failure as soft-fail with default ratio; this is
		// also how the legacy epay path behaves except it errors out. We mirror
		// the legacy behaviour to keep callers consistent.
		c.JSON(200, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}

	payMoney := getPayMoney(req.Amount, group)
	if payMoney < 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	payAmountCents := service.MoneyToCents(payMoney)
	if payAmountCents <= 0 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	totalAmountStr := service.CentsToMoneyStr(payAmountCents)

	svc, err := alipayServiceProvider()
	if err != nil || svc == nil {
		log.Printf("alipay service unavailable: %v", err)
		c.JSON(200, gin.H{"message": "error", "data": "支付服务暂不可用"})
		return
	}

	tradeNo := fmt.Sprintf("USR%dNO%s%d", userId, common.GetRandomString(6), time.Now().Unix())
	callbackAddress := service.GetCallbackAddress()
	notifyURL := callbackAddress + alipayNotifyPath
	returnURL := system_setting.ServerAddress + alipayReturnPath
	subject := fmt.Sprintf(alipaySubjectFormat, tradeNo)

	payURL, err := svc.TradePagePay(tradeNo, subject, totalAmountStr, notifyURL, returnURL)
	if err != nil {
		log.Printf("alipay TradePagePay failed: %v", err)
		c.JSON(200, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}

	// Compute display Amount in same units as other adapters (see topup.go).
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
		PaymentMethod:  PaymentMethodAlipay,
		CreateTime:     now,
		Status:         common.TopUpStatusPending,
		PayAmountCents: payAmountCents,
		ExpireTime:     now + alipayOrderExpireSeconds,
	}
	if err := topUp.Insert(); err != nil {
		log.Printf("alipay topup insert failed: %v (trade_no=%s)", err, tradeNo)
		c.JSON(200, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}

	c.JSON(200, gin.H{
		"message": "success",
		"data": gin.H{
			"pay_link": payURL,
			"trade_no": tradeNo,
		},
	})
}

// (AlipayNotify implementation lives in the next task.)

// alipayNotifyResponse is what the SDK ultimately expects us to send back.
// Alipay's spec dictates a literal "success" (no JSON) when we've accepted
// the callback; any other body triggers retries (up to 8 times). We return
// "failure" for non-recoverable validation errors and for signature errors
// so that the retry pressure surfaces operator issues quickly.
const (
	alipayNotifyOK  = "success"
	alipayNotifyErr = "failure"
)

// alipayParseGmt converts an Alipay "gmt_payment" string like
// "2026-05-14 12:00:01" (Asia/Shanghai) to a Unix timestamp. On parse failure
// it logs and falls back to the current time, since timestamp accuracy is
// not critical to idempotency or accounting — but a silent fallback would
// mask format changes from Alipay or upstream config errors.
func alipayParseGmt(gmt string) int64 {
	if gmt == "" {
		return time.Now().Unix()
	}
	// Alipay uses Asia/Shanghai for gmt_payment.
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		log.Printf("alipayParseGmt: LoadLocation(Asia/Shanghai) failed: %v (falling back to local)", err)
		loc = time.Local
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", gmt, loc)
	if err != nil {
		log.Printf("alipayParseGmt: ParseInLocation(%q) failed: %v (falling back to now)", gmt, err)
		return time.Now().Unix()
	}
	return t.Unix()
}

// AlipayNotify handles asynchronous payment notifications from Alipay.
//
// POST /api/user/alipay/notify (application/x-www-form-urlencoded)
//
// Validation order (all must pass before granting quota):
//  1. Parse form
//  2. VerifySign
//  3. DecodeNotification
//  4. app_id matches setting.AlipayAppId
//  5. seller_id matches setting.AlipaySellerId (when configured)
//  6. trade_status is TRADE_SUCCESS or TRADE_FINISHED
//  7. out_trade_no exists in DB as a pending TopUp
//  8. total_amount (cents) equals TopUp.PayAmountCents
//
// Idempotency is enforced via the (in-memory) LockOrder mutex and the
// CompleteTopUpByCondition conditional update (multi-replica safe). Quota is
// granted only when RowsAffected == 1.
//
// On amount mismatch the order is marked anomaly and we respond "failure" so
// Alipay retries while the operator investigates. On signature/decode errors
// we respond "failure" without touching state. On a duplicate (already
// completed) callback we still respond "success".
func AlipayNotify(c *gin.Context) {
	if err := c.Request.ParseForm(); err != nil {
		log.Printf("alipay notify: parse form failed: %v", err)
		c.String(http.StatusOK, alipayNotifyErr)
		return
	}
	values := c.Request.PostForm
	if len(values) == 0 {
		// Some setups POST as application/x-www-form-urlencoded; others may
		// arrive as multipart or query-string. Fall back to the merged Form.
		values = c.Request.Form
	}
	if len(values) == 0 {
		log.Printf("alipay notify: empty form")
		c.String(http.StatusOK, alipayNotifyErr)
		return
	}

	svc, err := alipayServiceProvider()
	if err != nil || svc == nil {
		log.Printf("alipay notify: service unavailable: %v", err)
		c.String(http.StatusOK, alipayNotifyErr)
		return
	}

	ctx := context.Background()

	if err := svc.VerifySign(ctx, values); err != nil {
		log.Printf("alipay notify: signature verification failed: %v", err)
		c.String(http.StatusOK, alipayNotifyErr)
		return
	}

	notification, err := svc.DecodeNotification(ctx, values)
	if err != nil || notification == nil {
		log.Printf("alipay notify: decode failed: %v", err)
		c.String(http.StatusOK, alipayNotifyErr)
		return
	}

	// Identity checks: app_id and seller_id (when set) must match our config.
	if notification.AppId != "" && setting.AlipayAppId != "" && notification.AppId != setting.AlipayAppId {
		log.Printf("alipay notify: app_id mismatch: got=%s want=%s tradeNo=%s",
			notification.AppId, setting.AlipayAppId, notification.OutTradeNo)
		c.String(http.StatusOK, alipayNotifyErr)
		return
	}
	if setting.AlipaySellerId != "" && notification.SellerId != "" && notification.SellerId != setting.AlipaySellerId {
		log.Printf("alipay notify: seller_id mismatch: got=%s want=%s tradeNo=%s",
			notification.SellerId, setting.AlipaySellerId, notification.OutTradeNo)
		c.String(http.StatusOK, alipayNotifyErr)
		return
	}

	// Only accept terminal-paid statuses.
	status := notification.TradeStatus
	if status != alipay.TradeStatusSuccess && status != alipay.TradeStatusFinished {
		log.Printf("alipay notify: non-paid trade_status=%s tradeNo=%s", status, notification.OutTradeNo)
		// Non-paid notifications (e.g. WAIT_BUYER_PAY) are still well-formed;
		// respond success so Alipay does not retry them indefinitely.
		c.String(http.StatusOK, alipayNotifyOK)
		return
	}

	tradeNo := notification.OutTradeNo
	if tradeNo == "" {
		log.Printf("alipay notify: missing out_trade_no")
		c.String(http.StatusOK, alipayNotifyErr)
		return
	}

	// Short-circuit lock; reduces duplicate-callback contention within one
	// replica. The DB conditional update is the authoritative idempotency
	// boundary across replicas.
	LockOrder(tradeNo)
	defer UnlockOrder(tradeNo)

	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil {
		log.Printf("alipay notify: top-up not found tradeNo=%s", tradeNo)
		c.String(http.StatusOK, alipayNotifyErr)
		return
	}

	// Amount equality (integer cents) — the canonical safety check.
	notifyCents, err := service.AlipayAmountToCents(notification.TotalAmount)
	if err != nil {
		reason := fmt.Sprintf("invalid total_amount=%q err=%v", notification.TotalAmount, err)
		log.Printf("alipay notify: %s tradeNo=%s", reason, tradeNo)
		_ = model.SetTopUpAnomaly(model.DB, tradeNo, reason)
		c.String(http.StatusOK, alipayNotifyErr)
		return
	}
	if topUp.PayAmountCents <= 0 {
		// Direct-integration orders ALWAYS set PayAmountCents > 0 at insert time
		// (see RequestAlipay). A zero here means data corruption or a row from
		// a different channel that should never have reached this notify path.
		reason := fmt.Sprintf("pay_amount_cents=0 on alipay order tradeNo=%s — refusing to accept notify", tradeNo)
		log.Printf("alipay notify: %s", reason)
		_ = model.SetTopUpAnomaly(model.DB, tradeNo, reason)
		c.String(http.StatusOK, alipayNotifyErr)
		return
	}
	if notifyCents != topUp.PayAmountCents {
		reason := fmt.Sprintf("amount mismatch: notify=%d expected=%d", notifyCents, topUp.PayAmountCents)
		log.Printf("alipay notify: %s tradeNo=%s", reason, tradeNo)
		_ = model.SetTopUpAnomaly(model.DB, tradeNo, reason)
		c.String(http.StatusOK, alipayNotifyErr)
		return
	}

	// Authoritative idempotent completion (multi-replica safe). The shared
	// helper performs CompleteTopUpByCondition + IncreaseUserQuota + RecordLog.
	paidAt := alipayParseGmt(notification.GmtPayment)
	granted, err := finalizeTopUpSuccess(topUp, notification.TradeNo, paidAt, "使用支付宝")
	if err != nil && !granted {
		// DB conditional update itself failed; let Alipay retry.
		log.Printf("alipay notify: CompleteTopUpByCondition failed tradeNo=%s err=%v", tradeNo, err)
		c.String(http.StatusOK, alipayNotifyErr)
		return
	}
	if err != nil {
		// granted==true but IncreaseUserQuota failed after the status flip;
		// status is already success, so we ack success and surface the
		// inconsistency via logs for ops to reconcile.
		log.Printf("alipay notify: IncreaseUserQuota failed tradeNo=%s userId=%d err=%v",
			tradeNo, topUp.UserId, err)
	}
	if !granted && err == nil {
		log.Printf("alipay notify: idempotent skip tradeNo=%s (already completed)", tradeNo)
	}

	c.String(http.StatusOK, alipayNotifyOK)
}

// alipayReturnRedirect issues a 302 to the console topup page with the given
// status (success|pending|fail). The frontend reads `pay` from the query
// string and renders the appropriate UI. The trade_no is forwarded so the
// console can highlight the relevant order in the history list.
func alipayReturnRedirect(c *gin.Context, status, tradeNo string) {
	q := url.Values{}
	q.Set("pay", status)
	if tradeNo != "" {
		q.Set("trade_no", tradeNo)
	}
	target := system_setting.ServerAddress + "/console/topup?" + q.Encode()
	c.Redirect(http.StatusFound, target)
}

// AlipayReturn handles the synchronous browser redirect after the user pays
// on the Alipay-hosted page.
//
// GET /api/user/alipay/return
//
// This is a UX-only convenience: the authoritative state transition lives in
// AlipayNotify (server-to-server). We NEVER grant quota here — the buyer
// could open the return URL manually and forge query params. Worst case the
// user sees `pay=pending` for a few seconds until the async notify wins the
// race; that is acceptable.
//
// Behaviour:
//  1. If signature verification fails, redirect to pay=fail.
//  2. If out_trade_no is missing, redirect to pay=fail (we can't even tell
//     the user which order they paid for, so treat as a malformed bounce).
//  3. If the local TopUp is already success (notify won the race),
//     redirect to pay=success.
//  4. Otherwise (including unknown order), redirect to pay=pending.
func AlipayReturn(c *gin.Context) {
	values := c.Request.URL.Query()

	svc, err := alipayServiceProvider()
	if err != nil || svc == nil {
		log.Printf("alipay return: service unavailable: %v", err)
		alipayReturnRedirect(c, "fail", "")
		return
	}
	if err := svc.VerifySign(c.Request.Context(), values); err != nil {
		log.Printf("alipay return: signature verification failed: %v", err)
		alipayReturnRedirect(c, "fail", "")
		return
	}

	tradeNo := values.Get("out_trade_no")
	if tradeNo == "" {
		log.Printf("alipay return: missing out_trade_no")
		alipayReturnRedirect(c, "fail", "")
		return
	}

	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil {
		log.Printf("alipay return: top-up not found tradeNo=%s (notify may not have fired yet)", tradeNo)
		alipayReturnRedirect(c, "pending", tradeNo)
		return
	}

	if topUp.Status == common.TopUpStatusSuccess {
		alipayReturnRedirect(c, "success", tradeNo)
		return
	}
	alipayReturnRedirect(c, "pending", tradeNo)
}

