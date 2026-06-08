# Seedance 代码评审问题修复方案

更新时间：2026-06-04

本文档记录代码评审发现的问题及修复方案。

## 🔴 问题 1：doubao 计费逻辑被静默改变

### 问题描述

在提取 `volcano` 共享包时，`volcano.ParseTaskResult` 改变了 token 选择逻辑：

**旧 doubao 代码：**
```go
taskResult.CompletionTokens = resTask.Usage.CompletionTokens
taskResult.TotalTokens = resTask.Usage.TotalTokens
```

**新 volcano.ParseTaskResult：**
```go
if resTask.Usage.CompletionTokens > 0 {
    taskResult.CompletionTokens = resTask.Usage.CompletionTokens
    taskResult.TotalTokens = resTask.Usage.CompletionTokens  // ← 用 completion 覆盖 total
} else if resTask.Usage.TotalTokens > 0 {
    taskResult.TotalTokens = resTask.Usage.TotalTokens
}
```

**影响：**
- 计费使用 `taskResult.TotalTokens`（`service/task_polling.go:556`）
- 如果上游返回 `total_tokens > completion_tokens`，迁移后 doubao 实际扣费会降低
- 这是未声明的行为变更，可能影响历史计费数据一致性

### 根因分析

1. Seedance 2.0 官方文档明确：**"准确 token 用量以接口返回的 completion tokens 为准"**
2. Doubao（豆包视频生成）官方文档未查到类似说明，需要确认
3. 提取共享代码时将 Seedance 的策略错误应用到 doubao

### 解决方案

#### 方案 A：参数化 token 选择策略（推荐）

修改 `volcano.ParseTaskResult` 增加策略参数：

```go
// TokenPriority defines which token field to use for billing
type TokenPriority int

const (
    // TokenPriorityCompletionFirst uses completion_tokens if available, else total_tokens
    TokenPriorityCompletionFirst TokenPriority = iota
    // TokenPriorityBothFields sets both CompletionTokens and TotalTokens independently
    TokenPriorityBothFields
)

// ParseTaskResult maps the upstream Volcano task response to the internal TaskInfo.
// tokenPriority controls how usage tokens are extracted for billing.
func ParseTaskResult(respBody []byte, tokenPriority TokenPriority) (*relaycommon.TaskInfo, error) {
    // ... parse logic ...
    
    case StatusSucceeded:
        taskResult.Status = model.TaskStatusSuccess
        taskResult.Progress = "100%"
        taskResult.Url = resTask.Content.VideoURL
        
        switch tokenPriority {
        case TokenPriorityCompletionFirst:
            // Seedance strategy: prefer completion_tokens
            if resTask.Usage.CompletionTokens > 0 {
                taskResult.CompletionTokens = resTask.Usage.CompletionTokens
                taskResult.TotalTokens = resTask.Usage.CompletionTokens
            } else if resTask.Usage.TotalTokens > 0 {
                taskResult.TotalTokens = resTask.Usage.TotalTokens
            }
        case TokenPriorityBothFields:
            // Doubao strategy: preserve both fields independently
            taskResult.CompletionTokens = resTask.Usage.CompletionTokens
            taskResult.TotalTokens = resTask.Usage.TotalTokens
        }
    // ...
}
```

调用处修改：
- `doubao/adaptor.go`: `volcano.ParseTaskResult(respBody, volcano.TokenPriorityBothFields)`
- `seedance/adaptor.go`: `volcano.ParseTaskResult(respBody, volcano.TokenPriorityCompletionFirst)`

**优点：**
- 明确区分两个平台的计费策略
- doubao 保持原有行为不变
- Seedance 按官方文档正确计费

#### 方案 B：回退 doubao 到独立实现（不推荐）

将 doubao 的 `ParseTaskResult` 改回本地实现，不使用 volcano 共享函数。

**缺点：**
- 违反 DRY 原则
- 放弃了共享包的维护优势

### 建议行动

1. **立即行动**：采用方案 A 参数化策略
2. **调研确认**：查阅豆包视频生成 API 文档，确认 `completion_tokens` vs `total_tokens` 的官方定义
3. **回归测试**：对比修复前后 doubao 的实际扣费金额
4. **文档化**：在 commit message 中明确说明此行为变更及影响

### 回归测试计划

```bash
# 1. 记录修复前 doubao 任务的 token 数据
# 查询最近10个成功的 doubao 任务
SELECT task_id, model, total_tokens, completion_tokens, quota 
FROM tasks 
WHERE platform = 'doubao' AND status = 'SUCCESS' 
ORDER BY created_at DESC LIMIT 10;

# 2. 应用修复

# 3. 提交相同参数的新任务，对比 token 和 quota

# 4. 验证：
# - 如果上游 total > completion，修复后 quota 应恢复到修复前水平
# - 如果上游 total == completion，修复前后 quota 应一致
```

---

## 🟠 问题 2：Fast 模型 + 1080p 未拦截

### 问题描述

文档（`seedance-dev.md`）明确写：
> 如需拒绝 fast + 1080p 的请求，应在请求校验阶段处理

但 `validate.go` 未实现校验，且 `calculateCombinedRatio` 对 fast 模型完全忽略分辨率。

**后果：**
- 用户提交 `fast + 1080p` 会按 fast 基础价预扣
- 上游可能拒绝请求（400）或按其他逻辑计价
- 计费可能错配

### 解决方案

#### 选项 A：校验阶段拒绝（推荐）

在 `validate.go` 的 `ValidateRequestAndSetAction` 中添加：

```go
// Validate prompt
if strings.TrimSpace(req.Prompt) == "" {
    return &dto.TaskError{...}
}

// Reject fast model + 1080p (fast models only support 480p/720p)
if IsFastModel(req.Model) && normalizeResolution(req.Size) == Resolution1080P {
    return &dto.TaskError{
        Code:        "invalid_resolution",
        Message:     "Fast models do not support 1080p resolution. Please use 480p or 720p.",
        StatusCode:  http.StatusBadRequest,
        LocalError:  true,
        Error:       errors.New("fast model does not support 1080p"),
    }
}
```

#### 选项 B：文档化降级策略（不推荐）

接受请求但在文档中说明：
- 系统不拦截 fast + 1080p 请求
- 上游会拒绝并返回 400 错误
- 任务失败后自动退款

**缺点：**
- 浪费用户和系统资源（提交 → 上游拒绝 → 退款）
- 错误体验比前端拦截差

### 建议行动

采用选项 A，在 `validate.go` 中添加校验。

---

## 🟠 问题 3：计费强依赖管理员配置正确的 ModelRatio

### 问题描述

`constants.go` 中硬编码了折扣比率：
```go
videoInputRatioMap = map[string]float64{
    ModelDreamina20: 28.0 / 46.0,  // 基于官方单价 46 元/百万token
    ...
}
```

整个计费正确性建立在管理员必须将 `ModelRatio` 设为 **480p/720p 不含视频** 的基准价（46 元/百万token → ratio 3.1507）。

**风险：**
- 管理员配错基准价 → 所有折扣全错
- 上游调价 → 需改 Go 代码重新部署
- 无任何校验或告警

### 解决方案

#### 短期方案：启动时校验（推荐）

在 `seedance/adaptor.go` 的 `Init` 方法中添加：

```go
func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
    a.ChannelType = info.ChannelType
    a.baseURL = info.ChannelBaseUrl
    a.apiKey = info.ApiKey
    
    // Validate ModelRatio configuration
    a.validateModelRatioConfig(info.OriginModelName)
}

func (a *TaskAdaptor) validateModelRatioConfig(modelName string) {
    // Expected base ratios (480p/720p without video input)
    expectedBaseRatios := map[string]float64{
        ModelDreamina20:     3.1507,  // 46 元/百万token
        ModelDreamina20Fast: 2.5342,  // 37 元/百万token
        ModelDoubao20:       3.1507,
        ModelDoubao20Fast:   2.5342,
    }
    
    expectedRatio, ok := expectedBaseRatios[modelName]
    if !ok {
        return  // Unknown model, skip validation
    }
    
    // Get actual configured ratio
    actualRatio, err := model.GetModelRatio(modelName)
    if err != nil || actualRatio == 0 {
        common.LogWarn(ctx, fmt.Sprintf(
            "Seedance model %s has no ModelRatio configured. "+
            "Expected base ratio: %.4f (for 480p/720p without video input). "+
            "Please configure in System Settings → Model Pricing.",
            modelName, expectedRatio,
        ))
        return
    }
    
    // Check if within ±5% tolerance
    tolerance := 0.05
    lowerBound := expectedRatio * (1 - tolerance)
    upperBound := expectedRatio * (1 + tolerance)
    
    if actualRatio < lowerBound || actualRatio > upperBound {
        common.LogWarn(ctx, fmt.Sprintf(
            "Seedance model %s ModelRatio (%.4f) differs from expected base ratio (%.4f). "+
            "This may cause incorrect billing. Base ratio assumes 480p/720p without video input. "+
            "Please verify configuration in System Settings → Model Pricing.",
            modelName, actualRatio, expectedRatio,
        ))
    }
}
```

#### 长期方案：动态配置（可选）

将折扣比率迁移到数据库配置表，与 `ModelRatio` 一起管理：

```sql
CREATE TABLE model_pricing_rules (
    id INT PRIMARY KEY AUTO_INCREMENT,
    model_name VARCHAR(255) NOT NULL,
    base_ratio DECIMAL(10,4),           -- 基准 ModelRatio
    video_input_discount DECIMAL(10,4), -- 视频输入折扣
    resolution_1080p_multiplier DECIMAL(10,4), -- 1080p 倍率
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY uk_model (model_name)
);
```

**优点：**
- 管理员可在前端统一配置所有计费参数
- 上游调价无需重新部署
- 配置历史可追溯

**缺点：**
- 需要前端配置界面开发
- 数据库迁移成本

### 建议行动

1. **立即**：实施短期方案，添加启动时校验
2. **中期**：文档化配置要求（已在 `seedance-admin-model-pricing-billing.md` 中）
3. **长期**：评估动态配置方案的优先级

---

## 🟠 问题 4：缺失关键逻辑单元测试

### 问题描述

新增代码只有集成测试脚本（`.py`/`.sh`），没有任何 `*_test.go`。

关键纯函数都未覆盖：
- `calculateCombinedRatio` - 计费核心逻辑
- `normalizeResolution` - 分辨率规范化
- `convertVolcanoContentToTaskSubmit` - 格式转换和 metadata 透传

### 解决方案

创建 `relay/channel/task/seedance/adaptor_test.go`：

```go
package seedance

import (
	"encoding/json"
	"testing"
	
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestCalculateCombinedRatio(t *testing.T) {
	tests := []struct {
		name          string
		modelName     string
		resolution    string
		hasVideo      bool
		expectedRatio float64
	}{
		// Main model - 480p/720p without video (baseline)
		{"main-720p-no-video", ModelDreamina20, Resolution720P, false, 1.0},
		{"main-480p-no-video", ModelDreamina20, Resolution480P, false, 1.0},
		
		// Main model - 480p/720p with video
		{"main-720p-video", ModelDreamina20, Resolution720P, true, 0.6087},
		{"main-480p-video", ModelDreamina20, Resolution480P, true, 0.6087},
		
		// Main model - 1080p without video
		{"main-1080p-no-video", ModelDreamina20, Resolution1080P, false, 1.1087},
		
		// Main model - 1080p with video
		{"main-1080p-video", ModelDreamina20, Resolution1080P, true, 0.6739},
		
		// Fast model - 480p/720p without video
		{"fast-720p-no-video", ModelDreamina20Fast, Resolution720P, false, 1.0},
		
		// Fast model - 480p/720p with video
		{"fast-720p-video", ModelDreamina20Fast, Resolution720P, true, 0.5946},
		
		// Fast model - 1080p (should not happen after validation, but test anyway)
		{"fast-1080p-no-video", ModelDreamina20Fast, Resolution1080P, false, 1.0},
		{"fast-1080p-video", ModelDreamina20Fast, Resolution1080P, true, 0.5946},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := calculateCombinedRatio(tt.modelName, tt.resolution, tt.hasVideo)
			if !floatEqual(actual, tt.expectedRatio, 0.0001) {
				t.Errorf("calculateCombinedRatio() = %v, want %v", actual, tt.expectedRatio)
			}
		})
	}
}

func TestNormalizeResolution(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"480p", Resolution480P},
		{"720p", Resolution720P},
		{"1080p", Resolution1080P},
		{"480P", Resolution480P},
		{"720P", Resolution720P},
		{"1080P", Resolution1080P},
		{"", Resolution720P},          // default
		{"invalid", Resolution720P},   // default
		{"360p", Resolution720P},      // unsupported → default
	}
	
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			actual := normalizeResolution(tt.input)
			if actual != tt.expected {
				t.Errorf("normalizeResolution(%q) = %q, want %q", tt.input, actual, tt.expected)
			}
		})
	}
}

func TestConvertVolcanoContentToTaskSubmit(t *testing.T) {
	tests := []struct {
		name     string
		input    VolcanoRequestBody
		raw      map[string]json.RawMessage
		expected relaycommon.TaskSubmitReq
	}{
		{
			name: "text-only",
			input: VolcanoRequestBody{
				Model: ModelDreamina20,
				Content: []ContentItemInput{
					{Type: "text", Text: "A cat playing"},
				},
			},
			raw: map[string]json.RawMessage{
				"model":   json.RawMessage(`"dreamina-seedance-2-0-260128"`),
				"content": json.RawMessage(`[{"type":"text","text":"A cat playing"}]`),
			},
			expected: relaycommon.TaskSubmitReq{
				Model:  ModelDreamina20,
				Prompt: "A cat playing",
				Metadata: map[string]interface{}{
					"content": []map[string]interface{}{},
				},
			},
		},
		{
			name: "image-with-role",
			input: VolcanoRequestBody{
				Model: ModelDreamina20,
				Content: []ContentItemInput{
					{Type: "text", Text: "animate this"},
					{
						Type: "image_url",
						ImageURL: &struct{ URL string `json:"url"` }{
							URL: "https://example.com/img.png",
						},
						Role: "first_frame",
					},
				},
			},
			raw: map[string]json.RawMessage{
				"model": json.RawMessage(`"dreamina-seedance-2-0-260128"`),
				"content": json.RawMessage(`[
					{"type":"text","text":"animate this"},
					{"type":"image_url","image_url":{"url":"https://example.com/img.png"},"role":"first_frame"}
				]`),
			},
			expected: relaycommon.TaskSubmitReq{
				Model:  ModelDreamina20,
				Prompt: "animate this",
				Images: []string{"https://example.com/img.png"},
				Metadata: map[string]interface{}{
					"content": []map[string]interface{}{
						{
							"type":      "image_url",
							"image_url": map[string]string{"url": "https://example.com/img.png"},
							"role":      "first_frame",
						},
					},
				},
			},
		},
		{
			name: "advanced-params-passthrough",
			input: VolcanoRequestBody{
				Model: ModelDreamina20,
				Content: []ContentItemInput{
					{Type: "text", Text: "test"},
				},
			},
			raw: map[string]json.RawMessage{
				"model":      json.RawMessage(`"dreamina-seedance-2-0-260128"`),
				"content":    json.RawMessage(`[{"type":"text","text":"test"}]`),
				"ratio":      json.RawMessage(`"9:16"`),
				"watermark":  json.RawMessage(`false`),
			},
			expected: relaycommon.TaskSubmitReq{
				Model:  ModelDreamina20,
				Prompt: "test",
				Metadata: map[string]interface{}{
					"ratio":     json.RawMessage(`"9:16"`),
					"watermark": json.RawMessage(`false`),
					"content":   []map[string]interface{}{},
				},
			},
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := convertVolcanoContentToTaskSubmit(tt.input, tt.raw)
			
			// Verify fields
			if actual.Model != tt.expected.Model {
				t.Errorf("Model = %v, want %v", actual.Model, tt.expected.Model)
			}
			if actual.Prompt != tt.expected.Prompt {
				t.Errorf("Prompt = %v, want %v", actual.Prompt, tt.expected.Prompt)
			}
			// ... more assertions
		})
	}
}

func floatEqual(a, b, epsilon float64) bool {
	return (a-b) < epsilon && (b-a) < epsilon
}
```

### 建议行动

1. **立即**：创建 `adaptor_test.go` 并实现上述测试
2. 运行 `go test ./relay/channel/task/seedance/... -v` 验证
3. 在 CI/CD 中强制要求计费相关代码的测试覆盖率 ≥ 80%

---

## 🟠 问题 5：volcano.ParseSubmitResponse 死参数

### 问题描述

函数签名有 3 个参数但函数体内一个都没用：

```go
func ParseSubmitResponse(c interface{ JSON(int, any) }, publicTaskID, modelName string) (string, []byte, *dto.TaskError)
```

注释说 "It also writes the OpenAIVideo response to the gin context" 但实际没有写（由调用方负责）。

### 解决方案

#### 方案 A：移除未使用参数（推荐）

```go
// ParseSubmitResponse parses the upstream submit response and returns the upstream task ID.
// The caller is responsible for writing the response to the gin context.
func ParseSubmitResponse(resp *http.Response) (upstreamTaskID string, responseBody []byte, taskErr *dto.TaskError) {
	defer resp.Body.Close()
	// ... implementation unchanged ...
}
```

调用处修改：
```go
// doubao/adaptor.go
upstreamTaskID, responseBody, taskErr := volcano.ParseSubmitResponse(resp)

// seedance/adaptor.go
upstreamTaskID, responseBody, taskErr := volcano.ParseSubmitResponse(resp)
```

#### 方案 B：保留参数但添加 `_ =` 标记（不推荐）

明确标记为未使用：
```go
func ParseSubmitResponse(c interface{ JSON(int, any) }, publicTaskID, modelName string) (string, []byte, *dto.TaskError) {
	_, _, _ = c, publicTaskID, modelName  // explicitly unused
	// ...
}
```

**缺点：**
- 不解决根本问题
- 未来维护者可能误以为需要使用这些参数

### 建议行动

采用方案 A，清理函数签名并更新调用处。

---

## 🟢 问题 6：次要问题

### 6.1 测试渠道按钮失败

Seedance stub adaptor 全部返回 `unsupported()`，导致"测试渠道"按钮失败。

**解决方案**：文档注明此为框架限制，非功能缺陷。

### 6.2 import 顺序混乱

`relay/relay_adaptor.go` 中 seedance import 顺序不符合规范。

**解决方案**：运行 `gofmt -w` 和 `goimports -w` 格式化代码。

---

## 修复优先级

| 优先级 | 问题 | 风险 | 工作量 |
|---|---|---|---|
| 🔴 P0 | doubao 计费逻辑变更 | 影响实际扣费金额 | 2-4 小时 |
| 🟠 P1 | Fast + 1080p 未拦截 | 计费错配 + 错误体验 | 30 分钟 |
| 🟠 P1 | 补充单元测试 | 计费逻辑未验证 | 4-6 小时 |
| 🟠 P2 | ModelRatio 配置校验 | 配置错误导致计费全错 | 1-2 小时 |
| 🟠 P2 | 清理死参数 | 代码质量 | 30 分钟 |
| 🟢 P3 | import 顺序 | 代码风格 | 5 分钟 |

---

## 总结

功能设计本身没有大问题，符合 KISS/DRY/SOLID 原则。主要问题集中在：

1. **共享代码抽取改变了既有行为** — doubao token 计费口径变化
2. **计费逻辑缺少单元测试** — 纯函数易测但未测
3. **隐式配置约定未校验** — ModelRatio 配置错误会导致所有折扣失效

建议优先修复 P0/P1 问题后再合并到主分支。
