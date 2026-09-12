# Stack Research: Durable Guidance Requests / Questions

Research into existing patterns this feature should reuse, for a durable
"Question"/"Guidance Request" construct scoped to a backlog item, session, or
standalone, rendered as a structured form and durably notifying the asker
when answered.

## 1. Proto/RPC layer

`proto/session/v1/backlog.proto` defines the backlog domain. The
`WatchBacklogItems` streaming RPC (declared around
`proto/session/v1/backlog.proto:1253` in the `BacklogService` block, request
message at `backlog.proto:1241-1251`) is the precedent for "durable, survives
reload, notifies subscribers":

- `WatchBacklogItemsRequest` (`backlog.proto:1241`) carries `status_filter`,
  `category_filter`, and `after_seq` — the last one is the reconnect/replay
  key: a client that disconnects can resume "after this sequence number"
  instead of getting a fresh snapshot.
- `BacklogItemEvent` (`backlog.proto:1126-1161`) is a `oneof` of typed
  sub-events (`status_changed`, `verdict_recorded`, `session_attached`,
  `item_updated`, `item_archived`, `item_removed`, `snapshot_complete`,
  `activity_note_added`), each carrying a monotonic `seq` (field 8) assigned
  by `pkg/events.EventBus` at `Publish` time. A GuidanceRequest feature would
  most naturally add new oneof variants here (e.g.
  `GuidanceRequestCreatedEvent`, `GuidanceRequestAnsweredEvent`) rather than a
  wholly separate proto file, mirroring how `activity_note_added` (ADR-002,
  see `session/ent/schema/backlog_activity_note.go`) was added as a sibling
  oneof case rather than folded into `item_updated`.
- The Go server implementation lives in
  `server/services/backlog_service_events.go`. `watchBacklogItems` (line 91)
  is the testable core: it subscribes to the bus *before* building the
  initial snapshot (line 107, to avoid losing events in the race window),
  then either replays `EventsSince(after_seq)` (line 119) or sends one
  synthetic snapshot event per current item (line 147), forces
  `is_snapshot: true` on replayed events to avoid double-flashing the UI
  (line 141), sends an explicit `snapshot_complete` marker if literally
  nothing was sent so the client's `for await` loop doesn't hang forever on
  an empty backlog (lines 167-182), then fans out live events forever until
  ctx is canceled (lines 185-203). `backlogItemMatchesFilters`
  (`backlog_service_events.go:212`) is how filtering is applied uniformly to
  both the snapshot and live phases.
- The RPC method itself (`WatchBacklogItems`, `backlog_service_events.go:80`)
  is a thin wrapper delegating to `watchBacklogItems` through the narrow
  `backlogItemEventSender` interface (line 39) — done specifically so tests
  can drive it with a fake sender, since connect-go's real
  `*connect.ServerStream[T]` isn't constructible outside its own package. Any
  new `WatchGuidanceRequests`-style RPC should copy this shape.

**Codegen workflow**: `make proto-gen` regenerates `gen/proto/go/...` (and the
TS client under `web-app/src/gen/`) from `.proto` sources — both are
gitignored (see root `CLAUDE.md`'s ent/proto generation notes) and must never
be hand-committed; only the `.proto` source diff is committed. Every Make
target that needs generated code depends on it already.

## 2. Storage layer

Ent (`entgo.io/ent`) is the established ORM. Schemas live under
`session/ent/schema/*.go` (hand-written; everything else under `session/ent/`
is generated and gitignored per root `CLAUDE.md`). `BacklogItem`
(`session/ent/schema/backlog_item.go:15`) is the central entity; it declares
edges to sibling per-item entities like `item_sessions`, `status_events`,
`stuck_states`, `progress_notes`, and `activity_notes`
(`backlog_item.go:196-227`), all with `entsql.OnDelete(entsql.Cascade)`.

**Closest existing precedent for a new "GuidanceRequest" entity**:
`BacklogActivityNote` (`session/ent/schema/backlog_activity_note.go`) — an
append-only, per-item log any session can write to, "with or without
STAPLER_SESSION_UUID, and regardless of whether that session is linked to the
item" (file's own doc comment). Its shape:
- `id` (UUID), `item_id` (UUID, plain field not nested edge for the FK),
  `message` (string), `author_session_uuid` / `author_session_title`
  (optional, best-effort caller identity), `created_at` (immutable).
- One edge back to `BacklogItem` (`Ref("activity_notes")`,
  `.Field("item_id").Unique().Required()`).
- One composed index `index.Fields("item_id", "created_at")` for "all notes
  for this item, in order."
- See `project_plans/backlog-item-activity-log/decisions/ADR-001-sibling-table-not-extend-progress-note.md`
  for the "new sibling table, don't extend an existing gated table"
  rationale — the same rationale applies to GuidanceRequest: it should be its
  own ent schema, not bolted onto `BacklogActivityNote` or `BacklogItem`
  itself, because it has different write-gating (only the asker can create
  it; only a person/subscribed session can answer it) and a different
  lifecycle (pending → answered, not append-only).
- A GuidanceRequest schema will likely need: `id`, an optional `item_id`
  (nullable — the requirements call for backlog-item-scoped, session-scoped,
  *or* standalone), an optional `session_id` (the asker), `question_type`
  (yes/no, multiple-choice, short-answer), `question_text`, a JSON/string
  field for choices (mirroring `BacklogItem.acceptance_criteria`'s "JSON
  []AcCriterion" convention, `backlog_item.go:28-30`), `status`
  (pending/answered/expired), `answer` (nullable string/JSON), `answered_at`,
  `answered_by`, `created_at`.

**Upsert / `--feature sql/upsert`**: `session/ent/generate.go` documents the
required generate invocation (`go run -mod=mod entgo.io/ent/cmd/ent generate
--feature sql/upsert ./session/ent/schema`); root `CLAUDE.md` repeats this as
CRITICAL. The one concrete consumer today is `ApprovalRule`'s
`UpsertRule` (`session/ent_repository.go:1532`, called from
`session/storage.go:775`) — a plain `.Create()...` chain (not
`.OnConflict()`; grep found no `.OnConflict(` call sites anywhere in
`session/`), so today's "upsert" is actually implemented as delete-then-
create or similar at the repository layer, not ent's native
`sql/upsert`-feature `OnConflict` API. **If GuidanceRequest needs true
create-or-update-by-natural-key semantics** (e.g. "one open guidance request
per session at a time" idempotency), it would be the first real consumer of
ent's `OnConflict` API rather than following the existing `UpsertRule`
pattern verbatim — worth flagging in the plan phase rather than assuming
`UpsertRule` is copy-pasteable.

`session/storage.go` (1552 lines) is the facade session/backlog code calls
into (`*Storage`), which delegates to `session/ent_repository*.go` files
(`ent_repository.go`, `ent_repository_backlog.go`, etc.) for the actual ent
queries. A `GuidanceRequestStore`/`GuidanceRequestData` pair would follow the
same `*Data` DTO + `*EntRepository` method + `*Storage` passthrough
three-layer convention visible in `ApprovalRuleData`/`UpsertRule`.

## 3. Notification mechanism

Project memory's "self-heal/auto-close actions should post a visible comment
+ notify()" refers to two related but distinct real subsystems — confirmed
by reading the source, not guessed:

**a. The event-bus `Notify()` adapter** — `EventBusNotifier`
(`server/services/backlog_notifier.go:12-35`) implements a `session.Notifier`
interface by publishing an `events.NewNotificationEvent(...)` onto the shared
`*events.EventBus`. Its doc comment explains *why* it's a separate adapter
file: `session` cannot import `pkg/events` directly (would be a cycle), so
this thin adapter lives in `server/services` and is wired in via
`BacklogLifecycleListener.SetNotifier` / `BacklogService.SetEventBus`. It
threads the backlog item ID through as the event's `sessionID` specifically
so the notification-subscriber's coalescing key (see below) doesn't merge
unrelated items' same-type notifications together — a bug that was fixed
here (comment at `backlog_notifier.go:21-28`).

**b. The durable `NotificationHistoryStore` + `GetNotificationHistory` RPC**
— this is what actually gives a notification survivability across a
disconected/restarted client, and is the real precedent for GuidanceRequests
"surviving the asking session being paused/restarted/no-longer-live":
- `server/notifications/subscriber.go`'s `StartSubscriber`
  (`subscriber.go:31`, delegating to `StartSubscriberWithInterval` at line
  37) subscribes to the event bus and persists every `EventNotification`
  event into a `NotificationHistoryStore` — decoupling "a live toast fired"
  from "a durable, queryable record exists." `eventToRecord`
  (`subscriber.go:135`) does the event→record mapping;
  `coalesceKey(sessionID, notifType)` (`subscriber.go:130`) is the dedup key
  referenced above.
- `server/notifications/store.go`'s `NotificationHistoryStore` (694 lines) is
  a JSON-file-backed store (`NewNotificationHistoryStore(filePath)`,
  `store.go:133`) with `Append`, `List` (with filter/pagination options,
  `store.go:281`), `MarkRead`, `Clear`, `GetUnreadCount`, retention pruning
  (`enforceRetention`, `store.go:585`) and orphan pruning tied to whether a
  session still exists (`PruneOrphaned`, `store.go:423`). This is a
  hand-rolled JSON store, not ent — notifications are not backed by the ent
  ORM at all today.
- `server/services/notification_service.go`'s `NotificationService` exposes
  the RPCs: `SendNotification` (line 76, validates localhost origin, applies
  per-session rate limiting via `NotificationRateLimiter`, resolves a
  session's display name via `ReviewQueuePoller` with a durable-storage
  fallback, then publishes via the event bus), `GetNotificationHistory`
  (line 168, the actual "come back later and see what you missed" read
  path — exactly the RPC a paused/restarted session's owner would poll or a
  UI would call to recover missed guidance-answered notifications),
  `MarkNotificationRead` (line 235), `ClearNotificationHistory` (line 261).
- MCP exposure: `server/mcp/tools_notifications.go`'s
  `getNotificationHistory` handler (line 69) backs the
  `mcp__stapler-squad__get_notification_history` tool referenced in project
  memory — confirmed real, not guessed (tool schema visible in this session's
  own MCP tool list).

**Implication for GuidanceRequests**: the notification mechanism alone
(fire-and-forget event + JSON history store) is not itself durable enough to
guarantee an asking session "resumes" correctly — it is a notify/read-later
channel, not a request/response one. A GuidanceRequest needs its own
ent-backed entity (pending/answered state machine, see §2) *plus* reuse of
this notification pipeline to alert the asker's *owner* (human, or another
session polling) that an answer landed — i.e. GuidanceRequest answered should
call the same `Notifier.Notify(...)` path `EventBusNotifier` already
provides, not reinvent a second notification channel.

## 4. Autonomous driver / triage integration points

**`session/autonomous_driver.go`** (723 lines) runs a turn-based control loop
in `(*AutonomousDriver).run` (line 258). Per turn it: waits for the tmux pane
to go idle (`waitForIdle`, line 535, driven by a `detection.DetectedStatus`
channel fed from `d.controller.AddStatusChangeListener`), builds an
"orchestration prompt" from the goal + pane tail
(`buildOrchestrationPrompt`, line 631), calls a headless LLM
(`d.headlessPool.CallBlocking`, line 330), and parses the response into an
`orchestrationDirective` (line 676) via `parseOrchestrationResponse` (line
693). The directive enum today is exactly three values
(`directiveNextMessage` iota-zero, `directiveDone`, `directiveWait` — lines
679-681): `DONE` ends the loop and fires a completion callback (line
343-353); `WAIT` (or a duplicate-nudge dedup match, line 362) skips
injecting anything this turn but still waits for idle before continuing
(line 374-380); anything else is injected into the pane via
`SubmitDriverContent` (line 394) and the loop again waits for idle (line
416-421).

This is the natural extension point for "pause and wait for a durable
answer": a new `directiveAskQuestion` (or similar) value, parsed from a
fourth response shape in `parseOrchestrationResponse`, that (a) creates a
GuidanceRequest scoped to the session/item, (b) enters a wait state distinct
from `directiveWait`'s short idle-then-continue — it must block potentially
indefinitely (session paused, human away) rather than retry every turn — and
(c) resumes the loop once the request's ent row transitions to answered,
either via a bus subscription (mirroring `WatchBacklogItems`'s
`eventCh`/`Subscribe` pattern) or a poll. There is no pre-existing generic
"pause a turn loop until an external signal" primitive in this file — the
closest existing wait shape is `waitForRateLimitClear` (line 475), which
polls until a `time.Duration` clears, not until an arbitrary external event
fires; a GuidanceRequest-aware wait would need a new function alongside it
that selects on both an event-bus channel and `ctx.Done()`.

**`session/backlog_triage.go`** (mostly prompt-construction: `TriageSuggestion`,
`TriageTask`, `HeadlessTriageResult` types and `BuildHeadlessTriagePrompt`,
lines 1-100+) is *not* itself a state machine or control loop — it renders
the one-shot prompt handed to a headless triage LLM call (parallel research →
synthesis → validation → JSON output, per the prompt template at lines
75-100). The actual triage control flow/state machine lives in
`server/services/backlog_service_triage.go` (3600+ lines, not fully read
here — flagged for the architecture-research dimension to cover in depth).
For "ask a clarifying question instead of guessing," the natural point is
inside `BuildHeadlessTriagePrompt`'s prompt template (or a new sibling
prompt) instructing the model it may emit a distinguished
"NEED_CLARIFICATION" JSON shape instead of (or interleaved with) the final
triage-result JSON — then `backlog_service_triage.go`'s result-parsing path
would need a branch that creates a GuidanceRequest and defers final triage
completion until it's answered, structurally parallel to the
`directiveAskQuestion` extension above but for the one-shot (non-turn-loop)
triage path rather than the autonomous driver's turn loop.

## 5. Frontend subscription pattern

`web-app/src/lib/hooks/useWatchBacklogItems.ts` (529 lines) is the hook to
mirror. Key structural pieces any new `useWatchGuidanceRequests`-style hook
should copy:
- A ConnectRPC streaming client created once via `createClient(BacklogService,
  getWatchTransport())` (line 181) against a shared "watch transport"
  singleton (`@/lib/api/transport`), not a fresh transport per hook instance.
- Dual-path freshness: an immediate REST snapshot fetch (`refresh`, line 186,
  calling `listBacklogItems`) fired in parallel with (not gated behind)
  opening the stream (line 211-214) — "Task 4.2.1a/pitfalls.md #1."
- `after_seq` gap detection (lines 244-276): tracks the last-seen `seq`,
  detects backward jumps (server restarted) and forward gaps (bus dropped an
  event under backpressure) and triggers a full resync (`triggerResync`, line
  231) in both cases.
- Exponential-backoff reconnect capped at `MAX_RETRIES = 5` / 30s (lines
  391-410), falling back to REST polling every `FALLBACK_POLL_INTERVAL_MS =
  30_000` once retries are exhausted (lines 431-443).
- Two independent "idle staleness" backstops — a 30s periodic timer (lines
  448-464) and a visibility/online-event listener with a 15s staleness
  threshold (lines 468-493) — both force a reconnect+refetch even if the
  stream produced zero errors but also zero events for too long.
- `connectionState` is hook-local React state (`"connecting" | "live" |
  "reconnecting" | "polling" | "stale"`, lines 38-43), explicitly *not*
  stored in Redux (see file-header note 2) — a debounced
  reconnecting→live transition (`LIVE_TRANSITION_DEBOUNCE_MS = 300`, line 72,
  `scheduleLiveTransition` line 221) prevents flicker on a flapping
  connection.
- A per-event `switch (event.event.case)` dispatch (lines 278-332) into a
  Redux slice (`backlogItemsSlice`'s `upsertItem`/`removeItem`/
  `appendActivityNote`) — `activityNoteAdded` (line 313) is the most direct
  precedent for how a `guidanceRequestCreated`/`guidanceRequestAnswered`
  event case would be handled: a dedicated single-entry event dispatched to a
  *targeted* reducer action, never a full-item `upsertItem` re-snapshot (see
  ADR-002 reference in the comment at line 314-316).

`web-app/src/lib/hooks/useBacklogService.ts` supplies `mapBacklogItem`, the
proto→domain mapping every rendering component actually consumes; the watch
hook maps through it (lines 514-526) rather than exposing raw proto shapes to
components.

## 6. Existing similar UI components

Two directly reusable/precedent pieces found under `web-app/src/components/`:

- **`web-app/src/components/backlog/VaguenessPromptModal.tsx`** — a real
  precedent for a forced-choice (yes/no-shaped) modal blocking further
  action until answered: `role="dialog"`, `aria-modal`, a focus trap
  (`useFocusTrap`, line 30) with **no Escape-key dismissal** ("the user must
  choose one of the two explicit options," line 21 comment) and two explicit
  buttons (`onRefine` / `onProceed`, lines 51-66). Rendered via
  `createPortal(content, document.body)` to escape CSS-transform ancestors
  (ADR-009 reference, line 73). This is the closest existing UI shape for a
  yes/no GuidanceRequest form.
- **`web-app/src/components/ui/RadioGroup.tsx`** — a generic, already-
  accessible multiple-choice primitive: `role="radiogroup"`/`role="radio"`/
  `aria-checked`, roving tabindex, and arrow-key cycling that both moves
  focus and selects in one step (no separate confirm keystroke). Generic
  over `RadioGroupOption<T extends string>` (value/label/optional
  description/optional `dataTestId`), already used by
  `RuleBuilderForm.tsx`, `TriggerFormModal.tsx`, `WorkspaceSwitchModal.tsx`,
  `ThemePicker.tsx`, and others. This is directly reusable, unmodified, for
  a GuidanceRequest's multiple-choice question type — no new component
  needed for that variant.
- No existing free-text/short-answer form primitive was found scoped
  specifically for this kind of Q&A; a short-answer question type would most
  likely reuse a plain styled `<textarea>`/`<input>` the way ordinary forms
  elsewhere in `web-app/src/components/` do (not investigated further — out
  of scope for a "does a reusable component already exist" check, since
  plain text inputs are commodity and every form component defines its own).

Backend-side, **`server/services/approval_handler.go`**'s `ApprovalHandler` /
`ApprovalStore` (`server/services/approval_store.go`) is worth flagging as a
*related but distinct* existing "pause and wait for an external answer"
mechanism — Claude Code's own `PermissionRequest` hook flow. `ApprovalStore`
(`approval_store.go:76`) is a JSON-file-backed store (not ent) with `Create`,
`Get`/`GetBySession`, `Resolve` (line 215), `CancelSession`, and disk
persistence (`persistToDiskLocked`/`loadFromDisk`) plus TTL-based
`CleanupExpired` (line 288, driven by `StartExpirationCleanup`,
`approval_handler.go:852`). Its `broadcastQuestionNotification`
(`approval_handler.go:695`) already fires an `INPUT_REQUIRED` notification
type when Claude's own tool-use classifier detects a question. This is a
narrower, synchronous-HTTP-hook-shaped mechanism (Claude blocks on the HTTP
response for allow/deny) rather than an async durable Q&A construct, so it
is not directly reusable as GuidanceRequest's storage layer, but its
disk-persisted pending-request-with-expiration shape and its
`INPUT_REQUIRED` notification type are both relevant prior art worth citing
in the architecture-research phase as "the existing analog for
request-that-blocks-a-session," distinct from the ent-backed
`BacklogActivityNote` precedent for "the existing analog for a durable
per-item record everyone can see."
