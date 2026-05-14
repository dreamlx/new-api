package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAlipayServiceInterface(t *testing.T) {
	// Compile-time check: RealAlipayService implements AlipayService
	var _ AlipayService = (*RealAlipayService)(nil)
	require.NotNil(t, NewRealAlipayService)
}

// MockAlipayService compile-time check will be added in Task 9 (mock implementation).
// Uncomment when MockAlipayService is available:
//
// func TestMockAlipayServiceImplementsInterface(t *testing.T) {
//	var _ AlipayService = (*MockAlipayService)(nil)
// }
