# ADR-003: pi's Approval Extension Fails Closed (Blocks) on Network Error or Timeout

**Status**: Accepted
**Date**: 2026-09-02
**Project**: pi-support

## Context

`research/architecture.md` §3 point 2 flags this as an explicit open question: Claude's
existing curl-based hook fails **open** on a payload decode error
(`ApprovalHandler.HandlePermissionRequest`'s parse-error branch,
`server/services/approval_handler.go:243`, "so as not to block Claude on parse errors"). pi
has no framework-level permission system at all (`research/build-vs-buy.md` §3: "Pi ships no
built-in permission system... runs with the permissions of the user and process that launched
it") — so unlike Claude, there is no secondary backstop if the extension itself fails to
decide. requirements.md frames pi-support as explicitly safety-motivated ("approval-rules
parity is in scope... zero enforcement today is the gap," "a user relying on approval rules
for safety has no coverage when running pi").

## Decision

The generated extension's `tool_call` handler treats a network error, a non-2xx response, or
a timeout from its `fetch()` call to `/api/hooks/permission-request` as **deny** (block the
tool call), not allow. The `fetch()` call uses a bounded timeout matching
`ApprovalHandler.approvalTimeout()`'s default (4 minutes,
`server/services/approval_handler.go:112`) plus a small margin, mirroring
`remoteApprovalHookAttemptTimeoutSeconds`'s reasoning
(`server/services/hook_injector.go:~sec 155`: must cover a real human decision, not just a
connectivity probe) — not a short 8-second timeout like `openCodePluginTemplate`'s
(`cmd/ssq-hooks/main.go:1215`), because unlike OpenCode's local in-process classification,
this call can legitimately be waiting on a human's manual-review decision the whole time.

## Alternatives Considered

| Option | Rejected because |
|---|---|
| Fail open on network error (mirror Claude's curl-hook default) | Requirements.md's "Alternatives Considered" section already rejects the "leave pi as free-text with no enforcement" baseline as unacceptable; failing open on every stapler-squad outage or crash reduces the feature to "enforcement only when the server happens to be up," which the security-relevance framing throughout `research/pitfalls.md` (PITFALL-1, "silent enforcement... worse than no enforcement") argues against explicitly. |
| Short timeout (8s, mirroring OpenCode) with fail-closed on timeout | Would treat a normal in-flight human-review wait (routinely well over 8 seconds) as a network failure and auto-deny every escalated request — breaks the "queue for manual review" flow (`ApprovalQueuedForReview` in `research/architecture.md`'s EventStorming table) that already works for Claude today. |

## Consequences

- A stapler-squad outage or crash, while pi-support is enabled, blocks **all** pi tool calls
  until the server is back up — an explicit, accepted trade-off given the feature is opt-in
  (default off) and the user who enables it is choosing safety over availability for this
  specific, secondary agent.
- The extension's own health signal (Phase 4, mirroring PITFALL-1's mitigation) must be
  visible enough that a user can tell "pi is blocked because stapler-squad is down" apart from
  "pi is blocked because a rule denied this specific tool call" — both present as a blocked
  tool call from pi's perspective, but the UI's extension-health badge (see `research/ux.md`
  §4) is the disambiguating signal, not the tool-call block itself.
- A future request to make this configurable (e.g. a per-installation fail-open override) is
  explicitly out of scope for this project; if raised later, it should be scoped as its own
  follow-up with its own explicit risk sign-off, not bundled in here.
- **Known residual risk**: this ADR's fail-closed guarantee covers only the *deliberate*
  decision path — a network error, non-2xx response, or timeout from the extension's own
  `fetch()` call. It does not, by itself, cover an *unexpected* uncaught exception thrown from
  elsewhere in the generated handler body (a bug in `ssqApprovalExtensionTemplate`, not a
  network condition) — pi's own default behavior for an uncaught handler exception is
  unconfirmed as of this ADR and is exactly what plan.md's Task 1.1.2c probes. If that probe
  finds pi defaults to "allow" on an uncaught exception, this is a real gap: any bug in the
  generated template's non-network code could silently disable enforcement. The mitigation —
  wrapping the entire handler body in a try/catch that defaults to blocking on any error, not
  just a `fetch()` failure — is a template-implementation requirement (see plan.md Task
  4.1.1a), not a change to this ADR's decision; this ADR's fail-closed policy is what that
  catch block falls back to.
