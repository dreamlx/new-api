#!/bin/bash
# backfill-wisemodel-logs.sh
# 将 logs.wisemodel_package_id 中的 NULL 值改为空字符串 ''
# 必须在部署 Phase B 功能后、服务启动前执行
#
# 用法: ./scripts/backfill-wisemodel-logs.sh [--dry-run]
#   --dry-run  只显示受影响行数，不实际修改

set -euo pipefail

ENV_FILE="${ENV_FILE:-.env.dev}"
MYSQL_CONTAINER="${MYSQL_CONTAINER:-mysql-dev}"
DRY_RUN=false

GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; BOLD='\033[1m'; NC='\033[0m'

for arg in "$@"; do
    case "$arg" in
        --dry-run) DRY_RUN=true ;;
    esac
done

# ── DSN 解析 ────────────────────────────────────────────────────────────────
parse_dsn() {
    local dsn="$1"
    DB_USER="${dsn%%:*}"; local rest="${dsn#*:}"
    DB_PASS="${rest%%@*}"; rest="${rest#*tcp(}"
    DB_HOST="${rest%%:*}"; rest="${rest#*:}"
    DB_PORT="${rest%%)*}"; DB_NAME="${dsn##*/}"
    # 去掉 DB_NAME 中的查询参数
    DB_NAME="${DB_NAME%%\?*}"
}

DB_HOST="${DB_HOST:-}"; DB_PORT="${DB_PORT:-3306}"
DB_USER="${DB_USER:-}"; DB_PASS="${DB_PASS:-}"; DB_NAME="${DB_NAME:-}"

if [ -z "$DB_HOST" ]; then
    [ ! -f "$ENV_FILE" ] && { echo -e "${RED}未找到 $ENV_FILE${NC}"; exit 1; }
    raw=$(grep '^SQL_DSN=' "$ENV_FILE" | head -1 | cut -d= -f2-)
    parse_dsn "$raw"
fi

# ── 选择执行模式 ─────────────────────────────────────────────────────────────
if command -v mysql &>/dev/null; then
    EXEC_MODE="local"
elif docker inspect "$MYSQL_CONTAINER" &>/dev/null 2>&1; then
    EXEC_MODE="docker"; DB_HOST_INNER="localhost"; DB_PORT_INNER="3306"
else
    echo -e "${RED}未找到 mysql 命令或容器 ${MYSQL_CONTAINER}${NC}"; exit 1
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

# ── 主流程 ────────────────────────────────────────────────────────────────────
main() {
    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║         Wisemodel Logs Backfill: NULL → ''              ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════════════╝${NC}"
    echo -e "  数据库: ${DB_HOST}:${DB_PORT}/${DB_NAME}  模式: ${EXEC_MODE}"
    if $DRY_RUN; then
        echo -e "  ${YELLOW}[DRY RUN 模式] 不会实际修改数据${NC}"
    fi
    echo ""

    # 1. 检查 wisemodel_package_id 列是否存在
    col_exists=$(qs "SELECT COUNT(*) FROM information_schema.columns
        WHERE table_schema='${DB_NAME}'
          AND table_name='logs'
          AND column_name='wisemodel_package_id'" 2>/dev/null || echo "0")

    if [ "${col_exists:-0}" = "0" ]; then
        echo -e "  ${RED}[错误] logs.wisemodel_package_id 列不存在${NC}"
        echo -e "  请先启动服务使 GORM AutoMigrate 建列：${YELLOW}make start${NC}"
        exit 1
    fi
    echo -e "  ${GREEN}✓ 列 logs.wisemodel_package_id 已存在${NC}"

    # 2. 统计 NULL 行数
    null_count=$(qs "SELECT COUNT(*) FROM logs WHERE wisemodel_package_id IS NULL")
    total_logs=$(qs "SELECT COUNT(*) FROM logs")
    echo ""
    echo -e "  总 logs 行数      : ${BOLD}${total_logs}${NC}"
    echo -e "  NULL 待回填行数   : ${YELLOW}${null_count}${NC}"

    if [ "${null_count:-0}" = "0" ]; then
        echo ""
        echo -e "  ${GREEN}✅ 无需回填，所有行均已有值${NC}"
        echo ""
        exit 0
    fi

    if $DRY_RUN; then
        echo ""
        echo -e "  ${YELLOW}[DRY RUN] 将会更新 ${null_count} 行（跳过实际执行）${NC}"
        echo ""
        exit 0
    fi

    # 3. 执行回填
    echo ""
    echo -e "  正在回填 ${null_count} 行..."
    qs "UPDATE logs SET wisemodel_package_id = '' WHERE wisemodel_package_id IS NULL"

    # 4. 验证回填结果
    remaining=$(qs "SELECT COUNT(*) FROM logs WHERE wisemodel_package_id IS NULL")
    echo ""
    if [ "${remaining:-0}" = "0" ]; then
        echo -e "  ${GREEN}✅ 回填完成！更新了 ${null_count} 行，剩余 NULL: 0${NC}"
    else
        echo -e "  ${RED}⚠ 回填后仍有 ${remaining} 行为 NULL，请重试${NC}"
        exit 1
    fi
    echo ""
}

main
