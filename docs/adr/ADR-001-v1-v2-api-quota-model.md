# ADR-001: V1 vs V2 API 额度模型设计

**状态**: 草案
**日期**: 2026-01-22
**决策者**: 待定

---

## 背景

New API 提供了两套外部用户API：
- **V1 API**: 面向个人用户的API（用户同步 + Token管理）
- **V2 API**: 面向平台集成的API（平台授权 + 消费流水）

当前CEC平台在使用V1 API创建带独立额度的Token时遇到问题：
- **错误信息**: "用户余额不足，当前余额: 1241379，请求分配: 6896552"
- **场景**: CEC自己有充足余额，只想给Token设置消费上限
- **问题本质**: V1独立额度模式要求预锁定用户余额

---

## 当前实现分析

### Token模型关键字段

```go
type Token struct {
    RemainQuota     int  `json:"remain_quota"`      // Token剩余额度
    UnlimitedQuota  bool `json:"unlimited_quota"`   // 是否无限额度
    DeductUserQuota bool `json:"deduct_user_quota"` // 消费时是否扣用户余额
}
```

### 三种Token模式对比

| 维度 | V1 独立额度 | V1 无限额度 | V2 无限额度 |
|------|------------|------------|------------|
| **UnlimitedQuota** | `false` | `true` | `true` |
| **DeductUserQuota** | N/A | `true` | `false` |
| **RemainQuota** | 分配的额度 | 0 | 0 |
| **创建时验证用户余额** | ✅ 必须足够 | ❌ | ❌ |
| **创建时扣除用户余额** | ✅ 预锁定 | ❌ | ❌ |
| **消费时扣费来源** | Token额度 | 用户余额 | **不扣任何余额** |
| **Token耗尽后** | 不可用 | 继续用(扣用户) | 继续用(不扣) |
| **平台计费方式** | 预付费 | 后付费 | 自己统计 |

### 代码逻辑追踪

**V1独立额度Token创建** (`controller/external_user.go:457-491`):
```go
// 验证用户余额是否足够分配
if !req.UnlimitedQuota && user.Quota < req.AllocatedQuota {
    return "用户余额不足"  // ← 当前问题点
}

// 从User扣除额度（预锁定）
if !req.UnlimitedQuota {
    tx.Model(user).Update("quota", user.Quota-req.AllocatedQuota)
}

// Token设置
token := &model.Token{
    RemainQuota:     req.AllocatedQuota,
    UnlimitedQuota:  false,
    DeductUserQuota: false,  // 不扣用户余额，扣Token余额
}
```

**V2 Token授权** (`controller/v2_external_user.go:192-201`):
```go
token := &model.Token{
    UnlimitedQuota: true,   // 无限额度
    RemainQuota:    0,
    // DeductUserQuota: false (默认值)
}
// 不验证用户余额，不扣除用户余额
```

**消费时扣费逻辑** (`service/quota.go:504-527`):
```go
if !relayInfo.TokenUnlimited {
    // V1独立额度：扣Token余额
    model.DecreaseTokenQuota(...)
} else if relayInfo.DeductUserQuota {
    // V1无限额度：扣用户余额
    model.DecreaseUserQuota(...)
}
// else: V2无限额度 → 不扣任何余额！
```

**预扣费检查** (`service/pre_consume_quota.go:39-43`):
```go
if userQuota <= 0 {
    return "用户额度不足"  // ← 所有模式都需要用户余额>0
}
```

---

## 问题分析

### CEC的使用场景

1. CEC是一个下游平台，自己有计费系统
2. CEC给终端用户创建Token时，想设置消费上限（风控）
3. CEC不想预锁定NewAPI用户余额（因为CEC自己管理余额）

### 当前的限制

| 需求 | V1独立额度 | V1无限额度 | V2无限额度 |
|------|-----------|-----------|-----------|
| Token有消费上限 | ✅ | ❌ | ❌ |
| 创建时不需预锁用户余额 | ❌ | ✅ | ✅ |
| **两个需求同时满足** | ❌ | ❌ | ❌ |

**核心矛盾**: 没有一种模式能同时满足"Token有消费上限"和"创建时不预锁用户余额"。

---

## 解决方案选项

### 方案A: 设置超大用户余额（推荐 - 最简单）

**思路**: 给CEC平台用户设置一个超大的初始余额，使得余额检查永远通过。

**实现方式**:

1. **数据库直接设置**:
```sql
UPDATE users SET quota = 499999500000 WHERE username = 'platform_cec';
-- 约100万美元的quota
```

2. **通过充值接口**:
```bash
curl -X POST /api/user/external/topup \
  -d '{"external_user_id": "cec", "amount_usd": 999999}'
```

3. **修改V2平台用户默认余额** (改1行代码):
```go
// model/user.go:981
Quota: 499999500000,  // 约100万美元
```

| 优点 | 缺点 |
|------|------|
| 不改业务逻辑 | 用户余额数字不真实 |
| 实现简单 | 需要手动设置 |
| 立即可用 | 对所有V2用户生效(方式3) |

**置信度**: 5/5 - 能解决当前问题

---

### 方案B: 新增"预算模式"字段

**思路**: 添加一个新的Token配置字段 `BudgetOnly`，表示"只作为消费上限，不预锁余额"。

**数据模型变更**:
```go
type Token struct {
    BudgetOnly bool `json:"budget_only"` // 预算模式：不预锁用户余额
}
```

**创建逻辑变更**:
```go
// 独立额度模式：验证用户余额是否足够分配
if !req.UnlimitedQuota && !req.BudgetOnly && user.Quota < req.AllocatedQuota {
    return "用户余额不足"
}

// 从User扣除额度（预锁定）- 预算模式跳过
if !req.UnlimitedQuota && !req.BudgetOnly {
    tx.Model(user).Update("quota", user.Quota-req.AllocatedQuota)
}
```

**消费逻辑无需变更** - Token的RemainQuota仍然正常工作。

| 优点 | 缺点 |
|------|------|
| 语义清晰 | 需要改代码 |
| 灵活控制 | 需要数据库迁移 |
| 向后兼容 | 增加复杂度 |

**置信度**: 4/5 - 更优雅但需要开发

---

### 方案C: V2支持Token额度限制

**思路**: 扩展V2 API，让平台可以给Token设置消费上限。

**V2授权请求扩展**:
```go
type V2TokenAuthorizeRequest struct {
    PlatformId   string `json:"platform_id"`
    TokenKey     string `json:"token_key"`
    MaxQuota     int    `json:"max_quota"`  // 新增：最大消费额度（0=无限）
}
```

**Token创建变更**:
```go
token := &model.Token{
    UnlimitedQuota: req.MaxQuota == 0,
    RemainQuota:    req.MaxQuota,
    DeductUserQuota: false,
}
```

| 优点 | 缺点 |
|------|------|
| V2原生支持 | 改动较大 |
| 统一平台体验 | 需要更新文档 |

**置信度**: 4/5 - 适合长期规划

---

## 决策矩阵

| 方案 | 实现复杂度 | 改动范围 | 立即可用 | 长期可维护 | 推荐度 |
|------|-----------|---------|---------|-----------|-------|
| A: 超大余额 | ⭐ | 无代码改动 | ✅ | ⭐⭐ | **优先** |
| B: 预算模式 | ⭐⭐⭐ | V1 API | ❌ | ⭐⭐⭐⭐ | 中期 |
| C: V2扩展 | ⭐⭐⭐⭐ | V2 API | ❌ | ⭐⭐⭐⭐⭐ | 长期 |

---

## 建议决策

### 短期（立即）: 采用方案A

1. 给CEC平台用户设置超大余额（数据库操作）
2. 继续使用V1独立额度模式创建Token
3. Token消费上限功能正常工作

### 中长期（可选）: 评估方案B或C

根据实际使用情况决定是否需要更优雅的解决方案。

---

## 附录：用户余额要求

**重要发现**: 即使是V2模式，调用LLM API时仍需要用户余额 > 0

```go
// service/pre_consume_quota.go:39-40
if userQuota <= 0 {
    return "用户额度不足"
}
```

因此，任何模式下平台用户都需要有正的余额才能使用API。

---

## 修订历史

| 版本 | 日期 | 变更内容 |
|------|------|---------|
| 0.1 | 2026-01-22 | 初稿，整理V1/V2差异和解决方案 |
