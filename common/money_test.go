package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMoneyToCents(t *testing.T) {
	require.Equal(t, int64(1450), MoneyToCents(14.50))
	require.Equal(t, int64(1), MoneyToCents(0.01))
	require.Equal(t, int64(0), MoneyToCents(0.001)) // rounds down
	require.Equal(t, int64(1), MoneyToCents(0.009)) // rounds to nearest
	require.Equal(t, int64(100), MoneyToCents(1.00))
	require.Equal(t, int64(999999), MoneyToCents(9999.99))
}

func TestCentsToMoneyStr(t *testing.T) {
	require.Equal(t, "14.50", CentsToMoneyStr(1450))
	require.Equal(t, "0.01", CentsToMoneyStr(1))
	require.Equal(t, "0.00", CentsToMoneyStr(0))
	require.Equal(t, "9999.99", CentsToMoneyStr(999999))
}

func TestAlipayAmountToCents(t *testing.T) {
	cents, err := AlipayAmountToCents("14.50")
	require.NoError(t, err)
	require.Equal(t, int64(1450), cents)

	cents, err = AlipayAmountToCents("0.01")
	require.NoError(t, err)
	require.Equal(t, int64(1), cents)

	_, err = AlipayAmountToCents("invalid")
	require.Error(t, err)

	_, err = AlipayAmountToCents("")
	require.Error(t, err)

	_, err = AlipayAmountToCents("-1.00")
	require.Error(t, err)
}
