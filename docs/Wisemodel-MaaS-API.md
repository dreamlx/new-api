# Wisemodel-MaaS 平台接入 API 文档

## 认证方式

在请求 Header 中添加参数：

```http
Authorization: Bearer <Token>
```

示例：

```
Authorization: Bearer ********************
```

- **接口协议**：HTTP  
- **请求格式**：JSON  
- **响应格式**：JSON  

---

## 一、用户绑定接口

**功能**：  
根据手机号查询用户是否存在，如果存在则返回绑定状态；若不存在则进行绑定，并写入 `Wisemodel-key`，返回绑定状态。

**接口描述**：用于同步用户信息到 API 提供商。

**接口路径**：`/api/user/bind`  
**HTTP Method**：`POST`

### 请求参数（Body）

| 参数名 | 类型 | 必需 | 说明 |
|--------|------|------|------|
| phone | string | 是 | 手机号 |
| wisemodel_key | string | 是 | Wisemodel API Key |
| username | string | 是 | 用户名 |

**示例：**
```json
{
  "phone": "13800138222",
  "wisemodel_key": "wisemodel-V1yourwisemodelkay12342",
  "username": "test_user202"
}
```

### 响应参数

| 参数名 | 类型 | 必需 | 说明 |
|--------|------|------|------|
| message | string | 是 | 提示信息 |
| success | boolean | 是 | 操作是否成功 |

**示例：**
```json
{"message":"绑定成功","success":true}
```

---

## 二、Wisemodel-key 删除接口

**功能**：删除用户的 `Wisemodel-key`。

**接口描述**：用于删除 API 提供商存储的 Wisemodel key。

**接口路径**：`/api/user/delete_wisemodel_key`  
**HTTP Method**：`POST`

### 请求参数

| 参数名 | 类型 | 必需 | 说明 |
|--------|------|------|------|
| phone | string | 是 | 手机号 |

**示例：**
```json
{
  "phone": "13800138222"
}
```

### 响应示例

```json
{"message":"删除成功","success":true}
```

---

## 三、Wisemodel-key 更新接口

**功能**：更新用户的 `Wisemodel-key`。

**接口描述**：用于更新 API 提供商存储的 Wisemodel key。

**接口路径**：`/api/user/update_wisemodel_key`  
**HTTP Method**：`POST`

### 请求参数

| 参数名 | 类型 | 必需 | 说明 |
|--------|------|------|------|
| phone | string | 是 | 手机号 |
| new_key | string | 是 | 新的 Wisemodel-key |

**示例：**
```json
{
  "phone": "13800138222",
  "new_key": "wisemodel-testapikey2"
}
```

### 响应示例

```json
{"message":"更新成功","success":true}
```

---

## 四、创建订单接口

**功能**：记录订单信息并绑定用户与模型或资源包，给用户账号充值。

**接口描述**：用于创建资源包购买订单。

**接口路径**：`/api/orders/record`  
**HTTP Method**：`POST`

### 请求参数（Body）

| 参数名 | 类型 | 必需 | 说明 |
|--------|------|------|------|
| order_id | string | 是 | 订单 ID |
| package_count | integer | 是 | 购买的资源包数量 |
| packages | array | 是 | 资源包列表 |

**包内字段（package）：**

| 参数名 | 类型 | 说明 |
|--------|------|------|
| id | string | 资源包 ID |
| points | integer | 若无 token，传积分 |
| tokens | integer | 若无积分，传 token |
| amount | integer | 资源包价格，免费包为 0 |
| phone | string | 用户手机号 |
| is_free | boolean | 是否免费 |
| valid_until | string | 有效期 |
| created_at | string | 创建时间 |

**示例（积分模式）：**
```json
{
  "order_id": "ORD202403210021",
  "package_count": 2,
  "packages": [
    {
      "id": "PKG001",
      "points": 100,
      "amount": 10.00,
      "phone": "13801138221",
      "is_free": false,
      "valid_until": "2026-12-31T23:59:59Z",
      "created_at": "2024-03-20T10:00:00Z"
    },
    {
      "id": "PKG002",
      "points": 120,
      "amount": 0,
      "phone": "13801138221",
      "is_free": true,
      "valid_until": "2024-12-31T23:59:59Z",
      "created_at": "2024-03-20T10:00:00Z"
    }
  ]
}
```

**示例（Token 模式）：**
```json
{
  "order_id": "ORD202403210021",
  "package_count": 2,
  "packages": [
    {
      "id": "PKG001",
      "tokens": 100,
      "amount": 10.00,
      "phone": "13801138221",
      "is_free": false,
      "valid_until": "2026-12-31T23:59:59Z",
      "created_at": "2024-03-20T10:00:00Z"
    },
    {
      "id": "PKG002",
      "tokens": 120,
      "amount": 0,
      "phone": "13801138221",
      "is_free": true,
      "valid_until": "2024-03-20T10:00:00Z",
      "created_at": "2024-03-20T10:00:00Z"
    }
  ]
}
```

### 响应示例

```json
{"message":"创建成功","success":true}
```

---

## 五、手机号更新接口

**功能**：更新绑定的手机号。

**接口描述**：用于更新 API 提供商存储的手机号。

**接口路径**：`/api/user/update_phone`  
**HTTP Method**：`POST`

### 请求参数

| 参数名 | 类型 | 必需 | 说明 |
|--------|------|------|------|
| old_phone | string | 是 | 原手机号 |
| new_phone | string | 是 | 新手机号 |

**示例：**
```json
{
  "old_phone": "13800138222",
  "new_phone": "13800138223"
}
```

### 响应示例

```json
{"message":"更新成功","success":true}
```

---

## 六、资源包使用情况接口

**功能**：查询用户的 Token 或积分使用情况。

**接口描述**：用于提供用户调用 API 产生的 token 数或积分数。

**接口路径**：`/api/user/package_usage`  
**HTTP Method**：`POST`

### 请求参数

| 参数名 | 类型 | 必需 | 说明 |
|--------|------|------|------|
| phone | string | 是 | 手机号 |

**示例：**
```json
{
  "phone": "13800138222"
}
```

### 响应参数

| 参数名 | 类型 | 说明 |
|--------|------|------|
| code | integer | 状态码 |
| data | array | 资源包使用数据 |
| msg | string | 消息 |

**data 对象字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| package_id | string | 资源包 ID |
| points / tokens | integer | 总积分 / Token |
| remain_points / remain_tokens | integer | 剩余积分 / Token |
| amount / amount_tokens | integer | 已用积分 / Token |
| available_models | array[string] | 当前资源包包含的模型 |
| details | array | 模型使用详情 |

**details 对象字段：**

| 字段 | 类型 | 说明 |
|------|------|------|
| model_name | string | 模型名称 |
| used_amount / used_amount_tokens | integer | 使用积分 / Token 数量 |

**积分示例：**
```json
{
  "code": 200,
  "data": [
    {
      "package_id": "PKG001",
      "points": 10000,
      "remain_points": 9982,
      "amount": 10,
      "available_models": ["BAAI/bge-large-zh-v1.5", "DeepSeek-V3", "DeepSeek-R1", "BAAI/bge-reranker-large"],
      "details": [
        {"model_name": "DeepSeek-V3", "used_amount": 12},
        {"model_name": "DeepSeek-R1", "used_amount": 6}
      ]
    }
  ],
  "msg": "success"
}
```

**Token 示例：**
```json
{
  "code": 200,
  "data": [
    {
      "package_id": "PKG001",
      "tokens": 10000,
      "remain_tokens": 9982,
      "amount_tokens": 10,
      "available_models": ["BAAI/bge-large-zh-v1.5", "DeepSeek-V3", "DeepSeek-R1", "BAAI/bge-reranker-large"],
      "details": [
        {"model_name": "DeepSeek-V3", "used_amount_tokens": 12},
        {"model_name": "DeepSeek-R1", "used_amount_tokens": 6}
      ]
    }
  ],
  "msg": "success"
}
```

---

## 七、取消授权接口

**功能**：  
用户取消授权时，需删除绑定信息（手机号、Wisemodel-key 等）。  
若存在付费订单（通过 `is_free` 字段判断），则返回提示并拒绝删除。

**接口描述**：用于删除 API 提供商存储的用户信息。

**接口路径**：`/api/user/delete_wisemodel_user`  
**HTTP Method**：`POST`

### 请求参数

| 参数名 | 类型 | 必需 | 说明 |
|--------|------|------|------|
| phone | string | 是 | 手机号 |

**示例：**
```json
{
  "phone": "13800138222"
}
```

### 响应示例

```json
{"message":"删除成功","success":true}
```

---
