package controller

import (
	"fmt"
	"log"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
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
// The default implementation reads the cached client from setting and wraps it
// in a RealAlipayService.
var alipayServiceProvider = func() (service.AlipayService, error) {
	client := setting.GetAlipayClient()
	if client == nil {
		return nil, fmt.Errorf("alipay client not configured")
	}
	return service.NewRealAlipayService(client), nil
}

// AlipayPayRequest is the JSON body for POST /api/user/alipay/pay.
type AlipayPayRequest struct {
	// Amount is the quantity of units to purchase (same semantic as Stripe/PayPal).
	Amount int64 `json:"amount"`
}

// computeAlipayPayMoney returns the CNY amount (yuan) to charge for the given
// requested unit count and user group. It re-uses the shared getPayMoney
// helper which already factors in QuotaDisplayType, group ratio and preset
// discount.
func computeAlipayPayMoney(amount int64, group string) float64 {
	return getPayMoney(amount, group)
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

	payMoney := computeAlipayPayMoney(req.Amount, group)
	if payMoney < 0.01 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}

	payAmountCents := common.MoneyToCents(payMoney)
	if payAmountCents <= 0 {
		c.JSON(200, gin.H{"message": "error", "data": "充值金额过低"})
		return
	}
	totalAmountStr := common.CentsToMoneyStr(payAmountCents)

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
		Currency:       "CNY",
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
