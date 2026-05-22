# HappyHorse 管理、模型、价格与扣费文档

更新时间：2026-05-21  
适用角色：管理员、运维、计费配置人员

## 1. 管理目标

HappyHorse 作为 new-api 独立渠道类型接入：

- 渠道类型：`59`
- 渠道名称：`HappyHorse`
- 默认上游地址：`https://dashscope.aliyuncs.com`
- 认证方式：渠道密钥填 DashScope/HappyHorse 上游 API Key
- 对外接口：
  - `POST /happyhorse/api/generate`
  - `GET /happyhorse/api/status/{task_id}`
  - `POST /v1/video/generations`
  - `GET /v1/video/generations/{task_id}`

## 2. 添加 HappyHorse 渠道

管理后台操作：

1. 进入渠道管理。
2. 新增渠道。
3. 渠道类型选择 `HappyHorse`。如果前端下拉只显示数字，选择 `59`。
4. 名称建议填写 `happyhorse`。
5. API 地址填写：
   ```text
   https://dashscope.aliyuncs.com
   ```
6. 密钥填写上游 API Key。
7. 模型列表填写需要开放的模型。
8. 分组选择要开放给用户的分组，例如 `default`。
9. 保存并启用渠道。

推荐模型列表：

```text
happyhorse-1.0-t2v,happyhorse-1.0-i2v,happyhorse-1.0-r2v,happyhorse-1.0-video-edit
```

说明：

- 当前 `/happyhorse/api/generate` 和 `/v1/video/generations` 均使用 4 个具体内部模型提交。
- `happyhorse-1.0/video` 已从当前 Go 源码的模型列表、模型识别、默认价格和查询响应中清除。
- 渠道模型列表应只配置 4 个内部模型。

## 3. 模型能力与请求要求

| 模型 | 能力 | 请求要求 |
| --- | --- | --- |
| `happyhorse-1.0-t2v` | 文生视频 | `input.prompt` |
| `happyhorse-1.0-i2v` | 图生视频 | `input.prompt` + `media[type=first_frame]` |
| `happyhorse-1.0-r2v` | 参考生视频 | `input.prompt` + 至少一个 `media[type=reference_image]` |
| `happyhorse-1.0-video-edit` | 视频编辑 | `input.prompt` + `media[type=video]` |

支持参数：

| 参数 | 支持值 | 默认 |
| --- | --- | --- |
| `parameters.resolution` | `720P`、`1080P` | `720P` |
| `parameters.ratio` | `16:9`、`9:16`、`1:1` | 空，由上游处理 |
| `parameters.duration` | 建议 5 或 10 | `5` |
| `parameters.quality` | 透传上游，如 `std`、`pro` | 空 |
| `parameters.sound` | `true`、`false` | 空 |
| `parameters.watermark` | `true`、`false` | 空 |
| `parameters.seed` | 整数 | 空 |

## 4. 部署与任务轮询要求

HappyHorse 复用 new-api 统一任务框架。任务提交和任务查询可以由 master 或 slave 节点处理，但任务状态推进、完成结算、补差和退费依赖后台任务轮询。

部署要求：

- 至少保留一个 master 节点运行任务轮询。
- master 节点需要开启 `UPDATE_TASK`。
- 纯 slave 部署可以提交任务，但任务不会自动推进到最终状态，也无法完成按输出秒数补差或退费。

运维判断：

- 如果 HappyHorse 任务长期停留在 `pending` / `queued`，优先检查 master 节点是否运行，以及 `UPDATE_TASK` 是否开启。
- 如果任务完成后没有发生补差或退费，优先检查任务轮询日志和 master 节点状态。
- 该行为与 Suno、Kling、Ali 等任务类渠道一致，不是 HappyHorse 专属限制。

## 5. 设置模型价格

HappyHorse 使用 new-api 的 `model_price` 按量计费，不建议设置为按次固定价格。

默认价格：

| 模型 | model_price |
| --- | ---: |
| `happyhorse-1.0-t2v` | 0.9 |
| `happyhorse-1.0-i2v` | 0.9 |
| `happyhorse-1.0-r2v` | 0.9 |
| `happyhorse-1.0-video-edit` | 0.9 |

后台设置建议：

1. 进入倍率或价格配置页面。
2. 找到 `model_price`。
3. 为 4 个内部模型配置价格。
4. 不要配置 `happyhorse-1.0/video`，当前代码不再识别该模型。
5. 不要把 HappyHorse 模型放入“按次计费价格补丁”列表。

推荐配置：

```json
{
  "happyhorse-1.0-t2v": 0.9,
  "happyhorse-1.0-i2v": 0.9,
  "happyhorse-1.0-r2v": 0.9,
  "happyhorse-1.0-video-edit": 0.9
}
```

## 6. 分辨率价格倍率

HappyHorse 分辨率倍率在代码中固定：

| 分辨率 | 倍率 | 来源 |
| --- | ---: | --- |
| `720P` | `1.0` | 基准价 |
| `1080P` | `1.6 / 0.9 = 1.777777...` | 上游 1080P 单价相对 720P 单价 |

含义：

- `model_price=0.9` 代表 720P 每秒价格。
- 1080P 按 `model_price * 1.777777...` 计费。
- 如果未来上游价格变化，需要同步修改分辨率倍率或模型价格策略。

## 7. 扣费计算

全局常量：

```text
QuotaPerUnit = 500000
```

预扣公式：

```text
预扣 quota = model_price * QuotaPerUnit * group_ratio * duration * resolution_ratio
```

完成后实际扣费公式：

```text
实际 quota = model_price * QuotaPerUnit * group_ratio * output_duration * resolution_ratio
```

字段来源：

| 字段 | 预扣来源 | 完成结算来源 |
| --- | --- | --- |
| `model_price` | 模型价格配置 | 任务 BillingContext |
| `group_ratio` | 用户分组倍率 | 任务 BillingContext |
| `duration` | 提交参数，默认 5 | 上游 `usage.output_video_duration` 优先，其次 `usage.duration` |
| `resolution_ratio` | 提交或转换出的分辨率 | 上游 `usage.SR`，1080 使用 1080P 倍率，否则 720P |

示例 1：720P 5 秒

```text
model_price = 0.9
QuotaPerUnit = 500000
group_ratio = 1
duration = 5
resolution_ratio = 1

quota = 0.9 * 500000 * 1 * 5 * 1 = 2250000
```

示例 2：1080P 5 秒

```text
model_price = 0.9
QuotaPerUnit = 500000
group_ratio = 1
duration = 5
resolution_ratio = 1.6 / 0.9

quota = 0.9 * 500000 * 1 * 5 * 1.777777... = 4000000
```

示例 3：Video Edit 输出 13.9 秒

```text
预扣 quota = 0.9 * 500000 * 1 * 5 * 1 = 2250000
实际 quota = 0.9 * 500000 * 1 * 13.9 * 1 = 6255000
补扣 quota = 6255000 - 2250000 = 4005000
```

## 8. 结算行为

HappyHorse 借用 new-api 任务计费链路：

1. 提交任务时按参数预扣。
2. 任务完成轮询时读取上游 `usage`。
3. 如果实际额度大于预扣，补扣差额。
4. 如果实际额度小于预扣，退回差额。
5. 如果任务失败，走现有失败退款逻辑。

特殊设计：

- HappyHorse 不走 `PerCallBilling` 固定按次计费例外。
- `controller/relay.go` 中对 `happyhorse-` 模型保留完成后补差路径。
- Video Edit 只按输出视频秒数计费，不按输入视频秒数计费。

## 9. 管理检查清单

上线前检查：

- HappyHorse 渠道类型显示为 `HappyHorse` 或类型值 `59`。
- API 地址为 `https://dashscope.aliyuncs.com`。
- 密钥可用。
- 渠道模型包含 4 个内部模型。
- 分组已授权给目标用户。
- 4 个内部模型均配置 `model_price`。
- 用户所在分组倍率符合预期。
- 余额足够覆盖预扣。
- `/happyhorse/api/generate` 可以提交任务。
- `/happyhorse/api/status/{task_id}` 可以查询任务。
- 日志中能看到预扣、完成补差或失败退款。

## 10. `happyhorse-1.0/video` 清除检查

当前 Go 源码已清除 `happyhorse-1.0/video`，它不再是 `/happyhorse/api/generate` 和 `/v1/video/generations` 的提交模型名，也不再作为查询响应中的展示模型名。

| 位置 | 当前状态 | 影响 |
| --- | --- | --- |
| `relay/channel/task/happyhorse/constants.go` | 已删除 `NativeModel`，`ModelList` 只保留 4 个内部模型 | 渠道模型展示不再包含官网聚合模型名 |
| `IsHappyHorseModel` | 只识别 4 个内部模型 | 请求 `happyhorse-1.0/video` 会进入 `unsupported model` |
| `relay/channel/task/happyhorse/validate.go` | 不再保留 NativeModel 专门拒绝分支 | 使用统一模型校验错误 |
| `relay/channel/task/happyhorse/billing.go` | 查询响应 `data.model` 返回 `task.Properties.UpstreamModelName` | 原生查询响应展示实际内部模型名 |
| `setting/ratio_setting/model_ratio.go` | `defaultModelPrice` 只保留 4 个内部模型价格 | 不再为 `happyhorse-1.0/video` 配置价格 |

结论：

- 当前功能提交模型应使用 `happyhorse-1.0-t2v`、`happyhorse-1.0-i2v`、`happyhorse-1.0-r2v`、`happyhorse-1.0-video-edit`。
- `happyhorse-1.0/video` 不应出现在渠道模型列表、模型价格配置或新测试用例中。
- 如果历史文档或历史测试产物中仍出现该字符串，只代表旧镜像或旧接口阶段的记录，不代表当前代码行为。

## 11. 常见问题

### 渠道类型只显示数字 59

说明前端构建或前端常量未更新。后端已定义 `ChannelTypeHappyHorse=59` 和名称 `HappyHorse`；需要重新构建前端并部署包含最新前端产物的镜像。

### `model_not_found`

检查渠道模型列表是否包含请求中的模型名。结构化接口请求的是 4 个内部模型，不是 `happyhorse-1.0/video`。

### 任务失败，提示素材下载失败

图片或视频 URL 不能被上游直接下载。换用公网可访问 URL 后重试。

### `aspect_ratio` 未返回

查询响应中的 `aspect_ratio` 当前来自上游 `usage.ratio`。如果上游未返回，该字段可能为空。

### 价格不符合预期

按以下顺序检查：

1. 模型是否配置了 `model_price`。
2. 用户分组倍率是否为预期值。
3. 分辨率是否为 `720P` 或 `1080P`。
4. 完成后上游 `usage.output_video_duration` 是否与预期一致。
5. 是否发生了补扣或退款。
