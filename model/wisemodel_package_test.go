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

func TestSortPackagesByValidUntilStableTieBreak(t *testing.T) {
	expire := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	pkgs := []*WisemodelPackage{
		{PackageId: "pkg-b", CreatedAt: newer, ValidUntil: &expire},
		{PackageId: "pkg-a", CreatedAt: older, ValidUntil: &expire},
	}

	SortPackagesByValidUntil(pkgs)
	if pkgs[0].PackageId != "pkg-a" || pkgs[1].PackageId != "pkg-b" {
		t.Fatalf("stable tie break failed, got %s then %s", pkgs[0].PackageId, pkgs[1].PackageId)
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

	t.Run("old log spills into next valid package when first package is exhausted", func(t *testing.T) {
		pkgA := &WisemodelPackage{PackageId: "PKG-A", CreatedAt: jan, ValidUntil: &mar, QuotaGranted: 500000}
		pkgB := &WisemodelPackage{PackageId: "PKG-B", CreatedAt: feb, ValidUntil: &apr, QuotaGranted: 1000000}
		pkgs := []*WisemodelPackage{pkgA, pkgB}

		logs := []Log{
			{CreatedAt: feb.Add(15 * 24 * time.Hour).Unix(), Quota: 800000, Type: LogTypeConsume},
		}

		attr := AttributeLogsToPackages(pkgs, logs)
		if attr["PKG-A"] != 500000 {
			t.Errorf("PKG-A want 500000, got %d", attr["PKG-A"])
		}
		if attr["PKG-B"] != 300000 {
			t.Errorf("PKG-B want 300000, got %d", attr["PKG-B"])
		}
	})
}

func TestBuildPackageUsageRows(t *testing.T) {
	expire := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	pkg := &WisemodelPackage{
		PackageId:         "PKG001_1742457600000000000",
		OriginalPackageId: "PKG001",
		OriginalPoints:    3,
		QuotaGranted:      1500000,
		AvailableModels:   "DeepSeek-V3,DeepSeek-R1",
		CreatedAt:         time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		ValidUntil:        &expire,
	}

	rows := BuildPackageUsageRows([]*WisemodelPackage{pkg}, map[string]int64{
		pkg.PackageId: 500000,
	})
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}

	row := rows[0]
	if row["package_id"] != "PKG001" {
		t.Fatalf("package_id want original id PKG001, got %#v", row["package_id"])
	}
	if row["package_record_id"] != pkg.PackageId {
		t.Fatalf("package_record_id want internal id %s, got %#v", pkg.PackageId, row["package_record_id"])
	}
	if row["remain_points"] != int64(2) {
		t.Fatalf("remain_points want 2, got %#v", row["remain_points"])
	}
	if row["amount"] != int64(1) {
		t.Fatalf("amount want 1, got %#v", row["amount"])
	}
}

func TestAttributeLogsToPackagesWithBaseline(t *testing.T) {
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	mar := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	apr := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	pkgA := &WisemodelPackage{PackageId: "PKG-A", CreatedAt: jan, ValidUntil: &mar, QuotaGranted: 500000}
	pkgB := &WisemodelPackage{PackageId: "PKG-B", CreatedAt: feb, ValidUntil: &apr, QuotaGranted: 1000000}
	pkgs := []*WisemodelPackage{pkgA, pkgB}

	attr := AttributeLogsToPackagesWithBaseline(pkgs, []Log{
		{CreatedAt: feb.Add(15 * 24 * time.Hour).Unix(), Quota: 500000, Type: LogTypeConsume},
	}, map[string]int64{
		"PKG-A": 400000,
	})

	if attr["PKG-A"] != 100000 {
		t.Fatalf("PKG-A want 100000 after baseline, got %d", attr["PKG-A"])
	}
	if attr["PKG-B"] != 400000 {
		t.Fatalf("PKG-B want 400000 after spillover, got %d", attr["PKG-B"])
	}
}

func TestDisplayPackageIdFallsBackToLegacyInternalId(t *testing.T) {
	pkg := &WisemodelPackage{PackageId: "PKG_LEGACY_1742457600000000000"}
	if got := pkg.DisplayPackageId(); got != "PKG_LEGACY" {
		t.Fatalf("want legacy display id PKG_LEGACY, got %s", got)
	}
}

func TestSelectPackageWithRemainingQuota(t *testing.T) {
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	mar := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	apr := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)

	pkgA := &WisemodelPackage{PackageId: "PKG-A", CreatedAt: jan, ValidUntil: &mar, QuotaGranted: 500000}
	pkgB := &WisemodelPackage{PackageId: "PKG-B", CreatedAt: feb, ValidUntil: &apr, QuotaGranted: 1000000}
	pkgs := []*WisemodelPackage{pkgA, pkgB}

	selected := SelectPackageWithRemainingQuota(pkgs, map[string]int64{
		"PKG-A": 500000,
		"PKG-B": 200000,
	})
	if selected == nil || selected.PackageId != "PKG-B" {
		t.Fatalf("want PKG-B, got %#v", selected)
	}
}
