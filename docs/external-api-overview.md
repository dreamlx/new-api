# New API 外部用户系统集成总览

> **一站式导航**：快速了解New API的外部用户集成方案，选择适合你的API版本

---

## 🎯 快速决策：选择适合你的API版本

### V1 vs V2 对比表

| 维度 | V1 API (个人用户) | V2 API (平台集成) |
|------|------------------|-------------------|
| **适用场景** | 前端用户系统集成 | 下游平台（如asd）集成 |
| **Token额度** | 独立额度（RemainQuota） | 无限额度（UnlimitedQuota） |
| **计费主体** | New API计费 | 平台自己计费 |
| **用户管理** | New API管理用户 | 平台自己管理用户 |
| **接口复杂度** | 7个接口 | 2个核心接口 |
| **Token生成** | New API生成 | 平台自己生成 |
| **计费模式** | 用户余额池 → 分配给Token | 平台消费，New API仅作为网关 |
| **🆕 Callback回调** | ✅ 支持（可选） | ❌ 不支持 |

### 使用场景示例

**选择 V1 API，如果你的场景是**：
- ✅ 开发面向个人用户的前端应用
- ✅ 用户需要充值和余额管理
- ✅ 每个用户可以创建多个API Token
- ✅ 需要Token独立额度控制
- ✅ 需要详细的用户消费记录
- ✅ 🆕 需要实时消费回调通知（如CEC平台集成）

**选择 V2 API，如果你的场景是**：
- ✅ 你是一个下游平台（如API转售平台）
- ✅ 你有自己的用户系统和计费系统
- ✅ 你需要自己生成和管理API Token
- ✅ 你需要New API仅作为LLM网关
- ✅ 你需要查询平台整体的消费流水

---

## 📚 核心概念速览

### Quota 计费机制
```
$1 USD = 500,000 quota
消耗quota = 分组倍率 × 模型倍率 × (输入tokens + 输出tokens × 补全倍率)
```

**示例**（deepseek-chat）：
- 输入：135 quota/1K tokens
- 输出：540 quota/1K tokens
- 模型倍率：0.135
- 补全倍率：4

### Token 机制对比

#### V1 Token（独立额度模式）
```
User充值 → User.Quota（余额池）
  ↓
创建Token → 从User.Quota扣除，分配给Token.RemainQuota
  ↓
Token消费 → Token.RemainQuota独立扣减
```

**特点**：
- Token额度独立，互不影响
- 用户余额分配给Token后不可收回
- 每个Token消耗自己的RemainQuota

#### V2 Token（无限额度模式）
```
Platform生成Token → New API授权
  ↓
Token.UnlimitedQuota = true（无限额度）
  ↓
Platform查询消费流水 → 平台自己计费
```

**特点**：
- Token无额度限制
- New API仅记录消费流水
- 平台根据流水自己计费

---

## 🚀 快速开始指南

### V1 个人用户快速开始（5分钟上手）

#### 1. 用户注册并充值
```bash
# 同步用户
curl -X POST "http://localhost:3000/api/user/external/sync" \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "user_001",
    "username": "testuser",
    "display_name": "测试用户",
    "email": "user@example.com"
  }'

# 充值 $50
curl -X POST "http://localhost:3000/api/user/external/topup" \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "user_001",
    "amount_usd": 50,
    "payment_id": "payment_123"
  }'
```

#### 2. 创建Token（分配$20额度）
```bash
curl -X POST "http://localhost:3000/api/user/external/token" \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "user_001",
    "token_name": "我的API密钥",
    "allocated_quota": 10000000,  # $20 = 10,000,000 quota
    "expires_in_days": 365
  }'
```

#### 3. 使用Token调用LLM
```bash
curl -X POST "http://localhost:3000/v1/chat/completions" \
  -H "Authorization: Bearer sk-xxxxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

👉 **完整文档**：[V1 个人用户API文档](external-user-api.md)

---

### V2 平台集成快速开始（3分钟上手）

#### 1. 授权平台Token
```bash
curl -X POST "http://localhost:3000/api/v2/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d '{
    "platform_id": "asd",
    "token_key": "sk-abc123def456789xyz",
    "metadata": {
      "platform_user_id": "user_123"
    }
  }'
```

**注意**：
- ✅ Token格式：`sk-{连续字符串}`（推荐32-48位字母数字）
- ❌ 禁止格式：`sk-2-xxx`（不能有额外的短横线）
- V2 Token自动获得**无限额度**（UnlimitedQuota=true）

#### 2. 查询平台消费流水
```bash
curl -X GET "http://localhost:3000/api/v2/external/platforms/asd/logs?start_date=2025-11-01&end_date=2025-11-30"
```

**响应包含**：
- 每条消费记录的完整Token密钥（便于平台匹配）
- 模型名称、输入/输出tokens
- Quota消费明细

👉 **完整文档**：[V2 平台Token API文档](external-user-api-v2.md)

---

## 📊 完整测试报告

### V1 API 真实测试
- **测试报告**：[Token独立额度真实测试报告](/tmp/real-api-test-report.md)
- **测试脚本**：`bash scripts/test-v1-token-quota.sh`
- **测试结果**：19/19 通过 ✅
- **关键验证**：
  - ✅ `remain_quota` 字段正确显示
  - ✅ 用户余额扣减逻辑正确
  - ✅ Token验证JSON结构完整

### V2 API 真实测试
- **测试报告**：[V2平台Token真实测试报告](/tmp/real-api-test-report.md)（第262行起）
- **测试脚本**：`bash scripts/test-v2-platform-quota.sh`
- **测试结果**：17/17 通过 ✅
- **关键验证**：
  - ✅ 首次授权正确（status: "authorized"）
  - ✅ 重复授权转无限额度（current_quota: 0）
  - ✅ 消费流水JSON格式完整（logs为空数组而非null）
  - ✅ token_key显示完整密钥

---

## 📖 详细文档导航

### V1 个人用户API
- **文档**：[external-user-api.md](external-user-api.md)
- **接口数量**：7个核心接口
- **核心接口**：
  1. `POST /api/user/external/sync` - 用户同步
  2. `POST /api/user/external/topup` - 用户充值
  3. `POST /api/user/external/token` - 创建Token（🆕 支持callback配置）
  4. `GET /api/user/external/:id/tokens` - Token列表
  5. `POST /api/user/external/token/verify` - Token验证
  6. `GET /api/user/external/:id/stats` - 用户统计
  7. `GET /api/user/external/:id/logs` - 消费记录

### 🆕 V1 Callback回调功能（可选扩展）
- **文档**：[callback-feature.md](callback-feature.md)
- **说明**：V1 API的可选扩展功能，非独立API
- **使用场景**：下游平台（如CEC）需要实时接收消费通知
- **工作原理**：
  1. Token创建时配置`callback_url`（可选）
  2. Token消费时自动异步回调下游平台
  3. 下游平台自己做统计汇总（"Agent消费"）
- **安全机制**：IP白名单（Nginx层配置）
- **失败处理**：只记录日志，不重试

### V2 平台Token API
- **文档**：[external-user-api-v2.md](external-user-api-v2.md)
- **接口数量**：2个核心接口
- **核心接口**：
  1. `POST /api/v2/external/tokens/authorize` - Token授权
  2. `GET /api/v2/external/platforms/:id/logs` - 平台消费流水

---

## ❓ 常见问题 (FAQ)

### Q1: V1和V2可以同时使用吗？
✅ **可以**。两种模式完全独立，互不干扰。
- V1用于个人用户前端
- V2用于下游平台集成
- 数据库层面完全隔离

### Q2: 如何选择V1还是V2？
**决策树**：
```
你是否有自己的计费系统？
├─ 是 → 使用 V2 API（平台模式）
└─ 否 → 使用 V1 API（个人用户模式）
```

### Q3: V2 Token为什么是无限额度？
**设计理念**：
- V2模式下，New API仅作为LLM网关
- 平台自己管理用户和计费
- 平台查询消费流水后自己扣费
- New API不参与额度控制

### Q4: Token独立额度是什么意思？
**V1模式特性**：
- 每个Token有自己的`RemainQuota`
- Token消费时只扣减自己的额度
- Token之间互不影响
- 用户可以创建多个Token，分别控制额度

### Q5: 充值金额如何转换为quota？
**转换规则**：
```
$1 USD = 500,000 quota
$50 USD = 25,000,000 quota
$100 USD = 50,000,000 quota
```

### Q6: 如何计算模型消费？
**计费公式**：
```
消耗quota = 分组倍率 × 模型倍率 × (输入tokens + 输出tokens × 补全倍率)
```

**deepseek-chat示例**：
- 输入1000 tokens：135 quota
- 输出1000 tokens：540 quota

### Q7: V2 Token格式有什么要求？
**正确格式**：
- ✅ `sk-a99416b67cb54e178e9ffe8a55c255ae`（推荐）
- ✅ `sk-` 后面是连续的字母数字组合

**错误格式**：
- ❌ `sk-2-a99416b67cb54e178e9ffe8a55c255ae`（额外的短横线）
- ❌ `abc123def456`（缺少sk-前缀）

**原因**：系统验证时按`-`分割，只取第一段，额外的短横线会导致验证失败。

---

## 🛠️ 开发资源

### 测试脚本
```bash
# V1 自动化测试
bash scripts/test-v1-token-quota.sh

# V2 自动化测试
bash scripts/test-v2-platform-quota.sh

# 完整用户故事测试
bash scripts/test-user-story.sh
```

### 单元测试
```bash
# 运行所有外部用户API单元测试
go test ./controller -v -timeout 60s -run "Test.*ExternalUser"
```

### 开发环境
- **启动数据库**：`make dev-db`
- **启动后端**：`make start`
- **运行测试**：`make test-api`

---

## 📝 技术支持

### 文档版本
- **V1 API**：v2.0（最后更新：2025-01-31）
- **V2 API**：v2.0（最后更新：2025-10-21）
- **本总览**：v1.0（创建日期：2025-11-18）

### 获取帮助
1. 查看详细API文档（V1或V2）
2. 运行测试脚本验证环境
3. 查看真实测试报告
4. 查阅 `CLAUDE.md` 开发记录

---

**快速导航**：
- [V1 个人用户API文档 →](external-user-api.md)
- [V2 平台Token API文档 →](external-user-api-v2.md)
- [开发指南](development-guide.md)
