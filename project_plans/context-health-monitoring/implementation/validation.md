# Validation Plan: context-health-monitoring

**Date**: 2026-08-02

## Happy Path Scenario

Given a running Claude Code session whose JSONL transcript accumulates a trailing
20-turn window containing 6 consecutive identical `Bash({"command":"git status"})`
tool calls (≥ default `LoopRepeatThreshold` of 3, doubled → red), when `TokenStore`
re-parses the transcript on its next fsnotify-triggered pass and `publishContextHealth`
evaluates the new `ContextHealthSignals` against `ContextHealthConfig`, then the
session's `Instance` transitions from `HealthGreen`/`HealthUnknown` to `HealthRed`, an
info-level `[ContextHealth] level transition` log line is emitted, a `session_updated`
event carrying `["context_health"]` is published over the existing `WatchSessions`
stream, and the session's card in the web UI renders a `✖ Context Needs Attention`
badge (last position in the badge row) whose tooltip/`aria-label` reads
`"Context health: Context Needs Attention — Repeated the same Bash call 6 times in a
row"` — with no page refresh and no new RPC call from the frontend.

## Requirement → Test Mapping

Requirement IDs follow `requirements.md`'s Scope → In Scope bullets:
- **R1a** — loop-detection heuristic (tool-call fingerprinting + streak counting)
- **R1b** — confusion/apology-language detection heuristic
- **R2** — green/amber/red indicator on the session card + tooltip (backend verdict + frontend badge)
- **R3** — configurable thresholds for the two heuristics
- **R4** — single new backend data point exposed via the existing ConnectRPC session surface

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| R1a | `session/tokens/context_health_test.go` | `TestExtractContextHealthSignals_CountsConsecutiveIdenticalCalls` | Unit (happy) | 3×`Bash("git status")` + 1×`Read` → `MaxConsecutiveRepeats==3`, `RepeatedToolName=="Bash"` (plan Task 1.2.1e) |
| R1a | `session/tokens/context_health_test.go` | `TestExtractContextHealthSignals_DistinctArgsAreNotRepeats` | Unit (error/negative) | `Read("/a.go")`,`Read("/b.go")`,`Read("/c.go")` → `MaxConsecutiveRepeats==1`, `RepeatedToolName==""` (plan Task 1.2.1e) |
| R1a | `session/tokens/context_health_test.go` | `TestToolCallFingerprint_IsStableAcrossWhitespaceAndCase` | Unit (happy — normalization) | Whitespace/case-differing inputs to the same tool hash to the same `ToolCallFingerprint` (plan Task 1.2.1e) |
| R1a | `session/tokens/context_health_test.go` | `TestParseReader_DoesNotRetainToolInput` | Unit (negative — privacy/security regression) | `tool_use` input containing `"SECRET_TOKEN=abc123"` never appears in `fmt.Sprintf("%+v", result)` (plan Task 1.2.1e; enforces the Domain Glossary's "no message content" contract on `ContextHealthSignals`) |
| R1a | `session/tokens/context_health_test.go` | `TestParseReader_PopulatesContextHealthSignals` | Integration (Parser walk over a real JSONL fixture) | 3 identical `Bash("npm test")` turns → `result.ContextHealth.MaxConsecutiveRepeats==3`, `ToolCallsInWindow==3` in one `ParseReader` pass, no second file read (plan Task 1.2.3b) |
| R1a | `session/tokens/context_health_test.go` | `TestParseReader_OnlyTrailingWindowContributes` | Integration | 4 identical calls + 25 distinct `Read` turns → early loop ages out of the `healthWindowTurns=20` ring, `MaxConsecutiveRepeats==1` (plan Task 1.2.3b) |
| R1a | `session/tokens/context_health_test.go` | `TestParseReader_SkipsSyntheticTurnsInHealthWindow` | Integration | A `"<synthetic>"`-model turn between two identical `Bash` calls does not break the streak, matching the existing `TurnTimeline` synthetic-skip rule (plan Task 1.2.3b) |
| R1b | `session/tokens/context_health_test.go` | `TestMatchConfusionPatterns_MatchesApologyPhrasing` | Unit (happy) | `"I apologize — that didn't work..."` → returns pattern name `"apology"`, never the raw text (plan Task 1.2.2b) |
| R1b | `session/tokens/context_health_test.go` | `TestMatchConfusionPatterns_IgnoresOrdinaryProse` | Unit (error/negative) | `"Sorted the imports and re-ran the tests; all 42 pass."` → returns `""` (plan Task 1.2.2b) |
| R1b | `session/tokens/context_health_test.go` | `TestMatchConfusionPatterns_FastPathSkipsLongNonMatchingText` | Unit (edge — perf/NFR guard) | 4 KB marker-free text returns `""` without invoking the regexp engine, protecting the <5 ms NFR on large text blocks (plan Task 1.2.2b) |
| R1b | `session/tokens/context_health_test.go` | `TestExtractContextHealthSignals_CountsConfusionAcrossWindow` | Integration (window aggregation) | 6-of-20 turns match → `ConfusionPhraseCount==6`, `LastConfusionPhrase` is a pattern name (plan Task 1.2.2b) |
| R2 (backend verdict) | `session/tokens/context_health_test.go` | `TestEvaluateContextHealth` (table rows: amber-at-threshold, red-at-2×-threshold, red-both-signals, green-no-signal) | Unit (happy) | Config thresholds + `ContextHealthSignals` → correct `ContextHealthLevel` + human-readable `Reason` string (plan Task 1.3.1b) |
| R2 (backend verdict) | `session/tokens/context_health_test.go` | `TestEvaluateContextHealth` (row: `ToolCallsInWindow < MinToolCallSamples`) | Unit (error/edge — insufficient-data floor) | `ToolCallsInWindow:3` with strong signals still yields `HealthUnknown`/`Reason==""` — never a false green or false amber on a young session (plan Task 1.3.1b, Story 1.3.1 1st AC) |
| R2 (backend verdict) | `session/tokens/context_health_test.go` | `TestEvaluateContextHealth_ZeroConfigUsesDefaults` | Unit (edge) | `config.ContextHealthConfig{}` behaves identically to the defaulted config — no nil/zero-value crash (plan Task 1.3.1b) |
| R2 (frontend badge) | `web-app/src/components/sessions/ContextHealthBadge.test.tsx` | `ContextHealthBadge_should_renderAmberPill_When_HealthAmber` | Unit (Jest/RTL, happy) | `health=AMBER`, `reason` set → `⚠ Context Degrading` pill, accessible name `"Context health: Context Degrading — <reason>"` (plan Task 3.1.1c) |
| R2 (frontend badge) | `web-app/src/components/sessions/ContextHealthBadge.test.tsx` | `ContextHealthBadge_should_renderRedPillWithDistinctGlyph_When_HealthRed` | Unit (Jest/RTL, happy) | `health=RED` → `✖` glyph (not a recolored `⚠`) (plan Task 3.1.1c) |
| R2 (frontend badge) | `web-app/src/components/sessions/ContextHealthBadge.test.tsx` | `ContextHealthBadge_should_renderNull_When_HealthGreenOrUnspecified` | Unit (Jest/RTL, negative) | `health=GREEN` and `health=UNSPECIFIED` both render `null` (plan Task 3.1.1c, Story 3.1.1 2nd AC) |
| R2 (frontend badge) | `web-app/src/components/sessions/ContextHealthBadge.test.tsx` | `ContextHealthBadge_should_returnNullAndWarn_When_UnrecognizedHealthValue` | Unit (Jest/RTL, error/edge) | `health=7 as ContextHealth` (forward-compat) → `null` + `console.warn`, no throw (plan Task 3.1.1c, Story 3.1.1 3rd AC) |
| R2 (frontend badge) | `web-app/src/components/sessions/ContextHealthBadge.test.tsx` | `ContextHealthBadge_should_appendPausedSuffix_When_IsPausedTrue` | Unit (Jest/RTL, happy) | `isPaused=true` → accessible name ends `" (paused)"` (plan Task 3.1.1c, Story 3.1.1 5th AC) |
| R2 (SessionCard integration) — **GAP, no test names in plan** | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard_should_renderContextHealthBadgeLast_When_MultipleBadgesPresent` (recommended) | Integration (component composition) | `subStatus===PROCESSING`, `memoryRssMb===400n`, `contextHealth===AMBER` → badge row DOM order ends with `ContextHealthBadge` (covers Story 3.2.1 1st AC — plan's Task 3.2.1a/b only implement + re-run existing suites, no new test named) |
| R2 (SessionCard integration) — **GAP** | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard_should_renderBothErrorChipAndHealthBadge_When_SubStatusErrorAndHealthRed` (recommended) | Integration | `subStatus===ERROR` + `contextHealth===RED` → both `✖ Error` and `✖ Context Needs Attention` present, distinct `role="status"` accessible names (covers Story 3.2.1 2nd AC) |
| R2 (SessionCard integration) — **GAP** | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard_should_passIsPausedToHealthBadge_When_SessionPaused` (recommended) | Integration | `status===SessionStatus.PAUSED` + `contextHealth===RED` → health badge accessible name ends `" (paused)"` (covers Story 3.2.1 3rd AC) |
| R2 (e2e wiring) | `tests/e2e/context-health-badge.spec.ts` | `context-health-badge > suppresses badge for a freshly created session` | E2E (Playwright) | Freshly created session → `getByTestId("badge-context-health")` has count 0 (plan Task 3.3.1b — the only deterministic e2e assertion without a synthetic-looping-agent fixture) |
| R3 | `config/config_test.go` | `TestContextHealthConfigOrDefault_AppliesDefaultsToZeroAndNegativeFields` | Unit (happy) | `{0, -4, 0}` → `{3, 5, 5}` (plan Task 1.1.1c) |
| R3 | `config/config_test.go` | `TestLoadConfigFromPath_PartialContextHealthBlockKeepsSetFieldAndDefaultsRest` | Integration (reads a real temp `config.json` from disk) | `{"loop_repeat_threshold": 7}` → `LoopRepeatThreshold==7`, `ConfusionPhraseThreshold==5` (default) (plan Task 1.1.1c) |
| R3 | `config/config_test.go` | `TestLoadConfig_MalformedContextHealthBlockFallsBackToDefaults` | Integration (error path, real file I/O) | `"context_health": "nonsense"` (string, not object) → `LoadConfig()` logs a warning, returns `DefaultConfig()`, no panic, `LoopRepeatThreshold==3` (plan Task 1.1.1c) |
| R4 | `session/tokens/proto_mapping_test.go` | `TestContextHealthLevelToProto` (table: Green/Amber/Red) | Unit (happy) | Each `ContextHealthLevel` maps to its exact proto enum value (plan Task 2.2.1a) |
| R4 | `session/tokens/proto_mapping_test.go` | `TestContextHealthLevelToProto` (row: `HealthUnknown` and `ContextHealthLevel(99)`) | Unit (error/edge) | Both map to `CONTEXT_HEALTH_UNSPECIFIED` — exhaustive `switch` never panics on an out-of-range value (plan Task 2.2.1a) |
| R4 | `server/adapters/instance_adapter_test.go` | `TestInstanceToProto_PopulatesContextHealthFields` | Integration (crosses `Instance`→proto boundary) | Verdict `{HealthRed, "…"}` → `proto.ContextHealth==CONTEXT_HEALTH_RED`, `proto.ContextHealthReason` equals the reason string (plan Task 2.2.2d) |
| R4 | `server/adapters/instance_adapter_test.go` | `TestInstanceToProto_LeavesContextHealthUnspecifiedWhenNeverComputed` | Integration (error/edge) | An `Instance` that never had `SetContextHealth` called (e.g. an Aider session — ADR-001) → `CONTEXT_HEALTH_UNSPECIFIED`, `ContextHealthReason==""` (plan Task 2.2.2d) |
| R4 | `session/instance_test.go` (existing file, extend) | `TestInstance_SetContextHealth_UpdatesSnapshotUnderRace` (recommended, not named in plan) | Unit/race | `inst.SetContextHealth(verdict)` then `inst.Snapshot().ContextHealth.Level` matches; `go test -race ./session/...` clean (covers Story 2.2.2 1st AC, which plan.md states as an AC but assigns no dedicated test — Task 2.2.2a is implementation-only) |
| R4 (`publishContextHealth` wiring) — **GAP, no automated test in plan (Task 2.2.3b is a manual smoke check only)** | `server/dependencies_test.go` or a new `server/context_health_publish_test.go` (recommended) | `TestShouldPublishContextHealth_WhenLevelChanges_ExpectTrue` (recommended — requires extracting the trigger predicate as a small pure function, e.g. `shouldPublishContextHealthTransition(prev, next tokens.ContextHealthLevel) bool`, out of the `server/dependencies.go` closure so it is unit-testable without standing up `TokenStore`/`EventBus`/`HistoryLinker`) | Unit | `prev=HealthGreen, next=HealthAmber` → `true` (log + publish) |
| R4 (`publishContextHealth` wiring) — **GAP** | same as above | `TestShouldPublishContextHealth_WhenLevelSameButReasonDiffers_ExpectFalse` (recommended) | Unit | `prev=next=HealthAmber` even though the underlying `Reason` string's embedded count changed → `false` (this is the exact case Story 2.2.3's 2nd AC calls out as "flagged as untested by adversarial review" — plan.md line ~421 — and it remains untested today) |
| R4 (`publishContextHealth` wiring) — **GAP** | same as above | `TestPublishContextHealth_WhenClaudeConversationUUIDEmpty_ExpectInstanceSkipped` (recommended) | Integration (fake `TokenStore`/`HistoryLinker`) | An Aider-style instance with `GetClaudeConversationUUID()==""` → no `SetContextHealth` call, no event (Story 2.2.3 3rd AC) |
| R4 (`publishContextHealth` wiring) — **GAP** | same as above | `TestPublishContextHealth_WhenNotificationHandlingPanics_ExpectGoroutineSurvivesAndLogsRecovery` (recommended) | Integration | A panic inside one instance's evaluation is recovered and logged; subsequent notifications on the same subscriber channel are still processed (verifies the panic-recovery wrapper added in the recent adversarial-review patch actually prevents the "silently and permanently stops all future ContextHealth updates" failure mode it names) |
| Migration | N/A | N/A | N/A | **N/A — no persisted schema, see plan.md Migration Plan.** `ContextHealth` is recomputed from the JSONL transcript on every parse and deliberately not persisted (mirrors `detected_status`); `TokenStore.walkAndEnqueue` re-parses on restart, so there is no migration-reversibility test to design. |

## UX Acceptance Tests

Source: `design/ux.md` "UX acceptance criteria (human-testable)" (8 numbered criteria).
Two structural facts constrain tool choice here: (1) the e2e harness (`tests/e2e/global-setup.ts`)
spins up an isolated server with no synthetic-looping-agent fixture, so **only the
suppressed (GREEN/UNSPECIFIED → null) state is deterministically reachable via a real
Playwright session** — amber/red states are exercised by constructing the React
component directly in Jest/RTL, exactly as plan.md Task 3.3.1b already concludes; (2)
criterion 7 (color contrast) is a static token-value check already covered by this
repo's existing Axe Core / Lighthouse CI gate on PRs touching `web-app/src/` (see
CLAUDE.md's E2E Tests section), not a new bespoke test.

| UX Criterion | Test File | Test Name | Tool | Steps |
|---|---|---|---|---|
| 1. At-a-glance distinction (fixed last position, no color-only signal) | `web-app/src/components/sessions/SessionCard.test.tsx` | `SessionCard_should_renderContextHealthBadgeLast_When_MultipleBadgesPresent` | Jest/RTL (structural proxy — a 2-second glance timing claim can't be machine-verified; position + distinct label text are) | Render `SessionCard` with a full badge set + `contextHealth=RED`; assert `ContextHealthBadge` is the last child in the badges row and its accessible name contains no other badge's label text |
| 2. Hover reveals the specific reason within ~1s beyond Radix's 400ms delay | `web-app/src/components/sessions/ContextHealthBadge.test.tsx` | `ContextHealthBadge_should_showReasonInTooltip_When_Hovered` | Jest/RTL (`userEvent.hover` + `findByRole("tooltip")`, since the badge itself carries `reason` synchronously via props — no async fetch to wait on) | Render badge with `health=AMBER`, `reason="Repeated the same Bash call 3 times in a row"`; `userEvent.hover(getByTestId("badge-context-health"))`; assert the tooltip's accessible content matches verbatim (no truncation) |
| 3. Keyboard users can reach the same information — **currently unmet as specced (design/ux.md Gap 1)** | `web-app/src/components/sessions/ContextHealthBadge.test.tsx` | `ContextHealthBadge_should_openTooltipOnFocus_When_TabbedTo` | Jest/RTL (`userEvent.tab()` + `fireEvent.focus`) | **Blocked on implementation fix**: add `tabIndex={0}` to the badge's `<span>` in Task 3.1.1b (not yet in the plan's JSX spec). Once added: tab to the badge, assert it receives focus and the tooltip opens via Radix's `onFocus` handler, with the same accessible name as the hover case. Write this test to fail against the plan's current unmodified JSX so the gap is caught at review, not discovered later |
| 4. No dead ends (no `onClick`, no destructive action, no modal) | `web-app/src/components/sessions/ContextHealthBadge.test.tsx` | `ContextHealthBadge_should_haveNoClickHandler_When_Rendered` | Jest/RTL | Render the badge; assert the rendered `<span>` has no `onClick`/`onKeyDown`-driven navigation (query the DOM node's own listeners via `userEvent.click` + a `jest.fn()` spy on `router.push`/`window.location` remaining uncalled) |
| 5. Tooltip text is never truncated (~97-char worst case string) | `web-app/src/components/sessions/ContextHealthBadge.test.tsx` | `ContextHealthBadge_should_renderFullReasonText_When_ReasonAndPausedBothPresent` | Jest/RTL | Render with the longest specced reason (`"Repeated the same Bash call 3 times in a row"`) + `isPaused=true`; assert the full ~97-char string is present verbatim in both `title` and `aria-label` (no `…`, no substring match) |
| 6. Screen-reader label present and correctly worded (`role="status"`, contains "Context health") | `web-app/src/components/sessions/ContextHealthBadge.test.tsx` | `ContextHealthBadge_should_exposeContextHealthAccessibleName_When_HealthNonNull` | Jest/RTL | For each of `AMBER`/`RED`: `getByRole("status", { name: /Context health/ })` resolves; for `GREEN`/`UNSPECIFIED`: query returns nothing (matches plan Story 3.1.1 4th AC) |
| 7. Color contrast ≥ 4.5:1 for badge text | *(no new test — existing CI gate)* | Axe Core WCAG AA check (already runs in "UX analysis CI" per project `CLAUDE.md`) | Axe Core / Lighthouse CI (existing, not new) | No bespoke test needed; the design doc's computed ratios (6.37:1 light amber, 6.8:1 light red, 5.43:1 dark amber, 5.28:1 dark red) are verified at PR time by the existing Axe Core gate on any PR touching `web-app/src/` — flag only if that gate is not actually wired to run against this component in CI (verify during Phase 6, not assumed here) |
| 8. Paused sessions read as "attention when resumed," not "act now" | `web-app/src/components/sessions/ContextHealthBadge.test.tsx` | `ContextHealthBadge_should_appendPausedSuffix_When_IsPausedTrue` | Jest/RTL | Same test as R2 row above (`isPaused=true` → accessible name ends `" (paused)"`); duplicated here because it is also the direct verification of UX criterion 8 — no separate implementation, same assertion serves both the requirement and the UX acceptance criterion |
| (suppression, cross-cutting) | `tests/e2e/context-health-badge.spec.ts` | `context-health-badge > suppresses badge for a freshly created session` | Playwright | `// @feature session:list, context-health-badge` header; `test.describe("context-health-badge", ...)`; navigate to a freshly created session's card; `await expect(page.getByTestId("badge-context-health")).toHaveCount(0)` — the one UX-relevant fact reachable end-to-end without a synthetic-looping-agent fixture (plan Task 3.3.1b) |

## Test Stack

- **Unit (Go)**: stdlib `testing`, table-driven tests following this repo's local
  convention in `session/tokens/*_test.go` — `TestX_WhenCondition_ExpectOutcome` /
  `TestX_DescriptiveCase` (verified against existing `session/tokens/parser_test.go`,
  `session/tokens/pricing_test.go` naming, which the plan's own task list already
  follows exactly — no `_should_..._When_...` convention is used on the Go side of
  this repo). Race detector (`go test -race`) required for anything touching
  `Instance`'s actor-serialized state (Story 2.2.2, `.claude/rules/go-double-checked-locking.md`).
- **Integration (Go)**: same `testing` package, no separate framework — "integration"
  here means the test exercises a real file-backed path (`ParseReader` over an actual
  JSONL fixture via `strings.NewReader`, `LoadConfigFromPath` against a real temp
  `config.json`, `InstanceToProto` crossing the `Instance`→proto adapter boundary)
  rather than a pure function in isolation.
- **Frontend unit/component (Jest/RTL)**: `npx jest --no-coverage --testPathPatterns="ContextHealthBadge|SessionCard"`,
  following the `_should_<effect>_When_<condition>` naming convention already in use in
  this directory (`ProgramDetailPanel.test.tsx`, `SuggestedRuleCard.test.tsx`) — this is
  the correct local convention to copy, not the plan's terser Go-style names.
- **E2E / UX**: Playwright, per `.claude/rules/e2e-test-conventions.md` — `// @feature`
  header, `getByTestId`/`getByRole` locators only, no `waitForTimeout`, page helpers
  under `tests/e2e/pages/` (none needed for the single suppression test currently
  planned). Run via `cd tests/e2e && npx playwright test context-health-badge.spec.ts`;
  the isolated test server is spun up automatically by `global-setup.ts`.
- **Accessibility**: existing Axe Core / Lighthouse CI gate (per project `CLAUDE.md`
  "UX analysis CI" — runs on PRs touching `web-app/src/`), covering UX criterion 7
  (contrast) with no new bespoke test.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line, with `session/tokens/context_health.go` and `session/tokens/proto_mapping.go` specifically checked (`go tool cover -func=coverage.out \| grep context_health`) since they carry all new domain logic |
| TypeScript/Jest | `npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` | ≥80% line, `ContextHealthBadge.tsx` should be at or near 100% given its small, fully-branch-tested surface (6 states × happy/edge) |

- All public service methods: happy path + error paths covered — see table above; the
  one confirmed exception is `publishContextHealth`'s trigger-rule predicate and
  panic-recovery wrapper, which today has **zero automated coverage** (Task 2.2.3b is a
  manual second-instance smoke check only). This is the most consequential gap in this
  validation pass because it is exactly the logic two separate adversarial-review
  BLOCKERs were raised against in the recent plan patch (level-only trigger rule;
  panic-recovery) — a regression here fails silently (no log, no event, no crash) and
  would not be caught by `make build && make test` as currently scoped. Recommend
  extracting `shouldPublishContextHealthTransition(prev, next tokens.ContextHealthLevel) bool`
  as a standalone pure function (small, no interface, consistent with
  `.claude/rules/interface-pollution-checklist.md`'s "concrete type first" guidance) so
  it can be unit-tested without a live `TokenStore`/`EventBus`/`HistoryLinker`, before
  Phase 5 implementation begins.
- All external integrations: this feature adds no new external calls (per
  `requirements.md` Non-functional Requirements — "no new external network calls"); the
  nearest equivalent is the `TokenStore.Subscribe()` push channel, covered by the
  `publishContextHealth` gap above rather than a literal external-service mock.
- UX acceptance criteria: 8 of 8 criteria in `design/ux.md` have a corresponding test or
  explicit manual/CI step in the table above; criterion 3 (keyboard access) additionally
  requires an implementation change (`tabIndex={0}`) not yet reflected in plan.md Task
  3.1.1b's JSX spec — flagged so it is fixed at implementation time rather than only
  documented as a known gap.

## Summary of Testing Gaps Found (for Phase 5 implementer)

1. **`publishContextHealth` trigger-rule and panic-recovery logic has no automated
   test** — only Task 2.2.3b's manual second-instance log check. Given this is the
   exact logic two adversarial-review BLOCKERs targeted, recommend extracting a pure
   `shouldPublishContextHealthTransition` predicate and adding the four tests listed in
   the Requirement → Test Mapping table's R4 rows before this ships.
2. **`SessionCard` integration tests for Story 3.2.1's three ACs are not named
   anywhere in plan.md** (Task 3.2.1a is implementation-only; Task 3.2.1b only re-runs
   existing suites). Three recommended tests added above.
3. **Keyboard accessibility (`tabIndex={0}`) is a known gap in the plan's JSX spec**
   (design/ux.md's own Gap 1), not just a missing test — the fix belongs in Task 3.1.1b
   itself, with the accompanying focus test added above.
4. **`Instance.SetContextHealth`'s race-safety AC (Story 2.2.2, 1st bullet) has no
   dedicated test name in plan.md** — Task 2.2.2a is implementation-only. Recommended
   addition included above.
