# Requirements: go-git-fork-full-git-replacement

**Date**: 2026-10-08
**Type**: feature addition (cross-cutting replacement of a subprocess dependency)
**Complexity**: 4 — high-stakes / cross-cutting

## Problem Statement
stapler-squad shells out to the `git` CLI in product code (about 50 call sites in `session/`, `server/`, `pkg/`, `testutil/`) and in tests (about 150 call sites in `session/` alone). `go-git` (v5.19.2) covers most read/write operations but cannot create or remove linked worktrees, and does not reproduce the CLI's credential-helper and ssh-fallback behaviour for fetch/push/clone. The team wants to know whether a private fork of go-git can close those gaps so the CLI is never needed — at runtime or in tests.

## Baseline
- Product code: `rev-parse` (19 sites), `push` (5), `fetch` (5), `worktree` (3), `diff` (3), `status`/`remote`/`for-each-ref`/`config`/`clone` (2 each), plus single uses of `symbolic-ref`, `rev-list`, `merge-base`, `log`, `commit`, `add` (counted by grep on `"git"` command construction; not exhaustive of helpers).
- Tests: `session/review_gate_test.go` still has 31 CLI fixture call sites after #955/#956; worktree add/remove, checkout, clone stay on the CLI because go-git cannot reproduce them.
- Cost today: each CLI call forks a process; on the developer machine the `git` on PATH is an ssh-fallback wrapper that spawns extra `git` processes per call, and EDR adds per-exec latency, so subprocess spawn is the limiting cost of the `session` test package after #955 (fork-lock mutex delay is the largest remaining mutex).
- Users/workaround: none — work-arounds are the CLI calls themselves.

## Users / Consumers
- stapler-squad server (session lifecycle: worktree create/cleanup, review gate, backlog PR flow).
- The repo's test suites (session, server/services).
- The maintainer (Tyler), who would own the fork.

## Success Metrics
- Zero `git` subprocess spawns in a full session create → work → pause → cleanup cycle, and in the `session` test package (measured with a PATH shim counting `git` execs; baseline: hundreds per package run).
- Behavioural parity: every existing `session` and `server/services` test passes with the CLI fallbacks removed, with no test deleted or weakened to get there.
- `session` package wall time and summed mutex delay do not regress against the post-#955 numbers (baseline ~72–92s wall).
- Runtime works on a machine with no `git` binary installed (verified in a minimal container).

## Appetite
Large (3–6 weeks), long-lived private fork acceptable. (If the scope does not fit, cut scope — do not move the deadline.)

## Constraints
- Must keep working with the user's existing auth: system credential helpers / keychain, the ssh-fallback behaviour (HTTPS→SSH), GitHub Enterprise hosts.
- The fork must be rebased on upstream go-git releases; the maintenance cost is owned by one person.
- Windows is not a stated target; macOS and Linux are.
- Behaviour must match the CLI on linked-worktree semantics (`.git` file, per-worktree HEAD/index, `commondir`), since tests exercise the real thing.

## Non-functional Requirements
- **Performance SLO**: no slower than the CLI path for the same operation on repos of the size stapler-squad manages (large monorepos included); not otherwise specified.
- **Scalability**: dozens of concurrent sessions each with its own worktree.
- **Security classification**: internal; touches credential handling (tokens, SSH keys) — must not weaken current handling.
- **Data residency**: no special requirements.

## Scope
### In Scope
- Linked-worktree support (add, remove, list/prune, open, per-worktree HEAD/index/config) in a go-git fork.
- In-process replacements for the remaining CLI call sites: rev-parse family, diff/status, merge-base/rev-list/log, remote/config, for-each-ref/symbolic-ref.
- Network operations (fetch/push/clone) with credential-helper, SSH-agent and ssh-fallback parity.
- Test fixtures using the fork instead of the CLI where behaviour is equivalent.
- A rollback path while the migration is incomplete.

### Out of Scope
- Replacing the `gh` CLI or other non-git tools.
- Upstreaming to go-git as a requirement (allowed, not required).
- Changing user-visible session behaviour.

## Rabbit Holes
- go-git's storage model assumes one worktree per repository; per-worktree HEAD/index/refs and `commondir` may touch much of the core, not just a new API.
- Credential-helper protocol and the HTTPS→SSH fallback are bespoke; reproducing them in-process may be larger than the worktree work.
- Parity of `git diff`/`status` output (renames, whitespace, large trees) with the CLI that existing code parses as text.
- Performance of go-git on very large repos (status/diff are known to be slow vs the CLI).
- Fork maintenance: rebase cadence, security fixes, version skew with libraries that depend on upstream go-git.

## Alternatives Considered
- Keep the CLI for the hard cases (status quo plus #955/#956 style test cleanups).
- A thin CLI-backed pool or long-running `git cat-file --batch`/`git` daemon to cut fork cost without replacing git.
- Another pure-Go implementation or libgit2 bindings (cgo) instead of forking go-git.
- Upstream the worktree work to go-git and wait.

## Feasibility Risks
- Linked worktrees may require deep storage-layer changes, making the fork hard to rebase.
- go-git performance on large repos may make "no CLI" slower than today.
- Credential/ssh-fallback parity may not be achievable in-process without re-implementing `git credential` helpers.
- Single-maintainer fork risk (security fixes, bus factor).

## Observability Requirements
Count and log every remaining CLI fallback (operation, call site) and every fork-path error with the operation name, so the "zero subprocess" metric is measurable in production and CI; surface counters on the existing metrics/log pipeline.

## Risk Control
Per-operation switch (config flag) that falls back to the CLI, defaulting to the CLI until each operation passes parity tests; staged rollout operation by operation, worktree add/remove last; the CLI path is not deleted until the fork path has run in production for a full release cycle.

## Open Questions
- Does upstream go-git (v5.x/v6 branch) already have, or have an open PR for, linked-worktree add/remove? What is its state?
- Which of the 50 product call sites genuinely need the CLI semantics vs. go-git equivalents (per-site audit)?
- What exactly does the ssh-fallback wrapper do, and is it product behaviour or a local dev convenience?
- Is performance on the largest repos stapler-squad manages acceptable with go-git status/diff?
- What is the realistic rebase burden of the fork per upstream release?

## Addendum (post-research)
**Date**: 2026-10-08. The sections above are the user-agreed text and are unchanged. This addendum records, factually, where the plan (`implementation/plan.md` Revision 6) and ADRs deviate from or narrow them. **Every item below is pending user confirmation (O-1, O-5, O-6, O-9).** Nothing here has been agreed by the user.

- **Success Metric 1 (zero `git` spawns) and Metric 4 (no `git` binary at runtime) are rescoped to the server process** (ADR-003 tiers; O-1, O-9). Agent programs, `gh`, IDEs and remote-host sessions run `git` themselves; the plan therefore measures "zero `git` spawns in the server process except allow-listed carve-outs" and verifies Metric 4 with a container that runs the server and non-git-using sessions only. Carve-outs: remote-host worktrees, repos failing the capability preflight (hooks, signing, LFS, split index and similar), local-path and `file://` remotes, operations that write worktree files and are not yet promoted, `gh pr create`, and the `torn_read`/`object_missing` route.
- **Metric 2 ("every existing test passes with the CLI fallbacks removed") is rescoped**: the `cli` backend is kept permanently as an implementation for those carve-outs (ADR-004 item 7; plan Epic 6.2 removes the cohort flag and non-capability fallbacks, not the `cli` backend). The metric is checked as "zero outside the allow-list, and the allow-list only shrinks" (validation.md G-3).
- **Constraint "HTTPS to SSH fallback" is defaulted off** (ADR-005, plan Story 3.1.3; O-5): research found the `git` wrapper that does this is a dotfiles convenience, not product behaviour, so the plan makes it an opt-in setting (`git_https_to_ssh_fallback`, default false). Until O-5 resolves, the network cohort can differ from today's behaviour for HTTPS-only repos on the maintainer's machine. Credential-helper binaries keep working through an always-on fallback (ADR-005).
- **Observability Requirements: counters are keyed on operation and fallback reason, not call site** (ADR-004 item 4): counting by call site collapses to the CLI helper after migration and changes with every edit. `file:line` appears only in the test-run dump (`SSQ_GIT_SPAWN_DUMP`).
- **In Scope "linked-worktree support ... in a go-git fork": the fork is expected to be empty** (plan section 0 end-state statement; O-6). Linked-worktree add, remove, list, prune and merge already exist in this repository on stock go-git v5.19.2 (`session/git/native_worktree_*.go`), and the locking and repack designs are wrapper-level. The plan still sets up the pinned fork (F0) as directed and asks at gate G7a whether to keep an empty fork or use stock go-git v5.
- **Appetite**: plan Revision 6 places `localwrite` (in-process commit/add/restore with CLI-compatible locking), the `network` cohort with credential client, and the full git-config resolver **outside** the 3 to 6 weeks as an explicit stretch phase behind a go/no-go gate (GL), because the pre-mortem judged them unlikely to fit one maintainer's appetite. Derived scope that has no requirements line of its own (config resolver, lock journal, reflog writer, extra lint analyzers, package-topology extraction) is justified by the Constraint "behaviour must match the CLI" and the "must not weaken credential handling" non-functional requirement.
