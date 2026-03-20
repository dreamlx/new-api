#!/bin/bash

# Wisemodel MaaS API 自动化测试脚本
# 版本: v1.0
# 最后更新: 2025-01-21

set -e

# 配置
BASE_URL="${BASE_URL:-https://open.ospreyai.cn}"
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
    # echo "15321866239"

}

# 测试1：用户绑定
test_user_bind() {
    test_start "用户绑定 - 新用户"

    PHONE=$(generate_phone)
    WM_KEY="wm_key_test_$(date +%s)"
    WM_KEY="wisemodel-iygugqeusgbodidvxxgl"
    RESPONSE=$(curl -s -X POST "$BASE_URL/api/wisemodel/user/bind" \
        -H "Authorization: Bearer $TOKEN" \
        -H "Content-Type: application/json" \
        -d "{
            \"phone\": \"$PHONE\",
            \"wisemodel_key\": \"$WM_KEY\",
            \"username\": \"测试用户_$(date +%s)\"
        }")

    if check_json_field "$RESPONSE" ".success" "true"; then
        log_success "  用户绑定成功"
        echo "$PHONE" > /tmp/wisemodel_test_phone.txt
        echo "$WM_KEY" > /tmp/wisemodel_test_wm_key.txt
    else
        log_error "  用户绑定失败"
        echo "  响应: $RESPONSE"
    fi
}

# 测试2：更新Wisemodel Key
test_update_key() {
    test_start "更新Wisemodel Key"

    PHONE=$(cat /tmp/wisemodel_test_phone.txt)
    # NEW_KEY="wm_key_updated_$(date +%s)"
    NEW_KEY="wisemodel-bvwulxeoviypfmzfvrwj"
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
    PACKAGE_ID="package16_$(date +%s)"

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
    CHAT_MODEL="${WISEMODEL_CHAT_MODEL:-minimax-m2-latest}"

    RESPONSE=$(curl -s -X POST "$BASE_URL/v1/chat/completions" \
        -H "Authorization: Bearer $WM_KEY" \
        -H "Content-Type: application/json" \
        -d "{
            \"model\": \"$CHAT_MODEL\",
            \"messages\": [
                {\"role\": \"user\", \"content\": \"Hello, reply with just OK\"}
            ],
            \"max_tokens\": 10
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
    sleep 1
    # test_update_key
    sleep 1
    test_create_order_points
    sleep 1
    # test_create_order_tokens
    sleep 1
    # test_create_order_free
    sleep 1
    test_package_usage
    sleep 1
    test_chat_call
    sleep 1
    test_verify_order_after_chat
    # sleep 1
    # test_update_phone
    # sleep 1
    # test_delete_key
    # sleep 1

    # # 错误处理测试
    # test_invalid_token
    # sleep 1
    # test_user_not_found
    # sleep 1
    # test_package_count_mismatch
    # sleep 1

    # # 高级功能测试
    # test_multiple_packages
    # sleep 1

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
