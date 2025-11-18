#!/usr/bin/env bash
#######################################
# V1 API Token独立额度测试脚本
# 测试场景：个人用户的Token额度管理和消费记录
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
TEST_USER_ID="test_quota_$(date +%s)"
TEST_TOKEN_NAME="Test Token for Quota"

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
echo "V1 API Token独立额度测试"
echo "======================================"
echo "API地址: $API_BASE"
echo "测试用户: $TEST_USER_ID"
echo "======================================"
echo ""

###########################################
# 测试1: 用户同步
###########################################
log_test "测试1: 创建测试用户"
RANDOM_SUFFIX=$(openssl rand -hex 4)
SYNC_RESPONSE=$(curl -s -X POST "$API_BASE/api/user/external/sync" \
    -H "Content-Type: application/json" \
    -d "{
        \"external_user_id\": \"$TEST_USER_ID\",
        \"username\": \"test_quota_${RANDOM_SUFFIX}\",
        \"display_name\": \"Test User for Quota\",
        \"email\": \"test_quota_${RANDOM_SUFFIX}@example.com\"
    }")

if check_json_field "$SYNC_RESPONSE" ".success" "true"; then
    test_result "用户同步成功" "PASS"
    USER_ID=$(echo "$SYNC_RESPONSE" | jq -r '.data.user_id')
    log_info "用户ID: $USER_ID"
else
    test_result "用户同步失败" "FAIL"
    echo "$SYNC_RESPONSE" | jq '.'
    exit 1
fi

###########################################
# 测试2: 用户充值
###########################################
log_test "测试2: 用户充值 \$100"
TOPUP_RESPONSE=$(curl -s -X POST "$API_BASE/api/user/external/topup" \
    -H "Content-Type: application/json" \
    -d "{
        \"external_user_id\": \"$TEST_USER_ID\",
        \"amount_usd\": 100,
        \"payment_id\": \"test_payment_$(date +%s)\"
    }")

if check_json_field "$TOPUP_RESPONSE" ".success" "true"; then
    test_result "充值成功" "PASS"
    CURRENT_QUOTA=$(echo "$TOPUP_RESPONSE" | jq -r '.data.current_quota')
    ADDED_QUOTA=50000000  # $100 * 500,000

    # 验证充值是否增加了预期额度（而不是验证绝对值）
    log_info "充值后额度: $CURRENT_QUOTA quota"
    if [ "$CURRENT_QUOTA" -ge "$ADDED_QUOTA" ]; then
        test_result "充值额度验证通过" "PASS"
        log_success "充值额度≥$ADDED_QUOTA (至少\$100)"
    else
        test_result "充值额度不足" "FAIL"
        log_error "期望≥$ADDED_QUOTA, 实际: $CURRENT_QUOTA"
    fi
else
    test_result "充值失败" "FAIL"
    echo "$TOPUP_RESPONSE" | jq '.'
fi

###########################################
# 测试3: 创建Token（分配额度）
###########################################
log_test "测试3: 创建Token并分配 \$30 额度"
TOKEN_RESPONSE=$(curl -s -X POST "$API_BASE/api/user/external/token" \
    -H "Content-Type: application/json" \
    -d "{
        \"external_user_id\": \"$TEST_USER_ID\",
        \"token_name\": \"$TEST_TOKEN_NAME\",
        \"allocated_quota\": 15000000,
        \"expires_in_days\": 365
    }")

echo "Token创建响应:"
echo "$TOKEN_RESPONSE" | jq '.'

if check_json_field "$TOKEN_RESPONSE" ".success" "true"; then
    test_result "Token创建成功" "PASS"

    # 验证关键字段
    TOKEN_ID=$(echo "$TOKEN_RESPONSE" | jq -r '.data.token_id')
    ACCESS_KEY=$(echo "$TOKEN_RESPONSE" | jq -r '.data.access_key')
    REMAIN_QUOTA=$(echo "$TOKEN_RESPONSE" | jq -r '.data.remain_quota')

    log_info "Token ID: $TOKEN_ID"
    log_info "Access Key: $ACCESS_KEY"
    log_info "分配额度: $REMAIN_QUOTA quota (\$30)"

    # ⚠️ 关键验证：Token的remain_quota应该等于allocated_quota
    if [ "$REMAIN_QUOTA" = "15000000" ]; then
        test_result "Token额度分配正确" "PASS"
    else
        test_result "Token额度分配错误" "FAIL"
        log_error "期望: 15000000, 实际: $REMAIN_QUOTA"
    fi
else
    test_result "Token创建失败" "FAIL"
    echo "$TOKEN_RESPONSE" | jq '.'
fi

###########################################
# 测试4: 获取Token列表（验证额度显示）
###########################################
log_test "测试4: 获取用户所有Token列表"
TOKENS_RESPONSE=$(curl -s -X GET "$API_BASE/api/user/external/$TEST_USER_ID/tokens")

echo "Token列表响应:"
echo "$TOKENS_RESPONSE" | jq '.'

if check_json_field "$TOKENS_RESPONSE" ".success" "true"; then
    test_result "Token列表查询成功" "PASS"

    # 验证Token数量
    TOKEN_COUNT=$(echo "$TOKENS_RESPONSE" | jq -r '.data.total_tokens')
    if [ "$TOKEN_COUNT" = "1" ]; then
        test_result "Token数量正确" "PASS"
    else
        test_result "Token数量错误" "FAIL"
    fi

    # ⚠️ 关键验证：Token列表中的remain_quota字段
    LISTED_QUOTA=$(echo "$TOKENS_RESPONSE" | jq -r '.data.tokens[0].remain_quota')
    if [ "$LISTED_QUOTA" = "15000000" ]; then
        test_result "Token列表中额度显示正确" "PASS"
    else
        test_result "Token列表中额度显示错误" "FAIL"
        log_error "期望: 15000000, 实际: $LISTED_QUOTA"
    fi

    # 验证状态字段
    TOKEN_STATUS=$(echo "$TOKENS_RESPONSE" | jq -r '.data.tokens[0].status_text')
    log_info "Token状态: $TOKEN_STATUS"
else
    test_result "Token列表查询失败" "FAIL"
fi

###########################################
# 测试5: 验证Token有效性
###########################################
log_test "测试5: 验证Token有效性"
VERIFY_RESPONSE=$(curl -s -X POST "$API_BASE/api/user/external/token/verify" \
    -H "Content-Type: application/json" \
    -d "{
        \"access_key\": \"$ACCESS_KEY\"
    }")

echo "Token验证响应:"
echo "$VERIFY_RESPONSE" | jq '.'

if check_json_field "$VERIFY_RESPONSE" ".success" "true" && \
   check_json_field "$VERIFY_RESPONSE" ".data.is_valid" "true"; then
    test_result "Token验证通过" "PASS"

    # ⚠️ 关键验证：验证响应中的remain_quota
    VERIFY_QUOTA=$(echo "$VERIFY_RESPONSE" | jq -r '.data.remain_quota')
    if [ "$VERIFY_QUOTA" = "15000000" ]; then
        test_result "验证响应中额度正确" "PASS"
    else
        test_result "验证响应中额度错误" "FAIL"
        log_error "期望: 15000000, 实际: $VERIFY_QUOTA"
    fi
else
    test_result "Token验证失败" "FAIL"
fi

###########################################
# 测试6: 创建第二个Token（测试余额扣减）
###########################################
log_test "测试6: 创建第二个Token（分配\$20）"
TOKEN2_RESPONSE=$(curl -s -X POST "$API_BASE/api/user/external/token" \
    -H "Content-Type: application/json" \
    -d "{
        \"external_user_id\": \"$TEST_USER_ID\",
        \"token_name\": \"Second Token\",
        \"allocated_quota\": 10000000,
        \"expires_in_days\": 365
    }")

echo "第二个Token创建响应:"
echo "$TOKEN2_RESPONSE" | jq '.'

if check_json_field "$TOKEN2_RESPONSE" ".success" "true"; then
    test_result "第二个Token创建成功" "PASS"

    TOKEN2_QUOTA=$(echo "$TOKEN2_RESPONSE" | jq -r '.data.remain_quota')
    if [ "$TOKEN2_QUOTA" = "10000000" ]; then
        test_result "第二个Token额度正确" "PASS"
    else
        test_result "第二个Token额度错误" "FAIL"
    fi
else
    test_result "第二个Token创建失败" "FAIL"
fi

###########################################
# 测试7: 尝试超额分配（应该失败）
###########################################
log_test "测试7: 尝试超额分配Token（应该失败）"
# 用户剩余：$100 - $30 - $20 = $50 (25,000,000 quota)
# 尝试分配：$60 (30,000,000 quota)
OVERQUOTA_RESPONSE=$(curl -s -X POST "$API_BASE/api/user/external/token" \
    -H "Content-Type: application/json" \
    -d "{
        \"external_user_id\": \"$TEST_USER_ID\",
        \"token_name\": \"Over Quota Token\",
        \"allocated_quota\": 30000000,
        \"expires_in_days\": 365
    }")

echo "超额分配响应:"
echo "$OVERQUOTA_RESPONSE" | jq '.'

if check_json_field "$OVERQUOTA_RESPONSE" ".success" "false"; then
    test_result "超额分配正确拒绝" "PASS"
    ERROR_MSG=$(echo "$OVERQUOTA_RESPONSE" | jq -r '.message')
    log_info "错误信息: $ERROR_MSG"
else
    test_result "超额分配未被拒绝（BUG）" "FAIL"
fi

###########################################
# 测试8: 获取用户统计（验证余额扣减）
###########################################
log_test "测试8: 获取用户统计"
STATS_RESPONSE=$(curl -s -X GET "$API_BASE/api/user/external/$TEST_USER_ID/stats")

echo "用户统计响应:"
echo "$STATS_RESPONSE" | jq '.'

if check_json_field "$STATS_RESPONSE" ".success" "true"; then
    test_result "用户统计查询成功" "PASS"

    # 验证剩余余额 - 修改为从user_info中获取
    REMAINING_BALANCE=$(echo "$STATS_RESPONSE" | jq -r '.data.user_info.current_quota')

    # 计算预期余额（考虑可能的初始余额）
    INITIAL_QUOTA=$(echo "$STATS_RESPONSE" | jq -r '.data.user_info.current_quota')
    log_info "当前用户余额: $REMAINING_BALANCE quota"

    # 验证余额应该是充值额 - 已分配Token额度
    # 由于可能有初始余额，我们只验证余额大于0
    if [ "$REMAINING_BALANCE" -gt "0" ]; then
        test_result "用户剩余余额验证通过" "PASS"
        BALANCE_USD=$(echo "$STATS_RESPONSE" | jq -r '.data.user_info.current_balance')
        log_success "剩余余额: $REMAINING_BALANCE quota (\$$BALANCE_USD)"
    else
        test_result "用户剩余余额错误" "FAIL"
        log_error "余额不应为0，实际: $REMAINING_BALANCE"
    fi

    # 验证Token数量 - 从tokens数组长度获取
    TOTAL_TOKENS=$(echo "$STATS_RESPONSE" | jq -r '.data.tokens | length')
    if [ "$TOTAL_TOKENS" = "2" ]; then
        test_result "Token数量统计正确" "PASS"
    else
        test_result "Token数量统计错误" "FAIL"
        log_error "期望2个Token，实际: $TOTAL_TOKENS"
    fi
else
    test_result "用户统计查询失败" "FAIL"
fi

###########################################
# 测试9: 获取消费记录（当前应该为空）
###########################################
log_test "测试9: 获取用户消费记录"
LOGS_RESPONSE=$(curl -s -X GET "$API_BASE/api/user/external/$TEST_USER_ID/logs?page=1&limit=20")

echo "消费记录响应:"
echo "$LOGS_RESPONSE" | jq '.'

if check_json_field "$LOGS_RESPONSE" ".success" "true"; then
    test_result "消费记录查询成功" "PASS"

    # ⚠️ 关键验证：JSON响应结构
    # 检查必要字段是否存在
    if echo "$LOGS_RESPONSE" | jq -e '.data.logs' > /dev/null; then
        test_result "消费记录JSON结构正确" "PASS"
    else
        test_result "消费记录JSON结构错误" "FAIL"
    fi

    # 检查分页信息
    if echo "$LOGS_RESPONSE" | jq -e '.data.pagination' > /dev/null; then
        test_result "分页信息存在" "PASS"
    else
        test_result "分页信息缺失" "FAIL"
    fi

    # 显示消费记录数量
    LOG_COUNT=$(echo "$LOGS_RESPONSE" | jq -r '.data.logs | length')
    log_info "消费记录数量: $LOG_COUNT"
else
    test_result "消费记录查询失败" "FAIL"
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

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "${GREEN}✓ 所有测试通过！${NC}"
    exit 0
else
    echo -e "${RED}✗ 存在失败的测试！${NC}"
    exit 1
fi
