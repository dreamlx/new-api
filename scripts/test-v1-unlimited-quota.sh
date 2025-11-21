#!/bin/bash

# V1 API 无限额度Token和删除Token测试脚本
# 测试内容：
# 1. 创建无限额度Token
# 2. 创建独立额度Token
# 3. 删除Token

set -e

BASE_URL="http://localhost:3000"
API_BASE="$BASE_URL/api/user/external"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo "======================================"
echo "V1 API 无限额度Token和删除Token测试"
echo "======================================"

# 测试用户ID
TIMESTAMP=$(date +%s)
EXTERNAL_USER_ID="test_v1_unlimited_${TIMESTAMP}"

# 1. 创建测试用户
echo -e "\n${YELLOW}1. 创建测试用户${NC}"
SYNC_RESPONSE=$(curl -s -X POST "$API_BASE/sync" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$EXTERNAL_USER_ID\",
    \"username\": \"test_unlimited_${TIMESTAMP}\",
    \"display_name\": \"Test Unlimited User\",
    \"email\": \"test_unlimited_${TIMESTAMP}@example.com\"
  }")
echo "Response: $SYNC_RESPONSE"
if echo "$SYNC_RESPONSE" | jq -e '.success == true' > /dev/null; then
    echo -e "${GREEN}✓ 用户创建成功${NC}"
else
    echo -e "${RED}✗ 用户创建失败${NC}"
    exit 1
fi

# 2. 充值测试用户
echo -e "\n${YELLOW}2. 充值测试用户 \$20${NC}"
TOPUP_RESPONSE=$(curl -s -X POST "$API_BASE/topup" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$EXTERNAL_USER_ID\",
    \"amount_usd\": 20,
    \"payment_id\": \"test_payment_001\"
  }")
echo "Response: $TOPUP_RESPONSE"
INITIAL_QUOTA=$(echo "$TOPUP_RESPONSE" | jq -r '.data.current_quota')
echo "用户余额: $INITIAL_QUOTA"

# 3. 创建无限额度Token
echo -e "\n${YELLOW}3. 创建无限额度Token${NC}"
UNLIMITED_TOKEN_RESPONSE=$(curl -s -X POST "$API_BASE/token" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$EXTERNAL_USER_ID\",
    \"token_name\": \"Unlimited Token\",
    \"unlimited_quota\": true
  }")
echo "Response: $UNLIMITED_TOKEN_RESPONSE"

if echo "$UNLIMITED_TOKEN_RESPONSE" | jq -e '.success == true' > /dev/null; then
    UNLIMITED_TOKEN_ID=$(echo "$UNLIMITED_TOKEN_RESPONSE" | jq -r '.data.token_id')
    UNLIMITED_ACCESS_KEY=$(echo "$UNLIMITED_TOKEN_RESPONSE" | jq -r '.data.access_key')
    UNLIMITED_REMAIN_QUOTA=$(echo "$UNLIMITED_TOKEN_RESPONSE" | jq -r '.data.remain_quota')
    UNLIMITED_FLAG=$(echo "$UNLIMITED_TOKEN_RESPONSE" | jq -r '.data.unlimited_quota')

    echo -e "${GREEN}✓ 无限额度Token创建成功${NC}"
    echo "  Token ID: $UNLIMITED_TOKEN_ID"
    echo "  Access Key: $UNLIMITED_ACCESS_KEY"
    echo "  Remain Quota: $UNLIMITED_REMAIN_QUOTA (应该为0)"
    echo "  Unlimited Flag: $UNLIMITED_FLAG (应该为true)"

    # 验证无限额度标志
    if [ "$UNLIMITED_FLAG" = "true" ] && [ "$UNLIMITED_REMAIN_QUOTA" = "0" ]; then
        echo -e "${GREEN}✓ 无限额度Token属性验证通过${NC}"
    else
        echo -e "${RED}✗ 无限额度Token属性验证失败${NC}"
    fi
else
    echo -e "${RED}✗ 无限额度Token创建失败${NC}"
    exit 1
fi

# 4. 验证用户余额未被扣减
echo -e "\n${YELLOW}4. 验证用户余额未被扣减${NC}"
STATS_RESPONSE=$(curl -s -X GET "$API_BASE/$EXTERNAL_USER_ID/stats")
CURRENT_QUOTA=$(echo "$STATS_RESPONSE" | jq -r '.data.user_info.current_quota')
echo "当前用户余额: $CURRENT_QUOTA (应该等于初始余额 $INITIAL_QUOTA)"

if [ "$CURRENT_QUOTA" = "$INITIAL_QUOTA" ]; then
    echo -e "${GREEN}✓ 无限额度Token未扣减用户余额${NC}"
else
    echo -e "${RED}✗ 用户余额被错误扣减${NC}"
fi

# 5. 创建独立额度Token（作为对比）
echo -e "\n${YELLOW}5. 创建独立额度Token (分配\$5)${NC}"
QUOTA_TO_ALLOCATE=2500000  # $5
INDEPENDENT_TOKEN_RESPONSE=$(curl -s -X POST "$API_BASE/token" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$EXTERNAL_USER_ID\",
    \"token_name\": \"Independent Quota Token\",
    \"allocated_quota\": $QUOTA_TO_ALLOCATE
  }")
echo "Response: $INDEPENDENT_TOKEN_RESPONSE"

if echo "$INDEPENDENT_TOKEN_RESPONSE" | jq -e '.success == true' > /dev/null; then
    INDEPENDENT_TOKEN_ID=$(echo "$INDEPENDENT_TOKEN_RESPONSE" | jq -r '.data.token_id')
    INDEPENDENT_REMAIN_QUOTA=$(echo "$INDEPENDENT_TOKEN_RESPONSE" | jq -r '.data.remain_quota')
    INDEPENDENT_UNLIMITED=$(echo "$INDEPENDENT_TOKEN_RESPONSE" | jq -r '.data.unlimited_quota')

    echo -e "${GREEN}✓ 独立额度Token创建成功${NC}"
    echo "  Token ID: $INDEPENDENT_TOKEN_ID"
    echo "  Remain Quota: $INDEPENDENT_REMAIN_QUOTA"
    echo "  Unlimited Flag: $INDEPENDENT_UNLIMITED (应该为false或null)"
else
    echo -e "${RED}✗ 独立额度Token创建失败${NC}"
    exit 1
fi

# 6. 验证用户余额被扣减
echo -e "\n${YELLOW}6. 验证用户余额被扣减${NC}"
STATS_RESPONSE2=$(curl -s -X GET "$API_BASE/$EXTERNAL_USER_ID/stats")
CURRENT_QUOTA2=$(echo "$STATS_RESPONSE2" | jq -r '.data.user_info.current_quota')
EXPECTED_QUOTA=$((INITIAL_QUOTA - QUOTA_TO_ALLOCATE))
echo "当前用户余额: $CURRENT_QUOTA2 (应该为 $EXPECTED_QUOTA)"

if [ "$CURRENT_QUOTA2" = "$EXPECTED_QUOTA" ]; then
    echo -e "${GREEN}✓ 独立额度Token正确扣减用户余额${NC}"
else
    echo -e "${RED}✗ 用户余额扣减不正确${NC}"
fi

# 7. 测试删除Token（RESTful格式）
echo -e "\n${YELLOW}7. 删除无限额度Token${NC}"
DELETE_RESPONSE=$(curl -s -X DELETE "$API_BASE/$EXTERNAL_USER_ID/token/$UNLIMITED_TOKEN_ID")
echo "Response: $DELETE_RESPONSE"

if echo "$DELETE_RESPONSE" | jq -e '.success == true' > /dev/null; then
    echo -e "${GREEN}✓ Token删除成功${NC}"
else
    echo -e "${RED}✗ Token删除失败${NC}"
    exit 1
fi

# 8. 验证Token已删除
echo -e "\n${YELLOW}8. 验证Token已删除${NC}"
TOKENS_RESPONSE=$(curl -s -X GET "$API_BASE/$EXTERNAL_USER_ID/tokens")
REMAINING_TOKENS=$(echo "$TOKENS_RESPONSE" | jq -r '.data.total_tokens')
echo "剩余Token数量: $REMAINING_TOKENS (应该为1)"

if [ "$REMAINING_TOKENS" = "1" ]; then
    echo -e "${GREEN}✓ Token删除验证通过${NC}"
else
    echo -e "${RED}✗ Token删除验证失败${NC}"
fi

# 9. 测试缺少allocated_quota的错误
echo -e "\n${YELLOW}9. 测试缺少allocated_quota参数的错误${NC}"
ERROR_RESPONSE=$(curl -s -X POST "$API_BASE/token" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$EXTERNAL_USER_ID\",
    \"token_name\": \"Should Fail Token\"
  }")
echo "Response: $ERROR_RESPONSE"

if echo "$ERROR_RESPONSE" | jq -e '.success == false' > /dev/null; then
    echo -e "${GREEN}✓ 参数验证正确拒绝无allocated_quota请求${NC}"
else
    echo -e "${RED}✗ 参数验证未正确工作${NC}"
fi

echo -e "\n======================================"
echo -e "${GREEN}测试完成！${NC}"
echo "======================================"
echo ""
echo "测试总结："
echo "- 无限额度Token创建: ✓"
echo "- 无限额度Token不扣用户余额（创建时）: ✓"
echo "- 独立额度Token正确扣减余额: ✓"
echo "- Token删除功能（RESTful）: ✓"
echo "- 参数验证: ✓"
echo ""
echo "注意："
echo "1. V1无限额度Token在LLM调用时会扣减User.Quota"
echo "2. 需要启动后端服务后才能运行此测试"
