# Research: Architecture — pi Support

**Status**: Completed | **Phase**: 2 — Research | **Agent**: 3 (Architecture)

## 0. Building on Prior Research

Two prior architecture docs in this repo cover the closest analogous problems and are
extended here rather than re-derived:

- `project_plans/agy-support/research/architecture.md:30-38` — `cmd/ssq-hooks/main.go`'s
  `handleInstall()` dispatches on a `target` string (`claude`/`gemini`/`agy`/`open-code`/
  `service`), and (`:56-91`) Gemini/agy's hook is a **settings.json patch**
  (`patchBeforeToolHook()` merges a flat `hooks.BeforeTool` string into an existing JSON
  file). That pattern **does not fit pi** — see §1 below for why.
- `project_plans/antigravity-opencode-parity/research/architecture.md` — establishes that
  status detection for non-Claude binaries today is 100% PTY-scrollback regex matching
  (`session/detection/binaries/*.go`, one `dtypes.StatusPatterns` struct per binary,
  registered in `session/detection/registry.go:9-17`'s `DefaultRegistry()`). No existing
  binary detector consumes a structured event stream — every one of them, including the
  two closest siblings (agy/opencode), is scraping terminal text. This is the same gap
  pi's `--mode json` proposal would be the *first* to close.

## 1. Does `ssq-hooks install pi` fit `handleInstall()`'s existing pattern? No — and here's the boundary that changes

`handleInstall()`'s existing cases (`installClaude`, `installGemini`/`installGeminiAuto`,
`installOpenCode`) all share one shape: **read-JSON → merge-a-key → write-JSON**, because
every one of Claude/Gemini/agy/open-code's hook systems is "the host CLI reads a static
config file at startup and shells out to `ssq-hooks check` on a matching event." pi's
mechanism (`project_plans/pi-support/requirements.md:31`, restated in the Constraints
section) is categorically different:

| | Claude / Gemini / agy / open-code | pi |
|---|---|---|
| Hook artifact | JSON key in `settings.json`/`settings.local.json` | A **TypeScript source file** in `.pi/extensions/*.ts` |
| Registration | Declarative (`hooks.BeforeTool = "<shell command>"`) | Imperative — the file's own code calls `pi.on('tool_call', handler)` |
| Execution model | Host CLI shells out to an **external process** (`ssq-hooks check`) per event | Handler runs **inside the pi process** (same JS runtime), and calls back out itself |
| Approve/deny surface | Hook's exit code / stdout JSON | `ctx.ui.confirm()` inside the extension, which must itself decide by calling out (HTTP) to stapler-squad |
| Trust gate | None | pi's own "project must be trusted" gate — an untrusted `.pi/` silently skips loading the extension entirely |

So `installPi()` is a new case in `handleInstall()`'s switch (same dispatch site,
`cmd/ssq-hooks/main.go`'s target switch, same pattern as `agy`/`open-code` being added
there) — **but its body is not `patchBeforeToolHook()`-shaped**. It needs a distinct
helper, e.g. `writePiExtension(projectDir, extensionSrc)`, that:

1. Renders a Go `//go:embed`-ed or templated `.ts` file (the extension source is
   stapler-squad-authored and shipped, per requirements.md's Out-of-Scope: "ships and
   manages only its own approval-hook extension" — not a general `pi install` wrapper).
2. Writes it to `<projectDir>/.pi/extensions/ssq-approval.ts` (mirrors
   `hook_injector.go:268-269`'s `claudeDir := filepath.Join(rootDir, ".claude")` /
   `settingsPath` pattern, but for a `.ts` file, not JSON — so `writeSettingsAtomic`
   (used throughout `hook_injector.go`/`approval_handler.go`) is reusable verbatim for the
   atomic-write mechanics; the read-merge-write JSON logic (`readExistingHooksSettings`,
   `hookAlreadyPresent`, `prependHookEntry`) is not, because there is no JSON to merge —
   idempotency here is "file content matches the shipped template" (hash compare), not
   "this key already points at our URL."
3. Bakes the callback URL into the generated source the same way
   `hookEndpoints(getHookBaseURLFn())` (`hook_injector.go:100-110`) bakes URLs into curl
   commands — i.e. resolved fresh per-injection from the live server address, never
   cached at build time, so `PORT=0` / dynamic-port test servers still work.
4. Surfaces success/failure as a structured result (not just `error`), because
   requirements.md's Observability section explicitly calls out "an extension
   install/health signal per session (so a silent injection failure doesn't silently
   disable enforcement)" — this is new; none of the JSON-patch installers need it, since
   a JSON merge failure is loud (file write error), but a *pi trust-gate skip* is silent
   at the pi-process level and only observable from stapler-squad's side by absence of a
   later "extension loaded" signal (see §4, `ExtensionInjectionFailed`/health-check event
   below).

`printUsage()` and the target-list usage string get `pi` added, same mechanical change as
the agy doc's `:52-53`.

## 2. Integration points with existing systems

### 2a. Approval decision path — `RulesService` / `approval_service.go` / `ApprovalHandler`

The existing Claude flow (`server/services/approval_handler.go:231` `HandlePermissionRequest`)
is: hook POSTs `classifier.PermissionRequestPayload` → secret scan → domain-age check →
`h.classifier.BuildContext`/`Classify` (backed by `RulesService.rebuildClassifier()`,
`server/services/rules_service.go:517`) → auto-allow/deny or queue for manual review via
`ApprovalStore` → `ApprovalService.ResolveApproval` (`server/services/approval_service.go:61`)
on human decision → `notificationStore`/`eventBus` broadcast.

**pi's extension must terminate at the exact same HTTP endpoint and payload shape**, not a
parallel one — this is requirements.md's explicit decision ("not a parallel, disconnected
rule system," Alternatives Considered). Concretely:

- The generated `.pi/extensions/ssq-approval.ts`'s `tool_call` handler constructs a
  `classifier.PermissionRequestPayload`-shaped JSON body (`ToolName`, `ToolInput`, `Cwd`)
  from pi's `ExtensionAPI` event args, and POSTs it to the **same**
  `HookPermissionApproval` endpoint (`hookEndpoints()`'s
  `base + "/api/hooks/permission-request"`, `hook_injector.go:103`) that Claude's curl-based
  hook already hits. This reuses `HandlePermissionRequest` unmodified for classification,
  audit-store recording (`h.analyticsStore.RecordFromResult`, seen at
  `approval_handler.go:272-278`/`323-324`), and manual-review queueing verbatim — the "same
  classifier/rule source" requirement (`allRuleSpecs`, `rules_service.go:475`) is satisfied
  for free because it's literally the same code path, not a reimplementation.
- The **response contract differs** from Claude's hook only in delivery mechanism, not
  shape: Claude's curl-based hook is fire-and-forget from the CLI's perspective (Claude
  Code's own hook runner blocks on the curl call and reads its JSON stdout, per
  `hookDecisionResponse`/`writeDecision`, `approval_handler.go:754-769`). pi's extension is
  **in-process** and needs the decision synchronously to return from `ctx.ui.confirm()` (or
  bypass it entirely on auto-allow/deny) — so the extension's HTTP call must be a blocking
  fetch with the same long-poll-until-resolved semantics `ApprovalHandler` already provides
  for manual review (`StartExpirationCleanup`, `approvalTimeout()` at `approval_handler.go:112`,
  4-minute default). No new blocking primitive needed server-side; the extension is just a
  new *client* of the existing one.
- **Audit parity** (requirements.md Observability: "an approval-block audit log entry for
  every pi extension allow/block decision, consistent with existing Claude-hook approval
  auditing") is automatic if the extension truly reuses `HandlePermissionRequest` — every
  `RecordFromResult` call there already fires for any caller hitting that endpoint,
  regardless of which CLI's hook posted the payload. The only pi-specific audit gap is the
  **extension-health** signal (§1 point 4), which has no Claude analog because Claude's
  hook is a stateless curl invocation with no load/trust-gate step to fail silently.

### 2b. `hook_injector.go`'s webhook endpoint pattern — reused, not extended

No new HTTP endpoint is needed for the approval path itself (§2a). One *may* be needed for
the health-check signal: either (a) the extension POSTs a one-time "extension loaded"
ping to a new `/api/hooks/pi-extension-loaded` endpoint on first `tool_call` (or on an
explicit pi lifecycle event if the SDK exposes one), analogous to how
`HookStopNotification`/`HookPromptSubmit` are additional endpoints in the same
`hookEndpoints()` map (`hook_injector.go:100-109`) beyond the core permission hook; or (b)
stapler-squad infers health passively from whether *any* permission-request payload has
ever arrived tagged as pi-sourced for a session whose program is pi, within some grace
period after session start, and marks it degraded if not. (a) is simpler and consistent
with the existing multi-endpoint-per-hook-type design; (b) needs no extension changes but
can't distinguish "trust-gate skipped the file" from "no tool calls happened yet
legitimately." Recommend (a) — add a 7th `HookName` (e.g. `HookExtensionHealth`) to the
existing map rather than inventing a separate mechanism.

### 2c. Session status / idle detection — genuinely new data path, not a `BinaryDetector` extension

`session/detection/registry.go:9-17`'s `DefaultRegistry()` registers one
`dtypes.BinaryDetector` per binary (`ClaudeDetector`, `GeminiDetector`, `AiderDetector`,
`OpencodeDetector`, `AgyDetector`), each purely a `dtypes.StatusPatterns` regex bundle
matched against PTY scrollback text (`session/detection/detector.go:259`
`detectFromText`/`pattern_set.go:114` `MatchLines`) — confirmed by reading
`binaries/aider.go` in full: it's a `Patterns()` method returning regex structs, nothing
else. **This is the wrong integration point for pi's status.** requirements.md's Scope is
explicit that pi status should come from `pi --mode json`/RPC **events**, not scrollback
regex — a structured, typed event stream stapler-squad would need a *new* consumer for
(something that reads pi's JSON/RPC output stream and maps `agent_end`/tool-call
events → `DetectedStatus`), which sits alongside `StatusDetector`/`PatternSet` as a second,
parallel status-derivation mechanism feeding the same `InstanceStatusInfo`
(`session/instance_status.go:12`) output shape, not as a `BinaryDetector` implementation
plugged into the existing regex registry. Concretely this likely means: a `PiRPCStatusSource`
(or similar) that `Instance` wires in only when `program` is pi and pi is launched with
`--mode json`, feeding `InstanceStatusManager` (`session/instance_status.go:27`) the same
`detection.DetectedStatus` enum the regex path produces elsewhere — so downstream
consumers (`GetStatusIcon`, `IsWaitingForUser`, `NeedsAttention`, `instance_status.go:97-214`)
don't need to know which of the two derivation mechanisms produced the value. The "idle
inferred from absence of activity after `agent_end`" heuristic
(requirements.md Rabbit Holes) needs its own timer logic, likely modeled on whatever
existing idle-timeout mechanism `session/detection/idle.go` implements for the regex path —
worth reading in planning to decide whether that timer can be shared or needs a pi-specific
copy.

### 2d. `session/claude_command_builder.go` — resume-flag injection pattern to mirror, not extend

`ClaudeCommandBuilder.isClaudeCommand()` (`claude_command_builder.go:71-86`) hardcodes
`commandName == "claude"`; `Build()` (`:35-63`) appends `--resume <uuid>` only for that
one program. This is **not a generic "any program can resume" abstraction today** — it's
Claude-specific by name and by flag syntax (`--resume <uuid>`, UUID-validated via
`isValidUUID`). Supporting pi's resume flag (`--session <id>` / `-c`, per requirements.md's
Scope) requires either:
- A second builder type, `PiCommandBuilder`, with pi's own flag syntax and pi's own
  session-ID validation rules (pi's session ID format is not necessarily a UUID — needs
  verification against the installed pi version, per requirements.md's Feasibility Risks),
  selected by whatever call site currently constructs a `ClaudeCommandBuilder` based on
  the session's configured program; or
- Generalizing `ClaudeCommandBuilder` into a per-program strategy (interface with
  `isMatch(program string) bool` + `resumeFlag(sessionID string) string`), with Claude and
  pi as two implementations, and the call site doing program-based dispatch instead of
  always constructing a `ClaudeCommandBuilder`.

Given the interface-pollution/primitive-obsession conventions this repo already enforces
elsewhere (see `.claude/skills/interface-pollution-checklist`,
`primitive-obsession-checklist`), the second option is the better fit *if* a third resumable
program is ever plausible; for a two-program world the first option (a parallel, smaller
`PiCommandBuilder`) is the appetite-appropriate minimal change and matches how this repo
already handles the Claude/Gemini/agy split elsewhere (separate types per binary, not one
mega-type with mode flags) — leave the final call to planning, since it's genuinely a
judgment call, not something research should prescribe.

## 3. Data flow and consistency requirements: pi extension (TS, in-process) ↔ Go-side classifier

The core architectural tension requirements.md's Rabbit Holes flags ("RPC mode's approval
surface is indirect... may require careful protocol sequencing to avoid deadlock or missed
events") is this: stapler-squad wants **two independent channels** into the same pi
process — (1) the extension's `tool_call` → HTTP → classifier round-trip for approval, and
(2) an RPC/`--mode json` event stream for status. These are not the same channel and pi's
own docs (per requirements.md) don't guarantee they compose cleanly: "RPC mode itself does
not expose a tool-call approval gate to external processes — only an in-process extension
can intercept," and combining both "may require... `extension_ui_request`/
`extension_ui_response` round-trip."

Consistency requirements this implies for planning:

1. **Single source of truth for the decision.** The extension must never make its own
   allow/deny decision locally (e.g. caching a previous verdict) — every `tool_call` must
   round-trip to `HandlePermissionRequest` so `RulesService`'s rule set (which can change
   between calls via `ReloadClaudeSettingsRules`, `rules_service.go:196`) is always
   consulted fresh, exactly as Claude's hook does today (no client-side caching exists
   there either).
2. **Fail-closed vs. fail-open on network failure.** Claude's curl hook has a documented
   fail-open path (`HandlePermissionRequest`'s parse-error branch, `approval_handler.go:243`,
   auto-allows on decode failure "so as not to block Claude on parse errors"). Requirements.md
   frames pi support as safety-relevant ("approval-rules parity is in scope... zero
   enforcement today is the gap"), so planning must decide explicitly whether the pi
   extension mirrors that fail-open default (consistent with Claude, but arguably
   defeats the point of adding enforcement) or fails closed (safer, but a stapler-squad
   outage would then block all pi tool calls) — call this out as an explicit open question
   for planning, since requirements.md doesn't resolve it either.
3. **Ordering between the status stream and the approval round-trip.** If both channels
   are live simultaneously, a `tool_call` event and its corresponding RPC status event
   (e.g. an `agent_end`/waiting-state transition) could race — the status consumer might
   observe "idle" while an approval is actually mid-flight blocking on the extension's
   HTTP call, because the RPC stream has no visibility into the in-process extension's
   blocking state. This is a real UX correctness risk (session list shows idle while pi is
   actually blocked awaiting human approval) that plain polling-based Claude status
   detection doesn't have (Claude's approval-wait state IS visible in scrollback text, so
   the regex detector naturally reflects it). Mitigation: the extension's `tool_call`
   handler, on entering the blocking HTTP wait, should itself be crossed-checked as a
   signal into whatever `InstanceStatusInfo` pi's status source produces — i.e. treat
   "extension currently awaiting a decision" as a status input independent of the RPC
   event stream, not merely inferred from RPC silence.
4. **Extension version/contract drift.** requirements.md's Feasibility Risks already
   flags pi's `ExtensionAPI`/`ctx.ui.confirm()` as third-party and unversioned against
   stapler-squad. Architecturally, this means the generated `.ts` file should be treated
   like a generated-artifact contract test target: planning should scope a smoke test that
   installs the extension against the actually-installed local `pi` binary and confirms it
   loads and fires, not just a unit test of the Go-side template renderer — a template that
   renders syntactically valid TS against a stale API shape would pass every Go-side test
   and still silently fail to enforce anything in production.

## 4. Event-Command-Policy Table (EventStorming)

Bounded contexts surfaced by this table: **pi Process** (TS extension + RPC stream,
outside stapler-squad's process boundary), **Approval/Classification** (existing
`RulesService`/`ApprovalHandler`/`ApprovalStore`, Go, already exists), **Session
Lifecycle** (existing `Instance`/`InstanceStatusManager`/command builders, Go, already
exists), and **Extension Provisioning** (new — `ssq-hooks install pi` / injection health).

| Event | Triggered By (Command) | Context | Policy (reaction) |
|---|---|---|---|
| `PiSessionCreated` | `CreateSession` (program=pi, feature flag on) | Session Lifecycle | Build launch command via `PiCommandBuilder`; enqueue `InjectPiExtension` |
| `ExtensionInjected` | `InjectPiExtension` (writes `.pi/extensions/ssq-approval.ts`) | Extension Provisioning | Start pi process; arm health-check timer |
| `ExtensionInjectionFailed` | `InjectPiExtension` (write error, e.g. permissions) | Extension Provisioning | Log + surface session-creation error to UI; do **not** start pi in an unenforced state silently |
| `ExtensionTrustGateBlocked` *(inferred, not directly observable — see §2b)* | pi process startup (external, no direct signal) | pi Process → Extension Provisioning | Health-check timer expiry with no `ExtensionLoadConfirmed` → mark session "approval enforcement degraded" in UI |
| `ExtensionLoadConfirmed` | Extension's first-run health ping → `POST /api/hooks/pi-extension-loaded` | pi Process → Extension Provisioning | Clear health-check timer; mark session enforcement-healthy |
| `ToolCallRequested` | pi's `tool_call` handler fires inside pi process | pi Process | Extension POSTs `PermissionRequestPayload` to `/api/hooks/permission-request` (blocking) |
| `PermissionClassified` | `HandlePermissionRequest` → `classifier.Classify` | Approval/Classification | AutoAllow/AutoDeny → respond immediately; Escalate → `CreateApproval` |
| `ApprovalQueuedForReview` | `CreateApproval` (ApprovalStore) | Approval/Classification | Broadcast notification (existing `broadcastApprovalNotification`); pi extension's HTTP call blocks pending resolution |
| `ApprovalDecided` | `ResolveApproval` (human via UI) | Approval/Classification | `ApprovalStore.Resolve` → unblocks extension's pending HTTP call → extension resolves `ctx.ui.confirm()` accordingly |
| `ApprovalTimedOut` | `StartExpirationCleanup` sweep, no decision within `approvalTimeout()` | Approval/Classification | Same fail-open/closed policy decision as §3 point 2 — extension's blocking call must have a matching client-side timeout, or it hangs pi indefinitely |
| `PiStatusEventReceived` | pi's `--mode json`/RPC stream emits `agent_end`/tool-call/etc. | pi Process | New RPC status consumer maps event → `DetectedStatus`; feeds `InstanceStatusManager` (§2c) |
| `PiIdleInferred` | Timer: no `PiStatusEventReceived` since last `agent_end` beyond threshold | Session Lifecycle | Set status to Idle — heuristic, may race with an in-flight `ToolCallRequested` (see §3 point 3) |
| `PiSessionResumed` | Session restart with existing pi session ID | Session Lifecycle | `PiCommandBuilder` injects `--session <id>`/`-c`; re-arm extension injection + health check (extension is per-`.pi/extensions/` directory, but confirm it survives resume rather than needing re-injection) |
| `ApprovalAuditRecorded` | `analyticsStore.RecordFromResult` (existing, unmodified) fires for any payload hitting `HandlePermissionRequest`, pi included | Approval/Classification | No new code — automatic once §2a's reuse is correct; the thing to verify in implementation is that pi-sourced payloads are tagged distinguishably enough in analytics to answer "was this a Claude or pi decision" if ever needed (currently `PermissionRequestPayload` carries no program field — check whether one needs adding) |

## Summary

- `ssq-hooks install pi` becomes a new `handleInstall()` case, but its body is a **new**
  `writePiExtension()` helper (template a `.ts` file into `.pi/extensions/`), not
  `patchBeforeToolHook()` reused — pi's hook is code that self-registers at runtime, not a
  JSON config Claude/Gemini/agy/open-code all share.
- Approval parity is achieved by having the generated extension call the **same**
  `/api/hooks/permission-request` endpoint `HandlePermissionRequest` already serves —
  `RulesService`/`ApprovalStore`/audit recording need zero changes; the real new work is
  (a) the extension's blocking-HTTP-call semantics matching `approvalTimeout()`, (b) an
  explicit fail-open-vs-closed decision on network failure, and (c) a new health-check
  signal/endpoint since pi's trust-gate can silently no-op the whole extension.
- Status detection is architecturally new, not a `BinaryDetector` regex plugin like every
  existing non-Claude binary — it needs a parallel RPC/JSON-event consumer feeding the same
  `InstanceStatusInfo` output, and must be reconciled against the extension's in-process
  blocking-approval state to avoid the session list showing "idle" while pi is actually
  blocked on a human decision.
