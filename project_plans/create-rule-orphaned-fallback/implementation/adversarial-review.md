# Adversarial Review: create-rule-orphaned-fallback

**Date**: 2026-08-03
**Verdict**: CONCERNS

All line numbers, test names, and code snippets in the plan were checked
against `origin/main` (`git show origin/main:<path>`) and confirmed accurate
— `web-app/src/components/sessions/ReviewQueuePanel.tsx:844-845`, the test
file's line numbers (942, 991, 1026, 1047), `makeApprovalItem`'s
`sessionId: "session-approval"` default (making `create-rule-session-approval`
the correct testid), and the 6 `EscalationCategory` constants in
`pkg/classifier/escalation.go` all match the plan's claims exactly. No
factual/citation errors found in the plan or its research docs.

## Blockers

None. No missing failure-mode handling, no architectural dead end, and no
acceptance criterion left uncovered severely enough to halt implementation.

## Concerns

- [ ] **The denylist's "regression-locking" safety net (Task 1.1.2d) has no CI
  enforcement today, undermining the plan's own stated rationale.** The
  Pattern Decisions table justifies the Set+guard-test design specifically
  because it "fails loudly" the moment a 6th non-no-match category is added
  without a matching frontend update. But per this repo's `CLAUDE.md`,
  frontend Jest is explicitly **not part of `make ci`** — and this exact gap
  is already independently tracked in three sibling backlog efforts in this
  same worktree (`project_plans/wire-jest-ci/`, `project_plans/jest-ci-wiring/`,
  `project_plans/ci-jest-gate/`), one of which cites zero `npx jest`
  invocations anywhere in `.github/workflows/*.yml`. Until one of those lands,
  a future PR that adds a 7th `EscalationCategory` constant to
  `pkg/classifier/escalation.go` without touching `ReviewQueuePanel.tsx` will
  merge cleanly — the guard test exists but nothing runs it. — Recommend
  adding one sentence to the plan's Risk Control section naming this
  dependency explicitly (the guard test is inert until Jest is wired into CI)
  so the PR description doesn't overstate the protection it provides.

- [ ] **The denylist's fail-open default for a wholly unrecognized category
  string is not tested or explicitly called out as intentional.** The 5
  Given-When-Then examples in Story 1.1.1 cover exactly three buckets: absent
  key, `"no-match"`, and the 5 known non-no-match constants. None covers a
  7th, *unanticipated* string (a typo, stale data, or a future category not
  yet mirrored to the frontend) — under the chosen denylist, any such string
  defaults to **show**. This is the precise failure class `pitfalls.md` #3
  already flags as "this codebase has already been burned by exactly this
  class of bug once" (citing `EscalationUnexpected`'s own doc comment: "never
  a silent no-op"). The existing Go-side precedent
  (`CategorizeEscalationRuleID`) deliberately fails toward a *known* default
  category rather than silently permitting an unrecognized input through — the
  frontend fix takes the opposite stance (permit by default) without stating
  that trade-off anywhere in the plan's prose or tests. Requirements.md's
  suggested fix text does specify a denylist shape, so this isn't a spec
  violation, but the choice is currently implicit rather than pinned. —
  Recommend one additional test: `escalation_reason_category` set to an
  unrecognized string (e.g. `"some-future-category"`) asserts the button
  **is shown**, with a comment stating this is the deliberately-chosen
  fail-open behavior. This turns an implicit assumption into an explicit,
  reviewable, regression-locked decision.

- [ ] **Epic 1.2 (optional Go test) is disproportionate ceremony for a
  complexity-1 fix and duplicates existing verified behavior.** AC5 states no
  backend change is required, and the plan itself notes the Go behavior under
  test (`review_queue_poller.go`'s `if a.EscalationCategory != ""` guard) is
  "already correct and unchanged by this fix." Adding it still requires a full
  `make build` (proto codegen) cycle per repo convention before `go test` can
  even run — a heavyweight prerequisite for a test-only task whose own
  acceptance criteria only "supports AC5 indirectly." Marking it "optional"
  in the plan doesn't remove the temptation for whoever implements to spend
  the time anyway, and it sits inside the same Dependency Visualization graph
  as the required tasks, implying more sequencing weight than it merits. —
  Recommend dropping Epic 1.2 from the implementation plan entirely and
  noting it as a one-line "considered and scoped out" bullet in the PR
  description instead, consistent with the complexity-1 / no-backend-change
  framing the requirements doc already committed to.

## Minors

- No test distinguishes `escalation_reason_category` **present as an explicit
  empty string** (`""`) from **entirely absent** (`undefined`). Both are
  handled identically and correctly by `isCreateRuleEligibleCategory` (neither
  is in the denylist Set, so both show the button), and the backend's
  `omitempty` tags mean an explicit `""` can never actually be serialized
  on-wire today — so this is a documentation gap, not a functional one. Worth
  a one-line comment near the helper noting `""` is handled the same as
  `undefined` for defense-in-depth, but not worth a dedicated test.
- The pre-existing test `"hides Create Rule button when item has no
  tool_input_command"` (`ReviewQueuePanel.test.tsx:341`) already covers the
  "tool_input_command absent regardless of category" case end-to-end and is
  untouched by this fix — confirmed by reading its fixture (metadata reduced
  to just `pending_approval_id`, no `tool_input_command`, no
  `escalation_reason_category`). The plan doesn't call this test out by name
  as part of its coverage story; doing so in the PR description would make
  the "no regression on the other gating clause" claim explicit rather than
  implicit in "run the full suite and confirm all pass."
- Exporting `NON_NO_MATCH_ESCALATION_CATEGORIES` as a named export for test
  access is a minor deviation from the file's existing convention of
  module-private lookup tables (`ESCALATION_REASON_EMOJI` is not exported).
  Reasonable trade-off for testability; not worth blocking on.
