# Research findings

## Verified in code
- `triageSem` cap 8: `server/services/backlog_service.go:567`; acquired `backlog_service_trigger_triage.go:378`.
- Call budget: `triageCallBudget = 3h` (`backlog_service_triage.go:411`); overridable via `livenessEngine.LivenessFor(BacklogStatusIdea, pipelineMode)` (`backlog_service_trigger_triage.go:408-413`). Wall-clock only: no turn/cost/token ceiling.
- sdd triage runs in a dedicated worktree (`backlog_service_trigger_triage.go` ~line 420), so a tmux variant would also need worktree + cleanup plumbing.
- `CapacityMonitor.checkIdleWaitLoop` (`capacity_monitor.go:~410`) works on `*session.Instance` + parsed transcript (`tokens.ParseResult`) — i.e. it needs a session with a transcript, not a headless call.

## Key insight (INFERRED)
Option 1 needs not just a tmux session but an agent-turn-level feedback loop; the existing monitor only knows how to compact/notify, which doesn't bound *fan-out* (#882 was many productive-looking turns, not an idle loop). Option 2/3 attack the actual failure mode more directly and cheaply. Transcript/stream-json of a headless call is already parseable, so *observation* may not require tmux; only *intervention* (compact) does, and kill is already possible via ctx cancel.

## Cheaper alternative (not in the issue)
Option 4: stream-json observation of the headless call — tail `--output-format stream-json`, count turns/subagent completions/cost, cancel `triageCtx` on threshold. Gives external observability + kill without PTY/session storage. Needs verification that the headless pool exposes the event stream.

## Gaps (UNVERIFIED — no data available locally)
- Duration/turn/cost distribution of sdd vs default triage: `~/.stapler-squad/logs/staplersquad.log` has no triage duration/cost fields to query. Need instrumentation (log per-call elapsed, turns, cost, mode).
- Per-tmux-session RSS: not measured here (host has 63 GB RAM, ~37 GB available at time of check, which is a poor proxy for the deployment under pressure).
