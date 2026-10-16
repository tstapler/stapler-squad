# Requirements: go-git-worktree-and-merge

**Date**: 2026-09-07
**Type**: feature addition (subsystem replacement within `session/git/`)
**Complexity**: 4 — high-stakes / cross-cutting

## Problem Statement
`session/git/` manages every session's isolated git worktree, and two of its subsystems still shell out to the `git` CLI for every call: worktree lifecycle (`worktree_ops.go`'s add/remove/list/prune/unlock, ~18 call sites) and merging origin's main branch into a session's worktree (`ops.go`'s `MergeMainIntoWorktree`: fetch + `merge --no-edit` + conflict detection + `merge abort`). This repo already converted several other `session/git/` operations to `go-git` this session (commit-to-commit diffing, working-tree diff content — PR #721) to eliminate subprocess fork/exec cost under load (50-90+ concurrent worktree sessions is this host's normal range), but `go-git` v5 (the version this repo depends on) has **no API at all** for multi-worktree management and only supports fast-forward merges — verified directly against the v5.19.2 source, not assumed. Closing these two gaps requires building custom code on top of `go-git`'s lower-level primitives, not a drop-in library call.

## Baseline
Today: `worktree_ops.go` spawns a `git` subprocess for every add/remove/list/prune/unlock (up to `runGitCommand`'s 30s timeout under load); `MergeMainIntoWorktree` spawns four subprocesses per call (fetch, merge, optional abort, conflict-file diff). Both are on the hot path of session creation/teardown and of backlog automation's PR-fix/rebase flow respectively.

## Users / Consumers
- `session/` package: every session creation (`NewGitWorktree`), teardown, and reconnection path.
- Backlog automation: `MergeMainIntoWorktree` is used to keep a long-running backlog session's branch current with origin's main branch before attempting a PR.
- Indirectly, anything that shells real `git` (e.g. `gh` CLI PR operations, `FetchBranch`, the deliberately-still-subprocess `CheckoutBranch`/`RemoteURL` conversions from a parallel in-flight PR) against the same worktree paths this subsystem creates — see Constraints.

## Success Metrics
- Zero `git` subprocess spawns for worktree add/remove/list/prune/unlock in the steady-state path (down from 100% today).
- `MergeMainIntoWorktree`'s common cases (fast-forward, clean 3-way merge, conflicting merge) no longer shell out to `git fetch`/`git merge`/`git merge --abort` for the merge/conflict-detection portion (fetch itself may still be subprocess-based per Constraints below).
- A worktree created by the new code is indistinguishable from one created by real `git worktree add` to any other tool that inspects it: `git worktree list --porcelain` run from the main repo, `git status`/`gh` CLI run inside the linked worktree, and this repo's own still-subprocess call sites (`FetchBranch`, `CheckoutBranch`) all continue to work unmodified against it.
- No regression in `session/git`'s existing test suite; new code has equivalent or better test coverage (real filesystem-backed tests, not mocks) than the subprocess implementation it replaces.

## Appetite
Large (3–6 weeks)
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

## Constraints
- **No cgo.** Release build is pinned to `CGO_ENABLED=0` with a dedicated CI gate (`.github/workflows/goreleaser-check.yml`) added after a past cgo-only-dependency regression. Pure Go only.
- **On-disk interop with real git is mandatory, not optional.** Worktrees created by this new code must produce the exact `.git`-redirect-file + `.git/worktrees/<name>/{gitdir,commondir,HEAD,index,...}` administrative layout real `git worktree add` produces, because other code in this repo (subprocess `git`, `gh` CLI, and a human running `git` by hand against `~/.stapler-squad/worktrees/`) continues to operate on these same paths. This is the single highest-risk constraint in this project — see Feasibility Risks.
- Must preserve `session/git`'s existing documented go-git landmines from this session's investigation (do not silently reintroduce them):
  - go-git's direct HEAD-ref-file read has a documented production bug (torn-read race) on a linked worktree — see `getHeadCommitSHA`'s doc comment in `util.go`. New worktree/merge code that reads HEAD on a worktree path must use or extend that existing hardened path, not a naive `repo.Head()` call.
  - go-git v5.19.2's `Repository.Merge()` only implements `FastForwardMerge` — confirmed directly against source. The three-way merge algorithm must be built from `go-git`'s tree/object/diff primitives, not assumed to exist as a library call.
- Must not depend on `go-git` v6's `x/` experimental packages (worktree management, in-progress ORT merge scaffolding) — those are alpha and can break between releases; this project owns its own code rather than pinning to unstable upstream API, though their design (already reviewed) may inform this project's approach.
- No new build-system dependency (no Bazel, no new C toolchain). Stays within this repo's existing Make + Go modules setup.

## Non-functional Requirements
- **Performance SLO**: worktree add/remove/list/prune and merge/conflict-detection must be at or below current subprocess latency under this host's normal concurrent-session load; the point of this project is removing subprocess spawn cost, not trading it for a slower pure-Go path.
- **Scalability**: must remain correct with 50-90+ concurrent worktrees against one main repo (this host's documented normal range this session).
- **Security classification**: internal (local developer tool, no external network exposure of this subsystem).
- **Data residency**: not applicable.

## Scope
### In Scope
- A worktree-management layer (add/remove/list/prune/unlock-equivalent) built on `go-git` primitives plus hand-rolled administrative-file handling, replacing `worktree_ops.go`'s subprocess calls and `worktree.go`'s `worktree list --porcelain`.
- A three-way merge implementation (base/ours/theirs, conflict markers, conflicted-file reporting) built on `go-git`'s tree-diff and object primitives, replacing `MergeMainIntoWorktree`'s merge/conflict-detection/abort portion.
- Real filesystem-backed tests for both, including interop tests that shell out to real `git` to verify the resulting worktrees/merge results are recognized correctly.
- A feature-flag-gated rollout (global + per-session override, live-settable — matching this repo's existing stream-hub/tymux rollout pattern from earlier this session) with fallback to the current subprocess implementation, given the corruption blast radius of a bug in worktree lifecycle management.

### Out of Scope
- `FetchBranch` (network fetch/auth) — stays subprocess; not part of this project (separate, deliberate exclusion already documented in `ops.go`).
- `RenameBranch` (`git branch -m`) — flagged this session as its own high-risk future project (checked-out-branch HEAD-symref corruption risk); not bundled into this one.
- Any `gh` CLI / GitHub API operation in `worktree_git.go` — unrelated to git plumbing.
- `worktree_git.go`'s status/commit/add conversions and `ops.go`'s `CheckoutBranch`/`RemoteURL` — being handled by a separate, already-in-flight conversion this session; do not duplicate.
- `diff.go`'s working-tree diff — already converted and merged this session (PR #721).
- The libgit2-transpile-via-Bazel approach — confirmed NO-GO by a due-diligence spike (see Alternatives Considered); not pursued further.
- Windows worktree support — this repo's deployment targets (Linux/macOS) don't require it; do not add complexity for a platform not in use.
- Octopus merges, `.gitattributes` merge drivers, submodule-aware merging — the merge implementation targets the actual call pattern (`origin/<mainBranch>` into a session branch, two parents), not general-purpose git merge.

## Rabbit Holes
- Worktree "locked"/"prunable" administrative states (`git worktree lock`/`unlock`, stale-worktree detection) — real git's rules here have edge cases; scope to what `worktree_ops.go`'s current callers actually use, not full parity with every `git worktree` subcommand.
- Merge rename detection (a file renamed on one side, modified on the other) — decide explicitly in planning whether to support this or treat it as a conflict, rather than discovering the gap mid-implementation.
- CRLF/line-ending normalization during merge — this repo's sessions are Linux/macOS-only; confirm whether this can be skipped entirely.
- Exact byte-for-byte match of real git's conflict-marker format and `git status`/`git diff` output for a conflicted file — needed for interop (backlog automation's fix-agent prompts currently show real conflict markers to an LLM), not just "some" conflict representation.

## Alternatives Considered
- **cgo-bound libgit2**: rejected — reintroduces cgo, breaks this repo's `CGO_ENABLED=0` release-build guarantee and its dedicated CI gate.
- **C-to-Go transpiler (libgit2 → Go via Bazel + cxgo)**: **confirmed NO-GO** by an isolated spike (`~/code/github.com/tstapler/libgit2-go-transpile-spike`, local-only, not pushed). The two smallest, most isolated files in libgit2 (~550 lines, zero network/TLS/crypto) broke `cxgo`'s own transpilation before any git-specific logic was touched — its bundled `<stdint.h>` shim emits an invalid C99 `INT64_MIN` constant (a universal C idiom, not a libgit2 quirk), and patching that reveals a second failure (broken Go-type-mapping) plus cross-file type redeclaration cxgo can't dedup. Bazel/`rules_go`/`gazelle` itself worked correctly end-to-end — the blocker is entirely transpilation correctness. Full evidence in that repo's `SPIKE_FINDINGS.md`. This option is closed; no further investigation planned.
- **Other pure-Go git libraries**: surveyed — none exist beyond `go-git`. `gogs/git-module` (used by Gogs/Gitea) is itself a shell-out-to-`git`-CLI wrapper, not a native implementation, so it doesn't solve the underlying problem.
- **Wait for go-git v6 GA**: rejected for now — v6 is alpha (`v6.0.0-alpha.5`), its worktree package lives under an explicitly experimental `x/` path, and its merge support is scaffolded (types declared) but not yet functional (`Repository.Merge()` still hard-rejects non-fast-forward as of alpha.5, verified against source). Revisit once v6 reaches GA — this project's design should not preclude migrating onto a stable upstream implementation later if one becomes available.

## Feasibility Risks
- **On-disk format interop** (see Constraints) is the biggest risk: getting `.git/worktrees/<name>/` administrative files subtly wrong could corrupt a worktree in a way real `git` also can't recover, or that only surfaces later (e.g. `git worktree prune` from an unrelated process silently deleting a worktree our code still thinks is valid). Needs dedicated research-phase investigation of the exact format (versioned across git releases?) before implementation.
- **go-git API sufficiency**: unclear whether `go-git` v5's public API exposes what's needed to write a real conflicted index (stage 1/2/3 entries) in a way `git status`/`git diff` recognize afterward, or whether this needs direct index-file manipulation. Research phase must validate this concretely (a spike against a real repo) before planning commits to an approach.
- **Concurrency**: multiple sessions may create/remove worktrees against the same main repo concurrently; real `git worktree add` has its own internal locking assumptions this custom implementation must replicate or it risks racing itself (two sessions' worktree-admin-file writes interleaving).

## Observability Requirements
- Tracing spans for worktree add/remove/list/prune and merge/conflict-detection operations, following this session's existing OTel work (`docs/how-to/enable-opentelemetry.md`), so a regression shows up in Tempo the same way the original diff-timeout investigation did.
- Metrics: operation latency (to prove the subprocess-elimination goal), and a conflict-rate counter for the merge path (distinguishing "no changes needed" / "clean fast-forward" / "clean 3-way merge" / "conflicted") to catch a merge-correctness regression in production before it silently produces wrong results.

## Risk Control
Feature-flag-gated rollout, global + per-session override, live-settable with no restart required — matching this repo's existing `stream_hub`/`tymux` feature-flag rollout pattern (`config.FeatureFlags`, `EffectiveXEnabled`, per-session override map, RPC + settings panel) built earlier this session. Both new subsystems (worktree management, merge) fall back to the existing subprocess implementation when their flag is off or a session-level override disables them, so a corruption bug can be killed instantly per-session or globally without a redeploy.

## Open Questions
- Does `go-git`'s index-writing API support representing a real 3-way conflict (stage 1/2/3 entries) faithfully enough for `git status`/`git diff` to show it the same way as a real `git merge` conflict? (Research phase.)
- Is the `.git/worktrees/<name>/` administrative format stable across the git versions this repo's users actually have installed, or does it vary in ways that matter? (Research phase.)
- Should worktree management and merge ship as two separate feature flags (independent rollout) or one? (Leaning toward two, given they're independent subsystems with independent risk profiles — confirm in planning.)
