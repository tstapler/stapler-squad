# Research: Feature Landscape for Async Session Creation

## 1. The existing async pattern to extend (`session_service.go:2397-2482`)

`CreateSession` (`server/services/session_service.go:1799`) already runs **half** of an
async-creation pattern, but only for the *tail* of the work — worktree/tmux startup — not
the *head* (GitHub URL resolution, alias/default resolution, branch inference). Today's flow:

1. Lines 1806–1938: **all** synchronous — fast-fail validation, title-uniqueness check,
   fork dispatch, restart-source resolution, GitHub URL detection + clone
   (`session.ResolveGitHubInputCtxWithHosts`, threaded with the RPC's `ctx` so a client
   disconnect cancels the clone subprocess), one-off directory generation.
2. Only *after* all of that does instance construction happen (not shown in the excerpt
   read, but implied — instance exists by line ~2370).
3. Line 2380: `s.eventBus.Publish(events.NewSessionCreatedEvent(instance))` — publishes
   Creating-status instance.
4. Lines 2382–2482: `s.trackCleanup(func() { ... })` — a goroutine (not a bare `go func()`)
   that does the *actual* `instance.Start(true)` (tmux + worktree), hook injection, status
   manager wiring, autonomous driver startup, then `SaveInstances` + a final
   `SessionUpdatedEvent` publish. Progress is surfaced via
   `instance.SetCreationProgress(msg)` + a `SessionUpdatedEvent{"creation_progress"}` publish
   before the goroutine starts real work, and cleared (`SetCreationProgress("")`) on success.
5. On failure inside the goroutine: `instance.SetCreationProgress("Startup failed: ...")`,
   `instance.ForceStatus(session.Stopped)`, save, publish. **Note: failures land on
   `Stopped`, not a distinguishable Failed state** — this is exactly the gap the new
   `SESSION_STATUS_FAILED` work needs to close.
6. The RPC returns `creatingProto` — an `InstanceToProto` snapshot taken *before* the
   goroutine starts, specifically to avoid a race with the goroutine's own
   `SetCreationProgress` calls (see the comment at line 2386).

**Why this is the base pattern, not a template to copy blindly**: `trackCleanup` (not
`go func()`) is used because it's tracked by `deleteCleanupWG`, so `Shutdown()` blocks until
the goroutine finishes — this is what currently prevents goroutine leaks across
server-restart/test-teardown. The new background-resolution goroutine (now doing GitHub
clone + alias/branch inference *before* worktree start) must be tracked the same way, or the
existing shutdown-blocking guarantee silently stops covering the (now largest, slowest) part
of session creation. `trackCleanup`'s doc comment (`session_service.go:320-329`) also
explains the exact regression this pattern was hardened against
(`TestCreateSession_should_ComposeProfileCLIFlagsBeforePresetExtraArgs_When_BothPresent`
flaking from a goroutine outliving its test's tempdir).

**The restructuring task**: currently steps 1(GitHub resolution)→2(instance
construction)→3(publish)→4(goroutine: worktree/tmux) run in that order, with resolution
*before* the instance exists. The requirement asks to move construction+publish to before
step 1, and merge steps 1 and 4 into a single background goroutine that runs resolution
*then* worktree/tmux — updating `creation_progress` at each phase transition ("Resolving
GitHub URL...", "Cloning repository...", "Setting up worktree...", "Starting session...").

## 2. Instance status/progress primitives already available

- `session.Instance.SetCreationProgress(msg string)` — `session/instance_actor_setters.go:68`.
  Already exists, already used by the current async tail. No new primitive needed for
  progress messages themselves — just more call sites, one per new phase.
- `session.Instance.ForceStatus(s Status)` — `session/instance_state.go:367`. Used today to
  force `Stopped` on async-start failure. The new work needs an equivalent `Failed` status
  value (see below) and presumably a `ForceStatus`-compatible transition into it, plus a way
  to store/read an error-reason string (something a card can render persistently — today's
  failure path only sets `creation_progress` to the error text and forces `Stopped`, which
  doesn't visually distinguish "you stopped this on purpose" from "this crashed while
  creating").
- `events.NewSessionCreatedEvent(instance)` / `events.NewSessionUpdatedEvent(instance,
  []string{"status", "creation_progress"})` — the event-bus primitives already used for the
  existing progress-publishing pattern; no new event type appears necessary per phase update
  (open question in requirements.md about retry re-publishing `SessionCreatedEvent` — my read
  of the existing pattern is that a `SessionUpdatedEvent` on `["status", "creation_progress"]`
  is sufficient, since `WatchSessions` subscribers already react to updates, not just
  creates — but this needs confirming against the actual `WatchSessions` handler in Phase 3).

## 3. Status enum — no `Failed` value exists yet

`proto/session/v1/types.proto:365-392`, `SessionStatus` (with `allow_alias = true`, several
deprecated wire-value-preserving aliases already in the enum — precedent for extending this
enum without breaking wire compat). Current values: `UNSPECIFIED(0)`, `ACTIVE(1)`,
`RUNNING(1, deprecated)`, `READY(2, deprecated)`, `LOADING(3, deprecated)`, `PAUSED(4)`,
`NEEDS_APPROVAL(5, deprecated)`, `CREATING(6)`, `STOPPED(7)`, `HIBERNATED(8)`,
`RESTORING(9)`, `CRASHED(10)`.

`CRASHED` (10) is the closest existing analog — its doc comment: "tmux pane exited
abnormally... Not auto-recovered; requires an explicit resume (see ResumeCrashedSession)."
This is a *post-Running* failure, semantically distinct from a *during-Creating* failure —
reusing it would conflate "the agent process died" with "the session never came up," which
have different valid recovery actions (resume vs. retry-creation). **Recommendation surfaced
by this research, not yet decided**: add a new `SESSION_STATUS_FAILED = 11` value rather than
overloading `CRASHED` or `STOPPED` — `STOPPED`'s own doc comment says "terminal state, cannot
transition further," which directly conflicts with the retry requirement (Failed → Creating
→ Running must be a legal transition). This is exactly the open question flagged in
requirements.md; Phase 3 should confirm by checking every place `Status` is switched over
(`session/instance_state.go`, `web-app/src/components/sessions/SessionCard.tsx`'s
`getStatusColor`/`getStatusText` switches at lines ~268/295) for exhaustiveness — a new enum
value that isn't added to both switches will silently render as "Unknown"/`statusUnknown`.

## 4. Frontend: `SessionCard.tsx` and `Omnibar.tsx`

- `SessionCard.tsx:235` — `isCreating = session.status === SessionStatus.CREATING`.
- `SessionCard.tsx:268-278` (`getStatusColor`) and `:295-305` (`getStatusText`) are parallel
  switches over `SessionStatus` with a `CREATING` case each ("Starting…" text, `statusLoading`
  color) and a `CRASHED` case (`statusCrashed` color, "Crashed" text) — both switches will
  need a new `FAILED` case, and there's a real design decision on whether Failed reuses
  `statusCrashed` styling or gets its own token.
- `creation_progress` display: requirements.md cites `SessionCard.tsx:955-959` for where the
  progress text currently renders — not yet visible in the switch statements read above, but
  confirmed to exist per the requirement's own line citation; the mechanism is already wired
  for the Creating case and should need no new plumbing beyond a Failed-state variant.
- `Omnibar.tsx` — the `onCreateSession` call sites (~lines 1160, 1208, 1295, 1742) are all
  wrapped in `try/await/finally` with `setIsSubmitting(false)` in `finally`, and (at line
  1742-1749) there's already a **client-side retry pattern**: on RPC error, `setError(message)`
  keeps the dialog open with a "Create & Open" retry button re-submitting `retryData`. That's
  a different mechanism than the new requirement (retry-in-place on an already-created Failed
  *session*, not retry-the-whole-RPC-before-any-instance-exists), but it shows the frontend
  idiom this codebase already uses for "let the user retry a failed submission" and should
  inform (not be reused for) the new Failed-card retry action.
- Per requirements.md, `Omnibar.tsx` needs to stop awaiting full completion — since
  `CreateSession` will now return almost immediately with a `Creating` instance regardless of
  session type, the existing `await onCreateSession(...); onClose();` pattern already does
  roughly the right thing structurally (close as soon as the promise resolves) — the RPC
  itself getting fast is what fixes the perceived hang, not a change to this call pattern.

## 5. Cancel-in-progress: `DeleteSession` is the closest existing precedent

`DeleteSession` (`server/services/session_service.go:3193`) already handles "the resource
might not be fully live yet" cases reasonably closely to what cancel-in-progress needs:

- Looks up by title-or-UUID from raw storage data (no PTY side effects).
- Stops any autonomous driver **before** destroying resources (use-after-delete hazard
  avoidance — same class of hazard a cancel-during-background-resolution would have).
- Captures the live instance via `FindLiveInstance` **before** `removeFromAllPollers` (comment
  explains a real bug this ordering fixes: calling `FindLiveInstance` after
  `removeFromAllPollers` always returned nil, silently skipping `Destroy()`'s worktree
  cleanup).
- `removeFromAllPollers` runs **before** `storage.DeleteInstance` specifically "to close the
  race window where external discovery could re-add the session."
- Actual `Destroy()` cleanup runs via `trackCleanup` (async), with a slow-cleanup timeout
  warning (`waitForDestroyLoggingSlowCleanup`, `s.deleteSessionCleanupTimeout`) — same
  detach-from-RPC-lifetime pattern the background resolution goroutine will need.
- Falls back to `KillTmuxSessionByTitle` when the instance isn't in the live in-memory poller
  (i.e., server restarted since creation) — directly relevant to "cancel a Creating session
  that's mid-clone when the server that started it has since restarted and this is a fresh
  process with no live goroutine to cancel."

**Gap `DeleteSession` doesn't cover**: cancelling a session **while its background creation
goroutine is still actively running** (mid-clone) requires actually interrupting that
goroutine — not just tearing down storage/tmux/worktree state after the fact. This needs an
explicit per-instance cancellation handle (e.g., a `context.CancelFunc` stored alongside the
instance, or a cancellation channel) that the background goroutine selects on between phases,
since the goroutine's context is deliberately detached from the RPC (per the "Context
lifetime" rabbit hole in requirements.md) and thus not killed by client disconnect the way
today's synchronous `ResolveGitHubInputCtxWithHosts(ctx, ...)` call is.

## 6. Stale-detection precedent: three independently-tuned detectors already exist

Directly useful precedent for the new stale-Creating detector — this codebase already has
**three** separate periodic staleness sweepers, deliberately kept independent (see
`server/services/stale_session_notifier.go:26-36`'s doc comment, which explicitly enumerates
and disclaims sharing state with the other two):

1. **`StaleSessionNotifier`** (`server/services/stale_session_notifier.go`, 155 lines) — a
   60s-interval sweeper (`staleSessionNotifierCheckInterval`) over `ACTIVE` sessions, using
   `config.StaleSessionConfig` (`config/types.go:123-148`: `ThresholdMinutes`,
   `ThresholdMinutesOrDefault()`, `NotifyEnabled *bool`, `NotifyEnabledOrDefault()`) — this is
   the **closest structural precedent** for the new detector: edge-triggered (fires once per
   stale "episode," tracked in a `map[string]time.Time` keyed by stable ID, entry removed on
   recovery so it re-arms), reads only in-memory instance state (no I/O), constructed
   tolerant of a nil `eventBus` for pre-wiring-order safety.
2. **`session/review_queue_determiner.go`**'s 5-minute `ReasonStale` badge (queue-membership
   concern, not a status transition).
3. **`session/backlog_lifecycle_stale.go`**'s `maxWorkSessionStaleness = 2 * time.Hour`
   (backlog-item-level, not session-level) — also shows the pattern of a plain
   package-level `const` threshold rather than config for a value not (yet) meant to be
   user-tunable, contrasted with `StaleSessionConfig`'s user-configurable threshold. The new
   Creating-staleness threshold per requirements.md **should** be config-driven (an "Open
   Question" explicitly asks for "a conservative default plus a config override"), so
   `StaleSessionConfig` is the closer analog to extend or mirror than
   `maxWorkSessionStaleness`.

`session/hibernation_sweeper.go` (409 lines) and `server/services/session_retention_sweeper.go`
(233 lines) are two more ticker-based background sweepers over instance state — worth a
closer read in Phase 3 for whichever one has the most similar "flip status + emit metric +
log" shape to what stale-Creating detection needs, but not required reading for this survey.

**Where the new detector should live** (Open Question in requirements.md): given the existing
precedent is "one small, independent, single-purpose sweeper per staleness concern" rather
than folding into an existing loop, the pattern most consistent with this codebase is a new,
small `StaleCreationSweeper`-shaped type (mirroring `StaleSessionNotifier`'s structure:
ticker + in-memory scan + edge-triggered dedup map) rather than adding a branch to any
existing sweeper's loop body.

## 7. Retry precedent: none server-side yet — `RestartSession` is adjacent but not equivalent

`RestartSession` (`server/services/session_service.go:4307`) exists but per requirements.md's
Non-Goals ("Retrying/cancelling sessions that are already Running (only applies to the
Creating/Failed states introduced/extended here)"), it's a different operation — it creates a
*new* session lineage from a *finished* one, not an in-place retry of a session that never
successfully started. No existing RPC retries a session in-place; this is genuinely new
surface. The requirement's own rabbit-hole note ("Retry-in-place vs. re-run-from-scratch")
correctly identifies this as needing explicit idempotent-cleanup-then-retry design — there is
no shortcut via an existing helper.

## 8. Edge cases and failure modes to design for

- **Rapid duplicate submissions.** Today's title-uniqueness check
  (`session_service.go:1829-1837`) reads `s.storage.ListInstanceData()` synchronously before
  any instance is created — this stays synchronous/fast-fail per Constraints, so two
  back-to-back submits with the same title are still serialized correctly *as long as the
  first `CreateSession` call's synchronous prefix (through instance creation + storage save)
  completes before the second one's uniqueness check runs*. Once GitHub resolution moves to
  the background, the **synchronous portion shrinks further** (good for this race — the
  window between "check title" and "save the Creating instance" gets *smaller*, not larger,
  since resolution work is no longer sandwiched in between). But: confirm the instance is
  persisted to storage (not just published as an event) synchronously before the RPC returns
  — if storage save is deferred into the background goroutine too, the uniqueness check for a
  second rapid request could race against an unpersisted first request. (Today's flow already
  saves synchronously somewhere before line 2380's publish — needs confirming exactly where
  in the untruncated middle section during Phase 3's detailed design.)
- **Cancel arriving right as background resolution succeeds.** Classic TOCTOU: the cancel
  handler and the resolution goroutine's success path both want to transition status and both
  race on cleanup. Needs a single-writer rule — e.g., the background goroutine checks a
  cancellation flag/context *immediately before* each state-mutating step (not just at phase
  boundaries), and the cancel path takes a lock (or uses a CAS on status) so that "already
  succeeded, ignore the late cancel" and "already cancelled, the success path must clean up
  instead of publishing Active" are both handled without a window where both sides "win."
  `Instance.ForceStatus` is a plain setter per the smell-check above (no CAS/guard) — check
  whether `session/instance_state.go` already has a compare-and-swap-shaped status transition
  helper broader than `ForceStatus`, or whether Phase 3 needs to add one specifically for this
  race (this needs a targeted look at `instance_state.go` in Phase 3, not just this survey).
- **Retry duplicating storage rows.** The retry action must reuse the existing instance's
  UUID/storage row (`UpsertRule`-style update-in-place — see the `--feature sql/upsert`
  constraint already called out in CLAUDE.md for the `ent` schema) rather than constructing a
  new `Instance` and calling whatever `CreateSession`-adjacent save path exists, or two rows
  end up representing one logical session.
- **Server restart mid-goroutine.** This is exactly stale-creation detection's job (item 6
  above) — a session left in `Creating` by a process that no longer exists needs to be
  detected on the *next* process's startup scan, not just by an in-process ticker (a ticker
  that only started counting from *this* process's boot will never notice a session that was
  already stale *before* the restart, unless the staleness check compares against a
  wall-clock "created_at" timestamp rather than an in-process duration — the check must use a
  persisted timestamp, not elapsed-goroutine-time).
- **Goroutine pile-up across many quick creations.** Flagged directly in requirements.md's
  Feasibility Risks. `trackCleanup`'s `deleteCleanupWG` already bounds this somewhat (each
  goroutine is tracked so `Shutdown` can observe/await the full set), but there's no cap on
  *concurrently running* creation goroutines today — if a user creates 20 GitHub-URL sessions
  in a burst, 20 concurrent clone subprocesses spin up. Out of scope per the requirement
  (single-user tool, "not a throughput concern" per Non-functional Requirements), but worth
  flagging as a known non-goal rather than silently unaddressed.
- **Background goroutine outliving a cancelled/deleted session's storage row.** If cancel
  deletes the storage row and event-bus entry but the goroutine is mid-clone and hasn't yet
  checked for cancellation, it will eventually try to `SetCreationProgress`/`SaveInstances`
  against an instance whose storage row is gone — needs the same category of use-after-delete
  guard `DeleteSession` already applies for the autonomous-driver case (stop-before-destroy
  ordering).
- **Detached-context goroutine that never terminates on genuine hang.** The rabbit-hole note
  about `context.WithTimeout(context.Background(), ...)` for the detached goroutine matters
  here: without *some* upper bound, a hung `git clone` against a flaky GHE host blocks that
  goroutine (and its subprocess) forever, which is also the scenario the stale-detector must
  independently catch and flip to Failed even if the goroutine itself never returns — i.e.,
  stale-detection and goroutine-timeout are two different backstops for the same failure mode
  and both are needed (a subprocess-level context timeout can't fully substitute for
  wall-clock stale detection, since the goroutine could be stuck somewhere that isn't the
  subprocess, e.g. a lock).

## 9. Unstated user needs beyond the explicit requirements

- **A way to tell "still resolving" apart from "actually stuck."** The success metric asks
  for a card to appear within ~1s with a human-readable `creation_progress` message, but users
  will also want some visual signal (spinner vs. static text) distinguishing "creation is
  actively progressing" from "creation has been sitting on the same message for 90 seconds
  and might be stuck" — before the stale-threshold auto-flip fires. This might already be
  covered by `SessionCard.tsx`'s existing `statusLoading` treatment for `CREATING`, but worth
  confirming in Phase 6 that a long-idle Creating session doesn't look identical to a
  freshly-started one.
- **Distinguishing user-cancelled from server-detected-stale from resolution-error** in the UI,
  not just in the backend's `Failed`-reason field. A user who explicitly cancelled shouldn't
  see the same toast/error copy as a session that failed because GHE was unreachable, or one
  auto-flipped by the stale detector — three different messages/copy, not just one generic
  "Failed" card, even though all three end in the same enum value.
- **Not losing the partially-typed omnibar state on a fast-fail vs. a background failure.**
  Today, a synchronous fast-fail (duplicate title, bad alias) keeps the dialog open with the
  form state intact so the user can fix and resubmit. Once background failures move to a
  toast+card mechanism after the dialog has already closed, the user has lost that form state
  — they'll need to re-open the omnibar and retype everything to try a *different* GitHub URL,
  even though "retry" (same input) is one click via the Failed card. This asymmetry (retry
  same input = easy, adjust input after failure = back to scratch) is implied by the scope cut
  but not explicitly called out as a UX trade-off in requirements.md — worth flagging to the
  user/product owner during Phase 3 design rather than discovering it in Phase 6 QA.
- **Metric/observability consumers beyond "local debugging."** Requirements.md explicitly
  scopes metrics to local pprof/OTEL debugging with no alerting — but the stale-detection
  metric in particular ("creation outcome... stale-timeout") is exactly the kind of signal a
  user would want summarized somewhere more visible than a raw metric stream (e.g., a small
  badge/count on the session list, "3 sessions failed to create today") if stale/failed
  creations become at all frequent in practice. Not required by this scope, but worth a note
  for a future iteration rather than assuming the metric alone satisfies the underlying need.
- **Idempotent retry across multiple rapid retry clicks.** If a user impatiently clicks
  "Retry" twice on a Failed card before the first retry attempt has updated status away from
  `Failed`, the retry handler needs the same single-writer discipline as the cancel/success
  race above — otherwise two concurrent retry-resolution goroutines for the same instance can
  interleave their `SetCreationProgress`/status writes.
