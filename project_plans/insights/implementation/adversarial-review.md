# Adversarial Review: insights

**Date**: 2026-05-31
**Verdict**: CONCERNS

---

## Blockers

*(None — the plan is implementable as written.)*

---

## Concerns

- [ ] **`zIndex` is not in `vars` — plan references `vars.zIndex.modal`** — The `theme-contract.css.ts` file confirms `zIndex` is a plain exported constant (`export const zIndex = { ... } as const`), NOT part of the `vars` theme contract. The plan's `SessionDetailDrawer.css.ts` task instructs using `vars.zIndex.modal` which does not exist. Correct pattern: `import { zIndex } from '@/styles/theme-contract.css'` and reference `zIndex.modal` (1000). The plan also proposes adding a `slideOver: 700` slot — the `zIndex` constant map already has `dropdown: 500` and `modal: 1000`. A drawer at 700 would sit below the existing `bottomNavMoreBackdrop: 1040` and `dialog: 1070`, which is correct for a page-level slide-over. Task 2.1.1a must be corrected: import `zIndex` (the constant), not `vars.zIndex`. — **Recommendation**: Fix the import pattern in Task 2.1.1a instructions; add `slideOver: 700` to the `zIndex` constant (not `vars`).

- [ ] **Budget warning state — `isWarning` is computed inside `ProjectedCostCard` but the warning banner in `InsightsDashboard` needs it too** — Task 3.1.2c says "add warning banner at top of page when `projection?.isWarning` (read from state lifted from card OR duplicate threshold check)". The `ProjectedCostCard` owns the `budgetThreshold` localStorage state. The dashboard can't read the card's internal state without lifting it. The plan acknowledges the ambiguity with "OR duplicate threshold check" but doesn't resolve it. Two options: (a) lift budget threshold state to `InsightsDashboard`, (b) accept the duplication. Option (a) is cleaner but adds a `useBudgetThreshold` hook or state to `InsightsDashboard`. The plan must pick one. — **Recommendation**: Lift `budgetThreshold` and `isHydrated` out of `ProjectedCostCard` into a `useBudgetThreshold` hook (`web-app/src/lib/hooks/useBudgetThreshold.ts`). Both `InsightsDashboard` (for the banner) and `ProjectedCostCard` (for the input UI) use this hook. Add this to the file change list.

- [ ] **`InsightsDashboardSkeleton` layout instruction uses inline styles** — Task 4.1.1a says "No `.css.ts` needed — use inline `style={{ display: 'grid', ... }}` only for layout grid". This directly contradicts the css-architecture rule: "No inline styles for layout". The instruction should instead say: import the existing `grid2` class from `InsightsDashboard.css.ts` (already exported) and use it for the grid layout in the skeleton. No new `.css.ts` file needed, but no inline `style` either. — **Recommendation**: Change Task 4.1.1a instruction to import and reuse `grid2` from `InsightsDashboard.css.ts` instead of using `style={{ display: 'grid' }}`.

- [ ] **`useInsightsSummary` creates `client` and `transport` on every render** — Reading the actual hook source, `transport` and `client` are created inline in the component function body (not memoized, not in a ref). On every render, a new transport + client instance is created. This is a pre-existing issue but the plan adds `filters.from` and `filters.to` to `useCallback` deps, which will trigger more `fetchSummary` recreations (and thus `startWatch` recreations and stream restarts) on every Date object reference change. If the caller passes `new Date()` inline (e.g., `filters.from: preset === 'today' ? new Date() : undefined`), the filter will change identity on every render and cause infinite re-fetch loops. — **Recommendation**: Add a task (1.1.1c) to memoize the `from`/`to` Date objects in `InsightsDashboard` using `useMemo` keyed on the preset/ISO string from URL params — never pass `new Date()` directly. Also note that `client`/`transport` should be memoized with `useMemo`/`useRef` in `useInsightsSummary` to prevent re-creation on re-renders.

- [ ] **`TableVirtuoso` plain table fallback (<50 rows) creates two code paths to maintain** — The plan says "Keep plain `<table>` path for `displayed.length <= 50`". This means two rendering paths for the same data, doubling the JSX to maintain for any future column or row changes. The `click`/`onSessionClick` handler and filter bar must work in both paths. This isn't wrong but adds maintenance burden that wasn't justified in the plan. — **Recommendation**: Either remove the fallback (always use `TableVirtuoso`) or document clearly that both paths must be updated in sync for any column/row changes. The threshold of 50 is also arbitrary — consider always using `TableVirtuoso` since it has negligible overhead at low row counts.

---

## Minors

- Task 1.1.3a: Using `new Fuse(sessions, ...)` inside `useMemo` is correct, but Fuse constructor accepts a key path as `'projectPath'` (camelCase) which corresponds to the TypeScript property name. Confirm the proto-generated field name in TypeScript is `projectPath` (not `project_path`) — based on the hook source it is indeed camelCase. No action needed, just verify.

- Task 4.4.2a: The surgical patch uses `findIndex` which is O(n) per event. With 500+ sessions and frequent update events, this could accumulate. A `Map<string, number>` index would make it O(1). Low priority since update events are sparse.

- Task 1.1.2d says to import `InsightsDashboardSkeleton` in `page.tsx` but `InsightsDashboardSkeleton` is created in Task 4.1.1a (Epic 4). The dependency ordering in the plan puts Task 4.1.1a in Phase 4, but Task 1.1.2d depends on it. Implementation order should be: create skeleton first, then wrap in Suspense. The plan's task numbering suggests otherwise. No plan change needed, just implementation ordering awareness.

- Requirements R2 states the detail view should show a "Turn timeline: ordered list of assistant turns with per-turn tokens". The plan correctly flags this as impossible with the current proto and uses `message_count` as a proxy. The plan should ensure the UI labels this clearly as "Message count" (not "Turns timeline") to avoid misleading users expecting per-turn data. Task 2.1.1b should be explicit about the label text.

- The `custom` preset in `TimeRangeFilter` requires date range validation — if `from > to`, the filter should either prevent submission or display a validation error. Task 1.1.2b does not mention this. Low risk (bad dates result in empty data, not crashes) but a UX concern.
