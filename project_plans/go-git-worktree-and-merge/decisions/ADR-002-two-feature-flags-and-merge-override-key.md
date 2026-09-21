# ADR-002: Two feature flags (worktree, merge), and the merge flag's override key

**Status**: Accepted
**Date**: 2026-09-07
**Project**: go-git-worktree-and-merge

## Context

`requirements.md`'s Open Questions leaned toward two independent flags
("worktree management and merge are independent subsystems with independent
risk profiles") but left it for planning to confirm. `research/architecture.md`
§4.5 found a concrete asymmetry that makes this not just a risk-profile
question but a structural one: `GitWorktree`'s methods already carry a
`sessionName` field (`session/git/worktree.go:79`) to key a per-session
override off of, exactly like `StreamHubSessionOverrides`/
`TymuxSessionOverrides` already do. `MergeMainIntoWorktree`, by contrast, is
a package-level function with signature `(worktreePath, mainBranch string)`
and **no session-name parameter at all** — there is no `sessionName` to key
an override off of without either (a) changing the function's signature
(caller changes at all three call sites — rejected per the Tech Debt
Disposition's "zero caller changes" goal) or (b) deriving a session name from
`worktreePath` via a lookup that exists elsewhere in the codebase
(`session/backlog_lifecycle_pr.go:119`'s `NewGitWorktreeFromStorage`
reconstruction) but isn't available inside `session/git` today without a new
import-direction dependency (`session/git` importing something from
`session/` proper would very likely create an import cycle, since
`session/` already imports `session/git`).

## Decision

**Two independent feature flags:**

- `native_git_worktree` (`config.NativeWorktreeFeatureFlag`) — gates
  `GitWorktree.Setup`/`SetupLocked`/`Remove`/`Cleanup`/`Prune` and the
  worktree-list helper functions. Per-scope override key: **`sessionName`**
  (`config.NativeWorktreeSessionOverrides map[string]bool`), mirroring
  `StreamHubSessionOverrides` exactly — `GitWorktree` already has this field.
- `native_git_merge` (`config.NativeMergeFeatureFlag`) — gates
  `MergeMainIntoWorktree`. Per-scope override key: **`worktreePath`**
  (`config.NativeMergeWorktreeOverrides map[string]bool`), not `sessionName`.

Both flags default **off** (unlike `StreamHubFeatureFlag`'s default-on):
this is corruption-blast-radius code touching every session's git state, not
a transparent perf optimization with no data-integrity downside — the
higher bar from `requirements.md`'s Risk Control section ("a corruption bug
can be killed instantly... without a redeploy") argues for an opt-in rollout,
not opt-out.

**Why `worktreePath` for the merge override, not a derived `sessionName`:**
`worktreePath` is the one piece of caller-supplied identity `MergeMainIntoWorktree`
already receives at every one of its three real call sites (`drift.go:74`,
`backlog_service_triage.go:2414`, and the `branchReconciler` value installed
in `backlog_lifecycle.go:672`), so keying the override off it requires no
signature change and no new cross-package lookup. It is coarser than a true
per-session key in one specific way: if a session's worktree path ever
changes mid-lifecycle (it doesn't, today — a `GitWorktree`'s `worktreePath`
is fixed at construction per `worktree.go`), an override set against the old
path would silently stop applying. This is judged an acceptable, explicitly
recorded gap rather than a blocker, since worktree paths are immutable for
the lifetime this project's scope covers.

**Both flags are evaluated fresh on every call** (no per-session-cached
"this session uses implementation X for life" decision), matching
`EffectiveStreamHubEnabled`'s existing precedent and required by
`research/architecture.md` §5's flag-flip-mid-burst analysis: the
`WithRepoWorktreeLock` critical section, shared by both implementations for
the same repo path, is what makes fresh-evaluation safe — without that
shared lock, fresh-evaluation would let two concurrent operations dispatch
to different implementations and race unlocked. This ADR treats that shared
lock as a load-bearing prerequisite of this decision, not a separate,
independent hardening step.

## Alternatives Considered

1. **One combined flag (`native_git`) gating both subsystems together.**
   Rejected: `research/architecture.md`'s own finding that the merge
   implementation's fidelity bar is meaningfully lower than the worktree
   implementation's (§3: no current consumer reads on-disk conflict-marker
   content, only the `[]string` path list) means the two subsystems can
   plausibly reach "ready to flip on" at different times. A combined flag
   would force them to ship and roll back together, discarding that
   independence for no benefit.
2. **Per-session override for merge too, via a new `worktreePath →
   sessionName` lookup function added to `session/git`.** Rejected for v1:
   the only existing reverse-lookup path
   (`session/backlog_lifecycle_pr.go:119`) lives in `session/`, which already
   imports `session/git` — adding the reverse import would very likely
   create an import cycle (not verified by compiling, but the dependency
   direction audit in `research/architecture.md` gives no indication a
   cycle-free path exists, and resolving it is out of this project's
   already-large appetite). `worktreePath`-keying achieves the same
   operational goal (an operator can silence native-merge for one specific
   troublesome worktree) without the cross-package plumbing.
3. **Per-repo (not per-worktree, not per-session) override for merge**,
   keyed by the main repo's `repoPath` (derivable from `worktreePath` via the
   existing `commondir` resolution). Considered but rejected as the default:
   it's coarser than necessary — a repo can have 50-90+ concurrent worktrees
   per `requirements.md`'s Non-Functional Requirements, and an operator
   debugging one specific merge failure would rather silence one worktree
   than every session against that repo. Not precluded as a future addition;
   `worktreePath`-keying does not block adding a repo-level override later.

## Consequences

- `config/config.go` gains four new fields
  (`NativeWorktreeSessionOverrides`, `NativeMergeWorktreeOverrides`) and two
  flag constants, plus getter/setter pairs mirroring the
  `StreamHubSessionOverrides`/`TymuxSessionOverrides` shape exactly (see
  `implementation/plan.md` Phase 4, Epic 4.1).
- `MergeMainIntoWorktree`'s public signature does not change — the ADR-002
  decision is precisely what keeps the Tech Debt Disposition's "zero caller
  changes" promise achievable for the merge subsystem.
- If a future project needs true per-session merge overrides, it must first
  resolve the `session/git` → `session/` import-direction question this ADR
  explicitly declines to take on now.
