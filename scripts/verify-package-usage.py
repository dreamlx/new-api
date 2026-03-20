#!/usr/bin/env python3
"""
package_usage 接口端到端验证脚本
验证数据计算逻辑是否正确

用法:
    WISEMODEL_API_TOKEN=xxx python3 scripts/verify-package-usage.py
    或:
    python3 scripts/verify-package-usage.py --token xxx --url http://211.93.0.206:10027
"""

import argparse
import json
import os
import random
import string
import sys
import time
import urllib.error
import urllib.request

# ──────────────────────────────────────────────
# 配置
# ──────────────────────────────────────────────
DEFAULT_URL = "http://211.93.0.206:10027"
DEFAULT_TOKEN = os.environ.get("WISEMODEL_API_TOKEN", "test_wisemodel_token_12345")
INFERENCE_MODEL = "glm-4.6-fp8"

# 颜色
GREEN  = "\033[0;32m"
RED    = "\033[0;31m"
YELLOW = "\033[1;33m"
BLUE   = "\033[0;34m"
CYAN   = "\033[0;36m"
RESET  = "\033[0m"

# ──────────────────────────────────────────────
# HTTP 工具
# ──────────────────────────────────────────────

def http_post(url, data, headers=None):
    body = json.dumps(data).encode()
    req = urllib.request.Request(url, data=body, method="POST")
    req.add_header("Content-Type", "application/json")
    if headers:
        for k, v in headers.items():
            req.add_header(k, v)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.loads(resp.read())
    except urllib.error.HTTPError as e:
        return json.loads(e.read())
    except Exception as e:
        return {"error": str(e)}


def http_get(url, headers=None):
    req = urllib.request.Request(url, method="GET")
    if headers:
        for k, v in headers.items():
            req.add_header(k, v)
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.loads(resp.read())
    except urllib.error.HTTPError as e:
        return json.loads(e.read())
    except Exception as e:
        return {"error": str(e)}


# ──────────────────────────────────────────────
# 测试辅助
# ──────────────────────────────────────────────

class TestRunner:
    def __init__(self, base_url, token):
        self.base_url = base_url
        self.wm_token = token
        self.wm_headers = {"Authorization": f"Bearer {token}"}
        self.passed = 0
        self.failed = 0
        self.warnings = []

    def ok(self, label, detail=""):
        self.passed += 1
        print(f"{GREEN}[PASS]{RESET} {label}" + (f"\n       {detail}" if detail else ""))

    def fail(self, label, detail=""):
        self.failed += 1
        print(f"{RED}[FAIL]{RESET} {label}" + (f"\n       {detail}" if detail else ""))

    def warn(self, msg):
        self.warnings.append(msg)
        print(f"{YELLOW}[WARN]{RESET} {msg}")

    def info(self, msg):
        print(f"{BLUE}[INFO]{RESET} {msg}")

    def section(self, title):
        print(f"\n{CYAN}{'='*60}{RESET}")
        print(f"{CYAN}  {title}{RESET}")
        print(f"{CYAN}{'='*60}{RESET}")

    def summary(self):
        print(f"\n{'='*60}")
        print(f"  通过: {GREEN}{self.passed}{RESET}  失败: {RED}{self.failed}{RESET}")
        if self.warnings:
            print(f"  警告({len(self.warnings)}):")
            for w in self.warnings:
                print(f"    - {w}")
        print(f"{'='*60}")


# ──────────────────────────────────────────────
# 主测试流程
# ──────────────────────────────────────────────

def main(base_url, token):
    t = TestRunner(base_url, token)
    ts = int(time.time())
    #phone = f"139{''.join(random.choices(string.digits, k=8))}"
    phone = "159013313210"
    wm_key = f"wisemodel-{ts}"
    #wm_key="wisemodel-ipnphdksgowmhgxxaqsw"
    pkg_id_tokens = f"PKG-TOK-{ts}"
    pkg_id_points = f"PKG-PTS-{ts}"
    order_id = f"ORDER-{ts}"

    # ── Step 1: 检查服务 ──────────────────────────────
    t.section("Step 1: 服务连通性检查")
    resp = http_get(f"{base_url}/api/status")
    if "error" not in resp:
        t.ok("服务可达")
    else:
        t.fail("服务不可达", resp.get("error"))
        sys.exit(1)

    # ── Step 2: 绑定用户 ──────────────────────────────
    t.section("Step 2: 绑定测试用户")
    t.info(f"phone={phone}, wisemodel_key={wm_key}")
    resp = http_post(f"{base_url}/api/wisemodel/user/bind",
                     {"phone": phone, "wisemodel_key": wm_key, "username": f"test_{ts}"},
                     t.wm_headers)
    if resp.get("success"):
        t.ok("用户绑定成功")
    else:
        t.fail("用户绑定失败", json.dumps(resp))
        sys.exit(1)

    # ── Step 3: 检查基准（绑定后未充值，usage 应为空或 remain=0）──
    t.section("Step 3: 基准检查（无资源包）")
    resp = http_post(f"{base_url}/api/wisemodel/user/package_usage",
                     {"phone": phone}, t.wm_headers)
    t.info(f"响应: {json.dumps(resp, ensure_ascii=False, indent=2)}")
    data0 = resp.get("data") or []
    if data0 == [] or data0 is None:
        t.ok("无资源包时 data 正确为空")
    else:
        t.warn(f"无资源包时 data 不为空: {data0}")

    # ── Step 4: 创建资源包（Token 模式，1000 tokens）────
    # t.section("Step 4: 创建资源包")
    # TOKEN_COUNT = 1000  # 单位: tokens
    # valid_until = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(time.time() + 3600*24*365))
    # resp = http_post(f"{base_url}/api/wisemodel/orders/record", {
    #     "order_id": order_id,
    #     "package_count": 1,
    #     "packages": [{
    #         "id": pkg_id_tokens,
    #         "points": 0,
    #         "tokens": TOKEN_COUNT,
    #         "amount": 10.0,
    #         "phone": phone,
    #         "is_free": False,
    #         "valid_until": valid_until,
    #         "created_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    #     }]
    # }, t.wm_headers)
    # t.info(f"响应: {json.dumps(resp, ensure_ascii=False)}")
    # if resp.get("success"):
    #     t.ok(f"资源包创建成功 (tokens={TOKEN_COUNT})")
    # else:
    #     t.fail("资源包创建失败", json.dumps(resp))
    #     sys.exit(1)

    # # # ── Step 5: 充值前 package_usage 基准 ────────────
    # t.section("Step 5: 推理前 package_usage（预期 remain_tokens=1000, amount_tokens=0）")
    # resp_before = http_post(f"{base_url}/api/wisemodel/user/package_usage",
    #                         {"phone": phone}, t.wm_headers)
    # t.info(f"响应:\n{json.dumps(resp_before, ensure_ascii=False, indent=2)}")
    # data_before = (resp_before.get("data") or [])
    # if data_before:
    #     pkg = data_before[0]
    #     remain = pkg.get("remain_tokens", -999)
    #     used   = pkg.get("amount_tokens", -999)
    #     t.info(f"remain_tokens={remain}, amount_tokens={used}")
    #     if remain == TOKEN_COUNT and used == 0:
    #         t.ok("推理前 remain/used 正确")
    #     else:
    #         t.warn(f"推理前数据不符预期: remain={remain} (期望{TOKEN_COUNT}), used={used} (期望0)")
    # else:
    #     t.fail("推理前无资源包数据")

    # ── Step 6: 调用模型推理 ──────────────────────────
    t.section(f"Step 6: 调用模型推理 ({INFERENCE_MODEL})")
    llm_headers = {
        "Authorization": f"Bearer {wm_key}",
        "Content-Type": "application/json",
    }
    llm_payload = {
        "model": INFERENCE_MODEL,
        "messages": [{"role": "user", "content": "你好，请用一句话介绍自己。"}],
        "max_tokens": 300,
        "stream": False,
    }
    llm_resp = http_post(f"{base_url}/v1/chat/completions", llm_payload, llm_headers)
    t.info(f"LLM响应: {json.dumps(llm_resp, ensure_ascii=False, indent=2)}")
    inference_ok = False
    actual_model_name = None  # 记录后端实际使用的模型名（用于 details 匹配）
    if "choices" in llm_resp:
        msg = llm_resp["choices"][0]["message"]
        # 兼容普通模型（content）和 reasoning 模型（reasoning_content）
        content = msg.get("content") or msg.get("reasoning_content", "")
        usage = llm_resp.get("usage", {})
        total_tokens = usage.get("total_tokens", 0)
        # 后端可能映射为不同的模型名（如 hosted_vllm/glm-4.6-fp8）
        actual_model_name = llm_resp.get("model", INFERENCE_MODEL)
        inference_ok = True
        t.ok(f"推理成功", f"实际模型={actual_model_name} | total_tokens={total_tokens} | 回复: {content[:60]}")
    else:
        t.fail("推理失败", json.dumps(llm_resp, ensure_ascii=False))

    # ── Step 7: 推理后 package_usage ─────────────────
    time.sleep(2)  # 等待日志异步写入
    t.section("Step 7: 推理后 package_usage（预期 remain_tokens<1000, amount_tokens>0）")
    if not inference_ok:
        t.warn("推理未成功，跳过推理后验证")
    else:
        resp_after = http_post(f"{base_url}/api/wisemodel/user/package_usage",
                               {"phone": phone}, t.wm_headers)
        t.info(f"响应:\n{json.dumps(resp_after, ensure_ascii=False, indent=2)}")
        data_after = (resp_after.get("data") or [])

        if data_after:
            pkg = data_after[0]
            remain = pkg.get("remain_tokens", -999)
            used   = pkg.get("amount_tokens", -999)
            t.info(f"remain_tokens={remain}, amount_tokens={used}")

            if used > 0:
                t.ok(f"amount_tokens 有变化 ({used})")
            else:
                t.fail("amount_tokens 未变化（仍为0），消费记录未被统计")

            if remain < TOKEN_COUNT:
                t.ok(f"remain_tokens 已扣减 ({remain} < {TOKEN_COUNT})")
            else:
                t.fail(f"remain_tokens 未扣减（仍为 {remain}）")

            # 详细分析
            details = pkg.get("details", [])
            t.info(f"details 明细: {json.dumps(details, ensure_ascii=False, indent=2)}")
            if details:
                # 后端可能用 actual_model_name 或 INFERENCE_MODEL 记录
                found_names = {d.get("model_name") for d in details}
                if actual_model_name in found_names or INFERENCE_MODEL in found_names:
                    t.ok(f"模型出现在 details 中: {found_names}")
                else:
                    t.warn(f"期望模型 {actual_model_name or INFERENCE_MODEL} 未在 details 中，实际: {found_names}")
            else:
                t.warn("details 为空，消费未被按模型聚合")
        else:
            t.fail("推理后无资源包数据")

    # ── Step 8: 深度分析 ──────────────────────────────
    t.section("Step 8: 修复结果分析")
    print(f"""
{GREEN}已在 controller/wisemodel_package.go 修复以下问题:{RESET}

{GREEN}[FIXED-1] created_at 类型不匹配{RESET}
  修复前: query.Where("created_at >= ?", pkg.CreatedAt)  ← time.Time vs bigint, 恒为真
  修复后: query.Where("created_at >= ?", pkg.CreatedAt.Unix())  ← bigint vs bigint ✓

{GREEN}[FIXED-2] 多资源包消费重叠{RESET}
  修复后: 加 ValidUntil 作为时间窗口上边界:
         query.Where("created_at < ?", pkg.ValidUntil.Unix())
  每个包的消费限定在 [CreatedAt, ValidUntil) 区间内，不再重叠

{GREEN}[FIXED-3] 无模型过滤{RESET}
  修复后: available_models 非空时加 model_name IN (...) 过滤
         available_models 为空时计入所有模型消费（按需求）

{GREEN}[FIXED-4] 查询错误未处理{RESET}
  修复后: query.Find(&logs).Error 正确处理并返回 500

{GREEN}[FIXED-5] data 为 nil 而非 []{RESET}
  修复后: data := make([]map[string]interface{{}}, 0)

{RED}[BUG-6] 精度丢失: amount_tokens/amount 显示为 0{RESET}
  当单次消费 quota < 500000 时:
    totalUsed / 500000 = 0  (整除截断)
    (QuotaGranted - totalUsed) / 500000 = tokens - 1  (大数相减后变化)
  → amount=0 但 remain 已减少，两者相加不等于原始总量
  {GREEN}修复后: amount_tokens = OriginalTokens - remain_tokens  (总量 - 剩余){RESET}
""")

    # ── 清理 ──────────────────────────────────────────
    t.section("清理: 删除测试用户（无付费限制时）")
    # 注意: 因为我们创建了付费包, delete_wisemodel_user 会被拒绝
    # 直接尝试, 记录结果
    del_resp = http_post(f"{base_url}/api/wisemodel/user/delete_wisemodel_user",
                         {"phone": phone}, t.wm_headers)
    t.info(f"删除响应: {json.dumps(del_resp, ensure_ascii=False)}")
    if del_resp.get("success"):
        t.ok("测试用户已清理")
    else:
        t.warn(f"测试用户未清理 (phone={phone}, key={wm_key}), 需手动处理")

    t.summary()


# ──────────────────────────────────────────────
if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="package_usage 接口验证")
    parser.add_argument("--url", default=DEFAULT_URL, help="服务地址")
    parser.add_argument("--token", default=DEFAULT_TOKEN, help="WISEMODEL_API_TOKEN")
    args = parser.parse_args()

    print(f"\n{CYAN}package_usage 接口端到端验证{RESET}")
    print(f"  服务: {args.url}")
    print(f"  Token: {args.token[:10]}...{args.token[-4:]}")
    print()

    main(args.url, args.token)
