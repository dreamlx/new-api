#!/usr/bin/env bash
#
# hammer_wisemodel_token.sh — 用一个【已有的 wisemodel 用户 token】高并发压测门控,
# 复现/验证"额度充足却被误判 quota exhausted"(高频访问然后无法访问)。
#
# 不需要管理令牌、不造任何数据,只对 /v1/chat/completions 发并发请求并分类统计。
#
# 用法:
#   TOKEN=wisemodel-xxxx MODEL=minimax-m2 BASE_URL=http://localhost:3000 \
#     ./scripts/hammer_wisemodel_token.sh
# 可选:CONCURRENCY=50  REQUESTS=1000
#
# 判定:若你确信该 token 的包【还有额度】,出现任何 EXHAUSTED 即原 bug 复现。
#
set -uo pipefail

BASE_URL=${BASE_URL:-http://localhost:3000}
export BASE_URL

# ---- self-re-exec worker:单次调用 → SUCCESS/EXHAUSTED/NOTALLOWED/UNAUTH/OTHER:<code> ----
if [ "${1:-}" = "__worker" ]; then
  token=$2; model=$3
  resp=$(curl -s -m 30 -w $'\n%{http_code}' -X POST "$BASE_URL/v1/chat/completions" \
    -H "Authorization: Bearer $token" -H 'Content-Type: application/json' \
    -d "{\"model\":\"$model\",\"messages\":[{\"role\":\"user\",\"content\":\"hi\"}],\"max_tokens\":16}")
  code=$(printf '%s' "$resp" | tail -n1)
  body=$(printf '%s' "$resp" | sed '$d')
  if printf '%s' "$body" | grep -qi 'quota exhausted\|wisemodel_quota_exhausted\|no_active_wisemodel_package'; then
    echo EXHAUSTED
  elif printf '%s' "$body" | grep -qi 'model_not_allowed_by_wisemodel_package'; then
    echo NOTALLOWED
  elif [ "$code" = 401 ]; then
    echo UNAUTH
  elif [ "$code" = 200 ]; then
    echo SUCCESS
  else
    echo "OTHER:$code"
  fi
  exit 0
fi

TOKEN=${TOKEN:-}
MODEL=${MODEL:-minimax-m2}
CONCURRENCY=${CONCURRENCY:-50}
REQUESTS=${REQUESTS:-1000}
SELF=$(cd "$(dirname "$0")" && pwd)/$(basename "$0")

red(){ printf '\033[31m%s\033[0m\n' "$*"; }
grn(){ printf '\033[32m%s\033[0m\n' "$*"; }
ylw(){ printf '\033[33m%s\033[0m\n' "$*"; }

command -v curl >/dev/null || { red "需要 curl"; exit 1; }
[ -n "$TOKEN" ] || { red "请设置 TOKEN=<wisemodel 用户 token>(/v1 用的那个,如 wisemodel-xxxx)"; exit 1; }
curl -s -o /dev/null --max-time 5 "$BASE_URL" || { red "无法访问 $BASE_URL"; exit 1; }

echo "压测: $REQUESTS 次 / 并发 $CONCURRENCY,token=$TOKEN,model=$MODEL,base=$BASE_URL"

# 预检一次,先把明显的配置问题暴露出来
warm=$("$SELF" __worker "$TOKEN" "$MODEL")
case "$warm" in
  UNAUTH)     red "预检 401:该 token 在 $BASE_URL 对应的库里不存在或已禁用。换 token 或把 BASE_URL 指向该 token 所在的部署。"; exit 1;;
  NOTALLOWED) red "预检 model_not_allowed:该 token 的包不覆盖模型 '$MODEL'。换成包覆盖的模型(如 minimax-m2)。"; exit 1;;
  EXHAUSTED)  ylw "预检即 EXHAUSTED:该包当前确实没额度了(正常拒绝)。换一个有余额的 token,或这就是预期。";;
  SUCCESS)    grn "预检 200:上游可用,既测门控也会真实扣减。";;
  OTHER:*)    ylw "预检 $warm:门控已通过(非 quota/模型/鉴权问题),多半上游渠道未就绪——不影响对 EXHAUSTED 的判定。";;
esac

RES=$(seq "$REQUESTS" | xargs -P "$CONCURRENCY" -I{} "$SELF" __worker "$TOKEN" "$MODEL")
succ=$(printf '%s\n' "$RES" | grep -c '^SUCCESS$')
exh=$( printf '%s\n' "$RES" | grep -c '^EXHAUSTED$')
nal=$( printf '%s\n' "$RES" | grep -c '^NOTALLOWED$')
una=$( printf '%s\n' "$RES" | grep -c '^UNAUTH$')
oth=$( printf '%s\n' "$RES" | grep -c '^OTHER')

printf -- '----------------------------------------\n'
printf '  SUCCESS    = %s\n' "$succ"
printf '  EXHAUSTED  = %s   <-- 误判耗尽信号(额度充足时应为 0)\n' "$exh"
printf '  NOTALLOWED = %s\n' "$nal"
printf '  UNAUTH     = %s\n' "$una"
printf '  OTHER      = %s   (上游/其它,与门控正确性无关)\n' "$oth"
printf -- '----------------------------------------\n'

if [ "$exh" -gt 0 ]; then
  red "❌ 出现 $exh 次 quota exhausted。若该包仍有额度=复现原 bug;若包已用尽=正常拒绝。"
  exit 2
fi
grn "✅ 高并发下未出现误判耗尽(EXHAUSTED=0)。"
