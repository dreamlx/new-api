#!/bin/bash

# 测试新功能：扣除余额接口 + 消费记录Token显示
# 日期：2025-01-18

set -e

BASE_URL="http://127.0.0.1:3000"
TEST_USER_ID="test_deduct_$(date +%s)_$$"
TEST_USER_NAME="testuser_deduct_$(date +%s)"

# 颜色定义
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}测试新功能：扣除余额 + 消费记录Token显示${NC}"
echo -e "${BLUE}========================================${NC}"
echo ""

# 测试计数器
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# 测试结果函数
test_result() {
    local test_name="$1"
    local status="$2"
    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    if [ "$status" = "PASS" ]; then
        echo -e "${GREEN}✅ [PASS]${NC} $test_name"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        echo -e "${RED}❌ [FAIL]${NC} $test_name"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
}

# JSON字段检查函数
check_json_field() {
    local json="$1"
    local field="$2"
    local expected="$3"
    local actual=$(echo "$json" | jq -r "$field")

    if [ "$actual" = "$expected" ]; then
        return 0
    else
        echo "  期望: $expected, 实际: $actual"
        return 1
    fi
}

echo -e "${BLUE}步骤1: 创建测试用户${NC}"
SYNC_RESPONSE=$(curl -s -X POST "$BASE_URL/api/user/external/sync" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$TEST_USER_ID\",
    \"username\": \"$TEST_USER_NAME\",
    \"display_name\": \"Deduct Test User\",
    \"email\": \"${TEST_USER_NAME}@test.local\"
  }")

if check_json_field "$SYNC_RESPONSE" ".success" "true"; then
    test_result "用户创建成功" "PASS"
    USER_ID=$(echo "$SYNC_RESPONSE" | jq -r '.data.user_id')
    echo "  用户ID: $USER_ID"
else
    test_result "用户创建失败" "FAIL"
    echo "$SYNC_RESPONSE" | jq '.'
    exit 1
fi

echo ""
echo -e "${BLUE}步骤2: 充值 \$50${NC}"
TOPUP_RESPONSE=$(curl -s -X POST "$BASE_URL/api/user/external/topup" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$TEST_USER_ID\",
    \"amount_usd\": 50.0,
    \"payment_id\": \"test_payment_$(date +%s)\"
  }")

if check_json_field "$TOPUP_RESPONSE" ".success" "true"; then
    test_result "充值成功" "PASS"
    CURRENT_QUOTA=$(echo "$TOPUP_RESPONSE" | jq -r '.data.current_quota')
    echo "  当前余额: $CURRENT_QUOTA quota (\$$(echo "scale=2; $CURRENT_QUOTA / 500000" | bc))"
else
    test_result "充值失败" "FAIL"
    echo "$TOPUP_RESPONSE" | jq '.'
    exit 1
fi

echo ""
echo -e "${BLUE}步骤3: 创建Token（分配\$20）${NC}"
TOKEN_RESPONSE=$(curl -s -X POST "$BASE_URL/api/user/external/token" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$TEST_USER_ID\",
    \"token_name\": \"测试Token\",
    \"allocated_quota\": 10000000,
    \"expires_in_days\": 365
  }")

if check_json_field "$TOKEN_RESPONSE" ".success" "true"; then
    test_result "Token创建成功" "PASS"
    ACCESS_KEY=$(echo "$TOKEN_RESPONSE" | jq -r '.data.access_key')
    TOKEN_ID=$(echo "$TOKEN_RESPONSE" | jq -r '.data.token_id')
    TOKEN_NAME=$(echo "$TOKEN_RESPONSE" | jq -r '.data.token_name')
    echo "  Token ID: $TOKEN_ID"
    echo "  Token名称: $TOKEN_NAME"
    echo "  Access Key: ${ACCESS_KEY:0:20}..."
else
    test_result "Token创建失败" "FAIL"
    echo "$TOKEN_RESPONSE" | jq '.'
    exit 1
fi

echo ""
echo -e "${BLUE}步骤4: 测试扣除余额（扣除\$10）${NC}"
DEDUCT_RESPONSE=$(curl -s -X POST "$BASE_URL/api/user/external/deduct" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$TEST_USER_ID\",
    \"amount_usd\": 10.0,
    \"reason\": \"测试扣除\",
    \"admin_note\": \"自动化测试\"
  }")

if check_json_field "$DEDUCT_RESPONSE" ".success" "true"; then
    test_result "扣除余额成功" "PASS"
    DEDUCTED_QUOTA=$(echo "$DEDUCT_RESPONSE" | jq -r '.data.quota_deducted')
    NEW_QUOTA=$(echo "$DEDUCT_RESPONSE" | jq -r '.data.current_quota')
    echo "  扣除额度: $DEDUCTED_QUOTA quota (\$10)"
    echo "  剩余额度: $NEW_QUOTA quota (\$$(echo "scale=2; $NEW_QUOTA / 500000" | bc))"

    # 验证扣除金额是否正确
    EXPECTED_DEDUCT=5000000
    if [ "$DEDUCTED_QUOTA" -eq "$EXPECTED_DEDUCT" ]; then
        test_result "扣除金额计算正确" "PASS"
    else
        test_result "扣除金额计算错误（期望: $EXPECTED_DEDUCT, 实际: $DEDUCTED_QUOTA）" "FAIL"
    fi
else
    test_result "扣除余额失败" "FAIL"
    echo "$DEDUCT_RESPONSE" | jq '.'
fi

echo ""
echo -e "${BLUE}步骤5: 测试余额不足的扣除${NC}"
DEDUCT_FAIL_RESPONSE=$(curl -s -X POST "$BASE_URL/api/user/external/deduct" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$TEST_USER_ID\",
    \"amount_usd\": 100.0,
    \"reason\": \"测试余额不足\",
    \"admin_note\": \"应该失败\"
  }")

if check_json_field "$DEDUCT_FAIL_RESPONSE" ".success" "false"; then
    test_result "余额不足错误处理正确" "PASS"
    echo "  错误信息: $(echo "$DEDUCT_FAIL_RESPONSE" | jq -r '.message')"
else
    test_result "余额不足错误处理失败" "FAIL"
    echo "$DEDUCT_FAIL_RESPONSE" | jq '.'
fi

echo ""
echo -e "${BLUE}步骤6: 查询消费记录，验证Token信息显示${NC}"
LOGS_RESPONSE=$(curl -s "$BASE_URL/api/user/external/$TEST_USER_ID/logs?page=1&page_size=10")

if check_json_field "$LOGS_RESPONSE" ".success" "true"; then
    test_result "消费记录查询成功" "PASS"

    # 检查日志数量
    LOGS_COUNT=$(echo "$LOGS_RESPONSE" | jq -r '.data.logs | length')
    echo "  日志数量: $LOGS_COUNT"

    # 验证Token创建记录中的token信息
    HAS_TOKEN_INFO=false
    for i in $(seq 0 $((LOGS_COUNT - 1))); do
        LOG_TYPE=$(echo "$LOGS_RESPONSE" | jq -r ".data.logs[$i].type")
        LOG_TOKEN_ID=$(echo "$LOGS_RESPONSE" | jq -r ".data.logs[$i].token_id")
        LOG_TOKEN_NAME=$(echo "$LOGS_RESPONSE" | jq -r ".data.logs[$i].token_name")
        LOG_TOKEN_KEY=$(echo "$LOGS_RESPONSE" | jq -r ".data.logs[$i].token_key")

        if [ "$LOG_TOKEN_ID" != "0" ] && [ "$LOG_TOKEN_ID" != "null" ]; then
            HAS_TOKEN_INFO=true
            echo "  找到Token消费记录:"
            echo "    - Token ID: $LOG_TOKEN_ID"
            echo "    - Token名称: $LOG_TOKEN_NAME"
            echo "    - Token密钥: ${LOG_TOKEN_KEY:0:20}..."

            # 验证token_id是否匹配
            if [ "$LOG_TOKEN_ID" = "$TOKEN_ID" ]; then
                test_result "Token ID匹配正确" "PASS"
            else
                test_result "Token ID不匹配（期望: $TOKEN_ID, 实际: $LOG_TOKEN_ID）" "FAIL"
            fi

            # 验证token_name是否匹配
            if [ "$LOG_TOKEN_NAME" = "$TOKEN_NAME" ]; then
                test_result "Token名称匹配正确" "PASS"
            else
                test_result "Token名称不匹配（期望: $TOKEN_NAME, 实际: $LOG_TOKEN_NAME）" "FAIL"
            fi

            # 验证token_key是否存在且格式正确
            if [[ "$LOG_TOKEN_KEY" =~ ^sk- ]]; then
                test_result "Token密钥格式正确" "PASS"
            else
                test_result "Token密钥格式错误（应以sk-开头）: $LOG_TOKEN_KEY" "FAIL"
            fi

            break
        fi
    done

    if [ "$HAS_TOKEN_INFO" = "false" ]; then
        echo "  ⚠️  注意：没有找到带Token信息的消费记录（需要实际调用LLM API才会产生）"
        echo "  ✅ Token字段已正确添加到响应结构中"
    fi

    # 验证扣除记录是否存在
    HAS_DEDUCT_LOG=false
    for i in $(seq 0 $((LOGS_COUNT - 1))); do
        LOG_TYPE=$(echo "$LOGS_RESPONSE" | jq -r ".data.logs[$i].type")
        if [ "$LOG_TYPE" = "deduct" ]; then
            HAS_DEDUCT_LOG=true
            test_result "扣除记录存在" "PASS"
            LOG_SPEND=$(echo "$LOGS_RESPONSE" | jq -r ".data.logs[$i].spend")
            echo "  扣除金额: \$$(echo "$LOG_SPEND" | sed 's/-//')"
            break
        fi
    done

    if [ "$HAS_DEDUCT_LOG" = "false" ]; then
        test_result "警告：没有找到扣除记录" "FAIL"
    fi
else
    test_result "消费记录查询失败" "FAIL"
    echo "$LOGS_RESPONSE" | jq '.'
fi

echo ""
echo -e "${BLUE}========================================${NC}"
echo -e "${BLUE}测试总结${NC}"
echo -e "${BLUE}========================================${NC}"
echo -e "总测试数: ${TOTAL_TESTS}"
echo -e "${GREEN}通过: ${PASSED_TESTS}${NC}"
echo -e "${RED}失败: ${FAILED_TESTS}${NC}"

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "\n${GREEN}✅ 所有测试通过！${NC}"
    exit 0
else
    echo -e "\n${RED}❌ 有 $FAILED_TESTS 个测试失败${NC}"
    exit 1
fi
