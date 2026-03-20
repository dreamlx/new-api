#!/bin/bash

# Wisemodel 问题数据修复脚本
#
# 修复两类问题：
#   原因① valid_until <= created_at 的时间窗口倒置包记录 → 删除
#   原因④ 有 wisemodel-token 但无任何资源包的用户 → 禁用其 token
#
# 用法：
#   ./scripts/fix-wisemodel-data.sh            # 只 Dry-Run，不改库
#   ./scripts/fix-wisemodel-data.sh --execute  # 真正执行修复
#
# 环境变量覆盖（可选）：
#   ENV_FILE=.env.prod MYSQL_CONTAINER=mysql-prod ./fix-wisemodel-data.sh

set -euo pipefail

EXECUTE="${1:-}"   # --execute 才真正写库
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

ok()      { echo -e "  ${GREEN}[OK]${NC}    $1"; }
fail()    { echo -e "  ${RED}[!!]${NC}    $1"; }
warn()    { echo -e "  ${YELLOW}[~~]${NC}    $1"; }
info()    { echo -e "  ${BLUE}--${NC}      $1"; }
action()  { echo -e "  ${CYAN}[FIX]${NC}   $1"; }
dryrun()  { echo -e "  ${YELLOW}[DRY]${NC}   $1"; }

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

if [ -z "$DB_HOST" ]; then
    [ ! -f "$ENV_FILE" ] && { echo "未找到 $ENV_FILE"; exit 1; }
    raw=$(grep '^SQL_DSN=' "$ENV_FILE" | head -1 | cut -d= -f2-)
    [ -z "$raw" ] && { echo "未找到 SQL_DSN"; exit 1; }
    parse_dsn "$raw"
fi

# ──────────────────────────────────────────────
# 执行器
# ──────────────────────────────────────────────
if command -v mysql &>/dev/null; then
    EXEC_MODE="local"
elif docker inspect "$MYSQL_CONTAINER" &>/dev/null 2>&1; then
    EXEC_MODE="docker"; DB_HOST_INNER="localhost"; DB_PORT_INNER="3306"
else
    echo "未找到 mysql 命令，也未找到容器 ${MYSQL_CONTAINER}"; exit 1
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

# ──────────────────────────────────────────────
# 修复函数
# ──────────────────────────────────────────────

fix_inverted_time_windows() {
    echo ""
    echo -e "${CYAN}[ 修复① ] 删除 valid_until <= created_at 的时间窗口倒置资源包${NC}"
    echo ""

    local count
    count=$(qs "SELECT COUNT(*) FROM wisemodel_packages
                WHERE valid_until IS NOT NULL
                  AND UNIX_TIMESTAMP(valid_until) <= UNIX_TIMESTAMP(created_at)")

    if [ "$count" -eq 0 ]; then
        ok "无时间窗口倒置记录，跳过"
        return
    fi

    warn "发现 ${count} 条时间窗口倒置记录："
    echo ""
    qt "SELECT id, user_id, package_id, order_id,
               created_at,
               valid_until,
               TIMESTAMPDIFF(DAY, valid_until, created_at) AS days_inverted
        FROM wisemodel_packages
        WHERE valid_until IS NOT NULL
          AND UNIX_TIMESTAMP(valid_until) <= UNIX_TIMESTAMP(created_at)
        ORDER BY created_at DESC;"
    echo ""

    if [ "$EXECUTE" = "--execute" ]; then
        # 创建备份表（不复制索引，避免 uniqueIndex 在重复运行时冲突）
        # SELECT WHERE 1=0 只复制列结构，不带约束
        qs "CREATE TABLE IF NOT EXISTS _wisemodel_packages_deleted_backup
              SELECT * FROM wisemodel_packages WHERE 1=0"

        # 在单事务内完成：备份 + 删除，保证原子性
        qs "START TRANSACTION;
            INSERT IGNORE INTO _wisemodel_packages_deleted_backup
              SELECT * FROM wisemodel_packages
              WHERE valid_until IS NOT NULL
                AND UNIX_TIMESTAMP(valid_until) <= UNIX_TIMESTAMP(created_at);
            DELETE FROM wisemodel_packages
              WHERE valid_until IS NOT NULL
                AND UNIX_TIMESTAMP(valid_until) <= UNIX_TIMESTAMP(created_at);
            COMMIT;"

        action "已删除 ${count} 条倒置记录（备份到 _wisemodel_packages_deleted_backup）"
    else
        dryrun "将删除 ${count} 条记录（加 --execute 执行）"
    fi
}

fix_tokens_without_packages() {
    echo ""
    echo -e "${CYAN}[ 修复④ ] 禁用无有效资源包的 wisemodel-token${NC}"
    echo ""

    local count
    count=$(qs "SELECT COUNT(DISTINCT t.id)
                FROM tokens t
                WHERE t.name = 'wisemodel-token'
                  AND t.status = 1
                  AND t.user_id NOT IN (
                      SELECT user_id FROM wisemodel_packages
                      WHERE valid_until IS NULL OR valid_until > NOW()
                  )")

    if [ "$count" -eq 0 ]; then
        ok "所有 wisemodel-token 均有对应有效资源包，无需修复"
        return
    fi

    warn "发现 ${count} 个 wisemodel-token 无有效资源包（若不修复，新中间件将返回 403）："
    echo ""
    qt "SELECT t.id AS token_id, t.user_id, u.phone, u.wisemodel_key,
               t.created_at AS token_created_at,
               (SELECT COUNT(*) FROM wisemodel_packages p WHERE p.user_id = t.user_id) AS total_pkgs,
               (SELECT COUNT(*) FROM wisemodel_packages p
                WHERE p.user_id = t.user_id
                  AND (p.valid_until IS NULL OR p.valid_until > NOW())) AS active_pkgs
        FROM tokens t
        JOIN users u ON t.user_id = u.id
        WHERE t.name = 'wisemodel-token'
          AND t.status = 1
          AND t.user_id NOT IN (
              SELECT user_id FROM wisemodel_packages
              WHERE valid_until IS NULL OR valid_until > NOW()
          )
        ORDER BY t.user_id;"
    echo ""
    info "处理策略：将这些 token 的 status 改为 2（禁用），避免用户直接收到 403。"
    info "需要后续由 Wisemodel 上游补推 CreateOrder 订单数据后，再重新启用 token。"
    echo ""

    if [ "$EXECUTE" = "--execute" ]; then
        qs "UPDATE tokens
            SET status = 2
            WHERE name = 'wisemodel-token'
              AND status = 1
              AND user_id NOT IN (
                  SELECT user_id FROM wisemodel_packages
                  WHERE valid_until IS NULL OR valid_until > NOW()
              )"

        action "已禁用 ${count} 个无包的 wisemodel-token"
        echo ""
        info "后续步骤："
        info "  1. 联系 Wisemodel 上游重新调用 POST /api/wisemodel/orders/record 补录订单"
        info "  2. 确认订单写入后执行以下 SQL 重新启用 token："
        echo ""
        echo "    UPDATE tokens SET status = 1"
        echo "    WHERE name = 'wisemodel-token'"
        echo "      AND status = 2"
        echo "      AND user_id IN ("
        echo "          SELECT user_id FROM wisemodel_packages"
        echo "          WHERE valid_until IS NULL OR valid_until > NOW()"
        echo "      );"
        echo ""
    else
        dryrun "将禁用 ${count} 个 token（加 --execute 执行）"
    fi
}

# ──────────────────────────────────────────────
# 执行后验证
# ──────────────────────────────────────────────
post_check() {
    echo ""
    echo -e "${CYAN}[ 验证 ] 修复后数据状态${NC}"
    echo ""

    local inverted active_no_pkg
    inverted=$(qs "SELECT COUNT(*) FROM wisemodel_packages
                   WHERE valid_until IS NOT NULL
                     AND UNIX_TIMESTAMP(valid_until) <= UNIX_TIMESTAMP(created_at)")
    active_no_pkg=$(qs "SELECT COUNT(DISTINCT t.id)
                        FROM tokens t
                        WHERE t.name = 'wisemodel-token'
                          AND t.status = 1
                          AND t.user_id NOT IN (
                              SELECT user_id FROM wisemodel_packages
                              WHERE valid_until IS NULL OR valid_until > NOW()
                          )")

    if [ "$inverted" -eq 0 ]; then
        ok "时间窗口倒置记录：0 条"
    else
        fail "仍有 ${inverted} 条时间窗口倒置记录"
    fi

    if [ "$active_no_pkg" -eq 0 ]; then
        ok "启用中的 wisemodel-token 全部有有效资源包"
    else
        fail "仍有 ${active_no_pkg} 个启用的 wisemodel-token 无有效资源包"
    fi
}

# ──────────────────────────────────────────────
# 主流程
# ──────────────────────────────────────────────
main() {
    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║      Wisemodel 问题数据修复脚本                 ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════╝${NC}"
    echo ""

    if [ "$EXECUTE" = "--execute" ]; then
        echo -e "  模式: ${RED}执行模式（将写入数据库）${NC}"
    else
        echo -e "  模式: ${YELLOW}Dry-Run（只预览，不修改）${NC}  加 --execute 参数才会真正修复"
    fi
    echo -e "  数据库: ${DB_HOST}:${DB_PORT}/${DB_NAME}  模式: ${EXEC_MODE}"

    fix_inverted_time_windows
    fix_tokens_without_packages

    if [ "$EXECUTE" = "--execute" ]; then
        post_check
    fi

    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║                    完成                         ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════╝${NC}"
    echo ""
}

main
