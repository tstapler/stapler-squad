# Backlog Feature — Test Coverage Audit

Scope: Go unit/integration tests, Playwright e2e tests, and `docs/registry/features/` cross-reference for the
autonomous backlog triage/review pipeline.

## Headline answer: is the autonomous flow tested end-to-end?

**No.** There is no test — unit or e2e — that drives a real autonomous AI session through
triage → plan → execution → review with a live Claude subprocess doing the actual work, gated behind
default-CI-visible tests. The closest thing is:

- `server/services/backlog_triage_harness_test.go:328` `TestTriageHarness_RealClaude` — this is the
  **only** test in the repo that invokes a real `claude` binary for *triage* specifically. It is gated
  behind `//go:build harness` (not compiled by `make build`/`make ci`), requires `claude` in `$PATH`,
  and additionally self-skips (`t.Skip`) if the sandbox blocks `Setsid` subprocess creation or if `claude`
  isn't found. It only exercises **triage** (LLM produces a JSON summary), not execution or review.
- The **execution** phase (an autonomous session actually writing code) is only tested via
  `session/autonomous_driver_test.go`, and every test there uses `fakeHeadlessPool`/`panicPool` — a
  hand-rolled fake that returns canned strings like `"DONE: Created PR ..."` instead of ever invoking Claude.
  No test starts a real `AutonomousDriver` against a live LLM.
- The **review** phase (`session/backlog_review_test.go`) only unit-tests the JSON parser
  (`ParseHeadlessVerdictResult`) and prompt builder in isolation — never a live review call.
- The **lifecycle glue** connecting these phases (`session/backlog_lifecycle_test.go`,
  `session/backlog_integration_test.go`) is tested against **real ent/SQLite storage**, but the "sessions"
  in these tests are just rows manually inserted into storage — `onSessionExited(sessionUUID)` is called
  directly; no tmux session, no Claude process, no real driver ever runs.
- The e2e suite (`tests/e2e/backlog.spec.ts`) never triggers a real triage/execution/review cycle either —
  see below.

So today, trust in "does the autonomous pipeline actually work end-to-end" rests entirely on
`TestTriageHarness_RealClaude` (triage-only, opt-in, sandbox-fragile) plus manual testing. Execution and
review against a real model are **not covered by any automated test that runs in CI**.

## Coverage matrix

| Capability | Go unit/integration | e2e | Registry `tested` |
|---|---|---|---|
| Create item (`CreateBacklogItem`) | Real (storage + service logic; `backlog_service_test.go`) | Real (UI flow, `backlog.spec.ts` Empty State + Item Creation blocks) | `false` (stale) |
| List/filter items (`ListBacklogItems`) | Real (`TestListBacklogItems_DefaultFilterHidesTerminalStatuses`) | Real (filter zero-state, clear filters) | `false` (stale) |
| Status transition guards (`CanTransition`, `TransitionGuard*`) | Real, table-driven (`session/backlog_test.go`) | UI-shell-only (one transition: idea→ready via "Mark Ready" button) | `false` (stale) |
| Approve plan (`ApprovePlan`) | Real (`TestApprovePlan_HappyPath...`, `_MissingPlanArtifactsPath...`) | None | `false` (stale) |
| Trigger triage (`TriggerTriage`) — orchestration/guards | Real (mocked LLM pool: double-trigger guard, orphan reclaim, live-session block, error path) | UI-shell-only (`backlog-triage-gate-disabled`: only checks button is disabled when repoPath empty; never actually triggers/polls a triage run) | `false` (stale) |
| Triage LLM call + parsing | **Mocked** (`fakeHeadlessPool`) for all "harness" fake-pool subtests; **Real Claude** only in `TestTriageHarness_RealClaude` (build-tag gated, self-skipping) | None | `false` (stale) |
| Triage result parser (`ParseHeadlessTriageResult`) | Real, table-driven incl. markdown-fence/preamble edge cases (`session/backlog_triage_test.go`) | None | n/a |
| Autonomous execution driver (turn loop, DONE/NEXT_MESSAGE, max-turns, panic recovery, stop) | **Mocked** headless pool only (`session/autonomous_driver_test.go`) — no real Claude subprocess ever driven | None | n/a |
| Review verdict parsing / prompt building / security pre-gate | Real, unit-level only — no real LLM review call tested (`session/backlog_review_test.go`) | None | n/a |
| Lifecycle transitions on session start/exit (`BacklogLifecycleListener`) | Real storage, but session start/exit is simulated by direct method calls, not a real running session (`backlog_lifecycle_test.go`, `backlog_integration_test.go`) | UI-shell-only | `false` (stale) |
| Reconcile stuck items | Real (`IT-005`) | None | n/a |
| Review gate skip (`SkipReviewGate`) | Real (`IT-003`, lifecycle tests) | None | n/a |
| MCP `report_progress` tool (agent→backend AC updates) | Real, incl. permission-denied guards (`tools_backlog_test.go`) | None | n/a |
| MCP `get_backlog_item` / `submit_triage_result` | Real (`tools_backlog_test.go`) | None | n/a |
| Encryption of item-source tokens (GitHub PAT etc.) | Real round-trip + "no config → no encryption" edge case (`backlog_service_encryption_test.go`) | None | n/a |
| Crypto primitives (`EncryptDecryptToken`, wrong key, key size) | Real, table-driven (`session/backlog_crypto_test.go`) | None | n/a |
| Slash-command / context-file generation for spawned sessions | Real file-system assertions (`backlog_commands_test.go`, `backlog_context_test.go`) incl. prompt-injection-payload-is-inert test | None | n/a |
| Suggest Next Item (`SuggestNextItem`) | Backend registry entry exists; no Go test file/function found | **None — explicitly `test.fixme`** (see below) | `false` |
| Archive item, attach session, sources (create/update/delete), sync trigger/history, override verdict, trigger re-review UI | Partial (`TriggerReReview` has Go tests; others have no test found) | None | `false` for all |

Registry caveat: **every single one of the 20 `docs/registry/features/backend/backlog/*.json` entries has
`"tested": false, "testIds": []`**, despite `server/services/backlog_service_test.go` alone containing 20+
real Go tests covering `CreateBacklogItem`, `ApprovePlan`, `TriggerReReview`, `ListBacklogItems`,
`TriggerTriage`, etc. The registry has clearly not been regenerated (`make registry-generate`) since these
tests were added — it is **not a reliable signal** for backend coverage right now. No
`docs/registry/features/frontend/` entries exist for backlog at all, and no `coverage-gaps.json` file exists
in this checkout.

## Known gap called out in test names (verbatim)

`tests/e2e/backlog.spec.ts:426-433`:

```ts
test('e2e:backlog-suggest-next-item - Suggest Next feature (not yet exposed in UI)', async () => {
  // The SuggestNextItem RPC exists in the backend
  // (gen/proto/go/session/v1/sessionv1connect/backlog.connect.go) but
  // there is currently no "Suggest Next" button or data-testid in the
  // frontend UI (web-app/src/app/backlog/page.tsx). This test is marked
  // fixme until the feature is surfaced in the UI.
  test.fixme(true, 'SuggestNextItem RPC is implemented but has no UI button yet — add data-testid="backlog-suggest-next-button" and implement the test once the feature is exposed');
});
```

This matches the demo GIF `docs/demos/backlog-Backlog-Status-Tra-a8d60-ture-not-yet-exposed-in-UI-.gif`.
`SuggestNextItem` has a registry entry (`backlog:suggest-next`, `tested: false`) and a backend handler, but
no Go test function was found for it either — it is essentially unverified end-to-end (no unit test, no e2e,
UI absent).

## e2e suite: UI-shell coverage only, no real pipeline execution

`tests/e2e/backlog.spec.ts` (11 real tests + 1 `fixme` + 1 `test.skip()` fallback inside a conditional) covers:
Empty State (4), Item Creation and List (3), Filter Zero State (2), Page Navigation (2), Status Transitions (2),
Triage gate (1). All of it drives the React UI against the live server, but:

- The one "Triage" test (`e2e:backlog-triage-gate-disabled`) only asserts the Trigger-Triage button is
  `disabled` + has the right `title` tooltip when `repoPath` is empty. It never clicks the (enabled) button,
  never triggers a real triage, never polls for a triage result appearing in the UI.
- The one status-transition test that actually flips state (`e2e:backlog-transition-idea-to-ready`) only
  covers `idea → ready` via the "Mark Ready" button — a purely local guard check (AC criteria present), not
  an AI-driven transition. `ready → in_progress`, `in_progress → review`, `review → done`, and the
  review/approval UI are **not exercised by e2e at all**.
- No e2e test spawns a session, waits for triage/plan/execution/review, or interacts with the review verdict
  UI (approve/override).

## Skipped/stubbed tests

- `server/services/backlog_triage_harness_test.go:246` — `t.Skip("cannot find /usr/bin/true for pool pre-check")`
- `server/services/backlog_triage_harness_test.go:255` — `t.Skip("subprocess with Setsid blocked by seccomp sandbox — run this test from a real terminal...")`
- `server/services/backlog_triage_harness_test.go:273` — `t.Skipf("git %v failed: %v (%s) — cannot run real Claude triage test", ...)`
- `server/services/backlog_triage_harness_test.go:278` — `t.Skipf("write README: %v", err)`
- `server/services/backlog_triage_harness_test.go:337` — `t.Skip("claude binary not in PATH — skipping real Claude triage test")`
- `tests/e2e/backlog.spec.ts:432` — `test.fixme(true, 'SuggestNextItem RPC is implemented but has no UI button yet...')`
- `tests/e2e/backlog.spec.ts:267` — conditional `test.skip()` inside `Filter Zero State` if all 5 priorities already exist in the test DB (environment-dependent skip, not a permanent stub, but worth noting as flaky-by-design)

Net effect: **all five skip points in the harness test gate the only real-Claude test in the repo.** In a
typical sandboxed CI runner (which is exactly the environment this audit ran in), `TestTriageHarness_RealClaude`
self-skips on the `Setsid`/seccomp check before it ever gets to invoke Claude — meaning even when someone
remembers to run `-tags=harness`, CI-like environments silently skip the one test that would prove the
pipeline works against a live model.

## Additional notes

- `session/backlog_test.go`, `backlog_triage_test.go`, `backlog_review_test.go`, `backlog_crypto_test.go`,
  `backlog_context_test.go`, `backlog_commands_test.go` are all real, table-driven, pure-logic unit tests
  (state machine guards, JSON parsers, prompt builders, crypto primitives, file rendering) — high confidence,
  no mocking needed since there's no I/O to fake.
- `session/backlog_lifecycle_test.go` and `session/backlog_integration_test.go` are real integration tests
  against actual ent/SQLite storage (not mocked), but the "session" side of the interaction is simulated —
  they never run a real tmux/Claude session, only call the listener methods directly with synthetic UUIDs.
- `server/services/backlog_service_test.go` and `backlog_service_encryption_test.go` are real integration
  tests against real storage with a **mocked headless LLM pool** (`fakeHeadlessPool`) for anything that would
  otherwise call Claude.
- `server/mcp/tools_backlog_test.go` is real, testing the MCP tool surface an autonomous session uses to
  report progress back to the backend (permission checks, AC status updates, JSON envelope shape) — this is
  solid coverage of the *protocol* between an agent and the backend, but it's driven by direct handler calls
  in the test, not by an actual running agent.
- `go test ./session -list '.*Backlog.*|.*Autonomous.*'` confirms 37 real (non-dead) test functions compile
  and are discoverable in the `session` package, matching what was read above.
- `go test ./server/services -list ...` and `./server/mcp -list ...` currently **fail to build** in this
  worktree on an unrelated symbol (`sessionv1connect.GitHubUserServiceHandler` undefined in
  `server/services/github_user_service.go`) — this is a pre-existing generated-proto staleness issue in the
  checkout, not caused by or related to the backlog test files; their content was verified real via direct
  `Read` and `grep` instead.

## Key file references

- `server/services/backlog_triage_harness_test.go:79-421` (fake-pool harness + real-Claude test)
- `session/autonomous_driver_test.go:1-482` (all fake-pool driven)
- `session/backlog_lifecycle_test.go:1-572`, `session/backlog_integration_test.go:1-503`
- `session/backlog_review_test.go:1-154`, `session/backlog_triage_test.go:1-119`
- `server/mcp/tools_backlog_test.go:1-441`
- `tests/e2e/backlog.spec.ts:1-487`, `tests/e2e/pages/BacklogPage.ts:1-221`
- `docs/registry/features/backend/backlog/*.json` (20 files, all `tested: false`)
