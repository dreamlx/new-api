# LH Integration — Operations Runbook

Ops 手册：canary 期间 + cutover 后 production 的部署、监控、日志、应急操作。

适用范围：`lh-main` 分支的 new-api 部署，集成 LH Enterprise backend 的环境。

---

## 0. 当前状态速查（2026-05-23）

| 项 | 值 |
|---|---|
| Canary host | `172.235.205.45` (Linode 4 vCPU / 8 GB) |
| new-api 镜像 | `new-api:lh-main`（本机 build，commit a82a6137d）|
| new-api 公网入口 | `http://172.235.205.45:3001`（canary 期间公网开放，cutover 后切回 loopback）|
| Admin login | `admin` / 见 `memory/canary_new_api_credentials.md` |
| Source on canary | `/root/new-api-src/`（git lh-main）|
| Compose on canary | `/root/new-api/docker-compose.yml` |

---

## 1. 架构

### 1.1 容器拓扑

| Container | Image | Role | Network |
|---|---|---|---|
| `new-api` | `new-api:lh-main` | 主服务 | `new-api_new-api-network` + `lh-enterprise_default` |
| `postgres` | `postgres:15` | new-api 自己的 DB（alias: `postgres`, `new-api-postgres`）| `new-api_new-api-network` |
| `redis` | `redis:latest` | 缓存 | `new-api_new-api-network` |
| `lh-enterprise-*` | 外部 LH stack | LH backend / web / postgres | `lh-enterprise_default`（new-api 同主机共存）|

### 1.2 网络

- `new-api_new-api-network` (172.18.0.0/16) — new-api 内部 stack（postgres + redis）
- `lh-enterprise_default` (172.19.0.0/16) — LH stack 网络，new-api **作为成员**接入，LH backend 通过容器名 `new-api` 解析
- 双网络挂载持久化：`/root/new-api/docker-compose.yml` 的 `networks` 区块

### 1.3 端口

| 端口 | 状态 | Cutover 后 |
|---|---|---|
| `0.0.0.0:3001` → new-api:3000 | canary 期间公网开放（debug）| **删除**，仅 docker network 内可达 |
| `new-api:3000`（docker network 内）| 持续可用 | 持续可用（LH backend 的访问路径）|

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

# 5. 等健康
until curl -sf http://localhost:3001/api/status | grep -q success; do sleep 2; done; echo READY
```

### 2.2 验证升级生效

```bash
# 镜像 digest 应该匹配 build 输出
docker inspect new-api --format '{{.Image}}'

# 行为级验证：跑一次 chat completion
curl -H "Authorization: Bearer <test_token>" -d '{"model":"deepseek-v4-flash","messages":[{"role":"user","content":"hi"}],"max_tokens":10}' -H "Content-Type: application/json" http://localhost:3001/v1/chat/completions
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

## 3. Cutover (D7) — 切换 docker network 访问

### 3.1 ops 侧动作

```bash
# 1. 编辑 compose 删除 ports 行
sed -i '/^\s\+-\s\+"3001:3000"/d' /root/new-api/docker-compose.yml

# 2. 应用
cd /root/new-api && docker compose up -d

# 3. 验证公网 :3001 拒接
curl -m 5 http://172.235.205.45:3001/api/status
# expected: Connection refused
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
# 加回 ports 行（compose 备份在 /root/new-api/docker-compose.yml.bak-*）
cp /root/new-api/docker-compose.yml.bak-20260523 /root/new-api/docker-compose.yml
docker compose up -d
```

---

## 4. 监控（手工查询路径）

**当前没有 Prometheus exporter / Grafana / 飞书告警 webhook**。所有监控走手工查询，D6 加 cron 脚本提供基础告警。

### 4.1 服务级健康

```bash
# 整体健康
curl -s http://localhost:3001/api/status | jq

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

### 5.3 Log retention（D6 配置后）

docker daemon log-opts（计划写入 `/etc/docker/daemon.json`）：
```json
{
  "log-driver": "json-file",
  "log-opts": {
    "max-size": "100m",
    "max-file": "10"
  }
}
```
→ 单容器最多 1 GB 日志，约 1-3 个月保留期。

---

## 6. 灾备 / 数据恢复

### 6.1 PG 备份

**当前最新备份**：`/root/new-api-pg-backup-20260512-065630.sql`（320K，pre-LH-customizations 状态）

**手工备份**：
```bash
docker exec -i postgres pg_dumpall -U root > /root/new-api-pg-backup-$(date +%Y%m%d-%H%M%S).sql
```

**D6 计划添加 cron**（每日 02:00 自动备份 + 保留 30 天）。

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

```bash
# 生成 bcrypt hash
ssh root@172.235.205.45 "htpasswd -bnBC 10 '' '<new_pass>' | tr -d ':\n' | sed 's/\\\$2y/\\\$2a/'"

# 写入 PG
ssh root@172.235.205.45 "docker exec -i postgres psql -U root -d new-api -c \"UPDATE users SET password='<bcrypt_hash>' WHERE username='admin';\""
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

### 8.1 Postgres DNS collision（已 fix）

new-api 容器同时在 `new-api_new-api-network` 和 `lh-enterprise_default` 网络。LH stack 自己的 postgres alias 也是 `postgres`，所以 new-api 的 `SQL_DSN=...@postgres:5432/...` 会解析到 LH 的库（错密码 + TLS required）→ 启动循环。

**Fix**: new-api 自己的 postgres 加 unique alias `new-api-postgres`，SQL_DSN 用新 alias。已在 compose 里写死。

**踩坑指标**：new-api 启动循环、log 报 `password authentication failed for user "root"` + IP 是 `172.19.0.x`（LH 网络）而不是 `172.18.0.x`（new-api 网络）。

### 8.2 SSH fail2ban

短时多次 SSH 连接会触发 fail2ban，~5-15 分钟解禁。批量操作建议合并到单条 SSH 命令。

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
  ├── .session_secret.txt    # SESSION_SECRET (chmod 600)
  ├── .admin_pass.txt        # 早期 reset 时密码备份（已过期）
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
