# Research: Common pitfalls for this refactor + feature

Scope: what typically goes wrong (a) extracting `RunPreGateSecurityCheck` out of
`session/backlog_review.go` into `pkg/threatscan`, and (b) adding regex-based
fuzzy-bypass prompt-injection/exfiltration detection. Verified against this
repo's actual code, not generic advice.

## 1. Refactor risks

### 1.1 Error string is contractually exact, not just semantically preserved

`session/backlog_review_test.go:548-554`
(`TestRunPreGateSecurityCheck_should_NeverEmbedRawSecretSubstringInErrorString_When_SecretDetectedInDiff`)
asserts the literal substring `"secret pattern detected: " + tc.name` is
present in the error's `%v` text — not just "no raw secret present". If the
migrated code changes the format to e.g. `"threat detected: %s"` or
`"threatscan: secret pattern %q matched"`, this test breaks even though the
no-raw-value contract is still upheld. Either:
- keep `RunPreGateSecurityCheck` a thin wrapper in `session/` that re-formats
  `pkg/threatscan`'s result into the exact legacy string (`"secret pattern
  detected: %s"`), or
- update the test deliberately and confirm no other consumer string-matches
  on the old format (checked: `web-app/src/components/backlog/detail/
  BlockedNotice.tsx` renders the `review-blocked-*` session's stored
  `ReviewVerdict.Summary` verbatim via `role=status`, it does not pattern-match
  the error text itself — see `BlockedNotice.tsx:55`. `ReviewingSection.tsx`
  and `sessionKind.ts` only match on the `"review-blocked-"` **session ID
  prefix**, not the error string. So the error string's *exact wording* is
  only load-bearing for the one Go test above, not for the frontend — but
  the test itself is explicit "must equal", so treat it as load-bearing
  until intentionally changed).

### 1.2 The `"review-blocked-"` UUID-prefix flow must stay untouched

`session/review_gate.go:281-282` builds the summary as `fmt.Sprintf("Review
blocked by security check: %v. Override required to proceed.", secErr)` and
calls `recordTerminalReviewVerdict(..., "review-blocked-"+uuid.New().String(),
ReviewVerdictFail, summary)`. This is a second wrapping layer around the
error text (AC 5 preserves this untouched) — the migration only needs
`secErr` (from the new `RunPreGateSecurityCheck` wrapper) to still be a
non-nil `error` whose `%v` never contains the matched value. Don't touch
`review_gate.go`'s summary-building or the `"review-blocked-"` prefix
convention itself; both are relied on by `web-app/src/lib/backlog/
sessionKind.ts:33` and three frontend test files.

### 1.3 Import-cycle direction is fine but verify it explicitly

`pkg/threatscan` must import nothing from `session/` or `server/` (AC 1).
Current `pkg/` packages (`analytics`, `ansi`, `classifier`, `events`,
`warren`) are all leaf packages with no `session/` imports — `pkg/classifier`
is the closest structural analog (regex-based rule classifier, already uses
just `regexp`, `context`, `os`, `sort`, `strings`, `time`,
`executor/safeexec`, `github.com/linkdata/deadlock` — no `session/` import).
Mirror that: `pkg/threatscan` should depend on nothing beyond stdlib (maybe
`regexp`, `strings`, `fmt`). `session/backlog_review.go` importing
`pkg/threatscan` is the expected one-way edge. Verify with `go list -deps
./pkg/threatscan/...` showing no `session/` entry, per AC 1's own suggested
check — don't just eyeball the import block.

### 1.4 Coverage gap during the move: table-driven cases must migrate 1:1, not be summarized

The 12 existing patterns each have specific edge-case shapes (e.g.
`AKIA[0-9A-Z]{16}` requires exactly 16 trailing chars, `sk-` OpenAI key
requires exactly 48 chars, `stripe_secret_key` allows `{24,}` open-ended).
A common failure mode when "porting" patterns is re-deriving them from memory
or tightening/loosening quantifiers slightly (e.g. changing `{24,}` to a
fixed length) which silently changes match behavior without any test failure
if the new test suite doesn't reuse the *exact* fixture strings from
`backlog_review_test.go`'s existing table tests. Port the regex literals
verbatim (copy-paste, not retype) and reuse the existing fixture value list
(`AKIA1234567890ABCDEF`, `ghp_`+36 a's, `sk-`+48 b's, `sk_live_`+24 c's,
`postgres://admin:s3cr3t-p4ss@db.internal/prod`, etc. from
`backlog_review_test.go:539-543`) as regression fixtures in the new package,
plus at least one deliberate near-miss per pattern (one char short of the
required length) to prove the boundary wasn't loosened.

### 1.5 Diff truncation happens *before* the security check runs, not after

`session/review_gate.go:158/164/183` computes the diff via `GetGitDiff`/
`GetGitDiffRef`, which internally truncates to `headless.MaxDiffSizeReview`
(40,000 bytes, `session/headless/features.go:59-60`) **before**
`RunPreGateSecurityCheck(diff)` is called at `review_gate.go:277`. This is
pre-existing behavior (not introduced by this refactor) but worth naming
explicitly: a secret or injection payload placed past byte 40,000 of a large
diff is invisible to the scanner today, and will remain invisible after
migration unless this item's plan phase decides to change the ordering
(explicitly out of scope per the requirements doc — AC 5 says preserve
behavior exactly). Don't "fix" this as a drive-by during the refactor; note
it as a known pre-existing gap if it comes up in review.

## 2. Feature risks (fuzzy-bypass regex patterns)

### 2.1 ReDoS is NOT a real risk here — Go's `regexp` is RE2-based

The requirements/prompt raises catastrophic backtracking on patterns like
`ignore.*previous.*instructions` against adversarial input as a concern.
**This does not apply to Go's standard `regexp` package.** Go's `regexp` is
built on RE2, which guarantees worst-case **linear time** in input length
regardless of pattern complexity — there is no backtracking engine to
exploit, unlike PCRE/Python `re`/JavaScript `RegExp`. Confirmed: every
existing pattern in `secretPatterns` and any new `regexp.MustCompile`-based
fuzzy pattern (e.g. `ignore(?:\s+\S+){0,5}?previous(?:\s+\S+){0,5}?
instructions`) compiles to an RE2 automaton with linear-time guarantees. The
one thing to still watch is **not** catastrophic backtracking but plain
**quadratic blowup from too many patterns × large input** (12 patterns today,
more after this item, each doing a full linear scan of the diff) — at 40KB
max diff size this is negligible, but if `pkg/threatscan` is later wired into
an unbounded-size scan target (e.g. #115's backlog-field scanning, out of
scope here) that's a real sizing question, not a regex-engine one. Don't
spend implementation time defending against ReDoS; do consider capping input
size defensively in `Scan()` itself so the package is safe to reuse from a
future caller that doesn't pre-truncate.

### 2.2 False positives on legitimate meta-content describing the patterns

This item's own `requirements.md` and this pitfalls doc both contain literal
example strings like `ignore.*previous.*instructions`, `display:none`,
`curl.*webhook` as pattern *names/examples in prose*, not attacks. If
`pkg/threatscan` scope ever gets pointed at markdown docs, PR descriptions,
or backlog item text (AC 6/7 patterns are `ScopeStrict`/`ScopeContext`, "not
necessarily wired into `RunPreGateSecurityCheck`'s existing secret-only call
site" — so this is a forward-looking risk, not an immediate one for AC 5's
call site which stays diff-only). AC 8 requires a false-positive guard test
against "a realistic legitimate AGENTS.md-style content sample" — this
repo's own `AGENTS.md`/`CLAUDE.md` (root of repo) are good real fixtures:
they discuss security review, secret scanning, and hooks in prose without
containing actual injection payloads. Use excerpts from those files (or this
research doc's own prose) as the false-positive fixture rather than inventing
synthetic "legitimate" text — real repo docs are more representative of what
will actually be scanned if #115 later wires scope-context scanning into
backlog fields.

Fuzzy/word-insertion patterns are the primary false-positive amplifier: a
naive `ignore(?:\s+\w+){0,4}\s+previous(?:\s+\w+){0,4}\s+instructions` will
also match incidental prose like "please ignore the previous draft's
instructions, use the new ones" in a normal PR description. Keep the filler
gap small (2-4 words, not unbounded `.*`) and anchor on the specific
imperative phrasing combination (verb+object pairs), not single common words,
to keep the false-positive rate manageable — and cover this explicitly with
a "looks similar but is legitimate" test case per new pattern category, not
just a "does it match the attack" test.

### 2.3 "Good enough" coverage is the named risk for security-critical code

AC 8 lists 4 required test categories (direct match, fuzzy-bypass, HTML
injection, false-positive guard) as a floor, not a ceiling. Given
`RunPreGateSecurityCheck`'s no-raw-value contract is independently
regression-tested today (section 1.1), the same rigor should apply to every
new pattern category: at minimum, one positive match test, one near-miss
(should NOT match) test, and confirmation the match reporting path (whatever
`ThreatMatch` looks like) never carries the raw matched substring — mirroring
`TestRunPreGateSecurityCheck_should_NeverEmbedRawSecretSubstringInErrorString_When_SecretDetectedInDiff`
for every new category, not just the migrated secret patterns. Don't let "12
secret patterns get 1:1 migration tests" crowd out equal rigor for the net-new
prompt-injection/HTML/exfiltration categories, which are the actual new
attack surface this item adds.

## Summary of what to carry into the plan phase

1. Decide explicitly (open question in requirements.md): `RunPreGateSecurityCheck`
   stays a thin `session/`-local wrapper that reformats `pkg/threatscan`'s
   result into the byte-exact legacy string — this is the lowest-risk option
   given 1.1's exact-string test.
2. Port the 12 regexes and their existing test fixtures verbatim; add one
   below-threshold near-miss fixture per pattern as an explicit regression
   guard against silent quantifier drift.
3. Do not budget time for ReDoS mitigation (Go's RE2 engine already
   guarantees linear time) — redirect that effort into false-positive
   guard tests using this repo's real `AGENTS.md`/`CLAUDE.md` as fixtures.
4. Keep fuzzy-match filler gaps small and anchored (2-4 words) to bound
   false-positive rate; add a "similar but legitimate" test per new pattern.
5. Note (don't fix) that the 40KB diff truncation happens before the
   security scan runs — out of scope per AC 5, but worth a one-line callout
   in the PR description so reviewers don't assume this item closes that gap.
