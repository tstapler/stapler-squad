# Research: Architecture — go-git-fork-full-git-replacement

Evidence labels: VERIFIED (opened file / ran command this session) vs INFERRED.

## 0. Prior art that changes the premise (VERIFIED)

- `project_plans/go-git-worktree-and-merge/` already covers a subset of this problem. Its `decisions/ADR-002-two-feature-flags-and-merge-override-key.md` is **Superseded: both flags removed 2026-09-22, native is now the only path**. The repo already has an in-repo native (go-git-primitive) implementation of worktree add/remove/prune/list and merge: `session/git/native_worktree_{add,remove,prune,list,common}.go`, `native_admin_writer.go`, `native_merge*.go`, `native_ref_lock_test.go`. `session/git/ops.go:941-950` dispatches `MergeMainIntoWorktree` to `nativeMergeMainIntoWorktreeLocked`.
- `project_plans/go-git-worktree-and-merge/research/build-vs-buy.md` concluded go-git v5.19.2 has no multi-worktree API and v6 alpha is not ready, and chose to build on go-git primitives *in this repo* rather than fork.
- Consequence: requirements.md's claim that go-git "cannot create or remove linked worktrees" is true of the library but **not of this codebase**. "Worktree add/remove last" in the Risk Control section is partly already done (and without a CLI fallback). The remaining CLI surface is smaller than the baseline implies; a per-site audit (open question 2) is the first task.
- No ADR in `docs/adr/` addresses forking go-git (grep for go-git in `docs/adr`: only `004-fugitive-inspired-git-integration.md` matched the listing for "git", unrelated to forking). Skill `.claude/skills/prefer-go-git-over-subshells/SKILL.md` sets the policy "prefer go-git over `safeexec.CommandContext("git", ...)`".

## 1. How git access is structured today

### Remaining product CLI sites (VERIFIED by grep of `safeexec.Command*("git"` / `exec.Command*("git"`, excluding tests/third_party; 40 hits, 21 files)

| File | Sites | Nature |
|---|---|---|
| `session/repo_path.go` | 7 | clone (L314, L418), `fetch --all --prune` (L375), others — the only network ops that matter |
| `server/services/unfinished_work_service.go` | 6 | status/log style reads |
| `session/git/util.go` | 3 | `rev-parse --git-common-dir`, `for-each-ref`, `rev-parse HEAD` |
| `session/backlog_review.go` | 3 | |
| `server/services/session_service_lifecycle.go`, `pkg/classifier/classifier.go` | 2 each | |
| ~14 files | 1 each | `session/git/ops.go:36` (`fetch origin -- <branch>`), `session/git_worktree_manager.go:564` (`rev-parse HEAD`), `session/vc/git_provider.go`, `session/vcs/{git,detect}.go`, `session/unfinished/state.go`, `server/services/*`, `testutil/gitfixture/identity.go` |
| `session/vc/git_provider.go` | `runGit` helper | push (L587/L592), fetch (L617) — whole provider is CLI-text-based |
| `session/vcs/git.go` | `g.run` helper | `stash push` etc. |

Network ops are fewer than requirements.md implies: fetch/clone/push are concentrated in `session/repo_path.go`, `session/git/ops.go:36`, and `session/vc/git_provider.go`. (INFERRED: requirements' "push (5), fetch (5)" likely includes `jj git push/fetch` in `session/vc/jj_provider.go`, which are `jj`, not `git`.) GitHub API calls in `github/` use REST/GraphQL via `NewConditionalRequest`, not git.

### Seams (VERIFIED unless noted)

- `session/git/util.go:43` `OpenRepo(path)` — single in-process open seam, lint-enforced by `tools/lint/norawgitopen`. 29 non-test files import `go-git/go-git`.
- `safeexec` — single *exec* seam, lint-enforced by `tools/lint/norawexec`. All CLI git calls go through `safeexec.CommandContext`, so they are greppable/countable and wrappable.
- `session/git/native_rollout.go` — `withOperationSpan(ctx, op, fn func() (implementation, outcome string, err error))` records an OTel span with `implementation`/`outcome` attrs and an `operationDurationMS` histogram labelled `operation`+`implementation`. This is an existing per-operation backend-labelling mechanism — the observability half of the requested fallback counters already exists for worktree/merge ops.
- Two parallel VCS abstractions with `git` + `jj` backends: `session/vcs/vcs.go` (`VCS` interface, `Type()`, status ops) and `session/vc/` (`provider.go`, `git_provider.go`, `jj_provider.go`). Both CLI-backed for git. Duplicated layers (INFERRED: historical; not audited for callers).
- `session/git_worktree_manager.go` and `session/git/worktree*.go` — higher-level worktree lifecycle; `worktree_ops.go` earlier refactor doc (`docs/archive/tasks/completed/worktree-refactoring.md`) mentioned a `repository_impl.go` interface but that file does not exist now (INFERRED: abandoned).

### Is there a single seam for per-operation backend swap?

**No.** There is a single *exec* seam (`safeexec`) and a single *open* seam (`OpenRepo`), but no operation-level interface (e.g. `GitBackend{RevParse, Fetch, Clone, Status...}`). CLI calls are call-site-scattered across 21 files, 3 packages (`session`, `server/services`, `pkg`), and two VCS abstraction layers; each site builds args and parses text itself. Native vs CLI switching exists only inside `session/git` (merge/worktree) and is now unconditional.

### Disposition: **Isolate via seam**

Introduce a narrow operation-level interface (one method per distinct CLI operation, typed results, no text parsing at callers) with `cli` and `gogit` implementations, plus a per-op selector; migrate callers to it incrementally. Not Refactor-first (the scattered sites are small, mostly 1 per file; a big-bang refactor delivers nothing) and not Extend-as-is (that would add more call-site-local `if flag {gogit} else {exec}` branches with no shared counters). The `vcs`/`vc` duplication should be left alone unless the audit shows one is dead.

## 2. Consuming a go-git fork

go.mod facts (VERIFIED): `github.com/go-git/go-git/v5 v5.19.2` direct; `go-billy/v5 v5.9.0`, `gcfg` indirect; no `vendor/` dir; no `replace` directive today; `go mod tidy`/`verify` in `Makefile:938`; goreleaser at `.goreleaser.yaml`.

| Option | Pros | Cons |
|---|---|---|
| `replace github.com/go-git/go-git/v5 => github.com/tstapler/go-git/v5 vX` | Zero import-path churn (29 files untouched); transitive deps that import upstream also get the fork, so types stay identical; trivial to drop (delete the line) = clean rollback | `replace` only applies in the **main module**: any consumer importing stapler-squad as a library would not inherit it (not a concern: this is an app); fork's `go.mod` module line must remain `.../go-git/v5` (go requires replacement's module path to match unless ... it does not for local/replace targets, but `go mod` verifies — INFERRED, test early); `tools/lint` is a separate module (`tools/lint/go.mod`) — check whether it needs its own replace |
| Module path rename (`github.com/tstapler/go-git/v5`) | Explicit, `go get`-able, works for dependents | Rewrite 29+ imports; **type identity breaks** with any library that takes/returns upstream `*git.Repository`/`plumbing.*` (two distinct types); rebases conflict on every file's import line |
| Vendoring (`go mod vendor`) | Offline/hermetic build | Repo has no vendor dir; `.github/workflows` and Makefile don't expect it; vendor diff noise; doesn't solve identity either |

Recommendation (INFERRED, validate with a spike): `replace` to a private/pinned fork tag. Libraries that import upstream go-git (e.g. anything under `go.sum`'s 8 go-git lines; check `go mod graph | grep go-git`) are transparently redirected, so the same-type guarantee holds — this is the main reason to prefer `replace` over a rename.

CI/release implications:
- Private fork repo needs `GOPRIVATE=github.com/tstapler/go-git` and `GONOSUMDB`/`GOFLAGS=-mod=mod` credentials in `.github/workflows/{build,lint,release,goreleaser-check,mcp-integration}.yml` and in goreleaser; if the fork is public, none of that is needed. Public-vs-private is the cheapest decision to make early.
- `go.sum` pins the fork's tag; use tags (`v5.19.2-ssq.1`) not branch pseudo-versions so `make tidy`/`verify` is reproducible.
- Dependabot/renovate will not auto-bump upstream through a `replace`; rebase cadence is manual (single-maintainer risk, as the requirements note).
- A drop-in alternative that avoids a fork for most of the scope: the repo already implements worktree/merge on top of unmodified go-git (`native_*.go`). The fork is only justified for what *cannot* be done on top of public go-git API (credential-helper/ssh fallback transport, if that can't be a custom `transport.Transport` plugged via `client.InstallProtocol`, which is public API — INFERRED, verify before choosing a fork).

### Per-operation CLI-fallback switch + counters

- Config: the retired `go-git-worktree-and-merge` flags used `config.FeatureFlags`, `EffectiveXEnabled`, a per-session override map, RPC + settings panel (requirements of that project, VERIFIED text in its requirements.md L82; the flags were then removed). Reuse the same `config.FeatureFlags` pattern, but as a **map** `git_backend: {"rev-parse": "gogit", "fetch": "cli", ...}` rather than one boolean per op, with default `cli`, to satisfy "default CLI until parity."
- Metrics/logs: reuse `withOperationSpan` + `operationDurationMS` (already labelled `operation`, `implementation`). Add one counter, `git_backend_fallback_total{operation,callsite,reason}` registered via `mustInt64CounterGit`, incremented whenever the CLI path runs (either by config or after an in-process error). A CI "zero spawn" gate is better done at the exec seam: count in `safeexec` when `name=="git"` (that is the one place all CLI calls pass, lint-enforced) — this measures the success metric directly and cannot be bypassed by a forgotten counter at a call site.
- Logging: use slog fields `op`, `backend`, `fallback_reason`; never log args that contain URLs (see 3.5).

## 3. Migration-specific failure modes

1. **Partial migration states.** With a per-op map, some ops run in-process and others via CLI on the same repo in one session lifecycle (e.g. native worktree add then CLI `rev-parse` inside it). Mixed states multiply the test matrix; mitigate by grouping ops into *cohorts* that flip together (read-only ref ops; diff/status; network; worktree writes) rather than 50 independent flags. Existing precedent: the earlier project ended with native-only and flags deleted, i.e. fallback has a lifetime; set one up front ("flag removed after N releases") so dead CLI branches don't linger.
2. **CLI vs in-process divergence on the same repo.** Known sources: go-git `Status()` is slow and differs on gitignore/untracked handling (BUG-104 in `docs/bugs/fixed/BUG-104-worktree-status-merkletrie-tree-diff-cpu.md` already hit this; solved by bespoke `worktree_dirty_fast.go`); `rev-parse` of abbreviated/ambiguous refs; rename detection and whitespace in `diff` that callers parse as text. Mitigation: differential tests (the repo already has `native_merge_differential_test.go` and golden/fuzz tests — extend the pattern), and a shadow-mode that runs both and logs mismatch (read-only ops only; never shadow a write).
3. **Index-lock / ref-lock races between CLI and in-process writers.** Git CLI takes `index.lock`, `HEAD.lock`, `refs/...lock` with O_EXCL; go-git v5 does not use the same lock files for the index, so a go-git index write concurrent with a CLI `git add`/`checkout` can silently lose a write, and the reverse can leave a stale `.lock`. The repo already has `session/git/worktree_lock.go` and `native_ref_lock_test.go` for the native path (VERIFIED file names; behavior not read). In a mixed state, the in-process lock must be the same filesystem lock file the CLI honours, or all writers to one worktree must go through one backend (cohort rule above; worktree writes flip last). External actors (the agent's own `git` run inside the session via Claude Code) will always use the CLI on the same worktree, so an in-process writer must always use CLI-compatible lock files — this is a permanent constraint, not just a migration one.
4. **Worktree metadata corruption.** `.git/worktrees/<name>/{HEAD,gitdir,commondir,locked}` and the worktree's `.git` file; a crash between writing admin files and the branch ref leaves orphans. Existing mitigation: `native_admin_writer.go` + ADR-001 atomic admin-file write protocol (`project_plans/go-git-worktree-and-merge/decisions/ADR-001-atomic-admin-file-write-protocol.md`), prune and reconciliation (`project_plans/session-worktree-reconciliation`). A *fork* that modifies go-git's storage layer for per-worktree HEAD/index would duplicate and conflict with this already-validated code; prefer keeping worktree code in-repo.
5. **Credential leakage in logs.** Today CLI errors embed stderr/argv (clone URLs with tokens are plausible: `repo_path.go:314/418` pass `originURL`/`cloneURL` as args). In-process go-git errors and `transport` debug output can also include URLs; the fallback counter's `callsite`/`reason` labels and span `RecordError(err)` (`native_rollout.go` records `err.Error()` verbatim into the span) will export those to OTel. Mitigation: a single redaction function (strip userinfo from URLs, `Authorization`, tokens) applied in the backend interface before any log/span/metric; never put URLs in metric labels. Also keep tokens out of `argv` (visible in `ps`) — migrating to in-process removes that exposure, a security *benefit* worth recording.
6. **No-`git`-binary runtime.** `gh` and the agent CLIs (Claude Code itself invokes `git`) still need git inside session worktrees; "runs on a machine with no git binary" conflicts with the product's core use (agents run git in the worktree). INFERRED; flag for the requirements owner.

## Summary decisions to carry into planning

- Disposition: Isolate via seam.
- Fork consumption: `replace` directive to a tagged fork, only if a spike shows custom transport/storage cannot be done via public go-git API.
- Per-op switch: cohort map in `config.FeatureFlags`, counters at the `safeexec` seam plus existing `withOperationSpan`.
