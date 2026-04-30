# 环境变量配置指南

本文档覆盖 new-api 所有环境变量的含义、默认值、使用效果及组合配置建议。

---

## 目录

1. [服务器与应用](#1-服务器与应用)
2. [数据库](#2-数据库)
3. [Redis 与缓存](#3-redis-与缓存)
4. [调试与性能分析](#4-调试与性能分析)
5. [网络与 HTTP 客户端](#5-网络与-http-客户端)
6. [限流](#6-限流)
7. [任务处理](#7-任务处理)
8. [渠道与模型同步](#8-渠道与模型同步)
9. [AI 模型行为](#9-ai-模型行为)
10. [文件与请求体](#10-文件与请求体)
11. [安全与认证](#11-安全与认证)
12. [分析统计](#12-分析统计)
13. [OAuth 与外部服务](#13-oauth-与外部服务)
14. [初始化](#14-初始化)
15. [组合配置方案](#15-组合配置方案)

---

## 1. 服务器与应用

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `PORT` | `3000` | HTTP 监听端口 |
| `GIN_MODE` | （空，非 debug）| Gin 框架模式；设为 `debug` 开启详细请求日志 |
| `FRONTEND_BASE_URL` | （空）| 前端代理 URL；非空时将前端请求反向代理到该地址，而不使用内置前端 |
| `NODE_TYPE` | `master` | 节点类型。设为 `slave` 时禁用主节点独有的后台任务（任务轮询、渠道自动更新等） |
| `HOSTNAME` | `new-api` | 节点主机名，用于 Pyroscope 分析的标签 |
| `SESSION_SECRET` | `random_string` | Session 加密密钥，**生产环境必须修改** |
| `CRYPTO_SECRET` | （继承 `SESSION_SECRET`）| 敏感数据加密密钥；未设置时自动使用 `SESSION_SECRET` |

**关键说明：**
- `SESSION_SECRET` 和 `CRYPTO_SECRET` 若使用默认值，攻击者可伪造会话或解密敏感字段。生产环境应设置为随机长字符串。
- `FRONTEND_BASE_URL` 通常用于前后端分离部署。主节点提供 API，前端由独立服务（如 CDN、Nginx）托管。

---

## 2. 数据库

### 核心连接

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `SQL_DSN` | （空，使用 SQLite）| 主数据库连接串。支持 MySQL DSN（`user:pass@tcp(host:3306)/db?parseTime=true`）或 PostgreSQL URL（`postgres://user:pass@host/db`） |
| `LOG_SQL_DSN` | （空，使用 `SQL_DSN`）| 独立的日志数据库连接串；未设置则与主库共用 |
| `SQLITE_PATH` | `./test.db` | SQLite 数据库文件路径（仅在 `SQL_DSN` 未设置时生效） |
| `ERROR_LOG_ENABLED` | `false` | 启用错误日志写入数据库 |

### 连接池调优

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `SQL_MAX_IDLE_CONNS` | `20` | 最大空闲连接数 |
| `SQL_MAX_OPEN_CONNS` | `100` | 最大打开连接数 |
| `SQL_MAX_LIFETIME` | `60` | 连接最长生命周期（秒） |
| `SQL_MAX_IDLE_TIME` | `30` | 连接最长空闲时间（秒） |

**组合使用：**

```
# MySQL 生产环境示例
SQL_DSN=user:password@tcp(127.0.0.1:3306)/new_api?parseTime=true
LOG_SQL_DSN=user:password@tcp(127.0.0.1:3306)/new_api_log?parseTime=true
SQL_MAX_IDLE_CONNS=50
SQL_MAX_OPEN_CONNS=200
SQL_MAX_LIFETIME=300
SQL_MAX_IDLE_TIME=60
ERROR_LOG_ENABLED=true
```

> `LOG_SQL_DSN` 建议在日志量大时单独设置，避免日志写入影响业务查询性能。

**数据库选择逻辑：**
- 未设置 `SQL_DSN` → 使用 SQLite（路径由 `SQLITE_PATH` 决定）
- `SQL_DSN` 以 `postgres://` 或 `postgresql://` 开头 → PostgreSQL
- 其他 → MySQL

---

## 3. Redis 与缓存

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `REDIS_CONN_STRING` | （空，禁用 Redis）| Redis 连接 URL，格式：`redis://[user:pass@]host:port/db` |
| `REDIS_POOL_SIZE` | `10` | Redis 连接池大小 |
| `MEMORY_CACHE_ENABLED` | `false` | 启用内存缓存；配置了 Redis 时自动启用 |
| `SYNC_FREQUENCY` | `60` | 缓存同步频率（秒）；影响 Redis key 的过期时间 |
| `BATCH_UPDATE_ENABLED` | `false` | 启用批量写入模式，将频繁的数据库更新合并为批次操作，降低 DB 压力 |
| `BATCH_UPDATE_INTERVAL` | `5` | 批量写入的间隔（秒），仅在 `BATCH_UPDATE_ENABLED=true` 时生效 |

**缓存分级说明：**

| 场景 | 推荐配置 |
|---|---|
| 单机轻量部署 | 仅设置 `MEMORY_CACHE_ENABLED=true`，不需要 Redis |
| 多节点/高并发 | 设置 `REDIS_CONN_STRING`，内存缓存自动启用，Redis 负责跨节点同步 |
| 高写入量 | 同时开启 `BATCH_UPDATE_ENABLED=true` + 合理设置 `BATCH_UPDATE_INTERVAL` |

**组合示例：**

```
REDIS_CONN_STRING=redis://user:password@localhost:6379/0
REDIS_POOL_SIZE=20
SYNC_FREQUENCY=30
BATCH_UPDATE_ENABLED=true
BATCH_UPDATE_INTERVAL=5
```

---

## 4. 调试与性能分析

### 调试模式

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `DEBUG` | `false` | 开启调试日志；输出更多内部状态信息 |
| `GIN_MODE` | （非 debug）| 设为 `debug` 时 Gin 输出每条路由注册信息和请求日志 |
| `ENABLE_PPROF` | `false` | 在端口 `8005` 启动 Go pprof 端点，用于 CPU/内存/goroutine 分析 |

**组合：** 本地开发建议同时设置：

```
DEBUG=true
GIN_MODE=debug
ENABLE_PPROF=true
```

### Pyroscope 持续性能分析

Pyroscope 是一个持续性能分析系统。**只要 `PYROSCOPE_URL` 非空，分析就会自动启动**，其余变量均为可选补充配置。

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `PYROSCOPE_URL` | （空）| Pyroscope 服务器地址；**此变量为开关**，空则不启用 |
| `PYROSCOPE_APP_NAME` | `new-api` | 上报给 Pyroscope 的应用名称 |
| `PYROSCOPE_BASIC_AUTH_USER` | （空）| Pyroscope HTTP Basic Auth 用户名 |
| `PYROSCOPE_BASIC_AUTH_PASSWORD` | （空）| Pyroscope HTTP Basic Auth 密码 |
| `HOSTNAME` | `new-api` | 上报给 Pyroscope 的主机名标签，多节点时用于区分来源 |
| `PYROSCOPE_MUTEX_RATE` | `5` | `runtime.SetMutexProfileFraction` 采样率，0 为关闭 |
| `PYROSCOPE_BLOCK_RATE` | `5` | `runtime.SetBlockProfileRate` 采样率，0 为关闭 |

**完整 Pyroscope 配置示例：**

```
PYROSCOPE_URL=http://pyroscope.internal:4040
PYROSCOPE_APP_NAME=new-api-prod
PYROSCOPE_BASIC_AUTH_USER=admin
PYROSCOPE_BASIC_AUTH_PASSWORD=secret
HOSTNAME=node-1
PYROSCOPE_MUTEX_RATE=5
PYROSCOPE_BLOCK_RATE=5
```

---

## 5. 网络与 HTTP 客户端

这组变量控制 new-api 向上游 AI 服务商发出请求时使用的 HTTP 客户端行为。

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `RELAY_TIMEOUT` | `0`（不限）| 中继请求总超时（秒）。`0` 表示无限制，适合超长推理任务 |
| `STREAMING_TIMEOUT` | `300` | 流式响应超时（秒）。如果客户端收到空补全，可尝试调大此值 |
| `RELAY_MAX_IDLE_CONNS` | `500` | HTTP 客户端最大空闲连接数 |
| `RELAY_MAX_IDLE_CONNS_PER_HOST` | `100` | 每个上游主机最大空闲连接数 |
| `TLS_INSECURE_SKIP_VERIFY` | `false` | 跳过 TLS 证书验证；**仅供测试，生产环境勿用** |
| `TRUSTED_REDIRECT_DOMAINS` | （空）| 支付回调可信域名列表，逗号分隔，支持子域名匹配。例：`example.com,myapp.io` |

**超时配置建议：**

| 使用场景 | 推荐值 |
|---|---|
| 普通对话（快速响应）| `RELAY_TIMEOUT=120`，`STREAMING_TIMEOUT=60` |
| 长推理/代码生成 | `RELAY_TIMEOUT=0`，`STREAMING_TIMEOUT=600` |
| 严格 SLA 要求 | `RELAY_TIMEOUT=30`，`STREAMING_TIMEOUT=30` |

**`RELAY_TIMEOUT` 与 `STREAMING_TIMEOUT` 的区别：**
- `RELAY_TIMEOUT`：整个请求（含等待首字节）的超时
- `STREAMING_TIMEOUT`：流式传输过程中，两次数据块之间的最大间隔；超时后断开连接

---

## 6. 限流

系统内置 4 组独立限流，均支持启用/禁用、请求数、时间窗口三项配置。

### API 请求限流

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `GLOBAL_API_RATE_LIMIT_ENABLE` | `true` | 是否启用全局 API 限流 |
| `GLOBAL_API_RATE_LIMIT` | `180` | 时间窗口内最大请求数 |
| `GLOBAL_API_RATE_LIMIT_DURATION` | `180` | 时间窗口长度（秒）|

### Web 页面限流

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `GLOBAL_WEB_RATE_LIMIT_ENABLE` | `true` | 是否启用全局 Web 限流 |
| `GLOBAL_WEB_RATE_LIMIT` | `60` | 时间窗口内最大请求数 |
| `GLOBAL_WEB_RATE_LIMIT_DURATION` | `180` | 时间窗口长度（秒）|

### 关键操作限流（注册、密码重置等）

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `CRITICAL_RATE_LIMIT_ENABLE` | `true` | 是否启用关键操作限流 |
| `CRITICAL_RATE_LIMIT` | `20` | 时间窗口内最大请求数 |
| `CRITICAL_RATE_LIMIT_DURATION` | `1200` | 时间窗口长度（秒，默认 20 分钟）|

### 搜索限流

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `SEARCH_RATE_LIMIT_ENABLE` | `true` | 是否启用搜索限流 |
| `SEARCH_RATE_LIMIT` | `10` | 时间窗口内最大请求数 |
| `SEARCH_RATE_LIMIT_DURATION` | `60` | 时间窗口长度（秒）|

> 设置 `*_ENABLE=false` 可关闭对应限流，适用于内网环境或压测时临时禁用。

---

## 7. 任务处理

任务处理主要面向 Midjourney、异步绘图等需要轮询状态的任务类型。

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `UPDATE_TASK` | `true` | 启用任务后台轮询。`NODE_TYPE=slave` 时自动禁用 |
| `TASK_QUERY_LIMIT` | `1000` | 每次轮询最多查询的任务数 |
| `TASK_TIMEOUT_MINUTES` | `1440` | 任务超时分钟数（默认 24 小时）；超时后标记失败并自动退款 |
| `TASK_PRICE_PATCH` | （空）| 任务定价补丁，逗号分隔列表 |

---

## 8. 渠道与模型同步

### 渠道自动更新

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `CHANNEL_UPDATE_FREQUENCY` | （空，禁用）| 渠道自动更新频率（秒）；非空则定期刷新渠道状态 |
| `CHANNEL_TEST_FREQUENCY` | （空，禁用）| 渠道测试频率（分钟）；定期发送测试请求检测渠道可用性 |
| `CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_ENABLED` | `true` | 启用上游模型列表自动同步 |
| `CHANNEL_UPSTREAM_MODEL_UPDATE_TASK_INTERVAL_MINUTES` | `30` | 上游模型列表同步间隔（分钟）|
| `CHANNEL_UPSTREAM_MODEL_UPDATE_MIN_CHECK_INTERVAL_SECONDS` | `600` | 单个渠道的最短检查间隔（秒），防止过于频繁 |

### 模型元数据同步

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `SYNC_UPSTREAM_BASE` | `https://basellm.github.io/llm-metadata` | 模型/供应商元数据的同步基础 URL |
| `SYNC_HTTP_TIMEOUT_SECONDS` | `10`–`15` | 元数据同步 HTTP 超时（秒）|
| `SYNC_HTTP_RETRY` | `3` | 元数据同步失败重试次数 |
| `SYNC_HTTP_MAX_MB` | `10` | 元数据同步响应最大大小（MB）|

---

## 9. AI 模型行为

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `GEMINI_SAFETY_SETTING` | `BLOCK_NONE` | Gemini 安全过滤级别 |
| `GEMINI_VISION_MAX_IMAGE_NUM` | `16` | Gemini 单次请求最大图片数量 |
| `COHERE_SAFETY_SETTING` | `NONE` | Cohere API 安全设置 |
| `GET_MEDIA_TOKEN` | `true` | 在 token 计费中统计图片/媒体 token |
| `GET_MEDIA_TOKEN_NOT_STREAM` | `false` | 在非流式（`stream=false`）场景下也统计媒体 token |
| `DIFY_DEBUG` | `true` | 向客户端输出 Dify 工作流和节点信息，便于调试 |
| `FORCE_STREAM_OPTION` | `true` | 强制在响应中返回用量（usage）信息，即使客户端未请求 |
| `AZURE_DEFAULT_API_VERSION` | `2025-04-01-preview` | Azure OpenAI 默认 API 版本 |
| `OPENROUTER_CONFIG_PATH` | `config/openrouter.yaml` | OpenRouter 模型配置文件路径 |

**`GET_MEDIA_TOKEN` 与 `GET_MEDIA_TOKEN_NOT_STREAM` 组合：**

| 配置 | 效果 |
|---|---|
| `GET_MEDIA_TOKEN=true` + `GET_MEDIA_TOKEN_NOT_STREAM=false` | 仅流式请求统计图片 token（默认）|
| `GET_MEDIA_TOKEN=true` + `GET_MEDIA_TOKEN_NOT_STREAM=true` | 所有请求都统计图片 token |
| `GET_MEDIA_TOKEN=false` | 完全不统计图片 token |

---

## 10. 文件与请求体

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `MAX_FILE_DOWNLOAD_MB` | `64` | 单个文件下载最大体积（MB）|
| `MAX_REQUEST_BODY_MB` | `128` | 请求体解压后最大体积（MB），防止 zip bomb 导致 OOM |
| `STREAM_SCANNER_MAX_BUFFER_MB` | `128` | 流式响应扫描器缓冲区最大值（MB）|

---

## 11. 安全与认证

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `SESSION_SECRET` | `random_string` | Session 加密密钥，**生产环境必须修改为随机长字符串** |
| `CRYPTO_SECRET` | （继承 `SESSION_SECRET`）| 敏感数据加密密钥，建议单独设置 |
| `WISEMODEL_API_TOKEN` | （空）| WiseModel 外部 API 的 Bearer Token 认证 |
| `TLS_INSECURE_SKIP_VERIFY` | `false` | 跳过 TLS 验证，**仅用于本地测试** |
| `TRUSTED_REDIRECT_DOMAINS` | （空）| 允许支付回调重定向的可信域名 |
| `NOTIFY_LIMIT_COUNT` | `2` | 通知发送频率上限（次数）|
| `NOTIFICATION_LIMIT_DURATION_MINUTE` | `10` | 通知频率限制窗口（分钟）|

---

## 12. 分析统计

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `UMAMI_WEBSITE_ID` | （空，禁用）| Umami Analytics 网站 ID；**非空则启用** |
| `UMAMI_SCRIPT_URL` | `https://analytics.umami.is/script.js` | Umami 统计脚本地址，自托管时需修改 |
| `GOOGLE_ANALYTICS_ID` | （空，禁用）| Google Analytics 4 Measurement ID；**非空则启用** |

---

## 13. OAuth 与外部服务

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `LINUX_DO_TOKEN_ENDPOINT` | `https://connect.linux.do/oauth2/token` | Linux DO OAuth Token 端点 |
| `LINUX_DO_USER_ENDPOINT` | `https://connect.linux.do/api/user` | Linux DO OAuth 用户信息端点 |

---

## 14. 初始化

| 变量名 | 默认值 | 说明 |
|---|---|---|
| `GENERATE_DEFAULT_TOKEN` | `false` | 启动时自动为默认用户生成 API Token |
| `POLLING_INTERVAL` | `0`（禁用）| 请求轮询间隔（秒）|

---

## 15. 组合配置方案

### 方案 A：本地开发

```env
GIN_MODE=debug
DEBUG=true
ENABLE_PPROF=true
SQLITE_PATH=./dev.db
SESSION_SECRET=dev-only-secret
BATCH_UPDATE_ENABLED=true
BATCH_UPDATE_INTERVAL=5
RELAY_TIMEOUT=0
STREAMING_TIMEOUT=300
```

### 方案 B：生产单节点（MySQL + Redis）

```env
# 应用
PORT=3000
SESSION_SECRET=<随机32字节字符串>
CRYPTO_SECRET=<随机32字节字符串>

# 数据库
SQL_DSN=user:password@tcp(127.0.0.1:3306)/new_api?parseTime=true
LOG_SQL_DSN=user:password@tcp(127.0.0.1:3306)/new_api_log?parseTime=true
SQL_MAX_IDLE_CONNS=50
SQL_MAX_OPEN_CONNS=200
SQL_MAX_LIFETIME=300
ERROR_LOG_ENABLED=true

# 缓存
REDIS_CONN_STRING=redis://localhost:6379/0
SYNC_FREQUENCY=30
BATCH_UPDATE_ENABLED=true
BATCH_UPDATE_INTERVAL=5

# 超时
RELAY_TIMEOUT=300
STREAMING_TIMEOUT=300
```

### 方案 C：生产多节点（主+从）

**主节点：**

```env
NODE_TYPE=master
SESSION_SECRET=<共享密钥，所有节点一致>
SQL_DSN=user:password@tcp(db-host:3306)/new_api?parseTime=true
REDIS_CONN_STRING=redis://redis-host:6379/0
SYNC_FREQUENCY=30
UPDATE_TASK=true
CHANNEL_UPDATE_FREQUENCY=300
```

**从节点：**

```env
NODE_TYPE=slave
SESSION_SECRET=<与主节点相同>
SQL_DSN=user:password@tcp(db-host:3306)/new_api?parseTime=true
REDIS_CONN_STRING=redis://redis-host:6379/0
SYNC_FREQUENCY=30
# 从节点不运行任务轮询和渠道更新，自动禁用
```

> 多节点部署中，`SESSION_SECRET`、`CRYPTO_SECRET` 和 `SQL_DSN`、`REDIS_CONN_STRING` 必须在所有节点保持一致。

### 方案 D：启用持续性能分析（Pyroscope）

```env
PYROSCOPE_URL=http://pyroscope.internal:4040
PYROSCOPE_APP_NAME=new-api
PYROSCOPE_BASIC_AUTH_USER=admin
PYROSCOPE_BASIC_AUTH_PASSWORD=secret
HOSTNAME=node-1
PYROSCOPE_MUTEX_RATE=5
PYROSCOPE_BLOCK_RATE=5
ENABLE_PPROF=true
```

---

## 快速检查清单（生产上线前）

- [ ] `SESSION_SECRET` 已修改为随机长字符串
- [ ] `CRYPTO_SECRET` 已单独设置
- [ ] `SQL_DSN` 已配置（MySQL/PostgreSQL）
- [ ] `TLS_INSECURE_SKIP_VERIFY` 为 `false`（默认）
- [ ] `GIN_MODE` 未设置为 `debug`
- [ ] `DEBUG` 为 `false`（默认）
- [ ] 多节点时所有节点 `SESSION_SECRET` 一致
