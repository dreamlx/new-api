#!/bin/bash

# Wisemodel 资源包补录脚本
#
# 找出有 wisemodel-token 但无有效资源包的用户，
# 逐个调用 /api/wisemodel/orders/record 补录资源包。
#
# 用法：
#   ./scripts/backfill-wisemodel-packages.sh            # Dry-Run，只列出受影响用户
#   ./scripts/backfill-wisemodel-packages.sh --execute  # 真正调用 API 补录
#
# 环境变量：
#   ENV_FILE=.env.dev               数据库 DSN 文件（默认 .env.dev）
#   MYSQL_CONTAINER=mysql-dev       Docker 容器名（默认 mysql-dev）
#   API_BASE=https://...            API 地址（默认读 .env.dev 里的 API_BASE，否则用下面默认值）
#   WISEMODEL_API_TOKEN=xxx         Bearer Token（默认从 .env.dev 读 WISEMODEL_API_TOKEN）
#
# 补录套餐参数（与 Wisemodel 确认的固定值）：
#   PACKAGE_ID=package16
#   POINTS=1000000000
#   AMOUNT=672
#   VALID_UNTIL=2178-04-08T06:25:14+08:00

set -euo pipefail

EXECUTE="${1:-}"
ENV_FILE="${ENV_FILE:-.env.dev}"
MYSQL_CONTAINER="${MYSQL_CONTAINER:-mysql-dev}"

# ──────────────────────────────────────────────
# 颜色
# ──────────────────────────────────────────────
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

ok()     { echo -e "  ${GREEN}[OK]${NC}    $1"; }
fail()   { echo -e "  ${RED}[!!]${NC}    $1"; FAILED=$((FAILED+1)); }
warn()   { echo -e "  ${YELLOW}[~~]${NC}    $1"; }
info()   { echo -e "  ${BLUE}--${NC}      $1"; }
action() { echo -e "  ${CYAN}[API]${NC}   $1"; }
dryrun() { echo -e "  ${YELLOW}[DRY]${NC}   $1"; }

FAILED=0

# ──────────────────────────────────────────────
# DSN 解析
# ──────────────────────────────────────────────
parse_dsn() {
    local dsn="$1"
    local userpass="${dsn%%@*}"
    DB_USER="${userpass%%:*}"; DB_PASS="${userpass#*:}"
    local hostpart="${dsn#*tcp(}"; hostpart="${hostpart%%)*}"
    DB_HOST="${hostpart%%:*}"; DB_PORT="${hostpart##*:}"
    DB_NAME="${dsn##*/}"
}

DB_HOST="${DB_HOST:-}"; DB_PORT="${DB_PORT:-3306}"
DB_USER="${DB_USER:-}"; DB_PASS="${DB_PASS:-}"; DB_NAME="${DB_NAME:-}"

load_env() {
    [ ! -f "$ENV_FILE" ] && { echo "未找到 $ENV_FILE"; exit 1; }

    if [ -z "$DB_HOST" ]; then
        local raw
        raw=$(grep '^SQL_DSN=' "$ENV_FILE" | head -1 | cut -d= -f2-)
        [ -z "$raw" ] && { echo "未找到 SQL_DSN"; exit 1; }
        parse_dsn "$raw"
    fi

    # 读取 API_BASE 和 WISEMODEL_API_TOKEN（若未通过环境变量传入）
    API_BASE="${API_BASE:-$(grep '^API_BASE=' "$ENV_FILE" 2>/dev/null | head -1 | cut -d= -f2- || true)}"
    API_BASE="${API_BASE:-https://open.ospreyai.cn}"

    WISEMODEL_API_TOKEN="${WISEMODEL_API_TOKEN:-$(grep '^WISEMODEL_API_TOKEN=' "$ENV_FILE" 2>/dev/null | head -1 | cut -d= -f2- || true)}"
    if [ -z "$WISEMODEL_API_TOKEN" ]; then
        echo "未找到 WISEMODEL_API_TOKEN（在 $ENV_FILE 或环境变量中设置）"
        exit 1
    fi
}

# ──────────────────────────────────────────────
# MySQL 执行器
# ──────────────────────────────────────────────
setup_executor() {
    if command -v mysql &>/dev/null; then
        EXEC_MODE="local"
    elif docker inspect "$MYSQL_CONTAINER" &>/dev/null 2>&1; then
        EXEC_MODE="docker"; DB_HOST_INNER="localhost"; DB_PORT_INNER="3306"
    else
        echo "未找到 mysql 命令，也未找到容器 ${MYSQL_CONTAINER}"; exit 1
    fi
}

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

# ──────────────────────────────────────────────
# 生成唯一 order_id（格式与示例一致：YYYYMMDDHHmmss + 6位随机）
# ──────────────────────────────────────────────
gen_order_id() {
    local ts
    ts=$(date '+%Y%m%d%H%M%S')
    local rand
    rand=$(( RANDOM * RANDOM % 1000000 ))
    printf "%s%06d" "$ts" "$rand"
}

# ──────────────────────────────────────────────
# 补录参数（与 Wisemodel 确认的固定值）
# ──────────────────────────────────────────────
PKG_ID="${PACKAGE_ID:-package16}"
PKG_POINTS="${POINTS:-1000000000}"
PKG_AMOUNT="${AMOUNT:-672}"
PKG_VALID_UNTIL="${VALID_UNTIL:-2178-04-08T06:25:14+08:00}"

# ──────────────────────────────────────────────
# 查询需要补录的用户
# ──────────────────────────────────────────────
get_phones_without_packages() {
    # 有效 wisemodel-token 但无任何有效资源包的用户手机号
    qs "SELECT DISTINCT u.phone
        FROM tokens t
        JOIN users u ON t.user_id = u.id
        WHERE t.name = 'wisemodel-token'
          AND t.status IN (1, 2)
          AND u.phone != ''
          AND t.user_id NOT IN (
              SELECT user_id FROM wisemodel_packages
              WHERE valid_until IS NULL OR valid_until > NOW()
          )
        ORDER BY u.phone"
}

# ──────────────────────────────────────────────
# 调用 API
# ──────────────────────────────────────────────
call_create_order() {
    local phone="$1"
    local order_id
    order_id=$(gen_order_id)
    local created_at
    created_at=$(date '+%Y-%m-%dT%H:%M:%S+08:00')

    local payload
    payload=$(cat <<EOF
{
  "order_id": "${order_id}",
  "package_count": 1,
  "packages": [{
    "id": "${PKG_ID}",
    "amount": ${PKG_AMOUNT},
    "points": ${PKG_POINTS},
    "is_free": false,
    "phone": "${phone}",
    "created_at": "${created_at}",
    "valid_until": "${PKG_VALID_UNTIL}"
  }]
}
EOF
)

    local response http_code body
    response=$(curl -s -w "\n%{http_code}" \
        --max-time 15 \
        -X POST "${API_BASE}/api/wisemodel/orders/record" \
        -H "Content-Type: application/json" \
        -H "Authorization: Bearer ${WISEMODEL_API_TOKEN}" \
        -d "$payload" 2>/dev/null)

    http_code=$(echo "$response" | tail -1)
    body=$(echo "$response" | head -n -1)

    echo "$http_code|$body|$order_id"
}

# ──────────────────────────────────────────────
# 主流程
# ──────────────────────────────────────────────
main() {
    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║      Wisemodel 资源包补录脚本                   ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════╝${NC}"
    echo ""

    if [ "$EXECUTE" = "--execute" ]; then
        echo -e "  模式: ${RED}执行模式（将调用 API 写入数据库）${NC}"
    else
        echo -e "  模式: ${YELLOW}Dry-Run（只预览，不调用 API）${NC}  加 --execute 才真正补录"
    fi

    load_env
    setup_executor

    echo -e "  数据库: ${DB_HOST}:${DB_PORT}/${DB_NAME}  执行器: ${EXEC_MODE}"
    echo -e "  API: ${API_BASE}/api/wisemodel/orders/record"
    echo -e "  套餐: ${PKG_ID}  points=${PKG_POINTS}  amount=${PKG_AMOUNT}  valid_until=${PKG_VALID_UNTIL}"
    echo ""

    # 修复 valid_until 列类型 timestamp → datetime（支持 2038 年以后的日期）
    info "检查 valid_until 列类型..."
    col_type=$(qs "SELECT DATA_TYPE FROM information_schema.COLUMNS
                   WHERE TABLE_SCHEMA = DATABASE()
                     AND TABLE_NAME = 'wisemodel_packages'
                     AND COLUMN_NAME = 'valid_until'")
    if [ "$col_type" = "timestamp" ]; then
        warn "valid_until 列类型为 timestamp，正在迁移为 datetime..."
        qs "ALTER TABLE wisemodel_packages
            MODIFY COLUMN valid_until datetime NULL,
            MODIFY COLUMN created_at  datetime NOT NULL DEFAULT CURRENT_TIMESTAMP"
        ok "列类型迁移完成（timestamp → datetime）"
    else
        ok "valid_until 列类型: ${col_type}，无需迁移"
    fi
    echo ""

    # 查询需要补录的手机号
    local phones
    phones=$(get_phones_without_packages)

    if [ -z "$phones" ]; then
        ok "没有需要补录的用户，所有 wisemodel-token 用户均有有效资源包"
        echo ""
        return
    fi

    local total
    total=$(echo "$phones" | wc -l | tr -d ' ')
    warn "发现 ${total} 个用户需要补录资源包："
    echo ""

    local idx=0
    while IFS= read -r phone; do
        [ -z "$phone" ] && continue
        idx=$((idx+1))

        if [ "$EXECUTE" = "--execute" ]; then
            action "[${idx}/${total}] 正在补录 phone=${phone} ..."
            result=$(call_create_order "$phone")
            http_code=$(echo "$result" | cut -d'|' -f1)
            body=$(echo "$result" | cut -d'|' -f2)
            order_id=$(echo "$result" | cut -d'|' -f3)

            if [ "$http_code" = "200" ]; then
                ok "[${idx}/${total}] phone=${phone}  order_id=${order_id}  响应: ${body}"
            else
                fail "[${idx}/${total}] phone=${phone}  HTTP ${http_code}  响应: ${body}"
            fi
        else
            dryrun "[${idx}/${total}] phone=${phone}  → 将调用 CreateOrder（order_id 运行时生成）"
        fi
    done <<< "$phones"

    echo ""

    if [ "$EXECUTE" = "--execute" ]; then
        if [ "$FAILED" -eq 0 ]; then
            ok "全部 ${total} 个用户补录成功"
        else
            fail "${FAILED} 个用户补录失败（见上方 [!!] 标记），请检查后手动重试"
        fi

        # 补录后重新启用之前被禁用的 token
        echo ""
        info "补录完成，重新启用已有有效包用户的 token（status=2 → 1）..."
        qs "UPDATE tokens
            SET status = 1
            WHERE name = 'wisemodel-token'
              AND status = 2
              AND user_id IN (
                  SELECT user_id FROM wisemodel_packages
                  WHERE valid_until IS NULL OR valid_until > NOW()
              )"
        ok "token 重新启用完成"
    fi

    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║                    完成                         ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════╝${NC}"
    echo ""
}

main
