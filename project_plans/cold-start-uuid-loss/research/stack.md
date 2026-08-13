# Stack Research: cold-start-uuid-loss

## Module / Go version
- `github.com/tstapler/stapler-squad`, Go 1.26.3 (go.mod:3).
- No new third-party dependency is needed. Everything the fix touches
  (`os`, `path/filepath`, `regexp`, `sort`, `strings`, the existing `session`,
  `pkg/events`, `server/notifications` packages) is already imported somewhere
  in the repo. `github.com/go-git/go-git/v5` is present repo-wide but is
  irrelevant here — this bug has nothing to do with git plumbing, only tmux
  process liveness + `~/.claude/projects/*.jsonl` filesystem state.

## Go packages/types involved

### `session/instance.go` — the two duplicate cold-restore blocks
- `startLocked(actorState *instanceState, firstTimeSetup bool) error` — lines 845-1007.
  Actor-safe body of the public `Start()` (line 828), which is routed through
  `sendSyncErr` (`session/actor.go`). This is the **live path for the vast
  majority of restarts**: `session_driver.go:541`, `health.go:179,229`,
  `instance_claude.go:88`, `instance_hibernate.go:110,154`,
  `instance_serialization.go:405,456`, `server/dependencies.go:681,698`,
  `server/mcp/tools_lifecycle.go`, `server/services/session_service.go` all call
  `instance.Start(...)` → `startLocked`.
- `func (i *Instance) start(firstTimeSetup bool, setupCleanup bool, cleanup *tmux.CleanupFunc) error`
  — lines 1023-1225 (approx). Not actor-routed — guards itself with
  `i.startMu.Lock()` directly instead of going through the mailbox. Only
  reachable via `StartWithCleanup()` (line 1011), which itself is called from
  exactly one place found in this repo: nothing in `grep -rn "StartWithCleanup("`
  turned up a live call site outside `instance.go`'s own definition — **this
  looks like a legacy/unused entry point**, but Phase 3 planning should confirm
  with a repo-wide "StartWithCleanup(" grep across `server/` and `cmd/` before
  assuming it's dead, since a missed call site would mean the fix has to patch
  both blocks anyway.
  - The two blocks (`instance.go:878-921` inside `startLocked`,
    `instance.go:1068-1127` inside `start`) are byte-for-byte identical in
    structure: `HasClaudeSession()` check → log line → VNC/CDP setup → `pm().Start()`
    → clear `ConversationUUID`/`HistoryFilePath` → `tryExtractConversationUUID()`
    *after* the fresh start already happened. Confirms the requirements doc's
    "Rabbit Hole" concern verbatim — this is real duplication, not a stale
    description.

### `session/instance_claude.go` — UUID capture/clear
- `HasClaudeSession() bool` (line 269) — the gate: `i.claudeSession != nil && i.claudeSession.ConversationUUID != ""`. Guarded by `claudeSessionMu` in every other accessor but this one accesses `i.claudeSession` in `startLocked`/`start` while only `stateMutex`/actor-confinement is held — not `claudeSessionMu`. Worth flagging in Phase 3: the field is read/written from two different lock domains (`claudeSessionMu` in normal accessors vs. bare actor/stateMutex confinement in the cold-restore blocks) and `tryExtractConversationUUID`'s own doc comment says "assumes stateMutex is already held" and "sets claudeSession fields directly" — i.e., it deliberately bypasses `claudeSessionMu`. Any reordering of the recovery call must preserve this existing (if slightly uncomfortable) convention rather than introduce a second locking scheme.
- `ClearConversationState()` (line 278) — clears `ConversationUUID`/`HistoryFilePath` outside the cold-restore path (used elsewhere, e.g. `recoverFromStaleResume`, line 83).
- `tryExtractConversationUUID()` (lines 308-363) — the recovery function at the center of the fix:
  1. No-ops if `ConversationUUID` is already set (line 310).
  2. **Fast path**: if tmux is alive, calls `detector.Detect(pid)` — inspects the live process's open file descriptors via `procinfo` (macOS `proc_pidinfo`-backed). Irrelevant to this bug since the bug only fires when tmux is dead.
  3. **Fallback path**: `detector.DetectByPath(effectivePath)` — the path-based, no-live-process-required detector. This is the one the fix needs to hoist earlier.
  4. Sets `i.claudeSession.ConversationUUID` / `i.HistoryFilePath` directly on success.
  - `i.historyDetector` is injectable (nil → falls back to `NewHistoryFileDetectorWithRealInspector()`), which is exactly the seam the requirements' unit-test plan (UUID present / recoverable via path / unrecoverable) should use — tests can inject a `HistoryFileDetector` built with `NewHistoryFileDetectorWithHomeDir` pointed at a temp dir, no process mocking needed for the path-fallback cases.

### `session/history_detector.go` — `HistoryFileDetector`
- `Detect(pid int32) (*HistoryFileInfo, error)` (line 59) — live-process path, uses `ProcessFileInspector` (mockable interface, `OpenFiles`/`IsAlive`).
- `DetectByPath(projectPath string) (*HistoryFileInfo, error)` (line 137) — **the fallback the fix needs to move earlier**. Reads `~/.claude/projects/<ClaudeProjectDirName(projectPath)>/*.jsonl`, filters out `agent-*.jsonl` and non-UUID basenames, and **picks the single most-recently-modified file** (`sort.Slice` by `ModTime`, line 190-193) — no attribution to "is this actually the same logical conversation," it's pure recency. This is the concrete mechanism behind the requirements' "Ambiguous JSONL selection" Rabbit Hole: a worktree path reused for a different conversation will have its newest JSONL picked with no cross-check (e.g. no correlation against a previously-known UUID, no check that the file's session start time is after the instance's `CreatedAt`).
- `ClaudeProjectDirName(projectPath string) string` (line 118) — pure function, deterministic path→directory-name encoding (`/` and other non-alnum → `-`). No I/O; easy to unit test directly if needed.
- `HistoryFileInfo{ConversationUUID, HistoryFilePath, ProjectDir}` — the return type both call sites consume.
- Directory-not-found and empty-dir are both `nil, nil` (not an error) — callers must treat "no info" as a valid outcome, not a failure to log/handle specially.

### `session/actor.go` — the actor-mailbox pattern `startLocked`'s doc comment refers to
- Single-goroutine-per-Instance confinement: `li.mailbox` (buffered? — worth checking `NewLiveInstance`, not read in this pass) carries `command` closures; `runActor` (lines 121-135) drains them serially and republishes an atomic snapshot after each one via `li.snapshot.Store(snap)`.
- `sendSyncErr` / `send` / `sendCtx` (lines 34-101) are the three ways external callers enqueue work. `startLocked` runs as a `sendSyncErr` command body (see `Start()`, instance.go:828-832) — i.e., **any code the fix adds inside `startLocked` already runs actor-confined**, and per the file's own doc comment, "Locked" twins access Instance fields directly without `stateMutex`. This matters for the fix: moving `tryExtractConversationUUID()` earlier inside `startLocked` needs no new locking — it's already running on the single actor goroutine. The non-actor `start()` twin does NOT have this guarantee beyond its own `startMu.Lock()`, which is a second reason to prefer consolidating onto `startLocked` if Phase 3 finds `start()`/`StartWithCleanup` truly dead.
- `runActor`'s own doc comment (lines 103-120) flags a **pre-existing, unrelated race**: a handful of legacy setters (`MarkViewed`, `MarkUserResponded`, `MarkAcknowledged`, `SetLastMeaningfulOutput`, `RecoverFromStopped`) mutate fields under `i.mu.Lock()` directly, bypassing the actor. Not in this bug's blast radius, but worth naming so the fix doesn't accidentally assume every Instance mutation goes through the actor.

## Community-recommended patterns for the concurrency primitives in use

The actor/mailbox pattern here (single goroutine owns mutable state, callers
communicate via a channel of closures, i.e. "Do not communicate by sharing
memory; instead, share memory by communicating") is the idiomatic Go
alternative to a plain mutex when the invariant is "many small, sequenced
mutations must never interleave." This matches current (2024-2026) Go
community guidance:
- It avoids the classic pitfall of a `sync.Mutex`-guarded struct where callers
  forget to lock before touching a field (the exact bug `runActor`'s doc
  comment flags about the five legacy direct-lock setters).
- The buffered-channel-of-closures shape (`mailbox chan command`) is the
  standard "worker with a request queue" idiom; `sendSyncErr`'s
  reply-channel-per-call pattern is the standard way to get a synchronous
  request/response out of an async worker without a second mutex.
- One thing to watch per current best practice: **every command sent to the
  mailbox should be short and non-blocking** (no I/O, no locks that could be
  held by another slow command) since it serializes all Instance mutations
  through one goroutine. `tryExtractConversationUUID()`'s `DetectByPath` does
  a directory `os.ReadDir` + per-file `os.Stat` — bounded, local-disk,
  already-happening-unconditionally-today (per the NFR in requirements.md),
  so moving it earlier inside the actor-confined `startLocked` doesn't add a
  new class of risk, but the fix should not add anything slower (e.g. a
  network call, an ent/DB round-trip) into this same actor-command path
  without an explicit off-actor dispatch, since that would stall every other
  queued command for this Instance (mailbox is per-Instance, not global, so
  this is a narrow blast radius, but a real one for that one session).
- No new concurrency primitive is needed for this fix — it is a
  control-flow reorder (call the existing recovery function earlier, in the
  same lock/actor context it already runs in today) plus a new event-publish
  call using an existing pattern (see below). `golang-concurrency`/`golang-development`
  skill guidance was consulted; nothing in this fix calls for atomics,
  singleflight, or a lock-free structure — the existing actor mailbox already
  is the "collapse to one struct/one owner" idiom those skills recommend.

## Prior art for a durable "session event/notification" surface

Confirmed: yes, a directly reusable, already-durable, session-scoped
notification mechanism exists — **`NotificationEvent`**
(`proto/session/v1/events.proto:388-417`) — and it is a better fit than the
`LifecycleEvent`/`fireLifecycleEvent` mechanism also present in this file.

### Option A (closest fit): `NotificationEvent` via `Notifier` interface + `EventBus`
- `session.Notifier` interface (`session/backlog_lifecycle.go:28-30`):
  `Notify(itemID, title, message string, notificationType, priority int32)`.
  Currently only wired into `BacklogLifecycleListener` (`SetNotifier`/`getNotifier`/`notify`,
  lines 717-735) — **`Instance` itself has no notifier field today.**
- Concrete adapter: `server/services/backlog_notifier.go`'s `EventBusNotifier`
  (implements `session.Notifier`, wraps `*events.EventBus`). It exists in
  `server/services` rather than `session` specifically because of an import
  cycle: `pkg/events` imports `session`, so `session` cannot import
  `pkg/events` back (comment at `backlog_notifier.go:8-11`). **This is the
  concrete architectural constraint the "started fresh" marker fix must
  respect** — `Instance` (package `session`) cannot call
  `pkg/events.NewNotificationEvent`/`EventBus.Publish` directly; it has to go
  through the same interface-in-consumer-package + adapter-in-server-layer
  indirection already established for backlog notifications.
- `pkg/events.NewNotificationEvent(sessionID, sessionName, notificationID, notificationType, priority int32, title, message string, metadata map[string]string) *Event`
  (`pkg/events/types.go:223-245`) is the constructor the adapter calls.
  `EventBusNotifier.Notify` (`backlog_notifier.go:29-34`) threads its first
  arg through as `sessionID` — for backlog it's currently the backlog
  `itemID`, but for this fix the natural value is the actual session's UUID
  (`i.UUID`), which is exactly what `NotificationEvent.session_id` is
  documented to mean end-to-end (see `SessionEvent`/`NotificationEvent` proto
  and `server/notifications/subscriber.go`'s coalescing key
  `sessionID:notificationType`).
- Durability: `server/notifications/subscriber.go`'s `StartSubscriber(ctx, bus, store *NotificationHistoryStore)`
  persists these events to a `NotificationHistoryStore` — this is what makes
  the requirement's "inspectable via the existing session events/status API,
  not only journalctl/log grep" achievable without new storage plumbing.
  `NotificationType` already has a value that fits without adding a new enum
  member: `NOTIFICATION_TYPE_WARNING = 8` (`proto/session/v1/types.proto:783`)
  or `NOTIFICATION_TYPE_FAILURE = 9` (line 784) for the "started fresh, could
  not resume" case; `NOTIFICATION_TYPE_AUTO_APPROVED = 13` (line 790, "no
  human action needed") is a plausible fit for the "started fresh but
  recovery wasn't even attempted/needed" non-error case if the fix wants a
  distinct log-only signal there too — but per Success Metrics, only the
  *unrecoverable* fresh-start case strictly needs a durable marker.
- **Gap**: since `Instance` has no `Notifier` field/setter today, the fix's
  smallest-surface option is either (a) add a `Notifier` field + setter to
  `Instance` (mirroring `BacklogLifecycleListener`'s pattern exactly — small,
  consistent with existing convention) and wire an `EventBusNotifier` into it
  wherever Instances are constructed (`server/dependencies.go` is the likely
  wiring point, same place `SetEventBus`/`SetNotifier` calls already happen
  for the backlog listener), or (b) route the marker through
  `BacklogLifecycleListener` only for backlog-created sessions — but that
  would fail the requirement for non-backlog session types (directory,
  worktree, one-off), which the requirements doc explicitly says share this
  code path. **(a) is the only option that covers all session types** per
  the requirements' Users/Consumers section.

### Option B (weaker fit, already free): `LifecycleEvent` / `fireLifecycleEvent`
- `session/instance.go:69-95`: `LifecycleEvent` (`EventStarted`/`EventExited`/`EventStopped`)
  + `LifecycleListener` interface (`OnLifecycleEvent(event LifecycleEvent, reason string)`),
  fired via `i.fireLifecycleEvent(event, reason)`.
  `EventStarted` already fires at the end of both `startLocked` (line 999) and
  `start` (line 1225) with an empty reason string today (`i.fireLifecycleEvent(EventStarted, "")`).
  The `reason` parameter is free-form and already plumbed to every registered
  listener (`BacklogLifecycleListener`, `sessionSummaryListener`,
  `review_queue_poller.go`'s reconciler) — trivial to change to something like
  `"cold-restore:fresh"` / `"cold-restore:resumed-known"` / `"cold-restore:resumed-recovered"`.
  **But**: `LifecycleListener` is in-process, in-memory only (no persistence,
  no proto/RPC surface) — it does not by itself satisfy "inspectable via the
  existing session events/status API." It's a good mechanism for the
  Observability Requirement's "structured event for each of the three
  outcomes" (log-level distinguishability, cheap, already free), but not a
  substitute for Option A's user-visible/durable marker. The two are
  complementary, not alternatives: cheap internal signal (B) + durable
  user-facing marker only for the failure case (A).

### `feedback_document_ai_decisions_in_edge_cases` memory precedent
The user's own stored instinct ("self-heal/auto-close actions should post a
visible comment + notify(), not act silently") is exactly `BacklogLifecycleListener.notify()`
/ `notifyTransitionFailed()` (`session/backlog_lifecycle.go:731-755`) — i.e.
the precedent this memory refers to *is* Option A's `Notifier` pattern,
already proven out for an analogous "silent state divergence" bug class
(BUG-030/040/041/046/048, per that method's doc comment). Reusing the same
interface for this bug is consistent with established practice, not a new
pattern.

## Open items for Phase 3 (plan) to resolve, not settled by this research
1. Confirm whether `start()`/`StartWithCleanup()` has any live caller outside
   `instance.go` — this research's grep found none, but a repo-wide search
   restricted to `.go` files under `server/`, `cmd/`, and test helpers should
   be re-run before deciding to patch only `startLocked`.
   `grep -rn "StartWithCleanup(" --include=*.go .` (excluding
   `.claude/worktrees/*` stale copies) is the exact command to re-run.
2. Decide whether `Instance` gets its own `Notifier` field (Option A(a) above)
   or whether the marker is instead threaded through the existing
   `BacklogLifecycleListener`/`sessionSummaryListener` `OnLifecycleEvent`
   hook and a *new* listener implementation bridges `EventStarted` reasons to
   `NotificationEvent` — the latter avoids touching `Instance`'s field list at
   all and reuses `LifecycleListener` (Option B) purely as the internal
   signal, with a new small adapter doing the B→A translation. This is
   probably the lower-blast-radius design for a Small-appetite fix and worth
   evaluating first.
