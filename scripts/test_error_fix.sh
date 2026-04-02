#!/bin/bash

# 测试错误修复脚本
# 模拟类似roo code的请求，包含可能导致智谱解析错误的参数

echo "🧪 测试GLM错误修复"
echo "=================="

# 测试请求 - 包含智谱可能不支持的参数
curl -X POST http://localhost:3000/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer test_token_here" \
  -d '{
    "model": "glm-4",
    "messages": [
      {
        "role": "user", 
        "content": "你好"
      }
    ],
    "temperature": 0.7,
    "max_tokens": 100,
    "stream": false,
    "tools": [
      {
        "type": "function",
        "function": {
          "name": "get_weather",
          "description": "获取天气信息"
        }
      }
    ],
    "tool_choice": "auto",
    "response_format": {
      "type": "json_object"
    }
  }' \
  -w "\n状态码: %{http_code}\n响应时间: %{time_total}s\n"

echo ""
echo "📋 检查是否还有panic错误（应该返回友好的错误信息而不是500 panic）"