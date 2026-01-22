#!/bin/bash

# 外部用户注销API测试脚本
# 测试 DELETE /api/user/external/:external_user_id

# 不使用 set -e，让测试继续执行

# 配置
API_BASE="${API_BASE:-http://localhost:3000}"
TEST_EXTERNAL_USER_ID="test_deactivate_$(date +%s)"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 计数器
TESTS_PASSED=0
TESTS_FAILED=0

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $1"
    ((TESTS_PASSED++))
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $1"
    ((TESTS_FAILED++))
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

# 检查JSON响应
check_response() {
    local response="$1"
    local expected_success="$2"
    local expected_message="$3"

    local success=$(echo "$response" | jq -r '.success')
    local message=$(echo "$response" | jq -r '.message')

    if [ "$success" = "$expected_success" ]; then
        if [ -n "$expected_message" ] && [[ "$message" != *"$expected_message"* ]]; then
            log_error "消息不匹配: 期望包含'$expected_message', 实际: '$message'"
            return 1
        fi
        return 0
    else
        log_error "success字段不匹配: 期望'$expected_success', 实际'$success'"
        return 1
    fi
}

echo "============================================"
echo "  外部用户注销API测试"
echo "============================================"
echo "API地址: $API_BASE"
echo "测试用户ID: $TEST_EXTERNAL_USER_ID"
echo ""

# ============================================
# 测试1: 创建测试用户
# ============================================
echo "--------------------------------------------"
echo "测试1: 创建测试用户"
echo "--------------------------------------------"

SYNC_RESPONSE=$(curl -s -X POST "$API_BASE/api/user/external/sync" \
    -H "Content-Type: application/json" \
    -d '{
        "external_user_id": "'"$TEST_EXTERNAL_USER_ID"'",
        "username": "test_deact_'"$(date +%s)"'",
        "display_name": "Test Deactivate User",
        "email": "test_'"$(date +%s)"'@example.com",
        "phone": ""
    }')

echo "响应: $SYNC_RESPONSE"

if check_response "$SYNC_RESPONSE" "true" ""; then
    USER_ID=$(echo "$SYNC_RESPONSE" | jq -r '.data.user_id')
    log_success "用户创建成功, user_id=$USER_ID"
else
    log_error "用户创建失败"
    echo "$SYNC_RESPONSE" | jq .
    exit 1
fi

# ============================================
# 测试2: 为用户充值
# ============================================
echo ""
echo "--------------------------------------------"
echo "测试2: 为用户充值"
echo "--------------------------------------------"

TOPUP_RESPONSE=$(curl -s -X POST "$API_BASE/api/user/external/topup" \
    -H "Content-Type: application/json" \
    -d '{
        "external_user_id": "'"$TEST_EXTERNAL_USER_ID"'",
        "amount_usd": 10.0,
        "payment_id": "test_payment_'"$(date +%s)"'"
    }')

echo "响应: $TOPUP_RESPONSE"

if check_response "$TOPUP_RESPONSE" "true" ""; then
    log_success "充值成功"
else
    log_error "充值失败"
fi

# ============================================
# 测试3: 创建Token
# ============================================
echo ""
echo "--------------------------------------------"
echo "测试3: 创建Token"
echo "--------------------------------------------"

TOKEN_RESPONSE=$(curl -s -X POST "$API_BASE/api/user/external/token" \
    -H "Content-Type: application/json" \
    -d '{
        "external_user_id": "'"$TEST_EXTERNAL_USER_ID"'",
        "token_name": "Test Deactivate Token",
        "allocated_quota": 1000000,
        "expires_in_days": 30
    }')

echo "响应: $TOKEN_RESPONSE"

if check_response "$TOKEN_RESPONSE" "true" ""; then
    TOKEN_ID=$(echo "$TOKEN_RESPONSE" | jq -r '.data.token_id')
    ACCESS_KEY=$(echo "$TOKEN_RESPONSE" | jq -r '.data.access_key')
    log_success "Token创建成功, token_id=$TOKEN_ID"
else
    log_error "Token创建失败"
fi

# ============================================
# 测试4: 注销用户不存在
# ============================================
echo ""
echo "--------------------------------------------"
echo "测试4: 注销不存在的用户"
echo "--------------------------------------------"

DELETE_NOTFOUND_RESPONSE=$(curl -s -X DELETE "$API_BASE/api/user/external/non_existent_user_12345")

echo "响应: $DELETE_NOTFOUND_RESPONSE"

if check_response "$DELETE_NOTFOUND_RESPONSE" "false" "用户不存在"; then
    log_success "正确返回用户不存在错误"
else
    log_error "应该返回用户不存在错误"
fi

# ============================================
# 测试5: 正常注销用户
# ============================================
echo ""
echo "--------------------------------------------"
echo "测试5: 正常注销用户"
echo "--------------------------------------------"

DELETE_RESPONSE=$(curl -s -X DELETE "$API_BASE/api/user/external/$TEST_EXTERNAL_USER_ID")

echo "响应: $DELETE_RESPONSE"

if check_response "$DELETE_RESPONSE" "true" "用户已注销"; then
    DELETED_EXTERNAL_USER_ID=$(echo "$DELETE_RESPONSE" | jq -r '.data.deleted_external_user_id')
    TOKENS_DISABLED=$(echo "$DELETE_RESPONSE" | jq -r '.data.tokens_disabled')
    log_success "用户注销成功"
    log_info "原始ID: $TEST_EXTERNAL_USER_ID"
    log_info "注销后ID: $DELETED_EXTERNAL_USER_ID"
    log_info "禁用Token数: $TOKENS_DISABLED"
else
    log_error "用户注销失败"
fi

# ============================================
# 测试6: 重复注销用户
# ============================================
echo ""
echo "--------------------------------------------"
echo "测试6: 重复注销已注销的用户"
echo "--------------------------------------------"

DELETE_AGAIN_RESPONSE=$(curl -s -X DELETE "$API_BASE/api/user/external/$TEST_EXTERNAL_USER_ID")

echo "响应: $DELETE_AGAIN_RESPONSE"

if check_response "$DELETE_AGAIN_RESPONSE" "false" "用户不存在"; then
    log_success "正确返回用户不存在（已注销）"
else
    log_error "应该返回用户不存在错误"
fi

# ============================================
# 测试7: 使用相同external_user_id重新注册
# ============================================
echo ""
echo "--------------------------------------------"
echo "测试7: 使用相同external_user_id重新注册"
echo "--------------------------------------------"

RESYNC_RESPONSE=$(curl -s -X POST "$API_BASE/api/user/external/sync" \
    -H "Content-Type: application/json" \
    -d '{
        "external_user_id": "'"$TEST_EXTERNAL_USER_ID"'",
        "username": "test_deact_new_'"$(date +%s)"'",
        "display_name": "New Test User",
        "email": "newtest_'"$(date +%s)"'@example.com"
    }')

echo "响应: $RESYNC_RESPONSE"

if check_response "$RESYNC_RESPONSE" "true" ""; then
    IS_NEW_USER=$(echo "$RESYNC_RESPONSE" | jq -r '.data.is_new_user')
    NEW_USER_ID=$(echo "$RESYNC_RESPONSE" | jq -r '.data.user_id')

    if [ "$IS_NEW_USER" = "true" ]; then
        log_success "重新注册成功，创建了新用户 (user_id=$NEW_USER_ID)"
    else
        log_warn "重新注册成功，但不是新用户 (user_id=$NEW_USER_ID)"
    fi

    if [ "$NEW_USER_ID" != "$USER_ID" ]; then
        log_success "新用户ID ($NEW_USER_ID) 与原用户ID ($USER_ID) 不同"
    else
        log_warn "新用户ID与原用户ID相同，可能复用了原账号"
    fi
else
    log_error "重新注册失败"
fi

# ============================================
# 测试8: 清理 - 注销重新注册的用户
# ============================================
echo ""
echo "--------------------------------------------"
echo "测试8: 清理 - 注销重新注册的用户"
echo "--------------------------------------------"

CLEANUP_RESPONSE=$(curl -s -X DELETE "$API_BASE/api/user/external/$TEST_EXTERNAL_USER_ID")

echo "响应: $CLEANUP_RESPONSE"

if check_response "$CLEANUP_RESPONSE" "true" "用户已注销"; then
    log_success "清理成功"
else
    log_warn "清理失败（可能用户已不存在）"
fi

# ============================================
# 测试结果汇总
# ============================================
echo ""
echo "============================================"
echo "  测试结果汇总"
echo "============================================"
echo -e "通过: ${GREEN}$TESTS_PASSED${NC}"
echo -e "失败: ${RED}$TESTS_FAILED${NC}"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}所有测试通过！${NC}"
    exit 0
else
    echo -e "${RED}有测试失败，请检查！${NC}"
    exit 1
fi
