package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewPaymentConstants(t *testing.T) {
	require.Equal(t, "anomaly", TopUpStatusAnomaly)
	require.Equal(t, "", RefundStatusNone)
	require.Equal(t, "refund_pending", RefundStatusPending)
	require.Equal(t, "refund_success", RefundStatusSuccess)
	require.Equal(t, "refund_failed", RefundStatusFailed)
	require.Equal(t, "refund_anomaly", RefundStatusAnomaly)
}
