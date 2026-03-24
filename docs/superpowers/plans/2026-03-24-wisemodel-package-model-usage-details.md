# Wisemodel Package Model Usage Details Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 `GET /api/wisemodel/user/package_usage` 响应的 `details` 字段中，按模型展示每个资源包的实际消耗量。

**Architecture:** 新增 `GetModelUsageByPackages` 批量查询函数（一次 SQL，GROUP BY package_id, model_name），扩展 `BuildPackageUsageRows` 接收 per-model 数据并填充 `details`，controller 串联新查询。

**Tech Stack:** Go, GORM, MySQL (`logs` 表已有 `wisemodel_package_id` 索引)

**Spec:** `docs/superpowers/specs/2026-03-24-wisemodel-package-model-usage-details-design.md`

---

## File Map

| 文件 | 改动类型 | 内容 |
|------|---------|------|
| `model/wisemodel_package.go` | Modify | 新增 `modelUsageRow` 结构体、`GetModelUsageByPackages` 函数、修改 `BuildPackageUsageRows` 签名及 details 填充逻辑 |
| `model/wisemodel_package_test.go` | Modify | 新增 `TestBuildPackageUsageRowsDetails` 测试 `BuildPackageUsageRows` 的 details 填充逻辑 |
| `controller/wisemodel_package.go` | Modify | `GetPackageUsage` 中调用 `GetModelUsageByPackages`，将结果传入 `BuildPackageUsageRows` |

---

## Task 1：新增 `modelUsageRow` 结构体和 `GetModelUsageByPackages` 函数

**Files:**
- Modify: `model/wisemodel_package.go`（在 `BuildPackageUsageRows` 函数之前插入）

- [ ] **Step 1: 在 `BuildPackageUsageRows` 函数之前，插入以下代码**

在 `model/wisemodel_package.go` 文件中，找到第 245 行 `func BuildPackageUsageRows(` 之前，插入：

```go
// modelUsageRow 用于 GetModelUsageByPackages 的数据库扫描结果（内部使用）
type modelUsageRow struct {
	WisemodelPackageId string `gorm:"column:wisemodel_package_id"`
	ModelName          string `gorm:"column:model_name"`
	UsedQuota          int64  `gorm:"column:used_quota"`
}

// GetModelUsageByPackages 批量查询多个资源包的 per-model quota 消费。
// 返回 map[pkgId] → 按 used_quota DESC 排序的消费明细列表。
// 只统计 wisemodel_package_id 精确匹配的日志（精确归因），FIFO 历史日志不纳入。
func GetModelUsageByPackages(pkgIds []string) (map[string][]modelUsageRow, error) {
	if len(pkgIds) == 0 {
		return map[string][]modelUsageRow{}, nil
	}
	var rows []modelUsageRow
	err := LOG_DB.Model(&Log{}).
		Where("wisemodel_package_id IN ? AND type = ?", pkgIds, LogTypeConsume).
		Group("wisemodel_package_id, model_name").
		Select("wisemodel_package_id, model_name, COALESCE(SUM(quota), 0) AS used_quota").
		Order("used_quota DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make(map[string][]modelUsageRow, len(pkgIds))
	for _, r := range rows {
		result[r.WisemodelPackageId] = append(result[r.WisemodelPackageId], r)
	}
	return result, nil
}
```

- [ ] **Step 2: 编译验证**

```bash
cd /Users/rrong/Documents/perf/projects/new-api
go build ./model/...
```

预期：无报错输出。

---

## Task 2：修改 `BuildPackageUsageRows` 填充 `details`

**Files:**
- Modify: `model/wisemodel_package.go`（`BuildPackageUsageRows` 函数）
- Modify: `model/wisemodel_package_test.go`（新增测试）

### Step 2a：先写失败测试

- [ ] **Step 1: 在 `model/wisemodel_package_test.go` 末尾追加测试**

```go
// TestBuildPackageUsageRowsDetails 验证 details 字段按模型正确填充
func TestBuildPackageUsageRowsDetails(t *testing.T) {
	expire := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)

	// Points 包
	pkgPoints := &WisemodelPackage{
		PackageId:      "PKG001_ts",
		OriginalPoints: 10000,
		QuotaGranted:   5000000, // 10000 * 0.5
		ValidUntil:     &expire,
	}
	// Tokens 包
	pkgTokens := &WisemodelPackage{
		PackageId:      "PKG002_ts",
		OriginalTokens: 500000,
		QuotaGranted:   250000, // 500000 * 0.5
		ValidUntil:     &expire,
	}

	packages := []*WisemodelPackage{pkgPoints, pkgTokens}

	// attribution: 各包消费了多少 quota
	attribution := map[string]int64{
		"PKG001_ts": 9000,   // 9000 quota = 18 points
		"PKG002_ts": 150000, // 150000 quota = 300000 tokens
	}

	// modelMap: per-model 消费明细
	modelMap := map[string][]modelUsageRow{
		"PKG001_ts": {
			{ModelName: "DeepSeek-V3", UsedQuota: 6000},  // 12 points
			{ModelName: "DeepSeek-R1", UsedQuota: 3000},  // 6 points
		},
		"PKG002_ts": {
			{ModelName: "BAAI/bge-large-zh-v1.5", UsedQuota: 150000}, // 300000 tokens
		},
	}

	rows := BuildPackageUsageRows(packages, attribution, modelMap)

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
	// 6000 quota * 1_000_000 / 500_000 = 12 points
	if d0["used_amount"] != int64(12) {
		t.Errorf("expected used_amount 12, got %v", d0["used_amount"])
	}
	// 确认 points 包不含 used_amount_tokens 字段
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
	// 150000 quota * 1_000_000 / 500_000 = 300000 tokens
	if d1["used_amount_tokens"] != int64(300000) {
		t.Errorf("expected used_amount_tokens 300000, got %v", d1["used_amount_tokens"])
	}
	// 确认 tokens 包不含 used_amount 字段
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
		QuotaGranted:   2500,
		ValidUntil:     &expire,
	}
	attribution := map[string]int64{"PKG003_ts": 0}
	modelMap := map[string][]modelUsageRow{} // 无消费明细

	rows := BuildPackageUsageRows([]*WisemodelPackage{pkg}, attribution, modelMap)

	details, ok := rows[0]["details"].([]interface{})
	if !ok {
		t.Fatalf("details is not []interface{}")
	}
	if len(details) != 0 {
		t.Errorf("expected empty details, got %d entries", len(details))
	}
}
```

- [ ] **Step 2: 运行测试，确认编译失败**

```bash
cd /Users/rrong/Documents/perf/projects/new-api
go test ./model/... -run "TestBuildPackageUsageRows" -v 2>&1 | head -30
```

预期：**编译错误**，两类报错同时出现：
- 新测试：`too many arguments in call to BuildPackageUsageRows`
- 旧测试（line 162）：`not enough arguments`（改签名后旧调用也会报错）

这两个都是预期的红灯。

### Step 2b：实现改动

- [ ] **Step 3: 修改 `BuildPackageUsageRows` 函数签名，并填充 details；同时修复旧测试调用**

**先更新现有测试 `TestBuildPackageUsageRows`（`model/wisemodel_package_test.go` 第 162-164 行）：**

```go
// 旧调用（改为传空 modelMap）
rows := BuildPackageUsageRows([]*WisemodelPackage{pkg}, map[string]int64{
    pkg.PackageId: 500000,
}, map[string][]modelUsageRow{})
```

**再替换 `BuildPackageUsageRows` 函数体（`model/wisemodel_package.go` 第 245 行）：**

将 `model/wisemodel_package.go` 中第 245 行的函数替换为：

```go
func BuildPackageUsageRows(packages []*WisemodelPackage, attribution map[string]int64, modelMap map[string][]modelUsageRow) []map[string]interface{} {
	rows := make([]map[string]interface{}, 0, len(packages))
	for _, pkg := range packages {
		consumed := attribution[pkg.PackageId]

		availableModels := []string{}
		if pkg.AvailableModels != "" {
			for _, modelName := range strings.Split(pkg.AvailableModels, ",") {
				if modelName = strings.TrimSpace(modelName); modelName != "" {
					availableModels = append(availableModels, modelName)
				}
			}
		}

		// 构建 per-model details
		details := []interface{}{}
		for _, u := range modelMap[pkg.PackageId] {
			if pkg.OriginalPoints > 0 {
				usedAmount := u.UsedQuota * WisemodelPointsPerUnit / int64(common.QuotaPerUnit)
				details = append(details, map[string]interface{}{
					"model_name":  u.ModelName,
					"used_amount": usedAmount,
				})
			} else {
				usedAmount := u.UsedQuota * WisemodelTokensPerUnit / int64(common.QuotaPerUnit)
				details = append(details, map[string]interface{}{
					"model_name":        u.ModelName,
					"used_amount_tokens": usedAmount,
				})
			}
		}

		row := map[string]interface{}{
			"package_id":        pkg.DisplayPackageId(),
			"package_record_id": pkg.PackageId,
			"available_models":  availableModels,
			"details":           details,
		}

		if pkg.OriginalPoints > 0 {
			remainPoints := (pkg.QuotaGranted - consumed) * WisemodelPointsPerUnit / int64(common.QuotaPerUnit)
			if remainPoints < 0 {
				remainPoints = 0
			}
			row["points"] = pkg.OriginalPoints
			row["remain_points"] = remainPoints
			row["amount"] = int64(pkg.OriginalPoints) - remainPoints
		} else {
			remainTokens := (pkg.QuotaGranted - consumed) * WisemodelTokensPerUnit / int64(common.QuotaPerUnit)
			if remainTokens < 0 {
				remainTokens = 0
			}
			row["tokens"] = pkg.OriginalTokens
			row["remain_tokens"] = remainTokens
			row["amount_tokens"] = int64(pkg.OriginalTokens) - remainTokens
		}

		rows = append(rows, row)
	}
	return rows
}
```

- [ ] **Step 4: 运行测试，确认通过**

```bash
cd /Users/rrong/Documents/perf/projects/new-api
go test ./model/... -run "TestBuildPackageUsageRows" -v
```

预期输出：
```
--- PASS: TestBuildPackageUsageRowsDetails (0.00s)
--- PASS: TestBuildPackageUsageRowsDetailsEmpty (0.00s)
PASS
```

- [ ] **Step 5: 运行全部 model 测试，确认无回归**

```bash
go test ./model/... -v 2>&1 | tail -20
```

预期：所有测试 PASS。

---

## Task 3：更新 Controller 串联新查询

**Files:**
- Modify: `controller/wisemodel_package.go`（`GetPackageUsage` 函数，第 192-201 行）

- [ ] **Step 1: 修改 `GetPackageUsage` 函数**

将第 192-201 行替换为：

```go
	attribution, err := model.CalculatePackageAttribution(user.Id, packages)
	if err != nil {
		c.JSON(500, gin.H{"message": "查询资源包消费失败: " + err.Error(), "success": false})
		return
	}

	// 收集所有 pkgId，批量查询 per-model 消费明细
	pkgIds := make([]string, len(packages))
	for i, p := range packages {
		pkgIds[i] = p.PackageId
	}
	modelMap, err := model.GetModelUsageByPackages(pkgIds)
	if err != nil {
		c.JSON(500, gin.H{"message": "查询模型使用明细失败: " + err.Error(), "success": false})
		return
	}

	// 构建响应
	data := model.BuildPackageUsageRows(packages, attribution, modelMap)

	c.JSON(200, gin.H{"code": 200, "data": data, "msg": "success"})
```

- [ ] **Step 2: 编译验证**

```bash
cd /Users/rrong/Documents/perf/projects/new-api
go build ./controller/... ./model/...
```

预期：无报错。

- [ ] **Step 3: 运行全部测试**

```bash
go test ./model/... -v
```

预期：全部 PASS。

- [ ] **Step 4: 提交**

```bash
git add model/wisemodel_package.go model/wisemodel_package_test.go controller/wisemodel_package.go
git commit -m "feat(wisemodel): GetPackageUsage 返回 per-model 用量明细（details 字段）

- 新增 modelUsageRow 结构体和 GetModelUsageByPackages 批量查询函数
  （一次 SQL GROUP BY wisemodel_package_id, model_name，避免 N+1）
- 修改 BuildPackageUsageRows 填充 details 字段
  points 包返回 used_amount，tokens 包返回 used_amount_tokens
- GetPackageUsage controller 串联新查询"
```

---

## Task 4：端到端验证

- [ ] **Step 1: 启动开发环境**

```bash
docker compose -f docker-compose.db-only.yml up -d
make start
```

- [ ] **Step 2: 确认 logs 表有 wisemodel_package_id 非空记录**

```bash
docker exec mysql-dev mysql -uroot -pdev123456 new_api_dev \
  -e "SELECT wisemodel_package_id, model_name, quota FROM logs WHERE wisemodel_package_id != '' AND type = 2 LIMIT 5;"
```

若无记录，调用一次 LLM 接口产生消费日志再验证。

- [ ] **Step 3: 调用接口验证 details 非空**

```bash
curl -s -X POST http://localhost:3000/api/wisemodel/user/package_usage \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer <wisemodel_auth_token>" \
  -d '{"phone":"<test_phone>"}' | python3 -m json.tool
```

检查点：
- `details` 为非空数组
- 每条有 `model_name` 字段
- points 包有 `used_amount`，tokens 包有 `used_amount_tokens`
- `used_amount` 之和 ≈ `amount`（精确归因部分，FIFO 部分不在 details 内，两者可有差异）

- [ ] **Step 4: 推送远端**

```bash
git push origin main
```
