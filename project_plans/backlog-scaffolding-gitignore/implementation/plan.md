# Implementation Plan: backlog-scaffolding-gitignore

**Feature**: Fix `addWorktreeExcludes` to resolve `info/exclude` via `--git-common-dir` (not `--git-dir`) so it is actually honored in linked worktrees, add `GetWorktreeDirtyPaths` to enumerate specific dirty paths, and use it to make `request_review`'s uncommitted-changes rejection name real paths instead of a blanket `git add -A` instruction.
**Date**: 2026-08-10
**Status**: Ready for implementation
**ADRs**: None — small, well-precedented infra fix reusing patterns already proven elsewhere in this codebase (`session/git/util.go:150-161`'s `--git-common-dir` resolution; `session/vc/git_provider.go`'s porcelain parsing). No novel or debatable technology choice.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `GitCommonDir` | The shared `.git` administrative directory a main repo and every linked worktree created from it both point to (resolved via `git rev-parse --path-format=absolute --git-common-dir`); `info/exclude` written here is honored by `git status` in every worktree. | Not a Go type — a concept named in comments/log lines. Existing precedent: `session/git/util.go:150-161`'s `findMainRepoPathForWorktree`. |
| `WorktreePrivateAdminDir` | The per-worktree admin directory returned by `git rev-parse --git-dir` for a linked worktree (e.g. `<main>/.git/worktrees/<session-name>`); an `info/exclude` written here is silently ignored by `git status`. | This is the directory `addWorktreeExcludes` was wrongly writing to — the root cause of the bug. |
| `addWorktreeExcludes` | Existing function (`session/backlog_commands.go:241`) that appends `git.ScaffoldingExcludePatterns` to a worktree's shared `info/exclude` file. | Only its exclude-file *path resolution* changes; its append/dedup logic is untouched. |
| `ScaffoldingExcludePatterns` | Existing `[]string` (`session/git/scaffolding.go:27`) — the single source of truth for scaffolding patterns (`.backlog-context.md`, `.claude/commands/backlog/`, `web-app/.next/`). | Already shipped; not modified by this plan. |
| `GetWorktreeDirtyPaths` | New function (`session/backlog_review.go`) returning the specific `[]string` of dirty paths in a worktree — untracked, modified, and renamed/copied (new path only) — deduplicated, or `(nil, nil)` if `worktreePath` is not a git repository at all. | Additive sibling to `IsWorktreeDirty`; does not replace it. |
| `IsWorktreeDirty` | Existing function (`session/backlog_review.go:626`) returning only a `bool` for whether a worktree has uncommitted changes. | Unmodified. Two callers: `server/mcp/tools_backlog.go:385` (in scope) and `session/review_gate.go:153` (out of scope, untouched). |
| `DirtyPath` | A single string path (repo-root-relative) returned by `GetWorktreeDirtyPaths`, sourced from `vc.FileChange.Path` — for renames this is the *new* path (the file's current location); `vc.FileChange.OldPath` is not surfaced in the string list. | Not a distinct Go type — plain `string` in a `[]string`; keeping it a primitive here is intentional (see Pattern Decisions — no value-object win over a raw path string for this call site). |
| `vc.FileChange` | Existing struct (`session/vc/types.go:64`) describing one changed path: `Path`, `Status`, `IsStaged`, `OldPath`, `Additions`, `Deletions`. | Reused as-is; not modified. |
| `vc.GitProvider` | Existing type (`session/vc/git_provider.go:21`) wrapping `git status --porcelain=v2 -z` via `GetChangedFiles()`. | `GetWorktreeDirtyPaths` is a thin adapter over `vc.NewGitProvider(worktreePath).GetChangedFiles()`. |
| `formatDirtyPathsRejectionMessage` | New unexported function (`server/mcp/tools_backlog.go`) that renders `request_review`'s rejection text from a `[]string` of dirty paths, capped at `maxRejectionMessagePaths` with an "...and N more" suffix. | Single call site; deliberately not an interface (see Pattern Decisions). |
| `setupLinkedWorktree` | New test helper (`session/backlog_commands_test.go`) building a real linked (`git worktree add`-backed) fixture on top of `setupTestGitRepo` + `git.NewGitWorktree(repoPath, name).Setup()`, registering `t.Cleanup` for teardown. | Distinguishes "linked worktree" tests (this bug's real topology) from `setupTestGitRepo`'s plain-repo tests (which cannot detect the `--git-dir`/`--git-common-dir` bug class — see pitfalls.md). |
| RED/GREEN regression demonstration | The manual two-point verification (Task 1.1.2b runs the new test against pre-fix code and records a FAIL; Task 1.2.1b reruns the identical test post-fix and records a PASS) that satisfies AC1's "fails before the fix, passes after" requirement without needing two separate permanent test files. | Not a Go construct — a task-execution convention recorded in this plan and in commit history. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Exclude-file path resolution | Small procedural fix in place (Transaction Script) — swap the subshell flag, keep the function shape | `session/git/util.go:150-161` (`findMainRepoPathForWorktree` already resolves `--path-format=absolute --git-common-dir`); research/stack.md | Export and call `findMainRepoPathForWorktree` from package `session` | That helper returns the *main repo root* (parent of the common dir), not the common dir itself — `addWorktreeExcludes` would have to re-derive `<root>/.git/info/exclude`, assuming a non-bare, standard `.git` layout. Resolving `--git-common-dir` directly (as this plan does) is more robust and is exactly what stack.md recommends: keep the subshell, swap the flag. |
| Dirty-path enumeration | Adapter over existing parser — `GetWorktreeDirtyPaths` adapts `vc.GitProvider.GetChangedFiles()`'s `[]FileChange` to a deduplicated `[]string` | `session/vc/git_provider.go:158-213` (`GetChangedFiles`/`parsePorcelainV2Z`); research/build-vs-buy.md | go-git's `Worktree.Status()` | Documented, measured cost already on record in this codebase: `session/unfinished/gogit_vcs_reader.go:861` notes go-git's `Worktree.Status()` causes a 1.85 GB allocation hashing every modified file's full content — that package built a stat-based approach specifically to avoid this. Reusing the CLI-porcelain path avoids reintroducing a known-bad cost. |
| Dirty-path enumeration (parsing) | Reuse `parsePorcelainV2Z` as-is; no new parsing code | research/build-vs-buy.md, requirements.md AC2 | Reimplement porcelain-v2 parsing inline in `GetWorktreeDirtyPaths` | `parsePorcelainV2Z` already handles NUL-delimited records, unquoted paths with spaces, and rename/copy continuation tokens correctly, and is exercised by its own tests. AC2 explicitly requires rename correctness be "preserved/reused, not reimplemented naively" — a fresh implementation risks silently regressing that. |
| `request_review` rejection message | Plain function, no interface | `.claude/rules/interface-pollution-checklist.md` (smell #1: speculative interface) | Define a `RejectionMessageFormatter` interface with one implementation | Exactly the "speculative interface" smell this repo's own checklist flags — one call site (`server/mcp/tools_backlog.go:385`), no near-term second implementation or variant behavior. |
| `IsWorktreeDirty` / `GetWorktreeDirtyPaths` relationship | Additive sibling function, `IsWorktreeDirty` untouched | research/architecture.md ("must be ADDITIVE, not a replacement"; 2 callers, only 1 in scope) | Merge into one function `(bool, []string, error)` and update both call sites | `session/review_gate.go:153`'s caller is explicitly out of scope. Changing `IsWorktreeDirty`'s signature would force an unrelated file edit there purely to keep it compiling, violating the "single-function blast radius" finding and adding risk with no requirement driving it. |
| Testing strategy for the `request_review` call-site wiring (AC3) | Unit-test the pure formatter (`formatDirtyPathsRejectionMessage`) directly; rely on pre-existing `TestRequestReview_*` tests (unmodified) to prove the surrounding paths didn't regress | `session/ent_repository.go:1170-1178` — `EntRepository.CreateSession`/`UpdateSession` are unimplemented stubs (`"CreateSession not yet implemented for EntRepository"`) | Build a full `Storage`-backed `Session`+`Worktree` ent fixture (via the production `AddInstance`→`saveInstancesToRepo` path) to integration-test `requestReview`'s real dirty-worktree rejection end-to-end | The only way to attach real `GitWorktreeData` to a session UUID that `GetWorktreeDataBySessionUUID` can read back is the full production `Instance`/`gitManager` session-creation path — disproportionate for testing a message-formatting call site in a Complexity-2 fix. The pure-formatter test plus the unchanged `TestRequestReview_*` suite together cover the observable behavior without that cost. |

---

## Migration Plan
N/A — no schema or data changes. `GetWorktreeDirtyPaths` and `formatDirtyPathsRejectionMessage` are new pure/adapter functions; `addWorktreeExcludes`'s change is a path-resolution fix with no persisted-data shape change.

## Observability Plan
- **Logs**: `server/mcp/tools_backlog.go`'s existing `log.InfoLog.Printf("[mcp:request_review] rejected: uncommitted changes ...")` line is extended to include the resolved `paths` slice. If `session.GetWorktreeDirtyPaths` itself errors (should be rare — it only runs after `IsWorktreeDirty` already succeeded against the same worktree), a new `log.WarningLog.Printf("[mcp:request_review] GetWorktreeDirtyPaths failed for session=%s worktree=%s: %v", ...)` line fires and the handler falls back to the generic (path-less) rejection wording rather than failing the RPC.
- **Metrics**: none — both new functions are sub-100ms local git subshell/CLI calls on an already-existing rejection path (no new persistent operation, no new hot path).
- **Alerts**: no new alerts required.

## Risk Control
- **Feature flag**: not gated — this is a bug fix (exclude-file resolution) plus an internal message-wording change, not new user-facing behavior requiring staged exposure.
- **Rollback procedure**: standard revert via PR close + revert commit. No data migration to unwind.
- **Staged rollout**: full rollout on merge.
- **Known accepted risks (not fixed by this plan, per requirements.md scope)**:
  - A latent, low-severity TOCTOU race in `addWorktreeExcludes`'s unlocked read-then-append to `info/exclude` (the existing `untrackMu` lock in `session/git/scaffolding.go` only covers `UntrackScaffolding`, not this function) — pitfalls.md confirms no known concurrent trigger today; worst case is a duplicate pattern line, not corruption. Explicitly out of scope per pitfalls.md ("do NOT let it block or scope-creep the primary fix").
  - No automated check that `IsWorktreeDirty` keeps shelling to `git status --porcelain` rather than switching to go-git's `Worktree.Status()` (requirements.md explicitly marks this out of scope, "no evidence needed"). Enforced only by code review + this plan's tasks never touching `IsWorktreeDirty`'s body.

## Unresolved Questions
None.

## Dependency Visualization

```
Epic 1.1 (RED baseline)              Epic 1.2 (fix)                  Epic 1.3 (dirty paths + message)
┌─────────────────────────┐          ┌──────────────────────────┐    ┌────────────────────────────────┐
│ 1.1.1a setupLinkedWorktree│          │ 1.2.1a swap --git-dir →  │    │ 1.3.1a GetWorktreeDirtyPaths    │
│         helper            │──┐       │ --git-common-dir + delete │    │         implementation          │
└─────────────────────────┘  │       │ dead fallback              │    └────────────┬────────────────────┘
              │               │       └────────────┬───────────────┘                 │
              ▼               │                    │                                 ▼
┌─────────────────────────┐  │                    ▼                    ┌────────────────────────────────┐
│ 1.1.2a write regression   │◄─┘       ┌──────────────────────────┐    │ 1.3.1b table-driven unit tests  │
│ test (still failing code) │          │ 1.2.1b rerun 1.1.2a's test│    │ (dirty/clean/renamed)           │
└────────────┬─────────────┘          │ → GREEN; rerun existing   │    └────────────┬────────────────────┘
              ▼                        │ scaffolding tests → pass  │                 │
┌─────────────────────────┐          └────────────┬───────────────┘                 ▼
│ 1.1.2b run test against   │                       │                    ┌────────────────────────────────┐
│ pre-fix code → RED        │                       ▼                    │ 1.3.2a formatDirtyPathsRejection│
│ (recorded, not a task dep)│          ┌──────────────────────────┐    │         Message                 │
└────────────────────────────┘          │ 1.2.1c new test: plain    │    └────────────┬────────────────────┘
                                        │ repo --git-dir ==         │                 │
                                        │ --git-common-dir (closes  │                 ▼
                                        │ gap on AC4)                │    ┌────────────────────────────────┐
                                        └──────────────────────────┘    │ 1.3.2b wire request_review call │
                                                                          │ site to use GetWorktreeDirtyPaths│
                                                                          │ + formatDirtyPathsRejectionMsg   │
                                                                          └────────────┬────────────────────┘
                                                                                       │
                                                                                       ▼
                                                                          ┌────────────────────────────────┐
                                                                          │ 1.3.2c unit tests for the       │
                                                                          │ formatter (lists paths, caps N) │
                                                                          └────────────┬────────────────────┘
                                                                                       │
                                                                                       ▼
                                                                          ┌────────────────────────────────┐
                                                                          │ 1.3.2d integration test:        │
                                                                          │ scaffolding-only linked worktree│
                                                                          │ → GetWorktreeDirtyPaths == []   │
                                                                          │ (needs 1.1.1a helper + 1.2.1a   │
                                                                          │ fix + 1.3.1a function)          │
                                                                          └────────────────────────────────┘
```

**Sequencing constraint**: Epic 1.3's Story 1.3.2 (specifically Task 1.3.2d) must land *after* Epic 1.2's fix — a scaffolding-only linked worktree cannot report zero dirty paths until `addWorktreeExcludes` actually excludes it. Within Epic 1.1/1.2, Task 1.1.2b (RED) must run *before* Task 1.2.1a (the fix) so the pre-fix failure is genuinely observed, not assumed.

---

## Phase 1: Fix scaffolding exclusion and surface specific dirty paths

### Epic 1.1: Prove the bug with a real linked-worktree fixture (RED baseline)
**Goal**: Build the one piece of test infrastructure this bug class has never had — a fixture using real `git worktree add` topology — and use it to observe the shipped bug failing, before touching production code.

#### Story 1.1.1: Linked-worktree test fixture helper
**As a** developer writing regression tests for worktree-topology bugs, **I want** a reusable helper that builds a real linked worktree, **so that** this and future tests can exercise the exact topology where `--git-dir` vs `--git-common-dir` actually differs (a plain `git init` repo cannot distinguish them).
**Acceptance Criteria**:
- A `setupLinkedWorktree` helper exists in `session/backlog_commands_test.go` returning a ready-to-use worktree path with cleanup registered.
  - *Given* a fresh `t.TempDir()`-backed main repo created by `setupTestGitRepo(t)`, *When* `setupLinkedWorktree(t, repoPath, "excludes-regression")` is called, *Then* it returns a `worktreePath` for which `git rev-parse --git-dir` (run with `Dir: worktreePath`) resolves to `<repoPath>/.git/worktrees/excludes-regression` and `git rev-parse --path-format=absolute --git-common-dir` resolves to `<repoPath>/.git` — i.e. the two are provably different directories, the precondition for this bug class to be observable at all.
**Files**: `session/backlog_commands_test.go`

##### Task 1.1.1a: Add `setupLinkedWorktree` helper (~4 min)
- In `session/backlog_commands_test.go`, add:
  ```go
  // setupLinkedWorktree builds a real linked worktree (git worktree add topology)
  // on top of an existing main repo — the topology where --git-dir and
  // --git-common-dir resolve to different directories, which a plain setupTestGitRepo
  // fixture cannot exercise (see pitfalls.md's "topology-blind git-dir fixture" note).
  func setupLinkedWorktree(t *testing.T, repoPath, sessionName string) string {
      t.Helper()
      worktree, _, err := git.NewGitWorktree(repoPath, sessionName)
      if err != nil {
          t.Fatalf("git.NewGitWorktree failed: %v", err)
      }
      if err := worktree.Setup(); err != nil {
          t.Fatalf("worktree.Setup failed: %v", err)
      }
      t.Cleanup(func() {
          if err := worktree.Cleanup(); err != nil {
              t.Logf("worktree.Cleanup failed (non-fatal): %v", err)
          }
      })
      return worktree.GetWorktreePath()
  }
  ```
- `git.NewGitWorktree` and `*GitWorktree.Setup`/`.Cleanup`/`.GetWorktreePath` already exist in `session/git/worktree_ops.go` and `session/git/worktree.go`; `git` is already imported in this test file.
- Files: `session/backlog_commands_test.go`

### Epic 1.2 depends on Epic 1.1's helper for its regression test; both must complete before Epic 1.3's Task 1.3.2d.

#### Story 1.1.2: Regression test proving the bug (pre-fix RED)
**As a** maintainer, **I want** a test that fails against the current (buggy) `addWorktreeExcludes`, **so that** the fix in Epic 1.2 is proven to actually close the gap, not just assumed to.
**Acceptance Criteria**:
- AC1 (requirements.md): a regression test using a real `git worktree add` fixture demonstrates the observable effect (`git status`/`IsWorktreeDirty` clean) after writing `.backlog-context.md` and running `addWorktreeExcludes`.
  - *Given* a `setupLinkedWorktree`-built worktree with no scaffolding written yet, *When* `.backlog-context.md` is written into the worktree and `addWorktreeExcludes(worktreePath)` is called against the *current, unfixed* code, *Then* `session.IsWorktreeDirty(ctx, worktreePath)` returns `(true, nil)` — the test FAILS (RED), because the pattern landed in `WorktreePrivateAdminDir`'s `info/exclude`, which `git status` never reads.
**Files**: `session/backlog_commands_test.go`

##### Task 1.1.2a: Write `TestAddWorktreeExcludes_LinkedWorktree_ExcludesScaffoldingFiles` (~5 min)
- In `session/backlog_commands_test.go`:
  ```go
  func TestAddWorktreeExcludes_LinkedWorktree_ExcludesScaffoldingFiles(t *testing.T) {
      repoPath := setupTestGitRepo(t)
      worktreePath := setupLinkedWorktree(t, repoPath, "excludes-regression")

      if err := os.WriteFile(filepath.Join(worktreePath, ".backlog-context.md"), []byte("scaffolding"), 0o644); err != nil {
          t.Fatalf("failed to write .backlog-context.md: %v", err)
      }

      addWorktreeExcludes(worktreePath)

      dirty, err := IsWorktreeDirty(context.Background(), worktreePath)
      if err != nil {
          t.Fatalf("IsWorktreeDirty returned error: %v", err)
      }
      if dirty {
          t.Errorf("expected worktree to be clean after addWorktreeExcludes, but IsWorktreeDirty returned true — " +
              "the exclude pattern likely landed in the worktree-private admin dir instead of the shared git-common-dir")
      }
  }
  ```
- Files: `session/backlog_commands_test.go`

##### Task 1.1.2b: Run the test against pre-fix code and record RED (~2 min)
- Run: `go test ./session/... -run TestAddWorktreeExcludes_LinkedWorktree_ExcludesScaffoldingFiles -v`
- Confirm it **fails** (the `t.Errorf` in Task 1.1.2a fires) — this is the RED half of AC1's "fails before the fix, passes after" requirement. Do not proceed to Epic 1.2 until this failure is observed and noted in the PR description or commit message.
- Files: none (verification step only)

---

### Epic 1.2: Fix `addWorktreeExcludes` to resolve via `--git-common-dir`
**Goal**: Close the actual bug — the sole code change acceptance criterion AC0 requires.

#### Story 1.2.1: Swap `--git-dir` → `--path-format=absolute --git-common-dir`
**As a** backlog automation user, **I want** scaffolding files to actually be excluded in real (linked-worktree) sessions, **so that** `request_review` never rejects a session for scaffolding-only "dirty" state.
**Acceptance Criteria**:
- AC0 (requirements.md): `addWorktreeExcludes` writes patterns to the git-common `info/exclude`, resolved via `--git-common-dir`, not `--git-dir`.
  - *Given* the same `setupLinkedWorktree` fixture as Story 1.1.2, *When* `addWorktreeExcludes(worktreePath)` runs against the *fixed* code, *Then* the file `<repoPath>/.git/info/exclude` (the git-common-dir location, confirmed via `git rev-parse --path-format=absolute --git-common-dir` from `worktreePath`) contains the line `.backlog-context.md`, and `<repoPath>/.git/worktrees/excludes-regression/info/exclude` (the old, wrong, worktree-private location) is untouched (does not exist or does not contain the pattern).
- AC1 (requirements.md), GREEN half: the identical Task 1.1.2a test now passes.
  - *Given* the same fixture and steps as Story 1.1.2's Given/When, *When* run against the fixed code, *Then* `session.IsWorktreeDirty(ctx, worktreePath)` returns `(false, nil)`.
- AC4 (requirements.md): plain (non-worktree) repos see no behavior change.
  - *Given* a plain repo built by `setupTestGitRepo(t)` (not a linked worktree), *When* `git rev-parse --git-dir` and `git rev-parse --path-format=absolute --git-common-dir` are both run with `Dir` set to the repo root, *Then* they resolve to the same absolute directory (`<repoPath>/.git`), proving the fix is a no-op there — and all pre-existing `TestWriteBacklogContextFile_*`/`TestWriteSlashCommands_*`/`TestUntrackTrackedScaffolding_*` tests in `session/backlog_commands_test.go` continue to pass unmodified.
**Files**: `session/backlog_commands.go`, `session/backlog_commands_test.go`

##### Task 1.2.1a: Swap the subshell flag and delete the dead fallback (~3 min)
- In `session/backlog_commands.go`, `addWorktreeExcludes` (currently lines 241-277):
  - Change `cmd := safeexec.CommandContext(ctx, "git", "rev-parse", "--git-dir")` to `cmd := safeexec.CommandContext(ctx, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")`, mirroring `session/git/util.go:153` exactly.
  - Rename the local variable `gitDir` to `gitCommonDir` for clarity (it's no longer the worktree-private dir).
  - Delete the now-dead fallback block:
    ```go
    if !filepath.IsAbs(gitDir) {
        gitDir = filepath.Join(worktreePath, gitDir)
    }
    ```
    `--path-format=absolute` guarantees an absolute path, exactly as it does for `findMainRepoPathForWorktree` (which has no equivalent fallback).
  - Update the log line and the `excludeFile := filepath.Join(gitDir, "info", "exclude")` reference to use `gitCommonDir`.
  - Update the function's doc comment (currently says `$GIT_DIR/info/exclude`) to say `$GIT_COMMON_DIR/info/exclude`.
- Files: `session/backlog_commands.go`

##### Task 1.2.1b: Rerun regression + existing scaffolding tests, confirm GREEN (~2 min)
- Run: `go test ./session/... -run TestAddWorktreeExcludes_LinkedWorktree_ExcludesScaffoldingFiles -v` — confirm it now **passes** (GREEN, closing AC1).
- Run: `go test ./session/... -run 'TestWriteBacklogContextFile|TestWriteSlashCommands|TestUntrackTrackedScaffolding'` — confirm all pre-existing tests still pass unmodified (AC4, first half).
- Files: none (verification step only)

##### Task 1.2.1c: Dedicated plain-repo equivalence test, closing AC4's remaining gap (~3 min)
- In `session/backlog_commands_test.go`, add:
  ```go
  // TestAddWorktreeExcludes_PlainRepo_GitCommonDirEqualsGitDir pins the mechanism by
  // which the --git-common-dir fix is a no-op for plain (non-worktree) repos: both
  // flags must resolve to the same directory there. Existing TestWriteBacklogContextFile_*
  // / TestWriteSlashCommands_* tests only assert the fix didn't break plain-repo behavior
  // indirectly (by continuing to pass) — this test asserts the specific equivalence claim
  // AC4 relies on.
  func TestAddWorktreeExcludes_PlainRepo_GitCommonDirEqualsGitDir(t *testing.T) {
      repoPath := setupTestGitRepo(t)

      gitDir := runGitRevParse(t, repoPath, "--git-dir")
      commonDir := runGitRevParse(t, repoPath, "--path-format=absolute", "--git-common-dir")

      if !filepath.IsAbs(gitDir) {
          gitDir = filepath.Join(repoPath, gitDir)
      }
      if gitDir != commonDir {
          t.Errorf("expected --git-dir (%s) and --git-common-dir (%s) to resolve identically for a plain repo", gitDir, commonDir)
      }
  }

  func runGitRevParse(t *testing.T, dir string, args ...string) string {
      t.Helper()
      cmd := safeexec.CommandContext(context.Background(), "git", append([]string{"rev-parse"}, args...)...)
      cmd.Dir = dir
      out, err := cmd.Output()
      if err != nil {
          t.Fatalf("git rev-parse %v failed: %v", args, err)
      }
      return strings.TrimSpace(string(out))
  }
  ```
- Files: `session/backlog_commands_test.go`

---

### Epic 1.3: Enumerate dirty paths and surface them in `request_review`'s rejection

#### Story 1.3.1: `GetWorktreeDirtyPaths` function
**As a** backlog agent whose `request_review` call is rejected, **I want** the system to know exactly which files are dirty, **so that** the rejection message (Story 1.3.2) can name them instead of guessing.
**Acceptance Criteria**:
- AC2 (requirements.md): a function exists returning the specific dirty paths, correctly handling untracked, modified, and renamed files, reusing existing rename-parsing rather than reimplementing it.
  - *Given* a plain repo (`setupTestGitRepo`) with one untracked file `foo.txt`, one modified tracked file `bar.txt`, and one renamed tracked file (`old.txt` → `new.txt`, via `git mv`), *When* `GetWorktreeDirtyPaths(repoPath)` is called, *Then* it returns a `[]string` containing exactly `"foo.txt"`, `"bar.txt"`, and `"new.txt"` (not `"old.txt"`), with no error, and no duplicate entries.
  - *Given* a freshly committed plain repo with no working-tree changes, *When* `GetWorktreeDirtyPaths(repoPath)` is called, *Then* it returns `(nil, nil)` (or an empty slice) — no error.
  - *Given* a plain directory that is not a git repository at all (e.g. `t.TempDir()` with no `git init`), *When* `GetWorktreeDirtyPaths(dir)` is called, *Then* it returns `(nil, nil)` — matching `IsWorktreeDirty`'s existing no-op-on-non-git contract (AC4's "directory-mode sessions see no behavior change" extends to this new function too) — rather than surfacing `vc.ErrNoVCSFound` as an error.
**Files**: `session/backlog_review.go`, `session/backlog_review_test.go`

##### Task 1.3.1a: Implement `GetWorktreeDirtyPaths` (~5 min)
- In `session/backlog_review.go`, add imports `"errors"` and `"github.com/tstapler/stapler-squad/session/vc"` to the existing import block.
- Add, next to `IsWorktreeDirty` (line 626):
  ```go
  // GetWorktreeDirtyPaths returns the specific paths with uncommitted changes
  // (untracked, modified, or renamed — new path only) in the git worktree at
  // worktreePath, deduplicated. Returns (nil, nil) — the same no-op contract as
  // IsWorktreeDirty — when worktreePath is not a git repository at all, rather
  // than surfacing vc.ErrNoVCSFound as an error. Additive sibling to
  // IsWorktreeDirty: does not replace its boolean-only callers.
  //
  // Reuses vc.GitProvider.GetChangedFiles/parsePorcelainV2Z (NUL-safe,
  // rename-aware porcelain-v2 parsing) rather than reimplementing status
  // parsing — see session/vc/git_provider.go.
  func GetWorktreeDirtyPaths(worktreePath string) ([]string, error) {
      provider, err := vc.NewGitProvider(worktreePath)
      if err != nil {
          if errors.Is(err, vc.ErrNoVCSFound) {
              return nil, nil
          }
          return nil, fmt.Errorf("GetWorktreeDirtyPaths: %w", err)
      }
      changes, err := provider.GetChangedFiles()
      if err != nil {
          return nil, fmt.Errorf("GetWorktreeDirtyPaths: %w", err)
      }
      seen := make(map[string]struct{}, len(changes))
      paths := make([]string, 0, len(changes))
      for _, c := range changes {
          if _, ok := seen[c.Path]; ok {
              continue
          }
          seen[c.Path] = struct{}{}
          paths = append(paths, c.Path)
      }
      return paths, nil
  }
  ```
- Files: `session/backlog_review.go`

##### Task 1.3.1b: Table-driven unit tests (dirty / clean / renamed / non-git) (~5 min)
- In `session/backlog_review_test.go`, add a table-driven `TestGetWorktreeDirtyPaths` covering the three `setupTestGitRepo`-based scenarios and one non-git-directory scenario from Story 1.3.1's Given/When/Then above (untracked+modified+renamed → 3 paths; clean → empty; non-git dir → `(nil, nil)`).
- Reuse `setupTestGitRepo` (already in package `session`, defined in `session/backlog_commands_test.go`) for the git-backed cases; use a bare `t.TempDir()` for the non-git case.
- Files: `session/backlog_review_test.go`

#### Story 1.3.2: `request_review` rejection message names real paths
**As a** backlog agent, **I want** `request_review`'s uncommitted-changes rejection to tell me exactly which files are dirty, **so that** I can commit precisely those files instead of blindly running `git add -A`.
**Acceptance Criteria**:
- AC3 (requirements.md): when genuinely dirty, the rejection message lists specific dirty path(s) instead of a blanket `git add -A && git commit` instruction.
  - *Given* `formatDirtyPathsRejectionMessage([]string{"web-app/src/foo.tsx"})`, *When* called, *Then* the returned string contains the literal substring `"web-app/src/foo.tsx"` and does not contain the substring `"git add -A"`.
  - *Given* `formatDirtyPathsRejectionMessage` called with 15 paths, *When* called, *Then* the returned string lists the first 10 (or however many `maxRejectionMessagePaths` is set to) and ends with a `"...and 5 more"`-style suffix rather than all 15, per pitfalls.md's "cap the count, not raw byte length" guidance.
- Story 1.3.2 AC (closes plan gap G1, per validation.md): a worktree containing *only* scaffolding files is never listed as dirty by `GetWorktreeDirtyPaths`, end-to-end, once Epic 1.2's fix is applied.
  - *Given* a `setupLinkedWorktree` fixture with `addWorktreeExcludes` already run against it, and `.backlog-context.md` plus `.claude/commands/backlog/status.md` written into the worktree afterward, *When* `GetWorktreeDirtyPaths(worktreePath)` is called, *Then* it returns an empty slice (or `nil`) — proving Epic 1.2 and Epic 1.3 compose correctly rather than only being separately unit-tested.
**Files**: `server/mcp/tools_backlog.go`, `server/mcp/tools_backlog_test.go`, `session/backlog_commands_test.go`

##### Task 1.3.2a: Implement `formatDirtyPathsRejectionMessage` (~4 min)
- In `server/mcp/tools_backlog.go`, near the `request_review` handler, add:
  ```go
  // maxRejectionMessagePaths caps how many dirty paths formatDirtyPathsRejectionMessage
  // lists explicitly — per pitfalls.md, cap the count, not raw byte length.
  const maxRejectionMessagePaths = 10

  // formatDirtyPathsRejectionMessage builds request_review's uncommitted-changes
  // rejection text from the specific dirty paths, instead of a blanket
  // "git add -A" instruction, so the agent knows exactly what to commit.
  func formatDirtyPathsRejectionMessage(paths []string) string {
      if len(paths) == 0 {
          return "request_review rejected: the worktree has uncommitted changes. " +
              "Run `git status` in the worktree, commit your work, then call request_review again."
      }
      shown := paths
      var suffix string
      if len(paths) > maxRejectionMessagePaths {
          shown = paths[:maxRejectionMessagePaths]
          suffix = fmt.Sprintf(" ...and %d more", len(paths)-maxRejectionMessagePaths)
      }
      return fmt.Sprintf(
          "request_review rejected: the worktree has uncommitted changes: %s%s. "+
              "Commit these specific paths (e.g. `git add <path> && git commit -m 'description of changes'`), then call request_review again.",
          strings.Join(shown, ", "), suffix,
      )
  }
  ```
- `fmt` and `strings` are already imported in `server/mcp/tools_backlog.go`.
- Files: `server/mcp/tools_backlog.go`

##### Task 1.3.2b: Wire the `request_review` call site (~3 min)
- In `server/mcp/tools_backlog.go`, replace the rejection block (currently lines 384-392):
  ```go
  if wt, wtErr := h.storage.GetWorktreeDataBySessionUUID(ctx, callerUUID); wtErr == nil && wt.WorktreePath != "" {
      if dirty, dirtyErr := session.IsWorktreeDirty(ctx, wt.WorktreePath); dirtyErr == nil && dirty {
          paths, pathsErr := session.GetWorktreeDirtyPaths(wt.WorktreePath)
          if pathsErr != nil {
              log.WarningLog.Printf("[mcp:request_review] GetWorktreeDirtyPaths failed for session=%s worktree=%s: %v", callerUUID, wt.WorktreePath, pathsErr)
          }
          log.InfoLog.Printf("[mcp:request_review] rejected: uncommitted changes in worktree for session=%s item=%s paths=%v", callerUUID, itemID, paths)
          return errResult(ErrInvalidArgument, formatDirtyPathsRejectionMessage(paths), ""), nil
      }
  }
  ```
  This preserves the exact existing gating logic (`wtErr == nil && wt.WorktreePath != ""`, then `dirtyErr == nil && dirty`) — only the message construction changes. If `GetWorktreeDirtyPaths` errors, `paths` is `nil` and `formatDirtyPathsRejectionMessage` falls back to its generic (path-less) wording — the rejection still happens, it just can't name specific paths.
- Files: `server/mcp/tools_backlog.go`

##### Task 1.3.2c: Unit tests for the formatter (~4 min)
- In `server/mcp/tools_backlog_test.go`, add:
  - `TestFormatDirtyPathsRejectionMessage_ListsEachPath` — asserts the message contains each given path and does not contain `"git add -A"`.
  - `TestFormatDirtyPathsRejectionMessage_CapsAtMaxPaths` — asserts a 15-path input yields exactly `maxRejectionMessagePaths` listed paths plus an "...and 5 more" suffix.
  - `TestFormatDirtyPathsRejectionMessage_EmptyPaths_UsesGenericWording` — asserts the `len(paths) == 0` branch's fallback text.
- Files: `server/mcp/tools_backlog_test.go`

##### Task 1.3.2d: Integration test — scaffolding-only linked worktree stays clean end-to-end (~4 min)
- In `session/backlog_commands_test.go` (co-located with `setupLinkedWorktree` from Task 1.1.1a, since it needs that helper and this file already imports `git`):
  ```go
  func TestGetWorktreeDirtyPaths_EmptyForScaffoldingOnlyLinkedWorktree(t *testing.T) {
      repoPath := setupTestGitRepo(t)
      worktreePath := setupLinkedWorktree(t, repoPath, "scaffolding-only")

      addWorktreeExcludes(worktreePath)

      if err := os.WriteFile(filepath.Join(worktreePath, ".backlog-context.md"), []byte("scaffolding"), 0o644); err != nil {
          t.Fatalf("failed to write .backlog-context.md: %v", err)
      }
      cmdDir := filepath.Join(worktreePath, ".claude", "commands", "backlog")
      if err := os.MkdirAll(cmdDir, 0o755); err != nil {
          t.Fatalf("failed to create .claude/commands/backlog: %v", err)
      }
      if err := os.WriteFile(filepath.Join(cmdDir, "status.md"), []byte("status"), 0o644); err != nil {
          t.Fatalf("failed to write status.md: %v", err)
      }

      paths, err := GetWorktreeDirtyPaths(worktreePath)
      if err != nil {
          t.Fatalf("GetWorktreeDirtyPaths returned error: %v", err)
      }
      if len(paths) != 0 {
          t.Errorf("expected no dirty paths for a scaffolding-only worktree, got %v", paths)
      }
  }
  ```
  This is the test that proves Epic 1.2's fix and Epic 1.3's `GetWorktreeDirtyPaths` compose correctly — it must run after both are implemented.
- Files: `session/backlog_commands_test.go`

---

## Final Verification (AC5)
- Run `go build ./...` — must exit 0.
- Run `make test`, or at minimum `go test ./session/... ./server/mcp/...` — must exit 0, including every test added in this plan plus all pre-existing tests in `session/backlog_commands_test.go`, `session/backlog_review_test.go`, and `server/mcp/tools_backlog_test.go`.
