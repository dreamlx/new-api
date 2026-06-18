package service

import payment "github.com/QuantumNous/new-api/service/payment"

func MoneyToCents(money float64) int64 { return payment.MoneyToCents(money) }

func CentsToMoneyStr(cents int64) string { return payment.CentsToMoneyStr(cents) }

func AlipayAmountToCents(amountStr string) (int64, error) {
	return payment.AlipayAmountToCents(amountStr)
}
