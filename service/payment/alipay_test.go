package payment

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAlipayServiceInterface(t *testing.T) {
	var _ AlipayService = (*RealAlipayService)(nil)
	var _ AlipayService = (*MockAlipayService)(nil)
	require.NotNil(t, NewRealAlipayService)
}
