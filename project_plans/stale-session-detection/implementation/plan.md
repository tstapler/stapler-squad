# Implementation Plan: stale-session-detection

**Feature**: Config-driven staleness detection for the main session list — a visual
badge, a "Stale" grouping strategy, an optional notification, and a
`min_session_idle_minutes` auto-approval-rule condition — built entirely on the
already-tracked `ReviewState.LastMeaningfulOutput`/`LastTerminalUpdate` signal.
**Date**: 2026-08-06
**Status**: Ready for implementation
**ADRs**: [ADR-001: Fourth Staleness Threshold + Approval-Rule Idle Semantics](../decisions/ADR-001-stale-session-fourth-threshold-and-idle-semantics.md)

---

## Step 0.5 — Creative Pass: Alternatives Considered for Overall Architecture

1. **Unify all four staleness thresholds (3 existing + this project's) into one shared
   config value**, read by every consumer with per-consumer multipliers.
   - *Strength*: one source of truth; trivially easy to reason about "what counts as
     stale."
   - *Weakness*: `review-gate-stale-session-rework`'s ADR-001 already proved, with live
     incident evidence (37/41 false-positive badges), that these consumers need
     independently-tunable values — a shared base+multiplier still couples unrelated
     tuning decisions (fixing the badge's noise level would unintentionally shift the
     rework-block gate's sensitivity).

2. **Centralize all staleness computation behind a new `StalenessService` actor**,
   refactoring the three existing detectors (Review Queue badge, rework-block gate,
   stuck-work detector) plus this project's two new consumers to all call through it.
   - *Strength*: eliminates every duplicate `time.Since(x) > threshold` in the codebase
     in one pass; cleanest architecture on paper.
   - *Weakness*: directly violates `requirements.md`'s Constraints — the three existing
     detectors are already-shipped, ADR-001-tuned territory this project is explicitly
     told not to re-derive or refactor (`review-gate-stale-session-rework`,
     `review-queue-state-detection`, `review-queue-event-driven` are sibling projects'
     scope). Blast radius far exceeds this feature's stated Complexity-2 budget.

3. **(Chosen) Incremental extension** — add exactly one new named threshold
   (`StaleSessionConfig.ThresholdMinutes`) and one new lightweight notifier
   (`StaleSessionNotifier`), extract *comparison* helpers (not threshold values) shared
   by the two new frontend consumers and reused by the one new Go consumer, and leave
   the three existing detectors completely untouched.
   - *Strength*: minimal, additive diff matching the stated Complexity-2 scope; respects
     `requirements.md`'s explicit "don't re-derive sibling projects' work" constraint;
     directly matches `build-vs-buy.md`'s recommendation ("extract the comparison, not
     the threshold *value*").
   - *Weakness*: leaves four independently-tuned thresholds in the codebase long-term,
     and the three pre-existing Go implementations of "time since last output" remain
     unconsolidated. This is a known, accepted tradeoff — documented in ADR-001 (this
     project) rather than silently left undocumented.

Approach 3 is used throughout this plan. Alternatives 1 and 2 are recorded in the
Pattern Decisions table below (see "Overall Architecture" row).

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `StaleSessionConfig` | New Go struct in `config/types.go` holding the two new config knobs for this feature. | Mirrors `SessionRetentionConfig`'s shape exactly. |
| `ThresholdMinutes` | `StaleSessionConfig` field — minutes of no meaningful output before the main-list card badge/grouping/notifier consider a session stale. `0` = unset, falls back to default. | JSON key `threshold_minutes`. |
| `NotifyEnabled` | `StaleSessionConfig` field (`*bool`) — whether crossing `ThresholdMinutes` also emits a notification-bus event. Nil = default `true`. | JSON key `notify_enabled,omitempty`. |
| `ThresholdMinutesOrDefault()` | Accessor on `StaleSessionConfig`; returns `ThresholdMinutes` or `defaultStaleSessionThresholdMinutes` (30) when `<= 0`. | Mirrors `RetentionDaysOrDefault()`. |
| `NotifyEnabledOrDefault()` | Accessor on `StaleSessionConfig`; returns `true` when `NotifyEnabled == nil`. | Mirrors `EnabledOrDefault()`. |
| `defaultStaleSessionThresholdMinutes` | Package-level `const` in `config/types.go`, value `30`. | Per ADR-001 (this project). |
| `StaleSessionNotifier` | New Go type in `server/services/`. Periodically scans live `ACTIVE` instances via `ReviewQueuePoller.GetInstances()` and emits a notification-bus event the first time a session crosses `ThresholdMinutes`. | Modeled on `SessionRetentionSweeper`. |
| `notifiedSessions` | `StaleSessionNotifier`'s in-memory `map[string]time.Time`, keyed by session ID — the edge-triggered "already notified this stale episode" dedup marker. Entry removed once the session recovers (idle time drops back under threshold). | Not persisted; lightweight per pitfalls.md §2's recommendation. |
| `GetTimeSinceLastMeaningfulOutput()` | Existing `*session.Instance` method (`session/instance_approval.go:114`) — reused verbatim by `StaleSessionNotifier` and the new approval-rule condition. Not modified by this project. | The one canonical Go "idle duration" call. |
| `MinSessionIdleMinutes` | New field on `ApprovalRuleProto` (field 30), `pkg/classifier.Rule`, `session/ent/schema/approvalrule.go`, `ApprovalRuleData`, and `RuleSpec` — the per-rule minutes threshold a rule author configures. `0` = condition not applied. | Renamed from the source issue's `session_age_minutes`; see ADR-001 Decision 2. |
| `SessionIdleMinutes` | New field on `pkg/classifier.ClassificationContext` — the *computed* idle-minutes value for the session being classified, populated by `ApprovalHandler` at classify time. Zero value (unset) means "unknown" and fails closed. | Distinct from `MinSessionIdleMinutes` (the rule's configured threshold) — one is the rule's knob, the other is the live measurement. |
| `isSessionStale(session, thresholdMinutes)` | New pure TS function in `web-app/src/lib/session-staleness.ts`. The single shared predicate used by both the `SessionCard` badge and the `Stale` grouping strategy. | Frontend counterpart to the Go comparison; no backend RPC needed. |
| `getLastActivityTimestamp(session)` | New pure TS function in the same file — extracted from `SessionCard.tsx`'s existing inline IIFE (max of `lastMeaningfulOutput`/`lastTerminalUpdate`). | Refactor-in-place; behavior-preserving. |
| `GroupingStrategy.Stale` | New enum member (`"stale"`) on `web-app/src/lib/grouping/strategies.ts`'s `GroupingStrategy` enum. | Requires a matching `GroupingStrategyLabels` entry (not compile-enforced — see UX research §3). |
| `staleThresholdMinutes` | Prop/variable name threaded `SessionList.tsx` → `SessionCard` and → `groupSessions()`, sourced from the new `useStaleSessionConfig` hook. | Consistent name across both consumers. |
| `useStaleSessionConfig` | New React hook in `web-app/src/lib/hooks/useStaleSessionConfig.ts` — fetches `SessionDefaultsConfig` once via `getSessionDefaults` and returns `{ thresholdMinutes, notifyEnabled }`. | Distinct from the existing `useSessionDefaults` hook (that one resolves per-directory launch defaults, not global feature config). |
| `staleSessionThresholdMinutes` (proto) | New field 12 on `SessionDefaultsConfig` (`proto/session/v1/session.proto`). Always carries the *resolved* value (never 0) in responses. | `0` in `UpdateGlobalDefaultsRequest` = "use server default." |
| `staleSessionNotifyEnabled` (proto) | New field 13 on `SessionDefaultsConfig`; new field 11 on `UpdateGlobalDefaultsRequest`. Plain resolved `bool`, matching `auto_yes`'s convention (no tri-state on the wire). | See Pattern Decisions row 7. |
| `minSessionIdleMinutes` (frontend) | camelCase field name in `RuleBuilderForm.tsx` / `useApprovalRules.ts` mirroring the Go/proto `MinSessionIdleMinutes`. | Rendered as a numeric input, sibling to the existing `requireCiPassing` checkbox. |
| `IsStale` (Go helper, optional) | Candidate shared Go helper `func IsStale(last time.Time, threshold time.Duration) bool` in `session/` — used only by the new `StaleSessionNotifier`, not retrofitted onto the three existing detectors. | Nice-to-have consolidation; see Task 4.1.1a for the scope decision. |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall architecture | Incremental extension: one new threshold + one new notifier, existing detectors untouched | build-vs-buy.md §4 | (1) Unify all 4 thresholds into one shared value; (2) Centralized `StalenessService` actor refactor | (1) recreates ADR-001's proven false-positive coupling risk; (2) exceeds Complexity-2 scope and touches sibling projects' already-shipped territory — see Step 0.5. |
| Config storage (`StaleSessionConfig`) | Transaction Script — plain struct + `OrDefault()` accessor (PoEAA) | `config/types.go`'s `SessionRetentionConfig` precedent | Domain Model — a `StaleSessionPolicy` object with behavior (e.g. an `IsExpired(now)` method carrying state) | Two scalar config knobs with a simple clamp-or-default rule don't warrant a behavior-carrying domain object; would be over-engineering for CRUD-shaped config. |
| Staleness predicate (Go + TS) | Shared pure function, not a Strategy interface (GoF) | build-vs-buy.md §4, architecture.md §1 | GoF Strategy — a `StalenessDetector` interface with swappable implementations | Only one algorithm (time comparison) exists or is anticipated; an interface with one implementation is exactly interface-pollution-checklist smell #1 (speculative interface). |
| Approval-rule condition composition | Flat AND-chain in `matchesRule`, extending the existing structure | `pkg/classifier/classifier.go:735` (`RequireCIPassing` precedent) | GoF Composite/Specification pattern for rule conditions | `matchesRule` already ANDs 5+ flat conditions with no OR/nesting requirement; introducing Specification here would mean refactoring every existing condition, well outside this project's scope. |
| Stale-notification dedup | In-memory `map[string]time.Time` inside `StaleSessionNotifier` (Transaction Script + simple state) | pitfalls.md §2's explicit recommendation | Repository — extend `BacklogStuckState`/`MarkStuck` to cover session-only rows | `BacklogStuckState` is keyed on `(item_id, reason)` and requires a backlog item; many main-list sessions (manual/one-off) have none. Extending its schema would conflate two intentionally-separate concerns (backlog-stuck vs. session-stale) and require a migration this Complexity-2 feature doesn't need. |
| SessionCard badge + grouping computation | Shared value-object-style pure helpers (`isSessionStale`, `getLastActivityTimestamp`) in one new module | architecture.md §1 (type-driven design: single predicate, not duplicated) | Inline computation duplicated separately in `SessionCard.tsx` and `strategies.ts` | Exactly the duplication `requirements.md`'s success metric #4 forbids; the Go side already has 3 duplicated implementations — don't add a frontend 4th/5th. |
| Approval-rule threshold field type | Primitive `int32` minutes, `0` = unset (type-driven, matches sibling fields) | `max_auto_rework_iterations`, `require_ci_passing` conventions | `google.protobuf.Duration` typed field | No existing field in `ApprovalRuleProto` uses `Duration`; would break the "0 = unset" idiom used by every sibling condition field in the same message. |
| `staleSessionNotifyEnabled` wire representation | Plain resolved `bool` (always the applied value) | `auto_yes` field convention | `google.protobuf.BoolValue` wrapper to carry Go's `*bool` tri-state onto the wire | No other field in `SessionDefaultsConfig`/`UpdateGlobalDefaultsRequest` does this (responses always echo the resolved value; requests always apply the explicit value) — a wrapper here would be the only inconsistent field for a distinction (on-disk config forward-compat) that doesn't matter over the wire. |
| Frontend re-render trigger for time-relative badge/grouping | Lightweight `setInterval`-driven tick state in `SessionList.tsx` (client poll) | Established pattern — 10 existing `setInterval` hooks in `web-app/src/lib/hooks/` | Server-push: subscribe to a backend "session became stale" event and only re-render on push | Over-engineered for a purely cosmetic client recomputation; would require the backend to run the threshold comparison a second time (once for its own notifier, once per-client-configured threshold for push), doubling logic for no perceptible UX gain at 30-60s granularity. |

---

## Migration Plan

Two schema-adjacent changes, both additive and backward-compatible — no data migration
script needed for either:

1. **`session/ent/schema/approvalrule.go`** — new field
   `field.Int32("min_session_idle_minutes").Optional().Default(0)`. Existing
   `ApprovalRule` rows on disk get the default `0` automatically on next read (ent's
   `Optional().Default()` backfills at the SQL layer; no `UPDATE` migration statement is
   required since `0` is exactly "condition not applied," which is the correct behavior
   for every pre-existing rule). Regenerate via
   `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema`
   (or `make ent-gen` / `make build`) per `.claude/rules/ent-schema-generation.md` —
   **never** omit `--feature sql/upsert`.
2. **`config.json` on disk** — `StaleSessionConfig` is a new, zero-value-safe struct
   field on `Config` (`config/config.go`). Existing config files simply decode it as the
   Go zero value (`ThresholdMinutes: 0, NotifyEnabled: nil`), and the `OrDefault()`
   accessors resolve that to `(30, true)` — identical forward-compat behavior to
   `SessionRetentionConfig`'s existing rollout. No `MigrateConfig` changes needed.

`make proto-gen` is required after the `ApprovalRuleProto`/`SessionDefaultsConfig`/
`UpdateGlobalDefaultsRequest` proto edits (content-hash-gated; just run it, per
`Makefile:398-413`).

## Observability Plan

- **Logs**: `StaleSessionNotifier.Start()` logs once at startup
  (`log.Info("stale session notifier started", "threshold_minutes", ..., "notify_enabled", ...)`),
  mirroring `SessionRetentionSweeper`'s/`HibernationSweeper`'s existing start-up log
  lines (`server/server.go:690`, `:699`). Each edge-triggered notification fire logs at
  `log.Info` with `session_id`/`session_title`/`idle_minutes` fields, mirroring
  `notifyIfActiveWorkSessionStale`'s logging shape.
- **Metrics**: no new metrics system is introduced (none exists for this class of signal
  today beyond structured logs). The approval-rule condition's match/no-match outcome is
  automatically captured by the existing `AnalyticsSummaryProto`/analytics store path
  every `Classify()` call already feeds — no new instrumentation needed there.
- **Alerts**: none — single-user, self-hosted, no alerting infrastructure
  (`CLAUDE.md`). The in-app notification (`NOTIFICATION_TYPE_WARNING`) *is* the alert
  mechanism for this feature.
- **Known, accepted clock skew**: the frontend's 60s re-render tick (Task 2.3.1b) and
  the backend's independent 60s `StaleSessionNotifier` tick (Task 4.1.1b) are two
  uncoordinated clocks re-evaluating the same threshold-crossing event — a push
  notification can arrive up to ~60s before or after the badge appears on an
  already-open tab. This is intentional (per architecture-review.md's Concern and
  pre-mortem.md Failure #4), not a bug: if reported, triage as expected behavior, not a
  new defect.

## Risk Control

- **Feature flag**: `StaleSessionConfig.NotifyEnabledOrDefault()` doubles as the
  notifier's own kill switch — setting `notify_enabled: false` in `config.json` (or via
  the Settings UI once Phase 1 ships) disables `StaleSessionNotifier` **within one 60s
  tick, no restart required**, because `checkAll()` calls `config.LoadConfig()` itself
  every tick (Task 4.1.1c) rather than holding a startup-captured `*config.Config`
  pointer — the pattern that would otherwise silently ignore the change for the life of
  the process (see `DefaultsService.sharedBacklogCfg`'s "PR #199 review F1" precedent,
  which this design avoids by construction rather than by a second manual propagation
  block). The badge and grouping strategy are pure, always-on frontend reads (no flag)
  — consistent with their "informational, not automation-affecting" risk tier (see UX
  research §5).
- **Rollback procedure**: every change in this plan is additive (new struct fields,
  omitempty JSON, `0`/unset-default proto fields, a new Go type, a new TS module). A
  revert of the PR(s) requires no data cleanup — no migration to roll back (see Migration
  Plan above).
- **Staged rollout** (single-developer instance, so "staged" means sequencing within this
  plan, not a canary): ship Phases 1–3 (config + badge + grouping — pure frontend reads,
  zero automation impact) first; ship Phase 4 (notifier) next, smoke-tested manually
  against a real idle session before merge; ship Phase 5 (approval-rule condition) last
  and only after manually verifying the fail-closed behavior against a live
  `FindLiveInstance` miss (pitfalls.md §5's "worse blast radius than a UI badge" warning)
  — this is the highest-risk phase since a miscalibrated rule can silently
  auto-approve/deny real actions.

## Unresolved Questions

None. *(Resolved: ADR-001 Decision 2 — Status: Accepted — settles the approval-rule
field name as `min_session_idle_minutes`, not the source issue's `session_age_minutes`,
on correctness grounds (age vs. idle semantic mismatch), and lists the rejected
alternative under "Alternatives Considered" rather than leaving it open. Story 5.1 is
unblocked; this plan's "Ready for implementation" status header is accurate. Phase 2
research fully resolved threshold value, config shape, and idle-vs-creation-time
semantics — see requirements.md's Open Questions section and this plan's ADR-001.)*

**Explicitly deferred, not unresolved** (a planning decision, not an open question):
distinguishing "stale + waiting on a prompt" from "stale + no visible reason" on the
badge (`features.md` §5/§8) is out of scope for this Complexity-2 plan — the underlying
data (`LastPromptDetected`) already exists and this is a natural Phase 2 follow-on, but
adding a second badge variant is not in `requirements.md`'s Scope section and would
expand this plan's blast radius without a stated requirement driving it.

---

## Dependency Visualization

```
Phase 1: Config Surface
  Epic 1.1 (Go config struct) ──┐
  Epic 1.2 (proto + RPC)  <─────┘ (needs 1.1's field names)
  Epic 1.3 (Settings UI)  <─────── (needs 1.2's generated TS types)
        │
        ├─────────────────────────────┬───────────────────────────┐
        ▼                             ▼                           ▼
Phase 2: Frontend Badge        Phase 3: Grouping Strategy   Phase 4: Notifier
  Epic 2.1 (shared helper)       Epic 3.1 (enum + switch)     Epic 4.1 (Go type)
  Epic 2.2 (badge UI)              (imports 2.1's helper)     Epic 4.2 (wiring)
  Epic 2.3 (re-render tick)                                     (independent of 2/3;
        │                                                        only needs Phase 1)
        └──────────────┬──────────────────────────────────────────┘
                        ▼
              Phase 5: Approval-Rule Condition (independent of 2/3/4;
                only needs proto field-30 availability, verified pre-existing)
                Epic 5.1 (proto + ent)
                Epic 5.2 (repo/store/service conversions)  <- needs 5.1's codegen
                Epic 5.3 (classifier match logic)          <- needs 5.1
                Epic 5.4 (approval_handler population)     <- needs 5.3
                Epic 5.5 (frontend rule builder UI)         <- needs 5.2's generated TS types
                Epic 5.6 (tests)                            <- needs 5.3, 5.4
                        │
                        ▼
              Phase 6: Feature Registry (needs all UI components to exist: 2.2, 3.1, 5.5)
```

Phases 2/3/4 can proceed in parallel once Phase 1 lands. Phase 5 is fully independent of
Phases 2–4 (different files, different proto message) and can start immediately after
verifying field 30 is still available. Phase 6 is last, gated on every new UI surface
existing.

---

## Phase 1: Config Surface

### Epic 1.1: `StaleSessionConfig` Go struct
**Goal**: A new, zero-value-safe config struct threaded onto `Config`, following the
`SessionRetentionConfig` precedent exactly.

#### Story 1.1.1: Add `StaleSessionConfig` with `OrDefault()` accessors
**As a** developer extending the badge/notifier/grouping features, **I want** a
config-driven threshold and notify flag with safe defaults, **so that** a misconfigured
`0`/unset value degrades to "use default," never to "everything is stale."

**Acceptance Criteria**:
- A zero-value `StaleSessionConfig{}` resolves to `ThresholdMinutesOrDefault() == 30` and
  `NotifyEnabledOrDefault() == true`.
  - *Given* a `StaleSessionConfig{}` (Go zero value, as decoded from a `config.json` file
    written before this feature existed), *When* `ThresholdMinutesOrDefault()` and
    `NotifyEnabledOrDefault()` are called, *Then* they return `30` and `true`
    respectively — never `0`/`false` by accident.
- A negative or zero explicit `ThresholdMinutes` also falls back to the default (not
  "always stale").
  - *Given* `StaleSessionConfig{ThresholdMinutes: -5}`, *When*
    `ThresholdMinutesOrDefault()` is called, *Then* it returns `30`.
**Files**: `config/types.go`

##### Task 1.1.1a: Add `StaleSessionConfig` struct + `defaultStaleSessionThresholdMinutes` const (~3 min)
- Add `const defaultStaleSessionThresholdMinutes = 30` near the existing
  `defaultSessionRetentionDays` const (`config/types.go:33`).
- Add `type StaleSessionConfig struct { ThresholdMinutes int \`json:"threshold_minutes,omitempty"\`; NotifyEnabled *bool \`json:"notify_enabled,omitempty"\` }`
  directly below `SessionRetentionConfig` (`config/types.go:38-46`).
- Files: `config/types.go`

##### Task 1.1.1b: Add `ThresholdMinutesOrDefault()` / `NotifyEnabledOrDefault()` accessors (~3 min)
- Mirror `RetentionDaysOrDefault()` (`config/types.go:58-63`) and `EnabledOrDefault()`
  (`config/types.go:49-54`) exactly, on `StaleSessionConfig`.
- Files: `config/types.go`

##### Task 1.1.1c: Wire `StaleSession StaleSessionConfig` field onto `Config` (~2 min)
- Add `StaleSession StaleSessionConfig \`json:"stale_session,omitempty"\`` next to
  `SessionRetention` (`config/config.go:343`). No entry needed in `DefaultConfig()`
  (matches `SessionRetentionConfig`'s pattern of relying solely on the `OrDefault()`
  accessors, not an explicit zero-value assignment).
- Files: `config/config.go`

##### Task 1.1.1d: Unit tests for the two accessors (~4 min)
- New `config/types_test.go` cases (or add to existing table if one already covers
  `SessionRetentionConfig`): zero value → defaults; explicit positive value → itself;
  negative value → default; `NotifyEnabled: ptr(false)` → `false`.
- Files: `config/types_test.go`

---

### Epic 1.2: Thread the threshold + notify flag to the frontend via `SessionDefaultsConfig`
**Goal**: No new RPC — extend the existing `GetSessionDefaults`/`UpdateGlobalDefaults`
pair, following `max_auto_rework_iterations`'s exact "0 = server default, response
always resolved" convention.

#### Story 1.2.1: Add `stale_session_threshold_minutes` + `stale_session_notify_enabled` to `SessionDefaultsConfig` and `UpdateGlobalDefaultsRequest`/`Response`
**As a** frontend developer building the badge/grouping/settings UI, **I want** the
resolved threshold and notify flag available on the existing session-defaults RPCs,
**so that** no new RPC surface is needed.

**Acceptance Criteria**:
- `GetSessionDefaults` response always includes a resolved (never-zero) threshold.
  - *Given* a `config.json` with no `stale_session` key at all, *When*
    `GetSessionDefaults` is called, *Then* `response.defaults.staleSessionThresholdMinutes == 30`
    and `response.defaults.staleSessionNotifyEnabled == true`.
- `UpdateGlobalDefaults` persists an explicit override and echoes it back resolved.
  - *Given* a client calls `UpdateGlobalDefaults({ staleSessionThresholdMinutes: 45, staleSessionNotifyEnabled: false, ... })`,
    *When* the RPC completes, *Then* `config.json`'s `stale_session.threshold_minutes == 45`
    and the response echoes `staleSessionThresholdMinutes: 45, staleSessionNotifyEnabled: false`.
  - *Given* a client calls `UpdateGlobalDefaults({ staleSessionThresholdMinutes: 0, ... })`
    (0 = "use server default" per the `max_auto_rework_iterations` convention), *When*
    the RPC completes, *Then* `config.json`'s `stale_session.threshold_minutes` is left
    unset (or explicitly `0`) and the response echoes the resolved default `30`.
**Files**: `proto/session/v1/session.proto`, `server/services/defaults_service.go`

##### Task 1.2.1a: Add proto fields (~3 min)
- `SessionDefaultsConfig` (`proto/session/v1/session.proto:1713-1731`): add
  `int32 stale_session_threshold_minutes = 12;` and `bool stale_session_notify_enabled = 13;`
  with doc comments matching `max_auto_rework_iterations`'s "0 means use the server
  default" style.
- `UpdateGlobalDefaultsRequest` (`proto/session/v1/session.proto:1763-1776`): add
  `int32 stale_session_threshold_minutes = 10;` and `bool stale_session_notify_enabled = 11;`.
- Files: `proto/session/v1/session.proto`

##### Task 1.2.1b: Run `make proto-gen` (~1 min)
- Regenerates `gen/proto/go/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts`.
- Files: (generated — `gen/proto/go/session/v1/session_pb.go`, `web-app/src/gen/session/v1/session_pb.ts`)

##### Task 1.2.1c: Extend `sessionDefaultsToProto` (~2 min)
- Add `StaleSessionThresholdMinutes: int32(cfg.StaleSession.ThresholdMinutesOrDefault())`
  and `StaleSessionNotifyEnabled: cfg.StaleSession.NotifyEnabledOrDefault()` to the
  struct literal at `server/services/defaults_service.go:498-508`.
- Files: `server/services/defaults_service.go`

##### Task 1.2.1d: Extend `UpdateGlobalDefaults` handler (~3 min)
- After the existing `cfg.MaxConcurrentBacklogWorkItems = ...` line
  (`server/services/defaults_service.go:131`), add:
  ```go
  if req.Msg.StaleSessionThresholdMinutes > 0 {
      cfg.StaleSession.ThresholdMinutes = int(req.Msg.StaleSessionThresholdMinutes)
  }
  notifyEnabled := req.Msg.StaleSessionNotifyEnabled
  cfg.StaleSession.NotifyEnabled = &notifyEnabled
  ```
  (mirrors the existing "0 = use default" convention for the int field; the bool field
  is always explicitly applied, matching `auto_yes`'s handling at line 125).
- Files: `server/services/defaults_service.go`

##### Task 1.2.1e: Go tests for the RPC round trip (~4 min)
- Extend `server/services/defaults_service_test.go` (or create if no existing test file
  covers `GetSessionDefaults`/`UpdateGlobalDefaults`): assert the three Given-When-Then
  scenarios from Story 1.2.1's acceptance criteria.
- Files: `server/services/defaults_service_test.go`

---

### Epic 1.3: Settings UI for the new threshold + notify flag
**Goal**: Let the user configure the threshold/notify flag without editing `config.json`
by hand, following `GlobalDefaultsForm.tsx`'s existing `maxAutoReworkIterations` field
exactly.

#### Story 1.3.1: Add threshold + notify controls to `GlobalDefaultsForm`
**As a** user, **I want** to change the stale-session threshold and toggle notifications
from Settings, **so that** I don't need to hand-edit `config.json`.

**Acceptance Criteria**:
- Loading the form pre-fills the current resolved values.
  - *Given* the backend returns `staleSessionThresholdMinutes: 30, staleSessionNotifyEnabled: true`
    from `getSessionDefaults`, *When* `GlobalDefaultsForm` mounts, *Then* the threshold
    input shows `30` and the notify checkbox is checked.
- Saving persists the new values via the existing save button.
  - *Given* the user changes the threshold input to `45` and unchecks notify, *When*
    they click Save, *Then* `updateGlobalDefaults` is called with
    `staleSessionThresholdMinutes: 45, staleSessionNotifyEnabled: false`.
**Files**: `web-app/src/components/settings/GlobalDefaultsForm.tsx`

##### Task 1.3.1a: Add state + load/save wiring (~4 min)
- Add `const [staleSessionThresholdMinutes, setStaleSessionThresholdMinutes] = useState(30);`
  and `const [staleSessionNotifyEnabled, setStaleSessionNotifyEnabled] = useState(true);`
  next to `maxAutoReworkIterations` (`GlobalDefaultsForm.tsx:37-38`).
- In `loadDefaults` (`:59-60` region), add the two `setX(defaults.xxx)` calls.
- In `handleSave`'s `updateGlobalDefaults` call (`:92-101`), add the two fields.
- Files: `web-app/src/components/settings/GlobalDefaultsForm.tsx`

##### Task 1.3.1b: Add the two form controls (~3 min)
- A numeric input (mirroring the `maxAutoReworkIterations` input around line 318) and a
  checkbox (mirroring `RuleBuilderForm.tsx`'s `require-ci-passing-checkbox` shape),
  each with a `data-testid`.
- Files: `web-app/src/components/settings/GlobalDefaultsForm.tsx`

##### Task 1.3.1c: Extend `GlobalDefaultsForm.test.tsx` (~4 min)
- Add cases for the two Given-When-Then scenarios above.
- Files: `web-app/src/components/settings/GlobalDefaultsForm.test.tsx`

---

## Phase 2: Frontend Staleness Helper & SessionCard Badge

### Epic 2.1: Shared frontend staleness helper
**Goal**: One pure module both the badge and the grouping strategy import — no
duplicated timestamp/threshold logic (architecture.md §1).

#### Story 2.1.1: Extract `getLastActivityTimestamp` + add `isSessionStale`
**As a** developer building the badge and the grouping strategy, **I want** one shared
predicate, **so that** the two features can never disagree about what counts as stale.

**Acceptance Criteria**:
- `isSessionStale` returns `false` for any non-`ACTIVE` status regardless of timestamps.
  - *Given* a `Session` with `status: SessionStatus.PAUSED` and
    `lastMeaningfulOutput` timestamped 5 hours ago, *When*
    `isSessionStale(session, 30)` is called, *Then* it returns `false`.
- `isSessionStale` returns `true` for an `ACTIVE` session whose last activity exceeds the
  threshold.
  - *Given* an `ACTIVE` session with `lastMeaningfulOutput` timestamped 45 minutes ago
    and no `lastTerminalUpdate`, *When* `isSessionStale(session, 30)` is called, *Then*
    it returns `true`.
- `isSessionStale` returns `false` for a session with no recorded activity at all (new
  session, never produced output).
  - *Given* an `ACTIVE` session with both `lastMeaningfulOutput` and `lastTerminalUpdate`
    unset (`seconds: 0n`), *When* `isSessionStale(session, 30)` is called, *Then* it
    returns `false` (per `features.md` §3's "don't flag a brand-new session" precedent).
- `getLastActivityTimestamp` is behavior-identical to the existing inline IIFE it
  replaces.
  - *Given* a session with `lastMeaningfulOutput.seconds = 100n` and
    `lastTerminalUpdate.seconds = 200n`, *When* `getLastActivityTimestamp(session)` is
    called, *Then* it returns the `lastTerminalUpdate` timestamp (the more recent of the
    two) — matching `SessionCard.tsx:679-683`'s existing `moSecs >= tuSecs ? ... : ...`
    logic exactly.
**Files**: `web-app/src/lib/session-staleness.ts` (new)

##### Task 2.1.1a: Create `session-staleness.ts` with `getLastActivityTimestamp` (~4 min)
- New file exporting `getLastActivityTimestamp(session: Session): Timestamp | undefined`,
  a direct extraction of the IIFE at `SessionCard.tsx:678-687`.
- Files: `web-app/src/lib/session-staleness.ts`

##### Task 2.1.1b: Add `isSessionStale` (~4 min)
- `export function isSessionStale(session: Session, thresholdMinutes: number): boolean`.
  Gate on `session.status === SessionStatus.ACTIVE` first (return `false` immediately
  otherwise — covers PAUSED/STOPPED/HIBERNATED/CREATING/RESTORING per `features.md` §4;
  archived sessions are already excluded transitively since `ArchiveWithStop` stops the
  session first, per architecture research). Then call `getLastActivityTimestamp`; if
  `undefined`, return `false`. Otherwise compare `Date.now() - timestampMs > thresholdMinutes * 60_000`.
- Files: `web-app/src/lib/session-staleness.ts`

##### Task 2.1.1c: Refactor `SessionCard.tsx`'s inline IIFE to call `getLastActivityTimestamp` (~3 min)
- Replace the body of the IIFE at `SessionCard.tsx:678-687` with a call to the new
  helper; behavior must be unchanged (existing "Active Xm ago" row still renders
  identically).
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 2.1.1d: Unit tests for `session-staleness.ts` (~5 min)
- New `web-app/src/lib/session-staleness.test.ts` covering all four Given-When-Then
  scenarios above plus a boundary case (`thresholdMinutes` exactly equal to elapsed
  time — document whether `>` or `>=` is used and assert it).
- Files: `web-app/src/lib/session-staleness.test.ts`

---

### Epic 2.2: SessionCard badge UI
**Goal**: A new peer badge in the existing header badge row, matching the
`chipStaleWork` icon+text+warning-token shape (UX research §1–3).

#### Story 2.2.1: Render the stale badge in `SessionCard`'s header row
**As a** user scanning 5–10 session cards, **I want** a glanceable "Stale" badge on any
silently-idle session, **so that** I don't have to open each card to find the one that
died.

**Acceptance Criteria**:
- Badge renders for a stale `ACTIVE` session, with accessible text (not color-only).
  - *Given* an `ACTIVE` session where `isSessionStale(session, staleThresholdMinutes)`
    is `true`, *When* `SessionCard` renders, *Then* a badge with visible text `"Stale"`
    and `role="img"` `aria-label` starting with `"Stale — no output for"` appears as a
    new peer in the header badge row (after the existing `SubStatusChip`, per the
    existing badge-row ordering at `SessionCard.tsx:505-548`).
- Badge does not render for a non-stale or non-`ACTIVE` session.
  - *Given* a `PAUSED` session whose last output was 6 hours ago, *When* `SessionCard`
    renders, *Then* no "Stale" badge appears anywhere in the card.
- Badge reuses the existing warning color tokens, not a new bespoke color.
  - *Given* the badge renders, *When* its CSS is inspected, *Then* it uses
    `vars.color.warningBg`/`warningText`/`warning` (the same triplet `chipStaleWork`
    uses in `StuckItem.css.ts`), not a new hex value.
**Files**: `web-app/src/components/sessions/SessionCard.tsx`, `SessionCard.css.ts`

##### Task 2.2.1a: Add `staleThresholdMinutes` prop to `SessionCard` (~2 min)
- Add `staleThresholdMinutes: number` to `SessionCard`'s props interface (near the other
  primitive props, above the `session` prop destructure at the top of the component).
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 2.2.1b: Add the badge JSX (~4 min)
- Insert a new `{isSessionStale(session, staleThresholdMinutes) && (<span role="img" aria-label={...} className={staleBadge}>🟠 Stale</span>)}`
  block as a new peer after the `SubStatusChip` block (`SessionCard.tsx:543-548`),
  importing `isSessionStale` from the new helper module.
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 2.2.1c: Add `staleBadge` vanilla-extract style (~3 min)
- New export in `SessionCard.css.ts` reusing `vars.color.warningBg`/`warningText`/`warning`
  (check `chipStaleWork` in `web-app/src/components/backlog-stuck/StuckItem.css.ts:44-51`
  for the exact token names to mirror).
- Files: `web-app/src/components/sessions/SessionCard.css.ts`

##### Task 2.2.1d: Thread `staleThresholdMinutes` from `SessionList` down to `SessionCard` (~3 min)
- In `SessionList.tsx`, call the new `useStaleSessionConfig()` hook (Task 2.3.1a) once
  near the top of the component, and pass `staleThresholdMinutes={staleSessionConfig.thresholdMinutes}`
  at the single `<SessionCard ... />` call site (`SessionList.tsx:1563` region).
- Files: `web-app/src/components/sessions/SessionList.tsx`

##### Task 2.2.1e: Component tests for the badge (~5 min)
- Extend or create `SessionCard.test.tsx`: assert Story 2.2.1's three Given-When-Then
  scenarios via React Testing Library (`getByRole("img", { name: /Stale/ })`).
- Files: `web-app/src/components/sessions/SessionCard.test.tsx`

---

### Epic 2.3: Periodic re-render tick (verified gap — no existing ticker covers this)
**Goal**: Confirmed by this plan's research pass: `SessionCard.tsx`'s `formatTimeAgo`
(`:274`) and the badge computation are both pure functions of render-time data with
**no existing interval** forcing a re-render purely from clock time passing (the
`setInterval` at `useSessionService.ts:961` is a websocket-reconnect backstop, not a
data/UI refresh). Since a genuinely-stale session by definition stops producing new
`WatchSessions` events, nothing today would flip its badge/group membership from
"fresh" to "stale" until an unrelated re-render happens to occur. This is a
correctness gap Phase 3/5's own research flagged as "verify, don't assume" —
verified here as real.

#### Story 2.3.1: Add `useStaleSessionConfig` hook + a lightweight re-render tick
**As a** user watching the session grid, **I want** the Stale badge/group to appear
within the poll interval of a session actually going stale, **so that** I don't have to
manually refresh the page to see it.

**Acceptance Criteria**:
- The threshold/notify config is fetched once and made available to `SessionList`.
  - *Given* `getSessionDefaults` returns `staleSessionThresholdMinutes: 45`, *When*
    `useStaleSessionConfig()` is called from `SessionList`, *Then* it returns
    `{ thresholdMinutes: 45, notifyEnabled: true }` after the fetch resolves (and a safe
    default `{ thresholdMinutes: 30, notifyEnabled: true }` before it resolves).
- A session that goes idle past the threshold is flagged within 60 seconds without any
  new session data arriving.
  - *Given* an `ACTIVE` session was fresh 29 minutes ago (below a 30-minute threshold)
    and produces no further output, *When* 2 more minutes of wall-clock time pass with
    no new `WatchSessions` event, *Then* `SessionList`'s periodic tick causes a
    re-render and the badge/grouping strategy correctly reclassify the session as stale
    within 60 seconds of crossing the threshold.
**Files**: `web-app/src/lib/hooks/useStaleSessionConfig.ts` (new), `web-app/src/components/sessions/SessionList.tsx`

##### Task 2.3.1a: Create `useStaleSessionConfig` hook (~4 min)
- New file, modeled on `useSessionDefaults.ts`'s `createClient`/`getSessionDefaults`
  pattern but simpler (single fetch on mount, no per-directory resolution): returns
  `{ thresholdMinutes: number; notifyEnabled: boolean }`, defaulting to `{30, true}`
  before the fetch resolves.
- Files: `web-app/src/lib/hooks/useStaleSessionConfig.ts`

##### Task 2.3.1b: Add a 60-second re-render tick in `SessionList` (~4 min)
- Add `const [, forceStaleRecompute] = useState(0);` and a `useEffect` with
  `setInterval(() => forceStaleRecompute((n) => n + 1), 60_000)` (cleaned up on
  unmount). Add the tick state as a dependency of the `groupedSessions` `useMemo`
  (`SessionList.tsx:648-650`) so a stale-crossing session's group membership recomputes
  on the next tick even with no new `sessions` data.
- Files: `web-app/src/components/sessions/SessionList.tsx`

##### Task 2.3.1c: Hook + tick tests (~5 min)
- `useStaleSessionConfig.test.ts` for the fetch/default behavior; a `SessionList.test.tsx`
  case using fake timers to assert the badge/group reclassifies after the tick without
  new session props (jest `jest.useFakeTimers()` + `jest.advanceTimersByTime(60_000)`).
- Files: `web-app/src/lib/hooks/useStaleSessionConfig.test.ts`, `web-app/src/components/sessions/SessionList.test.tsx`

---

## Phase 3: "Stale" Grouping Strategy

### Epic 3.1: `GroupingStrategy.Stale`
**Goal**: A pure client-side addition — zero backend changes — bucketing sessions into
"Stale" / everything-else using the same shared helper as the badge.

#### Story 3.1.1: Add the `Stale` enum member, label, and switch case
**As a** user, **I want** to group/filter the session list to show only stale sessions,
**so that** I can find all silently-dead agents at once instead of scanning card by
card.

**Acceptance Criteria**:
- Selecting the Stale strategy buckets sessions correctly.
  - *Given* three `ACTIVE` sessions where session A is stale (idle 45 min, threshold 30),
    session B is fresh (idle 2 min), and one `PAUSED` session C is idle 6 hours, *When*
    `groupSessions([A, B, C], GroupingStrategy.Stale)` is called with `thresholdMinutes: 30`,
    *Then* the result has a `"Stale"` group containing only session A (never B or the
    paused session C).
- The strategy has a display label (not silently blank in the selector).
  - *Given* the grouping strategy `<select>` renders, *When* the options are enumerated,
    *Then* `GroupingStrategyLabels[GroupingStrategy.Stale] === "Stale"` appears as a
    selectable option labeled "Stale."
**Files**: `web-app/src/lib/grouping/strategies.ts`

##### Task 3.1.1a: Add `Stale = "stale"` enum member + label (~2 min)
- Add to the `GroupingStrategy` enum (`strategies.ts:9-19`) and
  `GroupingStrategyLabels` (`:21-31`) — both are required per UX research §3's explicit
  warning that the labels map is not compile-enforced.
- Files: `web-app/src/lib/grouping/strategies.ts`

##### Task 3.1.1b: Extend `groupSessions` signature + add the `Stale` switch case (~4 min)
- `groupSessions` (`:69-73`) needs a new parameter (or an addition to the existing
  `GroupSessionsOptions` interface, `:67`) to receive `thresholdMinutes: number` — the
  grouping computation is otherwise a pure function of already-in-memory `Session[]`
  data, per architecture.md §4, and needs no new RPC.
- Add `case GroupingStrategy.Stale: groupKeys = [isSessionStale(session, options?.thresholdMinutes ?? 30) ? "Stale" : "Not Stale"];`
  (label is `"Not Stale"`, not `"Active"` — a PAUSED/STOPPED session in the fallback
  bucket under a header literally reading "Active" would be misleading; per
  design/ux.md Surface 2's explicit recommendation)
  alongside the other single-membership cases (pattern-match `case GroupingStrategy.Status:`,
  `:127-130`), importing `isSessionStale` from `session-staleness.ts`.
- Files: `web-app/src/lib/grouping/strategies.ts`

##### Task 3.1.1c: Thread `thresholdMinutes` from `SessionList`'s `groupSessions()` call (~2 min)
- Update the `groupSessions(sortedSessions, groupingStrategy)` call
  (`SessionList.tsx:649`) to pass `{ thresholdMinutes: staleSessionConfig.thresholdMinutes }`
  (from the Task 2.3.1a hook, already fetched for the badge).
- Files: `web-app/src/components/sessions/SessionList.tsx`

##### Task 3.1.1d: Unit tests for the `Stale` grouping case (~4 min)
- Add to `strategies.test.ts` if one exists, else create it: assert Story 3.1.1's two
  Given-When-Then scenarios, plus confirm `"Stale"` is excluded from the
  `specialGroups` end-of-list sort array (`:172-173`) so it sorts normally.
- Files: `web-app/src/lib/grouping/strategies.test.ts`

---

## Phase 4: Backend Stale-Session Notification

### Epic 4.1: `StaleSessionNotifier` type
**Goal**: A small, independent, `SessionRetentionSweeper`-shaped periodic sweeper —
edge-triggered, self-clearing, config-gated. Deliberately does not modify
`ReviewQueuePoller`'s own tuned logic (Pattern Decisions row 4).

#### Story 4.1.1: Implement the notifier with edge-triggered, self-clearing dedup
**As a** user not currently looking at the session grid, **I want** a push notification
the first time a session goes stale, **so that** I find out without having to keep
checking manually.

**Acceptance Criteria**:
- Fires exactly once per continuous stale episode.
  - *Given* an `ACTIVE` session crosses the 30-minute threshold at tick N, *When* ticks
    N+1 and N+2 run with the session still stale and no output, *Then* exactly one
    `NOTIFICATION_TYPE_WARNING` event is published for that session (not three).
- Re-arms after recovery.
  - *Given* the same session then produces output (idle time drops back under
    threshold) and later goes stale again, *When* the notifier ticks through this
    recover→re-stale sequence, *Then* a second, new notification fires for the second
    stale episode.
- Never fires for a non-`ACTIVE` session.
  - *Given* a session transitions `ACTIVE → PAUSED` in the same tick its idle time
    crosses the threshold, *When* the notifier evaluates that tick, *Then* no
    notification fires (status is checked at emission time, not just threshold-crossing
    time, per UX research §4's edge case).
- Respects the `notify_enabled` config flag.
  - *Given* `cfg.StaleSession.NotifyEnabledOrDefault() == false`, *When* the notifier
    ticks with a session past threshold, *Then* no event is published.
**Files**: `server/services/stale_session_notifier.go` (new)

##### Task 4.1.1a: Scaffold `StaleSessionNotifier` struct + constructor (~3 min)
- New file, modeled on `server/services/session_retention_sweeper.go`'s shape:
  `type StaleSessionNotifier struct { poller *session.ReviewQueuePoller; eventBus *events.EventBus; mu sync.Mutex; notifiedSessions map[string]time.Time }`,
  `func NewStaleSessionNotifier(poller *session.ReviewQueuePoller, eventBus *events.EventBus) *StaleSessionNotifier`.
  **Deliberately does not capture a `*config.Config` pointer on the struct** — see
  Task 4.1.1c: `checkAll()` calls `config.LoadConfig()` itself each tick, so a Settings
  UI change is observed on the next tick without a process restart. (Fixes
  adversarial-review.md's BLOCKER: the original design mirrored `HibernationSweeper`/
  `SessionRetentionSweeper`'s startup-captured-pointer pattern, which — per
  `DefaultsService.sharedBacklogCfg`'s own doc comment, "PR #199 review F1" — is exactly
  the pattern that silently stopped `UpdateGlobalDefaults` changes from reaching a
  long-lived consumer for `MaxAutoReworkIterations`/`MaxConcurrentBacklogWorkItems`.
  `StaleSessionNotifier` avoids that bug class by never holding a stale snapshot.)
  (Decision: do not extract a shared `session.IsStale` helper for this one call site —
  the comparison is a single `time.Since(x) > threshold` line; extracting a helper for
  exactly one new caller would be premature per the interface-pollution-checklist's
  "unjustified generic" smell (a plain-function extraction, not a generic — see
  architecture-review.md's Concerns for why this citation is imprecise but the
  conclusion holds: `pkg/classifier` structurally cannot import `session/`, so a shared
  Go helper could only ever serve this one call site). Revisit only if a second new Go
  consumer appears.)
- Files: `server/services/stale_session_notifier.go`

##### Task 4.1.1b: Implement `Start(ctx)` + `checkAll()` ticker loop (~5 min)
- `const staleSessionNotifierCheckInterval = 60 * time.Second` (30x headroom below the
  30-min default threshold, comfortably clearing the "≥3-5x poll interval" guidance from
  `review-gate-stale-session-rework`'s pitfalls.md §5). `Start(ctx)` mirrors
  `SessionRetentionSweeper.Start` (`session_retention_sweeper.go:44-62`): ticker loop,
  runs `checkAll()` immediately then on each tick, exits on `ctx.Done()`.
- Files: `server/services/stale_session_notifier.go`

##### Task 4.1.1c: Implement `checkAll()` — the edge-triggered dedup core (~6 min)
- `cfg := config.LoadConfig()` **first line of `checkAll()`** (cheap local JSON read on
  a 60s ticker, mirrors `GetSessionDefaults`'s already-correct per-request-fresh-read
  pattern at `defaults_service.go:86`) — this is what makes a Settings UI threshold/
  notify-flag change take effect on the next tick with no restart; do not hoist this to
  the constructor or the struct.
- For each `inst := range n.poller.GetInstances()`:
  - If `inst.GetStatus() != int(sessionv1.SessionStatus_SESSION_STATUS_ACTIVE)`: under
    `n.mu`, delete `inst.GetStableID()` from `notifiedSessions` if present (clears the
    dedup entry on **any** ACTIVE→non-ACTIVE transition, not only on idle-time
    recovery), then `continue`. (Fixes adversarial-review.md's Concern: without this,
    a session notified-stale, then paused, then resumed while still idle past
    threshold would never re-notify, because the old code's early `continue` for
    non-`ACTIVE` instances skipped the clear branch entirely.)
  - Compute `idle := inst.GetTimeSinceLastMeaningfulOutput()`,
    `threshold := time.Duration(cfg.StaleSession.ThresholdMinutesOrDefault()) * time.Minute`.
  - Under `n.mu`: if `idle > threshold` and `inst.GetStableID()` not already in
    `notifiedSessions`, record it and (outside the lock) call `n.notify(inst)` if
    `cfg.StaleSession.NotifyEnabledOrDefault()`. If `idle <= threshold` and the ID *is*
    in `notifiedSessions`, delete it (re-arm for next episode).
- Files: `server/services/stale_session_notifier.go`

##### Task 4.1.1d: Implement `notify(inst)` — publish onto the event bus (~3 min)
- Mirror `backlog_service_triage.go:1232-1239`'s `events.NewNotificationEvent` call
  exactly: `sessionID = inst.GetStableID()`, `sessionName = inst.Title`,
  `notificationID = uuid.New().String()`,
  `notificationType = int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING)`,
  `priority = int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM)`,
  title `"Session went stale"`, message referencing `idle` duration, metadata
  `{"session_id": sessionID, "reason": "stale"}`.
- Files: `server/services/stale_session_notifier.go`

##### Task 4.1.1e: Unit tests for edge-trigger/re-arm/status-gate/flag-gate/live-reload/pause-resume (~8 min)
- New `server/services/stale_session_notifier_test.go` using a fake/minimal
  `*session.Instance` (or the existing test-instance factory used by
  `backlog_service_triage_test.go`, if one exists) covering all four Story 4.1.1
  Given-When-Then scenarios. Verify via a fake `events.EventBus` subscriber (check
  existing test patterns for `eventBus.Publish` assertions, e.g. in
  `backlog_service_triage_test.go`, and mirror that harness).
- Add two tests closing the adversarial-review.md findings:
  - `checkAll_should_ObserveConfigChange_When_ConfigFileChangesBetweenTicks` — write a
    config file, construct the notifier, call `checkAll()` once, rewrite the config
    file with a different `threshold_minutes`/`notify_enabled`, call `checkAll()` again,
    assert the second call used the new value (e.g. a session idle between the old and
    new threshold only notifies after the change) — proves live-reload without
    restart, addressing the BLOCKER.
  - `checkAll_should_ReNotify_When_SessionPausesThenResumesStillStale` — session goes
    stale (notifies once), transitions to PAUSED (assert `notifiedSessions` entry is
    cleared), transitions back to ACTIVE while still past threshold, `checkAll()` runs
    again, assert a *second* notification fires — addressing the Concern.
- Files: `server/services/stale_session_notifier_test.go`

---

### Epic 4.2: Wire `StaleSessionNotifier` into server startup
**Goal**: Start/stop lifecycle identical to `HibernationSweeper`/`SessionRetentionSweeper`.

#### Story 4.2.1: Register the notifier in `wireDepsIntoServer`
**As an** operator, **I want** the notifier to start automatically with the server,
**so that** no manual step is needed to enable stale-session notifications.

**Acceptance Criteria**:
- The notifier starts only when a `ReviewQueuePoller` is available (mirrors the
  hibernation sweeper's nil-guard).
  - *Given* `deps.ReviewQueuePoller != nil`, *When* `wireDepsIntoServer` runs, *Then* a
    `StaleSessionNotifier` is constructed and `go notifier.Start(serverCtx)` is called,
    logging its resolved threshold at startup.
**Files**: `server/server.go`

##### Task 4.2.1a: Add the startup block (~3 min)
- Insert after the `SessionRetentionSweeper` block (`server/server.go:694-701`):
  ```go
  if deps.ReviewQueuePoller != nil {
      staleNotifier := services.NewStaleSessionNotifier(deps.ReviewQueuePoller, deps.EventBus)
      go staleNotifier.Start(serverCtx)
      log.Info("Stale session notifier started",
          "threshold_minutes", cfg.StaleSession.ThresholdMinutesOrDefault(),
          "notify_enabled", cfg.StaleSession.NotifyEnabledOrDefault())
  }
  ```
  Note: `cfg` is used only for this startup log line (the resolved values *at startup*);
  the constructor itself takes no `cfg` argument — `checkAll()` re-reads
  `config.LoadConfig()` every tick per Task 4.1.1c, so the startup log line's values may
  go stale but the notifier's actual behavior never does.
- Files: `server/server.go`

##### Task 4.2.1b: Manual smoke test (~5 min, not automated)
- Run a manual second instance (per `CLAUDE.md`'s manual-testing convention:
  `go build -o /tmp/ssq-manual-test . && PORT=8999 STAPLER_SQUAD_INSTANCE=stale-notifier-smoke /tmp/ssq-manual-test --tmux-keep-server`),
  create a session, let it sit idle past a temporarily-lowered threshold
  (`stale_session.threshold_minutes: 1` in that instance's config for the smoke test
  only), and confirm exactly one notification appears in the NotificationPanel, no
  repeats, and it clears on the next session output. Not a task file — a manual
  verification step recorded here per the Risk Control section's staged-rollout note.

---

## Phase 5: Approval-Rule `min_session_idle_minutes` Condition

### Epic 5.1: Proto + ent schema
**Goal**: Field 30 on `ApprovalRuleProto`, following `require_ci_passing`'s exact
footprint (stack.md §2, architecture.md §3).

#### Story 5.1.1: Add the proto field and ent schema field
**As a** rule author, **I want** a `min_session_idle_minutes` condition available on
approval rules, **so that** I can write rules like "deny if the requesting session has
been idle 60+ minutes."

**Acceptance Criteria**:
- The field round-trips through proto codegen with `0` meaning "not applied."
  - *Given* an `ApprovalRuleProto` constructed with no `min_session_idle_minutes` set,
    *When* it's serialized and deserialized, *Then* `MinSessionIdleMinutes == 0`
    (Go zero value), consistent with `require_ci_passing`'s `false`-means-off idiom.
**Files**: `proto/session/v1/types.proto`, `session/ent/schema/approvalrule.go`

##### Task 5.1.1a: Add `int32 min_session_idle_minutes = 30;` to `ApprovalRuleProto` (~2 min)
- Add directly below `bool require_ci_passing = 29;` (`proto/session/v1/types.proto:1107`),
  before the message's closing brace, with a doc comment mirroring
  `require_ci_passing`'s style ("matches only if the requesting session has been idle
  ≥ N minutes since its last meaningful output; 0 = condition not applied").
- Files: `proto/session/v1/types.proto`

##### Task 5.1.1b: Add ent schema field (~2 min)
- `field.Int32("min_session_idle_minutes").Optional().Default(0)` directly below
  `field.Bool("require_ci_passing")` (`session/ent/schema/approvalrule.go:80`).
- Files: `session/ent/schema/approvalrule.go`

##### Task 5.1.1c: Run `make proto-gen` + ent codegen (~2 min)
- `make build` regenerates both via its dependency graph (per stack.md §"Codegen
  requirements"), or run `make proto-gen && make ent-gen` explicitly. **Never** hand-run
  `go run entgo.io/ent/cmd/ent generate ./session/ent/schema` without
  `--feature sql/upsert` — see `.claude/rules/ent-schema-generation.md`.
- Files: (generated — `gen/proto/go/session/v1/types_pb.go`, `session/ent/approvalrule/*.go`, `session/ent/approvalrule.go`, `session/ent/approvalrule_*.go`, `web-app/src/gen/session/v1/types_pb.ts`)

---

### Epic 5.2: Repository / store / service conversion layers
**Goal**: The four hand-written conversion sites `require_ci_passing` touches (stack.md
§2, steps 3–6) — all must move together or the field silently drops on one hop.

#### Story 5.2.1: Thread `MinSessionIdleMinutes` through `ApprovalRuleData`, `RuleSpec`, and both proto↔domain conversion functions
**As a** developer, **I want** the new field to survive every persistence/conversion
hop, **so that** a rule saved via the RPC is the same rule read back later.

**Acceptance Criteria**:
- A rule with `MinSessionIdleMinutes: 60` survives a full save→load round trip through
  the ent-backed store.
  - *Given* a rule is upserted via `RulesService.UpsertApprovalRule` with
    `min_session_idle_minutes: 60`, *When* it's subsequently read back via
    `ListApprovalRules`, *Then* the returned proto's `min_session_idle_minutes == 60`.
**Files**: `session/repository.go`, `session/ent_repository.go`, `server/services/rules_store.go`, `server/services/rules_service.go`

##### Task 5.2.1a: Add `MinSessionIdleMinutes int32` to `ApprovalRuleData` (~2 min)
- Add below `RequireCIPassing bool` (`session/repository.go:223`).
- Files: `session/repository.go`

##### Task 5.2.1b: Wire ent row ↔ `ApprovalRuleData` conversion (~2 min)
- Add `MinSessionIdleMinutes: rule.MinSessionIdleMinutes,` alongside
  `RequireCIPassing: rule.RequireCiPassing,` (`session/ent_repository.go:1256`), and
  `.SetMinSessionIdleMinutes(data.MinSessionIdleMinutes)` alongside
  `.SetRequireCiPassing(data.RequireCIPassing)` (`session/ent_repository.go:1314`).
- Files: `session/ent_repository.go`

##### Task 5.2.1c: Add `MinSessionIdleMinutes` to `RuleSpec` + its 3 conversion sites (~4 min)
- Add `MinSessionIdleMinutes int32 \`json:"min_session_idle_minutes,omitempty"\`` below
  `RequireCIPassing` (`server/services/rules_store.go:48`), and thread it through all
  three conversion call sites at `:147`, `:235`, `:289`.
- Files: `server/services/rules_store.go`

##### Task 5.2.1d: Add `MinSessionIdleMinutes` to `pkg/classifier.Rule` + all 4 `rules_service.go` conversion sites (~5 min)
- Add `MinSessionIdleMinutes int32` to `classifier.Rule` (`pkg/classifier/classifier.go:367`,
  see Epic 5.3 for the struct edit itself). Thread through `rules_service.go`'s four
  proto↔spec↔classifier.Rule conversion call sites at lines `~119`, `~472`, `~493`,
  `~1222` (each currently has a `RequireCIPassing: ...` line to mirror).
- Files: `server/services/rules_service.go`

##### Task 5.2.1e: Go tests for the round trip (~6 min)
- **Field-specific, not folded into a general pattern test** (pre-mortem.md P1: this
  field crosses 4+ hand-edited, non-exhaustive conversion hops with no compile-time
  completeness check — a future refactor that drops it at any hop compiles cleanly and
  silently stops applying the idle-time gate, a "worse blast radius than a UI badge"
  per pitfalls.md). Add one dedicated test,
  `MinSessionIdleMinutes_should_SurviveRoundTrip_When_PersistedAndReloaded`, asserting
  the value survives proto → `ApprovalRuleData` → ent row → `RuleSpec` →
  `pkg/classifier.Rule` end to end with a value that would be truncated/dropped/zeroed
  by a mistake at any single hop (e.g. `60`, distinguishable from every other field's
  test value). This must fail if any one of the 5 conversion sites in Epic 5.2/5.3 is
  reverted or missed — do not rely solely on the existing `RequireCIPassing`-pattern
  test extended in place.
- Files: `server/services/rules_service_test.go` (or `rules_store_test.go`, matching wherever the existing `RequireCIPassing` round-trip test lives)

---

### Epic 5.3: Classifier match logic
**Goal**: `pkg/classifier` never imports `session/` — the field is a plain value the
caller computes and passes in (Pattern Decisions row 3; pitfalls.md §5).

#### Story 5.3.1: Add `MinSessionIdleMinutes` to `Rule`, `SessionIdleMinutes` to `ClassificationContext`, and the AND-condition in `matchesRule`
**As a** rule author, **I want** the condition to actually gate classification decisions,
**so that** a rule like "deny if idle ≥ 60 min" has real effect.

**Acceptance Criteria**:
- The condition matches when idle time meets the configured minimum.
  - *Given* a rule with `MinSessionIdleMinutes: 60` and
    `ClassificationContext{SessionIdleMinutes: 75}`, *When* `matchesRule` evaluates that
    rule, *Then* the condition passes (does not short-circuit to `false` on this check).
- The condition fails to match when idle time is below the minimum.
  - *Given* the same rule and `ClassificationContext{SessionIdleMinutes: 10}`, *When*
    `matchesRule` evaluates, *Then* it returns `false` (rule does not fire).
- The condition is ANDed with other conditions on the same rule, not OR'd.
  - *Given* a rule with both `RequireCIPassing: true` and `MinSessionIdleMinutes: 60`,
    and a context with `CIStatus: "success"` but `SessionIdleMinutes: 5`, *When*
    `matchesRule` evaluates, *Then* it returns `false` (the idle condition alone blocks
    the match even though CI passes).
- Fail-closed: a zero/unset `ctx.SessionIdleMinutes` never satisfies a
  `MinSessionIdleMinutes > 0` rule.
  - *Given* a rule with `MinSessionIdleMinutes: 60` and a
    `ClassificationContext{}` (zero value, as if the caller never populated
    `SessionIdleMinutes` because no live instance was found), *When* `matchesRule`
    evaluates, *Then* it returns `false` — the rule does not fire, and (at the
    `Classify()` level) evaluation falls through toward the escalate catch-all rather
    than silently matching.
**Files**: `pkg/classifier/classifier.go`

##### Task 5.3.1a: Add `MinSessionIdleMinutes int32` field to `Rule` (~2 min)
- Add below `RequireCIPassing bool` (`pkg/classifier/classifier.go:367`), with a doc
  comment mirroring `RequireCIPassing`'s: "matches only if `ClassificationContext.SessionIdleMinutes >= MinSessionIdleMinutes`. 0 = condition not applied. ANDed with all other conditions on this rule."
- Files: `pkg/classifier/classifier.go`

##### Task 5.3.1b: Add `SessionIdleMinutes int` field to `ClassificationContext` (~2 min)
- Add below `CIStatus string` (`pkg/classifier/classifier.go:75`), with a doc comment
  explaining the fail-closed contract: "0 (unset) means unknown/unavailable and must
  never accidentally satisfy a `MinSessionIdleMinutes > 0` condition — see
  `ApprovalHandler`'s population logic."
- Files: `pkg/classifier/classifier.go`

##### Task 5.3.1c: Add the AND-condition check in `matchesRule` (~3 min)
- Immediately after the existing `if rule.RequireCIPassing && ctx.CIStatus != ciConclusionSuccess { return false }`
  block (`pkg/classifier/classifier.go:735-737`), add:
  ```go
  if rule.MinSessionIdleMinutes > 0 && ctx.SessionIdleMinutes < int(rule.MinSessionIdleMinutes) {
      return false
  }
  ```
- Files: `pkg/classifier/classifier.go`

##### Task 5.3.1d: Unit tests mirroring `TestClassify_RequireCIPassing_*` (~6 min)
- New `TestClassify_MinSessionIdleMinutes_Matches_WhenIdleExceedsThreshold`,
  `_DoesNotMatch_WhenIdleBelowThreshold`,
  `_ANDsWithOtherConditions_WhenCombinedWithRequireCIPassing`, and
  `_FailsClosed_WhenContextIdleUnset`, following the 4-test shape of
  `TestClassify_RequireCIPassing_*` (`pkg/classifier/classifier_test.go:3271-3360`).
- Files: `pkg/classifier/classifier_test.go`

---

### Epic 5.4: `ApprovalHandler` context population
**Goal**: Populate `ClassificationContext.SessionIdleMinutes` from the live instance,
right next to the existing `CIStatus` population block — and nowhere else (keeps the
classifier package's "no session import" boundary intact).

#### Story 5.4.1: Populate `SessionIdleMinutes` in `HandlePermissionRequest`
**As a** developer, **I want** the idle-minutes context populated the same way CI
status already is, **so that** the fail-closed contract established in Epic 5.3 is
honored end to end.

**Acceptance Criteria**:
- A live instance's idle time is correctly converted to minutes and populated.
  - *Given* `h.liveFinder.FindLiveInstance(sessionID)` returns a non-nil `*Instance`
    whose `GetTimeSinceLastMeaningfulOutput()` returns `75 * time.Minute`, *When*
    `HandlePermissionRequest` builds `classCtx`, *Then* `classCtx.SessionIdleMinutes == 75`.
- No live instance found → context left unset (fail-closed).
  - *Given* `h.liveFinder.FindLiveInstance(sessionID)` returns `nil`, *When*
    `HandlePermissionRequest` builds `classCtx`, *Then* `classCtx.SessionIdleMinutes == 0`
    (Go zero value — never explicitly set to a sentinel "infinite" or "unknown" value).
**Files**: `server/services/approval_handler.go`

##### Task 5.4.1a: Add the population line inside the existing `liveFinder` block (~3 min)
- Inside the `if inst := h.liveFinder.FindLiveInstance(sessionID); inst != nil { ... }`
  block (`server/services/approval_handler.go:311-326`), alongside the existing
  `ghInfo := inst.Snapshot().GitHub` line, add:
  ```go
  classCtx.SessionIdleMinutes = int(inst.GetTimeSinceLastMeaningfulOutput().Minutes())
  ```
  Deliberately placed unconditionally within the existing nil-guarded block (not inside
  the `if ghInfo.GitHubPRNumber > 0` sub-block) — idle time is independent of whether the
  session has an open PR.
- Files: `server/services/approval_handler.go`

##### Task 5.4.1b: Go tests for the population + fail-closed path (~5 min)
- Extend whichever existing test file covers `HandlePermissionRequest`'s CI-status
  population (search for the existing test asserting `classCtx.CIStatus` population) to
  add the two Story 5.4.1 Given-When-Then scenarios.
- Files: `server/services/approval_handler_test.go`

---

### Epic 5.5: Frontend rule builder UI
**Goal**: A numeric input sibling to the existing `requireCiPassing` checkbox, following
`RuleBuilderForm.tsx`'s exact state/load/save/render shape.

#### Story 5.5.1: Add `minSessionIdleMinutes` input to `RuleBuilderForm`
**As a** user, **I want** to set the idle-minutes threshold when creating/editing an
approval rule, **so that** I don't need to call the RPC by hand.

**Acceptance Criteria**:
- Editing an existing rule pre-fills the current value.
  - *Given* `editRule.minSessionIdleMinutes === 60`, *When* `RuleBuilderForm` mounts in
    edit mode, *Then* the numeric input shows `60`.
- Saving includes the field in the upsert payload.
  - *Given* the user sets the input to `90` and submits, *When* the form saves, *Then*
    the `upsertApprovalRule` payload includes `minSessionIdleMinutes: 90`.
**Files**: `web-app/src/components/rules/RuleBuilderForm.tsx`, `web-app/src/lib/hooks/useApprovalRules.ts`

##### Task 5.5.1a: Add state + load/save wiring in `RuleBuilderForm` (~4 min)
- Mirror `requireCiPassing`'s three touchpoints exactly: `useState` declaration
  (`:109` region), template-seed/edit-rule hydration (`:163`, `:229`), and the save
  payload (`:319`).
- Files: `web-app/src/components/rules/RuleBuilderForm.tsx`

##### Task 5.5.1b: Add the numeric input JSX (~3 min)
- New labeled numeric input with `data-testid="min-session-idle-minutes-input"`, sibling
  to the `require-ci-passing-checkbox` block (`:564-567`), with placeholder/help text
  "0 = not applied."
- Files: `web-app/src/components/rules/RuleBuilderForm.tsx`

##### Task 5.5.1c: Thread the field through `useApprovalRules.ts` (~2 min)
- Add `minSessionIdleMinutes: ruleData.minSessionIdleMinutes ?? 0,` alongside the two
  existing `requireCiPassing: ...` lines (`useApprovalRules.ts:105`, `:141`).
- Files: `web-app/src/lib/hooks/useApprovalRules.ts`

##### Task 5.5.1d: Component tests (~4 min)
- Extend `RuleBuilderForm.test.tsx` with Story 5.5.1's two Given-When-Then scenarios.
- Files: `web-app/src/components/rules/RuleBuilderForm.test.tsx`

---

### Epic 5.6: End-to-end integration test
**Goal**: One test exercising the full chain per `requirements.md`'s Success Metrics
("exercised by at least one test").

#### Story 5.6.1: Integration test — a real rule denies on session idle time
**As a** developer, **I want** confidence the whole chain works together (not just each
layer in isolation), **so that** a regression in any hop is caught.

**Acceptance Criteria**:
- An `UpsertApprovalRule` call with `min_session_idle_minutes: 30` and `decision: DENY`,
  followed by a `HandlePermissionRequest` call against a session idle 45 minutes,
  results in a `Deny` decision citing that rule's ID.
  - *Given* a rule `{ Decision: Deny, MinSessionIdleMinutes: 30, ToolName: "Bash" }` is
    upserted, and a live instance for session `S1` reports
    `GetTimeSinceLastMeaningfulOutput() == 45 * time.Minute`, *When* a `Bash` tool-use
    permission request for `S1` is handled, *Then* the response decision is `Deny` with
    `RuleID` matching the upserted rule.
**Files**: `server/services/approval_handler_test.go` (or a new integration test file if the existing suite doesn't already compose `RulesService` + `ApprovalHandler` together)

##### Task 5.6.1a: Write the integration test (~6 min)
- Compose `RulesService.UpsertApprovalRule` → `ApprovalHandler.HandlePermissionRequest`
  end to end, using a fake/test `LiveInstanceFinder` returning a fixture instance with
  the desired idle time (check whether `approval_handler_test.go` already has a fake
  `liveFinder` fixture to reuse, per Task 5.4.1b's search).
- Files: `server/services/approval_handler_test.go`

---

## Phase 6: Feature Registry & Wrap-up

### Epic 6.1: Feature registry entries
**Goal**: Per `.claude/rules/feature-registry.md`, every new UI component gets a
per-feature JSON file; no new RPC exists in this plan so no backend entries are
required.

#### Story 6.1.1: Add frontend registry entries for the badge, grouping strategy, and rule-builder field
**Acceptance Criteria**:
- `make registry-generate` runs clean with no new coverage gaps introduced.
  - *Given* the three new registry JSON files exist and each references a `testIds`
    entry from Phases 2/3/5's test files, *When* `make registry-generate` runs, *Then*
    `docs/registry/coverage-gaps.json`'s count does not increase relative to `main`.
**Files**: `docs/registry/features/frontend/stale-session-indicator.json` (new), `docs/registry/features/frontend/stale-grouping-strategy.json` (new), `docs/registry/features/frontend/approval-rule-min-session-idle-minutes.json` (new)

##### Task 6.1.1a: Create the three new per-feature JSON files (~5 min)
- Follow the schema shown in `docs/registry/features/frontend/approval-analytics-reason-breakdown.json`
  (id, type, name, component, path/filePath, tested, testIds, lastModified). Reference
  the actual test names written in Tasks 2.2.1e, 3.1.1d, and 5.5.1d.
- Files: `docs/registry/features/frontend/stale-session-indicator.json`, `docs/registry/features/frontend/stale-grouping-strategy.json`, `docs/registry/features/frontend/approval-rule-min-session-idle-minutes.json`

##### Task 6.1.1a2: Update existing backend registry entries for the 3 modified RPCs (~3 min)
- This plan modifies, not just adds, `GetSessionDefaults`/`UpdateGlobalDefaults` (Epic
  1.2: two new fields each) and `UpsertApprovalRule`/`ListApprovalRules` (Epic 5.2: one
  new field). Per `.claude/rules/feature-registry.md`'s "Existing RPC method" case, find
  their existing entries under `docs/registry/features/backend/` and update `tested`,
  `testIds` (add Task 1.2.1e's and Task 5.2.1e's new test names), and `lastModified` —
  do not skip these on the assumption only new UI needs registry entries.
- Files: `docs/registry/features/backend/*.json` (existing files for the 3 RPCs above)

##### Task 6.1.1b: Run `make registry-generate` and commit the regenerated aggregates (~2 min)
- Verify `docs/registry/coverage-gaps.json`'s count against `main` per the acceptance
  criterion.
- Files: `docs/registry/backend-features.json`, `docs/registry/frontend-features.json`, `docs/registry/coverage-gaps.json` (all generated)

##### Task 6.1.1c: Final full-suite verification (~5 min, not a code task)
- `make build && make test` (backend), `cd web-app && npx jest --no-coverage` (frontend),
  `make lint`. All green before this plan is considered shipped.
