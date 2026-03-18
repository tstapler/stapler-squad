# Feature Plan: Decompose app/history/page.tsx

**Date**: 2026-03-18
**Status**: Draft
**Scope**: Refactor `app/history/page.tsx` from a 1,190-line monolith into a thin orchestrator page (<150 lines) with extracted hooks and child components

---

## Table of Contents

- [Problem Statement](#problem-statement)
- [Research Findings: Current Implementation](#research-findings-current-implementation)
- [ADR-015: Hook vs Component for Grouping Logic](#adr-015-hook-vs-component-for-grouping-logic)
- [ADR-016: Filter State Persistence Strategy](#adr-016-filter-state-persistence-strategy)
- [ADR-017: TypeScript Timestamp Typing from Protobuf](#adr-017-typescript-timestamp-typing-from-protobuf)
- [Architecture Overview](#architecture-overview)
- [Story 1: Extract useHistoryFilters Hook](#story-1-extract-usehistoryfilters-hook)
- [Story 2: Extract useHistoryGrouping Hook](#story-2-extract-usehistorygrouping-hook)
- [Story 3: Extract HistoryFilterBar Component](#story-3-extract-historyfilterbar-component)
- [Story 4: Extract HistoryGroupView and HistoryEntryCard Components](#story-4-extract-historygroupview-and-historyentrycard-components)
- [Story 5: Fix Timestamp Types and Slim Page to Orchestrator](#story-5-fix-timestamp-types-and-slim-page-to-orchestrator)
- [Known Issues and Bug Risks](#known-issues-and-bug-risks)
- [Testing Strategy](#testing-strategy)
- [Dependency Graph](#dependency-graph)

---

## Problem Statement

`web-app/src/app/history/page.tsx` is 1,190 lines long. It functions as a component
rather than a page: it contains inline sorting comparators, filtering logic, grouping
algorithms, date formatting utilities, localStorage persistence, keyboard navigation,
full-text search orchestration, and the complete render tree (filter bar, grouped entry
list, detail panel, messages modal).

This violates Next.js App Router conventions where pages should be thin orchestrators
that compose hooks and components. The reference page `app/page.tsx` (332 lines)
demonstrates proper separation: it delegates to `useSessionService`, `useKeyboard`,
`SessionList`, `SessionDetail`, and other focused modules.

**Consequences of the current structure:**
1. No unit-testable business logic -- filter/sort/group logic is buried in `useMemo` calls inside a page component.
2. Changes to filter UI require reading 1,190 lines of context.
3. Impossible to reuse grouping or filtering logic elsewhere (e.g., a future dashboard widget).
4. 4 uses of `any` for protobuf `Timestamp` values suppress TypeScript's safety.
5. The page re-renders entirely on every state change because all state lives in one component.

**Goal:** Reduce `page.tsx` to <150 lines. Extract logic into testable hooks and rendering
into focused components. Fix all `any` types.

---

## Research Findings: Current Implementation

### State Inventory (page.tsx lines 142-170)

| State Variable | Type | Purpose | Target Location |
|---|---|---|---|
| `entries` | `ClaudeHistoryEntry[]` | Raw loaded entries | Page (data loading) |
| `selectedEntry` | `ClaudeHistoryEntry \| null` | Currently selected entry | Page (selection) |
| `selectedIndex` | `number` | Keyboard nav index | Page (selection) |
| `messages` / `previewMessages` | `ClaudeMessage[]` | Modal and preview messages | Page (detail) |
| `showMessages` / `loadingMessages` / `loadingPreview` | `boolean` | Modal state | Page (detail) |
| `loading` / `searching` / `error` / `resuming` | `boolean \| string` | Loading states | Page (data loading) |
| `searchQuery` | `string` | Metadata search text | `useHistoryFilters` |
| `selectedModel` | `string` | Model dropdown filter | `useHistoryFilters` |
| `dateFilter` | `DateFilter` | Date range filter | `useHistoryFilters` |
| `sortField` | `SortField` | Sort column | `useHistoryFilters` |
| `sortOrder` | `SortOrder` | Sort direction | `useHistoryFilters` |
| `groupingStrategy` | `HistoryGroupingStrategy` | Group-by mode | `useHistoryFilters` |
| `searchMode` | `SearchMode` | metadata vs fulltext toggle | `useHistoryFilters` |
| `messageSearchQuery` | `string` | Modal message filter | Messages modal component |

### Utility Functions (lines 52-131)

6 functions defined at module scope:
- `loadFromStorage` / `saveToStorage` -- generic localStorage helpers
- `formatTimeAgo` / `formatDate` -- timestamp formatting (use `any` type)
- `truncateMiddle` -- string truncation
- `getDateGroup` / `isWithinDateFilter` -- date classification (use `any` type)

### Derived Data (lines 300-416)

4 `useMemo` blocks compute:
1. `uniqueModels` -- unique model strings for dropdown
2. `filteredEntries` -- filter + sort pipeline
3. `groupedEntries` -- group computation from filtered results
4. `flatEntries` -- flattened for keyboard navigation
5. `filteredMessages` -- modal message search

### Render Tree (lines 653-1189)

Approximately 536 lines of JSX covering:
- Page header with grouping indicator
- Error banner
- Filter bar (search mode toggle, metadata search, full-text search, dropdowns)
- Content area (entry list with groups OR full-text search results)
- Detail panel (entry details, message preview, action buttons)
- Keyboard hints bar
- Messages modal (with search and message list)

### Protobuf Timestamp Type

The generated `Timestamp` class from `@bufbuild/protobuf` has these properties:
- `seconds: bigint` -- UTC seconds since Unix epoch
- `nanos: number` -- nanosecond fraction
- `toDate(): Date` -- converts to JS Date

The `ClaudeHistoryEntry` type declares:
- `createdAt?: Timestamp`
- `updatedAt?: Timestamp`

The `ClaudeMessage` type declares:
- `timestamp?: Timestamp`

All 4 `any` usages in `page.tsx` (lines 71, 84, 99, 115) accept these optional
`Timestamp` values but type them as `any` instead of `Timestamp | undefined`.

### Existing Test Infrastructure

The project uses Jest with `ts-jest`, `jest-environment-jsdom`, and
`@testing-library/react`. Tests live alongside source in `__tests__/` directories
or as `*.test.ts(x)` files. The Jest config at `web-app/jest.config.js` maps
`@/` to `<rootDir>/src/`.

---

## ADR-015: Hook vs Component for Grouping Logic

### Context

The grouping logic (lines 356-399) transforms a flat array of `ClaudeHistoryEntry[]`
into an array of `{ groupKey, displayName, entries }` objects. This logic needs to
live somewhere after extraction. The question is whether it belongs in a custom hook
(`useHistoryGrouping`) or in a component (`HistoryGroupView`).

### Options Considered

| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| A. Custom hook (`useHistoryGrouping`) | Pure data transformation hook that takes filtered entries and strategy, returns grouped entries | Testable without rendering; reusable; follows existing hook patterns in codebase | One more hook in the dependency chain |
| B. Component-internal logic | `HistoryGroupView` receives flat entries and groups internally | Fewer files; grouping is a rendering concern | Not testable without rendering; couples grouping algorithm to view; impossible to reuse grouping logic |
| C. Plain utility function | Export a `groupEntries()` function from a utils module | Simplest; no hook overhead; easily testable | Loses memoization; caller must manage `useMemo` |

### Decision

**Option A: Custom hook (`useHistoryGrouping`)**

The grouping logic involves non-trivial sorting (date groups have a specific ordering),
handles multiple strategies, and its output drives both the grouped view and the
flattened keyboard-navigation array. Making it a hook allows:
- Memoization via `useMemo` is encapsulated
- Unit testing with plain Jest (no render needed -- hooks can be tested with `renderHook`)
- The `HistoryGroupView` component receives pre-computed groups and focuses purely on rendering
- The `flatEntries` derivation can live in the same hook since it depends on groups

### Rationale

The codebase already has 19 hooks in `web-app/src/lib/hooks/`. This follows the
established pattern. The grouping algorithm is a pure function of `(filteredEntries, strategy)`,
making it a natural fit for a hook that memoizes the computation.

---

## ADR-016: Filter State Persistence Strategy

### Context

Currently, filter state is persisted to `localStorage` via 7 separate `useEffect` calls
(lines 197-203), each watching one state variable. The persisted keys are defined in the
`STORAGE_KEYS` constant. On mount, a hydration effect (lines 173-182) reads all values back.

The question is whether to keep localStorage persistence or move to URL search params
(`useSearchParams`), which would enable deep-linking to specific filter configurations.

### Options Considered

| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| A. Keep localStorage (current) | Extract the same pattern into `useHistoryFilters` | No behavior change; safe refactor; users keep preferences across sessions | No deep-linking; cannot share filter URLs; SSR hydration complexity persists |
| B. URL search params (`useSearchParams`) | Store sort/filter/group in URL query string | Deep-linkable; shareable; SSR-friendly; Next.js native | All filter changes update URL (noisy browser history); bookmarked URLs may break if param names change; requires `Suspense` boundary for `useSearchParams` |
| C. Hybrid: URL for key filters, localStorage for preferences | Put `searchQuery` and `dateFilter` in URL; keep `sortField`, `sortOrder`, `groupingStrategy` in localStorage | Deep-link search queries; keep UI preferences sticky | Most complex; two persistence mechanisms; confusing mental model |

### Decision

**Option A: Keep localStorage persistence, encapsulated in the hook.**

Rationale:
1. This is a refactoring story, not a feature story. Changing persistence strategy
   introduces new behavior and new bugs.
2. The History Browser is typically accessed from the nav, not via shared links.
3. Deep-linking can be added later by swapping the hook's internal persistence mechanism
   without changing any consumers. The hook API (`filterState` + setters) stays the same.
4. The hydration dance (SSR renders defaults, client reads localStorage) is already
   working and well-understood. Moving to `useSearchParams` would require wrapping
   the page in `Suspense` and dealing with different Next.js 14 App Router behaviors.

The `useHistoryFilters` hook will encapsulate `loadFromStorage` / `saveToStorage` and
the hydration flag, so the page never sees these implementation details.

**Future migration path:** If deep-linking is desired later, the hook can internally
switch from localStorage to `useSearchParams`. The hook's return type stays identical.
This is a one-file change in the hook, zero changes in consumers.

---

## ADR-017: TypeScript Timestamp Typing from Protobuf

### Context

The `page.tsx` file has 4 uses of `any` for timestamp parameters:

```typescript
const formatTimeAgo = (timestamp: any): string => { ... }
const formatDate = (timestamp: any): string => { ... }
const getDateGroup = (timestamp: any): string => { ... }
const isWithinDateFilter = (timestamp: any, filter: DateFilter): boolean => { ... }
```

All callers pass `ClaudeHistoryEntry.updatedAt`, `ClaudeHistoryEntry.createdAt`, or
`ClaudeMessage.timestamp`, which are typed as `Timestamp | undefined` where `Timestamp`
is `@bufbuild/protobuf`'s `google.protobuf.Timestamp` class.

### Options Considered

| Option | Description | Pros | Cons |
|--------|-------------|------|------|
| A. Use `Timestamp \| undefined` directly | Import `Timestamp` from `@bufbuild/protobuf` and use it as the parameter type | Exact type; uses protobuf's `seconds: bigint` correctly; `Timestamp.toDate()` available | Couples utility functions to protobuf; `seconds` is `bigint`, current code uses `Number()` conversion |
| B. Define local interface | `interface TimestampLike { seconds: bigint \| number; nanos?: number }` | Decoupled from protobuf; works with plain objects in tests | Parallel type definition; may drift from protobuf |
| C. Convert to `Date` at boundary | All hooks/components receive `Date \| null` instead of `Timestamp` | Standard JS type; no protobuf dependency in UI layer | Conversion overhead at boundary; loses nanosecond precision; changes data flow |

### Decision

**Option A: Use `Timestamp | undefined` directly.**

Rationale:
1. The `Timestamp` type is already imported in `useHistoryFullTextSearch.ts` (line 13),
   so there is precedent for using it directly in hooks.
2. The `Timestamp` class has `.seconds` (bigint) and `.toDate()` (returns JS Date).
   Using `Number(timestamp.seconds)` is the existing pattern and works correctly for
   timestamps in the practical range (bigint to number is safe for dates before year 2255).
3. Utility functions that accept `Timestamp | undefined` can be tested by constructing
   `Timestamp` from `Timestamp.fromDate(new Date(...))` in tests.
4. This is the zero-conversion approach -- no new boundary mapping needed.

The utility functions will be moved to a shared module
`web-app/src/lib/utils/timestamp.ts` with signatures like:

```typescript
import { Timestamp } from "@bufbuild/protobuf";

export function formatTimeAgo(timestamp: Timestamp | undefined): string;
export function formatDate(timestamp: Timestamp | undefined): string;
export function getDateGroup(timestamp: Timestamp | undefined): string;
export function isWithinDateFilter(timestamp: Timestamp | undefined, filter: DateFilter): boolean;
```

---

## Architecture Overview

### Before (Current)

```
app/history/page.tsx (1,190 lines)
  |- Types, constants, utility functions (lines 1-131)
  |- All state (lines 142-170)
  |- All effects (lines 173-211)
  |- Data loading callbacks (lines 222-294)
  |- Derived data / useMemo (lines 300-416)
  |- Event handlers (lines 422-529)
  |- Keyboard navigation (lines 535-647)
  |- Render: filter bar, entry list, detail panel, modal (lines 653-1189)
```

### After (Target)

```
app/history/page.tsx (~120 lines)
  |- Imports hooks and components
  |- Data loading (entries, client ref)
  |- Selection state (selectedEntry, messages, modal)
  |- Composes: useHistoryFilters, useHistoryGrouping, useHistoryFullTextSearch
  |- Renders: HistoryFilterBar, HistoryGroupView, HistoryDetailPanel, HistoryMessagesModal
  |- Keyboard navigation wiring

lib/utils/timestamp.ts (~60 lines)
  |- formatTimeAgo(Timestamp | undefined): string
  |- formatDate(Timestamp | undefined): string
  |- getDateGroup(Timestamp | undefined): string
  |- isWithinDateFilter(Timestamp | undefined, DateFilter): boolean
  |- truncateMiddle(string, number): string

lib/hooks/useHistoryFilters.ts (~120 lines)
  |- Types: SortField, SortOrder, DateFilter, SearchMode, HistoryGroupingStrategy
  |- State: searchQuery, selectedModel, dateFilter, sortField, sortOrder,
  |         groupingStrategy, searchMode, isHydrated
  |- localStorage persistence (encapsulated)
  |- Derived: uniqueModels, filteredEntries (filter + sort pipeline)
  |- Actions: setters, clearFilters, cycleGroupingStrategy

lib/hooks/useHistoryGrouping.ts (~60 lines)
  |- Input: filteredEntries, groupingStrategy
  |- Output: groupedEntries, flatEntries
  |- Memoized group computation with date-order sorting

components/history/HistoryFilterBar.tsx (~130 lines)
  |- Search mode toggle (metadata / fulltext)
  |- Metadata search input
  |- Full-text search input (delegates to HistorySearchInput)
  |- Model, date, sort, grouping dropdowns
  |- Sort order toggle button

components/history/HistoryGroupView.tsx (~100 lines)
  |- Loading skeleton
  |- Empty states (no results, no history, no match)
  |- Group headers with entry count
  |- Renders HistoryEntryCard for each entry

components/history/HistoryEntryCard.tsx (~50 lines)
  |- Single entry card: name, time, model, message count, project path
  |- Selected state styling
  |- Click and keyboard handlers

components/history/HistoryDetailPanel.tsx (~120 lines)
  |- Entry detail fields
  |- Message preview section
  |- Action buttons (resume, view messages, export, copy ID)

components/history/HistoryMessagesModal.tsx (~90 lines)
  |- Modal overlay with focus trap
  |- Message search input
  |- Message list with role-based styling
```

### Data Flow

```
page.tsx
  |-- loadHistory() --> entries
  |
  |-- useHistoryFilters(entries)
  |     |-- filteredEntries (filtered + sorted)
  |     |-- filter state + setters
  |     |-- uniqueModels
  |
  |-- useHistoryGrouping(filteredEntries, groupingStrategy)
  |     |-- groupedEntries
  |     |-- flatEntries
  |
  |-- useHistoryFullTextSearch() (existing, unchanged)
  |
  |-- <HistoryFilterBar
  |     filterState={...}
  |     fullTextSearch={...}
  |   />
  |
  |-- <HistoryGroupView               (or <HistorySearchResults> when fulltext mode)
  |     groupedEntries={...}
  |     selectedEntry={...}
  |     onSelectEntry={...}
  |   />
  |
  |-- <HistoryDetailPanel
  |     entry={selectedEntry}
  |     previewMessages={...}
  |     onResume={...}
  |     onViewMessages={...}
  |     onExport={...}
  |   />
  |
  |-- <HistoryMessagesModal
  |     open={showMessages}
  |     messages={messages}
  |     onClose={...}
  |   />
```

---

## Story 1: Extract useHistoryFilters Hook

**Goal:** Move all filter/sort/group state and the filter+sort pipeline out of page.tsx
into a dedicated hook.

### Files to Create

- `web-app/src/lib/hooks/useHistoryFilters.ts`
- `web-app/src/lib/hooks/__tests__/useHistoryFilters.test.ts`
- `web-app/src/lib/utils/timestamp.ts`
- `web-app/src/lib/utils/__tests__/timestamp.test.ts`

### Files to Modify

- `web-app/src/app/history/page.tsx` -- replace inline state and useMemo with hook call

### Acceptance Criteria

**Given** the page imports `useHistoryFilters`,
**When** the hook is called with `entries: ClaudeHistoryEntry[]`,
**Then** it returns:
- `filterState`: `{ searchQuery, selectedModel, dateFilter, sortField, sortOrder, groupingStrategy, searchMode, isHydrated }`
- `setters`: `{ setSearchQuery, setSelectedModel, setDateFilter, setSortField, setSortOrder, setGroupingStrategy, setSearchMode }`
- `derived`: `{ uniqueModels, filteredEntries, hasActiveFilters }`
- `actions`: `{ clearFilters, cycleGroupingStrategy }`

**Given** filter state changes,
**When** `filteredEntries` is accessed,
**Then** it reflects the filter, search, and sort applied to the input entries (memoized).

**Given** a filter state change,
**When** the component re-renders,
**Then** the new value is persisted to localStorage.

**Given** the hook mounts on the client,
**When** localStorage has persisted values,
**Then** the hook hydrates state from localStorage after initial render.

### Tasks

1. **Create `web-app/src/lib/utils/timestamp.ts`** -- Extract `formatTimeAgo`, `formatDate`, `getDateGroup`, `isWithinDateFilter`, and `truncateMiddle` from `page.tsx`. Change the `any` parameter type to `Timestamp | undefined`. Import `Timestamp` from `@bufbuild/protobuf`.

2. **Create `web-app/src/lib/utils/__tests__/timestamp.test.ts`** -- Test all 4 timestamp functions with: valid Timestamp, undefined input, boundary values (exactly now, 59 seconds ago, 1 hour ago, etc.), and the date filter variants (today, week, month, all).

3. **Create `web-app/src/lib/hooks/useHistoryFilters.ts`** -- Move types (`SortField`, `SortOrder`, `DateFilter`, `SearchMode`, `HistoryGroupingStrategy`, `GroupingStrategyLabels`, `STORAGE_KEYS`), the `loadFromStorage`/`saveToStorage` helpers, all filter state declarations, all persistence effects, the `uniqueModels` useMemo, the `filteredEntries` useMemo, `hasActiveFilters`, `clearFilters`, and `cycleGroupingStrategy` into this hook. The hook signature: `useHistoryFilters(entries: ClaudeHistoryEntry[])`.

4. **Create `web-app/src/lib/hooks/__tests__/useHistoryFilters.test.ts`** -- Test with `@testing-library/react`'s `renderHook`:
   - Filtering by model returns only matching entries
   - Filtering by date range correctly includes/excludes entries
   - Search query filters by name, project, and model (case-insensitive)
   - Sort by each field in both directions
   - `clearFilters` resets all filter state to defaults
   - `cycleGroupingStrategy` cycles through all strategies
   - `hasActiveFilters` is true when any filter is non-default
   - Hydration: mock localStorage, verify state loads on mount

5. **Update `web-app/src/app/history/page.tsx`** -- Replace the 7 filter state declarations, 7 persistence effects, hydration effect, `uniqueModels` useMemo, `filteredEntries` useMemo, `hasActiveFilters`, `clearFilters`, and `cycleGroupingStrategy` with a single `const { filterState, setters, derived, actions } = useHistoryFilters(entries)` call. Remove the extracted utility functions and types from the file. Import them from their new locations.

---

## Story 2: Extract useHistoryGrouping Hook

**Goal:** Move the grouping computation out of page.tsx into a dedicated hook.

### Files to Create

- `web-app/src/lib/hooks/useHistoryGrouping.ts`
- `web-app/src/lib/hooks/__tests__/useHistoryGrouping.test.ts`

### Files to Modify

- `web-app/src/app/history/page.tsx` -- replace `groupedEntries` and `flatEntries` useMemo with hook call

### Acceptance Criteria

**Given** the hook is called with `filteredEntries` and `groupingStrategy`,
**When** `groupingStrategy` is `None`,
**Then** it returns a single group `{ groupKey: "all", displayName: "All Entries", entries: filteredEntries }`.

**Given** `groupingStrategy` is `Date`,
**When** entries span multiple date ranges,
**Then** groups are returned in the order: Today, Yesterday, This Week, This Month, Older, Unknown.

**Given** `groupingStrategy` is `Project` or `Model`,
**When** entries have varying project/model values,
**Then** groups are sorted alphabetically, and entries with no value are grouped under "No Project" or "Unknown Model".

**Given** grouped entries change,
**When** `flatEntries` is accessed,
**Then** it is a flat array of all entries across all groups, preserving group order.

### Tasks

1. **Create `web-app/src/lib/hooks/useHistoryGrouping.ts`** -- Move the `groupedEntries` useMemo (lines 356-399) and `flatEntries` useMemo (lines 402-404). The hook imports `getDateGroup` from `lib/utils/timestamp.ts` and `HistoryGroupingStrategy` from `useHistoryFilters.ts`. Returns `{ groupedEntries, flatEntries }`. Export the `HistoryGroup` interface: `{ groupKey: string; displayName: string; entries: ClaudeHistoryEntry[] }`.

2. **Create `web-app/src/lib/hooks/__tests__/useHistoryGrouping.test.ts`** -- Test:
   - `None` strategy returns single group with all entries
   - `Date` strategy groups correctly and sorts in date order
   - `Project` strategy groups by project, alphabetically, with "No Project" fallback
   - `Model` strategy groups by model with "Unknown Model" fallback
   - Empty input returns empty groups (no crash)
   - `flatEntries` preserves group ordering

3. **Update `web-app/src/app/history/page.tsx`** -- Replace inline `groupedEntries` and `flatEntries` useMemo calls with `const { groupedEntries, flatEntries } = useHistoryGrouping(filteredEntries, filterState.groupingStrategy)`.

---

## Story 3: Extract HistoryFilterBar Component

**Goal:** Move the filter bar JSX (lines 692-813) into a dedicated component.

### Files to Create

- `web-app/src/components/history/HistoryFilterBar.tsx`
- `web-app/src/components/history/HistoryFilterBar.module.css` (or reuse parent styles)

### Files to Modify

- `web-app/src/components/history/index.ts` -- add export
- `web-app/src/app/history/page.tsx` -- replace inline filter bar JSX with component

### Acceptance Criteria

**Given** the `HistoryFilterBar` component receives filter state, setters, and search props,
**When** the user changes a dropdown,
**Then** the corresponding setter is called.

**Given** `searchMode` is `"metadata"`,
**When** the filter bar renders,
**Then** the metadata search input and Search/Clear buttons are shown.

**Given** `searchMode` is `"fulltext"`,
**When** the filter bar renders,
**Then** the `HistorySearchInput` component is shown with full-text search props.

**Given** filter state values,
**When** dropdowns render,
**Then** each dropdown shows the current selected value.

### Tasks

1. **Create `web-app/src/components/history/HistoryFilterBar.tsx`** -- Extract the JSX from lines 692-813 of `page.tsx` into a new component. Define props interface with: all filter state values, all setters, `uniqueModels: string[]`, `searchInputRef`, `searching: boolean`, `hasActiveFilters: boolean`, full-text search props (`fullTextSearch` object), `onSearch: () => void`. Move the relevant CSS class references.

2. **Create `web-app/src/components/history/HistoryFilterBar.module.css`** -- Move filter-bar-specific styles from `history.module.css`. Keep shared styles (`.searchModeToggle`, `.searchModeButton`, `.searchContainer`, `.searchInput`, `.searchButton`, `.filters`, `.select`, `.sortOrderButton`, `.fullTextSearchInput`, `.spinnerSmall`) in the new module. The parent `history.module.css` retains layout and non-filter styles.

3. **Update `web-app/src/components/history/index.ts`** -- Add `export { HistoryFilterBar } from "./HistoryFilterBar"`.

4. **Update `web-app/src/app/history/page.tsx`** -- Replace the filter bar JSX block with `<HistoryFilterBar ... />`. Pass all required props from hook returns.

---

## Story 4: Extract HistoryGroupView and HistoryEntryCard Components

**Goal:** Move the grouped entry list rendering (lines 818-938), detail panel (lines 942-1075),
and messages modal (lines 1088-1187) into dedicated components.

### Files to Create

- `web-app/src/components/history/HistoryGroupView.tsx`
- `web-app/src/components/history/HistoryEntryCard.tsx`
- `web-app/src/components/history/HistoryDetailPanel.tsx`
- `web-app/src/components/history/HistoryMessagesModal.tsx`

### Files to Modify

- `web-app/src/components/history/index.ts` -- add exports
- `web-app/src/app/history/page.tsx` -- replace inline JSX with components

### Acceptance Criteria

**HistoryEntryCard:**
**Given** an entry and selection state,
**When** rendered,
**Then** it shows entry name, time ago, model, message count, and project path.

**Given** the entry is selected,
**When** rendered,
**Then** it has the `selected` CSS class.

**HistoryGroupView:**
**Given** `groupedEntries` with multiple groups,
**When** rendered,
**Then** each group has a header with display name and count, followed by entry cards.

**Given** `loading` is true,
**When** rendered,
**Then** it shows the loading skeleton.

**Given** `filteredEntries` is empty and `hasActiveFilters` is true,
**When** rendered,
**Then** it shows the "No results found" empty state with a "clear all filters" button.

**HistoryDetailPanel:**
**Given** a selected entry,
**When** rendered,
**Then** it shows entry details, message preview, and action buttons.

**Given** no selected entry,
**When** rendered,
**Then** it shows the "Select an entry" empty state.

**HistoryMessagesModal:**
**Given** `open` is true,
**When** rendered,
**Then** it shows the modal overlay with message list, search, and close button.

**Given** `open` is false,
**When** rendered,
**Then** nothing is rendered.

### Tasks

1. **Create `web-app/src/components/history/HistoryEntryCard.tsx`** -- Extract the entry card JSX (lines 898-930). Props: `entry: ClaudeHistoryEntry`, `isSelected: boolean`, `onSelect: () => void`. Import `formatTimeAgo` and `truncateMiddle` from `lib/utils/timestamp.ts`.

2. **Create `web-app/src/components/history/HistoryGroupView.tsx`** -- Extract the grouped entry list (lines 818-938) including loading, empty states, and grouped rendering. Props: `groupedEntries: HistoryGroup[]`, `flatEntries: ClaudeHistoryEntry[]`, `selectedEntry: ClaudeHistoryEntry | null`, `loading: boolean`, `entriesCount: number`, `filteredCount: number`, `hasActiveFilters: boolean`, `groupingStrategy: HistoryGroupingStrategy`, `onSelectEntry: (entry, index) => void`, `onClearFilters: () => void`. Renders `HistoryEntryCard` for each entry.

3. **Create `web-app/src/components/history/HistoryDetailPanel.tsx`** -- Extract the detail panel (lines 942-1075). Props: `entry: ClaudeHistoryEntry | null`, `previewMessages: ClaudeMessage[]`, `loadingPreview: boolean`, `resuming: boolean`, `onResume: (entry) => void`, `onViewMessages: (id) => void`, `onExport: (entry) => void`, `onCopyId: (id) => void`. Import `formatDate` from `lib/utils/timestamp.ts`.

4. **Create `web-app/src/components/history/HistoryMessagesModal.tsx`** -- Extract the messages modal (lines 1088-1187). Props: `open: boolean`, `messages: ClaudeMessage[]`, `messageSearchQuery: string`, `onSearchChange: (query) => void`, `onClose: () => void`. Includes `filteredMessages` useMemo and focus trap logic (lines 619-647). Import `formatDate` from `lib/utils/timestamp.ts`.

5. **Update `web-app/src/components/history/index.ts`** -- Add all new exports.

6. **Update `web-app/src/app/history/page.tsx`** -- Replace all inline JSX with component calls.

---

## Story 5: Fix Timestamp Types and Slim Page to Orchestrator

**Goal:** Ensure all `any` types are eliminated, the page is under 150 lines, CSS is
properly split, and all imports are clean.

### Files to Modify

- `web-app/src/app/history/page.tsx` -- final slimming pass
- `web-app/src/app/history/history.module.css` -- remove styles that moved to child components

### Acceptance Criteria

**Given** the refactoring is complete,
**When** `page.tsx` is counted,
**Then** it has fewer than 150 lines.

**Given** the codebase is searched for `any` in history-related files,
**When** the search runs,
**Then** zero results are found (excluding the ConnectRPC client ref, which can use the
proper generic type `ReturnType<typeof createPromiseClient<typeof SessionService>>`
as demonstrated in `useHistoryFullTextSearch.ts` line 114).

**Given** the existing test suite runs (`cd web-app && npm test`),
**When** all tests execute,
**Then** they pass without regressions.

**Given** the application builds (`cd web-app && npm run build`),
**When** the build completes,
**Then** zero TypeScript errors are reported.

### Tasks

1. **Fix the `clientRef` type in page.tsx** -- Change `useRef<any>(null)` to `useRef<ReturnType<typeof createPromiseClient<typeof SessionService>> | null>(null)`, matching the pattern in `useHistoryFullTextSearch.ts`.

2. **Review page.tsx line count** -- After Stories 1-4, the page should contain only: imports, the `HistoryBrowserPage` function with data loading state, hook calls, event handler wiring, keyboard effect, and the composed JSX return. Target: <150 lines.

3. **Clean up `history.module.css`** -- Remove CSS classes that were moved to child component modules. Retain only: `.container`, `.header`, `.title`, `.groupingIndicator`, `.shortcutHint`, `.errorBanner`, `.errorContent`, `.errorIcon`, `.errorTitle`, `.content`, `.keyboardHints` and related kbd styles. Verify no broken class references.

4. **Run full validation** -- Execute `cd web-app && npm run build && npm test` to verify zero TypeScript errors and all tests pass.

5. **Verify no remaining `any` types** -- Search all history-related files for `: any` and confirm zero matches.

---

## Known Issues and Bug Risks

### Bug Risk: Hydration Mismatch on Filter State [SEVERITY: Medium]

**Description**: The current localStorage hydration pattern (render with defaults on
server, then overwrite with localStorage values in a `useEffect`) works because the
initial render matches the server render. If the extraction changes the timing of when
`isHydrated` is set to `true`, the `filteredEntries` computation may flash with defaults
before showing the persisted filter state.

**Mitigation**:
- The `useHistoryFilters` hook must preserve the exact same hydration pattern: state
  starts at defaults, then a single `useEffect([], ...)` loads from localStorage and
  sets `isHydrated = true`.
- Do NOT eagerly read localStorage in `useState` initializers -- this causes hydration
  mismatches in Next.js App Router (SSR render has no `window`).
- Test: Add a test that verifies hook returns `isHydrated: false` on first render and
  `isHydrated: true` after the effect runs.

**Files Likely Affected**:
- `web-app/src/lib/hooks/useHistoryFilters.ts`

**Prevention Strategy**:
- Copy the hydration pattern exactly from the current page.tsx lines 155-182.
- Keep the `isHydrated` flag in the hook's return value so the page can conditionally
  render (currently it does not gate on hydration, but the flag is available).

### Bug Risk: Empty Group Handling [SEVERITY: Low]

**Description**: When all entries are filtered out (e.g., search matches nothing), the
grouping hook receives an empty array. The current code handles this because an empty
`filteredEntries` produces an empty `groups` Map, which produces an empty `groupedEntries`
array. However, the `flatEntries` derivation `groupedEntries.flatMap(g => g.entries)`
also returns `[]`, which means `selectedIndex` may point to a non-existent entry.

**Mitigation**:
- The `HistoryGroupView` component handles the empty state explicitly (loading, no results,
  no history). It does not attempt to render entry cards when `filteredEntries.length === 0`.
- The keyboard navigation in the page checks `flatEntries[newIndex]` before accessing
  it (line 577-578), so an out-of-bounds index is safe.
- Test: Add a `useHistoryGrouping` test with empty input and verify `flatEntries` is `[]`.

**Files Likely Affected**:
- `web-app/src/lib/hooks/useHistoryGrouping.ts`
- `web-app/src/components/history/HistoryGroupView.tsx`

### Bug Risk: Stale Closure in Keyboard Handler [SEVERITY: Medium]

**Description**: The keyboard navigation effect (lines 535-616) depends on
`[showMessages, searchQuery, selectedIndex, selectedEntry, flatEntries, selectEntry, loadMessages, cycleGroupingStrategy]`.
After extraction, if the `cycleGroupingStrategy` function comes from `useHistoryFilters`
and the dependency is not correctly wired, the keyboard handler may capture a stale
version of the function.

**Mitigation**:
- Ensure `cycleGroupingStrategy` from the hook is wrapped in `useCallback` with proper
  dependencies (it depends on `groupingStrategy` state).
- The keyboard effect's dependency array must include the hook-returned function reference.
- Test: Manual testing of pressing `G` after changing grouping via dropdown to verify
  the keyboard shortcut cycles from the current strategy, not from a stale one.

**Files Likely Affected**:
- `web-app/src/app/history/page.tsx` (keyboard effect)
- `web-app/src/lib/hooks/useHistoryFilters.ts` (`cycleGroupingStrategy`)

### Bug Risk: CSS Class Collision After Split [SEVERITY: Low]

**Description**: When CSS classes move from `history.module.css` to child component
modules (e.g., `HistoryFilterBar.module.css`), CSS Modules generates different hashed
class names. If a style in one module references a class that moved to another module
(e.g., the `.entryCard.selected` descendant selectors on lines 344-352), the styling
breaks.

**Mitigation**:
- The `.entryCard.selected` descendant selectors (lines 344-352) must move to the same
  module as `.entryCard` -- that is, to `HistoryEntryCard`'s component or a shared module.
- Audit all CSS selectors that reference multiple classes to ensure they co-locate.
- Approach: Keep the entry-card-specific styles in a new `HistoryEntryCard.module.css`
  and the detail-panel styles in `HistoryDetailPanel.module.css`.

**Files Likely Affected**:
- `web-app/src/app/history/history.module.css`
- New component-level CSS modules

### Bug Risk: fullTextSearch Object Identity [SEVERITY: Low]

**Description**: The `HistoryFilterBar` component receives the `fullTextSearch` object
from `useHistoryFullTextSearch`. If the object reference changes every render (because
the hook returns a new object literal), the `HistoryFilterBar` will re-render unnecessarily.
This is not a bug but a performance concern.

**Mitigation**:
- The `useHistoryFullTextSearch` hook already returns a stable object (spreading `state`
  which is a single `useState` value). The `search`, `loadMore`, `clearSearch`, `setQuery`
  functions are wrapped in `useCallback`.
- No action needed, but document that consumers should not destructure and re-wrap the
  full-text search return value into a new object on every render.

**Files Likely Affected**:
- `web-app/src/app/history/page.tsx`
- `web-app/src/components/history/HistoryFilterBar.tsx`

---

## Testing Strategy

### Unit Tests (Hooks)

| Test File | Hook Under Test | Key Scenarios |
|-----------|----------------|---------------|
| `useHistoryFilters.test.ts` | `useHistoryFilters` | Filter by model, date, search; sort by all fields; clear filters; cycle grouping; localStorage hydration; `hasActiveFilters` |
| `useHistoryGrouping.test.ts` | `useHistoryGrouping` | Group by none/date/project/model; empty input; date-order sorting; flatEntries derivation |

### Unit Tests (Utilities)

| Test File | Module Under Test | Key Scenarios |
|-----------|-------------------|---------------|
| `timestamp.test.ts` | `lib/utils/timestamp` | `formatTimeAgo` with various deltas; `formatDate` with valid/undefined; `getDateGroup` boundary cases (exactly midnight, yesterday boundary); `isWithinDateFilter` for all DateFilter values; `truncateMiddle` at/below/above max length |

### Integration Tests (Existing)

No new integration tests needed. The existing `npm test` suite in `web-app/` covers the
components and hooks that are NOT changing (e.g., `useHistoryFullTextSearch`,
`HistorySearchInput`, `HistorySearchResults`). The refactoring should not break these.

### Manual Verification Checklist

- [ ] History page loads and displays entries grouped by date (default)
- [ ] Switching grouping strategy via dropdown and `G` key both work
- [ ] All sort fields work in both directions
- [ ] Model and date filter dropdowns filter correctly
- [ ] Metadata search filters entries by name/project/model
- [ ] Full-text search mode shows HistorySearchResults
- [ ] Clicking an entry shows detail panel with preview messages
- [ ] "View Messages" opens modal with full conversation
- [ ] Message search in modal filters messages
- [ ] "Resume Session" creates a new session and navigates to home
- [ ] "Export" downloads JSON file
- [ ] "Copy ID" copies to clipboard
- [ ] Keyboard navigation (j/k, arrows, Enter, Escape, /) works
- [ ] Filter state persists across page reload (localStorage)
- [ ] No TypeScript errors in build
- [ ] No console errors in browser

---

## Dependency Graph

```
Story 1: useHistoryFilters + timestamp utils
    |
    +---> Story 2: useHistoryGrouping (depends on types from Story 1)
    |         |
    |         +---> Story 4: HistoryGroupView, HistoryEntryCard (depends on HistoryGroup type)
    |
    +---> Story 3: HistoryFilterBar (depends on filter state types from Story 1)
    |
    +---> Story 4: HistoryDetailPanel, HistoryMessagesModal (depends on timestamp utils from Story 1)
              |
              +---> Story 5: Final cleanup (depends on all stories)
```

**Execution order:**
1. **Story 1** must be completed first (types, utils, and hook that others depend on).
2. **Stories 2, 3, and 4** can be partially parallelized:
   - Story 2 depends on the types exported by Story 1 (`HistoryGroupingStrategy`).
   - Story 3 depends on the filter state types and return shape from Story 1.
   - Story 4 depends on the `HistoryGroup` type from Story 2 and timestamp utils from Story 1.
   - In practice: Story 2 is fast and should be done right after Story 1. Stories 3 and 4 can then proceed in parallel.
3. **Story 5** is the final integration pass and must be last.

**Recommended sequence:** Story 1 -> Story 2 -> Story 3 + Story 4 (parallel) -> Story 5
