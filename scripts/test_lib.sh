#!/bin/bash
# test_lib.sh — 共享工具函数库，由各测试脚本 source 引入
# 用法: source "$(dirname "$0")/test_lib.sh"

# ── 颜色 ─────────────────────────────────────────────────────────────────────
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# ── 全局计数 ──────────────────────────────────────────────────────────────────
TOTAL=0; PASSED=0; FAILED=0; SKIPPED=0
declare -a FAIL_MSGS=()
declare -a SKIP_MSGS=()

# ── 输出函数 ──────────────────────────────────────────────────────────────────
pass()    { echo -e "  ${GREEN}✓ PASS${NC} $1"; PASSED=$((PASSED+1)); TOTAL=$((TOTAL+1)); }
fail()    { echo -e "  ${RED}✗ FAIL${NC} $1"; FAILED=$((FAILED+1)); TOTAL=$((TOTAL+1)); FAIL_MSGS+=("$1"); }
skip()    { echo -e "  ${YELLOW}⊘ SKIP${NC} $1"; SKIPPED=$((SKIPPED+1)); SKIP_MSGS+=("$1"); }
info()    { echo -e "  ${BLUE}ℹ${NC} $1"; }
warn()    { echo -e "  ${YELLOW}⚠${NC} $1"; }
section() { echo -e "\n${BOLD}${CYAN}▌ $1${NC}"; }
manual()  {
    echo -e "  ${YELLOW}✋ MANUAL${NC} $1"
    echo -e "     ${YELLOW}→ $2${NC}"
    SKIP_MSGS+=("[MANUAL] $1")
}

# ── 环境变量（可被调用方覆盖） ─────────────────────────────────────────────────
BASE_URL="${BASE_URL:-http://localhost:3000}"
ADMIN_TOKEN="${ADMIN_TOKEN:-}"
RELAY_TOKEN="${RELAY_TOKEN:-}"              # 用于中继请求的 sk-xxx Token
RELAY_MODEL="${RELAY_MODEL:-gpt-3.5-turbo}"

# ── 检查依赖 ──────────────────────────────────────────────────────────────────
check_deps() {
    for cmd in curl jq; do
        if ! command -v "$cmd" &>/dev/null; then
            echo -e "${RED}缺少依赖: $cmd，请先安装${NC}"
            exit 1
        fi
    done
}

# ── 检查服务存活 ──────────────────────────────────────────────────────────────
check_service() {
    info "检查服务状态: $BASE_URL"
    if curl -sf "$BASE_URL/api/status" >/dev/null 2>&1; then
        pass "服务运行正常"
    else
        echo -e "${RED}服务未响应，请先启动服务再运行测试${NC}"
        exit 1
    fi
}

# ── HTTP 请求封装 ─────────────────────────────────────────────────────────────
# 返回值: 全局变量 HTTP_STATUS / HTTP_BODY
http_get() {
    local path="$1"; local auth="${2:-}"
    local resp
    resp=$(curl -s -w '\n__STATUS__%{http_code}' \
        ${auth:+-H "Authorization: Bearer $auth"} \
        "$BASE_URL$path" 2>&1)
    HTTP_BODY=$(echo "$resp" | sed '$d')
    HTTP_STATUS=$(echo "$resp" | tail -n1 | sed 's/__STATUS__//')
}

http_post() {
    local path="$1"; local body="$2"; local auth="${3:-}"
    local resp
    resp=$(curl -s -w '\n__STATUS__%{http_code}' \
        -X POST \
        ${auth:+-H "Authorization: Bearer $auth"} \
        -H "Content-Type: application/json" \
        -d "$body" \
        "$BASE_URL$path" 2>&1)
    HTTP_BODY=$(echo "$resp" | sed '$d')
    HTTP_STATUS=$(echo "$resp" | tail -n1 | sed 's/__STATUS__//')
}

# ── jq 字段断言 ───────────────────────────────────────────────────────────────
# assert_jq <json> <jq_path> <expected> <test_name>
assert_jq() {
    local json="$1" path="$2" expected="$3" name="$4"
    local actual
    actual=$(echo "$json" | jq -r "$path" 2>/dev/null)
    if [ "$actual" = "$expected" ]; then
        pass "$name"
    else
        fail "$name (期望='$expected', 实际='$actual')"
        info "原始响应: $(echo "$json" | head -c 300)"
    fi
}

# assert_jq_exists <json> <jq_path> <test_name>
assert_jq_exists() {
    local json="$1" path="$2" name="$3"
    if echo "$json" | jq -e "$path" >/dev/null 2>&1; then
        pass "$name"
    else
        fail "$name (路径 '$path' 不存在)"
        info "原始响应: $(echo "$json" | head -c 300)"
    fi
}

# assert_jq_not_null <json> <jq_path> <test_name>
assert_jq_not_null() {
    local json="$1" path="$2" name="$3"
    local val
    val=$(echo "$json" | jq -r "$path" 2>/dev/null)
    if [ "$val" != "null" ] && [ "$val" != "" ]; then
        pass "$name (值='$val')"
    else
        fail "$name (路径 '$path' 为 null 或空)"
        info "原始响应: $(echo "$json" | head -c 300)"
    fi
}

# ── DB 查询辅助（MySQL/SQLite 通用） ─────────────────────────────────────────
# 用法: db_query "SELECT ..."
# 依赖环境变量: DB_TYPE(mysql|sqlite), DB_DSN 或 SQLITE_FILE
db_query() {
    local sql="$1"
    if [ "${DB_TYPE:-}" = "mysql" ] && [ -n "${DB_DSN:-}" ]; then
        local host user pass dbname port
        # 解析 DSN: user:pass@tcp(host:port)/dbname
        user="${DB_DSN%%:*}"
        pass="${DB_DSN#*:}"; pass="${pass%%@*}"
        local hostpart="${DB_DSN#*tcp(}"; hostpart="${hostpart%%)*}"
        host="${hostpart%%:*}"; port="${hostpart##*:}"
        dbname="${DB_DSN##*/}"; dbname="${dbname%%\?*}"
        mysql -h"$host" -P"$port" -u"$user" -p"$pass" "$dbname" -e "$sql" 2>/dev/null
    elif [ "${DB_TYPE:-}" = "sqlite" ] && [ -n "${SQLITE_FILE:-}" ]; then
        sqlite3 "${SQLITE_FILE}" "$sql" 2>/dev/null
    else
        echo "[DB_SKIP] 未配置 DB_TYPE/DB_DSN/SQLITE_FILE，跳过数据库查询"
    fi
}

# ── 最终汇总 ──────────────────────────────────────────────────────────────────
print_summary() {
    local title="${1:-测试结果汇总}"
    echo ""
    echo -e "${BOLD}════════════════════════════════════════${NC}"
    echo -e "${BOLD} $title${NC}"
    echo -e "${BOLD}════════════════════════════════════════${NC}"
    echo -e "  总计: ${TOTAL}  ${GREEN}通过: ${PASSED}${NC}  ${RED}失败: ${FAILED}${NC}  ${YELLOW}跳过: ${SKIPPED}${NC}"

    if [ ${#FAIL_MSGS[@]} -gt 0 ]; then
        echo -e "\n${RED}失败项目:${NC}"
        for m in "${FAIL_MSGS[@]}"; do echo -e "  ${RED}✗${NC} $m"; done
    fi
    if [ ${#SKIP_MSGS[@]} -gt 0 ]; then
        echo -e "\n${YELLOW}需人工确认:${NC}"
        for m in "${SKIP_MSGS[@]}"; do echo -e "  ${YELLOW}⊘${NC} $m"; done
    fi
    echo ""

    if [ "$FAILED" -eq 0 ]; then
        echo -e "${GREEN}✅ 全部自动化用例通过${NC}"
    else
        echo -e "${RED}❌ 有 $FAILED 个用例失败，请检查${NC}"
        return 1
    fi
}
