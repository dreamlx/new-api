#!/bin/bash

# Wisemodel MaaS API 自动化测试脚本
# 版本: v1.0
# 最后更新: 2025-01-21

set -e

# 配置
BASE_URL="${BASE_URL:-http://192.168.1.100:3000}"
TOKEN="${WISEMODEL_API_TOKEN:-test_wisemodel_token_12345}"

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 测试计数
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# 辅助函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[PASS]${NC} $1"
    PASSED_TESTS=$((PASSED_TESTS + 1))
}

log_error() {
    echo -e "${RED}[FAIL]${NC} $1"
    FAILED_TESTS=$((FAILED_TESTS + 1))
}

log_warning() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

test_start() {
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    log_info "测试 #$TOTAL_TESTS: $1"
}

# 检查JSON响应
check_json_field() {
    local response="$1"
    local field="$2"
    local expected="$3"

    local actual=$(echo "$response" | jq -r "$field")
    if [ "$actual" = "$expected" ]; then
        return 0
    else
        log_warning "  期望: $expected, 实际: $actual"
        return 1
    fi
}

# 检查服务状态
check_service() {
    log_info "检查服务状态..."
    if curl -s "$BASE_URL/api/status" > /dev/null 2>&1; then
        log_success "服务运行正常"
        return 0
    else
        log_error "服务未运行，请先启动服务"
        exit 1
    fi
}

# 生成唯一手机号
generate_phone() {
    #echo "139$(date +%s | tail -c 9)"
    echo "18301852832"
    #echo "18531606721"
    # echo "15321866239"

}

# 测试1：用户绑定
test_user_bind() {
    test_start "用户绑定 - 新用户"

    PHONE=$(generate_phone)
    WM_KEY="wm_key_test_$(date +%s)"
    # WM_KEY="wisemodel-iygugqeusgbodidvxxgl"
    echo "$PHONE" > /tmp/wisemodel_test_phone.txt
    echo "$WM_KEY" > /tmp/wisemodel_test_wm_key.txt
    # RESPONSE=$(curl -s -X POST "$BASE_URL/api/wisemodel/user/bind" \
    #     -H "Authorization: Bearer $TOKEN" \
    #     -H "Content-Type: application/json" \
    #     -d "{
    #         \"phone\": \"$PHONE\",
    #         \"wisemodel_key\": \"$WM_KEY\",
    #         \"username\": \"测试用户_$(date +%s)\"
    #     }")

    # if check_json_field "$RESPONSE" ".success" "true"; then
    #     log_success "  用户绑定成功"
    #     echo "$PHONE" > /tmp/wisemodel_test_phone.txt
    #     echo "$WM_KEY" > /tmp/wisemodel_test_wm_key.txt
    # else
    #     log_error "  用户绑定失败"
    #     echo "  响应: $RESPONSE"
    # fi
}

# 测G试2：更新Wisemodel Key
test_update_key() {
    test_start "更新Wisemodel Key"

    PHONE=$(cat /tmp/wisemodel_test_phone.txt)
    NEW_KEY="wm_key_updated_$(date +%s)"
    # NEW_KEY="wisemodel-bvwulxeoviypfmzfvrwj"
    RESPONSE=$(curl -s -X POST "$BASE_URL/api/wisemodel/user/update_wisemodel_key" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"phone\": \"$PHONE\",
            \"new_key\": \"$NEW_KEY\"
        }")

    if check_json_field "$RESPONSE" ".success" "true"; then
        log_success "  Key更新成功"
        echo "$NEW_KEY" > /tmp/wisemodel_test_wm_key.txt
    else
        log_error "  Key更新失败"
        echo "  响应: $RESPONSE"
    fi
}

# 测试3：创建订单 - 积分模式
test_create_order_points() {
    test_start "创建订单 - 积分模式"

    PHONE=$(cat /tmp/wisemodel_test_phone.txt)
    ORDER_ID="ORDER_TEST_$(date +%s)"
    PACKAGE_ID="package16"

    RESPONSE=$(curl -s -X POST "$BASE_URL/api/wisemodel/orders/record" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"order_id\": \"$ORDER_ID\",
            \"package_count\": 1,
            \"packages\": [
                {
                    \"id\": \"$PACKAGE_ID\",
                    \"points\": 1000000000,
                    \"tokens\": 0,
                    \"amount\": 1.00,
                    \"phone\": \"$PHONE\",
                    \"is_free\": false,
                    \"valid_until\": \"2027-12-31T23:59:59Z\",
                    \"created_at\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\"
                }
            ]
        }")

    if check_json_field "$RESPONSE" ".success" "true"; then
        log_success "  订单创建成功 (积分模式: 10 points)"
        echo "$PACKAGE_ID" > /tmp/wisemodel_test_package_id.txt
    else
        log_error "  订单创建失败"
        echo "  响应: $RESPONSE"
    fi
}

# 测试4：创建订单 - Token模式
test_create_order_tokens() {
    test_start "创建订单 - Token模式"

    PHONE=$(cat /tmp/wisemodel_test_phone.txt)
    ORDER_ID="ORDER_TEST_$(date +%s)"
    PACKAGE_ID="PKG_TEST_$(date +%s)"

    RESPONSE=$(curl -s -X POST "$BASE_URL/api/wisemodel/orders/record" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"order_id\": \"$ORDER_ID\",
            \"package_count\": 1,
            \"packages\": [
                {
                    \"id\": \"$PACKAGE_ID\",
                    \"points\": 0,
                    \"tokens\": 1000000000,
                    \"amount\": 1.00,
                    \"phone\": \"$PHONE\",
                    \"is_free\": false,
                    \"valid_until\": \"2027-12-31T23:59:59Z\",
                    \"created_at\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\"
                }
            ]
        }")

    if check_json_field "$RESPONSE" ".success" "true"; then
        log_success "  订单创建成功 (Token模式: 20 tokens)"
    else
        log_error "  订单创建失败"
        echo "  响应: $RESPONSE"
    fi
}

# 测试5：创建订单 - 免费资源包
test_create_order_free() {
    test_start "创建订单 - 免费资源包"

    PHONE=$(cat /tmp/wisemodel_test_phone.txt)
    ORDER_ID="ORDER_FREE_$(date +%s)"
    PACKAGE_ID="PKG_FREE_$(date +%s)"

    RESPONSE=$(curl -s -X POST "$BASE_URL/api/wisemodel/orders/record" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"order_id\": \"$ORDER_ID\",
            \"package_count\": 1,
            \"packages\": [
                {
                    \"id\": \"$PACKAGE_ID\",
                    \"points\": 5,
                    \"tokens\": 0,
                    \"amount\": 0.00,
                    \"phone\": \"$PHONE\",
                    \"is_free\": true,
                    \"valid_until\": \"2027-12-31T23:59:59Z\",
                    \"created_at\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\"
                }
            ]
        }")

    if check_json_field "$RESPONSE" ".success" "true"; then
        log_success "  免费资源包创建成功"
    else
        log_error "  免费资源包创建失败"
        echo "  响应: $RESPONSE"
    fi
}

# 测试6：查询资源包使用情况
test_package_usage() {
    test_start "查询资源包使用情况"

    PHONE=$(cat /tmp/wisemodel_test_phone.txt)
    RESPONSE=$(curl -s -X POST "$BASE_URL/api/wisemodel/user/package_usage" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"phone\": \"$PHONE\"
        }")

    if check_json_field "$RESPONSE" ".code" "200"; then
        PACKAGE_COUNT=$(echo "$RESPONSE" | jq '.data | length')
        log_success "  查询成功，找到 $PACKAGE_COUNT 个资源包"

        # 验证可用模型
        FIRST_PKG=$(echo "$RESPONSE" | jq -r '.data[0].package_id')
        MODELS=$(echo "$RESPONSE" | jq -r '.data[0].available_models | length')
        log_info "  资源包ID: $FIRST_PKG, 可用模型数: $MODELS"
    else
        log_error "  查询失败"
        echo "  响应: $RESPONSE"
    fi
}

# 测试7：更新手机号
test_update_phone() {
    test_start "更新手机号"

    OLD_PHONE=$(cat /tmp/wisemodel_test_phone.txt)
    NEW_PHONE=$(generate_phone)

    RESPONSE=$(curl -s -X POST "$BASE_URL/api/wisemodel/user/update_phone" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"old_phone\": \"$OLD_PHONE\",
            \"new_phone\": \"$NEW_PHONE\"
        }")

    if check_json_field "$RESPONSE" ".success" "true"; then
        log_success "  手机号更新成功: $OLD_PHONE -> $NEW_PHONE"
        echo "$NEW_PHONE" > /tmp/wisemodel_test_phone.txt
    else
        log_error "  手机号更新失败"
        echo "  响应: $RESPONSE"
    fi
}

# 测试8：删除Wisemodel Key
test_delete_key() {
    test_start "删除Wisemodel Key"

    PHONE=$(cat /tmp/wisemodel_test_phone.txt)
    RESPONSE=$(curl -s -X POST "$BASE_URL/api/wisemodel/user/delete_wisemodel_key" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"phone\": \"$PHONE\"
        }")

    if check_json_field "$RESPONSE" ".success" "true"; then
        log_success "  Key删除成功"
    else
        log_error "  Key删除失败"
        echo "  响应: $RESPONSE"
    fi
}

# 测试9：错误处理 - 无效Token
test_invalid_token() {
    test_start "错误处理 - 无效Token"

    RESPONSE=$(curl -s -w "%{http_code}" -o /dev/null \
        -X POST "$BASE_URL/api/wisemodel/user/bind" \
        -H "Authorization: Bearer invalid_token_12345" \
        -H "Content-Type: application/json" \
        -d '{
            "phone": "13800000000",
            "wisemodel_key": "test",
            "username": "test"
        }')

    if [ "$RESPONSE" = "401" ]; then
        log_success "  正确返回401 Unauthorized"
    else
        log_error "  期望返回401，实际返回 $RESPONSE"
    fi
}

# 测试10：错误处理 - 用户不存在
test_user_not_found() {
    test_start "错误处理 - 用户不存在"

    RESPONSE=$(curl -s -X POST "$BASE_URL/api/wisemodel/user/delete_wisemodel_key" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d '{
            "phone": "99999999999"
        }')

    if check_json_field "$RESPONSE" ".success" "false"; then
        log_success "  正确返回用户不存在错误"
    else
        log_error "  错误处理失败"
        echo "  响应: $RESPONSE"
    fi
}

# 测试11：错误处理 - package_count不匹配
test_package_count_mismatch() {
    test_start "错误处理 - package_count不匹配"

    PHONE=$(cat /tmp/wisemodel_test_phone.txt)
    RESPONSE=$(curl -s -X POST "$BASE_URL/api/wisemodel/orders/record" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"order_id\": \"ORDER_ERROR_$(date +%s)\",
            \"package_count\": 3,
            \"packages\": [
                {
                    \"id\": \"PKG_ERROR\",
                    \"points\": 10,
                    \"tokens\": 0,
                    \"amount\": 10.00,
                    \"phone\": \"$PHONE\",
                    \"is_free\": false,
                    \"valid_until\": \"2027-12-31T23:59:59Z\",
                    \"created_at\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\"
                }
            ]
        }")

    if check_json_field "$RESPONSE" ".success" "false"; then
        log_success "  正确返回package_count不匹配错误"
    else
        log_error "  错误处理失败"
        echo "  响应: $RESPONSE"
    fi
}

# 测试12：多资源包订单
test_multiple_packages() {
    test_start "创建订单 - 多资源包"

    PHONE=$(cat /tmp/wisemodel_test_phone.txt)
    ORDER_ID="ORDER_MULTI_$(date +%s)"

    RESPONSE=$(curl -s -X POST "$BASE_URL/api/wisemodel/orders/record" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"order_id\": \"$ORDER_ID\",
            \"package_count\": 2,
            \"packages\": [
                {
                    \"id\": \"PKG_MULTI_1_$(date +%s)\",
                    \"points\": 15,
                    \"tokens\": 0,
                    \"amount\": 15.00,
                    \"phone\": \"$PHONE\",
                    \"is_free\": false,
                    \"valid_until\": \"2027-12-31T23:59:59Z\",
                    \"created_at\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\"
                },
                {
                    \"id\": \"PKG_MULTI_2_$(date +%s)\",
                    \"points\": 25,
                    \"tokens\": 0,
                    \"amount\": 25.00,
                    \"phone\": \"$PHONE\",
                    \"is_free\": false,
                    \"valid_until\": \"2027-12-31T23:59:59Z\",
                    \"created_at\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\"
                }
            ]
        }")

    if check_json_field "$RESPONSE" ".success" "true"; then
        log_success "  多资源包订单创建成功"
    else
        log_error "  多资源包订单创建失败"
        echo "  响应: $RESPONSE"
    fi
}

# 测试13：Chat接口调用（使用wisemodel_key作为Bearer Token）
test_chat_call() {
    test_start "Chat接口调用 - 使用wisemodel_key"

    if [ ! -f /tmp/wisemodel_test_wm_key.txt ]; then
        log_error "  未找到wisemodel_key，请先运行用户绑定测试"
        return
    fi

    WM_KEY=$(cat /tmp/wisemodel_test_wm_key.txt)
    CHAT_MODEL="${WISEMODEL_CHAT_MODEL:-minimax-m2.5-highspeed}"

    RESPONSE=$(curl -s -X POST "$BASE_URL/v1/chat/completions" \
        -H "Authorization: Bearer $WM_KEY" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"$CHAT_MODEL\",
            \"messages\": [
                {\"role\": \"user\", \"content\": \"Hello, reply with just OK, give me a funny story\"}
            ],
            \"max_tokens\": 1024
        }")

    CHOICES=$(echo "$RESPONSE" | jq '.choices | length' 2>/dev/null)
    if [ -n "$CHOICES" ] && [ "$CHOICES" -gt 0 ] 2>/dev/null; then
        CONTENT=$(echo "$RESPONSE" | jq -r '.choices[0].message.content // empty')
        PROMPT_TOKENS=$(echo "$RESPONSE" | jq -r '.usage.prompt_tokens // "N/A"')
        COMPLETION_TOKENS=$(echo "$RESPONSE" | jq -r '.usage.completion_tokens // "N/A"')
        log_success "  Chat调用成功"
        log_info "  模型: $CHAT_MODEL"
        log_info "  回复内容: $CONTENT"
        log_info "  Token使用: prompt=$PROMPT_TOKENS, completion=$COMPLETION_TOKENS"
    else
        log_error "  Chat调用失败"
        echo "  响应: $RESPONSE"
    fi
}

# 测试14：Chat后验证订单数据返回
test_verify_order_after_chat() {
    test_start "Chat后验证订单数据返回"

    PHONE=$(cat /tmp/wisemodel_test_phone.txt)
    RESPONSE=$(curl -s -X POST "$BASE_URL/api/wisemodel/user/package_usage" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"phone\": \"$PHONE\"
        }")

    log_info " $RESPONSE " 
    if check_json_field "$RESPONSE" ".code" "200"; then
        PACKAGE_COUNT=$(echo "$RESPONSE" | jq '.data | length')
        log_success "  订单数据返回成功，共 $PACKAGE_COUNT 条订单记录"

        # 遍历每个资源包，打印订单详情
        for i in $(seq 0 $((PACKAGE_COUNT - 1))); do
            PKG_ID=$(echo "$RESPONSE" | jq -r ".data[$i].package_id")
            DETAIL_COUNT=$(echo "$RESPONSE" | jq ".data[$i].details | length")

            # 积分包
            REMAIN_POINTS=$(echo "$RESPONSE" | jq -r ".data[$i].remain_points // empty")
            # Token包
            REMAIN_TOKENS=$(echo "$RESPONSE" | jq -r ".data[$i].remain_tokens // empty")
            

            if [ -n "$REMAIN_POINTS" ]; then
                TOTAL_POINTS=$(echo "$RESPONSE" | jq -r ".data[$i].points")
                USED=$(echo "$RESPONSE" | jq -r ".data[$i].amount")
                log_info "  [包$i] $PKG_ID: points=$TOTAL_POINTS, 已用=$USED, 剩余=$REMAIN_POINTS, 消费记录=$DETAIL_COUNT"
            elif [ -n "$REMAIN_TOKENS" ]; then
                TOTAL_TOKENS=$(echo "$RESPONSE" | jq -r ".data[$i].tokens")
                USED=$(echo "$RESPONSE" | jq -r ".data[$i].amount_tokens")
                log_info "  [包$i] $PKG_ID: tokens=$TOTAL_TOKENS, 已用=$USED, 剩余=$REMAIN_TOKENS, 消费记录=$DETAIL_COUNT"
            else
                log_info "  [包$i] $PKG_ID: 消费记录=$DETAIL_COUNT"
            fi
        done

        # 汇总是否有Chat消费被记录
        TOTAL_DETAIL_COUNT=$(echo "$RESPONSE" | jq '[.data[].details | length] | add // 0')
        if [ "$TOTAL_DETAIL_COUNT" -gt 0 ]; then
            log_success "  Chat消费已记录在订单中（共 $TOTAL_DETAIL_COUNT 条消费明细）"
        else
            log_warning "  订单数据已返回，但无消费明细（资源包valid_until已过期或消费时间不在窗口内）"
        fi
    else
        log_error "  订单数据查询失败"
        echo "  响应: $RESPONSE"
    fi
}

# Scenario A: 小包配额不足时自动回落大包
# Regression test for FIFO ordering fix.
# Verify that when a tiny package (points=10 => QuotaGranted=5, earlier expiry so FIFO picks
# it first) cannot cover the estimated quota for a chat request, the middleware correctly
# skips it and falls back to the large package (points=1_000_000_000 => QuotaGranted=500_000_000).
# The small package must remain fully intact (remain_points==10) after the call.
test_small_package_fallback_to_large() {
    test_start "Scenario A: 小包配额不足时自动回落大包"

    local PHONE
    PHONE=$(cat /tmp/wisemodel_test_phone.txt)
    local WM_KEY
    WM_KEY=$(cat /tmp/wisemodel_test_wm_key.txt)

    # Step 1: create small package (points=10, earlier expiry so FIFO picks it first)
    local SUFFIX_S
    SUFFIX_S=$(date +%s)${RANDOM}
    local SMALL_ORDER_ID="WM-SMALL-ORD-${SUFFIX_S}"
    local SMALL_PKG_ID="WM-SMALL-${SUFFIX_S}"

    local SMALL_RESP
    SMALL_RESP=$(curl -s -X POST "$BASE_URL/api/wisemodel/orders/record" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"order_id\": \"$SMALL_ORDER_ID\",
            \"package_count\": 1,
            \"packages\": [
                {
                    \"id\": \"$SMALL_PKG_ID\",
                    \"points\": 10,
                    \"tokens\": 0,
                    \"amount\": 0.01,
                    \"phone\": \"$PHONE\",
                    \"is_free\": false,
                    \"valid_until\": \"2026-12-31T23:59:59Z\",
                    \"created_at\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\",
                    \"model_names\": \"\"
                }
            ]
        }")

    # Step 2: create large package (points=1_000_000_000, later expiry)
    local SUFFIX_L
    SUFFIX_L=$(date +%s)${RANDOM}
    local LARGE_ORDER_ID="WM-LARGE-ORD-${SUFFIX_L}"
    local LARGE_PKG_ID="WM-LARGE-${SUFFIX_L}"

    local LARGE_RESP
    LARGE_RESP=$(curl -s -X POST "$BASE_URL/api/wisemodel/orders/record" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"order_id\": \"$LARGE_ORDER_ID\",
            \"package_count\": 1,
            \"packages\": [
                {
                    \"id\": \"$LARGE_PKG_ID\",
                    \"points\": 1000000000,
                    \"tokens\": 0,
                    \"amount\": 100.00,
                    \"phone\": \"$PHONE\",
                    \"is_free\": false,
                    \"valid_until\": \"2027-12-31T23:59:59Z\",
                    \"created_at\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\",
                    \"model_names\": \"\"
                }
            ]
        }")

    # Step 3: assert both orders created successfully
    local SMALL_OK LARGE_OK
    SMALL_OK=$(echo "$SMALL_RESP" | jq -r '.success' 2>/dev/null)
    LARGE_OK=$(echo "$LARGE_RESP" | jq -r '.success' 2>/dev/null)

    if [ "$SMALL_OK" != "true" ]; then
        log_error "  [A] 小包创建失败: $SMALL_RESP"
        return
    fi
    log_info "  [A] 小包创建成功 (pkg_id=$SMALL_PKG_ID, points=10, QuotaGranted=5, valid_until=2026-12-31)"

    if [ "$LARGE_OK" != "true" ]; then
        log_error "  [A] 大包创建失败: $LARGE_RESP"
        return
    fi
    log_info "  [A] 大包创建成功 (pkg_id=$LARGE_PKG_ID, points=1000000000, QuotaGranted=500000000, valid_until=2027-12-31)"

    # Allow DB sync before sending chat request
    sleep 1

    # Step 4: make one chat call with max_tokens=512 (estimated quota >> QuotaGranted=5)
    local CHAT_MODEL="${WISEMODEL_CHAT_MODEL:-minimax-m2.5-highspeed}"
    local CHAT_HTTP_CODE CHAT_RESP
    CHAT_HTTP_CODE=$(curl -s -o /tmp/wm_chat_resp_a.json -w "%{http_code}" \
        -X POST "$BASE_URL/v1/chat/completions" \
        -H "Authorization: Bearer $WM_KEY" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"$CHAT_MODEL\",
            \"messages\": [{\"role\": \"user\", \"content\": \"Reply OK\"}],
            \"max_tokens\": 512
        }")
    CHAT_RESP=$(cat /tmp/wm_chat_resp_a.json 2>/dev/null || true)

    # Step 5: assert HTTP 200, choices > 0
    if [ "$CHAT_HTTP_CODE" != "200" ]; then
        log_error "  [A] Chat调用返回非200: HTTP $CHAT_HTTP_CODE, body=$CHAT_RESP"
        return
    fi
    local CHOICES
    CHOICES=$(echo "$CHAT_RESP" | jq '.choices | length' 2>/dev/null || echo "0")
    if [ -n "$CHOICES" ] && [ "$CHOICES" -gt 0 ] 2>/dev/null; then
        log_info "  [A] Chat调用成功 HTTP 200, choices=$CHOICES (model=$CHAT_MODEL)"
    else
        log_error "  [A] Chat响应无choices: $CHAT_RESP"
        return
    fi

    sleep 2

    # Step 6: query package_usage
    local USAGE_RESP
    USAGE_RESP=$(curl -s -X POST "$BASE_URL/api/wisemodel/user/package_usage" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{\"phone\": \"$PHONE\"}")

    # Step 7: find small package remain_points by ID — expect still 10 (not consumed)
    local SMALL_REMAIN
    SMALL_REMAIN=$(echo "$USAGE_RESP" | jq --arg pid "$SMALL_PKG_ID" \
        '.data[] | select(.package_id == $pid) | .remain_points' 2>/dev/null)

    # Step 8: find large package remain_points by ID — expect < 1000000000 (consumed)
    local LARGE_REMAIN
    LARGE_REMAIN=$(echo "$USAGE_RESP" | jq --arg pid "$LARGE_PKG_ID" \
        '.data[] | select(.package_id == $pid) | .remain_points' 2>/dev/null)

    log_info "  [A] 小包剩余积分: $SMALL_REMAIN (期望=10, 不应被消耗)"
    log_info "  [A] 大包剩余积分: $LARGE_REMAIN (期望<1000000000, 应被消耗)"

    # Assert small package was NOT consumed (remain_points == 10)
    if [ "$SMALL_REMAIN" = "10" ]; then
        log_success "  [A] 小包未被消耗 (remain_points=10，FIFO回落符合预期)"
    else
        log_error "  [A] 小包状态异常: remain_points=$SMALL_REMAIN, 期望=10"
    fi

    # Assert large package WAS consumed (remain_points < 1000000000)
    local LARGE_CONSUMED=false
    if [ -n "$LARGE_REMAIN" ] && [ "$LARGE_REMAIN" -lt 1000000000 ] 2>/dev/null; then
        LARGE_CONSUMED=true
    fi
    if [ "$LARGE_CONSUMED" = "true" ]; then
        log_success "  [A] 大包已被消耗 (remain_points=$LARGE_REMAIN < 1000000000，回落成功)"
    else
        log_error "  [A] 大包未被消耗: remain_points=$LARGE_REMAIN, 期望<1000000000"
    fi
}

# Scenario B: 专用包与通用包模型隔离
# Verify that a model-specific package only absorbs requests for its designated model,
# while a universal package absorbs requests for other models.
test_model_specific_package_isolation() {
    test_start "Scenario B: 专用包与通用包模型隔离"

    local PHONE
    PHONE=$(cat /tmp/wisemodel_test_phone.txt)

    local SPECIFIC_MODEL="${WISEMODEL_SPECIFIC_MODEL:-minimax-m2}"
    local OTHER_MODEL="${WISEMODEL_OTHER_MODEL:-minimax-m2.5-highspeed}"

    local SUFFIX_S
    SUFFIX_S=$(date +%s)${RANDOM}
    local SPEC_ORDER_ID="WM-SPEC-ORD-${SUFFIX_S}"
    local SPEC_PKG_ID="WM-SPEC-${SUFFIX_S}"

    # Create specific-model package
    local SPEC_RESP
    SPEC_RESP=$(curl -s -X POST "$BASE_URL/api/wisemodel/orders/record" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"order_id\": \"$SPEC_ORDER_ID\",
            \"package_count\": 1,
            \"packages\": [
                {
                    \"id\": \"$SPEC_PKG_ID\",
                    \"points\": 1000000000,
                    \"tokens\": 0,
                    \"amount\": 100.00,
                    \"phone\": \"$PHONE\",
                    \"is_free\": false,
                    \"model_names\": \"$SPECIFIC_MODEL\",
                    \"valid_until\": \"2027-12-31T23:59:59Z\",
                    \"created_at\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\"
                }
            ]
        }")

    sleep 1

    local SUFFIX_U
    SUFFIX_U=$(date +%s)${RANDOM}
    local UNIV_ORDER_ID="WM-UNIV-ORD-${SUFFIX_U}"
    local UNIV_PKG_ID="WM-UNIV-${SUFFIX_U}"

    # Create universal package
    local UNIV_RESP
    UNIV_RESP=$(curl -s -X POST "$BASE_URL/api/wisemodel/orders/record" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"order_id\": \"$UNIV_ORDER_ID\",
            \"package_count\": 1,
            \"packages\": [
                {
                    \"id\": \"$UNIV_PKG_ID\",
                    \"points\": 1000000000,
                    \"tokens\": 0,
                    \"amount\": 100.00,
                    \"phone\": \"$PHONE\",
                    \"is_free\": false,
                    \"valid_until\": \"2027-12-31T23:59:59Z\",
                    \"created_at\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\"
                }
            ]
        }")

    # Assert both package creations succeeded
    local SPEC_OK
    SPEC_OK=$(echo "$SPEC_RESP" | jq -r '.success' 2>/dev/null)
    local UNIV_OK
    UNIV_OK=$(echo "$UNIV_RESP" | jq -r '.success' 2>/dev/null)

    if [ "$SPEC_OK" != "true" ]; then
        log_error "  [B] 专用包创建失败: $SPEC_RESP"
        return
    fi
    log_info "  [B] 专用包创建成功 (pkg_id=$SPEC_PKG_ID, model=$SPECIFIC_MODEL)"

    if [ "$UNIV_OK" != "true" ]; then
        log_error "  [B] 通用包创建失败: $UNIV_RESP"
        return
    fi
    log_info "  [B] 通用包创建成功 (pkg_id=$UNIV_PKG_ID, model=universal)"

    sleep 2

    local WM_KEY
    WM_KEY=$(cat /tmp/wisemodel_test_wm_key.txt)

    # --- Phase 1: call with SPECIFIC_MODEL ---
    local CHAT_RESP1
    CHAT_RESP1=$(curl -s -X POST "$BASE_URL/v1/chat/completions" \
        -H "Authorization: Bearer $WM_KEY" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"$SPECIFIC_MODEL\",
            \"messages\": [{\"role\": \"user\", \"content\": \"Hello, reply with just OK\"}],
            \"max_tokens\": 256
        }")

    local CHOICES1
    CHOICES1=$(echo "$CHAT_RESP1" | jq '.choices | length' 2>/dev/null)
    if [ -n "$CHOICES1" ] && [ "$CHOICES1" -gt 0 ] 2>/dev/null; then
        log_info "  [B] Phase1 Chat调用成功 (model=$SPECIFIC_MODEL)"
    else
        log_error "  [B] Phase1 Chat调用失败 (model=$SPECIFIC_MODEL): $CHAT_RESP1"
        return
    fi

    sleep 2

    # Query usage after Phase 1
    local USAGE1
    USAGE1=$(curl -s -X POST "$BASE_URL/api/wisemodel/user/package_usage" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{\"phone\": \"$PHONE\"}")

    local SPEC_REMAIN1
    SPEC_REMAIN1=$(echo "$USAGE1" | jq --arg pid "$SPEC_PKG_ID" \
        '.data[] | select(.package_id == $pid) | .remain_points' 2>/dev/null)
    local UNIV_REMAIN1
    UNIV_REMAIN1=$(echo "$USAGE1" | jq --arg pid "$UNIV_PKG_ID" \
        '.data[] | select(.package_id == $pid) | .remain_points' 2>/dev/null)

    log_info "  [B] Phase1后 专用包剩余积分: $SPEC_REMAIN1 (期望<1000000000)"
    log_info "  [B] Phase1后 通用包剩余积分: $UNIV_REMAIN1 (期望=1000000000)"

    # Assert specific package was charged after Phase 1
    local SPEC_CHARGED=false
    if [ -n "$SPEC_REMAIN1" ] && [ "$SPEC_REMAIN1" -lt 1000000000 ] 2>/dev/null; then
        SPEC_CHARGED=true
    fi
    if [ "$SPEC_CHARGED" = "true" ]; then
        log_success "  [B] 专用包已被专用模型请求消耗 (remain_points=$SPEC_REMAIN1，符合预期)"
    else
        log_error "  [B] 专用包未被消耗: remain_points=$SPEC_REMAIN1, 期望<1000000000"
    fi

    # Assert universal package was NOT charged after Phase 1
    if [ "$UNIV_REMAIN1" = "1000000000" ]; then
        log_success "  [B] 通用包在专用模型请求后未被消耗 (remain_points=1000000000，符合预期)"
    else
        log_error "  [B] 通用包状态异常: remain_points=$UNIV_REMAIN1, 期望=1000000000"
    fi

    # --- Phase 2: call with OTHER_MODEL ---
    local CHAT_RESP2
    CHAT_RESP2=$(curl -s -X POST "$BASE_URL/v1/chat/completions" \
        -H "Authorization: Bearer $WM_KEY" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"$OTHER_MODEL\",
            \"messages\": [{\"role\": \"user\", \"content\": \"Hello, reply with just OK\"}],
            \"max_tokens\": 256
        }")

    local CHOICES2
    CHOICES2=$(echo "$CHAT_RESP2" | jq '.choices | length' 2>/dev/null)
    if [ -n "$CHOICES2" ] && [ "$CHOICES2" -gt 0 ] 2>/dev/null; then
        log_info "  [B] Phase2 Chat调用成功 (model=$OTHER_MODEL)"
    else
        log_error "  [B] Phase2 Chat调用失败 (model=$OTHER_MODEL): $CHAT_RESP2"
        return
    fi

    sleep 2

    # Query usage after Phase 2
    local USAGE2
    USAGE2=$(curl -s -X POST "$BASE_URL/api/wisemodel/user/package_usage" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{\"phone\": \"$PHONE\"}")

    local UNIV_REMAIN2
    UNIV_REMAIN2=$(echo "$USAGE2" | jq --arg pid "$UNIV_PKG_ID" \
        '.data[] | select(.package_id == $pid) | .remain_points' 2>/dev/null)

    log_info "  [B] Phase2后 通用包剩余积分: $UNIV_REMAIN2 (期望<1000000000)"

    # Assert universal package was charged after Phase 2
    local UNIV_CHARGED=false
    if [ -n "$UNIV_REMAIN2" ] && [ "$UNIV_REMAIN2" -lt 1000000000 ] 2>/dev/null; then
        UNIV_CHARGED=true
    fi
    if [ "$UNIV_CHARGED" = "true" ]; then
        log_success "  [B] 通用包已被其他模型请求消耗 (remain_points=$UNIV_REMAIN2，符合预期)"
    else
        log_error "  [B] 通用包未被其他模型请求消耗: remain_points=$UNIV_REMAIN2, 期望<1000000000"
    fi
}

# Scenario C: 新包首次调用成功（懒初始化验证）
# Verify that a brand-new package (Redis key not yet seeded) works on first call —
# lazy-init correctly computes remaining quota from DB.
test_fresh_package_first_call_succeeds() {
    test_start "Scenario C: 新包首次调用成功（懒初始化验证）"

    local PHONE
    PHONE=$(cat /tmp/wisemodel_test_phone.txt)
    local WM_KEY
    WM_KEY=$(cat /tmp/wisemodel_test_wm_key.txt)

    local SUFFIX
    SUFFIX=$(date +%s)${RANDOM}
    local ORDER_ID="WM-FRESH-ORD-C-${SUFFIX}"
    local PKG_ID="PKG_FRESH_C_${SUFFIX}"

    # Step 2: Create fresh package with large points and a far-future expiry
    local CREATE_RESP
    CREATE_RESP=$(curl -s -X POST "$BASE_URL/api/wisemodel/orders/record" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"order_id\": \"$ORDER_ID\",
            \"package_count\": 1,
            \"packages\": [
                {
                    \"id\": \"$PKG_ID\",
                    \"points\": 1000000000,
                    \"tokens\": 0,
                    \"amount\": 100.00,
                    \"phone\": \"$PHONE\",
                    \"is_free\": false,
                    \"valid_until\": \"2028-12-31T23:59:59Z\",
                    \"created_at\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\",
                    \"model_names\": \"\"
                }
            ]
        }")

    local CREATE_OK
    CREATE_OK=$(echo "$CREATE_RESP" | jq -r '.success' 2>/dev/null)
    if [ "$CREATE_OK" != "true" ]; then
        log_error "  [C] 新包创建失败: $CREATE_RESP"
        return
    fi
    log_info "  [C] 新包创建成功 (pkg_id=$PKG_ID, points=1000000000)"

    # Step 4: Allow DB sync
    sleep 1

    # Step 5: Make chat call immediately — Redis key for this brand-new package doesn't exist yet
    local CHAT_MODEL="${WISEMODEL_CHAT_MODEL:-minimax-m2.5-highspeed}"
    local CHAT_RESP
    CHAT_RESP=$(curl -s -X POST "$BASE_URL/v1/chat/completions" \
        -H "Authorization: Bearer $WM_KEY" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"$CHAT_MODEL\",
            \"messages\": [{\"role\": \"user\", \"content\": \"Reply OK\"}],
            \"max_tokens\": 512
        }")

    # Step 6: Assert HTTP 200, choices > 0
    local CHOICES
    CHOICES=$(echo "$CHAT_RESP" | jq '.choices | length' 2>/dev/null)
    if [ -n "$CHOICES" ] && [ "$CHOICES" -gt 0 ] 2>/dev/null; then
        log_success "  [C] Chat调用成功 (懒初始化正常工作)"
    else
        log_error "  [C] Chat调用失败（懒初始化可能失败）: $CHAT_RESP"
        return
    fi

    # Step 7: Query package_usage
    local USAGE
    USAGE=$(curl -s -X POST "$BASE_URL/api/wisemodel/user/package_usage" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{\"phone\": \"$PHONE\"}")

    # Step 8: Assert remain_points < 1000000000 (lazy-init succeeded, quota was consumed)
    local REMAIN
    REMAIN=$(echo "$USAGE" | jq --arg pid "$PKG_ID" \
        '.data[] | select(.package_id == $pid) | .remain_points' 2>/dev/null)

    log_info "  [C] 新包剩余积分: $REMAIN (期望<1000000000)"

    if [ -n "$REMAIN" ] && [ "$REMAIN" -lt 1000000000 ] 2>/dev/null; then
        log_success "  [C] 懒初始化成功：新包积分已被消耗 (remain_points=$REMAIN)"
    else
        log_error "  [C] 懒初始化可能失败：remain_points=$REMAIN，期望<1000000000"
    fi
}

# Scenario D: 零配额包被正确跳过，大包接管
# Verify that a package with points=1 (QuotaGranted=0) is correctly skipped by
# SelectPackageWithRemainingQuota, while a large package handles the request.
test_small_quota_package_exhaustion() {
    test_start "Scenario D: 零配额包被正确跳过，大包接管"

    local PHONE
    PHONE=$(cat /tmp/wisemodel_test_phone.txt)
    local WM_KEY
    WM_KEY=$(cat /tmp/wisemodel_test_wm_key.txt)

    local SUFFIX
    SUFFIX=$(date +%s)${RANDOM}

    # Step 1: Create tiny package: points=1 => QuotaGranted=0 (pre-consume must skip it)
    local TINY_ORDER_ID="WM-TINY-ORD-D-${SUFFIX}"
    local TINY_PKG_ID="PKG_TINY_D_${SUFFIX}"

    local TINY_RESP
    TINY_RESP=$(curl -s -X POST "$BASE_URL/api/wisemodel/orders/record" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"order_id\": \"$TINY_ORDER_ID\",
            \"package_count\": 1,
            \"packages\": [
                {
                    \"id\": \"$TINY_PKG_ID\",
                    \"points\": 1,
                    \"tokens\": 0,
                    \"amount\": 0.01,
                    \"phone\": \"$PHONE\",
                    \"is_free\": false,
                    \"valid_until\": \"2026-01-01T23:59:59Z\",
                    \"created_at\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\",
                    \"model_names\": \"\"
                }
            ]
        }")

    local TINY_OK
    TINY_OK=$(echo "$TINY_RESP" | jq -r '.success' 2>/dev/null)
    if [ "$TINY_OK" != "true" ]; then
        log_error "  [D] 小包(points=1)创建失败: $TINY_RESP"
        return
    fi
    log_info "  [D] 小包创建成功 (pkg_id=$TINY_PKG_ID, points=1)"

    sleep 1

    # Step 2: Create large package to absorb the request
    local LARGE_SUFFIX
    LARGE_SUFFIX=$(date +%s)${RANDOM}
    local LARGE_ORDER_ID="WM-LARGE-ORD-D-${LARGE_SUFFIX}"
    local LARGE_PKG_ID="PKG_LARGE_D_${LARGE_SUFFIX}"

    local LARGE_RESP
    LARGE_RESP=$(curl -s -X POST "$BASE_URL/api/wisemodel/orders/record" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"order_id\": \"$LARGE_ORDER_ID\",
            \"package_count\": 1,
            \"packages\": [
                {
                    \"id\": \"$LARGE_PKG_ID\",
                    \"points\": 1000000000,
                    \"tokens\": 0,
                    \"amount\": 100.00,
                    \"phone\": \"$PHONE\",
                    \"is_free\": false,
                    \"valid_until\": \"2027-12-31T23:59:59Z\",
                    \"created_at\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\",
                    \"model_names\": \"\"
                }
            ]
        }")

    local LARGE_OK
    LARGE_OK=$(echo "$LARGE_RESP" | jq -r '.success' 2>/dev/null)
    if [ "$LARGE_OK" != "true" ]; then
        log_error "  [D] 大包创建失败: $LARGE_RESP"
        return
    fi
    log_info "  [D] 大包创建成功 (pkg_id=$LARGE_PKG_ID, points=1000000000)"

    sleep 1

    # Step 3: Make one chat call
    local CHAT_MODEL="${WISEMODEL_CHAT_MODEL:-minimax-m2.5-highspeed}"
    local CHAT_RESP
    CHAT_RESP=$(curl -s -X POST "$BASE_URL/v1/chat/completions" \
        -H "Authorization: Bearer $WM_KEY" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"$CHAT_MODEL\",
            \"messages\": [{\"role\": \"user\", \"content\": \"Reply OK\"}],
            \"max_tokens\": 512
        }")

    # Step 4: Assert HTTP 200 (large package should handle it)
    local CHOICES
    CHOICES=$(echo "$CHAT_RESP" | jq '.choices | length' 2>/dev/null)
    if [ -n "$CHOICES" ] && [ "$CHOICES" -gt 0 ] 2>/dev/null; then
        log_info "  [D] Chat调用成功（大包接管）"
    else
        log_error "  [D] Chat调用失败: $CHAT_RESP"
        return
    fi

    sleep 1

    # Step 5: Query package_usage
    local USAGE
    USAGE=$(curl -s -X POST "$BASE_URL/api/wisemodel/user/package_usage" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{\"phone\": \"$PHONE\"}")

    local TINY_REMAIN
    TINY_REMAIN=$(echo "$USAGE" | jq --arg pid "$TINY_PKG_ID" \
        '.data[] | select(.package_id == $pid) | .remain_points' 2>/dev/null)

    local LARGE_REMAIN
    LARGE_REMAIN=$(echo "$USAGE" | jq --arg pid "$LARGE_PKG_ID" \
        '.data[] | select(.package_id == $pid) | .remain_points' 2>/dev/null)

    log_info "  [D] 小包(points=1)剩余积分: $TINY_REMAIN (期望=1，未被消耗)"
    log_info "  [D] 大包剩余积分: $LARGE_REMAIN (期望<1000000000)"

    # Step 6: Assert tiny package remain_points is still 1 (pre-consume correctly skipped it)
    if [ "$TINY_REMAIN" = "1" ]; then
        log_success "  [D] 零配额包被正确跳过 (remain_points=1，未被消耗)"
    else
        log_error "  [D] 零配额包状态异常: remain_points=$TINY_REMAIN, 期望=1"
    fi

    # Step 7: Assert large package was consumed
    if [ -n "$LARGE_REMAIN" ] && [ "$LARGE_REMAIN" -lt 1000000000 ] 2>/dev/null; then
        log_success "  [D] 大包正确接管请求 (remain_points=$LARGE_REMAIN < 1000000000)"
    else
        log_error "  [D] 大包未被消耗: remain_points=$LARGE_REMAIN, 期望<1000000000"
    fi
}

# Scenario E: FIFO顺序消费 — 早到期的包先消费
# Verify FIFO ordering — the package with the earlier valid_until is consumed first.
test_fifo_package_consumption_order() {
    test_start "Scenario E: FIFO顺序消费 — 早到期的包先消费"

    local PHONE
    PHONE=$(cat /tmp/wisemodel_test_phone.txt)
    local WM_KEY
    WM_KEY=$(cat /tmp/wisemodel_test_wm_key.txt)

    local SUFFIX_A
    SUFFIX_A=$(date +%s)${RANDOM}

    # Step 2: Create package A — expires sooner (2026-06-30)
    local ORDER_A="WM-FIFO-ORD-A-${SUFFIX_A}"
    local PKG_A="PKG_FIFO_A_${SUFFIX_A}"

    local RESP_A
    RESP_A=$(curl -s -X POST "$BASE_URL/api/wisemodel/orders/record" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"order_id\": \"$ORDER_A\",
            \"package_count\": 1,
            \"packages\": [
                {
                    \"id\": \"$PKG_A\",
                    \"points\": 1000000000,
                    \"tokens\": 0,
                    \"amount\": 100.00,
                    \"phone\": \"$PHONE\",
                    \"is_free\": false,
                    \"valid_until\": \"2026-06-30T23:59:59Z\",
                    \"created_at\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\",
                    \"model_names\": \"\"
                }
            ]
        }")

    sleep 1

    local SUFFIX_B
    SUFFIX_B=$(date +%s)${RANDOM}

    # Step 3: Create package B — expires later (2028-06-30)
    local ORDER_B="WM-FIFO-ORD-B-${SUFFIX_B}"
    local PKG_B="PKG_FIFO_B_${SUFFIX_B}"

    local RESP_B
    RESP_B=$(curl -s -X POST "$BASE_URL/api/wisemodel/orders/record" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"order_id\": \"$ORDER_B\",
            \"package_count\": 1,
            \"packages\": [
                {
                    \"id\": \"$PKG_B\",
                    \"points\": 1000000000,
                    \"tokens\": 0,
                    \"amount\": 100.00,
                    \"phone\": \"$PHONE\",
                    \"is_free\": false,
                    \"valid_until\": \"2028-06-30T23:59:59Z\",
                    \"created_at\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\",
                    \"model_names\": \"\"
                }
            ]
        }")

    # Step 4: Assert both orders created
    local OK_A
    OK_A=$(echo "$RESP_A" | jq -r '.success' 2>/dev/null)
    local OK_B
    OK_B=$(echo "$RESP_B" | jq -r '.success' 2>/dev/null)

    if [ "$OK_A" != "true" ]; then
        log_error "  [E] 包A创建失败: $RESP_A"
        return
    fi
    log_info "  [E] 包A创建成功 (pkg_id=$PKG_A, valid_until=2026-06-30)"

    if [ "$OK_B" != "true" ]; then
        log_error "  [E] 包B创建失败: $RESP_B"
        return
    fi
    log_info "  [E] 包B创建成功 (pkg_id=$PKG_B, valid_until=2028-06-30)"

    # Step 5: Allow DB sync
    sleep 1

    # Step 6: Make 3 chat calls to ensure measurable consumption
    local CHAT_MODEL="${WISEMODEL_CHAT_MODEL:-minimax-m2.5-highspeed}"
    local i
    for i in 1 2 3; do
        local CHAT_RESP
        CHAT_RESP=$(curl -s -X POST "$BASE_URL/v1/chat/completions" \
            -H "Authorization: Bearer $WM_KEY" \
            -H "Content-Type: application/json" \
            -d "{
                \"model\": \"$CHAT_MODEL\",
                \"messages\": [{\"role\": \"user\", \"content\": \"Reply OK\"}],
                \"max_tokens\": 512
            }")
        local CHOICES
        CHOICES=$(echo "$CHAT_RESP" | jq '.choices | length' 2>/dev/null)
        if [ -n "$CHOICES" ] && [ "$CHOICES" -gt 0 ] 2>/dev/null; then
            log_info "  [E] Chat call $i 成功"
        else
            log_error "  [E] Chat call $i 失败: $CHAT_RESP"
            return
        fi
    done

    sleep 1

    # Step 7: Query package_usage
    local USAGE
    USAGE=$(curl -s -X POST "$BASE_URL/api/wisemodel/user/package_usage" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{\"phone\": \"$PHONE\"}")

    local REMAIN_A
    REMAIN_A=$(echo "$USAGE" | jq --arg pid "$PKG_A" \
        '.data[] | select(.package_id == $pid) | .remain_points' 2>/dev/null)

    local REMAIN_B
    REMAIN_B=$(echo "$USAGE" | jq --arg pid "$PKG_B" \
        '.data[] | select(.package_id == $pid) | .remain_points' 2>/dev/null)

    log_info "  [E] 包A剩余积分: $REMAIN_A (期望<1000000000，先消费)"
    log_info "  [E] 包B剩余积分: $REMAIN_B (期望=1000000000 或 > 包A剩余)"

    # Step 8: Assert package A was consumed (earlier expiry = first in FIFO order)
    if [ -n "$REMAIN_A" ] && [ "$REMAIN_A" -lt 1000000000 ] 2>/dev/null; then
        log_success "  [E] 包A（早到期）已被优先消耗 (remain_points=$REMAIN_A)"
    else
        log_error "  [E] 包A未被消耗: remain_points=$REMAIN_A, 期望<1000000000"
    fi

    # Step 9: Assert package B was not consumed OR consumed less than A
    local B_UNTOUCHED=false
    local B_LESS_CONSUMED=false
    if [ "$REMAIN_B" = "1000000000" ]; then
        B_UNTOUCHED=true
    fi
    if [ -n "$REMAIN_A" ] && [ -n "$REMAIN_B" ] && [ "$REMAIN_B" -gt "$REMAIN_A" ] 2>/dev/null; then
        B_LESS_CONSUMED=true
    fi

    if [ "$B_UNTOUCHED" = "true" ]; then
        log_success "  [E] 包B（晚到期）完全未被消耗 (remain_points=1000000000，FIFO正确)"
    elif [ "$B_LESS_CONSUMED" = "true" ]; then
        log_success "  [E] 包B消耗少于包A (remain_points=$REMAIN_B > $REMAIN_A，FIFO倾向正确)"
    else
        log_error "  [E] FIFO顺序异常: 包A remain=$REMAIN_A, 包B remain=$REMAIN_B"
    fi
}

# 主测试流程
main() {
    echo ""
    echo "╔════════════════════════════════════════════════════╗"
    echo "║     Wisemodel MaaS API 自动化测试              ║"
    echo "╚════════════════════════════════════════════════════╝"
    echo ""
    echo "测试配置:"
    echo "  BASE_URL: $BASE_URL"
    echo "  TOKEN: ${TOKEN:0:10}...${TOKEN: -4}"
    echo ""

    # 检查服务
    check_service
    echo ""

    # 依赖检查
    if ! command -v jq &> /dev/null; then
        log_error "需要安装 jq 工具: brew install jq"
        exit 1
    fi

    # 运行测试
    log_info "开始运行测试..."
    echo ""

    # 核心功能测试
    test_user_bind
    # sleep 1
    test_update_key
    # sleep 1
    # test_create_order_points
    # sleep 1
    # test_create_order_tokens
    # sleep 1
    # test_create_order_free
    sleep 1
    test_package_usage
    sleep 1
    test_chat_call
    test_chat_call
    test_chat_call
    sleep 1
    test_verify_order_after_chat
    # sleep 1
    # test_update_phone
    # sleep 1

    # test_delete_key
    # sleep 1

    # 错误处理测试
    # test_invalid_token
    # sleep 1
    # test_user_not_found
    # sleep 1
    # test_package_count_mismatch
    # sleep 1

    # 高级功能测试
    # test_multiple_packages
    # sleep 1

    # 场景测试
    sleep 1
    test_small_package_fallback_to_large
    sleep 1
    test_model_specific_package_isolation
    sleep 1
    test_fresh_package_first_call_succeeds
    sleep 1
    test_small_quota_package_exhaustion
    sleep 1
    test_fifo_package_consumption_order

    # 测试报告
    echo ""
    echo "╔════════════════════════════════════════════════════╗"
    echo "║                  测试报告                        ║"
    echo "╚════════════════════════════════════════════════════╝"
    echo ""
    echo "总测试数: $TOTAL_TESTS"
    echo -e "${GREEN}通过: $PASSED_TESTS${NC}"
    echo -e "${RED}失败: $FAILED_TESTS${NC}"
    echo ""

    if [ $FAILED_TESTS -eq 0 ]; then
        echo -e "${GREEN}✅ 所有测试通过！${NC}"
        return 0
    else
        echo -e "${RED}❌ 部分测试失败，请查看上面的详细信息${NC}"
        return 1
    fi
}

# 运行主函数
main

# 清理临时文件
rm -f /tmp/wisemodel_test_phone.txt /tmp/wisemodel_test_package_id.txt /tmp/wisemodel_test_wm_key.txt
