package payment

import (
	"fmt"
	"math"
	"strconv"
)

// Payment-domain money helpers. Kept in the payment subpackage because these
// conversions encode payment-specific rules (cents=fen, rounding policy, and
// Alipay's "%.2f" wire format).

// MoneyToCents converts yuan (float64) to cents (int64), rounding to nearest.
func MoneyToCents(money float64) int64 {
	return int64(math.Round(money * 100))
}

// CentsToMoneyStr converts cents (int64) to a yuan string in Alipay's
// expected "%.2f" wire format (e.g. 1450 -> "14.50").
func CentsToMoneyStr(cents int64) string {
	return fmt.Sprintf("%.2f", float64(cents)/100.0)
}

// AlipayAmountToCents parses an Alipay amount string (yuan) to cents.
// Returns an error for empty / non-numeric / negative input so callers
// can refuse the notify rather than silently treating it as zero.
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
