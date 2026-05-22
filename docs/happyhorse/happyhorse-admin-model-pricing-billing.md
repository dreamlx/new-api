# HappyHorse 管理、模型、价格与扣费文档

更新时间：2026-05-22
适用角色：管理员、运维、计费配置人员

## 1. 管理目标

HappyHorse 作为 new-api 独立渠道类型接入：

- 渠道类型：`59`
- 渠道名称：`HappyHorse`
- 默认上游地址：`https://dashscope.aliyuncs.com`
- 认证方式：渠道密钥填写 DashScope/HappyHorse 上游 API Key
- 对外接口：
  - `POST /happyhorse/api/generate`
  - `GET /happyhorse/api/status/{task_id}`
  - `POST /v1/video/generations`
  - `GET /v1/video/generations/{task_id}`

## 2. 添加 HappyHorse 渠道

后台配置建议：

1. 进入渠道管理。
2. 新增渠道。
3. 渠道类型选择 `HappyHorse`；如果前端未更新只显示数字，选择 `59`。
4. 名称建议填写 `happyhorse`。
5. API 地址填写 `https://dashscope.aliyuncs.com`，也可以按实际地域替换为对应 DashScope endpoint。
6. 密钥填写上游 API Key。
7. 模型列表填写需要开放的模型。
8. 分组选择要开放给用户的分组，例如 `default`。
9. 保存并启用渠道。

推荐模型列表：

```text
happyhorse-1.0-t2v,happyhorse-1.0-i2v,happyhorse-1.0-r2v,happyhorse-1.0-video-edit
```

不要配置 `happyhorse-1.0/video`。当前代码不再识别该模型。

## 3. 模型能力与请求要求

| 模型 | 能力 | 请求要求 |
| --- | --- | --- |
| `happyhorse-1.0-t2v` | 文生视频 | `prompt` |
| `happyhorse-1.0-i2v` | 图生视频 | `prompt` + 正好 1 张首帧图 |
| `happyhorse-1.0-r2v` | 参考生视频 | `prompt` + 1-9 张参考图 |
| `happyhorse-1.0-video-edit` | 视频编辑 | `prompt` + 正好 1 个输入视频，可选 0-5 张参考图 |

支持参数：

| 参数 | 支持值 | 默认 | 说明 |
| --- | --- | --- | --- |
| `resolution` | `720P`、`1080P` | `1080P` | 缺省按官方默认 `1080P` 处理 |
| `ratio` | `16:9`、`9:16`、`1:1`、`4:3`、`3:4` | 空 | 仅 T2V/R2V 支持；I2V 和 Video Edit 不支持 |
| `duration` | `3-15` | `5` | 仅 T2V/I2V/R2V 支持；Video Edit 不支持 |
| `watermark` | `true`、`false` | 空 | 透传上游 |
| `seed` | 整数 | 空 | 透传上游 |

`quality`、`sound` 当前不属于 HappyHorse 官方参数，代码不再透传到上游，文档和前端调用也不应继续使用。

## 4. 部署与任务轮询要求

HappyHorse 复用 new-api 统一任务框架。任务提交和任务查询可以由 master 或 slave 节点处理，但任务状态推进、完成结算、补差和退款依赖后台任务轮询。

部署要求：

- 至少保留一个 master 节点运行任务轮询。
- master 节点需要开启 `UPDATE_TASK=true`。
- 纯 slave 部署可以提交任务，但任务不会自动推进到最终状态，也无法完成按实际秒数补差或失败退款。

运维判断：

- 如果 HappyHorse 任务长期停留在 `pending` / `queued`，优先检查 master 节点是否运行，以及 `UPDATE_TASK` 是否开启。
- 如果任务完成后没有发生补差或退款，优先检查任务轮询日志和 master 节点状态。
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

含义：

- `model_price=0.9` 代表 720P 每秒价格。
- 1080P 按 `1.6 / 0.9 = 1.777777...` 分辨率倍率计费。
- 不要把 HappyHorse 模型放入按次计费价格补丁列表，否则会跳过完成后的实际秒数补差。

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

| 分辨率 | 倍率 | 来源 |
| --- | ---: | --- |
| `720P` | `1.0` | 基准价 |
| `1080P` | `1.6 / 0.9 = 1.777777...` | 上游 1080P 单价相对 720P 单价 |

代码中默认分辨率为 `1080P`。用户不传 resolution 时，预扣和上游请求都按 1080P 处理。

## 7. 扣费计算

全局常量：

```text
QuotaPerUnit = 500000
```

预扣公式：

```text
预扣 quota = model_price * QuotaPerUnit * group_ratio * estimated_seconds * resolution_ratio
```

完成后实际扣费公式：

```text
实际 quota = model_price * QuotaPerUnit * group_ratio * actual_seconds * resolution_ratio
```

字段来源：

| 字段 | 预扣来源 | 完成结算来源 |
| --- | --- | --- |
| `model_price` | 模型价格配置 | 任务 BillingContext |
| `group_ratio` | 用户分组倍率 | 任务 BillingContext |
| `estimated_seconds` | T2V/I2V/R2V 使用请求 duration 或默认 5；Video Edit 使用默认 5 作为预估 | 任务 BillingContext |
| `actual_seconds` | 不适用 | 普通模型优先 `usage.output_video_duration`，其次 `usage.duration`；Video Edit 使用 `usage.input_video_duration + usage.output_video_duration`，缺字段时回退 `usage.duration` |
| `resolution_ratio` | 提交或转换出的分辨率 | 优先上游 `usage.SR`；兼容 `720`、`"720"`、`1080`、`"1080"`；缺失时回退提交时的分辨率倍率 |

示例 1：1080P 5 秒默认预扣

```text
model_price = 0.9
QuotaPerUnit = 500000
group_ratio = 1
estimated_seconds = 5
resolution_ratio = 1.6 / 0.9

quota = 0.9 * 500000 * 1 * 5 * 1.777777... = 4000000
```

示例 2：720P 5 秒

```text
model_price = 0.9
QuotaPerUnit = 500000
group_ratio = 1
seconds = 5
resolution_ratio = 1

quota = 0.9 * 500000 * 1 * 5 * 1 = 2250000
```

示例 3：Video Edit 输入 13.9 秒，输出 13.9 秒，720P

```text
预扣 quota = 0.9 * 500000 * 1 * 5 * 1 = 2250000
实际 seconds = 13.9 + 13.9 = 27.8
实际 quota = 0.9 * 500000 * 1 * 27.8 * 1 = 12510000
补扣 quota = 12510000 - 2250000 = 10260000
```

## 8. 结算行为

HappyHorse 借用 new-api 任务计费链路：

1. 提交任务时按预估秒数和分辨率预扣。
2. 任务完成轮询时读取上游 `usage`。
3. 如果实际额度大于预扣，补扣差额。
4. 如果实际额度小于预扣，退回差额。
5. 如果任务失败，走现有失败退款逻辑。

特殊设计：

- HappyHorse 不走 `PerCallBilling` 固定按次计费。
- HappyHorse task adaptor 通过 `DisablePerCallBilling()` 显式声明保留完成后补差路径。
- `controller/relay.go` 不再硬编码 `happyhorse-` 模型名前缀。

## 9. 管理检查清单

上线前检查：

- HappyHorse 渠道类型显示为 `HappyHorse` 或类型值 `59`。
- API 地址为 `https://dashscope.aliyuncs.com` 或实际地域 endpoint。
- 密钥可用。
- 渠道模型只包含 4 个内部模型。
- 分组已授权给目标用户。
- 4 个内部模型均配置 `model_price`。
- 用户所在分组倍率符合预期。
- 用户余额足够覆盖默认 1080P 预扣。
- master 节点运行，且 `UPDATE_TASK=true`。
- `/happyhorse/api/generate` 可以提交任务。
- `/happyhorse/api/status/{task_id}` 可以查询任务。
- 日志中能看到预扣、完成补差或失败退款。

## 10. 常见问题

### 渠道类型只显示数字 59

说明前端构建或前端常量未更新。后端已定义 `ChannelTypeHappyHorse=59` 和名称 `HappyHorse`；需要重新构建并部署包含最新前端产物的镜像。

### `model_not_found`

检查渠道模型列表是否包含请求中的模型名。结构化接口请求的是 4 个内部模型，不是 `happyhorse-1.0/video`。

### 任务失败，提示素材下载失败

图片或视频 URL 不能被上游直接下载。换用公网可访问的 `http/https` URL 后重试。

### `aspect_ratio` 未返回

查询响应中的 `aspect_ratio` 来自上游 `usage.ratio`。如果上游未返回，该字段可能为空。

### 价格不符合预期

按以下顺序检查：

1. 模型是否配置了 `model_price`。
2. 用户分组倍率是否为预期值。
3. 分辨率是否为 `720P` 或 `1080P`。
4. 是否因缺省 resolution 使用了默认 `1080P`。
5. 完成后上游 `usage.output_video_duration`、`usage.input_video_duration` 是否与预期一致。
6. 是否发生了补扣或退款。
