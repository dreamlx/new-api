# Handoff — upstream/main -> main -> lh-main merge

## 当前状态

- 主 checkout 仍在 `lh-main`，没有改动已跟踪文件。
- 主 checkout 有两个原有 untracked 文件：
  - `one-api.db.pre-lhmain-20260619`
  - `web/package-lock.json`
- 第一段 merge 已在隔离 worktree 完成并提交：
  - branch: `merge/upstream-main-20260725`
  - path: `.worktrees/upstream-main-20260725`
  - commit: `74d758e92294487fdf46b2915ccf2413f43ed8db`
  - commit message: `Merge upstream/main into main`
- 第二段 merge 已在隔离 worktree 开始，但未完成、未提交：
  - branch: `merge/lh-main-upstream-20260725`
  - path: `.worktrees/lh-main-upstream-20260725`
  - base: `lh-main` at `c740e36f2`
  - merged input: `merge/upstream-main-20260725` at `74d758e92`

## 已完成：第一段 `upstream/main -> main`

执行路径：

```bash
git worktree add -b merge/upstream-main-20260725 .worktrees/upstream-main-20260725 origin/main
cd .worktrees/upstream-main-20260725
git merge upstream/main
```

处理策略：

- `web/` 全部采用 upstream 版本。
- 用户/session/dashboard auth 倾向 upstream。
- billing/quota 与 relay/channel 手工合成。
- 保留本地仍需要的 WiseModel/resource-package 相关兼容点。

第一段手工修正过的关键点：

- `relay/helper/price.go`
  - 保留 upstream quota saturation / strict conversion / `OtherRatios`。
  - 保留 paid model `preConsumedQuota` floor-to-1，防止正成本请求 round 到 0 后绕过 resource package gate。
  - 修过一次合成回归：fixed-price path 的 floor-to-1 原本会被 `usePrice` 分支重新计算覆盖，已移动到最终 `PriceData` 计算后。
  - 补了 `tiny-fixed-image-price` regression test。
- `controller/user.go`
  - 修复 merge 漏掉的 `downstream := c.Query("downstream")`。
- `relay/helper/stream_scanner.go`
  - 第一段按 upstream reset semantics：`info.StreamStatus = relaycommon.NewStreamStatus()`。
  - 注意：这会在第二段撞上 LH 的 preserve semantics test，见下方“未决点”。
- `web/`
  - 已确认 worktree/index 与 `upstream/main:web` 一致。

第一段验证已通过：

```bash
go test ./controller ./model ./relay/... ./service/... ./pkg/billingexpr/...
cd web && bun install --frozen-lockfile && bun run build
```

## 进行中：第二段 `main -> lh-main`

已执行：

```bash
git worktree add -b merge/lh-main-upstream-20260725 .worktrees/lh-main-upstream-20260725 lh-main
cd .worktrees/lh-main-upstream-20260725
git merge merge/upstream-main-20260725
```

硬冲突只有两个文件：

- `model/user.go`
- `relay/channel/api_request.go`

当前已经手工编辑过这两个文件，但还没有 `git add`，所以 index 仍处于 unresolved merge 状态。下一步应先检查内容、`gofmt`，再 `git add`。

### `model/user.go` 当前取舍

已手工合成方向：

- 保留 upstream：
  - `AuthVersion`
  - `AdminPermissions`
  - `AccessToken json:"-"`
- 保留 LH/local：
  - `Username validate:"max=64"`
  - `Quota gorm:"type:bigint;default:0"`
  - external user fields：`Phone`, `WechatOpenId`, `WechatUnionId`, `AlipayUserId`, `ExternalUserId`, `LoginType`, `IsExternal`, `ExternalData`, `WisemodelKey`

原因：

- LH platform shadow user uses very large quota (`PlatformQuotaForNewUser = 499999500000`), so `users.quota` still needs BIGINT.
- upstream dashboard/session auth needs `AuthVersion` / session invalidation fields.
- `AccessToken` should remain hidden from normal JSON serialization.

### `relay/channel/api_request.go` 当前取舍

已手工合成方向：

- 保留 upstream sanitized logging：
  - `common.SanitizeURLForLog(fullRequestURL)`
- 保留 LH replayable request body fix：
  - `makeReplayableBody(requestBody, info)` before `http.NewRequest`
  - applies to `DoApiRequest` and `DoFormRequest`
- 保留 upstream `applyUpstreamContentLength(req, info)`。

原因：

- LH production reliability requires HTTP/2 GOAWAY / PROTOCOL_ERROR retry support for small known-size request bodies.
- upstream sanitized URL logging should not regress.

## 关键未决点

### 1. `StreamScanner` preserve vs reset

当前第二段 worktree 状态：

- `relay/helper/stream_scanner.go` is upstream reset semantics:

```go
info.StreamStatus = relaycommon.NewStreamStatus()
```

- `relay/helper/stream_scanner_test.go` from `lh-main` expects preserve semantics:

```go
TestStreamScannerHandler_StreamStatus_PreservesPreInitialized
```

这会导致 `go test ./relay/helper` 失败，除非做出取舍。

待决选项：

1. 采用 upstream reset：修改/删除 LH preserve test。
2. 保留 LH preserve：把实现改回：

```go
if info.StreamStatus == nil {
    info.StreamStatus = relaycommon.NewStreamStatus()
}
```

之前讨论过这是“需讨论点”，不要静默拍板。

### 2. local-only adapters 是否长期保留

第一段 merge 后 local-only adapters 仍存在并能通过当前后端测试，包括：

- `relay/channel/ospreyai/*`
- `relay/channel/task/happyhorse/*`
- `relay/channel/task/seedance/*`
- `relay/channel/task/taskcommon/volcano/*`

如果 LH production 不再使用，可后续单独清理；本次未删除。

### 3. Gemini clean function parameters test

`lh-main` 的旧测试：

- `relay/channel/gemini/clean_function_parameters_test.go`

在 upstream 新架构中实现迁移到：

- `service/relayconvert/internal/shared/gemini/schema.go`

当前第二段 merge 显示旧测试会被删除。若要保留 canary，应迁移测试到新位置，而不是原路径硬保留。

## LH 必须守住的语义清单

第二段继续处理时，请优先检查这些合同没有丢：

- External user sync:
  - `POST /api/user/external/sync`
  - response includes `user_id`, `external_user_id`, `is_new_user`
- V2 platform token authorize:
  - `POST /api/v2/external/tokens/authorize`
  - accepts LH-generated `sk-...`
  - creates new-api token with `UnlimitedQuota=true`, `ExpiredTime=-1`
  - returns `token_id`, `status=authorized|exists`
- V2 revoke:
  - `DELETE /api/v2/external/tokens/:id`
  - disable/revoke, not hard delete
- Disabled token relay auth:
  - invalid/unknown key => 401
  - disabled/revoked key => 403 + `api_key_revoked`
- Pull-mode logs:
  - `GET /api/v2/external/logs`
  - `after_id` means `logs.id > after_id`
  - with `after_id`, order must be `id ASC`
  - fields: `log_id`, `token_id`, `request_id`, `channel_id`, `channel_name`, `cache_tokens`, `model_name`, `prompt_tokens`, `completion_tokens`, `created_at`
- OpenAI-compatible reasoning usage normalization:
  - if `total_tokens > prompt_tokens + completion_tokens`, bill output as `total_tokens - prompt_tokens`
- Replayable body:
  - keep `makeReplayableBody` behavior for small known-size bodies.
- `/v1/models`:
  - remains OpenAI-compatible and Bearer-auth compatible.

## Suggested next commands

From repo root:

```bash
cd .worktrees/lh-main-upstream-20260725

# Inspect unresolved index
git ls-files -u | cut -f2 | sort -u
git diff -- model/user.go relay/channel/api_request.go

# After deciding StreamScanner semantics and finishing edits
gofmt -w model/user.go relay/channel/api_request.go relay/helper/stream_scanner.go relay/helper/stream_scanner_test.go
git add model/user.go relay/channel/api_request.go relay/helper/stream_scanner.go relay/helper/stream_scanner_test.go

# Focused validation
go test ./relay/channel ./relay/helper ./relay/channel/openai ./controller ./model -run 'Replay|StreamScanner|Usage|Reasoning|External|V2|Token|User|Wisemodel'

# Broader backend validation used for first段
go test ./controller ./model ./relay/... ./service/... ./pkg/billingexpr/...

# Frontend is upstream; build if needed
cd web && bun install --frozen-lockfile && bun run build
```

## Do not forget

- 第一段 commit `74d758e92` 只存在本地 branch `merge/upstream-main-20260725`，还没有合入主 checkout `main`。
- 第二段 branch `merge/lh-main-upstream-20260725` 还没有 commit。
- 主 checkout 的 `lh-main` 没有被修改。
- 不要把 `gen-1`/task board 的 stale unreconciled 状态当成实际未处理；第一段结果已经人工复查、补修、验证并提交。
