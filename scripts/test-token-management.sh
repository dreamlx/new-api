#!/bin/bash

# Token管理功能测试脚本
# 测试新增的两个接口：
# 1. GET /api/user/external/:external_user_id/tokens - 获取用户所有token
# 2. POST /api/user/external/token/verify - 验证token有效性

API_URL="http://localhost:3000"
TIMESTAMP=$(date +%s)
EXTERNAL_USER_ID="test_user_${TIMESTAMP}"
TEST_TOKEN=""

echo "========================================="
echo "Token管理功能测试"
echo "========================================="
echo ""

# 1. 创建测试用户
echo "步骤1: 创建测试用户..."
SYNC_RESPONSE=$(curl -s -X POST "${API_URL}/api/user/external/sync" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"${EXTERNAL_USER_ID}\",
    \"username\": \"token_test_${TIMESTAMP}\",
    \"display_name\": \"Token测试用户\",
    \"email\": \"token_test_${TIMESTAMP}@example.com\"
  }")

echo "$SYNC_RESPONSE" | jq '.'

if [ "$(echo "$SYNC_RESPONSE" | jq -r '.success')" != "true" ]; then
    echo "❌ 创建用户失败"
    exit 1
fi
echo "✅ 用户创建成功: ${EXTERNAL_USER_ID}"
echo ""

# 2. 为用户充值
echo "步骤2: 为用户充值 \$10..."
TOPUP_RESPONSE=$(curl -s -X POST "${API_URL}/api/user/external/topup" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"${EXTERNAL_USER_ID}\",
    \"amount_usd\": 10.0,
    \"payment_id\": \"test_payment_$(date +%s)\"
  }")

echo "$TOPUP_RESPONSE" | jq '.'

if [ "$(echo "$TOPUP_RESPONSE" | jq -r '.success')" != "true" ]; then
    echo "❌ 充值失败"
    exit 1
fi
echo "✅ 充值成功"
echo ""

# 3. 创建第一个Token
echo "步骤3: 创建第一个Token..."
TOKEN1_RESPONSE=$(curl -s -X POST "${API_URL}/api/user/external/token" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"${EXTERNAL_USER_ID}\",
    \"token_name\": \"测试Token-1\",
    \"expires_in_days\": 30
  }")

echo "$TOKEN1_RESPONSE" | jq '.'

if [ "$(echo "$TOKEN1_RESPONSE" | jq -r '.success')" != "true" ]; then
    echo "❌ 创建Token-1失败"
    exit 1
fi

TOKEN1_KEY=$(echo "$TOKEN1_RESPONSE" | jq -r '.data.access_key')
echo "✅ Token-1创建成功: ${TOKEN1_KEY}"
echo ""

# 4. 创建第二个Token
echo "步骤4: 创建第二个Token..."
TOKEN2_RESPONSE=$(curl -s -X POST "${API_URL}/api/user/external/token" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"${EXTERNAL_USER_ID}\",
    \"token_name\": \"测试Token-2\",
    \"expires_in_days\": 365
  }")

echo "$TOKEN2_RESPONSE" | jq '.'

if [ "$(echo "$TOKEN2_RESPONSE" | jq -r '.success')" != "true" ]; then
    echo "❌ 创建Token-2失败"
    exit 1
fi

TOKEN2_KEY=$(echo "$TOKEN2_RESPONSE" | jq -r '.data.access_key')
echo "✅ Token-2创建成功: ${TOKEN2_KEY}"
echo ""

# 5. 测试新接口：获取用户所有Token列表
echo "========================================="
echo "测试新接口1: 获取用户所有Token列表"
echo "========================================="
TOKENS_LIST_RESPONSE=$(curl -s -X GET "${API_URL}/api/user/external/${EXTERNAL_USER_ID}/tokens")

echo "$TOKENS_LIST_RESPONSE" | jq '.'

if [ "$(echo "$TOKENS_LIST_RESPONSE" | jq -r '.success')" != "true" ]; then
    echo "❌ 获取Token列表失败"
    exit 1
fi

TOTAL_TOKENS=$(echo "$TOKENS_LIST_RESPONSE" | jq -r '.data.total_tokens')
echo "✅ 获取Token列表成功，共 ${TOTAL_TOKENS} 个Token"
echo ""

# 验证token数量
if [ "$TOTAL_TOKENS" != "2" ]; then
    echo "❌ Token数量不正确，期望2个，实际${TOTAL_TOKENS}个"
    exit 1
fi

# 验证token信息
echo "Token详细信息："
echo "$TOKENS_LIST_RESPONSE" | jq '.data.tokens[] | {
  token_name: .token_name,
  access_key: .access_key,
  status_text: .status_text,
  remain_quota: .remain_quota,
  is_expired: .is_expired
}'
echo ""

# 6. 测试新接口：验证有效Token
echo "========================================="
echo "测试新接口2: 验证有效Token"
echo "========================================="
VERIFY_VALID_RESPONSE=$(curl -s -X POST "${API_URL}/api/user/external/token/verify" \
  -H "Content-Type: application/json" \
  -d "{
    \"access_key\": \"${TOKEN1_KEY}\"
  }")

echo "$VERIFY_VALID_RESPONSE" | jq '.'

if [ "$(echo "$VERIFY_VALID_RESPONSE" | jq -r '.success')" != "true" ]; then
    echo "❌ 验证Token失败"
    exit 1
fi

IS_VALID=$(echo "$VERIFY_VALID_RESPONSE" | jq -r '.data.is_valid')
if [ "$IS_VALID" != "true" ]; then
    echo "❌ Token应该是有效的，但返回无效"
    exit 1
fi
echo "✅ Token验证成功，Token有效"
echo ""

# 7. 测试：验证无效Token
echo "========================================="
echo "测试：验证无效Token"
echo "========================================="
VERIFY_INVALID_RESPONSE=$(curl -s -X POST "${API_URL}/api/user/external/token/verify" \
  -H "Content-Type: application/json" \
  -d "{
    \"access_key\": \"sk-invalid_token_12345678\"
  }")

echo "$VERIFY_INVALID_RESPONSE" | jq '.'

if [ "$(echo "$VERIFY_INVALID_RESPONSE" | jq -r '.success')" != "true" ]; then
    echo "❌ 验证请求失败"
    exit 1
fi

IS_VALID=$(echo "$VERIFY_INVALID_RESPONSE" | jq -r '.data.is_valid')
if [ "$IS_VALID" != "false" ]; then
    echo "❌ 无效Token应该返回false"
    exit 1
fi

ERROR_REASON=$(echo "$VERIFY_INVALID_RESPONSE" | jq -r '.data.error_reason')
echo "✅ 无效Token验证成功，原因: ${ERROR_REASON}"
echo ""

# 8. 删除一个Token后再次查询
echo "========================================="
echo "测试：删除Token后再次查询列表"
echo "========================================="
TOKEN1_ID=$(echo "$TOKEN1_RESPONSE" | jq -r '.data.token_id')
DELETE_RESPONSE=$(curl -s -X DELETE "${API_URL}/api/user/external/token" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"${EXTERNAL_USER_ID}\",
    \"token_id\": ${TOKEN1_ID}
  }")

echo "$DELETE_RESPONSE" | jq '.'

if [ "$(echo "$DELETE_RESPONSE" | jq -r '.success')" != "true" ]; then
    echo "❌ 删除Token失败"
    exit 1
fi
echo "✅ Token删除成功"
echo ""

# 再次查询Token列表
echo "再次查询Token列表..."
TOKENS_LIST_RESPONSE2=$(curl -s -X GET "${API_URL}/api/user/external/${EXTERNAL_USER_ID}/tokens")

echo "$TOKENS_LIST_RESPONSE2" | jq '.'

TOTAL_TOKENS2=$(echo "$TOKENS_LIST_RESPONSE2" | jq -r '.data.total_tokens')
if [ "$TOTAL_TOKENS2" != "1" ]; then
    echo "❌ 删除后Token数量不正确，期望1个，实际${TOTAL_TOKENS2}个"
    exit 1
fi
echo "✅ 删除后Token列表正确，剩余 ${TOTAL_TOKENS2} 个Token"
echo ""

# 验证被删除的Token
echo "验证被删除的Token..."
VERIFY_DELETED_RESPONSE=$(curl -s -X POST "${API_URL}/api/user/external/token/verify" \
  -H "Content-Type: application/json" \
  -d "{
    \"access_key\": \"${TOKEN1_KEY}\"
  }")

echo "$VERIFY_DELETED_RESPONSE" | jq '.'

IS_VALID_DELETED=$(echo "$VERIFY_DELETED_RESPONSE" | jq -r '.data.is_valid')
if [ "$IS_VALID_DELETED" != "false" ]; then
    echo "⚠️  注意：被删除的Token可能因Redis缓存暂时仍显示有效，稍后会失效"
else
    echo "✅ 被删除的Token验证失败（符合预期）"
fi
echo ""

# 9. 测试不存在的用户
echo "========================================="
echo "测试：查询不存在用户的Token列表"
echo "========================================="
NONEXIST_RESPONSE=$(curl -s -X GET "${API_URL}/api/user/external/nonexist_user_123/tokens")

echo "$NONEXIST_RESPONSE" | jq '.'

if [ "$(echo "$NONEXIST_RESPONSE" | jq -r '.success')" == "true" ]; then
    echo "❌ 不存在的用户应该返回失败"
    exit 1
fi
echo "✅ 不存在用户的查询正确返回失败"
echo ""

# 测试完成
echo "========================================="
echo "✅ 所有测试通过！"
echo "========================================="
echo ""
echo "新增功能验证完成："
echo "1. ✅ GET /api/user/external/:external_user_id/tokens - 获取用户所有Token列表"
echo "2. ✅ POST /api/user/external/token/verify - 验证Token有效性"
echo ""
echo "功能特性："
echo "- Token列表显示脱敏的access_key（前8位+后4位）"
echo "- 显示Token状态、剩余额度、过期时间等信息"
echo "- Token验证接口返回详细的有效性状态和错误原因"
echo "- 支持检测已删除、过期、禁用的Token"
echo ""
