# 微信小程序集成指南

## 概述

本指南详细描述如何将微信小程序用户与 New API 系统集成，实现用户同步、充值管理和 LLM API 访问。

## 集成架构

### 系统架构图
```
微信小程序 <-> 后端服务 <-> New API 系统
    |           |             |
    |           |             ├── 用户管理
    |           |             ├── 计费系统
    |           |             └── LLM 网关
    |           |
    |           ├── 微信API集成
    |           ├── 支付处理
    |           └── 用户数据同步
    |
    ├── 用户认证
    ├── 支付功能
    └── LLM调用
```

### 核心组件
1. **微信小程序端**: 用户界面和微信API调用
2. **后端服务**: 微信API集成和New API桥接
3. **New API系统**: LLM网关和计费系统

## 前置准备

### 微信小程序配置
1. **AppID**: 微信小程序唯一标识
2. **AppSecret**: 服务端调用微信API的密钥
3. **服务器域名**: 配置request合法域名
4. **支付配置**: 微信支付商户号和密钥（如需要）

### 后端服务准备
```javascript
// 环境变量配置
const config = {
  wechat: {
    appId: process.env.WECHAT_APPID,
    appSecret: process.env.WECHAT_APP_SECRET
  },
  newApi: {
    baseUrl: process.env.NEW_API_BASE_URL, // 如: http://localhost:3000
    endpoints: {
      sync: '/api/user/external/sync',
      topup: '/api/user/external/topup',
      token: '/api/user/external/token',
      stats: '/api/user/external/{id}/stats'
    }
  }
};
```

## 集成步骤

### 步骤1: 微信小程序登录实现

#### 1.1 小程序端代码
```javascript
// app.js 或登录页面
Page({
  data: {
    userInfo: null,
    hasUserInfo: false
  },

  // 微信登录
  onWechatLogin() {
    wx.login({
      success: (res) => {
        if (res.code) {
          // 获取用户信息
          this.getUserProfile(res.code);
        } else {
          console.log('微信登录失败：' + res.errMsg);
        }
      }
    });
  },

  // 获取用户资料
  getUserProfile(code) {
    wx.getUserProfile({
      desc: '用于完善用户资料',
      success: (res) => {
        const userInfo = res.userInfo;
        console.log('用户信息：', userInfo);

        // 调用后端同步用户
        this.syncUserToNewAPI(code, userInfo);
      },
      fail: (err) => {
        console.log('获取用户信息失败：', err);
      }
    });
  },

  // 同步用户到New API
  syncUserToNewAPI(code, userInfo) {
    wx.request({
      url: 'https://your-backend.com/api/wechat/sync-user',
      method: 'POST',
      data: {
        code: code,
        userInfo: userInfo
      },
      success: (res) => {
        if (res.data.success) {
          // 保存用户登录状态
          wx.setStorageSync('userToken', res.data.token);
          wx.setStorageSync('externalUserId', res.data.external_user_id);

          this.setData({
            userInfo: userInfo,
            hasUserInfo: true
          });

          console.log('用户同步成功');
        }
      },
      fail: (err) => {
        console.log('用户同步失败：', err);
      }
    });
  }
});
```

#### 1.2 小程序配置文件
```json
// app.json
{
  "pages": [
    "pages/index/index",
    "pages/profile/profile",
    "pages/payment/payment"
  ],
  "permission": {
    "scope.userInfo": {
      "desc": "你的个人信息将用于更好的为你提供服务"
    }
  },
  "requiredBackgroundModes": ["audio"],
  "networkTimeout": {
    "request": 10000
  }
}
```

### 步骤2: 后端微信集成服务

#### 2.1 获取微信OpenID服务
```javascript
// services/wechatService.js
const axios = require('axios');

class WechatService {
  constructor(appId, appSecret) {
    this.appId = appId;
    this.appSecret = appSecret;
  }

  // 通过code获取openid和session_key
  async getOpenId(code) {
    const url = 'https://api.weixin.qq.com/sns/jscode2session';
    const params = {
      appid: this.appId,
      secret: this.appSecret,
      js_code: code,
      grant_type: 'authorization_code'
    };

    try {
      const response = await axios.get(url, { params });
      const data = response.data;

      if (data.errcode) {
        throw new Error(`微信API错误: ${data.errcode} - ${data.errmsg}`);
      }

      return {
        openid: data.openid,
        session_key: data.session_key,
        unionid: data.unionid || null
      };
    } catch (error) {
      console.error('获取微信OpenID失败:', error);
      throw error;
    }
  }

  // 解密微信数据（如手机号）
  decryptData(sessionKey, encryptedData, iv) {
    // 微信数据解密逻辑
    // 使用 crypto-js 或其他解密库
  }
}

module.exports = WechatService;
```

#### 2.2 New API 集成服务
```javascript
// services/newApiService.js
const axios = require('axios');

class NewApiService {
  constructor(baseUrl) {
    this.baseUrl = baseUrl;
    this.client = axios.create({
      baseURL: baseUrl,
      timeout: 10000,
      headers: {
        'Content-Type': 'application/json'
      }
    });
  }

  // 同步用户到New API
  async syncUser(userData) {
    try {
      const response = await this.client.post('/api/user/external/sync', userData);
      return response.data;
    } catch (error) {
      console.error('同步用户失败:', error.response?.data || error.message);
      throw error;
    }
  }

  // 用户充值
  async topupUser(topupData) {
    try {
      const response = await this.client.post('/api/user/external/topup', topupData);
      return response.data;
    } catch (error) {
      console.error('用户充值失败:', error.response?.data || error.message);
      throw error;
    }
  }

  // 创建Token
  async createToken(tokenData) {
    try {
      const response = await this.client.post('/api/user/external/token', tokenData);
      return response.data;
    } catch (error) {
      console.error('创建Token失败:', error.response?.data || error.message);
      throw error;
    }
  }

  // 获取用户统计
  async getUserStats(externalUserId) {
    try {
      const response = await this.client.get(`/api/user/external/${externalUserId}/stats`);
      return response.data;
    } catch (error) {
      console.error('获取用户统计失败:', error.response?.data || error.message);
      throw error;
    }
  }
}

module.exports = NewApiService;
```

#### 2.3 用户同步API端点
```javascript
// routes/wechat.js
const express = require('express');
const WechatService = require('../services/wechatService');
const NewApiService = require('../services/newApiService');

const router = express.Router();
const wechatService = new WechatService(process.env.WECHAT_APPID, process.env.WECHAT_APP_SECRET);
const newApiService = new NewApiService(process.env.NEW_API_BASE_URL);

// 微信用户同步端点
router.post('/sync-user', async (req, res) => {
  try {
    const { code, userInfo } = req.body;

    // 1. 获取微信OpenID
    const wechatAuth = await wechatService.getOpenId(code);

    // 2. 构造New API用户数据
    const newApiUserData = {
      external_user_id: `wx_mini_${wechatAuth.openid}`,
      username: `wx_user_${wechatAuth.openid.slice(-8)}`,
      display_name: userInfo.nickName || '微信用户',
      wechat_openid: wechatAuth.openid,
      wechat_unionid: wechatAuth.unionid || '',
      login_type: 'wechat',
      external_data: JSON.stringify({
        miniprogram_info: {
          session_key: wechatAuth.session_key,
          avatar_url: userInfo.avatarUrl,
          gender: userInfo.gender,
          country: userInfo.country,
          province: userInfo.province,
          city: userInfo.city,
          language: userInfo.language
        }
      })
    };

    // 3. 同步到New API
    const syncResult = await newApiService.syncUser(newApiUserData);

    // 4. 生成用户Token（可选，用于标识）
    const userToken = generateUserToken(wechatAuth.openid);

    res.json({
      success: true,
      message: '用户同步成功',
      token: userToken,
      external_user_id: newApiUserData.external_user_id,
      sync_result: syncResult
    });

  } catch (error) {
    console.error('用户同步错误:', error);
    res.status(500).json({
      success: false,
      message: '用户同步失败',
      error: error.message
    });
  }
});

// 生成用户Token
function generateUserToken(openid) {
  // 简单的token生成，生产环境建议使用JWT
  const crypto = require('crypto');
  return crypto.createHmac('sha256', process.env.JWT_SECRET || 'default_secret')
    .update(openid + Date.now())
    .digest('hex');
}

module.exports = router;
```

### 步骤3: 充值功能集成

#### 3.1 小程序端充值界面
```javascript
// pages/payment/payment.js
Page({
  data: {
    amounts: [
      { label: '¥6.88', value: 6.88, quota: '344万tokens' },
      { label: '¥19.9', value: 19.9, quota: '995万tokens' },
      { label: '¥49.9', value: 49.9, quota: '2495万tokens' }
    ],
    selectedAmount: null
  },

  // 选择充值金额
  selectAmount(e) {
    const amount = e.currentTarget.dataset.amount;
    this.setData({ selectedAmount: amount });
  },

  // 发起微信支付
  onPay() {
    if (!this.data.selectedAmount) {
      wx.showToast({ title: '请选择充值金额', icon: 'none' });
      return;
    }

    const externalUserId = wx.getStorageSync('externalUserId');
    if (!externalUserId) {
      wx.showToast({ title: '用户信息异常', icon: 'none' });
      return;
    }

    // 调用后端创建支付订单
    wx.request({
      url: 'https://your-backend.com/api/payment/create-order',
      method: 'POST',
      data: {
        external_user_id: externalUserId,
        amount_cny: this.data.selectedAmount,
        payment_type: 'wechat'
      },
      success: (res) => {
        if (res.data.success) {
          // 调用微信支付
          this.callWechatPay(res.data.payment_params);
        }
      }
    });
  },

  // 调用微信支付
  callWechatPay(paymentParams) {
    wx.requestPayment({
      timeStamp: paymentParams.timeStamp,
      nonceStr: paymentParams.nonceStr,
      package: paymentParams.package,
      signType: 'MD5',
      paySign: paymentParams.paySign,
      success: (res) => {
        console.log('支付成功:', res);
        wx.showToast({ title: '支付成功', icon: 'success' });
        // 跳转到成功页面或刷新余额
        this.refreshUserBalance();
      },
      fail: (res) => {
        console.log('支付失败:', res);
        wx.showToast({ title: '支付失败', icon: 'none' });
      }
    });
  },

  // 刷新用户余额
  refreshUserBalance() {
    const externalUserId = wx.getStorageSync('externalUserId');
    wx.request({
      url: `https://your-backend.com/api/user/stats/${externalUserId}`,
      method: 'GET',
      success: (res) => {
        if (res.data.success) {
          // 更新用户余额显示
          console.log('用户余额:', res.data.data.user_info.current_balance);
        }
      }
    });
  }
});
```

#### 3.2 后端支付处理
```javascript
// routes/payment.js
const express = require('express');
const NewApiService = require('../services/newApiService');

const router = express.Router();
const newApiService = new NewApiService(process.env.NEW_API_BASE_URL);

// 创建支付订单
router.post('/create-order', async (req, res) => {
  try {
    const { external_user_id, amount_cny, payment_type } = req.body;

    // 1. 汇率转换（简化版，生产环境应使用实时汇率）
    const exchangeRate = 0.14; // 1 CNY = 0.14 USD（示例汇率）
    const amount_usd = parseFloat((amount_cny * exchangeRate).toFixed(2));

    // 2. 创建微信支付订单（示例）
    const paymentOrder = await createWechatPaymentOrder({
      external_user_id,
      amount_cny,
      amount_usd
    });

    res.json({
      success: true,
      message: '订单创建成功',
      order_id: paymentOrder.order_id,
      payment_params: paymentOrder.payment_params
    });

  } catch (error) {
    console.error('创建支付订单失败:', error);
    res.status(500).json({
      success: false,
      message: '订单创建失败',
      error: error.message
    });
  }
});

// 支付回调处理
router.post('/wechat-notify', async (req, res) => {
  try {
    // 1. 验证微信支付回调
    const paymentResult = verifyWechatPayment(req.body);

    if (paymentResult.success) {
      // 2. 充值到New API
      const topupResult = await newApiService.topupUser({
        external_user_id: paymentResult.external_user_id,
        amount_usd: paymentResult.amount_usd,
        payment_id: `wechat_${paymentResult.transaction_id}`
      });

      console.log('充值成功:', topupResult);
    }

    // 3. 返回微信要求的格式
    res.json({ code: 'SUCCESS', message: '成功' });

  } catch (error) {
    console.error('支付回调处理失败:', error);
    res.json({ code: 'FAIL', message: error.message });
  }
});

// 创建微信支付订单（示例实现）
async function createWechatPaymentOrder(orderData) {
  // 这里应该调用微信支付API
  // 返回支付参数给小程序
  return {
    order_id: 'order_' + Date.now(),
    payment_params: {
      timeStamp: String(Date.now()),
      nonceStr: 'random_string',
      package: 'prepay_id=wx123456789',
      signType: 'MD5',
      paySign: 'calculated_sign'
    }
  };
}

// 验证微信支付回调（示例实现）
function verifyWechatPayment(notifyData) {
  // 这里应该验证微信支付回调的签名
  return {
    success: true,
    external_user_id: 'wx_mini_example',
    amount_usd: 6.88 * 0.14,
    transaction_id: 'wx_transaction_123'
  };
}

module.exports = router;
```

### 步骤4: LLM API 调用集成

#### 4.1 小程序端AI对话界面
```javascript
// pages/chat/chat.js
Page({
  data: {
    messages: [],
    inputText: '',
    isLoading: false,
    userApiToken: null
  },

  onLoad() {
    // 获取用户API Token
    this.getUserApiToken();
  },

  // 获取用户API Token
  getUserApiToken() {
    const externalUserId = wx.getStorageSync('externalUserId');

    wx.request({
      url: 'https://your-backend.com/api/user/get-api-token',
      method: 'POST',
      data: { external_user_id: externalUserId },
      success: (res) => {
        if (res.data.success) {
          this.setData({
            userApiToken: res.data.api_token
          });
        }
      }
    });
  },

  // 发送消息
  sendMessage() {
    const message = this.data.inputText.trim();
    if (!message || this.data.isLoading) return;

    // 添加用户消息
    const userMessage = {
      role: 'user',
      content: message,
      timestamp: Date.now()
    };

    this.setData({
      messages: [...this.data.messages, userMessage],
      inputText: '',
      isLoading: true
    });

    // 调用LLM API
    this.callLLMAPI(message);
  },

  // 调用LLM API
  callLLMAPI(message) {
    if (!this.data.userApiToken) {
      wx.showToast({ title: '请先获取API权限', icon: 'none' });
      return;
    }

    wx.request({
      url: 'https://your-backend.com/v1/chat/completions',
      method: 'POST',
      header: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${this.data.userApiToken}`
      },
      data: {
        model: 'qwen-turbo',
        messages: [
          {
            role: 'user',
            content: message
          }
        ],
        max_tokens: 1000,
        temperature: 0.7
      },
      success: (res) => {
        if (res.data.choices && res.data.choices.length > 0) {
          const assistantMessage = {
            role: 'assistant',
            content: res.data.choices[0].message.content,
            timestamp: Date.now()
          };

          this.setData({
            messages: [...this.data.messages, assistantMessage],
            isLoading: false
          });
        }
      },
      fail: (err) => {
        console.error('LLM API调用失败:', err);
        wx.showToast({ title: 'AI回复失败', icon: 'none' });
        this.setData({ isLoading: false });
      }
    });
  },

  // 输入框变化
  onInputChange(e) {
    this.setData({ inputText: e.detail.value });
  }
});
```

#### 4.2 小程序端页面布局
```xml
<!-- pages/chat/chat.wxml -->
<view class="chat-container">
  <!-- 消息列表 -->
  <scroll-view class="message-list" scroll-y="true" scroll-top="{{scrollTop}}">
    <view wx:for="{{messages}}" wx:key="timestamp" class="message-item {{item.role}}">
      <view class="message-content">{{item.content}}</view>
      <view class="message-time">{{item.timestamp}}</view>
    </view>

    <view wx:if="{{isLoading}}" class="loading-message">
      <text>AI思考中...</text>
    </view>
  </scroll-view>

  <!-- 输入区域 -->
  <view class="input-area">
    <input
      type="text"
      placeholder="输入消息..."
      value="{{inputText}}"
      bindinput="onInputChange"
      class="message-input"
    />
    <button
      bindtap="sendMessage"
      disabled="{{isLoading}}"
      class="send-button"
    >
      发送
    </button>
  </view>
</view>
```

### 步骤5: 用户管理功能

#### 5.1 用户资料页面
```javascript
// pages/profile/profile.js
Page({
  data: {
    userInfo: null,
    userStats: null,
    apiTokens: []
  },

  onLoad() {
    this.loadUserProfile();
  },

  // 加载用户资料
  loadUserProfile() {
    const externalUserId = wx.getStorageSync('externalUserId');

    wx.request({
      url: `https://your-backend.com/api/user/stats/${externalUserId}`,
      method: 'GET',
      success: (res) => {
        if (res.data.success) {
          this.setData({
            userStats: res.data.data.user_info,
            apiTokens: res.data.data.tokens || []
          });
        }
      }
    });
  },

  // 创建新的API Token
  createNewToken() {
    wx.showModal({
      title: '创建API Token',
      content: '请输入Token名称',
      editable: true,
      success: (res) => {
        if (res.confirm && res.content) {
          this.doCreateToken(res.content);
        }
      }
    });
  },

  // 执行创建Token
  doCreateToken(tokenName) {
    const externalUserId = wx.getStorageSync('externalUserId');

    wx.request({
      url: 'https://your-backend.com/api/user/create-token',
      method: 'POST',
      data: {
        external_user_id: externalUserId,
        token_name: tokenName,
        expires_in_days: 90
      },
      success: (res) => {
        if (res.data.success) {
          wx.showToast({ title: 'Token创建成功', icon: 'success' });
          this.loadUserProfile(); // 刷新页面
        }
      }
    });
  },

  // 查看使用统计
  viewUsageStats() {
    wx.navigateTo({
      url: '/pages/usage/usage'
    });
  }
});
```

## 错误处理和最佳实践

### 错误处理策略

#### 1. 网络错误处理
```javascript
// utils/request.js
const request = (options) => {
  return new Promise((resolve, reject) => {
    wx.request({
      ...options,
      success: (res) => {
        if (res.statusCode === 200) {
          resolve(res.data);
        } else {
          reject(new Error(`HTTP ${res.statusCode}: ${res.data?.message || '请求失败'}`));
        }
      },
      fail: (err) => {
        console.error('请求失败:', err);
        reject(new Error('网络连接失败'));
      }
    });
  });
};
```

#### 2. 用户状态管理
```javascript
// utils/userManager.js
class UserManager {
  static isLoggedIn() {
    const externalUserId = wx.getStorageSync('externalUserId');
    return !!externalUserId;
  }

  static getUserId() {
    return wx.getStorageSync('externalUserId');
  }

  static logout() {
    wx.removeStorageSync('externalUserId');
    wx.removeStorageSync('userToken');
    wx.reLaunch({
      url: '/pages/login/login'
    });
  }

  static async checkUserStatus() {
    if (!this.isLoggedIn()) {
      wx.reLaunch({
        url: '/pages/login/login'
      });
      return false;
    }
    return true;
  }
}

module.exports = UserManager;
```

### 最佳实践

#### 1. 安全考虑
- **不要在小程序中存储敏感信息**（如AppSecret）
- **使用HTTPS加密所有API通信**
- **实施适当的认证和授权机制**
- **定期更新API Token**

#### 2. 性能优化
- **缓存用户信息减少API调用**
- **实施请求防抖避免重复调用**
- **使用分页加载大量数据**

#### 3. 用户体验
- **提供清晰的错误提示**
- **实施加载状态显示**
- **支持离线功能（如缓存历史对话）**

## 部署和监控

### 部署清单
- [ ] 微信小程序审核通过
- [ ] 后端服务部署到生产环境
- [ ] New API系统配置完成
- [ ] 支付功能测试通过
- [ ] 用户数据同步测试通过

### 监控指标
- 用户注册成功率
- 支付成功率
- LLM API调用成功率
- 用户活跃度统计
- 系统错误率监控

## 常见问题和解决方案

### Q1: 用户同步失败
**可能原因**: OpenID获取失败、网络问题、参数验证错误
**解决方案**: 检查微信配置、验证网络连接、确认参数格式

### Q2: 支付回调处理异常
**可能原因**: 签名验证失败、订单状态不一致
**解决方案**: 验证微信支付配置、实施幂等性处理

### Q3: LLM API调用失败
**可能原因**: Token过期、余额不足、模型不可用
**解决方案**: 刷新Token、检查用户余额、选择可用模型

---

**文档版本**: v1.0
**最后更新**: 2025-09-16
**适用版本**: New API v2.1+

如有疑问，请参考 [外部用户API文档](./external-user-api.md) 或 [curl测试指南](./curl-testing-guide.md)。