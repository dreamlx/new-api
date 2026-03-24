#!/bin/bash
#
# backfill-wisemodel-available-models.sh
#
# 为历史 wisemodel_packages（available_models 为空）补录可用模型列表。
#
# 问题：CreateOrder 早期版本通过硬编码 PackageModels map 填充 available_models，
#       但 map 只有虚构 ID（PKG001/PKG002），生产包（如 package16）全部未匹配，
#       导致 available_models 始终为空，GetPackageUsage 无法回显可用模型。
#
# 用法：
#   # 交互式预览所有空包（不指定任何规则）
#   ./scripts/backfill-wisemodel-available-models.sh
#
#   # 为所有 original_package_id = 'package16' 的空包设置 minimax-m2
#   ./scripts/backfill-wisemodel-available-models.sh \
#       --rule 'package16=minimax-m2'
#
#   # 多个规则（多次 --rule）
#   ./scripts/backfill-wisemodel-available-models.sh \
#       --rule 'package16=minimax-m2' \
#       --rule 'package17=deepseek-v3,deepseek-r1'
#
#   # 给所有空包统一设置（慎用，忽略 original_package_id 差异）
#   ./scripts/backfill-wisemodel-available-models.sh \
#       --all-models 'minimax-m2'
#
#   # 确认无误后执行写入
#   ./scripts/backfill-wisemodel-available-models.sh \
#       --rule 'package16=minimax-m2' --execute
#
# 环境变量：
#   ENV_FILE=.env              （默认按 .env / .env.prod / .env.dev 顺序探测）
#   MYSQL_CONTAINER=mysql-dev  （无本地 mysql 时走 docker exec）

set -euo pipefail

# ──────────────────────────────────────────────
# 参数解析
# ──────────────────────────────────────────────
EXECUTE=false
ALL_MODELS=""
declare -a RULES=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --execute)   EXECUTE=true; shift ;;
        --all-models) ALL_MODELS="$2"; shift 2 ;;
        --rule)      RULES+=("$2"); shift 2 ;;
        *) echo "未知参数: $1"; echo "用法: $0 [--rule 'pkgId=models'] [--all-models 'models'] [--execute]"; exit 1 ;;
    esac
done

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

ok()   { echo -e "  ${GREEN}[OK]${NC}   $1"; }
fail() { echo -e "  ${RED}[!!]${NC}   $1"; exit 1; }
warn() { echo -e "  ${YELLOW}[~~]${NC}   $1"; }
info() { echo -e "  ${BLUE}--${NC}    $1"; }
hdr()  { echo -e "\n${BOLD}${CYAN}▌ $1${NC}"; }

MYSQL_CONTAINER="${MYSQL_CONTAINER:-mysql-dev}"

# ──────────────────────────────────────────────
# DSN 解析
# ──────────────────────────────────────────────
parse_dsn() {
    local dsn="$1"
    local userpass="${dsn%%@*}"
    DB_USER="${userpass%%:*}"
    DB_PASS="${userpass#*:}"
    local hostpart="${dsn#*tcp(}"; hostpart="${hostpart%%)*}"
    DB_HOST="${hostpart%%:*}"; DB_PORT="${hostpart##*:}"
    DB_NAME="${dsn##*/}"
    DB_NAME="${DB_NAME%%\?*}"
}

DB_HOST="${DB_HOST:-}"
DB_PORT="${DB_PORT:-3306}"
DB_USER="${DB_USER:-}"
DB_PASS="${DB_PASS:-}"
DB_NAME="${DB_NAME:-}"

load_env() {
    if [ -n "$DB_HOST" ]; then return; fi

    local candidates=()
    if [ -n "${ENV_FILE:-}" ]; then
        candidates=("$ENV_FILE")
    else
        candidates=(".env" ".env.prod" ".env.production" ".env.dev" "config/.env")
    fi

    local found=""
    for f in "${candidates[@]}"; do
        if [ -f "$f" ]; then found="$f"; break; fi
    done

    if [ -z "$found" ]; then
        fail "未找到 env 文件（试过: ${candidates[*]}），请指定 ENV_FILE=/path/to/.env"
    fi

    local raw
    raw=$(grep '^SQL_DSN=' "$found" 2>/dev/null | head -1 | cut -d= -f2- || true)
    [ -z "${raw:-}" ] && fail "$found 中未找到 SQL_DSN"
    parse_dsn "$raw"
    info "env 文件: $found  DSN 解析完成"
}

# ──────────────────────────────────────────────
# MySQL 执行器
# ──────────────────────────────────────────────
EXEC_MODE=""

setup_executor() {
    if command -v mysql >/dev/null 2>&1; then
        EXEC_MODE="local"
    elif docker inspect "$MYSQL_CONTAINER" >/dev/null 2>&1; then
        EXEC_MODE="docker"
    else
        fail "未找到 mysql 命令，也未找到容器 ${MYSQL_CONTAINER}"
    fi
    info "执行器: ${EXEC_MODE}  DB: ${DB_HOST}:${DB_PORT}/${DB_NAME}"
}

qs() {
    if [ "$EXEC_MODE" = "docker" ]; then
        docker exec "$MYSQL_CONTAINER" mysql \
            -h"localhost" -P"3306" \
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
            -h"localhost" -P"3306" \
            -u"${DB_USER}" -p"${DB_PASS}" "${DB_NAME}" \
            -e "$1" 2>&1
    else
        mysql -h"${DB_HOST}" -P"${DB_PORT}" -u"${DB_USER}" -p"${DB_PASS}" "${DB_NAME}" \
            -e "$1" 2>&1
    fi
}

# ──────────────────────────────────────────────
# 主逻辑
# ──────────────────────────────────────────────
main() {
    echo ""
    echo -e "${BOLD}╔════════════════════════════════════════════════════════╗${NC}"
    if $EXECUTE; then
        echo -e "${BOLD}║  Wisemodel available_models 补录（写入模式）            ║${NC}"
    else
        echo -e "${BOLD}║  Wisemodel available_models 补录（预览模式）            ║${NC}"
    fi
    echo -e "${BOLD}╚════════════════════════════════════════════════════════╝${NC}"
    echo ""

    load_env
    setup_executor

    # ── 现状概览 ────────────────────────────────────────────
    hdr "当前状态：available_models 为空的包"
    qt "SELECT
          original_package_id,
          COUNT(*)        AS pkg_count,
          MIN(created_at) AS oldest,
          MAX(created_at) AS newest
        FROM wisemodel_packages
        WHERE (available_models = '' OR available_models IS NULL)
          AND reclaimed_at IS NULL
        GROUP BY original_package_id
        ORDER BY pkg_count DESC;"

    total_empty=$(qs "SELECT COUNT(*) FROM wisemodel_packages
                      WHERE (available_models = '' OR available_models IS NULL)
                        AND reclaimed_at IS NULL")
    info "合计 ${total_empty} 个包 available_models 为空"

    if [ "${total_empty:-0}" -eq 0 ]; then
        ok "所有包均已有 available_models，无需补录"
        exit 0
    fi

    # ── 无规则时仅展示，退出 ───────────────────────────────
    if [ ${#RULES[@]} -eq 0 ] && [ -z "$ALL_MODELS" ]; then
        echo ""
        warn "未提供补录规则，仅展示现状。使用以下参数指定规则："
        echo ""
        echo -e "    ${YELLOW}# 按 original_package_id 指定模型（推荐）${NC}"
        echo -e "    $0 --rule 'package16=minimax-m2' --rule 'package17=deepseek-v3'"
        echo ""
        echo -e "    ${YELLOW}# 给所有空包统一设置（慎用）${NC}"
        echo -e "    $0 --all-models 'minimax-m2'"
        echo ""
        echo -e "    ${YELLOW}# 确认无误后加 --execute 写入${NC}"
        echo -e "    $0 --rule 'package16=minimax-m2' --execute"
        echo ""
        exit 0
    fi

    # ── 构建并预览 UPDATE 语句 ──────────────────────────────
    hdr "补录计划"

    declare -a update_sqls=()

    if [ -n "$ALL_MODELS" ]; then
        # 单一模型列表覆盖所有空包
        escaped_models="${ALL_MODELS//\'/\'\'}"  # 转义单引号
        count=$(qs "SELECT COUNT(*) FROM wisemodel_packages
                    WHERE (available_models = '' OR available_models IS NULL)
                      AND reclaimed_at IS NULL")
        info "全量补录 ${count} 个包 → available_models = '${ALL_MODELS}'"
        update_sqls+=("UPDATE wisemodel_packages
            SET available_models = '${escaped_models}'
            WHERE (available_models = '' OR available_models IS NULL)
              AND reclaimed_at IS NULL;")
    fi

    for rule in "${RULES[@]}"; do
        pkg_id="${rule%%=*}"
        models="${rule#*=}"
        escaped_pkg="${pkg_id//\'/\'\'}"
        escaped_models="${models//\'/\'\'}"
        count=$(qs "SELECT COUNT(*) FROM wisemodel_packages
                    WHERE original_package_id = '${escaped_pkg}'
                      AND (available_models = '' OR available_models IS NULL)
                      AND reclaimed_at IS NULL")
        if [ "${count:-0}" -eq 0 ]; then
            warn "original_package_id='${pkg_id}' 无匹配的空包，跳过"
        else
            info "规则: '${pkg_id}' → '${models}'（影响 ${count} 个包）"
            update_sqls+=("UPDATE wisemodel_packages
                SET available_models = '${escaped_models}'
                WHERE original_package_id = '${escaped_pkg}'
                  AND (available_models = '' OR available_models IS NULL)
                  AND reclaimed_at IS NULL;")
        fi
    done

    if [ ${#update_sqls[@]} -eq 0 ]; then
        warn "所有规则均无匹配包，无需执行"
        exit 0
    fi

    # ── 执行或仅预览 ────────────────────────────────────────
    echo ""
    if $EXECUTE; then
        hdr "执行写入"
        for sql in "${update_sqls[@]}"; do
            qs "$sql"
        done
        ok "补录完成"

        hdr "补录后状态"
        qt "SELECT
              original_package_id,
              COUNT(*) AS total,
              SUM(available_models = '' OR available_models IS NULL) AS still_empty,
              MIN(available_models) AS sample_models
            FROM wisemodel_packages
            WHERE reclaimed_at IS NULL
            GROUP BY original_package_id
            ORDER BY original_package_id;"
    else
        hdr "预览 SQL（加 --execute 才真正写入）"
        for sql in "${update_sqls[@]}"; do
            echo -e "${YELLOW}${sql}${NC}"
            echo ""
        done
        warn "以上为 DRY-RUN，未修改任何数据"
    fi

    echo ""
    echo -e "${BOLD}╚════════════════════════════════════════════════════════╝${NC}"
    echo ""
}

main
