# Requirements: session-resume-uuid-loss

**Date**: 2026-08-07
**Type**: bug fix
**Complexity**: 2 — focused feature (root-cause fix + recovery fallback + UI signal)

## Problem Statement
When a session's tmux pane is dead (killed by inactivity-timeout restart, service
restart, or hibernation) and the session is revived, `startLocked`/`start` in
`session/instance.go` decides whether to relaunch Claude with `--resume <uuid>` based
on `i.HasClaudeSession()` (in-memory `claudeSession.ConversationUUID`). If that
in-memory UUID is empty — even though a JSONL conversation history file for this
project path exists on disk — the revive silently launches a **brand-new Claude
process with no `--resume` flag**, discarding the working conversation. The only
signal is a single `log.Warn("cold start: tmux dead, no conversation UUID, starting
fresh", ...)` line; nothing surfaces to the user or the UI.

## Baseline
Today, `tryExtractConversationUUID()` (`session/instance_claude.go:308`) already
supports two recovery strategies: a live-process open-file scan (`Detect`, requires
`pm().IsAlive()`) and a path-based scan of `~/.claude/projects/<encoded-path>/` for
the most-recently-modified JSONL (`DetectByPath`, works even when the tmux pane is
dead). However, in the cold-restart path
(`session/instance.go:845-935` `startLocked`, mirrored at `:1023-1145` `start`), the
launch command is already built by `i.initTmuxSession()` (line 858/1040) — which
reads `i.claudeSession.ConversationUUID` via `ClaudeCommandBuilder.Build()`
(`session/claude_command_builder.go:35`) — **before** the `!firstTimeSetup` /
`!pm().IsAlive()` branch runs. `tryExtractConversationUUID()` is only called *after*
`pm().Start()` has already launched the (already-command-committed) fresh process
(`instance.go:906-921`), and by then the code has additionally just cleared
`claudeSession.ConversationUUID = ""` unconditionally (lines 910-913) — so recovery
at that point can only fix bookkeeping for the *next* restart, not the process that
was just started. Net effect: `DetectByPath` recovery exists but runs one step too
late to influence whether `--resume` is used. Users currently have no way to tell,
short of reading server logs, that a revive silently dropped their conversation;
they discover it only when the agent "forgets" prior context.

## Users / Consumers
- End users driving Claude Code sessions through stapler-squad's web UI / omnibar,
  across all session types affected by tmux-dead revival (directory, worktree,
  one-off). Confirmed observed on one-off sessions on macOS v1.40.0; the code path
  is shared by all session types (`session/instance.go`'s `start`/`startLocked` is
  session-type-agnostic).
- Indirectly: the `SessionDriver` inactivity watchdog and hibernation sweeper, whose
  restarts are the most common trigger of this code path (see Related, below).

## Success Metrics
- A revive where a JSONL history file exists for the session's project path
  resumes with `--resume <uuid>` instead of starting fresh, even when the in-memory
  UUID was never captured or was cleared by a prior restart — measured by: cold
  restart integration test asserts the launched command contains `--resume` when a
  JSONL fixture is present on disk and no in-memory UUID exists.
- When recovery genuinely fails (no JSONL exists at all — a real fresh start), the
  event is visible somewhere a user will see it (UI toast/badge or session
  metadata), not only a log `WARN` — measured by: a session-level field/event exists
  and is asserted by test; manual verification the web UI surfaces it.
- No regression to the existing live-process fast path (`Detect`) or to normal
  warm-tmux revival (`pm().IsAlive() == true` path is untouched).

## Appetite
Small (1–2 days) — this is a reordering of an existing, already-implemented
recovery mechanism (`DetectByPath`) plus a small UI/state signal, not new
detection logic.

## Constraints
- Must not change behavior for the common case (tmux alive → `RestoreWithWorkDir`)
  or for `firstTimeSetup` (brand-new session, correctly has no prior UUID).
- Must not introduce a live-process dependency into the pre-launch path — the whole
  point is this runs *before* the new tmux session/process exists, so only the
  path-based (`DetectByPath`) strategy is available at that point, not the
  open-file-scan (`Detect`) strategy.
- Two near-duplicate code blocks currently implement this logic
  (`startLocked` in `instance.go:845-1007` and legacy `start` in
  `instance.go:1023-1145`+). Both must be fixed consistently — check whether `start`
  is still a live call path (used by `StartWithCleanup`) or dead code before
  deciding whether to fix one or both.

## Non-functional Requirements
- **Performance SLO**: not specified — `DetectByPath` is a single `os.ReadDir` +
  stat-sort over one project directory; negligible added latency on an already
  multi-second cold-restart path.
- **Scalability**: not applicable.
- **Security classification**: internal (local dev tool, single-user state dir).
- **Data residency**: not applicable.

## Scope
### In Scope
- Move (or add) a path-based UUID recovery attempt (`DetectByPath`) to run *before*
  `i.initTmuxSession()` builds the launch command in the cold-restart branch of
  `startLocked` (and `start`, if still live), so a recovered UUID is honored by
  `ClaudeCommandBuilder` and the process launches with `--resume`.
- Ensure the recovered UUID, once found, is persisted (`SetClaudeConversationUUID`
  or equivalent) so it isn't immediately overwritten by the existing
  `claudeSession.ConversationUUID = ""` reset that runs later in the same function.
- Add a visible signal (session field, event, or log promoted to a
  UI-surfaced state) when a revive falls through to a genuine fresh start despite
  the recovery attempt, distinguishing "recovered and resumed" from "no history
  found, truly fresh" from "history found but recovery failed" (e.g. corrupt/
  unreadable JSONL).
- Unit/integration test coverage for: (a) in-memory UUID present → unchanged
  `--resume` behavior; (b) in-memory UUID absent but JSONL exists on disk →
  recovered and resumed; (c) in-memory UUID absent and no JSONL exists → fresh
  start, with the new visible signal set.

### Out of Scope
- Changing `driverInactivityTimeout` (10-minute hardcoded watchdog) or adding a
  config override for it — noted as related upstream churn but a separate item.
  (Flagged below under Related / Alternatives; not fixing here.)
- Investigating why UUID capture is racy/inconsistent *during* live sessions
  (`instance_workspace.go`'s post-start capture, `history_linker.go`) — this item
  only fixes the revive-time fallback, not live capture timing.
- Building a general "conversation recovery UI" beyond a minimal visible signal.

## Rabbit Holes
- The `startLocked` vs. legacy `start` duplication: if `start` is dead code (only
  `startLocked` is reachable via the actor), fixing both is wasted effort; if both
  are live, missing one leaves the bug half-fixed. Must resolve this in research,
  not assume.
- `ClaudeCommandBuilder.Build()` treats an invalid-format UUID as "no session" and
  silently drops to a fresh start with only a `log.Warn`. If `DetectByPath` is
  moved earlier, a malformed/unexpected JSONL filename could still hit this same
  silent-drop path — the visible-signal requirement should cover this case too,
  not just "no JSONL found at all."
- Multiple JSONL files can exist per project dir if a project path is reused across
  multiple stapler-squad sessions (e.g. two one-off sessions sharing a base dir is
  unlikely given `namegen.GenerateUnique`, but directory/worktree sessions revisiting
  the same path over time could accumulate several conversation files).
  `DetectByPath` already picks the most-recently-modified one — confirm this
  heuristic is still correct for the pre-launch-recovery use case, not just the
  post-launch one it was originally written for.

## Alternatives Considered
- **Persist UUID more durably / eagerly** (requirements doc's suggestion #2): would
  reduce how often recovery is needed at all, but doesn't eliminate the race
  (a restart can always land in the capture gap) — treated as a complementary
  hardening, not a substitute for fixing the ordering bug, and larger in scope
  (touches every UUID-capture call site) than the Small appetite here allows.
- **Refuse to cold-start at all when history exists but UUID missing** (docs's
  suggestion #1, stricter form: block/error instead of recovering): rejected as the
  primary fix — `DetectByPath` already gives us a good-enough automatic recovery
  path most of the time; blocking would trade silent data loss for a worse UX
  (session stuck needing manual intervention) in the common case where recovery
  would have succeeded anyway. Reserved as the fallback behavior *only* when
  recovery itself fails.

## Feasibility Risks
- `DetectByPath` resolves the project directory via `ClaudeProjectDirName(effectivePath)`
  where `effectivePath = i.GetEffectiveRootDir()`. Must confirm this is available
  and correct at the point-in-time we'd need to call it (before worktree/path
  resolution currently sequenced later in some branches) — needs verification in
  research, not assumed.
- Since `session/instance.go`'s `start`/`startLocked` are large, actor-model,
  multi-branch functions with existing heavy comment-documented lock-ordering
  subtleties (see `ClearConversationState`'s comments on `claudeSessionMu`/`i.mu`
  ordering), moving code within them carries real regression risk despite the
  small apparent diff — needs a plan that respects existing lock/actor
  conventions, and existing tests must be run, not just new ones added.

## Observability Requirements
*(complexity 2, but flagging per existing project convention of visible AI/recovery
decisions — see `feedback_document_ai_decisions_in_edge_cases` memory)*
The recovery outcome (recovered-and-resumed / no-history-found-fresh-start /
recovery-attempted-but-failed) should be logged at `Info`/`Warn` as today, plus
surfaced via a session-visible field so the web UI (or at minimum
`get_session`/`list_sessions` output) can show it — exact UI treatment left to
Phase 3 planning judgment (badge, toast, or session detail field are all
acceptable; a new metric/alert is not required for this Small-appetite fix).

## Risk Control
Not needed — this is a pure bug fix narrowing an existing silent-data-loss window;
no feature flag needed. Standard test coverage (see Scope) is the risk control.

## Open Questions
1. Is legacy `start()` (`instance.go:1023-`) still a reachable call path, or has it
   been fully superseded by the actor's `startLocked`? (Answer determines whether
   both blocks need the fix or just one.)
2. What does `i.GetEffectiveRootDir()` return at the earliest safe point in
   `startLocked`'s cold-restart branch — is it already resolved before
   `i.initTmuxSession()` runs, or does resolving it early change worktree-path
   behavior?
3. What's the right shape for the "visible signal" — a new `Instance` field
   persisted to storage (`session/storage.go`), a lifecycle event
   (`fireLifecycleEvent`), or reuse of an existing session-status/notification
   mechanism? Needs a look at how similar recoverable-but-notable conditions are
   already surfaced (if any) before inventing a new mechanism.
</content>
