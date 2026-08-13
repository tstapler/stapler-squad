# Validation Plan: Better File Tree UX

**Date**: 2026-05-16
**Requirements source**: `project_plans/better-file-tree/requirements.md`
**Plan source**: `project_plans/better-file-tree/implementation/plan.md`

---

## Coverage Summary

| Metric | Value |
|--------|-------|
| Requirements covered | 7 / 7 (R1–R7) |
| Acceptance criteria covered | 8 / 8 (AC-1–AC-8) |
| Total test cases | 54 |
| Jest unit tests | 20 |
| Jest component tests | 23 |
| Playwright e2e tests | 11 |

---

## Test Suite

### R1 — Resizable tree panel

| Test ID | Type | File | Test name | Covers |
|---------|------|------|-----------|--------|
| T-001 | Jest unit | `web-app/src/lib/hooks/useResizablePanel.test.ts` | `useResizablePanel — initializes width from localStorage when key is present` | AC-1, R1 |
| T-002 | Jest unit | `web-app/src/lib/hooks/useResizablePanel.test.ts` | `useResizablePanel — falls back to defaultWidth when localStorage is empty` | R1 |
| T-003 | Jest unit | `web-app/src/lib/hooks/useResizablePanel.test.ts` | `useResizablePanel — persists updated width to localStorage on change` | AC-1, R1 |
| T-004 | Jest unit | `web-app/src/lib/hooks/useResizablePanel.test.ts` | `useResizablePanel — clamps width to minWidth when drag would go below minimum` | R1 |
| T-005 | Jest unit | `web-app/src/lib/hooks/useResizablePanel.test.ts` | `useResizablePanel — clamps width to maxWidthFraction of container when drag would exceed maximum` | R1 |
| T-006 | Jest unit | `web-app/src/lib/hooks/useResizablePanel.test.ts` | `useResizablePanel — collapse() sets collapsed true and persists to localStorage` | AC-2, R1 |
| T-007 | Jest unit | `web-app/src/lib/hooks/useResizablePanel.test.ts` | `useResizablePanel — expand() restores last width from lastWidthRef` | AC-2, R1 |
| T-008 | Jest unit | `web-app/src/lib/hooks/useResizablePanel.test.ts` | `useResizablePanel — expand() falls back to defaultWidth when lastWidthRef is empty` | AC-2, R1 |
| T-009 | Jest unit | `web-app/src/lib/hooks/useResizablePanel.test.ts` | `useResizablePanel — initializes collapsed from localStorage when key is present` | AC-2, R1 |
| T-010 | Jest component | `web-app/src/components/sessions/FilesTab.test.tsx` | `FilesTab — collapse button hides tree pane and sets aria-hidden` | AC-2, R1 |
| T-011 | Jest component | `web-app/src/components/sessions/FilesTab.test.tsx` | `FilesTab — expand button restores tree pane to previous width` | AC-2, R1 |
| T-012 | Playwright e2e | `tests/e2e/better-file-tree.spec.ts` | `better-file-tree — AC-1: drag resize handle to 400px, reload, tree remains 400px wide` | AC-1, R1 |
| T-013 | Playwright e2e | `tests/e2e/better-file-tree.spec.ts` | `better-file-tree — AC-2: collapse button hides tree; expand button restores prior width` | AC-2, R1 |

---

### R2 — Mobile-friendly layout

| Test ID | Type | File | Test name | Covers |
|---------|------|------|-----------|--------|
| T-014 | Jest component | `web-app/src/components/sessions/FilesTab.test.tsx` | `FilesTab mobile — mobilePane defaults to tree on initial render` | R2 |
| T-015 | Jest component | `web-app/src/components/sessions/FilesTab.test.tsx` | `FilesTab mobile — selecting a file sets mobilePane to content` | AC-3, R2 |
| T-016 | Jest component | `web-app/src/components/sessions/FilesTab.test.tsx` | `FilesTab mobile — back button sets mobilePane back to tree` | AC-3, R2 |
| T-017 | Jest component | `web-app/src/components/sessions/FilesTab.test.tsx` | `FilesTab mobile — back button is not rendered at desktop viewport widths` | R2 |
| T-018 | Jest component | `web-app/src/components/sessions/FilesTab.test.tsx` | `FilesTab mobile — back button renders with text "← Files"` | AC-3, R2 |
| T-019 | Playwright e2e | `tests/e2e/better-file-tree.spec.ts` | `better-file-tree — AC-3: on 375px viewport selecting a file shows content pane full-width with back button` | AC-3, R2 |
| T-020 | Playwright e2e | `tests/e2e/better-file-tree.spec.ts` | `better-file-tree — AC-3: back button returns to tree on mobile viewport` | AC-3, R2 |

---

### R3 — File name truncation fix

| Test ID | Type | File | Test name | Covers |
|---------|------|------|-----------|--------|
| T-021 | Jest unit | `web-app/src/lib/utils/truncateMiddle.test.ts` | `truncateMiddle — returns name unchanged when name is shorter than maxLen` | R3 |
| T-022 | Jest unit | `web-app/src/lib/utils/truncateMiddle.test.ts` | `truncateMiddle — returns name unchanged when name equals maxLen exactly` | R3 |
| T-023 | Jest unit | `web-app/src/lib/utils/truncateMiddle.test.ts` | `truncateMiddle — truncates long name preserving extension: very-long-component-name.tsx becomes very-long-comp…name.tsx` | AC-4, R3 |
| T-024 | Jest unit | `web-app/src/lib/utils/truncateMiddle.test.ts` | `truncateMiddle — truncates name with no extension placing ellipsis in the middle` | R3 |
| T-025 | Jest unit | `web-app/src/lib/utils/truncateMiddle.test.ts` | `truncateMiddle — handles name shorter than maxChars with only an extension (no stem)` | R3 |
| T-026 | Jest unit | `web-app/src/lib/utils/truncateMiddle.test.ts` | `truncateMiddle — ensures head and tail are each at least 1 character when maxLen is at minimum (5)` | R3 |
| T-027 | Jest unit | `web-app/src/lib/utils/truncateMiddle.test.ts` | `truncateMiddle — handles empty string input without throwing` | R3 |
| T-028 | Jest unit | `web-app/src/lib/utils/truncateMiddle.test.ts` | `truncateMiddle — preserves multi-dot extension (e.g. .test.ts) as single suffix` | R3 |
| T-029 | Jest component | `web-app/src/components/sessions/FileTree.test.tsx` | `FileTree — renders truncated file name with title tooltip containing full path` | AC-4, R3 |
| T-030 | Jest component | `web-app/src/components/sessions/FileTree.test.tsx` | `FileTree — shows full un-truncated name when search term is active` | R3 |
| T-031 | Playwright e2e | `tests/e2e/better-file-tree.spec.ts` | `better-file-tree — AC-4: long file name is displayed truncated in the middle and tooltip shows full path` | AC-4, R3 |

---

### R4 — Preserve tree scroll position

| Test ID | Type | File | Test name | Covers |
|---------|------|------|-----------|--------|
| T-032 | Jest component | `web-app/src/components/sessions/FileTree.test.tsx` | `FileTree — scroll position is not reset when selectedPath prop changes` | AC-5, R4 |
| T-033 | Jest component | `web-app/src/components/sessions/FileTree.test.tsx` | `FileTree — scrollTo is not called when a file in the middle of the tree is selected` | AC-5, R4 |
| T-034 | Playwright e2e | `tests/e2e/better-file-tree.spec.ts` | `better-file-tree — AC-5: scroll to bottom, open a file, tree scroll position stays at bottom` | AC-5, R4 |

---

### R5 — Tree highlights and auto-reveals current file

| Test ID | Type | File | Test name | Covers |
|---------|------|------|-----------|--------|
| T-035 | Jest component | `web-app/src/components/sessions/FileTree.test.tsx` | `FileTree revealPath — expands all ancestor directories of the target path` | AC-6, R5 |
| T-036 | Jest component | `web-app/src/components/sessions/FileTree.test.tsx` | `FileTree revealPath — calls scrollTo after all ancestors are opened` | AC-6, R5 |
| T-037 | Jest component | `web-app/src/components/sessions/FileTree.test.tsx` | `FileTree revealPath — aborts in-progress reveal when a second revealPath call is made` | R5 |
| T-038 | Jest component | `web-app/src/components/sessions/FilesTab.test.tsx` | `FilesTab — setting initialSelectedPath calls revealPath on the FileTree ref` | AC-6, R5 |
| T-039 | Playwright e2e | `tests/e2e/better-file-tree.spec.ts` | `better-file-tree — AC-6: navigating to a file via external path auto-expands ancestors and scrolls row into view` | AC-6, R5 |

---

### R6 — Recently opened files panel

| Test ID | Type | File | Test name | Covers |
|---------|------|------|-----------|--------|
| T-040 | Jest unit | `web-app/src/lib/utils/recentFiles.test.ts` | `recent files deduplication — prepend-and-deduplicate keeps most recent at index 0` | R6 |
| T-041 | Jest unit | `web-app/src/lib/utils/recentFiles.test.ts` | `recent files deduplication — re-opening an existing path moves it to front and does not create duplicate` | R6 |
| T-042 | Jest unit | `web-app/src/lib/utils/recentFiles.test.ts` | `recent files deduplication — list is capped at 8 entries after adding a ninth` | R6 |
| T-043 | Jest component | `web-app/src/components/sessions/RecentFilesSection.test.tsx` | `RecentFilesSection — renders nothing when paths array is empty` | R6 |
| T-044 | Jest component | `web-app/src/components/sessions/RecentFilesSection.test.tsx` | `RecentFilesSection — renders one entry per path in the provided array` | AC-7, R6 |
| T-045 | Jest component | `web-app/src/components/sessions/RecentFilesSection.test.tsx` | `RecentFilesSection — each entry shows basename and parent directory name` | R6 |
| T-046 | Jest component | `web-app/src/components/sessions/RecentFilesSection.test.tsx` | `RecentFilesSection — each entry button has title attribute equal to the full path` | R6 |
| T-047 | Jest component | `web-app/src/components/sessions/RecentFilesSection.test.tsx` | `RecentFilesSection — clicking an entry calls onSelect with the full path` | R6 |
| T-048 | Jest component | `web-app/src/components/sessions/FilesTab.test.tsx` | `FilesTab — opens 3 files and Recent section shows all 3 paths most-recent-first` | AC-7, R6 |
| T-049 | Playwright e2e | `tests/e2e/better-file-tree.spec.ts` | `better-file-tree — AC-7: open 3 files; Recent section shows all 3 in most-recent-first order` | AC-7, R6 |

---

### R7 — Keyboard navigation enhancements

| Test ID | Type | File | Test name | Covers |
|---------|------|------|-----------|--------|
| T-050 | Jest component | `web-app/src/components/sessions/QuickOpenPalette.test.tsx` | `QuickOpenPalette — renders palette when isOpen is true` | AC-8, R7 |
| T-051 | Jest component | `web-app/src/components/sessions/QuickOpenPalette.test.tsx` | `QuickOpenPalette — Escape key calls onClose` | R7 |
| T-052 | Jest component | `web-app/src/components/sessions/QuickOpenPalette.test.tsx` | `QuickOpenPalette — typing a query filters results to matching file names` | AC-8, R7 |
| T-053 | Jest component | `web-app/src/components/sessions/QuickOpenPalette.test.tsx` | `QuickOpenPalette — ArrowDown moves active index to next result` | R7 |
| T-054 | Jest component | `web-app/src/components/sessions/QuickOpenPalette.test.tsx` | `QuickOpenPalette — ArrowUp wraps active index from first to last result` | R7 |
| T-055 | Jest component | `web-app/src/components/sessions/QuickOpenPalette.test.tsx` | `QuickOpenPalette — ArrowDown wraps active index from last to first result` | R7 |
| T-056 | Jest component | `web-app/src/components/sessions/QuickOpenPalette.test.tsx` | `QuickOpenPalette — Enter key calls onSelect with path of active result and calls onClose` | R7 |
| T-057 | Jest component | `web-app/src/components/sessions/QuickOpenPalette.test.tsx` | `QuickOpenPalette — shows recentPaths as initial results when query is empty` | R7 |
| T-058 | Jest component | `web-app/src/components/sessions/QuickOpenPalette.test.tsx` | `QuickOpenPalette — restores focus to previously focused element on unmount` | R7 |
| T-059 | Jest component | `web-app/src/components/sessions/FilesTab.test.tsx` | `FilesTab — Ctrl+P opens the quick-open palette` | AC-8, R7 |
| T-060 | Jest component | `web-app/src/components/sessions/FilesTab.test.tsx` | `FilesTab — Escape in the search input clears searchTerm and blurs the input` | R7 |
| T-061 | Playwright e2e | `tests/e2e/better-file-tree.spec.ts` | `better-file-tree — AC-8: Ctrl+P opens quick-open palette; typing "store" filters results to files matching "store"` | AC-8, R7 |

---

## Requirement-to-Test Traceability Matrix

| Req | AC | Test IDs | Coverage |
|-----|----|----------|----------|
| R1 | AC-1 | T-001, T-002, T-003, T-004, T-005, T-012 | Unit + e2e |
| R1 | AC-2 | T-006, T-007, T-008, T-009, T-010, T-011, T-013 | Unit + component + e2e |
| R2 | AC-3 | T-014, T-015, T-016, T-017, T-018, T-019, T-020 | Component + e2e |
| R3 | AC-4 | T-021, T-022, T-023, T-024, T-025, T-026, T-027, T-028, T-029, T-030, T-031 | Unit + component + e2e |
| R4 | AC-5 | T-032, T-033, T-034 | Component + e2e |
| R5 | AC-6 | T-035, T-036, T-037, T-038, T-039 | Component + e2e |
| R6 | AC-7 | T-040, T-041, T-042, T-043, T-044, T-045, T-046, T-047, T-048, T-049 | Unit + component + e2e |
| R7 | AC-8 | T-050, T-051, T-052, T-053, T-054, T-055, T-056, T-057, T-058, T-059, T-060, T-061 | Component + e2e |

---

## Test File Inventory

| File | Tests | Framework |
|------|-------|-----------|
| `web-app/src/lib/hooks/useResizablePanel.test.ts` | T-001–T-009 | Jest (unit, renderHook) |
| `web-app/src/lib/utils/truncateMiddle.test.ts` | T-021–T-028 | Jest (unit, pure function) |
| `web-app/src/lib/utils/recentFiles.test.ts` | T-040–T-042 | Jest (unit, pure logic) |
| `web-app/src/components/sessions/FilesTab.test.tsx` | T-010, T-011, T-014–T-018, T-038, T-048, T-059–T-060 | Jest + React Testing Library |
| `web-app/src/components/sessions/FileTree.test.tsx` | T-029–T-030, T-032–T-037 | Jest + React Testing Library |
| `web-app/src/components/sessions/RecentFilesSection.test.tsx` | T-043–T-047 | Jest + React Testing Library |
| `web-app/src/components/sessions/QuickOpenPalette.test.tsx` | T-050–T-058 | Jest + React Testing Library |
| `tests/e2e/better-file-tree.spec.ts` | T-012, T-013, T-019–T-020, T-031, T-034, T-039, T-049, T-061 | Playwright |

---

## Detailed Test Specifications

### `useResizablePanel` hook tests (T-001–T-009)

All tests use `renderHook` from `@testing-library/react`. `localStorage` is cleared in `beforeEach`.

**T-001** — seed `localStorage.setItem('filestab.treeWidth', '350')` before render; assert `result.current.width === 350`.

**T-002** — render with no localStorage entry; assert `result.current.width === 260` (defaultWidth).

**T-003** — render; call `act(() => result.current.handleProps.onPointerMove(...))`; assert `localStorage.getItem('filestab.treeWidth')` equals the new stringified width.

**T-004** — simulate a drag that would produce `width = 50`; assert `result.current.width === 160` (minWidth clamp).

**T-005** — set container clientWidth to 1000 via mocked `getBoundingClientRect`; simulate drag to 600; assert `result.current.width === 500` (50% clamp).

**T-006** — call `act(() => result.current.collapse())`; assert `result.current.collapsed === true`; assert `localStorage.getItem('filestab.treeCollapsed') === 'true'`.

**T-007** — set width to 320; call `collapse()`; call `expand()`; assert `result.current.width === 320`.

**T-008** — call `expand()` on a freshly initialized hook (no prior width); assert `result.current.width === 260` (defaultWidth).

**T-009** — seed `localStorage.setItem('filestab.treeCollapsed', 'true')` before render; assert `result.current.collapsed === true`.

---

### `truncateMiddle` utility tests (T-021–T-028)

**T-021** — `truncateMiddle('short.ts', 20)` returns `'short.ts'` unchanged.

**T-022** — `truncateMiddle('exactly20chars.ts', 18)` where name.length === maxLen returns name unchanged.

**T-023** — `truncateMiddle('very-long-component-name.tsx', 24)` returns a string that (a) starts with `'very'`, (b) ends with `'.tsx'`, (c) contains `'…'`, (d) has length ≤ 24.

**T-024** — `truncateMiddle('averylongfilenamewithoutext', 15)` returns a string containing `'…'` with no trailing extension segment.

**T-025** — `truncateMiddle('.bashrc', 10)` — name has no stem, only extension-like dotfile; returns string of length ≤ 10 containing `'…'` or the full string if short enough.

**T-026** — `truncateMiddle('abcde.ts', 5)` — at minimum maxLen: result length is ≤ 5 and contains `'…'`; head and tail portions are each at least 1 character.

**T-027** — `truncateMiddle('', 20)` returns `''` without throwing.

**T-028** — `truncateMiddle('Component.test.ts', 14)` — suffix is `.ts` (last dot only, not `.test.ts`); result preserves `.ts` suffix.

---

### `recentFiles` deduplication logic tests (T-040–T-042)

These test the inline state-update function `prev => [path, ...prev.filter(p => p !== path)].slice(0, 8)` extracted to a pure helper or tested via the `FilesTab` state update.

**T-040** — apply the reducer with `prev = ['/b']`, `path = '/a'`; result is `['/a', '/b']`.

**T-041** — apply with `prev = ['/a', '/b', '/c']`, `path = '/b'`; result is `['/b', '/a', '/c']` (no duplicate).

**T-042** — apply with `prev` containing 8 entries, `path = '/new'`; result has length 8 and `result[0] === '/new'`.

---

### `RecentFilesSection` component tests (T-043–T-047)

**T-043** — render with `paths={[]}`; assert `container.firstChild === null` (renders nothing).

**T-044** — render with 3 paths; assert 3 `<button>` elements are in the DOM.

**T-045** — render with `paths={['/root/src/Foo.tsx']}`; assert visible text includes `'Foo.tsx'` and `'src'` (parent dir).

**T-046** — render with `paths={['/a/b/c.ts']}`; assert the button has `title="/a/b/c.ts"`.

**T-047** — render; click the button for `'/a/b/c.ts'`; assert `onSelect` mock was called with `'/a/b/c.ts'`.

---

### `QuickOpenPalette` component tests (T-050–T-058)

All tests use `@testing-library/react` `render` with a mocked `SearchFiles` RPC via `vi.mock` / `jest.mock`.

**T-050** — render `<QuickOpenPalette ... />`; assert an `<input>` element exists in the document; assert the backdrop `<div>` has `position: fixed` or `data-testid="quick-open-backdrop"`.

**T-051** — render; fire `keyDown` on input with `key: 'Escape'`; assert `onClose` mock called once.

**T-052** — mock `SearchFiles` to return files including `['store.ts', 'userStore.ts', 'unrelated.ts']`; type `'store'` into the input; wait for results; assert rendered list contains `'store.ts'` and `'userStore.ts'` but not `'unrelated.ts'`.

**T-053** — render with 3 results; fire `keyDown ArrowDown`; assert the second result has the active-item class / `aria-selected="true"`.

**T-054** — render with 3 results; fire `keyDown ArrowUp` while activeIndex is 0; assert active index wraps to 2 (last).

**T-055** — render with 3 results; navigate to last; fire `keyDown ArrowDown`; assert active index wraps to 0 (first).

**T-056** — render; navigate to second result; fire `keyDown Enter`; assert `onSelect` called with the second result's path and `onClose` called once.

**T-057** — render with `recentPaths={['/a/b.ts', '/c/d.ts']}` and empty query; assert the two recent paths appear in the list without any RPC call.

**T-058** — spy on `document.activeElement` before render; unmount the component; assert `focus()` was called on the previously-focused element.

---

### `FilesTab` mobile layout tests (T-014–T-018)

All tests mock `useResizablePanel` to return stable defaults. Tests that check mobile-only elements assert visibility via computed CSS classes rather than viewport resize (viewport-dependent CSS is exercised in e2e).

**T-014** — render `<FilesTab ...>`; assert the internal `mobilePane` state is `'tree'` (tested indirectly: the back button has `data-testid="back-to-tree"` and is present in the DOM but invisible by class).

**T-015** — simulate `handleFileSelect('/path/to/file.ts')` (click a file node); assert `mobilePane` state transitions to `'content'` (detected via the `mobilePaneVisible` class on content pane or absence of `mobilePaneHidden`).

**T-016** — with `mobilePane === 'content'`, click the back button (`data-testid="back-to-tree"`); assert `mobilePane` transitions back to `'tree'`.

**T-017** — at viewport width 1024 px, assert the back button element has the CSS class that corresponds to `display: none` at desktop width (or has no `data-testid="back-to-tree"` rendered at all per the conditional render approach).

**T-018** — find the back button; assert its text content is `'← Files'`.

---

### Playwright e2e tests — `tests/e2e/better-file-tree.spec.ts`

All e2e tests use `page.setViewportSize` for mobile tests. The `better-file-tree` `test.describe` block sets `baseURL` and navigates to the FilesTab of a test session seeded with a directory containing files of known names including a file named `very-long-component-name.tsx` and multiple files with "store" in the name.

**T-012** — drag the resize handle from its default position rightward to approximately 400 px; assert the tree pane width is within ±5 px of 400; reload the page; assert the tree pane width is still within ±5 px of 400.

**T-013** — click the collapse button (aria-label "Collapse file tree"); assert the tree pane has `width: 0` or is not visible; click the expand button (aria-label "Expand file tree"); assert the tree pane width matches the width before collapse (within ±5 px).

**T-019** — set viewport to 375 × 812; click a file in the tree; assert content pane is visible and occupies at least 90% of viewport width; assert back button with text "← Files" is visible.

**T-020** — (continued from T-019) click "← Files" back button; assert tree pane is visible and content pane is not visible at mobile viewport.

**T-031** — locate the file node for `very-long-component-name.tsx`; assert its visible text label contains `'…'`; assert the text label does NOT contain the full untruncated name; assert the node's `title` attribute equals the full path of the file.

**T-034** — scroll the tree to the bottom (JS `scrollTop = tree.scrollHeight`); open a file whose node is in the middle of the list; assert the tree scroll container's `scrollTop` has not changed (within ±10 px tolerance for sub-pixel rounding).

**T-039** — navigate to the session with an `initialSelectedPath` query param pointing to a deeply nested file inside collapsed directories; assert the tree auto-expands ancestor directories and the corresponding file row is visible in the tree pane without manual scrolling.

**T-049** — open three files in sequence (click file A, click file B, click file C); assert the "Recent" section heading is visible; assert the three file basenames appear in the section in reverse order (C first, A last).

**T-061** — press `Control+P`; assert a search input inside the quick-open palette is focused; type `'store'`; wait for results to appear; assert at least one result entry contains `'store'` in its visible text; assert no result entry contains a known non-matching file name from the seed data.

---

## Notes for Implementation

1. **`recentFiles.test.ts`** — if the deduplication logic is not extracted to a standalone utility, move these three tests into `FilesTab.test.tsx` and test via state inspection after simulated `handleFileSelect` calls.

2. **`FileTree.test.tsx` scroll tests (T-032, T-033)** — react-arborist's `<Tree>` uses `react-window` under the hood. Mock `react-window`'s `FixedSizeList` with a spy on `scrollTo` and assert it is NOT called when `selectedPath` prop changes.

3. **`FileTree.test.tsx` revealPath tests (T-035–T-037)** — use `useImperativeHandle` via a `ref` in the test: `const ref = React.createRef<FileTreeHandle>(); render(<FileTree ref={ref} ... />); await act(() => ref.current!.revealPath('/a/b/c.ts'));`. Assert `treeRef.current?.open` was called for each ancestor.

4. **Playwright seed data** — the `better-file-tree.spec.ts` fixture must create a temporary git repo containing `very-long-component-name.tsx`, multiple files matching `*store*`, and a directory depth of at least 3 levels. Use the existing `tests/e2e/fixtures/` helpers or add a `beforeAll` that creates the repo via the `CreateSession` API.

5. **CSS class assertions in component tests** — use `data-testid` attributes rather than CSS class name assertions to avoid brittleness from vanilla-extract hashed class names.
