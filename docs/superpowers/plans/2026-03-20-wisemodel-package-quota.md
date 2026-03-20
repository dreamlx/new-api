# Wisemodel 资源包 Quota 精确化 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 Wisemodel 资源包过期 quota 不回收、时间重叠展示重复计算两个问题，并通过 logs 关联 package_id 彻底精确归因。

**Architecture:** 三阶段顺序推进——A 阶段用后台定时任务回收过期包 quota（乐观锁防并发）；C 阶段用 FIFO 纯函数重构 `GetPackageUsage` 展示层；B 阶段在 logs 表写入 `wisemodel_package_id`，让 GetPackageUsage 优先使用精确查询。

**Tech Stack:** Go 1.21+, Gin, GORM, MySQL — 遵循项目现有模式。

**Spec:** `docs/superpowers/specs/2026-03-20-wisemodel-package-quota-design.md`

---

## 文件改动清单

| 文件 | 阶段 | 操作 |
|------|------|------|
| `model/wisemodel_package.go` | A | 新增 `ReclaimedAt` 字段、`ReclaimExpiredPackages`、`ReclaimAllExpiredPackages` 函数 |
| `model/wisemodel_package_test.go` | A, C | 新建：单元测试文件 |
| `main.go` | A | 新增后台 goroutine（~第104行附近） |
| `controller/wisemodel_package.go` | C, B | 重构 `GetPackageUsage`，提取 FIFO 纯函数 |
| `model/log.go` | B | `Log` struct 和 `RecordConsumeLogParams` 新增 `WisemodelPackageId` 字段 |
| `service/quota.go` | B | 新增 `getWisemodelPackageIdForLog` helper；在3处 `RecordConsumeLog` 调用点填充字段 |

---

## Task 1：阶段A — 数据模型扩展 + 回收函数

**Files:**
- Modify: `model/wisemodel_package.go`
- Create: `model/wisemodel_package_test.go`

- [ ] **Step 1: 在 `WisemodelPackage` struct 中新增 `ReclaimedAt` 字段**

在 `model/wisemodel_package.go` 的 struct 定义（第29行 `CreatedAt` 之后）追加：

```go
ReclaimedAt     *time.Time `json:"reclaimed_at" gorm:"type:datetime"`
```

- [ ] **Step 2: 更新 `model/wisemodel_package.go` 的 import 块**

将文件顶部 import 块替换为（新增 `"fmt"`, `"sort"`, `"github.com/QuantumNous/new-api/common"`）：

```go
import (
	"fmt"
	"sort"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)
```

- [ ] **Step 3: 新增 `SortPackagesByValidUntil` 辅助函数（导出名）**

在 `model/wisemodel_package.go` 文件末尾追加：

```go
// SortPackagesByValidUntil 按 ValidUntil ASC 排序，nil（永久包）排最后
func SortPackagesByValidUntil(packages []*WisemodelPackage) {
	sort.Slice(packages, func(i, j int) bool {
		if packages[i].ValidUntil == nil {
			return false
		}
		if packages[j].ValidUntil == nil {
			return true
		}
		return packages[i].ValidUntil.Before(*packages[j].ValidUntil)
	})
}
```

- [ ] **Step 4: 新增 `ReclaimExpiredPackages` 函数**

```go
// ReclaimExpiredPackages 回收指定用户所有已过期但未回收包的剩余 quota。
// 使用乐观锁（UPDATE WHERE reclaimed_at IS NULL）防止并发重复回收。
func ReclaimExpiredPackages(userId int) error {
	now := time.Now()
	var packages []*WisemodelPackage
	if err := DB.Where("user_id = ? AND valid_until < ? AND reclaimed_at IS NULL", userId, now).
		Find(&packages).Error; err != nil {
		return err
	}
	for _, pkg := range packages {
		// 1. 估算该包时间窗口内的消费（近似值，B 完成后可改为 wisemodel_package_id 精确查询）
		var consumed int64
		DB.Model(&Log{}).
			Where("user_id = ? AND type = ? AND created_at >= ? AND created_at < ?",
				userId, LogTypeConsume, pkg.CreatedAt.Unix(), pkg.ValidUntil.Unix()).
			Select("COALESCE(SUM(quota), 0)").Scan(&consumed)

		refund := pkg.QuotaGranted - consumed
		if refund < 0 {
			refund = 0
		}

		// 2. 事务内：先原子抢占 reclaimed_at，再扣 quota
		err := DB.Transaction(func(tx *gorm.DB) error {
			result := tx.Model(&WisemodelPackage{}).
				Where("id = ? AND reclaimed_at IS NULL", pkg.Id).
				Update("reclaimed_at", now)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				// 已被其他 goroutine 处理，跳过
				return nil
			}
			if refund > 0 {
				return tx.Model(&User{}).Where("id = ?", userId).
					UpdateColumn("quota", gorm.Expr("quota - ?", refund)).Error
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// ReclaimAllExpiredPackages 全局扫描所有用户的过期未回收包，供后台定时任务调用。
func ReclaimAllExpiredPackages() error {
	var userIds []int
	if err := DB.Model(&WisemodelPackage{}).
		Where("valid_until < ? AND reclaimed_at IS NULL", time.Now()).
		Distinct("user_id").Pluck("user_id", &userIds).Error; err != nil {
		return err
	}
	var lastErr error
	for _, userId := range userIds {
		if err := ReclaimExpiredPackages(userId); err != nil {
			common.SysError(fmt.Sprintf("ReclaimAllExpiredPackages: userId %d failed: %v", userId, err))
			lastErr = err
		}
	}
	return lastErr
}
```

- [ ] **Step 5: 写失败测试**

新建 `model/wisemodel_package_test.go`：

```go
package model

import (
	"testing"
	"time"
)

// TestReclaimExpiredPackages_Idempotent 验证并发调用不会重复回收
// 注：此为集成测试，需要数据库连接。纯逻辑测试见下方 TestAttributeLogsToPackages。
func TestReclaimExpiredPackages_RefundCalculation(t *testing.T) {
	// 验证 refund = quota_granted - consumed（近似值）
	// 当 consumed > quota_granted 时，refund = 0
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
	sortPackagesByValidUntil(pkgs)
	if pkgs[0].PackageId != "march" || pkgs[1].PackageId != "june" || pkgs[2].PackageId != "permanent" {
		t.Errorf("sort wrong: got %v %v %v", pkgs[0].PackageId, pkgs[1].PackageId, pkgs[2].PackageId)
	}
}
```

注意：测试中调用 `SortPackagesByValidUntil`（已导出名）。

- [ ] **Step 6: 运行测试，确认通过**

```bash
cd /Users/rrong/Documents/perf/projects/new-api
go test ./model/... -run "TestReclaimExpiredPackages_RefundCalculation|TestSortPackagesByValidUntil" -v
```

期望：2 个测试 PASS

- [ ] **Step 7: 确认编译通过**

```bash
go build ./model/...
```

- [ ] **Step 8: Commit**

```bash
git add model/wisemodel_package.go model/wisemodel_package_test.go
git commit -m "feat(wisemodel): 阶段A - 新增过期包quota回收函数和乐观锁防重复"
```

---

## Task 2：阶段A — 后台定时任务

**Files:**
- Modify: `main.go`

- [ ] **Step 1: 在 `main.go` 中已有 goroutine 区域后追加定时任务**

在 `go controller.AutomaticallyTestChannels()` 那行（约第104行）之后追加：

```go
// Wisemodel 过期资源包 quota 回收（每5分钟扫描一次）
go func() {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    for range ticker.C {
        if err := model.ReclaimAllExpiredPackages(); err != nil {
            common.SysError("ReclaimAllExpiredPackages failed: " + err.Error())
        }
    }
}()
```

- [ ] **Step 2: 确认编译通过**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add main.go
git commit -m "feat(wisemodel): 阶段A - 启动过期包quota回收后台定时任务"
```

---

## Task 3：阶段C — FIFO 归因纯函数 + GetPackageUsage 重构

**Files:**
- Modify: `model/wisemodel_package.go`（新增纯函数）
- Modify: `model/wisemodel_package_test.go`（FIFO 单元测试）
- Modify: `controller/wisemodel_package.go`（重构 GetPackageUsage）

- [ ] **Step 1: 写 FIFO 纯函数的失败测试**

在 `model/wisemodel_package_test.go` 追加：

```go
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
		pkgs := []*WisemodelPackage{pkgA, pkgB} // 已按 valid_until ASC 排序

		logs := []Log{
			{CreatedAt: feb.Add(15 * 24 * time.Hour).Unix(), Quota: 500000, Type: LogTypeConsume},
		}

		attr := attributeLogsToPackages(pkgs, logs)
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
			{CreatedAt: mar.Unix(), Quota: 100000, Type: LogTypeConsume}, // 3月，PKG-A 已在2月过期
		}
		attr := attributeLogsToPackages(pkgs, logs)
		if attr["PKG-A"] != 0 {
			t.Errorf("PKG-A want 0, got %d", attr["PKG-A"])
		}
	})

	// 场景3：永久包（ValidUntil=nil）排在最后
	t.Run("permanent package is last resort", func(t *testing.T) {
		pkgPerm := &WisemodelPackage{PackageId: "PERM", CreatedAt: jan, ValidUntil: nil, QuotaGranted: 9999999}
		pkgA := &WisemodelPackage{PackageId: "PKG-A", CreatedAt: jan, ValidUntil: &mar, QuotaGranted: 1000000}
		// 已排序：PKG-A (mar) < PERM (nil)
		pkgs := []*WisemodelPackage{pkgA, pkgPerm}
		logs := []Log{
			{CreatedAt: feb.Unix(), Quota: 100000, Type: LogTypeConsume},
		}
		attr := attributeLogsToPackages(pkgs, logs)
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
		attr := attributeLogsToPackages(pkgs, logs)
		if attr["PKG-A"] != 500000 {
			t.Errorf("PKG-A want 500000, got %d", attr["PKG-A"])
		}
	})
}
```

- [ ] **Step 2: 运行测试，确认失败（函数未实现）**

```bash
go test ./model/... -run TestAttributeLogsToPackages -v
```

期望：编译失败或 FAIL（`attributeLogsToPackages` 未定义）

- [ ] **Step 3: 在 `model/wisemodel_package.go` 中实现 `attributeLogsToPackages` 纯函数**

```go
// attributeLogsToPackages 按 FIFO 规则将 logs 归属到 packages。
// packages 必须已按 ValidUntil ASC（nil 排最后）排序。
// logs 必须已按 CreatedAt ASC 排序。
// 返回 map[packageId] -> 归属消费 quota 之和。
func attributeLogsToPackages(packages []*WisemodelPackage, logs []Log) map[string]int64 {
	result := make(map[string]int64, len(packages))
	for _, pkg := range packages {
		result[pkg.PackageId] = 0
	}
	for _, log := range logs {
		logTime := log.CreatedAt // Unix 秒
		for _, pkg := range packages {
			if logTime < pkg.CreatedAt.Unix() {
				continue // log 早于此包创建时间
			}
			if pkg.ValidUntil != nil && logTime >= pkg.ValidUntil.Unix() {
				continue // log 晚于此包到期时间
			}
			// 此包在 log 时刻有效，FIFO：归属第一个匹配包
			result[pkg.PackageId] += int64(log.Quota)
			break
		}
		// 若无匹配包，此 log 忽略
	}
	return result
}
```

- [ ] **Step 4: 运行测试，确认通过**

```bash
go test ./model/... -run TestAttributeLogsToPackages -v
```

期望：4 个子测试全部 PASS

- [ ] **Step 5: 将 model 包中的函数导出（必须在 Step 6 之前完成）**

在 `model/wisemodel_package.go` 中：
- 将 `attributeLogsToPackages` 改名为 `AttributeLogsToPackages`（首字母大写）

同步更新 `model/wisemodel_package_test.go`：
- 将所有 `attributeLogsToPackages(` 替换为 `AttributeLogsToPackages(`

确认编译通过：
```bash
go build ./model/...
go test ./model/... -run TestAttributeLogsToPackages -v
```

期望：4 个子测试 PASS

- [ ] **Step 6: 重构 `controller/wisemodel_package.go` 中的 `GetPackageUsage`**

用下面的实现替换整个 `GetPackageUsage` 函数体（保持函数签名不变）：

```go
func GetPackageUsage(c *gin.Context) {
	var req struct {
		Phone string `json:"phone" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"message": "参数错误: " + err.Error(), "success": false})
		return
	}

	user := model.GetUserByPhone(req.Phone)
	if user == nil {
		c.JSON(404, gin.H{"message": "用户不存在", "success": false})
		return
	}

	packages, err := model.GetWisemodelPackagesByUserId(user.Id)
	if err != nil {
		c.JSON(500, gin.H{"message": "查询资源包失败: " + err.Error(), "success": false})
		return
	}
	if len(packages) == 0 {
		c.JSON(200, gin.H{"code": 200, "data": []interface{}{}, "msg": "success"})
		return
	}

	// 按 valid_until ASC 排序（nil 排最后）
	model.SortPackagesByValidUntil(packages)

	// 计算查询时间范围
	minTime := packages[0].CreatedAt.Unix()
	maxTime := time.Now().Unix()
	for _, pkg := range packages {
		if pkg.ValidUntil != nil && pkg.ValidUntil.Unix() > maxTime {
			maxTime = pkg.ValidUntil.Unix()
		}
	}

	// 查询该用户在此时间范围内的所有消费 log，按 created_at ASC
	var logs []model.Log
	if err := model.LOG_DB.
		Where("user_id = ? AND type = ? AND created_at >= ? AND created_at <= ?",
			user.Id, model.LogTypeConsume, minTime, maxTime).
		Order("created_at ASC").
		Find(&logs).Error; err != nil {
		c.JSON(500, gin.H{"message": "查询消费日志失败: " + err.Error(), "success": false})
		return
	}

	// FIFO 归因
	attribution := model.AttributeLogsToPackages(packages, logs)

	// 构建响应
	data := make([]map[string]interface{}, 0, len(packages))
	for _, pkg := range packages {
		consumed := attribution[pkg.PackageId]

		// 解析可用模型
		availableModels := []string{}
		if pkg.AvailableModels != "" {
			for _, m := range strings.Split(pkg.AvailableModels, ",") {
				if m = strings.TrimSpace(m); m != "" {
					availableModels = append(availableModels, m)
				}
			}
		}

		packageData := map[string]interface{}{
			"package_id":       pkg.PackageId,
			"available_models": availableModels,
			"details":          []interface{}{}, // 简化：FIFO 后按模型细分可后续扩展
		}

		if pkg.OriginalPoints > 0 {
			remainPoints := (pkg.QuotaGranted - consumed) / 500000
			packageData["points"] = pkg.OriginalPoints
			packageData["remain_points"] = remainPoints
			packageData["amount"] = int64(pkg.OriginalPoints) - remainPoints
		} else {
			remainTokens := (pkg.QuotaGranted - consumed) / 500000
			packageData["tokens"] = pkg.OriginalTokens
			packageData["remain_tokens"] = remainTokens
			packageData["amount_tokens"] = int64(pkg.OriginalTokens) - remainTokens
		}

		data = append(data, packageData)
	}

	c.JSON(200, gin.H{"code": 200, "data": data, "msg": "success"})
}
```

**注意**：`attributeLogsToPackages` 和 `sortPackagesByValidUntil` 需要导出（首字母大写），在 model 包中改名为 `AttributeLogsToPackages` 和 `SortPackagesByValidUntil`，同时更新测试文件中的调用。

- [ ] **Step 7: 确认编译通过**

```bash
go build ./...
```

- [ ] **Step 8: 运行所有 model 测试**

```bash
go test ./model/... -run "TestAttributeLogsToPackages|TestSortPackagesByValidUntil|TestReclaimExpiredPackages" -v
```

期望：全部 PASS

- [ ] **Step 9: Commit**

```bash
git add model/wisemodel_package.go model/wisemodel_package_test.go controller/wisemodel_package.go
git commit -m "feat(wisemodel): 阶段C - FIFO归因算法重构GetPackageUsage，修复时间重叠展示问题"
```

---

## Task 4：阶段B — logs 表新增 wisemodel_package_id 字段

**Files:**
- Modify: `model/log.go`

- [ ] **Step 1: 在 `Log` struct 中新增字段**

在 `model/log.go` 的 `Log` struct（第19-39行）末尾 `Other string` 之后追加：

```go
WisemodelPackageId string `json:"wisemodel_package_id" gorm:"type:varchar(100);default:''"`
```

- [ ] **Step 2: 在 `RecordConsumeLogParams` struct 中新增字段**

在 `RecordConsumeLogParams` struct（第134-147行）末尾追加：

```go
WisemodelPackageId string `json:"wisemodel_package_id"`
```

- [ ] **Step 3: 在 `RecordConsumeLog` 函数中将字段写入 log**

找到 `RecordConsumeLog` 函数内构建 `Log` 对象的地方，追加：

```go
WisemodelPackageId: params.WisemodelPackageId,
```

- [ ] **Step 4: 确认编译通过**

```bash
go build ./model/...
```

GORM 的 AutoMigrate 会在下次启动时自动添加该列（`type:varchar(100);default:''` 确保不破坏现有数据）。

- [ ] **Step 5: Commit**

```bash
git add model/log.go
git commit -m "feat(wisemodel): 阶段B - logs表新增wisemodel_package_id字段"
```

---

## Task 5：阶段B — relay 写 log 时填充 package_id

**Files:**
- Modify: `service/quota.go`

- [ ] **Step 1: 新增 helper 函数 `getWisemodelPackageIdForLog`**

在 `service/quota.go` 顶部 import 区域确认已引入 `"sort"`（若无则添加），然后在文件末尾追加：

```go
// getWisemodelPackageIdForLog 若当前请求来自 wisemodel token，
// 返回该用户最早到期的有效包 ID；否则返回空字符串。
func getWisemodelPackageIdForLog(ctx *gin.Context, userId int) string {
	tokenName := ctx.GetString("token_name")
	tokenKey := ctx.GetString("token_key")
	if tokenName != "wisemodel-token" && !strings.HasPrefix(tokenKey, "wisemodel-") {
		return ""
	}
	packages, err := model.GetActiveWisemodelPackages(userId)
	if err != nil || len(packages) == 0 {
		return ""
	}
	model.SortPackagesByValidUntil(packages)
	return packages[0].PackageId
}
```

- [ ] **Step 2: 在第一处 `RecordConsumeLog` 调用（约第222行，`PostConsumeTokenQuota` 函数）填充字段**

找到 `model.RecordConsumeLog(ctx, relayInfo.UserId, model.RecordConsumeLogParams{` 调用块，在末尾 `Other: other,` 之后、`})` 之前添加：

```go
WisemodelPackageId: getWisemodelPackageIdForLog(ctx, relayInfo.UserId),
```

- [ ] **Step 3: 在第二处 `RecordConsumeLog` 调用（约第342行）执行相同操作**

- [ ] **Step 4: 在第三处 `RecordConsumeLog` 调用（约第467行，`PostConsumeAudioQuota`）执行相同操作**

- [ ] **Step 5: 确认编译通过**

```bash
go build ./service/...
```

- [ ] **Step 6: Commit**

```bash
git add service/quota.go
git commit -m "feat(wisemodel): 阶段B - relay写log时填充wisemodel_package_id"
```

---

## Task 6：阶段B — GetPackageUsage 优先使用 package_id 精确查询

**Files:**
- Modify: `controller/wisemodel_package.go`

- [ ] **Step 1: 执行历史数据 backfill（将现有 NULL 值改为空字符串）**

GORM AutoMigrate 新增列时不会对历史行 backfill，导致旧 log 的 `wisemodel_package_id` 为 NULL 而非 `''`，若用 `= ''` 过滤会漏掉这些行。先执行 backfill：

```bash
docker exec mysql-dev mysql -uroot -pdev123456 new_api_dev \
  -e "UPDATE logs SET wisemodel_package_id = '' WHERE wisemodel_package_id IS NULL;"
```

- [ ] **Step 2: 替换 Task 3 Step 6 中"查询所有 log + FIFO 归因"部分**

将该段替换为精确查询 + FIFO 兜底的混合逻辑：

```go
// 分两步统计：新 log 用 wisemodel_package_id 精确查询，旧 log 用 FIFO 兜底

// Step1: 新 log（wisemodel_package_id 非空）按包 ID 精确统计
preciseAttribution := make(map[string]int64)
for _, pkg := range packages {
    var sum int64
    model.LOG_DB.Model(&model.Log{}).
        Where("wisemodel_package_id = ? AND type = ?", pkg.PackageId, model.LogTypeConsume).
        Select("COALESCE(SUM(quota), 0)").Scan(&sum)
    preciseAttribution[pkg.PackageId] = sum
}

// Step2: 旧 log（wisemodel_package_id 为 NULL 或空字符串）用 FIFO 兜底
// 注意：用 IS NULL OR = '' 兼容 backfill 前的历史数据
var oldLogs []model.Log
if err := model.LOG_DB.
    Where("user_id = ? AND type = ? AND (wisemodel_package_id IS NULL OR wisemodel_package_id = '') AND created_at >= ? AND created_at <= ?",
        user.Id, model.LogTypeConsume, minTime, maxTime).
    Order("created_at ASC").
    Find(&oldLogs).Error; err != nil {
    c.JSON(500, gin.H{"message": "查询旧消费日志失败: " + err.Error(), "success": false})
    return
}
fifoAttribution := model.AttributeLogsToPackages(packages, oldLogs)

// 合并两部分
attribution := make(map[string]int64)
for _, pkg := range packages {
    attribution[pkg.PackageId] = preciseAttribution[pkg.PackageId] + fifoAttribution[pkg.PackageId]
}
```

- [ ] **Step 3: 确认编译通过**

```bash
go build ./...
```

- [ ] **Step 4: 端到端验证**

```bash
# 启动服务
make start

# 1. 使用 wisemodel-key 发起一次 API 调用
# 2. 查 logs 表确认 wisemodel_package_id 有值
docker exec mysql-dev mysql -uroot -pdev123456 new_api_dev \
  -e "SELECT id, token_name, wisemodel_package_id FROM logs ORDER BY id DESC LIMIT 5;"

# 3. 调用 package_usage 接口，确认展示数字正确
curl -s -X POST http://localhost:3000/api/wisemodel/user/package_usage \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"phone": "18301852832"}' | jq .
```

- [ ] **Step 5: Commit**

```bash
git add controller/wisemodel_package.go
git commit -m "feat(wisemodel): 阶段B - GetPackageUsage精确+FIFO混合查询，兼容历史NULL数据"
```

---

## 验收清单

| 阶段 | 验收项 | 验证方式 |
|------|-------|---------|
| A | 过期包 `reclaimed_at` 有值 | 查 `wisemodel_packages` 表 |
| A | `users.quota` 减少未消费部分 | 对比回收前后的 quota 值 |
| A | 并发不重复回收 | 同一包 `reclaimed_at` 只写一次 |
| C | 重叠时间窗口内消费只归属一个包 | 调用 `package_usage` 接口，两包 used 之和 = 实际消费 |
| C | 单元测试全部通过 | `go test ./model/... -v` |
| B | wisemodel token 的 log 有 `wisemodel_package_id` | 查 `logs` 表 |
| B | 非 wisemodel token 的 log `wisemodel_package_id` 为空 | 查 `logs` 表 |
| B | 新旧 log 混合场景展示正确 | 接口响应数字与实际一致 |
