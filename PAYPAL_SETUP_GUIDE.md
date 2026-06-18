# PayPal 支付对接配置指南

## 1. 获取 PayPal 开发者凭证

### 步骤 1：创建 PayPal 开发者账户
1. 访问 [PayPal Developer Dashboard](https://developer.paypal.com/dashboard)
2. 如果没有 PayPal 账户，先注册一个
3. 登录 PayPal 开发者面板

### 步骤 2：创建应用

#### Sandbox 环境（推荐先用于测试）
1. 在左侧菜单，点击 **Apps & Credentials**
2. 确保选中 **Sandbox** 标签页
3. 在 **REST API apps** 部分，点击 **Create App**
4. 输入应用名称（如 "new-api-sandbox"）
5. 点击 **Create App**

#### Live 环境（生产环境）
1. 在左侧菜单，点击 **Apps & Credentials**
2. 切换到 **Live** 标签页
3. 重复上述步骤创建生产应用

### 步骤 3：获取凭证

创建应用后，你会看到以下信息：

```
Sandbox:
- Client ID: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
- Secret: yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy

Live:
- Client ID: (生产环境凭证)
- Secret: (生产环境凭证)
```

**重要：** 妥善保管这些凭证，不要在公开场合分享

---

## 2. 设置 Webhook

### 步骤 1：访问 Webhook 设置

1. 在 PayPal Developer Dashboard，点击 **Apps & Credentials**
2. 选择你创建的应用
3. 在 **Sandbox Webhooks** 或 **Live Webhooks** 部分，点击 **Add Webhook**

### 步骤 2：配置 Webhook URL

在 **Webhook URL** 字段中填入：
```
https://你的域名/api/paypal/webhook
```

例如：
```
https://api.example.com/api/paypal/webhook
```

### 步骤 3：订阅事件

选择以下事件类型：
- ✅ **CHECKOUT.ORDER.APPROVED** - 订单已批准
- ✅ **PAYMENT.CAPTURE.COMPLETED** - 支付已完成

点击 **Save** 保存

### 步骤 4：获取 Webhook ID

Webhook 创建后，你会看到一个长字符串作为 **Webhook ID**。复制此 ID 用作 **Webhook Secret**。

---

## 3. 在管理后台配置

### 访问配置页面

1. 登录管理员账户
2. 点击 **Settings（设置）**
3. 在支付相关选项中找到 **Payment（支付设置）**
4. 滚动到 **PayPal 设置** 部分

### 填写配置信息

| 字段 | 说明 | 示例 |
|------|------|------|
| **Client ID** | PayPal 应用的 Client ID | `AZBxxxxxxxxxxxxxA1B2C3D4E5F6G7H8` |
| **Client Secret** | PayPal 应用的 Client Secret（敏感信息） | `EJ2xxxxxxxxxxxxxY9Z8A7B6C5D4E3F` |
| **Webhook Secret** | PayPal Webhook ID | `1A2B3C4D5E6F7G8H9I0J` |
| **运行模式** | `Sandbox`（测试） 或 `Live`（生产） | 测试时选择 `Sandbox` |
| **最低充值金额** | 用户最少需充值的 USD 美元数 | `1`（$1 美元） |

### 保存设置

填写完所有字段后，点击 **更新 PayPal 设置** 按钮保存配置。

---

## 4. 测试支付

### 在 Sandbox 环境测试

1. **创建测试账户**
   - 在 PayPal Developer Dashboard，点击 **Accounts**
   - 你会看到预创建的买家和卖家账户

2. **访问充值页面**
   - 用普通用户账户登录应用
   - 进入 **充值** 页面
   - 选择 **PayPal** 作为支付方式

3. **完成测试支付**
   - 输入充值金额
   - 点击 **使用 PayPal 充值**
   - 使用 PayPal Sandbox 测试账户完成支付
   - 验证充值成功

### 常见测试账户

Sandbox 账户通常会自动创建，格式为：
- **买家账户**：`buyer-xxxxx@business.example.com` 密码：`sandbox_password`
- **卖家账户**：`merchant-xxxxx@business.example.com` 密码：`sandbox_password`

---

## 5. 切换到生产环境

### 准备工作

1. 确保在 Sandbox 环境充分测试
2. 获取 Live 环境的凭证
3. 在 Live 环境创建 Webhook

### 配置步骤

1. 在管理后台的 PayPal 设置中
2. 更新以下字段为 Live 凭证：
   - **Client ID** → Live 凭证
   - **Client Secret** → Live 凭证
   - **Webhook Secret** → Live Webhook ID
   - **运行模式** → 改为 `Live`

3. 点击 **更新 PayPal 设置**

### 验证生产环境

1. 用真实账户进行小额测试支付
2. 确认充值成功
3. 监控支付记录和 Webhook 日志

---

## 6. 故障排查

### 支付链接无法打开

**问题：** 点击支付后无反应或显示错误

**解决方案：**
1. 检查 Client ID 和 Client Secret 是否正确
2. 确保应用已在 PayPal Dashboard 中启用
3. 检查 Webhook URL 是否可从互联网访问

### Webhook 未收到

**问题：** 支付完成但未收到 Webhook 回调

**解决方案：**
1. 在 PayPal Dashboard 的 Webhook 部分检查"Recent Deliveries"
2. 确保 Webhook URL 正确且可访问
3. 检查服务器日志中是否有错误
4. 确保已订阅所需的事件类型

### 充值金额不正确

**问题：** 用户充值金额不符合预期

**解决方案：**
1. 检查"最低充值金额"配置
2. 验证汇率换算是否正确
3. 检查是否有货币兑换问题（USD ↔ 本地货币）

### 测试账户无法登录

**问题：** Sandbox 测试账户无法在 PayPal.com 登录

**解决方案：**
- 这是正常的，测试账户只能在 Sandbox.PayPal.com 使用
- 访问 https://sandbox.paypal.com 进行测试支付

---

## 7. 安全建议

✅ **应该做：**
- 使用强密码保护开发者账户
- 定期轮换 Webhook Secret
- 在生产环境使用 Live 凭证
- 记录所有支付事务用于审计

❌ **不应该做：**
- 在代码、日志或文档中硬编码凭证
- 将凭证提交到版本控制系统
- 在前端 JavaScript 中暴露 Secret
- 在不安全的渠道传递凭证

---

## 8. 常用链接

| 资源 | 链接 |
|------|------|
| PayPal 开发者主页 | https://developer.paypal.com |
| API 文档 | https://developer.paypal.com/docs/api/orders/v2 |
| Sandbox 测试 | https://sandbox.paypal.com |
| 安全最佳实践 | https://developer.paypal.com/docs/checkout/best-practices |
| 支持中心 | https://www.paypal.com/us/webapps/mpp/contact-us |

---

## 9. 技术说明

### 支付流程

```
用户 → 选择支付金额和方式(PayPal)
  ↓
后端 → 创建 PayPal 订单
  ↓
用户 → 跳转到 PayPal 完成支付
  ↓
PayPal → 重定向返回应用
  ↓
PayPal → 发送 Webhook 到后端
  ↓
后端 → 验证 Webhook，发放充值额度
  ↓
用户 → 充值完成
```

### 支付方法

新系统使用 **PayPal Orders API v2** 创建订单并捕获支付。

### 支持的货币

默认使用 USD（美元）。如需支持其他货币，需修改后端代码。

---

## 10. 支持

如有问题：

1. 查看 [PayPal 开发者文档](https://developer.paypal.com/docs)
2. 检查 Webhook 日志确认交易状态
3. 联系 PayPal 技术支持
4. 提交 Issue 到项目仓库

---

**最后更新：** 2026-05-06
**版本：** 1.0
