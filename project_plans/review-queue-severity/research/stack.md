# Research: Technology Stack — Review Queue Severity

## Headline finding: the requirements doc names the wrong frontend file

Requirements.md's problem statement says the queue is "surfaced in
`ReviewQueuePanel.tsx`, backed by `PendingApproval` / `ListPendingApprovals`."
That is incorrect — verified by reading both files:

- `web-app/src/components/sessions/ReviewQueuePanel.tsx` imports
  `Priority, AttentionReason, ReviewItem, ... from "@/gen/session/v1/types_pb"`
  — this is the **session-attention triage queue** (`session/queue/queue.go`'s
  `Priority`/`ReviewItem`, driven by session state: idle/error/approval-pending/etc).
  Requirements.md's own audit item #8 explicitly names this as a *distinct*
  concept from per-approval risk and says "must not be confused or merged."
  Yet the problem statement's own file reference points at exactly this file.
- The component that actually renders `PendingApproval`/`ListPendingApprovals`
  data (`PlainApproval`, sourced from `web-app/src/lib/api/approvalsApi.ts`)
  is **`ApprovalDrawer.tsx`** (list container, currently sorts
  `[...approvals].sort((a, b) => a.secondsRemaining - b.secondsRemaining)` at
  approvalsApi.ts:64), **`ApprovalPanel.tsx`** (a second list container used
  elsewhere), and **`ApprovalCard.tsx`** (the per-item card, takes a single
  `PlainApproval` prop, currently renders tool name / countdown / detail
  fields / approve-deny buttons — no severity concept at all today).

**Action for planning**: point every UI acceptance criterion (badge, sort,
filter) at `ApprovalCard.tsx` + `ApprovalDrawer.tsx` (+ `ApprovalPanel.tsx` if
it has its own independent list-rendering, not just delegating to
ApprovalDrawer — verify in planning) instead of `ReviewQueuePanel.tsx`. If
`ReviewQueuePanel.tsx` genuinely needs no changes for this feature, plan.md
should say so explicitly rather than silently dropping the reference.

## 1. Proto field addition + regen process

`PendingApprovalProto` (`proto/session/v1/types.proto:1034-1060`) is missing:
- `risk_level` (the actual scope item)
- also missing `escalation_reason`/`escalation_category`, which the Go
  `PendingApproval` struct already carries (`server/services/approval_store.go:36-37`)
  but which were apparently never wired to the wire type either — worth a
  scope-note in plan.md since acceptance criteria only mention severity, but
  the same "dropped on the floor" gap exists for these two fields.

Pattern to follow: `ApprovalRuleProto.risk_level` is `string risk_level = 8;`
(types.proto:1084) — **string on the wire**, not an int/enum. The codebase
already has canonical bidirectional string↔`classifier.RiskLevel` conversion
helpers to reuse rather than reinvent:
- `riskLevelString(r classifier.RiskLevel) string` — `server/services/analytics_store.go:574`
- `parseRiskLevel(s string) classifier.RiskLevel` — `server/services/rules_store.go:351`
- (also `riskLevelToInt`/`riskLevelStringFromInt` for the ent int-column path — not needed here since `PendingApproval` has no ent schema; see §2)

Add `string risk_level = 10;` to `PendingApprovalProto` (next free field number
after `seconds_remaining = 9`).

**Regen command**: `make proto-gen` (Makefile:398-413). This is a
staleness-guarded target — it only runs `buf generate proto` if any `proto/*.proto`
file is newer than `.proto-gen.stamp`, or the TS/Go generated output is
missing. Editing `types.proto` and running `make proto-gen` (or just `make build`,
which depends on it) is sufficient; no manual `buf` invocation needed.
Output paths per `buf.gen.yaml`:
- Go message + ConnectRPC bindings → `gen/proto/go/session/v1/*.pb.go` (via `buf.build/protocolbuffers/go` and `buf.build/connectrpc/go` remote plugins)
- TypeScript bindings → `web-app/src/gen/session/v1/*_pb.ts` (via local `protoc-gen-es`, requires `web-app/node_modules/.bin/protoc-gen-es` to exist — `proto-gen` target depends on `web-app/node_modules/.modules.yaml` to ensure this)

Also run `make proto-lint` (`buf lint proto`) and `make proto-build` (`buf build proto`)
before committing — both are cheap validation steps with no code generation.

**Known gotcha (from prior-session memory, not re-verified here)**:
`web-app/src/gen/` is git-tracked despite matching a `.gitignore` pattern —
regenerated files must be explicitly `git add`ed, they won't show as new
untracked files needing no special handling. Also: `buf-setup-action` can hit
GitHub API rate limits in CI — irrelevant to local `make proto-gen` but worth
knowing if CI fails on this PR for an unrelated-looking reason.

Also extend `GetApprovalAnalyticsResponse` (session.proto:1429-1433, wraps
`AnalyticsSummaryProto` at types.proto:1111-1136) with a severity breakdown.
`AnalyticsSummaryProto` already has a precedent field shape for exactly this —
`repeated RuleStatProto top_triggered_rules` — a repeated stat-proto is the
established pattern in this file (`ToolStatProto`, `CommandStatProto`,
`RuleStatProto`, `ProgramStatProto`, `ImportStatProto`, `SubcommandStatProto`
all follow this convention). Add something like:
```protobuf
message RiskLevelStatProto {
  string risk_level = 1;
  int32 count = 2;
}
```
and `repeated RiskLevelStatProto risk_level_breakdown = 17;` (next free field
number) to `AnalyticsSummaryProto`.

## 2. ent ORM changes: none needed for `PendingApproval`

`PendingApproval` (`server/services/approval_store.go:21-46`) and its disk
counterpart `PersistedApproval` (`:49-62`) are **plain Go structs with manual
JSON persistence** — confirmed by reading `approval_store.go`: no `ent.Schema`
file exists for it (`session/ent/schema/` only has `approvalrule.go` and
`classificationanalytics.go` with `risk_level` fields — both unrelated ent
entities, not `PendingApproval`). Persistence is a hand-rolled JSON file
(`pending_approvals.json`, gated by `filePath` in `ApprovalStore`), written/read
via the store's own methods — no ent generate step, no migration.

Required changes are therefore pure Go struct field additions, not schema
changes:
- `PendingApproval.RiskLevel classifier.RiskLevel` (or `string`, to match how
  `EscalationReason`/`EscalationCategory` are stored — those are plain
  strings on the struct, set once at creation and never re-derived per the
  existing doc comment at approval_store.go:32-35)
- `PersistedApproval.RiskLevel string `json:"risk_level,omitempty"`` — mirrors
  the `omitempty` treatment already used for `EscalationReason`/`EscalationCategory`
  (approval_store.go:59-60), satisfying requirement #7 (severity survives
  restart) with no new storage engine.

`ClassificationAnalytics` (ent schema, `risk_level` is `field.String()` at
`session/ent/schema/classificationanalytics.go:32`, indexed at `:70`) already
persists `RiskLevel` per recorded decision via `AnalyticsStore.RecordFromResult`
— confirmed this is genuinely just an aggregation query away for the
analytics breakdown (req #5), no new ent field/migration required there either.
`ApprovalRule.risk_level` is `field.Int()` (approvalrule.go:35, note: **int**,
not string — inconsistent with `ClassificationAnalytics`'s string column;
existing `riskLevelToInt`/`riskLevelStringFromInt` helpers in rules_store.go
already paper over this inconsistency for that call site — not something this
feature needs to touch or fix).

## 3. Frontend libraries/patterns already in web-app

**No dedicated badge/color-coding library** — badges are hand-rolled
`vanilla-extract` (`.css.ts`) classes applied via `className`, consistent with
`.claude/rules/css-architecture.md`'s repo-wide convention (all new component
styles go in a colocated `.css.ts`, tokens referenced as `vars.xxx`, never
`var('--string')`).

Existing color-coded status token groups usable for severity, all defined in
`web-app/src/styles/theme-contract.css.ts`:
- `vars.color.{success,successBg,successText}` (green tier)
- `vars.color.{warning,warningBg,warningText}` (amber tier)
- `vars.color.{error,errorBg,errorText,errorDark}` (red tier)
- `vars.statusBadge.{approvalBg/Fg/Border, inputBg/Fg/Border, completeBg/Fg/Border, uncommittedBg/Fg/Border, idleBg/Fg/Border, staleFg, processingBg/Fg/Border}` — a purpose-built badge token namespace already used for session status badges

**Gap**: only 3 semantic color tiers exist (success/warning/error) but
`classifier.RiskLevel` has 4 values (Low/Medium/High/Critical). No existing
"critical" tier distinct from "error" — `ApprovalCard.css.ts`'s
`countdownUrgent` already claims `vars.color.errorBg`/`errorText` for expiry
urgency, so reusing the same token for "Critical risk" risks visual overload
(two different concerns — time pressure vs. danger — sharing one red).
**Open question for plan.md**: add a new `vars.color.critical*` token triplet
to `theme-contract.css.ts` + `theme.css.ts` (per the CSS rule: "If you need a
token that doesn't exist yet, add it to `globals.css`/theme first, then
reference it"), or reuse `errorDark` for Critical and `error` for High.

**Existing precedent for a per-item colored badge tied to a `.css.ts` file**:
`ApprovalCard.css.ts`'s `countdownNormal`/`countdownWarning`/`countdownUrgent`
(3-tier severity-style badge already keyed off elapsed time, ApprovalCard.css.ts:83-99)
is the closest existing pattern to copy for a 4-tier risk badge — same file,
same component family.

**Sorting**: no external sort library; `ApprovalDrawer.tsx` already does
plain-array `.sort()` client-side (approvalsApi.ts / ApprovalDrawer.tsx:64).
Adding severity as primary/secondary sort key is a comparator change, no new
dependency. (`ReviewQueuePanel.tsx` — the *other*, unrelated panel — has a
richer `SortField`/`sortDirection`/URL-persisted-filter apparatus
(`useFilterState` hook, `FILTER_URL_KEYS`) that could be a structural model to
imitate for approval filtering, without literally reusing that file.)

**Filtering**: `ReviewQueuePanel.tsx` demonstrates the repo's established
filter-UI pattern (Set-based multi-select filters, `filterButton`/`filterButtonActive`
CSS states, URL-persisted via `useFilterState`) — a reasonable structural
reference for a severity filter control even though the underlying data model
(`ReviewItem`/`Priority`) is unrelated. No new UI library needed.

**No existing severity/risk badge component anywhere in the frontend today** —
`ApprovalRulesPanel.tsx` and `RuleBuilderForm.tsx` both handle `riskLevel` as
plain string state for a `<select>` form control (RuleBuilderForm.tsx:113,537)
with no colored badge rendering; `SuggestedRuleCard.tsx` passes `riskLevel`
through as an opaque field. This is genuinely new UI, not a wire-up of an
existing component.

## 4. Analytics/dashboard charting

**No charting library is used for approval analytics today, despite one being
installed.** `web-app/package.json` lists `"recharts": "^3.8.1"` as a
dependency, but it's used only in `web-app/src/app/insights/{ModelBreakdownChart,DailySpendChart,ModelOverTimeChart}.tsx`
— an unrelated "insights" (LLM cost) feature.

`ApprovalAnalyticsPanel.tsx` (which renders `GetApprovalAnalyticsResponse`)
explicitly avoids recharts — see its own comment: `// Simple inline bar
component — no charting library required.` (ApprovalAnalyticsPanel.tsx:59).
It hand-rolls two small presentational components:
- `Bar({ value, max, className })` — single-series bar, width scaled to `%` via inline `style`
- `StackedBar({ allow, deny, manual, total })` — 3-segment stacked bar (allow/deny/manual composition)

**Recommendation for the severity breakdown**: follow this file's own
established convention (`Bar`/`StackedBar` + vanilla-extract classes) rather
than introducing `recharts` into this panel — reusing `Bar` directly per risk
level, or adding a `StackedBar`-style 4-segment variant if a single combined
bar is wanted. Pulling in `recharts` here would be inconsistent with this
panel's explicit design choice and templates from a different feature area.

## Summary of touchpoints (supersedes/corrects requirements.md's file list)

| Layer | File | Change |
|---|---|---|
| Proto | `proto/session/v1/types.proto` | Add `risk_level` (string) to `PendingApprovalProto`; add `RiskLevelStatProto` + repeated field to `AnalyticsSummaryProto` |
| Proto gen | `make proto-gen` | Regenerates `gen/proto/go/session/v1/*.pb.go` + `web-app/src/gen/session/v1/*_pb.ts`; also run `make proto-lint`/`proto-build` |
| Go struct | `server/services/approval_store.go` | Add `RiskLevel` to `PendingApproval` + `PersistedApproval` (json `omitempty`); no ent schema involved |
| Go handler | `server/services/approval_handler.go` (~384-437) | Copy `escalation.RiskLevel` onto the new `PendingApproval` field (currently only `EscalationReason`/`EscalationCategory` copied) |
| Go conversion | reuse `riskLevelString`/`parseRiskLevel` (`analytics_store.go:574`, `rules_store.go:351`) | Avoid re-deriving string↔enum mapping |
| Go analytics | `server/services/analytics_store.go` | Aggregate existing `ClassificationAnalytics.RiskLevel` (ent, already stored) into the new breakdown field — no new ent field |
| Frontend types | `web-app/src/lib/api/approvalsApi.ts` | Add `riskLevel: string` to `PlainApproval` |
| Frontend UI (badge/sort) | `web-app/src/components/sessions/ApprovalCard.tsx` + `.css.ts`, `ApprovalDrawer.tsx` (+ `ApprovalPanel.tsx` if independent) | **Not** `ReviewQueuePanel.tsx` — see headline finding |
| Frontend UI (filter) | same as above; `ReviewQueuePanel.tsx`'s Set-filter pattern is a structural reference only | New severity filter control |
| Frontend theme | `web-app/src/styles/theme-contract.css.ts` + `theme.css.ts` | Possibly add a 4th "critical" color tier — open question |
| Frontend analytics | `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx` | Reuse existing hand-rolled `Bar`/`StackedBar` pattern; do not introduce `recharts` here |
