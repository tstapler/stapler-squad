# Implementation Plan: Antigravity CLI + Open Code Feature Parity

**Date:** 2026-07-01  
**Branch:** `stapler-squad-antigravity`  
**Requirements:** `project_plans/antigravity-opencode-parity/requirements.md`  
**Research:** `project_plans/antigravity-opencode-parity/research/`

---

## Summary

7 epics, 14 stories, 33 tasks.

| Epic | Title | Tasks | Blocker? |
|------|-------|-------|----------|
| E1 | Fix CLIAIClient positional-arg delivery | 3 | YES — blocks E2, opencode one-shot |
| E2 | Add agy to knownCLIAgents | 1 | Depends on E1 |
| E3 | Fix installAgy() single-path logic | 5 | No |
| E4 | Add agy detection patterns | 8 | No |
| E5 | Add opencode detection patterns | 3 | No |
| E6 | Test coverage for all new behavior | 13 | Depends on E1–E5 |
| E7 | Document opencode proxy approach | 1 | No |

---

## Architecture Decisions

### AD-1: PromptAsArg — positional arg vs. temp file

**Decision:** Add `PromptAsArg bool` to `CLIAgentSpec`. When `true`, `Complete()` appends the combined prompt string as the last positional argument to `Args()` instead of writing to stdin.

**Why not a temp file:** Both `agy --print` and `opencode run` expect the message inline as a positional arg, not as a `--file` path. Temp files would require agy/opencode support for file-based input, which is unconfirmed. Positional arg matches the documented CLI interface.

**Risk:** OS arg-length limit (~2 MB on Linux). For one-shot copilot queries (system prompt + brief user question), this limit is not a practical concern. If very large prompts are needed, revisit with `agy --print - < /tmp/prompt` (untested).

**Alternative considered:** A `PromptMode` enum (`stdin | arg | file`) — rejected as over-engineering for two tools.

### AD-2: opencode PromptAsArg — set true based on best evidence

**Decision:** Set `PromptAsArg: true` on the opencode spec. Research confirmed `opencode run [message..]` takes the message as positional args and the live test (`echo "hello" | opencode run`) produced a config error unrelated to stdin, making stdin behavior unconfirmed.

**If live validation reveals stdin works:** Revert `PromptAsArg: false` and add a comment citing the version tested. No other code changes required.

### AD-3: agy candidate order — antigravity-cli/ is primary

**Decision:** Probe `~/.gemini/antigravity-cli/hooks.json` first, fall back to `~/.gemini/config/hooks.json`. Create `antigravity-cli/` if neither exists.

**Rationale:** `~/.gemini/antigravity-cli/` is where agy stores all runtime state (settings.json, conversations/, brain/, mcp/, skills/). The current code comment says `config/` is "authoritative global path" but the live binary strings and directory layout show `antigravity-cli/` is where agy actually reads its hooks from.

**Current code is wrong:** `installAgy()` currently lists `config/` first in the comment but both paths are patched unconditionally. The research (features.md R2) confirms `antigravity-cli/` is primary.

### AD-4: Error and Success patterns — leave empty with TODO

**Decision:** Do not add guessed patterns for `agy` Error/Success or `opencode` Error/Success/Idle/Ready where no confirmed TUI strings exist.

**Rationale:** Silent false positives (reporting "Success" when the agent is actually stuck) are worse than unknown state (StatusUnknown). Patterns must be verified against real tmux captures before shipping. Leave empty slices with TODO comments citing the research gap.

**Exception:** `opencode_error_config` (`(?i)^Error:`) is added for the opencode error format observed in live testing (`Error: Configuration is invalid at...`). It is clearly identified as a tentative pattern.

---

## E1: Fix CLIAIClient to support positional-arg prompt delivery

**Context:** `agy --print "prompt"` is a string flag that requires the prompt as its value, NOT stdin (pitfall P1). `opencode run "prompt"` similarly expects a positional argument (pitfall P3). The current `Complete()` only pipes to stdin. This is a BLOCKER for agy and opencode one-shot functionality.

**Affected file:** `server/services/cli_ai_client.go`

### E1.S1: Extend CLIAgentSpec and Complete() with PromptAsArg mode

**E1.S1.T1: Add PromptAsArg bool field to CLIAgentSpec**

- File: `server/services/cli_ai_client.go`
- Change: Add `PromptAsArg bool` field to the `CLIAgentSpec` struct. Update the doc comment above `Args` to note: "When PromptAsArg is true, the combined prompt is appended as the last positional argument to Args() instead of being written to stdin."
- Existing specs (`claude`, `gemini`, `opencode`) are unaffected — `bool` zero value is `false`, preserving stdin behavior.
- AC: Field exists; `go build ./server/services/...` passes; no existing behavior changed.

**E1.S1.T2: Refactor Complete() to branch on PromptAsArg**

- File: `server/services/cli_ai_client.go`
- Change: In `Complete()`, after building `combined := systemPrompt + c.spec.PromptSeparator + userPrompt`, branch:
  - `if c.spec.PromptAsArg`: build `argv := append(c.spec.Args(), combined)`, create `executor.New(ctx, c.bin, argv, executor.WithTimeout(55*time.Second))` with no `WithStdin`.
  - else: existing path — `executor.New(ctx, c.bin, c.spec.Args(), executor.WithStdin(strings.NewReader(combined)), executor.WithTimeout(55*time.Second))`.
- AC: `Complete()` with `PromptAsArg=true` produces a command where the combined prompt is in `argv` and there is no stdin reader. With `PromptAsArg=false`, output is byte-for-byte equivalent to the current implementation.

**E1.S1.T3: Set PromptAsArg=true on the existing opencode spec**

- File: `server/services/cli_ai_client.go`
- Change: Update the opencode `CLIAgentSpec` entry:
  ```go
  {
      Name:            "opencode",
      Binary:          "opencode",
      Args:            func() []string { return []string{"run"} },
      PromptSeparator: "\n\n",
      PromptAsArg:     true,  // opencode run [message..] takes prompt as positional arg
  },
  ```
- AC: opencode's `Complete()` passes combined prompt as `opencode run "<combined-prompt>"`. Comment cites pitfall P3.

---

## E2: Add agy to knownCLIAgents

**Context:** `agy` is absent from `knownCLIAgents` in `server/services/cli_ai_client.go`. The `--print` flag confirmed from `agy --help`: takes prompt as string value (not stdin). Requires E1 complete (PromptAsArg must exist). Priority: below gemini, above opencode.

**Affected file:** `server/services/cli_ai_client.go`

### E2.S1: Insert agy CLIAgentSpec

**E2.S1.T1: Add agy spec to knownCLIAgents**

- File: `server/services/cli_ai_client.go`
- Change: Insert between the gemini and opencode entries:
  ```go
  {
      // Antigravity CLI (agy). --print takes the prompt as a positional string
      // argument, not from stdin. PromptAsArg=true routes through the arg path.
      // Priority: below gemini (no flag needed), above opencode.
      Name:            "agy",
      Binary:          "agy",
      Args:            func() []string { return []string{"--print"} },
      PromptSeparator: "\n\n---\n\n",
      PromptAsArg:     true,
  },
  ```
- AC: `NewBestAvailableAIClient(knownCLIAgents)` selects `agy` when `agy` is in PATH and neither `claude` nor `gemini` is available. `go build ./server/services/...` passes.

---

## E3: Fix installAgy() single-path logic

**Context:** `installAgy()` currently patches BOTH `~/.gemini/config/hooks.json` AND `~/.gemini/antigravity-cli/hooks.json` unconditionally (pitfall P2). This causes double-fire (two hook invocations per tool call) and partial-install risk on failure. Fix: mirror `installGemini()`'s first-found candidate loop. Primary path is `antigravity-cli/hooks.json` (where agy stores all runtime state); `config/hooks.json` is the fallback.

**Affected files:** `cmd/ssq-hooks/main.go`, `cmd/ssq-hooks/main_test.go`

### E3.S1: Refactor installAgy() to first-found path selection

**E3.S1.T1: Implement first-found candidate loop in installAgy()**

- File: `cmd/ssq-hooks/main.go`, function `installAgy()`, lines ~875–888
- Change: Replace the `hooksPaths` two-element loop (which patches both) with a `candidates` slice and first-found probe, matching `installGemini()` structure:
  ```go
  // 2. Discover agy hooks file — patch only the first found (mirrors installGemini).
  // ~/.gemini/antigravity-cli/ is agy's primary runtime state dir.
  // ~/.gemini/config/hooks.json is the fallback global config location.
  candidates := []string{
      filepath.Join(home, ".gemini", "antigravity-cli", "hooks.json"),
      filepath.Join(home, ".gemini", "config", "hooks.json"),
  }
  hooksPath := ""
  for _, c := range candidates {
      if _, err := os.Stat(c); err == nil {
          hooksPath = c
          break
      }
  }
  if hooksPath == "" {
      // Neither found: create the primary (antigravity-cli is where agy reads hooks).
      hooksPath = candidates[0]
  }
  // 3. Patch the selected file.
  if err := patchAntigravityHooks(hooksPath, destBin); err != nil {
      fmt.Fprintf(os.Stderr, "Error updating %s: %v\n", hooksPath, err)
      os.Exit(1)
  }
  fmt.Printf("Updated hook:     %s\n", hooksPath)
  fmt.Println("Done. Restart agy for the hook to take effect.")
  ```
- AC: Only ONE file is patched per run. If `antigravity-cli/hooks.json` exists, `config/hooks.json` is not touched. `go build ./cmd/ssq-hooks/` passes.

**E3.S1.T2: Verify TestInstallAgy_FreshInstall passes unchanged**

- File: `cmd/ssq-hooks/main_test.go`
- Action: Run `go test ./cmd/ssq-hooks/ -run TestInstallAgy_FreshInstall`. Since a fresh HOME has neither path, the new logic creates `antigravity-cli/hooks.json` (same as before). The test asserts exactly this path is created — no test change needed.
- AC: `TestInstallAgy_FreshInstall` passes. `TestInstallAgy_Idempotent` passes.

**E3.S1.T3: Add TestInstallAgy_PatchesOnlyFirstFound**

- File: `cmd/ssq-hooks/main_test.go`
- Change: Add test mirroring `TestInstallGemini_PatchesOnlyFirstFound`:
  ```go
  // installAgy_should_patchOnlyAntigravityCli_When_bothFilesExist
  func TestInstallAgy_PatchesOnlyFirstFound(t *testing.T) {
      home := t.TempDir()
      t.Setenv("HOME", home)
      agyDir := filepath.Join(home, ".gemini", "antigravity-cli")
      configDir := filepath.Join(home, ".gemini", "config")
      require.NoError(t, os.MkdirAll(agyDir, 0700))
      require.NoError(t, os.MkdirAll(configDir, 0700))
      // Both files exist
      require.NoError(t, os.WriteFile(filepath.Join(agyDir, "hooks.json"), []byte("{}"), 0644))
      require.NoError(t, os.WriteFile(filepath.Join(configDir, "hooks.json"), []byte(`{"other":"value"}`), 0644))
      installAgy()
      // Primary (antigravity-cli) patched
      raw1, _ := os.ReadFile(filepath.Join(agyDir, "hooks.json"))
      assert.Contains(t, string(raw1), "check --antigravity")
      // Fallback (config) untouched
      raw2, _ := os.ReadFile(filepath.Join(configDir, "hooks.json"))
      assert.JSONEq(t, `{"other":"value"}`, string(raw2))
  }
  ```
- AC: Test passes; verifies single-path contract.

**E3.S1.T4: Add TestInstallAgy_FallsBackToConfigJson**

- File: `cmd/ssq-hooks/main_test.go`
- Change: Add test:
  ```go
  // installAgy_should_fallBackToConfigJson_When_antigravityCliAbsent
  func TestInstallAgy_FallsBackToConfigJson(t *testing.T) {
      home := t.TempDir()
      t.Setenv("HOME", home)
      configDir := filepath.Join(home, ".gemini", "config")
      require.NoError(t, os.MkdirAll(configDir, 0700))
      require.NoError(t, os.WriteFile(filepath.Join(configDir, "hooks.json"), []byte("{}"), 0644))
      installAgy()
      // config/hooks.json patched
      raw, _ := os.ReadFile(filepath.Join(configDir, "hooks.json"))
      assert.Contains(t, string(raw), "check --antigravity")
      // antigravity-cli/hooks.json must NOT have been created
      _, err := os.Stat(filepath.Join(home, ".gemini", "antigravity-cli", "hooks.json"))
      assert.True(t, os.IsNotExist(err))
  }
  ```
- AC: Test passes; verifies fallback path selection when only `config/` exists.

**E3.S1.T4b: Add stale-entry cleanup — remove ssq-hooks from fallback file after patching primary**

- File: `cmd/ssq-hooks/main.go`, `installAgy()`, after the single-path patch succeeds
- Change: After patching the selected `hooksPath`, check if the OTHER candidate contains an ssq-hooks entry and remove it:
  ```go
  // Cleanup: if we patched the primary, remove any stale ssq-hooks entry in the fallback.
  // This handles users who ran an old version that patched both paths.
  otherPath := candidates[1]
  if hooksPath == candidates[0] {
      // We patched primary; clean fallback if present.
      _ = removeAntigravityHookEntry(otherPath, destBin)
  }
  ```
  Add `removeAntigravityHookEntry(path, binPath string) error` that reads the JSON, removes the `stapler-squad` key only if its command matches, and atomically rewrites. No-ops if file doesn't exist or entry is absent.
- AC: After running `ssq-hooks install agy` on a machine with both files pre-patched, only `antigravity-cli/hooks.json` retains the entry; `config/hooks.json` no longer has the `stapler-squad` key.

**E3.S1.T5: Add TestInstallAgy_CreatesAntigravityCliWhenNeitherExists**

- File: `cmd/ssq-hooks/main_test.go`
- Change: Add test (fresh-install variant, separate from `TestInstallAgy_FreshInstall` to be explicit):
  ```go
  // installAgy_should_createAntigravityCli_When_neitherPathExists
  func TestInstallAgy_CreatesAntigravityCliWhenNeitherExists(t *testing.T) {
      home := t.TempDir()
      t.Setenv("HOME", home)
      installAgy()
      agyHooks := filepath.Join(home, ".gemini", "antigravity-cli", "hooks.json")
      assert.FileExists(t, agyHooks)
      raw, _ := os.ReadFile(agyHooks)
      assert.Contains(t, string(raw), "check --antigravity")
      // config/hooks.json must NOT have been created
      _, err := os.Stat(filepath.Join(home, ".gemini", "config", "hooks.json"))
      assert.True(t, os.IsNotExist(err))
  }
  ```
- AC: Test passes; verifies fresh-install creates the primary path only.

---

## E4: Add agy detection patterns

**Context:** `AgyDetector.Patterns()` has `Idle: []`, `Active: []`, `Error: []`, `Success: []` (pitfall P4, requirement R3). Since agy shares the Gemini `jetski` TUI codebase, Idle and Active patterns confirmed from Gemini fixtures apply. Error and Success have no confirmed strings — leave empty with TODO.

**Affected files:** `session/detection/binaries/agy.go`, `session/detection/testdata/agy_idle.txt`, `session/detection/testdata/agy_active.txt`

### E4.S1: Add Idle patterns

**E4.S1.T1: Add agy_idle_readline pattern**

- File: `session/detection/binaries/agy.go`
- Change: Replace `Idle: []dtypes.StatusPattern{}` with:
  ```go
  Idle: []dtypes.StatusPattern{
      {
          Name:        "agy_idle_readline",
          Pattern:     `> ▌`,
          Description: "Agy readline input cursor on empty line (shared with Gemini idle; confirmed from gemini_idle.txt fixture)",
          Priority:    5,
      },
      {
          Name:        "agy_idle_insert",
          Pattern:     `\[INSERT\]`,
          Description: "Agy INSERT mode indicator in status bar (shared with Gemini idle; confirmed from gemini_idle.txt fixture)",
          Priority:    6,
      },
  },
  ```
- AC: Patterns compile; regex compiles with `regexp.MustCompile`.

### E4.S2: Add Active patterns

**E4.S2.T1: Add agy_active_running and agy_active_thinking**

- File: `session/detection/binaries/agy.go`
- Change: Replace `Active: []dtypes.StatusPattern{}` with:
  ```go
  Active: []dtypes.StatusPattern{
      {
          Name:        "agy_active_running",
          Pattern:     `= Running Agent\.\.\.`,
          Description: "Agy running agent indicator (shared with Gemini active; confirmed from gemini_active.txt fixture)",
          Priority:    11,
      },
      {
          Name:        "agy_active_thinking",
          Pattern:     `Thinking\.\.\. \(esc to cancel`,
          Description: "Agy thinking spinner with cancel hint (shared with Gemini active; confirmed from gemini_active.txt fixture)",
          Priority:    12,
      },
  },
  ```
- AC: Patterns compile.

### E4.S3: Add TODO comments for Error and Success

**E4.S3.T1: Update Error and Success empty slices with TODO comments**

- File: `session/detection/binaries/agy.go`
- Change: Replace bare `Error: []dtypes.StatusPattern{}` and `Success: []dtypes.StatusPattern{}` with annotated versions:
  ```go
  Error: []dtypes.StatusPattern{
      // TODO: Add error patterns once real agy terminal output is captured.
      // agy shares Gemini jetski TUI — check what Gemini shows on API errors.
      // Capture via: tmux capture-pane -p on a running agy session hitting a rate limit.
      // See: project_plans/antigravity-opencode-parity/research/architecture.md
  },
  Success: []dtypes.StatusPattern{
      // TODO: Add success/completion patterns once real agy terminal output is captured.
      // After a task completes, agy likely returns to Ready state (covered by agy_ready).
      // No distinct "done" indicator was found in architecture research.
  },
  ```
- AC: File compiles unchanged in behavior; comments explain the gap for future contributors.

### E4.S4: Create agy testdata fixture stubs

**E4.S4.T1: Create session/detection/testdata/agy_idle.txt**

- File: `session/detection/testdata/agy_idle.txt`
- Change: Create file with representative idle state content sourced from `gemini_idle.txt` patterns. Add a header comment:
  ```
  # agy idle state — STUB (mirrored from gemini_idle.txt; requires real agy capture)
  # Capture: tmux capture-pane -p on an agy session at the input prompt
  # agy shares Gemini jetski TUI codebase (confirmed from binary symbols)
  > ▌
  [INSERT]
  ```
- AC: File exists and contains the expected `> ▌` and `[INSERT]` strings.

**E4.S4.T2: Create session/detection/testdata/agy_active.txt**

- File: `session/detection/testdata/agy_active.txt`
- Change: Create file with active state content from Gemini fixture patterns:
  ```
  # agy active state — STUB (mirrored from gemini_active.txt; requires real agy capture)
  # Capture: tmux capture-pane -p on an agy session while it is processing
  = Running Agent... (ctrl+o to expand)
  ⌃" Thinking... (esc to cancel, 10s)
  ```
- AC: File exists and contains the expected strings.

---

## E5: Add opencode detection patterns

**Context:** `OpencodeDetector.Patterns()` is missing `Active`, `Error`, `Idle`, and `Success` (pitfall P5, requirement R5). The braille spinner `⠙⠹⠸⠼⠴⠦⠧⠇⠏⠋` is a confirmed Active/Processing indicator. A tentative error pattern is available from live testing. Ready/Idle and Success have no confirmed strings.

**Affected file:** `session/detection/binaries/opencode.go`

### E5.S1: Add braille spinner to Active

**E5.S1.T1: Add opencode_braille_spinner to Active**

- File: `session/detection/binaries/opencode.go`
- Change: Replace `Active: []dtypes.StatusPattern{}` with:
  ```go
  Active: []dtypes.StatusPattern{
      {
          Name:        "opencode_braille_spinner",
          Pattern:     `[⠙⠹⠸⠼⠴⠦⠧⠇⠏⠋]`,
          Description: "OpenCode braille spinner characters during LLM generation (separate from tool execution arrows)",
          Priority:    9,
      },
  },
  ```
- AC: Pattern compiles as valid regex. Note: `[...]` is a character class — the braille chars are multi-byte but valid inside a Go regex character class.

### E5.S2: Add tentative error pattern

**E5.S2.T1: Add opencode_error_config to Error**

- File: `session/detection/binaries/opencode.go`
- Change: Replace `Error: []dtypes.StatusPattern{}` with:
  ```go
  Error: []dtypes.StatusPattern{
      {
          Name:        "opencode_error_prefix",
          // Observed in live testing: "Error: Configuration is invalid at /path"
          // This is a tentative pattern — may match other tool errors in the TUI.
          // TODO: Validate against more opencode error types before promoting.
          Pattern:     `(?m)^Error:`,
          Description: "OpenCode error message line prefix (observed in live testing with config errors)",
          Priority:    5,
      },
  },
  ```
- AC: Pattern compiles. `(?m)^Error:` anchors to start of any line in multiline input.

### E5.S3: Add TODO comments for Ready/Idle and Success

**E5.S3.T1: Annotate empty Ready, Idle, and Success slices**

- File: `session/detection/binaries/opencode.go`
- Change: Replace bare empty slices for `Ready`, `Idle`, `Success` with annotated versions:
  ```go
  Ready: []dtypes.StatusPattern{
      // TODO: No distinctive idle/ready string was found in opencode TUI source.
      // The idle state appears as just the bordered input box with no spinner.
      // Absence-of-other-patterns results in StatusUnknown, which is acceptable.
      // See: project_plans/antigravity-opencode-parity/research/architecture.md
  },
  Idle: []dtypes.StatusPattern{
      // TODO: Same gap as Ready — no distinctive idle string found.
  },
  Success: []dtypes.StatusPattern{
      // TODO: No explicit completion indicator found in opencode TUI source.
      // After a task completes, opencode returns to idle input state.
      // Consider checking for token/cost summary text if opencode adds one.
  },
  ```
- AC: File compiles unchanged in behavior.

---

## E6: Test coverage for all new behavior

**Context:** Requirement R7 and pitfall P6: no regex accuracy tests exist for agy or opencode patterns. All new patterns must have positive and negative match tests. `installAgy()` path tests are in E3.

**Affected files:** `server/services/cli_ai_client_test.go` (create if absent), `session/detection/binaries/agy_test.go`, `session/detection/binaries/opencode_test.go`

### E6.S1: CLIAIClient unit tests

**E6.S1.T1: Test Complete() PromptAsArg=true appends prompt to argv**

- File: `server/services/cli_ai_client_test.go` (create if absent; package `services`)
- Change: Use a fake binary (e.g., `/bin/echo` or a test script) that prints its `argv[1..]` to stdout. Create a `CLIAgentSpec` with `Binary: "echo"` (or a test binary), `Args: func() []string { return []string{"--flag"} }`, `PromptAsArg: true`. Call `Complete(ctx, "sys", "user")`. Assert the output contains the combined prompt string `"sys\n\n---\n\nuser"`.
- AC: Test passes; confirms prompt appears in argv when `PromptAsArg=true`.

**E6.S1.T2: Structural test for agy spec in knownCLIAgents**

- File: `server/services/cli_ai_client_test.go`
- Change: Add a table-driven structural test that iterates `knownCLIAgents` and for the entry with `Name=="agy"` asserts: `Binary=="agy"`, `PromptAsArg==true`, `Args()[0]=="--print"`. No PATH resolution needed.
- AC: Test passes; fails fast if agy spec is removed or misconfigured.

**E6.S1.T3: Structural test for opencode spec PromptAsArg**

- File: `server/services/cli_ai_client_test.go`
- Change: Same pattern as T2 but for `Name=="opencode"`: assert `PromptAsArg==true`, `Args()[0]=="run"`.
- AC: Test passes.

### E6.S2: AgyDetector pattern accuracy tests

Pattern: `re := regexp.MustCompile(pattern); assert.True(t, re.MatchString(positiveCase)); assert.False(t, re.MatchString(negativeCase))`

File for all: `session/detection/binaries/agy_test.go`

**E6.S2.T1: agy_ready — positive "◇ Ready", negative "Working..."**

**E6.S2.T2: agy_working — positive "✦ Working", negative "◇ Ready"**

**E6.S2.T3: agy_permission — positive "Yes, allow once", negative "yes deny"**

**E6.S2.T4: agy_allow_execution — positive "Allow execution of:", negative "allow execution"** (case-insensitive check)

**E6.S2.T5: agy_idle_readline — positive "> ▌", negative "> text here"**

**E6.S2.T6: agy_idle_insert — positive "[INSERT]", negative "[NORMAL]"**

**E6.S2.T7: agy_active_running — positive "= Running Agent...", negative "Running"**

**E6.S2.T8: agy_active_thinking — positive "Thinking... (esc to cancel", negative "Thinking... (press enter)"**

For each (T1–T8):
- AC: Both `MatchString` assertions pass. Regex compiles without panic.

### E6.S3: OpencodeDetector pattern accuracy tests

File for all: `session/detection/binaries/opencode_test.go`

**E6.S3.T1: opencode_arrow_action — positive "→ Read foo.go", negative "Read foo.go" (no arrow)**

**E6.S3.T2: opencode_permission — positive "[ Allow (a) ]", negative "Allow (a)"** (no brackets)

**E6.S3.T3: opencode_bar_prefixed_options — positive "┃  4. Icons:", negative "4. Icons:"** (no bar)

**E6.S3.T4: opencode_permission_buttons — positive "Allow once   Allow always", negative "allow once"** (case-sensitive check per `(?i)`)

**E6.S3.T5: opencode_braille_spinner — positive "⠙ Thinking...", negative "Thinking..."** (spinner char required)

**E6.S3.T6: opencode_error_prefix — positive "Error: bad config", negative "error: not at start of line"** (multiline anchor)

For each (T1–T6):
- AC: Both `MatchString` assertions pass. Regex compiles without panic.

---

## E7: Document opencode proxy approach

**Context (requirement R4):** Research confirmed opencode v1.4.0 has no PreToolUse hook system — only `file_edited` and `session_completed` hooks, which are not permission gates. The proxy wrapper in `installOpenCode()` is the correct approach. This decision must be documented so future contributors know when to revisit it.

**Affected file:** `cmd/ssq-hooks/main.go`

### E7.S1: Add explanatory comment to installOpenCode()

**E7.S1.T1: Add architectural rationale comment above installOpenCode()**

- File: `cmd/ssq-hooks/main.go`, above `func installOpenCode()`
- Change: Replace or augment the existing comment:
  ```go
  // installOpenCode creates a shell wrapper at ~/.local/bin/open-code that routes
  // all opencode invocations through ssq-hooks proxy before execution.
  //
  // WHY A PROXY WRAPPER (not a native hook):
  // opencode v1.4.0 supports only two hook types in its config:
  //   - hook.file_edited: fires after a file is written (not a permission gate)
  //   - hook.session_completed: fires when the session ends (not a permission gate)
  // There is no PreToolUse / BeforeTool / pre-execution hook in opencode's hook system.
  // Source: ~/.config/opencode/node_modules/@opencode-ai/sdk/dist/gen/types.gen.d.ts
  //
  // The proxy intercepts the `open-code` binary name in PATH. The real opencode binary
  // is named `opencode` (no hyphen); users who alias `open-code` → this wrapper get
  // ssq-hooks intercept for every invocation.
  //
  // Revisit: if opencode adds a native PreToolUse hook, replace this proxy with
  // a patchOpenCodeConfig() call similar to patchBeforeToolHook().
  func installOpenCode() {
  ```
- AC: Comment is present; code is unchanged. `go build ./cmd/ssq-hooks/` passes.

---

## Execution Order

Dependencies between epics:

```
E1 → E2          (PromptAsArg struct must exist before agy spec can use it)
E1 → E6.S1       (CLIAIClient tests depend on PromptAsArg implementation)
E3 → E6.S2.T*    (installAgy tests in E3 cover path logic; E6.S2 adds pattern accuracy)
E4 → E6.S2       (patterns must exist before accuracy tests)
E5 → E6.S3       (patterns must exist before accuracy tests)
```

Recommended implementation order for a single developer:
1. E1 (BLOCKER — unlocks E2 and opencode one-shot)
2. E2 (1 task, trivial once E1 done)
3. E3 (independent of E1/E2; can be done in parallel)
4. E4 + E5 (independent; can be done in parallel)
5. E6 (write tests after code is in place)
6. E7 (cosmetic; can be done any time)

---

## Acceptance Criteria Summary (maps to requirements)

| Req | Criterion | Epic |
|-----|-----------|------|
| R1 | `agy --print "prompt"` is invoked correctly from CLIAIClient | E1, E2 |
| R2 | `ssq-hooks install agy` patches exactly ONE hooks file | E3 |
| R3 | `AgyDetector` has Idle and Active patterns; Error/Success have TODO comments | E4 |
| R4 | `installOpenCode()` comment explains why proxy is correct for v1.4.0 | E7 |
| R5 | `OpencodeDetector` has braille spinner Active pattern; Ready/Idle/Success have TODO | E5 |
| R6 | opencode spec uses `PromptAsArg: true` | E1.S1.T3 |
| R7 | All new patterns have positive+negative regex tests; installAgy has 3 new path tests | E3, E6 |
| — | `make quick-check` passes | All |
