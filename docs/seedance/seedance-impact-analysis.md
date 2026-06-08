# Seedance 2.0 影响分析报告

更新时间：2026-06-04

## 1. 范围

本报告分析 Seedance 2.0 独立渠道对 new-api 的影响。当前功能新增独立渠道类型 `60`，并支持：

```http
POST /v1/video/generations
GET  /v1/video/generations/{task_id}

POST /api/v3/contents/generations/tasks
GET  /api/v3/contents/generations/tasks/{task_id}
```

## 2. 主要代码影响

| 区域 | 文件 | 影响 |
| --- | --- | --- |
| 渠道类型 | `constant/channel.go`、`constant/api_type.go`、`common/api_type.go` | 新增 Seedance 类型（60），Dummy 后移为 61 |
| 分发 | `middleware/distributor.go` | `/api/v3/contents/generations/tasks` 进入任务 relay 分发 |
| 路由 | `router/video-router.go` | 新增火山官方兼容提交和查询路由 |
| 适配器注册 | `relay/relay_adaptor.go` | 注册 Seedance 普通 adaptor 和 task adaptor |
| 任务链路 | `relay/channel/task/seedance/*` | 新增请求转换、校验、状态解析、条件倍率 |
| Volcano 共享包 | `relay/channel/task/taskcommon/volcano/*` | 从 doubao/seedance 提取的共享 DTO 和辅助方法 |
| Doubao adaptor | `relay/channel/task/doubao/adaptor.go` | 改写为使用 volcano 共享包 |
| 响应格式 | `relay/relay_task.go` | 查询时支持火山官方兼容响应形态 |
| TaskSubmitReq | `relay/common/` | 新增 `Videos`、`Audios` 数组字段 |
| 前端 | `web/default/src/features/channels` | 新增 Seedance 渠道类型显示和图标映射 |
| 文档 | `docs/seedance/*` | 新增接口、管理、计费、开发和影响说明 |

## 3. 功能影响

Seedance 模型：

```text
dreamina-seedance-2-0-260128
dreamina-seedance-2-0-fast-260128
doubao-seedance-2-0-260128
doubao-seedance-2-0-fast-260128
```

当前请求契约：

- 默认分辨率为 `720p`。
- 支持分辨率 `480p`、`720p`、`1080p`；Fast 版本仅 `480p`、`720p`。
- duration 支持 `5`、`10`，缺省 `5`。
- 支持图生视频（`images[]`）、视频续写（`videos[]`）、音频驱动（`audios[]`）。
- prompt 必填。
- 火山 content[] 格式支持 `text`、`image_url`、`video`、`audio` 四种 type。

## 4. 计费影响

Seedance 使用 `model_ratio` 按 token 计费，复用 new-api 统一任务计费链路：

```text
EstimateBilling -> PreConsumeBilling -> SettleBilling -> AdjustBillingOnComplete
```

预扣公式：

```text
model_ratio / 2 × QuotaPerUnit × group_ratio × seedance_condition_ratio
```

完成结算公式：

```text
completion_tokens × model_ratio × group_ratio × seedance_condition_ratio
```

Token 优先级：

1. 优先 `usage.completion_tokens`
2. 缺失或为 0 时 `usage.total_tokens`
3. 两者都缺失时保持预扣额度

条件倍率：

- 输入含视频时自动折扣（主版本 ×0.6087，Fast 版本 ×0.5946）
- 1080p 时自动加价（不含视频 ×1.1087，含视频 ×0.6739）
- 基准为 480p/720p、不含视频 = 1.0

影响说明：

- 使用 `model_ratio` 而非 `model_price`，不会触发 `PerCallBilling`，任务完成后可进入 token 重算补差。
- Seedance 不需要加入 `TaskPricePatches`。
- 不需要改通用任务结算接口。
- 前端"模型计费"页面保存时会自动 `/2`，管理员需输入两倍值。

## 5. 查询和轮询影响

查询接口读本地 DB，不实时拉上游。任务推进、成功补差、失败退款依赖后台轮询。

部署要求：

- 至少一个 master 节点运行。
- master 节点开启 `UPDATE_TASK=true`。
- 纯 slave 部署可能导致任务长期停留在 pending/queued，且无法完成补差或退款。

查询响应格式按路径区分：

| 路径 | 响应格式 |
| --- | --- |
| `/v1/video/generations/{task_id}` | 通用 TaskDto 格式 |
| `/api/v3/contents/generations/tasks/{task_id}` | 原始上游 ResponseTask 格式 |
| `/v1/videos/{task_id}` | OpenAI Video API 格式（通过 `ConvertToOpenAIVideo`） |

## 6. 对 doubao 渠道的影响

本次开发从 doubao adaptor 中提取了共享 Volcano 兼容代码到 `volcano` 包。doubao adaptor 已改写为使用该共享包，引入两处行为变化：

### 6.1 ParseTaskResult 变化

| | 旧行为 | 新行为 |
| --- | --- | --- |
| Token 处理 | 无条件同时设 `CompletionTokens` 和 `TotalTokens` | 优先 `completion_tokens`，fallback `total_tokens` |

影响：结果一致或更准确。当上游同时返回 `completion_tokens` 和 `total_tokens` 且两者不同时，新行为优先使用 `completion_tokens`，更符合 Seedance 2.0 设计文档的 token 选择优先级。对 doubao 现有使用场景无负面影响。

### 6.2 HasVideoInMetadata 变化

| | 旧行为 | 新行为 |
| --- | --- | --- |
| 视频类型检测 | 只检查 `type=video_url` | 额外检查 `type=video`（超集） |

影响：覆盖更多视频类型，不会漏检。对 doubao 现有使用场景为正向影响。

### 6.3 不受影响的部分

- doubao adaptor 的 `ValidateRequestAndSetAction`、`BuildRequestBody`、`convertToRequestPayload` 等核心逻辑未变。
- doubao 的计费规则（`EstimateBilling`）未变，仍使用 `video_input` 条件倍率 key。
- doubao 的 `ConvertToOpenAIVideo` 未变。
- doubao 的模型列表、渠道名称未变。

## 7. 对其他任务渠道的影响

| 渠道 | 影响 |
| --- | --- |
| Midjourney | 无影响 |
| Suno | 无影响 |
| HappyHorse | 无影响 |
| Kling | 无影响 |
| Gemini / Vertex | 无影响 |
| 其他视频任务渠道 | 无影响 |

`BaseBilling` 默认返回 `false`，其他 task adaptor 通过嵌入 `BaseBilling` 保持原行为。

`videoFetchByIDRespBodyBuilder` 新增了火山官方兼容路径的响应格式处理（`/api/v3/contents/generations/tasks` 前缀检测），不影响其他渠道的查询路径。

## 8. 对非 Seedance relay 的影响

- `TaskSubmitReq` 新增 `Videos` 和 `Audios` 字段（`[]string` 类型，`json:"videos,omitempty"` 和 `json:"audios,omitempty"`），不影响其他请求的解析。
- `constant/channel.go` 中 `ChannelTypeDummy` 从 60 后移为 61，如果有硬编码 60 的地方需要检查。
- `constant/api_type.go` 中新增 `APITypeSeedance`，不影响已有 API 类型。

## 9. 风险与缓解

| 风险 | 影响 | 缓解 |
| --- | --- | --- |
| 前端 /2 自动除法导致 model_ratio 配错 | 计费只有应有一半 | 文档中明确警告，建议直接编辑 JSON |
| 上游不返回 usage | 无法 token 重算，保持预扣 | 预扣基于 model_ratio/2，通常大于实际；后续可加估算 fallback |
| doubao ParseTaskResult 行为变化 | token 选择逻辑微调 | 新行为更准确，且只在 `completion_tokens` 和 `total_tokens` 不同时有差异 |
| 任务轮询关闭 | 任务不推进、不补差、不退款 | 运维文档要求 master `UPDATE_TASK=true` |
| 视频下载 URL 24 小时过期 | 用户无法下载 | 文档提醒立即下载或转存 |
| Fast + 1080p 未被本地拒绝 | 上游 400 后走退款 | 可在后续版本添加本地校验 |
| 上游内容审核拒绝（如版权图片） | 任务失败，返回 `OutputVideoSensitiveContentDetected.PolicyViolation` | 非代码 bug，需用户更换输入素材；失败任务走现有退款逻辑 |

## 10. 验证建议

本地验证：

```powershell
go test ./relay/channel/task/seedance/... ./relay/channel/seedance/... -count=1
go vet ./relay/channel/task/seedance/... ./relay/channel/seedance/...
go build ./...
```

接口验证：

- `/v1/video/generations` 提交文生视频任务成功。
- `/v1/video/generations/{task_id}` 查询任务状态成功。
- `/api/v3/contents/generations/tasks` 提交火山兼容格式任务成功。
- `/api/v3/contents/generations/tasks/{task_id}` 查询返回原始上游格式。
- 输入视频场景应用折扣倍率。
- 1080p 场景应用加价倍率。
- Fast + 1080p 被拒绝或失败后退款。
- 完成响应中能解析 `usage.completion_tokens`。
- `completion_tokens` 缺失时 fallback 到 `total_tokens`。
- 任务成功后日志能看到 token 重算、补扣或退款。
- doubao 渠道现有功能未受影响。

## 11. 当前结论

Seedance 2.0 独立渠道的核心提交、查询、转换和计费链路已经具备上线条件。上线前需要确认：

- 渠道类型显示为 `Seedance` 或类型值 `60`。
- API 地址为第三方 Seedance 网关地址。
- Seedance 模型配置了 `model_ratio`（不是 `model_price`）。
- Seedance 模型没有加入 `TaskPricePatches`。
- 前端"模型计费"页面输入值正确（需要输入两倍值）。
- master 节点开启任务轮询。
- doubao 渠道经过回归测试确认无异常。
