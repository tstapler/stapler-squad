# ADR-002: Defer Flaky-Test-Aware Review Differentiation to Its Own Follow-Up

**Status**: Accepted
**Date**: 2026-08-11

## Context

Requirements item 3 asks whether flaky-test-classified items warrant a distinct review
strategy, and explicitly permits deferring full implementation ("may be out of scope for this
pass depending on planning-phase findings") — this is requirements.md's own named Fallback
Increment, not a scope cut invented at planning time.

Two research docs independently converge on the same concern:
- `research/pitfalls.md` §4: a title/description keyword heuristic ("flaky", "-race",
  "intermittent") both over-matches (a meta item *about* flakiness) and under-matches (a
  symptom-described item with no keyword), while a materially better-fitting *behavioral*
  signal already exists in this codebase's house style (`IsRepeatedFailure`/
  `IsRepeatedNoVerdictFailure`, `session/stuck_decisions.go`) — but building that signal
  well (review-verdict flip-flop across an otherwise-unchanged diff, or test-file-only rework
  cycles) is itself new detection logic, not a trivial reuse.
- `research/build-vs-buy.md` §3b: the keyword heuristic is cheap but only as good as the
  keyword list; the behavioral signal is more reliable but is greenfield work, not adaptation
  of an existing predicate.

## Decision

Defer requirement item 3 in full to its own follow-up backlog item. Ship this project as
multi-reason escalation (Signal 1) + capped-while-bouncing escalation (Signal 2) only.

## Reasoning

- The cost of shipping a wrong classifier (miscalibrated review strictness on a
  misclassified item, either loosening the bar for something that needed real deterministic
  verification, or tightening it for something already deterministic) is worse than shipping
  no differentiation yet — directly echoing pitfalls.md's own recommendation.
- A behavioral signal (the approach both research docs prefer over keyword matching) is
  greenfield detection logic requiring its own threshold calibration and false-positive
  analysis against live bouncing items — that is itself a complexity-2+ scoped piece of work,
  not a 2-5 minute task fitting inside this plan's Appetite (Small-Medium, this feature is
  already fully scoped by Signals 1+2).
- Requirements' own Rabbit Holes section warns against this becoming "a general
  intent-classification subsystem" — the safest way to honor that boundary is to not start
  down either heuristic path (keyword or behavioral) inside this project at all.

## Consequences

- No code changes for requirement item 3 in this project.
- A follow-up backlog item should be filed (see plan.md Story 3.2.1) that scopes the
  behavioral-signal approach specifically (review-verdict flip-flop / test-file-only rework
  cycles), not the keyword heuristic — carrying forward pitfalls.md's preference so the next
  planning pass doesn't have to re-derive it.
- Success Metrics verification for this project covers only Signals 1 and 2; requirement
  item 3's own success criteria (never explicitly stated as measurable in requirements.md)
  are out of scope until the follow-up item's own planning phase defines them.
