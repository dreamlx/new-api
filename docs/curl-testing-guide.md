# 外部用户API接口测试指南

本文档提供完整的 curl 命令来测试外部用户集成API的所有功能。

## 环境准备

### 1. 启动开发环境
```bash
# 启动数据库服务
make dev-db

# 启动后端服务
make start-backend
```

服务启动后：
- API服务地址：`http://localhost:3000`
- MySQL：`localhost:3307` (用户: root, 密码: dev123456)
- Redis：`localhost:6379`

### 2. 运行单元测试
```bash
# 运行所有外部用户API单元测试
go test ./controller -v -timeout 60s -run "Test.*ExternalUser"

# 单独运行某个测试
go test ./controller -v -run "TestSyncExternalUser"
go test ./controller -v -run "TestTopupExternalUser"
go test ./controller -v -run "TestCreateExternalUserToken"
go test ./controller -v -run "TestGetExternalUserStats"
```

## 重要说明：Token额度机制

### Token额度工作原理

本系统中的API Token采用**智能额度同步机制**，确保充值后的额度能够立即在所有现有Token中生效。

#### 核心机制说明

1. **用户主账户额度**: 每个用户都有一个主账户额度（`user.quota`），这是用户的总可用额度。
2. **Token额度快照**: 创建Token时，系统会将用户当时的主账户额度复制到Token的`remain_quota`字段。
3. **智能同步**: 当使用Token进行API调用时，系统会自动检查并同步Token额度与用户主账户额度。

#### 同步触发条件

- 当Token的剩余额度**小于**用户主账户额度时
- 且Token不是无限额度类型时
- 系统会自动将用户主账户的额度同步到Token中

#### 实际使用流程

```mermaid
sequenceDiagram
    participant Client as 客户端
    participant API as API系统
    participant DB as 数据库

    Note over Client,DB: 标准集成流程
    Client->>API: 1. 创建/同步用户
    API->>DB: 用户额度初始为0
    Client->>API: 2. 创建Token
    API->>DB: Token额度=用户额度(0)
    Client->>API: 3. 用户充值
    API->>DB: 更新用户额度=500,000
    Note over API: Token额度仍为0，但系统会自动同步
    Client->>API: 4. 使用Token调用API
    API->>API: 检测到Token额度<用户额度
    API->>DB: 自动同步Token额度=500,000
    API->>Client: API调用成功
```

### 最佳实践建议

#### ✅ 推荐的集成顺序

1. **创建/同步用户** → 获得用户ID
2. **为用户充值** → 确保账户有足够额度
3. **创建Token** → 获得访问凭证
4. **使用Token调用API** → 系统自动处理额度同步

#### ⚠️ 重要提示

- **无需重新创建Token**: 充值后，现有的Token会自动获得新的额度，无需重新生成
- **即时生效**: 额度同步在首次API调用时自动完成，无需等待
- **向后兼容**: 此机制完全向后兼容，不影响现有的集成代码

#### 🔧 故障排除

如果遇到"余额不足"错误：

1. **检查用户主账户额度**:
   ```bash
   # 通过用户统计API检查
   curl -X GET "http://localhost:3000/api/user/external/stats?external_user_id=your_user_id"
   ```

2. **确认充值是否成功**:
   - 检查充值API的返回结果
   - 确认`current_balance`字段显示正确金额

3. **重试API调用**:
   - 系统会在下次API调用时自动同步额度
   - 通常第一次调用会触发同步，后续调用正常工作

## API接口测试

### 1. 用户同步API

#### 1.1 创建新用户
```bash
curl -X POST http://localhost:3000/api/user/external/sync \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "test_user_001",
    "username": "testuser",
    "display_name": "测试用户",
    "email": "test@example.com",
    "phone": "13800138000",
    "login_type": "email"
  }'
```

**期望响应：**
```json
{
  "success": true,
  "message": "用户创建成功",
  "data": {
    "user_id": 1,
    "external_user_id": "test_user_001",
    "username": "testuser",
    "quota": 0
  }
}
```

#### 1.2 更新现有用户
```bash
curl -X POST http://localhost:3000/api/user/external/sync \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "test_user_001",
    "username": "updated_testuser",
    "display_name": "更新的测试用户",
    "email": "updated@example.com",
    "phone": "13900139000",
    "login_type": "sms"
  }'
```

**期望响应：**
```json
{
  "success": true,
  "message": "用户信息更新成功",
  "data": {
    "user_id": 1,
    "external_user_id": "test_user_001",
    "username": "updated_testuser",
    "quota": 0
  }
}
```

#### 1.3 测试参数验证 - 缺少必需字段
```bash
curl -X POST http://localhost:3000/api/user/external/sync \
  -H "Content-Type: application/json" \
  -d '{
    "username": "testuser",
    "email": "test@example.com"
  }'
```

**期望响应：**
```json
{
  "success": false,
  "message": "请求参数错误: Key: 'SyncExternalUserRequest.ExternalUserId' Error:Field validation for 'ExternalUserId' failed on the 'required' tag"
}
```

#### 1.4 测试参数验证 - 无效登录类型
```bash
curl -X POST http://localhost:3000/api/user/external/sync \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "test_user_002",
    "username": "testuser2",
    "email": "test2@example.com",
    "login_type": "invalid_type"
  }'
```

**期望响应：**
```json
{
  "success": false,
  "message": "请求参数错误: Key: 'SyncExternalUserRequest.LoginType' Error:Field validation for 'LoginType' failed on the 'oneof' tag"
}
```

### 2. 用户充值API

#### 2.1 成功充值
```bash
curl -X POST http://localhost:3000/api/user/external/topup \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "test_user_001",
    "amount_usd": 10.0,
    "payment_id": "stripe_payment_123456"
  }'
```

**期望响应：**
```json
{
  "success": true,
  "message": "充值成功",
  "data": {
    "user_id": 1,
    "external_user_id": "test_user_001",
    "amount_usd": 10.0,
    "quota_added": 5000000,
    "current_quota": 5000000,
    "payment_id": "stripe_payment_123456"
  }
}
```

#### 2.2 测试用户不存在
```bash
curl -X POST http://localhost:3000/api/user/external/topup \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "nonexistent_user",
    "amount_usd": 10.0,
    "payment_id": "stripe_payment_789"
  }'
```

**期望响应：**
```json
{
  "success": false,
  "message": "用户不存在"
}
```

#### 2.3 测试无效金额
```bash
curl -X POST http://localhost:3000/api/user/external/topup \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "test_user_001",
    "amount_usd": -5.0,
    "payment_id": "stripe_payment_invalid"
  }'
```

**期望响应：**
```json
{
  "success": false,
  "message": "请求参数错误: Key: 'ExternalUserTopUpRequest.AmountUSD' Error:Field validation for 'AmountUSD' failed on the 'min' tag"
}
```

#### 2.4 测试缺少支付ID
```bash
curl -X POST http://localhost:3000/api/user/external/topup \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "test_user_001",
    "amount_usd": 5.0
  }'
```

**期望响应：**
```json
{
  "success": false,
  "message": "请求参数错误: Key: 'ExternalUserTopUpRequest.PaymentId' Error:Field validation for 'PaymentId' failed on the 'required' tag"
}
```

### 3. Token管理API

#### 3.1 成功创建Token
```bash
curl -X POST http://localhost:3000/api/user/external/token \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "test_user_001",
    "token_name": "My API Token",
    "expires_in_days": 365
  }'
```

**期望响应：**
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

#### 3.2 使用默认过期时间创建Token
```bash
curl -X POST http://localhost:3000/api/user/external/token \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "test_user_001",
    "token_name": "Default Expiry Token"
  }'
```

**期望响应：**
```json
{
  "success": true,
  "message": "Token创建成功",
  "data": {
    "token_id": 2,
    "access_key": "sk-yyyyyyyyyyyyyyyyyyyy",
    "token_name": "Default Expiry Token",
    "expires_at": 1767195600,
    "remain_quota": 5000000
  }
}
```

#### 3.3 测试用户不存在
```bash
curl -X POST http://localhost:3000/api/user/external/token \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "nonexistent_user",
    "token_name": "Test Token"
  }'
```

**期望响应：**
```json
{
  "success": false,
  "message": "用户不存在"
}
```

#### 3.4 测试缺少Token名称
```bash
curl -X POST http://localhost:3000/api/user/external/token \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "test_user_001",
    "expires_in_days": 30
  }'
```

**期望响应：**
```json
{
  "success": false,
  "message": "请求参数错误: Key: 'ExternalUserTokenRequest.TokenName' Error:Field validation for 'TokenName' failed on the 'required' tag"
}
```

#### 3.5 删除Token
```bash
curl -X DELETE http://localhost:3000/api/user/external/token \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "test_user_001",
    "token_id": 1
  }'
```

**期望响应：**
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

#### 3.6 删除不存在的Token
```bash
curl -X DELETE http://localhost:3000/api/user/external/token \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "test_user_001",
    "token_id": 999999
  }'
```

**期望响应：**
```json
{
  "success": false,
  "message": "Token不存在或无权删除"
}
```

#### 3.7 删除Token时用户不存在
```bash
curl -X DELETE http://localhost:3000/api/user/external/token \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "nonexistent_user",
    "token_id": 1
  }'
```

**期望响应：**
```json
{
  "success": false,
  "message": "用户不存在"
}
```

### 4. 用户统计API

#### 4.1 获取用户统计信息
```bash
curl -X GET http://localhost:3000/api/user/external/test_user_001/stats
```

**期望响应：**
```json
{
  "success": true,
  "data": {
    "user_info": {
      "external_user_id": "test_user_001",
      "username": "updated_testuser",
      "display_name": "更新的测试用户",
      "current_quota": 5000000,
      "current_balance": 10.0,
      "used_quota": 0,
      "total_requests": 0,
      "balance_capacity": 10.0
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

#### 4.2 测试用户不存在
```bash
curl -X GET http://localhost:3000/api/user/external/nonexistent_user/stats
```

**期望响应：**
```json
{
  "success": false,
  "message": "用户不存在"
}
```

### 5. 消费记录查询API

#### 5.1 查询所有消费记录
```bash
curl -X GET "http://localhost:3000/api/user/external/test_user_001/logs"
```

**期望响应：**
```json
{
  "success": true,
  "data": {
    "logs": [
      {
        "time": "2024-01-30 15:30:25",
        "username": "testuser",
        "tokens": 32,
        "type": "consume",
        "model": "qwen-turbo",
        "spend": 0.0017144
      },
      {
        "time": "2024-01-30 10:00:00",  
        "username": "testuser",
        "tokens": 0,
        "type": "topup",
        "model": "",
        "spend": 10.0
      }
    ],
    "pagination": {
      "page": 1,
      "page_size": 20,
      "total": 2,
      "total_page": 1
    },
    "summary": {
      "total_tokens": 32,
      "total_spend": 10.0017144
    }
  }
}
```

#### 5.2 按日期范围查询
```bash
curl -X GET "http://localhost:3000/api/user/external/test_user_001/logs?start_date=2024-01-01&end_date=2024-01-31"
```

#### 5.3 按模型筛选查询
```bash
curl -X GET "http://localhost:3000/api/user/external/test_user_001/logs?model_name=qwen"
```

#### 5.4 分页查询
```bash
curl -X GET "http://localhost:3000/api/user/external/test_user_001/logs?page=1&page_size=10"
```

#### 5.5 组合查询条件
```bash
curl -X GET "http://localhost:3000/api/user/external/test_user_001/logs?start_date=2024-01-15&model_name=qwen&page=1&page_size=5"
```

#### 5.6 测试用户不存在
```bash
curl -X GET "http://localhost:3000/api/user/external/nonexistent_user/logs"
```

**期望响应：**
```json
{
  "success": false,
  "message": "用户不存在"
}
```

#### 5.7 测试无效日期格式
```bash
curl -X GET "http://localhost:3000/api/user/external/test_user_001/logs?start_date=invalid-date"
```

**说明：**
- 无效日期会被忽略，按所有记录查询
- `start_date` 和 `end_date` 必须是 `YYYY-MM-DD` 格式
- `model_name` 支持模糊匹配（使用 LIKE 查询）
- `page_size` 最大限制为100

### 6. 完整流程测试

以下是一个完整的用户生命周期测试流程：

```bash
#!/bin/bash
# 完整流程测试脚本

echo "=== 1. 创建新用户 ==="
curl -X POST http://localhost:3000/api/user/external/sync \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "flow_test_user",
    "username": "flowtest",
    "display_name": "流程测试用户",
    "email": "flowtest@example.com",
    "login_type": "email"
  }'

echo -e "\n\n=== 2. 用户充值 ==="
curl -X POST http://localhost:3000/api/user/external/topup \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "flow_test_user",
    "amount_usd": 20.0,
    "payment_id": "flow_test_payment_001"
  }'

echo -e "\n\n=== 3. 创建API Token ==="
curl -X POST http://localhost:3000/api/user/external/token \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "flow_test_user",
    "token_name": "Flow Test Token",
    "expires_in_days": 90
  }'

echo -e "\n\n=== 4. 查看用户统计 ==="
curl -X GET http://localhost:3000/api/user/external/flow_test_user/stats

echo -e "\n\n=== 5. 更新用户信息 ==="
curl -X POST http://localhost:3000/api/user/external/sync \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "flow_test_user",
    "username": "flowtest_updated",
    "display_name": "更新后的流程测试用户",
    "email": "flowtest_updated@example.com",
    "phone": "13800138888",
    "login_type": "sms"
  }'

echo -e "\n\n=== 6. 再次查看用户统计 ==="
curl -X GET http://localhost:3000/api/user/external/flow_test_user/stats

echo -e "\n\n=== 7. 查询消费记录 ==="
curl -X GET "http://localhost:3000/api/user/external/flow_test_user/logs?page_size=10"

echo -e "\n\n=== 8. 按日期查询消费记录 ==="
curl -X GET "http://localhost:3000/api/user/external/flow_test_user/logs?start_date=2024-01-01&end_date=2024-01-31"

echo -e "\n\n=== 9. 删除Token ==="
curl -X DELETE http://localhost:3000/api/user/external/token \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "flow_test_user",
    "token_id": 1
  }'

echo -e "\n\n=== 10. 验证Token已删除 ==="
curl -X GET http://localhost:3000/api/user/external/flow_test_user/stats
```

## 计费验证

### Quota 计算公式
- **1 USD = 500,000 quota**
- **计费单位**: common.QuotaPerUnit = 500,000
- **示例**: 充值 $10.00 → 增加 5,000,000 quota
- **余额显示**: quota ÷ 500,000 = 美元余额

### 验证计费正确性
```bash
# 充值 $25.50
curl -X POST http://localhost:3000/api/user/external/topup \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "test_user_001",
    "amount_usd": 25.50,
    "payment_id": "billing_test_001"
  }'

# 预期结果：
# - quota_added: 12,750,000 (25.50 * 500,000)
# - current_balance: 25.50
```

## LLM API 调用测试

在创建用户、充值、生成Token后，可以测试实际的LLM模型调用功能。

### 1. 测试可用模型
```bash
# 先查看用户可用的模型和余额
curl -X GET http://localhost:3000/api/user/external/test_user_001/stats | jq '.data.user_info.balance_capacity'
```

### 2. Chat Completions API 测试

#### 2.1 测试 Qwen Turbo (推荐)
```bash
curl http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-YOUR_TOKEN_HERE" \
  -d '{
    "model": "qwen-turbo",
    "messages": [
      {
        "role": "user",
        "content": "你好！"
      }
    ]
  }'
```

**期望响应：**
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

#### 2.2 测试系统角色和多轮对话
```bash
curl http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-YOUR_TOKEN_HERE" \
  -d '{
    "model": "qwen-turbo",
    "messages": [
      {
        "role": "system",
        "content": "你是一个有帮助的AI助手，专门回答技术问题。"
      },
      {
        "role": "user",
        "content": "什么是RESTful API？"
      }
    ],
    "max_tokens": 200,
    "temperature": 0.7
  }'
```

#### 2.3 测试 DeepSeek Chat (如果可用)
```bash
curl http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-YOUR_TOKEN_HERE" \
  -d '{
    "model": "deepseek-chat",
    "messages": [
      {
        "role": "user",
        "content": "解释一下机器学习的概念"
      }
    ]
  }'
```

#### 2.4 测试其他可用模型
```bash
# 测试 Qwen Plus (更强大的模型)
curl http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-YOUR_TOKEN_HERE" \
  -d '{
    "model": "qwen-plus",
    "messages": [
      {
        "role": "user",
        "content": "请写一个Python函数来计算斐波那契数列"
      }
    ]
  }'
```

### 3. 错误场景测试

#### 3.1 无效Token
```bash
curl http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-invalid-token" \
  -d '{
    "model": "qwen-turbo",
    "messages": [{"role": "user", "content": "test"}]
  }'
```

#### 3.2 不支持的模型
```bash
curl http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-YOUR_TOKEN_HERE" \
  -d '{
    "model": "nonexistent-model",
    "messages": [{"role": "user", "content": "test"}]
  }'
```

#### 3.3 余额不足
```bash
# 先创建一个低余额用户进行测试
# (需要先同步用户但不充值)
curl http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-LOW_BALANCE_TOKEN" \
  -d '{
    "model": "qwen-turbo",
    "messages": [{"role": "user", "content": "test"}]
  }'
```

### 4. 使用统计验证

#### 4.1 调用前查看统计
```bash
curl -X GET http://localhost:3000/api/user/external/test_user_001/stats | jq '.data.user_info | {current_quota, used_quota, total_requests}'
```

#### 4.2 进行LLM调用
```bash
curl http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-YOUR_TOKEN_HERE" \
  -d '{
    "model": "qwen-turbo",
    "messages": [{"role": "user", "content": "这是一个测试消息"}]
  }'
```

#### 4.3 调用后再次查看统计
```bash
curl -X GET http://localhost:3000/api/user/external/test_user_001/stats | jq '.data.user_info | {current_quota, used_quota, total_requests}'
```

**应该能观察到：**
- `used_quota` 增加（根据token使用量）
- `total_requests` 增加1
- `current_quota` 相应减少

#### 4.4 查询消费记录验证
```bash
# 查询最新的消费记录
curl -X GET "http://localhost:3000/api/user/external/test_user_001/logs?page_size=5" | jq '.data.logs[0]'
```

**应该能看到：**
- 最新一条记录为 `consume` 类型
- `model` 字段显示调用的模型名称
- `tokens` 字段显示消费的token数量
- `spend` 字段显示消费金额

### 5. 完整集成测试流程

以下是一个完整的集成测试，验证从用户创建到消费记录查询的整个流程：

```bash
#!/bin/bash
# 完整集成测试脚本

USER_ID="integration_test_$(date +%s)"
TOKEN_KEY=""

echo "=== 完整集成测试开始 ==="
echo "测试用户ID: $USER_ID"

# 1. 创建用户
echo "1. 创建用户..."
curl -s -X POST http://localhost:3000/api/user/external/sync \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$USER_ID\",
    \"username\": \"integration_user\",
    \"display_name\": \"集成测试用户\",
    \"email\": \"integration@test.com\"
  }" | jq '.success'

# 2. 充值
echo "2. 用户充值..."
curl -s -X POST http://localhost:3000/api/user/external/topup \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$USER_ID\",
    \"amount_usd\": 5.0,
    \"payment_id\": \"integration_test_payment\"
  }" | jq '.data.current_balance'

# 3. 创建Token
echo "3. 创建API Token..."
TOKEN_RESPONSE=$(curl -s -X POST http://localhost:3000/api/user/external/token \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$USER_ID\",
    \"token_name\": \"Integration Test Token\"
  }")

TOKEN_KEY=$(echo $TOKEN_RESPONSE | jq -r '.data.access_key')
echo "Token创建成功: ${TOKEN_KEY:0:20}..."

# 4. 调用LLM API
echo "4. 调用LLM API..."
curl -s -X POST http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN_KEY" \
  -d '{
    "model": "qwen-turbo",
    "messages": [{"role": "user", "content": "Hello, this is an integration test!"}]
  }' | jq '.usage.total_tokens'

# 5. 等待日志记录
echo "5. 等待日志记录写入..."
sleep 2

# 6. 查询消费记录
echo "6. 查询消费记录..."
curl -s -X GET "http://localhost:3000/api/user/external/$USER_ID/logs?page_size=3" | jq '{
  total_records: .data.pagination.total,
  latest_log: .data.logs[0] | {time, type, model, tokens, spend},
  total_spend: .data.summary.total_spend
}'

echo "=== 集成测试完成 ==="
```

**运行集成测试：**
```bash
chmod +x integration_test.sh
./integration_test.sh
```

## 错误处理测试

### 常见错误场景
1. **参数验证错误** - 400状态码
2. **用户不存在** - 404状态码
3. **服务器内部错误** - 500状态码

### 日志查看
```bash
# 查看实时日志
tail -f logs/oneapi-*.log

# 查看数据库连接
docker logs mysql-dev

# 查看Redis连接
docker logs redis-dev
```

## 测试清单

### 基础功能测试 ✅
- [ ] 用户同步API - 创建新用户
- [ ] 用户同步API - 更新现有用户
- [ ] 用户充值API - 成功充值
- [ ] Token创建API - 成功创建
- [ ] Token删除API - 成功删除
- [ ] 用户统计API - 获取统计信息（包含完整Token）
- [ ] 消费记录API - 查询所有记录
- [ ] 消费记录API - 按日期筛选
- [ ] 消费记录API - 按模型筛选
- [ ] 消费记录API - 分页查询

### LLM API 集成测试 ✅
- [ ] Chat Completions - qwen-turbo 模型
- [ ] Chat Completions - deepseek-chat 模型（如果可用）
- [ ] Chat Completions - qwen-plus 模型
- [ ] 系统角色和多轮对话
- [ ] 模型参数配置（max_tokens, temperature等）

### 边界情况测试 ✅
- [ ] 参数验证 - 缺少必需字段
- [ ] 参数验证 - 无效枚举值
- [ ] 用户不存在 - 所有相关API
- [ ] 金额验证 - 负数/零值
- [ ] Token名称验证 - 空值
- [ ] Token删除 - 不存在的Token
- [ ] Token删除 - 无权限删除他人Token

### LLM API 错误测试 ✅
- [ ] 无效Token - 认证失败
- [ ] 不支持的模型 - 模型不存在
- [ ] 余额不足 - quota耗尽
- [ ] 请求格式错误 - 无效JSON/参数

### 业务逻辑测试 ✅
- [ ] Quota计算准确性
- [ ] 用户信息更新完整性
- [ ] Token创建和权限
- [ ] Token删除和权限验证
- [ ] 统计数据一致性
- [ ] Token列表显示完整key
- [ ] LLM调用后用量统计更新
- [ ] Balance capacity 模型显示
- [ ] 渠道禁用实时生效

### 性能测试 (可选)
- [ ] 并发用户创建
- [ ] 批量充值处理
- [ ] 高频Token创建
- [ ] 统计查询性能
- [ ] 并发LLM API调用
- [ ] 大量token消耗场景

---

**注意事项：**
1. 所有API都需要 `Content-Type: application/json` 头
2. 测试前确保数据库和Redis服务正常运行
3. 建议先运行单元测试验证基础功能
4. 生产环境测试时请使用测试数据，避免影响真实用户
5. 每次测试后建议清理测试数据保持环境干净

**联系方式：**
如有问题请联系开发团队或查看项目文档：`docs/external-user-api.md`