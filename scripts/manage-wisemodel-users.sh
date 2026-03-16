#!/bin/bash
# ============================================================
# WiseModel 用户管理脚本
# 功能：1) 查询并展示 WiseModel 用户信息
#       2) 删除 WiseModel 用户（含关联 Token）
# 用法：./scripts/manage-wisemodel-users.sh [list|delete] [选项]
# ============================================================

set -euo pipefail

# ── 颜色 ──────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# ── 数据库配置（读取 .env.dev，允许环境变量覆盖）─────────
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="$SCRIPT_DIR/../.env.dev"

if [[ -f "$ENV_FILE" ]]; then
    # 解析 SQL_DSN=user:password@tcp(host:port)/dbname
    SQL_DSN=$(grep '^SQL_DSN=' "$ENV_FILE" | cut -d= -f2-)
fi

SQL_DSN="${SQL_DSN:-root:dev123456@tcp(localhost:3307)/new_api_dev}"

# 解析 DSN
DB_USER=$(echo "$SQL_DSN" | sed -E 's|^([^:]+):.*|\1|')
DB_PASS=$(echo "$SQL_DSN" | sed -E 's|^[^:]+:([^@]+)@.*|\1|')
DB_HOST=$(echo "$SQL_DSN" | sed -E 's|.*@tcp\(([^:)]+):.*|\1|')
DB_PORT=$(echo "$SQL_DSN" | sed -E 's|.*@tcp\([^:]+:([0-9]+)\).*|\1|')
DB_NAME=$(echo "$SQL_DSN" | sed -E 's|.*/([^/]+)$|\1|')

# Docker 容器名（用于 docker exec 模式）
MYSQL_CONTAINER="${MYSQL_CONTAINER:-mysql-dev}"

# mysql 执行方式：优先 docker exec，其次本地 mysql 客户端
_mysql_exec() {
    if [[ "$USE_DOCKER" == "true" ]]; then
        docker exec -i "$MYSQL_CONTAINER" \
            mysql -u"$DB_USER" -p"$DB_PASS" "$DB_NAME" \
            --default-character-set=utf8mb4 -s "$@"
    else
        mysql -h"$DB_HOST" -P"$DB_PORT" -u"$DB_USER" -p"$DB_PASS" "$DB_NAME" \
            --default-character-set=utf8mb4 -s "$@"
    fi
}

# ── 工具函数 ──────────────────────────────────────────────
log_info()    { echo -e "${BLUE}[INFO]${NC}  $*"; }
log_ok()      { echo -e "${GREEN}[OK]${NC}    $*"; }
log_warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
log_err()     { echo -e "${RED}[ERROR]${NC} $*" >&2; }
log_section() { echo -e "\n${BOLD}${CYAN}══ $* ══${NC}"; }

quota_to_usd() {
    # quota / 500000 = USD
    local q=$1
    printf "%.4f" "$(echo "scale=6; $q / 500000" | bc)"
}

check_mysql() {
    # 优先尝试 docker exec（容器内已有 mysql 客户端，无需本地安装）
    if command -v docker &>/dev/null && docker ps --format '{{.Names}}' 2>/dev/null | grep -q "^${MYSQL_CONTAINER}$"; then
        USE_DOCKER=true
        log_info "使用 Docker 容器 [$MYSQL_CONTAINER] 连接数据库"
        if ! _mysql_exec -e "SELECT 1" &>/dev/null; then
            log_err "docker exec 连接数据库失败，请检查容器 $MYSQL_CONTAINER 状态"
            exit 1
        fi
        return
    fi

    # 回退到本地 mysql 客户端
    USE_DOCKER=false
    if ! command -v mysql &>/dev/null; then
        log_err "未找到 mysql 客户端，且未检测到运行中的 Docker 容器 [$MYSQL_CONTAINER]"
        log_err "请选择以下任意一种方式："
        log_err "  1) 启动 MySQL 容器：docker compose -f docker-compose.db-only.yml up -d"
        log_err "  2) 安装本地客户端（macOS）：brew install mysql-client"
        log_err "  3) 安装本地客户端（Ubuntu/Debian）：apt-get install -y mysql-client"
        log_err "  4) 安装本地客户端（CentOS/RHEL）：yum install -y mysql"
        log_err "  5) 指定容器名：MYSQL_CONTAINER=<容器名> $0 list"
        exit 1
    fi
    if ! _mysql_exec -e "SELECT 1" &>/dev/null; then
        log_err "无法连接数据库 $DB_HOST:$DB_PORT/$DB_NAME，请检查配置"
        exit 1
    fi
}

run_sql() {
    _mysql_exec -e "$1" 2>/dev/null
}

run_sql_raw() {
    _mysql_exec --raw --batch -e "$1" 2>/dev/null
}

# ── 功能 1：查询并展示 WiseModel 用户 ─────────────────────
cmd_list() {
    local filter_phone="${1:-}"

    log_section "WiseModel 用户列表"

    # 构建查询条件
    local where="WHERE u.wisemodel_key != '' AND u.wisemodel_key IS NOT NULL"
    if [[ -n "$filter_phone" ]]; then
        where="$where AND u.phone = '$filter_phone'"
    fi

    # 用户汇总
    local total
    total=$(run_sql_raw "SELECT COUNT(*) FROM users u $where;" | tail -1)
    echo -e "  共找到 ${BOLD}${total}${NC} 个 WiseModel 用户\n"

    if [[ "$total" -eq 0 ]]; then
        log_warn "没有找到 WiseModel 用户"
        return
    fi

    # 表头
    printf "${BOLD}%-6s %-20s %-16s %-25s %-12s %-10s %-10s %-20s${NC}\n" \
        "ID" "用户名" "手机号" "WisemodelKey(前20)" "Quota" "USD余额" "状态" "创建时间"
    printf '%s\n' "$(printf '─%.0s' {1..115})"

    # 用户数据（逐行处理避免IFS问题）
    while IFS=$'\t' read -r id username phone wm_key quota status created_at; do
        local status_label
        case "$status" in
            1) status_label="${GREEN}启用${NC}" ;;
            2) status_label="${RED}禁用${NC}" ;;
            *) status_label="${YELLOW}未知${NC}" ;;
        esac

        local usd
        usd=$(quota_to_usd "$quota")

        local wm_short="${wm_key:0:20}..."
        local ts
        ts=$(date -r "$created_at" "+%Y-%m-%d %H:%M" 2>/dev/null || \
             date -d "@$created_at" "+%Y-%m-%d %H:%M" 2>/dev/null || \
             echo "$created_at")

        printf "%-6s %-20s %-16s %-25s %-12s \$%-9s " \
            "$id" "${username:0:20}" "${phone:0:16}" "$wm_short" "$quota" "$usd"
        echo -e "$status_label  $ts"

    done < <(run_sql_raw "
        SELECT u.id, u.username, COALESCE(u.phone,''), u.wisemodel_key,
               u.quota, u.status, u.created_time
        FROM users u
        $where
        ORDER BY u.id;
    " | tail -n +1)

    printf '%s\n' "$(printf '─%.0s' {1..115})"

    # 关联 Token 汇总
    log_section "关联 WiseModel Token 汇总"
    printf "${BOLD}%-6s %-20s %-16s %-14s %-12s %-12s${NC}\n" \
        "UserID" "用户名" "手机号" "Token状态" "已用Quota" "Token名"
    printf '%s\n' "$(printf '─%.0s' {1..85})"

    while IFS=$'\t' read -r uid uname phone tk_status used_quota tk_name; do
        local tk_label
        case "$tk_status" in
            1) tk_label="${GREEN}启用${NC}" ;;
            2) tk_label="${RED}禁用${NC}" ;;
            *) tk_label="${YELLOW}未知${NC}" ;;
        esac
        printf "%-6s %-20s %-16s " "$uid" "${uname:0:20}" "${phone:0:16}"
        echo -e "$tk_label            %-12s %-12s" "$used_quota" "$tk_name" 2>/dev/null || \
        printf "               %-12s %-12s\n" "$used_quota" "$tk_name"
    done < <(run_sql_raw "
        SELECT u.id, u.username, COALESCE(u.phone,''), t.status, t.used_quota, t.name
        FROM users u
        JOIN tokens t ON t.user_id = u.id
        $where
        ORDER BY u.id;
    " | tail -n +1)

    printf '%s\n' "$(printf '─%.0s' {1..85})"
}

# ── 功能 2：删除 WiseModel 用户 ───────────────────────────
cmd_delete() {
    local mode="${1:-interactive}"   # interactive | all | phone:<phone> | id:<id>
    local dry_run="${DRY_RUN:-false}"

    log_section "删除 WiseModel 用户"

    # 构建目标条件
    local where="WHERE u.wisemodel_key != '' AND u.wisemodel_key IS NOT NULL"
    local desc="所有 WiseModel 用户"

    case "$mode" in
        all)
            desc="所有 WiseModel 用户"
            ;;
        phone:*)
            local phone="${mode#phone:}"
            where="$where AND u.phone = '$phone'"
            desc="手机号为 $phone 的 WiseModel 用户"
            ;;
        id:*)
            local uid="${mode#id:}"
            where="$where AND u.id = '$uid'"
            desc="ID=$uid 的 WiseModel 用户"
            ;;
        interactive)
            # 交互式：列出用户让用户选择
            echo ""
            local rows
            rows=$(run_sql_raw "
                SELECT u.id, u.username, COALESCE(u.phone,''), u.wisemodel_key, u.quota
                FROM users u
                $where ORDER BY u.id;
            ")

            if [[ -z "$rows" ]]; then
                log_warn "没有找到 WiseModel 用户，无需删除"
                return
            fi

            echo -e "${BOLD}可删除的 WiseModel 用户：${NC}"
            printf "${BOLD}%-6s %-20s %-16s %-25s %-12s${NC}\n" \
                "ID" "用户名" "手机号" "WisemodelKey(前20)" "Quota"
            printf '%s\n' "$(printf '─%.0s' {1..85})"

            while IFS=$'\t' read -r id uname phone wm_key quota; do
                printf "%-6s %-20s %-16s %-25s %-12s\n" \
                    "$id" "${uname:0:20}" "${phone:0:16}" "${wm_key:0:20}..." "$quota"
            done <<< "$rows"
            printf '%s\n' "$(printf '─%.0s' {1..85})"

            echo ""
            echo "请选择删除方式："
            echo "  [a] 删除全部 WiseModel 用户"
            echo "  [p] 按手机号删除（输入手机号）"
            echo "  [i] 按用户ID删除（输入ID）"
            echo "  [q] 取消退出"
            echo ""
            read -rp "请输入选项: " choice

            case "$choice" in
                a|A)
                    mode="all"
                    desc="所有 WiseModel 用户"
                    ;;
                p|P)
                    read -rp "请输入手机号: " phone_input
                    mode="phone:$phone_input"
                    where="WHERE u.wisemodel_key != '' AND u.wisemodel_key IS NOT NULL AND u.phone = '$phone_input'"
                    desc="手机号为 $phone_input 的 WiseModel 用户"
                    ;;
                i|I)
                    read -rp "请输入用户ID（多个用逗号分隔）: " id_input
                    mode="id:$id_input"
                    where="WHERE u.wisemodel_key != '' AND u.wisemodel_key IS NOT NULL AND u.id IN ($id_input)"
                    desc="ID IN ($id_input) 的 WiseModel 用户"
                    ;;
                q|Q|*)
                    log_info "已取消操作"
                    return
                    ;;
            esac
            ;;
        *)
            log_err "未知模式: $mode"
            show_usage
            exit 1
            ;;
    esac

    # 预览将删除的记录
    local target_ids
    target_ids=$(run_sql_raw "SELECT u.id FROM users u $where;" | tr '\n' ',' | sed 's/,$//')

    if [[ -z "$target_ids" ]]; then
        log_warn "没有找到匹配的用户，操作结束"
        return
    fi

    local count
    count=$(run_sql_raw "SELECT COUNT(*) FROM users u $where;" | tail -1)

    log_warn "即将删除 ${BOLD}${count}${NC} 个用户（$desc）"
    log_warn "用户ID列表: $target_ids"

    # 关联 Token 预览
    local token_count
    token_count=$(run_sql_raw "SELECT COUNT(*) FROM tokens WHERE user_id IN ($target_ids);" | tail -1)
    log_warn "关联 Token 数量: $token_count 条（将一并删除）"

    if [[ "$dry_run" == "true" ]]; then
        log_info "[DRY RUN] 实际不会执行删除，以上为预览"
        return
    fi

    echo ""
    read -rp "$(echo -e "${RED}确认删除？此操作不可恢复！[yes/N] ${NC}")" confirm
    if [[ "$confirm" != "yes" ]]; then
        log_info "已取消操作"
        return
    fi

    # 执行删除（先 Token 后 User，避免外键问题）
    log_info "正在删除关联 Token..."
    run_sql "DELETE FROM tokens WHERE user_id IN ($target_ids);"
    log_ok "Token 删除完成"

    log_info "正在删除用户记录..."
    run_sql "DELETE FROM users WHERE id IN ($target_ids);"
    log_ok "用户删除完成"

    echo ""
    log_ok "已成功删除 ${count} 个 WiseModel 用户及 ${token_count} 条关联 Token"
}

# ── 帮助 ──────────────────────────────────────────────────
show_usage() {
    echo ""
    echo -e "${BOLD}用法:${NC}"
    echo "  $0 list                    # 列出所有 WiseModel 用户"
    echo "  $0 list <手机号>           # 列出指定手机号的用户"
    echo "  $0 delete                  # 交互式删除"
    echo "  $0 delete all              # 删除全部 WiseModel 用户"
    echo "  $0 delete phone:<手机号>   # 按手机号删除"
    echo "  $0 delete id:<用户ID>      # 按ID删除（多个逗号分隔）"
    echo ""
    echo -e "${BOLD}环境变量:${NC}"
    echo "  SQL_DSN    数据库连接串（默认读取 .env.dev）"
    echo "  DRY_RUN=true  预览模式，不实际删除"
    echo ""
}

# ── 主入口 ────────────────────────────────────────────────
main() {
    check_mysql

    local cmd="${1:-}"
    shift || true

    case "$cmd" in
        list)
            cmd_list "${1:-}"
            ;;
        delete)
            cmd_delete "${1:-interactive}"
            ;;
        -h|--help|help|"")
            show_usage
            ;;
        *)
            log_err "未知命令: $cmd"
            show_usage
            exit 1
            ;;
    esac
}

main "$@"
