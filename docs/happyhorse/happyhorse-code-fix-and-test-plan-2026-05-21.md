# HappyHorse 代码修复与补测方案

日期：2026-05-21

## 1. 目标

本文档记录 HappyHorse 视频兼容功能在评审后产生的代码修复项、补测项和当前完成状态。

本轮修复目标：

- 透出上游真实错误，避免泛化为 `task_id is empty`。
- 完成计费时增加分辨率 fallback，避免上游缺失 `usage.SR` 时少计费。
- 补充退款方向测试，覆盖实际输出秒数小于预扣秒数的场景。
- 补充 BuildRequestBody / wire body 测试，证明 HappyHorse 请求体不是 Ali schema。
- 提前校验 R2V `metadata.media`。
- 清理未使用代码。
- 明确并实现 `duration < 3` 本地 400 规则。

## 2. 修复任务状态

| 任务编号 | 任务 | 优先级 | 状态 |
|---|---|---:|---|
| HH-FIX-01 | 上游错误透出 | P0/P1 | 已完成 |
| HH-FIX-02 | 完成计费分辨率 fallback | P1 | 已完成 |
| HH-FIX-03 | 退款方向计费保障 | P1 | 已完成 |
| HH-FIX-04 | BuildRequestBody / wire body 测试 | P1 | 已完成 |
| HH-FIX-05 | R2V `metadata.media` 提前校验 | P2 | 已完成 |
| HH-FIX-06 | 清理未使用代码 | P2 | 已完成 |
| HH-FIX-07 | Wisemodel Redis nil 完整修复 | P2 | 不做，已从本轮移除 |
| HH-FIX-08 | `duration < 3` 本地 400 校验 | P2 | 已完成 |

## 3. HH-FIX-01：上游错误透出

### 问题

原逻辑在 `DoResponse` 中只判断上游响应里的 `output.task_id` 是否为空。

如果上游返回：

```json
{
  "output": {
    "code": "InvalidParameter",
    "message": "invalid media url"
  }
}
```

用户可能只能看到：

```text
task_id is empty in upstream response
```

### 已完成修复

修改位置：

```text
relay/channel/task/happyhorse/adaptor.go
```

新增 `upstreamErrorMessage`，在检查 `task_id` 前先检查上游 `output.code` / `output.message`。

错误消息规则：

- `code` 和 `message` 都存在：返回 `code: message`
- 只有 `code`：返回 `code`
- 只有 `message`：返回 `message`
- 两者都不存在：不视为上游错误

### 已补测试

```text
TestUpstreamErrorMessageBoth
TestUpstreamErrorMessageCodeOnly
TestUpstreamErrorMessageMsgOnly
TestUpstreamErrorMessageEmpty
```

## 4. HH-FIX-02：完成计费分辨率 fallback

### 问题

原完成计费逻辑主要依赖：

```go
resp.Usage.SR == 1080
```

如果用户提交 1080P，但上游完成响应没有返回 `usage.SR`，可能按 720P 结算，造成少计费。

### 已完成修复

修改位置：

```text
relay/channel/task/happyhorse/adaptor.go
```

分辨率判断优先级：

1. `usage.SR == 1080`：使用 1080P。
2. `usage.SR == 720`：使用 720P。
3. `usage.SR` 缺失或异常：从 `BillingContext.OtherRatios["resolution"]` 回退。
4. 无可用信息：默认 720P。

### 已补测试

```text
TestAdjustBillingOnCompleteResolutionFallback
```

## 5. HH-FIX-03：退款方向计费保障

### 问题

原测试主要覆盖补扣方向，缺少退款方向：

```text
预扣 5 秒，实际输出 3 秒，完成后应释放多扣额度。
```

### 已完成修复

新增退款方向测试，断言实际 quota 小于预扣 quota。

### 已补测试

```text
TestAdjustBillingOnCompleteRefundDirection
```

## 6. HH-FIX-04：BuildRequestBody / wire body 测试

### 问题

评审中提出 HappyHorse 请求体与 Ali adapter 的请求体不同，需要自动化测试证明 HappyHorse 输出的是文档约定格式，而不是 Ali schema。

### 已完成修复

新增 `adaptor_test.go`，覆盖：

- `/happyhorse/api/generate` 原生路径
- `/v1/video/generations` 通用路径
- 原生路径模型覆盖
- V1 R2V
- V1 video-edit

测试断言 HappyHorse 请求体包含：

```json
{
  "model": "happyhorse-1.0-i2v",
  "input": {
    "prompt": "...",
    "media": [
      {
        "type": "first_frame",
        "url": "https://example.com/image.jpg"
      }
    ]
  },
  "parameters": {
    "resolution": "720P",
    "ratio": "16:9",
    "duration": 5
  }
}
```

同时断言不出现 Ali 字段：

```text
img_url
first_frame_url
last_frame_url
size
prompt_extend
```

### 已补测试

```text
TestWireBodyV1T2V
TestWireBodyV1I2V
TestWireBodyV1R2V
TestWireBodyV1VideoEdit
TestWireBodyNativeStructured
TestBuildRequestBodyNativePath
TestBuildRequestBodyV1Path
TestBuildRequestBodyNativePathOverridesModel
TestBuildRequestBodyV1PathR2V
TestBuildRequestBodyV1PathVideoEdit
```

## 7. HH-FIX-05：R2V metadata.media 提前校验

### 问题

V1 R2V 请求允许通过 `metadata.media` 传递 HappyHorse media 结构。原逻辑只检查是否存在 `metadata.media`，具体结构在 BuildRequestBody 阶段才解析。

### 已完成修复

修改位置：

```text
relay/channel/task/happyhorse/validate.go
```

现在在校验阶段会：

1. 解析 `metadata.media`。
2. 校验至少存在一个 `reference_image`。
3. 校验该 `reference_image` 的 `url` 非空。
4. 失败时返回本地 400。

## 8. HH-FIX-06：清理未使用代码

### 已完成清理

已删除未使用或无意义包装：

```text
toHappyHorseStatus()
ModeToInternalModel()
ModeToModel
ValidModes
GenerateResponse
GenerateOutput
ErrorResponse
commonMarshal()
commonUnmarshal()
```

`convert.go` 已直接使用项目统一 JSON 方法：

```go
common.Marshal
common.Unmarshal
```

## 9. HH-FIX-07：Wisemodel Redis nil 完整修复

本任务不在本轮处理。

原因：

- 这是 Wisemodel 资源包链路的独立稳定性问题。
- 与 HappyHorse 包内修复没有直接耦合。
- 用户已确认本轮不需要处理。

当前状态：

```text
不做，已从本轮任务移除。
```

## 10. HH-FIX-08：duration < 3 本地 400 校验

### 规则

两个入口统一规则：

- `duration` 缺失：允许，后续默认 5 秒。
- 显式传入 `duration < 3`：本地返回 400。
- 显式传入 `duration >= 3`：允许。

### 已完成修复

修改位置：

```text
relay/channel/task/happyhorse/validate.go
```

原生入口：

```text
/happyhorse/api/generate
```

使用 `Parameters.Duration != nil` 判断用户是否显式传入。

V1 入口：

```text
/v1/video/generations
```

由于 `TaskSubmitReq.Duration` 是普通 `int`，无法区分“缺失”和“显式 0”，因此新增 `isDurationExplicit(c)`，通过 cached request body 判断原始 JSON 是否包含 `duration` 字段。

这样可以做到：

- 显式 `duration:0` / `duration:2` 返回 400。
- 缺失 `duration` 不误伤，仍走默认 5 秒。

### 已补测试

```text
TestValidateNativeDurationZero
TestValidateNativeDurationTooSmall
TestValidateNativeDurationMinAllowed
TestValidateNativeDurationMissing
TestValidateV1DurationTooSmall
TestValidateV1DurationMinAllowed
TestValidateV1DurationMissing
```

## 11. 补测状态

| 测试编号 | 覆盖内容 | 状态 |
|---|---|---|
| HH-TEST-01 | 上游 `output.code/message` 错误响应 | 已完成 |
| HH-TEST-02 | 错误消息边界：仅 code / 仅 message / 二者都有 | 已完成 |
| HH-TEST-03 | Native BuildRequestBody | 已完成 |
| HH-TEST-04 | V1 BuildRequestBody | 已完成 |
| HH-TEST-05 | V1 I2V wire body | 已完成 |
| HH-TEST-06 | V1 R2V wire body | 已完成 |
| HH-TEST-07 | V1 video-edit wire body | 已完成 |
| HH-TEST-08 | Native wire body | 已完成 |
| HH-TEST-09 | 不出现 Ali schema 字段 | 已完成 |
| HH-TEST-10 | 1080P SR 缺失 fallback | 已完成 |
| HH-TEST-11 | 退款方向计费 | 已完成 |
| HH-TEST-12 | R2V `metadata.media` 非法校验 | 已完成 |
| HH-TEST-13 | `duration < 3` 行为锁定 | 已完成 |

## 12. 当前验证结果

最近一次验证命令：

```bash
go test ./relay/channel/task/happyhorse -count=1
```

结果：

```text
通过
```

最近一次静态检查：

```bash
go vet ./relay/channel/task/happyhorse/...
```

结果：

```text
通过，无输出
```

## 13. 后续不在本轮处理的事项

以下事项属于架构优化或后续专项，不建议混入当前修复：

- 将 `PerCallBilling` 改为 TaskAdaptor 显式声明。
- 复用 Ali adapter。
- 重构 native 计费链路，减少 `GenerateRequest -> TaskSubmitReq -> GenerateRequest`。
- 统一 `duration` / `seconds` key。
- Wisemodel Redis nil 完整修复。

