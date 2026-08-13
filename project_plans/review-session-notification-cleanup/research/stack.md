# Stack Research: Notifications for Headless Review/Triage Sessions

Agent 1 (Stack) — Phase 2 research for `review-session-notification-cleanup`.

## 1. Event bus (`events.NewNotificationEvent` / `eventBus.Publish`)

- Package: `pkg/events` (`pkg/events/bus.go`, `types.go`). `server/events/forward.go`
  re-exports everything via type aliases and `var` function assignments so
  `server/...` code imports `server/events` but gets identical types/funcs —
  purely a transparent forwarding layer, not a second implementation.
- **Not** backed by any broker/queue (no Kafka/NATS/Redis). `EventBus` is a
  plain in-process pub/sub over Go channels:
  - `subscribers map[string]chan *Event`, one buffered channel per subscriber
    (`Subscribe(ctx)` returns `(<-chan *Event, id string)`, auto-unsubscribes
    on `ctx.Done()`).
  - `Publish(event)` assigns a monotonic `Seq`, appends to an in-memory ring
    buffer (`eventBufTTL = 1h`, `eventBufMaxLen = 10_000`, pruned every
    publish), then fans out non-blockingly (`select { case ch<-event: default:
    /* drop */ }`) — a slow subscriber never blocks others, but can silently
    miss events (mitigated by `EventsSince(afterSeq)` replay on reconnect).
  - `NewNotificationEvent(sessionID, sessionName, notificationID,
    notificationType, priority, title, message, metadata map[string]string)
    *Event` (`pkg/events/types.go:222-245`) just stamps an `Event{Type:
    EventNotification, ...}` struct — no persistence side effect itself.
- **Implication for this feature**: `eventBus.Publish` only fans out to
  live streaming subscribers; it is *not* where notification history is
  persisted. Persistence happens separately (see §2) in
  `server/services/notification_service.go` is NOT actually the persistence
  writer either — grep shows the JSON-file store's `Append`/`AppendDedup` is
  called elsewhere (subscriber-side), not inline with every `Publish` call
  site. Any suppression fix must therefore be applied at the *call sites*
  that construct `NewNotificationEvent(...)` for TASK_COMPLETE/IDLE/STALE
  (`server/review_queue_manager.go:355` `OnItemAdded`, and whatever emits
  the headless-triage equivalent — still to be pinpointed per the open
  question in requirements.md) — publishing fewer events is the fix, not
  filtering after publish, since after `Publish` the event has already
  reached the ring buffer and any subscriber.

## 2. ORM (ent) for `ItemSession`/`BacklogItem` — notifications persistence is NOT ent-backed

- `entgo.io/ent v0.14.5` (go.mod). Codegen invocation is pinned in
  `session/ent/generate.go`:
  ```go
  //go:generate go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema
  ```
  i.e. `sql/upsert` is already enabled repo-wide (per
  `.claude/rules/ent-schema-generation.md`) — no new feature flag needed for
  this item unless a new schema field requires further ent features.
- `session/ent/schema/item_session.go`: `ItemSession` has `session_uuid`
  (plain `field.String`, explicitly commented "Loose FK to Session; not an
  ent edge") plus a **required, unique** `edge.From("backlog_item",
  BacklogItem.Type)`. An index exists specifically for this lookup:
  `index.Fields("session_uuid")` — comment: `// CRITICAL: O(1) lookup on
  every EventExited hook`. `session_role` is a plain string field ("One of:
  work, triage, review" — not an ent enum type, just a documented
  convention), matching the requirement's `SessionRole` review/triage check.
- **Important finding**: `server/notifications/store.go`
  (`NotificationHistoryStore`) is a **flat JSON-file-backed store**, entirely
  separate from ent/SQLite. `NotificationRecord` structs are
  marshaled/unmarshaled to a single `notificationsFile{Version, UpdatedAt,
  Notifications []*NotificationRecord}` JSON document
  (`loadFromDisk`/`saveToDisk`), guarded by an in-process mutex. There is
  **no ent schema for notifications** — so AC2 (`item_id` in metadata) and
  AC3 (pruning) require no ent migration; they're pure Go/JSON-store logic.
  `enforceRetention()` (`store.go:437`) already does the age/count pruning
  AC3 needs to extend — add an existence check there (or a sibling pass),
  not a new ent query path.
- **Reusable lookup already implemented** for `session_uuid → BacklogItem`
  (exactly what AC2 needs): `EntRepository.GetItemSessionBySessionUUID(ctx,
  sessionUUID) (ItemSessionSummary, error)`
  (`session/storage_backlog.go:181-195`+). Loads the `backlog_item` edge
  (`WithBacklogItem()`), orders by `created_at desc` and takes first match
  (session_uuid is not unique — a session can be reused), returns
  `session.ErrNotFound` (sentinel, wraps `ent.IsNotFound`) when absent.
  `ItemSessionSummary.BacklogItemID` is what should map to `item_id` in
  notification metadata. This is the method to call from the notification
  metadata producer rather than writing a new ent query.

## 3. Go test conventions near session/notification code

- Standard library `testing` only for these packages — **no testify use** in
  `session/review_queue_determiner_test.go`, `server/review_queue_manager_test.go`,
  or `server/notifications/store_test.go` (confirmed via grep for
  `testify|require\.|assert\.` — zero hits in all three), even though
  `github.com/stretchr/testify v1.11.1` is present in `go.mod` and used
  elsewhere in the repo. New tests for this feature should match the local
  convention: plain `if got != want { t.Errorf(...) }` / `t.Fatalf(...)`,
  no assertion library dependency for these packages.
- Two naming styles coexist, pick whichever matches the file you're adding
  to:
  - Table-driven with a single top-level func + `t.Run(tt.name, ...)`
    subtests: `TestDefaultStatusDeterminer_Determine` in
    `review_queue_determiner_test.go` (struct-of-cases + loop).
  - BDD-style descriptive names, one func per scenario:
    `TestOnItemAdded_NotificationUsesStableID`,
    `TestOnItemAdded_NotificationFallsBackToTitleWhenNoMatch`,
    `TestMaybeAutoCreatePR_DoesNothing_When_ReasonNotTaskComplete` in
    `server/review_queue_manager_test.go` — pattern is
    `Test<Subject>_<should/does>_<Effect>_When_<Condition>`. This matches
    the `feature-testing-registry.md` rule's naming convention used
    elsewhere in the repo (`dispatchOmnibarAction_should_<effect>_When_<action>`),
    so AC4's regression test should follow this BDD style, e.g.
    `TestOnItemAdded_SuppressesNotification_When_SessionHidden` /
    `..._When_SessionRoleIsReviewOrTriage`.
  - `server/review_queue_manager_test.go` already has the fixture scaffolding
    to extend directly: `TestOnItemAdded_EventBusBehavior_BUG001` and the two
    `TestOnItemAdded_Notification*` tests above set up a `ReactiveQueueManager`
    + fake `eventBus` + `session.ReviewItem` and assert on published events —
    the new suppression test can reuse this harness, just constructing an
    `Instance` with `Hidden: true` (or an `ItemSession` with `SessionRole:
    "review"/"triage"`) and asserting `eventBus` received zero
    `EventNotification` events for that item.
- Shared test helpers live in a top-level `testutil` package (imported by
  `server/review_queue_manager_test.go` as
  `github.com/tstapler/stapler-squad/testutil`) — check there before writing
  new fixture boilerplate.

## 4. Existing "does this session/instance still exist" lookup (for AC3 pruning)

Two candidates exist; they answer different questions — pick based on what
"exists" should mean for pruning:

- **`session.Storage.FindInstanceDataByID(id string) (*InstanceData, error)`**
  (`session/storage.go:407-418`) — disk-backed (`ListInstanceData()` reads
  persisted instance state), matches by stable ID or title
  (`InstanceData.MatchesID`), returns sentinel `session.ErrInstanceDataNotFound`
  when absent. **This is the right one for AC3**: it reflects durable
  instance records, not just currently-live/monitored ones, so a notification
  for a session that finished and was cleaned up from the poller's live set
  but still has on-disk history resolves correctly either way. No new code
  needed to answer "does this instance still exist" — reuse this directly
  from wherever the pruning pass runs (likely alongside
  `NotificationHistoryStore.enforceRetention()`, since `Storage` and
  `NotificationHistoryStore` would need to be wired together — check whether
  `NotificationHistoryStore` currently has any reference to `*session.Storage`
  or needs one threaded in via `server/dependencies.go`).
- **`session.ReviewQueuePoller.FindInstance(sessionID string) *Instance`**
  (`session/review_queue_poller.go:895-907`) — in-memory only, iterates
  `rqp.instances` (the poller's currently-monitored live set), returns `nil`
  if not found. This is what `OnItemAdded`/`SendNotification` already use to
  resolve session name/stable-ID (`server/review_queue_manager.go:349-353`,
  `server/services/notification_service.go:91-96`) — good for "is this
  session currently being monitored" but **not** a reliable "does it still
  exist at all" check, since instances can be legitimately absent from the
  poller's live set (e.g. after tmux server restart) while still having
  valid persisted history. Do not use this alone for AC3's existence check.

Neither helper is ent-backed; both operate over `session.Storage`'s
JSON/on-disk `InstanceData`, consistent with §2's finding that notification
persistence and instance persistence are both flat-file, not SQL/ent.
