# Seedance 代码评审修复总结报告

更新时间：2026-06-04
分支：`feat/seedance-channel`
Commit: `3df4d132`

---

## 执行概览

### ✅ 已完成修复（4/6）

| 优先级 | 问题 | 状态 | 工作量 | 影响 |
|---|---|---|---|---|
| 🔴 P0 | doubao 计费逻辑变更（token 优先级） | ✅ 已修复 | 2 小时 | 防止 doubao 计费降低 |
| 🟠 P1 | Fast + 1080p 未拦截 | ✅ 已修复 | 30 分钟 | 避免计费错配 |
| 🟠 P2 | volcano.ParseSubmitResponse 死参数 | ✅ 已修复 | 30 分钟 | 代码质量改善 |
| 🟢 P3 | import 顺序混乱 | ✅ 已修复 | 5 分钟 | 代码风格统一 |

**总计：4 项修复，约 3 小时工作量**

### 🚧 待完成任务（2/6）

| 优先级 | 问题 | 预计工作量 | 说明 |
|---|---|---|---|
| 🟠 P1 | 补充核心计费逻辑单元测试 | 4-6 小时 | 需要为 `calculateCombinedRatio`、`normalizeResolution`、`convertVolcanoContentToTaskSubmit` 编写表驱动测试 |
| 🟠 P2 | ModelRatio 配置校验 | 1-2 小时 | 在 `seedance/adaptor.Init` 中添加启动时校验，告警配置错误 |

---

## 修复详情

### 1. 🔴 修复 doubao 计费逻辑变更（token 优先级）

#### 问题描述
在提取 `volcano` 共享包时，`ParseTaskResult` 将 doubao 的 token 计费从 `total_tokens` 改为优先 `completion_tokens`，这是未声明的行为变更，可能导致计费降低。

#### 修复方案
引入 `TokenPriority` 枚举参数化 token 选择策略：

```go
// volcano/volcano.go
type TokenPriority int

const (
    TokenPriorityCompletionFirst TokenPriority = iota  // Seedance 策略
    TokenPriorityBothFields                             // Doubao 策略（原行为）
)

func ParseTaskResult(respBody []byte, tokenPriority TokenPriority) (*relaycommon.TaskInfo, error)
```

**调用处：**
- `doubao/adaptor.go`: 使用 `TokenPriorityBothFields` 保持原行为
- `seedance/adaptor.go`: 使用 `TokenPriorityCompletionFirst` 按官方文档

#### 影响
- ✅ doubao 计费行为恢复到提取前的状态
- ✅ Seedance 按官方文档正确计费（"准确 token 用量以接口返回的 completion tokens 为准"）
- ✅ 明确区分两个平台的计费策略，避免隐式变更

#### 验证建议
```sql
-- 对比修复前后 doubao 任务的 token 和 quota
SELECT task_id, model, total_tokens, completion_tokens, quota 
FROM tasks 
WHERE platform = 'doubao' AND status = 'SUCCESS' 
  AND created_at > '2026-06-03'
ORDER BY created_at DESC LIMIT 20;
```

---

### 2. 🟠 添加 Fast 模型 + 1080p 拦截

#### 问题描述
文档明确说应拒绝 `fast + 1080p`，但 `validate.go` 未实现校验。用户提交会按 fast 基础价预扣，但上游可能拒绝（400）或按其他逻辑计价。

#### 修复方案
在 `validate.go` 的 `ValidateRequestAndSetAction` 中添加校验：

```go
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

#### 影响
- ✅ 前端校验，避免无效请求浪费资源
- ✅ 清晰的错误信息引导用户修正参数
- ✅ 防止计费错配（预扣 vs 上游实际价格）

---

### 3. 🟠 清理 volcano.ParseSubmitResponse 死参数

#### 问题描述
函数签名有 3 个参数 `(c, publicTaskID, modelName)` 但函数体内一个都没用，且注释说会写 gin context 但实际没写。

#### 修复方案
移除未使用参数，更新函数签名和所有调用处：

```go
// Before
func ParseSubmitResponse(c interface{ JSON(int, any) }, resp *http.Response, publicTaskID, modelName string) (...)

// After
func ParseSubmitResponse(resp *http.Response) (upstreamTaskID string, taskData []byte, taskErr *dto.TaskError)
```

#### 影响
- ✅ 移除误导性注释和死代码
- ✅ 简化函数签名，职责更清晰
- ✅ 调用方负责写响应（doubao 和 seedance 都已正确实现）

---

### 4. 🟢 修复 import 顺序

#### 问题描述
`relay/relay_adaptor.go` 中 seedance import 插在 jina 前面，顺序混乱。

#### 修复方案
运行 `go fmt` 自动修复 import 顺序。

#### 影响
- ✅ 代码风格统一
- ✅ 符合 Go 标准库约定

---

## 待完成任务说明

### 🟠 P1：补充核心计费逻辑单元测试

#### 背景
新增代码只有集成测试脚本（`.py`/`.sh`），没有任何 `*_test.go`。关键纯函数都未覆盖：
- `calculateCombinedRatio` — 计费核心逻辑
- `normalizeResolution` — 分辨率规范化
- `convertVolcanoContentToTaskSubmit` — 格式转换和 metadata 透传

#### 工作范围
创建 `relay/channel/task/seedance/adaptor_test.go`，实现表驱动测试：

```go
func TestCalculateCombinedRatio(t *testing.T) {
    tests := []struct {
        name          string
        modelName     string
        resolution    string
        hasVideo      bool
        expectedRatio float64
    }{
        // Main model - 720p without video (baseline)
        {"main-720p-no-video", ModelDreamina20, Resolution720P, false, 1.0},
        
        // Main model - 720p with video
        {"main-720p-video", ModelDreamina20, Resolution720P, true, 0.6087},
        
        // Main model - 1080p without video
        {"main-1080p-no-video", ModelDreamina20, Resolution1080P, false, 1.1087},
        
        // Main model - 1080p with video
        {"main-1080p-video", ModelDreamina20, Resolution1080P, true, 0.6739},
        
        // Fast model - 720p without video
        {"fast-720p-no-video", ModelDreamina20Fast, Resolution720P, false, 1.0},
        
        // Fast model - 720p with video
        {"fast-720p-video", ModelDreamina20Fast, Resolution720P, true, 0.5946},
        
        // ... 更多测试用例
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            actual := calculateCombinedRatio(tt.modelName, tt.resolution, tt.hasVideo)
            if !floatEqual(actual, tt.expectedRatio, 0.0001) {
                t.Errorf("got %v, want %v", actual, tt.expectedRatio)
            }
        })
    }
}
```

覆盖场景：
- 主版本 × (480p/720p/1080p) × (有无视频) = 6 种组合
- Fast 版本 × (480p/720p) × (有无视频) = 4 种组合
- 异常输入：空字符串、未知分辨率、未知模型

#### 优先级说明
计费逻辑直接影响扣费金额，属于"动钱的代码"，必须有单元测试覆盖。

---

### 🟠 P2：添加 ModelRatio 配置校验

#### 背景
所有折扣比率硬编码在 `constants.go`，计费正确性建立在管理员必须将 `ModelRatio` 设为特定基准价（480p/720p 不含视频）的隐式约定上。

**风险：**
- 管理员配错基准价 → 所有折扣全错
- 上游调价 → 需改 Go 代码重新部署
- 无任何校验或告警

#### 工作范围
在 `seedance/adaptor.go` 的 `Init` 方法中添加配置校验：

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

#### 优先级说明
配置错误会导致全部计费错误，但通过文档 + 启动日志告警可以降低风险，优先级低于单元测试。

---

## 代码变更统计

### Commit 1: `03b8e5a7` — 全参数透传 + volcano 共享包 + 完整文档
- 13 files changed, +3437 / -495

### Commit 2: `3df4d132` — 代码评审修复
- 6 files changed, +654 / -13
  - `volcano/volcano.go`: +41 / -4（Token 优先级参数化）
  - `doubao/adaptor.go`: +6 / -2（使用 TokenPriorityBothFields）
  - `seedance/adaptor.go`: +6 / -2（使用 TokenPriorityCompletionFirst）
  - `seedance/validate.go`: +11（Fast+1080p 拦截）
  - `relay_adaptor.go`: +4 / -2（import 顺序）
  - `code-review-fixes.md`: +586（评审文档）

**累计变更：19 files, +4091 / -508**

---

## 编译验证

所有修改已通过编译和 vet 检查：

```bash
✅ go build ./relay/channel/task/... 
✅ go vet ./relay/channel/task/...
✅ go fmt ./relay/relay_adaptor.go
```

---

## 推荐下一步行动

### 立即行动（合并前必须）
1. ✅ **已完成** — 修复 P0/P1 关键问题
2. ⚠️ **待完成** — 补充单元测试（P1）
   - 预计 4-6 小时
   - 直接关系到计费正确性
   - 建议在合并到主分支前完成

### 中期优化（可单独 PR）
3. ⚠️ **待完成** — 添加 ModelRatio 配置校验（P2）
   - 预计 1-2 小时
   - 降低配置错误风险
   - 可以在合并后单独提交

### 长期改进（需产品评估）
4. 考虑将折扣比率迁移到动态配置
   - 避免硬编码定价
   - 管理员可在前端统一配置
   - 上游调价无需重新部署

---

## 质量改进建议

基于本次评审经验，建议建立以下流程：

### 1. 共享代码提取检查清单
抽取共享代码时必须：
- [ ] 保证行为等价（或在 PR 中明确声明变更）
- [ ] 参数化策略差异（如 token 优先级）
- [ ] 单元测试覆盖共享逻辑
- [ ] 更新所有调用方
- [ ] 清理死参数和误导性注释

### 2. 计费代码审查要求
涉及计费的代码必须：
- [ ] 表驱动单元测试覆盖率 ≥ 80%
- [ ] 计费逻辑变更需要回归测试计划
- [ ] 文档化计费策略和配置要求
- [ ] 启动时校验关键配置项

### 3. Plan Mode 改进
在 Plan Mode 批准前：
- [ ] 检查是否引入隐式配置依赖
- [ ] 识别缺失的单元测试
- [ ] 评估共享代码抽取的行为等价性

---

## 总结

本次代码评审发现的问题集中在：

1. **共享代码抽取改变了既有行为** — doubao token 计费口径变化（已修复）
2. **计费逻辑缺少单元测试** — 纯函数易测但未测（待完成）
3. **隐式配置约定未校验** — ModelRatio 配置错误会导致所有折扣失效（待完成）

功能设计本身符合 KISS/DRY/SOLID 原则，volcano 共享包消除了重复代码，高级参数走 metadata 通用覆盖避免了逐字段写代码。

**关键修复（P0/P1）已完成，建议补充单元测试后合并到主分支。**

---

生成时间：2026-06-04
文档版本：v1.0
