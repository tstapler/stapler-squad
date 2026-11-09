# Implementation Plan: fix-worktreeconfig-extension-compat

**Feature**: Fix session creation failures caused by `extensions.worktreeConfig = true` in target repos
**Date**: 2026-11-04
**Status**: Ready for implementation
**ADRs**: ADR-001-fork-lowercase-normalization.md

---

## Domain Glossary

| Term | Definition |
|---|---|
| `verifyExtensions` | go-git fork function (`repository_extensions.go`) that validates a repo's extension list against supported-extension maps during `git.PlainOpen` / `OpenRepo` |
| `extensionsValidForV0` | Map in go-git fork listing the extension names permitted when `core.repositoryFormatVersion` is 0 or empty; lookup key is lowercased by `extensions()` |
| `builtinExtensions` | Map in go-git fork listing extensions the go-git core understands for `repositoryFormatVersion = 1` repos; must also contain `"worktreeconfig"` after the fix |
| `OpenRepo` | Stapler-squad wrapper around go-git's `PlainOpenWithOptions`; calls `verifyExtensions` and therefore fails today for repos with `worktreeConfig = true` |
| `vetExtension` | In-process backend function in `session/git/backend/gogit/layout.go`; returns `fallbackEligible` for `worktreeconfig = true`, intentionally routing such repos to the CLI backend |
| `vetRepoConfig` | Outer config-vetting function in `layout.go` that calls `vetExtension`; also returns `fallbackEligible` for any extension in a V0 repo (guard at line 336) |
| `fallbackEligible` | Error-wrapper type recognized by the backend Router; triggers CLI fallback rather than surfacing an in-process error |
| `gitprovider.Backend()` | Process-wide `backend.Backend` accessor (the Router); handles CLI fallback transparently for `worktreeConfig = true` repos |
| `backend.ResolveRef` | Backend interface method: `ResolveRef(ctx, loc, ref) (CommitSHA, error)` — resolves a git ref name to a commit SHA |
| `backend.Local` | Location type (`{Root: backend.RepoRoot(path)}`) for a local-disk repository, used to build calls into `gitprovider.Backend()` |
| `backend.RefName` | Typed string for a full git ref name (e.g. `"origin/main"`, `"refs/heads/main"`) |
| `resolveRemoteWorktreeBaseCommit` | Existing function in `session/git/ops.go` that correctly uses `backend.Backend.ResolveRef`; the prior-art pattern for the ops.go port |
| `worktreeConfig extension` | Git extension that enables per-worktree config file splitting; git writes it automatically to any repo that gains a linked worktree |
| `repositoryFormatVersion` | `core.repositoryformatversion` git config value; `0` (default) restricts valid extensions to `extensionsValidForV0` |
| `FetchBranch` | Function in `ops.go` that already correctly uses `gitprovider.Backend().Fetch`; unchanged by this fix |
| `v5.19.3-ssq.2` | The new fork tag that will carry the two-line `repository_extensions.go` fix |

---

## Pattern Decisions

| Decision | Chosen | Alternative Rejected | Reason |
|---|---|---|---|
| Fix `extensionsValidForV0` case mismatch | Lowercase all three camelCase keys in the map to match `extensions()`'s `strings.ToLower` output | Normalize at lookup time inside `verifyExtensions` | Maps are self-documenting; changing lookup logic obscures what keys are actually stored and would require touching `verifyExtensions` logic rather than pure data |
| Fix `ResolveOriginBranchSHA` / `ResolveLocalBranchSHA` | Port to `gitprovider.Backend().ResolveRef` (backend pattern) | Port all `OpenRepo` callers in ops.go to backend | 14+ callers are out of scope for this fix's appetite; the two named functions are the only ones in the failing session-creation path |
| `context.Background()` with timeout for ported functions | Use `context.Background()` + 10s timeout (matching `FetchBranch` timeout style) | Thread `context.Context` from callers | Callers (`ResolveDefaultBranchSHA`, `ResolveWorktreeBaseCommit`) are themselves non-context callers; API change propagation is a separate cleanup, out of scope |

---

## Tech Debt Disposition

**`session/git/ops.go` — remaining `OpenRepo` callers**

The 14+ other direct `OpenRepo` calls in `ops.go` (lines 275, 296, 352, 392, 434, 550, 742, 830, 948, 968) plus callers in `diff.go`, `branch_merge.go`, `scaffolding.go`, `worktree_git.go`, `worktree_ops.go` remain after this fix. The go-git fork fix mitigates their failure risk for `worktreeConfig = true` repos. Full port of all callers to `gitprovider.Backend()` is tracked separately (not gated by this fix). Disposition: **extend as-is** — the fork fix reduces risk; targeted port of the two in-scope functions; remaining callers are pre-existing debt, not regressed.

**`session/git/backend/gogit/enrichment_test.go:51` — `worktreeConfig = true` rejection assertion**

The test at line 51 currently asserts that `ReadEnrichment` returns an error for `[extensions]\n worktreeConfig = true\n`. After the fork fix, `verifyExtensions` no longer errors, but `vetRepoConfig`'s V0 guard (line 336 of `layout.go`) still fires for any extension in a V0 repo and returns `fallbackEligible`. The in-process backend route still fails; `ReadEnrichment` still returns an error. The test remains correct after this fix — no change required.

---

## Migration Plan

No data migration. No schema change. No config change. This fix ships as a go-git fork bump (`go.mod` replace directive version) + two function-body replacements in `ops.go`.

---

## Observability Plan

No new metrics or dashboards. The fix removes an error; monitor by absence:
- **Before fix**: `logs/staplersquad.log` contains `"failed to open git repo at ... core.repositoryformatversion does not support extension: worktreeconfig"` for every session creation attempt on affected repos.
- **After fix**: That log line no longer appears. Session creation succeeds; the first observable signal is a valid `Starting tmux session` log line.
- No new instrumentation required; existing session creation error logging is sufficient to confirm the fix.

---

## Risk Control

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Fork tag publishing fails or is slow | Low | Blocks Phase 1 | Phase 2 may be developed on a branch independently but **must not merge to main until Phase 1's go.mod bump lands in the same PR or has already merged**; landing Phase 2 alone leaves 14+ unported `OpenRepo` callers broken in production for `worktreeConfig = true` repos |
| `extensionsValidForV0` lowercase fix breaks a test expecting camelCase key behavior | Low | Test failure | The only existing tests are `TestVerifyExtensions` (5 cases); none test camelCase V0 keys; the new tests added in Story 1.1.3 are the regression guard |
| `ResolveRef("origin/"+branch)` DWIM expansion picks wrong ref | Very low | Wrong SHA returned | Occurs only when a local branch literally named `origin/main` exists; existing behavior of `resolveRemoteWorktreeBaseCommit` has the same DWIM behavior and has not been a problem in production |
| `enrichment_test.go:51` starts failing after fork fix | Low | CI failure | The V0 guard in `vetRepoConfig` fires before `vetExtension`; `ReadEnrichment` still returns error; test remains valid — verify this explicitly after applying the fork fix |

---

## Unresolved Questions

None. Architecture, pitfalls, and feature landscape are fully researched. The two changes are confirmed correct by the prior art in `resolveRemoteWorktreeBaseCommit` and by the `builtinExtensions`/`extensionsValidForV0` data inspection.

---

## Dependency Visualization

```
Phase 1: go-git fork fix
  Story 1.1.1 ─── fix extensionsValidForV0 (lowercase 3 keys)   ─┐
  Story 1.1.2 ─── add worktreeconfig to builtinExtensions         ├─► Story 1.1.4: publish v5.19.3-ssq.2
  Story 1.1.3 ─── add regression tests in fork                    ─┘
      │
      ▼ (requires published tag)
  Story 1.1.5: bump go.mod replace → v5.19.3-ssq.2

Phase 2: ops.go port (may be developed independently; MUST land with Phase 1)
  Story 2.1.1 ─── port ResolveOriginBranchSHA to backend.ResolveRef
  Story 2.1.2 ─── port ResolveLocalBranchSHA to backend.ResolveRef

⚠️  MERGE GATE: Phase 2 must not merge to main before Phase 1 (Story 1.1.5) is merged.
    Phase 2 alone leaves 14+ other OpenRepo callers broken; ship atomically.

Phase 3: verification (requires Phase 1 + Phase 2 complete)
  Story 3.1.1 ─── regression test in session/git/ package
  Story 3.1.2 ─── make test passes, verify enrichment_test.go:51 still passes
```

---

## Phase 1: Fix go-git fork

### Epic 1.1: Repair `repository_extensions.go` extension maps

**Goal**: Eliminate `ErrUnsupportedExtensionRepositoryFormatVersion: worktreeconfig` from `git.PlainOpen` / `OpenRepo` for any repo with `extensions.worktreeConfig = true`, at both `repositoryFormatVersion = 0` and `= 1`.

#### Story 1.1.1: Lowercase all camelCase keys in `extensionsValidForV0`
**As a** session worker, **I want** `OpenRepo` to succeed on repos where git wrote `extensions.worktreeConfig = true`, **so that** session creation pipelines that call `OpenRepo` directly are not blocked by a spurious extension-validation error.

**Acceptance Criteria**:
- `extensionsValidForV0` contains `"worktreeconfig"`, `"partialclone"`, `"preciousobjects"` (all lowercase).
  - *Given* a repo config with `core.repositoryFormatVersion = 0` and `extensions.worktreeConfig = true`, *When* `git.PlainOpenWithOptions` is called on that repo, *Then* it returns a non-nil `*Repository` with no error.
- `verifyExtensions` accepts `"partialclone"` and `"preciousobjects"` for V0 repos without error (implicit regression guard against narrowing the fix to only `worktreeConfig`).
  - *Given* a repo config with `core.repositoryFormatVersion = 0` and `extensions.partialClone = origin`, *When* `verifyExtensions` runs, *Then* it returns `nil`.

**Files**:
- `/home/tstapler/go/pkg/mod/github.com/tstapler/go-git/v5@v5.19.3-ssq.1/repository_extensions.go`

> Note: The module cache copy is read-only. The fix must be applied to a local writable clone of `github.com/tstapler/go-git` before tagging (see Story 1.1.4).

##### Task 1.1.1a: Clone fork to writable location
- Clone `git@github.com:tstapler/go-git.git` to `~/code/github.com/tstapler/go-git` (following the `~/code/<host>/<owner>/<repo>` convention; use `tyclone git@github.com:tstapler/go-git.git`).
- Verify checkout is on the tag `v5.19.3-ssq.1` (or the branch it was tagged from).
- Files: `~/code/github.com/tstapler/go-git/`

##### Task 1.1.1b: Apply lowercase fix to `extensionsValidForV0`
- In `repository_extensions.go`, change the three camelCase keys:
  - `"partialClone"` → `"partialclone"`
  - `"preciousObjects"` → `"preciousobjects"`
  - `"worktreeConfig"` → `"worktreeconfig"`
- Verify: `grep -n 'partialClone\|preciousObjects\|worktreeConfig' repository_extensions.go` returns no hits.
- Files: `~/code/github.com/tstapler/go-git/repository_extensions.go`

---

#### Story 1.1.2: Add `"worktreeconfig"` to `builtinExtensions`
**As a** session worker, **I want** `OpenRepo` to succeed on V1 repos with `extensions.worktreeConfig = true`, **so that** the fix covers both common `repositoryFormatVersion` values.

**Acceptance Criteria**:
- `builtinExtensions` contains `"worktreeconfig"`.
  - *Given* a repo config with `core.repositoryFormatVersion = 1` and `extensions.worktreeConfig = true`, *When* `git.PlainOpenWithOptions` is called, *Then* it returns a non-nil `*Repository` with no error.

**Files**:
- `~/code/github.com/tstapler/go-git/repository_extensions.go`

##### Task 1.1.2a: Add `"worktreeconfig"` entry to `builtinExtensions`
- Add `"worktreeconfig": {}` to `builtinExtensions` with a one-line comment: `// worktreeconfig enables per-worktree config splitting; go-git treats it as a no-op.`
- Files: `~/code/github.com/tstapler/go-git/repository_extensions.go`

---

#### Story 1.1.3: Add regression tests for all three formerly-broken V0 extensions
**As a** maintainer, **I want** test coverage for every camelCase V0 extension key, **so that** a future edit to `extensionsValidForV0` cannot silently re-break these cases.

**Acceptance Criteria**:
- New test cases in `TestVerifyExtensions` cover:
  1. `repositoryFormatVersion=0` + `extensions.worktreeConfig = true` → no error.
  2. `repositoryFormatVersion=0` + `extensions.partialClone = origin` → no error.
  3. `repositoryFormatVersion=0` + `extensions.preciousObjects = true` → no error.
  4. `repositoryFormatVersion=1` + `extensions.worktreeConfig = true` → no error.
  - *Given* a test case with `cfg.Raw.Section("extensions").SetOption("worktreeConfig", "true")` and `repositoryFormatVersion = "0"`, *When* `Open(st, nil)` is called, *Then* it returns a non-nil repo and no error.

**Files**:
- `~/code/github.com/tstapler/go-git/repository_extensions_test.go`

##### Task 1.1.3a: Add four new test cases to `TestVerifyExtensions`
- Add after the existing `repositoryformatversion=1: allows known extension` case:
  - `"repositoryformatversion=0: allows worktreeConfig=true"` using `SetOption("worktreeConfig", "true")`
  - `"repositoryformatversion=0: allows partialClone"` using `SetOption("partialClone", "origin")`
  - `"repositoryformatversion=0: allows preciousObjects"` using `SetOption("preciousObjects", "true")`
  - `"repositoryformatversion=1: allows worktreeConfig=true"` using `SetOption("worktreeConfig", "true")` with `Version_1`
- Run `go test ./... -run TestVerifyExtensions` in `~/code/github.com/tstapler/go-git/` and confirm 9 passing cases.
- Files: `~/code/github.com/tstapler/go-git/repository_extensions_test.go`

---

#### Story 1.1.4: Publish go-git fork tag `v5.19.3-ssq.2`
**As a** developer, **I want** a published, tagged go-git fork version containing the fix, **so that** the `go.mod` replace directive can reference a real module version.

**Acceptance Criteria**:
- Tag `v5.19.3-ssq.2` exists on `github.com/tstapler/go-git` and is fetchable.
  - *Given* the fix branch on `github.com/tstapler/go-git` has been pushed, *When* `go get github.com/tstapler/go-git/v5@v5.19.3-ssq.2` is run from any Go module, *Then* it downloads successfully.
- Tag commit message references the two bugs fixed.

**Files**: N/A (fork repository operations)

##### Task 1.1.4a: Commit, tag, and push to fork
- In `~/code/github.com/tstapler/go-git/`:
  - `git add repository_extensions.go repository_extensions_test.go`
  - `git commit -m "fix: lowercase extensionsValidForV0 keys; add worktreeconfig to builtinExtensions"`
  - `git tag v5.19.3-ssq.2`
  - `git push origin HEAD`
  - `git push origin v5.19.3-ssq.2`
- File an upstream PR to `go-git/go-git` with the same change (good citizenship; do not block this plan on its merge).

---

#### Story 1.1.5: Bump `go.mod` replace directive to `v5.19.3-ssq.2`
**As a** stapler-squad build, **I want** the go-git replace directive to point at the fixed fork version, **so that** `OpenRepo` uses the patched `verifyExtensions`.

**Acceptance Criteria**:
- `go.mod` line 232 reads `replace github.com/go-git/go-git/v5 => github.com/tstapler/go-git/v5 v5.19.3-ssq.2`.
  - *Given* the stapler-squad module with the bumped replace directive, *When* `go build ./...` runs, *Then* it succeeds with no errors.
- `go.sum` is updated.

**Files**:
- `/home/tstapler/Programming/stapler-squad/go.mod`
- `/home/tstapler/Programming/stapler-squad/go.sum`

##### Task 1.1.5a: Update replace directive and tidy
- Edit `go.mod` line 232: change `v5.19.3-ssq.1` to `v5.19.3-ssq.2`.
- Run `go mod tidy` in `/home/tstapler/Programming/stapler-squad/`.
- Run `go build ./...` and confirm zero errors.
- Files: `go.mod`, `go.sum`

---

## Phase 2: Port `ops.go` to backend pattern

### Epic 2.1: Remove direct `OpenRepo` dependency from ref-resolution entry points

**Goal**: Make `ResolveOriginBranchSHA` and `ResolveLocalBranchSHA` immune to `verifyExtensions` failures by routing through `gitprovider.Backend().ResolveRef` instead of `OpenRepo`.

#### Story 2.1.1: Port `ResolveOriginBranchSHA` to `backend.ResolveRef`
**As a** session creation pipeline, **I want** `ResolveOriginBranchSHA` to succeed for repos with `extensions.worktreeConfig = true`, **so that** `CreateBacklogWorktree` and other callers can branch from origin's true current tip without hitting `OpenRepo`.

**Acceptance Criteria**:
- `ResolveOriginBranchSHA` no longer calls `OpenRepo`.
  - *Given* a repo path where `extensions.worktreeConfig = true` and `origin/main` exists after a `FetchBranch`, *When* `ResolveOriginBranchSHA(repoPath, "main")` is called, *Then* it returns the fetched tip SHA with no error.
- `ResolveOriginBranchSHA` returns the correct SHA (fetched from origin, not ambient HEAD).
  - *Given* a repo where origin/main has advanced since the last local fetch, *When* `ResolveOriginBranchSHA` is called, *Then* it returns origin's current tip SHA, not the stale local copy.
- `("", backend.ErrRefNotFound)` from `ResolveRef` is wrapped as an error (not silently swallowed).
  - *Given* a successful `FetchBranch` for `"feature-x"` but no `origin/feature-x` ref exists after fetch, *When* `ResolveOriginBranchSHA(repoPath, "feature-x")` is called, *Then* it returns a non-nil error wrapping `backend.ErrRefNotFound` (note: `ErrObjectNotFound` is for a missing ODB object; `ErrRefNotFound` is for a missing ref name).

**Files**:
- `/home/tstapler/Programming/stapler-squad/session/git/ops.go`

##### Task 2.1.1a: Replace `OpenRepo` body with `backend.ResolveRef`
- In `ops.go`, replace lines 87–95 (from `repo, err := OpenRepo(repoPath)` through `return ref.Hash().String(), nil`) with:
  ```go
  ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
  defer cancel()
  loc := backend.Local{Root: backend.RepoRoot(repoPath)}
  sha, err := gitprovider.Backend().ResolveRef(ctx, loc, backend.RefName("refs/remotes/origin/"+mainBranch))
  if err != nil {
      return "", fmt.Errorf("failed to resolve origin/%s: %w", mainBranch, err)
  }
  return string(sha), nil
  ```
- Fully qualified `refs/remotes/origin/` (not `origin/`) so a local branch named `origin/<b>` cannot shadow the lookup (research/ops-resolve-sha-gap.md).
- Remove unused imports `plumbing` (if `ResolveLocalBranchSHA` still uses it, defer removal to Story 2.1.2).
- Files: `/home/tstapler/Programming/stapler-squad/session/git/ops.go`

##### Task 2.1.1b: Compile check
- Run `go build ./session/git/...` and confirm zero errors.
- Files: N/A

---

#### Story 2.1.2: Port `ResolveLocalBranchSHA` to `backend.ResolveRef`
**As a** session creation pipeline, **I want** `ResolveLocalBranchSHA` to succeed for repos with `extensions.worktreeConfig = true`, **so that** the offline fallback path (`ResolveDefaultLocalBranchSHA`) works correctly on affected repos.

**Acceptance Criteria**:
- `ResolveLocalBranchSHA` no longer calls `OpenRepo`.
  - *Given* a repo path where `extensions.worktreeConfig = true` and `refs/heads/main` exists locally, *When* `ResolveLocalBranchSHA(repoPath, "main")` is called, *Then* it returns the local branch's tip SHA with no error.
- Error wrapping preserves the original error.
  - *Given* a repo where `refs/heads/nonexistent` does not exist, *When* `ResolveLocalBranchSHA(repoPath, "nonexistent")` is called, *Then* it returns a non-nil error containing `"nonexistent"`.

**Files**:
- `/home/tstapler/Programming/stapler-squad/session/git/ops.go`

##### Task 2.1.2a: Replace `OpenRepo` body in `ResolveLocalBranchSHA`
- In `ops.go`, replace lines 104–112 (from `repo, err := OpenRepo(repoPath)` through `return ref.Hash().String(), nil`) with:
  ```go
  ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
  defer cancel()
  loc := backend.Local{Root: backend.RepoRoot(repoPath)}
  sha, err := gitprovider.Backend().ResolveRef(ctx, loc, backend.RefName("refs/heads/"+branchName))
  if err != nil {
      return "", fmt.Errorf("failed to resolve local branch %s: %w", branchName, err)
  }
  return string(sha), nil
  ```
- Remove the `plumbing` import from the import block if it is now unused (check for remaining `plumbing.` references in the file first).
- Files: `/home/tstapler/Programming/stapler-squad/session/git/ops.go`

##### Task 2.1.2b: Compile check
- Run `go build ./session/git/...` and confirm zero errors.
- Files: N/A

---

> **Amendment 2026-11-04 (review FAIL on backlog 9c33e913):** Phase 2 was never implemented; ops.go still called `OpenRepo`. Also: the `go.mod` replace is now `v5.19.3-ssq.3` (uncommitted); commit it only after `make test` and `make lint` pass. Add a missing-branch test asserting `errors.Is(err, backend.ErrRefNotFound)` for `ResolveLocalBranchSHA`. For `ResolveOriginBranchSHA` a missing branch fails at the fetch step (not ErrRefNotFound); reaching `ResolveRef` requires creating the branch on origin after cloning and running `git config --unset-all remote.origin.fetch` on the clone (adversarial-review-phase2.md). The `sha == ""` guard was dropped as dead code. Phase 2 scope is these two functions only; `IsUnbornRepo` (ops.go:296) still uses `OpenRepo` and is out of scope.

## Phase 3: Tests and verification

### Epic 3.1: Regression tests and CI validation

**Goal**: Confirm the full fix path works end-to-end with a targeted regression test, and that existing tests are not regressed.

#### Story 3.1.1: Add regression test for `ResolveOriginBranchSHA` with `worktreeConfig = true`
**As a** future maintainer, **I want** a test that would have caught Bug 3 before it shipped, **so that** porting `OpenRepo` callers back to direct calls is blocked by CI.

**Acceptance Criteria**:
- A new test in `session/git/` exercises `ResolveOriginBranchSHA` against a repo whose base `.git/config` has `[extensions]\nworktreeConfig = true`.
  - *Given* a local test repo with a remote, an `extensions.worktreeConfig = true` entry in its `.git/config`, and `main` on origin, *When* `ResolveOriginBranchSHA(repoPath, "main")` is called, *Then* it returns a non-empty SHA string and nil error.
- The test is deterministic and fast (no real network; uses `file://` remote or `git init --bare` local remote).
  - *Given* the test helper sets up a local disk origin (no external network required), *When* the test runs in CI without network access, *Then* it passes.

**Files**:
- `/home/tstapler/Programming/stapler-squad/session/git/ops_test.go` (new test case in existing file, or new `ops_worktreeconfig_test.go` if the existing file is large)

##### Task 3.1.1a: Write regression test
- Locate the test file for `ops.go` (likely `session/git/ops_test.go`).
- Add a test function `TestResolveOriginBranchSHA_WorktreeConfigExtension`:
  1. Create a bare local "origin" repo with `git init --bare` via `os/exec`.
  2. Clone it to a temp dir; add a commit; push.
  3. Append `[extensions]\nworktreeConfig = true\n` to `.git/config` in the clone.
  4. Call `ResolveOriginBranchSHA(cloneDir, "main")` and assert no error + 40-hex SHA.
- Files: `session/git/ops_test.go`

#### Story 3.1.2: Add regression test for `ResolveLocalBranchSHA` with `worktreeConfig = true`
**As a** future maintainer, **I want** a test that would have caught Bug 3 in `ResolveLocalBranchSHA`, **so that** both ported functions are covered.

**Acceptance Criteria**:
- A new test exercises `ResolveLocalBranchSHA` against a repo with `extensions.worktreeConfig = true`.
  - *Given* a local test repo with `extensions.worktreeConfig = true` in its `.git/config` and a local branch `main`, *When* `ResolveLocalBranchSHA(repoPath, "main")` is called, *Then* it returns a non-empty SHA string and nil error.

**Files**:
- `/home/tstapler/Programming/stapler-squad/session/git/ops_test.go`

##### Task 3.1.2a: Write regression test for `ResolveLocalBranchSHA`
- Add `TestResolveLocalBranchSHA_WorktreeConfigExtension` alongside the `ResolveOriginBranchSHA` test from Story 3.1.1.
- Reuse the same `setupTestRepo` helper (or create one if absent): init repo, add commit, append `[extensions]\nworktreeConfig = true\n` to `.git/config`, call `ResolveLocalBranchSHA(repoPath, "main")` and assert no error + 40-hex SHA.
- Files: `session/git/ops_test.go`

---

#### Story 3.1.3: Verify `enrichment_test.go:51` still passes after fork bump
**As a** maintainer, **I want** confirmation that `TestReadEnrichment_RejectsIncludesAndUnsupported` still passes after the go-git fork bump, **so that** the in-process backend's correct CLI-fallback behavior for `worktreeConfig = true` is not accidentally removed.

**Acceptance Criteria**:
- `TestReadEnrichment_RejectsIncludesAndUnsupported` passes unchanged after the fork bump.
  - *Given* `enrichment_test.go:51`'s input `[extensions]\n worktreeConfig = true\n`, *When* `ReadEnrichment` is called after the fork bump, *Then* it still returns a non-nil error (the V0 guard in `vetRepoConfig` fires before `vetExtension`; in-process backend falls back to CLI — `ReadEnrichment` does not use the CLI path and correctly errors).
- Verification method: run `go test ./session/git/backend/gogit/ -run TestReadEnrichment_RejectsIncludesAndUnsupported -v` and confirm PASS.

**Files**:
- `/home/tstapler/Programming/stapler-squad/session/git/backend/gogit/enrichment_test.go` (read-only verification; no changes required)

##### Task 3.1.2a: Run targeted test and confirm pass
- After Story 1.1.5 (go.mod bump), run:
  `cd /home/tstapler/Programming/stapler-squad && go test ./session/git/backend/gogit/ -run TestReadEnrichment_RejectsIncludesAndUnsupported -v`
- Confirm output contains `PASS`.
- Files: N/A

#### Story 3.1.3: Full `make test` green
**As a** CI gate, **I want** `make test` to pass with both fixes applied, **so that** the PR can merge.

**Acceptance Criteria**:
- `make test` exits 0.
  - *Given* both the fork bump (Phase 1) and the ops.go port (Phase 2) are applied, *When* `make test` runs in `/home/tstapler/Programming/stapler-squad/`, *Then* all test suites pass with exit code 0.
- `make lint` exits 0.
  - *Given* the same changes, *When* `make lint` runs, *Then* no lint errors are reported.

**Files**: N/A

##### Task 3.1.3a: Run full test and lint suite
- Run `make build && make test` in `/home/tstapler/Programming/stapler-squad/`.
- Run `make lint`.
- If either fails, diagnose and fix before claiming completion.
- Files: N/A
