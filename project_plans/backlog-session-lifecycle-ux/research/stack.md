# Stack Research: backlog-session-lifecycle-ux

## 1. ent ORM — version, patterns, best template to copy

**Version**: `entgo.io/ent v0.14.5` (go.mod). Generate command is pinned in
`session/ent/generate.go`:
```
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema
```
Always use this exact command (not the bare `ent generate`) — omitting
`--feature sql/upsert` silently breaks `UpsertRule`-style methods.

**Best template for the new respawn-event table**: `session/ent/schema/backlog_status_event.go`,
not `item_session.go` or `backlog_stuck_state.go`. It is already the exact
shape requirements.md asks for — an **append-only** audit-trail row per
event, FK'd to `BacklogItem` by a plain `item_id` field (not a full ent edge
declared on both sides), immutable `created_at`, and a composed
`(item_id, created_at)` index for "all events for an item, ordered by time"
queries:

```go
type BacklogStatusEvent struct{ ent.Schema }

func (BacklogStatusEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("item_id", uuid.UUID{}),
		field.String("from_status"),
		field.String("to_status"),
		field.String("triggered_by").Default("user"),
		field.String("note").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (BacklogStatusEvent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("item", BacklogItem.Type).
			Ref("status_events").Field("item_id").Unique().Required(),
	}
}

func (BacklogStatusEvent) Indexes() []ent.Index {
	return []ent.Index{index.Fields("item_id", "created_at")}
}
```

A `RespawnEvent` schema should mirror this near-verbatim: `id`, `item_id`,
`reason` (string, mirrors the call site: `AutoRespawnReview` /
`AutoRespawnAutonomousWork` / `AutoRespawnTriage` /
`RemediateStaleWorkSession`), `triggering_session_uuid` (loose FK, string —
`item_session.go`'s `session_uuid` field uses this exact "loose FK, not an
ent edge" pattern with a `Comment` explaining why), `resulting_session_uuid`
(same pattern, optional/nillable since a respawn attempt could conceivably
fail before a new session exists), immutable `created_at`, and a
`(item_id, created_at)` index. Two closely-related but distinct existing
patterns to note:
- `item_session.go` uses the "loose FK via plain string field, not an ent
  edge" pattern for linking to `Session` (a separate storage layer, not an
  ent-managed entity) — the same reasoning applies here since sessions
  aren't ent-managed.
- `backlog_stuck_state.go` is a *resolve-in-place* (not append-only) model
  with a 2-column unique index — explicitly the wrong template; requirements.md
  calls for append-only, and `backlog_status_event.go` is the append-only
  precedent already in the codebase for exactly this kind of audit trail.

Also note: `session/ent/schema/item_session.go`'s `end_reason` field already
exists and is persisted (line 33-36, comment documents the classification
bucket: `shutdown`/`timeout`/`process_error`/`claude_not_found`/`other`) —
no ent schema change is needed for `end_reason` surfacing, only proto +
frontend work (see §2 below, it is currently missing from the `ItemSession`
proto message).

## 2. Proto / ConnectRPC patterns for read-only list RPCs

**File**: `proto/session/v1/backlog.proto` (single file, ~1000+ lines,
covers all backlog-related messages/RPCs). Service is `BacklogService`
(line 720). Generated bindings land at:
- Go: `session/gen/session/v1/*.go`
- TS: `web-app/src/gen/session/v1/*_pb.ts`

Both regenerated via `make proto-gen` (buf-based; `github.com/bufbuild/buf
v1.57.2` in go.mod, `@bufbuild/protoc-gen-es ^2.11.0` / `@bufbuild/protobuf
^2.11.0` / `@connectrpc/connect ^2.1.1` / `@connectrpc/connect-web ^2.1.1`
in web-app/package.json).

**Confirmed gap**: `ItemSession` proto message (line 66-84) does **not**
currently expose `end_reason` — the ent field exists and is populated, but
was never added to the wire message. This is in-scope, low-risk proto work:
add `string end_reason = 18;` (next available field number) to the
`ItemSession` message, thread it through the Go→proto mapper (wherever
`ItemSession` rows are converted — search `server/services/` for the
`ItemSession` construction site), regenerate, then consume in
`SessionsSection.tsx`.

**Closest existing precedent for the new respawn-event list RPC**:
`ListStuckBacklogItems` (proto lines 835-838, message defs 1020-1061) — a
read-only list RPC returning `repeated StuckBacklogItem`, with an empty
request message (`ListStuckBacklogItemsRequest {}`) since it lists *all*
open items globally. The new RPC differs in being scoped to a single item
(matches `GetBacklogItemDiff`/`GetBacklogItemCost` shape instead — request
takes `string item_id`). Recommended shape:

```protobuf
message ListRespawnEventsRequest {
  string item_id = 1;
}
message RespawnEvent {
  string id = 1;
  string reason = 2;
  string triggering_session_uuid = 3;
  string resulting_session_uuid = 4;
  google.protobuf.Timestamp created_at = 5;
}
message ListRespawnEventsResponse {
  repeated RespawnEvent events = 1;
}

// in service BacklogService:
rpc ListRespawnEvents(ListRespawnEventsRequest) returns (ListRespawnEventsResponse) {}
```

Alternative worth considering: eagerly embed `repeated RespawnEvent
respawn_events` directly on `BacklogItem` (like `status_events` and
`progress_notes` already are, per the comment at proto line 140-142 "eagerly
loaded alongside status_events") rather than a separate RPC — avoids an
extra round-trip for the item-detail page, at the cost of always loading it
even when the collapsed-by-default timeline is never expanded. Given
requirements.md explicitly asks for "New RPC(s) to read per item," a
separate scoped RPC (not embedding) is the intended design — worth
confirming against plan.md once written, but the requirements text reads as
deliberate (avoids bloating the already-large `BacklogItem` payload on
every board fetch for data most users won't expand).

## 3. Collapsible/CollapsibleGroup + timeline list precedent

**File**: `web-app/src/components/ui/Collapsible.tsx`. Built on
`@radix-ui/react-accordion`. Two exports:

- `CollapsibleGroup` — wraps children in one shared `Accordion.Root
  type="multiple"`, so sibling `CollapsibleSection`s share Radix's
  roving-tabindex keyboard nav (Home/End/Arrow spanning all headers in the
  group). Props: `defaultValue?: string[]` (uncontrolled), `value?:
  string[]` + `onValueChange?` (controlled).
- `CollapsibleSection` — a single progressive-disclosure section. Props:
  `sectionKey` (unique key, also localStorage suffix), `title`,
  `defaultExpanded?` (standalone mode only — ignored with a dev-only console
  warning if used inside a group and it diverges from the group's own
  state), `onExpandedChange?` (standalone only), `children`. Auto-detects
  whether it's inside a `CollapsibleGroup` via context and renders either
  just an `Accordion.Item` (grouped) or its own implicit single-item
  `Accordion.Root` (standalone) — existing call sites need no changes
  either way.

**Direct fit, no new primitive needed.** The requirements.md ask — "UI:
collapsed-by-default timeline in item detail" — is already a solved pattern
in this codebase: `web-app/src/components/backlog/detail/WorkflowHistorySection.tsx`
is the template to copy almost verbatim. It renders `BacklogStatusEvent[]`
(the other existing append-only audit trail) as:
- `<CollapsibleSection sectionKey="workflow" title="Workflow"
  defaultExpanded={defaultExpanded}>` wrapping
- a `role="list"` container (`styles.workflowTimeline`) of `role="listitem"`
  rows, each showing a from/to/meta line + optional note,
- fed through the `useShowMore(item.id, "workflow", item.statusEvents, 8)`
  hook (`web-app/src/lib/hooks/useShowMore.ts`) which caps default rendering
  to the 8 most recent and exposes `hasMore`/`remaining`/`showAll`,
- and always renders even with zero events (explicit "No status history
  recorded." empty state — the comment explains this is deliberate: hiding
  the section entirely for old data made the feature look broken).

A `RespawnHistorySection.tsx` for the new timeline should copy this
structure: `CollapsibleSection` + `useShowMore` + `role="list"`/`"listitem"`
+ explicit empty state, styled via a new `RespawnHistorySection.css.ts`
(vanilla-extract, sibling pattern to `WorkflowHistorySection.css.ts`) rather
than reusing `WorkflowHistorySection.css` directly (component-colocated CSS
is the repo convention — see `.claude/rules/css-architecture.md`).

`BacklogItemDetail.tsx` is where existing detail sections
(`WorkflowHistorySection`, `SessionsSection`, `NotesSection`, etc.) are
composed inside the page's top-level `CollapsibleGroup` — the new
`RespawnHistorySection` (or the respawn timeline embedded inside
`SessionsSection.tsx`, since requirements.md scopes it to "item detail")
should be registered there alongside the others to inherit the shared
group's keyboard nav and `defaultValue` expand/collapse state.

**`Session.pause_reason` badge precedent**: currently only a `Tooltip` on
hover (`web-app/src/components/sessions/SessionCard.tsx` lines 491-493,
using `formatPauseReason` from `web-app/src/lib/sessions/formatPauseReason.ts`),
gated behind `isPaused && session.pauseReason`. Promoting to an
always-visible badge means rendering that same formatted string in a
non-hover badge element (no existing "badge" primitive found in this pass —
likely a small inline `<span>` styled via `.css.ts`, consistent with how
other status pills are done elsewhere in `SessionCard.tsx`; a follow-up
Explore pass on `SessionCard.css.ts` would confirm the existing badge/pill
token names to reuse, e.g. from `--warning-bg`/`--error-bg` in
`globals.css`).

## 4. Versions/deps summary

| Dep | Version | Source |
|---|---|---|
| `entgo.io/ent` | v0.14.5 | go.mod |
| `connectrpc.com/connect` | v1.19.0 | go.mod |
| `connectrpc.com/otelconnect` | v0.8.0 | go.mod |
| `github.com/bufbuild/buf` | v1.57.2 | go.mod |
| `github.com/bufbuild/connect-go` | v1.10.0 | go.mod |
| `google.golang.org/protobuf` | v1.36.11 | go.mod |
| `@bufbuild/protobuf` | ^2.11.0 | web-app/package.json |
| `@bufbuild/protoc-gen-es` | ^2.11.0 | web-app/package.json |
| `@connectrpc/connect` | ^2.1.1 | web-app/package.json |
| `@connectrpc/connect-web` | ^2.1.1 | web-app/package.json |
| `@vanilla-extract/css` | ^1.20.1 | web-app/package.json |
| `@vanilla-extract/recipes` | ^0.5.7 | web-app/package.json |
| `@vanilla-extract/next-plugin` | ^2.5.1 | web-app/package.json |
| `@radix-ui/react-accordion` | (used by Collapsible.tsx; version not checked — see web-app/package.json `@radix-ui/*` block if pin matters) | web-app/package.json |

## Open questions for plan.md

1. Confirm whether the new respawn-event RPC should be item-scoped
   (`ListRespawnEvents(item_id)`, matching `GetBacklogItemDiff`/
   `GetBacklogItemCost`) or embedded eagerly on `BacklogItem` (matching
   `status_events`/`progress_notes`). Leaning toward scoped RPC per
   requirements.md's explicit "New RPC(s)" wording and to avoid bloating
   the board-list payload.
2. Confirm the write-side wiring: the 4 call sites in
   `server/services/backlog_service_triage.go`
   (`AutoRespawnReview`/`AutoRespawnAutonomousWork`/`AutoRespawnTriage`/
   `RemediateStaleWorkSession`) currently only call `log.InfoLog` — each
   needs a `RespawnEvent` row insert added alongside (not instead of) the
   existing log call. Not yet read in this pass; do so before planning
   Task breakdown.
3. Confirm no existing "badge" component/token exists for
   `pause_reason`/`end_reason` promotion before inventing new CSS — a
   follow-up grep of `SessionCard.css.ts` for existing pill/badge styles is
   needed.
