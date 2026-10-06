# Architecture Research: `pkg/threatscan` extraction

Scope: this is a mechanical extraction of an existing, fully-scoped function
(`RunPreGateSecurityCheck`) plus a bounded pattern-set addition — not a
multi-actor domain, so no EventStorming table. Findings below apply
`.claude/rules/interface-pollution-checklist.md` directly to the two open
questions in `requirements.md`.

## Current call graph (verified)

- `session/backlog_review.go:20-52` — package-level `secretPatterns`
  (`[]struct{name string; re *regexp.Regexp}`, 12 entries) +
  `RunPreGateSecurityCheck(diff string) error`, both `package session`,
  imported only within `session/`.
- Sole caller: `session/review_gate.go:277`, inside `spawnReviewGate`
  (confirmed by grep — no other call site exists anywhere in the repo).
  ```go
  // session/review_gate.go:273-277
  // Security check — block if secrets detected.
  //
  // Same as the diff-error block above: this records a synthetic, non-Instance-backed
  // terminal verdict directly — it's a pre-flight guardrail, not the review call itself.
  if secErr := RunPreGateSecurityCheck(diff); secErr != nil {
  ```
  On error it formats `summary := fmt.Sprintf("Review blocked by security check: %v. Override required to proceed.", secErr)`,
  records a synthetic FAIL `ReviewVerdict` via `recordTerminalReviewVerdict`
  with ID prefix `"review-blocked-"`, notifies, and feeds
  `AutoReopenAfterFailedReview` — none of that machinery needs to change.
- Prior research context (as pointed to by the task): `project_plans/backlog-item-detail-ux/research/architecture.md:46`
  and `project_plans/backlog-cross-platform-audit/research/implementation-inventory.md:22`
  both independently describe `RunPreGateSecurityCheck` as a **pre-flight
  guardrail** — a synchronous check before the LLM review call, not part of
  the review call itself, recording a terminal FAIL verdict directly rather
  than going through the headless review path. Nothing in either doc implies
  additional callers or a different contract than what's in the current
  source; both confirm the single-call-site, guardrail-shaped behavior
  assumed above.
- Contract that must survive unchanged: `TestRunPreGateSecurityCheck_should_NeverEmbedRawSecretSubstringInErrorString_When_SecretDetectedInDiff`
  (`session/backlog_review_test.go:533`) asserts the returned error's `%v`
  text is exactly `"secret pattern detected: <name>"` and never contains the
  matched substring — this is consumed verbatim by
  `web-app/src/components/backlog/detail/BlockedNotice.tsx` and embedded in
  the stored `ReviewVerdict` summary, so the string shape is a real UI/data
  contract, not an implementation detail.

## Precedent: `pkg/classifier`

`pkg/classifier/classifier.go` defines an interface (`Classifier`) with
**one** implementation (`RuleBasedClassifier`) — by the letter of the
checklist this is itself a "speculative interface," but it's pre-existing
code, not a pattern to imitate here. The requirements doc already prescribes
a concrete, function-based API (`patterns.go` / `scanner.go` / `result.go`,
`Scan(content string, scope Scope) []ThreatMatch`) — that shape is correct
independent of what `classifier` did: `threatscan`'s scan logic is stateless
regex matching over a package-level pattern table, with no second
implementation (e.g. an ML-based scanner) imminent, so no `Scanner` interface
should be introduced. Use plain package functions, matching `pkg/analytics`'s
style (plain functions, no top-level interface) more than `pkg/classifier`'s.

## Answering the two open questions

### 1. Thin wrapper in `session/`, or drop `RunPreGateSecurityCheck` and call `pkg/threatscan` directly from `review_gate.go`?

**Keep `RunPreGateSecurityCheck` as a thin wrapper in `session/backlog_review.go`.**

This is *not* the "forwarding-only wrapper" smell (checklist item 4) because
it does real, non-trivial work at the boundary: translating a generic
`[]threatscan.ThreatMatch` into the exact legacy string
`"secret pattern detected: <name>"` that two external consumers
(`BlockedNotice.tsx` and the stored `ReviewVerdict.summary`) already depend
on verbatim. That translation is a `session/`-owned UX/data-contract concern
— `pkg/threatscan` has no reason to know about review-gate's error-string
convention, and encoding it there would leak a `session/`-specific format
into a package AC1 requires to have zero knowledge of `session/`. Collapsing
the wrapper into `review_gate.go` would just relocate the same formatting
line one call frame up for no benefit, while forcing `review_gate.go` to
import `threatscan.ThreatMatch` directly and re-derive the "first match wins,
never embed the value" rule at the call site instead of once, tested, in
`backlog_review.go`.

Concretely:
```go
// session/backlog_review.go — after extraction
func RunPreGateSecurityCheck(diff string) error {
    matches := threatscan.Scan(diff, threatscan.ScopeStrict)
    if len(matches) > 0 {
        return fmt.Errorf("secret pattern detected: %s", matches[0].PatternID)
    }
    return nil
}
```
`review_gate.go:277` needs **zero changes** — same function name, same
signature, same error text shape. This satisfies AC5 and AC10 exactly
(existing test keeps passing "unmodified in intent" — it calls
`RunPreGateSecurityCheck` by name, which still exists with the same
behavior).

This does mean `session/backlog_review.go` keeps a package-level function
that is a genuine (if thin) adapter, not a re-export — that's the correct
side of the checklist's line: a wrapper is a smell when it adds *no*
behavior, not when it adapts one contract to another that has independent
consumers.

### 2. `ThreatMatch` field set / `Scan` signature

Per requirements' proposed split, keep it concrete and minimal — no
interface, no generic:

```go
// pkg/threatscan/result.go
package threatscan

type ThreatMatch struct {
    PatternID string // stable, named ID — e.g. "aws_access_key_id" — never the matched text
    Scope     Scope
    Excerpt   string // caller-controlled context (e.g. surrounding line), NEVER the raw matched substring itself
}
```
`Excerpt` needs a documented invariant (mirrored in a test, per AC2/AC8):
it must never be built from the regex match itself — at most surrounding
non-matching context, if populated at all. Given AC2's "never included in
any returned error, log line, or `ThreatMatch` field," the safest initial
implementation is to leave `Excerpt` empty until a concrete consumer
(e.g. #115) needs it, rather than half-populating it and hoping every future
caller respects the invariant.

```go
// pkg/threatscan/scanner.go
package threatscan

func Scan(content string, scope Scope) []ThreatMatch { ... }
```
Plain function, not a method on a struct — there's no per-call configuration
or state (no rule mutation like `RuleBasedClassifier.AddRules`), so a struct
would be an unjustified layer (checklist item 6 territory: a
struct-wrapping-a-slice with no added behavior).

### Scope tagging: per-pattern, not per-call filter list

```go
// pkg/threatscan/patterns.go
package threatscan

type Scope int

const (
    ScopeStrict Scope = iota // secrets — high-confidence, low-noise; used by RunPreGateSecurityCheck
    ScopeContext              // prompt-injection / role-play / HTML-injection signals for backlog-field scanning (#115)
    ScopeAll                  // union — matches AC3's minimum set (ScopeAll, ScopeContext, ScopeStrict)
)

type pattern struct {
    id     string
    scopes []Scope // e.g. []Scope{ScopeStrict, ScopeAll} for a secret pattern
    re     *regexp.Regexp
}

var patterns = []pattern{ /* 12 migrated secret patterns, each tagged {ScopeStrict, ScopeAll} */
    /* + new prompt-injection/HTML/exfiltration patterns tagged {ScopeContext, ScopeAll} */
}
```
Per-pattern scope tags (not a scan-time filter list passed by the caller)
keep the pattern registry the single source of truth per the requirements'
own framing — `Scan(content, scope)` just filters `patterns` by
`slices.Contains(p.scopes, scope) || scope == ScopeAll`-equivalent logic
internally. This avoids a second, parallel list-of-lists a maintainer could
forget to update when adding a pattern.

## Integration points (confirmed complete list)

1. `session/backlog_review.go:20-52` — delete `secretPatterns`, reimplement
   `RunPreGateSecurityCheck` to call `threatscan.Scan(diff, threatscan.ScopeStrict)`
   (shown above). Add the `github.com/tstapler/stapler-squad/pkg/threatscan`
   import.
2. `session/review_gate.go:277` — **no code change**; only benefits from
   `backlog_review.go`'s import-graph change transitively.
3. `session/backlog_review_test.go:533` — no change required; still tests
   `RunPreGateSecurityCheck` directly and should pass unmodified.
4. No other call sites exist (grep-confirmed) — `session/backlog_context.go`
   (companion issue #115's future integration point) is explicitly out of
   scope for this item.

## Data-flow / consistency requirements

- **Never-embed-raw-value contract** must hold at two layers now instead of
  one: `pkg/threatscan.Scan` itself must never put matched text into
  `ThreatMatch.Excerpt` (new invariant, AC2, needs its own test in
  `pkg/threatscan`), *and* `RunPreGateSecurityCheck`'s translation to
  `fmt.Errorf` must keep using only `PatternID` (existing invariant,
  re-verified by the existing session-level test — AC10).
- **Import direction**: `pkg/threatscan` must not import anything from
  `session/` or `server/` (AC1). The wrapper function in `backlog_review.go`
  is the only place the dependency edge is `session/ → pkg/threatscan`;
  verify with `go list -deps ./pkg/threatscan/...` showing no
  `stapler-squad/session` or `stapler-squad/server` entries.
- **Behavior parity**: all 12 existing patterns must move with identical
  regex source and identical `name`→`PatternID` string (AC4) — the frontend
  and stored-verdict consumers don't care about the ID text changing, but
  nothing requires changing it either, so don't (minimizes diff, avoids
  accidentally invalidating anything that greps historical verdict summaries
  for a specific pattern name).

## Summary of the pattern applied

This is the checklist's item-4 case worked in the direction *away* from
deletion: a wrapper survives extraction not because "some session code calls
it" but because it does real translation work between two independently-
consumed contracts (a new internal `ThreatMatch` API vs. an existing external
string format). The extraction itself follows item-1/item-6 by using
concrete functions and a plain struct/slice registry in `pkg/threatscan`,
with no interface introduced on either side of the boundary.
