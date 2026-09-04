# Architecture Research: stale-session-detection

**Date**: 2026-08-06
**Agent**: 3 (Architecture)

## Prior analysis incorporated (read first, cited by file:line below rather than re-derived)

- `project_plans/review-gate-stale-session-rework/research/architecture.md` — full analysis of
  the two pre-existing staleness subsystems (Review Queue badge, durable `StuckReasonStaleWork`)
  plus the third one-shot notifier (`notifyIfActiveWorkSessionStale`). This document only covers
  what that one didn't: the fourth (SessionCard) consumer, config threading, the approval-rule
  condition, and the grouping strategy.
- **That project has since shipped** (confirmed live against `main`, not assumed from its own
  docs): `session/review_queue_poller.go:49` now reads 5 min (reverted from 2 min, see
  `project_plans/review-gate-stale-session-rework/decisions/ADR-001-staleness-threshold-recalibration.md`),
  `server/services/backlog_service_triage.go:1030` now has a fourth named constant
  `maxReworkBlockStaleness = 15 * time.Minute`, and `session/domain/backlog.go:122`
  now has `StuckReasonReworkBlockedStale`. So the "genuinely uncovered case" that project's
  research identified is closed — this project inherits **four** existing thresholds, not two:

| Threshold | Value | File:line | Purpose |
|---|---|---|---|
| `ReviewQueuePollerConfig.StalenessThreshold` | 5 min | `session/review_queue_poller.go:49` | Review Queue "Stale" badge |
| `maxReworkBlockStaleness` | 15 min | `server/services/backlog_service_triage.go:1030` | Gates `AutoReopenAfterFailedReview`'s stall check |
| `maxWorkSessionStaleness` | 2 hr | `session/backlog_lifecycle.go:1874` (line number per prior doc; unchanged) | Durable `StuckReasonStaleWork` detector |
| *(none yet)* | — | — | **New**: SessionCard visual indicator (this project) |

  All four are hardcoded Go constants today — **zero** `config/` surface exists for any of them,
  confirming requirements.md's claim.

- `project_plans/review-queue-state-detection/` — has `implementation/plan.md` +
  `implementation/validation.md` + an ADR, i.e. it has reached at least a completed planning
  cycle. Its territory (working/idle/stuck classification accuracy) is orthogonal to staleness
  *duration* thresholds — it answers "is the session actively working" not "how long has it been
  silent" — so it doesn't change any of the answers below. Not deep-dived further per
  requirements.md's explicit out-of-scope note.
- `project_plans/review-queue-event-driven/` — has a full implementation cycle (ADR, plan,
  validation) targeting event-driven `LastMeaningfulOutput` updates in place of polling. Whether
  or not that's landed, it only changes *how* `LastMeaningfulOutput`/`LastTerminalUpdate` get
  updated, not their shape or where consumers read them from — irrelevant to this project's
  architecture, which only reads the resulting timestamp fields (per requirements.md's
  constraint to reuse them, not touch update mechanics).

## Is this a multi-actor business domain needing an Event-Command-Policy table?

**No — skip it**, per the skill's own guidance. This is single-actor (one user, self-hosted,
no auth per requirements.md's Constraints) and each of the four architecture questions below is
a local wiring/threading decision (where to read a timestamp, where to add a config field, where
to add a switch case), not a flow of domain events between actors/policies. The
review-gate-stale-session-rework project's own table (§ "Event-Command-Policy table" in its
architecture.md) was already a stretch for that narrower case ("not a multi-actor business
domain... a lightweight table still clarifies the flow" — its own words); this project has even
less of that shape, since three of its four items are pure "add a read/render path" changes with
no state-machine transitions at all.

---

## 1. SessionCard.tsx stale indicator — direct read vs. shared `IsStale(threshold)` helper

**Recommendation: shared helper, computed once per-session, consumed by all 4 (soon 5) sites —
not a direct inline read on the card.**

### Current state of the "compute staleness" logic — already duplicated 3x independently

- **Go, Review Queue**: `Instance.GetTimeSinceLastMeaningfulOutput()`
  (`session/instance_approval.go:112-119`) — fast-path atomic read, falls back to
  `Snapshot().TimeSinceLastMeaningfulOutput(CreatedAt)`. Consumed at
  `session/review_queue_determiner.go:259`, compared against
  `DefaultReviewQueuePollerConfig().StalenessThreshold`.
- **Go, rework-block gate**: `server/services/backlog_service_triage.go` (the function around
  line 1033-1050, doc-commented "meaningful output) and whether it currently exceeds
  maxReworkBlockStaleness") — a **second, independent** Go implementation of essentially the same
  "time since last meaningful output" computation, reached via a different path
  (`ItemSession.LastProgressAt`/instance lookup) than the Review Queue's.
- **Go, durable stuck detector**: `session/backlog_lifecycle.go`'s `reconcileStaleWorkSessions`
  uses `ItemSession.LastProgressAt` (falling back to `CreatedAt`) — a **third** independent
  computation, per the prior architecture doc's table.
- **TypeScript, frontend (existing, pre-this-project)**: `SessionCard.tsx:679-683` already
  computes `lastActivity` inline (`moSecs >= tuSecs ? lastMeaningfulOutput : lastTerminalUpdate`)
  — but only to pick a timestamp to *display* ("Active 3m ago"), not to compare against a
  threshold. This is the closest existing frontend logic and exactly the timestamp-selection half
  of what a stale check needs; it has no threshold-comparison counterpart today.

So the codebase already has three non-shared Go implementations of "time since last output" and
one frontend timestamp-selection helper. Adding a fourth (SessionCard's stale badge) as another
ad-hoc inline computation would be the fourth divergent implementation of the same signal —
directly the kind of duplication requirements.md's Success Metrics warn against ("No duplicate
threshold/detector is introduced where an existing one... already covers the same signal").

### Recommended shape

- **Frontend**: add one small pure helper, e.g. `web-app/src/lib/session-staleness.ts`
  (new file — no existing shared session-computation module fits; `web-app/src/lib/grouping/`
  is grouping-specific, `web-app/src/lib/omnibar/` is omnibar-specific), exporting:
  ```ts
  export function getLastActivityTimestamp(session: Session): Timestamp | undefined { ... }
  export function isSessionStale(session: Session, thresholdMinutes: number): boolean { ... }
  ```
  `getLastActivityTimestamp` is a direct extraction of the existing inline IIFE at
  `SessionCard.tsx:678-687` (the `moSecs >= tuSecs ? ... : ...` logic) — refactor that IIFE to
  call the new helper rather than duplicating the max-of-two-timestamps logic a second time
  in this project's own new code. `isSessionStale` layers a threshold comparison on top.
  Both the new SessionCard badge and the new "Stale" grouping strategy (§4 below) should import
  this same helper — that is in fact the direct answer to requirements.md's Open Question #1
  ("reuse `ReasonStale`'s computation... or a third, distinct threshold"): the grouping strategy
  and the card badge are both new, sibling, frontend-only consumers and should share one
  frontend-side helper with each other regardless of what threshold value ends up chosen for
  each.
- **Backend**: no backend computation is needed for the SessionCard badge specifically — it's a
  pure frontend read of data already on the wire (`LastMeaningfulOutput`/`LastTerminalUpdate`
  are already proto fields per requirements.md's plumbing note, and already flow to the frontend
  since `SessionCard.tsx` already reads them). The badge needs **only** a threshold value from
  config (§2) plus the shared frontend helper above — no new RPC, no new backend field.
- Do **not** attempt to unify the 3 existing *Go* implementations as part of this project — that
  would expand blast radius into `review-gate-stale-session-rework`'s and
  `review-queue-state-detection`'s already-shipped/in-flight territory, which requirements.md's
  Constraints explicitly rule out re-deriving. The new Go-side consumer this project *does* add
  (§3, the approval-rule condition) should reuse `Instance.GetTimeSinceLastMeaningfulOutput()`
  directly (the same call the Review Queue already uses) rather than either of the other two Go
  implementations or a new one — see §3's rationale for why that specific one is the right one to
  reuse.

---

## 2. Config-driven threshold(s)

### Existing config threading pattern (confirmed via 2 live examples)

Config values reach `session/` and `server/services/` consumers by **constructor-injected
`*cfg.Config` pointer**, read directly off nested struct fields at the point of use — not copied,
not passed as bare primitives. Two live examples:

- `session/hibernation_sweeper.go:222`: `idleTimeout := time.Duration(s.cfg.Hibernation.IdleTimeoutMinutes) * time.Minute`
  — the sweeper struct holds `cfg *config.Config` (constructor `NewSessionRetentionSweeper`,
  `session/session_retention_sweeper.go:40`, is the sibling pattern for a different sweep) and
  reads the nested field live at check-time, so a config reload is picked up without restart.
- `config/types.go:16-21`: `HibernationConfig.IdleTimeoutMinutes int` and
  `ResourcePressureThreshold int` — plain `int` (minutes / percent), JSON-tagged, with defaults
  set centrally in `config/config.go:459-460` inside whatever function builds the zero-value
  default config (not `DefaultXConfig()`-per-package the way `session/review_queue_poller.go`
  does it — config-package defaults and package-local hardcoded-constant defaults are two
  different, currently-coexisting default-setting patterns in this codebase; this project should
  follow the `config/types.go` one since it's introducing new config, not adjusting an existing
  package constant in place).

### Recommendation: multiple, independently-named config fields — not one shared threshold

Mirror the shape ADR-001 already established for the Go constants (three named thresholds, not
one shared value, specifically *because* the consumers have different false-positive tolerances).
Config should expose the same multiplicity, e.g. a new `StaleSessionConfig` struct in
`config/types.go`:

```go
type StaleSessionConfig struct {
    // CardThresholdMinutes controls the SessionCard visual "stale" badge in the
    // main session list. Default: TBD by Phase 3 (independent from the other two
    // per the multi-threshold precedent below).
    CardThresholdMinutes int `json:"card_threshold_minutes,omitempty"`
    // NotifyEnabled gates whether crossing CardThresholdMinutes also emits a
    // notification-bus event (requirements.md's stale_notify flag).
    NotifyEnabled *bool `json:"notify_enabled,omitempty"` // nil => default false; see EnabledOrDefault pattern below
}
```

Do **not** collapse this into the *existing* three Go constants' storage — those already have
names and call sites (`StalenessThreshold`, `maxReworkBlockStaleness`, `maxWorkSessionStaleness`)
tuned by ADR-001 with documented rationale; retrofitting them to read from config is a **separate,
optional** follow-on (only if Phase 3 decides the badge and one of the three existing consumers
should now share a value — requirements.md's Open Question #2 leaves this open). The one thing
this project's config *does* need to newly support is the **fourth** consumer (SessionCard badge)
plus whatever `session_age_minutes` approval-rule condition needs (§3) — those are the two
genuinely new consumers, so those are the two config surfaces to add. If Phase 3 decides the
approval-rule condition should default to `maxReworkBlockStaleness`-style reuse rather than a
free-form per-rule minutes value (rules already take an arbitrary int per-rule, see §3), no
additional config field is needed for it at all.

For the nil-pointer-means-default idiom, follow `config/types.go`'s existing
`SessionRetentionConfig.Enabled *bool` / `EnabledOrDefault()` pattern
(`config/types.go:37-51`) exactly — it's the established way this codebase distinguishes
"unset, use default" from "explicitly false" for a config field added after existing configs were
saved to disk (same forward-compat concern applies here: existing `config.json` files on disk
won't have this field).

### Exposing the threshold to the frontend

`IdleTimeoutMinutes`/`ResourcePressureThreshold` (the two existing `HibernationConfig` int
fields) are **not** exposed to the frontend at all today (confirmed: no proto message references
either field name) — so that pair is not a usable precedent for *frontend-visible* config. The
right precedent instead is `SessionDefaultsConfig` (`proto/session/v1/session.proto:1713-1731`),
which already has a `GetSessionDefaults`/`ResolveDefaults` RPC pair and — directly on point —
already has a "0 means use server default, response always echoes the resolved value" int field
(`max_auto_rework_iterations`, line 1727) that is exactly the shape a
`stale_session_threshold_minutes` field needs. `server/services/defaults_service.go:496`'s
`sessionDefaultsToProto(cfg *config.Config)` is the single conversion function to extend with
the new field — the frontend already fetches `SessionDefaultsConfig` via existing hooks (whatever
calls `GetSessionDefaults`), so adding a field there is the lowest-new-surface-area way to get the
threshold to `SessionCard.tsx`/`strategies.ts` without a bespoke new RPC. This directly answers
"how does config get from the Go config package to computation in the frontend": `config.Config`
→ `sessionDefaultsToProto` → existing `SessionDefaultsConfig` proto message → existing frontend
fetch hook → passed into the new `isSessionStale(session, thresholdMinutes)` helper (§1).

---

## 3. `session_age_minutes` condition on `ApprovalRuleProto`

### Existing pattern for adding a new rule condition — `require_ci_passing` is the freshest, most direct precedent

`require_ci_passing` (`proto/session/v1/types.proto:1084` region, field 29 — the most recently
added field, immediately after the field-15–19-reserved / structured-criteria block) is a
**boolean gate condition**, not a pattern-match field like `command_pattern`/`file_pattern`. Its
full wiring is the template to copy:

1. **Proto**: `bool require_ci_passing = 29;` on `ApprovalRuleProto`
   (`proto/session/v1/types.proto:1084` area). A `session_age_minutes` condition should follow the
   same shape but as `int32 session_age_minutes = 30;` (next available field number) — 0 meaning
   "condition not set / not applied," matching the same "0 = unset" idiom used elsewhere in this
   proto family (`max_auto_rework_iterations`, above).
2. **Rule struct**: `pkg/classifier/classifier.go:367` `RequireCIPassing bool` field on the
   in-package rule type (name TBD — check the struct this field lives on, same file, for the
   sibling name) — add `SessionAgeMinutes int32` alongside it.
3. **Match logic**: `pkg/classifier/classifier.go:735`:
   ```go
   if rule.RequireCIPassing && ctx.CIStatus != ciConclusionSuccess { ... }
   ```
   inside `Classify()` (`pkg/classifier/classifier.go:428`). A new condition follows the identical
   shape: `if rule.SessionAgeMinutes > 0 && ctx.SessionAgeMinutes < rule.SessionAgeMinutes { ...no match... }`
   (or `>=`/`<=` — exact comparison direction is a Phase 3 semantic decision: "deny if session
   is stale *longer than* N minutes" reads as `ctx.SessionAgeMinutes >= rule.SessionAgeMinutes`).
4. **Context field**: `ClassificationContext` (`pkg/classifier/classifier.go:61-76`) needs a new
   field, e.g. `SessionAgeMinutes int` (or a `time.Duration`), alongside the existing `CIStatus
   string`. `BuildContext()` (`pkg/classifier/classifier.go:652`) itself is the wrong place to
   populate it — it only takes a `cwd string` and has no session/instance handle. Instead, follow
   `CIStatus`'s own population pattern: it's set **after** `BuildContext()` returns, at the call
   site in `server/services/approval_handler.go:309-327`, which already has
   `h.liveFinder.FindLiveInstance(sessionID)` in hand (line 311) for the CI-status lookup. Add:
   ```go
   if inst := h.liveFinder.FindLiveInstance(sessionID); inst != nil {
       classCtx.SessionAgeMinutes = int(inst.GetTimeSinceLastMeaningfulOutput().Minutes())
       // ...existing ghInfo/CIStatus block reuses the same `inst`...
   }
   ```
   This directly resolves requirements.md's Open Question #3 ("time since session created" vs.
   "time since last output"): **use `Instance.GetTimeSinceLastMeaningfulOutput()`**
   (`session/instance_approval.go:112`) — the same call the Review Queue badge already uses — not
   session creation time. Reasons: (a) requirements.md's own default preference is last-output
   time "for consistency with the rest of the feature," (b) this is the *one* existing Go helper
   among the three duplicated implementations (§1) that is a clean, already-exported,
   already-instance-scoped single-call API — the other two live inline inside
   `backlog_service_triage.go`/`backlog_lifecycle.go` functions with more surrounding
   backlog-item-specific logic not relevant to a generic approval-rule context, and (c) it keeps
   the new condition consistent with the SessionCard badge and grouping strategy, which also read
   `LastMeaningfulOutput`/`LastTerminalUpdate` — a rule author configuring
   `session_age_minutes: 60` should mean the same "60 minutes since last output" a user sees
   flagged stale on the card, not a different clock.
5. **Storage/ent + web UI**: `require_ci_passing`'s footprint shows this is *not* free — a new
   bool/int field on `ApprovalRuleProto` cascades through `session/ent/schema/approvalrule.go`
   (new ent field), full ent codegen (`session/ent/approvalrule_*.go`, ~15 generated files touched
   per the `RequireCIPassing` grep above — regenerate via the mandatory
   `--feature sql/upsert` command per `.claude/rules/ent-schema-generation.md`), `session/repository.go:223`
   (`RequireCIPassing bool` on the plain Go repo struct), `server/services/rules_store.go:48`
   (JSON-persisted rule spec struct) and `rules_service.go` (proto↔domain conversion, 3 call
   sites per the grep: lines ~119, ~472, ~493, ~1222). Budget for this full chain, not just the
   proto+classifier piece — the ent/migration layer is the largest share of the diff by file
   count even though it's entirely generated/mechanical.
6. **Web UI**: `require_ci_passing` also has a UI toggle somewhere in the rules editor (not
   independently confirmed by this pass, but implied by its being an editable per-rule condition)
   — Phase 3/5 should locate and mirror wherever that checkbox lives for the analogous new
   numeric-minutes input.

---

## 4. "Stale" grouping strategy — client-side, no new backend support needed

`strategies.ts`'s existing `groupSessions()` switch (`web-app/src/lib/grouping/strategies.ts:69-142`)
dispatches purely on fields already present on the `Session` proto message passed in (Category,
Tag, Branch, Path, Program, Status, SessionType, Project, Workflow — all either direct proto
fields or derived from ones already on the wire, e.g. Path-derived project name). Every existing
strategy is a pure function of data the frontend already has in memory; none make a fresh RPC
call per grouping pass.

`session.lastMeaningfulOutput`/`session.lastTerminalUpdate` are **already on the wire** in the
same `Session` message (confirmed: `SessionCard.tsx:679-680` reads them directly off `session.*`
today). So a `GroupingStrategy.Stale = "stale"` case needs **zero new backend support** — it's a
new `case` in the existing switch, bucketing sessions into (at minimum) "Stale" / "Active" groups
via the same `isSessionStale(session, thresholdMinutes)` helper proposed in §1, using the
threshold sourced from `SessionDefaultsConfig` per §2 (grouping strategies are computed
client-side already; the config value just needs to be in the React component's props/context at
the point `groupSessions()` is called — check whichever component currently threads
`GroupingStrategy` through, e.g. the session-list page, for where to also thread the threshold
number).

One nuance: unlike the other 9 strategies (which are static per-session categorical properties),
staleness is **time-relative** — a session's group membership changes merely by clock time
passing, with no session data changing. If the grouped view is memoized on session data alone
(shallow-compare on the `Session[]` array), a session could silently sit in the wrong bucket
until some unrelated re-render. Phase 3/5 should check whatever memoization wraps `groupSessions()`
calls (likely a `useMemo` keyed on `[sessions, strategy]`) and confirm there's already a periodic
re-render tick in this view (there almost certainly is, since `SessionCard`'s "Active 3m ago"
relative-time display already requires one) — if so, no new mechanism is needed, the existing
re-render cadence will naturally recompute the Stale group; this is a "verify, don't assume" item
rather than a known gap.

---

## Integration points summary

| Area | Files |
|---|---|
| Shared frontend staleness helper (NEW) | `web-app/src/lib/session-staleness.ts` (new file) |
| SessionCard badge | `web-app/src/components/sessions/SessionCard.tsx:678-687` (refactor existing IIFE to use new helper; add badge render) |
| Grouping strategy | `web-app/src/lib/grouping/strategies.ts:8-30` (enum+labels), `:69-142` (switch case) |
| Config struct | `config/types.go` (new `StaleSessionConfig`), `config/config.go:459` region (defaults) |
| Config → frontend | `server/services/defaults_service.go:496` `sessionDefaultsToProto`, `proto/session/v1/session.proto:1713-1731` `SessionDefaultsConfig` |
| Approval rule proto | `proto/session/v1/types.proto:1084` region (new field 30) |
| Approval rule match logic | `pkg/classifier/classifier.go:367` (struct field), `:428` `Classify()`, `:735` (match check), `:61-76` `ClassificationContext` (new field) |
| Approval rule context population | `server/services/approval_handler.go:309-327` (alongside existing `CIStatus`/`liveFinder.FindLiveInstance` block) |
| Canonical staleness computation (Go) | `session/instance_approval.go:112` `GetTimeSinceLastMeaningfulOutput()` — reuse, don't reimplement |
| Ent/storage cascade (approval rule field) | `session/ent/schema/approvalrule.go` + full ent codegen, `session/repository.go:223`, `server/services/rules_store.go:48`, `server/services/rules_service.go` (~4 call sites) |
| Existing 3 duplicated Go staleness computations (do not touch, cited for awareness only) | `session/review_queue_determiner.go:259`, `server/services/backlog_service_triage.go:1033-1050`, `session/backlog_lifecycle.go` `reconcileStaleWorkSessions` |
