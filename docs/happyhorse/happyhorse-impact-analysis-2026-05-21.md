# HappyHorse 影响分析报告

更新时间：2026-05-22

## 1. 范围

本报告分析 HappyHorse 视频兼容分支对 new-api 的影响。当前功能新增独立渠道类型 `59`，并支持：

```http
POST /happyhorse/api/generate
GET  /happyhorse/api/status/{task_id}
POST /v1/video/generations
GET  /v1/video/generations/{task_id}
```

## 2. 主要代码影响

| 区域 | 文件 | 影响 |
| --- | --- | --- |
| 渠道类型 | `constant/channel.go`、`constant/api_type.go`、`common/api_type.go` | 新增 HappyHorse 类型和默认地址 |
| 分发 | `middleware/distributor.go` | `/happyhorse/api/*` 进入任务 relay 分发 |
| 路由 | `router/video-router.go` | 新增原生提交和查询路由 |
| 适配器注册 | `relay/relay_adaptor.go` | 注册 HappyHorse 普通 adaptor 和 task adaptor |
| 任务链路 | `relay/channel/task/happyhorse/*` | 新增请求转换、校验、状态解析、计费补差和测试 |
| 计费入口 | `controller/relay.go`、`relay/channel/adapter.go`、`relay/channel/task/taskcommon/helpers.go`、`relay/relay_task.go` | 通过 `DisablePerCallBilling()` 声明完成后补差，不再硬编码模型名前缀 |
| 前端 | `web/src/constants/channel.constants.js`、`web/src/helpers/render.jsx` | 新增 HappyHorse 渠道名称和图标 |
| 文档 | `docs/happyhorse/*` | 新增接口、管理、计费、测试和影响说明 |

## 3. 功能影响

HappyHorse 模型：

```text
happyhorse-1.0-t2v
happyhorse-1.0-i2v
happyhorse-1.0-r2v
happyhorse-1.0-video-edit
```

已清除历史模型：

```text
happyhorse-1.0/video
```

当前请求契约：

- 默认分辨率为 `1080P`。
- T2V/R2V 支持比例 `16:9`、`9:16`、`1:1`、`4:3`、`3:4`。
- I2V 和 Video Edit 不支持 ratio。
- T2V/I2V/R2V 显式 duration 必须为 `3-15`。
- Video Edit 不支持 duration。
- I2V 正好 1 张首帧图。
- R2V 参考图数量为 1-9。
- Video Edit 正好 1 个输入视频，参考图数量为 0-5。
- media URL 必须非空且使用 `http/https`。
- `quality`、`sound` 不再透传。

## 4. 计费影响

HappyHorse 仍使用 new-api 统一任务计费链路：

```text
EstimateBilling -> PreConsumeBilling -> SettleBilling -> AdjustBillingOnComplete
```

预扣公式：

```text
model_price * QuotaPerUnit * group_ratio * estimated_seconds * resolution_ratio
```

完成结算公式：

```text
model_price * QuotaPerUnit * group_ratio * actual_seconds * resolution_ratio
```

秒数规则：

- T2V/I2V/R2V：优先 `usage.output_video_duration`，其次 `usage.duration`。
- Video Edit：使用 `usage.input_video_duration + usage.output_video_duration`；缺字段时回退 `usage.duration`。

分辨率规则：

- `720P` 倍率为 `1.0`。
- `1080P` 倍率为 `1.6 / 0.9`。
- `usage.SR` 兼容数字和字符串。
- `usage.SR` 缺失时回退提交时保存的分辨率倍率。

影响说明：

- 缺省请求从旧版本的 720P 预估变为 1080P 预估，默认预扣会上升。
- Video Edit 从只按输出秒数计费修正为输入秒数加输出秒数计费，实际扣费会上升并符合官方价格规则。
- HappyHorse 通过 adaptor 声明禁用 `PerCallBilling`，避免完成轮询时跳过补差。

## 5. 查询和轮询影响

查询接口读本地 DB，不实时拉上游。任务推进、成功补差、失败退款依赖后台轮询。

部署要求：

- 至少一个 master 节点运行。
- master 节点开启 `UPDATE_TASK=true`。
- 纯 slave 部署可能导致任务长期停留在 pending/queued，且无法完成补差或退款。

## 6. 兼容影响

### 对 HappyHorse 用户

- `/happyhorse/api/status?task_id=xxx` 不兼容，只支持 `/happyhorse/api/status/{task_id}`。
- `happyhorse-1.0/video` 不再支持。
- I2V 传 ratio 会返回 400。
- Video Edit 传 ratio 或 duration 会返回 400。
- `quality`、`sound` 不再作为 HappyHorse 参数透传。
- 空 URL、非 http/https URL 会在本地返回 400。

### 对其他任务渠道

- `TaskAdaptor` 接口新增 `DisablePerCallBilling()`，`BaseBilling` 默认返回 `false`，其他 task adaptor 通过嵌入 `BaseBilling` 保持原行为。
- `videoFetchByIDRespBodyBuilder` 移除了 query `task_id` fallback，当前已知视频查询路由均使用 path 参数或 context 参数。

### 对非 HappyHorse relay

分支中另有共享修复：

- Redis 删除操作增加 nil guard。
- Claude 文件 MIME 推断增强。
- StreamScanner 保留预初始化 `StreamStatus`。

这些改动不是 HappyHorse 核心功能，但已拆成独立提交，方便单独评审。

## 7. 风险与缓解

| 风险 | 影响 | 缓解 |
| --- | --- | --- |
| 默认 1080P 导致预扣升高 | 用户余额要求提高 | 文档和管理配置中明确默认值 |
| Video Edit 实际扣费升高 | 从旧测试结果看会出现更大补扣 | 按官方输入+输出计费，测试覆盖补扣方向 |
| 上游 `usage.SR` 返回字符串 | 旧实现解析失败 | 已增加 `SRValue` 兼容数字/字符串 |
| 本地校验更严格 | 旧的非官方参数请求会 400 | 文档列出新规则 |
| 任务轮询关闭 | 任务不推进、不补差、不退款 | 运维文档要求 master `UPDATE_TASK=true` |

## 8. 验证建议

本地验证：

```powershell
go test ./relay/channel/task/happyhorse ./relay/channel/happyhorse -count=1
go vet ./relay/channel/task/happyhorse/... ./relay/channel/happyhorse/...
go build ./...
```

接口验证：

- `/happyhorse/api/generate` 4 个模型提交成功。
- `/happyhorse/api/status/{task_id}` 查询完成结果。
- `/v1/video/generations` 4 个模型提交成功。
- `/v1/video/generations/{task_id}` 查询完成结果。
- 负向测试覆盖 duration、ratio、media 数量、空 URL、非 http/https URL。
- 计费日志覆盖普通模型输出秒数结算和 Video Edit 输入+输出秒数结算。

## 9. 当前结论

HappyHorse 核心提交、查询、转换和计费链路已经具备上线条件。上线前需要确认：

- 渠道模型列表不包含 `happyhorse-1.0/video`。
- 用户余额能覆盖默认 1080P 预扣。
- master 节点开启任务轮询。
- 文档和测试报告使用当前契约，不再引用旧的 `quality/sound`、Video Edit duration 或“只按输出秒数计费”口径。
