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
	// QuotaPerUnit=500_000, WisemodelPointsPerUnit=500_000
	// 1 point = 1 quota.
	// 3_000_000 points → QuotaGranted = 3_000_000
	// consumed=500_000 → remain_quota=2_500_000 → remain_points=2_500_000
	// amount = 3_000_000 - 2_500_000 = 500_000
	pkg := &WisemodelPackage{
		PackageId:         "PKG001_1742457600000000000",
		OriginalPackageId: "PKG001",
		OriginalPoints:    3_000_000,
		QuotaGranted:      3_000_000,
		RemainQuota:       2_500_000, // consumed 500_000
		AvailableModels:   "DeepSeek-V3,DeepSeek-R1",
		CreatedAt:         time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
		ValidUntil:        &expire,
	}

	rows := BuildPackageUsageRows([]*WisemodelPackage{pkg}, map[string][]ModelUsageRow{})
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
	if row["remain_points"] != int64(2_500_000) {
		t.Fatalf("remain_points want 2_500_000, got %#v", row["remain_points"])
	}
	if row["amount"] != int64(500_000) {
		t.Fatalf("amount want 500_000, got %#v", row["amount"])
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

// TestBuildPackageUsageRowsDetails 验证 details 字段按模型正确填充
func TestBuildPackageUsageRowsDetails(t *testing.T) {
	expire := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	// Points 包
	pkgPoints := &WisemodelPackage{
		PackageId:      "PKG001_ts",
		OriginalPoints: 10000,
		QuotaGranted:   10000,
		RemainQuota:    1000, // consumed 9000
		ValidUntil:     &expire,
	}
	// Tokens 包
	pkgTokens := &WisemodelPackage{
		PackageId:      "PKG002_ts",
		OriginalTokens: 500000,
		QuotaGranted:   500000,
		RemainQuota:    350000, // consumed 150000
		ValidUntil:     &expire,
	}

	packages := []*WisemodelPackage{pkgPoints, pkgTokens}

	modelMap := map[string][]ModelUsageRow{
		"PKG001_ts": {
			{ModelName: "DeepSeek-V3", UsedQuota: 6000},
			{ModelName: "DeepSeek-R1", UsedQuota: 3000},
		},
		"PKG002_ts": {
			{ModelName: "BAAI/bge-large-zh-v1.5", UsedQuota: 150000},
		},
	}

	rows := BuildPackageUsageRows(packages, modelMap)

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}

	// 验证 points 包的 details
	row0 := rows[0]
	details0, ok := row0["details"].([]interface{})
	if !ok {
		t.Fatalf("details is not []interface{}: %T", row0["details"])
	}
	if len(details0) != 2 {
		t.Fatalf("expected 2 detail entries for points pkg, got %d", len(details0))
	}
	d0 := details0[0].(map[string]interface{})
	if d0["model_name"] != "DeepSeek-V3" {
		t.Errorf("expected model_name DeepSeek-V3, got %v", d0["model_name"])
	}
	// 6000 quota * 500_000 / 500_000 = 6000 points
	if d0["used_amount"] != int64(6000) {
		t.Errorf("expected used_amount 6000, got %v", d0["used_amount"])
	}
	if _, exists := d0["used_amount_tokens"]; exists {
		t.Error("points package detail should not have used_amount_tokens field")
	}

	// 验证 tokens 包的 details
	row1 := rows[1]
	details1, ok := row1["details"].([]interface{})
	if !ok {
		t.Fatalf("details is not []interface{}: %T", row1["details"])
	}
	if len(details1) != 1 {
		t.Fatalf("expected 1 detail entry for tokens pkg, got %d", len(details1))
	}
	d1 := details1[0].(map[string]interface{})
	if d1["model_name"] != "BAAI/bge-large-zh-v1.5" {
		t.Errorf("expected model_name BAAI/bge-large-zh-v1.5, got %v", d1["model_name"])
	}
	// 150000 quota * 500_000 / 500_000 = 150000 tokens
	if d1["used_amount_tokens"] != int64(150000) {
		t.Errorf("expected used_amount_tokens 150000, got %v", d1["used_amount_tokens"])
	}
	if _, exists := d1["used_amount"]; exists {
		t.Error("tokens package detail should not have used_amount field")
	}
}

// TestBuildPackageUsageRowsDetailsEmpty 验证无消费时 details 为空数组
func TestBuildPackageUsageRowsDetailsEmpty(t *testing.T) {
	expire := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	pkg := &WisemodelPackage{
		PackageId:      "PKG003_ts",
		OriginalPoints: 5000,
		QuotaGranted:   5000,
		RemainQuota:    5000, // no consumption
		ValidUntil:     &expire,
	}
	modelMap := map[string][]ModelUsageRow{}

	rows := BuildPackageUsageRows([]*WisemodelPackage{pkg}, modelMap)

	details, ok := rows[0]["details"].([]interface{})
	if !ok {
		t.Fatalf("details is not []interface{}")
	}
	if len(details) != 0 {
		t.Errorf("expected empty details, got %d entries", len(details))
	}
}

func TestFilterPackagesByModel(t *testing.T) {
	cases := []struct {
		name         string
		packages     []*WisemodelPackage
		model        string
		wantEligible int // 期望返回的包数量
	}{
		{
			name:         "empty package list → no eligible packages",
			packages:     []*WisemodelPackage{},
			model:        "deepseek-chat",
			wantEligible: 0,
		},
		{
			name:         "all universal packages → all eligible",
			packages:     []*WisemodelPackage{{AvailableModels: ""}, {AvailableModels: ""}},
			model:        "deepseek-chat",
			wantEligible: 2,
		},
		{
			name:         "exact match",
			packages:     []*WisemodelPackage{{AvailableModels: "minimax-m2"}},
			model:        "minimax-m2",
			wantEligible: 1,
		},
		{
			name:         "case-insensitive match",
			packages:     []*WisemodelPackage{{AvailableModels: "MiniMax-M2"}},
			model:        "minimax-m2",
			wantEligible: 1,
		},
		{
			name:         "stored value with surrounding spaces",
			packages:     []*WisemodelPackage{{AvailableModels: " minimax-m2 "}},
			model:        "minimax-m2",
			wantEligible: 1,
		},
		{
			name:         "model in multi-model list",
			packages:     []*WisemodelPackage{{AvailableModels: "deepseek-v3,minimax-m2"}},
			model:        "minimax-m2",
			wantEligible: 1,
		},
		{
			name:         "model not in any package",
			packages:     []*WisemodelPackage{{AvailableModels: "deepseek-v3"}},
			model:        "minimax-m2",
			wantEligible: 0,
		},
		{
			name: "multiple packages, model in one of them",
			packages: []*WisemodelPackage{
				{AvailableModels: "deepseek-v3"},
				{AvailableModels: "minimax-m2"},
			},
			model:        "minimax-m2",
			wantEligible: 1,
		},
		{
			name: "mixed: one restricted, one universal → universal covers any model",
			packages: []*WisemodelPackage{
				{AvailableModels: "deepseek-v3"},
				{AvailableModels: ""},
			},
			model:        "minimax-m2",
			wantEligible: 1, // 空包（通用包）覆盖 minimax-m2
		},
		{
			name:         "empty model string → all packages returned",
			packages:     []*WisemodelPackage{{AvailableModels: "deepseek-v3"}, {AvailableModels: ""}},
			model:        "",
			wantEligible: 2,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := FilterPackagesByModel(tc.packages, tc.model)
			if len(got) != tc.wantEligible {
				t.Errorf("FilterPackagesByModel(%v, %q) returned %d packages, want %d",
					tc.packages, tc.model, len(got), tc.wantEligible)
			}
		})
	}
}
