# Implementation Plan: durable-guidance-requests

**Feature**: A durable "Question"/"Guidance Request" construct any subscribed
LLM/session can create (scoped to a backlog item, a session, or standalone),
rendered as a structured form (yes/no, single-select multiple choice, short
answer) across the backlog item detail, triage panel, and session views, with
durable cross-restart notification back to the asker.
**Date**: 2026-09-11
**Status**: Ready for implementation
**ADRs**:
- `decisions/ADR-001-pr-split-foundation-then-ui.md` — two-PR split (foundation, then UI)
- `decisions/ADR-002-dedicated-guidance-service.md` — new standalone `GuidanceService`, not an extension of `BacklogService`/`SessionService`
- `decisions/ADR-003-single-select-multiple-choice.md` — single-select, reuse `RadioGroup`
- `decisions/ADR-004-standalone-scope-no-dedicated-ui-this-iteration.md` — standalone scope: backend-complete, no bespoke UI view yet

## PR Split (see ADR-001 for full reasoning)

- **PR #1 — Foundation** = Phase 1 below. Ent schema, proto/RPC, storage,
  notification wiring, MCP tools, dedup/lifecycle reconciliation, triage
  integration, autonomous-driver integration, backend tests. Independently
  mergeable and testable via `make ci` + MCP tool calls + integration tests, with
  zero React changes.
- **PR #2 — UI Integration** = Phase 2 below. Frontend hook/Redux slice, shared
  form component, wiring into the three named views, `StuckReasonAwaitingGuidance`
  badge, e2e tests. Depends on PR #1 having merged (needs its generated proto
  types and RPC surface).

## Key Corrections to Prior Research (verified by reading code in this planning
pass — cited so implementers don't re-derive or re-trust the wrong claim)

- `research/architecture.md` §1 claims `NotificationService` is "already a
  separate, focused service" distinct from `BacklogService`/`SessionService`.
  **Verified false as an RPC-registration fact**: `NotificationService`'s RPCs
  live in `session.proto`'s `SessionService` block
  (`proto/session/v1/session.proto:150-152`) and `*NotificationService` is a
  plain struct embedded on `*SessionService`
  (`server/services/session_service.go:110,682`), with delegating methods (e.g.
  `server/services/session_service.go:5098`). See ADR-002 for how this changes
  (and doesn't change) the `GuidanceService` shape decision.
- `research/stack.md` §2 claims "grep found no `.OnConflict(` call sites
  anywhere in `session/`" and that `ApprovalRule`'s `UpsertRule` is the only
  upsert precedent, implying `GuidanceRequest` "would be the first real
  consumer of ent's `OnConflict` API." **Verified false**:
  `session/storage.go:1416-1439` (`Storage.SetSessionGoal`) already calls
  `client.SessionGoal.Create()....OnConflictColumns("session_uuid").Update(func(u
  *ent.SessionGoalUpsert) {...}).Exec(ctx)` — real, working, generated-by-
  `--feature sql/upsert` code. **Use `SetSessionGoal` as the upsert template**,
  not `ApprovalRule.UpsertRule`, for any "one open guidance request per
  natural key" idempotency need in Epic 1.5.

## Dependency Visualization

```
PHASE 1 (PR #1 — Foundation, backend only)
 Epic 1.1 (ent schema + storage)
   └─> Epic 1.2 (proto + GuidanceService RPCs)
         ├─> Epic 1.3 (notification wiring)
         ├─> Epic 1.4 (MCP tools)
         ├─> Epic 1.5 (dedup + StuckReason + lifecycle reconciler)
         ├─> Epic 1.6 (triage integration)
         └─> Epic 1.7 (autonomous-driver integration)
                                                  │
                                                  ▼
PHASE 2 (PR #2 — UI Integration, depends on PR #1 merged)
 Epic 2.1 (frontend data layer: hook + Redux slice)
   └─> Epic 2.2 (shared GuidanceRequestForm component)
         ├─> Epic 2.3 (BacklogItemDetail.tsx wiring)
         ├─> Epic 2.4 (TriageReviewPanel.tsx wiring)
         ├─> Epic 2.5 (SessionDetailView.tsx wiring, new GuidancePanel.tsx)
         └─> Epic 2.6 (StuckReasonAwaitingGuidance badge, feature registry)
               └─> Epic 2.7 (e2e tests)
```

---

# Phase 1: Foundation (PR #1)

## Epic 1.1: Ent Schema + Storage Layer
**Goal**: A durable, ACID-persisted `GuidanceRequest` entity survives process
restart with zero bespoke code, per `research/architecture.md` §5.

### Story 1.1.1: `GuidanceRequest` ent schema
**As a** backend developer, **I want** a `GuidanceRequest` ent schema modeled on
`SessionGoal`/`BacklogActivityNote`, **so that** requests persist durably and
support optional item/session scoping.
**Acceptance Criteria**:
- Schema compiles and `go run -mod=mod entgo.io/ent/cmd/ent generate --feature
  sql/upsert ./session/ent/schema` regenerates cleanly.
- `item_id` and `session_uuid` are both `Optional()` (not required edges) —
  standalone requests must not be forced through a required FK (per
  `research/features.md` §2 "Scope object deleted/archived" and ADR-004).
**Files**: `session/ent/schema/guidance_request.go` (new),
`session/ent/schema/backlog_item.go` (add optional edge).

##### Task 1.1.1a: Create the `GuidanceRequest` ent schema file (~5 min)
- New file `session/ent/schema/guidance_request.go`. Fields (mirroring
  `session/ent/schema/session_goal.go`'s shape): `id` (UUID, `Default(uuid.New)`),
  `item_id` (`field.UUID("item_id", uuid.UUID{}).Optional().Nillable()`),
  `session_uuid` (`field.String("session_uuid").Optional()`), `question_type`
  (`field.String("question_type").NotEmpty()` — one of `"yes_no"`,
  `"multiple_choice"`, `"short_answer"`, validated at the RPC layer, not via
  ent enum, matching `SessionGoal.status`'s plain-string convention),
  `question` (`field.String("question").MaxLen(2000).NotEmpty()`), `choices`
  (`field.String("choices").Optional().Comment("JSON []string, only for
  multiple_choice")`), `status` (`field.String("status").Default("pending")` —
  `"pending"`/`"answered"`/`"expired"`), `answer`
  (`field.String("answer").Optional()`), `answered_by`
  (`field.String("answered_by").Optional().Comment("best-effort identity: a
  UI session marker, or empty for direct RPC/MCP callers")`),
  `origin_session_uuid` (`field.String("origin_session_uuid").Optional().Comment("the
  asking session, if any — distinct from session_uuid which is the SCOPE, not
  necessarily the asker")`), `automated_origin`
  (`field.Bool("automated_origin").Default(false).Comment("true for
  triage-created requests — used by the dedup/cap guard in Epic 1.5")`),
  `created_at` (`field.Time("created_at").Default(time.Now).Immutable()`),
  `answered_at` (`field.Time("answered_at").Optional().Nillable()`).
- Add `Edges() []ent.Edge { return nil }` — deliberately no required ent edge to
  `BacklogItem` (unlike `BacklogActivityNote`), per ADR-004/pitfalls.md's
  "must survive scope object disappearing" finding. `item_id`/`session_uuid`
  are plain fields, queried directly (same convention `BacklogActivityNote.item_id`
  already uses for its FK-by-convention field, just without `.Required()`).
- `Indexes()`: `index.Fields("item_id", "status")`,
  `index.Fields("session_uuid", "status")`, `index.Fields("status",
  "created_at")` (for the lifecycle reconciler's age-based sweep in Epic 1.5).

##### Task 1.1.1b: Add optional `guidance_requests` edge on `BacklogItem` (~3 min)
- In `session/ent/schema/backlog_item.go`'s `Edges()`, add
  `edge.To("guidance_requests", GuidanceRequest.Type)` — no
  `entsql.OnDelete(entsql.Cascade)` annotation (deliberately: deleting an item
  must not delete its guidance requests, only orphan them for the reconciler
  in Epic 1.5 to handle explicitly, per pitfalls.md §3).

##### Task 1.1.1c: Regenerate ent code and confirm build (~3 min)
- Run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert
  ./session/ent/schema` (the exact invocation from `session/ent/generate.go`
  — NOT the plain `entgo.io/ent/cmd/ent generate` form, which silently breaks
  upsert methods per CLAUDE.md and `research/pitfalls.md` §6).
- Run `go build ./...` to confirm it compiles. Do NOT `git add` anything under
  `session/ent/` except `session/ent/schema/guidance_request.go` and the
  modified `session/ent/schema/backlog_item.go` — everything else generated is
  gitignored per root `CLAUDE.md`.

### Story 1.1.2: `GuidanceRequestData` DTO + repository methods
**As a** backend developer, **I want** a `*Storage` facade for guidance
requests mirroring `SessionGoalData`/`ApprovalRuleData`, **so that** callers
(RPC handlers, MCP tools, triage, autonomous driver) never touch the ent client
directly.
**Files**: `session/guidance_types.go` (new), `session/ent_repository_guidance.go` (new),
`session/storage.go` (append passthrough methods).

##### Task 1.1.2a: Define `GuidanceRequestData` struct and status/type consts (~4 min)
- New file `session/guidance_types.go`: `type GuidanceQuestionType string` with
  consts `GuidanceQuestionYesNo = "yes_no"`, `GuidanceQuestionMultipleChoice =
  "multiple_choice"`, `GuidanceQuestionShortAnswer = "short_answer"`; `type
  GuidanceRequestStatus string` with consts `GuidanceRequestPending = "pending"`,
  `GuidanceRequestAnswered = "answered"`, `GuidanceRequestExpired = "expired"`;
  `type GuidanceRequestData struct` with fields matching Task 1.1.1a's schema
  1:1 (Go-idiomatic names: `ID`, `ItemID *string`, `SessionUUID string`,
  `QuestionType GuidanceQuestionType`, `Question string`, `Choices []string`,
  `Status GuidanceRequestStatus`, `Answer string`, `AnsweredBy string`,
  `OriginSessionUUID string`, `AutomatedOrigin bool`, `CreatedAt time.Time`,
  `AnsweredAt *time.Time`) — mirrors `SessionGoalData`'s shape convention in
  `session/storage.go`.

##### Task 1.1.2b: `EntRepository` create/get/list methods (~5 min)
- New file `session/ent_repository_guidance.go`. `func (r *EntRepository)
  CreateGuidanceRequest(ctx context.Context, data GuidanceRequestData)
  (*GuidanceRequestData, error)` (plain `.Create()`, no upsert — each request
  is its own row, unlike `SessionGoal`'s 1:1-per-session upsert shape).
  `func (r *EntRepository) GetGuidanceRequest(ctx context.Context, id string)
  (*GuidanceRequestData, error)` (returns `session.ErrNotFound` on miss,
  matching `GetSessionGoal`'s convention). `func (r *EntRepository)
  ListGuidanceRequests(ctx context.Context, filter GuidanceRequestFilter)
  ([]GuidanceRequestData, error)` where `GuidanceRequestFilter` has optional
  `ItemID`, `SessionUUID`, `Status *GuidanceRequestStatus` fields.

##### Task 1.1.2c: `EntRepository` answer + expire methods (~5 min)
- Same file as 1.1.2b. `func (r *EntRepository) AnswerGuidanceRequest(ctx
  context.Context, id, answer, answeredBy string) (*GuidanceRequestData,
  error)` — a `Where(guidancerequest.ID(id),
  guidancerequest.StatusEQ("pending"))` compare-and-swap update (mirrors
  `BacklogItemPrecondition`-gated transitions per `research/features.md` §1d
  — prevents a double-answer race), returning a distinguishable
  "already answered/not found" error the RPC layer maps to
  `connect.CodeFailedPrecondition`. `func (r *EntRepository)
  ExpireGuidanceRequest(ctx context.Context, id string) error` for the Epic
  1.5 reconciler.

##### Task 1.1.2d: `*Storage` passthrough methods (~3 min)
- Append to `session/storage.go` (near `SetSessionGoal`/`GetSessionGoal` for
  locality): `CreateGuidanceRequest`, `GetGuidanceRequest`,
  `ListGuidanceRequests`, `AnswerGuidanceRequest`, `ExpireGuidanceRequest` —
  each a one-line delegation to `s.entRepo.XGuidanceRequest(...)` guarded by
  the same `client == nil` nil-check convention every other `*Storage` method
  in that file uses.

### Story 1.1.3: Storage-layer tests
**Files**: `session/guidance_request_test.go` (new).

##### Task 1.1.3a: Create/get/list round-trip test (~4 min)
- New file `session/guidance_request_test.go`, using
  `session.NewTestEntRepository(t)` (per `research/pitfalls.md` §4 — in-memory
  ent repo, NOT `t.TempDir()`). Table-driven: create a yes_no, a
  multiple_choice (with `Choices`), and a short_answer request; assert
  `GetGuidanceRequest` round-trips every field; assert `ListGuidanceRequests`
  filters correctly by `ItemID`/`SessionUUID`/`Status`.

##### Task 1.1.3b: Answer compare-and-swap + double-answer rejection test (~4 min)
- Same file. Answer a pending request, assert `Status` becomes `"answered"`
  and `AnsweredAt`/`Answer`/`AnsweredBy` are set; answer it again and assert
  the second call fails (not silently overwriting) — this is the race the
  compare-and-swap in Task 1.1.2c exists to prevent.

##### Task 1.1.3c: Optional-scope (standalone) persistence test (~3 min)
- Same file. Create a request with `ItemID == nil` and `SessionUUID == ""`;
  assert it persists and round-trips — proves ADR-004's "backend is fully
  general regardless of scope" claim, not just an assertion.

---

## Epic 1.2: Proto + `GuidanceService` RPCs
**Goal**: A new, independently-registered ConnectRPC service per ADR-002.

### Story 1.2.1: `guidance.proto` message + service definitions
**Files**: `proto/session/v1/guidance.proto` (new).

##### Task 1.2.1a: Enums and core `GuidanceRequestProto` message (~5 min)
- New file `proto/session/v1/guidance.proto`, `syntax = "proto3"; package
  session.v1;` (mirrors `proto/session/v1/backlog.proto`'s header exactly),
  `import "google/protobuf/timestamp.proto";`.
- `enum GuidanceQuestionType { GUIDANCE_QUESTION_TYPE_UNSPECIFIED = 0;
  GUIDANCE_QUESTION_TYPE_YES_NO = 1; GUIDANCE_QUESTION_TYPE_MULTIPLE_CHOICE =
  2; GUIDANCE_QUESTION_TYPE_SHORT_ANSWER = 3; }` (naming convention matches
  `NotificationType`'s `NOTIFICATION_TYPE_*` shape,
  `proto/session/v1/types.proto:940-945`).
- `enum GuidanceRequestStatus { GUIDANCE_REQUEST_STATUS_UNSPECIFIED = 0;
  GUIDANCE_REQUEST_STATUS_PENDING = 1; GUIDANCE_REQUEST_STATUS_ANSWERED = 2;
  GUIDANCE_REQUEST_STATUS_EXPIRED = 3; }`.
- `message GuidanceRequestProto { string id = 1; optional string item_id = 2;
  string session_uuid = 3; GuidanceQuestionType question_type = 4; string
  question = 5; repeated string choices = 6; GuidanceRequestStatus status = 7;
  string answer = 8; string answered_by = 9; string origin_session_uuid = 10;
  bool automated_origin = 11; google.protobuf.Timestamp created_at = 12;
  google.protobuf.Timestamp answered_at = 13; }`.

##### Task 1.2.1b: RPC request/response messages (~5 min)
- Same file. `CreateGuidanceRequestRequest` (`optional string item_id`,
  `string session_uuid`, `GuidanceQuestionType question_type`, `string
  question`, `repeated string choices`, `string origin_session_uuid`, `bool
  automated_origin`) / `CreateGuidanceRequestResponse { GuidanceRequestProto
  request = 1; }`. `GetGuidanceRequestRequest { string id = 1; }` /
  `GetGuidanceRequestResponse { GuidanceRequestProto request = 1; }`.
  `ListGuidanceRequestsRequest { optional string item_id = 1; string
  session_uuid = 2; optional GuidanceRequestStatus status_filter = 3; }` /
  `ListGuidanceRequestsResponse { repeated GuidanceRequestProto requests = 1;
  }`. `AnswerGuidanceRequestRequest { string id = 1; string answer = 2; string
  answered_by = 3; }` / `AnswerGuidanceRequestResponse { GuidanceRequestProto
  request = 1; }`.

##### Task 1.2.1c: `WatchGuidanceRequests` event message + streaming RPC + service block (~5 min)
- Same file. `message GuidanceRequestEvent { google.protobuf.Timestamp
  timestamp = 1; oneof event { GuidanceRequestCreatedEvent created = 2;
  GuidanceRequestAnsweredEvent answered = 3; GuidanceSnapshotCompleteEvent
  snapshot_complete = 4; } uint64 seq = 5; }` with `created`/`answered` each
  carrying `GuidanceRequestProto request = 1; bool is_snapshot = 2;` (mirrors
  `BacklogItemStatusChangedEvent`'s `is_snapshot` field,
  `proto/session/v1/backlog.proto:1174`) — see `backlog.proto:1138-1148`'s
  comment for exactly why `GuidanceSnapshotCompleteEvent` (an empty message)
  must exist and be sent when a filtered watch matches zero requests.
  `message WatchGuidanceRequestsRequest { optional string item_id = 1; string
  session_uuid = 2; uint64 after_seq = 3; }`.
- `service GuidanceService { rpc CreateGuidanceRequest(...) returns (...) {}
  rpc GetGuidanceRequest(...) returns (...) {} rpc ListGuidanceRequests(...)
  returns (...) {} rpc AnswerGuidanceRequest(...) returns (...) {} rpc
  WatchGuidanceRequests(WatchGuidanceRequestsRequest) returns (stream
  GuidanceRequestEvent) {} }`.

##### Task 1.2.1d: Regenerate proto and confirm build (~3 min)
- Run `make proto-gen`. Confirm `gen/proto/go/session/v1/...` and
  `web-app/src/gen/...` now include the new types (do not commit either — both
  gitignored per root `CLAUDE.md`). `go build ./...`.

### Story 1.2.2: `GuidanceService` Go implementation
**Files**: `server/services/guidance_service.go` (new).

##### Task 1.2.2a: `GuidanceService` struct + constructor + Create/Get RPCs (~5 min)
- New file. `type GuidanceService struct { storage *session.Storage;
  eventBus *events.EventBus; notifier session.Notifier }` — the concrete
  `*session.Storage`, matching `BacklogService`'s field
  (`server/services/backlog_service.go:123`), the exact precedent ADR-002
  says to model `GuidanceService` on. Do NOT use `session.InstanceStore` here
  — that's a narrow interface for instance load/save only
  (`session/storage.go:243-254`: `LoadInstances`, `ListInstanceData`,
  `SaveInstances`, `AddInstance`, `DeleteInstance`,
  `UpdateInstanceLastUserResponse`, `UpdateInstanceMetadata`) with none of the
  `CreateGuidanceRequest`/`GetGuidanceRequest`/`ListGuidanceRequests`/
  `AnswerGuidanceRequest` methods this service needs. (The separate MCP
  `guidanceHandlers` struct in Epic 1.4 correctly carries both `storage
  *session.Storage` and `store session.InstanceStore` as two distinct fields,
  mirroring `tools_goal.go:19-20` — that two-field convention is for the MCP
  handlers, not this RPC service struct.) `func NewGuidanceService(storage
  *session.Storage, eventBus *events.EventBus, notifier
  session.Notifier) *GuidanceService`. Implement `CreateGuidanceRequest` (proto
  → `session.GuidanceRequestData` mapping, validation: `question_type ==
  MULTIPLE_CHOICE` requires non-empty `choices`; call
  `s.storage.CreateGuidanceRequest`; publish via `s.eventBus` — see Epic 1.3)
  and `GetGuidanceRequest` (`connect.CodeNotFound` on miss, mirroring
  `BacklogService.GetBacklogItem`'s error-mapping convention at
  `server/services/backlog_service_triage.go:2679-2683`).

##### Task 1.2.2b: List/Answer RPCs (~5 min)
- Same file. `ListGuidanceRequests` (delegates to
  `storage.ListGuidanceRequests`). `AnswerGuidanceRequest`:
  - **Validate the answer shape before writing** (per ADR-003's Consequences
    section, which names this requirement explicitly): `answer` must be one
    of the request's `choices` for `multiple_choice`, exactly `"yes"`/`"no"`
    for `yes_no`, and non-empty for `short_answer`; reject with
    `connect.CodeInvalidArgument` otherwise. This is a required acceptance
    criterion for this task, not optional — Task 1.2.2a's create-time
    validation only checks `choices` is non-empty for multiple_choice
    questions, it does not constrain what a later `AnswerGuidanceRequest`
    call accepts. Without it, a direct RPC/MCP caller (the "any subscribed
    LLM/session" case, bypassing the UI's `RadioGroup`-constrained form in
    Epic 2.2 entirely) could answer a yes/no or multiple-choice question with
    an arbitrary string.
  - Delegates to `storage.AnswerGuidanceRequest`; on success, publish the
    answered event via `s.eventBus` AND call `s.notifier.Notify(...)` — this
    is the durable cross-restart notification path, see Epic 1.3; on the
    compare-and-swap failure from Task 1.1.2c, return
    `connect.CodeFailedPrecondition`.
  - **Notify-path failure must never surface as the RPC's own error.** By the
    time `s.notifier.Notify(...)` is called, the compare-and-swap DB write is
    already durably committed and is the source of truth — it cannot be
    safely retried (a retry would just hit the "already answered"
    `CodeFailedPrecondition` path). `EventBusNotifier.Notify`
    (`server/services/backlog_notifier.go:17`) has no return value at all, so
    there is nothing to check, but even if a future `Notifier` implementation
    could error, that error must be logged at most, never returned to the
    caller in place of the successful answer response — notification is
    supplementary, not authoritative, per `research/architecture.md` §5.

##### Task 1.2.2c: `WatchGuidanceRequests` streaming RPC (~5 min)
- Same file. Model directly on `server/services/backlog_service_events.go`'s
  `watchBacklogItems` (line 91): subscribe to `s.eventBus` before the initial
  snapshot query (avoids the race window, per that file's own comment),
  replay `EventsSince(after_seq)` on reconnect or send one synthetic
  "created" event per currently-pending matching request on fresh connect,
  force `is_snapshot: true` on replayed/synthetic events, send
  `GuidanceSnapshotCompleteEvent` if literally nothing was sent, then fan out
  live events filtered by `item_id`/`session_uuid` until ctx cancellation.
  Use the same narrow-sender-interface indirection
  (`backlogItemEventSender`-equivalent) `backlog_service_events.go:39`
  documents, for the same testability reason (a real
  `*connect.ServerStream[T]` isn't constructible outside its own package).

### Story 1.2.3: Wire `GuidanceService` into server startup
**Files**: `server/dependencies.go`, `server/server.go`.

##### Task 1.2.3a: Construct `GuidanceService` in `server/dependencies.go` (~4 min)
- Add a `GuidanceService *services.GuidanceService` field to the
  `RuntimeDeps`/`ServerDeps` structs (mirroring `BacklogService`'s field at
  `server/dependencies.go:92,479`). Construct it near
  `backlogSvc := services.NewBacklogService(...)`
  (`server/dependencies.go:1291`): `guidanceSvc :=
  services.NewGuidanceService(storage, eventBus, notifier)`. Assign into both
  deps structs at the same lines `BacklogService:` is assigned
  (`server/dependencies.go:179,1701`).

##### Task 1.2.3b: Register `GuidanceService` ConnectRPC handler + WS bridge (~5 min)
- In `server/server.go`, immediately after the `BacklogService` registration
  block (`server/server.go:585-606`), add an equivalent block: a
  `"guidance_requests"` feature-flag interceptor (same
  `interceptors.NewFeatureFlagInterceptor` shape as
  `server/server.go:588-591`), but reading
  `config.LoadConfig().GetFeatureFlagWithDefault("guidance_requests", true)`
  — **not** the plain `GetFeatureFlag`, which has no default-true path
  (`config/config.go:1661-1666`: an absent key returns `false`, confirmed by
  `config/config_test.go:1290`'s "backlog feature flag should be false by
  default"). Using `GetFeatureFlag` here would ship the entire feature OFF
  everywhere until a human manually flips the flag, silently neutering Epic
  1.6 (triage) and Epic 1.7 (autonomous driver) too. `GetFeatureFlagWithDefault`
  (`config/config.go:1668-1687`) is this codebase's one mechanism for a flag
  that defaults to on — same pattern `terminal:resync-exec-gate-fast-lane`
  uses to graduate a flag to on-by-default while still letting an explicit
  persisted `false` opt back out. Then `sessionv1connect.NewGuidanceServiceHandler(deps.GuidanceService, ...)`,
  register at `/api` + its path, and wrap `WatchGuidanceRequests`'s path in a
  `services.NewStreamingWSBridge(...)` exactly like
  `watchBacklogItemsPath` at `server/server.go:598-605`.

##### Task 1.2.3c: Document the flag's default-true intent at the call site (~2 min)
- No feature-flag default-registry exists in this codebase (verified: there
  is no "known flags" list in `config/config.go`/`config/types.go` — a flag's
  default lives entirely at its `GetFeatureFlag`/`GetFeatureFlagWithDefault`
  call site, per Task 1.2.3b's fix). This task is just a one-line comment
  next to the Task 1.2.3b interceptor closure explaining why `guidance_requests`
  uses `GetFeatureFlagWithDefault(..., true)` instead of the more common
  `GetFeatureFlag` — the feature ships on by default, and the persisted-off
  toggle exists only for the abuse/rollback scenario named in
  `research/pitfalls.md` §5, not because the feature ships disabled. No
  separate registration step is needed beyond that comment.

### Story 1.2.4: Feature registry entries
**Files**: `docs/registry/features/backend/*.json` (generated).

##### Task 1.2.4a: Run `make registry-generate` and commit backend entries (~3 min)
- Per root `CLAUDE.md`'s "Adding New Features" checklist — new proto RPCs
  require this. Mark each new RPC with `// +api: guidance:create`,
  `guidance:get`, `guidance:list`, `guidance:answer`, `guidance:watch` in
  `server/services/guidance_service.go`'s method doc comments (matching
  `backlog_service_triage.go:2668`'s `// +api: backlog:trigger-triage`
  convention) before running the generator.

---

## Epic 1.3: Notification Integration
**Goal**: Answering a request durably notifies the asker across a restart, per
`research/pitfalls.md` §2 — reuse existing mechanisms, do not invent a new
notify channel.

### Story 1.3.1: Live fan-out via EventBus (already covered structurally by Epic 1.2)
**Acceptance Criteria**: `CreateGuidanceRequest`/`AnswerGuidanceRequest` both
publish onto the shared `*events.EventBus` (Task 1.2.2a/1.2.2b already do
this) using a new `events.EventType` (e.g. `EventGuidanceRequestCreated`,
`EventGuidanceRequestAnswered`), NOT by reusing `BacklogItemEvent`'s oneof
(that's item-scoped only; see ADR-002).
**Files**: `pkg/events/types.go`.

##### Task 1.3.1a: Add `EventGuidanceRequestCreated`/`EventGuidanceRequestAnswered` event types (~3 min)
- In `pkg/events/types.go`, add the two new `EventType` consts next to
  `EventSessionCreated`/`EventUserInteraction` etc., plus a
  `GuidanceRequestEventPayload` struct (mirrors `BacklogItemEventPayload`'s
  shape) carrying the fields `GuidanceService`'s `WatchGuidanceRequests` (Task
  1.2.2c) needs to reconstruct a `GuidanceRequestEvent`.
- **Also add both new consts to `server/events/forward.go`'s const block**
  (alongside `EventSessionCreated`, `EventBacklogItemChanged`, etc. — that
  file is a hand-maintained alias file mirroring every `EventType` constant
  from `pkg/events`, verified: it currently mirrors all 9 existing
  `EventType` consts one-for-one). This is required, not optional:
  `server/services` package code — where `guidance_service.go`/
  `guidance_notifier.go` live per ADR-002 — imports `server/events`, not
  `pkg/events`, directly (confirmed: `server/services/backlog_notifier.go`
  and `server/dependencies.go` both import `server/events`; only
  `server/mcp/*` files import `pkg/events` directly). Skipping this step
  means `events.EventGuidanceRequestCreated`/`events.EventGuidanceRequestAnswered`
  won't compile anywhere in `server/services`.

### Story 1.3.2: Durable notify-on-answer via `NotificationHistoryStore`
**Files**: `server/services/guidance_notifier.go` (new, or extend `backlog_notifier.go`).

##### Task 1.3.2a: `GuidanceService.notifier.Notify(...)` call on answer (~4 min)
- In `AnswerGuidanceRequest` (Task 1.2.2b), after a successful answer, call
  `s.notifier.Notify(originSessionUUIDOrItemID, title, message,
  sessionv1.NotificationType_NOTIFICATION_TYPE_INPUT_REQUIRED,
  priority)` — reuse the existing `NOTIFICATION_TYPE_INPUT_REQUIRED` enum
  value (`proto/session/v1/types.proto:945`), already durable via
  `NotificationHistoryStore` and already exempt from
  `ClearNotificationHistory`'s unread-actionable-record protection
  (`research/features.md` §1f). Do NOT write directly into any live
  `*Instance` field to "notify" — that bypasses both durable mechanisms and
  silently fails once the asking session is paused/restarted (the exact
  failure shape `research/pitfalls.md` §2 names).
- `session.Notifier` is the existing interface (`EventBusNotifier` in
  `server/services/backlog_notifier.go:12-35` is its only current
  implementation) — reuse the same `Notifier` type for `GuidanceService`
  rather than defining a second one; pass the same `EventBusNotifier` instance
  constructed in `server/dependencies.go` into `NewGuidanceService` (Task
  1.2.3a).

##### Task 1.3.2b: Notification content + coalescing key (~3 min)
- Ensure the `Notify(...)` call passes a stable per-request coalescing
  identity so `subscriber.go`'s `coalesceKey(sessionID, notifType)`
  (`server/notifications/subscriber.go:130`) doesn't merge two different
  answered guidance requests for the same session into one collapsed
  notification — pass the guidance request's own ID as (or embedded in) the
  `sessionID` argument, mirroring `EventBusNotifier`'s own doc comment at
  `backlog_notifier.go:21-28` about exactly this collapsing bug.

### Story 1.3.3: Notification-path tests
**Files**: `server/services/guidance_service_test.go` (new).

##### Task 1.3.3a: Answer triggers exactly one durable notification (~4 min)
- New file. Build a `GuidanceService` with a fake `eventBus` and a fake
  `session.Notifier` (a test double recording calls, not a real
  `NotificationHistoryStore` — per `research/pitfalls.md` §4, no
  `time.Sleep`/`require.Eventually`; assert synchronously on the call
  recorded during the `AnswerGuidanceRequest` call itself). Assert the
  notification type is `NOTIFICATION_TYPE_INPUT_REQUIRED` and the message
  references the question.

##### Task 1.3.3b: Two different requests for the same session don't collapse into one notification identity (~3 min)
- Same file. Answer two different pending requests scoped to the same
  `session_uuid`; assert the notifier was called with two distinct
  coalescing identities (per Task 1.3.2b).

---

## Epic 1.4: MCP Tool Surface
**Goal**: Any Claude Code session — the actual "any subscribed LLM/session"
caller — can create/read/wait-on/answer a guidance request, per
`research/architecture.md` §4.
**Files**: `server/mcp/tools_guidance.go` (new).

### Story 1.4.1: `create_guidance_request` / `get_guidance_request` / `list_guidance_requests`
**Files**: `server/mcp/tools_guidance.go` (new).

##### Task 1.4.1a: `guidanceHandlers` struct + `create_guidance_request` (~5 min)
- New file, modeled on `server/mcp/tools_goal.go`'s `goalHandlers`
  (`tools_goal.go:18-23`) and `server/mcp/tools_backlog.go`'s
  `backlogHandlers` (`tools_backlog.go:322`). `type guidanceHandlers struct {
  storage *session.Storage; store InstanceStore; eventBus *events.EventBus;
  enabledCheck func() bool }`. `func (h *guidanceHandlers)
  createGuidanceRequest(ctx context.Context, req mcpgo.CallToolRequest)
  (*mcpgo.CallToolResult, error)` — params: `item_id` (optional), `session_id`
  (optional, defaults to the caller's `STAPLER_SESSION_UUID` like
  `set_session_goal` does at `tools_goal.go:31`), `question_type`,
  `question`, `choices` (optional), `automated_origin` (bool, defaults
  false — automated callers like triage set this explicitly to opt into the
  stricter dedup/cap in Epic 1.5).

##### Task 1.4.1b: `get_guidance_request` / `list_guidance_requests` (~4 min)
- Same file. Direct-read tools mirroring `get_backlog_item`'s pattern —
  `get_guidance_request` by ID, `list_guidance_requests` filtered by
  `item_id`/`session_id`/`status`.

### Story 1.4.2: `wait_for_guidance_answer` + `answer_guidance_request`
**Files**: `server/mcp/tools_guidance.go`.

##### Task 1.4.2a: `wait_for_guidance_answer` (~5 min)
- Same file. Model directly on `waitForBacklogEvent`
  (`server/mcp/tools_backlog.go:775`) and its `currentStateWaitResult`
  precheck (`tools_backlog.go:716`): first check persisted storage state for
  the request ID — if already `"answered"`, return immediately (no bus
  subscription needed, satisfying "an asker that missed the event because it
  wasn't running yet still gets a correct immediate answer," per
  `research/architecture.md` §4). Otherwise subscribe to `eventBus` filtered
  to this request's ID, bounded at ≤60s like the backlog equivalent.

##### Task 1.4.2b: `answer_guidance_request` (~4 min)
- Same file. A programmatic answer path — needed so PR #1 (no UI yet) is
  fully testable/usable end-to-end per ADR-001's reasoning, and so a
  non-web-UI human-equivalent caller (Slack, a script) can answer before or
  independent of PR #2's UI. Delegates to the same
  `GuidanceService.AnswerGuidanceRequest`/`Storage.AnswerGuidanceRequest`
  path Task 1.2.2b uses — do not duplicate the compare-and-swap logic.

### Story 1.4.3: Register tools + tests
**Files**: `server/mcp/server.go`, `server/mcp/tools_guidance_test.go` (new).

##### Task 1.4.3a: Register in `server/mcp/server.go` (~2 min)
- Add `registerGuidanceTools(s, &guidanceHandlers{storage: storage, store:
  store, eventBus: eventBus, enabledCheck: guidanceEnabled})` alongside the
  existing `registerBacklogTools`/`registerGoalTools` calls
  (`server/mcp/server.go:77-78`).

##### Task 1.4.3b: MCP tool tests (~5 min)
- New file `server/mcp/tools_guidance_test.go`. Cover: create → get round
  trip; create → answer_guidance_request → get shows answered; wait_for
  precheck returns immediately for an already-answered request (no blocking);
  wait_for blocks then returns once a fake bus event fires (use a synchronous
  test bus, not real time, per `research/pitfalls.md` §4).

---

## Epic 1.5: Abuse/Dedup Guard + StuckReason + Lifecycle Reconciler
**Goal**: Bound automated creation (per `research/pitfalls.md` §5), give
humans a visible "blocked on guidance" signal (per `research/features.md` §3),
and reap orphaned/stale requests (per `research/pitfalls.md` §3).

### Story 1.5.1: Per-item/session cap + automated-origin cooldown
**Files**: `server/services/guidance_service.go` (extend), `server/services/guidance_dedup.go` (new).

##### Task 1.5.1a: Outstanding-unanswered cap check in `CreateGuidanceRequest` (~4 min)
- Before creating, call `storage.ListGuidanceRequests` filtered to the same
  scope (`item_id` or `session_uuid`) and `status == "pending"`; if the count
  is at or above a constant `maxOutstandingGuidanceRequestsPerScope = 3`
  (mirroring `blockedCycleThreshold = 3` at
  `server/mcp/tools_backlog.go:1387`), reject with
  `connect.CodeResourceExhausted` for `automated_origin == true` callers
  (human-initiated requests are not capped — the requirement's "the human
  user answers" side should never be blocked by automation's own cap).

##### Task 1.5.1b: Cooldown dedup for automated-origin duplicate questions (~5 min)
- New file `server/services/guidance_dedup.go`. Mirror
  `session/nudge_dedup.go`'s exact-repeat suppression shape
  (`nudgeCooldown = 3 * time.Minute`, content-normalized comparison): a
  per-`(item_id, question)` cooldown map, consulted only for
  `automated_origin == true` creates, so a looping/malfunctioning triage run
  can't spam near-identical questions. Human-initiated creates bypass this
  entirely.

### Story 1.5.2: `StuckReasonAwaitingGuidance`
**Files**: `session/domain/backlog.go`.

##### Task 1.5.2a: Add the new `StuckReason` const (~3 min)
- In `session/domain/backlog.go`, alongside `StuckReasonPlanNotApproved`
  (line ~99) and its siblings, add: `// StuckReasonAwaitingGuidance: the item
  has a pending GuidanceRequest that no automated process can resolve on its
  own — surfaces until AnswerGuidanceRequest resolves it. StuckReasonAwaitingGuidance
  StuckReason = "awaiting_guidance"`. Note this is two distinct edits in the
  same file, not one: the const declaration goes here (line ~99), while the
  priority-ordering insertion below is a separate list elsewhere in the same
  file (line ~181) — don't conflate them into a single edit location. Wire it into whatever shared priority
  order PR #769 established (per project memory: "share one StuckReason
  priority order between item detail and board," `session/backlog_lifecycle*.go`)
  — find that priority list (likely in `session/backlog_lifecycle_stuck.go` or
  `server/services/backlog_service_stuck.go`) and insert
  `StuckReasonAwaitingGuidance` at an appropriate priority (recommend: high,
  just below `StuckReasonBlockedByDependency`, since both represent "nothing
  can proceed without external input").

##### Task 1.5.2b: Compute the stuck state from a pending item-scoped `GuidanceRequest` (~4 min)
- Wherever `StuckReason`s are computed for a `BacklogItemData` (the function
  PR #769 touched — locate via the priority-order list from 1.5.2a), add a
  check: if `storage.ListGuidanceRequests(itemID, status=pending)` is
  non-empty, surface `StuckReasonAwaitingGuidance`. Clear automatically once
  no pending requests remain (matching the existing `ResolveStuck`-on-recovery
  convention other `StuckReason`s use, `session/storage.go:1059-1062`).

### Story 1.5.3: Lifecycle reconciler
**Files**: `session/backlog_lifecycle_guidance.go` (new).

##### Task 1.5.3a: Age-based expiry sweep (~5 min)
- New file, modeled on `archiveStaleDoneItems`
  (`session/backlog_lifecycle_archive.go`): `func
  ExpireStaleGuidanceRequests(ctx context.Context, storage *Storage, maxAge
  time.Duration) error` — finds `status == "pending"` requests older than
  `maxAge` (recommend a generous default, e.g. 14 days, since the whole point
  is "survives being unanswered across restarts/pauses" — not a short TTL
  like `ApprovalStore`'s 4-hour orphan window) and transitions them to
  `"expired"` via `ExpireGuidanceRequest` (Task 1.1.2c). Idempotent by
  construction (query naturally excludes already-expired/answered rows), per
  `research/pitfalls.md` §3's precedent requirement.

##### Task 1.5.3b: Orphaned-scope handling (~4 min)
- Same file. `func ReconcileOrphanedGuidanceRequests(ctx context.Context,
  storage *Storage) error` — for item-scoped pending requests whose
  `item_id` no longer resolves to an existing `BacklogItem` (item was hard-
  deleted, if that's possible) or whose item reached a terminal status
  (`done`/`archived`), expire them; mirror `PruneOrphaned`'s explicit
  "nil map means not-ready-to-judge, not nothing-exists" guard
  (`server/notifications/store.go:423-459`, per `research/pitfalls.md` §3) —
  do not treat an empty/errored existing-items lookup as "everything is
  orphaned."
- **Add a symmetric check for session-scoped requests**: for pending requests
  with a non-empty `session_uuid` and no `item_id`, expire them if that
  `session_uuid` no longer resolves to a live/known instance (same
  "nil/errored lookup means not-ready-to-judge" guard applies here too — use
  whatever existing instance-lookup the reconciler already has access to,
  e.g. `storage.ListInstanceData`/`LoadInstances`). This closes the same gap
  item-scoped requests already close: without it, a session-scoped orphan
  (the asking session's UUID stops resolving — e.g. the session was deleted)
  would sit as "pending" for up to the full `maxAge` (14 days, Task 1.5.3a)
  instead of being reconciled immediately like an item-scoped orphan. This
  was flagged in adversarial review as an asymmetry the plan must explicitly
  resolve; resolving it via a symmetric check (rather than accepting the
  asymmetry) keeps both scope kinds consistent with the same
  "reconcile promptly, don't wait for the age sweep" behavior.

##### Task 1.5.3c: Wire the reconciler into the existing periodic sweep (~3 min)
- Find where `archiveStaleDoneItems`/`reconcileTerminalItemSessions` are
  invoked periodically (likely a ticker in `server/dependencies.go` or
  `session/backlog_lifecycle.go`'s own scheduling loop) and add calls to
  Tasks 1.5.3a/1.5.3b alongside them, same interval.

### Story 1.5.4: Tests
**Files**: `server/services/guidance_dedup_test.go` (new),
`session/backlog_lifecycle_guidance_test.go` (new).

##### Task 1.5.4a: Cap + cooldown tests (~4 min)
- Assert the 4th automated-origin create for the same scope is rejected while
  a human-initiated 4th create succeeds; assert a near-identical automated
  question within the cooldown window is suppressed.

##### Task 1.5.4b: Reconciler tests (~4 min)
- Assert a pending request older than `maxAge` transitions to expired;
  assert an already-answered request is untouched; assert an item-scoped
  request whose item is `done` is expired; assert re-running the reconciler
  twice is a no-op the second time (idempotency).

---

## Epic 1.6: Automated Triage Integration
**Goal**: Triage asks instead of guessing, per requirements' explicit "wire
automated triage to use this construct for at least one real clarification
scenario."

### Story 1.6.1: Triage prompt emits a distinguished blocking-question shape
**Files**: `session/backlog_triage.go`.

##### Task 1.6.1a: Add a `blocking_question` field to the triage result JSON schema (~4 min)
- In `session/backlog_triage.go`'s `BuildHeadlessTriagePrompt` (and
  `HeadlessTriageResult`'s struct definition), add an optional
  `BlockingQuestion *BlockingQuestion` field, where `BlockingQuestion` has
  `QuestionType`, `Question`, `Choices []string` — distinct from the existing
  free-text `TriageSuggestion{Rationale: "question"}` shape (which stays for
  non-blocking suggestions). Update the prompt template instructing the model:
  emit `blocking_question` only when triage genuinely cannot proceed without
  human input (ambiguous scope, conflicting acceptance criteria, etc.), at
  most one per triage run — mirroring `BuildHeadlessChatRetriagePrompt`'s
  existing "AT MOST ONE... never more than one question" instruction
  (`backlog_triage.go:176-184`).

##### Task 1.6.1b: `ParseHeadlessTriageResult` handles the new field (~3 min)
- In `session/backlog_triage.go`'s `ParseHeadlessTriageResult`
  (`backlog_triage.go:229`), ensure the new optional field round-trips
  through the existing JSON-extraction logic without breaking parsing of
  results that omit it (the common case).

### Story 1.6.2: `TriggerTriage` creates a `GuidanceRequest` and defers completion
**Files**: `server/services/backlog_service_triage.go`.

##### Task 1.6.2a: Branch on `result.BlockingQuestion` after parsing (~5 min)
- In the `TriggerTriage` goroutine, right after `result, parseErr :=
  session.ParseHeadlessTriageResult(raw)` (`backlog_service_triage.go:3019`)
  and before the call to `applyTriageResultToUpdate(&result, &update)`
  (`backlog_service_triage.go:3134`), add: if `result.BlockingQuestion !=
  nil`, call `s.storage.CreateGuidanceRequest(...)` with `ItemID: &itemID`,
  `AutomatedOrigin: true`, `OriginSessionUUID: triageSessionUUID` (captured
  earlier in the goroutine, per `research/architecture.md` §3's note that
  `itemID`/`itemRepoPath`/`isID`/`iteration` are already in scope at this
  point), then `return` early from the goroutine — skip
  `applyTriageResultToUpdate` and the subsequent status-advance entirely,
  leaving the item in its current status (idea) with a pending guidance
  request surfacing via `StuckReasonAwaitingGuidance` (Epic 1.5.2) rather than
  advancing triage on a guess.

##### Task 1.6.2b: Persist the triage-session/ItemSession end state correctly (~3 min)
- Same goroutine. Ensure the early-return path still calls
  `UpdateItemSessionEndedWithReason` (or the equivalent success-path call the
  non-blocking-question branch already makes) with a distinguishable reason
  (e.g. `"awaiting_guidance"`) so the orphan-detection/staleness sweeps
  (`reconcileOrphanedTriageItems`) don't misclassify this triage session as
  crashed or hung.

### Story 1.6.3: Tests
**Files**: `server/services/backlog_service_triage_guidance_test.go` (new).

##### Task 1.6.3a: Blocking question creates a `GuidanceRequest` and skips status advance (~5 min)
- Fake headless pool returning a `blocking_question`-bearing result; assert
  a `GuidanceRequest` was created scoped to the item with
  `AutomatedOrigin: true`; assert the item's status did NOT advance past
  `idea`; assert `StuckReasonAwaitingGuidance` is now present for the item.

##### Task 1.6.3b: Non-blocking triage result is unaffected (regression) (~3 min)
- Fake headless pool returning an ordinary (no `blocking_question`) result;
  assert triage completes exactly as before this feature — required by
  requirements.md's "no existing behavior regresses."

---

## Epic 1.7: Autonomous Driver Integration
**Goal**: A session's turn loop can durably pause for guidance and resume
even across a driver-goroutine restart, per `research/architecture.md` §3.

### Story 1.7.0: Thread guidance storage into `AutonomousDriver`
**Goal**: Give `(*AutonomousDriver).run()` a way to reach storage at all —
a prerequisite for Tasks 1.7.1b and 1.7.2a below, both of which call
`storage.CreateGuidanceRequest(...)`/`storage.ListGuidanceRequests(...)` from
inside `run()`. **Verified this is currently missing** (adversarial review
round 3's Blocker): `AutonomousDriver` (`session/autonomous_driver.go:71-110`)
has no `storage`/`*Storage` field, `NewAutonomousDriver`'s only `DriverOption`s
are `WithStartupTimeout`/`WithCostSink` (lines 49-58), and all three real
construction call sites omit any storage argument today.
**Files**: `session/autonomous_driver.go`,
`server/services/autonomous_orchestration_service.go`,
`server/services/session_creation_pipeline.go`.

##### Task 1.7.0a: Add a concrete `storage *Storage` field + `WithGuidanceStorage` option (~5 min)
- **Round-4 review reversed the narrow-interface approach from round 3 —
  read this before implementing.** The prior plan defined a same-package
  `GuidanceStorage` interface here (mirroring `HeadlessPoolClient`/
  `panePreviewer`, this file's two existing narrow-interface precedents) with
  just `CreateGuidanceRequest`/`ListGuidanceRequests`. That interface has now
  needed a new method in every round since it was introduced: round 4's
  review found it was missing `GetItemSessionBySessionUUID` (needed by Task
  1.7.1b), and Task 1.7.2b was already planning to add a third method,
  `MarkGuidanceRequestDelivered`, on top of that. Three methods discovered
  across two rounds, for a type whose only real backing implementation is
  `*Storage` in the same package, is exactly the "just add one more method"
  churn `GuidanceService` (Task 1.2.2a) explicitly chose the concrete
  `*session.Storage` type to avoid ("Do NOT use `session.InstanceStore`
  here... narrow interface... with none of the methods this service needs").
  **Decision: use the concrete `*Storage` type here too, not an interface.**
  This is a different call from `HeadlessPoolClient`/`panePreviewer`, and the
  difference is deliberate, not an inconsistency: those two interfaces are
  small, complete, and have been stable since they were introduced (one
  method each, never extended) — narrowing genuinely paid for itself there.
  `GuidanceStorage` never stabilized; every review round needed another
  method because the driver's guidance logic keeps needing more of what
  `*Storage` already offers. A concrete field trades a small amount of
  test-double flexibility (Story 1.7.3's tests now need a real
  `session.NewTestEntRepository(t)`-backed `*Storage`, not a hand-rolled
  fake — the same in-memory-ent pattern Task 1.1.3a already uses, so this is
  not new test machinery) for eliminating the whole class of "missing method
  X" compile-blockers this seam produced for two rounds running.
- In `session/autonomous_driver.go` (same package as `session/storage.go` —
  no import needed), add `storage *Storage` as a new field on the
  `AutonomousDriver` struct (`session/autonomous_driver.go:71-110`, alongside
  `costSink`).
- Add `func WithGuidanceStorage(storage *Storage) DriverOption { return
  func(a *AutonomousDriver) { a.storage = storage } }`, following the exact
  `WithCostSink`/`WithStartupTimeout` functor pattern (lines 49-58). No
  default needed in `NewAutonomousDriver`'s defaulting block (lines 139-157)
  — an unset `d.storage` stays nil, and Tasks 1.7.1b/1.7.2a's storage calls
  must nil-guard (skip the guidance-directive/per-turn-check logic entirely,
  log a warning) when `d.storage == nil`, mirroring `NoopDriverOption`'s
  existing "safe to omit" convention for `costSink`.
- `MarkGuidanceRequestDelivered` (Task 1.7.2b) and `GetItemSessionBySessionUUID`
  (Task 1.7.1b, already real today at `session/storage.go:1195-1198`) are
  both then automatically available on `d.storage` with zero further
  interface edits — the specific gap this change closes.

##### Task 1.7.0b: Wire `WithGuidanceStorage` at all three real construction call sites (~4 min)
- All three sites already have a concrete `*session.Storage` in scope today —
  verified by reading each; no additional plumbing (e.g. threading a new
  parameter through an enclosing function) is needed to obtain one:
  - `server/services/autonomous_orchestration_service.go:206`
    (`StartAutonomousDriverForInstance`): add a `withGuidanceStorage(inst)`
    helper mirroring the existing `withCostSink` (lines 184-197) — same
    `a.storageGetter == nil` / `concreteStorage == nil` guard, falling back
    to `session.NoopDriverOption` — and append its result to the
    `session.NewAutonomousDriver(inst, a.pool, inst.Prompt, 0, ...)` call's
    variadic opts.
  - `server/services/autonomous_orchestration_service.go:224`
    (`StartAutonomousDriverWithTimeout`): same `withGuidanceStorage(inst)`
    call appended to its `session.NewAutonomousDriver(...)` call.
  - `server/services/session_creation_pipeline.go:328`: `concreteStorage :=
    s.GetStorage()` is already computed two lines above (line 325, for
    `costOpt`) — reuse it: `guidanceOpt := session.NoopDriverOption; if
    concreteStorage != nil { guidanceOpt =
    session.WithGuidanceStorage(concreteStorage) }`, then append
    `guidanceOpt` to the `session.NewAutonomousDriver(p.instance,
    s.headlessPool, p.instance.Prompt, 0, costOpt, guidanceOpt)` call.
  - No adapter/satisfies-interface step is needed at any of these three
    sites: `WithGuidanceStorage` now takes the same `*session.Storage` type
    `a.storageGetter()`/`s.GetStorage()` already return.

Tasks 1.7.1b and 1.7.2a below depend on this story and may now assume
`d.storage` is a (possibly nil-checked) `*Storage` field inside `run()`,
rather than an undefined name.

### Story 1.7.1: New orchestration directive
**Files**: `session/autonomous_driver.go`.

##### Task 1.7.1a: Add `directiveAskQuestion` to the enum + response parsing (~4 min)
- In `session/autonomous_driver.go`, extend `orchestrationDirective`
  (line 676-681) with `directiveAskQuestion`.
- The actual thing that must change is
  `orchestrationDirectiveMarker = regexp.MustCompile(`(?i)(DONE|NEXT_MESSAGE|WAIT)\s*:`)`
  at line 668 — add a `QUESTION` alternative to that regex (e.g.
  `(DONE|NEXT_MESSAGE|WAIT|QUESTION)\s*:`), then extend the `switch keyword`
  in `parseOrchestrationResponse` (line 693, `case` block at lines 702-708)
  with a `case "QUESTION":` branch returning `directiveAskQuestion` and the
  payload (question text and type/choices, parsed from the body same as
  `NEXT_MESSAGE`'s body today).
- **This is a spot with real scar tissue** — the file's own comment at lines
  657-666 documents a past false-prefix-match bug (BUG-056: the old
  exact-prefix match rejected valid directives like
  "...guesswork.DONE: Reached the 20-turn limit...", burning 8 of 20 turns on
  one live item, because the model appends the directive onto the end of its
  last sentence with no separating newline). The fix that regex embodies —
  case-insensitive, anywhere-in-string matching via `FindAllStringSubmatchIndex`
  plus "last match wins" — must keep working for the new `QUESTION`
  alternative too. Add the new keyword to the existing regex and switch; do
  not write a second, separate parsing path for `QUESTION:` elsewhere in the
  file.

##### Task 1.7.1b: Handle `directiveAskQuestion` in the turn loop (~5 min)
- **Depends on Story 1.7.0**: `d.storage` (a `*Storage`) must exist on the
  struct before this task's `storage.CreateGuidanceRequest(...)` and
  `storage.GetItemSessionBySessionUUID(...)` calls compile; nil-guard per
  Task 1.7.0a if `d.storage` is unset.
- In `(*AutonomousDriver).run` (around lines 336-421), add a branch: on
  `directiveAskQuestion`, call `storage.CreateGuidanceRequest(...)` with
  `SessionUUID: sessionID`, `OriginSessionUUID: sessionID` — `sessionID` is
  the local variable `run()` already computes at
  `session/autonomous_driver.go:265` (`sessionID := d.inst.UUID`), in scope
  for the rest of the function; there is no `d.sessionUUID` field on
  `AutonomousDriver` and none needs to be added — `AutomatedOrigin: false`
  (this is a live session asking on its own behalf, not automated triage —
  the cap/cooldown in Epic 1.5.1 does not apply), then behave like
  `directiveWait` for this turn (skip injecting a nudge, still wait for
  idle) rather than ending the loop. Also set `ItemID` on the created
  request when the driver's session is backlog-linked (look it up the same
  way `onAutonomousDriverComplete` does, via
  `d.storage.GetItemSessionBySessionUUID(ctx, sessionID)`, real today at
  `session/storage.go:1195-1198`) — Task 1.5.2b's
  `StuckReasonAwaitingGuidance` computation queries pending requests by
  `ItemID`, not `SessionUUID`, so without this the item never shows as
  awaiting guidance for this path, undermining Task 1.7.1d's decision to
  rely on that badge as the sole visible signal.

##### Task 1.7.1c: Design decision — `maxTurns` budget while awaiting guidance (~4 min)
- **Explicit decision** (do not leave this as inherited default `WAIT`
  behavior with no comment — flagged in adversarial review): a
  `directiveAskQuestion` turn behaves like `directiveWait` per Task 1.7.1b,
  which per the existing comment at lines 356-361 means it still counts
  against `d.maxTurns` "same as a malformed response burning a turn." Given
  Epic 1.5.3a's 14-day default TTL for unanswered requests, a session asking
  a question can plausibly exhaust `maxTurns` (each wait-turn bounded by up
  to a 5-minute idle-wait, lines 374-376/416-418) long before a human
  answers.
- **Decision: option (b) — keep counting turns against the budget, but give
  the exhausted-while-awaiting-guidance exit a distinct, non-failure outcome
  classification.** Exempting guidance-wait turns entirely (option (a),
  a separate counter) was rejected: it would let a session that asked a
  question but never gets answered loop indefinitely consuming
  `headlessPool`/idle-wait resources, with no bound until the unrelated
  14-day reconciler sweep expires the request — worse than the existing
  `maxTurns` safety valve this codebase relies on elsewhere. Instead: when
  the loop exits via `turnCount >= d.maxTurns` (falls out of the `for` loop
  in `run`, around line 422-424), query storage for a pending session-scoped
  `GuidanceRequest` — the same check Task 1.7.2a already performs each turn
  — and, if one exists, set the new outcome fields described below instead of
  the plain "gave up" reason an ordinary `maxTurns` exhaustion produces, so
  callers/UI can distinguish "ran out of turns waiting on a human" from "ran
  out of turns and truly stuck." **This is a single storage query, not two
  checks**: tracking "the most recent directive seen was `directiveAskQuestion`"
  as separate state is unnecessary and must not be added — only that
  directive can have created a pending request in the first place, `run()`
  scopes `directive` per-iteration today with no cross-iteration "last
  directive" state, and adding one would be machinery this task doesn't need
  (adversarial review round 3 flagged this as unnecessary machinery to trim).
- **Outcome shape (resolves the re-review's Concern — this is now a decision,
  not a choice between two forms):** add a new `AwaitingGuidance bool` field
  to `AutonomousDriverOutcome` (`session/autonomous_driver.go:25-31`),
  alongside the existing `Done`, `Reason`, `PRUrl`, `Turns`, and `Stuck`
  fields — a boolean flag is more consistent with the struct's existing
  `Stuck bool // true if exited via maxTurns without DONE signal` convention
  than overloading `Reason`'s free-text value. On this exit path, set
  `AutonomousDriverOutcome{Done: false, Stuck: true, Turns: d.maxTurns,
  Reason: "awaiting_guidance", AwaitingGuidance: true}` — `AwaitingGuidance:
  true` is additive alongside `Stuck`/`Turns`/`Reason`, which the exhaustion
  path already sets and must keep setting unchanged; it does not replace or
  omit any of them. See Task 1.7.1d for how the one production consumer of
  this struct (`onAutonomousDriverComplete`) must branch on the new field —
  without that wiring, this classification is inert (the exact gap the
  re-review's Blocker identified).

##### Task 1.7.1d: Wire `AwaitingGuidance` into `onAutonomousDriverComplete` (~5 min)
**Files**: `server/services/autonomous_orchestration_service.go`.
- **Fix for the re-review's Blocker.** Without this task, Task 1.7.1c's new
  outcome classification is never read: `onAutonomousDriverComplete`
  (registered as the driver's `CompletionCallback`, called unconditionally on
  every exit including `maxTurns` exhaustion) branches purely on
  `outcome.Done`, so a `SessionRoleWork` session that exhausted its turn
  budget while awaiting an answered-later guidance request was still treated
  as an ordinary give-up: `MarkStuck(StuckReasonAutonomousStuck, ...)` fires,
  the `ItemSession` is ended via `UpdateItemSessionEnded`, and
  `AutoRespawnAutonomousWork` (`server/services/backlog_service_triage.go`
  ~line 1965) spawns a **brand-new session with a new session UUID** for the
  same item. Since the pending `GuidanceRequest` is scoped to the *original*
  session's UUID (Task 1.7.1b: `SessionUUID: sessionID`) and Task
  1.7.2a's per-turn check queries by the driver's *own* `sessionID`, the
  respawned driver can never find it — the request is orphaned until its
  14-day TTL expires.
- **Decision: keep the original `ItemSession`/session alive; do not end or
  respawn it.** This is the only option that lets the existing
  session-UUID-scoped `GuidanceRequest` lookup in Task 1.7.2a keep working
  unmodified — ending the session and writing the request's scope
  differently (the Blocker's other suggested option) would require
  additional plumbing not otherwise needed anywhere else in this plan, for
  no benefit. Concretely, in `onAutonomousDriverComplete`:
  - The pre-switch, role-generic `if !outcome.Done { ... MarkStuck(ctx,
    item.ID, domain.StuckReasonAutonomousStuck, ...) /
    MarkStuckNotified(...) ... }` block (currently lines ~316-324) must not
    fire for this outcome: change its guard to
    `if !outcome.Done && !outcome.AwaitingGuidance`. (In practice this only
    changes behavior for `SessionRoleWork`, since `directiveAskQuestion` is
    Epic 1.7's own turn-loop directive — Epic 1.6's triage path uses a
    separate mechanism per Task 1.6.2a and never produces
    `AwaitingGuidance: true`.)
  - **Also guard the session-level outcome badge, earlier in the same
    function (currently lines ~273-278), not just the pre-switch block
    above** (adversarial review round 3's Concern 1 — this is a distinct
    edit location from the pre-switch guard, before it in execution order):
    `if outcome.Done { inst.AutonomousOutcome = "done" } else {
    inst.AutonomousOutcome = "stuck" }` currently sets `"stuck"`
    unconditionally for any `!outcome.Done`, including `AwaitingGuidance:
    true`. Change the `else` to `else if !outcome.AwaitingGuidance`, so an
    `AwaitingGuidance: true` exit leaves `inst.AutonomousOutcome` at its
    prior value (ordinarily empty) instead of publishing "stuck" via the
    `SessionUpdatedEvent` on the next line. Without this, the fix's own
    claim that `StuckReasonAwaitingGuidance` becomes "the item's only visible
    'stuck' signal" holds at the backlog-item level but is false at the
    session level — the two signals would disagree for the same outcome.
  - **Do not use a blanket early `return` here — target only the two specific
    calls.** An earlier draft of this task wrapped
    `UpdateItemSessionEnded`/`AutoRespawnAutonomousWork` in `if
    outcome.AwaitingGuidance { log.Info(...); return }`, but `return` exits
    `onAutonomousDriverComplete` entirely, which also skips the generic,
    role-independent push-notification block later in the same function
    (currently lines ~573-607, `// Fire push notification via event bus.`,
    guarded only by `!inst.Hidden`) — that block is not specific to the
    stuck/respawn logic this task is fixing, and a `SessionRoleWork` outcome
    is the *only* branch that normally falls through uninterrupted to reach
    it (`SessionRoleTriage` and `SessionRoleReview` both already `return`
    early, and each says so explicitly in an adjacent comment — this task's
    `return` would put `SessionRoleWork` in that same category as an
    unexamined side effect, not a documented choice). The user should still
    be told their session paused (in addition to the separate
    guidance-request notification Epic 1.3 sends), so this notification must
    keep firing. Instead, inside the `case session.SessionRoleWork:` block's
    existing `if !outcome.Done { ... }` (currently lines ~349-426, which
    today unconditionally runs `UpdateItemSessionEnded` then gates
    `AutoRespawnAutonomousWork` on `RemediationDue`), wrap just those two
    calls: `if !outcome.AwaitingGuidance { <existing UpdateItemSessionEnded
    call> ... <existing autonomousStuckRespawner block> } else {
    log.Info("[AutonomousDriver] awaiting guidance, leaving item session open",
    "item", item.ID) }` — the existing "end the session so the respawn's
    liveness check doesn't self-block" logic (the BUG-048-derived comment at
    lines ~364-378) is specific to the genuinely-stuck give-up case and must
    not run here, but nothing else in the function is skipped.
  - **Also adjust the generic notification's text for this outcome** (same
    lines ~573-607) so it doesn't say something misleading like "stopped
    after N turns without completing... give the next instruction" for a
    session that is actually paused waiting on an answer: add an `else if
    outcome.AwaitingGuidance` branch alongside the existing `if outcome.Done
    { ... } else { ... }` (title `"Awaiting guidance"`, an
    `NOTIFICATION_TYPE_INFO`-shaped body naming the session, e.g. `"Session
    '%s' is paused, waiting for an answer to a question it asked."`) before
    the existing `else` (genuinely-stuck) branch — one additional `else if`,
    not a restructure of the block.
  - Net effect: for `AwaitingGuidance: true`, `onAutonomousDriverComplete`
    skips exactly three things — `MarkStuck(StuckReasonAutonomousStuck,
    ...)` (via the pre-switch guard above), `UpdateItemSessionEnded`, and
    `AutoRespawnAutonomousWork` (via the guard just above) — while every
    other statement in the function, including the generic push
    notification, still executes. The item's only visible "stuck" signal
    becomes `StuckReasonAwaitingGuidance`, computed independently by Task
    1.5.2b from the pending `GuidanceRequest`
    row itself (not from anything `onAutonomousDriverComplete` writes), and
    the `ItemSession`/session UUID stay open so a later driver run resumed
    on that same session (Task 1.7.2a's per-turn check is explicitly
    designed to work "even if the driver goroutine itself was killed by a
    restart") can still find and inject the eventual answer.
  - Note for Task 1.5.2b: for this path's `StuckReasonAwaitingGuidance` to
    actually surface, Task 1.7.1b's `CreateGuidanceRequest` call must also
    set `ItemID` (looked up the same way `onAutonomousDriverComplete` itself
    does, via `GetItemSessionBySessionUUID` → the linked backlog item) when
    the asking session is backlog-linked, since Task 1.5.2b's query is
    `ListGuidanceRequests(itemID, status=pending)`, not session-scoped.
    Amend Task 1.7.1b's `CreateGuidanceRequest(...)` call accordingly.

##### Known Limitation: no automated resume after `maxTurns` exhaustion while awaiting guidance
**Explicit decision (adversarial review round 3's Concern 2), not left open.**
Once `maxTurns` is exhausted while awaiting guidance (Task 1.7.1c/1.7.1d's
path), the `ItemSession` stays open per this task's design, but the driver
goroutine has fully exited — and nothing in this PR reconstructs a new
`AutonomousDriver` for that session UUID automatically once the guidance
request is answered. The item sits at `StuckReasonAwaitingGuidance`
indefinitely until either a human manually re-triggers autonomous mode (the
existing `UpdateSession` RPC toggle, `server/services/session_service.go:
3157-3166` — a real, already-shipped manual path, not hypothetical) or Task
1.5.3a's 14-day TTL expires the request.

**Decision: accept this gap for this PR; do not build event-driven
reconciliation to auto-restart a driver on guidance-answered.** Reasoning:
(a) the manual re-trigger path already exists today and becomes discoverable
once Epic 2's `GuidancePanel`/`StuckReasonAwaitingGuidance` badge ship, giving
a human a visible "answer, then flip autonomous mode back on" path; (b) the
14-day TTL (Task 1.5.3a) is a real backstop, not silence — the item cannot
sit awaiting a request forever undetected; (c) a proper fix — a listener that
reconstructs an `AutonomousDriver` for a session once its `GuidanceRequest` is
answered — is legitimate, independently-scoped follow-on work per
`requirements.md`'s explicit allowance for deferring work when full scope is
too large for one PR (the same reasoning ADR-001 already applies at the
coarser PR-split grain). This is a materially larger addition than anything
else in Epic 1.7 — it needs its own reconciliation loop, not a few-line
branch — so it is tracked in "Follow-On Work Explicitly Deferred" below, not
folded into Task 1.7.1c/1.7.1d.

### Story 1.7.2: Per-turn check for an answered request
**Files**: `session/autonomous_driver.go`.

##### Task 1.7.2a: Query storage for an answered-but-undelivered request at turn start (~5 min)
- **Depends on Story 1.7.0**: `d.storage` (a `*Storage`) must exist on the
  struct before this task's `storage.ListGuidanceRequests(...)` call
  compiles; nil-guard per Task 1.7.0a if `d.storage` is unset (skip the
  per-turn check entirely).
- Near the top of each turn (before `buildOrchestrationPrompt`, around line
  312-322), add: query `storage.ListGuidanceRequests(session_uuid=sessionID,
  status=answered)` — `sessionID` is the same `run()`-local variable (`:=
  d.inst.UUID` at line 265) Task 1.7.1b uses; there is no `d.sessionUUID`
  field — for the most recent answered request not yet injected
  (track "delivered" via a field on the request, or a driver-local set of
  already-injected IDs reset on driver construction — since a fresh
  `AutonomousDriver` on session resume re-queries persisted state directly,
  satisfying "resumes correctly even if the driver goroutine itself was
  killed by a restart" per `research/architecture.md` §3). If found,
  short-circuit the LLM orchestration call for this turn and inject the
  answer text via the existing `SubmitDriverContent` path (line 394) instead.

##### Task 1.7.2b: Mark the request delivered after injection (~3 min)
- Add a `delivered_at` optional timestamp field to `GuidanceRequestData`
  (small addition to Task 1.1.1a/1.1.2 — note in this task rather than
  reopening Epic 1.1, since it's discovered here) and a
  `Storage.MarkGuidanceRequestDelivered` method, called right after
  `SubmitDriverContent` succeeds in Task 1.7.2a, so a subsequent turn/driver
  restart doesn't re-inject the same answer twice.

### Story 1.7.3: Tests
**Files**: `session/autonomous_driver_guidance_test.go` (new).
**Storage for these tests**: since Task 1.7.0a now wires `d.storage` as the
concrete `*Storage` rather than a narrow interface, these tests construct a
real `*Storage` via `session.NewTestEntRepository(t)` (same in-memory-ent
pattern Task 1.1.3a already uses) and pass it to `WithGuidanceStorage(...)`
— not a hand-rolled fake. This is not new test machinery: it is the same
helper Epic 1.1's storage tests already depend on.

##### Task 1.7.3a: `directiveAskQuestion` creates a request and doesn't end the loop (~4 min)
- Fake headless call returning a `QUESTION:` response; assert a
  `GuidanceRequest` is created scoped to the session; assert the driver
  continues waiting (not `directiveDone`'s loop-exit behavior).

##### Task 1.7.3b: A fresh driver instance picks up an answer written by a "different process" (~5 min)
- Simulate restart: create driver A, have it ask a question (creating a
  persisted `GuidanceRequest`); answer the request directly via
  `storage.AnswerGuidanceRequest` (simulating a human answering while driver
  A's process was down); construct a brand-new `AutonomousDriver` instance
  (driver B) for the same session; assert its very first turn queries
  storage, finds the answered request, and injects it — without needing any
  in-memory state carried over from driver A. This proves Task 1.7.2a's
  per-turn lookup is durable across a driver-goroutine restart on the *same*
  session, but — as the re-review's Blocker noted — does not exercise
  `onAutonomousDriverComplete` at all, so it cannot by itself prove that
  outcome ever reaches that code with the original session intact. See Task
  1.7.3c for the test that covers that path.

##### Task 1.7.3c: `onAutonomousDriverComplete` skips teardown/respawn for `AwaitingGuidance` (~5 min)
**Files**: `server/services/autonomous_orchestration_service_guidance_test.go`
(new — `onAutonomousDriverComplete` lives in `server/services`, a different
package from `session/autonomous_driver_guidance_test.go`).
- This is the test the re-review's Blocker asked for: it exercises the real
  production path (`onAutonomousDriverComplete`), not a hand-constructed
  second driver. Set up an `AutonomousOrchestrationService` with a
  backlog-linked `ItemSession` (`SessionRoleWork`) whose collaborators —
  the storage's `MarkStuck`, `UpdateItemSessionEnded`, and the
  `autonomousStuckRespawner`'s `AutoRespawnAutonomousWork`
  (`AutonomousStuckRespawner` interface, `server/services/
  autonomous_orchestration_service.go:31`) — are fakes/mocks recording
  whether they were called. Invoke `onAutonomousDriverComplete` directly
  with `session.AutonomousDriverOutcome{Done: false, Stuck: true, Turns:
  d.maxTurns, Reason: "awaiting_guidance", AwaitingGuidance: true}` (Task
  1.7.1c's shape) for that session.
  - Assert `MarkStuck` (with `domain.StuckReasonAutonomousStuck`) was NOT
    called.
  - Assert `UpdateItemSessionEnded` was NOT called (the `ItemSession`'s
    `EndedAt` stays nil).
  - Assert `AutoRespawnAutonomousWork` was NOT called.
  - As a control, also assert that an otherwise-identical outcome with
    `AwaitingGuidance: false` (an ordinary turn-cap give-up) DOES trigger all
    three — proving the branch, not just the absence of a crash, is what's
    under test.

---

# Phase 2: UI Integration (PR #2)

## Epic 2.1: Frontend Data Layer
**Goal**: A streaming hook + Redux slice mirroring
`useWatchBacklogItems.ts`/`backlogItemsSlice`, per `research/stack.md` §5.

### Story 2.1.1: Redux slice
**Files**: `web-app/src/lib/redux/guidanceRequestsSlice.ts` (new — exact
sibling path to wherever `backlogItemsSlice` lives; confirm via `Glob
web-app/src/lib/redux/*Slice.ts` before creating).

##### Task 2.1.1a: Slice with upsert/remove/mark-answered reducers (~4 min)
- New file. State: `{ byId: Record<string, GuidanceRequestDomain>, ids:
  string[] }`. Reducers: `upsertGuidanceRequest` (single-entry, targeted —
  never a full re-snapshot, mirroring `activityNoteAdded`'s precedent at
  `useWatchBacklogItems.ts:313-316`), `removeGuidanceRequest`,
  `setGuidanceRequests` (bulk, for the initial snapshot fetch).

##### Task 2.1.1b: Domain mapping function (~3 min)
- In the same slice file or a sibling `guidanceRequestMappers.ts`:
  `mapGuidanceRequest(proto: GuidanceRequestProto): GuidanceRequestDomain`
  mirroring `useBacklogService.ts`'s `mapBacklogItem` convention — never
  expose raw proto shapes to components.

### Story 2.1.2: `useWatchGuidanceRequests` hook
**Files**: `web-app/src/lib/hooks/useWatchGuidanceRequests.ts` (new).

##### Task 2.1.2a: Client setup + snapshot fetch + stream open (~5 min)
- New file, structurally copying `useWatchBacklogItems.ts`: a
  `createClient(GuidanceService, getWatchTransport())` client created once;
  an immediate REST `listGuidanceRequests` call fired in parallel with (not
  gated behind) opening the `watchGuidanceRequests` stream — same
  "Task 4.2.1a/pitfalls.md #1" dual-path freshness pattern.
- Accept `{ itemId?: string; sessionUuid?: string }` params, matching
  `WatchGuidanceRequestsRequest`'s filter shape (Task 1.2.1c).

##### Task 2.1.2b: `after_seq` gap detection + reconnect/backoff (~5 min)
- Port `useWatchBacklogItems.ts`'s `after_seq` tracking (lines 244-276),
  exponential-backoff reconnect (`MAX_RETRIES = 5`, lines 391-410), and
  REST-polling fallback (`FALLBACK_POLL_INTERVAL_MS = 30_000`, lines
  431-443) verbatim in shape, parameterized for the new stream.

##### Task 2.1.2c: Event dispatch switch (~4 min)
- `switch (event.event.case)` dispatching `created`/`answered` into
  `upsertGuidanceRequest`, and `snapshot_complete` into a no-op (matching
  `useWatchBacklogItems.ts`'s handling of the analogous marker, per
  `backlog.proto:1138-1148`'s comment on why it must exist).

### Story 2.1.3: Hook tests
**Files**: `web-app/src/lib/hooks/useWatchGuidanceRequests.test.ts` (new).

##### Task 2.1.3a: Snapshot + live-event dispatch test (~4 min)
- Mirror the existing `useWatchBacklogItems.test.ts`'s structure (locate via
  Glob first) for the analogous cases: initial snapshot populates state; a
  live `answered` event upserts without a full refetch.

---

## Epic 2.2: Shared `GuidanceRequestForm` Component
**Goal**: One component rendering all three question types, reused across
all three view integrations (Epics 2.3-2.5) rather than three bespoke forms —
avoids the `jscpd` duplication gate (root `CLAUDE.md`'s "Duplication and
hotspot checks") across near-identical form markup.

### Story 2.2.1: Component + styles
**Files**: `web-app/src/components/guidance/GuidanceRequestForm.tsx` (new),
`web-app/src/components/guidance/GuidanceRequestForm.css.ts` (new).

##### Task 2.2.1a: Yes/no + multiple-choice rendering via `RadioGroup` (~5 min)
- New directory `web-app/src/components/guidance/`. Props:
  `{ request: GuidanceRequestDomain; onAnswer: (answer: string) => Promise<void>
  }`. For `question_type === "yes_no"`: render `RadioGroup` with two fixed
  options (`{value: "yes", label: "Yes"}`, `{value: "no", label: "No"}`) —
  reusing `RadioGroup` unmodified per ADR-003, not a bespoke yes/no button
  pair. For `"multiple_choice"`: render `RadioGroup` with `request.choices`
  mapped to `RadioGroupOption<string>[]`.

##### Task 2.2.1b: Short-answer rendering + submit handling (~4 min)
- For `"short_answer"`: a plain styled `<textarea>` (per
  `research/stack.md` §6's finding that no reusable free-text primitive
  exists — commodity, define locally) with a submit button, disabled while
  empty. Shared submit path for all three types calls `onAnswer` and shows a
  pending/error state (no optimistic-only UI — wait for the RPC to actually
  succeed before showing "answered," per the Evidence-and-Claims "read a
  mutation back" principle applied to UI).
- Render `status === "answered"` as a read-only summary (question + the
  recorded answer), not the interactive form, for already-answered requests
  appearing in a list.

##### Task 2.2.1c: Component styles (~3 min)
- New file `GuidanceRequestForm.css.ts` (vanilla-extract, per
  `docs/reference/css-architecture.md`), matching this component family's
  existing visual language (check `VaguenessPromptModal.css.ts` for the
  established dialog/form token usage before inventing new tokens).

### Story 2.2.2: Component tests
**Files**: `web-app/src/components/guidance/GuidanceRequestForm.test.tsx` (new).

##### Task 2.2.2a: Renders each question type correctly and calls `onAnswer` (~5 min)
- Three cases (yes_no, multiple_choice, short_answer): render, interact
  (click a radio / type text), submit, assert `onAnswer` called with the
  expected value. One case: `status === "answered"` renders read-only, no
  interactive controls, `data-testid` present per `e2e-test-conventions`.

---

## Epic 2.3: `BacklogItemDetail.tsx` Integration
**Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx`.

##### Task 2.3.1a: Subscribe and render pending requests for this item (~5 min)
- Add `useWatchGuidanceRequests({ itemId: item.id })`; render a new section
  (e.g. "Guidance Needed") listing pending `GuidanceRequestForm`s for this
  item, positioned near the existing `TriageErrorBanner`/stuck-state banners
  (consistent with how other item-level alerts are surfaced in this file —
  confirm exact insertion point by reading the file's existing banner-section
  layout before inserting, do not guess placement).

##### Task 2.3.2a: Answer wiring calls `answerGuidanceRequest` RPC (~3 min)
- `onAnswer` calls the `GuidanceService` client's `answerGuidanceRequest`
  (via the same client-creation convention `useBacklogService.ts` uses for
  other write RPCs), passing `answered_by` as a UI-session marker (e.g.
  `"web-ui"` or the browser's identifier if one already exists in this
  codebase's auth/session context — confirm via Grep for how other UI writes
  populate an actor field, e.g. `submit_review_verdict`'s caller identity).

---

## Epic 2.4: `TriageReviewPanel.tsx` Integration
**Files**: `web-app/src/components/backlog/TriageReviewPanel.tsx`.

##### Task 2.4.1a: Render the blocking guidance request for the item's triage session (~4 min)
- Same `useWatchGuidanceRequests({ itemId })` subscription (or lift it to a
  shared ancestor if `BacklogItemDetail.tsx` and `TriageReviewPanel.tsx`
  already share one — check before duplicating the hook call, per this
  repo's `jscpd` gate). Render the `GuidanceRequestForm` prominently when
  `automated_origin === true` and `status === "pending"` — this is the
  triage-specific "ask instead of guess" surface the requirement names.

##### Task 2.4.1b: Distinguish "triage is asking" from ordinary triage suggestions (~3 min)
- Ensure the existing free-text `TriageSuggestion{Rationale: "question"}`
  rendering (if `TriageReviewPanel.tsx` already shows suggestions) is
  visually/structurally distinct from the new structured
  `GuidanceRequestForm` — they represent different things (Task 1.6.1a kept
  them as separate concepts on purpose) and must not be visually merged.

---

## Epic 2.5: `SessionDetailView.tsx` Integration
**Files**: `web-app/src/components/sessions/GuidancePanel.tsx` (new),
`web-app/src/components/sessions/SessionDetailView.tsx`.

##### Task 2.5.1a: New `GuidancePanel.tsx` modeled on `GoalPanel.tsx` (~5 min)
- New file, same directory as `GoalPanel.tsx`. `export function
  GuidancePanel({ sessionUuid }: { sessionUuid: string })` — subscribes via
  `useWatchGuidanceRequests({ sessionUuid })`, renders pending
  `GuidanceRequestForm`s for this session (the autonomous-driver-originated
  case from Epic 1.7). Renders nothing (returns `null`) when there are no
  pending requests, matching `GoalPanel`'s pattern of only showing when
  relevant.

##### Task 2.5.1b: Wire into `SessionDetailView.tsx` (~2 min)
- Add `import { GuidancePanel } from "./GuidancePanel";` (mirroring the
  `GoalPanel` import at `SessionDetailView.tsx:29`) and render
  `<GuidancePanel sessionUuid={session.uuid} />` near the existing
  `<GoalPanel goal={session.goal} />` call (`SessionDetailView.tsx:1642`).

##### Task 2.5.2a: `GuidancePanel` tests (~3 min)
- New file `GuidancePanel.test.tsx`. Renders nothing with zero pending
  requests; renders a form with one; calls the answer RPC on submit.

---

## Epic 2.6: `StuckReasonAwaitingGuidance` Badge + Feature Registry
**Files**: wherever the board/item-detail `StuckReason` badge is rendered
(the file PR #769 touched for the shared priority order — confirm exact path
via Grep for `StuckReasonPlanNotApproved` in `web-app/src/` before editing).

##### Task 2.6.1a: Add a label/icon for `"awaiting_guidance"` to the badge mapping (~3 min)
- Add the new reason string to whatever `StuckReason → label/icon` mapping
  object the board and item detail share (per PR #769's "single shared
  StuckReason priority order" — this should be one mapping to edit, not two).

##### Task 2.6.1b: Run `make registry-generate` for frontend markers (~3 min)
- Add `// +feature: guidance-request-form` (or similarly-scoped IDs) to the
  first 10 lines of `GuidanceRequestForm.tsx` and `GuidancePanel.tsx` per
  root `CLAUDE.md`'s marker convention, then run `make registry-generate` and
  commit the resulting `docs/registry/features/frontend/*.json` files.

---

## Epic 2.7: E2E Tests
**Files**: `tests/e2e/guidance-requests.spec.ts` (new), `tests/e2e/pages/` (new page helper if needed).

##### Task 2.7.1a: End-to-end create-answer-see-it-resolved flow (~5 min)
- New spec file starting with `// @feature guidance:create, guidance:answer,
  ...` per `e2e-test-conventions`. Flow: seed a pending guidance request via
  the test server's API/MCP surface (not the UI, since creation is an
  LLM/session action, not a user action), navigate to
  `BacklogItemDetail`, assert the form renders via `data-testid`/ARIA-role
  locators (no CSS class selectors), answer it, assert
  `expect(locator).toHaveValue(...)`/the read-only "answered" state appears
  — no `waitForTimeout` anywhere.

##### Task 2.7.1b: Verify the `StuckReasonAwaitingGuidance` badge appears/clears (~4 min)
- Same or a sibling spec: assert the badge is visible while pending, assert
  it disappears after answering (per Task 1.5.2b's auto-clear).

---

## Follow-On Work Explicitly Deferred (not part of either PR)

- A global "guidance inbox" UI view for standalone-scoped requests (ADR-004).
- Multi-select multiple-choice and its checkbox-group UI primitive (ADR-003).
- Slack interactive-button rendering for guidance requests (named as a
  natural but separate extension in `research/stack.md` §6 and
  `research/features.md` §4 — this codebase's existing
  `slack_interactive_handler.go` already does the analogous thing for tool
  approvals; extending it to guidance requests is real but independent work).
- Active resumption of a paused session on answer (calling `resume_session`)
  rather than the chosen poll-on-next-turn/next-triage-pass model
  (`research/features.md` §2's option (b), explicitly deferred there in favor
  of option (a)).
- Event-driven auto-restart of an `AutonomousDriver` for a
  `maxTurns`-exhausted, awaiting-guidance session once its `GuidanceRequest`
  is answered (Epic 1.7's "Known Limitation" callout after Task 1.7.1d,
  adversarial review round 3's Concern 2 — distinct from the
  `resume_session`-on-a-still-looping-driver case above, since here the
  driver has fully exited, not merely paused). The manual `UpdateSession`
  autonomous-mode toggle (`server/services/session_service.go:3157-3166`) and
  the 14-day expiry TTL (Task 1.5.3a) are the interim backstops; this closes
  the gap properly instead of relying on either.
