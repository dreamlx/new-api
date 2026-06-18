#!/usr/bin/env bash
# check_upstream_conformity.sh — guard rails that prevent fork drift from
# upstream's current tree layout / conventions. Designed to run as a
# pre-commit hook or in CI on every PR touching this repo.
#
# Each check is a separate function below. Add new checks as new patterns
# of drift surface. Each check echoes problems to stderr and increments
# $errors; the script exits non-zero iff any check failed.
#
# Usage:
#   scripts/check_upstream_conformity.sh                  # check staged changes (default)
#   scripts/check_upstream_conformity.sh --all            # check the full tree
#   scripts/check_upstream_conformity.sh --base <ref>     # diff against ref (e.g. origin/main)

set -euo pipefail

MODE="staged"
BASE_REF=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        --all) MODE="all"; shift ;;
        --base) MODE="base"; BASE_REF="$2"; shift 2 ;;
        -h|--help)
            grep '^#' "$0" | sed 's/^# \?//'; exit 0 ;;
        *)
            echo "unknown arg: $1" >&2; exit 2 ;;
    esac
done

errors=0
RED='\033[0;31m'; YELLOW='\033[1;33m'; NC='\033[0m'

# ── Helpers ─────────────────────────────────────────────────────────────────
list_files() {
    # Echoes paths to check, one per line.
    case "$MODE" in
        staged)
            git diff --cached --name-only --diff-filter=ACM ;;
        base)
            git diff --name-only --diff-filter=ACM "$BASE_REF"...HEAD ;;
        all)
            git ls-files ;;
    esac
}

fail() {
    # fail "rule name" "explanation" "<offending file>"
    local rule="$1" reason="$2" file="$3"
    echo -e "${RED}✗${NC} [${rule}] ${file}" >&2
    echo -e "  ${reason}" >&2
    errors=$((errors + 1))
}

# ── Check 1: web frontend path layout ──────────────────────────────────────
# Upstream split web/ into web/classic/ (legacy) and web/default/ (v1.0
# frontend) in v1.0.0-rc.1. New JSX/JS files in the old `web/src/` path
# will silently disappear at the next sync. They must live under one of
# the two new roots.
check_web_path() {
    while IFS= read -r f; do
        [[ -z "$f" ]] && continue
        case "$f" in
            web/src/*)
                fail "web-path" \
                    "Upstream v1.0.0-rc.1 renamed web/src/ to web/classic/src/. New files in web/src/ will be lost on next upstream sync. Move to web/classic/src/ (legacy frontend) or web/default/src/ (new v1 frontend)." \
                    "$f"
                ;;
        esac
    done < <(list_files | grep -E '\.(jsx?|tsx?|css|json|md)$' || true)
}

# ── Check 2: JSON marshal/unmarshal via common.* wrappers ──────────────────
# Project rule (CLAUDE.md): all JSON encoding must route through
# common.Marshal/Unmarshal/etc., not encoding/json directly. encoding/json
# is fine as a type import (json.RawMessage, json.Number).
check_encoding_json() {
    while IFS= read -r f; do
        [[ -z "$f" ]] && continue
        # Only flag direct call sites (json.Marshal, json.Unmarshal, etc.),
        # not bare imports of "encoding/json" (which is fine for type refs).
        if grep -nE '\bjson\.(Marshal|Unmarshal|NewEncoder|NewDecoder)\b' "$f" >/dev/null 2>&1; then
            local hits
            hits=$(grep -nE '\bjson\.(Marshal|Unmarshal|NewEncoder|NewDecoder)\b' "$f")
            while IFS= read -r line; do
                fail "encoding-json" \
                    "Direct encoding/json call. Use common.Marshal / common.Unmarshal / common.DecodeJson instead (see CLAUDE.md rule 1)." \
                    "$f:${line%%:*}"
            done <<<"$hits"
        fi
    done < <(list_files | grep -E '\.go$' | grep -v '_test\.go$' | grep -v '^common/' || true)
}

# ── Check 3: WiseModel re-introduction ────────────────────────────────────
# WiseModel channel-partner code was moved off main to wisemodel-main
# (see commit b97ef50ea). Any reintroduction on main is a process error
# (PR should have targeted wisemodel-main instead).
check_no_wisemodel_on_main() {
    local current_branch
    current_branch=$(git symbolic-ref --short HEAD 2>/dev/null || echo "")
    [[ "$current_branch" != "main" ]] && return 0

    while IFS= read -r f; do
        [[ -z "$f" ]] && continue
        case "$f" in
            *wisemodel*)
                fail "no-wisemodel-on-main" \
                    "WiseModel code belongs on the wisemodel-main branch, not main. Retarget the PR." \
                    "$f"
                ;;
        esac
    done < <(list_files)
}

# ── Run all checks ────────────────────────────────────────────────────────
check_web_path
check_encoding_json
check_no_wisemodel_on_main

if [[ $errors -gt 0 ]]; then
    echo "" >&2
    echo -e "${YELLOW}${errors} conformity issue(s) found.${NC}" >&2
    echo "See above. Each rule explains how to fix." >&2
    exit 1
fi

echo "Upstream conformity: OK"
exit 0
