# Implementation Plan: context-health-monitoring

**Feature**: Derive a per-session green/amber/red `ContextHealth` signal (tool-call loop + apology/confusion language) from the existing Claude Code JSONL parsing pipeline, and surface it as a distinct badge + tooltip on the session card.
**Date**: 2026-08-02
**Status**: Ready for implementation
**ADRs**: `decisions/ADR-001-claude-code-only-health-substrate.md`, `decisions/ADR-002-push-context-health-onto-instance.md`

---

## Step 0.5 — Creative Pass: Where does ContextHealth computation live?

Three approaches were considered before committing.

| # | Approach | Strength (1 sentence) | Weakness (1 sentence) |
|---|----------|----------------------|----------------------|
| **(a)** | **Extend `session/tokens`** — compute signals inside the existing `Parser` walk over JSONL content blocks, store on `ParseResult`, evaluate against config at the consumer. **← CHOSEN** | The parser already decodes exactly the two inputs both heuristics need (`jsonlContent.Name` + `jsonlContent.Input` for `tool_use`, `jsonlContent.Text` for assistant text — `session/tokens/jsonl_types.go:34-45`) inside a pipeline that is already fsnotify-driven, worker-pooled, cached, and subscriber-notified end to end (`session/tokens/store.go:26-222`), so the marginal cost is one extra pass over an already-decoded struct. | The substrate is Claude-Code-specific (`session/tokens/doc.go`), so Aider/Gemini/OpenCode/Agy sessions get no signal at all — accepted and scoped explicitly in ADR-001. |
| (b) | New standalone `session/contexthealth` package consuming `TokenStore`'s output (`GetAll()`/`Subscribe()`) | Keeps the privacy-sensitive `session/tokens` package untouched and gives health its own testable module boundary. | `ParseResult` deliberately discards `jsonlContent.Input` (the tool arguments — `session/tokens/types.go:5-7`, PerfFix-4 note at `jsonl_types.go:48-51`), so a downstream consumer would have to **re-open and re-parse every JSONL file** to get loop-detection inputs — a second full parse pass per file change, directly violating the <5 ms NFR and duplicating the whole store/cache/watcher machinery. |
| (c) | Inline in `InsightsService` (`server/services/insights_service.go`) | Zero new files; the service already holds a `TokenStoreReader` and already streams aggregates over `WatchInsights`. | `InsightsService` is a *dashboard-level aggregate* RPC, not a per-session surface — the badge needs the value on the per-session `Session` proto that `WatchSessions` already streams, and putting domain computation in a ConnectRPC handler puts business logic in the transport layer. |

**Chosen: (a).** It matches `research/architecture.md`'s recommendation, is the only option with access to tool arguments without a second parse, and adds no new subsystem.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `ContextHealth` | The feature as a whole: a derived per-session judgement of whether the agent's reasoning appears to be degrading. | Not a Go type by itself — the concrete types below carry it. |
| `ContextHealthLevel` | Go `int` iota enum with exactly four values: `HealthUnknown`, `HealthGreen`, `HealthAmber`, `HealthRed`. | Sum type; exhaustive `switch` in `ContextHealthLevelToProto`. `HealthUnknown` is the zero value so an unset struct is never a false green. |
| `ContextHealthSignals` | Struct of **raw, config-independent** counts extracted from the JSONL transcript window: `MaxConsecutiveRepeats`, `RepeatedToolName`, `ConfusionPhraseCount`, `LastConfusionPhrase`, `ToolCallsInWindow`, `WindowEndsAt`. | Lives on `ParseResult`. Contains no message content — only tool names, a pattern *name*, and counts. |
| `ContextHealthVerdict` | Struct `{Level ContextHealthLevel; Reason string; Signals ContextHealthSignals}` — the result of applying config thresholds to `ContextHealthSignals`. | Computed at the consumer, not in the parser, so a threshold change takes effect without re-parsing files. |
| `ToolCallFingerprint` | `uint64` murmur3 hash of `toolName + "\x00" + normalizeToolInput(input)` identifying "the same tool call with the same-ish arguments". | Hash only — the raw `Input` bytes are never retained, preserving `session/tokens`' privacy guarantee. |
| `normalizeToolInput` | Function that canonicalises a raw `json.RawMessage` tool input before fingerprinting: trims whitespace, lowercases, and truncates to `maxFingerprintBytes`. | Deliberately exact-match-after-normalisation, not fuzzy (see Pattern Decisions). |
| `LoopSignal` | The loop-detection half of `ContextHealthSignals`: `MaxConsecutiveRepeats` (longest run of identical `ToolCallFingerprint` values in the window) plus `RepeatedToolName`. | Not a separate Go type — named here because the term appears in reason strings and tests. |
| `ConfusionSignal` | The confusion-detection half of `ContextHealthSignals`: `ConfusionPhraseCount` (assistant text blocks in the window matching `confusionPatterns`) plus `LastConfusionPhrase` (the matched pattern's *name*, e.g. `"apology"`). | Stores the pattern name, never the matched text. |
| `confusionPatterns` | Package-level `[]confusionPattern{Name, Regexp}` compiled once via `regexp.MustCompile` at init in `session/tokens/context_health.go`. | Mirrors `session/tokens/skill_detector.go:11-14`'s compile-once package-var idiom. |
| `healthTurnRing` | Fixed-capacity ring buffer of the last `healthWindowTurns` assistant-turn records (`ToolCallFingerprint` list + confusion hits), maintained by `Parser` during a file walk. | Sized-down copy of the `eventRing` idiom in `session/detection/events.go:28-34`. Bounds work to O(window), not O(session length). |
| `healthWindowTurns` | Package constant (`= 20`): how many trailing assistant turns the signals are computed over. | Fixed, not user-configurable in v1 — keeps `ParseResult` config-independent. |
| `ContextHealthConfig` | Config struct in `config/types.go` holding the three user-tunable thresholds. | Registered on `Config` as `json:"context_health,omitempty"`. |
| `LoopRepeatThreshold` | `ContextHealthConfig` field: consecutive identical `ToolCallFingerprint` repeats needed to raise the level. Default `3`. | `<= 0` means "unset → default", per the `*OrDefault()` idiom. |
| `ConfusionPhraseThreshold` | `ContextHealthConfig` field: confusion-pattern matches within the window needed to raise the level. Default `5`. | Same `<= 0` → default rule. |
| `MinToolCallSamples` | `ContextHealthConfig` field: minimum `ToolCallsInWindow` before any non-`HealthUnknown` level is emitted. Default `5`. | Implements UX research §4's "never show a false green on a 3-turn-old session". |
| `ContextHealthConfigOrDefault` | Method `func (c ContextHealthConfig) ContextHealthConfigOrDefault() ContextHealthConfig` returning a copy with defaults applied to zero/negative fields. | Exact shape of `CapacityConfig.CapacityConfigOrDefault()` (`config/types.go:274-299`). |
| `EvaluateContextHealth` | `func EvaluateContextHealth(sig ContextHealthSignals, cfg config.ContextHealthConfig) ContextHealthVerdict` — the single place thresholds are applied. | Pure function; no receiver, no interface. |
| `ContextHealthLevelToProto` | `func ContextHealthLevelToProto(l ContextHealthLevel) sessionv1.ContextHealth` — the single authoritative Go→proto mapping. | Direct copy of `session/detection/proto_mapping.go:8-35`'s convention. |
| `ContextHealth` (proto enum) | `enum ContextHealth` in `proto/session/v1/types.proto` with `CONTEXT_HEALTH_{UNSPECIFIED,GREEN,AMBER,RED}`. | Field `context_health = 72` on `Session`. |
| `context_health_reason` | Proto `string` field `= 73` on `Session` carrying the human-readable reason. | Empty when `context_health` is `UNSPECIFIED` — mirrors `detected_context = 69`'s contract. |
| `publishContextHealth` | Closure wired in `server/dependencies.go` that reads the `TokenStore.Subscribe()` channel, evaluates each session's verdict, calls `Instance.SetContextHealth`, logs level transitions, and publishes a `session_updated` event. | Follows the same push shape as `artifactExtractor.OnScanComplete` (`server/dependencies.go:1114-1138`) but is **not an exact mirror**: `TokenStore.Subscribe()` broadcasts a payload-less `struct{}` signal (verified in `session/tokens/store.go`), so unlike `OnScanComplete`'s single-title callback, every notification re-scans **all** live instances. Per-instance work is cheap (map read + pure function), so this is accepted as-is (adversarial-review CONCERN, not a blocker), but note the O(files re-parsed × live instances) startup-burst shape rather than assuming it's identical to the `Artifacts` precedent. |
| `ContextHealthBadge` | React component `web-app/src/components/sessions/ContextHealthBadge.tsx` rendering the icon+label chip with a Radix tooltip. | Separate file + `.css.ts`, per `.claude/rules/css-architecture.md`. |
| `getContextHealthInfo` | TS `function getContextHealthInfo(health: ContextHealth): ContextHealthInfo \| null` mapping the proto enum to `{label, icon, variant}`; returns `null` for `UNSPECIFIED`. | Modelled on `getDetectedStatusInfo` (`StatusBadge.tsx:39-68`). |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Where health is computed | Extend `session/tokens.Parser` in place; add `ContextHealthSignals` to `ParseResult` | `research/architecture.md` §1; existing `SkillActivations` precedent (`session/tokens/skill_detector.go`) | (b) new `session/contexthealth` package consuming `TokenStore` output; (c) inline in `InsightsService` | (b) can't see `jsonlContent.Input` — `ParseResult` discards it (`types.go:5-7`), forcing a second full parse of every JSONL file, breaking the <5 ms NFR; (c) puts domain logic in a ConnectRPC handler and targets the wrong (aggregate, not per-session) surface |
| Substrate for both heuristics | Claude Code JSONL transcript (`session/tokens`) | `research/architecture.md` §1 table | `session/detection`'s `PatternSet` pipeline (recommended by `research/build-vs-buy.md` Option 4) | `PatternSet.MatchLines` classifies terminal text into one of ten status categories and carries **no tool identity or arguments at all**; `DetectionEvent` stores only a category + 512-byte snippet (`session/detection/events.go:8-26`). Loop detection needs `(toolName, args)`, which only the JSONL path has. This plan follows `architecture.md` over `build-vs-buy.md` where they conflict — see ADR-001. |
| `ContextHealthLevel` | Go `int` iota sum type + exhaustive `switch` in `ContextHealthLevelToProto` | type-driven-design; `detection.DetectedStatus` (`session/detection/proto_mapping.go`) | `string` level (`"green"`/`"amber"`/`"red"`) | Primitive obsession: a string admits illegal values, gets no compiler exhaustiveness help, and would need runtime validation at the proto boundary |
| Analyzer shape | Two package-level functions (`extractContextHealthSignals`, `EvaluateContextHealth`) + parser-local `healthTurnRing`; **no new type with methods** | `.claude/rules/interface-pollution-checklist.md` items 1 & 4 | A `ContextHealthAnalyzer` struct or `ContextHealthComputer` interface | Speculative interface with exactly one implementation, and a forwarding-only wrapper over `Parser`. `architecture.md` §3 explicitly names this as the anti-pattern to avoid here |
| Threshold application | Raw `ContextHealthSignals` on `ParseResult`; thresholds applied later in `EvaluateContextHealth` | Parse-don't-validate / separation of measurement from policy | Applying thresholds inside `Parser.ParseFile` | Would make `ParseResult` config-dependent, force `Parser` to carry a `config.Config`, and require re-parsing every JSONL file whenever a user edits a threshold |
| Argument similarity | Exact match on murmur3 `ToolCallFingerprint` after `normalizeToolInput` | `research/stack.md` §1; `research/build-vs-buy.md` §3a; `github.com/spaolacci/murmur3` already direct-imported (`session/circular_buffer.go:10`, `pkg/analytics/escape_code_parser.go:14`) | `agext/levenshtein` fuzzy distance (currently an indirect dep) | Promoting an unused indirect dep on day one for an unvalidated fuzzy-matching need; exact-after-normalisation is O(1) per call and cheap. Escalate only if Phase 6 manual review shows false negatives |
| Bounded history | `healthTurnRing` fixed at `healthWindowTurns = 20` assistant turns | `session/detection/events.go:28-34` `eventRing` idiom; `research/pitfalls.md` §1 | Scanning the whole `TurnTimeline` per parse | Unbounded O(n) growth over a multi-hour session; also makes health "sticky forever" — one early loop would pin the badge red for the session's life |
| Confusion matching | Compile-once package-level `confusionPatterns` regex slice | `session/tokens/skill_detector.go:11-14`; `session/detection/pattern_set.go` compile-at-construction | An NLP/sentiment library | `requirements.md` Rabbit Holes: "a single well-scoped heuristic module, not a general NLP problem"; a library adds weight, latency, and per-threshold untunability |
| Concurrency | Compute signals inside `Parser.ParseReader` (no lock); they ride into the cache inside `parseAndCache`'s single existing `ts.mu.Lock()` (`store.go:211-219`) | `.claude/rules/go-double-checked-locking.md`; `architecture.md` §3 | A new per-session health cache with its own read-lock → miss → compute → write-lock path | That is precisely the double-checked-locking shape the rule was written for. Here there is **no new mutex and no re-read after write** — `parseAndCache` stores the locally-built `result` and never re-`Load()`s it |
| Delivery to the frontend | Push: `publishContextHealth` → `Instance.SetContextHealth` → `InstanceSnapshot` → `InstanceToProto` → existing `WatchSessions` stream | `Instance.SetArtifacts` + `artifactExtractor.OnScanComplete` (`session/dependencies.go:1114-1138`, `session/instance.go:743-749`, proto field `artifacts = 70`) | Pull: `InstanceToProto` queries a `TokenStoreReader` directly, as `architecture.md` §2 suggests | `InstanceToProto(inst, workflowNames)` has **24 call sites** (verified: `grep -rn "InstanceToProto(" --include="*.go" \| wc -l` → 24) and no store handle; threading one through changes every call site and 6 tests. The push model is already proven for the structurally identical JSONL-derived `Artifacts` field and gives level-transition logging for free. See ADR-002 |
| Proto shape | `enum ContextHealth` field `= 72` + `string context_health_reason = 73` | `detected_status = 68` / `detected_context = 69` (`proto/session/v1/types.proto:222-230`) | A nested `ContextHealthState` message with counts and timestamps | Answers `requirements.md`'s Open Question in favour of the minimal enum+reason shape; richer introspection (which heuristic, streak length, first-occurrence) belongs to the deferred debug surface, and the reason string already encodes the count |
| Frontend badge | New `ContextHealthBadge.tsx` + `ContextHealthBadge.css.ts`, distinct icon shapes, fixed last position in the badge row | `research/ux.md` §2-3; `.claude/rules/css-architecture.md` | Adding a `contextHealth` prop to the existing `StatusBadge` (as `research/stack.md` §5 suggests) | `research/ux.md` §2 is explicit that reusing `StatusBadge`/`SubStatusChip`'s exact form makes a transient "Error" visually indistinguishable from a cumulative "Red health". Distinct component = distinct visual weight and a stable scan position |
| Badge suppression | Render `null` for `UNSPECIFIED` **and** for `GREEN` | `SubStatusChip` returns `null` for `UNSPECIFIED`; `memoryBadge` hides at `mb <= 0` (`SessionCard.tsx:549-551`) | Always rendering a green tick | Twelve concurrent green ticks are noise, not signal (`research/ux.md` §2 "silent by default at green") |

**No non-standard technology is introduced.** Every dependency (`regexp`, `murmur3`), every mechanism (fsnotify→worker pool→cache→subscriber, ConnectRPC field on `Session`, vanilla-extract badge), and every config idiom already exists in this repo. Two ADRs are written — not for technology risk, but because both record a *scope/architecture constraint that contradicts a Phase-2 research recommendation* and future readers will otherwise re-litigate them.

---

## Migration Plan

*Omitted — no schema or data changes.* `ContextHealth` is recomputed from the JSONL transcript on every parse and is deliberately **not** persisted, matching `detected_status`, which has no ent column either (`session/ent/schema/session.go` holds only the coarse lifecycle `status` int). On restart, `TokenStore.walkAndEnqueue` (`session/tokens/store.go:225-255`) re-parses every transcript, so health recomputes naturally with no stale-state migration.

## Observability Plan

- **Logs**:
  - `log.Info("[ContextHealth] level transition", "session", inst.Title, "from", prev, "to", next, "reason", verdict.Reason, "loopRepeats", sig.MaxConsecutiveRepeats, "confusionCount", sig.ConfusionPhraseCount)` — emitted from `publishContextHealth` **only when the level actually changes**, satisfying `requirements.md` Observability Requirements and `research/pitfalls.md` §5's condition-change gating (no per-poll re-log).
  - `log.Warn("[ContextHealth] malformed context_health config, using defaults", ...)` — not needed as a new call site; `LoadConfig`'s existing malformed-JSON path already logs and returns `DefaultConfig()`.
- **Metrics**: none added. This is a single-user local tool with no metrics backend; the transition log lines are the review substrate for the false-positive assessment `requirements.md` schedules for Phase 6.
- **Alerts**: none. `requirements.md` is explicit: "No new oncall alert — this is a user-facing advisory signal."

## Risk Control

- **Feature flag**: none beyond config. Setting `context_health.loop_repeat_threshold` and `context_health.confusion_phrase_threshold` to large values (e.g. `9999`) makes the badge effectively silent without a code change — the mitigation `requirements.md` Risk Control already prescribes. A `<= 0` value means "unset → use default", never "disabled" (`research/pitfalls.md` §4 failure-safe interpretation).
- **Rollback procedure**: standard PR revert. Nothing is persisted, so a revert leaves no orphaned data or schema; the proto fields simply stop being populated and `getContextHealthInfo` returns `null` for the zero value.
- **Staged rollout**: not applicable (single local binary, no multi-tenant deploy). Validate against the live transcript corpus by running the second-instance pattern from `CLAUDE.md` (`PORT=8999 STAPLER_SQUAD_INSTANCE=claude-manual-test`) — **never** `make install-service`, which restarts the live `:8543` unit.

## Unresolved Questions

- [ ] Are `confusionPatterns`' default phrases tuned well enough against real transcripts to keep the false-positive rate acceptable? — does **not** block any story; it is the explicit Phase 6 manual-review task `requirements.md` Success Metrics already schedules — owner: implementer at Phase 6.
- [ ] Should a future `ContextHealthLevel` transition also feed the smart-notification-dedup path (`project_plans/smart-notification-dedup/`)? — out of scope this iteration (no toast/notification is being added); revisit if a notification is ever attached — owner: follow-on requirements.

## Explicit Follow-Ons (out of scope — no tasks planned here)

Per `requirements.md` "Out of Scope", none of the following has a task in this plan:
1. Token-usage-percentage as a third signal (blocked on `project_plans/token-cost-tracking`).
2. Approval-request-rate-spike signal.
3. "Restart with summary" handoff-packet generation + new-session creation (7-touchpoint registry).
4. Rules-engine integration gating auto-approvals on red health.
5. Approval-analytics health dimension.
6. **Non-Claude-Code agent backends** (Aider/Gemini/OpenCode/Agy) — see ADR-001. Those sessions produce no Claude JSONL transcript, so `context_health` stays `UNSPECIFIED` and the badge stays suppressed. This is correct-by-suppression, not a silent failure.
7. A `.claude/rules/` entry formalising badge/alert fatigue (`research/pitfalls.md` §5) — worth doing once both this and smart-notification-dedup have shipped.

## Dependency Visualization

```
                       Phase 1 — Backend signal
  ┌──────────────────────────────────────────────────────────────┐
  │ 1.1 config.ContextHealthConfig + OrDefault + registration     │
  │        (config/types.go, config/config.go)                    │
  │                          │                                    │
  │ 1.2 ContextHealthSignals extraction in Parser  ───────────┐   │
  │        (session/tokens/context_health.go, parser.go,       │  │
  │         jsonl_types.go, types.go)                          │  │
  │                          │                                 │  │
  │ 1.3 EvaluateContextHealth (needs 1.1 + 1.2)  ◄─────────────┘  │
  └──────────────────────────┬───────────────────────────────────┘
                             │
                       Phase 2 — Wire to the wire
  ┌──────────────────────────▼───────────────────────────────────┐
  │ 2.1 proto enum + fields 72/73  →  make proto-gen              │
  │                          │                                    │
  │ 2.2 ContextHealthLevelToProto (needs 1.3 + 2.1)               │
  │                          │                                    │
  │ 2.3 Instance.SetContextHealth + snapshot + InstanceToProto    │
  │                          │                                    │
  │ 2.4 publishContextHealth wiring (needs 1.3, 2.2, 2.3)         │
  └──────────────────────────┬───────────────────────────────────┘
                             │  (TS bindings emitted by 2.1)
                       Phase 3 — Frontend badge
  ┌──────────────────────────▼───────────────────────────────────┐
  │ 3.1 ContextHealthBadge.tsx + .css.ts + tests                  │
  │                          │                                    │
  │ 3.2 SessionCard integration                                   │
  │                          │                                    │
  │ 3.3 Feature registry + e2e spec                               │
  └──────────────────────────────────────────────────────────────┘

Phase 1 and 2.1 are independent and may run in parallel.
Everything in Phase 3 depends on `make proto-gen` from 2.1 having emitted
web-app/src/gen/session/v1/types_pb.ts.
```

---

## Phase 1: Backend signal computation

### Epic 1.1: Configurable thresholds

**Goal**: Add `ContextHealthConfig` to the config surface with safe defaults, following the `CapacityConfig` idiom exactly, so a malformed or missing `context_health` JSON block can never crash startup or produce a nonsensical threshold.

#### Story 1.1.1: Threshold config struct with safe defaults

**As a** stapler-squad user, **I want** loop and confusion thresholds configurable in `config.json`, **so that** I can raise them if the badge is noisy without rebuilding the binary.

**Acceptance Criteria**:
- `ContextHealthConfig` exposes `LoopRepeatThreshold`, `ConfusionPhraseThreshold`, and `MinToolCallSamples` as `json`-tagged `omitempty` ints.
  - *Given* a `config.json` containing `{"context_health": {"loop_repeat_threshold": 7}}`, *When* `LoadConfigFromPath` runs, *Then* `cfg.ContextHealth.LoopRepeatThreshold == 7` and `cfg.ContextHealth.ConfusionPhraseThreshold == 5` (the default, since it was absent).
- `ContextHealthConfigOrDefault()` treats any `<= 0` field as unset and substitutes the default — never as a literal zero threshold.
  - *Given* `ContextHealthConfig{LoopRepeatThreshold: 0, ConfusionPhraseThreshold: -4, MinToolCallSamples: 0}`, *When* `ContextHealthConfigOrDefault()` is called, *Then* it returns `ContextHealthConfig{LoopRepeatThreshold: 3, ConfusionPhraseThreshold: 5, MinToolCallSamples: 5}`.
- A malformed `context_health` block does not crash or block startup.
  - *Given* a `config.json` whose `context_health` value is the string `"nonsense"` instead of an object, *When* `LoadConfig()` runs, *Then* it logs a warning and returns `DefaultConfig()` with `ContextHealth.LoopRepeatThreshold == 3` — using `LoadConfig`'s existing parse-error fallback path, with no new error handling added.

**Files**: `config/types.go`, `config/config.go`, `config/config_test.go`

##### Task 1.1.1a: Add `ContextHealthConfig` struct + `ContextHealthConfigOrDefault` (~4 min)
- In `config/types.go`, immediately after `CapacityConfigOrDefault` (ends line 299), add the `ContextHealthConfig` struct with the three `int` fields, each `json:"...,omitempty"` and each doc-commented with its default.
- Add `func (c ContextHealthConfig) ContextHealthConfigOrDefault() ContextHealthConfig` following `CapacityConfigOrDefault`'s copy-then-fix-zero-fields shape (`config/types.go:275-299`): copy `c` into `out`, and for each field `if out.X <= 0 { out.X = <default> }`.
- Declare the three defaults as named constants (`defaultLoopRepeatThreshold = 3`, `defaultConfusionPhraseThreshold = 5`, `defaultMinToolCallSamples = 5`) next to the struct, not as inline literals.
- Files: `config/types.go`

##### Task 1.1.1b: Register `ContextHealth` on `Config` and default it at both call sites (~3 min)
- In `config/config.go`, add `ContextHealth ContextHealthConfig \`json:"context_health,omitempty"\`` to the `Config` struct immediately after the `Capacity` field (line 330).
- In `DefaultConfig()`, add `cfg.ContextHealth = ContextHealthConfig{}.ContextHealthConfigOrDefault()` immediately after the existing `cfg.Capacity = ...` line (line 452).
- In `LoadConfigFromPath`, add `cfg.ContextHealth = cfg.ContextHealth.ContextHealthConfigOrDefault()` immediately after the existing `cfg.Capacity = cfg.Capacity.CapacityConfigOrDefault()` line (line 894).
- Files: `config/config.go`

##### Task 1.1.1c: Unit tests for defaulting and malformed-config fallback (~5 min)
- In `config/config_test.go`, add `TestContextHealthConfigOrDefault_AppliesDefaultsToZeroAndNegativeFields` asserting the exact Given-When-Then values above.
- Add `TestLoadConfig_MalformedContextHealthBlockFallsBackToDefaults` writing a temp `config.json` with `"context_health": "nonsense"` and asserting `LoadConfig()` returns a config with `ContextHealth.LoopRepeatThreshold == 3` and no panic.
- Add `TestLoadConfigFromPath_PartialContextHealthBlockKeepsSetFieldAndDefaultsRest` for the `{"loop_repeat_threshold": 7}` case.
- Files: `config/config_test.go`

---

### Epic 1.2: Signal extraction from the JSONL transcript

**Goal**: Compute `ContextHealthSignals` during the existing `Parser` walk, over a bounded trailing window, without retaining any message content.

#### Story 1.2.1: Tool-call fingerprinting and loop-streak counting

**As** the ContextHealth analyzer, **I want** each `tool_use` block reduced to a `ToolCallFingerprint`, **so that** I can count consecutive identical calls without storing tool arguments.

**Acceptance Criteria**:
- `normalizeToolInput` produces the same output for inputs differing only in surrounding whitespace or case, and truncates at `maxFingerprintBytes`.
  - *Given* two `tool_use` blocks with `Name: "Bash"` and inputs `{"command": "git status"}` and `{"command":  "GIT STATUS"  }`, *When* `toolCallFingerprint` is computed for each, *Then* both return the same `ToolCallFingerprint` value.
- Consecutive identical fingerprints increment the streak; a differing fingerprint resets it to 1.
  - *Given* a transcript window whose `tool_use` blocks are `Bash("git status")`, `Bash("git status")`, `Bash("git status")`, `Read("/tmp/a.go")`, *When* `extractContextHealthSignals` runs, *Then* `Signals.MaxConsecutiveRepeats == 3` and `Signals.RepeatedToolName == "Bash"`.
- Distinct arguments to the same tool do **not** count as repeats.
  - *Given* `Read("/a.go")`, `Read("/b.go")`, `Read("/c.go")`, *When* signals are extracted, *Then* `Signals.MaxConsecutiveRepeats == 1` and `Signals.RepeatedToolName == ""`.
- No raw tool input is retained anywhere on `ContextHealthSignals` or `ParseResult`.
  - *Given* a `tool_use` block whose input contains the literal string `"SECRET_TOKEN=abc123"`, *When* the resulting `ParseResult` is serialised with `%+v`, *Then* the output contains neither `SECRET_TOKEN` nor `abc123`.

**Files**: `session/tokens/context_health.go` (new), `session/tokens/types.go`, `session/tokens/context_health_test.go` (new)

##### Task 1.2.1a: Create `session/tokens/context_health.go` with the signal types (~4 min)
- New file `session/tokens/context_health.go`, package `tokens`.
- Define `ContextHealthLevel int` with `const ( HealthUnknown ContextHealthLevel = iota; HealthGreen; HealthAmber; HealthRed )` and a `String()` method (for the transition log line).
- Define `ContextHealthSignals` with exactly: `MaxConsecutiveRepeats int`, `RepeatedToolName string`, `ConfusionPhraseCount int`, `LastConfusionPhrase string`, `ToolCallsInWindow int`, `WindowEndsAt time.Time`. Doc-comment that it holds no message content.
- Define `ContextHealthVerdict{Level ContextHealthLevel; Reason string; Signals ContextHealthSignals}`.
- Define constants `healthWindowTurns = 20` and `maxFingerprintBytes = 256`.
- Files: `session/tokens/context_health.go`

##### Task 1.2.1b: Implement `normalizeToolInput` + `toolCallFingerprint` (~4 min)
- In `session/tokens/context_health.go`, add `func normalizeToolInput(raw json.RawMessage) string`: return `""` for empty input; otherwise `strings.ToLower(strings.TrimSpace(string(raw)))` truncated to `maxFingerprintBytes` bytes.
- Add `func toolCallFingerprint(name string, raw json.RawMessage) uint64` returning `murmur3.Sum64([]byte(name + "\x00" + normalizeToolInput(raw)))`, importing `github.com/spaolacci/murmur3` (already a direct dependency — `session/circular_buffer.go:10`).
- Files: `session/tokens/context_health.go`

##### Task 1.2.1c: Implement `healthTurnRing` and `extractContextHealthSignals` (~5 min)
- In `session/tokens/context_health.go`, define an unexported `healthTurnRing` struct holding `entries [healthWindowTurns]healthTurnRecord`, `head int`, `count int`, plus `push(healthTurnRecord)` and `inOrder() []healthTurnRecord`. No mutex — it is parser-local, never shared across goroutines (copy the shape of `session/detection/events.go:28-34`, minus the lock).
- Define `healthTurnRecord{Fingerprints []uint64; ToolNames []string; ConfusionHits []string; At time.Time}`.
- Add `func extractContextHealthSignals(ring *healthTurnRing) ContextHealthSignals` walking `ring.inOrder()` in order, tracking the running fingerprint streak across turn boundaries, and returning the populated struct.
- Files: `session/tokens/context_health.go`

##### Task 1.2.1d: Add `ContextHealth ContextHealthSignals` to `ParseResult` (~2 min)
- In `session/tokens/types.go`, add `ContextHealth ContextHealthSignals` to `ParseResult` immediately after `SkillActivations` (line 22), with a doc comment noting it holds derived counts only, consistent with the package privacy note at lines 5-7.
- Files: `session/tokens/types.go`

##### Task 1.2.1e: Tests for fingerprinting, streaks, and the privacy guarantee (~5 min)
- New file `session/tokens/context_health_test.go`.
- `TestToolCallFingerprint_IsStableAcrossWhitespaceAndCase` — the Given-When-Then above.
- `TestExtractContextHealthSignals_CountsConsecutiveIdenticalCalls` — the 3×`Bash("git status")` + `Read` case, asserting `MaxConsecutiveRepeats == 3`, `RepeatedToolName == "Bash"`.
- `TestExtractContextHealthSignals_DistinctArgsAreNotRepeats` — the three-different-`Read` case, asserting `MaxConsecutiveRepeats == 1`.
- `TestParseReader_DoesNotRetainToolInput` — parse a JSONL fixture containing `SECRET_TOKEN=abc123` in a `tool_use` input; assert `fmt.Sprintf("%+v", result)` contains neither substring.
- Files: `session/tokens/context_health_test.go`

#### Story 1.2.2: Confusion-phrase detection over assistant text

**As** the ContextHealth analyzer, **I want** assistant text blocks scanned for apology/self-correction phrasing, **so that** repeated confusion is countable without storing the text.

**Acceptance Criteria**:
- `confusionPatterns` is compiled once at package init and matches case-insensitively.
  - *Given* an assistant text block `"I apologize — that didn't work. Let me try another approach."`, *When* `matchConfusionPatterns` runs, *Then* it returns the pattern name `"apology"` (the highest-priority match), and the raw text is not returned.
- Ordinary assistant prose does not match.
  - *Given* an assistant text block `"Sorted the imports and re-ran the tests; all 42 pass."`, *When* `matchConfusionPatterns` runs, *Then* it returns `""`.
- Counts accumulate across the window, and only the pattern *name* is retained.
  - *Given* a 20-turn window where 6 assistant turns contain a confusion phrase, *When* signals are extracted, *Then* `Signals.ConfusionPhraseCount == 6` and `Signals.LastConfusionPhrase` is a pattern name (e.g. `"apology"`), never the matched sentence.
- A fast-path guard skips the regexp engine on text with no candidate marker.
  - *Given* a 4 KB assistant text block containing neither `"sorry"`, `"apolog"`, `"mistake"`, nor `"didn't work"` (case-insensitively), *When* `matchConfusionPatterns` runs, *Then* it returns `""` without executing any regexp (mirroring `detectCommandsInText`'s `strings.ContainsRune(text, '/')` guard, `session/tokens/skill_detector.go:40-42`).

**Files**: `session/tokens/context_health.go`, `session/tokens/context_health_test.go`

##### Task 1.2.2a: Define `confusionPatterns` and `matchConfusionPatterns` (~5 min)
- In `session/tokens/context_health.go`, define `type confusionPattern struct { Name string; Re *regexp.Regexp }` and a package-level `var confusionPatterns = []confusionPattern{...}` compiled with `regexp.MustCompile`, in priority order:
  - `{"apology", (?i)\bi (?:apologize|apologise)\b|\bmy apologies\b|\bi'?m sorry\b}`
  - `{"self-correction", (?i)\bi (?:made|was) (?:a mistake|wrong|mistaken)\b|\bthat (?:was|is) (?:my|a) (?:mistake|error)\b}`
  - `{"retry", (?i)\bthat didn'?t work\b|\blet me try (?:a |an )?(?:different|another) approach\b|\blet me try again\b}`
- Add `var confusionFastPathMarkers = []string{"sorry", "apolog", "mistake", "didn't work", "didnt work", "try again", "another approach", "different approach"}` and `func matchConfusionPatterns(text string) string` that lowercases once, returns `""` if no marker is present, then runs the patterns in order and returns the first `Name` that matches.
- Files: `session/tokens/context_health.go`

##### Task 1.2.2b: Tests for confusion matching (~4 min)
- In `session/tokens/context_health_test.go`, add `TestMatchConfusionPatterns_MatchesApologyPhrasing`, `TestMatchConfusionPatterns_IgnoresOrdinaryProse`, and `TestMatchConfusionPatterns_FastPathSkipsLongNonMatchingText` (assert `""` for a 4 KB marker-free string).
- Add `TestExtractContextHealthSignals_CountsConfusionAcrossWindow` for the 6-of-20-turns case, asserting `ConfusionPhraseCount == 6` and that `LastConfusionPhrase` is one of the three pattern names.
- Files: `session/tokens/context_health_test.go`

#### Story 1.2.3: Wire signal extraction into the existing parser walk

**As** the `TokenStore`, **I want** `ContextHealthSignals` populated as a by-product of the parse I already do, **so that** no second pass over any JSONL file is needed.

**Acceptance Criteria**:
- `ParseReader` populates `result.ContextHealth` with no extra file read.
  - *Given* a JSONL transcript whose last three assistant turns each contain a single `tool_use` block `Bash({"command":"npm test"})`, *When* `Parser.ParseReader` is called once, *Then* `result.ContextHealth.MaxConsecutiveRepeats == 3` and `result.ContextHealth.ToolCallsInWindow == 3`.
- Only the trailing `healthWindowTurns` turns contribute.
  - *Given* a transcript with 4 consecutive identical `Bash` calls followed by 25 assistant turns each calling a distinct `Read`, *When* `ParseReader` runs, *Then* `result.ContextHealth.MaxConsecutiveRepeats == 1` — the early loop has aged out of the 20-turn window.
- Synthetic turns are excluded, matching the existing `TurnTimeline` rule.
  - *Given* an assistant entry with `"model": "<synthetic>"` between two identical `Bash` calls, *When* `ParseReader` runs, *Then* the two `Bash` calls are still counted as consecutive (`MaxConsecutiveRepeats == 2`) because the synthetic turn is skipped, exactly as `processAssistantEntry` already skips it for `TurnTimeline` (`session/tokens/parser.go:188-190`).

**Files**: `session/tokens/parser.go`, `session/tokens/context_health_test.go`

##### Task 1.2.3a: Thread a `healthTurnRing` through `ParseReader`/`processAssistantEntry` (~5 min)
- In `session/tokens/parser.go`, declare `ring := &healthTurnRing{}` in `ParseReader` alongside `modelCounts` (line 79) and pass it to `processAssistantEntry`.
- In `processAssistantEntry`, build a `healthTurnRecord` while iterating `msg.Content` (the loop at lines 164-181): in the existing `tool_use` branch also append `toolCallFingerprint(c.Name, c.Input)` and `c.Name`; add a new `else if c.Type == "text" && c.Text != ""` branch appending `matchConfusionPatterns(c.Text)` to `ConfusionHits` when non-empty.
- Set `record.At = turn.Timestamp` and `ring.push(record)` at the same point the function already guards `if msg.Model != syntheticModelSentinel` (line 188), so synthetic turns are excluded consistently.
- After the scan loop in `ParseReader`, set `result.ContextHealth = extractContextHealthSignals(ring)` next to the existing `result.PrimaryModel = ...` assignment (line 114).
- Files: `session/tokens/parser.go`

##### Task 1.2.3b: Parser-level integration tests (~5 min)
- In `session/tokens/context_health_test.go`, add `TestParseReader_PopulatesContextHealthSignals` (three identical `Bash` turns), `TestParseReader_OnlyTrailingWindowContributes` (4 identical + 25 distinct), and `TestParseReader_SkipsSyntheticTurnsInHealthWindow`, each constructed with `strings.NewReader` over a literal JSONL string as `session/tokens/parser_test.go` already does.
- Files: `session/tokens/context_health_test.go`

---

### Epic 1.3: Threshold evaluation

**Goal**: One pure function turns raw signals + config into a `ContextHealthVerdict`, with an explicit insufficient-data floor.

#### Story 1.3.1: `EvaluateContextHealth`

**As** the frontend, **I want** a single level and a human-readable reason, **so that** the badge can render without re-deriving thresholds client-side.

**Acceptance Criteria**:
- Below `MinToolCallSamples`, the level is `HealthUnknown` regardless of other signals.
  - *Given* `ContextHealthSignals{ToolCallsInWindow: 3, MaxConsecutiveRepeats: 3, ConfusionPhraseCount: 9}` and default config, *When* `EvaluateContextHealth` runs, *Then* it returns `Level == HealthUnknown` and `Reason == ""`.
- One heuristic at threshold yields amber; both at threshold, or either at 2× threshold, yields red.
  - *Given* `ContextHealthSignals{ToolCallsInWindow: 12, MaxConsecutiveRepeats: 3, RepeatedToolName: "Bash", ConfusionPhraseCount: 0}` and default config, *When* `EvaluateContextHealth` runs, *Then* `Level == HealthAmber` and `Reason == "Repeated the same Bash call 3 times in a row"`.
  - *Given* `ContextHealthSignals{ToolCallsInWindow: 12, MaxConsecutiveRepeats: 6, RepeatedToolName: "Bash", ConfusionPhraseCount: 0}` and default config, *When* `EvaluateContextHealth` runs, *Then* `Level == HealthRed`.
  - *Given* `ContextHealthSignals{ToolCallsInWindow: 12, MaxConsecutiveRepeats: 3, RepeatedToolName: "Edit", ConfusionPhraseCount: 5}` and default config, *When* `EvaluateContextHealth` runs, *Then* `Level == HealthRed` and `Reason` names both signals: `"Repeated the same Edit call 3 times in a row; 5 self-correction messages in the last 20 turns"`.
- Sufficient samples with no signal yields green with an empty reason.
  - *Given* `ContextHealthSignals{ToolCallsInWindow: 12, MaxConsecutiveRepeats: 1, ConfusionPhraseCount: 0}` and default config, *When* `EvaluateContextHealth` runs, *Then* `Level == HealthGreen` and `Reason == ""`.
- The function is pure and never reads a package-level or shared cache.
  - *Given* the same `ContextHealthSignals` and config passed twice from two goroutines, *When* `EvaluateContextHealth` runs concurrently, *Then* both calls return identical verdicts and `go test -race` reports no data race (no double-checked-locking slot is involved — the function holds no state; `.claude/rules/go-double-checked-locking.md`).

**Files**: `session/tokens/context_health.go`, `session/tokens/context_health_test.go`

##### Task 1.3.1a: Implement `EvaluateContextHealth` (~5 min)
- In `session/tokens/context_health.go`, add `func EvaluateContextHealth(sig ContextHealthSignals, cfg config.ContextHealthConfig) ContextHealthVerdict`, importing `github.com/tstapler/stapler-squad/config` (no import cycle: `config` imports only `log`, `executor`, `executor/safeexec` — verified).
- Call `cfg = cfg.ContextHealthConfigOrDefault()` first so a zero-value config passed by a test or an un-defaulted caller still behaves.
- Return `ContextHealthVerdict{Level: HealthUnknown, Signals: sig}` when `sig.ToolCallsInWindow < cfg.MinToolCallSamples`.
- Build `reasons []string`: append the loop reason when `sig.MaxConsecutiveRepeats >= cfg.LoopRepeatThreshold`; append the confusion reason when `sig.ConfusionPhraseCount >= cfg.ConfusionPhraseThreshold`. Compute `level`: `HealthGreen` if no reasons; `HealthRed` if both fired **or** either signal is `>= 2×` its threshold; else `HealthAmber`. Join reasons with `"; "`.
- Files: `session/tokens/context_health.go`

##### Task 1.3.1b: Table-driven tests for every threshold boundary (~5 min)
- In `session/tokens/context_health_test.go`, add `TestEvaluateContextHealth` as a table test with one row per Given-When-Then above plus boundary rows at `MaxConsecutiveRepeats == cfg.LoopRepeatThreshold-1` (green) and `ConfusionPhraseCount == cfg.ConfusionPhraseThreshold-1` (green).
- Add `TestEvaluateContextHealth_ZeroConfigUsesDefaults` passing `config.ContextHealthConfig{}` and asserting it behaves identically to the defaulted config.
- Files: `session/tokens/context_health_test.go`

---

## Phase 2: Expose over the existing ConnectRPC session surface

### Epic 2.1: Proto surface

**Goal**: Add the enum and the two `Session` fields at the next free field numbers, mirroring `detected_status`/`detected_context`.

#### Story 2.1.1: `ContextHealth` enum + `Session` fields 72/73

**As** a frontend client, **I want** `context_health` and `context_health_reason` on the `Session` message, **so that** I receive health updates on the `WatchSessions` stream I already consume.

**Acceptance Criteria**:
- The enum has exactly four values with `UNSPECIFIED = 0`.
  - *Given* `proto/session/v1/types.proto` after this change, *When* `make proto-gen` runs, *Then* `gen/proto/go/session/v1/types.pb.go` declares `ContextHealth_CONTEXT_HEALTH_UNSPECIFIED`, `_GREEN`, `_AMBER`, `_RED` and `web-app/src/gen/session/v1/types_pb.ts` exports a matching `ContextHealth` enum.
- The new fields occupy the next free numbers on `Session` and collide with nothing.
  - *Given* the `Session` message, whose highest field number in use is `71` (`workspace_key`) with no `reserved` ranges (verified), *When* the fields are added as `ContextHealth context_health = 72;` and `string context_health_reason = 73;`, *Then* `make proto-gen` completes without a field-number conflict and `make build` succeeds.

**Files**: `proto/session/v1/types.proto`

##### Task 2.1.1a: Add the enum and the two fields, then regenerate (~4 min)
- In `proto/session/v1/types.proto`, add `enum ContextHealth { CONTEXT_HEALTH_UNSPECIFIED = 0; CONTEXT_HEALTH_GREEN = 1; CONTEXT_HEALTH_AMBER = 2; CONTEXT_HEALTH_RED = 3; }` directly below the existing `enum DetectedStatus` block.
- In `message Session`, after `string workspace_key = 71;`, add `ContextHealth context_health = 72;` and `string context_health_reason = 73;` with doc comments mirroring `detected_status`/`detected_context` (lines 222-230), including "Empty when context_health is UNSPECIFIED" and "Only populated for Claude Code sessions — see ADR-001."
- Run `make proto-gen`.
- Files: `proto/session/v1/types.proto` (plus generated `gen/proto/go/session/v1/types.pb.go`, `web-app/src/gen/session/v1/types_pb.ts`)

---

### Epic 2.2: Delivery pipeline

**Goal**: Move the verdict from `TokenStore` onto the `Session` proto without changing `InstanceToProto`'s signature, following the `Artifacts` precedent.

#### Story 2.2.1: Authoritative Go→proto mapping

**As** a maintainer, **I want** exactly one place that maps `ContextHealthLevel` to the proto enum, **so that** the mapping can't drift across adapters.

**Acceptance Criteria**:
- The mapping is exhaustive and defaults safely.
  - *Given* `HealthAmber`, *When* `ContextHealthLevelToProto` is called, *Then* it returns `sessionv1.ContextHealth_CONTEXT_HEALTH_AMBER`; *and given* `HealthUnknown` or any out-of-range value, *Then* it returns `CONTEXT_HEALTH_UNSPECIFIED`.

**Files**: `session/tokens/proto_mapping.go` (new), `session/tokens/proto_mapping_test.go` (new)

##### Task 2.2.1a: Create `session/tokens/proto_mapping.go` (~3 min)
- New file `session/tokens/proto_mapping.go` mirroring `session/detection/proto_mapping.go:1-35` exactly in structure: package doc line declaring it "the single authoritative mapping; do not duplicate this logic in adapters or converters", a `switch` over all four `ContextHealthLevel` values, `default:` returning `CONTEXT_HEALTH_UNSPECIFIED`.
- Add `session/tokens/proto_mapping_test.go` with a table test covering all four values plus `ContextHealthLevel(99)`.
- Files: `session/tokens/proto_mapping.go`, `session/tokens/proto_mapping_test.go`

#### Story 2.2.2: Carry the verdict on `Instance` and into the proto

**As** the session card, **I want** `context_health` populated on every `Session` proto, **so that** the badge updates over the `WatchSessions` stream with no new RPC.

**Acceptance Criteria**:
- `Instance` stores the verdict behind the same actor-serialised setter used for `Artifacts`.
  - *Given* an `Instance`, *When* `inst.SetContextHealth(ContextHealthVerdict{Level: HealthAmber, Reason: "Repeated the same Bash call 3 times in a row"})` is called, *Then* `inst.Snapshot().ContextHealth.Level == HealthAmber` and `go test -race ./session/...` reports no race.
- `InstanceToProto` populates both fields, and populates neither when the level is unknown.
  - *Given* an `Instance` whose `ContextHealth` verdict is `{Level: HealthRed, Reason: "…"}`, *When* `InstanceToProto(inst, nil)` runs, *Then* `proto.ContextHealth == CONTEXT_HEALTH_RED` and `proto.ContextHealthReason` equals that reason string.
  - *Given* an `Instance` that has never had `SetContextHealth` called (e.g. an Aider session with no Claude JSONL transcript — ADR-001), *When* `InstanceToProto(inst, nil)` runs, *Then* `proto.ContextHealth == CONTEXT_HEALTH_UNSPECIFIED` and `proto.ContextHealthReason == ""`.

**Files**: `session/instance.go`, `session/instance_snapshot.go`, `server/adapters/instance_adapter.go`, `server/adapters/instance_adapter_test.go`

##### Task 2.2.2a: Add the `ContextHealth` field + `SetContextHealth` to `Instance` (~4 min)
- In `session/instance.go`, add `ContextHealth tokens.ContextHealthVerdict` next to the existing `Artifacts *artifacts.SessionArtifactsBlob` field (line 414), with the same "Populated asynchronously … Protected by mu" doc-comment style.
- Add `func (i *Instance) SetContextHealth(v tokens.ContextHealthVerdict)` immediately after `SetArtifacts` (lines 743-749), using the identical `i.sendSyncErr(func(s *instanceState) error { s.inst.ContextHealth = v; return nil })` body.
- Files: `session/instance.go`

##### Task 2.2.2b: Add `ContextHealth` to `InstanceSnapshot` and `buildSnapshot` (~2 min)
- In `session/instance_snapshot.go`, add `ContextHealth tokens.ContextHealthVerdict` to `InstanceSnapshot` next to `Artifacts` (line 137); it is a flat value struct, so no deep copy is required.
- Add `ContextHealth: i.ContextHealth,` to `buildSnapshot` next to the existing `Artifacts: i.Artifacts,` line (line 209).
- Files: `session/instance_snapshot.go`

##### Task 2.2.2c: Populate the proto fields in `InstanceToProto` (~3 min)
- In `server/adapters/instance_adapter.go`, immediately after the existing `DetectedStatus`/`DetectedContext` block (lines 169-173), add: `if snap.ContextHealth.Level != tokens.HealthUnknown { protoSession.ContextHealth = tokens.ContextHealthLevelToProto(snap.ContextHealth.Level); protoSession.ContextHealthReason = snap.ContextHealth.Reason }`, with a comment naming the field numbers ("fields 72–73") to match the existing convention.
- Add the `session/tokens` import.
- Files: `server/adapters/instance_adapter.go`

##### Task 2.2.2d: Adapter tests for populated and unpopulated cases (~4 min)
- In `server/adapters/instance_adapter_test.go`, add `TestInstanceToProto_PopulatesContextHealthFields` and `TestInstanceToProto_LeavesContextHealthUnspecifiedWhenNeverComputed`, following the existing `InstanceToProto(inst, nil)` test style (lines 47-99).
- Files: `server/adapters/instance_adapter_test.go`

#### Story 2.2.3: `publishContextHealth` wiring

**As** a user watching the board, **I want** the badge to update within one `TokenStore` notification cycle of the transcript changing, **so that** I see degradation without refreshing.

**Acceptance Criteria**:
- A transcript change propagates to a `session_updated` event.
  - *Given* a running server with a session whose `ConversationUUID` is `abc-123`, *When* `TokenStore` re-parses `abc-123.jsonl` and its verdict changes from `HealthGreen` to `HealthAmber`, *Then* `publishContextHealth` calls `inst.SetContextHealth` and publishes `events.NewSessionUpdatedEvent(inst, []string{"context_health"})`, which `WatchSessions` already converts via `InstanceToProto`.
- Only genuine level transitions are logged and published — an unchanged level is silent, **even if the reason string's embedded counts changed**.
  - *Given* a session already at `HealthAmber` with `MaxConsecutiveRepeats == 3`, *When* the transcript is re-parsed and the verdict is still `{HealthAmber, same reason}`, *Then* no log line is emitted and no event is published (condition-change gating, `research/pitfalls.md` §5).
  - *Given* a session already at `HealthAmber` with `Reason == "Repeated the same Bash call 3 times in a row"`, *When* the transcript is re-parsed and the verdict becomes `{HealthAmber, "Repeated the same Bash call 4 times in a row"}` (same `Level`, different `Reason`), *Then* `inst.SetContextHealth` is still called (so the tooltip reflects the new count) but no log line is emitted and no event is published — disambiguating the level-only trigger rule from a verdict-including-reason rule (this exact case was flagged as untested by adversarial review).
- A session with no matching transcript is left untouched.
  - *Given* an Aider session whose `GetClaudeConversationUUID()` returns `""`, *When* `publishContextHealth` runs, *Then* it skips that instance entirely — no `SetContextHealth` call, no event.

**Files**: `server/dependencies.go`

##### Task 2.2.3a: Add the `publishContextHealth` subscriber goroutine (~5 min)
- In `server/dependencies.go`, inside the existing `if homeDir, homeDirErr := os.UserHomeDir(); homeDirErr == nil` block, after `historyLinker.RegisterFileCallback(tokenStore.OnHistoryFileChanged)` (line 1094) and before `tokenStore.Start(...)`, start a goroutine that ranges over `tokenStore.Subscribe()`.
- **Trigger rule (resolves an adversarial-review BLOCKER — level-only, not verdict-including-reason):** gate strictly on `Level` change. Compare only `verdict.Level != inst.Snapshot().ContextHealth.Level`; a `Reason` string change with the same `Level` (e.g. repeat count ticking 3→4 while still `HealthAmber`) is NOT a trigger. This matches the Observability Plan's "only when the level actually changes" and Story 2.2.3's title — `Reason` is still stored via `SetContextHealth` (so the badge tooltip shows the latest count) but does not by itself cause a log line or event.
- On each notification: snapshot `historyLinker.Instances()`; for each instance, read `uuid := inst.GetClaudeConversationUUID()`, skip if empty; `pr := tokenStore.GetByUUID(uuid)`, skip if nil; compute `verdict := tokens.EvaluateContextHealth(pr.ContextHealth, cfg.ContextHealth)`; `prevLevel := inst.Snapshot().ContextHealth.Level`; always call `inst.SetContextHealth(verdict)` (so `Reason` stays current); if `verdict.Level == prevLevel`, continue without logging/publishing.
- On a `Level` change: `log.Info("[ContextHealth] level transition", …)` with the fields listed in the Observability Plan, then `eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"context_health"}))` — the exact call shape used for `"artifacts"` at line 1120.
- **Panic guard (resolves a second adversarial-review BLOCKER):** wrap the per-notification body in `defer func() { if r := recover(); r != nil { log.ErrorLog.Printf("[ContextHealth] recovered panic in publishContextHealth: %v", r) } }()`, matching the established convention for every other process-lifetime background goroutine in this file (see `server/dependencies.go:594-599`, `:914`) — without it, one panic silently and permanently stops all future ContextHealth updates for the process's life.
- Files: `server/dependencies.go`

##### Task 2.2.3b: Verify the backend end to end (~4 min)
- Run `make build && make test` and `go test -race ./session/tokens/... ./server/adapters/... ./config/...`.
- Start a second instance per `CLAUDE.md` (`go build -o /tmp/ssq-manual-test . && PORT=8999 STAPLER_SQUAD_INSTANCE=claude-manual-test /tmp/ssq-manual-test --tmux-keep-server &`) — **not** `make install-service` — and confirm a `[ContextHealth] level transition` line appears in its log for at least one live session. Kill it when done.
- Files: none (verification only)

---

## Phase 3: Session-card badge

### Epic 3.1: `ContextHealthBadge` component

**Goal**: A distinct, accessible, token-styled badge that is silent at green and absent when unknown.

#### Story 3.1.1: The badge component

**As** a user scanning a board of a dozen sessions, **I want** a visually distinct health badge, **so that** I don't confuse a cumulative degradation signal with a transient `Error` chip.

**Acceptance Criteria**:
- Each level maps to a distinct icon *shape* plus a text label — never colour alone (WCAG 2.1 SC 1.4.1, `research/ux.md` §3).
  - *Given* `ContextHealth.AMBER`, *When* `getContextHealthInfo` is called, *Then* it returns `{label: "Context Degrading", icon: "⚠", variant: "amber"}`; *given* `RED`, *Then* `{label: "Context Needs Attention", icon: "✖", variant: "red"}` — a different glyph, not a recoloured `⚠`.
- The component renders nothing for `UNSPECIFIED` and for `GREEN`.
  - *Given* `<ContextHealthBadge health={ContextHealth.UNSPECIFIED} reason="" />`, *When* it renders, *Then* the output is `null`; *and given* `health={ContextHealth.GREEN}`, *Then* the output is also `null`.
- Unrecognised wire values degrade to `null` with a console warning, not a throw.
  - *Given* `health={7 as ContextHealth}` (a value from a newer server), *When* the component renders, *Then* it returns `null` and calls `console.warn` — matching `SubStatusChip.tsx:157-167`'s forward-compatibility guard rather than `assertNever`'s throw.
- The tooltip and `aria-label` both contain the words "Context health".
  - *Given* `health={ContextHealth.AMBER}` and `reason="Repeated the same Bash call 3 times in a row"`, *When* it renders, *Then* the element has `role="status"` and an accessible name of `"Context health: Context Degrading — Repeated the same Bash call 3 times in a row"`.
- A paused session's tooltip says so.
  - *Given* `health={ContextHealth.RED}`, `reason="…"`, and `isPaused={true}`, *When* it renders, *Then* the accessible name ends with `" (paused)"` (`research/ux.md` §4).

**Files**: `web-app/src/components/sessions/ContextHealthBadge.tsx` (new), `web-app/src/components/sessions/ContextHealthBadge.css.ts` (new), `web-app/src/components/sessions/ContextHealthBadge.test.tsx` (new)

##### Task 3.1.1a: Create `ContextHealthBadge.css.ts` (~4 min)
- New file, vanilla-extract only (no `.module.css` — `.claude/rules/css-architecture.md`).
- Export a base `healthBadge` style (`inline-flex`, `gap: 4px`, `borderRadius: "999px"` — a pill, deliberately more circular than `StatusBadge.css.ts`'s `12px` so the shape differs at a glance) and `healthVariants = styleVariants({ amber: {...}, red: {...} })`.
- Reference only existing tokens: amber → `vars.color.warningBg` / `vars.color.warningText`; red → `vars.color.errorBg` / `vars.color.errorText` / `vars.color.error` — the same tokens `SubStatusChip.css.ts:41-60` already uses. No new entry in `theme-contract.css.ts` and no hardcoded hex.
- Files: `web-app/src/components/sessions/ContextHealthBadge.css.ts`

##### Task 3.1.1b: Create `ContextHealthBadge.tsx` (~5 min)
- Export `getContextHealthInfo(health: ContextHealth): {label, icon, variant} | null` — a `switch` returning `null` for `UNSPECIFIED` and `GREEN`, `{"Context Healthy"}` never rendered, `⚠`/`amber` for `AMBER`, `✖`/`red` for `RED`, and a `default:` that assigns to `const _exhaustive: never`, `console.warn`s, and returns `null`.
- Export `ContextHealthBadge({ health, reason, isPaused }: { health?: ContextHealth; reason?: string; isPaused?: boolean })`. Return `null` when `health` is `undefined`/`null` or `getContextHealthInfo` returns `null`.
- Build `const description = \`Context health: ${info.label}${reason ? \` — ${reason}\` : ""}${isPaused ? " (paused)" : ""}\`` and render `<Tooltip label={description} side="top">` (import from `../ui/Tooltip`, the exact path `SessionCard.tsx:5` already uses) wrapping a `<span role="status" aria-label={description} title={description} data-testid="badge-context-health">` containing the glyph (`aria-hidden`) and the label. The `data-testid` is required so the e2e spec can locate it without a CSS selector (`.claude/rules/e2e-test-conventions.md` §3).
- Files: `web-app/src/components/sessions/ContextHealthBadge.tsx`

##### Task 3.1.1c: Jest/RTL tests (~5 min)
- New file `ContextHealthBadge.test.tsx` following the existing colocated `*.test.tsx` convention (e.g. `ApprovalRulesPanel.test.tsx`).
- Cases: renders `null` for `UNSPECIFIED`; renders `null` for `GREEN`; renders `⚠ Context Degrading` for `AMBER` with the exact accessible name above; renders `✖` (not `⚠`) for `RED`; appends `" (paused)"` when `isPaused`; returns `null` and warns for an unknown numeric value.
- Verify with `cd web-app && npx jest --no-coverage --testPathPatterns="ContextHealthBadge"`.
- Files: `web-app/src/components/sessions/ContextHealthBadge.test.tsx`

---

### Epic 3.2: Session-card integration

#### Story 3.2.1: Render the badge in a fixed position on the card

**As** a user, **I want** the health badge always in the same slot, **so that** I learn its position once and can scan for it across cards.

**Acceptance Criteria**:
- The badge is the last element in the badge row, after every existing badge.
  - *Given* a session with `subStatus === PROCESSING`, `memoryRssMb === 400n`, and `contextHealth === AMBER`, *When* `SessionCard` renders, *Then* the DOM order within the badges row is `SubStatusChip` → memory badge → … → `ContextHealthBadge`, with the health badge last.
- The badge composes with, and never replaces, the existing status indicators.
  - *Given* a session with `subStatus === ERROR` and `contextHealth === RED`, *When* the card renders, *Then* both the `✖ Error` chip and the `✖ Context Needs Attention` health badge are present, each with its own `role="status"` and distinct accessible name.
- Paused sessions pass `isPaused` through.
  - *Given* a session with `status === SessionStatus.PAUSED` and `contextHealth === RED`, *When* the card renders, *Then* the health badge's accessible name ends with `" (paused)"`.

**Files**: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 3.2.1a: Render `ContextHealthBadge` in `SessionCard` (~3 min)
- Import `ContextHealthBadge` in `SessionCard.tsx`.
- Add `<ContextHealthBadge health={session.contextHealth} reason={session.contextHealthReason} isPaused={session.status === SessionStatus.PAUSED} />` as the **final** element of the badges row — immediately after the `{pendingProgramChange && (…)}` block (ends ~line 638) and immediately before that row's closing `</div>`. No conditional wrapper: the component's own `null` returns are the suppression mechanism.
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 3.2.1b: Verify the frontend build and lint (~3 min)
- Run `cd web-app && npx jest --no-coverage --testPathPatterns="ContextHealthBadge|SessionCard"` and the repo's `make lint`.
- Files: none (verification only)

---

### Epic 3.3: Registry and e2e coverage

#### Story 3.3.1: Register the feature and add an e2e spec

**As** a maintainer, **I want** the feature registered and e2e-covered, **so that** the PR satisfies `.claude/rules/feature-registry.md` and `.claude/rules/e2e-test-conventions.md`.

**Acceptance Criteria**:
- Per-feature registry files exist for both halves and the aggregate regenerates cleanly.
  - *Given* `docs/registry/features/frontend/context-health-badge.json` with `{"id": "context-health-badge", "type": "frontend", "name": "Context Health Badge", "filePath": "web-app/src/components/sessions/ContextHealthBadge.tsx", "tested": true, "testIds": ["ContextHealthBadge"]}`, *When* `make registry-generate` runs, *Then* it completes and the count in `docs/registry/coverage-gaps.json` does not increase.
- The e2e spec follows all four hard conventions.
  - *Given* `tests/e2e/context-health-badge.spec.ts`, *When* CI's convention check runs, *Then* it passes: the file starts with `// @feature session:list, context-health-badge`, uses `getByTestId`/`getByRole` only, contains no `waitForTimeout`, and any reusable navigation lives in `tests/e2e/pages/`.

**Files**: `docs/registry/features/frontend/context-health-badge.json` (new), `docs/registry/features/backend/context-health.json` (new), `tests/e2e/context-health-badge.spec.ts` (new)

##### Task 3.3.1a: Add the two registry entries and regenerate (~3 min)
- Create the two per-feature JSON files above, matching `docs/registry/schema.json`. The backend entry covers the `context_health` field on the existing session surface (no new RPC), with `testIds` listing the Go test names from Epics 1.2/1.3/2.2.
- Run `make registry-generate` and commit whatever it changes.
- Files: `docs/registry/features/frontend/context-health-badge.json`, `docs/registry/features/backend/context-health.json`

##### Task 3.3.1b: Add the e2e spec (~5 min)
- New `tests/e2e/context-health-badge.spec.ts` with the required `// @feature` header, a `test.describe("context-health-badge", …)` block, and one test asserting `await expect(page.getByTestId("badge-context-health")).toHaveCount(0)` for a freshly-created session — the suppression contract from Story 3.1.1, and the only assertion that can be made deterministically against the isolated test server without synthesising a looping agent. Amber/red rendering is covered by the Jest tests in Task 3.1.1c, which can construct the proto value directly.
- Do not start a server manually; `tests/e2e/global-setup.ts` handles it. Run `cd tests/e2e && npx playwright test context-health-badge.spec.ts`.
- Files: `tests/e2e/context-health-badge.spec.ts`
