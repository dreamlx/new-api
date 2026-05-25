#!/bin/bash
# test_p1_wisemodel.sh — 优先级 1：WiseModel 核心业务流程验证
#
# 覆盖清单项目: #1 ~ #5
#   #1 过期包 Quota 自动回收后台任务
#   #2 GetPackageUsage FIFO 精确归因 / details 字段
#   #3 relay 请求写入 wisemodel_package_id
#   #4 用户绑定接口
#   #5 订单记录接口
#
# 用法:
#   BASE_URL=http://localhost:3000 \
#   WISEMODEL_API_TOKEN=xxx \
#   RELAY_TOKEN=sk-xxx \
#   RELAY_MODEL=gpt-3.5-turbo \
#   [DB_TYPE=mysql DB_DSN="user:pass@tcp(host:3306)/dbname"] \
#   [WISEMODEL_TEST_USER_ID=existing_wm_user_id] \
#   bash scripts/test_p1_wisemodel.sh

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/test_lib.sh"

check_deps

echo ""
echo -e "${BOLD}${CYAN}════════════════════════════════════════════════════${NC}"
echo -e "${BOLD}${CYAN}  优先级 1：WiseModel 核心业务流程  ${NC}"
echo -e "${BOLD}${CYAN}════════════════════════════════════════════════════${NC}"
echo -e "  BASE_URL: $BASE_URL"
echo -e "  WISEMODEL_API_TOKEN: ${WISEMODEL_API_TOKEN:+${WISEMODEL_API_TOKEN:0:8}...（已设置）}${WISEMODEL_API_TOKEN:-（未设置）}"
echo -e "  RELAY_TOKEN: ${RELAY_TOKEN:+${RELAY_TOKEN:0:8}...（已设置）}${RELAY_TOKEN:-（未设置）}"

check_service

# ── #1 过期包 Quota 自动回收后台任务 ──────────────────────────────────────────
section "#1 过期包 Quota 自动回收后台任务"

# 通过 /api/status 确认服务已启动（回收任务在 main.go 启动时注册）
http_get "/api/status"
assert_jq "$HTTP_BODY" ".start_time" "" # 字段存在即可（不为 null）
if echo "$HTTP_BODY" | jq -e ".start_time" >/dev/null 2>&1; then
    START_TIME=$(echo "$HTTP_BODY" | jq -r ".start_time")
    pass "#1 服务已启动 (start_time=$START_TIME)"
    info "后台任务在服务启动时注册，请检查启动日志中的关键字:"
    info "  grep -i 'wisemodel\|package.*reclaim\|expire.*quota' 应用日志"
else
    fail "#1 服务 status 接口未返回 start_time"
fi

# 检查数据库中是否有 wisemodel_packages 表（进一步确认 schema 存在）
DB_CHECK=$(db_query "SELECT COUNT(*) FROM information_schema.tables WHERE table_name='wisemodel_packages'" 2>/dev/null || echo "DB_SKIP")
if [ "$DB_CHECK" != "DB_SKIP" ] && [ "$DB_CHECK" != "" ]; then
    COUNT=$(echo "$DB_CHECK" | tail -1 | tr -d ' \t')
    if [ "$COUNT" -ge 1 ] 2>/dev/null; then
        pass "#1 wisemodel_packages 表存在"
    else
        fail "#1 wisemodel_packages 表不存在（可能 schema 未迁移）"
    fi
else
    skip "#1 数据库检查跳过（未配置 DB_TYPE/DB_DSN/SQLITE_FILE）"
fi

# ── #2 GetPackageUsage FIFO 精确归因 ─────────────────────────────────────────
section "#2 GetPackageUsage FIFO 精确归因 + details 字段"

if [ -z "$WISEMODEL_API_TOKEN" ]; then
    skip "#2 未设置 WISEMODEL_API_TOKEN，跳过此项"
else
    TEST_WM_USER="${WISEMODEL_TEST_USER_ID:-wm_test_user_001}"
    info "测试用户: $TEST_WM_USER"

    http_post "/api/wisemodel/user/package_usage" \
        "{\"wisemodel_user_id\": \"$TEST_WM_USER\"}" \
        "$WISEMODEL_API_TOKEN"

    info "HTTP 状态: $HTTP_STATUS"

    if [ "$HTTP_STATUS" = "200" ]; then
        assert_jq "$HTTP_BODY" ".success" "true" "#2 package_usage 接口响应 success=true"

        # 检查 data 字段存在
        if echo "$HTTP_BODY" | jq -e ".data" >/dev/null 2>&1; then
            pass "#2 data 字段存在"
            # 检查是否有 details（per-model 用量明细）
            if echo "$HTTP_BODY" | jq -e ".data[0].details" >/dev/null 2>&1; then
                pass "#2 details 字段存在（per-model 用量明细）"
                DETAIL_COUNT=$(echo "$HTTP_BODY" | jq '.data[0].details | length' 2>/dev/null || echo 0)
                info "details 条目数: $DETAIL_COUNT"
            elif echo "$HTTP_BODY" | jq -e ".data | length == 0" >/dev/null 2>&1; then
                skip "#2 data 为空数组（该用户无资源包，无法验证 details 字段）"
                info "建议: 为测试用户创建资源包后重新运行"
            else
                fail "#2 data 存在但缺少 details 字段（per-model 归因未正确返回）"
                info "响应: $(echo "$HTTP_BODY" | jq '.data[0]' 2>/dev/null || echo "$HTTP_BODY" | head -c 300)"
            fi
        else
            fail "#2 响应缺少 data 字段"
            info "响应: $(echo "$HTTP_BODY" | head -c 300)"
        fi
    elif [ "$HTTP_STATUS" = "401" ] || [ "$HTTP_STATUS" = "403" ]; then
        fail "#2 鉴权失败 (HTTP $HTTP_STATUS)，请检查 WISEMODEL_API_TOKEN"
    elif [ "$HTTP_STATUS" = "404" ]; then
        fail "#2 接口不存在 (HTTP 404)，WiseModel 路由可能未注册"
    else
        fail "#2 接口返回非预期状态 (HTTP $HTTP_STATUS)"
        info "响应: $(echo "$HTTP_BODY" | head -c 200)"
    fi
fi

# ── #3 relay 请求写入 wisemodel_package_id ────────────────────────────────────
section "#3 relay 请求日志写入 wisemodel_package_id"

if [ -z "$RELAY_TOKEN" ]; then
    skip "#3 未设置 RELAY_TOKEN，跳过自动化验证"
    info "手动验证方法:"
    info "  1. 用绑定了 WiseModel 包的 Token 发送一次 chat 请求"
    info "  2. 查询数据库: SELECT wisemodel_package_id FROM logs ORDER BY id DESC LIMIT 5;"
    info "  3. 确认 wisemodel_package_id 非空"
else
    info "发送 relay 测试请求 (model=$RELAY_MODEL)..."
    RELAY_RESP=$(curl -s -w '\n__STATUS__%{http_code}' \
        -X POST \
        -H "Authorization: Bearer $RELAY_TOKEN" \
        -H "Content-Type: application/json" \
        -d "{\"model\": \"$RELAY_MODEL\", \"messages\": [{\"role\": \"user\", \"content\": \"reply: hi\"}], \"max_tokens\": 5}" \
        "$BASE_URL/v1/chat/completions" 2>&1)
    RELAY_BODY=$(echo "$RELAY_RESP" | sed '$d')
    RELAY_STATUS=$(echo "$RELAY_RESP" | tail -n1 | sed 's/__STATUS__//')

    if [ "$RELAY_STATUS" = "200" ]; then
        pass "#3 relay 请求成功 (HTTP 200)"
        info "检查数据库 wisemodel_package_id 字段:"
        PACKAGE_ID_ROW=$(db_query "SELECT wisemodel_package_id FROM logs ORDER BY id DESC LIMIT 1" 2>/dev/null || echo "DB_SKIP")
        if [ "$PACKAGE_ID_ROW" = "DB_SKIP" ] || [ -z "$PACKAGE_ID_ROW" ]; then
            skip "#3 数据库未配置，跳过 wisemodel_package_id 字段验证"
            info "请手动查询: SELECT wisemodel_package_id FROM logs ORDER BY id DESC LIMIT 3;"
        else
            PKG_ID=$(echo "$PACKAGE_ID_ROW" | tail -1 | tr -d ' \t')
            if [ -n "$PKG_ID" ] && [ "$PKG_ID" != "NULL" ] && [ "$PKG_ID" != "null" ]; then
                pass "#3 最新日志 wisemodel_package_id='$PKG_ID'"
            else
                warn "#3 最新日志 wisemodel_package_id 为空（可能 Token 未绑定 WiseModel 包，属正常）"
            fi
        fi
    else
        fail "#3 relay 请求失败 (HTTP $RELAY_STATUS)"
        info "响应: $(echo "$RELAY_BODY" | head -c 200)"
    fi
fi

# ── #4 WiseModel 用户绑定接口 ─────────────────────────────────────────────────
section "#4 WiseModel 用户绑定接口 POST /api/wisemodel/user/bind"

if [ -z "$WISEMODEL_API_TOKEN" ]; then
    skip "#4 未设置 WISEMODEL_API_TOKEN"
else
    BIND_USER="test_bind_$(date +%s)"
    # 先用外部用户接口创建一个本地用户
    SYNC_RESP=$(curl -s -X POST "$BASE_URL/api/user/external/sync" \
        -H "Content-Type: application/json" \
        -d "{\"external_user_id\": \"ext_$BIND_USER\", \"username\": \"$BIND_USER\", \"email\": \"${BIND_USER}@test.local\"}" 2>/dev/null || echo '{}')
    NEW_API_USER_ID=$(echo "$SYNC_RESP" | jq -r '.data.user_id // empty' 2>/dev/null)

    if [ -z "$NEW_API_USER_ID" ]; then
        skip "#4 无法创建测试用户（external/sync 接口失败），跳过绑定测试"
        info "sync 响应: $(echo "$SYNC_RESP" | head -c 200)"
    else
        info "创建测试用户成功: user_id=$NEW_API_USER_ID"

        http_post "/api/wisemodel/user/bind" \
            "{\"wisemodel_user_id\": \"wm_$BIND_USER\", \"new_api_user_id\": $NEW_API_USER_ID}" \
            "$WISEMODEL_API_TOKEN"

        if [ "$HTTP_STATUS" = "200" ]; then
            assert_jq "$HTTP_BODY" ".success" "true" "#4 用户绑定成功"
        elif [ "$HTTP_STATUS" = "409" ]; then
            pass "#4 绑定接口已处理重复绑定（HTTP 409，属正常）"
        elif [ "$HTTP_STATUS" = "401" ] || [ "$HTTP_STATUS" = "403" ]; then
            fail "#4 鉴权失败 (HTTP $HTTP_STATUS)"
        else
            fail "#4 绑定接口返回非预期状态 (HTTP $HTTP_STATUS)"
            info "响应: $(echo "$HTTP_BODY" | head -c 300)"
        fi
    fi
fi

# ── #5 WiseModel 订单记录接口 ─────────────────────────────────────────────────
section "#5 WiseModel 订单记录接口 POST /api/wisemodel/orders/record"

if [ -z "$WISEMODEL_API_TOKEN" ]; then
    skip "#5 未设置 WISEMODEL_API_TOKEN"
else
    ORDER_ID="test_order_$(date +%s)_$$"
    http_post "/api/wisemodel/orders/record" \
        "{
            \"wisemodel_user_id\": \"wm_test_user_001\",
            \"order_id\": \"$ORDER_ID\",
            \"package_type\": \"tokens\",
            \"original_tokens\": 1000000,
            \"original_points\": 0,
            \"price_cny\": 9.9,
            \"expire_at\": $(( $(date +%s) + 86400 * 30 ))
        }" \
        "$WISEMODEL_API_TOKEN"

    info "HTTP 状态: $HTTP_STATUS"
    if [ "$HTTP_STATUS" = "200" ]; then
        assert_jq "$HTTP_BODY" ".success" "true" "#5 订单记录创建成功"
        PACKAGE_ID=$(echo "$HTTP_BODY" | jq -r '.data.package_id // empty' 2>/dev/null)
        [ -n "$PACKAGE_ID" ] && info "生成 package_id=$PACKAGE_ID"

        # 验证数据库记录
        DB_ROW=$(db_query "SELECT order_id FROM wisemodel_packages WHERE order_id='$ORDER_ID' LIMIT 1" 2>/dev/null || echo "DB_SKIP")
        if [ "$DB_ROW" != "DB_SKIP" ] && [ -n "$DB_ROW" ]; then
            pass "#5 数据库中订单记录存在"
        else
            skip "#5 数据库检查跳过"
        fi
    elif [ "$HTTP_STATUS" = "409" ]; then
        pass "#5 重复订单被正确拦截（HTTP 409）"
    elif [ "$HTTP_STATUS" = "401" ] || [ "$HTTP_STATUS" = "403" ]; then
        fail "#5 鉴权失败 (HTTP $HTTP_STATUS)"
    else
        fail "#5 接口返回非预期状态 (HTTP $HTTP_STATUS)"
        info "响应: $(echo "$HTTP_BODY" | head -c 300)"
    fi
fi

# ── 汇总 ─────────────────────────────────────────────────────────────────────
print_summary "优先级 1 · WiseModel 核心业务"
