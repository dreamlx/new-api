#!/bin/bash
# backfill-wisemodel-quota-fix.sh
#
# 修正 wisemodel_packages.quota_granted 和 users.quota 中的历史错误数据。
#
# 问题：CreateOrder 曾将 points/tokens 按 1:1 映射到 QuotaPerUnit，
#       即 quota = points × 500,000，相当于把 1 point 当 $1 处理。
#       正确换算为 1,000,000 points = $1，即 quota = points × 0.5。
#       误差 1,000,000 倍，导致包永远无法耗尽。
#
# 修复：
#   1. wisemodel_packages.quota_granted = original_points(or tokens) * 500000 / 1000000
#   2. users.quota -= (old_quota_granted - new_quota_granted)（减去虚增部分）
#
# 默认只预览；加 --execute 才真正写库。
#
# 用法：
#   ./scripts/backfill-wisemodel-quota-fix.sh
#   ./scripts/backfill-wisemodel-quota-fix.sh --execute
#
# 环境变量：
#   ENV_FILE=.env.dev          （默认读取 SQL_DSN）
#   MYSQL_CONTAINER=mysql-dev  （无本地 mysql 时走 docker exec）

set -euo pipefail

EXECUTE="${1:-}"
ENV_FILE="${ENV_FILE:-.env.dev}"
MYSQL_CONTAINER="${MYSQL_CONTAINER:-mysql-dev}"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

ok()   { echo -e "  ${GREEN}[OK]${NC}  $1"; }
warn() { echo -e "  ${YELLOW}[..]${NC}  $1"; }
info() { echo -e "  ${BLUE}--${NC}   $1"; }
fail() { echo -e "  ${RED}[!!]${NC}  $1"; exit 1; }

parse_dsn() {
    local dsn="$1"
    DB_USER="${dsn%%:*}"
    local rest="${dsn#*:}"
    DB_PASS="${rest%%@*}"
    rest="${rest#*tcp(}"
    DB_HOST="${rest%%:*}"
    rest="${rest#*:}"
    DB_PORT="${rest%%)*}"
    DB_NAME="${dsn##*/}"
}

DB_HOST="${DB_HOST:-}"
DB_PORT="${DB_PORT:-3306}"
DB_USER="${DB_USER:-}"
DB_PASS="${DB_PASS:-}"
DB_NAME="${DB_NAME:-}"

load_env() {
    if [ -z "$DB_HOST" ]; then
        [ ! -f "$ENV_FILE" ] && fail "未找到 $ENV_FILE"
        local raw
        raw=$(grep '^SQL_DSN=' "$ENV_FILE" | head -1 | cut -d= -f2-)
        [ -z "${raw:-}" ] && fail "未找到 SQL_DSN"
        parse_dsn "$raw"
    fi
}

setup_executor() {
    if command -v mysql >/dev/null 2>&1; then
        EXEC_MODE="local"
    elif docker inspect "$MYSQL_CONTAINER" >/dev/null 2>&1; then
        EXEC_MODE="docker"
        DB_HOST_INNER="localhost"
        DB_PORT_INNER="3306"
    else
        fail "未找到 mysql 命令，也未找到容器 ${MYSQL_CONTAINER}"
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

qt() {
    if [ "$EXEC_MODE" = "docker" ]; then
        docker exec "$MYSQL_CONTAINER" mysql \
            -h"${DB_HOST_INNER}" -P"${DB_PORT_INNER}" \
            -u"${DB_USER}" -p"${DB_PASS}" "${DB_NAME}" \
            -e "$1" 2>&1
    else
        mysql -h"${DB_HOST}" -P"${DB_PORT}" -u"${DB_USER}" -p"${DB_PASS}" "${DB_NAME}" \
            -e "$1" 2>&1
    fi
}

main() {
    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║       修正 wisemodel_packages.quota_granted（历史错误）      ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo ""

    if [ "$EXECUTE" = "--execute" ]; then
        warn "执行模式：将更新数据库"
    else
        warn "Dry-Run 模式：仅预览，传 --execute 才会写库"
    fi

    load_env
    setup_executor

    info "数据库: ${DB_HOST}:${DB_PORT}/${DB_NAME}  执行器: ${EXEC_MODE}"

    # 检查表存在
    local has_table
    has_table=$(qs "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='${DB_NAME}' AND table_name='wisemodel_packages'")
    [ "${has_table:-0}" = "0" ] && fail "wisemodel_packages 表不存在"

    echo ""
    info "受影响包预览（quota_granted 将被修正）"
    qt "SELECT
          id,
          user_id,
          original_package_id,
          original_points,
          original_tokens,
          quota_granted                                                             AS quota_old,
          CASE
            WHEN original_points > 0 THEN CAST(original_points AS SIGNED) * 500000 / 1000000
            WHEN original_tokens > 0 THEN CAST(original_tokens AS SIGNED) * 500000 / 1000000
            ELSE quota_granted
          END                                                                       AS quota_new
        FROM wisemodel_packages
        WHERE (original_points > 0 OR original_tokens > 0)
          AND reclaimed_at IS NULL
        ORDER BY created_at DESC;"

    echo ""
    info "受影响用户 quota 余额预估修正量"
    qt "SELECT
          u.id AS user_id,
          u.username,
          u.quota AS quota_balance_now,
          SUM(CASE WHEN wp.original_points > 0
                   THEN CAST(wp.original_points AS SIGNED) * 500000
                        - CAST(wp.original_points AS SIGNED) * 500000 / 1000000
                   WHEN wp.original_tokens > 0
                   THEN CAST(wp.original_tokens AS SIGNED) * 500000
                        - CAST(wp.original_tokens AS SIGNED) * 500000 / 1000000
                   ELSE 0 END)                                                     AS will_deduct,
          u.quota - SUM(CASE WHEN wp.original_points > 0
                             THEN CAST(wp.original_points AS SIGNED) * 500000
                                  - CAST(wp.original_points AS SIGNED) * 500000 / 1000000
                             WHEN wp.original_tokens > 0
                             THEN CAST(wp.original_tokens AS SIGNED) * 500000
                                  - CAST(wp.original_tokens AS SIGNED) * 500000 / 1000000
                             ELSE 0 END)                                           AS quota_balance_after
        FROM wisemodel_packages wp
        JOIN users u ON u.id = wp.user_id
        WHERE (wp.original_points > 0 OR wp.original_tokens > 0)
          AND wp.reclaimed_at IS NULL
        GROUP BY u.id, u.username, u.quota;"

    if [ "$EXECUTE" != "--execute" ]; then
        echo ""
        ok "预览完成，确认无误后执行：./scripts/backfill-wisemodel-quota-fix.sh --execute"
        echo ""
        return
    fi

    echo ""
    info "开始执行修复事务 ..."

    qt "START TRANSACTION;

-- Step 1a: 修正 points 类型包的 quota_granted
UPDATE wisemodel_packages
SET quota_granted = CAST(original_points AS SIGNED) * 500000 / 1000000
WHERE original_points > 0
  AND reclaimed_at IS NULL;

-- Step 1b: 修正 tokens 类型包的 quota_granted
UPDATE wisemodel_packages
SET quota_granted = CAST(original_tokens AS SIGNED) * 500000 / 1000000
WHERE original_tokens > 0
  AND original_points = 0
  AND reclaimed_at IS NULL;

-- Step 2a: 修正 users.quota（减去 points 包的虚增部分）
UPDATE users u
JOIN (
    SELECT user_id,
           SUM(CAST(original_points AS SIGNED) * 500000)           AS old_total,
           SUM(CAST(original_points AS SIGNED) * 500000 / 1000000) AS new_total
    FROM wisemodel_packages
    WHERE original_points > 0
      AND reclaimed_at IS NULL
    GROUP BY user_id
) AS adj ON u.id = adj.user_id
SET u.quota = u.quota - (adj.old_total - adj.new_total);

-- Step 2b: 修正 users.quota（减去 tokens 包的虚增部分）
UPDATE users u
JOIN (
    SELECT user_id,
           SUM(CAST(original_tokens AS SIGNED) * 500000)           AS old_total,
           SUM(CAST(original_tokens AS SIGNED) * 500000 / 1000000) AS new_total
    FROM wisemodel_packages
    WHERE original_tokens > 0
      AND original_points = 0
      AND reclaimed_at IS NULL
    GROUP BY user_id
) AS adj ON u.id = adj.user_id
SET u.quota = u.quota - (adj.old_total - adj.new_total);

COMMIT;"

    echo ""
    info "修复后验证"
    qt "SELECT
          wp.user_id,
          u.username,
          wp.original_package_id,
          wp.original_points,
          wp.quota_granted       AS quota_granted_fixed,
          u.quota                AS user_quota_balance
        FROM wisemodel_packages wp
        JOIN users u ON u.id = wp.user_id
        WHERE (wp.original_points > 0 OR wp.original_tokens > 0)
          AND wp.reclaimed_at IS NULL;"

    echo ""
    ok "修复完成"
    echo ""
}

main
