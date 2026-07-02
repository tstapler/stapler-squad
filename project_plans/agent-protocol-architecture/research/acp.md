# ACP (Agent Client Protocol) — Research

## What ACP is

ACP is JSON-RPC 2.0 over stdio that lets an editor/UI (the **client**) drive any coding agent
(the **server**) through structured messages — session lifecycle, prompt turns, streaming
updates, permission requests, and file/terminal access — instead of scraping a terminal UI.
This framing is confirmed by the spec.

- Spec / org: https://agentclientprotocol.com/ and https://github.com/agentclientprotocol/agent-client-protocol
- Client→Agent methods: `initialize`, `authenticate`, `session/new`, `session/load`,
  `session/prompt`, `session/set_mode`, `session/cancel` (notification), `logout`
- Agent→Client methods: `session/request_permission`, `session/update` (streaming
  notification — this is the structured replacement for "watch the terminal and guess the
  state"), `fs/read_text_file`, `fs/write_text_file`, `terminal/create`, `terminal/output`,
  `terminal/wait_for_exit`, `terminal/kill`, `terminal/release`
- Wire `protocolVersion` is stable at **1**; schema artifacts iterate independently via semver
  (1.17.0 as of 2026-06-29), with an `unstable`/v2 track for in-progress features. So the core
  is settled but the spec is still actively evolving.

## Maturity

- Created 2025-06-23, publicly announced by Zed ~Aug/Sep 2025 — young (~1 year old).
- Originally under `zed-industries`, now spun out to a neutral `agentclientprotocol` org,
  jointly governed by Zed and JetBrains (official partnership announced Oct 2025); Niko
  Matsakis is also on the core team.
- **Clients (editors) that speak ACP**: Zed (originator), JetBrains IDEs
  (IntelliJ/PyCharm/WebStorm), Neovim (via `codecompanion.nvim`), and other generic
  ACP-compatible editors.
- **Agents that speak ACP natively (as servers)**: Google Gemini CLI (native, launched
  alongside Zed's announcement), and per `codecompanion.nvim` docs, 20+ agents including
  Codex, GitHub Copilot CLI, Goose, Cursor CLI, Kimi CLI, Kiro, OpenCode, Docker Cagent,
  Mistral Vibe. **Claude Code is not in this native list.**

## Claude Code specifically: no first-party support

- No first-party Anthropic ACP support exists today. The tracking request is
  `anthropics/claude-code#6686` ("Feature Request: Add support for ACP") — closed
  2026-02-19 via stale-bot auto-close, never implemented. Commenters directly asked
  Anthropic to build native support; Anthropic has not responded in the thread. Zed itself
  commented on that issue inviting Anthropic to build it ("Let the Anthropic team know if
  you'd like native support with no adapters!"). No blog post, PR, or roadmap statement from
  Anthropic commits to this.
- The originally-linked bridge, `Xuanwo/acp-claude-code` (238 stars), was a one-week
  community project. Its author **self-deprecated it in issue #64** on 2025-09-03 once Zed
  shipped an official adapter — the repo is now archived/frozen, accepting no more PRs. This
  confirms the linked issue is not an active bug thread; it's an end-of-life notice.
- The successor now lives inside the protocol's own org:
  **`agentclientprotocol/claude-agent-acp`** (2,172 stars, actively maintained, same-day
  commits as of this research). Critically, it wraps the **official Claude Agent SDK**
  directly and translates SDK events into ACP JSON-RPC — per Zed's blog post
  (https://zed.dev/blog/claude-code-via-acp): "built an adapter that wraps Claude Code's SDK
  and translates its interactions into ACP's JSON RPC format." This is *not* a pty/terminal
  scraper — it's a different, cleaner integration surface than either the community bridge or
  stapler-squad's own tmux-scraping approach. But it is still not an Anthropic-maintained
  artifact, and it targets the SDK, not the `claude` CLI binary stapler-squad currently
  launches in tmux.

## Concrete mapping onto stapler-squad's code

Read in full: `session/session_driver.go`, `session/detection/*.go`,
`session/autonomous_driver.go`, `session/instance_tmux.go`.

| stapler-squad mechanism | What it does today | ACP replacement |
|---|---|---|
| `session/detection/detector.go` — `StatusPattern` regex tables | Regex-classifies raw tmux pane bytes into states (`Active`, `NeedsApproval`, etc.). Comments confirm the exact fragility history cited in the task: spinner frame set (`· ✢ ✳ ✶ ✻ ✽ ● ✦`) needing `[ \t]*` to catch indented sub-item spinners; `collapseCarriageReturns` to undo `\r`-overwritten spinner redraws before matching; the U+273B asterism char used as a turn-completion bullet with a rotating random verb. | Directly obsoleted for ACP-speaking agents — `session/update` notifications carry structured state (`agent_thought_chunk`, `tool_call`, `plan`, etc.) instead of rendered glyphs. |
| `session/detection/idle.go` — debounced idle state machine | Maps `DetectedStatus` → `IdleState` with debounce delay to prevent flicker. | Obsoleted — ACP's turn-boundary signal is explicit, no debounce-by-guessing needed. |
| `session/detection/approval.go` + `session_driver.go` `isStartupDialog`/`shouldApprovePrompt` (substring match on `"trust this folder"`, `"allow reading"`, etc.) | Terminal-text pattern matching to detect and answer permission/trust dialogs. | Obsoleted — this is exactly `session/request_permission`, a structured RPC instead of scraped dialog text. |
| `session_driver.go` `SendKeys` + read-back verification retry (snapshot pane, sleep 500ms, re-snapshot, retry up to 3x if unchanged) | Sends literal keystrokes via tmux and verifies they "landed" by diffing pane content. | Obsoleted — `session/prompt` is a direct RPC call with a real response, no injection-and-hope. |
| `session_driver.go` 2s poll ticker + timeouts (`driverReadyTimeout`, `driverTotalTimeout`, `driverInactivityTimeout`) | Polling loop compensating for no push signal. | Obsoleted for the polling *mechanism* — ACP pushes `session/update` — though similar timeout/watchdog policy would still be needed at a higher level. |
| `session/autonomous_driver.go` — LLM-driven orchestration loop (goal/tail prompt → `NEXT_MESSAGE`/`DONE` decision → inject next message) | Custom turn-continuation policy built on top of the scraping/injection primitives above. | **Only partially replaced.** ACP would swap out the plumbing feeding this loop (`waitForIdle`, `Preview()` tail-scrape, `SendCommandImmediate` injection) for structured `session/update` consumption and `session/prompt` calls. It would **not** replace the policy itself — the max-turns logic, stuck detection, rate-limit backoff, and PR-URL regex extraction are business logic that stays, just fed cleaner input. |
| `session/instance_tmux.go` `classifyProgram` (`claudeProgram` vs. catch-all `plainProgram`) | Only two program kinds; anything that isn't literally the `claude` binary passes through unchanged — aider, shells, arbitrary scripts all run as `plainProgram`. | **Not addressed at all.** ACP only covers agents that speak it. stapler-squad's `plainProgram` catch-all means the terminal-scraping driver and detection layer must remain **indefinitely** for every non-ACP program, regardless of Claude Code adoption. |

## Prior art in this repo

`project_plans/agent-protocol-architecture/requirements.md` already frames this exact
question (MCP vs. A2A vs. ACP), naming `session_driver.go` and the same fragility history
almost verbatim. This document is Phase 2 research for that thread; `research/mcp.md` and
`research/a2a.md` are sibling deliverables not yet written. No other prior ACP/Zed
exploration exists in `docs/`, `docs/adr/`, or code comments.

## Risk assessment: depending on a bridge vs. waiting for native

- **Bridge risk is real but improved from the originally-linked lead.** The Xuanwo bridge
  cited in the task is dead (self-archived). Its replacement,
  `agentclientprotocol/claude-agent-acp`, is materially healthier: backed by the protocol's
  own org (Zed + JetBrains), wraps the official Claude Agent SDK rather than scraping a pty,
  and shows active commit activity. This is a different risk class than "one developer's
  weekend project with an open issue" — but it is still **not Anthropic**, and stapler-squad
  would be betting core session-control on a third party keeping pace with every Claude Code
  CLI/SDK release indefinitely, with zero contractual or roadmap assurance from Anthropic.
- **Feature-parity is unverified.** `claude-agent-acp` wraps the SDK, not the CLI binary
  stapler-squad currently launches in tmux. Whether SDK-mediated sessions have full parity
  with CLI behavior (custom slash commands, MCP tool config, subagents, hooks, permission
  modes) was not verified in this research and is a real open question before committing.
- **The migration is structurally partial regardless of bridge quality.** Because
  `classifyProgram` supports arbitrary non-Claude programs (`plainProgram`), adopting ACP for
  Claude Code sessions does not let stapler-squad delete `session_driver.go` or
  `session/detection/` — it adds a second session-control code path alongside the existing
  one, for Claude-only sessions. That is a net increase in system complexity unless/until
  non-Claude program support is deprioritized, which is out of scope here.
- **UX/architecture tension worth flagging**: stapler-squad's whole model is "attach a real
  terminal (tmux, mux) and watch/interact with the raw session." ACP replaces the terminal
  with structured RPC — any feature that depends on raw terminal visibility would need a
  translation layer (re-rendering `session/update` into terminal-like output) to preserve that
  UX, which is nontrivial additional work not covered by adopting the protocol itself.

## Verdict: prototype-behind-a-flag

Not adopt-now: Claude Code has zero first-party ACP support, Anthropic has given no
roadmap signal despite a year of direct community requests, and the best available bridge —
while now healthier than the dead link the user found — is still a third party mediating the
core lifecycle-control path of a shipped product. Committing to it now means inheriting its
release cadence and any feature-parity gaps between the CLI and the SDK-mediated path.

Not wait-for-native either: the ecosystem underneath ACP is real and improving faster than
"wait and watch" implies — 20+ native ACP agents, JetBrains now co-governing the spec, and the
Claude bridge specifically upgraded from a dead weekend project to an org-backed adapter
wrapping the official SDK. Treating this as pure vaporware would be wrong.

The strongest argument for the middle path: **the migration can only ever be partial** given
stapler-squad's `plainProgram` catch-all for non-Claude tools, so there is no scenario where
adopting ACP lets the terminal-scraping driver and detection layer be deleted — they must be
kept regardless. That means the honest first step is a feature-flagged, Claude-only
experimental session driver built against `agentclientprotocol/claude-agent-acp`, run in
parallel with (not replacing) the existing tmux driver, to validate reliability and feature
parity before any production commitment — never a wholesale swap.
