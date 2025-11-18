# 外部用户系统集成 API 文档 (V1 - 个人用户)

**📖 文档导航**：[← 返回API总览](external-api-overview.md) | [V2平台API →](external-user-api-v2.md)

---

## 项目概述

本文档描述了 New API 的 **V1 个人用户**集成方案，允许前端平台通过 API 与 New API 进行用户数据同步、充值管理和 Token 管理。

**适用场景**：
- ✅ 前端用户系统集成（微信、支付宝、邮箱等登录）
- ✅ 用户充值和余额管理
- ✅ Token独立额度控制
- ✅ 每个用户可创建多个Token

**如果你需要平台级别的集成（下游平台、无限额度Token）**，请查看 [V2 平台Token API文档](external-user-api-v2.md)。

---

## 🚀 快速开始（5分钟上手）

### 完整流程示例

以用户 Amos 的使用流程为例：

```bash
# 1. 用户微信登录后同步到 New API
curl -X POST "http://localhost:3000/api/user/external/sync" \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "amos_wechat_123",
    "username": "amos_chen",
    "display_name": "Amos Chen",
    "email": "amos@example.com",
    "wechat_openid": "wx_openid_12345",
    "login_type": "wechat"
  }'

# 2. 用户充值 $50
curl -X POST "http://localhost:3000/api/user/external/topup" \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "amos_wechat_123",
    "amount_usd": 50,
    "payment_id": "stripe_pi_1234567890"
  }'
# 响应：current_quota: 25000000 ($50)

# 3. 创建 Token（分配$20额度）
curl -X POST "http://localhost:3000/api/user/external/token" \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "amos_wechat_123",
    "token_name": "我的聊天应用",
    "allocated_quota": 10000000,
    "expires_in_days": 365
  }'
# 响应：access_key: "sk-xxxxxxxxxxxx", remain_quota: 10000000

# 4. 用户使用 Token 调用 LLM API
curl -X POST "http://localhost:3000/v1/chat/completions" \
  -H "Authorization: Bearer sk-xxxxxxxxxxxx" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'

# 5. 查看用户统计
curl "http://localhost:3000/api/user/external/amos_wechat_123/stats"
# 响应：current_quota: 15000000 ($30剩余)

# 6. 查询消费记录
curl "http://localhost:3000/api/user/external/amos_wechat_123/logs?page=1&limit=10"
```

**关键数据验证**：
- 充值：$50 = 25,000,000 quota
- 分配给Token：$20 = 10,000,000 quota
- 用户剩余：$30 = 15,000,000 quota

### 测试脚本

```bash
# 自动化测试（所有7个接口）
bash scripts/test-v1-token-quota.sh

# 真实curl测试
bash /tmp/real-api-test.sh

# 查看测试报告
cat /tmp/real-api-test-report.md
```

**测试结果**：19/19 通过 ✅

---

## 设计架构

### 核心理念
- **前端用户系统**：支持微信、支付宝、短信、邮箱等多种登录方式
- **New API 后端**：作为 LLM 网关和计费系统
- **映射机制**：通过 `external_user_id` 建立前端用户与 New API 用户的关联

### 计费策略

⚠️ **2025-11-18更新**：Token独立额度模式

#### Token独立额度架构
- **Token独立计费**：每个Token拥有独立的 `RemainQuota`，消耗各自额度
- **User余额池**：User充值到余额池，手动分配额度给Token
- **额度分配机制**：
  1. User充值 → `User.Quota` 增加
  2. 创建Token时从 `User.Quota` 扣除，分配给 `Token.RemainQuota`
  3. Token消费时只扣减自己的 `Token.RemainQuota`
  4. Token之间互不影响，独立计费

#### 货币和额度
- **货币统一**：前端收款任意货币 → 支付网关转换 → 后端只接收美元
- **汇率处理**：完全由前端网站和支付网关负责，New API 不处理汇率转换
- **计费逻辑**：$1 USD = 500,000 quota（使用 `common.QuotaPerUnit`）
- **模型计费**：基于 New API 的复杂计费公式：
  ```
  消耗quota = 分组倍率 × 模型倍率 × (输入tokens + 输出tokens × 补全倍率)
  ```

#### V1 vs V2 Token差异
| 特性 | V1 Token | V2 Token (平台) |
|------|----------|----------------|
| 额度模式 | 独立额度（RemainQuota） | 无限额度（UnlimitedQuota=true） |
| 计费主体 | New API计费 | 平台自己计费 |
| 适用场景 | 个人用户 | 下游平台（如asd） |

## API 接口

### 安全认证
- **无需认证**：外部用户 API 已移除认证限制，供前端系统直接调用
- **IP 白名单**：建议通过 Nginx 配置限制访问（可选）

### 接口列表
- `POST /api/user/external/sync` - 用户同步接口
- `POST /api/user/external/topup` - 用户充值接口
- `POST /api/user/external/token` - 创建 Access Key
- `DELETE /api/user/external/token` - 删除 Access Key
- `GET /api/user/external/{id}/tokens` - 获取用户所有Token列表 🆕
- `POST /api/user/external/token/verify` - 验证Token有效性 🆕
- `GET /api/user/external/{id}/stats` - 用户统计接口
- `GET /api/user/external/{id}/logs` - 消费记录查询接口

### 1. 用户同步接口

#### 创建或更新外部用户
```http
POST /api/user/external/sync
Content-Type: application/json
```

**请求参数**:
```json
{
  "external_user_id": "string, required, 外部用户唯一标识",
  "username": "string, required, 用户名",
  "display_name": "string, optional, 显示名称", 
  "email": "string, optional, 邮箱地址（可为虚拟邮箱）",
  "phone": "string, optional, 手机号码",
  "wechat_openid": "string, optional, 微信OpenID",
  "wechat_unionid": "string, optional, 微信UnionID",
  "alipay_userid": "string, optional, 支付宝用户ID",
  "login_type": "string, optional, 登录类型：email|wechat|alipay|sms",
  "aff_code": "string, optional, 推荐码（可选，用于推荐体系）",
  "external_data": "string, optional, 扩展数据（JSON字符串）"
}
```

**响应示例**:
```json
{
  "success": true,
  "message": "用户创建成功",
  "data": {
    "user_id": 123,
    "external_user_id": "test_user_001",
    "is_new_user": true
  }
}
```

**说明**:
- `external_user_id` 是前端用户系统的用户ID，作为唯一映射标识
- `email` 可以是虚拟邮箱，如 `"wechat_user_123@external.local"`
- 如果用户已存在，则更新用户信息，`is_new_user` 为 `false`
- `aff_code` 为推荐码，可选字段，用于构建推荐体系

### 2. 用户充值接口

#### 为外部用户充值
```http
POST /api/user/external/topup
Content-Type: application/json
```

**请求参数**:
```json
{
  "external_user_id": "string, required, 外部用户ID",
  "amount_usd": "number, required, 美元金额，最小0.01", 
  "payment_id": "string, required, 支付交易ID"
}
```

**响应示例**:
```json
{
  "success": true,
  "message": "充值成功",
  "data": {
    "amount_usd": 10.0,
    "quota_added": 5000000,
    "current_quota": 5000000,
    "current_balance": 10.0,
    "payment_id": "stripe_payment_123456"
  }
}
```

**说明**:
- `amount_usd` 必须是美元金额，前端负责所有货币转换
- `payment_id` 可以是任何支付方式的交易ID，用于追踪和对账
  - Stripe: `"stripe_pi_xxx"`
  - 微信支付: `"wechat_20241201_001"`
  - 支付宝: `"alipay_20241201_001"`
  - 充值卡: `"card_20241201_001"`
  - 自定义: `"custom_order_12345"`

### 3. Token 管理接口

#### 3.1 创建 Access Key

⚠️ **重要变更**（2025-11-18）：新增必填参数 `allocated_quota`，用于从用户余额分配额度给Token（Token独立额度模式）

```http
POST /api/user/external/token
Content-Type: application/json
```

**请求参数**:
```json
{
  "external_user_id": "string, required, 外部用户ID",
  "token_name": "string, required, Token名称",
  "allocated_quota": "number, required, 从User余额分配给Token的额度",
  "expires_in_days": "number, optional, 有效期天数，默认365",

  "callback_url": "string, optional, 🆕 回调URL（用于实时消费通知）",
  "callback_enabled": "boolean, optional, 🆕 是否启用回调，默认false",
  "callback_secret": "string, optional, 🆕 回调密钥（暂不使用，预留）"
}
```

**🆕 Callback回调参数说明**（可选功能）:
- `callback_url`: 下游平台接收消费通知的URL
  - 示例：`https://cec.example.com/api/consume-notify`
  - Token消费时会异步POST回调
- `callback_enabled`: 是否启用回调功能
  - 默认：false（不启用）
  - 适用场景：CEC等下游平台需要实时统计
- `callback_secret`: 回调签名密钥
  - 当前版本暂不使用（使用IP白名单保护）
  - 预留字段，未来可用于HMAC签名验证

**使用示例**：
```json
{
  "external_user_id": "cec_user_123",
  "token_name": "CEC用户Token",
  "allocated_quota": 10000000,
  "callback_url": "https://cec.example.com/api/consume-notify",
  "callback_enabled": true
}
```

**字段说明**:
- `allocated_quota`: **必填**，分配给Token的额度（单位：quota）
  - 此额度将从用户余额中扣除
  - 最小值：0（但建议至少10,000,000，即$20）
  - 最大值：不能超过用户当前余额
  - 示例：10,000,000 = $20 USD

**响应示例**（普通Token）:
```json
{
  "success": true,
  "message": "Token创建成功",
  "data": {
    "token_id": 1,
    "access_key": "sk-xxxxxxxxxxxxxxxxxxxx",
    "token_name": "My API Token",
    "expires_at": 1767195600,
    "remain_quota": 10000000
  }
}
```

**响应示例**（带Callback的Token）:
```json
{
  "success": true,
  "message": "Token创建成功",
  "data": {
    "token_id": 132,
    "access_key": "sk-xxxxxxxxxxxxxxxxxxxx",
    "token_name": "CEC用户Token",
    "expires_at": 1767195600,
    "remain_quota": 10000000,
    "callback_enabled": true,
    "callback_url_masked": "https://cec.example.com/***"
  }
}
```

**错误示例**:
```json
{
  "success": false,
  "message": "用户余额不足，当前余额: 5000000，请求分配: 10000000"
}
```

**注意事项**:
1. Token创建采用**数据库事务**保证原子性：
   - 从User余额扣除 `allocated_quota`
   - 为Token分配相同额度
   - 任一步骤失败则全部回滚
2. Token余额**独立计费**，消耗Token自身的 `RemainQuota`，不影响其他Token
3. User余额扣减后，剩余额度可继续分配给其他新Token

#### 3.2 删除 Access Key
```http
DELETE /api/user/external/token
Content-Type: application/json
```

**请求参数**:
```json
{
  "external_user_id": "string, required, 外部用户ID",
  "token_id": "number, required, 要删除的Token ID"
}
```

**响应示例**:
```json
{
  "success": true,
  "message": "Token删除成功",
  "data": {
    "token_id": 1,
    "external_user_id": "test_user_001"
  }
}
```

**说明**:
- 只能删除属于指定外部用户的Token
- Token删除后立即失效，无法恢复
- 删除不存在的Token或无权限的Token会返回404错误

#### 3.3 获取用户所有Token列表 🆕
```http
GET /api/user/external/{external_user_id}/tokens
```

**路径参数**:
- `external_user_id` (string, required): 外部用户ID

**响应示例**:
```json
{
  "success": true,
  "message": "查询成功",
  "data": {
    "external_user_id": "test_user_001",
    "total_tokens": 3,
    "tokens": [
      {
        "token_id": 1,
        "token_name": "生产环境Token",
        "access_key": "sk-abc12345****xyz9",
        "status": 1,
        "status_text": "启用",
        "remain_quota": 5000000,
        "used_quota": 1000000,
        "created_time": 1762310138,
        "expired_time": 1793846138,
        "accessed_time": 1762310200,
        "is_expired": false
      },
      {
        "token_id": 2,
        "token_name": "测试Token",
        "access_key": "sk-def67890****abc1",
        "status": 2,
        "status_text": "禁用",
        "remain_quota": 2000000,
        "used_quota": 500000,
        "created_time": 1762310100,
        "expired_time": 1764902100,
        "accessed_time": 1762310150,
        "is_expired": false
      },
      {
        "token_id": 3,
        "token_name": "已过期Token",
        "access_key": "sk-ghi12345****def2",
        "status": 1,
        "status_text": "启用",
        "remain_quota": 3000000,
        "used_quota": 0,
        "created_time": 1700000000,
        "expired_time": 1730000000,
        "accessed_time": 1730000000,
        "is_expired": true
      }
    ]
  }
}
```

**响应字段说明**:
- `total_tokens`: 该用户的Token总数
- `tokens`: Token列表，按创建时间倒序排列
  - `token_id`: Token唯一ID
  - `token_name`: Token名称
  - `access_key`: 脱敏后的访问密钥（只显示前8位和后4位）
  - `status`: Token状态码（1=启用, 2=禁用, 3=额度耗尽, 4=已过期）
  - `status_text`: Token状态文本
  - `remain_quota`: 剩余额度
  - `used_quota`: 已使用额度（估算值）
  - `created_time`: 创建时间（Unix时间戳）
  - `expired_time`: 过期时间（Unix时间戳，-1表示永不过期）
  - `accessed_time`: 最后访问时间（Unix时间戳）
  - `is_expired`: 是否已过期（根据当前时间计算）

**说明**:
- 返回指定用户的所有Token（包括已删除的不会返回）
- Token密钥经过脱敏处理，保护安全性
- 可用于前端展示用户的API密钥管理页面
- 用户不存在会返回404错误

#### 3.4 验证Token有效性 🆕
```http
POST /api/user/external/token/verify
Content-Type: application/json
```

**请求参数**:
```json
{
  "access_key": "string, required, 要验证的Token（包含sk-前缀）"
}
```

**响应示例（有效Token）**:
```json
{
  "success": true,
  "message": "验证完成",
  "data": {
    "is_valid": true,
    "token_id": 1,
    "token_name": "生产环境Token",
    "external_user_id": "test_user_001",
    "status": 1,
    "status_text": "启用",
    "remain_quota": 5000000,
    "expired_time": 1793846138,
    "is_expired": false
  }
}
```

**响应示例（无效Token）**:
```json
{
  "success": true,
  "message": "验证完成",
  "data": {
    "is_valid": false,
    "error_reason": "Token已过期（时间到期）"
  }
}
```

**响应字段说明**:
- `is_valid`: Token是否有效（true/false）
- 当Token有效时，返回以下字段：
  - `token_id`: Token ID
  - `token_name`: Token名称
  - `external_user_id`: 关联的外部用户ID
  - `status`: Token状态码
  - `status_text`: Token状态文本
  - `remain_quota`: 剩余额度
  - `expired_time`: 过期时间
  - `is_expired`: 是否已过期
- 当Token无效时，返回以下字段：
  - `error_reason`: 无效原因，可能的值：
    - "Token不存在"
    - "关联用户不存在"
    - "Token已被禁用"
    - "Token额度已耗尽"
    - "Token已过期（状态标记）"
    - "Token已过期（时间到期）"
    - "Token剩余额度不足"

**说明**:
- 用于验证Token是否可以正常使用
- 检查Token的存在性、状态、过期时间、额度等
- 即使验证失败，接口也返回200状态码，通过`is_valid`字段判断
- 可用于前端Token管理页面的实时验证功能
- ⚠️ 注意：刚删除的Token可能因Redis缓存暂时仍显示有效，几分钟后会失效

### 4. 用户统计接口

#### 获取用户使用统计
```http
GET /api/user/external/{external_user_id}/stats
```

**响应示例**:
```json
{
  "success": true,
  "data": {
    "user_info": {
      "external_user_id": "test_user_001",
      "username": "testuser",
      "display_name": "测试用户",
      "current_quota": 15000000,
      "current_balance": 30.0,
      "used_quota": 0,
      "total_requests": 0,
      "balance_capacity": {
        "deepseek-chat": {
          "input_tokens_1k": 111111,
          "model_ratio": 0.135,
          "completion_ratio": 4,
          "group_ratio": 1,
          "base_price_usd": 0.00027,
          "quota_per_1k_input": 135,
          "pricing_note": "输入：135 quota/1K tokens，输出：540 quota/1K tokens",
          "is_default_model": true
        },
        "qwen-turbo": {
          "input_tokens_1k": 17502,
          "model_ratio": 0.8572,
          "completion_ratio": 1,
          "group_ratio": 1,
          "base_price_usd": 0.0017144,
          "quota_per_1k_input": 857,
          "pricing_note": "输入：857 quota/1K tokens，输出：857 quota/1K tokens",
          "is_default_model": true
        },
        "_summary": {
          "total_balance_usd": 30.0,
          "total_quota": 15000000,
          "quota_per_usd": 500000,
          "billing_formula": "消耗quota = 分组倍率 × 模型倍率 × (输入tokens + 输出tokens × 补全倍率)",
          "models_available": 5,
          "note": "实际消费取决于输入和输出token数量，此处仅显示输入token的估算"
        }
      }
    },
    "tokens": [
      {
        "id": 1,
        "name": "My API Token",
        "key": "sk-xxxxxxxxxxxxxxxxxxxx",
        "status": 1,
        "expired_time": 1767195600
      }
    ],
    "recent_logs": [],
    "model_usage": {}
  }
}
```

**balance_capacity 说明**:
- 显示用户当前余额可以调用各种模型的次数
- `is_default_model: true` 表示该模型是渠道的默认测试模型，会优先显示
- `input_tokens_1k`: 可调用的1K输入tokens次数
- `pricing_note`: 详细的计费说明，包含输入和输出token的消费
- 只显示当前启用渠道的模型，禁用渠道的模型不会出现

### 5. 消费记录查询接口

#### 获取用户消费记录
```http
GET /api/user/external/{external_user_id}/logs
```

**查询参数**:
- `start_date` (string, optional): 开始日期，格式：2024-01-01
- `end_date` (string, optional): 结束日期，格式：2024-01-31  
- `username` (string, optional): 用户名筛选
- `model_name` (string, optional): 模型名筛选（支持模糊匹配）
- `page` (int, optional): 页码，默认1
- `page_size` (int, optional): 每页大小，默认20，最大100

**响应示例**:
```json
{
  "success": true,
  "data": {
    "logs": [
      {
        "time": "2024-01-30 15:30:25",
        "username": "testuser",
        "tokens": 80,
        "type": "consume",
        "model": "qwen-turbo",
        "spend": 0.002
      },
      {
        "time": "2024-01-30 10:00:00",
        "username": "testuser", 
        "tokens": 0,
        "type": "topup",
        "model": "",
        "spend": -10.0
      }
    ],
    "pagination": {
      "page": 1,
      "page_size": 20,
      "total": 25,
      "total_page": 2
    },
    "summary": {
      "total_tokens": 1250,
      "total_spend": 2.15
    }
  }
}
```

**字段说明**:
- `time`: 记录时间，格式：YYYY-MM-DD HH:mm:ss
- `username`: 用户名
- `tokens`: Token消费数量（prompt + completion），充值记录为0
- `type`: 记录类型
  - `consume`: 消费记录（调用LLM）
  - `topup`: 充值记录
  - `error`: 错误记录
- `model`: 使用的模型名称，充值记录为空
- `spend`: 花费金额（美元）
  - 正数：实际消费
  - 负数：充值金额（显示为负数便于区分）
- `pagination`: 分页信息
- `summary`: 汇总信息
  - `total_tokens`: 本页记录的总Token消费
  - `total_spend`: 本页记录的总花费

**使用示例**:
```bash
# 查询所有记录
GET /api/user/external/test_user_001/logs

# 按日期范围查询
GET /api/user/external/test_user_001/logs?start_date=2024-01-01&end_date=2024-01-31

# 按模型筛选
GET /api/user/external/test_user_001/logs?model_name=qwen

# 分页查询
GET /api/user/external/test_user_001/logs?page=2&page_size=10

# 组合查询
GET /api/user/external/test_user_001/logs?start_date=2024-01-15&model_name=qwen&page=1&page_size=50
```

## LLM API 使用

创建Token后，用户可以使用标准的OpenAI兼容API调用LLM模型：

### Chat Completions API
```http
POST /v1/chat/completions
Authorization: Bearer sk-xxxxxxxxxxxxxxxxxxxx
Content-Type: application/json
```

**请求示例**:
```json
{
  "model": "qwen-turbo",
  "messages": [
    {
      "role": "user",
      "content": "你好！"
    }
  ]
}
```

**响应示例**:
```json
{
  "choices": [
    {
      "message": {
        "content": "你好！很高兴见到你！😊 今天过得怎么样？有什么我可以帮你的吗？",
        "role": "assistant"
      },
      "finish_reason": "stop",
      "index": 0,
      "logprobs": null
    }
  ],
  "object": "chat.completion",
  "usage": {
    "prompt_tokens": 14,
    "completion_tokens": 18,
    "total_tokens": 32,
    "prompt_tokens_details": {
      "cached_tokens": 0
    }
  },
  "created": 1753902335,
  "system_fingerprint": null,
  "model": "qwen-turbo",
  "id": "chatcmpl-xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
}
```

## 完整用户流程示例

### 用户 Amos 的使用流程

```javascript
const newApi = new NewAPIClient('https://api.example.com');

// 1. 用户微信登录后同步到 New API
await newApi.syncUser({
  external_user_id: 'amos_wechat_123',
  username: 'amos_chen',
  display_name: 'Amos Chen',
  email: 'amos@example.com',
  wechat_openid: 'wx_openid_12345',
  login_type: 'wechat',
  aff_code: 'REFERRAL_ABC123'  // 可选推荐码
});

// 2. 用户充值 500元人民币（Stripe转换为$68.49）
await newApi.topupUser({
  external_user_id: 'amos_wechat_123',
  amount_usd: 68.49,  // Stripe转换后的美元金额
  payment_id: 'stripe_pi_1234567890'
});

// 3. 创建 Access Key
const token = await newApi.createToken({
  external_user_id: 'amos_wechat_123',
  token_name: 'My Chat App',
  expires_in_days: 365
});

// 4. 查看用户统计和可用模型
const stats = await newApi.getUserStats('amos_wechat_123');
console.log('可用模型：', Object.keys(stats.data.user_info.balance_capacity));
console.log('余额：$', stats.data.user_info.current_balance);

// 5. 用户使用 Access Key 调用 LLM API
const response = await fetch('https://api.example.com/v1/chat/completions', {
  method: 'POST',
  headers: {
    'Authorization': `Bearer ${token.access_key}`,
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({
    model: 'qwen-turbo',  // 使用可用的模型
    messages: [{ role: 'user', content: 'Hello!' }]
  })
});

// 6. 查看使用后的统计
const updatedStats = await newApi.getUserStats('amos_wechat_123');
console.log('消费后余额：$', updatedStats.data.user_info.current_balance);
console.log('总请求次数：', updatedStats.data.user_info.total_requests);

// 7. 查询消费记录
const logs = await newApi.getUserLogs('amos_wechat_123', {
  start_date: '2024-01-01',
  end_date: '2024-01-31',
  page: 1,
  page_size: 10
});
console.log('消费记录：', logs.data.logs);
console.log('总消费：$', logs.data.summary.total_spend);
console.log('消费Token数：', logs.data.summary.total_tokens);

// 8. 删除不需要的Token
await newApi.deleteToken({
  external_user_id: 'amos_wechat_123',
  token_id: token.data.token_id
});
console.log('Token删除成功');
```

### 常见消费记录查询场景

#### 场景1：用户账单查询
```javascript
// 查询当月消费记录
const monthlyLogs = await fetch('/api/user/external/user_001/logs?start_date=2024-01-01&end_date=2024-01-31');
const data = await monthlyLogs.json();

// 按类型统计
const consumeRecords = data.data.logs.filter(log => log.type === 'consume');
const topupRecords = data.data.logs.filter(log => log.type === 'topup');

console.log(`本月消费：${consumeRecords.length}次，充值：${topupRecords.length}次`);
```

#### 场景2：模型使用分析
```javascript
// 查询特定模型的使用情况
const modelLogs = await fetch('/api/user/external/user_001/logs?model_name=qwen&page_size=100');
const data = await modelLogs.json();

// 计算模型使用统计
const modelStats = data.data.logs.reduce((stats, log) => {
  const model = log.model;
  if (!stats[model]) stats[model] = { count: 0, tokens: 0, spend: 0 };
  stats[model].count++;
  stats[model].tokens += log.tokens;
  stats[model].spend += log.spend;
  return stats;
}, {});

console.log('模型使用统计：', modelStats);
```

#### 场景3：成本控制监控
```javascript
// 查询最近7天的消费趋势
const weeklyLogs = await fetch('/api/user/external/user_001/logs?start_date=2024-01-25&end_date=2024-01-31');
const data = await weeklyLogs.json();

// 按日期分组统计
const dailySpend = data.data.logs.reduce((daily, log) => {
  const date = log.time.split(' ')[0]; // 获取日期部分
  if (!daily[date]) daily[date] = 0;
  if (log.spend > 0) daily[date] += log.spend; // 只统计消费，不包括充值
  return daily;
}, {});

console.log('每日消费趋势：', dailySpend);
```

## 数据库变更

### 用户表扩展
```sql
-- 添加外部用户相关字段
ALTER TABLE users ADD COLUMN phone VARCHAR(20) DEFAULT '';
ALTER TABLE users ADD COLUMN wechat_openid VARCHAR(100) DEFAULT '';
ALTER TABLE users ADD COLUMN wechat_unionid VARCHAR(100) DEFAULT '';
ALTER TABLE users ADD COLUMN alipay_userid VARCHAR(100) DEFAULT '';
ALTER TABLE users ADD COLUMN external_user_id VARCHAR(100) DEFAULT '';
ALTER TABLE users ADD COLUMN login_type VARCHAR(20) DEFAULT 'email';
ALTER TABLE users ADD COLUMN is_external BOOLEAN DEFAULT false;
ALTER TABLE users ADD COLUMN external_data TEXT;

-- 创建索引
CREATE UNIQUE INDEX idx_users_external_user_id ON users(external_user_id);
```

## 错误处理

### 常见错误类型

| 状态码 | 错误信息 | 说明 |
|--------|----------|------|
| 400 | 参数错误 | 请求参数格式不正确或缺少必需字段 |
| 404 | 用户不存在 | 指定的外部用户ID不存在 |
| 500 | 用户名已存在，请使用其他用户名 | 用户名重复 |
| 500 | 邮箱已被使用，请使用其他邮箱 | 邮箱地址重复 |
| 500 | 推荐码已被使用，请使用其他推荐码 | 推荐码重复 |
| 500 | 外部用户ID已存在 | external_user_id重复 |

### 错误响应格式
```json
{
  "success": false,
  "message": "用户名已存在，请使用其他用户名",
  "error_detail": "Error 1062 (23000): Duplicate entry 'testuser' for key 'users.username'"
}
```

**说明**：
- `message`: 用户友好的错误信息
- `error_detail`: 详细的技术错误信息（开发环境）

## 渠道管理集成

### 模型可用性
- API 会实时反映管理界面的渠道启用/禁用状态
- 禁用渠道的模型会立即从 `balance_capacity` 中移除
- 启用渠道的模型会自动出现在用户统计中
- 测试模型（`test_model` 字段）会优先显示在列表首位

### 计费精度
- 支持小数模型倍率（如 deepseek-chat: 0.135）
- 使用四舍五入确保计费精度
- 完全兼容 New API 的复杂计费体系

## 推荐体系支持

### 推荐码功能
- `aff_code`: 可选字段，支持前端的推荐体系
- 默认为 NULL，避免数据库唯一索引冲突
- 支持创建和更新时设置推荐码
- 推荐码重复时返回明确的错误信息

### 使用示例
```json
{
  "external_user_id": "new_user_001",
  "username": "newuser",
  "aff_code": "INVITE_ABC123"
}
```

## 性能考虑

### 数据库优化
- `external_user_id` 字段有唯一索引，查询性能优异
- 支持并发用户创建和更新
- 渠道状态查询已优化，实时反映管理界面变更

### API 性能
- 所有外部用户 API 无需认证，减少了中间件开销
- balance_capacity 计算经过优化，支持实时计费展示
- 错误处理详细但不影响性能

## ❓ 常见问题 (FAQ)

### Q1: 为什么Token创建需要allocated_quota参数？
**A**: V1 API采用Token独立额度模式：
- 每个Token有自己的`RemainQuota`
- 创建Token时从用户余额池扣除并分配给Token
- Token消费时只扣减自己的额度，互不影响

**示例**：
```
用户充值 $100 → User.Quota = 50,000,000
创建Token1分配$30 → Token1.RemainQuota = 15,000,000
创建Token2分配$20 → Token2.RemainQuota = 10,000,000
用户剩余 $50 → User.Quota = 25,000,000
```

### Q2: 用户余额用完后Token还能用吗？
**A**: 能！Token余额独立于用户余额：
- Token创建时已经从用户余额扣除并分配
- Token消费时只扣减自己的`RemainQuota`
- 用户余额为0不影响已创建Token的使用

### Q3: 如何计算分配多少quota给Token？
**A**: 根据用户需求和模型价格估算：
```
deepseek-chat 示例：
- 输入：135 quota/1K tokens
- 输出：540 quota/1K tokens
- 10,000,000 quota ($20) ≈ 74,000次输入（1K tokens/次）

qwen-turbo 示例：
- 输入：857 quota/1K tokens
- 10,000,000 quota ($20) ≈ 11,600次输入（1K tokens/次）
```

### Q4: Token额度用完了怎么办？
**A**: 有两种方案：
1. **创建新Token**：用户如果有余额，可以创建新Token并分配额度
2. **无法续费**：现有Token额度用完后无法充值续费，只能创建新Token

### Q5: 能否修改Token的allocated_quota？
**A**: 不能。Token创建后：
- ❌ 不支持额度修改或追加
- ❌ 不支持额度回收到用户余额
- ✅ 只能查看剩余额度（remain_quota）
- ✅ 可以删除Token（但额度不退还）

### Q6: V1和V2 API有什么区别？
**A**: 核心区别在于计费模式：

| 维度 | V1 (本文档) | V2 平台API |
|------|-------------|-----------|
| Token额度 | 独立额度 | 无限额度 |
| 计费主体 | New API | 平台自己 |
| 适用场景 | 个人用户前端 | 下游平台集成 |
| Token生成 | New API | 平台自己 |

👉 查看 [V2平台API文档](external-user-api-v2.md)

### Q7: 充值记录为什么显示负数？
**A**: 这是设计上的特性：
- 消费记录：`spend > 0`（正数）
- 充值记录：`spend < 0`（负数）
- 便于前端区分类型和计算总花费

**示例**：
```json
{
  "type": "consume",
  "spend": 2.5    // 消费$2.5
},
{
  "type": "topup",
  "spend": -50    // 充值$50
}
```

### Q8: 如何处理余额不足的错误？
**A**: 创建Token时如果余额不足，API会返回明确错误：
```json
{
  "success": false,
  "message": "用户余额不足，当前余额: 5000000，请求分配: 10000000"
}
```

**解决方案**：
1. 提示用户充值
2. 减少Token分配额度
3. 删除不用的Token（但额度不退还）

### Q9: Token验证接口有缓存吗？
**A**: 是的，有Redis缓存：
- ⚠️ 刚删除的Token可能因缓存暂时仍显示有效
- ⏱️ 几分钟后缓存失效，Token会正确显示无效
- 💡 重要业务逻辑不要完全依赖验证接口

### Q10: 🆕 什么是Callback回调功能？
**A**: V1 API的可选扩展功能，适用于下游平台集成：

**工作原理**：
- Token创建时配置`callback_url`（可选）
- Token消费时自动异步POST通知到callback_url
- 下游平台接收通知后自己做统计汇总

**适用场景**：
- ✅ CEC等下游平台需要实时接收消费通知
- ✅ 平台需要按用户+Token维度统计（"Agent消费"）
- ✅ 平台需要自己做二次计费

**不适用场景**：
- ❌ 前端个人用户应用（直接查询消费记录即可）

**详细文档**：[callback-feature.md](callback-feature.md)

### Q11: 🆕 Callback回调如何保证安全？
**A**: 使用IP白名单保护（Nginx层配置）：

**推荐方案**：
```nginx
location /api/consume-notify {
    allow 1.2.3.4;       # CEC服务器IP
    allow 5.6.7.8/24;    # CEC服务器IP段
    deny all;
    proxy_pass http://backend;
}
```

**注意**：
- callback_secret字段已预留但当前不使用
- 未来如需HMAC签名验证，可升级实现

---

## 注意事项

1. **货币处理**：所有金额必须是美元，前端负责货币转换
2. **支付追踪**：`payment_id` 用于支付追踪和对账，请确保唯一性
3. **邮箱处理**：支持虚拟邮箱，用于微信/支付宝等无邮箱登录方式
4. **安全考虑**：建议通过 Nginx 配置 IP 白名单，确保只有授权的前端系统可以访问
5. **计费理解**：用户充值购买的是"购买力" quota，使用时按不同模型的真实价格消费
6. **推荐码**：可选字段，为 NULL 时不会产生唯一索引冲突
7. **模型显示**：只显示当前启用渠道的模型，测试模型优先显示
8. **实时性**：渠道启用/禁用会立即反映在 API 响应中
9. **Token 管理**：
   - Token列表接口返回脱敏密钥（前8位+后4位），前端可安全显示
   - 用户可以自由创建和删除自己的 Token
   - 删除 Token 后立即失效，无法恢复
   - Token 删除只能由该 Token 的所有者执行
   - Token额度用完后无法续费，只能创建新Token

## 开发工具

### 测试命令
详细的测试用例请参考：`docs/curl-testing-guide.md`

### 单元测试
```bash
# 运行所有外部用户API单元测试
go test ./controller -v -timeout 60s -run "Test.*ExternalUser"
```

---
*文档版本：v2.0*  
*最后更新：2025-01-31*