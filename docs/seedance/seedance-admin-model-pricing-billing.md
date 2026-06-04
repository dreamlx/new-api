# Seedance 管理、模型、价格与扣费文档

更新时间：2026-06-04
适用角色：管理员、运维、计费配置人员

## 1. 管理目标

Seedance 2.0 作为 new-api 独立渠道类型接入：

- 渠道类型：`60`
- 渠道名称：`Seedance`
- 默认上游地址：留空，由管理员填写第三方 Seedance 网关地址
- 认证方式：渠道密钥填写第三方网关 API Key
- 对外接口：
  - `POST /v1/video/generations`
  - `GET /v1/video/generations/{task_id}`
  - `POST /api/v3/contents/generations/tasks`
  - `GET /api/v3/contents/generations/tasks/{task_id}`

## 2. 添加 Seedance 渠道

后台配置建议：

1. 进入渠道管理。
2. 新增渠道。
3. 渠道类型选择 `Seedance`；如果前端未更新只显示数字，选择 `60`。
4. 名称建议填写 `seedance`。
5. API 地址填写第三方 Seedance 网关地址（如 `https://your-gateway.example.com`）。
6. 密钥填写网关 API Key。
7. 模型列表填写需要开放的模型。
8. 分组选择要开放给用户的分组，例如 `default`。
9. 保存并启用渠道。

推荐模型列表：

```text
dreamina-seedance-2-0-260128,dreamina-seedance-2-0-fast-260128
```

可选兼容火山官方模型名：

```text
doubao-seedance-2-0-260128,doubao-seedance-2-0-fast-260128
```

## 3. 模型能力与请求要求

| 模型 | 能力 | 支持分辨率 | 典型出片速度 |
| --- | --- | --- | --- |
| `dreamina-seedance-2-0-260128` | 文生/图生/视频续写/音频驱动 | 480p / 720p / 1080p | ≈ 4 分钟 |
| `dreamina-seedance-2-0-fast-260128` | 文生/图生/视频续写/音频驱动 | 480p / 720p | ≈ 2 分 40 秒 |
| `doubao-seedance-2-0-260128` | 文生/图生/视频续写/音频驱动 | 480p / 720p / 1080p | ≈ 4 分钟 |
| `doubao-seedance-2-0-fast-260128` | 文生/图生/视频续写/音频驱动 | 480p / 720p | ≈ 2 分 40 秒 |

支持参数：

| 参数 | 支持值 | 默认 | 说明 |
| --- | --- | --- | --- |
| `size` / `resolution` | `480p`、`720p`、`1080p` | `720p` | fast 版不支持 1080p |
| `duration` | `5`、`10` | `5` | 视频时长（秒） |
| `images` | URL 数组 | 空 | 图生视频 / 首尾帧 / 参考图 |
| `videos` | URL 数组 | 空 | 视频续写 |
| `audios` | URL 数组 | 空 | 音频驱动 |
| `seed` | 整数 | 空 | 随机种子 |
| `ratio` | `16:9`、`9:16`、`1:1` 等 | 空 | 画幅比，由上游决定默认值 |
| `return_last_frame` | bool | 空 | 返回最后一帧 |
| `generate_audio` | bool | 空 | 生成音频 |
| `draft` | bool | 空 | 草稿模式 |
| `camera_fixed` | bool | 空 | 固定摄像头 |
| `watermark` | bool | 空 | 水印 |
| `frames` | int | 空 | 帧数 |
| `callback_url` | string | 空 | 回调地址 |
| `service_tier` | string | 空 | 服务层级 |
| `execution_expires_after` | int | 空 | 执行超时时间 |
| `tools` | array | 空 | 工具调用 |

高级参数透传方式：OpenAI Video 格式通过 `metadata` 字段传递，Volcano content[] 格式直接在请求体顶层传入。详见接口文档。

## 4. 部署与任务轮询要求

Seedance 复用 new-api 统一任务框架。任务提交和任务查询可以由 master 或 slave 节点处理，但任务状态推进、完成结算、补差和退款依赖后台任务轮询。

部署要求：

- 至少保留一个 master 节点运行任务轮询。
- master 节点需要开启 `UPDATE_TASK=true`。
- 纯 slave 部署可以提交任务，但任务不会自动推进到最终状态，也无法完成按实际 token 补差或失败退款。

运维判断：

- 如果 Seedance 任务长期停留在 `pending` / `queued`，优先检查 master 节点是否运行，以及 `UPDATE_TASK` 是否开启。
- 如果任务完成后没有发生补差或退款，优先检查任务轮询日志和 master 节点状态。

## 5. 设置模型价格

Seedance 使用 `model_ratio` 按 token 计费，**不应**使用 `model_price` 或按次固定价格。

原因：

- `model_price` 适合固定按次或按规格价格，不适合 Seedance 2.0 的 token usage 精确结算。
- 固定价格容易触发 `PerCallBilling`，跳过完成后补差。
- `model_ratio` 能复用现有 token 重算链路。

推荐配置（直接编辑 ModelRatio JSON）：

```json
{
  "dreamina-seedance-2-0-260128": 3.1507,
  "dreamina-seedance-2-0-fast-260128": 2.5342,
  "doubao-seedance-2-0-260128": 3.1507,
  "doubao-seedance-2-0-fast-260128": 2.5342
}
```

### 通过前端"模型计费"可视化页面配置

前端输入框的单位是 `USD / 1M tokens`，页面保存时会自动 `/2` 转为 `ModelRatio`。因此应填写：

| 模型 | 官方单价 (元/百万token) | 前端输入价格 USD/1M tokens | 保存后 ModelRatio |
| --- | ---: | ---: | ---: |
| `dreamina-seedance-2-0-260128` | 46 | **6.3014** | 3.1507 |
| `dreamina-seedance-2-0-fast-260128` | 37 | **5.0685** | 2.5342 |

⚠️ **常见错误**：直接在输入框填写 `3.1507`，保存后实际 `ModelRatio = 1.5754`，导致计费只有应有一半。务必填写两倍值（如 `6.3014`）。

### model_ratio 推导公式

```text
model_ratio = 官方人民币单价 / (2 × USD2RMB) = 官方人民币单价 / 14.6
```

其中 `USD2RMB = 7.3` 是系统常量。如果部署方修改了 `USD2RMB`、`QuotaPerUnit` 或余额单位，需要按实际基准重新计算。

### 不应加入的配置

- **不应加入 `TaskPricePatches`** — 否则会跳过 token 重算。
- **不应配置 `model_price`** — 会导致按次计费，跳过完成后补差。
- Seedance task adaptor 不需要 `DisablePerCallBilling()` 返回 true — 只要使用 `model_ratio`，任务完成后可进入 token 重算。

## 6. 条件倍率

Seedance 2.0 token 单价受模型、输出分辨率、是否含输入视频影响。设计上将"不含输入视频"的较高价配置为基础 `model_ratio`，其他条件通过 `OtherRatios` 修正。

### 主版本

基准：`dreamina-seedance-2-0-260128`，480p/720p，输入不含视频，46 元/百万 token。

| 条件 | 官方单价 (元/百万token) | OtherRatio |
| --- | ---: | ---: |
| 480p/720p，输入不含视频 | 46 | 1.0 |
| 480p/720p，输入含视频 | 28 | 0.6087 |
| 1080p，输入不含视频 | 51 | 1.1087 |
| 1080p，输入含视频 | 31 | 0.6739 |

### Fast 版本

基准：`dreamina-seedance-2-0-fast-260128`，输入不含视频，37 元/百万 token。

| 条件 | 官方单价 (元/百万token) | OtherRatio |
| --- | ---: | ---: |
| 输入不含视频 | 37 | 1.0 |
| 输入含视频 | 22 | 0.5946 |

Fast 版本不支持 1080p。如需拒绝 fast + 1080p 的请求，应在请求校验阶段处理。

### 视频输入检测

系统在 `EstimateBilling` 阶段自动检测请求中是否包含视频输入：

- Volcano content[] 格式：检查 `content[]` 中是否包含 `type=video_url` 或 `type=video` 条目
- OpenAI Video 格式：检查顶层 `videos` 字段是否非空

检测到视频输入时，自动将 `OtherRatios["seedance_condition"]` 设为对应折扣值，预扣和结算都会使用该折扣。

### 图片输入不影响价格

官方定价中，**图片输入不影响 token 单价**。图生视频（只输入图片、不输入视频）与纯文生视频使用相同的基础价格。只有视频输入（`videos[]` 或 content[] 中 `type=video`）才会触发折扣。

| 输入类型 | 是否影响价格 | 说明 |
| --- | --- | --- |
| 文本（prompt） | 否 | 基础价格 |
| 图片（images[]） | 否 | 与文生视频同价 |
| 视频（videos[]） | **是** | 触发 0.6087/0.5946 折扣 |
| 音频（audios[]） | 否 | 不影响 token 单价 |

## 7. 扣费计算

全局常量：

```text
QuotaPerUnit = 500000
USD2RMB = 7.3
```

预扣公式：

```text
预扣 quota = model_ratio / 2 × QuotaPerUnit × group_ratio × seedance_condition_ratio
```

完成后实际扣费公式：

```text
实际 quota = completion_tokens × model_ratio × group_ratio × seedance_condition_ratio
```

Token 选择优先级：

1. 优先使用 `usage.completion_tokens`
2. 缺失或为 0 时使用 `usage.total_tokens`
3. 两者都缺失时保持预扣额度，不做 token 补差

示例：主版本 720p 文生视频，`completion_tokens=108900`

```text
model_ratio = 3.1507
group_ratio = 1
seedance_condition_ratio = 1

预扣 quota = 3.1507 / 2 × 500000 × 1 × 1 = 787675
实际 quota = 108900 × 3.1507 × 1 × 1 = 343111
退回 quota = 787675 - 343111 = 444564
```

实际费用：343111 quota ÷ 500000 × 7.3 = **¥5.01 CNY**（与官方 46 元/百万 token × 0.1089M = ¥5.009 一致）

示例：主版本 720p 输入含视频，`completion_tokens=108900`

```text
model_ratio = 3.1507
seedance_condition_ratio = 0.6087

实际 quota = 108900 × 3.1507 × 1 × 0.6087 = 208850
```

实际费用：208850 quota ÷ 500000 × 7.3 = **¥3.05 CNY**（与官方 28 元/百万 token × 0.1089M = ¥3.049 一致）

示例：主版本 1080p 文生视频（不含视频输入），`completion_tokens=108900`

```text
model_ratio = 3.1507
seedance_condition_ratio = 1.1087

预扣 quota = 3.1507 / 2 × 500000 × 1 × 1.1087 = 874528
实际 quota = 108900 × 3.1507 × 1 × 1.1087 = 380498
退回 quota = 874528 - 380498 = 494030
```

实际费用：380498 quota ÷ 500000 × 7.3 = **¥5.55 CNY**（与官方 51 元/百万 token × 0.1089M = ¥5.514 基本一致）

示例：主版本 1080p 输入含视频，`completion_tokens=108900`

```text
model_ratio = 3.1507
seedance_condition_ratio = 0.6739

预扣 quota = 3.1507 / 2 × 500000 × 1 × 0.6739 = 531345
实际 quota = 108900 × 3.1507 × 1 × 0.6739 = 231245
退回 quota = 531345 - 231245 = 300100
```

实际费用：231245 quota ÷ 500000 × 7.3 = **¥3.38 CNY**（与官方 31 元/百万 token × 0.1089M = ¥3.376 基本一致）

示例：Fast 版本 720p 输入含视频，`completion_tokens=108900`

```text
model_ratio = 2.5342
seedance_condition_ratio = 0.5946

预扣 quota = 2.5342 / 2 × 500000 × 1 × 0.5946 = 377419
实际 quota = 108900 × 2.5342 × 1 × 0.5946 = 164313
退回 quota = 377419 - 164313 = 213106
```

实际费用：164313 quota ÷ 500000 × 7.3 = **¥2.40 CNY**（与官方 22 元/百万 token × 0.1089M = ¥2.396 一致）

### 扣费场景汇总

| 场景 | model_ratio | condition_ratio | 预扣 quota | 实际 quota (108900 tokens) | 实际费用 |
| --- | ---: | ---: | ---: | ---: | --- |
| 主版本 720p 文生视频 | 3.1507 | 1.0 | 787675 | 343111 | ¥5.01 |
| 主版本 720p 输入含视频 | 3.1507 | 0.6087 | 479416 | 208850 | ¥3.05 |
| 主版本 1080p 文生视频 | 3.1507 | 1.1087 | 874528 | 380498 | ¥5.55 |
| 主版本 1080p 输入含视频 | 3.1507 | 0.6739 | 531345 | 231245 | ¥3.38 |
| Fast 720p 文生视频 | 2.5342 | 1.0 | 633550 | 275972 | ¥4.03 |
| Fast 720p 输入含视频 | 2.5342 | 0.5946 | 377419 | 164313 | ¥2.40 |

## 8. 结算行为

Seedance 复用 new-api 统一任务计费链路：

1. 提交任务时根据模型、分辨率、输入视频条件预扣。
2. 任务完成轮询时解析上游 `usage`。
3. 优先将 `completion_tokens` 写入 `TaskInfo.TotalTokens`。
4. 统一任务结算调用 `RecalculateTaskQuotaByTokens()`。
5. 实际额度大于预扣时补扣差额。
6. 实际额度小于预扣时退回差额。
7. 任务失败时走现有失败退款逻辑。

关键约束：

- Seedance 不应加入 `TaskPricePatches`。
- Seedance 不应使用 `model_price` 作为推荐计费配置。
- 不需要改通用任务结算接口。

## 9. 管理检查清单

上线前检查：

- 渠道类型显示为 `Seedance` 或类型值 `60`。
- API 地址为第三方 Seedance 网关地址。
- 密钥可用。
- 渠道模型列表包含实际请求模型名。
- Seedance 模型配置了 `model_ratio`（不是 `model_price`）。
- Seedance 模型没有加入 `TaskPricePatches`。
- `/v1/video/generations` 可以提交任务。
- `/v1/video/generations/{task_id}` 可以查询任务。
- `/api/v3/contents/generations/tasks` 可以提交任务。
- `/api/v3/contents/generations/tasks/{task_id}` 可以查询任务。
- 文生视频 720p 可以提交并轮询成功。
- 输入视频场景可以提交并应用输入视频折扣。
- 主版本 1080p 可以应用 1080p 单价倍率。
- 完成响应中能解析 `usage.completion_tokens`。
- 任务成功后日志能看到 token 重算、补扣或退款。
- master 节点运行，且 `UPDATE_TASK=true`。

## 10. 常见问题

### 前端"模型计费"页面配置价格不对

前端输入框的单位是 `USD/1M tokens`，保存时会自动 `/2`。如果目标是 `ModelRatio = 3.1507`，应输入 `$6.3014`，不是 `$3.1507`。直接编辑 `ModelRatio` JSON 则填写 `3.1507`。

### 日志显示"输出价格 $NaN"

这是经典前端 UI 的渲染问题，不影响实际计费。Seedance 是视频生成任务，没有传统的输入/输出 token 区分，`completionRatio` 未设置导致经典前端显示 NaN。新版前端无此问题。

### 任务失败后没有退款

检查 master 节点是否运行，以及 `UPDATE_TASK` 是否开启。任务状态推进和退款依赖后台轮询。

### 上游不返回 usage 怎么办

保持预扣额度，不做 token 补差。视频规格估算 fallback 可以作为后续独立需求。

### `completion_tokens` 和 `total_tokens` 用哪个

优先 `completion_tokens`。Seedance 2.0 官方文档明确准确 token 用量以接口返回的 completion tokens 为准。`total_tokens` 只作为兼容 fallback。

### 上游内容审核拒绝

上游可能因内容审核拒绝请求，返回错误码 `OutputVideoSensitiveContentDetected.PolicyViolation`。常见原因：

- 输入图片受版权保护（如知名卡通角色）
- 输入内容违反平台政策

这不是代码 bug，任务会走正常失败退款逻辑。遇到此类错误时需更换输入素材。
