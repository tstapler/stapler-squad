# Stack Research: stale-session-detection

Agent 1 (Stack), SDD Phase 2. Scope: exact existing patterns for the 4 in-scope
additions, dependency versions, and codegen requirements.

## 1. Config-driven staleness threshold(s) — `config/`

Two established patterns coexist for numeric config fields; both use a
`FooOrDefault()` accessor rather than a `Validate()`/clamp-in-place function. New
fields should follow whichever shape matches the number of independent thresholds
decided in Phase 3.

**Pattern A — flat field + package-level default constant** (closest fit for a
single global threshold):
```go
// config/types.go:30-33
const defaultSessionRetentionDays = 14

// config/types.go:43-62 (SessionRetentionConfig)
RetentionDays int `json:"retention_days,omitempty"`

func (c SessionRetentionConfig) RetentionDaysOrDefault() int {
	if c.RetentionDays <= 0 {
		return defaultSessionRetentionDays
	}
	return c.RetentionDays
}
```

**Pattern B — default + hard ceiling clamp** (fits if the threshold needs a safety
cap, e.g. to stop someone setting it to 1 minute and spamming notifications):
```go
// config/config.go:608-625
const (
	maxConcurrentBacklogWorkItemsDefault     = 2
	maxConcurrentBacklogWorkItemsHardCeiling = 10
)

func (c *Config) MaxConcurrentBacklogWorkItemsOrDefault() int {
	if c == nil || c.MaxConcurrentBacklogWorkItems <= 0 {
		return maxConcurrentBacklogWorkItemsDefault
	}
	if c.MaxConcurrentBacklogWorkItems > maxConcurrentBacklogWorkItemsHardCeiling {
		return maxConcurrentBacklogWorkItemsHardCeiling
	}
	return c.MaxConcurrentBacklogWorkItems
}
```

A struct-grouped config (`HibernationConfig`, `SessionRetentionConfig`,
`TmuxExecGateConfig`, `CapacityConfig` — see `config/config.go:337-343`) is the
convention when a feature has >1 related knob (e.g.
`stale_session_threshold_minutes` + `stale_notify` together would warrant a
`StaleSessionConfig` struct with both fields, mirroring `HibernationConfig`'s
`Enabled`/`IdleTimeoutMinutes` pairing at `config/types.go:12-28`). Top-level
`Config` struct fields are declared `config/config.go:230-343`; add a new field
there (with `,omitempty`) and initialize its default in `DefaultConfig()`
(`config/config.go:391`).

**Consumers to wire (both currently hardcoded, per requirements.md's findings,
confirmed here):**
- `session/review_queue_poller.go:38,49` — `ReviewQueuePollerConfig.StalenessThreshold`,
  default `5 * time.Minute`. Constructed via `DefaultReviewQueuePollerConfig()`
  (`session/review_queue_poller.go:43`), called from exactly two places:
  `session/startup_scanner.go:21` and `session/review_queue_poller.go:141`
  (`NewReviewQueuePoller`'s convenience constructor). Neither call site currently
  threads a `*config.Config` through — confirmed via
  `grep -rn "NewReviewQueuePollerWithConfig\|DefaultReviewQueuePollerConfig()"`,
  only those three non-test hits exist. This is the plumbing gap Phase 3 needs to
  close if the review-queue badge should read the new config value.
- `session/backlog_lifecycle.go:2098` — `const maxWorkSessionStaleness = 2 * time.Hour`,
  a package-level `const`, not a struct field — cannot be made config-driven without
  turning it into a field read at call time (it's referenced at 6+ call sites per
  `grep -n maxWorkSessionStaleness`, including `reviewVerdictIdleThreshold` which
  currently aliases it at line 2108).

## 2. `session_age_minutes` approval-rule condition — proto + full touchpoint chain

This is **not just a proto change** — `RequireCiPassing` (the most recently added
condition of this kind, "matches on session/PR state rather than command
structure") is the reference implementation and touches 6 files end to end. Full
chain, confirmed by `grep -rn "RequireCiPassing\|RequireCIPassing"`:

1. **`proto/session/v1/types.proto:1108`** — `bool require_ci_passing = 29;` on
   `ApprovalRuleProto` (message starts line 1076). Next available field number for
   a new condition is **30**. Add e.g. `int32 session_age_minutes = 30;` (0/unset
   = condition not applied, matching the `bool` fields' "false = off" convention).
2. **`session/ent/schema/approvalrule.go:79-80`** — ent schema field:
   `field.Bool("require_ci_passing").Default(false)`. A new int condition would be
   `field.Int32("session_age_minutes").Optional().Default(0)`. **Requires ent
   codegen** (see §4).
3. **`session/repository.go:223`** — `RuleData.RequireCIPassing bool` (the
   in-process repository DTO).
4. **`session/ent_repository.go:1256,1314`** — ent row ↔ `RuleData` conversion
   (`RequireCIPassing: rule.RequireCiPassing` / `SetRequireCiPassing(...)`).
5. **`server/services/rules_store.go:48`** — `RuleSpec.RequireCIPassing bool
   \`json:"require_ci_passing,omitempty"\`` (the JSON-file-backed spec type used by
   `auto_approve_rules.json`).
6. **`server/services/rules_service.go:119,472,493,1222`** — bidirectional
   proto↔spec↔classifier.Rule conversion functions (`ruleToProto`-style /
   `ruleToSpec`-style — 4 separate conversion sites, all must be updated together).
7. **`pkg/classifier/classifier.go`**:
   - `ClassificationContext` struct (line 61-76) — currently has `CIStatus string`
     (line 75, populated per-request from live GitHub check state). A
     `session_age_minutes`-based condition needs a comparable context field, e.g.
     `SessionAgeMinutes int` or `LastMeaningfulOutputAt time.Time` — **the Phase-3
     "creation time vs. last-output time" open question (requirements.md line
     147-151) determines which**, since `ClassificationContext` doesn't currently
     carry either timestamp.
   - `Rule` struct (line 346-374) — add `SessionAgeMinutesGT int` or similar next
     to `RequireCIPassing bool` (line 367).
   - `matchesRule` (line 688-740) — add the AND-condition check, mirroring
     `if rule.RequireCIPassing && ctx.CIStatus != ciConclusionSuccess { return false }`
     at line 735.
   - Context population: `RequireCIPassing`'s context value is populated at
     **`server/services/approval_handler.go:317,323`** (`classCtx.CIStatus =
     ghInfo.GitHubCheckConclusion`), not inside `classifier.BuildContext` — the
     new field needs an equivalent assignment in `ApprovalHandler`, sourced from
     `ReviewState.LastMeaningfulOutput`/`LastTerminalUpdate` per the requirements'
     constraint to reuse that existing signal (`session/review_state.go:50-76`).

Test pattern to mirror: `pkg/classifier/classifier_test.go:3271-3360`
(`TestClassify_RequireCIPassing_*`, 4 tests covering success/failure/no-PR/AND-combination).

## 3. "Stale" grouping strategy — `web-app/src/lib/grouping/strategies.ts`

Two-part addition, both required:
- `GroupingStrategy` enum (lines 9-19) — add e.g. `Stale = "stale"` as a new
  member.
- `GroupingStrategyLabels` (lines 21-31) — add the matching display label, e.g.
  `[GroupingStrategy.Stale]: "Stale"`.
- The `switch (strategy)` inside the group-computation function (lines 95-145,
  one `case` per existing strategy, e.g. `case GroupingStrategy.Status:` at line
  127-130 is the simplest single-membership precedent: `groupKeys =
  [getStatusDisplayName(session.status)];`). A Stale case would compute
  `groupKeys` from `session.lastMeaningfulOutput`/`lastTerminalUpdate` vs. the
  configured threshold (client needs the threshold value — likely via a config
  RPC read already used elsewhere, or embedded in the session proto/derived
  client-side).

No dedicated `.test.ts` file was located for `strategies.ts` in this pass —
Phase 3/4 should confirm whether one exists elsewhere before assuming test
coverage needs to be created from scratch.

## 4. Stale-session notification event — eventBus / NotificationService

**No new `NotificationType` enum value is needed.** Confirmed via
`grep -n "NOTIFICATION_TYPE_" proto/session/v1/types.proto:768-784` — the existing
`NOTIFICATION_TYPE_WARNING = 8` is exactly what the closest analog,
`notifyIfActiveWorkSessionStale` (`server/services/backlog_service_triage.go:1199-1240`),
already uses. That function is the direct template for the new "session in main
list went stale" event:

```go
// server/services/backlog_service_triage.go:1232-1239
s.eventBus.Publish(events.NewNotificationEvent(
	itemID, "", uuid.New().String(),
	int32(sessionv1.NotificationType_NOTIFICATION_TYPE_WARNING),
	int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM),
	"Rework blocked by a stale-but-alive session",
	fmt.Sprintf("..."),
	map[string]string{"item_id": itemID},
))
```

`NotificationService` (`server/services/notification_service.go:20-43`) itself
needs no changes — it's a thin RPC/broadcast wrapper around the same
`*events.EventBus` (`eventBus.Publish(event)`, line 117) that any caller,
including a new stale-detector, can reach if it's constructed with (or has
access to) the shared `*events.EventBus` instance. The new caller must gate the
publish on the new `stale_notify` config flag (§1) the way
`notifyIfActiveWorkSessionStale` gates on `s.sessionStopper == nil ||
s.eventBus == nil` (line 1200) — add a config-flag check alongside that nil
guard.

Frontend already consumes this event type end-to-end (`NotificationContext.tsx`,
`NotificationsNavBadge.tsx`, `NotificationPanel.tsx` per requirements.md) — no
frontend notification-plumbing changes needed, only the card-badge/grouping UI
(§3) is net-new frontend work.

## Dependency versions (go.mod / package.json)

- Go: `go 1.26.3` (go.mod:3)
- `connectrpc.com/connect v1.19.0`, `connectrpc.com/otelconnect v0.8.0`
- `entgo.io/ent v0.14.5`
- `google.golang.org/protobuf v1.36.11`
- Frontend: `react ^19.0.0`, `typescript ^5.9.3`, `@vanilla-extract/css ^1.20.1`,
  `@vanilla-extract/recipes ^0.5.7`, `@vanilla-extract/next-plugin ^2.5.1`

None of these versions block anything in scope — no upgrade needed for this
feature.

## Codegen requirements

**`make proto-gen` is required** (§2 adds a field to `ApprovalRuleProto`). Exact
command per `Makefile:398-413`: it's content-hash-gated (skips if `.proto-gen.stamp`
is newer than every `proto/**/*.proto` file and the generated Go/TS output already
exist), so just run `make proto-gen` — it internally invokes `buf generate proto`
and regenerates both `gen/proto/go/session/v1/*.go` and
`web-app/src/gen/session/v1/*_pb.ts`. Do not hand-invoke `buf generate` directly;
use the Makefile target so the stamp file stays in sync.

**ent codegen is required** if §2's ent schema field (`session_age_minutes`) is
added — confirmed the `ApprovalRule` ent entity exists at
`session/ent/schema/approvalrule.go` and mirrors the proto 1:1 (including
`require_ci_passing`, line 79-80). Per `session/ent/generate.go` and
`.claude/rules/ent-schema-generation.md`, the **exact required command** is:

```bash
go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema
```

`make ent-gen` (`Makefile:415-424`) wraps this identically (same flags) and is
stamp-gated like proto-gen — prefer `make ent-gen` (or just `make build`, which
depends on both `proto-gen` and `ent-gen`, `Makefile:133`) over a bare `go run`
invocation so the stamp file (`.ent-gen.stamp`) doesn't go stale. **Never** omit
`--feature sql/upsert` — doing so compiles but silently breaks `UpsertRule` and
similar upsert methods (this rule is already codified in the repo).

Sequence for §2's full chain: edit `proto/session/v1/types.proto` and
`session/ent/schema/approvalrule.go` → `make build` (regenerates both via its
dependency graph) → update the 4 hand-written Go conversion layers (§2 steps
3, 4, 5, 6) → update `pkg/classifier/classifier.go` (§2 step 7) → `go build ./...`
→ commit all `session/ent/` generated changes together with the schema edit, per
CLAUDE.md's ent-schema workflow note.

## Feature registry note

Per `.claude/rules/feature-registry.md` and requirements.md's constraint that
"new omnibar/session-creation touchpoints are not implicated," the only registry
obligation here is the standard one: any new/modified RPC (none of these 4 items
add a new RPC — the approval-rule condition and config threshold both ride on
existing `UpsertApprovalRule`/config-read RPCs) or new UI component (the Stale
grouping option, the SessionCard badge) needs a per-feature JSON file under
`docs/registry/features/frontend/`, then `make registry-generate`.
