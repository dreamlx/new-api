# 外部用户系统集成 API 文档 (V2 - 平台集成)

**📖 文档导航**：[← 返回API总览](external-api-overview.md) | [V1个人用户API →](external-user-api.md)

---

## 概述

本文档描述了 New API 为外部用户系统（下游平台）设计的 **V2 平台集成**方案。该方案旨在为下游平台提供更大的灵活性，允许平台方自主管理用户和API密钥（Token），同时利用 New API 作为强大的LLM网关。

**适用场景**：
- ✅ 下游平台集成（如API转售平台）
- ✅ 平台拥有自己的用户系统和计费系统
- ✅ 平台需要自己生成和管理Token
- ✅ New API仅作为LLM网关，不参与计费

**如果你的场景是面向个人用户的前端应用**，请查看 [V1 个人用户API文档](external-user-api.md)。

---

## 🚀 快速开始（3分钟上手）

### 完整工作流示例

```bash
# 1. 授权平台Token（首次）
curl -X POST "http://localhost:3000/api/v2/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d '{
    "platform_id": "asd",
    "token_key": "sk-abc123def456789xyz",
    "metadata": {
      "platform_user_id": "user_123",
      "user_type": "premium"
    }
  }'
# 响应：status: "authorized", current_quota: 5000000（首次有额度）

# 2. 重复授权同一Token（转为无限额度）
curl -X POST "http://localhost:3000/api/v2/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d '{
    "platform_id": "asd",
    "token_key": "sk-abc123def456789xyz",
    "initial_quota": 10000000
  }'
# 响应：status: "updated_unlimited", current_quota: 0（无限额度）

# 3. 用户使用Token调用LLM
curl -X POST "http://localhost:3000/v1/chat/completions" \
  -H "Authorization: Bearer sk-abc123def456789xyz" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# 4. 查询平台消费流水
curl "http://localhost:3000/api/v2/external/platforms/asd/logs?start_date=2025-11-01&end_date=2025-11-30"
# 响应包含：完整token_key、模型名、tokens数、quota消费
```

**关键特性**：
- ✅ Token自动获得无限额度（UnlimitedQuota=true）
- ✅ 平台查询消费流水后自己计费
- ✅ token_key返回完整密钥（便于平台匹配）

### 测试脚本

```bash
# 自动化测试（所有5个场景）
bash scripts/test-v2-platform-quota.sh

# 真实curl测试
bash /tmp/real-v2-api-test.sh

# 查看测试报告
cat /tmp/real-api-test-report.md  # 第262行起
```

**测试结果**：17/17 通过 ✅

---

### 核心设计理念：授权计费网关 (Authorized Billing Gateway)

v2 接口采用了一种简化的设计原则，其核心原则是：

- **New API作为纯计费网关**：不关心下游平台的用户管理体系，只负责验证被授权的密钥并进行计费。
- **保持内部兼容性**：New API内部仍然使用`users`表和`tokens`表来保持数据完整性，但对外部隐藏这些复杂性。
- **极简接口**：只提供两个核心接口，降低下游平台的集成复杂度。
- **零数据库修改**：基于现有`Log`表和`Token`表结构实现，无需任何数据库结构调整。

### v1 与 v2 对比

| 特性 | v1 (User-based) | v2 (Authorized Gateway) |
| :--- | :--- | :--- |
| **计费主体** | 用户 (`external_user_id`) | **密钥 (`token_key`)** |
| **密钥生成方** | New API | **下游平台** |
| **用户管理** | New API管理 | **下游平台管理** |
| **接口复杂度** | 7个接口 | **2个核心接口** |
| **New API角色** | 用户系统 + 计费网关 | **纯计费网关** |

---

## API 接口列表

### 1. 密钥授权接口

此接口用于在New API系统中授权一个由下游平台生成的密钥，并为其设置初始计费额度。

- **接口**: `POST /v2/external/tokens/authorize`
- **描述**: 授权一个新的API密钥，使其可以在New API系统中进行模型调用和计费。
- **幂等性**: 是。使用相同的`token_key`多次调用，将更新额度而非重复创建。
- **内部实现**: 基于`Token`表和`User`表，`platform_id`对应特殊用户`username="platform_{platform_id}"`。

**请求体 (`Request Body`)**:
```json
{
  "platform_id": "asd",
  "token_key": "sk-abc123def456789xyz",
  "initial_quota": 5000000,
  "metadata": {
    "platform_user_id": "user_123",
    "user_type": "premium",
    "created_by": "admin"
  }
}
```

**字段说明**:
- `platform_id`: 必填，下游平台的唯一标识符，对应New API内部用户`username="platform_{platform_id}"`。
- `token_key`: 必填，下游平台生成的API密钥，**必须以`sk-`开头**且长度不少于10个字符。系统会自动去掉`sk-`前缀后存储。
- `initial_quota`: ~~必填，初始额度，单位为quota~~ **已废弃**（2025-11-18）
- `metadata`: 可选，平台自定义的元数据，用于内部追踪。

⚠️ **2025-11-18重要变更**：
- V2平台Token统一使用**无限额度模式** (`UnlimitedQuota=true`)
- `initial_quota`参数已废弃，系统自动忽略
- V2平台在自己的系统完成计费，New API仅作为LLM网关
- 无需担心额度限制，所有V2 Token自动获得无限额度

**⚠️ 重要说明**：
- `token_key` 必须以 `sk-` 开头，格式为：`sk-{随机字符串}`
- **随机字符串部分禁止包含 `-` 分隔符**，因为系统验证时会按 `-` 分割并只取第一部分
- ✅ **正确示例**：`sk-a99416b67cb54e178e9ffe8a55c255ae`（推荐使用32-48位字母数字组合）
- ❌ **错误示例**：`sk-2-a99416b67cb54e178e9ffe8a55c255ae`（包含额外的 `-`，会导致验证失败）
- 系统会自动去掉 `sk-` 前缀后存储到数据库
- 用户调用 API 时需要带完整 token，例如：`Authorization: Bearer sk-a99416b67cb54e178e9ffe8a55c255ae`

**响应体 (`Response Body` - 成功)**:
```json
{
  "success": true,
  "message": "密钥授权成功",
  "data": {
    "token_key": "sk-abc123def456789xyz",
    "current_quota": 5000000,
    "quota_usd": 10.0,
    "status": "authorized",
    "created_at": "2025-10-21T10:30:00Z",
    "proxy_user_id": 1001
  }
}
```

**响应体 (`Response Body` - 密钥已存在)**:
```json
{
  "success": true,
  "message": "密钥已存在，额度已更新",
  "data": {
    "token_key": "sk-abc123def456789xyz",
    "previous_quota": 3000000,
    "current_quota": 5000000,
    "quota_added": 2000000,
    "status": "updated",
    "updated_at": "2025-10-21T10:30:00Z"
  }
}
```

**响应体 (`Response Body` - 错误)**:
```json
{
  "success": false,
  "message": "参数错误：token_key不能为空",
  "error_code": "INVALID_PARAMETER"
}
```

### 2. 平台消费流水查询接口

查询指定平台在时间范围内的所有消费记录，用于平台对账和审计。

- **接口**: `GET /v2/external/platforms/{platform_id}/logs`
- **描述**: 获取指定平台下所有密钥的消费日志，包含模型调用、token消费明细等。
- **内部实现**: 基于现有`Log`表和`Token`表，通过JOIN查询获取完整信息。

**查询参数 (`Query Parameters`)**:
- `platform_id`: 必填，路径参数，平台标识符
- `start_date`: 必填，开始日期，格式：2025-10-01
- `end_date`: 必填，结束日期，格式：2025-10-31
- `page`: 可选，页码，默认为1
- `page_size`: 可选，每页大小，默认20，最大100

**请求示例**:
```
GET /v2/external/platforms/asd/logs?start_date=2025-10-20&end_date=2025-10-21&page=1&page_size=20
```

**内部SQL查询逻辑**:
```sql
SELECT
  logs.id as log_id,
  logs.created_at,
  tokens.key as token_key,
  logs.model_name,
  logs.prompt_tokens,
  logs.completion_tokens,
  logs.quota,
  logs.other
FROM logs
LEFT JOIN tokens ON logs.token_id = tokens.id
LEFT JOIN users ON tokens.user_id = users.id
WHERE users.username = 'platform_asd'
  AND logs.type = 2  -- LogTypeConsume
  AND logs.created_at BETWEEN start_timestamp AND end_timestamp
ORDER BY logs.id DESC
```

**响应体 (`Response Body` - 成功)**:
```json
{
  "success": true,
  "message": "查询成功",
  "data": {
    "platform_id": "asd",
    "date_range": {
      "start_date": "2025-10-20",
      "end_date": "2025-10-21"
    },
    "logs": [
      {
        "log_id": "12345",
        "time": "2025-10-20T15:30:00Z",
        "token_key": "sk-abc123def456789xyz",
        "model_name": "claude-3-5-sonnet-20241022",
        "prompt_tokens": 150,
        "completion_tokens": 80,
        "total_tokens": 230,
        "quota_cost": 1150
      },
      {
        "log_id": "12344",
        "time": "2025-10-20T16:45:12Z",
        "token_key": "sk-xyz789uvw012345abc",
        "model_name": "gpt-4o",
        "prompt_tokens": 200,
        "completion_tokens": 120,
        "total_tokens": 320,
        "quota_cost": 960
      }
    ],
    "summary": {
      "total_requests": 2,
      "total_tokens": 550,
      "total_quota_consumed": 2110
    }
  }
}
```

**字段说明**：
- `token_key`: **完整的Token密钥**（2025-11-18修复）
  - ✅ 返回完整密钥：`sk-abc123def456789xyz`
  - ✅ 便于平台方匹配识别
  - ✅ 平台可根据token_key分组统计各Token消费
  - ⚠️ 注意安全：仅在授权的平台间传输
    ],
    "pagination": {
      "page": 1,
      "page_size": 20,
      "total_items": 2,
      "total_pages": 1,
      "has_next": false,
      "has_prev": false
    },
    "summary": {
      "total_requests": 2,
      "total_prompt_tokens": 350,
      "total_completion_tokens": 200,
      "total_tokens": 550,
      "total_quota_consumed": 2110,
      "unique_tokens": 2
    }
  }
}
```

**响应体 (`Response Body` - 平台不存在)**:
```json
{
  "success": false,
  "message": "指定的平台不存在",
  "error_code": "PLATFORM_NOT_FOUND"
}
```

**响应体 (`Response Body` - 参数错误)**:
```json
{
  "success": false,
  "message": "platform_id参数不能为空",
  "error_code": "MISSING_PARAMETER"
}
```

---

## 完整工作流示例

### 步骤1：下游平台生成并授权密钥

下游平台`asd`为用户`user_123`生成密钥，并在New API中授权：

```bash
curl -X POST "http://your-new-api-domain.com/v2/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d '{
    "platform_id": "asd",
    "token_key": "sk-user123key20251021abc",
    "initial_quota": 10000000,
    "metadata": {
      "platform_user_id": "user_123",
      "user_type": "premium"
    }
  }'
```

**响应**:
```json
{
  "success": true,
  "message": "密钥授权成功",
  "data": {
    "token_key": "sk-user123key20251021abc",
    "current_quota": 10000000,
    "quota_usd": 20.0,
    "status": "authorized",
    "created_at": "2025-10-21T10:30:00Z",
    "platform_user_id": "platform_asd"
  }
}
```

### 步骤2：终端用户使用密钥调用模型

终端用户使用授权的密钥调用标准OpenAI兼容接口：

```bash
curl -X POST "http://your-new-api-domain.com/v1/chat/completions" \
  -H "Authorization: Bearer sk-user123key20251021abc" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "messages": [
      {"role": "user", "content": "Hello, how are you?"}
    ],
    "max_tokens": 100
  }'
```

### 步骤3：下游平台查询平台消费记录

下游平台查询整个平台的消费流水进行对账：

```bash
curl -X GET "http://your-new-api-domain.com/v2/external/platforms/asd/logs?start_date=2025-10-20&end_date=2025-10-21&page_size=50"
```

**响应**会显示平台`asd`下所有密钥的消费记录，下游平台可以按`token_key`进行分组统计。

---

## 错误码说明

| 错误码 | HTTP状态码 | 说明 |
|--------|------------|------|
| `INVALID_PARAMETER` | 400 | 请求参数格式错误或缺少必填参数 |
| `PLATFORM_NOT_FOUND` | 404 | 指定的平台不存在 |
| `TOKEN_EXISTS` | 409 | 密钥已存在但属于不同平台 |
| `QUOTA_EXCEEDED` | 400 | 授权的额度超过限制 |
| `INTERNAL_ERROR` | 500 | 服务器内部错误 |

---

## 技术实现细节

### 数据库表结构利用

V2接口完全基于现有数据库表结构实现，无需任何修改：

#### Token表关键字段
```sql
-- model/token.go:13-30
type Token struct {
    Id          int    `json:"id"`
    UserId      int    `json:"user_id"`      -- 关联用户ID
    Key         string `json:"key"`          -- 完整token_key (sk-...)
    Name        string `json:"name"`         -- token名称
    RemainQuota int    `json:"remain_quota"` -- 剩余额度
    UsedQuota   int    `json:"used_quota"`   -- 已使用额度
    -- ... 其他字段
}
```

#### Log表关键字段
```sql
-- model/log.go:19-39
type Log struct {
    Id               int    `json:"id"`                -- 用作log_id
    UserId           int    `json:"user_id"`           -- 关联用户ID
    TokenId          int    `json:"token_id"`          -- 关联Token ID
    CreatedAt        int64  `json:"created_at"`        -- 时间戳
    Type             int    `json:"type"`              -- LogTypeConsume=2
    ModelName        string `json:"model_name"`        -- 模型名
    PromptTokens     int    `json:"prompt_tokens"`     -- 输入tokens
    CompletionTokens int    `json:"completion_tokens"` -- 输出tokens
    Quota            int    `json:"quota"`             -- 消费的quota (重要参考)
    -- ... 其他字段
}
```

#### 平台用户映射
- `platform_id: "asd"` → `User.Username: "platform_asd"`
- 该用户下的所有Token都属于该平台
- 通过JOIN查询获取平台级别的消费统计

### 现有函数复用

V2接口将复用以下现有函数：
- `model.GetLogByKey()` - 获取指定token的日志
- `model.GetAllLogs()` - 按条件查询日志
- `model.GetUserByUsername()` - 根据用户名获取用户信息
- `common.ValidateUserToken()` - 验证token有效性

### 性能考虑

- **索引利用**：充分利用现有索引（`idx_created_at_id`, `index_username_model_name`等）
- **分页查询**：支持大数据量的分页查询
- **JOIN优化**：Token表和User表都有适当的索引，JOIN性能良好

---

## ❓ 常见问题 (FAQ)

### Q1: 为什么V2 Token是无限额度？
**A**: V2模式的设计理念：
- 下游平台有自己的计费系统
- New API仅作为LLM网关，不参与额度控制
- 平台查询消费流水后自己计费
- UnlimitedQuota=true，Token永不因额度耗尽而失效

**业务流程**：
```
平台用户消费 → New API记录流水 → 平台查询流水 → 平台自己扣费
```

### Q2: Token格式为什么不能有连续的短横线？
**A**: 系统验证机制导致：
- 系统按`-`分割Token进行验证
- 只取第一段进行匹配
- 额外的短横线会导致分割错误

**示例**：
- ✅ `sk-a99416b67cb54e178e9ffe8a55c255ae` → 验证"a99416b67cb54e178e9ffe8a55c255ae"
- ❌ `sk-2-a99416b67cb54e178e9ffe8a55c255ae` → 验证"2"（错误）

### Q3: 首次授权和重复授权有什么区别？
**A**: 幂等性设计：

| 操作 | 首次授权 | 重复授权 |
|------|---------|---------|
| **状态** | "authorized" | "updated_unlimited" |
| **额度** | 首次设置的initial_quota | 0（无限额度） |
| **目的** | 创建Token | 转为无限额度模式 |

**实际效果**：
- 首次：Token带有initial_quota（如5,000,000）
- 重复：自动转为无限额度（current_quota=0）

### Q4: 为什么消费流水返回完整token_key？
**A**: 平台方的需求（2025-11-18修复）：
- 平台需要根据token_key匹配自己的用户
- 完整密钥便于平台分组统计各Token消费
- 安全性由平台间授权保证

**修复前**：返回Token名称（不可用）
**修复后**：返回完整密钥（`sk-abc123def456789xyz`）

### Q5: initial_quota参数还有用吗？
**A**: 已废弃（2025-11-18）：
- ⚠️ V2 Token统一使用无限额度模式
- initial_quota参数被系统忽略
- 即使传入也不会限制Token额度
- 建议不传该参数

### Q6: 如何计算平台应该向用户收费？
**A**: 根据消费流水自行计费：

**方案1：按quota计费**（推荐）
```javascript
total_quota = 2110  // 从API获取
price = total_quota / 500000  // $0.00422
```

**方案2：按tokens计费**
```javascript
total_tokens = 550  // prompt + completion
// 根据自己的定价策略计算
```

**方案3：自定义倍率**
```javascript
// 平台可以设置自己的加价倍率
platform_price = api_cost * 1.2  // 加价20%
```

### Q7: 如何处理消费流水的分页？
**A**: API支持分页查询：
```bash
# 第1页（20条）
GET /api/v2/external/platforms/asd/logs?start_date=2025-11-01&end_date=2025-11-30&page=1&page_size=20

# 第2页
GET /api/v2/external/platforms/asd/logs?start_date=2025-11-01&end_date=2025-11-30&page=2&page_size=20
```

**响应包含分页信息**：
```json
{
  "pagination": {
    "page": 1,
    "page_size": 20,
    "total_items": 150,
    "total_pages": 8,
    "has_next": true
  }
}
```

### Q8: V2和V1 API可以同时使用吗？
**A**: ✅ 可以，完全独立：
- V1用于个人用户前端
- V2用于平台级别集成
- 数据库层面完全隔离
- 不会互相影响

👉 查看 [V1个人用户API文档](external-user-api.md)

### Q9: 如何验证Token是否授权成功？
**A**: 直接调用LLM API测试：
```bash
curl -X POST "http://localhost:3000/v1/chat/completions" \
  -H "Authorization: Bearer sk-your-token" \
  -H "Content-Type: application/json" \
  -d '{"model": "deepseek-chat", "messages": [{"role": "user", "content": "test"}]}'
```

**成功**：返回模型响应
**失败**：返回401/403错误

---

## 注意事项

1. **额度单位**: `$1 USD = 500,000 quota`，所有额度相关字段均使用quota单位。
2. **幂等性**: `authorize`接口是幂等的，多次调用相同`token_key`将转为无限额度模式。
3. **日志唯一性**: 每条消费记录都有唯一的`log_id`（使用logs.id），可用于数据同步和去重。
4. **时区**: 所有时间字段均使用UTC时间，格式为ISO 8601。
5. **Token消费数据**: 响应中包含原始的`prompt_tokens`、`completion_tokens`和`quota_cost`，下游平台可自行换算费用或直接参考quota值。
6. **平台隔离**: 不同`platform_id`的数据完全隔离，确保数据安全。
7. **零数据库修改**: V2接口基于现有表结构实现，无需任何数据库调整。
8. **Token密钥安全**: 消费流水返回完整token_key，仅在授权的平台间传输，注意API安全防护。
9. **无限额度模式**: V2 Token自动获得无限额度（UnlimitedQuota=true），无需担心额度限制。

---
*文档版本：v2.0*
*最后更新：2025-11-18*