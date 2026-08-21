# Adversarial Review: Antigravity CLI + Open Code Feature Parity

**Reviewer:** Automated adversarial review  
**Date:** 2026-07-01  
**Verdict:** CONCERNS — proceed, but three issues need explicit acknowledgment and mitigations before shipping

---

## Verdict: CONCERNS

The plan is structurally sound. The `PromptAsArg bool` design is correct given that `executor.New()` calls `safeexec.CommandContext(ctx, c.name, c.args...)` directly — no shell is involved, so no shell escaping is needed, newlines are preserved, and the OS arg-length limit (~2 MB) is not a practical concern for copilot-sized prompts. The installAgy() fix correctly mirrors installGemini(). The empty-with-TODO pattern for unverified detection strings is the right call over false-positive patterns.

However, three issues are significant enough to require explicit acknowledgment or mitigation:

---

## Issue 1 (HIGH): `agy --print "prompt"` stdout output format is never verified

**What the research shows:** P1 confirmed that `echo "hello" | agy --print` fails with `flag needs an argument: -print`, proving the flag takes a string value. Requirements say `--print` is "confirmed from `agy --help`" for one-shot mode.

**What's missing:** No test ever ran `agy --print "actual prompt text"` and verified the output comes to stdout as clean text. The plan jumps from "the flag exists and takes an argument" to "it works like `claude --print`." These are different claims.

**Why this matters in production:** If `agy --print "prompt"` writes its response to a PTY/TTY rather than stdout (common in TUI apps that detect terminal presence), `executor.New()` with no PTY will either:
- Return empty stdout with no error (output went to TTY that nobody read)
- Hang waiting for a TTY that the subprocess expects to exist
- Fail with a non-zero exit code ("not a terminal")

The 55-second timeout in `Complete()` means a hang would block the copilot caller for nearly a minute before returning an error. `CLIAIClient` would be selected (binary exists in PATH) but silently broken.

**Mitigation required before shipping E2:**  
Run `agy --print "hello world" 2>&1 | cat` in a non-TTY context (or `agy --print "hello world" > /tmp/agy-out.txt && cat /tmp/agy-out.txt`) and confirm the response text appears in the file, not on screen. If agy detects TTY absence and falls back to stdout, the plan works. If it refuses to run without a TTY, `PromptAsArg` alone won't save us — a PTY wrapper or a different agy invocation will be needed. Add a comment in the spec citing which agy version this was tested against.

---

## Issue 2 (MEDIUM): `opencode run "prompt"` has never succeeded end-to-end — the live test was blocked by a pre-existing config error

**The plan's claim (AD-2):** "Research confirmed `opencode run [message..]` takes the message as positional args and the live test (`echo "hello" | opencode run`) produced a config error unrelated to stdin."

**The actual problem:** The config error (`Error: Configuration is invalid at /home/tstapler/.config/opencode/agents/skills/re-tool-radare2.md`) appeared when running `opencode run` with NO message at all (stdin piped from echo). This error fires at startup, before opencode reads any message from any source. It will also fire for `opencode run "prompt"`. The live test therefore told us nothing about positional arg behavior — both invocations fail identically on this machine.

**Concretely:** `PromptAsArg: true` is the right setting based on the `--help` text. But there is zero live evidence that `opencode run "a prompt"` successfully returns a response on a properly-configured machine. The plan acknowledges this as "if live validation reveals stdin works, revert PromptAsArg," but the reverse is also untested: we don't know if positional arg works either.

**Mitigation required before shipping E1.S1.T3:**  
Fix the opencode config error (likely by removing or correcting the invalid agent skill file at `~/.config/opencode/agents/skills/re-tool-radare2.md`) and then run `opencode run "say hello"` to confirm it returns a non-interactive response. Update E6.S1.T1 to use `opencode` as the actual test binary once config is fixed, not just `/bin/echo`. Add a comment in the spec citing the opencode version and output format observed.

If the config error cannot be fixed quickly, gate E1.S1.T3 on a TODO comment: "Validated with opencode vX.Y.Z once config issue at ~/.config/opencode/agents/skills/ is resolved."

---

## Issue 3 (MEDIUM): Existing installations with both paths patched will continue to double-fire after the fix — no migration path

**The bug:** The current `installAgy()` patches BOTH `~/.gemini/config/hooks.json` AND `~/.gemini/antigravity-cli/hooks.json`. If both files exist on a user's machine and agy reads both, the hook fires twice per tool call.

**The plan's fix:** Changes the installer to patch only the first-found path going forward.

**The migration gap:** Any user who ran the old `installAgy()` already has `ssq-hooks check --antigravity` in both files. The new installer will not touch `config/hooks.json` on re-run (because `antigravity-cli/hooks.json` exists and is found first). The double-fire continues for all existing installations until the user manually removes the hook from `config/hooks.json`.

The plan adds `TestInstallAgy_PatchesOnlyFirstFound` which correctly tests new behavior, and `TestInstallAgy_FallsBackToConfigJson` — but neither test covers the migration case: "user has BOTH files patched from old installer; re-running new installer should NOT leave double-hook state."

**Mitigation options (pick one):**

Option A — Active cleanup: When the new installer patches `antigravity-cli/hooks.json`, also check if `config/hooks.json` has a `stapler-squad` entry and remove it (leaving other content intact). This is the most user-friendly fix.

Option B — Detection and warn: If `config/hooks.json` has a `stapler-squad` entry AND `antigravity-cli/hooks.json` is being selected as primary, print a warning: "Warning: ~/.gemini/config/hooks.json also has a stapler-squad hook entry. Remove it to prevent double-fire: `ssq-hooks install agy --cleanup` or edit the file manually." Then add `--cleanup` as an optional flag that performs Option A.

Option C — Document it: Add a note in the `installAgy()` output when double-patching is detected, so users know to clean up.

The plan currently does none of these. Option B is lowest implementation risk and should be added to E3.

---

## Secondary Findings (do not block)

### `PromptAsArg bool` design is correct — no changes needed

`executor.New()` calls `safeexec.CommandContext(ctx, c.name, c.args...)` which routes through `exec.CommandContext`. The args are passed directly to the OS kernel — no shell, no quoting, no expansion. The combined prompt string (with embedded newlines, quotes, special chars) is passed as a single argv element verbatim. The `append(c.spec.Args(), combined)` construction is safe because each `Args()` call returns a new slice (no aliasing). Shell escaping is not needed and should NOT be added — it would corrupt the prompt.

### `handleCheck()` requires no changes for agy sessions

`handleCheck()` already handles `--antigravity` mode (lines 65–106): parses the Gemini-format payload via `parseGeminiPayload()`, falls back to `os.Getwd()` for missing Cwd, maps to workspace database via `getDBPathForCwd()`, and writes the decision via `writeAntigravityHookDecision()`. Agy sessions spawned from stapler-squad worktrees will have the correct Cwd in the hook payload. No changes needed.

### installAgy() fresh-install path is safe

`patchAntigravityHooks()` calls `os.MkdirAll(filepath.Dir(hooksPath), 0700)` before writing. When `~/.gemini/antigravity-cli/` doesn't exist yet (new install), the directory is created automatically. The E3.S1.T5 test explicitly covers this. No risk.

### Gemini pattern copies for agy are appropriately guarded

Adding `> ▌`, `[INSERT]`, `= Running Agent...`, and `Thinking...` patterns with explicit "STUB — requires real agy capture" comments is correct. The risk of false positives (patterns match in unexpected states) is lower than the risk of leaving all these slices empty (zero detection coverage). The TODO comments correctly direct future contributors.

### opencode_error_prefix pattern `(?m)^Error:` — false positive risk acknowledged

The pattern is intentionally tentative and marked with a TODO. The risk is real (any tool error line starting with "Error:" could trigger the pattern), but accepting `StatusUnknown` for all opencode error states is worse than accepting an occasional false-positive Error detection. The plan's call is defensible; just don't promote this pattern out of "tentative" status without capturing several real error types.

---

## Checklist for Proceeding

- [ ] Run `agy --print "hello world" > /tmp/out.txt` in a non-TTY context; confirm response text in file (Issue 1)
- [ ] Fix opencode config error; run `opencode run "say hello"`; confirm one-shot response (Issue 2)
- [ ] Add migration handling or warning for existing dual-patched installations (Issue 3)
- [ ] After the above: `make quick-check` passes
