# Architecture Review: backlog-triage-e2e-hardening

**Date**: 2026-06-23
**Verdict**: CONCERNS

## Findings

### Parser Fix (Phase 1A)

- [CLEAN] The brace-scan pattern copied from `ParseHeadlessVerdictResult` is correct. The proposed replacement body matches the source pattern precisely: `strings.Index` for `start`, `strings.LastIndex` for `end`, guard `end <= start`, unmarshal `raw[start:end+1]`, cap tasks at `maxHeadlessTriageTasks`. No import changes are needed (`strings`, `fmt`, `json` are already present).

- [CONCERN] The plan claims the current implementation is at lines 92–116 in `backlog_triage.go`. The actual function body runs from line 92 to line 116 — this is accurate. However, the existing `TestParseHeadlessTriageResult_StripsMarkdownFences` test passes `"` ` ` `json\n...\n` ` `` `"` which currently works via the fence-strip path. Under the brace-scan replacement, the same input still works because the `{` still appears in the string — the fenced case is implicitly covered. The plan does not explicitly call this out; the implementer should verify it mentally before removing the fence-strip code.

- [CONCERN] The brace-scan in `ParseHeadlessVerdictResult` uses `strings.Index` (first `{`) + `strings.LastIndex` (last `}`). This is the plan's approach too. But the plan's acceptance criterion says "Input with intermediate JSON during research phases and a final JSON block uses the LAST JSON block (brace-scan with `strings.LastIndex`)." This is only partially correct: `strings.LastIndex` finds the last `}`, not the last complete JSON object. If the final JSON block is well-formed and the intermediate one is also well-formed (as in the test case), the approach works because the last `}` belongs to the final block. The plan's test `TestParseHeadlessTriageResult_IntermediateJSONIgnored` is designed so this is true. No code change needed, but the claim in the acceptance criteria slightly overstates what the algorithm does — worth noting for future maintainers.

### UI Gate (Phase 1B)

- [CLEAN] Both `BacklogItemDetail.tsx` Trigger Triage locations are correctly identified. Lines 590–597 (idea block) and 603–610 (ready block) match the actual source. The proposed three-attribute pattern (`disabled`, `aria-disabled`, `title`) matches what the existing "Mark Ready" button does (lines 580–589 in the same file), so the implementation is consistent with codebase conventions.

- [CLEAN] The `BacklogItemCard.tsx` location is correctly identified. Line 33 is `return { label: "Trigger Triage", action: "trigger_triage" };`. The plan's change to add `disabled: !item.repoPath` is correct — the card's button renders `disabled={actionSpec.disabled || isTriageRunning}` (line 125), so the `disabled` field on the spec is already wired through. No further card changes are needed.

- [CONCERN] The plan does NOT add `aria-disabled` or `title` to the card button for the `repoPath` case — it only sets `disabled: !item.repoPath` on the `ActionSpec`. Looking at the card's render (lines 122–137), the button uses `disabled={actionSpec.disabled || isTriageRunning}` but there is no `aria-disabled` or `title` propagated from `ActionSpec`. The "idea" status Mark Ready button in the card (line 25–29) also only uses `disabled` on the spec — it does not set `aria-disabled` on the card either. So the card is consistent with the existing pattern; `aria-disabled` and `title` are detail-pane-only guards. This is acceptable, but the plan should explicitly acknowledge this asymmetry rather than leaving it implied.

- [CONCERN] The BacklogItemCard `getActionSpec` only handles `case "ready"` for Trigger Triage, but BacklogItemDetail shows the button in BOTH "idea" and "ready" status blocks. The card has no "idea" status case that maps to "trigger_triage" — in "idea" the card shows "Mark Ready". So the card fix is limited to ready status, which is correct. The plan accurately reflects this (Story 1.2.3 says "ready case"), but a reader might wonder whether the idea-status card needs guarding too — the plan does not explain why not. Worth a brief note.

### Unit Tests (Phase 2)

- [CLEAN] The three new test functions (`TestParseHeadlessTriageResult_PreambleBeforeJSON`, `_IntermediateJSONIgnored`, `_NoJSON`) directly cover the new brace-scan code paths. The `_IntermediateJSONIgnored` test correctly verifies that the LAST block wins. Existing test names match the file: `_ValidJSON`, `_StripsMarkdownFences`, `_InvalidJSON`, `_CapsTasksAt12`, `_EmptySuggestionsOK` — all present. The plan correctly lists them in acceptance criteria.

- [CONCERN] The `_PreambleBeforeJSON` test input does not include a fenced variant (```` ```json\n{...}\n``` ````). The requirements scope says "preamble-before-fenced-JSON" is also required ("add test cases for preamble-before-JSON and preamble-before-fenced-JSON"). The plan only covers preamble-before-bare-JSON. A fenced variant with preamble like `"Some text.\n` ` ` `json\n{...}\n` ` `` `"` is not tested. Under the brace-scan implementation this would also work (the `{` inside the fence is still the first `{`), but the missing test case is a gap against the stated requirements.

### E2e Tests (Phase 3)

- [CONCERN] Task 3.1.1a adds `repoPath` support to `fillNewItemForm` using `data-testid="backlog-repo-path-input"`. This testid does not appear in the actual `BacklogPage.ts` or `BacklogItemDetail.tsx`. It is assumed to exist in `BacklogItemForm.tsx` (not reviewed), but the plan does not confirm this. If that testid is absent, the `repoPathInput.fill(...)` call will silently time out. The plan should call out "verify this testid exists in BacklogItemForm before implementing" or the implementer needs to check.

- [CONCERN] The e2e test (Task 3.1.2a) asserts `await expect(triggerBtn).toHaveAttribute('aria-disabled', 'true')`. In the current source, `aria-disabled` is a boolean prop in React. When React renders `aria-disabled={false}`, the attribute is omitted from the DOM entirely; when `aria-disabled={true}`, the DOM attribute value is the string `"true"`. The test assertion `toHaveAttribute('aria-disabled', 'true')` is correct for the truthy case (button disabled without repoPath), so the assertion itself is sound. However, the plan should add a complementary assertion that after setting a repoPath the attribute is absent/false — otherwise the test only proves the disabled state, not that it can be re-enabled. This is mentioned in requirements (Success Metric 1) but the plan's test does not cover it.

- [CONCERN] The cleanup mechanism in `test.afterEach` calls `POST /session.v1.SessionService/DeleteBacklogItem`. This RPC name is unverified against the actual proto definition. The plan itself hedges ("Adjust the endpoint path to match the actual DeleteBacklogItem RPC"). Before the test can be merged, the implementer must confirm this RPC exists in `proto/session/v1/session.proto`. If it does not exist, the cleanup silently swallows the error (`catch { }`) and leaves test pollution. The plan should flag this as a prerequisite check.

- [CONCERN] The requirements (Success Metric 3) call for a happy-path test: "create item → set repo_path → trigger triage → verify loading indicator → verify item transitions to 'ready'". The plan covers only the disabled-button gate test. There is no happy-path test and no plan task for it. The plan's scope only matches part of requirements success metrics. The plan is intentionally narrower here (the open question about `claude` binary in CI is unresolved), but the mismatch between requirements and plan is not acknowledged. At minimum the plan should note this is deferred pending CI mock-triage resolution.

- [CLEAN] The `test.describe('Triage')` nesting inside `test.describe('Backlog')` is consistent with the existing spec structure. The `BASE_URL` constant is already defined in the file. The `BacklogPage` import is already present.

- [CLEAN] The `// @feature` annotation update correctly uses the same pattern as the existing feature catalog entries in the file.

### Feature Registry (Phase 4)

- [CONCERN] The plan's registry task (Task 4.1.1a) says to run `make registry-generate` "if the tooling auto-fills entries from source markers" and to "add `// +feature: backlog:trigger-triage` in the first 10 lines of `BacklogItemDetail.tsx` first". The `BacklogItemDetail.tsx` already has `// +feature: backlog:item-detail` on line 2. Adding a second `// +feature:` marker for `backlog:trigger-triage` to the same file may be the right approach, but the plan is ambiguous about whether to replace or append the marker. Looking at the existing file, it has a single marker. The plan's instruction to "add" the marker but not say where (replace or append) is unclear. This needs clarification.

- [CLEAN] The `docs/registry/frontend-features.json` path is correct. The JSON entry structure matches the schema used by other entries (verified by checking `docs/registry/` exists with the right files).

- [CLEAN] The feature registry step is necessary: the `.claude/rules/feature-registry.md` rule requires updating `frontend-features.json` for any new UI feature. The `backlog-trigger-triage` entry does not exist yet.

### File Paths

- [CLEAN] All file paths in the plan exist:
  - `session/backlog_triage.go` ✓
  - `session/backlog_review.go` ✓
  - `session/backlog_triage_test.go` ✓
  - `web-app/src/components/backlog/BacklogItemDetail.tsx` ✓
  - `web-app/src/components/backlog/BacklogItemCard.tsx` ✓
  - `tests/e2e/pages/BacklogPage.ts` ✓
  - `tests/e2e/backlog.spec.ts` ✓
  - `docs/registry/frontend-features.json` ✓

## Overall Assessment

The plan is architecturally sound and covers all three button locations correctly; the parser fix faithfully copies the right pattern from `ParseHeadlessVerdictResult`. The main risks are: (1) the `data-testid="backlog-repo-path-input"` testid in the e2e helper is unverified and could silently break, (2) the `DeleteBacklogItem` RPC used for cleanup is unverified against the proto, (3) the plan omits a preamble-before-fenced-JSON unit test required by the stated requirements, and (4) the happy-path triage e2e test from requirements Success Metric 3 is deferred without explicit acknowledgment. None of these are blockers for the fix tasks themselves, but they should be resolved before the PR is considered complete.
