# Wisemodel 资源包使用与计费测试设计

**日期**: 2026-04-28  
**范围**: `model/wisemodel_package*.go`, `service/wisemodel_quota.go`, `middleware/wisemodel_package_check.go`, `scripts/test-wisemodel-api.sh`

---

## 背景

通过代码审查发现以下 7 个风险区域，其中 Risk 1 已在本次修复中解决（`eligiblePackages` 在 FIFO 排序前被过滤，导致最新的小包被优先选中，pre-consume 失败）。本测试设计覆盖所有 7 个风险，防止回归并验证修复正确性。

### 已确认的风险清单

| # | 风险描述 | 受影响文件 | 状态 |
|---|---------|-----------|------|
| 1 | FIFO 排序后 eligiblePackages 顺序错误，导致小包被优先选中 | `middleware/wisemodel_package_check.go` | 已修复 |
| 2 | 小包 QuotaGranted < estimatedQuota，pre-consume 失败且无回退到大包 | `service/wisemodel_quota.go` | 修复后缓解 |
| 3 | Redis 懒加载非原子（GET → SetNX 之间存在竞争窗口） | `service/wisemodel_quota.go` | 待测试 |
| 4 | `CreateOrder` 使用 `tx.Create` 绕过 `CreateWisemodelPackage`，Redis key 从未预置 | `controller/wisemodel_package.go` | 待测试 |
| 5 | FIFO 归因逻辑：多包场景下 quota 分配是否正确 | `model/wisemodel_package.go` | 待测试 |
| 6 | Settle delta：预扣多退、预扣少补、上游失败全退 | `service/wisemodel_quota.go` | 待测试 |
| 7 | 模型过滤：专用包不应承载其他模型的请求 | `model/wisemodel_package.go` | 待测试 |

---

## 测试策略

采用 **风险驱动（Risk-driven）** 策略：每个风险区域对应专项单元测试 + 端对端集成测试。

- **Go 单元测试**：隔离 DB/Redis，快速验证核心逻辑不变式
- **Bash 集成测试**：对接真实服务，验证完整链路行为

---

## Part 1: Go 单元测试

### 1.1 文件布局

```
model/wisemodel_package_test.go      # 已存在，扩展
service/wisemodel_quota_test.go      # 新建
middleware/wisemodel_package_check_test.go  # 新建（仅测试可抽取的纯函数）
```

### 1.2 `model/wisemodel_package_test.go` — 扩展用例

#### Risk 1: FIFO 顺序验证（回归防护）

**`TestSelectPackageAfterFIFOSort_LargeOldPackageSelected`**
- 构造：旧包（CreatedAt 早，QuotaGranted=500_000_000），新包（CreatedAt 晚，QuotaGranted=7）
- 操作：`CalculatePackageAttribution` → `FilterPackagesByModel` → `SelectPackageWithRemainingQuota`
- 断言：返回旧包，而不是新包
- 意义：直接回归刚修复的 bug

#### Risk 2: 耗尽包被跳过

**`TestSelectPackageWithRemainingQuota_SkipsExhaustedPackage`**
- 构造：包 A（QuotaGranted=100，attribution=100，已耗尽），包 B（QuotaGranted=200，attribution=0）
- 操作：`SelectPackageWithRemainingQuota([]A, B, attribution)`
- 断言：返回包 B

#### Risk 5: FIFO 归因多包场景

**`TestAttributeLogsToPackages_DistributesAcrossPackages`**
- 构造：包 A（旧，QuotaGranted=100），包 B（新，QuotaGranted=200），日志合计 quota=250
- 操作：`AttributeLogsToPackages`
- 断言：A 归因=100，B 归因=150

**`TestAttributeLogsToPackages_WithBaseline_NoDuplicateCount`**
- 构造：同上，但 baseline 中 A 已精确归因 50
- 操作：`AttributeLogsToPackagesWithBaseline`
- 断言：A 总归因=100（baseline 50 + FIFO 50），B=150

**`TestAttributeLogsToPackages_LogBeforePackageCreation_Ignored`**
- 构造：日志 CreatedAt 早于包 CreatedAt
- 断言：该日志不被归因到此包

#### Risk 7: 模型过滤

**`TestFilterPackagesByModel_UniversalPackageAlwaysEligible`**
- 构造：`AvailableModels=""`
- 断言：任意 requestedModel 均返回此包

**`TestFilterPackagesByModel_SpecificPackageExcludedForOtherModel`**
- 构造：`AvailableModels="minimax-m2"`
- 断言：`requestedModel="minimax-m2.5-highspeed"` 返回空；`requestedModel="minimax-m2"` 返回此包

**`TestFilterPackagesByModel_CaseInsensitiveMatch`**
- 构造：`AvailableModels="MiniMax-M2"`，`requestedModel="minimax-m2"`
- 断言：返回此包（大小写不敏感）

**`TestFilterPackagesByModel_EmptyRequestedModel_ReturnsAll`**
- 断言：`requestedModel=""` 时返回全部包（不过滤）

### 1.3 `service/wisemodel_quota_test.go` — 新建

使用 `github.com/alicebob/miniredis/v2` 作为内存 Redis。

#### Risk 3 & 4: Redis 懒加载

**`TestPreConsumeWisemodelPkg_KeyAbsent_LazyInit`**
- 前置：Redis 无对应 key；DB mock 返回 `QuotaGranted=500_000`
- 操作：`PreConsumeWisemodelPkg(estimatedQuota=100)`
- 断言：返回 nil；Redis key 值 = 500_000 - 100 = 499_900

**`TestPreConsumeWisemodelPkg_KeyPresent_Decremented`**
- 前置：Redis key=1000
- 操作：`PreConsumeWisemodelPkg(estimatedQuota=300)`
- 断言：返回 nil；Redis key=700；context 中 `wisemodel_pre_consumed_quota=300`

**`TestPreConsumeWisemodelPkg_ExhaustedRollback`**
- 前置：Redis key=5
- 操作：`PreConsumeWisemodelPkg(estimatedQuota=100)`
- 断言：返回 `"wisemodel package quota exhausted"`；Redis key 恢复为 5

**`TestPreConsumeWisemodelPkg_RedisDisabled_FailOpen`**
- 前置：`common.RedisEnabled=false`
- 断言：返回 nil（fail-open）

**`TestPreConsumeWisemodelPkg_EmptyPkgId_NoOp`**
- 前置：context 无 `wisemodel_package_id`
- 断言：返回 nil，Redis 无变化

#### Risk 6: Settle delta

**`TestSettleWisemodelPkg_Overage_Refunds`**
- 前置：preConsumed=100，Redis key=900（已扣过）
- 操作：`SettleWisemodelPkg(actualQuota=60)`
- 断言：Redis key=940（退还 40）

**`TestSettleWisemodelPkg_Underage_Charges`**
- 前置：preConsumed=60，Redis key=940
- 操作：`SettleWisemodelPkg(actualQuota=100)`
- 断言：Redis key=900（补扣 40）

**`TestSettleWisemodelPkg_UpstreamFailure_FullRefund`**
- 前置：preConsumed=200，Redis key=800
- 操作：`SettleWisemodelPkg(actualQuota=0)`
- 断言：Redis key=1000（全额退还）

**`TestSettleWisemodelPkg_NoPkgId_NoOp`**
- 前置：context 无 `wisemodel_package_id`
- 断言：Redis 无变化

---

## Part 2: Bash 集成测试

在 `scripts/test-wisemodel-api.sh` 末尾追加以下 5 个测试函数，并在 `main()` 中调用。

### Scenario A — 小包耗尽后大包接管（Risk 1 & 2 & 4 E2E 回归）

```
test_small_package_fallback_to_large()
```

1. 为用户创建小包：`points=10`（QuotaGranted≈5），`model_names=""`（通用）
2. 为用户创建大包：`points=1_000_000_000`，`model_names=""`（通用）
3. 发起 chat 请求（max_tokens=1024）
4. **断言**：HTTP 200，`choices` 非空
5. 查询 `package_usage`
6. **断言**：小包 `remain_points=10`（未被消费），大包 `remain_points < 1_000_000_000`

### Scenario B — 专用包与通用包的模型隔离（Risk 7 E2E）

```
test_model_specific_package_isolation()
```

1. 创建专用包：`points=1_000_000_000`，`model_names="minimax-m2"` 
2. 创建通用包：`points=1_000_000_000`，`model_names=""`
3. 用 `model=minimax-m2` 发起 chat → **断言**成功
4. 查询 `package_usage` → **断言**专用包有消费，通用包无消费（或消费极少）
5. 用非 minimax-m2 模型发起 chat → **断言**成功
6. 查询 `package_usage` → **断言**此次消费落在通用包上

### Scenario C — Redis key 缺失时懒加载正常（Risk 3 & 4 E2E）

```
test_fresh_package_first_call_succeeds()
```

1. 创建全新包（新 package_record_id，Redis 必然无此 key）
2. 立即发起 chat 请求
3. **断言**：HTTP 200，`choices` 非空（懒加载成功）
4. 查询 `package_usage` → **断言**该新包有消费记录

### Scenario D — 小配额包被连续调用耗尽（Risk 2 边界）

```
test_small_quota_package_exhaustion()
```

1. 创建小包：`points=1`（QuotaGranted≈0，仅用于验证耗尽边界）
   - 注：若 QuotaGranted=0，所有调用均应立即失败
2. 发起 chat 请求（max_tokens=1）
3. **断言**：响应为 `insufficient_quota` 错误（QuotaGranted 太小，无法通过 pre-consume）
4. 查询 `package_usage` → **断言**小包 `remain_points=1`（未被消费，因 pre-consume 失败）

### Scenario E — 多包 FIFO 消费顺序（Risk 5 E2E）

```
test_fifo_package_consumption_order()
```

1. 创建包 A（通用，`valid_until` 较早，如 2026-12-31）
2. 创建包 B（通用，`valid_until` 较晚，如 2027-12-31）
3. 发起多次 chat 请求
4. 查询 `package_usage`
5. **断言**：包 A 有消费（先到期先消费），包 B 无消费或消费 < 包 A

---

## 测试基础设施

### Go 单元测试依赖

- `github.com/alicebob/miniredis/v2` — 内存 Redis，供 `service/` 测试使用
- 现有 `model/wisemodel_refactor_test.go` 中的 DB 构造模式可复用

### Bash 集成测试依赖

- 服务需正常运行（`BASE_URL` 可达）
- `jq` 已安装
- 用户（phone=18301852832）已存在，`wisemodel_key` 有效

---

## 验收标准

| 层级 | 标准 |
|------|------|
| Go 单元测试 | `go test ./model/... ./service/...` 全部通过，无 race（`-race` flag） |
| Bash 集成测试 | Scenario A~E 全部 `[PASS]` |
| 回归 | 已修复的 Risk 1 对应的 `TestSelectPackageAfterFIFOSort_LargeOldPackageSelected` 必须通过 |
