# Plan

Decision: ADR-029 (`docs/adr/ADR-029-sdd-triage-stays-headless-with-fanout-ceiling.md`). Research: `../research/findings.md`.

## Design
- `session/headless/fanout.go`: `fanoutCounter` — feed it each stream-json line; it counts distinct assistant message ids
  (turns) and `tool_use` blocks named `Agent`/`Task` (subagents). `exceeded(MaxTurns, MaxSubagents)` reports which limit.
- `CallOptions.MaxTurns` / `MaxSubagents` (0 = unlimited). `handleFirstCallLine` feeds the counter and calls
  `terminateStream(..., ErrFanoutCeilingExceeded)` on breach (same shape as the output cap). Counts are wrapped in the
  error (`*FanoutCeilingError`, `errors.Is`-compatible) so the abort is logged with the partial counters.
- `config.Config.HeadlessTriageMaxTurns` / `HeadlessTriageMaxSubagents` (defaults 600 / 120; 0 disables), applied in
  `backlog_service_trigger_triage.go` only when the item's pipeline mode is `sdd`.
- `classifyHeadlessCallError` → `fanout_ceiling`; the abort error (and the existing failure log line) carries `turns`/`subagents`.
  Counters for *completed* calls are NOT yet logged (headless has no logger/stats hook) — follow-up for AC1's instrumentation plan.
- sdd triage prompt gets an advisory fan-out budget (option 2) in `session/pipeline_mode_seed.go`.

## Tasks
1. Research + data (done, committed in findings.md). AC1, AC2.
2. ADR-029. AC3, AC4, AC6.
3. `fanoutCounter` + tests (turn count, dedupe by id, Agent/Task, malformed lines ignored).
4. Wire into pool scan loop + `CallOptions`; `ErrFanoutCeilingExceeded`; replay test of the #882 shape (1,094 turns,
   262 subagents) through the fake runner proving cancellation long before the 3 h budget; default-mode (no limits)
   unaffected. AC5.
5. Config fields + trigger wiring (sdd only) + classifier case + log fields; test for classifier and mode gating.
6. Prompt budget text; update prompt tests if any pin the template.
7. `make lint`/targeted `go test`, `gofmt`.

## Adversarial review
- Prompt-only bounds are advisory → enforcement is the stream ceiling (task 4).
- Abort loses partial artifacts → prompt budget asks for early wrap-up; thresholds generous and configurable.
- Cost invisible on cancelled calls → counters are logged on abort (partial-spend proxy).
- Merge overlap with #884 in `handleFirstCallLine`/classifier → independent accumulators, noted in ADR.
- Counting: stream splits one message across lines with the same id → dedupe by id; lines with no id count once each.
- False positives on legitimately huge items → per-config disable (`0`), distinct `end_reason` so it is visible.
