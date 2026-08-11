# Architecture Review: backlog-github-issue-link (re-review of Blocker)
**Date**: 2026-07-25
**Verdict**: RESOLVED

The original blocker — Task 4.1.2b's instruction-line template using a uniform `"%s: %s"` format applied to a bare-keyword `closingKeywordFor` output, producing the wrong `"Fixes: <url>"` instead of AC3's `"Fixes <url>"` — is fixed, and the fix is internally consistent across every place the plan touches this behavior.

1. **`closingKeywordFor` now returns fully-punctuated strings.** Task 4.1.1a's implementation returns `"Fixes "` (trailing space, no colon) for `/issues/` URLs and `"Related: "` (colon+space) for `/pull/` and the default/unrecognized case — matching AC3's literal wording exactly, punctuation included. Confirmed in the Domain Glossary row (line 17), Story 4.1.1's AC3 GWT examples (lines 295-298), and the function's doc comment and body (lines 304-323).

2. **The caller now concatenates directly with no added colon.** Task 4.1.2b's `fmt.Fprintf` format verb is `%s%s` (not `%s: %s`), passing `closingKeywordFor(item.ExternalURL)` and `item.ExternalURL` as the two args with no separator. The task includes an explicit inline note (plan.md:358) stating why `%s: %s` was rejected ("would incorrectly render `Fixes: https://...` (extra colon)"). Rendered output verified by hand: issue case → `` `Fixes https://github.com/acme/widget/issues/42` `` (no colon); PR case → `` `Related: https://github.com/acme/widget/pull/17` `` (colon present, since the colon now lives inside `closingKeywordFor`'s own return value, not the caller's format string). Both match AC3 exactly.

3. **Test assertions are now tight enough to catch a regression.** All three test sites were checked:
   - Task 4.1.1b (`closingKeywordFor` table test): uses `assert.Equal` for exact string equality against `"Fixes "` / `"Related: "`, not a substring check — a colon reintroduced or a trailing space dropped would fail this test directly.
   - Task 4.1.2c (`BuildSessionInitialPrompt` test): explicitly calls out "not a loose `Contains(..., "Fixes")`" and asserts the exact literal substring `"Fixes https://github.com/acme/widget/issues/42"` (no colon) for the issue case and `"Related: https://github.com/acme/widget/pull/17"` (with colon) for the PR case — both punctuation directions are exercised.
   - Task 5.1.2a (integration test in `backlog_service_test.go`): same exact-literal-substring discipline, with both an issue-URL case (no colon) and a PR-URL variant (with colon) explicitly required. This is the integration-level proof the original blocker's fix needed, and it's present.

4. **No new inconsistency found.** Grepped the full plan and ADR-001 for any stale reference to the old bare-keyword (`"Fixes"`/`"Related"` with no punctuation) behavior or the old `%s: %s` caller pattern — the only remaining occurrence of `%s: %s` in the document is Task 4.1.2b's own explanatory note describing what *not* to do, which is correct and intentional, not a leftover bug. The Domain Glossary rows for `closingKeywordFor` (line 17) and "Instruction line" (line 25), ADR-001 Decision 2 (lines 76-82), and requirements.md AC3 all describe the same fully-punctuated, no-colon-for-issues / colon-for-PRs behavior consistently — there is no place in the document that still implies the caller adds punctuation or that `closingKeywordFor` returns a bare keyword.

## Original Concerns (carried forward, not re-reviewed — see prior findings)
- Shotgun-surgery risk in the two hand-built ent.BacklogItem{} literals (concern, not blocker)
- backlogItemToProto not extended with ExternalURL (concern, not blocker, now explicitly acknowledged in plan per the fix)
- anyField discipline has no compiler enforcement beyond inline comments (concern, not blocker)

## Nitpicks (carried forward)
- 500-char raw byte-slice cap could split UTF-8 mid-rune (pre-existing pattern, not a regression)
