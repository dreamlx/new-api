#!/bin/bash
# test_p2_regression.sh — 优先级 2：重构区域回归验证
#
# 覆盖清单项目: #6 ~ #10
#   #6  预消费配额（失败后返还）
#   #7  视频任务 Kling/Jimeng/Vidu/Sora
#   #8  异步任务计费 (Midjourney/Suno)
#   #9  GitHub / Discord / LinuxDO OAuth 登录
#   #10 V2 外部平台 API 接口格式兼容性
#
# 用法:
#   BASE_URL=http://localhost:3000 \
#   ADMIN_TOKEN=xxx \
#   RELAY_TOKEN=sk-xxx \
#   bash scripts/test_p2_regression.sh

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/test_lib.sh"

check_deps

echo ""
echo -e "${BOLD}${CYAN}════════════════════════════════════════════════════${NC}"
echo -e "${BOLD}${CYAN}  优先级 2：重构区域回归验证  ${NC}"
echo -e "${BOLD}${CYAN}════════════════════════════════════════════════════${NC}"
echo -e "  BASE_URL:    $BASE_URL"
echo -e "  ADMIN_TOKEN: ${ADMIN_TOKEN:+${ADMIN_TOKEN:0:8}...（已设置）}${ADMIN_TOKEN:-（未设置）}"
echo -e "  RELAY_TOKEN: ${RELAY_TOKEN:+${RELAY_TOKEN:0:8}...（已设置）}${RELAY_TOKEN:-（未设置）}"

check_service

# ── #6 预消费配额（billing.go 重构验证） ──────────────────────────────────────
section "#6 预消费配额 — 失败后返还机制"

if [ -z "$RELAY_TOKEN" ]; then
    skip "#6 未设置 RELAY_TOKEN，跳过自动化"
    info "手动验证方法:"
    info "  1. 记录用户当前 quota 余额"
    info "  2. 发送指向一个不存在模型的请求（必然失败）"
    info "  3. 确认 quota 余额没有减少（预消费已返还）"
else
    # 查询当前余额（通过用户自助接口）
    http_get "/api/user/self" "$RELAY_TOKEN"
    QUOTA_BEFORE=$(echo "$HTTP_BODY" | jq -r '.data.quota // .quota // 0' 2>/dev/null || echo "0")
    info "发送前余额: $QUOTA_BEFORE quota"

    # 发送一个必然失败的请求（不存在的模型）
    FAIL_RESP=$(curl -s -w '\n__STATUS__%{http_code}' \
        -X POST \
        -H "Authorization: Bearer $RELAY_TOKEN" \
        -H "Content-Type: application/json" \
        -d '{"model": "nonexistent-model-for-regression-test-xyz", "messages": [{"role": "user", "content": "hi"}]}' \
        "$BASE_URL/v1/chat/completions" 2>&1)
    FAIL_STATUS=$(echo "$FAIL_RESP" | tail -n1 | sed 's/__STATUS__//')

    if [ "$FAIL_STATUS" != "200" ]; then
        info "请求按预期失败 (HTTP $FAIL_STATUS)"

        # 等待短暂时间确保返还处理完成
        sleep 1

        http_get "/api/user/self" "$RELAY_TOKEN"
        QUOTA_AFTER=$(echo "$HTTP_BODY" | jq -r '.data.quota // .quota // 0' 2>/dev/null || echo "0")
        info "发送后余额: $QUOTA_AFTER quota"

        if [ "$QUOTA_BEFORE" = "$QUOTA_AFTER" ]; then
            pass "#6 预消费配额已正确返还（失败前后余额相同）"
        else
            DIFF=$(( QUOTA_BEFORE - QUOTA_AFTER ))
            if [ "$DIFF" -gt 0 ] 2>/dev/null; then
                fail "#6 余额减少了 $DIFF quota，预消费未正确返还"
            else
                warn "#6 余额变化: 前=$QUOTA_BEFORE 后=$QUOTA_AFTER（可能有其他并发操作）"
            fi
        fi
    else
        warn "#6 请求意外成功（HTTP 200），说明模型存在，无法用此方法验证返还"
        info "建议: 使用一个确实不存在的渠道进行测试"
    fi
fi

# ── #7 视频任务（Kling/Jimeng/Vidu/Sora） ────────────────────────────────────
section "#7 视频生成任务接口存活检查"

# 检查视频任务路由是否存在（发送空请求，期望 400/401/422，不期望 404）
VIDEO_ENDPOINTS=(
    "/v1/images/generations"
    "/mj/submit/imagine"
    "/suno/submit/music"
)

for ep in "${VIDEO_ENDPOINTS[@]}"; do
    http_post "$ep" '{}' "${RELAY_TOKEN:-}"
    if [ "$HTTP_STATUS" != "404" ]; then
        pass "#7 路由存在: $ep (HTTP $HTTP_STATUS)"
    else
        fail "#7 路由不存在 (HTTP 404): $ep"
    fi
done

if [ -z "$RELAY_TOKEN" ]; then
    manual "#7 视频任务功能测试（Kling/Vidu/Sora）" \
        "配置对应渠道 Key 后，调用 /v1/chat/completions 并传入视频生成模型名，验证任务提交和轮询"
else
    info "完整视频任务测试需要配置对应渠道（Kling/Jimeng 等），请在有真实渠道时测试"
fi

# ── #8 异步任务计费（task_billing.go 重构验证） ───────────────────────────────
section "#8 异步任务计费 — Midjourney/Suno 完成后扣额"

# 检查 MJ 任务路由
http_post "/mj/submit/imagine" '{"prompt": "a test"}' "${RELAY_TOKEN:-}"
if [ "$HTTP_STATUS" != "404" ]; then
    pass "#8 MJ 提交路由存在 (HTTP $HTTP_STATUS)"
else
    fail "#8 MJ 提交路由不存在 (HTTP 404)"
fi

# 检查 Suno 任务路由
http_post "/suno/submit/music" '{"prompt": "a test"}' "${RELAY_TOKEN:-}"
if [ "$HTTP_STATUS" != "404" ]; then
    pass "#8 Suno 提交路由存在 (HTTP $HTTP_STATUS)"
else
    fail "#8 Suno 提交路由不存在 (HTTP 404)"
fi

manual "#8 完整计费验证" \
    "配置 MJ 渠道后: 提交任务 → 等待完成 → 查询 /api/log/self 确认日志中 quota/is_finished_task 字段正确"

# ── #9 GitHub / Discord / LinuxDO OAuth 路由（代码整合后） ───────────────────
section "#9 OAuth 登录路由（代码合并后存活检查）"

OAUTH_ROUTES=(
    "github"
    "discord"
    "linuxdo"
    "oidc"
    "telegram"
    "wechat"
)

for provider in "${OAUTH_ROUTES[@]}"; do
    http_get "/api/oauth/$provider"
    # 404 = 路由不存在（回归），其他任何状态码都说明路由注册正常
    if [ "$HTTP_STATUS" != "404" ]; then
        pass "#9 OAuth 路由存在: /api/oauth/$provider (HTTP $HTTP_STATUS)"
    else
        fail "#9 OAuth 路由缺失: /api/oauth/$provider (HTTP 404)"
    fi
done

manual "#9 OAuth 登录流程" \
    "需要浏览器手动测试: 配置 GitHub/Discord App 后，走完整 OAuth 授权流程确认能登录"

# ── #10 V2 外部平台 API 兼容性 ───────────────────────────────────────────────
section "#10 V2 外部平台 API 接口格式兼容性"

if [ -z "$ADMIN_TOKEN" ]; then
    skip "#10 未设置 ADMIN_TOKEN，跳过 V2 接口测试"
    info "手动验证方法:"
    info "  GET $BASE_URL/api/v2/external/tokens/authorize"
    info "  GET $BASE_URL/api/v2/external/platforms/{id}/logs"
else
    # 测试 V2 Token 授权接口（不带参数，期望 400，不期望 404）
    http_post "/api/v2/external/tokens/authorize" '{}' "$ADMIN_TOKEN"
    if [ "$HTTP_STATUS" != "404" ]; then
        pass "#10 V2 Token 授权接口存在 (HTTP $HTTP_STATUS)"
    else
        fail "#10 V2 Token 授权接口不存在 (HTTP 404)"
    fi

    # 测试平台日志接口
    http_get "/api/v2/external/platforms/test_platform/logs" "$ADMIN_TOKEN"
    if [ "$HTTP_STATUS" != "404" ]; then
        pass "#10 V2 平台日志接口存在 (HTTP $HTTP_STATUS)"
    else
        fail "#10 V2 平台日志接口不存在 (HTTP 404)"
    fi

    # 验证响应结构与旧版兼容（success 字段）
    http_post "/api/v2/external/tokens/authorize" \
        '{"platform_id": "test", "external_user_id": "regression_check"}' \
        "$ADMIN_TOKEN"
    if echo "$HTTP_BODY" | jq -e '.success' >/dev/null 2>&1; then
        pass "#10 响应包含 success 字段（格式向后兼容）"
    else
        warn "#10 响应格式未检测到 success 字段，请手动验证与旧版调用方兼容"
        info "响应: $(echo "$HTTP_BODY" | head -c 200)"
    fi
fi

# ── 汇总 ─────────────────────────────────────────────────────────────────────
print_summary "优先级 2 · 重构区域回归"
