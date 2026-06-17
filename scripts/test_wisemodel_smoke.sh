#!/usr/bin/env bash
#
# test_wisemodel_smoke.sh — wisemodel 资源包门控端到端冒烟 + 高并发误判耗尽复现
#
# 复现/验证的核心场景(原 bug):额度充足时,高频/并发访问是否会被门控误判成
# "wisemodel package quota exhausted" 而拒绝(即"高频访问然后无法访问")。
# 单账本 + 原子扣减重构后,健康(大额度)包在任意并发下都不应出现 quota exhausted。
#
#   Test A  大额度包 + 高并发  → 误判耗尽数必须为 0(原 bug 回归)
#   Test B  请求包未覆盖的模型 → 必须被拒(model_not_allowed,需求2)
#   Test C  小额度包持续消费    → 真正耗尽时干净拒绝、不超卖(需求1,需可用上游)
#
# 用法:
#   WISEMODEL_API_TOKEN=<管理令牌> ./scripts/test_wisemodel_smoke.sh
# 可选环境变量(含默认值):
#   BASE_URL=http://localhost:3000   MODEL=minimax-m2.5-highspeed
#   CONCURRENCY=20   REQUESTS=300   BIG_POINTS=1000000000   SMALL_POINTS=50
#   SKIP_DRAIN=1 (只跑高并发主测,跳过 Test C)
#
set -uo pipefail

BASE_URL=${BASE_URL:-http://localhost:3000}
export BASE_URL

# ---- worker 模式(被自身以 xargs 并发再调用,避免依赖 export -f,完全可移植)----
# 单次 /v1 调用并把结果分类为一行:SUCCESS / EXHAUSTED / NOTALLOWED / OTHER:<code>
if [ "${1:-}" = "__worker" ]; then
  key=$2; model=$3
  resp=$(curl -s -m 30 -w $'\n%{http_code}' -X POST "$BASE_URL/v1/chat/completions" \
    -H "Authorization: Bearer $key" -H 'Content-Type: application/json' \
    -d "{\"model\":\"$model\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}],\"max_tokens\":16}")
  code=$(printf '%s' "$resp" | tail -n1)
  body=$(printf '%s' "$resp" | sed '$d')
  if printf '%s' "$body" | grep -qi 'quota exhausted\|wisemodel_quota_exhausted\|no_active_wisemodel_package'; then
    echo EXHAUSTED
  elif printf '%s' "$body" | grep -qi 'model_not_allowed_by_wisemodel_package'; then
    echo NOTALLOWED
  elif [ "$code" = 200 ]; then
    echo SUCCESS
  else
    echo "OTHER:$code"
  fi
  exit 0
fi

# ---- 主流程 ----
MODEL=${MODEL:-minimax-m2.5-highspeed}
CONCURRENCY=${CONCURRENCY:-20}
REQUESTS=${REQUESTS:-300}
BIG_POINTS=${BIG_POINTS:-1000000000}
SMALL_POINTS=${SMALL_POINTS:-50}
ADMIN_TOKEN=${WISEMODEL_API_TOKEN:-}

PHONE_A=13900000001; KEY_A=wisemodel-smokeA
PHONE_C=13900000002; KEY_C=wisemodel-smokeC

# 自身绝对路径(供 xargs 在任意 cwd 下再调用)
SELF=$(cd "$(dirname "$0")" && pwd)/$(basename "$0")

red(){ printf '\033[31m%s\033[0m\n' "$*"; }
grn(){ printf '\033[32m%s\033[0m\n' "$*"; }
ylw(){ printf '\033[33m%s\033[0m\n' "$*"; }
hr(){ printf -- '----------------------------------------------------------\n'; }

command -v curl >/dev/null || { red "需要 curl"; exit 1; }
[ -n "$ADMIN_TOKEN" ] || { red "请设置 WISEMODEL_API_TOKEN (管理端令牌)"; exit 1; }
curl -s -o /dev/null --max-time 5 "$BASE_URL" || { red "无法访问 $BASE_URL,请先启动服务"; exit 1; }
HAVE_JQ=0; command -v jq >/dev/null && HAVE_JQ=1
ADM=(-H "Authorization: Bearer $ADMIN_TOKEN" -H 'Content-Type: application/json')

rfc3339(){ # $1 = +Nd / -Nd
  date -u -v"$1" +%Y-%m-%dT%H:%M:%SZ 2>/dev/null && return
  local s=${1:0:1} n=${1:1:${#1}-2}
  [ "$s" = "-" ] && date -u -d "-$n days" +%Y-%m-%dT%H:%M:%SZ || date -u -d "+$n days" +%Y-%m-%dT%H:%M:%SZ
}
bind_user(){ curl -s "${ADM[@]}" "$BASE_URL/api/wisemodel/user/bind" \
  -d "{\"phone\":\"$1\",\"wisemodel_key\":\"$2\",\"username\":\"$3\"}" >/dev/null; }
create_pkg(){ # phone order pkg points models valid_until
  curl -s "${ADM[@]}" "$BASE_URL/api/wisemodel/orders/record" -d '{
    "order_id":"'"$2"'","package_count":1,
    "packages":[{"id":"'"$3"'","points":'"$4"',"amount":1,"phone":"'"$1"'",
      "model_names":"'"$5"'","valid_until":"'"$6"'","created_at":"'"$(rfc3339 -1d)"'"}]}' >/dev/null; }
remain_of(){ # phone -> echo total remain
  local r; r=$(curl -s "${ADM[@]}" "$BASE_URL/api/wisemodel/user/package_usage" -d "{\"phone\":\"$1\"}")
  if [ "$HAVE_JQ" = 1 ]; then
    printf '%s' "$r" | jq -r '[.data[]? | (.remain_points // .remain_tokens // 0)] | add // 0'
  else
    printf '%s' "$r" | grep -o '"remain_\(points\|tokens\)":[0-9]*' | grep -o '[0-9]*' | paste -sd+ - | bc 2>/dev/null || echo "?"
  fi
}
# 并发跑 N 次,回显分类结果(每行一个)
fan_out(){ seq "$1" | xargs -P "$CONCURRENCY" -I{} "$SELF" __worker "$2" "$3"; }

hr; echo "Setup: 绑定用户 + 订阅资源包 ($BASE_URL, model=$MODEL)"; hr
bind_user "$PHONE_A" "$KEY_A" smoke_a
bind_user "$PHONE_C" "$KEY_C" smoke_c
VU=$(rfc3339 +365d)
create_pkg "$PHONE_A" "ORD-A-$(date +%s)" "PKG-A" "$BIG_POINTS" "$MODEL" "$VU"
echo "  用户A 大额度包: points=$BIG_POINTS, remain=$(remain_of "$PHONE_A")"

# ===== Test A: 高并发误判耗尽复现(原 bug 核心)=====
hr; echo "Test A: 高并发 x$CONCURRENCY,共 $REQUESTS 次 → 大额度包不应出现 quota exhausted"; hr
before=$(remain_of "$PHONE_A")
RES=$(fan_out "$REQUESTS" "$KEY_A" "$MODEL")
after=$(remain_of "$PHONE_A")
a_succ=$(printf '%s\n' "$RES" | grep -c '^SUCCESS$')
a_exh=$( printf '%s\n' "$RES" | grep -c '^EXHAUSTED$')
a_oth=$( printf '%s\n' "$RES" | grep -c '^OTHER')
echo "  SUCCESS=$a_succ  EXHAUSTED=$a_exh  OTHER(上游/其它)=$a_oth   remain: $before → $after"
[ "$a_oth" = "$REQUESTS" ] && ylw "  注意: 全部 OTHER —— 多半上游渠道未配置;门控仍执行(EXHAUSTED=$a_exh 才是关键)。"
VERDICT_A=PASS; [ "$a_exh" -gt 0 ] && VERDICT_A=FAIL

# ===== Test B: 模型未覆盖 → 拒绝 =====
hr; echo "Test B: 请求未覆盖模型 → 应被拒(model_not_allowed)"; hr
b_res=$("$SELF" __worker "$KEY_A" "definitely-not-covered-$(date +%s)")
echo "  结果: $b_res"
VERDICT_B=PASS; [ "$b_res" = "NOTALLOWED" ] || VERDICT_B=FAIL

# ===== Test C: 小额度包真正耗尽(需可用上游)=====
VERDICT_C=SKIP
if [ "${SKIP_DRAIN:-0}" != "1" ]; then
  hr; echo "Test C: 小额度包($SMALL_POINTS)持续消费 → 真正耗尽干净拒绝、不超卖"; hr
  create_pkg "$PHONE_C" "ORD-C-$(date +%s)" "PKG-C" "$SMALL_POINTS" "$MODEL" "$VU"
  warm=$("$SELF" __worker "$KEY_C" "$MODEL")
  if [ "$warm" != "SUCCESS" ]; then
    ylw "  跳过: 上游未产生成功调用(warm-up=$warm),无法驱动真实消费。"
    ylw "  (真正耗尽不超卖已由 go test 的 TestTryDeductPackageRemain_ConcurrentNoOversell 覆盖。)"
  else
    n=$((SMALL_POINTS * 4 + 20))
    cres=$(fan_out "$n" "$KEY_C" "$MODEL")
    c_succ=$(printf '%s\n' "$cres" | grep -c '^SUCCESS$')
    c_exh=$( printf '%s\n' "$cres" | grep -c '^EXHAUSTED$')
    c_after=$(remain_of "$PHONE_C")
    echo "  SUCCESS=$c_succ  EXHAUSTED=$c_exh  remain_after=$c_after"
    VERDICT_C=PASS
    [ "$c_exh" -gt 0 ] || { VERDICT_C=FAIL; red "  期望出现真正耗尽却没有"; }
    case "$c_after" in ''|*[!0-9-]*) ;; -*) VERDICT_C=FAIL; red "  remain 为负=超卖!";; esac
  fi
fi

hr; echo "结果汇总"; hr
printf "  Test A 高并发不误判耗尽 : %s (EXHAUSTED=%s, 期望0)\n" "$VERDICT_A" "$a_exh"
printf "  Test B 模型未覆盖被拒   : %s\n" "$VERDICT_B"
printf "  Test C 真正耗尽不超卖   : %s\n" "$VERDICT_C"
hr
[ "$VERDICT_A" = FAIL ] && { red "❌ 复现到原 bug:额度充足却在高并发下被误判 quota exhausted。"; exit 2; }
[ "$VERDICT_B" = FAIL ] && { red "❌ Test B 失败"; exit 2; }
[ "$VERDICT_C" = FAIL ] && { red "❌ Test C 失败"; exit 2; }
grn "✅ 高并发下额度充足始终可访问,未复现误判耗尽。"
