# Stack Research: durable-guidance-request

## New dependencies

**None.** Every piece of this feature is an idiomatic re-application of patterns already vendored/used in this repo: ent ORM (with `sql/upsert`), ConnectRPC/protobuf, the in-process `pkg/events.EventBus`, `config.FeatureFlags`, and React + `@connectrpc/connect`/`@connectrpc/connect-web` on the frontend (confirmed in `web-app/src/app/config/ConfigPageContent.tsx:6-7`). No new Go module or npm package is required.

## 1. ent schema — `GuidanceRequest`

Model on `session/ent/schema/backlog_stuck_state.go` (92 lines) for the "resolve-in-place, unique-key, atomic upsert" precedent named directly in the requirements (ADR-001), and on `session/ent/schema/item_session.go` for a plain child-entity edge shape (`edge.From(...).Ref(...).Field("item_id").Unique().Required()` + a loose `session_uuid` string FK, not an ent edge, for cross-package references that would create an import cycle).

Key decisions this feature must copy, not reinvent:

- **Status as validated string, not a native enum.** `BacklogStuckState.reason` and the wider house style (`BacklogStatus`, `ReviewOutcome`) store status/type/scope as plain `field.String(...)` validated in Go by an `IsValid()` method, per the comment at `session/ent/schema/backlog_stuck_state.go:37`. `GuidanceRequest.status` (`pending`/`answered`), `.request_type` (`yes-no`/`multiple-choice`/`short-answer`), and `.scope` (`backlog-item`/`session`/`standalone`) should all follow this, each with a `domain`-style `IsValid()` guard (mirroring `domain.StuckReason.IsValid()`).
- **`notified_at` / `answered_at` as `Optional().Nillable()` timestamps**, NULL = not yet happened — exactly `BacklogStuckState.notified_at`/`resolved_at`'s shape (`session/ent/schema/backlog_stuck_state.go:44-51`). This is what the atomic upsert's `ON CONFLICT` clause needs to update idempotently.
- **Options field**: store as `field.String("options").Optional()` holding a JSON array, matching `ItemSession.ac_snapshot`'s "JSON-in-a-string-field" convention (`session/ent/schema/item_session.go:41-43`) rather than adding a new list/JSON ent field type — keeps this schema consistent with the rest of the file set (grep across `session/ent/schema/*.go` shows no use of ent's native `field.JSON`/`field.Strings` for anything comparable).
- **Scope polymorphism** (`backlog-item` | `session` | `standalone`): since only one of `item_id`/`session_uuid` is populated depending on `scope`, follow `ItemSession.session_uuid`'s loose-FK pattern for the session reference (plain `field.String("session_uuid").Optional()`, not an ent edge — sessions aren't in the same ent domain) and a real ent edge (`edge.From("item", BacklogItem.Type).Ref(...).Field("item_id")`) only for the `backlog-item` scope, with the edge itself optional/non-required (unlike `ItemSession`'s `Required()` edge, since `GuidanceRequest` can exist with no backlog item at all).
- **Unique index for duplicate-creation/double-answer guard**: requirements call for "unique-index + status-guard duplicate handling." Follow `session/ent/schema/backlog_stuck_state.go:85-92`'s plain 2-column `index.Fields(...).Unique()` — pick the natural dedup key (e.g. `(scope_key, question_hash)` or, more simply, let creation-time application logic own dedup and use the unique index purely to make concurrent double-creation race-safe, same division of labor `MarkStuck` uses: Go-level validation + a DB-level uniqueness backstop).

## 2. ent generate — the `sql/upsert` feature flag

**Confirmed**: `Makefile:552-556`:
```make
ent-gen: ensure-tools
	go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```
`make build`, `make test`, and `make lint` all depend on `ent-gen` (regenerated from a stamp file, `.ent-gen.stamp`), so nothing extra needs wiring — adding the new schema file and running `make build` (or `make ent-gen` directly) regenerates the upsert-capable client. Do **not** run the bare `entgo.io/ent/cmd/ent generate` form — CLAUDE.md and the Makefile comment both flag it as breaking `UpsertRule`/`UpsertOne` methods project-wide. Per repo convention, do not commit `session/ent/*.go`/`session/ent/*/`-generated output — only `session/ent/schema/guidance_request.go` and any hand-written repository code are tracked.

For the actual `INSERT ... ON CONFLICT` call site, grep hits for `OnConflict` in non-test Go files show the existing call sites to mirror: `session/ent_repository_backlog.go`, `session/ent_repository.go`, `session/handoff_summary_service.go`, `session/session_summary_service.go`, `session/storage.go`. The `MarkStuck`-equivalent method (e.g. a `session.CreateOrGetGuidanceRequest` / an `AnswerGuidanceRequest` upsert-on-answer) belongs alongside these, using ent's `.OnConflictColumns(...)` / `.UpdateNewValues()` (or explicit `.Update(func(...))`) chain the same way.

## 3. ConnectRPC / protobuf — `proto/session/v1/backlog.proto` conventions

`backlog.proto` (survey via `grep -n "^message\|^service\|^  rpc "`) has ~90 messages following a strict `<Verb><Noun>Request`/`<Verb><Noun>Response` pairing (e.g. `CreateBacklogItemRequest`/`Response`, `TransitionBacklogItemStatusRequest`/`Response`). New RPCs for this feature (`CreateGuidanceRequest`, `AnswerGuidanceRequest`, `GetGuidanceRequest`, `ListGuidanceRequests` or similar) should be added to this same file (or a new sibling `proto/session/v1/guidance_request.proto` if the team prefers a dedicated file — either is consistent with the existing multi-file `proto/session/v1/*.proto` split by feature area, e.g. `handoff_summary.proto`, `session_summary.proto`).

For the **feature-flag/rollout RPC shape** specifically (a hard requirement — "existing live-settable rollout pattern"), the precedent is `session.proto`'s `TymuxRolloutService`-style RPCs (`GetTymuxRolloutStatus`, `SetTymuxSessionOverride`, `SetTymuxGlobalOverride`, all returning the same `TymuxRolloutStatus` message) plus the generic `GetFeatureFlags`/`UpdateFeatureFlag` pair (`proto/session/v1/session.proto:455,458`, messages at lines 2811-2902). AC3's triage-halt flag most likely only needs the generic `config.FeatureFlags` global-default + per-scope-override path (see §4) rather than a whole new dedicated Rollout service — `TymuxRolloutService`/`StreamHubRolloutService` exist because those features needed rich status objects (rehearsal timestamps, per-session override lists); a boolean halt-gate probably doesn't.

After editing `.proto`, regenerate via `make proto-gen` (buf-based, per CLAUDE.md's CI gotchas note on `gen/` being gitignored).

## 4. Feature flag — global default + per-scope override

Confirmed pattern in `config/config.go`:
- `Config.FeatureFlags map[string]bool` (`config/config.go:326-329`, `json:"feature_flags,omitempty"`).
- `GetFeatureFlag(name string) bool`, `GetFeatureFlagWithDefault(name, default bool) bool`, `SetFeatureFlag(name, value bool) error`, `DeleteFeatureFlag`, `GetFeatureFlagOverride(name) (value bool, ok bool)` (`config/config.go:1661-1711`).
- Named-constant + `knownFeatureFlags` registration convention in `server/services/feature_flag_service.go` (413 lines) — e.g. `const workspacePeersNudgeFlagName = "session:workspace-peers-nudge"`, a comment explaining any cross-package duplication needed to avoid an import cycle, then registered so `GetFeatureFlags` RPC surfaces it to the settings panel.
- **Per-scope override precedent**: `TymuxRolloutService` (`server/services/tymux_rollout_service.go`, 117 lines) is the fullest worked example of "global feature-flag default + per-scope (per-session) override, both live-settable via RPC": `cfg.GetTymuxGlobalOverride()`/`SetTymuxGlobalOverride(forceTymux *bool)` for the global flag, `cfg.TymuxSessionOverrides map[string]bool` + `SetTymuxSessionOverride(name string, forceTymux *bool)` for per-scope, with tri-state semantics (proto3 `optional bool` — unset clears the override and falls back to global default). `StreamHubRolloutService` (145 lines) is the sibling example. AC3's "triage halt-and-wait" flag should reuse this exact shape if it needs a per-item/per-project override in addition to the global default; if only a global on/off is needed, the plain `GetFeatureFlag`/`SetFeatureFlag` pair plus `GetFeatureFlags`/`UpdateFeatureFlag` RPCs suffice without a bespoke Rollout service.

## 5. Durable, event-bus-based notification (no new poll loop)

The constraint "must not add a `ScheduleWakeup`-style poll loop — ride the existing `BacklogItemEventPublisher`/`EventBus` pattern" maps directly onto machinery already fully built for backlog items:

- `pkg/events.EventBus` (`pkg/events/bus.go`): `Subscribe(ctx) (<-chan *Event, string)`, `Publish(event *Event)`, `SubscriberCount()`.
- `pkg/events/types.go`: `EventType` string constants (`EventBacklogItemChanged`, `EventSessionCreated`, `EventNotification`, etc., lines 15-35) and a per-domain payload struct (`BacklogItemEventPayload`, line 69) attached to a generic `Event{Type EventType, ...}` (line 124).
- `server/services/backlog_item_event_publisher.go` (86 lines): the adapter that turns a repository-level domain change (`session.BacklogItemChange`) into an `events.NewBacklogItemChangedEvent(payload)` published on the bus — wired in via `Storage.SetItemChangePublisher` because `session` cannot import `pkg/events` directly (import-cycle avoidance, the same reason `ItemSession.session_uuid` is a loose string FK rather than an edge). The entire publish body is wrapped in its own `recover()` so a panic in the notification side-channel can never take down the actual state-mutating call — a "best-effort side channel" idiom repeated at `runStuckDetector` and PTY forwarding.
- **The actual "wake the originating session up" consumer already exists**: `server/mcp/tools_backlog.go`'s `wait_for_backlog_event` MCP tool (`server/mcp/tools_backlog.go:619` onward) is the working precedent for "durable notification, not a poll loop" — a session-side MCP call blocks on an `EventBus.Subscribe` channel filtered by `item_id` + `event_type`, with a "check current state first, only block if not already satisfied" precheck (`currentStateWaitResult`, line 716) so a session that resumes *after* the event already fired doesn't hang.

For `GuidanceRequest`, the natural fit is: (a) add a new `BacklogChangeKind`/`event_type` filter value (e.g. `guidance_answered`) if the request is backlog-item-scoped, reusing `BacklogItemEventPayload` and the existing `wait_for_backlog_event` tool outright; and (b) for `session`/`standalone`-scoped requests (no backlog item), either extend `wait_for_backlog_event`'s filter to also accept a bare `session_uuid` key, or add a small parallel `EventType`/payload (e.g. `EventGuidanceRequestAnswered` + `GuidanceRequestEventPayload`) plus a new MCP tool (e.g. `wait_for_guidance_answer`) built the same way. Given the requirements explicitly want one shared mechanism across all 3 scopes, prefer extending the existing `EventBacklogItemChanged`-adjacent machinery with a scope-agnostic guidance event type over forking a second bespoke bus payload per scope.

## 6. MCP tool layer — ownership check

`server/mcp/tools_backlog.go:585` — `resolveItemLink(ctx, callerUUID, itemID) (session.ItemSessionSummary, *mcpgo.CallToolResult)` is the existing ownership-check helper (returns a populated `CallToolResult` error directly when the caller session isn't linked to the item, or the item doesn't exist — see the `ITEM_NOT_FOUND` handling noted at lines 2424-2431). It's called at 7+ existing tool sites (`resolveItemLink` grep hits at lines 1016, 1259, 1444, 1614, 1881, 2424, 2703) as the first line of every item-scoped MCP handler body. The new `create_guidance_request` MCP tool (and any item-scoped read-back tool) should call this exact helper for `backlog-item`-scoped requests before creating/reading a `GuidanceRequest` row — per the requirements constraint verbatim. `session`/`standalone`-scoped requests have no item to check against, so they only need the caller's own session identity (already available as `callerUUID`), no analogous ownership lookup.

## 7. Frontend — shared React component

- Confirmed stack: `@connectrpc/connect` + `@connectrpc/connect-web` (`createClient`, `createConnectTransport`), used directly, e.g. `web-app/src/app/config/ConfigPageContent.tsx:6-7,75,86`. No new frontend RPC library needed — a generated TS client for the new backlog.proto/guidance_request.proto RPCs comes for free from the existing `buf`/`protoc-gen-connect-es` codegen already producing the `SessionService`/`BacklogService` clients web-app imports.
- Component-reuse precedent for "one shared component rendered from 3 places" (backlog item detail, triage panel, session view): `web-app/src/components/backlog/GateVerdictBox.tsx`/`PlanVerdictBox.tsx` (each paired with a `.css.ts` vanilla-extract stylesheet and a colocated `.test.tsx`) are the closest existing example of a small, self-contained "render one verdict-shaped record's pending/decided state" component reused across surfaces. Model `GuidanceRequestCard`/`GuidanceRequestBox` (name TBD in the plan phase) on this pair: same `.tsx` + `.css.ts` (vanilla-extract, per `docs/reference/css-architecture.md`) + colocated Jest test structure, taking the `GuidanceRequest` proto message (or its mapped TS type) as a prop and switching rendering on `status`/`request_type` the same way `GateVerdictBox`/`PlanVerdictBox` switch on verdict outcome.
- Per CLAUDE.md, all `web-app/` package management must use `pnpm`, never `npm`/`yarn` (`docs/how-to/use-pnpm-in-web-app.md`), and frontend tests run via `cd web-app && npx jest --no-coverage` (not part of `make ci`).

## Summary of files this feature will touch (by convention, not by plan)

| Layer | New file(s) | Pattern source |
|---|---|---|
| ent schema | `session/ent/schema/guidance_request.go` | `backlog_stuck_state.go`, `item_session.go` |
| ent generate | (regenerated, not committed) | `Makefile:552-556`, `--feature sql/upsert` |
| proto | new messages/RPCs in `backlog.proto` or new `guidance_request.proto` | `backlog.proto` message pairs; `session.proto` `TymuxRolloutService`/`GetFeatureFlags` RPCs |
| feature flag | new const + `knownFeatureFlags` entry | `server/services/feature_flag_service.go`, `tymux_rollout_service.go` |
| event bus | new `EventType`/payload or extended `BacklogItemEventPayload` | `pkg/events/types.go`, `bus.go` |
| event adapter | extend `BacklogItemEventPublisher` or a sibling adapter | `server/services/backlog_item_event_publisher.go` |
| service layer | `server/services/guidance_request_service.go` (RPC handlers) + repository method(s) | `backlog_service_*.go` split-by-concern convention |
| MCP tools | new tool(s) in `server/mcp/tools_backlog.go` or a new `tools_guidance.go` | `resolveItemLink`, `wait_for_backlog_event` |
| frontend | `web-app/src/components/.../GuidanceRequestCard.tsx` + `.css.ts` + `.test.tsx` | `GateVerdictBox.tsx`/`PlanVerdictBox.tsx` |
