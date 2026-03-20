#!/bin/bash

# Wisemodel 相关数据导出脚本
# 导出以下表的所有关联数据：
#   - wisemodel_packages  全量
#   - users               wisemodel_key != '' 的用户
#   - tokens              name = 'wisemodel-token' 的 token
#   - logs                wisemodel 用户的充值/消费记录（type 1=充值, 2=消费）

set -euo pipefail

# ──────────────────────────────────────────────
# 配置
# ──────────────────────────────────────────────
ENV_FILE="${ENV_FILE:-.env.dev}"
OUTPUT_DIR="${OUTPUT_DIR:-./tmp/wisemodel-export-$(date +%Y%m%d_%H%M%S)}"
FORMAT="${FORMAT:-sql}"   # sql | csv

# ──────────────────────────────────────────────
# 颜色输出
# ──────────────────────────────────────────────
GREEN='\033[0;32m'
RED='\033[0;31m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m'

log_info()    { echo -e "${BLUE}[INFO]${NC} $1"; }
log_success() { echo -e "${GREEN}[OK]${NC}   $1"; }
log_error()   { echo -e "${RED}[ERR]${NC}  $1"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC} $1"; }

# ──────────────────────────────────────────────
# 解析 DSN：root:pass@tcp(host:port)/dbname
# ──────────────────────────────────────────────
parse_dsn() {
    local dsn="$1"
    # 提取 user:password
    local userpass="${dsn%%@*}"
    DB_USER="${userpass%%:*}"
    DB_PASS="${userpass#*:}"
    # 提取 host:port
    local hostpart="${dsn#*tcp(}"
    hostpart="${hostpart%%)*}"
    DB_HOST="${hostpart%%:*}"
    DB_PORT="${hostpart##*:}"
    # 提取 dbname
    DB_NAME="${dsn##*/}"
}

# ──────────────────────────────────────────────
# 读取 DSN（兼容含括号的值，不使用 source）
# ──────────────────────────────────────────────
load_dsn() {
    if [ ! -f "$ENV_FILE" ]; then
        log_error "未找到环境文件: $ENV_FILE"
        log_info  "可通过 ENV_FILE=路径 指定，或手动设置 DB_HOST/DB_PORT/DB_USER/DB_PASS/DB_NAME"
        exit 1
    fi
    local raw
    raw=$(grep '^SQL_DSN=' "$ENV_FILE" | head -1 | cut -d= -f2-)
    if [ -z "$raw" ]; then
        log_error "未在 $ENV_FILE 中找到 SQL_DSN"
        exit 1
    fi
    parse_dsn "$raw"
}

# ──────────────────────────────────────────────
# 允许通过环境变量直接覆盖数据库连接参数
# ──────────────────────────────────────────────
DB_HOST="${DB_HOST:-}"
DB_PORT="${DB_PORT:-3306}"
DB_USER="${DB_USER:-}"
DB_PASS="${DB_PASS:-}"
DB_NAME="${DB_NAME:-}"

if [ -z "$DB_HOST" ]; then
    load_dsn
fi


# ──────────────────────────────────────────────
# MySQL 执行器（本地 or docker exec）
# ──────────────────────────────────────────────
MYSQL_CONTAINER="${MYSQL_CONTAINER:-mysql-dev}"

# 根据环境决定执行方式，优先本地，回退 docker exec
setup_executor() {
    if command -v mysql &>/dev/null && command -v mysqldump &>/dev/null; then
        EXEC_MODE="local"
        log_info "执行模式: 本地 mysql client"
    elif docker inspect "$MYSQL_CONTAINER" &>/dev/null 2>&1; then
        EXEC_MODE="docker"
        # 容器内部用 3306，不用宿主机映射端口
        DB_HOST_INNER="localhost"
        DB_PORT_INNER="3306"
        log_info "执行模式: docker exec ${MYSQL_CONTAINER}"
    else
        log_error "未找到本地 mysql 命令，也未找到容器 ${MYSQL_CONTAINER}"
        log_info  "可通过 MYSQL_CONTAINER=容器名 指定 MySQL 容器"
        exit 1
    fi
}

# 封装 mysql 执行
run_mysql() {
    if [ "$EXEC_MODE" = "docker" ]; then
        docker exec "$MYSQL_CONTAINER" mysql \
            -h"${DB_HOST_INNER}" -P"${DB_PORT_INNER}" \
            -u"${DB_USER}" -p"${DB_PASS}" "${DB_NAME}" "$@"
    else
        mysql -h"${DB_HOST}" -P"${DB_PORT}" -u"${DB_USER}" -p"${DB_PASS}" "${DB_NAME}" "$@"
    fi
}

# 封装 mysqldump 执行
run_mysqldump() {
    if [ "$EXEC_MODE" = "docker" ]; then
        docker exec "$MYSQL_CONTAINER" mysqldump \
            -h"${DB_HOST_INNER}" -P"${DB_PORT_INNER}" \
            -u"${DB_USER}" -p"${DB_PASS}" "$@"
    else
        mysqldump -h"${DB_HOST}" -P"${DB_PORT}" -u"${DB_USER}" -p"${DB_PASS}" "$@"
    fi
}

check_connection() {
    log_info "测试数据库连接 ${DB_HOST}:${DB_PORT}/${DB_NAME} ..."
    if ! run_mysql -e "SELECT 1" &>/dev/null 2>&1; then
        log_error "数据库连接失败，请检查连接参数"
        exit 1
    fi
    log_success "数据库连接正常"
}

# ──────────────────────────────────────────────
# 导出函数
# ──────────────────────────────────────────────

# SQL 格式导出（可直接 source 还原）
export_sql() {
    local table="$1"
    local where="$2"
    local outfile="${OUTPUT_DIR}/${table}.sql"

    log_info "导出 ${table} (SQL) ..."
    echo "-- ══════════════════════════════════════════"
    echo "-- Table: ${table}  Filter: ${where:-全量}"
    echo "-- ══════════════════════════════════════════"

    # 写表头注释到文件
    {
        echo "-- =========================================="
        echo "-- Table: ${table}"
        echo "-- Exported: $(date '+%Y-%m-%d %H:%M:%S')"
        echo "-- Filter: ${where:-全量}"
        echo "-- =========================================="
        echo ""
    } > "$outfile"

    # 导出数据，同时打印到屏幕
    if [ -z "$where" ]; then
        run_mysqldump \
            --no-create-info \
            --single-transaction \
            --skip-add-locks \
            --complete-insert \
            "${DB_NAME}" "${table}" 2>/dev/null | tee -a "$outfile"
    else
        run_mysqldump \
            --no-create-info \
            --single-transaction \
            --skip-add-locks \
            --complete-insert \
            --where="${where}" \
            "${DB_NAME}" "${table}" 2>/dev/null | tee -a "$outfile"
    fi

    local rows
    rows=$(run_mysql --silent --skip-column-names -e "SELECT COUNT(*) FROM \`${table}\`${where:+ WHERE ${where}}" 2>/dev/null || echo "?")
    log_success "  → ${outfile}  (${rows} 行)"
    echo ""
}

# CSV 格式导出（便于 Excel/数据分析查看）
export_csv() {
    local table="$1"
    local where="$2"
    local outfile="${OUTPUT_DIR}/${table}.csv"

    log_info "导出 ${table} (CSV) ..."
    echo "── ${table}  Filter: ${where:-全量} ──"

    local sql
    if [ -z "$where" ]; then
        sql="SELECT * FROM \`${table}\`"
    else
        sql="SELECT * FROM \`${table}\` WHERE ${where}"
    fi

    run_mysql --batch --execute="${sql}" 2>/dev/null | tee "$outfile"

    local rows
    rows=$(run_mysql --silent --skip-column-names -e "SELECT COUNT(*) FROM \`${table}\`${where:+ WHERE ${where}}" 2>/dev/null || echo "?")
    log_success "  → ${outfile}  (${rows} 行)"
    echo ""
}

export_table() {
    local table="$1"
    local where="$2"
    if [ "$FORMAT" = "csv" ]; then
        export_csv "$table" "$where"
    else
        export_sql "$table" "$where"
    fi
}

# ──────────────────────────────────────────────
# 主流程
# ──────────────────────────────────────────────
main() {
    echo ""
    echo "╔══════════════════════════════════════════════════╗"
    echo "║         Wisemodel 数据导出工具                  ║"
    echo "╚══════════════════════════════════════════════════╝"
    echo ""
    echo "  数据库  : ${DB_HOST}:${DB_PORT}/${DB_NAME}"
    echo "  输出目录: ${OUTPUT_DIR}"
    echo "  导出格式: ${FORMAT}"
    echo ""

    setup_executor
    check_connection

    mkdir -p "$OUTPUT_DIR"

    # ── 1. wisemodel_packages 全量 ──
    export_table "wisemodel_packages" ""

    # ── 2. users：有 wisemodel_key 的用户 ──
    export_table "users" "wisemodel_key != ''"

    # ── 3. tokens：wisemodel token ──
    export_table "tokens" "name = 'wisemodel-token'"

    # ── 4. logs：wisemodel 用户的充值(1)/消费(2)记录 ──
    export_table "logs" \
        "user_id IN (SELECT id FROM users WHERE wisemodel_key != '') AND type IN (1, 2)"

    # ── 汇总统计 ──
    echo ""
    echo "╔══════════════════════════════════════════════════╗"
    echo "║                  导出汇总                       ║"
    echo "╚══════════════════════════════════════════════════╝"
    echo ""

    local wm_pkg_count user_count token_count log_count
    wm_pkg_count=$(run_mysql --silent --skip-column-names -e "SELECT COUNT(*) FROM wisemodel_packages" 2>/dev/null || echo "?")
    user_count=$(run_mysql --silent --skip-column-names -e "SELECT COUNT(*) FROM users WHERE wisemodel_key != ''" 2>/dev/null || echo "?")
    token_count=$(run_mysql --silent --skip-column-names -e "SELECT COUNT(*) FROM tokens WHERE name = 'wisemodel-token'" 2>/dev/null || echo "?")
    log_count=$(run_mysql --silent --skip-column-names -e "SELECT COUNT(*) FROM logs WHERE user_id IN (SELECT id FROM users WHERE wisemodel_key != '') AND type IN (1, 2)" 2>/dev/null || echo "?")

    printf "  %-20s %s 行\n" "wisemodel_packages" "$wm_pkg_count"
    printf "  %-20s %s 行\n" "users" "$user_count"
    printf "  %-20s %s 行\n" "tokens" "$token_count"
    printf "  %-20s %s 行\n" "logs" "$log_count"
    echo ""
    echo -e "${GREEN}导出完成！文件保存在: ${OUTPUT_DIR}/${NC}"
    echo ""
}

main
