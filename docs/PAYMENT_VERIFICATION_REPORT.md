# 微信支付 / 支付宝原生直连支付通道 — 验证报告

> 对应实施计划 `docs/superpowers/plans/2026-05-14-wechat-alipay-payment-impl-plan.md` 的 Task 43-46

## 自动化验证（本会话内完成）

### Task 43 — 数据库回归

| 数据库 | 状态 | 备注 |
|---|---|---|
| SQLite (in-memory) | ✅ 通过 | 所有 TopUp / refund state machine 测试基于 glebarez/sqlite，已覆盖。 |
| MySQL (>= 5.7.8) | ⏳ 待手工 | 需运维提供测试实例。所有 SQL 通过 GORM 抽象，未使用 MySQL/PG-only 语法。 |
| PostgreSQL (>= 9.6) | ⏳ 待手工 | 同上。引用了 `commonGroupCol` 等跨库变量。 |

**验证方式：**
```bash
# SQLite
go test ./model ./controller ./service ./common -count=1

# MySQL（运维）
DB_TYPE=mysql go test ./model -run TestTopUp -v

# PostgreSQL（运维）
DB_TYPE=postgres go test ./model -run TestTopUp -v
```

### 测试覆盖统计

| 层 | 通过测试数 | 主要内容 |
|---|---|---|
| `model` | 15 | TopUp 扩展字段、CompleteTopUpByCondition、SetTopUpAnomaly、4 状态退款机 |
| `common` | ~10 | MoneyToCents / CentsToMoneyStr / AlipayAmountToCents 边界 |
| `service` | 9 | CloseExpiredPendingTopUps、ReconcileStaleRefundsPending、微信支付 SDK 封装 |
| `controller` | 70 | RequestAlipay×4、AlipayNotify×5、AlipayReturn×5、RequestWxpay×4、WxpayNotify×6、GetTopUpInfo×4、TopUpStatus×4、ActiveQuery×7、Refund prepare/execute/notify×25 |
| **合计** | **104+** | 全部 PASS |

### 已知预先存在失败（与本工作无关）
- `controller.TestDeleteWisemodelUser` — `main` 分支也存在，git diff 显示测试文件未被本分支改动。
- `service.TestObserveChannelAffinityUsageCacheByRelayFormat_*` — 同上，与 channel affinity cache 重构相关，非支付。
- 多个 `relay/channel/*/adaptor.go:NN:2: unreachable code` `go vet` 警告 — 均位于本分支未触及的 relay 适配器代码。

### 编译验证
```bash
# 后端
go build ./controller/... ./service/... ./model/... ./common/... ./router/... ./setting/...
# → 无错误输出

# 前端
cd web && bun run build
# → built in 53.42s, 仅预先存在的 chunk-size 警告
```

## Task 44 — 多副本幂等手工测试（待运维执行）

### Runbook
1. 用 docker-compose 启动两个实例（同一数据库），分别监听 `:3000` 与 `:3001`。
2. 用 nginx 轮询 `/api/user/alipay/notify` 与 `/api/user/wxpay/notify`。
3. 模拟支付宝异步回调（使用沙箱）；用 `curl` 同时发送 2 次相同 payload 到两个端口：
   ```bash
   curl -X POST http://localhost:3000/api/user/alipay/notify -d "@payload.txt" &
   curl -X POST http://localhost:3001/api/user/alipay/notify -d "@payload.txt"
   ```
4. **预期结果：**
   - 两个实例都返回 `success` HTTP 200。
   - 数据库 `top_ups.status = 'success'`、`paid_at` 与 `provider_tx_id` 已写入。
   - 用户余额仅增加一次（不会双倍发放）。
   - `logs` 表只有一条 `LogTypeTopup` 记录。
5. 重复同样的实验，但其中一次发送在另一次完成后：
   - 第二次仍返回 `success`，不影响数据。

### 保护机制
- **内存级别**：`controller.LockOrder(tradeNo)` 防止单实例内并发。
- **跨实例**：`model.CompleteTopUpByCondition` 以 `WHERE status = 'pending'` 条件更新，仅一行影响；其他实例 `RowsAffected == 0` 直接幂等返回。

## Task 45 — 支付宝沙箱端到端（待运维执行）

### Runbook
1. 申请支付宝沙箱：https://open.alipay.com/develop/sandbox/app
2. 生成 RSA2048 密钥对：
   ```bash
   openssl genrsa -out app_private.pem 2048
   openssl rsa -in app_private.pem -pubout -out app_public.pem
   ```
3. 上传应用公钥到沙箱后台，下载支付宝公钥。
4. 在 admin 配置：
   - 启用 **支付宝支付**
   - **沙箱模式** = on
   - AppId、私钥、公钥按沙箱后台填入
   - Seller ID = 沙箱卖家账号 ID
5. 在 RechargeCard 点击 **支付宝** 充值 1 元 → 跳转沙箱支付页 → 用沙箱买家账号支付 → 异步回调 → 验证额度到账。

### 详细配置见 `docs/ALIPAY_SETUP_GUIDE.md`

## Task 46 — 微信 0.01 元小额端到端（待运维执行）

### Runbook
1. 准备真实微信支付商户号（或服务商商户号）+ 已审核应用。
2. 下载 API 证书 `apiclient_cert.pem` / `apiclient_key.pem`，记录 MchSerialNo。
3. 在商户后台设置 APIv3 密钥（32 位字符串）。
4. 获取微信支付公钥 ID（`PUB_KEY_ID_...`）和微信支付公钥 `pub_key.pem`。
5. 在 admin 配置：
   - 启用 **微信支付**
   - AppId / MchId / MchSerialNo / 微信支付公钥 ID / 微信支付公钥 / APIv3 密钥 / 商户私钥（apiclient_key.pem 完整内容）
6. 充值 0.01 元 → 弹出 QR Modal → 手机扫码支付 → 验证：
   - 前端轮询 `/api/user/topup/status` 返回 `status=success`
   - 数据库 `top_ups` 写入 `provider_tx_id`、`paid_at`
   - 用户余额增加 0.01 元对应额度
6. **退款测试**：作为 root 用户 → 找到该订单 → 提交退款 prepare → 二次确认 → 等待微信退款异步回调 → 验证：
   - `top_ups.refund_status` 从 `refund_pending` 流转到 `refund_success`
   - 用户余额减少相应额度
   - `logs` 表写入 `LogTypeRefund` 记录

### 详细配置见 `docs/WXPAY_SETUP_GUIDE.md`

## 架构总结

```
HTTP 请求
  ↓
router/api-router.go       — 注册路由（auth/public 分组）
  ↓
controller/topup_*.go      — RequestAlipay / AlipayNotify / AlipayReturn
controller/topup_wxpay*.go — RequestWxpay / WxpayNotify
controller/topup_refund*.go — Prepare / Execute / Wxpay refund notify
controller/topup.go        — GetTopUpStatus + 主动查单 + finalizeTopUpSuccess
  ↓
service/alipay.go          — AlipayService 接口（隔离 smartwalle/alipay SDK）
service/wechat_pay.go      — WechatPayService 接口（隔离 wechatpay-go SDK）
service/topup_expiry.go    — 定时关单 + 退款超时巡检
  ↓
setting/payment_alipay.go  — sync.Once 客户端 + ResetAlipayClient
setting/payment_wxpay.go   — 微信支付公钥模式客户端初始化 + GetWechatPayVerifier
  ↓
model/topup.go             — TopUp v2 字段 + 退款状态机
common/money.go            — 金额整数化（避免 float 精度）
common/constants.go        — TopUpStatus / RefundStatus 常量
```

### 幂等保护层次
1. **内存锁** `LockOrder(tradeNo) / UnlockOrder(tradeNo)` — 防单实例并发。
2. **DB 条件更新** `CompleteTopUpByCondition` / `MarkRefundPending` — 跨实例幂等。
3. **状态机守卫** WxpayRefundNotify 仅当 `Status=success AND RefundStatus=refund_pending` 时执行 `CompleteRefund`。
4. **HMAC 退款 token** `confirm_token` 5 分钟 TTL，绑定 admin_id，常量时间比较。

### 金额安全
- 全部以 **分（cents, int64）** 存储于 `top_ups.pay_amount_cents`。
- 异步回调时 `notify.AmountTotal != topUp.PayAmountCents` → `SetTopUpAnomaly` + 不发额度。
- 退款金额取自 `topUp.PayAmountCents`，确定性 `out_request_no = "RFD" + tradeNo`。

### 防止枚举
- `GetTopUpStatus` 对 unknown trade_no 与 foreign trade_no 返回 **字节级相同**的 404 body，杜绝订单号枚举。

---

**报告生成时间：** 2026-05-15
**分支：** `feat/wechat-alipay-payment`
**总提交数：** 48 commits ahead of `main`

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
