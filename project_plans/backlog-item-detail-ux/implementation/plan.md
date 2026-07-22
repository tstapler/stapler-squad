# Implementation Plan: backlog-item-detail-ux

**Feature**: Redesign `BacklogItemDetail.tsx`'s information architecture with progressive
disclosure, an always-visible lifecycle summary, a read-only diagnostic display for synthetic
(triage/blocked/manual-review) sessions, and consistent blocker indicators on board/list cards.
**Date**: 2026-07-21
**Status**: Ready for implementation
**ADRs**: ADR-027 (Radix Accordion for the shared Collapsible primitive)

---

## Step 0.5 — Alternatives Explored

| # | Approach | Strength | Weakness |
|---|---|---|---|
| A | **Incremental section-by-section extraction with a shared `Collapsible` primitive** (chosen) | Matches the existing extraction precedent already in `web-app/src/components/backlog/` (`GateVerdictBox.tsx`, `TriageReviewPanel.tsx` were both pulled out of what was once a monolithic detail view); each section can ship and be verified independently against the live daily-use tool without a long-lived parallel implementation. | More total files/PRs than a single rewrite; requires explicit discipline (Story 3.1's `itemId` remount fix) to avoid extraction merely relocating existing bugs rather than fixing them. |
| B | Full rewrite as a new component tree (e.g. build `BacklogItemDetailV2` in parallel, cut over at the end) | A clean slate sidesteps the `itemId` state-leak bug and the 6 duplication findings (D1–D6) entirely, with no legacy code to reconcile against. | Two parallel implementations of 1577 lines of business logic (12+ status-conditional branches, manual-review form, gate approval flow, session deletion) for the multi-week Large-appetite duration — high drift/missed-edge-case risk for a solo maintainer with no second reviewer, and no way to verify against production usage until the very end. |
| C | Tab-based reorganization using the already-installed `@radix-ui/react-tabs` instead of accordion-style collapsing | Zero new dependency (Tabs is already vendored and used elsewhere in this codebase). | Tabs enforce a single visible panel at a time, which directly conflicts with the "at-a-glance, without expanding anything" success metric — Sessions, Version Control, and Progress History are frequently relevant *simultaneously*, not mutually exclusive, and forcing them behind tab switches would make cross-referencing (e.g. "which session produced this PR") strictly harder than the status quo, not better. |

**Chosen: Approach A.** Given the Large appetite, solo maintainer, and existing `backlog/`
extraction precedent, incremental extraction with a shared primitive is the calibrated choice —
it reuses a pattern this codebase has already validated twice (`GateVerdictBox`,
`TriageReviewPanel`) and lets each epic below ship and be dogfooded independently.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| **BacklogItem** | The core domain entity (`web-app/src/lib/hooks/useBacklogService.ts:92`); has a `status: BacklogItemStatus` (`"idea" \| "refining" \| "ready" \| "queued" \| "in_progress" \| "review" \| "pr_pending" \| "done" \| "archived"`, see `useBacklogService.ts:22`). | Unchanged by this project. |
| **LinkedSession** | An `ItemSession` DB row surfaced to the frontend (`useBacklogService.ts:57`), one of `item.linkedSessions`. Has `role`, `sessionId`, `reviewVerdict?`, `triageResult?`. | Both **Real Sessions** and **Synthetic Sessions** are `LinkedSession`s — the distinction is derived, not a wire field. |
| **Real Session** | A `LinkedSession` backed by an actual `session.Instance`/tmux/PTY — a work session (`role === "work"`) or a genuine, non-headless review session. Openable today at `/?session=<id>` and unaffected by this project. | |
| **Synthetic Session** | A `LinkedSession` that is a DB-only row with **no backing `session.Instance`** — there is no terminal, no PTY, nothing to attach to. Always one of the three sub-kinds below. | The entire premise of Epic 4. |
| **Headless Diagnostic Session** | Synthetic session sub-kind with real structured output already on the wire: `sessionId` prefix `headless-triage-` or `headless-re-review-` (role `"triage"`, or role `"review"` for re-review), carrying `triageResult` or `reviewVerdict`. | Rendered via **Structured Diagnostic** (readOnly `TriageReviewPanel`/`GateVerdictBox`). |
| **Blocked Guardrail Session** | Synthetic session sub-kind for a pre-flight check that blocked review before any review ran: `sessionId` prefix `review-blocked-` or `diff-error-`, role `"review"`, verdict-only (`reviewVerdict.summary`, always `FAIL`). | Rendered via **Blocked-Before-Start Notice**, not a session viewer — no session ever executed. |
| **Manual Review Marker Session** | Synthetic session sub-kind for a user-submitted manual review override: `sessionId` prefix `manual-review-`, role `"review"`, verdict-only. | Same rendering treatment as Blocked Guardrail Session. |
| **Session Kind** | The closed classification `"work" \| "review" \| "headless_diagnostic" \| "blocked_guardrail" \| "manual_review_marker"` produced by `classifySessionKind()`. | New; see `web-app/src/lib/backlog/sessionKind.ts` (Epic 1). |
| **Lifecycle Stage** | One of 5 always-visible nodes in the **Stage Tracker**: Idea, Ready, In Progress, Review, Done. | `refining` folds into Idea; `queued` renders as a modifier badge on In Progress; `pr_pending` renders as a modifier badge on Review; `archived` renders as an overlay ribbon across the whole tracker, not a 6th node. |
| **Stage Tracker** | The compact horizontal stepper component bound strictly to `item.status`. | `web-app/src/components/backlog/detail/StageTracker.tsx` (Epic 2). |
| **Blocker Chip** | A derived (never stored) "waiting on X" indicator sourced from `useStuckBacklogItems()` / `StuckBacklogItem.reason`, reusing `stuckReason.ts`'s icon/label/duration formatting verbatim. Never rendered in a neutral/"OK" state — omitted entirely when the item isn't flagged stuck. | Shared component `web-app/src/components/backlog/BlockerChip.tsx` (Epic 1), consumed by both the detail view (full variant) and `BacklogItemCard.tsx` (compact variant, Epic 5). |
| **Liveness Line** | A single "Last activity Nm ago" line using `formatAgo()` (`stuckReason.ts`) against the item's most recent activity timestamp. | `web-app/src/components/backlog/detail/LivenessLine.tsx` (Epic 2). |
| **Lifecycle Summary** | The always-visible header region composing Stage Tracker + Blocker Chip + Liveness Line — the single authoritative status display, replacing the standalone status badge at `BacklogItemDetail.tsx:710-714` (D1). | `web-app/src/components/backlog/detail/LifecycleSummary.tsx` (Epic 2). |
| **Collapsible** | The shared progressive-disclosure primitive built on `@radix-ui/react-accordion` (ADR-027). | `web-app/src/components/ui/Collapsible.tsx` (Epic 1). |
| **Section Expand State** | Per-item, per-section boolean persisted via `localStorage`, keyed `backlog-detail-section-${itemId}-${sectionKey}`. | `useSectionExpandState()` hook (Epic 1). |
| **Current Work Session Selector** | The single memoized helper `getLatestWorkSession(item)` / `useCurrentWorkSession(item)` replacing 4 independent re-derivations (D3). | `web-app/src/lib/backlog/currentWorkSession.ts` (Epic 1). |
| **Session Diagnostic Panel** | The dispatcher component that routes a Synthetic Session to the correct read-only presentation based on its Session Kind — Structured Diagnostic for Headless Diagnostic Sessions, Blocked-Before-Start Notice for the other two sub-kinds. | `web-app/src/components/backlog/detail/SessionDiagnosticPanel.tsx` (Epic 4). |
| **Structured Diagnostic** | `TriageReviewPanel`/`GateVerdictBox` rendered with the new `readOnly` prop — action buttons hidden, summary/criteria/suggestions/tasks still rendered. | Not a log/terminal viewer — no raw transcript exists for these rows (Critical Reconciliation). |
| **Blocked-Before-Start Notice** | The plain explanatory text block rendering a Blocked Guardrail or Manual Review Marker session's `reviewVerdict.summary` verbatim (after the Story 4.1 security review), `role="status"`. | `web-app/src/components/backlog/detail/BlockedNotice.tsx` (Epic 4). |
| **StuckBacklogItem / StuckReason** | Pre-existing (already shipped) types from `web-app/src/gen/session/v1/backlog_pb` / `useStuckBacklogItems.ts`. | Read-only reuse; no backend changes. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall redesign strategy | Incremental section-by-section extraction (Approach A) | Step 0.5 analysis | (B) Full parallel rewrite; (C) Tab-based reorg via `@radix-ui/react-tabs` | (B) risks weeks of drift with no second reviewer; (C)'s single-visible-panel model conflicts with "several sections relevant simultaneously" (Sessions ↔ Version Control ↔ Progress History cross-referencing) |
| Collapsible primitive | `@radix-ui/react-accordion` wrapped as `Collapsible` | ADR-027 | Promoted `RecentFilesSection.tsx` `useState`+`aria-expanded` pattern | 8+ sections, several with nested interactive bodies (forms, delete buttons, nested diagnostic panels) — Radix's ARIA/keyboard-group correctness is the differentiator over a single-toggle pattern; `WorkflowsPanel.tsx`'s hand-rolled toggle is the concrete counter-example already in this codebase (no `aria-expanded`) |
| Section splitting | Extract Component (props down, callbacks up) | `GateVerdictBox.tsx`/`TriageReviewPanel.tsx` precedent | A shared React Context provider for detail-panel state | `.claude/rules/interface-pollution-checklist.md` explicitly discourages a context provider for a single-consumer-tree feature; narrow explicit props keep each new section independently testable |
| Current work session | Single memoized selector `useCurrentWorkSession(item)` | D3 finding | Status quo: 4 independent inline re-derivations | A single source of truth prevents the 4 call sites drifting out of sync as new logic is added; trivial to unit test in isolation |
| Read-only session viewer | `readOnly` prop on existing `TriageReviewPanel`/`GateVerdictBox` | Critical Reconciliation (architecture.md, pitfalls.md) | Adapt `VirtualLogList`/`logParser.ts` (`logs/` suite); adapt `XtermTerminal` in read-only mode | Confirmed: no raw transcript exists for synthetic sessions — the diagnostic content is already-fetched structured JSON (`TriageResult`/`ReviewVerdict`), not scrollback text. Building/adapting a log or terminal viewer would solve a problem that doesn't exist here. |
| Board card blocker indicator | Shared `BlockerChip` component, compact variant | ux.md §5, architecture.md §4 | Independent implementation duplicated in `BacklogItemCard.tsx` | Prevents the same drift `BacklogItemBadge.tsx`/`lib/backlog/status.ts` already avoided for status vocabulary — one source of truth for icon/label/duration |
| Stage tracker widget | Hand-rolled vanilla-extract component | build-vs-buy.md Piece (b) | Repurpose `@radix-ui/react-tabs` as a visual stepper | Tabs primitive implies "switch between mutually exclusive views," not "show sequential progress" — wrong semantics even if visually stretched to look like a stepper; no complex focus-management need Radix would add value for here |
| `pr_pending` / `queued` in the Stage Tracker | Modifier badges on existing nodes (Review, In Progress), not new stepper nodes | ux.md §5 recommendation, applied to this codebase's actual status set | Two additional stepper nodes (7 total) | ux.md's recommendation explicitly scoped to the 5-status backbone it analyzed; this codebase additionally has `queued`/`pr_pending` as real statuses (confirmed via `server/services/backlog_service_triage_test.go`, `BacklogItemDetail.tsx:922`) — treating them as modifiers keeps the tracker's node count fixed and matches how `queued` is already described in product language ("at capacity, will start automatically") rather than as a new lifecycle phase |

---

## Migration Plan

Omitted — confirmed no schema or backend data-model changes are required for this project (see
requirements.md's "RESEARCH FINDINGS" section: the diagnostic content already travels over the
wire in `ItemSession.review_verdict`/`triage_result`, `proto/session/v1/backlog.proto:77-78`).

## Observability Plan
- **Logs**: no new observability needed — pure UI/display feature. Standard request logging
  (already present for `GetBacklogItem`/`ListStuckBacklogItems`) is sufficient.
- **Metrics**: none.
- **Alerts**: no new alerts required.

## Risk Control
- **Feature flag**: not gated — single-user internal tool, per requirements.md's Risk Control
  section ("low risk, single-user internal tool").
- **Rollback procedure**: standard revert via PR close + revert commit, per epic (each epic is
  independently revertable since sections are extracted incrementally, not behind a single
  atomic cutover).
- **Staged rollout**: full rollout on merge, epic by epic, in the order below.

## Unresolved Questions

1. **Is `RunPreGateSecurityCheck`'s error string 100% safe to render verbatim in a now more-
   discoverable UI surface?** (`session/backlog_review.go:45`, consumed at
   `session/review_gate.go:220-236`.) Must be resolved by Story 4.1 (Task 4.1.1a) *before* Story
   4.3 (Blocked-Before-Start Notice) ships that summary string to the UI. If the review finds any
   path where a raw secret substring could leak into the formatted error, Task 4.1.1a's follow-up
   is to redact at the source (`session/backlog_review.go`) before this project's frontend work
   renders it, not to patch it only in the frontend.
2. **Does `/backlog`'s non-board list view (`BacklogItemBadge.tsx`) also need the compact
   `BlockerChip`, or is board-only sufficient?** requirements.md's Scope says "board/list card
   summaries (`/backlog`, `/backlog/board`)" (both), but `BacklogItemBadge.tsx` renders inline
   inside a narrower list row than `BacklogItemCard.tsx`'s board card. Story 5.2 makes this call
   explicit rather than silently skipping it — resolve before Story 5.2 starts by checking actual
   available width in the current `/backlog` list-row layout.
3. **Does the new e2e spec's synthetic-session fixture (Task 6.2.1e) need a new debug-seed
   endpoint**, analogous to `server/services/backlog_debug_seed_handler.go`'s existing
   `handleSeedQueued`, or can an existing e2e fixture item already carry a `headless-triage-*`
   row? Resolve at the start of Story 6.2 by checking `tests/e2e/pages/BacklogPage.ts` and any
   existing seed helpers before adding a new one.

---

## Dependency Visualization

```
Epic 1: Shared Primitives Foundation
  (Collapsible, currentWorkSession, sessionKind, BlockerChip)
        │
        ├──────────────┬──────────────┬───────────────┐
        ▼               ▼              ▼               ▼
Epic 2: Lifecycle    Epic 3: Section  Epic 5: Board   (sessionKind also
Summary Header       Decomposition +  Card             feeds Epic 4)
(needs BlockerChip,  Progressive      Consistency
 StageTracker)        Disclosure       (needs
        │             (needs           BlockerChip)
        │             Collapsible,
        │             currentWorkSession)
        │                   │
        │                   ▼
        │             Epic 4: Synthetic Session
        │             Diagnostic Display
        │             (needs sessionKind,
        │              SessionsSection from
        │              Epic 3 Story 3.4,
        │              readOnly TriageReviewPanel/
        │              GateVerdictBox)
        │                   │
        └─────────┬─────────┘
                   ▼
         Epic 6: Test Coverage
         (testid preservation, e2e spec, registry)
```

---

## Phase 1: Foundation

### Epic 1.1: Shared Primitives Foundation
**Goal**: Build the four reusable building blocks every later epic depends on — the Collapsible
primitive, the current-work-session selector, the session-kind classifier, and the shared
BlockerChip — so no later epic re-derives logic that already has a canonical home.

#### Story 1.1.1: Collapsible primitive
**As a** maintainer, **I want** one shared, ARIA-correct disclosure component, **so that** every
new collapsible section in the redesign gets keyboard/focus/ARIA correctness for free instead of
re-deriving it per section.

**Acceptance Criteria**:
- A `Collapsible` section renders a real `<button aria-expanded="true|false">` header, never a
  `<div onClick>`.
  - *Given* a `Collapsible` with `sectionKey="plan-artifacts"` and `defaultExpanded={false}`,
    *When* it first renders, *Then* its header button has `aria-expanded="false"` and its body
    content is not present in the DOM (not just visually hidden).
- Expand state persists per item and per section across a page reload.
  - *Given* the user expands the `sectionKey="version-control"` section for
    `itemId="itm_a1b2c3"`, *When* the page is reloaded, *Then* `localStorage` key
    `backlog-detail-section-itm_a1b2c3-version-control` is `"true"` and the section renders
    expanded on the next mount.
- Every header button meets the ≥44×44px touch target requirement.

**Files**: `web-app/package.json`, `web-app/src/components/ui/Collapsible.tsx`,
`web-app/src/components/ui/Collapsible.css.ts`, `web-app/src/components/ui/Collapsible.test.tsx`,
`web-app/src/lib/hooks/useSectionExpandState.ts`

##### Task 1.1.1a: Add `@radix-ui/react-accordion` dependency (~2 min)
- Run `cd web-app && npm install @radix-ui/react-accordion`.
- Files: `web-app/package.json`, `web-app/package-lock.json`

##### Task 1.1.1b: Create `Collapsible.css.ts` (~4 min)
- vanilla-extract styles targeting Radix's `data-state="open"|"closed"` attributes via
  `selectors`, per ADR-027; chevron rotation, header padding sized for ≥44px touch target, import
  tokens from `web-app/src/styles/theme.css.ts` (no hardcoded values).
- Files: `web-app/src/components/ui/Collapsible.css.ts`

##### Task 1.1.1c: Build `Collapsible.tsx` (~5 min)
- Wrap `@radix-ui/react-accordion`'s `Root`/`Item`/`Trigger`/`Content` into a single
  `CollapsibleSection({ sectionKey, title, defaultExpanded, children })` export; header button
  includes a chevron and reads `aria-expanded` from Radix's own state.
- Files: `web-app/src/components/ui/Collapsible.tsx`

##### Task 1.1.1d: Add `useSectionExpandState(itemId, sectionKey, defaultExpanded)` (~4 min)
- `localStorage`-backed hook, key `backlog-detail-section-${itemId}-${sectionKey}`; wraps
  reads/writes in `try/catch` (matches `RecentFilesSection.tsx`'s existing defensive pattern for
  `localStorage` access).
- Files: `web-app/src/lib/hooks/useSectionExpandState.ts`

##### Task 1.1.1e: Write `Collapsible.test.tsx` (~5 min)
- RTL tests: initial `aria-expanded` state, toggling on click, collapsed content absent from DOM
  (`queryByText` returns `null`, not just non-visible), touch target size via computed style.
- Files: `web-app/src/components/ui/Collapsible.test.tsx`

---

#### Story 1.1.2: Current work session selector
**As a** maintainer, **I want** a single memoized "current work session" helper, **so that** the
4 places in `BacklogItemDetail.tsx` that independently recompute it (D3) can never drift out of
sync.

**Acceptance Criteria**:
- `getLatestWorkSession(item)` returns the most recent `role === "work"` session, matching the
  existing inline logic exactly.
  - *Given* a `BacklogItem` whose `linkedSessions` is `[{sessionId: "s1", role: "work",
    startedAt: t1}, {sessionId: "s2", role: "work", startedAt: t2}]` with `t2 > t1`, *When*
    `getLatestWorkSession(item)` is called, *Then* it returns the session with `sessionId: "s2"`.
- All 4 existing inline re-derivations in `BacklogItemDetail.tsx` are replaced by calls to the
  shared helper, with no behavioral change (existing tests continue passing unmodified in
  assertions, only in how the value is sourced).

**Files**: `web-app/src/lib/backlog/currentWorkSession.ts`,
`web-app/src/lib/backlog/currentWorkSession.test.ts`,
`web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 1.1.2a: Create `currentWorkSession.ts` (~3 min)
- Extract `getLatestWorkSession(item: BacklogItem): LinkedSession | undefined` from
  `BacklogItemDetail.tsx:190`'s `[...(item?.linkedSessions ?? [])].reverse().find((s) => s.role
  === "work")`. Add `useCurrentWorkSession(item)` thin `useMemo` wrapper.
- Files: `web-app/src/lib/backlog/currentWorkSession.ts`

##### Task 1.1.2b: Write `currentWorkSession.test.ts` (~3 min)
- Cases: empty `linkedSessions`, single work session, multiple work sessions (most recent wins),
  no work sessions but other roles present.
- Files: `web-app/src/lib/backlog/currentWorkSession.test.ts`

##### Task 1.1.2c: Replace module-scope derivation at `BacklogItemDetail.tsx:190` (~2 min)
- Replace `latestWorkSession` local variable with `useCurrentWorkSession(item)`.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 1.1.2d: Replace Reviewing-section re-derivation (~3 min)
- Locate the independent recomputation inside the review-status block (~`BacklogItemDetail.tsx:
  828-868`) and point it at the same `useCurrentWorkSession(item)` value from Task 1.1.2c instead
  of recomputing.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 1.1.2e: Replace Actions-section and Sessions-list-active re-derivations (~4 min)
- Replace the Actions-section instance (~`BacklogItemDetail.tsx:1074`) and the
  active-session lookup inside the Sessions section's `SessionMonitor` block
  (~`BacklogItemDetail.tsx:1456-1462`) with the shared helper. Note: the active-session lookup
  computes a *different* thing (most recent session matching the current status's expected role,
  any role) — only replace the parts of that block that specifically ask "what's the current
  work session," not its full logic.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

---

#### Story 1.1.3: Session kind classifier
**As a** maintainer, **I want** one closed classification function for a `LinkedSession`'s kind,
**so that** the ad hoc prefix-string checks scattered across the Sessions section can't silently
diverge, and so the currently-uncaught `manual-review-*`/`diff-error-*` dead-link bug gets fixed
at its root.

**Acceptance Criteria**:
- `classifySessionKind()` correctly classifies all 5 kinds, including the two prefixes the
  *current* inline check at `BacklogItemDetail.tsx:1333` misses.
  - *Given* a `LinkedSession` with `role: "review"`, `sessionId: "manual-review-a1b2c3d4-
    1721577600000000000"`, *When* `classifySessionKind(session)` is called, *Then* it returns
    `"manual_review_marker"` — **not** `"review"` (the current bug: this session ID does not
    start with `"headless-"` or `"review-blocked-"` and its role is `"review"` not `"triage"`, so
    today's inline condition at line 1333 falls through to the clickable `<a href="/?session=...">`
    branch, producing a dead link to a session that was never Instance-backed).
  - *Given* a `LinkedSession` with `role: "review"`, `sessionId: "diff-error-<uuid>"`, *When*
    `classifySessionKind(session)` is called, *Then* it returns `"blocked_guardrail"` (same
    pre-existing dead-link bug, second prefix).
  - *Given* a `LinkedSession` with `role: "triage"`, `sessionId: "headless-triage-<uuid>"`,
    *When* `classifySessionKind(session)` is called, *Then* it returns `"headless_diagnostic"`.
  - *Given* a `LinkedSession` with `role: "work"`, `sessionId: "a1b2c3d4-..."` (a normal UUID,
    no synthetic prefix), *When* `classifySessionKind(session)` is called, *Then* it returns
    `"work"`.

**Files**: `web-app/src/lib/backlog/sessionKind.ts`, `web-app/src/lib/backlog/sessionKind.test.ts`

##### Task 1.1.3a: Create `sessionKind.ts` (~4 min)
- `export type SessionKind = "work" | "review" | "headless_diagnostic" | "blocked_guardrail" |
  "manual_review_marker"`. `classifySessionKind(session: LinkedSession): SessionKind` — check
  order: `role === "triage"` or `sessionId.startsWith("headless-")` → `"headless_diagnostic"`;
  `sessionId.startsWith("review-blocked-") || sessionId.startsWith("diff-error-")` →
  `"blocked_guardrail"`; `sessionId.startsWith("manual-review-")` → `"manual_review_marker"`;
  `role === "review"` → `"review"`; else → `"work"`.
- Files: `web-app/src/lib/backlog/sessionKind.ts`

##### Task 1.1.3b: Write `sessionKind.test.ts` (~4 min)
- One case per `SessionKind`, using the exact prefix strings from
  `server/services/backlog_service_triage.go:204-205`,
  `session/backlog_lifecycle.go:1744`, `session/review_gate.go:220,222` (`diff-error-`,
  `review-blocked-`), and `server/services/backlog_service_lifecycle.go:857` (`manual-review-`) —
  plus the two bug-fix regression cases from the story's acceptance criteria.
- Files: `web-app/src/lib/backlog/sessionKind.test.ts`

##### Task 1.1.3c: Wire `classifySessionKind()` into `BacklogItemDetail.tsx`'s session row (~4 min)
- Replace the inline condition at `BacklogItemDetail.tsx:1333` (`s.role === "triage" ||
  s.sessionId.startsWith("headless-") || s.sessionId.startsWith("review-blocked-")`) and the
  active-session-lookup exclusion at `BacklogItemDetail.tsx:1462`
  (`!s.sessionId.startsWith("headless-") && !s.sessionId.startsWith("review-blocked-")`) with
  `classifySessionKind(s) !== "work" && classifySessionKind(s) !== "review"` (or equivalent) —
  this is a mechanical swap only; the actual dispatch to `SessionDiagnosticPanel` happens in
  Epic 4, this task just makes the classification correct everywhere it's checked.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

---

#### Story 1.1.4: Shared `BlockerChip` component
**As a** maintainer, **I want** one `BlockerChip` component with a compact and full variant,
**so that** the detail view's Lifecycle Summary (Epic 2) and the board card (Epic 5) never
implement the "waiting on X" indicator twice.

**Acceptance Criteria**:
- The full variant renders icon + label + duration; the compact variant renders icon + label
  only (no duration), both never color-only.
  - *Given* a `StuckBacklogItem` with `reason: StuckReason.REWORK_CAP` and `firstDetectedAt` 3
    days ago, *When* `<BlockerChip variant="full" item={stuckItem} />` renders, *Then* it shows
    the icon `🔴`, the text label `"Rework cap hit"`, and the duration `"3d"` — all three, not
    color alone.
  - *Given* the same `StuckBacklogItem`, *When* `<BlockerChip variant="compact" item={stuckItem}
    />` renders, *Then* it shows `🔴` and `"Rework cap hit"` but omits the `"3d"` duration.

**Files**: `web-app/src/components/backlog/BlockerChip.tsx`,
`web-app/src/components/backlog/BlockerChip.css.ts`,
`web-app/src/components/backlog/BlockerChip.test.tsx`

##### Task 1.1.4a: Build `BlockerChip.tsx` (~4 min)
- Props: `{ item: StuckBacklogItem; variant: "full" | "compact" }`. Uses
  `getStuckReasonIcon`/`getStuckReasonLabel`/`formatStuckDuration` from
  `web-app/src/components/backlog-stuck/stuckReason.ts` verbatim — no re-derivation.
- Files: `web-app/src/components/backlog/BlockerChip.tsx`

##### Task 1.1.4b: Create `BlockerChip.css.ts` (~3 min)
- Reuse the same color-class-per-`StuckReason` convention as
  `web-app/src/components/backlog-stuck/stuckReason.css.ts` (import shared tokens, do not
  redefine colors).
- Files: `web-app/src/components/backlog/BlockerChip.css.ts`

##### Task 1.1.4c: Write `BlockerChip.test.tsx` (~4 min)
- Both variants, at least 2 `StuckReason` values, assert icon+text both present (not just one).
- Files: `web-app/src/components/backlog/BlockerChip.test.tsx`

---

## Phase 2: Lifecycle Summary Header

### Epic 2.1: Lifecycle Summary Header
**Goal**: Give the detail view a single always-visible header — Stage Tracker + Blocker Chip +
Liveness Line — so the success metric ("identify current lifecycle state and what it's blocked
on without expanding anything") is met from first render.

#### Story 2.1.1: Stage Tracker
**As a** user, **I want** a compact horizontal stepper bound to the item's real status, **so
that** I can see lifecycle position at a glance without reading status-conditional text buried in
the panel.

**Acceptance Criteria**:
- The tracker renders exactly 5 nodes (Idea, Ready, In Progress, Review, Done) regardless of
  `queued`/`pr_pending`/`refining`/`archived`.
  - *Given* `item.status === "queued"`, *When* `<StageTracker status="queued" />` renders,
    *Then* the "In Progress" node is highlighted as active and carries a `"Queued"` modifier
    badge, and no 6th node is rendered.
  - *Given* `item.status === "pr_pending"`, *When* `<StageTracker status="pr_pending" />`
    renders, *Then* the "Review" node is highlighted as active with a `"PR pending"` modifier
    badge.
  - *Given* `item.status === "refining"`, *When* `<StageTracker status="refining" />` renders,
    *Then* the "Idea" node is highlighted as active (no separate node for `refining`).
  - *Given* `item.status === "archived"`, *When* `<StageTracker status="archived" />` renders,
    *Then* an "Archived" overlay ribbon is rendered across the tracker and the underlying stage
    (whichever it was archived from — not reconstructable from `status` alone, so render the
    ribbon over a neutral/dimmed tracker rather than guessing a stage).

**Files**: `web-app/src/components/backlog/detail/StageTracker.tsx`,
`web-app/src/components/backlog/detail/StageTracker.css.ts`,
`web-app/src/components/backlog/detail/StageTracker.test.tsx`

##### Task 2.1.1a: Design `StageTracker.css.ts` (~4 min)
- Recipe with `pending`/`active`/`done` per-node variants, matching the status-chip variant
  pattern already in `GoalPanel.css.ts`'s `statusChipVariants`; a distinct visual treatment for
  the modifier badge and the archived ribbon (per ux.md's explicit "keep the tracker and blocker
  chip visually distinct" guidance, applied here to keep the archived ribbon visually distinct
  from a stage node too, so it never reads as a 6th stage).
- Files: `web-app/src/components/backlog/detail/StageTracker.css.ts`

##### Task 2.1.1b: Build `StageTracker.tsx` (~5 min)
- `deriveStageDisplay(status: BacklogItemStatus): { activeStage: Stage; modifier?: string;
  archived: boolean }` pure function + the rendering component. Stage order:
  `["idea", "ready", "in_progress", "review", "done"]`.
- Files: `web-app/src/components/backlog/detail/StageTracker.tsx`

##### Task 2.1.1c: Write `StageTracker.test.tsx` (~5 min)
- One test per status value in `KnownBacklogStatus` (`useBacklogService.ts:22`), asserting
  correct active stage + modifier + archived-ribbon presence per this story's acceptance
  criteria table.
- Files: `web-app/src/components/backlog/detail/StageTracker.test.tsx`

---

#### Story 2.1.2: Blocker Chip integration via `useStuckBacklogItems`
**As a** user, **I want** the detail view to show why an item is stuck using the same detection
already powering `/unfinished`, **so that** I don't have to infer the cause from Progress History
text.

**Acceptance Criteria**:
- The full-variant `BlockerChip` renders only when this item is present in
  `useStuckBacklogItems()`'s open items list.
  - *Given* `useStuckBacklogItems()` returns `items` containing `{itemId: "itm_a1b2c3", reason:
    StuckReason.STALE_WORK, ...}`, and the detail view is open for `itemId="itm_a1b2c3"`, *When*
    `LifecycleSummary` renders, *Then* it shows a `BlockerChip` with label `"Stale work session"`.
  - *Given* `useStuckBacklogItems()`'s `items` does not contain `itemId="itm_a1b2c3"`, *When*
    `LifecycleSummary` renders for that item, *Then* no `BlockerChip` is rendered at all — never
    a neutral/"OK" placeholder chip.

**Files**: `web-app/src/components/backlog/detail/LifecycleSummary.tsx`,
`web-app/src/components/backlog/detail/LifecycleSummary.test.tsx`

##### Task 2.1.2a: Call `useStuckBacklogItems()` inside `LifecycleSummary` (~3 min)
- `const { items } = useStuckBacklogItems(); const stuckMatch = items.find((i) => i.itemId ===
  item.id);` — per requirements.md's explicit note, no per-item RPC filter exists/is needed for a
  single-user tool; client-side `.find()` over the small open-items list is sufficient.
- Files: `web-app/src/components/backlog/detail/LifecycleSummary.tsx`

##### Task 2.1.2b: Conditionally render `BlockerChip` (~2 min)
- `{stuckMatch && <BlockerChip variant="full" item={stuckMatch} />}` — no fallback/else branch.
- Files: `web-app/src/components/backlog/detail/LifecycleSummary.tsx`

##### Task 2.1.2c: Write integration test mocking `useStuckBacklogItems` (~4 min)
- Mock the hook to return a matching item and a non-matching item across two test cases; assert
  chip presence/absence per this story's acceptance criteria.
- Files: `web-app/src/components/backlog/detail/LifecycleSummary.test.tsx`

---

#### Story 2.1.3: Liveness Line
**As a** user, **I want** a "last activity" line, **so that** I can tell whether an item is truly
idle without cross-referencing Progress History timestamps manually.

**Acceptance Criteria**:
- The line shows the most recent of: latest session's `lastCommitAt`, latest `statusEvents`
  entry's `createdAt`, latest `progressNotes` entry's `createdAt`.
  - *Given* an item whose most recent activity across all three sources is a `statusEvents` entry
    with `createdAt` 12 minutes ago, *When* `LivenessLine` renders, *Then* it shows `"Last
    activity 12m ago"` (via `formatAgo()` from `stuckReason.ts`).
  - *Given* an item with no sessions, no status events, and no progress notes (a brand-new
    `"idea"` item), *When* `LivenessLine` renders, *Then* it falls back to the item's own
    `createdAt` rather than showing `"unknown"`.

**Files**: `web-app/src/components/backlog/detail/LivenessLine.tsx`,
`web-app/src/components/backlog/detail/LivenessLine.test.tsx`

##### Task 2.1.3a: Build `LivenessLine.tsx` (~4 min)
- `deriveLastActivity(item): Timestamp` picks the max timestamp across the three sources
  (falling back to `item.createdAt`); renders via `formatAgo()`.
- Files: `web-app/src/components/backlog/detail/LivenessLine.tsx`

##### Task 2.1.3b: Write `LivenessLine.test.tsx` (~3 min)
- Cases: session-most-recent, status-event-most-recent, progress-note-most-recent, all-empty
  fallback.
- Files: `web-app/src/components/backlog/detail/LivenessLine.test.tsx`

---

#### Story 2.1.4: Assemble `LifecycleSummary` and wire into `BacklogItemDetail`
**As a** user, **I want** the Stage Tracker, Blocker Chip, and Liveness Line together at the top
of the panel, replacing the old standalone status badge, **so that** there is exactly one
authoritative place status is shown (D1).

**Acceptance Criteria**:
- `LifecycleSummary` fully replaces the old status badge markup; no duplicate status text remains
  in the header region.
  - *Given* the detail view is opened for an item with `status: "review"`, *When* the panel
    renders, *Then* `data-testid="lifecycle-summary"` is present and contains the Stage Tracker
    with "Review" active — and the old standalone `<span className={styles.statusBadge}>` markup
    previously at `BacklogItemDetail.tsx:710-714` no longer exists anywhere in the render tree.

**Files**: `web-app/src/components/backlog/detail/LifecycleSummary.tsx`,
`web-app/src/components/backlog/detail/LifecycleSummary.css.ts`,
`web-app/src/components/backlog/BacklogItemDetail.tsx`,
`web-app/src/components/backlog/BacklogItemDetail.test.tsx`

##### Task 2.1.4a: Compose `LifecycleSummary.tsx` (~4 min)
- `<div data-testid="lifecycle-summary"><StageTracker .../><BlockerChip .../ (conditional)><LivenessLine .../></div>`.
- Files: `web-app/src/components/backlog/detail/LifecycleSummary.tsx`

##### Task 2.1.4b: Create `LifecycleSummary.css.ts` (~3 min)
- Layout only (flex row/wrap for mobile); no color logic duplicated from child components.
- Files: `web-app/src/components/backlog/detail/LifecycleSummary.css.ts`

##### Task 2.1.4c: Wire into `BacklogItemDetail.tsx`, remove old status badge (~4 min)
- Replace the block at `BacklogItemDetail.tsx:700-716` (header region containing the standalone
  `statusBadge` span) with `<LifecycleSummary item={item} />`.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 2.1.4d: Update tests (~5 min)
- Write `LifecycleSummary.test.tsx` composition test; update any `BacklogItemDetail.test.tsx`
  assertions that queried the old `statusBadge` class/testid to instead query
  `lifecycle-summary`.
- Files: `web-app/src/components/backlog/detail/LifecycleSummary.test.tsx`,
  `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

---

## Phase 3: Section Decomposition + Progressive Disclosure

### Epic 3.1: Progressive Disclosure Extraction
**Goal**: Split `BacklogItemDetail.tsx`'s remaining 1577 lines into collapsible sibling
components, fix the `itemId` state-leak bug at its root, and consolidate the remaining
duplication findings (D2, D4, D6).

#### Story 3.1.1: `itemId` state-reset fix
**As a** user, **I want** switching between backlog items to reset all per-item UI state, **so
that** e.g. an open manual-review form on item A doesn't stay open when I click into item B.

**Acceptance Criteria**:
- Switching the selected item remounts `BacklogItemDetail` from scratch.
  - *Given* item A is open in the detail pane with `showManualReview === true` (the manual-review
    form visible), *When* the user clicks item B in the list, *Then* `BacklogItemDetail` remounts
    (React reinitializes all `useState` fields) and item B's detail view shows no manual-review
    form open.

**Files**: `web-app/src/app/backlog/page.tsx`, `web-app/src/app/backlog/board/page.tsx`,
`web-app/src/components/backlog/BacklogItemDetail.regression.test.tsx`

##### Task 3.1.1a: Add `key={selectedItemId}` to `<BacklogItemDetail>` (~2 min)
- In `web-app/src/app/backlog/page.tsx` (~line 599), change
  `<BacklogItemDetail itemId={selectedItemId} onClose={handleDetailClose} />` to
  `<BacklogItemDetail key={selectedItemId} itemId={selectedItemId} onClose={handleDetailClose} />`.
  Note: this is the actual drawer/panel host — `web-app/src/components/backlog/
  BacklogItemPanel.tsx` (the small embedded task-panel shown inside a *session* view) is a
  different, unrelated component and is correctly out of scope per requirements.md.
- Files: `web-app/src/app/backlog/page.tsx`

##### Task 3.1.1b: Verify/fix the board page's detail host (~3 min)
- Check whether `web-app/src/app/backlog/board/page.tsx` renders `BacklogItemDetail` via its own
  instance or reuses the same route/component as `page.tsx`; apply the identical `key` fix if it
  renders independently.
- Files: `web-app/src/app/backlog/board/page.tsx`

##### Task 3.1.1c: Add regression test (~4 min)
- Render `BacklogItemDetail` with `itemId="itm_a"`, open the manual-review form, then rerender
  with a `key`-changed `itemId="itm_b"`; assert the manual-review form testid is absent.
- Files: `web-app/src/components/backlog/BacklogItemDetail.regression.test.tsx`

---

#### Story 3.1.2: Extract Planning / Reviewing / Pull Request sections
**As a** maintainer, **I want** the Planning, Reviewing, and Pull Request blocks pulled into
their own collapsible sibling components, **so that** `BacklogItemDetail.tsx` shrinks and each
section is independently testable.

**Acceptance Criteria**:
- `ReviewingSection` and `PullRequestSection` are wrapped in `Collapsible`, each default-expanded
  only when the item is in the matching status.
  - *Given* `item.status === "review"`, *When* the detail view first renders for this item,
    *Then* `ReviewingSection`'s `Collapsible` has `aria-expanded="true"` by default; `PullRequestSection`
    is not rendered at all (it only renders for `status === "pr_pending"`).
  - *Given* `item.status === "done"`, *When* the detail view renders, *Then*
    `ReviewingSection`/`PullRequestSection` are both absent (their original status guards are
    preserved verbatim during extraction).
- PR info has exactly one data source (D4): `PullRequestSection` reads `item.prUrl`/`item.prNumber`
  directly; `VersionControlSection`'s `VcsWidget` no longer independently surfaces PR URL text —
  it links back to `PullRequestSection`'s data instead of re-deriving it.

**Files**: `web-app/src/components/backlog/detail/PlanningSection.tsx`,
`web-app/src/components/backlog/detail/ReviewingSection.tsx`,
`web-app/src/components/backlog/detail/PullRequestSection.tsx`,
`web-app/src/components/backlog/BacklogItemDetail.tsx`,
`web-app/src/components/backlog/BacklogItemDetail.test.tsx`,
`web-app/src/components/backlog/BacklogItemDetail.shipPR.test.tsx`

##### Task 3.1.2a: Extract `PlanningSection.tsx` (~5 min)
- Move the `idea`/`ready`-status triage UI block (`BacklogItemDetail.tsx:772-826`) verbatim into
  a new component with explicit props (`item`, `actionLoading`, action callbacks); no `Collapsible`
  wrapper (primary content, always visible when relevant, matches current always-expanded
  behavior for this block per requirements — only *secondary* sections get progressive
  disclosure).
- Files: `web-app/src/components/backlog/detail/PlanningSection.tsx`

##### Task 3.1.2b: Extract `ReviewingSection.tsx` wrapped in `Collapsible` (~5 min)
- Move the `GateVerdictBox` block (`BacklogItemDetail.tsx:828-892`) into a new component;
  `defaultExpanded={item.status === "review"}`.
- Files: `web-app/src/components/backlog/detail/ReviewingSection.tsx`

##### Task 3.1.2c: Extract `PullRequestSection.tsx` wrapped in `Collapsible` (~4 min)
- Move the `pr_pending`-status block (`BacklogItemDetail.tsx:922-960`) into a new component;
  `defaultExpanded={true}` (only rendered when relevant, so default-expanded is correct here).
- Files: `web-app/src/components/backlog/detail/PullRequestSection.tsx`

##### Task 3.1.2d: Consolidate D4 — single PR data source (~4 min)
- In `VersionControlSection` (extracted in Story 3.4), pass `onViewPr={() =>
  scrollToSection("pull-request")}` or simply omit the independent PR-URL text `VcsWidget` shows
  in `mode="full"` when `item.prUrl` is already shown by `PullRequestSection` for the current
  status; keep `VcsWidget`'s own PR *state* (e.g. CI/mergeability) since that's not duplicated —
  only the raw URL text is (D4 is about the URL being shown twice, not about removing VCS status).
- Files: `web-app/src/components/backlog/detail/PullRequestSection.tsx`,
  `web-app/src/components/backlog/detail/VersionControlSection.tsx`

##### Task 3.1.2e: Update tests + wire into `BacklogItemDetail.tsx` (~5 min)
- Replace the moved JSX blocks in `BacklogItemDetail.tsx` with the 3 new component calls; run
  `BacklogItemDetail.test.tsx` and `BacklogItemDetail.shipPR.test.tsx`, fix any import/query
  breakage (testids must resolve identically — see Epic 6 Story 6.1 for the full verification
  pass).
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`,
  `web-app/src/components/backlog/BacklogItemDetail.test.tsx`,
  `web-app/src/components/backlog/BacklogItemDetail.shipPR.test.tsx`

---

#### Story 3.1.3: Extract Description / Actions sections, consolidate D2
**As a** maintainer, **I want** Description and Actions (including the manual-review form) split
out, **so that** the giant status-conditional Actions block becomes independently reviewable.

**Acceptance Criteria**:
- `DescriptionSection` is collapsed by default (secondary info); `ActionsSection` is always
  expanded (primary, not secondary).
  - *Given* any item, *When* the detail view first renders, *Then* `DescriptionSection`'s
    `Collapsible` has `aria-expanded="false"` by default and `ActionsSection`'s content is
    present without needing to expand anything.
- Polling suspends while the manual-review form is open, not just during `editMode` (pitfalls.md
  gap).
  - *Given* `item.status === "review"` (polling condition true) and the user has
    `showManualReview === true` with unsaved text in `manualReviewSummary`, *When* 5 seconds
    elapse, *Then* no poll-triggered `load()` call fires (matching the existing `editMode`
    suspend pattern at `BacklogItemDetail.tsx:245`), so the in-progress form text is never
    clobbered by a refresh.
- D2 resolved: `AcCriteriaList`/`item.acCriteria` remains the sole "acceptance criteria status"
  display; `GateVerdictBox`'s `criteria` prop (backed by `item.gateCriteria`) is relabeled in its
  own UI to read as "review outcome per criterion" rather than a second, competing AC list.
  - *Given* an item with `acCriteria: [{index: 0, text: "...", status: "done"}]` and
    `gateCriteria: [{label: "...", passed: true}]`, *When* the detail view renders, *Then*
    `AcCriteriaList` shows the done/pending checklist and `GateVerdictBox` shows its
    pass/fail-per-criterion outcomes under a heading distinguishing it as the *review verdict*
    view of the criteria, not a duplicate checklist.

**Files**: `web-app/src/components/backlog/detail/DescriptionSection.tsx`,
`web-app/src/components/backlog/detail/ActionsSection.tsx`,
`web-app/src/components/backlog/GateVerdictBox.tsx`, `web-app/src/components/backlog/GateVerdictBox.css.ts`,
`web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 3.1.3a: Extract `DescriptionSection.tsx` wrapped in `Collapsible` (~3 min)
- Move `BacklogItemDetail.tsx:962-970` verbatim; `defaultExpanded={false}`.
- Files: `web-app/src/components/backlog/detail/DescriptionSection.tsx`

##### Task 3.1.3b: Extract `ActionsSection.tsx` (~5 min)
- Move `BacklogItemDetail.tsx:972-1264` (the full status-conditional action-button block
  including the manual-review inline form) verbatim into a new component with explicit callback
  props; no `Collapsible` wrapper (primary/always-visible).
- Files: `web-app/src/components/backlog/detail/ActionsSection.tsx`

##### Task 3.1.3c: Extend polling-suspend to `showManualReview` (~3 min)
- In the polling `useEffect` (`BacklogItemDetail.tsx:243-249`), change `&& !editMode` to `&&
  !editMode && !showManualReview`; update the effect's dependency array to include
  `showManualReview`.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 3.1.3d: Relabel `GateVerdictBox`'s criteria heading (D2) (~3 min)
- Add a small heading/label above the per-criterion outcomes inside `GateVerdictBox.tsx` reading
  something like "Review outcome per criterion" (exact copy at implementer's discretion) so it's
  visually distinct from `AcCriteriaList`'s "Acceptance Criteria" heading.
- Files: `web-app/src/components/backlog/GateVerdictBox.tsx`,
  `web-app/src/components/backlog/GateVerdictBox.css.ts`

##### Task 3.1.3e: Wire both sections into `BacklogItemDetail.tsx` (~4 min)
- Replace the moved blocks with the 2 new component calls; verify `manual-review-*` testids
  still resolve.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

---

#### Story 3.1.4: Extract remaining secondary sections, consolidate D6
**As a** maintainer, **I want** Plan Artifacts, Version Control, Sessions, Workflow, Progress
History, and Notes each pulled into their own collapsible siblings, **so that** the detail view
matches the "collapsed-by-default secondary sections" requirement end to end.

**Acceptance Criteria**:
- All 6 sections render as independent `Collapsible` siblings; default-expanded state matches
  each section's relevance to current status.
  - *Given* `item.status === "in_progress"`, *When* the detail view renders, *Then*
    `VersionControlSection` defaults to expanded (status is in the "actively worked" set) while
    `PlanArtifactsSection`, `WorkflowHistorySection`, `ProgressHistorySection`, and
    `NotesSection` (with empty `item.notes`) default to collapsed.
  - *Given* `item.notes` is a non-empty string, *When* the detail view renders, *Then*
    `NotesSection` defaults to expanded (non-empty secondary content is worth surfacing by
    default, matching the general progressive-disclosure principle of showing content that
    already has something to show).
- D6 resolved: pipeline mode is promoted to a compact badge in `LifecycleSummary` instead of only
  appearing 3 levels deep inside each session row.
  - *Given* the current work session's `pipelineModeSnapshot` resolves via
    `resolvePipelineModeDisplay()` to `{kind: "resolved", name: "Fast Track", drifted: false}`,
    *When* `LifecycleSummary` renders, *Then* it shows a `"Pipeline: Fast Track"` badge alongside
    the Stage Tracker — the per-session-row pipeline display in `SessionsSection` remains (D6
    doesn't require removing the detailed per-session breakdown, only adding the glanceable
    summary).

**Files**: `web-app/src/components/backlog/detail/PlanArtifactsSection.tsx`,
`web-app/src/components/backlog/detail/VersionControlSection.tsx`,
`web-app/src/components/backlog/detail/SessionsSection.tsx`,
`web-app/src/components/backlog/detail/WorkflowHistorySection.tsx`,
`web-app/src/components/backlog/detail/ProgressHistorySection.tsx`,
`web-app/src/components/backlog/detail/NotesSection.tsx`,
`web-app/src/components/backlog/detail/LifecycleSummary.tsx`,
`web-app/src/components/backlog/BacklogItemDetail.tsx`,
`web-app/src/components/backlog/BacklogItemDetail.css.ts`

##### Task 3.1.4a: Extract `PlanArtifactsSection.tsx` (~3 min)
- Move `BacklogItemDetail.tsx:1290-1295`; `defaultExpanded={false}`.
- Files: `web-app/src/components/backlog/detail/PlanArtifactsSection.tsx`

##### Task 3.1.4b: Extract `VersionControlSection.tsx` (~4 min)
- Move `BacklogItemDetail.tsx:1300-1319` (the `VcsWidget` IIFE block); `defaultExpanded={["in_progress",
  "review", "pr_pending"].includes(item.status)}`.
- Files: `web-app/src/components/backlog/detail/VersionControlSection.tsx`

##### Task 3.1.4c: Extract `SessionsSection.tsx` (~5 min)
- Move `BacklogItemDetail.tsx:1321-1472` (linked sessions list, total cost, `SessionMonitor`);
  `defaultExpanded={true}` (primary operational info, kept expanded by default per requirements'
  emphasis on session inspectability).
- Files: `web-app/src/components/backlog/detail/SessionsSection.tsx`

##### Task 3.1.4d: Extract `WorkflowHistorySection.tsx` (~3 min)
- Move `BacklogItemDetail.tsx:1476-1496`; `defaultExpanded={false}`.
- Files: `web-app/src/components/backlog/detail/WorkflowHistorySection.tsx`

##### Task 3.1.4e: Extract `ProgressHistorySection.tsx` (~3 min)
- Move `BacklogItemDetail.tsx:1499-1520`; `defaultExpanded={false}`.
- Files: `web-app/src/components/backlog/detail/ProgressHistorySection.tsx`

##### Task 3.1.4f: Extract `NotesSection.tsx` (~4 min)
- Move `BacklogItemDetail.tsx:1522-1571`; `defaultExpanded={Boolean(item.notes)}`.
- Files: `web-app/src/components/backlog/detail/NotesSection.tsx`

##### Task 3.1.4g: Add Pipeline badge to `LifecycleSummary` (D6) (~4 min)
- Pass `resolvePipelineModeDisplay(currentWorkSession, pipelineModes)` (already computed via
  `useCurrentWorkSession`, Story 1.1.2) into `LifecycleSummary` and render a compact badge when
  `kind === "resolved"` and `name !== "default"` (omit the badge entirely for the common
  default-pipeline case, to avoid header clutter for the normal path).
- Files: `web-app/src/components/backlog/detail/LifecycleSummary.tsx`,
  `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 3.1.4h: Verify Page Scroll Convention still holds (~3 min)
- Confirm `BacklogItemDetail.css.ts`'s root container still has both `height: "100%"` and
  `overflowY: "auto"` (per `.claude/rules/css-architecture.md`) now that section collapsing
  changes content height dynamically — collapsing/expanding sections must never clip content
  without a scrollbar.
- Files: `web-app/src/components/backlog/BacklogItemDetail.css.ts`

---

#### Story 3.1.5: Auto-expand-on-status-change, first-render-only
**As a** user, **I want** a section to only auto-expand once when it first becomes relevant, **so
that** a routine 5-second poll refresh never silently re-opens a section I deliberately collapsed.

**Acceptance Criteria**:
- Auto-expand rules apply on first mount per `itemId` only, never on a later poll-driven
  re-render of the same item.
  - *Given* the detail view mounts for `itemId="itm_a1b2c3"` with `item.status === "in_progress"`
    (`VersionControlSection` auto-expands per its default rule), and the user then manually
    collapses it, *When* the next 5-second poll refresh completes (still `itemId="itm_a1b2c3"`,
    status unchanged or changed), *Then* `VersionControlSection` remains collapsed — the poll
    does not re-apply the auto-expand default.

**Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx`,
`web-app/src/components/backlog/BacklogItemDetail.test.tsx`

##### Task 3.1.5a: Add a `hasAppliedInitialExpand` ref guard (~4 min)
- `const initialExpandAppliedRef = useRef(false); useEffect(() => {
  initialExpandAppliedRef.current = false; }, [itemId]);` — combined with Story 3.1.1's `key`
  remount, this ref naturally resets per item without extra plumbing; the guard exists so that
  *within* a single mount, a later prop/state change (e.g. status flips from `in_progress` to
  `review` mid-poll) still can't re-trigger a default-expand a user has since overridden. Each
  section's `defaultExpanded` prop is computed once (via `useMemo` keyed only on `itemId`, not on
  every `item` field) rather than recomputed on every poll tick.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 3.1.5b: Write regression test (~4 min)
- Render with `status: "in_progress"`, collapse `VersionControlSection` manually, simulate a poll
  tick that returns an updated `item` object (same `itemId`, new `updatedAt`), assert the section
  is still collapsed.
- Files: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`

---

## Phase 4: Synthetic Session Diagnostic Display

### Epic 4.1: Synthetic Session Diagnostic Display
**Goal**: Replace the inert `<span>`/dead-`<a>` rendering for synthetic sessions with three
distinct, correctly-classified read-only presentations, after confirming the guardrail summary
text is safe to surface.

#### Story 4.1.1: Security review of `review-blocked-*` summary text
**As a** maintainer, **I want** to confirm `RunPreGateSecurityCheck`'s error string never embeds
a raw secret before making it more discoverable in the UI, **so that** this redesign doesn't
accidentally widen a secret-leak surface.

**Acceptance Criteria**:
- The review produces an explicit, documented yes/no answer with the exact code path cited.
  - *Given* `session/backlog_review.go:45`'s `RunPreGateSecurityCheck(diff string) error`
    implementation, *When* its error-construction code path is read end-to-end (including
    whatever secret-detection library/pattern-matcher it calls), *Then* the plan's Unresolved
    Question #1 is either resolved to "confirmed safe — error strings only ever contain file
    path/pattern name/line number, never the matched secret substring" (with the exact line(s)
    proving it cited), or a follow-up redaction task is added to `session/backlog_review.go`
    before Story 4.1.2 proceeds.

**Files**: `session/backlog_review.go` (read-only review), `session/review_gate.go` (read-only
review); `project_plans/backlog-item-detail-ux/implementation/plan.md` (this file, updated with
the finding)

##### Task 4.1.1a: Read `RunPreGateSecurityCheck` end-to-end (~5 min)
- Read `session/backlog_review.go:45` and its full call chain (secret-detection library/regexes
  it invokes) to determine exactly what `secErr`'s `%v` formatting can contain.
- Files: `session/backlog_review.go`

##### Task 4.1.1b: Document the finding (~2 min)
- Update this plan's Unresolved Questions #1 with the confirmed answer, or open a follow-up task
  in `session/backlog_review.go` to redact before this UI surface ships.
- Files: `project_plans/backlog-item-detail-ux/implementation/plan.md`

---

#### Story 4.1.2: Structured Diagnostic renderer for Headless Diagnostic Sessions
**As a** user, **I want** to see the actual triage/re-review output for a `headless-triage-*` or
`headless-re-review-*` session, **so that** I can understand what happened without a dead link.

**Acceptance Criteria**:
- `readOnly` mode on both existing components hides action buttons but preserves all
  informational content.
  - *Given* `<TriageReviewPanel item={item} triageResult={result} readOnly onApply={noop}
    onSkip={noop} />` (callbacks still required by the type but unused when `readOnly`), *When*
    it renders, *Then* the Apply/Skip/Refine buttons are absent from the DOM and the summary,
    suggestions, and tasks list are present.
  - *Given* `<GateVerdictBox verdict="FAIL" summary="..." criteria={[...]} readOnly ... />`,
    *When* it renders, *Then* the Approve/Reopen/Override/Skip/Re-review buttons are absent and
    the verdict icon/label/summary/per-criterion outcomes are present.
- `SessionDiagnosticPanel` correctly routes a `headless_diagnostic`-kind session to the right
  component based on which field is populated.
  - *Given* a `LinkedSession` classified `"headless_diagnostic"` with `triageResult` populated
    and `reviewVerdict` empty (a `headless-triage-*` row), *When* `SessionDiagnosticPanel`
    renders it, *Then* it renders `TriageReviewPanel` in `readOnly` mode, not `GateVerdictBox`.
  - *Given* a `LinkedSession` classified `"headless_diagnostic"` with `reviewVerdict` populated
    and `triageResult` empty (a `headless-re-review-*` row), *When* `SessionDiagnosticPanel`
    renders it, *Then* it renders `GateVerdictBox` in `readOnly` mode.

**Files**: `web-app/src/components/backlog/TriageReviewPanel.tsx`,
`web-app/src/components/backlog/TriageReviewPanel.test.tsx`,
`web-app/src/components/backlog/GateVerdictBox.tsx`,
`web-app/src/components/backlog/GateVerdictBox.test.tsx`,
`web-app/src/components/backlog/detail/SessionDiagnosticPanel.tsx`

##### Task 4.1.2a: Add `readOnly?: boolean` to `TriageReviewPanel` (~4 min)
- Add the prop; when `true`, skip rendering the Apply/Skip/"Refine" button row and the
  dismiss-toast wiring (`isDismissed`/`setDismissed` localStorage calls become no-ops in this
  mode — a historical record shouldn't be dismissible).
- Files: `web-app/src/components/backlog/TriageReviewPanel.tsx`

##### Task 4.1.2b: Add `readOnly?: boolean` to `GateVerdictBox` (~4 min)
- Add the prop; when `true`, skip rendering the action-button row entirely (Approve/Reopen/
  Override/Skip Gate/Re-review), keep the verdict card + per-criterion list.
- Files: `web-app/src/components/backlog/GateVerdictBox.tsx`

##### Task 4.1.2c: Update both components' test files (~4 min)
- Add `readOnly`-mode test cases per this story's acceptance criteria to
  `TriageReviewPanel.test.tsx` and `GateVerdictBox.test.tsx`.
- Files: `web-app/src/components/backlog/TriageReviewPanel.test.tsx`,
  `web-app/src/components/backlog/GateVerdictBox.test.tsx`

##### Task 4.1.2d: Build `SessionDiagnosticPanel.tsx` dispatcher — headless branch (~5 min)
- `classifySessionKind(session) === "headless_diagnostic"` → render `TriageReviewPanel readOnly`
  if `session.triageResult` present, else `GateVerdictBox readOnly` if `session.reviewVerdict`
  present; wrap the whole panel in `role="status"` with a one-line state summary (e.g. "Triage
  completed — 3 suggestions").
- Files: `web-app/src/components/backlog/detail/SessionDiagnosticPanel.tsx`

---

#### Story 4.1.3: Blocked-Before-Start Notice for guardrail/manual-review markers
**As a** user, **I want** a plain explanation when review never actually ran (blocked before
starting, or a manual override), **so that** I don't mistake it for a real session with hidden
output.

**Acceptance Criteria**:
- Both `blocked_guardrail` and `manual_review_marker` kinds render the same explanatory-notice
  treatment, never a session-viewer affordance.
  - *Given* a `LinkedSession` classified `"blocked_guardrail"` with `sessionId:
    "review-blocked-<uuid>"` and `reviewVerdict.summary: "Review blocked by security check:
    secret detected in diff at path/to/file.go:42. Override required to proceed."`, *When*
    `SessionDiagnosticPanel` renders it, *Then* it shows a `BlockedNotice` with that summary text
    verbatim (pending Story 4.1.1's confirmation) under a `role="status"` region, and no "open
    session" affordance of any kind.
  - *Given* a `LinkedSession` classified `"manual_review_marker"` with `sessionId:
    "manual-review-a1b2c3d4-1721577600000000000"` and `reviewVerdict.summary: "Manual review:
    verified fix locally"`, *When* `SessionDiagnosticPanel` renders it, *Then* it shows the same
    `BlockedNotice` treatment with that summary.

**Files**: `web-app/src/components/backlog/detail/BlockedNotice.tsx`,
`web-app/src/components/backlog/detail/BlockedNotice.test.tsx`,
`web-app/src/components/backlog/detail/SessionDiagnosticPanel.tsx`

##### Task 4.1.3a: Build `BlockedNotice.tsx` (~4 min)
- Plain text block, `role="status"`, renders `session.reviewVerdict?.summary ?? "No summary
  recorded."`; distinct icon/label per kind (`"blocked_guardrail"` → "Blocked before starting",
  `"manual_review_marker"` → "Manual review").
- Files: `web-app/src/components/backlog/detail/BlockedNotice.tsx`

##### Task 4.1.3b: Wire into `SessionDiagnosticPanel.tsx` (~2 min)
- Add the `"blocked_guardrail"`/`"manual_review_marker"` branches to the dispatcher started in
  Task 4.1.2d.
- Files: `web-app/src/components/backlog/detail/SessionDiagnosticPanel.tsx`

##### Task 4.1.3c: Write `BlockedNotice.test.tsx` (~4 min)
- 3 fixtures: `review-blocked-*`, `diff-error-*`, `manual-review-*`, asserting the summary text
  renders and no clickable session-open affordance is present.
- Files: `web-app/src/components/backlog/detail/BlockedNotice.test.tsx`

---

#### Story 4.1.4: Wire `SessionDiagnosticPanel` into `SessionsSection`, fix the dead-link bug
**As a** user, **I want** every session row — real or synthetic — to be inspectable, **so that**
no row in the Sessions list is inert text or a broken link.

**Acceptance Criteria**:
- Real sessions (`work`, `review`) keep their existing `<a href="/?session=...">` behavior
  unchanged; all 3 synthetic kinds get an inline `Collapsible`-wrapped `SessionDiagnosticPanel`
  instead of a `<span>` or a dead `<a>`.
  - *Given* a session row classified `"work"`, *When* `SessionsSection` renders it, *Then* it is
    an `<a href="/?session=<id>">` exactly as today.
  - *Given* a session row classified `"manual_review_marker"` (previously a dead
    `<a href="/?session=manual-review-...">` per the Story 1.1.3 bug), *When* `SessionsSection`
    renders it after this change, *Then* it is a `Collapsible` header button (not an `<a>`) that
    expands to a `BlockedNotice`.

**Files**: `web-app/src/components/backlog/detail/SessionsSection.tsx`,
`web-app/src/components/backlog/detail/SessionsSection.test.tsx`

##### Task 4.1.4a: Replace the row-kind branching in `SessionsSection.tsx` (~5 min)
- Replace the inert-`<span>` / clickable-`<a>` conditional (moved here from
  `BacklogItemDetail.tsx:1333-1372` in Story 3.1.4c) with a `switch (classifySessionKind(s))`:
  `"work"`/`"review"` → existing `<a>` markup unchanged; the 3 synthetic kinds → a `Collapsible`
  wrapping `<SessionDiagnosticPanel session={s} />`, `defaultExpanded={false}`.
- Files: `web-app/src/components/backlog/detail/SessionsSection.tsx`

##### Task 4.1.4b: Write `SessionsSection.test.tsx` covering all 5 kinds (~5 min)
- One fixture per `SessionKind`, asserting the correct row treatment (link vs. collapsible
  diagnostic) per this story's acceptance criteria.
- Files: `web-app/src/components/backlog/detail/SessionsSection.test.tsx`

---

## Phase 5: Board Card Consistency

### Epic 5.1: Board Card Consistency
**Goal**: Give `BacklogItemCard.tsx` the same "waiting on X" signal the detail view now has,
using the shared `BlockerChip`, closing the gap the earlier `backlog-ux` project's US-3 left open
(session cards only, not backlog item cards).

#### Story 5.1.1: `BlockerChip` on `BacklogItemCard`
**As a** user scanning `/backlog/board`, **I want** stuck items to show the same blocker
indicator the detail view shows, **so that** I don't have to open each card to find out it's
stuck.

**Acceptance Criteria**:
- The compact `BlockerChip` renders in the card footer only for items present in
  `useStuckBacklogItems()`'s open list.
  - *Given* the board page has fetched `useStuckBacklogItems()` and its `items` includes
    `{itemId: "itm_x9y8z7", reason: StuckReason.PR_READY_UNMERGED, ...}`, and the board renders a
    `BacklogItemCard` for `item.id === "itm_x9y8z7"`, *When* the card renders, *Then* its footer
    shows a compact `BlockerChip` with icon `🟢` and label `"PR ready to merge"`.
  - *Given* an item not present in the stuck-items list, *When* its card renders, *Then* no
    `BlockerChip` appears in the footer (footer layout matches today's — `AcSummary` + action
    button only).

**Files**: `web-app/src/app/backlog/board/page.tsx`,
`web-app/src/components/backlog/BacklogItemCard.tsx`,
`web-app/src/components/backlog/BacklogItemCard.css.ts`,
`web-app/src/components/backlog/BacklogItemCard.test.tsx`

##### Task 5.1.1a: Wire `useStuckBacklogItems()` into the board page (~3 min)
- Call the hook once at `board/page.tsx` level (not per-card, to avoid N independent 60s polls)
  and pass the resolved `StuckBacklogItem | undefined` down as a prop per card.
- Files: `web-app/src/app/backlog/board/page.tsx`

##### Task 5.1.1b: Render `BlockerChip` in `BacklogItemCard.tsx`'s footer (~3 min)
- Add optional prop `stuckItem?: StuckBacklogItem`; render `<BlockerChip variant="compact"
  item={stuckItem} />` inside `styles.cardFooter` when present.
- Files: `web-app/src/components/backlog/BacklogItemCard.tsx`

##### Task 5.1.1c: Update `BacklogItemCard.css.ts` for the compact chip layout (~3 min)
- Add flex-wrap allowance in `cardFooter` so the chip doesn't overflow on narrow/mobile card
  widths.
- Files: `web-app/src/components/backlog/BacklogItemCard.css.ts`

##### Task 5.1.1d: Write `BacklogItemCard.test.tsx` blocker-chip cases (~4 min)
- 2 cases: `stuckItem` present (chip renders), absent (chip absent, footer unchanged from
  today's snapshot).
- Files: `web-app/src/components/backlog/BacklogItemCard.test.tsx`

---

#### Story 5.1.2: `/backlog` non-board list-view decision
**As a** maintainer, **I want** an explicit decision on whether `BacklogItemBadge.tsx` (the
non-board list row) also needs the compact `BlockerChip`, **so that** Unresolved Question #2
doesn't get silently skipped.

**Acceptance Criteria**:
- The decision is recorded with its reasoning, and either implemented or explicitly deferred with
  a reason.
  - *Given* the current `/backlog` list row width (as rendered by `BacklogItemBadge.tsx`),
    *When* the implementer measures available horizontal space next to the existing status
    chip/AC-count/title layout, *Then* the plan is updated with either "implemented — see Task
    5.1.2a" or "deferred — insufficient row width for a 3rd inline element without truncating the
    title further," with the reasoning visible in this file or a follow-up code comment.

**Files**: `web-app/src/components/backlog/BacklogItemBadge.tsx` (implementation, if the decision
is to implement)

##### Task 5.1.2a: Measure and decide (~3 min)
- Check `BacklogItemBadge.css.ts` for current width constraints and how many list rows typically
  render per screen at mobile width; make the call.
- Files: `web-app/src/components/backlog/BacklogItemBadge.tsx`,
  `web-app/src/components/backlog/BacklogItemBadge.css.ts`

##### Task 5.1.2b: Implement if in-scope (~4 min)
- If Task 5.1.2a decides yes: add compact `BlockerChip` to `BacklogItemBadge.tsx` following the
  same pattern as Story 5.1.1's `BacklogItemCard.tsx` change. If no: skip, documenting why in a
  code comment above the badge's existing status-chip render.
- Files: `web-app/src/components/backlog/BacklogItemBadge.tsx`

---

## Phase 6: Test Coverage

### Epic 6.1: Test Coverage
**Goal**: Verify the redesign preserves every existing testid, and add the e2e/registry coverage
requirements.md's success metrics demand.

#### Story 6.1.1: testid preservation verification
**As a** maintainer, **I want** confirmation that every pre-existing `data-testid` still resolves
after the Epic 3 decomposition, **so that** no downstream test (Jest or e2e) silently breaks.

**Acceptance Criteria**:
- All 9 listed testids resolve to exactly one element each after decomposition.
  - *Given* the full Epic 1–5 decomposition is complete, *When* `BacklogItemDetail.test.tsx`,
    `.regression.test.tsx`, `.shipPR.test.tsx`, and `.markdown.test.tsx` all run, *Then* every
    query for `manual-review-form`, `manual-review-outcome`, `manual-review-summary`,
    `manual-review-submit`, `backlog-action-ship-pr`, `backlog-action-override-done`,
    `backlog-action-re-review`, `backlog-action-manual-review`, and `backlog-action-restart-session`
    resolves to exactly one element, and all 4 test files pass with zero skipped/`.only` tests.

**Files**: `web-app/src/components/backlog/BacklogItemDetail.test.tsx`,
`web-app/src/components/backlog/BacklogItemDetail.regression.test.tsx`,
`web-app/src/components/backlog/BacklogItemDetail.shipPR.test.tsx`,
`web-app/src/components/backlog/BacklogItemDetail.markdown.test.tsx`

##### Task 6.1.1a: Run the full existing test suite and fix any breakage (~5 min)
- `cd web-app && npx jest --no-coverage --testPathPatterns="BacklogItemDetail"`; fix any query
  breakage from the Epic 3 extraction (imports moved, but testids must be unchanged).
- Files: (as listed above, whichever needs a fix)

---

#### Story 6.1.2: New e2e spec
**As a** maintainer, **I want** at least one e2e spec exercising the redesigned detail view, **so
that** the success metrics ("visible without expanding," "every session type inspectable") have a
regression guard beyond unit tests.

**Acceptance Criteria**:
- The spec covers: opening an item, verifying the Lifecycle Summary is visible without expanding
  anything, expanding a section, and opening the diagnostic panel for a synthetic session row.
  - *Given* a seeded backlog item with `status: "review"` and a `headless-triage-*` linked
    session, *When* the e2e spec opens the item's detail view, *Then* `page.getByTestId(
    "lifecycle-summary")` is visible with zero prior clicks, and clicking the synthetic session
    row's `Collapsible` header reveals the `TriageReviewPanel readOnly` summary text.
- Conventions followed per `.claude/rules/e2e-test-conventions.md`: `// @feature` header, no
  `waitForTimeout`, `data-testid`/ARIA locators only, new page helper under `tests/e2e/pages/`.

**Files**: `tests/e2e/backlog-item-detail-redesign.spec.ts`,
`tests/e2e/pages/BacklogItemDetailPage.ts`

##### Task 6.1.2a: Resolve Unresolved Question #3 (seed fixture) (~3 min)
- Check `tests/e2e/pages/BacklogPage.ts` and `server/services/backlog_debug_seed_handler.go` for
  an existing way to seed a `headless-triage-*` `ItemSession`; if none exists, add a minimal
  `handleSeedHeadlessTriageSession` following the exact pattern of the existing
  `handleSeedQueued`/`handleSeedStuckItem` e2e-only debug endpoints (registered only under
  `STAPLER_SQUAD_INSTANCE=e2e-local`, per the existing convention).
- Files: `server/services/backlog_debug_seed_handler.go` (only if a new endpoint is needed)

##### Task 6.1.2b: Add `BacklogItemDetailPage.ts` page helper (~4 min)
- Locators for `lifecycle-summary`, section `Collapsible` headers (by `aria-expanded` +
  accessible name), and the synthetic-session row's diagnostic panel.
- Files: `tests/e2e/pages/BacklogItemDetailPage.ts`

##### Task 6.1.2c: Write `backlog-item-detail-redesign.spec.ts` (~5 min)
- `// @feature backlog:item-detail` header; 4 test cases per this story's acceptance criteria,
  using `expect(locator).toBeVisible()`/`toHaveAttribute("aria-expanded", ...)`, never
  `waitForTimeout`.
- Files: `tests/e2e/backlog-item-detail-redesign.spec.ts`

---

#### Story 6.1.3: Registry updates
**As a** maintainer, **I want** the new components registered in `docs/registry/features/`, **so
that** `make registry-generate` doesn't report a net-new coverage gap.

**Acceptance Criteria**:
- Every new top-level component gets a per-feature JSON entry with `tested: true` and its test
  function names populated.
  - *Given* `LifecycleSummary.tsx`, `SessionDiagnosticPanel.tsx`, and the `BlockerChip` addition
    to `BacklogItemCard.tsx` are complete, *When* `make registry-generate` runs, *Then*
    `docs/registry/coverage-gaps.json`'s count is not greater than its pre-project baseline.

**Files**: `docs/registry/features/frontend/ui/backlog-item-detail-lifecycle-summary.json`,
`docs/registry/features/frontend/ui/backlog-item-detail-diagnostic-panel.json`,
`docs/registry/features/frontend/ui/backlog-item-card.json` (update existing)

##### Task 6.1.3a: Add `// +feature:` markers to new components (~3 min)
- Add `// +feature: backlog:item-detail-lifecycle-summary` (and similarly for
  `SessionDiagnosticPanel.tsx`) in the first 10 lines, per `.claude/rules/feature-registry.md`.
- Files: `web-app/src/components/backlog/detail/LifecycleSummary.tsx`,
  `web-app/src/components/backlog/detail/SessionDiagnosticPanel.tsx`

##### Task 6.1.3b: Run `make registry-generate` and commit changed files (~3 min)
- Run `make registry-generate`; verify `docs/registry/features/frontend/ui/backlog-item-card.json`'s
  `tested` flips to `true` with the new `BacklogItemCard.test.tsx` cases included in `testIds`.
- Files: `docs/registry/features/frontend/ui/*.json` (generated diffs)
