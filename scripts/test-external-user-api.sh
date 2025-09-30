#!/bin/bash

# 外部用户API自动化测试脚本
# 基于 docs/external-user-api.md 和 docs/curl-testing-guide.md

set -e  # 遇到错误立即退出

API_URL="${API_URL:-http://localhost:3000}"
EXTERNAL_USER_ID="test_auto_$(date +%s)"
USERNAME="auto_user_$(date +%s)"
EMAIL="${USERNAME}@test.local"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 测试计数器
TESTS_PASSED=0
TESTS_FAILED=0
TESTS_TOTAL=0

# 日志函数
log_info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

log_success() {
    echo -e "${GREEN}✓${NC} $1"
    TESTS_PASSED=$((TESTS_PASSED + 1))
}

log_error() {
    echo -e "${RED}✗${NC} $1"
    TESTS_FAILED=$((TESTS_FAILED + 1))
}

log_warning() {
    echo -e "${YELLOW}⚠${NC} $1"
}

# 测试函数
test_api() {
    local test_name="$1"
    local method="$2"
    local endpoint="$3"
    local data="$4"
    local expected_field="$5"

    TESTS_TOTAL=$((TESTS_TOTAL + 1))

    log_info "测试: $test_name"

    local response
    if [ "$method" = "GET" ]; then
        response=$(curl -s -X GET "$API_URL$endpoint" 2>&1)
    elif [ "$method" = "DELETE" ]; then
        response=$(curl -s -X DELETE "$API_URL$endpoint" \
            -H "Content-Type: application/json" \
            -d "$data" 2>&1)
    else
        response=$(curl -s -X POST "$API_URL$endpoint" \
            -H "Content-Type: application/json" \
            -d "$data" 2>&1)
    fi

    # 检查响应
    if echo "$response" | grep -q "$expected_field"; then
        log_success "$test_name - 通过"
        echo "$response" | python3 -m json.tool 2>/dev/null || echo "$response"
        echo ""
        return 0
    else
        log_error "$test_name - 失败"
        echo "响应: $response"
        echo ""
        return 1
    fi
}

# 开始测试
echo ""
echo "================================================"
echo "  外部用户API自动化测试"
echo "================================================"
echo ""
log_info "API地址: $API_URL"
log_info "测试用户ID: $EXTERNAL_USER_ID"
echo ""

# 检查API是否可访问（跳过检查，直接测试）
log_info "目标API: $API_URL"
echo ""

# 1. 测试用户同步API
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  1. 用户同步API测试"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

test_api \
    "创建新用户" \
    "POST" \
    "/api/user/external/sync" \
    "{\"external_user_id\":\"$EXTERNAL_USER_ID\",\"username\":\"$USERNAME\",\"display_name\":\"Auto Test User\",\"email\":\"$EMAIL\",\"wechat_unionid\":\"oUnion_auto_test\",\"login_type\":\"email\"}" \
    "\"is_new_user\":true"

USER_ID=$(curl -s -X POST "$API_URL/api/user/external/sync" \
    -H "Content-Type: application/json" \
    -d "{\"external_user_id\":\"$EXTERNAL_USER_ID\",\"username\":\"$USERNAME\",\"email\":\"$EMAIL\"}" \
    2>/dev/null | grep -o '"user_id":[0-9]*' | cut -d: -f2)

log_info "获取到User ID: $USER_ID"
echo ""

# 2. 测试充值API
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  2. 充值API测试"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

test_api \
    "用户充值 \$10" \
    "POST" \
    "/api/user/external/topup" \
    "{\"external_user_id\":\"$EXTERNAL_USER_ID\",\"amount_usd\":10.0,\"payment_id\":\"test_payment_$(date +%s)\"}" \
    "\"quota_added\":5000000"

# 3. 测试Token创建
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  3. Token管理API测试"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

TOKEN_RESPONSE=$(curl -s -X POST "$API_URL/api/user/external/token" \
    -H "Content-Type: application/json" \
    -d "{\"external_user_id\":\"$EXTERNAL_USER_ID\",\"token_name\":\"Auto Test Token\",\"expires_in_days\":30}")

TOKEN_ID=$(echo "$TOKEN_RESPONSE" | grep -o '"token_id":[0-9]*' | cut -d: -f2)
ACCESS_KEY=$(echo "$TOKEN_RESPONSE" | grep -o '"access_key":"[^"]*"' | cut -d'"' -f4)

if [ -n "$TOKEN_ID" ] && [ -n "$ACCESS_KEY" ]; then
    log_success "Token创建成功"
    log_info "Token ID: $TOKEN_ID"
    log_info "Access Key: $ACCESS_KEY"
    TESTS_PASSED=$((TESTS_PASSED + 1))
else
    log_error "Token创建失败"
    TESTS_FAILED=$((TESTS_FAILED + 1))
fi
TESTS_TOTAL=$((TESTS_TOTAL + 1))
echo ""

# 4. 测试用户统计API
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  4. 用户统计API测试"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

test_api \
    "获取用户统计" \
    "GET" \
    "/api/user/external/$EXTERNAL_USER_ID/stats" \
    "" \
    "\"external_user_id\""

# 5. 测试消费记录API
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  5. 消费记录API测试"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

test_api \
    "查询消费记录" \
    "GET" \
    "/api/user/external/$EXTERNAL_USER_ID/logs" \
    "" \
    "\"logs\""

# 6. 测试Token删除
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  6. Token删除API测试"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

if [ -n "$TOKEN_ID" ]; then
    test_api \
        "删除Token" \
        "DELETE" \
        "/api/user/external/token" \
        "{\"external_user_id\":\"$EXTERNAL_USER_ID\",\"token_id\":$TOKEN_ID}" \
        "\"success\":true"
else
    log_warning "跳过Token删除测试（Token ID不存在）"
fi

# 7. 测试不支持wechat_openid（负面测试）
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  7. 参数验证测试（负面测试）"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

TESTS_TOTAL=$((TESTS_TOTAL + 1))
log_info "测试: 不支持wechat_openid参数"

FAIL_RESPONSE=$(curl -s -X POST "$API_URL/api/user/external/topup" \
    -H "Content-Type: application/json" \
    -d '{"wechat_openid":"oTest123","amount_usd":5.0,"payment_id":"test_fail"}' 2>&1)

if echo "$FAIL_RESPONSE" | grep -iq "error\|invalid\|required\|external_user_id"; then
    log_success "正确拒绝wechat_openid参数"
    echo "响应: $FAIL_RESPONSE" | python3 -m json.tool 2>/dev/null || echo "$FAIL_RESPONSE"
else
    log_error "错误：仍然接受wechat_openid参数"
    echo "响应: $FAIL_RESPONSE"
fi
echo ""

# 测试总结
echo "================================================"
echo "  测试总结"
echo "================================================"
echo ""
echo "总测试数: $TESTS_TOTAL"
echo -e "${GREEN}通过: $TESTS_PASSED${NC}"
echo -e "${RED}失败: $TESTS_FAILED${NC}"
echo ""

if [ $TESTS_FAILED -eq 0 ]; then
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${GREEN}  ✓ 所有测试通过！${NC}"
    echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    exit 0
else
    echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${RED}  ✗ 部分测试失败！${NC}"
    echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    exit 1
fi