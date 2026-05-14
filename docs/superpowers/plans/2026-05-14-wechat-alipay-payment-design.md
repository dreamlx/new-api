# 微信支付 / 支付宝原生直连支付通道接入设计

| 字段 | 值 |
|------|-----|
| 日期 | 2026-05-14 |
| 分支 | `feat/wechat-alipay-payment` |
| 状态 | v2（已合并 codex review 反馈） |
| 范围 | 一次性充值（TopUp），不含订阅 |
| 场景 | PC Web（支付宝电脑网站支付 + 微信 Native 扫码） |

---

## 1. 背景与目标

项目当前已通过 `Calcium-Ion/go-epay` 易支付聚合器支持支付宝 / 微信。本次新增**原生官方 SDK 直连**通道，与现有 epay / Stripe / PayPal / Waffo 并存，互不影响。

**直连相比聚合器的好处**
- 费率更低（去除聚合器抽成）
- 资金直达商户银行账户，结算周期短
- 合规可审计（直接拥有完整支付流水与签名链）

**目标用户场景**：枭毅 API 控制台用户，PC 浏览器内充值，调用上游模型。

---

## 2. 架构总览

### 2.1 目录结构

```
controller/
  topup_alipay.go              # 支付宝充值 / 通知 / 同步跳转 / 退款
  topup_wxpay.go               # 微信充值 / 通知 / 退款 / 退款回调
setting/
  payment_alipay.go            # 配置项 + GetAlipayClient()
  payment_wxpay.go             # 配置项 + GetWechatPayClient()
service/
  alipay.go                    # 业务封装（下单/查单/退款）
  wechat_pay.go                # 同上
web/src/pages/Setting/Payment/
  SettingsPaymentGatewayAlipay.jsx
  SettingsPaymentGatewayWxpay.jsx
web/src/components/topup/      # 在 RechargeCard 注入新支付方式按钮
docs/
  ALIPAY_SETUP_GUIDE.md
  WXPAY_SETUP_GUIDE.md
```

### 2.2 新增依赖（`go.mod`）

- `github.com/smartwalle/alipay/v3` — 支付宝 SDK，**社区维护（非官方）**
- `github.com/wechatpay-apiv3/wechatpay-go` — 微信支付**官方**SDK（腾讯维护）

> ⚠️ **SDK 风险隔离**：所有 SDK 调用必须封装在 `service/alipay.go` / `service/wechat_pay.go` 的 interface 后，controller 层不直接 import SDK。这样未来替换为其他 SDK（如官方支付宝 Go SDK 一旦发布，或自实现）时，只动 service 层。
>
> 在 `go.mod` 中**锁定**精确小版本，并在 `service/` 单元测试中覆盖签名/验签/退款分支，作为 SDK 升级的回归保护。

### 2.3 复用现有基础设施（不新建）

- `model.TopUp` 表
- `LockOrder` / `UnlockOrder` 订单互斥锁（`controller/topup.go`）
- `service.GetCallbackAddress()` 回调地址
- `getMinTopup()` / `getPayMoney()` 金额换算（含分组倍率/折扣/QuotaPerUnit）
- `model.IncreaseUserQuota()` 增加用户额度
- `model.RecordLog()` 充值/退款日志
- `model.ManualCompleteTopUp()` 管理员补单（已通用）
- TradeNo 规范：`USR{uid}NO{rand6}{ts}`（与 epay 一致）

---

## 3. 数据流

### 3.1 支付宝充值（PC 网站支付）

```
1) 前端 RechargeCard 选金额 + 勾选 alipay → POST /api/user/alipay/pay
2) 后端 RequestAlipay:
   - 校验金额 >= getMinTopup()，计算 payAmountCents（以分为单位的整数，见 §3.5）
   - 生成 tradeNo = "USR{uid}NO{rand6}{ts}"
   - INSERT model.TopUp (status=pending, payment_method="alipay",
                         expire_time=now+30min)
   - 调 client.TradePagePay 生成支付 URL
   - 返回 { url: "https://openapi.alipay.com/gateway.do?..." }
3) 前端 window.open(url, "_blank") → 用户登录支付宝完成支付
4) 异步通知 POST /api/user/alipay/notify (服务端到服务端):
   - client.GetTradeNotification(req) 验签
   - 完整校验项（见 §3.6）：app_id / seller_id / out_trade_no / total_amount /
     trade_status ∈ {TRADE_SUCCESS, TRADE_FINISHED} / 商户号匹配
   - 用 DB 条件更新做幂等（见 §3.4）：
     UPDATE topups SET status='success', complete_time=NOW(), provider_tx_id=?
     WHERE trade_no=? AND status='pending'  → 影响 1 行才发额度
   - IncreaseUserQuota → RecordLog(LogTypeTopup)
   - 返回 "success"（必须，否则支付宝会重试 8 次）
5) 同步跳转 GET /api/user/alipay/return:
   - 仅做 UI 跳转，不发货：重定向 console/topup?pay=success | fail | pending
```

### 3.2 微信 Native 充值（PC 扫码）

```
1) POST /api/user/wxpay/pay
2) 后端 RequestWxpay:
   - 同上的金额校验、订单创建（payment_method="wxpay", expire_time=now+5min）
   - 调 native.NativeApiService.Prepay，amount.total 传分（整数）
   - 返回 { code_url, trade_no }（code_url 形如 weixin://wxpay/bizpayurl?pr=xxxx）
3) 前端用 qrcode.react 渲染二维码（PaymentConfirmModal 改造）
   - 倒计时 5 分钟（与微信订单 expire 对齐，仅作 UI 提示，不作订单生命周期来源）
   - 每 3s 轮询 GET /api/user/topup/status?trade_no=xxx（见 §6.8 接口归属校验）
4) 异步通知 POST /api/user/wxpay/notify:
   - SDK notification.Handler 完整校验：签名 / 时间戳 / nonce / 平台证书序列号 / AES-GCM 解密
   - 完整校验项（见 §3.6）：appid / mchid / out_trade_no / amount.total /
     trade_state == "SUCCESS"
   - DB 条件更新做幂等（同上）
   - IncreaseUserQuota → RecordLog
   - 返回 200 + { code: "SUCCESS" }（错误时返回 4xx + { code: "FAIL", message: "..." }，让微信重试）
5) 前端轮询命中 success → 关闭 Modal + 刷新余额
```

### 3.3 兜底：状态查询 + 关单

- 用户已支付但回调未到（5 min）→ 前端轮询触发后端调 SDK 主动查询订单 → 命中已支付则补发额度
- 订单超时未支付 → 后端定时任务（详见 §3.7）调用 alipay.TradeClose / wxpay close API，本地置 expired

### 3.4 幂等保证（多副本部署生效）

| 防线 | 适用 |
|------|------|
| **DB 条件更新**（主） | `UPDATE topups SET status='success' WHERE trade_no=? AND status='pending'`，依赖 `trade_no` unique 索引；仅当 `RowsAffected==1` 才发额度 |
| `LockOrder`（辅助） | 进程内防止并发回调走完一次完整路径浪费资源；不作为唯一防线（多副本无效） |
| 第三方平台层 | 支付宝 8 次重试 + 微信 15 次重试都依赖我们返回 success；DB 条件更新即可处理重复 |

> **不依赖**：跨进程 sync.Map / 单机 FOR UPDATE 长事务。

### 3.5 金额精度（统一以分为单位）

| 内部变量 | 类型 | 含义 |
|----------|------|------|
| `payAmountCents` | `int64` | 实付金额（分），用于支付宝/微信 API 与 DB 比较 |
| `model.TopUp.Money` | `float64` | **保留**字段，仅作为元单位的展示值（兼容现有 schema 与 PayPal/Stripe） |

- 所有金额计算用 `shopspring/decimal` 或 int64 cents
- 支付宝传 `fmt.Sprintf("%.2f", float64(cents)/100)` 转字符串
- 微信直接传 `cents`
- 验签金额比对必须用整数：`alipayTotalAmtToCents(s) == payAmountCents`
- TopUp 表新增 `pay_amount_cents int64`、`currency varchar(8)` 字段（仅新通道写入），便于精确对账

### 3.6 回调验签校验项（强制清单）

**支付宝**：
- [x] SDK `GetTradeNotification` 签名验证
- [x] `app_id == AlipayAppId`
- [x] `seller_id == AlipaySellerId`（配置项新增）
- [x] `out_trade_no` 在我方表内存在
- [x] `total_amount` 转 cents 后 `== topup.pay_amount_cents`
- [x] `trade_status ∈ {TRADE_SUCCESS, TRADE_FINISHED}`
- [x] `currency == "CNY"`（沙箱默认 CNY，预留扩展）

**微信**：
- [x] SDK `notification.Handler` 完整签名+时间戳+nonce+证书序列号校验
- [x] AES-GCM 用 APIv3Key 解密 resource
- [x] `appid == WxpayAppId`
- [x] `mchid == WxpayMchId`
- [x] `out_trade_no` 存在
- [x] `amount.total == topup.pay_amount_cents`
- [x] `trade_state == "SUCCESS"`

任一项失败 → 标记订单 `status='anomaly'` + 写 SysError 日志 + 返回 fail，**绝不发额度**。

### 3.7 订单超时关闭（新增定时任务）

- `model.TopUp` 新增字段 `expire_time int64`（创建时设置：支付宝 30min、微信 5min）
- 新增 cron job（5 min 间隔）扫描 `status='pending' AND expire_time < NOW()`
- 命中订单：调 `alipay.TradeClose` / wxpay `close` API → 本地置 `status='expired'`
- 调用失败的订单不阻塞；下一轮重试，超过 24h 仍 pending 写告警

### 3.8 异常路径

| 场景 | 处理 |
|------|------|
| 验签失败 | 返回 fail，记录 SysError，订单置 anomaly |
| 金额不一致 | 标记 anomaly，不发额度，告警 |
| DB 条件更新影响 0 行 | 说明已被处理过（幂等命中），返回 success |
| 网络超时（用户支付成功但回调未到） | 兜底 §3.3：定时任务 / 前端轮询触发主动查单 |
| 微信平台证书下载失败 | 启动健康检查阻塞启动；运行时失败用未过期缓存 + 告警；**不**降级到只解密不验签 |

---

## 4. 配置项

### 4.1 支付宝（`setting/payment_alipay.go`）

```go
var AlipayEnabled    bool      // 总开关
var AlipayAppId      string    // 商户 AppId
var AlipayPrivateKey string    // 商户应用私钥（PEM, PKCS8）
var AlipayPublicKey  string    // 支付宝公钥（验签用）
var AlipayIsSandbox  bool      // 沙箱开关
var AlipayMinTopUp   int       // 最低充值额度
var AlipaySignType   string    // 默认 "RSA2"
```

`GetAlipayClient()`:
- 首次调用基于 config 构造 `alipay.Client`，启用 `AutoVerifySign`
- `IsProduction` 由 `!AlipayIsSandbox` 决定
- 用 `sync.Once` 缓存
- 配置变更时 `ResetAlipayClient()` 重置

### 4.2 微信（`setting/payment_wxpay.go`）

```go
var WxpayEnabled         bool
var WxpayAppId           string  // 公众号/应用 AppID（Native 必填）
var WxpayMchId           string  // 商户号
var WxpayMchCertSerialNo string  // 商户证书序列号
var WxpayMchPrivateKey   string  // 商户 API 私钥（PEM）
var WxpayApiV3Key        string  // APIv3 密钥（解密回调）
var WxpayMinTopUp        int
```

`GetWechatPayClient()`:
- 启用 `option.WithWechatPayAutoAuthCipher`，SDK 自动下载 + 缓存平台证书
- Client 单例 + 重置接口

### 4.3 注册到 GetTopUpInfo

仿照 PayPal 注入逻辑（`controller/topup.go`）：
- `AlipayEnabled && AlipayAppId != "" && AlipayPrivateKey != "" && AlipayPublicKey != ""` 时
  把 `{name:"支付宝", type:"alipay", color:"...", min_topup:"..."}` 加入 `payMethods`
- `WxpayEnabled && WxpayMchId != "" && ...` 时加 `wxpay`

### 4.4 密钥存储与脱敏

**存储一致性**：与现有 `setting.StripeApiSecret` / `PayPalClientSecret` 一致，写 `option` 表。**本期不引入应用层加密**（保持与现有支付通道一致；KMS 接入是项目级的下一步工作）。

**强制脱敏要求**：
- 高敏字段（`AlipayPrivateKey`、`WxpayMchPrivateKey`、`WxpayApiV3Key`、`WxpayMchCertSerialNo`）在前端 admin 页面只回显 `***last4`
- 任何 `common.SysLog` / `logger.Logger.Info` 调用绝对不能打印完整密钥
- `model.UpdateOption` 写入后立即触发 `ResetAlipayClient()` / `ResetWechatPayClient()` 清缓存
- 支付通道相关 GET options 接口必须按白名单返回，**不**返回原始密钥字段

### 4.5 货币与金额语义（CNY/USD 换算）

**前端用户输入**：金额单位由 `operation_setting.GetQuotaDisplayType()` 决定：
- `QuotaDisplayTypeUSD` → 用户输入 USD
- `QuotaDisplayTypeCNY` → 用户输入 CNY
- `QuotaDisplayTypeTokens` → 用户输入 token 数量

**新通道支付实际收 CNY**，换算路径：
```
amount_input(用户输入，以展示单位计)
  ↓ 经 getPayMoney() 计算（含分组倍率/折扣）→ payMoney(USD-equivalent, float64)
  ↓ 乘以 USDExchangeRate（默认 7.3）→ payMoneyCNY(float64 元)
  ↓ Round to cents → payAmountCents(int64 分)
```

**TopUp 表字段含义**：
| 字段 | 含义 | 单位 |
|------|------|------|
| `Amount` | 用户充值的展示单位数量（与 QuotaDisplayType 一致） | USD/CNY/Tokens |
| `Money` | USD-equivalent 金额（沿用现有逻辑） | USD（float64 元） |
| `pay_amount_cents` | **本期新增**，实付 CNY 金额 | 分（int64） |
| `currency` | **本期新增**，固定 "CNY" | varchar(8) |

**额度发放**：依旧按 `Amount`（展示单位）转 quota（沿用现有 `IncreaseUserQuota` 路径）。

**汇率变更**：通过 `setting.USDExchangeRate` 动态读取；订单创建时把当时使用的汇率快照写入 TopUp（可选 `exchange_rate float64`，本期可不加，记录在 callback raw 摘要里）。

### 4.6 强制规范

- **Rule 1**：所有 JSON 操作走 `common.Marshal/Unmarshal`，不直接调 `encoding/json`
- **Rule 2**：跨数据库兼容，TopUp 新增字段用 GORM AutoMigrate + ADD COLUMN（SQLite 限制）
- **Rule 6**：请求 DTO 中可选标量字段（如 `*int`、`*float64`）必须用指针 + omitempty

---

## 5. 退款

### 5.1 入口与权限

```
POST /api/topup/refund
权限: root role only（middleware.RootAuth()）
body: { trade_no: "USR123NO...", reason: "用户申请", confirm_token: "..." }
```

- 必须 root 权限，普通 admin 无权调用
- 必须二次安全验证（confirm_token：当前会话内 5 分钟有效，由 `POST /api/topup/refund/prepare` 颁发）
- 所有退款请求写 `model.Log` 含 admin_id / trade_no / refund_amount_cents / reason
- 本期**仅支持全额退款**；部分退款 v2

### 5.2 退款状态机

`model.TopUp.refund_status` 字段（**与 status 解耦**）：

| 状态 | 含义 |
|------|------|
| `""`（空） | 未发生退款 |
| `refund_pending` | 已调用第三方退款，等待结果 |
| `refund_success` | 退款成功 + 已扣 quota |
| `refund_failed` | 退款失败（第三方拒绝） |
| `refund_anomaly` | 异常（超时未返回 / 状态对账不一致），需人工介入 |

### 5.3 决策矩阵

| 场景 | 处理 |
|------|------|
| 订单非 success 状态 | 拒绝 |
| `refund_status` 已是 `refund_success` 或 `refund_pending` | 拒绝（幂等） |
| 用户当前 `quota >= quota_granted` | 退全额 + 扣 `quota_granted` |
| 用户 `quota < quota_granted` 且 `force=false` | 拒绝（无法证明额度未用） |
| 用户 `quota < quota_granted` 且 `force=true` | 退全额 + 扣到 `max(0, quota - quota_granted)`，差额视为坏账记录 |
| 超过支付平台退款时限（默认 1 年） | 拒绝 |

> ⚠️ **声明**：`quota >= quota_granted` 不能严格证明充值额度未被使用（用户可能还有其他充值/赠送）。本期采用此简化策略，并在 UI 明示"如果用户额度有混合来源，请勿强制退款"。FIFO 归因 v2 处理。

### 5.4 流程（避免长事务持锁）

```
1) 校验权限 + confirm_token
2) DB 条件更新置 refund_pending：
   UPDATE topups SET refund_status='refund_pending', refund_request_time=NOW(),
                     refund_admin_id=?, refund_reason=?
   WHERE trade_no=? AND status='success' AND (refund_status='' OR refund_status='refund_failed')
   → 影响 1 行才继续；否则返回"已被其他流程处理"

3) 调第三方 API（短超时 10s + 重试 1 次）：
   - 支付宝：alipay.TradeRefund(out_trade_no, out_request_no, refund_amount)
   - 微信：refunds.Create(out_trade_no, out_refund_no, refund_amount)

4) 根据响应分支：
   - 成功：DB 事务内 → UPDATE refund_status='refund_success', refund_time=NOW(),
                       refund_trade_no=? + 扣 user.quota
                     + RecordLog(LogTypeRefund)
   - 拒绝（明确失败）：UPDATE refund_status='refund_failed' + 记录 reason
   - 超时/未知：UPDATE refund_status='refund_anomaly' + 告警，等回调或对账兜底
```

### 5.5 退款回调（权威路径之一，非"仅状态校正"）

- **支付宝**：同步返回结果即可信，无独立回调（已与流程一致）
- **微信**：`POST /api/user/wxpay/refund/notify`
  - 完整签名校验 + AES-GCM 解密
  - 按 `out_refund_no` 查 TopUp（新增 `refund_trade_no` 索引）
  - DB 条件更新：`UPDATE ... SET refund_status='refund_success' WHERE refund_trade_no=? AND refund_status IN ('refund_pending', 'refund_anomaly')`
  - 影响 1 行 → 扣 quota + log；影响 0 行 → 已处理过，返回 success
  - 处理 `SUCCESS / ABNORMAL / CLOSED` 三态

### 5.6 model.TopUp 新增字段

```go
// 通用扩展
PayAmountCents int64  `json:"pay_amount_cents"`              // 实付分
Currency       string `json:"currency" gorm:"type:varchar(8)"` // "CNY"
QuotaGranted   int64  `json:"quota_granted"`                  // 充值时增加的 quota
ExpireTime     int64  `json:"expire_time" gorm:"index"`       // 订单超时时间
ProviderTxId   string `json:"provider_tx_id" gorm:"type:varchar(255);index"` // 支付宝/微信交易号
PaidAt         int64  `json:"paid_at"`                        // 第三方支付完成时间
CallbackRaw    string `json:"-" gorm:"type:text"`             // 回调原始 payload 摘要（脱敏）

// 退款相关
RefundStatus      string `json:"refund_status" gorm:"type:varchar(32);index"`
RefundRequestTime int64  `json:"refund_request_time"`
RefundTime        int64  `json:"refund_time"`
RefundReason      string `json:"refund_reason" gorm:"type:text"`
RefundTradeNo     string `json:"refund_trade_no" gorm:"type:varchar(255);index"` // out_refund_no
RefundAdminId     int    `json:"refund_admin_id"`
RefundedQuota     int64  `json:"refunded_quota"`
```

新增常量：`LogTypeRefund`、`TopUpStatusExpired`、`TopUpStatusAnomaly`。

### 5.7 错误处理

| 场景 | 策略 |
|------|------|
| 网络超时 | 5s 后重试 1 次；仍失败 → 置 `refund_anomaly`，等回调/人工 |
| 验签失败 | 立即拒绝，不重试 |
| 第三方限流 | 返回 503 + Retry-After |
| 微信回调 ABNORMAL | 置 `refund_anomaly`，告警 |

### 5.8 审计

所有退款关键步骤必须 `RecordLog`，含：admin id / 退款金额（分）/ 原 tradeNo / 退款 tradeNo / 第三方响应摘要。

---

## 6. 前端集成

### 6.1 充值入口

`web/src/components/topup/RechargeCard.jsx`：
- `enable_alipay_topup=true` → 显示"支付宝"按钮（蓝色）
- `enable_wxpay_topup=true` → 显示"微信支付"按钮（绿色）

### 6.2 支付宝交互

```
点击 → POST /api/user/alipay/pay
拿到 { url } → window.open(url, '_blank')
轮询 GET /api/user/topup/status?trade_no=xxx → 决定显示 success/fail
```

### 6.3 微信交互

```
点击 → POST /api/user/wxpay/pay
拿到 { code_url, trade_no } → 弹出 PaymentConfirmModal
内嵌 qrcode.react 渲染二维码 + 5min 倒计时
每 3s 轮询 GET /api/user/topup/status?trade_no=xxx
```

新增依赖：`qrcode.react`（已普遍使用，bun 安装）

### 6.4 管理后台配置

- `web/src/pages/Setting/Payment/SettingsPaymentGatewayAlipay.jsx`
- `web/src/pages/Setting/Payment/SettingsPaymentGatewayWxpay.jsx`
- 仿 `SettingsPaymentGatewayWaffo.jsx` 风格：表单 + 测试连接按钮 + 保存
- 在 `web/src/components/settings/PaymentSetting.jsx` 注册 2 个新 Tab

### 6.5 i18n

- 所有新增中文走 `useTranslation()`
- `bun run i18n:extract` 同步到 6 种语言（zh-CN 主，其他先 fallback）

### 6.6 沙箱切换（仅支付宝）

- 后台配置面板 `is_sandbox` 开关
- 后端 `GetAlipayClient()` 据此选 production / sandbox gateway
- UI 上"沙箱模式"标签醒目显示

### 6.7 微信平台证书管理（生产关键）

- 启动时调 SDK `downloader.MgrInstance().RegisterDownloaderWithPrivateKey()` 拉取并落本地缓存
- 缓存路径：`./data/wxpay_certs/`（在 `.gitignore` 与 docker volume 范围内）
- 启动健康检查：证书下载失败 → 阻塞启动并打印明确错误
- 运行时拉取失败 → 继续使用未过期缓存 + 写 SysWarn 告警；**绝不**降级到只解密不验签
- 证书每 12h 自动轮换；接近过期（< 7 天）时主动告警

### 6.8 状态查询接口（必须做归属校验）

```
GET /api/user/topup/status?trade_no=xxx
```
- 必须从 JWT 取 user_id，校验 `topup.user_id == jwt.user_id`
- 不归属当前用户 → 返回 404（不暴露订单存在）
- 管理员查询走单独接口 `GET /api/admin/topup/:trade_no`

> 防止 trade_no 泄露后他人探测支付状态。

### 6.9 可观测性

下单 / 通知 / 退款关键步骤打 `common.SysLog`，便于 grep 排障。**绝不**打印密钥/证书原文。

---

## 7. 测试策略

### 7.1 单元测试

- `service/alipay_test.go` / `service/wechat_pay_test.go`：mock SDK，覆盖签名生成、验签、退款分支、错误响应、平台证书过期、平台证书下载失败
- `controller/topup_alipay_test.go` / `controller/topup_wxpay_test.go`：仿 `topup_paypal_test.go`，httptest + sqlmock，覆盖：
  - 重复回调幂等（DB 条件更新影响 0 行的路径）
  - 金额不一致 → anomaly
  - 验签失败 → fail
  - 状态查询接口的归属校验（不归属返 404）
- `model/topup_test.go`：DB 条件更新、退款状态机迁移、超时关单查询

### 7.2 集成测试（手工）

- **支付宝沙箱**：`https://openhome.alipay.com/develop/sandbox` 申请沙箱号 → 跑通完整充值/退款闭环
- **微信支付**：真实商户号 0.01 元小额测试，回调用内网穿透（cpolar/ngrok）调试
- **多副本幂等验证**：本地启动 2 个实例 + Nginx 轮询，模拟同一回调被两副本同时收到，验证只发一次额度

### 7.3 跨数据库兼容性 checklist（Rule 2）

- TopUp 新字段全部走 GORM AutoMigrate + ADD COLUMN（SQLite 限制）
- 字段类型：
  - 长文本（refund_reason / callback_raw）使用 `TEXT`，**不用** varchar
  - 短定长字符串（refund_trade_no / provider_tx_id / currency）使用 `varchar`
  - 索引字段（refund_status / refund_trade_no / provider_tx_id / expire_time）单独验证三库索引创建
- 不依赖 PostgreSQL/MySQL 专属语法
- DB 条件更新 `UPDATE ... WHERE status='pending'` 在三库行为一致（无须 FOR UPDATE）
- 三库分别跑一遍 `topup_alipay_test.go` / `topup_wxpay_test.go`

---

## 8. 上线前 checklist

1. 商户私钥/APIv3Key 在 `option` 表（与现有 Stripe 一致），admin 页面只回显 `***last4`，不入日志
2. 回调 URL 公网可达 + HTTPS
3. 沙箱开关默认关闭
4. 退款入口默认隐藏，需 root role 才显示，且需 confirm_token 二次验证
5. `topups.trade_no` 已 unique；新增 `refund_trade_no` / `provider_tx_id` / `expire_time` / `refund_status` 索引验证三库均创建成功
6. 微信平台证书启动健康检查通过，本地证书缓存目录存在并可写
7. 订单超时关闭定时任务（5 min 间隔）已注册
8. README 同级新增 `docs/ALIPAY_SETUP_GUIDE.md` 与 `docs/WXPAY_SETUP_GUIDE.md`
9. 多副本部署下，同一回调到达 2 个实例只发一次额度（DB 条件更新生效验证）

## 9. 回滚方案

所有改动通过配置开关控制（`AlipayEnabled` / `WxpayEnabled`）。线上若发现问题：
- 后台一键禁用支付通道（不再生成新订单）
- pending 订单由超时关闭任务自动收尾
- 已成功的订单不受影响
- 无需代码回滚

## 10. 实施分解

| # | 任务 | 预估 | 备注 |
|---|------|------|------|
| 1 | TopUp 模型扩展（新字段 + AutoMigrate + 三库验证） | 1d | 含金额整数化字段、退款状态机字段 |
| 2 | 引入 SDK 依赖 + service 层 interface（隔离 SDK） | 1d | 含 mock 实现用于测试 |
| 3 | 配置项 + 密钥脱敏 + Client 缓存重置 | 1d | |
| 4 | 支付宝下单 + notify + return（含完整验签清单） | 1.5d | |
| 5 | 微信下单 + notify（含平台证书管理） | 1.5d | |
| 6 | 通用 GET /api/user/topup/status（含归属校验） | 0.5d | |
| 7 | 退款（状态机 + 全额退款 + 微信退款回调 + confirm_token） | 2d | |
| 8 | 订单超时关闭定时任务 | 0.5d | |
| 9 | 前端配置页（2 个）+ Recharge/Modal 改造 | 1.5d | |
| 10 | i18n + 文档（2 份 setup guide） | 0.5d | |
| 11 | 单元测试 + 三库回归 + 沙箱联调 + 多副本幂等验证 | 3d | |
| **合计** |  | **~13.5d** | 比 v1 多 4.5d，主要是金额整数化迁移 + 退款状态机 + 多副本幂等验证 |

## 11. v2 范围之外（明确延后）

下列项目本期**不做**，记录到后续 backlog：

| 项 | 优先级 | 说明 |
|----|--------|------|
| 应用层密钥加密 / KMS 接入 | 高 | 项目级工作，影响所有支付通道（Stripe/PayPal/新通道） |
| 部分退款 | 中 | 业务方未提需求，全额退款覆盖 90% 场景 |
| FIFO 额度归因（quota 分批扣减） | 中 | 现有 quota 模型不区分来源，需要全局重构 |
| 抽公共 `CompleteTopUpPayment` 函数 | 低 | 现有 PayPal/Stripe 已有重复实现，重构超出本期范围 |
| 财务对账 CSV 导出 | 低 | 已记录字段，导出 UI 后续做 |
| 部署级分布式锁（Redis） | 低 | DB 条件更新已能解决幂等；性能瓶颈出现后再加 |
| 移动端 H5 / JSAPI 场景 | 低 | 与产品形态（PC 控制台）不匹配 |
| 多币种支持 | 低 | 当前只收 CNY，预留 currency 字段 |

---

## 附录 A：保留的项目身份

按 CLAUDE.md Rule 5，本次实现严格保留 nеw-аρi 与 QuаntumΝоuѕ 的所有引用，不修改任何相关品牌、署名、模块路径。

## 附录 B：v2 主要变更（基于 codex review）

| 章节 | v1 → v2 |
|------|---------|
| §3.1/3.2 | 通知流程显式标注 DB 条件更新 + 完整验签校验项；同步 return 不发货 |
| §3.4 | 幂等保证从 LockOrder + FOR UPDATE 改为 DB 条件更新（多副本生效） |
| §3.5 | 新增金额整数化（cents），支付宝/微信通信用整数 |
| §3.6 | 新增完整验签校验项清单（强制） |
| §3.7 | 新增订单超时关闭定时任务 |
| §4.4 | 密钥存储与现有方案对齐（不引入加密），强化脱敏与 Client 缓存重置 |
| §4.5 | 新增 CNY/USD 换算语义说明 + TopUp 字段含义表 |
| §5 | 退款大改：root 权限、confirm_token、状态机（4 态）、避免长事务、回调权威性、坏账处理 |
| §6.7 | 新增微信平台证书管理（启动健康检查 + 缓存策略） |
| §6.8 | 新增状态查询接口的归属校验（防 trade_no 探测） |
| §10 | 工作量从 ~9d 增至 ~13.5d |
| §11 | 新增"v2 范围之外"明确延后项 |
