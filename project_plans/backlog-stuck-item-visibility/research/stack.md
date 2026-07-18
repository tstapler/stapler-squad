# Research: Stack — backlog-stuck-item-visibility

## 1. ent ORM (entgo.io/ent) — schema conventions

- **Version**: `entgo.io/ent v0.14.5` (go.mod). Go toolchain `go 1.26.3`.
- **Generation command** (from `session/ent/generate.go`, MUST be used verbatim):
  ```
  go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema
  ```
  Omitting `--feature sql/upsert` compiles fine but silently breaks `UpsertRule`-style
  generated methods (see `.claude/rules/ent-schema-generation.md`).
- **Migration model**: no versioned migration tool (no Atlas migrations directory found).
  Schema is applied via `client.Schema.Create(ctx)` at startup — found in
  `session/ent_repository.go:86` and `server/analytics/db.go:46`. This is ent's
  auto-migration mode: it diffs the schema against the live sqlite DB and applies additive
  changes (new tables/columns) automatically on process start. A new stuck-state table
  will Just Work by adding a schema file and regenerating — no manual migration step.
- **Existing schema files**: `session/ent/schema/*.go` — 19 files, flat package `schema`,
  one type per file. Relevant precedents to model a new schema on:
  - `backlog_item.go` — main entity: `field.UUID("id", uuid.UUID{}).Default(uuid.New)`,
    `field.String("status").Default("idea")`, `field.Time(...).Default(time.Now)`,
    optional/nillable fields via `.Optional().Nillable()`, edges via `edge.To`/`edge.From`
    with `Ref(...)`, and `Indexes()` returning `index.Fields(...)` composites
    (e.g. `index.Fields("status", "priority")`, `index.Fields("status", "updated_at")`).
  - `backlog_status_event.go` — **directly analogous pattern** for what this feature needs:
    an **append-only event log** table (`BacklogStatusEvent`) with `item_id` FK-by-value
    (not a hard edge), `from_status`/`to_status`, `triggered_by`, optional `note`,
    immutable `created_at`, and an edge declared from the *parent* side
    (`edge.From("item", BacklogItem.Type).Ref("status_events").Field("item_id").Unique().Required()`)
    plus a compound index `index.Fields("item_id", "created_at")`.
  - Cascade delete pattern: `edge.To("status_events", BacklogStatusEvent.Type).Annotations(entsql.OnDelete(entsql.Cascade))` in `backlog_item.go`.

  **Recommendation for durable stuck-state**: add a new ent schema (e.g.
  `BacklogStuckState` or extend `BacklogStatusEvent`-style event log with a
  `stuck_reason` field) following the `BacklogStatusEvent` shape — an append-only or
  upserted-by-item-id row keyed on `item_id` with fields for `reason` (enum-as-string:
  rework_cap / pr_ready_unmerged / abandoned_review / cycling), `notified_at`,
  `detected_at`, and enough context (verdict outcome, iteration count, PR number) to
  render in the UI without extra joins. Use `--feature sql/upsert` generation since
  "already notified" bookkeeping is naturally an upsert-on-detect / clear-on-resolve
  pattern (replacing the in-memory `stuckReviewNotified`/`staleWorkNotified` maps in
  `session/backlog_lifecycle.go` lines ~124-133).

## 2. ConnectRPC — existing service patterns

- Proto files: `proto/session/v1/{backlog,unfinished,session,types,events,...}.proto`.
- **`BacklogService`** (`proto/session/v1/backlog.proto:351`) is the natural home for a
  new stuck-items RPC — it already owns all backlog item CRUD/lifecycle RPCs (28 RPCs:
  `ListBacklogItems`, `TransitionBacklogItemStatus`, `GetBacklogItemDiff`,
  `TriggerReReview`, `SubmitManualReview`, etc.), all implemented in
  `server/services/backlog_service.go`. This is a request/response (non-streaming)
  service — no existing streaming RPC on `BacklogService`.
- **`UnfinishedWorkService`** (`proto/session/v1/unfinished.proto`) is scoped to *git
  worktree* state (dirty/unmerged working directories), not backlog item status — it has
  no concept of backlog item ID, review verdicts, or PR mergeability. Piggybacking here
  would require bolting backlog-domain fields onto a git-worktree-domain message, which
  cuts against its existing scope. **Recommendation: do NOT piggyback on
  UnfinishedWorkService — add a new RPC to `BacklogService`** (e.g.
  `ListStuckBacklogItems` returning `repeated StuckBacklogItem` with `item_id`, `reason`
  enum, `detected_at`, `context` string, `pr_url`/`pr_number` where applicable). This
  matches requirements.md's "location TBD in planning — extend `/unfinished` or new
  view" for the *frontend* only; the *backend* RPC domain is clearly backlog, not
  unfinished-work.
- If polling (not push) is acceptable for the new UI view, a simple unary
  `ListStuckBacklogItems` RPC (called on an interval or on page load, following the
  existing `ListBacklogItems` pattern in `backlog_service.go`) is the lowest-risk choice
  and matches this repo's dominant RPC style (only `UnfinishedWorkService` and the
  terminal/session output services use server-streaming).
- After any `.proto` edit: run `make proto-gen`, which regenerates
  `session/gen/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts` — both are
  committed to git (per `instinct_alias_session` memory: `web-app/src/gen` is tracked
  despite being gitignored).

## 3. Frontend — React/TypeScript, vanilla-extract, existing hook patterns

- CSS: this repo uses vanilla-extract (`.css.ts` colocated files) for all new components
  per `.claude/rules/css-architecture.md` — reference `web-app/src/styles/theme.css.ts`
  tokens (`vars.color.xxx`, `vars.space[n]`, etc.), never hardcoded hex or `var(--x)`
  strings in new `.css.ts` files.
- `web-app/src/lib/hooks/useUnfinishedWork.ts` is the closest existing analogue for a
  "durable background state, polled/streamed into a list" hook:
  - Uses `@connectrpc/connect` + `@connectrpc/connect-web` `createClient` /
    `createConnectTransport` with `createAuthInterceptor()` from `@/lib/config`.
  - Maintains a `Map<key, T>` in React state, exposes a derived sorted array.
  - For streaming RPCs it opens a `WatchXxx` stream with `AbortController`, handles
    `event.payload.case` discriminated union (`worktreeUpdated` / `worktreeRemoved` /
    `scanCompleted`), and auto-reconnects on disconnect via a timer ref.
  - Exposes an imperative `triggerScan()` action mapped to a unary RPC
    (`ScanUnfinishedWorkRequestSchema`).
  - A new `useStuckBacklogItems` hook (if the RPC is unary/polled rather than streamed)
    would be simpler: fetch on mount + `setInterval`, no `AbortController`/reconnect
    logic needed unless a `WatchStuckBacklogItems` streaming RPC is added instead.
- Components under `web-app/src/components/unfinished/*.tsx` (`UnfinishedItem.tsx`,
  `UnfinishedRepoGroup.tsx`, `UnfinishedNavBadge.tsx`, `UnfinishedItemDetail.tsx`, each
  with a paired `.css.ts`) are the pattern to mirror for a new "stuck items" list/detail
  view and nav badge — grouping component + item component + a small badge component
  showing a count, each with a small colocated vanilla-extract stylesheet.
- Per `.claude/rules/feature-registry.md`, any new RPC needs a
  `docs/registry/features/backend/<feature>.json` entry (with `// +api: backlog:list-stuck`
  marker in the handler) and any new React page/component needs a
  `docs/registry/features/frontend/<feature>.json` entry (with a `// +feature:` marker
  in the file's first 10 lines), followed by `make registry-generate`.

## 4. Existing Go dependencies for reconciliation / event publishing

- **No new Go dependency is needed.** The repo already has everything required:
  - **Periodic reconciliation**: plain stdlib `time.NewTicker` — the existing pattern
    lives in `server/dependencies.go` (~line 819-836): a 60s ticker goroutine wrapping
    `backlogLifecycleListener.ReconcileStuck(ctx)` in a `recover()`-guarded closure so a
    panic can't kill the reconciliation loop. This is the exact mechanism referenced in
    requirements.md root cause #1 ("`ReconcilePRPending`'s 60s poll"). A new stuck-reason
    detector should be added as another step inside `ReconcileStuck`
    (`session/backlog_lifecycle.go:519`), alongside `reconcileStuckReviewItems`,
    `reconcileStaleWorkSessions`, and `ReconcilePRPending`, all called from the same
    ticker tick — no new scheduler/cron library needed.
  - **Event publishing**: in-process pub/sub via `pkg/events` (`bus.go`, `types.go`,
    `subscriber.go`) — `events.EventBus.Publish(...)`, consumed today in
    `server/services/backlog_service_triage.go:33` via
    `s.eventBus.Publish(events.NewNotificationEvent(sessionID, sessionName,
    notificationID, notificationType, priority, title, message, metadata))`.
    `BacklogLifecycleListener.notify(itemID, title, message, notificationType,
    priority)` (session/backlog_lifecycle.go:241) wraps the same mechanism for backlog
    items. `server/events/forward.go` bridges these to the frontend (likely over the
    existing notification stream) — reuse `notify()`/`NewNotificationEvent` directly for
    the new "PR ready to merge" notification path (root cause #1) rather than building a
    new event type.
  - **maxAutoReworkIterations = 3`** is defined in
    `server/services/backlog_service_triage.go:55` and checked at lines 421/482 — this
    is the rework-cap logic referenced in root cause #2; do not change the value/policy
    per requirements.md scope, only add durable-visibility surfacing around it.
  - The in-memory maps to replace for durable state (root cause #3):
    `staleWorkNotified`/`stuckReviewNotifiedMu`/`stuckReviewNotified` fields in
    `BacklogLifecycleListener` struct, `session/backlog_lifecycle.go` lines ~119-133,
    used at lines 609-614 and 678-683. These reset on every service restart — replacing
    them with an ent-backed table (see §1) directly fixes root cause #3.

## Summary of concrete recommendations
1. New ent schema table (upsert-friendly, modeled on `BacklogStatusEvent`) for durable
   stuck-state, keyed by `item_id` + `reason`, regenerated with
   `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema`.
2. New unary RPC(s) on the existing `BacklogService` (not `UnfinishedWorkService`) in
   `proto/session/v1/backlog.proto` + `server/services/backlog_service.go`, regenerated
   via `make proto-gen`.
3. New React hook (`useStuckBacklogItems`-style, modeled on `useUnfinishedWork.ts`) +
   new components modeled on `web-app/src/components/unfinished/*.tsx`, styled with
   vanilla-extract `.css.ts` files per `.claude/rules/css-architecture.md`.
4. No new Go dependencies: reuse `time.NewTicker` (as in `server/dependencies.go`) for
   the periodic detection pass appended to `ReconcileStuck`, and reuse `pkg/events` +
   `BacklogLifecycleListener.notify()` for the new "PR green and mergeable" notification
   path.
5. Feature registry entries (backend + frontend JSON files) and `make registry-generate`
   are required before the PR is complete, per `.claude/rules/feature-registry.md`.
