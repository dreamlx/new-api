#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/test_lib.sh"

MESSAGE_MODEL="${MESSAGE_MODEL:-hosted_vllm/minimax-m2.5}"
# MESSAGE_MODEL="${MESSAGE_MODEL:-minimax-m2.5-highspeed}"
MESSAGE_PROMPT="${MESSAGE_PROMPT:-请简短回复：hello}"
MAX_TOKENS="${MAX_TOKENS:-1280}"
ANTHROPIC_VERSION="${ANTHROPIC_VERSION:-2026-04-01}"
MESSAGES_PATH="${MESSAGES_PATH:-/v1/messages}"
BASE_URL="${BASE_URL:-http://192.168.1.100}"
#RELAY_TOKEN="${RELAY_TOKEN:-sk-Hdiw6CgxSxPNESnfwkVxmTLoll7vD6SAZ7j8uW94EZ24svWP}" 
RELAY_TOKEN="${RELAY_TOKEN:-sk-ZA5H33LzSxzx9EpXFwoASA}" 

require_deps() {
    for cmd in curl jq mktemp; do
        if ! command -v "$cmd" >/dev/null 2>&1; then
            echo "缺少依赖: $cmd"
            exit 1
        fi
    done
}

require_token() {
    if [ -z "${RELAY_TOKEN:-}" ]; then
        echo "请设置 RELAY_TOKEN 后再运行"
        exit 1
    fi
}

build_body() {
    local stream_flag="$1"
    jq -n \
        --arg model "$MESSAGE_MODEL" \
        --arg prompt "$MESSAGE_PROMPT" \
        --argjson max_tokens "$MAX_TOKENS" \
        --argjson stream "$stream_flag" \
        '{
            model: $model,
            max_tokens: $max_tokens,
            stream: $stream,
            messages: [
                {
                    role: "user",
                    content: $prompt
                }
            ]
        }'
}

print_non_stream_structure() {
    local body="$1"
    echo "结构摘要:"
    echo "$body" | jq '{
        top_level_keys: keys,
        id,
        type,
        role,
        model,
        stop_reason,
        content_item_types: [.content[]?.type],
        usage_keys: (.usage | keys? // [])
    }'
}

print_stream_structure() {
    local stream_file="$1"

    echo "原始 SSE:"
    cat "$stream_file"

    echo ""
    echo "结构摘要:"
    awk '
        /^event: / {
            event_name = substr($0, 8)
        }
        /^data: / {
            data = substr($0, 7)
            printf "%s\t%s\n", event_name, data
        }
    ' "$stream_file" | while IFS=$'\t' read -r event_name payload; do
        if [ -z "$payload" ]; then
            continue
        fi
        echo "event=$event_name"
        echo "$payload" | jq '{
            top_level_keys: keys,
            type,
            message_keys: (.message | keys? // []),
            delta_keys: (.delta | keys? // []),
            usage_keys: (.usage | keys? // [])
        }'
    done
}

request_non_stream() {
    local response
    local body
    local status

    response=$(curl -sS -w '\n__STATUS__%{http_code}' \
        -X POST \
        -H "Authorization: Bearer $RELAY_TOKEN" \
        -H "Content-Type: application/json" \
        -H "anthropic-version: $ANTHROPIC_VERSION" \
        -d "$(build_body false)" \
        "$BASE_URL$MESSAGES_PATH")
    body=$(echo "$response" | sed '$d')
    status=$(echo "$response" | tail -n1 | sed 's/__STATUS__//')

    echo ""
    echo "===== 非流式响应 (stream=false) ====="
    echo "HTTP $status"
    echo "$body" | jq '.'
    print_non_stream_structure "$body"
}

request_stream() {
    local stream_file
    local header_file
    local status

    stream_file="$(mktemp)"
    header_file="$(mktemp)"
    trap 'rm -f "$stream_file" "$header_file"' RETURN

    curl -sS -N \
        -D "$header_file" \
        -X POST \
        -H "Authorization: Bearer $RELAY_TOKEN" \
        -H "Content-Type: application/json" \
        -H "anthropic-version: $ANTHROPIC_VERSION" \
        -d "$(build_body true)" \
        "$BASE_URL$MESSAGES_PATH" >"$stream_file"

    status=$(awk 'toupper($1) ~ /^HTTP\// {code=$2} END {print code}' "$header_file")

    echo ""
    echo "===== 流式响应 (stream=true) ====="
    echo "HTTP $status"
    print_stream_structure "$stream_file"

    rm -f "$stream_file" "$header_file"
    trap - RETURN
}

main() {
    require_deps
    require_token

    echo "BASE_URL=$BASE_URL"
    echo "MESSAGES_PATH=$MESSAGES_PATH"
    echo "MESSAGE_MODEL=$MESSAGE_MODEL"
    echo "ANTHROPIC_VERSION=$ANTHROPIC_VERSION"

    request_non_stream
    request_stream
}

main "$@"
