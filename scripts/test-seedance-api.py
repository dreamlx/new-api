#!/usr/bin/env python3
"""
Seedance 渠道全接口测试脚本
测试两种入口格式（OpenAI Video 风格 + 火山官方兼容风格）
覆盖：文生视频、图生视频、视频续写、音频驱动、高级参数透传

用法 (Windows Python):
  # 快速模式：只测提交（不轮询等结果），几秒完成
  python scripts/test-seedance-api.py --token sk-xxx --quick

  # 全量模式：提交+轮询，需要数分钟
  python scripts/test-seedance-api.py --token sk-xxx

  # 使用本地图片
  python scripts/test-seedance-api.py --token sk-xxx --quick --image D:/Work/Document/可达鸭.jpeg

  # 指定测试用例（逗号分隔）
  python scripts/test-seedance-api.py --token sk-xxx --quick --only 1,5,11,12
"""

import argparse
import base64
import json
import os
import sys
import time
import urllib.request
import urllib.error

# ── 颜色 ──
GREEN = "\033[0;32m"
RED = "\033[0;31m"
YELLOW = "\033[1;33m"
CYAN = "\033[0;36m"
BOLD = "\033[1m"
NC = "\033[0m"

TOTAL = 0
PASSED = 0
FAILED = 0
SKIPPED = 0


def pass_(msg):
    global TOTAL, PASSED
    PASSED += 1
    TOTAL += 1
    print(f"  {GREEN}✓ PASS{NC} {msg}")


def fail_(msg):
    global TOTAL, FAILED
    FAILED += 1
    TOTAL += 1
    print(f"  {RED}✗ FAIL{NC} {msg}")


def skip_(msg):
    global SKIPPED
    SKIPPED += 1
    print(f"  {YELLOW}⊘ SKIP{NC} {msg}")


def info(msg):
    print(f"  {CYAN}ℹ{NC} {msg}")


def section(title):
    print(f"\n{BOLD}{CYAN}▌ {title}{NC}")


def should_run(case_id, only_set):
    """检查指定编号的用例是否应该运行"""
    if not only_set:
        return True
    return case_id in only_set


# ── HTTP 工具 ──

def http_get(url, token):
    req = urllib.request.Request(url)
    req.add_header("Authorization", f"Bearer {token}")
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return resp.status, resp.read().decode("utf-8", errors="replace")
    except urllib.error.HTTPError as e:
        body = e.read().decode("utf-8", errors="replace") if e.fp else ""
        return e.code, body
    except Exception as e:
        return 0, str(e)


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


def parse_json(text):
    try:
        return json.loads(text)
    except Exception:
        return None


def json_get(obj, *keys, default=""):
    """安全地从嵌套 dict 中取值"""
    v = obj
    for k in keys:
        if isinstance(v, dict) and k in v:
            v = v[k]
        else:
            return default
    if v is None:
        return default
    return v


# ── 图片转 base64 data URL ──

def file_to_data_url(filepath):
    if not filepath or not os.path.isfile(filepath):
        return ""
    ext = os.path.splitext(filepath)[1].lower().lstrip(".")
    mime_map = {"png": "image/png", "jpg": "image/jpeg", "jpeg": "image/jpeg",
                "webp": "image/webp", "gif": "image/gif"}
    mime = mime_map.get(ext, "image/png")
    with open(filepath, "rb") as f:
        b64 = base64.b64encode(f.read()).decode("ascii")
    return f"data:{mime};base64,{b64}"


# ── 轮询函数 ──

def poll_task_openai(base_url, token, task_id, label, poll_interval, max_polls):
    info(f"[{label}] 开始轮询 task_id={task_id} (OpenAI 风格)")
    for i in range(1, max_polls + 1):
        code, body = http_get(f"{base_url}/v1/video/generations/{task_id}", token)
        data = parse_json(body)
        status = json_get(data, "data", "status") or json_get(data, "status")
        info(f"[{label}] 轮询 #{i}: status={status}")
        if status in ("SUCCESS", "succeeded"):
            return data
        elif status in ("FAILURE", "failed"):
            print(f"  {RED}[{label}] 任务失败{NC}")
            print(f"  响应: {body[:500]}")
            return None
        time.sleep(poll_interval)
    print(f"  {RED}[{label}] 超过最大轮询次数 ({max_polls}){NC}")
    return None


def poll_task_volcano(base_url, token, task_id, label, poll_interval, max_polls):
    info(f"[{label}] 开始轮询 task_id={task_id} (火山兼容风格)")
    for i in range(1, max_polls + 1):
        code, body = http_get(f"{base_url}/api/v3/contents/generations/tasks/{task_id}", token)
        data = parse_json(body)
        status = json_get(data, "status")
        info(f"[{label}] 轮询 #{i}: status={status}")
        if status == "succeeded":
            return data
        elif status == "failed":
            print(f"  {RED}[{label}] 任务失败{NC}")
            print(f"  响应: {body[:500]}")
            return None
        time.sleep(poll_interval)
    print(f"  {RED}[{label}] 超过最大轮询次数 ({max_polls}){NC}")
    return None


# ── 提交 + 轮询 + 验证 ──

def submit_and_poll_openai(base_url, token, label, body_dict, poll_interval, max_polls, quick=False):
    info(f"[{label}] 提交任务...")
    code, resp_body = http_post(f"{base_url}/v1/video/generations", token, body_dict)
    data = parse_json(resp_body)

    if data is None:
        fail_(f"[{label}] 提交失败: 无法解析响应 (HTTP {code})")
        print(f"  响应: {resp_body[:300]}")
        return

    task_id = json_get(data, "task_id") or json_get(data, "id")
    if task_id:
        pass_(f"[{label}] 提交成功, task_id={task_id}")
    else:
        fail_(f"[{label}] 提交失败 (HTTP {code})")
        print(f"  响应: {json.dumps(data, ensure_ascii=False, indent=2)[:500]}")
        return

    if quick:
        skip_(f"[{label}] 快速模式，跳过轮询")
        return

    result = poll_task_openai(base_url, token, task_id, label, poll_interval, max_polls)
    if result is None:
        return

    # 检查视频 URL
    video_url = (json_get(result, "data", "data", "content", "video_url")
                 or json_get(result, "data", "result_url")
                 or json_get(result, "data", "data", "result_url"))
    if video_url:
        pass_(f"[{label}] 成功获取视频 URL")
        info(f"[{label}] video_url={video_url[:80]}...")
    else:
        fail_(f"[{label}] 未获取到视频 URL")

    # 检查 usage
    tokens = (json_get(result, "data", "data", "usage", "completion_tokens")
              or json_get(result, "data", "data", "usage", "total_tokens"))
    if tokens and str(tokens) not in ("0", "null", ""):
        pass_(f"[{label}] usage token 数据: {tokens}")
    else:
        fail_(f"[{label}] usage 无 token 数据")

    # 打印完整结果（截断）
    info(f"[{label}] 完整响应:")
    result_str = json.dumps(result, ensure_ascii=False, indent=2)
    for line in result_str.split("\n")[:30]:
        print(f"    {line}")
    if result_str.count("\n") > 30:
        print(f"    ... (共 {result_str.count(chr(10))} 行)")


def submit_and_poll_volcano(base_url, token, label, body_dict, poll_interval, max_polls, quick=False):
    info(f"[{label}] 提交任务...")
    code, resp_body = http_post(f"{base_url}/api/v3/contents/generations/tasks", token, body_dict)
    data = parse_json(resp_body)

    if data is None:
        fail_(f"[{label}] 提交失败: 无法解析响应 (HTTP {code})")
        print(f"  响应: {resp_body[:300]}")
        return

    task_id = json_get(data, "task_id") or json_get(data, "id")
    if task_id:
        pass_(f"[{label}] 提交成功, task_id={task_id}")
    else:
        fail_(f"[{label}] 提交失败 (HTTP {code})")
        print(f"  响应: {json.dumps(data, ensure_ascii=False, indent=2)[:500]}")
        return

    if quick:
        skip_(f"[{label}] 快速模式，跳过轮询")
        return

    result = poll_task_volcano(base_url, token, task_id, label, poll_interval, max_polls)
    if result is None:
        return

    # 检查视频 URL
    video_url = json_get(result, "content", "video_url")
    if video_url:
        pass_(f"[{label}] 成功获取视频 URL")
        info(f"[{label}] video_url={video_url[:80]}...")
    else:
        fail_(f"[{label}] 未获取到视频 URL")

    # 检查 usage
    tokens = (json_get(result, "usage", "completion_tokens")
              or json_get(result, "usage", "total_tokens"))
    if tokens and str(tokens) not in ("0", "null", ""):
        pass_(f"[{label}] usage token 数据: {tokens}")
    else:
        fail_(f"[{label}] usage 无 token 数据")

    # 打印完整结果
    info(f"[{label}] 完整响应:")
    result_str = json.dumps(result, ensure_ascii=False, indent=2)
    for line in result_str.split("\n")[:30]:
        print(f"    {line}")
    if result_str.count("\n") > 30:
        print(f"    ... (共 {result_str.count(chr(10))} 行)")


# ═══════════════════════════════════════════════════════════════
#  主函数
# ═══════════════════════════════════════════════════════════════

def main():
    parser = argparse.ArgumentParser(description="Seedance 渠道全接口测试")
    parser.add_argument("--token", required=True, help="API Token (sk-xxx)")
    parser.add_argument("--base-url", default="http://localhost:3000", help="服务地址")
    parser.add_argument("--model", default="dreamina-seedance-2-0-260128", help="主版本模型名")
    parser.add_argument("--fast-model", default="dreamina-seedance-2-0-fast-260128", help="Fast 版本模型名")
    parser.add_argument("--poll-interval", type=int, default=10, help="轮询间隔秒数")
    parser.add_argument("--max-polls", type=int, default=40, help="最大轮询次数")
    parser.add_argument("--image", default="", help="本地图片文件路径（图生视频用）")
    parser.add_argument("--face", default="", help="本地人脸图片路径（音频驱动用）")
    parser.add_argument("--video-url", default="https://samplelib.com/mp4/sample-5s.mp4", help="公网视频 URL")
    parser.add_argument("--audio-url", default="https://samplelib.com/mp3/sample-3s.mp3", help="公网音频 URL")
    parser.add_argument("--image-url", default="https://www.w3schools.com/html/pic_trulli.jpg", help="公网图片 URL (fallback)")
    parser.add_argument("--face-url", default="https://www.w3schools.com/w3images/avatar2.png", help="公网人脸图片 URL (fallback)")
    parser.add_argument("--quick", action="store_true", help="快速模式：只测提交不轮询，几秒完成")
    parser.add_argument("--only", default="", help="只运行指定用例，逗号分隔编号（如 1,5,11,12）")
    args = parser.parse_args()

    token = args.token
    base_url = args.base_url
    model = args.model
    fast_model = args.fast_model
    poll_interval = args.poll_interval
    max_polls = args.max_polls

    # 准备图片 URL
    image_data_url = file_to_data_url(args.image) if args.image else ""
    if image_data_url:
        info(f"图片已加载: {args.image} ({len(image_data_url)} bytes data URL)")
    else:
        image_data_url = args.image_url
        info(f"图片使用公网 URL: {args.image_url}")

    face_data_url = file_to_data_url(args.face) if args.face else ""
    if face_data_url:
        info(f"人脸图片已加载: {args.face}")
    else:
        face_data_url = args.face_url
        info(f"人脸图片使用公网 URL: {args.face_url}")

    video_url = args.video_url
    audio_url = args.audio_url
    quick = args.quick
    only_set = set()
    if args.only:
        only_set = {x.strip() for x in args.only.split(",") if x.strip()}
    info(f"视频 URL: {video_url}")
    info(f"音频 URL: {audio_url}")
    if quick:
        info("快速模式：只测提交，不轮询结果")

    # ═══════════════════════════════════════════════════════════
    print(f"\n{BOLD}════════════════════════════════════════{NC}")
    print(f"{BOLD} Seedance 渠道全接口测试{NC}")
    print(f"{BOLD}════════════════════════════════════════{NC}")
    print(f"  BASE_URL:   {base_url}")
    print(f"  MODEL:      {model}")
    print(f"  FAST_MODEL: {fast_model}")
    print(f"  POLL:       {poll_interval}s × {max_polls}次")
    print(f"  QUICK:      {'是' if quick else '否'}")
    print(f"  IMAGE:      {args.image or args.image_url}")
    print(f"  VIDEO_URL:  {video_url}")
    print(f"  AUDIO_URL:  {audio_url}")

    # ────────────────────────────────────────────────────
    #  1. OpenAI 风格：文生视频 (t2v)
    # ────────────────────────────────────────────────────
    if should_run("1", only_set):
        section("1. OpenAI 风格 — 文生视频 (t2v)")

        submit_and_poll_openai(base_url, token, "OpenAI-t2v", {
            "model": model,
            "prompt": "A small kitten chasing a butterfly in a sunny garden, cinematic shot",
            "duration": 5,
            "size": "720p"
        }, poll_interval, max_polls, quick)

    # ────────────────────────────────────────────────────
    #  2. OpenAI 风格：图生视频 (i2v)
    # ────────────────────────────────────────────────────
    if should_run("2", only_set):
        section("2. OpenAI 风格 — 图生视频 (i2v)")

        submit_and_poll_openai(base_url, token, "OpenAI-i2v", {
            "model": model,
            "prompt": "这只可达鸭突然觉醒了超能力，双手合十开始蓄力，周围电光闪烁，最后释放出金色的冲击波",
            "duration": 5,
            "size": "720p",
            "images": [image_data_url]
        }, poll_interval, max_polls, quick)

    # ────────────────────────────────────────────────────
    #  3. OpenAI 风格：视频续写 (v2v)
    # ────────────────────────────────────────────────────
    if should_run("3", only_set):
        section("3. OpenAI 风格 — 视频续写 (v2v)")

        submit_and_poll_openai(base_url, token, "OpenAI-v2v", {
            "model": model,
            "prompt": "Continue this video with smooth transition",
            "duration": 5,
            "size": "720p",
            "videos": [video_url]
        }, poll_interval, max_polls, quick)

    # ────────────────────────────────────────────────────
    #  4. OpenAI 风格：音频驱动 (a2v)
    # ────────────────────────────────────────────────────
    if should_run("4", only_set):
        section("4. OpenAI 风格 — 音频驱动 (a2v)")

        submit_and_poll_openai(base_url, token, "OpenAI-a2v", {
            "model": model,
            "prompt": "A person speaking with natural lip sync",
            "duration": 5,
            "size": "720p",
            "images": [face_data_url],
            "audios": [audio_url]
        }, poll_interval, max_polls, quick)

    # ────────────────────────────────────────────────────
    #  5. 火山兼容风格：文生视频 (t2v)
    # ────────────────────────────────────────────────────
    if should_run("5", only_set):
        section("5. 火山兼容风格 — 文生视频 (t2v)")

        submit_and_poll_volcano(base_url, token, "Volcano-t2v", {
            "model": model,
            "content": [
                {"type": "text", "text": "A white horse runs slowly on a grassland, cinematic shot"}
            ],
            "resolution": "720p",
            "duration": 5
        }, poll_interval, max_polls, quick)

    # ────────────────────────────────────────────────────
    #  6. 火山兼容风格：图生视频 (i2v)
    # ────────────────────────────────────────────────────
    if should_run("6", only_set):
        section("6. 火山兼容风格 — 图生视频 (i2v)")

        submit_and_poll_volcano(base_url, token, "Volcano-i2v", {
            "model": model,
            "content": [
                {"type": "text", "text": "这只可达鸭突然觉醒了超能力，双手合十开始蓄力，周围电光闪烁，最后释放出金色的冲击波"},
                {"type": "image_url", "image_url": {"url": image_data_url}, "role": "first_frame"}
            ],
            "resolution": "720p",
            "duration": 5
        }, poll_interval, max_polls, quick)

    # ────────────────────────────────────────────────────
    #  7. 火山兼容风格：视频续写 (v2v)
    # ────────────────────────────────────────────────────
    if should_run("7", only_set):
        section("7. 火山兼容风格 — 视频续写 (v2v)")

        submit_and_poll_volcano(base_url, token, "Volcano-v2v", {
            "model": model,
            "content": [
                {"type": "text", "text": "Continue this video scene naturally"},
                {"type": "video", "video": {"url": video_url}}
            ],
            "resolution": "720p",
            "duration": 5
        }, poll_interval, max_polls, quick)

    # ────────────────────────────────────────────────────
    #  8. 火山兼容风格：音频驱动 (a2v)
    # ────────────────────────────────────────────────────
    if should_run("8", only_set):
        section("8. 火山兼容风格 — 音频驱动 (a2v)")

        submit_and_poll_volcano(base_url, token, "Volcano-a2v", {
            "model": model,
            "content": [
                {"type": "text", "text": "A person speaking naturally"},
                {"type": "image_url", "image_url": {"url": face_data_url}},
                {"type": "audio", "audio": {"url": audio_url}}
            ],
            "resolution": "720p",
            "duration": 5
        }, poll_interval, max_polls, quick)

    # ────────────────────────────────────────────────────
    #  9. Fast 模型文生视频（OpenAI 风格）
    # ────────────────────────────────────────────────────
    if should_run("9", only_set):
        section("9. OpenAI 风格 — Fast 模型文生视频")

        submit_and_poll_openai(base_url, token, "OpenAI-fast-t2v", {
            "model": fast_model,
            "prompt": "A quick shot of a sunset over the ocean",
            "duration": 5,
            "size": "720p"
        }, poll_interval, max_polls, quick)

    # ────────────────────────────────────────────────────
    #  10. 负向测试：prompt 缺失
    # ────────────────────────────────────────────────────
    if should_run("10", only_set):
        section("10. 负向测试 — prompt 缺失")

        code, _ = http_post(f"{base_url}/v1/video/generations", token, {
            "model": model,
            "duration": 5
        })
        if code == 400:
            pass_("prompt 缺失返回 400")
        else:
            fail_(f"prompt 缺失应返回 400，实际={code}")

    # ────────────────────────────────────────────────────
    #  11. OpenAI 风格：高级参数透传（metadata 中的 ratio）
    # ────────────────────────────────────────────────────
    if should_run("11", only_set):
        section("11. OpenAI 风格 — 高级参数透传 (metadata)")

        submit_and_poll_openai(base_url, token, "OpenAI-advanced", {
            "model": model,
            "prompt": "A dog running on the beach, vertical video",
            "duration": 5,
            "size": "720p",
            "metadata": {
                "ratio": "9:16",
                "watermark": False
            }
        }, poll_interval, max_polls, quick)

    # ────────────────────────────────────────────────────
    #  12. 火山兼容风格：高级参数透传（顶层 ratio 等字段）
    # ────────────────────────────────────────────────────
    if should_run("12", only_set):
        section("12. 火山兼容风格 — 高级参数透传 (顶层字段)")

        submit_and_poll_volcano(base_url, token, "Volcano-advanced", {
            "model": model,
            "content": [
                {"type": "text", "text": "A cat playing with yarn, vertical video"}
            ],
            "resolution": "720p",
            "duration": 5,
            "ratio": "9:16",
            "watermark": False
        }, poll_interval, max_polls, quick)

    # ═══════════════════════════════════════════════════════════
    #  汇总
    # ═══════════════════════════════════════════════════════════
    print(f"\n{BOLD}════════════════════════════════════════{NC}")
    print(f"{BOLD} Seedance 测试结果汇总{NC}")
    print(f"{BOLD}════════════════════════════════════════{NC}")
    print(f"  总计: {TOTAL}  {GREEN}通过: {PASSED}{NC}  {RED}失败: {FAILED}{NC}  {YELLOW}跳过: {SKIPPED}{NC}")
    print()

    if FAILED == 0:
        print(f"{GREEN}✅ 全部自动化用例通过{NC}")
    else:
        print(f"{RED}❌ 有 {FAILED} 个用例失败{NC}")
        sys.exit(1)


if __name__ == "__main__":
    main()
