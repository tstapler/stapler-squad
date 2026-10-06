# Requirements: Extract approval gate security scanning into standalone `pkg/threatscan`

Source: [GitHub issue #120](https://github.com/TylerStaplerAtFanatics/stapler-squad/issues/120), migrated backlog item `a874e11c-e105-433e-80a7-0b496ffdc363`.

## Background

The approval gate's security scanning currently lives as 5 (actually 12 today —
see below) hardcoded `secretPatterns` in [`session/backlog_review.go`](../../session/backlog_review.go)'s
`RunPreGateSecurityCheck(diff string) error`, called once, pre-diff, from
[`session/review_gate.go:277`](../../session/review_gate.go#L277). It only catches secret-shaped
strings — nothing about prompt injection, HTML/exfiltration content, etc. As the
approval surface grows (more tool types, more backends, richer backlog fields),
the pattern set needs to evolve independently of `session/` application logic.

This is a **structural refactor** — no new scanning capability is required by
this item itself, though acceptance criteria below do ask for a starter set of
non-secret pattern categories to prove the new scope model. Companion issue
#115 (prompt-injection defense on backlog fields before LLM injection) is a
separate, dependent item — this item only needs to leave `pkg/threatscan` in a
state that #115 can consume; wiring it into `backlog_context.go` is **out of
scope** here unless trivial.

## Current state (verified against source, 2026-08-06)

- `session/backlog_review.go:20-39` — `secretPatterns` is a package-level
  `[]struct{name string; re *regexp.Regexp}` with **12** entries (aws key id,
  AKIA key, private key PEM, github PAT, openai key, stripe key, slack token,
  npm token, sendgrid key, twilio sid, generic bearer, database URL) — not 5;
  the issue text undercounts, but the description explicitly says "at minimum"
  for the new pattern additions, so the discrepancy doesn't change scope.
- `RunPreGateSecurityCheck(diff string) error` returns
  `fmt.Errorf("secret pattern detected: %s", p.name)` on first match — **never**
  includes the matched substring, only the pattern name. This no-raw-value
  contract is already covered by an existing test,
  `TestRunPreGateSecurityCheck_should_NeverEmbedRawSecretSubstringInErrorString_When_SecretDetectedInDiff`
  (see `session/backlog_review_test.go`), and is load-bearing: the frontend
  (`web-app/src/components/backlog/detail/BlockedNotice.tsx`) renders this
  error string verbatim in the UI, and `session/review_gate.go:281` embeds it
  in a stored `ReviewVerdict` summary. **The migrated package must preserve
  this contract exactly** — named pattern IDs in output, never the matched
  value — which is also explicitly required by the issue's acceptance criteria.
- Sole caller: `session/review_gate.go:277`, inside `spawnReviewGate`, gating
  before an LLM review call is spawned. On block it records a synthetic FAIL
  `ReviewVerdict` via `recordTerminalReviewVerdict` with ID prefix
  `"review-blocked-"` and feeds the auto-reopen/rework-cap machinery.
- No `session/backlog_context.go` caller exists yet — the companion issue
  (#115) that would add one is not implemented, confirmed by
  `grep -rn RunPreGateSecurityCheck` across the repo turning up only the
  `session/backlog_review.go` definition and the `session/review_gate.go`
  call site (plus docs/plan references).
- `pkg/` currently contains `analytics/`, `ansi/`, `classifier/`, `events/`,
  `warren/` — no `threatscan/` yet. These are the only precedent for
  `session/`-independent package structure in this repo; `pkg/classifier` is
  the closest analog (small, dependency-light, pattern/rule-based) and worth
  checking during research for structural conventions to mirror.

## Goal

Move secret-pattern (and new threat-pattern) scanning out of `session/` into
an independently testable, independently extensible `pkg/threatscan` package,
with zero behavior change for the existing secret-detection path and net-new
scope for prompt-injection / HTML-injection / exfiltration pattern classes.

## Acceptance Criteria

1. `pkg/threatscan` exists and imports nothing from `session/` or `server/`
   (verify via `go list -deps` or equivalent import-graph check — the reverse
   dependency, `session/` importing `pkg/threatscan`, is expected and fine).
2. Named pattern IDs appear in every error/log output from the package; the
   matched value is never included in any returned error, log line, or
   `ThreatMatch` field.
3. A `Scope` enum exists with at least `ScopeAll`, `ScopeContext`,
   `ScopeStrict` values, and callers can select scan breadth by scope.
4. All 12 existing `secretPatterns` from `session/backlog_review.go` are
   migrated into `pkg/threatscan` with equivalent detection coverage — no
   regression on any string the current patterns catch today.
5. `session/backlog_review.go`'s `RunPreGateSecurityCheck` (or its
   `session/review_gate.go:277` call site) is updated to call into
   `pkg/threatscan` for its scan, using strict scope on the diff, while
   preserving its existing signature/behavior contract (returns non-nil
   `error` on first match, error string never contains the raw matched
   value, `"review-blocked-"` verdict-recording flow in `review_gate.go`
   untouched).
6. New pattern categories are added to `pkg/threatscan` covering, at minimum:
   classic prompt injection (e.g. `ignore.*previous.*instructions`, "system
   prompt override" phrasing), hidden HTML elements (`display:none`, HTML
   comment injection), role-play/identity-hijack phrasing, and exfiltration
   signals (`send.*to.*http`, `curl.*webhook`). These new patterns are scoped
   appropriately (e.g. `ScopeStrict`/`ScopeContext`, not necessarily wired
   into `RunPreGateSecurityCheck`'s existing secret-only call site).
7. At least one pattern demonstrates fuzzy-bypass resistance — matching even
   with filler words inserted between key tokens (e.g. "ignore all the
   previous silly instructions") — with a passing unit test proving it.
8. Unit test coverage in `pkg/threatscan` includes: direct pattern match,
   fuzzy-word-insert bypass, HTML injection, and a false-positive guard test
   asserting no match fires on a realistic legitimate `AGENTS.md`-style
   content sample.
9. `go build ./...`, `make test`, and `make lint` all pass after the
   migration with no new failures introduced.
10. The existing test
   `TestRunPreGateSecurityCheck_should_NeverEmbedRawSecretSubstringInErrorString_When_SecretDetectedInDiff`
   (or its post-migration equivalent, if the function moves) continues to
   pass unmodified in intent.

## Explicitly out of scope

- Wiring a new scan call into `session/backlog_context.go` for backlog-field
  scanning ahead of LLM injection (companion issue #115) — this item only
  needs to leave the package in a state #115 can adopt without further
  restructuring.
- Any change to the review-gate's auto-reopen/rework-cap/notification
  behavior beyond what's needed to keep `RunPreGateSecurityCheck`'s contract
  identical.
- New scanning capability beyond the "at minimum" pattern categories listed
  in AC 6 — this is a refactor with a modest, bounded coverage addition, not
  an open-ended threat-detection feature.

## Open questions for research/plan phases

- Should `RunPreGateSecurityCheck` become a thin wrapper in `session/` that
  calls `pkg/threatscan.Scan(diff, threatscan.ScopeStrict)` and re-formats
  the first match into the existing error string, or should callers migrate
  directly to the new `Scan`/`ThreatMatch` API? (Affects whether
  `review_gate.go` needs to change at all vs. only `backlog_review.go`.)
- Exact `ThreatMatch` field set and `Scan` signature per the issue's proposed
  `patterns.go` / `scanner.go` / `result.go` split — validate against
  `pkg/classifier`'s existing structure for consistency.
- Whether scope tagging is per-pattern (a pattern belongs to one or more
  scopes) or the scan call filters a shared pattern list by scope at query
  time — both satisfy the acceptance criteria; pick whichever keeps the
  pattern registry a single source of truth.
