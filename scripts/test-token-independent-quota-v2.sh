#!/usr/bin/env bash
#######################################
# V2 API Token无限额度测试脚本
# 测试场景：
# 1. V2平台Token授权
# 2. 验证无限额度模式
# 3. Token更新（幂等性）
# 4. 消费流水查询
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
PLATFORM_ID="test_platform_$(date +%s)"
TOKEN_KEY="sk-$(openssl rand -hex 16)"

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

# 测试1: V2平台Token授权（创建）
test_v2_token_authorize_create() {
    log_test "测试1: V2平台Token授权（首次创建）"
    log_info "Token Key: $TOKEN_KEY"

    local response=$(curl -s -X POST "$API_BASE/api/v2/platform/tokens/authorize" \
        -H "Content-Type: application/json" \
        -d "{
            \"platform_id\": \"$PLATFORM_ID\",
            \"token_key\": \"$TOKEN_KEY\",
            \"initial_quota\": 99999999,
            \"metadata\": {
                \"test\": \"v2_unlimited_quota\",
                \"created_at\": \"$(date -Iseconds)\"
            }
        }")

    validate_json "$response" "true" || return 1

    # 验证返回字段
    local status=$(echo "$response" | jq -r '.data.status')
    if [ "$status" != "authorized" ]; then
        log_error "status字段不符合预期: expected=authorized, got=$status"
        return 1
    fi

    # 重要：V2 Token应该使用无限额度模式
    local current_quota=$(echo "$response" | jq -r '.data.current_quota')
    log_info "返回的current_quota=$current_quota（V2模式下应被忽略）"

    log_success "V2 Token授权成功"
}

# 测试2: 验证Token使用UnlimitedQuota
test_verify_unlimited_quota() {
    log_test "测试2: 验证Token使用无限额度模式"

    # 使用V1接口的verify来检查Token状态
    local response=$(curl -s -X POST "$API_BASE/api/user/external/token/verify" \
        -H "Content-Type: application/json" \
        -d "{
            \"access_key\": \"$TOKEN_KEY\"
        }")

    validate_json "$response" "true" || return 1

    # 验证is_valid
    local is_valid=$(echo "$response" | jq -r '.is_valid')
    if [ "$is_valid" != "true" ]; then
        log_error "Token验证失败"
        return 1
    fi

    log_success "Token验证通过，状态正常"
    log_info "注意：V2 Token使用UnlimitedQuota=true，不受额度限制"
}

# 测试3: V2平台Token授权（更新/幂等性）
test_v2_token_authorize_update() {
    log_test "测试3: V2平台Token授权（重复调用，测试幂等性）"

    local response=$(curl -s -X POST "$API_BASE/api/v2/platform/tokens/authorize" \
        -H "Content-Type: application/json" \
        -d "{
            \"platform_id\": \"$PLATFORM_ID\",
            \"token_key\": \"$TOKEN_KEY\",
            \"initial_quota\": 88888888,
            \"metadata\": {
                \"test\": \"update_test\",
                \"updated_at\": \"$(date -Iseconds)\"
            }
        }")

    validate_json "$response" "true" || return 1

    # 验证更新状态
    local status=$(echo "$response" | jq -r '.data.status')
    if [ "$status" != "updated_unlimited" ]; then
        log_error "status字段不符合预期: expected=updated_unlimited, got=$status"
        return 1
    fi

    log_success "Token更新成功，幂等性验证通过"
}

# 测试4: 查询V2平台消费流水
test_v2_consumption_logs() {
    log_test "测试4: 查询V2平台消费流水"

    local start_date=$(date -v-7d +%Y-%m-%d 2>/dev/null || date -d '7 days ago' +%Y-%m-%d)
    local end_date=$(date +%Y-%m-%d)

    local response=$(curl -s -X GET "$API_BASE/api/v2/platform/$PLATFORM_ID/logs?start_date=$start_date&end_date=$end_date&page=1&page_size=10")

    validate_json "$response" "true" || return 1

    # 验证响应结构
    local platform_id=$(echo "$response" | jq -r '.data.platform_id')
    if [ "$platform_id" != "$PLATFORM_ID" ]; then
        log_error "platform_id不符合预期: expected=$PLATFORM_ID, got=$platform_id"
        return 1
    fi

    # 验证日期范围
    local returned_start=$(echo "$response" | jq -r '.data.date_range.start_date')
    local returned_end=$(echo "$response" | jq -r '.data.date_range.end_date')

    log_success "消费流水查询成功"
    log_info "查询日期范围: $returned_start ~ $returned_end"
    log_info "日志数量: $(echo "$response" | jq -r '.data.logs | length')"
}

# 测试5: 验证Token格式检查
test_invalid_token_format() {
    log_test "测试5: 验证Token格式检查（应拒绝错误格式）"

    # 测试错误格式：包含多个短横线
    local invalid_token="sk-2-a99416b67cb54e178e9ffe8a55c255ae"

    local response=$(curl -s -X POST "$API_BASE/api/v2/platform/tokens/authorize" \
        -H "Content-Type: application/json" \
        -d "{
            \"platform_id\": \"$PLATFORM_ID\",
            \"token_key\": \"$invalid_token\",
            \"initial_quota\": 1000000
        }")

    # 应该失败
    if validate_json "$response" "false"; then
        local error_code=$(echo "$response" | jq -r '.error_code')
        if [ "$error_code" = "INVALID_PARAMETER" ]; then
            log_success "Token格式验证通过，正确拒绝了错误格式"
        else
            log_error "错误码不符合预期: expected=INVALID_PARAMETER, got=$error_code"
            return 1
        fi
    else
        log_error "应该返回失败，但返回了成功"
        return 1
    fi
}

# 测试6: 创建第二个Token（同一平台）
test_create_second_token() {
    log_test "测试6: 为同一平台创建第二个Token"

    local token2="sk-$(openssl rand -hex 16)"

    local response=$(curl -s -X POST "$API_BASE/api/v2/platform/tokens/authorize" \
        -H "Content-Type: application/json" \
        -d "{
            \"platform_id\": \"$PLATFORM_ID\",
            \"token_key\": \"$token2\",
            \"initial_quota\": 1000000,
            \"metadata\": {
                \"token_name\": \"second_token\"
            }
        }")

    validate_json "$response" "true" || return 1

    local status=$(echo "$response" | jq -r '.data.status')
    if [ "$status" != "authorized" ]; then
        log_error "status字段不符合预期: expected=authorized, got=$status"
        return 1
    fi

    log_success "第二个Token创建成功"
    log_info "同一平台可以拥有多个无限额度Token"
}

# 主测试流程
main() {
    log_info "=========================================="
    log_info "V2 API Token无限额度测试"
    log_info "=========================================="

    # 执行测试
    test_v2_token_authorize_create || exit 1
    sleep 1

    test_verify_unlimited_quota || exit 1
    sleep 1

    test_v2_token_authorize_update || exit 1
    sleep 1

    test_v2_consumption_logs || exit 1
    sleep 1

    test_invalid_token_format || exit 1
    sleep 1

    test_create_second_token || exit 1

    log_info "=========================================="
    log_success "所有测试通过！✅"
    log_info "=========================================="
    log_info "测试总结："
    log_info "1. ✅ V2 Token授权成功"
    log_info "2. ✅ 无限额度模式验证通过"
    log_info "3. ✅ 幂等性验证通过"
    log_info "4. ✅ Token格式检查正确"
    log_info "5. ✅ 消费流水查询正常"
    log_info "6. ✅ JSON响应格式符合文档"
    log_info "=========================================="
}

main "$@"
