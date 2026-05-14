# 微信支付 / 支付宝原生直连支付通道接入设计

| 字段 | 值 |
|------|-----|
| 日期 | 2026-05-14 |
| 分支 | `feat/wechat-alipay-payment` |
| 状态 | Draft（待 codex review） |
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

- `github.com/smartwalle/alipay/v3` — 支付宝 SDK，社区事实标准
- `github.com/wechatpay-apiv3/wechatpay-go` — 微信支付官方 SDK（腾讯维护）

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
   - 校验金额 >= getMinTopup()，计算 payMoney（含分组倍率/折扣）
   - 生成 tradeNo = "USR{uid}NO{rand6}{ts}"
   - INSERT model.TopUp (status=pending, payment_method="alipay")
   - 调 client.TradePagePay 生成支付 URL
   - 返回 { url: "https://openapi.alipay.com/gateway.do?..." }
3) 前端 window.open(url, "_blank") → 用户登录支付宝完成支付
4) 异步通知 POST /api/user/alipay/notify (服务端到服务端):
   - client.GetTradeNotification(req) 验签
   - 校验 trade_status=TRADE_SUCCESS、out_trade_no、total_amount
   - LockOrder → 检查 status=pending → 更新 success
   - IncreaseUserQuota → RecordLog
   - 返回 "success"（必须，否则支付宝会重试 8 次）
5) 同步跳转 GET /api/user/alipay/return:
   - 验签后 → 重定向 console/topup?pay=success | fail | pending
```

### 3.2 微信 Native 充值（PC 扫码）

```
1) POST /api/user/wxpay/pay
2) 后端 RequestWxpay:
   - 同上的金额校验、订单创建（payment_method="wxpay"）
   - 调 native.NativeApiService.Prepay 生成 code_url
     （形如 weixin://wxpay/bizpayurl?pr=xxxx）
   - 返回 { code_url, trade_no }
3) 前端用 qrcode.react 渲染二维码（PaymentConfirmModal 改造）
   - 倒计时 5 分钟（与微信订单 expire 对齐）
   - 每 3s 轮询 GET /api/user/topup/status?trade_no=xxx
4) 异步通知 POST /api/user/wxpay/notify:
   - APIv3 通知体是 AES-GCM 加密 resource，需用 APIv3Key 解密
   - 校验 transaction.trade_state=SUCCESS、out_trade_no、amount.total
   - LockOrder → 完成订单 → 增加 quota
   - 返回 200 + { code: "SUCCESS" }
5) 前端轮询命中 success → 关闭 Modal + 刷新余额
```

### 3.3 幂等保证

- `LockOrder(tradeNo)` 防并发回调
- 事务内 `FOR UPDATE` 取订单 + `status==pending` 检查防重复发额度
- 支付宝/微信对同一订单的多次回调，第二次起直接返回 success

### 3.4 异常路径

| 场景 | 处理 |
|------|------|
| 验签失败 | 返回 fail，记录 SysError 日志 |
| 金额不一致 | 标记订单异常（新增 status=anomaly），不发额度，告警 |
| 订单已 success | 直接返回 success（幂等） |
| 网络超时（用户支付成功但回调未到） | 5min 后由前端轮询 + 主动调 client.QueryOrder 兜底 |

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

### 4.4 密钥存储

- 高敏字段（私钥 / APIv3Key / 证书）写 `option` 表（与 `setting.StripeApiSecret` 一致路径）
- 通过 `model.UpdateOption` 写入；**不**落 `.env` 文件
- 后续可考虑接入 KMS（暂不在本次范围）

### 4.5 强制规范

- **Rule 1**：所有 JSON 操作走 `common.Marshal/Unmarshal`，不直接调 `encoding/json`
- **Rule 2**：跨数据库兼容，TopUp 新增字段用 GORM AutoMigrate + ADD COLUMN（SQLite 限制）
- **Rule 6**：请求 DTO 中可选标量字段（如 `*int`、`*float64`）必须用指针 + omitempty

---

## 5. 退款

### 5.1 入口

```
POST /api/topup/refund        # 管理员权限
body: { trade_no: "USR123NO...", reason: "用户申请" }
```

### 5.2 决策矩阵

| 场景 | 处理 |
|------|------|
| 订单非 success 状态 | 拒绝 |
| 用户当前 quota >= 充值时增加的 quota | 退全额 + 扣额度 |
| 用户当前 quota < 已增 quota（已使用） | 默认拒绝；可选 `force=true` 二次确认（quota 扣到 0 或负数） |
| 超过支付平台退款时限（默认 1 年） | 拒绝 |

### 5.3 流程

```
1) LockOrder(tradeNo)
2) 事务内：
   - SELECT TopUp FOR UPDATE，校验 status=success
   - 计算应扣 quota，校验 user.quota
   - 调 alipay.TradeRefund / wxpay refunds.Create
   - 第三方成功 → UPDATE TopUp.status="refunded" + 扣 user.quota + RecordLog(LogTypeRefund)
   - 第三方失败 → 回滚事务，返回 error
```

### 5.4 退款回调

- **支付宝**：同步返回结果即可信，无独立回调
- **微信**：APIv3 异步通知 `POST /api/user/wxpay/refund/notify`，仅作状态校正（防主动状态丢失）

### 5.5 model.TopUp 新增字段

```go
RefundTime    int64  `json:"refund_time"`
RefundReason  string `json:"refund_reason" gorm:"type:varchar(255)"`
RefundTradeNo string `json:"refund_trade_no" gorm:"type:varchar(255);index"`
```

新增 `LogTypeRefund` 常量。

### 5.6 错误处理

| 场景 | 策略 |
|------|------|
| 网络超时 | 5s 后重试 1 次，仍失败 → 人工介入 |
| 验签失败 | 立即拒绝，不重试 |
| 第三方限流 | 返回 503 + Retry-After |

### 5.7 审计

所有退款操作必须 `RecordLog`，含：操作 admin id、退款金额、原 tradeNo、退款 tradeNo。

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

### 6.7 可观测性

下单 / 通知 / 退款关键步骤打 `common.SysLog`，便于 grep 排障。

---

## 7. 测试策略

### 7.1 单元测试

- `service/alipay_test.go` / `service/wechat_pay_test.go`：mock SDK，覆盖签名生成、验签、退款分支
- `controller/topup_alipay_test.go` / `controller/topup_wxpay_test.go`：仿 `topup_paypal_test.go`，httptest + sqlmock
- `model/topup_test.go`：新增退款相关函数测试

### 7.2 集成测试（手工）

- **支付宝沙箱**：`https://openhome.alipay.com/develop/sandbox` 申请沙箱号 → 跑通完整充值/退款闭环
- **微信支付**：真实商户号 0.01 元小额测试，回调用内网穿透（cpolar/ngrok）调试

### 7.3 跨数据库兼容性 checklist

- TopUp 新字段（refund_time / refund_reason / refund_trade_no）走 GORM AutoMigrate
- 不使用 ALTER COLUMN，仅 ADD COLUMN（SQLite 限制）
- 不依赖 PostgreSQL/MySQL 专属语法

---

## 8. 上线前 checklist

1. 商户证书在 `option` 表加密存储
2. 回调 URL 公网可达 + HTTPS
3. 沙箱开关默认关闭
4. 退款入口默认隐藏，需 root role 才显示
5. `topups.trade_no` 已 unique，无需新增索引
6. README 同级新增 `docs/ALIPAY_SETUP_GUIDE.md` 与 `docs/WXPAY_SETUP_GUIDE.md`

---

## 9. 回滚方案

所有改动通过配置开关控制（`AlipayEnabled` / `WxpayEnabled`）。线上若发现问题，admin 后台一键禁用，无需代码回滚。

---

## 10. 实施分解（后续 plan 用）

| # | 任务 | 预估 |
|---|------|------|
| 1 | 引入 SDK 依赖 + 配置项骨架 | 0.5d |
| 2 | 支付宝下单 + notify + return | 1d |
| 3 | 微信下单 + notify | 1d |
| 4 | 通用 GET /api/user/topup/status 查询接口 | 0.5d |
| 5 | 退款（含微信退款回调） | 1.5d |
| 6 | 前端配置页（2 个） | 1d |
| 7 | 前端 RechargeCard / Modal 改造 | 1d |
| 8 | i18n + 文档（2 份 setup guide） | 0.5d |
| 9 | 单元测试 + 沙箱联调 | 2d |
| **合计** |  | **~9d** |

---

## 附录 A：保留的项目身份

按 CLAUDE.md Rule 5，本次实现严格保留 nеw-аρi 与 QuаntumΝоuѕ 的所有引用，不修改任何相关品牌、署名、模块路径。
