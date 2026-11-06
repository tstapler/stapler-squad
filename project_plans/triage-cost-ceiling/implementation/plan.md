# Plan

Design: in the headless first-call scan loop, parse `message.usage` from assistant lines, keep per-message-id max, price via `tokens.PricingTable`, and when running total > `CallOptions.MaxCostUSD` call `terminateStream(..., ErrCostCeilingExceeded)`. Triage supplies the ceiling from config.

Tasks
1. Capture real stream-json sample; add fixture; confirm usage/message-id shape (also resumed path).
2. `usageAccumulator` type in session/headless (dedupe, totals, estimate via PricingTable; token-ceiling fallback for unpriced model).
3. Wire into `handleFirstCallLine` (+ resumed path if used); add `ErrCostCeilingExceeded`; add `MaxCostUSD`/`MaxTokens` to `CallOptions`; plumb pricing into Pool.
4. Config field + default (`HeadlessTriageMaxCostUSD`), pass via CallOptions in `backlog_service_trigger_triage.go` (~L408).
5. `classifyHeadlessCallError` case `cost_ceiling`; non-retry handling; record spend + partial-output capture on item.
6. Unit tests (headless fake runner) for all AC 7 cases; classification test.
7. Regression: BUG-055 staleness tests untouched and passing; `make ci`.
8. Docs: short reference note on ceiling + tuning.

Adversarial review
- Risk: false-positive abort of legit expensive triage → generous default, configurable, distinct error so it's visible; consider warn-log at 50%.
- Risk: double counting repeated usage → dedupe by id, tested.
- Risk: price table drift/unknown model → token fallback.
- Risk: estimate under-counts vs CLI's total_cost_usd → ceiling is a coarse backstop; log estimate vs final cost to calibrate.
- Risk: dupl/complexity gates on scan loop → keep accumulator in its own type.
