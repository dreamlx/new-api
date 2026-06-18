package service

import payment "github.com/QuantumNous/new-api/service/payment"

type AlipayService = payment.AlipayService
type RealAlipayService = payment.RealAlipayService

var NewRealAlipayService = payment.NewRealAlipayService
