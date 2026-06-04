# HappyHorse 视频兼容实施计划

> **给执行代理：** 实施本计划时必须使用 `superpowers:subagent-driven-development`（推荐）或 `superpowers:executing-plans`，按任务逐步执行。任务步骤使用 checkbox（`- [ ]`）跟踪进度。

**目标：** 在 `new-api` 中新增 HappyHorse 视频任务能力，同时支持 `/v1/video/generations` 兼容入口和 `/happyhorse/api/*` 原生入口，并复用现有鉴权、渠道分发、任务存储、计费、结算和失败退款链路。

**架构：** 新增独立 HappyHorse 渠道类型和 `relay/channel/task/happyhorse` adaptor，不复用 Ali/DashScope 渠道类型。`/happyhorse/api/generate` 对外回退为 HappyHorse/DashScope 结构化提交格式；`/happyhorse/api/status/{task_id}` 使用 path 参数查询并返回 HappyHorse 简洁查询格式。`/v1/video/generations` 保留 new-api 通用格式，按模型名区分 HappyHorse 能力。

**2026-05-20 回退口径：** `/happyhorse/api/generate` 回退为 `model + input + parameters` 结构化请求，不再使用官网扁平提交结构。`/happyhorse/api/status` 从 query 参数改为 `GET /happyhorse/api/status/{task_id}`；响应仍返回 `task_id`、`status`、`message`、`data.video_url`、`data.resultUrls`。

**技术栈：** Go 1.22+、Gin、GORM、现有 `RelayTask` 任务链路、DashScope 异步任务 API、`common.Marshal` / `common.Unmarshal`。

---

## 参考文档

实施前必须先阅读以下文档，确保接口、协作流程和测试口径一致：

- `AGENTS.md`：项目级开发约定，尤其是 JSON 编解码、数据库兼容和受保护项目信息。
- `docs/happyhorse-api-docs.md`：根据 HappyHorse 官方文档整理的接口说明。
- `docs/happyhorse-video-compat-dev.md`：HappyHorse 兼容层开发文档。
- `docs/development/ai-coding-adoption-guide.zh.md`：AI 编码协作采用指南。
- `docs/development/team-workflow-guide.zh.md`：团队开发工作流指南。
- `D:\pythonproject\happhoserNewapi\docs\happyhorse 接口文档`：HappyHorse/DashScope 上游文档源。

## 执行规范

- 按任务小步实现，不要把渠道、路由、计费、状态映射和文档一次性混在同一个无边界改动里。
- 每个涉及行为变化的任务先写测试，再写实现。
- 修改业务 JSON 编解码时必须使用 `common.Marshal` / `common.Unmarshal`。
- 不新增数据库迁移；首版复用现有 `model.Task` 和 `PrivateData`。
- 不改 `/v1/video/generations` 既有通用响应格式。
- 不暴露上游 HappyHorse task id。
- 不绕过 `TokenAuth`、`Distribute`、`PreConsumeBilling`、`SettleBilling` 或失败退款。
- 不删除或替换项目中受保护的 `new-api` 与 `QuantumNous` 信息。

## Git 规范

### 分支

开发前创建独立分支，本任务指定使用：

```text
lc_y/happyhorse-video-compat
```

如果当前工作区已有用户未提交改动，先确认改动范围，不要回滚、覆盖或格式化无关文件。

### 提交粒度

按可验证任务小步提交，推荐提交边界：

- 新增渠道常量和 adaptor 注册。
- 新增 HappyHorse DTO。
- 新增字段转换和测试。
- 新增计费 helper 和测试。
- 新增 adaptor 请求/响应解析。
- 新增路由和分发。
- 新增状态映射和查询格式转换。
- 新增或更新文档。

避免把无关重构、格式化、前端变更、数据库迁移混入同一提交。

### 提交信息

提交信息使用简短英文前缀，说明行为：

```text
feat: add happyhorse channel type
feat: add happyhorse request conversion
feat: add happyhorse task adaptor
test: cover happyhorse billing adjustment
fix: preserve video generations response format
docs: update happyhorse compatibility plan
```

### 暂存规则

提交前只暂存本任务相关文件。优先使用精确路径：

```powershell
git add constant/channel.go relay/relay_adaptor.go
git add relay/channel/task/happyhorse
git add docs/happyhorse-video-compat-dev.md
```

不要使用不加检查的全量暂存：

```powershell
git add .
```

除非已经确认工作区没有无关改动。

### 忽略文件

`docs/superpowers/plans/` 命中 `.gitignore` 的 `plans` 规则，计划文件默认不会出现在 `git status` 中。

如果需要把计划文件纳入版本库，必须显式确认后再使用强制添加：

```powershell
git add -f "docs/superpowers/plans/2026-05-18-HappyHorse视频兼容实施计划.md"
```

不要在未确认目的的情况下修改 `.gitignore`。

### PR 要求

PR 描述必须包含：

- 需求背景。
- 修改范围。
- 接口变化。
- 计费变化。
- 任务 ID 暴露策略。
- 已运行测试。
- 未运行测试及原因。
- 已知风险。

PR 合并前必须确保 HappyHorse 相关测试通过，或明确说明阻塞原因。

## 已确定需求

- 对外接口：
  - `POST /v1/video/generations`
  - `GET /v1/video/generations/{task_id}`
  - `POST /happyhorse/api/generate`
  - `GET /happyhorse/api/status/{task_id}`
- `/happyhorse/api/generate` 接受 HappyHorse/DashScope 结构化格式：`model + input + parameters`。
- `/happyhorse/api/generate` 不再使用官网扁平提交格式：`model`、`prompt`、`mode`、`duration`、`aspect_ratio`、`quality`、`sound`。
- `/happyhorse/api/status/{task_id}` 使用 path 参数 `task_id`，该 `task_id` 是 new-api 公开任务 ID。
- `/v1/video/generations/{task_id}` 保留当前 new-api 通用查询格式。
- `/happyhorse/api/status/{task_id}` 返回 HappyHorse 简洁查询格式。
- 所有入口都必须走 new-api 计费。
- 新增 HappyHorse 渠道类型，不复用 Ali/DashScope 渠道类型。
- 用户只看到 new-api 公开任务 ID，上游 HappyHorse 任务 ID 只保存在内部。
- 任务完成后直接返回上游 `video_url`，不下载、不转存、不代理、不重写。
- 上游视频 URL 有效期 24 小时，存储在 Ali OSS 桶，无需签名处理。
- 首版不支持 callback。

## 支持模型

| 模型 | 能力 | 要求 |
| --- | --- | --- |
| `happyhorse-1.0-t2v` | 文生视频 | 只需要提示词 |
| `happyhorse-1.0-i2v` | 图生视频，基于首帧 | 需要一张首帧图 |
| `happyhorse-1.0-r2v` | 参考生视频 | 需要一张或多张参考图 |
| `happyhorse-1.0-video-edit` | 视频编辑 | 必须提供 `metadata.video_url` |

## 字段映射

当前 `/v1/video/generations` 实际解析为 `relay/common.TaskSubmitReq`：

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

映射到 HappyHorse canonical request：

| new-api 字段 | HappyHorse 字段 | 规则 |
| --- | --- | --- |
| `model` | `model` | 必须是支持的 HappyHorse 模型 |
| `prompt` | `prompt` | 必填 |
| `mode` | `mode` | 优先使用请求值；缺失时按模型能力补默认值 |
| `duration` | `duration` | 缺失时默认 `5` |
| `metadata.ratio` | `aspect_ratio` | 只支持 `16:9` / `9:16` / `1:1` |
| `metadata.quality` | `quality` | 透传 HappyHorse 官网字段 |
| `metadata.sound` | `sound` | 透传 HappyHorse 官网字段；显式 `false` 必须保留 |
| `metadata.resolution` | 内部计费/上游转换字段 | 只支持 `720P` / `1080P` |
| `size` | 内部计费/上游转换字段 | 仅当值精确为 `720P` / `1080P` 时接受 |
| `image` / `images` / `input_reference` / `metadata.video_url` | 内部 media 转换字段 | 用于生成上游 media，不作为官网扁平字段直接暴露 |

媒体映射：

- `happyhorse-1.0-t2v`：不生成 `input.media`。
- `happyhorse-1.0-i2v`：图片优先级为 `image > images[0] > input_reference`，输出 `type=first_frame`。
- `happyhorse-1.0-r2v`：优先使用 `metadata.media`；否则使用 `images[]`，再回退到 `image` 或 `input_reference`；输出 `type=reference_image`。
- `happyhorse-1.0-video-edit`：`metadata.video_url` 输出为 `type=video`；`metadata.reference_images[]` 输出为 `type=reference_image`。

最小校验：

- `duration` 默认 `5`。
- `resolution` 只支持 `720P` / `1080P`。
- `ratio` 只支持 `16:9` / `9:16` / `1:1`。
- 不从宽高、图片尺寸、视频尺寸、`1280x720` 等字符串自动推断分辨率或比例。
- 媒体格式、大小、数量、视频时长、帧率等完整约束交给上游校验；代码中保留明确 TODO 注释。

## 计费规则

借用 Sora 的任务计费链路，不复用 Sora 的倍率常量：

```text
ModelPriceHelperPerCall
-> HappyHorse EstimateBilling
-> OtherRatios
-> PreConsumeBilling
-> SettleBilling
-> 完成后补差 / 失败退款
```

价格口径：

```text
720P  = 0.9 元/秒
1080P = 1.6 元/秒
```

建议模型基础价格配置为：

```text
modelPrice = 0.9
```

预扣公式：

```text
quota = 0.9 * group_ratio * duration * resolutionRatio
```

倍率：

```text
720P  => 1
1080P => 1.6 / 0.9 = 1.7777778
```

完成后补差：

- 优先读取 `usage.output_video_duration`。
- 如果缺失，读取 `usage.duration`。
- 如果二者都缺失，保持预扣额度。
- 视频编辑只按输出秒数计费，忽略 `usage.input_video_duration`。
- 失败任务继续使用现有退款逻辑。

## 文件结构

- 修改 `constant/channel.go`：新增 HappyHorse 渠道类型和显示名称。
- 修改 `relay/relay_adaptor.go`：新渠道类型返回 HappyHorse task adaptor。
- 修改 `router/video-router.go`：注册 `/happyhorse/api/generate` 和 `/happyhorse/api/status/:task_id`。
- 修改 `middleware/distributor.go`：识别 HappyHorse 原生入口并设置视频提交/查询 relay mode。
- 新增 `relay/channel/task/happyhorse/constants.go`：模型名、状态、默认值、倍率常量。
- 新增 `relay/channel/task/happyhorse/dto.go`：HappyHorse 请求、响应、media、parameters、usage DTO。
- 新增 `relay/channel/task/happyhorse/convert.go`：new-api 请求到 HappyHorse canonical request 的转换。
- 新增 `relay/channel/task/happyhorse/billing.go`：预扣倍率和完成后补差工具。
- 新增 `relay/channel/task/happyhorse/adaptor.go`：task adaptor 实现。
- 新增 `relay/channel/task/happyhorse` 测试。

## 任务 1：新增渠道类型和 adaptor 注册

**文件：**

- 修改：`constant/channel.go`
- 修改：`relay/relay_adaptor.go`
- 验证：`go test ./relay/...`

- [ ] **步骤 1：新增 HappyHorse 渠道常量**

在 `constant/channel.go` 中选择一个未使用的稳定整数，例如：

```go
ChannelTypeHappyHorse = 65
```

在渠道名称映射中加入：

```go
ChannelTypeHappyHorse: "HappyHorse",
```

- [ ] **步骤 2：注册 task adaptor**

在 `relay/relay_adaptor.go` 中导入：

```go
taskhappyhorse "github.com/QuantumNous/new-api/relay/channel/task/happyhorse"
```

在 `GetTaskAdaptor` 中加入：

```go
case constant.ChannelTypeHappyHorse:
	return &taskhappyhorse.TaskAdaptor{}
```

- [ ] **步骤 3：编译检查**

运行：

```powershell
go test ./relay/... -run TestNonExistent -count=0
```

预期：如果 HappyHorse 包尚未创建，会因为新包不存在而失败；创建包后应通过编译。

## 任务 2：新增 DTO 和常量

**文件：**

- 新增：`relay/channel/task/happyhorse/constants.go`
- 新增：`relay/channel/task/happyhorse/dto.go`

- [ ] **步骤 1：新增常量**

创建 `relay/channel/task/happyhorse/constants.go`：

```go
package happyhorse

const (
	ChannelName = "happyhorse"

	ModelT2V       = "happyhorse-1.0-t2v"
	ModelI2V       = "happyhorse-1.0-i2v"
	ModelR2V       = "happyhorse-1.0-r2v"
	ModelVideoEdit = "happyhorse-1.0-video-edit"

	DefaultDuration   = 5
	DefaultResolution = "720P"

	StatusPending   = "PENDING"
	StatusRunning   = "RUNNING"
	StatusSucceeded = "SUCCEEDED"
	StatusFailed    = "FAILED"
	StatusCanceled  = "CANCELED"
	StatusUnknown   = "UNKNOWN"
)

var ModelList = []string{
	ModelT2V,
	ModelI2V,
	ModelR2V,
	ModelVideoEdit,
}

func IsHappyHorseModel(model string) bool {
	switch model {
	case ModelT2V, ModelI2V, ModelR2V, ModelVideoEdit:
		return true
	default:
		return false
	}
}
```

- [ ] **步骤 2：新增 DTO**

创建 `relay/channel/task/happyhorse/dto.go`：

```go
package happyhorse

type GenerateRequest struct {
	Model      string      `json:"model"`
	Input      Input       `json:"input"`
	Parameters *Parameters `json:"parameters,omitempty"`
}

type Input struct {
	Prompt string      `json:"prompt,omitempty"`
	Media  []MediaItem `json:"media,omitempty"`
}

type MediaItem struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

type Parameters struct {
	Resolution string `json:"resolution,omitempty"`
	Ratio      string `json:"ratio,omitempty"`
	Duration   int    `json:"duration,omitempty"`
	Watermark  *bool  `json:"watermark,omitempty"`
	Seed       *int   `json:"seed,omitempty"`
}

type GenerateResponse struct {
	Output    Output `json:"output"`
	RequestID string `json:"request_id,omitempty"`
}

type StatusResponse struct {
	Output    Output `json:"output"`
	Usage     *Usage `json:"usage,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

type Output struct {
	TaskID        string `json:"task_id,omitempty"`
	TaskStatus    string `json:"task_status,omitempty"`
	SubmitTime    string `json:"submit_time,omitempty"`
	ScheduledTime string `json:"scheduled_time,omitempty"`
	EndTime       string `json:"end_time,omitempty"`
	OrigPrompt    string `json:"orig_prompt,omitempty"`
	VideoURL      string `json:"video_url,omitempty"`
	Code          string `json:"code,omitempty"`
	Message       string `json:"message,omitempty"`
}

type Usage struct {
	Duration            float64 `json:"duration,omitempty"`
	InputVideoDuration  float64 `json:"input_video_duration,omitempty"`
	OutputVideoDuration float64 `json:"output_video_duration,omitempty"`
	VideoCount          int     `json:"video_count,omitempty"`
	SR                  int     `json:"SR,omitempty"`
	Ratio               string  `json:"ratio,omitempty"`
}

type ErrorResponse struct {
	Code      string `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}
```

- [ ] **步骤 3：编译检查**

运行：

```powershell
go test ./relay/channel/task/happyhorse -count=0
```

预期：包可以编译。

## 任务 3：编写转换测试

**文件：**

- 新增：`relay/channel/task/happyhorse/convert_test.go`
- 后续新增：`relay/channel/task/happyhorse/convert.go`

- [ ] **步骤 1：新增失败测试**

创建 `relay/channel/task/happyhorse/convert_test.go`，覆盖：

- t2v 基础转换。
- `duration` 缺失时默认 `5`。
- i2v 图片优先级 `image > images[0] > input_reference`。
- r2v `metadata.media` 直通。
- video-edit 缺少 `metadata.video_url` 时报错。
- video-edit 生成 `video` 和 `reference_image` media。
- 拒绝不支持的 `resolution`。
- 拒绝不支持的 `ratio`。

核心断言示例：

```go
got, err := ConvertTaskSubmitReq(relaycommon.TaskSubmitReq{
	Model:  ModelVideoEdit,
	Prompt: "put on striped sweater",
	Metadata: map[string]interface{}{
		"video_url": "https://example.com/input.mp4",
		"reference_images": []interface{}{
			"https://example.com/ref.png",
		},
	},
})

require.NoError(t, err)
require.Len(t, got.Input.Media, 2)
require.Equal(t, "video", got.Input.Media[0].Type)
require.Equal(t, "https://example.com/input.mp4", got.Input.Media[0].URL)
require.Equal(t, "reference_image", got.Input.Media[1].Type)
require.Equal(t, "https://example.com/ref.png", got.Input.Media[1].URL)
```

- [ ] **步骤 2：运行测试确认失败**

运行：

```powershell
go test ./relay/channel/task/happyhorse -run Convert -count=1
```

预期：因 `ConvertTaskSubmitReq` 尚不存在而失败。

## 任务 4：实现字段转换

**文件：**

- 新增：`relay/channel/task/happyhorse/convert.go`
- 测试：`relay/channel/task/happyhorse/convert_test.go`

- [ ] **步骤 1：实现转换函数**

创建 `ConvertTaskSubmitReq(req relaycommon.TaskSubmitReq) (*GenerateRequest, error)`：

- 校验模型必须是 HappyHorse 模型。
- 校验 `prompt` 非空。
- 构造 `parameters`，默认 `duration=5`、`resolution=720P`。
- `metadata.resolution` 或精确 `size=720P/1080P` 覆盖分辨率。
- `metadata.ratio` 只接受 `16:9`、`9:16`、`1:1`。
- `metadata.seed` 和 `metadata.watermark` 透传。
- 如果存在 `metadata.media`，优先按 HappyHorse media 数组解析。
- 按模型生成媒体字段。

必须遵守项目约定：业务 JSON 编解码使用 `common.Marshal` / `common.Unmarshal`，不要直接调用 `encoding/json` 的 marshal/unmarshal。

- [ ] **步骤 2：运行转换测试**

运行：

```powershell
go test ./relay/channel/task/happyhorse -run Convert -count=1
```

预期：全部通过。

## 任务 5：实现计费测试和工具

**文件：**

- 新增：`relay/channel/task/happyhorse/billing_test.go`
- 新增：`relay/channel/task/happyhorse/billing.go`

- [ ] **步骤 1：新增计费测试**

覆盖：

- `720P` 倍率为 `1`。
- `1080P` 倍率为 `1.7777778`。
- `BillableDuration` 优先使用 `usage.output_video_duration`。
- 缺失 `output_video_duration` 时回退到 `usage.duration`。
- usage 缺失时返回 `0`，表示保持预扣。

- [ ] **步骤 2：实现计费工具**

创建 `relay/channel/task/happyhorse/billing.go`：

```go
package happyhorse

import "strings"

func ResolutionRatio(resolution string) float64 {
	switch strings.ToUpper(resolution) {
	case "1080P":
		return 1.6 / 0.9
	default:
		return 1.0
	}
}

func BillableDuration(usage *Usage) float64 {
	if usage == nil {
		return 0
	}
	if usage.OutputVideoDuration > 0 {
		return usage.OutputVideoDuration
	}
	if usage.Duration > 0 {
		return usage.Duration
	}
	return 0
}
```

- [ ] **步骤 3：运行测试**

运行：

```powershell
go test ./relay/channel/task/happyhorse -run "ResolutionRatio|BillableDuration" -count=1
```

预期：全部通过。

## 任务 6：实现 HappyHorse TaskAdaptor

**文件：**

- 新增：`relay/channel/task/happyhorse/adaptor.go`
- 可能修改：`relay/channel/task/happyhorse/billing.go`

- [ ] **步骤 1：实现 adaptor 基础方法**

`TaskAdaptor` 需要包含：

- `Init`
- `GetModelList`
- `GetChannelName`
- `ValidateRequestAndSetAction`
- `BuildRequestURL`
- `BuildRequestHeader`
- `BuildRequestBody`
- `DoRequest`
- `DoResponse`
- `FetchTask`
- `ParseTaskResult`
- `EstimateBilling`
- `AdjustBillingOnComplete`

上游创建 URL：

```text
{baseURL}/api/v1/services/aigc/video-generation/video-synthesis
```

上游查询 URL：

```text
{baseURL}/api/v1/tasks/{upstream_task_id}
```

请求头：

```http
Authorization: Bearer {channel_key}
Content-Type: application/json
X-DashScope-Async: enable
```

- [ ] **步骤 2：实现状态映射**

映射规则：

```go
switch resp.Output.TaskStatus {
case StatusPending:
	taskInfo.Status = model.TaskStatusQueued
	taskInfo.Progress = "10%"
case StatusRunning:
	taskInfo.Status = model.TaskStatusInProgress
	taskInfo.Progress = "50%"
case StatusSucceeded:
	taskInfo.Status = model.TaskStatusSuccess
	taskInfo.Progress = "100%"
	taskInfo.Url = resp.Output.VideoURL
case StatusFailed, StatusCanceled, StatusUnknown:
	taskInfo.Status = model.TaskStatusFailure
	taskInfo.Progress = "100%"
	taskInfo.Reason = firstNonEmpty(resp.Output.Message, resp.Output.TaskStatus)
default:
	taskInfo.Status = model.TaskStatusInProgress
	taskInfo.Progress = "30%"
}
```

- [ ] **步骤 3：实现任务创建响应**

`/happyhorse/api/generate` 返回 HappyHorse 格式，但 `output.task_id` 必须替换为 new-api 公开任务 ID：

```json
{
  "output": {
    "task_status": "PENDING",
    "task_id": "task_xxx"
  },
  "request_id": "..."
}
```

`/v1/video/generations` 保留当前通用任务创建格式。

- [ ] **步骤 4：编译检查**

运行：

```powershell
go test ./relay/channel/task/happyhorse -count=1
go test ./relay/... -run TestNonExistent -count=0
```

预期：HappyHorse 包通过，relay 编译通过或仅有无关既有失败。

## 任务 7：新增 HappyHorse 原生路由和分发

**文件：**

- 修改：`router/video-router.go`
- 修改：`middleware/distributor.go`

- [ ] **步骤 1：注册路由**

在 `router/video-router.go` 中新增：

```go
happyHorseRouter := router.Group("/happyhorse/api")
happyHorseRouter.Use(middleware.RouteTag("relay"))
happyHorseRouter.Use(middleware.TokenAuth(), middleware.Distribute())
{
	happyHorseRouter.POST("/generate", controller.RelayTask)
	happyHorseRouter.GET("/status/:task_id", controller.RelayTaskFetch)
}
```

- [ ] **步骤 2：分发识别 HappyHorse 路径**

在 `middleware/distributor.go` 中识别：

```text
POST /happyhorse/api/generate -> RelayModeVideoSubmit
GET  /happyhorse/api/status/{task_id} -> RelayModeVideoFetchByID
```

提交时读取请求体中的 `model` 参与渠道选择。

查询时不提前选择渠道，使用本地任务记录中的渠道和上游任务 ID。

- [ ] **步骤 3：支持 path task_id**

确保 `GET /happyhorse/api/status/{task_id}` 能把 path 中的 `task_id` 传给现有任务查询逻辑。现有 `videoFetchByIDRespBodyBuilder` 已优先读取 `c.Param("task_id")`，因此实现重点是路由改为 `GET("/status/:task_id", controller.RelayTaskFetch)`。

## 任务 8：响应格式处理

**文件：**

- 可能修改：`relay/relay_task.go`
- 修改：`relay/channel/task/happyhorse/adaptor.go`

- [ ] **步骤 1：保留 `/v1/video/generations` 通用格式**

不要把 `/v1/video/generations/{task_id}` 改成 `/v1/videos/{task_id}` 的 OpenAI Video 格式。该入口继续返回当前通用任务格式。

- [ ] **步骤 2：HappyHorse 原生查询返回文档格式**

`/happyhorse/api/status/{task_id}` 返回 HappyHorse 查询格式，并将 `output.task_id` 替换为 new-api 公开任务 ID。

- [ ] **步骤 3：直接返回上游 URL**

上游返回 `output.video_url` 后：

- 写入 `task.PrivateData.ResultURL`。
- 查询响应直接返回该 URL。
- 不走视频代理，不下载，不重写。

## 任务 9：完成后补差

**文件：**

- 修改：`relay/channel/task/happyhorse/adaptor.go`
- 修改：`relay/channel/task/happyhorse/billing.go`
- 可能新增或修改服务层计费测试。

- [ ] **步骤 1：实现 `EstimateBilling`**

读取 context 中保存的 `TaskSubmitReq`，转换为 HappyHorse request，返回：

```go
map[string]float64{
	"duration":   float64(req.Parameters.Duration),
	"resolution": ResolutionRatio(req.Parameters.Resolution),
}
```

- [ ] **步骤 2：实现完成后补差**

`AdjustBillingOnComplete` 读取上游 `StatusResponse.Usage`：

- 优先 `usage.output_video_duration`。
- 其次 `usage.duration`。
- 都没有则返回 `0`，保持预扣。
- 视频编辑忽略 `usage.input_video_duration`。

如果现有完成结算 API 不能干净支持小数秒补差，需要新增聚焦 helper 和测试，避免影响其他渠道计费。

- [ ] **步骤 3：确认失败退款不变**

不要为 HappyHorse 单独增加失败结算逻辑。失败任务继续走现有 `RefundTaskQuota`。

- [ ] **步骤 4：确认 HappyHorse 不进入 PerCallBilling 跳过分支**

HappyHorse 虽然走 `ModelPriceHelperPerCall` 和统一预扣，但完成后必须读取上游 `usage.output_video_duration` 补差。

如果 `PerCallBilling=true`，`service/task_polling.go` 会跳过 `AdjustBillingOnComplete`，导致无法按实际输出秒数重新结算。

因此 `controller/relay.go` 中 HappyHorse 模型需要排除 `PerCallBilling`：

```go
PerCallBilling: (common.StringsContains(constant.TaskPricePatches, relayInfo.OriginModelName) || relayInfo.PriceData.UsePrice) &&
	!strings.HasPrefix(relayInfo.OriginModelName, "happyhorse-")
```

该例外为计费链路必要设计，不是残留改动。

## 任务 10：测试计划

**文件：**

- 新增测试：`relay/channel/task/happyhorse/*_test.go`
- 视现有测试夹具情况新增 router / middleware 测试。

- [ ] **步骤 1：转换测试**

覆盖：

- t2v 转换。
- i2v 图片优先级 `image > images[0] > input_reference`。
- r2v `metadata.media` 直通。
- r2v `images[]` 转 `reference_image`。
- video-edit `metadata.video_url` 必填。
- video-edit `metadata.reference_images` 转 `reference_image`。
- 默认 `duration=5`。
- 只接受指定比例。
- 只接受指定分辨率。

- [ ] **步骤 2：计费测试**

覆盖：

- `720P` 倍率 `1`。
- `1080P` 倍率 `1.7777778`。
- 完成后优先按 `usage.output_video_duration` 补差。
- 缺失时按 `usage.duration` 补差。
- usage 缺失时保持预扣。
- 视频编辑忽略 `usage.input_video_duration`。

- [ ] **步骤 3：状态测试**

覆盖：

- `PENDING` -> queued。
- `RUNNING` -> in-progress / processing。
- `SUCCEEDED` 保存并返回上游 `video_url`。
- `FAILED` 保留错误 code/message。
- `CANCELED` -> failed。
- `UNKNOWN` -> failed。

- [ ] **步骤 4：验证命令**

运行：

```powershell
go test ./relay/channel/task/happyhorse -count=1
go test ./relay/... -run HappyHorse -count=1
go test ./service/... -run TaskBilling -count=1
```

预期：HappyHorse 相关测试全部通过。如有无关既有失败，记录具体 package 和失败信息。

## 全量测试失败拆解与解决方案

本分支在功能测试过程中，`go test ./...` 曾暴露以下 5 类问题。处理原则是：HappyHorse 相关失败必须修复；环境问题记录前置条件；无关既有失败独立收敛，避免混淆 HappyHorse 改动。

### 1. 缺少 `web/dist`

类型：环境 / 构建产物问题。

根包 embed 前端产物，未生成 `web/dist` 时会失败：

```text
main.go: pattern web/dist: no matching files found
```

解决方案：

- 跑根包测试前在 `web/` 执行 `bun install && bun run build`。
- 只验证后端逻辑时，可排除根包。

### 2. `scripts` 缺少 `jq`

类型：环境依赖问题。

解决方案：

- Docker 测试镜像安装 `jq`。
- 或后端验证时排除 `scripts` 包。

### 3. Redis nil panic

类型：测试环境 Redis 未初始化。

修复：

- `controller/wisemodel_package.go`：`common.RDB.Set(...)` 前增加 `common.RDB != nil` 守卫。
- `common/redis.go`：`RedisDel` / `RedisDelKey` 增加 `RDB == nil` 直接返回，避免后台 goroutine 清缓存 panic。

验证：

```powershell
go test ./controller -run TestDeleteWisemodelUser -count=1 -v
```

### 4. Claude 文件类型转换

类型：OpenAI 文件 content 转 Claude content 的兼容问题。

修复：

- 从 `GetFile().FileName` 提取扩展名。
- 使用 `service.GetMimeTypeByExtension` 推导 MIME。
- 当 `mimeType == ""` 或 `mimeType == "application/octet-stream"` 时，用扩展名推导结果覆盖。
- `text/*` 转 Claude `text`，并 base64 解码为字符串。
- `application/pdf` 转 Claude `document`。
- `image/*` 转 Claude `image`。
- 其他类型跳过。

验证：

```powershell
go test ./relay/channel/claude -count=1 -v
```

### 5. StreamStatus 预初始化错误丢失

类型：无条件重建 `StreamStatus` 导致已有错误计数丢失。

修复：

```go
if info.StreamStatus == nil {
	info.StreamStatus = relaycommon.NewStreamStatus()
}
```

验证：

```powershell
go test ./relay/helper -run TestStreamScannerHandler_StreamStatus_PreInitialized -count=1
```

### 6. HappyHorse 相关验证

验证：

```powershell
go test ./relay/channel/task/happyhorse ./relay/channel/happyhorse -count=1
```

## 评审门禁

提交评审前必须完成以下检查：

- 确认 `/v1/video/generations/{task_id}` 没有被改成 `/v1/videos/{task_id}` 的 OpenAI Video 格式。
- 确认 `/happyhorse/api/status/{task_id}` 使用公开 new-api task id 查询，响应中也只返回公开 task id。
- 确认上游 `video_url` 直接返回，没有引入下载、代理、转存或 URL 重写。
- 确认 HappyHorse 所有入口都经过 `TokenAuth`、`Distribute`、任务预扣和失败退款。
- 确认 HappyHorse 模型没有被 `PerCallBilling` 跳过完成后补差，轮询完成时会进入 `AdjustBillingOnComplete`。
- 确认没有在业务代码中直接调用 `encoding/json.Marshal` 或 `encoding/json.Unmarshal`。
- 确认没有新增数据库专用 SQL 或只兼容单一数据库的逻辑。
- 确认没有移除或改写 `new-api`、`QuantumNous` 相关受保护信息。
- 确认文档同步更新：接口行为变更时同步 `docs/happyhorse-api-docs.md` 和 `docs/happyhorse-video-compat-dev.md`。
- 确认提交只包含本任务相关文件，没有混入无关格式化、旧文档残留或用户未确认改动。
- 确认如需提交 `docs/superpowers/plans/` 下的计划文件，已使用 `git add -f` 并在 PR 中说明原因。

## 验收标准

- `/v1/video/generations` 可以创建四种 HappyHorse 模型任务。
- `/v1/video/generations/{task_id}` 保持当前 new-api 通用查询格式。
- `/happyhorse/api/generate` 接受 HappyHorse 文档格式。
- `/happyhorse/api/status/{task_id}` 返回 HappyHorse 查询格式。
- 所有入口都使用 new-api 鉴权、渠道分发、计费、任务持久化和失败退款。
- 对外只暴露 new-api 公开任务 ID。
- 上游 HappyHorse task id 只保存在内部。
- 成功任务直接返回上游 `video_url`。
- `duration` 默认 `5`。
- `resolution` 只支持 `720P` 和 `1080P`。
- `ratio` 只支持 `16:9`、`9:16`、`1:1`。
- 不实现自动分辨率或比例推断。
- 视频编辑输入视频读取 `metadata.video_url`。
- 视频编辑只按输出秒数计费。
- 完成后补差优先使用 `usage.output_video_duration`，其次使用 `usage.duration`。
- 首版不支持 callback。
