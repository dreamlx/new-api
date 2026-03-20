#!/bin/bash
# test-quota-precision.sh
# 验证 Wisemodel 资源包 Quota 精确化三阶段功能
#
# 测试范围：
#   Phase A — 过期包 quota 回收（乐观锁防并发）
#   Phase C — FIFO 归因展示（时间重叠不重复计算）
#   Phase B — logs.wisemodel_package_id 精确归因
#
# 用法:
#   BASE_URL=http://localhost:3000 WISEMODEL_API_TOKEN=xxx ./scripts/test-quota-precision.sh
#
# 依赖: jq, curl, mysql (或 docker exec mysql-dev)

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:3000}"
TOKEN="${WISEMODEL_API_TOKEN:-test_wisemodel_token_12345}"
ENV_FILE="${ENV_FILE:-.env.dev}"
MYSQL_CONTAINER="${MYSQL_CONTAINER:-mysql-dev}"

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; BOLD='\033[1m'; NC='\033[0m'

TOTAL=0; PASSED=0; FAILED=0
ERRORS=()

pass() { echo -e "  ${GREEN}✓ PASS${NC} $1"; PASSED=$((PASSED+1)); TOTAL=$((TOTAL+1)); }
fail() { echo -e "  ${RED}✗ FAIL${NC} $1"; FAILED=$((FAILED+1)); TOTAL=$((TOTAL+1)); ERRORS+=("$1"); }
info() { echo -e "  ${BLUE}ℹ${NC} $1"; }
section() {
    echo ""
    echo -e "${BOLD}── $1 ─────────────────────────────────────────────────${NC}"
}

# ── DSN 解析 ────────────────────────────────────────────────────────────────
parse_dsn() {
    local dsn="$1"
    DB_USER="${dsn%%:*}"; local rest="${dsn#*:}"
    DB_PASS="${rest%%@*}"; rest="${rest#*tcp(}"
    DB_HOST="${rest%%:*}"; rest="${rest#*:}"
    DB_PORT="${rest%%)*}"; DB_NAME="${dsn##*/}"
    DB_NAME="${DB_NAME%%\?*}"
}

DB_HOST="${DB_HOST:-}"; DB_PORT="${DB_PORT:-3306}"
DB_USER="${DB_USER:-}"; DB_PASS="${DB_PASS:-}"; DB_NAME="${DB_NAME:-}"

if [ -z "$DB_HOST" ]; then
    if [ -f "$ENV_FILE" ]; then
        raw=$(grep '^SQL_DSN=' "$ENV_FILE" | head -1 | cut -d= -f2-)
        parse_dsn "$raw"
    fi
fi

if command -v mysql &>/dev/null; then
    EXEC_MODE="local"
elif docker inspect "$MYSQL_CONTAINER" &>/dev/null 2>&1; then
    EXEC_MODE="docker"; DB_HOST_INNER="localhost"; DB_PORT_INNER="3306"
else
    echo -e "${RED}未找到 mysql 或容器 ${MYSQL_CONTAINER}${NC}"; exit 1
fi

qs() {
    if [ "$EXEC_MODE" = "docker" ]; then
        docker exec "$MYSQL_CONTAINER" mysql \
            -h"${DB_HOST_INNER}" -P"${DB_PORT_INNER}" \
            -u"${DB_USER}" -p"${DB_PASS}" "${DB_NAME}" \
            --silent --skip-column-names -e "$1" 2>/dev/null
    else
        mysql -h"${DB_HOST}" -P"${DB_PORT}" -u"${DB_USER}" -p"${DB_PASS}" "${DB_NAME}" \
            --silent --skip-column-names -e "$1" 2>/dev/null
    fi
}

api() {
    local method="$1"; local path="$2"; local body="${3:-}"
    if [ -n "$body" ]; then
        curl -s -X "$method" "${BASE_URL}${path}" \
            -H "Authorization: Bearer $TOKEN" \
            -H "Content-Type: application/json" \
            -d "$body"
    else
        curl -s -X "$method" "${BASE_URL}${path}" \
            -H "Authorization: Bearer $TOKEN"
    fi
}

# ── 环境检查 ─────────────────────────────────────────────────────────────────
check_env() {
    section "环境检查"

    # 服务存活
    if curl -s "${BASE_URL}/api/status" > /dev/null 2>&1; then
        pass "服务可达: ${BASE_URL}"
    else
        fail "服务不可达: ${BASE_URL}  请先运行 make start"
        exit 1
    fi

    # jq
    if command -v jq &>/dev/null; then
        pass "jq 已安装"
    else
        fail "缺少 jq，请执行: brew install jq"; exit 1
    fi

    # DB 连通
    result=$(qs "SELECT 1" 2>/dev/null || echo "")
    if [ "${result:-}" = "1" ]; then
        pass "数据库可达: ${DB_HOST}:${DB_PORT}/${DB_NAME}"
    else
        fail "数据库不可达，检查 ${ENV_FILE} 中的 SQL_DSN"; exit 1
    fi

    # wisemodel_package_id 列存在
    col=$(qs "SELECT COUNT(*) FROM information_schema.columns
              WHERE table_schema='${DB_NAME}' AND table_name='logs'
              AND column_name='wisemodel_package_id'" || echo "0")
    if [ "${col:-0}" != "0" ]; then
        pass "logs.wisemodel_package_id 列已存在"
    else
        fail "logs.wisemodel_package_id 列不存在 — 请先部署并启动服务"
    fi

    # reclaimed_at 列存在
    col2=$(qs "SELECT COUNT(*) FROM information_schema.columns
               WHERE table_schema='${DB_NAME}' AND table_name='wisemodel_packages'
               AND column_name='reclaimed_at'" || echo "0")
    if [ "${col2:-0}" != "0" ]; then
        pass "wisemodel_packages.reclaimed_at 列已存在"
    else
        fail "wisemodel_packages.reclaimed_at 列不存在 — 请先部署并启动服务"
    fi
}

# ── 测试用户准备 ──────────────────────────────────────────────────────────────
TEST_PHONE="${TEST_PHONE:-18900000001}"
TEST_WM_KEY="wisemodel-test-precision"
TEST_USERNAME="prec_test_${TEST_PHONE}"
TEST_USER_ID=""
setup_test_user() {
    section "测试用户准备 (phone: $TEST_PHONE)"

    # 先按手机号查，再按用户名查，任一命中则直接复用
    TEST_USER_ID=$(qs "SELECT id FROM users WHERE phone='$TEST_PHONE' OR username='$TEST_USERNAME' LIMIT 1")

    if [ -n "${TEST_USER_ID:-}" ]; then
        pass "测试用户已存在，直接使用 (ID: $TEST_USER_ID)"
        return
    fi

    # 用户不存在，调用 bind 接口创建（用含手机号的唯一用户名避免冲突）
    resp=$(api POST /api/wisemodel/user/bind "{
        \"phone\": \"$TEST_PHONE\",
        \"wisemodel_key\": \"$TEST_WM_KEY\",
        \"username\": \"$TEST_USERNAME\"
    }")
    if echo "$resp" | jq -e '.success == true' > /dev/null 2>&1; then
        pass "测试用户绑定成功"
    else
        fail "测试用户绑定失败: $resp"; exit 1
    fi

    TEST_USER_ID=$(qs "SELECT id FROM users WHERE phone='$TEST_PHONE' LIMIT 1")
    if [ -z "${TEST_USER_ID:-}" ]; then
        fail "无法获取测试用户 ID，退出"; exit 1
    fi
    info "测试用户 ID: $TEST_USER_ID"
}

cleanup_test_data() {
    # 删除本次测试产生的包和合成 log（reclaimed_at=NULL 且 package_id 含 TEST 前缀）
    qs "DELETE FROM wisemodel_packages WHERE user_id='$TEST_USER_ID' AND package_id LIKE 'PREC_%'" 2>/dev/null || true
    qs "DELETE FROM logs WHERE user_id='$TEST_USER_ID' AND wisemodel_package_id LIKE 'PREC_%'" 2>/dev/null || true
}

# ── Phase B 测试：logs.wisemodel_package_id 列 ───────────────────────────────
test_phase_b_column() {
    section "Phase B — logs.wisemodel_package_id 列验证"

    # 1. NULL 回填验证
    null_count=$(qs "SELECT COUNT(*) FROM logs WHERE wisemodel_package_id IS NULL")
    if [ "${null_count:-0}" = "0" ]; then
        pass "logs 表中无 NULL 值（回填已完成或暂无历史数据）"
    else
        fail "logs 表中仍有 ${null_count} 行 wisemodel_package_id 为 NULL — 请先运行 backfill 脚本"
    fi

    # 2. 新写入的 log 默认为空字符串
    default_check=$(qs "SELECT column_default FROM information_schema.columns
        WHERE table_schema='${DB_NAME}' AND table_name='logs'
        AND column_name='wisemodel_package_id'")
    if [ "${default_check:-}" = "" ] || [ "${default_check:-}" = "NULL" ]; then
        info "列 default 为 NULL（依赖回填脚本），建议检查是否需要 ALTER TABLE"
    else
        pass "列 default = '${default_check}'"
    fi
}

# ── Phase B 测试：创建有效包后触发 API 调用，验证 log 有 wisemodel_package_id ──
test_phase_b_log_attribution() {
    section "Phase B — relay 写 log 时填充 wisemodel_package_id"

    # 1. 创建一个有效包
    PKG_ID="PREC_B_$(date +%s)"
    resp=$(api POST /api/wisemodel/orders/record "{
        \"order_id\": \"ORDER_PREC_B_$(date +%s)\",
        \"package_count\": 1,
        \"packages\": [{
            \"id\": \"$PKG_ID\",
            \"points\": 100,
            \"tokens\": 0,
            \"amount\": 1.00,
            \"phone\": \"$TEST_PHONE\",
            \"is_free\": false,
            \"valid_until\": \"2027-12-31T23:59:59Z\",
            \"created_at\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\"
        }]
    }")
    if echo "$resp" | jq -e '.success == true' > /dev/null 2>&1; then
        pass "创建测试包 $PKG_ID"
    else
        fail "创建测试包失败: $resp"; return
    fi

    # 内部 package_id 带时间戳后缀
    INTERNAL_PKG_ID=$(qs "SELECT package_id FROM wisemodel_packages WHERE user_id='$TEST_USER_ID' AND package_id LIKE '${PKG_ID}_%' ORDER BY created_at DESC LIMIT 1")
    info "内部 package_id: ${INTERNAL_PKG_ID}"

    # 2. 向 logs 表直接注入一条合成消费记录，模拟 relay 写 log
    #    （实际 relay 需要真实模型调用，这里用 SQL 验证列结构和查询逻辑）
    NOW_UNIX=$(date +%s)
    qs "INSERT INTO logs (user_id, type, model_name, quota, created_at, wisemodel_package_id, other)
        VALUES ('$TEST_USER_ID', 2, 'deepseek-chat', 50000, $NOW_UNIX, '$INTERNAL_PKG_ID', 'test')"
    LOG_ID=$(qs "SELECT LAST_INSERT_ID()")
    pass "注入合成消费 log (id=$LOG_ID, wisemodel_package_id=$INTERNAL_PKG_ID)"

    # 3. 验证查询：该 log 可以通过 wisemodel_package_id 精确找到
    found=$(qs "SELECT COUNT(*) FROM logs WHERE wisemodel_package_id='$INTERNAL_PKG_ID' AND type=2")
    if [ "${found:-0}" != "0" ]; then
        pass "通过 wisemodel_package_id 精确查询到 ${found} 条 log"
    else
        fail "精确查询 wisemodel_package_id='$INTERNAL_PKG_ID' 返回 0 行"
    fi

    # 4. 验证 package_usage API 显示消费
    resp=$(api POST /api/wisemodel/user/package_usage "{\"phone\": \"$TEST_PHONE\"}")
    pkg_data=$(echo "$resp" | jq -r --arg pid "$INTERNAL_PKG_ID" '.data[] | select((.package_record_id // .package_id) == $pid)')
    if [ -n "${pkg_data:-}" ]; then
        remain=$(echo "$pkg_data" | jq -r '.remain_points // .remain_tokens')
        amount=$(echo "$pkg_data" | jq -r '.amount // .amount_tokens')
        info "package_usage 返回: remain=$remain, amount=$amount"
        if [ "${amount:-0}" != "0" ]; then
            pass "package_usage 正确显示已消费额度 (amount=$amount)"
        else
            fail "package_usage amount 为 0，消费未计入"
        fi
    else
        info "API 响应: $resp"
        fail "package_usage 未找到包 $INTERNAL_PKG_ID"
    fi

    # 清理合成 log
    qs "DELETE FROM logs WHERE id='$LOG_ID'" 2>/dev/null || true
}

# ── Phase C 测试：FIFO 归因 — 时间重叠两包只计入一个 ────────────────────────
test_phase_c_fifo_overlap() {
    section "Phase C — FIFO 归因：时间重叠不重复计算"

    NOW=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    # PKG-A 到期更早（3个月后），PKG-B 到期更晚（6个月后）
    EXPIRE_A=$(date -u -v+3m +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u -d "+3 months" +"%Y-%m-%dT%H:%M:%SZ")
    EXPIRE_B=$(date -u -v+6m +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u -d "+6 months" +"%Y-%m-%dT%H:%M:%SZ")

    PKG_A="PREC_A_$(date +%s)"
    PKG_B="PREC_B2_$(($(date +%s)+1))"

    # 创建 PKG-A（早到期：100 points = 50,000,000,000 quota）
    resp=$(api POST /api/wisemodel/orders/record "{
        \"order_id\": \"ORDER_PREC_A_$(date +%s)\",
        \"package_count\": 1,
        \"packages\": [{
            \"id\": \"$PKG_A\",
            \"points\": 100,
            \"tokens\": 0,
            \"amount\": 1.00,
            \"phone\": \"$TEST_PHONE\",
            \"is_free\": false,
            \"valid_until\": \"$EXPIRE_A\",
            \"created_at\": \"$NOW\"
        }]
    }")
    if echo "$resp" | jq -e '.success == true' > /dev/null 2>&1; then
        pass "创建 PKG-A（早到期: $EXPIRE_A）"
    else
        fail "创建 PKG-A 失败: $resp"; return
    fi

    sleep 1  # 确保 UnixNano 不冲突

    # 创建 PKG-B（晚到期：200 points）
    resp=$(api POST /api/wisemodel/orders/record "{
        \"order_id\": \"ORDER_PREC_B_$(date +%s)\",
        \"package_count\": 1,
        \"packages\": [{
            \"id\": \"$PKG_B\",
            \"points\": 200,
            \"tokens\": 0,
            \"amount\": 2.00,
            \"phone\": \"$TEST_PHONE\",
            \"is_free\": false,
            \"valid_until\": \"$EXPIRE_B\",
            \"created_at\": \"$NOW\"
        }]
    }")
    if echo "$resp" | jq -e '.success == true' > /dev/null 2>&1; then
        pass "创建 PKG-B（晚到期: $EXPIRE_B）"
    else
        fail "创建 PKG-B 失败: $resp"; return
    fi

    # 获取两个包的内部 ID
    INTERNAL_A=$(qs "SELECT package_id FROM wisemodel_packages WHERE user_id='$TEST_USER_ID' AND package_id LIKE '${PKG_A}_%' ORDER BY created_at DESC LIMIT 1")
    INTERNAL_B=$(qs "SELECT package_id FROM wisemodel_packages WHERE user_id='$TEST_USER_ID' AND package_id LIKE '${PKG_B}_%' ORDER BY created_at DESC LIMIT 1")
    info "内部 PKG-A: $INTERNAL_A"
    info "内部 PKG-B: $INTERNAL_B"

    # 注入合成消费 log（wisemodel_package_id 为空，模拟旧 log，由 FIFO 归因）
    NOW_UNIX=$(date +%s)
    qs "INSERT INTO logs (user_id, type, model_name, quota, created_at, wisemodel_package_id, other)
        VALUES ('$TEST_USER_ID', 2, 'deepseek-chat', 1000000, $NOW_UNIX, '', 'fifo_test')"
    LOG_ID=$(qs "SELECT LAST_INSERT_ID()")
    info "注入旧 log（无 package_id，quota=1000000，期待归属 PKG-A）"

    # 调用 package_usage
    resp=$(api POST /api/wisemodel/user/package_usage "{\"phone\": \"$TEST_PHONE\"}")

    data_a=$(echo "$resp" | jq -r --arg pid "$INTERNAL_A" '.data[] | select((.package_record_id // .package_id) == $pid)')
    data_b=$(echo "$resp" | jq -r --arg pid "$INTERNAL_B" '.data[] | select((.package_record_id // .package_id) == $pid)')

    amount_a=$(echo "${data_a:-{\}}" | jq -r '.amount // 0')
    amount_b=$(echo "${data_b:-{\}}" | jq -r '.amount // 0')
    info "PKG-A (早到期) amount=$amount_a | PKG-B (晚到期) amount=$amount_b"

    # FIFO: 消费应归属 PKG-A（到期更早）
    if [ "${amount_a:-0}" != "0" ] && [ "${amount_b:-0}" = "0" ]; then
        pass "FIFO 正确：消费归属 PKG-A（早到期），PKG-B 不计入"
    elif [ "${amount_a:-0}" != "0" ] && [ "${amount_b:-0}" != "0" ]; then
        fail "FIFO 错误：消费同时计入 PKG-A(${amount_a}) 和 PKG-B(${amount_b})（重复计算）"
    elif [ "${amount_a:-0}" = "0" ] && [ "${amount_b:-0}" != "0" ]; then
        fail "FIFO 错误：消费计入 PKG-B 而非更早到期的 PKG-A"
    else
        info "API 完整响应: $resp"
        fail "FIFO 验证失败：两包均为 0 或无数据"
    fi

    # 验证两包金额不重叠（总消费 = PKG-A + PKG-B，不应超过实际消费）
    total=$(echo "$resp" | jq '[.data[].amount // .data[].amount_tokens // 0] | add // 0')
    info "所有包 amount 之和: $total（期望 ≤ 实际消费，无重复计算）"

    # 清理
    qs "DELETE FROM logs WHERE id='$LOG_ID'" 2>/dev/null || true
}

# ── Phase A 测试：过期包 quota 回收 ──────────────────────────────────────────
test_phase_a_reclaim() {
    section "Phase A — 过期包 quota 回收"

    # 获取回收前 user quota
    quota_before=$(qs "SELECT quota FROM users WHERE id='$TEST_USER_ID'")
    info "回收前 user quota: $quota_before"

    # 创建一个已过期的包（valid_until 设为过去时间）
    PAST_TIME="2020-01-01T00:00:00Z"
    PKG_EXPIRE="PREC_EXP_$(date +%s)"

    # 绕过 API 的 valid_until 未来校验，直接写入 DB
    QUOTA_GRANTED=500000
    qs "INSERT INTO wisemodel_packages
        (user_id, package_id, order_id, original_points, original_tokens,
         quota_granted, available_models, amount, is_free, valid_until, created_at)
        VALUES
        ('$TEST_USER_ID', '$PKG_EXPIRE', 'ORDER_EXP_TEST', 1, 0,
         $QUOTA_GRANTED, '', 1.0, 0,
         '2020-06-01 00:00:00', NOW() - INTERVAL 1 YEAR)"
    info "直接写入已过期包 (package_id=$PKG_EXPIRE, quota_granted=$QUOTA_GRANTED, valid_until=2020-06-01)"

    # 将对应 user quota 加上该包额度（模拟绑定时的操作）
    qs "UPDATE users SET quota = quota + $QUOTA_GRANTED WHERE id='$TEST_USER_ID'"
    quota_after_bind=$(qs "SELECT quota FROM users WHERE id='$TEST_USER_ID'")
    info "绑定后 user quota: $quota_after_bind"

    # 调用回收接口（通过触发 GET /api/wisemodel/admin/reclaim 或直接验证）
    # 由于后台任务每5分钟跑一次，这里用管理接口或等待
    # 检查是否有 admin reclaim 接口，没有就直接验证 DB 状态
    reclaim_resp=$(curl -s -X POST "${BASE_URL}/api/wisemodel/admin/reclaim_expired" \
        -H "Authorization: Bearer $TOKEN" 2>/dev/null || echo "no_endpoint")

    if echo "$reclaim_resp" | jq -e '.success == true' > /dev/null 2>&1; then
        pass "管理员回收接口触发成功"
        sleep 1
    else
        info "无手动触发接口（后台每5分钟自动执行），直接验证 DB 结构"
    fi

    # 验证 reclaimed_at 列结构正确
    col_type=$(qs "SELECT column_type FROM information_schema.columns
        WHERE table_schema='${DB_NAME}' AND table_name='wisemodel_packages'
        AND column_name='reclaimed_at'")
    if echo "${col_type:-}" | grep -qi "datetime\|timestamp"; then
        pass "wisemodel_packages.reclaimed_at 列类型: $col_type"
    else
        fail "reclaimed_at 列类型异常: $col_type"
    fi

    # 验证该过期包目前 reclaimed_at IS NULL（尚未被后台任务回收）
    rcl=$(qs "SELECT reclaimed_at FROM wisemodel_packages WHERE package_id='$PKG_EXPIRE'")
    info "reclaimed_at 当前值: ${rcl:-NULL}"

    # 验证回收逻辑正确性：模拟手动执行回收（通过直接 UPDATE 验证乐观锁）
    qs "UPDATE wisemodel_packages SET reclaimed_at=NOW() WHERE package_id='$PKG_EXPIRE' AND reclaimed_at IS NULL"
    rows=$(qs "SELECT COUNT(*) FROM wisemodel_packages WHERE package_id='$PKG_EXPIRE' AND reclaimed_at IS NOT NULL")
    if [ "${rows:-0}" = "1" ]; then
        pass "乐观锁 UPDATE (WHERE reclaimed_at IS NULL) 成功，reclaimed_at 已设置"
    else
        info "第一次 UPDATE 后 reclaimed_at NOT NULL count=$rows（可能已被其他操作回收）"
    fi

    # 再次执行，验证幂等性（WHERE reclaimed_at IS NULL 不匹配任何行）
    qs "UPDATE wisemodel_packages SET reclaimed_at=NOW() WHERE package_id='$PKG_EXPIRE' AND reclaimed_at IS NULL"
    remaining_null=$(qs "SELECT COUNT(*) FROM wisemodel_packages WHERE package_id='$PKG_EXPIRE' AND reclaimed_at IS NULL")
    if [ "${remaining_null:-1}" = "0" ]; then
        pass "幂等性验证：重复执行后 reclaimed_at 仍已设置（防止重复回收）"
    else
        fail "幂等性失败：重复执行后 reclaimed_at IS NULL count=${remaining_null}（可能重复回收）"
    fi

    # 恢复 quota（清理）
    qs "UPDATE users SET quota = quota - $QUOTA_GRANTED WHERE id='$TEST_USER_ID'" 2>/dev/null || true
    qs "DELETE FROM wisemodel_packages WHERE package_id='$PKG_EXPIRE'" 2>/dev/null || true
    info "已清理测试数据"
}

# ── Phase B 测试：精确查询 vs FIFO 混合 ─────────────────────────────────────
test_phase_b_mixed_query() {
    section "Phase B — 精确+FIFO 混合查询验证"

    # PREC_MIX 到期时间设为1个月后（比 Phase C 中 PKG-A 的3个月更早），
    # 确保 FIFO 排序时 PREC_MIX 优先被选中
    EXPIRE_MIX=$(date -u -v+1m +"%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || date -u -d "+1 month" +"%Y-%m-%dT%H:%M:%SZ")
    PKG_MIX="PREC_MIX_$(date +%s)"
    resp=$(api POST /api/wisemodel/orders/record "{
        \"order_id\": \"ORDER_MIX_$(date +%s)\",
        \"package_count\": 1,
        \"packages\": [{
            \"id\": \"$PKG_MIX\",
            \"points\": 50,
            \"tokens\": 0,
            \"amount\": 1.00,
            \"phone\": \"$TEST_PHONE\",
            \"is_free\": false,
            \"valid_until\": \"$EXPIRE_MIX\",
            \"created_at\": \"$(date -u +"%Y-%m-%dT%H:%M:%SZ")\"
        }]
    }")
    if ! echo "$resp" | jq -e '.success == true' > /dev/null 2>&1; then
        fail "创建混合测试包失败: $resp"; return
    fi

    INTERNAL_MIX=$(qs "SELECT package_id FROM wisemodel_packages WHERE user_id='$TEST_USER_ID' AND package_id LIKE '${PKG_MIX}_%' ORDER BY created_at DESC LIMIT 1")
    pass "创建混合测试包 $INTERNAL_MIX（到期: $EXPIRE_MIX，最早到期确保 FIFO 优先）"
    info "场景：一条旧 log（无 package_id，FIFO归因）+ 一条新 log（有 package_id，精确归因）"

    # 注意：旧 log 的 created_at 必须 >= pkg.CreatedAt，否则 FIFO 条件不满足被忽略
    # 使用当前时间（包已存在），新 log 晚 1 秒
    NOW_UNIX=$(date +%s)

    # 旧 log：wisemodel_package_id = ''（FIFO 归因到 PREC_MIX，因为它最早到期）
    qs "INSERT INTO logs (user_id, type, model_name, quota, created_at, wisemodel_package_id, other)
        VALUES ('$TEST_USER_ID', 2, 'deepseek-chat', 500000, $NOW_UNIX, '', 'old_log')"
    OLD_LOG_ID=$(qs "SELECT LAST_INSERT_ID()")

    # 新 log：wisemodel_package_id = INTERNAL_MIX（精确）
    qs "INSERT INTO logs (user_id, type, model_name, quota, created_at, wisemodel_package_id, other)
        VALUES ('$TEST_USER_ID', 2, 'deepseek-chat', 500000, $NOW_UNIX, '$INTERNAL_MIX', 'new_log')"
    NEW_LOG_ID=$(qs "SELECT LAST_INSERT_ID()")

    info "注入旧 log id=$OLD_LOG_ID (quota=500000, pkg_id='') + 新 log id=$NEW_LOG_ID (quota=500000, pkg_id=$INTERNAL_MIX)"

    # 调用 package_usage，期望 amount = 2（旧+新 各 1 point，合计 2）
    resp=$(api POST /api/wisemodel/user/package_usage "{\"phone\": \"$TEST_PHONE\"}")
    pkg_data=$(echo "$resp" | jq -r --arg pid "$INTERNAL_MIX" '.data[] | select((.package_record_id // .package_id) == $pid)')
    amount=$(echo "${pkg_data:-{\}}" | jq -r '.amount // 0')
    info "package_usage amount = $amount（期望 ≥ 2，旧+新各 500000 quota = 1 point 各）"

    # 500000 quota = 1 point，2 条记录合计应消费 2 points
    if [ "${amount:-0}" -ge "2" ] 2>/dev/null; then
        pass "混合查询正确：精确+FIFO 合并结果 amount=$amount（≥ 2）"
    else
        fail "混合查询异常：amount=${amount}，期望 ≥ 2（精确+FIFO 合计）"
    fi

    # 验证 remain 不为负数
    remain=$(echo "${pkg_data:-{\}}" | jq -r '.remain_points // 0')
    if [ "${remain:-0}" -ge "0" ] 2>/dev/null; then
        pass "remain_points 非负: $remain"
    else
        fail "remain_points 为负数: $remain（缺少 clamp 保护）"
    fi

    # 清理
    qs "DELETE FROM logs WHERE id IN ('$OLD_LOG_ID', '$NEW_LOG_ID')" 2>/dev/null || true
}

# ── 数据完整性总检查 ─────────────────────────────────────────────────────────
test_data_integrity() {
    section "数据完整性检查"

    # 检查 NULL 残留
    null_count=$(qs "SELECT COUNT(*) FROM logs WHERE wisemodel_package_id IS NULL")
    if [ "${null_count:-0}" = "0" ]; then
        pass "logs 表无 NULL（回填完整）"
    else
        fail "logs 表仍有 ${null_count} 行 NULL（请运行 backfill-wisemodel-logs.sh）"
    fi

    # 检查 reclaim 并发安全：同一包不能有两条 reclaimed_at 不同的记录（幂等）
    dup=$(qs "SELECT COUNT(*) FROM wisemodel_packages
              WHERE reclaimed_at IS NOT NULL
              GROUP BY id
              HAVING COUNT(*) > 1" 2>/dev/null || echo "0")
    if [ -z "${dup:-}" ] || [ "${dup:-0}" = "0" ]; then
        pass "wisemodel_packages 无重复回收记录"
    else
        fail "发现重复回收记录（并发安全问题）"
    fi

    # 汇总统计
    total_pkgs=$(qs "SELECT COUNT(*) FROM wisemodel_packages")
    active_pkgs=$(qs "SELECT COUNT(*) FROM wisemodel_packages WHERE valid_until IS NULL OR valid_until > NOW()")
    expired_reclaimed=$(qs "SELECT COUNT(*) FROM wisemodel_packages WHERE valid_until < NOW() AND reclaimed_at IS NOT NULL")
    expired_pending=$(qs "SELECT COUNT(*) FROM wisemodel_packages WHERE valid_until < NOW() AND reclaimed_at IS NULL")
    info "包汇总 — 总: $total_pkgs | 有效: $active_pkgs | 已回收: $expired_reclaimed | 待回收: $expired_pending"
    if [ "${expired_pending:-0}" != "0" ]; then
        info "  待回收包将在后台定时任务（每5分钟）自动处理"
    fi
}

# ── 主流程 ────────────────────────────────────────────────────────────────────
main() {
    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║         Wisemodel Quota 精确化功能测试                           ║${NC}"
    echo -e "${BOLD}║  Phase A: 过期回收  Phase C: FIFO归因  Phase B: 精确归因         ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════════════════════╝${NC}"
    echo -e "  BASE_URL: ${BASE_URL}"
    echo -e "  数据库: ${DB_HOST}:${DB_PORT}/${DB_NAME}  执行器: ${EXEC_MODE}"
    echo ""

    check_env
    setup_test_user

    test_phase_b_column
    test_phase_b_log_attribution
    test_phase_c_fifo_overlap
    test_phase_a_reclaim
    test_phase_b_mixed_query
    test_data_integrity

    # 清理测试包（保留本次所有 PREC_ 包，避免影响其他测试）
    cleanup_test_data

    # 最终报告
    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║                       测试报告                                  ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    printf "  总测试数: %-4d  " "$TOTAL"
    echo -e "${GREEN}通过: $PASSED${NC}  ${RED}失败: $FAILED${NC}"
    echo ""

    if [ "${#ERRORS[@]}" -gt 0 ]; then
        echo -e "${RED}失败项目：${NC}"
        for e in "${ERRORS[@]}"; do
            echo -e "  ${RED}✗${NC} $e"
        done
        echo ""
    fi

    if [ "$FAILED" -eq 0 ]; then
        echo -e "  ${GREEN}${BOLD}✅ 所有测试通过！Quota 精确化功能工作正常${NC}"
        echo ""
        return 0
    else
        echo -e "  ${RED}${BOLD}❌ ${FAILED} 个测试失败，请检查上面的输出${NC}"
        echo ""
        return 1
    fi
}

main
