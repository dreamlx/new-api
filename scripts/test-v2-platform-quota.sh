#!/usr/bin/env bash
#######################################
# V2 API 平台Token测试脚本
# 测试场景：下游平台（asd）的无限额度Token和消费流水
#######################################

set -euo pipefail

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# 配置
API_BASE="${API_BASE:-http://localhost:3000}"
PLATFORM_ID="${PLATFORM_ID:-asd}"
TEST_TOKEN_KEY="sk-test_$(openssl rand -hex 16)"

# 日志函数
log_info() { echo -e "${BLUE}[INFO]${NC} $*"; }
log_success() { echo -e "${GREEN}[✓]${NC} $*"; }
log_error() { echo -e "${RED}[✗]${NC} $*"; }
log_test() { echo -e "${YELLOW}[TEST]${NC} $*"; }

# 测试计数
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# 测试结果记录
test_result() {
    local test_name="$1"
    local result="$2"
    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    if [ "$result" = "PASS" ]; then
        PASSED_TESTS=$((PASSED_TESTS + 1))
        log_success "$test_name"
    else
        FAILED_TESTS=$((FAILED_TESTS + 1))
        log_error "$test_name"
    fi
}

# JSON验证函数
check_json_field() {
    local json="$1"
    local field="$2"
    local expected="$3"

    local actual=$(echo "$json" | jq -r "$field")
    if [ "$actual" = "$expected" ]; then
        return 0
    else
        echo "Expected: $expected, Got: $actual" >&2
        return 1
    fi
}

echo "======================================"
echo "V2 API 平台Token测试"
echo "======================================"
echo "API地址: $API_BASE"
echo "平台ID: $PLATFORM_ID"
echo "测试Token: $TEST_TOKEN_KEY"
echo "======================================"
echo ""

###########################################
# 测试1: Token授权（无限额度）
###########################################
log_test "测试1: 平台Token授权（无限额度模式）"
AUTHORIZE_RESPONSE=$(curl -s -X POST "$API_BASE/api/v2/external/tokens/authorize" \
    -H "Content-Type: application/json" \
    -d "{
        \"platform_id\": \"$PLATFORM_ID\",
        \"token_key\": \"$TEST_TOKEN_KEY\",
        \"initial_quota\": 5000000,
        \"metadata\": {
            \"test_type\": \"quota_validation\",
            \"created_at\": \"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"
        }
    }")

echo "Token授权响应:"
echo "$AUTHORIZE_RESPONSE" | jq '.'

if check_json_field "$AUTHORIZE_RESPONSE" ".success" "true"; then
    test_result "Token授权成功" "PASS"

    # ⚠️ 关键说明：首次授权可能返回initial_quota，重复授权后转为无限额度
    CURRENT_QUOTA=$(echo "$AUTHORIZE_RESPONSE" | jq -r '.data.current_quota')
    STATUS=$(echo "$AUTHORIZE_RESPONSE" | jq -r '.data.status')

    log_info "当前额度: $CURRENT_QUOTA"
    log_info "状态: $STATUS"

    # 首次授权状态应该是"authorized"
    if [ "$STATUS" = "authorized" ]; then
        test_result "V2 Token首次授权状态正确" "PASS"
        log_success "✓ Token已成功授权（首次创建）"
    else
        test_result "V2 Token授权状态异常" "FAIL"
        log_error "期望状态: authorized，实际: $STATUS"
    fi
else
    test_result "Token授权失败" "FAIL"
    echo "$AUTHORIZE_RESPONSE" | jq '.'
    exit 1
fi

###########################################
# 测试2: 重复授权（幂等性）
###########################################
log_test "测试2: 重复授权同一Token（幂等性测试）"
REAUTHORIZE_RESPONSE=$(curl -s -X POST "$API_BASE/api/v2/external/tokens/authorize" \
    -H "Content-Type: application/json" \
    -d "{
        \"platform_id\": \"$PLATFORM_ID\",
        \"token_key\": \"$TEST_TOKEN_KEY\",
        \"initial_quota\": 10000000,
        \"metadata\": {
            \"test_type\": \"idempotency_test\"
        }
    }")

echo "重复授权响应:"
echo "$REAUTHORIZE_RESPONSE" | jq '.'

if check_json_field "$REAUTHORIZE_RESPONSE" ".success" "true"; then
    test_result "重复授权成功（幂等性）" "PASS"

    # 检查状态是否更新为updated_unlimited
    STATUS=$(echo "$REAUTHORIZE_RESPONSE" | jq -r '.data.status')
    if [[ "$STATUS" == *"updated"* ]]; then
        test_result "Token更新状态正确" "PASS"
        log_info "更新状态: $STATUS"
    else
        test_result "Token更新状态可能错误" "FAIL"
    fi

    # 验证仍然是无限额度
    CURRENT_QUOTA=$(echo "$REAUTHORIZE_RESPONSE" | jq -r '.data.current_quota')
    if [ "$CURRENT_QUOTA" = "0" ]; then
        test_result "重复授权后仍为无限额度" "PASS"
    else
        test_result "重复授权后额度异常" "FAIL"
    fi
else
    test_result "重复授权失败" "FAIL"
fi

###########################################
# 测试3: 创建第二个Token
###########################################
log_test "测试3: 创建第二个平台Token"
TEST_TOKEN_KEY2="sk-test2_$(openssl rand -hex 16)"

AUTHORIZE2_RESPONSE=$(curl -s -X POST "$API_BASE/api/v2/external/tokens/authorize" \
    -H "Content-Type: application/json" \
    -d "{
        \"platform_id\": \"$PLATFORM_ID\",
        \"token_key\": \"$TEST_TOKEN_KEY2\",
        \"initial_quota\": 1000000,
        \"metadata\": {
            \"test_type\": \"second_token\"
        }
    }")

if check_json_field "$AUTHORIZE2_RESPONSE" ".success" "true"; then
    test_result "第二个Token创建成功" "PASS"
else
    test_result "第二个Token创建失败" "FAIL"
fi

###########################################
# 测试4: 获取平台消费流水（验证JSON结构）
###########################################
log_test "测试4: 获取平台消费流水"

# 获取当前日期用于查询
START_DATE=$(date -u +%Y-%m-%d)
END_DATE=$(date -u +%Y-%m-%d)

LOGS_RESPONSE=$(curl -s -X GET \
    "$API_BASE/api/v2/external/platforms/$PLATFORM_ID/logs?start_date=$START_DATE&end_date=$END_DATE&page=1&page_size=20")

echo "平台消费流水响应:"
echo "$LOGS_RESPONSE" | jq '.'

if check_json_field "$LOGS_RESPONSE" ".success" "true"; then
    test_result "平台消费流水查询成功" "PASS"

    # ⚠️ 关键验证：JSON响应结构完整性
    # 验证必要字段
    if echo "$LOGS_RESPONSE" | jq -e '.data.platform_id' > /dev/null; then
        test_result "平台ID字段存在" "PASS"
    else
        test_result "平台ID字段缺失" "FAIL"
    fi

    if echo "$LOGS_RESPONSE" | jq -e '.data.date_range' > /dev/null; then
        test_result "日期范围字段存在" "PASS"
    else
        test_result "日期范围字段缺失" "FAIL"
    fi

    # 验证logs数组（应该是空数组[]，而不是null）
    LOGS_ARRAY=$(echo "$LOGS_RESPONSE" | jq -r '.data.logs')
    if [ "$LOGS_ARRAY" != "null" ]; then
        test_result "消费记录数组存在" "PASS"
    else
        test_result "消费记录数组为null（应该是空数组）" "FAIL"
    fi

    if echo "$LOGS_RESPONSE" | jq -e '.data.pagination' > /dev/null; then
        test_result "分页信息存在" "PASS"
    else
        test_result "分页信息缺失" "FAIL"
    fi

    if echo "$LOGS_RESPONSE" | jq -e '.data.summary' > /dev/null; then
        test_result "汇总信息存在" "PASS"
    else
        test_result "汇总信息缺失" "FAIL"
    fi

    # 显示消费记录数量
    LOG_COUNT=$(echo "$LOGS_RESPONSE" | jq -r '.data.logs | length')
    log_info "消费记录数量: $LOG_COUNT"

    # 如果有消费记录，验证单条记录结构
    if [ "$LOG_COUNT" -gt "0" ]; then
        log_info "验证单条消费记录结构..."

        # ⚠️ 关键验证：单条记录必需字段
        FIRST_LOG=$(echo "$LOGS_RESPONSE" | jq -r '.data.logs[0]')

        if echo "$FIRST_LOG" | jq -e '.log_id' > /dev/null; then
            test_result "记录包含log_id字段" "PASS"
        else
            test_result "记录缺少log_id字段" "FAIL"
        fi

        if echo "$FIRST_LOG" | jq -e '.time' > /dev/null; then
            test_result "记录包含time字段" "PASS"
        else
            test_result "记录缺少time字段" "FAIL"
        fi

        if echo "$FIRST_LOG" | jq -e '.token_key' > /dev/null; then
            test_result "记录包含token_key字段" "PASS"
            TOKEN_KEY_IN_LOG=$(echo "$FIRST_LOG" | jq -r '.token_key')
            log_info "消费记录中的Token: $TOKEN_KEY_IN_LOG"
        else
            test_result "记录缺少token_key字段" "FAIL"
        fi

        if echo "$FIRST_LOG" | jq -e '.model_name' > /dev/null; then
            test_result "记录包含model_name字段" "PASS"
        else
            test_result "记录缺少model_name字段" "FAIL"
        fi

        if echo "$FIRST_LOG" | jq -e '.quota_cost' > /dev/null; then
            test_result "记录包含quota_cost字段" "PASS"
        else
            test_result "记录缺少quota_cost字段" "FAIL"
        fi
    else
        log_info "当前无消费记录（正常，刚创建的Token）"
    fi

    # 验证汇总信息结构
    SUMMARY=$(echo "$LOGS_RESPONSE" | jq -r '.data.summary')
    if echo "$SUMMARY" | jq -e '.total_requests' > /dev/null; then
        test_result "汇总包含total_requests字段" "PASS"
    else
        test_result "汇总缺少total_requests字段" "FAIL"
    fi

    if echo "$SUMMARY" | jq -e '.total_quota_consumed' > /dev/null; then
        test_result "汇总包含total_quota_consumed字段" "PASS"
    else
        test_result "汇总缺少total_quota_consumed字段" "FAIL"
    fi
else
    test_result "平台消费流水查询失败" "FAIL"
    echo "$LOGS_RESPONSE" | jq '.'
fi

###########################################
# 测试5: 无效日期查询（边界测试）
###########################################
log_test "测试5: 测试无效日期参数处理"
INVALID_DATE_RESPONSE=$(curl -s -X GET \
    "$API_BASE/api/v2/external/platforms/$PLATFORM_ID/logs?start_date=invalid&end_date=2025-01-01")

echo "无效日期响应:"
echo "$INVALID_DATE_RESPONSE" | jq '.'

if check_json_field "$INVALID_DATE_RESPONSE" ".success" "false"; then
    test_result "无效日期正确拒绝" "PASS"
    ERROR_MSG=$(echo "$INVALID_DATE_RESPONSE" | jq -r '.message')
    log_info "错误信息: $ERROR_MSG"
else
    test_result "无效日期未被正确处理" "FAIL"
fi

###########################################
# 测试6: 缺少必需参数（边界测试）
###########################################
log_test "测试6: 测试缺少日期参数处理"
MISSING_PARAM_RESPONSE=$(curl -s -X GET \
    "$API_BASE/api/v2/external/platforms/$PLATFORM_ID/logs?page=1")

echo "缺少参数响应:"
echo "$MISSING_PARAM_RESPONSE" | jq '.'

if check_json_field "$MISSING_PARAM_RESPONSE" ".success" "false"; then
    test_result "缺少参数正确拒绝" "PASS"
else
    test_result "缺少参数未被正确处理" "FAIL"
fi

###########################################
# 测试7: Token格式验证
###########################################
log_test "测试7: 测试无效Token格式（带多个短横线）"
INVALID_TOKEN="sk-2-invalid-token-with-dashes"
INVALID_TOKEN_RESPONSE=$(curl -s -X POST "$API_BASE/api/v2/external/tokens/authorize" \
    -H "Content-Type: application/json" \
    -d "{
        \"platform_id\": \"$PLATFORM_ID\",
        \"token_key\": \"$INVALID_TOKEN\",
        \"initial_quota\": 5000000
    }")

echo "无效Token格式响应:"
echo "$INVALID_TOKEN_RESPONSE" | jq '.'

if check_json_field "$INVALID_TOKEN_RESPONSE" ".success" "false"; then
    test_result "无效Token格式正确拒绝" "PASS"
    ERROR_MSG=$(echo "$INVALID_TOKEN_RESPONSE" | jq -r '.message')
    log_info "错误信息: $ERROR_MSG"
else
    test_result "无效Token格式未被拒绝（BUG）" "FAIL"
fi

###########################################
# 测试总结
###########################################
echo ""
echo "======================================"
echo "测试总结"
echo "======================================"
echo "总测试数: $TOTAL_TESTS"
echo -e "${GREEN}通过: $PASSED_TESTS${NC}"
echo -e "${RED}失败: $FAILED_TESTS${NC}"
echo "======================================"

echo ""
echo "⚠️ 关键验证点总结:"
echo "1. V2 Token使用无限额度（current_quota=0）"
echo "2. 消费流水JSON结构完整"
echo "3. token_key字段正确显示（完整密钥）"
echo "4. 分页和汇总信息完整"
echo "5. 错误处理正确"

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "\n${GREEN}✓ 所有测试通过！${NC}"
    exit 0
else
    echo -e "\n${RED}✗ 存在失败的测试！${NC}"
    exit 1
fi
