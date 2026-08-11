# Implementation Plan: QA Engineering Tooling

Status: Ready for implementation
Phase: 3 - Planning complete
Created: 2026-04-17
Source requirements: `project_plans/qa-engineering-tooling/requirements.md`
Source research: `project_plans/qa-engineering-tooling/research/`
ADRs: `project_plans/qa-engineering-tooling/decisions/`

---

## Resolved Open Questions

Three open questions from `synthesis.md` were resolved before planning began:

**Feature granularity**: A "feature" is a ConnectRPC service method (backend) or a React page-level component / named user flow (frontend). Utility components, internal helpers, and generated code are NOT features. Utility = not directly invocable by a user via the UI or a named RPC.

**E2E coverage target**: 30% of critical paths for initial phase. Critical paths are: session lifecycle (create, pause, resume, stop/delete), history search, workspace management (switch workspace, list targets). These seven flows cover the highest-value regression surface.

**Registry schema versioning**: Simple `"version": "1"` field in JSON. Manual migration required for breaking changes. No auto-migration tooling for MVP.

---

## Architecture Summary

Five tools delivered in six stories (stories 3 and 6 are integration work, not new tools):

```
proto/session/v1/session.proto
server/services/*.go
        |
        v
[Story 1: Backend Scanner]  -->  docs/registry/backend-features.json
        |
web-app/src/**/*.tsx
        |
        v
[Story 2: Frontend Scanner]  -->  docs/registry/frontend-features.json
        |
        v
[Story 3: Registry CI Validation]  (reads both registry files, no new output)
        |
tests/e2e/
        |
        v
[Story 4: E2E Harness Enhancement]  -->  Allure reports, feature coverage report
        |
        v
[Story 5: UX Analysis Automation]  -->  docs/qa/ux-findings-{pr}.md, CI gate
        |
        v
[Story 6: Feature Flow Video Capture]  -->  GitHub artifacts, PR comment
```

### Dependency Graph

```
Story 1 (Backend Scanner)
    |
    +-- Story 3 (Registry CI Validation) <-- Story 2 (Frontend Scanner)
                |
                +-- Story 4 (E2E Harness)
                            |
                            +-- Story 5 (UX Analysis)
                            |
                            +-- Story 6 (Video Capture)
```

Stories 5 and 6 depend on Story 4 (shared Playwright infrastructure) but are independent of each other and can be implemented in parallel.

### Key Conventions

**Feature ID scheme**: `{scope}:{action}`
- Backend examples: `session:create`, `session:pause`, `history:search`, `workspace:switch`
- Frontend examples: `ui:session-list`, `ui:new-session-modal`, `ui:review-queue`

**Marker system**:
- Go: `// +api: session:create` on handler methods in `server/services/`
- TypeScript: `// +feature: ui:new-session-modal` at the top of feature component files

**Registry location**: `docs/registry/` (committed to repo, updated by CI scanner)

**Generated code exclusion rules**:
- Go scanner: skip files matching `*.pb.go`, `*.gen.go`, `*_test.go`
- TypeScript scanner: skip files matching `*_pb.ts`, `*.pb.ts`, `*.test.ts`, `*.spec.ts`, `*.stories.ts`

---

## Story 1: Backend Feature Scanner

**As a** developer, **I want** a Go tool that scans the ConnectRPC service definition and handler implementations and outputs a structured JSON registry, **so that** I have an authoritative, up-to-date list of every backend API feature.

**Acceptance criteria**:
- Running `make registry-generate-backend` produces `docs/registry/backend-features.json`
- The registry contains one entry per RPC method defined in `proto/session/v1/session.proto`
- Each entry includes: `id`, `type: "backend"`, `service`, `method`, `protoFile`, `tested: false`, `testIds: []`, `lastModified`
- Generated `.pb.go` files are excluded from Go AST scan
- Private (unexported) Go functions are excluded
- Test files (`*_test.go`) are excluded
- False positive rate on first run is below 5% (verified by manual audit of output)
- Registry file validates against the schema defined in `docs/registry/schema.json`

**Implementation tasks** (each 1-4 hours, single responsibility):

### Task 1.1: Write proto-to-registry extractor
- Location: `tools/scanner/backend/proto_scanner.go`
- Input: path to `proto/session/v1/session.proto`
- Output: slice of `BackendFeature` structs (service name, method name, proto file path)
- Approach: use buf CLI (`buf build` + `buf export`) or parse proto file directly with `github.com/bufbuild/protocompile`
- Exclude: all non-service definitions (message types, enums)
- Test: unit test with a minimal fixture proto file

### Task 1.2: Write Go handler marker extractor
- Location: `tools/scanner/backend/marker_scanner.go`
- Input: path to `server/services/` directory
- Approach: walk all `.go` files; skip `*.pb.go`, `*_test.go`; for each file, walk AST looking for `// +api:` comments; extract the feature ID from the comment
- Output: map of feature ID -> file path + function name
- Test: unit test with fixture Go files containing marked and unmarked functions

### Task 1.3: Merge and emit registry JSON
- Location: `tools/scanner/backend/cmd/main.go`
- Merge proto scan results and marker scan results; for each proto RPC, look up marker in handler scan; emit `BackendFeature` JSON
- If proto RPC has no matching marker, include in registry with `markerFound: false` (not an error at scan time; CI validation will warn)
- Emit `docs/registry/backend-features.json`
- Test: integration test using actual proto/ and server/services/ directories

### Task 1.4: Add Makefile target and schema file
- `make registry-generate-backend`: runs the scanner
- `docs/registry/schema.json`: JSON Schema document for the registry format
- README entry in `tools/scanner/README.md`

**Context preparation**:
Read these files before implementing:
- `proto/session/v1/session.proto` (all RPC method names)
- `server/services/session_service.go` (ConnectRPC handler pattern)
- `buf.gen.yaml` and `buf.yaml` (buf CLI config)
- ADR-002 (`project_plans/qa-engineering-tooling/decisions/ADR-002-backend-scanner.md`)

**Integration checkpoint after Story 1**: Run `make registry-generate-backend`. Open `docs/registry/backend-features.json`. Count entries. The proto has 45 RPC methods. Verify count is 45 +/- 2. Spot-check 5 entries manually against `session.proto`. If false positive rate exceeds 5%, apply stricter marker filtering before proceeding.

---

## Story 2: Frontend Feature Scanner

**As a** developer, **I want** a TypeScript tool that scans React component files for `// +feature:` markers and outputs a frontend feature registry, **so that** I know which UI features exist and can cross-reference them against the backend registry.

**Acceptance criteria**:
- Running `make registry-generate-frontend` produces `docs/registry/frontend-features.json`
- Only files with a `// +feature:` marker are included (no marker = not a feature)
- Each entry includes: `id`, `type: "frontend"`, `component`, `path`, `tested: false`, `testIds: []`, `lastModified`
- Files matching `*_pb.ts`, `*.test.ts`, `*.spec.ts`, `*.stories.ts` are excluded
- A cross-reference report is produced at `docs/registry/coverage-gaps.json` listing backend features with no matching frontend component
- Running `make registry-generate` runs both backend and frontend scanners sequentially

**Implementation tasks**:

### Task 2.1: Write TypeScript AST component scanner
- Location: `tools/scanner/frontend/src/component-scanner.ts`
- Input: path to `web-app/src/`
- Approach: use TypeScript Compiler API (`ts.createProgram`); walk source files; for each file, check for `// +feature:` comment in the first 10 lines; if found, extract feature ID and component name from the default export
- Exclude files matching: `*_pb.ts`, `*.pb.ts`, `*.test.tsx`, `*.spec.tsx`, `*.stories.tsx`, files under `__tests__/`
- Output: `FrontendFeature[]` struct
- Test: unit tests with fixture component files (marked and unmarked)

### Task 2.2: Write coverage gap reporter
- Location: `tools/scanner/frontend/src/gap-reporter.ts`
- Input: `docs/registry/backend-features.json` + `docs/registry/frontend-features.json`
- Logic: for each backend feature with `type: "full-stack"`, check if a matching frontend feature exists by ID prefix (e.g., backend `session:create` matches frontend `ui:new-session-modal` via explicit mapping, OR by convention check)
- Output: `docs/registry/coverage-gaps.json` with `{ "unmatchedBackend": [...], "unmatchedFrontend": [...] }`
- Note: mapping is advisory; gaps are warnings not errors

### Task 2.3: Wire CLI entry point and Makefile
- Location: `tools/scanner/frontend/src/main.ts`
- `make registry-generate-frontend`: runs frontend scanner
- `make registry-generate`: runs `registry-generate-backend` then `registry-generate-frontend`

**Context preparation**:
Read these files before implementing:
- `web-app/src/` top-level structure (identify which files are page-level components vs utilities)
- `web-app/src/app/` React route structure
- ADR-003 (`project_plans/qa-engineering-tooling/decisions/ADR-003-frontend-scanner.md`)
- Story 1 registry output at `docs/registry/backend-features.json`

**Integration checkpoint after Story 2**: Run `make registry-generate`. Verify both JSON files exist. Open `docs/registry/coverage-gaps.json`. The gap report should list gaps, not panic. Add `// +feature:` markers to 5-10 key components in `web-app/src/` and re-run; verify they appear in `frontend-features.json`.

---

## Story 3: Registry CI Validation

**As a** developer, **I want** a CI job that validates the committed registry files match what the scanner would generate, **so that** registry drift is caught before it reaches the default branch.

**Acceptance criteria**:
- A GitHub Actions workflow step `Registry Validation` runs on every PR touching Go, TypeScript, or proto files
- The step runs `make registry-generate` and compares output against committed registry files
- If divergence exceeds 2% of entries, the step exits non-zero (blocks PR merge)
- If divergence is 1-2%, the step posts a warning comment on the PR but does not block
- The step posts a PR comment with the diff when new features are detected (human review required)
- The committed registry files are the source of truth; scanner output is the challenger

**Implementation tasks**:

### Task 3.1: Write registry diff script
- Location: `tools/scanner/validate-registry.sh`
- Logic: run `make registry-generate` to temp dir; compute diff against committed files; calculate divergence percentage; exit 1 if >2%; exit 0 with warning output if 1-2%
- Output: structured diff summary to stdout (JSON)

### Task 3.2: Add CI workflow
- Location: `.github/workflows/registry-validation.yml`
- Trigger: `pull_request` on paths `**.go`, `**.proto`, `web-app/src/**`
- Steps: checkout, setup Go + Node, build binary, run `make registry-generate`, run validation script, post PR comment via `gh` CLI if diff detected
- Non-blocking initially; promote to blocking after 2-week validation period (comment in workflow YAML)

### Task 3.3: Add `make registry-diff` target
- Developer convenience: shows what would change without writing files
- Output: colored diff to stdout

**Context preparation**:
Read these files before implementing:
- `.github/workflows/build.yml` (existing CI structure and actions)
- `docs/registry/backend-features.json` and `docs/registry/frontend-features.json` (schema)
- Story 1 and Story 2 outputs

**Integration checkpoint after Story 3**: Open a draft PR that adds a new `// +api:` marker. Verify the `Registry Validation` CI job runs and posts a PR comment describing the new feature detected.

---

## Story 4: E2E Test Harness Enhancement

**As a** developer, **I want** an enhanced Playwright test harness with Allure reporting, feature ID decorators, and 7 critical-flow test cases, **so that** I have CI-enforced E2E coverage of the most important user flows with traceability to the feature registry.

**Acceptance criteria**:
- Allure reporter is installed and generates reports from `npx allure generate`
- All new tests include a `// @feature {feature-id}` comment mapping to a registry entry
- 7 critical-path test cases are implemented and passing in CI (see list below)
- A CI coverage report is posted as a PR comment: "Feature E2E coverage: N/42 tested"
- Tests run sequentially (existing config) with isolation via `STAPLER_SQUAD_INSTANCE=test-{pid}`
- Flakiness target: fewer than 2 failures in 10 consecutive CI runs before declaring Story 4 stable
- All tests use explicit `waitForSelector` or `waitForResponse`; no `waitForTimeout` calls
- Tests use `data-testid` attributes or ARIA roles; no CSS nth-child or positional selectors

**Critical-path tests** (7 flows = 30% of session lifecycle + history + workspace):

| Test ID | Feature ID | Description |
|---------|------------|-------------|
| e2e:session-create | session:create | Create a new session with title and path |
| e2e:session-pause | session:update (pause) | Pause a running session |
| e2e:session-resume | session:update (resume) | Resume a paused session |
| e2e:session-delete | session:delete | Delete a session, verify removal from list |
| e2e:history-search | history:search | Search history for a known term, verify results appear |
| e2e:workspace-list | workspace:list-targets | Open workspace switcher, verify targets load |
| e2e:workspace-switch | workspace:switch | Switch workspace to a different branch |

**Implementation tasks**:

### Task 4.1: Add Allure reporter to Playwright config
- Install `allure-playwright` package in `tests/e2e/package.json`
- Update `tests/e2e/playwright.config.ts` to add Allure reporter alongside existing `html` reporter
- Add `make e2e-report` target: runs `npx allure generate tests/e2e/allure-results --clean`

### Task 4.2: Write session lifecycle tests
- Location: `tests/e2e/session-lifecycle.spec.ts`
- Tests: `e2e:session-create`, `e2e:session-pause`, `e2e:session-resume`, `e2e:session-delete`
- Each test: starts fresh isolated server instance; creates session via UI; asserts state; tears down
- Use existing `TestServer` helper from `tests/e2e/helpers/test-server.ts`
- Use `STAPLER_SQUAD_INSTANCE=test-${process.pid}` for test isolation
- Page object: `tests/e2e/pages/SessionsPage.ts` encapsulating locators

### Task 4.3: Write history search test
- Location: `tests/e2e/history-search.spec.ts`
- Test: `e2e:history-search`
- Fixture: pre-seed history via API call before test; search for seeded term; assert result appears
- Use ConnectRPC API directly in fixture setup (no UI clicks for setup data)

### Task 4.4: Write workspace management tests
- Location: `tests/e2e/workspace-management.spec.ts`
- Tests: `e2e:workspace-list`, `e2e:workspace-switch`
- Fixture: session must have a git repo with multiple branches; use a local fixture bare repo
- Note: workspace switch restarts the session; test must wait for restart to complete

### Task 4.5: Add feature coverage report to CI
- Location: `tools/coverage/feature-coverage.ts`
- Input: `docs/registry/backend-features.json` + Playwright test file glob to extract `@feature` comments
- Output: JSON report + markdown summary
- CI step in `.github/workflows/build.yml`: runs after E2E tests; posts PR comment via `gh`

**Context preparation**:
Read these files before implementing:
- `tests/e2e/playwright.config.ts` (current config)
- `tests/e2e/helpers/test-server.ts` (server lifecycle API)
- `tests/e2e/global-setup.ts` and `tests/e2e/global-teardown.ts`
- `tests/e2e/smoke.spec.ts` (existing test pattern)
- `docs/registry/backend-features.json` (feature IDs to reference)

**Integration checkpoint after Story 4**: Run `npx playwright test` from `tests/e2e/`. All 7 new tests must pass. Run 3 times consecutively to check for flakiness. Run `npx allure generate` and open the report. Verify feature IDs appear in test descriptions.

---

## Story 5: UX Analysis Automation

**As a** developer, **I want** automated accessibility and UX quality checks that run on UI-touching PRs, **so that** accessibility regressions are caught before merge and subjective UX feedback is available without manual review time.

**Acceptance criteria**:
- Axe Core runs via Playwright on every PR touching `web-app/src/`; WCAG 2.1 AA violations with severity `critical` or `serious` block CI
- Lighthouse CI runs on the same PRs; performance score below 70 is a warning (not blocking)
- Claude vision analysis runs as an advisory step; outputs `docs/qa/ux-findings-{pr-number}.md`
- Claude analysis is NOT a CI gate; its output is a PR comment with the top 3 findings
- Claude step is skipped if `ANTHROPIC_API_KEY` is not set (graceful degradation)
- Total UX analysis step adds no more than 5 minutes to CI

**Implementation tasks**:

### Task 5.1: Integrate Axe Core via Playwright
- Install `@axe-core/playwright` in `tests/e2e/package.json`
- Location: `tests/e2e/accessibility.spec.ts`
- Test: navigate to each primary route (`/`, `/review-queue`); run `checkA11y`; assert zero `critical` or `serious` violations
- Mark with `// @feature ui:accessibility-gate`
- The test file itself is the CI gate; failure blocks merge

### Task 5.2: Add Lighthouse CI
- Install `@lhci/cli` in `tests/e2e/package.json`
- Location: `tests/e2e/lighthouse.config.js`
- Configure: `assert.preset = 'lighthouse:recommended'`; performance threshold 70
- Add `make e2e-lighthouse` target
- CI: add step after build, runs Lighthouse on `http://localhost:8544`; post score as PR comment via `gh`; warning (non-blocking) on score below 70

### Task 5.3: Write Claude UX analysis script
- Location: `tools/ux-analysis/analyze.ts`
- Input: list of screenshots (captured by Playwright during E2E tests), PR number, feature ID
- API: Claude API with vision, using `claude-sonnet-4-6` (cost-efficient for advisory)
- Prompt strategy: include design system context (CSS custom properties from `web-app/src/app/globals.css`), accessibility target (WCAG 2.1 AA), and feature context
- Output: `docs/qa/ux-findings-{pr-number}.md` with top findings, confidence scores, and action items
- Guardrails: max 3 screenshots per analysis call; max 2 API calls per PR; skip if estimated cost exceeds $1
- Skip silently if `ANTHROPIC_API_KEY` is not set

### Task 5.4: Wire UX analysis into CI
- Location: `.github/workflows/ux-analysis.yml`
- Trigger: `pull_request` on paths `web-app/src/**`
- Steps: install Playwright browsers, start test server, capture screenshots per feature, run analyze.ts, post PR comment with findings summary
- `ANTHROPIC_API_KEY` set as GitHub Secret; step skipped if secret not present
- Runs in parallel with test job (not blocking)

**Context preparation**:
Read these files before implementing:
- `web-app/src/app/globals.css` (design tokens for Claude prompt context)
- `tests/e2e/playwright.config.ts` (screenshot config)
- ADR-004 (`project_plans/qa-engineering-tooling/decisions/ADR-004-ux-analysis.md`)
- `docs/registry/frontend-features.json` (feature IDs for context)

**Integration checkpoint after Story 5**: Open a PR that changes a CSS color. Verify: (1) Axe test runs and passes, (2) Lighthouse report appears as PR comment, (3) if `ANTHROPIC_API_KEY` is set, Claude analysis comment appears. Verify CI does not fail if API key is absent.

---

## Story 6: Feature Flow Video Capture

**As a** developer, **I want** Playwright to automatically record video of E2E test flows when a PR touches feature-marked files, **so that** I can visually review feature behavior in PRs without local setup.

**Acceptance criteria**:
- Video recording is enabled only when env var `RECORD_FEATURES=true` is set
- CI sets `RECORD_FEATURES=true` on PRs that touch files matching `docs/registry/` or containing `// +feature:` or `// +api:` markers
- Videos are uploaded to GitHub Actions artifacts with 30-day retention
- A PR comment is posted with links to the video artifacts when recording is active
- Video capture adds no more than 30% overhead to test execution time (measured, not estimated)
- If video encoding fails, tests continue and CI marks video step as warning (not blocking)

**Implementation tasks**:

### Task 6.1: Update Playwright config for conditional video
- Modify `tests/e2e/playwright.config.ts` to set `video: 'on'` when `process.env.RECORD_FEATURES === 'true'`, else keep existing `'retain-on-failure'`
- Ensure `outputDir` is set to `tests/e2e/test-results/` for consistent artifact path

### Task 6.2: Write feature-change detection script
- Location: `tools/ci/detect-feature-changes.sh`
- Input: git diff of changed files in PR (`git diff --name-only origin/main...HEAD`)
- Logic: check if any changed file matches `docs/registry/*`, contains `// +feature:`, or contains `// +api:`
- Output: exits 0 if feature changes detected (set `RECORD_FEATURES=true`), exits 1 otherwise
- Test: unit tests using fixture file lists

### Task 6.3: Add video upload and PR comment steps to CI
- Location: `.github/workflows/e2e-video.yml`
- Trigger: `pull_request` on all paths (detection script filters internally)
- Steps:
  1. Run feature-change detection
  2. If detected: run Playwright tests with `RECORD_FEATURES=true`
  3. Upload `tests/e2e/test-results/` as artifact (30-day retention, name `e2e-videos-{pr}-{sha}`)
  4. Post PR comment with artifact link using `gh pr comment`
- If upload fails: post warning comment; do not fail the job

**Context preparation**:
Read these files before implementing:
- `tests/e2e/playwright.config.ts` (current video config)
- `.github/workflows/build.yml` (artifact upload pattern)
- Story 4 test output structure

**Integration checkpoint after Story 6**: Open a PR that adds a `// +feature:` marker to a component. Verify the `e2e-video.yml` workflow runs, produces videos, uploads artifacts, and posts a PR comment with the artifact link. Verify a PR with no feature changes does NOT trigger video recording.

---

## Registry Schema Reference

`docs/registry/backend-features.json`:
```json
{
  "version": "1",
  "generatedAt": "2026-04-17T00:00:00Z",
  "features": [
    {
      "id": "session:create",
      "type": "backend",
      "backend": {
        "service": "SessionService",
        "method": "CreateSession",
        "protoFile": "proto/session/v1/session.proto",
        "markerFound": true,
        "handlerFile": "server/services/session_service.go"
      },
      "tested": false,
      "testIds": [],
      "lastModified": "2026-04-17T00:00:00Z"
    }
  ]
}
```

`docs/registry/frontend-features.json`:
```json
{
  "version": "1",
  "generatedAt": "2026-04-17T00:00:00Z",
  "features": [
    {
      "id": "ui:new-session-modal",
      "type": "frontend",
      "frontend": {
        "component": "NewSessionModal",
        "path": "web-app/src/components/NewSessionModal/NewSessionModal.tsx",
        "markerLine": 1
      },
      "tested": false,
      "testIds": [],
      "lastModified": "2026-04-17T00:00:00Z"
    }
  ]
}
```

---

## File Structure After Implementation

```
docs/
  registry/
    backend-features.json       <- Story 1 output
    frontend-features.json      <- Story 2 output
    coverage-gaps.json          <- Story 2 output
    schema.json                 <- Story 1, registry JSON Schema
  qa/
    ux-findings-{pr-number}.md  <- Story 5 output (per PR)

tools/
  scanner/
    README.md
    validate-registry.sh        <- Story 3
    backend/
      proto_scanner.go          <- Story 1
      marker_scanner.go         <- Story 1
      cmd/main.go               <- Story 1
    frontend/
      src/
        component-scanner.ts    <- Story 2
        gap-reporter.ts         <- Story 2
        main.ts                 <- Story 2
  coverage/
    feature-coverage.ts         <- Story 4
  ux-analysis/
    analyze.ts                  <- Story 5
  ci/
    detect-feature-changes.sh   <- Story 6

tests/e2e/
  playwright.config.ts          <- Story 4, 6 (modified)
  accessibility.spec.ts         <- Story 5 (new)
  session-lifecycle.spec.ts     <- Story 4 (new)
  history-search.spec.ts        <- Story 4 (new)
  workspace-management.spec.ts  <- Story 4 (new)
  lighthouse.config.js          <- Story 5 (new)
  pages/
    SessionsPage.ts             <- Story 4 (new page object)

.github/workflows/
  registry-validation.yml       <- Story 3 (new)
  ux-analysis.yml               <- Story 5 (new)
  e2e-video.yml                 <- Story 6 (new)
  build.yml                     <- Story 4 (modified: add coverage step)
```

---

## Known Issues

### Potential Bug: Go AST Scanner Picks Up Generated Proto Files [SEVERITY: Medium]

**Description**: The Go scanner walks `server/services/` and may also pick up generated `*.pb.go` files if the vendor directory or `gen/` directory is in scope. `gen/session/v1/session_grpc.pb.go` contains function signatures that look like handler registrations.

**Mitigation**:
- Scanner skips files where `filepath.Base(filename)` ends in `.pb.go`
- Scanner skips the `gen/` directory entirely
- Add a unit test with a fixture that contains a `.pb.go` file and verify it produces zero scanner hits

**Files likely affected**: `tools/scanner/backend/proto_scanner.go`, `tools/scanner/backend/marker_scanner.go`

**Prevention**: Add explicit exclusion list as a constant at the top of each scanner file; include in code review checklist.

---

### Potential Bug: E2E Tests Leave Orphaned tmux Sessions [SEVERITY: High]

**Description**: If an E2E test fails mid-execution before teardown runs (panic, timeout), the tmux session created during the test persists. Subsequent test runs on the same machine (particularly in CI retry scenarios) accumulate orphaned sessions. The `STAPLER_SQUAD_INSTANCE` isolation prevents data file conflicts but does not prevent tmux session naming conflicts if the same process reuses the same PID after restart.

**Mitigation**:
- `TestServer.stop()` kills the server process on teardown, which triggers the server's own session cleanup
- Add a `global-teardown.ts` step that runs `pkill -f "stapler-squad.*--test-mode"` as a last resort
- Use unique tmux session prefix per test run: `STAPLER_SQUAD_TMUX_PREFIX=test-{pid}-{timestamp}-`
- Add a CI pre-step that verifies no orphaned test processes before starting

**Files likely affected**: `tests/e2e/helpers/test-server.ts`, `tests/e2e/global-teardown.ts`

**Prevention**: Always verify test teardown runs even on test failure by using Playwright's `test.afterAll` with try/catch.

---

### Potential Bug: Registry Drift from Marker-Proto Mismatch [SEVERITY: Medium]

**Description**: A developer adds a new RPC method to `session.proto` but forgets to add the `// +api:` marker to the handler in `server/services/`. The backend registry reflects the proto method (proto scan finds it) but `markerFound: false`. CI validation passes because the entry IS in the registry. Coverage gap silently widens.

**Mitigation**:
- CI validation step posts a warning comment for any entry where `markerFound: false`
- `make registry-diff` highlights entries without markers in yellow
- Add documentation to `CLAUDE.md` (or a `CONTRIBUTING.md`) stating the marker convention

**Files likely affected**: `tools/scanner/validate-registry.sh`, `.github/workflows/registry-validation.yml`

**Prevention**: Make the marker convention visible at the point of handler creation; add a comment block in `session_service.go` explaining the pattern.

---

### Potential Bug: Claude API Vision Hallucination on Stapler Squad-Specific UI Patterns [SEVERITY: Low]

**Description**: Claude's vision analysis may flag Stapler Squad's terminal-embedded UI elements (ANSI escape codes rendered in `<pre>` blocks, monospace terminal output areas) as accessibility violations or poor contrast. These are intentional design choices. Generic prompts will not exclude them.

**Mitigation**:
- Prompt includes explicit context: "This application renders terminal output in monospace pre elements. Terminal color contrast is intentional and should not be flagged as an accessibility violation."
- Claude output is advisory only; Axe Core is the blocking accessibility gate
- Track noise rate manually: after first 10 analyses, count how many findings required no action. If >10%, revise prompt.

**Files likely affected**: `tools/ux-analysis/analyze.ts`

**Prevention**: Seed the Claude prompt with design system context from `globals.css` and a brief description of terminal-rendering components.

---

### Potential Bug: TypeScript AST Scanner Misses Dynamic Feature Imports [SEVERITY: Low]

**Description**: React components loaded via `React.lazy(() => import('./path'))` or dynamic `import()` patterns are not resolvable by static TypeScript AST analysis. If a feature component is dynamically imported (likely for code splitting on route entry), the scanner will not find it even if the file has a `// +feature:` marker — because the scanner walks the import graph from the entry point, not all files.

**Mitigation**:
- Scanner does NOT walk the import graph; it walks all files in `web-app/src/` directly and checks for the `// +feature:` comment in each file
- Dynamic import resolution is not needed; the marker in the file is sufficient for discovery
- Document this design choice in `tools/scanner/README.md`

**Files likely affected**: `tools/scanner/frontend/src/component-scanner.ts`

**Prevention**: Scanner walks files, not imports. This is the intentional design (marker-based, not graph-based).

---

### Potential Bug: Video Codec Failure in GitHub Actions Ubuntu Runner [SEVERITY: Medium]

**Description**: Playwright uses VP8/VP9 codec for WebM video. The `ubuntu-latest` GitHub Actions runner includes the necessary libraries, but codec availability is not guaranteed after a runner image update. A silent codec failure causes Playwright to skip video generation without throwing an error. The CI job uploads an empty artifact and posts a misleading PR comment.

**Mitigation**:
- After tests complete with `RECORD_FEATURES=true`, add a validation step: `ls tests/e2e/test-results/**/*.webm | wc -l` must be > 0
- If count is 0, post a warning comment "Video recording failed — no .webm files found" and continue (non-blocking)
- Pin the GitHub Actions runner to `ubuntu-22.04` instead of `ubuntu-latest` for the video job to avoid runner image surprises

**Files likely affected**: `.github/workflows/e2e-video.yml`, `tests/e2e/playwright.config.ts`

**Prevention**: Explicit video file count check as a post-test validation step; non-blocking failure mode.

---

## Bug Prevention Checklist

Before submitting each story for review, verify:

- [ ] Scanner files explicitly list excluded patterns (`.pb.go`, `_pb.ts`, `*_test.go`)
- [ ] All E2E tests use `waitForSelector` or `waitForResponse`; no `waitForTimeout`
- [ ] All E2E tests have a teardown path that runs even on test failure
- [ ] Claude API calls are guarded: skip if API key absent; cap at N screenshots; max $1 per call
- [ ] Video CI step has explicit file count validation before posting artifact link
- [ ] Registry CI validation is non-blocking for first 2 weeks (comment in YAML)
- [ ] All new `make` targets are documented in `make help` output
- [ ] `data-testid` attributes are added to any new UI elements referenced in E2E tests

---

## Decision Summary (see ADRs for full rationale)

| Decision | Choice | ADR |
|----------|--------|-----|
| Registry storage | Static JSON committed to `docs/registry/` | ADR-001 |
| Backend scanner approach | Dual-scan: buf proto extract + Go AST marker scan | ADR-002 |
| Frontend scanner approach | TypeScript Compiler API + `// +feature:` markers | ADR-003 |
| UX analysis pipeline | Axe Core blocking + Claude advisory | ADR-004 |
| E2E harness | Extend existing Playwright + Allure reporter | (synthesis.md) |
| Video capture trigger | Feature-change PRs only (`RECORD_FEATURES=true`) | (synthesis.md) |
| Coverage target | 30% of critical paths (7 flows) | (resolved above) |
| Feature granularity | ConnectRPC method OR page-level React component | (resolved above) |
| Schema versioning | `"version": "1"` field; manual migration | (resolved above) |
