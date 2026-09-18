# ADR-027: OpenCode's `Escalate` Decision Maps to Deny (Fail-Closed), Not Allow

**Status**: Accepted
**Date**: 2026-08-11
**Project**: backlog item "Implement patchOpenCodeHooks(): replace open-code proxy wrapper with native plugin hook"

## Context

`classifier.ClassificationResult.Decision` is a three-state enum: `AutoAllow`, `AutoDeny`,
`Escalate` (`pkg/classifier/classifier.go:36-43`). `Escalate` means "no rule matched, needs
human review" — it is the catch-all, not a rare case (`pkg/classifier/classifier.go:755` names
it the "Escalate catch-all"). Every existing hook adapter maps it to a live-dialog fallback in
the target tool:

| Target | `Escalate` handling | Source |
|---|---|---|
| Claude | writes nothing to stdout → Claude Code shows its native permission dialog | `cmd/ssq-hooks/main.go:153-155` |
| Gemini/Agy | exit 0, empty output → agy/Gemini shows its own dialog | `cmd/ssq-hooks/main.go:180-183` |
| Antigravity | `{"decision":"ask"}` on stdout → Antigravity shows its own dialog | `cmd/ssq-hooks/main.go:208-210` |
| OpenCode (current proxy, `handleProxy()`) | prints "Command requires manual review (escalated). Currently unsupported in standalone proxy mode." and `os.Exit(1)` — hard-aborts the whole invocation | `cmd/ssq-hooks/main.go:423-424` |

`patchOpenCodeHooks()` (this item) replaces the current bash-wrapper proxy with a plugin
subscribing to `@opencode-ai/plugin`'s `Hooks.tool.execute.before` hook. Per
`~/.config/opencode/node_modules/@opencode-ai/plugin/dist/index.d.ts:193-199`, that hook's
`output` is `{ args: any }` — there is no decision field at all. The *only* signal a plugin can
send back is throw-to-block or don't-throw-to-proceed: a **binary** channel. There is no
confirmed room for a third "ask, let the user decide" state.

OpenCode does have a hook with a genuine three-state contract — `permission.ask`, whose
`output.status` is `"ask" | "deny" | "allow"` (`index.d.ts:183-185`) — which would otherwise be
the structurally better fit for round-tripping `Escalate`. It cannot be used: it is broken two
independent, both-currently-unfixed ways upstream, confirmed via `gh issue view` against
`anomalyco/opencode`:

- [**#7006**](https://github.com/anomalyco/opencode/issues/7006) (OPEN) — the `Plugin.trigger("permission.ask", ...)` call site was removed by commit `38e0dc9` ("Move service state into InstanceState, flatten service facades") and never restored. Four independent community PRs (#19453, #30509, #34329, #39442) have attempted to restore it; none confirmed merged as of this triage.
- [**#19927**](https://github.com/anomalyco/opencode/issues/19927) (CLOSED by 60-day inactivity bot, not by a fix) — even when the hook does fire, it's wrapped in `if (!needsAsk) { ...Plugin.trigger("permission.ask"...) }`, i.e. it only fires for commands that *already* have an allow rule and is skipped for the first-encounter `needsAsk=true` case — exactly the case a security gate needs to intercept.

So the real decision is not "which hook" but "how does a binary throw/don't-throw channel
represent a three-state decision," and it needs to be made explicitly rather than defaulted
silently inside `writeOpenCodeHookDecision()`.

## Decision

**Map `Escalate` to deny** (throw inside `tool.execute.before`, matching the `AutoDeny` path),
with a reason string that says the command needs manual review and is being blocked because
OpenCode has no live-dialog fallback for this hook — not the same generic denial text as an
`AutoDeny` classifier hit. Concretely, `writeOpenCodeHookDecision()`'s `Escalate` case writes a
distinct reason (e.g. `"requires manual review (no rule matched); OpenCode's tool.execute.before hook has no ask/dialog fallback, so this is blocked rather than silently allowed — approve manually via the review queue or add a classifier rule"`) and exits non-zero, same as `AutoDeny`.

This is a fail-closed choice, made for three reasons:

1. **Consistency with current behavior.** The proxy `handleProxy()` replaces already hard-aborts
   (`os.Exit(1)`) on `Escalate` today (`main.go:423-424`) — mapping to deny is not a new,
   stricter posture for OpenCode users, it is the same posture the existing (if differently
   shaped) mechanism already has. Mapping to allow would be a **regression** relative to what
   OpenCode users experience today, not a neutral default.
2. **A security gate with a silent hole is worse than no gate.** `pitfalls.md` §1 (triage
   research) makes this same point about subagent/batch-tool coverage; it applies identically
   here. Auto-allowing anything the classifier couldn't confidently rule on defeats the purpose
   of shipping `patchOpenCodeHooks()` as a security control at all.
3. **`Escalate` is not rare.** It is the deliberate catch-all for "no explicit rule fired" — a
   fail-open mapping would silently weaken the policy for the plurality of commands that don't
   match an explicit allow/deny rule, not just an edge case.

The tradeoff accepted: this will produce **more visible blocks** for OpenCode users than for
Claude/Gemini/Antigravity users hitting the same command, because there is no equivalent "let
the user decide right now" UI to fall back to. A blocked, no-context command is more disruptive
than a dialog prompt. This is a real UX regression relative to the other three targets, accepted
deliberately in exchange for not silently weakening the policy.

## Consequences

- `writeOpenCodeHookDecision()`'s `Escalate` branch throws/exits non-zero with a distinct reason
  string (see above), not the same code path as `AutoDeny`'s classifier-driven reason, so a user
  reading the error can tell "no rule decided this" apart from "a rule explicitly denied this."
- Users who find OpenCode blocking commands more often than Claude/Gemini/Antigravity for the
  same operations should be pointed at adding classifier rules (`AutoAllow`) for their common
  safe operations, not at disabling the hook — the fix for "too many escalations" is narrowing
  the catch-all via rules, the same lever available for every other target.
- `TestWriteOpenCodeHookDecision_Escalate` (see validation plan) asserts non-zero exit + the
  distinct reason text, not the generic `AutoDeny` reason.
- **Revisit trigger:** if `permission.ask` is fixed upstream (either #7006's trigger-call
  restoration or #19927's `needsAsk` guard removal lands and is confirmed released), re-open
  this decision — the plugin could then subscribe to `permission.ask` in addition to (or instead
  of) `tool.execute.before` to recover a genuine tri-state UX matching the other three targets.
  Do not switch to `permission.ask` before then; it does not fire reliably today (see Context).

## Alternatives Considered

- **Map `Escalate` → allow-through** (option 1 in `research/architecture.md` §5): rejected — see
  Decision reasons 1–3. Cheapest to implement, but silently weakens the policy for the
  deliberately-broad catch-all case, and is a regression vs. today's hard-abort proxy behavior.
- **Subscribe to `permission.ask` instead of / in addition to `tool.execute.before`** (option 3):
  rejected for now — the hook does not fire reliably (see Context, #7006/#19927), so building the
  design around it would ship a gate that silently doesn't work on the exact case (`Escalate`)
  it exists to handle. Revisit once upstream fixes land (see Revisit trigger above).
