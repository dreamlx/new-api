package service

import payment "github.com/QuantumNous/new-api/service/payment"

type WechatPayService = payment.WechatPayService
type NotificationResult = payment.NotificationResult
type RefundNotificationResult = payment.RefundNotificationResult
type RealWechatPayService = payment.RealWechatPayService

var NewRealWechatPayService = payment.NewRealWechatPayService
