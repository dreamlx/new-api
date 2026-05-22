# HappyHorse 接口文档

更新时间：2026-05-22
适用范围：new-api HappyHorse 渠道类型 `59`
默认上游地址：`https://dashscope.aliyuncs.com`

本文描述当前 new-api 中 HappyHorse 的实际接口契约。`/happyhorse/api/*` 使用 HappyHorse/DashScope 结构化请求格式；`/v1/video/generations` 使用 new-api 通用视频任务格式，并在内部转换为 HappyHorse 上游格式。

## 1. 接口总览

| 接口 | 方法 | 说明 | 鉴权 |
| --- | --- | --- | --- |
| `/happyhorse/api/generate` | `POST` | 创建 HappyHorse 视频任务 | `Authorization: Bearer <new-api token>` |
| `/happyhorse/api/status/{task_id}` | `GET` | 查询 HappyHorse 视频任务 | `Authorization: Bearer <new-api token>` |
| `/v1/video/generations` | `POST` | new-api 通用视频生成入口 | `Authorization: Bearer <new-api token>` |
| `/v1/video/generations/{task_id}` | `GET` | new-api 通用视频查询入口 | `Authorization: Bearer <new-api token>` |

`task_id` 均为 new-api 公开任务 ID，不暴露上游 DashScope 任务 ID。

## 2. 支持模型

| 模型 | 能力 | 媒体要求 |
| --- | --- | --- |
| `happyhorse-1.0-t2v` | 文生视频 | 不需要 media |
| `happyhorse-1.0-i2v` | 图生视频 | 正好 1 个 `first_frame` |
| `happyhorse-1.0-r2v` | 参考生视频 | 1-9 个 `reference_image` |
| `happyhorse-1.0-video-edit` | 视频编辑 | 正好 1 个 `video`，可选 0-5 个 `reference_image` |

`happyhorse-1.0/video` 已从当前 Go 源码的模型列表、模型识别、默认价格和查询响应中清除，不再作为提交模型或展示模型使用。

## 3. 通用参数规则

| 参数 | 适用模型 | 规则 |
| --- | --- | --- |
| `parameters.resolution` / `metadata.resolution` / `size` | 全部 | 仅支持 `720P`、`1080P`；缺省使用 `1080P` |
| `parameters.ratio` / `metadata.ratio` | T2V、R2V | 支持 `16:9`、`9:16`、`1:1`、`4:3`、`3:4` |
| `parameters.ratio` / `metadata.ratio` | I2V | 不支持；传入返回本地 400 |
| `parameters.ratio` / `metadata.ratio` | Video Edit | 不支持；传入返回本地 400 |
| `parameters.duration` / `duration` | T2V、I2V、R2V | 缺省 `5`；显式传入必须在 `3-15` |
| `parameters.duration` / `duration` | Video Edit | 不支持；显式传入返回本地 400 |
| `parameters.watermark` / `metadata.watermark` | 全部 | 透传上游 |
| `parameters.seed` / `metadata.seed` | 全部 | 透传上游 |
| `quality` / `sound` | 全部 | 当前不支持，不再透传上游 |

媒体 URL 必须非空，且必须以 `http://` 或 `https://` 开头。素材仍需能被上游直接下载；文件大小、格式、时长等复杂校验由上游返回错误。

## 4. `/happyhorse/api/generate`

### 请求

```http
POST /happyhorse/api/generate
Authorization: Bearer <token>
Content-Type: application/json
```

请求体结构：

```json
{
  "model": "happyhorse-1.0-t2v",
  "input": {
    "prompt": "A white horse runs slowly on a grassland, cinematic shot",
    "media": []
  },
  "parameters": {
    "resolution": "1080P",
    "ratio": "16:9",
    "duration": 5,
    "watermark": false,
    "seed": 1234
  }
}
```

### 响应

```json
{
  "task_id": "task_xxx",
  "status": "pending"
}
```

## 5. 原生请求示例

### T2V 文生视频

```json
{
  "model": "happyhorse-1.0-t2v",
  "input": {
    "prompt": "A white horse runs slowly on a grassland, cinematic shot"
  },
  "parameters": {
    "resolution": "1080P",
    "ratio": "16:9",
    "duration": 5
  }
}
```

### I2V 图生视频

I2V 不支持 `ratio`，输出比例跟随首帧图。

```json
{
  "model": "happyhorse-1.0-i2v",
  "input": {
    "prompt": "Animate the image with gentle motion",
    "media": [
      {
        "type": "first_frame",
        "url": "https://example.com/first-frame.jpeg"
      }
    ]
  },
  "parameters": {
    "resolution": "1080P",
    "duration": 5
  }
}
```

### R2V 参考生视频

```json
{
  "model": "happyhorse-1.0-r2v",
  "input": {
    "prompt": "Create a short video using these frame references",
    "media": [
      {
        "type": "reference_image",
        "url": "https://example.com/ref-1.jpg"
      },
      {
        "type": "reference_image",
        "url": "https://example.com/ref-2.jpg"
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

### Video Edit 视频编辑

Video Edit 不支持 `duration`。最终计费按上游返回的输入视频秒数加输出视频秒数计算。

```json
{
  "model": "happyhorse-1.0-video-edit",
  "input": {
    "prompt": "Make this video more cinematic with smooth motion",
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

## 6. `/happyhorse/api/status/{task_id}`

### 请求

```http
GET /happyhorse/api/status/{task_id}
Authorization: Bearer <token>
```

### 成功响应

```json
{
  "task_id": "task_xxx",
  "status": "completed",
  "data": {
    "model": "happyhorse-1.0-t2v",
    "mode": "text-to-video",
    "duration": 5,
    "aspect_ratio": "16:9",
    "video_url": "https://dashscope-a717.oss-accelerate.aliyuncs.com/...mp4?...",
    "resultUrls": [
      "https://dashscope-a717.oss-accelerate.aliyuncs.com/...mp4?..."
    ]
  }
}
```

状态映射：

| 响应状态 | 说明 |
| --- | --- |
| `pending` | 任务已创建或等待执行 |
| `running` | 任务执行中 |
| `completed` | 任务成功完成 |
| `failed` | 任务失败 |

## 7. `/v1/video/generations`

### 请求

```http
POST /v1/video/generations
Authorization: Bearer <token>
Content-Type: application/json
```

通用字段：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 4 个 HappyHorse 内部模型之一 |
| `prompt` | string | 是 | 视频提示词 |
| `duration` | int | T2V/I2V/R2V 可选 | 缺省 5；显式传入必须 3-15；Video Edit 不支持 |
| `metadata.resolution` | string | 否 | `720P` 或 `1080P`，默认 `1080P` |
| `metadata.ratio` | string | T2V/R2V 可选 | `16:9`、`9:16`、`1:1`、`4:3`、`3:4`；I2V 不支持 |
| `image` | string | I2V 可用 | 首帧图 URL；I2V 最终必须正好 1 张首帧图 |
| `images` | array | R2V 可用 | 参考图 URL 数组，数量 1-9 |
| `metadata.media` | array | R2V 可用 | HappyHorse media 结构；如果存在，优先用于 R2V |
| `metadata.video_url` | string | Video Edit 必填 | 输入视频 URL |
| `metadata.reference_images` | array | Video Edit 可选 | 参考图 URL 数组，数量 0-5 |

### T2V

```json
{
  "model": "happyhorse-1.0-t2v",
  "prompt": "A white horse runs slowly on a grassland, cinematic shot",
  "duration": 5,
  "metadata": {
    "resolution": "1080P",
    "ratio": "16:9"
  }
}
```

### I2V

```json
{
  "model": "happyhorse-1.0-i2v",
  "prompt": "Animate the image with gentle motion",
  "image": "https://example.com/first-frame.jpeg",
  "duration": 5,
  "metadata": {
    "resolution": "1080P"
  }
}
```

### R2V

```json
{
  "model": "happyhorse-1.0-r2v",
  "prompt": "Create a short video using these frame references",
  "images": [
    "https://example.com/ref-1.jpg",
    "https://example.com/ref-2.jpg"
  ],
  "duration": 5,
  "metadata": {
    "resolution": "1080P",
    "ratio": "3:4"
  }
}
```

### Video Edit

```json
{
  "model": "happyhorse-1.0-video-edit",
  "prompt": "Make this video more cinematic with smooth motion",
  "metadata": {
    "video_url": "https://example.com/input.mp4",
    "reference_images": [
      "https://example.com/ref.png"
    ],
    "resolution": "1080P"
  }
}
```

### 创建响应

```json
{
  "id": "task_xxx",
  "task_id": "task_xxx",
  "object": "video",
  "model": "happyhorse-1.0-t2v",
  "status": "queued",
  "progress": 0
}
```

## 8. `/v1/video/generations/{task_id}`

```http
GET /v1/video/generations/{task_id}
Authorization: Bearer <token>
```

成功响应摘要：

```json
{
  "code": "success",
  "data": {
    "task_id": "task_xxx",
    "status": "SUCCESS",
    "result_url": "https://dashscope-a717.oss-accelerate.aliyuncs.com/...mp4?...",
    "quota": 4000000,
    "properties": {
      "upstream_model_name": "happyhorse-1.0-t2v",
      "origin_model_name": "happyhorse-1.0-t2v"
    },
    "data": {
      "usage": {
        "SR": "1080",
        "ratio": "16:9",
        "duration": 5,
        "output_video_duration": 5
      }
    }
  }
}
```

`usage.SR` 兼容数字和字符串，例如 `720`、`"720"`、`1080`、`"1080"`。缺失或非法时，完成结算回退到提交时保存的分辨率倍率。

## 9. 校验规则

| 场景 | 错误 |
| --- | --- |
| `model=happyhorse-1.0/video` | `unsupported model` |
| 缺少 `prompt` | `prompt is required` |
| I2V 缺少或多于 1 个 `first_frame` | `i2v requires exactly 1 first_frame media item` |
| I2V 传入 `ratio` | `i2v does not support ratio parameter` |
| R2V 参考图数量不在 1-9 | `r2v requires at least 1 reference_image media item` 或 `r2v supports at most 9 reference images` |
| Video Edit 缺少或多于 1 个 `video` | `video-edit requires exactly 1 video media item` |
| Video Edit 参考图超过 5 个 | `video-edit supports at most 5 reference images` |
| Video Edit 显式传入 `ratio` | `happyhorse-1.0-video-edit does not support ratio parameter` |
| Video Edit 显式传入 `duration` | `video-edit does not support duration parameter` |
| 媒体 URL 为空 | `media item url is required`、`images contains empty url` 或 `reference_images contains empty url` |
| 媒体 URL 非 http/https | `media url must use http or https scheme` |
| 非法分辨率 | 仅支持 `720P`、`1080P` |
| 非法比例 | 仅支持 `16:9`、`9:16`、`1:1`、`4:3`、`3:4` |
| T2V/I2V/R2V 显式 duration 超范围 | `duration must be between 3 and 15 seconds` |

## 10. 注意事项

- 查询接口读本地任务表，不实时拉上游；任务推进、成功补差和失败退款依赖后台轮询。
- 部署时至少保留一个 master 节点开启 `UPDATE_TASK=true`。
- 成功结果直接返回上游 OSS URL，不下载、不转存、不代理、不重写。
- 上游 URL 有有效期，业务侧不要把测试 URL 当作长期素材地址。
- `aspect_ratio` 来自上游 `usage.ratio`；如果上游未返回，查询响应可能不包含该字段。
