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

// TestAttributeLogsToPackages 验证 FIFO 归因算法
func TestAttributeLogsToPackages(t *testing.T) {
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	mar := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	apr := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	// 场景1：两包时间重叠，2月消费归属最早到期包（PKG-A）
	t.Run("overlap: feb consumption goes to PKG-A", func(t *testing.T) {
		pkgA := &WisemodelPackage{PackageId: "PKG-A", CreatedAt: jan, ValidUntil: &mar, QuotaGranted: 1000000}
		pkgB := &WisemodelPackage{PackageId: "PKG-B", CreatedAt: feb, ValidUntil: &apr, QuotaGranted: 2000000}
		pkgs := []*WisemodelPackage{pkgA, pkgB}

		logs := []Log{
			{CreatedAt: feb.Add(15 * 24 * time.Hour).Unix(), Quota: 500000, Type: LogTypeConsume},
		}

		attr := AttributeLogsToPackages(pkgs, logs)
		if attr["PKG-A"] != 500000 {
			t.Errorf("PKG-A want 500000, got %d", attr["PKG-A"])
		}
		if attr["PKG-B"] != 0 {
			t.Errorf("PKG-B want 0, got %d", attr["PKG-B"])
		}
	})

	// 场景2：log 在所有包有效期外，忽略
	t.Run("log outside all windows is ignored", func(t *testing.T) {
		pkgA := &WisemodelPackage{PackageId: "PKG-A", CreatedAt: jan, ValidUntil: &feb, QuotaGranted: 1000000}
		pkgs := []*WisemodelPackage{pkgA}
		logs := []Log{
			{CreatedAt: mar.Unix(), Quota: 100000, Type: LogTypeConsume},
		}
		attr := AttributeLogsToPackages(pkgs, logs)
		if attr["PKG-A"] != 0 {
			t.Errorf("PKG-A want 0, got %d", attr["PKG-A"])
		}
	})

	// 场景3：永久包（ValidUntil=nil）排在最后
	t.Run("permanent package is last resort", func(t *testing.T) {
		pkgPerm := &WisemodelPackage{PackageId: "PERM", CreatedAt: jan, ValidUntil: nil, QuotaGranted: 9999999}
		pkgA := &WisemodelPackage{PackageId: "PKG-A", CreatedAt: jan, ValidUntil: &mar, QuotaGranted: 1000000}
		pkgs := []*WisemodelPackage{pkgA, pkgPerm}
		logs := []Log{
			{CreatedAt: feb.Unix(), Quota: 100000, Type: LogTypeConsume},
		}
		attr := AttributeLogsToPackages(pkgs, logs)
		if attr["PKG-A"] != 100000 {
			t.Errorf("PKG-A want 100000, got %d", attr["PKG-A"])
		}
		if attr["PERM"] != 0 {
			t.Errorf("PERM want 0, got %d", attr["PERM"])
		}
	})

	// 场景4：单包无重叠，结果同原时间窗口
	t.Run("single package no overlap", func(t *testing.T) {
		pkgA := &WisemodelPackage{PackageId: "PKG-A", CreatedAt: jan, ValidUntil: &apr, QuotaGranted: 1000000}
		pkgs := []*WisemodelPackage{pkgA}
		logs := []Log{
			{CreatedAt: feb.Unix(), Quota: 300000, Type: LogTypeConsume},
			{CreatedAt: mar.Unix(), Quota: 200000, Type: LogTypeConsume},
		}
		attr := AttributeLogsToPackages(pkgs, logs)
		if attr["PKG-A"] != 500000 {
			t.Errorf("PKG-A want 500000, got %d", attr["PKG-A"])
		}
	})
}
