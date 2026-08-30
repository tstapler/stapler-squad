# Pitfalls Research: Context-Compaction Detection

Phase 2 (Pitfalls Research) for `project_plans/context-compaction-detection/`. Findings below are
grounded in direct reads of the current detection pipeline, proto definitions, CI workflows, and
lint config — file:line citations throughout.

## 1. Regex-based terminal-output detection pitfalls

### 1a. "Priority" is decorative, not functional — the real ordering is a hardcoded category chain

`StatusPattern.Priority` (`session/detection/dtypes/dtypes.go:11`) looks like it controls match
order, and every existing pattern in `session/detection/detector.go` and
`session/detection/binaries/claude.go` has a `Priority: N` field populated (25, 26, etc.). **It is
never read anywhere outside test/YAML-serialization code** — confirmed via
`grep -rn '\.Priority\b' session/detection/*.go` (excluding `_test.go`) returning zero hits. The
actual match order is a fixed sequence of category checks hardcoded in
`PatternSet.MatchLines` (`session/detection/pattern_set.go:69-141`):

```
Error → TestsFailing → NeedsApproval → InputRequired → readline_typing (special-cased) →
WaitingForAgent → Success → Active → Processing → screen_overwrite fallback → Idle → Ready
```

Within a category, patterns are checked in **slice declaration order** (`for i, regex := range
ps.errorRegexes`), not sorted by `Priority`. `TestStatusDetector_PriorityOrder`
(`session/detection/detector_test.go:278-312`) actually asserts this fixed *category* ordering
(e.g. "Error > Processing", "NeedsApproval > Active"), not the `Priority` field.

**Implication for this feature**: adding a "Compacting" category means deciding where it slots
into this hardcoded chain in `pattern_set.go`, not just picking a `Priority` number. Setting a
high `Priority` value on the new pattern (as the plan doc might naively do, mirroring existing
patterns) will have **zero effect** on match precedence — it's cosmetic. The plan phase must pick
an explicit insertion point in `MatchLines` and justify it (compaction should very likely rank
above `Active`/`esc_to_interrupt`, since a compacting session still shows "esc to interrupt" in
its status line per the existing fixtures at `testdata/claude_active.txt:5`, but should probably
rank below `Error`/`NeedsApproval`/`InputRequired` since those need to win if compaction message
happens to co-occur with a real error line in the tail).

### 1b. Overlap with the existing "N% until auto-compact" pattern space

Confirmed via `grep -n "auto-compact" session/detection/testdata/*.txt`: three existing fixtures
(`claude_active.txt:5`, `claude_thinking_verb.txt:5`, `claude_asterism_active.txt:5`) all contain
`esc to interrupt   10% until auto-compact` and are asserted to resolve to `StatusExecuting`/
`StatusProcessing`. The *actively compacting* string (still unconfirmed per requirements.md's Open
Question) must be written narrowly enough not to also match this "approaching" indicator, or every
existing fixture with `N% until auto-compact` becomes a regression risk the moment the new pattern
is added — directly threatening Acceptance Criterion 7 ("no existing fixture's classification
changes"). Any candidate regex needs a negative-case test against these three fixtures specifically.

### 1c. ANSI stripping happens, but a compaction message split across PTY read chunks won't compose across `Detect()` calls

`PTYNormalizer.Normalize` (`session/detection/normalizer.go:11-13`) runs `stripANSI` +
`collapseCarriageReturns` before any pattern match, and `stripANSI` (`detector.go:141-144`) does
handle CSI/OSC/charset-designator escapes and CSI cursor-forward-as-space — so color codes and
cursor-positioned spacing are already covered for whatever new pattern is added. **However**, each
call to `Detect`/`DetectWithContext` operates on a single `output []byte` snapshot (typically the
last `StatusDetectionTailBytes` = 4096 bytes, `detector.go:241`) — there is no cross-call
buffering. If Claude Code's compaction-in-progress string is written across two separate PTY reads
that straddle a poll interval, or if it scrolls off the 4096-byte tail window before the next poll
captures it, detection can simply miss the state for that cycle. Given the reported 30-60s
duration (requirements.md line 12), this is a low risk *for this specific message* — a single
short-lived status-bar line re-rendered every spinner frame is very likely to be present in tail
snapshot windows throughout the 30-60s window — but it's a real risk if the compaction indicator
turns out to be a single one-shot line print rather than a persistently-redrawn status line. This
should be resolved empirically during ACT 1 (capturing the literal string) rather than assumed.

## 2. Proto enum pitfalls — additive-only append is required and lightly enforced

`proto/session/v1/types.proto:380-393` defines `DetectedStatus` with values 0-11
(`DETECTED_STATUS_UNSPECIFIED` through `DETECTED_STATUS_WAITING_FOR_AGENT`). The next value must
be `12`, appended at the end — never inserted or renumbered. This repo has a documented precedent
for this exact discipline in `.claude/rules/session-creation-registry.md` ("Add next value here"),
though there is no repo-wide ADR specifically titled "proto enum compatibility"; the practice is
established by that rule plus `buf.yaml`.

**Enforcement level, checked directly**:
- `buf.yaml:14-19` sets `breaking: use: [FILE]` with an exception for `FIELD_SAME_NAME`, annotated
  `# First version, no breaking changes to check`. `buf breaking` is **not invoked anywhere in
  CI** — `grep -rn "buf breaking"` across `.github/workflows/*.yml` and the `Makefile` returns
  nothing; only `buf lint proto`, `buf build proto`, and `buf generate proto` run
  (`Makefile:406,427,430`, `.github/workflows/lint.yml:66-67`). So renumbering or removing an
  enum value would **not be caught by CI** — the breaking-change config exists but has no
  triggering job (likely because it needs a `--against <git-ref>` baseline that isn't wired up).
  This means the "append only" rule is enforced by convention/review only, not tooling, for this
  change.
- `buf.yaml:12` sets `enum_zero_value_suffix: _UNSPECIFIED` — the new value must not be `0` and
  any new enum this feature introduces (unlikely, since it's reusing `DetectedStatus`) would need
  to keep `_UNSPECIFIED = 0` as the first value.

**Silent-swallow risk on the Go side**: `DetectedStatusToProto` (`session/detection/proto_mapping.go:8-35`)
and `DetectedStatusToSubStatus` (`proto_mapping.go:44-70`) both end in a `default:` case returning
`UNSPECIFIED`. If a new `StatusCompacting` Go value is added to the iota in `detector.go` but the
corresponding `case StatusCompacting:` is forgotten in either mapping function, the code compiles
cleanly and **silently degrades to `DETECTED_STATUS_UNSPECIFIED`** at the wire boundary — no
error, no test failure unless a test explicitly asserts the mapping. This is the single highest-
risk gap for AC3 in the requirements doc; see §4 below for the full list of similar switches.

## 3. Interface-pollution risk — the "compaction tracker" temptation

The most natural over-engineered shape here is a `CompactionTracker`/`CompactionDetector` struct
or interface layered on top of `StatusDetector` — e.g. something that tracks "entered compacting
state at T, still compacting, exited at T+N" as a stateful abstraction. This would trip smell #1
(*speculative interface*, one implementation) and likely #4 (*forwarding wrapper*) from
`.claude/rules/interface-pollution-checklist.md`, because:

- The existing architecture is stateless pattern matching: `StatusDetector.Detect` takes raw bytes
  and returns a `DetectedStatus` on every call, with no notion of "currently in state X" persisted
  between calls (state continuity, if any, lives entirely in the caller re-polling and getting the
  same status again). A `CompactionTracker` would introduce a new state-tracking concept nothing
  else in the detection package has, for a feature whose whole job — per Acceptance Criterion 4 —
  is exactly what `StatusDetector.Detect`/`DetectWithContext` already does for every other status:
  regex match → return enum value.
- The correct pattern (checklist's "Correct Pattern" #1/#6) is to treat `StatusCompacting` as just
  one more `DetectedStatus` enum value with one more pattern group entry, reusing every existing
  code path (`PatternSet.MatchLines`, `DetectedStatusToProto`, `deriveWorkingState.ts`) with zero
  new types. Nothing in the requirements needs duration tracking, retry counting, or any behavior
  beyond "is this string in the tail right now" — the same shape as `StatusWaitingForAgent`, which
  is the closest recent precedent (`detector.go:31`, added as a plain enum value + pattern group,
  no wrapper type).
- Frontend mirrors this: `deriveWorkingState.ts` and `StatusBadge.tsx`/`SubStatusChip.tsx` are
  pure enum→enum / enum→render mapping functions (see §4) — a "compacting" badge is one more
  `case` in each, not a new component hierarchy or a "CompactionIndicator" wrapper component
  layered over the existing badge.

## 4. Exhaustiveness pitfalls — most Go switch sites silently swallow a new value; TS mostly doesn't

Beyond `StatusString()` (`detector.go:646-672`, ends with a bare `return "Unknown"` after the
switch — a new value falls through silently to "Unknown") and `GetPatternNames`
(`detector.go:693-726`, no case for an unlisted status → `patterns` stays `nil` → empty slice
returned, no error), a repo-wide search turned up these additional Go switch sites over
`DetectedStatus`/`SubStatus`, **none of which fail loudly** if a new value is added and not
threaded through:

| Site | Behavior on unmatched value |
|---|---|
| `session/status_mapping.go:18-36` `AttentionReasonFromDetected` | `default: return ""` |
| `session/instance_status.go:108-123` `GetStatusIcon` | `default: return "●"` |
| `session/instance_status.go:146-159+` `GetStatusDescription` | falls to a generic default string |
| `session/review_queue_determiner.go:127-155,199-232` | if/else-style switch, no default — condition simply never matches |
| `session/autonomous_driver.go:461-467` `isImmediateStatus` | `default: return false` |
| `session/detection/events.go:66-90` `categoryName` | falls through to `return "unknown"` |
| `session/detection/idle.go:186-232` `mapStatusToIdleState` | falls through to `return IdleStateWaiting` |
| `server/adapters/instance_adapter.go:248-271` `toProtoSubStatusFromInfo` | falls through to `SUB_STATUS_UNSPECIFIED` |
| `server/adapters/review_queue_adapter.go:12-36` `subStatusFromItem` | falls through to `SUB_STATUS_UNSPECIFIED` |

**Root cause confirmed in `.golangci.yml:22-114`**: the `exhaustive` linter is enabled
(`linters.enable: [..., exhaustive]`, line 10) but is explicitly *excluded* everywhere except
`session/detection/` itself — the exclusion rules (lines 83-114) blanket-disable it for
`^server/`, `^session/[^d]`, `^session/d[^e]`, `^session/de[^t]`, `^session/det[^e]`,
`^session/dete[^c]` (i.e. every `session/*` path except one starting `session/detec...`), plus
`^main.go`, `^pkg/`, `^cmd/`, etc. The comment at line 83-84 confirms this is intentional: *"exhaustive
enforces DetectedStatus switch coverage in session/detection/ only. All other packages use iota
types with intentional default: clauses."* So `exhaustive` **will** catch a missing case inside
`session/detection/` (e.g. in `pattern_set.go` if it ever switched over the enum, which it
doesn't — it returns fixed constants per category) but will **not** catch any of the 9 sites in
the table above. Every one of them needs a manual audit-and-add pass; none will fail CI on its own
if `StatusCompacting` is forgotten there. Acceptance Criterion 7 ("additive only, no existing
classification changes") makes this a currently-invisible defect class:  a UI icon/description/
attention-reason silently regressing to a generic fallback for the new state would not fail any
test unless a test is written specifically for it.

**Frontend is mostly safer**, confirmed by direct read of `deriveWorkingState.ts:46-66`:
- `web-app/src/lib/utils/deriveWorkingState.ts` — the `detectedStatus`-based fallback switch ends
  `default: return assertNever(session.detectedStatus)` (line 64-65) — **this is a TypeScript
  compile error** if a new `DetectedStatus` enum member isn't added as an explicit case. Good.
  However, the *first* switch in the same function, over `session.subStatus` (lines 27-43), has
  **no default and no `assertNever`** — it just `break`s out on `SubStatus.UNSPECIFIED` and falls
  through to the second switch for anything unlisted. If this feature adds a new `SubStatus` value
  (it likely won't — the plan should reuse `SubStatus.PROCESSING` per the mapping table in that
  file's own doc comment, lines 11-21) rather than only a new `DetectedStatus` value, that first
  switch would silently fall through with no compiler enforcement.
- `web-app/src/components/sessions/StatusBadge.tsx:39-68` `getDetectedStatusInfo` — also ends
  `default: return assertNever(status)` — compile error if unhandled.
- `web-app/src/components/sessions/SubStatusChip.tsx:33-168` — `default: { const _exhaustive:
  never = subStatus; console.warn(...); return null; }` — compile-time enforced at the TS level,
  but note this is a deliberately *soft* runtime fallback (warn + render nothing) for the case
  where an older frontend build receives a newer proto wire value from the server it doesn't know
  about yet — intentional, not a gap to fix.
- No project-wide ESLint `switch-exhaustiveness-check` rule exists (`web-app/.eslintrc.json`
  checked) — exhaustiveness on the frontend is entirely dependent on hand-written
  `assertNever`/`never`-typed patterns being present at each site, not a lint rule that would catch
  a *new* switch someone writes without that pattern (e.g. in a new session-card sub-component).

## 5. Feature registry rule — what happens if skipped

`.claude/rules/feature-registry.md` requires new/updated entries under `docs/registry/features/`
plus `make registry-generate`. Checked `.github/workflows/registry-validation.yml` directly:

- CI runs on any PR touching `**.go`, `**.proto`, or `web-app/src/**` (workflow `on.pull_request.paths`,
  lines 6-11) — a session-card badge + new proto enum value change would trigger this workflow.
- The job **re-runs `make registry-generate` itself** (step "Run registry generation", line ~33)
  and then diffs the regenerated per-feature files against what's committed via
  `tools/scanner/validate-registry.sh` (step "Validate registry divergence"). The script comment
  states: *"Exits 1 if divergence > 2%, 0 otherwise"* and the PR-comment body states *"Divergence >
  2% blocks merges."* So skipping the manual `make registry-generate` step locally does **not**
  silently pass — CI regenerates and diffs regardless, and a genuinely new frontend-observable
  feature surface (the compacting badge, if it crosses a >2% divergence threshold on its own — a
  single new feature entry is unlikely to alone exceed 2% of the whole registry, so this may be a
  soft/advisory-in-practice gate for a change this size, not a hard block) is caught, though the
  practical severity depends on total registry size at the time.
- Test coverage (`testIds` populated) is explicitly **advisory only** — the workflow comment
  states *"Coverage reporting is advisory only"* — so a `tested: false` entry does not block the
  PR, only the existence/shape mismatch does.

Net: skipping the registry update risks a CI-blocking divergence flag (not silent), but the
practical bar to actually trip the >2% threshold for one badge/one status value may be low given
this is a per-feature-file architecture (each entry is its own file, so "divergence" is presumably
measured as file-level add/remove, not a percentage of one monolithic diff — the exact metric
computation lives in `tools/scanner/validate-registry.sh`/`report-coverage.py`, not fully verified
in this pass; worth a quick read during planning if the exact threshold behavior matters).

## 6. Testing pitfalls — timing-based flakiness risk for a "transient" state

Confirmed via `grep -n "^func Test" session/detection/detector_test.go` (28 test functions) and
spot-reading several: **every existing test in `detector_test.go` operates on static byte-string
literals or `testdata/*.txt` fixtures read via `os.ReadFile`-style helpers — none use
`time.Sleep`, polling loops, or any wall-clock dependency.** `TestStatusDetector_PriorityOrder`
(`detector_test.go:278-312`) is representative: table-driven, each case is a literal input string
passed straight to `sd.Detect([]byte(tc.input))` with a synchronous assertion. There is no live-PTY
or live-Claude-process test anywhere in this file.

This matters directly for the new feature: the "compacting" state is transient (30-60s per the
issue) only in the *live system* — but at the unit-test level (which is what Acceptance Criterion 4
requires: "verified by a Go test using the new fixture(s)"), it is exactly as static as every other
status. There is **no inherent flakiness risk** here as long as the implementation follows the
existing pattern exactly: capture the literal compaction-in-progress string as a static fixture
file under `session/detection/testdata/`, and assert `Detect()`/`DetectWithContext()` against that
fixture's byte content directly, the same way `claude_active.txt` / `claude_thinking_verb.txt` /
`claude_asterism_active.txt` are already tested. The actual risk is upstream of the test suite: if
the fixture is captured from a live/ephemeral Claude Code session (per the Open Question in
requirements.md — ACT 1 is not yet verified), getting a *faithful, byte-accurate* capture of a
30-60s-transient real-world string (including exact spinner/percentage formatting, whitespace, and
any variable duration text) requires deliberate capture tooling (e.g. redirecting a real PTY
session's raw output to a file during an actual auto-compaction event) rather than reconstructing
the string from memory or documentation — a fixture built from a guessed/approximate string is the
same risk called out in requirements.md's Open Question, just moved from "will the pattern match
production" to "will the test fixture match production," and a mismatched fixture would make the
test pass while the real pattern fails to fire in the field.
