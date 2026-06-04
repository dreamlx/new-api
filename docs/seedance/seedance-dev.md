# Seedance 2.0 渠道开发文档

更新时间：2026-06-04

本文记录当前 Seedance 2.0 渠道的最终开发口径。历史讨论中的旧规则以本文为准。

## 1. 目标

新增 Seedance 2.0 独立视频任务渠道，支持两类入口：

```http
POST /v1/video/generations
GET  /v1/video/generations/{task_id}

POST /api/v3/contents/generations/tasks
GET  /api/v3/contents/generations/tasks/{task_id}
```

两类入口都走 new-api 现有鉴权、渠道分发、任务记录、计费预扣、完成结算和失败退款链路。对外只暴露 new-api 公开 `task_id`，不暴露上游任务 ID。

## 2. 当前路由

### OpenAI Video 风格

```http
POST /v1/video/generations
GET  /v1/video/generations/{task_id}
```

请求格式：

```json
{
  "model": "dreamina-seedance-2-0-fast-260128",
  "prompt": "A small kitten chasing a butterfly in a sunny garden",
  "duration": 5,
  "size": "720p",
  "images": ["https://example.com/first.png"],
  "videos": ["https://example.com/source.mp4"],
  "audios": ["https://example.com/voice.mp3"]
}
```

### 火山官方兼容风格

```http
POST /api/v3/contents/generations/tasks
GET  /api/v3/contents/generations/tasks/{task_id}
```

请求格式：

```json
{
  "model": "dreamina-seedance-2-0-fast-260128",
  "content": [
    {
      "type": "text",
      "text": "cinematic pan --rs 720p --dur 5"
    },
    {
      "type": "image_url",
      "image_url": {
        "url": "https://example.com/first.png"
      },
      "role": "first_frame"
    }
  ]
}
```

查询返回原始上游 `ResponseTask` 格式（不做任何转换），包含 `status`、`content.video_url`、`usage`、`duration`、`resolution`、`framespersecond` 等字段。

## 3. 支持模型

| 模型 | 能力 | 支持分辨率 |
| --- | --- | --- |
| `dreamina-seedance-2-0-260128` | 文生/图生/视频续写/音频驱动 | 480p / 720p / 1080p |
| `dreamina-seedance-2-0-fast-260128` | 文生/图生/视频续写/音频驱动 | 480p / 720p |
| `doubao-seedance-2-0-260128` | 同上（火山官方模型名兼容） | 480p / 720p / 1080p |
| `doubao-seedance-2-0-fast-260128` | 同上（火山官方模型名兼容） | 480p / 720p |

Fast 版本不支持 1080p。如需拒绝 fast + 1080p 的请求，应在请求校验阶段处理；如果由上游拒绝，失败任务继续走现有退款逻辑。

## 4. 参数契约

| 参数 | 规则 |
| --- | --- |
| `size` / `resolution` | 支持 `480p`、`720p`、`1080p`；fast 版仅 `480p`、`720p`；缺省 `720p` |
| `duration` | 支持 `5`、`10`；缺省 `5` |
| `seed` | 整数，透传上游 |
| `images` | URL 数组，支持图生视频 / 首尾帧 / 参考图 |
| `videos` | URL 数组，支持视频续写 |
| `audios` | URL 数组，支持音频驱动 |
| `prompt` | 必填，可包含 `[Image N]` / `[Video N]` / `[Audio N]` 多模态标记 |
| `ratio` | 画幅比，如 `"16:9"`、`"9:16"`，透传上游 |
| `return_last_frame` | bool，透传上游 |
| `generate_audio` | bool，透传上游 |
| `draft` | bool，透传上游 |
| `camera_fixed` | bool，透传上游 |
| `watermark` | bool，透传上游 |
| `frames` | int，透传上游 |
| `callback_url` | string，透传上游（网关自行轮询，通常不需要） |
| `service_tier` | string，透传上游 |
| `execution_expires_after` | int，透传上游 |
| `tools` | array，透传上游 |

高级参数的透传方式见第 5.3 节。

## 5. 双入口请求转换

### 5.1 OpenAI Video 风格 → 火山 content[] 转换

| OpenAI 风格字段 | 火山 content[] 字段 |
| --- | --- |
| `model` | `model` |
| `prompt` | `content[]` 中 `type=text` |
| `images[]` | `content[]` 中 `type=image_url` |
| `videos[]` | `content[]` 中 `type=video`（上游格式 `video_url`） |
| `audios[]` | `content[]` 中 `type=audio`（上游格式 `audio_url`） |
| `duration` / `seconds` | `duration` |
| `size` | `resolution` |
| `seed` | `seed` |
| `metadata` | 全量透传至上游（见 5.3 节） |

转换发生在 `seedance/adaptor.go` 的 `convertToRequestPayload` 方法中。

### 5.2 高级参数透传

两种入口格式均支持全量透传上游 `volcano.RequestPayload` 的所有字段：

**OpenAI Video 格式**：通过 `metadata` 字段传递，`UnmarshalMetadata` 自动 overlay 到 `volcano.RequestPayload`。

```json
{
  "model": "dreamina-seedance-2-0-260128",
  "prompt": "...",
  "size": "720p",
  "metadata": {
    "ratio": "9:16",
    "watermark": false,
    "generate_audio": true
  }
}
```

**Volcano content[] 格式**：直接在请求体顶层传入，原始 JSON 字段自动透传到 `Metadata`，再由 `UnmarshalMetadata` overlay。

```json
{
  "model": "dreamina-seedance-2-0-260128",
  "content": [{"type": "text", "text": "..."}],
  "resolution": "720p",
  "ratio": "9:16",
  "watermark": false,
  "generate_audio": true
}
```

支持透传的上游参数：

| 参数 | 类型 | 说明 |
| --- | --- | --- |
| `ratio` | string | 画幅比（`16:9`、`9:16`、`1:1`） |
| `callback_url` | string | 回调地址 |
| `return_last_frame` | bool | 返回最后一帧 |
| `generate_audio` | bool | 生成音频 |
| `draft` | bool | 草稿模式 |
| `camera_fixed` | bool | 固定摄像头 |
| `watermark` | bool | 水印 |
| `frames` | int | 帧数 |
| `service_tier` | string | 服务层级 |
| `execution_expires_after` | int | 执行超时时间 |
| `tools` | array | 工具调用 |

透传机制：

1. OpenAI Video 格式：`metadata` → `TaskSubmitReq.Metadata` → `UnmarshalMetadata` → `volcano.RequestPayload`
2. Volcano content[] 格式：原始 JSON 顶层字段 → `TaskSubmitReq.Metadata`（排除 `content` 和 `model`）→ `UnmarshalMetadata` → `volcano.RequestPayload`

`content` 字段不直接透传，因为输入格式（`video`/`audio` 嵌套结构）与上游格式（`video_url`/`audio_url`）不同，需在 `convertVolcanoContentToTaskSubmit` 中转换。`model` 字段不透传以防止计费绕过。

### 5.3 Overlay 优先级

`UnmarshalMetadata` 将 Metadata 中的字段 overlay 到 `volcano.RequestPayload` 后，`convertToRequestPayload` 中的显式映射会**覆盖** overlay 结果。因此：

**显式字段优先于 metadata 中的同名字段。**

例如，如果 OpenAI Video 格式同时传入 `size: "720p"` 和 `metadata.resolution: "1080p"`：

1. `UnmarshalMetadata` 将 `metadata.resolution` overlay 到 `r.Resolution = "1080p"`
2. 随后 `r.Resolution = req.Size`（即 `"720p"`）覆盖 overlay 结果
3. 最终 `r.Resolution = "720p"`

优先级链（后覆盖前）：

```text
UnmarshalMetadata overlay → 显式字段映射（Size→Resolution, Duration, Seed, Prompt）
```

此设计确保客户端通过标准字段（`size`、`duration`、`seed`）传入的值始终优先，`metadata` 中的同名字段作为补充。如果 `metadata` 中包含了请求体中不存在的上游参数（如 `ratio`、`watermark`），则 overlay 不被覆盖，正常透传。

### 5.4 content[] 中 role 字段

`image_url` 类型条目支持 `role` 字段，用于指定图片在生成中的角色：

| role 值 | 说明 |
| --- | --- |
| `first_frame` | 指定该图片为视频首帧 |
| `last_frame` | 指定该图片为视频尾帧 |
| 空 / 省略 | 通用参考图，由上游决定如何使用 |

`role` 字段仅对 `image_url` 类型有意义，对 `text`、`video`、`audio` 类型无意义。

代码不做 role 值枚举校验，原样透传上游。未来上游如新增 role 值，无需改代码。

`return_last_frame` 顶层参数与 `role: "last_frame"` 不同：前者控制**响应**是否包含最后一帧图片 URL，后者提供**输入**的目标尾帧图片。

## 6. 错误处理

### 6.1 提交阶段错误

提交失败返回 `dto.TaskError` 格式，包含 `code`、`message`、`data` 三个 JSON 字段。HTTP 状态码由 `TaskError.StatusCode` 决定。

| 场景 | HTTP 状态码 | code | 说明 |
| --- | --- | --- | --- |
| JSON 解析失败 | 400 | `invalid_request` | 请求体不是合法 JSON |
| prompt 缺失 | 400 | `invalid_request` | 必填字段缺失 |
| 模型未配置价格 | 400 | `model_price_error` | 未配置 model_ratio |
| 上游返回非 200 | 原样转发 | `fail_to_fetch_task` | 错误消息为上游原始响应 |
| 上游限流 | 429 | `rate_limit_exceeded` | 分组上游负载饱和 |
| 读取请求体失败 | 500 | `read_body_failed` | 内部错误 |
| 解析上游响应失败 | 500 | `unmarshal_response_body_failed` | 上游返回非预期格式 |
| 上游响应缺少 task ID | 500 | `invalid_response` | 上游返回格式异常 |

### 6.2 查询阶段错误

任务失败时，查询响应中包含上游错误信息，原样转发不修改：

- OpenAI Video 风格（`/v1/video/generations/{task_id}`）：`data.data.error.code` / `data.data.error.message`
- 火山兼容风格（`/api/v3/contents/generations/tasks/{task_id}`）：`error.code` / `error.message`

上游错误码示例：

| 上游错误码 | 说明 |
| --- | --- |
| `OutputVideoSensitiveContentDetected.PolicyViolation` | 输出内容审核拒绝（常见于受版权保护的图片输入） |
| `InputValidationError` | 输入参数校验失败 |
| `QuotaExceeded` | 上游配额不足 |

### 6.3 重试行为

new-api 在以下情况自动尝试其他渠道重试：

- 上游返回 429（限流）
- 上游返回 5xx（服务器错误，504/524 除外）

以下情况**不会**重试：

- 本地验证失败（`LocalError = true`），如 prompt 缺失、JSON 格式错误、计费配置错误
- 上游返回 400（客户端错误）
- 上游返回 408（Azure 超时）

### 6.4 敏感信息脱敏

如果错误消息中包含 `post`、`dial`、`http` 等关键词（可能泄露上游 URL 或凭据），系统会调用 `common.MaskSensitiveInfo` 脱敏后再返回给客户端。原始错误信息仅记录在服务端日志中。

## 7. 火山 content[] 格式

火山官方兼容入口尽量原样透传。校验 `model` 和 `content[]` 后，保留 `content[]` 原结构，只补必要的模型映射、默认值和计费字段提取。

不先转成 OpenAI 风格请求，避免无意义的中间结构。

`content[]` 中支持的 type：

| type | 说明 | 字段 | role 可选值 |
| --- | --- | --- | --- |
| `text` | 文本提示词 | `text` | 不适用 |
| `image_url` | 图片 | `image_url.url` | `first_frame`（首帧）、`last_frame`（尾帧）、空（参考图） |
| `video` | 视频（输入格式） | `video.url` | 不适用 |
| `audio` | 音频 | `audio.url` | 不适用 |

注意：输入格式的 `video` 和 `audio` 使用嵌套结构（`video.url`），上游格式使用 `video_url` 和 `audio_url` 的扁平结构。转换在 `convertToRequestPayload` 中完成。

## 8. 上游请求

Seedance task adaptor 对上游统一使用火山官方兼容接口：

```http
POST {base_url}/api/v3/contents/generations/tasks
GET  {base_url}/api/v3/contents/generations/tasks/{upstream_task_id}
```

请求头：

```http
Content-Type: application/json
Accept: application/json
Authorization: Bearer {channel_key}
```

上游响应包含 `duration`、`resolution`、`framespersecond`、`usage`、`content.video_url` 等结算和展示需要的字段。

## 9. 代码结构

```
relay/channel/seedance/adaptor.go          — 普通 channel adaptor（仅注册，不支持非任务请求）
relay/channel/task/seedance/
  adaptor.go      — 任务 adaptor：Init, BuildRequestURL, BuildRequestHeader,
                    EstimateBilling, BuildRequestBody, DoRequest, DoResponse,
                    FetchTask, ParseTaskResult, ConvertToOpenAIVideo,
                    convertToRequestPayload, calculateCombinedRatio,
                    normalizeResolution
  constants.go    — 模型名、分辨率常量、条件倍率映射、status 常量（re-export）
  validate.go     — 双入口请求校验和转换（ValidateRequestAndSetAction,
                    convertVolcanoContentToTaskSubmit）
relay/channel/task/taskcommon/volcano/
  volcano.go      — 共享 Volcano 兼容 DTO 和辅助方法（见第 8 节）
```

## 10. volcano 共享包

从 doubao 和 seedance adaptor 中提取的共享 Volcano 兼容代码，位于 `relay/channel/task/taskcommon/volcano/volcano.go`。

### 10.1 提取原因

doubao 和 seedance 的上游都是火山兼容接口（`/api/v3/contents/generations/tasks`），DTO 结构、URL 构建、请求头、FetchTask、ParseSubmitResponse、ParseTaskResult 完全相同，属于 DRY 违规。

### 10.2 共享内容

| 组件 | 说明 |
| --- | --- |
| `ContentItem` / `MediaURL` | 上游 content[] 条目 DTO |
| `RequestPayload` | 上游提交请求 DTO |
| `ResponsePayload` | 上游提交响应 DTO（含 `id` 即 upstream task_id） |
| `ResponseTask` | 上游任务状态响应 DTO（含 `status`, `content.video_url`, `usage` 等） |
| `Status*` 常量 | `pending`, `queued`, `processing`, `running`, `succeeded`, `failed` |
| `BuildTaskURL` | 构建提交 URL |
| `BuildFetchURL` | 构建查询 URL |
| `SetCommonHeaders` | 设置 Content-Type、Accept、Authorization |
| `FetchTask` | 执行上游任务状态查询 |
| `ParseSubmitResponse` | 解析提交响应，提取 upstream task_id |
| `ParseTaskResult` | 解析任务状态响应，映射到 `TaskInfo`，含 token 优先级逻辑 |
| `HasVideoInMetadata` | 检测 metadata content[] 中是否包含视频条目 |

### 10.3 对 doubao 渠道的影响

doubao adaptor 已改写为使用 volcano 共享包。行为变化：

| 变化点 | 旧行为 | 新行为 | 影响 |
| --- | --- | --- | --- |
| `ParseTaskResult` | 无条件同时设 `CompletionTokens` 和 `TotalTokens` | 优先 `completion_tokens`，fallback `total_tokens` | 结果一致或更准确 |
| `HasVideoInMetadata` | 只检查 `type=video_url` | 额外检查 `type=video`（超集） | 覆盖更多视频类型，不会漏检 |

两处变化都是改进性质，不会导致少计费或错计费。

## 11. 返回格式

### `/v1/video/generations` 创建响应

```json
{
  "id": "task_xxx",
  "task_id": "task_xxx",
  "object": "video",
  "model": "dreamina-seedance-2-0-fast-260128",
  "status": "queued",
  "progress": 0,
  "created_at": 1779348046
}
```

### `/v1/video/generations/{task_id}` 查询响应

通用 TaskDto 格式：

```json
{
  "code": "success",
  "message": "",
  "data": {
    "task_id": "task_xxx",
    "status": "SUCCESS",
    "progress": "100%",
    "result_url": "https://ark-acg-ap-southeast-1.tos-ap-southeast-1.volces.com/...mp4?...",
    "data": {
      "id": "cgt-...",
      "status": "succeeded",
      "duration": 5,
      "resolution": "720p",
      "ratio": "16:9",
      "framespersecond": 24,
      "usage": {
        "completion_tokens": 108900,
        "total_tokens": 108900
      },
      "content": {
        "video_url": "https://ark-acg-ap-southeast-1.tos-ap-southeast-1.volces.com/...mp4?..."
      }
    }
  }
}
```

### `/api/v3/contents/generations/tasks/{task_id}` 查询响应

直接返回原始上游 `ResponseTask` JSON（`originTask.Data`）：

```json
{
  "id": "cgt-...",
  "model": "dreamina-seedance-2-0-260128",
  "status": "succeeded",
  "duration": 5,
  "resolution": "720p",
  "ratio": "16:9",
  "framespersecond": 24,
  "seed": 0,
  "content": {
    "video_url": "https://ark-acg-ap-southeast-1.tos-ap-southeast-1.volces.com/...mp4?..."
  },
  "usage": {
    "completion_tokens": 108900,
    "total_tokens": 108900
  },
  "created_at": 1779348046,
  "updated_at": 1779348302
}
```

`ratio` 字段为输出视频画幅比（如 `"16:9"`、`"9:16"`、`"1:1"`），由上游根据输入自动决定。

### `/v1/videos/{task_id}` 查询响应

OpenAI Video API 格式，通过 `seedance.ConvertToOpenAIVideo()` 生成。包含 usage、duration、resolution、framespersecond 等元数据。

## 12. 查询行为

所有查询接口读本地任务数据库，不实时拉上游。任务推进、成功补差、失败退款依赖后台轮询。

后台轮询流程（`service/task_polling.go`，15 秒间隔）：

1. `GetAllUnFinishSyncTasks` 获取所有未完成任务
2. 按平台分组，分发到 `UpdateVideoTasks`
3. 对每个任务调用 `adaptor.FetchTask` → `GET {base_url}/api/v3/contents/generations/tasks/{upstream_task_id}`
4. 调用 `adaptor.ParseTaskResult` 解析响应
5. 更新本地任务状态和进度
6. 成功时调用 `settleTaskBillingOnComplete` 完成计费调整
7. 失败时调用 `RefundTaskQuota` 退款

部署要求：

- 至少一个 master 节点运行。
- master 节点开启 `UPDATE_TASK=true`。
- 纯 slave 部署可能导致任务长期停留在 pending/queued，且无法完成补差或退款。

## 13. 状态映射

| 上游状态 | new-api 状态 | 进度 |
| --- | --- | --- |
| `pending` / `queued` | `QUEUED` | 10% |
| `processing` / `running` | `IN_PROGRESS` | 50% |
| `succeeded` | `SUCCESS` | 100% |
| `failed` | `FAILURE` | 100% |
| 其他 | `IN_PROGRESS` | 30% |

映射实现在 `volcano.ParseTaskResult` 中。

## 14. 计费规则

Seedance 使用 `model_ratio` 按 token 计费，不使用 `model_price` 或按次固定价格。

### 14.1 预扣

```text
预扣 quota = model_ratio / 2 × QuotaPerUnit × group_ratio × seedance_condition_ratio
```

### 14.2 完成结算

```text
实际 quota = completion_tokens × model_ratio × group_ratio × seedance_condition_ratio
```

### 14.3 Token 选择优先级

1. 优先使用 `usage.completion_tokens`（Seedance 2.0 官方文档明确准确 token 用量以 completion tokens 为准）
2. 缺失或为 0 时使用 `usage.total_tokens`
3. 两者都缺失时保持预扣额度，不做 token 补差

优先级逻辑实现在 `volcano.ParseTaskResult` 中：

```go
if resTask.Usage.CompletionTokens > 0 {
    taskResult.CompletionTokens = resTask.Usage.CompletionTokens
    taskResult.TotalTokens = resTask.Usage.CompletionTokens
} else if resTask.Usage.TotalTokens > 0 {
    taskResult.TotalTokens = resTask.Usage.TotalTokens
}
```

### 14.4 条件倍率

基础 `model_ratio` 配置为"不含输入视频"的较高费率，其他条件通过 `OtherRatios` 修正。

检测逻辑（`EstimateBilling`）：

- Volcano content[] 格式：`volcano.HasVideoInMetadata(req.Metadata)` 检查 `content[]` 中是否包含 `type=video_url` 或 `type=video` 条目
- OpenAI Video 格式：`len(req.Videos) > 0` 检查顶层 `videos` 字段是否非空

组合倍率计算（`calculateCombinedRatio`）：

**主版本（`dreamina-seedance-2-0-260128` / `doubao-seedance-2-0-260128`）**：

| 条件 | 官方单价 (元/百万token) | OtherRatio |
| --- | ---: | ---: |
| 480p/720p，输入不含视频 | 46 | 1.0 |
| 480p/720p，输入含视频 | 28 | 0.6087 |
| 1080p，输入不含视频 | 51 | 1.1087 |
| 1080p，输入含视频 | 31 | 0.6739 |

**Fast 版本（`dreamina-seedance-2-0-fast-260128` / `doubao-seedance-2-0-fast-260128`）**：

| 条件 | 官方单价 (元/百万token) | OtherRatio |
| --- | ---: | ---: |
| 输入不含视频 | 37 | 1.0 |
| 输入含视频 | 22 | 0.5946 |

### 14.5 结算行为

Seedance 复用 new-api 统一任务计费链路：

1. 提交任务时根据模型、分辨率、输入视频条件预扣。
2. 任务完成轮询时解析上游 `usage`。
3. 优先将 `completion_tokens` 写入 `TaskInfo.TotalTokens`。
4. 统一任务结算调用 `RecalculateTaskQuotaByTokens()`。
5. 实际额度大于预扣时补扣差额。
6. 实际额度小于预扣时退回差额。
7. 任务失败时走现有失败退款逻辑。

### 14.6 不应加入的配置

- **不应加入 `TaskPricePatches`** — 否则会跳过 token 重算。
- **不应配置 `model_price`** — 会导致按次计费，跳过完成后补差。

### 14.7 计费示例

主版本 720p 文生视频，`completion_tokens=108900`：

```text
model_ratio = 3.1507
group_ratio = 1
seedance_condition_ratio = 1

预扣 quota = 3.1507 / 2 × 500000 × 1 × 1 = 787675
实际 quota = 108900 × 3.1507 × 1 × 1 = 343111
退回 quota = 787675 - 343111 = 444564
```

实际费用：343111 quota ÷ 500000 × 7.3 = **¥5.01 CNY**（与官方 46 元/百万 token × 0.1089M = ¥5.009 一致）

主版本 720p 输入含视频，`completion_tokens=108900`：

```text
model_ratio = 3.1507
seedance_condition_ratio = 0.6087

实际 quota = 108900 × 3.1507 × 1 × 0.6087 = 208850
```

主版本 1080p 文生视频（不含视频输入），`completion_tokens=108900`：

```text
model_ratio = 3.1507
seedance_condition_ratio = 1.1087

预扣 quota = 3.1507 / 2 × 500000 × 1 × 1.1087 = 874528
实际 quota = 108900 × 3.1507 × 1 × 1.1087 = 380498
退回 quota = 874528 - 380498 = 494030
```

实际费用：380498 quota ÷ 500000 × 7.3 = **¥5.55 CNY**（与官方 51 元/百万 token × 0.1089M = ¥5.514 基本一致，1.1087 四舍五入导致微小差异）

主版本 1080p 输入含视频，`completion_tokens=108900`：

```text
model_ratio = 3.1507
seedance_condition_ratio = 0.6739

预扣 quota = 3.1507 / 2 × 500000 × 1 × 0.6739 = 531345
实际 quota = 108900 × 3.1507 × 1 × 0.6739 = 231245
退回 quota = 531345 - 231245 = 300100
```

实际费用：231245 quota ÷ 500000 × 7.3 = **¥3.38 CNY**（与官方 31 元/百万 token × 0.1089M = ¥3.376 基本一致）

## 15. 与 HappyHorse 的对比

| | HappyHorse | Seedance |
| --- | --- | --- |
| 渠道类型 | 59 | 60 |
| 上游平台 | DashScope | 火山引擎 |
| 计费方式 | 元/秒（`model_price` × 秒数） | 元/百万token（`model_ratio` × token 数） |
| 原生查询格式 | `/happyhorse/api/status/{task_id}` 自定义 `NativeStatusResponse` | `/api/v3/contents/generations/tasks/{task_id}` 原始上游 `ResponseTask` |
| OpenAI Video 格式 | 不支持（`OpenAIVideoConverter` 未实现） | 支持（`ConvertToOpenAIVideo` 已实现） |
| 分辨率 | 720P/1080P，默认 1080P | 480p/720p/1080p，默认 720p |
| 条件倍率 | 分辨率 × 秒数 | 分辨率 × 输入视频 × token |
| 视频续写 | Video Edit 模型 | videos[] 字段 |
| 禁用按次计费 | `DisablePerCallBilling()` 显式声明 | 不需要（使用 `model_ratio` 自然避免） |

## 16. 验证命令

```powershell
go test ./relay/channel/task/seedance/... ./relay/channel/seedance/... -count=1
go vet ./relay/channel/task/seedance/... ./relay/channel/seedance/...
go build ./...
```

## 17. 验收标准

- Seedance 模型可以通过 `/v1/video/generations` 创建任务。
- 火山官方兼容格式可以通过 `/api/v3/contents/generations/tasks` 创建任务。
- `/v1/video/generations/{task_id}` 保持通用查询格式。
- `/api/v3/contents/generations/tasks/{task_id}` 返回原始上游格式。
- 对外只暴露 new-api 公开任务 ID。
- 完成结果直接返回上游 `video_url`。
- 所有入口都走 new-api 计费，不存在绕过余额或分组倍率的路径。
- 预扣和完成后补差符合 Seedance 2.0 官方 token 单价。
- 输入视频场景自动应用视频输入折扣。
- 1080p 场景自动应用 1080p 单价倍率。
- Fast 版本 1080p 被拒绝或失败后退款。
- `completion_tokens` 缺失时 fallback 到 `total_tokens`。
- 任务成功后日志能看到 token 重算、补扣或退款。
- 首版不支持 callback。
- 首版不自动推断分辨率。
- 上游可能因内容审核拒绝请求，返回错误码如 `OutputVideoSensitiveContentDetected.PolicyViolation`（常见于受版权保护的图片输入）。这不是代码 bug，需更换输入素材。
- 上游响应中 `ratio` 字段（如 `"16:9"`）表示输出画幅比，由上游自动决定，不可在请求中指定。
