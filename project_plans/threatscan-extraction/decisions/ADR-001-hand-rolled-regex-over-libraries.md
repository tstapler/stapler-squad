# ADR-001: Hand-rolled stdlib `regexp` patterns, not a third-party library

Status: Accepted
Date: 2026-08-06
Related: `project_plans/threatscan-extraction/requirements.md`, `research/build-vs-buy.md`

## Context

`pkg/threatscan` needs to solve three sub-problems: secret detection (12
existing patterns), prompt-injection/HTML-injection/exfiltration detection (4
new categories, AC6), and filler-word-tolerant "fuzzy" matching (AC7). Each
sub-problem has an existing open-source Go library that could plausibly be
adopted instead of writing/maintaining regexes by hand:
[gitleaks](https://github.com/gitleaks/gitleaks) (secret detection),
[go-promptguard](https://github.com/mdombrov-33/go-promptguard) (prompt
injection), and Levenshtein-distance libraries such as
`github.com/agext/levenshtein` (already present in `go.sum`, transitively) for
fuzzy matching.

## Decision

Build all patterns by hand using stdlib `regexp` only. Add no new `go.mod`
dependency for any of the three sub-problems.

## Rationale

- **gitleaks' `detect.Finding` returns the matched secret text by design** —
  its whole purpose is showing the user what leaked. This item's AC2 is the
  inverse: the matched value must never appear in any returned error, log
  line, or `ThreatMatch` field, verified by an existing test
  (`TestRunPreGateSecurityCheck_should_NeverEmbedRawSecretSubstringInErrorString_When_SecretDetectedInDiff`).
  Adopting gitleaks would mean importing an API whose entire contract runs
  opposite to a security invariant this package must uphold, then wrapping
  every call to redact what the library deliberately surfaces — more
  integration code than the 12 regexes it would replace. It also pulls ~34
  transitive imports (git-history walking, TOML config parsing, syntax
  highlighting) for capability this package doesn't need, since it scans an
  in-memory diff/text string, not a git repository or file tree.
- **go-promptguard is the only Go-native prompt-injection library found**, and
  it's a single-maintainer, unestablished project whose optional LLM-judge
  mode would add latency this call site (`session/review_gate.go:277`,
  invoked synchronously before every review-gate spawn) has no budget for.
  Industry practice (surveyed commercial guardrail products, Aug 2026) still
  treats pattern/heuristic matching as the standard *first* line of defense
  ahead of ML/LLM classifiers, not something with a mature off-the-shelf
  replacement — this item isn't reinventing a solved problem.
- **Levenshtein-distance libraries solve character-level typo tolerance**
  ("recieve" ~ "receive"), not AC7's actual requirement: phrase-level
  filler-word insertion ("ignore all the previous silly instructions" vs.
  "ignore previous instructions"). Feeding that pair through edit distance
  produces a large distance that doesn't threshold cleanly against unrelated
  text without heavy tuning. `github.com/agext/levenshtein` is also only a
  transitive dependency of `entgo.io/ent`'s HCL/atlas schema tooling — not an
  intentional application dependency — so depending on it directly would be
  fragile against an unrelated `ent` upgrade dropping that chain.
- **Precedent already exists in this repo**: `pkg/classifier/classifier.go`
  uses plain stdlib `regexp` for its own rule-based tool-use classifier,
  including the same bounded-repetition idiom this item needs
  (`find\s+.*(-(exec|delete|ok)\b|--delete\b)`). A bounded filler-gap pattern
  — `key1\s+(?:\S+\s+){0,N}key2` — directly encodes AC7's requirement with
  zero new dependencies and matches the codebase's established convention for
  this class of problem.
- **The "never log the matched value" contract is idiosyncratic to this
  codebase's frontend/verdict-rendering path.** No surveyed library was
  designed around that guarantee; adopting one would still require wrapping
  it to enforce an invariant it doesn't natively provide.

## Consequences

- The 12 secret patterns and the ~7 new patterns are hand-maintained inside
  `pkg/threatscan/patterns.go`, not sourced from an upstream, community-curated
  rule set. If AC6's bounded scope later grows into an open-ended
  threat-detection feature (explicitly out of scope for this item), this
  decision should be revisited — gitleaks' rule breadth becomes a real
  advantage once the pattern set outgrows a dozen well-known credential
  shapes.
- ReDoS is a non-issue regardless of this decision: Go's `regexp` package is
  RE2-based (linear-time, no backtracking engine), so the bounded-repetition
  patterns this ADR commits to carry no catastrophic-backtracking risk even
  under adversarial input.
- No new `go.mod` entry keeps `pkg/threatscan`'s import graph trivially
  auditable for AC1 (no `session`/`server` imports) — a zero-dependency
  package's dependency graph is easy to verify by inspection or `go list
  -deps`, which is also why the plan adds a permanent `depguard` rule rather
  than relying on manual review alone.
