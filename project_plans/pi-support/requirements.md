# Requirements: pi-support

**Date**: 2026-09-02
**Type**: feature addition
**Complexity**: 4 — high-stakes / cross-cutting

## Problem Statement
stapler-squad manages AI coding-agent sessions (Claude Code, Aider, etc.) in isolated tmux sessions + git worktrees, but only Claude Code gets first-class treatment: session-resume flag injection (`ClaudeCommandBuilder`), a hooks-based approval-rules system (`.claude/settings.local.json` injection via `hook_injector.go`, classified/audited by `RulesService`/`approval_service.go`), and presumably status detection for the session list. The user wants to run **pi** (`@earendil-works/pi-coding-agent`, a TypeScript-extensible coding agent CLI) alongside Claude Code with the same level of integration, not just as an arbitrary free-text command.

## Baseline
Today, pi can already be launched in a stapler-squad session because the base program is a free-text string (`session/claude_command_builder.go` passes non-Claude commands through unchanged). But: reconnecting to a stopped pi session loses conversation continuity (no resume-flag injection), pi isn't a selectable preset in the session-creation UI, its live status (idle/working/waiting-for-input) isn't reflected in the session list, and pi tool calls are not gated by stapler-squad's approval-rules engine at all — a user relying on approval rules for safety has no coverage when running pi.

## Users / Consumers
- Tyler, running pi as a second/alternative coding agent alongside Claude Code sessions in stapler-squad's web UI.
- stapler-squad's existing approval/rules infrastructure (`RulesService`, `approval_service.go`, `hook_injector.go`) and session list/status UI, which need a pi-shaped input to extend to.

## Success Metrics
- Stopping and resuming a pi-backed session continues the same pi conversation (verified via `pi --session <id>` injection), matching today's Claude resume behavior — currently: conversation is lost on resume.
- The session-creation UI offers "pi" as a first-class preset (icon + default command), not just a manually typed string — currently: no preset exists.
- The session list reflects pi's live state (working / idle / waiting-for-input) sourced from `pi --mode json`/RPC events — currently: no status signal exists for non-Claude programs.
- A pi tool call that stapler-squad's approval rules would block for Claude Code is also blocked for pi, via a shipped pi extension calling the same approval webhook/classifier path — currently: zero enforcement.
- All of the above ship behind an opt-in setting, defaulting off, so existing Claude-only workflows are unaffected until explicitly enabled.

## Appetite
Large (3–6 weeks)
*(Scope must fit the appetite. If it doesn't fit, cut scope — do not move the deadline.)*

## Constraints
- No hard deadline.
- Must not regress existing Claude Code session behavior (resume, hooks, status) — pi support is additive and gated behind an opt-in setting.
- pi's own hook/extensibility model differs structurally from Claude Code's: Claude hooks are a static `.claude/settings.json` config entry (a shell command Claude invokes); pi's equivalent is a TypeScript **extension** (`.pi/extensions/*.ts`) that registers a `tool_call` event handler at runtime inside the pi process. stapler-squad must generate/inject and maintain this extension file rather than a JSON config block.

## Non-functional Requirements
- **Performance SLO**: not specified — extension injection and status-event parsing should not perceptibly slow session start/turnaround (same order as existing Claude hook injection).
- **Scalability**: not applicable — single-user, per-session scope, same order of magnitude as existing session count.
- **Security classification**: internal. The pi approval-hook extension is security-relevant (it gates tool execution) — treat with the same care as `hook_injector.go`/`approval_service.go`.
- **Data residency**: no special requirements.

## Scope
### In Scope
- **Resume support**: detect pi commands and inject the appropriate resume flag (`--session <id>` / `-c`) on session restart, mirroring `ClaudeCommandBuilder`.
- **UI preset/branding**: add "pi" as a selectable preset in session-creation UI (icon, default command `pi`), alongside the existing free-text option.
- **Status/output parsing**: run pi in `--mode json` (or RPC mode) to derive working/idle/waiting-for-input state for the session list, analogous to whatever mechanism exists for Claude Code today.
- **Approval-rules parity**: ship a stapler-squad-authored pi extension (TypeScript, registering a `tool_call` handler) that is injected into `.pi/extensions/` (mirroring how `hook_injector.go` injects `.claude/settings.local.json`), and wire its allow/block decisions through the same approval path (`RulesService`/classifier/webhook) that Claude Code hooks use today — not a parallel, disconnected rule system.
- **Multi-agent-in-one-session UX**: some UI treatment for pi as a genuinely first-class program choice alongside Claude Code within stapler-squad's existing session model (beyond "just another string a user could type") — exact UX to be defined in planning/design, not prescribed here.
- **Risk control**: all of the above gated behind an opt-in feature flag/setting, off by default.
- **Observability**: standard session logging (existing slog pipeline) for pi sessions; an extension install/health signal per session (so a silent injection failure doesn't silently disable enforcement); an approval-block audit log entry for every pi extension allow/block decision, consistent with existing Claude-hook approval auditing.

### Out of Scope
- Building general tooling to manage arbitrary third-party pi extensions/packages (`pi install`/`pi list`/etc.) — stapler-squad ships and manages only its own approval-hook extension.
- Supporting other non-Claude agent CLIs beyond pi (e.g. Aider) as part of this project — this is pi-specific.
- Changing or redesigning the existing Claude Code hook/approval system itself, beyond what's needed to let pi share the same webhook/classifier backend.

## Rabbit Holes
- **Extension distribution/versioning**: pi extensions are TypeScript, auto-discovered from `.pi/extensions/*.ts` (project) or `~/.pi/agent/extensions/*.ts` (global); project-local extensions "load only after the project is trusted" — the trust-gate interaction needs explicit handling or the extension silently won't load.
- **Status inference is heuristic**: pi's JSON/RPC event stream has no explicit "waiting for input" event — idle is inferred from the absence of activity after `agent_end`. This may need tuning against Claude's existing idle-detection heuristic (if one exists) to behave consistently in the session list.
- **RPC mode's approval surface is indirect**: RPC mode itself does not expose a tool-call approval gate to external processes — only an in-process *extension* can intercept via `tool_call` and then surface an `extension_ui_request`/`extension_ui_response` round-trip if using RPC mode simultaneously. Combining "stapler-squad runs pi in RPC mode for status" with "the approval extension also needs to block/allow" may require careful protocol sequencing to avoid deadlock or missed events.
- **RulesService integration depth**: "real parity" implies pi's extension consults the same classifier/rule source (`allRuleSpecs`, seed/claude-settings/user rules) rather than just forwarding raw approval requests to the webhook — scope this carefully in planning since `RulesService` is Claude-settings-shaped in places (e.g. `ReloadClaudeSettingsRules`, `claude-settings`-sourced rule filtering).
- **Multi-agent-in-one-session UX** has no prescribed design yet — could balloon; planning should timebox a concrete design rather than open-ended UX exploration.

## Alternatives Considered
- Leaving pi as a free-text command with no special handling (today's baseline) — rejected; loses resume continuity and all approval enforcement, which is the core ask.
- Building a fully separate, pi-only rules/audit system rather than integrating with `RulesService` — rejected per the "approval-rules parity is in scope" decision; would fragment approval auditing across two disconnected systems.

## Feasibility Risks
- pi's extension API (`ExtensionAPI`, `pi.on(event, handler)`, `ctx.ui.confirm()`) is third-party and versioned independently of stapler-squad; a pi upgrade could change or break the injected extension's contract with no warning.
- Project-local extension trust-gating could block the approval extension from loading on first run of an untrusted directory, silently disabling enforcement unless the health-metric observability requirement catches it.
- No public API stability guarantee was found in the fetched docs (`https://pi.dev/docs/latest/extensions`, `/rpc`, `/json`, `/sessions`, `/usage`) — treat pi's CLI/extension surface as subject to change; research phase should re-verify against the installed pi version before implementation.

## Observability Requirements
- Standard session logging: pi sessions use the existing `slog`-based `staplersquad.log` pipeline, same as any other program.
- Extension install/health metric: per-session signal indicating whether the pi approval-hook extension was successfully injected and loaded (detect and surface injection or trust-gate failures rather than failing silently).
- Approval-block audit log: every allow/block decision made by the pi extension is logged in a form consistent with existing Claude-hook approval auditing (`approval_service.go`/`audit.go`).

## Risk Control
Feature flag / opt-in setting: all pi-support functionality (resume injection, UI preset, status parsing, approval-hook extension injection) ships disabled by default behind a setting, so existing Claude-only workflows are unaffected until a user explicitly opts in.

## Open Questions
- Should the pi approval extension call the *same* webhook endpoint (`hookEndpoints`/remote-approval curl command) that Claude hooks call, or a new pi-specific endpoint that then delegates into `RulesService`? (Research phase.)
- What does "multi-agent-in-one-session UX" concretely look like — a program picker per session, a way to switch mid-session, or something else? (Planning phase should produce a concrete design, timeboxed.)
- Does the installed/target pi version match the fetched docs (`pi.dev/docs/latest`) exactly, or is there a pinned version stapler-squad should target? (Research phase — verify against actual installed CLI.)
- How should idle/working/waiting-for-input state be surfaced in the session list model — is there an existing `SessionStatus`-like enum/field to extend, or does one need to be introduced? (Research phase.)
