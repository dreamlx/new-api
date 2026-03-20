# Wisemodel 资源包 Quota 精确化设计

**日期**: 2026-03-20
**状态**: 待实现
**优先级**: A → C → B（顺序执行）

---

## 背景与问题

当前 Wisemodel 资源包系统存在两个缺陷：

1. **过期包 quota 未回收**：绑定包时将 `quota_granted` 累加到 `users.quota`，但包到期后未消费的 quota 不会回收，用户可继续使用已失效包的额度。

2. **时间重叠导致展示重复计算**：`GetPackageUsage` 通过时间窗口从 `logs` 表反推消费，当两个包时间重叠时，同一笔消费会被计入多个包，展示数字不准确。

根本原因：`logs` 表无 `package_id` 字段，系统无法知道某次 API 调用消耗的是哪个包的额度。

---

## 约束条件

- 所有改动**不影响非 wisemodel 用户**的任何接口和计费逻辑
- DB 新增字段必须可空或有默认值，不需要停机 migration
- relay 核心链路改动需加 wisemodel token 条件判断，非 wisemodel 请求走原逻辑

---

## 各阶段数据准确性等级

| 阶段组合 | 展示准确性 | 回收准确性 |
|---------|-----------|-----------|
| 仅 A | 不变（近似） | 近似（时间窗口重叠误差） |
| A + C | 精确 | 近似 |
| A + C + B | 精确 | 精确（B 完成后 A 的回收也可升级为精确查询） |

**阶段 A 的回收量计算与展示层存在同样的时间窗口重叠误差，属于有意识的近似值，B 完成后可升级为精确计算。**

---

## 阶段 A：过期包 quota 回收

### 目标
包到期时，未消费的 quota 从 `users.quota` 中扣回，防止用户用过期额度继续消费。

### 数据库变更
```sql
ALTER TABLE wisemodel_packages
  ADD COLUMN reclaimed_at DATETIME NULL DEFAULT NULL;
```

### 新增函数：`ReclaimExpiredPackages(userId int)`
位置：`model/wisemodel_package.go`

逻辑（**原子性幂等写入，防并发重复回收**）：
```
1. 查询 WHERE user_id=? AND valid_until < NOW() AND reclaimed_at IS NULL
2. 对每个过期包：
   a. 用时间窗口估算消费（近似值，B 完成后可替换为精确查询）：
      SELECT SUM(quota) FROM logs
      WHERE user_id=? AND created_at >= pkg.CreatedAt AND created_at < pkg.ValidUntil
   b. refund = max(0, quota_granted - min(consumed, quota_granted))
   c. 开启事务：
      ① 原子标记（乐观锁）：
         UPDATE wisemodel_packages SET reclaimed_at=NOW()
         WHERE id=? AND reclaimed_at IS NULL
         → RowsAffected == 0 说明已被其他请求处理，跳过，提交事务
      ② RowsAffected == 1：执行 users.quota -= refund
      ③ 提交事务
```

**并发安全说明**：步骤②先原子抢占 `reclaimed_at`，再扣 quota。即使两个并发请求同时到达，也只有一个能成功执行 UPDATE（RowsAffected=1），另一个直接跳过，不会重复扣减。

### 触发点：独立后台定时任务

**不放在中间件请求路径上**，理由：
- 中间件职责是"是否放行"，不应修改账本状态
- 避免每次请求增加额外 DB 开销

改为独立 goroutine 定时扫描（启动时注册）：
```go
// main.go 或 router 初始化时启动
go func() {
    ticker := time.NewTicker(5 * time.Minute)
    for range ticker.C {
        model.ReclaimAllExpiredPackages() // 扫描全局过期未回收的包
    }
}()
```

新增 `ReclaimAllExpiredPackages()`：全局扫描所有用户的过期未回收包，逻辑同上，不限 userId。

### 验收标准
- 绑定一个 `valid_until` 为过去时间的包
- 等待定时任务触发（或手动触发测试函数）
- `wisemodel_packages.reclaimed_at` 有值，`users.quota` 减少了未消费部分
- 并发调用不会重复扣减（`reclaimed_at` 只写一次）
- 正常用户（非 wisemodel）的 quota 不受影响

---

## 阶段 C：FIFO 展示归因

### 目标
`GetPackageUsage` 展示准确，时间重叠的包不重复计算同一笔消费。

### 算法：FIFO 归因
位置：`controller/wisemodel_package.go` → `GetPackageUsage`

替换原有"每包独立按时间窗口查 logs"逻辑：

```
前置：将所有包中 valid_until IS NULL 的视为"永久有效"，排序时置于最后（NULLS LAST）

1. 获取用户所有包，按 valid_until ASC NULLS LAST 排序
2. 查询用户所有消费 log（type=LogTypeConsume），按 created_at ASC
   时间范围：[所有包中最早 created_at, 所有包中最晚 valid_until]
3. 对每条 log entry：
   a. 找出该 log 时刻同时有效的包：
      pkg.CreatedAt <= log.CreatedAt AND (pkg.ValidUntil IS NULL OR pkg.ValidUntil > log.CreatedAt)
   b. 若没有匹配包（log 在所有包有效期外）→ 该 log 忽略，不计入任何包
   c. 若有匹配包 → 选 valid_until 最早的（NULLS LAST），将 log.quota 归属到该包
4. 每个包的展示数据：
   used   = 该包归属消费之和
   remain = max(0, quota_granted - used)
```

**边界条件处理**：
- **单包无重叠**：算法退化为原时间窗口，结果一致
- **log 时刻无有效包**：直接跳过（可能是包过期后 relay 延迟到达的消费日志）
- **valid_until IS NULL 的包**：永久有效，排序置于最后，确保优先消耗有截止日期的包

### 不变部分
- DB 结构不变，relay 链路不变，其他接口不变

### 验收标准
- 创建两个时间重叠的包（PKG-A: 1-3月，PKG-B: 2-4月）
- 在 2月 消费一笔
- 调用 `POST /api/wisemodel/user/package_usage`
- 该笔消费只出现在 PKG-A（valid_until 更早），PKG-B 不计入
- 单包场景、无消费场景、永久包场景结果正常

---

## 阶段 B：logs 记录 package_id

### 目标
每次 API 调用直接在 log 中记录消耗的是哪个包，彻底消除时间窗口近似误差。

### 数据库变更
```sql
ALTER TABLE logs
  ADD COLUMN wisemodel_package_id VARCHAR(100) NOT NULL DEFAULT '';
```

### 改动一：扩展 `RecordConsumeLogParams` struct
位置：`model/log.go`

```go
// 新增字段，零值为空字符串，其他所有调用点无需修改
WisemodelPackageId string `json:"wisemodel_package_id" gorm:"type:varchar(100);default:''"`
```

**选择 struct 扩展而非 context 传递**：12 个现有调用点不需要修改（空字符串即默认值），向后兼容性最好。

### 改动二：relay 写 log 时记录 package_id
位置：`service/quota.go` 中 `PostConsumeQuota` 或 `RecordConsumeLog` 调用处

```go
// 仅对 wisemodel token 执行，复用中间件相同的识别逻辑
if relayInfo.TokenName == "wisemodel-token" || strings.HasPrefix(relayInfo.TokenKey, "wisemodel-") {
    packages, _ := model.GetActiveWisemodelPackages(relayInfo.UserId)
    // 排序同 FIFO：valid_until ASC NULLS LAST
    if len(packages) > 0 {
        params.WisemodelPackageId = packages[0].PackageId
    }
}
```

**关于重复 DB 查询**：`GetActiveWisemodelPackages` 在中间件已查过一次，此处再查一次。考虑到写 log 是异步路径，可接受额外一次查询，后续如需优化可通过 request context 传递中间件的查询结果。

### 改动三：`GetPackageUsage` 优先使用 package_id 查询
位置：`controller/wisemodel_package.go`

```
对每个包：
  新 log（wisemodel_package_id != ''）：
    WHERE wisemodel_package_id = pkg.PackageId
  旧 log（wisemodel_package_id == ''）：
    使用阶段 C 的 FIFO 归因算法（兜底）
合并两部分消费，计算 remain
```

### 隔离保障
- `wisemodel_package_id` 默认空字符串，非 wisemodel 请求不写该字段
- 条件判断包裹在 `if isWisemodelToken` 内，其他 token 走原有逻辑
- 旧 log 数据无该字段，由阶段 C 的 FIFO 兜底，向后兼容

### 阶段 B 完成后升级阶段 A 的回收精度
`ReclaimExpiredPackages` 的消费量计算可从时间窗口查询改为：
```sql
SELECT SUM(quota) FROM logs WHERE wisemodel_package_id = ?
```
精确回收，无重叠误差。

### 验收标准
- 使用 wisemodel-key 发起 API 调用
- 查 `logs` 表，`wisemodel_package_id` 有对应包 ID
- 非 wisemodel 用户的 log，`wisemodel_package_id` 为空字符串
- `GetPackageUsage` 对新 log 用精确查询，对旧 log 用 FIFO 兜底
- 两包时间重叠场景，展示与实际完全一致

---

## 文件改动汇总

| 文件 | 阶段 | 改动说明 |
|------|------|---------|
| `model/wisemodel_package.go` | A | 新增 `ReclaimedAt` 字段；新增 `ReclaimExpiredPackages`、`ReclaimAllExpiredPackages` 函数 |
| `main.go` 或初始化文件 | A | 启动后台定时任务（每 5 分钟扫描） |
| `controller/wisemodel_package.go` | C, B | 重构 `GetPackageUsage`：FIFO 算法 + package_id 精确查询 |
| `model/log.go` | B | `Log` struct 及 `RecordConsumeLogParams` 新增 `WisemodelPackageId` 字段 |
| `service/quota.go` | B | `PostConsumeQuota` 写 log 时判断 wisemodel token 并填充 `WisemodelPackageId` |

---

## 依赖关系

```
A（近似回收）─── 独立 ───▶ 先做，修复实际资金漏洞（近似精度）
C（FIFO 展示）── 独立 ───▶ 修复展示层，与 A 无依赖
B（logs 归因）── 依赖 C ──▶ 旧 log 兜底复用 C 的 FIFO；完成后可升级 A 的回收为精确
```
