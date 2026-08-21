# Research: Build vs. Buy — Review Queue Severity

## Question

Should severity classification for the review queue be built from scratch, or sourced
from an existing solution?

## 1. Existing OSS library / framework (severity classification, badge components, triage-queue UI)

**Findings:**
- `go.mod`: no severity/priority/classification/badge library dependency exists (checked
  for `badge|severity|priority|classif|alert|incident|status|color` — only hit is
  `github.com/mattn/go-colorable`, an ANSI-color terminal writer, unrelated).
- `web-app/package.json`: no badge/chip/tag/severity UI library dependency. The only
  UI-adjacent icon lib is `lucide-react` (already a dependency, used for icons generally,
  not severity-specific).
- The repo already has an in-house generic `Badge` component
  (`web-app/src/components/ui/Badge.tsx` + `Badge.css.ts`, vanilla-extract `recipe` with
  `intent`/`size` variants per ADR-009's CSS architecture) and a proven pattern for
  mapping a proto enum to a labeled/colored/iconed badge:
  `web-app/src/components/sessions/StatusBadge.tsx`'s `getAttentionReasonInfo()` /
  `getDetectedStatusInfo()` — enum value → `{ label, icon, variant }` → span with
  vanilla-extract variant class + `role="status"` + `aria-label`.

**Pros of adopting an external library:** none identified that clear the bar — this is
an enum (4 values) rendered as a colored badge with a sort key, not a general-purpose
design problem.

**Cons:**
- Adds a new dependency (bundle size, security-review surface, ADR-009 CSS-architecture
  compliance risk if the library ships its own CSS-in-JS or global stylesheet) for
  something the repo's own `Badge`/`StatusBadge` pattern already solves in ~30 lines.
- Any severity-badge npm package would need re-theming to the repo's vanilla-extract
  token contract (`web-app/src/styles/theme.css.ts`) anyway — the "reuse" value is
  mostly absorbed by that adaptation cost.

**Verdict: Not recommended.** Fork the existing `StatusBadge.tsx` pattern into a new
`SeverityBadge.tsx` using the existing `Badge` primitive instead.

## 2. SaaS / managed (hosted incident-triage/classification service, e.g. PagerDuty-style)

**Findings:** The review queue is an internal, synchronous human-approval gate for
in-flight agent tool-use requests (`PendingApproval` / `ListPendingApprovals`,
`ReviewQueuePanel.tsx`) — not an incident-management or on-call paging workflow. There is
no alerting, escalation-to-a-person/role, or SLA-tracking requirement in scope (the
requirements doc explicitly rules out "automatic escalation routing to a specific
person/role" as out of scope, citing no existing role/assignee concept in the codebase).
A hosted triage/classification service would add an external network dependency and auth
surface for a purely-local computation (mapping an already-known `classifier.RiskLevel`
enum to a badge and a sort key).

**Verdict: Not recommended — inapplicable.** No SaaS integration makes sense here; this
is local enum-to-display logic, not incident routing.

## 3. LLM-generated bespoke logic vs. existing `pkg/classifier` package

**Findings — confirms the requirements doc's audit, no gaps found beyond what's
documented:**
- `pkg/classifier.RiskLevel` (`pkg/classifier/classifier.go:16-24`) is a 4-value `iota`
  enum (`RiskLow`/`RiskMedium`/`RiskHigh`/`RiskCritical`) already populated on every
  `ClassificationResult` and every seed `Rule`, including the issue's own `rm`/force-push
  examples (`RiskCritical` seed rules, e.g. lines ~765, 792, 825, 863 in
  `classifier.go`).
- String ⇄ enum conversion already exists and is reused across the backend — no need to
  write it fresh:
  - `riskLevelString(classifier.RiskLevel) string` — canonical version,
    `server/services/analytics_store.go:574-587`.
  - `parseRiskLevel(string) classifier.RiskLevel` — `server/services/rules_store.go:351-364`.
  - `riskLevelToInt` / `riskLevelStringFromInt` (delegates to the canonical
    `riskLevelString`) — `server/services/rules_store.go:377-399`, used for JSON
    persistence of `ApprovalRule.RiskLevel` as an int in the ent-backed store.
  - This is the exact shape needed for wiring `RiskLevel` onto `PendingApprovalProto`
    (`risk_level string` field, matching the existing convention already used on
    `ApprovalRuleProto.risk_level` at `proto/session/v1/types.proto:1084` and
    `SuggestedRuleProto.risk_level` at `:1455`).
- **Gap found (minor, not blocking):** `classifier.RiskLevel` itself has zero methods
  defined in `pkg/classifier` — no `String()`, no ordering helper analogous to
  `queue.Priority.IsHigherThan()`. Every existing call site does bare int/enum
  comparison (e.g. `f.RiskLevel == RiskCritical` in `classifier.go:452`) or goes through
  the ad hoc `riskLevelString`/`parseRiskLevel` pair living in `server/services`, not on
  the type itself. This is pre-existing scatter, not something this feature needs to fix,
  but the plan should decide once whether new sort/compare logic for the queue lives as a
  `RiskLevel` method (idiomatic, matches `queue.Priority`'s existing pattern) or stays a
  free function in `server/services` (consistent with current `riskLevelString`
  placement) — worth a one-line decision in `plan.md`, not new design work.

**Verdict: Recommended — reuse as-is.** No bespoke severity-classification logic needs
to be written. The task is entirely: (a) copy `RiskLevel` from
`ClassificationResult`/`Rule` onto `PendingApproval` at creation time in
`approval_handler.go`'s `createApproval` path, where it's currently computed and
discarded; (b) add the `risk_level` proto field and regenerate; (c) reuse
`riskLevelString`/`parseRiskLevel` (or promote them to `RiskLevel` methods, per the
one-line decision above) for wire/JSON conversion. LLM-generating new pattern-matching or
classification rules would be duplicate, wasted work — the classifier already produces
the right value; it's just dropped before reaching the queue.

## 4. Fork or adapt existing prior art as a template

**Findings — two adjacent, non-identical templates, and the doc is right to warn against
conflating them:**

- **`session/queue/queue.go`'s `Priority` enum** (`PriorityUrgent`=1 .. `PriorityLow`=4,
  `session/queue/queue.go:51-100`) is the closest **structural** template for a Go enum
  with `String()`/`Emoji()`/`IsHigherThan()`/`IsValid()` methods and
  `CountByPriority()`-style aggregation (`queue.go:354-361`, directly analogous to the
  in-scope "approval analytics breakdown by severity"). **However**, per the requirements
  doc (and confirmed here), it answers a different question ("which *session* needs a
  human," derived from session/detection state) than per-*approval-request* risk
  (derived from classified tool-use content) — it must not be merged, only used as a
  *shape* template. Note also the inverted convention: `Priority`'s numeric value is
  urgency-descending (1=most urgent), while `RiskLevel`'s `iota` value is
  severity-ascending (0=`RiskLow` .. 3=`RiskCritical`) — a sort implementation adapted
  from `Priority` must not blindly copy the comparison direction.
- **`ApprovalRuleProto.risk_level` / `SuggestedRuleProto.risk_level`**
  (`proto/session/v1/types.proto:1084`, `:1455`) is the closest **wire-format** template
  — both are already `string risk_level` fields on proto messages in the same file,
  populated via the same `riskLevelString`/`parseRiskLevel` helpers. Adding
  `string risk_level` to `PendingApprovalProto` (currently absent,
  `types.proto:1034-1060`) is a direct copy of this existing convention, not a new
  design.
- **Frontend**: `RuleBuilderForm.tsx` currently renders `risk_level` as a plain
  `<select>` (`RuleBuilderForm.tsx:537`) with no color/badge — it is *not* a usable
  visual template. `StatusBadge.tsx` (see §1) is the better frontend template: same
  "enum → labeled/colored/iconed badge" shape as needed here, just for a different enum
  (`AttentionReason`/`DetectedStatus` instead of `RiskLevel`).

**Verdict: Recommended — adapt, don't invent.**
- Backend enum shape: adapt `queue.Priority`'s method pattern (`String()`,
  `IsHigherThan()`) onto `RiskLevel` if promoting the free functions to methods (see §3
  gap) — mind the inverted ordinal direction.
  Wire format: direct copy of the existing `risk_level string` proto field convention
  already used twice in the same file.
- Frontend: adapt `StatusBadge.tsx`'s enum→badge-info pattern into a new
  `SeverityBadge.tsx`, built on the existing `Badge` primitive — not
  `RuleBuilderForm.tsx`'s plain `<select>`.

## Summary Table

| Option | Verdict | Why |
|---|---|---|
| External OSS severity/badge library | Not recommended | Repo already has `Badge`/`StatusBadge` pattern; 4-value enum doesn't justify a new dependency or re-theming cost |
| SaaS/managed triage service | Not recommended (inapplicable) | Internal synchronous approval gate, not incident/on-call routing; no external service need |
| Reuse `pkg/classifier.RiskLevel` + existing string/int conversion helpers | **Recommended** | Already produces the right value at classification time; only dropped before reaching the queue. Wire-through problem, not new classification logic |
| Fork/adapt `queue.Priority` (Go shape) + `ApprovalRuleProto.risk_level` (wire) + `StatusBadge.tsx` (UI) as templates | **Recommended** | Closest prior art for each layer; mind `Priority`'s inverted urgency-descending ordinal vs. `RiskLevel`'s severity-ascending `iota` when adapting sort logic |
