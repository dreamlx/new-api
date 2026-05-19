package controller

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/shopspring/decimal"
	"github.com/smartwalle/alipay/v3"
)

func GetTopUpInfo(c *gin.Context) {
	// 获取支付方式
	payMethods := operation_setting.PayMethods

	// 如果启用了 Stripe 支付，添加到支付方法列表
	if setting.StripeApiSecret != "" && setting.StripeWebhookSecret != "" && setting.StripePriceId != "" {
		// 检查是否已经包含 Stripe
		hasStripe := false
		for _, method := range payMethods {
			if method["type"] == "stripe" {
				hasStripe = true
				break
			}
		}

		if !hasStripe {
			stripeMethod := map[string]string{
				"name":      "Stripe",
				"type":      "stripe",
				"color":     "rgba(var(--semi-purple-5), 1)",
				"min_topup": strconv.Itoa(setting.StripeMinTopUp),
			}
			payMethods = append(payMethods, stripeMethod)
		}
	}

	// 如果启用了 Waffo 支付，添加到支付方法列表
	enableWaffo := setting.WaffoEnabled &&
		((!setting.WaffoSandbox &&
			setting.WaffoApiKey != "" &&
			setting.WaffoPrivateKey != "" &&
			setting.WaffoPublicCert != "") ||
			(setting.WaffoSandbox &&
				setting.WaffoSandboxApiKey != "" &&
				setting.WaffoSandboxPrivateKey != "" &&
				setting.WaffoSandboxPublicCert != ""))
	if enableWaffo {
		hasWaffo := false
		for _, method := range payMethods {
			if method["type"] == "waffo" {
				hasWaffo = true
				break
			}
		}

		if !hasWaffo {
			waffoMethod := map[string]string{
				"name":      "Waffo (Global Payment)",
				"type":      "waffo",
				"color":     "rgba(var(--semi-blue-5), 1)",
				"min_topup": strconv.Itoa(setting.WaffoMinTopUp),
			}
			payMethods = append(payMethods, waffoMethod)
		}
	}

	// 如果启用了 PayPal 支付，添加到支付方法列表
	if setting.PayPalClientId != "" && setting.PayPalClientSecret != "" {
		hasPayPal := false
		for _, method := range payMethods {
			if method["type"] == "paypal" {
				hasPayPal = true
				break
			}
		}

		if !hasPayPal {
			paypalMethod := map[string]string{
				"name":      "PayPal",
				"type":      "paypal",
				"color":     "rgba(var(--semi-blue-6), 1)",
				"min_topup": strconv.Itoa(setting.PayPalMinTopUp),
			}
			payMethods = append(payMethods, paypalMethod)
		}
	}

	data := gin.H{
		"enable_online_topup": operation_setting.PayAddress != "" && operation_setting.EpayId != "" && operation_setting.EpayKey != "",
		"enable_stripe_topup": setting.StripeApiSecret != "" && setting.StripeWebhookSecret != "" && setting.StripePriceId != "",
		"enable_creem_topup":  setting.CreemApiKey != "" && setting.CreemProducts != "[]",
		"enable_waffo_topup": enableWaffo,
		"enable_paypal_topup": setting.PayPalClientId != "" && setting.PayPalClientSecret != "",
		"enable_alipay_topup": setting.AlipayEnabled,
		"enable_wxpay_topup":  setting.WxpayEnabled,
		"waffo_pay_methods": func() interface{} {
			if enableWaffo {
				return setting.GetWaffoPayMethods()
			}
			return nil
		}(),
		"creem_products": setting.CreemProducts,
		"pay_methods":         payMethods,
		"min_topup":           operation_setting.MinTopUp,
		"stripe_min_topup":    setting.StripeMinTopUp,
		"waffo_min_topup":     setting.WaffoMinTopUp,
		"paypal_min_topup":    setting.PayPalMinTopUp,
		"alipay_min_topup":    setting.AlipayMinTopUp,
		"wxpay_min_topup":     setting.WxpayMinTopUp,
		"amount_options":      operation_setting.GetPaymentSetting().AmountOptions,
		"discount":            operation_setting.GetPaymentSetting().AmountDiscount,
	}
	common.ApiSuccess(c, data)
}

type EpayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"`
}

type AmountRequest struct {
	Amount int64 `json:"amount"`
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

func getPayMoney(amount int64, group string) float64 {
	dAmount := decimal.NewFromInt(amount)
	// 充值金额以“展示类型”为准：
	// - USD/CNY: 前端传 amount 为金额单位；TOKENS: 前端传 tokens，需要换成 USD 金额
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		dAmount = dAmount.Div(dQuotaPerUnit)
	}

	topupGroupRatio := common.GetTopupGroupRatio(group)
	if topupGroupRatio == 0 {
		topupGroupRatio = 1
	}

	dTopupGroupRatio := decimal.NewFromFloat(topupGroupRatio)
	dPrice := decimal.NewFromFloat(operation_setting.Price)
	// apply optional preset discount by the original request amount (if configured), default 1.0
	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok {
		if ds > 0 {
			discount = ds
		}
	}
	dDiscount := decimal.NewFromFloat(discount)

	payMoney := dAmount.Mul(dPrice).Mul(dTopupGroupRatio).Mul(dDiscount)

	return payMoney.InexactFloat64()
}

func getMinTopup() int64 {
	minTopup := operation_setting.MinTopUp
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dMinTopup := decimal.NewFromInt(int64(minTopup))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		minTopup = int(dMinTopup.Mul(dQuotaPerUnit).IntPart())
	}
	return int64(minTopup)
}

func RequestEpay(c *gin.Context) {
	var req EpayRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}
	if req.Amount < getMinTopup() {
		c.JSON(200, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney < 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	if !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		c.JSON(200, gin.H{"message": "error", "data": "支付方式不存在"})
		return
	}

	// Only epay-compatible payment methods should reach here
	// PayPal, Stripe, Waffo, Creem have their own dedicated endpoints
	if req.PaymentMethod == "paypal" || req.PaymentMethod == "stripe" || req.PaymentMethod == "waffo" || req.PaymentMethod == "creem" {
		c.JSON(200, gin.H{"message": "error", "data": "该支付方式有专属接口，请调用相应接口"})
		return
	}

	callBackAddress := service.GetCallbackAddress()
	returnUrl, _ := url.Parse(system_setting.ServerAddress + "/console/log")
	notifyUrl, _ := url.Parse(callBackAddress + "/api/user/epay/notify")
	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("USR%dNO%s", id, tradeNo)
	client := GetEpayClient()
	if client == nil {
		c.JSON(200, gin.H{"message": "error", "data": "当前管理员未配置支付信息"})
		return
	}
	uri, params, err := client.Purchase(&epay.PurchaseArgs{
		Type:           req.PaymentMethod,
		ServiceTradeNo: tradeNo,
		Name:           fmt.Sprintf("TUC%d", req.Amount),
		Money:          strconv.FormatFloat(payMoney, 'f', 2, 64),
		Device:         epay.PC,
		NotifyUrl:      notifyUrl,
		ReturnUrl:      returnUrl,
	})
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "拉起支付失败"})
		return
	}
	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount := decimal.NewFromInt(int64(amount))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		amount = dAmount.Div(dQuotaPerUnit).IntPart()
	}
	topUp := &model.TopUp{
		UserId:        id,
		Amount:        amount,
		Money:         payMoney,
		TradeNo:       tradeNo,
		PaymentMethod: req.PaymentMethod,
		CreateTime:    time.Now().Unix(),
		Status:        "pending",
	}
	err = topUp.Insert()
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "创建订单失败"})
		return
	}
	c.JSON(200, gin.H{"message": "success", "data": params, "url": uri})
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
	var params map[string]string

	if c.Request.Method == "POST" {
		// POST 请求：从 POST body 解析参数
		if err := c.Request.ParseForm(); err != nil {
			log.Println("易支付回调POST解析失败:", err)
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

	if len(params) == 0 {
		log.Println("易支付回调参数为空")
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	client := GetEpayClient()
	if client == nil {
		log.Println("易支付回调失败 未找到配置信息")
		_, err := c.Writer.Write([]byte("fail"))
		if err != nil {
			log.Println("易支付回调写入失败")
		}
		return
	}
	verifyInfo, err := client.Verify(params)
	if err == nil && verifyInfo.VerifyStatus {
		_, err := c.Writer.Write([]byte("success"))
		if err != nil {
			log.Println("易支付回调写入失败")
		}
	} else {
		_, err := c.Writer.Write([]byte("fail"))
		if err != nil {
			log.Println("易支付回调写入失败")
		}
		log.Println("易支付回调签名验证失败")
		return
	}

	if verifyInfo.TradeStatus == epay.StatusTradeSuccess {
		log.Println(verifyInfo)
		LockOrder(verifyInfo.ServiceTradeNo)
		defer UnlockOrder(verifyInfo.ServiceTradeNo)
		topUp := model.GetTopUpByTradeNo(verifyInfo.ServiceTradeNo)
		if topUp == nil {
			log.Printf("易支付回调未找到订单: %v", verifyInfo)
			return
		}
		if topUp.Status == "pending" {
			topUp.Status = "success"
			err := topUp.Update()
			if err != nil {
				log.Printf("易支付回调更新订单失败: %v", topUp)
				return
			}
			//user, _ := model.GetUserById(topUp.UserId, false)
			//user.Quota += topUp.Amount * 500000
			dAmount := decimal.NewFromInt(int64(topUp.Amount))
			dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
			quotaToAdd := int(dAmount.Mul(dQuotaPerUnit).IntPart())
			err = model.IncreaseUserQuota(topUp.UserId, quotaToAdd, true)
			if err != nil {
				log.Printf("易支付回调更新用户失败: %v", topUp)
				return
			}
			log.Printf("易支付回调更新用户成功 %v", topUp)
			model.RecordLog(topUp.UserId, model.LogTypeTopup, fmt.Sprintf("使用在线充值成功，充值金额: %v，支付金额：%f", logger.LogQuota(quotaToAdd), topUp.Money))
		}
	} else {
		log.Printf("易支付异常回调: %v", verifyInfo)
	}
}

// GetTopUpStatus returns the current state of a top-up order by trade_no.
//
// GET /api/user/topup/status?trade_no=USR...
//
// Ownership: the order's UserId MUST match the JWT-authenticated user id.
// Foreign or non-existent trade numbers return the same 404 "订单不存在"
// response, so a probing user cannot enumerate or confirm other users'
// trade numbers via response-shape differences.
func GetTopUpStatus(c *gin.Context) {
	tradeNo := c.Query("trade_no")
	if tradeNo == "" {
		c.JSON(http.StatusOK, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	userId := c.GetInt("id")
	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil || topUp.UserId != userId {
		// Return the same response for "not found" and "not yours" to avoid
		// leaking the existence of other users' trade numbers.
		c.JSON(http.StatusNotFound, gin.H{"message": "error", "data": "订单不存在"})
		return
	}

	// Active-query fallback: if the order is still pending and stale enough
	// that the async webhook should have arrived, poll the upstream provider
	// once. Reduces user-visible latency when the webhook is slow without
	// hammering the SDK on every poll.
	if topUp.Status == common.TopUpStatusPending && (time.Now().Unix()-topUp.CreateTime) > topUpActiveQueryStaleSeconds {
		if tryActiveQueryTopUp(c.Request.Context(), topUp) {
			// Re-fetch only when the active query completed the order, so the
			// response reflects the new success state. Anomaly state (set on
			// amount mismatch) is intentionally not surfaced; the user keeps
			// seeing "pending" while ops investigates via logs.
			if refreshed := model.GetTopUpByTradeNo(tradeNo); refreshed != nil {
				topUp = refreshed
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "success",
		"data": gin.H{
			"trade_no":       topUp.TradeNo,
			"status":         topUp.Status,
			"amount":         topUp.Amount,
			"money":          topUp.Money,
			"payment_method": topUp.PaymentMethod,
			"create_time":    topUp.CreateTime,
			"paid_at":        topUp.PaidAt,
			"expire_time":    topUp.ExpireTime,
		},
	})
}

// topUpActiveQueryStaleSeconds is the minimum age (in seconds) a pending
// order must reach before GetTopUpStatus will fall back to an upstream SDK
// query. The threshold exists so frequent polling from the QR-code page does
// not turn into an SDK request storm; the async webhook usually wins within
// a few seconds of the user paying.
const topUpActiveQueryStaleSeconds = 5

// tryActiveQueryTopUp pokes the upstream provider for a single pending order
// and, if the provider confirms the payment, completes the order through
// the same idempotent path as the async notify handlers. Errors are logged
// and swallowed: failure to actively query MUST NOT fail the status read.
//
// Returns true only when the order transitioned to a terminal SUCCESS state
// during this call (so the caller knows to re-fetch). Returns false on any
// non-success outcome including network errors, provider-still-pending, and
// amount mismatches (which set anomaly but stay invisible to the user).
func tryActiveQueryTopUp(ctx context.Context, topUp *model.TopUp) bool {
	switch topUp.PaymentMethod {
	case PaymentMethodAlipay:
		return activeQueryAlipay(ctx, topUp)
	case PaymentMethodWxpay:
		return activeQueryWxpay(ctx, topUp)
	}
	return false
}

func activeQueryAlipay(ctx context.Context, topUp *model.TopUp) bool {
	svc, err := alipayServiceProvider()
	if err != nil || svc == nil {
		log.Printf("topup active query: alipay service unavailable tradeNo=%s err=%v", topUp.TradeNo, err)
		return false
	}
	rsp, err := svc.TradeQuery(ctx, topUp.TradeNo)
	if err != nil || rsp == nil {
		log.Printf("topup active query: alipay TradeQuery failed tradeNo=%s err=%v", topUp.TradeNo, err)
		return false
	}
	if rsp.TradeStatus != alipay.TradeStatusSuccess && rsp.TradeStatus != alipay.TradeStatusFinished {
		return false
	}
	cents, err := common.AlipayAmountToCents(rsp.TotalAmount)
	if err != nil {
		reason := fmt.Sprintf("active query: invalid total_amount=%q err=%v", rsp.TotalAmount, err)
		log.Printf("topup active query: %s tradeNo=%s", reason, topUp.TradeNo)
		_ = model.SetTopUpAnomaly(model.DB, topUp.TradeNo, reason)
		return false
	}
	if topUp.PayAmountCents > 0 && cents != topUp.PayAmountCents {
		reason := fmt.Sprintf("active query amount mismatch: provider=%d expected=%d", cents, topUp.PayAmountCents)
		log.Printf("topup active query: %s tradeNo=%s", reason, topUp.TradeNo)
		_ = model.SetTopUpAnomaly(model.DB, topUp.TradeNo, reason)
		return false
	}
	paidAt := alipayParseGmt(rsp.SendPayDate)
	_, err = finalizeTopUpSuccess(topUp, rsp.TradeNo, paidAt, "使用支付宝")
	if err != nil {
		log.Printf("topup active query: alipay finalize failed tradeNo=%s err=%v", topUp.TradeNo, err)
	}
	// err==nil means the row is now terminal (either we flipped it this call,
	// or a concurrent caller already did and we observed rows=0).
	return err == nil
}

func activeQueryWxpay(ctx context.Context, topUp *model.TopUp) bool {
	svc, err := wechatPayServiceProvider()
	if err != nil || svc == nil {
		log.Printf("topup active query: wxpay service unavailable tradeNo=%s err=%v", topUp.TradeNo, err)
		return false
	}
	tx, err := svc.QueryOrderByOutTradeNo(ctx, topUp.TradeNo)
	if err != nil || tx == nil {
		log.Printf("topup active query: wxpay QueryOrderByOutTradeNo failed tradeNo=%s err=%v", topUp.TradeNo, err)
		return false
	}
	if tx.TradeState == nil || *tx.TradeState != "SUCCESS" {
		return false
	}
	var providerTxId string
	if tx.TransactionId != nil {
		providerTxId = *tx.TransactionId
	}
	var amountTotal int64
	if tx.Amount != nil && tx.Amount.Total != nil {
		amountTotal = *tx.Amount.Total
	}
	if topUp.PayAmountCents > 0 && amountTotal != topUp.PayAmountCents {
		reason := fmt.Sprintf("active query amount mismatch: provider=%d expected=%d", amountTotal, topUp.PayAmountCents)
		log.Printf("topup active query: %s tradeNo=%s", reason, topUp.TradeNo)
		_ = model.SetTopUpAnomaly(model.DB, topUp.TradeNo, reason)
		return false
	}
	var paidAt int64
	if tx.SuccessTime != nil {
		if pt, perr := time.Parse(time.RFC3339, *tx.SuccessTime); perr == nil {
			paidAt = pt.Unix()
		}
	}
	_, err = finalizeTopUpSuccess(topUp, providerTxId, paidAt, "使用微信")
	if err != nil {
		log.Printf("topup active query: wxpay finalize failed tradeNo=%s err=%v", topUp.TradeNo, err)
	}
	return err == nil
}

// finalizeTopUpSuccess is the shared idempotent completion path used by the
// alipay/wxpay async notify handlers and by the active-query fallback in
// GetTopUpStatus. It performs:
//  1. CompleteTopUpByCondition (multi-replica safe; only flips pending->success)
//  2. IncreaseUserQuota
//  3. RecordLog
//
// providerPrefix is the human-readable Chinese provider name used in the log
// message ("使用支付宝" / "使用微信"), preserving the legacy log format that
// operators may be parsing.
//
// Returns granted=true only when this call actually flipped the order to
// success (RowsAffected==1). granted=false means a concurrent completion
// won the race; the caller should treat it as a successful idempotent skip.
//
// The caller MUST hold LockOrder(topUp.TradeNo) when invoking this from a
// path where contention with other notify deliveries is expected; the DB
// conditional update is the authoritative cross-replica boundary.
func finalizeTopUpSuccess(topUp *model.TopUp, providerTxId string, paidAt int64, providerPrefix string) (granted bool, err error) {
	dAmount := decimal.NewFromInt(topUp.Amount)
	dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
	quotaToAdd := int(dAmount.Mul(dQuotaPerUnit).IntPart())

	rowsAffected, err := model.CompleteTopUpByCondition(model.DB, topUp.TradeNo, providerTxId, paidAt, int64(quotaToAdd))
	if err != nil {
		return false, err
	}
	if rowsAffected == 0 {
		// Already completed by a concurrent caller; idempotent no-op.
		return false, nil
	}

	if quotaToAdd <= 0 {
		return true, nil
	}
	if err := model.IncreaseUserQuota(topUp.UserId, quotaToAdd, true); err != nil {
		// Status already flipped to success; surface the error to the caller
		// but treat the order as granted so the caller does not retry the
		// completion (which would be a no-op anyway).
		log.Printf("finalizeTopUpSuccess: IncreaseUserQuota failed tradeNo=%s userId=%d err=%v",
			topUp.TradeNo, topUp.UserId, err)
		return true, err
	}
	model.RecordLog(topUp.UserId, model.LogTypeTopup,
		fmt.Sprintf("%s在线充值成功，充值金额：%s，订单号：%s",
			providerPrefix, logger.LogQuota(quotaToAdd), topUp.TradeNo))
	return true, nil
}

func RequestAmount(c *gin.Context) {
	var req AmountRequest
	err := c.ShouldBindJSON(&req)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "参数错误"})
		return
	}

	if req.Amount < getMinTopup() {
		c.JSON(200, gin.H{"message": "error", "data": fmt.Sprintf("充值数量不能小于 %d", getMinTopup())})
		return
	}
	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		c.JSON(200, gin.H{"message": "error", "data": "获取用户分组失败"})
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney <= 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	c.JSON(200, gin.H{"message": "success", "data": strconv.FormatFloat(payMoney, 'f', 2, 64)})
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

	if err := model.ManualCompleteTopUp(req.TradeNo); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}

