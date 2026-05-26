# Upstream Sync Playbook

How to merge `QuantumNous/new-api` (upstream) into our fork's `main` branch
without it becoming a multi-hour ordeal. Based on the lessons from the
2026-05 sync (211 commits across v0.13.0 → v1.0.0-rc.8), which is the
worst-case we should ever encounter again — if this playbook is followed.

---

## TL;DR — happy path

```bash
# 1. Refresh remotes
git fetch upstream
git checkout main && git pull origin main

# 2. Identify upstream tags we don't have yet
BASE=$(git merge-base main upstream/main)
git tag --sort=creatordate | awk -v base="$BASE" 'started || $0 ~ /v[0-9]/ { print }' \
    | head -5   # picks the next ~5 release tags to merge in order

# 3. Merge tag-by-tag (small batches), resolving conflicts per the patterns below
for TAG in <each tag in chronological order>; do
    git merge $TAG --no-edit
    # If conflicts arise:
    #   - i18n locales:        python3 scripts/merge_upstream_i18n.py
    #   - JSX/Go conflicts:    see "Common conflict patterns" below
    #   - Verify:              go build ./... && go test ./controller/ ./model/ ./service/ -count=1 -short
    #   - Commit:              git commit (auto-message OK for small batches)
done

# 4. Final pass: merge upstream/main HEAD to pick up any commits past the last tag
git merge upstream/main --no-edit

# 5. Push when satisfied
git push origin main
```

---

## When to sync

**Cadence: bi-weekly.** This is the most important rule in this document.

- Every 2 weeks, **someone** (assign an owner per quarter) pulls the last 2
  weeks of upstream commits. Typical delta: 10–40 commits, 0–4 conflict files.
- The 2026-05 sync took 4 hours because **2 months of upstream had accumulated**.
  At bi-weekly cadence, the same volume would have been 4 × 30-minute sessions
  — total time roughly the same, but the per-session difficulty is bounded.

**Trigger an out-of-cycle sync** when upstream releases a tag containing:
- A security advisory affecting code we use
- A breaking change in a function signature we call (e.g. `model.Recharge`'s
  Stripe-only refactor in v0.13.1)
- A directory rename (e.g. `web/src` → `web/classic/src` in v1.0.0-rc.1)

---

## Setup (one-time per fresh clone)

```bash
git remote -v   # confirm upstream points to QuantumNous
# If missing:
#   git remote add upstream https://github.com/Calcium-Ion/new-api.git
git fetch upstream --tags
```

The `upstream` remote should point at `QuantumNous/new-api` (or the
appropriate maintained fork). The `origin` remote is our own fork
(`dreamlx/new-api`).

---

## Batch strategy

**Merge by upstream release tag, not all-at-once.** This gives natural
semantic checkpoints:

- v0.13.x patch tags are usually small (5–20 commits each)
- v1.0.0-rc.x release candidates are usually 5–25 commits
- Major rewrites (e.g. v1.0.0-rc.1's frontend split) get their own batch
  so the conflict reflection happens on that batch alone

Pick the next tag chronologically; merge it; resolve; verify; commit;
repeat. If a batch has too many conflicts to reason about, abort
(`git merge --abort`) and split further (cherry-pick individual commits).

---

## Common conflict patterns and how to resolve them

These are the conflict types that recurred during the 2026-05 sync.
Each shows up multiple times across upstream releases, so internalize them.

### Pattern 1: Both sides extend the same registry/list

**Files where this happens repeatedly:**
- `controller/topup.go` — `GetTopUpInfo` payment provider registration
- `router/api-router.go` — webhook + pay route groups
- `model/option.go` — switch-case for setting keys
- `web/classic/src/components/settings/PaymentSetting.jsx` — provider imports + state
- `web/classic/src/components/topup/{RechargeCard,index}.jsx` — payment buttons

**Resolution: union.** Take both sides' additions. Order within the list
usually doesn't matter; if it does, follow upstream's grouping (Stripe →
Creem → Waffo → WaffoPancake → ours).

### Pattern 2: Struct field addition

**Files where this happens:**
- `model/topup.go` — TopUp struct (we added v2 fields; upstream adds PaymentProvider, etc.)
- `model/user.go` — User struct (we added external user fields; upstream adds CreatedAt/LastLoginAt)
- `model/log.go` — Log struct (upstream adds UpstreamRequestId, etc.)

**Resolution: keep ours + add theirs.** Our extension fields and upstream's
new fields almost never overlap semantically. Place upstream's fields in
their original group (often before our extension fields), then ours below
a `// External user integration fields` (or similar) comment.

### Pattern 3: i18n locale JSON conflicts

Conflicts in all 7 of `web/classic/src/i18n/locales/*.json` (and now
`web/default/src/i18n/locales/*.json` too).

**Resolution:**
```bash
python3 scripts/merge_upstream_i18n.py
```

The script reads stage 2 (ours) and stage 3 (theirs) of each conflicted
locale file, unions the `translation` dict (our values win on non-empty
collision), writes back sorted, and stages the result. Inspect with
`git diff --cached` before committing.

### Pattern 4: Upstream function signature drift

**Examples we've hit:**
- `model.GetAllLogs / model.SumUsedQuota` — upstream keeps adding filter
  parameters (`requestId`, then `upstreamRequestId`)
- `model.CompleteSubscriptionOrder` — went 2→3→4 args across two patches
- `model.Recharge` — was generic; upstream made it Stripe-only

**Resolution:** When upstream changes a function's signature, update all
of our additional callers (`controller/topup_paypal.go`, `topup_alipay.go`,
`topup_wxpay.go`, etc.). If upstream's function gained payment-specific
semantics (like Stripe-only), introduce a parallel function for our
payment method (`RechargePayPal`, modeled after `RechargeCreem` /
`RechargeWaffo`).

### Pattern 5: Directory rename (rare, high-impact)

Hit once in v1.0.0-rc.1: `web/src/...` was renamed to `web/classic/src/...`
because upstream introduced a new default frontend at `web/default/`.

**Resolution:** Git's merge driver auto-suggests moving conflicting files
to the new location (status `AU` = "added by us, unmerged"). `git add` the
new paths to accept the move. Verify with `git ls-files | grep web/src/`
that the old path is empty afterwards.

**Prevention:** `scripts/check_upstream_conformity.sh` rejects new files
in the (now obsolete) `web/src/` tree.

### Pattern 6: Embed target moved

`main.go` `//go:embed` directives point at `web/classic/dist` and
`web/default/dist`. After a directory rename, these may not exist on
your machine until you run `bun run build` in each frontend. For local
build verification (without running the frontend build):

```bash
mkdir -p web/classic/dist web/default/dist
echo '<!doctype html>' > web/classic/dist/index.html
echo '<!doctype html>' > web/default/dist/index.html
```

These placeholder files are gitignored (`.gitignore` already has
`web/*/dist`).

---

## Verification checklist

After each merge commit, run **all** of these before moving to the next batch:

```bash
# Conflict markers all resolved
git diff --check

# Go side compiles
go build ./...

# Go side passes vet (some upstream packages have pre-existing warnings
# in common/custom-event.go and several relay/channel/*/adaptor.go;
# filter those out to spot regressions)
go vet ./... 2>&1 | grep -v "common/custom-event\|unreachable code"

# Touched-package tests pass
go test ./controller/ ./model/ ./middleware/ ./service/ -count=1 -short

# (Optional, for big batches) full test sweep
go test ./... -count=1 -short
```

If any step fails, **fix it in the same batch's merge commit** rather than
deferring — a deferred fix means the next merge starts on broken code.

---

## Recovery

### Abort a single batch
```bash
git merge --abort
```
Safe at any point during conflict resolution. Resets to pre-merge state;
untracked files are not touched.

### Undo the last completed batch
```bash
# If not pushed yet:
git reset --hard HEAD~1   # destroys the merge commit and its resolution

# If pushed: revert the merge commit (do not force-push):
git revert -m 1 <merge-commit-sha>
```

### Bail out of the whole sync
If a batch reveals a structural problem too big to handle in-session
(e.g. realizing the whole approach needs revision), keep all completed
batches, but leave the in-progress merge unresolved and switch branches.
Come back later from the same point. Other team members are not affected
because nothing is pushed yet.

---

## Lessons from the 2026-05 big sync (record, not procedure)

These are observations we want to remember, even after this playbook
gets older:

- **The "this batch is bigger than expected" sensation is a signal.** When
  v0.13.0's 81-commit batch first conflicted, it was tempting to think
  "just a few conflicts, push through." In reality, 81 commits had layered
  changes touching the same payment registry 3 times. Splitting into
  smaller batches would have been faster than one big merge.
- **The biggest single structural change in the year was the `web/`
  rename.** It was telegraphed by upstream (v1.0 announcement); we caught
  it after the fact during a 2-month-late sync. With bi-weekly cadence
  we'd have caught it within 2 weeks and addressed it as the only conflict
  in a small batch.
- **i18n conflicts are unavoidable but cheap.** The `merge_upstream_i18n.py`
  script makes them a ~10-second action regardless of how many locales /
  keys conflict.
- **Function-signature drift is the most subtle pain.** Upstream adds a
  parameter to `model.GetAllLogs`; our `controller/log.go` callers break;
  the failure is at compile time, easy to fix, but easy to miss if you
  only build a subset.

---

## Process / governance

These are not technical steps, but they keep this playbook viable:

1. **Bi-weekly sync owner.** Rotates per quarter. Owner does the sync,
   commits the merges, and pushes. ~30 minutes per session.
2. **MR conformity check.** Every PR runs `scripts/check_upstream_conformity.sh`
   (CI or pre-commit hook). Prevents new files landing in obsoleted
   directories, prevents direct `encoding/json` usage, prevents WiseModel
   code on `main`.
3. **PR review for `main`.** Any PR touching `main` should be reviewed by
   the CTO (or whoever owns upstream sync). Customer-specific code (LH,
   WiseModel) belongs on its respective branch, never on `main`.
4. **Customer branches sync from main.** After each `main` sync,
   `lh-main` / `wisemodel-main` / etc. merge `main` into themselves
   (their owner does this; they handle customer-specific conflicts).
