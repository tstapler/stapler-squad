# Implementation Plan: backlog-item-unarchive

**Feature**: Restore an archived backlog item to active status from the UI, without a
stale `archived_at` timestamp, plus a confirmation gate on the archive action now that
it's reversible.
**Date**: 2026-08-29
**Status**: Ready for implementation
**ADRs**: [ADR-001: Reuse `TransitionBacklogItemStatus` instead of adding a dedicated
`UnarchiveBacklogItem` RPC](../decisions/ADR-001-reuse-transition-rpc-over-new-unarchive-rpc.md)

---

## Domain Glossary

This feature reuses existing domain types verbatim — no new types are introduced.

| Term | Definition | Notes |
|------|-----------|-------|
| `BacklogStatus` | Enum-like string type for a backlog item's lifecycle stage (`idea`, `ready`, `queued`, `in_progress`, `review`, `pr_pending`, `done`, `archived`, `refining`). | `session/domain/backlog.go`. No new value added. |
| `archived_at` | Nillable timestamp on `BacklogItem` marking when an item entered `archived` status. | `session/ent/schema/backlog_item.go:79-81`. This feature fixes the one path (`archived → idea`) that failed to clear it. |
| `TransitionBacklogItemStatus` | The generic, CAS-protected, guard-checked RPC + repository method used for every manual status change except `archive`/`delete`. | `session/ent_repository_backlog.go:869`. This feature adds one gated builder call to it; no new method or RPC. |
| `BacklogItemPrecondition` | Optional CAS precondition (`ExpectedStatus`, `ExpectedUpdatedAt`) passed into `TransitionBacklogItemStatus` to prevent a stale write. | `session/repository.go`. Unchanged by this feature; the new unarchive call site sets `ExpectedStatus: "archived"`. |
| `BacklogStatusEvent` | Immutable audit row recorded on every status transition (`from_status`, `to_status`, `triggered_by`). | Already written unconditionally by `TransitionBacklogItemStatus` (`recordStatusEvent`, `ent_repository_backlog.go:938`) — satisfied for free by reusing this path. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Backend restore path | Fix `EntRepository.TransitionBacklogItemStatus` to call `.ClearArchivedAt()` when `from == BacklogStatusArchived`; UI calls the existing RPC with target `idea` | `research/architecture.md` (Option B), `research/build-vs-buy.md` (Option B) | **Option A** — dedicated `UnarchiveBacklogItem` RPC + new `BacklogItemUnarchivedEvent` | Duplicates or wraps guard/CAS/audit logic `TransitionBacklogItemStatus` already provides (forwarding-only-wrapper smell, `.claude/rules/interface-pollution-checklist.md`); new event type carries no info beyond the existing `old_status`/`new_status` on `BacklogItemStatusChangedEvent`; larger blast radius (proto + 2 generated-code consumers + TS event union) for zero behavioral gain. See ADR-001. |
| Backend restore path (cont.) | (same as above) | (same as above) | **Option C** — fork `UnarchiveSession`'s bare-timestamp-flip style into a new RPC | Actively incorrect for this domain: skips `CanTransitionBacklog`/CAS/`recordStatusEvent`, which sessions have no equivalent of to skip in the first place. Strictly dominated by Option A. See ADR-001. |
| Frontend dispatch | New `case "unarchive":` in `BacklogItemDetail.tsx`'s existing `handleAction` switch, calling `transitionStatus(item.id, "idea")` — same shape as sibling cases (`send_back_idea`, `mark_ready`) | `research/build-vs-buy.md`, `research/architecture.md` fact #6 | A new dedicated frontend RPC hook (`unarchiveBacklogItem`) | No new RPC exists to call (per the above); the generic `transitionStatus` wrapper already does exactly this shape of call for every other status target. |
| Confirmation mechanism | Reuse the same native `confirm()` call already used for `delete`, at the same `case "archive":` call site | `research/ux.md` §4 | A custom modal/dialog component | AC0 only requires "matching the pattern used for delete" — `confirm()` is that pattern; introducing a new modal component for one call site would be disproportionate to the change. |
| Button component/state | Copy the `done` block's button shape exactly (`styles.actionButton`, `disabled={actionLoading !== null}`, `aria-busy`, `data-testid`) — no `aria-disabled`/precondition styling | `research/ux.md` §1, §3 | `aria-disabled` + conditional `title` pattern (used by `mark_ready`) | Unarchive has no precondition beyond "another action isn't already running" — matches `reopen`/`override_done`, not `mark_ready`. |

---

## Migration Plan

*(Omitted — no schema or data changes. `archived_at`'s `ClearArchivedAt()` builder method
already exists on the generated ent code; see `research/stack.md` and
`.claude/rules/ent-schema-generation.md` — this feature does not trigger that rule.)*

## Observability Plan

- **Logs**: None added. The existing `TransitionBacklogItemStatus` code path has no
  structured logging beyond error returns today; this feature does not change that
  baseline.
- **Metrics**: None added. No existing metric distinguishes transition targets; adding
  one would be disproportionate to a single new UI action reusing an existing RPC.
- **Alerts**: None. `ErrPreconditionFailed` on a concurrent double-unarchive surfaces
  as a normal RPC error to the caller (same handling as every other CAS-protected
  transition) — no new alerting surface needed.

## Risk Control

- **Feature flag**: None. This is additive UI (a button that only renders for
  `status === "archived"`, a codepath already reachable via other transitions) plus a
  backend fix scoped to a single `from` value — no flag is warranted for a change this
  contained.
- **Rollback procedure**: Revert the PR. The backend fix is a single gated builder call
  with no data migration; reverting leaves `archived_at` un-cleared again (the
  pre-existing bug) but introduces no new failure mode.
- **Staged rollout**: Not applicable — internal tool, no external users, deployed via
  the existing `make install-service` flow.

## Unresolved Questions

None. All ambiguities flagged in research (button label, confirm wording, restore
target, CAS approach, registry scope) were resolved during research and are reflected
in the Pattern Decisions table and task list below.

## Dependency Visualization

```
Phase 1 (backend)                         Phase 2 (frontend)
┌─────────────────────────────┐           ┌──────────────────────────────┐
│ 1.1.1a  Fix ClearArchivedAt  │           │ 2.1.1a  Add confirm() to     │
│         in TransitionBacklog │           │         archive case         │
│         ItemStatus           │           │         (independent of      │
│         │                    │           │         backend fix)         │
│         ▼                    │           │                              │
│ 1.1.1b  Go regression test   │           │ 2.2.1a  Add "archived" branch │
│         │                    │           │         + Unarchive button   │
│         ▼                    │           │         in ActionsSection    │
│ 1.2.1a  Update backend       │           │         │                    │
│         registry entry       │           │         ▼                   │
└─────────────┬────────────────┘           │ 2.2.2a  Wire case "unarchive"│
              │                            │         in handleAction +   │
              │                            │         ACTION_SUCCESS_     │
              │                            │         MESSAGES entry      │
              │                            │         │                   │
              │                            │         ▼                   │
              │                            │ 2.3.1a  ActionsSection test │
              │                            │ 2.3.2a  BacklogItemDetail   │
              │                            │         test (confirm +    │
              │                            │         unarchive wiring)  │
              │                            └──────────────┬──────────────┘
              │                                            │
              └────────────────────┬───────────────────────┘
                                    ▼
                    Phase 3 (registry + e2e — needs both)
              ┌──────────────────────────────────────────┐
              │ 3.1.1a  New frontend registry entry       │
              │ 3.2.1a  Playwright e2e spec                │
              │ 3.3.1a  make registry-generate + verify    │
              │         coverage-gaps.json doesn't grow    │
              └──────────────────────────────────────────┘
```

Phase 1 and Phase 2 touch disjoint files and have no code dependency on each other —
they may be implemented in either order or in parallel. Phase 3's e2e spec exercises
both, so it is sequenced last.

---

## Phase 1: Backend — clear `archived_at` on the way out of `archived`

### Epic 1.1: Fix the `archived_at` leak in `TransitionBacklogItemStatus`

**Goal**: `archived → idea` transitions clear `archived_at`, with the fix scoped
tightly enough to not touch any other transition pair, and with the existing
CAS/audit guarantees preserved.

#### Story 1.1.1: `TransitionBacklogItemStatus` clears `archived_at` when leaving `archived`

**As a** user who unarchives a backlog item, **I want** its `archived_at` timestamp
cleared, **so that** the item doesn't carry a stale archival marker once it's active
again.

**Acceptance Criteria** (maps to requirements.md AC1, AC3, AC4):

- AC1: A backend path exists to move a backlog item from `archived` back to `idea`
  that also clears `archived_at`.
  - *Given* a backlog item with `Status: "archived"` and `ArchivedAt: <non-nil
    timestamp>`, *When* `EntRepository.TransitionBacklogItemStatus(ctx, item.ID,
    BacklogStatusIdea, &BacklogItemPrecondition{ExpectedStatus: "archived"},
    TriggeredByUser)` is called, *Then* the returned/reloaded item has
    `Status == "idea"` and `ArchivedAt == nil`.
- AC3: The restore is recorded in the audit history.
  - *Given* the same call as above, *When* it succeeds, *Then* a new
    `BacklogStatusEvent` row exists with `FromStatus: "archived"`, `ToStatus: "idea"`,
    `TriggeredBy: TriggeredByUser`.
- AC4: Existing archive/delete behavior and tests are unaffected.
  - *Given* `TestBacklogIntegration_ArchiveRecordsStatusEvent` (unchanged,
    `session/backlog_integration_test.go:258`) and the existing
    `TestTransitionBacklogItemStatus_should_rejectStaleReopen_When_ItemAlreadyShippedSinceReview`
    (`session/ent_repository_backlog_transition_test.go:27`), *When* the fix (gated on
    `from == BacklogStatusArchived`) is applied, *Then* both continue to pass unmodified
    — neither test's `from` status is `archived`, so the new `ClearArchivedAt()` call
    never fires for them.

**Files**: `session/ent_repository_backlog.go`,
`session/ent_repository_backlog_transition_test.go`

##### Task 1.1.1a: Add the gated `ClearArchivedAt()` call (~3 min)

- In `EntRepository.TransitionBacklogItemStatus` (`session/ent_repository_backlog.go`),
  the `update` builder is currently built at line 883 (`update :=
  r.client.BacklogItem.Update().Where(backlogitem.ID(parsedID))`) and the `.SetStatus(...)`
  chain begins at line 894. `current.Status` (from the `Get` at line 875) is already
  available before the chain is built.
- Add, immediately before the `affected, err := update. ...` chain (around line 893):
  ```go
  if current.Status == string(BacklogStatusArchived) {
      update = update.ClearArchivedAt()
  }
  ```
- This must run whether or not a `precondition` was supplied (the `if precondition !=
  nil` block above only adds `Where` clauses; this is a separate, unconditional check
  against `current.Status`, matching the existing `fromStatus := current.Status`
  fallback pattern used a few lines below at line 930).
- Files: `session/ent_repository_backlog.go`

##### Task 1.1.1b: Go regression test for the `archived_at` clear (~5 min)

- Add to `session/ent_repository_backlog_transition_test.go` (same package, same
  `createTestEntRepository` helper used by the existing tests in this file):
  ```go
  // TestTransitionBacklogItemStatus_should_ClearArchivedAt_When_TransitioningFromArchivedToIdea
  // covers the fix for the "archived_at never clears" bug named in
  // project_plans/backlog-item-unarchive/requirements.md: a raw archived->idea
  // transition previously flipped `status` back to "idea" but left a stale
  // non-nil `archived_at`, so an unarchived item still looked archived by that
  // field. Also asserts the CAS precondition and audit trail (BacklogStatusEvent)
  // both still fire correctly for this specific from/to pair.
  func TestTransitionBacklogItemStatus_should_ClearArchivedAt_When_TransitioningFromArchivedToIdea(t *testing.T) {
      repo, cleanup := createTestEntRepository(t)
      defer cleanup()
      ctx := context.Background()

      archivedAt := time.Now().Add(-time.Hour)
      item, err := repo.CreateBacklogItem(ctx, BacklogItemData{
          Title:      "archived item to restore",
          Status:     string(BacklogStatusArchived),
          ArchivedAt: &archivedAt,
      })
      require.NoError(t, err)
      require.NotNil(t, item.ArchivedAt)

      restored, err := repo.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusIdea, &BacklogItemPrecondition{
          ExpectedStatus: string(BacklogStatusArchived),
      }, TriggeredByUser)
      require.NoError(t, err)
      assert.Equal(t, string(BacklogStatusIdea), restored.Status)
      assert.Nil(t, restored.ArchivedAt, "archived_at must be cleared when leaving archived status")

      fetched, err := repo.GetBacklogItem(ctx, item.ID)
      require.NoError(t, err)
      assert.Nil(t, fetched.ArchivedAt)
      require.Len(t, fetched.StatusEvents, 1)
      assert.Equal(t, string(BacklogStatusArchived), fetched.StatusEvents[0].FromStatus)
      assert.Equal(t, string(BacklogStatusIdea), fetched.StatusEvents[0].ToStatus)
      assert.Equal(t, TriggeredByUser, fetched.StatusEvents[0].TriggeredBy)

      // A second unarchive attempt with a now-stale precondition must fail —
      // proves the CAS protection this fix must not weaken.
      _, err = repo.TransitionBacklogItemStatus(ctx, item.ID, BacklogStatusIdea, &BacklogItemPrecondition{
          ExpectedStatus: string(BacklogStatusArchived),
      }, TriggeredByUser)
      require.ErrorIs(t, err, ErrPreconditionFailed)
  }
  ```
  Add `"time"` to the existing import block if not already present (it is not, per the
  file's current imports: `context`, `errors`, `sync`, `testing`).
- Run: `go build ./... && go test ./session -run TestTransitionBacklogItemStatus_should_ClearArchivedAt`
- Files: `session/ent_repository_backlog_transition_test.go`

### Epic 1.2: Backend feature registry update

**Goal**: The registry reflects that `backlog:transition-status` now has test coverage
for this behavior, per `.claude/rules/feature-registry.md`.

#### Story 1.2.1: Update the existing backend registry entry

**As a** maintainer reviewing the registry, **I want** the `transition-status` entry to
show it's tested, **so that** `coverage-gaps.json` doesn't count it as a gap.

**Acceptance Criteria**:
- The existing entry is updated, not duplicated (no new RPC was added).
  - *Given* `docs/registry/features/backend/backlog/transition-status.json` currently
    has `"tested": false, "testIds": []`, *When* this task completes, *Then* it has
    `"tested": true` and `"testIds"` includes
    `"TestTransitionBacklogItemStatus_should_ClearArchivedAt_When_TransitioningFromArchivedToIdea"`.

**Files**: `docs/registry/features/backend/backlog/transition-status.json`

##### Task 1.2.1a: Edit the registry entry (~2 min)

- Edit `docs/registry/features/backend/backlog/transition-status.json`: set
  `"tested": true`, set `"testIds": ["TestTransitionBacklogItemStatus_should_ClearArchivedAt_When_TransitioningFromArchivedToIdea"]`,
  update `"lastModified"` to the current ISO 8601 timestamp.
- Files: `docs/registry/features/backend/backlog/transition-status.json`

---

## Phase 2: Frontend — confirm-before-archive and the Unarchive action

### Epic 2.1: Archive confirmation gate

**Goal**: Archiving requires an explicit confirmation, worded accurately now that
archive is reversible (AC0).

#### Story 2.1.1: Add a `confirm()` gate to the `archive` action

**As a** user clicking Archive, **I want** a confirmation prompt, **so that** I don't
accidentally hide an item without meaning to.

**Acceptance Criteria** (maps to requirements.md AC0):
- A confirmation prompt appears before archiving, matching the delete pattern (same
  `confirm()` mechanism, same call-site shape), with wording that does not claim
  irreversibility.
  - *Given* `BacklogItemDetail.tsx`'s `handleAction("archive")` is invoked (e.g. via
    the existing `done`-status Archive button), *When* `window.confirm` returns `true`
    (user clicks OK), *Then* `archiveBacklogItem(item.id)` is called exactly as before.
  - *Given* the same invocation, *When* `window.confirm` returns `false` (user clicks
    Cancel), *Then* `archiveBacklogItem` is **not** called and no toast is shown.
  - *Given* the confirm dialog is shown, *Then* its message is exactly `"Archive this
    item? It will be hidden from the default list, but can be restored later."` — not
    delete's `"Permanently delete..."` wording, since that would misrepresent archive
    as irreversible.

**Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 2.1.1a: Add the confirm gate to `case "archive"` (~2 min)

- In `handleAction`'s switch (`web-app/src/components/backlog/BacklogItemDetail.tsx:538-540`),
  change:
  ```tsx
  case "archive":
    await archiveBacklogItem(item.id);
    break;
  ```
  to:
  ```tsx
  case "archive":
    if (!confirm("Archive this item? It will be hidden from the default list, but can be restored later.")) return;
    await archiveBacklogItem(item.id);
    break;
  ```
  (Matches the exact `if (!confirm(...)) return;` shape already used one case below at
  line 542 for `delete`.)
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

### Epic 2.2: Unarchive UI action

**Goal**: An archived item's detail view offers an "Unarchive" button that restores it
to `idea` (AC2).

#### Story 2.2.1: Render an Unarchive button when `item.status === "archived"`

**As a** user viewing an archived item's detail page, **I want** an Unarchive button,
**so that** I can restore it without leaving the page.

**Acceptance Criteria** (maps to requirements.md AC2):
- The button renders only for archived items, with the same accessibility/loading
  conventions as its siblings.
  - *Given* `ActionsSection` is rendered with `item.status === "archived"` and
    `terminalState === null`, *When* rendered, *Then*
    `screen.getByTestId("backlog-action-unarchive")` is present, showing label
    "Unarchive", with `disabled={actionLoading !== null}` and `aria-busy={actionLoading
    === "unarchive"}` (no `aria-disabled`, no conditional `title` — no precondition
    exists for this action).
  - *Given* the same render, *When* the button is clicked, *Then* `onAction` is called
    with `"unarchive"`.
  - *Given* `item.status` is any value other than `"archived"`, *When* rendered,
    *Then* `screen.queryByTestId("backlog-action-unarchive")` is absent.

**Files**: `web-app/src/components/backlog/detail/ActionsSection.tsx`

##### Task 2.2.1a: Add the `item.status === "archived"` render branch (~4 min)

- In `web-app/src/components/backlog/detail/ActionsSection.tsx`, add a new branch
  immediately after the `done` block (after line 341, before the "Backward
  transitions" comment on line 343) — sibling position, not nested inside the
  backward-transitions block, since `"archived"` is intentionally absent from that
  block's status list (line 344) and must stay absent (backward transitions apply to
  active-pipeline statuses, not archived):
  ```tsx
  {item.status === "archived" && (
    <button
      className={styles.actionButton}
      onClick={() => onAction("unarchive")}
      disabled={actionLoading !== null}
      aria-busy={actionLoading === "unarchive"}
      title="Restore to Idea for re-triage"
      data-testid="backlog-action-unarchive"
    >
      <ActionButtonLabel pending={actionLoading === "unarchive"} label="Unarchive" />
    </button>
  )}
  ```
- Files: `web-app/src/components/backlog/detail/ActionsSection.tsx`

#### Story 2.2.2: Wire the `unarchive` action to the restore path

**As a** user clicking Unarchive, **I want** the item actually restored, **so that** it
reappears in default list views with a success message.

**Acceptance Criteria** (maps to requirements.md AC1 end-to-end from the UI, AC2):
- Clicking Unarchive calls the existing generic transition path with target `idea`.
  - *Given* `handleAction("unarchive")` is invoked for an item with `id: "item-1"`,
    *When* it runs, *Then* `transitionStatus("item-1", "idea")` is called (the same
    helper `send_back_idea`/`mark_ready` already use), and on success a toast reading
    `"Unarchived."` is shown (a new `ACTION_SUCCESS_MESSAGES.unarchive` entry, not the
    generic `"Done."` fallback).

**Files**: `web-app/src/components/backlog/BacklogItemDetail.tsx`

##### Task 2.2.2a: Add the `case "unarchive"` dispatch and success message (~3 min)

- In `ACTION_SUCCESS_MESSAGES` (`web-app/src/components/backlog/BacklogItemDetail.tsx:59-75`),
  add: `unarchive: "Unarchived.",` (alongside the existing `archive: "Archived.",`
  entry at line 70).
- In `handleAction`'s switch, add a new case adjacent to `send_back_idea`
  (`:549-551`), since both call the identical underlying transition:
  ```tsx
  case "unarchive":
    await transitionStatus(item.id, "idea");
    break;
  ```
- A dedicated `"unarchive"` action id (rather than reusing `"send_back_idea"`) is used
  so the toast/aria-busy semantics read correctly to the user ("Unarchived." vs. "Sent
  back to triage.") even though the backend call is identical — this was an explicit
  open question in `research/build-vs-buy.md`, resolved here in favor of the clearer
  user-facing copy.
- Files: `web-app/src/components/backlog/BacklogItemDetail.tsx`

### Epic 2.3: Frontend test coverage

**Goal**: New behavior (Unarchive button, archive confirm gate) has Jest coverage,
satisfying requirements.md AC5's frontend half.

#### Story 2.3.1: `ActionsSection` renders and wires the Unarchive button

**Acceptance Criteria**: covered by Story 2.2.1's Given/When/Then above.

**Files**: `web-app/src/components/backlog/detail/ActionsSection.unarchive.test.tsx` (new)

##### Task 2.3.1a: Write `ActionsSection.unarchive.test.tsx` (~5 min)

- New file, following the exact structure of the existing
  `web-app/src/components/backlog/detail/ActionsSection.queuedPlanApproval.test.tsx`
  (same `makeItem` helper shape, same `noop`, same full-props `render(<ActionsSection
  .../>)` calls — `ActionsSection` has no default props, every prop must be passed).
  ```tsx
  import React from "react";
  import { render, screen, fireEvent } from "@testing-library/react";
  import { ActionsSection } from "./ActionsSection";
  import type { BacklogItem } from "@/lib/hooks/useBacklogService";

  function makeItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
    return {
      id: "item-1",
      title: "An archived item",
      status: "archived",
      priority: 2,
      skipPlanning: false,
      skipReviewGate: false,
      autoSpawnSession: false,
      autoCreatePR: false,
      planApproved: false,
      acCriteria: [],
      linkedSessions: [],
      statusEvents: [],
      progressNotes: [],
      totalEstimatedCostUsd: 0,
      ...overrides,
    };
  }

  const noop = () => {};
  const baseProps = {
    actionLoading: null,
    latestWorkSession: undefined,
    showManualReview: false,
    manualReviewOutcome: "PASS",
    manualReviewSummary: "",
    onManualReviewOutcomeChange: noop,
    onManualReviewSummaryChange: noop,
    onManualReviewSubmit: noop,
    onManualReviewCancel: noop,
    terminalState: null as const,
  };

  describe("ActionsSection — archived status Unarchive action", () => {
    it("renders an Unarchive button when the item is archived", () => {
      render(<ActionsSection item={makeItem()} onAction={noop} {...baseProps} />);
      expect(screen.getByTestId("backlog-action-unarchive")).toBeInTheDocument();
    });

    it("calls onAction('unarchive') when clicked", () => {
      const onAction = jest.fn();
      render(<ActionsSection item={makeItem()} onAction={onAction} {...baseProps} />);
      fireEvent.click(screen.getByTestId("backlog-action-unarchive"));
      expect(onAction).toHaveBeenCalledWith("unarchive");
    });

    it("does not render for a non-archived status", () => {
      render(<ActionsSection item={makeItem({ status: "idea" })} onAction={noop} {...baseProps} />);
      expect(screen.queryByTestId("backlog-action-unarchive")).not.toBeInTheDocument();
    });

    it("disables the button while another action is in flight", () => {
      render(
        <ActionsSection item={makeItem()} onAction={noop} {...baseProps} actionLoading="delete" />
      );
      expect(screen.getByTestId("backlog-action-unarchive")).toBeDisabled();
    });
  });
  ```
- Run: `cd web-app && npx jest --no-coverage --testPathPatterns="ActionsSection.unarchive"`
- Files: `web-app/src/components/backlog/detail/ActionsSection.unarchive.test.tsx` (new)

#### Story 2.3.2: `BacklogItemDetail` gates archive behind confirm and dispatches unarchive

**Acceptance Criteria**: covered by Story 2.1.1 and Story 2.2.2's Given/When/Then
above (both the accepted and declined confirm paths).

**Files**: `web-app/src/components/backlog/BacklogItemDetail.archiveConfirmAndUnarchive.test.tsx` (new)

##### Task 2.3.2a: Write the new test file (~5 min)

- New file, copying the mocking scaffold verbatim from the existing
  `web-app/src/components/backlog/BacklogItemDetail.shipPR.test.tsx` (same
  `jest.mock` calls for `SessionMonitor`, `GateVerdictBox`, `TriageReviewPanel`,
  `TriageLoadingIndicator`, `useSessionRepoPaths`, `usePathCompletions`,
  `useSessionService`, `useAnalytics`, `useWatchBacklogItems`, `@/lib/store`,
  `@connectrpc/connect`, `@connectrpc/connect-web`, `useStuckBacklogItems`; same
  `useBacklogService` mock shape with `transitionStatus`, `archiveBacklogItem`,
  `deleteBacklogItem` as individually-referenceable `jest.fn()`s so each test can
  assert on the right one). Add a `beforeEach` that mocks `window.confirm`:
  ```tsx
  import React from "react";
  import { render, screen, act, fireEvent, waitFor } from "@testing-library/react";
  import { BacklogItemDetail } from "./BacklogItemDetail";
  import type { BacklogItem } from "@/lib/hooks/useBacklogService";

  // ... (copy the same jest.mock block from BacklogItemDetail.shipPR.test.tsx verbatim)

  const getBacklogItem = jest.fn();
  const listPipelineModes = jest.fn().mockResolvedValue([]);
  const archiveBacklogItem = jest.fn().mockResolvedValue(undefined);
  const transitionStatus = jest.fn().mockResolvedValue(true);

  jest.mock("@/lib/hooks/useBacklogService", () => ({
    useBacklogService: () => ({
      getBacklogItem,
      transitionStatus,
      triggerTriage: jest.fn(),
      cancelTriage: jest.fn(),
      spawnSessionFromItem: jest.fn(),
      approvePlan: jest.fn(),
      overrideVerdict: jest.fn(),
      triggerReReview: jest.fn(),
      triggerShipPR: jest.fn(),
      submitManualReview: jest.fn(),
      archiveBacklogItem,
      deleteBacklogItem: jest.fn(),
      updateBacklogItem: jest.fn().mockResolvedValue(null),
      listPipelineModes,
      lastError: null,
    }),
  }));

  beforeAll(() => {
    jest.spyOn(console, "error").mockImplementation(() => {});
  });
  afterAll(() => {
    jest.restoreAllMocks();
  });

  function makeDoneItem(overrides: Partial<BacklogItem> = {}): BacklogItem {
    return {
      id: "item-1",
      title: "A done item",
      status: "done",
      priority: 3,
      skipPlanning: false,
      skipReviewGate: false,
      autoSpawnSession: false,
      autoCreatePR: false,
      planApproved: false,
      acCriteria: [],
      linkedSessions: [],
      statusEvents: [],
      progressNotes: [],
      totalEstimatedCostUsd: 0,
      ...overrides,
    };
  }

  async function renderItem(item: BacklogItem) {
    getBacklogItem.mockReset().mockResolvedValue(item);
    archiveBacklogItem.mockClear();
    transitionStatus.mockClear();
    localStorage.clear();
    render(<BacklogItemDetail itemId={item.id} />);
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
  }

  describe("BacklogItemDetail — archive confirmation gate", () => {
    it("archives when the user confirms, with wording that doesn't claim irreversibility", async () => {
      const confirmSpy = jest.spyOn(window, "confirm").mockReturnValue(true);
      await renderItem(makeDoneItem());

      fireEvent.click(screen.getByTestId("backlog-action-archive"));

      expect(confirmSpy).toHaveBeenCalledWith(
        "Archive this item? It will be hidden from the default list, but can be restored later."
      );
      await waitFor(() => expect(archiveBacklogItem).toHaveBeenCalledWith("item-1"));
      confirmSpy.mockRestore();
    });

    it("does not archive when the user declines the confirmation", async () => {
      const confirmSpy = jest.spyOn(window, "confirm").mockReturnValue(false);
      await renderItem(makeDoneItem());

      fireEvent.click(screen.getByTestId("backlog-action-archive"));

      expect(confirmSpy).toHaveBeenCalled();
      expect(archiveBacklogItem).not.toHaveBeenCalled();
      confirmSpy.mockRestore();
    });
  });

  describe("BacklogItemDetail — Unarchive action", () => {
    it("shows the Unarchive button for an archived item and calls transitionStatus(id, 'idea') when clicked", async () => {
      await renderItem(makeDoneItem({ status: "archived" }));

      const button = screen.getByTestId("backlog-action-unarchive");
      expect(button).toBeInTheDocument();
      fireEvent.click(button);

      await waitFor(() => expect(transitionStatus).toHaveBeenCalledWith("item-1", "idea"));
    });

    it("does not show the Unarchive button for a non-archived item", async () => {
      await renderItem(makeDoneItem({ status: "done" }));
      expect(screen.queryByTestId("backlog-action-unarchive")).not.toBeInTheDocument();
    });
  });
  ```
- Run: `cd web-app && npx jest --no-coverage --testPathPatterns="archiveConfirmAndUnarchive"`
- Files: `web-app/src/components/backlog/BacklogItemDetail.archiveConfirmAndUnarchive.test.tsx` (new)

---

## Phase 3: Registry entries and end-to-end coverage

### Epic 3.1: Frontend feature registry entry

**Goal**: Per `.claude/rules/feature-registry.md`, the new UI action is registered.

#### Story 3.1.1: Add a frontend registry entry for the Unarchive action

**Acceptance Criteria**:
- A new per-feature file exists for the Unarchive UI action.
  - *Given* `docs/registry/features/frontend/` contains one file per UI feature (e.g.
    `backlog-stuck-items.json`), *When* this task completes, *Then*
    `docs/registry/features/frontend/backlog-item-unarchive.json` exists with
    `"tested": true` and `testIds` listing the Jest test names and the e2e spec path
    from Epic 3.2.

**Files**: `docs/registry/features/frontend/backlog-item-unarchive.json` (new)

##### Task 3.1.1a: Create the frontend registry entry (~3 min)

- New file, following the shape of
  `docs/registry/features/frontend/backlog-stuck-items.json`:
  ```json
  {
    "id": "backlog-item-unarchive",
    "type": "frontend",
    "component": "ActionsSection",
    "path": "web-app/src/components/backlog/detail/ActionsSection.tsx",
    "markerLine": 1,
    "tested": true,
    "testIds": [
      "tests/e2e/backlog-item-unarchive.spec.ts",
      "ActionsSection — archived status Unarchive action renders an Unarchive button when the item is archived",
      "ActionsSection — archived status Unarchive action calls onAction('unarchive') when clicked",
      "BacklogItemDetail — archive confirmation gate archives when the user confirms, with wording that doesn't claim irreversibility",
      "BacklogItemDetail — Unarchive action shows the Unarchive button for an archived item and calls transitionStatus(id, 'idea') when clicked"
    ],
    "lastModified": "2026-08-29T00:00:00Z"
  }
  ```
- Files: `docs/registry/features/frontend/backlog-item-unarchive.json` (new)

### Epic 3.2: End-to-end coverage

**Goal**: A Playwright spec exercises the full unarchive flow through the real UI,
satisfying `.claude/rules/e2e-test-conventions.md` and the pitfalls research's note
that AC5's Jest coverage doesn't by itself satisfy the e2e layer.

#### Story 3.2.1: E2E spec for archive-confirm and unarchive

**As a** CI reviewer, **I want** an e2e test proving the button works against a real
server, **so that** the feature is verified beyond mocked unit tests.

**Acceptance Criteria** (covers requirements.md AC0, AC1, AC2 end-to-end):
- *Given* a backlog item seeded via `createBacklogItemDirect` then archived via
  `archiveBacklogItemDirect` (both already in `tests/e2e/pages/BacklogMutations.ts`),
  *When* the test navigates to `/backlog?item=<id>` and clicks
  `backlog-action-unarchive`, *Then* the Unarchive button disappears and an
  idea-status action (e.g. `backlog-action-mark-ready`, gated on AC criteria — or more
  robustly, assert via a direct API re-fetch that `status === "idea"` and
  `archivedAt` is unset) confirms the restore.
- *Given* a fresh `idea`-status item, *When* the test clicks
  `backlog-action-send-back-idea`'s equivalent trigger for reaching `done` then clicks
  Archive, *Then* a native `confirm()` dialog appears (Playwright's
  `page.on("dialog", ...)` handler) before the archive completes.

**Files**: `tests/e2e/backlog-item-unarchive.spec.ts` (new)

##### Task 3.2.1a: Write the Playwright spec (~5 min)

- New file, following the annotation/locator/no-`waitForTimeout` conventions already
  used by `tests/e2e/backlog-item-id-deep-link.spec.ts` (deep-link navigation via
  `/backlog?item=${itemId}`) and reusing the existing debug-mutation helpers in
  `tests/e2e/pages/BacklogMutations.ts` (`createBacklogItemDirect`,
  `archiveBacklogItemDirect`) — no new seed endpoint is needed.
  ```ts
  // @feature backlog-item-unarchive

  import { test, expect } from "@playwright/test";
  import { createBacklogItemDirect, archiveBacklogItemDirect } from "./pages/BacklogMutations";

  const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

  test.describe("backlog-item-unarchive", () => {
    test("Unarchive button restores an archived item to idea and clears archived_at", async ({ page, request }) => {
      const itemId = await createBacklogItemDirect(request, { title: `e2e-unarchive-${Date.now()}` });
      await archiveBacklogItemDirect(request, itemId);

      await page.goto(`/backlog?item=${itemId}`, { waitUntil: "domcontentloaded" });
      await expect(page.getByTestId("backlog-action-unarchive")).toBeVisible();

      await page.getByTestId("backlog-action-unarchive").click();
      await expect(page.getByTestId("backlog-action-unarchive")).not.toBeVisible();

      const resp = await request.post(`${BASE_URL}/api/session.v1.BacklogService/GetBacklogItem`, {
        headers: { "Content-Type": "application/json" },
        data: { itemId },
      });
      const body = await resp.json();
      expect(body.item.status).toBe("idea");
      expect(body.item.archivedAt ?? null).toBeNull();
    });

    test("Archive action shows a confirmation dialog before archiving", async ({ page, request }) => {
      const itemId = await createBacklogItemDirect(request, {
        title: `e2e-archive-confirm-${Date.now()}`,
        status: "done",
      });

      await page.goto(`/backlog?item=${itemId}`, { waitUntil: "domcontentloaded" });

      let dialogMessage = "";
      page.once("dialog", async (dialog) => {
        dialogMessage = dialog.message();
        await dialog.dismiss();
      });
      await page.getByTestId("backlog-action-archive").click();
      await expect.poll(() => dialogMessage).toContain("can be restored later");

      // Item must still be "done" — dismissing the confirm must not archive it.
      const resp = await request.post(`${BASE_URL}/api/session.v1.BacklogService/GetBacklogItem`, {
        headers: { "Content-Type": "application/json" },
        data: { itemId },
      });
      const body = await resp.json();
      expect(body.item.status).toBe("done");
    });
  });
  ```
- Verify the exact `GetBacklogItem` response shape (field name casing —
  `archivedAt` vs `archived_at`) against `tests/e2e/pages/BacklogMutations.ts`'s
  existing request/response conventions before finalizing; adjust field access if the
  generated JSON uses snake_case.
- Files: `tests/e2e/backlog-item-unarchive.spec.ts` (new)

### Epic 3.3: Registry generation

#### Story 3.3.1: Regenerate the aggregated registry and verify no new gaps

**Acceptance Criteria**:
- *Given* Phases 1–2's per-feature files are updated/created, *When* `make
  registry-generate` runs, *Then* the aggregated `docs/registry/backend-features.json`
  / `frontend-features.json` reflect the changes and `docs/registry/coverage-gaps.json`'s
  count does not increase.

**Files**: generated — `docs/registry/backend-features.json`,
`docs/registry/frontend-features.json`, `docs/registry/coverage-gaps.json`

##### Task 3.3.1a: Run and verify registry generation (~2 min)

- Run `make registry-generate`.
- Diff `docs/registry/coverage-gaps.json` before/after; confirm the count of untested
  features did not grow (it should shrink by one, since `backlog:transition-status`
  moves from untested to tested).
- Commit the generated diffs alongside the per-feature source file changes.
- Files: `docs/registry/backend-features.json`, `docs/registry/frontend-features.json`,
  `docs/registry/coverage-gaps.json`
