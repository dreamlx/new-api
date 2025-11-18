#!/usr/bin/env bash
#######################################
# V1 API Token独立额度测试脚本
# 测试场景：
# 1. 用户同步
# 2. 用户充值
# 3. Token创建（额度分配）
# 4. 验证额度独立性
# 5. Token消费记录查询
#######################################

set -euo pipefail

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# API配置
API_BASE="http://localhost:3000"
EXTERNAL_USER_ID="test_quota_user_$(date +%s)"

# 日志函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $*"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $*"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $*"
}

log_test() {
    echo -e "${YELLOW}[TEST]${NC} $*"
}

# 验证JSON响应
validate_json() {
    local response="$1"
    local expected_success="$2"

    if ! echo "$response" | jq . >/dev/null 2>&1; then
        log_error "无效的JSON响应"
        echo "$response"
        return 1
    fi

    local success=$(echo "$response" | jq -r '.success')
    if [ "$success" != "$expected_success" ]; then
        log_error "success字段不符合预期: expected=$expected_success, got=$success"
        echo "$response" | jq .
        return 1
    fi

    log_success "JSON格式验证通过"
    echo "$response" | jq .
    return 0
}

# 测试1: 用户同步
test_user_sync() {
    log_test "测试1: 用户同步"

    local response=$(curl -s -X POST "$API_BASE/api/user/external/sync" \
        -H "Content-Type: application/json" \
        -d "{
            \"external_user_id\": \"$EXTERNAL_USER_ID\",
            \"display_name\": \"测试用户-额度独立\",
            \"email\": \"test_quota@example.com\"
        }")

    validate_json "$response" "true" || return 1

    # 验证返回字段
    local user_id=$(echo "$response" | jq -r '.data.user_id')
    if [ -z "$user_id" ] || [ "$user_id" = "null" ]; then
        log_error "user_id字段缺失"
        return 1
    fi

    echo "$user_id" > /tmp/test_user_id.txt
    log_success "用户同步成功，user_id=$user_id"
}

# 测试2: 用户充值
test_user_topup() {
    log_test "测试2: 用户充值 \$100 USD"

    local response=$(curl -s -X POST "$API_BASE/api/user/external/topup" \
        -H "Content-Type: application/json" \
        -d "{
            \"external_user_id\": \"$EXTERNAL_USER_ID\",
            \"amount_usd\": 100,
            \"payment_id\": \"test_payment_$(date +%s)\"
        }")

    validate_json "$response" "true" || return 1

    # 验证额度
    local current_quota=$(echo "$response" | jq -r '.data.current_quota')
    local expected_quota=50000000  # $100 * 500,000

    if [ "$current_quota" != "$expected_quota" ]; then
        log_error "充值额度不符合预期: expected=$expected_quota, got=$current_quota"
        return 1
    fi

    log_success "充值成功，当前额度=$current_quota"
}

# 测试3: 创建Token1 - 分配$30
test_create_token1() {
    log_test "测试3: 创建Token1，分配 \$30 额度"

    local allocated_quota=15000000  # $30 * 500,000

    local response=$(curl -s -X POST "$API_BASE/api/user/external/token" \
        -H "Content-Type: application/json" \
        -d "{
            \"external_user_id\": \"$EXTERNAL_USER_ID\",
            \"token_name\": \"测试Token1\",
            \"allocated_quota\": $allocated_quota,
            \"expires_in_days\": 365
        }")

    validate_json "$response" "true" || return 1

    # 验证Token额度
    local remain_quota=$(echo "$response" | jq -r '.data.remain_quota')
    if [ "$remain_quota" != "$allocated_quota" ]; then
        log_error "Token额度不符合预期: expected=$allocated_quota, got=$remain_quota"
        return 1
    fi

    # 保存Token
    local access_key=$(echo "$response" | jq -r '.data.access_key')
    echo "$access_key" > /tmp/test_token1.txt

    log_success "Token1创建成功，access_key=$access_key, quota=$remain_quota"
}

# 测试4: 创建Token2 - 分配$40
test_create_token2() {
    log_test "测试4: 创建Token2，分配 \$40 额度"

    local allocated_quota=20000000  # $40 * 500,000

    local response=$(curl -s -X POST "$API_BASE/api/user/external/token" \
        -H "Content-Type: application/json" \
        -d "{
            \"external_user_id\": \"$EXTERNAL_USER_ID\",
            \"token_name\": \"测试Token2\",
            \"allocated_quota\": $allocated_quota,
            \"expires_in_days\": 365
        }")

    validate_json "$response" "true" || return 1

    # 验证Token额度
    local remain_quota=$(echo "$response" | jq -r '.data.remain_quota')
    if [ "$remain_quota" != "$allocated_quota" ]; then
        log_error "Token额度不符合预期: expected=$allocated_quota, got=$remain_quota"
        return 1
    fi

    # 保存Token
    local access_key=$(echo "$response" | jq -r '.data.access_key')
    echo "$access_key" > /tmp/test_token2.txt

    log_success "Token2创建成功，access_key=$access_key, quota=$remain_quota"
}

# 测试5: 验证User余额扣减
test_user_quota_deduction() {
    log_test "测试5: 验证User余额扣减（应该剩余 \$30）"

    local response=$(curl -s -X GET "$API_BASE/api/user/external/$EXTERNAL_USER_ID/stats")

    validate_json "$response" "true" || return 1

    # 验证剩余额度
    local current_quota=$(echo "$response" | jq -r '.data.quota')
    local expected_quota=15000000  # $100 - $30 - $40 = $30

    if [ "$current_quota" != "$expected_quota" ]; then
        log_error "User剩余额度不符合预期: expected=$expected_quota, got=$current_quota"
        return 1
    fi

    log_success "User余额验证通过，剩余=$current_quota (约 \$30)"
}

# 测试6: 尝试创建Token3 - 超额（应失败）
test_create_token3_overflow() {
    log_test "测试6: 尝试创建Token3，分配 \$50（应失败，余额不足）"

    local allocated_quota=25000000  # $50 * 500,000

    local response=$(curl -s -X POST "$API_BASE/api/user/external/token" \
        -H "Content-Type: application/json" \
        -d "{
            \"external_user_id\": \"$EXTERNAL_USER_ID\",
            \"token_name\": \"测试Token3\",
            \"allocated_quota\": $allocated_quota,
            \"expires_in_days\": 365
        }")

    # 应该失败
    if validate_json "$response" "false"; then
        local message=$(echo "$response" | jq -r '.message')
        if [[ "$message" == *"余额不足"* ]]; then
            log_success "余额不足检查通过，正确拒绝了超额分配"
        else
            log_error "错误消息不符合预期: $message"
            return 1
        fi
    else
        log_error "应该返回失败，但返回了成功"
        return 1
    fi
}

# 测试7: 查询Token列表
test_list_tokens() {
    log_test "测试7: 查询用户Token列表"

    local response=$(curl -s -X GET "$API_BASE/api/user/external/$EXTERNAL_USER_ID/tokens")

    validate_json "$response" "true" || return 1

    # 验证Token数量
    local token_count=$(echo "$response" | jq -r '.data.total_tokens')
    if [ "$token_count" != "2" ]; then
        log_error "Token数量不符合预期: expected=2, got=$token_count"
        return 1
    fi

    log_success "Token列表查询成功，总数=$token_count"
}

# 测试8: 验证Token独立额度
test_verify_token_independence() {
    log_test "测试8: 验证Token额度独立性"

    local token1=$(cat /tmp/test_token1.txt)

    local response=$(curl -s -X POST "$API_BASE/api/user/external/token/verify" \
        -H "Content-Type: application/json" \
        -d "{
            \"access_key\": \"$token1\"
        }")

    validate_json "$response" "true" || return 1

    # 验证Token1仍有$30额度
    local remain_quota=$(echo "$response" | jq -r '.remain_quota')
    local expected_quota=15000000  # $30

    if [ "$remain_quota" != "$expected_quota" ]; then
        log_error "Token1额度不符合预期: expected=$expected_quota, got=$remain_quota"
        return 1
    fi

    log_success "Token独立额度验证通过，Token1额度=$remain_quota"
}

# 测试9: 查询消费记录
test_consumption_logs() {
    log_test "测试9: 查询用户消费记录"

    local response=$(curl -s -X GET "$API_BASE/api/user/external/$EXTERNAL_USER_ID/logs?page=1&limit=10")

    validate_json "$response" "true" || return 1

    # 验证日志结构
    local logs=$(echo "$response" | jq -r '.data.logs')
    if [ "$logs" = "null" ]; then
        log_error "logs字段缺失"
        return 1
    fi

    log_success "消费记录查询成功"
    echo "$response" | jq '.data.logs[] | {time, type, content, spend}' | head -20
}

# 主测试流程
main() {
    log_info "=========================================="
    log_info "V1 API Token独立额度测试"
    log_info "=========================================="

    # 执行测试
    test_user_sync || exit 1
    sleep 1

    test_user_topup || exit 1
    sleep 1

    test_create_token1 || exit 1
    sleep 1

    test_create_token2 || exit 1
    sleep 1

    test_user_quota_deduction || exit 1
    sleep 1

    test_create_token3_overflow || exit 1
    sleep 1

    test_list_tokens || exit 1
    sleep 1

    test_verify_token_independence || exit 1
    sleep 1

    test_consumption_logs || exit 1

    log_info "=========================================="
    log_success "所有测试通过！✅"
    log_info "=========================================="
    log_info "测试总结："
    log_info "1. ✅ Token独立额度分配正确"
    log_info "2. ✅ User余额正确扣减"
    log_info "3. ✅ 超额分配被正确拒绝"
    log_info "4. ✅ Token额度相互独立"
    log_info "5. ✅ JSON响应格式符合文档"
    log_info "=========================================="

    # 清理
    rm -f /tmp/test_user_id.txt /tmp/test_token1.txt /tmp/test_token2.txt
}

main "$@"
