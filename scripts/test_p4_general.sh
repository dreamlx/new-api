#!/bin/bash
# test_p4_general.sh — 优先级 4：通用功能回归
#
# 覆盖清单项目: #16 ~ #20
#   #16 渠道亲和度缓存（同 Token 连续请求落到同一渠道）
#   #17 OpenRouter 兼容 API（刚恢复的功能）
#   #18 WebAuthn/Passkey 登录（路由 + 接口存活）
#   #19 Claude/Gemini 缓存计费（cache_tokens 字段）
#   #20 Pyroscope 性能监控（接口存活）
#
# 用法:
#   BASE_URL=http://localhost:3000 \
#   ADMIN_TOKEN=xxx \
#   RELAY_TOKEN=sk-xxx \
#   RELAY_MODEL=gpt-3.5-turbo \
#   bash scripts/test_p4_general.sh

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/test_lib.sh"

check_deps

echo ""
echo -e "${BOLD}${CYAN}════════════════════════════════════════════════════${NC}"
echo -e "${BOLD}${CYAN}  优先级 4：通用功能回归  ${NC}"
echo -e "${BOLD}${CYAN}════════════════════════════════════════════════════${NC}"
echo -e "  BASE_URL:    $BASE_URL"
echo -e "  ADMIN_TOKEN: ${ADMIN_TOKEN:+${ADMIN_TOKEN:0:8}...（已设置）}${ADMIN_TOKEN:-（未设置）}"
echo -e "  RELAY_TOKEN: ${RELAY_TOKEN:+${RELAY_TOKEN:0:8}...（已设置）}${RELAY_TOKEN:-（未设置）}"
echo -e "  RELAY_MODEL: $RELAY_MODEL"

check_service

# ── #16 渠道亲和度缓存 ────────────────────────────────────────────────────────
section "#16 渠道亲和度缓存"

if [ -z "$ADMIN_TOKEN" ]; then
    skip "#16 未设置 ADMIN_TOKEN，跳过亲和度缓存管理接口测试"
else
    # 检查亲和度缓存接口
    http_get "/api/log/channel_affinity_usage_cache" "$ADMIN_TOKEN"
    if [ "$HTTP_STATUS" = "200" ]; then
        pass "#16-A 亲和度缓存统计接口 (HTTP 200)"
        CACHE_SIZE=$(echo "$HTTP_BODY" | jq '.data | length // 0' 2>/dev/null || echo "?")
        info "当前亲和度缓存条目数: $CACHE_SIZE"
    elif [ "$HTTP_STATUS" = "404" ]; then
        fail "#16-A 亲和度缓存接口不存在 (HTTP 404)"
    else
        warn "#16-A HTTP $HTTP_STATUS"
    fi

    # 检查清空缓存接口
    http_get "/api/option/channel_affinity_cache" "$ADMIN_TOKEN"
    if [ "$HTTP_STATUS" != "404" ]; then
        pass "#16-B 亲和度缓存清空接口存在 (HTTP $HTTP_STATUS)"
    else
        fail "#16-B 亲和度缓存清空接口不存在 (HTTP 404)"
    fi
fi

if [ -z "$RELAY_TOKEN" ]; then
    manual "#16 亲和度行为验证" \
        "用同一 sk-xxx Token 连续发送 3 次请求 → 查询 /api/log/self 确认 channel_id 相同"
else
    info "亲和度缓存行为验证（需要至少 2 个同模型渠道才有效果）"
    info "发送第 1 次请求..."
    RESP1=$(curl -s -w '\n__STATUS__%{http_code}' \
        -X POST -H "Authorization: Bearer $RELAY_TOKEN" -H "Content-Type: application/json" \
        -d "{\"model\": \"$RELAY_MODEL\", \"messages\": [{\"role\": \"user\", \"content\": \"1\"}], \"max_tokens\": 3}" \
        "$BASE_URL/v1/chat/completions" 2>&1)
    S1=$(echo "$RESP1" | tail -n1 | sed 's/__STATUS__//')

    sleep 0.5

    info "发送第 2 次请求..."
    RESP2=$(curl -s -w '\n__STATUS__%{http_code}' \
        -X POST -H "Authorization: Bearer $RELAY_TOKEN" -H "Content-Type: application/json" \
        -d "{\"model\": \"$RELAY_MODEL\", \"messages\": [{\"role\": \"user\", \"content\": \"2\"}], \"max_tokens\": 3}" \
        "$BASE_URL/v1/chat/completions" 2>&1)
    S2=$(echo "$RESP2" | tail -n1 | sed 's/__STATUS__//')

    if [ "$S1" = "200" ] && [ "$S2" = "200" ]; then
        pass "#16-C 两次请求均成功"
        info "请在 /api/log/self 查询最近 2 条日志，确认 channel_id 是否相同"

        if [ -n "$ADMIN_TOKEN" ]; then
            sleep 1
            http_get "/api/log/self?page_size=2" "$RELAY_TOKEN"
            CH1=$(echo "$HTTP_BODY" | jq -r '.data.logs[0].channel_id // empty' 2>/dev/null)
            CH2=$(echo "$HTTP_BODY" | jq -r '.data.logs[1].channel_id // empty' 2>/dev/null)
            info "第 1 次日志 channel_id=$CH1 / 第 2 次 channel_id=$CH2"
            if [ -n "$CH1" ] && [ -n "$CH2" ] && [ "$CH1" = "$CH2" ]; then
                pass "#16-C 亲和度生效（两次请求落在同一渠道 channel_id=$CH1）"
            else
                warn "#16-C channel_id 不同或为空（可能只有一个渠道，无法验证亲和度）"
            fi
        fi
    else
        warn "#16-C 请求失败（S1=$S1, S2=$S2），跳过亲和度验证"
    fi
fi

# ── #17 OpenRouter 兼容 API ───────────────────────────────────────────────────
section "#17 OpenRouter 兼容 API"

# 模型列表接口
http_get "/api/openrouter/v1/models"
if [ "$HTTP_STATUS" = "200" ]; then
    pass "#17-A /api/openrouter/v1/models 返回 200"
    assert_jq_exists "$HTTP_BODY" ".data" "#17-A 响应包含 data 数组"

    MODEL_COUNT=$(echo "$HTTP_BODY" | jq '.data | length' 2>/dev/null || echo 0)
    info "模型数量: $MODEL_COUNT"

    if [ "$MODEL_COUNT" -gt 0 ] 2>/dev/null; then
        # 验证 OpenRouter 格式必需字段
        REQUIRED_FIELDS=(".data[0].id" ".data[0].name" ".data[0].context_length" ".data[0].pricing")
        for f in "${REQUIRED_FIELDS[@]}"; do
            assert_jq_exists "$HTTP_BODY" "$f" "#17-B 字段 $f 存在"
        done

        # 验证价格字段格式
        assert_jq_exists "$HTTP_BODY" ".data[0].pricing.prompt" "#17-B pricing.prompt 字段"
        assert_jq_exists "$HTTP_BODY" ".data[0].pricing.completion" "#17-B pricing.completion 字段"
    else
        warn "#17 模型列表为空（可能没有已配置的渠道）"
    fi
else
    fail "#17 /api/openrouter/v1/models 返回 $HTTP_STATUS"
    info "响应: $(echo "$HTTP_BODY" | head -c 200)"
fi

# 状态接口
http_get "/api/openrouter/status"
if [ "$HTTP_STATUS" = "200" ]; then
    pass "#17-C /api/openrouter/status 返回 200"
else
    fail "#17-C /api/openrouter/status 返回 $HTTP_STATUS"
fi

# 单个模型查询
http_get "/api/openrouter/v1/models/deepseek-chat"
if [ "$HTTP_STATUS" = "200" ] || [ "$HTTP_STATUS" = "404" ]; then
    # 200=存在该模型 404=不存在，两者都说明路由工作正常
    pass "#17-D 单模型查询接口路由正常 (HTTP $HTTP_STATUS)"
else
    fail "#17-D 单模型查询接口异常 (HTTP $HTTP_STATUS)"
fi

# 未认证访问配置接口应返回 401
http_get "/api/openrouter/config"
if [ "$HTTP_STATUS" = "401" ] || [ "$HTTP_STATUS" = "403" ]; then
    pass "#17-E 配置接口需要认证 (HTTP $HTTP_STATUS)"
elif [ "$HTTP_STATUS" = "200" ] && [ -n "$ADMIN_TOKEN" ]; then
    pass "#17-E 配置接口在有 token 时可访问"
else
    warn "#17-E 配置接口 HTTP $HTTP_STATUS（期望 401/403）"
fi

# ── #18 WebAuthn/Passkey 路由 ─────────────────────────────────────────────────
section "#18 WebAuthn/Passkey 路由存活检查"

PASSKEY_ROUTES=(
    "POST:/api/user/passkey/login/begin"
    "POST:/api/user/passkey/login/finish"
    "POST:/api/user/passkey/register/begin"
    "POST:/api/user/passkey/register/finish"
    "GET:/api/user/passkey"
    "DELETE:/api/user/passkey/1"
)

for route in "${PASSKEY_ROUTES[@]}"; do
    METHOD="${route%%:*}"
    PATH="${route#*:}"
    if [ "$METHOD" = "GET" ] || [ "$METHOD" = "DELETE" ]; then
        RESP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
            -X "$METHOD" \
            ${RELAY_TOKEN:+-H "Authorization: Bearer $RELAY_TOKEN"} \
            "$BASE_URL$PATH" 2>/dev/null || echo "000")
    else
        RESP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
            -X "$METHOD" \
            -H "Content-Type: application/json" \
            ${RELAY_TOKEN:+-H "Authorization: Bearer $RELAY_TOKEN"} \
            -d '{}' \
            "$BASE_URL$PATH" 2>/dev/null || echo "000")
    fi

    if [ "$RESP_STATUS" != "404" ] && [ "$RESP_STATUS" != "000" ]; then
        pass "#18 路由存在: $METHOD $PATH (HTTP $RESP_STATUS)"
    else
        fail "#18 路由不存在: $METHOD $PATH (HTTP $RESP_STATUS)"
    fi
done

manual "#18 Passkey 完整登录流程" \
    "需要浏览器 + 生物识别设备: 注册 Passkey → 退出 → 用 Passkey 重新登录，确认整个流程正常"

# ── #19 缓存 Token 计费（Claude/Gemini） ─────────────────────────────────────
section "#19 缓存 Token 计费（cache_tokens 字段）"

if [ -z "$RELAY_TOKEN" ]; then
    skip "#19 未设置 RELAY_TOKEN"
    info "手动验证方法:"
    info "  1. 配置 Claude 或 DeepSeek 渠道"
    info "  2. 发送两次相同的长 prompt 请求"
    info "  3. 查询 /api/log/self，确认第二次有 cache_creation_input_tokens 或 cached_tokens"
else
    # 发送两次相同 prompt（测试缓存是否生效）
    CACHE_PROMPT="这是一段用于测试缓存计费的重复内容。在同一个会话中，相同的 prompt 应该触发缓存机制，后端日志应当记录 cache_read_tokens 或类似字段。请回复：已收到。"

    info "发送第 1 次请求（创建缓存）..."
    CACHE_RESP1=$(curl -s -w '\n__STATUS__%{http_code}' \
        -X POST -H "Authorization: Bearer $RELAY_TOKEN" -H "Content-Type: application/json" \
        -d "{\"model\": \"$RELAY_MODEL\", \"messages\": [{\"role\": \"user\", \"content\": \"$CACHE_PROMPT\"}], \"max_tokens\": 10}" \
        "$BASE_URL/v1/chat/completions" 2>&1)
    S1=$(echo "$CACHE_RESP1" | tail -n1 | sed 's/__STATUS__//')
    B1=$(echo "$CACHE_RESP1" | sed '$d')

    if [ "$S1" = "200" ]; then
        pass "#19-A 第 1 次请求成功"
        # 检查响应中 usage 字段
        if echo "$B1" | jq -e '.usage' >/dev/null 2>&1; then
            USAGE1=$(echo "$B1" | jq '.usage')
            info "第 1 次 usage: $USAGE1"
            # 检查是否有缓存相关字段
            if echo "$USAGE1" | jq -e '.prompt_cache_hit_tokens // .cache_read_input_tokens // .cached_tokens' >/dev/null 2>&1; then
                pass "#19-B 响应 usage 包含缓存 token 字段"
            else
                info "#19 响应 usage 字段中无缓存 token（可能渠道不支持，或第 1 次不会命中缓存）"
            fi
        fi
    else
        warn "#19 第 1 次请求失败 (HTTP $S1)，无法测试缓存计费"
    fi

    # 查询日志验证计费字段
    sleep 1
    http_get "/api/log/self?page_size=3" "$RELAY_TOKEN"
    if echo "$HTTP_BODY" | jq -e '.data.logs[0]' >/dev/null 2>&1; then
        LOG0=$(echo "$HTTP_BODY" | jq '.data.logs[0]')
        info "最新日志字段: $(echo "$LOG0" | jq 'keys' 2>/dev/null)"

        # 检查日志中的缓存相关字段
        CACHE_FIELDS=("cache_creation_tokens" "cache_read_tokens" "cached_prompt_tokens")
        FOUND_CACHE_FIELD=false
        for f in "${CACHE_FIELDS[@]}"; do
            if echo "$LOG0" | jq -e ".$f" >/dev/null 2>&1; then
                VAL=$(echo "$LOG0" | jq -r ".$f")
                pass "#19-C 日志包含缓存字段: $f=$VAL"
                FOUND_CACHE_FIELD=true
            fi
        done

        if [ "$FOUND_CACHE_FIELD" = "false" ]; then
            warn "#19-C 日志中未发现缓存 token 字段（可能当前渠道不支持缓存计费，属正常）"
            info "使用 Claude/DeepSeek/阿里通义 等支持缓存的渠道重新测试"
        fi
    fi
fi

# ── #20 性能监控接口（Pyroscope / Pprof） ────────────────────────────────────
section "#20 性能监控接口"

if [ -z "$ADMIN_TOKEN" ]; then
    skip "#20 未设置 ADMIN_TOKEN"
else
    # 检查性能统计接口
    http_get "/api/performance/stats" "$ADMIN_TOKEN"
    if [ "$HTTP_STATUS" = "200" ]; then
        pass "#20-A 性能统计接口 (HTTP 200)"
        # 验证关键字段
        PERF_FIELDS=("goroutines" "heap_alloc" "cpu_percent" "memory_percent")
        for f in "${PERF_FIELDS[@]}"; do
            if echo "$HTTP_BODY" | jq -e ".data.$f // .$f" >/dev/null 2>&1; then
                pass "#20-B 性能字段存在: $f"
            else
                warn "#20-B 性能字段不存在: $f"
            fi
        done
    elif [ "$HTTP_STATUS" = "404" ]; then
        fail "#20-A 性能统计接口不存在 (HTTP 404)"
    else
        warn "#20-A 性能统计接口 HTTP $HTTP_STATUS"
    fi

    # 检查 GC 触发接口
    http_post "/api/performance/gc" '{}' "$ADMIN_TOKEN"
    if [ "$HTTP_STATUS" = "200" ]; then
        pass "#20-C 手动 GC 触发接口 (HTTP 200)"
    elif [ "$HTTP_STATUS" = "404" ]; then
        fail "#20-C GC 触发接口不存在 (HTTP 404)"
    else
        warn "#20-C GC 触发接口 HTTP $HTTP_STATUS"
    fi

    # 检查磁盘缓存清空接口
    http_get "/api/performance/disk_cache_stats" "$ADMIN_TOKEN"
    if [ "$HTTP_STATUS" != "404" ]; then
        pass "#20-D 磁盘缓存统计接口存在 (HTTP $HTTP_STATUS)"
    else
        warn "#20-D 磁盘缓存统计接口不存在（可能路径不同）"
    fi
fi

# Pyroscope 只能通过日志确认
manual "#20 Pyroscope 集成验证" \
    "检查服务启动日志: grep -i 'pyroscope' 应用日志，确认 Pyroscope agent 已成功挂载（仅在配置了 PYROSCOPE_SERVER_ADDRESS 时适用）"

# ── 汇总 ─────────────────────────────────────────────────────────────────────
print_summary "优先级 4 · 通用功能回归"
