# Research: Build vs. Buy — pi-support

Agent 6, SDD research phase. Question: build stapler-squad's pi-integration pieces
from scratch, or adopt/adapt existing solutions?

## 1. Existing OSS library/framework for the approval-gate extension

`@earendil-works/pi-coding-agent`'s own docs (`pi/packages/coding-agent/docs/extensions.md`)
confirm the `tool_call` event + `ctx.ui.confirm()` pattern described in requirements.md is the
documented extension API, not a guess. Pi ships **no built-in permission system** — extensions
are the only gate mechanism, and the docs explicitly say Pi "runs with the permissions of the
user and process that launched it."

Community approval-gate extensions exist and confirm the pattern is common and wanted:
- [`rytswd/pi-agent-extensions`](https://github.com/rytswd/pi-agent-extensions) — intercepts write/edit calls, rule groups, `PI_NO_GATE=1` kill switch.
- [`MasuRii/pi-permission-system`](https://github.com/MasuRii/pi-permission-system) — centralized tool/bash/MCP/skill permission gate, OpenCode-policy-shaped.
- [`joeygibson/pi-extensions`](https://github.com/joeygibson/pi-extensions) (`security-guard`) — substring-pattern rules with prompt/block/allow actions.
- Community discussion ([earendil-works/pi#3373](https://github.com/earendil-works/pi/discussions/3373)) treats approval-gating as the most-wanted extension category.

None of these integrate with an external classifier/webhook — they're all self-contained local
rule engines with their own config format. Adopting one would mean either (a) using it standalone
and building a *second*, disconnected rule system — which requirements.md's Alternatives Considered
section already rejects — or (b) forking one and rewiring its internals to call out to
stapler-squad's `RulesService`/webhook, which is no less work than writing ~30-50 lines from
scratch against the documented `tool_call` API, plus adds an upstream dependency of unknown
maintenance quality (single-maintainer hobby repos, no version pinning guarantees, license
unverified).

**Verdict: Not recommended.** Reference `MasuRii/pi-permission-system`'s command/decision shape
for extension design ideas, but write the extension in-house — parity with `RulesService` requires
custom glue regardless of starting point.

### JSONL parsing for the status/RPC event stream

The codebase already hand-rolls JSONL parsing for two other agents' transcript formats:
`session/claude_adapter.go` (`ReadCanonicalTurnsFromFile`, `buildCanonicalTurnFromRawClaudeTurn`)
and `session/agy_adapter.go` (reads `history.jsonl`/`transcript.jsonl`). Both use
`bufio.Scanner` + `encoding/json.Unmarshal` per line, already in `go.mod` (stdlib only — no
JSONL-specific dependency exists in `go.mod`/`go.sum`). Pi's `--mode json`/RPC event stream is a
comparably simple newline-delimited JSON schema.

**Verdict: Not recommended to add a library.** A third-party Go JSONL library would be inconsistent
with the two existing hand-rolled parsers and adds a dependency for something `bufio.Scanner` +
`encoding/json` already does adequately twice over in this repo. Follow the existing pattern
(`claude_adapter.go`/`agy_adapter.go`) for a new `pi_adapter.go`.

## 2. SaaS/managed API

Not applicable, as anticipated in the research prompt — pi is a local CLI orchestrated in a tmux
session, and nothing in the extension/RPC docs suggests a hosted equivalent. No further action.

## 3. LLM-generated implementation vs. battle-tested, given the security-critical nature

The TS extension is small (tool_call handler + `ctx.ui.confirm` + HTTP/exec call to stapler-squad)
but it is a security gate: a subtly wrong implementation (wrong event name, swallowed exception
treated as "allow," a `block: true` object shape typo the pi runtime silently ignores) means
dangerous commands run *unblocked* with no visible failure — worse than not having pi support at
all, because a user believes they're covered by approval rules and aren't.

Two concrete failure modes specific to pi (both flagged in requirements.md's Feasibility Risks/
Rabbit Holes and confirmed by the docs pulled above):
- **Fail-open by default.** Pi ships no permission system; if the extension throws, fails to load,
  or the project-trust gate silently rejects it, tool calls proceed with zero interception —
  there's no framework-level fail-closed backstop the way there might be in a sandboxed model.
- **Trust-gate silent disable.** Project-local extensions "load only after the project is
  trusted" — an untrusted directory silently no-ops the approval extension unless the
  install/health-metric observability requirement (already scoped in requirements.md) surfaces it.

Mitigations that make hand-writing this reasonable rather than risky:
- The existing `cmd/ssq-hooks` binary already solves the *hard* part (talking to
  `RulesService`/`approval_service.go`/the webhook) for four other agents (Claude, Gemini, agy,
  OpenCode) via a stable `ssq-hooks check --<agent>` exit-code contract
  (`server/services/hook_injector.go`'s curl-based PermissionRequest hook,
  `cmd/ssq-hooks/main.go`'s `openCodePluginTemplate`). The pi extension's job shrinks to: shell
  out to `ssq-hooks check --pi`, translate its exit code into `{block, reason}`, same as
  `openCodePluginTemplate`'s `execFileSync`/catch pattern for OpenCode.
- That pattern is proven correct against 4 agents already and unit-testable
  (`hook_injector_test.go`, `approval_service_test.go` exist).
- The health-signal requirement (in scope) directly targets the fail-open risk: alert on
  extension-not-loaded rather than trusting silent success.
- Bounded timeout (the OpenCode plugin's `execFileSync(..., { timeout: 8000 })`, fail-closed on
  timeout) is a pattern to replicate verbatim for the pi extension.

**Verdict: Viable to hand-write, conditioned on copying the existing exit-code/timeout contract
verbatim rather than re-deriving it, and treating the health-signal requirement as non-negotiable
(not a nice-to-have) given pi's fail-open default.** This is the recommended path, not because the
code is trivial, but because the risky part (the classifier/webhook integration) is already
battle-tested infrastructure being reused, not reinvented in TypeScript.

## 4. Fork/adapt `cmd/ssq-hooks` vs. a separate injection mechanism

Read `cmd/ssq-hooks/main.go`'s `installAgy` (`main.go:996-1054`), `installOpenCode`
(`main.go:1269-1325`), `patchAntigravityHooks`/`patchOpenCodeHooks`, and the dispatch switch
(`main.go:812-820`, `installClaude()`/`installGemini()`/`installAgy()`/`installOpenCode()`). The
existing installers share one shape:

1. Copy the `ssq-hooks` binary to `~/.local/bin/ssq-hooks`.
2. Write/patch a generated hook artifact (JSON-config patch for Claude/Gemini/agy;
   a generated plugin *file* — closest analogue — for OpenCode, since opencode has no JSON hook
   config, just an auto-loaded plugin directory).
3. Idempotency: re-running produces the same result (`patchOpenCodeHooks`'s doc comment: "pure
   function of ssqHooksPath... no explicit already-present check needed").
4. Cleanup of stale artifacts from earlier installer versions (`removeStaleOpenCodeWrapper`,
   `removeAntigravityHookEntry`).

pi's `.pi/extensions/*.ts` model is structurally closest to **OpenCode's plugin file**, not
Claude/Gemini's JSON-patch model: no config file to merge into, just a generated file dropped in
a directory the agent auto-discovers. `openCodePluginContent`'s use of `encoding/json.Marshal` to
safely embed the binary path as a JS string literal (rather than `fmt.Sprintf %q`, called out in
the code comment as importantly *not* JS-escaping-safe) is a template directly reusable for pi's
TS string embedding.

Adding `installPi()` following this exact shape is low-risk and consistent:
- Dispatch: add a `"pi"` case alongside `main.go:812-820`.
- Template: a `piExtensionTemplate` const analogous to `openCodePluginTemplate`, registering
  `pi.on("tool_call", ...)`, calling `ssq-hooks check --pi` via `execFileSync` (or pi's
  equivalent child-process API) with the same bounded timeout and fail-closed catch.
- Write target: `.pi/extensions/ssq-hooks.ts` (project-local, matching the requirement) — note
  the trust-gate risk above applies specifically because this is project-local, unlike
  OpenCode's global-only plugin dir.
- `ssq-hooks check --pi` needs a new subcommand branch parsing pi's tool-call JSON shape (name/
  input/session/cwd) and returning the same allow/deny exit-code contract the other agents use —
  this reuses `approval_service.go` classification logic, satisfying the RulesService-parity
  requirement.

**Verdict: Recommended.** Extend `cmd/ssq-hooks` with `installPi()` mirroring `installOpenCode()`
rather than building a separate injection mechanism. This keeps one binary, one install
entrypoint, one exit-code contract, and one classifier path across all five agents — directly
satisfying requirements.md's "not a parallel, disconnected rule system" constraint, and it is the
lowest-risk option specifically because three of the four precedents (`installGemini`,
`installAgy`, `installOpenCode`) already solve "idempotent generated-file injection into a
third-party agent's extension surface," which is exactly this problem.

## Summary table

| Option | Verdict |
|---|---|
| Adopt/fork a community pi approval extension (rytswd/MasuRii/joeygibson) | Not recommended — none integrate with an external classifier; adapting one is no cheaper than writing from scratch and adds an unmaintained dependency |
| Add a Go JSONL parsing library | Not recommended — `bufio.Scanner`+`encoding/json` already used twice in-repo (`claude_adapter.go`, `agy_adapter.go`); stay consistent |
| SaaS/managed API | Not applicable — local CLI orchestration |
| Hand-write the TS approval extension | Viable/Recommended, conditioned on reusing `ssq-hooks`'s existing exit-code/timeout contract and treating the health-signal requirement as mandatory given pi's fail-open-by-default design |
| Extend `cmd/ssq-hooks` with `installPi()` (mirroring `installOpenCode()`) rather than a new injection mechanism | Recommended |
