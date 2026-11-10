# Build vs Buy: `worktreeConfig` Extension Compatibility Fix

**Date:** 2026-11-03  
**Question:** How should the `extensionsValidForV0` case-sensitivity bug be fixed?

---

## Root Cause

`repository_extensions.go`'s `extensions()` function (line 73) normalises every key to lowercase via `strings.ToLower(opt.Key)` before returning it. The `extensionsValidForV0` map, however, still uses camelCase keys:

```go
extensionsValidForV0 = map[string]struct{}{
    "noop":            {},
    "partialClone":    {},    // ← "partialclone" after ToLower, no match
    "preciousObjects": {},    // ← "preciousobjects" after ToLower, no match
    "worktreeConfig":  {},    // ← "worktreeconfig" after ToLower, no match
}
```

A repository whose `.git/config` contains `worktreeConfig = true` (set by `git worktree add`) has its key normalised to `"worktreeconfig"`, which does not match `"worktreeConfig"` in the map. `verifyExtensions` therefore falls into the `ErrUnsupportedExtensionRepositoryFormatVersion` path and rejects the open.

The fix is two lines — lowercase the three non-`noop` keys:

```go
extensionsValidForV0 = map[string]struct{}{
    "noop":            {},
    "partialclone":    {},
    "preciousobjects": {},
    "worktreeconfig":  {},
}
```

---

## Finding 1 — Fork vs Upstream Are Identical on This File

```
diff upstream/v5.19.2/repository_extensions.go \
     tstapler/v5.19.3-ssq.1/repository_extensions.go
(no output — files are byte-for-byte identical)
```

`README-SSQ.md` in the fork confirms: "Zero patches today. `ssq/v5` equals upstream `v5.19.3` plus two fork-only files." The fork is a pure module shim; `repository_extensions.go` has never been patched.

The bug is **inherited from upstream**, not introduced by the fork.

---

## Finding 2 — Upstream v5.19.x Has Not Fixed This

The module cache only contains `go-git/go-git/v5@v5.19.2` and the fork's `v5.19.3-ssq.1`. Both have the identical camelCase map.

Web search and issue tracker research found no merged upstream PR that fixes the case mismatch in `extensionsValidForV0`. Community reports (Chainloop issue #2966, claudeissues.com #34225 and #75804) confirm the bug is still present in v5.17.1 through at least mid-2026, with workarounds being either pinning to v5.16.5 or manually unsetting the extension flag.

There is no evidence a v5.20+ or v6 release has landed that addresses this. The go-git project moves slowly on extension support.

---

## Finding 3 — Fork's Purpose and Scope

Per `README-SSQ.md`: the fork exists specifically to hold Go-source patches that cannot go upstream quickly enough. The rebase runbook is documented. It is the correct vehicle for a fix that upstream has not yet merged.

The only differences from upstream are:
- `.github/workflows/ssq-ci.yml` (CI for the fork)
- `README-SSQ.md` (this file)
- Several non-`repository_extensions.go` files (`config/config.go`, `plumbing/object/tree.go`, `plumbing/revlist/revlist.go`, transport HTTP files, `repository.go`) — presumably included from the upstream v5.19.3 delta vs v5.19.2, not custom patches

---

## Options Evaluated

| Option | Effort | Risk | Ships when? |
|---|---|---|---|
| **1. Fix in fork** (`extensionsValidForV0` lowercase, bump to `v5.19.3-ssq.2`) | ~30 min | Minimal — 2-line change, well-understood fork mechanics | Immediately after CI |
| **2. Fix upstream** (`go-git/go-git` PR) | Hours to days (review, CI, release) | Upstream may not merge quickly; bug pre-dates v5.17.1 | Unknown; months likely |
| **3. Stapler-squad workaround** (strip/ignore extension before `OpenRepo`) | ~1 hour | Hides the real bug; adds stapler-squad complexity; `OpenRepo` callers are numerous | Immediately |
| **4. Upgrade to v6** | Unknown; v6 API is not stable and not in the module cache | High migration risk | Unknown |

---

## Recommendation: Fix in the Fork (Option 1), File Upstream PR in Parallel (Option 2)

**Primary action:** Apply the 2-line lowercase fix to `repository_extensions.go` in `tstapler/go-git`, tag `v5.19.3-ssq.2`, and bump the `replace` directive in `stapler-squad/go.mod`. This is exactly why the fork exists.

**Secondary action:** Open a PR to `go-git/go-git` with the same fix and a regression test (a worktree repo opened with `worktreeConfig = true`). This is good citizenship and the right long-term resolution, but do not block the stapler-squad fix on it.

**Do not use a stapler-squad workaround** — it adds indirection without fixing the root cause, complicates every future `OpenRepo` call site, and would need to be removed once the fork carries the fix anyway.

**Do not pursue v6** — no evidence it fixes this, and the API migration cost is unrelated to this bug.

---

## Verification Plan

After patching the fork:

1. Create a bare repo, run `git worktree add`, confirm `.git/config` has `worktreeConfig = true`.
2. `go test ./... -run TestOpenWorktreeConfigRepo` (new regression test) in the fork.
3. In stapler-squad, run `make test` with the bumped `replace` line — existing worktree lifecycle tests exercise `OpenRepo` on worktrees.
