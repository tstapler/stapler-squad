# Feature Research: Antigravity CLI (agy) + Open Code (opencode) Parity

Research date: 2026-07-01
agy version: 1.0.15 (system; requirements doc said 1.0.13 — binary is newer)
opencode version: 1.4.0

---

## R1: Agy one-shot AI client (`--print` flag)

**Confirmed.** `agy --help` output:

```
--print        Run a single prompt non-interactively and print the response
-p             Short alias for --print
--print-timeout Timeout for print mode wait (default 5m0s)
```

`agy --print` is the correct one-shot flag, matching Claude's `--print`. It is absent from `knownCLIAgents` in `server/services/cli_ai_client.go`. The correct spec:

```go
{
    Name:   "agy",
    Binary: "agy",
    Args:   func() []string { return []string{"--print"} },
    PromptSeparator: "\n\n---\n\n",
}
```

Priority: below Gemini (which has no flag), above opencode. Stdin delivery matches Claude's pattern.

---

## R2: Agy hooks path — authoritative single-path fix

**Current bug:** `installAgy()` unconditionally patches both:
1. `~/.gemini/config/hooks.json`
2. `~/.gemini/antigravity-cli/hooks.json`

Both files currently have identical content (ssq-hooks installed in both). This is wrong — it should use first-found logic like `installGemini()`.

**Path discovery:**

From `strings /home/tstapler/.local/bin/agy` the binary contains references to both `~/.gemini/config` and `antigravity-cli`, so both directories are known to agy. Directory content confirms:

| Path | Contents |
|------|----------|
| `~/.gemini/antigravity-cli/` | hooks.json, settings.json, conversations/, brain/, knowledge/, skills/, mcp/, plugins/ — agy's **primary runtime state dir** |
| `~/.gemini/config/` | hooks.json, config.json, mcp_config.json — lighter-weight **global config dir** |

`~/.gemini/settings.json` is Gemini CLI's settings (contains `hooks.BeforeTool`), not agy's.

**Recommended candidate order** (matching `installGemini()` pattern):

```go
candidates := []string{
    filepath.Join(home, ".gemini", "antigravity-cli", "hooks.json"),  // primary (agy runtime dir)
    filepath.Join(home, ".gemini", "config", "hooks.json"),            // fallback (global config)
}
```

Rationale: `~/.gemini/antigravity-cli/` is where agy stores all runtime data including settings.json (which has `model`, `permissions`, etc.). Patch only the first existing file; create the primary if neither exists.

**Hook format** (already correct in `patchAntigravityHooks`):

```json
{
  "stapler-squad": {
    "enabled": true,
    "PreToolUse": [
      {
        "matcher": "*",
        "hooks": [
          {
            "type": "command",
            "command": "/home/tstapler/.local/bin/ssq-hooks check --antigravity",
            "timeout": 10
          }
        ]
      }
    ]
  }
}
```

This is the hooks.json format confirmed from the live file. The `stapler-squad` namespace key is a custom section (agy uses named hook groups). The `patchAntigravityHooks()` logic is correct; only the caller (`installAgy()`) needs to be fixed to use first-found logic.

---

## R3: Agy detection patterns

**Confirmed existing patterns (from `session/detection/binaries/agy.go`):**

| State | Pattern |
|-------|---------|
| Ready | `(?:◇|✓).*(?:Ready|ready)` |
| Processing | `(?:✦|⏲).*(?:Working|working)` |
| NeedsApproval | `(?i)Yes, allow once` and `(?i)Allow execution of:` |

**Missing states:** InputRequired, Error, Idle, Success — all empty slices.

agy uses the same TUI codebase as Gemini CLI (internal codebase `jetski`). Binary symbols confirm: `google3/third_party/jetski/...`. The Gemini detector should be used as a reference for what additional patterns may apply.

agy-specific UI patterns observed (from binary strings):
- `statusLine` — agy renders a status line
- `Approve all` — batch approval prompt string
- Various tool labels: `RunCommand`, `SearchCode`, `GithubUnstage`, `GithubDiscard`

For InputRequired: agy displays a text input prompt similar to Gemini. Likely candidate pattern: empty input prompt after tool completion, or explicit "Type your response" strings.

For Success/Idle: after completion agy likely returns to a `◇ Ready` or similar prompt — covered by existing `agy_ready` pattern.

**Recommendation:** Add the same patterns as GeminiDetector for InputRequired/Error/Idle/Success since both use jetski TUI. Validate against live agy sessions before submitting.

---

## R4: Open Code native hooks — does opencode have PreToolUse?

**No.** opencode 1.4.0 does NOT have a native PreToolUse/BeforeTool hook.

Evidence from `~/.config/opencode/node_modules/@opencode-ai/sdk/dist/gen/types.gen.d.ts`:

```typescript
hook?: {
    file_edited?: {
        [key: string]: Array<{
            command: Array<string>;
            environment?: { [key: string]: string; };
        }>;
    };
    session_completed?: Array<{
        command: Array<string>;
        environment?: { [key: string]: string; };
    }>;
};
```

The `hook` key (singular — different from `hooks`) supports only:
- `file_edited` — triggers after a file is written/edited
- `session_completed` — triggers when the session ends

Neither is a permission gate. There is no `tool_use`, `pre_tool_use`, or `before_tool` hook in opencode's hook system.

The `permission` config does allow coarse-grained tool gating:

```typescript
permission?: {
    edit?: "ask" | "allow" | "deny";
    bash?: ("ask" | "allow" | "deny") | { [key: string]: "ask" | "allow" | "deny" };
    webfetch?: "ask" | "allow" | "deny";
    doom_loop?: "ask" | "allow" | "deny";
    external_directory?: "ask" | "allow" | "deny";
};
```

This allows per-command-prefix bash gating (e.g., `{"rm -rf": "deny"}`), but it is static configuration — not a dynamic hook that can consult ssq-hooks' rule engine.

**Conclusion: The proxy wrapper approach for opencode is correct.** The current `installOpenCode()` creates a shell wrapper at `~/.local/bin/open-code` that calls `ssq-hooks proxy -- open-code "$@"`. Since opencode has no native PreToolUse hook, the only intercept point is to wrap the binary itself.

**Config file location:** `~/.config/opencode/opencode.json` (confirmed: `opencode debug paths` shows `config = /home/tstapler/.config/opencode`). The `$schema` in the config points to `https://opencode.ai/config.json`.

---

## R4 addendum (2026-08-11 re-verification): the plugin API is a real PreToolUse-equivalent

**The 2026-07-01 research above is incomplete, not wrong about what it checked.** It only inspected
the static `hook`/`permission` config schema (`opencode.json`) and missed OpenCode's separate JS/TS
**plugin system** (`@opencode-ai/plugin`), which exposes a `Hooks` interface with a
`tool.execute.before` hook — a real, `throw`-to-block, intercept-before-execute hook, functionally
equivalent to Claude's `PreToolUse`.

**Verified directly (VERIFIED, 2026-08-11):**
- Local install: `~/.config/opencode/node_modules/@opencode-ai/plugin/dist/index.d.ts` (package
  version 1.3.10, installed alongside opencode CLI 1.4.0 — `opencode --version`) declares
  `"tool.execute.before"?: (input: { tool: string; sessionID: string; callID: string }, output: { args: any }) => Promise<void>`
  on the `Hooks` interface, alongside `"tool.execute.after"`, `"permission.ask"`, and
  `"command.execute.before"`. This is distinct from — and not visible in — the `hook`/`permission`
  config-schema types (`file_edited`/`session_completed`/`permission.*`) that the original R4
  research read from `@opencode-ai/sdk/dist/gen/types.gen.d.ts`.
- Docs: [opencode.ai/docs/plugins/](https://opencode.ai/docs/plugins/) (page last updated
  2026-08-09) documents `tool.execute.before` with two worked examples — mutating
  `output.args.command` to escape a bash command, and `throw new Error(...)` inside the hook to
  block a `read` call on `.env` files. Throwing aborts the tool call; this is the block semantic
  `PreToolUse` needs.
- This hook predates the original 2026-07-01 research by roughly six months — it is not new
  since that research ran; the research simply looked at the wrong API surface (config schema vs.
  plugin `Hooks` interface).

**Two other plugin hooks exist and are relevant but should NOT be relied on for this class of
gating:**
- `"permission.ask"` — looks like the natural fit for a permission-prompt-driven gate, but is
  **broken/unreliable**, not merely undocumented. [Issue #7006](https://github.com/anomalyco/opencode/issues/7006)
  ("`permission.ask` plugin hook is defined but not triggered", open as of 2026-08-11) reports it
  never fires at all. [Issue #19927](https://github.com/anomalyco/opencode/issues/19927)
  ("`permission.ask` plugin hook is bypassed for first-encounter commands (`needsAsk=true`)",
  closed 2026-07-03 as `not_planned` by the repo's stale-issue bot, not because it was fixed —
  the linked root-cause analysis in the issue thread, an `if (!needsAsk)` guard in
  `packages/opencode/src/permission/index.ts`, was never merged) confirms the hook is unwired
  for exactly the case (a command seen for the first time) where a permission gate matters most.
  **Do not build `patchOpenCodeHooks()` on `permission.ask`** — use `tool.execute.before`, which
  is independently wired and has a working `throw`-to-block path.
- `"tool.execute.before"` coverage of subagent (`task`-tool-spawned) tool calls is **disputed, not
  confirmed broken**: [Issue #5894](https://github.com/anomalyco/opencode/issues/5894) originally
  reported subagent tool calls bypass `tool.execute.before` entirely (a claimed security-policy
  bypass), but maintainer replies in the thread (closed 2026-04-15 as `completed`) dispute the
  repro — one maintainer could not reproduce it and traced the reporter's test case to the
  subagent using the `bash` tool to invoke commands rather than a hooked tool; another maintainer
  confirmed hooks are loaded per-`Instance` and load fresh for the subagent's own session/prompt
  loop, so they should fire. Net: likely fine, but not independently re-verified against a live
  ssq-hooks-style plugin in this environment — worth a live-session check before the follow-on
  implementation item ships, not before this doc correction.

**`opencode run` (needed to live-test any of this) does not complete end-to-end in this
environment today** — see Issue 2 in `implementation/validation.md`'s adversarial-issues section
(a local `~/.config/opencode/agents/skills/*.md` config error). This blocks live verification of
`tool.execute.before` actually intercepting a real session in this environment; the findings above
are from static inspection of the installed plugin package's type declarations and the public docs
page, not a live-fired hook.

**Revised conclusion for R4:** OpenCode has no JSON/`hooks.json`-style declarative hook config
comparable to Claude's `settings.json` hooks — that part of the original research still holds. But
it does have a real code-level `PreToolUse`-equivalent via the JS/TS plugin API
(`tool.execute.before`), which the original research missed by only reading the config-schema
types. The proxy-wrapper approach (`installOpenCode()`) is no longer the only available intercept
point — replacing it with a native plugin-based hook is now feasible. That replacement
(`patchOpenCodeHooks()`) is scoped as its own follow-on backlog item, not implemented here.

---

## R5: Open Code detection patterns

**Current patterns (from `session/detection/binaries/opencode.go`):**

| State | Pattern |
|-------|---------|
| Ready | (empty) |
| Processing | `→\s*(Read|Write|Edit|Create|Delete)\b` |
| NeedsApproval | `\[\s*Allow\s*\([aA]\)\s*\]` |
| InputRequired | `┃\s*\d+\.\s+\w` and `(?i)Allow\s+once.*Allow\s+always\|...` |

**Missing:** Ready, Error, Idle, Success.

From SDK event types in `types.gen.d.ts`:
- `session.idle` / `session.status` with `type: "idle"` — opencode explicitly models idle state
- `session.status` with `type: "busy"` — processing
- `session.status` with `type: "retry"` — error/retry
- `permission.updated` / `permission.replied` — permission dialog events

For TUI detection:
- **Ready/Idle:** opencode TUI likely shows a prompt like `>` or `│` at idle. The `opencode run` output ends with no spinner when complete.
- **Error:** opencode likely shows error messages in the TUI. Candidate pattern: `(?i)error:|failed:` or ANSI error color sequences.
- **Success:** After `opencode run` completes successfully, the session ends — look for exit codes or final summary text.

These patterns require validation against live opencode TUI output. The current proxy-based intercept means opencode detection is purely TUI-based (no hook events available).

---

## R6: Open Code one-shot client validation

**Current:** `opencode run` with no additional args in `knownCLIAgents`.

From `opencode run --help`:
- `opencode run [message..]` — message as positional arg OR stdin
- `--dangerously-skip-permissions` flag (auto-approve permissions — NOT recommended for prod)
- No explicit `--stdin` or `--print` flag found

**Gap:** The current spec passes prompt via stdin (combined system+user prompt), but `opencode run` takes the message as positional `[message..]`. Stdin delivery may not work as expected for multi-line prompts.

**Recommended validation:** Test `echo "prompt" | opencode run` vs `opencode run "prompt"` to confirm stdin is read when no positional args given. If stdin does not work, the `Args` lambda needs to change to forward the prompt as a positional argument (requires refactoring `CLIAgentSpec` to support prompt-as-arg mode).

The `--format json` flag could be useful for structured output parsing but is not currently used.

---

## R7: Test coverage gaps (pre-implementation)

| Component | Current tests | Target |
|-----------|--------------|--------|
| AgyDetector patterns | 3 Ready/Processing/NeedsApproval | Add InputRequired, Error, Idle, Success |
| OpencodeDetector patterns | 3 Processing/NeedsApproval/InputRequired | Add Ready, Error, Idle, Success |
| `installAgy()` path logic | None observed | Unit test: first-found path selection |
| `agy` in knownCLIAgents | None | Unit test: binary found, --print arg used |

---

## Summary: Decision Matrix

| Requirement | Finding | Action |
|-------------|---------|--------|
| R1: agy --print | Confirmed from `agy --help` | Add spec to `knownCLIAgents` |
| R2: agy hooks path | Both paths written; ~antigravity-cli/ is primary | Fix installAgy() to first-found, primary = antigravity-cli |
| R3: agy patterns | InputRequired/Error/Idle/Success missing | Add jetski-equivalent patterns, validate live |
| R4: opencode native hooks | No PreToolUse hook exists (only file_edited, session_completed) | Keep proxy wrapper; document why |
| R5: opencode patterns | Ready/Error/Idle/Success missing | Research TUI output strings, add patterns |
| R6: opencode run stdin | Unconfirmed — needs live test | Test `echo | opencode run` before shipping |
