#!/bin/bash

# V2 外部系统集成 API 测试脚本
# 测试授权计费网关模式的核心功能

set -e

# API基础URL
BASE_URL="http://localhost:3000"
API_PREFIX="/api/v2"
FAILED_TESTS=0
TOTAL_TESTS=0

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# 测试数据
PLATFORM_ID="testplatform"
TOKEN_KEY="sk-v2-test-$(date +%s)"
INITIAL_QUOTA=1000000  # $2 USD

echo -e "${YELLOW}🚀 开始V2 API集成测试${NC}"
echo "========================================"

# 测试1: 密钥授权 - 创建新密钥
echo -e "\n${YELLOW}📝 测试1: 密钥授权 - 创建新密钥${NC}"
TOTAL_TESTS=$((TOTAL_TESTS + 1))

RESPONSE=$(curl -s -X POST "${BASE_URL}${API_PREFIX}/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d "{
    \"platform_id\": \"${PLATFORM_ID}\",
    \"token_key\": \"${TOKEN_KEY}\",
    \"initial_quota\": ${INITIAL_QUOTA},
    \"metadata\": {
      \"platform_user_id\": \"user_123\",
      \"user_type\": \"premium\",
      \"created_by\": \"admin\"
    }
  }")

# 检查响应
if echo "$RESPONSE" | jq -e '.success == true' >/dev/null 2>&1; then
    echo -e "${GREEN}✅ 密钥授权成功${NC}"
    echo "Token Key: $(echo "$RESPONSE" | jq -r '.data.token_key')"
    echo "Current Quota: $(echo "$RESPONSE" | jq -r '.data.current_quota')"
    echo "Quota USD: $(echo "$RESPONSE" | jq -r '.data.quota_usd')"
    echo "Status: $(echo "$RESPONSE" | jq -r '.data.status')"
    PROXY_USER_ID=$(echo "$RESPONSE" | jq -r '.data.proxy_user_id')
else
    echo -e "${RED}❌ 密钥授权失败${NC}"
    echo "$RESPONSE"
    FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# 测试2: 密钥授权 - 重复授权同一密钥（更新额度）
echo -e "\n${YELLOW}📝 测试2: 密钥授权 - 重复授权同一密钥（更新额度）${NC}"
TOTAL_TESTS=$((TOTAL_TESTS + 1))

NEW_QUOTA=2000000  # $4 USD

RESPONSE=$(curl -s -X POST "${BASE_URL}${API_PREFIX}/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d "{
    \"platform_id\": \"${PLATFORM_ID}\",
    \"token_key\": \"${TOKEN_KEY}\",
    \"initial_quota\": ${NEW_QUOTA},
    \"metadata\": {
      \"platform_user_id\": \"user_123\",
      \"user_type\": \"premium_updated\"
    }
  }")

if echo "$RESPONSE" | jq -e '.success == true and .data.status == "updated"' >/dev/null 2>&1; then
    echo -e "${GREEN}✅ 密钥额度更新成功${NC}"
    echo "Previous Quota: $(echo "$RESPONSE" | jq -r '.data.previous_quota')"
    echo "Current Quota: $(echo "$RESPONSE" | jq -r '.data.current_quota')"
    echo "Quota Added: $(echo "$RESPONSE" | jq -r '.data.quota_added')"
else
    echo -e "${RED}❌ 密钥额度更新失败${NC}"
    echo "$RESPONSE"
    FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# 测试3: 密钥授权 - 尝试在不同平台使用相同密钥
echo -e "\n${YELLOW}📝 测试3: 密钥授权 - 冲突检测（不同平台相同密钥）${NC}"
TOTAL_TESTS=$((TOTAL_TESTS + 1))

CONFLICT_RESPONSE=$(curl -s -X POST "${BASE_URL}${API_PREFIX}/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d "{
    \"platform_id\": \"anotherplatform\",
    \"token_key\": \"${TOKEN_KEY}\",
    \"initial_quota\": 500000,
    \"metadata\": {}
  }")

if echo "$CONFLICT_RESPONSE" | jq -e '.success == false and .error_code == "TOKEN_EXISTS"' >/dev/null 2>&1; then
    echo -e "${GREEN}✅ 密钥冲突检测正常${NC}"
    echo "Error Message: $(echo "$CONFLICT_RESPONSE" | jq -r '.message')"
else
    echo -e "${RED}❌ 密钥冲突检测失败${NC}"
    echo "$CONFLICT_RESPONSE"
    FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# 测试4: 参数验证 - 无效platform_id
echo -e "\n${YELLOW}📝 测试4: 参数验证 - 无效platform_id${NC}"
TOTAL_TESTS=$((TOTAL_TESTS + 1))

INVALID_RESPONSE=$(curl -s -X POST "${BASE_URL}${API_PREFIX}/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d "{
    \"platform_id\": \"invalid@platform\",
    \"token_key\": \"sk-test-invalid\",
    \"initial_quota\": 100000
  }")

if echo "$INVALID_RESPONSE" | jq -e '.success == false and .error_code == "INVALID_PARAMETER"' >/dev/null 2>&1; then
    echo -e "${GREEN}✅ 参数验证正常${NC}"
    echo "Error Message: $(echo "$INVALID_RESPONSE" | jq -r '.message')"
else
    echo -e "${RED}❌ 参数验证失败${NC}"
    echo "$INVALID_RESPONSE"
    FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# 测试5: 参数验证 - 无效token_key
echo -e "\n${YELLOW}📝 测试5: 参数验证 - 无效token_key${NC}"
TOTAL_TESTS=$((TOTAL_TESTS + 1))

INVALID_TOKEN_RESPONSE=$(curl -s -X POST "${BASE_URL}${API_PREFIX}/external/tokens/authorize" \
  -H "Content-Type: application/json" \
  -d "{
    \"platform_id\": \"validplatform\",
    \"token_key\": \"invalid-token-key\",
    \"initial_quota\": 100000
  }")

if echo "$INVALID_TOKEN_RESPONSE" | jq -e '.success == false and .error_code == "INVALID_PARAMETER"' >/dev/null 2>&1; then
    echo -e "${GREEN}✅ Token验证正常${NC}"
    echo "Error Message: $(echo "$INVALID_TOKEN_RESPONSE" | jq -r '.message')"
else
    echo -e "${RED}❌ Token验证失败${NC}"
    echo "$INVALID_TOKEN_RESPONSE"
    FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# 等待一下让数据库同步
sleep 1

# 测试6: 平台消费日志查询 - 空日志
echo -e "\n${YELLOW}📝 测试6: 平台消费日志查询 - 空日志${NC}"
TOTAL_TESTS=$((TOTAL_TESTS + 1))

TODAY=$(date +%Y-%m-%d)
LOGS_RESPONSE=$(curl -s -X GET "${BASE_URL}${API_PREFIX}/external/platforms/${PLATFORM_ID}/logs?start_date=${TODAY}&end_date=${TODAY}&page_size=20")

if echo "$LOGS_RESPONSE" | jq -e '.success == true' >/dev/null 2>&1; then
    echo -e "${GREEN}✅ 平台日志查询成功${NC}"
    echo "Platform ID: $(echo "$LOGS_RESPONSE" | jq -r '.data.platform_id')"
    echo "Total Items: $(echo "$LOGS_RESPONSE" | jq -r '.data.pagination.total_items')"
    echo "Total Pages: $(echo "$LOGS_RESPONSE" | jq -r '.data.pagination.total_pages')"
else
    echo -e "${RED}❌ 平台日志查询失败${NC}"
    echo "$LOGS_RESPONSE"
    FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# 测试7: 平台消费日志查询 - 无效平台
echo -e "\n${YELLOW}📝 测试7: 平台消费日志查询 - 无效平台${NC}"
TOTAL_TESTS=$((TOTAL_TESTS + 1))

NO_PLATFORM_RESPONSE=$(curl -s -X GET "${BASE_URL}${API_PREFIX}/external/platforms/nonexistent/logs?start_date=${TODAY}&end_date=${TODAY}")

if echo "$NO_PLATFORM_RESPONSE" | jq -e '.success == false and .error_code == "PLATFORM_NOT_FOUND"' >/dev/null 2>&1; then
    echo -e "${GREEN}✅ 无效平台检测正常${NC}"
    echo "Error Message: $(echo "$NO_PLATFORM_RESPONSE" | jq -r '.message')"
else
    echo -e "${RED}❌ 无效平台检测失败${NC}"
    echo "$NO_PLATFORM_RESPONSE"
    FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# 测试8: 日期格式验证
echo -e "\n${YELLOW}📝 测试8: 日期格式验证${NC}"
TOTAL_TESTS=$((TOTAL_TESTS + 1))

INVALID_DATE_RESPONSE=$(curl -s -X GET "${BASE_URL}${API_PREFIX}/external/platforms/${PLATFORM_ID}/logs?start_date=invalid-date&end_date=2025-13-32")

if echo "$INVALID_DATE_RESPONSE" | jq -e '.success == false and .error_code == "INVALID_PARAMETER"' >/dev/null 2>&1; then
    echo -e "${GREEN}✅ 日期格式验证正常${NC}"
    echo "Error Message: $(echo "$INVALID_DATE_RESPONSE" | jq -r '.message')"
else
    echo -e "${RED}❌ 日期格式验证失败${NC}"
    echo "$INVALID_DATE_RESPONSE"
    FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# 测试9: 缺失参数验证
echo -e "\n${YELLOW}📝 测试9: 缺失参数验证${NC}"
TOTAL_TESTS=$((TOTAL_TESTS + 1))

MISSING_PARAM_RESPONSE=$(curl -s -X GET "${BASE_URL}${API_PREFIX}/external/platforms/${PLATFORM_ID}/logs")

if echo "$MISSING_PARAM_RESPONSE" | jq -e '.success == false and .error_code == "MISSING_PARAMETER"' >/dev/null 2>&1; then
    echo -e "${GREEN}✅ 缺失参数验证正常${NC}"
    echo "Error Message: $(echo "$MISSING_PARAM_RESPONSE" | jq -r '.message')"
else
    echo -e "${RED}❌ 缺失参数验证失败${NC}"
    echo "$MISSING_PARAM_RESPONSE"
    FAILED_TESTS=$((FAILED_TESTS + 1))
fi

# 测试总结
echo -e "\n${YELLOW}📊 测试总结${NC}"
echo "========================================"
echo "总测试数: $TOTAL_TESTS"
echo -e "成功: ${GREEN}$((TOTAL_TESTS - FAILED_TESTS))${NC}"
echo -e "失败: ${RED}$FAILED_TESTS${NC}"

if [ $FAILED_TESTS -eq 0 ]; then
    echo -e "${GREEN}🎉 所有测试通过！V2 API功能正常${NC}"
    exit 0
else
    echo -e "${RED}❌ 有 $FAILED_TESTS 个测试失败，请检查实现${NC}"
    exit 1
fi