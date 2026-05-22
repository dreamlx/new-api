# HappyHorse 视频兼容影响分析报告

日期：2026-05-21

## 1. 结论

本次 HappyHorse 视频兼容改动整体影响面可控，主要收敛在 HappyHorse 新增渠道、HappyHorse 模型前缀、HappyHorse 原生接口路径，以及既有视频任务通用入口。

未发现会明显破坏既有渠道或既有接口的高风险问题。需要重点关注的既有影响点是：Claude 文件 MIME 处理增强、任务计费完成后补差链路、视频任务查询响应格式分流。

## 2. 改动范围

### 2.1 新增 HappyHorse 渠道

新增渠道类型：

```text
ChannelTypeHappyHorse = 59
APITypeHappyHorse = 59
```

涉及文件：

- `constant/channel.go`
- `constant/api_type.go`
- `common/api_type.go`
- `relay/relay_adaptor.go`
- `middleware/distributor.go`
- `web/src/constants/channel.constants.js`
- `web/src/helpers/render.jsx`

影响说明：

- 新增渠道编号 59，未复用历史渠道编号。
- 前端渠道下拉新增 HappyHorse 名称和图标。
- 默认上游地址为 `https://dashscope.aliyuncs.com`。
- 既有渠道编号、名称、默认地址不变。

### 2.2 新增 HappyHorse 任务适配器

新增目录：

```text
relay/channel/happyhorse/
relay/channel/task/happyhorse/
```

核心职责：

- 支持 HappyHorse 视频任务提交。
- 支持 HappyHorse 任务状态查询。
- 支持 `/happyhorse/api/generate` 原生接口。
- 支持 `/happyhorse/api/status/:task_id` 原生查询接口。
- 支持 `/v1/video/generations` 通用视频任务入口转换到 HappyHorse 上游格式。
- 支持 `/v1/video/generations/{task_id}` 查询 HappyHorse 任务并返回 new-api 通用格式。

影响说明：

- HappyHorse 非任务类 relay 请求会返回不支持。
- 通用聊天、图片、音频、Embedding、Rerank 等旧接口不会被 HappyHorse 适配器接管。

## 3. 接口影响

### 3.1 新增接口

新增用户可见接口：

```http
POST /happyhorse/api/generate
GET /happyhorse/api/status/:task_id
```

影响说明：

- 两个接口均走 new-api Token 鉴权和渠道分发。
- 查询接口使用 new-api 自己生成的公开 `task_id`。
- 查询返回 HappyHorse 原生风格 JSON。

### 3.2 复用既有接口

继续支持既有通用视频接口：

```http
POST /v1/video/generations
GET /v1/video/generations/{task_id}
```

影响说明：

- 请求模型为 `happyhorse-*` 时，走 HappyHorse 渠道。
- 返回仍保持 new-api/OpenAI 风格视频任务格式。
- 非 HappyHorse 模型仍按原有渠道分发，不受 HappyHorse 原生响应格式影响。

### 3.3 查询格式分流

状态查询响应分流逻辑：

- `/happyhorse/api/status/:task_id` 返回 HappyHorse 原生格式。
- `/v1/video/generations/{task_id}` 返回 new-api 通用视频任务格式。

影响文件：

```text
relay/relay_task.go
```

影响说明：

- HappyHorse 原生状态格式只在路径以 `/happyhorse/api/status` 开头时生效。
- 旧的 `/v1/video/generations/{task_id}` 查询不受该分支影响。

## 4. 模型影响

支持模型：

```text
happyhorse-1.0-t2v
happyhorse-1.0-i2v
happyhorse-1.0-r2v
happyhorse-1.0-video-edit
```

已清除的历史模型：

```text
happyhorse-1.0/video
```

影响说明：

- Go 源码中不再支持 `happyhorse-1.0/video`。
- 如果数据库渠道配置里仍保留该模型，需要在管理后台手动删除。
- 使用该旧模型名请求会返回 `unsupported model`。

## 5. 计费影响

### 5.1 预扣逻辑

HappyHorse 沿用 new-api 统一预扣逻辑：

```text
预扣额度 = 模型价格 * 分组倍率 * 时长 * 分辨率倍率 * QuotaPerUnit
```

默认模型价格：

```text
happyhorse-1.0-t2v        0.9
happyhorse-1.0-i2v        0.9
happyhorse-1.0-r2v        0.9
happyhorse-1.0-video-edit 0.9
```

分辨率倍率：

```text
720P  = 1.0
1080P = 1.6 / 0.9
```

### 5.2 完成后补差

HappyHorse 模型在 `controller/relay.go` 中排除 `PerCallBilling`：

```text
!strings.HasPrefix(relayInfo.OriginModelName, "happyhorse-")
```

目的：

- 让 HappyHorse 在任务完成轮询时走 `AdjustBillingOnComplete`。
- 按上游返回的 `usage.output_video_duration` 重新计算实际额度。
- 视频编辑只按输出秒数计费，不按输入视频秒数计费。

影响说明：

- 该例外仅对 `happyhorse-` 模型前缀生效。
- Sora、Gemini、Ali、Doubao、Midjourney 等既有任务模型不会命中该例外。

### 5.3 任务表 quota 字段

现有框架行为：

- 任务提交时 `tasks.quota` 记录预扣额度。
- 完成后补差会调整用户额度、渠道额度、日志流水。
- `tasks.quota` 不一定回写为最终实际消耗。

影响说明：

- 这是任务框架既有行为，不是 HappyHorse 单独引入。
- HappyHorse 的实际资金调整以用户额度、渠道额度和日志流水为准。

## 6. 共享代码影响

### 6.1 `middleware/distributor.go`

新增 `/happyhorse/api/` 分支。

影响说明：

- POST 请求从 body 中读取 model 并选择渠道。
- GET 查询不重新选择渠道，符合任务查询既有模式。
- 不影响 `/mj/`、`/suno/`、`/v1/videos`、`/v1/video/generations` 旧路径判断。

### 6.2 `relay/relay_adaptor.go`

新增 HappyHorse 普通 adaptor 和 task adaptor 注册。

影响说明：

- 普通 adaptor 仅用于渠道名称、模型列表和不支持提示。
- 真实视频任务由 task adaptor 处理。

### 6.3 `relay/relay_task.go`

新增 HappyHorse 原生状态响应转换。

影响说明：

- 只对 `/happyhorse/api/status` 路径生效。
- 其他任务查询响应格式不变。

### 6.4 `common/redis.go`

`RedisDel` 和 `RedisDelKey` 增加 `RDB == nil` 守卫。

影响说明：

- Redis 正常启用时行为不变。
- Redis 未初始化的测试或特殊环境中避免 panic。

### 6.5 `controller/wisemodel_package.go`

资源包 Redis 初始化增加 `common.RDB != nil` 守卫。

影响说明：

- 正常 Redis 环境行为不变。
- 无 Redis 环境避免 nil pointer panic。

### 6.6 `relay/helper/stream_scanner.go`

`StreamStatus` 改为仅在 nil 时初始化。

影响说明：

- 保留调用方已记录的 stream 错误状态。
- 旧逻辑会无条件覆盖已有 `StreamStatus`，该改动属于修复型。

### 6.7 `relay/channel/claude/relay-claude.go`

增强 Claude 文件 MIME 处理：

- 根据文件名扩展名推导 MIME。
- `text/*` 转为 Claude text。
- `application/pdf` 转为 Claude document。
- `image/*` 转为 Claude image。
- 其他不支持类型跳过。

影响说明：

- 这是对既有 Claude 文件输入行为的真实改变。
- 预期改善文本文件、PDF、图片文件的 Claude 转换一致性。
- 不支持的二进制文件会跳过，而不是误当图片发送。

## 7. 数据库影响

本次改动未新增数据库表、字段或迁移。

需要管理员配置的内容：

- 新增 HappyHorse 渠道。
- 渠道类型选择 `HappyHorse`。
- API 地址使用 `https://dashscope.aliyuncs.com`。
- 模型列表只保留四个内部模型：
  - `happyhorse-1.0-t2v`
  - `happyhorse-1.0-i2v`
  - `happyhorse-1.0-r2v`
  - `happyhorse-1.0-video-edit`
- 删除历史模型 `happyhorse-1.0/video`。

## 8. 前端影响

前端新增：

- HappyHorse 渠道选项。
- HappyHorse 渠道图标。

影响说明：

- 只影响渠道管理页面的展示和选择。
- 不改变既有渠道的配置表单结构。

## 9. 风险点

### 9.1 已识别低风险点：R2V metadata.media 校验较轻

`/v1/video/generations` 的 R2V 请求允许通过 `metadata.media` 传递 HappyHorse media 结构。

当前校验只检查是否存在 `metadata.media`，更深层的结构解析在请求体构建阶段完成。

影响：

- 如果用户传入格式错误的 `metadata.media`，错误可能发生在 BuildRequestBody 阶段。
- 常规推荐用法是使用 `images[]`，不依赖 `metadata.media`。

建议：

- 若后续希望错误更早返回 400，可在 `validateHappyHorseTaskRequest` 中提前解析并校验 `metadata.media`。

### 9.2 管理后台残留旧模型

如果数据库已有渠道配置中仍存在 `happyhorse-1.0/video`，前端可能还能显示该历史模型。

影响：

- 请求该模型会被后端拒绝。

建议：

- 管理员手动清理渠道模型列表。
- 只配置四个当前支持模型。

### 9.3 Claude MIME 行为变更

Claude 文件输入的处理逻辑被增强，属于共享代码改动。

影响：

- 文本文件会作为 text 内容发送。
- PDF 会作为 document 内容发送。
- 图片作为 image 内容发送。
- 未知二进制类型跳过。

建议：

- 合并前保留 Claude 文件转换相关回归测试。

## 10. 验证结果

本轮影响面验证命令：

```bash
go test ./relay/channel/task/happyhorse ./relay/channel/happyhorse ./service ./model -count=1
```

结果：

```text
ok github.com/QuantumNous/new-api/relay/channel/task/happyhorse
ok github.com/QuantumNous/new-api/service
ok github.com/QuantumNous/new-api/model
```

```bash
go test ./relay/helper ./common -count=1
```

结果：

```text
ok github.com/QuantumNous/new-api/relay/helper
ok github.com/QuantumNous/new-api/common
```

```bash
go vet ./relay/channel/task/happyhorse/... ./relay/channel/happyhorse/...
```

结果：

```text
无输出，检查通过
```

## 11. 提交建议

建议提交范围：

1. HappyHorse 功能代码与测试：
   - `relay/channel/happyhorse/`
   - `relay/channel/task/happyhorse/`
   - 路由、分发、渠道常量、API 类型、计费注册、前端渠道展示

2. 配套修复：
   - Redis nil 守卫
   - Wisermodel RDB nil 守卫
   - StreamStatus 保留
   - Claude 文件 MIME 处理增强

3. HappyHorse 文档：
   - `docs/happyhorse/`

不建议提交：

- 临时接口测试 JSON
- `tmp-happyhorse-*`
- `tmp-new-api-bin`
- 与 HappyHorse 主题无关的文档或实验文件

