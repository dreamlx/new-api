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
- **模型计费**：不同模型有不同的 `modelPrice`，实际消费 = `modelPrice * QuotaPerUnit * groupRatio`

## API 接口

### 安全认证
- **IP 白名单**：通过 Nginx 配置限制访问
- **管理员权限**：前端平台使用管理员 Token 调用 API

### 1. 用户同步接口

#### 创建或更新外部用户
```http
POST /api/user/external/sync
Content-Type: application/json
Authorization: Bearer {admin_token}
```

**请求参数**:
```json
{
  "external_user_id": "string, required, 外部用户唯一标识",
  "username": "string, required, 用户名",
  "display_name": "string, optional, 显示名称", 
  "email": "string, optional, 邮箱地址（可为虚拟邮箱）"
}
```

**响应示例**:
```json
{
  "success": true,
  "message": "用户创建成功",
  "data": {
    "user_id": 123
  }
}
```

**说明**:
- `external_user_id` 是前端用户系统的用户ID，作为唯一映射标识
- `email` 可以是虚拟邮箱，如 `"wechat_user_123@virtual.local"`
- 如果用户已存在，则更新用户信息

### 2. 用户充值接口

#### 为外部用户充值
```http
POST /api/user/external/topup
Content-Type: application/json
Authorization: Bearer {admin_token}
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
    "amount_usd": 68.49,
    "quota_added": 34245000,
    "current_quota": 34245000,
    "current_balance": 68.49,
    "payment_id": "stripe_pi_1234567890"
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

### 3. 模型列表接口

#### 获取可用模型和费率
```http
GET /api/user/external/models
Authorization: Bearer {admin_token}
```

**响应示例**:
```json
{
  "success": true,
  "data": {
    "models": {
      "gpt-4": {
        "name": "gpt-4",
        "price_per_1k": 0.03,
        "quota_per_1k": 15000,
        "description": "GPT-4 模型",
        "billing_type": "tokens"
      },
      "dall-e-3": {
        "name": "dall-e-3", 
        "price_per_1k": 0.04,
        "quota_per_1k": 20000,
        "description": "DALL-E 3 图像生成",
        "billing_type": "calls"
      }
    },
    "quota_per_unit": 500000,
    "currency": "USD"
  }
}
```

### 4. 创建 Access Key 接口

#### 为外部用户创建 Token
```http
POST /api/user/external/token
Content-Type: application/json
Authorization: Bearer {admin_token}
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
    "token_id": 456,
    "access_key": "sk-1234567890abcdef",
    "token_name": "My API Key",
    "expires_at": 1735689600,
    "remain_quota": 34245000
  }
}
```

### 5. 用户统计接口

#### 获取用户使用统计
```http
GET /api/user/external/{external_user_id}/stats
Authorization: Bearer {admin_token}
```

**响应示例**:
```json
{
  "success": true,
  "data": {
    "user_info": {
      "external_user_id": "amos_wechat_123",
      "username": "amos_chen",
      "display_name": "Amos Chen",
      "current_quota": 34245000,
      "current_balance": 68.49,
      "used_quota": 0,
      "total_requests": 0,
      "balance_capacity": {
        "gpt-4": {
          "tokens_1k": 2283,
          "price": 0.03
        },
        "gpt-3.5-turbo": {
          "tokens_1k": 68490,
          "price": 0.001
        }
      }
    },
    "tokens": [
      {
        "id": 456,
        "name": "My API Key",
        "key": "sk-1234...abcdef",
        "status": 1,
        "expired_time": 1735689600
      }
    ],
    "recent_logs": [],
    "model_usage": {}
  }
}
```

## 完整用户流程示例

### 用户 Amos 的使用流程

```javascript
const newApi = new NewAPIClient('https://api.example.com', 'admin_token_xxx');

// 1. 用户微信登录后同步到 New API
await newApi.syncUser({
  external_user_id: 'amos_wechat_123',
  username: 'amos_chen',
  display_name: 'Amos Chen',
  email: 'wechat_amos_123@virtual.local'
});

// 2. 用户充值 500元人民币（Stripe转换为$68.49）
await newApi.topupUser({
  external_user_id: 'amos_wechat_123',
  amount_usd: 68.49,  // Stripe转换后的美元金额
  payment_id: 'stripe_pi_1234567890'
});

// 3. 获取可用模型列表
const models = await newApi.getModels();

// 4. 创建 Access Key
const token = await newApi.createToken({
  external_user_id: 'amos_wechat_123',
  token_name: 'My Chat App',
  expires_in_days: 365
});

// 5. 用户使用 Access Key 调用 LLM API
const response = await fetch('https://api.example.com/v1/chat/completions', {
  method: 'POST',
  headers: {
    'Authorization': `Bearer ${token.access_key}`,
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({
    model: 'gpt-4',
    messages: [{ role: 'user', content: 'Hello!' }]
  })
});

// 6. 查看使用统计
const stats = await newApi.getUserStats('amos_wechat_123');
```

## 数据库变更

### 用户表扩展
```sql
-- 添加外部用户ID字段
ALTER TABLE users ADD COLUMN external_user_id VARCHAR(100) UNIQUE;

-- 创建索引
CREATE UNIQUE INDEX idx_users_external_user_id ON users(external_user_id);
```

## 错误代码

| 状态码 | 错误信息 | 说明 |
|--------|----------|------|
| 400 | 参数错误 | 请求参数格式不正确 |
| 404 | 用户不存在 | 指定的外部用户ID不存在 |
| 500 | 内部服务器错误 | 服务器处理异常 |

## 注意事项

1. **货币处理**：所有金额必须是美元，前端负责货币转换
2. **支付追踪**：`payment_id` 用于支付追踪和对账，请确保唯一性
3. **邮箱处理**：支持虚拟邮箱，用于微信/支付宝等无邮箱登录方式
4. **安全考虑**：通过 Nginx 配置 IP 白名单，确保只有授权的前端系统可以访问
5. **计费理解**：用户充值购买的是"购买力" quota，使用时按不同模型的真实价格消费

---
*文档版本：v1.0*  
*最后更新：2025-01-29*