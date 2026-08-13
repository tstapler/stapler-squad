# Frontend Architecture Audit — Validation Plan

**Deliverable being validated:** `docs/architecture/frontend-audit-2026.md`  
**Date:** 2026-05-08  
**Validator role:** QA Architect  
**Source documents:** `requirements.md` (5 success criteria), `implementation/plan.md` (6 epics, 12 stories, 72 tasks)

---

## How to Use This Document

Each row in the tables below maps one **requirement** to one or more **test cases**. For each test case:

- **Pass criterion** — the minimum observable condition that constitutes a pass
- **Verification method** — exactly how a reviewer confirms it (manual file open, search, count, render)

A test case with no verdict recorded should be treated as **UNTESTED** until a reviewer completes it and records PASS or FAIL.

---

## Section 1 — Pattern Inventory Completeness

*Requirements source: Success Criterion 1 — "Catalog of every distinct pattern used for state, data fetching, CSS, and component structure — with file-level examples of each variant."*  
*Plan source: Epic 1 (Stories 1.1–1.4), acceptance criterion: "covers all 4 domains; each domain lists every known variant with at least 2 file-level examples and a one-sentence verdict."*

| # | Requirement | Test Case | Pass Criterion | Verification Method | Verdict |
|---|---|---|---|---|---|
| TC-INV-01 | State management domain covered | Report contains a "State Management Patterns" section | Section heading exists; table lists ≥6 variants matching plan story 1.1 known variants | Open `frontend-audit-2026.md`; search for heading "State Management" | — |
| TC-INV-02 | State management: ≥2 file examples per variant | Each of the 6 state variants lists ≥2 concrete file paths | At least 12 distinct `web-app/src/` file paths appear in this section | Count file path references (e.g. `*.tsx`, `*.ts`) in the state management table | — |
| TC-INV-03 | State management: verdict per variant | Each variant row has a classification | Every row in the table contains one of: "canonical", "tolerated", or "violation/deprecated" | Scan table column "Classification" or equivalent | — |
| TC-INV-04 | Data fetching domain covered | Report contains a "Data Fetching Patterns" section | Section heading exists; table lists ≥5 patterns matching plan story 1.2 known variants (A–E) | Open report; search for heading "Data Fetching" | — |
| TC-INV-05 | Data fetching: ≥2 file examples per pattern | Each of the 5 patterns lists ≥2 concrete file paths | At least 10 distinct file paths appear in this section | Count file path references in the data fetching table | — |
| TC-INV-06 | Data fetching: error handling and loading state documented per pattern | Each pattern row documents how errors and loading states are handled | Every pattern row has non-empty "Error Handling" and "Loading State" columns (or inline prose) | Inspect each pattern row for presence of error/loading description | — |
| TC-INV-07 | CSS/styling domain covered | Report contains a "CSS Pattern Inventory" section | Section heading exists; lists ≥4 compliance tiers and at least one file count per tier | Open report; search for heading "CSS" | — |
| TC-INV-08 | CSS: ≥2 file examples per tier | Each compliance tier lists ≥2 file paths | Non-compliant tier lists at least `ApprovalAnalyticsPanel.css.ts` and `app/debug/escape-codes/page.css.ts`; compliant tier lists ≥2 files | Open those files and confirm they exist at the stated paths | — |
| TC-INV-09 | CSS: token coverage verdict per tier | Each tier has a classification or verdict | Each tier row describes what the violation is (or why it is canonical) | Scan tier descriptions | — |
| TC-INV-10 | Component structure domain covered | Report contains a "Component Structure Patterns" section | Section heading exists; categorizes components by pattern type (pure/mixed/feature-shell/violation) | Open report; search for heading "Component Structure" | — |
| TC-INV-11 | Component structure: ≥2 file examples per variant | Each of the ≥4 structural pattern types lists ≥2 concrete file paths | At least 8 distinct component file paths appear in this section | Count file path references in the component structure section | — |
| TC-INV-12 | All 4 domains present | Report has all four inventory domains in one document | Headings (or subsections) for all 4 domains exist within the same `frontend-audit-2026.md` file | Search for all four headings in sequence | — |
| TC-INV-13 | Top-10 components by line count listed | Plan story 1.4 requires "top-10 by line count" | A list or table of ≥10 components ordered by LOC appears in the component structure section | Verify ordering is consistent with known sizes: `Omnibar.tsx` 1,146 LOC; `SessionDetail.tsx` 1,132 LOC; `SessionWizard.tsx` 912 LOC | — |

**Domain sub-total: 13 test cases**

---

## Section 2 — Duplication Map Accuracy

*Requirements source: Success Criterion 2 — "Specific instances of duplicated logic or near-identical components that should be unified, with file paths and line numbers."*  
*Plan source: Epic 2 (Stories 2.1–2.2), acceptance criterion: "≥8 specific duplication instances, each with both file paths, relevant line ranges, estimated duplicate LOC, and duplication type."*

| # | Requirement | Test Case | Pass Criterion | Verification Method | Verdict |
|---|---|---|---|---|---|
| TC-DUP-01 | ≥8 duplication instances documented | Duplication Map section contains a table with ≥8 rows | Count rows in the duplication table | Search report section "Duplication Map"; count table rows | — |
| TC-DUP-02 | NavBadge trio verifiable | The three nav badge files are listed with line ranges | Report lists `ApprovalNavBadge.tsx`, `ReviewQueueNavBadge.tsx`, `NotificationsNavBadge.tsx` with specific line ranges | Open each file at the stated line ranges and confirm JSX structure is identical except for hook name and count field | — |
| TC-DUP-03 | ApprovalPanel / ApprovalDrawer duplication verifiable | Both files listed with their overlapping line ranges | Report lists `ApprovalPanel.tsx` and `ApprovalDrawer.tsx` with ≥1 specific line range pair for the duplicated approval item render | Open both files at stated lines; confirm render structure matches | — |
| TC-DUP-04 | Error conversion duplication verifiable | `useApprovals.ts` and `ApprovalsContext.tsx` error-conversion blocks listed with line numbers | Report provides a side-by-side code excerpt or exact line numbers for both | Open `lib/hooks/useApprovals.ts` and `lib/contexts/ApprovalsContext.tsx` at stated lines; confirm duplicate code block | — |
| TC-DUP-05 | Triple `useSessionService` instantiation documented | Three instantiation sites listed | Report names all three call sites: `SessionServiceContext`, `OmnibarContext`, and the third hook (e.g. `useSessionActions`) with file paths | Open each file and confirm `useSessionService` is called at top level in each | — |
| TC-DUP-06 | ConnectRPC transport duplication documented | ≥5 of the 7 hooks that create their own transport are listed | Report lists `useSessionVcs.ts`, `useVcsStatus.ts`, `useBranchSuggestions.ts`, `useWorktreeSuggestions.ts`, `usePathCompletions.ts` with file paths | Open any two of the listed files; confirm both contain a `createConnectTransport` call | — |
| TC-DUP-07 | Each duplication instance has estimated duplicate LOC | Every row in the duplication table has a numeric LOC estimate | No row has an empty LOC column | Scan the LOC column of the duplication table | — |
| TC-DUP-08 | Each duplication instance has a duplication type | Every row classifies the duplication as logic / JSX / pattern | No row has an empty "Type" column | Scan the Type column of the duplication table | — |
| TC-DUP-09 | SessionWizard vs. Omnibar relationship documented | Report addresses whether these two paths duplicate session creation logic | A subsection or note explicitly states whether `SessionWizard.tsx` and `Omnibar.tsx` overlap in creation form logic | Open report; search for "SessionWizard" in duplication or component structure section | — |

**Domain sub-total: 9 test cases**

---

## Section 3 — Consolidation Opportunity Actionability

*Requirements source: Success Criterion 3 — "Prioritized list of refactors ranked by impact/risk, each with: what to consolidate, why, what the canonical form should be."*  
*Plan source: Epic 3 (Story 3.1), acceptance criterion: "≥8 candidates ranked by impact×(1/risk); each entry states: opportunity name, files affected, estimated LOC reduction, canonical form description, risk level, and registry dependency."*

| # | Requirement | Test Case | Pass Criterion | Verification Method | Verdict |
|---|---|---|---|---|---|
| TC-CON-01 | ≥8 consolidation opportunities listed | Table or numbered list with ≥8 entries | Count entries in the Consolidation Opportunities section | Open report; count opportunity rows | — |
| TC-CON-02 | Each opportunity states what to consolidate | No opportunity row has an empty "What" column | Every entry names the specific component(s), hook(s), or pattern to consolidate | Scan each entry for a subject clause naming the artifact(s) | — |
| TC-CON-03 | Each opportunity states canonical form | Every entry describes the target state after consolidation | Each entry contains a sentence beginning with or equivalent to "Canonical form:" or "After refactor:" | Inspect each opportunity entry for a canonical form description | — |
| TC-CON-04 | Each opportunity lists specific file paths | Every entry names ≥1 file path (absolute or repo-relative) | File paths are of the form `web-app/src/...` or `lib/...` | Verify by opening the two files with the highest LOC reduction (Opp 6 and Opp 7) and confirming they exist | — |
| TC-CON-05 | Each opportunity has estimated LOC reduction | Numeric estimate in every entry | No entry has "unknown" or blank for LOC reduction | Scan LOC column; verify Opp 1 ≈ 40, Opp 2 ≈ 100, Opp 5 ≈ 70, Opp 7 "no net reduction" noted | — |
| TC-CON-06 | Opportunities ranked by impact × (1/risk) | The table is ordered from highest to lowest priority score | The NavBadge unification (Risk S, Impact M) does not rank below CockpitActionsContext (Risk H); `useApprovals`/`ApprovalsContext` merge (Risk M, Impact H) ranks in the top 3 | Compare ranking order against risk/impact values in each row | — |
| TC-CON-07 | Risk level assigned per opportunity | Every entry has a risk label | All 8 entries have S, M, or H in a "Risk" column | Scan Risk column | — |
| TC-CON-08 | Registry dependency flagged where applicable | Opportunities touching omnibar or session creation flow carry a registry callout | Opp 6 (CockpitActionsContext), Opp 7 (SessionDetail), and Opp 8 (SessionWizard/Omnibar) each contain a reference to the session creation registry and/or omnibar action registry | Search for "7-touchpoint" or "session-creation-registry" or "omnibar action registry" near each of those three opportunities | — |
| TC-CON-09 | ADR flags documented | The three decisions requiring ADR sign-off are listed | Report contains a section or callout noting ADR flags for: (1) sessions state ownership, (2) intra-sessions/ layer separation, (3) SessionWizard deprecation | Search for "ADR" in the report; confirm all three flags appear | — |

**Domain sub-total: 9 test cases**

---

## Section 4 — Tooling Recommendation Verifiability

*Requirements source: Success Criterion 4 — "ESLint plugins, TypeScript strict settings, or custom rules that would automate ongoing enforcement of the chosen patterns."*  
*Plan source: Epic 4 (Stories 4.1–4.2), acceptance criterion: "≥5 specific tools/rules with: tool name, install command, config snippet, what it enforces, CI integration point, and whether it addresses a gap identified in research."*

| # | Requirement | Test Case | Pass Criterion | Verification Method | Verdict |
|---|---|---|---|---|---|
| TC-TOOL-01 | ≥5 tooling recommendations present | Report section "Tooling Recommendations" contains ≥5 entries | Count numbered or tabulated recommendations | Open report; count recommendation entries | — |
| TC-TOOL-02 | Each recommendation has a package name | Every entry names the npm package or ESLint rule | No entry refers to a tool without naming it (e.g. "some ESLint plugin") | Verify: Rec 1 has `no-restricted-syntax`, Rec 2 has `@typescript-eslint/strict`, Rec 4 has `jscpd` | — |
| TC-TOOL-03 | Each recommendation has an install command | Every entry that requires installation provides `npm install ...` or equivalent | Entries for Rec 2, 3, 4 provide install commands; Rec 1 and 5 note no new package needed | Check each entry for presence of an install command or explicit "no install needed" note | — |
| TC-TOOL-04 | Each recommendation has an example rule config | Every entry includes a JSON/YAML/TS config snippet | Rec 1 includes `no-restricted-syntax` JSON snippet; Rec 4 includes `.jscpd.json` snippet | Copy the config snippet for Rec 1 into a scratch `.eslintrc.json` override for `*.css.ts`; confirm it parses as valid JSON | — |
| TC-TOOL-05 | Each recommendation states what violation it catches | Every entry names the specific violation it would have caught in the existing codebase | Rec 1 names `ApprovalAnalyticsPanel.css.ts` hex violations; Rec 4 names the `useApprovals`/`ApprovalsContext` duplication | For Rec 1: open `web-app/src/components/sessions/ApprovalAnalyticsPanel.css.ts` lines 300, 315, 320; confirm hex literals are present at those lines (simulates what the rule would catch) | — |
| TC-TOOL-06 | Each recommendation has a CI integration point | Every entry describes where in CI it runs | All entries reference one of: `eslint` step, separate lint step, PR workflow script | Scan each entry for a CI integration note | — |
| TC-TOOL-07 | Gap addressed column or prose present | Each recommendation traces to a documented gap | Each recommendation names the specific gap from Story 4.1 gap table that it closes | Open the gap table in the report; confirm each recommendation row has a pointer back to a gap | — |
| TC-TOOL-08 | TypeScript strict settings gap documented | Report notes that `strict` is not fully enabled and recommends enabling it | The gap table lists current `tsconfig.json` strict settings and what is missing | Open `web-app/tsconfig.json` and compare against the gap table in the report: verify `strict`, `strictNullChecks`, `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes` are correctly stated as present or absent | — |
| TC-TOOL-09 | Token coverage report script is actionable | Rec 5 token coverage script includes a run command and output format | Script path (`scripts/audit-css-tokens.ts`) and run command (`npx ts-node ...`) and output format (CSV columns) are all specified | Confirm the run command is syntactically valid by inspection; confirm CSV column names are specified | — |

**Domain sub-total: 9 test cases**

---

## Section 5 — Architecture Diagram Correctness

*Requirements source: Success Criterion 5 — "Which layers exist, which are violated, and where."*  
*Plan source: Epic 5 (Stories 5.1–5.2), acceptance criterion: "(a) prose description of intended layer model, (b) list of known violations with file-level evidence, (c) ≥1 Mermaid diagram."*

| # | Requirement | Test Case | Pass Criterion | Verification Method | Verdict |
|---|---|---|---|---|---|
| TC-DIAG-01 | Mermaid diagram present and renderable | Report contains ≥1 fenced `mermaid` code block | At least one ` ```mermaid ` block exists in the report | Search for ` ```mermaid ` in the raw markdown; paste the block into the Mermaid Live Editor at `https://mermaid.live` and confirm it renders without error | — |
| TC-DIAG-02 | Import path spot-check 1: CockpitShell → SessionList | Diagram shows `CockpitShell` → `CockpitActionsContext` → `SessionList` dependency chain | All three nodes appear in the diagram and are connected in the stated direction | Open `web-app/src/components/sessions/SessionList.tsx`; confirm it imports from `CockpitActionsContext` (or has a parent that does); compare to diagram edge | — |
| TC-DIAG-03 | Import path spot-check 2: OmnibarContext ↔ Omnibar | Diagram shows the bidirectional coupling between `OmnibarContext` and `Omnibar` | Both nodes appear; an edge or annotation notes the coupling (context renders component) | Open `web-app/src/lib/contexts/OmnibarContext.tsx`; search for an import of `Omnibar` component; confirm the diagram edge direction matches reality | — |
| TC-DIAG-04 | Import path spot-check 3: SessionDetail → Redux | Diagram shows `SessionDetail` → `useAppSelector` / `sessionsSlice` path | `SessionDetail` node appears; an edge connects it to Redux store or `useAppSelector` | Open `web-app/src/components/sessions/SessionDetail.tsx` line 29; confirm `useAppSelector` is called; confirm diagram reflects this | — |
| TC-DIAG-05 | Layer model prose description present | Report describes the intended layer model from `eslint-plugin-boundaries` config | Prose section names the element types (e.g. `components`, `lib/contexts`, `lib/hooks`, `app`, `styles`) and their allowed import directions | Read `web-app/.eslintrc.json` `boundaries` config; verify the layer names in the report match the `element types` defined there | — |
| TC-DIAG-06 | Known violations listed with file evidence | Report lists boundary violations with file:line evidence | At minimum three violations are documented: `OmnibarContext` importing `Omnibar`; `SessionDetail` direct Redux selector; `ReviewQueuePanel` dual context import | Open each named file at the stated line; confirm the import or call exists | — |
| TC-DIAG-07 | Second diagram (data fetching layer) present | Plan calls for a second diagram showing 3 `useSessionService` instantiation sites | A second Mermaid diagram or clearly separate diagram section documents the data fetching layer | Search for second ` ```mermaid ` block; confirm it shows at least `SessionServiceContext`, `OmnibarContext`, and the third instantiation site, all pointing to the same Redux store | — |

**Domain sub-total: 7 test cases**

---

## Section 6 — Requirements Coverage Fraction

*Meta-validation: What percentage of requirements.md success criteria does the plan address?*

| Success Criterion (from requirements.md) | Addressed in Plan? | Covering Epic(s) | Coverage Assessment |
|---|---|---|---|
| SC-1: Pattern inventory (4 domains, file-level examples, variant classification) | Yes | Epic 1 (Stories 1.1–1.4) | Full — all 4 domains have dedicated stories with specific file tasks and a verdict column requirement |
| SC-2: Duplication map (specific instances, file paths, line numbers) | Yes | Epic 2 (Stories 2.1–2.2) | Full — 8 named instances with both file paths and line-range extraction tasks |
| SC-3: Consolidation opportunities (prioritized, canonical form, what/why) | Yes | Epic 3 (Story 3.1) | Full — 8 ranked opportunities, each with canonical form, LOC estimate, risk rating, and registry dependency note |
| SC-4: Tooling recommendations (ESLint plugins, TS strict, custom rules) | Yes | Epic 4 (Stories 4.1–4.2) | Full — 6 recommendations (plan exceeds the ≥5 minimum), with install commands, config snippets, and CI points |
| SC-5: Architecture boundary diagram (layers, violations, where) | Yes | Epic 5 (Stories 5.1–5.2) | Full — prose layer model + two Mermaid diagrams + violations table with file evidence |

**Requirements coverage fraction: 5 / 5 = 100%**

All five success criteria from `requirements.md` are directly addressed by at least one epic in `plan.md`. No success criterion is unaddressed, partially addressed due to a missing story, or deferred to a future phase.

---

## Section 7 — Report Assembly Completeness

*Plan source: Epic 6 (Story 6.1), acceptance criterion: document exists, contains all 5 sections, has ToC, and opportunities link back to research.*

| # | Test Case | Pass Criterion | Verification Method | Verdict |
|---|---|---|---|---|
| TC-ASSY-01 | File exists at stated path | `docs/architecture/frontend-audit-2026.md` exists in the repository | `ls web-app/../docs/architecture/frontend-audit-2026.md` returns the file | Run `ls` or use file browser to confirm path | — |
| TC-ASSY-02 | Table of contents present | Report has a ToC with anchor links to all 5 main sections | A section titled "Contents" or "Table of Contents" appears before section 1; each entry links to an `#anchor` | Click each ToC link in a Markdown renderer; confirm each navigates to the correct section | — |
| TC-ASSY-03 | Executive summary present | Report has an executive summary (3–5 bullets) | A section or callout at the top named "Executive Summary" or "Summary" with 3–5 bullet points appears | Open report; search for "Executive Summary" or "Summary" heading near the top | — |
| TC-ASSY-04 | Scope and methodology stated | Header block names audit date, scope (web-app/src/), and out-of-scope items | Report header or intro paragraph names the scope boundary (`web-app/src/`, excluding `gen/`), audit date (2026-05-08 or similar), and methodology (read-only analysis) | Inspect header block | — |
| TC-ASSY-05 | Registry callout blocks present | Opportunities 6, 7, 8 carry callout blocks referencing `.claude/rules/session-creation-registry.md` | Search for "7-touchpoint" or "session-creation-registry" at least 3 times (once per affected opportunity) | `grep -c "session-creation-registry" docs/architecture/frontend-audit-2026.md` should return ≥3 | — |

**Domain sub-total: 5 test cases**

---

## Test Case Count Summary

| Category | Test Cases | Traceability |
|---|---|---|
| TC-INV (Pattern Inventory Completeness) | 13 | Epic 1 / SC-1 |
| TC-DUP (Duplication Map Accuracy) | 9 | Epic 2 / SC-2 |
| TC-CON (Consolidation Actionability) | 9 | Epic 3 / SC-3 |
| TC-TOOL (Tooling Verifiability) | 9 | Epic 4 / SC-4 |
| TC-DIAG (Architecture Diagram Correctness) | 7 | Epic 5 / SC-5 |
| TC-ASSY (Report Assembly Completeness) | 5 | Epic 6 |
| **Total** | **52** | — |

| Metric | Value |
|---|---|
| Requirements success criteria covered | 5 / 5 |
| Requirements coverage fraction | **100%** |
| Test cases requiring file-open verification | 31 (60%) |
| Test cases verifiable by text search alone | 13 (25%) |
| Test cases requiring external tool (Mermaid render, JSON parse) | 3 (6%) |
| Test cases that are count-based | 5 (10%) |

---

## Validation Execution Notes

### Pre-conditions

Before running any test case, confirm the deliverable file exists (TC-ASSY-01). All other test cases depend on it.

### High-priority spot-checks (run these first)

If time is limited, prioritize these 6 test cases — they each verify the most likely failure modes:

1. **TC-DUP-01** — ≥8 duplication instances (minimum count check)
2. **TC-CON-08** — Registry callout blocks present (most common omission in audit reports)
3. **TC-TOOL-04** — Config snippets parse as valid JSON (most likely to be broken)
4. **TC-DIAG-01** — Mermaid diagram renders without error (structural failure gate)
5. **TC-DIAG-03** — `OmnibarContext` ↔ `Omnibar` bidirectional coupling in diagram (the one confirmed boundary violation)
6. **TC-INV-02** — ≥2 file examples per state management variant (most common thin-coverage failure)

### Known risks to validation

- **TC-DUP-02 through TC-DUP-06**: Line number accuracy degrades if the report was written from cached research rather than live file reads. Spot-check at least one stated line number per duplication instance.
- **TC-CON-06** (ranking correctness): The plan prescribes a specific ordering; if the report re-orders without updating risk/impact scores, the ranking will appear inconsistent. Check that Risk H opportunities do not rank above Risk S opportunities with equal impact.
- **TC-DIAG-05** (layer model names match `.eslintrc.json`): The report may use colloquial layer names that differ from the exact `element types` in the eslint config. Accept minor naming differences (e.g. "contexts layer" for `lib/contexts`) but flag if a layer is missing entirely.
