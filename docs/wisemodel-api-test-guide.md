# Wisemodel MaaS API 测试指南

## 目录
- [环境准备](#环境准备)
- [接口测试](#接口测试)
  - [1. 用户绑定](#1-用户绑定)
  - [2. 删除Wisemodel Key](#2-删除wisemodel-key)
  - [3. 更新Wisemodel Key](#3-更新wisemodel-key)
  - [4. 更新手机号](#4-更新手机号)
  - [5. 创建订单（资源包充值）](#5-创建订单资源包充值)
  - [6. 查询资源包使用情况](#6-查询资源包使用情况)
  - [7. 删除Wisemodel用户](#7-删除wisemodel用户)
- [完整测试流程](#完整测试流程)

---

## 环境准备

### 1. 启动数据库
```bash
cd /Users/dreamlinx/Dropbox/Projects/NetBeansProjects/new-api
make dev-db
```

### 2. 配置环境变量
在 `.env.dev` 文件中添加：
```bash
WISEMODEL_API_TOKEN=test_wisemodel_token_12345
```

### 3. 启动后端服务
```bash
make start
```

### 4. 验证服务
```bash
curl http://localhost:3000/api/status
```

---

## 接口测试

**通用Header**：
```
Authorization: Bearer test_wisemodel_token_12345
Content-Type: application/json
```

### 1. 用户绑定

**接口**：`POST /api/wisemodel/user/bind`

**功能**：绑定用户，不存在则创建新用户

**测试用例 1.1 - 新用户绑定**：
```bash
curl -X POST http://localhost:3000/api/wisemodel/user/bind \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "13800138001",
    "wisemodel_key": "wm_key_test_001",
    "username": "测试用户001"
  }'
```

**预期响应**：
```json
{
  "message": "绑定成功",
  "success": true
}
```

**验证点**：
- ✅ 返回 success: true
- ✅ 数据库中创建新用户记录
- ✅ wisemodel_key 正确保存

**测试用例 1.2 - 已存在用户更新Key**：
```bash
# 再次调用相同手机号，不同key
curl -X POST http://localhost:3000/api/wisemodel/user/bind \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "13800138001",
    "wisemodel_key": "wm_key_test_001_updated",
    "username": "测试用户001"
  }'
```

**预期**：
- ✅ 更新现有用户的 wisemodel_key
- ✅ 不创建重复用户

---

### 2. 删除Wisemodel Key

**接口**：`POST /api/wisemodel/user/delete_wisemodel_key`

**功能**：清空用户的 wisemodel_key 字段

**测试用例 2.1 - 正常删除**：
```bash
curl -X POST http://localhost:3000/api/wisemodel/user/delete_wisemodel_key \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "13800138001"
  }'
```

**预期响应**：
```json
{
  "message": "删除成功",
  "success": true
}
```

**测试用例 2.2 - 用户不存在**：
```bash
curl -X POST http://localhost:3000/api/wisemodel/user/delete_wisemodel_key \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "99999999999"
  }'
```

**预期响应**：
```json
{
  "message": "用户不存在",
  "success": false
}
```

---

### 3. 更新Wisemodel Key

**接口**：`POST /api/wisemodel/user/update_wisemodel_key`

**功能**：更新用户的 wisemodel_key

**测试用例 3.1 - 正常更新**：
```bash
curl -X POST http://localhost:3000/api/wisemodel/user/update_wisemodel_key \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "13800138001",
    "new_key": "wm_key_new_002"
  }'
```

**预期响应**：
```json
{
  "message": "更新成功",
  "success": true
}
```

---

### 4. 更新手机号

**接口**：`POST /api/wisemodel/user/update_phone`

**功能**：修改用户手机号

**测试用例 4.1 - 正常更新**：
```bash
curl -X POST http://localhost:3000/api/wisemodel/user/update_phone \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "old_phone": "13800138001",
    "new_phone": "13800138002"
  }'
```

**预期响应**：
```json
{
  "message": "更新成功",
  "success": true
}
```

**测试用例 4.2 - 新手机号已被使用**：
```bash
# 先创建第二个用户
curl -X POST http://localhost:3000/api/wisemodel/user/bind \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "13800138003",
    "wisemodel_key": "wm_key_test_003",
    "username": "测试用户003"
  }'

# 尝试将第一个用户的手机号改为已存在的手机号
curl -X POST http://localhost:3000/api/wisemodel/user/update_phone \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "old_phone": "13800138002",
    "new_phone": "13800138003"
  }'
```

**预期响应**：
```json
{
  "message": "新手机号已被使用",
  "success": false
}
```

---

### 5. 创建订单（资源包充值）

**接口**：`POST /api/wisemodel/orders/record`

**功能**：处理资源包购买，转换为 quota 充值

**测试用例 5.1 - 积分模式资源包**：
```bash
curl -X POST http://localhost:3000/api/wisemodel/orders/record \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "order_id": "ORDER_TEST_001",
    "package_count": 1,
    "packages": [
      {
        "id": "PKG001",
        "points": 10,
        "tokens": 0,
        "amount": 10.00,
        "phone": "13800138002",
        "is_free": false,
        "valid_until": "2025-12-31T23:59:59Z",
        "created_at": "2025-01-21T10:00:00Z"
      }
    ]
  }'
```

**预期响应**：
```json
{
  "message": "创建成功",
  "success": true
}
```

**验证点**：
- ✅ 用户 quota 增加：10 points × 500,000 = 5,000,000
- ✅ 创建 wisemodel_packages 记录
- ✅ 创建充值日志（LogTypeTopup）

**测试用例 5.2 - Token模式资源包**：
```bash
curl -X POST http://localhost:3000/api/wisemodel/orders/record \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "order_id": "ORDER_TEST_002",
    "package_count": 1,
    "packages": [
      {
        "id": "PKG002",
        "points": 0,
        "tokens": 20,
        "amount": 20.00,
        "phone": "13800138002",
        "is_free": false,
        "valid_until": "2025-12-31T23:59:59Z",
        "created_at": "2025-01-21T10:05:00Z"
      }
    ]
  }'
```

**验证点**：
- ✅ 用户 quota 增加：20 tokens × 500,000 = 10,000,000

**测试用例 5.3 - 免费资源包**：
```bash
curl -X POST http://localhost:3000/api/wisemodel/orders/record \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "order_id": "ORDER_TEST_003",
    "package_count": 1,
    "packages": [
      {
        "id": "PKG003",
        "points": 5,
        "tokens": 0,
        "amount": 0.00,
        "phone": "13800138002",
        "is_free": true,
        "valid_until": "2025-12-31T23:59:59Z",
        "created_at": "2025-01-21T10:10:00Z"
      }
    ]
  }'
```

**验证点**：
- ✅ is_free = true
- ✅ amount = 0
- ✅ 日志显示"免费资源包"

**测试用例 5.4 - 多资源包订单**：
```bash
curl -X POST http://localhost:3000/api/wisemodel/orders/record \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "order_id": "ORDER_TEST_004",
    "package_count": 2,
    "packages": [
      {
        "id": "PKG004",
        "points": 15,
        "tokens": 0,
        "amount": 15.00,
        "phone": "13800138002",
        "is_free": false,
        "valid_until": "2025-12-31T23:59:59Z",
        "created_at": "2025-01-21T10:15:00Z"
      },
      {
        "id": "PKG005",
        "points": 25,
        "tokens": 0,
        "amount": 25.00,
        "phone": "13800138002",
        "is_free": false,
        "valid_until": "2025-12-31T23:59:59Z",
        "created_at": "2025-01-21T10:15:00Z"
      }
    ]
  }'
```

**验证点**：
- ✅ package_count = 2 与实际数组长度一致
- ✅ 两个资源包都成功处理

**测试用例 5.5 - 错误处理：package_count不匹配**：
```bash
curl -X POST http://localhost:3000/api/wisemodel/orders/record \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "order_id": "ORDER_TEST_005",
    "package_count": 3,
    "packages": [
      {
        "id": "PKG006",
        "points": 10,
        "tokens": 0,
        "amount": 10.00,
        "phone": "13800138002",
        "is_free": false,
        "valid_until": "2025-12-31T23:59:59Z",
        "created_at": "2025-01-21T10:20:00Z"
      }
    ]
  }'
```

**预期响应**：
```json
{
  "message": "package_count(3)与实际packages数量(1)不一致",
  "success": false
}
```

---

### 6. 查询资源包使用情况

**接口**：`POST /api/wisemodel/user/package_usage`

**功能**：查询用户所有资源包的使用统计

**测试用例 6.1 - 正常查询**：
```bash
curl -X POST http://localhost:3000/api/wisemodel/user/package_usage \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "13800138002"
  }'
```

**预期响应示例**：
```json
{
  "code": 200,
  "data": [
    {
      "package_id": "PKG001",
      "available_models": ["DeepSeek-V3", "DeepSeek-R1"],
      "points": 10,
      "remain_points": 10,
      "amount": 0,
      "details": []
    },
    {
      "package_id": "PKG002",
      "available_models": ["BAAI/bge-large-zh-v1.5", "BAAI/bge-reranker-large"],
      "tokens": 20,
      "remain_tokens": 20,
      "amount_tokens": 0,
      "details": []
    }
  ],
  "msg": "success"
}
```

**验证点**：
- ✅ 返回所有资源包
- ✅ 积分模式显示 points/remain_points
- ✅ Token模式显示 tokens/remain_tokens
- ✅ available_models 正确显示（根据 PackageModels 映射）

---

### 7. 删除Wisemodel用户

**接口**：`POST /api/wisemodel/user/delete_wisemodel_user`

**功能**：取消授权，清空 wisemodel_key，删除所有资源包（仅限无付费订单用户）

**测试用例 7.1 - 创建无付费订单的用户**：
```bash
# 先创建用户和免费资源包
curl -X POST http://localhost:3000/api/wisemodel/user/bind \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "13800138010",
    "wisemodel_key": "wm_key_test_010",
    "username": "测试用户010"
  }'

curl -X POST http://localhost:3000/api/wisemodel/orders/record \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "order_id": "ORDER_FREE_001",
    "package_count": 1,
    "packages": [
      {
        "id": "PKG_FREE_001",
        "points": 5,
        "tokens": 0,
        "amount": 0.00,
        "phone": "13800138010",
        "is_free": true,
        "valid_until": "2025-12-31T23:59:59Z",
        "created_at": "2025-01-21T11:00:00Z"
      }
    ]
  }'

# 删除用户
curl -X POST http://localhost:3000/api/wisemodel/user/delete_wisemodel_user \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "13800138010"
  }'
```

**预期响应**：
```json
{
  "message": "删除成功",
  "success": true
}
```

**验证点**：
- ✅ wisemodel_key 被清空
- ✅ 所有资源包记录被删除

**测试用例 7.2 - 有付费订单的用户（应拒绝）**：
```bash
curl -X POST http://localhost:3000/api/wisemodel/user/delete_wisemodel_user \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "13800138002"
  }'
```

**预期响应**：
```json
{
  "message": "存在付费订单，无法删除",
  "success": false
}
```

---

## 完整测试流程

### 场景1：新用户完整流程

```bash
#!/bin/bash
BASE_URL="http://localhost:3000"
TOKEN="test_wisemodel_token_12345"
PHONE="13900139000"

echo "=== 1. 用户绑定 ==="
curl -X POST $BASE_URL/api/wisemodel/user/bind \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"phone\": \"$PHONE\",
    \"wisemodel_key\": \"wm_key_flow_001\",
    \"username\": \"流程测试用户\"
  }"

echo -e "\n\n=== 2. 购买资源包 ==="
curl -X POST $BASE_URL/api/wisemodel/orders/record \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"order_id\": \"FLOW_ORDER_001\",
    \"package_count\": 1,
    \"packages\": [
      {
        \"id\": \"PKG_FLOW_001\",
        \"points\": 50,
        \"tokens\": 0,
        \"amount\": 50.00,
        \"phone\": \"$PHONE\",
        \"is_free\": false,
        \"valid_until\": \"2025-12-31T23:59:59Z\",
        \"created_at\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\"
      }
    ]
  }"

echo -e "\n\n=== 3. 查询使用情况 ==="
curl -X POST $BASE_URL/api/wisemodel/user/package_usage \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"phone\": \"$PHONE\"
  }"

echo -e "\n\n=== 4. 更新手机号 ==="
NEW_PHONE="13900139001"
curl -X POST $BASE_URL/api/wisemodel/user/update_phone \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"old_phone\": \"$PHONE\",
    \"new_phone\": \"$NEW_PHONE\"
  }"

echo -e "\n\n=== 5. 更新Wisemodel Key ==="
curl -X POST $BASE_URL/api/wisemodel/user/update_wisemodel_key \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"phone\": \"$NEW_PHONE\",
    \"new_key\": \"wm_key_flow_002\"
  }"

echo -e "\n\n=== 测试完成 ==="
```

### 场景2：错误处理测试

```bash
#!/bin/bash
BASE_URL="http://localhost:3000"
TOKEN="test_wisemodel_token_12345"

echo "=== 1. 无效Token测试 ==="
curl -X POST $BASE_URL/api/wisemodel/user/bind \
  -H "Authorization: Bearer invalid_token" \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "13800138000",
    "wisemodel_key": "test",
    "username": "test"
  }'

echo -e "\n\n=== 2. 缺少必填字段 ==="
curl -X POST $BASE_URL/api/wisemodel/user/bind \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "13800138000"
  }'

echo -e "\n\n=== 3. 用户不存在 ==="
curl -X POST $BASE_URL/api/wisemodel/user/delete_wisemodel_key \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "phone": "99999999999"
  }'

echo -e "\n\n=== 4. 资源包参数错误 ==="
curl -X POST $BASE_URL/api/wisemodel/orders/record \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "order_id": "ERROR_TEST",
    "package_count": 1,
    "packages": [
      {
        "id": "PKG_ERROR",
        "points": 0,
        "tokens": 0,
        "amount": 10.00,
        "phone": "13800138002",
        "is_free": false,
        "valid_until": "2025-12-31T23:59:59Z",
        "created_at": "2025-01-21T12:00:00Z"
      }
    ]
  }'

echo -e "\n\n=== 错误测试完成 ==="
```

---

## 数据库验证

### 查看创建的用户
```sql
SELECT id, phone, username, wisemodel_key, quota, created_at
FROM users
WHERE phone LIKE '138%' OR phone LIKE '139%'
ORDER BY created_at DESC;
```

### 查看资源包记录
```sql
SELECT id, user_id, package_id, order_id,
       original_points, original_tokens, quota_granted,
       amount, is_free, created_at
FROM wisemodel_packages
ORDER BY created_at DESC;
```

### 查看充值日志
```sql
SELECT id, user_id, type, content, quota, created_at
FROM logs
WHERE type = 2  -- LogTypeTopup
ORDER BY created_at DESC
LIMIT 20;
```

---

## 预期结果检查清单

### ✅ 功能完整性
- [ ] 所有7个接口均可正常调用
- [ ] Bearer Token 认证生效
- [ ] 用户绑定和更新正常
- [ ] 资源包创建和quota转换正确
- [ ] 使用情况查询正确
- [ ] 删除操作有正确的权限检查

### ✅ 数据一致性
- [ ] quota 转换公式正确：1 point/token = 500,000 quota
- [ ] 资源包记录与用户quota同步
- [ ] 积分/Token模式正确区分
- [ ] 可用模型列表正确映射

### ✅ 错误处理
- [ ] 无效Token返回401
- [ ] 参数缺失返回400
- [ ] 用户不存在返回404
- [ ] 业务逻辑错误返回正确错误信息

### ✅ 日志记录
- [ ] 充值操作创建日志
- [ ] 日志内容包含必要信息
- [ ] 付费/免费区分正确

---

## 故障排查

### 问题1：401 Unauthorized
**原因**：
- Token 未配置或配置错误
- Authorization header 格式错误

**解决**：
```bash
# 检查环境变量
echo $WISEMODEL_API_TOKEN

# 检查 .env.dev 文件
cat .env.dev | grep WISEMODEL

# 确保格式正确
curl -H "Authorization: Bearer your_token" ...
```

### 问题2：用户不存在
**原因**：
- 手机号错误
- 用户未先绑定

**解决**：
```bash
# 先调用绑定接口
curl -X POST .../user/bind ...
```

### 问题3：资源包创建失败
**原因**：
- package_count 与实际数量不一致
- points 和 tokens 都为0
- valid_until 格式错误

**解决**：
```json
{
  "package_count": 1,  // 必须等于 packages 数组长度
  "packages": [
    {
      "points": 10,  // points 或 tokens 必须有一个 > 0
      "valid_until": "2025-12-31T23:59:59Z"  // 使用 RFC3339 格式
    }
  ]
}
```

---

## 性能测试

### 并发测试（可选）
```bash
# 使用 Apache Bench 测试
ab -n 100 -c 10 \
  -H "Authorization: Bearer test_wisemodel_token_12345" \
  -H "Content-Type: application/json" \
  -p payload.json \
  http://localhost:3000/api/wisemodel/user/package_usage
```

### 压力测试建议
- 单接口QPS测试
- 批量订单处理测试
- 大量用户查询测试

---

## 总结

本测试指南覆盖：
- ✅ 7个API接口的完整测试用例
- ✅ 正常流程和错误处理
- ✅ 数据库验证方法
- ✅ 故障排查指南
- ✅ 完整测试流程脚本

**下一步**：
1. 运行所有测试用例
2. 验证数据库数据正确性
3. 与Wisemodel平台进行联调
4. 生产环境部署

---
*测试指南版本：v1.0*
*最后更新：2025-01-21*
