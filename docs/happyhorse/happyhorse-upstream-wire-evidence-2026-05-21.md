# HappyHorse 上游请求体取证报告

日期：2026-05-21

## 1. 取证目的

本次取证用于回应评审中的 P0 问题：HappyHorse 适配器使用的结构化请求体是否能被真实上游接受。

结论：真实上游 `https://dashscope.aliyuncs.com/api/v1/services/aigc/video-generation/video-synthesis` 已接受 HappyHorse 文档格式请求体，并返回真实上游任务 ID；该任务最终状态为 `SUCCEEDED`。

## 2. 取证方式

未修改业务代码。使用 HappyHorse 渠道中配置的 API Key，直接向真实上游端点发送 HappyHorse 文档格式 JSON。

请求头包含：

```text
Content-Type: application/json
X-DashScope-Async: enable
Authorization: Bearer <REDACTED_API_KEY>
```

## 3. 实际发送的请求体

```json
{
  "model": "happyhorse-1.0-t2v",
  "input": {
    "prompt": "A white horse walks slowly on a grassland, cinematic shot"
  },
  "parameters": {
    "resolution": "720P",
    "ratio": "16:9",
    "duration": 5
  }
}
```

关键点：

- 使用 `model/input/parameters` 结构。
- 使用 `parameters.ratio`，不是 Ali schema 的 `parameters.size`。
- 未使用 `input.img_url`、`input.first_frame_url`、`input.last_frame_url`。

## 4. 上游提交响应

```json
{
  "request_id": "a70bf1e9-9e07-9cf0-ba53-473fdb5fb2f2",
  "output": {
    "task_id": "dc8b56df-6ace-4fb0-b36a-7a556d4ad170",
    "task_status": "PENDING"
  }
}
```

上游返回 `PENDING` 和真实 `task_id`，说明真实上游接受了该请求体并创建了任务。

## 5. 上游最终结果

```json
{
  "request_id": "a2e1ea80-65cf-981b-b9ec-92ab87733faf",
  "output": {
    "task_id": "dc8b56df-6ace-4fb0-b36a-7a556d4ad170",
    "task_status": "SUCCEEDED",
    "submit_time": "2026-05-21 18:22:06.274",
    "scheduled_time": "2026-05-21 18:22:06.302",
    "end_time": "2026-05-21 18:23:30.577",
    "orig_prompt": "A white horse walks slowly on a grassland, cinematic shot",
    "video_url": "https://dashscope-a717.oss-accelerate.aliyuncs.com/.../refiner_watermark.mp4?<REDACTED_SIGNED_QUERY>"
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

视频 URL 的 OSS 签名 query 已脱敏。

## 6. 脱敏 curl trace 摘要

```text
POST /api/v1/services/aigc/video-generation/video-synthesis HTTP/1.1
Host: dashscope.aliyuncs.com
Authorization: Bearer <REDACTED_API_KEY>
Content-Type: application/json
X-DashScope-Async: enable

{
  "model": "happyhorse-1.0-t2v",
  "input": {
    "prompt": "A white horse walks slowly on a grassland, cinematic shot"
  },
  "parameters": {
    "resolution": "720P",
    "ratio": "16:9",
    "duration": 5
  }
}
```

完整原始 trace 和临时响应文件不纳入仓库，避免误提交 Authorization 或带签名 query 的 OSS URL。本文档仅保留脱敏后的关键证据。

## 7. 对评审问题的回应

评审中指出 HappyHorse schema 与 Ali adapter schema 不一致，这一事实成立。

本次取证补充说明：当前 HappyHorse 文档格式请求体已经被真实 DashScope 上游接受，并成功完成任务。因此，HappyHorse 不复用 Ali 扁平 schema 是当前实现的有效路径，而不是请求体无法工作的缺陷。

后续仍可将 `BuildRequestBody` 单测作为代码级证据，证明 new-api 适配器构造的请求体与本次真实上游请求体保持同类结构。

