# ADR-002: Active PR is derived (with a user pin); legacy fields are an always-on projection

**Status**: Accepted (owner decision 2026-11-03) — **Project**: session-multi-pr-tracking

## Context
Many callers (27 non-test Go files, 8 web-app files, the mobile app) read one PR. Stacked work can have two open PRs at once. Actions on a stale PR (merge, close) are destructive.

## Decision
- `ActivePR` is a pure function: pinned PR, else open PR whose head branch equals the session's current branch, else newest `open` PR, else newest non-terminal `unknown`, else newest effective-terminal (an unverified terminal row counts as terminal only once its verify is exhausted; before that it is a non-terminal `unknown`), else none (a refinement of the owner's "newest non-terminal": an `unknown`, i.e. unreachable, PR never outranks an older open one). Every PR action also accepts an explicit `PullRequestRef`; an explicit ref must be attached to the session or the call fails with `FailedPrecondition`. An unset ref never means PR number 0.
- Only a user RPC pins; automation never does.
- Legacy columns (`github_pr_*`, `Instance.GitHubPR*`) are ALWAYS a projection of `ActivePR` once the session has any link, written from one place (`Instance.AttachPR` in `session/instance_pr_set.go`, snapshot built by the pure `buildSnapshot`). The mirror does not depend on the flag or on any scope: a flag flip republishes no snapshots, `buildSnapshot` reads no config, and the legacy value is always a valid attached PR. (Revised: the first draft gated the projection per reader scope, which cannot work because one set of legacy values serves consumers governed by different scopes, and a flip republishes nothing.)
- Flag `session_multi_pr` (default off, live-settable via the feature-flag RPC, never an env var) with scopes `reader:poller`, `reader:backlog`, `reader:actions`, `reader:ui`. The scopes gate readers only and depend on each other in that order, applied when the flag is read (an out-of-order enable is stored but resolves off until its prerequisite scopes are effective; the flag service has no per-scope write-time guard, Triad iteration 2). Per-PR row state is kept current from the first deploy by a flag-independent state refresher (it updates rows only and drives no session behavior with scopes off), because the unflagged gate and sweeper read those rows; the first flip therefore has no cold poll burst and no flag-off staleness leak. The refresher is the one always-on component with network cost, so it alone has a live kill switch: the scope `refresher_off` of the same flag (default off, so the refresher runs; outside the enable order; Phase 4 pre-mortem #3). The other always-on parts (link and state writes, the projection, the fail-closed sweeper and gate, boot maintenance) are listed in the plan's Risk Control.
- Safe-direction exceptions are not flag-gated: the retention sweeper and the worktree-cleanup gate fail closed on any non-terminal, `unknown` or "legacy number but no link" PR.
- Session terminal (`github_pr_status_terminal`) is derived: at least one link, all linked states terminal (`unknown` is not terminal) and discovery idle. It is persisted through a clearable full-save path (the old full saves could only set it true). Discovery keeps a slow cadence for all-terminal, non-archived sessions so PR B after a merged PR A is found.
- Ratchets: a new field-scoped `make pr-field-guard` for direct writes to the PR fields outside `session/instance*.go` (the existing `actor-field-guard` regex is field-agnostic and would fail on unrelated `session_driver.go` lines, VERIFIED), an allow-list test of legacy-field readers tagged snapshot-backed or `InstanceData`-backed, and a second allow-list for the backlog item `PrNumber/PrURL` readers.

## Alternatives rejected
- Purely explicit active PR: forces a choice on every existing caller that has none.
- Purely derived with no pin: no escape hatch for the stacked ambiguity.
- Stored `current_pr_id`: a second source of truth that drifts.
- Single global flag: cannot soak the poller before the UI.

## Consequences
Rollback is a flag flip, no restore. A drift reconciler (always on; corrects through the actor) plus counter guards the projection during rollout; it only corrects drift. Legacy numbers that no link represents (the revert-then-redeploy case) are imported by the boot backfill BEFORE instances load, so the projection can never overwrite them. Slow discovery adds at most one cached list call per all-terminal session per slow tick.
