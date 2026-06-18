package service

import payment "github.com/QuantumNous/new-api/service/payment"

func NewAlipayServiceFromSettings() (AlipayService, error) {
	return payment.NewAlipayServiceFromSettings()
}

func NewWechatPayServiceFromSettings() (WechatPayService, error) {
	return payment.NewWechatPayServiceFromSettings()
}
