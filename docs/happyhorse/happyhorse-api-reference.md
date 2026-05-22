# HappyHorse 接口文档

更新时间：2026-05-21  
适用范围：`new-api` HappyHorse 渠道类型 `59`  
上游默认地址：`https://dashscope.aliyuncs.com`

> 说明：HappyHorse 官网文档提供的是官网扁平示例；当前 new-api 内部实现按已确认方案，对 `/happyhorse/api/generate` 使用 HappyHorse/DashScope 结构化请求格式。官网文档参考：[HappyHorse API Docs](https://ai-happyhorse.github.io/happyhorse-api-docs/)

## 1. 接口总览

| 接口 | 方法 | 说明 | 鉴权 |
| --- | --- | --- | --- |
| `/happyhorse/api/generate` | `POST` | 创建 HappyHorse 视频任务 | `Authorization: Bearer <new-api token>` |
| `/happyhorse/api/status/{task_id}` | `GET` | 查询 HappyHorse 视频任务 | `Authorization: Bearer <new-api token>` |
| `/v1/video/generations` | `POST` | new-api 通用视频生成入口 | `Authorization: Bearer <new-api token>` |
| `/v1/video/generations/{task_id}` | `GET` | new-api 通用视频查询入口 | `Authorization: Bearer <new-api token>` |

`/happyhorse/api/*` 面向 HappyHorse 结构化格式用户；`/v1/video/generations` 面向 new-api 通用视频格式用户。两类入口都会走相同的鉴权、渠道分发、任务记录、计费预扣、完成结算和失败退款链路。

## 2. 支持模型

| 模型 | 能力 | 必需媒体 |
| --- | --- | --- |
| `happyhorse-1.0-t2v` | 文生视频 | 无 |
| `happyhorse-1.0-i2v` | 图生视频 | `media[type=first_frame]` |
| `happyhorse-1.0-r2v` | 参考生视频 | 至少一个 `media[type=reference_image]` |
| `happyhorse-1.0-video-edit` | 视频编辑 | 一个 `media[type=video]`，可选 `reference_image` |

`happyhorse-1.0/video` 已从当前 Go 源码的 HappyHorse 模型列表、模型识别、默认价格和查询响应中清除，不再作为提交模型或展示模型使用。

## 3. 创建任务

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
    "media": [
      {
        "type": "reference_image",
        "url": "https://example.com/ref.jpg"
      }
    ]
  },
  "parameters": {
    "resolution": "720P",
    "ratio": "16:9",
    "duration": 5,
    "quality": "pro",
    "sound": false,
    "watermark": false,
    "seed": 1234
  }
}
```

字段说明：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `model` | string | 是 | 必须是 4 个内部模型之一 |
| `input.prompt` | string | 是 | 视频提示词 |
| `input.media` | array | 按模型 | 图片、视频或参考图输入 |
| `input.media[].type` | string | 按模型 | `first_frame`、`reference_image`、`video` |
| `input.media[].url` | string | 按模型 | 上游可直接下载的公网 URL |
| `parameters.resolution` | string | 否 | `720P` 或 `1080P`，默认 `720P` |
| `parameters.ratio` | string | 否 | `16:9`、`9:16`、`1:1` |
| `parameters.duration` | int | 否 | 默认 `5`；显式 `0` 会被保留到请求解析，但计费预估会回退默认 5 秒 |
| `parameters.quality` | string | 否 | 透传上游，例如 `std`、`pro` |
| `parameters.sound` | bool | 否 | 透传上游；显式 `false` 不会丢失 |
| `parameters.watermark` | bool | 否 | 透传上游 |
| `parameters.seed` | int | 否 | 透传上游 |

### 响应

```json
{
  "task_id": "task_xxx",
  "status": "pending"
}
```

`task_id` 是 new-api 公开任务 ID，不暴露上游任务 ID。

## 4. 创建任务示例

### T2V 文生视频

```json
{
  "model": "happyhorse-1.0-t2v",
  "input": {
    "prompt": "A white horse runs slowly on a grassland, cinematic shot"
  },
  "parameters": {
    "resolution": "720P",
    "ratio": "16:9",
    "duration": 5
  }
}
```

### I2V 图生视频

```json
{
  "model": "happyhorse-1.0-i2v",
  "input": {
    "prompt": "Animate the image with gentle motion",
    "media": [
      {
        "type": "first_frame",
        "url": "https://help-static-aliyun-doc.aliyuncs.com/file-manage-files/zh-CN/20241022/emyrja/dog_and_girl.jpeg"
      }
    ]
  },
  "parameters": {
    "resolution": "720P",
    "ratio": "16:9",
    "duration": 5
  }
}
```

### R2V 参考生视频

```json
{
  "model": "happyhorse-1.0-r2v",
  "input": {
    "prompt": "Create a short video using these football frame references",
    "media": [
      {
        "type": "reference_image",
        "url": "https://help-static-aliyun-doc.aliyuncs.com/file-manage-files/zh-CN/20241108/xzsgiz/football1.jpg"
      },
      {
        "type": "reference_image",
        "url": "https://help-static-aliyun-doc.aliyuncs.com/file-manage-files/zh-CN/20241108/tdescd/football2.jpg"
      }
    ]
  },
  "parameters": {
    "resolution": "720P",
    "ratio": "16:9",
    "duration": 5
  }
}
```

### Video Edit 视频编辑

```json
{
  "model": "happyhorse-1.0-video-edit",
  "input": {
    "prompt": "Make this video more cinematic with smooth motion",
    "media": [
      {
        "type": "video",
        "url": "https://help-static-aliyun-doc.aliyuncs.com/file-manage-files/zh-CN/20241115/cqqkru/1.mp4"
      }
    ]
  },
  "parameters": {
    "resolution": "720P",
    "ratio": "16:9",
    "duration": 5
  }
}
```

## 5. 查询任务

### 请求

```http
GET /happyhorse/api/status/{task_id}
Authorization: Bearer <token>
```

示例：

```http
GET /happyhorse/api/status/task_omYeJhksk5qAa8lshCWdcx8UrWTbpERv
```

### 成功响应

```json
{
  "task_id": "task_omYeJhksk5qAa8lshCWdcx8UrWTbpERv",
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

状态说明：

| 状态 | 说明 |
| --- | --- |
| `pending` | 任务已创建或等待执行 |
| `running` | 任务执行中 |
| `completed` | 任务成功完成 |
| `failed` | 任务失败 |

失败响应示例：

```json
{
  "task_id": "task_xxx",
  "status": "failed",
  "message": "Task failed"
}
```

## 6. 校验规则

| 场景 | 错误 |
| --- | --- |
| `model=happyhorse-1.0/video` | 不支持，返回 `unsupported model` |
| 不支持的模型 | `unsupported model` |
| 缺少 `input.prompt` | `prompt is required` |
| I2V 缺少 `first_frame` | `i2v requires a first_frame media item` |
| R2V 缺少 `reference_image` | `r2v requires at least one reference_image media item` |
| Video Edit 缺少 `video` | `video-edit requires a video media item` |
| 非法分辨率 | 仅支持 `720P`、`1080P` |
| 非法比例 | 仅支持 `16:9`、`9:16`、`1:1` |

## 7. 注意事项

- 素材 URL 必须能被 HappyHorse 上游直接下载。
- 成功结果直接返回上游 OSS URL，不下载、不转存、不代理。
- 上游 URL 有有效期，业务侧不要把测试 URL 当作长期素材地址。
- `aspect_ratio` 当前优先取上游 `usage.ratio`；如果上游未返回，查询响应可能不包含该字段。

## 8. `/v1/video/generations` 使用方法

`/v1/video/generations` 使用 new-api 通用视频请求格式，模型名仍使用 HappyHorse 内部模型名。

### 创建任务

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
| `duration` | int | 否 | 默认 5 秒 |
| `metadata.resolution` | string | 否 | `720P` 或 `1080P`，默认 `720P` |
| `metadata.ratio` | string | 否 | `16:9`、`9:16`、`1:1` |
| `image` | string | I2V 可用 | 首帧图 URL |
| `images` | array | R2V 可用 | 参考图 URL 数组 |
| `metadata.video_url` | string | Video Edit 必填 | 输入视频 URL |
| `metadata.reference_images` | array | Video Edit 可选 | 参考图 URL 数组 |

### T2V 文生视频

```json
{
  "model": "happyhorse-1.0-t2v",
  "prompt": "A white horse runs slowly on a grassland, cinematic shot",
  "duration": 5,
  "metadata": {
    "resolution": "720P",
    "ratio": "16:9"
  }
}
```

### I2V 图生视频

```json
{
  "model": "happyhorse-1.0-i2v",
  "prompt": "Animate the image with gentle motion",
  "image": "https://help-static-aliyun-doc.aliyuncs.com/file-manage-files/zh-CN/20241022/emyrja/dog_and_girl.jpeg",
  "duration": 5,
  "metadata": {
    "resolution": "720P",
    "ratio": "16:9"
  }
}
```

### R2V 参考生视频

```json
{
  "model": "happyhorse-1.0-r2v",
  "prompt": "Create a short video using these football frame references",
  "images": [
    "https://help-static-aliyun-doc.aliyuncs.com/file-manage-files/zh-CN/20241108/xzsgiz/football1.jpg",
    "https://help-static-aliyun-doc.aliyuncs.com/file-manage-files/zh-CN/20241108/tdescd/football2.jpg"
  ],
  "duration": 5,
  "metadata": {
    "resolution": "720P",
    "ratio": "16:9"
  }
}
```

### Video Edit 视频编辑

```json
{
  "model": "happyhorse-1.0-video-edit",
  "prompt": "Make this video more cinematic with smooth motion",
  "duration": 5,
  "metadata": {
    "video_url": "https://help-static-aliyun-doc.aliyuncs.com/file-manage-files/zh-CN/20241115/cqqkru/1.mp4",
    "resolution": "720P",
    "ratio": "16:9"
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

### 查询任务

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
    "quota": 2250000,
    "properties": {
      "upstream_model_name": "happyhorse-1.0-t2v",
      "origin_model_name": "happyhorse-1.0-t2v"
    },
    "data": {
      "usage": {
        "SR": 720,
        "ratio": "16:9",
        "duration": 5,
        "output_video_duration": 5
      }
    }
  }
}
```

### `/happyhorse/api/*` 与 `/v1/video/generations` 差异

| 项目 | `/happyhorse/api/generate` | `/v1/video/generations` |
| --- | --- | --- |
| 请求格式 | `model + input + parameters` | new-api 通用视频格式 |
| 查询接口 | `/happyhorse/api/status/{task_id}` | `/v1/video/generations/{task_id}` |
| 提交响应 | `task_id/status` | OpenAI-compatible video 任务结构 |
| 查询响应 | HappyHorse 简洁结构 | new-api 通用任务结构 |
| 模型名 | 4 个内部模型 | 4 个内部模型 |
