# pi testdata notes

`basic_session.jsonl` was captured live against pi 0.84.4
(`@earendil-works/pi-coding-agent`) via `--mode json --print`, prompting it to
list files and read a file in a scratch directory. This confirms the real
event vocabulary (`session`, `agent_start`, `tool_execution_start`,
`message_update`, `tool_execution_end`, `agent_end`, `agent_settled`) and
field names (`toolCallId`, `toolName`, `args`, `result`, `isError`) — see
Phase 1 spike findings in `project_plans/pi-support/implementation/plan.md`.

**Idle gap**: `--print` mode exits immediately after the turn settles, so no
natural inactivity gap exists in this capture to measure `piIdleGracePeriod`
against. `piIdleGracePeriod` is set to 5s to match the existing
`DefaultIdleDetectorConfig.IdleThreshold` precedent for Claude
(`session/detection/idle.go:39`), not an empirically observed pi-specific
value — revisit if real interactive/RPC-mode usage shows this is too
aggressive or too slow.
