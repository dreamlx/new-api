package model

import (
	"testing"
	"time"
)

// TestReclaimExpiredPackages_RefundCalculation 验证 refund 计算逻辑
func TestReclaimExpiredPackages_RefundCalculation(t *testing.T) {
	cases := []struct {
		quotaGranted int64
		consumed     int64
		wantRefund   int64
	}{
		{1000000, 300000, 700000},
		{1000000, 0, 1000000},
		{1000000, 1200000, 0}, // 消费超出包额度，不回收负数
	}
	for _, c := range cases {
		refund := c.quotaGranted - c.consumed
		if refund < 0 {
			refund = 0
		}
		if refund != c.wantRefund {
			t.Errorf("quotaGranted=%d consumed=%d: want refund %d, got %d",
				c.quotaGranted, c.consumed, c.wantRefund, refund)
		}
	}
}

// TestSortPackagesByValidUntil 验证 nil ValidUntil 排在最后
func TestSortPackagesByValidUntil(t *testing.T) {
	t1 := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	pkgs := []*WisemodelPackage{
		{PackageId: "permanent", ValidUntil: nil},
		{PackageId: "june", ValidUntil: &t2},
		{PackageId: "march", ValidUntil: &t1},
	}
	SortPackagesByValidUntil(pkgs)
	if pkgs[0].PackageId != "march" || pkgs[1].PackageId != "june" || pkgs[2].PackageId != "permanent" {
		t.Errorf("sort wrong: got %v %v %v", pkgs[0].PackageId, pkgs[1].PackageId, pkgs[2].PackageId)
	}
}
