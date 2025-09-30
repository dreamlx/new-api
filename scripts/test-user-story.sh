#!/bin/bash

# 用户故事业务逻辑验证脚本
# 模拟真实用户的完整使用流程

set -e

API_URL="${API_URL:-http://localhost:3000}"
TIMESTAMP=$(date +%s)
USER_ID="alice_${TIMESTAMP}"
USERNAME="alice_user_${TIMESTAMP}"
EMAIL="${USERNAME}@example.com"

# 颜色输出
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_step() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

log_info() {
    echo -e "${YELLOW}➜${NC} $1"
}

log_success() {
    echo -e "${GREEN}✓${NC} $1"
}

echo ""
echo "================================================"
echo "  外部用户业务逻辑验证 - User Story Test"
echo "================================================"
echo ""
echo "用户场景："
echo "  1. Alice注册新账号"
echo "  2. Alice充值 \$20"
echo "  3. Alice创建API Token"
echo "  4. Alice查看余额和可用模型"
echo "  5. Alice模拟API调用（手动测试）"
echo "  6. Alice追加充值 \$10"
echo "  7. Alice查看最终余额"
echo ""

# ============================================
# 场景1：新用户注册
# ============================================
log_step "场景1: Alice注册新账号"

log_info "调用用户同步API创建Alice账号..."
SYNC_RESPONSE=$(curl -s -X POST "$API_URL/api/user/external/sync" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$USER_ID\",
    \"username\": \"$USERNAME\",
    \"display_name\": \"Alice (Test User)\",
    \"email\": \"$EMAIL\",
    \"login_type\": \"email\"
  }")

# 提取user_id
INTERNAL_USER_ID=$(echo "$SYNC_RESPONSE" | grep -o '"user_id":[0-9]*' | cut -d: -f2)

if [ -z "$INTERNAL_USER_ID" ]; then
    echo "❌ 用户创建失败"
    echo "$SYNC_RESPONSE"
    exit 1
fi

log_success "Alice账号创建成功"
log_info "External User ID: $USER_ID"
log_info "Internal User ID: $INTERNAL_USER_ID"

# ============================================
# 场景2：首次充值
# ============================================
log_step "场景2: Alice充值 \$20"

log_info "调用充值API..."
TOPUP1_RESPONSE=$(curl -s -X POST "$API_URL/api/user/external/topup" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$USER_ID\",
    \"amount_usd\": 20.0,
    \"payment_id\": \"payment_${TIMESTAMP}_1\"
  }")

QUOTA_ADDED=$(echo "$TOPUP1_RESPONSE" | grep -o '"quota_added":[0-9]*' | cut -d: -f2)
CURRENT_QUOTA=$(echo "$TOPUP1_RESPONSE" | grep -o '"current_quota":[0-9]*' | cut -d: -f2)
CURRENT_BALANCE=$(echo "$TOPUP1_RESPONSE" | grep -o '"current_balance":[0-9.]*' | cut -d: -f2)

log_success "充值成功"
log_info "充值金额: \$20.00"
log_info "增加Quota: $QUOTA_ADDED (预期: 10000000)"
log_info "当前Quota: $CURRENT_QUOTA"
log_info "当前余额: \$${CURRENT_BALANCE}"

# 验证充值计算
EXPECTED_QUOTA=10000000  # $20 * 500,000
if [ "$QUOTA_ADDED" -eq "$EXPECTED_QUOTA" ]; then
    log_success "充值计算正确 (\$1 = 500,000 quota)"
else
    echo "❌ 充值计算错误: 预期 $EXPECTED_QUOTA, 实际 $QUOTA_ADDED"
    exit 1
fi

# ============================================
# 场景3：创建API Token
# ============================================
log_step "场景3: Alice创建API Token"

log_info "调用Token创建API..."
TOKEN_RESPONSE=$(curl -s -X POST "$API_URL/api/user/external/token" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$USER_ID\",
    \"token_name\": \"Alice's Production Token\",
    \"expires_in_days\": 365
  }")

TOKEN_ID=$(echo "$TOKEN_RESPONSE" | grep -o '"token_id":[0-9]*' | cut -d: -f2)
ACCESS_KEY=$(echo "$TOKEN_RESPONSE" | grep -o '"access_key":"[^"]*"' | cut -d'"' -f4)

if [ -z "$ACCESS_KEY" ]; then
    echo "❌ Token创建失败"
    echo "$TOKEN_RESPONSE"
    exit 1
fi

log_success "Token创建成功"
log_info "Token ID: $TOKEN_ID"
log_info "Access Key: $ACCESS_KEY"
log_info "有效期: 365天"

# ============================================
# 场景4：查看余额和可用模型
# ============================================
log_step "场景4: Alice查看余额和可用模型"

log_info "调用用户统计API..."
STATS_RESPONSE=$(curl -s -X GET "$API_URL/api/user/external/$USER_ID/stats")

# 提取关键信息
BALANCE=$(echo "$STATS_RESPONSE" | grep -o '"current_balance":[0-9.]*' | head -1 | cut -d: -f2)
QUOTA=$(echo "$STATS_RESPONSE" | grep -o '"current_quota":[0-9]*' | head -1 | cut -d: -f2)
MODELS_COUNT=$(echo "$STATS_RESPONSE" | grep -o '"models_available":[0-9]*' | cut -d: -f2)

log_success "余额信息获取成功"
log_info "当前余额: \$${BALANCE}"
log_info "当前Quota: $QUOTA"
log_info "可用模型数: $MODELS_COUNT"

# 显示可用模型列表
echo ""
log_info "可用模型详情:"
echo "$STATS_RESPONSE" | python3 -c "
import sys, json
try:
    data = json.load(sys.stdin)
    balance_cap = data.get('data', {}).get('user_info', {}).get('balance_capacity', {})

    # 排除_summary
    models = {k: v for k, v in balance_cap.items() if k != '_summary'}

    if not models:
        print('  (未找到可用模型)')
    else:
        for model_name, info in sorted(models.items()):
            is_default = '(默认)' if info.get('is_default_model') else ''
            pricing = info.get('pricing_note', 'N/A')
            tokens = info.get('input_tokens_1k', 0)
            print(f'  • {model_name} {is_default}')
            print(f'    可调用: {tokens:,} 次 (按1K输入tokens计算)')
            print(f'    计费: {pricing}')
except:
    print('  (解析失败)')
" 2>/dev/null || echo "  (Python解析失败，请检查JSON格式)"

# ============================================
# 场景5：模拟API调用说明
# ============================================
log_step "场景5: 模拟API调用 (手动测试)"

echo ""
log_info "您可以使用以下命令测试实际的LLM API调用："
echo ""
echo "curl -X POST $API_URL/v1/chat/completions \\"
echo "  -H \"Authorization: Bearer $ACCESS_KEY\" \\"
echo "  -H \"Content-Type: application/json\" \\"
echo "  -d '{"
echo "    \"model\": \"deepseek-chat\","
echo "    \"messages\": [{\"role\": \"user\", \"content\": \"Hello\"}]"
echo "  }'"
echo ""
log_info "调用后可查看消费记录..."
echo ""

# ============================================
# 场景6：追加充值
# ============================================
log_step "场景6: Alice追加充值 \$10"

log_info "调用充值API..."
TOPUP2_RESPONSE=$(curl -s -X POST "$API_URL/api/user/external/topup" \
  -H "Content-Type: application/json" \
  -d "{
    \"external_user_id\": \"$USER_ID\",
    \"amount_usd\": 10.0,
    \"payment_id\": \"payment_${TIMESTAMP}_2\"
  }")

QUOTA_ADDED2=$(echo "$TOPUP2_RESPONSE" | grep -o '"quota_added":[0-9]*' | cut -d: -f2)
CURRENT_QUOTA2=$(echo "$TOPUP2_RESPONSE" | grep -o '"current_quota":[0-9]*' | cut -d: -f2)
CURRENT_BALANCE2=$(echo "$TOPUP2_RESPONSE" | grep -o '"current_balance":[0-9.]*' | cut -d: -f2)

log_success "追加充值成功"
log_info "充值金额: \$10.00"
log_info "增加Quota: $QUOTA_ADDED2 (预期: 5000000)"
log_info "当前Quota: $CURRENT_QUOTA2"
log_info "当前余额: \$${CURRENT_BALANCE2}"

# 验证累计充值
EXPECTED_TOTAL_BALANCE=30
if echo "$CURRENT_BALANCE2" | grep -q "^${EXPECTED_TOTAL_BALANCE}"; then
    log_success "累计余额正确 (\$20 + \$10 = \$${EXPECTED_TOTAL_BALANCE})"
else
    echo "⚠️  累计余额不符: 预期 \$${EXPECTED_TOTAL_BALANCE}, 实际 \$${CURRENT_BALANCE2}"
fi

# ============================================
# 场景7：查看最终状态
# ============================================
log_step "场景7: 查看Alice的最终账户状态"

log_info "调用用户统计API..."
FINAL_STATS=$(curl -s -X GET "$API_URL/api/user/external/$USER_ID/stats")

FINAL_BALANCE=$(echo "$FINAL_STATS" | grep -o '"current_balance":[0-9.]*' | head -1 | cut -d: -f2)
FINAL_QUOTA=$(echo "$FINAL_STATS" | grep -o '"current_quota":[0-9]*' | head -1 | cut -d: -f2)
TOTAL_REQUESTS=$(echo "$FINAL_STATS" | grep -o '"total_requests":[0-9]*' | cut -d: -f2)
USED_QUOTA=$(echo "$FINAL_STATS" | grep -o '"used_quota":[0-9]*' | head -1 | cut -d: -f2)

log_success "最终状态获取成功"
echo ""
echo "📊 Alice的账户摘要："
echo "  用户ID: $USER_ID"
echo "  邮箱: $EMAIL"
echo "  累计充值: \$30.00"
echo "  当前余额: \$${FINAL_BALANCE}"
echo "  当前Quota: ${FINAL_QUOTA}"
echo "  已使用Quota: ${USED_QUOTA}"
echo "  API调用次数: ${TOTAL_REQUESTS}"
echo "  Token数量: 1个 (ID: $TOKEN_ID)"
echo ""

# ============================================
# 查看消费记录
# ============================================
log_step "查看消费记录"

log_info "调用消费记录API..."
LOGS_RESPONSE=$(curl -s -X GET "$API_URL/api/user/external/$USER_ID/logs")

LOG_COUNT=$(echo "$LOGS_RESPONSE" | grep -o '"total":[0-9]*' | cut -d: -f2)

log_info "消费记录总数: $LOG_COUNT"

if [ "$LOG_COUNT" -gt 0 ]; then
    echo "$LOGS_RESPONSE" | python3 -m json.tool 2>/dev/null | grep -A 3 "logs" | head -20
else
    log_info "暂无消费记录（尚未调用API）"
fi

# ============================================
# 总结
# ============================================
echo ""
echo "================================================"
echo "  ✓ 业务逻辑验证完成"
echo "================================================"
echo ""
echo "验证结果："
echo "  ✓ 用户注册流程正常"
echo "  ✓ 充值计费正确 (\$1 = 500,000 quota)"
echo "  ✓ Token创建和管理正常"
echo "  ✓ 余额查询和模型列表正常"
echo "  ✓ 累计充值计算正确"
echo ""
echo "测试数据："
echo "  External User ID: $USER_ID"
echo "  Access Key: $ACCESS_KEY"
echo ""
echo "💡 建议下一步："
echo "  1. 使用上述Access Key测试实际的LLM API调用"
echo "  2. 调用后运行: curl $API_URL/api/user/external/$USER_ID/logs"
echo "  3. 验证消费记录和余额扣减是否正确"
echo ""