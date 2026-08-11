# Adversarial Review: backlog-github-issue-link (re-review of Blocker)
**Date**: 2026-07-25
**Verdict**: RESOLVED

## Scope of this re-review

Scoped strictly to the single original BLOCKER: Task 4.1.2b's instruction-line
template applying a uniform `"%s: %s"` format to `closingKeywordFor`'s
(previously bare) keyword output, producing `"Fixes: <url>"` instead of AC3's
literal `"Fixes <url>"`. I did not re-review anything outside this blocker and
its blast radius.

## 1. Rendered string trace — both cases

`closingKeywordFor` (`plan.md` Task 4.1.1a, lines 314-323) now returns the
fully-punctuated prefix directly:
- `/issues/` URL → `"Fixes "` (trailing space, no colon)
- `/pull/` URL, empty string, or unrecognized shape → `"Related: "` (colon+space)

Task 4.1.2b's caller (lines 350-360) was changed to concatenate with **no**
separator:

```go
fmt.Fprintf(&sb, "...include the line `%s%s` in the PR body...",
    item.ExternalURL, closingKeywordFor(item.ExternalURL), item.ExternalURL)
```

Tracing both cases:
- Issue: `closingKeywordFor(url) + url` = `"Fixes " + "https://github.com/acme/widget/issues/42"` = `` `Fixes https://github.com/acme/widget/issues/42` `` — single space, no colon.
- PR: `"Related: " + "https://github.com/acme/widget/pull/17"` = `` `Related: https://github.com/acme/widget/pull/17` `` — colon+space.

Both match AC3's literal wording (`requirements.md` lines 65-69: `"Fixes "` for
issues, `"Related: "` for PRs) exactly, character for character. The plan
itself calls out the fix explicitly at line 358: *"Note the format verb is
`%s%s`, not `%s: %s` — ... Using `%s: %s` here would incorrectly render
`"Fixes: https://..."`"* — this is the correct diagnosis of the original bug,
stated as a guard against reintroducing it.

## 2. Stale references to the old (wrong) behavior

Searched `plan.md`, `ADR-001`, and `requirements.md` for any remaining
bare-keyword-plus-separate-colon depiction. Found none in any of these three
files — every occurrence of `closingKeywordFor`, the Domain Glossary rows for
it and "Instruction line," ADR-001 Decision 2, and all GWT examples in Story
4.1.1/4.1.2 consistently describe the fully-punctuated, no-separator design.

One stale artifact does exist: `implementation/architecture-review.md` (the
original review that raised this blocker) still shows the old code/bug
verbatim in its own Blockers/Nitpicks sections (e.g. line 11: `closingKeywordFor`
"deliberately returns the bare keyword `Fixes`/`Related`"; line 23: default
`return "Related"` with no colon). This is expected and correct — it's a
timestamped review record of the plan *before* the fix, not a live spec, and
nothing in the task asked for it to be updated. It is not a residual bug in
the plan; the plan itself (`plan.md`) has no such stale content.

## 3. Are the tightened test assertions actually strict enough?

Checked every assertion the fix touches:
- Task 4.1.1b (`closingKeywordFor` table test): now specifies `assert.Equal`
  (exact-value), not substring, for all four cases (`"Fixes "`, `"Related: "`,
  empty→`"Related: "`, unrecognized→`"Related: "`). Strict.
- Task 4.1.2c (`BuildSessionInitialPrompt` unit test): asserts the **exact
  literal substring** `"Fixes https://github.com/acme/widget/issues/42"` (no
  colon) for the issue case, and separately `"Related: https://github.com/acme/widget/pull/17"`
  (with colon) for the PR case — both explicitly called out as "not a loose
  `Contains(..., "Fixes")`." This is the exact class of assertion the original
  blocker said was missing. Strict, and covers both keyword branches.
- Task 5.1.2a (integration test): same treatment — exact literal substring for
  the issue case, plus an explicit added PR-URL variant/table case asserting
  `"Related: https://github.com/acme/widget/pull/17"` with colon. This closes
  what would otherwise be an asymmetric gap (issue case tested strictly, PR
  case untested at the integration boundary) — confirmed both are now covered.

One place remains loose by design, correctly so: AC4's negative-case test
description (line 337, "output contains neither ... `"Fixes"`/`"Related:"`
closing-keyword text") is an *absence* check — no punctuation precision matters
when asserting a string doesn't appear at all. Not a gap.

Story 4.1.3 / Task 4.1.3a (token-budget truncation test) describes its
assertion loosely ("still contains the fact line and instruction line text"
— plan.md line 374) rather than pinning exact punctuation. This is acceptable:
this test's job is proving truncation doesn't drop the section, not
re-verifying punctuation already pinned exactly by 4.1.2c/5.1.2a. Not a new
gap introduced by the fix — it was already scoped that way pre-fix.

## 4. Do the "Known follow-ups" acknowledgment notes actually appear in the plan?

Confirmed by direct read of `plan.md`, not just the fix's own summary:

- **backlogItemToProto scope boundary** — present, `plan.md` lines 403-404,
  immediately after Task 5.1.1b. Reads sensibly: explicitly frames it as "a
  conscious scope boundary, not an oversight," ties it to the absence of any
  AC requiring web UI exposure, and leaves the door open for a future PR.
- **Shotgun-surgery risk in the two `ent.BacklogItem{}` literals** — present,
  `plan.md` line 405, same location. Reads sensibly: names both call sites,
  correctly attributes the pattern to how `ExternalID` was historically
  missed, and explicitly declines to fix it in this PR with a calibration
  rationale ("disproportionate for a 9-AC plumbing change").
- **GitHub bare-URL-recognition manual-verification recommendation** —
  present, `plan.md` line 446, immediately after Task 6.1.1b. Reads sensibly:
  states the limitation (can't unit-test live GitHub closing-keyword parsing
  of a bare full URL), explicitly says every AC and `make test` can pass while
  this remains unverified, and recommends a concrete manual step (one real
  test PR against a real linked issue) without blocking merge on it.

All three landed in the actual file, not only in a fix subagent's self-report.

## Verdict

**RESOLVED.** The rendered instruction line matches AC3's literal wording
exactly for both the issue and PR cases. No stale depiction of the old
bare-keyword-plus-colon behavior remains in `plan.md`, `ADR-001`, or
`requirements.md` (the pre-fix `architecture-review.md` correctly retains it
as a dated review record, not a live spec). The test assertions the original
blocker called out as insufficiently strict were tightened to exact-literal
checks, symmetrically for both keyword branches, at both the unit and
integration level. No new problem was introduced by the fix.

## Original Concerns (carried forward, not re-reviewed)
- Unverified assumption: GitHub recognizing bare full URL as closing reference (now acknowledged in plan per the fix — confirmed present, `plan.md` line 446)
- backlogItemToProto scope gap (now acknowledged in plan per the fix — confirmed present, `plan.md` lines 403-404)
- Shotgun-surgery risk in the two ent.BacklogItem{} literals (now acknowledged in plan per the fix — confirmed present, `plan.md` line 405)
- Task 1.1.1b's build-failure claim (now corrected per the fix — confirmed present, `plan.md` line 127: "it is expected to **pass** at this checkpoint ... not a red flag if it succeeds")

## Minors (carried forward, not re-reviewed)
- ent NULL-scan mechanism description was imprecise (cosmetic, doesn't affect implementation correctness)
- Extra blank line in fact-line rendering (cosmetic)
- UserModifiedFields bypass permanence not explicitly stated (cosmetic)
