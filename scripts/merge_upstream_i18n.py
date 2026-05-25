#!/usr/bin/env python3
"""Resolve i18n locale merge conflicts via programmatic key union.

Run from the repo root after `git merge` (or `git pull`) reports conflicts
in `web/{classic,default}/src/i18n/locales/*.json`. The script reads the
three merge stages git keeps in the index, computes a key-by-key union of
the `translation` dict (our values win on collision), writes the merged
JSON, and stages the result so a follow-up `git commit` resolves the merge.

Usage:
    python3 scripts/merge_upstream_i18n.py                 # default: scan both locale roots
    python3 scripts/merge_upstream_i18n.py <root>...       # specific dirs

Why this script exists: every upstream sync of new-api adds translation
keys, and our side also adds keys, so every locale JSON ends up conflicted.
A 3-way text merge can't reason about JSON object semantics, so we always
have to do the union by hand. This script is just that union, automated.
"""
import json
import subprocess
import sys
from pathlib import Path


DEFAULT_ROOTS = [
    "web/classic/src/i18n/locales",
    "web/default/src/i18n/locales",
]


def _git_show(stage: int, path: str) -> dict:
    """Read a specific merge stage of a file from the git index."""
    raw = subprocess.check_output(
        ["git", "show", f":{stage}:{path}"], stderr=subprocess.DEVNULL
    )
    return json.loads(raw.decode("utf-8"))


def _in_conflict(path: str) -> bool:
    """True iff path has both stage 2 (ours) and stage 3 (theirs) — i.e. is unmerged."""
    try:
        subprocess.check_output(["git", "show", f":2:{path}"], stderr=subprocess.DEVNULL)
        subprocess.check_output(["git", "show", f":3:{path}"], stderr=subprocess.DEVNULL)
        return True
    except subprocess.CalledProcessError:
        return False


def merge_locale(path: Path) -> tuple[int, int, int]:
    """Union-merge a single locale file. Returns (ours_count, theirs_count, merged_count)."""
    p = str(path)
    ours = _git_show(2, p)["translation"]
    theirs = _git_show(3, p)["translation"]

    # Union with collision policy: prefer non-empty value; on dual non-empty, ours wins.
    merged = dict(theirs)
    for k, v_ours in ours.items():
        v_theirs = theirs.get(k, "")
        if v_ours:
            merged[k] = v_ours
        elif v_theirs:
            merged[k] = v_theirs
        else:
            merged[k] = v_ours  # both empty

    # Sort keys for stable diff (matches `bun run i18n:extract` output order).
    output = {"translation": dict(sorted(merged.items()))}
    path.write_text(
        json.dumps(output, ensure_ascii=False, indent=2) + "\n", encoding="utf-8"
    )
    return len(ours), len(theirs), len(merged)


def main(argv: list[str]) -> int:
    roots = argv[1:] if len(argv) > 1 else DEFAULT_ROOTS
    locales: list[Path] = []
    for root in roots:
        root_path = Path(root)
        if not root_path.exists():
            print(f"  (skip: {root} does not exist)")
            continue
        locales.extend(p for p in root_path.glob("*.json") if _in_conflict(str(p)))

    if not locales:
        print("No i18n locale files are in conflict. Nothing to do.")
        return 0

    print(f"Resolving {len(locales)} conflicted locale file(s):")
    paths_to_stage: list[str] = []
    for path in locales:
        ours_n, theirs_n, merged_n = merge_locale(path)
        print(f"  {path}: ours={ours_n}, theirs={theirs_n}, merged={merged_n}")
        paths_to_stage.append(str(path))

    print("\nStaging merged files...")
    subprocess.check_call(["git", "add", "--"] + paths_to_stage)
    print("Done. Review with `git diff --cached` then commit the merge.")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
