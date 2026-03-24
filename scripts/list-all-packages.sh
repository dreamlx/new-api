#!/bin/bash

# 查看所有 Wisemodel 资源包信息
# 用法: ./scripts/list-all-packages.sh [--expired] [--active] [--free]
# 选项:
#   --expired  只显示已过期资源包
#   --active   只显示有效资源包
#   --free     只显示免费资源包

set -euo pipefail

ENV_FILE="${ENV_FILE:-.env.dev}"
MYSQL_CONTAINER="${MYSQL_CONTAINER:-mysql-dev}"

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; BOLD='\033[1m'; NC='\033[0m'

# 解析参数
FILTER=""
FILTER_LABEL="全部"
for arg in "$@"; do
    case "$arg" in
        --expired) FILTER="AND (p.valid_until IS NOT NULL AND p.valid_until <= NOW())"; FILTER_LABEL="已过期" ;;
        --active)  FILTER="AND (p.valid_until IS NULL OR p.valid_until > NOW())";      FILTER_LABEL="有效中" ;;
        --free)    FILTER="AND p.is_free = 1";                                          FILTER_LABEL="免费包" ;;
    esac
done

# 解析 DSN
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
            -e "$1" 2>&1
    else
        mysql -h"${DB_HOST}" -P"${DB_PORT}" -u"${DB_USER}" -p"${DB_PASS}" "${DB_NAME}" \
            -e "$1" 2>&1
    fi
}

main() {
    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║              所有 Wisemodel 资源包信息  [${FILTER_LABEL}]              ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════════════════════╝${NC}"
    echo -e "  数据库: ${DB_HOST}:${DB_PORT}/${DB_NAME}  执行器: ${EXEC_MODE}"
    echo ""

    # ── 检查表是否存在 ────────────────────────────────────────────────
    table_exists=$(qs "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='${DB_NAME}' AND table_name='wisemodel_packages'" || echo "0")
    if [ "${table_exists:-0}" = "0" ]; then
        echo -e "  ${RED}[错误] wisemodel_packages 表不存在${NC}"
        echo -e "  请先启动服务使其自动建表："
        echo -e "  ${YELLOW}  make start${NC}"
        exit 1
    fi

    # ── 汇总统计 ─────────────────────────────────────────────────────
    total=$(qs     "SELECT COUNT(*) FROM wisemodel_packages p WHERE 1=1 ${FILTER}")
    active=$(qs    "SELECT COUNT(*) FROM wisemodel_packages p WHERE (p.valid_until IS NULL OR p.valid_until > NOW()) ${FILTER}")
    expired=$(qs   "SELECT COUNT(*) FROM wisemodel_packages p WHERE p.valid_until IS NOT NULL AND p.valid_until <= NOW() ${FILTER}")
    free_cnt=$(qs  "SELECT COUNT(*) FROM wisemodel_packages p WHERE p.is_free = 1 ${FILTER}")
    paid_cnt=$(qs  "SELECT COUNT(*) FROM wisemodel_packages p WHERE p.is_free = 0 ${FILTER}")

    echo -e "  总资源包数: ${BOLD}${total}${NC}  |  有效: ${GREEN}${active}${NC}  |  已过期: ${RED}${expired}${NC}  |  免费: ${YELLOW}${free_cnt}${NC}  |  付费: ${BLUE}${paid_cnt}${NC}"
    echo ""

    # ── 检查 users 表是否存在 ──────────────────────────────────────────
    has_users=$(qs "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='${DB_NAME}' AND table_name='users'" || echo "0")
    has_ori_pkg_id=$(qs "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema='${DB_NAME}' AND table_name='wisemodel_packages' AND column_name='original_package_id'" || echo "0")

    if [ "${has_ori_pkg_id:-0}" != "0" ]; then
        original_pkg_select="CASE WHEN p.original_package_id IS NULL OR p.original_package_id = '' THEN '(empty)' ELSE p.original_package_id END"
    else
        original_pkg_select="'(no-column)'"
    fi

    # ── 资源包明细 ───────────────────────────────────────────────────
    echo -e "${BLUE}[ 资源包明细 ]${NC}"
    echo ""
    if [ "${has_users:-0}" != "0" ]; then
        qt "SELECT
              p.id                                                        AS id,
              ${original_pkg_select}                                      AS original_package_id,
              p.package_id                                                AS package_id,
              p.order_id                                                  AS order_id,
              u.phone                                                     AS user_phone,
              p.user_id                                                   AS user_id,
              p.original_points                                           AS points,
              p.original_tokens                                           AS tokens,
              ROUND(p.quota_granted / 500000, 4)                         AS quota_usd,
              p.amount                                                    AS amount_cny,
              IF(p.is_free, 'FREE', 'PAID')                              AS type,
              IF(p.valid_until IS NULL OR p.valid_until > NOW(),
                 'active', 'expired')                                     AS status,
              DATE_FORMAT(p.valid_until, '%Y-%m-%d %H:%i')               AS valid_until,
              DATE_FORMAT(p.created_at,  '%Y-%m-%d %H:%i')               AS created_at
            FROM wisemodel_packages p
            LEFT JOIN users u ON p.user_id = u.id
            WHERE 1=1 ${FILTER}
            ORDER BY p.created_at DESC;"
    else
        qt "SELECT
              p.id                                                        AS id,
              ${original_pkg_select}                                      AS original_package_id,
              p.package_id                                                AS package_id,
              p.order_id                                                  AS order_id,
              p.user_id                                                   AS user_id,
              p.original_points                                           AS points,
              p.original_tokens                                           AS tokens,
              ROUND(p.quota_granted / 500000, 4)                         AS quota_usd,
              p.amount                                                    AS amount_cny,
              IF(p.is_free, 'FREE', 'PAID')                              AS type,
              IF(p.valid_until IS NULL OR p.valid_until > NOW(),
                 'active', 'expired')                                     AS status,
              DATE_FORMAT(p.valid_until, '%Y-%m-%d %H:%i')               AS valid_until,
              DATE_FORMAT(p.created_at,  '%Y-%m-%d %H:%i')               AS created_at
            FROM wisemodel_packages p
            WHERE 1=1 ${FILTER}
            ORDER BY p.created_at DESC;"
    fi

    echo ""

    # ── 按用户汇总 ───────────────────────────────────────────────────
    echo -e "${BLUE}[ 按用户汇总 ]${NC}"
    echo ""
    if [ "${has_users:-0}" != "0" ]; then
        qt "SELECT
              IFNULL(u.phone, '(unknown)')                                    AS phone,
              p.user_id                                                        AS user_id,
              COUNT(p.id)                                                      AS total_pkgs,
              SUM(CASE WHEN p.valid_until IS NULL OR p.valid_until > NOW() THEN 1 ELSE 0 END) AS active_pkgs,
              SUM(CASE WHEN p.valid_until IS NOT NULL AND p.valid_until <= NOW() THEN 1 ELSE 0 END) AS expired_pkgs,
              SUM(p.original_points)                                           AS total_points,
              SUM(p.original_tokens)                                           AS total_tokens,
              ROUND(SUM(p.quota_granted) / 500000, 4)                         AS total_quota_usd,
              ROUND(SUM(p.amount), 2)                                          AS total_amount_cny,
              MAX(DATE_FORMAT(p.valid_until, '%Y-%m-%d'))                      AS latest_expiry
            FROM wisemodel_packages p
            LEFT JOIN users u ON p.user_id = u.id
            WHERE 1=1 ${FILTER}
            GROUP BY p.user_id, u.phone
            ORDER BY 4 DESC, 3 DESC, u.phone;"
    else
        echo -e "  ${YELLOW}[提示] users 表不存在，仅按 user_id 汇总${NC}"
        echo ""
        qt "SELECT
              p.user_id                                                        AS user_id,
              COUNT(p.id)                                                      AS total_pkgs,
              SUM(CASE WHEN p.valid_until IS NULL OR p.valid_until > NOW() THEN 1 ELSE 0 END) AS active_pkgs,
              SUM(CASE WHEN p.valid_until IS NOT NULL AND p.valid_until <= NOW() THEN 1 ELSE 0 END) AS expired_pkgs,
              SUM(p.original_points)                                           AS total_points,
              SUM(p.original_tokens)                                           AS total_tokens,
              ROUND(SUM(p.quota_granted) / 500000, 4)                         AS total_quota_usd,
              ROUND(SUM(p.amount), 2)                                          AS total_amount_cny,
              MAX(DATE_FORMAT(p.valid_until, '%Y-%m-%d'))                      AS latest_expiry
            FROM wisemodel_packages p
            WHERE 1=1 ${FILTER}
            GROUP BY p.user_id
            ORDER BY 3 DESC, 2 DESC;"
    fi

    echo ""
}

main
