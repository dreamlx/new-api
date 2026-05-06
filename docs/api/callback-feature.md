# Token消费回调功能文档

> **📍 重要说明**：本功能是 **V1 个人用户API的可选扩展功能**，不是独立的第三种API。
>
> - **V1 API**: 个人用户模式（用户+Token，独立额度）
> - **V2 API**: 平台集成模式（无限额度Token）
> - **Callback功能**: V1 API的可选扩展，Token创建时可配置callback_url

**📖 文档导航**：[← 返回API总览](external-api-overview.md) | [V1个人用户API →](external-user-api.md)

---

## 功能概述

Token消费回调功能允许下游平台（如CEC）实时接收New API中Token的消费通知。每当Token成功消费时，New API会异步发送HTTP POST回调通知到配置的URL。

**设计原则**：
- ✅ **KISS**: 最小化改动，复用现有日志系统
- ✅ **YAGNI**: 只实现核心回调功能，不做复杂重试
- ✅ **异步非阻塞**: 回调失败不影响主请求流程

---

## 使用场景

### CEC平台集成示例

```
CEC平台需求：
  ↓
用户在CEC注册 → CEC为用户创建New API Token
  ↓
用户通过Token调用LLM → New API记录消费并回调CEC
  ↓
CEC接收回调 → 按用户+Token统计消费 → CEC自己计费
```

**关键特点**：
- New API只回调**单次LLM请求**的消费信息
- CEC自己做二次统计（按Token、按用户汇总）
- "Agent消费" = CEC对用户多个Token消费的汇总

---

## API使用说明

### 1. 创建支持回调的Token

**接口**: `POST /api/user/external/token`

**请求示例**:
```bash
curl -X POST "http://localhost:3000/api/user/external/token" \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "cec_user_123",
    "token_name": "CEC用户Token-001",
    "allocated_quota": 10000000,
    "expires_in_days": 365,

    "callback_url": "https://cec.example.com/api/consume-notify",
    "callback_enabled": true
  }'
```

**请求参数**:
| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `external_user_id` | string | ✅ | 外部用户ID |
| `token_name` | string | ✅ | Token名称 |
| `allocated_quota` | int | ✅ | 分配额度 |
| `expires_in_days` | int | ❌ | 过期天数（默认365） |
| `callback_url` | string | ❌ | 回调URL |
| `callback_enabled` | bool | ❌ | 是否启用回调（默认false） |
| `callback_secret` | string | ❌ | 签名密钥（保留字段，暂不使用，推荐IP白名单） |

**响应示例**:
```json
{
  "success": true,
  "message": "Token创建成功",
  "data": {
    "token_id": 132,
    "access_key": "sk-abc123def456789...",
    "token_name": "CEC用户Token-001",
    "expires_at": 1731449596,
    "remain_quota": 10000000,
    "callback_enabled": true,
    "callback_url_masked": "https://cec.example.com/***"
  }
}
```

---

### 2. 回调触发机制

**触发时机**: Token成功消费LLM请求后

**触发流程**:
```
LLM请求完成
  ↓
记录消费日志（model/log.go:RecordConsumeLog）
  ↓
检查token.callback_enabled
  ↓ (是)
异步goroutine发送HTTP POST
  ↓
CEC接收通知（https://cec.example.com/api/consume-notify）
```

---

### 3. 回调数据格式

**HTTP POST请求**:
```http
POST https://cec.example.com/api/consume-notify
Content-Type: application/json
User-Agent: New-API-Callback/1.0

{
  "event": "token_consumed",
  "timestamp": 1700123456,

  "external_user_id": "cec_user_123",
  "token_id": 132,
  "token_key": "sk-abc123def456789...",  // 完整Token密钥
  "token_name": "CEC用户Token-001",

  "model": "deepseek-chat",
  "prompt_tokens": 100,
  "completion_tokens": 50,
  "quota_consumed": 1350,
  "amount_usd": 0.0027,

  "log_id": 74393,
  "request_id": "log_74393"
}
```

**字段说明**:
| 字段 | 类型 | 说明 |
|------|------|------|
| `event` | string | 固定值："token_consumed" |
| `timestamp` | int64 | Unix时间戳 |
| `external_user_id` | string | CEC用户ID |
| `token_id` | int | Token ID |
| `token_key` | string | 完整Token密钥（便于CEC分组统计） |
| `token_name` | string | Token名称 |
| `model` | string | 模型名称（如deepseek-chat） |
| `prompt_tokens` | int | 输入tokens数量 |
| `completion_tokens` | int | 输出tokens数量 |
| `quota_consumed` | int | 消耗的quota |
| `amount_usd` | float64 | 消耗的美元金额 |
| `log_id` | int | 日志ID |
| `request_id` | string | 请求追踪ID |

---

## CEC端实现示例

### Python Flask实现

```python
from flask import Flask, request
from datetime import datetime

app = Flask(__name__)

@app.route('/api/consume-notify', methods=['POST'])
def consume_notify():
    # 1. 提取回调数据
    data = request.json
    user_id = data['external_user_id']
    token_key = data['token_key']
    amount = data['amount_usd']
    model = data['model']

    # 2. 保存消费记录到CEC数据库
    consumption = TokenConsumption(
        user_id=user_id,
        token_key=token_key,
        token_name=data['token_name'],
        timestamp=datetime.fromtimestamp(data['timestamp']),
        model=model,
        prompt_tokens=data['prompt_tokens'],
        completion_tokens=data['completion_tokens'],
        amount_usd=amount
    )
    db.session.add(consumption)
    db.session.commit()

    # 3. 返回成功响应
    return {'success': True}, 200

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=5000)
```

---

### CEC端统计示例

```python
from sqlalchemy import func

# 1. 用户总消费（"Agent消费"）
def get_user_total_consumption(user_id, start_date, end_date):
    """用户通过所有Token的总消费"""
    return db.session.query(
        func.sum(TokenConsumption.amount_usd)
    ).filter(
        TokenConsumption.user_id == user_id,
        TokenConsumption.timestamp.between(start_date, end_date)
    ).scalar()

# 2. 单个Token消费
def get_token_consumption(token_key, start_date, end_date):
    """单个Token的消费统计"""
    return db.session.query(
        func.count(TokenConsumption.id).label('request_count'),
        func.sum(TokenConsumption.amount_usd).label('total_cost')
    ).filter(
        TokenConsumption.token_key == token_key,
        TokenConsumption.timestamp.between(start_date, end_date)
    ).first()

# 3. 用户各Token消费明细（Agent级别统计）
def get_user_tokens_breakdown(user_id, start_date, end_date):
    """用户的各Token消费明细"""
    return db.session.query(
        TokenConsumption.token_key,
        TokenConsumption.token_name,
        func.count(TokenConsumption.id).label('request_count'),
        func.sum(TokenConsumption.amount_usd).label('total_cost')
    ).filter(
        TokenConsumption.user_id == user_id,
        TokenConsumption.timestamp.between(start_date, end_date)
    ).group_by(
        TokenConsumption.token_key,
        TokenConsumption.token_name
    ).all()

# 使用示例
user_total = get_user_total_consumption('cec_user_123', '2025-11-18', '2025-11-19')
print(f"用户总消费: ${user_total:.4f}")

token_stats = get_token_consumption('sk-abc123...', '2025-11-18', '2025-11-19')
print(f"Token消费: {token_stats.request_count}次请求, ${token_stats.total_cost:.4f}")

breakdown = get_user_tokens_breakdown('cec_user_123', '2025-11-18', '2025-11-19')
for item in breakdown:
    print(f"  {item.token_name}: {item.request_count}次, ${item.total_cost:.4f}")
```

---

## 安全性设计

### IP白名单（推荐）

**Nginx配置示例**:
```nginx
# 在Nginx层限制回调接口的访问来源
location /api/consume-notify {
    # 只允许New API服务器IP访问
    allow 1.2.3.4;      # New API服务器IP
    deny all;

    proxy_pass http://cec_backend;
}
```

**优势**:
- ✅ 简单有效
- ✅ 性能开销小
- ✅ 防止未授权访问
- ✅ 无需在代码层验证

**callback_secret字段**: 已保留但暂不使用，为未来扩展预留（如需HMAC签名验证）

---

## 错误处理

### 回调失败策略

**原则**: 回调失败不影响主流程（KISS）

**处理逻辑**:
```
回调失败
  ↓
记录日志到New API系统日志
  ↓
不重试，不阻塞
  ↓
CEC可通过补偿接口查询遗漏记录
```

**日志示例**:
```
2025-11-18 10:30:00 [INFO] callback success: tokenId=132, status=200
2025-11-18 10:31:00 [WARN] callback failed: tokenId=132, status=500
2025-11-18 10:32:00 [ERROR] callback request failed: tokenId=132, url=https://cec.example.com/api/notify, error=timeout
```

### 补偿机制（可选）

**CEC定期查询补偿**:
```bash
# CEC定期调用V1 API的消费记录查询接口
curl "http://new-api.com/api/user/external/cec_user_123/logs?start_time=1700000000&end_time=1700086400"
```

对比回调接收的记录和查询结果，补偿遗漏的消费记录。

---

## 性能影响

### 异步非阻塞设计

```go
// 主流程：记录日志
err := LOG_DB.Create(log).Error
if err != nil {
    return // 失败立即返回
}

// 异步回调：不阻塞主流程
if params.TokenId > 0 {
    gopool.Go(func() {
        SendConsumeCallback(userId, params.TokenId, log)
    })
}
```

**性能指标**:
- ✅ 主流程延迟：0ms（异步执行）
- ✅ 回调超时：3秒
- ✅ 失败影响：无（仅记录日志）

---

## 数据库迁移

**自动迁移**: GORM自动创建新字段

启动New API服务时，GORM会自动检测`Token`模型新增的字段并创建：
- `callback_url` - VARCHAR(500)
- `callback_enabled` - BOOLEAN, DEFAULT false
- `callback_secret` - VARCHAR(64)

**手动迁移**（可选）:
```sql
ALTER TABLE tokens ADD COLUMN callback_url VARCHAR(500) DEFAULT '';
ALTER TABLE tokens ADD COLUMN callback_enabled BOOLEAN DEFAULT false;
ALTER TABLE tokens ADD COLUMN callback_secret VARCHAR(64) DEFAULT '';

CREATE INDEX idx_tokens_callback_enabled ON tokens(callback_enabled);
```

---

## 测试指南

### 测试脚本

参见：`scripts/test-callback-feature.sh`

### 手动测试步骤

**1. 创建支持回调的Token**:
```bash
curl -X POST "http://localhost:3000/api/user/external/token" \
  -H "Content-Type: application/json" \
  -d '{
    "external_user_id": "test_user_001",
    "token_name": "测试Token",
    "allocated_quota": 10000000,
    "callback_url": "http://localhost:5000/api/consume-notify",
    "callback_enabled": true
  }'
```

**2. 启动CEC模拟服务**:
```python
# 运行上面的Flask示例
python cec_callback_server.py
```

**3. 使用Token调用LLM**:
```bash
curl -X POST "http://localhost:3000/v1/chat/completions" \
  -H "Authorization: Bearer sk-[刚创建的Token]" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'
```

**4. 验证CEC收到回调**:
检查CEC服务日志，应该看到回调数据。

---

## 常见问题

### Q1: Agent消费和LLM消费如何区分？

**A**: 不需要区分。

- New API只记录**单次LLM请求**的消费
- "Agent消费" = CEC对用户多个Token消费的汇总
- CEC收到回调后自己按`token_key`分组统计

### Q2: 回调失败怎么办？

**A**: 回调失败只记录日志，不影响主流程。

- 原因：KISS原则，不做复杂重试
- 补偿：CEC定期调用`GET /api/user/external/:id/logs`查询遗漏

### Q3: 回调是同步还是异步？

**A**: 异步（goroutine）。

- 不阻塞LLM请求主流程
- 超时3秒自动放弃

### Q4: 如何保证回调安全？

**A**: 使用IP白名单（推荐）。

- 在Nginx层配置allow/deny规则
- 只允许New API服务器IP访问回调接口
- 简单有效，性能开销小
- callback_secret字段已保留，未来可扩展HMAC签名验证

### Q5: 能否修改已创建Token的回调配置？

**A**: 当前不支持，只能重新创建Token。

未来可扩展`PATCH /api/user/external/token/:id`接口支持更新。

---

## 技术亮点

✅ **KISS原则**: 只扩展3个数据库字段，复用现有日志系统
✅ **YAGNI原则**: 不引入消息队列等重依赖
✅ **异步非阻塞**: goroutine发送，0延迟
✅ **失败友好**: 回调失败不影响服务，只记录日志
✅ **安全简单**: IP白名单机制，Nginx层防护
✅ **灵活配置**: Token级别控制，不是全局配置
✅ **完整Token密钥**: 便于CEC按Token分组统计

---

**最后更新**: 2025-11-18
**作者**: Claude Code (专家模式)
**版本**: v1.0
