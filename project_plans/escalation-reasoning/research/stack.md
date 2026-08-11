# Research: Stack, Versions, and Existing Patterns

## No new dependencies

Confirmed: this feature is pure plumbing (new struct field + proto field +
existing UI primitives). No new Go module, npm package, or proto plugin is
needed. Matches AC6 ("no new UI dependency") and the repo's general
anti-dependency bias.

Relevant existing versions (from `go.mod` / `web-app/package.json`):
- Go `1.26.3`
- `connectrpc.com/connect v1.19.0`, `google.golang.org/protobuf v1.36.11`
- `buf.gen.yaml` (v2 config): remote plugins `buf.build/protocolbuffers/go` and
  `buf.build/connectrpc/go` generate into `gen/proto/go`; a TS plugin
  (`protoc-gen-es`, invoked via `web-app/node_modules/.bin/protoc-gen-es`)
  generates `web-app/src/gen/session/v1/*_pb.ts`
- `make proto-gen` (`Makefile`) is gated on a stamp file (`$(PROTO_STAMP)`)
  compared against `find proto -name '*.proto' -newer ...` — touching any
  `.proto` file and re-running `make proto-gen` regenerates both Go and TS
  bindings in one step
- Frontend: React `19.0.0`, `@vanilla-extract/css ^1.20.1`,
  `@vanilla-extract/recipes ^0.5.7`, `@bufbuild/protobuf ^2.11.0`,
  TypeScript `^5.9.3`
- Test tooling: Jest `^30.2.0`, `ts-jest ^29.4.11`, `@playwright/test ^1.57.0`

## (a) Go struct field addition + JSON persistence backward-compat

Pattern already in use, no versioning scheme needed. `PersistedApproval`
(`server/services/approval_store.go:42-53`) round-trips via plain
`encoding/json` (`json.MarshalIndent` in `persistToDiskLocked` at line 291,
`json.Unmarshal` in `loadFromDisk` at line 342). There is **no schema-version
field** anywhere in this file or its sibling persistence structs — backward
compat is achieved for free by Go's `encoding/json` semantics: a field absent
from an on-disk JSON blob unmarshals to its zero value (`""` for a new
`string` field), and unknown fields in old JSON are silently ignored on
struct evolution. This is the idiomatic path for adding
`EscalationReason string \`json:"escalation_reason,omitempty"\`` to both
`PendingApproval` and `PersistedApproval` — no migration code, no version
bump, just add the field and the two literal-construction call sites
(`persistToDiskLocked` line ~298, and wherever `loadFromDisk` reconstructs
`PendingApproval` from `PersistedApproval`, ~line 367+).

Use `omitempty` on the new JSON tag to keep old-format persisted files
diffing cleanly against new ones in tests, consistent with existing optional
fields elsewhere in the codebase (e.g. `AnalyticsEntry.RuleID
\`json:"rule_id,omitempty"\`` at `server/services/analytics_store.go:27`).

## (b) Proto field addition + existing enum-like category pattern

**No proto change needed for `ReviewItem`** (per requirements — metadata is
already `map<string,string>`; add key `"escalation_reason"` there, following
the existing convention of `tool_input_command`, `cwd`, `orphaned`, etc. at
`session/review_queue_poller.go:807-829` / consumed in
`web-app/src/components/sessions/ReviewQueuePanel.tsx:726-743`).

**A proto change IS needed for `AnalyticsSummaryProto`**
(`proto/session/v1/types.proto:1108-1134`) for AC4's breakdown. The existing
pattern for a categorical breakdown in this exact message is:
```protobuf
map<string, int32> decision_counts = 2;          // plain string-keyed map
repeated RuleStatProto top_triggered_rules = 5;   // string id + count, when more than just a count is needed
```
and mirrored Go-side in `AnalyticsSummary` / `ComputeSummary`
(`server/services/analytics_store.go:317-...`, `summaryToProto` at
`server/services/rules_service.go:515-...`). There is **no proto `enum`**
used anywhere for classification categories — `RuleStatProto.rule_id`,
`ProgramStatProto.category`, `AnalyticsEntry.Decision`/`RiskLevel`, and the
ent-persisted `ClassificationAnalytics.decision`/`risk_level`/`rule_id`
fields (`session/ent/schema/classificationanalytics.go:24-46`) are all plain
`string`. The one proto `enum` in this codebase used as a category
(`SessionType` at `proto/session/v1/types.proto:354-366`) is for a small,
closed, UI-driven set with dedicated frontend dropdown wiring (see
`.claude/rules/session-creation-registry.md`) — a much heavier pattern than
this feature needs.

**Recommendation**: follow the `decision_counts` precedent exactly — add
`map<string, int32> escalation_reason_counts = 17;` (next free field number)
to `AnalyticsSummaryProto`, and a plain `string` field (not a Go iota-typed
enum, unlike `classifier.ClassificationDecision`/`RiskLevel` which are
internal-only int enums per `pkg/classifier/classifier.go:17-35`) on
whatever Go struct carries the reason end-to-end. The taxonomy's 5 category
strings (`no-match`, `explicit-rule`, `domain-age`, `secret-scan`,
`unclassifiable`) should be derived at write-time from `RuleID`/`Decision`
exactly as `ReclassifyGaps`/`ComputeSummary` already derive `decision`
buckets — no new type needed, just a small categorization function
(candidate location: `server/services/analytics_store.go`, alongside
`decisionString`).

After editing `proto/session/v1/types.proto`, run `make proto-gen` (requires
`buf` toolchain configured via `buf.gen.yaml`); this regenerates
`gen/proto/go/session/v1/*.pb.go` and
`web-app/src/gen/session/v1/*_pb.ts` together.

## (c) React / vanilla-extract patterns for conditional badge/text + button intent

**AC6 (plain-text reason)**: reuse the exact `itemContext` pattern already in
`ReviewQueuePanel.tsx:718-720`:
```tsx
{queueItem.context && !queueItem.metadata?.["pending_approval_id"] && (
  <p className={itemContext}>{queueItem.context}</p>
)}
```
`itemContext` is a vanilla-extract class imported from
`ReviewQueuePanel.css.ts` (import list at line ~40). The approval-metadata
block just below it (lines 726-743) is the more directly reusable template —
conditionally rendering fields from `queueItem.metadata[...]` inside the
`pending_approval_id` branch, e.g. the `cwd` detail row (733-738) or the
`orphaned` badge (739-741). The new `escalation_reason` value should render
the same way: a conditional block keyed on
`queueItem.metadata?.["escalation_reason"]`, styled with `itemContext` (per
AC6, "no new UI dependency" — reuse the existing class, don't add a new
one).

**AC7 (button intent)**: `Button` component
(`web-app/src/components/ui/Button.tsx` + `Button.css.ts`) is a
`@vanilla-extract/recipes` `recipe()` with a closed `intent` variant set —
`primary | secondary | danger | ghost` (`Button.css.ts:33-72`), each pulling
colors from the shared `vars` theme contract (`@/styles/theme.css`), per
`.claude/rules/css-architecture.md`. The exact target is the "Create Rule"
button at `ReviewQueuePanel.tsx:818-838`, currently `intent="ghost"`
(line 820). AC7 requires switching this to `intent="secondary"` specifically
for no-match escalations (`RuleID==""` + `Decision==Escalate`) — this needs a
conditional expression on `intent`, e.g.
`intent={isNoMatchEscalation ? "secondary" : "ghost"}`, computed from the new
`escalation_reason` metadata value (or from `rule_id` metadata already
present as `queueItem.metadata?.["tool_name"]`'s sibling — confirm exact key
name during planning). No `intent="primary"` usage is acceptable here per
AC7 (reserved for the Approve button, line 786-787).

## Other touchpoints confirmed by direct read (for planning-phase precision)

- `classifier.ClassificationResult` (`pkg/classifier/classifier.go:39-47`):
  `Decision ClassificationDecision`, `RiskLevel RiskLevel`, `Reason string`,
  `Alternative string`, `RuleID string`, `RuleName string`, `Source string`.
  `ClassificationDecision` and `RiskLevel` are Go `int`/`iota` enums
  (`classifier.go:17-35`), internal-only — never serialized as proto/JSON
  ints; `analyticsDataToEntry`/`decisionString` convert them to strings at
  the boundary.
- `session.ApprovalMetadata` (`session/review_queue_poller.go:55-61`): fields
  `ApprovalID, ToolName, ToolInput, Cwd, Orphaned` — needs a new
  `EscalationReason string` field. Sole construction site is
  `server/services/approval_store.go:144` (`GetApprovalMetadataBySession`),
  which will need to read the new `PendingApproval.EscalationReason` field.
- `PendingApproval`/`PersistedApproval` construction sites: created at
  `server/services/approval_handler.go:358-369` (`createApproval:` label) —
  this is where the function-scoped escalation-reason value (from either the
  domain-age `reason` at line 248/261, or classifier `result.Reason`/`RuleID`
  from the scoped block at lines 280-313) must be threaded in; both are
  currently out of scope at the `createApproval:` label per the
  requirements doc, confirming the plan-phase task of hoisting a
  function-scoped `var escalationReason string` above the `if
  h.classifier != nil` block.
