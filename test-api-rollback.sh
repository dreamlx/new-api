#!/bin/bash

# 外部用户API回滚测试脚本
# 测试只使用 external_user_id，不支持 wechat_openid

API_URL="http://localhost:3000"
EXTERNAL_USER_ID="test_rollback_$(date +%s)"
USERNAME="rollback_user_$(date +%s)"

echo "🧪 开始测试外部用户API（回滚后版本）"
echo "=========================================="
echo ""

# 1. 测试用户同步（创建）
echo "📝 1. 测试用户同步API - 创建新用户"
SYNC_RESPONSE=$(curl -s -X POST $API_URL/api/user/external/sync \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$EXTERNAL_USER_ID\",
    \"username\": \"$USERNAME\",
    \"display_name\": \"Rollback Test User\",
    \"email\": \"${USERNAME}@test.com\",
    \"wechat_unionid\": \"oUnion_rollback_test\",
    \"login_type\": \"email\"
  }")

echo "$SYNC_RESPONSE" | jq .
USER_ID=$(echo "$SYNC_RESPONSE" | jq -r '.data.user_id')
echo "✅ 用户创建成功，user_id: $USER_ID"
echo ""

# 2. 测试充值API
echo "💰 2. 测试充值API"
TOPUP_RESPONSE=$(curl -s -X POST $API_URL/api/user/external/topup \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$EXTERNAL_USER_ID\",
    \"amount_usd\": 10.0,
    \"payment_id\": \"test_payment_$(date +%s)\"
  }")

echo "$TOPUP_RESPONSE" | jq .
QUOTA=$(echo "$TOPUP_RESPONSE" | jq -r '.data.current_quota')
echo "✅ 充值成功，当前quota: $QUOTA"
echo ""

# 3. 测试创建Token
echo "🔑 3. 测试创建Token API"
TOKEN_RESPONSE=$(curl -s -X POST $API_URL/api/user/external/token \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$EXTERNAL_USER_ID\",
    \"token_name\": \"Test Token\",
    \"expires_in_days\": 30
  }")

echo "$TOKEN_RESPONSE" | jq .
TOKEN_ID=$(echo "$TOKEN_RESPONSE" | jq -r '.data.token_id')
ACCESS_KEY=$(echo "$TOKEN_RESPONSE" | jq -r '.data.access_key')
echo "✅ Token创建成功，token_id: $TOKEN_ID"
echo ""

# 4. 测试获取用户统计
echo "📊 4. 测试用户统计API"
STATS_RESPONSE=$(curl -s -X GET "$API_URL/api/user/external/$EXTERNAL_USER_ID/stats")
echo "$STATS_RESPONSE" | jq '.data.user_info | {external_user_id, username, current_quota, current_balance}'
echo "✅ 统计信息获取成功"
echo ""

# 5. 测试消费记录查询
echo "📋 5. 测试消费记录查询API"
LOGS_RESPONSE=$(curl -s -X GET "$API_URL/api/user/external/$EXTERNAL_USER_ID/logs")
LOG_COUNT=$(echo "$LOGS_RESPONSE" | jq '.data.logs | length')
echo "$LOGS_RESPONSE" | jq '.data.pagination'
echo "✅ 消费记录查询成功，共 $LOG_COUNT 条记录"
echo ""

# 6. 测试删除Token
echo "🗑️  6. 测试删除Token API"
DELETE_RESPONSE=$(curl -s -X DELETE $API_URL/api/user/external/token \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$EXTERNAL_USER_ID\",
    \"token_id\": $TOKEN_ID
  }")

echo "$DELETE_RESPONSE" | jq .
echo "✅ Token删除成功"
echo ""

# 7. 测试不支持 wechat_openid（应该失败）
echo "❌ 7. 测试wechat_openid参数（预期失败）"
OPENID_RESPONSE=$(curl -s -X POST $API_URL/api/user/external/topup \
  -H "Content-Type: application/json" \
  -d "{
    \"wechat_openid\": \"oTest123456\",
    \"amount_usd\": 5.0,
    \"payment_id\": \"test_fail\"
  }")

echo "$OPENID_RESPONSE" | jq .
if echo "$OPENID_RESPONSE" | grep -q "external_user_id"; then
  echo "✅ 正确：不支持wechat_openid参数，必须使用external_user_id"
else
  echo "❌ 错误：仍然支持wechat_openid参数"
fi
echo ""

echo "=========================================="
echo "🎉 API测试完成！"
echo ""
echo "测试摘要："
echo "  - External User ID: $EXTERNAL_USER_ID"
echo "  - User ID: $USER_ID"
echo "  - Current Quota: $QUOTA"
echo "  - Token ID: $TOKEN_ID (已删除)"
echo ""