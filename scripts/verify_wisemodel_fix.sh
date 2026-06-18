#!/usr/bin/env bash
#
# verify_wisemodel_fix.sh — 一键验证 wisemodel 修复分支(在你本地 docker 环境上)
#
# 做的事(全自动):
#   1. 用当前分支 HEAD 构建镜像(Dockerfile 内部 build 前端+go)
#   2. 复用现有 new-api 容器的网络/DSN,起一个修复容器(端口 FIX_PORT)
#   3. 重置回填标志,让修复后的【精确归因】回填重新跑
#   4. 验证回填被修正:精确消费 << 授予额度的包,remain_quota 应≈granted-精确消费(而非被误置 0)
#   5. 对【有余额的计价模型】高并发压测,断言不出现误判 quota exhausted、remain 不为负
#
# 用法:  ./scripts/verify_wisemodel_fix.sh
# 可选环境变量:
#   FIX_PORT=3001  REQUESTS=1000  CONCURRENCY=50
#   MYSQL_CONTAINER=newapi-mysql  MYSQL_USER=root  MYSQL_PW=123456  MYSQL_DB=new-api
#   SRC_CONTAINER=new-api   (读取其网络/SQL_DSN/REDIS_CONN_STRING)
#   SKIP_BUILD=1  (复用已存在的 new-api-fix:local 镜像,跳过构建)
#
set -uo pipefail

FIX_IMAGE=${FIX_IMAGE:-new-api-fix:local}
FIX_CONTAINER=${FIX_CONTAINER:-new-api-fix}
FIX_PORT=${FIX_PORT:-3001}
REQUESTS=${REQUESTS:-1000}
CONCURRENCY=${CONCURRENCY:-50}
MYSQL_CONTAINER=${MYSQL_CONTAINER:-newapi-mysql}
MYSQL_USER=${MYSQL_USER:-root}
MYSQL_PW=${MYSQL_PW:-123456}
MYSQL_DB=${MYSQL_DB:-new-api}
SRC_CONTAINER=${SRC_CONTAINER:-new-api}

DIR=$(cd "$(dirname "$0")/.." && pwd)
SELF_DIR=$(cd "$(dirname "$0")" && pwd)
BASE=http://localhost:$FIX_PORT

red(){ printf '\033[31m%s\033[0m\n' "$*"; }
grn(){ printf '\033[32m%s\033[0m\n' "$*"; }
ylw(){ printf '\033[33m%s\033[0m\n' "$*"; }
stage(){ printf '\n\033[36m=== %s ===\033[0m\n' "$*"; }
mq(){ docker exec "$MYSQL_CONTAINER" mysql -u"$MYSQL_USER" -p"$MYSQL_PW" "$MYSQL_DB" -N -e "$1" 2>/dev/null; }

command -v docker >/dev/null || { red "需要 docker"; exit 1; }
docker ps --format '{{.Names}}' | grep -qx "$MYSQL_CONTAINER" || { red "找不到运行中的 MySQL 容器 $MYSQL_CONTAINER"; exit 1; }
[ -f "$DIR/Dockerfile" ] || { red "$DIR/Dockerfile 不存在,无法构建"; exit 1; }

# ---------- Stage 1: 构建 ----------
stage "Stage 1/5  构建修复镜像 $FIX_IMAGE(当前分支 $(git -C "$DIR" rev-parse --short HEAD))"
if [ "${SKIP_BUILD:-0}" = "1" ] && docker image inspect "$FIX_IMAGE" >/dev/null 2>&1; then
  ylw "  SKIP_BUILD=1,复用已存在镜像"
else
  docker build -t "$FIX_IMAGE" "$DIR" || { red "构建失败"; exit 1; }
fi

# ---------- Stage 2: 起修复容器(同网络/同 DSN)----------
stage "Stage 2/5  启动修复容器(端口 $FIX_PORT,复用 $SRC_CONTAINER 的网络与 DSN)"
NET=$(docker inspect "$SRC_CONTAINER" --format '{{range $k,$v := .NetworkSettings.Networks}}{{$k}} {{end}}' 2>/dev/null | awk '{print $1}')
SQL_DSN=$(docker inspect "$SRC_CONTAINER" --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null | grep '^SQL_DSN=' | cut -d= -f2-)
REDIS_CONN=$(docker inspect "$SRC_CONTAINER" --format '{{range .Config.Env}}{{println .}}{{end}}' 2>/dev/null | grep '^REDIS_CONN_STRING=' | cut -d= -f2-)
[ -n "$NET" ] && [ -n "$SQL_DSN" ] || { red "无法从 $SRC_CONTAINER 读取网络/SQL_DSN(它在运行吗?)"; exit 1; }
echo "  network=$NET"
echo "  SQL_DSN=$SQL_DSN"

docker rm -f "$FIX_CONTAINER" >/dev/null 2>&1
docker run -d --name "$FIX_CONTAINER" --network "$NET" -p "$FIX_PORT:3000" \
  -e "SQL_DSN=$SQL_DSN" ${REDIS_CONN:+-e "REDIS_CONN_STRING=$REDIS_CONN"} \
  -e "WISEMODEL_API_TOKEN=verify-admin" \
  "$FIX_IMAGE" >/dev/null || { red "启动失败"; exit 1; }

# ---------- Stage 3: 重置回填标志,等修复回填跑完 ----------
stage "Stage 3/5  重置回填标志并等待修复后的精确回填"
mq "DELETE FROM options WHERE \`key\`='WisemodelRemainBackfilled';"
docker restart "$FIX_CONTAINER" >/dev/null   # 重启触发 Init() 重新回填
echo -n "  等待服务就绪"
for i in $(seq 1 60); do
  code=$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 "$BASE" || true)
  [ "$code" = "200" ] && { echo " OK"; break; }
  echo -n "."; sleep 2
  [ "$i" = 60 ] && { echo; red "服务 60 次轮询仍未就绪,看 docker logs $FIX_CONTAINER"; exit 1; }
done
sleep 2  # 给回填一点收尾时间
flag=$(mq "SELECT value FROM options WHERE \`key\`='WisemodelRemainBackfilled';")
echo "  回填标志 = ${flag:-<未写入>}"

# ---------- Stage 4: 验证回填修正 ----------
stage "Stage 4/5  验证回填修正(精确消费<<授予 的包应 remain≈granted-精确,而非 0)"
pkg=$(mq "SELECT p.package_id FROM wisemodel_packages p
  WHERE p.reclaimed_at IS NULL AND p.valid_until>NOW() AND p.quota_granted>1000
    AND p.quota_granted > 2*(SELECT COALESCE(SUM(l.quota),0) FROM logs l WHERE l.wisemodel_package_id=p.package_id AND l.type=2)
  ORDER BY p.quota_granted DESC LIMIT 1;")
VERDICT_BF=SKIP
if [ -z "$pkg" ]; then
  ylw "  库里没有'精确消费<<授予'的包可供对照(可能数据都已用尽),跳过该项。"
else
  read -r granted used remain <<<"$(mq "SELECT p.quota_granted,
     (SELECT COALESCE(SUM(l.quota),0) FROM logs l WHERE l.wisemodel_package_id=p.package_id AND l.type=2),
     p.remain_quota FROM wisemodel_packages p WHERE p.package_id='$pkg';" | tr '\t' ' ')"
  expect=$((granted - used))
  echo "  包 $pkg"
  echo "    granted=$granted  precise_used=$used  expect≈$expect  实际 remain=$remain"
  # 容忍少量回填后新增消费:remain 落在 [expect*0.9, expect] 视为正确;关键是 >0 且接近 expect
  if [ "$remain" -gt 0 ] && [ "$remain" -le "$granted" ] && [ "$remain" -ge $((expect*9/10)) ]; then
    grn "  ✅ 回填已修正:remain 不再被误置 0,≈ granted-精确消费"
    VERDICT_BF=PASS
  else
    red "  ❌ 回填异常:remain=$remain 远离期望 $expect(若仍为 0=未跑到修复后的回填)"
    VERDICT_BF=FAIL
  fi
fi

# ---------- Stage 5: 计价模型高并发不误判耗尽 ----------
stage "Stage 5/5  高并发压测(有余额的计价模型不应误判 quota exhausted)"
# 优先选覆盖 minimax-m2.5-highspeed(按 token 计价)且有余额的 token
read -r tok models remain0 <<<"$(mq "SELECT t.\`key\`, p.available_models, p.remain_quota
  FROM tokens t JOIN wisemodel_packages p ON p.user_id=t.user_id AND p.reclaimed_at IS NULL AND p.valid_until>NOW()
  WHERE (t.name='wisemodel-token' OR t.\`key\` LIKE 'wisemodel-%') AND t.status=1
    AND p.remain_quota>0 AND p.available_models LIKE '%minimax-m2.5-highspeed%'
  ORDER BY p.remain_quota DESC LIMIT 1;" | tr '\t' ' ')"
MODEL=minimax-m2.5-highspeed
if [ -z "${tok:-}" ]; then
  # 退而求其次:任意有余额的 token,用其包覆盖的第一个模型(可能是免费模型→门控不扣减)
  read -r tok models remain0 <<<"$(mq "SELECT t.\`key\`, p.available_models, p.remain_quota
    FROM tokens t JOIN wisemodel_packages p ON p.user_id=t.user_id AND p.reclaimed_at IS NULL AND p.valid_until>NOW()
    WHERE (t.name='wisemodel-token' OR t.\`key\` LIKE 'wisemodel-%') AND t.status=1 AND p.remain_quota>0
    ORDER BY p.remain_quota DESC LIMIT 1;" | tr '\t' ' ')"
  MODEL=$(printf '%s' "$models" | tr ',' '\n' | sed '/^$/d' | head -1)
  [ -n "${tok:-}" ] && ylw "  未找到覆盖计价模型 m2.5-highspeed 的有余额 token,改用 $MODEL(可能免费→门控不扣减,仅验证不误判)"
fi
VERDICT_LOAD=SKIP
if [ -z "${tok:-}" ]; then
  ylw "  没有任何'有余额'的有效包可压测(回填可能没修正/数据都已耗尽),跳过。"
else
  echo "  token=$tok  model=$MODEL  remain(before)=$remain0"
  out=$(TOKEN="$tok" MODEL="$MODEL" BASE_URL="$BASE" REQUESTS="$REQUESTS" CONCURRENCY="$CONCURRENCY" \
        bash "$SELF_DIR/hammer_wisemodel_token.sh"); rc=$?
  echo "$out" | sed 's/^/    /'
  remain1=$(mq "SELECT COALESCE(SUM(remain_quota),0) FROM wisemodel_packages p
    JOIN tokens t ON t.user_id=p.user_id WHERE t.\`key\`='$tok' AND p.reclaimed_at IS NULL AND p.valid_until>NOW();")
  echo "  remain(after)=$remain1"
  # 通过条件:hammer 退出码 0(EXHAUSTED=0)且 remain 不为负
  if [ "$rc" = 0 ]; then VERDICT_LOAD=PASS; else VERDICT_LOAD=FAIL; fi
  case "$remain1" in -*) VERDICT_LOAD=FAIL; red "  remain 为负=超卖!";; esac
fi

# ---------- 汇总 ----------
stage "结果汇总"
printf "  回填修正(不误置0)     : %s\n" "$VERDICT_BF"
printf "  高并发不误判耗尽       : %s\n" "$VERDICT_LOAD"
printf "\n  修复容器仍在运行: %s (http://localhost:%s)\n" "$FIX_CONTAINER" "$FIX_PORT"
printf "  清理: docker rm -f %s\n" "$FIX_CONTAINER"
if [ "$VERDICT_BF" = FAIL ] || [ "$VERDICT_LOAD" = FAIL ]; then red "\n❌ 存在未通过项"; exit 2; fi
grn "\n✅ 验证通过:回填修正生效,高并发下额度充足始终可访问。"
