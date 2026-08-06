# Implementation Plan: create-rule-orphaned-fallback

**Feature**: Show the Create Rule button for orphaned pre-PR-315 approvals (missing `escalation_reason_category`) by switching the frontend gate from an allowlist (`=== "no-match"`) to a denylist against the 5 known non-no-match categories.
**Date**: 2026-08-03
**Status**: Ready for implementation
**ADRs**: None — single-file conditional fix, no new architecture, no debatable technology choice (see Step 5 rationale below).

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `EscalationCategory` | Go string type (`pkg/classifier/escalation.go`) classifying why a request was escalated. 6 constants: `EscalationNoMatch` (`"no-match"`), `EscalationExplicitRule` (`"explicit-rule"`), `EscalationDomainAge` (`"domain-age"`), `EscalationSecretScan` (`"secret-scan"`), `EscalationUnclassifiable` (`"unclassifiable"`), `EscalationUnexpected` (`"unexpected"`) | Single source of truth for category strings |
| `escalation_reason_category` | The frontend/wire-format metadata key (plain `map<string,string>`) carrying an `EscalationCategory` value into the review queue item | Set in `session/review_queue_poller.go` L854-855 only `if a.EscalationCategory != ""` |
| Orphaned approval | A `PendingApproval` deserialized from `pending_approvals.json` that was written to disk *before* PR #315 (escalation-reasoning) was deployed; its `EscalationCategory` field is the Go zero value `""` because the on-disk JSON predates the field and both fields are `json:",omitempty"` | Distinct from `PendingApproval.Orphaned` (an unrelated existing field meaning "session process is gone") |
| No-match | The `EscalationCategory` value meaning "no rule matched; classifier's default-escalate fallback applied" | **Correction**: an earlier draft claimed this was the only escalation type before PR #315 — false. Domain-age and secret-scan escalation *decisions* predate PR #315 by ~4 months (commit `1c31f024d`); PR #315 only added the category *label*. Treating "absent category" as "no-match" is therefore an accepted trade-off, not a proven equivalence — see Risk Control's "Named trade-off" |
| Allowlist gating (current/buggy) | `escalation_reason_category === "no-match"` — button shows only for one exact matching string; every other value (including `undefined`) hides it | The bug: `undefined` fails this check |
| Denylist gating (the fix) | Button shows unless `escalation_reason_category` is one of the 5 known non-no-match values; `undefined` and `"no-match"` both pass | Matches the requirement's suggested fix exactly |
| `orphanedCleanupThreshold` | 4-hour constant (`server/services/approval_store.go` L65) bounding how long an orphaned (session-gone) approval persists before cleanup | Not to be confused with "orphaned approval" in this plan's title — same word, different concept. Disambiguation is this plan's own addition; requirements.md uses "orphaned" for both senses without itself distinguishing them |
| Rollout window | The narrow time span during which in-flight `pending_approvals.json` entries created before a PR #315-class deploy are still active | Self-healing — bounded by the ~4-minute approval timeout or the 4-hour cleanup threshold |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Gating logic location | Frontend-only fix in `ReviewQueuePanel.tsx` | research/pitfalls.md #1, #2 (only 1 gating call site repo-wide; server-side default would poison `omitempty` disk state) | (c) Server-side default: set `EscalationCategory = EscalationNoMatch` when empty in `ApprovalStore.loadFromDisk` | Because the field is `json:",omitempty"`, a defaulted value gets re-serialized on next persist, permanently erasing the "predates the field" signal that lets us distinguish orphaned from genuinely-classified approvals |
| Gate expression shape | Named predicate function `isCreateRuleEligibleCategory()` backed by a module-level `Set<string>` denylist | research/pitfalls.md #3 (denylist inverts the allowlist; recommends a named/cross-referenced construct so a future 7th category is caught) + build-vs-buy.md (no shared utility exists for this shape; a fresh colocated helper is consistent with existing convention) | (a) Inline `.includes()` array check directly in the JSX conditional | An inline array literal re-created and re-scanned on every render, and buried in a JSX conditional, is harder to keep in sync with `pkg/classifier/escalation.go` than a single named, commented, module-scope construct that a test can assert against directly |
| Category taxonomy / new TS union type | No new type — keep `escalation_reason_category` as plain `string \| undefined` | build-vs-buy.md ("no TS union type exists anywhere for the category values... requirements' non-goals rule out inventing one") | Introduce a `type EscalationCategory = "no-match" \| "explicit-rule" \| ...` TS union mirroring the Go consts | Requirements' non-goals explicitly exclude changing/formalizing the taxonomy; proto/Go/TS all treat metadata as an untyped map already (research/stack.md) |
| Coverage-gap guard | Add a test asserting the denylist `Set` has exactly 5 entries matching the 5 known non-no-match `EscalationCategory` constants | research/pitfalls.md #3 | Rely on incidental per-category tests only | Per-category tests only catch categories someone remembered to test; a size/membership assertion fails loudly the moment `pkg/classifier/escalation.go` grows a 7th constant without a matching frontend update |
| Go-side test for the empty-string metadata-population path | Included as an **optional/nice-to-have** task (1.2.1a), not required for acceptance | requirements.md AC5 ("No backend change is required..."); stack.md notes the gap but the *existing* Go behavior (omit the key when empty) is already correct and unchanged by this fix | Skip entirely | The behavior under test isn't being modified — this task only closes a documentation/regression-locking gap on the producer side of the contract the frontend fix now depends on; safe to defer if time-boxed |

---

## Migration Plan
N/A — no schema/data changes. No backend code path is modified (frontend-only fix); `pending_approvals.json` format and `PendingApproval`/`PersistedApproval` structs are untouched.

## Observability Plan
No new logs/metrics/alerts required. Existing `log.Debug("enriched approval item with hook metadata", ...)` in `session/review_queue_poller.go` (~L857) already logs `escalation_category` per item and is sufficient to spot-check orphaned items (empty string) during the rollout window if needed.

## Risk Control
- No feature flag — the change only widens button visibility for a bounded, self-healing, already-in-production edge case; standard PR review + regression tests are sufficient gating.
- Rollback: revert the single-file diff to `web-app/src/components/sessions/ReviewQueuePanel.tsx` (plus its test file) — no data migration or backend coordination needed.
- Staged rollout: not needed given the change's low blast radius (UI-gating-only, frontend-only, additive visibility).
- **Named trade-off (from `pm:triad-review` UX pass, added post-review):** the denylist is fail-open — an orphaned approval's true cause could be domain-age or secret-scan (those escalation paths predate PR #315; only the label is new — see requirements.md's Correction note), not only no-match, and an unrecognized future `EscalationCategory` also defaults to "show." This is accepted because the existing "Reason not recorded" copy already renders alongside the button for these items, and the window is bounded/self-healing — call this out explicitly in the PR description rather than leaving it implicit.
- **CI enforcement gap:** the denylist size/membership guard test (Task 1.1.2d) only catches drift within `ReviewQueuePanel.tsx`'s own test file and is not CI-enforced — `web-app` Jest is not currently wired into `make ci` (tracked separately by sibling backlog items `wire-jest-ci`/`jest-ci-wiring`/`ci-jest-gate`). Until that's wired up, this guard only protects a reviewer who happens to run the test locally.

## Unresolved Questions
- [ ] Should `isCreateRuleEligibleCategory` be exported and given 2-3 direct unit tests (undefined→true, "no-match"→true, "explicit-rule"→false) rather than only being exercised indirectly via full-component render tests? — blocks nothing (Story 1.1.1 already achieves behavioral coverage), but architecture-review.md Concern 2 and the Engineering triad lens both flag this as the gap between the story's stated "testable predicate" goal and what's actually tested in isolation — owner: implementer, low-cost to add during Task 1.1.1b.
- [ ] Should the fail-open case (an unrecognized/future category value showing the button) get its own explicit test, per validation.md's `isCreateRuleEligibleCategory_should_returnTrue_When_CategoryIsUnrecognizedFutureValue`? — not currently in plan.md's task list (only in validation.md) — owner: implementer, recommend folding into Task 1.1.2d.

## Dependency Visualization
```
Task 1.1.1a (branch setup from origin/main)
        │
        ▼
Task 1.1.1b (add denylist Set + isCreateRuleEligibleCategory helper)
        │
        ▼
Task 1.1.1c (swap L845 gate to use helper)
        │
        ├──────────────┬───────────────────────┬─────────────────────┐
        ▼              ▼                       ▼                     ▼
Task 1.1.2a       Task 1.1.2b             Task 1.1.2c           Task 1.1.2d
(AC1 test:        (AC2 regression:        (AC3 tests:           (denylist-size
orphaned/missing  no-match still          5 non-no-match        guard test,
category shows    shows button —          categories each        pitfalls.md #3)
button)           already passes,         hidden)
                  assert no regression)
        │              │                       │                     │
        └──────────────┴───────────────────────┴─────────────────────┘
                                   │
                                   ▼
                     Task 1.1.3a (run jest, verify all pass)
                                   │
                                   ▼
                Task 1.2.1a (OPTIONAL — Go regression test for
                empty-EscalationCategory metadata-omission contract)
```

---

## Phase 1: Fix Create Rule Button Gating for Orphaned Approvals

### Epic 1.1: Frontend denylist-based gating fix

**Goal**: Replace the exact-match `=== "no-match"` allowlist check at `ReviewQueuePanel.tsx:845` with a denylist against the 5 known non-no-match `EscalationCategory` values, so an absent/missing category (orphaned pre-deploy approval) is treated the same as `"no-match"`.

#### Story 1.1.1: Introduce a denylist-based eligibility helper and wire it into the gate

**As a** developer maintaining the review queue UI, **I want** Create Rule button visibility driven by a single named, testable predicate instead of an inline exact-match string comparison, **so that** an orphaned approval (missing category) is correctly treated as eligible, and a future new `EscalationCategory` constant fails a test instead of silently defaulting to "show".

**Acceptance Criteria**:
- Create Rule button shows for a review-queue item whose `escalation_reason_category` metadata key is absent, provided `tool_input_command` is present.
  - *Given* a queue item with `metadata = { pending_approval_id: "approval-123", tool_input_command: "git push origin main" }` (no `escalation_reason_category` key at all — the orphaned pre-deploy shape), *When* `ReviewQueuePanel` renders, *Then* `screen.getByTestId("create-rule-session-approval")` is present in the document.
- Create Rule button still shows for `escalation_reason_category === "no-match"` (no regression to PR #315's happy path).
  - *Given* a queue item with `metadata = { tool_input_command: "rm -rf /tmp/foo", tool_name: "Bash", escalation_reason: "No matching rule; escalated for manual review.", escalation_reason_category: "no-match" }` (the existing fixture at `ReviewQueuePanel.test.tsx:1026-1045`), *When* the panel renders, *Then* `create-rule-session-approval` is still present and still has class `button({ intent: "secondary", size: "md" })`.
- Create Rule button stays hidden for every explicit non-"no-match" category from PR #315.
  - *Given* five queue items, each with `tool_input_command` set and `escalation_reason_category` set to one of `"explicit-rule"`, `"domain-age"`, `"secret-scan"`, `"unclassifiable"`, `"unexpected"` respectively, *When* each renders, *Then* `screen.queryByTestId("create-rule-session-approval")` is `null` for all five.
- Fix is covered by a regression test asserting the specific failure mode (missing category ⇒ visible; explicit non-no-match category ⇒ hidden), and by a size/membership guard on the denylist itself.
  - *Given* the module-level `NON_NO_MATCH_ESCALATION_CATEGORIES` Set exported (or accessible) from `ReviewQueuePanel.tsx`, *When* a test asserts `NON_NO_MATCH_ESCALATION_CATEGORIES.size === 5` and that it contains exactly `["explicit-rule", "domain-age", "secret-scan", "unclassifiable", "unexpected"]`, *Then* the assertion passes today and would fail the moment a 6th non-no-match category is added to `pkg/classifier/escalation.go` without a matching frontend update.
- No backend change is required for this fix approach.
  - *Given* the chosen approach is frontend-only (Pattern Decisions table), *When* the PR diff is reviewed, *Then* it touches only `web-app/src/components/sessions/ReviewQueuePanel.tsx` and `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx` (plus, optionally, the nice-to-have Go test in Epic 1.2, which is test-only and asserts existing behavior rather than changing it).

**Files**: `web-app/src/components/sessions/ReviewQueuePanel.tsx`, `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx`

##### Task 1.1.1a: Confirm routing — branch from `origin/main`, not local `main` (~3 min)
- The local `main` worktree is 32 ahead / 26 behind `origin/main` and does **not** contain PR #315 (escalation-reasoning) — `ReviewQueuePanel.tsx:845` and the taxonomy in `pkg/classifier/escalation.go` do not exist on local `main` at all.
- Run `git fetch origin main` then create the working branch from `origin/main` explicitly: `git checkout -b create-rule-orphaned-fallback origin/main` (do **not** branch from local `main`).
- Verify the branch point has the target line: `grep -n 'escalation_reason_category' web-app/src/components/sessions/ReviewQueuePanel.tsx` should show line 845 with `=== "no-match"`.
- Files: none changed — verification/setup only.

##### Task 1.1.1b: Add the denylist `Set` and `isCreateRuleEligibleCategory` helper (~4 min)
- In `web-app/src/components/sessions/ReviewQueuePanel.tsx`, near the top of the file (module scope, alongside other lookup tables like `ESCALATION_REASON_EMOJI`), add:
  ```ts
  // The 5 EscalationCategory values other than "no-match" introduced by PR #315
  // (escalation-reasoning) — see pkg/classifier/escalation.go's EscalationCategory
  // consts, the single source of truth for these strings. An orphaned pre-deploy
  // approval has no escalation_reason_category key at all (undefined); treat that
  // the same as "no-match" for Create Rule eligibility — see backlog 5fb93d9d.
  const NON_NO_MATCH_ESCALATION_CATEGORIES = new Set<string>([
    "explicit-rule",
    "domain-age",
    "secret-scan",
    "unclassifiable",
    "unexpected",
  ]);

  function isCreateRuleEligibleCategory(category: string | undefined): boolean {
    return category === undefined || !NON_NO_MATCH_ESCALATION_CATEGORIES.has(category);
  }
  ```
- Files: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

##### Task 1.1.1c: Swap the exact-match gate at L845 for the new helper (~2 min)
- Replace:
  ```tsx
  queueItem.metadata?.["tool_input_command"] &&
    queueItem.metadata?.["escalation_reason_category"] === "no-match" && (
  ```
  with:
  ```tsx
  queueItem.metadata?.["tool_input_command"] &&
    isCreateRuleEligibleCategory(queueItem.metadata?.["escalation_reason_category"]) && (
  ```
- Do not touch the emoji-lookup read at L751 (`ESCALATION_REASON_EMOJI[...] ?? ""`) — already null-safe and out of scope (research/pitfalls.md #1).
- Files: `web-app/src/components/sessions/ReviewQueuePanel.tsx`

#### Story 1.1.2: Regression tests for all 5 acceptance criteria

**As a** future maintainer, **I want** the orphaned-approval Create Rule visibility bug and its fix locked in by tests, **so that** a regression can't reintroduce the exact-match bug or silently miss a new escalation category.

**Acceptance Criteria**: (see Story 1.1.1's AC list — this story is the test implementation of the same 5 criteria)

**Files**: `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx`

##### Task 1.1.2a: Extend the orphaned-approval fixture to assert Create Rule button visibility (AC1) (~4 min)
- The existing test `"shows the orphaned-approval fallback copy when escalation_reason is absent"` at `ReviewQueuePanel.test.tsx:991-1006` builds exactly the orphaned shape (`tool_input_command` set, no `escalation_reason`/`escalation_reason_category` key) but only asserts the fallback reason-text copy.
- Add a new test in the `describe("escalation reason", ...)` block (starts `ReviewQueuePanel.test.tsx:942`), right after the existing orphaned-fallback test:
  ```ts
  it("ReviewQueue_should_showCreateRuleButton_On_OrphanedApprovalMissingCategory", () => {
    const item = makeApprovalItem({
      metadata: {
        pending_approval_id: "approval-123",
        tool_input_command: "git push origin main",
        tool_name: "Bash",
        // no escalation_reason / escalation_reason_category — orphaned pre-deploy approval
      },
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    expect(screen.getByTestId("create-rule-session-approval")).toBeInTheDocument();
  });
  ```
- Files: `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx`

##### Task 1.1.2b: Confirm the existing no-match test still passes unmodified (AC2) (~2 min)
- No new code needed — `"renders create-rule button with intent=secondary when category is no-match"` at `ReviewQueuePanel.test.tsx:1026-1045` already covers this exactly. Just re-run it after Task 1.1.1c's change to confirm no regression (folded into Task 1.1.3a's full run).
- Files: none (verification only)

##### Task 1.1.2c: Add hidden-button tests for the remaining 4 non-no-match categories (AC3) (~5 min)
- `"omits create-rule button when category is domain-age"` at `ReviewQueuePanel.test.tsx:1047-1061` already covers `domain-age`. Add 4 more tests (or parametrize with `it.each`) for `explicit-rule`, `secret-scan`, `unclassifiable`, `unexpected`, e.g.:
  ```ts
  it.each([
    "explicit-rule",
    "secret-scan",
    "unclassifiable",
    "unexpected",
  ])("omits create-rule button when category is %s", (category) => {
    const item = makeApprovalItem({
      metadata: {
        pending_approval_id: "approval-123",
        tool_input_command: "rm -rf /tmp/foo",
        tool_name: "Bash",
        escalation_reason: "test reason",
        escalation_reason_category: category,
      },
    });
    mockUseReviewQueueContext.mockReturnValue(makeContextValue([item]));

    renderPanel();

    expect(screen.queryByTestId("create-rule-session-approval")).not.toBeInTheDocument();
  });
  ```
- Files: `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx`

##### Task 1.1.2d: Add a denylist size/membership guard test (AC4, pitfalls.md #3) (~3 min)
- Export `NON_NO_MATCH_ESCALATION_CATEGORIES` (or a small accessor) from `ReviewQueuePanel.tsx` for test import, and add:
  ```ts
  import { NON_NO_MATCH_ESCALATION_CATEGORIES } from "../ReviewQueuePanel";

  it("denylist matches exactly the 5 known non-no-match EscalationCategory values", () => {
    expect(Array.from(NON_NO_MATCH_ESCALATION_CATEGORIES).sort()).toEqual(
      ["domain-age", "explicit-rule", "secret-scan", "unclassifiable", "unexpected"].sort()
    );
  });
  ```
- Comment cross-references `pkg/classifier/escalation.go` so an editor of one file is prompted to check the other, per research/pitfalls.md #3.
- Files: `web-app/src/components/sessions/ReviewQueuePanel.tsx` (export), `web-app/src/components/sessions/__tests__/ReviewQueuePanel.test.tsx`

#### Story 1.1.3: Verify

##### Task 1.1.3a: Run the full ReviewQueuePanel test suite and confirm all pass (~3 min)
- `cd web-app && npx jest --testPathPatterns="ReviewQueuePanel.test" --no-coverage`
- Confirm: previously-passing tests (no-match, domain-age, fallback copy) still pass; all newly added tests pass; total test count increased by 6 (1 orphaned-visibility + 4 parametrized hidden-category + 1 denylist-guard).
- Files: none (verification only)

---

### Epic 1.2 (Optional / nice-to-have): Lock in the backend metadata-omission contract

**Goal**: Close the test gap noted in research/stack.md — no existing Go test exercises the empty-`EscalationCategory` path through `review_queue_poller.go`'s metadata-population `if a.EscalationCategory != ""` guard (L854-855). This is **not required** by any acceptance criterion (AC5 explicitly says no backend change is needed) — the frontend fix works regardless of whether this task is done, since it depends only on the *absence* of the key, which is already the current (correct, unchanged) backend behavior. Include only if time permits; otherwise this Story's absence should be called out as a scoped-out item in the PR description with the one-line reason above.

#### Story 1.2.1: Regression test for empty-string EscalationCategory metadata omission

**As a** developer, **I want** a test proving that an empty `EscalationCategory` produces no `escalation_reason_category` metadata key at all (not an empty-string key), **so that** the contract the frontend fix depends on (absent key, not empty-string key) can't silently change in a future edit to `review_queue_poller.go`.

**Acceptance Criteria**:
- Supports AC5 indirectly (documents why "no backend change" is a safe choice, by pinning the current behavior it relies on).
  - *Given* a `stubApprovalMetadataProvider` seeding an `ApprovalMetadata` with `EscalationCategory: ""` (mirroring `TestReviewQueuePoller_EnrichesApprovalMetadata_ByUUID` at `session/review_queue_poller_test.go:945-988`, but with the category field omitted/empty instead of `"no-match"`), *When* `poller.checkSession(inst, nil)` runs and the resulting queue item's `Metadata` map is inspected, *Then* `_, exists := item.Metadata["escalation_reason_category"]` is `false` (key entirely absent, not present-with-empty-string).

**Files**: `session/review_queue_poller_test.go`

##### Task 1.2.1a: Add `TestReviewQueuePoller_OmitsEscalationCategoryMetadata_WhenEmpty` (~5 min, optional)
- Model on `TestReviewQueuePoller_EnrichesApprovalMetadata_ByUUID` (`session/review_queue_poller_test.go:945-988`): same `newSimpleTestPollerWithManager()` / `newControllerWithMock()` / `stubApprovalMetadataProvider` scaffolding, but seed `ApprovalMetadata{ApprovalID: "approval-2", ToolName: "Bash", EscalationCategory: ""}` (no `EscalationReason` either, matching the orphaned shape).
- Assert `_, exists := item.Metadata["escalation_reason_category"]; if exists { t.Errorf(...) }` — proving the key is omitted, not empty-stringed.
- Run: `go test ./session/... -run TestReviewQueuePoller_OmitsEscalationCategoryMetadata_WhenEmpty` (requires `make build` first per repo convention for proto/generated deps).
- Files: `session/review_queue_poller_test.go`
