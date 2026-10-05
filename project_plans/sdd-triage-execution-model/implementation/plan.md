# Plan

## Recommendation (pending data)
Hybrid, cheapest-first: (a) instrument, (b) option 3 + option 4 hard cost/turn/subagent ceiling on the headless call as a backstop for all modes, (c) option 2 prompt-level phase bounds for sdd mode. Defer option 1 (tmux) unless data shows a need for mid-flight *compaction* rather than kill.

## Tasks
1. Instrument per-triage-call metrics (mode, elapsed, turns, subagent count, cost) to the log/metrics. Backend, 3h.
2. Query 2+ weeks of metrics; write distribution report. Docs, 2h.
3. Measure tmux session overhead (RSS, PTY, storage) per session in deployment. Infra, 2h.
4. Spike: expose headless stream-json events to the triage caller. Backend, 4h.
5. Turn/subagent/cost ceiling that cancels triageCtx, configurable per pipeline mode (coordinate with #884). Backend, 6h.
6. sdd prompt: phase-count/fan-out bound + early-wrap instruction. Backend, 3h.
7. Tests: ceiling fires on synthetic stream replicating #882 shape; default mode unaffected. Test, 4h.
8. ADR recording decision. Docs, 2h.

## Adversarial review
- Risk: prompt-level bounds (opt 2) are advisory; model ignored "no later turn" style guidance before → must pair with enforcement (task 5).
- Risk: killing mid-pipeline loses partial artifacts; ceiling should trigger a graceful "wrap up" first, then cancel.
- Risk: cost isn't known mid-call without stream events; fall back to turn count.
- Risk: tmux route increases memory pressure and orphan-reconcile complexity.
