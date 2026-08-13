# Validation Plan: QA Engineering Tooling

Status: Draft | Phase: 4 - Validation complete
Created: 2026-04-18
Input: `docs/tasks/qa-engineering-tooling.md` + `requirements.md`

This document maps every test case to a specific requirement or acceptance criterion.
No code is written before this plan exists.

---

## Requirements Traceability Matrix

Each requirement from `requirements.md` is assigned an ID and mapped to test cases below.

| Req ID | Requirement | Story | Test Coverage |
|--------|-------------|-------|---------------|
| REQ-01 | Living backend feature inventory, auto-updates from code changes | Story 1 | UT-1.1, UT-1.2, IT-1.1, IT-1.2 |
| REQ-02 | Living frontend feature inventory, auto-updates from code changes | Story 2 | UT-2.1, IT-2.1, IT-2.2 |
| REQ-03 | Frontend cross-references backend registry to identify implementation gaps | Story 2 | IT-2.2 |
| REQ-04 | Registry updates automatically from CI on source code changes | Story 3 | IT-3.1, IT-3.2 |
| REQ-05 | E2E test coverage of critical flows via Playwright, runs in CI | Story 4 | E2E-4.1 – E2E-4.7 |
| REQ-06 | Feature flow video capture on PRs that touch feature code | Story 6 | IT-6.1, IT-6.2, E2E-6.1 |
| REQ-07 | UX analysis produces actionable feedback within minutes | Story 5 | IT-5.1, IT-5.2, IT-5.3 |
| REQ-08 | Backend scanner uses Go stack | Story 1 | UT-1.1, UT-1.2 |
| REQ-09 | Frontend scanner uses TypeScript/React stack | Story 2 | UT-2.1 |
| REQ-10 | Extends (not replaces) existing Playwright setup | Story 4, 6 | IT-4.1, IT-6.1 |
| REQ-11 | AI analysis uses Claude API | Story 5 | IT-5.3 |
| REQ-12 | Each tool is independently shippable | All | (phase-gate checkpoints) |

---

## Test Pyramid

```
                    /\
                   /  \
                  / E2E \       8 tests
                 /  (10%) \
                /___________\
               /             \
              / Integration    \    16 tests
             /    (25%)         \
            /_____________________\
           /                       \
          /       Unit Tests         \   28 tests
         /         (65%)              \
        /_________________________________\
```

**Total**: 52 test cases across 3 layers.
**Rationale**: Heavy unit testing at scanner/parser level where logic is complex and inputs are controlled. Integration tests verify tool composition and CI workflow behavior. E2E tests verify only the 7 critical user flows (per 30% coverage target from plan).

---

## Unit Tests

### Story 1: Backend Feature Scanner

#### UT-1.1: Proto scanner — service method extraction
**Covers**: REQ-01, REQ-08
**File**: `tools/scanner/backend/proto_scanner_test.go`
**Approach**: Fixture-based unit tests with minimal synthetic `.proto` files

| Test Case | Input | Expected Output | Risk Level |
|-----------|-------|-----------------|------------|
| UT-1.1a | Proto with 3 service methods | 3 `BackendFeature` structs | Low |
| UT-1.1b | Proto with message types and enums only (no services) | Empty slice, no error | Medium |
| UT-1.1c | Proto with nested RPC options | Features extracted; options ignored | Low |
| UT-1.1d | Malformed proto file (syntax error) | Error returned; no partial results | High |
| UT-1.1e | Proto with multiple service blocks | All methods from all services extracted | Medium |

**Boundary values**: 0 service methods (empty proto), 1 method, 45 methods (full session.proto).

#### UT-1.2: Marker scanner — Go handler discovery
**Covers**: REQ-01, REQ-08
**File**: `tools/scanner/backend/marker_scanner_test.go`
**Approach**: Fixture directory with synthetic Go files

| Test Case | Input | Expected Output | Risk Level |
|-----------|-------|-----------------|------------|
| UT-1.2a | Go file with `// +api: session:create` comment | 1 entry, correct ID and function name | Low |
| UT-1.2b | Go file with no `// +api:` markers | Empty map, no error | Low |
| UT-1.2c | Go file ending in `.pb.go` | Excluded; 0 results | **High** (FP risk) |
| UT-1.2d | Go file ending in `_test.go` | Excluded; 0 results | **High** (FP risk) |
| UT-1.2e | Go file with `// +api:` in a comment inside a function body | NOT included (only top-level or method-level) | **High** (FP risk) |
| UT-1.2f | Go file with malformed marker `// +api:` (no ID) | Error or skip with warning | Medium |
| UT-1.2g | Directory with mixed valid/invalid/excluded files | Only valid marked functions returned | Medium |
| UT-1.2h | Unexported function with `// +api:` marker | Included if marker present (marker wins over visibility) | Medium |

**Boundary values**: 0 markers in directory, 1 marker, markers in multiple files.

#### UT-1.3: Registry merger — proto + marker cross-reference
**Covers**: REQ-01
**File**: `tools/scanner/backend/merger_test.go`

| Test Case | Input | Expected Output | Risk Level |
|-----------|-------|-----------------|------------|
| UT-1.3a | 3 proto RPCs, all 3 have matching markers | 3 features, all `markerFound: true` | Low |
| UT-1.3b | 3 proto RPCs, 1 has no marker | 3 features; 1 with `markerFound: false` | Medium |
| UT-1.3c | 3 proto RPCs, marker has extra ID not in proto | Proto entries only; extra marker logged as warning | Medium |
| UT-1.3d | Empty proto results, non-empty markers | Empty features slice | Low |
| UT-1.3e | Duplicate marker IDs in different files | Error returned (ambiguous mapping) | **High** |

---

### Story 2: Frontend Feature Scanner

#### UT-2.1: TypeScript AST component scanner — marker extraction
**Covers**: REQ-02, REQ-09
**File**: `tools/scanner/frontend/src/component-scanner.test.ts`
**Approach**: Fixture `.tsx` files in `tools/scanner/frontend/src/__fixtures__/`

| Test Case | Input | Expected Output | Risk Level |
|-----------|-------|-----------------|------------|
| UT-2.1a | TSX file with `// +feature: ui:session-list` in first 10 lines | 1 `FrontendFeature` struct | Low |
| UT-2.1b | TSX file with no `// +feature:` | Excluded; 0 results | Low |
| UT-2.1c | File ending in `_pb.ts` | Excluded regardless of content | **High** (FP risk) |
| UT-2.1d | File ending in `.test.tsx` or `.spec.tsx` | Excluded | **High** (FP risk) |
| UT-2.1e | File ending in `.stories.tsx` | Excluded | Medium |
| UT-2.1f | `// +feature:` comment on line 12 (beyond first 10) | Not detected | Medium |
| UT-2.1g | TSX file with `// +feature:` and no default export | Feature ID extracted from comment; component name: filename | Medium |
| UT-2.1h | TSX file with multiple `// +feature:` lines | Only first one used; warning logged | Medium |
| UT-2.1i | TSX file in `__tests__/` directory | Excluded | Medium |

**Boundary values**: marker at line 1, marker at line 10, marker at line 11 (boundary — excluded).

#### UT-2.2: Gap reporter — cross-reference logic
**Covers**: REQ-03
**File**: `tools/scanner/frontend/src/gap-reporter.test.ts`

| Test Case | Input | Expected Output | Risk Level |
|-----------|-------|-----------------|------------|
| UT-2.2a | 3 backend features, 3 matching frontend features | No gaps | Low |
| UT-2.2b | 3 backend features, 0 frontend features | 3 unmatched backend entries | Low |
| UT-2.2c | 0 backend features, 3 frontend features | 3 unmatched frontend entries | Low |
| UT-2.2d | Malformed JSON input (missing `features` array) | Error returned; no partial output | Medium |

---

### Story 3: Registry Validation

#### UT-3.1: Divergence percentage calculator
**Covers**: REQ-04
**File**: `tools/scanner/validate-registry_test.sh` (bats or bash unit test)

| Test Case | Input | Expected Exit Code | Risk Level |
|-----------|-------|--------------------|------------|
| UT-3.1a | Old: 42 entries, New: 42 entries, 0 changed | Exit 0, no output | Low |
| UT-3.1b | Old: 42 entries, New: 43 entries (1 added) | Exit 0, warning output (2.4% divergence) | Medium |
| UT-3.1c | Old: 42 entries, New: 38 entries (4 removed) | Exit 1 (9.5% > 2% threshold) | **High** |
| UT-3.1d | Empty new registry | Exit 1 (100% divergence) | **High** |
| UT-3.1e | Exactly 2% divergence (1 change in 50 entries) | Exit 0 with warning | Medium |
| UT-3.1f | 2.1% divergence | Exit 1 (above threshold) | Medium |

**Boundary values**: exactly 2% (pass), 2.0001% (fail), 0% (pass clean), 100% (hard fail).

---

### Story 5: UX Analysis

#### UT-5.1: Claude API prompt builder
**Covers**: REQ-07, REQ-11
**File**: `tools/ux-analysis/analyze.test.ts`

| Test Case | Input | Expected Output | Risk Level |
|-----------|-------|-----------------|------------|
| UT-5.1a | 1 screenshot, feature ID `ui:session-list` | Prompt includes feature context and design system | Low |
| UT-5.1b | 3 screenshots (at max cap) | Prompt includes all 3 images | Low |
| UT-5.1c | 4 screenshots (over cap) | Only first 3 used; 4th logged as skipped | **High** (cost risk) |
| UT-5.1d | `ANTHROPIC_API_KEY` not set | Function returns early with `null`; no API call | **High** |
| UT-5.1e | Estimated cost > $1 | Function returns early with `null` | **High** (cost risk) |

---

### Story 6: Video Capture

#### UT-6.1: Feature-change detection script
**Covers**: REQ-06
**File**: `tools/ci/detect-feature-changes_test.sh`

| Test Case | Input | Expected Exit Code | Risk Level |
|-----------|-------|--------------------|------------|
| UT-6.1a | Changed files include `docs/registry/backend-features.json` | Exit 0 (feature changes detected) | Low |
| UT-6.1b | Changed files include a file containing `// +feature:` | Exit 0 | Medium |
| UT-6.1c | Changed files include a file containing `// +api:` | Exit 0 | Medium |
| UT-6.1d | Changed files are all `*.md` documentation | Exit 1 (no feature changes) | Low |
| UT-6.1e | Empty changed files list | Exit 1 | Low |
| UT-6.1f | Changed file name contains `+feature` in path (no comment match) | Exit 1 (path doesn't count; content must match) | Medium |

---

## Integration Tests

### Story 1: Backend Scanner — End-to-End Tool

#### IT-1.1: Scanner produces valid registry from real codebase
**Covers**: REQ-01, REQ-08
**File**: `tools/scanner/backend/integration_test.go`
**Setup**: Uses actual `proto/session/v1/session.proto` and `server/services/` directory.

| Test Case | Assertion | Risk Level |
|-----------|-----------|------------|
| IT-1.1a | `make registry-generate-backend` exits 0 | Low |
| IT-1.1b | `docs/registry/backend-features.json` created | Low |
| IT-1.1c | JSON validates against `docs/registry/schema.json` | Medium |
| IT-1.1d | Entry count >= 10 (sanity lower bound) | Medium |
| IT-1.1e | Zero entries have `id: ""` (all IDs are non-empty) | Low |
| IT-1.1f | No entry has type other than `"backend"` | Low |

#### IT-1.2: Generated code excluded
**Covers**: REQ-01 (FP prevention)
**File**: `tools/scanner/backend/integration_test.go`

| Test Case | Assertion | Risk Level |
|-----------|-----------|------------|
| IT-1.2a | No registry entry has `handlerFile` ending in `.pb.go` | **High** |
| IT-1.2b | No registry entry has `handlerFile` path containing `/gen/` | **High** |
| IT-1.2c | No registry entry has `handlerFile` ending in `_test.go` | **High** |

---

### Story 2: Frontend Scanner — End-to-End Tool

#### IT-2.1: Scanner produces valid registry from real codebase
**Covers**: REQ-02, REQ-09
**File**: `tools/scanner/frontend/src/integration.test.ts`
**Setup**: Scan actual `web-app/src/` after adding `// +feature:` markers to 5 known components.

| Test Case | Assertion | Risk Level |
|-----------|-----------|------------|
| IT-2.1a | `make registry-generate-frontend` exits 0 | Low |
| IT-2.1b | `docs/registry/frontend-features.json` created | Low |
| IT-2.1c | JSON validates against `docs/registry/schema.json` | Medium |
| IT-2.1d | Exactly 5 entries (matching the 5 marked components) | **High** (precision check) |
| IT-2.1e | No entry with `id` containing `_pb` or `.pb` | **High** (FP check) |

#### IT-2.2: Coverage gap report is accurate
**Covers**: REQ-03
**File**: `tools/scanner/frontend/src/integration.test.ts`

| Test Case | Assertion | Risk Level |
|-----------|-----------|------------|
| IT-2.2a | `docs/registry/coverage-gaps.json` created after `make registry-generate` | Low |
| IT-2.2b | If backend has 10 features and frontend has 5, `unmatchedBackend` has 5 entries | Medium |
| IT-2.2c | Coverage gap report is valid JSON (no parse error) | Low |

---

### Story 3: CI Registry Validation

#### IT-3.1: Validation catches drift
**Covers**: REQ-04
**Approach**: Run validation script with synthetic pre/post registry pairs.

| Test Case | Setup | Assertion | Risk Level |
|-----------|-------|-----------|------------|
| IT-3.1a | Commit registry; add new `// +api:` marker; run validation | Exits non-zero, diff output present | **High** |
| IT-3.1b | Commit registry; no changes; run validation | Exits 0, clean output | Low |
| IT-3.1c | Validation script run without generating registry first | Error message clear; exits non-zero | Medium |

#### IT-3.2: GitHub Actions workflow structure
**Covers**: REQ-04
**Approach**: Validate the workflow YAML is syntactically correct and has required fields.

| Test Case | Assertion | Risk Level |
|-----------|-----------|------------|
| IT-3.2a | `.github/workflows/registry-validation.yml` parses as valid YAML | Low |
| IT-3.2b | Workflow triggers on `pull_request` with `paths` filter | Medium |
| IT-3.2c | Workflow has `if: always()` on the validation step (not blocked by earlier step failure) | Medium |

---

### Story 4: E2E Harness

#### IT-4.1: Allure reporter integration
**Covers**: REQ-05, REQ-10
**File**: `tests/e2e/` (run Playwright + verify Allure output)

| Test Case | Assertion | Risk Level |
|-----------|-----------|------------|
| IT-4.1a | `npx playwright test smoke.spec.ts` still passes (no regression) | Low |
| IT-4.1b | After test run, `tests/e2e/allure-results/` directory created | Low |
| IT-4.1c | `npx allure generate tests/e2e/allure-results --clean` exits 0 | Low |
| IT-4.1d | Allure report contains test result entries | Low |
| IT-4.1e | Existing smoke tests still appear in Allure report | Medium |

---

### Story 5: UX Analysis

#### IT-5.1: Axe Core accessibility scan (CI gate)
**Covers**: REQ-07
**File**: `tests/e2e/accessibility.spec.ts`

| Test Case | Assertion | Risk Level |
|-----------|-----------|------------|
| IT-5.1a | `npx playwright test accessibility.spec.ts` exits 0 on current build | Low |
| IT-5.1b | Test navigates to `/` and to secondary route without 404 | Low |
| IT-5.1c | Axe produces 0 `critical` violations on `/` | **High** |
| IT-5.1d | Axe produces 0 `serious` violations on `/` | **High** |
| IT-5.1e | Axe does NOT flag terminal `<pre>` elements as violations | **High** (known FP risk from plan) |

#### IT-5.2: Lighthouse CI run
**Covers**: REQ-07

| Test Case | Assertion | Risk Level |
|-----------|-----------|------------|
| IT-5.2a | `make e2e-lighthouse` runs without crashing | Low |
| IT-5.2b | Lighthouse performance score is at least 50 (sanity floor, not threshold) | Medium |
| IT-5.2c | Lighthouse output is valid JSON | Low |

#### IT-5.3: Claude API analysis script
**Covers**: REQ-07, REQ-11

| Test Case | Setup | Assertion | Risk Level |
|-----------|-------|-----------|------------|
| IT-5.3a | `ANTHROPIC_API_KEY` not set | Script exits 0 with "skipped" message | **High** |
| IT-5.3b | `ANTHROPIC_API_KEY` set; 1 screenshot provided | Returns JSON with at least `accessibility_issues` array | Medium |
| IT-5.3c | Output file `docs/qa/ux-findings-test.md` written | File contains at least 10 lines | Medium |
| IT-5.3d | 4 screenshots provided (over cap) | Only 3 processed; 4th logged; no API error | **High** (cost risk) |

---

### Story 6: Video Capture

#### IT-6.1: Playwright config respects `RECORD_FEATURES`
**Covers**: REQ-06, REQ-10

| Test Case | Setup | Assertion | Risk Level |
|-----------|-------|-----------|------------|
| IT-6.1a | `RECORD_FEATURES=false`, run smoke test | No `.webm` files in `test-results/` | Low |
| IT-6.1b | `RECORD_FEATURES=true`, run smoke test | At least 1 `.webm` file in `test-results/` | **High** |
| IT-6.1c | `RECORD_FEATURES=true`, test fails mid-run | `.webm` file exists for the failed test | **High** |

#### IT-6.2: CI workflow structure
**Covers**: REQ-06

| Test Case | Assertion | Risk Level |
|-----------|-----------|------------|
| IT-6.2a | `.github/workflows/e2e-video.yml` is valid YAML | Low |
| IT-6.2b | Workflow has artifact upload step with `retention-days: 30` | Medium |
| IT-6.2c | Workflow posts PR comment step guarded by `if: steps.detect.outputs.feature_changed == 'true'` | Medium |
| IT-6.2d | Workflow has video file count validation step (non-blocking failure mode) | **High** |

---

## E2E Tests (Critical Path Coverage)

These 7 tests are the implementation of REQ-05 (30% critical path E2E coverage).
Each test maps to a registry feature ID and must pass in CI with <2% flakiness.

### E2E-4.1: Session Create
**Feature ID**: `session:create`
**Covers**: REQ-05
**File**: `tests/e2e/session-lifecycle.spec.ts`

```
Given: The web UI is loaded at localhost:8543
When: User clicks "New Session", fills in title and path, clicks Create
Then: Session appears in session list with status "Running"
And: Session title matches the input
And: Session has non-empty ID
```

**Isolation**: Fresh `STAPLER_SQUAD_INSTANCE=test-{pid}` per test
**Teardown**: Server stop; verify no orphaned tmux sessions
**Explicit waits**: `waitForSelector('[data-testid="session-card"]')` (no timeouts)

---

### E2E-4.2: Session Pause
**Feature ID**: `session:update` (pause action)
**Covers**: REQ-05
**File**: `tests/e2e/session-lifecycle.spec.ts`

```
Given: A running session exists
When: User clicks "Pause" on the session card
Then: Session status changes to "Paused"
And: Pause button disappears; Resume button appears
```

**Prerequisite**: Session created (shares setup fixture with E2E-4.1 if sequential)

---

### E2E-4.3: Session Resume
**Feature ID**: `session:update` (resume action)
**Covers**: REQ-05
**File**: `tests/e2e/session-lifecycle.spec.ts`

```
Given: A paused session exists
When: User clicks "Resume" on the session card
Then: Session status changes to "Running"
And: Resume button disappears; Pause button appears
```

---

### E2E-4.4: Session Delete
**Feature ID**: `session:delete`
**Covers**: REQ-05
**File**: `tests/e2e/session-lifecycle.spec.ts`

```
Given: A session exists (any status)
When: User clicks "Delete" on the session card and confirms the dialog
Then: Session is no longer visible in the session list
And: Session count decreases by 1
```

**Edge case**: Cancel on confirmation dialog — session remains

---

### E2E-4.5: History Search
**Feature ID**: `history:search`
**Covers**: REQ-05
**File**: `tests/e2e/history-search.spec.ts`

```
Given: History has been pre-seeded via API with a known conversation entry
  (seed via ConnectRPC call before UI interaction)
When: User enters the seeded search term in the search bar
Then: Search results appear containing the seeded entry
And: Results appear within 3 seconds of input
And: Empty search clears results
```

**Fixture approach**: API-based seed; avoids UI-dependent setup

---

### E2E-4.6: Workspace List
**Feature ID**: `workspace:list-targets`
**Covers**: REQ-05
**File**: `tests/e2e/workspace-management.spec.ts`

```
Given: A session is attached to a git repository with multiple branches
When: User opens the workspace switcher
Then: Branch list loads and displays at least 2 branches
And: Current branch is highlighted
And: List renders within 5 seconds
```

**Fixture**: Local bare git repo in `tests/e2e/fixtures/test-repo.git/` (pre-created with 3 branches)

---

### E2E-4.7: Workspace Switch
**Feature ID**: `workspace:switch`
**Covers**: REQ-05
**File**: `tests/e2e/workspace-management.spec.ts`

```
Given: Workspace switcher is open with at least 2 branches visible
When: User selects a different branch
Then: Session restarts on the new branch
And: Session status transitions through "restarting" → "running"
And: Workspace indicator shows the new branch name
```

**Note**: This test must wait for session restart to complete (non-deterministic duration).
**Wait strategy**: `waitForSelector('[data-testid="session-status-running"]', { timeout: 30000 })`

---

### E2E-6.1: Video capture for feature-change PR
**Covers**: REQ-06
**Approach**: Manual verification (not automated in CI) — run locally once before Story 6 is declared stable.

```
Given: RECORD_FEATURES=true
When: npx playwright test session-lifecycle.spec.ts
Then: tests/e2e/test-results/ contains .webm files
And: File count equals or exceeds the number of test cases (4 for session lifecycle)
And: Each video file is > 10 KB (not empty/corrupt)
```

---

## Risk-Based Test Prioritization

| Priority | Test IDs | Risk | Why |
|----------|----------|------|-----|
| P0 — Must pass before any story is declared done | UT-1.2c, UT-1.2d, UT-2.1c, UT-2.1d, IT-1.2a-c, IT-2.1e | False positive prevention | Scanner FP rate >5% invalidates entire registry |
| P0 — Must pass before Story 4 ships | E2E-4.1 through E2E-4.7 | Core coverage | If E2E tests are flaky, the harness delivers negative value |
| P1 — Must pass before Story 5 ships | IT-5.1c, IT-5.1d, IT-5.1e | Accessibility gate reliability | Gate must not produce FPs on terminal UI |
| P1 — Must pass before Story 5 ships | IT-5.3a, UT-5.1d, UT-5.1e | Claude cost guards | Uncapped API calls can exhaust budget |
| P1 — Must pass before Story 6 ships | IT-6.1b, IT-6.1c, IT-6.2d | Video reliability | Silent failure mode produces misleading PR comments |
| P2 — Should pass but don't block | IT-5.2a, IT-5.2b | Lighthouse advisory | Warning, not blocking; failures acceptable at launch |

---

## Test Infrastructure Requirements

### Go Unit Tests
- Standard `go test ./...`; no additional dependencies
- Fixture files in `tools/scanner/backend/testdata/`
- Exclude from normal `go build` (test-only fixtures)

### TypeScript Unit Tests
- `jest` or `vitest` (whichever is already in `tests/e2e/package.json`)
- Fixture files in `tools/scanner/frontend/src/__fixtures__/`

### Shell Script Tests
- `bats-core` for bash unit tests (or simple inline `assert` functions)
- Install: `brew install bats-core`

### E2E Tests
- Existing `npx playwright test` command
- Server isolation via `STAPLER_SQUAD_INSTANCE=test-{process.pid}`
- Pre-condition: local tmux available; Go binary built (`make build`)
- Post-condition: all server processes stopped; tmux sessions cleaned up
- CI: `ubuntu-22.04` runner (pinned to avoid runner image surprises)

### Flakiness Gate
Before declaring Story 4 stable: run `npx playwright test` 3 times consecutively.
Pass criteria: 0 test failures across all 3 runs.
If any flakiness detected: debug and fix before proceeding to Story 5.

---

## Coverage Requirements

| Layer | Target | Measured By |
|-------|--------|-------------|
| Unit tests (scanner logic) | 100% branch coverage on exclusion logic (`.pb.go`, `_test.go` filters) | `go test -cover`, `jest --coverage` |
| Unit tests (all other logic) | 80% statement coverage | Same tools |
| Integration tests | All acceptance criteria in each story | Manual checklist per story |
| E2E tests | 7/7 critical flows passing, <2% flakiness | Playwright + Allure flakiness report |
| Requirements | 12/12 requirements covered | This document's traceability matrix |

---

## Definition of Done

A story is done when:
- [ ] All unit tests for that story pass (`go test` / `jest`)
- [ ] All integration tests for that story pass
- [ ] All E2E tests referencing that story's features pass (if applicable)
- [ ] P0 risks for that story are verified (see Risk-Based Prioritization)
- [ ] Integration checkpoint from `docs/tasks/qa-engineering-tooling.md` passes
- [ ] Bug Prevention Checklist from plan is checked off
- [ ] `make quick-check` passes (existing CI gate)

The full feature is done when:
- [ ] All 12 requirements have at least one passing test in the traceability matrix
- [ ] `docs/registry/backend-features.json` and `docs/registry/frontend-features.json` exist and are valid
- [ ] E2E coverage report shows ≥7 features tested
- [ ] Registry validation CI job runs on a real PR and produces expected output
- [ ] UX analysis runs without error (or gracefully skips if API key absent)
- [ ] Video capture produces at least one `.webm` file in a test run with `RECORD_FEATURES=true`
