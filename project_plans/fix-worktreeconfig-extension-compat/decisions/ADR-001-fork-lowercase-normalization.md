# ADR-001: Lowercase `extensionsValidForV0` Keys in go-git Fork (vs. Lookup-Time Normalization)

**Date**: 2026-11-04
**Status**: Accepted
**Deciders**: Tyler Stapler

---

## Context

`repository_extensions.go` in the `github.com/tstapler/go-git/v5` fork stores three extension
names in `extensionsValidForV0` with camelCase keys (`"partialClone"`, `"preciousObjects"`,
`"worktreeConfig"`). The `extensions()` function that populates the slice passed to
`verifyExtensions` lowercases every key with `strings.ToLower`, so the actual lookup keys are
`"partialclone"`, `"preciousobjects"`, `"worktreeconfig"`. All three lookups fail silently —
the keys are not found in the map, so the extensions land in `unsupported`, and
`verifyExtensions` returns `ErrUnsupportedExtensionRepositoryFormatVersion: worktreeconfig`
(or the others) for any repo that has these extensions set with `repositoryFormatVersion = 0`.

Two approaches can fix the case mismatch:

1. **Normalize the map keys** — change the three camelCase entries in `extensionsValidForV0`
   to their lowercase equivalents, matching what `extensions()` already produces.

2. **Normalize at lookup time** — keep the map keys as-is; add `strings.ToLower(ext.name)`
   before the `extensionsValidForV0[ext.name]` lookup in `verifyExtensions`.

A third approach — porting all `OpenRepo` callers to `gitprovider.Backend()` — eliminates the
dependency on `verifyExtensions` entirely for those callers, but would require touching 14+
call sites across `ops.go` and other files. This is out of scope for this bug's appetite.

---

## Decision

**Normalize the map keys** (approach 1): change all three camelCase entries in
`extensionsValidForV0` to lowercase. Also add `"worktreeconfig"` to `builtinExtensions`
(which already stores lowercase keys).

---

## Rationale

### Why approach 1 over approach 2

**Self-documenting data.** The map is a whitelist of exactly the extension names that are
recognized. Storing the canonical form (lowercase, matching the normalized key space that
`extensions()` produces) makes the invariant explicit: "every key in this map is lowercase."
A reader does not need to trace through `verifyExtensions` to understand why `"partialClone"`
is not in the map — it simply is not, because only lowercase keys belong here.

**Minimal diff.** Approach 1 is a three-character change per entry (remove capital letter).
Approach 2 adds a `strings.ToLower` call in `verifyExtensions`, which modifies logic rather
than data. The logic change is harder to review and easier to remove accidentally during a
future refactor of `verifyExtensions`.

**Consistency.** `builtinExtensions` already stores lowercase keys (`"noop"`, `"noop-v1"`).
After approach 1, both maps are consistent. After approach 2, the maps would remain
inconsistent — `extensionsValidForV0` with camelCase keys, `builtinExtensions` with
lowercase — requiring the normalization to exist in perpetuity.

**No behavior difference.** Both approaches produce identical runtime behavior. The choice is
purely about code clarity and future maintainability.

### Why not approach 3

Porting all `OpenRepo` callers to `gitprovider.Backend()` is the correct long-term direction
(the `resolveRemoteWorktreeBaseCommit` pattern proves it), but:
- 14+ callers in `ops.go` alone; more in `diff.go`, `branch_merge.go`, `worktree_*.go`.
- Each port requires understanding the caller's error-handling contract with `backend.ResolveRef`
  vs. the go-git plumbing API it currently uses — not a mechanical substitution.
- The appetite for this bug fix is "Small (1-2 days)"; a full migration is not.

The two directly-failing call sites (`ResolveOriginBranchSHA`, `ResolveLocalBranchSHA`) are
ported as Story 2.1.1 and 2.1.2. Remaining callers are pre-existing debt mitigated by the
fork fix.

---

## Consequences

**Positive:**
- `extensionsValidForV0` and `builtinExtensions` are both consistently lowercase.
- All existing `TestVerifyExtensions` cases continue to pass (no behavioral change).
- Future additions to either map follow an obvious convention.
- The fix is four lines of data change, making it easy to upstream to `go-git/go-git`.

**Negative / Accepted:**
- Requires publishing a new fork tag (`v5.19.3-ssq.2`) and updating `go.mod`, which is a
  blocking dependency on the fork publishing step (Story 1.1.4). The ops.go port (Phase 2)
  can be developed and merged independently to unblock the specific failing session-creation
  path while the fork is being published.
