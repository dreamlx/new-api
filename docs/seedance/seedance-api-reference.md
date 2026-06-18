# Seedance 接口文档

更新时间：2026-06-04
适用范围：new-api Seedance 渠道类型 `60`
默认上游地址：留空（管理员填写第三方网关地址）

本文描述当前 new-api 中 Seedance 2.0 的实际接口契约。`/v1/video/generations` 使用 new-api 通用视频任务格式；`/api/v3/contents/generations/tasks` 使用火山官方兼容格式。两类入口在内部都路由到同一个 Seedance task adaptor，使用同一套计费逻辑。

## 1. 接口总览

| 接口 | 方法 | 说明 | 鉴权 |
| --- | --- | --- | --- |
| `/v1/video/generations` | `POST` | OpenAI Video 风格创建视频任务 | `Authorization: Bearer <new-api token>` |
| `/v1/video/generations/{task_id}` | `GET` | OpenAI Video 风格查询视频任务 | `Authorization: Bearer <new-api token>` |
| `/v1/videos/{task_id}` | `GET` | OpenAI 原生 Video API 查询（兼容路径） | `Authorization: Bearer <new-api token>` |
| `/api/v3/contents/generations/tasks` | `POST` | 火山官方兼容创建视频任务 | `Authorization: Bearer <new-api token>` |
| `/api/v3/contents/generations/tasks/{task_id}` | `GET` | 火山官方兼容查询视频任务 | `Authorization: Bearer <new-api token>` |

`task_id` 均为 new-api 公开任务 ID，不暴露上游任务 ID。

`/v1/videos/{task_id}` 与 `/v1/video/generations/{task_id}` 路由到同一个 handler，功能完全相同。`/v1/videos` 路径兼容 OpenAI 官方 API 格式。

## 2. 支持模型

| 模型 | 能力 | 支持分辨率 | 备注 |
| --- | --- | --- | --- |
| `dreamina-seedance-2-0-260128` | 文生/图生/视频续写/音频驱动 | 480p / 720p / 1080p | 主版本，质量优先 |
| `dreamina-seedance-2-0-fast-260128` | 文生/图生/视频续写/音频驱动 | 480p / 720p | Fast 版本，速度优先 |
| `doubao-seedance-2-0-260128` | 同上 | 同上 | 火山官方模型名兼容 |
| `doubao-seedance-2-0-fast-260128` | 同上 | 同上 | 火山官方模型名兼容 |

## 3. 通用参数规则

| 参数 | 适用模型 | 规则 |
| --- | --- | --- |
| `size` / `resolution` | 全部 | 支持 `480p`、`720p`、`1080p`；fast 版仅 `480p`、`720p`；缺省 `720p` |
| `duration` | 全部 | 支持 `5`、`10`；缺省 `5` |
| `seed` | 全部 | 整数，透传上游 |
| `images` | 全部 | URL 数组，支持图生视频 / 首尾帧 / 参考图 |
| `videos` | 全部 | URL 数组，支持视频续写 |
| `audios` | 全部 | URL 数组，支持音频驱动 |

## 4. `/v1/video/generations`（OpenAI Video 风格）

### 请求

```http
POST /v1/video/generations
Authorization: Bearer <token>
Content-Type: application/json
```

```json
{
  "model": "dreamina-seedance-2-0-fast-260128",
  "prompt": "A small kitten chasing a butterfly in a sunny garden",
  "duration": 5,
  "size": "720p"
}
```

字段说明：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 见支持模型表 |
| `prompt` | string | 是 | 文本描述，可包含 `[Image N]` / `[Video N]` / `[Audio N]` 多模态标记 |
| `duration` | int | 否 | 视频时长（秒），支持 5/10，默认 5 |
| `size` | string | 否 | 分辨率，默认 `720p` |
| `images` | string[] | 否 | 图生视频 / 首尾帧 / 参考图 URL |
| `videos` | string[] | 否 | 视频续写 URL |
| `audios` | string[] | 否 | 音频驱动 URL |
| `seed` | int | 否 | 随机种子 |
| `metadata` | object | 否 | 上游高级参数透传（见下方高级参数表） |

#### 高级参数（通过 `metadata` 透传）

OpenAI Video 格式的 `metadata` 字段支持透传以下上游参数：

| metadata 字段 | 类型 | 说明 |
| --- | --- | --- |
| `ratio` | string | 画幅比（如 `"16:9"`、`"9:16"`、`"1:1"`） |
| `callback_url` | string | 回调地址（网关自行轮询，通常不需要） |
| `return_last_frame` | bool | 是否返回最后一帧 |
| `generate_audio` | bool | 是否生成音频 |
| `draft` | bool | 草稿模式 |
| `camera_fixed` | bool | 固定摄像头 |
| `watermark` | bool | 是否添加水印 |
| `frames` | int | 帧数 |
| `service_tier` | string | 服务层级 |
| `execution_expires_after` | int | 执行超时时间 |
| `tools` | array | 工具调用 |

示例：

```json
{
  "model": "dreamina-seedance-2-0-260128",
  "prompt": "A cat walking on a tightrope",
  "duration": 5,
  "size": "720p",
  "metadata": {
    "ratio": "9:16",
    "watermark": false
  }
}
```

### 创建响应

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

## 5. `/v1/video/generations/{task_id}`（OpenAI Video 风格查询）

```http
GET /v1/video/generations/{task_id}
Authorization: Bearer <token>
```

### 排队中响应

```json
{
  "code": "success",
  "message": "",
  "data": {
    "task_id": "task_xxx",
    "status": "QUEUED",
    "progress": "10%",
    "data": null
  }
}
```

### 处理中响应

```json
{
  "code": "success",
  "message": "",
  "data": {
    "task_id": "task_xxx",
    "status": "IN_PROGRESS",
    "progress": "50%",
    "data": null
  }
}
```

### 成功响应（通用 TaskDto 格式）

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

### 失败响应

```json
{
  "code": "success",
  "message": "",
  "data": {
    "task_id": "task_xxx",
    "status": "FAILURE",
    "progress": "100%",
    "fail_reason": "OutputVideoSensitiveContentDetected.PolicyViolation",
    "data": {
      "id": "cgt-...",
      "status": "failed",
      "error": {
        "code": "OutputVideoSensitiveContentDetected.PolicyViolation",
        "message": "Output video sensitive content detected"
      }
    }
  }
}
```

关键字段提取：

| 要获取 | JSON 路径 |
| --- | --- |
| 任务状态 | `data.status` — `QUEUED` / `IN_PROGRESS` / `SUCCESS` / `FAILURE` |
| 进度 | `data.progress` — 如 `"10%"` / `"50%"` / `"100%"` |
| 视频下载 URL | `data.result_url` 或 `data.data.content.video_url` |
| Token 用量 | `data.data.usage.completion_tokens` / `total_tokens` |
| 视频元数据 | `data.data.duration` / `data.data.resolution` / `data.data.ratio` |
| 失败原因 | `data.fail_reason` 或 `data.data.error.message` |

## 6. `/api/v3/contents/generations/tasks`（火山官方兼容创建）

### 请求

```http
POST /api/v3/contents/generations/tasks
Authorization: Bearer <token>
Content-Type: application/json
```

文生视频示例：

```json
{
  "model": "dreamina-seedance-2-0-fast-260128",
  "content": [
    {
      "type": "text",
      "text": "A white horse runs slowly on a grassland, cinematic shot"
    }
  ],
  "resolution": "720p",
  "duration": 5
}
```

图生视频示例（首帧）：

```json
{
  "model": "dreamina-seedance-2-0-fast-260128",
  "content": [
    {
      "type": "text",
      "text": "A cat playing with yarn"
    },
    {
      "type": "image_url",
      "image_url": {
        "url": "https://example.com/first.png"
      },
      "role": "first_frame"
    }
  ],
  "resolution": "720p",
  "duration": 5
}
```

首尾帧示例：

```json
{
  "model": "dreamina-seedance-2-0-260128",
  "content": [
    {
      "type": "text",
      "text": "从首帧缓慢过渡到尾帧"
    },
    {
      "type": "image_url",
      "image_url": {
        "url": "https://example.com/first.png"
      },
      "role": "first_frame"
    },
    {
      "type": "image_url",
      "image_url": {
        "url": "https://example.com/last.png"
      },
      "role": "last_frame"
    }
  ],
  "resolution": "720p",
  "duration": 5
}
```

视频续写示例：

```json
{
  "model": "dreamina-seedance-2-0-fast-260128",
  "content": [
    {
      "type": "text",
      "text": "Continue this video scene naturally"
    },
    {
      "type": "video",
      "video": {
        "url": "https://example.com/source.mp4"
      }
    }
  ],
  "resolution": "720p",
  "duration": 5
}
```

音频驱动示例：

```json
{
  "model": "dreamina-seedance-2-0-fast-260128",
  "content": [
    {
      "type": "text",
      "text": "A person speaking naturally"
    },
    {
      "type": "image_url",
      "image_url": {
        "url": "https://example.com/face.png"
      }
    },
    {
      "type": "audio",
      "audio": {
        "url": "https://example.com/voice.mp3"
      }
    }
  ],
  "resolution": "720p",
  "duration": 5
}
```

content[] 中支持的 type：

| type | 说明 | 字段 | `role` 可选值 |
| --- | --- | --- | --- |
| `text` | 文本提示词 | `text` | 不适用 |
| `image_url` | 图片 | `image_url.url` | `first_frame`（首帧）、`last_frame`（尾帧）、空（参考图） |
| `video` | 视频（输入格式 `video.url`） | `video.url` | 不适用 |
| `audio` | 音频（输入格式 `audio.url`） | `audio.url` | 不适用 |

`role` 字段说明：

| role 值 | 适用 type | 说明 |
| --- | --- | --- |
| `first_frame` | `image_url` | 指定该图片为视频首帧 |
| `last_frame` | `image_url` | 指定该图片为视频尾帧 |
| 空 / 省略 | `image_url` | 通用参考图，由上游决定如何使用 |

额外字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `duration` | int | 视频时长（秒） |
| `seed` | int | 随机种子 |
| `resolution` | string | 分辨率 |
| `ratio` | string | 画幅比（如 `"16:9"`、`"9:16"`） |
| `callback_url` | string | 回调地址 |
| `return_last_frame` | bool | 是否返回最后一帧 |
| `generate_audio` | bool | 是否生成音频 |
| `draft` | bool | 草稿模式 |
| `camera_fixed` | bool | 固定摄像头 |
| `watermark` | bool | 是否添加水印 |
| `frames` | int | 帧数 |
| `service_tier` | string | 服务层级 |
| `execution_expires_after` | int | 执行超时时间 |
| `tools` | array | 工具调用 |

所有上游参数均自动透传，无需额外配置。

### 创建响应

与 OpenAI Video 风格的创建响应格式相同：

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

## 7. `/api/v3/contents/generations/tasks/{task_id}`（火山官方兼容查询）

```http
GET /api/v3/contents/generations/tasks/{task_id}
Authorization: Bearer <token>
```

### 排队中响应

```json
{
  "id": "cgt-...",
  "model": "dreamina-seedance-2-0-260128",
  "status": "queued",
  "created_at": 1779348046,
  "updated_at": 1779348046
}
```

### 处理中响应

```json
{
  "id": "cgt-...",
  "model": "dreamina-seedance-2-0-260128",
  "status": "running",
  "created_at": 1779348046,
  "updated_at": 1779348060
}
```

### 成功响应（直接返回上游 ResponseTask 格式）

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

### 失败响应

```json
{
  "id": "cgt-...",
  "model": "dreamina-seedance-2-0-260128",
  "status": "failed",
  "error": {
    "code": "OutputVideoSensitiveContentDetected.PolicyViolation",
    "message": "Output video sensitive content detected"
  },
  "created_at": 1779348046,
  "updated_at": 1779348100
}
```

`ratio` 字段为输出视频的画幅比（如 `"16:9"`、`"9:16"`、`"1:1"`），由上游根据输入内容决定。

## 8. 状态映射

| 上游状态 | new-api 状态 | 进度 | 说明 |
| --- | --- | --- | --- |
| `pending` / `queued` | `QUEUED` | 10% | 排队中 |
| `processing` / `running` | `IN_PROGRESS` | 50% | 处理中 |
| `succeeded` | `SUCCESS` | 100% | 成功 |
| `failed` | `FAILURE` | 100% | 失败 |
| 其他 | `IN_PROGRESS` | 30% | 未知状态 fallback |

## 9. 错误响应

### 提交阶段错误

提交失败时返回 `TaskError` 格式，HTTP 状态码非 200：

```json
{
  "code": "invalid_request",
  "message": "prompt is required",
  "data": null
}
```

常见错误码：

| HTTP 状态码 | code | 说明 |
| --- | --- | --- |
| 400 | `invalid_request` | 请求格式错误（如 JSON 解析失败） |
| 400 | `invalid_request` | prompt 缺失 |
| 400 | `model_price_error` | 模型未配置价格 |
| 400 | `fail_to_fetch_task` | 上游返回非 200 错误（错误信息为上游原始响应） |
| 429 | `rate_limit_exceeded` | 分组上游负载饱和，稍后重试 |
| 500 | `read_body_failed` | 读取请求体失败 |
| 500 | `read_response_body_failed` | 读取上游响应失败 |
| 500 | `unmarshal_response_body_failed` | 解析上游响应失败 |
| 500 | `invalid_response` | 上游响应缺少 task ID |

### 查询阶段错误

任务失败时，查询响应中包含 `error` 字段。错误格式取决于查询路径：

- **OpenAI Video 风格**（`/v1/video/generations/{task_id}`）：`data.data.error`
- **火山兼容风格**（`/api/v3/contents/generations/tasks/{task_id}`）：顶层 `error`

上游错误信息原样转发，new-api 不修改上游错误码和消息。

常见上游错误码：

| 上游错误码 | 说明 |
| --- | --- |
| `OutputVideoSensitiveContentDetected.PolicyViolation` | 输出内容审核拒绝（常见于受版权保护的图片输入） |
| `InputValidationError` | 输入参数校验失败 |
| `QuotaExceeded` | 上游配额不足 |
| `ModelNotFound` | 模型不存在 |

### 重试行为

new-api 在以下情况自动尝试其他渠道重试：

- 上游返回 429（限流）
- 上游返回 5xx（服务器错误，504/524 除外）

以下情况**不会**重试：

- 本地验证失败（prompt 缺失、JSON 格式错误等）
- 上游返回 400（客户端错误）
- 本地计费/配置错误

## 10. 多模态用法示例

### 文生视频

```json
{
  "model": "dreamina-seedance-2-0-fast-260128",
  "prompt": "A white horse runs slowly on a grassland, cinematic shot, 4k",
  "duration": 5,
  "size": "720p"
}
```

### 图生视频（首帧）

```json
{
  "model": "dreamina-seedance-2-0-fast-260128",
  "prompt": "这只可达鸭突然觉醒了超能力",
  "images": ["https://example.com/duck.png"],
  "duration": 5,
  "size": "720p"
}
```

### 图生视频（首尾帧）

OpenAI Video 格式：

```json
{
  "model": "dreamina-seedance-2-0-260128",
  "prompt": "开场静止在 [Image 1]，缓慢推进，最终定格在 [Image 2]",
  "images": [
    "https://example.com/first.png",
    "https://example.com/last.png"
  ],
  "duration": 5,
  "size": "720p"
}
```

> ⚠️ OpenAI Video 格式通过 `images[]` 数组顺序隐式区分首尾帧，但不支持 `role` 字段。如需明确指定 `first_frame` / `last_frame`，应使用火山 content[] 格式。

火山 content[] 格式（明确指定首尾帧）：

```json
{
  "model": "dreamina-seedance-2-0-260128",
  "content": [
    { "type": "text", "text": "从首帧缓慢过渡到尾帧" },
    { "type": "image_url", "image_url": { "url": "https://example.com/first.png" }, "role": "first_frame" },
    { "type": "image_url", "image_url": { "url": "https://example.com/last.png" }, "role": "last_frame" }
  ],
  "resolution": "720p",
  "duration": 5
}
```

### 视频续写

OpenAI Video 格式：

```json
{
  "model": "dreamina-seedance-2-0-fast-260128",
  "prompt": "承接 [Video 1] 的镜头风格，自然延续场景",
  "videos": ["https://example.com/source.mp4"],
  "duration": 5,
  "size": "720p"
}
```

火山 content[] 格式：

```json
{
  "model": "dreamina-seedance-2-0-fast-260128",
  "content": [
    { "type": "text", "text": "自然延续场景" },
    { "type": "video", "video": { "url": "https://example.com/source.mp4" } }
  ],
  "resolution": "720p",
  "duration": 5
}
```

> 注意：火山 content[] 输入格式使用 `type: "video"` 和 `video.url` 嵌套结构，上游格式使用 `type: "video_url"` 和 `video_url.url` 扁平结构。系统自动完成转换。

### 音频驱动

OpenAI Video 格式：

```json
{
  "model": "dreamina-seedance-2-0-fast-260128",
  "prompt": "[Image 1] 中的人物开口说话，配以 [Audio 1] 的语音",
  "images": ["https://example.com/face.png"],
  "audios": ["https://example.com/voice.mp3"],
  "duration": 5,
  "size": "720p"
}
```

火山 content[] 格式：

```json
{
  "model": "dreamina-seedance-2-0-fast-260128",
  "content": [
    { "type": "text", "text": "人物开口说话" },
    { "type": "image_url", "image_url": { "url": "https://example.com/face.png" } },
    { "type": "audio", "audio": { "url": "https://example.com/voice.mp3" } }
  ],
  "resolution": "720p",
  "duration": 5
}
```

> 注意：火山 content[] 输入格式使用 `type: "audio"` 和 `audio.url` 嵌套结构，上游格式使用 `type: "audio_url"` 和 `audio_url.url` 扁平结构。系统自动完成转换。

### 高级参数透传

OpenAI Video 格式（通过 `metadata`）：

```json
{
  "model": "dreamina-seedance-2-0-260128",
  "prompt": "A cat playing with yarn, vertical video",
  "duration": 5,
  "size": "720p",
  "metadata": {
    "ratio": "9:16",
    "watermark": false,
    "generate_audio": true
  }
}
```

火山 content[] 格式（顶层字段直接透传）：

```json
{
  "model": "dreamina-seedance-2-0-260128",
  "content": [
    { "type": "text", "text": "A cat playing with yarn, vertical video" }
  ],
  "resolution": "720p",
  "duration": 5,
  "ratio": "9:16",
  "watermark": false,
  "generate_audio": true
}
```

### 1080p 视频生成

```json
{
  "model": "dreamina-seedance-2-0-260128",
  "prompt": "A cinematic aerial shot over a mountain range, ultra HD",
  "duration": 5,
  "size": "1080p"
}
```

> 1080p 仅主版本支持，Fast 版本不支持。1080p 计费倍率为 1.1087（不含视频输入）或 0.6739（含视频输入）。

### 10 秒视频

```json
{
  "model": "dreamina-seedance-2-0-260128",
  "prompt": "A slow-motion wave crashing on the shore",
  "duration": 10,
  "size": "720p"
}
```

## 11. 注意事项

- 视频下载 URL 为上游 TOS 签名 URL，**24 小时有效**，需立刻下载或转存。
- 查询接口读本地任务表 + 后台轮询；任务推进、成功补差和失败退款依赖 master 节点轮询。
- 部署时至少保留一个 master 节点开启 `UPDATE_TASK=true`。
- `videos[]` 元素要求高度 ≥ 300px，否则上游 400 拒绝。
- Fast 版本不支持 1080p，仅支持 480p / 720p。
- 首尾帧场景下，输出画幅可能被上游强制成方形以撮合两张参考图。
- 上游可能因内容审核拒绝请求，返回错误码如 `OutputVideoSensitiveContentDetected.PolicyViolation`。这通常是因为输入图片受版权保护或违反内容政策。遇到此类错误时需更换输入素材。
- 上游响应中 `ratio` 字段（如 `"16:9"`、`"9:16"`）表示输出视频画幅比，由上游根据输入自动决定。
- `metadata` 中的字段如果与请求体显式字段同名（如同时传 `size: "720p"` 和 `metadata.resolution: "1080p"`），显式字段优先。详见开发文档。
