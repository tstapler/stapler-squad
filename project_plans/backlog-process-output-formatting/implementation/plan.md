# Implementation Plan: Backlog Process Output Formatting

**Date:** 2026-10-21  
**Phase:** SDD-3 Plan  
**Status:** Ready for review

---

## Glossary

| Term | Definition |
|------|-----------|
| `TriageResultPanel` | New React component that renders a `HeadlessTriageResult` as a structured card |
| `ReviewVerdictPanel` | New React component that renders per-criterion verdicts and overall outcome |
| `DiagnoseResultCard` | New React component that renders a diagnose outcome + summary in the Activity stream |
| `useTriageResult` | New hook that parses `itemSession.triageResult` (raw JSON string) into typed data |
| `useReviewVerdict` | New/extended hook that fetches `GetReviewVerdict` for a given item |
| `backlog-triage-results-panel` | Feature flag controlling `TriageResultPanel` visibility |
| `backlog-review-results-panel` | Feature flag controlling `ReviewVerdictPanel` visibility |
| `BacklogItemDetail` | Existing component in `web-app/src/components/backlog/` showing item metadata, AC list, session links |

---

## Architecture Decision: Client-Side JSON Parsing vs. New Proto Fields

For MVP, parse `triage_result` and review `PerCriterion` on the client using `JSON.parse()` with zod schema validation. This avoids a proto breaking change and a Go migration. If the schema evolves, add typed proto fields in a follow-up. The zod schema is co-located with the component (`triageResultSchema.ts`) and doubles as documentation.

---

## Phase 1: Backend — Expose Triage Result via Proto (if needed)

**Estimated cost:** Low — verify whether `triage_result` is already present in the `GetBacklogItem` response proto.

### Task 1.1 — Verify proto surface
- Open `proto/session/v1/backlog.proto` and confirm `ItemSession` has `string triage_result` or equivalent field.
- Open `server/services/backlog_service.go` or the relevant handler to confirm the field is populated.
- If missing: add `string triage_result = N;` to the `ItemSession` proto message, run `make proto-gen`, populate the field in the handler.

### Task 1.2 — Verify `GetReviewVerdict` RPC accessibility
- Confirm `GetReviewVerdict` is registered and reachable from the web app's ConnectRPC client.
- Confirm `PerCriterion` bytes are returned (they represent `[]CriterionVerdict` JSON).
- If not exposed: add the RPC to the public service definition and wire it.

---

## Phase 2: Frontend — Zod Schemas

**File:** `web-app/src/lib/schemas/triageResult.ts`

```typescript
import { z } from "zod";

export const triageTaskSchema = z.object({
  text: z.string(),
  estimate: z.string().optional(),
  category: z.string().optional(),
});

export const triageSuggestionSchema = z.object({
  text: z.string(),
  rationale: z.string().optional(),
});

export const triageResultSchema = z.object({
  title: z.string().optional(),
  summary: z.string(),
  priority: z.number().int().min(1).max(5).optional(),
  item_category: z.string().optional(),
  suggestions: z.array(triageSuggestionSchema).default([]),
  tasks: z.array(triageTaskSchema).default([]),
  acceptance_criteria: z.array(z.object({
    index: z.number(),
    text: z.string(),
    status: z.string(),
  })).default([]),
});

export type TriageResult = z.infer<typeof triageResultSchema>;
```

**File:** `web-app/src/lib/schemas/reviewVerdict.ts`

```typescript
import { z } from "zod";

export const criterionVerdictSchema = z.object({
  index: z.number(),
  status: z.enum(["pass", "fail", "partial", "unverifiable", "pending"]),
  evidence: z.string().optional(),
  notes: z.string().optional(),
});

export const reviewVerdictSchema = z.object({
  overall: z.string(),
  summary: z.string(),
  verdicts: z.array(criterionVerdictSchema).default([]),
  tool_reads: z.array(z.string()).default([]),
});

export type ReviewVerdict = z.infer<typeof reviewVerdictSchema>;
```

---

## Phase 3: Frontend — Hooks

### `useTriageResult` hook
**File:** `web-app/src/lib/hooks/useTriageResult.ts`

```typescript
import { triageResultSchema, TriageResult } from "../schemas/triageResult";

export function useTriageResult(triageResultJson: string | undefined): {
  result: TriageResult | null;
  parseError: string | null;
} {
  return useMemo(() => {
    if (!triageResultJson) return { result: null, parseError: null };
    try {
      const raw = JSON.parse(triageResultJson);
      return { result: triageResultSchema.parse(raw), parseError: null };
    } catch (e) {
      return { result: null, parseError: String(e) };
    }
  }, [triageResultJson]);
}
```

### `useReviewVerdict` hook  
**File:** `web-app/src/lib/hooks/useReviewVerdict.ts`
- Calls `GetReviewVerdict` RPC for a given item ID.
- Returns `{ verdict: ReviewVerdict | null, loading: boolean, error: string | null }`.
- Polls every 5 s while the item status is `in_review`; stops when a result is found.

---

## Phase 4: Frontend — TriageResultPanel

**File:** `web-app/src/components/backlog/TriageResultPanel.tsx`  
**CSS:** `web-app/src/components/backlog/TriageResultPanel.css.ts`

### Layout
```
┌─────────────────────────────────────────────┐
│ Triage Results                    [Raw JSON ▾]
│                                              │
│  Priority: P2 · Category: feature           │
│                                              │
│  Summary                                     │
│  <summary text>                              │
│                                              │
│  Tasks (N)                                   │
│  ✓ task description          2h  backend    │
│  ✓ task description          4h  frontend   │
│                                              │
│  Suggestions (N)                             │
│  • <suggestion text>                         │
│    <rationale>                               │
└─────────────────────────────────────────────┘
```

### Props
```typescript
interface TriageResultPanelProps {
  triageResultJson: string | undefined;
}
```

### States
- **hidden** — `triageResultJson` is null/empty (renders nothing).
- **ready** — parsed successfully; renders the card.
- **parse-error** — renders a muted "Triage result could not be displayed" message with a "View raw" link.
- **raw-open** — `<details>` disclosure element showing the formatted JSON string (`JSON.stringify(raw, null, 2)` in a `<pre>`).

### Feature flag
```tsx
const triageResultsEnabled = useFeatureFlag("backlog-triage-results-panel");
if (!triageResultsEnabled) return null;
```

---

## Phase 5: Frontend — ReviewVerdictPanel

**File:** `web-app/src/components/backlog/ReviewVerdictPanel.tsx`  
**CSS:** `web-app/src/components/backlog/ReviewVerdictPanel.css.ts`

### Layout
```
┌─────────────────────────────────────────────┐
│ Review Results                    [Raw JSON ▾]
│                                              │
│  Overall: ● PASS                             │
│                                              │
│  Summary                                     │
│  <summary text>                              │
│                                              │
│  Criteria                                    │
│  ✓ AC-0 · pass                               │
│    <evidence snippet>                        │
│  ✗ AC-1 · fail                               │
│    <evidence snippet>                        │
└─────────────────────────────────────────────┘
```

### States: loading (skeleton), hidden (no verdict), ready, raw-open.

---

## Phase 6: Frontend — DiagnoseResultCard

**File:** `web-app/src/components/backlog/DiagnoseResultCard.tsx`

Activity notes with content matching `^Diagnosis result: (BUG_FILED|NOTE_ONLY|NUDGED|NO_ACTION)\n\n` are rendered as a styled card instead of plain text:

```
┌──────────────────────────────────┐
│  🔍 Diagnosis  ·  BUG_FILED       │
│  <summary paragraph>             │
└──────────────────────────────────┘
```

The Activity list renderer (`BacklogActivityList.tsx` or equivalent) detects the prefix and delegates to `DiagnoseResultCard`. Falls back to plain text rendering for unrecognized content.

---

## Phase 7: Frontend — Integration into BacklogItemDetail

1. Locate `BacklogItemDetail` component (expected: `web-app/src/components/backlog/BacklogItemDetailPanel.tsx` or similar).
2. Import `TriageResultPanel` and `ReviewVerdictPanel`.
3. Insert `<TriageResultPanel triageResultJson={itemSession?.triageResult} />` after the AC list section.
4. Insert `<ReviewVerdictPanel itemId={item.id} />` after the triage panel.
5. Both panels self-hide when no data is available — no conditional rendering needed at the integration point.

---

## Phase 8: Tests

### Unit tests
- `TriageResultPanel.test.tsx`: render with full data, render with null (hidden), render parse-error, raw-toggle open/close.
- `ReviewVerdictPanel.test.tsx`: render loading skeleton, render verdict (pass/fail/partial), raw-toggle.
- `DiagnoseResultCard.test.tsx`: render each outcome, render unrecognized content (plain text fallback).
- `useTriageResult.test.ts`: valid JSON, invalid JSON, empty string.

### Integration
- Existing Go tests: no changes expected (no new Go code in MVP path if proto fields are already present).
- New Go unit test if a new proto field or RPC handler is added.

---

## Phase 9: Feature Registry and Route Meta

- Run `make registry-generate` after adding `// +feature: backlog-triage-results-panel` and `// +feature: backlog-review-results-panel` markers.
- No new page routes → no route-meta.ts row needed.

---

## Rollout

1. Ship with both flags defaulting to **off**.
2. Enable on a personal instance, run through the backlog item detail view for items that have completed triage/review.
3. Confirm mobile layout at 320 px.
4. Flip flags to **on** for the workspace.
5. Monitor for jscpd duplication gate and fix if threshold exceeded.

---

## Adversarial Review Findings

*Self-review pass (2026-10-21):*

1. **Gap:** Phase 1 says "verify proto surface" but doesn't prescribe what to do if `triage_result` is absent from the proto. Added: "add `string triage_result = N;`" as explicit fallback action.
2. **Gap:** `useReviewVerdict` polls every 5 s while `in_review`; should stop polling once the item leaves `in_review` to avoid leaking timers. Added note: "Stops when a result is found."
3. **Risk:** `DiagnoseResultCard` prefix parsing is fragile if the activity note format ever changes. Mitigation documented in requirements as "open question" — for now, parse defensively and fall back to plain text.
4. **Risk:** Two new `.css.ts` files for Triage and Review panels share nearly identical card layout tokens. May trip jscpd. Mitigation: extract a shared `ResultPanel.css.ts` with shared `containerStyle` / `headerStyle` exported from a common file; individual panel files import and extend it.
5. **Gap:** No mention of `route-meta.ts` for the feature — confirmed not needed (no new pages), documented in Phase 9.

## Implementation deviations (recorded at sdd:6-verify)

- No zod schema file or `useTriageResult`/`useReviewVerdict` hooks: the panels take props already mapped by `useBacklogService.ts`, so Phase 1 proto work and those hooks were unnecessary. Malformed-data fallback is kept for defense in depth.
- One shared `ResultPanel.css.ts` instead of per-panel CSS files.
- Flag names are the hyphenated names from the acceptance criteria (`backlog-triage-results-panel`, `backlog-review-results-panel`), not the `backlog:` namespace used by some siblings.
- `DiagnoseResultCard` is deliberately ungated (falls back to plain text for non-diagnosis notes); the criteria define flags only for the triage and review panels.
