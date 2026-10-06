# Research: Feature Landscape — cold-start-uuid-loss

## 1. Existing "session/conversation resume" infrastructure

The UUID-detection logic is **not centralized** — there are at least four independent
call sites that each do their own live-process-then-path-fallback detection, plus a
background service that does the same thing on a timer:

| Call site | File:line | Trigger | Uses `DetectByPath`? |
|---|---|---|---|
| `HistoryLinker.correlateSession` | `session/history_linker.go:231-317` | Background: 5s poll loop **and** fsnotify watcher on `~/.claude/projects/` | Yes (line 278), gated by `pathFallbackAllowed` |
| `Instance.tryExtractConversationUUID` | `session/instance_claude.go:308-363` | Called synchronously from `startLocked`/`start` (after fresh-start), `instance_workspace.go` (`SwitchWorkspace`), and `agy_adapter.go`/`claude_adapter.go` (Import) | Yes (line 342), unconditional fallback |
| `startLocked` cold-restore branch | `session/instance.go:878-921` | Live actor-routed path, called via `Instance.Start()` | Only *after* deciding fresh vs. resume (the bug) |
| `start` (lowercase) cold-restore branch | `session/instance.go:1068-1127` | **Dead in production** — see finding below | Same bug, duplicated |

**Key finding — the two "duplicate" blocks are not both live.** This resolves the Open
Question in requirements.md directly. `Instance.Start(firstTimeSetup)`
(`session/instance.go:828`) is the actual public entry point used everywhere in
production (`session_driver.go`'s `handleDriverFailure`/`recoverFromStaleResume`,
`instance_workspace.go`, server wiring) — it routes through the actor mailbox to
`startLocked` (line 845). The sibling `start()` method (line 1023, the one with
`startMu.Lock()`) is only reachable via `Instance.StartWithCleanup()`
(`session/instance.go:1011`), and **`StartWithCleanup` has zero call sites outside test
files** (verified via `grep -rn "StartWithCleanup(" --include="*.go" .` — every hit is in
`*_test.go` or `tmux.go`'s own method definition). So:

- `startLocked` (session/instance.go:845-935) is the one and only fix target for the
  actual bug.
- `start()` (session/instance.go:1023-1150) is **dead code in production** but is
  exercised by `session/instance_cold_restore_test.go`'s `TestColdRestore_WithUUID` and
  siblings, whose doc comments claim to test "`Start(false)`" behavior but actually
  invoke the unused twin via `StartWithCleanup`. This means the existing cold-restore
  test suite validates the *wrong* code path today — a real gap, separate from (but
  related to) the bug itself. Phase 3 planning should decide: (a) delete `start()` and
  `StartWithCleanup()` as dead code and repoint those tests at `Instance.Start()`, or (b)
  fix both blocks for consistency even though only one is reachable. Given the "Small"
  appetite and the requirement to "not expand blast radius," (a) is likely cheaper and
  also closes the stale-test-coverage gap as a side effect — but it does touch ~40 test
  call sites across 6 files, which may itself exceed Small appetite; worth flagging to
  Phase 3 as a deferrable cleanup rather than bundling into the bug fix.

A third, intentional (non-buggy) UUID-clear exists in `Instance.Restart()`
(`session/instance.go:1554-1560`): when restarting a *paused* worktree session, it
deliberately clears the UUID before recreating the worktree, because the freshly
recreated worktree's encoded Claude project path may not match the one the old UUID's
JSONL lives under. This is a related pattern worth being aware of but is out of scope
(different trigger, already handles the mismatch deliberately) — not one of the two
blocks named in requirements.md.

## 2. HistoryLinker: a proactive background version of the same fallback

`session/history_linker.go` is a previously-built, still-running background service
(`server/dependencies.go:809`, started in `server/server.go:154`) that does almost
exactly what the fix needs to do, just not synchronously at cold-start decision time:

- Polls every 5s (`historyLinkerPollInterval`) over all registered instances, and reacts
  instantly to fsnotify events on `~/.claude/projects/`.
- For any instance where `HasClaudeSession()` is false, it unconditionally tries
  `DetectByPath` (`pathFallbackAllowed = !alreadyLinked || ...`, `history_linker.go:274`)
  and calls `inst.SetHistoryInfo(...)` to persist the recovered UUID.
- Exponential backoff (5s → 5min cap) after 3 consecutive misses, parking entirely after
  10 misses until the next `ScanAll()` (startup or fsnotify-triggered).

**Why this doesn't already prevent the bug**: `HistoryLinker` runs asynchronously and
independently of the synchronous cold-restore decision in `startLocked`. The bug fires
at the instant `startLocked` runs — it does not wait for or coordinate with
`HistoryLinker`'s next tick. This directly substantiates the "racy window" framing in
requirements.md: if a restart happens to land in the gap between a UUID being cleared
and `HistoryLinker`'s next scan (or while it's backed off), `startLocked` loses the
race. It also means the requirements' "Recapture reliability" success metric
(narrowing the racy window) has two independent levers already in the codebase: (1)
moving the fallback earlier inside `startLocked` itself (in-scope, per requirements),
and (2) `HistoryLinker`'s existing loop, which already does over 90% of what a
"persist to DB earlier" alternative (requirements.md's Alternative #2) would
accomplish, just via fsnotify + polling rather than a write-through on capture. Worth
noting to Phase 3: the deferred "persist to DB on every capture" alternative may be less
necessary than requirements.md assumes, since `HistoryLinker` already re-persists
(via `SetHistoryInfo`) opportunistically — the gap is purely about the synchronous
decision path not consulting either source before committing to fresh-start.

One design detail directly relevant to edge case (a) below: `HistoryLinker` explicitly
**skips** the path fallback for sessions that are already linked *and* in
`Paused`/`Hibernated`/`Stopped` status specifically because "the newest JSONL" heuristic
becomes wrong once other sessions have written newer files to the same directory after
the pause (comment at `history_linker.go:268-273`). This guard only applies when
`alreadyLinked` is true, though — for a session with an *empty* UUID (the exact case
this bug is about) `pathFallbackAllowed` is unconditionally true regardless of status,
so `HistoryLinker` (and the proposed `startLocked` fix) would still trust "newest JSONL"
blindly for a session that has never linked. That is the same ambiguity the bug's own
"Ambiguous JSONL selection" rabbit hole calls out, just confirmed to also apply to the
already-existing background linker, not only the code the plan will touch.

## 3. Edge cases and failure modes

### (a) `DetectByPath` finds a JSONL belonging to a different conversation

`session/history_detector.go:137-199`, `DetectByPath` implementation:

- Encodes the project path via `ClaudeProjectDirName` (non-alphanumeric → `-`,
  `history_detector.go:118-129`) to get the `~/.claude/projects/<encoded>/` directory.
- Lists every `*.jsonl` file in that directory except `agent-*.jsonl`, validates each
  basename is a UUID (`isValidUUID`), and picks **whichever has the latest
  `ModTime()`** (`sort.Slice` by `modTime`, descending; `history_detector.go:189-193`).
- There is **no correlation to session identity** beyond the directory path — no branch
  name, no session UUID stored in/alongside the JSONL, no content check. Any two
  `Instance`s (or a human running `claude` by hand) that share the same encoded project
  path will have their JSONLs pooled in the same directory, and "most recently
  modified" is the only tiebreaker.
- Concretely wrong-resume scenarios this allows: a worktree path is torn down and a
  **new, unrelated** session (different branch, different backlog item) is later
  created re-using the exact same directory path (this happens routinely for
  `SessionTypeExistingWorktree` re-runs on the same branch, or `SessionTypeDirectory`
  sessions pointed at a shared scratch dir) — if that new session's cold-start decision
  runs before its own conversation has a JSONL yet (i.e., truly first launch, UUID
  legitimately empty), `DetectByPath` would find the *old* session's JSONL (still the
  newest file in the directory if the old session finished more recently than "now")
  and hand `startLocked` a UUID for a **completely different conversation** — this is
  worse than the current bug (silent fresh-start) because it silently resumes into the
  wrong context.
- Today, `tryExtractConversationUUID`'s prod-reachable use (post-fresh-start
  enrichment) mitigates this since it only affects *future* resumes, not the current
  launch's `--resume` flag — the current session output already reflects a genuinely
  fresh start regardless of what UUID gets attached afterward. **Moving this check
  earlier — the whole point of the fix — removes that safety margin**: once
  `DetectByPath`'s result gates the actual `--resume` flag, a false positive here
  actively injects the wrong conversation. This is the sharpest edge in the whole
  effort and matches the "Ambiguous JSONL selection" rabbit hole almost exactly; a real
  fix likely needs some confidence signal beyond mtime (e.g., only trust the fallback
  when exactly one JSONL exists in the directory, or when the file's own first-line
  metadata / cwd matches the instance's expected path more strongly than "same encoded
  dir").

### (b) Two sessions sharing an effective root dir concurrently

`GetEffectiveRootDir()` (`session/instance_worktree.go:166-173`) returns the worktree
path for worktree-backed sessions, else `i.Path`. Nothing in `Instance` enforces
uniqueness of this value across concurrently-live `Instance`s — two sessions (e.g. a
`SessionTypeDirectory` pointed manually at a path, plus a separate session created
later against the same path) can legitimately share a root dir, meaning they share a
`~/.claude/projects/<encoded>/` directory and therefore share the exact JSONL pool
`DetectByPath` scans. If both are cold-restarting around the same time with empty
UUIDs, both would independently call `DetectByPath` and could both attribute
themselves to the same "newest" JSONL — not a crash, but a silent cross-session
UUID collision (two `Instance`s both believing they own the same conversation,
diverging from there since only one can actually be the process that keeps appending
to that file). `HistoryLinker.correlateSession`'s comment (line 268-273) already
half-acknowledges this class of problem for the already-linked case; it doesn't fully
solve it for the never-linked case, which is the one this bug fix operates in.

### (c) Restart-churn interaction with `driverInactivityTimeout`

`session/session_driver.go:44-46` defines `driverInactivityTimeout = 10 * time.Minute`.
Traced the actual path from timeout to `Start`:

1. The driver loop (`session_driver.go:389-425`) computes `idle` from the later of
   `initialPromptSentAt`, `LastMeaningfulOutputTime()`, and a nudge timestamp. Once
   `idle > graceTimeout` (10 min, or shorter for backlog-tagged sessions via
   `driverBacklogNudgeGrace`), it calls `handleDriverFailure(inst, ..., "inactivity timeout")`
   (line 423) and the goroutine returns.
2. `handleDriverFailure` (`session_driver.go:509-570`): uses a `*atomic.Bool` "retried"
   flag with `CompareAndSwap(false, true)` (line 510) — **the first failure restarts,
   the second (with the same `retried` pointer) only marks the session for human
   attention** (`markSessionNeedsAttention`, no further restart). This caps
   inactivity-driven restart churn to at most one automatic restart per driver
   lineage — a new driver goroutine is spawned at line 569 inheriting the same
   `retried` pointer, so a *second* stall after the restart escalates to review
   rather than looping.
3. Inside that single restart: if `inst.GetEffectiveStatus() == Stopped`, it calls
   `inst.RecoverFromStopped()` then **`inst.Start(false)`** (line 541) — this is the
   exact `Instance.Start` → `startLocked` cold-restore path the bug is in. So a
   inactivity-timeout-triggered restart of a `Stopped` session is a live, real trigger
   for the bug, not just theoretical.
4. Because the driver's own churn is capped at one restart, the "restarted several
   times in a short window" scenario from requirements.md's Baseline is **not**
   self-inflicted by `driverInactivityTimeout` alone — it requires an *external*
   compounding restart (a full service restart, hibernation-then-revive, or a manual
   restart) landing close in time to the driver's own single auto-restart. Concretely:
   inactivity timeout fires → `Start(false)` → cold-restore branch runs,
   `HasClaudeSession()` was true so it resumes correctly and then (post-fresh-start-only
   path, not hit here since it resumed) — but if a *second*, externally-triggered
   restart (e.g. `make install-service`, hibernation sweep) happens **before** the
   newly-resumed process gets a chance to have its UUID re-validated/persisted (there is
   no explicit re-persist-on-successful-resume step — the UUID was already in memory and
   assumed still good), and if in between something cleared it (e.g. a
   `recoverFromStaleResume` collision, or the process legitimately produced a new JSONL
   after `/clear` that `HistoryLinker` hasn't caught up to yet), the second restart can
   land with `HasClaudeSession()` false and no time for `HistoryLinker`'s 5s-interval
   background scan (or its backoff-throttled retries) to have refreshed it first. This
   matches requirements.md's own observation ("resumed correctly on a later revive,"
   i.e. racy/inconsistent) — the race is between **external restart cadence** and
   **`HistoryLinker`'s poll/backoff cadence**, not between the driver's own timeout and
   itself.
5. One concrete narrowing lever within Small appetite, beyond "move `DetectByPath`
   earlier": `handleDriverFailure`'s restart branch (line 537-541) calls
   `inst.Start(false)` directly rather than going through any explicit
   "try `HistoryLinker`'s already-known UUID first" step — but `Instance.Start` already
   is that step once fixed, so no extra plumbing needed here; this confirms the fix
   belongs entirely inside `startLocked`, not in `session_driver.go` (consistent with
   requirements.md's Out of Scope on `driverInactivityTimeout` itself).

## 4. Unstated needs beyond the explicit requirements

- **Manual conversation picker for ambiguous JSONLs.** Requirements.md's Open Questions
  implicitly raises this ("is there an existing session-event/notification mechanism...")
  but doesn't ask whether the user should be able to *choose* among multiple candidate
  JSONLs when `DetectByPath` finds more than one recent file for a path. Given edge case
  (a) above (shared-path collisions are a real, not hypothetical, occurrence in this
  codebase — worktree paths get reused constantly), a fully automatic "trust the newest
  mtime" resolution is a real risk once this fallback becomes load-bearing. A cheap,
  Small-appetite-compatible partial answer: when `DetectByPath` finds >1 candidate, do
  **not** auto-resume — treat it the same as "no JSONL found" (fresh start + the new
  durable marker), since a wrong-conversation resume is worse than a lost one. Full
  manual picker UI is out of appetite but the "ambiguous → don't guess" policy is cheap
  and directly serves the same success metric ("No silent fresh-start when
  unrecoverable" already anticipates a class of "can't be attributed with confidence"
  outcome — ambiguity is exactly that class).
- **Durable marker discoverability.** The existing precedent this repo already follows
  (per `feedback_document_ai_decisions_in_edge_cases` in user memory) is "self-heal/
  auto-close actions should post a visible comment + notify(), not act silently." Two
  existing mechanisms already fit without new proto surface (respecting the Small
  appetite's "no new proto fields" constraint):
  - `NotificationEvent` (`proto/session/v1/events.proto:390-417`) — already has
    `notification_type`, `priority`, `title`, `message`, `metadata` map, broadcast to
    all connected clients. A `NOTIFICATION_TYPE_*` for "started fresh, could not
    resume" (or reuse an existing generic type + a `metadata["reason"] =
    "cold_start_no_uuid"` key) would surface this without any proto changes.
  - `ReviewQueue` / `AttentionReason` (`session/queue/queue.go:14`,
    `session/review_queue.go:15-24` — `ReasonStale`, `ReasonErrorState`, etc.) — the
    existing pattern `session_driver.go:576-596`'s `markSessionNeedsAttention` already
    uses to surface stuck sessions in the UI. Adding a reason (e.g.
    `ReasonConversationLost`) reuses an existing UI surface (the review queue list)
    rather than inventing one — but review-queue additions are typically for
    *actionable* problems the user needs to resolve, so this is a stronger fit if the
    product intent is "flag for the user to notice" rather than "just log it happened."
  Either mechanism satisfies "not just a log line, inspectable via the existing session
  events/status API" from the Success Metrics — Phase 3 should pick one rather than
  build both, and the choice affects whether the marker also needs the fresh-start
  reason recorded on the `Instance`/`Session` snapshot itself (so it survives past the
  live event stream for anyone who reconnects later) — `EventStarted`
  (`session/instance.go:76`) currently always fires with an empty reason string
  (`i.fireLifecycleEvent(EventStarted, "")`, lines 999 and 1225); threading a non-empty
  reason through on the cold-start-fresh path is the cheapest way to make the marker
  durable in the existing lifecycle-listener fan-out without new proto fields.
- **Distinguishing "resumed via known UUID" vs "resumed via recovered UUID" in the log
  event**, per the Observability Requirement in requirements.md — today
  `startLocked`'s cold-restore branch only distinguishes "has UUID" vs "no UUID" in its
  log lines (`instance.go:881-885`); once the fallback runs *before* the decision, a
  third state exists (recovered-then-resumed) that current logging has no distinct
  line for. This is directly requested ("log a distinguishable structured event for
  each of the three outcomes") and is cheap to add alongside the reorder.

## Summary of concrete pointers for Phase 3 planning

- Fix target: `startLocked` only (`session/instance.go:845-935`). `start()`
  (`:1023-1150`) is dead code in production, reachable only through
  `StartWithCleanup`, which nothing outside tests calls — flag as a separate
  cleanup/test-repointing task, not required for the bug fix itself, but note that
  `instance_cold_restore_test.go` currently validates the wrong path.
  A verification bar for the fix, either as a real fix or as a documented deferral: the
  live cold-restore behavior must be exercised through `Instance.Start()`, not
  `StartWithCleanup()`.
- `DetectByPath` (`session/history_detector.go:137-199`) has no way to distinguish
  "the one true JSONL" from "a stale JSONL left by a different session that reused this
  path" — picks purely by mtime. Promoting it to a load-bearing pre-decision check (as
  required) needs an explicit policy for the >1-candidate case; treating ambiguity as
  equivalent to "not found" (fresh start + marker) is the cheapest safe default.
  `HistoryLinker` already has the same blind spot for never-linked sessions
  (`pathFallbackAllowed` doesn't gate on candidate count either) — worth flagging as a
  shared fix opportunity, not just a `startLocked`-local one, though only `startLocked`
  is in scope per requirements.md.
- `HistoryLinker` (`session/history_linker.go`) already does background best-effort
  recovery + persistence; it doesn't eliminate the race because it's asynchronous to
  the synchronous cold-start decision. This weakens the case for the "persist to DB
  earlier" fallback design (Alternatives Considered #2) — the gap is the missing
  synchronous check, not missing persistence infrastructure.
- No proto changes needed for the durable marker: `NotificationEvent` or
  `ReviewQueue`/`AttentionReason` both already exist and are wired to the UI; pick one.
  `EventStarted`'s currently-unused reason parameter is the cheapest carrier if the
  marker also needs to persist on the instance snapshot itself.
