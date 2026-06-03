# Seedance 2.0 独立渠道与计费设计文档

更新时间：2026-06-03
适用角色：管理员、运维、计费配置人员、后端开发人员

## 1. 设计目标

本文用于确定 Seedance 2.0 系列模型在 new-api 中的独立渠道设计和计费方案。最新约束是：

- 不允许在现有渠道中修改 Seedance 2.0 逻辑。
- 不把 Seedance 2.0 逻辑塞进 `OpenAI`、`Sora`、`DoubaoVideo`、`VolcEngine` 等已有渠道。
- 需要同时兼容 OpenAI Video 风格入口和火山官方兼容入口。
- 计费以 Seedance 2.0 官方 token usage 为准。

设计原则：

- KISS：新增一个聚焦 Seedance 2.0 的任务渠道，避免改动多个既有渠道。
- YAGNI：只覆盖当前明确需要的 Seedance 2.0 主版本和 fast 版本。
- DRY：复用 new-api 现有任务 relay、预扣、轮询、补差、失败退款和 token 重算能力。
- SOLID：独立 channel adaptor 负责渠道注册，独立 task adaptor 负责协议转换和 usage 提取，service 层继续负责结算。

## 2. 最终结论

新增独立渠道：

```text
渠道类型：seedance
渠道名称：seedance
建议类型值：60
默认上游地址：留空，由管理员填写第三方 Seedance 网关地址
```

说明：`seedance` 是新增渠道的显示名和配置名；Seedance 2.0 是该渠道第一阶段支持的模型系列。

现有 `ChannelTypeDummy = 60` 是计数占位，新增渠道时应插入到 Dummy 之前，并将 Dummy 后移为 `61`。不要把新渠道加在 Dummy 后面。

推荐模型列表：

```text
dreamina-seedance-2-0-260128,dreamina-seedance-2-0-fast-260128
```

可选兼容火山官方模型名：

```text
doubao-seedance-2-0-260128,doubao-seedance-2-0-fast-260128
```

对外入口同时支持：

```text
POST /v1/video/generations
GET  /v1/video/generations/{task_id}

POST /api/v3/contents/generations/tasks
GET  /api/v3/contents/generations/tasks/{task_id}
```

两个入口都进入 `seedance` 新 task adaptor，使用同一套计费逻辑。

## 3. 为什么必须新增渠道

| 方案 | 是否满足“不改现有渠道” | 协议适配 | 修改量 | 计费准确性 | 结论 |
| --- | --- | --- | --- | --- | --- |
| 修改 `OpenAI` / `Sora` | 否 | 中 | 中 | 需补齐 | 不采用 |
| 修改 `DoubaoVideo` / `VolcEngine` | 否 | 高 | 小 | 高 | 不采用 |
| 新增 `seedance` 独立渠道 | 是 | 高 | 中 | 高 | 采用 |

虽然 `DoubaoVideo` / `VolcEngine` 的协议更接近火山官方入口，但在最新约束下不能修改现有渠道，因此它们只能作为参考实现，不能作为落地点。

新增独立渠道的收益：

- 不影响现有 `OpenAI`、`Sora`、`DoubaoVideo`、`VolcEngine` 用户。
- Seedance 2.0 的双入口、模型别名、usage 解析和计费倍率都收敛在独立包内。
- 后续回归范围清晰，只需要验证 seedance 渠道和通用任务链路。

## 4. 渠道注册设计

### 4.1 后端常量

在 `constant/channel.go` 中新增：

```go
ChannelTypeSeedance = 60
ChannelTypeDummy     = 61
```

同步更新：

- `ChannelBaseURLs`：新增 seedance 默认地址，建议为空字符串，让管理员填写第三方网关地址。
- `ChannelTypeNames`：新增 `ChannelTypeSeedance: "seedance"`。

在 `constant/api_type.go` 中新增：

```go
APITypeSeedance // 60
APITypeDummy     // 61
```

在 `common/api_type.go` 中新增映射：

```go
case constant.ChannelTypeSeedance:
    apiType = constant.APITypeSeedance
```

### 4.2 普通 channel adaptor

新增目录：

```text
relay/channel/seedance/
```

职责：

- 类似 HappyHorse 的普通 adaptor，仅用于渠道类型注册、模型列表展示和非任务请求拒绝。
- 不承载聊天、图片、embedding 等普通 relay 能力。
- 返回错误说明：`seedance supports video task relay only`。

在 `relay/relay_adaptor.go` 中注册：

```go
case constant.APITypeSeedance:
    return &seedance.Adaptor{}
```

### 4.3 任务 task adaptor

新增目录：

```text
relay/channel/task/seedance/
```

职责：

- 验证两类 Seedance 请求。
- 将 OpenAI Video 风格请求转换为火山 `content[]` 请求。
- 火山官方兼容入口尽量原样透传。
- 提交任务、轮询任务、解析状态、解析视频 URL。
- 解析 `usage.completion_tokens` 和 `usage.total_tokens`。
- 计算提交时的条件倍率，完成后交给统一 token 重算补差。

在 `relay/relay_adaptor.go` 中注册：

```go
case constant.ChannelTypeSeedance:
    return &taskseedance.TaskAdaptor{}
```

## 5. 双入口设计

### 5.1 OpenAI Video 风格入口

入口：

```text
POST /v1/video/generations
GET  /v1/video/generations/{task_id}
```

请求示例：

```json
{
  "model": "dreamina-seedance-2-0-fast-260128",
  "prompt": "A small kitten chasing a butterfly in a sunny garden",
  "duration": 5,
  "size": "720p",
  "images": ["https://example.com/first.png"],
  "videos": ["https://example.com/source.mp4"],
  "audios": ["https://example.com/voice.mp3"]
}
```

转换规则：

| OpenAI 风格字段 | 火山 content[] 请求字段 |
| --- | --- |
| `model` | `model` |
| `prompt` | `content[]` 中 `type=text` |
| `images[]` | `content[]` 中 `type=image_url` |
| `videos[]` | `content[]` 中 `type=video` |
| `audios[]` | `content[]` 中 `type=audio` |
| `duration` / `seconds` | `duration` |
| `size` | `resolution` |
| `seed` | `seed` |
| `metadata` | 仅读取当前需要字段，不整体扩展协议 |

### 5.2 火山官方兼容入口

入口：

```text
POST /api/v3/contents/generations/tasks
GET  /api/v3/contents/generations/tasks/{task_id}
```

请求示例：

```json
{
  "model": "dreamina-seedance-2-0-fast-260128",
  "content": [
    {
      "type": "text",
      "text": "cinematic pan --rt 16:9 --rs 720p --dur 5"
    },
    {
      "type": "image_url",
      "image_url": {
        "url": "https://example.com/first.png"
      },
      "role": "first_frame"
    }
  ]
}
```

处理规则：

- 校验 `model` 和 `content[]`。
- 保留 `content[]` 原结构。
- 只补必要的模型映射、默认值和计费字段提取。
- 不先转成 OpenAI 风格请求，避免无意义的中间结构。

### 5.3 路由与分发

需要新增或调整：

| 文件 | 设计职责 |
| --- | --- |
| `router/video-router.go` | 增加 `/api/v3/contents/generations/tasks` 提交和查询路由 |
| `middleware/distributor.go` | 识别 `/api/v3/contents/generations/tasks`，提交时读取 `model`，查询时不重新选渠道 |
| `relay/relay_task.go` | 查询时按路径返回对应响应形态 |

`/v1/video/generations` 已有路由和分发能力，只要 seedance 渠道被选中，就会进入 seedance task adaptor。

## 6. 上游请求设计

seedance task adaptor 对上游统一使用火山官方兼容接口：

```text
POST {base_url}/api/v3/contents/generations/tasks
GET  {base_url}/api/v3/contents/generations/tasks/{upstream_task_id}
```

选择原因：

- 第三方文档明确支持该入口。
- 响应中包含 `duration`、`resolution`、`framespersecond`、`usage`、`content.video_url` 等结算和展示需要字段。
- 比 OpenAI `/v1/videos` 路径更接近 Seedance 2.0 官方形态。

## 7. 模型价格配置

Seedance 2.0 推荐使用 `model_ratio`，不使用 `model_price`。

原因：

- `model_price` 更适合固定按次或按规格价格，不适合 Seedance 2.0 的 token usage 精确结算。
- 固定价格容易触发 `PerCallBilling`，跳过完成后补差。
- `model_ratio` 能复用现有 token 重算链路。

当前换算基准：

```text
modelRatio = 官方人民币元/百万 token / 14.6
```

推荐基础配置：

| 模型 | 基准条件 | 元/百万 token | 推荐 model_ratio |
| --- | --- | ---: | ---: |
| `dreamina-seedance-2-0-260128` | 480P/720P，输入不含视频 | 46 | 3.1507 |
| `dreamina-seedance-2-0-fast-260128` | 输入不含视频 | 37 | 2.5342 |
| `doubao-seedance-2-0-260128` | 480P/720P，输入不含视频 | 46 | 3.1507 |
| `doubao-seedance-2-0-fast-260128` | 输入不含视频 | 37 | 2.5342 |

推荐配置：

```json
{
  "dreamina-seedance-2-0-260128": 3.1507,
  "dreamina-seedance-2-0-fast-260128": 2.5342
}
```

如果部署方修改过 `USD2RMB`、`QuotaPerUnit` 或余额单位，上表需要按实际基准重新计算。

## 8. 条件倍率

Seedance 2.0 token 单价受模型、输出分辨率、是否含输入视频影响。设计上将“不含输入视频”的较高价配置为基础 `model_ratio`，其他条件通过 `OtherRatios` 修正。

### 8.1 主版本

基准：`dreamina-seedance-2-0-260128` / `doubao-seedance-2-0-260128`，480P/720P，输入不含视频，46 元/百万 token。

| 条件 | 官方单价 | OtherRatio |
| --- | ---: | ---: |
| 480P/720P，输入不含视频 | 46 | `1.0` |
| 480P/720P，输入含视频 | 28 | `28 / 46 = 0.6087` |
| 1080P，输入不含视频 | 51 | `51 / 46 = 1.1087` |
| 1080P，输入含视频 | 31 | `31 / 46 = 0.6739` |

### 8.2 Fast 版本

基准：`dreamina-seedance-2-0-fast-260128` / `doubao-seedance-2-0-fast-260128`，输入不含视频，37 元/百万 token。

| 条件 | 官方单价 | OtherRatio |
| --- | ---: | ---: |
| 输入不含视频 | 37 | `1.0` |
| 输入含视频 | 22 | `22 / 37 = 0.5946` |

Fast 版本不支持 1080P。第一阶段建议请求校验阶段拒绝 fast + 1080P；如果由上游拒绝，失败任务继续走现有退款逻辑。

## 9. 扣费计算

预扣阶段：

```text
预扣 quota = model_ratio / 2 * QuotaPerUnit * group_ratio * seedance_condition_ratio
```

完成后实际扣费：

```text
实际 quota = token_count * model_ratio * group_ratio * seedance_condition_ratio
```

Token 选择：

1. 优先使用 `usage.completion_tokens`。
2. 缺失或为 0 时使用 `usage.total_tokens`。
3. 两者都缺失时保持预扣额度，不做 token 补差。

示例：主版本 720P 文生视频，`completion_tokens=108900`。

```text
model_ratio = 46 / 14.6 = 3.1507
group_ratio = 1
condition_ratio = 1

actual_quota = 108900 * 3.1507 * 1 * 1 = 343111
```

示例：主版本 720P 输入含视频，`completion_tokens=108900`。

```text
model_ratio = 3.1507
condition_ratio = 28 / 46 = 0.6087

actual_quota = 108900 * 3.1507 * 0.6087 = 208850
```

## 10. 结算行为

seedance 渠道复用 new-api 统一任务计费链路：

1. 提交任务时根据模型、分辨率、输入视频条件预扣。
2. 任务完成轮询时解析上游 usage。
3. 将 `completion_tokens` 优先写入 `TaskInfo.TotalTokens`。
4. 统一任务结算调用 `RecalculateTaskQuotaByTokens()`。
5. 实际额度大于预扣时补扣差额。
6. 实际额度小于预扣时退回差额。
7. 任务失败时走现有失败退款逻辑。

关键约束：

- seedance 不应加入 `TaskPricePatches`。
- seedance 不应使用 `model_price` 作为推荐计费配置。
- seedance task adaptor 不需要 `DisablePerCallBilling()` 返回 true；只要使用 `model_ratio`，任务完成后可进入 token 重算。
- 不需要改通用任务结算接口。

## 11. 最小实现范围

第一阶段建议改动范围：

| 文件区域 | 设计职责 |
| --- | --- |
| `constant/channel.go` | 新增 `ChannelTypeSeedance`，Dummy 后移 |
| `constant/api_type.go` | 新增 `APITypeSeedance` |
| `common/api_type.go` | 新增 channel type 到 api type 映射 |
| `relay/channel/seedance` | 新增普通 channel adaptor，仅支持任务渠道注册 |
| `relay/channel/task/seedance` | 新增 task adaptor，处理双入口、上游请求、状态解析、usage 提取和计费倍率 |
| `relay/relay_adaptor.go` | 注册普通 adaptor 和 task adaptor |
| `router/video-router.go` | 增加 `/api/v3/contents/generations/tasks` 和 `/:task_id` 路由 |
| `middleware/distributor.go` | 识别火山官方兼容入口，提交时读取 `model`，查询时不重新选渠道 |
| `relay/relay_task.go` | 查询时支持火山官方兼容响应形态 |
| `relay/common/TaskSubmitReq` | 如需要支持 `videos[]` / `audios[]`，只补必要数组字段 |
| `setting/ratio_setting/model_ratio.go` | 可选增加默认 `model_ratio`；也可先由管理员手动配置 |
| `web/default/src/features/channels` | 新增 seedance 渠道类型显示和图标映射 |
| `web/default/src/i18n/locales` | 如新增显示文案，补齐所有支持语言 |

不进入第一阶段：

- 不修改 `OpenAI`、`Sora`、`DoubaoVideo`、`VolcEngine` 现有渠道行为。
- 不重构所有视频任务 adaptor。
- 不接入 billing expression 作为任务计费入口。
- 不维护按每个视频规格展开的静态“元/个”价格表。
- 不实现 Seedance 1.0 / 1.5 / draft / 有声视频等额外模型。
- 不改数据库结构。

## 12. 上线检查清单

上线前检查：

- 渠道类型显示为 `seedance`。
- API 地址为第三方 Seedance 网关地址。
- 密钥可用。
- 渠道模型列表包含实际请求模型名。
- `/v1/video/generations` 可以提交任务。
- `/v1/video/generations/{task_id}` 可以查询任务。
- `/api/v3/contents/generations/tasks` 可以提交任务。
- `/api/v3/contents/generations/tasks/{task_id}` 可以查询任务。
- Seedance 2.0 模型配置了 `model_ratio`。
- Seedance 2.0 模型没有配置为固定 `model_price`。
- Seedance 2.0 模型没有加入 `TaskPricePatches`。
- 文生视频 720P 可以提交并轮询成功。
- 输入视频场景可以提交并应用输入视频折扣。
- 主版本 1080P 可以应用 1080P 单价倍率。
- fast 版本 1080P 被拒绝或失败后退款。
- 完成响应中能解析 `usage.completion_tokens`。
- `completion_tokens` 缺失时能 fallback 到 `usage.total_tokens`。
- 任务成功后日志能看到 token 重算、补扣或退款。
- master 节点运行，且 `UPDATE_TASK=true`。
- 现有 `OpenAI`、`Sora`、`DoubaoVideo`、`VolcEngine` 任务行为未变化。

## 13. 文档自审结论

### 是否符合 KISS

符合。新增一个独立 seedance 渠道，比在多个既有渠道中增加分支更直观。双入口和计费都收敛在一个 task adaptor 内。

### 是否符合 YAGNI

符合。第一阶段只覆盖 Seedance 2.0 主版本和 fast 版本，不扩展历史模型、draft、有声视频、完整视频规格价格表或 billing expression。

### 是否符合 DRY

基本符合。设计复用通用任务 relay、轮询、补差和 token 重算链路。由于最新约束不允许修改现有渠道，seedance adaptor 会参考 DoubaoVideo 的协议转换形态，但不抽取公共包，以免为了复用而触碰既有渠道行为。

### 是否符合 SOLID

符合。普通 channel adaptor 只负责渠道注册和非任务请求拒绝；task adaptor 只负责 seedance 协议和 usage 提取；service 层继续负责统一结算。

### 已修正的问题

- 不再推荐修改 `DoubaoVideo` / `VolcEngine`。
- 不再把 OpenAI Video 风格入口等同于 `OpenAI` 渠道类型。
- 明确双入口都进入 seedance 独立 task adaptor。
- 明确“不维护完整视频规格价格表”不等于不支持条件倍率。

## 14. 常见问题

### 为什么不继续用 DoubaoVideo？

协议上 DoubaoVideo 很接近 Seedance 2.0，但最新约束是不允许在现有渠道中修改。新增 seedance 独立渠道可以保留 DoubaoVideo 现有行为不变，同时得到更清晰的 Seedance 专属边界。

### 为什么不继续用 OpenAI 类型？

OpenAI 类型当前可能能通，但它走 Sora task adaptor，路径、轮询、视频 URL 和 usage 解析都不是 Seedance 2.0 的最佳形态。独立 seedance 渠道可以兼容 `/v1/video/generations` 入口，但不依赖 OpenAI/Sora 渠道实现。

### 会不会要求客户端改成 content[]？

不会强制。new-api 对外同时支持 `/v1/video/generations` 和 `/api/v3/contents/generations/tasks`。OpenAI Video 风格请求由 seedance task adaptor 转成火山 `content[]` 上游请求；火山官方兼容请求则尽量原样进入 seedance task adaptor。

### 为什么不使用 HappyHorse 的元/秒计费？

HappyHorse 官方原生是分辨率乘秒数计费。Seedance 2.0 官方准确口径是 token usage。直接使用元/秒会在输入视频、最低 token、1080P 等场景偏离官方账单。

### 不维护完整视频规格价格表是什么意思？

不是不支持分辨率、输入视频等价格差异，而是不维护类似“480P/720P/1080P * 5s/10s * 16:9/1:1 * 是否输入视频”的静态元/个价格表。Seedance 2.0 第一阶段只维护官方 token 单价和少量条件倍率，最终费用以 `usage.completion_tokens` 为准。

### 如果上游不返回 usage 怎么办？

第一阶段保持预扣额度，不做 token 补差。视频规格估算 fallback 可以作为后续独立需求，不进入本阶段。

### `completion_tokens` 和 `total_tokens` 用哪个？

优先 `completion_tokens`，因为 Seedance 2.0 文档明确准确 token 用量以接口返回的 completion tokens 为准。`total_tokens` 只作为兼容 fallback。
