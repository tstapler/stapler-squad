# Requirements: async-session-creation

**Date**: 2026-08-26
**Type**: feature addition (cross-cutting change to a core RPC)
**Complexity**: 4 — high-stakes / cross-cutting

## Problem Statement

`CreateSession` (`server/services/session_service.go:1799`) resolves everything —
GitHub URL cloning, alias/default resolution, branch/session-type inference —
synchronously, before the `ManagedInstance` is ever created and before
`SessionCreatedEvent` is published. For fast paths (plain directory sessions)
this is invisible. For slow paths — especially resolving/cloning a GitHub PR
URL against a GitHub Enterprise host like `github.netflix.net`, which is
VPN/corp-network-gated and can be slow or flaky — the omnibar dialog blocks
with no feedback for the RPC's full duration (up to `createSessionTimeout`,
150s), then force-refreshes once it finally resolves. The user sees this as a
freeze, not a slow-but-working operation.

## Baseline

Today, a user pasting a GitHub Enterprise PR URL into the omnibar and hitting
Create sees the dialog hang with a "Creating…" button label but no other
feedback, for as long as the clone/resolution takes (seconds to the full
150s timeout on a bad network day). The new session is invisible in the
session list until the RPC returns; if resolution fails, the omnibar
resurfaces an error but any *partial* work (e.g. an already-started clone)
has no visible trace. There is no way to cancel a stuck creation, retry a
failed one without re-entering the whole omnibar flow, or notice a creation
that silently got orphaned (e.g. by a server restart mid-goroutine).

## Users / Consumers

- All `stapler-squad` users creating sessions via the omnibar or the "New
  Session" dialog — both call the same `CreateSession` RPC
  (`sessionv1connect.SessionServiceHandler.CreateSession`).
- The web-app's session list (`SessionCard.tsx`, `useSessionService.ts`),
  which already renders `SESSION_STATUS_CREATING` with a spinner and
  `creation_progress` text (`SessionCard.tsx:235,955-959`) but currently
  never gets a chance to show it early, since the RPC doesn't return (and
  the instance doesn't exist) until all resolution work is done.
- **MCP tool callers** — agents (e.g. a Claude Code session) invoking the
  `create_session` (`server/mcp/tools_lifecycle.go`) or
  `create_session_for_pr` (`server/mcp/tools_github.go`) MCP tools. Unlike
  the omnibar/web UI, these callers expect a tool call that returns once,
  synchronously, with either a fully-resolved session or a clear error — an
  agent has no UI to watch a `Creating` card update in. Today these two
  tools implement their own fully-synchronous, separate creation path
  (`session.NewInstance` → `inst.Start(true)` → `store.AddInstance`, never
  calling `SessionService.CreateSession`) and would otherwise block on the
  same slow GitHub-clone/worktree-startup work this project fixes for the
  web UI — an out-of-scope gap identified in a Product-lens triad review
  and pulled into scope by explicit user decision. See Scope and
  `implementation/plan.md`'s Epic 2.3.

## Success Metrics

- `CreateSession` returns in low hundreds of ms for every session type
  (down from up to 150s for slow GitHub-URL resolutions), measured via the
  new tracing span/latency metric on the RPC itself.
- A new session appears in the session list with `SESSION_STATUS_CREATING`
  and a human-readable `creation_progress` message within ~1s of the user
  hitting Create, regardless of session type — verified via Playwright in
  Phase 6.
- A creation that fails during background resolution surfaces as: (a) a
  toast at the moment of failure, and (b) a persistent Failed status with an
  error message on the session card that remains after the toast dismisses.
- A session stuck in `Creating` past a configurable staleness threshold is
  automatically detected and flipped to Failed (with a distinguishable
  "orphaned/stale" reason) rather than hanging indefinitely, and this
  condition emits a metric.
- `create_session`/`create_session_for_pr` MCP tool callers see no
  regression in effective wait time in the **normal case**: the handler
  blocks internally until the instance reaches a terminal status, and the
  tool call returns a fully-created session or a clear error — behaviorally
  identical to today's synchronous return, while gaining the same
  correctness/consistency benefits (epoch-fenced terminal writes, idempotent
  cleanup, stale-timeout protection) as the web UI path, since both now
  route through the same `CreateSession` + Background Resolution Pipeline.
  Only in the **abnormal case** — the internal wait itself exceeds its own
  bounded timeout (`mcpAwaitTerminalTimeout`, see `implementation/plan.md`'s
  Epic 2.3) — does the tool call return early with a distinct "still
  creating" result (session ID included, for follow-up via `get_session`),
  which is strictly better than today's alternative of an open-ended hang
  past `createSessionTimeout`.
- Zero regression in existing synchronous fast-path behavior (duplicate
  title, invalid alias, missing path, etc. — see Non-Goals below) — these
  must still return a synchronous RPC error exactly as they do today.

## Appetite

Large (3–6 weeks)
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

## Constraints

- `CreateSession` is a live, continuously-used RPC — every teammate's
  running `stapler-squad` instance depends on it. No feature flag; rollback
  plan is `git revert` + redeploy (see Risk Control).
- Must preserve today's synchronous, fast-fail validation for cheap checks
  (missing title, missing path, duplicate title, resume_id format, fork
  source not found, alias not found) — these are cheap and users benefit
  from immediate, in-dialog error feedback rather than a Failed card.
- Must preserve `session/ent` schema/generation constraints from
  `CLAUDE.md` (`--feature sql/upsert`, generated code not committed).
- Must not break `session/backlog_plugin_github*.go` or other internal
  callers of the GitHub resolution helpers being touched.
- `create_session`/`create_session_for_pr`'s existing MCP-tool-specific
  behavior — path-traversal validation, title-collision pre-check, rate
  limiting (`createSessionLimiter`, 3/min, `server/mcp/rate_limiter.go`),
  MCP config injection, hook injection, and (for `create_session_for_pr`)
  existing-session-for-PR short-circuit — must be preserved even though the
  underlying instance-construction call changes. The MCP tool call itself
  must still behave as synchronous-until-terminal for its caller in the
  normal case (returns a fully-resolved session or a clear failure), even
  though it now internally routes through the fast-returning `CreateSession`
  RPC/pipeline rather than its own inline construction — with the single
  documented exception of the bounded-wait-timeout fallback described in
  Success Metrics above.

## Non-functional Requirements

- **Performance SLO**: `CreateSession` RPC p99 latency < 500ms after this
  change, for all session types (currently unbounded up to 150s for
  GitHub-URL sessions).
- **Scalability**: not applicable — single-user-per-instance, low session
  creation volume (not a throughput concern).
- **Security classification**: internal (local dev tool, keychain-backed
  credentials already handled elsewhere).
- **Data residency**: no special requirements.

## Scope

### In Scope

- Restructure `CreateSession` so that, for **every** session type, the
  `ManagedInstance` is created (status `SESSION_STATUS_CREATING`) and
  `SessionCreatedEvent` published *before* any potentially-slow resolution
  work (GitHub URL clone/fetch, alias/default resolution, branch/session-type
  inference, worktree setup, tmux startup) — with that work continuing in a
  background goroutine, detached from the RPC's request context/timeout,
  that updates the same instance in place via `creation_progress` messages
  as it advances through phases (e.g. "Resolving GitHub URL...", "Cloning
  repository...", "Setting up worktree...").
- Preserve today's synchronous fast-fail validations (see Constraints) —
  these still return a normal RPC error before any instance is created.
- New/extended status handling: a `SESSION_STATUS_FAILED` (or equivalent)
  outcome with an inline error message, surfaced via both a toast at
  failure time and a persistent state on the session card.
- Frontend: `Omnibar.tsx` stops awaiting full completion before closing —
  closes as soon as the RPC (now fast) returns with the placeholder
  instance, and shows any later failure via the toast + card mechanism
  above rather than blocking the dialog.
- Cancel-in-progress creation: allow deleting/cancelling a session still in
  `Creating` status, cleaning up any partial clone/worktree/tmux state.
- Retry failed creation: a retry action on a Failed session card that
  re-runs just the resolution/provisioning step in place, without
  re-entering the omnibar flow or creating a duplicate instance.
- Stale-creation detection: a background check that flips a session stuck
  in `Creating` past a configurable threshold to Failed with a distinct
  "stale/orphaned" reason (covers server-restart-mid-goroutine and similar).
- Observability: structured logs for each phase transition
  (Creating → resolving → Running/Failed) with session ID and per-phase
  timing; a metric for creation outcome (success/failed/stale) and
  duration; a tracing span around the background resolution goroutine
  consistent with however this repo's existing OpenTelemetry setup
  (`.claude/docs/opentelemetry.md`) is wired in, if applicable.
- **Route the `create_session` and `create_session_for_pr` MCP tools
  (`server/mcp/tools_lifecycle.go`, `server/mcp/tools_github.go`) through
  the same restructured `CreateSession` + Background Resolution Pipeline**,
  instead of their current separate, fully-synchronous
  `session.NewInstance`/`inst.Start(true)`/`store.AddInstance` path — so
  every session-creation entry point (omnibar, "New Session" dialog, MCP
  tools) shares one pipeline's correctness guarantees (epoch fencing,
  idempotent cleanup, stale-timeout protection, telemetry). Since an MCP
  tool call is expected to return once with a usable result, not leave the
  calling agent in limbo, the handler internally calls the new fast
  `CreateSession` to obtain an instance ID and then blocks (with its own
  bounded timeout) until the instance reaches a terminal status
  (`Active`/`Running` or `Failed`), returning a fully-resolved session or a
  mapped error in the normal case — not returning early with a bare
  `Creating` placeholder the way the web UI does. If that internal wait
  itself exceeds its own bounded timeout, the tool call falls back to a
  distinct "still creating" result rather than hanging indefinitely (see
  Success Metrics). See
  `implementation/plan.md`'s Epic 2.3 for the mechanism design.

### Out of Scope

- Changing how GitHub URL *detection* works in the omnibar (already fixed
  separately — see the `github.netflix.net` enterprise-host detection fix
  landed just before this project).
- Any change to the "New Session" dialog's own UI beyond what it inherits
  for free by calling the same `CreateSession` RPC.
- Retrying/cancelling sessions that are already `Running` (only applies to
  the `Creating`/`Failed` states introduced/extended here).
- Any change to `create_session`/`create_session_for_pr`'s MCP tool call
  signatures, input schemas, or success/error response shapes — the
  mechanism underneath changes, but the tool contract as seen by a calling
  agent does not, aside from the new (strictly-better-than-today) "still
  creating" fallback result on the rare internal-timeout path.
- Multi-instance/distributed coordination of stale-creation detection —
  single-process, single-instance is the only deployment model today.

## Rabbit Holes

- **Splitting the ~250-line `CreateSession` handler safely.** A large amount
  of downstream logic (defaults resolution, branch/session-type inference,
  worktree/tmux startup) currently depends on values computed during the
  synchronous resolution phase. Moving that resolution to "after instance
  creation" means restructuring data flow so the background goroutine has
  everything it needs, and so errors at any point in that chain map cleanly
  onto the Failed-status path instead of the old direct-RPC-error path.
  This is the largest single risk in the appetite.
- **Context lifetime for the background goroutine.** The RPC's context
  currently threads down into `ResolveGitHubInputCtxWithHosts` so a client
  disconnect/timeout cancels the underlying clone subprocess
  (`session_service.go:1915-1921`). Once resolution moves to a
  goroutine that must outlive the RPC, a new context strategy is needed
  (e.g. `context.WithTimeout(context.Background(), ...)` scoped to a
  reasonable max, with its own cancellation on explicit cancel-in-progress).
- **Retry-in-place vs. re-run-from-scratch.** Retrying a failed creation
  must not duplicate storage rows, re-publish spurious events, or leave two
  worktrees/tmux sessions if the failure happened after partial worktree
  setup. Idempotent cleanup-then-retry needs explicit design.
- **Stale-creation threshold tuning.** Too aggressive and a legitimately
  slow (but working) clone gets killed; too lax and orphaned sessions sit
  invisible for a long time. Needs to be informed by real observed
  clone/worktree/tmux-startup timings, not guessed.

## Alternatives Considered

- **Frontend-only fix** (close the omnibar immediately / fire-and-forget,
  add a client-side optimistic placeholder card): considered and rejected
  as the full solution, because it doesn't make the *real* session appear
  promptly in the list (the backend still doesn't create/publish the
  instance until after the slow clone) — it would only mask the dialog
  freeze without fixing the underlying "invisible until fully done"
  behavior the user actually asked to fix.
- **Scope to GitHub-URL sessions only**: considered; user explicitly chose
  to generalize the create-then-resolve-async pattern to all session
  creation paths for consistency, given the Large appetite.
- **Feature-flag the new path**: considered; user explicitly chose no flag,
  relying on git revert as the rollback mechanism.

## Feasibility Risks

- Restructuring a live-critical RPC's control flow risks regressing one of
  the many existing session-creation modes (directory, one-off, restart,
  fork, alias, autonomous, remote) — see `.claude/docs/session-creation-registry.md`'s
  7 touchpoints, all of which likely need re-verification.
- Background goroutines that outlive the RPC need careful lifecycle
  management (leak prevention on repeated failures, no goroutine pile-up
  across many quick creations) — existing async pattern at
  `session_service.go:2397-2413` (worktree/tmux startup) is the closest
  precedent and should be studied as the base pattern to extend rather than
  inventing a new one.
- Retry/cancel introduce new possible states/races (e.g. cancel arriving
  just as background resolution succeeds) that need explicit handling to
  avoid orphaned worktrees or double-published events.

## Observability Requirements

- Structured logs for every phase transition (`Creating` → per-phase
  progress → `Running`/`Failed`/stale-`Failed`), including session ID and
  per-phase duration, using the existing logging conventions in
  `server/services/session_service.go` (`log.Info`/`log.Error` calls
  already used throughout that file).
- A metric for creation outcome (success / failed / stale-timeout) and
  total creation duration, plus a tracing span around the background
  resolution goroutine — wired into whatever OpenTelemetry/Datadog setup
  already exists per `.claude/docs/opentelemetry.md`, rather than
  introducing a new observability stack.
- No new oncall alerting is assumed necessary for a local dev tool; the
  metric should be usable for local debugging (e.g. via the existing
  `--profile`/pprof or OTEL pipeline) rather than paging anyone.

## Risk Control

- No feature flag — user explicitly chose to rely on `git revert` +
  redeploy as the rollback procedure if the restructure misbehaves in
  production.
- Mitigate via thorough Phase 6 verification against
  `.claude/docs/session-creation-registry.md`'s 7 touchpoints and all
  session creation modes before shipping, since there is no flag to fall
  back on short of a full revert.

## Open Questions

- Exact shape of the new Failed status: is `SESSION_STATUS_FAILED` the
  right new enum value, or should this reuse/extend an existing status in
  `proto/session/v1/types.proto`? (Research phase to confirm current enum
  values and any existing failure-adjacent status.)
- Where should the stale-creation detector live — a ticker inside the
  existing session-management background loop, or a new dedicated
  goroutine started at server startup? (Architecture question for Phase 3.)
- What's a reasonable default staleness threshold, given no historical
  timing data exists yet for GHE clones specifically? **Resolved by user
  decision (unresolved after Phase 2 research — no empirical GHE clone
  timing data existed to derive this from): default to 10 minutes,
  config-overridable.**
- How does retry interact with the `SessionCreatedEvent` stream — does
  retry re-publish an event, or is the existing instance's status change
  (Failed → Creating → Running) sufficient for `WatchSessions` subscribers
  to pick up without a new event type?
- What should the MCP path's internal bounded-wait timeout
  (`mcpAwaitTerminalTimeout`) be? **Resolved during plan.md's Epic 2.3
  design: 150s, matching the existing `createSessionTimeout` this replaces
  — see `implementation/plan.md`'s Domain Glossary for the full
  justification.**
