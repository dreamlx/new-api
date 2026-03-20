#!/bin/bash

# Wisemodel 消费明细缺失诊断脚本
# 自动判断原因，只输出结论，不做任何修复

set -euo pipefail

ENV_FILE="${ENV_FILE:-.env.dev}"
MYSQL_CONTAINER="${MYSQL_CONTAINER:-mysql-dev}"

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

ok()   { echo -e "  ${GREEN}[OK]${NC}   $1"; }
fail() { echo -e "  ${RED}[!!]${NC}   $1"; FOUND_ISSUES=$((FOUND_ISSUES + 1)); }
warn() { echo -e "  ${YELLOW}[??]${NC}   $1"; }
info() { echo -e "  ${BLUE}--${NC}     $1"; }

FOUND_ISSUES=0

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
    # 静默执行 SQL，返回单个值
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
    # 带表头打印（用于展示详情）
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
# 诊断
# ──────────────────────────────────────────────

main() {
    echo ""
    echo -e "${BOLD}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║     Wisemodel 消费明细缺失 · 诊断报告           ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════╝${NC}"
    echo ""
    echo -e "  数据库: ${DB_HOST}:${DB_PORT}/${DB_NAME}  模式: ${EXEC_MODE}"
    echo ""

    # ── 前置：确认有 wisemodel 用户 ──────────────────
    echo -e "${CYAN}[ 前置检查 ]${NC}"
    user_cnt=$(qs "SELECT COUNT(*) FROM users WHERE wisemodel_key != ''")
    if [ "$user_cnt" -eq 0 ]; then
        fail "没有任何 wisemodel 用户，请先调用 /api/wisemodel/user/bind"
        echo ""; echo -e "  ${RED}诊断终止：无用户数据${NC}"; exit 1
    fi
    ok "wisemodel 用户数: ${user_cnt}"

    pkg_cnt=$(qs "SELECT COUNT(*) FROM wisemodel_packages")
    if [ "$pkg_cnt" -eq 0 ]; then
        fail "没有任何资源包，请先调用 /api/wisemodel/orders/record"
        echo ""; echo -e "  ${RED}诊断终止：无资源包数据${NC}"; exit 1
    fi
    ok "资源包总数: ${pkg_cnt}"

    echo ""

    # ── 原因① 时间窗口倒置 ───────────────────────────
    echo -e "${CYAN}[ 原因① ] valid_until <= created_at（时间窗口不合法）${NC}"
    bad_window=$(qs "SELECT COUNT(*) FROM wisemodel_packages
                     WHERE valid_until IS NOT NULL
                       AND UNIX_TIMESTAMP(valid_until) <= UNIX_TIMESTAMP(created_at)")
    if [ "$bad_window" -gt 0 ]; then
        fail "有 ${bad_window} 个资源包的 valid_until <= created_at，时间窗口倒置，永远查不到消费日志"
        qt "SELECT package_id,
                   created_at,
                   valid_until,
                   TIMESTAMPDIFF(DAY, valid_until, created_at) AS days_inverted
            FROM wisemodel_packages
            WHERE valid_until IS NOT NULL
              AND UNIX_TIMESTAMP(valid_until) <= UNIX_TIMESTAMP(created_at)
            LIMIT 10;"
    else
        ok "所有资源包时间窗口合法（valid_until > created_at）"
    fi

    expired=$(qs "SELECT COUNT(*) FROM wisemodel_packages
                  WHERE valid_until IS NOT NULL AND valid_until < NOW()")
    [ "$expired" -gt 0 ] && warn "有 ${expired} 个资源包已过期（valid_until < 当前时间），过期后的消费不计入该包" \
                          || ok  "无已过期资源包"
    echo ""

    # ── 原因② 消费日志未写入 ──────────────────────────
    echo -e "${CYAN}[ 原因② ] chat 调用未产生消费日志（logs 表无记录）${NC}"
    log_cnt=$(qs "SELECT COUNT(*) FROM logs
                  WHERE user_id IN (SELECT id FROM users WHERE wisemodel_key != '')
                    AND type = 2")
    if [ "$log_cnt" -eq 0 ]; then
        fail "logs 表中无任何 wisemodel 用户的消费记录（type=2），chat 调用未产生计费"
        # 检查充值日志是否存在
        topup_cnt=$(qs "SELECT COUNT(*) FROM logs
                        WHERE user_id IN (SELECT id FROM users WHERE wisemodel_key != '')
                          AND type = 1")
        [ "$topup_cnt" -gt 0 ] && info "充值日志（type=1）存在 ${topup_cnt} 条，说明用户系统正常，问题在 chat relay 层" \
                                || info "充值日志也为 0，请检查 /api/wisemodel/orders/record 是否成功执行"
    else
        ok "消费日志存在，共 ${log_cnt} 条"
        # 最近几条
        info "最近 5 条消费日志："
        qt "SELECT user_id, FROM_UNIXTIME(created_at) AS time, model_name, quota
            FROM logs
            WHERE user_id IN (SELECT id FROM users WHERE wisemodel_key != '')
              AND type = 2
            ORDER BY id DESC LIMIT 5;"
    fi
    echo ""

    # ── 原因③ 模型名不匹配 ───────────────────────────
    echo -e "${CYAN}[ 原因③ ] available_models 限制导致消费被过滤${NC}"
    pkg_with_model_limit=$(qs "SELECT COUNT(*) FROM wisemodel_packages WHERE available_models != ''")
    if [ "$pkg_with_model_limit" -eq 0 ]; then
        ok "所有资源包未设置 available_models 限制，不受模型过滤影响"
    else
        info "有 ${pkg_with_model_limit} 个资源包设置了模型限制"
        # 检查实际使用的模型是否在限制列表内
        mismatch=$(qs "SELECT COUNT(DISTINCT l.id)
                       FROM logs l
                       JOIN wisemodel_packages p ON p.user_id = l.user_id
                       WHERE l.user_id IN (SELECT id FROM users WHERE wisemodel_key != '')
                         AND l.type = 2
                         AND p.available_models != ''
                         AND NOT FIND_IN_SET(l.model_name,
                               REPLACE(REPLACE(p.available_models, ', ', ','), ' ', ','))")
        if [ "$mismatch" -gt 0 ]; then
            fail "有 ${mismatch} 条消费日志的模型名不在资源包的 available_models 中，被过滤掉"
            info "资源包配置的模型 vs 实际使用的模型："
            qt "SELECT p.package_id, p.available_models AS pkg_models,
                       GROUP_CONCAT(DISTINCT l.model_name) AS actual_models
                FROM wisemodel_packages p
                JOIN logs l ON l.user_id = p.user_id AND l.type = 2
                WHERE p.available_models != ''
                GROUP BY p.package_id, p.available_models
                LIMIT 10;"
        else
            ok "消费日志中使用的模型均在 available_models 范围内"
        fi
    fi
    echo ""

    # ── 原因④ user_id 不一致 ─────────────────────────
    echo -e "${CYAN}[ 原因④ ] wisemodel token 的 user_id 与资源包 user_id 不一致${NC}"
    token_no_pkg=$(qs "SELECT COUNT(DISTINCT t.user_id)
                       FROM tokens t
                       WHERE t.name = 'wisemodel-token'
                         AND t.status = 1
                         AND t.user_id NOT IN (SELECT user_id FROM wisemodel_packages)")
    if [ "$token_no_pkg" -gt 0 ]; then
        fail "有 ${token_no_pkg} 个启用中的 wisemodel token，其 user_id 在 wisemodel_packages 中无对应资源包"
        qt "SELECT t.id, t.user_id, u.phone, u.wisemodel_key, t.status
            FROM tokens t
            JOIN users u ON t.user_id = u.id
            WHERE t.name = 'wisemodel-token'
              AND t.user_id NOT IN (SELECT user_id FROM wisemodel_packages)
            LIMIT 10;"
    else
        ok "所有启用中的 wisemodel token 均有对应资源包"
    fi
    echo ""

    # ── 原因⑤ 消费时间不在包的有效窗口内 ─────────────
    echo -e "${CYAN}[ 原因⑤ ] 消费日志时间不在资源包有效窗口内${NC}"
    if [ "$log_cnt" -gt 0 ]; then
        matched=$(qs "SELECT COUNT(DISTINCT l.id)
                      FROM logs l
                      JOIN wisemodel_packages p ON p.user_id = l.user_id
                      WHERE l.user_id IN (SELECT id FROM users WHERE wisemodel_key != '')
                        AND l.type = 2
                        AND l.created_at >= UNIX_TIMESTAMP(p.created_at)
                        AND (p.valid_until IS NULL OR l.created_at < UNIX_TIMESTAMP(p.valid_until))
                        AND (p.available_models = ''
                             OR FIND_IN_SET(l.model_name,
                                  REPLACE(REPLACE(p.available_models, ', ', ','), ' ', ',')))")
        total_logs=$(qs "SELECT COUNT(*) FROM logs
                         WHERE user_id IN (SELECT id FROM users WHERE wisemodel_key != '')
                           AND type = 2")
        if [ "$matched" -eq 0 ]; then
            fail "消费日志共 ${total_logs} 条，但 0 条落在任何资源包的有效时间窗口内"
        elif [ "$matched" -lt "$total_logs" ]; then
            warn "消费日志共 ${total_logs} 条，仅 ${matched} 条落在有效时间窗口内，其余在窗口外不被统计"
        else
            ok "全部 ${total_logs} 条消费日志均在有效时间窗口内"
        fi
    else
        warn "无消费日志，跳过时间窗口匹配检查"
    fi
    echo ""

    # ── 汇总 ─────────────────────────────────────────
    echo -e "${BOLD}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║                  诊断结论                       ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════╝${NC}"
    echo ""
    if [ "$FOUND_ISSUES" -eq 0 ]; then
        echo -e "  ${GREEN}未发现明确问题。${NC}"
        echo -e "  消费数据应能正常返回，请检查 package_usage 接口调用参数是否正确。"
    else
        echo -e "  ${RED}发现 ${FOUND_ISSUES} 个问题（见上方 [!!] 标记）${NC}"
        echo -e "  请逐一排查上方标记为 [!!] 的原因。"
    fi
    echo ""
}

main
