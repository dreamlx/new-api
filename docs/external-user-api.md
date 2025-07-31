# 外部用户系统集成 API 文档

## 项目概述

本文档描述了 New API 的外部用户系统集成方案，允许前端平台通过 API 与 New API 进行用户数据同步、充值管理和 Access Key 管理。

## 设计架构

### 核心理念
- **前端用户系统**：支持微信、支付宝、短信、邮箱等多种登录方式
- **New API 后端**：作为 LLM 网关和计费系统
- **映射机制**：通过 `external_user_id` 建立前端用户与 New API 用户的关联

### 计费策略
- **货币统一**：前端收款任意货币 → 支付网关转换 → 后端只接收美元
- **汇率处理**：完全由前端网站和支付网关负责，New API 不处理汇率转换
- **计费逻辑**：$1 USD = 500,000 quota（使用 `common.QuotaPerUnit`）
- **模型计费**：基于 New API 的复杂计费公式：
  ```
  消耗quota = 分组倍率 × 模型倍率 × (输入tokens + 输出tokens × 补全倍率)
  ```

## API 接口

### 安全认证
- **无需认证**：外部用户 API 已移除认证限制，供前端系统直接调用
- **IP 白名单**：建议通过 Nginx 配置限制访问（可选）

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

### 3. 创建 Access Key 接口

#### 为外部用户创建 Token
```http
POST /api/user/external/token
Content-Type: application/json
```

**请求参数**:
```json
{
  "external_user_id": "string, required, 外部用户ID",
  "token_name": "string, required, Token名称",
  "expires_in_days": "number, optional, 有效期天数，默认365"
}
```

**响应示例**:
```json
{
  "success": true,
  "message": "Token创建成功",
  "data": {
    "token_id": 1,
    "access_key": "sk-xxxxxxxxxxxxxxxxxxxx",
    "token_name": "My API Token",
    "expires_at": 1767195600,
    "remain_quota": 5000000
  }
}
```

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
        "key": "sk-xxxx...xxxx",
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

## 注意事项

1. **货币处理**：所有金额必须是美元，前端负责货币转换
2. **支付追踪**：`payment_id` 用于支付追踪和对账，请确保唯一性
3. **邮箱处理**：支持虚拟邮箱，用于微信/支付宝等无邮箱登录方式
4. **安全考虑**：建议通过 Nginx 配置 IP 白名单，确保只有授权的前端系统可以访问
5. **计费理解**：用户充值购买的是"购买力" quota，使用时按不同模型的真实价格消费
6. **推荐码**：可选字段，为 NULL 时不会产生唯一索引冲突
7. **模型显示**：只显示当前启用渠道的模型，测试模型优先显示
8. **实时性**：渠道启用/禁用会立即反映在 API 响应中

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