# Implementation Plan: session-list-density

**Feature**: Increase legibility and information density of the sessions sidebar
(`SessionRow.tsx` list view, `SessionCard.tsx`/`BoardCard.tsx` board view) by
demoting low-value default columns, wrapping instead of ellipsis-truncating
names/paths, adding a segment-aware opaque-path collapse, and fixing two
accessibility gaps (truncated-text accessible names, tooltip-only data on
touch/keyboard).
**Date**: 2026-09-24
**Status**: Ready for implementation
**ADRs**: ADR-001 (`decisions/ADR-001-agent-memory-accessible-disclosure.md`)

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `SessionRow` | Existing React component rendering one session as a single-line-grid list row. | `web-app/src/components/sessions/SessionRow.tsx` |
| `SessionCard` | Existing React component rendering one session as a multi-line card; shared by List's card mode and Board view. | `web-app/src/components/sessions/SessionCard.tsx` |
| `BoardCard` | Thin (72-line) wrapper around `SessionCard` adding a drag handle + move menu. Not redesigned separately — fixing `SessionCard` fixes it. | `web-app/src/components/sessions/BoardCard.tsx` |
| `ColumnKey` | Existing string union (`"agent" \| "memory" \| "elapsed" \| "diff" \| "branch"`) identifying an optional row column. | `session-columns.ts` |
| `ColumnDef` | Existing metadata record (`key`, `label`, `gridWidth`, `defaultVisible`) per `ColumnKey`. | `session-columns.ts` |
| `visibleColumns` | The caller-supplied `ColumnKey[]` a `SessionRow` renders as optional grid cells. | Prop on `SessionRowProps` |
| `truncateWorkspacePath` | **New** pure function: `(path: string, maxLen: number) => string`. Segment-aware path truncation that never elides the leading semantic segment(s) or the trailing segment, and collapses only `OpaqueSegment`s. | New file, `web-app/src/lib/utils/truncateWorkspacePath.ts` |
| `SegmentKind` | **New** discriminated union `"semantic" \| "opaque"` returned by the segment classifier inside `truncateWorkspacePath`. | Internal to the new util |
| `classifySegment` | **New** internal helper: given a path segment string, returns its `SegmentKind`. | Internal to the new util |
| `OpaqueSegment` | A path segment (or a trailing run within one) that is a bare hex hash (`/^[0-9a-f]{7,40}$/i`), a UUID, or ends in a `[-_][0-9a-f]{8,}$` run — machine-generated noise, safe to collapse. | Confirmed real examples: `research/features.md` (`6eb0b580fa0331d5`, `stapler-squad-wasted-space_18d807dfb97a2b28`) |
| `SemanticSegment` | Any segment that is not an `OpaqueSegment` — a repo/category/branch/human-slug name. Always preserved in full. | e.g. `backlog`, `pr-424-compute-nop` |
| `SegmentBudget` | The `maxLen` value a caller passes to `truncateWorkspacePath`: a single fixed constant per component (`ROW_PATH_MAX_LEN = 72`, `CARD_PATH_MAX_LEN`), not a live pixel measurement and not varied by container-query breakpoint (see Story 2.1.2's Resolution Note for why the row doesn't use a normal/narrow pair). | See Story 2.1.1 / 2.2.1 for the concrete constants |
| `RowContainerQuery` | The `containerType: "inline-size"` + `containerName: "sessionRow"` pair applied to `SessionRow`'s CSS root, copied verbatim from `FilesTab.css.ts`'s pattern. | `SessionRow.css.ts` |
| `CardContainerQuery` | Same pattern applied to `SessionCard`'s CSS root with `containerName: "sessionCard"`. | `SessionCard.css.ts` |
| `ElapsedSecondLine` | The hardcoded JSX block rendering the `elapsed` column as a second flex row beneath the name/path, instead of a grid cell. | `SessionRow.tsx` |
| `AccessibleDisclosure` | The focusable, keyboard/hover-triggerable UI pattern (the existing shared `Tooltip` component, per ADR-001) that replaces bare `title`-only tooltips for the `agent`/`memory` columns. | `web-app/src/components/ui/Tooltip.tsx` (Radix-backed) |
| `estimateSize` | Existing `@tanstack/react-virtual` config value (currently `50`) used as the virtualizer's first-paint row-height guess before `measureElement` corrects it. | `SessionList.tsx:646` |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| Overall architecture approach | Extend existing utilities/CSS via a seam; minimal, mechanical inline edits to `SessionRow`/`SessionCard` | Step 0.5 creative pass | (A) Full component refactor of `SessionRow`/`SessionCard` into subcomponents before adding density changes | Disproportionate for a Complexity-2, presentation-only feature; requirements.md's Appetite section explicitly rules out extending scope for exactly this kind of speculative restructuring |
| Overall architecture approach | (same as above) | Step 0.5 creative pass | (B) New interactive "breadcrumb" component with click-to-expand collapsed segments (GitHub-style) | Strongest UX per `ux.md` §1 prior art, but introduces new focus management / click-away / mobile-tap-target surface far beyond a text heuristic — exactly the "Rabbit Hole" requirements.md warns against |
| Path truncation | Pure function (`truncateWorkspacePath`), Transaction-Script-level simplicity | PoEAA; `architecture.md` §1 | GoF Strategy (`TruncationStrategy` interface with swappable implementations) | Only one algorithm is needed today; Strategy adds indirection with no second implementation to justify it |
| Segment classification | Type-driven discriminated union (`SegmentKind = "semantic" \| "opaque"`) returned from `classifySegment()` | type-driven-design skill | Boolean flag (`isOpaque: boolean`) threaded through call sites | A named union matches the repo's existing idiom (`ColumnKey`) and is clearer at each call site than a bare boolean |
| Column visibility model | Keep existing `ColumnDef`/boolean `defaultVisible`; `elapsed` skipped in `buildRowGridTemplate()`'s loop and hardcoded to a 2nd-line JSX render | `architecture.md` §2 | New `layout: "grid" \| "secondary-line"` field on `ColumnDef` | Over-engineered for one always-2nd-line column; a one-line loop skip + hardcoded JSX is sufficient and keeps `ColumnPicker.tsx` untouched |
| Narrow-layout switching | Native CSS Container Queries (`containerType`/`containerName`/`@container`), copied from `FilesTab.css.ts` | `stack.md`, `pitfalls.md` §2, `build-vs-buy.md` §2 | Viewport `@media` breakpoint | Requirements.md's own Alternatives Considered: the sidebar is a fixed ~280px column independent of viewport width, so a viewport breakpoint fires at the wrong times |
| Agent/memory accessible disclosure | Reuse existing shared `Tooltip` (Radix-backed, focus + hover triggered) + `tabIndex={0}`; fold agent/memory into row-level `aria-label` | ADR-001 | New bespoke `aria-describedby` + `role="tooltip"` component built from scratch | `Tooltip.tsx` already wraps Radix's native implementation of that exact pattern; a second implementation would duplicate it and risk the jscpd gate |
| Row-level accessible path name | Row's concatenated `aria-label` uses the full, untruncated `session.existingDir` (drop `abbreviatePath` call entirely, don't reuse `truncateWorkspacePath` there) | `ux.md` §3 (WCAG: accessible name should be the full string) | Route the row-level `aria-label` through `truncateWorkspacePath` (same function as the visible text) | The row-level label already concatenates several fields into one string with no visual-length constraint of its own; using the full path there is strictly more correct and costs nothing, whereas reusing the truncated form would still be "abbreviated," not "full" |
| Virtualizer variable-height rows | Tune `estimateSize`'s numeric constant only; keep the existing `measureElement` wiring | `build-vs-buy.md` §3, `pitfalls.md` §1 | Add a manual `.measure()` call or a custom height cache | `measureElement` + `ResizeObserver` already handles dynamic height correction; manual `.measure()` is a known gotcha (`TanStack/virtual#425`) not worth introducing |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `SessionRow.tsx` (483-line render body), `SessionCard.tsx` (1015-line render body), `SessionList.tsx` (recurring long-function/deep-nesting findings) | Pre-existing `[advisory]` complexity/length findings from `mcp__kibitzer__architecture_assessment` (`architecture.md` §5) — long functions with deep conditional nesting | **Isolate via seam** | New logic (`truncateWorkspacePath.ts`, new container-query CSS classes in `SessionRow.css.ts`/`SessionCard.css.ts`) is built as small, independently-testable units the existing components merely call. The unavoidable inline edits inside the two components (JSX restructuring for the 2nd line, column-visibility flips, class swaps) are narrow and mechanical, guided by the new units — not new business logic grafted into the long bodies. Refactor-first is disproportionate for a Complexity-2, presentation-only feature and is explicitly out of scope per requirements.md; Extend-as-is would add to the existing debt. Per `architecture.md` §6. |
| `session-columns.ts` | None — 38 lines, no kibitzer findings | **Extend as-is** | Already a small, clean, single-purpose module; the one-line `elapsed` loop-skip fits its existing shape without adding complexity. |

---

## Migration Plan

Not applicable — no schema or persisted-data changes.

## Observability Plan

Not applicable per requirements.md (Complexity 2, "Observability Requirements: Not applicable"). No new logs/metrics/alerts.

## Risk Control

- **Feature flag**: None — requirements.md's own Risk Control section states this is a low-risk, easily-revertable presentation-layer change with no flag needed.
- **Rollback procedure**: Standard PR revert; no data migration to unwind.
- **Staged rollout**: None — single-user personal tool; existing e2e/Axe/Lighthouse CI (already wired for `web-app/src/` PRs) is the gate.

## Unresolved Questions

- The exact `SegmentBudget` constants (Story 2.1.1's single `ROW_PATH_MAX_LEN = 72`, Story 2.2.1's `CARD_PATH_MAX_LEN = 96`) are initial estimates from the research, not measured against real rendered column widths — Task 2.1.1c/2.2.1c (**not** 2.2.1b, which is code-wiring only) call for a manual visual check once the row/card renders, with the constants adjustable in those tasks without touching the truncation algorithm itself. `ROW_PATH_MAX_LEN` was raised from an earlier `56` back to `72` (2026-09-24 adversarial re-review) after computing that `56` forced the plan's own canonical example (`~/.stapler-squad/workspaces/…/worktrees/stapler-squad-wasted-space…`, 67 chars post-segment-collapse) into `truncateMiddle`'s buggy embedded-dot fallback — see Task 1.1.1c's added regression test and Story 2.1.1's Resolution Note-adjacent AC for the math.
- Whether Radix's `Tooltip` shows on tap on the project's actual target mobile browsers (iOS Safari vs. Chrome Android) is asserted from Radix's documented behavior, not verified in this plan — Story 3.2.1's manual-test task calls this out explicitly; if it turns out tap does work, ADR-001's "does not fully fix touch-only sighted users" caveat can be revisited as a fast-follow note, not a blocker.

## Dependency Visualization

```
Phase 1: Foundation (shared utilities — no visible UI change)
  Epic 1.1 truncateWorkspacePath util ──────────┐
  Epic 1.2 Column-visibility + elapsed 2nd line  │
                                                  │
Phase 2: Row & Card layout redesign              │
  Epic 2.1 SessionRow wrap/narrow/estimateSize ◄─┤ (needs 1.1, 1.2)
  Epic 2.2 SessionCard path/narrow layout +      │
    column-gating (Story 2.2.3) ◄────────────────┘ (needs 1.1, 1.2 — 2.2.3 needs
                                                      COLUMN_DEFS/DEFAULT_VISIBLE_COLUMNS from 1.2)

Phase 3: Accessibility fixes
  Epic 3.1 Accessible-name integrity ◄── needs 2.1 (path span markup exists)
  Epic 3.2 Agent/memory disclosure (ADR-001) ◄── needs 1.2 (columns demoted)
  Epic 3.3 Touch-target sizing ◄── independent of 2.x/3.x, can run in parallel

Phase 4: Regression guard & verification
  Epic 4.1 e2e locator audit ◄── needs all of Phase 2 + 3 merged
  Epic 4.2 Final verification (build/lint/test/jscpd) ◄── needs 4.1
```

---

## Phase 1: Foundation

### Epic 1.1: Shared segment-aware path truncation utility

**Goal**: Replace the two divergent private `abbreviatePath` implementations with
one shared, unit-tested, segment-aware truncation function usable by both
`SessionRow.tsx` and `SessionCard.tsx`.

#### Story 1.1.1: Build `truncateWorkspacePath`

**As a** developer of `SessionRow`/`SessionCard`, **I want** a single pure function
that collapses opaque path segments while preserving semantic ones, **so that**
long workspace/worktree paths stay scannable without a divergent, ad hoc
abbreviation per component.

**Acceptance Criteria**:
- `truncateWorkspacePath` returns the input unchanged when `path.length <= maxLen`.
  - *Given* `path = "~/repo/src"` and `maxLen = 40`, *When* `truncateWorkspacePath(path, maxLen)` is called, *Then* it returns `"~/repo/src"` unchanged.
- A whole-segment opaque hash/UUID is collapsed to a single `…` while the leading and trailing segments survive. `truncateWorkspacePath` itself performs the `/Users/<user>`/`/home/<user>` → `~` collapse as a pre-processing step (see Task 1.1.1b) — this is not left to the caller.
  - *Given* `path = "/Users/tstapler/.stapler-squad/workspaces/6eb0b580fa0331d5/worktrees/stapler-squad-wasted-space"` and `maxLen = 50`, *When* called, *Then* the result contains `~/.stapler-squad/…/stapler-squad-wasted-space` (the `6eb0b580fa0331d5` workspace-hash segment is replaced by a single `…`, `~` collapse still applies) and does not contain the literal string `6eb0b580fa0331d5`.
- A trailing opaque hex suffix on an otherwise-semantic segment/name is collapsed, keeping the semantic prefix.
  - *Given* `path = "worktrees/stapler-squad-wasted-space_18d807dfb97a2b28"` and `maxLen = 40`, *When* called, *Then* the result contains `stapler-squad-wasted-space_…` and does not contain the literal string `18d807dfb97a2b28`.
- A single unbroken opaque-looking token with no path separator (the confirmed `pr-424-compute-nop-18c993e1e9402c…` example) is still collapsed at its trailing hex run, not left to overflow.
  - *Given* `path = "pr-424-compute-nop-18c993e1e9402c8f1a"` (no `/`) and `maxLen = 25`, *When* called, *Then* the result starts with `pr-424-compute-nop-` and ends with `…`, and does not contain the full 20+ char hex run.
- A short, legitimately all-hex-looking segment below the length threshold is NOT collapsed (false-positive guard).
  - *Given* `path = "repo/deadbeef/src"` (7-char segment `deadbeef`) and `maxLen = 100` (fits without truncation), *When* called, *Then* the result is returned unchanged (the length check short-circuits before classification, so no false-positive risk at this length) — a second, explicit unit test also asserts that at a `maxLen` forcing truncation, a hex-like segment shorter than 7 chars (e.g. `dead`) is treated as `SemanticSegment`, never collapsed.
- If no opaque segment is found and the path still exceeds `maxLen`, fall back to `truncateMiddle`'s char-budget behavior on the whole string.
  - *Given* `path = "a-very-long-but-entirely-human-readable-branch-name-with-no-hash"` and `maxLen = 30`, *When* called, *Then* the result is exactly `truncateMiddle(path, 30)`'s output.
- Falsy/empty input is returned unchanged (matches `truncateMiddle`'s existing defensive pattern).
  - *Given* `path = ""`, *When* called, *Then* the result is `""`.

**Files**: `web-app/src/lib/utils/truncateWorkspacePath.ts` (new), `web-app/src/lib/utils/truncateWorkspacePath.test.ts` (new)

##### Task 1.1.1a: Write `classifySegment` and the opaque-detection regexes (~4 min)
- In the new file, add `type SegmentKind = "semantic" | "opaque"` and `function classifySegment(segment: string): SegmentKind`.
- Regexes: `HEX_HASH_RE = /^[0-9a-f]{7,40}$/i`, `UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i` (copy verbatim per `build-vs-buy.md`'s "copy a canonical UUID regex from a trusted reference" guidance), `TRAILING_OPAQUE_RE = /[-_][0-9a-f]{8,}$/i`.
- A segment is `"opaque"` if it matches `HEX_HASH_RE` or `UUID_RE` in full, or if `TRAILING_OPAQUE_RE` matches (in which case the caller collapses only the matched suffix, not the whole segment — see Task 1.1.1b).
- Also add `HOME_DIR_RE = /^\/(?:home|Users)\/[^/]+/` (copied from `WorkspaceSwitcher.tsx`'s existing `abbreviatePath`) — this is not a `SegmentKind` classification (the username segment is semantic-looking, not hex/UUID) but a distinct pre-processing substitution consumed by Task 1.1.1b.
- Files: `web-app/src/lib/utils/truncateWorkspacePath.ts`

##### Task 1.1.1b: Write `truncateWorkspacePath` main function body (~5 min)
- Early-return unchanged if `!path`.
- **Step 0 (home-dir collapse, must run before length-check and segment classification):** `path = path.replace(HOME_DIR_RE, "~")`. This is required because splitting an absolute path on `/` puts the username as an *interior* segment (`Users`, `tstapler`), which `classifySegment` would never flag as opaque (it's not hex/UUID) — without this step the function would regress to showing the full `/Users/<user>/...` home directory instead of `~/...`, contradicting both `abbreviatePath` implementations it replaces and Story 1.1.1's own AC #2.
- Early-return the (now possibly `~`-collapsed) path unchanged if `path.length <= maxLen`.
- Split on `/`. For each interior segment (not first/last), classify; collapse a run of one-or-more consecutive whole-opaque segments to a single `…` segment.
- For any segment (including first/last) with a `TRAILING_OPAQUE_RE` match, replace just the matched suffix with `…` (keep the segment's semantic prefix and its position).
- Rejoin with `/`. If the rejoined result is still longer than `maxLen`, apply `truncateMiddle(rejoined, maxLen)` as the final fallback (import from `./truncateMiddle`).
- Files: `web-app/src/lib/utils/truncateWorkspacePath.ts`

##### Task 1.1.1c: Write unit tests covering the Given-When-Then cases above (~5 min)
- One test per acceptance criterion above, plus the two workspace-path/worktree-path examples from `research/features.md` verbatim as regression fixtures.
- Explicit home-dir-collapse test: `truncateWorkspacePath("/Users/tstapler/repo/src", 40)` → asserts the result starts with `~/repo/src` (or is exactly that string, since it fits under `maxLen`) and contains no literal `Users` or `tstapler` substring. A second variant combines both behaviors: a real `/Users/tstapler/...` path long enough to also trigger opaque-segment collapse, asserting the result both starts with `~/` and omits the opaque hash.
- **Canonical `ROW_PATH_MAX_LEN` regression test (added 2026-09-24, adversarial re-review)**: `truncateWorkspacePath("/Users/tstapler/.stapler-squad/workspaces/6eb0b580fa0331d5/worktrees/stapler-squad-wasted-space_18d807dfb97a2b28", 72)` → asserts the result contains the substring `stapler-squad-wasted-space` intact (not mid-word-truncated to e.g. `stapler-squad-w`) and does not equal what `truncateMiddle`'s embedded-dot fallback would have produced. Math: after home-dir collapse (`~`) and opaque-segment collapse, the rejoined string is `~/.stapler-squad/workspaces/…/worktrees/stapler-squad-wasted-space…` = 67 chars, which is `<= 72`, so Task 1.1.1b's `truncateMiddle` fallback branch never executes for this input — this test exists specifically to pin that behavior so a future budget decrease can't silently regress it back into the fallback (see adversarial-review.md's 2026-09-24 Blocker for why `56` failed this exact case: `.lastIndexOf(".")` finds `.stapler-squad`'s dot, treats the remaining 65 chars as an inviolable "suffix," `keep` goes negative, and it falls through to plain right-truncation that chops `stapler-squad-wasted-space` mid-word).
- Files: `web-app/src/lib/utils/truncateWorkspacePath.test.ts`

##### Task 1.1.1d: Run the new test file and confirm green (~2 min)
- `cd web-app && npx jest truncateWorkspacePath --no-coverage`
- Files: none (verification only)

#### Story 1.1.2: Retire the two divergent `abbreviatePath` implementations

**As a** maintainer, **I want** exactly one path-abbreviation function in the
codebase, **so that** the jscpd duplication gate doesn't flag near-identical logic
and the two components can't silently diverge again.

**Acceptance Criteria**:
- `SessionRow.tsx`'s private `abbreviatePath` (line 168) is deleted; its call site for the visible path text now calls `truncateWorkspacePath`, and its call site for the row-level `aria-label` uses the full `session.existingDir` directly (per the Pattern Decisions table).
  - *Given* a `SessionRow` rendered with `session.existingDir = "/Users/tstapler/repo"`, *When* the component renders, *Then* the visible path `<span>` text is `truncateWorkspacePath("/Users/tstapler/repo", <SegmentBudget>)` and the row's `aria-label` attribute contains the literal substring `/Users/tstapler/repo` (unabbreviated).
- `WorkspaceSwitcher.tsx`'s private `abbreviatePath` (line 208) is deleted; its call site uses `truncateWorkspacePath` with a locally appropriate `maxLen` (36, matching its prior hardcoded threshold).
  - *Given* `WorkspaceSwitcher` rendering a workspace whose path is `/Users/tstapler/.stapler-squad/workspaces/6eb0b580fa0331d5`, *When* it renders, *Then* the displayed label no longer contains the literal `abbreviatePath` function and instead calls `truncateWorkspacePath(path, 36)`.
- No other file in `web-app/src/` still defines a function literally named `abbreviatePath`.
  - *Given* the retirement is complete, *When* `grep -rn "function abbreviatePath" web-app/src/` is run, *Then* it returns no matches.

**Files**: `web-app/src/components/sessions/SessionRow.tsx`, `web-app/src/components/layout/WorkspaceSwitcher.tsx`

##### Task 1.1.2a: Update `SessionRow.tsx`'s path call sites (~4 min)
- Remove the `abbreviatePath` function (lines 168-170).
- Import `truncateWorkspacePath` from `@/lib/utils/truncateWorkspacePath`.
- Line 350: replace `{abbreviatePath(session.existingDir)}` with `{truncateWorkspacePath(session.existingDir, ROW_PATH_MAX_LEN)}` (constant introduced in Story 2.1.1 — use a local placeholder constant `72` here if Story 2.1.1 hasn't landed yet in execution order, then let 2.1.1 supersede it).
- Line 306: replace `path: ${abbreviatePath(session.existingDir)}` with `path: ${session.existingDir}` (full path, per Pattern Decisions).
- Files: `web-app/src/components/sessions/SessionRow.tsx`

##### Task 1.1.2b: Update `WorkspaceSwitcher.tsx`'s call site (~3 min)
- Remove the `abbreviatePath` function (lines 208-213).
- Import `truncateWorkspacePath` and replace the call site with `truncateWorkspacePath(path, 36)`.
- Files: `web-app/src/components/layout/WorkspaceSwitcher.tsx`

##### Task 1.1.2c: Grep-verify no remaining `abbreviatePath` definitions (~2 min)
- `grep -rn "function abbreviatePath" web-app/src/` — expect zero results.
- Files: none (verification only)

---

### Epic 1.2: Demote `agent`/`memory` columns, move `elapsed` to a second line

**Goal**: Flip the two low-value columns off by default and stop reserving grid
space for `elapsed` in the single-line grid, per requirements.md's Success
Metrics.

#### Story 1.2.1: Flip `agent`/`memory` to `defaultVisible: false`

**As a** user, **I want** the agent glyph and memory columns hidden by default,
**so that** rows aren't cluttered with columns that show "—" on ~80% of my
sessions.

**Acceptance Criteria**:
- `COLUMN_DEFS`'s `agent` and `memory` entries have `defaultVisible: false`; `elapsed`, `diff`, `branch` are unchanged.
  - *Given* a fresh `SessionRow` rendered with no `visibleColumns` prop passed (uses `DEFAULT_VISIBLE_COLUMNS`), *When* it renders, *Then* neither the agent-icon `<span>` nor the memory-badge `<span>` is present in the DOM, and the Columns picker still lists both as available, unchecked, toggles.
- The Columns picker (`ColumnPicker.tsx`) requires no code change — it already reads `COLUMN_DEFS` generically.
  - *Given* the flip above, *When* a user opens the Columns picker and checks "Agent", *Then* `visibleColumns` includes `"agent"` and the agent-icon `<span>` reappears in the row, unchanged from today's behavior.

**Files**: `web-app/src/components/sessions/session-columns.ts`

##### Task 1.2.1a: Flip the two `defaultVisible` flags (~2 min)
- Edit `COLUMN_DEFS` entries for `agent` and `memory` to `defaultVisible: false`.
- Files: `web-app/src/components/sessions/session-columns.ts`

##### Task 1.2.1b: Manually verify `ColumnPicker.tsx` and `DEFAULT_VISIBLE_COLUMNS` consumers need no change (~3 min)
- Grep `DEFAULT_VISIBLE_COLUMNS` usages; confirm each caller re-derives visibility from `COLUMN_DEFS` rather than hardcoding the old default set.
- Files: none (verification only)

#### Story 1.2.2: Move `elapsed` out of the grid loop onto a hardcoded second line

**As a** user, **I want** the "last active" time to always show without consuming
a dedicated grid column, **so that** the row layout has more room for the
name/path.

**Acceptance Criteria**:
- `buildRowGridTemplate()` skips the `elapsed` `ColumnDef` when building the grid-column list.
  - *Given* `visibleColumns = ["agent", "memory", "elapsed"]`, *When* `buildRowGridTemplate(visibleColumns, { reserveCheckbox: true })` is called, *Then* the returned template string contains the `gridWidth` values for `agent` and `memory` but not for `elapsed`.
- `SessionRow.tsx` renders the elapsed `<time>` element inside a new second flex line beneath the name/path (`ElapsedSecondLine`), not as a grid cell, and it is still gated on `visibleColumns.includes("elapsed")`.
  - *Given* a `SessionRow` with default visible columns (`elapsed` included per Story 1.2.1), *When* it renders, *Then* the `<time>` element is a DOM descendant of the `nameCell` container (same parent as `pathLine`), not a direct grid-cell child of the row's own `display: grid` element.

**Files**: `web-app/src/components/sessions/session-columns.ts`, `web-app/src/components/sessions/SessionRow.tsx`, `web-app/src/components/sessions/SessionRow.css.ts`

##### Task 1.2.2a: Skip `elapsed` in `buildRowGridTemplate()` (~2 min)
- In the `for (const def of COLUMN_DEFS)` loop, add `if (def.key === "elapsed") continue;` before the `gridWidth` push.
- Files: `web-app/src/components/sessions/session-columns.ts`

##### Task 1.2.2b: Add an `elapsedSecondLine` CSS class (~3 min)
- New `style()` export in `SessionRow.css.ts`: flex row, `gap: 4px`, `fontSize: vars.fontSize.xs`, `color: vars.color.textMuted`, `marginTop: "2px"` — visually similar to the existing `elapsed`/`elapsedIcon` styles it reuses.
- Files: `web-app/src/components/sessions/SessionRow.css.ts`

##### Task 1.2.2c: Move the elapsed JSX block into the second line (~4 min)
- Cut the `{visibleColumns.includes("elapsed") && (<time ...>...)}` block (current lines 537-565) out of its position as a grid-cell sibling.
- Wrap it in a new `<span className={elapsedSecondLine}>` placed as the last child inside `nameCellStyle`'s `<span>`, after the `failureMessageLine` block.
- Files: `web-app/src/components/sessions/SessionRow.tsx`

##### Task 1.2.2d: Update `SessionRow.css.ts`'s fallback `gridTemplateColumns` comment/value (~2 min)
- The no-JS fallback at `row`'s `gridTemplateColumns: "24px 8px 1fr 20px auto 32px auto"` (line 15) still lists a 6th `auto` track for `elapsed` — remove that track and update the comment to reflect `elapsed` no longer being a grid column.
- Files: `web-app/src/components/sessions/SessionRow.css.ts`

---

## Phase 2: Row & Card layout redesign

### Epic 2.1: `SessionRow` — wrap, narrow-container layout, virtualizer tuning

**Goal**: Replace ellipsis truncation with wrapping + `truncateWorkspacePath`,
add the container-query narrow layout, and tune the virtualizer's height
estimate for the new taller rows.

#### Story 2.1.1: Wrap name/path instead of ellipsis, apply `truncateWorkspacePath`

**As a** user, **I want** long session names and paths to wrap onto additional
lines instead of being cut off, **so that** I can read them without hovering.

**Acceptance Criteria**:
- `nameStyle`'s CSS no longer sets `whiteSpace: "nowrap"`/`textOverflow: "ellipsis"`; it wraps normally, with `overflow-wrap: anywhere` as the safety net for unbroken tokens (per `ux.md` §4).
  - *Given* `session.title = "pr-424-compute-nop-18c993e1e9402c8f1a-very-long-descriptive-title"` rendered in a `SessionRow` at the default ~280px sidebar width, *When* the row renders, *Then* the name `<span>` wraps onto 2+ lines rather than being clipped with `…`, and no character overflows the row's width.
- The path `<span>`'s visible text is `truncateWorkspacePath(session.existingDir, ROW_PATH_MAX_LEN)` where `ROW_PATH_MAX_LEN = 72` is a single named constant used at every container width (see Story 2.1.2's Resolution Note for why this is one value, not a normal/narrow pair).
  - *Given* `session.existingDir = "/Users/tstapler/.stapler-squad/workspaces/6eb0b580fa0331d5/worktrees/stapler-squad-wasted-space_18d807dfb97a2b28"`, *When* the row renders at any container width, *Then* the visible path text contains neither the literal `6eb0b580fa0331d5` nor `18d807dfb97a2b28` substrings, and the `<span>`'s `Tooltip`/`aria-label` still show the full original path unchanged (per the Pattern Decisions row-level vs. per-element split).
  - *Given* the same fixture, *Then* segment-collapse alone (before any `truncateMiddle` fallback) already fits the budget: `~/.stapler-squad/workspaces/…/worktrees/stapler-squad-wasted-space…` is 67 chars <= `ROW_PATH_MAX_LEN = 72`, so the `truncateMiddle` fallback in Task 1.1.1b never fires and the trailing segment `stapler-squad-wasted-space…` is never mid-word-truncated (contrast with the rejected `56` value, which forced the fallback and mangled it to `stapler-squad-w…` — see adversarial-review.md's 2026-09-24 Blocker for the char-by-char derivation).
- `SessionRow`'s minimum row height (`minHeight: "38px"`) is preserved as a floor, not a fixed cap, so wrapped 2-3 line rows can grow taller.
  - *Given* the wrapped-name case above, *When* the row renders, *Then* its computed height exceeds 38px and is not clipped.

**Files**: `web-app/src/components/sessions/SessionRow.css.ts`, `web-app/src/components/sessions/SessionRow.tsx`

##### Task 2.1.1a: Update `name` and `path` CSS to wrap (~4 min)
- In `SessionRow.css.ts`, remove `overflow: "hidden"`, `textOverflow: "ellipsis"`, `whiteSpace: "nowrap"` from `name` (lines 110-117) and `path` (lines 126-134); add `overflowWrap: "anywhere"` to both.
- Files: `web-app/src/components/sessions/SessionRow.css.ts`

##### Task 2.1.1b: Add the `ROW_PATH_MAX_LEN` constant and wire `truncateWorkspacePath` (~4 min)
- Near the top of `SessionRow.tsx`, add a single `const ROW_PATH_MAX_LEN = 72;`. There is no separate narrow-width constant — Story 2.1.2's Resolution Note explains why a single shared budget is used at every container width instead of a normal/narrow pair.
- Replace line 350's `{abbreviatePath(session.existingDir)}` (already changed to a placeholder in Task 1.1.2a) with `{truncateWorkspacePath(session.existingDir, ROW_PATH_MAX_LEN)}`.
- Files: `web-app/src/components/sessions/SessionRow.tsx`

##### Task 2.1.1c: Manual visual check of wrapped rows, in both mounting contexts (~8 min)
- Run the manual dev instance (`~/.stapler-squad/manual-builds/manual-1`, port `62871`, per this repo's CLAUDE.md) with a session whose title/path matches the acceptance-criteria fixtures; confirm visually that wrapping looks reasonable **at both the ~280px default sidebar width and a collapsed/narrow width (~200px)**, and adjust `ROW_PATH_MAX_LEN` if egregiously wrong at either (see Unresolved Questions). Since one constant now serves both widths, this check must explicitly cover both, not just the default.
- **Second mounting context (pre-mortem.md P1 #1)**: `SessionList` also mounts as a leaf inside the user-resizable pane-split cockpit layout (`web-app/src/components/pane/PaneSplitRenderer.tsx`, rendered when `pane.viewKind === "session-list"`; resized via `ResizeHandle`/`RESIZE_PANE`, `PaneSplitRenderer.tsx:182-188`), where pane width is a continuous, user-dragged value, not the fixed ~280px sidebar the check above covers. Repeat the same visual check with a cockpit pane dragged to approximately **200px, 280px, 400px, and 600px**, and confirm no clipping/overflow and no egregiously-wasted whitespace at any of those widths — the fixed-sidebar check alone does not establish that `ROW_PATH_MAX_LEN`/the 200px breakpoint are sane across this second, wider, continuously-resizable range.
- Files: none (verification/tuning only — may adjust the constant from Task 2.1.1b)

#### Story 2.1.2: Container-query narrow layout for `SessionRow`

**As a** user on a narrow sidebar/mobile viewport, **I want** the row to adapt
its layout, **so that** nothing clips or requires horizontal scroll.

**Resolution Note (addresses architecture-review.md BLOCKER 2 and adversarial-review.md's `NARROW` breakpoint BLOCKER together, since both are the same root problem — this story's narrow-layout mechanism)**:
The sidebar's real normal width is a **fixed ~280px** column (`SessionList.css.ts:6`'s own comment: "the session list now lives in a fixed 280px column"). The original `NARROW = "(max-width: 320px)"` breakpoint is wider than that, so it would match unconditionally at the sidebar's default width, making the "narrow" 40-char budget the permanent state and making the 72-char "normal width" acceptance criterion unreachable — that criterion is corrected above (Story 2.1.1) rather than left stale.
The original mechanism to pick between two truncation budgets — rendering the path text twice (normal-budget `<span>` + narrow-budget `<span>`) and toggling visibility with `@container`/`display:none` — is dropped entirely, not just re-thresholded: both spans stay in the DOM regardless of which is visible, so a `.textContent` read or a `getByRole`-style locator sees both differently-truncated strings; it also reintroduces the duplicated truncation-call pattern Story 1.1.2 exists to eliminate, and adds render bulk to `SessionRow.tsx`'s already-flagged long render body (Tech Debt Disposition table).
**Chosen direction**: a single computed budget — one `ROW_PATH_MAX_LEN = 72` constant (Story 2.1.1) used at every container width, with no JS-conditional budget and no dual render. This is the simpler of the two remediation options architecture-review.md offered (drop the JS-conditional budget, per requirements.md's Complexity-2 Appetite favoring the smaller diff) rather than adding a CSS-custom-property-plus-`getComputedStyle` bridge, which would add JS-reads-CSS complexity to save the ~16 characters of truncation budget the old narrow tier saved in the collapsed case. The container-query breakpoint (corrected below to `(max-width: 200px)`, genuinely narrower than the fixed 280px default) still exists and still does real work — it drives purely-visual narrow tweaks (font-size reduction, second-line wrapping) that involve no JS truncation-budget decision at all, so no dual-render or JS-measurement problem arises for those. (2026-09-24 re-review: the single value was briefly set to `56` — the old *narrow*-tier number — instead of `72` — the old *normal*-tier number — which regressed the plan's own canonical example into `truncateMiddle`'s embedded-dot fallback; corrected back to `72`, which was never narrower than the sidebar's real fixed ~280px width in the first place, so nothing about this story's container-query mechanism changes. This raise does not conflict with the `(max-width: 200px)` breakpoint: per this Resolution Note, that breakpoint only drives visual/CSS tweaks — font-size, chip wrapping — never the truncation-budget decision, and Story 2.1.1's CSS already switches the path from ellipsis/`nowrap` to `overflow-wrap: anywhere` wrapping (Task 2.1.1a), so a longer character budget just wraps onto more lines at 200px rather than overflowing or getting clipped. Task 2.1.1c's manual visual check still covers both widths in case the extra line count looks cramped in practice, but no structural conflict is expected.)

**Acceptance Criteria**:
- `SessionRow.css.ts`'s `row` class gets `containerType: "inline-size"` + `containerName: "sessionRow"`, copied from `FilesTab.css.ts`'s pattern, applied to the row's own root (not any ancestor scroll/virtualizer wrapper, per `pitfalls.md` §2's ancestor-`overflow` warning).
  - *Given* the updated CSS, *When* inspected in devtools, *Then* the `.row` element (not `SessionList`'s scroll container) carries `container-type: inline-size; container-name: sessionRow`.
- A `NARROW = "(max-width: 200px)"` breakpoint — genuinely narrower than the sidebar's fixed ~280px default width, so it only matches a collapsed/mobile-narrow layout, not the everyday case — triggers purely visual tweaks: the second-line elapsed/status chips wrap onto their own line if needed, and font size reduces slightly. It does **not** switch the path-truncation budget; `ROW_PATH_MAX_LEN` (Story 2.1.1) is a single value used at every width.
  - *Given* a `SessionRow` rendered inside a container narrower than 200px, *When* it renders, *Then* the second-line chips wrap and no text is clipped, while the path `<span>`'s truncation budget is unchanged from the default-width case.
- No horizontal scrollbar appears on the row at any container width from 200px to 400px.
  - *Given* the row rendered at 200px, 280px, and 400px container widths, *When* inspected, *Then* `scrollWidth <= clientWidth` at each width (no horizontal overflow).

**Files**: `web-app/src/components/sessions/SessionRow.css.ts`, `web-app/src/components/sessions/SessionRow.tsx`

##### Task 2.1.2a: Add `containerType`/`containerName` to `row` (~2 min)
- Add `containerType: "inline-size"`, `containerName: "sessionRow"` to the `row` style object.
- Files: `web-app/src/components/sessions/SessionRow.css.ts`

##### Task 2.1.2b: Add narrow-breakpoint CSS overrides (~4 min)
- Add a `narrowPathHint` class (or extend `path`) with an `"@container": { "sessionRow (max-width: 200px)": {...} }` block per the `FilesTab.css.ts` shape — narrow-specific, CSS-only tweaks (e.g. `fontSize` reduction, `flexWrap: "wrap"` on `pathLine`). No truncation-budget logic lives here — see this story's Resolution Note.
- Files: `web-app/src/components/sessions/SessionRow.css.ts`

##### Task 2.1.2c: (removed — see Resolution Note)
- The dual-render/`display:none`-toggle approach originally planned here is dropped. `SessionRow.tsx` renders the path exactly once, via `truncateWorkspacePath(session.existingDir, ROW_PATH_MAX_LEN)` (already wired in Task 2.1.1b) — no additional JS or markup is needed for the narrow case, since the truncation budget no longer varies by container width.
- Files: none

#### Story 2.1.3: Tune `estimateSize` for wrapped rows, measure the density tradeoff

**As a** user scrolling the session list, **I want** minimal layout jump when
rows have become taller, **so that** scrolling feels stable — and, since
this feature is literally named "density," **I want** the tradeoff of fewer
rows fitting on screen at once to be visible and reviewed, not silently
absorbed (pre-mortem.md P1 #3: wrapping single-line 38px rows into 2-4 line
rows could roughly halve rows-visible-per-viewport, and no prior task or
success metric measured this).

**Acceptance Criteria**:
- `SessionList.tsx`'s `useVirtualizer`'s `estimateSize` row value is increased from `50` to a value reflecting the new typical 2-line row height (e.g. `64`), while `measureElement` (already wired) continues to correct the real height post-render.
  - *Given* the updated constant, *When* the session list first mounts with 50+ sessions and the user scrolls quickly, *Then* the visual "jump" between estimated and measured row positions is smaller than before the change (qualitative manual check — no new automated assertion, per `build-vs-buy.md` §3's "config tweak, not architecture change" framing).
- The number of fully-visible rows at the sidebar's default fixed-280px width/typical viewport height is recorded before and after the full Phase 1-2 diff, and the before/after counts are reviewed rather than assumed acceptable. This is **not** a hard numeric target (requirements.md sets none) — the bar is "recorded and reviewed," so a regression is visible instead of shipping silently.
  - *Given* the pre-change build (row height 38px, single-line) and the post-change build (wrapped rows + `elapsed` second line), *When* the sidebar is scrolled with 20+ real sessions at its default width/a representative viewport height, *Then* both row counts are written down (e.g. in this task's notes or a PR comment) and a large drop (roughly halving or more) is explicitly called out as a tradeoff to accept or push back on, not left unmentioned.

**Files**: `web-app/src/components/sessions/SessionList.tsx`

##### Task 2.1.3a: Bump the row `estimateSize` constant (~2 min)
- Change `SessionList.tsx:646`'s `(flatItems[i]?.kind === "header" ? 40 : 50)` to `(flatItems[i]?.kind === "header" ? 40 : 64)`.
- Files: `web-app/src/components/sessions/SessionList.tsx`

##### Task 2.1.3b: Manual scroll-jank check (~3 min)
- Using the manual dev instance, scroll a list of 50+ sessions with wrapped rows; confirm no visible layout thrash beyond what existed pre-change.
- Files: none (verification only)

##### Task 2.1.3c: Measure and record rows-visible-per-viewport, before vs. after (~6 min)
- Using the manual dev instance, on `main` (pre-change) and again on this feature's branch (post-change), count how many session rows are fully visible without scrolling at the sidebar's default fixed-280px width, with the same 20+-session fixture list and the same browser window height. Record both counts.
- If the drop is large (roughly halving or worse), flag it explicitly in the PR description rather than proceeding silently — this task does not itself gate merge (requirements.md sets no hard target), but the count must be visible to a reviewer.
- **Optional design question for the implementer, not a mandate**: whether the hardcoded `elapsed` second line (Task 1.2.2c) and/or the failure-message line could share a line with the path on rows that don't otherwise wrap, rather than every row unconditionally growing by a line, is worth weighing if the before/after count looks bad — this plan does not unilaterally cut that scope; note it as a tradeoff for whoever implements to decide, not a silent redesign.
- Files: none (verification/measurement only)

---

### Epic 2.2: `SessionCard`/`BoardCard` — path truncation, column gating, and narrow layout

**Goal**: Apply `truncateWorkspacePath` to `SessionCard`'s raw path renders,
gate the Program row and memory badge behind `visibleColumns` to match
`SessionRow`'s Story 1.2.1 demotion (Story 2.2.3), and add the same
container-query narrow-layout pattern; no separate Board-specific work, per
`architecture.md` §7's confirmation that `BoardCard` is a thin wrapper.

#### Story 2.2.1: Apply `truncateWorkspacePath` to `SessionCard`'s path fields

**As a** user viewing Board/Card sessions, **I want** the same opaque-segment
collapsing as the list row, **so that** long paths are equally readable there.

**Acceptance Criteria**:
- `SessionCard.tsx`'s three raw path renders (`existingDir` line ~1047, `activeDir` line ~1054, `clonedRepoPath` line ~1095) use `truncateWorkspacePath(value, CARD_PATH_MAX_LEN)` for visible text; each `title={...}` attribute keeps the full untruncated value (already true — no change needed there).
  - *Given* `session.existingDir` set to the same fixture path used in Story 2.1.1's acceptance criteria, *When* `SessionCard` renders, *Then* the visible "Path:" value text does not contain the opaque hash/UUID substrings, while the element's `title` attribute still contains the full original path unchanged.
- `CARD_PATH_MAX_LEN = 96` is a named constant (cards have more horizontal room than rows, per requirements.md's own note).
  - *Given* the constant, *When* grepped in `SessionCard.tsx`, *Then* it appears as a single named `const`, not inlined as a magic number at each of the three call sites.

**Files**: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 2.2.1a: Add `CARD_PATH_MAX_LEN` constant and import `truncateWorkspacePath` (~2 min)
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 2.2.1b: Wire the three path render call sites (~4 min)
- Lines ~1047 (`existingDir`), ~1054 (`activeDir`), ~1095 (`clonedRepoPath`): wrap the visible `{session.xxx}` text in `truncateWorkspacePath(session.xxx, CARD_PATH_MAX_LEN)`; leave the `title={session.xxx}` attributes as full values (already correct for `existingDir`/`clonedRepoPath`; confirm `activeDir`'s render also has a `title` — add one if missing, matching the sibling rows' pattern).
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 2.2.1c: Manual visual check of the three path fields, tune `CARD_PATH_MAX_LEN`, in both mounting contexts (~8 min)
- Mirrors Task 2.1.1c for the row case. Using the manual dev instance, render a card for a session whose `existingDir`/`activeDir`/`clonedRepoPath` match the acceptance-criteria fixtures (including the opaque-hash example from Story 2.1.1); confirm all three path fields display readably at the card's normal width and adjust `CARD_PATH_MAX_LEN` if egregiously wrong. This is the task the Unresolved Questions section refers to for tuning `CARD_PATH_MAX_LEN` — Task 2.2.1b (above) is code-wiring only and does no visual verification.
- **Second mounting context (pre-mortem.md P1 #1)**: Board view's card layout is reachable from the same resizable pane-split cockpit layout as `SessionRow` (`web-app/src/components/pane/PaneSplitRenderer.tsx`, `pane.viewKind === "session-list"`, resized via `ResizeHandle`/`RESIZE_PANE`). Repeat the check with a Board column/cockpit pane at approximately 200px, 280px, 400px, and 600px to confirm `CARD_PATH_MAX_LEN` doesn't clip/overflow or look egregiously wasteful across that continuously-resizable range, not just at the Board column's typical fixed width.
- Files: none (verification/tuning only — may adjust the constant from Task 2.2.1a)

#### Story 2.2.2: Container-query narrow layout for `SessionCard`, verify `BoardCard` chrome width

**As a** user viewing a narrow Board column or narrow mobile card list, **I want**
the card to adapt its layout, **so that** content isn't clipped.

**Acceptance Criteria**:
- `SessionCard.css.ts`'s card root gets `containerType: "inline-size"` + `containerName: "sessionCard"`, same pattern as `FilesTab.css.ts`.
  - *Given* the updated CSS, *When* inspected, *Then* the card's root element carries the container-query properties.
- `BoardCard.css.ts`'s chrome (drag handle `40px` width, per the existing `handleWidth`-style rule found in that file) does not force the inner `SessionCard` below a usable minimum width once the card's own narrow-layout kicks in.
  - *Given* a Board column narrowed to 260px total width (accounting for BoardCard's `~44px` drag-handle/chrome), *When* the card renders, *Then* the inner `SessionCard` content area is still >= 200px wide and no text is clipped without wrapping.

**Files**: `web-app/src/components/sessions/SessionCard.css.ts`, `web-app/src/components/sessions/BoardCard.css.ts`

##### Task 2.2.2a: Add `containerType`/`containerName` to `SessionCard`'s root class (~2 min)
- Files: `web-app/src/components/sessions/SessionCard.css.ts`

##### Task 2.2.2b: Add narrow-breakpoint overrides for the card's info rows (~4 min)
- Mirror Task 2.1.2b's shape: an `"@container": { "sessionCard (max-width: 260px)": {...} }` block on the `infoRow`/`value` classes to allow wrapping instead of clipping.
- Files: `web-app/src/components/sessions/SessionCard.css.ts`

##### Task 2.2.2c: Manually verify `BoardCard`'s chrome width against the new minimum (~3 min)
- Using the manual dev instance's Board view, narrow a board column to its minimum supported width and confirm the card doesn't visually break.
- Files: none (verification only)

#### Story 2.2.3: Gate `SessionCard`'s Program row and memory badge behind `visibleColumns`

**As a** user, **I want** the agent-program row and memory badge hidden by
default on Board/Card view too, **so that** the Board card gets the same
"~80% of rows show no useful agent/memory data" cleanup Story 1.2.1 already
gives the List row — requirements.md's Scope→In Scope and Success Metric #2
both require this on "the default List row **and** Board card layout," but
`SessionCard.tsx` currently renders its Program info row and memory badge
unconditionally, with no `visibleColumns` gating at all (unlike
`SessionRow.tsx`, which already consumes `visibleColumns`/`buildRowGridTemplate()`
per Epic 1.2).

**Acceptance Criteria**:
- `SessionCardProps` (`SessionCard.tsx:205`) gains an optional
  `visibleColumns?: ColumnKey[]` prop, defaulting inside the component to
  `DEFAULT_VISIBLE_COLUMNS` (imported from `./session-columns`) when omitted
  — the same default source `SessionRow` uses, so after Epic 1.2's flip this
  default already excludes `"agent"`/`"memory"` with no separate constant to
  keep in sync.
  - *Given* `SessionCard` rendered with no `visibleColumns` prop, *When* it renders, *Then* the effective visible-columns set equals `DEFAULT_VISIBLE_COLUMNS` from `session-columns.ts`.
- The Program info row (`SessionCard.tsx:1033-1036`, the `<div className={infoRow}><span className={label}>Program:</span>...` block) only renders when `visibleColumns.includes("agent")`.
  - *Given* `SessionCard` rendered with no `visibleColumns` prop (the new default, `agent` excluded per Story 1.2.1's flip), *When* it renders, *Then* the "Program:" info row is absent from the DOM.
  - *Given* `SessionCard` rendered with `visibleColumns` including `"agent"`, *When* it renders, *Then* the "Program:" info row is present, unchanged from today's pre-redesign behavior.
- The memory badge (`SessionCard.tsx:818-838`, the IIFE rendering `<span className={memoryBadge}...>`) only renders when `visibleColumns.includes("memory")` (in addition to its existing `mb <= 0` guard — both conditions must pass).
  - *Given* `SessionCard` rendered with no `visibleColumns` prop and `session.memoryRssMb > 0`, *When* it renders, *Then* the memory badge is absent from the DOM.
  - *Given* `SessionCard` rendered with `visibleColumns` including `"memory"` and `session.memoryRssMb > 0`, *When* it renders, *Then* the memory badge is present, unchanged from today's behavior.
- `BoardCard.tsx` needs **no code change**: it already extends `SessionCardProps` and spreads `{...sessionCardProps}` straight through to `SessionCard` (`BoardCard.tsx:16,68`), so the new optional `visibleColumns` prop flows through automatically the moment a caller passes it — same "thin wrapper inherits the fix" relationship as Stories 2.2.1/2.2.2.
  - *Given* the `SessionCardProps` change above, *When* `BoardCard.tsx` is inspected, *Then* no edit to that file is required for this story's behavior to apply.
- No existing caller of `SessionCard`/`BoardCard` (Board/List card-mode call sites) needs to pass `visibleColumns` for the default-hidden behavior to take effect — confirmed by grep, not assumed.
  - *Given* `grep -rn "<SessionCard\|<BoardCard" web-app/src/`, *When* run, *Then* every call site either omits `visibleColumns` (picks up the new default) or is a test fixture explicitly exercising the prop.

**Files**: `web-app/src/components/sessions/SessionCard.tsx`, `web-app/src/components/sessions/SessionCard.test.tsx` (existing or new)

##### Task 2.2.3a: Add `visibleColumns` prop and default (~3 min)
- Add `visibleColumns?: ColumnKey[];` to `SessionCardProps`; import `ColumnKey`, `DEFAULT_VISIBLE_COLUMNS` from `./session-columns`.
- Inside the component body, destructure with a default: `const effectiveColumns = visibleColumns ?? DEFAULT_VISIBLE_COLUMNS;`
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 2.2.3b: Gate the Program info row (~2 min)
- Wrap the existing `<div className={infoRow}>` block at lines 1033-1036 in `{effectiveColumns.includes("agent") && ( ... )}`.
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 2.2.3c: Gate the memory badge (~2 min)
- In the IIFE at lines 818-838, add `if (!effectiveColumns.includes("memory")) return null;` alongside the existing `if (mb <= 0) return null;` early return.
- Files: `web-app/src/components/sessions/SessionCard.tsx`

##### Task 2.2.3d: Grep-verify no caller needs updating, add/extend unit tests (~5 min)
- `grep -rn "<SessionCard\|<BoardCard" web-app/src/` — confirm no non-test call site passes `visibleColumns` today (so all pick up the new default) and no call site breaks.
- Add (or extend an existing) `SessionCard.test.tsx` test asserting the Program row and memory badge are absent by default and present when `visibleColumns` explicitly includes `"agent"`/`"memory"`.
- Files: `web-app/src/components/sessions/SessionCard.test.tsx`

---

## Phase 3: Accessibility fixes

### Epic 3.1: Accessible-name integrity for truncated text

**Goal**: Confirm every truncated visible string has a full-value accessible
name reachable by assistive tech — not a "don't let visual truncation lie to
assistive tech" regression.

#### Story 3.1.1: Verify/complete full-value accessible names on all new truncation call sites

**As a** screen-reader user, **I want** the full session name/path announced,
**not the visually truncated string, **so that** I don't lose information
sighted users have via wrapping.

**Acceptance Criteria**:
- `SessionRow`'s path `<span>` (`Tooltip label={session.existingDir}` + `aria-label={Path: ${session.existingDir}}`) continues to carry the full path — confirmed unchanged by Story 2.1.1/1.1.2, not a new gap.
  - *Given* the changes from Phase 1/2, *When* the path span is inspected via the accessibility tree, *Then* its accessible name is the full, untruncated `session.existingDir` value.
- `SessionRow`'s row-level `aria-label` (line 306) carries the full path per the Pattern Decisions table, not a truncated one.
  - *Given* a session with a long opaque-segment path, *When* the row's `aria-label` is read, *Then* it contains the complete path string with no elided segments.
- `SessionCard`'s three path fields' `title` attributes carry the full value (Story 2.2.1 already guarantees this; this story is the explicit verification pass).
  - *Given* the `SessionCard` path fields after Story 2.2.1, *When* each is inspected, *Then* `title` equals the full untruncated path.

**Files**: none new — verification-only story confirming Phase 1/2 output; may produce small fix-up edits if a gap is found.

##### Task 3.1.1a: Grep-audit every new `truncateWorkspacePath` call site for a co-located full-value `title`/`aria-label` (~5 min)
- `grep -n "truncateWorkspacePath" web-app/src/components/sessions/*.tsx` and manually confirm each has a full-value companion attribute within the same JSX element or its immediate wrapper.
- Files: none (verification only; fix in place if a gap is found — e.g. `activeDir`'s missing `title`, flagged in Task 2.2.1b)

##### Task 3.1.1b: Add a regression test asserting the row `aria-label` contains the full path (~4 min)
- In `SessionRow`'s existing test file (or a new one if none exists — check `SessionRow.test.tsx` first), add a test rendering a session with a fixture opaque path and asserting `getByRole(...).getAttribute("aria-label")` contains the full literal path substring.
- Files: `web-app/src/components/sessions/SessionRow.test.tsx` (existing or new)

---

### Epic 3.2: Agent/memory accessible disclosure (ADR-001)

**Goal**: Replace `title`-only tooltips on the demoted `agent`/`memory` columns
with the shared, keyboard-focusable `Tooltip` component, and fold the data into
the row's `aria-label`.

#### Story 3.2.1: Wrap agent icon and memory badge in `Tooltip`, add `tabIndex`, fold into `aria-label`

**As a** keyboard-only or screen-reader user, **I want** to reach the agent
program and memory RSS data even though the columns are hidden by default,
**so that** re-enabling them via the Columns picker isn't the only way to get
that information.

**Acceptance Criteria**:
- The agent-icon `<span>` (currently `title={session.program}`, `SessionRow.tsx:452-455`) is wrapped in `<Tooltip label={session.program}>`, gains `tabIndex={0}`, and its bare `title` attribute is removed (the `Tooltip` component supersedes it).
  - *Given* `visibleColumns` includes `"agent"` and the row is rendered, *When* the agent-icon element receives keyboard focus (Tab), *Then* the Radix tooltip content becomes visible (`role="tooltip"` element present in the DOM with `session.program`'s text).
- The memory-badge `<span>` (currently `title={...}`, `SessionRow.tsx:507`) gets the same treatment: `<Tooltip label={`Process RSS: ${memMB} MB`}>` wrapper, `tabIndex={0}`, bare `title` removed.
  - *Given* `visibleColumns` includes `"memory"` and `memMB = 512`, *When* the memory-badge element receives keyboard focus, *Then* the tooltip content shows `"Process RSS: 512 MB"`.
- The row-level `aria-label` (line 306) is extended to include `, agent: ${session.program}, memory: ${memMB} MB` (only when `memMB > 0`, mirroring the existing conditional display logic), regardless of whether those columns are currently visible.
  - *Given* a session with `program = "claude"` and `memMB = 200`, rendered with `agent`/`memory` NOT in `visibleColumns` (the new default), *When* the row's `aria-label` is read, *Then* it contains `agent: claude` and `memory: 200 MB` even though neither column renders as a visible grid cell.

**Files**: `web-app/src/components/sessions/SessionRow.tsx`

##### Task 3.2.1a: Wrap the agent-icon span in `Tooltip`, add `tabIndex` (~4 min)
- Import `Tooltip` (already imported at the top of the file). Wrap the `<span className={agentIconStyle} ...>` block (lines 450-459) in `<Tooltip label={session.program}>`; remove the `title` prop from the inner span; add `tabIndex={0}`.
- Files: `web-app/src/components/sessions/SessionRow.tsx`

##### Task 3.2.1b: Wrap the memory-badge span in `Tooltip`, add `tabIndex` (~4 min)
- Same treatment for the `memoryBadge` block (lines 501-520): wrap in `<Tooltip label={memMB > 0 ? `Process RSS: ${memMB} MB` : "No memory data"}>`; remove `title`; add `tabIndex={0}`.
- Files: `web-app/src/components/sessions/SessionRow.tsx`

##### Task 3.2.1c: Extend the row-level `aria-label` string (~3 min)
- Edit line 306's template literal to append `${session.program ? `, agent: ${session.program}` : ""}${memMB > 0 ? `, memory: ${memMB} MB` : ""}` before the existing `context: lost` clause.
- Files: `web-app/src/components/sessions/SessionRow.tsx`

##### Task 3.2.1d: Manual keyboard-tab check + note the touch caveat (~4 min)
- Using the manual dev instance, Tab through a row and confirm the tooltip appears for the agent icon and memory badge without a mouse. Per the Unresolved Questions section, manually check (or note as unverified) whether tap on a touch device also surfaces it.
- Files: none (verification only)

---

### Epic 3.3: Touch-target sizing for overflow buttons

**Goal**: Meet the 44×44px minimum on `(pointer: coarse)` for the row's "···"
button and confirm the card's overflow button's width, not just height.

#### Story 3.3.1: Fix `rowOverflowButton` and verify `SessionCard`'s `overflowButton` width

**As a** touch-device user, **I want** the "···" overflow buttons to be reliably
tappable, **so that** I don't mis-tap adjacent controls.

**Acceptance Criteria**:
- `rowOverflowButton` (`SessionRow.css.ts:202-217`) gains a `(pointer: coarse)` media block setting `minHeight: 44` and `minWidth: 44`, matching `inlineActionButton`'s existing precedent (lines 188-194).
  - *Given* a `(pointer: coarse)` device, *When* the row's overflow button is measured, *Then* its bounding box is >= 44×44px.
- `SessionCard.css.ts`'s `overflowButton` (line ~498) gains `minWidth: "44px"` alongside its existing `minHeight: "44px"`.
  - *Given* the same measurement, applied to the card's overflow button, *Then* both width and height are >= 44px.
- A new or extended Playwright test in `tests/e2e/touch-targets.spec.ts` asserts both buttons' bounding boxes, since neither is covered by the file's existing `more-actions-button` test (that test targets the session-detail header's button, a different element).
  - *Given* the mobile viewport test setup already in `touch-targets.spec.ts`, *When* a new test locates the row's overflow button via `getByRole("button", { name: /More session actions/i })` and the card's via the same role/name inside a `session-card`, *Then* both assertions pass at >= 44×44px.

**Files**: `web-app/src/components/sessions/SessionRow.css.ts`, `web-app/src/components/sessions/SessionCard.css.ts`, `tests/e2e/touch-targets.spec.ts`

##### Task 3.3.1a: Add `(pointer: coarse)` sizing to `rowOverflowButton` (~3 min)
- Add an `"@media": { "(pointer: coarse)": { minHeight: 44, minWidth: 44, padding: "10px" } }` block, mirroring `inlineActionButton`'s shape.
- Files: `web-app/src/components/sessions/SessionRow.css.ts`

##### Task 3.3.1b: Add `minWidth: "44px"` to `SessionCard.css.ts`'s `overflowButton` (~2 min)
- Files: `web-app/src/components/sessions/SessionCard.css.ts`

##### Task 3.3.1c: Add the two new Playwright assertions to `touch-targets.spec.ts` (~5 min)
- Following `e2e-test-conventions` (ARIA-role locators, no `waitForTimeout`), add two tests inside the mobile-viewport `describe` block using `getByRole("button", { name: /More session actions/i }).first()` for the row case and scoping to `getByTestId("session-card").getByRole("button", { name: /More session actions/i })` for the card case; reuse `assertTouchTarget`'s pattern (or call it directly if a `data-testid` is added — prefer the existing helper's `testId`-based signature by adding `data-testid="session-row-overflow-button"`/`"session-card-overflow-button"` if the role-based locator proves ambiguous with multiple rows on screen).
- Files: `tests/e2e/touch-targets.spec.ts`, possibly `web-app/src/components/sessions/SessionRow.tsx`/`SessionCard.tsx` (if a `data-testid` is added for locator stability)

##### Task 3.3.1d: Run the updated spec locally (~3 min)
- `cd tests/e2e && npx playwright test touch-targets.spec.ts`
- Files: none (verification only)

---

## Phase 4: Regression guard & verification

### Epic 4.1: e2e locator and duplication-gate audit

**Goal**: Confirm none of the ~30+ Playwright specs touching session list/card
break from column removal or row-height changes, and that no new jscpd
duplication was introduced.

#### Story 4.1.1: Grep-audit `tests/e2e/` for column/row-height-sensitive locators

**As a** maintainer, **I want** confirmation the e2e suite still passes after
this redesign, **so that** the density change doesn't silently regress covered
behavior.

**Acceptance Criteria**:
- A grep pass over `tests/e2e/` confirms no spec asserts on the removed grid-cell structure for `agent`/`memory` by anything other than `data-testid`/ARIA role (which still work since those elements still exist in the DOM, just gated behind `visibleColumns`).
  - *Given* the grep pass, *When* run, *Then* every match referencing `agent`/`memory` columns uses `getByTestId`/`getByRole`, not a CSS selector or raw grid-position assumption.
- `tests/e2e/accessibility.spec.ts:513-528`'s pre-existing documented `FAILED`-status rendering discrepancy between `SessionRow`/`SessionCard` is confirmed unchanged (not worsened) by this feature's edits to the same conditional-rendering region of `SessionRow.tsx` (the `isFailed` blocks touched in Epic 1.2/3.2).
  - *Given* the full diff from Phases 1-3, *When* `tests/e2e/accessibility.spec.ts` is run, *Then* the pre-existing documented-bug test's result (pass/skip/xfail, whichever it currently is) is unchanged from the pre-feature baseline.
- The full Playwright e2e suite passes.
  - *Given* `cd tests/e2e && npm test`, *When* run, *Then* the suite exits 0.

**Files**: none new — audit + potential small fix-ups in `tests/e2e/` if a fragile locator is found.

##### Task 4.1.1a: Grep `tests/e2e/` for `agent`/`memory`/row-height-related locators (~5 min)
- `grep -rn "agent\|memory\|estimateSize\|row-height" tests/e2e/*.spec.ts`
- Files: none (audit only)

##### Task 4.1.1b: Re-run and diff `accessibility.spec.ts`'s FAILED-status test against baseline (~4 min)
- `cd tests/e2e && npx playwright test accessibility.spec.ts` before and after the Phase 1-3 diff (via `git stash`-free comparison — check out `main` in a scratch worktree if needed, or simply confirm the test's current pass/skip status and that it's unchanged post-diff).
- Files: none (verification only)

##### Task 4.1.1c: Run the full e2e suite (~10 min, background)
- `cd tests/e2e && npm test`
- Files: none (verification only)

### Epic 4.2: Final verification gate

**Goal**: Confirm the standard project quality gates pass, including the
duplication gate this plan specifically designed around.

#### Story 4.2.1: Run `make quick-check`, web-app tests, and the jscpd duplication gate

**As a** reviewer, **I want** proof the change passes CI-equivalent checks
locally before it's proposed for review, **so that** the PR doesn't bounce on
mechanical failures.

**Acceptance Criteria**:
- `make quick-check` (build + test + lint) passes.
  - *Given* the full diff, *When* `make quick-check` is run, *Then* it exits 0.
- `cd web-app && npx jest --no-coverage` passes, including the new `truncateWorkspacePath.test.ts` and any `SessionRow.test.tsx` additions.
  - *Given* the full diff, *When* run, *Then* all tests pass.
- The web-app jscpd duplication gate passes (confirms Story 1.1.2's consolidation actually avoided introducing new duplication).
  - *Given* the full diff, *When* `pnpm run lint:duplicates` (or `make ready-duplication-gate-web`) is run in `web-app/`, *Then* it passes under the repo's current threshold.

**Files**: none new — verification only.

##### Task 4.2.1a: Run `make quick-check` (~5 min)
- Files: none

##### Task 4.2.1b: Run `cd web-app && npx jest --no-coverage` (~3 min)
- Files: none

##### Task 4.2.1c: Run the jscpd duplication gate (~3 min)
- `cd web-app && pnpm run lint:duplicates`
- Files: none
