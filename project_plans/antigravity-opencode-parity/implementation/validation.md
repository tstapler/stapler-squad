# Validation Plan: Antigravity CLI + Open Code Feature Parity

**Date:** 2026-07-01
**Requirements:** `project_plans/antigravity-opencode-parity/requirements.md`
**Plan:** `project_plans/antigravity-opencode-parity/implementation/plan.md`
**Adversarial Review:** `project_plans/antigravity-opencode-parity/implementation/adversarial-review.md`
**Requirements Coverage:** 7/7

---

## Summary

| Test Type | Count |
|-----------|-------|
| Unit tests | 18 |
| Integration tests | 8 |
| Snapshot / structural tests | 3 |
| **Total** | **29** |

Requirements coverage: **7/7** (R1–R7 each have at least one test case).

---

## Requirement-to-Test Traceability

| Req | Description | Tests |
|-----|-------------|-------|
| R1 | Agy one-shot AI client (`agy --print` in `knownCLIAgents`) | UT-01, UT-02, UT-03 |
| R2 | Agy hook install single-path logic | IT-01, IT-02, IT-03, IT-04, IT-05, IT-06 |
| R3 | Agy detection patterns (Idle, Active; Error/Success TODO) | UT-05–UT-12, ST-01, ST-02 |
| R4 | Open Code proxy approach documented (no native hooks) | ST-03, IT-07 |
| R5 | Open Code detection patterns (braille spinner, error prefix) | UT-13–UT-18 |
| R6 | Open Code one-shot `opencode run` with `PromptAsArg: true` | UT-01, UT-04 |
| R7 | Test coverage parity: all new patterns have positive+negative tests | All UT-01–UT-18 |

---

## Unit Tests

Unit tests live in two files:
- `server/services/cli_ai_client_test.go` (package `services`)
- `session/detection/binaries/agy_test.go` and `opencode_test.go` (package `detection`)

---

### CLIAIClient: PromptAsArg Dispatch (covers R1, R6, R7)

**UT-01: `Complete()` with `PromptAsArg=true` routes combined prompt to argv, not stdin**

- File: `server/services/cli_ai_client_test.go`
- Setup: Create a `CLIAgentSpec` with `Binary: "echo"`, `Args: func() []string { return []string{"--flag"} }`, `PromptSeparator: "\n\n---\n\n"`, `PromptAsArg: true`. Call `Complete(ctx, "system", "user")`.
- Assert: Output equals `--flag system\n\n---\n\nuser` (echo prints its argv). No stdin write path invoked.
- Requirements: R1, R6, R7
- Plan ref: E6.S1.T1

**UT-02: `Complete()` with `PromptAsArg=false` sends combined prompt to stdin (regression guard)**

- File: `server/services/cli_ai_client_test.go`
- Setup: Create a spec with `Binary: "cat"`, `Args: func() []string { return []string{} }`, `PromptAsArg: false`. Call `Complete(ctx, "sys", "user")`.
- Assert: Output is `sys\n\nuser` (cat echoes stdin). No argv change.
- Requirements: R1, R6, R7
- Plan ref: E6.S1.T1 (inverse)

**UT-03: agy spec has correct fields in `knownCLIAgents`**

- File: `server/services/cli_ai_client_test.go`
- Setup: Iterate `knownCLIAgents`; locate entry where `Name == "agy"`.
- Assert: `Binary == "agy"`, `PromptAsArg == true`, `Args()[0] == "--print"`, entry is positioned after `"gemini"` and before `"opencode"`.
- Requirements: R1, R7
- Plan ref: E6.S1.T2

**UT-04: opencode spec has `PromptAsArg: true` and `Args()[0] == "run"`**

- File: `server/services/cli_ai_client_test.go`
- Setup: Iterate `knownCLIAgents`; locate entry where `Name == "opencode"`.
- Assert: `PromptAsArg == true`, `Args()[0] == "run"`.
- Requirements: R6, R7
- Plan ref: E6.S1.T3

---

### AgyDetector Pattern Accuracy (covers R3, R7)

All tests follow the pattern:
```go
re := regexp.MustCompile(pattern)
assert.True(t, re.MatchString(positiveCase), "should match")
assert.False(t, re.MatchString(negativeCase), "should not match")
```
All live in `session/detection/binaries/agy_test.go`.

**UT-05: `agy_ready` pattern**

- Positive: `"◇ Ready"` — matches
- Negative: `"Working..."` — does not match
- Requirements: R3, R7
- Plan ref: E6.S2.T1

**UT-06: `agy_working` pattern**

- Positive: `"✦ Working"` — matches
- Negative: `"◇ Ready"` — does not match
- Requirements: R3, R7
- Plan ref: E6.S2.T2

**UT-07: `agy_permission` pattern**

- Positive: `"Yes, allow once"` — matches
- Negative: `"yes deny"` (wrong keyword) — does not match
- Requirements: R3, R7
- Plan ref: E6.S2.T3

**UT-08: `agy_allow_execution` pattern**

- Positive: `"Allow execution of:"` — matches
- Negative: `"allow execution of:"` (if pattern is case-sensitive; verify against implementation) — test confirms case-sensitivity intent
- Requirements: R3, R7
- Plan ref: E6.S2.T4

**UT-09: `agy_idle_readline` pattern**

- Positive: `"> ▌"` — matches
- Negative: `"> text here"` (block cursor absent) — does not match
- Requirements: R3, R7
- Plan ref: E6.S2.T5

**UT-10: `agy_idle_insert` pattern**

- Positive: `"[INSERT]"` — matches
- Negative: `"[NORMAL]"` — does not match
- Requirements: R3, R7
- Plan ref: E6.S2.T6

**UT-11: `agy_active_running` pattern**

- Positive: `"= Running Agent..."` — matches
- Negative: `"Running"` (no leading `=` or trailing `...`) — does not match
- Requirements: R3, R7
- Plan ref: E6.S2.T7

**UT-12: `agy_active_thinking` pattern**

- Positive: `"Thinking... (esc to cancel"` — matches
- Negative: `"Thinking... (press enter)"` (wrong cancel hint) — does not match
- Requirements: R3, R7
- Plan ref: E6.S2.T8

---

### OpencodeDetector Pattern Accuracy (covers R5, R7)

All live in `session/detection/binaries/opencode_test.go`.

**UT-13: `opencode_arrow_action` pattern**

- Positive: `"→ Read foo.go"` — matches
- Negative: `"Read foo.go"` (no arrow) — does not match
- Requirements: R5, R7
- Plan ref: E6.S3.T1

**UT-14: `opencode_permission` pattern**

- Positive: `"[ Allow (a) ]"` — matches
- Negative: `"Allow (a)"` (no brackets) — does not match
- Requirements: R5, R7
- Plan ref: E6.S3.T2

**UT-15: `opencode_bar_prefixed_options` pattern**

- Positive: `"┃  4. Icons:"` — matches
- Negative: `"4. Icons:"` (no bar) — does not match
- Requirements: R5, R7
- Plan ref: E6.S3.T3

**UT-16: `opencode_permission_buttons` pattern**

- Positive: `"Allow once   Allow always"` — matches (`(?i)` makes this case-insensitive)
- Negative: `"allow once"` (single option, confirm pattern requires both tokens) — does not match
- Requirements: R5, R7
- Plan ref: E6.S3.T4

**UT-17: `opencode_braille_spinner` pattern**

- Positive: `"⠙ Thinking..."` (contains a braille character) — matches
- Negative: `"Thinking..."` (no braille character) — does not match
- Requirements: R5, R7
- Plan ref: E6.S3.T5

**UT-18: `opencode_error_prefix` pattern (`(?m)^Error:`)**

- Positive: `"Error: bad config"` — matches (at start of string)
- Negative: `"  error: not at start of line"` (indented, or mid-line) — does not match (multiline anchor `^` requires line-start)
- Also negative: `"myError: something"` — does not match (`^` anchors to line start)
- Requirements: R5, R7
- Plan ref: E6.S3.T6

---

## Integration Tests

Integration tests exercise file system state, process exit codes, and multi-step flows. All `installAgy()` tests use `t.TempDir()` + `t.Setenv("HOME", ...)` for isolation.

**IT-01: `TestInstallAgy_FreshInstall` (existing — baseline)**

- Setup: Clean `HOME` (no `.gemini/` dir). Run `installAgy()`.
- Assert: `~/.gemini/antigravity-cli/hooks.json` is created and contains `"check --antigravity"`. `~/.gemini/config/hooks.json` does NOT exist.
- Requirements: R2
- Plan ref: E3.S1.T2 (existing test, must continue to pass after refactor)

**IT-02: `TestInstallAgy_Idempotent` (existing — baseline)**

- Setup: Run `installAgy()` twice on the same `HOME`.
- Assert: Second run produces identical file content (no duplicate entries). Exit zero.
- Requirements: R2
- Plan ref: E3.S1.T2 (existing test)

**IT-03: `TestInstallAgy_PatchesOnlyFirstFound`**

- Setup: Create both `~/.gemini/antigravity-cli/hooks.json` (`{}`) and `~/.gemini/config/hooks.json` (`{"other":"value"}`). Run `installAgy()`.
- Assert: `antigravity-cli/hooks.json` contains `"check --antigravity"`. `config/hooks.json` equals `{"other":"value"}` exactly (untouched).
- Requirements: R2
- Plan ref: E3.S1.T3

**IT-04: `TestInstallAgy_FallsBackToConfigJson`**

- Setup: Create only `~/.gemini/config/hooks.json` (`{}`). Run `installAgy()`.
- Assert: `config/hooks.json` contains `"check --antigravity"`. `~/.gemini/antigravity-cli/hooks.json` does NOT exist (not created as a side effect).
- Requirements: R2
- Plan ref: E3.S1.T4

**IT-05: `TestInstallAgy_CreatesAntigravityCliWhenNeitherExists`**

- Setup: Clean `HOME`. Run `installAgy()`.
- Assert: `~/.gemini/antigravity-cli/hooks.json` created, contains `"check --antigravity"`. `config/hooks.json` not created.
- Requirements: R2
- Plan ref: E3.S1.T5

**IT-06: `TestInstallAgy_StaleEntryCleanup`**

- Setup: Both paths pre-patched from old installer (both contain `"check --antigravity"` from a previous run). Run `installAgy()` with the new code.
- Assert: `antigravity-cli/hooks.json` retains the entry (primary). `config/hooks.json` no longer contains `stapler-squad` key (stale entry removed). Other content in `config/hooks.json` preserved.
- Requirements: R2 (addresses adversarial Issue 3 — migration path)
- Plan ref: E3.S1.T4b

**IT-07: `go build ./...` passes after all changes**

- Command: `go build ./...` from repo root
- Assert: Exit 0, no compile errors. This validates R4 indirectly — the proxy comment block is syntactically valid Go and the function compiles.
- Requirements: R4 (structural), R1, R2, R3, R5, R6
- Plan ref: AC on every Epic

**IT-08: `make quick-check` passes**

- Command: `make quick-check` (build + test + lint)
- Assert: Exit 0. All new tests pass. `golangci-lint` emits no new errors. This is the final gate for all epics.
- Requirements: R1, R2, R3, R4, R5, R6, R7
- Plan ref: "make quick-check passes" row in plan AC summary

---

## Snapshot / Structural Tests

These verify non-behavioral artifacts: fixture files, doc comments, and TODO annotations. Run as part of the unit test suite or as file-existence assertions in a test helper.

**ST-01: `session/detection/testdata/agy_idle.txt` exists and matches expected content**

- Assert: File exists. `strings.Contains(content, "> ▌")` is true. `strings.Contains(content, "[INSERT]")` is true.
- Requirements: R3
- Plan ref: E4.S4.T1

**ST-02: `session/detection/testdata/agy_active.txt` exists and matches expected content**

- Assert: File exists. Content contains `"= Running Agent..."`. Content contains `"Thinking..."`.
- Requirements: R3
- Plan ref: E4.S4.T2

**ST-03: `installOpenCode()` in `cmd/ssq-hooks/main.go` has proxy rationale comment**

- Method: Read `cmd/ssq-hooks/main.go`; locate `func installOpenCode()`. Assert that the preceding block comment contains the string `"file_edited"` and `"session_completed"` (naming the two hook types confirmed by research), and contains `"no PreToolUse"` or equivalent phrasing.
- This can be verified by `grep` in CI or as part of code review; it is listed here as a structural requirement so reviewers know to look for it.
- Requirements: R4
- Plan ref: E7.S1.T1

---

## Adversarial Issues → Test Impact

The adversarial review rated the plan **CONCERNS** (not FAIL). Three open issues affect the test plan:

### Issue 1 (HIGH): `agy --print` stdout behavior unverified

**Impact on tests:** UT-01 uses `/bin/echo` as a proxy binary to test the `PromptAsArg` dispatch logic — this is valid and correct for the unit test. However, UT-01 does NOT validate that the real `agy --print "prompt"` writes its response to stdout in a non-TTY context.

**Additional test required before shipping E2:**
Add a manual verification step (not automated): run `agy --print "hello world" > /tmp/agy-out.txt && cat /tmp/agy-out.txt` in a non-TTY context, confirm the response file is non-empty. Document the agy version tested in a comment on the agy spec entry in `knownCLIAgents`. This step gates merging E2.

### Issue 2 (MEDIUM): `opencode run "prompt"` never succeeded end-to-end

**Impact on tests:** UT-04 validates the struct field only. The live behavior of `opencode run "prompt"` returning a response is blocked by a pre-existing config error in `~/.config/opencode/agents/skills/re-tool-radare2.md`.

**Additional test required before shipping E1.S1.T3:**
Fix the opencode config error and run `opencode run "say hello"` to confirm one-shot response. Update `UT-01` to optionally use `opencode` as the test binary once config is clean. Gate merging E1.S1.T3 on this verification.

### Issue 3 (MEDIUM): Migration — existing dual-patched installations

**Covered by:** IT-06 (`TestInstallAgy_StaleEntryCleanup`). This test was added to this plan as a direct response to the adversarial finding. It must pass before E3 is considered complete.

---

## Readiness Gate

### Criterion 1: All requirements have at least one test case

| Req | Test(s) | Status |
|-----|---------|--------|
| R1 | UT-01, UT-02, UT-03 | PASS |
| R2 | IT-01 through IT-06 | PASS |
| R3 | UT-05–UT-12, ST-01, ST-02 | PASS |
| R4 | ST-03, IT-07 | PASS |
| R5 | UT-13–UT-18 | PASS |
| R6 | UT-01, UT-04 | PASS |
| R7 | All UT-01–UT-18 | PASS |

**Result: PASS (7/7)**

### Criterion 2: Plan has clear acceptance criteria for every task

Each of the 33 tasks in E1–E7 has an explicit AC block. The AC format is consistent: file, change, and pass condition. All build/compile ACs are verifiable. All test ACs name the assertion.

**Result: PASS**

### Criterion 3: No critical ambiguities remain (adversarial review = CONCERNS or better)

Adversarial review verdict: **CONCERNS** (threshold is CONCERNS or better — met). The three issues are HIGH/MEDIUM severity but do not indicate plan logic errors — they are pre-ship validation steps that the review explicitly framed as "checklist for proceeding." The `PromptAsArg` design is validated as correct. The installAgy() path logic is validated as correct. The detection pattern approach (STUB + TODO) is validated as the right call.

**Result: PASS** (adversarial concerns are mitigations required at implementation time, not plan ambiguities)

### Criterion 4: Test cases specific enough to implement without further clarification

All 29 test cases include: file path, package, setup steps, explicit assertions, and plan references. No test requires the implementer to make a design decision not already resolved in the plan or its architecture decisions (AD-1 through AD-4).

**Result: PASS**

---

## Readiness Gate Verdict

**PASS**

All four gate criteria are met. The plan is ready to enter Phase 5 (Implementation). Before opening the PR, the implementer must also complete the three adversarial checklist items:

1. Verify `agy --print "hello world"` writes to stdout in a non-TTY context (Issue 1 — gates E2)
2. Fix opencode config error and verify `opencode run "say hello"` succeeds (Issue 2 — gates E1.S1.T3)
3. Confirm `IT-06` (`TestInstallAgy_StaleEntryCleanup`) passes (Issue 3 — gates E3)
