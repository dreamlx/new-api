#!/usr/bin/env bash
# Seedance 渠道全接口测试脚本
#
# ⚠️ 本脚本已被 Python 版本替代，推荐使用：
#   python scripts/test-seedance-api.py --token sk-xxx
#
# 本脚本仍可使用，但 Windows 环境下推荐 Python 版本（零外部依赖）。
#
# 测试两种入口格式（OpenAI Video 风格 + 火山官方兼容风格）
# 覆盖：文生视频、图生视频、视频续写、音频驱动
#
# 用法:
#   TOKEN=sk-xxx bash scripts/test-seedance-api.sh
#
# 环境变量:
#   BASE_URL      - 服务地址，默认 http://localhost:3000
#   TOKEN         - API Token (sk-xxx)，必填
#   MODEL         - 主版本模型名，默认 dreamina-seedance-2-0-260128
#   FAST_MODEL    - Fast 版本模型名，默认 dreamina-seedance-2-0-fast-260128
#   POLL_INTERVAL - 轮询间隔秒数，默认 10
#   MAX_POLLS     - 最大轮询次数，默认 40
#   IMAGE_FILE    - 本地图片文件路径（图生视频用），默认空
#   VIDEO_URL     - 公网视频 URL（视频续写用），默认 https://samplelib.com/mp4/sample-5s.mp4
#   AUDIO_URL     - 公网音频 URL（音频驱动用），默认 https://samplelib.com/mp3/sample-3s.mp3
#   FACE_FILE     - 本地人脸图片路径（音频驱动用），默认空
#
# 注意：图片支持本地文件（自动转 base64 data URL）或公网 URL。
#       视频/音频不支持 base64，必须用公网 URL（已提供默认公网素材）。
#       无需 jq 依赖，使用 python3 解析 JSON。
#
# 运行示例 (PowerShell + WSL):
#   wsl -e bash -c "TOKEN=sk-xxx bash /mnt/d/Work/new-api/scripts/test-seedance-api.sh"
#
#   # 使用本地可达鸭图片
#   wsl -e bash -c "TOKEN=sk-xxx IMAGE_FILE=/mnt/d/Work/Document/可达鸭.jpeg bash /mnt/d/Work/new-api/scripts/test-seedance-api.sh"

set -uo pipefail

BASE_URL="${BASE_URL:-http://localhost:3000}"
TOKEN="${TOKEN:-}"
MODEL="${MODEL:-dreamina-seedance-2-0-260128}"
FAST_MODEL="${FAST_MODEL:-dreamina-seedance-2-0-fast-260128}"
POLL_INTERVAL="${POLL_INTERVAL:-10}"
MAX_POLLS="${MAX_POLLS:-40}"

# 本地文件路径（图片会自动转 base64 data URL）
IMAGE_FILE="${IMAGE_FILE:-}"
FACE_FILE="${FACE_FILE:-}"

# 公网 URL（已提供默认测试素材）
IMAGE_URL="${IMAGE_URL:-https://www.w3schools.com/html/pic_trulli.jpg}"
FACE_URL="${FACE_URL:-https://www.w3schools.com/w3images/avatar2.png}"
VIDEO_URL="${VIDEO_URL:-https://samplelib.com/mp4/sample-5s.mp4}"
AUDIO_URL="${AUDIO_URL:-https://samplelib.com/mp3/sample-3s.mp3}"

# 颜色
GREEN='\033[0;32m'; RED='\033[0;31m'; YELLOW='\033[1;33m'; CYAN='\033[0;36m'; BOLD='\033[1m'; NC='\033[0m'

TOTAL=0; PASSED=0; FAILED=0; SKIPPED=0

pass()  { echo -e "  ${GREEN}✓ PASS${NC} $1"; PASSED=$((PASSED+1)); TOTAL=$((TOTAL+1)); }
fail()  { echo -e "  ${RED}✗ FAIL${NC} $1"; FAILED=$((FAILED+1)); TOTAL=$((TOTAL+1)); }
skip()  { echo -e "  ${YELLOW}⊘ SKIP${NC} $1"; SKIPPED=$((SKIPPED+1)); }
info()  { echo -e "  ${CYAN}ℹ${NC} $1"; }
section() { echo -e "\n${BOLD}${CYAN}▌ $1${NC}"; }

# ── 前置检查 ──
if [ -z "$TOKEN" ]; then
    echo -e "${RED}错误: 未设置 TOKEN 环境变量${NC}"
    echo "用法: TOKEN=sk-xxx bash scripts/test-seedance-api.sh"
    exit 1
fi
command -v curl &>/dev/null || { echo -e "${RED}缺少依赖: curl${NC}"; exit 1; }

# ── JSON 解析（无需 jq，用 python3 或 grep/sed） ──
# json_value <json> <key>
# 从 JSON 字符串中提取顶层或简单嵌套 key 的值
json_value() {
    local json="$1" key="$2"
    # 优先用 python3
    if command -v python3 &>/dev/null; then
        echo "$json" | python3 -c "
import sys, json
d = json.load(sys.stdin)
keys = '$key'.split('.')
v = d
for k in keys:
    if isinstance(v, dict) and k in v:
        v = v[k]
    else:
        v = None
        break
print('' if v is None else (str(v).lower() if isinstance(v, bool) else str(v)))
" 2>/dev/null
        return
    fi
    # fallback: grep/sed（不完美但够用）
    echo "$json" | grep -o "\"$key\"[[:space:]]*:[[:space:]]*\"[^\"]*\"" | head -1 | sed 's/.*:.*"\(.*\)"/\1/'
}

# ── 图片转 base64 data URL ──
file_to_data_url() {
    local f="$1"
    if [ ! -f "$f" ]; then
        echo ""
        return
    fi
    local ext="${f##*.}"
    ext=$(echo "$ext" | tr '[:upper:]' '[:lower:]')
    local mime
    case "$ext" in
        png)  mime="image/png" ;;
        jpg|jpeg) mime="image/jpeg" ;;
        webp) mime="image/webp" ;;
        gif)  mime="image/gif" ;;
        *)    mime="image/png" ;;
    esac
    local b64
    b64=$(base64 -w0 "$f" 2>/dev/null || base64 "$f" 2>/dev/null)
    echo "data:${mime};base64,${b64}"
}

# 准备图片 URL（优先用本地文件转 data URL，否则用公网 URL）
IMAGE_DATA_URL=""
if [ -n "$IMAGE_FILE" ] && [ -f "$IMAGE_FILE" ]; then
    IMAGE_DATA_URL=$(file_to_data_url "$IMAGE_FILE")
    info "图片已加载: $IMAGE_FILE ($(echo "$IMAGE_DATA_URL" | wc -c) bytes data URL)"
else
    IMAGE_DATA_URL="$IMAGE_URL"
    info "图片使用公网 URL: $IMAGE_URL"
fi

FACE_DATA_URL=""
if [ -n "$FACE_FILE" ] && [ -f "$FACE_FILE" ]; then
    FACE_DATA_URL=$(file_to_data_url "$FACE_FILE")
    info "人脸图片已加载: $FACE_FILE"
else
    FACE_DATA_URL="$FACE_URL"
    info "人脸图片使用公网 URL: $FACE_URL"
fi

# 视频/音频使用公网 URL（已提供默认值）
info "视频 URL: $VIDEO_URL"
info "音频 URL: $AUDIO_URL"

# ── 通用函数 ──

# poll_task_openai <task_id> <label>
poll_task_openai() {
    local task_id="$1" label="$2"
    info "[$label] 开始轮询 task_id=$task_id (OpenAI 风格)"
    for i in $(seq 1 "$MAX_POLLS"); do
        local resp
        resp=$(curl -s -H "Authorization: Bearer $TOKEN" "$BASE_URL/v1/video/generations/$task_id")
        local status
        status=$(json_value "$resp" "status")
        if [ -z "$status" ]; then
            status=$(json_value "$resp" "data.status")
        fi
        info "[$label] 轮询 #$i: status=$status"
        case "$status" in
            SUCCESS|succeeded) echo "$resp"; return 0;;
            FAILURE|failed)
                echo -e "${RED}[$label] 任务失败${NC}"
                echo "$resp"
                return 1
                ;;
        esac
        sleep "$POLL_INTERVAL"
    done
    echo -e "${RED}[$label] 超过最大轮询次数 ($MAX_POLLS)${NC}"
    return 1
}

# poll_task_volcano <task_id> <label>
poll_task_volcano() {
    local task_id="$1" label="$2"
    info "[$label] 开始轮询 task_id=$task_id (火山兼容风格)"
    for i in $(seq 1 "$MAX_POLLS"); do
        local resp
        resp=$(curl -s -H "Authorization: Bearer $TOKEN" "$BASE_URL/api/v3/contents/generations/tasks/$task_id")
        local status
        status=$(json_value "$resp" "status")
        info "[$label] 轮询 #$i: status=$status"
        case "$status" in
            succeeded) echo "$resp"; return 0;;
            failed)
                echo -e "${RED}[$label] 任务失败${NC}"
                echo "$resp"
                return 1
                ;;
        esac
        sleep "$POLL_INTERVAL"
    done
    echo -e "${RED}[$label] 超过最大轮询次数 ($MAX_POLLS)${NC}"
    return 1
}

# submit_and_poll_openai <label> <json_body>
submit_and_poll_openai() {
    local label="$1" body="$2"
    local task_id resp final

    resp=$(curl -s -X POST "$BASE_URL/v1/video/generations" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json" \
      -d "$body")

    task_id=$(json_value "$resp" "task_id")
    if [ -z "$task_id" ]; then
        task_id=$(json_value "$resp" "id")
    fi
    if [ -n "$task_id" ] && [ "$task_id" != "null" ]; then
        pass "[$label] 提交成功, task_id=$task_id"
    else
        fail "[$label] 提交失败"
        echo "$resp"
        return 1
    fi

    final=$(poll_task_openai "$task_id" "$label") || true
    if [ -n "$final" ]; then
        local video_url
        video_url=$(json_value "$final" "data.data.content.video_url")
        if [ -z "$video_url" ] || [ "$video_url" = "null" ]; then
            video_url=$(json_value "$final" "data.result_url")
        fi
        if [ -n "$video_url" ] && [ "$video_url" != "null" ]; then
            pass "[$label] 成功获取视频 URL"
            info "[$label] video_url=${video_url:0:80}..."
        else
            fail "[$label] 未获取到视频 URL"
        fi
        local tokens
        tokens=$(json_value "$final" "data.data.usage.completion_tokens")
        if [ -z "$tokens" ] || [ "$tokens" = "null" ] || [ "$tokens" = "0" ]; then
            tokens=$(json_value "$final" "data.data.usage.total_tokens")
        fi
        if [ -n "$tokens" ] && [ "$tokens" != "null" ] && [ "$tokens" != "0" ]; then
            pass "[$label] usage token 数据: $tokens"
        else
            fail "[$label] usage 无 token 数据"
        fi
    fi
}

# submit_and_poll_volcano <label> <json_body>
submit_and_poll_volcano() {
    local label="$1" body="$2"
    local task_id resp final

    resp=$(curl -s -X POST "$BASE_URL/api/v3/contents/generations/tasks" \
      -H "Authorization: Bearer $TOKEN" \
      -H "Content-Type: application/json" \
      -d "$body")

    task_id=$(json_value "$resp" "task_id")
    if [ -z "$task_id" ]; then
        task_id=$(json_value "$resp" "id")
    fi
    if [ -n "$task_id" ] && [ "$task_id" != "null" ]; then
        pass "[$label] 提交成功, task_id=$task_id"
    else
        fail "[$label] 提交失败"
        echo "$resp"
        return 1
    fi

    final=$(poll_task_volcano "$task_id" "$label") || true
    if [ -n "$final" ]; then
        local video_url
        video_url=$(json_value "$final" "content.video_url")
        if [ -n "$video_url" ] && [ "$video_url" != "null" ]; then
            pass "[$label] 成功获取视频 URL"
            info "[$label] video_url=${video_url:0:80}..."
        else
            fail "[$label] 未获取到视频 URL"
        fi
        local tokens
        tokens=$(json_value "$final" "usage.completion_tokens")
        if [ -z "$tokens" ] || [ "$tokens" = "null" ] || [ "$tokens" = "0" ]; then
            tokens=$(json_value "$final" "usage.total_tokens")
        fi
        if [ -n "$tokens" ] && [ "$tokens" != "null" ] && [ "$tokens" != "0" ]; then
            pass "[$label] usage token 数据: $tokens"
        else
            fail "[$label] usage 无 token 数据"
        fi
    fi
}

# ═══════════════════════════════════════════════════════════════
#  测试开始
# ═══════════════════════════════════════════════════════════════

echo -e "${BOLD}════════════════════════════════════════${NC}"
echo -e "${BOLD} Seedance 渠道全接口测试${NC}"
echo -e "${BOLD}════════════════════════════════════════${NC}"
echo -e "  BASE_URL:   $BASE_URL"
echo -e "  MODEL:      $MODEL"
echo -e "  FAST_MODEL: $FAST_MODEL"
echo -e "  POLL:       ${POLL_INTERVAL}s × ${MAX_POLLS}次"
echo -e "  IMAGE_FILE: ${IMAGE_URL:-(default)}"
echo -e "  VIDEO_URL:  $VIDEO_URL"
echo -e "  AUDIO_URL:  $AUDIO_URL"
echo ""

# ────────────────────────────────────────────────────
#  1. OpenAI 风格：文生视频 (t2v)
# ────────────────────────────────────────────────────
section "1. OpenAI 风格 — 文生视频 (t2v)"

submit_and_poll_openai "OpenAI-t2v" "{
  \"model\": \"$MODEL\",
  \"prompt\": \"A small kitten chasing a butterfly in a sunny garden, cinematic shot\",
  \"duration\": 5,
  \"size\": \"720p\"
}"

# ────────────────────────────────────────────────────
#  2. OpenAI 风格：图生视频 (i2v)
# ────────────────────────────────────────────────────
section "2. OpenAI 风格 — 图生视频 (i2v)"

submit_and_poll_openai "OpenAI-i2v" "{
  \"model\": \"$MODEL\",
  \"prompt\": \"这只可达鸭突然觉醒了超能力，双手合十开始蓄力，周围电光闪烁，最后释放出金色的冲击波\",
  \"duration\": 5,
  \"size\": \"720p\",
  \"images\": [\"$IMAGE_DATA_URL\"]
}"

# ────────────────────────────────────────────────────
#  3. OpenAI 风格：视频续写 (v2v)
# ────────────────────────────────────────────────────
section "3. OpenAI 风格 — 视频续写 (v2v)"

submit_and_poll_openai "OpenAI-v2v" "{
  \"model\": \"$MODEL\",
  \"prompt\": \"Continue this video with smooth transition\",
  \"duration\": 5,
  \"size\": \"720p\",
  \"videos\": [\"$VIDEO_URL\"]
}"

# ────────────────────────────────────────────────────
#  4. OpenAI 风格：音频驱动 (a2v)
# ────────────────────────────────────────────────────
section "4. OpenAI 风格 — 音频驱动 (a2v)"

submit_and_poll_openai "OpenAI-a2v" "{
  \"model\": \"$MODEL\",
  \"prompt\": \"A person speaking with natural lip sync\",
  \"duration\": 5,
  \"size\": \"720p\",
  \"images\": [\"$FACE_DATA_URL\"],
  \"audios\": [\"$AUDIO_URL\"]
}"

# ────────────────────────────────────────────────────
#  5. 火山兼容风格：文生视频 (t2v)
# ────────────────────────────────────────────────────
section "5. 火山兼容风格 — 文生视频 (t2v)"

submit_and_poll_volcano "Volcano-t2v" "{
  \"model\": \"$MODEL\",
  \"content\": [
    {\"type\": \"text\", \"text\": \"A white horse runs slowly on a grassland, cinematic shot\"}
  ],
  \"resolution\": \"720p\",
  \"duration\": 5
}"

# ────────────────────────────────────────────────────
#  6. 火山兼容风格：图生视频 (i2v)
# ────────────────────────────────────────────────────
section "6. 火山兼容风格 — 图生视频 (i2v)"

submit_and_poll_volcano "Volcano-i2v" "{
  \"model\": \"$MODEL\",
  \"content\": [
    {\"type\": \"text\", \"text\": \"这只可达鸭突然觉醒了超能力，双手合十开始蓄力，周围电光闪烁，最后释放出金色的冲击波\"},
    {\"type\": \"image_url\", \"image_url\": {\"url\": \"$IMAGE_DATA_URL\"}, \"role\": \"first_frame\"}
  ],
  \"resolution\": \"720p\",
  \"duration\": 5
}"

# ────────────────────────────────────────────────────
#  7. 火山兼容风格：视频续写 (v2v)
# ────────────────────────────────────────────────────
section "7. 火山兼容风格 — 视频续写 (v2v)"

submit_and_poll_volcano "Volcano-v2v" "{
  \"model\": \"$MODEL\",
  \"content\": [
    {\"type\": \"text\", \"text\": \"Continue this video scene naturally\"},
    {\"type\": \"video\", \"video\": {\"url\": \"$VIDEO_URL\"}}
  ],
  \"resolution\": \"720p\",
  \"duration\": 5
}"

# ────────────────────────────────────────────────────
#  8. 火山兼容风格：音频驱动 (a2v)
# ────────────────────────────────────────────────────
section "8. 火山兼容风格 — 音频驱动 (a2v)"

submit_and_poll_volcano "Volcano-a2v" "{
  \"model\": \"$MODEL\",
  \"content\": [
    {\"type\": \"text\", \"text\": \"A person speaking naturally\"},
    {\"type\": \"image_url\", \"image_url\": {\"url\": \"$FACE_DATA_URL\"}},
    {\"type\": \"audio\", \"audio\": {\"url\": \"$AUDIO_URL\"}}
  ],
  \"resolution\": \"720p\",
  \"duration\": 5
}"

# ────────────────────────────────────────────────────
#  9. Fast 模型文生视频（OpenAI 风格）
# ────────────────────────────────────────────────────
section "9. OpenAI 风格 — Fast 模型文生视频"

submit_and_poll_openai "OpenAI-fast-t2v" "{
  \"model\": \"$FAST_MODEL\",
  \"prompt\": \"A quick shot of a sunset over the ocean\",
  \"duration\": 5,
  \"size\": \"720p\"
}"

# ────────────────────────────────────────────────────
#  10. 负向测试：prompt 缺失
# ────────────────────────────────────────────────────
section "10. 负向测试 — prompt 缺失"

resp=$(curl -s -o /dev/null -w "%{http_code}" -X POST "$BASE_URL/v1/video/generations" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"model\": \"$MODEL\", \"duration\": 5}")

if [ "$resp" = "400" ]; then
    pass "prompt 缺失返回 400"
else
    fail "prompt 缺失应返回 400，实际=$resp"
fi

# ═══════════════════════════════════════════════════════════════
#  汇总
# ═══════════════════════════════════════════════════════════════
echo ""
echo -e "${BOLD}════════════════════════════════════════${NC}"
echo -e "${BOLD} Seedance 测试结果汇总${NC}"
echo -e "${BOLD}════════════════════════════════════════${NC}"
echo -e "  总计: ${TOTAL}  ${GREEN}通过: ${PASSED}${NC}  ${RED}失败: ${FAILED}${NC}  ${YELLOW}跳过: ${SKIPPED}${NC}"
echo ""

if [ "$FAILED" -eq 0 ]; then
    echo -e "${GREEN}✅ 全部自动化用例通过${NC}"
else
    echo -e "${RED}❌ 有 $FAILED 个用例失败${NC}"
    exit 1
fi
