# Implementation Plan: go-git-fork-full-git-replacement

**Feature**: Replace the `git` CLI in the stapler-squad server process with in-process go-git, backed by a long-lived private fork (`github.com/tstapler/go-git`, v5 line) that carries only the patches public go-git API cannot support, behind a per-cohort CLI fallback.
**Date**: 2026-10-08
**Status**: Revision 6 (Phase 4 patch pass from `pre-mortem.md`, `consistency.md`, `validation.md`; see "Revision 6 change log"; **planning freeze: no Revision 7 until shipped code exists, further review findings go to a backlog list in `gates.md`**); before that Revision 5 (Re-review 3 concerns R3-1 to R3-6 applied, see "Revision 5 change log"; before that Revision 4, Phase 3 repair pass 3: adversarial blockers N3/N4 and concerns C-a to C-d, architecture concerns C3 to C6 resolved, see "Revision 4 change log" below; Revision 3 and 2 logs retained under it). Spikes first. Every epic after Phase 0 is conditional on a named gate. ADR-003 and ADR-006 are still NEEDS USER DECISION (O-1, O-2); Epic 1.1's router bakes in their recommended defaults, and Epics 1.1/1.3 may start before G7 on that explicit assumption.

> **Plain statement of the likely end state (adversarial C1)**: the in-repo fixes now designed for locking (operation-scoped lock, Story 0.2.4/2.3.1) and repack (public `ObjectStorage.Reindex()`, `storage/filesystem/object.go:74`) are wrapper-level, and the former F3 (index v3) premise was wrong (go-git v5.19.2 decodes and encodes index v2 to v4: `plumbing/format/index/decoder.go:19`, `encoder.go:17`). Gates G4 and G6 will therefore most likely return "wrapper works", and the most probable end state is **an empty pinned fork F0** (a `tstapler/go-git` fork at the upstream tag, consumed by `replace`, carrying zero patches). The user chose a fork at 3 to 6 weeks; that choice is honoured, but **O-6 (keep an empty pinned fork, or drop the fork and use stock go-git v5) is an explicit decision point for Tyler at checkpoint G7a, before Epic 1.2 and before any Phase 2+ work**. What evidence would justify a fork patch: a failing test against public API that no wrapper can fix (the fork-only test, 0.3) or a measured G3 perf gap in a hot go-git path fixable in under 200 lines. A no-patch result is a successful outcome of the spikes, not a failed plan.

## Revision 6 change log (Phase 4 patch pass)
| Source item | Resolution in this plan |
|---|---|
| Pre-mortem P1#1 scope blowout | Story 1.3.4 ships **resolver v1** (conservative superset, route-to-CLI) first; the full git-compatible resolver becomes **v2, stretch**. Epic 2.3 `localwrite` moves **out of the 3 to 6 week appetite** into an explicit stretch phase behind go/no-go gate GL. Week-3 scope-freeze tripwire T3 with a stated drop order (section 0.3.1). Planning freeze (Status line) |
| Pre-mortem P1#2 in-process write loses agent work | Router precondition `live_session`; `localwrite` opt-in per repo, default off, never default-flipped in the first release; lock race, partial-read, `pack-refs`/`branch -D` stress and CLI-commit-in-gap tests are **required CI**; lock journal now records `released` **after** the rename (Story 2.3.1) |
| Pre-mortem P1#3 false-clean feeds destructive paths | Typed `Intent` on `IsDirty`/`Status`/`DiffNumstat`; destructive intent routes to the CLI when go-git says clean (`destructive_confirm`); Task 0.1.1c lists the destructive call sites; oracle corpus extended; the `diffstatus` flip is blocked on zero `false_clean` over the shadow window and over every repo with a session in the prior 30 days |
| Pre-mortem P2#4 spawn goal arrives too late | Fixtures (Epic 5.1) start week 1; weekly spawn/wall-time report; gate GS (spawn-reduction checkpoint) |
| Pre-mortem P2#5 gate graduation | Report-only for 10 consecutive green main runs, then blocking (Story 5.3.1) |
| Pre-mortem P2#6 shadow noise | Stable-disagreement definition, per-mismatch repro record, window measured in operations as well as days (Stories 1.1.3, 2.1.3) |
| Consistency B1 | Container run and fixture clone use `git://` (host-side `git daemon`) or an in-process remote; local-path clone and push are named tier-3 carve-outs (Stories 5.1.1, 5.3.2) |
| Consistency B2 | Story 2.3.1 matrix split: production CI matrix covers promoted operations only; `Remove`/`RemoveGlob`/`Move`/`Reset(Hard)`/`Checkout` rows are spike-only and fault-injection-promotion only |
| Consistency C4 | Write-cohort promotion evidence defined (Story 6.1.1): oracle parity, post-write CLI verification in the dogfood window, soak with CLI writers |
| Consistency C5 | Gate and story numbering sentence under Epic 0.2; glossary "G0..G8 plus G7a"; dependency diagram shows G7, G7a, T3, GS, GL |
| Consistency C6, N7 | Epic 4.1 header and diagram state G3 and G4; Epic 1.2 diagram entry states G7a |
| Consistency C7 | Fail-closed cases renamed `FC1`..`FC11` in the plan and ADR-006; `F0`..`F5` stay fork patches |
| Consistency C8 | Credential provider takes an injected `TokenSource`; `gitwiring` adapts `github.GetKeychainTokenForHost`; `gogit` never imports `github` (verified: `go list -deps ./github` lists `config`, `executor/safeexec`, `session/git`, `session/tmux`, `session/lifecycle`) |
| Consistency C9 | Credential-helper binary fallback is **always on** (not opt-in); ADR-005 reworded |
| Consistency C10 | New `FallbackReason` `unsupported_pull_mode`; evaluation precedence stated in Story 1.1.3 |
| Consistency C2 / validation G-1 | Mutex-delay non-regression check in Stories 5.1.2 and 6.1.1 |
| Validation G-2 | Nightly p50 re-check against the G3 baseline after any flip or fork rebase (Story 5.2.1, 6.3.1) |
| Validation G-5 | macOS plus Linux CI matrix task for xattr lock recovery (Task 2.3.1e) |
| Consistency N6 | Unverified first-person claim removed; the cited `git_provider.go:563,568` are **index-only** operations (`restore --staged`, `reset HEAD`), not worktree writers, so the claim was wrong and is corrected |

## Revision 5 change log (Re-review 3 concerns R3-1 to R3-6)
- R3-2: `Remove`, `RemoveGlob`, `Move` moved to the `unsafe_worktree_write` CLI list (go-git deletes/renames worktree files before `SetIndex`, VERIFIED `worktree_status.go:584-604,651-657,713-742`); the "atomic" claim is stated as refs and index only (Story 2.3.1, Task 2.3.2 note, allow-list table).
- R3-4: `capability_local_transport` preflight uses the plan's own resolver and checks both the fetch and the push URL plus any direct `RemoteURL` (Task 1.3.4b, Story 3.2.1); go-git's `insteadOf` reads only the repo's config (VERIFIED `config.go:373-376`, `:666`, `remote.go:84-86,113-114,416`).
- R3-1: commit order (index first, then refs) is stated as deliberately the reverse of git's, with a test of a CLI commit inside the gap; transient window named in ADR-003.
- R3-3: lock creation is temp-file-with-xattr-token plus `link()`; `released` and `abandoned` journal records are never reported at startup; temp-name leftovers are scanned.
- R3-5: "holds no lock while waiting" corrected (waits are bounded, held locks are dropped on failure); pack-refs/branch -D stress test added.
- R3-6: ADR-003 decision text states that keeping `Checkout`, `Reset(Hard|Merge)`, `Restore(Worktree)` (and now `Remove`/`Move`) on the CLI narrows the no-git end state.

## Revision 4 change log (repair pass 3)
| Re-review item | Resolution in this plan |
|---|---|
| Adv N3: resolver fails closed on `includeIf "hasconfig:remote.*.url:"`, which the maintainer's config reaches through an unconditional include (10 such entries, VERIFIED `grep -c '^\[includeIf "hasconfig' ~/.gitconfig.overlay`) | Story 1.3.4 (c) now implements `hasconfig:remote.*.url:<glob>` the way git does (reproduced against git 2.50.1): a lazy **URL-collection pass** over the whole scope sequence that treats every `hasconfig` condition as true, then the main pass that matches the collected URLs with wildmatch (`WM_PATHNAME`, `*` does not cross `/`). The fail-closed set is restated exactly (the cases where git itself dies or crashes, plus an unported glob feature), the criterion (iii) that pinned the defect is replaced, and the maintainer's shape is a fixture with a soak assertion on the first dogfood day |
| Adv N4: go-git's config decoder loses include position and conflates a bare key with `key =` (reproduced against v5.19.2) | Own tokenizer `session/git/native/gitconfig/parse.go` (stream-order events, git grammar); `plumbing/format/config` and `config.LoadConfig` are banned from the routing path by a depguard deny rule; capability detectors move to their own package so the rule is enforceable; oracle matrix over `git config --list --show-origin --show-scope --includes -z` including the repeated-section, bare-key and continuation rows |
| Adv C-a: `Checkout`/`Reset` write HEAD first, an aborted scope leaves HEAD, index and worktree inconsistent | The scoped storer now **buffers refs as well as the index** behind held lock files and commits both at scope end, so any error before the commit phase discards ref and index together (consistent, old state). `Wrote()` flips **before** the first commit-phase rename and is aggregated per backend call. Operations that write worktree files (`Checkout`, `Reset(Hard\|Merge)`, `Restore(Worktree)`) stay on the CLI (`unsafe_worktree_write`) until a fault-injection parity test passes |
| Adv C-b: stale-lock recovery can delete a live agent's lock | Recovery redesigned (Story 2.3.1 policy): a lock is auto-removed only if its owner's instance flock is free **and** the lock carries the random token we wrote into it (xattr) **and** it is older than 60 s **and** no running session uses that repository; the inode is no longer an identity; the record is dropped before unlink; an intent record precedes creation; all instance journal directories are scanned; the unrecovered cases are enumerated |
| Adv C-c: go-git's file transport spawns `git` for local-path and `file://` remotes | New capability `local_transport` (CLI route, ADR-003 tier 3) plus a structural tripwire: the `file` protocol is re-registered with a transport that returns a typed error before any exec; the PATH shim gets stubs for every helper name; local-bare-remote oracle tests move to `git://` |
| Adv C-d: `includeIf gitdir:` and symlinked parents / `$PWD` | Resolver evaluates `gitdir:` against the logical and the real path candidates and fails closed if the two give different include sets; oracle matrix adds a symlinked-parent fixture run with and without `PWD` |
| Arch C3: Story 1.1.5 names the wrong sites | Story 1.1.5 file list, tasks and acceptance criteria rewritten around `RemoteExecutionTarget.Runner()`, the five `git.WithCommandRunner` calls and `instance_worktree.go:140`; `remote_service.go:115,579` explicitly need no change |
| Arch C4: two more native-to-`session/git` dependencies | Story 1.1.0 rule 8 and the verified-dependency list: `MergeDeps` gains `Fetch`, `IsDirtyCleanCacheTTL` moves to `native`; "four hidden" becomes the scratch-compile list of eight; `go build ./session/git/native/` after each file group |
| Arch C5: `Locate` placement | `Locate` lives in `backend` taking a two-method interface (no `tmux` import); `gitwiring` keeps `NewRouter` and cohort parsing; depguard denies `gitwiring` to every package except the server entry points |
| Arch C6: gate matching and `remote_host` | The formula already excluded `remote_host` from the right side; a contradictory sentence in the Observability Plan said the opposite and is corrected; Story 5.3.1 now states it |
| Reviewer claims found wrong or imprecise | See the final report of this pass. Summary: 10 not 11 `hasconfig` includes; git does not skip hasconfig-included remote URLs, it dies; APFS inode reuse was not reproduced with 1 s gaps; `git --exec-path` runs only when `git-upload-pack` is not on `PATH`; git 2.50.1 segfaults on a bare `remote.<n>.url` when a `hasconfig` include is evaluated |

## Revision 3 change log (repair pass 2)
| Re-review item | Resolution in this plan |
|---|---|
| Arch B1: `native` cannot import `session/git`; `OpenRepo` not on move list; `*GitWorktree` methods; rule 2 vs `native_rollout.go` | Story 1.1.0 rewritten: `OpenRepo` moves to `native/open.go` (alias in `session/git`, nolint line moves, no analyzer change); the only two `*GitWorktree` methods (`native_worktree_add.go:67,108`) become functions; four more hidden dependencies found and handled (`WithRepoWorktreeLock`, `getHeadCommitSHA`, `MergeMainResult`, `gitignoreFSCache`); rule 2 now names the exact allowed leaf imports (`log`, `telemetry`, `redact`, OTel) with a strict depguard allow-list |
| Arch B2: one runner in `NewRouter`; `Remote` carries none | `Remote{Host, Path, Runner}` (runner built by the caller that already owns the SSH client pool); nil runner is a typed error, never `LocalRunner`; new Story 1.1.5 wires the three production runner construction sites |
| Arch C1 / C2 / C3 | `Runner.Run` now returns `([]byte, error)` (structural match with `tmux.CommandRunner`); spawn counter counts per `Runner.Run`; gate is `backstop <= local backend spawns`; `classifier.RefReader` has two named path-string methods |
| Adv N1: config reader is not git's resolver | Story 1.3.4 and ADR-006 redesigned: own `gitconfig` resolver (git precedence, `include`/`includeIf`, `GIT_CONFIG_*`), hook rule is "any executable non-`.sample` file", fail closed only on an unresolvable include or a listed environment variable; differential test against `git config --list --show-origin`; soak ceiling on `capability_detect_error` |
| Adv N2: locking read-your-writes, self-deadlock, refs, stale locks, reflog | Story 0.2.4 / 2.3.1 / ADR-003 rewritten: scoped storer serves `Index()` from the pending lock file; census of every `Index()`/`SetIndex()` sequence in go-git v5.19.2 with a census test; fallback is decided before the lock and only before the first irreversible rename; `<ref>.lock` then `packed-refs.lock` for delete and CAS; lock journal plus per-process flock for stale-lock recovery; reflog lines written in the ref writer (an op stays on CLI until its reflog parity test is green) |
| Adv R2 (G8/S7) | Three explicit S7 outcomes so G8 is satisfiable; `torn_read` narrowed to a typed error set and a named operation list, with a soak ceiling; shadow stays mandatory for `refs` |
| Adv R4 (gate bypasses) | Story 5.3.1 rewritten: 20 `Setenv("PATH"` sites and 29 `LookPath("git")` test skips counted (not 3 and 2), `testutil/spawngate` helpers and a `norawgitpath` check, skip-set ratchet, `-count=1`, `./...` scope, attribution by temp-dir name, allow-list ratchet test (no CODEOWNERS file exists), container coverage report |
| Reviewer claims found wrong | See the final report of this pass; summarised: `circuit_breaker_test.go:141` is not a spawn (it only builds an `exec.Cmd`); no `CODEOWNERS` file exists; `init.templateDir` need not be read for hook detection |


## Revision 2 change log (retained)
| Review blocker | Resolution in this plan |
|---|---|
| Arch 1: package topology / import cycle | New Story 1.1.0; leaf `session/git/backend`, lower-level `session/git/native`, composition root `session/gitwiring` (see 1.1.0); Stories 1.1.1, 1.1.2, 2.1.1, 2.1.2, 2.2, 4.1.1 revised |
| Arch 2: remote hosts / `tmux.CommandRunner` | Typed `RepoLocation` on every method; router sends `Remote` to `cli` unconditionally; `cli` backend executes through a `Runner` port; audit and lint cover runner calls |
| Arch 3: zero-spawn gate keyed on callsite | Counter and gate keyed on `operation` + `FallbackReason`; `safeexec` backstop counter has no callsite label |
| Adv 1: hooks/signing preflight | Redesigned (Story 1.3.4): resolve real git dir and commondir; effective scoped config incl. `core.hooksPath`; per-operation check, content+mode hashing only for cached expensive facts; fail closed |
| Adv 2: `refs` valid-SHA-no-object failure | New spike S7 (Story 0.2.8), gate G8, detection predicate, permanent `torn_read` route, Epic 6.2 may not delete it |
| Adv 3: locking | Operation-scoped `index.lock` design (ADR-003 amended); S4, G4, Story 2.3.1 rewritten; CLI-reader-sees-partial-index test |
| Adv 4: gate misses spawns outside `safeexec` | Two-layer gate (PATH shim + counters); 10 test `exec.Command("git")` sites, `gh`, `exec.LookPath("git")` added as scope/carve-outs with owners |
| C2/Claims 4, 8, 9 | F3 removed; histogram name corrected; O-8 closed |
| C1 | Stated above; O-6 promoted to G7a |
**ADRs**: [ADR-001](../decisions/ADR-001-fork-v5-not-v6.md) fork v5 not v6 · [ADR-002](../decisions/ADR-002-consume-fork-via-replace-directive.md) `replace` directive · [ADR-003](../decisions/ADR-003-meaning-of-no-git-and-permanent-lock-compatibility.md) meaning of "no git" and permanent CLI-compatible locking (NEEDS USER DECISION) · [ADR-004](../decisions/ADR-004-per-operation-backend-seam-and-cohort-switch.md) backend seam and cohort switch · [ADR-005](../decisions/ADR-005-credentials-and-ssh-live-outside-the-fork.md) credentials outside the fork · [ADR-006](../decisions/ADR-006-repo-capability-preflight-and-hooks-signing-policy.md) capability preflight and hooks policy (NEEDS USER DECISION)

Evidence labels: VERIFIED (command run or file opened; cited) vs INFERRED. Sources are the research files under `../research/`.

---

## 0. Direction, and what the fork is actually for

**Direction (binding, from the user)**: plan a long-lived private fork of go-git, 3 to 6 weeks, target "no git binary at runtime" for stapler-squad's own operations. The research recommends a hybrid with no fork. This plan does what the user chose, and states where the research findings shrink the fork.

### 0.1 What is already solved (do not rebuild)
- In-process linked-worktree add, remove, prune, list, unlock, and three-way merge exist in this repo on stock go-git v5.19.2: `session/git/native_worktree_{add,remove,prune,list,common}.go`, `native_admin_writer.go`, `native_merge*.go` (#730; flags removed in #849). The atomic admin-file protocol is in `project_plans/go-git-worktree-and-merge/decisions/ADR-001-atomic-admin-file-write-protocol.md`. (research/architecture.md 0, build-vs-buy.md headline 1.)
- A per-operation span and histogram labelled `operation`/`implementation`: `withOperationSpan`, `session/git/native_rollout.go:106`; the existing histogram is `git_operation_duration_ms` (`native_rollout.go:53`).
- Test fixtures partly in-process already (`b5af54fb8`, `85bd89b5c`, #955/#956).

### 0.2 Remaining CLI surface that matters (the real gaps)
| Gap | Where | Solvable without a fork? |
|---|---|---|
| Add worktree for an existing branch (resume path) | `session/git/worktree_ops.go:217` | Yes: reuse native admin writer (INFERRED; Story 4.1.1) |
| Self-heal `worktree remove` and `worktree list` | `worktree_ops.go:214,291`; `server/services/path_completion_service.go:217` | Yes: `nativeRemoveWorktree`, `nativeListWorktrees` already exist (VERIFIED, features.md A16/A17/D1) |
| Remote-host worktrees over SSH | `session/git/remote_worktree.go:86,103`; `server/services/session_service_create.go:372` | **No, by construction**. Carve-out (ADR-003) |
| Network: clone, fetch, push, pull, credentials | `session/repo_path.go`, `session/git/ops.go:36`, `session/vc/git_provider.go`, `unfinished_work_service.go:476` | Yes for http(s) and ssh, outside the fork: credential client plus `AuthMethod` (ADR-005). **No for local-path and `file://` remotes**: go-git's `file` transport execs `git-upload-pack`/`git-receive-pack` (and `git --exec-path` when they are not on `PATH`) through `execabs`, `plumbing/transport/file/client.go:39-99` (VERIFIED), so those remotes are a CLI carve-out (`capability_local_transport`) and the transport is tripwired (Story 3.2.1) |
| Read ops: rev-parse family, for-each-ref, symbolic-ref, config, remote, merge-base, log, rev-list | ~25 sites | Yes (features.md A1-A15, B9-B13) |
| Index-vs-worktree and HEAD-vs-index diff, status parity and speed | `session/vc/git_provider.go:94,200,473,623-633`; `unfinished_work_service.go`, `backlog_review.go` | Partly: compose in-repo; reuse `worktree_dirty_fast.go` (BUG-104). Perf is gate G3 |
| Commit, add, restore, branch rename, push -u upstream config | `session/vc/git_provider.go`, `worktree_git.go:244` | Yes, subject to hooks/signing preflight (ADR-006) |
| **Index and ref writes that interoperate with CLI locks** (`index.lock`, `<ref>.lock`, `packed-refs.lock`) | go-git calls `Storer.Index()` and `Storer.SetIndex()` as separate calls, several times per operation (full census in Story 0.2.4: `worktree_status.go:134,354/390,412/441,586/603,682/708,723/741`, `worktree.go:375/436,452/484`, `worktree_commit.go:62,109/125`), `SetIndex` truncates in place (`storage/filesystem/index.go`, `dotgit.go:237` `IndexWriter` = `fs.Create`), `RemoveRef`/`packed-refs` use an inode flock and temp-and-rename with no `packed-refs.lock` (`dotgit.go:859-1010`), and go-git writes no reflog | A bare `Storer` decorator cannot serialize the read-modify-write, and one that diverts `SetIndex` to a lock file breaks read-your-writes unless `Index()` serves the pending content. In-repo: an **operation-scoped lock** (take `index.lock` before the read, write to the lock file, serve later reads from it, rename) around each mutating operation (Story 2.3.1), a ref writer that follows git's lock order and appends reflog lines, and a journal for stale locks. F1 is only needed if that cannot be made to work. Spike S4 decides |
| Object lookup survives a concurrent `git repack`/`gc` (#2242) | go-git packfile index | Probably in-repo: `ObjectStorage.Reindex()` is public (`object.go:74`) but not goroutine-safe (sets `s.index = nil` unlocked), so the wrapper opens a fresh repository per retry. Spike S6 decides |
| Index v3/v4 (skip-worktree, intent-to-add) | go-git index decoder/encoder | **Supported** in v5.19.2 (decoder min 2 max 4, `decoder.go:19`; encoder handles v2 to v4, `encoder.go:17,144`; `Entry.SkipWorktree`/`IntentToAdd`, `index.go:150-153`). No fork patch. Spike probe round-trips real v3/v4 indexes (Task 0.2.3b). Optional extensions (UNTR, FSMN, REUC) are dropped on write (harmless); unknown mandatory extensions (split index `link`) return `ErrUnknownExtension` (`decoder.go:308`), mapped to CLI fallback `capability_unsupported_index` |
| Valid-SHA-with-no-object `HEAD` read on linked worktrees (documented at `session/git/util.go:326-345`) | go-git ref read against a linked worktree | Unknown root cause. Spike S7 (Story 0.2.8) is required before the `refs` cohort leaves `shadow` |
| Checkout performance for worktree creation (#1956) | go-git `Checkout` | Fork (F4) only if G3 shows a regression vs CLI |

### 0.3 Fork deliverables: needed versus already solved
A patch lands in the fork only if it passes the **fork-only test**: a spike demonstrates, with a failing test, that no wrapper around public go-git API (custom `storage.Storer`, `billy.Filesystem`, `client.InstallProtocol` transport, or in-repo helper) can deliver it. Candidates and their status:

| ID | Patch | Needed? | Decided by |
|---|---|---|---|
| F0 | Fork repo, `ssq/v5` branch, CI, `replace` wiring, rebase runbook | **Yes** (the user's direction; infrastructure even with zero patches) | G1 |
| F1 | An **operation-level lock API** (not merely a `SetIndex` hook): hold `index.lock` (O_EXCL) from before `Index()` through the rename, write the new index to the lock file then rename; same shape for `<ref>.lock` | Unlikely. The in-repo design wraps whole operations (`WithIndexLock(repo, fn)`) and substitutes a temp-write-and-rename `Storer.SetIndex` inside that scope; F1 only if S4 shows go-git writes the index along a path the wrapper cannot reach | S4 / G4 |
| F2 | Retry object/pack-index refresh on `object not found` after repack | Unlikely (`Reindex()` is public) | S6 / G6 |
| ~~F3~~ | ~~Index v3 read/write~~ | **Removed.** Premise contradicted by v5.19.2 source (above) | n/a |
| F4 | Checkout/Status perf patches | Only if measured regression | S3 / G3 |
| F5 | Cherry-picked security fixes ahead of upstream releases | Only on demand | Runbook 6.3 |

**Fork patch constraint (adversarial C3)**: fork patches must be behaviour-only: no new exported symbol that app code calls, or the call is isolated behind a build tag with a stock-go-git fallback, so deleting the `replace` line still compiles. Each fork tag's acceptance criteria include the rollback test (Story 1.2.3). A private fork needs `GOPRIVATE` plus a token, and GitHub Actions secrets are not exposed to PRs from forks, so external-contributor CI and Dependabot break; this favours a public fork (O-4).

**Honest sizing**: see the end-state statement at the top. The fork is expected to be an empty pinned F0, or at most one small patch. Most of the 3 to 6 weeks is the backend seam, the credential client, diff/status work, test migration and rollout. If at the end of Phase 0 no patch passes the fork-only test, O-6 goes to Tyler at G7a before Epic 1.2. The no-fork hybrid (stock go-git, no `replace`) remains the fallback at every gate.

### 0.3.1 Calendar and cut line (adversarial C6)
Appetite is 3 to 6 weeks and the stated rule is "cut scope, do not move the deadline". Windows below run in parallel unless noted; dogfood windows (7 days shadow, 14 days default-clean) overlap with development of the next cohort, they are not sequential.
| Week | Work |
|---|---|
| 1 | Phase 0: Story 0.1.1 audit (including the destructive-call-site list, Task 0.1.1c), Story 0.1.2 baseline, spikes S1, S2, S3, S6, S7 (1 to 2 days each, two agents in parallel); **S4 is reduced to its go/no-go core** (lost-update and partial-read reproduction plus a `WithIndexLock` prototype; the rest of its criteria belong to Story 2.3.1 and are stretch); S5 is stretch. **Epic 5.1 starts now** (Story 5.1.1 helpers, 5.1.2 batch 1). G7a checkpoint end of week 1 |
| 2 | Epic 1.1 (topology, interface, router), Epic 1.3 (redactor, counters, **resolver v1**, oracle harness); fixture batches 2 to 4; `refs` enters `shadow` end of week. **Gate GS-1 report** (spawn count and wall time vs Story 0.1.2) |
| 3 | `refs` shadow window runs (7 days and at least 500 shadowed calls per operation family) while Epic 2.2 (`diffstatus`) and Epic 4.1 (worktree residue, already-native code) are built; fixture batches continue. **Tripwire T3 at end of week 3** |
| 4 | `refs` flips; `diffstatus` enters `shadow`; zero-spawn gate Layer 1 in report-only mode |
| 5 | `diffstatus` flips only if `false_clean` is 0 over its window (Story 2.2.1); worktree residue flips; Epic 5.2 soak (read, worktree and fixture variants) |
| 6 | Rollout evidence, gate Layer 1 graduation check, nightly SLO check wired (Story 5.2.1) |
| Stretch (outside the 3 to 6 weeks, own go/no-go **GL**) | Epic 2.3 `localwrite` (including the lock layer, journal and reflog writer), Epic 3.1/3.2 network and credential client, resolver v2 (full git-compatible resolver, Story 1.3.4). Not scheduled; entered only on an explicit Tyler decision at GL after the MVP shipped |
**MVP cut (what ships)**: seam + router + `refs` + `diffstatus` (reads, destructive intents CLI-confirmed) + `worktree` residue (already-native code) + in-process test fixtures + zero-spawn gate (Layer 1 first). **`localwrite`, `network` and resolver v2 are not part of this project's appetite** (they were the first two drop-order items before; they are now explicit stretch, so the appetite is not blown by default).

**Tripwire T3 (scope freeze, recorded in `gates.md` at end of week 3)**: if any of (a) resolver v1 is not green, (b) `refs` has not entered `shadow`, (c) gate GS-1 shows no reduction in `session` package spawn count against the Story 0.1.2 baseline, then scope freezes at seam + `refs` + test fixtures and the drop order below applies immediately, in this order, first dropped first: (1) Layer 2 of the zero-spawn gate (Story 5.3.1; Layer 1 stays), (2) Epic 4.1 worktree residue (stays CLI), (3) Story 2.2.2 diff text and numstat (keep `IsDirty`/`Status`), (4) Epic 2.2 `diffstatus` altogether (stays CLI). Never dropped: seam, router, `refs`, fixture migration. Epic 6.2 (one release later) is outside the appetite regardless.

**Gate GS (spawn-reduction checkpoint)**: a weekly line in `gates.md` with the `session` package spawn count and wall time against Story 0.1.2. Provisional targets (Tyler may reset them after Story 0.1.2 gives the fixture-versus-product split): spawns down at least 30% by end of week 3 (GS-1) and at least 60% by end of week 5 (GS-2). No stretch work (GL) may start before GS-2 is met.

**Gate GL (stretch go/no-go)**: Tyler decides whether to open the `localwrite`/`network`/resolver-v2 stretch phase, with evidence: MVP shipped, GS-2 met, the share of the maintainer's repos routed to the CLI by resolver v1 (Story 1.3.4), and S4/S5 outcomes.

### 0.4 Creative pass: three approaches considered
| Approach | Key strength | Key weakness |
|---|---|---|
| A. Deep fork: rewrite go-git storage for per-worktree HEAD/index/commondir, CLI locking, perf | Total control; one dependency | Rebase against upstream's active worktree/storage work (#2336, #1956, #1896 touched the same layer within 6 weeks); duplicates validated native code |
| **B. Thin-patch fork plus out-of-tree adapters (chosen)** | Honours the user's fork direction, keeps rebase cost bounded, reuses native worktree code | The fork may turn out nearly empty; adapters live in the app, not the fork |
| C. No fork: hybrid plus Option 4 CLI-cost levers (the research recommendation) | Least maintenance, stays on stable v5 | Contradicts the user's explicit choice; kept as the fallback at every gate |

---

## Domain Glossary

| Term | Definition | Notes |
|---|---|---|
| `Backend` | Interface in leaf package `session/git/backend` with one typed method per distinct git operation | Callers never build argv or parse git text; the package imports nothing from `session/git`, `session/tmux` or `session/lifecycle` |
| `RepoLocation` | Sum type: `Local{Root}` or `Remote{Host, Path, Runner}`; first parameter of every `Backend` method | `Router` sends `Remote` to `cli` unconditionally, reason `remote_host`, no cohort override. A `Remote` with a nil `Runner` is the typed error `ErrNoRemoteRunner`; it never falls through to the local runner. `Host` is the identity for logs; `Runner` is the per-target SSH runner the caller already owns |
| `Runner` | Port in `backend`: `Run(ctx, dir, name string, args ...string) ([]byte, error)`, byte-for-byte the signature of `tmux.CommandRunner.Run` (`session/tmux/command_runner.go:52`), so any `tmux.CommandRunner` (`LocalRunner` or `*SSHRunner`) satisfies it structurally (the interface is a subset: `Start`/`IsRemote` are not required). Returns combined stdout and stderr. Optional `StdoutRunner` (`RunStdout`, same shape) is preferred by `cli` when present | Keeps remote-host sessions working; avoids importing `session/tmux` into `backend` |
| `native` package | `session/git/native`: the on-disk worktree/merge/dirty-fast code extracted from `session/git` with exported entry points, plus `OpenRepo` | Lowest layer; imports go-git, billy, stdlib and exactly the leaf packages listed in Story 1.1.0 rule 2 |
| Composition root | `session/gitwiring` (new, `NewRouter(cohorts backend.CohortMap, local backend.Runner) backend.Backend`), called once from server startup and the `session` manager; owns `NewRouter` and cohort parsing only | The only place that imports `backend`, `cli`, `gogit` and `config` together; no package under `session/git`, `session/vcs`, `session/vc` or `pkg/` may import it (depguard). `Locate(r backend.Locatable, host, path string) backend.RepoLocation` lives in `backend` (`Locatable` is `Runner` plus `IsRemote() bool`, so `tmux.CommandRunner` satisfies it without a `tmux` import) |
| `Operation` | Name of one `Backend` method, e.g. `ResolveRef`, `Fetch` | String newtype `OperationName`; closed set |
| `Cohort` | Group of operations that flip backend together: `refs`, `diffstatus`, `localwrite`, `network`, `worktree` | Sum type, not string |
| `BackendMode` | `cli` / `gogit` / `shadow` | Sum type; `shadow` applies **per operation**, only to read-only operations. In the `network` cohort `fetch`/`ls-remote` may shadow; `push`, `clone` never do |
| `CohortMap` | `config.Config.GitBackendCohorts map[string]string` (`git_backend_cohorts`); absent key means `cli` | Parsed once into `map[Cohort]BackendMode` |
| `Implementation` | `cli` or `gogit`, the backend that actually ran | Existing `withOperationSpan` label |
| `FallbackReason` | Closed enum: `config`, `remote_host`, `agent_tool` (`gh`, ADR-003 tier 2 carve-out), `torn_read`, `object_missing` (valid SHA, no object; S7), `object_not_found`, `capability_lfs`, `capability_hooks`, `capability_gpgsign`, `capability_unsupported_index` (unknown mandatory extension such as split index), `capability_sparse`, `capability_shallow`, `capability_ssh_proxy`, `capability_submodule`, `capability_local_transport` (remote is a local path or `file://`, so go-git would exec `git-upload-pack`), `capability_detect_error`, `unsafe_worktree_write` (operation writes worktree files and has no passing fault-injection parity test), `lock_unavailable` (lock journal could not record the intent, decided before any lock is taken), `live_session` (a `localwrite` operation on a repository where the session registry shows a running, non-paused session), `destructive_confirm` (go-git answered "clean" for an `Intent=Destructive` read, so the CLI confirms), `unsupported_pull_mode` (`Pull` with `pull.rebase` or a diverged branch, which go-git cannot do), `error` | Never free text; safe as a metric label. **Precedence when several reasons apply** (first match wins, evaluated before the backend call): `remote_host`, `config`, `agent_tool`, `capability_detect_error`, `capability_local_transport`, `capability_hooks`, `capability_gpgsign`, `capability_lfs`, `capability_unsupported_index`, `capability_sparse`, `capability_shallow`, `capability_submodule`, `capability_ssh_proxy`, `live_session`, `unsafe_worktree_write`, `unsupported_pull_mode`, `lock_unavailable`; the reasons produced by a gogit call (`torn_read`, `object_missing`, `object_not_found`, `destructive_confirm`, `error`) come after. `capability_index_v3` was removed (go-git supports v2 to v4) |
| `RepoCapabilities` | Facts about a repo that decide CLI routing (ADR-006). Expensive facts (LFS filter attributes, index extensions) are cached keyed by resolved `CommonDir` plus the content hash of **every file in their input closure** (config include closure, attribute files); hooks, signing and `core.hooksPath` are **re-evaluated on every mutating call** | Never keyed on `.git/hooks` directory mtime (wrong for linked worktrees, chmod, hooksPath, global config) |
| `CapabilityPreflight` | Function computing `RepoCapabilities` from an `EffectiveConfig` | Fails closed only on the cases listed in Story 1.3.4 |
| `GitConfigResolver` | `session/git/native/gitconfig`: own tokenizer (`parse.go`) plus resolver computing the `EffectiveConfig` the CLI would see (system, XDG, `~/.gitconfig`, repo, `config.worktree`, `include`/`includeIf` incl. `hasconfig:remote.*.url:`, `GIT_CONFIG_*`), recording the include closure | Not go-git's `ConfigScoped`/`LoadConfig` (stops at the first existing global file, `config/config.go:183-194`, follows no include) and not go-git's `plumbing/format/config` decoder (loses include position, conflates bare key with `key =`) |
| `LockJournal` | Append-only intent/held/released records, one journal per app config directory, each lock carrying a random token (xattr on the lock file) | Lets startup remove only a lock a dead instance of this server left behind and nobody has touched since; never an identity based on inode |
| `RepoRoot` / `GitDir` / `CommonDir` | Worktree top, per-worktree admin dir, shared `.git` | Distinct newtypes; `GitDir != CommonDir` in a linked worktree |
| `LinkedWorktree` | Worktree whose `.git` is a file pointing at `<CommonDir>/worktrees/<AdminDir>` | |
| `AdminDir` | `<CommonDir>/worktrees/<name>` holding `HEAD`, `gitdir`, `commondir`, `index`, `locked` | Allocated atomically with numeric-suffix retry |
| `CLICompatLock` | Operation-scoped lock: `index.lock` (O_EXCL) taken **before** the index is read; inside the scope the storer serves `Index()` and ref reads from the pending lock-file content (read-your-writes) and **buffers ref writes behind held `<ref>.lock` files**; at scope end it commits in a fixed order (index rename, then ref renames, `HEAD` last); an error before the commit phase discards index and refs together. Likewise `packed-refs.lock` (delete, compare-and-set), `HEAD.lock`, `config.lock`. Contention returns `ErrLocked`; a lock we did not create is never deleted | Permanent constraint (ADR-003). Implemented as `WithIndexLock(repo, fn)` around a whole `Add`/`Commit`/`Restore`; no CLI fallback is ever taken while a scope is open |
| `ServerProcess` | The stapler-squad daemon | Target of "no git" |
| `AgentProcess` | Claude Code, Aider, `gh`, IDEs and users running git in a worktree | Out of scope; always a CLI writer |
| `RemoteHost` | Machine reached via `sshremote` where git runs for remote sessions | Carve-out |
| `ZeroSpawnGate` | Two layers. Layer 1: the suite (`./...`, `-count=1`) runs with a recording shim directory on `PATH` holding a stub for every helper name the server or go-git can exec (`git`, `git-upload-pack`, `git-receive-pack`, `git-upload-archive`, `git-lfs`, `gh`) (and a no-git `PATH` run with a skip-set ratchet); any hit outside the allow-list fails. Layer 2: `git_backend_cli_spawn_total{operation,reason}` counted once per `Runner.Run` of `git`, keyed on operation name plus `FallbackReason`, plus a label-free `safeexec` name=="git" backstop counter; pass iff `backstop <= sum(backend counter, reason != remote_host)` | Allow-list is keyed on (operation, reason), never on file:line, lives in one file and may only shrink (ratchet test) |
| `OracleTest` | Test that uses the real git CLI to validate in-process output; build tag `gitoracle` | Exempt from `ZeroSpawnGate` (shim-hit allow-list entry by tag) |
| `Redactor` | `redact.Git(string) string` stripping URL userinfo, tokens, credential-helper output | Applied before any log, span, or label |
| `CredentialProvider` | Resolves a `transport.AuthMethod` for a host (keychain, `gh` token, helper binary) from an injected `TokenSource` (`TokenForHost(host string) string`) plus the credential-helper client | Out-of-tree (ADR-005); never imports the `github` package |
| `ForkTag` | `vX.Y.Z-ssq.N` tag on `tstapler/go-git` | Used in the `replace` line |
| `SpikeGate` | Named go/no-go decision with a recorded outcome in `implementation/gates.md`: `G0`..`G8` plus `G7a` (spikes S1 to S7 map to `G1` to `G6` and `G8`: S7 is Story 0.2.8 and gate `G8`; `G7`/`G7a` are the user checkpoints in Story 0.2.7), and the schedule gates `T3` (week-3 tripwire), `GS` (spawn-reduction) and `GL` (stretch go/no-go) in section 0.3.1 | Later epics list their gate |
| `Intent` | Sum type on `IsDirty`/`Status`/`DiffNumstat` requests: `Display` or `Destructive` (the answer gates worktree removal, cleanup, pause worktree delete or a review-gate pass) | `Destructive` + gogit "clean" is confirmed by the CLI (`destructive_confirm`); gogit "dirty" is trusted |
| `ShadowMismatch` | A `shadow`-mode call where CLI and gogit results differ. Classes: `racy` (disappears on the pinned re-read), `real` (persists), `false_clean` (a `real` mismatch where gogit said clean and the CLI said dirty; never waived) | `real` and `false_clean` block flipping the cohort |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| `Backend` seam | Strategy plus Adapter around two implementations; ports-and-adapters with `backend` as the leaf port package, one composition root | GoF / Hexagonal | Call-site `if flag` branches | No shared counters; scatter (ADR-004) |
| Cohort routing | `routed` Decorator over `Backend` | GoF | Per-op booleans in `FeatureFlags` | `map[string]bool` cannot express `shadow`; 50-flag matrix |
| Backend operations | Service Layer returning typed results | PoEAA | Transaction Script per call site | Eliminates text parsing at 62 sites |
| `OperationName`, `Cohort`, `BackendMode`, `FallbackReason` | Sum types / closed enums, parse-don't-validate at config load | type-driven-design | raw strings | Illegal modes unrepresentable; safe metric labels |
| `RepoRoot`, `GitDir`, `CommonDir` | Distinct newtypes | type-driven-design | `string` paths | Linked worktrees make these differ; the silent wrong-answer trap (features.md A9) |
| `CapabilityPreflight` | Specification (predicate set); cheap predicates evaluated per call, expensive facts cached by content hash; fails closed | Fowler/Evans | `.git/hooks` mtime cache; ad hoc checks per call | One place to route unsupported repos without staleness |
| Index/ref writes | Operation-scoped `CLICompatLock` (Unit-of-Work style transaction helper around whole mutating operations) with temp-write-and-rename | PoEAA Unit of Work | A `Storer` hook that locks inside `SetIndex` (Index()/SetIndex() are separate calls: cannot serialize read-modify-write); patch go-git first | Lock must span read to rename, and the scope's storer must serve its own pending writes (S4 verifies against the call-site census); F1 only if unreachable |
| Credentials | Strategy chain (keychain, `gh` token, helper binary) | GoF | Patch go-git, or shell to `git credential` | ADR-005 |
| Fallback | Circuit of named reasons with counter | Release It! | Silent fallback | Measurable zero-spawn (requirements Observability) |
| Fork | Additive patch series on `ssq/v5` | ADR-001 | Deep storage rewrite | Rebase cost |
| Differential testing | Oracle/golden tests against real git | Test oracle | Mock-only tests | Mistakes only appear when real git touches the repo (build-vs-buy) |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|---|---|---|---|
| 62 scattered CLI sites in 29 files | No operation-level seam (architecture.md 1) | **Isolate via seam** (Epic 1.1) | Sites average about 2 per file; big-bang delivers nothing |
| `session/vcs/` and `session/vc/` duplicated git+jj layers | Two CLI-text-based abstractions (`vcs.GitClient.run`, `vc.GitProvider.runGit`) | **Decision task Story 0.1.3** (after the 0.1.1 audit): delete the dead layer, or record which layer owns each operation, before Epic 2 so no operation is implemented twice. Live layer(s) route through `Backend`; layers are not merged | Reviewer concern: avoid implementing each op twice |
| `session/vcs/detect.go:123` `exec.LookPath("git")` pre-check | Returns "git not available" in a no-git container before any backend runs | **Refactor-first** (Story 5.3.2a): replace with `Backend` capability probe; owner Story 5.3.2 | Would disable git entirely in the no-git run |
| `gh pr create` at `session/git/worktree_git.go:581` | `gh` runs git internally, invisible to every counter | **Carve-out** (ADR-003 amended; decision O-10) | Out of server-process scope |
| `GitWorktree.commandRunner()` (`session/git/worktree.go:392`), the git-bearing runner path (`session_service_create.go:483` builds the `*tmux.SSHRunner`, `:573` wraps it in `session.NewRemoteExecutionTarget`; `session/execution_target.go:119` `RemoteExecutionTarget.Runner()`; five `git.WithCommandRunner(runner)` calls at `session/instance_worktree.go:152,161,196,265,271`; a direct `runner.Run(..., "git", "rev-parse", ...)` at `:140`; `NewRemoteWorktreeOps(runner)` at `session_service_create.go:354,525`) | Existing `tmux.CommandRunner` seam for remote hosts; the SSH runner is built per target from server state (key store, known hosts, client pool). `remote_service.go:115` (dial test) and `:579` (health prober running `true`) never run git (VERIFIED by reading both) | **Preserve**: the runner travels inside `backend.Remote{Host, Path, Runner}`, built by `backend.Locate` where the runner already is (Story 1.1.5); nothing reconstructs an SSH runner | Remote sessions must not regress; `backend` cannot import `server/services` or `tmux` |
| `session/repo_path.go` (7 CLI sites, clone/fetch with token URLs) | Network ops with token-bearing argv and CLI-error leakage | **Refactor-first** into the network cohort (Story 3.2.1) | Highest credential-exposure site |
| `native_rollout.go` `withOperationSpan` records `err.Error()` verbatim | Credential leak into OTel (architecture.md 3.5) | **Refactor-first**: apply `Redactor` (Story 1.3.1) before any new network path | Security |
| `session/git/util.go:326-345` `getHeadCommitSHA` retry-then-CLI | Ad hoc torn-read workaround | **Isolate via seam**: becomes the `refs` cohort retry policy | One named failure mode, one counter |
| `session/git/native_*.go` bespoke worktree/merge (31 files incl. tests; 13 non-test) plus `worktree_dirty_fast.go`, `gitignore_fs_cache.go`, `OpenRepo` | Large bespoke on-disk-format code (~7k lines incl. merge), mostly unexported in `package git`, with eight dependencies on non-native `session/git` symbols (Story 1.1.0) | **Extract** into `session/git/native` with exported entry points (Task 1.1.0a); `session/git` keeps thin wrappers and type aliases so existing callers do not change; new native code goes in `native`, not `session/git` | Validated by fuzz/differential tests; keep oracle tests permanently. Needed so `backend/gogit` can call it without a cycle |
| `session/git/scaffolding.go:56,79` unlocked `Storer.Index()`/`SetIndex()` | In-process index write with no `index.lock` (its own doc comment says so, `scaffolding.go:10`) | **Fold into** `WithIndexLock` in Story 2.3.1 | Same lost-update hazard as any other in-process index write |
| 20 `Setenv("PATH"` test sites in 12 files and 29 `exec.LookPath("git")` test sites in 10 files (`git grep` 2026-10-08, excluding `.claude`) | Replace or skip around any PATH shim, so the zero-spawn gate and the no-git run can pass vacuously | **Refactor-first** (Story 5.3.1b): `testutil/spawngate` helpers; owner Story 5.3.1 | The gate is only as strong as its least-covered test |
| `session/git/worktree_dirty_fast.go` | Hand-rolled status fast path (BUG-104) | **Extend as-is**; reuse for status cohort | Already the performance answer |
| `testutil/gitfixture/identity.go` (CLI in non-test package) | Test support compiled into product | **Isolate via seam** (Story 5.1.1, single owner; Story 2.3.2 only consumes it) | Counts toward the zero-spawn gate |
| 10 direct `exec.Command("git"` sites in `*_test.go` (`server/services/backlog_service_test.go`, `backlog_service_triage_test.go`, `session/backlog_lifecycle_test.go`) and other test helpers outside `safeexec` | Invisible to the `safeexec` counter | **In scope of the shim layer** (Story 5.3.1); migrated in Story 5.1.2c, owner 5.1.2c | Requirements metric 1 is defined with a PATH shim |
| No-`-C` call sites (A13 x2, B3, B12) | Inherit process CWD, latent bug | **Refactor-first** within Story 2.1.2 | Cheapest fix; interface takes `RepoRoot` explicitly |

---

## Migration Plan
No database schema. Two additive data changes:
- **Config**: new field `git_backend_cohorts` in `config.json`. Backward compatible: absent means every cohort is `cli`. Reversible: remove the key. Existing configs are never rewritten on load.
- **`go.mod`**: add `replace github.com/go-git/go-git/v5 => github.com/tstapler/go-git/v5 vX.Y.Z-ssq.N`. Zero-downtime is not relevant (build-time). Rollback: delete the `replace` line, run `go mod tidy`, rebuild. Verified by Story 1.2.3.
- **On-disk repo state**: no format is changed; in-process writers must produce state real git reads (`git fsck`, `git worktree list`) and use CLI-compatible locks, checked by `OracleTest`s.

## Observability Plan
- **Logs** (slog fields `op`, `cohort`, `implementation`, `fallback_reason`, `duration_ms`; all strings pass `Redactor`): entry/exit at each `Backend` method; fallback logged at WARN with reason; shadow mismatch logged at WARN with a redacted diff summary; credential provider logs host and provider kind only, never tokens or helper output; startup logs the effective `CohortMap`.
- **Metrics** (registered with `mustInt64CounterGit`, `native_rollout.go`):
  - `git_operation_duration_ms{operation,implementation}` (the existing histogram, `native_rollout.go:53`; extend its coverage to all ops over 100 ms: status, diff, fetch, push, clone, worktree add; do not create a second histogram)
  - `git_backend_fallback_total{operation,cohort,reason}`
  - `git_backend_cli_spawn_total{operation,reason}`: counted by a counting `Runner` wrapper once per `Run` of `git` (so a `cli` method that runs two `git` commands counts two), `operation` is the typed `OperationName`, `reason` is the `FallbackReason`; the router puts both in the `context.Context` (`backend.WithCallInfo`) that reaches the wrapper (not `runtime.Caller`); both closed enums, no URLs/args. This is the zero-spawn gate metric.
  - `git_cli_spawn_backstop_total{subcommand}`: counted in `executor/safeexec` when `name=="git"`, **no callsite label**. `LocalRunner` wraps `safeexec` (`session/tmux/command_runner.go:82`), so every local backend spawn increments both counters; an SSH run increments only the backend counter (reason `remote_host`), because no local `git` starts. Gate invariant: `sum(backstop) <= sum(git_backend_cli_spawn_total{reason != "remote_host"})`. A bypass makes the left side larger and fails the gate; a smaller left side means the wrapper miscounted and is a test failure of the wrapper, not a pass. `remote_host` is **excluded from the right side** on purpose: SSH runs never reach the backstop, so counting them on the right would let remote runs mask a local bypass. The inequality (not equality) covers a wrapper that over-counts; the exclusion covers remote runs. `file:line` appears only in the `SSQ_GIT_SPAWN_DUMP` test output, never as a metric label.
  - `git_capability_detect_total{outcome}` (`ok`, `hooks`, `gpgsign`, `lfs`, `unsupported_index`, `detect_error`, ...): feeds the soak ceiling on `detect_error` (Story 1.3.4).
  - `git_lock_stale_recovered_total{outcome}` (`removed_own`, `reported_foreign`, `reported_untokened`, `reported_live_session`, `reported_too_young`, `reported_owner_alive`, `recovery_unsupported_fs`): Story 2.3.1 journal recovery.
  - `git_backend_shadow_mismatch_total{operation,class}` (`class` is `racy`, `real` or `false_clean`) and `git_backend_shadow_calls_total{operation}` (the window denominator, so an idle machine cannot pass vacuously); each `real`/`false_clean` mismatch also writes a redacted repro record (operation, both results, HEAD SHA, index mtime, repo-root hash, never a path or URL with credentials) under the config dir's `shadow-mismatches/` for triage
  - `git_backend_postwrite_verify_total{operation,outcome}`: dogfood-only post-write CLI verification of in-process writes (Story 6.1.1 write-cohort evidence); off in CI gate runs
  - `git_backend_error_total{operation,implementation}`
  - `git_credential_resolve_total{provider,outcome}`
  - `git_lock_wait_ms{lock}` and `git_lock_contention_total{lock}`
- **Alerts**: stapler-squad is a local single-user daemon, so nothing pages. Gates instead: (a) CI fails when the PATH-shim run or `git_backend_cli_spawn_total` in the full test run shows a spawn with an (operation, reason) pair outside the allow-list, or when the backstop exceeds the local backend spawn total; (b) a cohort flip PR is blocked if the dogfood window shows any `shadow_mismatch_total{class="real"|"false_clean"} > 0` (`racy` is reported, not blocking) or any `fallback_total{reason="error"}`; (c) WARN log plus a "CLI-routed repos" debug view for capability fallbacks.

## Risk Control
- **Feature flag**: `git_backend_cohorts` (ADR-004); default `cli` for all five cohorts until that cohort passes its gate. Per-operation override `<cohort>/<op>`. Environment override `STAPLER_SQUAD_GIT_BACKEND=cli` forces everything to CLI for emergency rollback without editing config.
- **Rollback procedure**: set the cohort (or the env var) back to `cli` and restart; no data migration to undo. If the fork itself is suspect, delete the `replace` line and rebuild (works because fork patches are behaviour-only, see 0.3). Dead CLI branches are not deleted until the cohort has run in the maintainer's production instance for one full release cycle (release-please cadence). Permanent CLI routes (remote_host, capability_*, torn_read/object_missing unless S7 fixes them) are never deleted.
- **Shadow overhead and sampling**: `shadow` doubles read cost (CLI plus go-git) on a machine where EDR makes spawns the dominant cost. Expected overhead is one extra in-process read per shadowed call (microseconds to tens of ms) on top of the unchanged CLI spawn. Shadow samples 100% in the first 24 h then 10% for the rest of the window (config `git_backend_shadow_sample`, default 1.0).
- **Staged rollout**: Stage 0 `shadow` for read operations on the maintainer's instance. Stage 1 cohort `gogit` on the maintainer's instance, in order refs, diffstatus, localwrite, network, worktree. Stage 2 default flips to `gogit` in release N with `cli` override available. Stage 3 release N+1 removes the cohort flag and the non-capability fallbacks for cohorts that stayed clean; the `cli` backend itself stays as a permanent implementation for ADR-003/006 carve-outs. Worktree and network cohorts flip last. **`localwrite` (stretch) never default-flips in the first release**: it stays opt-in per repository (`git_backend_localwrite_repos`, an explicit list of repo roots, default empty) and applies only where the session registry shows no running session (`live_session`).

## Unresolved Questions
Resolved from research (recorded so nobody re-asks):
- **Upstream linked-worktree?** Yes, v6 `x/plumbing/worktree` add/list/remove-metadata/open, experimental, absent from v5; no prune/lock/move; `Remove` leaves the directory (stack.md section 2, VERIFIED). Repo already has its own native implementation.
- **Which sites genuinely need the CLI?** features.md audit: CLI-only after native work are existing-branch add (D2), credential-helper network ops (E1-E7), commit with hooks/signing (C3), index/worktree diff text (B4-B6), status rename/untracked output (B1), remote-host git (D3, C5). Re-verified by script in Story 0.1.1.
- **ssh-fallback wrapper?** A dotfiles convenience at `~/.local/bin/git` (`stapler-scripts/git-ssh-fallback`), not product behaviour (build-vs-buy.md, pitfalls.md 3.3, VERIFIED).

Still unresolved:
- [ ] **O-1 What does "no git" mean** (ADR-003 tiers)? Server-process only, with agents/remote/credential helpers carved out. — blocks Story 5.3.2 wording and the final success metric — owner: Tyler
- [ ] **O-2 Hooks and signing policy** (ADR-006): route hooked/signed repos to CLI (default) versus implement in-process? — blocks Epic 2.3 — owner: Tyler
- [ ] **O-3 go-git performance on the largest managed repos** (status, diff, worktree add vs CLI). Unmeasured. — blocks Epics 2.2, 2.3, 4.1 flips — owner: spike S3 (Story 0.2.3)
- [ ] **O-4 Public or private fork** (ADR-002; private needs `GOPRIVATE` and CI token plumbing). — blocks Story 1.2.2 — owner: Tyler (default private)
- [ ] **O-5 Is HTTPS-to-SSH fallback a product feature** or only the maintainer's wrapper? Default off, opt-in setting. — blocks Story 3.1.3 — owner: Tyler
- [ ] **O-6 DECISION POINT FOR TYLER (not hidden): if no patch passes the fork-only test, keep an empty pinned fork, or drop the fork and use stock go-git v5 (the research-recommended hybrid)?** The plan's expectation is that no patch will be needed (see the end-state statement at the top). Asked at G7a (end of week 1), before Epic 1.2 and before Phase 2 work. Default if silent: keep the empty pinned fork (the user's original direction), no fork patch epics run. Evidence that would justify a patch: a failing test against public API no wrapper can fix, or a G3 perf gap fixable in under 200 lines. — blocks Epics 1.2 and 4.2 — owner: Tyler, informed by G3, G4, G6
- [ ] **O-7 Realistic rebase burden per upstream release** (research gives cadence only: releases every 2 to 3 months, about 12 advisories in 18 months, INFERRED-grade). — blocks Story 6.3.2 — owner: spike S1
- [x] **O-8 RESOLVED**: `defaultPlainOpenOptions` sets `EnableDotGitCommonDir: true` (`session/git/util.go:36`). Story 0.1.1 only records it.
- [ ] **O-10 Does `gh pr create` (`worktree_git.go:581`) stay?** `gh` runs git internally and fails in the no-git container. Default: named ADR-003 carve-out (tier 2); alternative: replace with the GitHub REST API via the existing native `github` package. — blocks Story 5.3.2 wording — owner: Tyler
- [ ] **O-11 `session/vc` vs `session/vcs`**: delete the dead layer or name an owner per operation (Story 0.1.3). — blocks Epic 2.1 migration — owner: Story 0.1.3
- [ ] **O-9 Success-metric rewording**: "zero spawns" becomes "zero `git` spawns in `ServerProcess` except allow-listed carve-outs". — blocks Story 5.3.1 — owner: Tyler (follows O-1)
- [ ] **O-12 Provisional thresholds introduced in Revision 6** (all are proposals, none user-agreed): GS-1 30% and GS-2 60% spawn reduction; resolver v2 trigger at 10% of repos over-detected by v1; 500 shadowed calls per operation family; 20 shadowed status calls per repo and the 30-day repo list; gate graduation after 10 green `main` runs; 14-day write-cohort dogfood window. — blocks nothing; reviewed at G7 — owner: Tyler
- [ ] **O-13 Stretch scope** (gate GL): are `localwrite`, `network` and resolver v2 wanted at all after the MVP ships? Default if silent: not built. — owner: Tyler

### INFERRED claims the spikes must verify before dependent work starts
| Claim (source) | Spike |
|---|---|
| `replace` accepts a fork whose go.mod keeps the upstream module line (architecture.md 2). Go requires a replacement's `module` directive to match the replaced path, so the upstream-named line is the supported form and ADR-002's rename fallback is likely moot (adversarial minor) | S1 |
| `client.InstallProtocol` is a sufficient hook for a custom transport (features.md, architecture.md) | S5 |
| go-git v5 has no `ProxyCommand`/`IdentityFile`/`Include` handling (stack.md 3) | S5 |
| An operation-scoped lock (`index.lock` taken before read, temp write, rename) serializes against concurrent CLI `git add` and never exposes a partial index to a CLI reader (this plan) | S4 |
| A scoped `Storer` whose `Index()` decodes the pending lock file after the first `SetIndex()` makes every composite go-git sequence (`Commit(All)`, `Reset`, `Checkout`, ...) produce the same tree as the CLI (this plan; the stale-read hazard itself is VERIFIED in source, `worktree_commit.go:40-62,109-125`) | S4 (Task 0.2.4c) |
| `<ref>.lock` then `packed-refs.lock` in the ref writer excludes CLI `pack-refs`/`branch -d` (git's documented order; not verified by running it here) | S4 |
| Per-instance `flock` plus a lock journal identifies a dead server's locks without PID-reuse risk (this plan) | S4 / Story 2.3.1 `SIGKILL` test |
| Reflog lines written by our ref writer match the CLI's for commit, amend, rename, reset, checkout (git's reflog message formats; not verified here) | Story 2.3.1 parity tests |
| The own config resolver equals `git config --list --show-origin --show-scope --includes -z` for every fixture. Already reproduced against git 2.50.1 and now specified: `includeIf gitdir:` from a linked worktree matches the admin dir `<CommonDir>/worktrees/<name>` (both `gitdir:<main>/.git/worktrees/**` and `gitdir:<main>/` match, `gitdir:<worktree path>/` does not); `hasconfig:remote.*.url:` uses a URL-collection pass and `WM_PATHNAME` wildmatch (Story 1.3.4 (c)). Still unverified until the oracle runs: the whole matrix on Linux and on other git versions (git older than 2.36 has no `hasconfig`) | Task 1.3.4a0 oracle test |
| `plumbing/format/config` is **not** usable: VERIFIED against v5.19.2 that it merges repeated sections (include position lost), decodes bare `key` and `key =` both to `""`, and keeps `hooksPath`/`HooksPath` as separate options. The resolver uses its own tokenizer | Task 1.3.4a0 |
| Creating a temp file with the xattr (`user.ssq.lock-token`) and `link()`ing it to the lock name gives `O_EXCL` semantics with no tokenless window (Re-review 3: VERIFIED on APFS); the xattr works on APFS and the xattr survives `rename` (VERIFIED with `xattr -w`/`mv` on this machine); ext4/xfs/btrfs support `user.*` xattrs, tmpfs only from Linux 6.6, and some network filesystems do not (INFERRED) | Task 2.3.1c fixture on the CI filesystems; `ENOTSUP` disables auto-recovery (`recovery_unsupported_fs`) |
| The known ways a shim run is silently weakened are `go test` result caching, `Setenv("PATH")` (20 sites), skip-if-no-git (29 sites) and absolute git paths; there may be others (for example a test that clears the environment with `cmd.Env`), which the first full gate run in report-only mode is expected to surface | Story 5.3.1 |
| go-git can return a valid SHA with no object for a linked-worktree HEAD, and the object-exists check detects it (`session/git/util.go:333-345`) | S7 |
| go-git v5.19.2 round-trips index v3 and v4 (skip-worktree, intent-to-add) correctly; the decoder/encoder constants suggest yes (`decoder.go:19`, `encoder.go:17`) | Task 0.2.3b |
| Concurrent CLI `git add` plus go-git index write loses a write (pitfalls.md 0) | S4 |
| Wrapper can retry `object not found` after repack (pitfalls.md 0) | S6 |
| go-git `Status`/`Add` rename, untracked-dir, CRLF parity (features.md B1) | S3 |
| `CommitOptions.Amend` exists; hooks are not run (features.md C3) | Story 0.2.3b |
| `Worktree.Checkout` writes LFS pointers (pitfalls.md 2.1) | Story 0.2.3b |
| go-git `MergeBase`/`Log` fail on shallow repos (#2409) | Story 0.2.3b |

## Dependency Visualization
```
Phase 0  (all spikes first; each ends in a gate)
  0.1 baseline+audit+vc/vcs decision ─G0─┐
  0.2 S1 fork+replace ─G1┤   S2 v5 vs v6 ─G2
      S3 perf ───────G3  │   S4 locking ──G4   S5 credentials ─G5   S6 repack ─G6
      S7 refs/linked-wt HEAD ─G8      G7a user checkpoint (O-6) ─ end of week 1
      G7 full user checkpoint (O-1, O-2, O-4, O-5, O-10)      T3 tripwire ─ end of week 3
                         ▼
Phase 1  1.1.0 package topology ─► 1.1 Backend seam ──► 1.3 observability+redaction+oracle harness+resolver v1
         1.2 fork repo+replace (needs G1, G2, G7a)        5.1 fixtures start week 1 (GS-1, GS-2)
                         │
Phase 2  2.1 refs cohort (G8) ─► 2.2 diffstatus (G3) ─ ─ GL ─ ─► 2.3 localwrite (STRETCH: G3,G4,O-2,GL)
                         │
Phase 3  (STRETCH, after GL) 3.1 credentials (G5) ─► 3.2 network cohort
                         │
Phase 4  4.1 worktree residue (G3 and G4)    4.2 fork patches (G4/G6/G3 + O-6) ─┐
                         │                                               │
Phase 5  5.1 fixtures ─► 5.2 concurrency soak (reads+worktree; 2.3 variants only if stretch ran) ─► 5.3 zero-spawn + no-git container (O-1)
                         │
Phase 6  6.1 staged rollout ─► 6.2 flag/fallback removal, cli backend stays (+1 release) ; 6.3 fork runbook (parallel from 1.2)
```

---

# Phase 0: Spikes and baseline (do these first; no product code changes)

## Epic 0.1: Measurement and audit baseline
**Goal**: Reconcile the call-site counts with a repeatable command and close the audit gaps research left open.

### Story 0.1.1: Count call sites with a recorded script
**As the** maintainer, **I want** one reproducible command for product and test git-CLI site counts, **so that** progress is measured against a fixed definition.
**Acceptance Criteria**:
- A script prints product and test counts and per-subcommand histograms, and its output reconciles the requirements' "~50 / ~150" against the research's "62 / 223".
  - *Given* the repo at `test/go-git-fixtures` (HEAD `85bd89b5c`), *When* `scripts/git-cli-sites.sh` runs, *Then* it prints non-test `"git",` sites **62** in **29** files, test sites **223** (**121** under `session/`), direct `exec`/`safeexec` command-constructor sites **36** in **19** files, and go-git importers **30**. (VERIFIED 2026-10-08 with the commands in Task 0.1.1a; the three definitions differ, which explains the requirements' "about 50".)
- The audit answers whether `defaultPlainOpenOptions` enables `EnableDotGitCommonDir` and lists `session/vcs/git.go` `GitClient` subcommands.
  - *Given* `session/git/util.go`, *When* the open helper is read, *Then* `docs`-style note `implementation/audit.md` states yes/no and names each `GitClient` subcommand.
- The audit also counts the spawn sites that the `"git",` grep and `safeexec` counter miss, so the gate scope is honest.
  - *Given* the repo, *When* the script runs, *Then* it additionally prints: `commandRunner().Run(..., "git", ...)`/`runner.Run(..., "git"...)`/`runGitCommand(` product sites (25 on 2026-10-08 under pattern `\.Run\([^)]*"git"|commandRunner\(\)\.Run|runGitCommand\(`), direct `exec.Command("git"` test sites (**10**, in `server/services/backlog_service_test.go`, `backlog_service_triage_test.go`, `session/backlog_lifecycle_test.go`), `exec.LookPath("git")` sites (30 on 2026-10-08: 1 product, `session/vcs/detect.go:123`, and 29 in 10 test files), `Setenv("PATH"` test sites (20 in 12 files), and `gh` invocations that run git internally (`worktree_git.go:581`). Each category is assigned an owner story in the Tech Debt Disposition table. (Counts from `git grep -n 'LookPath("git")' -- '*.go' ':!.claude'` and `git grep -n 'Setenv("PATH"' -- '*_test.go' ':!.claude'`; a plain `grep -r` also walks `.claude/worktrees` copies and inflates both.)
**Files**: `scripts/git-cli-sites.sh`, `project_plans/go-git-fork-full-git-replacement/implementation/audit.md`

##### Task 0.1.1a: Write and run the counting script (~5 min)
- Script body (record verbatim in `audit.md`):
  `git grep -nE '"git",' -- '*.go' ':!*_test.go' ':!.claude' ':!third_party' | wc -l` (62), the same with `-l` (29 files);
  `git grep -nE '"git",' -- '*_test.go' ':!.claude' ':!third_party' | wc -l` (223); same under `session/` (121);
  `git grep -nE '(safeexec|exec)\.Command(Context)?\([^)]*"git"' -- '*.go' ':!*_test.go' ':!.claude' ':!third_party' | wc -l` (36; 19 files);
  `git grep -lE 'go-git/go-git/v5' -- '*.go' ':!*_test.go' ':!.claude' | wc -l` (30).
- Add a per-subcommand histogram (rev-parse, fetch, push, worktree, diff, status, ...).
- Files: `scripts/git-cli-sites.sh`

##### Task 0.1.1b: Close the audit gaps (~5 min)
- Record that `session/git/util.go:36` sets `EnableDotGitCommonDir: true` (O-8, already answered); enumerate `GitClient` methods in `session/vcs/git.go`; list callers of `session/vcs` vs `session/vc` to see whether either layer is dead.
- Files: `session/git/util.go` (read), `session/vcs/git.go` (read), `project_plans/go-git-fork-full-git-replacement/implementation/audit.md`

##### Task 0.1.1c: List destructive-decision call sites (~5 min)
- Enumerate every call site whose dirty/clean/status/diff answer gates a destructive or irreversible action (worktree removal, cleanup, `pause_session` worktree delete, review-gate "nothing to commit" pass, backlog terminal-state cleanup) and record them in `audit.md` as `destructive-sites`. Each becomes `Intent=Destructive` in Story 2.1.2/2.2.2c; any site not listed defaults to `Display`, so the list is reviewed by Tyler at G7. Start from callers of `IsDirty`/`IsDirtyUncached`/`worktreeIsDirtyFast` and of `git status`/`git diff` in `session/git/worktree_ops.go`, `session/git_worktree_manager.go`, `session/backlog_lifecycle.go`, `server/services/unfinished_work_service.go`, and the review gate.
- Files: `project_plans/go-git-fork-full-git-replacement/implementation/audit.md`

### Story 0.1.2: Baseline spawn count and test wall time
**As the** maintainer, **I want** a measured baseline of `git` execs and time, **so that** "no regression" and "zero spawn" have numbers.
**Acceptance Criteria**:
- `session` package test run reports its git exec count and wall time.
  - *Given* a PATH shim named `git` that appends one line to `$SSQ_GIT_SPAWN_LOG` then execs `/usr/bin/git`, *When* `go test ./session -timeout=20m` runs, *Then* the log line count is recorded in `audit.md` next to the wall time (post-#955 expected about 72 to 92 s, requirements) and the summed mutex delay.
**Files**: `scripts/git-spawn-shim/git`, `project_plans/go-git-fork-full-git-replacement/implementation/audit.md`

##### Task 0.1.2a: Write the shim (~3 min)
- POSIX sh, log subcommand only (no URLs), then `exec /usr/bin/git "$@"`.
- Files: `scripts/git-spawn-shim/git`

##### Task 0.1.2b: Run baseline and record (~5 min)
- `PATH=$PWD/scripts/git-spawn-shim:$PATH SSQ_GIT_SPAWN_LOG=/tmp/ssq-spawns.log go test ./session -timeout=20m`; record count by subcommand and wall time.
- Files: `project_plans/go-git-fork-full-git-replacement/implementation/audit.md`

### Story 0.1.3: Decide the fate of `session/vc` vs `session/vcs`
**As the** maintainer, **I want** one owner per git operation, **so that** Epic 2 implements each operation once.
**Acceptance Criteria**:
- Decision recorded.
  - *Given* the caller lists from Task 0.1.1b, *When* the decision is written, *Then* `audit.md` states for `vcs.GitClient` and `vc.GitProvider` either "dead, delete in Story 2.1.2" or "live, owns operations X"; and Story 1.1.2b moves code only from the layers marked live.
**Files**: `project_plans/go-git-fork-full-git-replacement/implementation/audit.md`

**Gate G0** (end of Epic 0.1): counts and baseline recorded in `implementation/gates.md`. Go: proceed. No-go: not applicable (no decision depends on the numbers beyond being on record).

## Epic 0.2: Feasibility spikes (time-boxed: 1 to 2 days each; outcomes written to `implementation/gates.md`)
**Goal**: Replace every INFERRED claim the plan leans on with a run result, and decide what the fork must contain.
**Numbering (consistency C5)**: spikes S1 to S6 are Stories 0.2.1 to 0.2.6 and gates G1 to G6. S7 is Story 0.2.8 and gate **G8** (there is no spike S8 and no gate G7 spike: `G7` and `G7a` are the user checkpoints in Story 0.2.7, which sits after Story 0.2.8 in this file because it consumes every spike's outcome). Gates in use: G0 to G6, G7, G7a, G8.

### Story 0.2.1: S1 fork bootstrap and `replace` viability
**As the** maintainer, **I want** to prove a `replace` to a fork builds the whole repo with type identity intact, **so that** ADR-002 stands.
**Acceptance Criteria**:
- Repo builds and tests against an unpatched fork.
  - *Given* a fork `tstapler/go-git` at tag `v5.19.2-ssq.0` (identical to the repo's pinned upstream v5.19.2; the bump to v5.19.3 is a separate later step so the S1 result is not confounded) and a branch with `replace github.com/go-git/go-git/v5 => github.com/tstapler/go-git/v5 v5.19.2-ssq.0`, *When* `go mod tidy && go mod verify && go build ./... && go test ./session/git/...` run, *Then* all exit 0 and `go mod graph | grep go-git` shows no second upstream copy.
- A trivial patch demonstrates the rebase flow and cost.
  - *Given* a one-line patch on `ssq/v5` and upstream tag `v5.19.3`, *When* the patch is rebased onto a scratch tag, *Then* conflicts are counted and recorded (feeds O-7).
**Files**: external repo `github.com/tstapler/go-git`; scratch branch of `go.mod`, `go.sum`; `implementation/gates.md`

##### Task 0.2.1a: Fork upstream and tag (~5 min)
- `gh repo fork go-git/go-git --clone=false --fork-name go-git`, set private per O-4 default; branch `ssq/v5` from tag `v5.19.2`; tag `v5.19.2-ssq.0`; then rebase onto `v5.19.3` as the first rebase-cost measurement.
- Files: external

##### Task 0.2.1b: Wire the replace on a scratch branch and build (~5 min)
- Add the `replace`; run `go mod tidy`, `go mod verify`, `go build ./...`; check `tools/lint/go.mod` for go-git imports and add a replace there only if present.
- Files: `go.mod`, `go.sum`, `tools/lint/go.mod`

##### Task 0.2.1c: Private-fork CI access check (~5 min)
- Set `GOPRIVATE=github.com/tstapler/go-git` and a token in one workflow (`.github/workflows/build.yml`) on a scratch branch; confirm module download. Record what the other four workflows and goreleaser need.
- Files: `.github/workflows/build.yml`, `.goreleaser.yaml`

**Gate G1**: Go if the build and tests pass with identical types. If `replace` fails but a rename works, record the identity break and reopen ADR-002. **No-go on the fork** if neither works; fall back to the hybrid and tell the user.

### Story 0.2.2: S2 v5 versus v6 confirmation
**As the** maintainer, **I want** the v6 migration size measured, **so that** ADR-001's choice is evidence-based.
**Acceptance Criteria**:
- v6 compile cost is recorded.
  - *Given* a scratch branch importing `github.com/go-git/go-git/v6 v6.0.0-beta.1` and `go-billy/v6`, *When* `go build ./...` runs, *Then* the number of files needing edits and compile errors is recorded (expect at least 30 importing files).
**Files**: scratch branch (discarded), `implementation/gates.md`

##### Task 0.2.2a: Mechanical import rewrite and build (~5 min)
- Rewrite imports with `gofmt -r`/sed on a throwaway branch, build, count errors.
- Files: scratch

**Gate G2**: Default stays v5 (ADR-001). Switch to v6 only if v6.0.0 stable is out and the measured migration is under 1 day of work and `x/plumbing/worktree` would delete native worktree code. Otherwise ADR-001 is confirmed.

### Story 0.2.3: S3 performance versus CLI on the largest managed repo
**As the** maintainer, **I want** go-git and CLI timings per operation, **so that** the "no slower than CLI" SLO is checked before any cohort flips.
**Acceptance Criteria**:
- Benchmark table exists for status, diff, rev-list count, rev-parse HEAD, worktree add (existing branch), commit.
  - *Given* the 27 GB-`.git` repo used in build-vs-buy measurements and a 5 GB-or-smaller repo, *When* `BenchmarkBackend/<op>/{cli,gogit}` runs 30 iterations each, *Then* p50/p95 per op are in `gates.md`; the CLI baseline includes spawn (about 39 to 52 ms for `rev-parse HEAD`, 182 ms for `status --porcelain`, build-vs-buy measurements).
- INFERRED edge behaviours are checked.
  - *Given* a fixture with a rename plus modify, an untracked directory, CRLF files, a shallow clone and an LFS pointer, *When* go-git `Status`, `Log`, `MergeBase`, `Checkout` run, *Then* each divergence from CLI is listed (feeds Epic 2.2 golden tests and the preflight).
**Files**: `session/git/backend/gogit/bench_test.go` (spike, build-tagged `spike`), `implementation/gates.md`

##### Task 0.2.3a: Benchmark harness (~5 min)
- Table-driven benchmark running the CLI via the existing runner and go-git via `OpenRepo`; reuse `worktree_dirty_fast.go` for dirty status as a third column.
- Files: `session/git/backend/gogit/bench_test.go`

##### Task 0.2.3b: Edge-behaviour probe (~5 min)
- Probe: `CommitOptions.Amend` exists, hooks not run, LFS pointer on checkout, shallow `MergeBase`, rename status. Record yes/no.
- Index v3/v4 probe: build real v3 (skip-worktree via sparse checkout, intent-to-add via `git add -N`) and v4 (path-compressed) indexes with the CLI, decode and re-encode with go-git v5.19.2, and compare `git ls-files -s`/`git status --porcelain` before and after. Also confirm a split-index (`link` extension) repo returns `ErrUnknownExtension` (`decoder.go:308`) and maps to `capability_unsupported_index`. Route to CLI only for gaps found (probably sparse-aware status, upstream #2460).
- Files: `session/git/backend/gogit/probe_test.go`

**Gate G3**: Per operation, an op may flip to `gogit` only if its p50 is no worse than CLI p50. Ops that fail stay `cli` (or get a fork perf patch F4 if the gap is in a hot go-git path and the patch is under 200 lines). `status` fails over to the existing `worktree_dirty_fast.go` for boolean dirtiness regardless.

### Story 0.2.4: S4 CLI-compatible locking
**As the** maintainer, **I want** to reproduce lost writes between CLI and go-git and test an operation-scoped lock, **so that** we know whether fork patch F1 is needed.
**Appetite note (Revision 6)**: `localwrite` is stretch, so inside the 3 to 6 weeks only the go/no-go core of S4 runs (failure reproduced; operation-scoped lock evaluated for lost updates; CLI reader never sees a partial index; the F1 question answered for O-6). The remaining criteria below (read-your-writes matrix, census, crash leftovers, abort consistency, no-fallback-in-lock, refs) are the acceptance bar of Story 2.3.1 and run only if gate GL opens the stretch phase. The matrix rows below that include `Remove`, `RemoveGlob`, `Move`, `Reset(Hard)` and `Checkout` are **spike-only feasibility probes**: those operations are CLI-routed (`unsafe_worktree_write`) in production and become promotable only through the fault-injection test.
**Design under test** (replaces the earlier `Storer` decorator, which could not work): go-git's `Worktree.Add`/`Commit`/`Remove` call `Storer.Index()`, mutate in memory, then call `Storer.SetIndex()` separately (`worktree_status.go:134,390`, `worktree_commit.go:62,125`), and the stock `SetIndex` truncates the index in place (`storage/filesystem/index.go`, `dotgit.go:237` `IndexWriter` = `fs.Create`). A lock taken inside `SetIndex` makes the replace atomic but still loses the update (read happened before the lock); a lock taken in `Index()` and released in `SetIndex()` leaks on read-only calls and error paths. So: `WithIndexLock(repo, fn)` takes `<GitDir>/index.lock` with `O_EXCL` **before** invoking `fn` (which performs the whole go-git `Add`/`Commit`), `fn` runs against a scoped `Storer` (wrapping `filesystem.Storage`, passed to `git.Open(storer, fs)`, which is public API): before the first `SetIndex()` its `Index()` reads the real `index`; `SetIndex()` rewrites the held lock file (never `index`); **after the first `SetIndex()`, `Index()` decodes the lock file's current content**, so every later read in the same scope sees the scope's own writes (read-your-writes). Decoding from the file, not caching a pointer, means a caller that mutates a returned `*index.Index` and then errors without calling `SetIndex` cannot corrupt the pending state. On success the lock file is fsynced and renamed over `index`; on error it is removed only if we created it. **Why read-your-writes is required (VERIFIED, go-git v5.19.2)**: `Commit` first runs `autoAddModifiedAndDeleted` when `All` is set (`worktree_commit.go:40-44`), which ends in `SetIndex` (`:109-125`), and only then reads `Storer.Index()` at `:62` to build the tree. A scope whose `Index()` returned the stale on-disk index there would commit a tree that silently omits the auto-added changes while returning success. Read-only operations (`Status`) take no lock. Refs: `<ref>.lock` created O_EXCL, new value written, renamed (go-git's stock `setRefRwfs` locks the ref inode in place while the CLI replaces by rename, so the two do not exclude each other); delete and compare-and-set follow git's order, `<ref>.lock` then `packed-refs.lock` (Story 2.3.1). `GitDir` is the resolved per-worktree dir (`<CommonDir>/worktrees/<name>` for a linked worktree), not `.git`; with `EnableDotGitCommonDir` go-git resolves `index` and `HEAD` to it and `refs`, `packed-refs`, `config`, `logs`, `objects` to `CommonDir` (adversarial re-review, `repository_filesystem.go`).

**Census of every `Index()`/`SetIndex()` sequence in go-git v5.19.2** (VERIFIED by `grep -rn 'Storer.Index()\|SetIndex' --include='*.go'` over the module, excluding tests; outside `worktree*.go` only `submodule.go:61,259,394` read the index and are not used by this repo): reads only: `worktree_status.go:134` (`diffStagingWithWorktree`, reached from `Status`), `:250` (`diffTreeWithStaging`), `worktree_commit.go:62` (`Commit` tree build), `submodule.go`. Read-modify-write pairs: `worktree_status.go:354/390` (`doAdd`: reads at `:354`, then calls `Status` unless `skipStatus` on a plain file, which reads again at `:134`, then one `SetIndex` at `:390`), `:412/441` (`AddGlob`), `:586/603` (`Remove`), `:682/708` (`RemoveGlob`), `:723/741` (`Move`), `worktree.go:375/436` (`resetIndex`, via `Reset` and `Checkout`), `:452/484` (`resetWorktree`, via `Reset(Hard)` and `Checkout`), `worktree_commit.go:109/125` (`autoAddModifiedAndDeleted`). Composite sequences that cross two of these inside one public call: `Commit(All)` (`:109/125` then `:62`), `Commit(All+Amend)`, `Add` (`:354`, nested `:134`, then `:390`), `Checkout`/`Reset(Hard)` (`:375/436` then `:452/484`), `AddWithOptions(All)`, `Pull` (`Checkout`). The set is pinned by a **census test** (Task 0.2.4c) so a go-git or fork upgrade that adds a call site fails CI instead of silently escaping the lock.
**Acceptance Criteria**:
- Failure reproduced.
  - *Given* a repo with a linked `LinkedWorktree` and 20 iterations of concurrent `git add a.txt` (CLI) and stock go-git `Worktree.Add("b.txt")`, *When* both run against the same `index`, *Then* the test shows at least one lost entry (expected per research; if not reproducible, record that).
- Operation-scoped lock evaluated for lost updates.
  - *Given* the same race wrapped in `WithIndexLock`, *When* it runs 200 iterations, *Then* `git ls-files` shows all 400 entries and no stale `.lock` remains; the CLI either waits-and-fails with its own "index.lock exists" message or succeeds, never both writing.
- A CLI reader never sees a partial or corrupt index.
  - *Given* a goroutine looping CLI `git status` and `git ls-files -s` (reader) while go-git writes via `WithIndexLock`, *When* it runs 200 write iterations on a repo with 5,000 index entries, *Then* no reader invocation reports `index file corrupt`, `bad index file sha1 signature` or a truncated entry list (stock go-git in-place truncation is expected to fail this test, which is the reproduction).
- Read-your-writes across every composite sequence in the census (this is what the first S4 draft missed: it tested only `Add`).
  - *Given* a twin pair of fixture repos (one operated by `WithIndexLock` + go-git, one by the CLI) with a modified tracked file, a deleted tracked file, an untracked file and a staged rename, *When* each of these runs inside one `WithIndexLock` scope: `Commit(All)`, `Commit(All+Amend)`, `Commit(Amend)` after an `Add`, `Add` then `Commit` in the same scope, `Remove` then `Status`, `RemoveGlob` then `Commit`, `Move` then `Status`, `Restore(Staged)`, `Reset(Mixed)`, `Reset(Hard)`, `Checkout(Branch)` and `Checkout(Hash)` with a dirty index, `AddWithOptions(All)`, *Then* the resulting commit tree (`git rev-parse HEAD^{tree}`), `git ls-files -s` and `git status --porcelain=v1` equal the CLI twin's (`git commit -a`, `git commit --amend`, `git add -A`, `git reset`, `git checkout`) after normalizing author/time. `Commit(All)` additionally asserts the new commit contains the modified and deleted paths (a stale `Index()` at `worktree_commit.go:62` would drop them).
- Census is pinned.
  - *Given* the go-git module source for the version in `go.mod`, *When* `TestIndexCallSiteCensus` walks it with `go/ast` and collects every call to `Storer.Index`/`Storer.SetIndex` outside tests, *Then* the set of (file, enclosing function) pairs equals the list committed in `gates.md`; any addition or removal fails with the diff. This runs on every fork rebase (runbook 6.3.1).
- Crash-leftover lock handling mirrors git and never blocks agents indefinitely.
  - *Given* a pre-existing `index.lock` not in our journal (fresh, then 10 minutes old), *When* `WithIndexLock` runs, *Then* both return `ErrLocked{Path, Age, Journaled:false}`; a foreign lock is never auto-deleted; a lock we created ourselves is removed on every error path (verified with an injected error between read and rename, and a panic). *Given* a lock created by a server instance that was then `SIGKILL`ed (journal entry present, instance flock released, token on the file, file older than 60 s, no running session in that repository), *When* the next instance starts, *Then* exactly that lock is removed; a lock at the same path **without our token** (a CLI re-created it, whatever its inode), a lock younger than 60 s, a lock in a repository with a running session, and a lock whose instance flock is still held are all reported and untouched (Story 2.3.1 defines the policy and the full outcome table).
- A failed operation leaves ref, index and HEAD mutually consistent (adversarial C-a).
  - *Given* a `Commit(All)` whose scope errors (injected `object_missing`) after the commit object is written and the new ref value is buffered but before the commit phase, *When* the scope aborts, *Then* `refs/heads/<b>`, `HEAD` and `index` equal their pre-operation bytes, no `*.lock` remains, and `Wrote()` is false so the router may run the CLI; *given* the same injection at the first commit-phase rename, *Then* `Wrote()` is already true (it is set **before** the rename attempt, because a failed rename is ambiguous) and no CLI replay occurs. Stock go-git is the reproduction of the problem: `Checkout` calls `setHEADToBranch` (`worktree.go:187-196`) before `Reset`, and `Reset` calls `setHEADCommit` before `resetIndex`/`resetWorktree` (`worktree.go:299-320`), so an error in `resetIndex` leaves HEAD moved and the index old; `Checkout` even moves HEAD before `Reset`'s `containsUnstagedChanges` check, so `ErrUnstagedChanges` is returned **after** HEAD has moved (VERIFIED by reading v5.19.2).
  - *Given* `Checkout(Branch)`, `Checkout(Hash)`, `Reset(Hard)`, `Reset(Merge)` and `Restore(Worktree)` with a write error injected through a billy filesystem decorator at the Nth worktree file write for every N from 1 to the file count, *When* each runs inside the scope, *Then* record whether HEAD, index and worktree equal either the pre-state or the CLI twin's post-state; the fault-injection test **is the promotion criterion** (Story 2.3.2): until it is green for an operation, that operation stays on the CLI (`unsafe_worktree_write`).
- No CLI fallback inside the lock.
  - *Given* an injected `object_missing` between the index read and the ref rename in `Commit`, *When* the router handles it, *Then* the scope aborts and releases `index.lock` first, the CLI `git commit` then runs and succeeds (no "index.lock exists" from our own lock), and the final tree equals the CLI twin's. *Given* the same error injected after the ref rename, *Then* no fallback is attempted and the error is surfaced.
- Refs: set, delete, packed-refs.
  - *Given* a CLI loop running `git pack-refs --all --prune` and another running `git branch -d`/`git branch x` against 50 branches while go-git deletes and creates branches through the ref writer, *When* run 60 s, *Then* no branch ends in an inconsistent state (`git for-each-ref` equals the expected set, `git fsck` clean), no `*.lock` remains, and each side either succeeds or reports the other's lock.
- If public API cannot make this work, the exact missing hook is recorded (becomes F1, defined as an operation-level lock API, not a `SetIndex` hook).
**Files**: `session/git/backend/gogit/lockspike_test.go`, `session/git/backend/gogit/censustest_test.go`, `implementation/gates.md`

##### Task 0.2.4a: Reproduce the lost-update and partial-read races (~5 min)
- Files: `session/git/backend/gogit/lockspike_test.go`

##### Task 0.2.4b: Prototype `WithIndexLock` and the ref-lock writer (~5 min x2)
- Cover `index`, `<ref>.lock`, `packed-refs.lock`, `HEAD.lock`, `config.lock`; start from `native_merge.go:180-230` (existing `<ref>.lock` create-exclusive-write-rename protocol) and `worktree_lock.go`. Reconcile with the existing `repoWorktreeLock` (`session/git/worktree_lock.go:48,86`, mutex plus flock): the flock serializes ssq-internal callers only; `WithIndexLock` is the CLI-compatible lock and is acquired inside it, never the other way round (lock order documented to avoid deadlock).
- Files: `session/git/backend/gogit/lockspike_test.go`

##### Task 0.2.4c: Census test and read-your-writes matrix (~5 min x2)
- Implement `TestIndexCallSiteCensus` and the composite-sequence matrix from the acceptance criteria; commit the census list to `gates.md` (it must include `status.go:122` `preloadStatus`, a read the adversarial re-review found missing from the hand-written list; the same scoped-storer rule covers it). Add the buffered-ref abort test and the worktree-write fault-injection test (every N from 1 to the file count) from the S4 acceptance criteria.
- Files: `session/git/backend/gogit/censustest_test.go`, `session/git/backend/gogit/lockspike_test.go`

**Gate G4**: Pass on all criteria (lost update, partial read, read-your-writes matrix, census, crash-leftover, no-fallback-in-lock, refs) means F1 is out of the fork and the design ships in-repo (Story 2.3.1). If public API cannot reach the index write, F1 goes into the fork as an operation-level lock API (Story 4.2.1). If neither is acceptable, `localwrite` and `worktree` cohorts stay `cli` and `localwrite` is dropped from scope (the user is told).

### Story 0.2.5: S5 credentials and transport
**As the** maintainer, **I want** a working credential-helper client and custom transport, **so that** the network cohort is feasible without forking.
**Acceptance Criteria**:
- Credential protocol client round-trips.
  - *Given* a `CredentialRequest{Protocol:"https", Host:"github.com"}` and the macOS `git-credential-osxkeychain` helper plus `gh auth git-credential`, *When* the client execs the helper directly with `get`, *Then* it returns username and password and never spawns `git` (checked with the spawn shim).
- Real network operation succeeds.
  - *Given* a private test repo on github.com and one on a GitHub Enterprise host, *When* go-git `Fetch` runs with the resulting `transport.AuthMethod` over a custom `http.Client` installed by `client.InstallProtocol`, *Then* both succeed, and a forced cross-host redirect does not forward `Authorization` (asserted with an `httptest` server).
- SSH gaps are listed.
  - *Given* hosts with `ProxyCommand`, `ProxyJump`, `IdentityFile` and `Include` in `~/.ssh/config`, *When* go-git SSH connects, *Then* each unsupported directive is listed for the preflight (ADR-006).
**Files**: `session/git/backend/gogit/credential/credential.go` (spike), `credential_test.go`, `implementation/gates.md`

##### Task 0.2.5a: Credential client prototype (~5 min)
- Files: `session/git/backend/gogit/credential/credential.go`

##### Task 0.2.5b: HTTP transport with redirect policy and test (~5 min)
- Files: `session/git/backend/gogit/credential/transport.go`, `credential_test.go`

##### Task 0.2.5c: SSH config gap probe (~5 min)
- Files: `session/git/backend/gogit/credential/ssh_probe_test.go`

**Gate G5**: Go if the client works for keychain, `gh` and GHE and the redirect test passes. No-go means the `network` cohort stays `cli` permanently for that host class and the plan's "no git" target excludes network ops (ADR-003 carve-out, user told).

### Story 0.2.6: S6 concurrent repack
**As the** maintainer, **I want** to reproduce `object not found` under repack and test a refresh-and-retry wrapper, **so that** we know whether F2 is needed.
**Acceptance Criteria**:
- Failure reproduced and fix evaluated.
  - *Given* a repo with 50 packed commits and a goroutine looping `git repack -ad` while 8 goroutines read `repo.CommitObject(head)` through go-git, *When* run for 60 s, *Then* the count of `plumbing.ErrObjectNotFound` is recorded with stock go-git (upstream #2242), and zero remain after the wrapper opens a **fresh** `*git.Repository` via `OpenRepo` and retries once (`ObjectStorage.Reindex()`, `object.go:74`, is public but not goroutine-safe: it sets `s.index = nil` unlocked, so a shared handle is never reindexed).
**Files**: `session/git/backend/gogit/repackspike_test.go`, `implementation/gates.md`

##### Task 0.2.6a: Reproduce (~5 min)
- Files: `session/git/backend/gogit/repackspike_test.go`

##### Task 0.2.6b: Prototype retry wrapper (~5 min)
- Files: `session/git/backend/gogit/repackspike_test.go`

**Handle lifetime rule (adversarial C5)**: the `gogit` backend opens a fresh `*git.Repository` per call through `OpenRepo` and never caches one across calls (go-git caches the pack index forever per `ObjectStorage`, `object.go:53-71`, and is documented as not thread-safe, upstream #773); it serialises per repo where go-git requires it, via the existing `repoWorktreeLock` (ssq-internal only).

**Gate G6**: Wrapper works means F2 stays in-repo. Needs a storage patch means F2 into the fork (Story 4.2.2). Neither means reads that race repack fall back to CLI by reason `object_not_found`.

### Story 0.2.8: S7 `refs` linked-worktree HEAD failure root cause (needed before `refs` leaves `shadow`)
**As the** maintainer, **I want** the two documented go-git failures on linked worktrees reproduced and root-caused, **so that** the `refs` cohort does not flip on an unexplained failure and the production mitigation is not deleted blind.
`session/git/util.go:326-345` records (1) `repo.Head()` erroring right after a CLI `git worktree add` (deterministic, single-threaded unit test), and (2) in production `repo.Head()` returning a syntactically valid SHA with no object (absent from `git cat-file -t`, `rev-list --all`, reflog, `fsck --unreachable`) and **no error**. The `torn_read` error-class retry (3 x 20 ms) cannot detect (2).
**Acceptance Criteria**:
- Failures reproduced (or the budget spent trying).
  - *Given* a repo and a CLI `git worktree add`/`git update-ref`/`git commit` loop racing go-git `Head()`+`CommitObject()` reads on the linked worktree, *When* run for 60 s with and without `EnableDotGitCommonDir`, *Then* the count of (1) errors and (2) valid-SHA-no-object results is recorded, together with a third class (3) **stale-but-existing SHA**: a read that returns a SHA that exists but is not the ref's current value (a loose ref deleted by a concurrent `git pack-refs --prune`, then read from an old `packed-refs`). Include a `pack-refs --all --prune` racer in the stress loop and a total budget of 3 configurations x 60 s plus one 10-minute run; "cannot be reproduced" is only a valid record after that budget is spent.
- Root cause stated, detection predicate defined, or non-reproduction recorded with its consequences.
  - *Given* the reproduction, *When* analyzed, *Then* `gates.md` records exactly one of three outcomes. **(A) Reproduced and root-caused**: states the cause ("fails because X") and either a fix (in-repo or fork patch) or a **detection predicate**: after any `ResolveRef`/`Head` read the gogit backend verifies the object exists (`repo.Storer.HasEncodedObject`/`CommitObject`) and returns reason `object_missing`, so (2) becomes a detectable error that routes to the CLI. **(B) Not reproducible within the budget**: records the budget and counts; the predicate, the permanent `torn_read`/`object_missing` CLI route and the soak ceiling below are adopted anyway (the production symptom documented at `session/git/util.go:326-345` is treated as real and unexplained). **(C) Reproduced but the predicate cannot detect it** (class 3, or a class-2 case where the SHA's object exists): the affected operations (`ResolveRef`, `CurrentBranch`, `RefExists` on linked worktrees) are marked `cli`-only in the router until a fix exists; the rest of `refs` may proceed.
**Files**: `session/git/backend/gogit/refsspike_test.go`, `implementation/gates.md`

**`torn_read` and `object_missing` are narrow, not catch-alls (adversarial R2).** `torn_read` may be assigned only when the error is in a closed typed set: `plumbing.ErrReferenceNotFound` for a ref whose loose file or `packed-refs` entry exists on a second read within the 3 x 20 ms retry window, `io.ErrUnexpectedEOF`/`io.EOF` from decoding a ref, `packed-refs` or `HEAD` file, or a `*fs.PathError` with `ENOENT` on a loose-ref path that vanishes between `stat` and `open`. Any other go-git error keeps reason `error` (gate-blocking, Story 6.1.1). Both reasons are allow-listed in the zero-spawn gate only for the operations `ResolveRef`, `CurrentBranch`, `RefExists`, `ListRefs`, never for a write or a status/diff operation. Soak ceiling: `(torn_read + object_missing) / refs-cohort calls` must stay below a ceiling that G8 records in `gates.md` (provisional 1%, to be replaced by 2x the steady-state rate S7 measures, minimum 0.1%); the Epic 5.2 soak fails above it.

**Gate G8**: the `refs` cohort may not leave `shadow` until `gates.md` records outcome A, B or C (so a non-reproducible S7 satisfies G8 under B; it neither blocks `refs` forever nor waves it through). **Shadow mode stays mandatory for `refs` under every outcome**: the predicate detects absence only, so a stale-but-existing SHA (class 3) is caught only by a `class="real"` shadow mismatch, and the 7-day zero-real-mismatch window in Story 2.1.3 is never waived. `torn_read` and `object_missing` stay a **permanent** allow-listed CLI route and are **excluded from Epic 6.2 deletion** unless outcome A removed the cause and the soak (Epic 5.2) shows zero occurrences.

### Story 0.2.7: Fork-necessity decision record
**As the** maintainer, **I want** the spike outcomes consolidated into one decision on what the fork carries, **so that** the user can confirm or cancel the fork patches (O-6).
**Acceptance Criteria**:
- `gates.md` lists G1 to G6 outcomes and the final fork patch list.
  - *Given* outcomes G3, G4, G6, G8, *When* the fork-only test is applied to F1, F2, F4, F5 (F3 was removed), *Then* each is marked "fork" or "in-repo" with a link to the failing test that justifies "fork", and the file ends with the explicit question O-6 for Tyler.
**Files**: `project_plans/go-git-fork-full-git-replacement/implementation/gates.md`

##### Task 0.2.7a: Write the gates file and stop for user confirmation (~5 min)
- Files: `project_plans/go-git-fork-full-git-replacement/implementation/gates.md`

**Gate G7a (early user checkpoint, end of week 1, before Epic 1.2 and any Phase 2 work)**: Tyler decides **O-6** (keep an empty pinned fork or drop to stock go-git) with `gates.md` evidence in hand, and confirms the end-state expectation stated at the top of this plan. Epics 1.1 and 1.3 may already be running; they do not depend on the fork.

**Gate G7 (full user checkpoint)**: Tyler confirms O-1 (ADR-003), O-2 (ADR-006), O-4, O-5, O-10. Epics 1.1 and 1.3 may start before G7 on the explicit assumption that the recommended defaults of ADR-003 and ADR-006 hold (the `Router`'s capability routing and carve-outs encode them); if Tyler picks differently, Story 1.1.3 and Story 1.3.4 are revised.

---

# Phase 1: Seam, fork infrastructure, observability

## Epic 1.1: Backend seam and cohort switch (Isolate via seam; ADR-004)
**Goal**: One interface, two implementations, a cohort switch defaulting to CLI, and a lint rule so new CLI call sites cannot appear. The router's capability routing and carve-outs bake in the recommended defaults of ADR-003 and ADR-006 (both NEEDS USER DECISION); revise Stories 1.1.3 and 1.3.4 if Tyler decides otherwise.

### Story 1.1.0: Package topology (verified blocker; do first)
**As the** maintainer, **I want** the package graph fixed before any code moves, **so that** the seam compiles without import cycles and does not drag `session/tmux` or `session/lifecycle` into `pkg/`.
Verified facts (2026-10-08, `go list -deps`): `session/git` depends on `session/tmux` and `session/lifecycle`; `pkg/classifier` depends only on `executor/safeexec`, `log`, `telemetry`, `config/workspacepath`; `session/vcs` does not import `session/git`; the native worktree functions the `gogit` backend must reuse (`nativeListWorktrees`, `nativeRemoveWorktree`, `nativeUnlockWorktree`, `writeNativeWorktreeAdminFiles`, `checkoutNativeWorktree`, `worktreeIsDirtyFast`, `headTreeHashes`) are unexported in `package git`.
Verified native-to-`session/git` dependencies that block a verbatim move (identifier intersection of the 13 non-test `native_*.go` files plus `worktree_dirty_fast.go` against every other non-test `session/git` declaration, then each hit read, 2026-10-08): (1) `OpenRepo`/`defaultPlainOpenOptions` (`util.go:36-45`), called at `native_worktree_common.go:21` and `native_worktree_add.go:68`; (2) the only two `*GitWorktree` methods in the native files, `nativeSetupNewWorktree` (`native_worktree_add.go:67`, reads `g.repoPath`, `g.worktreePath`, `g.branchName`, `g.baseCommitSHA` and writes `g.baseCommitSHA`) and `resolveNativeAddBaseCommit` (`:108`), plus `(*GitWorktree).IsDirtyUncached` at `worktree_dirty_fast.go:281`; (3) `WithRepoWorktreeLock` (`native_merge.go:378`), which lives in `worktree_lock.go` and imports `config` (`config.GetConfigDir`), so it cannot move; (4) `getHeadCommitSHA` (`native_merge.go:481`, `util.go:362`), which has a CLI fallback (`getHeadCommitSHAViaCLI`, `util.go:462`) and so must not move into `native`; (5) `MergeMainResult` (`ops.go:919`, used outside the package as `git.MergeMainResult`, e.g. `session/backlog_lifecycle.go:173`); (6) `gitignoreFSCache`/`newCachedFilesystem` (`gitignore_fs_cache.go:52,83`, used by `worktree_dirty_fast.go:212,256`); (7) `FetchBranch` (`ops.go:27`, called at `native_merge.go:410`), which runs `git fetch origin -- <branch>` through `safeexec`, so a verbatim move would put an unrouted spawn inside `native` that the gate attributes to nothing; (8) `IsDirtyCleanCacheTTL` (`worktree.go:54`, used at `gitignore_fs_cache.go:23`), an exported constant shared with the dirty cache. Items (7) and (8) were found by the architecture re-review 2 scratch compile of the 13 non-test `native_*.go` files plus `worktree_dirty_fast.go` and `gitignore_fs_cache.go` as an isolated package, whose undefined identifiers were exactly `MergeMainResult`, `OpenRepo`, `GitWorktree`, `getHeadCommitSHA`, `WithRepoWorktreeLock`, `IsDirtyCleanCacheTTL`, `FetchBranch` (test files not probed); I re-read both new call sites. The first re-review named only (1) and the `log`/`telemetry` imports.
Package rules (enforced by depguard, Task 1.1.0c):
1. `session/git/backend` is a **leaf**: interface, typed results, `RepoLocation`, `Runner` port, errors, `Cohort`, `BackendMode`, `FallbackReason`, `OperationName`, `Router`. Allowed imports (depguard strict allow-list): the standard library, `go.opentelemetry.io/otel/**`, and the repo leaves `log`, `telemetry` and `session/git/redact` (the router owns the fallback and spawn counters). go-git types are **not** exposed in its API. It never imports `session/git`, `session/tmux`, `session/lifecycle`, `session/git/native`, or `config`.
2. `session/git/native` holds the extracted on-disk worktree/merge/dirty-fast/capability code with exported entry points. **Exact allowed imports (depguard strict allow-list)**: the standard library, `github.com/go-git/**` (this includes `go-billy`), `go.opentelemetry.io/otel/**`, and the three repo leaves `github.com/tstapler/stapler-squad/log`, `.../telemetry`, `.../session/git/redact`. Nothing else, in particular not `config`, `executor/safeexec`, `session/git`, `session/tmux` or `session/lifecycle`. **Why this relaxation and not ports**: the verified current native files import `log` (`native_merge_base.go:39`, `native_worktree_remove.go:30`), `telemetry` and OTel (`native_rollout.go`: `operationDurationMS`, `worktreeRetryTotal`, `withOperationSpan`; `native_merge.go:37`: `mergeOutcomeTotal`), not only `native_rollout.go`; `go list -deps ./log ./telemetry` lists only `config/workspacepath`, `log` and `telemetry` (VERIFIED 2026-10-08), so these leaves cannot form a cycle with `session/*`; and they are process-global singletons whose meters already default to no-op before `telemetry.Initialize`, so a port would add indirection with no test benefit. `native_rollout.go` therefore moves wholesale into `native` (exported `WithOperationSpan`, `RecordWorktreeRetry`), applying `redact.Git` per Story 1.3.1.
3. `session/git/backend/cli` and `session/git/backend/gogit` import `backend` and (`gogit` only) `native`; never `session/git`.
4. `session/git` keeps thin exported wrappers and type aliases over `native` so existing callers do not change (`type MergeMainResult = native.MergeMainResult`, `func OpenRepo(p string) (*git.Repository, error) { return native.OpenRepo(p) }`), and receives a `backend.Backend` by constructor injection; it never constructs one.
5. **Composition root** is `session/gitwiring` (`NewRouter(cohorts backend.CohortMap, local backend.Runner) backend.Backend`; `local` serves `Local` locations only) plus cohort parsing, called once from server startup and the `session` manager. It is the only package importing `backend`, `cli`, `gogit` and `config` together, and **no package under `session/git`, `session/vcs`, `session/vc` or `pkg/` may import it** (depguard deny rule; a fixture file in each importing `gitwiring` must fail `make lint`). `Remote` locations carry their own runner (Story 1.1.5), so the router never needs to construct or look up an SSH runner. `backend.Runner.Run` has the same signature as `tmux.CommandRunner.Run`, so a `tmux.CommandRunner` is passed as a `backend.Runner` with no adapter; the only adapter is the optional `StdoutRunner` wrapper for `LocalRunner` in `gitwiring`. **`Locate` is in `backend`, not `gitwiring`**: `func Locate(r Locatable, host, path string) RepoLocation` with `type Locatable interface { Runner; IsRemote() bool }`; a `tmux.CommandRunner` value satisfies it structurally, so `backend` needs no `tmux` import and `session/git` can build locations without importing `gitwiring` (which would drag `gogit` and `native` into every `session/git` importer).
6. `pkg/classifier` and `session/vcs` take an injected `backend.Backend`, or, if a dependency on `backend` is not wanted in `pkg/`, a narrow port defined locally (`classifier.RefReader`) satisfied by the router. Neither imports `session/git`.
7. Cohort/mode types are defined in `backend`; `config` holds only the raw `GitBackendCohorts map[string]string` field (no import of `backend`); parsing happens in `gitwiring` (single parse-at-boundary owner).
8. Dependencies a moved file needs from the rest of `session/git` are **injected, not imported**: `native.MergeMainIntoWorktree(deps native.MergeDeps, ...)` where `MergeDeps{WorktreeLock func(repoPath string, fn func() error) error; HeadSHA func(path string) (string, error); Fetch func(repoPath, branch string) error}` (wrappers in `session/git` pass `WithRepoWorktreeLock`, `getHeadCommitSHA` and `FetchBranch`; after Epic 3.2 the wiring passes `Backend.Fetch`, so the spawn is routed and counted by the seam; `native` never starts a process); `MergeMainResult` and `GitignoreFSCache` move into `native` and are aliased back; `IsDirtyCleanCacheTTL` moves into `native` as `native.CleanCacheTTL` and `session/git` keeps `const IsDirtyCleanCacheTTL = native.CleanCacheTTL`; `nativeSetupNewWorktree` becomes `native.SetupNewWorktree(p native.SetupParams) (baseSHA string, err error)` and the `*GitWorktree` wrapper stores the returned SHA into `g.baseCommitSHA`; `IsDirtyUncached` stays in `session/git` as a one-line wrapper over `native.IsDirtyFast`.
9. `OpenRepo` lives in `native/open.go` with its `//nolint:norawgitopen this is the wrapper itself` comment; the `norawgitopen` analyzer is nolint-per-call and package-agnostic (`tools/lint/norawgitopen/analyzer.go:47-72`), so only its message text changes (`use native.OpenRepo, or session/git.OpenRepo`). `gogit` and `native` open repositories only through `native.OpenRepo`; nothing duplicates the options.
**Acceptance Criteria**:
- Graph is acyclic and clean.
  - *Given* the new packages, *When* `go list -deps ./session/git/backend` runs, *Then* it lists no `session/git`, `session/tmux`, `session/lifecycle` or `config` package; `go list -deps ./session/git/native ./session/git/backend/gogit ./session/git/backend/cli` lists no package `session/git` (exact path, not its subpackages), `session/tmux`, `session/lifecycle`, `config` or `executor/safeexec`; and `go list -deps ./pkg/classifier` lists none of `session/tmux`, `session/lifecycle`, `session/git`; `go list -deps ./session/git/redact` lists only the standard library; `go list -deps ./session/git ./session/vcs ./session/vc ./pkg/classifier` lists no `session/gitwiring`.
- depguard rules present (strict allow-lists).
  - *Given* a test fixture file in each of `session/git/backend/` and `session/git/native/` importing `session/git`, and another importing `config`, *When* `make lint` runs, *Then* each fails on the depguard rule; and a fixture in `native/` importing `log` and `telemetry` passes (the allowed leaves).
- Behaviour preserved by the move.
  - *Given* the existing `session/git` tests (`native_*_test.go` move with their files), *When* `go test ./session/git/... ./session/...` runs after each file group moves, *Then* pass counts are unchanged, `git diff --stat` shows no removed `func Test`, and `grep -rn 'func OpenRepo' session/git` shows only the one-line alias.
**Files**: `session/git/backend/doc.go`, `session/git/native/*.go`, `session/gitwiring/wiring.go`, `.golangci.yml`, `tools/lint/norawgitopen/analyzer.go`

##### Task 1.1.0a: Extract the native code into `session/git/native` (~5 min per file group; 5 groups)
- After **each** group run `go build ./session/git/native/ ./session/git/` and `go vet` on both (the scratch compile is what found items (7) and (8); the identifier-intersection method alone missed them), and add a `go test -run xxx ./session/git/native/` compile check for the moved test files, which the scratch compile did not probe.
- Group order: (i) `native/open.go` (`OpenRepo`) and `native_rollout.go`; (ii) worktree add/common/list/prune/remove/admin_writer, converting the two `*GitWorktree` methods to functions (rule 8); (iii) `gitignore_fs_cache.go` and `worktree_dirty_fast.go` (`IsDirtyFast`, `HeadTreeHashes`); (iv) `native_merge*.go` with `MergeDeps`; (v) merge-base. Export what `gogit` needs (`ListWorktrees`, `RemoveWorktree`, `UnlockWorktree`, `WriteAdminFiles`, `Checkout`, `IsDirtyFast`, `HeadTreeHashes`, `MergeBase`, `AllocateAdminDirName`, `NewAdminFileWriter`); leave one-line wrappers or aliases in `session/git` for current callers. New native code (e.g. Story 4.1.1) lands here, not in `session/git` (already 13 non-test `native_*` files, 31 including tests).
- Files: `session/git/native/*.go`, `session/git/native_*.go`, `session/git/worktree_dirty_fast.go`, `session/git/gitignore_fs_cache.go`, `session/git/util.go`, `session/git/ops.go`

##### Task 1.1.0b: Composition root (~5 min)
- Files: `session/gitwiring/wiring.go`

##### Task 1.1.0c: depguard rules (~5 min)
- Rules, each a strict allow-list with `files:` scoped per the globbing note in `.golangci.yml:66-74`: `backend/**` allows `$gostd`, `go.opentelemetry.io/otel`, `log`, `telemetry`, `session/git/redact`; `native/**` allows the rule-2 list; `pkg/**` must not import `session/*`. Confirm the installed golangci-lint's depguard supports `list-mode: strict` by the failing-fixture test above; if it does not, use a `deny` list that names `session/git`, `session/tmux`, `session/lifecycle`, `config`, `executor` and keep the `go list -deps` acceptance test as the strict check (run in `make ready`).
- Files: `.golangci.yml`

### Story 1.1.1: Types and config
**As the** maintainer, **I want** typed cohorts and a parsed cohort map, **so that** invalid modes cannot exist at runtime.
**Acceptance Criteria**:
- Config parsing is total and defaulting.
  - *Given* `config.json` with `{"git_backend_cohorts":{"refs":"gogit","network":"bogus"}}`, *When* `gitwiring` parses the raw map, *Then* `CohortMap` has `refs=gogit`, `network=cli` with a WARN naming the key (no panic), and every other cohort is `cli`.
- `shadow` is rejected for write operations.
  - *Given* `{"localwrite":"shadow"}`, *When* the map is parsed, *Then* `localwrite` resolves to `cli` with a WARN; `{"network":"shadow"}` shadows only the read operations of that cohort (`fetch`), never `push` or `clone`.
**Files**: `config/config.go`, `session/git/backend/cohort.go`, `session/gitwiring/parse.go`, `session/gitwiring/parse_test.go`

##### Task 1.1.1a: Add the raw field and sum types (~5 min)
- Field `GitBackendCohorts map[string]string` with `json:"git_backend_cohorts,omitempty"` in `config`; types `Cohort`, `BackendMode`, `ParseCohortMap` live in `backend` and `gitwiring`, not `config`.
- Files: `config/config.go`, `session/git/backend/cohort.go`, `session/gitwiring/parse.go`

##### Task 1.1.1b: Tests incl. env override `STAPLER_SQUAD_GIT_BACKEND=cli` (~5 min)
- Files: `session/gitwiring/parse_test.go`

### Story 1.1.2: Backend interface and CLI implementation
**As the** maintainer, **I want** each CLI operation wrapped once behind typed methods, **so that** callers stop building argv.
**Interface sketch (acceptance artifact; method set is derived from the 0.1.1 histogram)**: every method takes `RepoLocation` first, then newtypes, never `(string, string)` pairs.
```go
type RepoLocation interface{ isRepoLocation() }          // sum type
type Local struct{ Root RepoRoot }                       // Local filesystem repo; Root may be any dir inside it (gogit uses DetectDotGit)
type Remote struct{ Host RemoteHost; Path RemotePath; Runner Runner } // runs through Runner only; nil Runner => ErrNoRemoteRunner
type RepoRoot string; type GitDir string; type CommonDir string
type BranchName string; type RefName string; type CommitSHA string
type OperationName string                                // closed set of method names
// Runner is a subset of tmux.CommandRunner (same Run signature, combined stdout+stderr), so a
// tmux.CommandRunner value satisfies it without an adapter (session/tmux/command_runner.go:52).
type Runner interface{ Run(ctx context.Context, dir, name string, args ...string) ([]byte, error) }
// StdoutRunner is optional: cli prefers it when the runner implements it, because some typed
// parsers (rev-parse) must not see stderr warnings. The gitwiring LocalRunner adapter implements it;
// SSH runners do not and keep today's combined-output behaviour.
type StdoutRunner interface{ RunStdout(ctx context.Context, dir, name string, args ...string) ([]byte, error) }

type Backend interface {
    CurrentBranch(ctx context.Context, loc RepoLocation) (BranchName, error)
    ResolveRef(ctx context.Context, loc RepoLocation, ref RefName) (CommitSHA, error) // ErrUnborn
    RefExists(ctx context.Context, loc RepoLocation, ref RefName) (bool, error)
    RepoRoot(ctx context.Context, loc RepoLocation) (RepoRoot, error) // rev-parse --show-toplevel (backs classifier.RefReader.ShowToplevel)
    GitDir(ctx context.Context, loc RepoLocation) (GitDir, error)      // rev-parse --git-dir (backs classifier.RefReader.GitDir)
    CommonDir(ctx context.Context, loc RepoLocation) (CommonDir, error)
    MergeBase(ctx context.Context, loc RepoLocation, a, b RefName) (CommitSHA, error)
    IsDirty(ctx context.Context, loc RepoLocation, intent Intent) (bool, error)   // Intent: Display | Destructive (Task 0.1.1c)
    Status(ctx context.Context, loc RepoLocation, intent Intent) (StatusResult, error)
    DiffNumstat(ctx context.Context, loc RepoLocation, spec DiffSpec) ([]NumstatRow, error)
    Fetch(ctx context.Context, loc RepoLocation, req FetchRequest) error
    Push(ctx context.Context, loc RepoLocation, req PushRequest) error
    AddWorktreeForExistingBranch(ctx context.Context, loc RepoLocation, path WorktreePath, br BranchName) error
    // ... remaining methods enumerated from the audit; BranchName/RefName/CommitSHA never both plain string.
}
```
`DiffSpec` (which also carries `Intent`), `FetchRequest`, `PushRequest` are structs so the primitive-obsession advisory check configured in `.claude/inspect.json` does not fire on parameter piles.
**Acceptance Criteria**:
- Interface covers every operation in the Story 0.1.1 audit.
  - *Given* the audit's histogram, *When* `go doc ./session/git/backend` runs, *Then* every distinct subcommand family (rev-parse variants, for-each-ref, symbolic-ref, config get/set, remote, merge-base, rev-list count, log, status, diff, add, restore, commit, branch, fetch, push, clone, worktree list/add/remove) maps to one method, and no method has two adjacent same-typed `string` parameters.
- CLI implementation preserves current behaviour.
  - *Given* `Local{RepoRoot("/tmp/r")}` on a repo with no commits, *When* `ResolveRef(HEAD)` runs on the `cli` backend, *Then* it returns `ErrUnborn` (typed), matching today's exit-128 handling.
- CLI implementation executes through the `Runner` port, never `safeexec` directly.
  - *Given* a fake `Runner` recording calls and `Remote{Host:"h", Path:"/p", Runner: fake}`, *When* `CurrentBranch` runs on the `cli` backend, *Then* the fake receives `("git", "rev-parse", "--abbrev-ref", "HEAD")` with dir `/p`, and `safeexec` is not called. For `Local`, `gitwiring` supplies `tmux.LocalRunner{}` (wrapped for `StdoutRunner`), which wraps `safeexec` (so `norawgitcli` still sees one sanctioned site).
- A real `tmux.CommandRunner` satisfies the port.
  - *Given* the compile-time assertions `var _ backend.Runner = tmux.LocalRunner{}` and `var _ backend.Runner = (*tmux.SSHRunner)(nil)` in `session/gitwiring`, *When* `go build ./...` runs, *Then* it compiles with no adapter type. Parsers that need clean stdout use `StdoutRunner` when present and, for runners without it, tolerate `warning:`/`hint:` stderr lines the way `runGitCommand` does today (table test with injected warning noise on `rev-parse`, `symbolic-ref`, `for-each-ref`).
**Files**: `session/git/backend/backend.go`, `session/git/backend/location.go`, `session/git/backend/types.go`, `session/git/backend/cli/cli.go`, `session/git/backend/cli/cli_test.go`, `session/git/backend/errors.go`

##### Task 1.1.2a: Interface, typed results, typed errors (~5 min)
- Files: `session/git/backend/backend.go`, `session/git/backend/location.go`, `session/git/backend/types.go`, `session/git/backend/errors.go`

##### Task 1.1.2b: CLI implementation by moving argv construction (~5 min per file group; 4 groups)
- Move argv construction from `session/git/util.go`, `worktree_git.go` (`runGitCommand`, `worktree.go:392` `commandRunner()`), `session/vc/git_provider.go` `runGit`, `session/vcs/git.go` `run` into `cli`, but only from the layers Story 0.1.3 marked live. The existing `tmux.CommandRunner` seam is preserved as the `Runner` port, not bypassed.
- Files: `session/git/backend/cli/*.go`

### Story 1.1.3: Routed backend with fallback and remote routing
**As the** maintainer, **I want** a router that honours the cohort map and falls back per named reason, **so that** rollback is a config change and remote sessions are never routed in-process.
**Acceptance Criteria**:
- Routing and counted fallback.
  - *Given* `refs=gogit` and a gogit implementation returning `ErrObjectNotFound`, *When* `ResolveRef(HEAD)` runs, *Then* the CLI result is returned and `git_backend_fallback_total{operation="ResolveRef",cohort="refs",reason="object_not_found"}` increments by 1.
- Remote always CLI.
  - *Given* `refs=gogit` and `loc = Remote{Host:"h", Path:"/p", Runner: fakeSSH}`, *When* `CurrentBranch` runs, *Then* the `cli` backend handles it via `fakeSSH`, `fallback_total{reason="remote_host"}` increments, the gogit backend's method is never called, and **no local filesystem access occurs** (asserted with a gogit backend double that fails the test if invoked and a path that does not exist locally). No cohort setting overrides this.
- A remote location with no usable runner is an error, never a local run.
  - *Given* `loc = Remote{Host:"h", Path:"/p", Runner: nil}` and a router built with a counting `local` runner, *When* any method runs, *Then* it returns `ErrNoRemoteRunner` (typed, `errors.Is`), the `local` runner's call count is 0, and the gogit double is not invoked. Likewise for a `Runner` whose `Run` returns a dial error: the error is returned, nothing falls through to `local`.
- No fallback while a write scope is open (ADR-003 amended).
  - *Given* a `localwrite` operation routed to gogit, *When* the router decides routing, *Then* capability preflight, hook/signing checks and per-operation reflog gating are all evaluated **before** the gogit call starts; and *given* gogit returns a fallback-eligible error, *Then* the router retries on the CLI only if the gogit call reported `Wrote == false` (no ref, index or config rename happened and its lock scope is closed; the scope's `Wrote()` in Story 2.3.1), otherwise it returns the error and increments `git_backend_error_total` (no fallback). Tested with fake gogit backends returning `ErrFallbackEligible{Wrote:false}` and `{Wrote:true}`.
- `Wrote` belongs to the backend call, not to one lock scope (adversarial C-a).
  - *Given* a gogit method that opens two scopes (an `Add` scope that commits, then a `Commit` scope that returns a fallback-eligible error with its own `Wrote()==false`), *When* the router sees the error, *Then* the error carries `Wrote:true` (the call-level flag is the OR over every scope and over every worktree write, held in a `backend.CallState` carried in the `context.Context`), no CLI replay of the whole operation happens, and `git_backend_error_total` increments.
- Shadow mode never mutates and reports mismatch.
  - *Given* `diffstatus=shadow` with differing outputs, *When* `Diff` runs, *Then* the CLI output is returned, `shadow_mismatch_total{operation="Diff",class="real"}` is 1, and the mismatch log passes `Redactor`.
- Shadow mismatches are classified racy versus real (adversarial C4).
  - *Given* a shadow call where CLI and go-git results differ, *When* the router re-reads both once against the same HEAD SHA and index file mtime, *Then* a difference that disappears is counted `class="racy"` (not gate-blocking) and one that persists `class="real"` (gate-blocking). Both reads are pinned to the HEAD SHA and index mtime captured before the first read. A `real` mismatch where go-git said clean and the CLI said dirty is counted `class="false_clean"` (never waived, Story 2.2.1). Only a **stable** disagreement counts: if HEAD or the index mtime changed between the first and second read the pair is discarded and re-run once, and a pair that still changes is `racy`. Every `real`/`false_clean` mismatch writes the redacted repro record named in the Observability Plan.
- Destructive intents are asymmetric (pre-mortem P1#3).
  - *Given* `diffstatus=gogit` and `IsDirty(loc, Destructive)` where go-git returns clean, *When* the router handles it, *Then* the CLI runs and its answer is returned (reason `destructive_confirm`, counted, allow-listed as `(IsDirty|Status|DiffNumstat, destructive_confirm)` under the ratchet); *given* go-git returns dirty, *Then* that answer is returned with no CLI spawn; *given* `Intent=Display`, *Then* go-git's answer is returned either way. Tested with a gogit double that returns clean while the CLI twin reports dirty: the destructive call reports dirty, and a `Display` call reports the (wrong) clean, which is what `shadow` exists to catch.
- `localwrite` requires an opted-in repo with no live session (pre-mortem P1#2).
  - *Given* `localwrite=gogit`, a repo root absent from `git_backend_localwrite_repos`, *When* any `localwrite` operation runs, *Then* it routes to the CLI with reason `config`; *given* the repo is listed and an injected `LiveSessionProbe func(repoRoot string) bool` (supplied by the `session` manager, the same predicate as lock-recovery condition (iv)) returns true, *Then* it routes to the CLI with reason `live_session` and the gogit double is never called; *given* listed and no live session, *Then* it runs in-process. The probe is evaluated per call, never cached.
- Reason precedence is deterministic (consistency C10).
  - *Given* a hooked repo running `Remove`, *When* the router decides, *Then* the reason is the first applicable entry of the precedence list in the glossary (`capability_hooks` before `unsafe_worktree_write`); a table test covers every adjacent pair in that list. A diverged or `pull.rebase` `Pull` yields `unsupported_pull_mode`, never `error`.
**Files**: `session/git/backend/routed.go`, `session/git/backend/routed_test.go`

##### Task 1.1.3a: Router and `FallbackReason` enum (~5 min)
- Files: `session/git/backend/routed.go`, `session/git/backend/fallback.go`

##### Task 1.1.3b: Shadow mode for read operations only, racy/real classification (~5 min)
- Files: `session/git/backend/shadow.go`, `session/git/backend/routed_test.go`

### Story 1.1.4: Lint rule forbidding raw git CLI
**As the** maintainer, **I want** an analyzer that blocks new git CLI call sites outside the CLI backend and the sanctioned runner, **so that** migration cannot regress.
**Acceptance Criteria**:
- Analyzer flags a violation, including runner calls.
  - *Given* a new file `server/services/x.go` containing `safeexec.CommandContext(ctx, "git", "status")` or `runner.Run(ctx, dir, "git", "status")` or `g.commandRunner().Run(...)`/`g.runGitCommand(...)`, *When* `make lint-custom` runs, *Then* it fails naming `norawgitcli`; the same calls inside `session/git/backend/cli/` and `session/gitwiring/` (the sanctioned Runner) pass. Existing violations are baselined with `//nolint:norawgitcli // migrating, ticket`; the baseline count equals the Story 0.1.1 audit's constructor-site count (36 `safeexec`/`exec` constructor sites plus the runner-call sites counted by the audit under its own definition, 25 on 2026-10-08), **not** 62, which is the `"git",` literal count including argument tables and test helpers.
**Files**: `tools/lint/norawgitcli/analyzer.go`, `tools/lint/norawgitcli/analyzer_test.go`, `tools/lint/norawgitcli/testdata/`, `Makefile`

##### Task 1.1.4a: Analyzer modelled on `tools/lint/norawgitopen` (~5 min)
- Files: `tools/lint/norawgitcli/analyzer.go`, `tools/lint/norawgitcli/analyzer_test.go`

##### Task 1.1.4b: Wire into `make lint-custom` and baseline existing sites (~5 min)
- `tools/lint` is its own Go module (`tools/lint/go.mod`); baseline from the audit script's counts.
- Files: `Makefile`, `tools/lint/go.mod`

### Story 1.1.5: Production wiring of locations and runners
**As the** maintainer, **I want** every production caller to build a `RepoLocation` from the runner it already owns, **so that** remote sessions reach the SSH runner through the router and the acceptance tests with fakes are matched by real wiring.
Verified facts (2026-10-08, `git grep` and reading each site; the Revision 3 text named the wrong sites): `tmux.NewSSHRunner(` has three non-test call sites, but only **one feeds git**. `server/services/remote_service.go:115` is `TestRemoteConnection` (it only calls `runner.Dial`) and `:579` is `BuildRemoteHealthProber` (it runs `true` through `sshremote`); neither runs git and **neither changes**. The git-bearing path is: `server/services/session_service_create.go:483` builds the `*tmux.SSHRunner`, `:573` wraps it with `session.NewRemoteExecutionTarget`, `session/execution_target.go:119` `RemoteExecutionTarget.Runner()` returns it, and `session/instance_worktree.go` passes it into `git.WithCommandRunner(runner)` at `:152,161,196,265,271` and also calls `runner.Run(ctx, i.ExistingWorktree, "git", "rev-parse", "--abbrev-ref", "HEAD")` directly at `:140`; `session_service_create.go:354,525` call `git.NewRemoteWorktreeOps(runner)`. `session/git/worktree.go:392` `commandRunner()` is per `GitWorktree`; `tmux.CommandRunner.IsRemote()` already distinguishes local from remote (`command_runner.go`). The runner needs the key store, known-hosts store and pool, which live in `server/services` (and `server/services` imports `session`), so nothing below it can rebuild one. Decision: **the caller passes the runner in**, inside `Remote`, built by `backend.Locate` (rule 5, Story 1.1.0). `NewRouter` keeps a single `local` runner for `Local` only.
**Acceptance Criteria**:
- One constructor maps a runner to a location.
  - *Given* `backend.Locate(r backend.Locatable, host, path string) backend.RepoLocation`, *When* `r.IsRemote()` is false, *Then* it returns `Local{Root: path}`; when true it returns `Remote{Host, Path, Runner: r}` and never a `Local`. A table test covers `tmux.LocalRunner{}`, a fake with `IsRemote()==true`, and a nil runner (returns `Local`, matching `commandRunner()`'s default). A compile-time assertion in a `session/gitwiring` or `server` test file (never in `backend`) proves `var _ backend.Locatable = tmux.CommandRunner(nil)`.
- Production remote create path reaches the runner through the router, and it is the same runner value.
  - *Given* the remote session creation path (`session_service_create.go:354,483,525,573`) with a recording fake SSH runner standing in for `tmux.NewSSHRunner`, *When* a remote worktree is created, *Then* the recording runner receives `git worktree add <path> <branch>` with dir `<RepoPath>` (today's argv, `remote_worktree.go:86`) **via `Backend`**, the existing `remote_worktree` tests still pass, with every cohort set to `gogit` the gogit double is never invoked, **and** the runner that reaches `Backend` is `==` (same value, asserted by identity of the recording fake) to the one `RemoteExecutionTarget.Runner()` returns for that session.
- Every git call in `session/instance_worktree.go` goes through the location.
  - *Given* the five `git.WithCommandRunner(runner)` sites (`:152,161,196,265,271`) and the direct `runner.Run(..., "git", "rev-parse", ...)` at `:140`, *When* they are migrated, *Then* each builds its `GitWorktree` from `backend.Locate(runner, host, path)` and `:140` becomes a `Backend.CurrentBranch(loc)` call (or carries `//nolint:norawgitcli // migrating, ticket` counted in the `norawgitcli` baseline, Story 1.1.4, until its cohort lands); `grep -n '"git"' session/instance_worktree.go session/execution_target.go` shows no unbaselined hit.
- The three `NewSSHRunner` construction sites still build their own runners and two of them are untouched.
  - *Given* `go build ./...`, *When* Story 1.1.5 lands, *Then* `git diff --stat` shows no change to `server/services/remote_service.go`, `grep -rn 'NewSSHRunner(' --include='*.go' . | grep -v _test` still lists exactly the three sites (`remote_service.go:115,579`, `session_service_create.go:483`), and no package outside `server/`, `cmd/` and `session/` manager wiring imports `session/gitwiring`.
**Files**: `session/git/backend/locate.go`, `session/git/backend/locate_test.go`, `session/instance_worktree.go`, `session/execution_target.go`, `session/git/worktree.go`, `session/git/remote_worktree.go`, `server/services/session_service_create.go` (`:354,483,525,573` only)

##### Task 1.1.5a: `Locate` and its table test (~3 min)
- Files: `session/git/backend/locate.go`, `session/git/backend/locate_test.go`

##### Task 1.1.5b: Migrate `NewRemoteWorktreeOps(runner)` and `GitWorktree.runner` to hold a `backend.RepoLocation` built by `Locate` (~5 min x2)
- `RemoteWorktreeOps` keeps its `runner` field only until its callers move to `Backend` methods; `AddWorktreeForExistingBranch`/`RemoveWorktree` on a `Remote` run through `cli`. Non-git commands it issues (`test -d` at `remote_worktree.go:82`) keep using the runner directly (outside `norawgitcli`). The `sh -c` script at `:169` runs `git init`, `git rev-parse`, `git add` and `git commit` on the **remote** host inside a shell string; no local `git` starts, so it is a remote-only carve-out that neither `norawgitcli` (argv0 is `sh`) nor the spawn counters see. Record it in `audit.md` as a known blind spot of both layers, not as covered.
- Files: `session/git/remote_worktree.go`, `session/git/worktree.go`

##### Task 1.1.5c: Migrate `session/instance_worktree.go` (five `WithCommandRunner` sites and `:140`), `session/execution_target.go`, and `session_service_create.go:354,483,525,573` to `Locate`; add the production create-path test (~5 min x3)
- `remote_service.go:115,579` need no change (they do not run git).
- Files: `session/instance_worktree.go`, `session/execution_target.go`, `server/services/session_service_create.go`

## Epic 1.2: Fork repository and `replace` wiring (needs G1, G2; ADR-001, ADR-002)
**Goal**: A pinned, CI-verified fork consumed by the app, rollbackable by deleting one line. Runs only after G7a (O-6). If Tyler chooses the hybrid, this epic and Epic 4.2 are dropped and the plan continues on stock go-git.

### Story 1.2.1: Fork repo hygiene
**As the** maintainer, **I want** the fork to track upstream with CI, **so that** rebases are routine.
**Acceptance Criteria**:
- Fork has CI and an upstream remote.
  - *Given* `tstapler/go-git` branch `ssq/v5`, *When* a push happens, *Then* `go test ./...` for upstream's own suite runs in GitHub Actions and `git remote -v` shows `upstream` = `go-git/go-git`.
**Files**: external `.github/workflows/ssq-ci.yml`, `README-SSQ.md` (fork repo)

##### Task 1.2.1a: CI workflow and upstream remote (~5 min)
- Files: external

### Story 1.2.2: Wire `replace` into the app and CI
**As the** maintainer, **I want** the app to build against `v5.19.3-ssq.1`, **so that** the fork is the real dependency.
**Acceptance Criteria**:
- Build, tidy and verify pass in CI.
  - *Given* `replace ... => github.com/tstapler/go-git/v5 v5.19.3-ssq.N` and `GOPRIVATE` set (private per O-4), *When* `make ci` and the goreleaser-check workflow run, *Then* both pass, and `go mod graph` shows one go-git.
**Files**: `go.mod`, `go.sum`, `.github/workflows/build.yml`, `.github/workflows/lint.yml`, `.github/workflows/release.yml`, `.github/workflows/goreleaser-check.yml`, `.github/workflows/mcp-integration.yml`, `.goreleaser.yaml`

##### Task 1.2.2a: go.mod replace; bump to v5.19.3 base only after S1 passed at v5.19.2 (~3 min)
- Files: `go.mod`, `go.sum`

##### Task 1.2.2b: CI credentials and `GOPRIVATE` in each workflow (~5 min per 2-3 workflows)
- Files: `.github/workflows/*.yml`, `.goreleaser.yaml`

### Story 1.2.3: Prove rollback of the fork
**As the** maintainer, **I want** a documented and tested rollback, **so that** a bad fork tag is recoverable in minutes.
**Acceptance Criteria**:
- Removing the line restores stock behaviour, for every fork tag.
  - *Given* the replaced build, *When* the `replace` line is deleted and `go mod tidy && make ci` run, *Then* both pass on upstream `v5.19.3`. This test is part of the acceptance criteria of **every** fork tag: a tag that adds an exported symbol the app calls, without a build-tag stock fallback, fails it.
**Files**: `go.mod`, `docs/how-to/roll-back-go-git-fork.md`

##### Task 1.2.3a: Run and document (~5 min)
- Files: `docs/how-to/roll-back-go-git-fork.md`

## Epic 1.3: Observability, redaction, oracle harness
**Goal**: Measure fallback and spawn, and keep credentials out of telemetry, before any new network or write path ships.

### Story 1.3.1: Redactor
**As the** maintainer, **I want** one redaction function on every log/span/label path, **so that** tokens never reach OTel or logs.
**Acceptance Criteria**:
- URL userinfo, tokens and helper output are stripped.
  - *Given* the error text `fatal: unable to access 'https://x-access-token:ghp_abc123@github.com/o/r.git/'`, *When* passed to `redact.Git`, *Then* the result is `fatal: unable to access 'https://***@github.com/o/r.git/'` and contains no `ghp_`.
- `withOperationSpan` redacts before `RecordError`.
  - *Given* an operation returning the error above, *When* the span is exported by an in-memory exporter, *Then* the recorded exception message has no `ghp_`.
**Files**: `session/git/redact/redact.go`, `session/git/redact/redact_test.go`, `session/git/native/rollout.go` (moved from `session/git/native_rollout.go` in Task 1.1.0a group i; `redact` imports the standard library only so `native` and `backend` may import it)

##### Task 1.3.1a: Redactor and table test incl. `Authorization:` headers (~5 min)
- Files: `session/git/redact/redact.go`, `session/git/redact/redact_test.go`

##### Task 1.3.1b: Apply in `withOperationSpan` and slog helpers (~5 min)
- Files: `session/git/native/rollout.go`

### Story 1.3.2: Spawn counters keyed on operation and reason
**As the** maintainer, **I want** every `git` exec counted by typed operation and fallback reason, plus a label-free backstop, **so that** the zero-spawn metric stays meaningful after the seam exists (a `callsite` label collapses to the `cli` helper after migration and `file:line` changes on every edit).
**Acceptance Criteria**:
- Counter carries operation and reason only.
  - *Given* the router sends `Fetch` to the `cli` backend for reason `capability_ssh_proxy`, *When* the backend spawns, *Then* `git_backend_cli_spawn_total{operation="Fetch",reason="capability_ssh_proxy"}` increments by 1 **per `Runner.Run` of `git`** (a `cli` method that runs two commands counts two; operation and reason travel in the `context.Context` the router passes, via `backend.WithCallInfo`, not derived with `runtime.Caller`) and no label contains a URL or token.
- Backstop catches bypasses, as an inequality.
  - *Given* code calling `safeexec.CommandContext(ctx, "git", "status")` directly, *When* it runs, *Then* `git_cli_spawn_backstop_total{subcommand="status"}` increments but `git_backend_cli_spawn_total` does not, so `sum(backstop) > sum(backend, reason != remote_host)` and the gate fails. *Given* a `Local` call through `LocalRunner`, *Then* both increment by 1 and the gate passes. *Given* a `Remote` call through a fake SSH runner, *Then* only `git_backend_cli_spawn_total{reason="remote_host"}` increments, the left side is unchanged and the gate passes (so remote runs neither misfire the gate nor need to be excluded from it). *Given* the backend counter exceeds the backstop for local reasons, *Then* the wrapper's own unit test fails (it counted a spawn that did not happen).
- `file:line` is test-only.
  - *Given* `SSQ_GIT_SPAWN_DUMP=/tmp/d.log`, *When* tests run, *Then* each spawn's `file:line` and operation are written to the dump; no metric has a `callsite` label.
**Files**: `executor/safeexec/safeexec.go`, `executor/safeexec/spawncount.go`, `executor/safeexec/spawncount_test.go`, `session/git/backend/cli/spawn.go`

##### Task 1.3.2a: Subcommand parser (skip `-C <path>`, `--git-dir`, `-c k=v`) and label-free backstop counter (~5 min)
- Files: `executor/safeexec/spawncount.go`, `executor/safeexec/safeexec.go`

##### Task 1.3.2b: Operation+reason counting `Runner` wrapper (reads `backend.WithCallInfo` from ctx) and test-run dump (~5 min)
- Files: `session/git/backend/cli/spawn.go`, `executor/safeexec/spawncount.go`, `executor/safeexec/spawncount_test.go`

### Story 1.3.3: Oracle and differential test harness
**As the** maintainer, **I want** a reusable helper comparing in-process results to real git, **so that** each cohort has parity tests.
**Acceptance Criteria**:
- Helper checks in-process output with real git.
  - *Given* a repo mutated by the gogit backend, *When* `AssertGitFsckClean` and `AssertWorktreeListMatches` run (build tag `gitoracle`), *Then* `git fsck --strict` exits 0 and `git worktree list --porcelain` matches the in-process list.
**Files**: `testutil/gitoracle/oracle.go`, `testutil/gitoracle/oracle_test.go`

##### Task 1.3.3a: Helpers `AssertGitFsckClean`, `AssertWorktreeListMatches`, `RunBoth(op)` (~5 min)
- Files: `testutil/gitoracle/oracle.go`

##### Task 1.3.3b: `make test-oracle` target and CI job (~5 min)
- Files: `Makefile`, `.github/workflows/build.yml`

### Story 1.3.4: Capability preflight (ADR-006, redesigned)
**As the** maintainer, **I want** repos with unsupported features routed to the CLI, **so that** hooks, signing and LFS behaviour never silently change.

**Scope split (Revision 6, pre-mortem P1#1)**: only **resolver v1** is inside the appetite. The design below ((b) to (c4), the tokenizer contract, pass A/B `hasconfig`, the `FC` set, the oracle matrix) is **resolver v2**, the full git-compatible resolver, and is stretch behind gate GL. Hooks (d), signing (f), expensive facts (g) and the router integration are shared by both.
**Resolver v1 (ships first; conservative superset, route to CLI when unsure)**:
- Reads, in git's file order, system candidates (union), XDG, `~/.gitconfig`, `<CommonDir>/config`, `<GitDir>/config.worktree` when `extensions.worktreeConfig` is set, with a minimal line tokenizer that tracks `[section "sub"]` and `key = value` (no value semantics beyond include paths, booleans spelled `true/yes/on/1`, and presence of a key). It records capabilities by **key presence anywhere in the closure**.
- **Every `include.path` and every `includeIf.<cond>.path` target is read regardless of its condition** (depth 10, missing target skipped, relative to the including file, `~/` expanded). A key reached only through a conditional include therefore counts as present: v1 over-detects (errs toward the CLI) and never evaluates `gitdir:`, `onbranch:`, `hasconfig:` or wildmatch. This is deliberate: the literal rule "any include routes to the CLI" would route **every** repo on the maintainer's machine, because both of his config files contain includes (VERIFIED 2026-10-08: `grep -c 'includeIf\|^\[include' ~/.gitconfig ~/.config/git/config` returns 1 and 2 matching lines), turning every cohort and the shadow window into a no-op.
- Routes to the CLI with `capability_detect_error` (closed sub-reason): an unreadable include target, a tokenizer error, include depth over 10 or a cycle, an unresolvable `commondir`, any of `GIT_DIR`, `GIT_INDEX_FILE`, `GIT_COMMON_DIR`, `GIT_WORK_TREE`, `GIT_CONFIG_PARAMETERS`, `GIT_CONFIG_COUNT`, `GIT_CONFIG_KEY_*`, `GIT_CONFIG_VALUE_*` set in the server's environment, and `core.hooksPath` present anywhere in the closure (treated as hooks for every mutating operation) .
- Routes by capability: any key `commit.gpgsign`, `tag.gpgsign`, `push.gpgSign`, `gpg.format`, `gpg.program` or `user.signingkey` present (any value, a bare key counts: superset) gives `capability_gpgsign` for the operations of Story 1.3.4 (f); `hook.*` or any executable non-`.sample` file in `<CommonDir>/hooks` gives `capability_hooks` for every mutating operation (e); LFS, sparse, shallow, submodule, split index and SSH-proxy detection are unchanged (g).
- **Read cohorts (`refs`, `worktree` residue) do not consume the resolver at all**: rev-parse family, for-each-ref, symbolic-ref, merge-base and the native worktree code are config-insensitive in the properties v1 checks. `diffstatus` consumes it only for filter/attribute/`core.autocrlf`/`core.excludesFile`/`core.fsmonitor`/sparse keys (presence anywhere in the closure routes the repo's `diffstatus` to the CLI only when the key can change status output: `filter.<name>.*` referenced by an attribute, sparse, split index; `core.autocrlf` and `core.excludesFile` are covered by the false-clean oracle corpus, Story 2.2.1, not by routing).
- Promotion to v2 is a measured decision: over the first 24 h of `shadow` and again at the 14-day window the maintainer's instance reports `git_capability_detect_total{outcome}` by capability; **if more than 10% of the repos with a session in the prior 30 days are routed to the CLI only because v1 over-detects (a key reached solely through a conditional include), v2 becomes a GL candidate**; otherwise v2 is not built. The 10% figure is provisional (Tyler may change it at G7).
- v1 acceptance criteria: the capability, hooks, linked-worktree, chmod/edit, `core.hooksPath`, fail-closed (restricted to the v1 list above), LFS-needs-an-attribute, caching and depguard criteria below apply to v1; each criterion that mentions `hasconfig`, wildmatch, `gitdir:`/`onbranch:` evaluation, the tokenizer rows, the `FC1`..`FC11` set or the config oracle matrix is v2-only. v1 adds one criterion: *given* a fixture `HOME` where `~/.gitconfig` includes a file that sets `commit.gpgsign` only inside an `includeIf "gitdir:/elsewhere/"`, *When* `Capabilities` runs for a repo outside that path, *Then* v1 returns `capability_gpgsign` (over-detection, documented, and routes to the CLI), and the `detect_error` ratio stays below 1% on the maintainer's real config chain.

**Design (v2; replaces the `.git/hooks` mtime cache, and the first redesign's use of go-git `ConfigScoped`)**:
(a) **Resolve directories.** `.git` is a *file* in a linked worktree, so resolve the real `GitDir` and `CommonDir` first (read the `gitdir:`/`commondir` files; an unresolvable `commondir` is a fail-closed case).
(b) **Own config resolver, not go-git's.** `session/git/native/gitconfig` computes the `EffectiveConfig` the CLI would see, with these inputs in git's precedence order (lowest to highest): system, XDG global, `~/.gitconfig`, repo `<CommonDir>/config`, per-worktree `<GitDir>/config.worktree` (only when `extensions.worktreeConfig` is true in the repo config), then command scope from `GIT_CONFIG_COUNT`/`GIT_CONFIG_KEY_<n>`/`GIT_CONFIG_VALUE_<n>`. Why not `config.LoadConfig`/`ConfigScoped` (VERIFIED against v5.19.2): `LoadConfig` returns after the **first existing** global file (`config/config.go:183-194`, `return ReadConfig(f)` inside the loop), so on the maintainer's machine `~/.gitconfig` exists and `~/.config/git/config` is never read; `Paths` knows only `/etc/gitconfig` for system scope (`:219-220`), whereas on this macOS machine the system file is `/Library/Developer/CommandLineTools/usr/share/git-core/gitconfig` (`git config --show-scope --list --show-origin`), and go-git has no handling of `include`/`includeIf` or of any `GIT_CONFIG_*` variable (`grep` of non-test source: no hits). Scope details: `GIT_CONFIG_GLOBAL` (when set) replaces both global files, `GIT_CONFIG_SYSTEM` replaces the system file, `GIT_CONFIG_NOSYSTEM=1` drops system scope. System scope is the **union of existing candidates** (`$GIT_CONFIG_SYSTEM`, `/etc/gitconfig`, `/opt/homebrew/etc/gitconfig`, `/usr/local/etc/gitconfig`, the Apple Command Line Tools and Xcode `share/git-core/gitconfig` paths) because the exact file depends on which `git` binary the CLI path would run and we deliberately do not spawn one to ask; a union can only add keys at the lowest precedence, so it errs toward routing to the CLI, never away. **Each file is tokenized by our own parser, `gitconfig/parse.go`; go-git's `plumbing/format/config` decoder is not used anywhere in the routing path.** Reproduced against v5.19.2 (`/tmp` scratch program, 2026-10-08): for `[core] hooksPath=/a`, `[include] path=x`, `[core] hooksPath=/b` the decoder returns one `core` section holding `[/a, /b]` and a separate `include`, so the include position is gone (git's `--show-origin` order is `/a`, x's value, `/b`, effective `/b`); `gpgsign` as a bare key, `gpgsign =` and `gpgsign = ""` all decode to `Value == ""` while git reads the first as `true` and the other two as the empty string (`false` under `--type=bool`); `[Core] hooksPath` and `HooksPath` stay separate options; a backslash-newline inside quotes decodes to `a\n b` where git yields `a b`. **Tokenizer contract** (rows verified against git 2.50.1 with `git config -f f.cfg --list -z`, each becomes an oracle fixture): events in stream order `Section{name lowercased, subsection verbatim, present?}`, `Entry{key lowercased, value, HasValue, line}`; a bare key has `HasValue=false` and means boolean true, `key =` and `key = ""` mean the empty string; section names and keys are case-insensitive, subsections case-sensitive; subsection escapes inside `"..."` are `\\`, `\"` and any other `\x` is `x`; the deprecated `[section.Sub]` form lowercases the subsection; `[a ""]` is a present empty subsection; a header may be followed on the same line by `key = v` or a `;`/`#` comment; values are trimmed, interior whitespace kept, `"` toggles quoting, `;`/`#` start a comment only outside quotes, escapes `\n \t \b \\ \"` are valid and any other backslash escape is an error; a trailing `\` joins the next line verbatim (so `x\` + `  y` is `x  y`) both inside and outside quotes; CRLF line ends and a leading UTF-8 BOM are accepted; a final line without newline is accepted; a key must start with a letter and contain letters, digits and `-`; an unterminated quote, a bad escape or a bad header is a **parse error** (git dies with `bad config line`, so the resolver reports `capability_detect_error`); a key before any section is accepted by git and listed without a section (we record it with an empty section and never match it). Includes are processed **at the event where `include.path`/`includeIf.<cond>.path` is read**, in stream order, depth limit 10.
(c) **Includes and conditions are resolved, not feared.** `include.path` and `includeIf.<cond>.path` are expanded **at the event where the key is read** (git semantics, VERIFIED with `--show-origin`: the included file's entries sit between the entries before and after the directive), relative paths relative to the including file, `~/` expanded, depth limit 10 (git's own limit). A **missing include target is skipped silently, as git does**; it is not an error. "Any include exists" is **not** a fail-closed condition: both of the maintainer's config files contain includes (`~/.gitconfig:4`, `~/.config/git/config:63,74`, VERIFIED by `grep -n include`), so that rule would have routed every operation on the dogfood machine to the CLI. The resolver returns the **include closure** (every file read, with content hashes) plus the extra cache-key inputs named in (g).
  - **(c1) `gitdir:` / `gitdir/i:`** use git-config(1) "Conditional includes" rules (trailing `/` means `/**`, leading `~/` and `./`, a missing `**/` prefix rule) with `WM_PATHNAME` wildmatch (c4). The text matched is the per-worktree git dir: for a linked worktree that is `<CommonDir>/worktrees/<name>`, not the worktree path (VERIFIED with git 2.50.1: `gitdir:<main>/.git/worktrees/**` and `gitdir:<main>/` match from a linked worktree, `gitdir:<worktree path>/` and `gitdir:<main>/.git` do not). That is exactly the stapler-squad case: a repo under `~/WorkProjects/` whose linked worktrees live elsewhere still gets the `gitdir:~/WorkProjects/` include. **Symlinked parents (adversarial C-d)**: git's answer depends on how the process was launched (VERIFIED: a pattern written with the symlinked path matched under logical `$PWD` and `GIT_DIR=<logical>`, and did not under `cd -P`, `env -u PWD git -C` or `cd / && git -C`; a pattern written with the real path matched in all four). Go sets `PWD=Dir` for children (`go doc os/exec.Cmd`, `Dir`), so a CLI spawned through `safeexec` sees the logical path, while other launchers do not. The resolver therefore evaluates every `gitdir:` pattern against **two candidates**, the logical path (the `RepoRoot` string exactly as the caller supplied it, cleaned, symlinks unresolved) and the real path (`filepath.EvalSymlinks`), runs the whole resolution once per candidate, and **if the two resolutions produce different entry lists it fails closed** (`capability_detect_error`, sub-reason `gitdir_symlink_ambiguous`). When the candidates agree (the common case, including the maintainer's `~/WorkProjects`) nothing changes.
  - **(c2) `onbranch:`** matches against the current branch read in-process from `<GitDir>/HEAD` (a symref to `refs/heads/<b>`; a detached or unreadable HEAD makes the condition false, settled by an oracle row). The HEAD content is a cache-key input.
  - **(c3) `hasconfig:remote.*.url:<glob>`**, reproduced against git 2.50.1 and specified from the observed behaviour. Git evaluates it **after** the remote URLs of the whole sequence are known, in two passes:
    1. **URL-collection pass (pass A)**, run lazily and at most once per `Resolve`, the first time the main pass evaluates a `hasconfig:remote.*.url:` condition. It walks the complete scope sequence (system, XDG, `~/.gitconfig`, repo, `config.worktree`, then command scope from `GIT_CONFIG_COUNT/KEY/VALUE`) exactly like the main pass except that every `hasconfig:remote.*.url:` condition is treated as **true without reading its glob** and `gitdir:`/`onbranch:` conditions are evaluated normally. It collects the **value** of every `remote.<name>.url` entry (`<name>` may itself contain dots; `pushurl`, `insteadOf` and `url.<base>.*` are not applied or collected) from entries in files read directly or through plain `include.path`, and from the command scope. **Entries inside a file read through any true `includeIf` (of any kind, and files that file includes) must not define `remote.<name>.url`**: git dies with `fatal: remote URLs cannot be configured in file directly or indirectly included by includeIf.hasconfig:remote.*.url` (VERIFIED, including when the hasconfig condition itself does not match and when the offending file is reached through a `gitdir:` includeIf), so the resolver reports `capability_detect_error` there too (the CLI would also fail, and routing to it surfaces git's real message).
    2. **Main pass (pass B)**: the same walk; a `hasconfig:remote.*.url:<glob>` condition is true iff **any** collected URL matches `<glob>` (c4). URLs defined later in the same file, in the repo config after a global-scope includeIf, or in an unconditionally included file all count (VERIFIED, URL after the directive, URL in repo config for a global-scope directive, URL in a plain include). Any other `hasconfig:<key-pattern>:` is **false** (VERIFIED: `hasconfig:foo.*.bar:x` includes nothing and git exits 0).
    The maintainer's shape is therefore ordinary: an unconditional `[include] path=~/.gitconfig.overlay` whose file holds 10 `includeIf "hasconfig:remote.*.url:..."` entries (VERIFIED count) pointing at one further file that defines no remote URL; pass A reads the overlay and the target once, pass B includes the target only when a remote URL matches. Nothing is a fail-closed case.
  - **(c4) Glob matching** is git's `wildmatch` with `WM_PATHNAME`, ported for the characters `*`, `**`, `?` and literals: `*` and `?` never match `/`; `**` is special only as a whole path component (start of pattern or after `/`, and at end of pattern or before `/`), where `/**/` matches zero or more directories and a trailing `/**` matches the rest including an empty rest; elsewhere `**` behaves as `*`; matching is case-sensitive. VERIFIED rows (pattern, URL, git's answer): `https://h/Foo/**` vs `https://h/Foo/` match, vs `https://h/Foo` no match; `**/repo` vs `repo` match; `https://h/*/repo` vs `https://h/a/b/repo` no match; `https://h/**/repo` vs `https://h/repo` match; `https://h/a**b/repo` vs `https://h/aX/Yb/repo` no match; `https://h/?/repo` vs `https://h///repo` no match; `*` vs `https://h/x` no match, `**` vs the same match; `https://H/**` vs `https://h/x` no match; a keyword spelled `HASCONFIG:` is not recognised. **A glob containing `[` or `\` is not ported and fails closed** (`unsupported_glob`); the maintainer's 10 patterns contain neither (VERIFIED by extracting the glob part and grepping), and the restriction is lifted by porting bracket expressions once oracle rows exist.
  - **(c5) Unknown condition keywords.** Anything that is not `gitdir:`, `gitdir/i:`, `onbranch:` or `hasconfig:` fails closed (`unknown_condition`): git ignores such a condition today, but a newer git may implement it, and the resolver must not silently disagree.
  - **Fail-closed set, exact and complete** (each yields `capability_detect_error` with a closed sub-reason label, nothing else does; the `FC` prefix keeps them distinct from the fork patches `F0` to `F5` of section 0.3): FC1 an include target that exists but cannot be read; FC2 a tokenizer parse error in any file we read (git dies with `bad config line`); FC3 include depth over 10 or a cycle; FC4 `unknown_condition`; FC5 `unsupported_glob`; FC6 a `remote.<n>.url` entry inside a true-`includeIf` file during pass A (git dies); FC7 a `remote.<n>.url` entry **without a value** (bare key) whenever pass A runs (git 2.50.1 **segfaults**, rc 139, VERIFIED with `[remote "bk"] url` plus a `hasconfig` include); FC8 `gitdir_symlink_ambiguous` (c1); FC9 an include path of the form `~user/...` or a relative include path coming from the command scope (INFERRED from git-config(1), not run here: git expands or rejects them in ways the resolver does not reproduce; the oracle matrix adds both rows and may narrow this); FC10 an unresolvable `commondir`; FC11 any of `GIT_DIR`, `GIT_INDEX_FILE`, `GIT_COMMON_DIR`, `GIT_WORK_TREE`, `GIT_CONFIG_PARAMETERS` set in the server's environment (go-git ignores them, the CLI honours them). Assumes git 2.36 or newer, the first release with `hasconfig`; an older CLI ignores `hasconfig`, so the resolver can only add keys relative to it, which errs toward the CLI route.
(d) **Hooks: any executable regular file.** The effective hooks dir is `core.hooksPath` from the `EffectiveConfig` (relative paths relative to the worktree top for commit-time hooks, `~` expanded) else `<CommonDir>/hooks`. A repo has hooks iff that directory contains any entry that is a regular file or symlink to one, is executable by the server's uid (`access(X_OK)` semantics), and whose name does not end in `.sample`. No hook-name list is used (git's list is long and grows: `applypatch-msg`, `pre-applypatch`, `post-applypatch`, `pre-commit`, `pre-merge-commit`, `prepare-commit-msg`, `commit-msg`, `post-commit`, `pre-rebase`, `post-checkout`, `post-merge`, `pre-push`, `pre-receive`, `update`, `proc-receive`, `post-receive`, `post-update`, `reference-transaction`, `push-to-checkout`, `pre-auto-gc`, `post-rewrite`, `sendemail-validate`, `fsmonitor-watchman`, `p4-*`, `post-index-change`). Any `hook.*` key in the `EffectiveConfig` (config-defined hooks, newer git) also counts as hooks. Non-executable files are ignored, as git ignores them. `init.templateDir` is not read: templates are copied into `<CommonDir>/hooks` at `git init`/`clone` time, so the copied files are seen directly.
(e) **Which operations are hook-gated: every mutating `Operation`.** The closed `OperationName` set carries a `Mutating()` flag (the same flag that forbids `shadow`). Any mutating operation, not only `Commit`/`Push`/checkout/`Merge`, is routed to the CLI when hooks are present: `Add`, `Remove`, `Restore`, `ResetMixed`, `BranchRename`, `BranchDelete`, `SetUpstream`, `AddWorktree*`, `Fetch`, `Push`, `Pull`, `Commit`, because `post-index-change`, `reference-transaction`, `post-checkout` and `post-rewrite` can fire on any of them. Hooks and signing are **never cached**: re-evaluated on every mutating call (one `ReadDir` plus `stat`s, microseconds; the `EffectiveConfig` itself is cached by include-closure content hash).
(f) **Signing**: `commit.gpgsign` (and `tag.gpgsign`, `push.gpgSign` for `Push`), `gpg.format`, `gpg.program`, `user.signingkey`, parsed with git's boolean rules (`true`/`yes`/`on`/`1`; a **bare key with no `=` is true**, while `key =` and `key = ""` are the empty string, which `--type=bool` reads as false: VERIFIED with git 2.50.1, and only expressible because the own tokenizer keeps `HasValue`) from the `EffectiveConfig`.
(g) **Expensive facts, cached by the content hash of the whole input closure**: LFS and other filter drivers, detected as: some attribute source assigns `filter=<name>` **and** the `EffectiveConfig` defines `filter.<name>.clean|smudge|process` (a global `git lfs install` defines `filter.lfs.*` everywhere, so config alone must not route; the attribute must reference it). Attribute sources: every tracked `.gitattributes` (found by scanning index entries by basename, read from the worktree), `<CommonDir>/info/attributes`, `core.attributesFile` (default `$XDG_CONFIG_HOME/git/attributes` or `~/.config/git/attributes`), and the system `gitattributes` candidates. Also cached: index-extension probe (split index), sparse, shallow, submodules, SSH config directives. Cache key = resolved `CommonDir` plus content hashes of every file in the resolver's include closure and of each attribute source, plus the index file's trailing SHA-1 for the `.gitattributes` path list, plus the resolver's **non-file inputs**: both `gitdir:` candidate strings (c1), the `<GitDir>/HEAD` content when any `onbranch:` condition was evaluated (c2), and the relevant `GIT_CONFIG_*` environment values; never a directory mtime. (Remote URLs live in closure files, so `hasconfig` results are covered by the file hashes.)
(h) **Fail closed means (v2) exactly FC1 to FC11 in (c) plus I/O errors reading a required file**; each such case returns reason `capability_detect_error` (counter label `detect_error:<sub-reason>` from a closed enum, so a regression names itself) and routes to the CLI. Because a silent 100% `capability_detect_error` would make the cohort a no-op, three assertions on `git_capability_detect_total{outcome="detect_error"} / total < 1%` on the maintainer's instance: (1) **the first dogfood checklist**, evaluated after the first 24 h of `shadow` (not only at the end) with the real `~/.gitconfig`, `~/.config/git/config` and overlay chain, and the checklist also lists any `detect_error` sub-reason with a nonzero count; (2) the soak (Epic 5.2); (3) the 14-day default-flip criterion. Revise the ceiling only with a recorded reason.
**Acceptance Criteria**:
- Each capability is detected and routes the right cohort.
  - *Given* `RepoRoot` fixtures with (a) `.gitattributes` `*.bin filter=lfs`, (b) executable `<CommonDir>/hooks/pre-commit`, (c) `commit.gpgsign=true`, (d) a split-index (`link` extension) repo, *When* `Capabilities(loc)` runs, *Then* it returns `lfs`, `hooks:pre-commit`, `gpgsign`, `unsupported_index` respectively and the router sends `localwrite` (b, c), `worktree`/`diffstatus` (a, d) to `cli` with reasons `capability_hooks`, `capability_gpgsign`, `capability_lfs`, `capability_unsupported_index`. A v3 or v4 index alone does **not** route to the CLI.
- Linked worktrees resolve the right hooks dir.
  - *Given* a linked worktree (its `.git` is a file) whose main repo has an executable `pre-commit`, *When* `Commit` is routed, *Then* reason is `capability_hooks`.
- No staleness on chmod or edit.
  - *Given* a `pre-commit` hook that is non-executable, *When* `Commit` runs in-process, then the hook is `chmod +x`'d, *Then* the very next `Commit` routes to the CLI (no cache hit); likewise for adding a hook file, and for editing a hook (mode unchanged, which never mattered to the stat check).
- The resolver agrees with git on the maintainer's config shape (the case the first redesign got wrong).
  - *Given* a fixture `HOME` where `~/.gitconfig` exists **and** `~/.config/git/config` sets `core.hooksPath=<dir>` only inside `[includeIf "gitdir:<fixture-parent>/"]` (with a second unconditional `[include] path=~/.gitconfig-extra` that sets `commit.gpgsign`), and an executable hook in `<dir>`, *When* `Commit` is routed for a repo and for a linked worktree of a repo under `<fixture-parent>`, *Then* both return `capability_hooks`, and `commit.gpgsign` from the include returns `capability_gpgsign`; the same fixture with the repo outside `<fixture-parent>` returns neither.
  - *Given* each fixture `HOME` in the table below, *When* the resolver output (every key, `HasValue`, value, scope and origin file, in order) is compared with `git config --list --show-origin --show-scope --includes -z` run in the same repo (oracle build tag `gitoracle`; `-z` is what distinguishes a bare key, printed as `key` with no newline-value, from `key =`), *Then* they are equal. Fixtures: global in `~/.gitconfig` only; global in XDG only; both present; `GIT_CONFIG_GLOBAL` pointing elsewhere; `GIT_CONFIG_SYSTEM` and `GIT_CONFIG_NOSYSTEM=1`; `GIT_CONFIG_COUNT=2` with keys; nested includes (3 deep); relative include path; missing include target; `includeIf` `gitdir:`, `gitdir/i:`, `onbranch:` matching and non-matching, from the main checkout and from a linked worktree; `extensions.worktreeConfig=true` with `config.worktree` setting `core.hooksPath`; and the **tokenizer rows** (also run per file as `git config -f f.cfg --list -z`): the same section before and after an `[include]` (`/a`, included value, `/b`, effective `/b`); `gpgsign` bare, `gpgsign =`, `gpgsign = ""`, `gpgsign = false`, `gpgsign = yes` (true, false, false, false, true under `--type=bool`); `[Core]` with `HooksPath`; a quoted and an unquoted backslash continuation; `[a "x\"y\\z\q"]`; `[a.Foo]`; `[a ""]`; `[a] k = 1` on the header line; a `;` and a `#` comment, `;` inside quotes; CRLF; BOM; no final newline; a key before any section; and the error rows (bad escape `a\qb`, unterminated quote, key starting with a digit, `[ a ]` with spaces), where git dies and the resolver must return a parse error (FC2).
  - *Given* the **`hasconfig` fixtures** reproduced in this revision, *When* the resolver and git run on each, *Then* they agree: no remote (nothing included); remote `https://git.corp.example/org/repo` against `https://**.corp.example/**`; scp-style `git@host.example:team/x.git` against `git@host.example:team/**`; `https://a/b.example/x` against `https://*.example/x` (no match, `*` stops at `/`); `pushurl`-only and `url.<base>.insteadOf`-only (no match); a URL supplied only through `GIT_CONFIG_COUNT` (match); a dotted remote name `remote.a.b.url` (match); a URL defined after the directive in the same file, in repo config for a global-scope directive, and in a plain-included file (all match); a nested `hasconfig` include reached from a plain include; `hasconfig:foo.*.bar:x` (nothing included, no error); the glob rows of (c4); and the three **expected-death rows** (a hasconfig-included file defining `remote.x.url`, matching and non-matching; a `gitdir:`-includeIf file defining a remote URL while any `hasconfig` exists) where git exits non-zero and the resolver must return FC6; and the bare `remote.<n>.url` row, where the oracle comparison is skipped (git crashes) and the resolver must return FC7.
  - *Given* the **maintainer's shape** in a fixture `HOME` (generic hostnames, not the real ones): `~/.gitconfig` with `[include] path=~/.gitconfig-proxy`, `~/.config/git/config` with `[includeIf "gitdir:<fixture-parent>/"]` and an unconditional `[include] path=~/.gitconfig.overlay` whose file holds 10 `includeIf "hasconfig:remote.*.url:..."` entries (patterns using only `*` and `**`, as in `https://**.corp.example/**`, `https://**.corp.example:*/**`, `ssh://**.corp.example/**`, `git@host.example:Org/**`) all pointing at one included file that defines no remote URL, *When* `Capabilities` runs for a matching remote, a non-matching remote and a repo with no remote, in the main checkout and in a linked worktree, *Then* none returns `capability_detect_error`, the included file's keys appear exactly when git includes them, and the output equals the oracle's.
  - *Given* a symlinked parent (`<root>/lnk -> <root>/real`, repo at `<root>/real/repo`, a `gitdir:<root>/lnk/` pattern and a second fixture with the real-path pattern), *When* the oracle is run with a logical `PWD`, with `cd -P`, with `env -u PWD git -C`, and with `cd / && git -C`, *Then* the resolver's two-candidate result (c1) equals the oracle in every launch mode where the candidates agree and returns FC8 where they differ; fixtures use `t.TempDir()` paths, which are symlinked on macOS (`/var` to `/private/var`), so the row is exercised there.
- Hook detection is "any executable non-`.sample` file", for every mutating operation.
  - *Given* executable files named `post-index-change`, `post-rewrite`, `pre-merge-commit`, `pre-rebase`, `post-update`, `push-to-checkout`, `reference-transaction`, and a made-up `zz-future-hook`, each alone in `<CommonDir>/hooks`, *When* each of `Add`, `Remove`, `Restore`, `BranchRename`, `BranchDelete`, `SetUpstream`, `Commit`, `Push` is routed, *Then* every combination returns `capability_hooks`; a `pre-commit.sample` and a non-executable `pre-commit` alone return no hooks.
- Include handling fails closed only where justified.
  - *Given* (i) an `[include]` whose target does not exist, (ii) a target that exists with mode `000`, (iii) **an `includeIf "hasconfig:remote.*.url:<glob>"` whose target defines no remote URL (matching and non-matching), which is resolved, not an error** (the Revision 3 criterion pinned the opposite and was the N3 defect), (iv) a 12-deep include chain, (v) an include cycle, (vi) `GIT_INDEX_FILE` set in the environment, (vii) a config that only contains an unconditional `[include]` of an existing, readable, harmless file, (viii) a `hasconfig` glob containing `[`, (ix) an `includeIf "weird:x"`, (x) the tokenizer error rows, (xi) the FC6 and FC7 rows above, *When* `Capabilities` runs, *Then* (i), (iii) and (vii) succeed (no `capability_detect_error`), and (ii), (iv) to (vi) and (viii) to (xi) return `capability_detect_error` with the sub-reason named in (c) and route to the CLI.
- `core.hooksPath` and global config are honoured.
  - *Given* `core.hooksPath=/tmp/hooks` set in `--local`, then separately in `--global` (isolated `HOME`), then separately in `config.worktree`, with an executable hook in that directory and none in `<CommonDir>/hooks`, *When* `Commit` is routed, *Then* all cases return `capability_hooks`; and `commit.gpgsign=true` set only in `--global` returns `capability_gpgsign`.
- Fail closed on resolution errors.
  - *Given* an unreadable repo config file or an unresolvable `commondir`, *When* `Capabilities` runs, *Then* it returns `capability_detect_error` and the router uses the CLI.
- LFS needs an attribute that references a configured driver.
  - *Given* a global `filter.lfs.clean` (as `git lfs install` writes) and a repo whose `.gitattributes` files, `info/attributes` and `core.attributesFile` contain no `filter=lfs`, *When* `Capabilities` runs, *Then* no `lfs` capability is reported; adding `*.bin filter=lfs` to, in turn, the root `.gitattributes`, a nested `sub/.gitattributes`, `<CommonDir>/info/attributes`, and the `core.attributesFile` named in `~/.config/git/config`, each returns `lfs`.
- Expensive facts are cached and invalidated by content, not by mtime of a directory.
  - *Given* a cached LFS result, *When* `.gitattributes` content changes (same size), or a file in the config include closure changes content, *Then* the next call recomputes.
- The maintainer's instance does not silently run on the CLI.
  - *Given* the dogfood instance with the real `~/.gitconfig`, `~/.config/git/config` and the overlay chain (10 `hasconfig` includes), *When* the first 24 h of `shadow` have run (first dogfood checklist), the soak runs, and the 14-day default-flip window ends, *Then* at each of the three points `git_capability_detect_total{outcome="detect_error"}` is below 1% of all `Capabilities` calls, every nonzero `detect_error:<sub-reason>` is listed and explained in the checklist, and the "CLI-routed repos" debug view lists no repo routed only for `detect_error`.
- Routing code cannot reach go-git's config decoder.
  - *Given* a fixture file in `session/git/native/gitconfig/` and one in `session/git/native/capability/` importing `github.com/go-git/go-git/v5/plumbing/format/config` or `github.com/go-git/go-git/v5/config`, *When* `make lint` runs, *Then* each fails on a depguard deny rule; `gitconfig` is stdlib-only (strict allow-list) and `capability` may import `gitconfig`, `redact`, `log` and go-git packages other than those two.
**Files**: `session/git/backend/capability.go` (the `CapabilityProbe` port and `Capability` enum only; `backend` stays go-git-free), `session/git/native/gitconfig/parse.go`, `session/git/native/gitconfig/parse_test.go`, `session/git/native/gitconfig/wildmatch.go`, `session/git/native/gitconfig/wildmatch_test.go`, `session/git/native/gitconfig/resolver.go`, `session/git/native/gitconfig/resolver_test.go`, `session/git/native/gitconfig/oracle_test.go` (tag `gitoracle`), `session/git/native/capability/capability.go`, `session/git/native/capability/capability_test.go` (detectors in their own package so the depguard deny rule is enforceable; read config only through `gitconfig`, never go-git `ConfigScoped`/`LoadConfig`/`plumbing/format/config`), `session/git/backend/routed.go`, `.golangci.yml`

##### Task 1.3.4a0: Tokenizer `parse.go` and its per-file oracle (~5 min x2)
- Implement the tokenizer contract in (b) with no go-git import; table-test every row, then run each row as `git config -f f.cfg --list -z` (tag `gitoracle`) and compare `HasValue`/value/order. Any row where the tokenizer and git disagree is a failing test, not a documented limit.
- Files: `session/git/native/gitconfig/parse.go`, `session/git/native/gitconfig/parse_test.go`

##### Task 1.3.4a1: `wildmatch.go` (`WM_PATHNAME`, `*`, `**`, `?`) with an oracle that drives real `hasconfig` and `gitdir` conditions (~5 min x2)
- Table of at least the (c4) rows plus a generated set (pattern from a small alphabet of `a`, `/`, `*`, `**`, `?`; text from the same alphabet, length at most 6) compared with git by building a repo whose remote URL is the text and whose config has `includeIf "hasconfig:remote.*.url:<pattern>"`, then checking whether the included key appears. Patterns with `[` or a backslash are asserted to return FC5, not compared.
- Files: `session/git/native/gitconfig/wildmatch.go`, `session/git/native/gitconfig/wildmatch_test.go`

##### Task 1.3.4a2: `resolver.go` with scope order, `GIT_CONFIG_*`, include expansion at the directive, `gitdir:`/`gitdir/i:` two-candidate evaluation, `onbranch:`, pass A and pass B for `hasconfig`, depth 10, FC1 to FC11, git boolean parsing, closure and cache-key inputs, and the oracle matrix (~5 min x3)
- Run the differential fixtures against `git config --list --show-origin --show-scope --includes -z`. If `includeIf gitdir:` for a linked worktree disagrees with the (c1) statement, the oracle decides and ADR-006 is amended. Add the maintainer-shape, `hasconfig`, expected-death and symlink fixtures from the acceptance criteria.
- Files: `session/git/native/gitconfig/resolver.go`, `session/git/native/gitconfig/resolver_test.go`, `session/git/native/gitconfig/oracle_test.go`

##### Task 1.3.4a: Resolve GitDir/CommonDir; per-operation hooks and signing checks over the `EffectiveConfig` (~5 min x2)
- Files: `session/git/native/capability/capability.go`

##### Task 1.3.4b: Cached expensive detectors (LFS, index extensions, sparse, shallow, submodules, SSH proxy directives, local-path or `file://` remotes) keyed by content hash (~5 min x2)
- `local_transport` is decided per network operation by the plan's own resolver (Story 3.1.3, over the `gitconfig` `EffectiveConfig`), not by go-git's `insteadOf` (which reads only the repo's own config, `config/config.go:373-376`): apply the resolver to the URL the backend will actually pass to go-git and check `transport.NewEndpoint(<that URL>).Protocol == "file"` for **both** the fetch URL (`URLs[0]`) and the push URL (the last element of `RemoteConfig.URLs`, which lists `url` entries then `pushurl` entries, `config.go:666`; `remote.go:84-86,113-114,416`), plus any direct `RemoteURL` option (Story 3.2.1). Not cached.
- Files: `session/git/native/capability/capability.go`

##### Task 1.3.4c: Integrate into router, fail-closed, counters (~5 min)
- Files: `session/git/backend/routed.go`, `session/git/native/capability/capability_test.go`

---

# Phase 2: Local cohorts (each cohort enters `shadow` first)

## Epic 2.1: `refs` cohort (rev-parse family, for-each-ref, symbolic-ref, config, remote, merge-base, rev-list, log; features.md A1-A15, B9-B13)
**Goal**: Move the largest spawn source in-process. Condition: G0 done, Epic 1.1 done, **G8 (S7) passed before the cohort leaves `shadow`** (O-8 is already answered: `EnableDotGitCommonDir` is set, `session/git/util.go:36`).

### Story 2.1.1: gogit implementation of ref reads
**As the** maintainer, **I want** go-git implementations with CLI-identical edge semantics, **so that** `refs` can flip.
**Acceptance Criteria**:
- Linked-worktree HEAD is the per-worktree one.
  - *Given* a `LinkedWorktree` on branch `feat/x` created from main on `main`, *When* `CurrentBranch(RepoRoot)` runs on gogit, *Then* it returns `feat/x` (not `main`), equals the CLI result, and the open path uses `OpenRepo` (`EnableDotGitCommonDir: true`, fresh handle per call, no cached `*git.Repository`).
- Missing-object HEAD is detected.
  - *Given* the S7 reproduction (or a fixture whose `HEAD` ref names a SHA with no object), *When* `ResolveRef(HEAD)` runs on gogit, *Then* the object-exists predicate from S7 returns reason `object_missing` and the router serves the CLI result; the gogit backend never returns a SHA it has not verified exists.
- Sentinels preserved.
  - *Given* a detached HEAD at `abc1234...`, *When* `CurrentBranch` runs, *Then* it returns the literal `HEAD`; *Given* an unborn repo, *When* `ResolveRef(HEAD)` runs, *Then* it returns `ErrUnborn` for both backends.
- Paths canonicalized.
  - *Given* a repo at `/var/folders/.../r` (symlink to `/private/var/...`), *When* `RepoRoot`/`CommonDir` run, *Then* gogit returns the same realpath as `git rev-parse --path-format=absolute`.
**Files**: `session/git/backend/gogit/refs.go`, `session/git/backend/gogit/refs_test.go`, `session/git/util.go` (calls `backend.Backend`, injected)

##### Task 2.1.1a: `RepoRoot`, `GitDir`, `CommonDir`, `CurrentBranch`, `ResolveRef`, `RefExists`, `IsSymbolic` (~5 min each x3)
- Files: `session/git/backend/gogit/refs.go`

##### Task 2.1.1b: `ListRefs` (for-each-ref prefix), `Config`, `RemoteURL`, `MergeBase`, `RevListCount`, `LogSubjects` (~5 min each x3)
- Map `merge-base` no-result to the CLI's exit-1 error; use native `native_merge_base.go`.
- Files: `session/git/backend/gogit/refs.go`

##### Task 2.1.1c: Differential tests per op through `RunBoth` (~5 min each x3)
- Include unborn, detached, packed refs, deleted-branch-with-worktree, `refs/remotes/origin/HEAD` absent, shallow clone (expect `shadow` mismatch listed).
- Files: `session/git/backend/gogit/refs_test.go`

### Story 2.1.2: Migrate callers to the interface
**As the** maintainer, **I want** each caller to use `Backend`, **so that** the cohort switch has effect.
**Acceptance Criteria**:
- No-`-C` sites now take an explicit `RepoRoot`.
  - *Given* `session/backlog_lifecycle.go:1767` and `server/services/backlog_service_lifecycle.go:658` (`rev-parse --verify`), *When* the server runs with `cwd=/` , *Then* `RefExists(repoRoot, branch)` still works (previously inherited process CWD).
- Spawn drop measured.
  - *Given* `refs=gogit` and the Story 0.1.2 shim, *When* `go test ./session` runs, *Then* `rev-parse`, `for-each-ref`, `symbolic-ref`, `config` spawn counts are 0 and total spawns fall by at least the baseline `rev-parse` count (recorded in `audit.md`).
**Files**: `pkg/classifier/classifier.go`, `session/vcs/detect.go`, `session/repo_path.go`, `session/backlog_commands.go`, `session/git/util.go`, `session/git/ops.go`, `session/git_worktree_manager.go`, `session/instance_worktree.go`, `server/services/search_service.go`, `server/services/workspace_service.go`, `server/services/unfinished_work_service.go`, `server/services/session_service_lifecycle.go`

##### Task 2.1.2a: Migrate `session/git/*` sites (util.go, ops.go) (~5 min)
- Replace `getHeadCommitSHA` retry logic with the cohort retry policy (3 x 20 ms, reason `torn_read`) **plus** the S7 `object_missing` predicate. The existing mitigation (`headSHARetryAttempts`, CLI fallback, `util.go:326-345`) is not deleted until S7 proves the cause is fixed (G8).
- `session/git` receives `backend.Backend` by constructor injection from `session/gitwiring`; it does not build the router (no import cycle).
- Files: `session/git/util.go`, `session/git/ops.go`

##### Task 2.1.2b: Migrate `session/` sites (~5 min x2)
- Files: `session/repo_path.go`, `session/backlog_commands.go`, `session/git_worktree_manager.go`, `session/instance_worktree.go`; only callers of layers Story 0.1.3 marked live (`session/vc`, `session/vcs`)

##### Task 2.1.2c: Migrate `server/services` and `pkg/classifier` sites (~5 min x2)
- `pkg/classifier` takes the narrow locally defined port (`classifier.RefReader`, Story 1.1.0 rule 6), so `session/tmux`, `session/lifecycle` and `backend.RepoLocation` never enter `pkg/`. The two call sites are `pkg/classifier/classifier.go:713` (`git -C <cwd> rev-parse --show-toplevel`, sets `RepoRoot`/`IsGitRepo`) and `:721` (`git -C <cwd> rev-parse --git-dir`, `IsWorktree = strings.Contains(out, "worktrees")`; both currently use `safeexec ... .Output()` with a 5 s timeout, VERIFIED). The port takes plain path strings:
  ```go
  type RefReader interface {
      ShowToplevel(ctx context.Context, dir string) (string, error) // rev-parse --show-toplevel; dir may be any directory, error means "not a repo"
      GitDir(ctx context.Context, dir string) (string, error)       // rev-parse --git-dir; absolute path (per-worktree dir for a linked worktree)
  }
  ```
  `gitwiring` supplies the adapter (`dir` becomes `Local{Root: RepoRoot(dir)}` and the calls become `Backend.RepoRoot`/`Backend.GitDir`, which are in the Story 2.1.1 method set); the 5 s timeout stays in the classifier via `ctx`. Acceptance: *Given* a cwd that is a subdirectory of a linked worktree, *When* the classifier builds its context with the gogit adapter and again with the CLI adapter, *Then* `RepoRoot`, `IsGitRepo` and `IsWorktree` are equal; *given* a non-repo directory, *Then* both report `IsGitRepo=false` and the classifier makes no `git` spawn when `refs=gogit`.
- Files: listed above

##### Task 2.1.2d: Baseline the lint allow-list down (~3 min)
- Files: `tools/lint/norawgitcli/*`

### Story 2.1.3: Shadow soak and flip
**As the** maintainer, **I want** a dogfood window in shadow mode, **so that** the flip is based on zero mismatches.
**Acceptance Criteria**:
- Flip criteria are mechanical.
  - *Given* `refs=shadow` on the maintainer's instance for 7 days **and** at least 500 `git_backend_shadow_calls_total` per operation family (an idle machine cannot pass vacuously; the window extends until both hold), *When* the counters are read, *Then* `git_backend_shadow_mismatch_total{class="real"}` is 0 (else fix, using the redacted repro records, and restart the clock; `class="racy"` mismatches, which disappear on a pinned re-read, are reported but do not block, per Story 1.1.3) and p50 per op is within G3, and G8 has passed; only then is `refs` set to `gogit`.
- The first dogfood checklist checks capability detection on day one (adversarial N3).
  - *Given* `refs=shadow` has run for the first 24 h on the maintainer's instance with the real global, XDG and overlay config chain, *When* the checklist in the runbook is run, *Then* it records `git_capability_detect_total{outcome="detect_error"}` over total (below 1%, Story 1.3.4 (h)), lists every nonzero `detect_error:<sub-reason>`, and if the ratio is at or above the ceiling the window is paused and the sub-reason fixed in the resolver before the 7-day clock continues. A 100% `detect_error` rate would otherwise route every call to the CLI and make the shadow window measure nothing.
**Files**: `implementation/gates.md`, `docs/how-to/flip-git-backend-cohort.md`

##### Task 2.1.3a: Runbook for flipping and reading counters (~5 min)
- Files: `docs/how-to/flip-git-backend-cohort.md`

## Epic 2.2: `diffstatus` cohort (status, diff, numstat; features.md B1-B8; conditional on G3)
**Goal**: In-process status and diff where parity and speed are proven; CLI elsewhere.

### Story 2.2.1: Dirty check and status
**As the** maintainer, **I want** boolean dirtiness and porcelain-equivalent status in-process, **so that** the hottest read stops spawning.
**Acceptance Criteria**:
- Dirty check uses the fast path.
  - *Given* a `LinkedWorktree` with one modified tracked file and CRLF-attribute files, *When* `IsDirty(RepoRoot)` runs on gogit, *Then* it returns true via `worktree_dirty_fast.go` and returns false for a clean worktree even with CRLF files (matching `git status --porcelain` empty), p50 not worse than G3 CLI baseline.
- Porcelain golden parity or CLI routing.
  - *Given* fixtures for rename+modify, untracked directory, deleted file, staged and unstaged same file, *When* `Status` runs on both, *Then* each is byte-identical for the parsed `XY path` list, otherwise the case is listed in `capability`-style `diffstatus_unsupported` and routes to CLI.
- False-clean corpus (pre-mortem P1#3). All of these are INFERRED parity risks (features.md B1), not yet measured.
  - *Given* fixtures with: CRLF files under `core.autocrlf=true` and `input`; a global `core.excludesFile` that ignores a new untracked file, and the same repo with the file not ignored; a tracked symlink whose target changed; an intent-to-add entry (`git add -N`); `assume-unchanged` and `skip-worktree` bits; a submodule with a dirty worktree; a file-mode change with `core.fileMode`; and an untracked file under a nested `.gitignore` negation, *When* `IsDirty`/`Status` run on gogit and the CLI, *Then* the test **fails on any case where gogit says clean and the CLI says dirty**; a gogit-dirty/CLI-clean case is reported but does not fail (the safe direction).
- Destructive intents never trust "clean".
  - *Given* the call sites listed in Task 0.1.1c migrated with `Intent=Destructive`, *When* any of them runs under `diffstatus=gogit`, *Then* a gogit "clean" is confirmed by the CLI (Story 1.1.3), asserted per site with the gogit double returning clean over a dirty fixture.
- The `diffstatus` flip is blocked on zero false-clean (extends Story 2.1.3's criteria).
  - *Given* `diffstatus=shadow`, *When* the flip PR is prepared, *Then* `git_backend_shadow_mismatch_total{class="false_clean"}` is 0 over the shadow window, the window covers **every repository that had a session in the 30 days before it started** (the list recorded in `gates.md`) with at least 20 shadowed `IsDirty`/`Status` calls each (not one repo on one machine), `real` is 0, and `gates.md` records the counter values. Any `false_clean` restarts the clock.
**Files**: `session/git/backend/gogit/status.go`, `session/git/backend/gogit/status_test.go`, `session/git/worktree_dirty_fast.go`

##### Task 2.2.1a: `IsDirty` over the fast path (~5 min)
- Files: `session/git/backend/gogit/status.go`

##### Task 2.2.1b: `Status` with golden fixtures; mismatch cases route to CLI (~5 min x2)
- Files: `session/git/backend/gogit/status_test.go`

### Story 2.2.2: Diff text and numstat
**As the** maintainer, **I want** diffs the UI and the LLM prompts can consume, **so that** B4-B8 stop spawning only where output is equivalent.
**Acceptance Criteria**:
- Numstat matches.
  - *Given* a worktree with 3 modified files, one binary, one rename, *When* `DiffNumstat(cached=false)` runs on both backends, *Then* additions/deletions per file are equal and binary rows are `-`/`-`; mismatches route to CLI.
- Unified diff goldens decide the route.
  - *Given* the same fixtures, *When* `Diff(path)` runs, *Then* if output differs in header/mode/`\ No newline` lines, the op stays `cli` (decision recorded per op); otherwise it flips. Three-dot ranges use `MergeBase` and handle multiple merge bases by picking the CLI's choice.
**Files**: `session/git/backend/gogit/diff.go`, `session/git/backend/gogit/diff_test.go`

##### Task 2.2.2a: Index-vs-worktree and HEAD-vs-index composition via `Status` plus blob diff (~5 min x2)
- Files: `session/git/backend/gogit/diff.go`

##### Task 2.2.2b: Range diff and `merge-base` handling (~5 min)
- Files: `session/git/backend/gogit/diff.go`

##### Task 2.2.2c: Migrate callers (`git_provider.go`, `unfinished/state.go`, `unfinished_work_service.go`, `backlog_review.go`, `diagnostic_service.go`) (~5 min x2)
- Every call passes an explicit `Intent`; the sites listed as `destructive-sites` in `audit.md` (Task 0.1.1c) pass `Destructive`, and a test fails if a listed site passes `Display`.
- Files: those

## Epic 2.3: `localwrite` cohort (STRETCH, outside the 3 to 6 week appetite; add, restore, commit, branch, config set, push -u upstream config; features.md C1-C6; conditional on G3, G4, O-2 and gate GL)
**Goal**: In-process local mutations that interoperate with CLI writers.
**Stretch conditions (Revision 6, pre-mortem P1#1, P1#2)**: this epic starts only when gate GL opens it (section 0.3.1). Even then: (1) `localwrite` is **opt-in per repository** (`git_backend_localwrite_repos`, default empty) and never default-flips in the first release; (2) it applies only when no running session uses the repository (`live_session` routing, Story 1.1.3); (3) every race, partial-read, abort-consistency, `pack-refs`/`branch -D` stress and CLI-commit-in-gap test in Story 2.3.1 is **required CI** on any PR touching `session/git/backend/gogit/`, not soak-only; (4) `Commit(Amend)`, `BranchRename` and ref deletes stay on the CLI until their reflog parity and fault-injection tests are green.

### Story 2.3.1: Operation-scoped CLI-compatible lock layer in-repo (only if G4 passes)
**As the** maintainer, **I want** every in-process repo write to hold the CLI's lock files across its whole read-modify-write, **so that** an agent's concurrent `git add` cannot lose writes and a CLI reader never sees a partial index.
**Acceptance Criteria**:
- No lost writes, no stale locks.
  - *Given* a `LinkedWorktree` and 200 iterations of CLI `git add f<i>` racing gogit `Add("g<i>")` through `WithIndexLock`, *When* the race finishes, *Then* `git ls-files` lists all 400 entries and no `index.lock` remains; and a pre-existing fresh `index.lock` makes gogit return `ErrLocked` (like the CLI) rather than overwrite or delete it.
- CLI reader never sees a partial index.
  - *Given* a CLI `git status`/`git add` loop running while gogit commits 200 times, *When* the loop ends, *Then* no invocation reported a corrupt or truncated index (index written to the lock file then renamed; never `fs.Create` on `index`).
- Error paths do not leak the lock.
  - *Given* an injected error between the index read and the rename, and separately a panic, *When* `WithIndexLock` unwinds, *Then* the lock file we created is removed and `Status` (read-only) never takes the lock.
- Lock order is deadlock-free.
  - *Given* `repoWorktreeLock` (`session/git/worktree_lock.go:86`, mutex plus flock) held by an ssq caller, *When* `WithIndexLock` is taken inside it by 24 concurrent goroutines, *Then* no deadlock occurs (`-race`, 60 s) and the documented order is repoWorktreeLock then index.lock.
- Read-your-writes holds for every **promoted** composite sequence (the S4 matrix, productionised, Revision 6 split per consistency B2).
  - *Given* the production matrix `Commit(All)`, `Commit(All+Amend)`, `Commit(Amend)` after `Add`, `Add` then `Commit`, `Restore(Staged)`, `Reset(Mixed|Soft)` and `AddWithOptions(All)`, *When* it runs through `WithIndexLock` in CI on every PR touching `session/git/backend/gogit/`, *Then* resulting trees equal the CLI twin's, and `TestIndexCallSiteCensus` passes for the go-git version in `go.mod`. `Remove`, `RemoveGlob`, `Move`, `Reset(Hard|Merge)`, `Checkout` and `Restore(Worktree)` are **not** in this matrix: they are CLI-routed (`unsafe_worktree_write`, criterion below) and appear only in the S4 spike probe and in the fault-injection promotion test.
- Required CI, not soak-only (pre-mortem P1#2).
  - *Given* the lost-update race (200 iterations), the CLI-reader partial-index loop, the abort-consistency injection, the `pack-refs --all --prune` plus `branch -D` stress test and the CLI-commit-in-the-gap test, *When* a PR touches `session/git/backend/gogit/`, `session/git/backend/routed.go` or the lock journal, *Then* all of them run in the PR's required CI job (time-boxed to 5 minutes by lowering iteration counts; the full counts stay in the nightly soak) and a failure blocks merge.
- The scope never falls back to the CLI while open; the decision is made before the lock.
  - *Given* a mutating operation, *When* `WithIndexLock` returns, *Then* the lock file is already released (renamed or removed) before any caller code can spawn `git`; the scope exposes `Wrote()`, **set before the first commit-phase rename is attempted** (a failed rename is ambiguous: the target may or may not have changed) and, for operations that write worktree files, before the first worktree write (via a billy filesystem decorator); written objects are inert and do not count. `Wrote()` is recorded in the call-level `backend.CallState` (Story 1.1.3), so it is the OR over every scope and worktree write of one backend call: an `Add` scope that committed followed by a `Commit` scope that errors reports `Wrote()==true` for the call. A fallback-eligible error with `Wrote()==false` aborts the scope and lets the router run the CLI afterwards; with `Wrote()==true` the error is returned unchanged. Capability, hook, signing, reflog-parity and object-existence checks all run before the scope opens (Story 1.1.3). Injecting `object_missing` before the ref rename in `Commit` yields a successful CLI commit with no "index.lock exists"; injecting it after yields the error and no second attempt.
- Refs follow git's lock order, including delete and compare-and-set.
  - *Given* ref operations through the ref writer, *When* it sets, deletes or compare-and-sets a ref, *Then* it takes `<ref>.lock` (create O_EXCL) first, verifies the expected old value **under the lock** (not before it), and for deletion then takes `packed-refs.lock`, rewrites `packed-refs` into the lock file without the ref, renames it, unlinks the loose ref, and releases in reverse order; per-worktree refs (`HEAD`, `refs/bisect/*`, `refs/worktree/*`, `refs/rewritten/*`) lock under `GitDir`, all others under `CommonDir`. go-git's `RemoveReference`, `CheckAndSetReference` and `PackRefs` are never called directly. The Task 0.2.4 `pack-refs --all --prune` and `branch -d` race passes against this implementation.
- The commit phase has a fixed order and an abort before it is atomic across refs and index (adversarial C-a). "Atomic" covers refs and index only, never worktree files; any operation that deletes, renames or writes worktree files is not in this list (Re-review 3 R3-2).
  - *Given* a `Commit(All)`, `Add`, `Restore(Staged)`, `Reset(Mixed|Soft)`, `BranchRename` or branch create/delete, *When* the scope runs, *Then* go-git's ref writes (`SetReference`, `CheckAndSetReference`, `RemoveReference`) land in the scoped storer's **buffer** behind held `<ref>.lock` files (created `O_EXCL` on first write, content written and fsynced, reads of that ref inside the scope served from the buffer), and the scope end runs: fsync all lock files; set `Wrote()`; rename the index lock; for each buffered ref append its reflog line (Story 2.3.1 reflog criterion) then rename its lock, branch refs first and `HEAD` last, `packed-refs.lock` renames last for deletes. **Index before ref** because a crash between the two then leaves the staged-new, HEAD-old state `git add` produces, instead of HEAD-new with an old index, which would present the commit's own changes as staged reversions. This is **deliberately the reverse of the CLI**, which moves the ref first and renames the index last (VERIFIED in Re-review 3 with a `reference-transaction` hook on `git commit -a`: `index.lock` still exists and the index is still old at `prepared` and `committed`); ADR-003 names the transient window and why it is accepted. Test: a CLI `git commit` started inside that window (held with a test hook between the two renames) fails cleanly on the ref lock or the old-value check and leaves no lock file. An error anywhere before the first rename removes every lock file this scope created and leaves every ref, HEAD and the index byte-identical to the pre-state (asserted by the S4 injection above). Lock acquisition never blocks indefinitely, but it does wait while holding locks: a ref-lock retry (`core.filesRefLockTimeout` 100 ms) and a `packed-refs` retry (`core.packedRefsTimeout` 1000 ms; INFERRED from git-config(1), not run here) run while `index.lock` and earlier ref locks are already held, as git itself does (VERIFIED in Re-review 3: with `refs/heads/<b>.lock` held, `git commit -a` exits 128 with `cannot lock ref 'HEAD'` and leaves no `index.lock`). `index.lock` has no retry (git has none). Every wait is bounded and the scope drops all its locks on any failure, so lock-order deadlock is impossible; the cost is spurious failures against concurrent `git gc --auto`/`pack-refs`, which the router may replay on the CLI when `Wrote()==false`. Stress test: ssq `Commit` against a looping `git pack-refs --all` and a looping `git branch -D`/create. **Worktree-writing operations** (`Checkout`, `Reset(Hard|Merge)`, `Restore(Worktree)`, the checkout inside `Pull`, and `Remove`, `RemoveGlob` and `Move`) cannot be buffered, so they stay on the CLI (`unsafe_worktree_write`) until the fault-injection test of Task 0.2.4c is green for that operation. `Remove`/`Move` are on this list because go-git v5.19.2 deletes or renames the worktree file before `SetIndex` (`worktree_status.go:584-604,651-657`; `Move` at `:713-742` renames the file, then writes the index), and `RemoveGlob`/directory removal delete several files in a loop, so an abort would discard the index change after the file is already gone. The alternative (index and ref renames first, then file deletes, with a defined failure state) is not planned.
- Stale locks left by a killed server are recovered only when it is provably safe, and a lock we did not create is never deleted.
  - *Given* the policy below, *When* the tests `SIGKILL` a child server mid-`Commit` (index lock held, token on the file, journal written), wait out the 60 s age rule on a fake clock injected into the recovery function, restart it with no running session in that repository, and run `git add` in that worktree, *Then* startup removes exactly that lock within the startup sequence, increments `git_lock_stale_recovered_total{outcome="removed_own"}`, and the CLI `git add` succeeds. Each of the following leaves the lock in place and increments the named outcome: a stray `index.lock` with no journal record or no token (`reported_foreign` or `reported_untokened`, with path and age in the `ErrLocked` text); a lock a CLI re-created at the same path after our crash, **including when it has the same inode number** (no token, `reported_untokened`; the test forces inode reuse by creating and unlinking in a loop until the number repeats or reports that this filesystem did not repeat it); a lock whose token matches but is younger than 60 s (`reported_too_young`); a lock in a repository where the server's session registry shows a running (non-paused) session (`reported_live_session`); a lock whose owning instance still holds its flock (`reported_owner_alive`); and a filesystem where `setxattr` returns `ENOTSUP` (`recovery_unsupported_fs`, the lock is never recoverable and is reported).
  - *Given* a journal directory that is missing (deleted, or the instance directory changed through `STAPLER_SQUAD_INSTANCE` or a workspace-mode `SwitchDatabase`), *When* startup recovery runs, *Then* it scans **every** journal directory it can enumerate (the default config dir's `locks/`, `instances/*/locks/`, `workspaces/*/locks/`, all under the same OS user) and each journal's own instance flock files, so a restart under a different instance still sees the old journal; locks whose journal is gone are reported `reported_untokened`/`reported_foreign` and listed once at startup in one WARN line naming the repositories (the unrecovered case is explicit, not silent).
- Reflog parity (go-git writes none).
  - *Given* each ref update the ref writer performs (new commit, `Commit(Amend)`, `BranchRename`, `Reset`, branch create, checkout of a branch), *When* `git reflog show HEAD` and `git reflog show <branch>` are compared with the CLI twin's after normalizing hashes and timestamps, *Then* the entry count and messages match (`commit: <subject>`, `commit (initial): ...`, `commit (amend): ...`, `Branch: renamed old to new`, `reset: moving to ...`, `checkout: moving from a to b`), written to `<GitDir>/logs/HEAD` and `<CommonDir>/logs/refs/heads/<branch>` per git's rules and honouring `core.logAllRefUpdates` from the `EffectiveConfig` (unset means true for a non-bare repo, `false` and `always` as git defines). An operation whose reflog parity test is not green stays routed to the CLI (router-level per-operation gate, reason `error` is not used; it is simply `cli` in the cohort table) until it is.
**Files**: `session/git/backend/gogit/clilock.go`, `session/git/backend/gogit/clilock_test.go`, `session/git/backend/gogit/refwriter.go`, `session/git/backend/gogit/lockjournal.go`, `session/git/backend/gogit/lockjournal_test.go`, `session/git/backend/gogit/reflog.go`

**Lock-journal and stale-lock policy (defined here, referenced by ADR-003).** (1) Each server process generates an `instance_id` at start and holds an exclusive `flock` on `<config dir>/locks/instance.<id>.lock` for its lifetime; the kernel releases it on any death, so liveness checks are immune to PID reuse (the repo already uses `gofrs/flock`, `session/git/worktree_lock.go:13`). (2) **Identity is a token, not an inode.** The adversarial re-review 2 observed inode reuse after delete and re-create; Not reproduced on APFS with 1 s gaps (VERIFIED: three cycles gave three different inode numbers; the no-gap variant was not run, so reuse is INFERRED possible), so the design assumes reuse can happen (Linux filesystems commonly reuse inode numbers) and never relies on `(dev, inode)`; they stay in the record only as a debugging hint. Creating any CLI-compatible lock (`index.lock`, `<ref>.lock`, `packed-refs.lock`, `HEAD.lock`, `config.lock`) is a three-step sequence: (a) append and fsync an **intent** record `{path, token, instance_id, created_at}` to the journal, where `token` is 128 random bits; if the append fails the operation fails **before any lock exists** with `lock_unavailable` (the router runs the CLI, `Wrote()==false`); (b) create a temp file `<lock>.<rand>` in the same directory and set the token on it as the extended attribute `user.ssq.lock-token` (`xattr` on macOS, `fsetxattr` on Linux); (c) `link(2)` the temp file to the lock name, which fails with `EEXIST` exactly as `O_EXCL` would, then `unlink` the temp name. The token is therefore present from the instant the lock exists, with no tokenless window (Re-review 3 R3-3, VERIFIED on APFS: a second `ln t1 index.lock` fails with `File exists`, the hard link shares the xattr, and the xattr survives `mv`). On Linux `O_TMPFILE` plus `linkat` is the equivalent. A crash between (b) and (c) leaves a `*.lock.<rand>` temp file; the startup scan reports such names (never deletes them automatically unless the journal intent matches and the five conditions below hold). After our rename the token xattr stays on the real `index` or ref file; it is harmless (the CLI's next rename replaces the inode) and a token on a non-lock file is never treated as a lock. The index lock cannot carry the token in its content (it becomes the index), which is why the token is an attribute. If (c) fails with `EEXIST` (the CLI holds the lock) the failure path removes the temp file and appends an **abandoned** record, so the next startup never reports a CLI-held lock as ours. On release the process renames or unlinks the lock **first** and appends the **released** record **after** (Revision 6, pre-mortem P1#2(d); the earlier order left a crash window in which a lock was neither recovered nor reported). The failure direction is still a reported lock, never a deleted one, because recovery removes only a lock that carries our token (condition (ii) below): if we crash after the rename and before the record, startup finds an intent with no released record, and either the path no longer exists (the record is dropped silently) or a CLI has since created a new lock there, which has no token and is reported `reported_untokened`, never removed. **A lock whose creation attempt is `abandoned`, or whose newest record is `released`, is never reported at startup.** The `ErrLocked{Journaled:true}` crash gap of the earlier order no longer exists; the soak still counts `ErrLocked{Journaled:true}` occurrences. (3) At startup, for each journal record with an intent and no released record, a lock is **auto-removed only if all five hold**: (i) the record's `instance_id` flock can be acquired non-blocking (owner dead); (ii) the file at `path` carries a token equal to the record's; (iii) its mtime is at least 60 s old (a CLI agent holds `index.lock` for milliseconds to seconds; a fresh lock is never ours to remove); (iv) the server's session registry shows **no running (non-paused) session whose repository root or worktree is the lock's repository** ("never delete a lock in a worktree with a live agent"); (v) `setxattr`/`getxattr` are supported on that filesystem. Anything else is reported with the outcome label that names the failed condition and the record is kept for the next startup (or dropped after reporting once when the path no longer exists). The registry check covers agents this server started; an IDE or shell git process is covered by (ii) and (iii), because such a process creates its lock without our token. (4) Foreign locks are never auto-deleted regardless of age (git's own rule; the 10-minute-old fixture in S4 stays `ErrLocked`), but `ErrLocked` carries `Path`, `Age`, the outcome and a one-line operator instruction. **Recovery runs only at startup**, never on contention, so a live agent's lock can never be deleted by a failed `WithIndexLock`. (5) `SIGTERM`/`SIGINT` (including `make install-service` restarts) cancel the context so the scope's deferred cleanup runs; the journal exists for `SIGKILL`, OOM and power loss, and the restart-sensitivity named in `docs/explanation/tmux-keep-server-on-restart.md` is covered by test, not assumed.

##### Task 2.3.1a: `WithIndexLock` with the pending-index and buffered-ref storer, call-level `Wrote()` (`backend.CallState`) and the fixed commit phase; `refwriter.go` (set, delete, CAS, `packed-refs.lock`); `HEAD.lock`/`config.lock` writers; billy decorator that flips `Wrote()` on the first worktree write (~5 min x3)
- Also fold `session/git/scaffolding.go:56,79` (an existing unlocked `Index()`/`SetIndex()` pair) into `WithIndexLock`.
- Files: `session/git/backend/gogit/clilock.go`, `session/git/backend/gogit/refwriter.go`, `session/git/scaffolding.go`

##### Task 2.3.1b: Race, partial-read, read-your-writes matrix, no-fallback-in-lock, error-path and refs tests (~5 min x3)
- Files: `session/git/backend/gogit/clilock_test.go`

##### Task 2.3.1c: Lock journal (intent/held/released), token xattr, per-instance flock, multi-directory startup scan, five-condition recovery, `SIGKILL` test, inode-reuse and `ENOTSUP` fixtures (~5 min x3)
- The session-registry check (condition iv) is an injected `func(repoRoot string) bool` supplied by the `session` manager, so `gogit` does not import `session`.
- Files: `session/git/backend/gogit/lockjournal.go`, `session/git/backend/gogit/lockjournal_test.go`, `server/server.go` (startup hook)

##### Task 2.3.1d: Reflog writer inside the ref writer and the parity tests (~5 min x2)
- Files: `session/git/backend/gogit/reflog.go`

##### Task 2.3.1e: macOS plus Linux CI matrix for xattr lock recovery (validation G-5) (~5 min)
- A required CI job runs the lock-journal recovery tests (token xattr round trip, `link()` creation, `ENOTSUP` handling, the `SIGKILL` recovery test, the inode-reuse probe) on `macos-latest` (APFS) and `ubuntu-latest` (ext4/overlayfs), plus one run on a `tmpfs` directory (`TMPDIR` on `/dev/shm`) expecting `recovery_unsupported_fs` on kernels without `user.*` tmpfs xattrs. Behaviour that differs by filesystem fails the job instead of being skipped; an expected-`ENOTSUP` outcome is asserted, not tolerated.
- Files: `.github/workflows/build.yml`, `session/git/backend/gogit/lockjournal_test.go`

### Story 2.3.2: Commit, add, restore, branch rename, upstream config
**As the** maintainer, **I want** these mutations in-process when safe, **so that** the review gate and commit flow stop spawning.
**Acceptance Criteria**:
- Hook/signing preflight is honoured (evaluated per call, Story 1.3.4).
  - *Given* a repo with an executable `pre-commit` hook, *When* `Commit` runs under `localwrite=gogit`, *Then* the CLI path runs (reason `capability_hooks`) and the hook executes; *Given* no hooks and `commit.gpgsign=false`, *When* `Commit` runs, *Then* it runs in-process with the author from config and `git log -1 --format=%an` (oracle) equals the configured name.
- `push -u` writes upstream config.
  - *Given* `SetUpstream(branch="feat/x", remote="origin")`, *When* run, *Then* `git config branch.feat/x.remote` returns `origin` and `branch.feat/x.merge` returns `refs/heads/feat/x`.
- Amend and rename, gated on reflog parity.
  - *Given* a commit and `branch -m old new` in a worktree, *When* run in-process, *Then* HEAD of that worktree points at `refs/heads/new`, `branch.old.*` config moved, `git fsck` is clean, **and** `git reflog` output equals the CLI twin's (Story 2.3.1 reflog criterion). `Commit(Amend)`, `Reset` and `BranchRename` ship in-process only once their reflog parity test is green; until then the router keeps them on the CLI (go-git writes no reflog at all, VERIFIED: `grep -ril reflog` over non-test go-git v5.19.2 source matches only `internal/revision/parser.go`), so losing `git reflog` recovery after an amend is never a silent regression.
- `Commit(All)` builds from the up-to-date index.
  - *Given* a worktree with a modified and a deleted tracked file, *When* `Commit(All)` runs in-process, *Then* the commit's tree equals `git commit -a`'s (guards the `worktree_commit.go:62` read-after-`SetIndex` sequence, Task 0.2.4c).
**Files**: `session/git/backend/gogit/write.go`, `session/git/backend/gogit/write_test.go`, `session/vc/git_provider.go`, `session/git/worktree_git.go` (`testutil/gitfixture/identity.go` is owned by Story 5.1.1; this story only consumes it)

##### Task 2.3.2a: `Add`, `Restore(Staged)`, `ResetMixed`, each inside `WithIndexLock` (~5 min x2)
- `Checkout`, `Reset(Hard|Merge)`, `Restore(Worktree)`, `Remove`, `RemoveGlob` and `Move` are **not** in this task: they write worktree files and stay on the CLI (`unsafe_worktree_write`) until the Task 0.2.4c fault-injection test is green for the operation, which promotes it in a separate PR. Live uses to keep routed: `session/vcs/git.go:243-310` (VERIFIED 2026-10-08 by reading: `checkout -b`, `checkout <target>`, `stash pop` and `checkout .` write worktree files; `reset HEAD` there is index-only). The earlier citation of `session/vc/git_provider.go:563,568` was wrong: those lines are `restore --staged` and `reset HEAD`, which are index-only (`Restore(Staged)`, `Reset(Mixed)`) and are promotable operations, not worktree writers. Allow-list entries for them are `(operation, unsafe_worktree_write)` pairs under the ratchet (Story 5.3.1).
- Files: `session/git/backend/gogit/write.go`

##### Task 2.3.2b: `Commit` (incl. amend; index lock held through index read, tree write, ref update), `BranchRename`, `SetUpstream` (~5 min x2)
- Files: `session/git/backend/gogit/write.go`

##### Task 2.3.2c: Migrate callers and the oracle tests (~5 min x2)
- Files: listed above

---

# Phase 3: Network (conditional on G5)

## Epic 3.1: Credential provider and transport (ADR-005; STRETCH, outside the appetite, gate GL)
**Goal**: Reproduce enough of `git credential` to fetch/push/clone to github.com and GHE in-process.

### Story 3.1.1: Credential provider chain
**As the** maintainer, **I want** host-scoped credentials from keychain, `gh` and helper binaries, **so that** auth matches today's CLI behaviour.
**Acceptance Criteria**:
- Provider order and host scoping.
  - *Given* hosts `github.com` and `ghe.example.com` each with a distinct token served by an injected `TokenSource` fake, *When* `CredentialFor("ghe.example.com")` runs, *Then* it returns the GHE token and never the github.com token; if the `TokenSource` has none it execs the configured `credential.helper` binary directly with `get` and the spawn shim records 0 `git` execs. **The helper-binary fallback is always on, not opt-in** (consistency C9: requirements Constraints require the user's system credential helpers to keep working; ADR-005 is reworded to match). It is decided at G7 with the other ADR defaults.
- The credential package stays a leaf (consistency C8).
  - *Given* the package `session/git/backend/gogit/credential`, *When* `go list -deps` runs on it, *Then* it lists none of `github`, `config`, `executor/safeexec`, `session/git`, `session/tmux`, `session/lifecycle` (VERIFIED 2026-10-08 that `go list -deps ./github` lists `config`, `executor/safeexec`, `session/git`, `session/tmux` and `session/lifecycle`, so importing `github` would fail Story 1.1.0's dependency check). Host-scoped token resolution comes through `type TokenSource interface{ TokenForHost(host string) string }`, supplied by `gitwiring`, which adapts `github.GetKeychainTokenForHost` (`github/keychain.go:131`) and the `gh` token. No code is extracted out of `github/keychain.go`.
**Files**: `session/git/backend/gogit/credential/provider.go`, `provider_test.go`, `session/gitwiring/tokensource.go`

##### Task 3.1.1a: Productionize the S5 prototype with host scoping (~5 min x2)
- Files: `session/git/backend/gogit/credential/provider.go`

##### Task 3.1.1b: `approve`/`reject` on success/failure, helper timeout and `safeexec` process-group kill (~5 min)
- Files: `session/git/backend/gogit/credential/provider.go`

### Story 3.1.2: HTTPS transport with redirect policy
**As the** maintainer, **I want** an HTTP client that never forwards credentials cross-host, **so that** CVE-2026-41506 and #2136 cannot bite us.
**Acceptance Criteria**:
- Cross-host redirect drops credentials.
  - *Given* an `httptest` server at host A redirecting to host B, *When* `Fetch` runs with A's token, *Then* host B's request has no `Authorization` header and the operation fails with a redacted error.
**Files**: `session/git/backend/gogit/credential/transport.go`, `transport_test.go`

##### Task 3.1.2a: Custom `http.Client` and `client.InstallProtocol` (~5 min)
- Files: `session/git/backend/gogit/credential/transport.go`

### Story 3.1.3: `insteadOf` and opt-in HTTPS-to-SSH fallback (O-5)
**As the** maintainer, **I want** URL rewriting handled before go-git is called, **so that** HTTPS-to-SSH setups still work.
**Acceptance Criteria**:
- `insteadOf` applied.
  - *Given* `url."git@github.com:".insteadOf=https://github.com/` in `~/.gitconfig`, *When* `Fetch` runs for `https://github.com/o/r.git`, *Then* the SSH URL is used with agent auth.
- Fallback is opt-in.
  - *Given* setting `git_https_to_ssh_fallback=false` (default), *When* HTTPS auth fails, *Then* no SSH retry happens and the error is returned; with `true` one retry via SSH agent happens and is counted.
- ssh_config limits route to CLI.
  - *Given* a host with `ProxyJump` in `~/.ssh/config`, *When* `Fetch` runs, *Then* the CLI is used (reason `capability_ssh_proxy`).
**Files**: `session/git/backend/gogit/credential/urlrewrite.go`, `urlrewrite_test.go`, `config/config.go`

##### Task 3.1.3a: `insteadOf` resolver over the `gitconfig` `EffectiveConfig` (Story 1.3.4; not go-git `ConfigScoped`, which misses `~/.config/git/config` when `~/.gitconfig` exists and follows no include) (~5 min)
- Files: `session/git/backend/gogit/credential/urlrewrite.go`

##### Task 3.1.3b: Opt-in fallback and the setting (~5 min)
- Files: `config/config.go`, `session/git/backend/gogit/credential/urlrewrite.go`

## Epic 3.2: `network` cohort (STRETCH, outside the appetite, gate GL; clone, fetch, push, pull; features.md E1-E7)

### Story 3.2.1: Refactor `repo_path.go` clone/fetch into the backend (Refactor-first)
**As the** maintainer, **I want** tokens kept out of argv and `.git/config`, **so that** the clone path no longer leaks credentials.
**Acceptance Criteria**:
- No token in config on any path.
  - *Given* a clone with a token-bearing `originURL` and a forced failure after clone but before cleanup, *When* the error path runs, *Then* `.git/config` `remote.origin.url` contains no userinfo (gogit uses `Auth`, never a token URL) and `ps` argv in the CLI path never contains the token (CLI path gets the token via credential helper env, not argv).
- Local-path and `file://` remotes never reach go-git's file transport (adversarial C-c).
  - *Given* go-git v5.19.2's `file` protocol, which runs `git-upload-pack`/`git-receive-pack` (and `git --exec-path` when those are not on `PATH`) through `execabs` (`plumbing/transport/file/client.go:39-99`, VERIFIED by reading; the Revision 3 text did not mention it), *When* `Clone`, `Fetch`, `FetchAll`, `FetchBranch`, `Push`, `Pull` or `ListRemote` is routed, *Then* two independent guards hold. (1) **Preflight**: the router takes the URL the backend will actually hand to go-git, after the plan's own resolver (Story 3.1.3) has applied `insteadOf`/`pushInsteadOf` (go-git's own rule, `config.go:373-376`, reads only the repo's config and misses a global rewrite into a path), and asks `transport.NewEndpoint(<that URL>)` for **both the fetch URL and the push URL** (`pushurl` / the last of `RemoteConfig.URLs`; fetch uses `URLs[0]`, push the last element, `remote.go:84-86,113-114,416`) plus any direct `RemoteURL` option; `Protocol == "file"` on either (a path, `file://`, or an `insteadOf` that rewrites into one) routes the operation to the CLI, reason `capability_local_transport`, an ADR-003 tier 3 carve-out; the same predicate is applied to the clone source and to every configured remote for `FetchAll`. (2) **Tripwire**: `gitwiring` re-registers the `file` protocol with `client.InstallProtocol("file", tripwire)` at startup, where `tripwire` returns the typed `ErrLocalTransport` from `NewUploadPackSession`/`NewReceivePackSession` before any `exec`; the router maps it to the CLI with `Wrote()==false`, and a test asserts that with `PATH` containing only shim stubs a `gogit` clone of a local bare repo produces zero shim hits from go-git itself. An in-process local transport (`plumbing/transport/server.NewClient(server.NewFilesystemLoader(...))`, which exists in v5.19.2) is **not** adopted: it would skip the `pre-receive`/`update`/`post-update` hooks the CLI's `git receive-pack` runs for a local push target, a behaviour change.
  - *Given* local clones in this repo (`session/repo_path.go:314,418`, the `mainrepo` remote, fixtures that clone from a bare directory), *When* Epic 5.1 migrates fixtures, *Then* they build the bare remote with `PlainInit` plus in-process object writes (not a `file` clone) wherever the test is not itself about the CLI; the remaining ones are `gitoracle` tests.
- Prune and refspec semantics preserved.
  - *Given* `fetch --all --prune` with remote branch `old` deleted upstream, *When* `FetchAll(prune=true)` runs on gogit, *Then* `refs/remotes/origin/old` is removed, equal to CLI; branch names with `:` or leading `-` are rejected before refspec construction.
**Files**: `session/repo_path.go`, `session/git/backend/gogit/network.go`, `session/git/backend/gogit/network_test.go`, `session/git/backend/gogit/transport_tripwire.go`, `session/git/backend/gogit/transport_tripwire_test.go`, `session/git/ops.go`

##### Task 3.2.1a: `Clone`, `FetchAll`, `FetchBranch` in gogit and CLI, the `local_transport` preflight and the `file` protocol tripwire; wire `Backend.Fetch` into `native.MergeDeps.Fetch` (Story 1.1.0 rule 8) (~5 min x4)
- Files: `session/git/backend/gogit/network.go`, `session/git/backend/cli/network.go`

##### Task 3.2.1b: Migrate `repo_path.go`, `ops.go` callers (~5 min x2)
- Files: `session/repo_path.go`, `session/git/ops.go`

### Story 3.2.2: Push and pull
**As the** maintainer, **I want** push (with `-u`) and pull in-process, **so that** the backlog PR flow stops spawning.
**Acceptance Criteria**:
- Non-fast-forward maps to the same error.
  - *Given* a remote ahead of local, *When* `Push` runs on both backends, *Then* both return `ErrNonFastForward` (typed) with the same wrapped meaning.
- Pull that cannot fast-forward routes to CLI.
  - *Given* `pull.rebase=true` or diverged branches, *When* `Pull` runs under `network=gogit`, *Then* the CLI path runs with reason `unsupported_pull_mode` (a member of the closed `FallbackReason` enum, not gate-blocking, unlike `error`), because go-git `Pull` only fast-forwards (#942).
- Pre-push hook present routes to CLI (ADR-006).
**Files**: `session/git/backend/gogit/network.go`, `session/vc/git_provider.go`, `session/git/worktree_git.go`, `server/services/unfinished_work_service.go`

##### Task 3.2.2a: `Push`, `Pull` with capability checks (~5 min x2)
- Files: `session/git/backend/gogit/network.go`

##### Task 3.2.2b: Migrate callers (~5 min x2)
- Files: listed above

### Story 3.2.3: Network oracle suite
**As the** maintainer, **I want** real-remote parity tests, **so that** fetch/push semantics are checked against the CLI.
**Acceptance Criteria**:
- Bare-remote parity over a transport go-git implements.
  - *Given* a local bare repo served by `git daemon --export-all --enable=receive-pack --base-path=<dir>` on a free port (`git://127.0.0.1:<port>/repo.git`; the daemon is a CLI helper started by the `gitoracle`-tagged test, so it is exempt from the gate), *When* clone, fetch (tags auto-follow), push of a new branch, and delete-ref push run on both backends, *Then* resulting refs and `git fsck` agree (`gitoracle` tag). The `file://` and path cases are covered by the Story 3.2.1 tripwire test instead, because `gogit` never runs them.
**Files**: `session/git/backend/gogit/network_oracle_test.go`

##### Task 3.2.3a: Oracle tests (~5 min x2)
- Files: `session/git/backend/gogit/network_oracle_test.go`

---

# Phase 4: Worktree residue and fork patches

## Epic 4.1: `worktree` cohort residue (existing-branch add, self-heal remove/list; conditional on G3 for checkout perf and on G4 (INFERRED: the admin-file writes share the CLI-compatible lock protocol); consistency C6)
**Goal**: Remove the last local worktree CLI calls using the already-validated native admin writer.

### Story 4.1.1: Add worktree for an existing branch (`worktree_ops.go:217`)
**As the** maintainer, **I want** resume to create a worktree on an existing branch in-process, **so that** the pause/resume path stops spawning.
**Acceptance Criteria**:
- Resume attaches without moving or deleting the branch.
  - *Given* branch `feat/x` at commit `abc1234` and no worktree (paused session), *When* `AddWorktreeForExistingBranch(path, "feat/x")` runs, *Then* `<path>/.git` is a file `gitdir: <common>/worktrees/<name>`, HEAD is `ref: refs/heads/feat/x`, the ref still points at `abc1234`, and oracle `git worktree list --porcelain` and `git fsck` agree.
- Branch checked out elsewhere is refused.
  - *Given* `feat/x` already checked out at `/w/old`, *When* the same call runs for `/w/new`, *Then* it returns `ErrBranchInUse{Path:"/w/old"}` (CLI wording preserved: "already used by worktree").
- Race recovery (lost a concurrent create) still works.
  - *Given* two goroutines adding the same branch, *When* both run, *Then* exactly one succeeds, the other returns `ErrBranchInUse`, and no orphan admin dirs remain (`git worktree prune --dry-run` empty).
**Files**: `session/git/native/worktree_add_existing.go`, `session/git/native/worktree_add_existing_test.go`, `session/git/worktree_ops.go`

##### Task 4.1.1a: Reuse `AllocateAdminDirName` and admin writer; write checkout without creating the ref (~5 min x2)
- Files: `session/git/native/worktree_add_existing.go`

##### Task 4.1.1b: Concurrency and oracle tests (~5 min x2)
- Files: `session/git/native/worktree_add_existing_test.go`

##### Task 4.1.1c: Replace `worktree_ops.go:217` CLI call, keep CLI fallback behind `worktree` cohort (~5 min)
- Files: `session/git/worktree_ops.go`

### Story 4.1.2: Self-heal remove and list
**As the** maintainer, **I want** `worktree_ops.go:214,291` and `path_completion_service.go:217` on the native functions, **so that** no local `worktree` CLI call remains.
**Acceptance Criteria**:
- Self-heal handles partial admin dirs.
  - *Given* a worktree whose admin dir has `gitdir` but no `commondir` (crash mid-add) and a `locked` file, *When* the self-heal remove runs, *Then* `nativeUnlockWorktree` then `nativeRemoveWorktree` leave no admin dir, never touch `refs/heads/*`, and `git worktree prune --dry-run` is empty.
- List parity.
  - *Given* 3 worktrees (one locked, one directory deleted), *When* `ListWorktrees` runs in-process, *Then* it equals the porcelain parse including `prunable` and `locked`.
**Files**: `session/git/worktree_ops.go`, `server/services/path_completion_service.go`, `session/git/native/worktree_list.go`

##### Task 4.1.2a: Route `:214`, `:291` and `path_completion_service.go:217` to native (~5 min x2)
- Files: those

##### Task 4.1.2b: Tests for partial/locked/missing-dir cases (~5 min)
- Files: `session/git/native/worktree_remove_test.go`

### Story 4.1.3: Remote-host worktrees stay CLI (carve-out, enforced by type)
**As the** maintainer, **I want** the carve-out enforced by the type system and allow-listed by reason, **so that** remote sessions are never answered from the wrong local repo and the zero-spawn gate does not misreport them.
**Acceptance Criteria**:
- Remote locations always route to the CLI backend.
  - *Given* `session/git/remote_worktree.go:86,103` and `session_service_create.go:372` migrated to `Remote{Host, Path, Runner}` locations built by `backend.Locate` (Story 1.1.5), *When* the router handles them with every cohort set to `gogit`, *Then* the `cli` backend runs through that SSH `Runner`, reason `remote_host`, and the gogit backend is never invoked (Story 1.1.3 test).
- Remote runs are visible to the gate without tripping it.
  - *Given* `tmux.LocalRunner` for `Local` repos and an SSH runner for `Remote`, *When* the gate runs, *Then* `git_backend_cli_spawn_total{reason="remote_host"}` counts remote runs, the allow-list has an entry for `(any operation, remote_host)`, and the invariant is the inequality `sum(backstop) <= sum(backend counter, reason != remote_host)` (Observability Plan): SSH runs never reach the `safeexec` backstop, so they add nothing to the left side and are excluded from the right side, and cannot cause a false failure. A `Local` call through `LocalRunner` that is not allow-listed fails the gate. (Prior text claimed remote runs "never go through local safeexec" without evidence; the corrected statement is that `LocalRunner` is a local-spawn path and `SSHRunner` is not, VERIFIED at `session/tmux/command_runner.go:82` and `ssh_runner.go:335`.)
**Files**: `session/git/remote_worktree.go`, `session/git/backend/locate.go`, `executor/safeexec/spawn_allowlist.go`

##### Task 4.1.3a: Allow-list keyed on (operation, reason) (~3 min)
- Files: `executor/safeexec/spawn_allowlist.go`

## Epic 4.2: Fork patches (each conditional on its spike gate and O-6; each passes the fork-only test)
**Goal**: Carry only patches whose need was demonstrated by a failing test against public API. Runs only if O-6 resolved to keep the fork and a gate produced a patch; expected to be empty (see top of plan). Patches must be behaviour-only (0.3).

### Story 4.2.1: F1 operation-level lock API (only if G4 = "public API cannot reach the index write")
**As the** maintainer, **I want** go-git to hold `index.lock` across the read-modify-write (not just replace the file in `SetIndex`), **so that** concurrent CLI writers are safe.
**Acceptance Criteria**:
- The S4 failing test now passes against the fork.
  - *Given* the S4 race test with stock go-git failing, *When* run against `v5.19.3-ssq.N`, *Then* it passes 200 iterations, the CLI-reader-never-sees-partial-index loop passes, upstream's own `storage/filesystem` tests still pass, and the rollback test (Story 1.2.3) passes.
**Files**: fork `storage/filesystem/index.go` (additive new file plus minimal hook), fork tests

##### Task 4.2.1a: Patch and fork tests (~5 min x3)
- Files: fork repo

##### Task 4.2.1b: Tag `-ssq.N`, bump `replace`, run oracle (~5 min)
- Files: `go.mod`, `go.sum`

### Story 4.2.2: F2 object lookup survives repack (only if G6 = "needs storage patch")
**As the** maintainer, **I want** one refresh-and-retry on `ErrObjectNotFound`, **so that** concurrent `gc`/`repack` does not break reads.
**Acceptance Criteria**:
- S6 failing test passes against the fork.
  - *Given* the S6 repack loop, *When* run for 60 s against the fork, *Then* zero `ErrObjectNotFound` occur.
**Files**: fork `storage/filesystem/object.go` (additive), fork tests

##### Task 4.2.2a: Patch, tests, tag, bump (~5 min x3)
- Files: fork repo, `go.mod`

### Story 4.2.3: F4 performance patches (only if G3 shows a regression vs CLI on a hot op)
**As the** maintainer, **I want** targeted perf patches, **so that** the SLO is met without leaving an op on CLI.
**Acceptance Criteria**:
- Measured improvement.
  - *Given* the S3 benchmark for worktree add on the large repo, *When* re-run on the patched fork, *Then* p50 is at or below CLI p50 and no upstream test regresses; otherwise the patch is dropped and the op stays `cli`.
**Files**: fork repo, `gates.md`

##### Task 4.2.3a: Patch and benchmark (~5 min x3)
- Files: fork repo

### Story 4.2.4: Upstream-eligible patches proposed upstream
**As the** maintainer, **I want** clean patches offered upstream, **so that** the carried set shrinks over time.
**Acceptance Criteria**:
- Each patch is classified.
  - *Given* each fork patch, *When* reviewed, *Then* `README-SSQ.md` in the fork marks it "upstream-eligible (PR link)" or "fork-only (reason)".
**Files**: fork `README-SSQ.md`

##### Task 4.2.4a: Classify and open PRs where eligible (~5 min)
- Files: fork repo

---

# Phase 5: Tests, soak, and the zero-spawn gate

## Epic 5.1: Test fixture migration (223 test sites; 121 under `session/`)
**Goal**: Fixtures stop spawning git except `OracleTest`s and unavoidable cases, with no test weakened.

### Story 5.1.1: Fixture helpers on the backend
**As the** maintainer, **I want** shared in-process fixture helpers, **so that** 223 call sites do not each get rewritten ad hoc.
**Acceptance Criteria**:
- Helpers cover init, commit, branch, worktree add, config, clone.
  - *Given* `testutil/gitfixture`, *When* a test calls `NewRepo(t).Commit("a.txt")`, *Then* no `git` exec occurs (shim count 0) and `git fsck` (oracle) is clean; `testutil/gitfixture/identity.go` uses `SetConfig` instead of `git config --local`.
  - *Given* the `clone` helper, *When* a test needs a remote, *Then* the helper builds the bare remote in-process (`PlainInit(bare)` plus in-process object and ref writes, or a `git://` remote served by a `gitoracle`-tagged `git daemon`) and **never performs a local-path or `file://` clone**: go-git's `file` transport execs `git-upload-pack` (Story 3.2.1) and those routes are CLI carve-outs (`capability_local_transport`). Shim count is 0 for the in-process shape; tests that are about local clones are `gitoracle` tests. (Consistency B1.)
- Starts in week 1 (Revision 6, pre-mortem P2#4): this epic is the cheapest spawn reduction for the `session` test package and does not depend on any cohort or on the fork; its batches are reported weekly against gate GS (section 0.3.1).
**Files**: `testutil/gitfixture/*.go`, `testutil/gitfixture/gitfixture_test.go`

##### Task 5.1.1a: Extend fixtures using Backend (~5 min x3)
- Files: `testutil/gitfixture/*.go`

### Story 5.1.2: Migrate session tests in batches
**As the** maintainer, **I want** the 121 `session/` sites migrated in batches of about 15 per PR, **so that** each PR is reviewable and parity is preserved.
**Acceptance Criteria**:
- Parity and speed.
  - *Given* each batch PR, *When* `go test ./session` runs, *Then* pass count is unchanged (no test deleted or skipped; `git diff --stat` shows no removed `func Test`), wall time does not exceed the post-#955 baseline (about 72 to 92 s), **summed mutex delay (`go test -mutexprofile`, summed delay versus Story 0.1.2's `audit.md` figure) does not exceed the baseline** (requirements Success Metric 3; consistency C2, validation G-1; test `sessionMutexDelay_should_NotExceedBaseline_When_BatchMerged` run by a new `scripts/test-session-timing.sh`, which does not exist yet), and the spawn count falls by the batch's sites.
**Files**: `session/*_test.go` (batches; start with `session/review_gate_test.go`, 31 CLI sites), `scripts/test-session-timing.sh` (new)

##### Task 5.1.2a: Batch 1, `review_gate_test.go` (~5 min)
- Files: `session/review_gate_test.go`

##### Task 5.1.2b: Batches 2 to 8, about 15 sites each (~5 min per file group)
- Files: remaining `session/*_test.go`

##### Task 5.1.2c: Migrate `server/services` and others (the other 102 test sites), including the 10 direct `exec.Command("git"` sites (`server/services/backlog_service_test.go`, `backlog_service_triage_test.go`, `session/backlog_lifecycle_test.go`) that bypass `safeexec` (~5 min per file group)
- Owner of the direct-exec test sites: this task. Until migrated they are listed in the gate's shim allow-list by file.
- Files: `server/services/*_test.go`, `session/backlog_lifecycle_test.go`, `pkg/**/*_test.go`

### Story 5.1.3: Keep the CLI oracle permanently
**As the** maintainer, **I want** the real-git cross-check kept, **so that** go-git-created state is always validated by real git.
**Acceptance Criteria**:
- Oracle job runs in CI.
  - *Given* `make test-oracle` with tag `gitoracle`, *When* run in CI, *Then* it covers worktree add/remove, merge, commit, network, `fsck`, and is excluded from `ZeroSpawnGate`.
**Files**: `Makefile`, `.github/workflows/build.yml`

##### Task 5.1.3a: Confirm coverage list and CI wiring (~5 min)
- Files: `Makefile`, `.github/workflows/build.yml`

## Epic 5.2: Concurrency soak including repack (needs 2.3, 4.1)
**Goal**: Prove dozens of sessions behave under concurrent in-process and CLI writers.

### Story 5.2.1: Soak test
**As the** maintainer, **I want** a repeatable soak, **so that** the rollout gate is evidence.
**Acceptance Criteria**:
- Soak passes.
  - *Given* 24 sessions, each looping create worktree, add+commit, status, remove on one repo for 5 minutes while one goroutine runs `git repack -ad` and another runs CLI `git add`/`checkout` inside the worktrees (simulating `AgentProcess`), *When* the soak ends, *Then* zero errors other than named retries, `git fsck --strict` is clean, `git worktree prune --dry-run` is empty, no `.lock` files remain, resident memory growth is under 500 MB, and `git_lock_contention_total` is reported.
**Files**: `session/git/backend/soak_test.go` (tag `soak`), `Makefile`
- Soak also reports counts of `torn_read` and `object_missing` fallbacks (input to G8's deletion decision) and **fails** if `(torn_read + object_missing) / refs-cohort calls` exceeds the ceiling recorded at G8 (Story 0.2.8), if `git_capability_detect_total{outcome="detect_error"}` exceeds 1% of `Capabilities` calls (Story 1.3.4), if any journaled lock outlives its scope, or if any `*.lock` file remains; it runs a variant with shadow sampling on and a variant in which one writer process is `SIGKILL`ed mid-operation and restarted (Story 2.3.1 recovery), after which the other 23 sessions must complete without a lock error.

##### Task 5.2.1a: Soak harness (~5 min x3)
- Files: `session/git/backend/soak_test.go`

##### Task 5.2.1b: `make test-soak` and nightly CI job (~5 min)
- Files: `Makefile`, `.github/workflows/soak.yml` (new)

##### Task 5.2.1c: Nightly performance SLO re-check (validation G-2) (~5 min)
- The nightly job re-runs the Story 0.2.3 benchmark subset (`status`, `diff`, `rev-parse HEAD`, worktree add) for every operation whose cohort is `gogit` on the largest managed repo and fails if gogit p50 exceeds the recorded G3 CLI p50 for that operation (`perfRegression_should_FailNightly_When_P50ExceedsRecordedG3Baseline`). The same check runs once immediately after each cohort flip and after each fork rebase or `go-git` version bump (Story 6.3.1), so the SLO is enforced after the spike, not only at it.
- Files: `.github/workflows/soak.yml`, `session/git/backend/gogit/bench_test.go`

## Epic 5.3: Zero-spawn gate and no-git container (needs O-1, O-9, O-10)

### Story 5.3.1: Two-layer zero-spawn gate
**As the** maintainer, **I want** CI to fail when the server or its tests spawn unexpected `git`, **so that** the metric holds over time and matches the requirements' own definition ("measured with a PATH shim counting `git` execs").
**Scope and carve-outs (explicit, with owners)**:
| Spawn source | In gate? | Owner |
|---|---|---|
| `safeexec` git constructor sites (36) and runner sites (25) | Yes, layer 2 (operation, reason) and layer 1 shim | Epics 2 to 4 |
| 10 direct `exec.Command("git"` sites in `*_test.go` | Yes, layer 1 shim only (invisible to `safeexec`) | Story 5.1.2c |
| `gh pr create` (`session/git/worktree_git.go:581`) and git run by `gh` | **Carve-out**, allow-listed by operation `CreatePR`, reason `agent_tool` (ADR-003 amended, O-10) | Tyler decides O-10 |
| `exec.LookPath("git")` in `session/vcs/detect.go:123` | Not a spawn, but removed or replaced so detection works with no git binary | Story 5.3.2a |
| Remote-host runs through `Runner` | Yes, counted `(any, remote_host)`, allow-listed | Story 4.1.3 |
| `OracleTest`s (tag `gitoracle`) | Excluded by tag | Story 1.3.3 |
| go-git `file` transport (`execabs`: `git-upload-pack`, `git-receive-pack`, `git --exec-path`) for local-path and `file://` remotes | Layer 1 only (not `safeexec`); prevented by the `capability_local_transport` route and the `file` protocol tripwire; allow-listed as `(Clone\|Fetch\|FetchAll\|FetchBranch\|Push\|Pull, capability_local_transport)` on the CLI backend | Story 3.2.1 |
| Operations that write worktree files and are not yet promoted (`Checkout`, `Reset(Hard\|Merge)`, `Restore(Worktree)`, `Remove`, `RemoveGlob`, `Move`) | Allow-listed as `(operation, unsafe_worktree_write)`, under the ratchet | Story 2.3.2 |
| 20 `Setenv("PATH"` test sites in 12 files (`git grep` 2026-10-08: `cmd/ssq-hooks/main_test.go` x6, `server/port_owner_test.go:20,39`, `server/services/hook_status_service_test.go:57`, `server/services/path_completion_service_test.go:480`, `server/server_integration_test.go:52`, `github/client_pr_by_number_test.go:187`, `session/pr_tracking_test.go:26`, `session/worktree_pr_poller_discovery_test.go:56`, `session/vc/git_provider_test.go:1462`, `session/headless/pool_test.go:576,589`, `session/git/ops_test.go:249`, `session/unfinished/gogitstore/gogitstore_test.go:249,300`) | Each replaces `PATH` wholesale and drops a shim directory, so a spawn inside it is invisible to layer 1 | Migrated to `spawngate.SetPath`; `norawgitpath` forbids new ones | Story 5.3.1b |
| 29 `exec.LookPath("git")` test sites in 10 files (`session/mcp_integration_test.go:33,149`, 26 in `session/unfinished/gogitstore/*_test.go`, `session/vc/git_provider_test.go:1451`; the two `mcp_integration_test.go` sites verified to be `t.Skip`, the rest to be confirmed by Task 5.3.1b), 1 product site (`session/vcs/detect.go:123`) | With no git binary the skipping ones report "pass"; the no-git run is vacuous for them | Migrated to `spawngate.RequireGit(t)` and tracked in a ratcheted skip list | Story 5.3.1b |
| `executor/circuit_breaker_test.go:141` (`"/usr/bin/git"`) | **Not a spawn**: the test builds an `exec.Cmd` and only calls `commandClass(tc.cmd)` (`:155-157`), nothing runs. Not a gate hole (the second review listed it as one; checked by reading the test) | `//nolint:norawgitpath` with that reason | Story 5.3.1b |
**Acceptance Criteria**:
- Layer 1: PATH shim over the whole module, uncached.
  - *Given* all cohorts `gogit` and a shim directory on `PATH` (also exported as `SSQ_GIT_SHIM_DIR`) holding one stub per helper name the server, a test or go-git can exec: `git`, `git-upload-pack`, `git-receive-pack`, `git-upload-archive`, `git-lfs` and `gh` (the last so `gh pr create` hits are attributed to the `agent_tool` carve-out instead of silently running in the container); every stub is the same script, records its own `argv[0]` basename, and `git-upload-pack`/`git-receive-pack` are what make go-git's file transport visible: go-git execs them directly when they are on `PATH` and runs `git --exec-path` only when they are not (`plumbing/transport/file/client.go:77-99`, VERIFIED by reading), so both shapes appear in the log, not only the second as the adversarial re-review 2 implied. Each stub appends `(argv0, argv, cwd, parent test-binary name from $PPID)` to `$SSQ_GIT_SPAWN_LOG` and exits non-zero, *When* `go test -count=1 ./...` runs with no `-run` filter (the `-count=1` is mandatory: a cached pass replays without running, so no shim hit would be recorded; the gate script refuses to run without it), *Then* any shim hit outside `scripts/git-spawn-shim/allow.txt` fails the run, and the report is grouped per package (the test-binary name) with each hit's cwd and argv. The module's `tools/lint` is a separate module (`tools/lint/go.mod`) and is run in a second invocation. Packages that spawn from `github/`, `executor/`, `testutil/`, `cmd/` and `pkg/` are covered by `./...`, not only `./session ./server/...`.
- Attribution is honest about what a shim can know.
  - *Given* a hit whose cwd is under a `t.TempDir()`, *When* the report is written, *Then* it names the test: `t.TempDir()` embeds the sanitized test name in the directory (`TestFooBar2166080487/001`, `TestFooBarsubx441792601/001` for `TestFooBar/sub/x`, VERIFIED with a scratch test on 2026-10-08), so the report parses it out; for any other cwd it reports `(package, cwd, argv)` only and says so. A child process cannot learn the Go test name, and `t.Setenv` cannot carry it because it panics in `t.Parallel()` tests.
- Layer 2: operation and reason, as an inequality.
  - *Given* the same run with `SSQ_GIT_SPAWN_DUMP`, *When* the dump is read, *Then* every `git_backend_cli_spawn_total{operation,reason}` pair is in the allow-list (keyed on (operation, reason), never `file:line`), and `sum(git_cli_spawn_backstop_total) <= sum(git_backend_cli_spawn_total{reason != remote_host})`; a violation fails with the offending spawn's `file:line` from the dump. **`remote_host` is excluded from the right-hand sum** (adversarial C6): SSH runs are counted by `git_backend_cli_spawn_total{reason="remote_host"}` but never by the `safeexec` backstop, so including them would let remote runs hide a local bypass; the allow-list keeps its `(any operation, remote_host)` entry, but that entry is not part of the inequality. A test feeds a synthetic run with one remote spawn and one unrouted local spawn and asserts the gate fails. Spawns by go-git's `execabs` file transport are counted by neither layer-2 counter (they do not pass `safeexec`); layer 1 catches them, and the Story 3.2.1 tripwire prevents them.
- A program that a test launches and that runs `git` itself cannot hide a real spawn.
  - *Given* a test that starts a fake agent which runs `git`, *When* its stub hit is logged, *Then* `allow.txt` keys the entry on the **parent process being that test's own child** (the stub records `$PPID` and the grandparent test-binary name), not on argv alone, so the same argv from the server process still fails; the rule is stated at the top of `allow.txt`. The no-git container report (Story 5.3.2) states the coverage limit: the container drives a hand-picked flow (create, edit, pause, cleanup, local clone) plus the `tests/e2e` suite, and any operation outside both is covered by layer 1 only.
- Paths that replace or bypass the shim are removed.
  - *Given* `tools/lint/norawgitpath` (an analyzer, in `make lint-custom`), *When* a test calls `t.Setenv("PATH", ...)` or `os.Setenv("PATH", ...)` outside `testutil/spawngate`, or passes a literal absolute git path (`/usr/bin/git`, `/opt/homebrew/bin/git`, `/usr/local/bin/git`) to `exec.Command*`/`safeexec.*`, *Then* lint fails. All 20 `Setenv("PATH"` sites are migrated to `spawngate.SetPath(t, dirs...)`, which sets `PATH` to `dirs` plus `$SSQ_GIT_SHIM_DIR` when set, so "git is not discoverable" tests still see the shim (and still cannot find a real git). `executor/circuit_breaker_test.go:141` carries a `nolint` with the reason above.
- The no-git run cannot pass vacuously.
  - *Given* `PATH` containing no `git`, *When* `go test -count=1 -json ./...` runs, *Then* the set `S_nogit` of tests that skip is compared with the set `S_normal` skipping under a normal PATH: `S_nogit \ S_normal` must be a subset of `scripts/no-git-skips.txt`, each entry produced only by `spawngate.RequireGit(t)` (skip message prefix `requires-git:`); and every test that passes under the normal PATH must pass or be in that file. The file is a **ratchet**: CI compares it with `origin/main:scripts/no-git-skips.txt` and fails if it gained entries. The 29 `exec.LookPath("git")` skips are the initial content only until each is migrated to an in-process fixture (Story 5.1.2c); `vcs/detect.go` no longer short-circuits (Task 5.3.2a).
- The allow-lists cannot launder spawns.
  - *Given* `executor/safeexec/spawn_allowlist.go` (operation, reason pairs) and `scripts/git-spawn-shim/allow.txt` (shim entries), *When* a diff adds an entry to either, *Then* `TestSpawnAllowlistRatchet` fails unless the committed baseline counts (`spawn_allowlist.baseline`, `allow.txt` line count) are edited in the same diff, and CI additionally compares both with `origin/main` and fails if either grew without an issue link in the commit message. There is no `CODEOWNERS` file in this repository (VERIFIED: none at `.github/`, root or `docs/`), so the control is the ratchet plus the CI comparison, not an ownership rule. The earlier "capability fixtures that explicitly require git" and the `gitoracle` tag are not free exclusions: a capability fixture that needs a real git binary is an `OracleTest` (tag `gitoracle`), and the tag's test list is itself under the same ratchet.
- The gate graduates from report-only to blocking by a stated rule, and is time-boxed (pre-mortem P2#5).
  - *Given* the gate runs in report-only mode, *When* 10 consecutive CI runs on `main` produce no unexplained hit (provisional count; Tyler may change it) *Then* a one-line PR flips it to blocking; *Given* the shim layer is not at least report-only by end of week 4, *Then* tripwire T3's drop order removes Layer 2 (the backstop-versus-backend inequality) first and keeps Layer 1. Allow-list growth needs the ratchet edit and an issue link (above); there is no other waiver path.
**Files**: `executor/safeexec/spawn_gate_test.go`, `executor/safeexec/spawn_allowlist.go`, `executor/safeexec/spawn_allowlist.baseline`, `scripts/git-spawn-shim/git` (plus links named `git-upload-pack`, `git-receive-pack`, `git-upload-archive`, `git-lfs`, `gh`), `scripts/git-spawn-shim/allow.txt`, `scripts/spawn-gate.sh`, `scripts/no-git-skips.txt`, `testutil/spawngate/spawngate.go`, `tools/lint/norawgitpath/analyzer.go`, `Makefile`

##### Task 5.3.1a: Gate test, shim layer, per-package report, `-count=1` guard and `make ready` hook (~5 min x2)
- Files: `executor/safeexec/spawn_gate_test.go`, `scripts/spawn-gate.sh`, `Makefile`

##### Task 5.3.1b: `testutil/spawngate` (`SetPath`, `RequireGit`), migrate the 20 PATH sites and 29 skip sites, `norawgitpath` analyzer, skip-set comparison and ratchets (~5 min x4)
- Files: the 12 and 10 files listed above, `testutil/spawngate/spawngate.go`, `tools/lint/norawgitpath/analyzer.go`, `scripts/no-git-skips.txt`

### Story 5.3.2: No-git-binary container run (scope per ADR-003)
**As the** maintainer, **I want** the server proven to run without `git` installed, **so that** tier 1 is verified.
**Acceptance Criteria**:
- Container run.
  - *Given* a minimal container (`debian:stable-slim` pinned by digest, no `git`, no `gh`) with the built binary and a pre-cloned local repo on a volume, *When* the server starts and a session on a non-git-using program is created, worked on (file edits), paused and cleaned up through the API, *Then* every call succeeds and the backend spawn counters are 0. The container flow never clones from or pushes to a local-path or `file://` remote (those are tier-3 carve-outs, `capability_local_transport`, and cannot succeed with no `git` on `PATH`; consistency B1); where the flow needs a remote it uses `git://` served by a `git daemon --export-all --enable=receive-pack` that runs **on the host side** of the container boundary (the daemon is a `gitoracle`-tier test helper, not part of the server), or an HTTP remote, and the clone/push steps run only if `network=gogit` (stretch, Epic 3.2); until then the container flow starts from a pre-cloned volume and skips clone and push. The doc states explicitly that git-using agents inside sessions, `gh`, hooked/signed/LFS repos and remote hosts still need git (ADR-003 tiers 2 and 3), so "no git" means the server-process tier only.
- The hand-picked flow is not the only coverage; the gap is measured, not assumed.
  - *Given* the container flow extended to also exercise diff (staged and unstaged), merge-main-into-worktree, `Commit` and push (both only when `localwrite`/`network` are enabled, i.e. stretch; push goes to the host-side `git://` remote above, never a local bare path), PR-info (native `github` package, no git), branch rename and the backlog-lifecycle create-to-terminal-cleanup path through the ConnectRPC API, *When* it finishes, *Then* `run.sh` reads the per-`OperationName` counters (`git_backend_cli_spawn_total` and the gogit-implementation counts of `git_operation_duration_ms`) and writes a coverage table of every `Backend` method with its hit count; `what-no-git-means.md` embeds the list of methods with zero hits as "not exercised by the container run" (so a surviving spawn in them is acknowledged as invisible to this layer; it is still caught by the PATH-shim layer over `./...`, Story 5.3.1). The container run does not drive the Playwright suite (`tests/e2e/global-setup.ts` always starts its own local server, so it cannot target a container without a separate change); that limit is stated in the doc.
**Files**: `tests/no-git-container/Dockerfile`, `tests/no-git-container/run.sh`, `docs/explanation/what-no-git-means.md`

##### Task 5.3.2a: Remove the `exec.LookPath("git")` pre-check from `session/vcs/detect.go:123` (~5 min)
- `gitAvailable` returns false without a git binary today, so VCS detection fails in the container before any backend runs. Replace with a `Backend` capability probe (`IsRepo(loc)`), in-process for `Local`.
- Files: `session/vcs/detect.go`, `session/vcs/detect_test.go`

##### Task 5.3.2b: Dockerfile and script (~5 min x2)
- Files: `tests/no-git-container/*`

##### Task 5.3.2c: Explanation doc of tiers and carve-outs (~5 min)
- Files: `docs/explanation/what-no-git-means.md`

---

# Phase 6: Rollout, retirement, maintenance

## Epic 6.1: Staged rollout
**Goal**: Flip cohorts on evidence, worktree and network last.

### Story 6.1.1: Per-cohort promotion
**As the** maintainer, **I want** a mechanical promotion checklist, **so that** flips are not judgement calls.
**Acceptance Criteria**:
- Promotion criteria are checked.
  - *Given* a **read** cohort (`refs`, `diffstatus`, the read half of `network`) at stage `shadow`, *When* promoted to `gogit` on the maintainer's instance, *Then* these hold: 7 days and at least 500 shadowed calls per operation family with zero `shadow_mismatch_total{class="real"|"false_clean"}` (racy mismatches reported, not blocking); G8 passed for `refs` (outcome A, B or C recorded); for `diffstatus`, the Story 2.2.1 repo-coverage and zero-`false_clean` criteria; oracle and soak green (including the `torn_read`/`object_missing` and `detect_error` ceilings); G3 p50 not worse than CLI (and the nightly SLO check of Task 5.2.1c green after the flip); **summed mutex delay not above the Story 0.1.2 baseline** (consistency C2); no `fallback_total{reason="error"}`; and the result is recorded in `gates.md` with the counter values.
- Write cohorts never shadow, so they have their own evidence (consistency C4): `localwrite` (stretch), the write halves of `network` (`push`, `clone`) and the write half of `worktree` (`AddWorktreeForExistingBranch`, self-heal remove).
  - *Given* a write cohort, *When* promoted to `gogit` on the maintainer's instance, *Then* all of these hold and are recorded in `gates.md`: (1) the cohort's oracle suite is green (`fsck --strict` clean, `git worktree list`, reflog and tree equality against the CLI twin); (2) for `localwrite`, the required-CI lock tests of Story 2.3.1 are green; (3) a **dogfood window of 14 days** during which each in-process write is followed by **post-write CLI verification** (a `git fsck --connectivity-only` plus `git status --porcelain` comparison on the written repo, sampled at 100% for the first 7 days and 10% after, behind a dogfood-only flag and counted `git_backend_postwrite_verify_total{operation,outcome}`; the verification spawn is excluded from the gate by that flag), with zero failed verifications; (4) the soak (Epic 5.2) is green, including the `SIGKILL`-and-restart variant; (5) no `fallback_total{reason="error"}` and no `ErrLocked{Journaled:true}` (the journal gap of the previous order no longer exists); (6) `localwrite` is enabled only per listed repo, never by default, in the first release.
- Default flips in order.
  - *Given* cohorts promoted in order refs, diffstatus, network (stretch), worktree, *When* release N ships, *Then* the default for each cohort with 14 days clean is `gogit` and `cli` stays selectable via config and `STAPLER_SQUAD_GIT_BACKEND=cli`. **`localwrite` is excluded from default flips in the first release** (opt-in per repo, Epic 2.3 stretch conditions).
**Files**: `config/git_backend.go`, `docs/how-to/flip-git-backend-cohort.md`, `implementation/gates.md`

##### Task 6.1.1a: Default-flip code change per cohort (~3 min each)
- Files: `config/git_backend.go`

##### Task 6.1.1b: Update runbook and gate record (~5 min)
- Files: `docs/how-to/flip-git-backend-cohort.md`, `implementation/gates.md`

## Epic 6.2: Remove the cohort flag and non-capability fallbacks (one full release cycle later)
**Goal**: Delete dead fallback branches and the per-cohort flag, **keeping the `cli` backend as a permanent implementation** for the carve-outs. (Reworded from "CLI retirement": hooked, signed, LFS, remote-host and `gh` cases keep needing the CLI, per ADR-003 and ADR-006.)

### Story 6.2.1: Remove flag and fallbacks for clean cohorts
**As the** maintainer, **I want** dead fallback code removed, **so that** the codebase has one path per operation plus the explicit carve-outs.
**Acceptance Criteria**:
- Removal criteria.
  - *Given* release N+1 and a cohort at `gogit` default through release N with zero `fallback_total{reason="error"}`, *When* the cohort flag and its non-capability fallback branches are deleted, *Then* `make ready` passes; the `cli` backend, the capability-preflight routes (ADR-006), the `remote_host` route and the `torn_read`/`object_missing` route remain; and the `norawgitcli` baseline is exactly the remaining sanctioned sites (the `cli` backend and `gitwiring` Runner), documented in the story, not "0 for that cohort".
- Mitigation is not deleted before proof.
  - *Given* the `getHeadCommitSHA` retry-then-CLI mitigation (`session/git/util.go:326-345`), *When* this story runs, *Then* it is deleted only if G8 recorded a fix for the root cause and the Epic 5.2 soak shows zero `torn_read`/`object_missing` occurrences; otherwise it stays as the permanent route.
**Files**: `session/git/backend/routed.go`, `config/config.go`, `session/gitwiring/parse.go`

##### Task 6.2.1a: Delete flag and per-cohort fallbacks, keep carve-outs (~5 min per cohort)
- Files: as above

## Epic 6.3: Fork maintenance runbook
**Goal**: Make the single-maintainer rebase burden explicit and bounded.

### Story 6.3.1: Rebase and advisory runbook
**As the** maintainer, **I want** a monthly routine, **so that** upstream security fixes reach the fork within days.
**Acceptance Criteria**:
- Runbook is complete and rehearsed.
  - *Given* a new upstream v5 tag, *When* the runbook is followed, *Then* `ssq/v5` is rebased, a new `-ssq.N` tag is cut, `replace` is bumped, `make ci`, oracle and soak pass, and the nightly performance SLO check (Task 5.2.1c) is re-run against the new tag before the `replace` bump merges (validation G-2), and the time and conflict count are logged in `docs/how-to/rebase-go-git-fork.md`; GHSA feed and release tags are watched (a scheduled GitHub Action opens an issue on a new upstream release or advisory).
**Files**: `docs/how-to/rebase-go-git-fork.md`, `.github/workflows/go-git-upstream-watch.yml`

##### Task 6.3.1a: Runbook (~5 min)
- Files: `docs/how-to/rebase-go-git-fork.md`

##### Task 6.3.1b: Upstream-watch workflow (~5 min)
- Files: `.github/workflows/go-git-upstream-watch.yml`

### Story 6.3.2: Quantify rebase burden (resolves O-7)
**As the** maintainer, **I want** measured cost per rebase, **so that** the user can judge whether the fork stays worth it.
**Acceptance Criteria**:
- Report after 2 rebases.
  - *Given* the first two real upstream releases after the fork exists, *When* each rebase finishes, *Then* minutes spent, conflicting files and advisory count are recorded; if conflicts exceed 3 files or time exceeds 2 hours twice, open a review of ADR-001/the patch set.
**Files**: `docs/how-to/rebase-go-git-fork.md`

##### Task 6.3.2a: Log measurements (~3 min each rebase)
- Files: `docs/how-to/rebase-go-git-fork.md`
