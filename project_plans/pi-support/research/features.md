# Research: Features — pi Support (Agent 2)

## 1. What exists today for other non-Claude agents

The codebase already has two prior parity efforts to learn from: `agy-support` (older,
now superseded by the `ssq-hooks`-era architecture it describes) and
`antigravity-opencode-parity` (2026-07/08, matches the *current* codebase — this is the
primary precedent to mirror for pi).

### 1a. Status detection — `session/detection/binaries/*.go`
Each agent gets its own `BinaryDetector` implementation (`AgyDetector`, `OpencodeDetector`,
`GeminiDetector`, `ClaudeDetector`, `AiderDetector`), registered in
`session/detection/registry.go:DefaultRegistry()`. Each returns a `PatternSet` of regexes
per state (`Ready`, `Processing`, `NeedsApproval`, `InputRequired`, `Error`, `Idle`,
`Success`) matched against scrollback text — this is the mechanism pi's `--mode json`/RPC
status should plug into (either as a new `BinaryDetector` keyed on regex-matched JSON
lines, or as a structurally different event-sink path — see `session/detection/event_sink.go`
and `plugins.go` for the plugin-detector escape hatch already used for out-of-tree
detectors).

**Hard lesson from antigravity-opencode-parity (R3/R4/P4/P5/P6):** detectors for agy/opencode
were initially written as "clone the sibling TUI's regexes" guesses, and every state category
that wasn't guessed (`InputRequired`, `Error`, `Idle`, `Success`) shipped as an **empty
slice** with **no regression test** — silent misreporting that nothing catches until a live
session hits it. Every one of pi's status states needs (a) a captured real sample under
`session/detection/testdata/`, and (b) a positive+negative match test, not just a
"non-nil slice" structural test. Since pi is JSON/RPC-based rather than TUI-scrollback-based,
this may be a genuinely new detector shape, not a regex clone — verify against a live `pi`
process before writing patterns.

### 1b. Resume-flag injection — `session/claude_command_builder.go`
`ClaudeCommandBuilder.Build()` is single-purpose and hardcoded to the string `"claude"`
(`isClaudeCommand()` checks the basename of the first word). It:
1. Passes non-Claude commands through unchanged.
2. No-ops if there's no `ClaudeSessionData`/`ConversationUUID`.
3. Validates the ID is UUID-shaped (`isValidUUID`) before trusting it, warning (not
   erroring) and falling back to the bare command on mismatch.
4. Appends `--resume <uuid>`.

For pi, requirements specify `pi --session <id>` (or `-c`) — the ID format is NOT
guaranteed to be a UUID (unconfirmed against the actual pi CLI per the requirements'
Feasibility Risks). **This builder is not designed for multiple agents** — building pi
support means either generalizing it to a strategy per detected program, or adding a
sibling `PiCommandBuilder` with its own ID-validation rule. Reuse the "warn + fall back to
bare command" safety pattern regardless — resume flag injection must never crash a session
start.

### 1c. Approval-hook injection — `server/services/hook_injector.go` + `approval_handler.go`
Two files do double duty and are easy to conflate:
- `hook_injector.go`: generic `InjectHooksConfig(rootDir, sessionTitle, hooks []HookName, opts...)`
  — writes/merges `.claude/settings.json`'s `hooks.<Event>` array, with a `WithRemoteHookTarget`
  option for cross-host approval (curl to a webhook). Has explicit re-entrancy/idempotency
  handling (`hookAlreadyPresent`, `prependHookEntry`) and JSON-repair-on-corruption
  (`repairSettingsJSON`) — corrupted `settings.json` is a real failure mode this file already
  defends against.
- `approval_handler.go`: `InjectHookConfig`/`InjectHookConfigRemote` (note: singular "Hook",
  distinct near-duplicate name from `hook_injector.go`'s plural) plus `HandlePermissionRequest`,
  the actual HTTP handler the injected hook's `curl` command calls at runtime. This is where
  `PermissionRequestPayload` → `classifier.Classify()` → allow/deny/escalate happens, with
  Slack/dashboard notification, analytics recording, and headless-pool auto-approval branches.

pi's design is structurally different (a **TypeScript extension file** registering a runtime
`tool_call` handler, not a static JSON hook entry) — so pi needs a *new* injector
(write/maintain `.pi/extensions/<name>.ts`) but should terminate at the **same**
`HandlePermissionRequest`-shaped endpoint/classifier path, not a parallel approval system.
Concretely: the generated extension's HTTP call target should be `hookApprovalURL()`
(`approval_handler.go:812`) or an equivalent pi-specific route that still calls into
`RulesService`/`classifier.Classify()`, exactly as the requirements' "not a parallel,
disconnected rule system" scope line demands.

### 1d. Rule sourcing — `server/services/rules_service.go`
`RulesService.allRuleSpecs()` merges user + seed + `"claude-settings"`-sourced rules; the
`claude-settings` source is populated by `ReloadClaudeSettingsRules` (parses
`~/.claude/settings.json` + project-level equivalents) and is explicitly Claude-shaped —
`filterRulesBySource`, `rebuildClaudeSettingsRules`, and the RPC name itself
(`ReloadClaudeSettingsRulesRequest`) all bake in "claude-settings" as the source label.
Requirements flags this directly ("RulesService is Claude-settings-shaped in places").
Adding a `pi-extension` (or similarly named) `RuleSource` is a real design decision for the
planning phase, not a research-phase implementation detail — but the shape to extend is now
concrete: add a new `classifier.RuleSource` value, a `rebuildPiExtensionRules`-style
hot-swap function mirroring `rebuildClaudeSettingsRules`, and decide whether pi rules come
from a separate reload RPC or a generalized one.

### 1e. Program presets / UI — `web-app/src/lib/constants/programs.ts`
Confirmed pattern from agy-support features.md: a `{ value, label, description }` entry in
`programs.ts` plus an emoji/icon mapping in the session-row component is what makes a program
a "first-class preset" rather than free text. `config.GetAvailablePrograms()` filters
candidates through `exec.LookPath`, so a preset entry is safe to add unconditionally — it
just won't appear in the dropdown if the `pi` binary isn't installed.

### 1f. Feature flags — `config/config.go:1496-1540`
`Config.FeatureFlags map[string]bool` + `GetFeatureFlag`/`GetFeatureFlagWithDefault`/
`SetFeatureFlag` is the existing generic opt-in mechanism (nil-safe, defaults false). This is
exactly the mechanism the requirements' "gated behind an opt-in setting, defaulting off" scope
line calls for — no new flag infrastructure needed, just a new flag name (e.g.
`"pi-support"`) gating all four pi-support surfaces (resume, UI preset, status parsing,
approval extension).

## 2. Edge cases / failure modes pi's design must handle

Distilled from agy-support's `pitfalls.md` (older, ssq-hooks-era) and
antigravity-opencode-parity's `pitfalls.md`/`features.md` (current-era, directly
analogous):

1. **Payload/schema drift is the #1 recurring failure across every prior agent-parity
   effort.** agy-support's top pitfall (P-1, CRITICAL) was an unconfirmed hook payload
   schema; antigravity-opencode-parity's R4 addendum shows the *opposite* mistake — declaring
   "no PreToolUse-equivalent exists" from reading only the declarative config schema and
   missing the separate plugin/extension API that actually has one. For pi: verify the
   **installed** pi version's actual `ExtensionAPI`/`tool_call` payload shape and
   `ctx.ui.confirm()` contract by running it, not by trusting `pi.dev/docs/latest` (the
   requirements' own Feasibility Risks flag this — docs may be ahead of or behind the
   installed binary, as literally happened with agy 1.0.15-vs-1.0.13).
2. **Prompt/argument delivery mismatches (P1/P3 in antigravity-opencode-parity):** agy's
   `--print` takes the prompt as a CLI arg, not stdin; opencode's `run` takes a positional
   arg, not stdin. If pi's non-interactive/status mode (`--mode json`) has a similar
   stdin-vs-arg mismatch for however stapler-squad drives it, this silently produces empty
   or hung one-shot calls rather than an error — must be verified against a live `pi
   --mode json` invocation, not assumed to match Claude's convention.
3. **Trust-gate interaction (explicit in pi-support requirements' Rabbit Holes):** pi's
   project-local extensions only load after the project is trusted. This has no direct
   precedent in agy/opencode (their hook injection is unconditional), so it's a genuinely
   new failure mode: injecting the approval extension into an untrusted worktree could
   silently produce zero enforcement. The requirements already call for an install/health
   signal to catch this — model it explicitly (e.g., a startup probe that confirms the
   extension actually registered, not just that the file was written), analogous to how
   `hook_injector.go` already treats "hook written to disk" and "hook actually effective" as
   two different, independently-verifiable facts (JSON-repair-on-corruption exists precisely
   because "wrote successfully" ≠ "will be read correctly").
4. **Double-write / partial-install races on shared config files (agy-support P-4, P-9;
   antigravity-opencode-parity P2):** agy-support's `installAgy()` bug (patched two config
   paths unconditionally, causing double-fired hooks and partial-install on failure) is a
   direct precedent for pi's `.pi/extensions/*.ts` injection — use atomic write (temp file +
   rename), first-found/single-target logic (don't guess at multiple candidate extension
   directories and write to all of them), and an idempotency check before overwriting.
5. **Exit-code / response-format contract per agent is not uniform** (antigravity-opencode-parity
   R4/P found three *different* approval-gate shapes across agy hooks.json, Gemini
   `BeforeTool` string+exit-code, and opencode's throw-to-block plugin hook). pi's
   `tool_call` handler + `ctx.ui.confirm()` round-trip is a fourth shape again — don't assume
   it matches any prior agent's allow/deny signaling; confirm pi's actual contract
   (return value? thrown exception? async confirm() response?) before wiring
   `HandlePermissionRequest`'s decision back into it.
6. **RPC-mode + approval-extension interaction risk is explicitly called out in the pi
   requirements' Rabbit Holes** ("careful protocol sequencing to avoid deadlock or missed
   events") — this has no direct precedent in agy/opencode research (neither of those used
   RPC mode simultaneously with hook injection) and is pi-specific new risk surface for the
   architecture research agent / planning phase to scope carefully.
7. **Detection patterns as unverified clones (antigravity-opencode-parity P4/P6):** don't
   let pi's status detector ship with any state's regex/parser written from a guess about
   "how pi probably formats X" — every state needs a captured real sample and a
   positive+negative test, same lesson as agy/opencode's InputRequired/Error/Idle/Success
   gaps.
8. **`--gemini`/`--antigravity`-style flag conflation across agents is itself a smell** —
   agy-support's `handleCheck()` grew a single boolean flag (`--gemini`) that was later
   reused for antigravity too. For pi, prefer a dedicated `--pi`-shaped path (or whatever the
   planning phase's classifier extension point looks like) rather than overloading an
   existing non-Claude flag, to avoid the schema-conflation risk in point 1.

## 3. Unstated needs — what agy-support / antigravity-opencode-parity learned the hard way

- **"It has a hook" is not enough — the hook has to actually be verified to fire, with a
  live capture, before the classifier-mapping code is written.** Every "CRITICAL" or
  "BLOCKER" pitfall in both prior projects was exactly this: research read documentation or
  static types and drew a conclusion that a live test later overturned (agy's `--print`
  needing an arg not stdin; opencode's "no PreToolUse" reversed by the R4 addendum). The
  unstated need behind "verify against the installed pi version" in the requirements'
  Feasibility Risks is not a formality — it is the single highest-leverage research step,
  and it should happen *before* committing to an extension-payload design, not after.
- **Every previously-shipped "empty state = fine" shortcut became a real, silent
  session-list bug.** The unstated expectation is that pi's status states are only
  "supported" once each has a passing accuracy test against real captured output — an
  empty pattern slice that merely doesn't crash is not acceptable parity, even though
  nothing in the requirements literally says "write a regex test."
- **Approval enforcement that can silently go dark is worse than no enforcement**, because
  a user relying on it believes they're covered. Both prior projects' worst pitfalls
  (P-1/P2/P4/P5 across the two agy docs) are all variants of "the gate looks installed but
  doesn't actually work, and nothing tells you." pi-support's own Observability Requirements
  section (extension install/health signal) is explicitly trying to prevent a repeat of
  this — treat that health signal as load-bearing, not a nice-to-have, given the track
  record.
- **Reuse over parallel systems is a hard-won principle, not just tidiness.** The
  "Alternatives Considered" rejection of a separate pi-only rules system, and
  antigravity-opencode-parity's insistence on routing through the same classifier/webhook
  path rather than a bespoke opencode-specific one, both reflect the same lesson: a second,
  disconnected approval/audit system fragments the one thing (`RulesService`/`audit.go`)
  a user actually checks when deciding whether they're safe. Any pi-specific plumbing added
  in planning should be judged against "does this still funnel into the one audit trail."
- **Naming collisions between near-identical functions are a real maintenance hazard** —
  `hook_injector.go`'s `InjectHooksConfig` vs. `approval_handler.go`'s `InjectHookConfig`
  (singular/plural, different files, different behavior) is already a trap in this codebase.
  Planning should give pi's new injector a name that can't be confused with either existing
  one (e.g. `InjectPiExtension`, not `InjectHookConfigPi`).

## Key files for the planning phase

- `session/claude_command_builder.go` (+ `claude_command_builder_test.go`) — resume-flag precedent
- `session/detection/registry.go`, `session/detection/binaries/{agy,opencode,gemini}.go`,
  `session/detection/binaries/*_test.go`, `session/detection/testdata/` — status-detector precedent and testdata convention
- `session/detection/event_sink.go`, `session/detection/plugins.go` — possible plugin-detector escape hatch for JSON/RPC-based status instead of scrollback regex
- `server/services/hook_injector.go`, `server/services/approval_handler.go` — Claude hook injection + approval HTTP handler (`HandlePermissionRequest`, `hookApprovalURL`)
- `server/services/rules_service.go` — `allRuleSpecs`, `ReloadClaudeSettingsRules`, `rebuildClaudeSettingsRules`, `RuleSource` filtering
- `config/config.go:1496-1540` — `FeatureFlags` opt-in mechanism to reuse directly
- `web-app/src/lib/constants/programs.ts` — program preset entry point
- `project_plans/agy-support/research/{features,architecture,pitfalls}.md` — older ssq-hooks-era precedent (useful for pitfall patterns, but architecture is stale)
- `project_plans/antigravity-opencode-parity/research/{features,pitfalls}.md` — current-architecture precedent, most directly analogous to pi-support
