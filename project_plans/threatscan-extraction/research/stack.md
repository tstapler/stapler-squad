# Research: Tech stack for `pkg/threatscan`

## Question 1 — Is stdlib `regexp` sufficient, or does AC 7 (fuzzy-bypass resistance) need a different approach?

**Verdict: stdlib `regexp` is sufficient. No new dependency needed.**

Go's `regexp` package (RE2 engine) supports bounded repetition — `{0,N}` — which is
exactly what's needed to tolerate filler words between key tokens. AC 7's example
("ignore all the previous silly instructions") is matched directly by a pattern like:

```go
regexp.MustCompile(`(?i)\bignore\b(?:\s+\S+){0,4}\s+\bprevious\b(?:\s+\S+){0,4}\s+\binstructions?\b`)
```

This is the same style already used throughout `pkg/classifier/classifier.go` for
multi-token command matching (e.g. `find\s+.*(-(exec|delete|ok)\b|--delete\b)` at
[classifier.go:804](/home/tstapler/Programming/stapler-squad/pkg/classifier/classifier.go#L804)),
just with a bounded gap instead of `.*`. RE2 has no backreferences/lookahead, but
none are needed for filler-word tolerance — bounded-repetition gaps are within RE2's
supported syntax and stay linear-time (no ReDoS risk), which matters because this
scans arbitrary user/LLM-supplied diff and backlog text.

**Existing precedent for token-normalization in this repo (not NLP, not a library):**
[`session/search/tokenizer.go`](/home/tstapler/Programming/stapler-squad/session/search/tokenizer.go)
implements a full lowercase → word-split → stopword-removal → Porter-stemming
pipeline in ~420 lines of pure stdlib (`strings`, `unicode`) for full-text search
relevance, with zero external dependencies. This confirms the repo's established
pattern for "fuzzy-ish" text handling is hand-rolled stdlib, not a library — if the
plan phase decides bounded-gap regex isn't expressive enough for some pattern (e.g.
needs word-order independence), the fallback technique is "tokenize + normalize +
substring/subsequence check" mirroring `tokenizer.go`'s shape, still stdlib-only.
For AC 7's stated scope (one demonstrably fuzzy-resistant pattern with a passing
test), the bounded-repetition regex is simpler, matches existing repo idiom in
`pkg/classifier`, and needs no new file — recommend that over building a token
pipeline unless the plan phase finds a specific pattern regex can't express.

## Question 2 — What dependencies are needed? Is there an existing fuzzy-matching/NLP library to reuse?

**None needed. `github.com/agext/levenshtein` exists in `go.sum` but is not a candidate for reuse.**

```
$ go mod why github.com/agext/levenshtein
github.com/tstapler/stapler-squad/session/ent/enttest
entgo.io/ent/dialect/sql/schema
ariga.io/atlas/sql/mysql
ariga.io/atlas/schemahcl
github.com/hashicorp/hcl/v2
github.com/agext/levenshtein
```

It's a transitive-only dependency pulled in by `entgo.io/ent`'s HCL/atlas schema
tooling (`go.mod:go.mod` lists it under the `// indirect` block), not something the
application intentionally added for text matching. Depending on it directly would be
fragile — an `ent` upgrade that drops the HCL dependency chain would silently break
`pkg/threatscan`'s build. No other fuzzy-matching, NLP, or Levenshtein-distance
library appears anywhere in `go.mod`. `golang.org/x/text` (which would offer
transforms/normalization) is also absent from the module graph.

**Recommendation:** `pkg/threatscan` should add zero new `go.mod` entries. This
also keeps it trivially auditable for AC 1 (no `session/`/`server/` imports) since a
zero-dependency package's import graph is easy to verify by inspection or `go list
-deps`.

## Question 3 — Structuring a small, dependency-light package: conventions from `pkg/classifier` and `pkg/events`

**File split.** Both sibling packages split by concern into small files rather than
one monolith:
- `pkg/classifier/`: `classifier.go` (core types + engine), `command_parser.go`
  (input parsing), `escalation.go` (a small supporting enum + helpers, 97 lines).
- `pkg/events/`: `bus.go` (engine), `types.go` (245 lines, just types/constants),
  `subscriber.go` (43 lines), `notification_metadata.go` (31 lines).

This matches the issue's proposed `patterns.go` / `scanner.go` / `result.go` split
exactly: `result.go` ≈ `types.go`-style (the `ThreatMatch`/`Scope` types, small),
`patterns.go` ≈ the pattern registry (data), `scanner.go` ≈ `bus.go`/`classifier.go`
(the `Scan` engine). Recommend that split.

**Pattern registry must be a constructor function, not a package-level `var` —
this is a hard constraint from `.golangci.yml`, not a style preference.**
`gochecknoglobals` is enabled repo-wide. `.golangci.yml` grandfathers exceptions
for legacy packages only:

```yaml
# gochecknoglobals: existing packages have globals from before this rule was
# introduced. Suppress blanket per-package; new packages (pkg/warren,
# pkg/events) are NOT excluded and must not introduce globals.
- path: "^pkg/analytics/"
  linters: [gochecknoglobals]
- path: "^pkg/classifier/"
  linters: [gochecknoglobals]
```

`session/backlog_review.go`'s `var secretPatterns = []struct{...}{...}` (the thing
being migrated) only compiles clean today because `session/` is globally excluded.
`pkg/threatscan` is a brand-new package and will **not** get an exclusion by
default — a package-level `var patterns = []Pattern{...}` will fail `make lint`.
Mirror `pkg/classifier`'s `SeedRules()` pattern instead:

```go
// patterns.go
func DefaultPatterns() []Pattern { return []Pattern{ /* ... */ } }
```

confirmed correct because `pkg/classifier.NewRuleBasedClassifier()` calls
`SeedRules()` (a function returning `[]Rule`) rather than referencing a package
var, specifically to satisfy this same lint constraint. (Plan-phase task: don't
add `pkg/threatscan` to the gochecknoglobals exclusion list to route around this —
follow the function-constructor pattern instead, since the repo has explicitly
marked new packages as not exempt.)

**Naming and error-message discipline.** Every `Pattern`/`Rule`-like struct in both
sibling packages carries a stable `ID`/`Name` used in output instead of the matched
data — directly matches AC 2's "named pattern IDs in output, never the matched
value" requirement; this is not a new invention, it's the same discipline already
used for `Rule.ID`/`Rule.Reason` in `pkg/classifier`.

**Test naming convention.** `TestX_should_Y_When_Z`, one `_test.go` file per source
file (`classifier_test.go`, `command_parser_test.go`, `escalation_test.go`,
`bus_test.go`, `types_test.go`), table-driven where the case count is high. The
existing test this migration must keep passing,
`TestRunPreGateSecurityCheck_should_NeverEmbedRawSecretSubstringInErrorString_When_SecretDetectedInDiff`,
already follows this convention.

**Concurrency primitive, if the scanner needs mutable shared state.**
`pkg/classifier.RuleBasedClassifier` uses `github.com/linkdata/deadlock` (already a
direct dependency, drop-in replacement for `sync.RWMutex` with deadlock detection)
for its rule-list mutex. If `pkg/threatscan`'s pattern registry needs to be mutable
at runtime (it likely doesn't — AC 6/7 only ask for a static built-in set), reuse
`deadlock.RWMutex` rather than plain `sync.RWMutex`, matching the sibling package.
For a static, immutable pattern list computed once at scan time (the likely
design, since there's no requirement for runtime rule editing like
`pkg/classifier`'s `ReplaceRules`/`AddRules`), no mutex is needed at all.

## Question 4 — Import-boundary enforcement for AC 1

AC 1 requires `pkg/threatscan` to import nothing from `session/` or `server/`,
verifiable via `go list -deps`. There is currently no automated guard for this on
any `pkg/*` package — `.golangci.yml`'s only relevant `depguard` rule
(`no_server_in_core`) scopes to `session/`, `config/`, `log/` importing `server/`,
not the reverse direction or `pkg/`. Recommend the plan phase add a matching
`depguard` rule scoped to `**/pkg/threatscan/**/*.go` denying
`github.com/tstapler/stapler-squad/session` and `.../session/**` and
`.../server` and `.../server/**`, mirroring the existing `no_server_in_core` block
verbatim but for the new package — this converts AC 1 from a one-time manual check
into a permanent CI-enforced invariant, consistent with how `no_server_in_core`
already protects `session/`.

## Summary of recommendations

1. Use stdlib `regexp` (RE2, bounded `{0,N}` repetition) for all patterns,
   including the AC 7 fuzzy-bypass example. No new dependency.
2. Do not add `github.com/agext/levenshtein` or any other library — it's a
   transitive-only dependency of the `ent`/`atlas` toolchain, not an intentional
   app dependency, and stdlib already covers the requirement.
3. Split into `patterns.go` / `scanner.go` / `result.go`, matching
   `pkg/classifier`'s and `pkg/events`'s file-per-concern convention.
4. Pattern registry must be `func DefaultPatterns() []Pattern`, not a
   package-level `var` — `gochecknoglobals` is enforced for new `pkg/*` packages
   and `pkg/threatscan` will not be grandfathered in.
5. No mutex needed for a static pattern list; if runtime mutation is ever added,
   reuse `github.com/linkdata/deadlock` (already a direct dependency) as
   `pkg/classifier` does.
6. Add a `depguard` rule in `.golangci.yml` scoped to `pkg/threatscan/**` denying
   `session`/`server` imports, to make AC 1 a permanent lint-enforced invariant
   rather than a one-time check.
