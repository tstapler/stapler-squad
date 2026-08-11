# Research: Build vs. Buy — respawn-event audit trail + UI wiring

**Date**: 2026-08-01
**Scope**: `project_plans/backlog-session-lifecycle-ux/requirements.md` — (a) new append-only respawn-event table + read RPC + timeline UI, (b) UI wiring of 3 already-persisted backend fields into existing widgets.

## 1. Existing OSS library/framework for audit log / activity feed

**ent audit-log support**: ent has no built-in audit-log extension. Community options exist (`entgo.io/contrib` has no audit-log package as of this writing; third-party ent audit-log generators exist on GitHub but are unmaintained/low-adoption single-purpose codegen plugins, not general infra). None are already a dependency (`go.mod` has no audit/event-sourcing library — grep for "audit"/"event" in `go.mod` returned nothing beyond the project's own code).

**Precedent already in this exact codebase**: `session/ent/schema/` already contains *five* ent schemas that are exactly this pattern — an append-only, timestamped, foreign-keyed event log:
- `backlog_status_event.go` — `BacklogStatusEvent` (status transitions per backlog item)
- `analytics_event.go`
- `error_event.go`
- `escape_event.go`
- `source_sync_event.go`

This is a recurring, already-solved shape in this codebase, hand-rolled each time with plain ent schemas (`id`, `item_id`/FK, a few descriptive string fields, `created_at`, one composite index). There is no "event log framework" wrapping these — each is ~30-50 lines of straight ent schema.

**React/frontend activity-feed library**: no timeline/activity-feed npm package is installed (`web-app/package.json` has no `react-activity-feed`, `react-timeline-*`, etc.). The closest available primitives are Radix (`@radix-ui/react-accordion`, `-dialog`, `-tabs`, `-tooltip`) plus the project's own `Collapsible`/`CollapsibleSection` components, already used for exactly this kind of progressive-disclosure list.

**Proportionality**: the requirements state 4 write call sites and one bounded read (tens of events per item, no aggregation). This is far below the complexity threshold where a general audit-log library pays for itself (schema flexibility, polymorphic actor/target modeling, built-in diffing) — those libraries solve problems (arbitrary entity/field diffing, multi-tenant audit compliance) this feature doesn't have.

- **Verdict: Not recommended.** No proportionate library exists for the Go/ent side; the existing in-repo pattern (`BacklogStatusEvent` et al.) already is the "library" for this codebase's conventions, at effectively zero adoption cost since the pattern is copy-paste-adapt.

## 2. SaaS/managed API

Not applicable. `stapler-squad` is a local, self-hosted, single-user tool (SQLite-backed, `localhost:8543`, per `CLAUDE.md`). A managed audit-log/event SaaS (e.g. Segment, PostHog activity feeds, LogRocket) would introduce a network dependency, external account, and data-residency question for a feature whose whole job is showing the single user local session history. Confirmed and dismissed.

- **Verdict: Not recommended** (out of scope for this tool's architecture).

## 3. LLM-generated implementation vs. battle-tested library

Given: an append-only table (5 fields: id, item_id FK, reason, triggering_session_id, resulting_session_id, created_at), one composite index (`item_id`, `created_at`), and a bounded read (no pagination cursor needed at "tens of events per item" per the requirements' own Feasibility Risks / Non-functional Requirements sections). This is materially *simpler* than `BacklogStatusEvent`, which already ships in this codebase and has an `Optional().Nillable()` field plus a `triggered_by` default — patterns to copy directly.

Correctness risk analysis:
- The ent schema itself (fields + one edge + one index) is boilerplate matching an existing, tested template almost line-for-line — low risk of subtle bugs, and any generated-code error surfaces immediately via `go build` after running the mandatory `--feature sql/upsert` generate command (`.claude/rules/ent-schema-generation.md`).
- The actual risk surface is the 4 write call sites in `backlog_service_triage.go` (`AutoRespawnReview` ~line 1663-1689, `AutoRespawnAutonomousWork` ~line 1387-1410, `AutoRespawnTriage` ~line 1727, `RemediateStaleWorkSession` ~line 1488) needing a DB write added alongside their existing `log.InfoLog.Printf` calls — this is app logic, not something a generic library would do for you regardless of choice; a library only helps with the schema/storage layer, which is the low-risk part here.
- Per `.claude/rules/interface-pollution-checklist.md`, this codebase explicitly penalizes speculative abstraction layers (Manager/Handler/Repository wrapping with no added behavior). Reaching for a generic audit-log library here would itself risk violating that rule — it would add an abstraction layer (mapping this domain's respawn semantics onto a generic library's generic event schema) with no correctness benefit over the concrete, minimal ent table this codebase already uses for every other event-log need.

- **Verdict: Recommended — hand-write, following the `BacklogStatusEvent` template.** Lower risk than adopting a library for this scope, and consistent with repo conventions.

## 4. Fork or adapt — best existing template

**Best template: `session/ent/schema/backlog_status_event.go` (`BacklogStatusEvent`)**, verbatim structural match:

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
		edge.From("item", BacklogItem.Type).Ref("status_events").Field("item_id").Unique().Required(),
	}
}

func (BacklogStatusEvent) Indexes() []ent.Index {
	return []ent.Index{index.Fields("item_id", "created_at")}
}
```

A respawn-event schema maps onto this near 1:1: swap `from_status`/`to_status` for `reason` (the respawn trigger: stale-work, review-cap-not-hit, autonomous-turn-exhausted, triage-retrigger — one string field is sufficient per the "flat event log, no aggregation" rabbit-hole guardrail) and add two nullable session-reference fields (`triggering_session_id`, `resulting_session_id`) to satisfy the requirement "capturing reason, timestamp, triggering session ID, resulting session ID." `triggered_by` can be dropped or repurposed (respawns are always system-triggered, unlike status transitions which can be user- or system-triggered) — confirm in planning.

**Read path precedent**: `BacklogStatusEvent` is *not* exposed via a separate paginated RPC — it is embedded as `repeated BacklogStatusEvent status_events = 20;` directly on the `BacklogItem` proto message (`proto/session/v1/backlog.proto` line 133), returned as part of the existing `GetBacklogItem`/list response. The frontend then does client-side capping via the `useShowMore` hook (see `ProgressHistorySection.tsx`, `SHOW_MORE_CAP = 8`) rather than server-side pagination. This is the same "bounded, no complex query" shape the new respawn-event read path needs — **recommend the same embed-on-the-item-message + client-side-cap pattern** rather than inventing a new dedicated paginated RPC, since respawn events per item are also "in the tens" (per Feasibility Risks). A dedicated `ListRespawnEvents` RPC is *viable* if the planning phase decides item-detail payload size matters, but is not the default per this precedent.

**Write path precedent**: whichever pattern `BacklogStatusEvent` rows are inserted through today in `session/ent_repository_backlog.go` (repository method, not raw ent client calls scattered in `backlog_service_triage.go`) should be forked for the 4 respawn call sites — keeps the write path consistent with the rest of the backlog repository layer instead of introducing ad hoc ent calls inside `backlog_service_triage.go`.

- **Verdict: Recommended.** Fork `backlog_status_event.go` directly; do not design a new schema shape from scratch.

## 5. Frontend timeline UI — reuse vs. new dependency

**No timeline UI library needed or installed.** `ProgressHistorySection.tsx` (`web-app/src/components/backlog/detail/ProgressHistorySection.tsx`) is already a working example of exactly the target shape:
- Wrapped in `CollapsibleSection` (`@/components/ui/Collapsible`) — collapsed-by-default progressive disclosure, matching the requirement's explicit ask to reuse this primitive.
- Uses `useShowMore(item.id, "<section-key>", items, SHOW_MORE_CAP)` for the "show N more" bounded-list pattern — directly reusable for a respawn-event list.
- Uses `formatDate` from `@/lib/backlog/formatDate` (a project-local date formatter) for timestamps — **no `date-fns`/`dayjs`/`moment` is installed** in `web-app/package.json`; the project already has its own formatting utility, so no new date library is needed either.
- Renders each event as a `role="list"`/`role="listitem"` div, not a specialized timeline widget — plain CSS (vanilla-extract, per `ProgressHistorySection.css.ts`) handles the visual line/meta layout.

`WorkflowHistorySection.tsx` (same directory) is a second, near-identical precedent worth a quick look in planning as an alternate template if its event shape (multi-stage, not just linear notes) fits the respawn reason taxonomy better.

Installed UI deps that are relevant and already available: `@radix-ui/react-accordion` (if a nested/grouped timeline view is wanted), `lucide-react` (icons for reason/outcome badges), `react-virtuoso` (only relevant if event counts ever exceed the "tens" assumption — not needed at current scale).

- **Verdict: Recommended.** Build the respawn-event timeline as a new `RespawnHistorySection.tsx` (or similar) directly modeled on `ProgressHistorySection.tsx`: `CollapsibleSection` + `useShowMore` + `formatDate` + plain listitem rows + vanilla-extract CSS. No new frontend dependency.

## Summary Table

| Option | Pros | Cons | Verdict |
|---|---|---|---|
| 1. OSS audit-log/activity-feed library (Go/ent or React) | Handles generic polymorphic audit needs | No proportionate option exists as a dependency; adds abstraction this codebase's own rules (`interface-pollution-checklist.md`) discourage for a 5-field/4-call-site scope | **Not recommended** |
| 2. SaaS/managed API | N/A | Wrong architecture for a local single-user tool | **Not recommended** |
| 3. Hand-write (LLM-generated) vs. adopt library | Matches scope exactly; lowest correctness risk since it copies a tested in-repo template; consistent with repo's Go-idiomatic/concrete-over-abstract convention | Requires the 4 call-site edits regardless of schema choice (not avoidable by a library) | **Recommended** |
| 4. Fork `BacklogStatusEvent` (`session/ent/schema/backlog_status_event.go`) | Near-1:1 structural match; write/read patterns (repository method, embedded-on-item-message RPC field) already proven in production | Field semantics (`reason` vs `from_status`/`to_status`, two session-ID fields) need adaptation, not pure copy | **Recommended** |
| 5. Frontend: reuse `ProgressHistorySection.tsx` pattern (`CollapsibleSection` + `useShowMore` + `formatDate`) | Zero new dependencies; exact progressive-disclosure UX already established and required by the spec | None significant | **Recommended** |

## Bottom line

Build from scratch, not buy — but "from scratch" here means forking two already-proven in-repo templates (`BacklogStatusEvent` for the ent schema/write/read pattern, `ProgressHistorySection.tsx` for the timeline UI), not designing new patterns. No new Go or npm dependency is warranted at this scope.
