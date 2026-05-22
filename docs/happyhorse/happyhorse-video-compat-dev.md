# HappyHorse 视频兼容开发文档

本文档描述在 `new-api` 中新增 HappyHorse 视频渠道兼容能力的需求、接口行为、字段转换、计费规则和实施计划。

## 2026-05-20 回退后最新接口口径

`/happyhorse/api/generate` 的提交请求体回退为 HappyHorse/DashScope 结构化格式，不再使用官网扁平提交格式。

`/happyhorse/api/status` 的查询路径改为 path 参数：

```http
GET /happyhorse/api/status/{task_id}
```

本节为最新口径，覆盖本文档后续历史段落中出现的“官网扁平格式”和 `status?task_id=xxx` 旧描述。

官网扁平格式仅作为参考，不作为当前 `/happyhorse/api/generate` 的提交格式：

```text
https://ai-happyhorse.github.io/happyhorse-api-docs/
```

创建任务接口：

```http
POST /happyhorse/api/generate
```

请求体使用结构化格式：

```json
{
  "model": "happyhorse-1.0-i2v",
  "input": {
    "prompt": "Animate the image with gentle motion",
    "media": [
      {
        "type": "first_frame",
        "url": "https://example.com/image.jpeg"
      }
    ]
  },
  "parameters": {
    "resolution": "720P",
    "ratio": "16:9",
    "duration": 5,
    "quality": "pro",
    "sound": false
  }
}
```

查询任务接口：

```http
GET /happyhorse/api/status/{task_id}
```

查询返回继续使用 HappyHorse 简洁格式：

```json
{
  "task_id": "task_xxx",
  "status": "completed",
  "message": "Video generation completed.",
  "data": {
    "model": "happyhorse-1.0/video",
    "mode": "text-to-video",
    "duration": 5,
    "aspect_ratio": "16:9",
    "resultUrls": [
      "https://example.com/video.mp4"
    ],
    "video_url": "https://example.com/video.mp4"
  }
}
```

当前规则：

- `/happyhorse/api/generate` 接受 `model + input + parameters` 结构化请求。
- `/happyhorse/api/generate` 提交响应保持 `task_id/status` 简洁结构。
- `/happyhorse/api/status/{task_id}` 使用 path 参数中的 new-api 公开任务 ID 查询。
- `/happyhorse/api/status/{task_id}` 查询响应保持 `task_id/status/message/data` 简洁结构。
- `/v1/video/generations` 和 `/v1/video/generations/{task_id}` 不受本次回退影响。

### 回退实施计划

1. 路由层把 HappyHorse 查询接口从 `GET /happyhorse/api/status` 调整为 `GET /happyhorse/api/status/:task_id`，继续复用 `controller.RelayTaskFetch`。
2. `/happyhorse/api/generate` 校验层从解析扁平 DTO 改为解析 `GenerateRequest`。
3. 结构化请求原样保存给上游提交，同时归一化为 `TaskSubmitReq`，保证鉴权、渠道分发、预扣、任务记录和完成结算不分叉。
4. 按 `model` 校验媒体字段：t2v 不要求 `media`，i2v 要求 `first_frame`，r2v 要求 `reference_image`，video-edit 要求 `video`。
5. 文档和测试用例改为结构化提交格式，并把 `status?task_id=xxx` 全部改为 `status/{task_id}`。

## 目标

新增 HappyHorse 视频任务能力，支持两类入口：

```http
POST /v1/video/generations
GET  /v1/video/generations/{task_id}

POST /happyhorse/api/generate
GET  /happyhorse/api/status/{task_id}
```

两类入口都必须走 `new-api` 现有的鉴权、渠道分发、任务记录、计费预扣、完成结算和失败退款链路。

内部统一转换口径回到 HappyHorse/DashScope 结构化格式；提交上游时可直接使用 `GenerateRequest`，同时归一化为 `TaskSubmitReq` 供鉴权、任务记录、预扣和完成结算使用。文档来源：

```text
https://ai-happyhorse.github.io/happyhorse-api-docs/
D:\pythonproject\happhoserNewapi\docs\happyhorse 接口文档
```

对外返回格式按入口区分：

- `/v1/video/generations`：保留当前 `new-api` 通用任务格式。
- `/happyhorse/api/generate`：提交响应返回 HappyHorse 简洁 `task_id/status` 格式。
- `/v1/video/generations/{task_id}`：保留当前 `new-api` 通用查询格式。
- `/happyhorse/api/status/{task_id}`：返回 HappyHorse 简洁查询格式。

## 支持模型

| 模型 | 能力 | HappyHorse media 规则 |
| --- | --- | --- |
| `happyhorse-1.0-t2v` | 文生视频 | 不需要 `input.media` |
| `happyhorse-1.0-i2v` | 图生视频，基于首帧 | `type=first_frame`，有且仅取一张首帧 |
| `happyhorse-1.0-r2v` | 参考生视频 | `type=reference_image`，支持多张参考图 |
| `happyhorse-1.0-video-edit` | 视频编辑 | 必须有一个 `type=video`，可选 `type=reference_image` |

## HappyHorse 标准格式

内部 canonical request 使用 HappyHorse 文档格式：

```json
{
  "model": "happyhorse-1.0-t2v",
  "input": {
    "prompt": "一只猫在草地上奔跑",
    "media": [
      {
        "type": "reference_image",
        "url": "https://example.com/ref.png"
      }
    ]
  },
  "parameters": {
    "resolution": "720P",
    "ratio": "16:9",
    "duration": 5,
    "watermark": false,
    "seed": 1234
  }
}
```

`POST /happyhorse/api/generate` 完全接受该文档格式，不支持扁平化 HappyHorse 原生入口作为首版目标。

## `/v1/video/generations` 字段映射

当前 relay 实际解析结构是 `relay/common.TaskSubmitReq`：

```go
type TaskSubmitReq struct {
    Prompt         string                 `json:"prompt"`
    Model          string                 `json:"model,omitempty"`
    Mode           string                 `json:"mode,omitempty"`
    Image          string                 `json:"image,omitempty"`
    Images         []string               `json:"images,omitempty"`
    Size           string                 `json:"size,omitempty"`
    Duration       int                    `json:"duration,omitempty"`
    Seconds        string                 `json:"seconds,omitempty"`
    InputReference string                 `json:"input_reference,omitempty"`
    Metadata       map[string]interface{} `json:"metadata,omitempty"`
}
```

基础字段映射：

| `/v1/video/generations` 字段 | HappyHorse 字段 | 规则 |
| --- | --- | --- |
| `model` | `model` | 按模型名区分能力 |
| `prompt` | `prompt` | 必填 |
| `mode` | `mode` | 优先使用请求值；缺失时按模型能力补默认值 |
| `duration` | `duration` | 缺失时默认 `5` |
| `metadata.ratio` | `aspect_ratio` | 仅支持 `16:9`、`9:16`、`1:1` |
| `metadata.quality` | `quality` | 透传 HappyHorse 官网字段 |
| `metadata.sound` | `sound` | 透传 HappyHorse 官网字段；显式 `false` 不能丢失 |
| `metadata.resolution` | 内部计费/上游转换字段 | 仅支持 `720P`、`1080P`；不作为官网扁平字段返回 |
| `size` | 内部计费/上游转换字段 | 仅当值精确为 `720P` 或 `1080P` 时接受 |
| `image` / `images` / `input_reference` / `metadata.video_url` | 内部 media 转换字段 | 用于生成上游 `media`，不作为官网扁平字段直接暴露 |

首版不从宽高、`size=1280x720`、图片比例或视频比例自动推断分辨率和比例。

## 媒体字段映射

### 文生视频

请求：

```json
{
  "model": "happyhorse-1.0-t2v",
  "prompt": "一座纸板城市在夜晚亮起灯光",
  "duration": 5,
  "metadata": {
    "resolution": "720P",
    "ratio": "16:9"
  }
}
```

转换：

```json
{
  "model": "happyhorse-1.0-t2v",
  "input": {
    "prompt": "一座纸板城市在夜晚亮起灯光"
  },
  "parameters": {
    "resolution": "720P",
    "ratio": "16:9",
    "duration": 5
  }
}
```

### 图生视频

图片优先级固定为：

```text
image > images[0] > input_reference
```

请求：

```json
{
  "model": "happyhorse-1.0-i2v",
  "prompt": "让画面中的猫向前奔跑",
  "image": "https://example.com/first-frame.png",
  "duration": 5,
  "metadata": {
    "resolution": "720P"
  }
}
```

转换：

```json
{
  "model": "happyhorse-1.0-i2v",
  "input": {
    "prompt": "让画面中的猫向前奔跑",
    "media": [
      {
        "type": "first_frame",
        "url": "https://example.com/first-frame.png"
      }
    ]
  },
  "parameters": {
    "resolution": "720P",
    "duration": 5
  }
}
```

### 参考生视频

`metadata.media` 优先。如果没有 `metadata.media`，使用 `images[]`；如果 `images[]` 为空，则允许 `image` 或 `input_reference` 作为单张参考图。

请求：

```json
{
  "model": "happyhorse-1.0-r2v",
  "prompt": "[Image 1]中的角色拿起[Image 2]中的折扇",
  "images": [
    "https://example.com/person.png",
    "https://example.com/fan.png"
  ],
  "duration": 5,
  "metadata": {
    "resolution": "720P",
    "ratio": "16:9"
  }
}
```

转换：

```json
{
  "model": "happyhorse-1.0-r2v",
  "input": {
    "prompt": "[Image 1]中的角色拿起[Image 2]中的折扇",
    "media": [
      {
        "type": "reference_image",
        "url": "https://example.com/person.png"
      },
      {
        "type": "reference_image",
        "url": "https://example.com/fan.png"
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

### 视频编辑

`/v1/video/generations` 的视频编辑请求要求输入视频放在：

```text
metadata.video_url
```

参考图放在：

```text
metadata.reference_images
```

请求：

```json
{
  "model": "happyhorse-1.0-video-edit",
  "prompt": "让视频中的角色穿上参考图中的条纹毛衣",
  "duration": 5,
  "metadata": {
    "resolution": "720P",
    "video_url": "https://example.com/input.mp4",
    "reference_images": [
      "https://example.com/ref-1.png",
      "https://example.com/ref-2.png"
    ]
  }
}
```

转换：

```json
{
  "model": "happyhorse-1.0-video-edit",
  "input": {
    "prompt": "让视频中的角色穿上参考图中的条纹毛衣",
    "media": [
      {
        "type": "video",
        "url": "https://example.com/input.mp4"
      },
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
    "resolution": "720P",
    "duration": 5
  }
}
```

## 最小校验规则

首版只做最小校验：

- `model` 必须是支持的 HappyHorse 模型。
- `prompt` 必须非空。
- `duration` 缺失时使用 `5`。
- `resolution` 如传入，只接受 `720P` 或 `1080P`。
- `ratio` 如传入，只接受 `16:9`、`9:16` 或 `1:1`。
- `happyhorse-1.0-i2v` 必须能解析出一张首帧图。
- `happyhorse-1.0-r2v` 必须能解析出至少一张参考图。
- `happyhorse-1.0-video-edit` 必须存在 `metadata.video_url`。

其余完整校验交给 HappyHorse 上游返回错误，但实现代码中应写明后续校验项：

- 图片格式、尺寸、大小。
- 参考图数量限制。
- 视频格式、时长、大小、分辨率、帧率。
- `duration` 的完整范围。
- `seed` 范围。
- `watermark` 类型。

## 上游请求

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

上游创建成功响应示例：

```json
{
  "output": {
    "task_status": "PENDING",
    "task_id": "0385dc79-5ff8-4d82-bcb6-xxxxxx"
  },
  "request_id": "4909100c-7b5a-9f92-bfe5-xxxxxx"
}
```

上游查询成功响应示例：

```json
{
  "request_id": "99243b47-ec5f-9413-9993-xxxxxx",
  "output": {
    "task_id": "4673458e-28be-4a05-bf2a-xxxxxx",
    "task_status": "SUCCEEDED",
    "submit_time": "2026-04-20 17:55:17.075",
    "scheduled_time": "2026-04-20 17:55:17.129",
    "end_time": "2026-04-20 17:56:36.658",
    "orig_prompt": "一座纸板城市在夜晚亮起灯光",
    "video_url": "https://dashscope-result.oss-cn-beijing.aliyuncs.com/xxx.mp4"
  },
  "usage": {
    "duration": 5,
    "input_video_duration": 0,
    "output_video_duration": 5,
    "video_count": 1,
    "SR": 720,
    "ratio": "16:9"
  }
}
```

## 返回格式

### `/v1/video/generations`

保留当前 `new-api` 通用任务创建格式，任务 ID 为 `new-api` 自己生成的公开 `task_id`。上游 HappyHorse `task_id` 只写入内部任务数据：

```text
model.Task.PrivateData.UpstreamTaskID
```

### `/v1/video/generations/{task_id}`

保留当前 `new-api` 通用查询格式。完成时直接返回上游 `video_url`，不下载、不转存、不代理、不重写。

上游视频 URL 约束：

- 有效期 24 小时。
- 存储在 Ali OSS 桶。
- 无需签名。

### `/happyhorse/api/generate`

返回 HappyHorse 创建任务格式，但 `output.task_id` 使用 `new-api` 公开任务 ID，不暴露上游 `task_id`。

### `/happyhorse/api/status/{task_id}`

按 HappyHorse 查询格式返回。请求中的 `task_id` 是 `new-api` 公开任务 ID，内部再映射到上游 `task_id` 查询。

## 状态映射

| HappyHorse 状态 | new-api 状态 | 说明 |
| --- | --- | --- |
| `PENDING` | `queued` | 排队中 |
| `RUNNING` | `processing` 或 `in_progress` | 处理中，按当前通用格式实际字段取值 |
| `SUCCEEDED` | `succeeded` | 成功 |
| `FAILED` | `failed` | 失败 |
| `CANCELED` | `failed` | 首版映射为失败 |
| `UNKNOWN` | `failed` | 首版映射为失败 |

`CANCELED` 和 `UNKNOWN` 的原始状态应保留在 metadata 或 HappyHorse 原生响应中。

## 计费规则

HappyHorse 借用 Sora 的计费链路，不借用 Sora 的倍率常量。

统一链路：

```text
ModelPriceHelperPerCall
-> HappyHorse EstimateBilling
-> OtherRatios
-> PreConsumeBilling
-> SettleBilling
-> 成功后补差 / 失败退款
```

价格口径：

```text
720P  = 0.9 元/秒
1080P = 1.6 元/秒
```

建议模型基础价格配置为 720P 单秒价：

```text
basePrice = 0.9
```

预扣公式：

```text
precharge = 0.9 * group_ratio * duration * resolutionRatio
```

倍率：

```text
720P  ratio = 1
1080P ratio = 1.6 / 0.9 = 1.7777778
```

视频编辑只按输出秒数计费，不按输入视频秒数计费。

完成后补差规则：

1. 优先读取 `usage.output_video_duration`。
2. 如果缺失，读取 `usage.duration`。
3. 如果都缺失，保持预扣额度。
4. 对视频编辑，忽略 `usage.input_video_duration`。

## 实施计划

### 1. 新增渠道类型

修改：

- `constant/channel.go`
- `relay/relay_adaptor.go`

要求：

- 新增 `ChannelTypeHappyHorse`。
- 渠道名称映射增加 `HappyHorse`。
- `GetTaskAdaptor` 在 HappyHorse 渠道类型下返回 HappyHorse task adaptor。

### 2. 新增 HappyHorse task adaptor

创建目录：

```text
relay/channel/task/happyhorse
```

建议文件：

```text
adaptor.go
dto.go
convert.go
billing.go
constants.go
```

职责：

- `adaptor.go`：实现 task adaptor 接口。
- `dto.go`：定义 HappyHorse 文档格式和上游响应结构。
- `convert.go`：实现 `/v1/video/generations` 与 HappyHorse canonical request 的转换。
- `billing.go`：实现预扣倍率和完成后补差。
- `constants.go`：维护模型、状态、倍率、默认值。

### 3. 新增 HappyHorse 路由

修改：

- `router/video-router.go`
- `middleware/distributor.go`

新增：

```http
POST /happyhorse/api/generate
GET  /happyhorse/api/status/{task_id}
```

要求：

- 使用 `TokenAuth`。
- 使用 `Distribute`。
- `POST /happyhorse/api/generate` 设置 `RelayModeVideoSubmit`。
- `GET /happyhorse/api/status/{task_id}` 设置 `RelayModeVideoFetchByID`，并从 path 读取 `task_id`。
- HappyHorse 原生入口也必须走 new-api 计费链路。

### 4. 实现创建任务转换

`/v1/video/generations`：

- 解析 `TaskSubmitReq`。
- 按模型名选择 t2v/i2v/r2v/video-edit。
- 生成 HappyHorse canonical request。
- 调用上游 DashScope video synthesis。
- 保存公开 `task_id` 与上游 `task_id` 映射。
- 返回当前 `new-api` 通用任务创建格式。

`/happyhorse/api/generate`：

- 直接解析 HappyHorse 文档格式。
- 规范化并做最小校验。
- 调用同一条提交链路。
- 返回 HappyHorse 创建任务格式。

### 5. 实现查询转换

`/v1/video/generations/{task_id}`：

- 使用公开 `task_id` 查本地任务。
- 使用 `PrivateData.UpstreamTaskID` 查询上游。
- 更新本地任务状态。
- 返回当前 `new-api` 通用查询格式。

`/happyhorse/api/status/{task_id}`：

- query 参数 `task_id` 为公开任务 ID。
- 内部映射到上游任务 ID。
- 返回 HappyHorse 查询格式。

### 6. 实现计费和补差

- `EstimateBilling` 返回 `duration` 和 `resolution` 两个倍率。
- `duration` 使用请求值或默认 `5`。
- `resolution` 为 `720P` 时倍率 `1`。
- `resolution` 为 `1080P` 时倍率 `1.7777778`。
- `AdjustBillingOnComplete` 读取上游 usage 并按输出秒数重算 quota。
- 失败任务走现有 `RefundTaskQuota`。

补充确认：

- HappyHorse 模型需要排除 `PerCallBilling`。
- `PerCallBilling=true` 时，轮询完成结算会跳过 `AdjustBillingOnComplete`。
- HappyHorse 需要在任务完成后读取上游 `usage.output_video_duration`，按实际输出秒数重算额度。
- 因此 `controller/relay.go` 中对 `happyhorse-` 模型的 `PerCallBilling` 例外为有意保留，不是残留改动。

### 7. 测试

建议新增测试覆盖：

- t2v 字段转换。
- i2v 图片优先级：`image > images[0] > input_reference`。
- r2v `metadata.media` 直通。
- r2v `images[]` 转 `reference_image`。
- video-edit `metadata.video_url` 必填。
- video-edit `metadata.reference_images` 转 `reference_image`。
- `duration` 默认 `5`。
- `resolution=720P` 倍率为 `1`。
- `resolution=1080P` 倍率为 `1.7777778`。
- `ratio` 仅接受 `16:9`、`9:16`、`1:1`。
- `/happyhorse/api/generate` 接受 HappyHorse 文档格式。
- `/happyhorse/api/status/{task_id}` 查询公开任务 ID。
- `SUCCEEDED` 直接返回上游 `video_url`。
- `FAILED` 保存错误信息。
- `CANCELED` 和 `UNKNOWN` 映射为 failed。
- 成功后按 `usage.output_video_duration` 补差。
- `usage.output_video_duration` 缺失时按 `usage.duration` 补差。
- usage 缺失时保持预扣。
- 失败任务退款。

## 全量测试失败拆解与处理记录

在 HappyHorse 功能测试后，全量 `go test ./...` 暴露出 5 类问题。处理结论如下：

### 1. 缺少 `web/dist`

类型：环境 / 构建产物问题。

根包 `main.go` embed 前端构建产物，测试环境未生成 `web/dist` 时会报：

```text
pattern web/dist: no matching files found
```

处理方案：

- 本地或 CI 跑根包测试前执行 `bun install && bun run build`。
- 只验证后端逻辑时，可暂时排除根包。

### 2. `scripts` 缺少 `jq`

类型：环境依赖问题。

`scripts` 测试依赖 `jq`。Docker 测试镜像未安装 `jq` 时会失败。

处理方案：

- 在测试容器内安装 `jq`。
- 或只做后端包验证时排除 `scripts` 包。

### 3. Redis nil panic

类型：测试环境 Redis 未初始化导致的 nil pointer。

修复点：

- `controller/wisemodel_package.go`：`common.RDB.Set(...)` 前增加 `common.RDB != nil` 守卫。
- `common/redis.go`：`RedisDel` / `RedisDelKey` 增加 `RDB == nil` 直接返回，避免测试环境后台 goroutine 清缓存时 panic。

### 4. Claude 文件类型转换

类型：既有兼容逻辑与测试期望不一致。

修复点：

- `relay/channel/claude/relay-claude.go` 从 `GetFile().FileName` 提取扩展名。
- 使用 `service.GetMimeTypeByExtension` 推导 MIME。
- 当 `mimeType == ""` 或 `mimeType == "application/octet-stream"` 时，用文件名推导结果覆盖。
- `text/*` 转 Claude `text`，并将 base64 解码为字符串。
- `application/pdf` 转 Claude `document`。
- `image/*` 转 Claude `image`。
- 其他不支持类型跳过。

### 5. StreamStatus 预初始化错误丢失

类型：已有 `StreamStatus` 被无条件覆盖，导致预记录错误计数丢失。

修复点：

```go
if info.StreamStatus == nil {
	info.StreamStatus = relaycommon.NewStreamStatus()
}
```

### 6. HappyHorse `PerCallBilling` 例外

类型：计费链路必要设计。

HappyHorse 借用 task 计费链路，但不能被标记为完成后不补差的按次计费任务。否则轮询完成时会跳过 `AdjustBillingOnComplete`，无法按上游输出秒数重新结算。

因此需要保留：

```go
!strings.HasPrefix(relayInfo.OriginModelName, "happyhorse-")
```

## 已验证命令

```powershell
go test ./controller -run TestDeleteWisemodelUser -count=1 -v
go test ./relay/channel/claude -count=1 -v
go test ./relay/helper -run TestStreamScannerHandler_StreamStatus_PreInitialized -count=1
go test ./relay/channel/task/happyhorse ./relay/channel/happyhorse -count=1
```

## 接受标准

- HappyHorse 模型可以通过 `/v1/video/generations` 创建任务。
- HappyHorse 文档格式可以通过 `/happyhorse/api/generate` 创建任务。
- `/v1/video/generations/{task_id}` 保持当前通用查询格式。
- `/happyhorse/api/status/{task_id}` 返回 HappyHorse 查询格式。
- 对外只暴露 `new-api` 公开任务 ID，不暴露上游任务 ID。
- 完成结果直接返回上游 `video_url`。
- 所有入口都走 new-api 计费，不存在绕过余额或分组倍率的路径。
- 预扣和完成后补差符合 HappyHorse 720P/1080P 单秒价格。
- 首版不支持 callback。
- 首版不自动推断分辨率和比例。
