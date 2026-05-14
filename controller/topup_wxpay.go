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

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

const (
	PaymentMethodWxpay = "wxpay"

	wxpayOrderExpireSeconds = 30 * 60

	wxpayDescriptionFormat = "TopUp Order #%s"

	wxpayNotifyPath = "/api/user/wxpay/notify"
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
