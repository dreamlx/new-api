#!/bin/bash
# OpenRouter API 测试脚本
# 用于验证 OpenRouter 兼容接口的功能

set -e

# 配置
BASE_URL="${BASE_URL:-http://localhost:3000}"
ADMIN_TOKEN="${ADMIN_TOKEN:-}"  # 如果需要测试管理接口，设置此变量

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 计数器
TOTAL_TESTS=0
PASSED_TESTS=0
FAILED_TESTS=0

# 测试函数
test_api() {
    local name="$1"
    local method="$2"
    local endpoint="$3"
    local expected_status="$4"
    local data="$5"
    local auth_header="$6"

    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    echo -e "\n${BLUE}[TEST]${NC} $name"
    echo "  ${method} ${BASE_URL}${endpoint}"

    # 构建curl命令
    local curl_cmd="curl -s -w '\n%{http_code}' -X ${method}"

    if [ -n "$auth_header" ]; then
        curl_cmd="$curl_cmd -H 'Authorization: Bearer $auth_header'"
    fi

    if [ -n "$data" ]; then
        curl_cmd="$curl_cmd -H 'Content-Type: application/json' -d '$data'"
    fi

    curl_cmd="$curl_cmd '${BASE_URL}${endpoint}'"

    # 执行请求
    local response
    response=$(eval $curl_cmd 2>&1)

    # 分离响应体和状态码
    local status_code=$(echo "$response" | tail -n1)
    local body=$(echo "$response" | sed '$d')

    # 检查状态码
    if [ "$status_code" = "$expected_status" ]; then
        echo -e "  ${GREEN}✓ PASS${NC} (Status: $status_code)"
        PASSED_TESTS=$((PASSED_TESTS + 1))

        # 如果是200，显示部分响应
        if [ "$status_code" = "200" ]; then
            echo "  Response preview:"
            echo "$body" | head -c 500
            echo ""
        fi
    else
        echo -e "  ${RED}✗ FAIL${NC} (Expected: $expected_status, Got: $status_code)"
        echo "  Response: $body"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
}

# 检查JSON格式
check_json_structure() {
    local name="$1"
    local endpoint="$2"
    local json_path="$3"

    TOTAL_TESTS=$((TOTAL_TESTS + 1))

    echo -e "\n${BLUE}[CHECK]${NC} $name"

    local response
    response=$(curl -s "${BASE_URL}${endpoint}")

    # 使用 jq 检查路径是否存在
    if echo "$response" | jq -e "$json_path" > /dev/null 2>&1; then
        echo -e "  ${GREEN}✓ PASS${NC} - JSON path '$json_path' exists"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        echo -e "  ${RED}✗ FAIL${NC} - JSON path '$json_path' not found"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
}

echo "========================================"
echo "  OpenRouter API 测试"
echo "  Base URL: ${BASE_URL}"
echo "========================================"

# ==================== 公开接口测试 ====================

echo -e "\n${YELLOW}=== 公开接口测试 ===${NC}"

# 测试1: 获取模型列表
test_api "获取模型列表" "GET" "/api/openrouter/v1/models" "200"

# 测试2: 检查响应结构
check_json_structure "检查 data 数组存在" "/api/openrouter/v1/models" ".data"

# 测试3: 获取状态
test_api "获取 OpenRouter 状态" "GET" "/api/openrouter/status" "200"

# 测试4: 获取单个模型（可能存在的模型）
test_api "获取单个模型 (deepseek-chat)" "GET" "/api/openrouter/v1/models/deepseek-chat" "200"

# 测试5: 获取不存在的模型
test_api "获取不存在的模型" "GET" "/api/openrouter/v1/models/non-existent-model-xyz" "404"

# ==================== 管理接口测试 ====================

if [ -n "$ADMIN_TOKEN" ]; then
    echo -e "\n${YELLOW}=== 管理接口测试 ===${NC}"

    # 测试6: 获取配置（需要认证）
    test_api "获取配置 (需要认证)" "GET" "/api/openrouter/config" "200" "" "$ADMIN_TOKEN"

    # 测试7: 重新加载配置
    test_api "重新加载配置" "POST" "/api/openrouter/config/reload" "200" "" "$ADMIN_TOKEN"

    # 测试8: 未认证访问管理接口
    test_api "未认证访问配置接口" "GET" "/api/openrouter/config" "401"
else
    echo -e "\n${YELLOW}=== 跳过管理接口测试 (未设置 ADMIN_TOKEN) ===${NC}"
fi

# ==================== OpenRouter 格式验证 ====================

echo -e "\n${YELLOW}=== OpenRouter 格式验证 ===${NC}"

# 获取模型列表并验证格式
echo -e "\n${BLUE}[VALIDATE]${NC} 验证 OpenRouter 响应格式"
RESPONSE=$(curl -s "${BASE_URL}/api/openrouter/v1/models")

# 检查必需字段
FIELDS_TO_CHECK=(
    ".data[0].id"
    ".data[0].name"
    ".data[0].context_length"
    ".data[0].architecture"
    ".data[0].pricing"
)

for field in "${FIELDS_TO_CHECK[@]}"; do
    TOTAL_TESTS=$((TOTAL_TESTS + 1))
    if echo "$RESPONSE" | jq -e "$field" > /dev/null 2>&1; then
        echo -e "  ${GREEN}✓${NC} Field $field exists"
        PASSED_TESTS=$((PASSED_TESTS + 1))
    else
        echo -e "  ${RED}✗${NC} Field $field missing"
        FAILED_TESTS=$((FAILED_TESTS + 1))
    fi
done

# ==================== 测试结果汇总 ====================

echo -e "\n========================================"
echo "  测试结果汇总"
echo "========================================"
echo -e "  总测试数: ${TOTAL_TESTS}"
echo -e "  ${GREEN}通过: ${PASSED_TESTS}${NC}"
echo -e "  ${RED}失败: ${FAILED_TESTS}${NC}"

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "\n${GREEN}所有测试通过！${NC}"
    exit 0
else
    echo -e "\n${RED}有 ${FAILED_TESTS} 个测试失败${NC}"
    exit 1
fi
