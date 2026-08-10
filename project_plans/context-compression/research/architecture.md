# Architecture Research: Context Compression at 85% Threshold

Builds on prior confirmed research (do not re-derive):
- `project_plans/context-compaction-detection/research/architecture.md` — full PTY-bytes → status-detector → proto → UI-badge trace of Claude Code's own "X% until auto-compact" signal. Confirms the controller observes Claude Code as a PTY/tmux subprocess only.
- `project_plans/context-health-monitoring/research/architecture.md` and `implementation/plan.md` — degradation-detection + handoff-summary design, same PTY-observation model.

## 1. How the controller actually sends input — is "inject a synthetic user message" sound?

Two distinct paths exist, and the proposal conflates them:

**a) Interactive/tmux path (the one that matters for a long-running session).** `session/instance_tmux.go:133-166`'s `buildClaudeCommand` launches the *interactive* Claude Code TUI in a tmux pane as plain `claude --resume <uuid> [--append-system-prompt ...] [--allowedTools ...] [--permission-mode ...]` — no `--output-format` flag at all for the normal long-running case (that flag is added at `instance_tmux.go:157-158` only when `i.OneShot` is true, i.e. a single non-interactive query/response, not the persistent session).

Input delivery into that pane goes through `session/command_executor.go:297-304` (`executeCommand`):
```go
commandText := cmd.Text + "\n"
if _, err := ce.ptyAccess.Write([]byte(commandText)); err != nil { ... }
```
This is a literal keystroke write into the PTY that the TUI is reading — indistinguishable, from Claude Code's perspective, from a human typing into the terminal and hitting Enter. `ClaudeController.SendCommand`/`SendCommandImmediate` (`session/claude_controller.go:452,480`) are just a priority queue in front of that same write.

**Verdict on (1):** "inject a synthetic user message mid-session" is architecturally sound *as a mechanism* — the controller already has exactly this capability, used today for steering (`steer_session`), workflow commands, etc. There is no separate structured-turn API to hook into; a synthetic summarization turn would be delivered exactly like any other operator command: PTY keystrokes into the same `--resume`'d conversation. The risk is not feasibility, it's collision — the same queue/pane the operator or automation is actively driving would have to interleave a compression turn without corrupting an in-flight exchange, and the "response" would have to be scraped back out of rendered terminal text (see `session/session_driver.go:742-787`'s `parseJSONField`, which only works for `--output-format json`/`stream-json` output, unavailable in interactive mode) rather than a clean API return value.

## 2. Is "track token usage from Claude API response metadata" possible as literally proposed?

**No** — not from a raw API response, because the controller never calls the Anthropic API and never sees one. But the underlying capability — token usage tracking — already exists in this codebase via a different, already-built mechanism:

- `session/tokens/` (`doc.go:1-20`) is a full package that parses **Claude Code's own local JSONL transcript files** (`~/.claude/projects/<project-hash>/*.jsonl`, confirmed in `project_plans/token-monitoring/requirements.md:24`) — not live API responses, but the CLI's own on-disk session log, which does contain per-turn `input`/`output`/cache token counts (`session/tokens/parser.go`, `jsonl_types.go`).
- `TokenStore` (`session/tokens/store.go:53-225`) caches parsed results keyed by file path, invalidated on file-modtime change, kept fresh via **fsnotify** callbacks (`OnHistoryFileChanged`, doc.go:13-15) plus a background walker — i.e. near-real-time, event-driven updates as Claude Code appends to its own transcript, not a polling hack.
- `server/services/capacity_monitor.go:190-201` already reads live per-session context usage straight from this store: `parseRes.TurnTimeline[len(parseRes.TurnTimeline)-1].Input` gives the token count of the *last turn*, which is exactly the number needed to compute `used/context_window`.

So: token tracking is possible, but only by parsing Claude Code's own transcript output on disk (option (b) in the task's framing is actually option (a)-adjacent: not raw API metadata, but Claude Code's own structured record of it, already being consumed here) — no heuristic Go-side estimation is needed, and none is done today. The `-p --output-format json`/`stream-json` flag (`instance_tmux.go:158`, `session/headless/`) is a separate, narrower mechanism used only for one-shot non-interactive calls (e.g. headless pool workers in `session/headless/caller.go`), irrelevant to tracking the main interactive session's usage.

## 3. Realistic architectural alternative — and it's already built

`server/services/capacity_monitor.go` is not a hypothetical alternative — it is a **shipped, running service** that does almost exactly what this backlog item asks for, minus the compression-injection step:

- `checkThresholds` (`capacity_monitor.go:228-247`) computes `ContextTokensUsed / ContextTokensMax` from the same `TokenStore` and fires when it crosses `config.CapacityConfig.ContextWindowWarnPct` — a **configurable per-workspace threshold**, default **0.75** (`config/types.go:295-296,315-316`), directly analogous to the proposed default-0.85 knob.
- `handleTransitionTrigger` (`capacity_monitor.go:249-290`) rate-limits repeat firing to once per 5 minutes per session (thrash prevention, matching the "not more than once per N turns" acceptance criterion in spirit) and **publishes a UI notification event** (`m.eventBus.Publish(events.NewNotificationEvent(...))`) suggesting a switch to a different CLI/model — it does not inject a synthetic summarization turn.

Given (1) and (2), the sane scope is not a parallel Go-side `session/context_compressor.go` pipeline that reimplements Hermes's head/tail-protected summarization and writes it back in as fake user input. The proposal's own "Open architectural question" (requirements.md:66-80) already anticipates this correctly. The real gap, concretely:

- **Visibility** — `context-compaction-detection`'s plan already covers surfacing Claude Code's *own* auto-compaction signal (the "X% until auto-compact" status-line text) end-to-end into the UI.
- **Early warning before destructive auto-compact** — `capacity_monitor.go` already fires a threshold-crossing notification at 75% today; this item's "85% threshold" framing is a second, redundant signal path unless the two are explicitly reconciled (either raise/reuse `ContextWindowWarnPct` for this purpose, or justify why a second threshold at a different percentage is needed).
- **What's actually missing**, if anything, is `context-health-monitoring`'s territory: turning a capacity warning into an actionable operator choice (restart/resume-with-summary before Claude Code's own auto-compact kicks in and silently drops context), not a competing compression engine that writes synthetic turns into `--resume`'d PTY input.

Building `session/context_compressor.go` as filed would duplicate `CapacityMonitor`'s threshold/detection logic, duplicate `context-compaction-detection`'s visibility work, and add a genuinely risky new capability (unsolicited synthetic PTY input mid-conversation, competing with the operator's own commands in the same queue) for a problem the existing 75%-threshold warning + Claude Code's native auto-compact + `context-health-monitoring`'s handoff-summary design already address without it.

## Files referenced

- `session/claude_controller.go:452-512` — `SendCommand`/`SendCommandImmediate`
- `session/command_executor.go:270-304` — `executeCommand`, PTY write
- `session/instance_tmux.go:105-166` — `buildLaunchCommand`/`buildClaudeCommand`, `--resume`, OneShot-only `--output-format json`
- `session/session_driver.go:742-813` — JSON output parsing (OneShot-only), `tryExtractClaudeSessionID`
- `session/tokens/doc.go`, `store.go:53-225`, `parser.go` — JSONL transcript parsing, fsnotify-driven `TokenStore`
- `server/services/capacity_monitor.go:190-290` — existing live threshold-crossing warning system
- `config/types.go:295-316` — `ContextWindowWarnPct` (default 0.75)
- `project_plans/token-monitoring/requirements.md:15-24` — confirms JSONL transcript as the token-usage source of truth
