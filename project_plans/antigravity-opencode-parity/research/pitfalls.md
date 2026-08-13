# Pitfalls Research: Antigravity CLI + Open Code Feature Parity

## P1 — BLOCKER: agy `--print` requires an argument, not stdin

**Severity:** Blocks R1 (agy one-shot AI client).

Verified by running `echo "hello world" | agy --print`:
```
flag needs an argument: -print
```

The agy `--print` flag is a *string flag* that requires the prompt as its value:
```
agy --print "your prompt here"
```

This is fundamentally different from `claude --print`, which reads the prompt from stdin when no positional argument is provided. The current `CLIAIClient.Complete()` writes the combined system+user prompt to the process's stdin and passes no positional arguments. Adding agy to `knownCLIAgents` with `Args: func() []string { return []string{"--print"} }` will fail at runtime for every invocation.

**Required fix:** Extend `CLIAgentSpec` to support prompt-as-argument delivery, or verify that agy supports a stdin-based mode (e.g., `agy --print -` or `agy` with no flags and a TTY check). If agy has no stdin mode, `CLIAIClient` needs a new `PromptAsArg bool` field and a refactored `Complete()` that appends the prompt to `Args()` rather than writing it to stdin. Note that large prompts passed as command-line arguments risk hitting OS arg-length limits (~2 MB on Linux), so this is a design tradeoff.

**Files affected:**
- `server/services/cli_ai_client.go` — `CLIAgentSpec`, `CLIAIClient.Complete()`
- Possibly `server/services/cli_ai_client_test.go` if it exists

---

## P2 — MEDIUM: `installAgy()` patches both config paths unconditionally — double-fire and partial-install risk

**Severity:** Functional correctness + subtle regression.

`installAgy()` unconditionally patches BOTH:
1. `~/.gemini/config/hooks.json`
2. `~/.gemini/antigravity-cli/hooks.json`

Compare with `installGemini()`, which uses first-found logic and only patches ONE file. Two problems arise:

**Double-fire:** If agy reads hook configuration from both paths simultaneously, the `ssq-hooks check --antigravity` command runs twice per tool call. Each invocation records a separate analytics entry, and the second invocation's decision may arrive after the first's deny/allow is already processed, leading to undefined behavior in the agy hook pipeline.

**Partial-install on failure:** The loop in `installAgy()` calls `os.Exit(1)` if the second path write fails, leaving `~/.gemini/config/hooks.json` patched but `~/.gemini/antigravity-cli/hooks.json` not. This is an inconsistent state that a re-run of `ssq-hooks install agy` may not heal correctly (first path idempotent, second path may throw a different error).

**Test gap:** `TestInstallAgy_FreshInstall` only asserts that `~/.gemini/antigravity-cli/hooks.json` is patched. It never checks whether `~/.gemini/config/hooks.json` was created. There is no analog to `TestInstallGemini_PatchesOnlyFirstFound` for agy. After switching to single-path logic, a test for "both files present → patch only first-existing" must be added (mirroring the Gemini test pattern).

**Required fix:** Research the authoritative agy v1.0.13 hooks path (check agy documentation or `agy install --help`). Change `installAgy()` to use the same candidates+first-found loop as `installGemini()`. Add a test mirroring `TestInstallGemini_PatchesOnlyFirstFound`.

**Files affected:**
- `cmd/ssq-hooks/main.go` — `installAgy()`
- `cmd/ssq-hooks/main_test.go` — add `TestInstallAgy_PatchesOnlyFirstFound`, `TestInstallAgy_UsesConfigJson`, etc.

---

## P3 — MEDIUM: `opencode run` takes the message as positional args, not stdin

**Severity:** Blocks R6 (opencode one-shot client validation) if stdin is assumed.

Verified: `opencode --help` shows:
```
opencode run [message..]     run opencode with a message
```

Testing `echo "hello" | opencode run` produces a configuration error — opencode does not read the prompt from stdin. The message must be supplied as positional arguments: `opencode run "do this task"`.

The current `knownCLIAgents` entry for opencode uses:
```go
Args: func() []string { return []string{"run"} },
```

This passes no message argument. When `CLIAIClient.Complete()` pipes the prompt to stdin, opencode will either fail (no message given) or silently ignore stdin and start in interactive mode — neither of which returns the expected one-shot response.

**Required fix:** Same architectural constraint as P1: either verify that `opencode run` reads from stdin when the `[message..]` positional is absent, or extend `CLIAgentSpec` to support passing the prompt as a positional argument. If opencode reads from stdin, document which version confirmed this behavior.

**Note:** opencode's `run` subcommand may support `opencode run --help` for more details on stdin support.

**Files affected:**
- `server/services/cli_ai_client.go` — opencode `CLIAgentSpec.Args`

---

## P4 — LOW: agy detection patterns are unverified Gemini clones — missing 5 state categories

**Severity:** Silent session status misreporting.

`agy.go` comment: "pattern strings are identical to GeminiDetector because agy shares the same TUI codebase." This assumption has not been verified against agy v1.0.13's actual terminal output. If agy customized its UI strings (e.g., "✦ Thinking" instead of "✦ Working"), the pattern `(?:✦|⏲).*(?:Working|working)` will never match and sessions will always show Processing=false.

Additionally, the following state categories are completely absent for agy:
- `InputRequired` (empty slice) — agy may prompt for user confirmation outside of NeedsApproval flows
- `Error` — no error state detection; API failures or rate limits will show as idle/unknown
- `Idle` — no patterns for the agy readline/input prompt after a completed turn  
- `Success` — no patterns for turn-complete indicators

**Risk:** R3 compliance requires "patterns for states currently missing." Without verifying the actual agy TUI output strings first, any patterns added will be guesses that may produce false positives or false negatives.

**Required fix:** Capture actual agy v1.0.13 terminal output in a tmux session (use `tmux capture-pane -p`) for each state: idle after startup, while processing, during a permission prompt, after a completed turn, and on an error. Use captured strings to write pattern tests that assert matching/non-matching against real output before adding to `agy.go`.

**Files affected:**
- `session/detection/binaries/agy.go` — `AgyDetector.Patterns()`
- `session/detection/binaries/agy_test.go` — add per-pattern matching tests

---

## P5 — LOW: opencode detection patterns lack Ready, Error, Idle, Success

**Severity:** Session status always shows unknown for most opencode states.

`opencode.go` has no patterns for:
- `Ready` (empty) — opencode sessions will never show as ready/idle
- `Error` — the actual opencode error format (`Error: Configuration is invalid at ...`) is not captured
- `Idle` — no readline prompt pattern  
- `Success` — no turn-complete indicator

During testing, opencode produced:
```
Error: Configuration is invalid at /home/tstapler/.config/opencode/agents/skills/re-tool-radare2.md
↳ Invalid input: expected record, received array tools
```

This is a real error format from opencode that could be used as a starting point for an error pattern, but only if it represents the general opencode error format.

**Required fix:** Research opencode TUI output for all state types (opencode.ai docs, GitHub issues/screenshots). Capture real terminal output. Add patterns and corresponding tests in `opencode_test.go`.

**Files affected:**
- `session/detection/binaries/opencode.go` — `OpencodeDetector.Patterns()`
- `session/detection/binaries/opencode_test.go` — add pattern matching tests

---

## P6 — LOW: All detection tests are structural-only — no regex accuracy tests

**Severity:** Pattern regressions won't be caught by the test suite.

Both `agy_test.go` and `opencode_test.go` only test:
- `Name()` returns the correct string
- Non-nil slice assertions (`len(p.Ready) == 0 → fatal`)
- `FilterContent()` is a no-op

No test exercises an actual regex match against a real or synthetic terminal output string. Claude's tests (`claude_test.go`) follow the same structural-only pattern, but claude has production usage that validates patterns through real sessions. Agy and opencode do not.

**Risk:** A typo in a new regex (e.g., an unescaped dot or broken `(?i)` flag position) will pass all tests but silently fail at runtime. R7 states "each new pattern or behavior must have a corresponding test."

**Required fix:** For every new pattern added to `agy.go` and `opencode.go`, add a matching-accuracy test in `agy_test.go` and `opencode_test.go`. Pattern tests should check both a positive-match case and a negative case (input that should NOT match). Example:

```go
func TestAgyDetector_Patterns_should_matchReadyOutput(t *testing.T) {
    d := NewAgyDetector()
    re := regexp.MustCompile(d.Patterns().Ready[0].Pattern)
    assert.True(t, re.MatchString("◇ Ready"), "should match ready state")
    assert.False(t, re.MatchString("Working..."), "should not match working state")
}
```

**Files affected:**
- `session/detection/binaries/agy_test.go`
- `session/detection/binaries/opencode_test.go`

---

## Summary Table

| ID | Severity | Blocks | Component | Description |
|----|----------|--------|-----------|-------------|
| P1 | BLOCKER | R1 | cli_ai_client.go | `agy --print` needs arg not stdin; CLIAIClient only supports stdin |
| P2 | MEDIUM | R2 | main.go, main_test.go | `installAgy()` patches both paths; double-fire + partial-install risk |
| P3 | MEDIUM | R6 | cli_ai_client.go | `opencode run` expects positional arg, not stdin |
| P4 | LOW | R3 | agy.go, agy_test.go | agy patterns are unverified Gemini clones; 5 state categories missing |
| P5 | LOW | R5 | opencode.go, opencode_test.go | opencode patterns missing Ready, Error, Idle, Success |
| P6 | LOW | R7 | *_test.go | No regex accuracy tests for any agy/opencode patterns |
