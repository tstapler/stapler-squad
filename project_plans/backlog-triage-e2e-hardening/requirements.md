# Requirements: backlog-triage-e2e-hardening

**Date**: 2026-06-23
**Type**: bug fix
**Complexity**: 2 — targeted fixes across backend parser, frontend guard, and e2e test layer

## Problem Statement

The backlog triage feature (`TriggerTriage` RPC + headless pool) fails in the e2e
environment in two distinct ways:

1. **UI gate missing**: The "Trigger Triage" button in `BacklogItemDetail` is shown
   and enabled even when `item.repoPath` is empty. Items created via the empty-state
   inline form never set a `repo_path`. Clicking the button returns
   `CodeFailedPrecondition: "set repo_path before triggering triage"`. The error is
   displayed but gives no actionable guidance.

2. **Parser brittle on realistic LLM output**: `ParseHeadlessTriageResult`
   (`session/backlog_triage.go`) only strips leading triple-backtick fences. After a
   multi-step triage run (4 parallel research subagents + synthesis pass), Claude
   commonly emits a natural-language preamble before the JSON block (e.g.
   "Triage complete. Here is the result:\n\n```json\n{...}\n```"). The JSON unmarshal
   fails → goroutine marks session `ended_at` with empty `triage_result` → UI
   permanently shows "failed" state with a retry button.

Both failures are confirmed by static code analysis. There are no e2e tests for the
triage flow so regressions are invisible in CI.

## Users / Consumers

- **Operator (Tyler)**: clicks "Trigger Triage" in the detail pane; expects triage to
  either run or explain why it cannot.
- **BacklogService goroutine**: drives the headless pool call; expects a parseable JSON
  response from Claude.
- **CI e2e suite**: should catch regressions on the triage happy path.

## Success Metrics

1. Clicking "Trigger Triage" on an item with empty `repoPath` is **blocked at the UI
   layer** — button is disabled and shows a tooltip ("Set repository path first").
2. `ParseHeadlessTriageResult` correctly extracts JSON from output that has a
   natural-language preamble **and** from output that has no preamble (regression-free).
3. An e2e test (`e2e:backlog-triage-*`) exercises: create item → set repo_path →
   trigger triage → verify loading indicator → verify item transitions to "ready".
4. All existing backlog e2e tests still pass.

## Constraints

- Must not break the existing non-triage backlog tests.
- The `ParseHeadlessTriageResult` change must pass the existing unit tests in
  `session/backlog_triage_test.go`.
- No new proto changes needed — this is purely a backend parser + frontend guard fix.
- E2e test must use a real (or mock) server that can return a deterministic triage result;
  no external LLM calls in CI.

## Scope

### In Scope

- `BacklogItemDetail.tsx`: disable "Trigger Triage" buttons when `!item.repoPath`;
  add `title` tooltip explaining the requirement.
- `session/backlog_triage.go → ParseHeadlessTriageResult`: make the JSON extractor
  robust — find the first `{` ... last matching `}` in the output after stripping any
  fence wrapper. Fall back gracefully if no JSON found.
- `session/backlog_triage_test.go`: add test cases for preamble-before-JSON and
  preamble-before-fenced-JSON.
- `tests/e2e/backlog.spec.ts`: add triage happy-path e2e test (requires backend to
  return mock triage JSON, or real headless call if claude binary available in test env).
- `tests/e2e/pages/BacklogPage.ts`: add `triggerTriage` and `waitForTriageComplete`
  helpers.

### Out of Scope

- Fixing the empty-state form to collect `repo_path` (separate UX initiative).
- Adding a "Set repository path" inline prompt in the detail pane (nice-to-have).
- Changing the headless pool or the triage prompt itself.
- Adding triage cancellation support (`handleCancelTriage` is a TODO).

## Open Questions

1. Does the CI e2e environment have `claude` binary available? If not, we need a way
   to stub the triage call (e.g., `STAPLER_SQUAD_MOCK_TRIAGE=true` env var that returns
   canned JSON immediately). **Resolution**: check `which claude` in CI config;
   if absent, add a `--mock-triage` flag to the test server for deterministic results.
2. Is the re-trigger flash (1-poll-cycle "failed" state between old session tombstone
   and new session appearing) user-visible enough to require a fix in this iteration?
   **Resolution**: defer; only fix if observed in manual testing.
