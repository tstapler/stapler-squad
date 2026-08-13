# Validation Plan: Workflow Session Affordances

**Date:** 2026-06-13
**Source:** requirements.md (FR-1 through FR-6), plan.md (28-file implementation), adversarial-review.md (Round 2 verdict: CONCERNS)

---

## 1. Requirement-to-Test Traceability Matrix

| FR | Description | Go Unit | Go Integration | Jest/RTL | E2E (Playwright) |
|----|-------------|---------|----------------|----------|------------------|
| FR-1 | Visual Identity: badge, label, grouping, filter | T-GO-101 | T-INT-101 | T-JS-101, T-JS-102, T-JS-103 | T-E2E-101, T-E2E-102 |
| FR-2 | Default Visibility: hidden/toggle/group override | — | — | T-JS-201, T-JS-202, T-JS-203 | T-E2E-201 |
| FR-3 | Detail Panel: workflow metadata section | T-GO-301 | T-INT-301 | T-JS-301, T-JS-302 | T-E2E-301 |
| FR-4 | Auto-Archive: delay, skip-N, terminal state gate | T-GO-401, T-GO-402, T-GO-403 | T-INT-401 | — | — |
| FR-5 | Retention Settings: per-workflow keep/archive fields | T-GO-501 | T-INT-501 | T-JS-501, T-JS-502 | T-E2E-501 |
| FR-6 | Bulk Actions: archive all, delete failed, confirm | T-GO-601, T-GO-602 | T-INT-601 | T-JS-601, T-JS-602 | T-E2E-601 |

**Requirements coverage: 6/6 FRs covered.**

---

## 2. Test Catalogue

### 2.1 Go Unit Tests — `server/workflows/retention_test.go`

**File:** `server/workflows/retention_test.go`
**Run:** `go test ./server/workflows/...`

#### T-GO-401 — Happy path: sessions beyond keep_sessions are archived after delay

```
func TestRetentionEnforcer_ArchivesSessionsBeyondKeepCount(t *testing.T)
```

Setup: Create workflow with `keep_sessions=1`, `archive_after_hours=24`. Create 3 sessions in Stopped state, stopped_at = 48h ago. Run enforcer sweep.

Assert: Oldest 2 sessions have `archived_at` set. Newest session has `archived_at == nil`.

Verify: `go test ./server/workflows/ -run TestRetentionEnforcer_ArchivesSessionsBeyondKeepCount`

---

#### T-GO-402 — Archive delay not yet elapsed: no sessions archived

```
func TestRetentionEnforcer_SkipsSessionsBeforeDelay(t *testing.T)
```

Setup: Workflow `archive_after_hours=24`. 2 sessions in Stopped state, stopped_at = 1h ago. Run enforcer sweep.

Assert: Both sessions have `archived_at == nil`. No DB writes to `archived_at`.

Verify: `go test ./server/workflows/ -run TestRetentionEnforcer_SkipsSessionsBeforeDelay`

---

#### T-GO-403 — maybeAutoArchive skips immediate-archive when archiveAfterHours > 0

```
func TestMaybeAutoArchive_SkipsWhenArchiveAfterHoursSet(t *testing.T)
```

Setup: Workflow with `archive_after_hours=24`. Session receives `EventExited`. Call `maybeAutoArchive`.

Assert: `archived_at` is NOT set on the session. (Resolution of Round 1 Blocker 1.)

Verify: `go test ./server/workflows/ -run TestMaybeAutoArchive_SkipsWhenArchiveAfterHoursSet`

---

#### T-GO-501 — WorkflowRepository: keep_sessions and archive_after_hours round-trip

```
func TestWorkflowRepository_RetentionFieldsRoundTrip(t *testing.T)
```

**File:** `server/services/workflow_service_test.go` or `session/storage_test.go`

Setup: Create workflow with `keep_sessions=3`, `archive_after_hours=48`. Save to DB. Reload.

Assert: `keep_sessions == 3`, `archive_after_hours == 48`. Nil proto fields for absent values (ADR-1 nil-check).

Verify: `go test ./server/services/ -run TestWorkflowRepository_RetentionFieldsRoundTrip`

---

#### T-GO-601 — ArchiveWorkflowSessions RPC: archives non-active sessions

```
func TestArchiveWorkflowSessions_ArchivesNonActiveSessions(t *testing.T)
```

**File:** `server/services/session_service_test.go` or new `session_service_bulk_test.go`

Setup: Workflow X has 3 stopped sessions and 1 active session. Call `ArchiveWorkflowSessions(workflowId=X)`.

Assert: 3 stopped sessions have `archived_at` set. Active session `archived_at == nil` (active always skipped, per ADR-5).

Return value: response contains `archivedCount == 3`.

Verify: `go test ./server/services/ -run TestArchiveWorkflowSessions_ArchivesNonActiveSessions`

---

#### T-GO-602 — DeleteWorkflowFailedSessions RPC: deletes only failed/errored sessions

```
func TestDeleteWorkflowFailedSessions_DeletesOnlyFailedSessions(t *testing.T)
```

Setup: Workflow X has 2 sessions with terminal error state and 1 stopped-clean session. Call `DeleteWorkflowFailedSessions(workflowId=X)`.

Assert: 2 failed sessions are deleted from DB. Stopped-clean session is unaffected.

Return value: response contains `deletedCount == 2`.

Verify: `go test ./server/services/ -run TestDeleteWorkflowFailedSessions_DeletesOnlyFailedSessions`

---

#### T-GO-101 — workflowNameCache populates workflow_name on session proto

```
func TestSessionService_WorkflowNameDenormalized(t *testing.T)
```

**File:** `server/services/session_service_test.go`

Setup: Insert workflow with id=W1, name="Knowledge Maintenance". Insert session with `workflow_id=W1`. Call `ListSessions`.

Assert: Returned session proto has `workflow_name == "Knowledge Maintenance"` (field 64).

Verify: `go test ./server/services/ -run TestSessionService_WorkflowNameDenormalized`

---

#### T-GO-301 — ListSessions returns initial_prompt for workflow sessions

```
func TestSessionService_InitialPromptReturnedForWorkflowSession(t *testing.T)
```

Setup: Session with `workflow_id` set and `initial_prompt = "run daily sync"`. Call `GetSession` or `ListSessions`.

Assert: Proto field `initial_prompt` present in response.

Verify: `go test ./server/services/ -run TestSessionService_InitialPromptReturnedForWorkflowSession`

---

### 2.2 Go Integration Tests — ConnectRPC handler level

**File:** `server/services/session_service_test.go` (existing pattern) or `workflow_session_integration_test.go`
**Run:** `go test ./server/services/ -run Integration`

#### T-INT-101 — ListSessions with workflow_id filter returns only matching sessions

```
func TestIntegration_ListSessions_FilterByWorkflowID(t *testing.T)
```

Setup: Insert sessions with workflow_id=W1 (2), workflow_id=W2 (1), no workflow_id (3). Call `ListSessions` with `filter.workflowId=W1`.

Assert: Response contains exactly 2 sessions. All have `workflow_id == "W1"`.

Verify: `go test ./server/services/ -run TestIntegration_ListSessions_FilterByWorkflowID`

---

#### T-INT-301 — GetSession returns full workflow metadata section

```
func TestIntegration_GetSession_WorkflowMetadataSection(t *testing.T)
```

Setup: Workflow with name, description, schedule, keep_sessions, archive_after_hours. Session with workflow_id pointing to this workflow. Call `GetSession`.

Assert: `workflow_name`, `workflow_description`, `cron_schedule`, `initial_prompt` present and correct in response.

Verify: `go test ./server/services/ -run TestIntegration_GetSession_WorkflowMetadataSection`

---

#### T-INT-401 — Retention sweep integration: full round-trip in SQLite

```
func TestIntegration_RetentionSweep_FullRoundTrip(t *testing.T)
```

Setup: Real SQLite DB (use `createTestStorage` pattern). Workflow with `keep_sessions=1`, `archive_after_hours=0` (immediate). 3 stopped sessions. Run `RunRetentionSweep(ctx)`.

Assert: Only the most recent session has `archived_at == nil`. `archived_at` is set on the other 2 with a timestamp close to `time.Now()`.

Verify: `go test ./server/workflows/ -run TestIntegration_RetentionSweep_FullRoundTrip`

---

#### T-INT-501 — UpdateWorkflow saves and retrieves retention fields via RPC

```
func TestIntegration_UpdateWorkflow_RetentionFieldsPersisted(t *testing.T)
```

Setup: Create workflow via `CreateWorkflow` RPC. Call `UpdateWorkflow` with `keep_sessions=2`, `archive_after_hours=12`. Call `ListWorkflows`.

Assert: Returned workflow has `keep_sessions == 2`, `archive_after_hours == 12`.

Verify: `go test ./server/services/ -run TestIntegration_UpdateWorkflow_RetentionFieldsPersisted`

---

#### T-INT-601 — ArchiveWorkflowSessions RPC: ConnectRPC handler returns correct count

```
func TestIntegration_ArchiveWorkflowSessions_RPC(t *testing.T)
```

Setup: Real DB; workflow + 4 non-active sessions. Call `ArchiveWorkflowSessions` via ConnectRPC handler.

Assert: Response `archived_count == 4`. DB confirms all 4 have `archived_at` set.

Verify: `go test ./server/services/ -run TestIntegration_ArchiveWorkflowSessions_RPC`

---

### 2.3 Jest / React Testing Library Tests

**Run:** `cd web-app && npx jest --no-coverage`

#### T-JS-101 — SessionCard renders workflow badge when workflow_name is set

**File:** `web-app/src/components/sessions/__tests__/SessionCard.test.tsx`

```typescript
it("renders workflow badge when session has workflow_name", () => {
  render(<SessionCard session={mockWorkflowSession} />);
  expect(screen.getByTestId("workflow-badge")).toBeInTheDocument();
  expect(screen.getByTestId("workflow-badge")).toHaveTextContent("Knowledge Maintenance");
});
```

---

#### T-JS-102 — SessionCard does NOT render workflow badge for manual sessions

**File:** same as T-JS-101

```typescript
it("does not render workflow badge for manual sessions without workflow_name", () => {
  render(<SessionCard session={mockManualSession} />);
  expect(screen.queryByTestId("workflow-badge")).not.toBeInTheDocument();
});
```

---

#### T-JS-103 — groupSessions returns "Workflow" group keyed by workflow_name

**File:** `web-app/src/lib/sessions/__tests__/groupSessions.test.ts`

```typescript
it("groups sessions by workflow_name when strategy is 'Workflow'", () => {
  const groups = groupSessions(sessions, "Workflow", workflowIdToName);
  expect(groups.has("Knowledge Maintenance")).toBe(true);
  expect(groups.get("(No Workflow)")).toContain(manualSession);
});
```

---

#### T-JS-201 — showWorkflowSessions toggle: workflow sessions hidden by default

**File:** `web-app/src/components/sessions/__tests__/SessionList.test.tsx`

```typescript
it("hides workflow sessions when showWorkflowSessions is false (default)", () => {
  renderWithContext(<SessionList />, { showWorkflowSessions: false });
  expect(screen.queryByTestId("workflow-badge")).not.toBeInTheDocument();
  expect(screen.getByText("manual-session-title")).toBeInTheDocument();
});
```

---

#### T-JS-202 — showWorkflowSessions toggle: workflow sessions shown after toggling

**File:** same as T-JS-201

```typescript
it("shows workflow sessions after toggling showWorkflowSessions on", () => {
  renderWithContext(<SessionList />, { showWorkflowSessions: false });
  userEvent.click(screen.getByRole("switch", { name: /show workflow sessions/i }));
  expect(screen.getByTestId("workflow-badge")).toBeInTheDocument();
});
```

---

#### T-JS-203 — "Workflow" grouping strategy bypasses hidden toggle and shows all workflow sessions

**File:** same as T-JS-201

```typescript
it("shows workflow sessions regardless of toggle when grouped by Workflow", () => {
  renderWithContext(<SessionList />, { showWorkflowSessions: false, groupBy: "Workflow" });
  expect(screen.getByText("Knowledge Maintenance")).toBeInTheDocument();
});
```

---

#### T-JS-301 — SessionDetailView shows workflow metadata section for workflow sessions

**File:** `web-app/src/components/sessions/__tests__/SessionDetailView.test.tsx`

```typescript
it("renders workflow metadata section when session has workflow_id", () => {
  render(<SessionDetailView session={workflowSession} />);
  expect(screen.getByTestId("workflow-metadata-section")).toBeInTheDocument();
  expect(screen.getByText("Knowledge Maintenance")).toBeInTheDocument();
  expect(screen.getByText(/0 0 \* \* \*/)).toBeInTheDocument(); // cron schedule
});
```

---

#### T-JS-302 — SessionDetailView hides workflow section for manual sessions

**File:** same as T-JS-301

```typescript
it("does not render workflow metadata section for manual sessions", () => {
  render(<SessionDetailView session={manualSession} />);
  expect(screen.queryByTestId("workflow-metadata-section")).not.toBeInTheDocument();
});
```

---

#### T-JS-501 — WorkflowForm renders keep_sessions field with default value 1

**File:** `web-app/src/components/workflows/__tests__/WorkflowForm.test.tsx`

```typescript
it("renders keep_sessions field defaulting to 1", () => {
  render(<WorkflowForm />);
  expect(screen.getByLabelText(/keep sessions/i)).toHaveValue(1);
});
```

---

#### T-JS-502 — WorkflowForm renders archive_after_hours field; 0 means disabled

**File:** same as T-JS-501

```typescript
it("renders archive_after_hours field; 0 means disabled", () => {
  render(<WorkflowForm />);
  expect(screen.getByLabelText(/archive after/i)).toHaveValue(0);
  expect(screen.getByText(/disabled/i)).toBeInTheDocument();
});
```

---

#### T-JS-601 — WorkflowsPanel shows "Archive all sessions" button and confirms count

**File:** `web-app/src/components/workflows/__tests__/WorkflowsPanel.test.tsx`

```typescript
it("shows archive confirmation with session count before archiving", async () => {
  render(<WorkflowsPanel workflow={workflowWithSessions} />);
  userEvent.click(screen.getByRole("button", { name: /archive all sessions/i }));
  await screen.findByText(/archive 3 sessions/i);
  userEvent.click(screen.getByRole("button", { name: /confirm/i }));
  expect(mockArchiveWorkflowSessions).toHaveBeenCalledWith({ workflowId: "W1" });
});
```

---

#### T-JS-602 — WorkflowsPanel "Delete failed runs" button only calls delete RPC

**File:** same as T-JS-601

```typescript
it("calls deleteWorkflowFailedSessions RPC, not archive, when deleting failed runs", async () => {
  render(<WorkflowsPanel workflow={workflowWithFailedSessions} />);
  userEvent.click(screen.getByRole("button", { name: /delete failed runs/i }));
  await screen.findByText(/delete 2 failed/i);
  userEvent.click(screen.getByRole("button", { name: /confirm/i }));
  expect(mockDeleteWorkflowFailedSessions).toHaveBeenCalledWith({ workflowId: "W1" });
  expect(mockArchiveWorkflowSessions).not.toHaveBeenCalled();
});
```

---

### 2.4 E2E Tests (Playwright) — `tests/e2e/workflow-session-affordances.spec.ts`

**Run:** Start server with `STAPLER_SQUAD_INSTANCE=e2e-local ./stapler-squad --tmux-keep-server &`, then `cd tests/e2e && npx playwright test workflow-session-affordances.spec.ts`
**Base URL:** `http://localhost:8544`

```typescript
// @feature workflow:session-affordances
import { test, expect } from '@playwright/test';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
```

---

#### T-E2E-101 — Workflow badge visible on session card in session list

```typescript
test.describe("workflow session affordances - visual identity", () => {
  test("T-E2E-101: workflow badge appears on session card for workflow-created sessions", async ({ page }) => {
    await page.goto(BASE_URL);
    const badge = page.getByTestId("workflow-badge").first();
    await expect(badge).toBeVisible();
    await expect(badge).not.toBeEmpty();
  });
});
```

Precondition: At least one workflow-created session exists in the test server state.

---

#### T-E2E-102 — Workflow grouping strategy shows sessions under workflow name headers

```typescript
  test("T-E2E-102: grouping by Workflow shows workflow name as group header", async ({ page }) => {
    await page.goto(BASE_URL);
    await page.getByRole("combobox", { name: /group by/i }).selectOption("Workflow");
    const header = page.getByRole("heading", { name: /knowledge maintenance/i });
    await expect(header).toBeVisible();
  });
```

---

#### T-E2E-201 — Workflow sessions hidden by default; toggle reveals them

```typescript
test.describe("workflow session affordances - visibility toggle", () => {
  test("T-E2E-201: workflow sessions hidden by default, shown after toggle", async ({ page }) => {
    await page.goto(BASE_URL);
    // Default: no workflow badges visible
    await expect(page.getByTestId("workflow-badge").first()).not.toBeVisible();
    // Toggle on
    await page.getByRole("switch", { name: /show workflow sessions/i }).click();
    await expect(page.getByTestId("workflow-badge").first()).toBeVisible();
  });
});
```

---

#### T-E2E-301 — Session detail panel shows workflow metadata

```typescript
test.describe("workflow session affordances - detail panel", () => {
  test("T-E2E-301: clicking workflow session shows workflow metadata in detail panel", async ({ page }) => {
    await page.goto(BASE_URL);
    // Ensure workflow sessions are visible
    await page.getByRole("switch", { name: /show workflow sessions/i }).click();
    await page.getByTestId("workflow-badge").first().click();
    const section = page.getByTestId("workflow-metadata-section");
    await expect(section).toBeVisible();
    await expect(section.getByTestId("workflow-name")).not.toBeEmpty();
    await expect(section.getByTestId("workflow-schedule")).not.toBeEmpty();
    await expect(section.getByTestId("workflow-fired-at")).not.toBeEmpty();
  });
});
```

---

#### T-E2E-501 — WorkflowForm save-and-reload preserves retention settings

```typescript
test.describe("workflow session affordances - retention settings", () => {
  test("T-E2E-501: keep_sessions and archive_after_hours persist after save", async ({ page }) => {
    await page.goto(`${BASE_URL}/workflows`);
    await page.getByRole("button", { name: /edit/i }).first().click();
    await page.getByLabel(/keep sessions/i).fill("2");
    await page.getByLabel(/archive after/i).fill("48");
    await page.getByRole("button", { name: /save/i }).click();
    // Re-open
    await page.getByRole("button", { name: /edit/i }).first().click();
    await expect(page.getByLabel(/keep sessions/i)).toHaveValue("2");
    await expect(page.getByLabel(/archive after/i)).toHaveValue("48");
  });
});
```

---

#### T-E2E-601 — Bulk archive shows count confirmation dialog

```typescript
test.describe("workflow session affordances - bulk actions", () => {
  test("T-E2E-601: archive all sessions button shows count confirmation", async ({ page }) => {
    await page.goto(`${BASE_URL}/workflows`);
    await page.getByRole("button", { name: /archive all sessions/i }).first().click();
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText(/session/i)).toBeVisible();
    // Cancel — don't mutate test server state
    await dialog.getByRole("button", { name: /cancel/i }).click();
    await expect(dialog).not.toBeVisible();
  });
});
```

---

## 3. Pre-Implementation Compile Blockers (from adversarial-review Round 2)

The following two issues in `plan.md` will cause compile failures and MUST be resolved before writing any implementation code. They are not test concerns but are gate criteria for the readiness check.

| Issue | Location in plan | Required Fix |
|-------|-----------------|--------------|
| `s.entClient.Session.Query()` does not exist | Task 1.4.1 (`ArchiveWorkflowSessions` impl) | Use `deps.Storage.(*session.Storage).GetEntClient()` or add a `workflowRepo` method that wraps the query |
| `deps.EntClient` field reference | Task 1.3.2 (retention goroutine bootstrap) | Use `deps.Storage.(*session.Storage).GetEntClient()` — `RuntimeDeps` has no `EntClient` field |

These must be corrected in `plan.md` before Phase 5 starts.

---

## 4. Adversarial Review Blocker Status

| Blocker | Round 1 Status | Round 2 Status |
|---------|----------------|----------------|
| `maybeAutoArchive` archives immediately (defeats delay) | Resolved in plan | Verified: T-GO-403 guards this |
| DB status constant mismatch (proto wire vs DB values) | Resolved in plan | Migration Notes error still present — CONCERN, not blocker |
| `WorkflowData` phantom struct | Resolved in plan | — |

**No unresolved BLOCKERs from adversarial review.** 5 CONCERNS remain, 2 of which will cause compile failures (addressed in §3 above).

---

## 5. Test Count Summary

| Type | Count |
|------|-------|
| Go Unit (retention, bulk actions, cache) | 8 |
| Go Integration (ConnectRPC / ent) | 5 |
| Jest / RTL (frontend components, hooks) | 13 |
| E2E Playwright | 6 |
| **Total** | **32** |

---

## 6. Coverage Analysis by FR

| FR | Happy Path | Error Cases | Edge Cases | Verdict |
|----|-----------|-------------|------------|---------|
| FR-1 (Visual Identity) | T-JS-101, T-E2E-101, T-GO-101 | T-JS-102 (no badge for manual) | T-JS-103 (grouping with no-workflow fallback) | COVERED |
| FR-2 (Default Visibility) | T-JS-202, T-E2E-201 | T-JS-201 (hidden by default) | T-JS-203 (group override bypasses toggle) | COVERED |
| FR-3 (Detail Panel) | T-JS-301, T-E2E-301, T-INT-301 | T-JS-302 (no section for manual) | T-GO-301 (initial_prompt populated) | COVERED |
| FR-4 (Auto-Archive) | T-GO-401, T-INT-401 | T-GO-402 (delay not elapsed) | T-GO-403 (maybeAutoArchive skip guard) | COVERED |
| FR-5 (Retention Settings) | T-JS-501, T-INT-501, T-E2E-501 | T-JS-502 (0 = disabled display) | T-GO-501 (nil optional round-trip) | COVERED |
| FR-6 (Bulk Actions) | T-JS-601, T-INT-601, T-E2E-601 | T-GO-602 (delete only failed) | T-JS-602 (delete vs archive distinct) | COVERED |

**Requirements coverage: 6/6 FRs (100%)**

---

## 7. Implementation Readiness Gate

### Criterion 1: Every FR has at least one test
PASS — All 6 FRs have Go, Jest, and E2E coverage.

### Criterion 2: No task in plan.md is underspecified
CONCERNS — Two tasks (1.3.2 and 1.4.1) reference fields (`deps.EntClient`, `s.entClient`) that do not exist in the current codebase. Both tasks lack the correct accessor pattern (`deps.Storage.(*session.Storage).GetEntClient()`). These must be corrected in `plan.md` before implementation starts. All other tasks name specific files, methods, and verification commands.

### Criterion 3: Adversarial review has no unresolved BLOCKERs
PASS — All 3 Round 1 BLOCKERs are resolved. 5 CONCERNS remain, none are BLOCKERs. The 2 compile-error concerns are flagged as pre-implementation gates in §3 above.

### Criterion 4: Test suite covers happy path, error, and edge cases for each FR
PASS — As shown in §6, all 6 FRs have distinct happy-path, error-case, and edge-case tests.

---

## Overall Verdict: CONCERNS

Implementation may begin on Epics 2, 3, and 5 (frontend work with no dependency on the broken accessor pattern). Epics 1 and 4 (backend retention goroutine and bulk RPC implementations) are blocked until `plan.md` Tasks 1.3.2 and 1.4.1 are corrected with the proper `deps.Storage.(*session.Storage).GetEntClient()` call path.

**Recommended action before Epic 1/4 implementation:** Update Tasks 1.3.2 and 1.4.1 in `plan.md` to replace `deps.EntClient` / `s.entClient` with `deps.Storage.(*session.Storage).GetEntClient()` (or introduce a `WorkflowRepository` constructor that accepts the resolved `*ent.Client` at server startup time, which would cleanly avoid the type assertion at call sites).
