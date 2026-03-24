# Design: Wisemodel 包使用量按模型分组明细

**日期**: 2026-03-24
**状态**: 已批准

---

## 背景

`POST /api/wisemodel/user/package_usage` 接口当前返回每个资源包的总消费量（`amount` / `amount_tokens`），但 `details` 字段始终为空数组 `[]`。

需求：在 `details` 中按模型维度展示各模型的实际消耗量。

---

## 目标响应格式

### Points 包

```json
{
  "package_id": "PKG001",
  "package_record_id": "PKG001_1742457600000000000",
  "points": 10000,
  "remain_points": 9982,
  "amount": 18,
  "available_models": ["DeepSeek-V3", "DeepSeek-R1"],
  "details": [
    {"model_name": "DeepSeek-V3", "used_amount": 12},
    {"model_name": "DeepSeek-R1", "used_amount": 6}
  ]
}
```

### Tokens 包

```json
{
  "package_id": "PKG002",
  "package_record_id": "PKG002_1742457600000001111",
  "tokens": 1000000,
  "remain_tokens": 880000,
  "amount_tokens": 120000,
  "available_models": ["BAAI/bge-large-zh-v1.5"],
  "details": [
    {"model_name": "BAAI/bge-large-zh-v1.5", "used_amount_tokens": 120000}
  ]
}
```

---

## 设计方案（方案 A：精确归因，批量查询）

### 原则

- 只查询 `wisemodel_package_id` 精确匹配的日志（走现有索引）
- 批量查询所有包（一次 SQL，而非 N 次循环）避免 N+1
- `wisemodel_package_id` 过滤值直接使用 `pkg.PackageId`（数据库原值，不假设格式）
- 历史 FIFO 归因日志（`wisemodel_package_id = ''`）不纳入 `details`，因无法精确归因到单个模型
- `details` 只含有实际消费记录的模型，零消费模型不出现

### 数据流

```
GetPackageUsage (controller)
  │
  ├─ [已有] GetWisemodelPackagesByUserId()
  ├─ [已有] CalculatePackageAttribution()    → attribution map[pkgId → totalQuota(int64)]
  ├─ [新增] GetModelUsageByPackages(pkgIds)  → modelMap map[pkgId → []modelUsageRow]
  │
  └─ BuildPackageUsageRows(packages, attribution, modelMap)  → 填充 details
```

---

## 改动文件

### `model/wisemodel_package.go`

#### 1. 新增内部结构体（仅用于数据库扫描，不对外暴露）

```go
// modelUsageRow 用于 GetModelUsageByPackages 的数据库扫描结果
type modelUsageRow struct {
    WisemodelPackageId string `gorm:"column:wisemodel_package_id"`
    ModelName          string `gorm:"column:model_name"`
    UsedQuota          int64  `gorm:"column:used_quota"`
}
```

#### 2. 新增函数 `GetModelUsageByPackages`

```go
// GetModelUsageByPackages 批量查询多个资源包的 per-model quota 消费
// 返回 map[pkgId] → 按 used_quota DESC 排序的 modelUsageRow 列表
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

#### 3. 修改 `BuildPackageUsageRows` 签名

增加第三个参数 `modelMap map[string][]modelUsageRow`，填充 `details` 字段。

单位换算（与现有 `remain_points` 计算方式完全一致）：
- Points 包：`used_amount = usedQuota * WisemodelPointsPerUnit / int64(common.QuotaPerUnit)`
- Tokens 包：`used_amount_tokens = usedQuota * WisemodelTokensPerUnit / int64(common.QuotaPerUnit)`

```go
func BuildPackageUsageRows(
    packages []*WisemodelPackage,
    attribution map[string]int64,
    modelMap map[string][]modelUsageRow,  // 新增
) []map[string]interface{} {
    rows := make([]map[string]interface{}, 0, len(packages))
    for _, pkg := range packages {
        // ... 原有字段逻辑不变 ...

        // 填充 details
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
        row["details"] = details
        rows = append(rows, row)
    }
    return rows
}
```

### `controller/wisemodel_package.go`

在 `CalculatePackageAttribution` 之后追加 per-model 查询：

```go
// 收集所有 pkgId（直接使用数据库原值，不假设格式）
pkgIds := make([]string, len(packages))
for i, p := range packages {
    pkgIds[i] = p.PackageId
}

// 查询 per-model 消费明细
modelMap, err := model.GetModelUsageByPackages(pkgIds)
if err != nil {
    c.JSON(500, gin.H{"message": "查询模型使用明细失败: " + err.Error(), "success": false})
    return
}

data := model.BuildPackageUsageRows(packages, attribution, modelMap)
```

---

## 不变的内容

- `CalculatePackageAttribution` 逻辑不改动
- logs 表结构不改动，无 schema migration
- 其他任何非 wisemodel 路径不受影响

---

## 验证方式

1. **触发写入**：确保开发环境中调用 LLM 后 logs 表有 `wisemodel_package_id` 非空的记录
   ```sql
   SELECT wisemodel_package_id, model_name, quota FROM logs
   WHERE wisemodel_package_id != '' AND type = 2 LIMIT 10;
   ```

2. **接口验证**：调用 `POST /api/wisemodel/user/package_usage`，检查：
   - `details` 为非空数组
   - `model_name` 与 logs 中一致
   - `used_amount` 等于该模型的 `SUM(quota) * 2`（因 WisemodelPointsPerUnit/QuotaPerUnit = 2）

3. **多包场景**：两个包共用一个用户，确认各包 `details` 只含属于本包的日志

4. **空消费场景**：新包未使用时，`details = []`（空数组，不报错）

5. **编译验证**：`go build ./controller/... ./model/...` 无报错
