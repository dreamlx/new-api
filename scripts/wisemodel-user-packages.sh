#!/bin/bash

# Wisemodel 用户资源包统计脚本
# 列出每个 wisemodel 用户拥有的资源包数量及状态

set -euo pipefail

ENV_FILE="${ENV_FILE:-.env.dev}"
MYSQL_CONTAINER="${MYSQL_CONTAINER:-mysql-dev}"

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; BOLD='\033[1m'; NC='\033[0m'

parse_dsn() {
    local dsn="$1"
    DB_USER="${dsn%%:*}"; local rest="${dsn#*:}"
    DB_PASS="${rest%%@*}"; rest="${rest#*tcp(}"
    DB_HOST="${rest%%:*}"; rest="${rest#*:}"
    DB_PORT="${rest%%)*}"; DB_NAME="${dsn##*/}"
}

DB_HOST="${DB_HOST:-}"; DB_PORT="${DB_PORT:-3306}"
DB_USER="${DB_USER:-}"; DB_PASS="${DB_PASS:-}"; DB_NAME="${DB_NAME:-}"

if [ -z "$DB_HOST" ]; then
    [ ! -f "$ENV_FILE" ] && { echo "未找到 $ENV_FILE"; exit 1; }
    raw=$(grep '^SQL_DSN=' "$ENV_FILE" | head -1 | cut -d= -f2-)
    parse_dsn "$raw"
fi

if command -v mysql &>/dev/null; then
    EXEC_MODE="local"
elif docker inspect "$MYSQL_CONTAINER" &>/dev/null 2>&1; then
    EXEC_MODE="docker"; DB_HOST_INNER="localhost"; DB_PORT_INNER="3306"
else
    echo "未找到 mysql 或容器 ${MYSQL_CONTAINER}"; exit 1
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

qt() {
    if [ "$EXEC_MODE" = "docker" ]; then
        docker exec "$MYSQL_CONTAINER" mysql \
            -h"${DB_HOST_INNER}" -P"${DB_PORT_INNER}" \
            -u"${DB_USER}" -p"${DB_PASS}" "${DB_NAME}" \
            -e "$1" 2>/dev/null
    else
        mysql -h"${DB_HOST}" -P"${DB_PORT}" -u"${DB_USER}" -p"${DB_PASS}" "${DB_NAME}" \
            -e "$1" 2>/dev/null
    fi
}

main() {
    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║           Wisemodel 用户资源包统计                          ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════════════════╝${NC}"
    echo -e "  数据库: ${DB_HOST}:${DB_PORT}/${DB_NAME}  执行器: ${EXEC_MODE}"
    echo ""

    # ── 汇总行 ────────────────────────────────────────────────────────
    total_users=$(qs "SELECT COUNT(DISTINCT user_id) FROM tokens WHERE name='wisemodel-token'")
    users_with_pkg=$(qs "SELECT COUNT(DISTINCT t.user_id)
                         FROM tokens t
                         WHERE t.name='wisemodel-token'
                           AND t.user_id IN (SELECT DISTINCT user_id FROM wisemodel_packages)")
    users_no_pkg=$((total_users - users_with_pkg))

    echo -e "  wisemodel-token 用户总数: ${BOLD}${total_users}${NC}"
    echo -e "  有资源包: ${GREEN}${users_with_pkg}${NC}    无资源包: ${RED}${users_no_pkg}${NC}"
    echo ""

    # ── 明细表：每个用户的资源包情况 ──────────────────────────────────
    echo -e "${BLUE}[ 用户资源包明细 ]${NC}"
    echo ""
    qt "SELECT
          u.phone                                          AS 手机号,
          u.id                                             AS user_id,
          t.status                                         AS token状态,
          COUNT(p.id)                                      AS 总包数,
          SUM(CASE WHEN p.valid_until IS NULL OR p.valid_until > NOW() THEN 1 ELSE 0 END)
                                                           AS 有效包数,
          SUM(CASE WHEN p.valid_until IS NOT NULL AND p.valid_until <= NOW() THEN 1 ELSE 0 END)
                                                           AS 已过期包数,
          IFNULL(MAX(p.valid_until), '无资源包')           AS 最远有效期
        FROM tokens t
        JOIN users u ON t.user_id = u.id
        LEFT JOIN wisemodel_packages p ON p.user_id = t.user_id
        WHERE t.name = 'wisemodel-token'
        GROUP BY u.id, u.phone, t.status
        ORDER BY 有效包数 DESC, 总包数 DESC, u.phone;"

    echo ""

    # ── 无有效包的用户（会被中间件拦截）──────────────────────────────
    blocked=$(qs "SELECT COUNT(DISTINCT t.user_id)
                  FROM tokens t
                  WHERE t.name='wisemodel-token'
                    AND t.status=1
                    AND t.user_id NOT IN (
                        SELECT user_id FROM wisemodel_packages
                        WHERE valid_until IS NULL OR valid_until > NOW()
                    )")

    if [ "$blocked" -gt 0 ]; then
        echo -e "${RED}[ 警告：${blocked} 个启用中的 token 无有效资源包，请求将被拦截 ]${NC}"
        echo ""
        qt "SELECT u.phone, u.id AS user_id,
                   COUNT(p.id) AS 历史包数
            FROM tokens t
            JOIN users u ON t.user_id = u.id
            LEFT JOIN wisemodel_packages p ON p.user_id = t.user_id
            WHERE t.name='wisemodel-token' AND t.status=1
              AND t.user_id NOT IN (
                  SELECT user_id FROM wisemodel_packages
                  WHERE valid_until IS NULL OR valid_until > NOW()
              )
            GROUP BY u.id, u.phone
            ORDER BY u.phone;"
        echo ""
        echo -e "  → 运行 ${YELLOW}./scripts/backfill-wisemodel-packages.sh --execute${NC} 补录资源包"
    else
        echo -e "  ${GREEN}所有启用中的 token 均有有效资源包${NC}"
    fi

    echo ""
}

main
