# HappyHorse 视频兼容开发文档

更新时间：2026-05-22

本文记录当前 HappyHorse 视频兼容实现的最终开发口径。历史讨论中的旧规则以本文为准。

## 1. 目标

新增 HappyHorse 视频任务能力，支持两类入口：

```http
POST /v1/video/generations
GET  /v1/video/generations/{task_id}

POST /happyhorse/api/generate
GET  /happyhorse/api/status/{task_id}
```

两类入口都走 new-api 现有鉴权、渠道分发、任务记录、计费预扣、完成结算和失败退款链路。对外只暴露 new-api 公开 `task_id`，不暴露上游 DashScope 任务 ID。

## 2. 当前路由

`/happyhorse/api/generate` 使用 HappyHorse/DashScope 结构化请求格式：

```json
{
  "model": "happyhorse-1.0-t2v",
  "input": {
    "prompt": "A white horse runs slowly on a grassland"
  },
  "parameters": {
    "resolution": "1080P",
    "ratio": "16:9",
    "duration": 5
  }
}
```

`/happyhorse/api/status` 只支持 path 参数：

```http
GET /happyhorse/api/status/{task_id}
```

`/happyhorse/api/status?task_id=xxx` 不作为兼容接口。

## 3. 支持模型

| 模型 | 能力 | media 规则 |
| --- | --- | --- |
| `happyhorse-1.0-t2v` | 文生视频 | 不需要 `input.media` |
| `happyhorse-1.0-i2v` | 图生视频 | 正好 1 个 `type=first_frame` |
| `happyhorse-1.0-r2v` | 参考生视频 | 1-9 个 `type=reference_image` |
| `happyhorse-1.0-video-edit` | 视频编辑 | 正好 1 个 `type=video`，可选 0-5 个 `type=reference_image` |

已清除历史聚合模型名 `happyhorse-1.0/video`。

## 4. 参数契约

| 参数 | 规则 |
| --- | --- |
| `resolution` | 支持 `720P`、`1080P`；默认 `1080P` |
| `ratio` | T2V/R2V 支持 `16:9`、`9:16`、`1:1`、`4:3`、`3:4`；I2V 和 Video Edit 不支持 |
| `duration` | T2V/I2V/R2V 显式传入必须 `3-15`；缺省使用 `5` |
| `duration` + Video Edit | 不支持，显式传入返回 400 |
| `watermark` | 透传上游 |
| `seed` | 透传上游 |
| `quality` / `sound` | 不支持，不再透传上游 |
| media URL | 必须非空并使用 `http://` 或 `https://` |

## 5. `/v1/video/generations` 字段映射

| V1 字段 | HappyHorse 字段 | 规则 |
| --- | --- | --- |
| `model` | `model` | 4 个内部模型之一 |
| `prompt` | `input.prompt` | 必填 |
| `duration` | `parameters.duration` | 仅 T2V/I2V/R2V；缺省 5；显式 3-15 |
| `metadata.resolution` / `size` | `parameters.resolution` | 默认 1080P |
| `metadata.ratio` | `parameters.ratio` | 仅 T2V/R2V |
| `image` / `images[0]` / `input_reference` | I2V `media[type=first_frame]` | I2V 最终必须正好 1 张首帧 |
| `images[]` | R2V `media[type=reference_image]` | 1-9 张参考图 |
| `metadata.media` | R2V 原生 media | 存在时优先用于 R2V，并校验只能是 `reference_image` |
| `metadata.video_url` | Video Edit `media[type=video]` | 必填且正好 1 个 |
| `metadata.reference_images` | Video Edit `media[type=reference_image]` | 0-5 张 |

首版不从宽高、`size=1280x720`、图片比例或视频比例自动推断分辨率和比例。

## 6. 请求转换示例

### T2V

V1 请求：

```json
{
  "model": "happyhorse-1.0-t2v",
  "prompt": "A city at night",
  "duration": 5,
  "metadata": {
    "resolution": "1080P",
    "ratio": "16:9"
  }
}
```

上游请求：

```json
{
  "model": "happyhorse-1.0-t2v",
  "input": {
    "prompt": "A city at night"
  },
  "parameters": {
    "resolution": "1080P",
    "ratio": "16:9",
    "duration": 5
  }
}
```

### I2V

V1 请求：

```json
{
  "model": "happyhorse-1.0-i2v",
  "prompt": "Animate the image",
  "image": "https://example.com/first-frame.png",
  "duration": 5,
  "metadata": {
    "resolution": "1080P"
  }
}
```

上游请求：

```json
{
  "model": "happyhorse-1.0-i2v",
  "input": {
    "prompt": "Animate the image",
    "media": [
      {
        "type": "first_frame",
        "url": "https://example.com/first-frame.png"
      }
    ]
  },
  "parameters": {
    "resolution": "1080P",
    "duration": 5
  }
}
```

### R2V

V1 请求：

```json
{
  "model": "happyhorse-1.0-r2v",
  "prompt": "Use the reference frames",
  "images": [
    "https://example.com/ref-1.png",
    "https://example.com/ref-2.png"
  ],
  "duration": 5,
  "metadata": {
    "resolution": "1080P",
    "ratio": "4:3"
  }
}
```

上游请求：

```json
{
  "model": "happyhorse-1.0-r2v",
  "input": {
    "prompt": "Use the reference frames",
    "media": [
      {
        "type": "reference_image",
        "url": "https://example.com/ref-1.png"
      },
      {
        "type": "reference_image",
        "url": "https://example.com/ref-2.png"
      }
    ]
  },
  "parameters": {
    "resolution": "1080P",
    "ratio": "4:3",
    "duration": 5
  }
}
```

### Video Edit

V1 请求：

```json
{
  "model": "happyhorse-1.0-video-edit",
  "prompt": "Make this video cinematic",
  "metadata": {
    "resolution": "1080P",
    "video_url": "https://example.com/input.mp4",
    "reference_images": [
      "https://example.com/ref.png"
    ]
  }
}
```

上游请求：

```json
{
  "model": "happyhorse-1.0-video-edit",
  "input": {
    "prompt": "Make this video cinematic",
    "media": [
      {
        "type": "video",
        "url": "https://example.com/input.mp4"
      },
      {
        "type": "reference_image",
        "url": "https://example.com/ref.png"
      }
    ]
  },
  "parameters": {
    "resolution": "1080P"
  }
}
```

Video Edit 不发送 `parameters.duration`。

## 7. 上游请求

HappyHorse 上游实际为 DashScope 异步任务接口：

```http
POST /api/v1/services/aigc/video-generation/video-synthesis
GET  /api/v1/tasks/{task_id}
```

请求头：

```http
Authorization: Bearer {channel_key}
Content-Type: application/json
X-DashScope-Async: enable
```

成功查询响应中的 `usage.SR` 可能是数字或字符串。当前实现兼容 `720`、`"720"`、`1080`、`"1080"`。

## 8. 返回格式

### `/v1/video/generations`

保留 new-api 通用任务创建格式。任务 ID 为 new-api 自己生成的公开 `task_id`，上游 `task_id` 只写入内部任务数据。

### `/v1/video/generations/{task_id}`

保留 new-api 通用查询格式。完成时直接返回上游 `video_url`，不下载、不转存、不代理、不重写。

### `/happyhorse/api/generate`

返回简洁结构：

```json
{
  "task_id": "task_xxx",
  "status": "pending"
}
```

### `/happyhorse/api/status/{task_id}`

返回 HappyHorse 简洁查询格式：

```json
{
  "task_id": "task_xxx",
  "status": "completed",
  "data": {
    "model": "happyhorse-1.0-t2v",
    "mode": "text-to-video",
    "duration": 5,
    "aspect_ratio": "16:9",
    "video_url": "https://example.com/video.mp4",
    "resultUrls": [
      "https://example.com/video.mp4"
    ]
  }
}
```

## 9. 状态映射

| HappyHorse 状态 | new-api 状态 | 说明 |
| --- | --- | --- |
| `PENDING` | `queued` | 排队中 |
| `RUNNING` | `processing` / `in_progress` | 处理中 |
| `SUCCEEDED` | `succeeded` | 成功 |
| `FAILED` | `failed` | 失败 |
| `CANCELED` | `failed` | 映射为失败 |
| `UNKNOWN` | `failed` | 映射为失败 |

## 10. 计费规则

HappyHorse 借用 new-api task 计费链路，不借用 Sora 倍率常量。

预扣：

```text
quota = model_price * QuotaPerUnit * group_ratio * estimated_seconds * resolution_ratio
```

完成结算：

```text
quota = model_price * QuotaPerUnit * group_ratio * actual_seconds * resolution_ratio
```

秒数规则：

- T2V/I2V/R2V：优先 `usage.output_video_duration`，其次 `usage.duration`。
- Video Edit：使用 `usage.input_video_duration + usage.output_video_duration`；缺字段时回退 `usage.duration`。

分辨率规则：

- 预扣使用请求中的 resolution，缺省 `1080P`。
- 完成结算优先读取上游 `usage.SR`。
- `usage.SR` 缺失或非法时，回退提交时保存的分辨率倍率。

HappyHorse task adaptor 通过 `DisablePerCallBilling()` 显式声明禁用按次计费，确保任务完成后会进入 `AdjustBillingOnComplete` 补差结算。

## 11. 测试覆盖

当前 HappyHorse 单测覆盖：

- 4 个模型请求转换。
- 原生入口和 V1 入口字段校验。
- duration 缺省、过小、过大、边界值。
- T2V/R2V ratio `4:3`、`3:4`。
- I2V 禁止 ratio。
- Video Edit 禁止 ratio。
- I2V 正好 1 张首帧。
- R2V 参考图 1-9。
- Video Edit 正好 1 个视频，0-5 张参考图。
- media URL 非空和 http/https 校验。
- `quality` / `sound` 不进入上游请求体。
- `usage.SR` 数字/字符串兼容。
- Video Edit 输入秒数加输出秒数计费。
- 补扣和退款方向。

验证命令：

```powershell
go test ./relay/channel/task/happyhorse ./relay/channel/happyhorse -count=1
go vet ./relay/channel/task/happyhorse/... ./relay/channel/happyhorse/...
go build ./...
```

## 12. 验收标准

- HappyHorse 模型可以通过 `/v1/video/generations` 创建任务。
- HappyHorse 结构化格式可以通过 `/happyhorse/api/generate` 创建任务。
- `/v1/video/generations/{task_id}` 保持通用查询格式。
- `/happyhorse/api/status/{task_id}` 返回 HappyHorse 查询格式。
- 对外只暴露 new-api 公开任务 ID。
- 完成结果直接返回上游 `video_url`。
- 所有入口都走 new-api 计费，不存在绕过余额或分组倍率的路径。
- 预扣和完成后补差符合 HappyHorse 720P/1080P 单秒价格。
- Video Edit 按输入视频秒数加输出视频秒数计费。
- 首版不支持 callback。
- 首版不自动推断分辨率和比例。
