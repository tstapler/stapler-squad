# Stack Research: Context-Compaction Detection

## 1. Detection machinery already in place (`session/detection/`)

No new libraries are needed. This feature is additive within the existing regex-based
detection stack — stdlib `regexp` only, no new dependency.

### `pattern_set.go`
- `PatternSet` compiles `dtypes.StatusPatterns` (one `[]dtypes.StatusPattern` per status
  category) into parallel `[]*regexp.Regexp` slices at construction time
  (`NewPatternSet` → `compile()`), using plain `regexp.Compile` (Go stdlib RE2, no PCRE
  features — no lookahead/lookbehind available).
- `MatchLines(text string, rawPTY []byte) (DetectedStatus, string, string)` is a
  **hardcoded, fixed-order category chain**, not a priority-sorted flat list:
  `Error → TestsFailing → NeedsApproval → InputRequired → (readline typing) →
  WaitingForAgent → Success → Active → Processing → (screen-overwrite fallback) →
  Idle → Ready (catch-all)`. The per-pattern `Priority` int field is documentary only —
  within a category, patterns are checked in slice order (array order = declaration
  order in `claude.go`), first match wins; `Priority` does not drive an actual sort at
  match time (confirmed by reading `compile()` — it copies patterns in input order,
  no sort call anywhere in `pattern_set.go`).
- **Implication for this feature**: adding a `StatusCompacting` category requires (a) a
  new `Compacting []StatusPattern` field on `dtypes.StatusPatterns`, (b) a new
  `compactingRegexes []*regexp.Regexp` field + compile-group entry in `PatternSet`, and
  (c) a new check block inserted into `MatchLines` at the **correct position in the fixed
  chain** — almost certainly before `Active`, because of the priority-inversion risk
  described in §3 below.

### `normalizer.go`
`PTYNormalizer.Normalize` = `stripANSI(collapseCarriageReturns(content))`, stateless,
no external deps (hand-rolled ANSI stripper + CR-collapse, both private funcs in the
same package — not shown but referenced from `detector.go`). No change needed for this
feature; the new fixture(s) go through the same normalization path as everything else.

### `detector.go` / `dtypes.go`
- `DetectedStatus` is a plain Go `iota` enum (`session/detection/detector.go:18-31`).
  Adding `StatusCompacting` is a one-line const addition — **append at the end of the
  iota block**, don't insert in the middle (would silently renumber every later value;
  nothing currently persists the raw int across process boundaries as far as this
  research found, but `session/gen`/proto mapping makes the enum's Go-side ordinal
  irrelevant anyway since `proto_mapping.go` is an explicit switch, not `int(status)`).
- `dtypes.StatusPatterns` (`session/detection/dtypes/dtypes.go:14-24`) is the YAML-taggable
  struct consumed by both the Go default patterns (`claude.go`) and
  `NewStatusDetectorFromFile`'s YAML loader (`gopkg.in/yaml.v3 v3.0.1`, already a direct
  dependency — see `go.mod:48`). A new `Compacting` field must carry a `yaml:"compacting"`
  tag to stay loadable from the same YAML shape.

### `binaries/claude.go`
Existing `Active` category patterns (priority field values shown, though unused for
ordering — see above) include, at `session/detection/binaries/claude.go:112-136`:
- `esc_to_interrupt` — `` esc\s+(to\s+)?(interrupt|cancel) ``
- `synthesizing` — `` (?i)Synthesizing\.{0,3} ``
- `claude_thinking_verb` — `` (?m)^[ \t]*[·✢✳✶✻✽●*✦][ \t]+[A-Z][a-zA-Z'\-éèêàâùûôîïëüöäÿæœ]*(?:…|\.{1,3}) `` —
  matches **any** capitalized spinner-verb-plus-ellipsis line (e.g. `✻ Moonwalking…`).
- `running_status` — `` Running\.{3,} ``

**Critical finding**: `claude_thinking_verb` is generic enough that a line like
`✻ Compacting…` or `* Compacting conversation…` would **already match it** today and
classify as `StatusExecuting` (Active category). Since `MatchLines` checks `Active`
*before* any new `Compacting` category would need to be inserted, the new pattern group
must be spliced into the chain **ahead of `Active`** (i.e. between `WaitingForAgent`/
`Success` and `Active`), or the generic thinking-verb pattern will win first and the new
state will never be reachable. This is a concrete correctness risk for the planning
phase, not just a style note.

## 2. Proto toolchain generating `session/gen/session/v1` (Note: repo publishes into `gen/proto/go`, not `session/gen`)

- Correction on the requirements doc's assumed path: Go bindings actually land in
  `gen/proto/go/session/v1/*.go` (see `buf.gen.yaml`'s `go_package_prefix` override and
  `Makefile:403`'s existence check `gen/proto/go/session/v1/session.pb.go`), and
  TS bindings in `web-app/src/gen/session/v1/*_pb.ts`. `proto_mapping.go` imports
  `sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"`.
- Toolchain: **buf** (`buf.gen.yaml`, `version: v2`), driving three plugins:
  - `buf.build/protocolbuffers/go` (remote) → Go message structs, `paths=source_relative`
  - `buf.build/connectrpc/go` (remote) → ConnectRPC service code
  - local `web-app/node_modules/.bin/protoc-gen-es` (`target=ts`) → TypeScript
    (`@bufbuild/protobuf` ecosystem — `protoc-gen-es`, generates the `_pb.ts` files
    imported as `@/gen/session/v1/types_pb` in the frontend).
- `make proto-gen` (`Makefile:398-413`) is idempotent/staleness-checked: only re-runs
  `buf generate proto` if any `.proto` file is newer than a stamp file, or if the
  generated Go/TS output is missing, or if `protoc-gen-es` itself was rebuilt. Depends on
  `ensure-tools` (installs `buf` etc.) and `web-app/node_modules/.modules.yaml` (pnpm
  install marker — confirms the frontend package manager is **pnpm**, consistent with
  existing repo memory that npm-vs-pnpm mismatches break CI silently).
- Enum convention enforced by buf lint (`buf.yaml`: `enum_zero_value_suffix: _UNSPECIFIED`,
  `STANDARD` lint ruleset) — a new `DetectedStatus` value must follow the existing
  `DETECTED_STATUS_*` naming and the zero value stays `_UNSPECIFIED`. Append new values
  at the end of the enum (`proto/session/v1/types.proto:380-392`, currently ends at
  `DETECTED_STATUS_WAITING_FOR_AGENT = 11`) — buf's `breaking: use: [FILE]` check (with
  only a `FIELD_SAME_NAME` exception) would flag renumbering/removing existing values,
  so a straightforward append to `= 12` is the only safe move.
- **Second touchpoint the requirements doc doesn't mention but exists in the code**:
  there is a **parallel `SubStatus` enum** (`proto/session/v1/types.proto:412-434`,
  values 0-10) and a second authoritative mapping function
  `DetectedStatusToSubStatus` in `proto_mapping.go` (`session/detection/proto_mapping.go`)
  that the frontend's `deriveWorkingState.ts` actually treats as the **primary** signal
  (see §3). If compaction should render as `SubStatus`-driven in the UI (recommended —
  it's the field `deriveWorkingState` checks first), planning must also add
  `SUB_STATUS_COMPACTING` to that enum and a case to `DetectedStatusToSubStatus`, not just
  `DetectedStatusToProto`. This is a 4th proto/mapping touchpoint beyond what
  requirements.md's Acceptance Criterion 3 lists explicitly.

## 3. Frontend test tooling for `deriveWorkingState.ts`

- File: `web-app/src/lib/utils/deriveWorkingState.ts`. Test:
  `web-app/src/lib/utils/__tests__/deriveWorkingState.test.ts`.
- Runner: **Jest 30.2** (`web-app/package.json`) via `ts-jest 29.4` (no Babel), config in
  `web-app/jest.config.js` — multi-`projects` setup; the relevant project is
  `displayName: "web-app"`, `testEnvironment: "jest-environment-jsdom"`,
  `testMatch: ["**/__tests__/**/*.ts?(x)", ...]`, path alias `^@/(.*)$` →
  `<rootDir>/src/$1` (so `@/gen/session/v1/types_pb` resolves to the buf-generated file).
- `deriveWorkingState` has **two switches**, and they are **not symmetrically
  exhaustive**:
  1. First switch is on `session.subStatus` (`SubStatus`) — primary signal. It has
     **no `default`/`assertNever`**; an unhandled `SubStatus` value simply falls through
     to the second switch (silent, not a compile error).
  2. Second switch is on `session.detectedStatus` (`DetectedStatus`) — fallback, used
     only when `subStatus` is `UNSPECIFIED`. This one **does** end in
     `default: return assertNever(session.detectedStatus)` (`assertNever` from
     `@/lib/utils/assertNever`), which **is** a TypeScript compile-time exhaustiveness
     guard — adding a new `DetectedStatus` enum value without adding a `case` here will
     fail `tsc`/`ts-jest` type-checking.
- **Planning implication**: a new `SubStatus.SUB_STATUS_COMPACTING` case must be added
  explicitly to the first switch — it will **not** be caught by a compiler error if
  forgotten, only by a missing/incorrect test, because that switch has no exhaustiveness
  check. The second switch's `assertNever` *will* force a `DetectedStatus.COMPACTING` case
  to be added if a new `DetectedStatus` value is introduced (self-enforcing).
- Test pattern convention observed in the existing suite: grouped `describe` blocks per
  target `WorkingState`, table-driven over an array of enum members, test names
  `deriveWorkingState_should_return<STATE>_When_subStatus_is_${SubStatus[subStatus]}`
  (string-interpolates the enum's reverse-mapping name). A new `describe("... —
  SubStatus.COMPACTING")` block (or added member to an existing group, if compacting is
  folded into a new distinct `WorkingState` rather than reusing `PROCESSING`) should follow
  this exact naming/grouping convention.
- Run command: `cd web-app && npx jest --no-coverage --testPathPatterns="deriveWorkingState"`.
- No new `WorkingState` proto value is strictly required by the requirements (AC 5 only
  asks for a "distinct frontend working state" mapping, not necessarily a new enum value)
  — but the existing `WorkingState` enum (`proto/session/v1/types.proto:397-407`: ACTIVE,
  PROCESSING, IDLE, WAITING) has no slot that reads as "compacting" distinctly from
  "processing." If the acceptance criterion's "visually distinct badge" (AC 6) is meant
  literally, either (a) a new `WORKING_STATE_COMPACTING` proto value is the cleanest fit
  (mirrors the `SubStatus`/`DetectedStatus` addition pattern), or (b) the session-card
  component keys its badge directly off `SubStatus.COMPACTING` (bypassing `WorkingState`
  entirely for this one visual). This is a planning-phase decision, not fully dictated by
  the existing type contracts — flagging both options for `/sdd:3-plan`.

## 4. The literal compaction-in-progress terminal string — confirmed vs. inferred

**Not fully confirmed against live output** (no live compaction was triggered/captured in
this research pass — that remains the single open risk item for AC 1). However, strong
converging evidence from two independent sources points to the same literal string:

### A. Local Claude Code binary (installed, `claude --version` → `2.1.223 (Claude Code)`,
binary at `/home/tstapler/.local/share/claude/versions/2.1.223`)
`strings` extraction of the binary (a bundled/minified JS build, not source) found — in
immediate proximity to each other, consistent with being fields of one status/state
object in the CLI's Ink-based renderer:
```
applyCompactProgress
isCompacting
compactingHintText
compactingStartTime
compact_start / compact_end / pre_compact / post_compact   (telemetry event names)
claudeBlue_FOR_SYSTEM_SPINNER / claudeBlueShimmer_FOR_SYSTEM_SPINNER
Compacting conversation                                     ← literal UI string
```
This reads as a spinner state object (`isCompacting` bool, `compactingHintText` string,
`compactingStartTime` timestamp, `applyCompactProgress` — a progress-updater function)
whose override text is the literal string `Compacting conversation`, rendered through the
same "shimmer spinner" mechanism already used for the generic thinking-verb spinner
(`claude_thinking_verb` pattern in `claude.go` matches exactly this shape: spinner glyph +
capitalized verb phrase + ellipsis). A second, unrelated occurrence — `Compacting at auto
window ( <N> tokens) /autocompact to configure` — is a separate settings/telemetry string
(config command output), not the in-progress spinner text.

### B. GitHub issue anthropics/claude-code#30115 ("Show progress indicator during context
compaction")
Filed against an **older** CLI version where "The CLI gives no feedback during this
operation, which can make it appear frozen" — the requester explicitly proposed the CLI
should show:
```
⠋ Compacting conversation... 42%
```
as their mockup of desired behavior (screenshot attached showing today's frozen state).
The proposed text independently matches the exact phrase (`Compacting conversation`) found
in the current binary's strings, plus a percentage — consistent with `applyCompactProgress`
/`compact_progress` existing as real, implemented properties in the version installed
locally (2.1.223, i.e. a materially newer build than whatever shipped when #30115 was
filed). This is circumstantial corroboration, not a changelog confirmation that the
feature request was implemented verbatim — GitHub's issue state/timeline wasn't checked
in this pass (only issue body content was fetched).

### Best-available hypothesis for AC 1 (state confidence: INFERRED, not VERIFIED)
The in-progress compaction line most likely renders as a spinner-glyph line matching the
existing `claude_thinking_verb` shape, e.g.:
```
✻ Compacting conversation… (esc to interrupt)
```
or with a trailing percentage per the `applyCompactProgress` evidence, e.g.:
```
✻ Compacting conversation… 42% (esc to interrupt)
```
possibly *without* the `N% until auto-compact` trailer line that appears in the
*approaching*-threshold fixtures, since that trailer is describing distance-to-threshold,
which is meaningless once compaction has actually started.

**Before implementation (not before planning), AC 1 must be closed by one of:**
1. Triggering `/compact` manually in a live `claude` session in this repo and capturing
   raw PTY bytes (`script` or the existing scrollback-capture mechanism used to produce
   the other `testdata/*.txt` fixtures) — the most reliable method, and cheap (compaction
   is a single manual command, not a 30-60s wait for auto-trigger).
2. Searching recent (post-#30115-resolution) closed issues/PRs/CHANGELOG entries in
   `anthropics/claude-code` for confirmation the feature shipped and its exact copy.
Recommend planning phase schedule this as the literal first implementation task (blocks
AC 1, 2, 4 — everything downstream of having a real fixture), consistent with
requirements.md's own framing that this is "the single largest unknown."

## Summary of concrete file touchpoints identified (supplementing requirements.md)

| Layer | File | Change |
|---|---|---|
| Go enum | `session/detection/detector.go` | append `StatusCompacting` to `DetectedStatus` iota |
| Go pattern types | `session/detection/dtypes/dtypes.go` | add `Compacting []StatusPattern \`yaml:"compacting"\`` field |
| Go pattern matching | `session/detection/pattern_set.go` | add `compactingRegexes` field, compile-group entry, and a `MatchLines` check block positioned **before** `Active` |
| Go binary patterns | `session/detection/binaries/claude.go` | add `Compacting: []dtypes.StatusPattern{...}` once AC 1's string is confirmed |
| Fixtures | `session/detection/testdata/claude_compacting*.txt` | new fixture(s), distinct from `claude_active.txt`'s "10% until auto-compact" |
| Proto | `proto/session/v1/types.proto` | append `DETECTED_STATUS_COMPACTING = 12` to `DetectedStatus`; **also** consider `SUB_STATUS_COMPACTING = 11` on `SubStatus` (not in requirements.md, but is what the frontend actually reads first) |
| Go proto mapping | `session/detection/proto_mapping.go` | case in `DetectedStatusToProto`; **also** case in `DetectedStatusToSubStatus` if `SubStatus` gets the new value |
| Frontend mapping | `web-app/src/lib/utils/deriveWorkingState.ts` | case in the `subStatus` switch (no compiler safety net — must not be forgotten) and/or the `detectedStatus` switch (compiler-enforced via `assertNever`) |
| Frontend tests | `web-app/src/lib/utils/__tests__/deriveWorkingState.test.ts` | new `describe` block, existing naming convention |
| UI badge | `web-app/src/components/sessions/*` (session card) | new visual variant — exact component not yet located, needs a targeted search in planning phase for where `WorkingState`/`SubStatus` badges are currently rendered |
| Codegen | `make proto-gen` | required after any `.proto` edit; regenerates `gen/proto/go/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts` |
| Registry | `docs/registry/features/frontend/*.json` | new entry per `.claude/rules/feature-registry.md` if the compaction badge is a new frontend-observable surface (AC 8) |
