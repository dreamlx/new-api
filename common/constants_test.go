package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewPaymentConstants(t *testing.T) {
	require.Equal(t, "anomaly", TopUpStatusAnomaly)
}
