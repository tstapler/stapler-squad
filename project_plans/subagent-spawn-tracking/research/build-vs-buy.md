# Build vs. Buy: subagent-spawn-tracking

## Verdict: build, stdlib-only. No library, no SaaS, no novel algorithm.

This feature is "match an existing regex's capture group, `strconv.Atoi` it, put it on a
proto field, badge it in React." Every one of the four evaluation dimensions below resolves
to the same conclusion, so this write-up stays short rather than manufacturing tradeoffs
that don't exist.

### 1. Existing OSS library or framework — not needed, and no internal helper to reuse either

Go stdlib `regexp` (named capture group `(?P<count>\d+)` + `match[re.SubexpIndex("count")]`
or a plain `FindStringSubmatch` positional group) is sufficient — no third-party regex engine
adds anything here (no lookahead/lookbehind requirement, no Unicode-script matching, no
performance profile that stdlib `regexp`'s RE2 engine can't handle at terminal-scrollback
scale).

Checked for an existing internal "extract int from regex match" helper that should be reused
instead of writing a new one (`grep -rn strconv.Atoi` near regexp call sites, repo-wide). None
exists as a shared helper — every call site inlines its own `FindStringSubmatch` +
`strconv.Atoi`:

- `session/git/worktree_git.go:372-375` — `if m := prNumberFromURLRe.FindStringSubmatch(prURL); m != nil { if n, convErr := strconv.Atoi(m[1]); convErr == nil && n > 0 { prNumber = n } }`
- `server/dependencies.go:1304-1305` — `if m := prNumFromTitle.FindStringSubmatch(inst.Title); m != nil { prNumber, _ = strconv.Atoi(m[1]) }`
- `github/url_parser.go:299,354` — `prNum, err := strconv.Atoi(matches[4])`
- `session/storage_backlog.go:802-804`, `session/pty_discovery.go:807`, `session/mux/picker.go:65` — same shape

This is a consistent, repeated idiom (5+ call sites), not a candidate for a new shared
utility either — each is a 2-3 line inline block and extracting a generic helper would be the
"unjustified generic" smell from `.claude/rules/interface-pollution-checklist.md` (single-use
abstraction over a trivial operation). Write the new capture-group parse inline, following the
same shape.

### 2. SaaS / managed API — not applicable

This is 100% local terminal-scrollback text processing (`session/detection/pattern_set.go`,
`detector.go`). There is no network call, no external service, and nothing to outsource — a
managed API would add latency and an operational dependency for work regexp already does
in-process in microseconds.

### 3. LLM-generated implementation vs. battle-tested library — N/A / trivial for this size

The "algorithm" is: match a capture group, parse it as an int, clamp/default on parse failure.
This is stdlib-level work with no meaningful correctness surface to battle-test — there's no
edge case here that a unit test with a table of terminal-output fixtures (the existing pattern
in `session/detection/*_test.go`) doesn't fully cover. Framing this as "LLM-written code vs.
proven library" would be inventing a comparison the feature doesn't warrant; skip it.

### 4. Fork or adapt — yes, an in-repo idiom already exists; copy it, don't reinvent

Two existing patterns in `session/detection/` and `session/git/` already do "extract numeric
capture group from regex match" and should be the template:

- **`session/detection/approval.go:264-275`** (`extractCaptureGroups`) — takes a
  `FindStringSubmatch` result plus a list of named `CaptureKeys` and maps them into a
  `map[string]string`, used inline at `approval.go:205-213` right where patterns are matched
  against scrollback lines. This is the closest structural precedent: it already lives in the
  detection package this feature will extend, and `ApprovalPattern.CaptureKeys` already
  demonstrates naming/registering a capture group in a `StatusPattern`-like struct.
- **`session/git/worktree_git.go:372-375`** — the tightest idiom for the actual int-parsing
  step: `FindStringSubmatch` → nil-check → `strconv.Atoi` → guard `convErr == nil && n > 0`
  before trusting the value, with a documented fallback path if parsing fails. This
  nil-check-then-guarded-Atoi shape is exactly what a subagent count parse should follow.

**Recommendation:** implement the count capture in the same style as `approval.go`'s
match-and-extract loop (since detection already iterates compiled patterns per line there),
using `worktree_git.go`'s guarded-`Atoi` pattern for the numeric conversion. No new package,
no new dependency, no shared helper worth extracting.
