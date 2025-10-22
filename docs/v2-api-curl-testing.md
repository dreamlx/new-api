# V2 外部系统集成 API Curl 测试指南

## 概述

V2 API 采用授权计费网关模式，提供极简的2个核心接口，支持下游平台自主管理用户和API密钥。

## 核心特性

- ✅ **极简接口**: 仅2个核心接口
- ✅ **零数据库修改**: 完全基于现有表结构
- ✅ **下游自主权**: 平台自管理用户和密钥
- ✅ **授权计费网关**: New API专注计费和模型调用
- ✅ **幂等性设计**: 支持重复授权同一密钥

## API 接口

### 1. 密钥授权接口

**接口**: `POST /api/v2/external/tokens/authorize`

**功能**: 授权一个新的API密钥，使其可以在New API系统中进行模型调用和计费

#### 基本测试

```bash
# 创建新密钥授权
curl -X POST "http://localhost:3000/api/v2/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d '{
    "platform_id": "testplatform",
    "token_key": "sk-platform-abc123def456",
    "initial_quota": 1000000,
    "metadata": {
      "platform_user_id": "user_123",
      "user_type": "premium",
      "created_by": "admin"
    }
  }'
```

**预期响应**:
```json
{
  "success": true,
  "message": "密钥授权成功",
  "data": {
    "token_key": "sk-platform-abc123def456",
    "current_quota": 1000000,
    "quota_usd": 2.0,
    "status": "authorized",
    "created_at": "2025-10-22T10:30:00Z",
    "proxy_user_id": 1001
  }
}
```

#### 更新已有密钥额度

```bash
# 重复授权同一密钥会更新额度
curl -X POST "http://localhost:3000/api/v2/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d '{
    "platform_id": "testplatform",
    "token_key": "sk-platform-abc123def456",
    "initial_quota": 2000000,
    "metadata": {
      "platform_user_id": "user_123",
      "user_type": "premium_updated"
    }
  }'
```

**预期响应**:
```json
{
  "success": true,
  "message": "密钥已存在，额度已更新",
  "data": {
    "token_key": "sk-platform-abc123def456",
    "previous_quota": 1000000,
    "current_quota": 2000000,
    "quota_added": 1000000,
    "status": "updated",
    "updated_at": "2025-10-22T10:35:00Z",
    "proxy_user_id": 1001
  }
}
```

#### 参数说明

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `platform_id` | string | 是 | 平台唯一标识符，对应内部用户`username="platform_{platform_id}"` |
| `token_key` | string | 是 | 下游平台生成的API密钥，建议以`sk-`开头 |
| `initial_quota` | int | 是 | 初始额度，单位为quota。`$1 USD = 500,000 quota` |
| `metadata` | object | 否 | 平台自定义的元数据，用于内部追踪 |

#### 错误测试

```bash
# 无效platform_id格式
curl -X POST "http://localhost:3000/api/v2/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d '{
    "platform_id": "invalid@platform",
    "token_key": "sk-test",
    "initial_quota": 100000
  }'

# 无效token_key格式
curl -X POST "http://localhost:3000/api/v2/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d '{
    "platform_id": "validplatform",
    "token_key": "invalid-token",
    "initial_quota": 100000
  }'

# 密钥冲突（不同平台相同密钥）
curl -X POST "http://localhost:3000/api/v2/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d '{
    "platform_id": "anotherplatform",
    "token_key": "sk-platform-abc123def456",
    "initial_quota": 500000
  }'
```

### 2. 平台消费流水查询接口

**接口**: `GET /api/v2/external/platforms/{platform_id}/logs`

**功能**: 获取指定平台下所有密钥的消费日志，用于平台对账和审计

#### 基本测试

```bash
# 查询今日消费日志
curl -X GET "http://localhost:3000/api/v2/external/platforms/testplatform/logs?start_date=2025-10-22&end_date=2025-10-22&page=1&page_size=20"
```

**预期响应**:
```json
{
  "success": true,
  "message": "查询成功",
  "data": {
    "platform_id": "testplatform",
    "date_range": {
      "start_date": "2025-10-22",
      "end_date": "2025-10-22"
    },
    "logs": [
      {
        "log_id": "12345",
        "time": "2025-10-22T15:30:00Z",
        "token_key": "sk-platform-abc123def456",
        "model_name": "claude-3-5-sonnet-20241022",
        "prompt_tokens": 150,
        "completion_tokens": 80,
        "total_tokens": 230,
        "quota_cost": 1150
      }
    ],
    "pagination": {
      "page": 1,
      "page_size": 20,
      "total_items": 1,
      "total_pages": 1,
      "has_next": false,
      "has_prev": false
    },
    "summary": {
      "total_requests": 1,
      "total_prompt_tokens": 150,
      "total_completion_tokens": 80,
      "total_tokens": 230,
      "total_quota_consumed": 1150,
      "unique_tokens": 1
    }
  }
}
```

#### 查询参数说明

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `platform_id` | string | 是 | 路径参数，平台标识符 |
| `start_date` | string | 是 | 开始日期，格式：YYYY-MM-DD |
| `end_date` | string | 是 | 结束日期，格式：YYYY-MM-DD |
| `page` | int | 否 | 页码，默认为1 |
| `page_size` | int | 否 | 每页大小，默认20，最大100 |

#### 错误测试

```bash
# 平台不存在
curl -X GET "http://localhost:3000/api/v2/external/platforms/nonexistent/logs?start_date=2025-10-22&end_date=2025-10-22"

# 缺失日期参数
curl -X GET "http://localhost:3000/api/v2/external/platforms/testplatform/logs"

# 无效日期格式
curl -X GET "http://localhost:3000/api/v2/external/platforms/testplatform/logs?start_date=invalid-date&end_date=2025-13-32"
```

## 完整工作流示例

### 步骤1: 下游平台授权密钥

```bash
# 平台"myapp"为用户"user_456"生成密钥
curl -X POST "http://localhost:3000/api/v2/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d '{
    "platform_id": "myapp",
    "token_key": "sk-myapp-user456-20251022",
    "initial_quota": 5000000,
    "metadata": {
      "platform_user_id": "user_456",
      "user_type": "enterprise",
      "department": "engineering"
    }
  }'
```

### 步骤2: 终端用户使用密钥调用模型

```bash
# 用户使用授权的密钥调用标准OpenAI兼容接口
curl -X POST "http://localhost:3000/v1/chat/completions" \
  -H "Authorization: Bearer sk-myapp-user456-20251022" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "claude-3-5-sonnet-20241022",
    "messages": [
      {"role": "user", "content": "Hello, how are you?"}
    ],
    "max_tokens": 100
  }'
```

### 步骤3: 下游平台查询消费流水

```bash
# 平台查询今日消费流水进行对账
curl -X GET "http://localhost:3000/api/v2/external/platforms/myapp/logs?start_date=2025-10-22&end_date=2025-10-22&page_size=50"
```

## 错误码说明

| 错误码 | HTTP状态码 | 说明 |
|--------|------------|------|
| `INVALID_PARAMETER` | 400 | 请求参数格式错误或缺少必填参数 |
| `PLATFORM_NOT_FOUND` | 404 | 指定的平台不存在 |
| `TOKEN_EXISTS` | 409 | 密钥已存在但属于不同平台 |
| `QUOTA_EXCEEDED` | 400 | 授权的额度超过限制 |
| `INTERNAL_ERROR` | 500 | 服务器内部错误 |

## 快速测试脚本

运行完整的集成测试：

```bash
# 确保服务运行
make dev-db
make start-backend

# 运行测试脚本
./scripts/test-v2-api.sh
```

## 注意事项

1. **额度单位**: `$1 USD = 500,000 quota`，所有额度相关字段均使用quota单位
2. **幂等性**: `authorize`接口是幂等的，多次调用相同`token_key`将更新额度而非创建重复记录
3. **日志唯一性**: 每条消费记录都有唯一的`log_id`（使用logs.id），可用于数据同步和去重
4. **时区**: 所有时间字段均使用UTC时间，格式为ISO 8601
5. **平台隔离**: 不同`platform_id`的数据完全隔离，确保数据安全
6. **零数据库修改**: V2接口基于现有表结构实现，无需任何数据库调整

---

*文档版本：v1.0*
*最后更新：2025-10-22*