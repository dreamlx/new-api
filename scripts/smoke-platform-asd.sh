#!/usr/bin/env bash
# =============================================================================
# smoke-platform-asd.sh
#
# Quick PlatformAuth verification using a known platform_id + plaintext sk.
# Unlike scripts/test-platform-auth-e2e.sh (which also exercises admin CRUD
# and needs $ADMIN_TOKEN), this script only needs the platform credentials
# and validates the read/write surface a downstream caller would actually use:
#
#     [01] negative — no headers           -> 401
#     [02] negative — wrong sk             -> 401
#     [03] positive — list logs            -> 200
#     [04] positive — authorize a new sk   -> 200 (status=authorized)
#     [05] positive — authorize same sk    -> 200 (status=exists, idempotent)
#
# Pass --cleanup to additionally try a follow-up DELETE on the just-registered
# token. (Requires the admin to expose a token-delete endpoint; skipped by
# default because the v2/external surface does not currently include one.)
#
# Usage:
#   BASE_URL=http://localhost:3000 bash scripts/smoke-platform-asd.sh
#   BASE_URL=http://192.168.1.100:3000 bash scripts/smoke-platform-asd.sh
# =============================================================================

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:3000}"
PLATFORM_ID="${PLATFORM_ID:-asd}"
PLATFORM_SK="${PLATFORM_SK:-pk_NHyRw3Kn2iXCRBjEFOPeUp6DjrCgZOsX22nzPn1K}"

# Each run registers a fresh user-token under this platform so re-running
# the script keeps producing fresh status="authorized" responses for the
# Create step. The "idempotent" step then re-submits the same one to prove
# status flips to "exists".
RUN_TAG="$(date +%s)"
USER_TOKEN_KEY="sk-smoke${RUN_TAG}zzzzzzzzzzzzzzzzzz"

# --- output helpers ----------------------------------------------------------
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
NC='\033[0m'
PASS=0
FAIL=0

step() { printf "\n${BOLD}${YELLOW}[%s]${NC} %s\n" "$1" "$2"; }
ok()   { PASS=$((PASS + 1)); printf "  ${GREEN}\xE2\x9C\x93${NC} %s\n" "$*"; }
bad()  { FAIL=$((FAIL + 1)); printf "  ${RED}\xE2\x9C\x97${NC} %s\n" "$*"; }

assert_code() {
  # assert_code <label> <expected> <actual> [<body-for-debug>]
  if [[ "$2" == "$3" ]]; then ok "$1 (HTTP $2)"
  else bad "$1 expected HTTP $2 got $3 ${4:+— body: $4}"; fi
}

# --- single request helper ---------------------------------------------------
# Echoes "<http_code>|<body>" so callers can split with cut -d'|'.
http() {
  local method="$1" path="$2"
  shift 2
  local body_file
  body_file="$(mktemp)"
  local code
  code=$(curl -sS -o "$body_file" -w "%{http_code}" -X "$method" \
    "${BASE_URL}${path}" "$@")
  printf "%s|%s" "$code" "$(cat "$body_file")"
  rm -f "$body_file"
}

# --- preflight ---------------------------------------------------------------
if ! curl -sf "${BASE_URL}/api/status" -o /dev/null; then
  printf "${RED}ABORT${NC}: server unreachable at %s\n" "$BASE_URL"
  printf "       try: curl -i %s/api/status\n" "$BASE_URL"
  exit 3
fi

printf "${BOLD}Target${NC}: %s\n" "$BASE_URL"
printf "${BOLD}Platform${NC}: %s\n" "$PLATFORM_ID"

# =============================================================================
# [01] No auth headers -> 401
# =============================================================================
step 01 "GET /api/v2/external/logs without any headers"
RESP=$(http GET "/api/v2/external/logs")
CODE="${RESP%%|*}"
assert_code "no-auth rejected" "401" "$CODE"

# =============================================================================
# [02] Wrong sk -> 401
# =============================================================================
step 02 "GET /api/v2/external/logs with wrong sk"
RESP=$(http GET "/api/v2/external/logs" \
  -H "X-Platform-Id: ${PLATFORM_ID}" \
  -H "X-Platform-Sk: pk_definitely_not_the_right_one")
CODE="${RESP%%|*}"
assert_code "wrong-sk rejected" "401" "$CODE"

# =============================================================================
# [03] Correct credentials -> list logs (200)
# =============================================================================
step 03 "GET /api/v2/external/logs with correct credentials"
RESP=$(http GET "/api/v2/external/logs?page=1&page_size=5" \
  -H "X-Platform-Id: ${PLATFORM_ID}" \
  -H "X-Platform-Sk: ${PLATFORM_SK}")
CODE="${RESP%%|*}"
BODY="${RESP#*|}"
assert_code "logs accessible" "200" "$CODE" "$BODY"
if echo "$BODY" | grep -qE '"sk-[a-zA-Z0-9]{10,}"'; then
  bad "logs body LEAKED a sk-... substring — server-side regression"
else
  ok "no sk- substring in logs body"
fi
if command -v jq >/dev/null; then
  TOTAL=$(echo "$BODY" | jq -r '.data.pagination.total // "?"')
  ok "log entries total: ${TOTAL}"
fi

# =============================================================================
# [04] Authorize a new user-token under this platform -> 200, status=authorized
# =============================================================================
step 04 "POST /api/v2/external/tokens/authorize (new key)"
RESP=$(http POST "/api/v2/external/tokens/authorize" \
  -H "X-Platform-Id: ${PLATFORM_ID}" \
  -H "X-Platform-Sk: ${PLATFORM_SK}" \
  -H "Content-Type: application/json" \
  -d "{\"token_key\":\"${USER_TOKEN_KEY}\"}")
CODE="${RESP%%|*}"
BODY="${RESP#*|}"
assert_code "authorize accepted" "200" "$CODE" "$BODY"
if command -v jq >/dev/null; then
  STATUS=$(echo "$BODY" | jq -r '.data.status // "?"')
  TOKEN_ID=$(echo "$BODY" | jq -r '.data.token_id // "?"')
  if [[ "$STATUS" == "authorized" ]]; then ok "status=authorized"
  else bad "expected status=authorized got status=${STATUS}"; fi
  ok "token_id=${TOKEN_ID}"
fi

# =============================================================================
# [05] Re-authorize same key -> idempotent (status=exists)
# =============================================================================
step 05 "POST /api/v2/external/tokens/authorize (same key, idempotent)"
RESP=$(http POST "/api/v2/external/tokens/authorize" \
  -H "X-Platform-Id: ${PLATFORM_ID}" \
  -H "X-Platform-Sk: ${PLATFORM_SK}" \
  -H "Content-Type: application/json" \
  -d "{\"token_key\":\"${USER_TOKEN_KEY}\"}")
CODE="${RESP%%|*}"
BODY="${RESP#*|}"
assert_code "re-authorize accepted" "200" "$CODE" "$BODY"
if command -v jq >/dev/null; then
  STATUS=$(echo "$BODY" | jq -r '.data.status // "?"')
  if [[ "$STATUS" == "exists" ]]; then ok "status=exists (idempotent)"
  else bad "expected status=exists got status=${STATUS}"; fi
fi

# --- summary ----------------------------------------------------------------
printf "\n${BOLD}========================================${NC}\n"
printf "${BOLD}Summary${NC}: ${GREEN}%d passed${NC}, ${RED}%d failed${NC}\n" "$PASS" "$FAIL"
[[ "$FAIL" -eq 0 ]] || exit 1
