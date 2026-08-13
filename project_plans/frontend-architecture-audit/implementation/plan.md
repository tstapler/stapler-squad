# Frontend Architecture Audit — Implementation Plan

**Deliverable:** `docs/architecture/frontend-audit-2026.md` in the main repository  
**Phase:** Research → Report (no code changes)  
**Date:** 2026-05-08  
**Author:** Tyler Stapler

---

## Overview

This plan produces a written architecture report for the `web-app/src/` frontend. The report documents existing patterns, maps duplication, ranks consolidation opportunities, recommends tooling enforcement, and diagrams the actual layer model. All work is read-only analysis; the output is documentation.

---

## Epic 1: Pattern Inventory

**Goal:** Catalog every distinct pattern for state management, data fetching, CSS, and component structure, with at least one concrete file-level example per variant.

**Priority:** P1  
**Effort:** L (largest epic — 4 domains, 9 existing context files, 22 hooks, 115 CSS files)

**Acceptance Criterion:** Report section "Pattern Inventory" covers all 4 domains; each domain lists every known variant with at least 2 file-level examples and a one-sentence verdict on whether the variant is canonical, tolerated, or deprecated.

---

### Story 1.1 — State Management Pattern Inventory

**Priority:** P1 | **Effort:** M

Document every state management variant in use.

**Known variants from research:**
- Global provider context backed by Redux slice (`SessionServiceContext` → `sessionsSlice`)
- Global provider context backed by RTK Query (`ApprovalsContext` → `useGetApprovalsQuery`)
- Local scoped context for subtree state (`SessionVcsContext`, `CockpitActionsContext`)
- Direct RTK Query hook usage without context wrapper (`useApprovals` consumed by `ApprovalDrawer`)
- Direct Redux selector in component without hook wrapper (`SessionDetail.tsx` line 29: `useAppSelector(selectAllSessions)`)
- localStorage-persisted context (`NavigationContext`)

**Tasks:**
1. Read `web-app/src/lib/contexts/SessionServiceContext.tsx` — extract: what state it holds, backing store, key consumers. Note the three instantiation sites.
2. Read `web-app/src/lib/contexts/ApprovalsContext.tsx` — extract: polling interval, RTK Query hook name, error conversion pattern.
3. Read `web-app/src/lib/contexts/ReviewQueueContext.tsx` — extract: state shape, backing hook, consumers.
4. Read `web-app/src/lib/contexts/CockpitActionsContext.ts` — list all 20 callback slots; note it is a prop-drilling proxy.
5. Read `web-app/src/lib/contexts/SessionVcsContext.tsx` — note it is the only sub-tree-scoped context.
6. Read `web-app/src/lib/contexts/NavigationContext.tsx` — extract localStorage persistence pattern.
7. Read `web-app/src/lib/contexts/NotificationContext.tsx` — count methods, note junction with `SessionServiceContext`.
8. Read `web-app/src/lib/contexts/OmnibarContext.tsx` — identify line 45 `useSessionService` instantiation. Note the component/context coupling (imports `Omnibar` component inside provider).
9. Read `web-app/src/lib/contexts/AuthContext.tsx` — extract `enabled` guard pattern used by all data hooks.
10. Read `web-app/src/store/` (or equivalent Redux files) — enumerate slices (`sessionsSlice`, `reviewQueueSlice`, others).
11. Write report section: "State Management Patterns" — table of all 9 contexts, their backing store, scope, and classification (canonical/tolerated/violation).

---

### Story 1.2 — Data Fetching Pattern Inventory

**Priority:** P1 | **Effort:** M

Document every data fetching variant in the 22 API-calling hooks.

**Known variants from research:**
- Pattern A: Redux-backed ConnectRPC with manual dispatch (`useSessionService`, `useReviewQueue`)
- Pattern B: RTK Query polling with error shape conversion (`useApprovals`)
- Pattern C: Direct ConnectRPC client per hook, no shared transport (`useSessionVcs`, `useVcsStatus`, `useBranchSuggestions`, etc.)
- Pattern D: WebSocket streaming with custom transport (`useTerminalStream`, `useTerminalSnapshot`, `useLiveTail`)
- Pattern E: Local pagination via `useState` + `loadMore` (`useNotificationHistory`, `useSearchHistory`, `usePathHistory`)

**Tasks:**
1. Read `web-app/src/lib/hooks/useSessionService.ts` — extract: transport creation, Redux dispatch calls, error handling pattern, the three `catch` blocks. Note 646-line length.
2. Read `web-app/src/lib/hooks/useApprovals.ts` — extract: RTK Query call, pollingInterval value, error conversion `useMemo` block.
3. Read `web-app/src/lib/hooks/useReviewQueue.ts` — extract: Redux dispatch pattern, fallback polling logic.
4. Read `web-app/src/lib/hooks/useSessionVcs.ts` and `web-app/src/lib/hooks/useVcsStatus.ts` — extract: `createClient` call site, transport creation, returned shape.
5. Read `web-app/src/lib/hooks/useTerminalStream.ts` — extract: `createWatchTransport` usage.
6. Read `web-app/src/lib/hooks/useNotificationHistory.ts` — extract pagination state and `loadMore` pattern.
7. Read `web-app/src/lib/hooks/usePathCompletions.ts` — confirm pattern C and error-silencing behavior (returns empty array, no exposed error).
8. Write report section: "Data Fetching Patterns" — table of 5 patterns with canonical example, error handling approach, loading state approach, and verdict.

---

### Story 1.3 — CSS / Token Pattern Inventory

**Priority:** P1 | **Effort:** M

Document CSS patterns across all 115 `.css.ts` files, focusing on token usage compliance.

**Known variants from research:**
- Fully compliant: `vars.*` tokens only, `recipe()` for variants (target pattern per ADR-009)
- Partially compliant: `vars.*` tokens mixed with hardcoded `px` values
- Non-compliant: hardcoded hex color values in `.css.ts` files (confirmed violations: `ApprovalAnalyticsPanel.css.ts`, `app/debug/escape-codes/page.css.ts`)
- Borderline: header dark overrides with inline CSS variable overrides (WCAG justification)

**Tasks:**
1. Read `web-app/src/styles/theme-contract.css.ts` — enumerate all token groups and count. Document the 4 intentional gaps (chart colors, line-height, animation timing, zIndex/breakpoints as constants).
2. Read `web-app/src/styles/theme.css.ts` — confirm two-theme structure, note terminal color hardcoding (intentional per inline comment).
3. Read `web-app/src/components/sessions/ApprovalAnalyticsPanel.css.ts` — extract the 3 hardcoded hex violations at lines 300, 315, 320.
4. Read `web-app/src/app/debug/escape-codes/page.css.ts` — extract the 11 escape-code badge hex violations (lines 189–199).
5. Read `web-app/src/components/layout/Header.css.ts` — examine lines 22–23 and 163; classify the dark override as borderline/intentional.
6. Read `web-app/src/components/sessions/SessionCard.css.ts` — note as the largest CSS file (808 lines) with most hardcoded px values; sample 5–10 instances.
7. Read `web-app/src/components/ui/NotificationPanel.css.ts` — note as second largest (710 lines); sample px values.
8. Read `web-app/.eslintrc.json` — extract current no-restricted-syntax rules; confirm the hex-in-JSX rule and note its gap (does not cover `.css.ts` files).
9. Write report section: "CSS Pattern Inventory" — table of compliance tiers, file counts per tier, and specific violation locations.

---

### Story 1.4 — Component Structure Pattern Inventory

**Priority:** P1 | **Effort:** M

Document component structure patterns: size distribution, data/presentation split, and coupling patterns.

**Known variants from research:**
- Pure presentation (all data via props): `SessionCard.tsx` (22 props, 19 callbacks)
- Mixed responsibility (data + logic + render): `SessionDetail.tsx`, `SessionList.tsx`, `ReviewQueuePanel.tsx`
- Feature shell (routes state from context into sub-components): `CockpitShell` pattern
- Context boundary violation: `OmnibarContext` importing and rendering `Omnibar` component inside the provider

**Tasks:**
1. Read `web-app/src/components/sessions/SessionDetail.tsx` lines 1–100 — extract props interface (lines 39–54), `useSessionActions` and `useAppSelector` call sites (line 29), tab state management, fullscreen flag, modal open states.
2. Read `web-app/src/components/sessions/SessionCard.tsx` lines 86–115 — extract full props interface; count data props vs. callback props.
3. Read `web-app/src/components/sessions/ReviewQueuePanel.tsx` lines 1–50 and 200–220 — extract context imports (lines 6, 212) to confirm data + presentation mix.
4. Read `web-app/src/components/sessions/SessionList.tsx` lines 1–50 — extract context consumption pattern.
5. Read `web-app/src/components/sessions/SessionWizard.tsx` lines 1–50 — establish whether it duplicates or complements `Omnibar.tsx` creation flow.
6. Read `web-app/src/components/sessions/Omnibar.tsx` lines 42–80 — extract `OmnibarFormState` and `OmnibarUIState` definitions; note the two-mode structure.
7. Read `web-app/src/components/sessions/ApprovalNavBadge.tsx`, `ReviewQueueNavBadge.tsx`, `NotificationsNavBadge.tsx` — confirm ~20-line identical-pattern nav badge components.
8. Read `web-app/src/components/sessions/ApprovalPanel.tsx` and `ApprovalDrawer.tsx` lines 1–40 — extract hook call and scope difference; confirm ~200-line JSX duplication.
9. Write report section: "Component Structure Patterns" — categorize by pattern, list top-10 by line count, note the two active session creation paths.

---

## Epic 2: Duplication Map

**Goal:** Produce a precise map of duplicated or near-identical code with exact file paths, line numbers, similarity scores, and duplication type (logic/JSX/pattern).

**Priority:** P1  
**Effort:** M

**Acceptance Criterion:** Report section "Duplication Map" lists at least 8 specific duplication instances, each with: both file paths, relevant line ranges, estimated duplicate LOC, and duplication type classification.

---

### Story 2.1 — Near-Duplicate Component Map

**Priority:** P1 | **Effort:** S

Document JSX-level component duplication.

**Known instances from research:**
1. `ApprovalNavBadge.tsx` / `ReviewQueueNavBadge.tsx` / `NotificationsNavBadge.tsx` — ~20 lines each, identical badge rendering, differ only in context hook and count field (~60 LOC total, ~40 duplicate)
2. `ApprovalPanel.tsx` / `ApprovalDrawer.tsx` — same approval list UI with approve/deny buttons, ~200 lines duplicated

**Tasks:**
1. Read full text of `ApprovalNavBadge.tsx`, `ReviewQueueNavBadge.tsx`, `NotificationsNavBadge.tsx` — extract JSX structure side-by-side; identify exactly which lines differ (hook name + count field).
2. Read `web-app/src/components/sessions/ApprovalPanel.tsx` and `ApprovalDrawer.tsx` in full — map sections that differ (scope: `sessionId` filter vs. all-sessions) and sections that are identical (approval item render, action buttons).
3. Confirm whether `SessionWizard.tsx` and `Omnibar.tsx` + `OmnibarCreationPanel.tsx` duplicate session creation form logic or serve distinct purposes — read the first 100 lines of each for intent.
4. Write "Near-Duplicate Components" subsection with a table: Component A | Component B | Duplicate LOC | Difference | Proposed canonical form.

---

### Story 2.2 — Duplicated Hook / Context Logic Map

**Priority:** P1 | **Effort:** S

Document logic-level duplication in hooks and contexts.

**Known instances from research:**
1. `useApprovals.ts` and `ApprovalsContext.tsx` — functionally identical RTK Query wrappers; both start 5-second polling; identical error conversion block
2. `useSessionService` instantiated 3× (`SessionServiceContext`, `OmnibarContext`, `useSessionActions`) — all backed by same Redux slice but each runs independent hook initialization
3. Error conversion pattern repeated across `useApprovals.ts` and `ApprovalsContext.tsx` — no shared helper
4. `{ loading: boolean; error: Error | null }` return shape constructed differently in ~8 hooks

**Tasks:**
1. Extract the exact duplicate error-conversion code block from `useApprovals.ts` (useMemo version) and `ApprovalsContext.tsx` (inline version) — show side-by-side with line numbers.
2. Read `web-app/src/lib/hooks/useSessionActions.ts` (or equivalent) — confirm it is a third instantiation of `useSessionService`; identify which methods it re-wraps.
3. Scan `useSessionVcs.ts`, `useVcsStatus.ts`, `useBranchSuggestions.ts`, `useWorktreeSuggestions.ts` for `createConnectTransport` call — confirm each creates its own transport instance; extract and compare the transport config options to identify what, if anything, differs.
4. Write "Duplicated Hook / Context Logic" subsection with: duplication instance, file A, file B, duplicate code excerpt, LOC estimate, consequence (e.g., double polling, redundant hook init).

---

## Epic 3: Consolidation Opportunities

**Goal:** Produce a prioritized list of at least 8 specific consolidation candidates, each with a clear description of what to consolidate, what the canonical form should be, and an estimated line-count reduction.

**Priority:** P1  
**Effort:** M

**Acceptance Criterion:** Report section "Consolidation Opportunities" lists ≥8 candidates ranked by impact×(1/risk). Each entry states: opportunity name, files affected, estimated LOC reduction, canonical form description, risk level (S/M/H), and dependency on any registry checklist (omnibar, session creation 7-touchpoints).

**Note on registry constraint:** Opportunities touching `Omnibar.tsx`, `OmnibarCreationPanel.tsx`, `OmnibarContext.tsx`, `useSessionService.ts`, or session type enums must flag the 7-touchpoint session creation registry and/or omnibar action registry as a prerequisite for any future implementation.

---

### Story 3.1 — Rank Consolidation Candidates by Impact × (1/Risk)

**Priority:** P1 | **Effort:** M

Synthesize research findings into a ranked list. The 8 specific opportunities to document (derived from research):

**Opportunity 1 — Unify NavBadge trio into `<NavBadge>` primitive**
- Files: `ApprovalNavBadge.tsx`, `ReviewQueueNavBadge.tsx`, `NotificationsNavBadge.tsx`
- Canonical form: single `<NavBadge count={n} />` component; callers pass count from their own context hook
- LOC reduction: ~40 lines (3 × ~20 lines → 1 × ~20 lines)
- Risk: S (pure presentational, no registry dependency)
- Impact: M (removes duplication, enforces consistent badge appearance)

**Opportunity 2 — Merge `useApprovals` + `ApprovalsContext` into one singleton**
- Files: `lib/hooks/useApprovals.ts`, `lib/contexts/ApprovalsContext.tsx`
- Canonical form: single `ApprovalsContext` that accepts optional `sessionId` filter; `useApprovals` becomes a thin selector hook over the context — eliminates double polling
- LOC reduction: ~100 lines (103 + 67 → ~70)
- Risk: M (RTK Query subscription semantics must be preserved; `sessionId` filter edge case)
- Impact: H (eliminates two independent 5-second polling loops)

**Opportunity 3 — Extract shared RTK Query error conversion helper**
- Files: `lib/hooks/useApprovals.ts` (useMemo version), `lib/contexts/ApprovalsContext.tsx` (inline version), and any future hooks
- Canonical form: `lib/utils/rtkQueryError.ts` exporting `toErrorOrNull(queryError: unknown): Error | null`
- LOC reduction: ~15 lines saved immediately; prevents future drift
- Risk: S (pure utility extraction, no state changes)
- Impact: S (code quality, not functional)

**Opportunity 4 — Introduce `{ loading, error, data }` standard hook interface**
- Files: All ~8 hooks with `{ loading: boolean; error: Error | null }` return shapes
- Canonical form: shared type `HookResult<T> = { loading: boolean; error: Error | null; data: T }` in `lib/types/hooks.ts`; each hook adopts it
- LOC reduction: negligible initially; prevents structural divergence
- Risk: S (type-only change)
- Impact: M (establishes standard; enables generic loading/error UI components)

**Opportunity 5 — Shared ConnectRPC transport factory**
- Files: `useSessionVcs.ts`, `useVcsStatus.ts`, `useBranchSuggestions.ts`, `useWorktreeSuggestions.ts`, `usePathCompletions.ts`, `useRepositorySuggestions.ts`, `useFileService.ts` (7 hooks each calling `createConnectTransport`)
- Canonical form: `lib/transport.ts` exporting a singleton `getTransport()` factory; hooks call `getTransport()` instead of creating their own
- LOC reduction: ~70 lines (7 × ~10-line transport setup blocks)
- Risk: M (singleton transport must handle auth state correctly; ensure `AuthContext` `enabled` guard works with shared instance)
- Impact: M (reduces connection overhead, centralizes transport config)

**Opportunity 6 — Extract `CockpitActionsContext` into a proper actions registry**
- Files: `lib/contexts/CockpitActionsContext.ts`, `components/sessions/SessionList.tsx`, `components/sessions/SessionCard.tsx`
- Canonical form: replace the 20-slot callback bag with `useSessionServiceContext()` direct consumption in `SessionCard` and `SessionList`; `CockpitActionsContext` either shrinks to only the non-service callbacks or is eliminated
- LOC reduction: ~150 lines (20 props × plumbing at 3 levels)
- Risk: H (touches 7-touchpoint session creation registry; `SessionCard` currently has 22 props; refactor requires verifying all 20 action callback sources)
- Impact: H (eliminates the largest prop-drilling proxy; improves component isolation)

**Opportunity 7 — Split `SessionDetail.tsx` into data and presentation layers**
- Files: `components/sessions/SessionDetail.tsx` (1,132 lines)
- Canonical form: `SessionDetailShell.tsx` (data fetching: `useSessionActions`, `useAppSelector`) + `SessionDetailView.tsx` (pure presentation, accepts typed props); tab sub-components extracted to `SessionDetailTabs/`
- LOC reduction: no net reduction initially, but enables testability; prevents further growth
- Risk: H (large refactor; must not break tab routing; no registry dependency but high coupling)
- Impact: H (largest mixed-responsibility component; currently untestable in isolation)

**Opportunity 8 — Resolve `SessionWizard.tsx` vs. `Omnibar.tsx` parallel creation paths**
- Files: `components/sessions/SessionWizard.tsx` (912 lines), `components/sessions/Omnibar.tsx` (1,146 lines), `components/sessions/OmnibarCreationPanel.tsx` (725 lines)
- Decision needed: are these two paths intentional (wizard for onboarding, omnibar for power users) or is one superseded? If superseded, the session wizard represents ~912 deletable lines.
- Canonical form: document the intent; if both are kept, extract shared `SessionTypeSelector` and `PathInput` primitives from the overlapping form logic
- LOC reduction: up to ~912 lines if wizard is deprecated; ~200 lines in shared extraction if both kept
- Risk: H (7-touchpoint session creation registry applies to any structural change; omnibar action registry applies)
- Impact: H (if superseded, largest single deletion opportunity in the codebase)

**Tasks:**
1. Read `web-app/src/components/sessions/SessionWizard.tsx` lines 1–60 and lines 880–912 — determine if there are route-level imports or navigation links pointing to it; determine if it has active tests.
2. Read `web-app/src/app/` directory structure — find any page that renders `SessionWizard` to determine active/deprecated status.
3. Read `web-app/src/lib/contexts/CockpitActionsContext.ts` in full — confirm all 20 slots; check whether any slots are not available via `useSessionServiceContext` directly.
4. Write "Consolidation Opportunities" section: ranked table (highest impact×lowest risk first), with all 8 entries fully specified.

---

## Epic 4: Tooling Recommendations

**Goal:** Produce actionable tooling recommendations with specific install commands, rule configurations, and CI integration points.

**Priority:** P2  
**Effort:** M

**Acceptance Criterion:** Report section "Tooling Recommendations" covers ≥5 specific tools/rules with: tool name, install command, config snippet, what it enforces, CI integration point, and whether it addresses a gap identified in research.

---

### Story 4.1 — Audit Current ESLint/TypeScript Config Against Research Gaps

**Priority:** P2 | **Effort:** S

Confirm the gaps and produce a gap table before recommending fixes.

**Known gaps from research:**
1. No ESLint rule for hardcoded hex/px inside `.css.ts` files (only JSX `style` prop is covered)
2. No `@typescript-eslint/strict` ruleset (only `next/core-web-vitals` baseline)
3. `stylelint` is installed but does not run against `.css.ts` files
4. No prop-count lint rule (no enforcement against 22-prop components)
5. No duplicate-code detection tool in CI
6. No import-order enforcement beyond `boundaries`

**Tasks:**
1. Read `web-app/.eslintrc.json` in full — extract all rules, plugins, and extends; produce a gap table.
2. Read `web-app/tsconfig.json` — check for `strict`, `strictNullChecks`, `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`.
3. Read `web-app/package.json` — list all installed ESLint plugins and their versions; confirm `stylelint` version.
4. Write gap table: Current Rule | What It Covers | What It Misses.

---

### Story 4.2 — Produce Specific Tooling Recommendations

**Priority:** P2 | **Effort:** M

**Recommendations to document (derived from research):**

**Recommendation 1 — Custom ESLint rule: no-hardcoded-css-values-in-css-ts**
- Gap addressed: hex colors in `ApprovalAnalyticsPanel.css.ts` and `page.css.ts` not caught by current rules
- Approach: custom ESLint plugin or `no-restricted-syntax` AST selector targeting string literals matching `/#[0-9a-fA-F]{3,6}/` inside `.css.ts` files
- Config snippet:
  ```json
  {
    "no-restricted-syntax": [
      "error",
      {
        "selector": "Literal[value=/^#[0-9a-fA-F]{3,8}$/]",
        "message": "Hardcoded hex colors are not allowed in .css.ts files. Use vars.color.* tokens."
      }
    ]
  }
  ```
- Apply via `.eslintrc` override for `**/*.css.ts` files
- CI integration: already runs `eslint` in CI; add override

**Recommendation 2 — `@typescript-eslint/strict` ruleset**
- Gap addressed: no strict TypeScript lint rules beyond `next/core-web-vitals`
- Install: already installed (peer dep of `next`); enable via extends
- Config snippet: add `"plugin:@typescript-eslint/strict"` to `extends` in `.eslintrc.json`
- Key rules gained: `no-unnecessary-type-assertion`, `prefer-nullish-coalescing`, `no-non-null-assertion`, `consistent-type-imports`
- Risk: will surface existing violations — recommend running with `--no-fix` first to assess scope before enabling in CI as errors

**Recommendation 3 — `eslint-plugin-react` prop-count enforcement**
- Gap addressed: `SessionCard.tsx` has 22 props; no current enforcement
- Install: `npm install --save-dev eslint-plugin-react` (may already be installed via `next`)
- Rule: `react/no-multi-comp` and custom rule or `max-props-per-component` via `eslint-plugin-react-perf` — or a custom `no-restricted-syntax` rule counting `JSXOpeningElement` prop count
- Practical limit: warn at 12 props, error at 20 props
- Note: This is a warning-tier recommendation; enforcing as CI error risks too many existing violations

**Recommendation 4 — `jscpd` (duplicate code detector) as pre-merge check**
- Gap addressed: no duplicate code detection in CI
- Install: `npm install --save-dev jscpd`
- Config `.jscpd.json`: threshold 5%, min-lines 10, format `["typescript", "tsx"]`, ignore `gen/`
- CI integration: add `npx jscpd web-app/src --reporters "json,console" --threshold 5` as a separate lint step; report only (no block) initially
- Specific value: would surface the `useApprovals`/`ApprovalsContext` duplication and the nav badge trio

**Recommendation 5 — Token coverage report script**
- Gap addressed: no report of how many `.css.ts` files use `vars.*` tokens vs. hardcoded values
- Approach: not a lint plugin but a `scripts/audit-css-tokens.ts` script that:
  1. Counts `.css.ts` files
  2. Greps for `vars\.` usage vs. `"#[0-9a-fA-F]"` and `"[0-9]+px"` literals
  3. Outputs a CSV: file | vars_token_count | hardcoded_hex_count | hardcoded_px_count
- Run as: `npx ts-node scripts/audit-css-tokens.ts > reports/css-token-coverage.csv`
- CI integration: run in PR workflow; post summary as PR comment via `gh` CLI

**Recommendation 6 — `eslint-plugin-boundaries` extension: intra-`sessions/` layer split**
- Gap addressed: `eslint-plugin-boundaries` enforces cross-layer imports but not data-fetching vs. presentation split inside `components/sessions/`
- Approach: introduce sub-element types `sessions-data` and `sessions-ui`; enforce that `sessions-ui` components cannot import from context directly (must receive data as props)
- Install: no new package; extend existing `eslint-plugin-boundaries` config
- Risk: M — requires reclassifying existing files; will surface many violations initially
- Note: ADR-level decision needed before implementing (see ADR flags section)

**Tasks:**
1. Check `web-app/tsconfig.json` for current `strict` settings.
2. Confirm `eslint-plugin-react` version in `package.json`.
3. Confirm `jscpd` is not already installed.
4. Write "Tooling Recommendations" section with all 6 recommendations, install commands, config snippets, and CI integration points.

---

## Epic 5: Architecture Boundary Diagram

**Goal:** Document the actual layer model — what layers exist, what violations occur, and where — and produce a Mermaid diagram of the real dependency flow.

**Priority:** P2  
**Effort:** S

**Acceptance Criterion:** Report section "Architecture Boundary Diagram" contains (a) a prose description of the intended layer model from `eslint-plugin-boundaries` config, (b) a list of known violations with file-level evidence, and (c) at least one Mermaid diagram showing actual component/context dependency flow for the sessions domain.

---

### Story 5.1 — Document Actual Layer Model and Violations

**Priority:** P2 | **Effort:** S

**Tasks:**
1. Read `web-app/.eslintrc.json` — extract the full `eslint-plugin-boundaries` element type list and allowed-import matrix.
2. Read `web-app/src/lib/contexts/OmnibarContext.tsx` — confirm the boundary violation: context imports and renders `Omnibar` component; check whether `eslint-plugin-boundaries` currently allows or blocks this import.
3. Read `web-app/src/components/sessions/SessionDetail.tsx` line 29 — confirm direct `useAppSelector` call; classify as data/presentation boundary violation.
4. Read `web-app/src/components/sessions/ReviewQueuePanel.tsx` — confirm dual context consumption.
5. Produce layer model table: Layer | Allowed Imports | Known Violations | File Evidence.

---

### Story 5.2 — Produce Mermaid Dependency Diagram

**Priority:** P2 | **Effort:** S

**Tasks:**
1. From findings in all prior stories, draft a Mermaid diagram for the sessions domain showing:
   - App layout → provider chain (9 global contexts)
   - `CockpitShell` → `CockpitActionsContext` → `SessionList` → `SessionCard` flow
   - `SessionDetail` → `useSessionActions` → `SessionServiceContext` → `sessionsSlice` flow
   - `ReviewQueuePanel` → `ReviewQueueContext` + `ApprovalsContext` dual dependency
   - `OmnibarContext` ↔ `Omnibar` bidirectional coupling
2. Draft a second diagram for the data fetching layer showing the 3 `useSessionService` instantiation sites and their shared Redux store.
3. Write report section: "Architecture Boundary Diagram" — prose description + two Mermaid diagrams.

---

## Epic 6: Report Assembly

**Goal:** Assemble all section outputs into the final report document.

**Priority:** P1  
**Effort:** S

**Acceptance Criterion:** `docs/architecture/frontend-audit-2026.md` exists in the repository, contains all 5 sections from Epics 1–5, has a table of contents, and each consolidation opportunity links back to the relevant research file.

---

### Story 6.1 — Write Final Report

**Priority:** P1 | **Effort:** S

**Tasks:**
1. Create `docs/architecture/frontend-audit-2026.md`.
2. Write header: project context, audit date, scope, methodology.
3. Assemble section 1: Pattern Inventory (from Epic 1 story outputs).
4. Assemble section 2: Duplication Map (from Epic 2 story outputs).
5. Assemble section 3: Consolidation Opportunities (ranked table from Epic 3).
6. Assemble section 4: Tooling Recommendations (from Epic 4).
7. Assemble section 5: Architecture Boundary Diagram (Mermaid diagrams + prose from Epic 5).
8. Write executive summary (3–5 bullets: biggest wins, highest risks, recommended first action).
9. Add table of contents with section anchors.
10. Cross-reference: for each consolidation opportunity that touches the omnibar or session creation flow, add a callout block: "Implementation requires completing the 7-touchpoint session creation registry checklist (`.claude/rules/session-creation-registry.md`)."

---

## ADR Flags — Decisions Requiring ADR-Level Sign-Off

The following decisions surfaced during planning that are architectural in nature and require an ADR before any future implementation phase proceeds:

### ADR Flag 1 — Sessions State Ownership
**Decision needed:** Should `sessions[]` state live exclusively in `SessionServiceContext` (backed by Redux), or should the Redux layer be eliminated in favor of context-only state?  
**Context:** Three simultaneous `useSessionService` instances all write to the same Redux slice; the Redux layer adds indirection without clear benefit given no time-travel or devtools usage was documented.  
**Stakeholders:** frontend lead, anyone owning the ConnectRPC streaming subscription  
**Precedent:** `ReviewQueueContext` also uses Redux; a decision here affects both

### ADR Flag 2 — Intra-`sessions/` Layer Separation Enforcement
**Decision needed:** Should `eslint-plugin-boundaries` be extended with sub-element types `sessions-data` and `sessions-ui` to enforce the data-fetching vs. presentation split inside `components/sessions/`?  
**Context:** This would surface violations in `SessionDetail.tsx`, `SessionList.tsx`, `ReviewQueuePanel.tsx` immediately; enabling as CI errors requires a migration plan.  
**Risk:** All current violations would need to be fixed or explicitly suppressed before CI can enforce

### ADR Flag 3 — `SessionWizard.tsx` Deprecation vs. Parallel Path
**Decision needed:** Is `SessionWizard.tsx` (912 lines) an intentional parallel creation path (e.g., onboarding wizard) or a superseded implementation of the Omnibar creation flow?  
**Context:** Both implement session type selection and path input; the 7-touchpoint registry currently does not document which components are canonical for session creation.  
**Impact:** If deprecated, this is the largest single deletion opportunity (~912 lines + tests)

---

## Execution Notes

### File Reading Order (Dependency-Ordered)

Phase 1 (no dependencies): Epic 1 stories (1.1–1.4) — all read-only, can proceed in any order  
Phase 2 (depends on Phase 1 findings): Epic 2 stories (2.1–2.2) — compare files identified in Phase 1  
Phase 3 (depends on Phase 1+2): Epic 3 story (3.1) — ranking requires knowing all instances  
Phase 4 (parallel with Phase 3): Epic 4 stories (4.1–4.2) and Epic 5 stories (5.1–5.2) — independent of ranking  
Phase 5 (depends on all): Epic 6 — report assembly  

### Registry Constraints

Any consolidation opportunity that touches the following files requires the corresponding registry checklist:
- `Omnibar.tsx`, `OmnibarCreationPanel.tsx`, `OmnibarContext.tsx` → omnibar action registry (`.claude/rules/feature-testing-registry.md`)
- `OmnibarContext.tsx`, `useSessionService.ts`, session type enums → session creation registry (`.claude/rules/session-creation-registry.md`, 7 touchpoints)
- This plan is analysis only; these constraints apply to any future implementation phase

---

## Summary Table

| Epic | Stories | Tasks | Priority | Effort |
|---|---|---|---|---|
| Epic 1: Pattern Inventory | 4 | 36 | P1 | L |
| Epic 2: Duplication Map | 2 | 8 | P1 | M |
| Epic 3: Consolidation Opportunities | 1 | 4 | P1 | M |
| Epic 4: Tooling Recommendations | 2 | 7 | P2 | M |
| Epic 5: Architecture Boundary Diagram | 2 | 7 | P2 | S |
| Epic 6: Report Assembly | 1 | 10 | P1 | S |
| **Total** | **12** | **72** | — | — |

**ADR flags raised:** 3
