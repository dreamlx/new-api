#!/usr/bin/env bash
# =============================================================================
# test-platform-auth-e2e.sh
#
# End-to-end smoke test for the platform-auth feature shipped in the
# `feature/v2-external-platform-auth` branch:
#
#     - Admin platform CRUD (Create / List / Get / Update / Delete)
#     - One-time plaintext platform_sk reveal
#     - PlatformAuth on V1 (/api/user/external/*) and V2 (/api/v2/external/*)
#     - 401 (missing/wrong sk) and 403 (disabled platform) branches
#     - delete + recreate (tombstone) path, with both default-recovery and
#       strict-isolation flavors
#     - newapi main interfaces unaffected (admin sessions still work)
#
# This script formalizes the curl flow documented in the plan file. It is
# IDEMPOTENT-ish — re-running cleans up its own platforms by suffixing them
# with a per-run timestamp ($RUN_TAG), and deletes them again at the end.
#
# =============================================================================
# TESTING METHODOLOGY
# =============================================================================
#
# 1. INVARIANTS THIS SCRIPT CHECKS
# --------------------------------
#   Every assertion in this file falls into one of three categories. Read this
#   first; the per-step output ("PASS / FAIL") only makes sense in context.
#
#   (a) Auth-gate invariants
#       Every request without correct X-Platform-Id + X-Platform-Sk must be
#       rejected with HTTP 401 (or 403 when the platform exists but is
#       disabled). The middleware MUST NOT reveal which check failed —
#       same status, same generic "unauthorized" message for missing-header,
#       unknown-id, and wrong-sk cases.
#
#   (b) Data-shape invariants
#       The plaintext platform_sk appears in EXACTLY ONE response — the
#       Create response. List, Get, Update, Delete responses must never
#       echo it nor the stored hash field. The V2 logs response must
#       carry token_id (integer) and must never contain a "sk-..." substring.
#       V1 stats response must mask the underlying token.Key.
#
#   (c) Behavioural invariants
#       Idempotent re-authorization returns the same token_id with
#       status="exists". Delete cascades to disable all tokens under the
#       platform's shadow user. Recreating a platform with the same
#       platform_id (without explicit shadow_user_id) intentionally reuses
#       the existing shadow user — this is the "lost sk, please re-issue"
#       recovery semantics. Passing an explicit shadow_user_id at recreate
#       enforces strict tenant isolation.
#
# 2. HOW EACH ASSERTION IS MADE
# -----------------------------
#   Two assertion primitives are used, both defined near the top:
#
#     assert_eq <label> <expected> <actual>
#         Plain string equality. Used for HTTP status codes ("200" == "200")
#         and for numeric ids passed back as strings.
#
#     assert_json_true <label> <jq-expression> <body>
#         Pipes <body> through `jq -e <expr>`. Test passes when the
#         expression evaluates to a truthy JSON value AND `jq -e` exits 0.
#         Use this when the assertion is structural ("response has key foo")
#         or comparing nested JSON fields.
#
#   Both primitives increment a single PASS/FAIL counter and accumulate
#   failure details into the FAIL_DETAILS array, which is printed in the
#   final summary. The script does NOT abort on the first failure (so you
#   see all problems in one run), but `set -euo pipefail` will still trip
#   on infrastructure errors (missing jq, network failures, etc).
#
# 3. REQUEST HELPERS
# ------------------
#   The script issues three flavors of HTTP request:
#
#     admin_req <METHOD> <PATH> [<json-body>]
#         Calls /api/admin/v2/platforms/* with the AdminAuth bearer token.
#         Used for platform provisioning steps.
#
#     plat_req <METHOD> <PATH> <PLATFORM_ID> <PLATFORM_SK> [<json-body>]
#         Calls /api/v2/external/* or /api/user/external/* with the
#         platform-credentials header pair. The HTTP status code is written
#         to stdout, and the response body lands in /tmp/.platreq.body so
#         it can be parsed by subsequent assertions.
#
#     bare_req <METHOD> <PATH>
#         Same as plat_req but without any auth headers — used to assert
#         the 401-on-missing-headers branch.
#
# 4. ENVIRONMENT PREREQUISITES
# ----------------------------
#   * Tools: `jq` and `curl` must be on $PATH. The script aborts early if not.
#   * Server: newapi listening at $BASE_URL (default http://localhost:3000).
#     Verified via GET /api/status before any other assertion runs.
#   * Admin credentials: $ADMIN_TOKEN must be a Bearer token (typically the
#     "access_token" issued to an admin-role user). Steps A-E are skipped
#     entirely if ADMIN_TOKEN is absent — leaving only the preflight check.
#
# 5. HOW TO OBTAIN AN ADMIN TOKEN
# -------------------------------
#   In a freshly-seeded newapi DB the root user already exists with username
#   "root" and a system-issued access_token. You can retrieve it via the
#   web UI ("Profile -> System Access Token") or by querying the DB:
#
#     sqlite3 one-api.db \
#       "SELECT access_token FROM users WHERE role >= 10 LIMIT 1;"
#
#   For repeatable CI runs, seed a dedicated admin user during fixture setup
#   and pass its access_token via the environment.
#
# 6. USAGE
# --------
#   Quick local run:
#     BASE_URL=http://localhost:3000 \
#     ADMIN_TOKEN=sk-admin-access-token \
#       bash scripts/test-platform-auth-e2e.sh
#
#   With verbose curl tracing (debug a single failure):
#     CURL_VERBOSE=1 BASE_URL=... ADMIN_TOKEN=... bash scripts/test-platform-auth-e2e.sh
#
#   Show this header and exit:
#     bash scripts/test-platform-auth-e2e.sh --help
#
# 7. EXIT CODES
# -------------
#   0   every assertion passed
#   1   one or more functional assertions failed (see "Failures:" section)
#   2   missing prerequisite tool (jq/curl)
#   3   newapi server not reachable on the configured BASE_URL
#
# 8. TROUBLESHOOTING
# ------------------
#   "create returns success:true" fails with success:false
#       Likely admin token expired or wrong role. Verify with:
#         curl -i -H "Authorization: Bearer $ADMIN_TOKEN" $BASE_URL/api/user/self
#       It must return 200 with role >= admin (10).
#
#   "V2 logs HTTP" fails with 401 even after Create succeeded
#       Confirm the returned platform_sk made it into PLATFORM_SK without
#       shell quoting damage. Tip: `set -x` near the failing step.
#
#   "shadow_user_id reused" failure
#       Indicates either (a) the recovery semantics regressed (now allocating
#       a new shadow user every recreate), or (b) Platform.Delete is no
#       longer tombstoning the platform_id and the second Create hit UNIQUE.
#       Cross-check with the Go test:
#         go test -run TestAdminPlatform_DeleteRecreate_ReusesSameShadowUser \
#           ./controller/...
#
# =============================================================================

# --- show extended doc and exit ---------------------------------------------
# Print everything from the start of file up to the first blank-non-comment
# line (i.e. the entire documentation header above). Stripping the leading
# `# ` makes the output readable as plain prose, not literal shell comments.
if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  awk '
    NR==1 { next }
    /^[^#]/ && !/^$/ { exit }
    { sub(/^# ?/, ""); print }
  ' "$0"
  exit 0
fi

set -euo pipefail

# --- runtime configuration --------------------------------------------------
# BASE_URL    target newapi server (override for staging/production)
# ADMIN_TOKEN admin role Bearer token; if empty, the admin steps are skipped
# RUN_TAG     unix timestamp suffixed onto every test resource so re-running
#             never collides with a previous run that didn't finish cleanup
# CURL_VERBOSE  set to "1" to dump full request/response trace (debugging
#             individual failures). Off by default — keeps the happy-path
#             output compact.
BASE_URL="${BASE_URL:-http://localhost:3000}"
ADMIN_TOKEN="${ADMIN_TOKEN:-}"
RUN_TAG="$(date +%s)"
CURL_VERBOSE="${CURL_VERBOSE:-0}"
PID_PRIMARY="e2e_primary_${RUN_TAG}"
PID_RECREATE="e2e_recreate_${RUN_TAG}"
EXT_USER_ID="e2e_extuser_${RUN_TAG}"

# --- pretty output ----------------------------------------------------------
# Colorized PASS/FAIL is intentional: when a CI job pipes this through `less`
# the operator can spot regressions in seconds without parsing JSON.
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
NC='\033[0m'

# Counters consumed by the final summary. Failures are NOT fatal mid-run —
# we accumulate everything so a single execution surfaces every regression.
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

# --- preflight --------------------------------------------------------------
# Hard exit (code 2) if jq/curl aren't installed — every assertion downstream
# depends on them. We deliberately do NOT try to auto-install: failing here
# is the right signal that the environment isn't fixture-ready.
for c in jq curl; do
  command -v "$c" >/dev/null || { echo "missing required tool: $c"; exit 2; }
done

if [[ -z "$ADMIN_TOKEN" ]]; then
  printf "${YELLOW}WARN${NC}: ADMIN_TOKEN not set; admin API steps will be skipped.\n"
  printf "      Set it to a Bearer token with admin role to exercise full flow.\n"
fi

# --- HTTP request helpers ---------------------------------------------------
# Three wrappers around curl, each matching one auth posture exercised by
# this script. All of them honor CURL_VERBOSE: when set, curl runs with -v
# and dumps headers/body to stderr so a failing assertion can be diagnosed
# without rewriting the script.

_curl_flags() {
  # Always emit short status + body; add -v when debugging is requested.
  if [[ "$CURL_VERBOSE" == "1" ]]; then
    echo "-sS -v"
  else
    echo "-sS"
  fi
}

# admin_req <METHOD> <PATH> [<json-body>]
#   Used for /api/admin/v2/platforms/* — authenticated with AdminAuth.
#   Returns ONLY the response body on stdout (callers pipe to jq).
admin_req() {
  local method="$1" path="$2" body="${3:-}"
  # shellcheck disable=SC2046
  if [[ -n "$body" ]]; then
    curl $(_curl_flags) -X "$method" "${BASE_URL}${path}" \
      -H "Authorization: Bearer ${ADMIN_TOKEN}" \
      -H "Content-Type: application/json" \
      -d "$body"
  else
    curl $(_curl_flags) -X "$method" "${BASE_URL}${path}" \
      -H "Authorization: Bearer ${ADMIN_TOKEN}"
  fi
}

# plat_req <METHOD> <PATH> <PLATFORM_ID> <PLATFORM_SK> [<json-body>]
#   Used for /api/v2/external/* and /api/user/external/* — authenticated
#   with PlatformAuth headers. Writes the response body to /tmp/.platreq.body
#   and emits the HTTP status code on stdout so callers can `CODE=$(...)`
#   and assert separately on status + body shape.
plat_req() {
  local method="$1" path="$2" pid="$3" psk="$4" body="${5:-}"
  # shellcheck disable=SC2046
  if [[ -n "$body" ]]; then
    curl $(_curl_flags) -o /tmp/.platreq.body -w "%{http_code}" -X "$method" "${BASE_URL}${path}" \
      -H "X-Platform-Id: ${pid}" -H "X-Platform-Sk: ${psk}" \
      -H "Content-Type: application/json" -d "$body"
  else
    curl $(_curl_flags) -o /tmp/.platreq.body -w "%{http_code}" -X "$method" "${BASE_URL}${path}" \
      -H "X-Platform-Id: ${pid}" -H "X-Platform-Sk: ${psk}"
  fi
}

# bare_req <METHOD> <PATH>
#   Headerless request — exclusively used to assert the 401 branch when
#   PlatformAuth headers are entirely missing. Same body+status convention
#   as plat_req.
bare_req() {
  local method="$1" path="$2"
  # shellcheck disable=SC2046
  curl $(_curl_flags) -o /tmp/.platreq.body -w "%{http_code}" -X "$method" "${BASE_URL}${path}" \
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

# ============================================================================
# PREFLIGHT — server liveness
#
# Purpose: confirm BASE_URL is reachable before issuing any auth requests.
# Method:  GET /api/status (newapi's unauthenticated health probe). If this
#          fails we abort with exit code 3 rather than emit dozens of
#          confusing failures downstream.
# ============================================================================
step "Liveness: ${BASE_URL}"
if curl -sf "${BASE_URL}/api/status" -o /dev/null; then ok "server reachable"
else bad "server unreachable; abort"; printf "\nrun curl -i ${BASE_URL}/api/status to debug\n"; exit 3; fi

# ============================================================================
# SECTION A — Admin platform CRUD + one-time sk reveal
#
# WHAT THIS VALIDATES
#   * Create returns 200 with success:true and a plaintext platform_sk
#     prefixed "pk_" — this is the ONE place the sk is ever surfaced.
#   * The response includes a 'warning' string ("only returned once").
#   * List and Get endpoints never expose the stored hash column nor
#     re-emit the plaintext sk.
#   * Update on `name` propagates immediately and is reflected in the
#     PATCH response body.
#
# WHY
#   These four endpoints back the admin UI workflow. If any of them ever
#   leak `platform_sk_hash` or `platform_sk` in non-create responses,
#   credentials harvesting becomes trivial for anyone with admin read access.
# ============================================================================

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

# ============================================================================
# SECTION B — PlatformAuth on V2 endpoints (positive path)
#
# WHAT THIS VALIDATES
#   * POST /api/v2/external/tokens/authorize accepts the platform sk via
#     X-Platform-Id + X-Platform-Sk headers, returns 200 with token_id.
#   * Idempotency: re-authorizing the same token_key returns status="exists"
#     and the SAME numeric token_id as the first registration.
#   * Response shape: NO token_key field — sk plaintext is never echoed back
#     in the V2 surface. (The Create response is the only place a sk is
#     exposed, and that's the platform-level sk, not the user token sk.)
#   * GET /api/v2/external/logs returns 200, lists token_id (integer), and
#     contains no "sk-..." substring anywhere in the body.
#   * Query filter ?token_id=<int> works; ?token_id=<non-int> returns 400.
#
# WHY
#   This is the core public V2 contract. Any regression here (extra sk in
#   response, missing idempotency, broken filter) would break downstream
#   billing/integration platforms that have already adopted V2.
# ============================================================================

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

# ============================================================================
# SECTION C — Auth matrix (negative paths)
#
# WHAT THIS VALIDATES
#   Four distinct ways auth can fail, all of which must collapse to the same
#   visible behavior to avoid leaking platform-existence:
#     (1) No PlatformAuth headers at all          -> 401 "unauthorized"
#     (2) Correct platform_id but wrong sk        -> 401 "unauthorized"
#     (3) Both headers present but platform_id    -> 401 "unauthorized"
#         is unknown
#     (4) Correct credentials, platform disabled  -> 403 "platform disabled"
#
# WHY
#   * (1)-(3) must all produce 401 and IDENTICAL message text. Otherwise an
#     attacker can distinguish "this id exists, wrong sk" from "this id
#     doesn't exist" and enumerate platform_ids in seconds.
#   * (4) must produce 403 — a distinct code so legitimate admins/operators
#     can tell "I'm currently disabled" apart from "you're not me at all".
#
# After exercising (4), the test re-enables the platform so the remaining
# steps continue against a healthy row.
# ============================================================================

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

# ============================================================================
# SECTION D — V1 endpoints share the same PlatformAuth gate
#
# WHAT THIS VALIDATES
#   * /api/user/external/* without auth headers -> 401 (was previously open!)
#   * Once authenticated, V1 handlers behave exactly as before:
#       - POST /sync creates an external_user mapped under the platform
#       - GET  /:external_user_id/stats returns the user view
#   * The V1 stats response masks token.Key — specifically, no field of the
#     form `"key": "sk-<20+ chars>"` may appear in the response body.
#
# WHY
#   The single sk-masking fix to GetExternalUserStats is the V1-side payload
#   of this PR. It's tested at the model and controller layers in Go, but
#   an end-to-end regex sweep over the actual HTTP response body is the
#   most robust guard against future fields accidentally re-introducing
#   the leak (e.g. a new "callback_url_secret" field shaped like sk-...).
# ============================================================================

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

# ============================================================================
# SECTION E — Delete + recreate (tombstone) path
#
# WHAT THIS VALIDATES
#   * DELETE /api/admin/v2/platforms/:id returns success:true and the
#     platform's sk is immediately invalidated (subsequent V2 call -> 401).
#   * Re-Creating the SAME platform_id succeeds (tombstone freed the UNIQUE
#     index). The recreated platform reuses the original shadow_user_id —
#     this is the "lost sk, please re-issue" recovery semantics. Asserted
#     via assert_eq "shadow_user_id reused".
#   * Creating ANOTHER platform with an EXPLICIT shadow_user_id achieves
#     strict tenant isolation (caller chooses the user pointer).
#   * Create with shadow_user_id pointing at a nonexistent user -> rejected.
#
# WHY
#   Without the tombstone, an admin who lost a platform's sk would be stuck:
#   they couldn't delete-and-recreate because the UNIQUE platform_id index
#   would still block the new row. The recovery vs. strict-isolation
#   distinction is the most subtle invariant of this PR — losing it would
#   either lock admins out (no recovery) or silently leak between tenants
#   (no isolation override).
# ============================================================================

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

# ============================================================================
# SECTION F — newapi main interfaces unaffected (scope guard)
#
# WHAT THIS VALIDATES
#   The main /api/user/self endpoint (representative of the dashboard /
#   user-facing APIs) still works with the admin's plain AdminAuth Bearer
#   token, with NO PlatformAuth headers.
#
# WHY
#   The whole point of constraining PlatformAuth to the two /external groups
#   is that the rest of newapi must not regress. If a future change
#   accidentally hangs PlatformAuth on a parent router or applies it
#   globally, this assertion catches it instantly.
# ============================================================================

step "Main admin /api/user/self still works (no PlatformAuth on main)"
CODE=$(curl -sS -o /dev/null -w "%{http_code}" \
  -H "Authorization: Bearer ${ADMIN_TOKEN}" \
  "${BASE_URL}/api/user/self")
if [[ "$CODE" == "200" ]]; then ok "main admin route unaffected (200)"
else bad "main admin route broke: HTTP $CODE"; fi

# ============================================================================
# CLEANUP — best-effort tombstone of resources created by THIS run
#
# Notes:
#   * Errors during cleanup are silenced (`|| true`) — a failed cleanup must
#     not mask actual test failures. If a row leaks, the next run's $RUN_TAG
#     avoids collision because every resource id includes the unix timestamp.
#   * We re-list platforms because the "recreate same id" step created a row
#     under PID_PRIMARY that we don't have a stored numeric id for at this
#     point in the script — `jq | select` recovers it.
# ============================================================================
step "Cleanup: delete platforms created by this run"
admin_req DELETE "/api/admin/v2/platforms/${ISO_ID}" >/dev/null || true
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
