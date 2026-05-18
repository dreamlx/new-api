# 微信支付（Native 扫码）配置指南

> 适用于 nеw-аρi（QuаntumΝоuѕ）后台运营人员 / 站点管理员。

## 1. 概述

本指南介绍如何为 nеw-аρi 启用 **微信支付直连通道**。当前实现使用：

- **下单模式**：Native（扫码）——用户在 PC 端浏览器看到二维码，用微信扫码完成支付；
- **API 版本**：APIv3（基于 HTTP + JSON + RSA2048 签名）；
- **验签模式**：微信支付公钥模式，通过 `wechatpay-go` 的 `WithWechatPayPublicKeyAuthCipher` 初始化客户端，不再依赖平台证书自动下载。

## 2. 前置条件

请在开始之前确认以下事项：

1. 已注册并通过审核的 **微信支付商户号 (MchId)**——可在 [pay.weixin.qq.com](https://pay.weixin.qq.com/) 商户平台查看。
2. 一个可用于挂载 Native 支付的 **AppId**（公众号 / 小程序 / 移动应用，任选其一，且已与商户号绑定）。
3. **商户 API 证书**——一对 PEM 文件：
   - `apiclient_cert.pem`（证书本体，用于读取证书序列号 MchSerialNo）
   - `apiclient_key.pem`（商户私钥，用于签名）
4. **APIv3 密钥**——一个 32 位字符串，需要在商户平台后台手动设置。
5. **微信支付公钥 ID 与公钥**——在商户平台获取，通常包含：
   - 公钥 ID：形如 `PUB_KEY_ID_...`
   - 公钥内容：`pub_key.pem`，需带完整 `-----BEGIN PUBLIC KEY-----` / `-----END PUBLIC KEY-----`
6. 公网可达的 **HTTPS 服务器**（微信支付异步通知强制要求 HTTPS）。
7. nеw-аρi 部署完毕，**系统设置 → 通用设置 → 服务器地址** 已正确填写。

## 3. 配置步骤

### 3.1 在微信商户平台获取凭证

1. 登录 [微信支付商户平台](https://pay.weixin.qq.com/)。
2. **账户中心 → 商户信息**：记下 **商户号 (MchId)**。
3. **账户中心 → API 安全 → API 证书**：
   - 下载 **商户 API 证书**（解压后得到 `apiclient_cert.pem` 与 `apiclient_key.pem`）。
   - 在证书管理页面可直接看到 **商户证书序列号 (MchSerialNo)**——一段较长的十六进制字符串。
4. **账户中心 → API 安全 → APIv3 密钥**：
   - 点击 **设置 APIv3 密钥**，输入 32 位字符串（请妥善保存，**离开页面后无法再查看**，遗失只能重置）。
5. **账户中心 → API 安全 → 微信支付公钥**：
   - 获取 **微信支付公钥 ID**（`PUB_KEY_ID_...`）。
   - 下载或复制对应的 **微信支付公钥**（`pub_key.pem`）。
6. **产品中心 → 我的产品 → Native 支付**：确认产品已开通。

### 3.2 在 nеw-аρi 后台填写配置

进入 **管理后台 → 系统设置 → 支付设置 → 微信支付**，按下列字段填入：

| 字段 | 说明 |
| --- | --- |
| **AppId** | 与商户号绑定的公众号/小程序/移动应用 AppId |
| **商户号 MchId** | 微信支付商户号 |
| **商户证书序列号** | API 证书序列号（不是商户号，长度通常 40 位左右） |
| **微信支付公钥 ID** | 商户平台提供的公钥 ID，形如 `PUB_KEY_ID_...` |
| **微信支付公钥** | `pub_key.pem` 的完整 PEM 内容，需带 BEGIN/END 标记 |
| **API v3 密钥** | 32 位字符串，前面手动设置的那个 |
| **商户私钥** | `apiclient_key.pem` 的完整 PEM 内容，需带 BEGIN/END 标记 |
| **异步通知地址（可选）** | 留空时自动使用 `<服务器地址>/api/user/wxpay/notify` |
| **最低充值数量** | 触发微信支付按钮的最低充值数量，例如 1 |
| **启用微信支付** | 总开关，关闭后用户充值页不显示微信扫码按钮 |

> APIv3 密钥、商户私钥与微信支付公钥是 **敏感信息**。后端会对返回值做掩码处理（仅显示 `***xxxx`），刷新页面后无需重新填写，留空即视为沿用已保存的值。

最后点击 **更新微信支付设置** 保存。

## 4. 微信支付公钥模式

当前实现使用微信支付公钥模式。系统启动或首次调用微信支付时，会读取后台保存的：

- 商户号 `WxpayMchId`
- 商户证书序列号 `WxpayMchSerialNo`
- 商户私钥 `WxpayPrivateKey`
- 微信支付公钥 ID `WxpayPublicKeyId`
- 微信支付公钥 `WxpayPublicKey`
- APIv3 密钥 `WxpayApiV3Key`

并通过 `WithWechatPayPublicKeyAuthCipher` 创建 SDK 客户端。支付通知和退款通知验签由 `WxpayPublicKeyId + WxpayPublicKey` 完成，通知内容解密仍使用 APIv3 密钥。

> 当前版本不再使用 `WithWechatPayAutoAuthCipher`，也不会从 `/v3/certificates` 自动拉取平台证书。请不要按旧文档排查“平台证书下载失败”，应优先检查公钥 ID、公钥内容、商户私钥和 APIv3 密钥是否匹配当前商户号。

## 5. 小额测试 (0.01 元)

> 微信支付要求实际扣款，**沙箱被官方下线**。最稳妥的真实测试方式是直接发起小额订单。

1. 在 nеw-аρi 后台短暂将 **充值单价** 调整为允许 0.01 元充值的档位（例如：1 元 = 1 额度，发起 1 单位的充值）。
2. 用户充值页选择 **微信支付** → 弹出扫码 Modal。
3. 用手机微信扫码 → 完成 0.01 元支付。
4. 验证：
   - 扫码弹窗几秒内显示 **支付成功**，并自动关闭；
   - 用户额度同步到账；
   - 后端日志 `wxpay notify` 收到对应通知并验签通过。
5. 测试完毕后恢复正式充值单价。

## 6. 回调地址

微信支付直连通道涉及 **两个** 异步通知 URL，**两者都需在商户平台 → 产品中心 → 开发配置中登记**：

| 用途 | URL 模板 |
| --- | --- |
| 支付异步通知 | `<服务器地址>/api/user/wxpay/notify` |
| 退款异步通知 | `<服务器地址>/api/user/wxpay/refund/notify` |

其中 `<服务器地址>` = **系统设置 → 通用设置 → 服务器地址**（自动去除末尾斜杠）。

支付通知是订单成功的权威来源；退款通知是退款最终状态的权威来源。两者都必须能从公网到达。

## 7. 常见问题

### 7.1 验签失败 / 公钥不匹配

- 检查 **微信支付公钥 ID** 是否与 `pub_key.pem` 对应，不能填商户证书序列号；
- 检查 **微信支付公钥** 是否为商户平台提供的完整 PEM 内容，包含 BEGIN/END 行；
- 检查商户平台是否更换过微信支付公钥，更换后必须同步更新 nеw-аρi 配置。

### 7.2 解密失败 / APIv3 密钥不正确

- 检查 nеw-аρi 中填入的 **API v3 密钥** 是否与商户平台设置的完全一致（32 位，区分大小写）；
- 若曾经在商户平台 **重置** 过 APIv3 密钥，必须同步更新 nеw-аρi 配置，否则所有异步通知都将解密失败；
- 重新设置 APIv3 密钥后，建议保存配置并重启服务，确保 SDK 重新加载。

### 7.3 客户端初始化失败

- 检查商户私钥是否完整、商户证书序列号是否与私钥匹配；
- 检查 `WxpayMchId`、`WxpayMchSerialNo`、`WxpayPrivateKey`、`WxpayPublicKeyId`、`WxpayPublicKey` 是否都已配置；
- 检查服务器到 `api.mch.weixin.qq.com` 的网络连通性（443 端口出站）。

### 7.4 退款通知未到达

退款是异步流程，提交退款请求后 `refund_status` 进入 `refund_pending`，必须等微信回调才能标记 `refund_success`。如果通知长时间未到：

- 检查 **退款异步通知 URL** 是否已在商户平台登记并可公网访问；
- 系统内置 cron 每小时扫描，若退款超过 **24 小时仍处于 `refund_pending`** 状态，会自动标记为 `refund_anomaly`，提示运维介入；
- 出现 `refund_anomaly` 时，可在商户平台手动查询退款状态后，决定是否在 nеw-аρi 后台手动调整（具体操作流程请遵循内部 SOP）。

### 7.5 扫码后提示 "商户号与 AppId 未关联"

- 在商户平台 **产品中心 → AppId 账号管理** 中，将当前 AppId 绑定到商户号；
- 等待几分钟生效后重试。

## 8. 退款流程

当前后端保留 root 专用退款执行接口，但前端充值历史页暂未接入退款按钮；常规用户售后建议按 `docs/REFUND_PRIVATE_DOMAIN_HANDLING.md` 引导到私域人工处理。

如需进行测试或应急手动退款，可由 root 调用接口：

1. `POST /api/topup/refund/prepare` 获取 5 分钟有效的 `confirm_token`；
2. `POST /api/topup/refund` 提交 `trade_no`、`confirm_token` 和退款原因；
3. 系统调用微信支付 `v3/refund/domestic/refunds` 接口：
   - 微信返回受理成功（HTTP 200 / 202）后，订单 `refund_status` 变为 `refund_pending`，**额度先不扣减**；
   - 微信侧实际退款完成后，向 **退款异步通知 URL** 发起回调；
   - nеw-аρi 验签通过后，将 `refund_status` 更新为 `refund_success` 并扣减用户额度。

> 与支付宝的同步退款不同，**微信退款的成功状态必须由异步通知确认**。请在 SOP 中明确告知运营人员：提交退款后看到 `refund_pending` 是正常状态，需等待回调；如超过 24 小时仍未确认，再按 7.4 的流程介入。

## 9. 进阶参考

如果你需要深入了解或排查实现细节，相关源码位置（仅供参考，**请勿在生产环境直接修改**）：

- 后端订单创建 / 异步回调 / 退款回调：`controller/topup_wxpay.go`
- 退款执行流程：`controller/topup_refund_execute.go`
- 配置项注册及默认值：`setting/payment_wxpay.go`
- 微信支付 SDK 封装：`service/wechat_pay.go`
- 退款超时巡检 cron：`service/refund_anomaly.go`
