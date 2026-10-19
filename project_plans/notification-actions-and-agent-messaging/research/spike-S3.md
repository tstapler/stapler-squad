# Spike S3: Claude Channels as a push path into a live session

**Result: FAIL under the story's criterion (mechanism works, opt-in is interactive on every run).** Fallback applies: `SubmitContentWithEnter` via `SteerDelivery` remains the only transport; Channels is a future optimization. Results await owner review (plan owner item 8); the default (keystroke transport) proceeds.

- Date 2026-10-18, Claude Code `2.1.303`, `haiku`, private tmux `nam-spikeA`. Prototype (python stdio MCP server declaring `capabilities.experimental["claude/channel"]`, emitting `notifications/claude/channel` on a local HTTP POST to :62876) lives in the scratchpad, not committed.
- Docs read: https://code.claude.com/docs/en/channels and https://code.claude.com/docs/en/channels-reference. Key facts: channels are a **research preview**; `--channels` accepts only an Anthropic-maintained allowlist (or the org's `allowedChannelPlugins`); custom channels need `--dangerously-load-development-channels server:<name>` in an **interactive** session (ignored with `-p`); Team/Enterprise orgs block channels until an owner sets `channelsEnabled`; Pro/Max users without an org skip the org checks; events are dropped silently if the session did not load the server as a channel.

## Measurements (VERIFIED)

1. `claude --model haiku --dangerously-load-development-channels server:wake` in a project with `.mcp.json` naming the server. Two interactive prompts blocked startup: (a) the one-time per-project MCP approval (`New MCP server found in this project: wake`), (b) on every launch `WARNING: Loading development channels ... 1. I am using this for local development / 2. Exit`. After confirming (b) the banner read `Channels (experimental) messages from server:wake inject directly in this session`.
2. Push test, no send-keys to the pane after the confirmation: `curl -X POST --data 'wake 1234: PR #901 checks passed' http://127.0.0.1:62876/`. The TUI showed `← wake: wake 1234: PR #901 checks passed` as an inbound turn and the agent replied within ~12 s ("I couldn't find PR #901 from this session..."). So the push mechanism works as a user-like turn.
3. Relaunch with `--channels server:wake` (no development flag): banner `server:wake · server: entries need --dangerously-load-development-channels`; a POST (`wake 5678: second push`) returned `ok` from the prototype but **nothing appeared in the TUI** (silent drop, as documented).

## Why FAIL

The criterion requires no manual approval beyond a one-time launch setting ssq can set at spawn. Here the development-channel warning (measured 1(b)) needs an interactive confirmation on every launch, and the non-development path is allowlist-only. Satisfying the prompt would mean ssq sending a keystroke at spawn, which is the same transport class, plus a research-preview protocol that may change. Org-managed accounts additionally need `channelsEnabled`.

## Plan consequences

- Channels transport story stays disabled: `Delivery_should_useChannelWhenSpikePassed` is written and skipped with `t.Skip("blocked on S3: project_plans/notification-actions-and-agent-messaging/research/spike-S3.md")` (no Go test files in this milestone; the skip note is recorded here as required).
- Revisit when Channels leaves research preview or exposes a non-interactive opt-in.
