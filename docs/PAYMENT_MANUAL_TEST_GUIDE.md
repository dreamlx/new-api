# 支付宝 / 微信支付 — 手动真实测试指南

> 面向运维 / 测试人员：把刚刚开发的支付宝原生直连 + 微信 Native 支付通道
> 在**真实生产凭证**下跑通一遍。读者已经知道项目背景，本文不再赘述。

---

## 阅读约定

- 所有 shell 命令默认 **bash**（Windows 用 git-bash / WSL）。
- 所有 `<占位符>` 都需要替换，不要原样照抄。
- "✅ 已有"=你截图里能看到；"❓ 待确认"=可能已有但需核对；"❌ 缺失"=必须去拿。
- **金额一律先用 1 元（支付宝）/ 0.01 元（微信）测试**，绝对不要直接打大额。

---

## 第 0 章 · 凭证清单与缺口

### 0.1 你目前手上的文件

```
支付宝（4 个 txt）
  20260408-支付宝企业配置-基本信息-杭州桐屹.txt
  20260408-支付宝企业配置-alipayPublicKey-杭州桐屹.txt
  20260408-支付宝企业配置-应用公钥RSA2048-杭州桐屹.txt
  20260408-支付宝企业配置-应用私钥RSA2048-敏感数据-杭州桐屹.txt

微信支付（一个证书包 + 一个 API 公钥）
  杭州桐屹科技-微信支付-API公钥-pub_key.pem
  杭州桐屹-微信商户API证书-1738223441_20260316_cert.zip
    └─ apiclient_cert.p12
    └─ apiclient_cert.pem
    └─ apiclient_key.pem
```

### 0.2 已有信息 ✅

| # | 项 | 来源 | 备注 |
|---|---|---|---|
| 1 | 支付宝 AppId | `基本信息-杭州桐屹.txt` | 16 位数字 |
| 2 | 支付宝应用私钥 | `应用私钥RSA2048-敏感数据...txt` | 给本系统 |
| 3 | 支付宝公钥 | `alipayPublicKey-杭州桐屹.txt` | 给本系统 |
| 4 | 应用公钥 | `应用公钥RSA2048-杭州桐屹.txt` | **给开放平台**，不是本系统 |
| 5 | 微信商户号 MchId | 证书包文件名前缀 `1738223441` | |
| 6 | 微信商户私钥 | `apiclient_key.pem` | 给本系统 |
| 7 | 微信商户 API 证书 | `apiclient_cert.pem` | 用 openssl 提取序列号 |
| 8 | 微信 API 公钥 | `pub_key.pem` | ⚠️ 见 0.4 风险章节 |

### 0.3 待确认 ❓ 与缺失 ❌

| # | 项 | 类型 | 怎么获取 |
|---|---|---|---|
| A | 支付宝 Seller ID (PID) | ❓ 可选但推荐 | 开放平台 → 账户中心 → 账户信息 → "合作伙伴 ID（PID）" |
| B | 支付宝应用是否已开通"电脑网站支付" | ❓ 必查 | 开放平台 → 我的应用 → 应用详情 → 能力列表 |
| C | 微信关联 AppId | ❌ 必须 | 微信公众平台 / 开放平台 → 公众号或开放平台应用详情 → AppID（关联到这个 MchId） |
| D | 微信 APIv3 密钥（32 位） | ❌ 必须 | 商户平台 → 账户中心 → API 安全 → APIv3 密钥（如果之前没设过，需要新建） |
| E | 微信商户证书序列号 MchSerialNo | ❓ 可从文件提取 | 见 1.2 节用 openssl 提取 |
| F | 公网 HTTPS 域名 | ❌ 必须 | 必须 HTTPS，HTTP 微信和支付宝都不收 |
| G | 系统 ServerAddress 已配置 | ❌ 必须 | admin → 系统设置 → 通用 → 服务器地址 |

### 0.4 ⚠️ 已知风险：微信"平台证书 vs API 公钥"

你的目录里有 `pub_key.pem`，这是**微信 2024 末改版后**给新接入商户的**回调验签公钥**，用来替代旧的"平台证书自动下载"机制。

**当前实现走的是旧路径**（`WithWechatPayAutoAuthCipher`，会去 `https://api.mch.weixin.qq.com/v3/certificates` 拉平台证书）。两种可能：

1. **过渡期**：你的商户号仍能下载平台证书 → 当前实现可工作，正常测试。
2. **纯新接入**：商户号只支持 API 公钥模式 → `NativePrepay` 第一次调用会因 SDK 拿不到平台证书报 `decrypt failed` 或 `certificate visitor not found`。

**应对策略：先按现状跑一次小额。** 如果第 6 章（微信下单）报上述错，把日志发出来，需要补一个 `WithWechatPayPublicKeyAuthCipher` 模式的 PR（约 2 小时工作量）。

---

## 第 1 章 · 凭证预处理（在本地完成）

### 1.1 校验 4 个支付宝文件

```bash
cd <文件存放目录>

# 看 AppId（应该是纯数字 16 位）
cat "20260408-支付宝企业配置-基本信息-杭州桐屹.txt"

# 校验私钥格式（必须是 PKCS8 PEM；如果是 PKCS1 需要转换）
head -1 "20260408-支付宝企业配置-应用私钥RSA2048-敏感数据，请妥善保管-杭州桐屹.txt"
# 期望输出：-----BEGIN RSA PRIVATE KEY-----  或  -----BEGIN PRIVATE KEY-----
# 如果只是一长串 base64 没有 BEGIN/END 包裹，需要自己包裹再用
```

**如果私钥是裸 base64（没有 BEGIN/END 头）**，自己加：

```bash
{ echo '-----BEGIN RSA PRIVATE KEY-----'; \
  fold -w 64 "20260408-支付宝企业配置-应用私钥RSA2048-敏感数据，请妥善保管-杭州桐屹.txt"; \
  echo '-----END RSA PRIVATE KEY-----'; } > alipay_app_private_key.pem
```

填入 admin 时**复制整个 PEM 文件内容**（包含 BEGIN/END 行）。

### 1.2 提取微信商户证书序列号 MchSerialNo

```bash
unzip "杭州桐屹-微信商户API证书-1738223441_20260316_cert.zip" -d wxpay_cert
cd wxpay_cert

# 提取序列号
openssl x509 -in apiclient_cert.pem -noout -serial
# 输出形如：serial=4F2B8A1C9D6E...
# 去掉 "serial=" 前缀，剩下的 40 个十六进制字符就是 MchSerialNo
# 一并核对证书是否过期：
openssl x509 -in apiclient_cert.pem -noout -dates
# notBefore=Mar 16 11:23:00 2026 GMT
# notAfter=Mar 16 11:23:00 2031 GMT  ← 必须在当前日期之后
```

把序列号大写形式存到记事本里，等会儿填 admin。

### 1.3 校验微信商户私钥

```bash
openssl rsa -in apiclient_key.pem -check -noout
# 期望输出：RSA key ok
```

如果报错，私钥损坏，重新从微信商户后台下载证书包。

---

## 第 2 章 · 系统侧准备

### 2.1 备份数据库

**生产数据库务必先打快照。** SQLite 直接 cp：

```bash
cp data/one-api.db data/one-api.db.before-payment-test
```

MySQL：

```bash
mysqldump -u root -p new_api > new_api.before-payment-test.sql
```

PostgreSQL：

```bash
pg_dump -U postgres new_api > new_api.before-payment-test.sql
```

### 2.2 准备测试账户

- 一个**普通用户**（不是 root）— 用来发起充值。
- 一个 **root 用户** — 用来测退款（不能用普通用户测退款）。
- 普通用户的当前余额记录下来，方便对账：

```sql
SELECT id, username, quota FROM users WHERE username='<普通用户名>';
-- 记下 quota，加到一个文本里
```

### 2.3 准备日志监控终端

后台启动一个跟随日志的终端：

```bash
# Linux/macOS
tail -f logs/log.txt | grep -iE 'alipay|wxpay|wechat|refund|topup'

# 或者直接看 stdout
docker logs -f new-api 2>&1 | grep -iE 'alipay|wxpay|wechat|refund|topup'
```

### 2.4 服务器地址确认

admin → 系统设置 → 通用 → "服务器地址"必须是公网 HTTPS，例如 `https://api.example.com`。

```bash
# 自检：从外网能不能访问
curl -I https://<你的域名>/api/status
# 期望：HTTP/2 200
```

如果是 200，HTTPS 证书有效，可以继续。

---

## 第 3 章 · admin 配置（先支付宝）

### 3.1 进入支付设置

admin → 系统设置 → 支付设置 → 找到"支付宝"面板。

### 3.2 填表

| 字段 | 值 |
|---|---|
| AppId | 0.2 #1 那个 16 位数字 |
| 应用私钥 | 0.2 #2 整个 PEM（含 BEGIN/END） |
| 支付宝公钥 | 0.2 #3 整个 PEM |
| Seller ID（可选） | 0.3 A 那个 PID，**强烈建议填**，多一层校验 |
| 沙箱模式 | **关闭** |
| 最低充值数量 | 1 |
| 启用支付宝支付 | **开关打开** |

点"更新支付宝设置"，期望 toast `更新成功`。

### 3.3 后端验证配置生效

```bash
# 看进程内的设置是否真的更新了（最快办法：调一次需要这些值的接口）
curl -s https://<你的域名>/api/user/topup_info \
  -H 'Cookie: <你的登录 cookie，浏览器 F12 复制>' | jq
# 期望返回里有：
# "enable_alipay_topup": true,
# "alipay_min_topup": 1
```

如果 `enable_alipay_topup` 是 `false`，回 admin 看是不是开关没保存上。

### 3.4 在支付宝开放平台登记回调（推荐）

虽然本系统下单时主动把 `return_url` / `notify_url` 塞进 SDK，但建议在开放平台应用配置里也填一遍，避免某些场景下被回退到平台默认：

- 开放平台 → 我的应用 → 应用详情 → 开发设置
- 授权回调地址：`https://<你的域名>/api/user/alipay/return`
- （若有"异步通知地址"项）`https://<你的域名>/api/user/alipay/notify`

---

## 第 4 章 · 支付宝下单 & 异步通知测试

### 4.1 创建充值订单

用**普通用户**身份登录前端 → 充值页 → 选"支付宝"按钮 → 充值 **1 元**。

**期望：** 浏览器新开 tab 跳到支付宝收银台（域名 `mclient.alipay.com` 或 `openapi.alipay.com`），显示订单 1 元。

**如果新 tab 没打开 / 弹出报错：**

```bash
# F12 → Network → POST /api/user/alipay/pay → 看响应 body
# 后端日志关键词：
grep -i "alipay TradePagePay\|alipay topup insert\|alipay service" logs/log.txt | tail -20
```

常见错：

| 错误信息 | 原因 | 修复 |
|---|---|---|
| `支付服务暂不可用` | client 没初始化 | 检查私钥/公钥格式，重新保存 admin |
| `alipay TradePagePay failed: ...sign verify error` | 私钥不匹配应用公钥 | 把"应用公钥"那个 txt 上传到支付宝开放平台 |
| `充值金额过低` | minTopUp 没配 | admin 把最低充值改为 1 |
| `获取用户分组失败` | 用户被禁用或分组异常 | 换一个正常用户重试 |

### 4.2 数据库快照（下单后）

```sql
-- 拿到刚创建的订单
SELECT trade_no, status, payment_method, money, pay_amount_cents, expire_time, create_time
FROM top_ups
WHERE user_id = <普通用户 id>
ORDER BY id DESC LIMIT 1;
```

**期望：** `status='pending'`、`payment_method='alipay'`、`pay_amount_cents=100`、`expire_time = create_time + 1800`。

### 4.3 真实付款

用支付宝 App 扫码（或浏览器登录支付宝）支付那 1 元。

**期望同步效果：**
- 浏览器 tab 自动跳回 `https://<你的域名>/console/topup?pay=success&trade_no=USR...`
- 前端 toast `支付成功`，用户余额 **+1 元对应额度**。

**期望异步效果（后端日志，约付款后 1~30 秒）：**
```
[INFO] alipay notify: idempotent skip USR... (already completed)
```
（说明异步通知到了，但 GetTopUpStatus 主动查单已经把订单完成了，notify 走幂等路径）

**或者：**
```
[INFO] 使用支付宝在线充值成功，充值金额：100，订单号：USR...
```
（说明 notify 比 status 主动查更早到达）

任一其一即正常。

### 4.4 数据库再次快照（付款后）

```sql
SELECT trade_no, status, paid_at, provider_tx_id FROM top_ups WHERE trade_no='<上一步的 trade_no>';
SELECT id, username, quota FROM users WHERE id=<普通用户 id>;
SELECT * FROM logs WHERE user_id=<普通用户 id> AND type=1 ORDER BY id DESC LIMIT 1;  -- type=1 是 LogTypeTopup
```

**期望：**
- `status='success'`
- `paid_at` 是付款时间戳
- `provider_tx_id` 是支付宝交易号（形如 `2026051522001...`）
- `users.quota` 比测试前增加 `1 * common.QuotaPerUnit`（项目默认 500000，即 50 万）
- `logs` 有一行充值记录

### 4.5 异步通知到达验证（关键）

如果第 4.3 步只看到 `idempotent skip`，**没看到首次完成的日志**，说明 notify 路径没走通，问题可能出在：

- 防火墙 / 安全组阻挡了支付宝服务器（`110.75.x.x`、`140.205.x.x` 等）
- nginx 没把 `/api/user/alipay/notify` 转发到后端
- 服务器地址里写了非公网 IP

排查命令：

```bash
# 从外网模拟一次回调（注意：没有有效签名，会返回 failure，但能验证路由通不通）
curl -X POST https://<你的域名>/api/user/alipay/notify \
  -d "out_trade_no=PROBE&trade_status=TRADE_SUCCESS" \
  -i
# 期望：HTTP/2 200，body 是 "failure"（签名验证失败，正常）
# 后端日志期望：alipay notify: signature verification failed: ...
```

如果连这一步都 502 / timeout，回去检查 nginx 配置。

---

## 第 5 章 · 微信支付下单 & 通知测试

### 5.1 admin 配置

支付设置 → 微信支付面板：

| 字段 | 值 |
|---|---|
| AppId | 0.3 C 拿到的关联 AppId |
| 商户号 MchId | `1738223441` |
| 商户证书序列号 | 1.2 节 openssl 输出的 serial |
| APIv3 密钥 | 0.3 D 设置的 32 位字符串 |
| 商户私钥 | `apiclient_key.pem` 整个文件内容 |
| 异步通知地址 | 留空（用默认） |
| 最低充值数量 | 1 |
| 启用微信支付 | **开关打开** |

点"更新微信支付设置"，期望 toast `更新成功`。

### 5.2 在商户平台登记回调

商户平台 → 产品中心 → 开发配置：
- 支付通知 URL：`https://<你的域名>/api/user/wxpay/notify`
- 退款通知 URL：`https://<你的域名>/api/user/wxpay/refund/notify`

### 5.3 创建充值订单（0.01 元）

普通用户登录 → 充值页 → 点"微信支付" → 充值 **0.01 元**。

⚠️ 微信 Native 模式的最小金额就是 0.01 元（1 分钱）。

**期望：** 弹出二维码 Modal，里面是一张 QR code 图。

**如果 Modal 不弹 / 报错：**

```bash
# F12 → Network → POST /api/user/wxpay/pay → Response body
# 后端日志关键词：
grep -i "wechat native prepay\|wxpay\|wechatpay" logs/log.txt | tail -30
```

⚠️ **此处最容易撞上 0.4 节的风险**：如果错误是

```
wechat native prepay failed: ... certificate visitor ...
wechat native prepay failed: ... no certificate found ...
wechat native prepay failed: ... decrypt failed ...
```

说明商户号是新接入、只支持 API 公钥模式，需要切换 SDK 选项。这种情况停下来，把日志发出来。

其他常见错：

| 错误 | 原因 | 修复 |
|---|---|---|
| `appid mch_id 不匹配` | AppId 没和这个 MchId 关联 | 商户后台关联 AppId 后重试 |
| `签名错误` | MchSerialNo 填错 | 重新 openssl 提取 |
| `参数错误：amount.total` | 金额非整数分 | 充值金额取整 |
| `商户号 mch_id 不正确` | MchId 多了空格 | admin 重新填 |

### 5.4 数据库快照（下单后）

```sql
SELECT trade_no, status, payment_method, money, pay_amount_cents, expire_time
FROM top_ups WHERE user_id=<普通用户 id> ORDER BY id DESC LIMIT 1;
```

**期望：** `payment_method='wxpay'`，`pay_amount_cents=1`，`status='pending'`。

### 5.5 扫码支付

用手机微信扫 Modal 里的二维码 → 支付 0.01 元。

**期望前端效果：**
- Modal 每 3 秒轮询一次 `/api/user/topup/status`，付款后下一次轮询 status 变 `success`
- Modal 自动跳到"支付成功，额度已到账"状态，2 秒后关闭
- 用户余额 +0.01 元对应额度

**期望后端日志：**
```
[INFO] 使用微信在线充值成功，充值金额：1，订单号：USR...
```

### 5.6 数据库二次快照

```sql
SELECT trade_no, status, paid_at, provider_tx_id FROM top_ups WHERE trade_no='<上一步的>';
SELECT quota FROM users WHERE id=<普通用户 id>;  -- 比测试前增加 1 * QuotaPerUnit / 100
```

**期望：** `status='success'`，`provider_tx_id` 是微信交易号（形如 `4200001...`），`paid_at` 已写入。

### 5.7 异步通知到达验证

```bash
# 从外网探活
curl -X POST https://<你的域名>/api/user/wxpay/notify \
  -H 'Content-Type: application/json' \
  -d '{}' -i
# 期望：HTTP/2 401，body 包含 "code":"FAIL"（无法解密，正常）
# 后端日志：wxpay notify: decrypt failed: ...
```

如果连这一步都 502 / timeout，nginx 路由没通。

---

## 第 6 章 · 退款测试

### 6.1 支付宝退款（同步完成）

用 **root 账号** 登录 → 充值历史 → 找到第 4 章付款的那笔 → 点"退款"。

```
弹窗 1：填写退款理由（例如 "测试退款"），点"获取确认 token"
   ↓
弹窗 2：显示退款摘要 + 5 分钟倒计时，点"确认退款"
```

**期望：**
- 后端日志 `alipay TradeRefund` 调用 + `管理员退款，订单号：USR...，原因：测试退款`
- 数据库：

```sql
SELECT trade_no, status, refund_status, refund_time, refund_admin_id, refunded_quota
FROM top_ups WHERE trade_no='<付款用的那个>';
-- 期望：refund_status='refund_success'
--      refunded_quota = 之前发放的额度
--      refund_admin_id = root 用户 id
```

- 用户余额扣回 1 元对应额度。
- 支付宝 App 收到退款通知（约 5 分钟内到账原支付账户）。

### 6.2 微信退款（异步完成）

同样用 root 在充值历史里对第 5 章那笔 0.01 元订单发起退款。

**期望立即效果：**
- 数据库 `refund_status` 进入 `refund_pending`，不是 success。
- 用户余额**还没扣**（要等异步通知）。

**期望约 30 秒~3 分钟后：**
- 微信发回 `/api/user/wxpay/refund/notify`
- 后端日志 `wxpay refund notify: completed refund USR...`
- 数据库 `refund_status='refund_success'`，`refund_time` 已写入
- 用户余额扣回 0.01 元对应额度

```sql
-- 中间状态
SELECT trade_no, refund_status FROM top_ups WHERE trade_no='<微信付款单号>';
-- 应该先看到 refund_pending，几分钟后再查变 refund_success
```

如果超过 30 分钟还在 `refund_pending`，检查微信商户后台是否登记了退款通知 URL（5.2 节）。

### 6.3 重复退款防护测试

在 6.1 退款完成后，**再点一次"退款"**。

**期望：**
- 前端报 `订单已退款` 或类似拒绝信息
- 数据库 `refunded_quota` 不变，没有二次扣额度

如果重复退款成功了，立即停止测试上报问题（说明状态机守卫失效）。

---

## 第 7 章 · 异常路径测试（可选但推荐）

### 7.1 订单超时自动关单

```bash
# 在 cron tick 之前手工把一个 pending 订单的 expire_time 改小，模拟过期
sqlite3 data/one-api.db "UPDATE top_ups SET expire_time=1 WHERE trade_no='<某 pending 订单>';"
# 等 5 分钟（或重启服务后立即跑一次 cron）
# 期望：status 从 pending → expired
sqlite3 data/one-api.db "SELECT status FROM top_ups WHERE trade_no='<上面那个>';"
```

后端日志期望：`CloseExpiredPendingTopUps: ok=1 fail=0`。

### 7.2 异步通知幂等

```bash
# 用同一笔已完成的订单的真实通知 body（从日志里复制）
# 重放两次
for i in 1 2; do
  curl -X POST https://<你的域名>/api/user/alipay/notify -d @real_notify_body.txt -i
  sleep 1
done
# 期望：两次都返回 success，但日志只有第一次写了 "充值成功"，第二次是 "idempotent skip"
# 期望：用户余额只增加一次
```

### 7.3 status 接口越权探测

```bash
# 用 A 用户的 token 查 B 用户的订单号
curl https://<你的域名>/api/user/topup/status?trade_no=<B用户的订单号> \
  -H 'Authorization: Bearer <A 用户的 token>'
# 期望：HTTP 404，body {"message":"error","data":"订单不存在"}
# 注意：不能泄露"订单存在但属于别人"的信息
```

---

## 第 8 章 · 测试完成 checklist

逐项打 ✅ 才算通过：

```
□ 4.3 支付宝付款，用户余额增加
□ 4.4 数据库 top_ups.status=success
□ 4.5 异步通知日志出现 (idempotent skip 或 充值成功 任一)
□ 5.5 微信扫码付款，前端 Modal 自动跳成功状态
□ 5.6 数据库 top_ups.status=success
□ 5.7 nginx 路由探活返回 401（未签名）
□ 6.1 支付宝退款，refund_status=success，余额扣回
□ 6.2 微信退款，refund_status=success（异步完成）
□ 6.3 重复退款被拒绝
□ 7.1 订单超时被 cron 关单（可选）
□ 7.2 异步通知重放幂等（可选）
□ 7.3 越权探测返回 404 而非 403/200（可选）
```

---

## 附录 A · 数据库回滚

如果测试失败、需要回滚到测试前的状态：

```bash
# SQLite
cp data/one-api.db.before-payment-test data/one-api.db
# 重启服务

# MySQL
mysql -u root -p new_api < new_api.before-payment-test.sql

# PostgreSQL
psql -U postgres -d new_api -f new_api.before-payment-test.sql
```

## 附录 B · 关键日志关键词速查

```bash
# 支付宝
grep -iE 'alipay (TradePagePay|TradeQuery|TradeRefund|notify|return)' logs/log.txt

# 微信
grep -iE 'wechat (native prepay|query order|refund)|wxpay notify' logs/log.txt

# 退款
grep -iE 'refund_pending|refund_success|MarkRefund|CompleteRefund' logs/log.txt

# cron
grep -iE 'CloseExpiredPendingTopUps|ReconcileStaleRefundsPending' logs/log.txt
```

## 附录 C · 测试中遇到 0.4 节那个风险怎么办

如果第 5.3 节微信下单失败，错误信息含 `certificate visitor` / `no certificate found` / `decrypt failed`：

1. 立刻停止测试，把以下信息发给开发：
   - 失败的 `wechat native prepay failed: ...` 完整一行
   - 商户号 `1738223441` 是 2024 末之后接入的吗？（去商户后台看入网时间）
2. 开发会补一个支持 `WithWechatPayPublicKeyAuthCipher` 的小 PR（约 2 小时）
3. PR 合并 → 重启服务 → admin 多填一项"WechatPayPubKey"（用 `pub_key.pem` 内容）→ 重新测试。

---

## 附录 D · 关于本次测试涉及的真实金额

- 支付宝 1 元 → 测完退款，会原路返回（约 5 分钟内）。
- 微信 0.01 元 → 测完退款，会原路返回（约几分钟内）。

理论上零成本。但请确认：
- 测试账号是企业账号，付款来自公司账户。
- 退款会回到付款的同一账户（支付宝 → 付款的支付宝账户；微信 → 付款的微信钱包）。
- 如果中途测试中断，订单留在 `pending` 状态，cron 会在 30 分钟后关单。已付款但通知没到达的订单，可以人工调用 `POST /api/user/alipay/notify` 重放，或者直接在数据库手工把 status 改为 `success`（**不推荐，幂等保护可能会绕过**）。

---

**文档版本：** 2026-05-15
**对应分支：** `feat/wechat-alipay-payment`
**对应实施文档：** `docs/superpowers/plans/2026-05-14-wechat-alipay-payment-impl-plan.md`
**对应配置文档：** `docs/ALIPAY_SETUP_GUIDE.md`、`docs/WXPAY_SETUP_GUIDE.md`
