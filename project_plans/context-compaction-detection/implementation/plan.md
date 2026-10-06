# Implementation Plan: context-compaction-detection

**Feature**: Detect Claude Code's "compacting context" terminal state as its own `DetectedStatus`/`SubStatus` value, distinct from generic Processing/Executing, and surface it as a dedicated session-card badge.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: [ADR-001: Populate `SubStatus.COMPACTING` in addition to `DetectedStatus.COMPACTING`](../decisions/ADR-001-substatus-not-just-detectedstatus.md)

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `DetectedStatus` | Go enum (`session/detection/detector.go:18-32`) of terminal-output classifications (`StatusReady`, `StatusProcessing`, `StatusExecuting`, ...). Mirrored 1:1 by proto `sessionv1.DetectedStatus`. | This plan adds `StatusCompacting`. |
| `StatusCompacting` | New `DetectedStatus` value meaning "Claude Code is actively summarizing/compacting older conversation history right now" (as opposed to `10% until auto-compact`, which is an *approaching* threshold, not an in-progress state). | Appended after `StatusWaitingForAgent`, iota value 11. |
| `SubStatus` | Proto enum (`proto/session/v1/types.proto:412-434`) — the fine-grained activity signal the frontend actually renders as a chip. Derived from `DetectedStatus` at read time via two adapter functions; never stored. | This plan adds `SUB_STATUS_COMPACTING = 11`. |
| `WorkingState` | Proto enum (`proto/session/v1/types.proto:398-408`) — coarse 4-value bucket (`ACTIVE`/`PROCESSING`/`IDLE`/`WAITING`) used for review-queue filtering. Derived from `SubStatus`/`DetectedStatus` in `deriveWorkingState.ts`, not a new signal. | `SubStatus.COMPACTING` joins the existing `PROCESSING` bucket; `DetectedStatus.COMPACTING` joins the existing `ACTIVE` bucket (see Story 3.1.1). |
| `StatusPattern` | `session/detection/dtypes.StatusPattern` — one named regex + description + decorative `Priority` int (never read at match time — confirmed via `pitfalls.md`). | New pattern: `compacting_conversation`. |
| `PatternSet` | `session/detection/pattern_set.go` — compiles a `StatusPatterns` struct into per-category `[]*regexp.Regexp` slices and runs the **hardcoded** priority chain in `MatchLines()`. The real precedence mechanism; `StatusPattern.Priority` is not it. | Gets a new `compactingRegexes` field + a new chain link. |
| `Compacting` | New field name on `dtypes.StatusPatterns` (`yaml:"compacting"`) holding the Claude-specific `[]StatusPattern` for this state — mirrors the existing `WaitingForAgent []StatusPattern` field added for the `StatusWaitingForAgent` precedent. | `session/detection/dtypes/dtypes.go`. |
| `BinaryDetector` | Interface (`dtypes.BinaryDetector`) implemented by `binaries.ClaudeDetector` — supplies the per-binary `StatusPatterns` literal actually used by `DetectForProgram`. | `session/detection/binaries/claude.go`'s `Patterns()` gets the new `Compacting` field populated; no other binary gets it (non-goal). |
| `MatchLines` chain | The fixed sequence of category checks inside `PatternSet.MatchLines` (`pattern_set.go:69-141`): Error → TestsFailing → NeedsApproval → InputRequired → readline_typing → WaitingForAgent → Success → **Compacting (new)** → Active → Processing → screen-overwrite fallback → Idle → Ready. | Compacting is inserted between Success and Active — see Story 1.2.2 for why it must precede Active specifically. |
| `DetectedStatusToProto` | `session/detection/proto_mapping.go:8` — the single authoritative `DetectedStatus` → proto `DetectedStatus` mapping. | Gets a `StatusCompacting` case. |
| `DetectedStatusToSubStatus` | `session/detection/proto_mapping.go:44` — the single authoritative `DetectedStatus` → proto `SubStatus` mapping. **Has zero call sites outside its own package/tests** — the RPC path actually goes through the duplicate switches below (confirmed dead code on the hot path, per `features.md`). | Still must be updated for consistency/tests, but is not what ships the badge. |
| `toProtoSubStatusFromInfo` | `server/adapters/instance_adapter.go:240-272` — the function **actually** wired to the outgoing `SubStatus` field on the main session list/watch RPC (called at `instance_adapter.go:167`). Duplicates `DetectedStatusToSubStatus` almost line-for-line. | **Highest-risk manual site** — missing this ships a feature that compiles, passes tests, and never appears on the board. |
| `subStatusFromItem` | `server/adapters/review_queue_adapter.go:12-36` — same duplicate-switch shape, scoped to the review-queue panel's `ReviewItem` proto. | Same failure mode as above, smaller blast radius (review queue only). |
| `chipCompacting` | New vanilla-extract style export in `web-app/src/components/sessions/SubStatusChip.css.ts`, composed from the shared `chip` base + the same `accentBg`/`primary` tokens as `chipWaitingForAgent` (calm, self-managing framing — not warning/error tokens). | Per `.claude/rules/css-architecture.md`, tokens only, no hardcoded colors. |
| `SubStatusChip` | `web-app/src/components/sessions/SubStatusChip.tsx` — THE actual badge-rendering component AC6 targets (switches on `subStatus` directly, has a compile-enforced `_exhaustive: never` default case). | Gets a `case SubStatus.COMPACTING:` rendering `⟳ Compacting context`. |
| `deriveWorkingState` | `web-app/src/lib/utils/deriveWorkingState.ts` — two sequential switches (SubStatus primary, DetectedStatus fallback), the second one is `assertNever`-guarded. | Both switches need a new case. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Detection mechanism | Stdlib `regexp` (RE2) pattern group, following the exact `StatusWaitingForAgent` precedent: own iota value, own pattern group (not folded into `Active`), explicit priority-placement comment, dedicated fixture + test | `build-vs-buy.md`, `features.md` (PR #115/#121/#132 precedent) | **Hook-based signal** — Claude Code's documented `PreCompact`/`PostCompact` hooks, wired through the existing `hook_injector.go` infrastructure, would give a deterministic start/end event instead of scraping text | Stronger long-term signal (see ADR follow-up note below), but requires new hook-injection wiring that exceeds this item's committed regex-based scope per requirements.md's own framing. Flagged as a fast-follow, not built here. |
| Detection mechanism (rejected 2nd alternative) | — | Step 0.5 brainstorm | **Infer "compacting" purely from the existing "N% until auto-compact" trend reaching 0%**, without a new confirmed string | Indirect/heuristic — can only signal "about to compact," never "compacting right now" (violates AC1's "actively in progress" requirement), and still needs a real regex for the eventual in-progress line, so it doesn't actually avoid the fixture-capture problem it's trying to sidestep. |
| Data model shape | One more stateless enum value + regex group, zero new Go types | `pitfalls.md`'s explicit "CompactionTracker" anti-pattern warning | A `CompactionTracker` struct tracking start time / duration / percentage | Requirements' non-goals explicitly exclude a progress meter or duration tracking; a stateful tracker is unjustified complexity for a value that's recomputed fresh on every `Detect()` call like every other transient `DetectedStatus`. |
| Signal surfaced to frontend | Populate **both** `DetectedStatus.COMPACTING` and `SubStatus.COMPACTING` | `ux.md`, `architecture.md` (precedence-conflict finding) | `DetectedStatus` only, per AC2/AC3's literal wording | `SessionCard.tsx:533-548` renders `SubStatusChip` (SubStatus-driven) whenever `subStatus` is not `UNSPECIFIED`/`IDLE`, and only falls back to `StatusBadge` (`DetectedStatus`-driven) otherwise. Compaction happens while `SubStatus` is already non-idle (typically `PROCESSING`), so a `DetectedStatus`-only implementation would be silently suppressed — the badge would never render. See ADR-001. |
| Priority placement in `MatchLines` | Insert the `Compacting` check between `Success` and `Active` | Regex-overlap analysis (this session) | Insert after `Active`/`Processing` (lowest priority, "just another activity flavor") | Both candidate compacting strings (`✻ Compacting conversation… (esc to interrupt)` / `⠋ Compacting conversation... 42%`) already satisfy `Active`'s `esc_to_interrupt` and/or `claude_thinking_verb` regexes (`claude.go:114-118`, `126-130`). Checking Compacting after Active would make it permanently unreachable — the exact bug `claude_thinking_verb` was called out as already covering this text in `features.md`. |
| Proto enum numbering | Append-only: `DETECTED_STATUS_COMPACTING = 12`, `SUB_STATUS_COMPACTING = 11` | `stack.md` (buf breaking-change discipline), `pitfalls.md` (confirmed `buf breaking` is configured but never invoked in CI — convention-enforced only) | Insert alphabetically or renumber for tidiness | Proto enum values are wire-stable identifiers; renumbering breaks any already-serialized/cached client. No CI check catches this for this specific change, so it must be done correctly by hand. |

---

## Migration Plan

N/A — no schema or data migration. This feature adds append-only proto enum values (`DetectedStatus`, `SubStatus`) and a new Go const/struct field; neither is a stored-data schema change (both are derived at read time, never persisted — see `SubStatus`'s own doc comment at `types.proto:410-411`).

## Observability Plan

- **Logs**: No new log lines required. The existing `DetectionEvent` ring buffer (`session/detection/events.go`, `EventRingCap = 2000`) already records every `Detect()`/`DetectWithContext()` call including the new `StatusCompacting` result via `categoryName()` — visible through the existing `RecentEvents(n)` accessor with no additional wiring.
- **Metrics**: None added. This is a transient, recomputed-every-poll classification like every other `DetectedStatus` value; there is no existing per-status counter/histogram to extend, and requirements' non-goals exclude duration tracking.
- **Alerts**: None. Matches every other transient sub-status (no toast/notification precedent in `SubStatusChip.tsx`).

## Risk Control

- **Feature flag**: None — matches the additive, no-flag precedent of `StatusWaitingForAgent` (PR #132). The change is inert until the new regex actually matches live PTY output, so there is no behavior to gate.
- **Rollback procedure**: Standard revert. Because proto enum values are append-only and additive, reverting the Go/TS commits does not require a proto migration — older clients simply never see `SUB_STATUS_COMPACTING`/`DETECTED_STATUS_COMPACTING` on the wire again.
- **Staged rollout**: None needed — regex/enum-only change with existing fixture-based unit test coverage (Story 1.3.1) is sufficient given the small blast radius; standard PR review + CI gate before merge.

## Unresolved Questions

- [ ] Is the literal in-progress compaction string byte-accurate? — blocks Story 1.1.1 — owner: implementer, via the live-capture attempt in Task 1.1.1a; falls back to the documented INFERRED fixture (Task 1.1.1b) if no live Claude Code session is available in the implementation environment. Either path unblocks all downstream tasks — the regex in Task 1.2.2c is designed to match both hypothesis variants.
- [ ] Should the `PreCompact`/`PostCompact` hook mechanism replace this regex approach long-term? — not a blocker for this plan (out of scope per requirements.md) — flagged as a fast-follow ADR candidate; owner: whoever picks up the follow-up item, not this implementation.

## Dependency Visualization

```
Phase 1: Detection Layer (Go)
  Epic 1.1 Fixture Capture ──────────────┐
    Task 1.1.1a (live capture attempt)   │
    Task 1.1.1b (fallback inferred       │
                 fixture, conditional)   │
    Task 1.1.1c (INFERRED gating: follow-│
                 up item + canary log,   │
                 conditional on 1.1.1b)  │
                                          ▼
  Epic 1.2 Enum + Pattern Pipeline ◄──────┘
    Task 1.2.1a (detector.go enum)
    Task 1.2.1b (events.go, idle.go)
    Task 1.2.2a (dtypes.go field)
    Task 1.2.2b (pattern_set.go: 3 spots) ◄── depends on 1.2.2a
    Task 1.2.2c (claude.go pattern)       ◄── depends on 1.1.1a/b (fixture text informs regex)
                                          │
                                          ▼
  Epic 1.3 Regression Tests
    Task 1.3.1a (positive detect test)   ◄── depends on 1.2.2c
    Task 1.3.1b (AC7 negative-case test) ◄── depends on 1.2.2c
                                          │
                                          ▼
Phase 2: Proto Layer
  Epic 2.1 Proto enums
    Task 2.1.1a (types.proto both enums)
    Task 2.1.1b (make proto-gen)          ◄── depends on 2.1.1a
                                          │
                                          ▼
  Epic 2.2 Go Mapping Functions           ◄── depends on 2.1.1b AND Epic 1.2
    Task 2.2.1a (proto_mapping.go both funcs)
    Task 2.2.2a (instance_adapter.go — HIGH RISK)
    Task 2.2.3a (review_queue_adapter.go)
    Task 2.2.2b (table-driven parity test)  ◄── depends on 2.2.2a AND 2.2.3a
                                             (both switches must exist first)
    Task 2.2.4a (status_mapping.go + instance_status.go)
    Task 2.2.4b (instance_status.go icon/description)
                                          │
                                          ▼
Phase 3: Frontend                         ◄── depends on 2.1.1b (generated TS types)
  Epic 3.1 WorkingState + StatusBadge
    Task 3.1.1a (deriveWorkingState.ts + test)
    Task 3.1.2a (StatusBadge.tsx case)
  Epic 3.2 SubStatusChip UI
    Task 3.2.1a (SubStatusChip.css.ts chipCompacting)
    Task 3.2.2a (SubStatusChip.tsx case + test) ◄── depends on 3.2.1a
    Task 3.2.3a (SessionRow.tsx rowActive, optional polish)
                                          │
                                          ▼
Phase 4: Registry + Verification
  Epic 4.1 Feature registry
    Task 4.1.1a (new frontend registry JSON + make registry-generate)
  Epic 4.2 Final verification              ◄── depends on ALL prior tasks
    Task 4.2.1a (make lint, go test, jest, make ci)
```

---

## Phase 1: Detection Layer (Go backend regex/enum pipeline)

### Epic 1.1: Fixture Capture & Regex Grounding
**Goal**: Resolve requirements.md's Open Question — obtain (or explicitly mark as inferred) a fixture with the exact bytes Claude Code emits while actively compacting, before any regex is written against a guess.

#### Story 1.1.1: Capture the compaction-in-progress PTY fixture
**As a** implementer of this feature, **I want** a real or explicitly-labeled-inferred fixture of Claude Code's in-progress compaction output, **so that** the new regex is grounded in evidence rather than pure guesswork.
**Acceptance Criteria**:
- A fixture file exists at `session/detection/testdata/claude_compacting.txt` containing the compaction-in-progress line(s), distinct from the "N% until auto-compact" approaching-indicator already present in `claude_active.txt:5`, `claude_thinking_verb.txt:5`, `claude_asterism_active.txt:5`.
  - *Given* a live Claude Code session reaches high context usage or `/compact` is invoked manually, *When* the raw PTY bytes are captured mid-compaction, *Then* `claude_compacting.txt` contains the byte-accurate captured text and a matching Go doc comment in the new test (Story 1.3.1) states it is VERIFIED.
  - *Given* no live Claude Code session is available in the implementation environment, *When* the implementer falls back to the best-available hypothesis, *Then* `claude_compacting.txt` contains `✻ Compacting conversation… (esc to interrupt)` (or a trailing `NN%` variant) and both the fixture-adjacent test and the new `StatusPattern.Description` are marked INFERRED with a pointer to this plan for follow-up verification.
**Files**: `session/detection/testdata/claude_compacting.txt`

##### Task 1.1.1a: Attempt live capture of real compaction output (~5 min)
- In a real Claude Code terminal session (interactive, not `--print`), run enough turns to approach the auto-compact threshold shown by the existing "N% until auto-compact" indicator, or invoke `/compact` directly if the CLI exposes it as a slash command.
- While compaction is visibly in progress, capture the raw pane bytes: `tmux capture-pane -t <session-window> -p -e > /tmp/compact_capture.txt` (the `-e` flag preserves escape sequences, matching how `session/detection` receives real PTY output).
- Diff the captured text against the existing `claude_active.txt`/`claude_thinking_verb.txt` fixtures to confirm it's a genuinely new line, not just the familiar "N% until auto-compact" text.
- If successful: commit the **literal unedited raw bytes** from the capture (not a hand-retyped or "cleaned up" transcription) into `session/detection/testdata/claude_compacting.txt` — preserve real spinner glyphs, percentages, and any surrounding ANSI/cursor sequences exactly as captured. Pre-mortem P2 #4 flags hand-cleaned fixtures as a distinct failure mode from the INFERRED-string risk (#1): a fixture that "looks right" to a human but doesn't byte-match what the detector receives at runtime defeats the point of live capture.
- Files: `session/detection/testdata/claude_compacting.txt` (created)

##### Task 1.1.1b: Fallback — build the INFERRED fixture (~3 min, only if 1.1.1a is not possible)
- If no live Claude Code session is reachable in this environment, write `session/detection/testdata/claude_compacting.txt` using the best-available hypothesis from research: `✻ Compacting conversation… (esc to interrupt)` on its own line, following the existing fixtures' plain-text-with-trailing-newline convention (see `claude_active.txt` for format).
- Add a one-line `# INFERRED — see project_plans/context-compaction-detection/implementation/plan.md Story 1.1.1` comment is not possible in a plain testdata `.txt` file — instead, put this provenance note in the Go test's doc comment (Story 1.3.1) and in the `compacting_conversation` pattern's `Description` field (Task 1.2.2c), not in the fixture file itself.
- Files: `session/detection/testdata/claude_compacting.txt` (created)

##### Task 1.1.1c: INFERRED fallback gating — required if 1.1.1a did not produce a live capture (~10 min)
**Status (2026-08-12): Both gating conditions satisfied.** No live Claude Code compaction
was reachable in the implementing session's tmux scrollback (checked across all attached
sessions). Follow-up backlog item filed: `9637b022-a3d7-4639-ba37-fe2842e5d6dc` — "Verify
context-compaction regex against live Claude Code capture". Canary added to
`session/detection/detector.go` (`compactingCanary`, called from `detectFromText`).

**Added 2026-08-06 (Phase 4 pre-mortem, P1 #1)**: an INFERRED fixture must not be the basis for a "done" claim on this item — pre-mortem's top-ranked failure mode is that the whole feature ships green (compiles, all tests pass, `make ci` green) while the regex never matches real Claude Code output in production, because nobody re-verifies the guessed string after merge. Two things are both required, not optional, whenever Task 1.1.1a did not yield a real capture:
- File (or have already filed, if this backlog item was migrated from one) a follow-up backlog item titled "Verify context-compaction regex against live Claude Code capture" before this item is marked complete — link it in the PR description. This item does not close as "done" on an unverified regex; it closes as "shipped with a tracked follow-up."
- Add a temporary bake-in canary: in `StatusDetector.Detect`/`DetectWithContext` (`session/detection/detector.go`), log (at debug level, via the existing detection logging path — check `detector.go`/`event_sink.go` for the established pattern) the full raw text of any line matching `(?i)compact` that did **not** classify as `StatusCompacting`. This catches a near-miss regex (real string differs slightly from the INFERRED guess) once real usage starts, without waiting for a user bug report. Remove the canary once the follow-up item confirms the regex against a real capture.
- Files: `session/detection/detector.go` (canary log line), follow-up backlog item (tracked externally, not a file in this repo)

---

### Epic 1.2: Extend the Detection Enum and Pattern Pipeline
**Goal**: Add `StatusCompacting` end-to-end through every lint-enforced and pipeline-critical Go site so the new status is reachable and classified correctly.

#### Story 1.2.1: Add `StatusCompacting` to the `DetectedStatus` enum and its lint-enforced switch sites
**As a** maintainer, **I want** `StatusCompacting` to satisfy the `exhaustive` linter everywhere it's scoped (`session/detection/` only, per `.golangci.yml:83-96`), **so that** `make lint` doesn't fail and no switch silently falls through to a wrong default.
**Acceptance Criteria**:
- `StatusCompacting` is a new named `DetectedStatus` value, and `StatusString()`, `GetPatternNames()` (`detector.go`), `categoryName()` (`events.go`), and `mapStatusToIdleState()` (`idle.go`) all have an explicit case for it.
  - *Given* `StatusCompacting.String()` is called, *When* evaluated, *Then* it returns `"Compacting"`.
  - *Given* `sd.GetPatternNames(StatusCompacting)` is called, *When* evaluated, *Then* it returns `["compacting_conversation"]` for a detector loaded with the Claude pattern set.
**Files**: `session/detection/detector.go`, `session/detection/events.go`, `session/detection/idle.go`

##### Task 1.2.1a: Add the enum value and `StatusString`/`GetPatternNames` cases (~4 min)
- In `session/detection/detector.go:20-32`, append `StatusCompacting` after `StatusWaitingForAgent` in the `const (...)` block, with a comment: `// StatusCompacting is set when Claude is actively summarizing/compacting older conversation history (distinct from the "N% until auto-compact" approaching-threshold indicator).`
- In `StatusString()` (`detector.go:646-672`), add `case StatusCompacting: return "Compacting"` before the final `case StatusUnknown:`.
- In `GetPatternNames()` (`detector.go:693-726`), add `case StatusCompacting: patterns = p.Compacting` alongside the other cases.
- Update the doc comment on `Detect()` (`detector.go:270-272`) listing priority order to insert `Compacting` between `Success` and `Active`, matching the actual `MatchLines` order (Task 1.2.2b).
- Files: `session/detection/detector.go`

##### Task 1.2.1b: Add `categoryName` and `mapStatusToIdleState` cases (~3 min)
- In `session/detection/events.go`'s `categoryName()` (line 65), add `case StatusCompacting: return "compacting"`.
- In `session/detection/idle.go`'s `mapStatusToIdleState()` (line 185), add a case mirroring `StatusExecuting`/`StatusProcessing` (lines 187-197) — compaction is active work, not idle:
  ```go
  case StatusCompacting:
      // Claude is actively compacting — same treatment as Processing/Executing.
      id.lastActivity = id.timeNow()
      id.lastActivityNs.Store(id.lastActivity.UnixNano())
      return IdleStateActive
  ```
- Files: `session/detection/events.go`, `session/detection/idle.go`

#### Story 1.2.2: Add the `Compacting` pattern group to `dtypes`, `pattern_set.go`, and the Claude binary detector
**As a** maintainer, **I want** a dedicated regex group for compaction, checked before `Active` in the priority chain, **so that** the new state is actually reachable instead of being swallowed by `esc_to_interrupt`/`claude_thinking_verb`.
**Acceptance Criteria**:
- `dtypes.StatusPatterns` has a new `Compacting []StatusPattern` field; `PatternSet` compiles it into `compactingRegexes` and checks it between `Success` and `Active` in `MatchLines`.
  - *Given* the text `✻ Compacting conversation… (esc to interrupt)`, *When* run through `PatternSet.MatchLines`, *Then* it returns `StatusCompacting`, NOT `StatusExecuting` (which `esc_to_interrupt`/`claude_thinking_verb` would otherwise produce).
**Files**: `session/detection/dtypes/dtypes.go`, `session/detection/pattern_set.go`, `session/detection/binaries/claude.go`

##### Task 1.2.2a: Add the `Compacting` field to `dtypes.StatusPatterns` (~2 min)
- In `session/detection/dtypes/dtypes.go:15-26`, add `Compacting []StatusPattern \`yaml:"compacting"\`` after the `WaitingForAgent` field, with a doc comment `// Claude is actively compacting/summarizing conversation history`.
- This is a struct field addition only — no changes needed to `detector.go`'s `getDefaultPatterns()` generic literal (an unset slice field is safely nil, same as how `claude.go`'s own literal already omits `WaitingForAgent`).
- Files: `session/detection/dtypes/dtypes.go`

##### Task 1.2.2b: Wire `compactingRegexes` through `pattern_set.go` (~5 min)
- Add `compactingRegexes []*regexp.Regexp` to the `PatternSet` struct (`pattern_set.go:10-23`), after `waitingForAgentRegexes`.
- Add `{"compacting", ps.patterns.Compacting, &ps.compactingRegexes},` to the `groups` table in `compile()` (`pattern_set.go:41-52`), after the `waiting_for_agent` entry.
- In `MatchLines()` (`pattern_set.go:69-141`), insert a new loop **between** the `Success` loop (ends line 111) and the `Active` loop (starts line 112):
  ```go
  // Compacting — checked BEFORE Active so a "Compacting conversation" line is not
  // swallowed by esc_to_interrupt or claude_thinking_verb (both would otherwise
  // classify it as generic Active/Executing — see claude.go's compacting_conversation
  // pattern comment for the exact regex overlap this avoids).
  for i, regex := range ps.compactingRegexes {
      if regex.MatchString(text) {
          return StatusCompacting, ps.patterns.Compacting[i].Name, ps.patterns.Compacting[i].Description
      }
  }
  ```
- Files: `session/detection/pattern_set.go`

##### Task 1.2.2c: Add the `compacting_conversation` pattern to the Claude binary detector (~4 min)
- In `session/detection/binaries/claude.go`'s `Patterns()` (after the `Success` field, before the closing `}` of the `dtypes.StatusPatterns{...}` literal — matches where `WaitingForAgent` would slot in if this binary set it), add:
  ```go
  Compacting: []dtypes.StatusPattern{
      {
          Name: "compacting_conversation",
          // INFERRED/VERIFIED (see Story 1.1.1 in project_plans/context-compaction-detection/
          // implementation/plan.md for provenance). Requires "Compacting" capitalized at/near
          // line start (optionally after a spinner glyph) so this does NOT match the unrelated
          // "NN% until auto-compact" approaching-threshold indicator (lowercase "compact",
          // mid-line) — see claude_active.txt:5, claude_thinking_verb.txt:5, claude_asterism_active.txt:5.
          Pattern:     `(?im)^[ \t]*[·✢✳✶✻✽●*✦⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏]?[ \t]*Compacting\b`,
          Description: "Claude is compacting conversation history to free up context space",
          Priority:    26,
      },
  },
  ```
- Adjust the exact regex/description wording if Task 1.1.1a captured different real text than the INFERRED hypothesis.
- Files: `session/detection/binaries/claude.go`

---

### Epic 1.3: Fixture-Based Regression Tests
**Goal**: Prove the new status is reachable (AC4) and prove the three existing "N% until auto-compact" fixtures are unaffected (AC7).

#### Story 1.3.1: Positive detection test + AC7 negative-case regression guard
**As a** maintainer, **I want** a Go test asserting the new fixture classifies as `StatusCompacting` and the three pre-existing "approaching" fixtures do NOT, **so that** a future regex tweak can't silently regress either direction.
**Acceptance Criteria**:
- `go test ./session/detection/...` passes, including a new test for the positive case and an explicit negative-case assertion against the 3 existing fixtures.
  - *Given* `session/detection/testdata/claude_compacting.txt`'s contents, *When* run through the Claude per-binary `StatusDetector.DetectWithContext`, *Then* it returns `(StatusCompacting, "Claude is compacting conversation history to free up context space")`.
  - *Given* `claude_active.txt`, `claude_thinking_verb.txt`, and `claude_asterism_active.txt` (each containing "...10% until auto-compact"), *When* each is run through the same detector, *Then* each still returns its pre-existing expected status (`StatusExecuting`) and specifically NOT `StatusCompacting`.
**Files**: `session/detection/detector_test.go` (or a new `session/detection/compacting_test.go` if preferred for isolation — either satisfies AC4/AC7)

##### Task 1.3.1a: Add the positive `TestStatusDetector_DetectCompacting` test (~4 min)
- Follow the exact shape of `TestStatusDetector_DetectWaitingForAgent` (`detector_test.go:106-129`): load the Claude binary pattern set (via `binaries.NewClaudeDetector().Patterns()` + `NewPatternSet`, matching how other per-binary tests are structured — check `TestGeminiPatterns_AgyCoverage` at `detector_test.go:731` for the per-binary detector construction pattern), read `testdata/claude_compacting.txt`, call `DetectWithContext`, assert `status == StatusCompacting` and the description matches.
- If the fixture came from Task 1.1.1b (INFERRED), add a `// NOTE: fixture is INFERRED, not verified against a live capture — see plan.md Story 1.1.1` comment directly above the test function.
- Files: `session/detection/detector_test.go`

##### Task 1.3.1b: Add the AC7 negative-case regression test (~4 min)
- Add `TestStatusDetector_DetectActive_NotCompacting_ApproachingThresholdFixtures` — reads `claude_active.txt`, `claude_thinking_verb.txt`, `claude_asterism_active.txt` (already used elsewhere per `snapshot_test.go:40,146,165`), runs each through `DetectWithContext`, asserts each still equals its currently-expected status (check `snapshot_test.go`'s existing `fixture`/expected-status table for the exact pre-change values to assert unchanged) AND asserts `status != StatusCompacting` for all three.
- Run `go test ./session/detection/... -run 'Compacting|Snapshot'` locally to confirm both the new tests pass and the pre-existing snapshot tests for these 3 fixtures are unchanged (AC7's "no side effects" requirement).
- Files: `session/detection/detector_test.go`

---

## Phase 2: Proto Layer

### Epic 2.1: New Proto Enum Values + Codegen
**Goal**: Add the wire-level enum values both AC3 (DetectedStatus) and ADR-001 (SubStatus) require, append-only.

#### Story 2.1.1: Append `DETECTED_STATUS_COMPACTING` and `SUB_STATUS_COMPACTING`
**As a** maintainer, **I want** two new append-only proto enum values, **so that** the frontend can receive the new signal over the wire without breaking older clients.
**Acceptance Criteria**:
- `proto/session/v1/types.proto`'s `DetectedStatus` enum gains `DETECTED_STATUS_COMPACTING = 12;` and `SubStatus` enum gains `SUB_STATUS_COMPACTING = 11;`, both appended (not inserted/renumbered).
  - *Given* the regenerated Go/TS bindings, *When* `sessionv1.DetectedStatus_DETECTED_STATUS_COMPACTING` and `sessionv1.SubStatus_SUB_STATUS_COMPACTING` are referenced from Go, and `DetectedStatus.COMPACTING`/`SubStatus.COMPACTING` from TS, *Then* both compile.
**Files**: `proto/session/v1/types.proto`, generated: `gen/proto/go/session/v1/*.go`, `web-app/src/gen/session/v1/types_pb.ts`

##### Task 2.1.1a: Append both enum values (~2 min)
- In `proto/session/v1/types.proto:380-393`, add `DETECTED_STATUS_COMPACTING = 12;` as the last line before the enum's closing `}`, with a comment: `// Claude is actively compacting/summarizing conversation history (distinct from the "N% until auto-compact" approaching indicator).`
- In `proto/session/v1/types.proto:412-434`, add `SUB_STATUS_COMPACTING = 11;` as the last line before the enum's closing `}`, with a matching comment.
- Files: `proto/session/v1/types.proto`

##### Task 2.1.1b: Regenerate bindings (~2 min)
- Run `make proto-gen` from repo root.
- Confirm `gen/proto/go/session/v1/types.pb.go` contains `DetectedStatus_DETECTED_STATUS_COMPACTING` and `SubStatus_SUB_STATUS_COMPACTING`, and `web-app/src/gen/session/v1/types_pb.ts` contains the corresponding TS enum members.
- Files: `gen/proto/go/session/v1/*.go` (generated), `web-app/src/gen/session/v1/types_pb.ts` (generated)

---

### Epic 2.2: Go Mapping Functions (all four sites — none are lint-enforced, all must be updated by hand)
**Goal**: Wire `StatusCompacting` through both authoritative mapping functions AND the two adapter functions that actually feed the RPC responses — missing the adapters ships a no-op feature.

#### Story 2.2.1: `proto_mapping.go` — both `DetectedStatusToProto` and `DetectedStatusToSubStatus`
**As a** maintainer, **I want** the two authoritative mapping functions kept in sync with the adapters, **so that** package-level unit tests and any future caller of these functions (even though today's RPC path bypasses them) stay correct.
**Acceptance Criteria**:
- *Given* `StatusCompacting`, *When* passed to `DetectedStatusToProto`, *Then* it returns `sessionv1.DetectedStatus_DETECTED_STATUS_COMPACTING`; *When* passed to `DetectedStatusToSubStatus`, *Then* it returns `sessionv1.SubStatus_SUB_STATUS_COMPACTING`.
**Files**: `session/detection/proto_mapping.go`

##### Task 2.2.1a: Add both cases (~3 min)
- In `DetectedStatusToProto` (`proto_mapping.go:8-35`), add `case StatusCompacting: return sessionv1.DetectedStatus_DETECTED_STATUS_COMPACTING`.
- In `DetectedStatusToSubStatus` (`proto_mapping.go:44-70`), add `case StatusCompacting: return sessionv1.SubStatus_SUB_STATUS_COMPACTING`.
- Files: `session/detection/proto_mapping.go`

#### Story 2.2.2: `instance_adapter.go` `toProtoSubStatusFromInfo` — the actual RPC-path function (HIGHEST RISK)
**As a** maintainer, **I want** the function that literally sets `protoSession.SubStatus` on the main session list/watch RPC (`instance_adapter.go:167`) updated, **so that** the badge actually appears on the live session board — not just in unit tests of a dead-code mapping function.
**Acceptance Criteria**:
- *Given* an active session whose `InstanceStatusInfo.ClaudeStatus == detection.StatusCompacting`, *When* `toProtoSubStatusFromInfo` runs, *Then* it returns `sessionv1.SubStatus_SUB_STATUS_COMPACTING`, and this value flows through `instance_adapter.go:167` into `protoSession.SubStatus` on every session-list/watch response.
- **Added 2026-08-06 (Phase 4 pre-mortem, P1 #2, now mandatory not optional)**: a table-driven test iterating every `detection.DetectedStatus` value asserts both `toProtoSubStatusFromInfo` and `subStatusFromItem` (Story 2.2.3) return a non-`SUB_STATUS_UNSPECIFIED` value for every status that has a defined mapping — turning "manually remember to update two duplicate switches" into a CI-enforced guarantee. This test is a blocking completion condition for Epic 2.2, not an optional nice-to-have.
**Files**: `server/adapters/instance_adapter.go`, `server/adapters/review_queue_adapter.go`

##### Task 2.2.2a: Add the case to `toProtoSubStatusFromInfo` (~3 min)
- In `server/adapters/instance_adapter.go`'s `toProtoSubStatusFromInfo` (lines 240-272), add `case detection.StatusCompacting: return sessionv1.SubStatus_SUB_STATUS_COMPACTING` in the `switch info.ClaudeStatus` block (alongside the other cases at lines 248-269) — placement in the switch is order-independent (Go `switch` doesn't fall through), but the rate-limit precedence check above the switch (line 245) must remain unchanged so `RATE_LIMITED` still wins over `COMPACTING` if both are somehow true simultaneously.
- **This is the site most likely to be forgotten** — it has no compile-time enforcement (no `exhaustive` lint here, per `.golangci.yml:85-86` excluding `^server/`). Task 2.2.2b below (mandatory, not conditional) is the required guard against that.
- Files: `server/adapters/instance_adapter.go`

##### Task 2.2.2b: Mandatory table-driven parity test across both adapter switches (~15 min)
- Add `TestToProtoSubStatus_CoversAllDetectedStatusValues` (or extend an existing adapter test file) that iterates every named `detection.DetectedStatus` constant (`StatusReady`, `StatusProcessing`, `StatusExecuting`, `StatusNeedsApproval`, `StatusInputRequired`, `StatusError`, `StatusTestsFailing`, `StatusIdle`, `StatusSuccess`, `StatusWaitingForAgent`, `StatusCompacting` — NOT `StatusUnknown`, which legitimately maps to `SUB_STATUS_UNSPECIFIED`) and asserts both `toProtoSubStatusFromInfo` (`instance_adapter.go`) and `subStatusFromItem` (`review_queue_adapter.go`) return a non-`SUB_STATUS_UNSPECIFIED` value for each.
- This directly prevents the pre-mortem's #2 failure mode: `StatusCompacting` silently missing from one of the two duplicate switches while the other is updated, backend detection logs show it working, but the session card in the browser doesn't — a discrepancy that looks like a frontend bug.
- Run this test as part of Task 2.2.3a below (same PR), not deferred.
- Files: `server/adapters/instance_adapter_test.go` (or a shared adapter test file covering both functions)

#### Story 2.2.3: `review_queue_adapter.go` `subStatusFromItem`
**As a** maintainer, **I want** the review-queue panel's independent SubStatus derivation updated too, **so that** a compacting session shown in the review queue also gets the correct chip.
**Acceptance Criteria**:
- *Given* a `ReviewItem` whose `ClaudeStatus == detection.StatusCompacting`, *When* `subStatusFromItem` runs, *Then* it returns `sessionv1.SubStatus_SUB_STATUS_COMPACTING`.
**Files**: `server/adapters/review_queue_adapter.go`

##### Task 2.2.3a: Add the case (~2 min)
- In `server/adapters/review_queue_adapter.go`'s `subStatusFromItem` (lines 12-36), add `case detection.StatusCompacting: return sessionv1.SubStatus_SUB_STATUS_COMPACTING`.
- Files: `server/adapters/review_queue_adapter.go`

#### Story 2.2.4: `status_mapping.go` `AttentionReasonFromDetected` + `instance_status.go` icon/description polish
**As a** maintainer, **I want** compaction explicitly grouped with the "no attention needed" states, and the human-readable status icon/description to say "Compacting" instead of falling through to a generic default, **so that** compaction never wrongly surfaces in the review queue and status text reads correctly wherever it's shown.
**Acceptance Criteria**:
- *Given* `StatusCompacting`, *When* passed to `AttentionReasonFromDetected`, *Then* it returns `""` (zero `AttentionReason` — no review-queue entry), matching the requirements' non-goal that compaction should not alter session lifecycle/attention behavior.
- *Given* `InstanceStatusInfo{ClaudeStatus: StatusCompacting}`, *When* `GetStatusDescription()` is called, *Then* it returns `"Compacting"` (not the generic `"Unknown"` default fallback).
**Files**: `session/status_mapping.go`, `session/instance_status.go`

##### Task 2.2.4a: Join `StatusCompacting` to the "no attention needed" group (~2 min)
- In `session/status_mapping.go`'s `AttentionReasonFromDetected` (line 17-37), add `detection.StatusCompacting` to the existing no-attention case at line 32: `case detection.StatusExecuting, detection.StatusProcessing, detection.StatusWaitingForAgent, detection.StatusReady, detection.StatusUnknown, detection.StatusCompacting:`.
- Files: `session/status_mapping.go`

##### Task 2.2.4b: Add icon/description cases in `instance_status.go` (~3 min)
- In `GetStatusIcon()` (`instance_status.go:108-124`), add `case detection.StatusCompacting: return "⟳"` before the `default:`.
- In `GetStatusDescription()` (`instance_status.go:146-169`), add `case detection.StatusCompacting: desc = "Compacting"` alongside the other cases.
- Both functions have a `default:` clause (not `exhaustive`-linted here — `session/instance_status.go` doesn't match the `session/detect*` exclusion carve-out, so double check `make lint` still passes; if `instance_status.go` turns out to be in a linted path, this task becomes required rather than optional-polish).
- Files: `session/instance_status.go`

---

## Phase 3: Frontend

### Epic 3.1: `WorkingState` Derivation + `StatusBadge` Fallback
**Goal**: Satisfy AC5 (deriveWorkingState mapping + test) and keep `StatusBadge.tsx`'s compile-enforced `assertNever` switch exhaustive.

#### Story 3.1.1: `deriveWorkingState.ts` — both switches
**As a** frontend developer, **I want** `SubStatus.COMPACTING` and `DetectedStatus.COMPACTING` each mapped to a `WorkingState`, **so that** review-queue filtering by working state correctly buckets compacting sessions as "still busy," and the compile-enforced fallback switch doesn't break the build.
**Acceptance Criteria**:
- `deriveWorkingState({ subStatus: SubStatus.COMPACTING })` returns `WorkingState.PROCESSING` (joins the existing `PROCESSING`/`NEEDS_APPROVAL`/`INPUT_REQUIRED`/`ERROR`/`TESTS_FAILING`/`RATE_LIMITED`/`WAITING_FOR_AGENT` → `PROCESSING` group — "still busy, not idle").
  - *Given* `{ subStatus: SubStatus.COMPACTING }`, *When* `deriveWorkingState` is called, *Then* it returns `WorkingState.PROCESSING`, covered by a new Jest test in `deriveWorkingState.test.ts`.
- `deriveWorkingState({ subStatus: SubStatus.UNSPECIFIED, detectedStatus: DetectedStatus.COMPACTING })` returns `WorkingState.ACTIVE` (joins the `EXECUTING`/`WAITING_FOR_AGENT` → `ACTIVE` group — compaction is actively-running work with an interrupt available, same category as those two).
  - *Given* `{ subStatus: SubStatus.UNSPECIFIED, detectedStatus: DetectedStatus.COMPACTING }`, *When* `deriveWorkingState` is called, *Then* it returns `WorkingState.ACTIVE`, covered by a new Jest test.
**Files**: `web-app/src/lib/utils/deriveWorkingState.ts`, `web-app/src/lib/utils/__tests__/deriveWorkingState.test.ts`

##### Task 3.1.1a: Add both switch cases (~3 min)
- In the primary `SubStatus` switch (`deriveWorkingState.ts:27-43`), add `case SubStatus.COMPACTING:` to the existing `return WorkingState.PROCESSING;` group (line 28-35).
- In the fallback `DetectedStatus` switch (`deriveWorkingState.ts:46-66`), add `case DetectedStatus.COMPACTING:` to the existing `return WorkingState.ACTIVE;` group (line 47-49), **above** the `assertNever(session.detectedStatus)` default — required or the build breaks (TS compile error, not just a lint warning).
- Update the file's top-of-function doc comment (lines 4-22) to document the new mapping.
- Files: `web-app/src/lib/utils/deriveWorkingState.ts`

##### Task 3.1.1b: Add Jest tests for both new cases (~3 min)
- In `web-app/src/lib/utils/__tests__/deriveWorkingState.test.ts`, add two test cases following the existing naming convention: one asserting `SubStatus.COMPACTING → WorkingState.PROCESSING`, one asserting the `DetectedStatus.COMPACTING` fallback path → `WorkingState.ACTIVE`.
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="deriveWorkingState"` to confirm.
- Files: `web-app/src/lib/utils/__tests__/deriveWorkingState.test.ts`

#### Story 3.1.2: `StatusBadge.tsx` — keep the compile-enforced `assertNever` switch exhaustive
**As a** frontend developer, **I want** `getDetectedStatusInfo` to handle `DetectedStatus.COMPACTING`, **so that** the TS build doesn't fail (this switch is `assertNever`-guarded, not just linted).
**Acceptance Criteria**:
- *Given* `DetectedStatus.COMPACTING` passed to `getDetectedStatusInfo`, *When* evaluated, *Then* it returns `{ label: "Compacting", icon: "⟳", variant: "processing" }` — note this fallback badge will rarely actually render in practice, because `SessionCard.tsx`'s precedence rule (Story 3.2.2) shows `SubStatusChip` first whenever `subStatus` is non-idle, which it will be during compaction; this case exists purely to satisfy the compile-time exhaustiveness guard and to cover the rare non-Active-session edge case where only `detectedStatus` is set.
**Files**: `web-app/src/components/sessions/StatusBadge.tsx`

##### Task 3.1.2a: Add the case (~2 min)
- In `web-app/src/components/sessions/StatusBadge.tsx`'s `getDetectedStatusInfo` (lines 39-68), add `case DetectedStatus.COMPACTING: return { label: "Compacting", icon: "⟳", variant: "processing" };` before the `default: return assertNever(status);` line — this MUST be added or `npx tsc` / `next build` fails to compile once the new enum value exists.
- Files: `web-app/src/components/sessions/StatusBadge.tsx`

---

### Epic 3.2: `SubStatusChip` — the Actual Badge UI (AC6)
**Goal**: Ship the visible "⟳ Compacting context" pill on the session card, using vanilla-extract tokens matching `chipWaitingForAgent`'s calm framing, per `.claude/rules/css-architecture.md`.

#### Story 3.2.1: `chipCompacting` vanilla-extract variant
**As a** frontend developer, **I want** a new `chipCompacting` style export composed from the shared `chip` base and the same `accentBg`/`primary` tokens `chipWaitingForAgent` uses, **so that** the compacting badge visually reads as "calm, self-managing, no action needed" rather than a warning.
**Acceptance Criteria**:
- `chipCompacting` is exported from `SubStatusChip.css.ts`, uses only `vars.*` token references (no hardcoded hex/`var()` strings), and respects the existing `prefers-reduced-motion` handling on the shared `spinner` class.
  - *Given* the `chipCompacting` class is applied, *When* rendered under `prefers-reduced-motion: reduce`, *Then* the spinner shows the existing static dot (from the shared `spinner` style, lines 122-144), not an animated arc — no new CSS needed for this since `chipCompacting` reuses the existing `spinner` element, it doesn't need its own animation rule.
**Files**: `web-app/src/components/sessions/SubStatusChip.css.ts`

##### Task 3.2.1a: Add the `chipCompacting` export (~3 min)
- In `web-app/src/components/sessions/SubStatusChip.css.ts`, add (near `chipWaitingForAgent`, lines 111-120):
  ```ts
  export const chipCompacting = style([
    chip,
    {
      background: vars.color.accentBg,
      color: vars.color.primary,
      border: `1px solid ${vars.color.primary}`,
      fontWeight: vars.fontWeight.normal,
    },
  ]);
  ```
- **Fixed 2026-08-06 (Phase 4 cross-artifact consistency check, BLOCKER)**: this task originally omitted a `fontWeight` override, which — since the shared `chip` base sets `fontWeight: vars.fontWeight.semibold` — would have rendered `chipCompacting` at full semibold weight, directly contradicting `design/ux.md`'s Visual spec (`"fontSize xs, fontWeight normal (matches chipWaitingForAgent, not chipProcessing's semibold — slightly quieter than 'Thinking…')"`). Reconciled by adopting `ux.md`'s spec: `chipCompacting` takes `chipWaitingForAgent`'s `fontWeight: normal` but keeps `chipProcessing`'s token pair and full opacity (still "actively working, no user action needed," just visually quieter than the semibold default) — it does NOT take `chipWaitingForAgent`'s `opacity: 0.85`, since compaction is a foreground activity like Processing, not a background wait.
- Files: `web-app/src/components/sessions/SubStatusChip.css.ts`

#### Story 3.2.2: `SubStatusChip.tsx` new case + Jest test
**As a** user watching the session board, **I want** to see a distinct "⟳ Compacting context" badge instead of the generic "Thinking…" chip during compaction, **so that** I know the session is self-managing and not stuck or waiting on me.
**Acceptance Criteria**:
- `SubStatusChip({ subStatus: SubStatus.COMPACTING })` renders `<span className={chipCompacting} role="status" aria-label="Compacting context" title="Claude is summarizing older conversation history to free up context space">⟳ Compacting context</span>`, and no other `SubStatus` case's rendered output changes (additive-only per AC7's spirit extended to the frontend).
  - *Given* `session.status === SessionStatus.ACTIVE` and `session.subStatus === SubStatus.COMPACTING`, *When* `SessionCard` renders (`SessionCard.tsx:543-548`'s existing condition already covers this — `subStatus !== UNSPECIFIED && subStatus !== IDLE`), *Then* the `⟳ Compacting context` pill is visible and the `StatusBadge` fallback at lines 533-539 does NOT also render (mutually exclusive per the existing `session.subStatus === UNSPECIFIED || IDLE` gate).
**Files**: `web-app/src/components/sessions/SubStatusChip.tsx`, `web-app/src/components/sessions/__tests__/SubStatusChip.test.tsx`

##### Task 3.2.2a: Add the `case SubStatus.COMPACTING:` render branch (~3 min)
- In `web-app/src/components/sessions/SubStatusChip.tsx`, add (near `case SubStatus.WAITING_FOR_AGENT:`, before the `default:` exhaustiveness guard at lines 157-168 — required, this switch has the same `_exhaustive: never` compile guard as `StatusBadge.tsx`):
  ```tsx
  case SubStatus.COMPACTING:
    return (
      <span
        className={chipCompacting}
        role="status"
        aria-label="Compacting context"
        title="Claude is summarizing older conversation history to free up context space"
      >
        <span className={spinner} aria-hidden="true" />
        ⟳ Compacting context
      </span>
    );
  ```
- Import `chipCompacting` from `./SubStatusChip.css` alongside the existing named imports at the top of the file (line 4-16).
- Files: `web-app/src/components/sessions/SubStatusChip.tsx`

##### Task 3.2.2b: Add the Jest test (~3 min)
- In `web-app/src/components/sessions/__tests__/SubStatusChip.test.tsx`, add a test following the existing per-`SubStatus` case pattern: render `<SubStatusChip subStatus={SubStatus.COMPACTING} />`, assert the text `⟳ Compacting context` is present and the `chipCompacting` class is applied (or query by `role="status"` + accessible name "Compacting context", matching however existing tests query other cases).
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="SubStatusChip"` to confirm.
- Files: `web-app/src/components/sessions/__tests__/SubStatusChip.test.tsx`

#### Story 3.2.3 (optional polish, not required by any AC): `SessionRow.tsx` `rowActive` highlight
**As a** user scanning the session list, **I want** a compacting session's row to get the same subtle "active" row highlight that Processing/WaitingForAgent sessions get, **so that** the row-level visual treatment stays consistent with the new chip.
**Acceptance Criteria**:
- *Given* `session.status === SessionStatus.ACTIVE` and `session.subStatus === SubStatus.COMPACTING`, *When* `SessionRow` renders, *Then* the row gets the `rowActive` class, matching the existing `PROCESSING`/`WAITING_FOR_AGENT` treatment.
**Files**: `web-app/src/components/sessions/SessionRow.tsx`

##### Task 3.2.3a: Extend the `rowActive` condition (~2 min)
- In `web-app/src/components/sessions/SessionRow.tsx:196-199`, add `SubStatus.COMPACTING` to the existing `||` condition: `(session.subStatus === SubStatus.PROCESSING || session.subStatus === SubStatus.WAITING_FOR_AGENT || session.subStatus === SubStatus.COMPACTING)`.
- This is not required by any acceptance criterion — skip if time-constrained; it's a minor consistency improvement flagged by `architecture.md`'s "not enforced anywhere" list, not a functional gap.
- Files: `web-app/src/components/sessions/SessionRow.tsx`

---

## Phase 4: Registry & Final Verification

### Epic 4.1: Feature Registry (AC8)
**Goal**: Register the new frontend-observable badge surface per `.claude/rules/feature-registry.md`.

#### Story 4.1.1: New frontend registry entry
**As a** maintainer, **I want** the new "compacting context" badge registered as a discrete frontend feature, **so that** `docs/registry/coverage-gaps.json` correctly reflects test coverage and doesn't silently grow.
**Acceptance Criteria**:
- A new file `docs/registry/features/frontend/session-compacting-badge.json` exists with `tested: true` and `testIds` pointing at the new Jest test name(s) from Task 3.2.2b.
  - *Given* the new registry file and `make registry-generate`, *When* `docs/registry/coverage-gaps.json` is regenerated, *Then* its total gap count does not increase (the new feature ships already-tested).
**Files**: `docs/registry/features/frontend/session-compacting-badge.json`

##### Task 4.1.1a: Create the registry entry and regenerate (~3 min)
- Create `docs/registry/features/frontend/session-compacting-badge.json` following the exact shape of the example in `docs/registry/features/frontend/approval-analytics-reason-breakdown.json`:
  ```json
  {
    "id": "session-compacting-badge",
    "type": "frontend",
    "name": "Compacting context badge on the session card",
    "component": "SubStatusChip",
    "path": "web-app/src/components/sessions/SubStatusChip.tsx",
    "filePath": "web-app/src/components/sessions/SubStatusChip.tsx",
    "markerLine": 0,
    "tested": true,
    "testIds": ["<exact describe/test name from Task 3.2.2b>"],
    "lastModified": "2026-08-06T00:00:00Z"
  }
  ```
- Run `make registry-generate` from repo root; confirm `git diff docs/registry/` shows only additive changes and `docs/registry/coverage-gaps.json`'s count did not increase.
- Files: `docs/registry/features/frontend/session-compacting-badge.json`, `docs/registry/*.json` (regenerated aggregates)

---

### Epic 4.2: Final Verification
**Goal**: Confirm the additive-only guarantee (AC7) holds across the whole stack before this is considered done.

#### Story 4.2.1: Full verification pass
**As a** maintainer, **I want** every relevant check green before calling this shippable, **so that** "done" is backed by an actual command run, not a claim.
**Acceptance Criteria**:
- *Given* all prior tasks complete, *When* `make lint`, `go test ./session/detection/...`, and the two frontend Jest suites are run, *Then* all pass with zero pre-existing test regressions.
**Files**: N/A (verification only)

##### Task 4.2.1a: Run the full verification suite (~5 min)
- `make lint` — confirms the `exhaustive` linter is satisfied for all 6 `session/detection/` sites (Story 1.2.1) and no new lint debt elsewhere.
- `go test ./session/detection/...` — confirms Story 1.3.1's positive + negative tests pass and, critically, that no pre-existing fixture's expected status changed (AC7).
- `cd web-app && npx jest --no-coverage --testPathPatterns="deriveWorkingState|SubStatusChip"` — confirms Stories 3.1.1/3.2.2's new tests pass.
- `make ci` — if time allows, run the full pipeline as the definitive pre-PR check (build, all tests, lint, registry validation).
- **If Task 1.1.1b's INFERRED fallback was used instead of a verified live capture (Task 1.1.1a)**: confirm Task 1.1.1c's two gating conditions before calling this shippable — a follow-up backlog item exists and is linked in the PR description, and the temporary canary log line is present in `detector.go`. This is a manual check (not CI-enforced, unlike Task 2.2.2b) — do not skip it.
- Files: N/A
