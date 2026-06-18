#!/bin/bash
# test_p3_new_features.sh — 优先级 3：新功能验收
#
# 覆盖清单项目: #11 ~ #15
#   #11 订阅系统（创建计划 → 用户订阅 → 消费扣减）
#   #12 自定义 OAuth 提供商
#   #13 渠道上游自动更新检测
#   #14 Waffo 支付入口
#   #15 繁体中文（zh-TW）语言完整性
#
# 用法:
#   BASE_URL=http://localhost:3000 \
#   ADMIN_TOKEN=xxx \
#   bash scripts/test_p3_new_features.sh

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/test_lib.sh"

check_deps

echo ""
echo -e "${BOLD}${CYAN}════════════════════════════════════════════════════${NC}"
echo -e "${BOLD}${CYAN}  优先级 3：新功能验收  ${NC}"
echo -e "${BOLD}${CYAN}════════════════════════════════════════════════════${NC}"
echo -e "  BASE_URL:    $BASE_URL"
echo -e "  ADMIN_TOKEN: ${ADMIN_TOKEN:+${ADMIN_TOKEN:0:8}...（已设置）}${ADMIN_TOKEN:-（未设置）}"

check_service

# ── #11 订阅系统 ──────────────────────────────────────────────────────────────
section "#11 订阅系统"

if [ -z "$ADMIN_TOKEN" ]; then
    skip "#11 未设置 ADMIN_TOKEN，跳过订阅接口测试"
else
    # 11-A: 列出订阅计划
    http_get "/api/subscription/plans" "$ADMIN_TOKEN"
    if [ "$HTTP_STATUS" = "200" ]; then
        pass "#11-A 订阅计划列表接口 (HTTP 200)"
        PLAN_COUNT=$(echo "$HTTP_BODY" | jq '.data | length' 2>/dev/null || echo "?")
        info "现有计划数: $PLAN_COUNT"
    elif [ "$HTTP_STATUS" = "404" ]; then
        fail "#11-A 订阅计划接口不存在 (HTTP 404) — 路由可能未注册"
    else
        warn "#11-A 订阅计划接口 HTTP $HTTP_STATUS"
        info "响应: $(echo "$HTTP_BODY" | head -c 200)"
    fi

    # 11-B: 创建测试订阅计划
    PLAN_NAME="test_plan_$(date +%s)"
    http_post "/api/subscription/plans" \
        "{
            \"name\": \"$PLAN_NAME\",
            \"description\": \"自动化测试计划\",
            \"quota\": 500000,
            \"price\": 9.9,
            \"duration_days\": 30,
            \"enabled\": true
        }" \
        "$ADMIN_TOKEN"

    if [ "$HTTP_STATUS" = "200" ] || [ "$HTTP_STATUS" = "201" ]; then
        PLAN_ID=$(echo "$HTTP_BODY" | jq -r '.data.id // .id // empty' 2>/dev/null)
        pass "#11-B 创建订阅计划成功 (id=$PLAN_ID)"

        # 11-C: 验证计划可以被查询
        if [ -n "$PLAN_ID" ]; then
            http_get "/api/subscription/plans/$PLAN_ID" "$ADMIN_TOKEN"
            if [ "$HTTP_STATUS" = "200" ]; then
                assert_jq "$HTTP_BODY" ".data.name // .name" "$PLAN_NAME" "#11-C 查询计划名称一致"
            else
                warn "#11-C 查询计划失败 (HTTP $HTTP_STATUS)"
            fi

            # 清理：删除测试计划
            http_del=$(curl -s -o /dev/null -w "%{http_code}" \
                -X DELETE \
                -H "Authorization: Bearer $ADMIN_TOKEN" \
                "$BASE_URL/api/subscription/plans/$PLAN_ID" 2>/dev/null || echo "0")
            [ "$http_del" = "200" ] && info "测试计划已清理"
        fi
    elif [ "$HTTP_STATUS" = "404" ]; then
        fail "#11-B 创建计划接口不存在 (HTTP 404)"
    else
        warn "#11-B 创建计划返回 HTTP $HTTP_STATUS"
        info "响应: $(echo "$HTTP_BODY" | head -c 300)"
    fi

    # 11-D: 检查用户订阅接口
    http_get "/api/subscription/user/self" "$ADMIN_TOKEN"
    if [ "$HTTP_STATUS" != "404" ]; then
        pass "#11-D 用户订阅查询接口存在 (HTTP $HTTP_STATUS)"
    else
        fail "#11-D 用户订阅查询接口不存在 (HTTP 404)"
    fi
fi

manual "#11 完整订阅流程" \
    "创建计划 → 后台为用户分配订阅 → 用该用户 Token 发起请求 → 确认订阅配额被消费"

# ── #12 自定义 OAuth 提供商 ───────────────────────────────────────────────────
section "#12 自定义 OAuth 提供商"

if [ -z "$ADMIN_TOKEN" ]; then
    skip "#12 未设置 ADMIN_TOKEN"
else
    # 检查自定义 OAuth CRUD 接口
    http_get "/api/oauth/custom/providers" "$ADMIN_TOKEN"
    if [ "$HTTP_STATUS" = "200" ]; then
        pass "#12-A 自定义 OAuth 提供商列表接口 (HTTP 200)"
    elif [ "$HTTP_STATUS" = "404" ]; then
        fail "#12-A 自定义 OAuth 提供商接口不存在 (HTTP 404)"
    else
        warn "#12-A HTTP $HTTP_STATUS"
        info "响应: $(echo "$HTTP_BODY" | head -c 200)"
    fi

    # 检查创建接口
    http_post "/api/oauth/custom/providers" \
        '{"name": "test_oidc", "client_id": "test", "client_secret": "test", "auth_url": "https://example.com/auth", "token_url": "https://example.com/token"}' \
        "$ADMIN_TOKEN"
    if [ "$HTTP_STATUS" != "404" ]; then
        pass "#12-B 自定义 OAuth 创建接口存在 (HTTP $HTTP_STATUS)"
    else
        fail "#12-B 自定义 OAuth 创建接口不存在 (HTTP 404)"
    fi
fi

manual "#12 自定义 OAuth 登录流程" \
    "配置一个有效的 OIDC provider → 在登录页面使用自定义 OAuth → 确认可以完成授权和用户创建"

# ── #13 渠道上游自动更新 ─────────────────────────────────────────────────────
section "#13 渠道上游自动更新检测"

if [ -z "$ADMIN_TOKEN" ]; then
    skip "#13 未设置 ADMIN_TOKEN"
else
    # 检查上游更新接口
    http_get "/api/channel/upstream_updates" "$ADMIN_TOKEN"
    if [ "$HTTP_STATUS" = "200" ]; then
        pass "#13-A 上游更新检测接口存在 (HTTP 200)"
        UPDATE_COUNT=$(echo "$HTTP_BODY" | jq '.data | length' 2>/dev/null || echo "?")
        info "检测到待更新渠道数: $UPDATE_COUNT"
    elif [ "$HTTP_STATUS" = "404" ]; then
        # 也可能在不同路径下
        http_get "/api/channel/check_upstream" "$ADMIN_TOKEN"
        if [ "$HTTP_STATUS" != "404" ]; then
            pass "#13-A 上游更新接口存在于 /api/channel/check_upstream (HTTP $HTTP_STATUS)"
        else
            fail "#13-A 未找到上游更新检测接口（尝试了 /upstream_updates 和 /check_upstream）"
        fi
    else
        warn "#13-A HTTP $HTTP_STATUS"
    fi

    # 检查渠道详情是否有 upstream_update 相关字段（通过渠道列表接口）
    http_get "/api/channel/?p=1&page_size=1" "$ADMIN_TOKEN"
    if echo "$HTTP_BODY" | jq -e '.data.data[0]' >/dev/null 2>&1; then
        CHANNEL_DATA=$(echo "$HTTP_BODY" | jq '.data.data[0]')
        info "渠道字段: $(echo "$CHANNEL_DATA" | jq 'keys' 2>/dev/null | head -c 200)"
        if echo "$CHANNEL_DATA" | jq -e '.upstream_models_count // .new_models_count // .has_upstream_update' >/dev/null 2>&1; then
            pass "#13-B 渠道数据包含上游更新字段"
        else
            warn "#13-B 渠道数据中未发现上游更新字段，请在前端渠道管理页面手动确认"
        fi
    else
        warn "#13-B 无法获取渠道详情验证字段"
    fi
fi

# ── #14 Waffo 支付入口 ────────────────────────────────────────────────────────
section "#14 Waffo 支付入口"

# 检查 Waffo 充值接口（不带参数，期望 400/401，不期望 404）
http_post "/api/user/topup/waffo" '{}' "${RELAY_TOKEN:-${ADMIN_TOKEN:-}}"
if [ "$HTTP_STATUS" != "404" ]; then
    pass "#14 Waffo 支付接口存在 (HTTP $HTTP_STATUS)"
else
    # 尝试其他可能的路径
    http_post "/api/pay/waffo" '{}' "${RELAY_TOKEN:-${ADMIN_TOKEN:-}}"
    if [ "$HTTP_STATUS" != "404" ]; then
        pass "#14 Waffo 支付接口存在于 /api/pay/waffo (HTTP $HTTP_STATUS)"
    else
        fail "#14 Waffo 支付接口不存在 (HTTP 404 on both paths)"
    fi
fi

# 验证 Waffo 是否出现在支付选项中（通过前端或配置接口）
http_get "/api/status"
if echo "$HTTP_BODY" | jq -e '.waffo_enabled // .payment_options' >/dev/null 2>&1; then
    info "支付选项: $(echo "$HTTP_BODY" | jq '.payment_options // .waffo_enabled' 2>/dev/null)"
fi

manual "#14 Waffo 支付完整流程" \
    "在管理后台配置 Waffo 商户 ID/密钥 → 在充值页面确认 Waffo 选项出现 → 完成一笔测试充值"

# ── #15 繁体中文（zh-TW）完整性检查 ─────────────────────────────────────────
section "#15 繁体中文 (zh-TW) 语言文件完整性"

ZH_TW_FILE="/Users/rrong/Documents/perf/projects/new-api-0422/web/src/i18n/locales/zh-TW.json"
ZH_CN_FILE="/Users/rrong/Documents/perf/projects/new-api-0422/web/src/i18n/locales/zh-CN.json"

if [ -f "$ZH_TW_FILE" ]; then
    pass "#15-A zh-TW.json 文件存在"

    # 检查文件是否为有效 JSON
    if jq empty "$ZH_TW_FILE" 2>/dev/null; then
        pass "#15-A zh-TW.json 是有效 JSON"

        # 比较 key 数量
        TW_KEYS=$(jq 'keys | length' "$ZH_TW_FILE" 2>/dev/null || echo 0)
        CN_KEYS=$(jq 'keys | length' "$ZH_CN_FILE" 2>/dev/null || echo 0)
        info "zh-TW key 数: $TW_KEYS / zh-CN key 数: $CN_KEYS"

        # 找出 zh-CN 有但 zh-TW 没有的 key（缺失翻译）
        MISSING_KEYS=$(jq -r --slurpfile tw "$ZH_TW_FILE" \
            'keys[] | select(. as $k | ($tw[0] | has($k)) | not)' \
            "$ZH_CN_FILE" 2>/dev/null | head -20 || echo "")

        if [ -z "$MISSING_KEYS" ]; then
            pass "#15-B zh-TW 翻译 key 完整（无缺失）"
        else
            MISSING_COUNT=$(echo "$MISSING_KEYS" | wc -l | tr -d ' ')
            warn "#15-B zh-TW 缺少 $MISSING_COUNT 个翻译 key（相对于 zh-CN）"
            info "缺失的 key（前20）:"
            echo "$MISSING_KEYS" | while read -r k; do info "  - $k"; done
            # 超过 10% 才算 FAIL
            THRESHOLD=$(( CN_KEYS / 10 ))
            if [ "$MISSING_COUNT" -gt "$THRESHOLD" ] 2>/dev/null; then
                fail "#15-B 缺失 key 数量 ($MISSING_COUNT) 超过 10%，需要补充翻译"
            else
                pass "#15-B 缺失 key 在可接受范围内 ($MISSING_COUNT < $THRESHOLD)"
            fi
        fi

        # 检查一些核心 key 是否存在（用中文 source key）
        CORE_KEYS=("登录" "注册" "渠道" "令牌" "日志")
        for k in "${CORE_KEYS[@]}"; do
            if jq -e --arg k "$k" 'has($k)' "$ZH_TW_FILE" >/dev/null 2>&1; then
                pass "#15-C 核心 key 存在: '$k'"
            else
                warn "#15-C 核心 key 缺失: '$k'"
            fi
        done
    else
        fail "#15-A zh-TW.json 不是有效的 JSON 文件"
    fi
else
    fail "#15 zh-TW.json 文件不存在: $ZH_TW_FILE"
fi

# ── 汇总 ─────────────────────────────────────────────────────────────────────
print_summary "优先级 3 · 新功能验收"
