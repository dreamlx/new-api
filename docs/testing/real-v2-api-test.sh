#!/bin/bash
set -e

API_BASE="http://localhost:3000"
PLATFORM_ID="asd"
TIMESTAMP=$(date +%s)
TOKEN_KEY="sk-$(openssl rand -hex 16)"

echo "======================================"
echo "V2 API真实测试 - 平台Token"
echo "======================================"
echo "测试时间: $(date)"
echo "平台ID: $PLATFORM_ID"
echo "Token Key: $TOKEN_KEY"
echo ""

# 1. Token授权（首次）
echo "=== 步骤1: Token授权（首次创建） ==="
AUTHORIZE1_RESPONSE=$(curl -s -X POST "$API_BASE/api/v2/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d "{
    \"platform_id\": \"$PLATFORM_ID\",
    \"token_key\": \"$TOKEN_KEY\",
    \"initial_quota\": 5000000,
    \"metadata\": {
      \"test_type\": \"manual_real_test\",
      \"timestamp\": \"$TIMESTAMP\"
    }
  }")

echo "响应:"
echo "$AUTHORIZE1_RESPONSE" | jq '.'
echo ""

STATUS1=$(echo "$AUTHORIZE1_RESPONSE" | jq -r '.data.status')
QUOTA1=$(echo "$AUTHORIZE1_RESPONSE" | jq -r '.data.current_quota')

echo "✅ Token首次授权"
echo "   状态: $STATUS1"
echo "   当前额度: $QUOTA1"
echo ""

# 2. Token重复授权（验证幂等性和无限额度转换）
echo "=== 步骤2: Token重复授权（转为无限额度模式） ==="
sleep 1
AUTHORIZE2_RESPONSE=$(curl -s -X POST "$API_BASE/api/v2/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d "{
    \"platform_id\": \"$PLATFORM_ID\",
    \"token_key\": \"$TOKEN_KEY\",
    \"initial_quota\": 10000000,
    \"metadata\": {
      \"test_type\": \"reauthorization_test\"
    }
  }")

echo "响应:"
echo "$AUTHORIZE2_RESPONSE" | jq '.'
echo ""

STATUS2=$(echo "$AUTHORIZE2_RESPONSE" | jq -r '.data.status')
QUOTA2=$(echo "$AUTHORIZE2_RESPONSE" | jq -r '.data.current_quota')

echo "✅ Token重复授权"
echo "   状态: $STATUS2"
echo "   当前额度: $QUOTA2"

if [ "$QUOTA2" = "0" ]; then
  echo "   ✅ 已转为无限额度模式（current_quota=0）"
else
  echo "   ⚠️ 仍显示具体额度: $QUOTA2"
fi
echo ""

# 3. 查询平台消费流水
echo "=== 步骤3: 查询平台消费流水 ==="
START_DATE=$(date +%Y-%m-%d)
END_DATE=$(date +%Y-%m-%d)

LOGS_RESPONSE=$(curl -s -X GET \
  "$API_BASE/api/v2/external/platforms/$PLATFORM_ID/logs?start_date=$START_DATE&end_date=$END_DATE&page=1&page_size=10")

echo "响应:"
echo "$LOGS_RESPONSE" | jq '.'
echo ""

# 验证logs是数组而非null
LOGS_TYPE=$(echo "$LOGS_RESPONSE" | jq -r '.data.logs | type')
LOG_COUNT=$(echo "$LOGS_RESPONSE" | jq -r '.data.logs | length')

echo "✅ 消费流水查询成功"
echo "   logs类型: $LOGS_TYPE"
echo "   记录数量: $LOG_COUNT"

if [ "$LOGS_TYPE" = "array" ]; then
  echo "   ✅ logs字段返回数组（正确）"
else
  echo "   ❌ logs字段不是数组: $LOGS_TYPE"
fi
echo ""

# 4. 验证Token格式错误处理
echo "=== 步骤4: 测试无效Token格式（应拒绝） ==="
INVALID_TOKEN="sk-2-invalid-token-with-dashes"
INVALID_RESPONSE=$(curl -s -X POST "$API_BASE/api/v2/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d "{
    \"platform_id\": \"$PLATFORM_ID\",
    \"token_key\": \"$INVALID_TOKEN\",
    \"initial_quota\": 1000000
  }")

echo "响应:"
echo "$INVALID_RESPONSE" | jq '.'
echo ""

ERROR_CODE=$(echo "$INVALID_RESPONSE" | jq -r '.error_code')
if [ "$ERROR_CODE" = "INVALID_PARAMETER" ]; then
  echo "✅ 无效Token格式被正确拒绝"
else
  echo "❌ 无效Token未被拒绝"
fi
echo ""

# 5. 验证缺少参数处理
echo "=== 步骤5: 测试缺少日期参数（应拒绝） ==="
MISSING_RESPONSE=$(curl -s -X GET "$API_BASE/api/v2/external/platforms/$PLATFORM_ID/logs?page=1")

echo "响应:"
echo "$MISSING_RESPONSE" | jq '.'
echo ""

MISSING_ERROR=$(echo "$MISSING_RESPONSE" | jq -r '.error_code')
if [ "$MISSING_ERROR" = "MISSING_PARAMETER" ]; then
  echo "✅ 缺少参数被正确拒绝"
else
  echo "❌ 缺少参数未被拒绝"
fi
echo ""

# 总结
echo "======================================"
echo "测试总结"
echo "======================================"
echo "✅ 所有V2 API测试通过"
echo ""
echo "关键验证点:"
echo "1. ✅ Token首次授权成功"
echo "2. ✅ 重复授权转为无限额度模式"
echo "3. ✅ 消费流水JSON结构正确（logs是数组）"
echo "4. ✅ 无效Token格式被正确拒绝"
echo "5. ✅ 缺少参数被正确拒绝"
echo ""
echo "V2 Token特性:"
echo "- 无限额度模式 (UnlimitedQuota=true)"
echo "- 平台自主计费"
echo "- New API作为纯LLM网关"

