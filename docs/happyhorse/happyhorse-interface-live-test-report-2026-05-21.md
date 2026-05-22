# HappyHorse 接口实测报告

测试时间：2026-05-21 09:55:40 - 10:05:45  
测试环境：Docker 容器 `new-api`，镜像 `dreamlx/new-api:latest`，服务地址 `http://localhost:3000`  
测试渠道：`happyhorse`，渠道类型 `59`，上游地址 `https://dashscope.aliyuncs.com`  
测试依据：[HappyHorse 接口文档](./happyhorse-api-reference.md)  
原始结果目录：`tmp-happyhorse-live-2026-05-21`

## 1. 结论

| 接口 | 覆盖范围 | 结论 |
| --- | --- | --- |
| `POST /happyhorse/api/generate` | 4 个 HappyHorse 内部模型 | 通过 |
| `GET /happyhorse/api/status/{task_id}` | 4 个 HappyHorse 内部模型查询 | 通过 |
| `POST /v1/video/generations` | 4 个 HappyHorse 内部模型 | 通过 |
| `GET /v1/video/generations/{task_id}` | 4 个 HappyHorse 内部模型查询 | 通过 |
| `happyhorse-1.0/video` 清除校验 | 提交旧官网聚合模型名 | 通过，返回 `unsupported model` |
| 不存在任务查询 | `GET /happyhorse/api/status/{task_id}` | 通过，返回 `task_not_exist` |

本轮共提交 8 个真实生成任务，全部完成成功；2 个负向校验均符合预期。

## 2. 测试用例总览

| 用例 | 接口 | 模型 | task_id | 最终状态 | 输出秒数 | 结果 |
| --- | --- | --- | --- | --- | ---: | --- |
| HH-NATIVE-01 | `/happyhorse/api/generate` + `/happyhorse/api/status/{task_id}` | `happyhorse-1.0-t2v` | `task_AalyYgWmuj39OEu2NcBMhRu6482P0hvC` | `completed` | 5 | 通过 |
| HH-NATIVE-02 | `/happyhorse/api/generate` + `/happyhorse/api/status/{task_id}` | `happyhorse-1.0-i2v` | `task_lxCOo68VKRxsGXXItVb6DWK1OvxAJJXa` | `completed` | 5 | 通过 |
| HH-NATIVE-03 | `/happyhorse/api/generate` + `/happyhorse/api/status/{task_id}` | `happyhorse-1.0-r2v` | `task_ghWvf0bXjPMncrtFBY9tXi0ttrkJ742X` | `completed` | 5 | 通过 |
| HH-NATIVE-04 | `/happyhorse/api/generate` + `/happyhorse/api/status/{task_id}` | `happyhorse-1.0-video-edit` | `task_yrq5qY3l4djyirKBsoYnhuzU5HByN3Sy` | `completed` | 13 | 通过 |
| HH-V1-01 | `/v1/video/generations` + `/v1/video/generations/{task_id}` | `happyhorse-1.0-t2v` | `task_bbfOf1i1FPno8fhK021uS2vVR7JjYEN6` | `SUCCESS` | 5 | 通过 |
| HH-V1-02 | `/v1/video/generations` + `/v1/video/generations/{task_id}` | `happyhorse-1.0-i2v` | `task_5GpN965kyfR21zh6hz2DMPkf7YfUp5Cx` | `SUCCESS` | 5 | 通过 |
| HH-V1-03 | `/v1/video/generations` + `/v1/video/generations/{task_id}` | `happyhorse-1.0-r2v` | `task_CcxhH3ZuKT5D342CVwMlpgLB1JRcCu3h` | `SUCCESS` | 5 | 通过 |
| HH-V1-04 | `/v1/video/generations` + `/v1/video/generations/{task_id}` | `happyhorse-1.0-video-edit` | `task_dn778ahmcdyagXlzULYkAQ5kNABQzHTH` | `SUCCESS` | 13.9 | 通过 |

## 3. `/happyhorse/api/generate` 原生结构化接口

### HH-NATIVE-01：T2V 文生视频

请求：

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

提交响应：

```json
{
  "task_id": "task_AalyYgWmuj39OEu2NcBMhRu6482P0hvC",
  "status": "pending"
}
```

最终查询响应摘要：

```json
{
  "task_id": "task_AalyYgWmuj39OEu2NcBMhRu6482P0hvC",
  "status": "completed",
  "data": {
    "model": "happyhorse-1.0-t2v",
    "mode": "text-to-video",
    "duration": 5,
    "aspect_ratio": "16:9",
    "video_url": "https://dashscope-a717.oss-accelerate.aliyuncs.com/...refiner_watermark.mp4?...",
    "resultUrls": ["https://dashscope-a717.oss-accelerate.aliyuncs.com/...refiner_watermark.mp4?..."]
  }
}
```

原始文件：

- `tmp-happyhorse-live-2026-05-21/native-t2v-request.json`
- `tmp-happyhorse-live-2026-05-21/native-t2v-submit.json`
- `tmp-happyhorse-live-2026-05-21/native-t2v-status-latest.json`

### HH-NATIVE-02：I2V 图生视频

请求：

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

提交响应：

```json
{
  "task_id": "task_lxCOo68VKRxsGXXItVb6DWK1OvxAJJXa",
  "status": "pending"
}
```

最终查询响应摘要：

```json
{
  "task_id": "task_lxCOo68VKRxsGXXItVb6DWK1OvxAJJXa",
  "status": "completed",
  "data": {
    "model": "happyhorse-1.0-i2v",
    "mode": "image-to-video",
    "duration": 5,
    "video_url": "https://dashscope-a717.oss-accelerate.aliyuncs.com/...refiner_watermark.mp4?...",
    "resultUrls": ["https://dashscope-a717.oss-accelerate.aliyuncs.com/...refiner_watermark.mp4?..."]
  }
}
```

结论：通过。`data.model` 已返回内部模型名 `happyhorse-1.0-i2v`。

原始文件：

- `tmp-happyhorse-live-2026-05-21/native-i2v-request.json`
- `tmp-happyhorse-live-2026-05-21/native-i2v-submit.json`
- `tmp-happyhorse-live-2026-05-21/native-i2v-status-latest.json`

### HH-NATIVE-03：R2V 参考生视频

请求：

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

提交响应：

```json
{
  "task_id": "task_ghWvf0bXjPMncrtFBY9tXi0ttrkJ742X",
  "status": "pending"
}
```

最终查询响应摘要：

```json
{
  "task_id": "task_ghWvf0bXjPMncrtFBY9tXi0ttrkJ742X",
  "status": "completed",
  "data": {
    "model": "happyhorse-1.0-r2v",
    "mode": "reference-to-video",
    "duration": 5,
    "aspect_ratio": "16:9",
    "video_url": "https://dashscope-a717.oss-accelerate.aliyuncs.com/...refiner_watermark.mp4?...",
    "resultUrls": ["https://dashscope-a717.oss-accelerate.aliyuncs.com/...refiner_watermark.mp4?..."]
  }
}
```

原始文件：

- `tmp-happyhorse-live-2026-05-21/native-r2v-request.json`
- `tmp-happyhorse-live-2026-05-21/native-r2v-submit.json`
- `tmp-happyhorse-live-2026-05-21/native-r2v-status-latest.json`

### HH-NATIVE-04：Video Edit 视频编辑

请求：

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

提交响应：

```json
{
  "task_id": "task_yrq5qY3l4djyirKBsoYnhuzU5HByN3Sy",
  "status": "pending"
}
```

最终查询响应摘要：

```json
{
  "task_id": "task_yrq5qY3l4djyirKBsoYnhuzU5HByN3Sy",
  "status": "completed",
  "data": {
    "model": "happyhorse-1.0-video-edit",
    "mode": "video-edit",
    "duration": 13,
    "video_url": "https://dashscope-a717.oss-accelerate.aliyuncs.com/...merged.mp4?...",
    "resultUrls": ["https://dashscope-a717.oss-accelerate.aliyuncs.com/...merged.mp4?..."]
  }
}
```

结论：通过。上游 `usage.output_video_duration=13.9`，原生状态响应中 `duration` 返回整数 `13`。

原始文件：

- `tmp-happyhorse-live-2026-05-21/native-video-edit-request.json`
- `tmp-happyhorse-live-2026-05-21/native-video-edit-submit.json`
- `tmp-happyhorse-live-2026-05-21/native-video-edit-status-latest.json`

## 4. `/v1/video/generations` 通用接口

### HH-V1-01：T2V 文生视频

请求：

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

提交响应：

```json
{
  "id": "task_bbfOf1i1FPno8fhK021uS2vVR7JjYEN6",
  "task_id": "task_bbfOf1i1FPno8fhK021uS2vVR7JjYEN6",
  "object": "video",
  "model": "happyhorse-1.0-t2v",
  "status": "queued",
  "progress": 0,
  "created_at": 1779328540
}
```

最终查询响应摘要：

```json
{
  "code": "success",
  "data": {
    "task_id": "task_bbfOf1i1FPno8fhK021uS2vVR7JjYEN6",
    "status": "SUCCESS",
    "quota": 2250000,
    "progress": "100%",
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

原始文件：

- `tmp-happyhorse-live-2026-05-21/v1-t2v-request.json`
- `tmp-happyhorse-live-2026-05-21/v1-t2v-submit.json`
- `tmp-happyhorse-live-2026-05-21/v1-t2v-status-latest.json`

### HH-V1-02：I2V 图生视频

请求：

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

提交响应：

```json
{
  "id": "task_5GpN965kyfR21zh6hz2DMPkf7YfUp5Cx",
  "task_id": "task_5GpN965kyfR21zh6hz2DMPkf7YfUp5Cx",
  "object": "video",
  "model": "happyhorse-1.0-i2v",
  "status": "queued",
  "progress": 0,
  "created_at": 1779328541
}
```

最终查询响应摘要：

```json
{
  "code": "success",
  "data": {
    "task_id": "task_5GpN965kyfR21zh6hz2DMPkf7YfUp5Cx",
    "status": "SUCCESS",
    "quota": 2250000,
    "progress": "100%",
    "properties": {
      "upstream_model_name": "happyhorse-1.0-i2v",
      "origin_model_name": "happyhorse-1.0-i2v"
    },
    "data": {
      "usage": {
        "SR": 720,
        "duration": 5,
        "output_video_duration": 5
      }
    }
  }
}
```

结论：通过。`image` 已转换为 HappyHorse `media[type=first_frame]`。

原始文件：

- `tmp-happyhorse-live-2026-05-21/v1-i2v-request.json`
- `tmp-happyhorse-live-2026-05-21/v1-i2v-submit.json`
- `tmp-happyhorse-live-2026-05-21/v1-i2v-status-latest.json`

### HH-V1-03：R2V 参考生视频

请求：

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

提交响应：

```json
{
  "id": "task_CcxhH3ZuKT5D342CVwMlpgLB1JRcCu3h",
  "task_id": "task_CcxhH3ZuKT5D342CVwMlpgLB1JRcCu3h",
  "object": "video",
  "model": "happyhorse-1.0-r2v",
  "status": "queued",
  "progress": 0,
  "created_at": 1779328541
}
```

最终查询响应摘要：

```json
{
  "code": "success",
  "data": {
    "task_id": "task_CcxhH3ZuKT5D342CVwMlpgLB1JRcCu3h",
    "status": "SUCCESS",
    "quota": 2250000,
    "progress": "100%",
    "properties": {
      "upstream_model_name": "happyhorse-1.0-r2v",
      "origin_model_name": "happyhorse-1.0-r2v"
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

结论：通过。`images[]` 已转换为 HappyHorse `media[type=reference_image]`。

原始文件：

- `tmp-happyhorse-live-2026-05-21/v1-r2v-request.json`
- `tmp-happyhorse-live-2026-05-21/v1-r2v-submit.json`
- `tmp-happyhorse-live-2026-05-21/v1-r2v-status-latest.json`

### HH-V1-04：Video Edit 视频编辑

请求：

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

提交响应：

```json
{
  "id": "task_dn778ahmcdyagXlzULYkAQ5kNABQzHTH",
  "task_id": "task_dn778ahmcdyagXlzULYkAQ5kNABQzHTH",
  "object": "video",
  "model": "happyhorse-1.0-video-edit",
  "status": "queued",
  "progress": 0,
  "created_at": 1779328541
}
```

最终查询响应摘要：

```json
{
  "code": "success",
  "data": {
    "task_id": "task_dn778ahmcdyagXlzULYkAQ5kNABQzHTH",
    "status": "SUCCESS",
    "quota": 2250000,
    "progress": "100%",
    "properties": {
      "upstream_model_name": "happyhorse-1.0-video-edit",
      "origin_model_name": "happyhorse-1.0-video-edit"
    },
    "data": {
      "usage": {
        "SR": 720,
        "duration": 27.8,
        "input_video_duration": 13.9,
        "output_video_duration": 13.9
      }
    }
  }
}
```

结论：通过。`metadata.video_url` 已转换为 HappyHorse `media[type=video]`；结算使用 `output_video_duration=13.9`，不按输入视频秒数单独计费。

原始文件：

- `tmp-happyhorse-live-2026-05-21/v1-video-edit-request.json`
- `tmp-happyhorse-live-2026-05-21/v1-video-edit-submit.json`
- `tmp-happyhorse-live-2026-05-21/v1-video-edit-status-latest.json`

## 5. 计费观察

本轮测试模型价格使用默认 `model_price=0.9`，`QuotaPerUnit=500000`，`group_ratio=1`，分辨率 `720P` 的倍率为 `1.0`。

| 场景 | 预扣 quota | 上游输出秒数 | 实际应计 quota | 补差日志 |
| --- | ---: | ---: | ---: | --- |
| T2V / I2V / R2V | 2250000 | 5 | 2250000 | 无补差 |
| Native Video Edit | 2250000 | 13.9 | 6255000 | 补扣 4005000 |
| V1 Video Edit | 2250000 | 13.9 | 6255000 | 补扣 4005000 |

数据库日志观察：

| 日志范围 | 说明 |
| --- | --- |
| `logs.id=797` - `804` | 8 个任务提交时各预扣 `2250000` |
| `logs.id=805` | Native Video Edit 完成后补扣 `4005000` |
| `logs.id=806` | V1 Video Edit 完成后补扣 `4005000` |

任务表观察：

- `tasks.quota` 当前仍记录提交时预扣值 `2250000`。
- 实际补差体现在 `logs` 额度流水中。
- 这与现有任务计费链路行为一致：任务完成后会做余额补差，但任务表 `quota` 字段保留预扣值。

## 6. 负向校验

### HH-NEG-01：旧官网聚合模型名已不支持

请求：

```json
{
  "model": "happyhorse-1.0/video",
  "input": {
    "prompt": "This should be rejected"
  },
  "parameters": {
    "resolution": "720P",
    "ratio": "16:9",
    "duration": 5
  }
}
```

响应：

```json
{
  "code": "invalid_request",
  "message": "unsupported model: happyhorse-1.0/video; supported models: happyhorse-1.0-t2v, happyhorse-1.0-i2v, happyhorse-1.0-r2v, happyhorse-1.0-video-edit",
  "data": null
}
```

结论：通过。Go 源码清除 `happyhorse-1.0/video` 后，接口已不再接受该模型名。

### HH-NEG-02：不存在任务 path 查询

请求：

```http
GET /happyhorse/api/status/task_not_exist_probe_20260521
```

响应：

```json
{
  "code": "task_not_exist",
  "message": "task_not_exist",
  "data": null
}
```

结论：通过。`/happyhorse/api/status/{task_id}` path 路由有效。

## 7. 遗留观察

当前数据库渠道配置中仍可看到历史模型列表包含 `happyhorse-1.0/video`：

```text
happyhorse-1.0/video,happyhorse-1.0-t2v,happyhorse-1.0-i2v,happyhorse-1.0-r2v,happyhorse-1.0-video-edit
```

这不会使当前代码重新支持该模型名；本轮负向校验已确认提交 `happyhorse-1.0/video` 会返回 `unsupported model`。建议后台渠道模型列表清理为：

```text
happyhorse-1.0-t2v,happyhorse-1.0-i2v,happyhorse-1.0-r2v,happyhorse-1.0-video-edit
```
