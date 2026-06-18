# LH Integration — Operations Runbook

Ops 手册：canary 期间 + cutover 后 production 的部署、监控、日志、应急操作。

适用范围：`lh-main` 分支的 new-api 部署，集成 LH Enterprise backend 的环境。

> **⚠️ Canary compose 不在 git — 以本 runbook 为准**
>
> 仓库根 `docker-compose.yml` 是 upstream Calcium-Ion/new-api 通用模板，**不是** canary 实际跑的版本。Canary `/root/new-api/docker-compose.yml` 是 ops 手 maintained 的 LH 特化版，cutover 前后状态不同，详见 §1（架构 + 端口）/ §3（cutover SOP）/ §8（known gotchas）。
>
> 新部署或灾备重建：**不要直接套仓库版**，按本 runbook §2 / §3 配置。未来如要把 LH 版 compose 入 git，单独开 ADR 讨论 fork 策略。

---

## 0. 当前状态速查（2026-05-25，post-cutover）

| 项 | 值 |
|---|---|
| Canary host | `172.235.205.45` (Linode 4 vCPU / 8 GB) |
| new-api 镜像 | `new-api:lh-main`（本机 build，commit a82a6137d）|
| new-api 入口 | canary 期间：`http://172.235.205.45:3001` 公网开放。Cutover 后：公网 `:3001` 拒接；LH backend 走 docker network `http://new-api:3000`；ops 维护走 Tailscale `http://lh-canary:3001`（IP `100.89.8.49`）|
| Admin login | `admin` / 见 `memory/canary_new_api_credentials.md` |
| Source on canary | `/root/new-api-src/`（git lh-main）|
| Compose on canary | `/root/new-api/docker-compose.yml` |

---

## 1. 架构

### 1.1 容器拓扑

| Container | Image | Role | Network (cutover 前) | Network (cutover 后) |
|---|---|---|---|---|
| `new-api` | `new-api:lh-main` | 主服务 | `new-api-network` + `lh-enterprise_default` | `new-api-network` + `lh-enterprise_default` + `lh-shared`（3 networks）|
| `postgres` | `postgres:15` | new-api 自己的 DB（alias: `postgres`, `new-api-postgres`）| `new-api-network` | `new-api-network` |
| `redis` | `redis:latest` | 缓存 | `new-api-network` | `new-api-network` |
| `lh-enterprise-backend-1` | LH backend | 调用 new-api 的 client | `lh-enterprise_default` | `lh-enterprise_default` + `lh-shared` |
| `lh-enterprise-{postgres,web}-1` | LH 其他组件 | DB / web UI | `lh-enterprise_default` | `lh-enterprise_default`（**不进 lh-shared**）|

### 1.2 网络

- `new-api-network`（new-api compose 自动创建）— new-api 内部 stack（postgres + redis）通信
- `lh-enterprise_default`（LH compose 自动创建）— LH 内部 stack（backend + postgres + web）通信
- `lh-shared`（external bridge，LH `compose.prod.yml` 声明 `name: lh-shared, driver: bridge`）— **cutover 桥**：LH backend ↔ new-api 跨 stack 通信走这个网络
  - LH 侧 PR：[dreamlx/lh-enterprise#352](https://github.com/dreamlx/lh-enterprise/pull/352)
  - new-api 侧 join：见 §3.1 cutover SOP
  - LH postgres / web **不在 lh-shared 上**（设计上限制跨 stack 服务可见性，只把 backend 这一个 actor 暴露给 new-api）

**Cutover 后实际拓扑 nuance（已 verify 2026-05-25）**：new-api 同时 attach 3 个网络（`new-api-network` 内部 stack + 历史 `lh-enterprise_default` pre-attach + 今天加的 `lh-shared`），docker DNS 解析 `new-api` 时优先返回 `lh-enterprise_default` subnet IP（172.19.0.x）而不是 lh-shared (172.20.0.x)。Functionally OK，LH backend → new-api 流量通畅，但 `lh-shared` contract 不是 sole 路径。如要让 lh-shared 成为唯一 cross-stack bridge，单独 follow-up issue 移除 new-api 的 `lh-enterprise_default` attachment。

### 1.3 端口

| 端口 | 状态 | Cutover 后 |
|---|---|---|
| `0.0.0.0:3001` → new-api:3000 | canary 期间公网开放（debug）| **改为 Tailscale-bound publish**（见下）|
| `100.89.8.49:3001` → new-api:3000 | 不存在 | **新增**，docker bind 到 Tailscale interface；公网 `:3001` 拒接，tailnet 内可达 |
| `new-api:3000`（docker network 内）| 持续可用 | 持续可用（LH backend 走 `http://new-api:3000`）|

---

## 2. 部署 / 升级 SOP

### 2.1 标准升级流程

在本地（Mac）：
```bash
# 1. 提交 + push 到 GitHub lh-main
git push origin lh-main
```

在 canary（SSH 到 `root@172.235.205.45`）：
```bash
# 2. 拉最新代码
cd /root/new-api-src
git fetch origin lh-main
git pull --ff-only origin lh-main

# 3. Build 新镜像（约 5-8 分钟，多阶段 bun + go + debian）
docker build -t new-api:lh-main .

# 4. 切换镜像（约 10-30s downtime）
cd /root/new-api
docker compose up -d --force-recreate new-api

# 5. 等健康（publish 绑 Tailscale IP,非 localhost — 见 §3.6;host 上 localhost:3001 返回 000）
NEW_API_LOCAL="http://$(tailscale ip -4):3001"
until curl -sf "$NEW_API_LOCAL/api/status" | grep -q '"success"'; do sleep 2; done; echo READY
```

### 2.2 验证升级生效

```bash
# 镜像 digest 应该匹配 build 输出
docker inspect new-api --format '{{.Image}}'

# 行为级验证：跑一次 chat completion（$NEW_API_LOCAL 见 §2.1 step 5 / §3.6）
curl -H "Authorization: Bearer <test_token>" -d '{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"max_tokens":10}' -H "Content-Type: application/json" "$NEW_API_LOCAL/v1/chat/completions"
```

### 2.3 Rollback

镜像有 tag `new-api:lh-main`（不带版本号），所以 rollback 靠 git revert + rebuild。**建议每次重大升级前手动 tag**：

```bash
# Before upgrade
docker tag new-api:lh-main new-api:lh-main-pre-$(date +%Y%m%d-%H%M)

# Rollback if needed
docker tag new-api:lh-main-pre-YYYYMMDD-HHMM new-api:lh-main
docker compose up -d --force-recreate new-api
```

PG 数据 rollback 用 backup（见 §6 灾备）。

---

## 3. Cutover (D7) — 切换 docker network 访问 + Tailscale-only 维护

设计：LH backend 走 docker network DNS `http://new-api:3000`（容器间直连，无公网暴露）。Ops 维护入口从 `0.0.0.0:3001` 收紧到 `100.89.8.49:3001`（Tailscale-bound），公网 `:3001` 拒接。

### 3.1 ops 侧动作

依赖：LH 侧 [dreamlx/lh-enterprise#352](https://github.com/dreamlx/lh-enterprise/pull/352) 已 merge + LH backend 已 `docker compose up -d` re-create（`lh-shared` 网络物化）。

```bash
# 0. 备份当前 compose
cp /root/new-api/docker-compose.yml /root/new-api/docker-compose.yml.bak-$(date +%Y%m%d-%H%M%S)

# 1. 前置 verify
systemctl is-active tailscaled                          # active
tailscale ip -4                                         # 100.89.8.49
docker network inspect lh-shared >/dev/null && echo OK  # LH 侧已物化才能 join

# 2. 编辑 compose（结构改动，sed 不够，用 vim 或 yq）
#    diff:
#      services:
#        new-api:
#    -     ports:
#    -       - "3001:3000"
#    +     ports:
#    +       - "100.89.8.49:3001:3000"
#          networks:
#            - new-api-network
#    +       - lh-shared
#      networks:
#        new-api-network:
#    +   lh-shared:
#    +     external: true
#    +     name: lh-shared
vim /root/new-api/docker-compose.yml

# 3. 应用
cd /root/new-api && docker compose up -d

# 4. verify listen IP（lockdown evidence）
ss -tlnp | grep ':3001'
# expected: 100.89.8.49:3001（NOT 0.0.0.0:3001）

# 5. verify Tailscale 维护通道
curl -m 5 http://100.89.8.49:3001/api/status
# 或本地 lh-canary ssh alias：curl -m 5 http://lh-canary:3001/api/status
# expected: {"success": true, ...}

# 6. verify cross-stack DNS（LH backend ↔ new-api）
docker exec lh-enterprise-backend-1 getent hosts new-api
# expected: 172.x.x.x  new-api  （lh-shared 子网 IP）

# 7. verify 公网拒接（从 tailnet 外机器）
curl -m 5 http://172.235.205.45:3001/api/status
# expected: Connection refused / timeout
```

### 3.2 LH 侧动作（同步）

```bash
# LH backend .env
NEW_API_BASE_URL=http://new-api:3000

# LH backend 重启
docker compose restart backend
```

### 3.3 acceptance test

```bash
# 容器内 smoke test（LH 侧脚本，#319 comment 4525642906）
docker exec -i lh-enterprise-backend-1 node - < smoke_test.js
# Expected: 4 passed, 0 failed
```

### 3.4 Rollback cutover

如果 cutover 出问题，临时恢复公网访问：

```bash
# 1. 用 §3.1 step 0 生成的最新 backup 还原（ls -t 取最新）
LATEST_BAK=$(ls -t /root/new-api/docker-compose.yml.bak-* | head -1)
cp "$LATEST_BAK" /root/new-api/docker-compose.yml
docker compose up -d

# 或 inline 反向 sed（不依赖 backup）
sed -i 's|100.89.8.49:3001:3000|0.0.0.0:3001:3000|' /root/new-api/docker-compose.yml
docker compose up -d
```

### 3.5 Boot ordering — Tailscale 必须先于 docker

方案 A 把 docker publish bind 到 Tailscale IP `100.89.8.49`，所以 `tailscaled.service` 必须先于 `docker.service` ready，否则 `docker compose up` 报 `bind: cannot assign requested address`。

默认 systemd 顺序通常 OK（`tailscaled` 依赖 `network.target`，早于 `docker.service`），但 reboot 后建议 verify：

```bash
# reboot 后检查顺序
systemctl is-active tailscaled    # active
tailscale ip -4                   # 100.89.8.49（确认 IP 没变）
systemctl is-active docker        # active
docker compose -f /root/new-api/docker-compose.yml ps  # new-api running
```

**failure mode：** docker 启动报 bind error → `systemctl status tailscaled` 检查；如果 Tailscale IP 变了（罕见，Linode 不会自动改但 manual `tailscale logout`/`up` 会重分配），改 compose 里的 IP 或考虑把 IP 外提到 `.env` 的 `TAILSCALE_IP` 变量。

### 3.6 Cutover 后 host 内诊断命令替换

本 runbook 多处示例命令用 `localhost:3001`（canary 期 `0.0.0.0:3001` publish 时 ssh 进 host 跑 loopback 通）。Cutover 后 publish 只绑 `100.89.8.49`，host 上 `localhost:3001` 不再 work，需替换为 Tailscale IP。

**建议在 canary host 的 `~/.bashrc` 加：**
```bash
export NEW_API_LOCAL="http://$(tailscale ip -4):3001"
```

后续所有 host 内诊断命令用 `$NEW_API_LOCAL/api/status` 等，runbook 例子里的 `http://localhost:3001/...` 一律换成 `$NEW_API_LOCAL/...`。

**已转用 `$NEW_API_LOCAL`**：§2.1 readiness probe + 行为级验证、§4.1 健康查询。
**仍用字面 `localhost:3001`（host 上会 000,诊断时记得换 `$NEW_API_LOCAL`）**：§4.4 login/channel-test 示例、§7 等其余诊断命令。

> 从开发者本地访问无影响：`http://lh-canary:3001/...`（ssh config alias）走 Tailscale 直达，cutover 前后一致。

---

## 4. 监控（手工查询路径）

**当前没有 Prometheus exporter / Grafana / 飞书告警 webhook**。所有监控走手工查询，D6 加 cron 脚本提供基础告警。

### 4.1 服务级健康

```bash
# 整体健康（$NEW_API_LOCAL 见 §3.6;host 上 localhost:3001 返回 000）
curl -s "$NEW_API_LOCAL/api/status" | jq

# Container 状态
docker ps --filter name=new-api --format 'table {{.Names}}\t{{.Status}}'

# Resource usage
docker stats --no-stream new-api
```

### 4.2 Channel 健康

**Admin UI**：`http://172.235.205.45:3001/console/channel`
- 每条 channel 的"响应时间"列显示最近 channel test 结果
- 状态列：已启用 / 已禁用 / 自动禁用

**Channel test API**：
```bash
# Login first (saves cookie to /tmp/newapi_cookies.txt)
curl -c /tmp/newapi_cookies.txt -X POST -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"<see memory>"}' \
  http://localhost:3001/api/user/login

# Test channel by ID
curl -b /tmp/newapi_cookies.txt -H "New-Api-User: 1" \
  http://localhost:3001/api/channel/test/{id}
```

### 4.3 错误率快速诊断

```bash
# Fatal errors in new-api log (last 100 lines)
docker logs new-api --tail 100 2>&1 | grep -E "FATAL|ERROR|panic"

# Recent 5xx pattern check
docker logs new-api --since 1h 2>&1 | grep -E "5[0-9][0-9]" | tail -20
```

### 4.4 DB 连接 / migration 状态

```bash
docker exec -i postgres psql -U root -d new-api -c "SELECT version();"
docker exec -i postgres psql -U root -d new-api -c "\dt" | head -20
```

### 4.5 Channel health 自动监测

每 5 分钟 cron 跑 `/root/new-api/scripts/channel_health_check.sh`，直接查 PG `channels` 表 status 列（不烧上游 channel test 成本），仅在 status 变化时往 `/var/log/channel-health.log` 写一行结构化 log：

```
2026-05-23T15:21:17+00:00 CHANNEL_STATE_CHANGE id=4 name=C1-deepseek-v4-pro-primary enabled -> manual-disabled response_time=1567ms
```

监测范围：LH 集成 channel 仅（id 3, 4, 5, 6）。状态值：
- `1=enabled` 正常
- `2=manual-disabled` ops 手动禁用
- `3=auto-disabled` new-api 因连续失败自动禁用（**这条是真实告警信号**）

**LH 侧的工作**：tail 这个 log file，匹配 `auto-disabled` 模式触发飞书/邮件告警。new-api 这边只负责检测，不做投递。

State 持久化在 `/var/lib/new-api-monitor/channel-{id}.txt`。

**手工跑**：
```bash
/root/new-api/scripts/channel_health_check.sh
# (only prints if state changed since last run)
```

---

## 5. 日志查询

### 5.1 位置

| 类型 | 路径 |
|---|---|
| new-api 应用日志 | `/root/new-api/logs/` (mount bind from container `/app/logs`) |
| docker 容器日志 | `docker logs new-api` (json-file 驱动) |
| postgres 日志 | `docker logs postgres` |

### 5.2 常用查询

```bash
# 按 request_id 查（LH support 场景）
docker exec -i postgres psql -U root -d new-api -c \
  "SELECT id, user_id, channel_id, model_name, prompt_tokens, completion_tokens, quota, created_at, content FROM logs WHERE request_id = '<id>' LIMIT 5;"

# 按 channel 拉最近用量
docker exec -i postgres psql -U root -d new-api -c \
  "SELECT channel_id, COUNT(*) as calls, SUM(prompt_tokens+completion_tokens) as tokens, SUM(quota) as total_quota FROM logs WHERE created_at > extract(epoch from now()) - 86400 GROUP BY channel_id ORDER BY total_quota DESC;"

# 按 platform user 拉日志（LH reconciliation 视角）
docker exec -i postgres psql -U root -d new-api -c \
  "SELECT * FROM logs WHERE user_id = (SELECT id FROM users WHERE external_user_id='<orgId>') ORDER BY created_at DESC LIMIT 20;"
```

### 5.3 Log retention

`new-api` container 已配 compose 级 logging（`/root/new-api/docker-compose.yml` 的 `services.new-api.logging`）：

```yaml
logging:
  driver: json-file
  options:
    max-size: "100m"
    max-file: "10"
```

→ 单容器最多 1 GB docker engine 日志，约 1-3 个月保留期。**远超 LH §4.4 14 天要求**。

Verify: `docker inspect new-api --format '{{json .HostConfig.LogConfig}}'`

> postgres / redis 容器没配 rotation（log 量小，目前不必要）。如未来需要：在 compose 对应 service 加同样 `logging:` 块。

---

## 6. 灾备 / 数据恢复

### 6.1 PG 备份

**自动备份**：每日 02:00 UTC cron 跑 `/root/new-api/scripts/pg_backup.sh`，输出 `/root/new-api-pg-backup-YYYYMMDD-HHMMSS.sql.gz`（chmod 600），保留 30 天后自动删除。日志在 `/var/log/pg-backup.log`。

**手工触发**：
```bash
/root/new-api/scripts/pg_backup.sh
```

**Verify cron 装好**：
```bash
crontab -l | grep pg_backup
# 0 2 * * * /root/new-api/scripts/pg_backup.sh >> /var/log/pg-backup.log 2>&1
```

### 6.2 PG 恢复

```bash
# 1. 停掉 new-api（避免写入 race）
docker compose stop new-api

# 2. Restore
docker exec -i postgres psql -U root -d new-api < /root/new-api-pg-backup-YYYYMMDD-HHMMSS.sql

# 3. 重启 new-api
docker compose up -d new-api
```

### 6.3 容器重建

如果 `new-api` 容器丢失（image 还在）：
```bash
cd /root/new-api && docker compose up -d
```

如果整个 stack 丢失：
```bash
cd /root/new-api
docker compose down  # 不带 -v，保留 pg_data volume
docker compose up -d
```

PG `pg_data` 是 named volume，跨 `docker compose down` 保留。除非显式 `docker volume rm new-api_pg_data` 才会删数据。

---

## 7. 常用操作

### 7.1 Admin 密码重置

SSH 到 canary 后两步：

```bash
# Step 1: 生成 bcrypt hash（注意：单引号包密码避免 shell 解释；$2y -> $2a 是 Go bcrypt 兼容性需要）
HASH=$(htpasswd -bnBC 10 '' 'NEW_PASSWORD_HERE' | tr -d ':\n' | sed 's/\$2y/\$2a/')
echo "$HASH"

# Step 2: 写入 PG（$HASH 在双引号内会展开，PG 内嵌单引号包字符串）
docker exec -i postgres psql -U root -d new-api -c "UPDATE users SET password='$HASH' WHERE username='admin';"
```

Verify：用新密码 curl login：
```bash
curl -c /tmp/test.txt -X POST -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"NEW_PASSWORD_HERE"}' \
  http://localhost:3001/api/user/login
# expected: {"success":true,...}
```

### 7.2 创建 system token

通过 admin UI：`/console/token` → 添加 → 设置 unlimited_quota + expired_time=-1

通过 API：见 `controller/token.go` POST `/api/token/`。

### 7.3 添加 / 修改 channel

通过 admin UI：`/console/channel` → 添加渠道。Channel test 一定要跑（latency 数据进 SLA tracking）。

通过 API：POST `/api/channel/` body shape 见 `controller/channel.go:527` `AddChannelRequest`。

### 7.4 设置 model ratio

任何新 model id 在通过 channel 服务之前必须在 `ModelRatio` + `CompletionRatio` 配置：

通过 admin UI：`/console/setting` → 模型设置。

通过 API：PUT `/api/option/` body `{"key":"ModelRatio","value":"<json string>"}`。

---

## 8. Known gotchas

### 8.1 Postgres DNS collision（preemptive defensive，未实际触发）

**风险**：两个 docker compose stack（new-api + lh-enterprise）各自有 postgres，默认 alias 都叫 `postgres`。若 new-api 与 LH postgres 处于同一网络，new-api 的 `SQL_DSN=...@postgres:5432/...` 可能解析到 LH 的库（错密码 + TLS required）→ 启动循环。

**Cutover 实际**：[lh-enterprise#352](https://github.com/dreamlx/lh-enterprise/pull/352) 只把 LH backend join `lh-shared`，**LH postgres 未 join**，因此 new-api 跟 LH postgres 不会在同一网络，DNS 冲突不会发生。

**仍保留的 defensive 配置**（万一未来 LH 把 postgres 也 join lh-shared）：
- new-api compose 给自己的 postgres 加 alias `new-api-postgres`
- `SQL_DSN=postgresql://root:123456@new-api-postgres:5432/new-api`

**踩坑指标**（debug 用）：new-api 启动循环、log 报 `password authentication failed for user "root"` + IP 是 LH 网络段而不是 new-api 网络段。

### 8.2 SSH fail2ban（2026-06-19 实装）

**更正**：此前 SOP 误标 fail2ban 已存在；实际 2026-06-19 之前**根本没装**（也无 sshguard/denyhosts/crowdsec）。当时观察到的"短时多次连接被拒"是 sshd 自身的 `maxstartups 10:30:100` 并发节流（临时丢弃过多半开连接，非定时封 IP），不是 fail2ban。

2026-06-19 起已安装 fail2ban v1.1.0 并启用。配置 `/etc/fail2ban/jail.local`：
- `[sshd]` jail，`backend = systemd`（读 journald，本机 sshd 日志不落 `/var/log/auth.log`）
- `maxretry = 5` / `findtime = 10m` / `bantime = 15m`
- `ignoreip = 127.0.0.1/8 ::1 100.64.0.0/10` —— **Tailscale CGNAT 段白名单：走 `lh-canary`（Tailscale）的维护连接永不被封，不会自锁**

公网 `:22` 暴露、bot 持续爆破 → fail2ban 自动封。**注意**：从 `lh-canary-public`（公网 172.235.205.45）反复失败仍会被封（公网源 IP 不在 ignoreip）——维护一律走 `lh-canary`（Tailscale）。

```bash
fail2ban-client status sshd          # 查看 jail / 已封 IP
fail2ban-client set sshd unbanip <IP> # 手动解封
```

### 8.3 Admin API 需要 `New-Api-User` header

`/api/channel/`、`/api/token/`、`/api/option/` 等 admin API 除 session cookie 外还要 `New-Api-User: <user_id>` header（一般 admin 是 `1`）。

### 8.4 ab 的 "Failed requests" 包含 Length 类

stress test 时 `ab` 把 "response body 长度不一致" 也算 Failed。Chat completion 每次 token 数变所以一直会有这个数。看 `Non-2xx` + `Connect/Receive/Exceptions` 才是真失败。

### 8.5 reasoning 模型 max_tokens 太小返回空 content

DeepSeek v4 系列是 reasoning 模型，response 有 `reasoning_content` 字段，推理 token 算在 max_tokens 配额里。`max_tokens=10` 时所有 budget 被推理吃掉，`content` 是空字符串，`finish_reason=length`。

**生产端建议**：客户调用 max_tokens 不少于 1000，否则空响应风险。

---

## 9. Reference

### 9.1 Channel ID ↔ Name 映射

| ID | Name | Type | Role | Upstream |
|---|---|---|---|---|
| 1 | ospreyAI | OpenAI compat | 历史，非 LH 流量 | open.ospreyai.cn |
| 2 | harry | Anthropic | 历史，已禁用 | api.exchangetoken.ai |
| 3 | C0-deepseek-v4-flash-base | DeepSeek | LH base tier | api.deepseek.com |
| 4 | C1-deepseek-v4-pro-primary | DeepSeek | LH paid primary | api.deepseek.com |
| 5 | C2-deepseek-v4-pro-fallback | OpenAI compat | LH paid fallback | dashscope.aliyuncs.com（百炼）|
| 6 | C3-qwen-max-primary | OpenAI compat | LH qwen paid | dashscope.aliyuncs.com（百炼）|

API 拉权威列表：`GET /api/channel/?p=0&page_size=20` + `Authorization` + `New-Api-User: 1`。

### 9.2 ModelRatio canary 默认值

| Model | ModelRatio | CompletionRatio |
|---|---|---|
| deepseek-v4-flash | 0.1 | 2 |
| deepseek-v4-pro | 0.15 | 4 |
| qwen-max | 0.8 | 3 |

**TBD**：生产 PriceBook 定稿后重设。

### 9.3 文件位置（canary）

```
/root/new-api-src/           # git source, lh-main branch
/root/new-api/               # docker-compose root
  ├── docker-compose.yml     # compose config（include LH network attach）
  ├── docker-compose.yml.bak-*  # 历次 backup
  ├── .session_secret.txt    # SESSION_SECRET (chmod 600) — 在 compose env 引用
  ├── .admin_pass.txt        # ⚠️ 文件内容已过期；**当前密码见 memory/canary_new_api_credentials.md**，不要从这里读
  ├── data/                  # new-api persistent data
  └── logs/                  # new-api app logs
/root/new-api-pg-backup-*.sql # PG dumps
```

### 9.4 Linode 控制台

控制台账号、Cloud Firewall 规则、Backups 配置 — 见 LH ops password vault。

### 9.5 Stress test baseline（2026-05-23）

| Workload | RPS | p50 | p99 |
|---|---|---|---|
| `/api/status` c=50 | 6,816 | 6ms | 37ms |
| `/v1/chat/completions` deepseek-v4-flash c=10 | 9.65 | 819ms | 1,204ms |
| `/v1/chat/completions` deepseek-v4-flash c=20 | 14.69 | 840ms | 1,144ms |

完整数据见 issue #319 comment 4525740794。

---

## 10. SLA & Escalation

**承诺 SLA**：99.0% 月度可用（≤7.2 小时停机/月）。

**Escalation tier**:
1. P1（服务完全不可用）：ops 立刻介入，PG 备份恢复 / docker compose recreate
2. P2（部分功能降级 / 单 channel down）：admin UI 排查 channel test，禁用故障 channel 让流量 failover
3. P3（latency degraded）：检查上游状态页（DeepSeek / 百炼），考虑临时 priority 调整

**联系人**：ops 当前是 user @dreamlx。
