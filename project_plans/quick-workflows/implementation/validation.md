# Quick Workflows — Validation Plan

**Feature branch:** `stapler-squad-quick-workflows`
**Plan date:** 2026-06-11
**Test IDs start from:** T-UNIT-GO-001, T-INT-GO-001, T-UNIT-TS-018 (continuing from existing T-UNIT-TS-017), T-PITFALL-003 (continuing from T-PITFALL-002), T-E2E-001

---

## Requirement-to-Test Traceability Matrix

| User Story | Test IDs |
|---|---|
| US-01 Omnibar Invocation | T-UNIT-TS-018 through T-UNIT-TS-031, T-PITFALL-003 through T-PITFALL-007, T-E2E-003, T-E2E-004 |
| US-02 Workflow Management Panel | T-UNIT-TS-032 through T-UNIT-TS-039, T-INT-GO-005 through T-INT-GO-008, T-E2E-001, T-E2E-002, T-E2E-005 |
| US-03 Workflow Scheduling (Cron) | T-UNIT-GO-007 through T-UNIT-GO-011, T-INT-GO-009, T-E2E-006 |
| US-04 Workflow Definition Schema | T-UNIT-GO-001 through T-UNIT-GO-006, T-INT-GO-001 through T-INT-GO-004 |
| US-05 Storage | T-INT-GO-001 through T-INT-GO-004, T-UNIT-GO-001, T-UNIT-GO-002 |

---

## Test Suite

### Go Unit Tests

---

ID: T-UNIT-GO-001
Requirement: US-04, US-05
Description: ValidateWorkflowSlug rejects slugs shorter than 2 chars, longer than 64, leading/trailing hyphens, consecutive hyphens, and uppercase letters; accepts valid slugs like "knowledge-sync" and "ab"
Type: unit
File: `session/workflow_slug_test.go`

---

ID: T-UNIT-GO-002
Requirement: US-04, US-05
Description: ValidateWorkflowSlug returns nil for all valid patterns: single hyphen in middle, all lowercase alphanum, numeric suffix, long 64-char slug
Type: unit
File: `session/workflow_slug_test.go`

---

ID: T-UNIT-GO-003
Requirement: US-03
Description: validateCronExpression in server/workflows accepts standard 5-field expressions ("0 8 * * 1", "*/5 * * * *") and rejects 6-field expressions with seconds, malformed strings, and empty string
Type: unit
File: `server/workflows/scheduler_test.go`

---

ID: T-UNIT-GO-004
Requirement: US-02, US-04
Description: entWorkflowToProto correctly maps all ent.Workflow fields to WorkflowProto: UUID string conversion, optional field pass-through (empty string for nil optionals), timestamp conversion via timestamppb.New
Type: unit
File: `server/services/workflow_service_test.go`

---

ID: T-UNIT-GO-005
Requirement: US-02
Description: CreateWorkflow handler returns CodeInvalidArgument when slug is invalid (blank, leading hyphen, uppercase)
Type: unit
File: `server/services/workflow_service_test.go`

---

ID: T-UNIT-GO-006
Requirement: US-02, US-04
Description: CreateWorkflow handler returns CodeInvalidArgument when command is blank or targetDirectory is blank (ADR-9: handler-level enforcement of requirement)
Type: unit
File: `server/services/workflow_service_test.go`

---

ID: T-UNIT-GO-007
Requirement: US-03
Description: CreateWorkflow handler returns CodeInvalidArgument when cronExpression is non-empty and invalid; accepts valid 5-field expression without error
Type: unit
File: `server/services/workflow_service_test.go`

---

ID: T-UNIT-GO-008
Requirement: US-02
Description: CreateWorkflow handler returns CodeAlreadyExists (not an opaque 500) when repo.Create returns ent.ConstraintError for duplicate slug
Type: unit
File: `server/services/workflow_service_test.go`

---

ID: T-UNIT-GO-009
Requirement: US-02
Description: UpdateWorkflow handler returns CodeNotFound when repo.GetByID returns ent.NotFoundError
Type: unit
File: `server/services/workflow_service_test.go`

---

ID: T-UNIT-GO-010
Requirement: US-02
Description: DeleteWorkflow handler calls scheduler.Remove(workflowID) after repo.Delete succeeds; verifies the mock scheduler was called with the correct ID
Type: unit
File: `server/services/workflow_service_test.go`

---

ID: T-UNIT-GO-011
Requirement: US-03
Description: RunWorkflow handler delegates to scheduler.FireNow(ctx, wf, arg) with the arg from the request; returns the session ID from FireNow in the response; does NOT call sessionSvc.CreateSession directly
Type: unit
File: `server/services/workflow_service_test.go`

---

ID: T-UNIT-GO-012
Requirement: US-03
Description: WorkflowScheduler.FireNow interpolates {{input}} placeholder in inputTemplate with the provided arg; falls back to raw arg when inputTemplate is empty
Type: unit
File: `server/workflows/scheduler_test.go`

---

ID: T-UNIT-GO-013
Requirement: US-03
Description: WorkflowScheduler.FireNow sets oneOff=true when workflow.SessionType == "one_off"; sets oneOff=false for "directory" session type
Type: unit
File: `server/workflows/scheduler_test.go`

---

ID: T-UNIT-GO-014
Requirement: US-03
Description: WorkflowScheduler.Reload removes the existing cron entry ID and adds a new one when called with an already-registered workflow; entry map is updated
Type: unit
File: `server/workflows/scheduler_test.go`

---

ID: T-UNIT-GO-015
Requirement: US-03
Description: WorkflowScheduler.Reload does not add a cron entry when workflow.CronEnabled is false; calls c.Remove if an entry previously existed
Type: unit
File: `server/workflows/scheduler_test.go`

---

ID: T-UNIT-GO-016
Requirement: US-03
Description: WorkflowScheduler.Remove deletes the entry from the cron engine and removes it from entryMap; subsequent Remove on same ID is a no-op (not a panic)
Type: unit
File: `server/workflows/scheduler_test.go`

---

ID: T-UNIT-GO-017
Requirement: US-03
Description: WorkflowScheduler.Start calls repo.ListEnabled and registers a cron entry for every enabled workflow; verifies correct cron expressions are used
Type: unit
File: `server/workflows/scheduler_test.go`

---

ID: T-UNIT-GO-018
Requirement: US-03
Description: WorkflowScheduler.FireNow publishes an event to the event bus on session creation success; verifies the event bus mock was called with workflow name and session ID
Type: unit
File: `server/workflows/scheduler_test.go`

---

### Go Integration Tests (real SQLite via ent test client)

---

ID: T-INT-GO-001
Requirement: US-04, US-05
Description: EntWorkflowRepository.Create persists a workflow to SQLite and returns a record with auto-generated UUID, populated timestamps, and all provided fields
Type: integration
File: `session/ent_workflow_repository_test.go`

---

ID: T-INT-GO-002
Requirement: US-05
Description: EntWorkflowRepository.Create returns ent.ConstraintError on duplicate slug; second Create with same slug produces the error without panicking
Type: integration
File: `session/ent_workflow_repository_test.go`

---

ID: T-INT-GO-003
Requirement: US-05
Description: EntWorkflowRepository.Update applies partial update (only non-nil pointer fields); verifies unchanged fields are preserved; UpdatedAt is advanced
Type: integration
File: `session/ent_workflow_repository_test.go`

---

ID: T-INT-GO-004
Requirement: US-05
Description: EntWorkflowRepository.ListEnabled returns only records where cron_enabled=true; ListAll returns all records sorted ascending by created_at
Type: integration
File: `session/ent_workflow_repository_test.go`

---

ID: T-INT-GO-005
Requirement: US-02
Description: CreateWorkflow RPC end-to-end: sends valid CreateWorkflowRequest to a real SessionService backed by in-memory SQLite; response contains WorkflowProto with correct fields; database contains one row
Type: integration
File: `server/services/workflow_service_integration_test.go`

---

ID: T-INT-GO-006
Requirement: US-02
Description: ListWorkflows RPC returns all created workflows in insertion order; empty list response when no workflows exist (not an error)
Type: integration
File: `server/services/workflow_service_integration_test.go`

---

ID: T-INT-GO-007
Requirement: US-02
Description: UpdateWorkflow RPC mutates only provided optional fields; verify unchanged fields are preserved; verify scheduler.Reload is called via mock
Type: integration
File: `server/services/workflow_service_integration_test.go`

---

ID: T-INT-GO-008
Requirement: US-02
Description: DeleteWorkflow RPC removes the workflow; subsequent GetByID via ListWorkflows returns empty list; scheduler.Remove is called via mock
Type: integration
File: `server/services/workflow_service_integration_test.go`

---

ID: T-INT-GO-009
Requirement: US-03
Description: RunWorkflow RPC (with mock scheduler.FireNow) returns a session ID; verify the arg from RunWorkflowRequest.Arg is passed through to FireNow unchanged
Type: integration
File: `server/services/workflow_service_integration_test.go`

---

### Jest / React Testing Library Tests (TypeScript frontend)

---

ID: T-UNIT-TS-018
Requirement: US-01
Description: WorkflowDetector.detect returns InputType.Workflow with confidence 1.0 and workflowFound=true for "@known-slug" matching a loaded workflow
Type: unit
File: `web-app/src/lib/omnibar/detectors/WorkflowDetector.test.ts`

---

ID: T-UNIT-TS-019
Requirement: US-01
Description: WorkflowDetector.detect returns interpolatedPrompt with {{input}} substituted when "@known-slug https://example.com" is input and workflow has inputTemplate
Type: unit
File: `web-app/src/lib/omnibar/detectors/WorkflowDetector.test.ts`

---

ID: T-UNIT-TS-020
Requirement: US-01
Description: WorkflowDetector.detect returns InputType.Workflow with confidence 0.4 and workflowFound=false for "@unknown-slug" not in the loaded workflow list
Type: unit
File: `web-app/src/lib/omnibar/detectors/WorkflowDetector.test.ts`

---

ID: T-UNIT-TS-021
Requirement: US-01
Description: WorkflowDetector.detect returns null for GitHub URL "https://github.com/owner/repo" — does not claim URLs
Type: unit
File: `web-app/src/lib/omnibar/detectors/WorkflowDetector.test.ts`

---

ID: T-UNIT-TS-022
Requirement: US-01
Description: WorkflowDetector.detect returns null for local path "/path/to/dir" — does not claim absolute paths
Type: unit
File: `web-app/src/lib/omnibar/detectors/WorkflowDetector.test.ts`

---

ID: T-UNIT-TS-023
Requirement: US-01
Description: WorkflowDetector.detect returns null for bare "@" with no slug following it
Type: unit
File: `web-app/src/lib/omnibar/detectors/WorkflowDetector.test.ts`

---

ID: T-UNIT-TS-024
Requirement: US-01
Description: WorkflowDetector.detect matches case-insensitively: "@Knowledge-Sync" matches slug "knowledge-sync"; result metadata slug is normalized to lowercase
Type: unit
File: `web-app/src/lib/omnibar/detectors/WorkflowDetector.test.ts`

---

ID: T-UNIT-TS-025
Requirement: US-01
Description: WorkflowDetector.detect strips trailing whitespace from "@slug " (space after slug with no arg); still resolves to the correct workflow
Type: unit
File: `web-app/src/lib/omnibar/detectors/WorkflowDetector.test.ts`

---

ID: T-UNIT-TS-026
Requirement: US-01
Description: WorkflowDetector.detect sets workflowArg to the full trailing text when "@slug multiple word arg" is entered; interpolatedPrompt substitutes the full arg
Type: unit
File: `web-app/src/lib/omnibar/detectors/WorkflowDetector.test.ts`

---

ID: T-UNIT-TS-027
Requirement: US-01
Description: WorkflowDetector.detect uses the workflow's targetDirectory and sessionType from the WorkflowEntry in the result metadata
Type: unit
File: `web-app/src/lib/omnibar/detectors/WorkflowDetector.test.ts`

---

ID: T-UNIT-TS-028
Requirement: US-01
Description: dispatchOmnibarAction with type "run_workflow" calls deps.runWorkflow(workflowSlug, workflowArg) and deps.close(); does not call any session create method directly
Type: unit
File: `web-app/src/lib/omnibar/actions/dispatch.test.ts`

---

ID: T-UNIT-TS-029
Requirement: US-01
Description: dispatchOmnibarAction with type "run_workflow" silently no-ops (no throw) when deps.runWorkflow is absent (optional dep pattern matching spawnShell?)
Type: unit
File: `web-app/src/lib/omnibar/actions/dispatch.test.ts`

---

ID: T-UNIT-TS-030
Requirement: US-01
Description: TypeScript compile check: adding InputType.Workflow to the enum without updating INPUT_TYPE_INFO causes tsc --noEmit to fail; adding both atomically passes. Verified via the build step, not a Jest test — documented here for traceability.
Type: unit
File: `web-app/src/lib/omnibar/types.ts` (verified by `cd web-app && npx tsc --noEmit`)

---

ID: T-UNIT-TS-031
Requirement: US-01
Description: OmnibarContext registers WorkflowDetector into the default registry when workflows list is non-empty; cleanup unregisters it; useEffect dependency on workflows array causes re-registration when workflows change
Type: unit
File: `web-app/src/lib/contexts/OmnibarContext.test.tsx`

---

ID: T-PITFALL-003
Requirement: US-01
Description: DetectorRegistry pitfall guard: "@knowledge-sync" MUST resolve to WorkflowDetector (InputType.Workflow) and NOT to PathWithBranchDetector; verified by running full registry.detect() against a registry that includes WorkflowDetector at priority 25
Type: unit
File: `web-app/src/lib/omnibar/detector.test.ts`

---

ID: T-PITFALL-004
Requirement: US-01
Description: DetectorRegistry pitfall guard: "https://github.com/owner/repo/pull/42" MUST resolve to GitHubPRDetector (InputType.GitHubPR) and NOT to WorkflowDetector — GitHub URL takes priority at 10
Type: unit
File: `web-app/src/lib/omnibar/detector.test.ts`

---

ID: T-PITFALL-005
Requirement: US-01
Description: DetectorRegistry pitfall guard: "new:/some/path" MUST resolve to NewSessionDetector (priority 35) and NOT WorkflowDetector (priority 25); confirms no cross-detection
Type: unit
File: `web-app/src/lib/omnibar/detector.test.ts`

---

ID: T-PITFALL-006
Requirement: US-01
Description: DetectorRegistry pitfall guard: "owner/repo" shorthand MUST resolve to GitHubShorthandDetector and NOT WorkflowDetector
Type: unit
File: `web-app/src/lib/omnibar/detector.test.ts`

---

ID: T-PITFALL-007
Requirement: US-01
Description: DetectorRegistry pitfall guard: existing session search input ("my session name") MUST still resolve to SessionSearchDetector (priority 200) after WorkflowDetector is registered — confirms no regression in fallback behavior
Type: unit
File: `web-app/src/lib/omnibar/detector.test.ts`

---

ID: T-UNIT-TS-032
Requirement: US-02
Description: useWorkflows hook fetches workflows on mount and returns them in the workflows array; loading is true before fetch resolves, false after
Type: unit
File: `web-app/src/lib/hooks/useWorkflows.test.ts`

---

ID: T-UNIT-TS-033
Requirement: US-02
Description: useWorkflows.createWorkflow triggers a re-fetch after the RPC call resolves; new workflow appears in the returned list
Type: unit
File: `web-app/src/lib/hooks/useWorkflows.test.ts`

---

ID: T-UNIT-TS-034
Requirement: US-02
Description: useWorkflows.deleteWorkflow performs an optimistic local state update (removes item from list before server confirms); list is refreshed after RPC resolves
Type: unit
File: `web-app/src/lib/hooks/useWorkflows.test.ts`

---

ID: T-UNIT-TS-035
Requirement: US-02
Description: useWorkflows sets error state when the fetch RPC fails; loading returns to false; workflows array is empty on error
Type: unit
File: `web-app/src/lib/hooks/useWorkflows.test.ts`

---

ID: T-UNIT-TS-036
Requirement: US-02
Description: WorkflowForm renders all 11 fields; slug field is disabled/read-only when workflow prop is provided (edit mode); slug field is editable in create mode (no workflow prop)
Type: unit
File: `web-app/src/components/workflows/WorkflowForm.test.tsx`

---

ID: T-UNIT-TS-037
Requirement: US-02
Description: WorkflowForm pre-fills all fields from the workflow prop in edit mode; changing a field and clicking Save calls onSave with the updated values
Type: unit
File: `web-app/src/components/workflows/WorkflowForm.test.tsx`

---

ID: T-UNIT-TS-038
Requirement: US-02
Description: WorkflowForm clicking Cancel calls onCancel without calling onSave
Type: unit
File: `web-app/src/components/workflows/WorkflowForm.test.tsx`

---

ID: T-UNIT-TS-039
Requirement: US-02
Description: WorkflowsPanel renders empty state message "No workflows yet. Create one to get started." when useWorkflows returns an empty list
Type: unit
File: `web-app/src/components/workflows/WorkflowsPanel.test.tsx`

---

ID: T-UNIT-TS-040
Requirement: US-02
Description: WorkflowsPanel renders a list of workflow cards with slug badge, name, command chip, and schedule chip ("Manual only" when cron_enabled=false, cron expression when cron_enabled=true)
Type: unit
File: `web-app/src/components/workflows/WorkflowsPanel.test.tsx`

---

ID: T-UNIT-TS-041
Requirement: US-02
Description: WorkflowsPanel clicking "New Workflow" button transitions to create mode and renders WorkflowForm without a workflow prop
Type: unit
File: `web-app/src/components/workflows/WorkflowsPanel.test.tsx`

---

ID: T-UNIT-TS-042
Requirement: US-02
Description: WorkflowsPanel delete confirmation modal renders with createPortal (rendered to document.body, not inside the component tree); delete confirmation calls deleteWorkflow from the hook
Type: unit
File: `web-app/src/components/workflows/WorkflowsPanel.test.tsx`

---

ID: T-UNIT-TS-043
Requirement: US-01, US-02
Description: resetDefaultRegistry() resets the singleton to null so subsequent getDefaultRegistry() calls return a fresh instance; verifies test isolation mechanism works
Type: unit
File: `web-app/src/lib/omnibar/detector.test.ts`

---

### Playwright E2E Tests

---

ID: T-E2E-001
Requirement: US-02
Description: workflows_should_showEmptyState_When_noWorkflows — navigate to /workflows page; assert empty state text is visible; assert "New Workflow" button is present
Type: e2e
File: `tests/e2e/workflows.spec.ts`

---

ID: T-E2E-002
Requirement: US-02
Description: workflows_should_createWorkflow_When_formSubmitted — click "New Workflow"; fill in all required fields via aria-label / data-testid locators; submit; assert new workflow card appears in the list with correct slug badge, name, and command
Type: e2e
File: `tests/e2e/workflows.spec.ts`

---

ID: T-E2E-003
Requirement: US-01
Description: workflows_should_invokeFromOmnibar_When_atSlugTyped — beforeEach: create a workflow via the management panel form (fixture); open omnibar; type "@{slug}"; assert dropdown shows workflow suggestion with name and description; select suggestion; assert a new session appears with the workflow's configured name prefix
Type: e2e
File: `tests/e2e/workflows.spec.ts`

---

ID: T-E2E-004
Requirement: US-01
Description: workflows_should_prePopulateFormFields_When_workflowDetected — open omnibar; type "@{slug} https://test.example"; assert creation panel working directory field is pre-filled with workflow's targetDirectory; assert prompt field contains interpolated text with the URL
Type: e2e
File: `tests/e2e/workflows.spec.ts`

---

ID: T-E2E-005
Requirement: US-02
Description: workflows_should_deleteWorkflow_When_deleteConfirmed — create a workflow (fixture); click Delete on the workflow card; confirm the modal dialog; assert the workflow card is no longer in the list
Type: e2e
File: `tests/e2e/workflows.spec.ts`

---

ID: T-E2E-006
Requirement: US-03
Description: workflows_should_showScheduleChip_When_cronEnabled — create a workflow with cronExpression "0 9 * * 1" and cronEnabled=true via the form; assert the workflow card shows the cron expression as the schedule chip (not "Manual only")
Type: e2e
File: `tests/e2e/workflows.spec.ts`

---

## Summary

| Test Type | Count |
|---|---|
| Go unit tests | 18 (T-UNIT-GO-001 through T-UNIT-GO-018) |
| Go integration tests | 9 (T-INT-GO-001 through T-INT-GO-009) |
| Jest/RTL unit tests | 26 (T-UNIT-TS-018 through T-UNIT-TS-043) |
| Pitfall guard tests | 5 (T-PITFALL-003 through T-PITFALL-007) |
| Playwright e2e tests | 6 (T-E2E-001 through T-E2E-006) |
| **Total** | **64** |

Requirements coverage: **5/5 user stories** (US-01 through US-05 all have at least one test case)

---

## Readiness Gate Evaluation

### CRITERIA-1: All user stories have at least one test case

| Story | Covered By |
|---|---|
| US-01 Omnibar Invocation | T-UNIT-TS-018 through T-UNIT-TS-031, T-PITFALL-003 through T-PITFALL-007, T-E2E-003, T-E2E-004 |
| US-02 Workflow Management Panel | T-UNIT-TS-032 through T-UNIT-TS-042, T-INT-GO-005 through T-INT-GO-008, T-E2E-001, T-E2E-002, T-E2E-005 |
| US-03 Workflow Scheduling (Cron) | T-UNIT-GO-003, T-UNIT-GO-007, T-UNIT-GO-012 through T-UNIT-GO-018, T-INT-GO-009, T-E2E-006 |
| US-04 Workflow Definition Schema | T-UNIT-GO-001, T-UNIT-GO-004, T-UNIT-GO-005, T-UNIT-GO-006, T-INT-GO-001 through T-INT-GO-004 |
| US-05 Storage | T-INT-GO-001 through T-INT-GO-004, T-UNIT-GO-001, T-UNIT-GO-002 |

**Result: PASS — 5/5 stories covered**

---

### CRITERIA-2: All blocking issues from adversarial-review.md are addressed in plan.md

| Issue | Plan Resolution |
|---|---|
| BLOCK-1: PathWithBranchDetector conflict | Resolved in second pass — `.+` before `@` requires at least one char; bare `@slug` does not match. Pitfall guards T-PITFALL-003 through T-PITFALL-007 cover this. |
| BLOCK-2: unregister() missing from DetectorRegistry | Plan explicitly requires adding `unregister()` in Task 3.1.2 before OmnibarContext uses it (Task 3.3.1). Implementation ordering enforces this. |
| BLOCK-3+4: RunWorkflow circular dependency | Resolved via ADR-8: `RunWorkflow` delegates to `scheduler.FireNow()`. `WorkflowSchedulerInterface` is defined in `server/services/` package. Deferred setter `sessionSvc.SetWorkflowService(workflowSvc)` breaks the bootstrapping cycle. |
| BLOCK-5: targetDirectory required vs Optional schema | Resolved via ADR-9: schema is `Optional()` for future flexibility; `CreateWorkflow` handler validates non-empty at application layer. Test T-UNIT-GO-006 covers this. |
| BLOCK-6: ToServerDeps() omission | Tasks 1.2.3 and 4.1.3 explicitly require updating `ToServerDeps()` for both `WorkflowRepo` and `WorkflowScheduler`. Risk 5b documents it as HIGH. |

**Residual concerns from second pass (non-blocking):**

- NEW-CONCERN-1 (Low): `ValidateCronExpression` import direction — `server/services` importing `server/workflows` is safe because `server/workflows` does not import `server/services` (uses structural interfaces). No blocking import cycle. Test T-UNIT-GO-003 verifies the validation logic.
- NEW-CONCERN-2 (Medium): Phase placement of `WorkflowScheduler` in dependency builder — plan mentions "BuildCoreDeps" but the scheduler depends on SessionService which is available earlier than BuildRuntimeDeps. Implementer must resolve this during wiring. No test can enforce build phase ordering, but the deferred injection pattern is explicitly documented.
- NEW-CONCERN-3 (Low): `SetWorkflowService` setter documented in two separate tasks (2.2.3 and 4.1.3) — low risk of omission since both tasks touch the same file.

**Result: PASS — all 4 original blocking issues have documented resolutions in plan.md**

---

### CRITERIA-3: No circular dependency risks remain unaddressed

The plan documents and resolves two distinct circular dependency scenarios:

1. **WorkflowScheduler → SessionService**: Resolved by defining `SessionServiceInterface` in `server/workflows/` package. `WorkflowScheduler` depends on the interface, not the concrete type in `server/services/`. `server/workflows` never imports `server/services`.

2. **RunWorkflow (in WorkflowService) → SessionService → WorkflowService**: Resolved by ADR-8. `WorkflowService.RunWorkflow` calls `scheduler.FireNow()`, not `sessionSvc.CreateSession()`. `WorkflowService` only depends on `WorkflowRepository` and `WorkflowSchedulerInterface`. No path from `WorkflowService` back to `SessionService`.

3. **ValidateCronExpression import** (`server/services` imports `server/workflows`): Safe because `server/workflows` only imports `session/`, `session/ent/`, and third-party packages — not `server/services/`. Verified in second-pass review.

The deferred injection pattern (`SetWorkflowService` setter) is the correct mechanism for the bootstrapping initialization order and avoids requiring either service to hold the other at construction time.

**Result: PASS — all identified circular dependency risks have documented mitigations**

---

### CRITERIA-4: 7-touchpoint session creation registry check

The plan explicitly addresses this in the "Session Creation Mode Registry" section:

> "The `run_workflow` action creates sessions through the existing `create_session` / `one_off` paths. It does NOT introduce a new `SESSION_TYPE_WORKFLOW` proto enum value. Therefore, the 7-touchpoint session creation registry is NOT triggered for this feature."

This is the correct architectural decision because:
- Cron-triggered sessions use `oneOff: true` + `SessionType_DIRECTORY` (ADR-6) — the existing one-off path
- OmnibarContext's `runWorkflow` builds `OmnibarSessionData` with `sessionType` from the workflow config and calls `createSession` — all existing session creation paths
- No new `SESSION_TYPE_WORKFLOW` proto enum value is introduced

The workflow layer is a configuration facade over existing session creation mechanisms. The 7-touchpoint registry applies only when a **new session type** is introduced in the proto enum.

**Result: PASS — no new session creation mode introduced; 7-touchpoint registry correctly identified as not applicable**

---

## Overall Readiness Gate Verdict: CONCERNS

**All 4 criteria pass.** The plan is implementable as written. The following non-blocking concerns should be addressed during implementation:

1. **NEW-CONCERN-2 (Medium):** The exact build phase for constructing `WorkflowScheduler` (BuildCoreDeps vs BuildServiceDeps vs BuildRuntimeDeps) needs to be determined by the implementer by inspecting the actual `BuildRuntimeDeps` code in `server/dependencies.go` lines 337–373. The deferred injection pattern is sound but the phase placement ambiguity could cause a confusing build error.

2. **CONCERN-5 (Medium):** Two invocation paths exist for firing a workflow session: (a) OmnibarContext's `runWorkflow` directly calls `createSession` (skips the RPC), (b) `RunWorkflow` RPC calls `scheduler.FireNow`. This means cron-fired sessions and "Run Now" button sessions both publish an event bus notification, but omnibar-invoked workflow sessions do not. This behavioral difference should be explicitly documented in the PR description and the `OmnibarContext.runWorkflow` implementation should include a comment explaining the intentional bypass of the RPC path.

3. **CONCERN-6 addressed in plan:** The E2E test for omnibar invocation (T-E2E-003) requires a beforeEach fixture to create a workflow first. The plan acknowledges this in Task 4.4.4. The test must use the management panel UI to create the workflow before testing omnibar invocation, not assume a pre-seeded database.

These concerns do not block implementation. All 4 readiness criteria pass.
