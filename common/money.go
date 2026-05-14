package common

import (
	"fmt"
	"math"
	"strconv"
)

// MoneyToCents converts yuan (float64) to cents (int64), rounding to nearest
func MoneyToCents(money float64) int64 {
	return int64(math.Round(money * 100))
}

// CentsToMoneyStr converts cents (int64) to yuan string (Alipay format: "14.50")
func CentsToMoneyStr(cents int64) string {
	return fmt.Sprintf("%.2f", float64(cents)/100.0)
}

// AlipayAmountToCents parses an Alipay amount string (yuan) to cents
func AlipayAmountToCents(amountStr string) (int64, error) {
	if amountStr == "" {
		return 0, fmt.Errorf("invalid amount: empty string")
	}
	money, err := strconv.ParseFloat(amountStr, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid amount: %s", amountStr)
	}
	if money < 0 {
		return 0, fmt.Errorf("negative amount: %s", amountStr)
	}
	return MoneyToCents(money), nil
}
