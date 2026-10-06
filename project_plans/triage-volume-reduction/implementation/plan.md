# Plan: triage volume reduction (complexity 2)

## Design
1. **Config** (`config/config.go`): `MaxConcurrentTriage` (`max_concurrent_triage`, 0→8, clamp 1..64) and
   `HeadlessTriageModel` (`headless_triage_model`, ""→`family:sonnet`; `none` → account default, explicit opt-out).
   Accessors `MaxConcurrentTriageOrDefault()`, `HeadlessTriageModelOrDefault()` (nil-safe like siblings).
2. **Concurrency** (`backlog_service.go`): `triageSem` sized from `cfg.MaxConcurrentTriageOrDefault()` (cfg nil-safe).
   Existing goroutine already blocks on the semaphore = queue. Test: cap 1, two triages, assert second doesn't start CallBlocking until first finishes.
3. **Model** (`backlog_service_trigger_triage.go`): if `triageExecModel == ""` and the resolved program is claude/empty use `cfg.HeadlessTriageModelOrDefault()` as the model *for the call and the ItemSession `ResolvedModel`*; executor hash stays on RAW pipeline value (`triageExecModel`), per ComputeExecutorHash contract. Extract `resolveTriageModel(cfg, families, execModel)` for testing.
4. **Skip policy** (new `server/services/backlog_service_triage_skip.go`), revised after adversarial review (B1-B3):
   - `triageInputHash(title, description, criteriaTexts)` = sha256(title\0description\0each AC *text* only, in order)[:16]
     (no index/status/note, which mutate during work — fixes the mutable-AC concern).
   - `MaybeTriggerTriage` (new items only): skip with reason when item has non-empty parsed AC, or `PlanApproved`.
   - `AutoRespawnTriage` (stuck-item remediation): only the unchanged-hash rule (prior completed triage result's
     `InputHash` == current hash); checked **before** the queued->idea transition (B3). On skip: activity note
     ("human action needed: approve plan / edit item") + return nil; backoff exhaustion then surfaces the
     existing "paused" notification, which is accurate because retrying identical input is pointless. AC/plan skips are NOT applied here (B2).
   - Guidance-answer resume (`guidance_request_service.go` -> `AutoRespawnTriage`) must bypass the gate (B1): split
     `AutoRespawnTriage` into `autoRespawnTriage(ctx,id,applyGate bool)`; interface method (stuck remediation) = gate on;
     new exported `ResumeTriage` used by the guidance path = gate off. Manual `TriggerTriage` RPC untouched.
   - `HeadlessTriageResult.InputHash` set at result-persist time from the *post-triage* item content (original title/description +
     result.AcceptanceCriteria texts when applied, else original AC) so it matches the item as it will exist after triage.
5. PR description: measurement from `measurement.md`.

## Tasks (file → test)
- T1 config accessors → `config/config_test.go` table tests.
- T2 semaphore sizing + test (`backlog_service_triage_concurrency_test.go`).
- T3 `resolveTriageModel` + tests (pinned wins; default applied only when program is ""/claude; `none` opt-out; bad configured family falls back to the built-in sonnet default, never to account default).
- T4 skip policy + hash + `InputHash` field + wiring + tests (AC present, plan approved, unchanged hash, changed hash runs, activity note written, manual RPC still runs).
- T5 `go test ./server/services ./session ./config`, `make lint`.

## Risks / decisions
- Items with AC skipped stay `idea` (per pitfalls.md); note tells operator how to proceed. Decision: acceptable per AC2; no status change.
- Batching and external_url dedupe are out of scope (documented in requirements).
