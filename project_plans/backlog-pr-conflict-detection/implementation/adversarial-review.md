# Adversarial Review: backlog-pr-conflict-detection

**Date**: 2026-07-12 (iteration 2)
**Verdict**: CONCERNS

## Blockers

None. Both iteration-1 blockers are resolved, verified against the actual plan text (not the fix agent's summary):

- [x] **Belt-and-suspenders OR condition** — every conflict-detection mention in the current plan uses `mss == "DIRTY" || mg == "CONFLICTING"` consistently: Domain Glossary rows for `HasConflicts` (line 27), `mergeable`/`mergeStateStatus` (lines 35-36), and the new "Conflict condition (`mss`/`mg`)" row (line 37); Task 1.1.1a's doc-comment sketch (line 149); Task 1.1.1d's actual code block (line 213, `if mss == "DIRTY" || mg == "CONFLICTING" {`); Story 1.2.1's AC (lines 233-234, which explicitly exercises the stale-`mergeable` cli/cli#9583 case); and the Unresolved Questions section (line 85). A repo-wide grep for `CONFLICTING`/`DIRTY` in plan.md turned up no leftover single-field condition anywhere. Story 1.3.1's table now has 7 cases, and case 7 (`mergeable=MERGEABLE, mergeStateStatus=DIRTY → HasConflicts=true`, lines 280/283) is exactly the previously-missing regression case for #9583.
- [x] **Guidance-text substrings now tested** — new Task 1.3.1b (`TestParsePRStatusPayload_ConflictGuidanceText`, lines 292-299) asserts `strings.Contains` for all three required substrings (`--force-with-lease`, `.gitignore`, `leave the conflict markers`) against `FeedbackText`, with a first subtest on the primary trigger case and a second subtest reusing case 7 to prove the guidance text is identical regardless of which field tripped the OR. This closes the gap exactly as recommended in iteration 1.

Internal consistency check: Task Summary counts (5 epics, 11 stories, 20 tasks) match the actual body — verified by counting every `#####  Task` line (4+1+5+4+6=20) and every `Story` heading (1+1+3+2+4=11). The Dependency Visualization diagram was correctly updated to include both new tasks (1.3.1b listed with its `depends on 1.2.1a` note at line 106; 2.2.2b listed with its `depends on 2.2.2a` note at line 116) — no dangling/orphaned task references.

## Concerns

- [x] **Resolved** — architecture-review.md Concern A (`FeedbackText`/bool drift): the fix agent's `render()` reshape (Task 1.1.1c, Domain Glossary rows for `render`/`conflict`/`failedChecks`/`blockingReview`, Pattern Decisions table) is coherently threaded through the whole plan — no task still describes building `FeedbackText` via interleaved `sb.WriteString` calls during evaluation; Task 1.1.1d (evaluation) only sets `status.HasConflicts`/`status.conflict`, and Task 1.2.1a's guidance text is correctly nested inside `render()`'s existing `if s.conflict != nil` nil-check, so there's no new nil-pointer risk from the reshape.
- [x] **Resolved** — architecture-review.md Concern B (untested log line): Task 2.2.2b now redirects `log.InfoLog` to a `log.NewDummyLogger` and asserts `conflict=true`; Tasks 2.2.3a/2.2.3b were also updated to assert `conflict=false` alongside `CI=true`/`reviews=true` respectively, closing the transposition risk the architecture review flagged.

- [ ] **New, minor: conflict guidance text can display a misleading `mergeStateStatus` value when `mergeable` alone triggers the OR.** Task 1.1.1d captures `status.conflict = &conflictInfo{mergeStateStatus: payload.MergeStateStatus}` — the raw field value, regardless of which side of the OR fired. Story 1.3.1's case 3 (`mergeable=CONFLICTING, mergeStateStatus=BLOCKED → HasConflicts=true`) means the rendered guidance (Task 1.2.1a) would read "...has merge conflicts against its base branch (mergeStateStatus=BLOCKED)..." — a value that, read on its own, looks like a required-check/review gate rather than a conflict, which is confusing for the agent consuming this prompt. This is not new in the sense of being introduced by this iteration's fix (the same field-capture choice would have existed pre-reshape too), but it's more visible now that `render()` centralizes the message. Low severity — doesn't affect `HasConflicts` correctness, only prompt wording clarity for one of seven table-tested cases. No task currently asserts the message body for case 3.

- [ ] **Carried over from iteration 1, still unresolved: no test exercises the `gh pr view` failure path for `ReconcilePRPending`.** `fakePRPendingChecker` (Story 2.2.1) still defines `mergedErr`/`statusErr` fields, but no task in the current Epic 2.2 (2.2.1a, 2.2.2a, 2.2.2b, 2.2.3a, 2.2.3b, 2.2.4a) sets them non-nil or asserts the continue-and-no-spawn behavior on error. Unchanged from iteration 1 — the fix pass did not touch this gap.

- [ ] **Carried over from iteration 1, still unresolved: `GetPRStatus`'s wrapper (post-extraction) has zero direct test coverage.** All Epic 1.3 tests still call `parsePRStatusPayload` directly; nothing exercises the thin `checkGHCLI()` → run `gh` → `return parsePRStatusPayload(raw)` wrapper itself. Low risk given its size, but still untested as of this iteration.

- [ ] **Carried over from iteration 1, still unresolved: no test proves the full chain is wired together.** Phase 1 tests never touch `ReconcilePRPending`; Phase 2 tests never go through real JSON parsing (they hand-construct `PRStatus` inside `fakePRPendingChecker.status`). A logic mismatch between `GetPRStatus` and the gate (e.g., `GetPRStatus` not actually calling the updated `parsePRStatusPayload`) would compile cleanly and pass every planned test.

- [ ] **Carried over from iteration 1, still unresolved: the "clean give-up vs. silent failure" gap has no tracked artifact beyond plan.md.** Still correctly scoped out of this Medium-appetite project, but still has no durable pointer (issue/code comment) outside the planning doc.

## Minors

- Struct-literal AC at Story 1.1.1 (line 142) — `&PRStatus{CIFailing: false, HasBlockingReviews: false, HasConflicts: false, FeedbackText: ""}` — happens to still hold under the reshaped struct (new unexported fields are nil/zero-valued for the healthy case too), but is worth a one-line acknowledgment in the eventual test code that this is a partial literal relying on zero-value unexported fields, not a full-field comparison, so a future struct-shape change doesn't silently break the intent.
- "Exactly 2 production files" framing (Task Summary) still slightly undersells new surface area (`prPendingChecker` interface, `newPRPendingChecker` var, plus now `render()`/`conflictInfo`/`reviewInfo`) — same nit as iteration 1, unchanged, still just a communication nit not a design flaw.
- Tasks 1.3.2a and 2.2.3a still go beyond requirements.md's literal Scope text (which names only `HasBlockingReviews` for first-ever regression coverage), covering `CIFailing` too — still defensible "while we're touching this file" hardening, unchanged from iteration 1.
- Verified again: the log line's pre-existing `item=%s` clause (`backlog_lifecycle.go:531-583`) already satisfies half the Observability Requirement; only `conflict=%v` is new. No action needed.
