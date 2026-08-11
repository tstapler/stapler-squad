# Architecture Research: Backlog Management Layer

**Date**: 2026-05-10
**Status**: Complete

---

## Summary

Three key decisions emerge from this research:

1. **Context injection via `.backlog-context.md` written to the worktree** — the only mechanism that survives session restart, requires no server-side injection on resume, and lets the agent read context at any time without a live MCP connection.
2. **Review gate runs as a dedicated short-lived Stapler Squad session** (not a raw Go goroutine) — leverages existing session infrastructure, keeps LLM calls out of the Go request path, and produces an auditable transcript.
3. **Agent lifecycle is event-driven spawn, not a persistent background session** — a persistent session is expensive and fragile; each triage or review action spawns a `one_shot` session that exits cleanly, matching the existing `OneShot` field in `InstanceData`.

---

## State Machine Design

### Lifecycle States

```
idea ──► ready ──► in_progress ──► review ──► done
  │        │            │             │
  └────────┴────────────┴─────────────┴──► archived
```

| State | Meaning | Entry Guard |
|---|---|---|
| `idea` | Captured but not fleshed out | — (creation default) |
| `ready` | AC defined, context complete, spawn-able | AC list non-empty |
| `in_progress` | At least one active session linked | Session spawned from item |
| `review` | Session(s) exited; gate pending | `EventExited` lifecycle event OR manual trigger |
| `done` | Gate passed (or manually overridden) | Gate verdict PASS or override |
| `archived` | Removed from active board | Any state, explicit user action |

### Transition Table

| From | To | Trigger | Guard |
|---|---|---|---|
| `idea` | `ready` | User action / agent suggestion | `len(acceptance_criteria) > 0` |
| `ready` | `in_progress` | Session spawned from item | session linked successfully |
| `in_progress` | `review` | `EventExited` hook OR agent calls `request_review` MCP tool | — |
| `in_progress` | `ready` | User aborts session without completing | explicit user action |
| `review` | `done` | Gate verdict = PASS | ReviewVerdict.outcome == PASS |
| `review` | `done` | Manual override | override_reason non-empty |
| `review` | `in_progress` | User requeues after FAIL/PARTIAL | explicit user action |
| `done` | `review` | User triggers re-review | explicit user action |
| any | `archived` | User action | — |

### How Session Events Drive Transitions

The existing `LifecycleListener` interface (`instance.go:75`) is the natural hook point. A `BacklogItemManager` implements `OnLifecycleEvent`:

- `EventExited` → look up `ItemSession` records linked to this session ID → if any are in `in_progress` state → transition item to `review` → enqueue review gate job.
- `EventStarted` → update `ItemSession.started_at` timestamp.

This keeps backlog state change logic out of the session service itself (additive, no breaking changes).

### Implementation Note

The `Status` int type (already used for sessions) should **not** be reused for backlog items. Define a separate `BacklogStatus` string type (e.g., `"idea"`, `"ready"`, `"in_progress"`, `"review"`, `"done"`, `"archived"`) stored as a string field in the ent schema, matching the convention in `session_type` (also a string field in the session schema).

---

## Plugin Architecture

### Interface Design

```go
// ItemSource is implemented by each external source plugin.
type ItemSource interface {
    // ID returns the stable plugin identifier (e.g., "github_issues").
    ID() string

    // Fetch returns raw items from the external source since the given cursor.
    // cursor is opaque — the caller stores and returns it unchanged.
    // Returning empty cursor means "start from scratch on next fetch."
    Fetch(ctx context.Context, cfg SourceConfig, cursor string) ([]RawItem, string, error)

    // MapToBacklogItem converts a RawItem into a BacklogItem upsert payload.
    // The plugin controls field mapping; the core controls conflict resolution.
    MapToBacklogItem(raw RawItem) BacklogItemDraft

    // ExternalID returns the stable external identifier for deduplication.
    ExternalID(raw RawItem) string
}
```

### Polling vs. Webhooks

**Recommendation: polling with configurable interval (default 15 min), webhooks as opt-in enhancement.**

Rationale:
- Polling requires no inbound network exposure (no webhook receiver to run and secure).
- GitHub Issues webhooks need a public HTTPS endpoint — incompatible with the default `localhost:8543` deployment model.
- A `SourceSyncer` goroutine runs a ticker; each tick calls `ItemSource.Fetch()` and processes results.
- Webhook support can be added later as a second transport without changing the `ItemSource` interface.

### Sync and Conflict Resolution

**Model: local-wins for fields the user has touched.**

Each `BacklogItem` tracks a `user_modified_fields` bitmask (or JSON set of field names). On upsert:

1. Call `ItemSource.Fetch()` to get new/updated raw items.
2. For each item, look up by `(source_id, external_id)`.
3. If not found → create with `user_modified_fields = {}`.
4. If found → for each field in the incoming payload, skip if that field name is in `user_modified_fields`; otherwise overwrite.
5. Update `source_last_seen_at` and advance cursor.

**Idempotent upserts** use ent's `--feature sql/upsert` (already required by the existing codebase — see CLAUDE.md). The upsert key is `(source_id, external_id)`.

**Sync log**: a `SourceSyncEvent` ent entity stores per-run results (run timestamp, items created/updated/skipped/errored, cursor before/after). This satisfies US-12.

---

## Context Injection Decision

### Options Evaluated

**Option A: Write `.backlog-context.md` to the worktree root**

Pros:
- Persists across session restarts with no server involvement.
- Agent can `Read` it at any time, including after network interruption.
- No MCP connection required for the agent to orient itself.
- Standard file conventions — agents already read `CLAUDE.md` from the repo root.
- Works with any session type (directory, worktree, one-off).
- Contents are version-controllable (can be `.gitignore`d or committed as an artifact).

Cons:
- Writes a file to the user's working directory (potentially unexpected).
- Requires cleanup when the item is archived or the session is abandoned.
- Not real-time — if AC changes after spawn, the file is stale until regenerated.

**Option B: Prepend to `CLAUDE.md`**

Pros:
- Claude Code reads `CLAUDE.md` automatically on startup.

Cons:
- Mutates the user's project instructions file — a destructive operation that corrupts the git working tree.
- No clean rollback if the session is abandoned mid-work.
- Conflicts if multiple sessions share the same repo directory.
- Breaks `CLAUDE.md` for other sessions on the same repo.

**Option C: MCP resource the agent pulls**

Pros:
- Always up-to-date (agent fetches current item state on demand).
- No filesystem writes.

Cons:
- Requires a live MCP connection to the Stapler Squad server — not guaranteed if the server restarts.
- Agent must know to call `get_backlog_item` at startup; nothing forces this without injecting a prompt.
- Adds latency on every orientation call.
- Does not work offline or when the MCP server is restarting.

### Decision: Option A — `.backlog-context.md` in the worktree

Write a single file at `<worktree_root>/.backlog-context.md` at session spawn time. Contents:

```markdown
# Backlog Item Context

**Item ID**: <uuid>
**Title**: <title>
**Priority**: <1-5>
**Status**: in_progress

## Description
<description>

## Acceptance Criteria
- [ ] <criterion 1>
- [ ] <criterion 2>

## Notes
<notes>

## Source
<optional: GitHub issue URL>

---
*This file is managed by Stapler Squad. Do not edit manually.*
```

Add `/.backlog-context.md` to the repo's `.gitignore` template when creating the worktree, or keep it as an untracked file (the review gate reads it from the worktree path, not git history). The MCP tool `get_backlog_item` remains available as a supplement for real-time AC updates, but the file is the primary context vector.

---

## Review Gate Architecture

### Option A: Go goroutine calling LLM API directly

Pros:
- Simple: no tmux session, no process management.
- Faster: no session startup overhead.

Cons:
- Adds Anthropic API client dependency to the Go server (currently none).
- LLM calls block a goroutine or require complex async handling.
- No transcript / audit trail of the review reasoning.
- Harder to iterate on the review prompt without rebuilding the binary.
- Error handling (rate limits, timeouts) must be implemented from scratch.
- Inconsistent with the project's philosophy: agents run in sessions.

### Option B: Dedicated short-lived Stapler Squad session

Pros:
- Reuses all existing session infrastructure (tmux, worktree, lifecycle events).
- Transcript stored in tmux scrollback — auditable reasoning.
- Review prompt can be iterated without code changes (it's a prompt file or injected text).
- Fits the existing `one_shot = true` + `status = OneShot` pattern already in `InstanceData`.
- `EventExited` from the review session triggers verdict parsing and DB write.
- Rate limit handling, retries, and context window management are Claude Code's problem, not ours.

Cons:
- ~3-5s session startup overhead per review.
- Requires the review session to write its verdict somewhere parseable (stdout, a file, or MCP tool call).

### Decision: Dedicated short-lived session

Spawn a `one_shot` session with:
- Working directory: the worktree of the completed session.
- Initial prompt: structured template including the git diff, AC list, and verdict output format instructions.
- `MCPServerURL` injected so the session can call `submit_review_verdict(itemId, verdicts[])`.

The `submit_review_verdict` MCP tool writes the `ReviewVerdict` record and triggers the `review → done` or `review → in_progress` transition. The Go server does not need to parse LLM output — the agent calls the structured tool.

**Verdict output format** (via MCP tool, not text parsing):
```
submit_review_verdict(
  item_id: "<uuid>",
  session_id: "<uuid>",
  verdicts: [
    { criterion_index: 0, outcome: "PASS", evidence: "Added tests in foo_test.go lines 42-67" },
    { criterion_index: 1, outcome: "FAIL", evidence: "No migration found for DB schema change" },
  ],
  summary: "Overall: PARTIAL"
)
```

---

## Drift Detection

Drift = agent is running but making no meaningful progress toward the item's AC. Options:

### 1. Git diff polling (recommended primary signal)

Every N minutes (default: 10 min), compare `git diff <base_commit>..HEAD` byte count for the session's worktree. If the diff has not changed for M consecutive polls (default: 3 = 30 min), flag as "possibly drifted." This piggybacks on the existing `DiffStatsData` infrastructure.

Implementation: extend the existing `PRStatusPoller` pattern — a background goroutine per active `in_progress` item that polls diff size and writes to `ItemSession.last_diff_change_at`.

### 2. Heartbeat via MCP tool (recommended supplement)

The `report_progress` MCP tool (US-14) is the agent's explicit progress signal. Each call updates `ItemSession.last_progress_at`. If `last_progress_at` is stale relative to `last_diff_change_at`, the item is a candidate for drift notification.

### 3. Token budget tracking (low value, skip for MVP)

Token usage requires parsing Claude Code's output or using the API directly — neither is currently available in Stapler Squad. Not recommended for MVP.

### 4. Session exit code (event, not drift signal)

`EventExited` is already the trigger for the `in_progress → review` transition. Exit code indicates crash vs. clean exit, but doesn't distinguish "drifted" from "finished." Use as a complement, not a drift detector.

### Recommended approach for MVP

- **Primary**: Git diff polling every 10 min; flag after 30 min of no diff change.
- **Supplement**: `report_progress` MCP calls reset the "possibly drifted" timer.
- **Notification**: when drift detected, add a notification card (same channel as `NeedsApproval` notifications) rather than taking autonomous action. Human decides.
- **No autonomous intervention** in MVP — avoids the failure mode of Gastown/Beads.

---

## Agent Lifecycle Recommendation

### Option A: Persistent background session

One long-lived tmux session running the triage agent at all times.

Problems:
- Consumes terminal resources even when idle.
- Fragile: session crash = silent loss of capability until detected.
- Context window fills over time → quality degrades.
- Hard to monitor from the existing session list UI (always "Running").
- Exactly the failure mode of Gastown: persistent agents that drift.

### Option B: Event-driven spawn (recommended)

Spawn a `one_shot` session on demand, for a specific task, with a bounded prompt.

Triggers for spawning:
- User requests triage help for a specific item (US-4).
- Session `EventExited` triggers a review gate session.
- User requests "suggest what to work on next."

Each spawned session:
- Tagged with `backlog:triage` or `backlog:review` for UI filtering.
- Uses `one_shot = true` so it exits after completing its single task.
- Links to the `BacklogItem` via `ItemSession` with `session_role = "triage" | "review" | "work"`.
- Is visible in the session list, so the user can observe it and intervene.

This matches the project's philosophy ("explicit approve gates") and uses infrastructure already present in `InstanceData.OneShot` and the `OneShot` ent field.

**Lifecycle of a triage spawn:**
1. User clicks "Help me flesh out this item" → server spawns `one_shot` session.
2. Session prompt = structured template: item title/description + instruction to output structured AC suggestions.
3. Agent calls `report_progress` or `submit_triage_suggestions` MCP tool with structured output.
4. Session exits → `EventExited` fires → server reads verdict from DB → updates item.
5. Triage session appears briefly in session list tagged `backlog:triage`; auto-hides after 24h if `Stopped`.

---

## Data Model Sketch

### Entities

```
BacklogItem
├── id              UUID (PK)
├── title           string (not empty)
├── description     text (markdown)
├── acceptance_criteria  JSON ([]AcCriterion{index, text, status})
├── priority        int (1–5)
├── labels          []string (stored via edge to Label entity, or JSON for MVP)
├── status          string ("idea"|"ready"|"in_progress"|"review"|"done"|"archived")
├── user_modified_fields  JSON (set of field names for conflict resolution)
├── notes           text (markdown, user-only, never overwritten by sync)
├── created_at      time
├── updated_at      time
├── archived_at     time (nullable)
│
├── ── edges ──
├── item_sessions   []ItemSession
└── source          ItemSource (nullable FK)

ItemSession
├── id              UUID (PK)
├── session_uuid    string (FK → Session.uuid)
├── item_id         UUID (FK → BacklogItem)
├── session_role    string ("work"|"triage"|"review")
├── started_at      time (nullable)
├── ended_at        time (nullable)
├── created_at      time
│
└── verdict         ReviewVerdict (nullable, 1:1)

ReviewVerdict
├── id              UUID (PK)
├── item_session_id UUID (FK → ItemSession)
├── overall_outcome string ("PASS"|"FAIL"|"PARTIAL"|"UNVERIFIABLE")
├── per_criterion   JSON ([]CriterionVerdict{index, outcome, evidence})
├── summary         text
├── override_by     string (nullable — set if human overrode)
├── override_reason text (nullable)
├── override_at     time (nullable)
└── created_at      time

ItemSource
├── id              UUID (PK)
├── plugin_id       string ("github_issues") — identifies the ItemSource plugin
├── display_name    string
├── config          JSON (plugin-specific: org, repo, label_filters, sync_interval_minutes)
├── enabled         bool
├── sync_cursor     string (opaque, returned by ItemSource.Fetch)
├── last_synced_at  time (nullable)
├── created_at      time
└── updated_at      time

SourceSyncEvent
├── id              UUID (PK)
├── source_id       UUID (FK → ItemSource)
├── started_at      time
├── finished_at     time (nullable)
├── items_created   int
├── items_updated   int
├── items_skipped   int
├── items_errored   int
├── error_message   text (nullable)
└── cursor_after    string
```

### Key Relationships

```
BacklogItem  1──* ItemSession  1──0..1 ReviewVerdict
BacklogItem  *──1 ItemSource   (nullable: locally created items have no source)
ItemSource   1──* SourceSyncEvent
Session.uuid ──── ItemSession.session_uuid  (loose FK: string, not ent edge, to avoid schema coupling)
```

### Notes on Ent Implementation

- `BacklogItem`, `ItemSession`, `ReviewVerdict`, `ItemSource`, `SourceSyncEvent` are new ent schemas in `session/ent/schema/`.
- Use `--feature sql/upsert` (already required) for idempotent GitHub sync upserts, keyed on `(source_id, external_id)` — add `external_id` field to `BacklogItem`.
- Backlog ent schemas live alongside session schemas (same SQLite DB) — no second database.
- The link from `ItemSession` to `Session` uses a string UUID field (not an ent edge) to keep the backlog package free from a hard import dependency on the session package's generated ent code. A lookup service bridges them at runtime.

---

## Open Questions (Deferred to Planning Phase)

1. **Label storage**: JSON array on `BacklogItem` vs. a shared `Label` ent entity (with edges from both `BacklogItem` and `Session`). JSON is simpler for MVP; shared entity enables cross-domain label queries.
2. **AcCriterion status**: Should per-criterion completion status (`pending|done`) be stored on `BacklogItem` (updated by `report_progress`) or only on `ReviewVerdict`? Both have merit; storing on item gives live progress without a completed review.
3. **Multi-attempt items**: When an item moves from `review` back to `in_progress` (FAIL verdict → user requeues), does a new `ItemSession` record start, or does the same one resume? Recommendation: new `ItemSession` per spawn; `ReviewVerdict` links to the specific attempt.
4. **Backlog proto file location**: `proto/backlog/v1/` (new domain) vs. extending `proto/session/v1/`. Separate domain is cleaner and avoids bloating the session proto.
