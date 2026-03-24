#!/bin/bash
#
# diagnose-wisemodel-quota-state.sh
#
# 只读诊断：检查生产环境 wisemodel_packages 的 quota 状态，
# 识别三类问题：
#   A. quota_granted 错误（与 original_points/tokens 换算不符）
#   B. available_models 为空（PackageModels map 未配置）
#   C. 需要补录资源包的用户
#
# 用法（在服务器上）：
#   ENV_FILE=.env ./scripts/diagnose-wisemodel-quota-state.sh
#   ENV_FILE=.env.prod ./scripts/diagnose-wisemodel-quota-state.sh
#
# 环境变量：
#   ENV_FILE      env 文件路径（默认按下方顺序自动探测）
#   DB_HOST / DB_PORT / DB_USER / DB_PASS / DB_NAME  直接指定 DB 参数（覆盖 ENV_FILE）
#   MYSQL_CONTAINER   Docker 容器名（默认 mysql-dev，无本地 mysql 时启用）

set -euo pipefail

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
fail() { echo -e "  ${RED}[!!]${NC}   $1"; ISSUES=$((ISSUES+1)); }
warn() { echo -e "  ${YELLOW}[~~]${NC}   $1"; }
info() { echo -e "  ${BLUE}--${NC}    $1"; }
hdr()  { echo -e "\n${BOLD}${CYAN}▌ $1${NC}"; }

ISSUES=0
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
    DB_NAME="${DB_NAME%%\?*}"   # 去掉 ?parseTime=true 等参数
}

DB_HOST="${DB_HOST:-}"
DB_PORT="${DB_PORT:-3306}"
DB_USER="${DB_USER:-}"
DB_PASS="${DB_PASS:-}"
DB_NAME="${DB_NAME:-}"

load_env() {
    if [ -n "$DB_HOST" ]; then return; fi

    # 自动探测 env 文件
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
        echo -e "  ${RED}[!!]${NC}  未找到 env 文件（试过: ${candidates[*]}）"
        echo -e "        请指定：ENV_FILE=/path/to/.env $0"
        exit 1
    fi

    local raw
    raw=$(grep '^SQL_DSN=' "$found" 2>/dev/null | head -1 | cut -d= -f2- || true)
    if [ -z "${raw:-}" ]; then
        echo -e "  ${RED}[!!]${NC}  $found 中未找到 SQL_DSN"
        exit 1
    fi
    parse_dsn "$raw"
    echo -e "  ${BLUE}--${NC}    env 文件: $found  DSN 解析完成"
}

# ──────────────────────────────────────────────
# MySQL 执行器
# ──────────────────────────────────────────────
EXEC_MODE=""
DB_HOST_INNER=""
DB_PORT_INNER=""

setup_executor() {
    if command -v mysql >/dev/null 2>&1; then
        EXEC_MODE="local"
    elif docker inspect "$MYSQL_CONTAINER" >/dev/null 2>&1; then
        EXEC_MODE="docker"
        DB_HOST_INNER="localhost"
        DB_PORT_INNER="3306"
    else
        echo -e "  ${RED}[!!]${NC}  未找到 mysql 命令，也未找到容器 ${MYSQL_CONTAINER}"
        echo -e "        请确保 mysql client 已安装，或设置 MYSQL_CONTAINER 环境变量"
        exit 1
    fi
    info "执行器: ${EXEC_MODE}  DB: ${DB_HOST}:${DB_PORT}/${DB_NAME}"
}

# 静默查询（返回值用）
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

# 带表头的输出
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

# ──────────────────────────────────────────────
# 主诊断
# ──────────────────────────────────────────────
main() {
    echo ""
    echo -e "${BOLD}╔════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║      Wisemodel Quota 状态诊断（只读）                  ║${NC}"
    echo -e "${BOLD}╚════════════════════════════════════════════════════════╝${NC}"
    echo ""

    load_env
    setup_executor

    # ── 1. 汇总概览 ─────────────────────────────────────────
    hdr "1. 资源包汇总"
    qt "SELECT
          COUNT(*)                                               AS total_packages,
          SUM(original_points > 0)                              AS points_packages,
          SUM(original_tokens > 0 AND original_points = 0)     AS tokens_packages,
          SUM(original_points = 0 AND original_tokens = 0)     AS unknown_type,
          SUM(available_models = '' OR available_models IS NULL) AS missing_models,
          SUM(reclaimed_at IS NOT NULL)                         AS reclaimed,
          SUM(valid_until IS NOT NULL AND valid_until < NOW()
              AND reclaimed_at IS NULL)                         AS expired_not_reclaimed
        FROM wisemodel_packages;"

    # ── 2. quota_granted 异常检测 ────────────────────────────
    hdr "2. quota_granted 异常（应 = original_points × 0.5 或 original_tokens × 0.5）"

    local bad_quota
    bad_quota=$(qs "SELECT COUNT(*)
        FROM wisemodel_packages
        WHERE reclaimed_at IS NULL
          AND (
            (original_points > 0 AND ABS(quota_granted - CAST(original_points AS SIGNED) * 500000 / 1000000) > 1)
            OR
            (original_tokens > 0 AND original_points = 0 AND ABS(quota_granted - CAST(original_tokens AS SIGNED) * 500000 / 1000000) > 1)
          )")

    if [ "${bad_quota:-0}" -gt 0 ]; then
        fail "发现 ${bad_quota} 个包的 quota_granted 与换算公式不符（需要运行 quota-fix 脚本）"
        echo ""
        qt "SELECT
              id,
              original_package_id,
              original_points,
              original_tokens,
              quota_granted                                                       AS quota_granted_current,
              CASE
                WHEN original_points > 0
                THEN CAST(original_points AS SIGNED) * 500000 / 1000000
                WHEN original_tokens > 0
                THEN CAST(original_tokens AS SIGNED) * 500000 / 1000000
                ELSE quota_granted
              END                                                                 AS quota_granted_correct,
              quota_granted - CASE
                WHEN original_points > 0
                THEN CAST(original_points AS SIGNED) * 500000 / 1000000
                WHEN original_tokens > 0
                THEN CAST(original_tokens AS SIGNED) * 500000 / 1000000
                ELSE quota_granted
              END                                                                 AS quota_excess,
              created_at
            FROM wisemodel_packages
            WHERE reclaimed_at IS NULL
              AND (
                (original_points > 0 AND ABS(quota_granted - CAST(original_points AS SIGNED) * 500000 / 1000000) > 1)
                OR
                (original_tokens > 0 AND original_points = 0 AND ABS(quota_granted - CAST(original_tokens AS SIGNED) * 500000 / 1000000) > 1)
              )
            ORDER BY created_at DESC;"
    else
        ok "所有包的 quota_granted 换算正确"
    fi

    # ── 3. original_points=0 且未回收的包（quota-fix 会跳过） ─
    hdr "3. original_points=0 且 original_tokens=0 的未回收包（quota-fix 会跳过）"
    local unknown_type
    unknown_type=$(qs "SELECT COUNT(*) FROM wisemodel_packages
                       WHERE original_points = 0 AND original_tokens = 0 AND reclaimed_at IS NULL")

    if [ "${unknown_type:-0}" -gt 0 ]; then
        fail "发现 ${unknown_type} 个包 original_points=0 且 original_tokens=0（类型未知，需手动检查）"
        qt "SELECT id, original_package_id, quota_granted, available_models, created_at
            FROM wisemodel_packages
            WHERE original_points = 0 AND original_tokens = 0 AND reclaimed_at IS NULL
            ORDER BY created_at DESC LIMIT 10;"
    else
        ok "没有 original_points/tokens 均为 0 的异常包"
    fi

    # ── 4. available_models 为空的包 ──────────────────────────
    hdr "4. available_models 为空的包"
    local missing_models
    missing_models=$(qs "SELECT COUNT(*) FROM wisemodel_packages
                         WHERE (available_models = '' OR available_models IS NULL)
                           AND reclaimed_at IS NULL")

    if [ "${missing_models:-0}" -gt 0 ]; then
        warn "${missing_models} 个有效包 available_models 为空（PackageModels map 未配置对应 id）"
        qt "SELECT original_package_id, COUNT(*) AS cnt, MIN(created_at) AS oldest
            FROM wisemodel_packages
            WHERE (available_models = '' OR available_models IS NULL)
              AND reclaimed_at IS NULL
            GROUP BY original_package_id
            ORDER BY cnt DESC;"
    else
        ok "所有有效包均有 available_models"
    fi

    # ── 5. 各用户包余额现状 ────────────────────────────────────
    hdr "5. 用户资源包余额（quota_granted 视角）"
    qt "SELECT
          u.id                                                   AS user_id,
          u.phone,
          u.username,
          COUNT(wp.id)                                           AS pkg_count,
          SUM(wp.quota_granted)                                  AS total_quota_granted,
          SUM(CASE
            WHEN wp.original_points > 0
            THEN CAST(wp.original_points AS SIGNED) * 500000 / 1000000
            WHEN wp.original_tokens > 0
            THEN CAST(wp.original_tokens AS SIGNED) * 500000 / 1000000
            ELSE wp.quota_granted END)                           AS total_quota_correct,
          SUM(wp.quota_granted) - SUM(CASE
            WHEN wp.original_points > 0
            THEN CAST(wp.original_points AS SIGNED) * 500000 / 1000000
            WHEN wp.original_tokens > 0
            THEN CAST(wp.original_tokens AS SIGNED) * 500000 / 1000000
            ELSE wp.quota_granted END)                           AS quota_excess
        FROM users u
        JOIN wisemodel_packages wp ON wp.user_id = u.id AND wp.reclaimed_at IS NULL
        WHERE u.phone != ''
        GROUP BY u.id, u.phone, u.username
        ORDER BY quota_excess DESC;"

    # ── 6. 需要补录的用户 ─────────────────────────────────────
    hdr "6. 有效 wisemodel-token 但无有效资源包的用户（需要补录）"
    local need_backfill
    need_backfill=$(qs "SELECT COUNT(DISTINCT t.user_id)
        FROM tokens t
        JOIN users u ON t.user_id = u.id
        WHERE t.name = 'wisemodel-token'
          AND t.status = 1
          AND u.phone != ''
          AND t.user_id NOT IN (
              SELECT user_id FROM wisemodel_packages
              WHERE valid_until IS NULL OR valid_until > NOW()
          )")

    if [ "${need_backfill:-0}" -gt 0 ]; then
        warn "${need_backfill} 个用户需要补录"
        qt "SELECT u.phone, u.username, t.name AS token_name
            FROM tokens t
            JOIN users u ON t.user_id = u.id
            WHERE t.name = 'wisemodel-token'
              AND t.status = 1
              AND u.phone != ''
              AND t.user_id NOT IN (
                  SELECT user_id FROM wisemodel_packages
                  WHERE valid_until IS NULL OR valid_until > NOW()
              )
            ORDER BY u.phone;"
    else
        ok "所有 wisemodel-token 用户均已有有效资源包"
    fi

    # ── 7. 小结 ───────────────────────────────────────────────
    echo ""
    echo -e "${BOLD}╔════════════════════════════════════════════════════════╗${NC}"
    if [ "$ISSUES" -eq 0 ]; then
        echo -e "${BOLD}║  ${GREEN}✓ 未发现问题${NC}${BOLD}                                           ║${NC}"
    else
        echo -e "${BOLD}║  ${RED}发现 ${ISSUES} 类问题，见上方 [!!] 标记${NC}${BOLD}                    ║${NC}"
        echo -e "${BOLD}║                                                        ║${NC}"
        echo -e "${BOLD}║  修复步骤：                                            ║${NC}"
        echo -e "${BOLD}║    1. 修正 quota: ./scripts/backfill-wisemodel-quota-fix.sh --execute${NC}${BOLD}  ║${NC}"
        echo -e "${BOLD}║    2. 补录包:     ./scripts/backfill-wisemodel-packages.sh --execute   ║${NC}"
        echo -e "${BOLD}║    3. 重新诊断:   再次运行本脚本确认结果               ║${NC}"
    fi
    echo -e "${BOLD}╚════════════════════════════════════════════════════════╝${NC}"
    echo ""
}

main
