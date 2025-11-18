#!/usr/bin/env python3
"""
CEC回调服务器模拟脚本
用于测试New API的Token消费回调功能

运行方式：
  python3 cec_callback_server.py

然后在另一个终端执行测试脚本：
  bash scripts/test-callback-feature.sh
"""

from flask import Flask, request, jsonify
import hmac
import hashlib
import json
from datetime import datetime

app = Flask(__name__)

# Token密钥配置（模拟CEC维护的密钥表）
TOKEN_SECRETS = {
    # token_id -> secret
    # 运行测试脚本时会动态添加
}

# 存储接收到的回调记录
callback_records = []


def verify_signature(request_body: bytes, signature: str, secret: str) -> bool:
    """验证HMAC-SHA256签名"""
    if not signature or not secret:
        return False

    expected_signature = hmac.new(
        secret.encode(),
        request_body,
        hashlib.sha256
    ).hexdigest()

    return hmac.compare_digest(expected_signature, signature)


@app.route('/api/consume-notify', methods=['POST'])
def consume_notify():
    """接收New API的消费回调"""
    print("\n" + "="*60)
    print("📩 收到回调通知")
    print("="*60)

    # 打印原始请求信息
    print(f"时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}")
    print(f"Content-Type: {request.headers.get('Content-Type')}")
    print(f"User-Agent: {request.headers.get('User-Agent')}")

    # 提取回调数据
    data = request.json
    if not data:
        print("❌ 错误: 请求体为空")
        return jsonify({'error': 'Empty request body'}), 400

    # 打印核心信息
    print(f"\n📊 消费详情:")
    print(f"  Event: {data.get('event')}")
    print(f"  External User ID: {data.get('external_user_id')}")
    print(f"  Token ID: {data.get('token_id')}")
    print(f"  Token Key: {data.get('token_key')}")
    print(f"  Token Name: {data.get('token_name')}")
    print(f"  Model: {data.get('model')}")
    print(f"  Prompt Tokens: {data.get('prompt_tokens')}")
    print(f"  Completion Tokens: {data.get('completion_tokens')}")
    print(f"  Quota Consumed: {data.get('quota_consumed')}")
    print(f"  Amount USD: ${data.get('amount_usd'):.4f}")
    print(f"  Log ID: {data.get('log_id')}")
    print(f"  Request ID: {data.get('request_id')}")

    # 验证签名（如果配置了secret）
    signature = request.headers.get('X-Callback-Signature')
    token_id = data.get('token_id')

    if signature:
        print(f"\n🔐 签名验证:")
        print(f"  收到签名: {signature[:16]}...")

        # 获取该Token的密钥
        secret = TOKEN_SECRETS.get(token_id)
        if secret:
            if verify_signature(request.data, signature, secret):
                print(f"  ✅ 签名验证通过")
            else:
                print(f"  ❌ 签名验证失败！")
                return jsonify({'error': 'Invalid signature'}), 401
        else:
            print(f"  ⚠️  未配置Token密钥，跳过验证")
    else:
        print(f"\n⚠️  未收到签名，跳过验证")

    # 保存回调记录
    callback_records.append({
        'timestamp': datetime.now().isoformat(),
        'data': data
    })

    print(f"\n✅ 回调处理完成，已保存记录")
    print("="*60)

    # 返回成功响应
    return jsonify({'success': True}), 200


@app.route('/api/stats', methods=['GET'])
def get_stats():
    """查询统计信息"""
    user_id = request.args.get('user_id')
    token_key = request.args.get('token_key')

    filtered_records = callback_records

    # 按用户过滤
    if user_id:
        filtered_records = [r for r in filtered_records if r['data'].get('external_user_id') == user_id]

    # 按Token过滤
    if token_key:
        filtered_records = [r for r in filtered_records if r['data'].get('token_key') == token_key]

    # 统计
    total_records = len(filtered_records)
    total_cost = sum(r['data'].get('amount_usd', 0) for r in filtered_records)
    total_tokens = sum(
        r['data'].get('prompt_tokens', 0) + r['data'].get('completion_tokens', 0)
        for r in filtered_records
    )

    return jsonify({
        'total_records': total_records,
        'total_cost_usd': round(total_cost, 4),
        'total_tokens': total_tokens,
        'records': filtered_records
    })


@app.route('/api/records', methods=['GET'])
def get_records():
    """查看所有回调记录"""
    return jsonify({
        'total': len(callback_records),
        'records': callback_records
    })


@app.route('/api/clear', methods=['POST'])
def clear_records():
    """清空回调记录"""
    global callback_records
    count = len(callback_records)
    callback_records = []
    return jsonify({
        'success': True,
        'cleared': count
    })


@app.route('/api/config/secret', methods=['POST'])
def add_secret():
    """动态添加Token密钥配置"""
    data = request.json
    token_id = data.get('token_id')
    secret = data.get('secret')

    if not token_id or not secret:
        return jsonify({'error': 'token_id and secret required'}), 400

    TOKEN_SECRETS[token_id] = secret
    return jsonify({
        'success': True,
        'token_id': token_id,
        'message': f'Secret added for token {token_id}'
    })


@app.route('/', methods=['GET'])
def index():
    """首页"""
    return """
    <h1>CEC回调服务器模拟</h1>
    <p>用于测试New API的Token消费回调功能</p>
    <h2>API端点：</h2>
    <ul>
        <li>POST /api/consume-notify - 接收回调通知</li>
        <li>GET /api/stats?user_id=xxx&token_key=xxx - 查询统计</li>
        <li>GET /api/records - 查看所有回调记录</li>
        <li>POST /api/clear - 清空回调记录</li>
        <li>POST /api/config/secret - 添加Token密钥</li>
    </ul>
    <h2>当前统计：</h2>
    <ul>
        <li>总回调记录数: """ + str(len(callback_records)) + """</li>
        <li>已配置Token密钥: """ + str(len(TOKEN_SECRETS)) + """</li>
    </ul>
    """


if __name__ == '__main__':
    print("\n" + "="*60)
    print("🚀 CEC回调服务器启动")
    print("="*60)
    print(f"监听地址: http://0.0.0.0:5000")
    print(f"回调接口: http://localhost:5000/api/consume-notify")
    print("\n可用API端点:")
    print("  POST /api/consume-notify - 接收回调")
    print("  GET  /api/stats - 查询统计")
    print("  GET  /api/records - 查看记录")
    print("  POST /api/clear - 清空记录")
    print("  POST /api/config/secret - 添加密钥")
    print("\n按Ctrl+C停止服务器")
    print("="*60 + "\n")

    app.run(host='0.0.0.0', port=5000, debug=False)
