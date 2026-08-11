# Architecture Research: Escalation Reasoning on Review Queue Items

All line numbers below are VERIFIED against the current worktree
(`backlog/stapler-squad-escalation-reasoning`) as of this research pass.

## 0. Prior SDD research consulted

- `project_plans/ai-rule-generation/research/architecture.md` — covers `GenerateSuggestedRule`
  / `SuggestedRuleProto`. Relevant precedent: proto messages for AI-facing rule suggestions
  live in `types.proto` alongside `ApprovalRuleProto`, and `RulesService` is the sole owner of
  `analyticsStore` + `classifier` for rule-suggestion purposes. **Not directly reused** here —
  this feature doesn't call `GenerateSuggestedRule`, it only reuses the *existing*
  `SuggestedRuleCard`/`createPortal` flow already wired to `tool_input_command` (AC3), which
  that doc's §7–§9 describes as unaffected by this change.
- `project_plans/review-queue-event-driven/research/architecture.md` — covers PTY→status-cache
  event plumbing and `StatusChangeListener`/idle-timeout wiring. **Not relevant** to this
  feature — escalation reasoning is populated synchronously inside `HandlePermissionRequest`
  and read out of `PendingApproval`/`ApprovalMetadata`/`ReviewItem.Metadata`, none of which
  touch the poller's event-driven status-change path. No overlap to build on.

## 1. Go data flow: reuse `classifier.ClassificationResult`, don't invent a new type

**Decision: reuse `classifier.ClassificationResult` directly**, hoisted to function scope in
`HandlePermissionRequest` (`server/services/approval_handler.go`). Do not introduce a
`classifier.EscalationReason` type — the domain-age branch already constructs a full
`ClassificationResult` literal (lines 251–257) purely to pass to
`analyticsStore.RecordFromResult`; that same value already carries `RuleID`/`RuleName`/`Reason`,
which is exactly what's needed downstream. Hoisting one variable and reusing it in both
branches avoids a second parallel struct with the same three fields (a "struct-wraps-struct"
smell per `.claude/rules/interface-pollution-checklist.md`).

### Hoist point

Declare **before** the domain-age check (before line ~237, so it's in scope both at the
`goto createApproval` on line 262 and at the `createApproval:` label on line 315 — Go's goto
rule only forbids jumping into a variable's scope, and `result` inside the classifier's
`if h.classifier != nil { ... }` block at line 280 is already out of scope by line 315, so this
hoist doesn't change that legality):

```go
// escalation carries the classifier/domain-check result that caused this request to
// reach manual review (RuleID/Reason/RuleName). Zero value ("") when the classifier is
// disabled and no domain-age hit fired — CategorizeEscalationRuleID("") still resolves
// to "no-match", which is accurate wording for that case too.
var escalation classifier.ClassificationResult
```

### Domain-age branch (current lines 246–263) — populate `escalation`, drop the discard

```go
if isNew {
    threshDays := int(h.domainChecker.NewDomainThreshold().Hours() / 24)
    reason := fmt.Sprintf("Domain %q was registered within the last %d days — possible phishing or supply-chain risk.", domain, threshDays)
    escalation = classifier.ClassificationResult{
        Decision:  classifier.Escalate,
        RiskLevel: classifier.RiskHigh,
        RuleID:    "new-domain-check",
        RuleName:  "New Domain Check",
        Reason:    reason,
    }
    log.ForSession(sessionID).Info("[ApprovalHandler] escalating — newly-registered domain", "tool", payload.ToolName, "domain", domain)
    if h.analyticsStore != nil {
        h.analyticsStore.RecordFromResult(payload, escalation, sessionID, "", 0)
    }
    goto createApproval // escalation now carries the reason; no more `_ = reason` discard
}
```

This removes the dead `_ = reason` at current line 261 — `reason` now lives inside `escalation.Reason`.

### Classifier-escalate branch (current lines 290–312) — add the missing explicit case

The requirements note this switch "only has a fallthrough comment with no explicit case" for
`Escalate`. Add one that captures `result` into the hoisted variable:

```go
switch result.Decision {
case classifier.AutoAllow:
    ... // unchanged
    return
case classifier.AutoDeny:
    ... // unchanged
    return
case classifier.Escalate:
    escalation = result // NEW — was previously silently dropped when the switch fell through
}
```

No `return` on the new case — execution must still fall through to `createApproval:` exactly as
before; the only change is that `result` is no longer thrown away.

### Threading through the persistence/metadata chain

New taxonomy helper (new file `pkg/classifier/escalation.go`, since categorization is a pure
function of `RuleID` and both `approval_handler.go` AC1/AC3 and `analytics_store.go` AC4 need
the *identical* branching — putting it in one place in the `classifier` package, which both
`server/services` files already import, avoids two independently-maintained copies drifting):

```go
package classifier

// EscalationCategory buckets an escalated (or secret-scan auto-denied) request into the
// reviewer-facing / analytics taxonomy from requirements.md.
type EscalationCategory string

const (
    EscalationNoMatch       EscalationCategory = "no-match"
    EscalationExplicitRule  EscalationCategory = "explicit-rule"
    EscalationDomainAge     EscalationCategory = "domain-age"
    EscalationSecretScan    EscalationCategory = "secret-scan"
    EscalationUnclassifiable EscalationCategory = "unclassifiable"
)

// CategorizeEscalationRuleID buckets a RuleID (from ClassificationResult.RuleID or a
// persisted AnalyticsEntry.RuleID) into the escalation-reason taxonomy. Only RuleID is
// needed — the taxonomy is fully determined by which rule (or lack thereof) fired.
func CategorizeEscalationRuleID(ruleID string) EscalationCategory {
    switch ruleID {
    case "":
        return EscalationNoMatch
    case "new-domain-check":
        return EscalationDomainAge
    case "secret-scan":
        return EscalationSecretScan
    case "shell-expansion-program":
        return EscalationUnclassifiable
    default:
        return EscalationExplicitRule
    }
}

// EscalationReasonText returns the plain-language reviewer-facing explanation. The
// classifier already populates Reason for explicit-rule, domain-age, and unclassifiable
// (see pkg/classifier/classifier.go:489,540 for the shell-expansion-program Reason text,
// and the "new-domain-check" literal built in approval_handler.go). Only no-match has no
// natural Reason (no rule fired), so it gets a synthesized fallback.
func EscalationReasonText(result ClassificationResult) string {
    if result.Reason != "" {
        return result.Reason
    }
    return "No approval rule matched this request — escalated to manual review by default."
}
```

New/changed Go field names (concrete, for the plan phase to implement verbatim):

| Type | File | New field | Go type | Notes |
|---|---|---|---|---|
| `PendingApproval` | `server/services/approval_store.go:21` | `EscalationReason` | `string` | plain-language (AC1) |
| | | `EscalationCategory` | `string` | one of the 5 taxonomy values, stored as `string` not `classifier.EscalationCategory` to avoid `services` importing a type alias it re-exports nowhere else — matches existing style (`PermissionMode string`, not a typed enum) |
| `PersistedApproval` | `server/services/approval_store.go:42` | `EscalationReason` | `string` `json:"escalation_reason,omitempty"` | |
| | | `EscalationCategory` | `string` `json:"escalation_category,omitempty"` | |
| `session.ApprovalMetadata` | `session/review_queue_poller.go:54` | `EscalationReason` | `string` | |
| | | `EscalationCategory` | `string` | |
| `ReviewItem.Metadata` map keys | `session/review_queue_poller.go:807–830` (poller enrichment block) | `metadata["escalation_reason"]` | — | AC1/AC6, plain string in the existing `map[string]string` |
| | | `metadata["escalation_reason_category"]` | — | AC3/AC7 gating key |

At `PendingApproval` construction (current lines 356–369 in `approval_handler.go`):

```go
approval := &PendingApproval{
    ID:                 approvalID,
    SessionID:          sessionID,
    ClaudeSessionID:    payload.SessionID,
    ToolName:           payload.ToolName,
    ToolInput:          payload.ToolInput,
    Cwd:                payload.Cwd,
    PermissionMode:     payload.PermissionMode,
    EscalationReason:   classifier.EscalationReasonText(escalation),
    EscalationCategory: string(classifier.CategorizeEscalationRuleID(escalation.RuleID)),
    CreatedAt:          time.Now(),
    ExpiresAt:          time.Now().Add(h.approvalTimeout()),
}
```

`ApprovalStore.GetApprovalMetadataBySession` (`approval_store.go:137–154`) — thread the two new
fields into the `session.ApprovalMetadata{...}` literal alongside `ApprovalID`/`ToolName`/etc.

Poller enrichment block (`review_queue_poller.go:811–828`) — add two `item.Metadata[...] = a....`
lines next to the existing `pending_approval_id`/`tool_name`/`cwd` assignments, gated the same
way (only when `a.EscalationReason != ""` / `a.EscalationCategory != ""`, matching the existing
`if a.Cwd != ""` / `if a.Orphaned` style already in that block).

**Secret-scan note (AC1 exclusion, confirmed):** the secret-scan branch (lines 207–233) returns
immediately via `h.writeDecision(w, "deny", msg)` before ever reaching `createApproval:` — it
never constructs a `PendingApproval`, confirming the requirement's "secret-scan excluded, never
creates a queue item." `EscalationSecretScan` is therefore only ever produced by the **analytics**
aggregation (§3 below) reading `AnalyticsEntry.RuleID == "secret-scan"` post-hoc from the
`RecordFromResult` call already at line 222–228 — never from `PendingApproval`/`ReviewItem`.

## 2. `approval_store.go` persist/load round-trip — confirmed non-issue for AC2

Read `server/services/approval_store.go` end to end (400 lines). The round trip is:

- `persistToDiskLocked()` (line 291) builds `[]PersistedApproval` from the in-memory
  `map[string]*PendingApproval` and does an atomic write (`.tmp` file + `os.Rename`).
- `loadFromDisk()` (line 342) does `json.Unmarshal(data, &persisted)` into `[]PersistedApproval`,
  then rebuilds `*PendingApproval` structs, forcing `Orphaned: true` on every one (line 382).

**Confirmed: no migration handling is needed for old on-disk JSON files lacking the two new
fields.** `encoding/json.Unmarshal` simply leaves a struct field at its Go zero value when the
corresponding JSON key is absent from the input — for `string` fields that's `""`. This is
already exactly how every other field added over time behaves; there is no versioned-schema
mechanism anywhere in this file (no `"version"` key, no migration function), and none is needed:
an old `pending_approvals.json` written before this change will simply produce
`PendingApproval{EscalationReason: "", EscalationCategory: ""}` for every entry loaded, which the
frontend/poller can treat identically to "not yet computed" (same pattern already used for
`Orphaned` defaulting false on missing key, before that field existed). No `omitempty`-vs-required
distinction matters here since these are additive, non-breaking fields on both read and write
paths — add them to `persistToDiskLocked`'s `PersistedApproval{...}` literal and `loadFromDisk`'s
`&PendingApproval{...}` literal exactly like every existing field, no special-casing required.

## 3. Analytics breakdown: `AnalyticsSummaryProto` / `ComputeSummary` pattern + AC4 time window

### Existing coverage-gap precedent (`server/services/analytics_store.go:395–406`, read in full — 630 lines)

`ComputeSummary` (line 317) loops over `[]AnalyticsEntry` once, incrementing per-category
counters into local `map[string]T` accumulators, then converts to sorted `[]XStat` via
`topNXxx` helpers at the end. The coverage-gap counters (`CoverageGapCount`,
`uncoveredToolCounts`, `uncoveredProgramStats`) are the closest precedent: a single `if` inside
the main loop (lines 396–406) that only fires for `Decision == "escalate" && RuleID == ""`.

### New aggregation logic (extends the same loop, same file)

Add one `map[string]int` accumulator and one `if` branch inside the existing loop at
`analytics_store.go:341` (`for _, e := range entries`):

```go
escalationReasonCounts := make(map[string]int) // category string -> count

// ... inside the loop, alongside the existing coverage-gap branch:
if e.Decision == "escalate" || (e.Decision == "auto_deny" && e.RuleID == "secret-scan") {
    cat := classifier.CategorizeEscalationRuleID(e.RuleID)
    escalationReasonCounts[string(cat)]++
}
```

This is deliberately **not** restricted to `RuleID == ""` (unlike the coverage-gap branch) —
it must also catch `explicit-rule`/`domain-age`/`unclassifiable` (all `Decision == "escalate"`
with non-empty `RuleID`) and the one `Decision == "auto_deny"` exception (`secret-scan`), per
the taxonomy in requirements.md.

New `AnalyticsSummary` field (Go struct, `analytics_store.go:88`):

```go
// EscalationReasonCounts breaks down escalated (and secret-scan auto-denied) decisions
// by the escalation-reason taxonomy: "no-match", "explicit-rule", "domain-age",
// "secret-scan", "unclassifiable". Keys are classifier.EscalationCategory string values.
EscalationReasonCounts map[string]int `json:"escalation_reason_counts"`
```

Initialize it in `ComputeSummary`'s zero-entries early return (line 318–323) the same way
`DecisionCounts` is initialized, and assign `summary.EscalationReasonCounts = escalationReasonCounts`
near the other `summary.TopXxx = topNXxx(...)` assignments (lines 409–416).

### Proto message shape (mirrors `RuleStatProto`/`ToolStatProto` precedent at `types.proto:1136–1154`)

Simplest-shape choice: reuse the **existing `map<string, int32>` pattern** already used for
`decision_counts` (`types.proto:1110`) rather than inventing a new `EscalationReasonStatProto`
message — there's no need for a repeated-message shape here since there is no secondary sort
axis (unlike `RuleStatProto`, which pairs a count with a human name resolved at runtime; the 5
category keys are a small fixed enum-like set the frontend already knows how to label).

```protobuf
// proto/session/v1/types.proto — add as field 17 (next available on AnalyticsSummaryProto)
message AnalyticsSummaryProto {
  // ... existing fields 1-16 unchanged ...

  // Breakdown of escalated (and secret-scan auto-denied) decisions by reason category:
  // "no-match" | "explicit-rule" | "domain-age" | "secret-scan" | "unclassifiable".
  map<string, int32> escalation_reason_counts = 17;
}
```

`summaryToProto` (`rules_service.go:515`) — add alongside the existing `p.DecisionCounts` loop:

```go
p.EscalationReasonCounts = make(map[string]int32, len(s.EscalationReasonCounts))
for k, v := range s.EscalationReasonCounts {
    p.EscalationReasonCounts[k] = int32(v)
}
```

Requires `make proto-gen` after the `.proto` edit (regenerates
`gen/proto/go/session/v1/*.go` and `web-app/src/gen/session/v1/*_pb.ts`).

### AC4 "selectable time window" — already fully implemented, reuse as-is

**No new RPC/proto param needed.** `GetApprovalAnalyticsRequest.window_days`
(`session.proto:1422`, `optional int32`) already exists and is already the single source of
truth for the time window across every breakdown in this panel:

- Backend: `RulesService.GetApprovalAnalytics` (`rules_service.go:159–201`) reads
  `req.Msg.WindowDays` (default 7, clamped 1–90 at line 163–169), computes `since`, loads
  `analyticsStore.LoadWindow(since)`, and calls `ComputeSummary(entries)` — the new
  `EscalationReasonCounts` field falls out of this same call for free once §3's loop change
  lands. No handler changes needed beyond `summaryToProto`.
- Frontend: `ApprovalAnalyticsPanel.tsx:104` already holds `const [windowDays, setWindowDays] =
  useState(7)`, feeds it to `useApprovalAnalytics({ windowDays })` (line 106), and renders a
  4-button selector (7/14/30/90 days, `WINDOW_OPTIONS` at line 86–91) that calls
  `setWindowDays`. `summary` returned from that hook is already the whole
  `AnalyticsSummaryProto`-derived object scoped to the current `windowDays` — a new
  "Escalation Reasons" section just reads `summary.escalationReasonCounts` off the same object,
  no new fetch/param plumbing required.

### Frontend rendering (AC4 "+ frontend rendering test")

Model the new section on the existing "Top Triggered Rules" table
(`ApprovalAnalyticsPanel.tsx:276–301`), which is the closest existing precedent for a small
fixed-cardinality category breakdown with a `Bar`. Suggested placement: a new `tableSection`
directly after the "Top Triggered Rules" block (still inside the `twoColGrid` row at line
247–304) or as its own full-width section after "Top Python Imports" (line 306–333) — either
works structurally; the plan phase should pick based on visual balance, not architecture.

```tsx
{summary && Object.keys(summary.escalationReasonCounts).length > 0 && (
  <div className={tableSection}>
    <h3 className={sectionTitle}>Escalation Reasons</h3>
    <div className={tableWrapper}>
      <table className={table}>
        <thead>
          <tr>
            <th className={th}>Reason</th>
            <th className={`${th} ${thRight}`}>Count</th>
            <th className={th}>Share</th>
          </tr>
        </thead>
        <tbody>
          {Object.entries(summary.escalationReasonCounts)
            .sort(([, a], [, b]) => b - a)
            .map(([category, count]) => (
              <tr key={category} className={row}>
                <td className={td}>{ESCALATION_CATEGORY_LABELS[category] ?? category}</td>
                <td className={`${td} ${tdRight}`}>{count}</td>
                <td className={`${td} ${tdBar}`}>
                  <Bar value={count} max={maxEscalationCount} className={barRule} />
                </td>
              </tr>
            ))}
        </tbody>
      </table>
    </div>
  </div>
)}
```

with a small label map (category key → reviewer-facing string) colocated near `WINDOW_OPTIONS`:

```ts
const ESCALATION_CATEGORY_LABELS: Record<string, string> = {
  "no-match": "No Rule Match",
  "explicit-rule": "Explicit Rule",
  "domain-age": "New Domain",
  "secret-scan": "Secret Detected",
  "unclassifiable": "Unclassifiable (shell expansion)",
};
```

Test: extend `ApprovalAnalyticsPanel.test.tsx` with a case that mocks
`useApprovalAnalytics` to return a `summary.escalationReasonCounts` fixture and asserts the
section renders category labels + counts — same pattern the file already uses for the
"Top Triggered Rules" assertions (confirm exact mock shape by reading that test file in the
plan phase; not read in full here since the RPC/type shape is what matters for this research
pass, not the existing test's mock scaffolding).

## 4. `ReviewQueuePanel.tsx` — exact JSX insertion points

Read `web-app/src/components/sessions/ReviewQueuePanel.tsx` (1437 lines) around both target
regions.

### The `itemContext`-suppression "bug" (line 718) — confirmed, and why it matters here

```tsx
{queueItem.context && !queueItem.metadata?.["pending_approval_id"] && (
  <p className={itemContext}>{queueItem.context}</p>
)}
```

This line **never renders `itemContext` for approval items** — `pending_approval_id` is always
set for `ReasonApprovalPending` items (that's the whole point of the enrichment block in
`review_queue_poller.go:808`), so the condition is always false for exactly the item type this
feature targets. This is not a bug to "fix" by removing the guard (that would make `context`
double up with the approval-specific `commandPreview` block); it's the reason AC6 says "via the
existing `itemContext` CSS class" rather than "by removing the suppression" — the plan needs a
**second**, separate use of the `itemContext` class *inside* the `pending_approval_id` branch
(lines 726–743), not a change to line 718's guard.

### Exact insertion (inside the existing `pending_approval_id` branch, lines 726–743)

```tsx
{queueItem.metadata?.["pending_approval_id"] && (
  <>
    {queueItem.metadata["escalation_reason"] && (
      <p className={itemContext}>{queueItem.metadata["escalation_reason"]}</p>
    )}
    {(queueItem.metadata["tool_input_command"] || queueItem.metadata["tool_input_file"]) && (
      <pre className={commandPreview}>
        {queueItem.metadata["tool_input_command"] || queueItem.metadata["tool_input_file"]}
      </pre>
    )}
    {/* ... cwd / orphaned unchanged ... */}
  </>
)}
```

Placed **first**, before the command preview `<pre>`, so the reviewer reads "why" before "what"
— directly answering the Problem statement ("the reviewer sees *what* is being requested but not
*why*").

### The Create Rule button block (lines 818–838) — AC7 intent change

Current:
```tsx
{queueItem.metadata?.["tool_input_command"] && (
  <Button
    intent="ghost"
    size="md"
    ...
  >
    ✦ Create Rule
  </Button>
)}
```

AC7 requires `intent="secondary"` for no-match, never `"primary"`. Minimal compliant change:

```tsx
{queueItem.metadata?.["tool_input_command"] && (
  <Button
    intent="secondary"  // was "ghost" — AC7
    size="md"
    ...
  >
    ✦ Create Rule
  </Button>
)}
```

**Open question for the plan phase** (not resolved by this research, flagged rather than
guessed): AC3's phrasing ("**No-match** escalations surface the existing … flow") could mean
either (a) purely descriptive — the existing flow already works for no-match and needs no
gating change, just the AC7 intent styling — or (b) prescriptive — the button should only
render when `queueItem.metadata["escalation_reason_category"] === "no-match"`, since suggesting
a *new* rule is redundant when an explicit rule or domain-age check already fired. Both are
one-line changes (`{queueItem.metadata?.["tool_input_command"] && (...)}` vs.
`{queueItem.metadata?.["tool_input_command"] && queueItem.metadata?.["escalation_reason_category"] === "no-match" && (...)}`)
— defer the choice to `/sdd:3-plan` or the requirements author, since it's a product decision,
not an architectural one.

## 5. EventStorming grammar (Event-Command-Policy table)

This domain has 4 actors (classifier, domain-checker, secret-scanner, human reviewer) and
multiple decision paths converging on one queue item, which justifies EventStorming as a
sanity check for *where the reason field belongs* (per the research question).

| # | Event / Command | Trigger | Actor | Payload (relevant fields) |
|---|---|---|---|---|
| 1 | **Command**: `RequestPermission` | Claude Code hook POST | Claude Code (external) | `ToolName`, `ToolInput`, `Cwd` |
| 2 | **Policy**: secret-scan (runs first, line 207) | on 1 | secret-scanner | scans `ToolInput["command"]` |
| 2a | **Event**: `SecretDetected` → **Command**: `AutoDeny` | if scan hits | secret-scanner | `RuleID="secret-scan"`, `Reason=msg` — **terminal**, no queue item (confirmed §1) |
| 3 | **Policy**: domain-age check (line 237) | on 1, if 2 didn't fire | domain-checker | resolves domains in command, calls `IsNewlyRegistered` |
| 3a | **Event**: `NewDomainDetected` → **Command**: `Escalate` | if new domain | domain-checker | `RuleID="new-domain-check"`, `Reason=<domain text>` |
| 4 | **Policy**: classify (line 280) | on 1, if 2/3a didn't fire | classifier | `ClassificationResult{Decision, RuleID, Reason}` |
| 4a | **Event**: `RequestClassified` | on 4 | classifier | `Decision ∈ {AutoAllow, AutoDeny, Escalate}`, `RuleID`, `Reason` |
| 4b | **Command**: `AutoAllow` / `AutoDeny` | if 4a.Decision ≠ Escalate | classifier | **terminal**, no queue item |
| 4c | **Command**: `Escalate` | if 4a.Decision == Escalate | classifier | same `RuleID`/`Reason` as 4a |
| 5 | **Event**: `ApprovalCreated` | on 3a or 4c (both converge at `createApproval:` label) | ApprovalHandler | **`EscalationReason`, `EscalationCategory` belong HERE** — the single convergence point where both upstream policies land |
| 6 | **Event**: `ApprovalPersisted` | on 5 | ApprovalStore | disk write includes `EscalationReason`/`EscalationCategory` (AC2) |
| 7 | **Event**: `ReviewItemEnriched` | poller tick reads 5/6 | ReviewQueuePoller | copies `EscalationReason`/`EscalationCategory` into `ReviewItem.Metadata` (AC1/AC3/AC6) |
| 8 | **Command**: `ResolveApproval` (Approve/Deny) | human reviewer clicks | human reviewer | consumes `ApprovalID`; unaffected by this feature |
| 9 | **Event**: `ApprovalResolved` | on 8 | ApprovalStore | terminal |

**Sanity check result: the reason field belongs on `ApprovalCreated` (step 5), not recomputed
later.** Both upstream policies (domain-age at 3a, classifier-escalate at 4c) produce the same
shape (`RuleID` + `Reason`) *before* reaching the single convergence point at the
`createApproval:` label — that convergence point is exactly the hoisted `escalation` variable's
scope in §1. Recomputing the reason later (e.g., at poller-enrichment time, step 7, by
re-deriving it from `ToolInput`) would require either (a) storing the raw classification inputs
long enough to re-run the classifier a second time — wasteful and could produce a *different*
answer if rules changed between escalation and poller tick — or (b) duplicating the
`RuleID`→category/text mapping in two places. Both are worse than carrying the already-computed
`EscalationReason`/`EscalationCategory` strings through the existing
`PendingApproval → PersistedApproval → ApprovalMetadata → ReviewItem.Metadata` pipe, which this
design does. This also matches why `CategorizeEscalationRuleID`/`EscalationReasonText` are pure
functions of already-captured data (`RuleID`, `Reason`) rather than functions that re-inspect
`ToolInput` — the classification decision is a fact about the past, not something to
re-derive from current state.

## Summary of every touch point (for the plan phase task list)

| # | File | Change |
|---|---|---|
| 1 | `pkg/classifier/escalation.go` (new) | `EscalationCategory` type + 5 consts, `CategorizeEscalationRuleID`, `EscalationReasonText` |
| 2 | `server/services/approval_handler.go` | hoist `var escalation classifier.ClassificationResult`; populate in domain-age branch (drop `_ = reason`); add explicit `case classifier.Escalate: escalation = result`; set `EscalationReason`/`EscalationCategory` on `PendingApproval` literal |
| 3 | `server/services/approval_store.go` | add `EscalationReason`/`EscalationCategory` to `PendingApproval`, `PersistedApproval`, `persistToDiskLocked`, `loadFromDisk`, `GetApprovalMetadataBySession` |
| 4 | `session/review_queue_poller.go` | add `EscalationReason`/`EscalationCategory` to `ApprovalMetadata` struct; copy into `item.Metadata["escalation_reason"]` / `["escalation_reason_category"]` in the enrichment block |
| 5 | `server/services/analytics_store.go` | add `escalationReasonCounts` accumulator + branch in `ComputeSummary`'s loop; add `EscalationReasonCounts map[string]int` to `AnalyticsSummary` |
| 6 | `proto/session/v1/types.proto` | add `map<string, int32> escalation_reason_counts = 17;` to `AnalyticsSummaryProto`; run `make proto-gen` |
| 7 | `server/services/rules_service.go` | `summaryToProto` — convert new map field |
| 8 | `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx` | new "Escalation Reasons" table section + `ESCALATION_CATEGORY_LABELS` map |
| 9 | `web-app/src/components/sessions/ApprovalAnalyticsPanel.test.tsx` | rendering test for new section (AC4) |
| 10 | `web-app/src/components/sessions/ReviewQueuePanel.tsx` | new `itemContext` paragraph inside `pending_approval_id` branch (AC1/AC6); `Button intent` ghost→secondary on Create Rule (AC7) |
| 11 | `server/services/approval_handler_test.go` | verify no regressions (AC5) + new coverage for both escalation branches populating `escalation` |
| 12 | `session/review_queue_determiner_test.go` | verify no regressions (AC5) |
| 13 | `tests/e2e/*.spec.ts` (new) | real Playwright spec: session → hook POST → ApprovalStore → poller → rendered `/review-queue` page (AC8) |
