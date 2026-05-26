#!/usr/bin/env bash
#
# End-to-end smoke test for the platform-auth feature:
#   - Admin platform CRUD (Create / List / Get / Update / Delete)
#   - One-time plaintext platform_sk reveal
#   - PlatformAuth on V1 (/api/user/external/*) and V2 (/api/v2/external/*)
#   - 401 (missing/wrong sk) and 403 (disabled platform) branches
#   - delete + recreate (tombstone) path
#   - newapi main interfaces unaffected (admin tokens still work)
#
# This script formalizes the curl flow documented in the plan file. It is
# IDEMPOTENT-ish — re-running cleans up its own platforms by suffixing them
# with a per-run timestamp.
#
# Prereqs:
#   * jq, curl
#   * newapi server reachable at $BASE_URL
#   * Admin session/access token in $ADMIN_TOKEN (Authorization: Bearer ...)
#
# Usage:
#   BASE_URL=http://localhost:3000 ADMIN_TOKEN=sk-admin... \
#     scripts/test-platform-auth-e2e.sh
#
# Exit code 0 if every check passes; non-zero on the first failure with
# enough context to locate the regression.

set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:3000}"
ADMIN_TOKEN="${ADMIN_TOKEN:-}"
RUN_TAG="$(date +%s)"
PID_PRIMARY="e2e_primary_${RUN_TAG}"
PID_RECREATE="e2e_recreate_${RUN_TAG}"
EXT_USER_ID="e2e_extuser_${RUN_TAG}"

# ---------- pretty output ----------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
NC='\033[0m'

PASS=0
FAIL=0
STEP=0
declare -a FAIL_DETAILS=()

step() {
  STEP=$((STEP + 1))
  printf "\n${BOLD}${YELLOW}[%02d]${NC} %s\n" "$STEP" "$*"
}

ok() {
  PASS=$((PASS + 1))
  printf "  ${GREEN}\xE2\x9C\x93 PASS${NC} %s\n" "$*"
}

bad() {
  FAIL=$((FAIL + 1))
  printf "  ${RED}\xE2\x9C\x97 FAIL${NC} %s\n" "$*"
  FAIL_DETAILS+=("step $STEP: $*")
}

# ---------- preflight ----------
for c in jq curl; do
  command -v "$c" >/dev/null || { echo "missing required tool: $c"; exit 2; }
done

if [[ -z "$ADMIN_TOKEN" ]]; then
  printf "${YELLOW}WARN${NC}: ADMIN_TOKEN not set; admin API steps will be skipped.\n"
  printf "      Set it to a Bearer token with admin role to exercise full flow.\n"
fi

# ---------- helpers ----------

# admin_req <METHOD> <PATH> [<json-body>]
admin_req() {
  local method="$1" path="$2" body="${3:-}"
  if [[ -n "$body" ]]; then
    curl -sS -X "$method" "${BASE_URL}${path}" \
      -H "Authorization: Bearer ${ADMIN_TOKEN}" \
      -H "Content-Type: application/json" \
      -d "$body"
  else
    curl -sS -X "$method" "${BASE_URL}${path}" \
      -H "Authorization: Bearer ${ADMIN_TOKEN}"
  fi
}

# plat_req <METHOD> <PATH> <PLATFORM_ID> <PLATFORM_SK> [<json-body>]
plat_req() {
  local method="$1" path="$2" pid="$3" psk="$4" body="${5:-}"
  if [[ -n "$body" ]]; then
    curl -sS -o /tmp/.platreq.body -w "%{http_code}" -X "$method" "${BASE_URL}${path}" \
      -H "X-Platform-Id: ${pid}" -H "X-Platform-Sk: ${psk}" \
      -H "Content-Type: application/json" -d "$body"
  else
    curl -sS -o /tmp/.platreq.body -w "%{http_code}" -X "$method" "${BASE_URL}${path}" \
      -H "X-Platform-Id: ${pid}" -H "X-Platform-Sk: ${psk}"
  fi
}

# bare_req <METHOD> <PATH>  — no headers; used for 401 negative tests
bare_req() {
  local method="$1" path="$2"
  curl -sS -o /tmp/.platreq.body -w "%{http_code}" -X "$method" "${BASE_URL}${path}" \
    -H "Content-Type: application/json"
}

# assert_eq <label> <expected> <actual>
assert_eq() {
  if [[ "$2" == "$3" ]]; then ok "$1 ($2)"; else bad "$1 expected=$2 got=$3"; fi
}

assert_json_true() {
  local label="$1" expr="$2" body="$3"
  if echo "$body" | jq -e "$expr" >/dev/null 2>&1; then ok "$label"; else bad "$label  body=$body"; fi
}

# ---------- preflight liveness ----------
step "Liveness: ${BASE_URL}"
if curl -sf "${BASE_URL}/api/status" -o /dev/null; then ok "server reachable"
else bad "server unreachable; abort"; printf "\nrun curl -i ${BASE_URL}/api/status to debug\n"; exit 3; fi

# =============================================================================
# A. Admin platform CRUD + one-time sk reveal
# =============================================================================

if [[ -n "$ADMIN_TOKEN" ]]; then

step "Admin Create: ${PID_PRIMARY}"
CREATE_RESP=$(admin_req POST "/api/admin/v2/platforms/" \
  "{\"platform_id\":\"${PID_PRIMARY}\",\"name\":\"E2E Primary\"}")
assert_json_true "create returns success:true" '.success == true' "$CREATE_RESP"
PLATFORM_SK=$(echo "$CREATE_RESP" | jq -r '.data.platform_sk')
PLATFORM_ID_INT=$(echo "$CREATE_RESP" | jq -r '.data.id')
SHADOW_USER_ID=$(echo "$CREATE_RESP" | jq -r '.data.shadow_user_id')
assert_json_true "platform_sk prefix=pk_" '.data.platform_sk | startswith("pk_")' "$CREATE_RESP"
assert_json_true "warning present" '.data.warning | length > 0' "$CREATE_RESP"

step "Admin List excludes nothing under the new id, no hash leak"
LIST_BODY=$(admin_req GET "/api/admin/v2/platforms/?page=1&page_size=50")
assert_json_true "list contains primary" \
  ".data.items | map(.platform_id) | index(\"${PID_PRIMARY}\") != null" "$LIST_BODY"
if echo "$LIST_BODY" | grep -q 'platform_sk_hash'; then bad "list leaked hash field"; else ok "no hash field in list"; fi
if echo "$LIST_BODY" | grep -q '"platform_sk"'; then bad "list leaked plaintext sk"; else ok "no plaintext sk in list"; fi

step "Admin Get single platform"
GET_BODY=$(admin_req GET "/api/admin/v2/platforms/${PLATFORM_ID_INT}")
assert_json_true "get returns same platform_id" ".data.platform_id == \"${PID_PRIMARY}\"" "$GET_BODY"

step "Admin Update name"
UPD=$(admin_req PATCH "/api/admin/v2/platforms/${PLATFORM_ID_INT}" '{"name":"E2E Primary Renamed"}')
assert_json_true "update name reflected" '.data.name == "E2E Primary Renamed"' "$UPD"

# ===========================================================================
# B. PlatformAuth on V2 endpoints
# ===========================================================================

step "V2 authorize a new sk under ${PID_PRIMARY}"
TOKEN_KEY_1="sk-e2etok1${RUN_TAG}aaaaaaaaaaaaaaaaaa"
CODE=$(plat_req POST "/api/v2/external/tokens/authorize" "$PID_PRIMARY" "$PLATFORM_SK" \
  "{\"token_key\":\"${TOKEN_KEY_1}\"}")
assert_eq "V2 authorize HTTP" "200" "$CODE"
TOKEN_ID_1=$(jq -r '.data.token_id' /tmp/.platreq.body)
assert_json_true "V2 authorize status=authorized" '.data.status == "authorized"' "$(cat /tmp/.platreq.body)"
assert_json_true "response does NOT include token_key" '.data | has("token_key") | not' "$(cat /tmp/.platreq.body)"

step "V2 authorize is idempotent (status=exists)"
CODE=$(plat_req POST "/api/v2/external/tokens/authorize" "$PID_PRIMARY" "$PLATFORM_SK" \
  "{\"token_key\":\"${TOKEN_KEY_1}\"}")
assert_eq "V2 idempotent HTTP" "200" "$CODE"
assert_json_true "status=exists" '.data.status == "exists"' "$(cat /tmp/.platreq.body)"
TOKEN_ID_1B=$(jq -r '.data.token_id' /tmp/.platreq.body)
assert_eq "same token_id on re-auth" "$TOKEN_ID_1" "$TOKEN_ID_1B"

step "V2 logs returns token_id NOT token_key"
CODE=$(plat_req GET "/api/v2/external/logs" "$PID_PRIMARY" "$PLATFORM_SK")
assert_eq "V2 logs HTTP" "200" "$CODE"
LOGS_BODY=$(cat /tmp/.platreq.body)
if echo "$LOGS_BODY" | grep -q '"token_key":'; then bad "V2 logs response still emits token_key"; else ok "no token_key in logs response"; fi
if echo "$LOGS_BODY" | grep -qE '"sk-[a-zA-Z0-9]{10,}"'; then bad "V2 logs leaked sk substring"; else ok "no sk substring in logs body"; fi

step "V2 logs filter by token_id"
CODE=$(plat_req GET "/api/v2/external/logs?token_id=${TOKEN_ID_1}" "$PID_PRIMARY" "$PLATFORM_SK")
assert_eq "filter HTTP" "200" "$CODE"

step "V2 logs token_id=notanint => 400"
CODE=$(plat_req GET "/api/v2/external/logs?token_id=notanint" "$PID_PRIMARY" "$PLATFORM_SK")
assert_eq "bad filter HTTP" "400" "$CODE"

# ===========================================================================
# C. Auth matrix (negative paths)
# ===========================================================================

step "V2 logs WITHOUT headers => 401"
CODE=$(bare_req GET "/api/v2/external/logs")
assert_eq "no-auth HTTP" "401" "$CODE"

step "V2 logs WRONG sk => 401"
CODE=$(plat_req GET "/api/v2/external/logs" "$PID_PRIMARY" "pk_wrong_${RUN_TAG}")
assert_eq "wrong-sk HTTP" "401" "$CODE"

step "V2 logs UNKNOWN platform_id => 401"
CODE=$(plat_req GET "/api/v2/external/logs" "ghost_${RUN_TAG}" "pk_irrelevant")
assert_eq "unknown-pid HTTP" "401" "$CODE"

step "V2 logs DISABLED platform => 403"
admin_req PATCH "/api/admin/v2/platforms/${PLATFORM_ID_INT}" '{"status":2}' >/dev/null
CODE=$(plat_req GET "/api/v2/external/logs" "$PID_PRIMARY" "$PLATFORM_SK")
assert_eq "disabled HTTP" "403" "$CODE"
# Re-enable for the rest of the script.
admin_req PATCH "/api/admin/v2/platforms/${PLATFORM_ID_INT}" '{"status":1}' >/dev/null

# ===========================================================================
# D. V1 endpoints — same PlatformAuth gate, handlers unchanged
# ===========================================================================

step "V1 stats WITHOUT headers => 401"
CODE=$(bare_req GET "/api/user/external/${EXT_USER_ID}/stats")
assert_eq "V1 no-auth HTTP" "401" "$CODE"

step "V1 sync (auth required) creates external user"
CODE=$(plat_req POST "/api/user/external/sync" "$PID_PRIMARY" "$PLATFORM_SK" \
  "{\"external_user_id\":\"${EXT_USER_ID}\",\"username\":\"e2e_${RUN_TAG}\",\"email\":\"e2e${RUN_TAG}@example.com\"}")
assert_eq "V1 sync HTTP" "200" "$CODE"

step "V1 stats returns masked sk (no full token.Key)"
CODE=$(plat_req GET "/api/user/external/${EXT_USER_ID}/stats" "$PID_PRIMARY" "$PLATFORM_SK")
assert_eq "V1 stats HTTP" "200" "$CODE"
STATS_BODY=$(cat /tmp/.platreq.body)
if echo "$STATS_BODY" | grep -qE '"key":\s*"sk-[a-zA-Z0-9]{20,}"'; then
  bad "V1 stats response includes unmasked sk"
else
  ok "V1 stats sk masked (or no tokens yet)"
fi

# ===========================================================================
# E. Delete + recreate (tombstone) path
# ===========================================================================

step "Delete primary platform"
CODE=$(admin_req DELETE "/api/admin/v2/platforms/${PLATFORM_ID_INT}" | jq -r '.success')
assert_eq "delete success" "true" "$CODE"

step "After delete: V2 call with old sk => 401"
CODE=$(plat_req GET "/api/v2/external/logs" "$PID_PRIMARY" "$PLATFORM_SK")
assert_eq "post-delete HTTP" "401" "$CODE"

step "Recreate platform with same id (recovery semantics)"
RECREATE_RESP=$(admin_req POST "/api/admin/v2/platforms/" \
  "{\"platform_id\":\"${PID_PRIMARY}\",\"name\":\"E2E Recovery\"}")
assert_json_true "recreate succeeds" '.success == true' "$RECREATE_RESP"
NEW_SK=$(echo "$RECREATE_RESP" | jq -r '.data.platform_sk')
NEW_SHADOW=$(echo "$RECREATE_RESP" | jq -r '.data.shadow_user_id')

step "Recovery: same platform_id reuses shadow user"
assert_eq "shadow_user_id reused" "$SHADOW_USER_ID" "$NEW_SHADOW"

step "Strict isolation: recreate ${PID_RECREATE} with explicit shadow_user_id"
# Pick the original shadow user id deliberately so we can compare against a
# manually-overridden creation. We expect the user to exist.
ISO_RESP=$(admin_req POST "/api/admin/v2/platforms/" \
  "{\"platform_id\":\"${PID_RECREATE}\",\"shadow_user_id\":${SHADOW_USER_ID}}")
assert_json_true "explicit shadow_user_id accepted" '.success == true' "$ISO_RESP"
assert_eq "shadow_user_id matches override" "$SHADOW_USER_ID" "$(echo "$ISO_RESP" | jq -r '.data.shadow_user_id')"
ISO_ID=$(echo "$ISO_RESP" | jq -r '.data.id')

step "Reject explicit shadow_user_id pointing at nonexistent user"
BAD=$(admin_req POST "/api/admin/v2/platforms/" \
  "{\"platform_id\":\"e2e_bad_shadow_${RUN_TAG}\",\"shadow_user_id\":999999999}")
assert_json_true "rejected with success:false" '.success == false' "$BAD"

# ===========================================================================
# F. newapi main interfaces unaffected
# ===========================================================================

step "Main admin /api/user/self still works (no PlatformAuth on main)"
CODE=$(curl -sS -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  "${BASE_URL}/api/user/self")
if [[ "$CODE" == "200" ]]; then ok "main admin route unaffected (200)"
else bad "main admin route broke: HTTP $CODE"; fi

# ---------- cleanup ----------
step "Cleanup: delete platforms created by this run"
admin_req DELETE "/api/admin/v2/platforms/${ISO_ID}" >/dev/null || true
# Lookup the recreate-by-same-id row to clean it up too.
LIST_AFTER=$(admin_req GET "/api/admin/v2/platforms/?page=1&page_size=200")
TARGET_ID=$(echo "$LIST_AFTER" | jq -r ".data.items[] | select(.platform_id == \"${PID_PRIMARY}\") | .id" | head -1)
if [[ -n "$TARGET_ID" ]]; then
  admin_req DELETE "/api/admin/v2/platforms/${TARGET_ID}" >/dev/null || true
fi
ok "cleanup attempted"

else
  printf "${YELLOW}admin steps skipped (no ADMIN_TOKEN).${NC}\n"
fi

# ===========================================================================
# Summary
# ===========================================================================
printf "\n${BOLD}========================================${NC}\n"
printf "${BOLD}Summary${NC}: ${GREEN}%d passed${NC}, ${RED}%d failed${NC} (across %d steps)\n" \
  "$PASS" "$FAIL" "$STEP"

if (( FAIL > 0 )); then
  printf "\n${RED}Failures:${NC}\n"
  for d in "${FAIL_DETAILS[@]}"; do printf "  - %s\n" "$d"; done
  exit 1
fi
exit 0
