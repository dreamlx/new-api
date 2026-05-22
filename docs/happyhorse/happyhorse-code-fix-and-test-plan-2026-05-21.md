# HappyHorse 修复与补测计划

更新时间：2026-05-22

## 1. 背景

HappyHorse 基础提交和查询链路已跑通，但评审指出官方契约、计费、边界校验和测试覆盖仍有缺口。本文记录已完成的修复项和补测项。

## 2. 已完成修复

| 编号 | 问题 | 处理结果 |
| --- | --- | --- |
| HH-FIX-01 | 真实上游 schema 取证不足 | 已新增 wire body 测试和真实上游取证报告 |
| HH-FIX-02 | 上游错误信息被泛化 | `output.code/message` 会优先转成上游错误 |
| HH-FIX-03 | 1080P 完成响应缺少 `usage.SR` 时可能少计费 | 完成结算缺少 SR 时回退提交时保存的 resolution 倍率 |
| HH-FIX-04 | `PerCallBilling` 使用 `happyhorse-` 硬编码 | 改为 `TaskAdaptor.DisablePerCallBilling()` 声明 |
| HH-FIX-05 | `/happyhorse/api/status?task_id=xxx` 旧兼容 | 已收敛为 `/happyhorse/api/status/{task_id}` |
| HH-FIX-06 | 内部计费 key 使用 `duration` | 已统一为 `seconds` |
| HH-FIX-07 | 默认分辨率不符合官方默认 | `DefaultResolution` 从 `720P` 改为 `1080P` |
| HH-FIX-08 | duration 只校验下限 | T2V/I2V/R2V 显式 duration 必须为 `3-15` |
| HH-FIX-09 | Video Edit 少计费 | 改为 `input_video_duration + output_video_duration` |
| HH-FIX-10 | `usage.SR` 只支持 int | 新增 `SRValue`，兼容数字和字符串 |
| HH-FIX-11 | ratio 支持不完整 | T2V/R2V 增加 `4:3`、`3:4` |
| HH-FIX-12 | I2V ratio 语义错误 | I2V 传 ratio 返回 400 |
| HH-FIX-12B | Video Edit ratio 语义错误 | Video Edit 传 ratio 返回 400 |
| HH-FIX-13 | R2V 媒体数量限制不完整 | R2V 参考图限制为 1-9 |
| HH-FIX-14 | Video Edit 媒体限制不完整 | 正好 1 个 video，0-5 个 reference_image |
| HH-FIX-15 | `quality` / `sound` 非官方参数 | 删除 DTO 和转换逻辑，不再透传 |
| HH-FIX-16 | V1 绕过路径 | V1 也复用 media 规则校验 |
| HH-FIX-17 | 空 URL 被静默跳过 | V1 和转换层遇到空 URL 返回错误 |

## 3. 当前官方契约

| 项 | 当前实现 |
| --- | --- |
| 默认分辨率 | `1080P` |
| T2V/R2V ratio | `16:9`、`9:16`、`1:1`、`4:3`、`3:4` |
| I2V / Video Edit ratio | 不支持 |
| T2V/I2V/R2V duration | 缺省 5；显式传入 3-15 |
| Video Edit duration | 不支持 |
| I2V media | 正好 1 个 `first_frame` |
| R2V media | 1-9 个 `reference_image` |
| Video Edit media | 正好 1 个 `video`，0-5 个 `reference_image` |
| media URL | 非空，且必须为 http/https |
| `quality` / `sound` | 不支持，不透传 |

## 4. 计费规则

预扣：

```text
model_price * QuotaPerUnit * group_ratio * estimated_seconds * resolution_ratio
```

完成结算：

```text
model_price * QuotaPerUnit * group_ratio * actual_seconds * resolution_ratio
```

秒数：

- T2V/I2V/R2V：`usage.output_video_duration` 优先，其次 `usage.duration`。
- Video Edit：`usage.input_video_duration + usage.output_video_duration`；缺字段时回退 `usage.duration`。

分辨率：

- `usage.SR` 支持 `720`、`"720"`、`1080`、`"1080"`。
- `usage.SR` 缺失或非法时回退提交时保存的 resolution 倍率。

## 5. 补测项

### 参数校验

- duration 缺省通过。
- duration `2` 返回 400。
- duration `16` 返回 400。
- duration `3`、`15` 通过。
- Video Edit 显式 duration 返回 400。
- T2V/R2V 接受 `4:3`、`3:4`。
- I2V 传 ratio 返回 400。
- Video Edit 传 ratio 返回 400。
- I2V 0 张或多张首帧返回 400。
- R2V 0 张参考图返回 400。
- R2V 1 张、9 张通过。
- R2V 10 张返回 400。
- Video Edit 0 个 video 返回 400。
- Video Edit 1 个 video 通过。
- Video Edit 2 个 video 返回 400。
- Video Edit 5 张参考图通过。
- Video Edit 6 张参考图返回 400。
- media URL 为空返回 400。
- media URL 非 http/https 返回 400。

### V1 绕过路径

- V1 Video Edit 显式 duration 返回 400。
- V1 R2V `metadata.media` 超过 9 张参考图返回 400。
- V1 media URL 非 http/https 返回 400。
- V1 I2V 多张 `images` 返回 400。
- V1 Video Edit 超过 5 张 `reference_images` 返回 400。
- V1 I2V `images[]` 中含空字符串返回 400。
- V1 Video Edit `reference_images[]` 中含空字符串返回 400。

### 请求体转换

- 未传 resolution 时上游请求体使用 `1080P`。
- `quality`、`sound` 不出现在上游 wire body。
- I2V wire body 只包含 `first_frame`，不包含 ratio。
- Video Edit wire body 不包含 duration。

### 计费

- `SR:1080` 和 `SR:"1080"` 都按 1080P 倍率结算。
- `SR:720` 和 `SR:"720"` 都按 720P 倍率结算。
- 缺失 SR 时回退提交时 resolution。
- Video Edit 使用输入秒数加输出秒数。
- 实际秒数小于预扣秒数时退款。
- 实际秒数大于预扣秒数时补扣。

## 6. 验证命令

```powershell
go test ./relay/channel/task/happyhorse ./relay/channel/happyhorse -count=1
go vet ./relay/channel/task/happyhorse/... ./relay/channel/happyhorse/...
go build ./...
```

## 7. 验收结论

当前修复目标：

- 保证 HappyHorse 请求体符合官方结构化 schema。
- 保证 native 和 V1 两个入口执行同一套参数契约。
- 保证 Video Edit 按官方输入+输出视频秒数计费。
- 保证 `usage.SR` 类型变化不会破坏任务结算。
- 保证非官方参数不继续误导用户。
- 保证测试可覆盖主要边界和绕过路径。
