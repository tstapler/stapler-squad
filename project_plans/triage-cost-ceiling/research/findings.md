# Research findings

- VERIFIED: `total_cost_usd` is only available on the terminal `result` line (`session/headless/caller.go:58-63`, `sendFirstCallSuccess`); there is no mid-call cost signal today.
- VERIFIED: the first-call scan loop (`caller.go` `scanFirstCallLines` / `handleFirstCallLine`) already sees every stream-json line and can kill the subprocess via `terminateStream(cio, drainLines, err)` (same path as ErrIdleTimeout / ErrOutputCapExceeded). That is the natural hook.
- VERIFIED: `tokens.PricingTable` exists (`session/tokens`), already used by `GeminiCaller` and `BacklogService.SetTokenStore`; reuse it to estimate $ from usage rather than hard-coding prices.
- INFERRED (verify with a live `claude -p --output-format stream-json --verbose` capture): assistant lines carry `message.id` and `message.usage` {input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens}; a message split into several content-block lines repeats the same usage, so dedupe by message id (take max per id).
- VERIFIED: resumed-call path (`readResumedCallStream`) is separate; triage uses first-call path only if sessions aren't resumed — confirm, and apply the check there too if triage resumes.
- Error plumbing: `classifyHeadlessCallError` (`backlog_service_triage.go:2637`) is the single mapping point; add a case before the timeout heuristic.
- Options considered: (a) lower wall-clock — rejected (BUG-055 coupling); (b) post-hoc cost check — too late; (c) token ceiling only — robust to price drift but not $-meaningful; (d) USD estimate via PricingTable with token-ceiling fallback when model unpriced — chosen.
- Default sizing: incident cost $106.69; typical triage cost should be sourced from existing recorded triage costs (query backlog cost data) — propose default ~$15 pending data. UNVERIFIED.
