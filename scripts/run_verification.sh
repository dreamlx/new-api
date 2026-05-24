#!/bin/bash
# run_verification.sh — 破坏性升级功能完整性验证总入口
#
# 运行全部 4 个优先级的测试脚本，汇总最终结果。
#
# 快速开始:
#   cp .env.example .env.test   # 按需修改
#   source .env.test && bash scripts/run_verification.sh
#
# 或直接指定环境变量:
#   BASE_URL=http://localhost:3000 \
#   ADMIN_TOKEN=sk-xxx-admin \
#   RELAY_TOKEN=sk-xxx-relay \
#   RELAY_MODEL=gpt-3.5-turbo \
#   bash scripts/run_verification.sh [p2|p3|p4]   # 可指定只跑某个优先级
#
# 环境变量说明:
#   BASE_URL              服务地址（默认 http://localhost:3000）
#   ADMIN_TOKEN           管理员/Root API Token
#   RELAY_TOKEN           普通用户 Token（sk-xxx 格式，用于发 relay 请求）
#   RELAY_MODEL           测试用模型名（默认 gpt-3.5-turbo）
#   DB_TYPE               数据库类型: mysql | sqlite
#   DB_DSN                MySQL DSN: user:pass@tcp(host:3306)/dbname
#   SQLITE_FILE           SQLite 文件路径（DB_TYPE=sqlite 时使用）

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FILTER="${1:-all}"   # 可传 p1/p2/p3/p4/all

# ── 颜色 ─────────────────────────────────────────────────────────────────────
GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'
BLUE='\033[0;34m'; CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

echo ""
echo -e "${BOLD}${CYAN}╔══════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}${CYAN}║     破坏性升级功能完整性验证 (new-api-0422)          ║${NC}"
echo -e "${BOLD}${CYAN}╚══════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${BOLD}环境配置:${NC}"
echo -e "  BASE_URL:              ${BASE_URL:-http://localhost:3000}"
echo -e "  ADMIN_TOKEN:           ${ADMIN_TOKEN:+${ADMIN_TOKEN:0:8}...（已设置）}${ADMIN_TOKEN:-（未设置 — P2/P3/P4 部分测试将跳过）}"
echo -e "  RELAY_TOKEN:           ${RELAY_TOKEN:+${RELAY_TOKEN:0:8}...（已设置）}${RELAY_TOKEN:-（未设置 — 部分 relay 测试将跳过）}"
echo -e "  RELAY_MODEL:           ${RELAY_MODEL:-gpt-3.5-turbo}"
echo -e "  DB_TYPE:               ${DB_TYPE:-（未配置，DB 验证将跳过）}"
echo -e "  过滤器:                 $FILTER"
echo ""

# 导出所有变量到子进程
export BASE_URL="${BASE_URL:-http://localhost:3000}"
export ADMIN_TOKEN="${ADMIN_TOKEN:-}"
export RELAY_TOKEN="${RELAY_TOKEN:-}"
export RELAY_MODEL="${RELAY_MODEL:-gpt-3.5-turbo}"
export DB_TYPE="${DB_TYPE:-}"
export DB_DSN="${DB_DSN:-}"
export SQLITE_FILE="${SQLITE_FILE:-}"

# ── 运行单个测试脚本，捕获退出码 ──────────────────────────────────────────────
OVERALL_EXIT=0

run_suite() {
    local name="$1"
    local script="$2"
    local label="$3"

    if [ "$FILTER" != "all" ] && [ "$FILTER" != "$name" ]; then
        return
    fi

    echo ""
    echo -e "${BOLD}${BLUE}┌─────────────────────────────────────────────────────┐${NC}"
    echo -e "${BOLD}${BLUE}│  运行: $label${NC}"
    echo -e "${BOLD}${BLUE}└─────────────────────────────────────────────────────┘${NC}"

    set +e
    bash "$SCRIPT_DIR/$script"
    EXIT_CODE=$?
    set -e

    if [ "$EXIT_CODE" -ne 0 ]; then
        OVERALL_EXIT=1
        echo -e "\n${RED}⚠ $label 有测试失败${NC}"
    fi
}

run_suite "p2" "test_p2_regression.sh"   "P2 · 重构区域回归 (#6-10)"
run_suite "p3" "test_p3_new_features.sh" "P3 · 新功能验收 (#11-15)"
run_suite "p4" "test_p4_general.sh"      "P4 · 通用功能回归 (#16-20)"

# ── 总结 ─────────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}${CYAN}╔══════════════════════════════════════════════════════╗${NC}"
echo -e "${BOLD}${CYAN}║              最终验证结果                           ║${NC}"
echo -e "${BOLD}${CYAN}╚══════════════════════════════════════════════════════╝${NC}"
echo ""
echo -e "${YELLOW}需要人工确认的项目（脚本中标注 MANUAL / SKIP）:${NC}"
echo "  • #7  配置真实渠道后测试 Kling/Jimeng/Vidu 视频任务"
echo "  • #8  配置 MJ 渠道后完成一次任务，验证计费日志"
echo "  • #9  浏览器验证 GitHub/Discord/LinuxDO OAuth 完整登录流程"
echo "  • #12 浏览器验证自定义 OAuth 完整登录流程"
echo "  • #14 在管理后台配置 Waffo Key 后，验证充值页面入口"
echo "  • #18 使用生物识别设备验证 Passkey 注册和登录"
echo "  • #20 在日志中确认 Pyroscope 已挂载（如已配置）"
echo ""

if [ "$OVERALL_EXIT" -eq 0 ]; then
    echo -e "${GREEN}✅ 所有自动化测试通过！请人工完成上述 MANUAL 项目后签字确认。${NC}"
else
    echo -e "${RED}❌ 有自动化测试失败，请先修复上述问题再进行发布。${NC}"
fi

exit $OVERALL_EXIT
