# Research: Technology Stack for backlog-item-activity-log

All citations are `path:line` against commit `ff91d6e4c` (main, HEAD at research time) in
`/Users/tstapler/code/github.com/tstapler/stapler-squad`.

## Headline finding: a near-identical mechanism already exists

`BacklogProgressNote` (ent schema) + `AppendProgressNote`/`ListProgressNotesForItem`
(repository) + `progress_notes` (proto field 27 on `BacklogItem`) + `ProgressHistorySection`
(web UI) is **already** an append-only, per-item note log with its own ent table, proto
message, event-bus wiring, and UI section. It is missing exactly one thing the new feature
needs: **author/provenance** (`session UUID`, `session title`) — `BacklogProgressNote` has no
author field at all, only `criterion_index`, `note`, `status`, `created_at`
(`session/ent/schema/backlogprogressnote.go:22-42`). It is also written only by the
role-gated `report_progress` tool and by two internal callers
(`server/services/backlog_service_lifecycle.go:780`, `session/storage.go:941`), never by an
ungated caller.

The schema comment already documents an item-level (non-per-criterion) sentinel:
`criterion_index` is `Min(-1)`, and **`-1` is the established convention for "this note isn't
about one AC criterion"** (`session/ent/schema/backlogprogressnote.go:27-30`), used today by
`SetBacklogItemPRAndTransition`'s status-transition note
(`server/services/backlog_service_lifecycle.go:780`) and by an observed-drift summary note
(`session/storage.go:941`). This means the *shape* of "one freeform item-level note" is
already a first-class, exercised code path — the only structural gap is provenance.

This gives two viable designs for Phase 3 to choose between (not decided here, per
requirements.md's Open Questions):

1. **Extend `BacklogProgressNote`** with two new optional/nillable fields (e.g.
   `author_session_uuid string`, `author_title string`) and call `AppendProgressNote` (or a
   new sibling method) with `criterion_index = -1` for ad hoc updates. Reuses the existing
   table, proto message, `ProgressHistorySection` UI, and event path with minimal new surface
   — but conflates "official AC-criterion-status audit trail" with "anyone's freeform note" in
   one table/UI section, and the `status` field (currently `"pending"/"in_progress"/"done"/"fail"`,
   comment at `session/ent/schema/backlogprogressnote.go:37`) has no natural value for a pure
   comment.
2. **New dedicated ent type** (e.g. `BacklogItemUpdate`/`BacklogActivityNote`) modeled directly
   on `BacklogProgressNote`'s file (same field/edge/index shape, swap `criterion_index`+`status`
   for `author_session_uuid`+`author_title`). Keeps the "official progress audit trail" and
   "anyone-can-post note" concepts separate, matching requirements.md's explicit exclusion of
   touching the gated tools' semantics, at the cost of a second near-identical table/proto
   message/UI section.

Either way, the **regen command, edge pattern, proto field-numbering slot, event-bus kind, and
web UI section are all already fully precedented** by `BacklogProgressNote` end to end — see
below.

---

## 1. ent ORM schema conventions

- Schema files: `session/ent/schema/*.go`. One type per file, no shared base.
- Regen command (verbatim, from the `//go:generate` directive):
  `session/ent/generate.go:3`:
  ```go
  //go:generate go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema
  ```
  Note this is a `go:generate` directive living in package `ent` at `session/ent/generate.go`,
  so the `./schema` path is relative to `session/ent/`; run via `go generate ./session/ent/...`
  or `cd session/ent && go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema`.
  CLAUDE.md and requirements.md both flag `--feature sql/upsert` as required — omitting it
  breaks `UpsertRule` and similar generated methods.

- **`BacklogItem` schema** (`session/ent/schema/backlog_item.go`): ~35 fields (title,
  description, acceptance_criteria as JSON string, priority, status, various timestamps/flags),
  edges to `ItemSession`, `Session`, `BacklogStatusEvent`, `BacklogStuckState`,
  `BacklogProgressNote`, `ItemSource`, `BacklogItemDependency` (`session/ent/schema/backlog_item.go:172-205`).
  All child-list edges use `edge.To(..., X.Type).Annotations(entsql.OnDelete(entsql.Cascade))`
  (e.g. `edge.To("progress_notes", BacklogProgressNote.Type).Annotations(entsql.OnDelete(entsql.Cascade))`,
  `session/ent/schema/backlog_item.go:180-181`).

- **Exact precedent for a "child list" type with an edge back to the parent — `BacklogProgressNote`**
  (`session/ent/schema/backlogprogressnote.go`, full file, 60 lines):
  ```go
  type BacklogProgressNote struct{ ent.Schema }

  func (BacklogProgressNote) Fields() []ent.Field {
      return []ent.Field{
          field.UUID("id", uuid.UUID{}).Default(uuid.New),
          field.UUID("item_id", uuid.UUID{}),
          field.Int("criterion_index").Min(-1), // -1 sentinel = item-level note
          field.String("note").Optional(),
          field.String("status").Comment("... pending, in_progress, done, fail"),
          field.Time("created_at").Default(time.Now).Immutable(),
      }
  }

  func (BacklogProgressNote) Edges() []ent.Edge {
      return []ent.Edge{
          edge.From("item", BacklogItem.Type).
              Ref("progress_notes").
              Field("item_id").
              Unique().
              Required(),
      }
  }

  func (BacklogProgressNote) Indexes() []ent.Index {
      return []ent.Index{
          index.Fields("item_id", "created_at"), // "all notes for this item, in order"
      }
  }
  ```
  A new activity-log type (whether it's a new type or an extension of this one) should copy
  this exact `edge.From(...).Ref(...).Field("item_id").Unique().Required()` shape and the
  `index.Fields("item_id", "created_at")` index for ordered-history queries.

- **`ReviewVerdict`** (`session/ent/schema/review_verdict.go`) is the other one-to-one/edge
  precedent, but it's `edge.From(...).Ref("review_verdict").Unique().Required()` off
  `ItemSession` (one verdict per session), not a many-per-item list — less relevant than
  `BacklogProgressNote`.

## 2. MCP tool registration pattern

File: `server/mcp/tools_backlog.go` (2649 lines), package `mcp`.

### Identity/gating helpers (lines 24-70)

Three tiers exist today, and the new tool needs a **fourth, not currently present**:

1. **`callerSessionUUID(ctx)`** (`server/mcp/tools_backlog.go:46-52`) — returns the session
   UUID from context or a hard `PERMISSION_DENIED`-shaped error. Used by
   `report_progress`/`request_review`/`report_blocked`/`report_duplicate`/`report_pr_created`/
   `submit_triage_result`/`update_session_task`/`list_workspace_peers` — every tool whose
   write "actually depends on the caller's session identity" (doc comment,
   `server/mcp/tools_backlog.go:39-45`).
2. **`callerSessionUUIDForAudit(ctx)`** (`server/mcp/tools_backlog.go:63-70`) — returns the
   session UUID from context, or the sentinel string `"manual"` (`manualCallerSentinel`,
   line 63) — **never errors**. Used today only by `create_backlog_item`
   (`server/mcp/tools_backlog.go:1734`) and `import_github_issue` (line 1815), purely for an
   audit-trail log line (`log.InfoLog.Printf("[mcp:create_backlog_item] session=%s ...", callerUUID, ...)`,
   line 1787). **This is the closest existing precedent for "callable from any session, with
   or without STAPLER_SESSION_UUID"** — the new tool should use this, not `callerSessionUUID`.
3. **`tools_goal.go`'s `session_id`-param-with-fallback pattern** — see next section. This adds
   an *explicit* identity resolution path on top of tier 1/2, letting a caller name a
   *different* session's identity instead of (or in addition to) its own.

None of the three tiers today combine "no error if absent" with "record session *title*, not
just UUID" — `callerSessionUUIDForAudit` only returns the UUID (or `"manual"`), so a new
handler must separately resolve the `*session.Instance` (via `h.store.LoadInstances()` +
match on UUID, the same lookup `findInstanceByUUID` in `tools_goal.go:384-397` performs) to
get `.Title` (`session.Instance.Title`, `session/instance.go:128`).

### `tools_goal.go`'s exact `session_id` + env-fallback pattern (406 lines)

`set_session_goal`'s handler (`server/mcp/tools_goal.go:94-184`) is the concrete pattern named
in requirements.md. Verbatim:

```go
// server/mcp/tools_goal.go:100-116
// Priority: (1) session_id param if provided, (2) callerSessionUUID from context.
var targetUUID string
var resolvedInst *session.Instance
if sessionID, ok := args["session_id"].(string); ok && sessionID != "" {
    inst, errR := h.findInstanceByID(sessionID)
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

Tool definition (`server/mcp/tools_goal.go:28-31`):
```go
mcpgo.NewTool("set_session_goal",
    mcpgo.WithDescription("... If session_id is omitted, the calling session's UUID is used (agent self-reporting). ..."),
    mcpgo.WithString("session_id",
        mcpgo.Description("Session ID (title) of the target session. Optional — defaults to the calling session if unset."),
    ),
    ...
```

**Important caveat for the new tool**: `set_session_goal`'s fallback branch still calls
`callerSessionUUID(ctx)`, which **errors** when `STAPLER_SESSION_UUID` is unset
(`server/mcp/tools_goal.go:111`) — so `tools_goal.go`'s pattern does *not* by itself handle
"no `session_id` AND no `STAPLER_SESSION_UUID`" gracefully; it only avoids requiring
`STAPLER_SESSION_UUID` when the caller supplies `session_id` explicitly. To satisfy
requirements.md's "callable from any session... with or without STAPLER_SESSION_UUID" (i.e.
graceful with *neither*), the new tool's fallback must use `callerSessionUUIDForAudit` (tier 2
above), not `callerSessionUUID`, when `session_id` is absent — i.e. combine `tools_goal.go`'s
`session_id`-param resolution with `tools_backlog.go`'s never-erroring audit sentinel, rather
than copying either helper unmodified.

### Where the new tool would be registered

- Handler struct: extend `backlogHandlers` (`server/mcp/tools_backlog.go:215-231` — fields
  `storage *session.Storage`, `store session.InstanceStore`, `eventBus *events.EventBus`,
  `enabledCheck func() bool`, etc.) or add the method there; no new dependency is needed beyond
  what it already has.
- Registration call site: `registerBacklogTools(s, &backlogHandlers{...})` is invoked from
  `server/mcp/server.go:66`; add the new `s.AddTool(mcpgo.NewTool("post_backlog_update", ...), h.postBacklogUpdate)` block inside `registerBacklogTools` (`server/mcp/tools_backlog.go:2309-2649`), following the exact shape of the `report_progress`/`create_backlog_item` blocks (e.g. `server/mcp/tools_backlog.go:2363-2385`).
- Error/result helpers already available: `errResult`/`okResult` (`server/mcp/tools_discovery.go:73,79`), `MCPResult` (`server/mcp/types.go:14`), error-code constants incl. `ErrInvalidArgument = "INVALID_ARGUMENT"` and `ErrPermissionDenied = "PERMISSION_DENIED"` (`server/mcp/types.go:63`, `server/mcp/tools_backlog.go:85`).
- `get_backlog_item`'s registration/description (`server/mcp/tools_backlog.go:2310-2319`) would need its description updated once the new update log is exposed there, and `getBacklogItem`'s handler (wherever it builds the JSON response — same file) would need to include the new field, mirroring how `progress_notes` already rides along on the `BacklogItem` proto/JSON shape.

## 3. Proto / ConnectRPC change surface

File: `proto/session/v1/backlog.proto` (not `session.proto` — `BacklogItem` and its RPCs live
in the dedicated `backlog.proto` file within the same `session.v1` package;
`proto/session/v1/session.proto` exists but doesn't define `BacklogItem`).

- **Existing `BacklogProgressNote` proto message** (`proto/session/v1/backlog.proto:141-147`):
  ```proto
  message BacklogProgressNote {
    string id = 1;
    int32 criterion_index = 2;
    string note = 3;
    string status = 4; // "pending", "in_progress", "done", "fail"
    google.protobuf.Timestamp created_at = 5;
  }
  ```
- **`BacklogItem` message**, field 27 is the existing precedent to mirror
  (`proto/session/v1/backlog.proto:150-205`):
  ```proto
  repeated BacklogProgressNote progress_notes = 27;
  ```
  The last field currently defined is `plan_rejected_at = 34` (line 204), so a new
  `repeated BacklogItemUpdate activity_log = 35;` (or reusing/extending `BacklogProgressNote`
  itself, per the Phase-3 design decision above) is the next free field number.
- **`GetBacklogItemRequest`/`GetBacklogItemResponse`** (`proto/session/v1/backlog.proto:302-308`):
  ```proto
  message GetBacklogItemRequest { string item_id = 1; }
  message GetBacklogItemResponse { BacklogItem item = 1; }
  ```
  `GetBacklogItemResponse` wraps the full `BacklogItem`, so no separate RPC/message change is
  needed beyond the new field on `BacklogItem` itself — `GetBacklogItem`
  (`proto/session/v1/backlog.proto:878`) and `WatchBacklogItems`
  (`proto/session/v1/backlog.proto:1049`, `rpc WatchBacklogItems(WatchBacklogItemsRequest) returns (stream BacklogItemEvent) {}`)
  both transit full `BacklogItem` snapshots (see §4), so they pick up the new field
  automatically once it exists on the message and the Go↔proto mapper is updated.
- Regen workflow: `make proto-gen` (`Makefile:411`, target depends on `ensure-tools` and
  `web-app/node_modules/.modules.yaml` — i.e. run from repo root, generates both Go
  (`gen/proto/go/session/v1/`) and TypeScript (web-app) bindings in one pass).
- The actual Go↔proto conversion for `BacklogItem.progress_notes` — the mapper a new field
  must extend — lives in `server/services/backlog_service.go:785-796`:
  ```go
  if len(item.ProgressNotes) > 0 {
      protoNotes := make([]*sessionv1.BacklogProgressNote, len(item.ProgressNotes))
      for i, n := range item.ProgressNotes {
          protoNotes[i] = &sessionv1.BacklogProgressNote{ ... }
      }
      p.ProgressNotes = protoNotes
  }
  ```

## 4. Event bus / live update plumbing

Three layers, all already exercised by `report_progress`'s sibling `UpdateAcCriterionStatus`
flow — the exact pattern a new "update posted" event should follow:

1. **Repository layer publishes after the DB write** — `session/storage_backlog.go:1084-1102`
   (`UpdateAcCriterionStatus`): after `r.client.BacklogItem.UpdateOneID(...).Save(ctx)`, it
   calls
   ```go
   result := backlogItemToData(updated)
   r.publishItemChanged(ctx, &result, BacklogItemChange{
       Kind:          ChangeItemUpdated,
       UpdatedFields: []string{"acceptanceCriteria"},
   })
   ```
   with the comment "Best-effort publish: never blocks or fails the ... update itself." A new
   `AppendActivityUpdate`-style repository method should do the same:
   `r.publishItemChanged(ctx, &result, BacklogItemChange{Kind: ChangeItemUpdated, UpdatedFields: []string{"activityLog"}})`
   (or a new dedicated `ChangeKind` if the design wants a distinct wire event — see below).
   Note `AppendProgressNote` itself (`session/ent_repository_backlog.go:2033-2049`) does
   **not** currently publish an event at all — only `UpdateAcCriterionStatus` does, for the
   `acceptance_criteria` field it also touches. A brand-new activity-log write path has no
   existing publish call to imitate 1:1 other than this one.
2. **`session.BacklogItemChange`** (`session/backlog_item_change.go:36-63`) is the
   package-internal DTO (`Kind`, `OldStatus`, `NewStatus`, `UpdatedFields`, `SessionID`,
   `ClaimantHostID`, `ArchivedAt`, `RemovedReason`, `Verdict`) — mirrored 1:1 by
   `events.BacklogItemEventPayload` (`pkg/events/types.go:59-90`) via the adapter
   `BacklogItemEventPublisher.PublishItemChanged` (`server/services/backlog_item_event_publisher.go:32-56`),
   which exists specifically to break the import cycle (`session` cannot import `pkg/events`
   since `pkg/events` imports `session`). Adding a new `BacklogChangeKind` (e.g.
   `ChangeActivityUpdatePosted`) requires updating **both** enums
   (`session/backlog_item_change.go:20-29` and `pkg/events/types.go:37-53`) **and** the
   explicit switch `mapBacklogChangeKind` (`server/services/backlog_item_event_publisher.go:64-83`,
   which `panic`s on an unmapped kind, caught by the adapter's own `recover()`).
3. **`WatchBacklogItems` consumes and converts** — `server/services/backlog_service_events.go`:
   - `convertEventToBacklogItemEvent` (lines 251-343) switches on `payload.Kind`; the
     `BacklogChangeItemUpdated, BacklogChangeTriageProgressUpdated` case (lines 307-319) is the
     one to extend/reuse — it builds a `BacklogItemEvent_ItemUpdated` wire event carrying the
     full updated `BacklogItem` snapshot plus `UpdatedFields`, which is exactly the
     "field changed on the item, re-fetch/re-render it" semantics an activity-log post needs
     (no new proto event-message required, matching the comment at lines 307-311: "A ... write
     reuses this same wire event rather than a new proto message").
   - `WatchBacklogItems` RPC (`server/services/backlog_service_events.go:84-90`) →
     `watchBacklogItems` core logic (lines 95-205): subscribes to `s.eventBus` before building
     the snapshot (race-safety comment lines 104-107), then either replays `EventsSince(afterSeq)`
     (reconnect) or sends one `BacklogItemEvent_ItemUpdated` snapshot per visible item
     (fresh connect, `snapshotEventForItem`, lines 228-244), then fans out live events
     (lines 186-204) filtered by `backlogItemMatchesFilters` (lines 213-224, status/category
     only — an activity-log post on an item already passing those filters will be delivered
     with no new filter logic needed).
   - No changes needed to `WatchBacklogItemsRequest`/`BacklogItemEvent` proto messages if the
     `ChangeItemUpdated` kind is reused; a new dedicated `BacklogChangeKind` would need a new
     case in `convertEventToBacklogItemEvent`'s switch but could still reuse the
     `BacklogItemEvent_ItemUpdated` variant rather than adding a new oneof member.

## 5. Existing sanitization precedent

Two distinct, both real, precedents exist — requirements.md names the first; the second is
more directly analogous to a stored freeform note field.

### PR #534 / commit `7b9aee4cd` — path-traversal sanitization (the one requirements.md names)

`git show 7b9aee4cd` (full diff: `.claude/docs/fixed/BUG-079-triage-title-path-traversal.md`,
`server/services/backlog_service_triage.go`, `server/services/backlog_service_triage_test.go`).
Introduces `sanitizeTriageTitle` (`server/services/backlog_service_triage.go`, added right
after the existing `slugify` helper):
```go
func sanitizeTriageTitle(title, itemID string) string {
    if s := slugify(title); s != "" {
        return s
    }
    return "item-" + itemID[:min(len(itemID), 8)]
}
```
Root cause: `session.ParseHeadlessTriageResult`'s LLM-controlled `Title` field reached a
`filepath.Join` (path traversal via `"../../../etc/passwd"`-shaped titles), a git commit
message, and a branch rename, all unsanitized. Fix reuses the existing `slugify()` helper
(strips everything but lowercase alnum/hyphen, so `".."` and path separators can't survive) and
falls back to an itemID-derived slug when sanitization empties the string. **This precedent is
about a string used as a path/branch/commit-message segment** — it is the right pattern *if*
the new freeform note text is ever interpolated into a filesystem path, shell command, or
branch/commit name, but a plain activity-log note (stored as data, rendered as text) is not
that shape of risk.

### `sanitizeField`/`SanitizeForAgentContext` — the more directly applicable precedent

`session/backlog_context.go:12-27`:
```go
var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

func SanitizeForAgentContext(s string, maxLen int) string { return sanitizeField(s, maxLen) }

func sanitizeField(s string, maxLen int) string {
    s = htmlTagRe.ReplaceAllString(s, "")
    if len(s) > maxLen {
        s = s[:maxLen] + " [truncated]"
    }
    return s
}
```
This is the sanitizer `BacklogProgressNote.note`'s own schema comment points to — "Rendered
call sites are responsible for truncation (see sanitizeField); stored unbounded here"
(`session/ent/schema/backlogprogressnote.go:35`) — confirming the repo's convention is
**store raw/unbounded, sanitize+truncate at every render-into-another-context call site**, not
at write time. The concrete call site: `writeFullNotesHistorySection`
(`session/backlog_review.go:153-170`) renders every `ProgressNoteData.Note` into a review/triage
LLM prompt via `sanitizeField(n.Note, 300)` (line 167) — i.e. the *same* free-text note field
this new feature extends is already re-injected into another agent's prompt context today, and
already treats that as an injection surface worth stripping HTML tags from and capping length
on. A new freeform activity-log note field should apply the identical `sanitizeField` treatment
at any point it is rendered back into an LLM prompt (triage/review context, another session's
`get_backlog_item` output if that's ever templated into a prompt) — and, per requirements.md's
explicit sanitization mandate, at minimum enforce an input length cap at the MCP tool boundary
(matching `set_session_goal`'s `if len(goal) > 2000 { return errResult(...) }` pattern,
`server/mcp/tools_goal.go:122-124`) even before any prompt-rendering call site exists.

## 6. Web UI

Component: `web-app/src/components/backlog/BacklogItemDetail.tsx` (1602 lines) — main detail
page, imports and composes many `./detail/*Section` subcomponents (e.g. `NotesSection` at
line 58, `ProgressHistorySection` at line 57).

- **Live subscription**: `useWatchBacklogItems` hook
  (`web-app/src/lib/hooks/useWatchBacklogItems.ts`), invoked at
  `web-app/src/components/backlog/BacklogItemDetail.tsx:233`
  (`const { connectionState } = useWatchBacklogItems();`) — this is the ConnectRPC streaming
  client for `WatchBacklogItems` (§4); its `ItemUpdated`/`StatusChanged`/etc. events update a
  shared cache the detail page reads from, which is why extending the existing
  `BacklogChangeItemUpdated`/`ItemUpdated` wire event (rather than adding a new oneof variant)
  means **no new frontend event-handling code is needed** — the existing subscription already
  re-delivers the full updated `BacklogItem` (including any new field) on every relevant
  mutation.
- **Exact component precedent to clone for an "Activity Log" section — `ProgressHistorySection`**
  (`web-app/src/components/backlog/detail/ProgressHistorySection.tsx`, full file, 67 lines):
  ```tsx
  export function ProgressHistorySection({ item, defaultExpanded }: ProgressHistorySectionProps) {
    const { visible, hasMore, remaining, showAll } = useShowMore(
      item.id, "progress-history", item.progressNotes, SHOW_MORE_CAP /* = 8 */
    );
    if (item.progressNotes.length === 0) return null;
    return (
      <CollapsibleSection sectionKey="progress-history" title="Progress History" defaultExpanded={defaultExpanded}>
        <div className={styles.section}>
          <div className={styles.progressNoteList} role="list" aria-label="Implementer progress history">
            {visible.map((n) => (
              <div key={n.id} className={styles.progressNoteItem} role="listitem">
                <div className={styles.progressNoteMeta}>
                  <span>Criterion #{n.criterionIndex}</span><span>·</span><span>{n.status}</span>
                  {n.createdAt && (<><span>·</span><span>{formatDate(n.createdAt)}</span></>)}
                </div>
                {n.note && <span>{n.note}</span>}
              </div>
            ))}
          </div>
          {hasMore && <button onClick={showAll} data-testid="progress-history-show-more">Show {remaining} more</button>}
        </div>
      </CollapsibleSection>
    );
  }
  ```
  Wired into the page at `web-app/src/components/backlog/BacklogItemDetail.tsx:1573`
  (`<ProgressHistorySection item={item} defaultExpanded={progressHistoryExpanded} />`), with
  its collapse-state persisted via `useSectionExpandState(itemId, "progress-history", false)`
  (line 364) — a new `ActivityLogSection` would follow the identical
  `CollapsibleSection` + `useShowMore` + `useSectionExpandState` composition, one new
  `sectionKey` string, and a new entry in the `["progress-history", ...]`-shaped array at
  line 424-425.
- **Data mapping**: `web-app/src/lib/hooks/useBacklogService.ts` — imports
  `BacklogProgressNote as BacklogProgressNoteProto` (line 17), defines the frontend
  `ProgressNote` interface (line 284) and `mapProgressNote(n: BacklogProgressNoteProto): ProgressNote`
  (line 419), wired into the full `BacklogItem` mapper at line 534
  (`progressNotes: (p.progressNotes ?? []).map(mapProgressNote)`). A new activity-log field
  needs the identical proto-import → interface → mapper-function → mapper-field-assignment
  chain in this one file.

## 7. Feature registry

Directory: `docs/registry/features/{backend,frontend}/` (a newer two-directory layout;
`docs/registry/features/*.json` also has some older top-level files like `analytics.json` —
new backend/frontend entries should go in the subdirectories, matching every backlog-related
entry found).

- **Backend entries** live under `docs/registry/features/backend/backlog/` (e.g.
  `get-item.json`, `create-item.json`, `archive-item.json`, `approve-plan.json` — 20+ files,
  one per RPC). Shape (`docs/registry/features/backend/backlog/get-item.json`):
  ```json
  {
    "id": "backlog:get-item",
    "type": "backend",
    "service": "BacklogService",
    "method": "GetBacklogItem",
    "protoFile": "proto/session/v1/backlog.proto",
    "markerFound": true,
    "handlerFile": "server/services/backlog_service_query.go",
    "tested": false,
    "testIds": [],
    "lastModified": "2026-07-11T09:32:29.851760768-07:00"
  }
  ```
  A `// +api: backlog:watch`-style marker already exists on `WatchBacklogItems`
  (`server/services/backlog_service_events.go:83`, confirming the `// +api: <id>` comment
  convention CLAUDE.md describes); a new/changed RPC or MCP tool touching `GetBacklogItem`
  wouldn't need a new marker (the method itself doesn't change), but a **new MCP tool**
  (`post_backlog_update`) is not an RPC and doesn't go through this backend registry shape at
  all today — none of the 20+ backend files are for MCP tools specifically (they're all
  `service`/`method`/`protoFile` keyed), so it's unclear from this directory alone whether MCP
  tools get their own registry entries; this should be confirmed against
  `.claude/docs/feature-registry.md` / `make registry-generate`'s actual scan targets during
  planning rather than assumed.
- **Frontend entries** live under `docs/registry/features/frontend/` (flat, not nested per
  feature — e.g. `backlog-stuck-items.json`, `backlog-category-selector.json`,
  `backlog-pipeline-mode-selector.json`). Shape
  (`docs/registry/features/frontend/backlog-stuck-items.json`):
  ```json
  {
    "id": "backlog-stuck-items",
    "type": "frontend",
    "component": "StuckItemsSection",
    "path": "web-app/src/components/backlog-stuck/StuckItemsSection.tsx",
    "markerLine": 1,
    "tested": true,
    "testIds": [ "...", "StuckItemsSection_should_..." ],
    "lastModified": "2026-08-14T00:00:00Z"
  }
  ```
  This confirms the `// +feature: <id>` marker (CLAUDE.md: "in first 10 lines of React files")
  convention — a new `ActivityLogSection.tsx` would need `// +feature: backlog-activity-log`
  (or similar) in its first 10 lines, then `make registry-generate` to produce this JSON file
  automatically.
- Regen commands per CLAUDE.md: `make registry-generate` (scan → update per-feature files),
  `make registry-diff` (dry run), `make registry-aggregate` (local-only monolithic JSON).

---

## Summary of concrete file-touch list this stack implies (informational, not a plan)

- `session/ent/schema/backlogprogressnote.go` (extend) **or** a new sibling schema file,
  regenerated via `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema`
  from `session/ent/`.
- `session/ent_repository_backlog.go` (new/extended `Append*`/`List*` methods, mirroring
  lines 2033-2073), `session/storage.go` (thin wrapper, mirroring lines 1319-1334),
  `session/storage_backlog.go` (event-publish call, mirroring lines 1084-1102).
- `session/backlog_item_change.go` + `pkg/events/types.go` +
  `server/services/backlog_item_event_publisher.go` only if a new `BacklogChangeKind` is
  introduced (optional — reusing `ChangeItemUpdated` avoids all three).
- `proto/session/v1/backlog.proto` (new field on `BacklogItem`, next free number `35`) →
  `make proto-gen`.
- `server/services/backlog_service.go:785-796`-style mapper extension.
- `server/mcp/tools_backlog.go` (new tool + registration in `registerBacklogTools`).
- `web-app/src/lib/hooks/useBacklogService.ts` (proto import, interface, mapper).
- `web-app/src/components/backlog/detail/` new `ActivityLogSection.tsx` (clone of
  `ProgressHistorySection.tsx`) wired into `BacklogItemDetail.tsx`.
- `docs/registry/features/frontend/` new JSON (via `make registry-generate` after adding a
  `// +feature:` marker) and a backend entry if MCP tools turn out to be in-scope for the
  registry (confirm during planning).
