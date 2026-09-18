# Architecture Research: backlog-item-activity-log

Research for the requirements in `project_plans/backlog-item-activity-log/requirements.md`.
All claims below are cited `file:line` against the repo at commit `ff91d6e4c` (HEAD at research time,
2026-08-18, branch `main`).

## Headline finding

**There is already an exact structural precedent for this feature**: `BacklogProgressNote`, the
append-only log backing `report_progress`'s "full history of notes across a work session." It spans
every layer named in the task (ent schema → repository → storage → proto → RPC → MCP tool →
frontend section component), and the new "backlog item activity log" capability should be built as a
sibling of it, not a new pattern. The one thing it does **not** already solve — and the one place the
new feature must deliberately do better — is live event delivery (see Finding 4).

---

## 1. Overall layering — call-chain diagram (report_progress)

```
MCP client (agent)
   │ tools/call report_progress {item_id, criteria_index, status, note}
   ▼
server/mcp/tools_backlog.go:767  backlogHandlers.reportProgress()
   │  1. featureDisabledResult() gate                                    :768
   │  2. callerSessionUUID(ctx) — HARD-FAILS if STAPLER_SESSION_UUID     :771  (see Finding 2)
   │     is not present in ctx (env var or HTTP header, see below)
   │  3. validate item_id / criteria_index / status args                :778-804
   │  4. h.storage.GetItemSessionBySessionAndItem(ctx, callerUUID, itemID) — the
   │     role-gate check: PERMISSION_DENIED if this session isn't linked  :809-815
   │  5. h.storage.UpdateAcCriterionStatus(ctx, itemID, idx, acStatus, note) :826
   │  6. h.storage.AppendProgressNote(ctx, itemID, idx, note, acStatus)   :834  (best-effort,
   │                                                                        logged not failed on error)
   ▼
session/storage.go:1309  Storage.UpdateAcCriterionStatus()  — thin type-assert-and-forward
   ▼                        to *EntRepository
session/storage_backlog.go:1051  EntRepository.UpdateAcCriterionStatus()
   │  - Get(ctx, id)                                                     :1057
   │  - parse+mutate the AcceptanceCriteria JSON blob in Go               :1065-1078
   │  - UpdateOneID(...).SetAcceptanceCriteria(...).Save(ctx)             :1084-1089
   │  - r.publishItemChanged(ctx, &result, BacklogItemChange{             :1099
   │        Kind: ChangeItemUpdated, UpdatedFields: []string{"acceptanceCriteria"}})
   ▼
session/ent_repository_backlog.go:1594  EntRepository.publishItemChanged()
   │  - attachItemSessionsForPublish(ctx, item)  (eager-loads ItemSessions only) :1601
   │  - publishItemChangedSnapshot(item, change)                          :1602
   ▼
session/ent_repository_backlog.go:1621  EntRepository.publishItemChangedSnapshot()
   │  - nil-checks r.itemChangePublisher, recover()-wraps the call        :1622-1629
   │  - r.itemChangePublisher.PublishItemChanged(item, change)            :1630
   ▼
server/services/backlog_item_event_publisher.go:32  BacklogItemEventPublisher.PublishItemChanged()
   │  (the cross-package adapter — session/ cannot import pkg/events, see :11-19)
   │  - maps session.BacklogChangeKind -> events.BacklogChangeKind        :44, :58-83
   │  - builds events.BacklogItemEventPayload                            :43-54
   │  - p.Bus.Publish(events.NewBacklogItemChangedEvent(payload))         :55
   ▼
pkg/events/types.go:218  NewBacklogItemChangedEvent() → *events.Event{Type: EventBacklogItemChanged}
   ▼
EventBus.Publish (pkg/events bus; assigns Seq, fans out to all subscriber channels)
   ▼
server/services/backlog_service_events.go:95  BacklogService.watchBacklogItems()
   │  - subscribed via eventBus.Subscribe(ctx) before building the snapshot :108
   │  - live loop: reads evt off eventCh, filters Type/ItemID/status+category :186-203
   │  - convertEventToBacklogItemEvent(evt, costFor)                        :200, :251
   │     switch payload.Kind case BacklogChangeItemUpdated:
   │       → sessionv1.BacklogItemEvent_ItemUpdated{ItemUpdated: &BacklogItemUpdatedEvent{
   │           ItemId, UpdatedFields, Item: protoItem, IsSnapshot}}          :307-319
   │  - sender.Send(converted) — a connect.ServerStream[BacklogItemEvent]    :200
   ▼
ConnectRPC streaming response over HTTP (WatchBacklogItems RPC, +api: backlog:watch
   server/services/backlog_service_events.go:83)
   ▼
web-app/src/lib/hooks/useWatchBacklogItems.ts:128  useWatchBacklogItems()
   │  - opens the stream via createClient(BacklogService, transport)         :186
   │  - for-await loop over the stream (not shown above line 220 read, but
   │    referenced at :171-176's doc comment: plain HTTP, not the WS bridge)
   │  - dispatch(upsertItem(item)) into Redux                                (backlogItemsSlice)
   ▼
web-app/src/lib/store/backlogItemsSlice.ts:49  upsertItem reducer
   │  - REPLACES state.items[id] with the incoming proto-shaped item, with one
   │    itemSessions-specific anti-clobber guard (:55-67) — see Finding 4.
   ▼
web-app/src/components/backlog/BacklogItemDetail.tsx:57 imports
web-app/src/components/backlog/detail/ProgressHistorySection.tsx:23  ProgressHistorySection()
   - renders item.progressNotes (plain React text interpolation — no dangerouslySetInnerHTML,
     no markdown parser; XSS-safe by construction) :36-52
```

Two important side branches off this chain:

- **`report_progress`'s history write (`AppendProgressNote`, step 6 above) never calls
  `publishItemChanged` at all.** Confirmed by reading the full function body,
  `session/ent_repository_backlog.go:2033-2049` — it is a bare `r.client.BacklogProgressNote.Create()`
  with no publish call. Only the *criterion-status* write (step 5, `UpdateAcCriterionStatus`)
  publishes an event, and that event's `Item` snapshot is built from the object `Save()` just
  returned — which was never eager-loaded with `WithProgressNotes` — so the live event's embedded
  item has an **empty/stale `progress_notes` list** even though a note was just appended. See
  Finding 4 for why this matters to the new feature.
- **`get_backlog_item` (`server/mcp/tools_backlog.go:291`) and the `GetBacklogItem` RPC both read
  through `Storage.GetBacklogItem` → `EntRepository.GetBacklogItem`
  (`session/ent_repository_backlog.go:337-361`), which *does* eager-load `ProgressNotes` via
  `WithProgressNotes(...)` (`:349-351`)** — so a full re-fetch always shows current progress notes;
  only the *live push* path is stale.

---

## 2. Session identity resolution

### 2a. `session_id` param + `STAPLER_SESSION_UUID` env-fallback precedent

`server/mcp/tools_goal.go`'s `set_session_goal` (`setSessionGoal`, `:94-184`) is the exact precedent
requirements.md points to:

```go
// server/mcp/tools_goal.go:100-116
// Priority: (1) session_id param if provided, (2) callerSessionUUID from context.
var targetUUID string
var resolvedInst *session.Instance
if sessionID, ok := args["session_id"].(string); ok && sessionID != "" {
    inst, errR := h.findInstanceByID(sessionID)   // resolves by title/ID, not UUID
    if errR != nil {
        return errR, nil
    }
    targetUUID = inst.UUID
    resolvedInst = inst
} else {
    callerUUID, err := callerSessionUUID(ctx)
    if err != nil {
        return errResult(ErrPermissionDenied, "session_id is required or call from within a Stapler Squad session", "Provide session_id or set STAPLER_SESSION_UUID."), nil
    }
    targetUUID = callerUUID
}
```

This is a **hard-fail** pattern (`callerSessionUUID`, `server/mcp/tools_backlog.go:46-52`) — it errors
if there's no session UUID in context and no explicit `session_id` was given. The new tool must NOT
use this variant, because per requirements.md it must work even for a caller with no session identity
at all (a bare `claude mcp add` from a terminal). The correct existing precedent for that is
`callerSessionUUIDForAudit` (`server/mcp/tools_backlog.go:54-70`):

```go
// server/mcp/tools_backlog.go:63-70
const manualCallerSentinel = "manual"

func callerSessionUUIDForAudit(ctx context.Context) string {
    if uuid, ok := sessionUUIDFromContext(ctx); ok {
        return uuid
    }
    return manualCallerSentinel
}
```

— already used by `create_backlog_item`/`import_github_issue` for exactly this "log who called this,
but never reject the call for lacking identity" purpose (see its doc comment, same file
`:54-62`). **This is the function the new tool's provenance capture should call.**

### 2b. Where the session UUID actually enters `ctx`

Two independent injection paths feed `sessionUUIDFromContext` (`server/mcp/tools_backlog.go:34-37`):

1. **stdio MCP transport** — `RunServer` reads the env var directly and wraps the root context once,
   before the stdio loop starts:
   ```go
   // server/mcp/server.go:111-114
   if uuid := os.Getenv("STAPLER_SESSION_UUID"); uuid != "" {
       ctx = WithSessionUUID(ctx, uuid)
       log.InfoLog.Printf("[mcp] session UUID injected from environment: %s", uuid)
   }
   ```
2. **HTTP MCP transport** (the primary path today) — a per-request middleware reads a header instead
   of an env var, because the HTTP server process's own env var would be wrong (it's the *caller's*
   session UUID that matters, not the server's):
   ```go
   // server/server.go:709-715
   mcpWithUUID := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
       if uuid := r.Header.Get("X-Stapler-Session-UUID"); uuid != "" {
           r = r.WithContext(servermcp.WithSessionUUID(r.Context(), uuid))
       }
       mcpHTTPHandler.ServeHTTP(w, r)
   })
   ```
   The header itself is populated client-side when a spawned session's Claude Code MCP config is
   written: `session/instance_tmux.go:355-360` bakes
   `"headers":{"X-Stapler-Session-UUID":%q}` into the per-session MCP server config JSON using the
   instance's own UUID. `server/mcp/proxy.go:46-54` mirrors this for the stdio-proxy-to-HTTP path
   (reads `STAPLER_SESSION_UUID` env, forwards it as the same header).

Either way, by the time a handler runs, `sessionUUIDFromContext(ctx)` (`tools_backlog.go:34-37`) is
the single chokepoint — the new tool only needs to call `callerSessionUUIDForAudit(ctx)`, not worry
about which transport it arrived over.

### 2c. Resolving a session **title** from a UUID

No such lookup exists on `session.Storage`/ent (session titles are not persisted per-ItemSession row
today — `ItemSession` proto, `proto/session/v1/backlog.proto:77-121`, has no title field). The
existing pattern for turning a UUID into a `Title` is an **in-memory instance scan**, already used
twice in `tools_goal.go`:

```go
// server/mcp/tools_goal.go:384-397
// findInstanceByUUID finds an instance by its UUID (used for cache updates).
// Returns nil, nil if not found (non-fatal).
func (h *goalHandlers) findInstanceByUUID(uuid string) (*session.Instance, *mcpgo.CallToolResult) {
    instances, err := h.store.LoadInstances()
    if err != nil {
        return nil, nil
    }
    for _, inst := range instances {
        if inst.UUID == uuid {
            return inst, nil
        }
    }
    return nil, nil
}
```

`session.Instance.Title` is a plain exported field (`session/instance.go:122-128`:
`ID string; Title string; UUID string; ...`). `backlogHandlers` (the struct the new tool's handler
would live on) **already carries `store session.InstanceStore`**
(`server/mcp/tools_backlog.go:217`), so a `findInstanceByUUID`-style helper is a same-file, same-struct
addition — no new dependency needed. Caveat: this lookup only finds *currently loaded* instances
(`LoadInstances()` reads live/known sessions); a session that has since been torn down will resolve to
"" and the provenance record should fall back to just the raw UUID in that case (never fail the post
over a missing title — matches `callerSessionUUIDForAudit`'s fail-open spirit).

---

## 3. Consistency / concurrency model

Two different patterns coexist in this codebase for backlog item writes, and the new feature should
copy the first, not the second:

### 3a. Pure-append pattern (safe under concurrency, no locking needed) — **the one to copy**

`BacklogProgressNote`'s writer does a single `INSERT`, nothing else:

```go
// session/ent_repository_backlog.go:2033-2049
func (r *EntRepository) AppendProgressNote(ctx context.Context, itemID string, criterionIndex int, note, status string) error {
    parsedItemID, err := uuid.Parse(itemID)
    ...
    _, err = r.client.BacklogProgressNote.Create().
        SetItemID(parsedItemID).
        SetCriterionIndex(criterionIndex).
        SetNote(note).
        SetStatus(status).
        Save(ctx)
    ...
}
```

No read-modify-write, no mutex, no CAS precondition — and none is needed: two concurrent callers each
issue an independent `INSERT`, and the database's own row-level atomicity guarantees both rows land
(ordering between them is whatever `created_at`/autoincrement gives, which is exactly the "ordered
list of entries" shape requirements.md asks for). This is the reason `BacklogProgressNote` needed no
concurrency design at all — it sidesteps the hazard entirely by never re-reading a shared field. **The
new "activity log" append should use this exact shape**: a dedicated ent type with its own table (see
Finding 5), never a JSON blob field mutated in Go.

### 3b. Read-modify-write pattern (the hazard to avoid)

By contrast, `UpdateAcCriterionStatus` — the *other* half of `report_progress` — reads the whole
`BacklogItem.acceptance_criteria` JSON blob, mutates one array element in Go, and writes the whole
blob back:

```go
// session/storage_backlog.go:1057-1089
item, err := r.client.BacklogItem.Get(ctx, parsedID)
...
criteria, parseErr := ParseAcCriteria(AcCriteriaJSON(item.AcceptanceCriteria))
...
criteria[criterionIndex].Status = AcStatus(status)
...
serialized, serErr := SerializeAcCriteria(criteria)
...
updated, err := r.client.BacklogItem.UpdateOneID(parsedID).
    SetAcceptanceCriteria(string(serialized)).
    Save(ctx)
```

This has **no optimistic-locking guard** (no `Where(updated_at = ...)` CAS clause) — two concurrent
`report_progress` calls updating *different* criteria on the same item can lose one write to a
last-write-wins race on the shared JSON blob. This is a real, pre-existing gap, but it is explicitly
**out of scope** for this feature (requirements.md's "Must not weaken or change the behavior of
report_progress...") — noted here only so planning doesn't accidentally imitate this shape for the new
append-only log. If a future item wants to fix it, the precedent for CAS on this table already exists
one level up, at the whole-item-status layer (next section).

### 3c. Optimistic locking, where it IS used

`TransitionBacklogItemStatus` (status changes, not appends) uses an explicit CAS precondition:

```go
// session/repository.go:750-760
// BacklogItemPrecondition is used for optimistic locking on update/transition.
type BacklogItemPrecondition struct {
    ExpectedStatus    string
    ExpectedUpdatedAt *time.Time
    Note              string
}
```
```go
// session/ent_repository_backlog.go:1359-1361 (one of three call sites, cf. :853-856, :1562-1564)
if latest.Status != precondition.ExpectedStatus {
    return nil, fmt.Errorf("%w: expected status %q, got %q", ErrPreconditionFailed, precondition.ExpectedStatus, latest.Status)
}
```
`ErrPreconditionFailed` is defined at `session/repository.go:13`. This is the right tool for
"transition item from status A to B, fail if someone else already moved it" — not applicable to an
append-only list, which has no "current value" to race on.

**Conclusion for planning**: the new update-log write is structurally identical to
`AppendProgressNote` (3a) — a single `Create()` call against its own table, no mutex, no CAS. No new
concurrency primitive is needed.

---

## 4. Event bus data flow — and the gap the new feature must not repeat

### 4a. Today's Go event type

`pkg/events/types.go` defines the whole vocabulary:

- `EventBacklogItemChanged EventType = "backlog_item_changed"` (`:30`) — the one `Event.Type` value
  relevant here.
- `BacklogChangeKind` (`:35-54`) — currently 7 values: `status_transition`, `verdict_recorded`,
  `session_attached`, `item_updated`, `item_archived`, `item_removed`, `triage_progress_updated`.
  **A new update-posted event needs an 8th kind here** (mirrored in `session/backlog_item_change.go`'s
  parallel `BacklogChangeKind` — see below for why there are two).
- `BacklogItemEventPayload` (`:59-90`) — the untyped grab-bag struct carried by `Event`. Fields are
  populated ad hoc per `Kind` (e.g. `OldStatus`/`NewStatus` only for `status_transition`,
  `SessionID` only for `session_attached`). A new kind would add its own field(s) here (e.g. a
  pointer to the new update DTO) following the same "only the fields relevant to Kind are populated"
  convention stated in the struct's own doc comment (`:57-58`).
- `Event` (`:94-135`) wraps `Seq` (assigned by the bus on `Publish`), `Timestamp`, and
  `BacklogItemPayload`.

### 4b. Why there are two `BacklogChangeKind` types

`session/backlog_item_change.go` defines its **own** copy of the same enum
(`session.BacklogChangeKind`, `:11-30`) plus `BacklogItemChange` (`:36-64`, the ent-repository-facing
struct) and the `ItemChangePublisher` interface (`:78-80`) — because `session/` cannot import
`pkg/events` (`pkg/events` imports `session` for `*session.Instance`/`*session.BacklogItemData`, so
the reverse import would cycle; stated explicitly at `:7-10`). `server/services/backlog_item_event_publisher.go`
is the adapter that lives in a package that can see both, converting one enum to the other via an
explicit switch (`mapBacklogChangeKind`, `:64-83`) that **panics on an unmapped kind** (caught by its
own `recover()`, `:33-37`, and logged, never propagated). **Any new `BacklogChangeKind` value must be
added to both enums and to this switch**, or a real event silently vanishes into the panic-recover log
line instead of reaching subscribers.

### 4c. Construction and publish call sites

Every backlog-mutation write path that wants live delivery calls
`(*EntRepository).publishItemChanged(ctx, item, change)` (`session/ent_repository_backlog.go:1594-1602`,
convenience wrapper) or its lower-level twin `publishItemChangedSnapshot` (`:1621-1631`, used only by
`DeleteBacklogItem` — see its doc comment for why). Both are nil-safe and `recover()`-guarded so a
publish failure can never fail the write that triggered it (`:1622-1629`). The adapter itself
(`BacklogItemEventPublisher.PublishItemChanged`, `server/services/backlog_item_event_publisher.go:32-56`)
wraps its whole body in a second, independent `recover()` (`:33-37`) — belt-and-suspenders against a
panic anywhere in payload construction. **`AppendProgressNote` conspicuously does NOT call either of
these** (confirmed by reading its full body, `session/ent_repository_backlog.go:2033-2049`) — this is
the gap flagged in Finding 1's side-branch note.

### 4d. `WatchBacklogItems`'s handling (already documented per-hop in Finding 1)

Recap of the two delivery phases in `server/services/backlog_service_events.go:95-205`:

- **Fresh connect** (`after_seq` unset): iterates `s.storage.ListBacklogItems(ctx, ...)`
  (`:152-166`) and sends one `BacklogItemEvent_ItemUpdated` per item as a synthetic snapshot
  (`snapshotEventForItem`, `:228-244`). **`ListBacklogItems` does not eager-load `ProgressNotes`**
  (confirmed: no `WithProgressNotes` call anywhere in its body,
  `session/ent_repository_backlog.go:651-` onward) — so a client that first learns about an item via
  this snapshot branch (rather than an explicit `GetBacklogItem` call) never receives its progress-note
  history over the wire at all, silently. Any new "updates" field on `BacklogItem` proto would have
  the identical blind spot unless `ListBacklogItems` is also changed to eager-load it, which is a
  real cost/latency tradeoff (loading the full history for every item in a list-scoped query) that
  planning should weigh explicitly — the "separate `ListBacklogUpdates` RPC" option in Finding 5 avoids
  this entirely by never bundling the list into every `BacklogItem` read.
- **Reconnect** (`after_seq` set): replays buffered events since that sequence via
  `s.eventBus.EventsSince(msg.GetAfterSeq())` (`:124`), force-marking `IsSnapshot: true`
  (`forceIsSnapshot`, `:399-412`) to avoid double-toast/double-`aria-live` on the frontend.
- **Live loop**: blocks on `eventCh`, filters by `Type`/status/category, converts, sends
  (`:186-204`).

### 4e. The concrete clobbering risk for the new feature

`web-app/src/lib/store/backlogItemsSlice.ts:49-72`'s `upsertItem` reducer **replaces**
`state.items[id]` wholesale with whatever `BacklogItem` the event carries, with exactly one
carve-out — an anti-clobber guard for `itemSessions` specifically (`:55-67`, comment explains it
exists because "a partially-loaded event push ... racing a fully-loaded one" was already a known
failure mode for that field). **There is no equivalent guard for `progressNotes`.** Combined with
Finding 4c (AC-status-change events publish a snapshot whose `Item` was never eager-loaded with
`WithProgressNotes`), this means: today, a live `report_progress` AC-status event can push an `Item`
with an empty `progress_notes` array, and the reducer will happily stomp a previously-populated
`state.items[id].progressNotes` down to empty until the next full `GetBacklogItem`/`listBacklogItems`
refresh silently repairs it. **This is the one existing behavior the new feature must not blindly
mirror.** Two ways to avoid inheriting it:

1. Always eager-load the new updates list on every `BacklogItemData` snapshot passed into
   `publishItemChanged` (expensive/verbose — same cost concern as 4d) and add an
   `upsertItem`-style anti-clobber guard for the new field, mirroring the existing `itemSessions` one.
2. Give the new event kind its **own** dedicated payload (a single new update entry, not the whole
   item) and have the frontend reducer *append* it to a per-item list rather than replace the item's
   list wholesale — the same shape `applyInlineVerdict`
   (`web-app/src/lib/hooks/useWatchBacklogItems.ts:99-119`) already uses for verdict data: patch a
   specific sub-field from the event's own inline payload rather than trusting the embedded `Item`
   snapshot to be complete.

Option 2 is the cleaner mirror of an already-proven pattern in this codebase and sidesteps 4d's
list-query cost question entirely.

---

## 5. Proto message design options

### 5a. Existing shape to extend or mirror

`proto/session/v1/backlog.proto`:

```proto
// :141-147
message BacklogProgressNote {
  string id = 1;
  int32 criterion_index = 2;
  string note = 3;
  string status = 4; // "pending", "in_progress", "done", "fail"
  google.protobuf.Timestamp created_at = 5;
}
```

```proto
// :150-205 (BacklogItem message; relevant lines only)
message BacklogItem {
  ...
  repeated ItemSession item_sessions = 18;
  ...
  repeated BacklogStatusEvent status_events = 20;
  ...
  // progress_notes is the implementer's append-only report_progress audit
  // trail — eagerly loaded alongside status_events (see GetBacklogItem).
  repeated BacklogProgressNote progress_notes = 27;
  ...
  google.protobuf.Timestamp plan_rejected_at = 34;   // highest field number in use today
}
```

The wire event carrying `BacklogChangeItemUpdated`/`BacklogChangeTriageProgressUpdated` today:

```proto
// referenced via server/services/backlog_service_events.go:307-319 — exact proto message
// definition is BacklogItemUpdatedEvent (not read verbatim above, but its Go usage shows its shape):
//   ItemId string; UpdatedFields []string; Item *BacklogItem; IsSnapshot bool
```

### 5b. Option A — `repeated BacklogUpdate updates = 35;` on `BacklogItem` (mirror progress_notes exactly)

- New proto message `BacklogUpdate { string id; string author_session_uuid; string author_session_title;
  string message; google.protobuf.Timestamp created_at; }`, field-numbered as the next free slot
  (`35`, since `34` is the highest in use per `:204`).
- Add `repeated BacklogUpdate updates = 35;` to `BacklogItem` (mirrors `progress_notes = 27` exactly).
- **Requires**: `make proto-gen` (regenerates `gen/proto/go/session/v1` and
  `web-app/src/gen/session/v1/backlog_pb.ts`); a new ent schema type (`session/ent/schema/backlogupdate.go`,
  copy `backlogprogressnote.go`'s shape verbatim — `field.UUID("id")`, `field.UUID("item_id")`,
  `field.String("message")`, `field.String("author_session_uuid").Optional()`,
  `field.String("author_session_title").Optional()`, `field.Time("created_at")`); an `edge.To("updates",
  BacklogUpdate.Type)` on `BacklogItem`'s `Edges()` (mirror `:180-181`); ent regen via
  `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
  (`session/ent/generate.go`); `WithUpdates(...)` eager-load in `GetBacklogItem`
  (mirror `:349-351`); a `backlogItemToData`/`backlogItemToProto` field mapping (mirror `:236-239` and
  `server/services/backlog_service.go:785-796`); and a new `session.BacklogChangeKind` +
  `events.BacklogChangeKind` pair (Finding 4b) if live delivery goes through the whole-item snapshot.
- **Tradeoff**: reuses every existing rendering/eager-load/registry convention exactly, but inherits
  Finding 4d/4e's cost-and-clobber concerns if it also rides the `BacklogItem` snapshot for live
  delivery — some mitigation (item-scoped `ListBacklogUpdates`-style fetch, or an anti-clobber guard)
  is still needed regardless of which storage shape is chosen.

### 5c. Option B — separate `ListBacklogUpdates` RPC + dedicated event payload, no `BacklogItem` field

- Same new proto message (`BacklogUpdate`) and same ent schema/storage layer as Option A, but
  **not** embedded as a `repeated` field on `BacklogItem` at all.
- New RPC `rpc ListBacklogUpdates(ListBacklogUpdatesRequest) returns (ListBacklogUpdatesResponse);`
  on `BacklogService`, requested on demand by `BacklogItemDetail` (mirrors how `ItemSession`s already
  get their own dedicated fetch path in places, though most of `BacklogItem`'s edges are in fact
  eager-loaded inline — this option is the one genuine structural deviation from today's pattern).
  Requires its own `+api:` marker and a `docs/registry/features/` entry (`make registry-generate`).
- New oneof variant on `BacklogItemEvent` — e.g. `BacklogItemUpdatePostedEvent{ItemId, Update
  *BacklogUpdate}` — carrying **only the new entry**, not a full `Item` snapshot. The frontend reducer
  appends this single entry to a per-item updates list (new backlogItemsSlice sub-state, or extend the
  existing `BacklogItem` domain object client-side without it being wire-carried on every full-item
  push) — the `applyInlineVerdict`-style approach from Finding 4e/Option 2.
- **Tradeoff**: more new surface area (new RPC + registry entries + a client-side merge instead of a
  wholesale replace) but cleanly avoids both the "every list/snapshot query now eager-loads unbounded
  history" cost (4d) and the clobber risk (4e) by construction — the update log is never part of the
  object `upsertItem` replaces wholesale.

### 5d. Recommendation surfaced for planning (not a decision — planning's call per requirements.md)

Given Finding 4e is a *real, currently-live* clobbering risk and Option A's mirror-exactly path
inherits it, Option A is only clean if paired with the same kind of anti-clobber guard
`backlogItemsSlice.ts:55-67` already uses for `itemSessions` — extending that guard to
`updates`/`progressNotes` fixes both the new feature and (retroactively) documents the same
pre-existing gap for `progressNotes`. Option B sidesteps the problem by construction at the cost of a
new RPC + registry entries. Both are viable; this is exactly the "1-2 concrete options ... for the
planner to decide" requirements.md asks for.

---

## Citation index (all files referenced above)

| File | Relevant lines |
|---|---|
| `server/mcp/tools_backlog.go` | 34-37, 46-52, 54-70, 215-287, 291-415, 767-841 |
| `server/mcp/tools_goal.go` | 94-184, 371-397 |
| `server/mcp/server.go` | 106-118 |
| `server/mcp/proxy.go` | 46-55 |
| `server/server.go` | 708-718 |
| `session/instance.go` | 122-151 |
| `session/instance_tmux.go` | 355-360 |
| `session/storage.go` | 1309-1335 |
| `session/storage_backlog.go` | 1048-1105 |
| `session/ent_repository_backlog.go` | 152-153, 236-239, 336-361, 651-691, 1302, 1359-1361, 1594-1631, 2026-2071 |
| `session/backlog_item_change.go` | 1-81 |
| `session/repository.go` | 13, 433-493, 750-760 |
| `session/ent/schema/backlog_item.go` | 20-217 |
| `session/ent/schema/backlogprogressnote.go` | 1-59 |
| `session/ent/generate.go` | (command reference, per CLAUDE.md) |
| `pkg/events/types.go` | 27-135, 217-224 |
| `server/services/backlog_item_event_publisher.go` | 1-84 |
| `server/services/backlog_service_events.go` | 1-413 |
| `proto/session/v1/backlog.proto` | 77-121, 137-205 |
| `web-app/src/lib/hooks/useWatchBacklogItems.ts` | 1-220 |
| `web-app/src/lib/hooks/useBacklogService.ts` | 171, 534 |
| `web-app/src/lib/store/backlogItemsSlice.ts` | 48-74 |
| `web-app/src/components/backlog/BacklogItemDetail.tsx` | 57, 364, 424, 1573 |
| `web-app/src/components/backlog/detail/ProgressHistorySection.tsx` | 1-67 |
| `session/backlog_context.go` | 1-37 |
| `server/services/backlog_service.go` | 785-796 |
