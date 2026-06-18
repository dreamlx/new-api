#!/usr/bin/env python3
"""
快速回归测试：验证代码评审修复

用法：
  python scripts/test-code-review-fixes.py --token sk-xxx --base-url http://localhost:3000
"""

import argparse
import json
import sys
import urllib.request
import urllib.error

GREEN = "\033[0;32m"
RED = "\033[0;31m"
YELLOW = "\033[1;33m"
CYAN = "\033[0;36m"
BOLD = "\033[1m"
NC = "\033[0m"

PASSED = 0
FAILED = 0

def pass_(msg):
    global PASSED
    PASSED += 1
    print(f"  {GREEN}✓ PASS{NC} {msg}")

def fail_(msg):
    global FAILED
    FAILED += 1
    print(f"  {RED}✗ FAIL{NC} {msg}")

def section(title):
    print(f"\n{BOLD}{CYAN}▌ {title}{NC}")

def http_post(url, token, body_dict):
    data = json.dumps(body_dict).encode("utf-8")
    req = urllib.request.Request(url, data=data, method="POST")
    req.add_header("Authorization", f"Bearer {token}")
    req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            return resp.status, resp.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", errors="replace") if e.fp else ""
        return e.code, body
    except Exception as e:
        return 0, str(e)

def main():
    parser = argparse.ArgumentParser(description="代码评审修复回归测试")
    parser.add_argument("--token", required=True, help="API Token")
    parser.add_argument("--base-url", default="http://localhost:3000", help="服务地址")
    args = parser.parse_args()

    token = args.token
    base_url = args.base_url

    print(f"\n{BOLD}════════════════════════════════════════{NC}")
    print(f"{BOLD} 代码评审修复回归测试{NC}")
    print(f"{BOLD}════════════════════════════════════════{NC}")
    print(f"  BASE_URL: {base_url}")

    # ================================================================
    # Test 1: Fast + 1080p 应该被拦截（修复 #2）
    # ================================================================
    section("Test 1: Fast + 1080p 拦截验证")

    code, body = http_post(f"{base_url}/v1/video/generations", token, {
        "model": "dreamina-seedance-2-0-fast-260128",
        "prompt": "A sunset over mountains",
        "duration": 5,
        "size": "1080p"  # Fast 模型不支持 1080p
    })

    if code == 400:
        data = json.loads(body) if body else {}
        if data.get("code") == "invalid_resolution":
            pass_("Fast + 1080p 正确返回 400 invalid_resolution")
        else:
            fail_(f"Fast + 1080p 返回 400 但错误码不对: {data.get('code')}")
    else:
        fail_(f"Fast + 1080p 应该返回 400，实际返回 {code}")
        print(f"    响应: {body[:200]}")

    # ================================================================
    # Test 2: Fast + 720p 应该被接受
    # ================================================================
    section("Test 2: Fast + 720p 正常提交验证")

    code, body = http_post(f"{base_url}/v1/video/generations", token, {
        "model": "dreamina-seedance-2-0-fast-260128",
        "prompt": "A sunset over mountains",
        "duration": 5,
        "size": "720p"  # Fast 模型支持 720p
    })

    if code == 200:
        data = json.loads(body) if body else {}
        if data.get("task_id"):
            pass_("Fast + 720p 正确返回 200 并获得 task_id")
        else:
            fail_("Fast + 720p 返回 200 但缺少 task_id")
    else:
        fail_(f"Fast + 720p 应该返回 200，实际返回 {code}")
        print(f"    响应: {body[:200]}")

    # ================================================================
    # Test 3: Main + 1080p 应该被接受
    # ================================================================
    section("Test 3: Main + 1080p 正常提交验证")

    code, body = http_post(f"{base_url}/v1/video/generations", token, {
        "model": "dreamina-seedance-2-0-260128",
        "prompt": "A sunset over mountains",
        "duration": 5,
        "size": "1080p"  # Main 模型支持 1080p
    })

    if code == 200:
        data = json.loads(body) if body else {}
        if data.get("task_id"):
            pass_("Main + 1080p 正确返回 200 并获得 task_id")
        else:
            fail_("Main + 1080p 返回 200 但缺少 task_id")
    else:
        fail_(f"Main + 1080p 应该返回 200，实际返回 {code}")
        print(f"    响应: {body[:200]}")

    # ================================================================
    # Test 4: Volcano 格式 Fast + 1080p 也应该被拦截
    # ================================================================
    section("Test 4: Volcano 格式 Fast + 1080p 拦截验证")

    code, body = http_post(f"{base_url}/api/v3/contents/generations/tasks", token, {
        "model": "dreamina-seedance-2-0-fast-260128",
        "content": [
            {"type": "text", "text": "A sunset over mountains"}
        ],
        "resolution": "1080p",  # Fast 模型不支持 1080p
        "duration": 5
    })

    if code == 400:
        data = json.loads(body) if body else {}
        if data.get("code") == "invalid_resolution":
            pass_("Volcano Fast + 1080p 正确返回 400 invalid_resolution")
        else:
            fail_(f"Volcano Fast + 1080p 返回 400 但错误码不对: {data.get('code')}")
    else:
        fail_(f"Volcano Fast + 1080p 应该返回 400，实际返回 {code}")
        print(f"    响应: {body[:200]}")

    # ================================================================
    # Test 5: 高级参数透传仍然正常工作（确保 ParseSubmitResponse 修复无回归）
    # ================================================================
    section("Test 5: 高级参数透传回归测试")

    code, body = http_post(f"{base_url}/api/v3/contents/generations/tasks", token, {
        "model": "dreamina-seedance-2-0-260128",
        "content": [
            {"type": "text", "text": "A vertical video of a cat"}
        ],
        "resolution": "720p",
        "duration": 5,
        "ratio": "9:16",  # 高级参数
        "watermark": False  # 高级参数
    })

    if code == 200:
        data = json.loads(body) if body else {}
        if data.get("task_id"):
            pass_("高级参数透传请求正确返回 200 并获得 task_id")
        else:
            fail_("高级参数透传返回 200 但缺少 task_id")
    else:
        fail_(f"高级参数透传应该返回 200，实际返回 {code}")
        print(f"    响应: {body[:200]}")

    # ================================================================
    # 汇总
    # ================================================================
    print(f"\n{BOLD}════════════════════════════════════════{NC}")
    print(f"{BOLD} 回归测试结果汇总{NC}")
    print(f"{BOLD}════════════════════════════════════════{NC}")
    print(f"  总计: {PASSED + FAILED}  {GREEN}通过: {PASSED}{NC}  {RED}失败: {FAILED}{NC}")
    print()

    if FAILED == 0:
        print(f"{GREEN}✅ 全部回归测试通过{NC}")
        sys.exit(0)
    else:
        print(f"{RED}❌ 有 {FAILED} 个测试失败{NC}")
        sys.exit(1)

if __name__ == "__main__":
    main()
