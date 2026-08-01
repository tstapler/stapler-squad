# Implementation Plan: backlog-session-lifecycle-ux

**Feature**: Surface already-persisted session/pause/remediation reasoning in the board card and item detail, and add a durable respawn-event audit trail for the 4 automated respawn/remediation call sites.
**Date**: 2026-08-01
**Status**: Ready for implementation
**ADRs**: ADR-001-respawn-event-audit-trail.md

---

## Step 0.5 — Alternatives Considered (creative pass)

Three shapes were considered for the net-new piece of this project (the respawn-event audit trail); the UI-wiring piece (end_reason, pause_reason, remediation attempts) has only one credible shape per component and is covered directly in Pattern Decisions below.

1. **Flat append-only `RespawnEvent` ent table, embedded on `BacklogItem.respawn_events`** (chosen). *Strength*: mirrors two already-shipped, working precedents (`BacklogStatusEvent`, `BacklogProgressNote`) exactly — schema, write helper, eager-load, frontend `CollapsibleSection` — so there is no new pattern to review or maintain. *Weakness*: `BacklogItem` payload grows by one more eagerly-loaded array on every `GetBacklogItem` call, even for items that never respawn (mitigated: the array is empty/omitted for the common case, matching `status_events`).
2. **Separate `ListRespawnEvents(item_id)` RPC, paginated.** *Strength*: keeps `GetBacklogItem` payload lean and would scale cleanly if respawn volume ever became unbounded per item. *Weakness*: invents a new RPC shape and a new frontend fetch/loading-state path for a "tens of events per item" dataset that doesn't need pagination yet (per pitfalls.md, capping is still needed, but that's a `LIMIT` on the existing query, not a new endpoint) — an unjustified generic per the interface-pollution-checklist.
3. **Reuse `BacklogStatusEvent` itself, adding a `kind` discriminator column ("status_transition" vs "respawn") instead of a new table.** *Strength*: zero new schema/table. *Weakness*: conflates two different audit concerns with different field shapes (`from_status`/`to_status` vs `triggering_session_uuid`/`resulting_session_uuid`) into one nullable-everything table — exactly the "resolve-in-place vs. episode-history" conflation the requirements' Rabbit Holes section explicitly warns against, and a clear PoEAA violation (one table modeling two unrelated concepts).

Option 1 is chosen; options 2 and 3 are recorded as rejected alternatives in the Pattern Decisions table.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `ItemSession` | ent entity / Go DTO (`ItemSessionSummary`) representing one headless (triage/review) or interactive (work) session tied to a backlog item. | `session/ent/schema/item_session.go`, `session/repository.go` |
| `EndReason` | `ItemSession.end_reason` / `ItemSessionSummary.EndReason` — string classifying how a headless call ended: `""` (clean/unclassified), `shutdown`, `timeout`, `process_error`, `claude_not_found`, `other`. | Already committed at the ent/Go DTO layer; NOT yet on the proto `ItemSession` message — this plan adds that. |
| `PauseReason` | `Session.pause_reason` — string on the live tmux `Session`: `manual`, `auto:inactivity`, `auto:session_limit`, `auto:resource`. | Already on proto + `formatPauseReason.ts`; currently tooltip-only on `SessionCard.tsx`. |
| `BacklogStuckState` | ent entity tracking an open "item X is stuck for reason Y" row, including `remediation_attempts`/`next_remediation_at`/`context`. | `session/ent/schema/backlog_stuck_state.go` |
| `StuckBacklogItem` | Proto message / frontend type exposing a `BacklogStuckState` row, already including `remediation_attempts`/`next_remediation_at`/`context`. | `proto/session/v1/backlog.proto:1022` |
| `RemediationAttempts` | `StuckBacklogItem.remediation_attempts` — count of automated remediation attempts made for an open stuck row; `>= MAX_REMEDIATION_ATTEMPTS` (5) means "parked". | Already wired to `BlockerChip`'s data source (`stuckItem` prop); only the chip's rendering needs to change. |
| `RespawnEvent` | **New** ent entity / proto message: one append-only audit row per automated respawn/remediation attempt (`id`, `item_id`, `reason`, `triggering_session_uuid`, `resulting_session_uuid`, `created_at`). | Modeled on `BacklogStatusEvent`. |
| `RespawnReason` | **New** Go untyped string constants for `RespawnEvent.reason`: `RespawnReasonAutonomousTurn`, `RespawnReasonStaleWork`, `RespawnReasonReviewAbandoned`, `RespawnReasonTriageOrphaned`. | `session/backlog.go`, alongside `SessionRoleWork`. |
| `BlockerChip` | Existing shared frontend component rendering a `StuckBacklogItem` as an icon+label(+duration) chip. | Extended (not replaced) to also render a `×N` remediation-attempt suffix + next-retry `aria-label`. |
| `CollapsibleSection` / `CollapsibleGroup` | Existing shared frontend primitives for collapsed-by-default detail panels. | Reused verbatim for the new `RespawnHistorySection`. |
| `useShowMore` | Existing frontend hook capping a list's default rendering to N most-recent items, with a persisted "show more" expand choice. | Reused verbatim for `RespawnHistorySection`. |
| `RespawnHistorySection` | **New** frontend component (modeled on `WorkflowHistorySection.tsx`) rendering the `RespawnEvent` timeline in the item detail panel. | `web-app/src/components/backlog/detail/RespawnHistorySection.tsx` |
| `formatEndReason` / `EndReasonBadge` | **New** frontend formatter + inline chip surfacing `ItemSession.end_reason` in `SessionsSection.tsx`, following the same icon/text/`aria-label` triad as `formatPauseReason`/`BlockerChip`. | `web-app/src/lib/backlog/formatEndReason.ts` |
| `recordRespawnEvent` / `CreateRespawnEvent` | **New** repository write path (`session/ent_repository_backlog.go` + `session/storage.go`) appending a `RespawnEvent` row, best-effort (log-and-continue on failure), mirroring `recordStatusEvent`. | |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `RespawnEvent` read path | Embed `repeated RespawnEvent respawn_events` on `BacklogItem`, eager-loaded only in `GetBacklogItem` (not the board-list `ListBacklogItems`) | PoEAA (Fowler) — aggregate carries its own audit trail, matching `status_events`/`progress_notes` | Separate `ListRespawnEvents(item_id)` RPC | Two precedents (`BacklogStatusEvent`, `BacklogProgressNote`) already embed rather than paginate at "tens of events" scale; a new RPC shape is an unjustified generic (interface-pollution-checklist smell #5) here |
| `RespawnEvent` write path | Best-effort repository method (`CreateRespawnEvent`), log-and-continue on failure, called after each of the 4 respawn call sites' spawn attempt completes | PoEAA — Transaction Script (no cross-aggregate invariant to protect) | Wrap respawn + event write in one `ent.Tx` | `recordStatusEvent`'s own doc comment: "a write failure is logged, not returned: an audit-log gap must never block the status transition itself" — a respawn that already succeeded must not be reported as failed because the audit row failed |
| `RespawnEvent` session references | Plain string fields (`triggering_session_uuid`, `resulting_session_uuid`), no ent edge/FK | Type-driven design — value reference, not entity ownership | `edge.To(ItemSession)` foreign key | Matches `ItemSession.session_uuid`'s existing "loose FK, not an edge" convention; also must tolerate "resulting session doesn't exist yet" (a respawn attempt that failed before spawning, or hit the concurrency cap and queued instead) |
| Board-card & detail remediation-attempt surfacing | Extend existing `BlockerChip.tsx` (already receives the fully-populated `StuckBacklogItem` — including `remediation_attempts`/`next_remediation_at`/`context` — via the `stuckItem` prop client-side-joined by `useStuckBacklogItems()`) to render a `×N` suffix + next-retry `aria-label` | GoF — enhance the existing type rather than add a new one | New `RemediationBadge` component duplicating `BlockerChip`'s data source | **Verified live**: `BacklogItemCard.tsx` (line 208) and `LifecycleSummary.tsx` (line 48) already pass a fully-populated `StuckBacklogItem` into `BlockerChip`. The "board card needs a new field + a new backend join" risk research flagged (pitfalls.md §3) does not apply — the data already arrives; only `BlockerChip.tsx`'s rendering is incomplete. Avoids interface-pollution-checklist smell #6 and a redundant second backend join |
| `end_reason` surfacing scope | `SessionsSection.tsx` only (item detail, where `ItemSession` rows are rendered) | Type-driven design — respect the actual domain boundary | Also thread `end_reason` onto the generic `Session` proto so `SessionCard.tsx`/`SessionList.tsx` show it | **Verified**: `end_reason` is structurally an `ItemSession` field (headless triage/review call outcome); the generic tmux `Session` message (`SessionCard.tsx`/`SessionList.tsx`) carries no backlog/`ItemSession` linkage today. Forcing it through would require inventing an out-of-scope cross-entity join |
| `pause_reason` on `SessionList.tsx`/`SessionCard.tsx` | Badge-only (always-visible on `SessionCard.tsx`); no new filter/grouping control in v1 | Appetite — ship the documented fallback increment first | New `GroupingStrategy.PauseReason` / filter chip on `SessionList.tsx` | requirements.md's Open Questions defaults to "badge-only if appetite is tight"; the net-new `RespawnEvent` schema is already the larger risk item for this appetite |
| `RespawnReason` vocabulary | Untyped Go string constants in `session/backlog.go`, alongside `SessionRoleWork = "work"` | Type-driven design, calibrated to existing idiom | `type RespawnReason string` newtype with a validating constructor | The directly analogous existing vocabulary (`SessionRoleWork`) is already plain untyped string constants in the same file — match the established idiom (interface-pollution-checklist) rather than introduce a new pattern for a comparable concept |
| End-of-session/remediation chip visual language | Reuse `vars.color.successBg/successText/success`, `warningBg/warningText/warning`, `errorBg/errorText/error` tokens from `theme.css.ts` (same tokens `stuckReason.css.ts` already uses) | Existing design system | New status-color tokens for end_reason severity | `.claude/rules/css-architecture.md` — never hardcode hex, reuse the theme contract; these three severity tokens already cover "clean/in-progress/error" exactly |

---

## Migration Plan
- **Migration file**: none by hand — this project uses ent's auto-migration (`ent.Schema` → `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`, applied via the existing `ent` auto-migrate-on-boot path already used for every other schema in this repo). No manual SQL migration file exists elsewhere in `session/ent/schema/*.go` siblings, so none is added here.
- **Reversibility**: additive-only (one new table `respawn_events`, one new nullable/optional-fields column set — no columns dropped or renamed on any existing table). Reverting the code revert is sufficient; the orphaned table is inert and can be dropped manually if desired, but nothing depends on its absence.
- **Zero-downtime strategy**: single-user local SQLite-backed service restarted via `make install-service`; no rolling-deploy concern. ent's auto-migration runs on next boot.
- **Rollback procedure**: `git revert` the schema/proto/service commits; redeploy. The `respawn_events` table (if already created) is simply unused, not migrated away — matches this repo's existing rollback posture for additive ent schemas.

## Observability Plan
- **Logs**: `CreateRespawnEvent` write failures log via `log.ErrorLog.Printf` (mirrors `recordStatusEvent`), including item ID, reason, and the underlying error — never silently swallowed. Each of the 4 call sites keeps its existing `log.InfoLog.Printf` respawn-trigger line unchanged; the new DB write is additive, not a replacement.
- **Metrics**: none — single-user internal tool, no metrics infra exists for this subsystem (per requirements' Observability Requirements).
- **Alerts**: none — same reasoning.

## Risk Control
- **Feature flag**: none — additive, observability-only surfaces; no behavior change to gate. Matches requirements' Risk Control section ("not needed").
- **Rollback procedure**: see Migration Plan above; a simple revert.
- **Staged rollout**: N/A — single-user, single-environment deployment (`make install-service`).

## Unresolved Questions
- [ ] Exact copy/wording for the `RespawnHistorySection` empty state ("No respawns yet" vs. omit-when-empty vs. always-render-with-explicit-text) — this plan follows `WorkflowHistorySection`'s "always render, explicit empty state" precedent (Story 4.5.2) rather than `ProgressHistorySection`'s hide-when-empty, per features.md's explicit recommendation, but final copy is a 1-line judgment call for whoever implements Story 4.5.2 — owner: implementer, resolve inline, not a blocker.
- [ ] Whether `TriggerTriageResponse`/`TriggerReReviewResponse`'s `ItemSession` field is populated (non-nil) in every success path used by `AutoRespawnTriage`/`AutoRespawnReview`, or only in some — Task 4.4.2b/4.4.3a include a 2-minute verification read before wiring the resulting-session-uuid capture; if the field is ever nil on a success path, fall back to `resulting_session_uuid` unset (matches the schema's nillable design) rather than blocking — owner: implementer, resolve inline.

## Dependency Visualization

```
Phase 0 (verify BUG-053 state)
      │
      ├──────────────────────────────────────────────────────────┐
      ▼                                                            ▼
Phase 1 (end_reason: proto → backend → frontend)         Phase 2 (pause_reason badge promotion)
      │                                                            │
      │                                                  Phase 3 (BlockerChip ×N — pure frontend,
      │                                                            no dependency on Phase 1/2)
      │
      ▼
Phase 4 (RespawnEvent audit trail)
  4.1 ent schema ──▶ 4.2 repository write/read ──▶ 4.3 proto + conversion ──▶ 4.4 instrument 4 call sites
                                                          │
                                                          ▼
                                                    4.5 frontend RespawnHistorySection
      │
      ▼
Phase 5 (feature registry + make registry-generate — depends on all of 1–4 being complete)
```
Phases 1, 2, and 3 have no dependencies on each other and can be implemented in any order (or in parallel by separate workers). Phase 4's five epics are strictly sequential (schema → repo → proto → call sites → frontend) since each depends on the previous layer existing. Phase 5 is last.

---

## Phase 0: Pre-flight Verification

### Epic 0.1: Confirm BUG-053 `end_reason` plumbing state before starting
**Goal**: De-risk Phase 1 by confirming the ent-schema/DTO layer for `end_reason` is committed and stable, not mid-flight in someone else's uncommitted working tree (pitfalls.md §2's flagged risk).

#### Story 0.1.1: Verify git state of the two files pitfalls.md flagged
**As a** planner/implementer, **I want** to confirm `end_reason`'s backend plumbing is fully committed, **so that** Phase 1's proto/frontend work isn't built on a moving target.
**Acceptance Criteria**:
- Confirmed via `git status --short` and `git diff` on `session/ent/schema/item_session.go` and `server/services/backlog_service_triage.go` that neither file has uncommitted changes touching `end_reason`.
  - *Given* the repo working tree at `/home/tstapler/Programming/stapler-squad`, *When* running `git status --short session/ent/schema/item_session.go server/services/backlog_service_triage.go`, *Then* the output is empty (no uncommitted changes) — confirmed during planning research (2026-08-01): both commands returned empty, and `end_reason` is present in `session/ent/schema/item_session.go` as of commit `0ac676001`, already merged.
**Files**: none changed — verification only.

##### Task 0.1.1a: Re-run the git verification at implementation time (~2 min)
- Run `git status --short session/ent/schema/item_session.go server/services/backlog_service_triage.go` and `git log -1 --oneline -- session/ent/schema/item_session.go`.
- If either shows uncommitted/newer changes than confirmed here, pause Phase 1 and re-read the current field/behavior before proceeding — do not assume this plan's field names still match.
- Files: none (read-only verification).

---

## Phase 1: `end_reason` — proto, backend, frontend

### Epic 1.1: Thread `ItemSession.end_reason` onto the wire

#### Story 1.1.1: Add `end_reason` to the `ItemSession` proto message
**As a** frontend developer, **I want** `end_reason` available on the `ItemSession` proto message, **so that** `SessionsSection.tsx` can render it.
**Acceptance Criteria**:
- `ItemSession` message in `proto/session/v1/backlog.proto` has a new `string end_reason = 18;` field, and `make proto-gen` regenerates both Go (`session/gen/session/v1/backlog.pb.go`) and TS (`web-app/src/gen/session/v1/backlog_pb.ts`) bindings without error.
  - *Given* `proto/session/v1/backlog.proto`'s `ItemSession` message (fields 1–17, ending at `pipeline_mode_snapshot_hash = 17`), *When* `end_reason = 18` is added and `make proto-gen` runs, *Then* `sessionv1.ItemSession` (Go) and `ItemSession` (TS, from `@/gen/session/v1/backlog_pb`) both expose an `EndReason`/`endReason` string field.
**Files**: `proto/session/v1/backlog.proto`

##### Task 1.1.1a: Add the proto field (~2 min)
- In `proto/session/v1/backlog.proto`, add `string end_reason = 18;` inside `message ItemSession { ... }` (after `pipeline_mode_snapshot_hash = 17;`).
- Files: `proto/session/v1/backlog.proto`

##### Task 1.1.1b: Regenerate bindings (~3 min)
- Run `make proto-gen` from the repo root.
- Verify `session/gen/session/v1/backlog.pb.go` and `web-app/src/gen/session/v1/backlog_pb.ts` both now contain `EndReason`/`endReason`.
- Files: `session/gen/session/v1/backlog.pb.go`, `web-app/src/gen/session/v1/backlog_pb.ts` (generated, do not hand-edit)

#### Story 1.1.2: Populate `end_reason` in the Go→proto conversion
**As a** frontend developer, **I want** the `end_reason` field actually populated on every `ItemSession` the backend returns, **so that** the proto field added in 1.1.1 isn't silently empty.
**Acceptance Criteria**:
- `itemSessionToProto` (`server/services/backlog_service.go:472`) sets `EndReason: is.EndReason` on the returned `*sessionv1.ItemSession`.
  - *Given* an `ItemSessionSummary` with `EndReason: "process_error"` (as returned by `session.Storage.ListItemSessions`/`GetBacklogItem` for a headless triage call that failed), *When* `itemSessionToProto` converts it, *Then* the resulting `*sessionv1.ItemSession.EndReason` equals `"process_error"`.
**Files**: `server/services/backlog_service.go`

##### Task 1.1.2a: Populate the field (~2 min)
- In `server/services/backlog_service.go`, inside `itemSessionToProto` (starts line 472), add `EndReason: is.EndReason,` to the `p := &sessionv1.ItemSession{...}` struct literal (alongside `SessionRole`, `CreatedAt`, etc.).
- Files: `server/services/backlog_service.go`

### Epic 1.2: Surface `end_reason` in `SessionsSection.tsx`

#### Story 1.2.1: Thread `endReason` through the frontend type + mapper
**As a** frontend developer, **I want** `LinkedSession.endReason` available in React state, **so that** `SessionsSection.tsx` can render a chip from it.
**Acceptance Criteria**:
- `LinkedSession` interface gains an `endReason?: string` field, and `mapItemSession` (`web-app/src/lib/hooks/useBacklogService.ts:277`) populates it from `s.endReason`.
  - *Given* a proto `ItemSessionProto` with `endReason: "timeout"`, *When* `mapItemSession` converts it, *Then* the resulting `LinkedSession.endReason` equals `"timeout"`.
**Files**: `web-app/src/lib/hooks/useBacklogService.ts`

##### Task 1.2.1a: Add the field + mapping (~3 min)
- In `web-app/src/lib/hooks/useBacklogService.ts`, add `endReason?: string;` to the `LinkedSession` interface (near line 62, alongside `role`/`startedAt`).
- In `mapItemSession` (line 277), add `endReason: s.endReason || undefined,` to the `session: LinkedSession = {...}` literal.
- Files: `web-app/src/lib/hooks/useBacklogService.ts`

#### Story 1.2.2: Build `formatEndReason` + severity mapping
**As a** user, **I want** an end_reason chip that reads like a sentence and carries the right severity color, **so that** I can tell "clean" from "errored" at a glance, matching the codebase's established icon/text/aria-label triad.
**Acceptance Criteria**:
- `formatEndReason(reason: string)` returns a full-sentence label for each of `shutdown`/`timeout`/`process_error`/`claude_not_found`/`other`, and a severity of `"none" | "warning" | "error"` — `""`/`shutdown` → `"none"` (no badge rendered), `timeout`/`process_error` → `"warning"`, `claude_not_found`/`other` → `"error"`.
  - *Given* `end_reason = "process_error"`, *When* `formatEndReason("process_error")` is called, *Then* it returns `{ label: "Headless call failed (process error)", severity: "warning" }`.
  - *Given* `end_reason = ""`, *When* `formatEndReason("")` is called, *Then* it returns `{ label: "", severity: "none" }` and the calling component renders no chip at all (per ux.md §4: empty string is the documented success case, not "unknown").
**Files**: `web-app/src/lib/backlog/formatEndReason.ts` (new), `web-app/src/lib/backlog/formatEndReason.css.ts` (new, if a standalone chip class is needed) — colocated with `formatPauseReason.ts`'s sibling location under `web-app/src/lib/sessions/` is also acceptable; place under `web-app/src/lib/backlog/` since `end_reason` is an `ItemSession` (backlog) concept, not a generic `Session` one (see Pattern Decisions).

##### Task 1.2.2a: Write `formatEndReason.ts` (~4 min)
- Create `web-app/src/lib/backlog/formatEndReason.ts` exporting `formatEndReason(reason: string): { label: string; severity: "none" | "warning" | "error" }`, mirroring `formatPauseReason.ts`'s switch-statement shape:
  - `""` / `"shutdown"` → `{ label: "", severity: "none" }`
  - `"timeout"` → `{ label: "Headless call timed out", severity: "warning" }`
  - `"process_error"` → `{ label: "Headless call failed (process error)", severity: "warning" }`
  - `"claude_not_found"` → `{ label: "Headless call failed — claude CLI not found", severity: "error" }`
  - `"other"` → `{ label: "Headless call failed (unclassified)", severity: "error" }`
  - default → `{ label: "", severity: "none" }`
- Files: `web-app/src/lib/backlog/formatEndReason.ts`

##### Task 1.2.2b: Add chip CSS classes reusing theme tokens (~3 min)
- Create `web-app/src/lib/backlog/formatEndReason.css.ts` exporting `chipWarning`/`chipError` `style()` classes, copying `stuckReason.css.ts`'s `chip`/`chipAbandonedReview`/`chipReworkCap` shape (`vars.color.warningBg`/`warningText`/`warning` and `errorBg`/`errorText`/`error` respectively).
- Files: `web-app/src/lib/backlog/formatEndReason.css.ts`

#### Story 1.2.3: Render the end_reason chip in `SessionsSection.tsx`
**As a** user viewing an item's Sessions section, **I want** to see, per session row, whether it ended cleanly, timed out, or errored, **so that** I don't have to open logs to know.
**Acceptance Criteria**:
- Each session row in `SessionsSection.tsx` that has a non-`"none"`-severity `endReason` renders a chip with `aria-hidden` icon + visible text + `aria-label` carrying the full sentence; rows with a clean end render no chip.
  - *Given* a `LinkedSession` with `endReason: "process_error"` rendered in `SessionsSection.tsx`'s session list, *When* the row renders, *Then* it shows a chip with visible text "Headless call failed (process error)" and `aria-label="Headless call failed (process error)"`, using `formatEndReason`'s `"warning"` severity class.
  - *Given* a `LinkedSession` with `endReason: ""` (or `undefined`), *When* the row renders, *Then* no end_reason chip appears for that row.
**Files**: `web-app/src/components/backlog/detail/SessionsSection.tsx`

##### Task 1.2.3a: Add the chip to each session row (~4 min)
- In `web-app/src/components/backlog/detail/SessionsSection.tsx`, import `formatEndReason` and the two chip classes from Task 1.2.2.
- In the per-session row render (where `linkedSessions`/`visible.map(...)` is iterated), add, immediately after the session-role/status text: `{formatEndReason(s.endReason ?? "").severity !== "none" && <span className={...} aria-label={label}><span aria-hidden="true">{icon}</span> {label}</span>}` (icon: reuse "⚠️" for warning, "⛔" for error, matching `stuckReason.ts`'s glyph vocabulary).
- Files: `web-app/src/components/backlog/detail/SessionsSection.tsx`

---

## Phase 2: `pause_reason` badge promotion on `SessionCard.tsx`

### Epic 2.1: Always-visible pause_reason badge

#### Story 2.1.1: Replace tooltip-only pause reason with an always-visible badge
**As a** user on a touch device, **I want** to see why a session is paused without hovering, **so that** the information isn't hidden behind a gesture mobile doesn't support.
**Acceptance Criteria**:
- The paused-session branch of `SessionCard.tsx` (currently `isPaused && session.pauseReason ? <Tooltip>...` at line 491) renders `formatPauseReason(session.pauseReason)`'s text as a visible sibling node next to the status chip, not only inside a `Tooltip`/`aria-label`.
  - *Given* a `Session` with `status: PAUSED` and `pauseReason: "auto:inactivity"`, *When* `SessionCard.tsx` renders on a touch device (no hover), *Then* the text "Paused automatically — no recent activity" is visible on-screen without any pointer interaction.
- The existing `Tooltip`/`aria-label` wrapping is retained (not removed) for desktop hover discoverability and screen-reader redundancy, matching the established triad (icon + visible text + `aria-label`).
  - *Given* the same session, *When* rendered on desktop, *Then* hovering the chip still shows the `Tooltip` with the same text (no regression), and the chip's `aria-label` still reads `"Session status: Paused — auto:inactivity"` or equivalent full-sentence text.
**Files**: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 2.1.1a: Add the visible text sibling (~4 min)
- In `web-app/src/components/sessions/SessionCard.tsx`, in the `isPaused && session.pauseReason` branch (lines 491–500), add a `<span>` sibling immediately after the status `<span>`, rendering `formatPauseReason(session.pauseReason)` as visible text, styled with the existing pill/badge class used elsewhere in this file (check `SessionCard.css.ts`'s `badges`/pill token at line 130 first — reuse it, don't invent a new class per `.claude/rules/css-architecture.md`).
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 2.1.1b: Verify no layout regression on mobile width (~3 min)
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="SessionCard"` to confirm existing snapshot/behavior tests still pass; visually spot-check via the manual-test-instance pattern (`.claude/CLAUDE.md`'s "Manual/interactive testing" section) at a narrow viewport if a visual regression is plausible.
- Files: none changed (verification task).

---

## Phase 3: `BlockerChip` remediation-attempt surfacing (board + detail)

### Epic 3.1: Extend `BlockerChip` with a respawn/remediation-count suffix

#### Story 3.1.1: Render `×N` + next-retry hint from already-available `StuckBacklogItem` fields
**As a** user scanning the board or an item's detail Lifecycle Summary, **I want** to see how many times an item has been auto-remediated and whether it's still retrying or parked, **so that** I can distinguish "actively self-healing" from "truly stuck" without opening `/unfinished`.
**Acceptance Criteria**:
- `BlockerChip` renders a `×N` suffix span when `item.remediationAttempts > 0`, with `aria-label` spelling out the count in words (e.g. `"Respawned 3 times"`), per ux.md §3's explicit accessibility requirement.
  - *Given* a `StuckBacklogItem` with `reason: STALE_WORK`, `remediationAttempts: 3`, rendered via `<BlockerChip variant="compact" item={stuckItem} />` on `BacklogItemCard.tsx`, *When* the chip renders, *Then* it shows the existing icon+label plus a `×3` suffix span with `aria-label="Respawned 3 times"`.
- When `remediationAttempts >= 5` (parked, matching `StuckItem.tsx`'s `MAX_REMEDIATION_ATTEMPTS` constant), the chip additionally communicates "parked" — reuse the wording `StuckItem.tsx` already uses for its disabled "Retry now" button state, rather than inventing new copy.
  - *Given* a `StuckBacklogItem` with `remediationAttempts: 5`, *When* the chip renders (either variant), *Then* its `aria-label` includes "remediation attempts exhausted" (or the exact `StuckItem.tsx` wording), and the visible label reflects parked state rather than "still retrying".
- When `remediationAttempts === 0`, no `×N` suffix renders (unchanged from today) — avoids visual noise for the common "never remediated" case, matching ux.md §4's "don't manufacture a label for nothing" guidance.
  - *Given* a `StuckBacklogItem` with `remediationAttempts: 0`, *When* the chip renders, *Then* no `×N` suffix appears, and existing snapshot tests for `BlockerChip` (pre-existing `remediationAttempts: 0` fixtures) remain unchanged.
**Files**: `web-app/src/components/backlog/BlockerChip.tsx`, `web-app/src/components/backlog/BlockerChip.css.ts`

##### Task 3.1.1a: Add the `×N` suffix span (~4 min)
- In `web-app/src/components/backlog/BlockerChip.tsx`, after the existing `variant === "full" && <span className={styles.duration}>...</span>` block, add: `{item.remediationAttempts > 0 && <span className={styles.remediationCount} aria-label={\`Respawned ${item.remediationAttempts} time${item.remediationAttempts === 1 ? "" : "s"}${item.remediationAttempts >= 5 ? " — remediation attempts exhausted" : ""}\`} data-testid="blocker-chip-remediation-count">×{item.remediationAttempts}</span>}`.
- Files: `web-app/src/components/backlog/BlockerChip.tsx`

##### Task 3.1.1b: Add the `.remediationCount` CSS class (~2 min)
- In `web-app/src/components/backlog/BlockerChip.css.ts`, add a `remediationCount` `style()` next to the existing `duration` class — same visual weight (small, muted text), no new color token (inherits the parent chip's severity color).
- Files: `web-app/src/components/backlog/BlockerChip.css.ts`

##### Task 3.1.1c: Update existing `BlockerChip` tests + snapshots (~4 min)
- Update `web-app/src/components/backlog/BlockerChip.test.tsx` (or nearest existing test file) with 3 new cases: `remediationAttempts: 0` → no suffix; `remediationAttempts: 3` → `×3` + correct `aria-label`; `remediationAttempts: 5` → parked wording present.
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="BlockerChip"`.
- Files: `web-app/src/components/backlog/BlockerChip.test.tsx` (or existing equivalent — confirm exact filename first via `Glob`)

---

## Phase 4: `RespawnEvent` audit trail

### Epic 4.1: ent schema

#### Story 4.1.1: New `RespawnEvent` ent schema
**As a** backend developer, **I want** a `RespawnEvent` ent entity, **so that** respawn/remediation attempts can be durably recorded.
**Acceptance Criteria**:
- `session/ent/schema/respawn_event.go` defines `id` (UUID), `item_id` (UUID, plain field), `reason` (string), `triggering_session_uuid` (string, optional/nillable), `resulting_session_uuid` (string, optional/nillable), `created_at` (time, immutable, default now), an edge `From("item", BacklogItem.Type).Ref("respawn_events").Field("item_id").Unique().Required()`, and an index on `(item_id, created_at)`.
  - *Given* the new schema file, *When* `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` runs, *Then* `session/ent/respawnevent/` and `session/ent/client.go`'s `RespawnEvent` client are generated without error, and `go build ./...` succeeds.
**Files**: `session/ent/schema/respawn_event.go` (new), `session/ent/schema/backlog_item.go`

##### Task 4.1.1a: Write the `RespawnEvent` schema (~4 min)
- Create `session/ent/schema/respawn_event.go`, copying `session/ent/schema/backlog_status_event.go`'s structure: `field.UUID("id", ...).Default(uuid.New)`, `field.UUID("item_id", uuid.UUID{})`, `field.String("reason")`, `field.String("triggering_session_uuid").Optional().Nillable()`, `field.String("resulting_session_uuid").Optional().Nillable()`, `field.Time("created_at").Default(time.Now).Immutable()`. `Edges()`: `edge.From("item", BacklogItem.Type).Ref("respawn_events").Field("item_id").Unique().Required()`. `Indexes()`: `index.Fields("item_id", "created_at")`.
- Files: `session/ent/schema/respawn_event.go`

##### Task 4.1.1b: Add the `respawn_events` edge to `BacklogItem` (~2 min)
- In `session/ent/schema/backlog_item.go`'s `Edges()` (line ~123), add `edge.To("respawn_events", RespawnEvent.Type).Annotations(entsql.OnDelete(entsql.Cascade)),` alongside the existing `status_events`/`progress_notes` edges (line ~127–132) — cascade delete matches those two exactly (respawn-event rows are meaningless once their parent item is deleted, and features.md confirmed this is the only case session-uuid references would ever dangle, which cascade-delete also removes).
- Files: `session/ent/schema/backlog_item.go`

##### Task 4.1.1c: Regenerate ent and verify build (~3 min)
- Run `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` (per `.claude/rules/ent-schema-generation.md` — NOT the bare command).
- Run `go build ./...`.
- Commit all `session/ent/` generated changes together with the schema files (per CLAUDE.md's ent workflow note).
- Files: `session/ent/**` (generated, do not hand-edit beyond what `ent generate` produces)

### Epic 4.2: Repository write/read path

#### Story 4.2.1: `RespawnEventData` DTO + `CreateRespawnEvent` write path
**As a** backend developer, **I want** a `Storage.CreateRespawnEvent` method, **so that** `server/services/backlog_service_triage.go` (a different package than the ent repository) can append a respawn-event row.
**Acceptance Criteria**:
- `session/repository.go` defines `RespawnEventData{ ID, Reason, TriggeringSessionUUID, ResultingSessionUUID string; CreatedAt time.Time }` (mirroring `BacklogStatusEventData`'s shape).
- `session/ent_repository_backlog.go` defines `respawnEventToData(*ent.RespawnEvent) RespawnEventData` and `func (r *EntRepository) CreateRespawnEvent(ctx context.Context, itemID, reason, triggeringSessionUUID, resultingSessionUUID string) error`, which parses `itemID`, builds a `Create()` call (setting `triggering_session_uuid`/`resulting_session_uuid` only when non-empty, matching `recordStatusEvent`'s `note` pattern), and on `Save` failure logs via `log.ErrorLog.Printf` and returns `nil` (best-effort — see Pattern Decisions).
- `session/storage.go` defines `func (s *Storage) CreateRespawnEvent(ctx context.Context, itemID, reason, triggeringSessionUUID, resultingSessionUUID string) error`, forwarding to the `*EntRepository` type-assertion pattern used by `UpdateItemSessionEnded`/`ListItemSessions` (lines 990, 1108).
  - *Given* an item with UUID `9264efe7-b4c2-455a-9e2a-ab0196a63ecd` and no prior respawn events, *When* `storage.CreateRespawnEvent(ctx, "9264efe7-b4c2-455a-9e2a-ab0196a63ecd", "stale_work_remediation", "abc-triggering-uuid", "def-resulting-uuid")` is called, *Then* a new row exists in the `respawn_events` table with `item_id = 9264efe7-...`, `reason = "stale_work_remediation"`, both session UUIDs set, and `created_at` populated.
  - *Given* the DB is temporarily unavailable, *When* `CreateRespawnEvent` is called, *Then* it logs an error and returns `nil` (does not propagate an error to the caller — matches `recordStatusEvent`'s "never block the parent operation" contract).
**Files**: `session/repository.go`, `session/ent_repository_backlog.go`, `session/storage.go`

##### Task 4.2.1a: Add `RespawnEventData` DTO (~2 min)
- In `session/repository.go`, add `type RespawnEventData struct { ID, Reason, TriggeringSessionUUID, ResultingSessionUUID string; CreatedAt time.Time }` near `BacklogStatusEventData` (line ~312).
- Files: `session/repository.go`

##### Task 4.2.1b: Add `respawnEventToData` converter (~2 min)
- In `session/ent_repository_backlog.go`, add `func respawnEventToData(e *ent.RespawnEvent) RespawnEventData { return RespawnEventData{ ID: e.ID.String(), Reason: e.Reason, TriggeringSessionUUID: derefOrEmpty(e.TriggeringSessionUuid), ResultingSessionUUID: derefOrEmpty(e.ResultingSessionUuid), CreatedAt: e.CreatedAt } }` near `backlogStatusEventToData` (line ~133) — check the generated ent field accessor name (`TriggeringSessionUuid` vs `TriggeringSessionUUID`) after Task 4.1.1c and use whatever `ent generate` actually produced; add a small local `derefOrEmpty(*string) string` helper if one doesn't already exist in this file.
- Files: `session/ent_repository_backlog.go`

##### Task 4.2.1c: Add `EntRepository.CreateRespawnEvent` (~4 min)
- In `session/ent_repository_backlog.go`, add `func (r *EntRepository) CreateRespawnEvent(ctx context.Context, itemID, reason, triggeringSessionUUID, resultingSessionUUID string) error`, modeled on `recordStatusEvent` (line 45): parse `itemID` via `uuid.Parse`, build `r.client.RespawnEvent.Create().SetItemID(parsedID).SetReason(reason)`, conditionally `.SetTriggeringSessionUuid(...)`/`.SetResultingSessionUuid(...)` when non-empty, `.Save(ctx)`; on error, `log.ErrorLog.Printf(...)` and return `nil`; on `uuid.Parse` failure, return the error (a malformed itemID is a caller bug, unlike a transient DB failure).
- Files: `session/ent_repository_backlog.go`

##### Task 4.2.1d: Add `Storage.CreateRespawnEvent` forwarding method (~2 min)
- In `session/storage.go`, add `func (s *Storage) CreateRespawnEvent(ctx context.Context, itemID, reason, triggeringSessionUUID, resultingSessionUUID string) error`, following the `UpdateItemSessionEnded` pattern (line 990): type-assert `s.repo.(*EntRepository)`, return a descriptive error if the assertion fails, else forward the call.
- Files: `session/storage.go`

#### Story 4.2.2: Eager-load `respawn_events` in `GetBacklogItem`, capped to the 50 most recent
**As a** backend developer, **I want** `GetBacklogItem` to return an item's respawn history, **so that** the frontend can render it without a second RPC.
**Acceptance Criteria**:
- `GetBacklogItem` (`session/ent_repository_backlog.go:311`) adds `.WithRespawnEvents(func(q *ent.RespawnEventQuery) { q.Order(ent.Desc(respawnevent.FieldCreatedAt)).Limit(50) })` to its query chain, and `backlogItemToData` reverses the resulting slice to ascending order before assigning `data.RespawnEvents` (matching `StatusEvents`'/`ProgressNotes`' ascending convention, per pitfalls.md §4's read-path-must-be-bounded requirement).
  - *Given* a backlog item with 75 `RespawnEvent` rows spanning several days, *When* `GetBacklogItem` is called, *Then* the returned `BacklogItemData.RespawnEvents` contains exactly the 50 most recent rows, in ascending `created_at` order (oldest of the 50 first, newest last).
  - *Given* a backlog item with 0 `RespawnEvent` rows, *When* `GetBacklogItem` is called, *Then* `BacklogItemData.RespawnEvents` is an empty (not nil-panicking) slice.
**Files**: `session/ent_repository_backlog.go`, `session/repository.go`

##### Task 4.2.2a: Add `RespawnEvents` field to `BacklogItemData` (~1 min)
- In `session/repository.go`, add `RespawnEvents []RespawnEventData` to the `BacklogItemData` struct (line ~346, near where `ItemSessions`/eager-loaded slices live — check existing struct for a comparable field to place it next to).
- Files: `session/repository.go`

##### Task 4.2.2b: Eager-load in `GetBacklogItem` + propagate in `backlogItemToData` (~4 min)
- In `session/ent_repository_backlog.go`, add `.WithRespawnEvents(func(q *ent.RespawnEventQuery) { q.Order(ent.Desc(respawnevent.FieldCreatedAt)).Limit(50) })` to the `GetBacklogItem` query chain (line ~317–326, alongside `WithStatusEvents`/`WithProgressNotes`).
- In `backlogItemToData` (line ~190–241), add a propagation block after the existing `ProgressNotes` block (line ~217–223): if `item.Edges.RespawnEvents != nil`, build `data.RespawnEvents` by mapping + reversing (oldest-first) via `respawnEventToData`.
- Add `"github.com/tstapler/stapler-squad/session/ent/respawnevent"` to this file's import block.
- Files: `session/ent_repository_backlog.go`

### Epic 4.3: Proto + conversion

#### Story 4.3.1: `RespawnEvent` proto message + `BacklogItem.respawn_events` field
**As a** frontend developer, **I want** `RespawnEvent` on the wire, **so that** `RespawnHistorySection.tsx` (Phase 4.5) has data to render.
**Acceptance Criteria**:
- `proto/session/v1/backlog.proto` gains `message RespawnEvent { string id = 1; string reason = 2; string triggering_session_uuid = 3; string resulting_session_uuid = 4; google.protobuf.Timestamp created_at = 5; }` and `BacklogItem` gains `repeated RespawnEvent respawn_events = 30;` (next available field number after `category = 29`).
  - *Given* the updated proto and a `make proto-gen` run, *When* the Go/TS bindings regenerate, *Then* `sessionv1.BacklogItem.RespawnEvents` (Go) and `BacklogItem.respawnEvents` (TS) both exist and are `RespawnEvent[]`-typed.
**Files**: `proto/session/v1/backlog.proto`

##### Task 4.3.1a: Add the proto message + field (~3 min)
- In `proto/session/v1/backlog.proto`, add the `RespawnEvent` message definition near `BacklogStatusEvent`/`BacklogProgressNote` (after line ~98), and add `repeated RespawnEvent respawn_events = 30;` to `BacklogItem` (after `optional string category = 29;`, line ~150).
- Files: `proto/session/v1/backlog.proto`

##### Task 4.3.1b: Regenerate bindings (~3 min)
- Run `make proto-gen`.
- Verify `session/gen/session/v1/backlog.pb.go` and `web-app/src/gen/session/v1/backlog_pb.ts` contain the new `RespawnEvent` type and `BacklogItem.RespawnEvents`/`respawnEvents` field.
- Files: `session/gen/session/v1/backlog.pb.go`, `web-app/src/gen/session/v1/backlog_pb.ts` (generated)

#### Story 4.3.2: Populate `respawn_events` in `backlogItemToProto`
**As a** frontend developer, **I want** `GetBacklogItem`'s response to actually carry the respawn events, **so that** the proto field isn't silently empty.
**Acceptance Criteria**:
- `backlogItemToProto` (`server/services/backlog_service.go:628`) maps `item.RespawnEvents` to `p.RespawnEvents`, converting each `RespawnEventData` to `*sessionv1.RespawnEvent` (including `timestamppb.New(e.CreatedAt)`).
  - *Given* a `BacklogItemData` with 2 `RespawnEvents` (one with both session UUIDs set, one with only `TriggeringSessionUUID` set — a respawn attempt that hit the concurrency cap and queued instead of spawning), *When* `backlogItemToProto` converts it, *Then* the resulting `*sessionv1.BacklogItem.RespawnEvents` has 2 entries, the second with `ResultingSessionUuid == ""`.
**Files**: `server/services/backlog_service.go`

##### Task 4.3.2a: Add the conversion (~4 min)
- In `server/services/backlog_service.go`, in `backlogItemToProto` (starts line 628), after the existing progress-notes/status-events mapping block, add: iterate `item.RespawnEvents`, build `*sessionv1.RespawnEvent{ Id: e.ID, Reason: e.Reason, TriggeringSessionUuid: e.TriggeringSessionUUID, ResultingSessionUuid: e.ResultingSessionUUID, CreatedAt: timestamppb.New(e.CreatedAt) }`, assign to `p.RespawnEvents` only if non-empty (matching the `if len(item.ItemSessions) > 0` guard style already used at line 613).
- Files: `server/services/backlog_service.go`

### Epic 4.4: Instrument the 4 respawn call sites

#### Story 4.4.1: `AutoRespawnAutonomousWork` + `RemediateStaleWorkSession`
**As a** user, **I want** every autonomous-turn respawn and every stale-work remediation recorded, **so that** I can see them in the item's respawn history.
**Acceptance Criteria**:
- A new unexported `autoRespawnAutonomousWork(ctx, itemID, reason, triggeringSessionUUID string) error` in `backlog_service_triage.go` contains `AutoRespawnAutonomousWork`'s existing body, captures the spawned session's UUID from `SpawnSessionFromItemResponse.SessionUuid` (empty if `Queued == true`), and calls `s.storage.CreateRespawnEvent(ctx, itemID, reason, triggeringSessionUUID, resultingUUID)` after a successful (non-error) spawn attempt — including the queued case, with `resultingUUID == ""`.
- The public `AutoRespawnAutonomousWork(ctx, itemID)` becomes a thin wrapper calling `s.autoRespawnAutonomousWork(ctx, itemID, session.RespawnReasonAutonomousTurn, "")` (preserving its existing exported signature for interface conformance).
- `RemediateStaleWorkSession` calls `s.autoRespawnAutonomousWork(ctx, itemID, session.RespawnReasonStaleWork, active.SessionUUID)` directly (bypassing the public wrapper) at both of its respawn-triggering points (the `active == nil` early-return at line 1478, and the normal path at line 1499).
  - *Given* an in-progress item with an active stale work session `SessionUUID: "stale-abc"`, *When* `RemediateStaleWorkSession` runs its normal path (kills the pane, ends the session, respawns) and the respawn succeeds with a new session UUID `"fresh-def"`, *Then* a `RespawnEvent` row exists with `reason: "stale_work_remediation"`, `triggering_session_uuid: "stale-abc"`, `resulting_session_uuid: "fresh-def"`.
  - *Given* an in-progress item whose autonomous respawn hits the concurrency cap and queues instead of spawning (`SpawnSessionFromItemResponse.Queued == true`), *When* `autoRespawnAutonomousWork` runs via the public wrapper, *Then* a `RespawnEvent` row still exists with `reason: "autonomous_turn_respawn"`, `resulting_session_uuid: ""`.
**Files**: `server/services/backlog_service_triage.go`, `session/backlog.go`

##### Task 4.4.1a: Add `RespawnReason` constants (~1 min)
- In `session/backlog.go`, near `SessionRoleWork = "work"` (line 50), add: `RespawnReasonAutonomousTurn = "autonomous_turn_respawn"`, `RespawnReasonStaleWork = "stale_work_remediation"`, `RespawnReasonReviewAbandoned = "review_respawn"`, `RespawnReasonTriageOrphaned = "triage_respawn"`.
- Files: `session/backlog.go`

##### Task 4.4.1b: Refactor `AutoRespawnAutonomousWork` into public wrapper + internal helper (~5 min)
- In `server/services/backlog_service_triage.go`, rename the existing `AutoRespawnAutonomousWork` body to a new unexported `func (s *BacklogService) autoRespawnAutonomousWork(ctx context.Context, itemID, reason, triggeringSessionUUID string) error`.
- At the `spawnResp, spawnErr := s.SpawnSessionFromItem(...)` call (line 1412), capture `spawnResp` (currently discarded); after the `spawnErr != nil` check, add: `resultingUUID := ""; if spawnResp != nil && spawnResp.Msg != nil { resultingUUID = spawnResp.Msg.SessionUuid }` then `s.storage.CreateRespawnEvent(ctx, itemID, reason, triggeringSessionUUID, resultingUUID)`.
- Add back `func (s *BacklogService) AutoRespawnAutonomousWork(ctx context.Context, itemID string) error { return s.autoRespawnAutonomousWork(ctx, itemID, session.RespawnReasonAutonomousTurn, "") }`.
- Files: `server/services/backlog_service_triage.go`

##### Task 4.4.1c: Wire `RemediateStaleWorkSession`'s two call sites (~3 min)
- In `server/services/backlog_service_triage.go`, change the line-1478 `return s.AutoRespawnAutonomousWork(ctx, itemID)` (the `active == nil` early return) and the line-1499 `return s.AutoRespawnAutonomousWork(ctx, itemID)` (normal path, after `active` was captured) to `return s.autoRespawnAutonomousWork(ctx, itemID, session.RespawnReasonStaleWork, ...)`, passing `""` for the triggering UUID at line 1478 (no `active` session exists there — it already ended) and `active.SessionUUID` at line 1499.
- Files: `server/services/backlog_service_triage.go`

#### Story 4.4.2: `AutoRespawnReview`
**As a** user, **I want** every abandoned-review re-review-trigger recorded, **so that** I can see it in the item's respawn history.
**Acceptance Criteria**:
- `AutoRespawnReview` (line 1645) captures the `TriggerReReviewResponse.ItemSession`'s session UUID (verify the exact field — `ItemSession.SessionUuid` per the proto message read during research) after a successful `s.TriggerReReview(...)` call, and calls `s.storage.CreateRespawnEvent(ctx, itemID, session.RespawnReasonReviewAbandoned, "", resultingUUID)`.
  - *Given* an item abandoned in review with no active session, *When* `AutoRespawnReview` successfully triggers a re-review producing a new review `ItemSession` with `SessionUuid: "review-xyz"`, *Then* a `RespawnEvent` row exists with `reason: "review_respawn"`, `triggering_session_uuid: ""`, `resulting_session_uuid: "review-xyz"`.
**Files**: `server/services/backlog_service_triage.go`

##### Task 4.4.2a: Verify `TriggerReReviewResponse.ItemSession` is populated on the success path used here (~2 min)
- Read `server/services/backlog_service_triage.go` around line 2510–2560 (the section already reviewed during planning) to confirm `TriggerReReview`'s success path always returns a non-nil `ItemSession` with a populated `SessionUuid`. If any success branch can return a nil/empty `ItemSession`, note it — the event write in Task 4.4.2b must tolerate that (empty `resulting_session_uuid`, not a crash).
- Files: none (verification only).

##### Task 4.4.2b: Wire the event write (~4 min)
- In `server/services/backlog_service_triage.go`, in `AutoRespawnReview` (line 1645), after the `s.TriggerReReview(...)` call succeeds (line 1695–1697), capture the response, extract `resultingUUID := reviewResp.Msg.ItemSession.GetSessionUuid()` (guarding nil), and call `s.storage.CreateRespawnEvent(ctx, itemID, session.RespawnReasonReviewAbandoned, "", resultingUUID)`.
- Files: `server/services/backlog_service_triage.go`

#### Story 4.4.3: `AutoRespawnTriage`
**As a** user, **I want** every orphaned-triage re-trigger recorded, **so that** I can see it in the item's respawn history.
**Acceptance Criteria**:
- `AutoRespawnTriage` (line 1717) captures `TriggerTriageResponse.ItemSession`'s session UUID after a successful `s.TriggerTriage(...)` call, and calls `s.storage.CreateRespawnEvent(ctx, itemID, session.RespawnReasonTriageOrphaned, "", resultingUUID)`.
  - *Given* an idea-status item whose most recent triage session orphaned, *When* `AutoRespawnTriage` successfully re-triggers triage producing a new triage `ItemSession` with `SessionUuid: "triage-123"`, *Then* a `RespawnEvent` row exists with `reason: "triage_respawn"`, `triggering_session_uuid: ""`, `resulting_session_uuid: "triage-123"`.
**Files**: `server/services/backlog_service_triage.go`

##### Task 4.4.3a: Verify `TriggerTriageResponse.ItemSession` shape (~2 min)
- Confirm `TriggerTriageResponse { ItemSession item_session = 1; }` (already confirmed in proto during planning) is populated on `TriggerTriage`'s relevant success path (read the handler body if not already covered by Task 4.4.2a's read).
- Files: none (verification only).

##### Task 4.4.3b: Wire the event write (~3 min)
- In `server/services/backlog_service_triage.go`, in `AutoRespawnTriage` (line 1717), after `s.TriggerTriage(...)` succeeds (line 1733–1735), capture the response, extract `resultingUUID := triageResp.Msg.ItemSession.GetSessionUuid()` (guarding nil), and call `s.storage.CreateRespawnEvent(ctx, itemID, session.RespawnReasonTriageOrphaned, "", resultingUUID)`.
- Files: `server/services/backlog_service_triage.go`

##### Task 4.4.3c: Go build + targeted unit tests for all 4 call sites (~5 min)
- Run `go build ./...` and `go test ./server/services/... ./session/...` (scoped, not the full `make ci`, for fast iteration — full `make ci` runs at the end per Phase 5).
- Files: none (verification only).

### Epic 4.5: Frontend `RespawnHistorySection`

#### Story 4.5.1: Thread `respawnEvents` through the frontend `BacklogItem` type + mapper
**As a** frontend developer, **I want** `BacklogItem.respawnEvents` available in React state, **so that** `RespawnHistorySection.tsx` has data.
**Acceptance Criteria**:
- `web-app/src/lib/hooks/useBacklogService.ts` defines a `RespawnEvent` frontend interface (`id`, `reason`, `triggeringSessionUuid`, `resultingSessionUuid`, `createdAt`) and adds `respawnEvents: RespawnEvent[]` to the `BacklogItem` interface (near `statusEvents`/`progressNotes`, line ~122–124), populated in `mapBacklogItem` (line ~381–449) via `(p.respawnEvents ?? []).map(mapRespawnEvent)`, mirroring `mapStatusEvent`/`mapProgressNote`.
  - *Given* a proto `BacklogItemProto` with 2 `respawnEvents`, *When* `mapBacklogItem` converts it, *Then* the resulting `BacklogItem.respawnEvents` has 2 entries with `createdAt` as an ISO string (matching `statusEvents`' date-mapping convention).
**Files**: `web-app/src/lib/hooks/useBacklogService.ts`

##### Task 4.5.1a: Add the type + mapper (~4 min)
- In `web-app/src/lib/hooks/useBacklogService.ts`, add `export interface RespawnEvent { id: string; reason: string; triggeringSessionUuid: string; resultingSessionUuid: string; createdAt: string; }` near `StatusEvent`'s definition.
- Add `respawnEvents: RespawnEvent[];` to the `BacklogItem` interface (line ~122–124 vicinity).
- Add `function mapRespawnEvent(e: RespawnEventProto): RespawnEvent { return { id: e.id, reason: e.reason, triggeringSessionUuid: e.triggeringSessionUuid, resultingSessionUuid: e.resultingSessionUuid, createdAt: e.createdAt ? new Date(Number(e.createdAt.seconds) * 1000).toISOString() : "" }; }` mirroring the existing `mapStatusEvent`/`mapProgressNote` pattern (find their exact shape first via `Grep` before writing, to match date-handling conventions precisely).
- In `mapBacklogItem` (line 381), add `respawnEvents: (p.respawnEvents ?? []).map(mapRespawnEvent),` alongside the `statusEvents`/`progressNotes` lines (line 448–449).
- Files: `web-app/src/lib/hooks/useBacklogService.ts`

#### Story 4.5.2: `RespawnHistorySection.tsx` component
**As a** user, **I want** a collapsible respawn-history timeline on the item detail panel, **so that** I can see every automated respawn attempt, its reason, and the sessions involved.
**Acceptance Criteria**:
- `RespawnHistorySection.tsx` renders a `CollapsibleSection` (collapsed by default, `sectionKey="respawn-history"`) containing a `role="list"` of `respawnEvents`, capped via `useShowMore(item.id, "respawn-history", item.respawnEvents, 8)`, each row showing the reason (human-readable, via a new `formatRespawnReason` mirroring `formatPauseReason`), timestamp (`formatDate`), and triggering/resulting session references as inert text (short session-UUID prefix, e.g. first 8 chars) — never a broken link, per ux.md §4's "session no longer exists" guidance (in practice, per features.md, this case can't occur since `ItemSession` rows only cascade-delete with the parent item, at which point the `RespawnEvent` rows are also gone — but the row rendering still treats an absent `resulting_session_uuid` as "queued, no session yet" text, not a crash).
- The section **always renders**, with an explicit "No automated respawns recorded for this item." empty state when `respawnEvents.length === 0` — following `WorkflowHistorySection`'s precedent (not `ProgressHistorySection`'s hide-when-empty), since "never respawned" is itself informative signal (features.md §1's explicit recommendation).
  - *Given* a `BacklogItem` with `respawnEvents: []`, *When* `RespawnHistorySection` renders, *Then* it shows the `CollapsibleSection` header plus the text "No automated respawns recorded for this item." — not an omitted section.
  - *Given* a `BacklogItem` with one `respawnEvent` `{ reason: "stale_work_remediation", triggeringSessionUuid: "stale-abc123", resultingSessionUuid: "fresh-def456", createdAt: "2026-07-30T12:00:00Z" }`, *When* `RespawnHistorySection` renders, *Then* the row shows "Stale work session remediated" (or equivalent human label), a formatted date, and both session references as plain (non-link) text.
  - *Given* a `BacklogItem` with a `respawnEvent` where `resultingSessionUuid === ""` (a queued/failed spawn attempt), *When* the row renders, *Then* it shows "(no session — spawn was queued or failed)" in place of a session reference, rather than an empty string or crash.
**Files**: `web-app/src/components/backlog/detail/RespawnHistorySection.tsx` (new), `web-app/src/components/backlog/detail/RespawnHistorySection.css.ts` (new), `web-app/src/lib/backlog/formatRespawnReason.ts` (new)

##### Task 4.5.2a: Write `formatRespawnReason.ts` (~3 min)
- Create `web-app/src/lib/backlog/formatRespawnReason.ts` exporting `formatRespawnReason(reason: string): string`, mapping `"autonomous_turn_respawn"` → `"Autonomous turn budget respawned"`, `"stale_work_remediation"` → `"Stale work session remediated"`, `"review_respawn"` → `"Abandoned review re-triggered"`, `"triage_respawn"` → `"Orphaned triage re-triggered"`, default → the raw reason string.
- Files: `web-app/src/lib/backlog/formatRespawnReason.ts`

##### Task 4.5.2b: Write `RespawnHistorySection.tsx` (~5 min)
- Create `web-app/src/components/backlog/detail/RespawnHistorySection.tsx`, copying `WorkflowHistorySection.tsx`'s structure verbatim (same imports: `CollapsibleSection`, `useShowMore`, `formatDate`), substituting `item.respawnEvents` for `item.statusEvents`, `SHOW_MORE_CAP = 8`, and the empty-state text specified above. Each row renders: reason (via `formatRespawnReason`), `formatDate(ev.createdAt)`, and a small "triggered by session `{first8(triggeringSessionUuid)}` → resulted in session `{first8(resultingSessionUuid)}`" line (omit the "triggered by" clause entirely when `triggeringSessionUuid === ""`; show "(no session — spawn was queued or failed)" when `resultingSessionUuid === ""`).
- Files: `web-app/src/components/backlog/detail/RespawnHistorySection.tsx`

##### Task 4.5.2c: Write `RespawnHistorySection.css.ts` (~2 min)
- Create `web-app/src/components/backlog/detail/RespawnHistorySection.css.ts`, copying `WorkflowHistorySection.css.ts`'s classes (`showMoreButton`, timeline row classes) — reuse `styles.section`/`styles.emptyText` from the shared `../BacklogItemDetail.css` import (same as `WorkflowHistorySection.tsx` does) rather than redefining them.
- Files: `web-app/src/components/backlog/detail/RespawnHistorySection.css.ts`

#### Story 4.5.3: Register `RespawnHistorySection` in `BacklogItemDetail.tsx`
**As a** user, **I want** the respawn history section to appear in the item detail panel, **so that** Story 4.5.2's component is actually reachable.
**Acceptance Criteria**:
- `BacklogItemDetail.tsx` imports `RespawnHistorySection`, adds a `respawnHistoryExpanded`/`setRespawnHistoryExpanded` pair via `useSectionExpandState(itemId, "respawn-history", false)` (mirroring `workflowExpanded` at line 320), registers `["respawn-history", respawnHistoryExpanded, setRespawnHistoryExpanded]` in the group-state array (line ~378–379), and renders `<RespawnHistorySection item={item} defaultExpanded={respawnHistoryExpanded} />` immediately after `<ProgressHistorySection .../>` (line 1242).
  - *Given* a backlog item detail panel rendered for any item, *When* the page loads, *Then* a "Respawn History" (or equivalent title) collapsible section appears below "Progress History", collapsed by default, and expanding/collapsing it persists across a remount (same `useSectionExpandState` localStorage behavior as the other sections).
**Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 4.5.3a: Wire the import, state, and render call (~4 min)
- In `web-app/src/components/backlog/BacklogItemDetail.tsx`: add `import { RespawnHistorySection } from "./detail/RespawnHistorySection";` (near line 48); add `const [respawnHistoryExpanded, setRespawnHistoryExpanded] = useSectionExpandState(itemId, "respawn-history", false);` (near line 321); add `["respawn-history", respawnHistoryExpanded, setRespawnHistoryExpanded],` to the group-state array (near line 379); add `<RespawnHistorySection item={item} defaultExpanded={respawnHistoryExpanded} />` after line 1242.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 4.5.3b: Frontend test run (~3 min)
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="BacklogItemDetail|RespawnHistorySection|BlockerChip|SessionCard|SessionsSection"`.
- Files: none (verification only).

---

## Phase 5: Feature registry + wrap-up

### Epic 5.1: Feature registry entries

#### Story 5.1.1: Register new/changed surfaces per `.claude/rules/feature-registry.md`
**As a** maintainer, **I want** the registry to reflect every new/changed feature from this project, **so that** `make registry-generate` shows no net-new coverage gap.
**Acceptance Criteria**:
- New per-feature JSON files exist for: `end_reason` backend field surfacing (backend), `pause_reason` badge promotion (frontend), `BlockerChip` remediation-count (frontend), `RespawnEvent` audit trail (backend — the `CreateRespawnEvent`/eager-load path), `RespawnHistorySection` (frontend).
- `make registry-generate` runs clean and `docs/registry/coverage-gaps.json`'s count does not grow relative to its pre-project baseline.
  - *Given* the repo state after all Phase 1–4 tasks are complete, *When* `make registry-generate` runs, *Then* it exits 0 and the diff to `docs/registry/coverage-gaps.json` shows no net-new untested-feature entries beyond what's explicitly justified in the PR description.
**Files**: `docs/registry/features/backend/*.json` (new, ~2 files), `docs/registry/features/frontend/*.json` (new, ~3 files)

##### Task 5.1.1a: Add backend registry entries (~3 min)
- Create `docs/registry/features/backend/backlog-end-reason.json` and `docs/registry/features/backend/backlog-respawn-event.json`, following the schema shown in `.claude/rules/feature-registry.md` (`id`, `type: "backend"`, `name`, `markerFound`, `tested`, `testIds`).
- Files: `docs/registry/features/backend/backlog-end-reason.json`, `docs/registry/features/backend/backlog-respawn-event.json`

##### Task 5.1.1b: Add frontend registry entries (~3 min)
- Create `docs/registry/features/frontend/session-pause-reason-badge.json`, `docs/registry/features/frontend/blocker-chip-remediation-count.json`, `docs/registry/features/frontend/respawn-history-section.json`, following the schema in `.claude/rules/feature-registry.md`.
- Files: `docs/registry/features/frontend/session-pause-reason-badge.json`, `docs/registry/features/frontend/blocker-chip-remediation-count.json`, `docs/registry/features/frontend/respawn-history-section.json`

##### Task 5.1.1c: Run `make registry-generate` and verify (~2 min)
- Run `make registry-generate` from repo root; review the diff to `docs/registry/backend-features.json`/`frontend-features.json`/`coverage-gaps.json`; commit all changed registry files together.
- Files: `docs/registry/backend-features.json`, `docs/registry/frontend-features.json`, `docs/registry/coverage-gaps.json` (generated)

### Epic 5.2: Final validation

#### Story 5.2.1: Full CI gate
**As a** maintainer, **I want** the definitive pre-push check to pass, **so that** the PR is mergeable.
**Acceptance Criteria**:
- `make ci` passes (build, generated protos, full Go + frontend test suite, lint).
  - *Given* all Phase 1–5 changes committed, *When* `make ci` runs, *Then* it exits 0.
**Files**: none (verification only).

##### Task 5.2.1a: Run `make ci` (~5 min, mostly wait time — run with `&` per benchmark convention if it's a long-running check)
- Run `make build && make test && make lint` (or `make ci` directly); fix any failures surfaced (expected to be none if each phase's own verification tasks passed).
- Files: none (verification only), or whatever files a lint/test failure requires touching.
