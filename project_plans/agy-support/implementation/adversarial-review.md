# Adversarial Review: agy-support

**Date**: 2026-05-25
**Reviewer**: Adversarial architecture reviewer
**Verdict**: CONCERNS

---

## Blockers

_(None — no issues that must be resolved before implementation starts)_

---

## Concerns

- [ ] **P-1 schema assumption baked into fallback logic without live validation** — The plan correctly identifies that the `$TOOL_INPUT` schema is unconfirmed (P-1), but then proceeds to hardcode two specific schema variants (A and B) in `parseGeminiPayload()`. If the real agy payload uses a third schema (e.g., a flat `{"command": "..."}` without a wrapping `name`/`args` envelope), both variant checks will fail silently and every call classifies as `Escalate`. This is safe (not a false-allow) but means the hook effectively does nothing on agy until someone notices and captures the real payload. **Recommendation**: Task 4.1.2 (live capture) should be listed as a BLOCKER-equivalent gate — implement the capture BEFORE writing `parseGeminiPayload()`, not after. Alternatively, log a warning on unknown schema at a higher visibility level (stderr, not only when `STAPLER_DEBUG=1`).

- [ ] **Exit code contract for AutoAllow not confirmed against live agy** — The plan states "exit 0 = allow" as confirmed from `install-gemini-hook.sh`, but that script is a bash script in this codebase, not official agy documentation. If agy interprets the absence of a specific stdout signal as a block (rather than exit code), then `writeGeminiHookDecision()` returning exit 0 with no stdout would silently allow everything regardless of rule decisions. **Recommendation**: Explicitly test with a known-deny rule against a live agy session before merging. Add a comment in `writeGeminiHookDecision()` marking the assumption as unconfirmed-but-best-effort.

- [ ] **`io.ReadAll` in `parseGeminiPayload()` vs streaming concern** — The plan switches from `json.NewDecoder(os.Stdin).Decode` (streaming) to `io.ReadAll` (buffers entire payload). For typical tool inputs this is fine, but if agy ever passes a large file content as `$TOOL_INPUT` (e.g., for a `read_file` tool), this buffers the full content in memory. More importantly, `io.ReadAll` followed by `json.Unmarshal` means a parse error loses the raw bytes for debugging — the debug logging correctly saves `raw` before unmarshal, which is good. Minor concern but worth noting the design diverges from the existing Claude path.

- [ ] **`patchBeforeToolHook` atomic write: `settingsPath + ".tmp"` in same directory assumed** — The atomic rename (`os.Rename(tmpPath, settingsPath)`) is only truly atomic if tmp and dst are on the same filesystem/mount. Since both are always under `~/.gemini/`, this is almost certainly safe on any normal Linux setup. But if the user has a bind-mount or separate filesystem for `~/.gemini/`, the rename will fail with `EXDEV`. The existing `patchClaudeSettings` has this same limitation (accepted). **Recommendation**: Document this limitation in a comment, matching the existing `patchClaudeSettings` approach.

- [ ] **`installGemini()` now uses `check --gemini` but old manual instructions used `check` (no flag)** — The current `installGemini()` prints `ssq-hooks check` (no `--gemini` flag) in its manual instructions. The upgraded version will write `destBin + " check --gemini"` to the settings file. If any user previously followed the manual instructions and has `ssq-hooks check` (no flag) in their Gemini settings, re-running `install gemini` will detect `existing != hookCmd` (because `check` != `check --gemini`) and overwrite the old hook. This is correct behavior, but users should be warned. **Recommendation**: Add a migration note in the confirmation print: "Updated hook (previously: ssq-hooks check → now: ssq-hooks check --gemini)."

---

## Minors

- The `AskUserQuestion` guard in `handleCheck()` is moved inside the `else` block (Claude-only path) — this is architecturally correct since Gemini's equivalent is handled inside `parseGeminiPayload()`. The refactoring is safe but the comment in the old code ("not a permission gate") should move to the new location to remain co-located with the logic.

- The plan lists `os.Exit(0)` inside `parseGeminiPayload()` for `ask_for_user_input` — this is a side-effectful early exit inside a function that callers expect to return a value. Idiomatic Go would have `parseGeminiPayload()` return a sentinel value and let `handleCheck()` call `os.Exit(0)`. The plan's approach works but is harder to test. Low impact for a single-binary CLI.

- Task ordering in the plan table shows Epics 2.1 and 2.2 as independent, but 2.2 (`--gemini` flag in `handleCheck()`) has a compile-time dependency on 2.1 (`parseGeminiPayload()`). This is implied by the dependency diagram at the top but not explicit in the task list. Implement 2.1 before 2.2.

- The plan does not mention updating the `make quick-check` / `make ci` pipeline verification for the new code path. Since all changes are in `cmd/ssq-hooks/main.go`, existing Go unit test coverage applies. No new test file is required for the plan to be complete, but a note acknowledging this would close the loop.

- REQ-4 (detector.go comment) is low-risk but the plan correctly notes it. The `gemini_permission` pattern `(?i)Yes, allow once` is also present in `file_permission_claude` (`(?i)Yes, allow once` substring). This is not a bug — the claude pattern is higher priority (20 vs 17) and matches a broader string. The agy/gemini path through `gemini_permission` fires for the same text. This is an existing overlap, not introduced by this plan.

---

## Verdict Rationale

No issues rise to BLOCKER level because:
1. The unknown-schema risk (P-1) is mitigated by graceful escalation — worst case is the hook passes everything to agy's native dialog, not a security regression.
2. The exit-code assumption is derived from an existing bash script in the codebase, which provides reasonable (if unideal) evidence.
3. All changes are in one file (`cmd/ssq-hooks/main.go`) with one minor detector.go comment — blast radius is small and easily reverted.

The CONCERNS are real: the plan should complete Task 4.1.2 (live capture) before finalizing `parseGeminiPayload()`, and the exit-code contract should be verified against live agy before the PR is merged. These are implementation-order concerns, not plan-level blockers.
