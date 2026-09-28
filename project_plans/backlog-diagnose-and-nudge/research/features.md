# Research: Feature Landscape — backlog-diagnose-and-nudge

Agent 2 (Features), Phase 2. Scope: what similar mechanisms already exist in this
codebase (the strongest signal — this is a mature, already-self-healing system),
what exists in industry, edge cases, and Tyler's unstated needs.

## 1. Prior-art status check: `superseded-rework-session-retirement`

Requirements.md's "PRIOR ART" note asked whether the 8 named `spawnSessionAfterGates`
bypasses (`AttachSessionToItem`, review-gate spawn, `TriggerReReview`, triage, manual
verdict, Jules reservation, `RemediateStaleWorkSession`, `forceResetItem`) were closed.

**Verified by grep, 2026-09-27**: none of the 8 call `archiveItemWorkSessions` (or any
archive helper) directly — they remain proactive-archive bypasses at the point of spawn.
`git log` shows only one relevant follow-up commit,
[`02bac89d2`](server/services/backlog_service_lifecycle.go) ("route
OverrideVerdict/AttachSessionToItem transitions through the injected engine"), which
fixed a *different* bug (bypassing `ConfiguredWorkflowEngine`'s transition rules), not
archival.

**But the shipped fix's actual design absorbs this gap by construction.** PR #808
(`7cb2c74f4`, "feat(backlog): archive superseded rework-round sessions") didn't just add
per-site archive calls — it also added `server/services/superseded_session_sweeper.go`,
a 60s ticker wired into `server/server.go:1209`
(`services.NewSupersededSessionSweeper(deps.Storage, deps.SessionService)`, confirmed
live, not dead code). Its `sweep()` scans every `in_progress`/`review` item's
`ListItemSessions` history, and `findSupersededSessions`
(`server/services/backlog_service_triage.go:1320`) picks the non-current round **per
item+role by `CreatedAt`, regardless of which code path created it**. It never
hard-kills — `archiveIfNotLive` skips anything `IsSessionLive`, so a bypass session a
user is actively viewing survives until it goes idle, then gets swept next tick. This
means **all 8 named bypasses are already covered as a defense-in-depth backstop**, just
not proactively at spawn time, and only while the item is `in_progress`/`review` (not
`done`/`archived` — that's `reconcileTerminalItemSessions`'s separate, already-shipped
job, `session/backlog_lifecycle_archive.go:110-146`).

**Implication for this feature's "stale retry-session cleanup" scope**: don't build a
parallel sweeper or a parallel "which round is current" definition. The new
handoff-then-cleanup mechanism should call the *same* primitives
(`ArchiveSessionByUUID`/`SetArchivedAtIfNilAndStop`, `KillTmuxPaneOnly` — never
`StopSessionByUUID`, which runs `CleanupWorktree` and would delete the shared worktree
out from under a still-current round, per `superseded-rework-session-retirement`'s hard
safety constraint) and should feed `findSupersededSessions`'s existing tie-break, not
invent a third "is this session current" predicate. The one gap worth closing
proactively: `superseded_session_sweeper.go`'s own doc comment names archive-time
best-effort failure as gap #1 it exists to heal — if diagnose-and-nudge's own new
archive-then-handoff path needs the same best-effort tolerance, reuse this sweeper as
the safety net rather than adding local retry logic.

A second, separately-shipped safety layer is directly relevant too: PR #810
(`c2a30a8b9`, "fix(session): never auto-restore, revive or retry an archived session")
added `Instance.IsArchived()` (`session/instance_state.go`) and wired it into five
auto-lifecycle guard points (health checker, cold restore, hot restore, poller revival,
session driver retry) per `project_plans/superseded-rework-session-retirement/implementation/plan.md`.
Its own `follow-ups.md` records an **explicitly deferred** item worth surfacing to the
plan phase: no lint rule yet enforces that a *future* sixth auto-restart path also
checks `IsArchived()` — "an automated start has no syntactic marker" was the reason it
was deferred, not solved. If diagnose-and-nudge adds any new automated-restart-adjacent
code path (e.g. auto-resuming a session as part of the handoff), it must call
`IsArchived()` itself; nothing will catch a miss.

## 2. Existing autonomous-nudge precedent: PR-fix steering

`server/services/backlog_service_pr_fix_steer.go` is a fully-shipped instance of exactly
the "fully autonomous nudging, no human approval gate" pattern this feature needs, one
project-memory entry deep already (`project_pr_fix_steering_learnings.md`). Read in
full; the reusable pieces:

- **Identity/readiness re-check immediately before the write**, not just at dispatch
  time: `steerActiveSessionForPRFix` (`:245`) calls `s.sessionSteerer.IsReadyForSteer(activeSessionUUID)`
  (`server/services/session_service.go:1066`) right before `SteerActiveSession`, with the
  comment "Security: fixContext is unauthenticated GitHub PR/review content written
  verbatim into the PTY. Never deliver it unless the pane is confirmed idle — a
  busy/unknown state must degrade, not guess." This is the concrete precedent for this
  feature's "target session must be `detection.StatusIdle` AND identity-reverified
  immediately before the write" requirement — don't design a new gate, call this one (or
  its logical successor) at the same call-site position.
- **`IsReadyForSteer` is stricter than raw `StatusIdle`.** It requires `IsControllerActive`,
  zero `QueuedCommands`, no in-flight command, **and** `isSafeSteerStatus` — which itself
  requires `StatusIdle` *and* the status-context description to be on a hardcoded
  `safeIdleStatusContexts` allowlist (`claude_readline_prompt`, `claude_shortcuts_prompt`,
  `claude_accept_edits`) pinned against `TestSafeIdleStatusContexts_MatchClaudeIdlePatternDescriptions`.
  This directly operationalizes the requirements' "Rabbit Hole": distinguishing "safe to
  nudge" (idle) from "actively working, just slow" (also looks idle) — `StatusIdle` alone
  is documented as NOT sufficient here (`command_prompt`/`vim_normal_mode` share the same
  `DetectedStatus` value but mean a raw shell/editor prompt where injected text would be
  misread as a literal command). Project memory (`instinct_detection_status_ready_dead_code.md`)
  already flags `StatusReady` as dead code with the same StatusIdle-context-allowlist
  gating pattern as the fix — this feature's design doc should point at `isSafeSteerStatus`
  by name rather than re-deriving a status predicate.
- **Concurrency guard**: `steerInFlight.LoadOrStore(itemID, struct{}{})` /
  `defer s.steerInFlight.Delete(itemID)` — guards the whole steer body against a 60s
  reconcile tick and a webhook goroutine racing for the same item. A diagnose dispatch
  will have an equivalent race (a manual "Diagnose" click racing an automated poller);
  reuse this idiom rather than a bespoke mutex.
- **Cooldown/dedup, not just a hard cap**: `isDuplicateSteerReason` +
  `steerDedup`/`steerConflictDebounce` sync.Maps suppress re-sending semantically
  identical steer content within `steerCooldown`. This is a second, complementary
  precedent to the requirement's named "AutonomousDriver turn-cap" one — worth deciding
  explicitly in the plan phase whether the new nudge cap is a hard per-item counter
  (AutonomousDriver-style, `maxTurns` default 20, in-memory, not live-configurable) or a
  cooldown-plus-cap (PR-fix-steer-style, content-aware dedup). The requirements ask for
  "configurable... mirroring AutonomousDriver's turn-cap precedent" but
  `effectiveReworkCap` (`server/services/backlog_service_triage.go:92`, see §3) is the
  actually-live-settable-per-item-override precedent already in this codebase
  (`BacklogItemData.ReworkCapOverride`, global default via
  `config.Config.MaxAutoReworkIterationsOrDefault()`, Settings → Defaults UI) — closer to
  what the "rollout flags: live-settable, no env vars" project-memory rule demands than
  AutonomousDriver's constructor-baked constant is.
- **Explicit degrade path, not silent failure**: every unsafe/unready condition routes to
  `degradeToRespawnBlocked` (`:319`), which falls back to the pre-existing notify-only
  behavior rather than erroring. Diagnose-and-nudge's "post a diagnostic note when
  inconclusive" scope item is this same shape — model it on `degradeToRespawnBlocked`.

## 3. Existing rework-cap and notification precedent (as requested)

`server/services/backlog_service_triage.go`:

- `effectiveReworkCap(item)` (`:92-100`): per-item override (`BacklogItemData.ReworkCapOverride`,
  `0` meaning unlimited via `math.MaxInt`) falling back to
  `s.maxAutoReworkIterations()` → `config.Config.MaxAutoReworkIterationsOrDefault()`
  (default 3, Settings → Defaults UI). Four call sites compare a per-role session count
  against this cap: `AutoReopenAfterFailedReview` (`:1909`), `AutoRespawnAutonomousWork`
  (`:2061`), `AutoReopenForPRFix` (`:2275`), `AutoRespawnReview` (`:2426`) — each on cap
  hit calls `notifyReworkCapHit` and leaves the item for manual action rather than
  looping forever. This is the exact shape a "diagnostic dispatch cap" should copy: a
  config-backed, per-item-overridable int, not a hardcoded constant.
- `notifyReworkCapHit` (`:169-197`) is the canonical "surface an automated decision to
  Tyler" pattern requested for investigation: (1) durable state via
  `s.storage.MarkStuck(ctx, itemID, domain.StuckReasonReworkCap, currentStatus, <message>)`
  then `MarkStuckNotified` — the write is unconditional and independent of whether the
  live toast succeeds ("must never suppress the notification itself"); (2) live event via
  `s.eventBus.Publish(events.NewNotificationEvent(itemID, "", uuid.New().String(), ...))`.
  `itemID` is deliberately threaded as the event's `sessionID` field (not just metadata)
  specifically so `server/notifications/subscriber.go`'s coalescing key
  (`sessionID:notificationType`) doesn't merge two different items' same-type
  notifications within its 500ms window — a bug the comment documents was already hit
  once (`backlog_notifier.go:22-29`). Every new notification this feature emits
  (dispatch started, bug filed, nudge sent, cap hit, inconclusive) must follow this same
  two-part durable-plus-live shape and the same itemID-as-sessionID convention.
- `EventBusNotifier` (`server/services/backlog_notifier.go`) is the adapter — `session`
  package can't import `pkg/events` directly (import cycle), so this bridges
  `session.Notifier` to `*events.EventBus`. A new diagnostic-agent notifier should extend
  this adapter, not create a parallel one.

## 4. MCP tool surface a diagnostic agent would actually call

From `server/mcp/tools_backlog.go`, `tools_backlog_pr.go`, `tools_lifecycle.go`,
`tools_terminal.go`:

| Tool | Key params | Design-relevant constraint |
|---|---|---|
| `create_backlog_item` | `title*`, `description`, `acceptance_criteria[]`, `priority` (1-5), `category` (enum), `repo_path`, `base_branch`, `notes`, `skip_triage` | Not role/item-gated — any session may call it. Requirements say "same evidence bar as prior bug ce71ad1a" — the evidence bar is enforced by convention/prompt design, not by this tool's schema, which accepts a bare title with everything else optional. |
| `post_backlog_update` | `item_id*`, `message*` (max 2000 chars), `session_id` (attribution override) | Explicitly "not an official verdict... never changes item status" — exactly the right tool for the "inconclusive" branch. Also not role/item-gated. |
| `report_duplicate` | `item_id*`, `duplicate_ref*` (GitHub URL, max 500 chars), `reason*` (max 1000 chars) | Two modes gated on whether the caller session is the item's assigned work session; **both modes require GitHub-verifiable existence of `duplicate_ref` before any state change** — not directly relevant to diagnose-and-nudge's scope (it's about closing an item as a dup of other *work*, not diagnosing a stuck session) but is the closest existing model for "verify before acting" gating a diagnostic agent's own bug-filing should mirror. |
| `resume_session` | `session_id*` (title, not UUID) | **Only for a *paused* session** — "Recreates the git worktree and restarts the tmux session." Distinct semantics from nudging a live-but-idle session. If the target session is `Paused` (not just idle), `resume_session` is the correct tool; if it's `Active`/idle, `write_to_session`/`steer_session` is correct instead. The diagnostic agent's tool choice must branch on session status, not always reach for one tool. |
| `steer_session` | `session_id*` (title), `message*` (max 4096 bytes) | Always appends newline; **not rate-limited** ("high-level semantic operation"); this is the `SteerActiveSession`/`IsReadyForSteer`-backed path (§2) — the natural default for "nudge." |
| `write_to_session` | `session_id*` (title), `input*` (max 4096 bytes), `press_enter` (default true) | Raw, unfiltered PTY write, fire-and-forget, **rate-limited to 1/sec/session**. Lower-level than `steer_session`; use only if steer's semantics (always-newline, "high-level") don't fit. |

Two cross-cutting gotchas for the diagnostic agent's tool use: (1) session-scoped
lifecycle tools (`resume_session`, `write_to_session`, `steer_session`) all key on
**title**, not UUID, while the context bundle will naturally carry the session's UUID
(`Instance.UUID`/`Snapshot()`) — the diagnose service must resolve UUID→title before
handing the diagnostic agent a tool call, or hand the agent the title directly rather
than making it guess. (2) None of these tools independently re-verify archived/superseded
status — `steer_session`'s underlying `IsReadyForSteer` does *not* check `IsArchived()`
(confirmed: `session_service.go:1066-1079` checks liveness/queue/status only). A
diagnostic agent could in principle nudge an archived zombie session if the context
bundle handed it a stale session_id — the "identity-reverified immediately before the
write" requirement should explicitly include an `IsArchived()` check alongside
`IsReadyForSteer`, since the underlying primitive doesn't already do it for you.

## 5. Async generation + staleness precedent: `HandoffSummaryGenerator`

`session/handoff_summary_service.go`'s `HandoffSummaryGenerator` is the exact shape the
"old session generates handoff summary → hands to new session → old session torn down"
requirement needs, and it's already production code, not a stub:

- `BeginGeneration`/`GenerateAndPersist` (`:307`, `:344`) is an acquire-lock/async-generate/
  persist pattern with `tryAcquire`/`isInFlight` (`:123`, `:146`) guarding against a
  double-generate race.
- `ReconcileStaleness` (`:168`) explicitly handles a **stuck `GENERATING` row** — this is
  the feasibility risk named in requirements.md ("HandoffSummaryGenerator has a 60s
  timeout, resolves to ERROR") already solved once. The new "stale retry-session cleanup"
  path should call this generator directly rather than re-implementing timeout/ERROR
  handling, and its teardown ordering ("summary must be durably persisted before old
  session teardown") should gate on this same generator's terminal-state check
  (`FindRowBySessionID` returning a non-`GENERATING`, non-transient status) before
  proceeding to `KillTmuxPaneOnly`.
- There is an open bug directly on this exact seam:
  `docs/bugs/open/BUG-092-handoff-summary-terminal-status-race-under-ci-load.md` — worth
  reading in the pitfalls/architecture research pass, since it's a race in the same
  terminal-status-transition machinery this feature's cleanup ordering depends on.

## 6. Industry analogues

- **Self-healing CI / auto-retry bots** (e.g. GitHub's own flaky-test auto-rerun,
  Spinnaker's automated rollback): the common failure mode this codebase has already hit
  once and fixed (superseded-rework-session-retirement) — a "retry" mechanism that
  doesn't know when to stop retrying a fundamentally-dead unit of work, producing
  resource-exhaustion incidents. The lesson already internalized here (rework caps +
  `notifyReworkCapHit` degrade-to-manual) is the standard mitigation; diagnose-and-nudge
  is functionally a **triage layer on top of an existing self-healing layer**, so its own
  cap must compose with, not race, `effectiveReworkCap`'s cap — a diagnostic nudge that
  itself spawns a rework round could double-count against or bypass that cap if not
  wired through the same counter.
- **PagerDuty/Opsgenie auto-remediation runbooks**: the standard safety pattern is
  "verify precondition → act → verify postcondition → log," with an explicit
  circuit-breaker (stop auto-remediating after N consecutive failures on the same alert
  class) distinct from a simple retry cap — this maps onto the "Nudge safety gates" +
  "Configurable nudge cap/cooldown" requirement, but note PagerDuty-style tools also
  typically **page a human on circuit-breaker trip**, which `notifyReworkCapHit`'s
  pattern already covers (durable `MarkStuck` + live notification) — no new alerting
  channel needed.
- **Sentry/Rollbar auto-triage (issue grouping + auto-assign)**: the closest analogue to
  "file a bug with the same evidence bar as ce71ad1a" — those tools' auto-created issues
  are notoriously noisy when the grouping heuristic is too loose, and teams often mute
  auto-filed issues wholesale after a few false positives erode trust. This is a strong
  argument for treating the "same evidence bar as ce71ad1a" instruction as a hard
  precondition on `create_backlog_item` calls (require the diagnostic agent to cite
  specific log lines/session state as evidence in the description, not just "session
  seems stuck"), not a soft prompt suggestion — the cost of a wrong auto-filed bug is
  Tyler's trust in every subsequent auto-filed bug.
- **Dependabot/Renovate auto-merge**: relevant mainly as a *contrast* — those tools farm
  out the "is this safe" decision to a battery of required CI checks before ever acting
  unattended. This feature has no equivalent gate before nudging (by explicit requirement
  — human-approval gate declined), so the `detection.StatusIdle`-plus-context-allowlist
  check (§2) is effectively this feature's whole precondition battery; it should be
  treated with the same rigor a merge gate would get, not as a minor implementation
  detail.

## 7. Edge cases and failure modes the design must handle

1. **TOCTOU between context-bundle assembly and the nudge write.** The context bundle is
   assembled first (potentially slow — token-budget compaction, git diff, log reads), then
   a dispatched agent decides, then it nudges. The target session's state can change in
   that window (goes from idle to busy, gets archived by the sweeper in §1, or gets
   superseded by a brand-new rework round). This is exactly why "identity-reverified
   immediately before the write" is in scope — but per §4, that reverification must
   include `IsArchived()`, which `IsReadyForSteer` doesn't check today.
2. **The diagnostic agent's own session needs the same safety net it's nudging others
   with.** It's itself a Stapler Squad session, subject to the same health-checker/
   poller/driver lifecycle as §1's guards. If it hangs or gets killed mid-diagnosis, does
   it leave the target item in a half-nudged state (e.g. `post_backlog_update` posted but
   no structured "dispatch outcome" log line)? The structured-logging requirement should
   include a dispatch-start log distinct from the outcome log, so an incomplete dispatch
   is itself detectable (mirrors the "second line of defense" instinct from §1).
3. **MCP server disconnection mid-diagnosis** (explicitly named feasibility risk,
   observed this session as ECONNREFUSED). A dispatched diagnostic agent whose MCP tools
   vanish mid-call will get a tool-call error, not a graceful "inconclusive" outcome,
   unless the dispatch wrapper catches this specifically and posts a `post_backlog_update`
   note (or a `notifyReworkCapHit`-style durable record) saying diagnosis failed due to
   infra rather than silently vanishing. Needs a documented, tested failure path, not just
   a mention in the requirements doc.
4. **Nudging a session that's genuinely "working, just slow" vs. actually idle** (named
   rabbit hole) — `isSafeSteerStatus`'s allowlist (§2) is the existing mitigation but is
   pinned to Claude Code's specific idle-prompt wording; a session running a
   different program (`isClaudeCodeProgram` check already exists in
   `backlog_service_pr_fix_steer.go:180` for exactly this reason) needs the same guard
   reused, not re-derived.
5. **Compaction budget exceeded mid-bundle-assembly for a genuinely enormous stuck
   session** (e.g. one with hundreds of rework rounds' worth of history) — the
   requirements say compact via native `/compact` + `HandoffSummaryGenerator`, "not
   truncation," but `HandoffSummaryGenerator` has its own 60s timeout (§5) that could
   itself trip while assembling a bundle for a *different* target session than the one
   the timeout was designed around (target-session summarization vs. the new diagnostic
   agent's own context growth — the plan explicitly warns these are not the same thing;
   worth stress-testing that the same 60s ceiling is acceptable for both uses or whether
   the bundle-assembly path needs its own timeout).
6. **Stale-retry-session cleanup racing the SupersededSessionSweeper.** If diagnose-and-
   nudge's own cleanup archives/tears down a superseded retry session at the same moment
   §1's 60s sweeper independently decides the same session is superseded, both call
   `ArchiveSessionByUUID`/`KillTmuxPaneOnly` — these are documented as idempotent/CAS-safe
   (`archiveIfNotLive`'s doc comment), so this is likely safe already, but the plan phase
   should state that explicitly as a verified invariant rather than an assumption.

## 8. Tyler's unstated needs beyond the explicit requirements

- **Trust calibration, not just automation.** The problem statement's success metric is
  "number of times Tyler personally has to manually diagnose... trends toward zero," but
  the secondary metric (dispatch outcomes: bug filed / nudge succeeded / inconclusive) is
  really asking for a track record Tyler can audit — per the Sentry/Rollbar analogy (§6),
  a few bad auto-filed bugs will cost more trust than they save time. The design should
  make it trivial to see, per dispatch, which of the three outcomes fired and why,
  probably as a visible list on `StuckItemDetail`/`BacklogItemDetail`, not buried in logs
  only — this is implied by "structured logging for every dispatch/nudge/bug-filed/cap-hit
  event" but the requirement as written doesn't say *where Tyler sees it*, only that it's
  logged.
- **A way to tell the diagnostic agent "no, don't nudge this one again."** The rework-cap
  precedent (§3) has a per-item override (`ReworkCapOverride`) settable via Settings →
  Defaults. Given "document AI decisions in edge cases" and "no ad-hoc PRs from main
  session" project-memory patterns (Tyler consistently wants a visible, reviewable trail
  for autonomous actions), a per-item "diagnose cap" override and/or a way to silence
  future auto-dispatch for one item (distinct from the global feature flag) is a
  plausible unstated need — worth asking in the plan/validate phase rather than assuming
  the global flag is the only control surface.
- **Consistency with the already-declined human-approval gate.** Requirements explicitly
  decline a human-approval gate before nudging, but Tyler's `feedback_document_ai_decisions_in_edge_cases.md`
  memory item says "self-heal/auto-close actions should post a visible comment +
  notify(), not act silently" — the design should treat *every* branch (bug filed, nudge
  sent, AND inconclusive) as requiring a `post_backlog_update`-or-equivalent visible trail,
  not just the inconclusive branch as literally scoped. A silent successful nudge is still
  an autonomous write Tyler didn't approve; make it visible even when it works.
- **Mobile/desktop parity for the new "Diagnose" button** (project memory:
  `feedback_mobile_desktop_ux.md` — always consider both form factors). Not mentioned in
  requirements.md at all; `StuckItemDetail.tsx`/`BacklogItemDetail.tsx` both need the
  affordance to work with touch targets and on small screens, since this is a UI surface
  addition, not just backend orchestration.
