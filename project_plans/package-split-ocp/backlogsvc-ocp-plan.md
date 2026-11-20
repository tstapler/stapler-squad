# backlogsvc in-place OCP refactors (roadmap Q2, answered yes 2026-11-19)

One refactor per PR, no behaviour change. Verified against `main` at 0b183b1d3 before planning.

## Step 1: sizing (each candidate checked against the code)

| # | Candidate | What the code shows today | Remaining work | Risk | OCP payoff |
|---|---|---|---|---|---|
| 1 | Admission `Gate` slice (Prepare/Admit/Outcome) | Already done: `spawnAdmissionSteps` in `backlog_service_spawn_admission.go` (11 steps, order pinned by `TestSpawnSessionAfterGates_should_ApplyAdmissionChecksInOrder`). No Prepare/Outcome phase exists in the code to model. | None. A Prepare/Outcome split would be speculative (no second shape to fit). | n/a | Already "new check = function + row". |
| 2 | One `CacheInvalidator` | Already done: `CacheInvalidator` + `invalidateEngineCache` in `backlog_service.go`; pipeline, liveness and stage-config all route through it. `stageConfigEngine` is typed `CacheInvalidator`; `pipelineEngine`/`livenessEngine` stay read-side interfaces by design (duck-typed). | None worth a PR. | n/a | Done. |
| 3 | Headless methods in their own files | Partly done: `TriggerReReview` lives in `backlog_service_headless_run.go`. `SetHeadlessPool`, `SetBackendSelector`, `SetGeminiCaller`, `availabilityChecker`, `resolveHeadlessCaller` (88 lines) still sit in the 1581-line `backlog_service.go`. 2 production call sites of `resolveHeadlessCaller` (headless_run, trigger_triage), 4 existing tests. | Move the 88 lines to `backlog_service_headless_callers.go`. | Lowest: pure move, same package. | Locates the one place a new backend/program is wired; shrinks the hub file. |
| 4 | Session-role table replacing `Headless*UUIDPrefix` `HasPrefix` switches | Already done, and in `session/domain/headless_roles.go`, not backlogsvc: `headlessRolePrefixes` + `HeadlessRoleOf`/`IsHeadlessRole`. No `HasPrefix` switch on these prefixes remains in `server/mcp` or backlogsvc (`git grep HasPrefix server/services/backlogsvc` hits only steer-heading and diff parsing). The remaining raw uses are prefix *constructors* (measured with `git grep -n 'UUIDPrefix +' -- '*.go'`, non-test: `HeadlessTriageUUIDPrefix + uuid` 2 sites, `HeadlessReReviewUUIDPrefix + uuid` 2 sites, both in backlogsvc/debugseed) and `session/jules` (Jules is deliberately not a headless role). | None for backlogsvc. Optional follow-up: a `domain.NewHeadlessUUID(role)` minting helper for the 4 headless constructor sites (separate proposal, not a Q2 item). | n/a | Done. |

Conclusion: only candidate 3 is real work. Candidates 1, 2 and 4 landed in earlier waves; the roadmap line is stale.

## Step 2: PR order

1. **PR A (this PR) `refactor(backlogsvc): move headless caller wiring into its own file`** (candidate 3).
   - Files: `backlog_service.go` (-89), new `backlog_service_headless_callers.go`, new `backlog_service_headless_callers_test.go`, this plan.
   - Characterisation first (green on old code): 11 tests pin `SetHeadlessPool` claude registration, `SetBackendSelector` (swap + nil), `SetGeminiCaller` (nil guard, lazy map), and every `resolveHeadlessCaller` branch the existing 4 tests missed (raw claude with selector, empty with selector, known backend pinned, selector fallback unpinned, unknown name skips selector, claude with no pool wired, per-stage feature key reaches the selector).
   - Rollback: revert the commit; no schema, API or flag change.
2. No further PRs. If the owner wants a follow-up, the only open item is the optional minting helper in candidate 4 (touches `session/domain` plus 4 call sites; needs its own sizing).
3. Candidates 1 and 2: do not open PRs; record as done in `roadmap.md` (follow-up doc edit, no code).

## Behaviour-preservation checks for every PR in this plan
Normalised old-vs-new diff (only package/import header lines differ), `go test -list` name diff, full-package `-v` RUN/PASS count, at least 6 mutations of the moved code killed by the characterisation tests.
