# ADR-001: pi's Approval Extension Calls `/api/hooks/permission-request` Directly via `fetch()`, Not `ssq-hooks check --pi`

**Status**: Accepted
**Date**: 2026-09-02
**Project**: pi-support

## Context

Two research docs propose two different transports for the pi extension's `tool_call`
handler to reach stapler-squad's classifier:

1. `research/stack.md` / `research/architecture.md` §2a: the extension's `tool_call`
   handler builds a `classifier.PermissionRequestPayload`-shaped JSON body and POSTs it
   directly (TS `fetch()`) to the same URL Claude's curl-based hook already hits —
   `hookEndpoints()`'s `HookPermissionApproval` entry
   (`server/services/hook_injector.go:103`, `base + "/api/hooks/permission-request"`),
   served by `ApprovalHandler.HandlePermissionRequest`
   (`server/services/approval_handler.go:231`), which is backed by the live, hot-reloadable
   `RulesService` (`server/services/rules_service.go`).
2. `research/build-vs-buy.md` §3/§4: shell out to the already-shipped `ssq-hooks check --pi`
   binary via `execFileSync`, mirroring `installOpenCode()`'s
   `openCodePluginTemplate` (`cmd/ssq-hooks/main.go:1199-1221`), and translate its exit code
   into an allow/block decision, the same way Gemini/agy/OpenCode's hooks do.

Reading `cmd/ssq-hooks/main.go`'s `handleCheck()` (`main.go:63`) and `loadClassifier()`
(`main.go:572`) shows these are **not equivalent paths**: `ssq-hooks check` constructs its
own in-process `classifier.RuleBasedClassifier`, loaded fresh from on-disk rule storage
(`loadClassifier(storage)`), and classifies locally — it never calls the running server's
`HandlePermissionRequest` HTTP endpoint or its live `RulesService` instance. Gemini/agy/
OpenCode's approval parity today is therefore classified against a point-in-time snapshot of
rules re-read from disk on every check, not the server's live, hot-reloadable classifier
(the one `ReloadClaudeSettingsRules` and `UpsertApprovalRule` mutate in place via
`rebuildClassifier()`/`rebuildClaudeSettingsRules()`).

The user's own requirements decision is explicit: "real integration with RulesService/
classifier, not a disconnected pi-only system... POSTing to the existing
`/api/hooks/permission-request` endpoint... requires ZERO changes to RulesService." That
decision names the live server endpoint specifically, which only Option 1 satisfies.

## Decision

The generated `.pi/extensions/ssq-approval.ts`'s `tool_call` handler calls `fetch()`
directly against the `/api/hooks/permission-request` URL baked into the template at
injection time (mirroring how `hookEndpoints(getHookBaseURLFn())` resolves fresh per-injection
today), translating pi's tool-call event fields (`toolName`/`toolCallId`/`args`, exact shape
pending Phase 1's live-verification spike) into a `classifier.PermissionRequestPayload`-shaped
JSON body (`pkg/classifier/classifier.go:57-65`). No `ssq-hooks` binary is shelled out to for
pi's approval path.

## Alternatives Considered

| Option | Rejected because |
|---|---|
| `ssq-hooks check --pi` exit-code contract (mirrors OpenCode) | Classifies against a disk-reloaded snapshot, not the live server's `RulesService` — fails the requirements' explicit "real integration with RulesService" decision; would also require distributing/locating the `ssq-hooks` binary path inside a TS extension via `execFileSync`, an extra moving part `fetch()` avoids entirely. |
| A new pi-specific HTTP endpoint that itself delegates into `RulesService` | Unnecessary indirection — `HandlePermissionRequest`'s request/response contract is already agent-neutral (plain JSON POST), per `research/architecture.md` §2a; a second endpoint would duplicate secret-scan/domain-age/audit logic already implemented once. |

## Consequences

- Zero changes to `RulesService`, `ApprovalHandler`, or `ApprovalStore` are needed for
  classification/audit to work for pi — confirmed by this ADR's research, not just assumed.
- The extension needs its own JSON-body construction and response-handling logic in
  TypeScript (no code sharing with `cmd/ssq-hooks`'s Go structs) — a small, isolated,
  hand-written piece of TS (~30-50 lines per `research/build-vs-buy.md`), not a reuse of the
  Go-side payload parsers.
- The extension's HTTP call must tolerate the same long-poll-until-resolved latency
  `ApprovalHandler` already provides for manual review (up to `approvalTimeout()`'s 4-minute
  default, `server/services/approval_handler.go:112`) — see ADR-003 for the timeout/fail-open
  vs fail-closed policy this implies.
- One remaining gap: `PermissionRequestPayload` carries no field distinguishing which agent
  (Claude vs pi) sent a given payload. Adding an optional `Source`/`Program` field (additive,
  `omitempty`) is scoped as a task in plan.md so audit records remain distinguishable, but is
  not required for classification correctness.
