# Research: Pitfalls — pi Support

**Status**: Completed | **Phase**: 2 — Research

---

## PITFALL-1 (CRITICAL, SECURITY): Silent extension load failure creates a false sense of enforcement

### Risk
The requirements' "Risk Control" is a feature flag that gates the *whole feature* — but once a user opts in, the thing that actually enforces approval rules for pi is a separately-loaded artifact: a project-local `.pi/extensions/*.ts` file. Per `https://pi.dev/docs/latest/extensions`, project-local extensions "load only after the project is trusted," and pi's docs do not state what happens when an extension fails to load (crash, syntax error, missing file) — no confirmed warning, no confirmed hard failure. There are at least three independent ways stapler-squad's approval extension silently doesn't run even though the user believes pi support (and therefore enforcement) is on:

1. **Project-trust gate**: first run of an untrusted directory (a new worktree — stapler-squad creates one per session) never loads the project-local extension at all, and only *global* or CLI-supplied (`-e`) extensions get a vote on trust — the injected extension itself can't bootstrap its own trust.
2. **Injection/write failure**: analogous to the existing `InjectHooksConfig` call in `server/mcp/tools_lifecycle.go:263`, which on error only does `log.Warn("mcp hook injection failed for session", ...)` and lets session creation proceed — i.e. **this exact silent-degradation pattern already exists in the codebase today for Claude hooks**. A pi extension write failure ported the same way would leave a session running with zero tool gating and no user-visible signal.
3. **Runtime failure inside pi itself** (syntax error in the generated `.ts`, an unhandled exception in the `tool_call` handler, an incompatible extension API after a pi upgrade) — this happens entirely inside the `pi` process, outside any Go code path stapler-squad controls, so even a perfect "did injection succeed" check on the Go side cannot detect it.

Unlike Claude Code's hook model (a static JSON entry Claude itself validates and where a malformed hook typically surfaces as a Claude-side error), pi's extension is TypeScript executed by pi's own runtime — failures there are opaque to the orchestrator that requested them.

### Why this matters more here than for Claude hooks
For Claude, `InjectHooksConfig` failing merely means *no hook is registered* — a state that is at least discoverable by re-reading `.claude/settings.local.json`. For pi, "the file exists on disk" is necessary but *not sufficient* for enforcement — trust-gating and in-process load success are two additional silent failure points that have no file-system-visible signal at all.

### Mitigation
- Treat this as the single most important design constraint from the requirements' Observability section ("extension install/health signal per session ... so a silent injection failure doesn't silently disable enforcement"). Do not implement it as a nice-to-have; implement it before shipping the feature flag as "on."
- Get an explicit, verifiable **health signal from inside the pi process**, not just "file written." Candidates: have the extension itself emit a startup event (e.g. via `ctx.ui.setStatus` / `notify`, or a custom RPC event if the extension API allows) that stapler-squad's RPC-mode listener can assert it received within N seconds of session start; if it's absent, surface a hard warning banner in the session list — not a log line.
- Handle the trust gate explicitly: since only global/CLI extensions can vote on trust, consider shipping the approval extension as a **global** extension (`~/.pi/agent/extensions/`) rather than project-local, or auto-answer the `project_trust` event from a global helper extension, so the approval gate never depends on a per-worktree trust decision the user hasn't made yet.
- Never present "pi support enabled" and "approval rules enforced for pi" as the same claim in the UI. Model them as two independent statuses (mirroring the requirements' explicit ask for a per-session extension-health signal), and default to treating "health unknown" as "not enforced" (fail closed in the UI's claim, even though pi itself may fail open on tool execution).
- Root cause to design against: the feature flag controls *rollout*, not *enforcement correctness* — a flag being "on" says nothing about whether the specific mechanism gating tool calls in this specific session actually loaded. This is the class of bug the existing `hook_injector.go` silent-`log.Warn` pattern already has for Claude; do not copy that pattern forward for pi without the health-check safety net the requirements call for.

---

## PITFALL-2 (HIGH): JSONL framing — Go's `bufio.Scanner`/`bufio.NewReader.ReadString('\n')` are safe here, but only by accident of choice, not by default Go idiom

### Risk
pi's RPC docs (`https://pi.dev/docs/latest/rpc`) state strict JSONL framing: split only on `\n` (LF), and explicitly warn that Node's `readline` is "not protocol-compliant" because it also splits on `U+2028`/`U+2029`, which can legally appear inside a JSON string value. This warning is Node-specific, but the underlying risk is protocol-level, not language-specific: **any line-oriented reader that treats Unicode line/paragraph separators as record boundaries will corrupt the JSON stream** if a pi output string (e.g. an agent's text response) happens to contain `U+2028` or `U+2029`.

Go's `bufio.Scanner` with the default `ScanLines` split function only splits on `\n` (and strips a trailing `\r`) — it does **not** split on `U+2028`/`U+2029`, so a naive `bufio.Scanner`-based line reader in Go is actually protocol-compliant by default, unlike Node's `readline`. This is good news, but it's easy to accidentally regress:
- Using `strings.Split(output, "\n")` on a buffer that was assembled with a different newline-normalization step (e.g. any code path that does Unicode-aware line splitting, `\R` regex-based splitting, or a terminal-scrollback-oriented line splitter reused from Claude's status-detection code) could reintroduce the Node-equivalent bug.
- Any dependency-provided JSON-stream framer (an off-the-shelf "line-delimited JSON" library) may or may not honor "split on LF only" — must be verified, not assumed, before reuse.

### Mitigation
- When implementing the pi RPC/JSON reader, use `bufio.NewScanner` (default `ScanLines`) or an explicit `bufio.Reader.ReadString('\n')` loop reading raw bytes — do not route pi's stdout through any of stapler-squad's existing terminal-scrollback/ANSI-aware line-splitting utilities built for Claude Code's TUI output (those were designed for human-readable terminal text, not machine JSONL, and may have Unicode-line-boundary handling tuned for display rather than protocol correctness).
- Add a unit test asserting the pi JSONL reader does NOT split a line containing an embedded `U+2028` character — the single concrete, named risk from the docs, and it's cheap to pin down.
- Verify with `go doc bufio.ScanLines` / source before shipping, since this is exactly the kind of claim that should be pinned to source, not memory: `bufio.ScanLines` (Go stdlib) drops the line terminator by scanning for `\n` only.

---

## PITFALL-3 (MEDIUM): pi's extension/RPC API has no documented stability guarantee — version drift breaks the injected extension with no warning

### Risk
Confirmed by direct fetch of `https://pi.dev/docs/latest/extensions` and `/rpc`: neither page states a semver policy, deprecation window, or backward-compatibility commitment for the extension API or RPC event schema. The docs even hint at forward evolution ("a fork/clone option reserved for future conversation restore control"). Concretely, event names in the docs (`tool_execution_start/update/end`, `agent_end`, `agent_settled`) differ from the *event name* implied in the requirements/rabbit-holes doc (`tool_call`) — the requirements assume a `tool_call` event handler; the RPC event stream instead exposes `tool_execution_start/update/end` correlated by `toolCallId`. This is itself a small piece of version drift evidence: either the extension-hook API (`pi.on('tool_call', ...)`, in-process) and the RPC event stream (`tool_execution_*`, cross-process) are two genuinely different vocabularies for the same concept, or the docs are already inconsistent — either way, an implementation that assumes the wrong one will silently miss events.

### Mitigation
- Re-verify against the actually-installed pi binary version before implementation, exactly as the requirements' Feasibility Risks already flag — do not trust the fetched `/docs/latest` pages as necessarily matching the pinned/installed version.
- Since the extension-injected `tool_call` handler and the RPC `tool_execution_*` event stream are two separate mechanisms (in-process interception vs. out-of-process status stream — matches the requirements' "RPC mode's approval surface is indirect" rabbit hole), do not conflate them in the implementation: use the extension for blocking/approval (the only mechanism that can actually gate execution), and RPC/JSON events only for session-list status — never assume RPC events alone can enforce anything.
- Pin the exact pi version stapler-squad supports/tests against (a version check at session-start, logged and/or surfaced similarly to the health signal in Pitfall 1) rather than assuming forward compatibility. Treat an extension API shape change after a pi upgrade as an operational incident class, not a one-time migration.

---

## Precedent Pitfalls From Similar Past Integrations (agy / opencode)

Cross-checked `project_plans/agy-support/research/pitfalls.md` and `project_plans/antigravity-opencode-parity/research/pitfalls.md` — both integrated third-party CLI agents' hook/detection surfaces into this same approval/status infrastructure. Applicability to pi:

| Precedent pitfall | Applies to pi? | Why |
|---|---|---|
| P-1 (agy): unknown/unverified hook payload schema, needs live capture before coding | **Yes** | Same root issue as PITFALL-3 above — the `tool_call` event's actual `input`/`toolCallId` shape should be captured from a live pi run, not assumed from docs alone. |
| P-2 (agy): non-string config field breaks naive JSON patching | **Partially** | pi's extension is a whole `.ts` file, not a JSON key being patched — but the *config* registering the extension path (`settings.json`'s `extensions` array) is patched JSON and should get the same "don't silently clobber an unexpected existing value" guard as `patchBeforeToolHook`. |
| P-4 (agy): write/read race between install and a running process | **Yes** | Injecting/updating the `.pi/extensions/*.ts` file while a pi session is mid-run risks the same partial-read race `hook_injector.go`-style atomic-rename (`tmp` + `os.Rename`) already guards against for Claude — apply the same pattern to the pi extension file write. |
| P-5/P-2 (agy): multiple candidate config paths, patch only first-found to avoid double-fire | **Yes, in a different shape** | pi has two extension scopes (global `~/.pi/agent/extensions/` vs project `.pi/extensions/`) with different trust semantics (see Pitfall 1) — placing the approval extension in the wrong one, or both, risks either the trust-gate silent-skip or duplicate tool_call handlers firing twice for one call. Decide scope deliberately; don't default to "just project-local" the way agy's installer defaulted to "patch both paths" and had to be walked back. |
| P-6 (agy): hook output-format incompatibility (stdout JSON vs exit-code-only contract) assumed wrong | **Yes** | The requirements' open question ("same webhook endpoint... or new pi-specific endpoint") is exactly this risk restated: verify pi's actual `tool_call` block/allow return contract (a return value, not stdout/exit-code as in the Claude-hook-shell-command model) against source/live testing before wiring `RulesService` through it. |
| P-8 (agy): pre-GA schema churn risk | **Yes** | Matches PITFALL-3 — pi's extension API is explicitly not guaranteed-stable; same design response (multi-variant tolerance / version pinning / health signal) applies. |
| P4/P5/P6 (antigravity-opencode): CLI status-detection patterns copied from a sibling tool without verifying against the tool's actual TUI/output strings | **Yes, for status parsing** | The requirements note pi's idle/waiting-for-input state is *inferred*, not an explicit event — exactly the "unverified pattern guess" failure mode from the antigravity-opencode precedent. Capture real pi JSON/RPC event sequences (not TUI text, since pi is run in `--mode json`/RPC rather than raw TUI) for each state transition before writing detection logic, and add positive/negative-match tests per the antigravity-opencode P6 recommendation. |
| P1/P3 (antigravity-opencode): CLI prompt-delivery mechanism (stdin vs positional arg) assumed wrong, breaking one-shot invocation | **No — not directly applicable** | pi-support here is about long-running interactive sessions with resume-flag injection, not one-shot `CLIAIClient`-style prompt delivery; this precedent's specific failure mode (stdin vs `--print "arg"`) doesn't map onto this project's scope. Still worth a quick sanity check when wiring `--session <id>`/`-c` resume-flag injection, since flag-vs-positional-arg mistakes are the same *class* of error even if this specific precedent doesn't transfer. |

---

## Summary of Risk Levels

| Pitfall | Severity | Action |
|---|---|---|
| P-1: Silent extension load failure = false enforcement | CRITICAL (security) | Implement an in-process health signal (not just file-write success) before shipping; treat "health unknown" as "not enforced" in the UI; consider global-scope extension to dodge project-trust gating |
| P-2: JSONL framing / Unicode line separators | HIGH | Use `bufio.Scanner`/raw LF-only reader for pi JSONL, not any Unicode-aware or terminal-oriented line splitter reused from Claude code; add a test with an embedded U+2028 |
| P-3: No documented API stability guarantee; `tool_call` vs `tool_execution_*` vocabulary mismatch | MEDIUM | Re-verify against installed pi version; keep extension-based blocking and RPC-based status strictly separate mechanisms; version-pin and health-check |
| Precedent: unverified payload/status-pattern schemas (agy/opencode) | MEDIUM | Live-capture real `tool_call` input shape and JSON/RPC status event sequences before coding; add positive/negative pattern tests |
| Precedent: write/race + multi-path config ambiguity (agy) | LOW–MEDIUM | Atomic rename for extension file writes; deliberately choose global vs project-local extension scope, don't default to "patch both" |

**Top 3 to address first**: P-1 (health signal design, before any code), P-3 (verify `tool_call` vs `tool_execution_*` against the actual installed pi version and actual extension return-value contract), P-2 (pin down the JSONL reader implementation with a Unicode-line-separator test).
