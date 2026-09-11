# Implementation Plan: pi-support

**Feature**: First-class support for the `pi` coding-agent CLI (resume, UI preset, live status, approval-rules parity) alongside Claude Code, gated behind one opt-in feature flag.
**Date**: 2026-09-02
**Status**: Ready for implementation
**ADRs**:
- `../decisions/ADR-001-pi-extension-calls-permission-request-http-endpoint-directly.md`
- `../decisions/ADR-002-approval-extension-shipped-as-global-not-project-local.md`
- `../decisions/ADR-003-pi-approval-extension-fails-closed-on-network-error.md`

---

## Step 0.5 — Alternatives considered (creative pass)

Three whole-system shapes were considered before committing to the design below:

1. **"Clone the OpenCode playbook" — `ssq-hooks check --pi` exit-code contract, project-local extension.**
   *Strength*: maximum code reuse — `cmd/ssq-hooks`'s `check`/`install` dispatch, exit-code
   contract, and binary-copy step are already written and tested for four agents.
   *Weakness*: `ssq-hooks check` classifies against a disk-reloaded snapshot
   (`loadClassifier()`, `cmd/ssq-hooks/main.go:572`), not the live server's `RulesService` —
   fails the requirements' explicit "real integration with RulesService" decision, and a
   project-local extension is silently trust-gated per fresh worktree (pi-support's actual
   deployment pattern, one worktree per session). Rejected — see ADR-001, ADR-002.
2. **"RPC-mode everything" — drive pi in RPC mode for both status and approval, relaying
   `extension_ui_request`/`extension_ui_response` over stdin.**
   *Strength*: one process, one protocol, one Go-side reader instead of two independent
   channels (extension HTTP + JSON status stream).
   *Weakness*: RPC mode's approval surface is indirect and bidirectional — the Go side would
   have to correlate an in-flight `tool_call` with its RPC UI round-trip while *also* parsing
   the same stream for status events, with no documented deadlock-avoidance guidance
   (`research/architecture.md` §3, `research/pitfalls.md` PITFALL-3). Rejected — chosen
   instead: `--mode json` (one-way, status only) + an independent, in-process extension that
   calls out over plain HTTP for approval, decoupling the two concerns entirely (this is the
   design below).
3. **Chosen: split the two concerns onto two independent channels.** `--mode json` (one-way
   JSONL stdout) feeds a new `PiStatusSource` for session-list status only, with zero
   approval responsibility. The shipped `.pi/extensions/ssq-approval.ts` intercepts
   `tool_call` in-process and calls `/api/hooks/permission-request` directly over HTTP,
   with zero status responsibility. The only coupling between the two is a read-only
   reconciliation step (Epic 5.3): `PiStatusSource` consults the existing, program-agnostic
   `ExternalApprovalMonitor`/pending-approvals store (`session/external_approval.go:178`
   `GetAllPendingApprovals`) to override an inferred "idle" with "needs approval" when a pi
   tool call is actually blocked. *Strength*: each channel is independently simple (a JSONL
   reader; a ~40-line TS `fetch()` call) and independently testable; the coupling point reuses
   an existing generic store instead of inventing a new signal. *Weakness*: two channels means
   two things that can each independently fail — mitigated by Phase 4's health signal treating
   "health unknown" as "not enforced" regardless of what the status channel reports.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `piProgram` | New `programKind` sum-type variant (`session/instance_tmux.go:57-63`) holding a proven-pi base command string, mirroring `claudeProgram`. | Struct `piProgram{ base string }`; `sealedProgramKind()` method makes it a case of the existing sealed interface, not a new parallel abstraction. |
| `isPi(program string) bool` | Basename-match predicate mirroring `isClaude` (`session/instance_tmux.go:78-85`) — true iff the first whitespace token's basename (after path-stripping) is `"pi"`. | Guards against false positives like `pipenv` or `mypi`, same reasoning as `isClaude`'s doc comment. |
| `classifyProgram(program string) programKind` | Existing dispatcher (`session/instance_tmux.go:67-72`), extended with an `isPi` branch before falling through to `plainProgram`. | One new `if` branch, not a rewrite. |
| `buildPiCommand(base, piSessionID string) string` | New sibling to `buildClaudeCommand` (`session/instance_tmux.go:216-239`) that appends pi's resume flag (`--session <id>`, exact syntax confirmed by Phase 1's spike) when `piSessionID != ""`. | Never appends an unvalidated ID unshell-quoted — mirrors `shellQuote(claudeSessionID)` at `instance_tmux.go:222`. |
| `PiSessionData` | New struct (`session/storage.go`, sibling to `ClaudeSessionData` at `storage.go:190-197`) persisting pi's own session/conversation identifier across restarts. | Fields: `SessionID string`, `LastAttached time.Time`. No UUID validation — pi's ID format is confirmed-or-refuted by Phase 1's spike, not assumed. |
| `PiEvent` | Minimal discriminator struct (`{Type string}`) used to peek a `--mode json` JSONL line's `type` field before re-decoding into a concrete event struct — mirrors the peek-then-decode pattern already used for `classifier.PermissionRequestPayload`-style parsing in `cmd/ssq-hooks/main.go`. | New file `session/pi_adapter.go`, following `claude_adapter.go`/`agy_adapter.go`'s existing JSONL-parsing convention (`research/build-vs-buy.md` §1). |
| `PiSessionHeaderEvent`, `PiAgentStartEvent`, `PiAgentEndEvent`, `PiToolExecutionStartEvent`, `PiToolExecutionEndEvent` | Concrete typed structs for each `--mode json` event kind actually observed in Phase 1's spike capture. | Names and fields are placeholders pending the spike — do not hand-code field names from docs alone (PITFALL-3: `tool_call` vs `tool_execution_*` naming already inconsistent between doc pages). |
| `PiStatusSource` | New type (`session/pi_status_source.go`) that owns the `pi --mode json` subprocess, its `bufio.Scanner`-based reader, and an idle-inference timer; exposes `CurrentStatus() (detection.DetectedStatus, string)`. | Parallel to `ClaudeController`, not a `BinaryDetector` — `research/architecture.md` §2c is explicit this is not a regex-registry plugin. |
| `piIdleGracePeriod` | Duration constant: how long after the last `agent_end` with no new activity before `PiStatusSource` reports `detection.StatusIdle`. | Default value is a Phase 1 spike output, not guessed; starts as a `const` for easy tuning, not a magic literal inline. |
| `piSources` | New `*xsync.Map[string, *PiStatusSource]` field on `InstanceStatusManager` (`session/instance_status.go:27-38`), parallel to the existing `controllers` map. | Chosen over generalizing `ClaudeController` into a shared interface — see Pattern Decisions. |
| `RegisterPiStatusSource` / `UnregisterPiStatusSource` | New methods on `InstanceStatusManager` mirroring `RegisterController`/`UnregisterController` (`instance_status.go:40-48`), operating on `piSources` instead of `controllers`. | |
| `PiExtensionHealth` | New sum type (`server/services/pi_extension_health.go`): `PiExtensionHealthUnknown` / `PiExtensionHealthLoaded` / `PiExtensionHealthFailed`. | Three states, not a bool — `research/ux.md` §4 is explicit that "unknown" must be visually distinct from "confirmed failed," and defaulting to "assume healthy" is the exact failure mode PITFALL-1 warns about. |
| `PiExtensionHealthTracker` | New per-session server-side component recording the timestamp of the extension's first-run health ping and exposing `HealthFor(sessionID string) PiExtensionHealth`, with a timeout that flips unconfirmed sessions from `Unknown` to `Failed` after a grace window. | |
| `HookExtensionHealth` | New `HookName` constant (`server/services/hook_injector.go:14-33`'s block), mapped in `hookEndpoints()` (`hook_injector.go:100-109`) to `base + "/api/hooks/pi-extension-loaded"`. | Reuses the existing hook-URL registry for consistency (`research/architecture.md` §2b's recommendation), even though the pi extension reads this URL from a rendered template rather than via `InjectHooksConfig`'s JSON-merge path. |
| `ssqApprovalExtensionTemplate` | New Go string template (`cmd/ssq-hooks/main.go`, sibling to `openCodePluginTemplate` at `main.go:1199-1221`) — the TypeScript source of `.pi/extensions/ssq-approval.ts`, rendered with the live permission-request and extension-health URLs baked in. | No `execFileSync`/`ssq-hooks` binary call inside it — see ADR-001. |
| `installPi()` | New case in `handleInstall()`'s dispatch switch (`cmd/ssq-hooks/main.go:802-829`), writing the rendered extension to `~/.pi/agent/extensions/ssq-approval.ts` (global scope — see ADR-002). | |
| `PermissionRequestPayload.Source` | New optional field (`pkg/classifier/classifier.go:57-65`), `json:"source,omitempty"`, e.g. `"claude"` / `"pi"`. | Additive, backward compatible — Claude's existing curl hook command is unmodified and simply omits it (empty string), per `research/architecture.md` §4's EventStorming table gap note. |
| `pi-support` feature flag | New named key in the existing `Config.FeatureFlags map[string]bool` (`config/config.go:405-408`, `GetFeatureFlag`/`SetFeatureFlag` at `:1496-1540`). | No new flag infrastructure — reuse verbatim, per `research/features.md` §1f. |
| Approval extension health badge | New UI element on `SessionCard.tsx`, `role="img"` + descriptive `aria-label`, three-state (loaded/failed/unknown), shown only when `pi-support` is on and `program` is pi. | Modeled on the existing external-session/remote-host badge pattern (`SessionCard.tsx:607-623`) — NOT folded into `SessionStatus` (`research/ux.md` §4). |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|---|---|---|---|---|
| Program dispatch (`piProgram`) | Extend the existing sealed sum type `programKind` (`instance_tmux.go:57-63`) with a new case | type-driven-design + existing repo precedent | A separate `PiCommandBuilder` type mirroring the (largely unused) `ClaudeCommandBuilder` in `session/claude_command_builder.go`, as `research/stack.md`/`research/architecture.md` §2d proposed | Grepping the repo shows `ClaudeCommandBuilder.Build()` is **not called from any production code path** — the real resume-flag injection happens in `instance_tmux.go`'s `buildLaunchCommand`/`buildClaudeCommand` via the `programKind` sealed interface, which the codebase already generalized past a single-purpose builder. Extending the interface that's actually wired in is lower-risk than building a second, disconnected abstraction the research docs assumed was current. |
| pi resume-ID validation | No UUID regex gate (unlike Claude's `isValidUUID`) until Phase 1 confirms pi's actual ID format; validate only "non-empty, shell-safe" | Type-driven design (Parse-Don't-Validate, deferred) | Reuse `isValidUUID` unconditionally | `research/architecture.md` §2d and `research/stack.md` both flag pi's session-ID format as unconfirmed — hard-coding a UUID assumption risks silently dropping every legitimate non-UUID resume the way `ClaudeCommandBuilder.isValidUUID` would for Claude. |
| Approval decision transport | Extension calls `/api/hooks/permission-request` directly via `fetch()` | ADR-001 | `ssq-hooks check --pi` exit-code contract (mirrors OpenCode) | Classifies against a disk snapshot, not the live `RulesService` — see ADR-001. |
| Extension placement | Global (`~/.pi/agent/extensions/`) | ADR-002 | Project-local (`.pi/extensions/`) | Per-worktree trust-gate risk — see ADR-002. |
| Network-failure policy | Fail closed (deny) | ADR-003 | Fail open (mirror Claude's curl hook) | Defeats the purpose of adding enforcement — see ADR-003. |
| Extension-health signal | Explicit health-ping endpoint + three-state `PiExtensionHealth`, "unknown" treated as "not enforced" | Observer/health-check pattern (GoF-adjacent), PITFALL-1's mitigation | Infer health passively from "any permission-request payload ever arrived" | Passive inference can't distinguish "trust-gate skipped the file" from "no tool calls happened yet legitimately" — `research/architecture.md` §2b already rejects this for exactly that reason. |
| Status derivation for pi | New parallel `PiStatusSource` type + `piSources` map on `InstanceStatusManager`, feeding the same `InstanceStatusInfo` output struct | Strategy-adjacent (GoF), but deliberately NOT a shared interface — see below | Extract a `ProgramStatusController` interface that both `ClaudeController` and `PiStatusSource` implement | `ClaudeController`'s interface surface (`GetStatusAndIdleInfo`, `GetQueuedCommandsCount`, `GetCurrentCommand() *Command`) is shaped around PTY-scrollback scraping and a shell-command injection queue — concepts that don't exist for pi's JSON-event-stream model. Forcing a shared interface would require stub/dummy implementations of Claude-only concepts purely to satisfy a type signature; a second map keyed the same way, consulted as a fallback in `InstanceStatusManager.GetStatus()`, achieves the "same output shape" goal (`InstanceStatusInfo`) `research/architecture.md` §2c actually asks for, without the accidental complexity. |
| pi status vs. approval-block race | `PiStatusSource.CurrentStatus()` overrides an inferred Idle with `detection.StatusNeedsApproval` by consulting the existing `ExternalApprovalMonitor`/pending-approvals store (`session/external_approval.go:178`) | Read-only reconciliation against an existing source of truth | A new custom "extension is blocking" signal pushed from inside the `.ts` extension itself | `research/architecture.md` §3 point 3 proposed a new signal; the pending-approvals store already tracks exactly this fact program-agnostically (any payload that reached `HandlePermissionRequest` and got escalated is already in `ApprovalStore`) — reusing it avoids adding a third channel between the extension and Go side. |
| JSONL reader for `--mode json` | `bufio.NewScanner` with default `ScanLines` (LF-only) split, raised buffer size | Existing repo convention (`claude_adapter.go`, `agy_adapter.go`) | Any Unicode-line-aware or terminal-scrollback-oriented line splitter reused from Claude's TUI detection code | PITFALL-2: those splitters may treat `U+2028`/`U+2029` as line breaks, corrupting a JSON string value that legitimately contains one — `bufio.ScanLines` does not, by inspection of its stdlib source. |
| Program-preset UI entry | Add to existing `PROGRAMS: ProgramOption[]` array (`web-app/src/lib/constants/programs.ts:7-17`) plus a capability-gate map extension (mirroring `AUTO_APPROVE_SUPPORTED_AGENTS`, `autoApprove.ts:4`) | Existing repo convention | A new `SessionType`/multi-agent picker widget | `research/ux.md` §1 confirms none of the 7 session-creation-mode touchpoints (`docs/reference/session-creation-registry.md`) apply — `program` is a plain string, not a `SessionType`; a new widget would be scope creep against the requirements' explicit timebox on "multi-agent-in-one-session UX." |
| Approval-health UI indicator | Standalone badge, `role="img"` + `aria-label`, separate from `SessionStatus` | Existing repo convention (`SessionCard.tsx:607-623`'s external-session/remote-host badges) | Fold into `SessionStatus` enum as a new value | `research/ux.md` §4: extension-load health is a metadata fact about enforcement, not a session-lifecycle fact about whether the agent is responsive — conflating the two overloads one glyph with two meanings, which `SessionCard.tsx:309-311`'s own comment already treats as a boundary to respect. |

---

## Migration Plan

N/A — no database/ent schema changes. `PiSessionData`/`PiExtensionHealth` are in-memory/JSON-config-persisted (mirroring `ClaudeSessionData`'s existing JSON persistence in `session/storage.go`), not new SQL tables. `PermissionRequestPayload.Source` is an additive, `omitempty` JSON field with no migration needed.

## Observability Plan

- **Logs**: `slog`-based entries (existing pipeline) at: `piProgram`/`isPi` detection (debug, mirrors the existing non-logged fast paths — do not log on every session start per `claude_command_builder.go`'s own "gigabytes of output" lesson at line 44-46); `PiStatusSource` state transitions (info, "pi status changed to X" — same sink `research/stack.md` §"Go side" recommends); extension injection success/failure (`installPi()`, info/error); extension health-ping received/timed-out (info/warn); every classified pi permission request (already automatic via existing `RecordFromResult` — no new code, per `research/architecture.md` §2a).
- **Metrics**: `pi_extension_health_status{status="loaded|failed|unknown"}` gauge per active pi session (feeds the UI badge and gives an operator-facing signal beyond the UI); `pi_status_source_events_total{type}` counter for JSONL event throughput (cheap early-warning if pi's event vocabulary drifts per PITFALL-3 — a sudden new `type` value showing up unclassified is a signal worth counting, not just logging).
- **Alerts**: none new required — this is a single-user, opt-in, local-orchestration feature (requirements.md's Non-functional Requirements: "not applicable" for scalability/alerting). The UI health badge is the alerting surface for this feature, not a paging alert.

## Risk Control

- **Feature flag**: `"pi-support"` in `Config.FeatureFlags`, default `false` (via `GetFeatureFlag`'s nil-safe default). Gates all four surfaces: resume injection, UI preset, status source, extension injection/enforcement.
- **Rollback procedure**: standard revert via PR close + revert commit. Because everything is additive and flag-gated, no data migration reversal is needed. If the extension itself needs removal from a machine after a rollback, `ssq-hooks install pi --uninstall` (mirroring the existing `uninstall` flag pattern already used by `installService`, `main.go:1330-1331`) removes `~/.pi/agent/extensions/ssq-approval.ts`.
- **Staged rollout**: full rollout on merge, gated by the opt-in flag — the flag itself is the staging mechanism (single-user tool, no cohort concept applies).

### Fallback scope if Story 1.1.2's go/no-go gate fails

Pre-scoped now, per pre-mortem P1 #1, so tripping the gate is an immediate decision, not a new round of analysis:

- **Still ships**: Phase 2 (feature flag + resume), Phase 3 (UI preset/capability gating), Phase 5 (status detection). All three are independent of Phase 4 per the Dependency Visualization below — nothing about them assumes approval-blocking works.
- **Dropped entirely**: Phase 4 (Epics 4.1–4.3 — extension template/injector, health signal, `Source` audit field). No partial Phase 4 ships — a health-signal/injector for an extension that can't actually block anything has nothing left to gate.
- **requirements.md Success Metrics impact**: only the approval-parity metric ("A pi tool call that stapler-squad's approval rules would block for Claude Code is also blocked for pi...") becomes **not delivered**. The other four Success Metrics (resume continuity, UI preset, live status, opt-in default-off) are unaffected and still ship.
- **What happens next**: this fallback activates automatically the moment Story 1.1.2's finding is recorded — no escalation is needed to decide *whether* Phases 2/3/5 ship reduced-scope. The only remaining open choice is whether to additionally pursue a pinned older pi version confirmed to support blocking (optional follow-up, not a blocker to shipping this reduced scope).

## Unresolved Questions

**Phase 1 spike RESULTS (2026-09-02, verified live against pi 0.84.4 / `@earendil-works/pi-coding-agent`, run through Netflix's internal `pi-agent` distribution — `newt --app-type pi-agent pi -- ...` — to get real model access; upstream pi's own extension/CLI behavior is unaffected by Netflix's distro, only auth plumbing is):**

- [x] Resume flag/ID format (Story 1.1.3): `--session <uuid>` resumes correctly — confirmed by asking a resumed session what a file it had read in the prior invocation contained, with no re-read, and it answered correctly from prior context. ID format is a standard dashed UUID (pi reports it in the `session` header event's `id` field, e.g. `01a065a6-28ae-7973-8b45-c633ab1b138f`). `--continue`/`-c` also exists per `pi --help` (continues the most recent session) but was not separately live-tested since `--session <uuid>` alone satisfies Story 2.2.1. Session files are stored as `<session-dir>/<timestamp>_<uuid>.jsonl` and appended to across resumes (confirmed: same file grew, not a new one).
- [x] Extension-API event name/contract (Story 1.1.2): `pi.on("tool_call", handler)` fires after `tool_execution_start`, before the tool actually executes. `return { block: true, reason }` genuinely blocks — live-verified: a probe extension returning `{block:true, reason:"probe-block"}` caused `tool_execution_end.result` to be `{content:[{type:"text",text:"probe-block"}], isError:true}` and the model never saw real file/command output (it reasoned about the block itself). Confirmed for `read`, `ls`, and `bash` tool calls.
- [x] **Go/no-go gate: GO.** `{block:true}` blocks for real — Phase 4 proceeds as planned.
- [x] Uncaught/unexpected exception path (Task 1.1.2c): an uncaught `throw new Error(...)` from inside the `tool_call` handler **also fails closed by default** — `tool_execution_end.result.content[0].text` was the raw exception message, `isError:true`, tool did not execute. This is better than assumed: pi's own framework already defaults an uncaught exception to a blocked/errored tool call, so Task 4.1.1a's try/catch requirement is now defense-in-depth/clarity (a deliberate, readable deny with a stable `reason` string) rather than the only thing standing between a bug and fail-open — worth a one-line comment in `ssqApprovalExtensionTemplate` recording this, but no design change needed.
- [x] Global-extension trust behavior (Story 1.1.4): a probe extension placed at `~/.pi/agent/extensions/probe.ts` fired and blocked on the very first tool call in a brand-new, never-before-seen directory (`/tmp/pi-trust-probe`) — no trust-prompt interaction, no `trust`/`approve` markers anywhere in stdout/stderr. ADR-002's premise confirmed.
- [x] Idle-inference grace period (Story 1.1.1/5.2.2): **not empirically observable from this spike** — `--mode json --print` exits immediately once the turn settles, so no natural inactivity gap exists in a `--print`-mode capture to measure against. Set `piIdleGracePeriod = 5 * time.Second`, matching the existing `DefaultIdleDetectorConfig.IdleThreshold` precedent for Claude (`session/detection/idle.go:39`) rather than a pi-specific measured value. Revisit if real interactive/RPC-mode usage shows this is wrong. Documented in `session/detection/testdata/pi/NOTES.md`.
- [x] Real event vocabulary/field names confirmed and captured to `session/detection/testdata/pi/basic_session.jsonl`: `session` (`version`, `id`, `timestamp`, `cwd`), `agent_start`, `agent_end` (carries the full `messages` array, not a lightweight event), `agent_settled`, `tool_execution_start` (`toolCallId`, `toolName`, `args`), `message_update` (streaming deltas, incl. `toolcall_start`/`toolcall_delta`/`toolcall_end`, `text_start`/`text_delta`/`text_end`, `thinking_*`), `tool_execution_end` (`toolCallId`, `toolName`, `result.content[]`, `isError`), `turn_start`/`turn_end`. Note `agent_end`'s `messages` array is large (includes full cost/usage/thinking-signature data per turn) — worth confirming `pi_status_source_events_total` and any per-event logging don't log this payload verbatim (Observability Plan's existing "no gigabytes of output" caution already covers this).
- [ ] Whether `InstanceStatusInfo.PendingApprovals` is actually populated for Claude sessions today via some call site outside `InstanceStatusManager.GetStatus()` (it is declared on the struct but that method's body, as read, never assigns it) — blocks Story 5.3.1's choice of where to wire the pi-approval-block override — owner: implementer, quick trace of `PendingApprovals`' write sites before writing Story 5.3.1's code (not a research-phase blocker, a five-minute grep before that specific task).
- [ ] Concrete values for the health re-ping interval (Story 4.2.3) and the status-subprocess relaunch retry count/backoff (Story 5.2.3) — both are placeholders pending implementation-time tuning, not researched constants — owner: implementer, at the time each story is coded.

**Environment note for future spikes in this repo**: this worktree had no model-provider credentials configured for `pi` directly (`pi auth check` returned `credentials_not_configured` for anthropic/openai/google). Netflix engineers get real model access transparently via the internal `pi-agent` Newt app-type (`newt --app-type pi-agent pi -- <pi args>`), which routes through the SBN dev-agent proxy (`localhost:9123`, requires Netflix VPN) with no manual API key — see `corp/pi-agent`'s README. This is Netflix-internal auth plumbing only; it does not change any upstream pi CLI/extension behavior documented above, so the findings above hold for any pi 0.84.4 install regardless of auth backend.

## Dependency Visualization

```
Phase 1: Verification Spike (no code ships; de-risks everything downstream)
  Epic 1.1 ── Story 1.1.1 (capture --mode json)
           ── Story 1.1.2 (capture tool_call/extension contract)
           ── Story 1.1.3 (confirm resume flag)
           ── Story 1.1.4 (confirm global-extension trust behavior)
        │        │             │             │
        ▼        ▼             ▼             ▼
Phase 2: Feature flag + resume (Epic 2.1 → Epic 2.2, needs 1.1.3)
  Epic 2.1 also includes Story 2.1.2 (disable-flag warning) — deliberately
  has NO dependency on Phase 4's installPi()/uninstall path (see Story
  2.1.2's "mandatory warning, not auto-uninstall" decision)
        │
        ├──────────────────────────────────────────┐
        ▼                                           ▼
Phase 3: UI preset/capability gating          Phase 4: pi approval extension
  (Epic 3.1, needs 2.1; independent of          Epic 4.1 (needs 1.1.2's go/no-go gate to
   Phase 4/5 otherwise)                            resolve go, 1.1.2's Task 1.1.2c uncaught-
                                                     exception finding, 1.1.4, ADR-001/002)
                                                  Epic 4.2 (health signal, needs 4.1)
                                                    Story 4.2.3 (periodic re-ping, needs 4.2.1)
                                                  Epic 4.3 (fail-closed policy, needs 4.1, ADR-003)
                                                       │
                                                       ▼
                                              Phase 5: Status detection
                                                Epic 5.1 (JSONL reader, needs 1.1.1)
                                                Epic 5.2 (PiStatusSource + idle timer, needs 5.1)
                                                  Story 5.2.3 (subprocess-death detection +
                                                    bounded relaunch, needs 5.2.1's subprocess
                                                    launch, Task 5.2.1c)
                                                Epic 5.3 (approval-block reconciliation, needs 4.2, 5.2)
                                                       │
                                                       ▼
                                              Phase 6: Observability, e2e, docs, registry
                                                (needs all of 2-5 substantially complete)
```

---

## Phase 1: Verification Spike

### Epic 1.1: Live pi CLI verification
**Goal**: Resolve every unknown research flagged as "must verify against the installed binary before coding" — PITFALL-1/2/3's shared root cause across two prior agent-parity efforts was skipping exactly this step.

#### Story 1.1.1: Capture a real `pi --mode json` event stream
**As a** developer implementing `PiStatusSource`, **I want** a real, captured JSONL transcript of a pi session's lifecycle, **so that** event type names/fields are copied from observation, not guessed from docs.
**Acceptance Criteria**:
- A captured transcript file exists covering session start, at least one tool call (start/update/end), and a period of inactivity.
  - *Given* pi 0.84.4 (or whatever version `npm view @earendil-works/pi-coding-agent version` currently reports) installed locally, *When* `pi --mode json <<< "list files in this directory"` is run and stdout is redirected to a file, *Then* the file contains a first line with `"type":"session"` and at least one `tool_execution_start`/`tool_execution_end` pair.
- The transcript is saved under `session/detection/testdata/pi/` for reuse by later unit tests.
  - *Given* the captured file above, *When* it's copied to `session/detection/testdata/pi/basic_session.jsonl`, *Then* `git status` shows it as a new tracked file (not gitignored — `testdata/` for other agents is already tracked, per `research/features.md` §1a's testdata convention).
**Files**: `session/detection/testdata/pi/basic_session.jsonl` (new)

##### Task 1.1.1a: Install pi locally and capture a basic session transcript (~5 min)
- Run `npm install -g @earendil-works/pi-coding-agent` (or the repo's preferred local-tool convention) and confirm `pi --version`.
- Run a short non-interactive `pi --mode json` invocation, redirect stdout to `/tmp/pi-capture.jsonl`.
- Files: none (local capture only)

##### Task 1.1.1b: Save the capture as versioned testdata and note the observed idle gap (~4 min)
- Copy `/tmp/pi-capture.jsonl` to `session/detection/testdata/pi/basic_session.jsonl`.
- In a short comment atop the file (or a sibling `NOTES.md` if the convention elsewhere uses one — check `session/detection/testdata/` for precedent first), record the wall-clock gap between the last `agent_end` and the next real activity, to seed `piIdleGracePeriod`.
- Files: `session/detection/testdata/pi/basic_session.jsonl`

#### Story 1.1.2: Capture the extension's `tool_call`/`ctx.ui.confirm()` contract
**As a** developer implementing the pi approval extension (`ssq-approval.ts`), **I want** to confirm the actual blockable event name and its allow/deny return contract, **so that** `ssqApprovalExtensionTemplate` isn't built against a doc page that's already inconsistent with the RPC page (PITFALL-3).
**Acceptance Criteria**:
- A minimal test extension confirms whether `pi.on("tool_call", handler)` fires and whether returning `{block: true, reason}` (or throwing, or calling `ctx.ui.confirm()`) is what actually prevents tool execution.
  - *Given* a throwaway `.pi/extensions/probe.ts` registering `pi.on("tool_call", async (event, ctx) => { console.error("PROBE", JSON.stringify(event)); return { block: true, reason: "probe" }; })` in a scratch trusted directory, *When* pi is asked to run a tool (e.g. read a file), *Then* stderr shows the logged event shape and the tool call is either blocked or not — both outcomes are informative and must be recorded.
- If `{block: true}` does not block, the probe is repeated using `ctx.ui.confirm()` and/or a thrown exception until one contract is confirmed to actually block.
  - *Given* the first probe's outcome, *When* it does not block, *Then* a second probe variant using `ctx.ui.confirm()` (or throw) is run and its outcome recorded in the same notes.
- **Go/no-go gate**: if NONE of `{block:true}`, `ctx.ui.confirm()`, or a thrown exception actually prevents the tool from executing, approval-parity (all of Phase 4) is blocked and must be escalated back to the user/requirements owner before Phase 4 starts — mirroring the "if refuted, ADR-002 must be revisited" language already used for Story 1.1.4's trust-gate finding.
  - *Given* all three variants above are confirmed non-blocking, *When* this finding is recorded, *Then* Phase 4 does not start until the user/requirements owner has reviewed the finding and chosen a path (revisit requirements, pin/require an older pi version confirmed to block, or ship status-only with approval-parity explicitly marked not deliverable for this pi version).
- Whether an *unexpected*, uncaught exception from the handler (simulating a bug, not a deliberate deny) allows or blocks the tool call is also recorded — see Task 1.1.2c.
  - *Given* a probe extension registering `pi.on("tool_call", async () => { throw new Error("unexpected bug"); })`, *When* pi is asked to run a tool, *Then* whether the tool call executes anyway is recorded; if it defaults to allow, this is a critical residual-risk finding that ADR-003 and Task 4.1.1a must account for (see below).
**Files**: scratch-only (a throwaway `.pi/extensions/probe.ts` outside the repo, deleted after); findings recorded in Task 1.1.2b's target file.

##### Task 1.1.2a: Write and run a throwaway probe extension against a real tool call (~5 min)
- Create `.pi/extensions/probe.ts` in a scratch trusted directory (not this repo) per the AC above.
- Run pi interactively, trigger a tool call, observe stderr and whether the call actually executed.
- Files: none (scratch, not committed)

##### Task 1.1.2b: Record the confirmed contract for Phase 4 to consume (~3 min)
- Write the confirmed event name, payload field names (`toolName`/`toolCallId`/`args` or whatever was actually observed), and the confirmed blocking mechanism into a short note at the top of `cmd/ssq-hooks/main.go`'s future `ssqApprovalExtensionTemplate` location as a `// Verified against pi 0.84.4 on 2026-09-02: ...` comment (written in Task 4.1.1, but the finding is captured here first so it isn't lost between sessions). If the go/no-go gate above was tripped (no mechanism blocks), record that explicitly here instead, and stop — do not proceed to Task 1.1.2c or Phase 4 until the escalation is resolved.
- Files: (finding captured for Task 4.1.1 to consume; no file changed in this task if 4.1.1 hasn't started yet — store the note in the spike's own scratch location, e.g. a comment block pasted into this plan's Unresolved Questions section update)

##### Task 1.1.2c: Probe the unexpected-uncaught-exception path (~4 min)
- Using the same scratch probe extension, replace the deliberate `{block: true}` handler body with one that throws an error unrelated to any allow/deny decision (simulating a template bug, not a policy choice), and observe whether pi executes the tool call anyway.
- Record the outcome alongside Task 1.1.2b's notes: this finding is a required input to ADR-003's residual-risk note and to Task 4.1.1a's try/catch requirement.
- Files: none (scratch, not committed)

#### Story 1.1.3: Confirm pi's resume flag and session-ID format
**As a** developer implementing `buildPiCommand`, **I want** to confirm pi's actual resume syntax, **so that** the injected flag doesn't silently no-op or error on restart.
**Acceptance Criteria**:
- The exact flag(s) and ID format are confirmed by running a real resume.
  - *Given* a pi session started, given a prompt, and exited, *When* `pi --session <the-id-pi-reported>` (or `pi -c`) is run again in the same directory, *Then* the prior conversation's context is visible in the resumed session, and the ID's format (UUID vs opaque token vs file path) is recorded.
**Files**: none (finding feeds Story 2.2.1's implementation directly)

##### Task 1.1.3a: Run a pi session, note its session ID, exit, and resume it (~5 min)
- Start `pi`, send one message, note how pi reports its session ID (header JSON line's `"id"` field, or CLI output).
- Exit, then run `pi --session <id>` and separately `pi -c`, confirming which flag (or both) resumes correctly.
- Files: none

### Epic 1.2: (implicit — no second epic; Story 1.1.4 folds into Epic 1.1 for grouping simplicity)

#### Story 1.1.4: Confirm global-extension trust behavior
**As a** developer relying on ADR-002, **I want** to confirm a freshly-installed global extension loads without a manual trust prompt, **so that** ADR-002's premise is verified, not just doc-derived.
**Acceptance Criteria**:
- A global extension loads and fires on the very first pi invocation in a brand-new, never-before-seen project directory.
  - *Given* `~/.pi/agent/extensions/probe.ts` installed (the same probe from Story 1.1.2, moved to global scope) and a freshly-created empty directory pi has never seen, *When* `pi` is run in that directory and a tool call is triggered, *Then* the probe's log line appears with no trust-prompt interaction required.
**Files**: none (finding confirms/refutes ADR-002; if refuted, ADR-002 must be revisited before Phase 4 starts)

##### Task 1.1.4a: Move the probe extension to global scope and re-test against a fresh directory (~4 min)
- Copy `.pi/extensions/probe.ts` to `~/.pi/agent/extensions/probe.ts`.
- `mkdir /tmp/pi-trust-probe && cd /tmp/pi-trust-probe && pi` and trigger a tool call.
- Record whether a trust prompt appeared and whether the probe fired anyway.
- Files: none

---

## Phase 2: Feature Flag + Resume Support

### Epic 2.1: Feature flag plumbing
**Goal**: Establish the single opt-in gate every other phase checks before doing anything pi-specific.

#### Story 2.1.1: Add the `pi-support` feature flag
**As a** user, **I want** pi-support fully off by default, **so that** my existing Claude-only workflows are unaffected until I explicitly opt in.
**Acceptance Criteria**:
- `GetFeatureFlag("pi-support")` returns `false` on a config with no explicit setting.
  - *Given* a `Config{}` with `FeatureFlags: nil`, *When* `cfg.GetFeatureFlag("pi-support")` is called, *Then* it returns `false` (existing nil-safe behavior, no new code needed for this specific case — this AC is a regression guard, not new logic).
- Setting the flag persists and is re-readable.
  - *Given* a loaded `Config`, *When* `cfg.SetFeatureFlag("pi-support", true)` is called, *Then* `cfg.GetFeatureFlag("pi-support")` returns `true` and the on-disk config file's `feature_flags` object contains `"pi-support": true`.
**Files**: no new Go code in `config/config.go` itself (the generic mechanism already exists) — this story is a documentation/constant task.

##### Task 2.1.1a: Add a named constant for the flag key (~2 min)
- Add `const FeaturePiSupport = "pi-support"` near any existing similarly-named flag constants in `config/config.go` (search for how other flag keys, if any, are named — if none exist as constants today, add this as the first, with a one-line comment pointing at this plan).
- Files: `config/config.go`

##### Task 2.1.1b: Add a Go test asserting default-false and round-trip persistence (~4 min)
- Add `TestFeatureFlag_PiSupport_DefaultsFalseAndPersists` in `config/config_test.go` covering the two AC bullets above.
- Files: `config/config_test.go`

#### Story 2.1.2: Warn that disabling `pi-support` doesn't remove the global extension
**As a** user, **I want** a clear warning when I disable pi-support, **so that** I know pi usage outside stapler-squad — and any pi session started while it was on — is still gated by the globally-installed extension until I explicitly uninstall it.

**Decision — mandatory warning, not auto-uninstall**: of the two options pre-mortem P1 #4 named, a mandatory acknowledgment warning is chosen over automatically running `ssq-hooks install pi --uninstall` on flag-disable. Auto-uninstall would make a plain settings toggle (Phase 2) silently shell out to a capability that doesn't exist until Phase 4 ships (`installPi()`'s uninstall path, Story 4.1.2) — reversing this plan's own dependency order and adding a new subprocess-execution failure mode (what happens if the uninstall itself fails?) to what should be a simple config write. A warning has no such dependency, ships in Phase 2 alone, and fails safe: it informs rather than silently mutating machine state (`~/.pi/agent/extensions/ssq-approval.ts`) the user didn't explicitly ask to change via this toggle.

**Acceptance Criteria**:
- Toggling `pi-support` from `true` to `false` shows a mandatory, undismissable-until-acknowledged warning naming the exact residual effect, when the extension file exists on disk.
  - *Given* `pi-support` is currently `true` and `~/.pi/agent/extensions/ssq-approval.ts` exists, *When* the user toggles the flag to `false` in settings, *Then* a modal appears with text to the effect of: "Disabling pi-support does NOT remove the pi approval extension (`ssq-approval.ts`) already installed at `~/.pi/agent/extensions/ssq-approval.ts`. Direct `pi` usage outside stapler-squad remains subject to it. Run `ssq-hooks install pi --uninstall` to remove it." and the flag change is not persisted until the user clicks an explicit "I understand" acknowledgment.
- No warning (and no extra step) when there's nothing installed to warn about.
  - *Given* `~/.pi/agent/extensions/ssq-approval.ts` does not exist on disk, *When* the user toggles `pi-support` to `false`, *Then* no modal appears and the flag change persists immediately, same as toggling any other flag.
**Files**: `server/services/` (new small endpoint reporting whether the extension file exists — the check must run server-side, since `~/.pi` is on the server's filesystem, not the browser's), `web-app/src/components/settings/` (wherever the `pi-support` toggle lives)

##### Task 2.1.2a: Add a server-side "extension installed?" check (~3 min)
- A small handler (e.g. `GET /api/pi-extension-status`) that `os.Stat`s `~/.pi/agent/extensions/ssq-approval.ts` and returns whether it exists. No new persistent state — a stat call is enough.
- Files: `server/services/` (new or existing settings/config handler)

##### Task 2.1.2b: Add the mandatory acknowledgment warning modal (~5 min)
- A modal/dialog gating the flag-toggle's persistence on an explicit "I understand" click, per the AC's exact-effect wording.
- Files: `web-app/src/components/settings/` (wherever the `pi-support` toggle lives)

##### Task 2.1.2c: Wire the toggle-off handler to check first and skip the modal when nothing's installed (~3 min)
- Files: `web-app/src/components/settings/`

##### Task 2.1.2d: Test both branches (~4 min)
- Files: matching test file for the settings component/handler above.

### Epic 2.2: `piProgram` sealed-interface variant + resume-flag injection
**Goal**: Mirror `claudeProgram`'s resume behavior for pi, using the confirmed flag syntax from Story 1.1.3.

#### Story 2.2.1: Detect pi commands and inject the resume flag
**As a** user restarting a pi-backed session, **I want** the same conversation to continue, **so that** pi sessions behave like Claude sessions across restarts.
**Acceptance Criteria**:
- `isPi` correctly matches bare and path-qualified pi invocations, and rejects lookalikes.
  - *Given* the program strings `"pi"`, `"/usr/local/bin/pi"`, and `"pi --model x"`, *When* `isPi` is called on each, *Then* it returns `true` for all three.
  - *Given* the program string `"pipenv run pi-helper"`, *When* `isPi` is called, *Then* it returns `false` (basename of the first token is `pipenv`, not `pi`).
- `classifyProgram` returns `piProgram` for a pi command and `buildLaunchCommand` appends the confirmed resume flag when a `PiSessionData.SessionID` is present.
  - *Given* `i.Program = "pi"` and `i.piSession = &PiSessionData{SessionID: "abc123"}` (using whatever concrete ID Story 1.1.3 confirmed), *When* `i.buildLaunchCommand("")` is called, *Then* the returned string is `pi --session 'abc123'` (flag syntax per Story 1.1.3's finding, shell-quoted per `shellQuote`).
- No session data means no flag, matching Claude's "no-op, not an error" behavior.
  - *Given* `i.Program = "pi"` and `i.piSession == nil`, *When* `i.buildLaunchCommand("")` is called, *Then* the returned string is exactly `pi` with no resume flag appended.
**Files**: `session/instance_tmux.go`, `session/storage.go`, `session/instance.go`

##### Task 2.2.1a: Add `piProgram` to the sealed `programKind` interface (~3 min)
- Add `type piProgram struct{ base string }` and `func (piProgram) sealedProgramKind() {}` next to `claudeProgram`/`plainProgram` in `session/instance_tmux.go`.
- Files: `session/instance_tmux.go`

##### Task 2.2.1b: Add `isPi` and extend `classifyProgram` (~4 min)
- Add `isPi(program string) bool` mirroring `isClaude` (lines 74-85), matching basename `"pi"`.
- Extend `classifyProgram` (lines 65-72) with an `isPi` branch returning `piProgram{base: program}`, checked after `isClaude` (order doesn't matter today since the two never both match, but check-Claude-first preserves existing behavior byte-for-byte).
- Files: `session/instance_tmux.go`

##### Task 2.2.1c: Add `PiSessionData` struct (~2 min)
- Add `type PiSessionData struct { SessionID string; LastAttached time.Time }` in `session/storage.go` near `ClaudeSessionData` (lines 190-197).
- Files: `session/storage.go`

##### Task 2.2.1d: Add `buildPiCommand` and wire it into `buildLaunchCommand` (~5 min)
- Add `func (i *Instance) buildPiCommand(base, piSessionID string) string` mirroring `buildClaudeCommand` (lines 216-239), appending the Story-1.1.3-confirmed resume flag with `shellQuote`.
- In `buildLaunchCommand`'s `switch` (line 155), add a `case piProgram:` branch calling `i.buildPiCommand(p.base, <piSession ID lookup>)`.
- Files: `session/instance_tmux.go`

##### Task 2.2.1e: Add `piSession *PiSessionData` field on `Instance` and wire restart/resume capture (~5 min)
- Add the field near the existing `claudeSession *ClaudeSessionData` field on `Instance` (`session/instance.go`).
- Mirror the capture-on-`Restart` pattern at `instance.go:2164-2168` (`claudeSessionID` capture) for pi, guarded by `isPi(i.Program)` and the `pi-support` feature flag.
- Files: `session/instance.go`

##### Task 2.2.1f: Unit tests for `isPi`, `classifyProgram`, `buildPiCommand` (~5 min)
- Add table-driven tests covering all three AC bullets above, in `session/instance_tmux_test.go` (create if it doesn't exist, matching whatever test file already covers `isClaude`/`buildClaudeCommand`).
- Files: `session/instance_tmux_test.go`

---

## Phase 3: UI Preset and Capability Gating

### Epic 3.1: pi as a first-class program option
**Goal**: Make pi selectable and visually ranked as a peer to Claude Code, per `research/ux.md`'s "same picker, ranked visually" recommendation — timeboxed, no new widget.

#### Story 3.1.1: Add pi to the program picker
**As a** user creating a session, **I want** to pick "pi" from the same dropdown Claude Code is in, **so that** pi feels first-class rather than a manually-typed string.

This story is also the complete resolution of requirements.md's "Multi-agent-in-one-session UX" in-scope item: the existing flat picker, with pi added as one more `<option>`, *is* the UX treatment — no additional widget is planned. See the "Program-preset UI entry" row in Pattern Decisions for why a dedicated multi-agent widget was rejected as scope creep against the requirements' own timebox.

Note on requirements.md's "icon + default command" success-metric wording: the picker itself is a native `<select>`, which cannot render a per-option icon. The "icon" lives on the running session's card (Story 4.2.2's health badge), not the dropdown — this story covers "default command" only; see `design/ux.md` Surface 1.

**Acceptance Criteria**:
- pi appears in `PROGRAMS` immediately after Claude Code's entries, before Aider — `PROGRAMS` itself is unconditional (it's also the source `getProgramDisplay`/`isKnownProgram` use for sessions that already have `program: "pi"` set, e.g. from before the flag was disabled).
  - *Given* the updated `PROGRAMS` array, *When* a test reads `PROGRAMS.findIndex(p => p.value === "pi")`, *Then* the index is greater than `PROGRAMS.findIndex(p => p.value === "claude")` and less than `PROGRAMS.findIndex(p => p.value === "aider")`.
- `getProgramDisplay("pi")` returns the friendly label, and `isKnownProgram("pi")` returns `true`, regardless of the flag (these are label/lookup helpers, not the picker's rendered option list).
  - *Given* `program = "pi"`, *When* `getProgramDisplay("pi")` is called, *Then* it returns `"pi"` (the label field) rather than falling through to the raw-string branch.
- **The picker's rendered options are flag-gated**: with `pi-support` off, "pi" does not appear as a selectable option at all.
  - *Given* `pi-support` is `false`, *When* the session-creation panel renders its Program `<select>`, *Then* the rendered `<option>` list does not include `"pi"` — matching `design/ux.md`'s cross-surface "Opt-in invisibility" criterion.
  - *Given* `pi-support` is `true`, *When* the panel renders, *Then* `"pi"` appears in the rendered options at the position specified above.
**Files**: `web-app/src/lib/constants/programs.ts`, `web-app/src/components/sessions/OmnibarCreationPanel.tsx` (or wherever the `<select>` derives its option list)

##### Task 3.1.1a: Insert the pi entry into `PROGRAMS`, and gate it out of the rendered picker options when the flag is off (~4 min)
- Add `{ value: "pi", label: "pi", description: "@earendil-works/pi-coding-agent — TypeScript-extensible CLI" }` to `PROGRAMS` after the two Claude entries (after line 10) and before the Aider entry (line 11). `PROGRAMS` itself stays unconditional (label/lookup helpers need it regardless of the flag).
- Add a small derived helper (e.g. `getPickerPrograms(piSupportEnabled: boolean): ProgramOption[]`) that filters the `"pi"` entry out of `PROGRAMS` when `piSupportEnabled` is `false`, and have the creation panel's `<select>` render from that helper's output instead of `PROGRAMS` directly.
- Files: `web-app/src/lib/constants/programs.ts`, `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

##### Task 3.1.1b: Unit test the ordering, display-name behavior, and flag-gating (~4 min)
- Add/extend `programs.test.ts` (or equivalent) asserting the AC's index ordering and `getProgramDisplay`/`isKnownProgram` behavior, plus `getPickerPrograms(false)` excludes `"pi"` and `getPickerPrograms(true)` includes it at the expected position.
- Files: `web-app/src/lib/constants/programs.test.ts` (create if absent)

#### Story 3.1.2: Gate the approval-rules capability indicator per program
**As a** user, **I want** to see when approval-rules enforcement doesn't yet apply to my chosen program, **so that** I'm never falsely confident I'm covered.
**Acceptance Criteria**:
- A new capability check, `isApprovalExtensionSupported(program)`, returns `true` only for `"pi"` (and implicitly `"claude"` via the existing hook mechanism, represented separately since Claude's mechanism isn't the extension) — scoped narrowly to what Phase 4 actually ships.
  - *Given* `program = "pi"` and the `pi-support` flag on, *When* `isApprovalExtensionSupported("pi")` is called, *Then* it returns `true`.
  - *Given* `program = "opencode"`, *When* `isApprovalExtensionSupported("opencode")` is called, *Then* it returns `false` (OpenCode's own hook mechanism is out of scope here — this check is pi-specific, not a general capability map).
- The creation panel shows the same disabled+tooltip UX pattern `isAutoApproveSupported` already uses when the health signal (Phase 4) reports unhealthy.
  - *Given* a session-creation panel with `program = "pi"` and a hypothetical "extension health: failed" state, *When* rendered, *Then* a `role="alert"` warning is shown (per `research/ux.md` §3's escalation from the passive `preset-program-warning` span), not a silent hint.
**Files**: `web-app/src/lib/sessions/autoApprove.ts` (sibling function), `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

##### Task 3.1.2a: Add `isApprovalExtensionSupported` alongside `isAutoApproveSupported` (~3 min)
- Add the function to `autoApprove.ts` (or a new small `piApproval.ts` sibling file if keeping single-responsibility per file is preferred — match whatever convention `autoApprove.ts`'s own file-splitting precedent suggests), with the same basename-matching helper already defined there.
- Files: `web-app/src/lib/sessions/autoApprove.ts`

##### Task 3.1.2b: Unit test the capability check (~2 min)
- Extend `autoApprove.test.ts` with cases for `isApprovalExtensionSupported`.
- Files: `web-app/src/lib/sessions/autoApprove.test.ts`

##### Task 3.1.2c: Wire a placeholder warning UI (health state stubbed, real wiring in Phase 4) (~5 min)
- Add the `role="alert"` warning element to `OmnibarCreationPanel.tsx`, driven by a prop/stub that Phase 4's Story 4.2.2 will connect to the real health signal (avoids Phase 3 blocking on Phase 4's server work, while keeping the UI change isolated and reviewable now).
- Files: `web-app/src/components/sessions/OmnibarCreationPanel.tsx`

---

## Phase 4: pi approval extension (`ssq-approval.ts`) — Approval-Rules Parity

### Epic 4.1: Extension template + injector
**Goal**: Ship the security-critical `.ts` extension, applying Story 1.1.2's confirmed contract and ADR-001/ADR-002.

#### Story 4.1.1: Author `ssqApprovalExtensionTemplate`
**As a** stapler-squad maintainer, **I want** a generated TypeScript extension that gates pi tool calls through the real classifier, **so that** pi tool calls get the same coverage Claude's hook already provides.
**Acceptance Criteria**:
- The rendered template is syntactically valid TypeScript that pi's jiti loader accepts.
  - *Given* `ssqApprovalExtensionTemplate` rendered with a test URL pair, *When* the output is written to a scratch `.pi/extensions/ssq-approval.ts` and pi is started against a trivial project, *Then* pi starts without a load error (confirmed live, not just `tsc --noEmit`, per `research/architecture.md` §3 point 4's "generated-artifact contract test" recommendation).
- The handler POSTs a `classifier.PermissionRequestPayload`-shaped body and blocks/allows per Story 1.1.2's confirmed contract.
  - *Given* a probe tool call that the classifier would `AutoDeny` (e.g. matching an existing high-risk rule), *When* the real extension (not the Story-1.1.2 throwaway probe) intercepts it, *Then* the tool call does not execute and pi surfaces the deny reason to the user.
- The entire handler body is wrapped in a try/catch that defaults to blocking on any unexpected error, per Task 1.1.2c's finding that an uncaught exception is not guaranteed to fail closed on its own.
  - *Given* a fault injected into the handler body unrelated to the classifier call (e.g. a `TypeError` from malformed event data), *When* the tool call fires, *Then* the catch block returns/throws whatever Story 1.1.2's confirmed mechanism uses to deny, not allow — the generated template must not rely on pi's own default behavior for an uncaught exception.
**Files**: `cmd/ssq-hooks/main.go`

##### Task 4.1.1a: Write `ssqApprovalExtensionTemplate` using Story 1.1.2's confirmed event name/contract (~5 min)
- Add the Go string template constant, structurally mirroring `openCodePluginTemplate` (`main.go:1199-1221`) but using `fetch()` instead of `execFileSync`, per ADR-001.
- **Required**: wrap the entire handler body in a try/catch; the catch block must default to blocking (using Story 1.1.2's confirmed blocking mechanism) on any unexpected/uncaught error, not just on a deliberate classifier deny — per Task 1.1.2c's finding and ADR-003's residual-risk note. This closes the gap where fail-closed only covered the deliberate-decision path, not a bug in the template itself.
- Files: `cmd/ssq-hooks/main.go`

##### Task 4.1.1b: Write `ssqApprovalExtensionContent(permissionURL, healthURL string) string` (~4 min)
- Mirror `openCodePluginContent`'s safe-embedding approach (`json.Marshal`, not `%q`) for both URLs, per the existing comment's rationale at `main.go:1224-1228`.
- Files: `cmd/ssq-hooks/main.go`

##### Task 4.1.1c: Write a Go test rendering the template and asserting both URLs are safely embedded (~4 min)
- Mirror whatever test exists for `openCodePluginContent` (grep `cmd/ssq-hooks/*_test.go`), adapted for the two-URL case.
- Files: `cmd/ssq-hooks/main_test.go` (or the actual existing test file name)

##### Task 4.1.1d: Live-verify the rendered template loads in a real pi process (~5 min)
- Manually run the Task 1.1.1a-style capture flow against the real rendered output (not the throwaway probe), confirming no load error and confirming a deny actually blocks. Record the result as a comment near the template.
- Files: `cmd/ssq-hooks/main.go` (comment only)

#### Story 4.1.2: `installPi()` writes the extension to global scope
**As a** user running `ssq-hooks install pi`, **I want** the extension installed once per machine, **so that** it applies to every pi invocation without per-worktree trust friction (ADR-002).
**Acceptance Criteria**:
- `ssq-hooks install pi` writes `~/.pi/agent/extensions/ssq-approval.ts` and copies the `ssq-hooks` binary, mirroring `installOpenCode`'s two steps.
  - *Given* a clean `$HOME` with no prior install, *When* `ssq-hooks install pi` is run, *Then* `~/.pi/agent/extensions/ssq-approval.ts` exists with content matching `ssqApprovalExtensionContent`'s output for the current server's resolved URLs, and `~/.local/bin/ssq-hooks` exists.
- Re-running the install is idempotent.
  - *Given* the extension already installed, *When* `ssq-hooks install pi` is run again with no changes, *Then* the file's content is byte-identical to before (pure function of the two URLs, same reasoning as `patchOpenCodeHooks`'s doc comment).
**Files**: `cmd/ssq-hooks/main.go`

##### Task 4.1.2a: Add the `"pi"` case to `handleInstall()`'s dispatch switch (~2 min)
- Add alongside the existing `"claude"`/`"gemini"`/`"agy"`/`"open-code"` cases at `main.go:811-819`.
- Files: `cmd/ssq-hooks/main.go`

##### Task 4.1.2b: Write `installPi()` (~5 min)
- Mirror `installOpenCode()`'s structure (`main.go:1273-1327`): resolve `$HOME`, copy the binary, write the extension to `~/.pi/agent/extensions/ssq-approval.ts` via atomic temp-file+rename (mirroring `patchOpenCodeHooks`, `main.go:1241-1251`), print a best-effort detected pi version.
- Files: `cmd/ssq-hooks/main.go`

##### Task 4.1.2c: Add `printUsage()`'s target-list entry for `pi` (~2 min)
- Add `pi` to the usage string alongside the other install targets.
- Files: `cmd/ssq-hooks/main.go`

##### Task 4.1.2d: Test idempotency and atomic-write behavior (~4 min)
- Add a test mirroring whatever covers `patchOpenCodeHooks`'s idempotency, adapted for the global path.
- Files: `cmd/ssq-hooks/main_test.go`

### Epic 4.2: Extension-health signal
**Goal**: Close PITFALL-1's core gap — "installed" and "enforcing" must be independently observable.

#### Story 4.2.1: New health-ping endpoint and `PiExtensionHealthTracker`
**As a** stapler-squad server, **I want** to know within a bounded window whether a session's pi extension actually loaded, **so that** a silent trust-gate skip or load failure doesn't masquerade as enforcement.
**Acceptance Criteria**:
- The extension's confirmed load path (per Story 1.1.2/1.1.4's findings — e.g. a `session_start` event, or the first successful `tool_call` registration) triggers one POST to `/api/hooks/pi-extension-loaded` with the session's identifying header.
  - *Given* a pi session with the extension installed and the project directory not trust-gated (global scope), *When* pi starts and the extension's load-confirmation logic fires, *Then* `PiExtensionHealthTracker.HealthFor(sessionID)` transitions from `PiExtensionHealthUnknown` to `PiExtensionHealthLoaded` within the health-check grace window.
- No ping within the grace window flips the state to `Failed`, not staying `Unknown` forever.
  - *Given* a pi session started with `pi-support` on but the extension deliberately not installed (simulating a trust-gate skip), *When* the grace window (e.g. 10 seconds — tune during implementation) elapses with no ping, *Then* `HealthFor(sessionID)` returns `PiExtensionHealthFailed`.
**Files**: `server/services/hook_injector.go`, `server/services/approval_handler.go`, new `server/services/pi_extension_health.go`

##### Task 4.2.1a: Add `HookExtensionHealth` to the `HookName` block and `hookEndpoints()` map (~2 min)
- Add the constant and its `/api/hooks/pi-extension-loaded` mapping, per the Domain Glossary entry.
- Files: `server/services/hook_injector.go`

##### Task 4.2.1b: Define `PiExtensionHealth` sum type and `PiExtensionHealthTracker` (~5 min)
- New file, three-state type + a small in-memory map (session ID → last-ping timestamp + state), guarded by a mutex or `xsync.Map` matching the repo's existing concurrency idiom (`session/instance_status.go`'s `xsync.Map` usage).
- Files: `server/services/pi_extension_health.go`

##### Task 4.2.1c: Add the HTTP handler for the health-ping endpoint (~4 min)
- `HandlePiExtensionLoaded(w, r)` on `ApprovalHandler` (or a small new handler type, consistent with existing single-responsibility handler methods), records the ping via `PiExtensionHealthTracker`.
- Files: `server/services/approval_handler.go`

##### Task 4.2.1d: Register the route and wire the grace-window timeout (~4 min)
- Register the new route where other `/api/hooks/*` routes are registered (find the router setup, likely `server/server.go`), and start a grace-window timer per session on session start (guarded by the feature flag and `isPi(program)`).
- Files: `server/server.go`, `server/services/pi_extension_health.go`

##### Task 4.2.1e: Unit tests for the tracker's three-state transitions (~5 min)
- Cover: no ping → `Unknown` immediately, ping → `Loaded`, no ping past grace window → `Failed`, a late ping after `Failed` (decide and test the intended behavior — likely "still flips to `Loaded`" since a slow-but-eventually-successful load is still enforcement).
- Files: `server/services/pi_extension_health_test.go`

#### Story 4.2.2: Surface health in the UI
**As a** user, **I want** to see at a glance whether pi's approval enforcement is actually active for a session, **so that** I never mistake "pi-support is on" for "this session is protected."
**Acceptance Criteria**:
- The session card shows a three-state badge (loaded/failed/unknown) only when `pi-support` is on and the session's program is pi.
  - *Given* a session with `program: "pi"`, the flag on, and health state `Failed`, *When* `SessionCard` renders, *Then* a badge with `role="img"` and `aria-label="pi approval extension: not loaded — tool calls are unenforced"` is visible.
  - *Given* the same session with health state `Loaded`, *When* rendered, *Then* the badge's `aria-label` reflects "loaded"/enforced, distinct text from the failed case.
- The badge defaults to "unknown," never silently reading as "loaded."
  - *Given* a session created moments ago with no health signal yet, *When* rendered before the grace window elapses, *Then* the badge shows the "unknown" state, not "loaded."
**Files**: `web-app/src/components/sessions/SessionCard.tsx`, a new ConnectRPC field or polling endpoint exposing health state to the frontend (see Task 4.2.2b)

##### Task 4.2.2a: Add the three-state badge component (~5 min)
- Mirror the external-session/remote-host badge pattern (`SessionCard.tsx:607-623`) — icon + `role="img"` + `aria-label`, three variants.
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 4.2.2b: Expose `PiExtensionHealth` to the frontend (~5 min)
- Add a field to whichever existing session-status RPC response already carries `InstanceStatusInfo`-derived data (find the proto message via `docs/reference/feature-registry.md`'s pattern), or a small dedicated polling endpoint if no existing message fits without a larger proto change — prefer the smaller diff.
- Files: `proto/session/v1/session.proto` (if extending an existing message) + `make proto-gen` output (not committed, per repo convention) + the Go service populating the field

##### Task 4.2.2c: Wire the placeholder warning from Task 3.1.2c to the real health signal (~3 min)
- Replace the stub prop from Phase 3 with the real health state once a session exists (session-creation time itself has no health yet — this connects the *running-session* card badge, not the creation-panel warning, which stays stubbed/absent until Phase 4 ships; note this explicitly in a code comment referencing this story).
- Files: `web-app/src/components/sessions/SessionCard.tsx`

#### Story 4.2.3: Periodic health re-ping to survive a server restart
**As a** user with a long-running pi session, **I want** the health badge to recover automatically after a stapler-squad server restart, **so that** "unknown" doesn't silently persist forever for a session whose extension actually loaded fine and is still enforcing.
**Context**: `PiExtensionHealthTracker`'s state (Story 4.2.1) is in-memory only, and the extension currently pings its load-confirmation endpoint exactly once, at initial load. A server restart while a pi session is still running (an expected, documented event elsewhere in this repo — tmux/session state outlives server restarts) wipes the tracker's map; since the extension already fired its one-shot ping before the restart, nothing re-arms it, and the badge reverts to `Unknown` for the rest of that session's life even though enforcement is still active. Of the two options considered (re-ping periodically from the long-running extension process, vs. the server proactively re-probing on reconnect), a periodic ping from the extension is chosen as simpler: the extension is already alive in-process for the life of the pi session, needs no new server-side reconnect-detection logic, and reuses the exact same endpoint/payload as the initial ping — only the call site (a `setInterval` instead of a one-shot call) changes.
**Acceptance Criteria**:
- The extension re-sends its health ping periodically, not just once at load.
  - *Given* the rendered extension template, *When* its source is inspected, *Then* a periodic re-ping (e.g. every 2 minutes — tune during implementation, must be comfortably shorter than the grace window used to flip a *missing* ping to `Failed`) is scheduled for the lifetime of the pi process, in addition to the initial load-time ping.
- A server restart no longer permanently strands a live session's health at `Unknown`.
  - *Given* a pi session whose extension already reported `Loaded`, *When* the stapler-squad server process is restarted (simulating a service restart) and at least one re-ping interval elapses afterward, *Then* `PiExtensionHealthTracker.HealthFor(sessionID)` returns `Loaded` again, without requiring the pi session itself to restart.
- The re-ping does not itself flip a healthy session to `Failed` between intervals.
  - *Given* the grace-window timeout from Story 4.2.1, *When* it is compared against the re-ping interval, *Then* the grace window is documented (code comment) as sized to tolerate at least one missed re-ping (e.g. grace window >= 2x the re-ping interval) before flipping to `Failed`, so ordinary network jitter doesn't cause flapping.
**Files**: `cmd/ssq-hooks/main.go` (template), `server/services/pi_extension_health.go`

##### Task 4.2.3a: Add a periodic re-ping loop to `ssqApprovalExtensionTemplate` (~4 min)
- Add a `setInterval`-based re-send of the same health-ping POST used at load time, started alongside the initial ping; clear it on process exit if the extension API exposes a teardown hook (best-effort, not required).
- Files: `cmd/ssq-hooks/main.go`

##### Task 4.2.3b: Size the grace window relative to the re-ping interval (~2 min)
- Adjust `PiExtensionHealthTracker`'s grace-window constant (Story 4.2.1) so it comfortably tolerates one missed re-ping, and document the relationship in a code comment.
- Files: `server/services/pi_extension_health.go`

##### Task 4.2.3c: Test that a tracker reset (simulating a server restart) recovers to `Loaded` after one re-ping (~4 min)
- Construct a tracker, record a `Loaded` ping, discard and recreate the tracker (simulating restart), feed one more ping, and assert `HealthFor` returns `Loaded` again within the grace window.
- Files: `server/services/pi_extension_health_test.go`

### Epic 4.3: Audit distinguishability
**Goal**: Small, additive close-out per `research/architecture.md` §4's EventStorming gap note.

#### Story 4.3.1: Add `Source` to `PermissionRequestPayload`
**As an** operator reviewing approval analytics, **I want** to tell whether a decision came from Claude or pi, **so that** per-agent approval patterns are distinguishable if ever needed.
**Acceptance Criteria**:
- The field is optional and backward compatible.
  - *Given* Claude's existing curl-based hook command (unmodified), *When* it POSTs its existing JSON body (no `source` key), *Then* `HandlePermissionRequest` still parses it successfully with `Source == ""`.
  - *Given* the pi extension's new body including `"source":"pi"`, *When* posted, *Then* `HandlePermissionRequest` parses `Source == "pi"` and it is visible on the resulting audit/analytics record.
**Files**: `pkg/classifier/classifier.go`, `server/services/approval_handler.go`, `ssqApprovalExtensionTemplate`'s body construction

##### Task 4.3.1a: Add the field to `PermissionRequestPayload` (~2 min)
- `Source string \`json:"source,omitempty"\`` in `pkg/classifier/classifier.go:57-65`.
- Files: `pkg/classifier/classifier.go`

##### Task 4.3.1b: Thread it through to the audit/analytics record (~4 min)
- Wherever `RecordFromResult`/`recordResult`-equivalent logic writes an audit row, add the `Source` value (defaulting to `"claude"` when empty, for readability, at the recording boundary only — not by mutating the wire payload).
- Files: `server/services/approval_handler.go`

##### Task 4.3.1c: Set `source: "pi"` in the extension template's request body (~2 min)
- Update `ssqApprovalExtensionTemplate` (Task 4.1.1a) to include the field.
- Files: `cmd/ssq-hooks/main.go`

---

## Phase 5: Status Detection

### Epic 5.1: JSONL event reader
**Goal**: A protocol-correct, hand-rolled reader per PITFALL-2, using Story 1.1.1's real captured events.

#### Story 5.1.1: `session/pi_adapter.go` event types and reader
**As a** developer, **I want** a typed, tested JSONL reader for `pi --mode json` output, **so that** `PiStatusSource` has a reliable event feed.
**Acceptance Criteria**:
- The reader correctly parses every event type present in Story 1.1.1's captured transcript.
  - *Given* `session/detection/testdata/pi/basic_session.jsonl` fed line-by-line into the reader, *When* each line is processed, *Then* every line decodes into a known typed event with no "unrecognized type" error, and the session-header line's `version` field is read and logged.
- A line containing an embedded `U+2028` character is not mis-split.
  - *Given* a synthetic JSONL line containing a string value with a literal `U+2028` character, *When* the reader processes a buffer containing this line followed by a real newline-terminated next line, *Then* exactly two events are produced, not three (regression test for PITFALL-2).
**Files**: `session/pi_adapter.go`, `session/pi_adapter_test.go`

##### Task 5.1.1a: Define the event envelope and concrete event structs (~5 min)
- `PiEvent{Type string}` peek struct plus concrete structs for each type found in Story 1.1.1's capture (session header, agent_start/end, tool_execution_start/update/end at minimum).
- Files: `session/pi_adapter.go`

##### Task 5.1.1b: Implement the `bufio.Scanner`-based line reader with raised buffer size (~4 min)
- Mirror `claude_adapter.go`'s JSONL-reading shape; raise `scanner.Buffer(...)` above the default 64KB per `research/stack.md`'s note on large `message_update` deltas.
- Files: `session/pi_adapter.go`

##### Task 5.1.1c: Table-driven test against the captured transcript (~5 min)
- Feed `basic_session.jsonl` through the reader, assert every line decodes to a known type.
- Files: `session/pi_adapter_test.go`

##### Task 5.1.1d: Regression test for the `U+2028` line-splitting risk (~3 min)
- Construct the synthetic buffer described in the AC and assert exactly two events result.
- Files: `session/pi_adapter_test.go`

### Epic 5.2: `PiStatusSource` and idle inference
**Goal**: Feed `detection.DetectedStatus` values into the same output shape Claude's status uses, per the chosen Pattern Decision (parallel map, not shared interface).

#### Story 5.2.1: `PiStatusSource` owns the subprocess and maps events to status
**As a** user, **I want** the session list to show pi as Processing/Idle/Ready accurately, **so that** pi sessions behave like Claude sessions in the UI.
**Acceptance Criteria**:
- A `tool_execution_start` with no matching `tool_execution_end` yet reports `StatusExecuting`.
  - *Given* a `PiStatusSource` fed the captured transcript's events up to (and not including) the closing `tool_execution_end`, *When* `CurrentStatus()` is called, *Then* it returns `detection.StatusExecuting`.
- After `agent_end` and `piIdleGracePeriod` elapses with no further events, status becomes `StatusIdle`.
  - *Given* a `PiStatusSource` fed an `agent_end` event and then no further events for longer than `piIdleGracePeriod`, *When* `CurrentStatus()` is called, *Then* it returns `detection.StatusIdle`.
- Before any events arrive, status is `StatusReady`.
  - *Given* a freshly constructed `PiStatusSource` with no events fed yet, *When* `CurrentStatus()` is called, *Then* it returns `detection.StatusReady`.
**Files**: `session/pi_status_source.go`, `session/pi_status_source_test.go`

##### Task 5.2.1a: Define `PiStatusSource` struct and constructor (~3 min)
- Fields: underlying `*exec.Cmd`, current status (atomic), last-activity timestamp, idle timer.
- Files: `session/pi_status_source.go`

##### Task 5.2.1b: Implement the event-to-status mapping and idle timer (~5 min)
- `tool_execution_start` → `StatusExecuting`; `agent_end` → start idle timer; timer fires after `piIdleGracePeriod` → `StatusIdle`; any new event cancels the pending idle transition.
- Files: `session/pi_status_source.go`

##### Task 5.2.1c: Wire the subprocess launch (`exec.Command(..., "--mode", "json")`) with `StdoutPipe` (~4 min)
- Guarded by the `pi-support` flag and `isPi(i.Program)`, started alongside the tmux-launched interactive pi process (a second, `--mode json` instance is required per `research/architecture.md`'s framing that status and the interactive session are separate concerns — confirm during implementation whether pi supports attaching `--mode json` to an *existing* session or requires its own invocation; if the latter, note the resource/process-count implication in a code comment).
- Files: `session/pi_status_source.go`, `session/instance.go` (start/stop lifecycle hook)

##### Task 5.2.1d: Unit tests for the three AC bullets, using synthetic event feeds (~5 min)
- Files: `session/pi_status_source_test.go`

#### Story 5.2.2: Wire `PiStatusSource` into `InstanceStatusManager`
**As a** developer, **I want** pi's status to flow into the same `InstanceStatusInfo` the UI already reads, **so that** no frontend changes are needed beyond the health badge.
**Acceptance Criteria**:
- `InstanceStatusManager.GetStatus()` returns pi-derived status when no Claude controller is registered but a `PiStatusSource` is.
  - *Given* an instance with `program: "pi"`, a registered `PiStatusSource` reporting `StatusExecuting`, and no registered `ClaudeController`, *When* `GetStatus(instance)` is called, *Then* the returned `InstanceStatusInfo.ClaudeStatus == detection.StatusExecuting` and `IsControllerActive == true`.
**Files**: `session/instance_status.go`

##### Task 5.2.2a: Add `piSources` map and `RegisterPiStatusSource`/`UnregisterPiStatusSource` (~4 min)
- Mirror the existing `controllers` map's shape (`instance_status.go:27-48`).
- Files: `session/instance_status.go`

##### Task 5.2.2b: Extend `GetStatus()` to fall back to `piSources` (~5 min)
- After checking `controllers`, check `piSources` if no Claude controller was found; populate `ClaudeStatus`/`StatusContext`/`IsControllerActive` from `PiStatusSource.CurrentStatus()`; leave `QueuedCommands`/`SubagentCount`/`LastCommandStatus` at zero value (documented as intentional in a code comment, per the Pattern Decision).
- Files: `session/instance_status.go`

##### Task 5.2.2c: Unit test the fallback branch (~4 min)
- Files: `session/instance_status_test.go`

#### Story 5.2.3: Detect and surface status-subprocess death
**As a** user, **I want** to know when the status-only `pi --mode json` subprocess has died, **so that** the session list never silently freezes at a stale last-known status forever.
**Context**: Task 5.2.1c spawns a second, status-only pi process per session; nothing today detects that process exiting or crashing (OOM, pi bug, signal). Give this the same three-state rigor already given to the approval extension's health tracking in Epic 4.2, so the two silent-failure modes get consistent treatment:
  - **Known-good**: the subprocess is running and has emitted an event recently.
  - **Stale-unknown**: no recent event, but the subprocess hasn't been confirmed dead yet (mirrors `PiExtensionHealthUnknown` — do not read this as "still idle," since idle and dead look identical from the outside without a liveness check).
  - **Confirmed-dead**: `cmd.Wait()` has returned, confirming the process exited.
**Acceptance Criteria**:
- A `cmd.Wait()` goroutine detects process exit and transitions `PiStatusSource` out of any inferred status into a distinct dead/unavailable state.
  - *Given* a `PiStatusSource` with its subprocess killed out-of-band (e.g. `cmd.Process.Kill()` in a test), *When* the `cmd.Wait()` goroutine observes the exit, *Then* `CurrentStatus()` returns a new `detection.StatusUnavailable` (or equivalent) value distinct from `StatusIdle`/`StatusReady`, not a frozen copy of whatever status was last inferred.
- A bounded number of automatic restarts are attempted before giving up.
  - *Given* the subprocess has died, *When* `PiStatusSource` detects this, *Then* it attempts to relaunch the `pi --mode json` subprocess up to a small bounded retry count (e.g. 3 attempts with backoff — tune during implementation), and only reports the confirmed-dead/unavailable state once retries are exhausted.
  - *Given* a relaunch attempt succeeds, *When* the new subprocess emits its first event, *Then* `CurrentStatus()` resumes normal inference and the retry counter resets.
- The UI-visible status distinguishes "unavailable" from ordinary idle, so a user doesn't mistake a dead status subprocess for a quiet session.
  - *Given* `InstanceStatusManager.GetStatus()` for an instance whose `PiStatusSource` reports the confirmed-dead/unavailable state after exhausting retries, *When* the session list renders, *Then* the surfaced status is distinguishable from `StatusIdle` (either a new `detection.DetectedStatus` value the UI already knows how to render distinctly, or a `StatusContext` string flagged for the UI to render as a warning, whichever requires the smaller UI diff).
**Files**: `session/pi_status_source.go`, `session/pi_status_source_test.go`, `session/instance_status.go` (if a new `detection.DetectedStatus` value is added, also `session/detection/*`)

##### Task 5.2.3a: Add a `cmd.Wait()` goroutine that observes subprocess exit (~4 min)
- Launch a goroutine alongside the `StdoutPipe` reader (Task 5.2.1c) that blocks on `cmd.Wait()` and, on return, marks the source's internal state as "process exited" (distinct from any inferred idle/executing state).
- Files: `session/pi_status_source.go`

##### Task 5.2.3b: Implement bounded relaunch with backoff (~5 min)
- On detected exit, attempt up to N relaunches (small constant, e.g. 3) with a short backoff between attempts; reset the retry counter on a successful relaunch that emits at least one event; stop retrying and flip to the confirmed-dead/unavailable state once exhausted.
- Files: `session/pi_status_source.go`

##### Task 5.2.3c: Surface the unavailable state through `InstanceStatusManager.GetStatus()` (~4 min)
- Wire the new state into the pi-fallback branch added in Task 5.2.2b, choosing the smaller-diff option named in the AC (new `DetectedStatus` value vs. a flagged `StatusContext` string).
- Files: `session/instance_status.go`

##### Task 5.2.3d: Unit tests for detection, bounded retry, and exhausted-retry surfacing (~5 min)
- Cover: kill mid-session → detected within one poll interval; kill + successful relaunch → status resumes normal inference and retry counter resets; kill + N failed relaunches → confirmed-dead/unavailable state surfaced and retries stop.
- Files: `session/pi_status_source_test.go`

### Epic 5.3: Approval-block reconciliation
**Goal**: Prevent the session list from showing "idle" while pi is actually blocked on a human decision — the race `research/architecture.md` §3 point 3 names explicitly.

#### Story 5.3.1: Override inferred Idle with NeedsApproval when a pending approval exists
**As a** user, **I want** a pi session that's actually waiting on my approval decision to show as "needs approval," not "idle," **so that** I don't overlook a blocked session.
**Acceptance Criteria**:
- A session with an inferred-idle `PiStatusSource` but a live pending approval in the store reports `StatusNeedsApproval`, not `StatusIdle`.
  - *Given* `PiStatusSource.CurrentStatus()` would return `StatusIdle` on its own, and `ExternalApprovalMonitor.GetAllPendingApprovals()` (or the confirmed equivalent per the Unresolved Questions trace) reports one pending approval for this session's ID, *When* `InstanceStatusManager.GetStatus(instance)` is called, *Then* the returned status is `detection.StatusNeedsApproval`.
- With no pending approval, the idle inference is unaffected.
  - *Given* the same setup with zero pending approvals, *When* `GetStatus(instance)` is called, *Then* the status remains `detection.StatusIdle`.
**Files**: `session/instance_status.go` (or `session/external_approval.go`, depending on where the five-minute trace from the Unresolved Questions section lands the right call site)

##### Task 5.3.1a: Trace `PendingApprovals`'/pending-approval lookup's actual live wiring for Claude sessions (~5 min)
- Confirm whether `session/external_approval.go:178`'s `GetAllPendingApprovals` (or a different call site) is the correct, already-wired source of truth to reuse, per the Unresolved Questions item.
- Files: none (investigation only; informs Task 5.3.1b)

##### Task 5.3.1b: Implement the override in `GetStatus()`'s pi-fallback branch (~5 min)
- Files: `session/instance_status.go`

##### Task 5.3.1c: Unit test both AC bullets (~4 min)
- Files: `session/instance_status_test.go`

---

## Phase 6: Observability, E2E, and Documentation

### Epic 6.1: Structured logging and metrics
**Goal**: Satisfy the Observability Plan above with concrete, testable log lines and counters.

#### Story 6.1.1: Add the logging and metrics points named in the Observability Plan
**Acceptance Criteria**:
- Extension injection failure logs at error level with the session/instance identifier.
  - *Given* `installPi()`'s file-write step fails (e.g. permissions error, simulated in a test via a read-only temp dir), *When* the failure occurs, *Then* a `log.Error` line is emitted containing the target path and the underlying error.
- The `pi_status_source_events_total{type}` counter increments once per parsed event, including once for an unrecognized `type` value (not silently dropped).
  - *Given* a `PiStatusSource` fed one `agent_start` and one `totally_unknown_type` event, *When* both are processed, *Then* the counter shows `{type="agent_start"} == 1` and `{type="totally_unknown_type"} == 1` (or an equivalent `{type="unrecognized"}` bucket — decide and document the exact label scheme in the metric's doc comment).
**Files**: `cmd/ssq-hooks/main.go` (or wherever `installPi`'s error path lives), `session/pi_status_source.go`, wherever this repo's existing metrics registration convention lives (check for an existing `metrics.go`/OTel setup per `docs/how-to/enable-opentelemetry.md`)

##### Task 6.1.1a: Add the error-path log line to `installPi()` (~2 min)
- Files: `cmd/ssq-hooks/main.go`

##### Task 6.1.1b: Add the event-throughput counter to `PiStatusSource` (~4 min)
- Files: `session/pi_status_source.go`

##### Task 6.1.1c: Test both (~4 min)
- Files: `cmd/ssq-hooks/main_test.go`, `session/pi_status_source_test.go`

### Epic 6.2: E2E coverage, docs, and registry updates
**Goal**: Close out the repo's standing conventions (feature registry, e2e annotations, docs placement) so pi-support doesn't become an undocumented exception.

#### Story 6.2.1: E2E test for the pi preset + resume flow
**As a** developer, **I want** an e2e test exercising pi's preset selection and a simulated resume, **so that** the UI-level integration is guarded against regression.
**Acceptance Criteria**:
- A Playwright spec creates a session with the pi preset and asserts the program picker's selected value.
  - *Given* the e2e test server with `pi-support` enabled via test config, *When* the spec selects "pi" from the program dropdown and creates a session, *Then* the created session's `program` field is `"pi"`.
**Files**: `tests/e2e/pi-session.spec.ts` (new)

##### Task 6.2.1a: Write the spec, feature-annotated per `e2e-test-conventions` (~5 min)
- `// @feature session:create, pi-support` header per the skill's convention; `data-testid`/ARIA locators only; no `waitForTimeout`.
- Files: `tests/e2e/pi-session.spec.ts`

#### Story 6.2.2: Feature registry and docs
**Acceptance Criteria**:
- `make registry-generate` picks up the new RPC field/route and program-picker addition with no manual registry edits needed beyond running the command.
  - *Given* all Phase 2-5 code merged, *When* `make registry-generate` is run, *Then* `git diff` shows new/updated entries under `docs/registry/features/` reflecting the new health-ping route and any new `// +feature:`-marked component, with no unrelated churn.
- A short how-to doc exists for enabling pi-support, mirroring the style of `docs/how-to/enable-opentelemetry.md`.
  - *Given* the finished feature, *When* a new user reads `docs/how-to/enable-pi-support.md`, *Then* it covers: enabling the flag, running `ssq-hooks install pi`, and what the health badge states mean.
**Files**: `docs/registry/features/*` (generated), `docs/how-to/enable-pi-support.md` (new)

##### Task 6.2.2a: Run and commit `make registry-generate`'s output (~3 min)
- Files: `docs/registry/features/*`

##### Task 6.2.2b: Write the how-to doc (~5 min)
- Files: `docs/how-to/enable-pi-support.md`

##### Task 6.2.2c: Add the new doc to the top-level `CLAUDE.md` Reference Documents Index (~2 min)
- Files: `CLAUDE.md`
