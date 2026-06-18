package payment

import (
	"errors"

	"github.com/QuantumNous/new-api/setting"
)

// Payment provider factories. Both the HTTP controllers and the background
// sweeps need to construct provider services from the current admin-saved
// settings. Centralising the construction here keeps both layers wired
// through the same path.

// NewAlipayServiceFromSettings returns a RealAlipayService backed by the
// cached SDK client, or an error when Alipay is not configured.
func NewAlipayServiceFromSettings() (AlipayService, error) {
	client := setting.GetAlipayClient()
	if client == nil {
		return nil, errors.New("alipay client not configured")
	}
	return NewRealAlipayService(client), nil
}

// NewWechatPayServiceFromSettings returns a RealWechatPayService backed by
// the cached SDK client + verifier, or an error when WeChat Pay is not
// configured. The verifier MAY be nil for paths that only call Close /
// QueryOrder (no notification consumption); the notify handler enforces a
// non-nil verifier separately.
func NewWechatPayServiceFromSettings() (WechatPayService, error) {
	client := setting.GetWechatPayClient()
	if client == nil {
		return nil, errors.New("wechat pay client not configured")
	}
	verifier := setting.GetWechatPayVerifier()
	return NewRealWechatPayService(client, setting.WxpayMchId, setting.WxpayAppId, setting.WxpayApiV3Key, verifier), nil
}
