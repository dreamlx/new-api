#!/bin/bash

# Token消费回调功能测试脚本
# 测试New API的回调功能是否正常工作

set -e

# 测试配置
BASE_URL="http://localhost:3000"
CALLBACK_URL="http://localhost:5000/api/consume-notify"
TEST_USER_ID="test_callback_$(date +%s)"
TEST_SECRET="test_secret_abc123"

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 测试结果计数
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# 测试结果函数
test_result() {
    local test_name=$1
    local result=$2
    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    if [ "$result" = "PASS" ]; then
        echo -e "${GREEN}✓ PASS${NC}: $test_name"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        echo -e "${RED}✗ FAIL${NC}: $test_name"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
}

# 检查JSON字段
check_json_field() {
    local json=$1
    local field=$2
    local expected=$3
    local actual=$(echo "$json" | jq -r "$field")

    if [ "$actual" = "$expected" ]; then
        return 0
    else
        echo "  期望: $expected, 实际: $actual"
        return 1
    fi
}

echo "================================"
echo "Token消费回调功能测试"
echo "================================"
echo ""

# ===== 测试1: 用户同步 =====
echo "1. 同步测试用户..."
SYNC_RESPONSE=$(curl -s -X POST "$BASE_URL/api/user/external/sync" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$TEST_USER_ID\",
    \"username\": \"callback_test_user\",
    \"display_name\": \"回调测试用户\",
    \"email\": \"callback@test.com\"
  }")

if check_json_field "$SYNC_RESPONSE" ".success" "true"; then
    test_result "用户同步" "PASS"
    USER_ID=$(echo "$SYNC_RESPONSE" | jq -r '.data.user_id')
    echo "  用户ID: $USER_ID"
else
    test_result "用户同步" "FAIL"
    echo "$SYNC_RESPONSE" | jq
    exit 1
fi

echo ""

# ===== 测试2: 用户充值 =====
echo "2. 用户充值 \$50..."
TOPUP_RESPONSE=$(curl -s -X POST "$BASE_URL/api/user/external/topup" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$TEST_USER_ID\",
    \"amount_usd\": 50,
    \"payment_id\": \"test_payment_$(date +%s)\"
  }")

if check_json_field "$TOPUP_RESPONSE" ".success" "true"; then
    test_result "用户充值" "PASS"
    CURRENT_QUOTA=$(echo "$TOPUP_RESPONSE" | jq -r '.data.current_quota')
    echo "  当前余额: $CURRENT_QUOTA quota"
else
    test_result "用户充值" "FAIL"
    echo "$TOPUP_RESPONSE" | jq
    exit 1
fi

echo ""

# ===== 测试3: 创建支持回调的Token =====
echo "3. 创建支持回调的Token..."
TOKEN_RESPONSE=$(curl -s -X POST "$BASE_URL/api/user/external/token" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$TEST_USER_ID\",
    \"token_name\": \"回调测试Token\",
    \"allocated_quota\": 10000000,
    \"expires_in_days\": 365,
    \"callback_url\": \"$CALLBACK_URL\",
    \"callback_enabled\": true,
    \"callback_secret\": \"$TEST_SECRET\"
  }")

if check_json_field "$TOKEN_RESPONSE" ".success" "true"; then
    test_result "Token创建" "PASS"

    TOKEN_ID=$(echo "$TOKEN_RESPONSE" | jq -r '.data.token_id')
    ACCESS_KEY=$(echo "$TOKEN_RESPONSE" | jq -r '.data.access_key')
    CALLBACK_ENABLED=$(echo "$TOKEN_RESPONSE" | jq -r '.data.callback_enabled')
    CALLBACK_URL_MASKED=$(echo "$TOKEN_RESPONSE" | jq -r '.data.callback_url_masked')

    echo "  Token ID: $TOKEN_ID"
    echo "  Access Key: $ACCESS_KEY"
    echo "  Callback Enabled: $CALLBACK_ENABLED"
    echo "  Callback URL: $CALLBACK_URL_MASKED"

    # 验证callback字段
    if [ "$CALLBACK_ENABLED" = "true" ]; then
        test_result "回调配置启用" "PASS"
    else
        test_result "回调配置启用" "FAIL"
    fi

    if [[ "$CALLBACK_URL_MASKED" == *"/***"* ]]; then
        test_result "回调URL脱敏显示" "PASS"
    else
        test_result "回调URL脱敏显示" "FAIL"
    fi
else
    test_result "Token创建" "FAIL"
    echo "$TOKEN_RESPONSE" | jq
    exit 1
fi

echo ""

# ===== 测试4: 创建不支持回调的Token（对照组） =====
echo "4. 创建不支持回调的Token（对照组）..."
TOKEN_NO_CALLBACK_RESPONSE=$(curl -s -X POST "$BASE_URL/api/user/external/token" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$TEST_USER_ID\",
    \"token_name\": \"普通Token\",
    \"allocated_quota\": 5000000,
    \"expires_in_days\": 365
  }")

if check_json_field "$TOKEN_NO_CALLBACK_RESPONSE" ".success" "true"; then
    test_result "普通Token创建" "PASS"

    TOKEN_NO_CALLBACK_ID=$(echo "$TOKEN_NO_CALLBACK_RESPONSE" | jq -r '.data.token_id')
    ACCESS_KEY_NO_CALLBACK=$(echo "$TOKEN_NO_CALLBACK_RESPONSE" | jq -r '.data.access_key')
    CALLBACK_ENABLED_NO=$(echo "$TOKEN_NO_CALLBACK_RESPONSE" | jq -r '.data.callback_enabled')

    echo "  Token ID: $TOKEN_NO_CALLBACK_ID"
    echo "  Access Key: $ACCESS_KEY_NO_CALLBACK"
    echo "  Callback Enabled: $CALLBACK_ENABLED_NO"

    # 验证callback字段不存在或为空
    if [ "$CALLBACK_ENABLED_NO" = "null" ] || [ "$CALLBACK_ENABLED_NO" = "false" ] || [ "$CALLBACK_ENABLED_NO" = "" ]; then
        test_result "普通Token无回调配置" "PASS"
    else
        test_result "普通Token无回调配置" "FAIL"
    fi
else
    test_result "普通Token创建" "FAIL"
fi

echo ""

# ===== 测试5: 检查数据库字段 =====
echo "5. 验证Token数据库字段..."
echo "  提示：请手动检查数据库tokens表，确认以下字段存在："
echo "  - callback_url"
echo "  - callback_enabled"
echo "  - callback_secret"
echo ""
echo "  SQL查询示例："
echo "  SELECT id, name, callback_enabled, callback_url, callback_secret FROM tokens WHERE id IN ($TOKEN_ID, $TOKEN_NO_CALLBACK_ID);"
echo ""
echo "  ${YELLOW}⚠ 需要手动验证${NC}"

echo ""

# ===== 测试6: 模拟LLM调用（触发回调） =====
echo "6. 使用Token调用LLM（应触发回调）..."
echo "  提示：本测试需要："
echo "  1. New API服务正在运行"
echo "  2. 至少配置了一个可用的LLM渠道"
echo "  3. CEC回调服务正在监听 $CALLBACK_URL"
echo ""
echo "  执行以下命令测试回调："
echo ""
echo "  curl -X POST \"$BASE_URL/v1/chat/completions\" \\"
echo "    -H \"Authorization: Bearer $ACCESS_KEY\" \\"
echo "    -H \"Content-Type: application/json\" \\"
echo "    -d '{"
echo "      \"model\": \"deepseek-chat\","
echo "      \"messages\": [{\"role\": \"user\", \"content\": \"测试回调功能\"}]"
echo "    }'"
echo ""
echo "  ${YELLOW}⚠ 需要手动执行LLM调用并观察回调日志${NC}"

echo ""

# ===== 测试7: 回调日志检查 =====
echo "7. 检查回调日志..."
echo "  提示：查看New API系统日志，应该看到："
echo "  - callback success: tokenId=$TOKEN_ID, status=200"
echo "  或"
echo "  - callback failed: tokenId=$TOKEN_ID, status=XXX"
echo "  或"
echo "  - callback request failed: tokenId=$TOKEN_ID, error=..."
echo ""
echo "  ${YELLOW}⚠ 需要手动检查系统日志${NC}"

echo ""

# ===== 测试总结 =====
echo "================================"
echo "测试总结"
echo "================================"
echo "总测试数: $TOTAL_TESTS"
echo -e "通过: ${GREEN}$PASSED_TESTS${NC}"
echo -e "失败: ${RED}$FAILED_TESTS${NC}"
echo ""

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "${GREEN}✓ 所有自动化测试通过！${NC}"
    echo ""
    echo "手动测试步骤："
    echo "1. 启动CEC回调模拟服务（参考 docs/callback-feature.md）"
    echo "2. 执行上面的LLM调用命令"
    echo "3. 检查CEC服务是否收到回调数据"
    echo "4. 检查New API系统日志中的回调记录"
else
    echo -e "${RED}✗ 存在测试失败，请检查错误信息${NC}"
    exit 1
fi

echo ""
echo "测试用户信息（用于手动清理）："
echo "  External User ID: $TEST_USER_ID"
echo "  User ID: $USER_ID"
echo "  Token ID (回调): $TOKEN_ID"
echo "  Token ID (普通): $TOKEN_NO_CALLBACK_ID"
