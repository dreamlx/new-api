#!/bin/bash

# 回填 wisemodel_packages.original_package_id
#
# 规则：
# 1. package_id = 'package16' 或以 'package16' 为前缀 → original_package_id = 'package16'
# 2. 其他情况                                      → original_package_id = package_id
#
# 默认只预览，不写库；加 --execute 才真正更新。
#
# 用法：
#   ./scripts/backfill-wisemodel-original-package-id.sh
#   ./scripts/backfill-wisemodel-original-package-id.sh --execute
#
# 环境变量：
#   ENV_FILE=.env.dev
#   MYSQL_CONTAINER=mysql-dev

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

computed_original_expr() {
    cat <<'EOF'
CASE
  WHEN package_id = 'package16' OR package_id LIKE 'package16%' THEN 'package16'
  ELSE package_id
END
EOF
}

main() {
    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║      回填 wisemodel_packages.original_package_id            ║${NC}"
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

    local has_table
    has_table=$(qs "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='${DB_NAME}' AND table_name='wisemodel_packages'")
    [ "${has_table:-0}" = "0" ] && fail "wisemodel_packages 表不存在"

    local has_column
    has_column=$(qs "SELECT COUNT(*) FROM information_schema.columns WHERE table_schema='${DB_NAME}' AND table_name='wisemodel_packages' AND column_name='original_package_id'")
    [ "${has_column:-0}" = "0" ] && fail "original_package_id 列不存在，请先启动服务完成 AutoMigrate"

    local expr
    expr="$(computed_original_expr)"

    echo ""
    info "待回填统计"
    qt "SELECT
          COUNT(*) AS pending_rows,
          SUM(CASE WHEN package_id = 'package16' OR package_id LIKE 'package16%' THEN 1 ELSE 0 END) AS package16_rows,
          SUM(CASE WHEN NOT (package_id = 'package16' OR package_id LIKE 'package16%') THEN 1 ELSE 0 END) AS other_rows
        FROM wisemodel_packages
        WHERE original_package_id IS NULL OR original_package_id = '';"

    echo ""
    info "样例预览（前 20 条）"
    qt "SELECT
          id,
          package_id,
          COALESCE(NULLIF(original_package_id, ''), '(empty)') AS current_original_package_id,
          ${expr} AS target_original_package_id
        FROM wisemodel_packages
        WHERE original_package_id IS NULL OR original_package_id = ''
        ORDER BY created_at DESC
        LIMIT 20;"

    if [ "$EXECUTE" != "--execute" ]; then
        echo ""
        ok "预览完成，确认无误后执行：./scripts/backfill-wisemodel-original-package-id.sh --execute"
        echo ""
        return
    fi

    echo ""
    info "开始回填 original_package_id ..."
    qs "UPDATE wisemodel_packages
        SET original_package_id = ${expr}
        WHERE original_package_id IS NULL OR original_package_id = '';"

    echo ""
    info "回填后统计"
    qt "SELECT
          COUNT(*) AS empty_rows_after,
          SUM(original_package_id = 'package16') AS package16_rows_after
        FROM wisemodel_packages;"

    echo ""
    ok "回填完成"
    echo ""
}

main
