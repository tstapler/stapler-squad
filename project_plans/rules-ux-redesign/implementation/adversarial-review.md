# Adversarial Review: Rules UX Redesign Implementation Plan

## Reviewer posture

This review assumes the plan will be implemented by a competent developer who follows it literally. Every ambiguity becomes a bug. Every optimistic assumption becomes a missed edge case. The goal is to find what breaks before code is written.

---

## Epic 1: Proto + Backend Extension — Feasibility Check

### Field number safety

**FEASIBLE.**

The current `ApprovalRuleProto` uses fields 1–14. The plan proposes 20–28. Fields 15–19 are genuinely unused (confirmed by reading `types.proto` lines 806–822). No conflict.

The plan's decision to add `tool_category` (field 28) is correct — it is already in `RuleSpec` and `ApprovalRuleData` but absent from the proto, causing a silent gap when listing seed rules via the UI.

### Story 1.3 — `specsToRules()` guard for mutual exclusivity

**RISKY.** The plan adds a mutual-exclusivity guard in `Upsert()` that rejects rules with both `commandPattern` and structured criteria. This is the correct semantic decision. However:

**Issue:** The validation guard is described as being in `Upsert()` of `rules_store.go`, but the `Upsert()` method only validates regex patterns — the `hasCriteria && commandPattern != ""` check must be added there explicitly. If a developer adds it to `UpsertApprovalRule()` in `rules_service.go` instead (which calls `rulesStore.Upsert()`), the guard would be bypassed if `Upsert()` is ever called directly. **Plan recommendation: add the guard in `rules_store.go Upsert()` as the authoritative enforcement point, and note this explicitly.**

The plan correctly places it in `Upsert()`. Rating: FEASIBLE — but the implementation note must be precise.

### Story 1.3f — `specsToRules()` criteria population

**FEASIBLE.** The `classifier.CommandCriteria` struct fields (`Programs`, `Subcommands`, `BlockedSubcommands`, `RequiredFlags`, `ForbiddenFlags`, `RequiredFlagPrefixes`, `PythonModes`, `SafePythonImportsOnly`) exactly match the names in the plan. The `RedirectionPattern *regexp.Regexp` field is intentionally omitted (correct — the UI cannot generate one).

### Story 1.4c — `ruleToSpec()` for seed rules

**RISKY.** This is the subtlest backend change. The `ruleToSpec()` function converts `classifier.Rule` (in-memory) to `RuleSpec` (serializable). Seed rules are constructed in Go code via `SeedRules()` with `Criteria` directly set. After the fix, `ruleToSpec()` will populate structured fields from `r.Criteria`.

**Issue:** The `ToolCategory` field is on `classifier.Rule` as a string, but the current `ruleToSpec()` does not extract it (confirmed by reading lines 234–257 of `rules_service.go`). The plan adds `r.ToolCategory` extraction. However, `classifier.Rule` must actually have a `ToolCategory` field — this must be verified before implementing. If it does not exist, Story 1.4c requires adding `ToolCategory string` to `classifier.Rule` as well.

**Action required (patch):** Before implementing, run `grep -n "ToolCategory" pkg/classifier/classifier.go` to confirm the field exists. If absent, add it and update `SeedRules()` for any seed rules that use tool category matching.

**Preliminary rating: RISKY** (field existence unconfirmed). Mark this as a required pre-implementation check.

---

## Epic 2: URL Query Param Prefill — Next.js App Router Feasibility

### Story 2.1 — `useSearchParams()` in App Router

**RISKY.**

In Next.js App Router, `useSearchParams()` requires:
1. The component calling it must be a Client Component (`"use client"` at the top).
2. The component must be wrapped in a `<Suspense>` boundary in its parent — otherwise Next.js 13/14/15 throws a build error or shows a fallback during prerendering.

**The plan describes reading `searchParams` in the `/rules` page.** If the `/rules` page is currently a Server Component (no `"use client"` directive), converting it to a Client Component will disable server-side data fetching for that page. If it is already a Client Component (likely, given it renders `ApprovalRulesPanel` which has `"use client"`), the Suspense wrapping requirement still applies.

**Verified risk:** The Next.js App Router docs state: "useSearchParams will cause the page to be dynamically rendered when used in a Client Component without a Suspense boundary." In production builds, this can cause a `useSearchParams was used in <Component> without a Suspense boundary` error.

**Patch to plan (Story 2.1):** The `/rules` page must wrap any component that calls `useSearchParams()` in `<Suspense fallback={null}>`. The prefill-reading logic should be extracted into a small `PrefillReader` client component:

```tsx
// web-app/src/components/rules/PrefillReader.tsx
"use client";
import { useSearchParams } from 'next/navigation';
import { Suspense } from 'react';

function PrefillReaderInner({ onPrefill }: { onPrefill: (p: RuleBuilderPrefill | null) => void }) {
  const params = useSearchParams();
  const raw = params.get('prefill');
  useEffect(() => { onPrefill(raw ? decodePrefill(raw) : null); }, [raw]);
  return null;
}

export function PrefillReader(props) {
  return <Suspense fallback={null}><PrefillReaderInner {...props} /></Suspense>;
}
```

This is a minor structural change but prevents a production build failure. **Rating: RISKY → FEASIBLE after patch.**

### Story 2.2 — `buildPrefillHref` replaces `href="/rules"`

**FEASIBLE.** The existing `<a href="/rules">` elements are plain HTML anchors. Replacing with `buildPrefillHref(...)` is a straightforward string substitution. No routing framework interaction needed — these are full-page navigation links, not `<Link>` components.

One concern: the subcommand value in `CommandDistributionTable` uses `s.subcommand` which may be an empty string for programs with no detectable subcommand. The prefill should only include `subcommands` in the payload if `s.subcommand` is non-empty:

```typescript
buildPrefillHref({
  programs: [s.programName],
  ...(s.subcommand ? { subcommands: [s.subcommand] } : {}),
})
```

**Patch to plan (Story 2.2):** Add this guard.

---

## Epic 3: Frontend Structured Rule Builder — Feasibility Check

### Story 3.1 — TagInput component

**FEASIBLE.** The implementation is well-specified. The vanilla-extract `recipe()` import is confirmed available (`@vanilla-extract/recipes` in `package.json`). The keyboard contract (Enter/comma to add, Backspace to remove last) is standard and implementable.

**Potential pitfall:** The paste handler must prevent the default paste behavior to avoid both the raw text and the split tags appearing. Use `e.preventDefault()` before processing `e.clipboardData.getData('text')`.

### Story 3.2 — RuleBuilderForm with react-hook-form + zod

**RISKY.**

The plan uses `z.enum(['structured', 'regex'])` for mode — this is correct.

**Issue 1 — zod v4 API:** The project uses `zod` v4.1.11 (confirmed from `package.json`). The import path in the plan is `'zod/v4'`. This is the correct zod v4 import path for packages that have a compatibility shim. However, `@hookform/resolvers/zod` must also support zod v4 — verify `@hookform/resolvers` version supports zod 4. If the resolvers package does not yet support zod v4, the integration will fail at runtime with a schema validation error.

**Patch to plan (Story 3.2):** Add a pre-implementation check: `npm ls @hookform/resolvers` to confirm the installed version supports zod v4. If it does not, use zod v3 API (`import { z } from 'zod'`) which is the stable API form, or use manual validation instead of zodResolver.

**Issue 2 — Mode switch confirmation UX:** The plan suggests using `browser confirm()` for the mode-switch confirmation. `confirm()` is a blocking call that freezes UI thread execution. In production Next.js apps, this works but is considered poor UX (cannot be styled, blocks everything). An inline confirmation div (show "Clear structured fields? [Yes] [No]" inline in the form) is listed as the alternative. The plan should commit to the inline div approach — not `confirm()`.

**Patch to plan (Story 3.2):** Replace "Uses browser `confirm()`" with "Uses an inline confirmation div — two small Yes/No buttons appear in-place for 3 seconds, auto-dismissing to No if not clicked."

**Issue 3 — Priority auto-default values:** The plan says DENY defaults to 950, ESCALATE to 450, ALLOW to 100. The existing form defaults to 10 for all decisions. The plan's defaults are much more sensible, but there is a UX edge case: if the user switches decision type after editing the priority manually, auto-defaulting will overwrite their custom priority. The auto-default should only fire when priority equals the previous decision's default, not when it has been user-modified.

**Issue 4 — ToolCategory dropdown options:** The plan lists `builtin-agent, mcp-read, mcp-write, any` as dropdown options for tool category. Verify that these are the exact values the classifier uses — grep `ToolCategory` in the classifier and seed rules. If they differ, the UI will create non-functional rules silently.

### Story 3.3 — RulePreview with client-side TypeScript port

**RISKY.**

**Issue 1 — `extractSubcommand` port complexity:** The Go `extractSubcommand` function in `command_parser.go` uses:
- A `deepSubcommandPrograms` set (11 programs: gh, aws, gcloud, az, doctl, fly, flyctl, kubectl, docker, heroku, ip)
- A `prefixFlagArgs` map (flags that consume the next token as a value: git `-C`, `-c`; many others)
- `isSubcommandLike()` predicate (starts with letter, max 25 chars, only word chars)

Porting this accurately is feasible but is the most likely source of preview–backend divergence. If the TypeScript port of `extractSubcommand` is wrong, the preview will show commands matching that the real classifier does not, or vice versa. This degrades user trust in the preview.

**The key risk:** The `prefixFlagArgs` map for each program is internal state in `command_parser.go`. It must be manually transcribed to TypeScript and kept in sync. This is a maintenance burden.

**Mitigation (patch to plan):** Add a note: "The `prefixFlagArgs` data must be transcribed from `pkg/classifier/command_parser.go` and maintained in sync. Consider adding a comment in both files referencing the other."

**Issue 2 — `useDeferredValue` is not a debounce:** `useDeferredValue` marks rendering as non-urgent — it does not add a time delay. On fast machines with simple criteria, the preview will update on every keystroke (no 200 ms delay). This is actually better behavior than the requirements specify (AC-4.4 says "within 200ms"), but it means CPU-heavy preview computation could cause jank on slow hardware with large example banks. The example bank (30 commands × 18 programs = 540 items) is small enough that this is unlikely to matter.

**Issue 3 — Empty example bank for unknown programs:** If a user types a program not in the example bank (e.g., `terraform`), the preview shows "No examples available." This is correct behavior per the plan but should be called out explicitly in Story 3.3 as expected behavior, not a bug.

**Rating: RISKY but manageable.** The port is the right approach; the maintainability risk is real.

### Story 3.4 — TemplateLibrary with Radix Dialog

**FEASIBLE.** `@radix-ui/react-dialog` v1.1.15 is confirmed installed. The card grid approach is appropriate for 13 templates.

**One gap in the plan:** Template `escalate-unknown` uses `commandPattern: ".*"` as a catch-all. A rule with only `commandPattern: ".*"` and the mutual-exclusivity guard removed (because `programs` is empty) is valid. But `".*"` as a regex would match every command — this is the intended behavior for a catch-all, but the priority must be set very low (e.g., 10) to avoid overriding everything. The template definition must include `priority: 10` and the description must explain this.

**Patch to plan (Story 3.4):** Add `priority: 10` to the `escalate-unknown` template and note its catch-all semantics in the description.

Also: The `file-editing-tools` template lists `toolName: Edit` but the classifier also uses `Write`, `Edit`, and other names for file editing. The template should use `toolPattern: "Edit|Write|MultiEdit"` instead of `toolName: Edit` to be more useful.

**Patch to plan (Story 3.4):** Change `file-editing-tools` template to use `toolPattern: "Edit|Write|MultiEdit"` and mode `Regex` (since it uses a tool pattern regex, not a tool name).

### Story 3.5 — Integration into ApprovalRulesPanel

**FEASIBLE with one concern.**

**Issue:** The plan says to pass `prefill` via "prop drilling from the page or a context." Given that `ApprovalRulesPanel` is a deeply nested component, prop drilling will be messy. The cleaner approach is a React context (`RuleBuilderPrefillContext`) or to handle prefill entirely within the `/rules` page component by passing a single `initialPrefill` prop to `ApprovalRulesPanel`.

**Patch to plan (Story 3.5):** Commit to the simpler approach: the `/rules` page reads the prefill from the URL (via the `PrefillReader` component) and passes it as an `initialPrefill?: RuleBuilderPrefill` prop to `ApprovalRulesPanel`. No context needed.

---

## Epic 4: Enhanced Rule Table Display — Feasibility Check

### Story 4.1 — `describeRule` helper

**FEASIBLE.** The logic is straightforward and fully within TypeScript. The plan covers all branches. Minor gap: what if both `programs` and `toolName` are set? The plan's logic evaluates `programs` first, which is correct (structured takes priority), but this case should be explicitly noted.

### Story 4.2 — MatchDescription component

**FEASIBLE.** The replacement of raw `matchChip` code elements with `MatchDescription` is a drop-in substitution.

### Story 4.3 — Edit button

**FEASIBLE.** The `editingRule` state and scroll-to-builder pattern is straightforward.

---

## Epic 5: Richer Analytics — Feasibility Check

### Story 5.1 — Manual outcome summary card

**FEASIBLE.** The `decisionCounts["manual_allow"]` and `decisionCounts["manual_deny"]` keys are already populated in the existing `ComputeSummary()` output (verified by reading `analytics_store.go`). No proto change needed for this story.

### Story 5.2 — Command Distribution manual columns

**RISKY.** This requires:
- Proto change to `SubcommandStatProto` (fields 5–6)
- `SubcommandStat` struct change in `analytics_store.go`
- `ComputeSummary()` aggregation change
- `summaryToProto()` mapping change
- Frontend table changes

This is a 5-file change for analytics data that may have very few rows with non-zero `manual_allow`/`manual_deny` for most users. The scope-to-value ratio is low.

**More critically:** The plan says to add the accumulation in `ComputeSummary()` during the analytics loop. But `ComputeSummary()` iterates over `AnalyticsEntry` records, and `AnalyticsEntry.Decision` is already a string (`"manual_allow"`, `"manual_deny"`, etc.). The change is mechanically straightforward.

**Risk factor:** The `analytics_store.go` `ComputeSummary()` function is the most complex part of the backend (400+ lines). Modifying it risks introducing bugs in the existing analytics calculations. Suggest adding a focused test for the manual count accumulation before and after the change.

**Rating: RISKY** (medium complexity, low usage impact). Consider deferring to a follow-up story.

### Story 5.3 — Coverage gap outcome badges

**RISKY → BLOCKED for the per-program breakdown.**

**Issue:** `ProgramStatProto` and `ToolStatProto` currently have only `count` (field 3). Adding `manual_allow` (field 4) and `manual_deny` (field 5) requires changes to:
- `types.proto` (2 message changes)
- `AnalyticsSummary` and `ProgramStat`/`ToolStat` structs in `analytics_store.go`
- `ComputeSummary()` accumulation for uncovered programs/tools (currently it only counts total occurrences)
- `summaryToProto()` — 2 more mapping sections

**The ComputeSummary() accumulation gap:** The current uncovered programs/tools accumulation only increments `count`. To add manual outcome data, the loop must also track which entries among the uncovered ones had `manual_allow` or `manual_deny` outcomes. This requires either:
1. A separate pass over the uncovered entries after classification, or
2. Tracking per-program outcomes during the main loop and joining with uncovered status at the end.

The plan does not fully specify which approach. This is an implementation gap.

**Patch to plan (Story 5.3):** Add explicit implementation guidance:

During `ComputeSummary()`, maintain a `uncoveredProgramOutcomes map[string][2]int` (program → [manualAllow, manualDeny]). An entry is considered "uncovered" when `entry.RuleID == ""` (no rule matched). After the main loop, join with the existing `uncoveredPrograms` map to set the outcome counts. Same approach for tools.

**Rating: RISKY** (requires careful implementation). The approach is sound once spelled out.

---

## Three Most Likely Implementation Failures

### Failure 1: `useSearchParams()` Suspense boundary missing

The Next.js App Router constraint on `useSearchParams()` requiring a `<Suspense>` boundary is not obvious and is frequently missed. Without it, the production build will either fail or produce a degraded fallback page with no search param access. This affects the entire US-7 analytics-to-rule workflow.

**Probability: HIGH.** This is the most common Next.js App Router mistake.

**Mitigation:** The `PrefillReader` component pattern (added to plan) prevents this.

### Failure 2: `ruleToSpec()` tool category field not on `classifier.Rule`

If `classifier.Rule` does not have a `ToolCategory string` field, Story 1.4c will fail to compile. This would be discovered at `make build` time, but it means the developer must also add the field to `classifier.Rule` and handle it in `SeedRules()` — an additional change not called out in the plan.

**Probability: MEDIUM.** The field exists in `ApprovalRuleData` and `RuleSpec` but its presence in `classifier.Rule` is unconfirmed.

**Mitigation:** Add a pre-implementation check (added to plan).

### Failure 3: react-hook-form + zod v4 resolver incompatibility

If `@hookform/resolvers` does not support zod v4, the `zodResolver` integration will throw a runtime error when the form attempts to validate. Since `zod` v4 changed its internal API, older resolver versions are incompatible.

**Probability: MEDIUM-HIGH.** zod v4 was released recently; many ecosystem packages lag.

**Mitigation:** Pre-implementation check added to plan; fallback to manual validation documented.

---

## Story-by-Story Ratings

| Story | Rating | Reason |
|-------|--------|--------|
| 1.1 Proto fields | FEASIBLE | Field numbers verified, defaults safe |
| 1.2 Ent schema | FEASIBLE | Standard ent JSON fields pattern |
| 1.3 Domain + storage | FEASIBLE | Mechanical field propagation |
| 1.4 Service layer | RISKY | `ToolCategory` on `classifier.Rule` unconfirmed; requires pre-check |
| 1.5 Build check | FEASIBLE | Standard make commands |
| 2.1 Prefill mechanism | RISKY → FEASIBLE (after Suspense patch) | App Router Suspense requirement |
| 2.2 Analytics links | FEASIBLE | Simple href replacement; guard for empty subcommand added |
| 3.1 TagInput | FEASIBLE | Well-specified, no external deps needed |
| 3.2 RuleBuilderForm | RISKY | zod v4 resolver unconfirmed; confirm() UX patched |
| 3.3 RulePreview | RISKY | Port accuracy depends on `prefixFlagArgs` transcription |
| 3.4 TemplateLibrary | FEASIBLE (after patches) | escalate-unknown priority + file-editing template patched |
| 3.5 Integration | FEASIBLE (after prop approach clarified) | initialPrefill prop simplifies wiring |
| 4.1 describeRule | FEASIBLE | Pure TS logic |
| 4.2 MatchDescription | FEASIBLE | Simple component |
| 4.3 Edit button | FEASIBLE | Trivial change |
| 5.1 Summary card | FEASIBLE | Uses existing decisionCounts data |
| 5.2 Cmd distribution | RISKY | Multi-file change to complex function; low-risk to defer |
| 5.3 Gap badges | RISKY | Per-program manual outcome accumulation needs explicit guidance (added) |

---

## Overall Verdict: CONCERNS

The plan is implementable but has 4 patches required before coding begins:

**P1 (BLOCKING if missed) — Story 2.1:** Add `<Suspense>` wrapper via `PrefillReader` component to prevent Next.js App Router `useSearchParams()` production failure.

**P2 (BLOCKING if field absent) — Story 1.4:** Add pre-implementation check for `ToolCategory` on `classifier.Rule`. If absent, add the field as a prerequisite.

**P3 (BLOCKING if incompatible) — Story 3.2:** Add pre-implementation check for `@hookform/resolvers` zod v4 support. Fall back to manual validation if incompatible.

**P4 (CORRECTNESS) — Stories 2.2, 3.4, 5.3:** Three smaller patches:
- Guard against empty subcommand in prefill payload construction
- Fix `escalate-unknown` template priority (add priority: 10)
- Fix `file-editing-tools` template to use `toolPattern` instead of `toolName`
- Add explicit accumulation guidance for Story 5.3 per-program manual outcomes

---

## Patched Plan Re-review

After applying the 4 patches above:

- Epic 1: FEASIBLE (pending `ToolCategory` field pre-check)
- Epic 2: FEASIBLE (Suspense wrapper added)
- Epic 3: FEASIBLE (zod pre-check added; templates patched; confirm() replaced with inline UX)
- Epic 4: FEASIBLE
- Epic 5: Story 5.1 FEASIBLE; Stories 5.2–5.3 CONCERNS (medium complexity, recommend scoping to follow-up if timeline is tight, but implementable with the added guidance)

**Post-patch verdict: CONCERNS → approaching CLEAN**

The remaining concerns are implementation-complexity risks (Epic 3 preview port, Epic 5 analytics accumulation) rather than architectural blockers. The plan is safe to execute with careful attention to the pre-implementation checks listed above.

---

## Recommendations for Implementation Order

1. Run pre-implementation checks before writing any code:
   - `grep -n "ToolCategory" pkg/classifier/classifier.go`
   - `npm ls @hookform/resolvers` — check if `>=3.10` (zod v4 support added in resolvers 3.10)
   - Verify `/rules` page file path and whether it is a Server or Client Component
2. Implement Epic 1 completely and run `make build && make test` before touching the frontend
3. Implement Story 3.1 (TagInput) in isolation with unit tests before Story 3.2
4. Defer Stories 5.2 and 5.3 to a separate PR if the timeline is tight — they are valuable but not part of the core UX redesign
