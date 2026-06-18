# HappyHorse 接口测试报告

更新时间：2026-05-22

## 1. 报告说明

本报告记录 HappyHorse 接口测试结论。2026-05-21 的真实上游测试证明 4 个模型基础链路可用；2026-05-22 代码契约已更新，默认分辨率、参数校验和 Video Edit 计费规则发生变化。

历史真实任务中使用的 `720P` 请求仍可作为“链路可用”证据，但当前新请求应按最新接口文档执行：

- 默认 resolution 为 `1080P`。
- T2V/I2V/R2V duration 显式传入必须为 `3-15`。
- Video Edit 不支持 duration。
- I2V 和 Video Edit 不支持 ratio。
- T2V/R2V 支持 `4:3`、`3:4`。
- `quality`、`sound` 不再透传。
- Video Edit 按输入视频秒数加输出视频秒数计费。

## 2. 测试范围

| 接口 | 覆盖 |
| --- | --- |
| `POST /happyhorse/api/generate` | 4 个 HappyHorse 内部模型 |
| `GET /happyhorse/api/status/{task_id}` | 4 个 HappyHorse 内部模型查询 |
| `POST /v1/video/generations` | 4 个 HappyHorse 内部模型 |
| `GET /v1/video/generations/{task_id}` | 4 个 HappyHorse 内部模型查询 |
| 参数负向测试 | duration、ratio、media 数量、URL、旧模型名 |
| 计费观察 | 普通模型输出秒数；Video Edit 输入+输出秒数 |

## 3. 真实上游成功结果

以下任务来自 2026-05-21/2026-05-22 的容器实测，证明基础提交、轮询、结果 URL 返回链路可用。

| 用例 | 接口 | 模型 | 结果 | 说明 |
| --- | --- | --- | --- | --- |
| HH-NATIVE-01 | `/happyhorse/api/generate` | `happyhorse-1.0-t2v` | 通过 | 返回 `completed`，有视频 URL |
| HH-NATIVE-02 | `/happyhorse/api/generate` | `happyhorse-1.0-i2v` | 通过 | 返回 `completed`，有视频 URL |
| HH-NATIVE-03 | `/happyhorse/api/generate` | `happyhorse-1.0-r2v` | 通过 | 返回 `completed`，有视频 URL |
| HH-NATIVE-04 | `/happyhorse/api/generate` | `happyhorse-1.0-video-edit` | 通过 | 返回 `completed`，有视频 URL |
| HH-V1-01 | `/v1/video/generations` | `happyhorse-1.0-t2v` | 通过 | 返回 `SUCCESS`，有视频 URL |
| HH-V1-02 | `/v1/video/generations` | `happyhorse-1.0-i2v` | 通过 | 返回 `SUCCESS`，有视频 URL |
| HH-V1-03 | `/v1/video/generations` | `happyhorse-1.0-r2v` | 通过 | 返回 `SUCCESS`，有视频 URL |
| HH-V1-04 | `/v1/video/generations` | `happyhorse-1.0-video-edit` | 通过 | 返回 `SUCCESS`，有视频 URL |

## 4. 当前推荐测试请求

### Native T2V

```json
{
  "model": "happyhorse-1.0-t2v",
  "input": {
    "prompt": "A white horse walks slowly on grass, natural daylight"
  },
  "parameters": {
    "resolution": "1080P",
    "ratio": "16:9",
    "duration": 5
  }
}
```

### Native I2V

```json
{
  "model": "happyhorse-1.0-i2v",
  "input": {
    "prompt": "Animate the image with gentle natural motion",
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

### Native R2V

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

### Native Video Edit

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

### V1 Video Edit

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

## 5. 负向测试结果

| 用例 | 请求 | 预期 |
| --- | --- | --- |
| 旧模型名 | `model=happyhorse-1.0/video` | 400，`unsupported model` |
| T2V duration 2 | 显式 `duration=2` | 400，`duration must be between 3 and 15 seconds` |
| T2V duration 16 | 显式 `duration=16` | 400，`duration must be between 3 and 15 seconds` |
| Video Edit duration | 显式传 `duration` | 400，`video-edit does not support duration parameter` |
| I2V ratio | I2V 传 `ratio` | 400，`i2v does not support ratio parameter` |
| Video Edit ratio | Video Edit 传 `ratio` | 400，`happyhorse-1.0-video-edit does not support ratio parameter` |
| I2V 多首帧 | 传多张首帧 | 400，`exactly 1 first_frame` |
| R2V 10 张参考图 | 10 个 `reference_image` | 400，`at most 9 reference images` |
| Video Edit 2 个 video | 2 个 `video` | 400，`exactly 1 video` |
| Video Edit 6 张参考图 | 6 个 `reference_image` | 400，`at most 5 reference images` |
| 空 URL | `images[]` 或 `reference_images[]` 含空字符串 | 400，`contains empty url` |
| 非法图片媒体 | `ftp://...` 或非法图片 base64 data URL | 400，`image base64 data url` |
| 非法视频媒体 | `data:video/mp4;base64,...` 或非 http/https 视频 URL | 400，`video media must use http or https url` |

## 6. 计费观察

基础价格：

```text
model_price = 0.9
QuotaPerUnit = 500000
```

当前计费口径：

- 720P 5 秒：`0.9 * 500000 * 5 = 2250000`
- 1080P 5 秒：`0.9 * 500000 * 5 * (1.6 / 0.9) = 4000000`
- Video Edit：实际秒数为 `input_video_duration + output_video_duration`

示例：Video Edit 上游返回：

```json
{
  "usage": {
    "input_video_duration": 13.9,
    "output_video_duration": 13.9,
    "SR": 720
  }
}
```

实际扣费：

```text
actual_seconds = 13.9 + 13.9 = 27.8
actual_quota = 0.9 * 500000 * 27.8 = 12510000
```

如果提交时显式指定 `720P`，按预估 5 秒预扣 `2250000`，完成后补扣：

```text
12510000 - 2250000 = 10260000
```

如果未传 resolution，当前默认 `1080P`，预扣会按 1080P 倍率计算。

## 7. 验证命令

```powershell
go test ./relay/channel/task/happyhorse ./relay/channel/happyhorse -count=1
go vet ./relay/channel/task/happyhorse/... ./relay/channel/happyhorse/...
go build ./...
```

## 8. 结论

当前测试结论：

- 4 个模型的基础成功链路可用。
- 原生入口和 V1 入口均已补齐主要参数边界校验。
- Video Edit 计费已修正为输入+输出秒数。
- `usage.SR` 已兼容数字和字符串。
- 文档中的旧 720P 默认、`quality/sound`、Video Edit duration、只按输出秒数计费等口径已废弃。
