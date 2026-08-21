# Validation Plan: Logs UX — Datadog/Splunk-Like Experience

**Date**: 2026-05-14
**Requirements**: `project_plans/logs-datadog-ux/requirements.md`
**Plan**: `project_plans/logs-datadog-ux/implementation/plan.md`

---

## Requirement → Test Traceability Matrix

| Requirement | Description | Test IDs |
|---|---|---|
| FR-1 | Live tail auto-scroll, pause on scroll-up, Jump to Latest | T-UNIT-001, T-UNIT-002, T-UNIT-003, T-UNIT-004, T-COMP-001, T-COMP-002, T-E2E-001, T-E2E-002 |
| FR-2 | Horizontal scroll, no line wrapping, sticky gutter | T-COMP-003, T-COMP-004, T-E2E-003 |
| FR-3 | Search bar, highlight, match count, ESC clears | T-UNIT-005, T-UNIT-006, T-UNIT-007, T-COMP-005, T-COMP-006, T-E2E-004 |
| FR-4 | Log level badges with WCAG AA colors | T-UNIT-008, T-UNIT-009, T-UNIT-010, T-COMP-007, T-COMP-008, T-SEC-003 |
| FR-5 | Expandable row, JSON pretty-print, copy button | T-UNIT-011, T-UNIT-012, T-COMP-009, T-COMP-010, T-COMP-011, T-E2E-005 |
| FR-6 | Mobile: touch targets ≥ 44px, collapsible search | T-COMP-012, T-COMP-013, T-E2E-006, T-E2E-007 |
| FR-7 | Level filter chips, multi-select, ALL behavior | T-UNIT-013, T-UNIT-014, T-COMP-014, T-COMP-015, T-E2E-008 |
| NFR-1 | Virtual scroll — only visible rows in DOM | T-COMP-016, T-PERF-001, T-PERF-002 |
| NFR-2 | Keyboard navigation (/, g, G, n, N, Enter) | T-COMP-017, T-COMP-018, T-E2E-009 |
| NFR-3 | No new endpoints required (preferred) | T-E2E-010 |
| SEC | ANSI OSC sequences — no clickable `<a>` XSS | T-UNIT-015, T-UNIT-016, T-UNIT-017, T-SEC-001, T-SEC-002 |

---

## Unit Tests

All unit tests live in `web-app/src/lib/logs/__tests__/logParser.test.ts` and
`web-app/src/lib/hooks/__tests__/useLogViewer.test.ts`.

Run with: `cd web-app && npx jest --no-coverage --testPathPatterns="logParser|useLogViewer"`

---

### logParser.ts — Level Detection

**File**: `web-app/src/lib/logs/__tests__/logParser.test.ts`

#### T-UNIT-008 · FR-4
```
describe("detectLevel")
  it("T-UNIT-008: detectLevel_should_returnERROR_When_lineContainsERROR")
```
Assert: `detectLevel("2026-01-01 ERROR connection reset")` returns `"ERROR"`.

#### T-UNIT-009 · FR-4
```
  it("T-UNIT-009: detectLevel_should_returnERROR_When_lineContainsERR")
```
Assert: `detectLevel("ERR: connection refused")` returns `"ERROR"`.

#### T-UNIT-010 · FR-4
```
  it("T-UNIT-010: detectLevel_should_returnUNKNOWN_When_noLevelPresent")
```
Parameterized table of inputs:
- `"2026-01-01 WARN service degraded"` → `"WARN"`
- `"[WARNING] threshold exceeded"` → `"WARN"`
- `"INFO request completed in 120ms"` → `"INFO"`
- `"DEBUG polling interval: 2s"` → `"DEBUG"`
- `"TRACE span created"` → `"TRACE"`
- `"no level information"` → `"UNKNOWN"`
- `"debug mode active"` → `"DEBUG"` (case-insensitive)
- `"error prone design"` → `"ERROR"` (word-boundary match)

---

### logParser.ts — Search Highlight Segmentation

**File**: `web-app/src/lib/logs/__tests__/logParser.test.ts`

#### T-UNIT-005 · FR-3
```
describe("segmentText")
  it("T-UNIT-005: segmentText_should_returnHighlightedSegment_When_queryFound")
```
Assert: `segmentText("hello world", "world")` returns
`[{ text: "hello ", highlight: false }, { text: "world", highlight: true }]`.

#### T-UNIT-006 · FR-3
```
  it("T-UNIT-006: segmentText_should_returnSingleSegment_When_queryEmpty")
```
Assert: `segmentText("hello world", "")` returns `[{ text: "hello world", highlight: false }]`.

#### T-UNIT-007 · FR-3
```
  it("T-UNIT-007: segmentText_should_matchCaseInsensitively_When_queryDifferentCase")
```
Assert: `segmentText("Hello World", "hello")` contains one highlighted segment with text `"Hello"`.
Also: query not found → single non-highlighted segment containing full input.

---

### logParser.ts — JSON Detection

**File**: `web-app/src/lib/logs/__tests__/logParser.test.ts`

#### T-UNIT-011 · FR-5
```
describe("tryParseJson")
  it("T-UNIT-011: tryParseJson_should_returnParsedObject_When_validJson")
```
Assert: `tryParseJson('{"level":"error","msg":"oops"}')` returns `{ level: "error", msg: "oops" }`.

#### T-UNIT-012 · FR-5
```
  it("T-UNIT-012: tryParseJson_should_returnNull_When_plainText")
```
Assert: `tryParseJson("plain text log line")` returns `null`.
Also: truncated JSON, leading whitespace (should parse after trim), empty string → `null`.

---

### logParser.ts — ANSI / XSS Security

**File**: `web-app/src/lib/logs/__tests__/logParser.test.ts`

#### T-UNIT-015 · SEC
```
describe("renderAnsi — XSS guards")
  it("T-UNIT-015: renderAnsi_should_stripScriptTag_When_inputContainsScript")
```
Assert: `renderAnsi('<script>alert(1)</script>')` does not contain the string `"<script"`.

#### T-UNIT-016 · SEC
```
  it("T-UNIT-016: renderAnsi_should_stripOscJavascriptHref_When_OSCHyperlinkPresent")
```
Input: `"\x1b]8;;javascript:alert(1)\x07click me\x1b]8;;\x07"`
Assert: output does not contain `"javascript:"`.
Assert: output does not contain an `<a` tag with `href` pointing to `javascript:`.

#### T-UNIT-017 · SEC
```
  it("T-UNIT-017: renderAnsi_should_notThrow_When_partialAnsiSequence")
```
Input: `"text\x1b[31"` (truncated).
Assert: function does not throw and return value contains `"text"`.

---

### useLogViewer.ts — Live Tail State Machine

**File**: `web-app/src/lib/hooks/__tests__/useLogViewer.test.ts`

Uses `renderHook` from `@testing-library/react`.

#### T-UNIT-001 · FR-1
```
describe("useLogViewer — live tail state machine")
  it("T-UNIT-001: useLogViewer_should_startFollowing_When_initialized")
```
Assert: initial state has `isFollowing === true` and `queuedNewLineCount === 0`.

#### T-UNIT-002 · FR-1
```
  it("T-UNIT-002: useLogViewer_should_pauseFollowing_When_scrolledUp")
```
Call `result.current.onAtBottom(false)`.
Assert: `isFollowing === false`.
Assert: subsequent new-log appends increment `queuedNewLineCount`.

#### T-UNIT-003 · FR-1
```
  it("T-UNIT-003: useLogViewer_should_resumeFollowing_When_jumpToLatestCalled")
```
Setup: call `onAtBottom(false)` to pause.
Call `result.current.jumpToLatest()`.
Assert: `isFollowing === true` and `queuedNewLineCount === 0`.

#### T-UNIT-004 · FR-1
```
  it("T-UNIT-004: useLogViewer_should_autoResume_When_userScrollsToBottomNaturally")
```
Setup: call `onAtBottom(false)` to pause.
Call `result.current.onAtBottom(true)`.
Assert: `isFollowing === true` and `queuedNewLineCount === 0`.

---

### useLogViewer.ts — Expansion State

**File**: `web-app/src/lib/hooks/__tests__/useLogViewer.test.ts`

#### T-UNIT-013 (reused below under FR-7 too, unique ID here) · FR-5 accordion

#### T-UNIT-011-B · FR-5
```
describe("useLogViewer — expansion state")
  it("UNIT-EXP-001: toggleRow_should_expandRow_When_rowIndexProvided")
```
Call `toggleRow(3)`.
Assert: `expandedRowIndex === 3`.

```
  it("UNIT-EXP-002: toggleRow_should_collapseRow_When_sameIndexCalledTwice")
```
Call `toggleRow(3)` twice.
Assert: `expandedRowIndex === null`.

```
  it("UNIT-EXP-003: toggleRow_should_collapseFirst_When_differentIndexCalled")
```
Call `toggleRow(3)` then `toggleRow(7)`.
Assert: `expandedRowIndex === 7` (accordion: only one open at a time).

---

### useLogViewer.ts — Client-Side Filter Logic

**File**: `web-app/src/lib/hooks/__tests__/useLogViewer.test.ts`

#### T-UNIT-013 · FR-7
```
describe("useLogViewer — level filter")
  it("T-UNIT-013: filteredLogs_should_returnOnlyMatchingLevels_When_levelFiltersSet")
```
Seed hook with mock entries at levels ERROR, WARN, INFO, DEBUG.
Set `levelFilters = ["ERROR", "WARN"]`.
Assert: `filteredLogs` contains only ERROR and WARN entries, none INFO or DEBUG.

#### T-UNIT-014 · FR-7
```
  it("T-UNIT-014: filteredLogs_should_returnAll_When_levelFiltersIsALL")
```
Set `levelFilters = ["ALL"]`.
Assert: all 4 entries returned.
Also: `levelFilters = []` → all entries returned (empty = no filter).

---

### useLogViewer.ts — Search Performance (unit-level)

**File**: `web-app/src/lib/hooks/__tests__/useLogViewer.test.ts`

#### T-UNIT-PERF-001 · NFR-1, FR-3

```
describe("useLogViewer — search filter performance")
  it("T-UNIT-PERF-001: filteredLogs_should_completeUnder100ms_When_10kLinesAndQuery")
```
Seed hook with 10,000 mock log entries (each ~80 chars).
Set `searchQuery = "needle"` (present in every 100th entry).
Wrap filter call in `performance.now()` start/end.
Assert: elapsed time < 100 ms.

---

## Component Tests

All component tests use React Testing Library (RTL).
Run with: `cd web-app && npx jest --no-coverage --testPathPatterns="LogViewer|LogRow|LevelFilterChips|JumpToLatest|ExpandedLogDetail"`

---

### LogRow — No Line Wrap / Horizontal Scroll

**File**: `web-app/src/components/logs/__tests__/LogRow.test.tsx`

#### T-COMP-003 · FR-2
```
describe("LogRow — layout")
  it("T-COMP-003: LogRow_should_haveNoWrap_When_rendered")
```
Render `<LogRow>` with a 500-character message.
Assert: the message body div has computed `white-space === "nowrap"` (via `getComputedStyle`).

#### T-COMP-004 · FR-2
```
  it("T-COMP-004: LogRow_should_haveOverflowX_When_rendered")
```
Assert: the inner body div has `overflow-x` value of `"auto"` or `"scroll"`.
Assert: the gutter element (`data-testid="log-gutter"`) is not inside the horizontally-scrollable div (sticky separation confirmed).

---

### LogRow — Level Badge Colors

**File**: `web-app/src/components/logs/__tests__/LogRow.test.tsx`

#### T-COMP-007 · FR-4
```
describe("LogRow — level badge")
  it("T-COMP-007: LogRow_should_applyErrorClass_When_levelIsERROR")
```
Render with `entry.level = "ERROR"`.
Assert: badge element has the CSS class generated by the `error` variant of the `levelBadge` recipe.
Assert: row container has the error tint class.

#### T-COMP-008 · FR-4
```
  it("T-COMP-008: LogRow_should_applyCorrectClasses_When_levelIsWarnInfoDebug")
```
Parameterized: WARN → warn class; INFO → info class; DEBUG → debug class; UNKNOWN → no tint class.

---

### LogRow — Aria Attributes

**File**: `web-app/src/components/logs/__tests__/LogRow.test.tsx`

#### T-COMP-017 · NFR-2
```
describe("LogRow — accessibility")
  it("T-COMP-017: LogRow_should_haveAriaExpanded_When_expandedStateChanges")
```
Render `<LogRow isExpanded={false} />`.
Assert: `aria-expanded="false"` on the row element.
Re-render with `isExpanded={true}`.
Assert: `aria-expanded="true"`.

#### T-COMP-018 · NFR-2
```
  it("T-COMP-018: LogRow_should_callOnToggle_When_EnterKeyPressed")
```
Render with `onToggle` mock.
Focus the row element.
Fire `keyDown` with `key="Enter"`.
Assert: `onToggle` called once.
Fire `keyDown` with `key=" "` (Space).
Assert: `onToggle` called twice total.

---

### JumpToLatestButton

**File**: `web-app/src/components/logs/__tests__/JumpToLatestButton.test.tsx`

#### T-COMP-001 · FR-1
```
describe("JumpToLatestButton")
  it("T-COMP-001: JumpToLatestButton_should_beVisible_When_notFollowingAndNewLines")
```
Render `<JumpToLatestButton newLineCount={5} onClick={jest.fn()} />`.
Assert: button is visible in the document.
Assert: button text contains `"5"`.

#### T-COMP-002 · FR-1
```
  it("T-COMP-002: JumpToLatestButton_should_callOnClick_When_clicked")
```
Render with `onClick` mock.
Click the button.
Assert: `onClick` called once.

---

### LevelFilterChips — Multi-Select Behavior

**File**: `web-app/src/components/logs/__tests__/LevelFilterChips.test.tsx`

#### T-COMP-014 · FR-7
```
describe("LevelFilterChips")
  it("T-COMP-014: LevelFilterChips_should_selectMultipleLevels_When_chipsClicked")
```
Render with `active={[]}` and `onChange` mock.
Click ERROR chip.
Assert: `onChange` called with `["ERROR"]`.
Click WARN chip.
Assert: `onChange` called with array containing both `"ERROR"` and `"WARN"`.

#### T-COMP-015 · FR-7
```
  it("T-COMP-015: LevelFilterChips_should_deselectSpecificLevels_When_ALL_clicked")
```
Render with `active={["ERROR", "WARN"]}`.
Click ALL chip.
Assert: `onChange` called with `["ALL"]` (or empty, per implementation contract).

---

### LevelFilterChips — Touch Target Size

**File**: `web-app/src/components/logs/__tests__/LevelFilterChips.test.tsx`

#### T-COMP-012 · FR-6
```
describe("LevelFilterChips — touch targets")
  it("T-COMP-012: LevelFilterChips_should_have44pxMinHeight_When_rendered")
```
Render all chips.
For each chip button, assert `min-height` in computed styles is `"44px"` (or assert that the element's bounding rect height is ≥ 44 in the jsdom layout).

---

### LogViewerToolbar — Search Bar Collapse (Mobile)

**File**: `web-app/src/components/logs/__tests__/LogViewerToolbar.test.tsx`

#### T-COMP-013 · FR-6
```
describe("LogViewerToolbar — mobile search collapse")
  it("T-COMP-013: Toolbar_should_showSearchIcon_When_viewportNarrow")
```
Mock `window.innerWidth = 390` before render (or use `ResizeObserver` mock).
Render `<LogViewerToolbar />`.
Assert: search input is not visible (collapsed state).
Assert: a search icon button with accessible label is present.

```
  it("T-COMP-013b: Toolbar_should_expandSearch_When_searchIconTapped")
```
Click the search icon button.
Assert: search input is visible and receives focus (`document.activeElement`).

---

### ExpandedLogDetail — JSON Pretty-Print

**File**: `web-app/src/components/logs/__tests__/ExpandedLogDetail.test.tsx`

#### T-COMP-009 · FR-5
```
describe("ExpandedLogDetail")
  it("T-COMP-009: ExpandedLogDetail_should_prettyPrintJson_When_entryIsJson")
```
Render with `entry.message = '{"level":"error","msg":"oops","count":3}'`.
Assert: rendered output contains formatted multi-line text (newlines present in a `<pre>` block).
Assert: the key `"level"` appears as text content.

#### T-COMP-010 · FR-5
```
  it("T-COMP-010: ExpandedLogDetail_should_showRawLine_When_entryIsPlainText")
```
Render with `entry.message = "plain log line"`.
Assert: a `<pre>` element contains `"plain log line"`.

#### T-COMP-011 · FR-5
```
  it("T-COMP-011: ExpandedLogDetail_should_copyToClipboard_When_copyButtonClicked")
```
Mock `navigator.clipboard.writeText` as `jest.fn()`.
Render with a plain-text entry.
Click the copy button.
Assert: `navigator.clipboard.writeText` called with `entry.message`.

---

### VirtualLogList — Virtual Scroll DOM Rows

**File**: `web-app/src/components/logs/__tests__/VirtualLogList.test.tsx`

#### T-COMP-016 · NFR-1
```
describe("VirtualLogList — virtualization")
  it("T-COMP-016: VirtualLogList_should_renderFewRows_When_10kEntriesProvided")
```
Render `<VirtualLogList data={[...10,000 entries...]} height={600} />`.
Query all `[data-testid^="log-row"]` elements.
Assert: count is ≤ 100 (virtuoso renders only ~20-40 rows in a 600px window; cap at 100 to allow overscan).
Assert: count is > 0 (at least something renders).

---

### LogViewer — Keyboard Shortcuts

**File**: `web-app/src/components/logs/__tests__/LogViewer.test.tsx`

#### T-COMP-005 · FR-3
```
describe("LogViewer — keyboard shortcuts")
  it("T-COMP-005: LogViewer_should_focusSearch_When_slashKeyPressed")
```
Render `<LogViewer source="app" />`.
Fire `keyDown` with `key="/"` on the log viewer container.
Assert: search input has focus (`document.activeElement === searchInput`).

#### T-COMP-006 · FR-3
```
  it("T-COMP-006: LogViewer_should_clearAndBlurSearch_When_EscapeKeyPressed")
```
Focus the search input and type "error".
Fire `keyDown` with `key="Escape"` on the search input.
Assert: search input value is empty.

---

### LogViewer — Match Count Display

**File**: `web-app/src/components/logs/__tests__/LogViewer.test.tsx`

#### T-COMP-006B · FR-3
```
  it("T-COMP-006B: LogViewer_should_showMatchCount_When_searchQueryMatches")
```
Provide 50 log entries where 12 contain "needle".
Type "needle" into search input.
Assert: toolbar text content includes `"12"` and `"50"` (or `"12 / 50 matches"` pattern).

---

### LogViewer — Live Tail ARIA

**File**: `web-app/src/components/logs/__tests__/LogViewer.test.tsx`

#### T-COMP-017B · NFR-2
```
  it("T-COMP-017B: LogViewer_should_haveRoleLog_When_rendered")
```
Assert: scroll container has `role="log"`.
Assert: scroll container has `aria-live="polite"`.
Assert: scroll container has `aria-label` containing "Log output".

---

### LogRow — Search Highlight Rendering

**File**: `web-app/src/components/logs/__tests__/LogRow.test.tsx`

#### T-COMP-005B · FR-3
```
describe("LogRow — search highlight")
  it("T-COMP-005B: LogRow_should_renderMarkElement_When_searchQueryMatchesMessage")
```
Render `<LogRow entry={...message: "error in service"...} searchQuery="error" />`.
Assert: one `<mark>` element is present in the rendered output.
Assert: `<mark>` text content is `"error"` (case-insensitive match of input).

---

## E2E Tests

All E2E tests live in `tests/e2e/log-viewer.spec.ts`.

File header:
```typescript
// @feature logs:view, logs:search, logs:filter, logs:expand, logs:mobile
```

Run with:
```bash
# start test server first (separate terminal)
STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local ./stapler-squad --tmux-keep-server &
cd tests/e2e && npx playwright test log-viewer.spec.ts
```

All locators use `data-testid` or ARIA roles only (no CSS class selectors, no `waitForTimeout`).

---

### FR-1: Live Tail

#### T-E2E-001 · FR-1
```
test.describe("log-viewer", () => {
  it("log-viewer_should_autoScrollToBottom_When_liveTailEnabled")
```
1. Navigate to `/logs` page.
2. Mock API (`page.route`) to return 200 log entries via streaming.
3. Wait for entries to appear (`expect(page.locator('[data-testid^="log-row"]').last()).toBeVisible()`).
4. Assert: `[data-testid="jump-to-latest"]` is **not** visible.
5. Assert: the last log row is inside the viewport (use `isIntersectingViewport`).

#### T-E2E-002 · FR-1
```
  it("log-viewer_should_showJumpToLatest_When_userScrollsUp")
```
1. Load 500 log entries, wait for render.
2. Scroll up by 300px via `page.mouse.wheel(0, -300)` on the log container.
3. Wait for `[data-testid="jump-to-latest"]` to be visible.
4. Click `[data-testid="jump-to-latest"]`.
5. Assert: `[data-testid="jump-to-latest"]` disappears.
6. Assert: last log row is visible in viewport (follow resumed).

---

### FR-2: Horizontal Scroll / No Wrap

#### T-E2E-003 · FR-2
```
  it("log-viewer_should_scrollHorizontallyWithoutWrapping_When_longLinePresent")
```
1. Load one log entry with a 1000-character message.
2. Assert: the log row height is ≤ 60px (single-line — no wrap bloating the height).
3. Assert: the scrollable body div has `scrollWidth > clientWidth` (horizontal scroll available).
4. Assert: no `<br>` elements inside the log row body.

---

### FR-3: Search & Highlight

#### T-E2E-004 · FR-3
```
  it("log-viewer_should_highlightMatchesAndShowCount_When_searchQueryEntered")
```
1. Load 100 log entries; 15 contain the word "timeout".
2. Press `/` to focus search bar (no `waitForTimeout` — use `expect(searchInput).toBeFocused()`).
3. Type "timeout".
4. Assert: at least one `<mark>` element is visible on screen.
5. Assert: toolbar contains text matching `/15\s*\/\s*100/` (match count).
6. Press `Escape`.
7. Assert: search input value is empty.
8. Assert: no `<mark>` elements remain.

---

### FR-4: Level Coloring (visual, via aria/data attributes)

Level coloring is primarily verified through component tests (T-COMP-007, T-COMP-008) and
the accessibility contrast check (T-SEC-003). E2E smoke check is included in T-E2E-008.

---

### FR-5: Expandable Row

#### T-E2E-005 · FR-5
```
  it("log-viewer_should_expandRow_When_rowClicked")
```
1. Load 10 log entries; first entry is JSON: `{"level":"error","msg":"db connection failed"}`.
2. Click `[data-testid="log-row-0"]`.
3. Assert: `[aria-expanded="true"]` on `[data-testid="log-row-0"]`.
4. Assert: `[data-testid="expanded-detail-0"]` is visible.
5. Assert: expanded panel text contains `"db connection failed"`.
6. Click `[data-testid="log-row-1"]`.
7. Assert: `[aria-expanded="false"]` on row 0 (accordion collapsed first).
8. Assert: `[aria-expanded="true"]` on row 1.

---

### FR-6: Mobile Layout

#### T-E2E-006 · FR-6
```
  it("log-viewer_should_collapseSearch_When_viewportIsNarrow")
```
```typescript
test.use({ viewport: { width: 390, height: 844 } }); // iPhone 14
```
1. Navigate to `/logs`.
2. Assert: search input is NOT visible (collapsed to icon).
3. Click the search icon button (`[aria-label*="search" i]`).
4. Assert: search input is visible and focused.
5. Type "foo".
6. Assert: search input value is "foo" (search still functional when expanded).

#### T-E2E-007 · FR-6
```
  it("log-viewer_should_haveSufficientTouchTargets_When_mobileViewport")
```
```typescript
test.use({ viewport: { width: 390, height: 844 } });
```
1. Navigate to `/logs`.
2. For each filter chip (`[role="group"] button`): evaluate bounding box height via
   `element.getBoundingClientRect().height` (injected via `page.evaluate`).
3. Assert: every chip height ≥ 44.
4. Assert: search icon button height ≥ 44 and width ≥ 44.
5. Assert: `[data-testid="jump-to-latest"]` height ≥ 44 when visible.

---

### FR-7: Level Filter

#### T-E2E-008 · FR-7, FR-4 (smoke)
```
  it("log-viewer_should_filterToErrorOnly_When_errorChipSelected")
```
1. Load 50 entries: 10 ERROR, 15 WARN, 20 INFO, 5 DEBUG.
2. Click the ERROR filter chip.
3. Assert: all visible `[data-testid^="log-row"]` elements have `[data-level="ERROR"]`.
4. Assert: rows with other levels are not visible.
5. Click the WARN chip (multi-select).
6. Assert: visible rows have `data-level` in `["ERROR", "WARN"]`.
7. Click the ALL chip.
8. Assert: all 50 rows are now visible.

---

### NFR-2: Keyboard Navigation

#### T-E2E-009 · NFR-2
```
  it("log-viewer_should_navigateRowsViaKeyboard_When_arrowKeysPressed")
```
1. Load 20 log entries.
2. Focus the log container (click it once, then verify focus via `activeElement`).
3. Press `g` → assert first row is in viewport (scrolled to top).
4. Press `j` → assert row 1 is selected (`aria-selected="true"` or focused).
5. Press `j` again → assert row 2 is selected.
6. Press `Enter` → assert `aria-expanded="true"` on row 2.
7. Press `Enter` → assert `aria-expanded="false"` on row 2.
8. Press `G` → assert last row is in viewport.

---

### NFR-3: No New Endpoints (smoke)

#### T-E2E-010 · NFR-3
```
  it("log-viewer_should_useExistingEndpoints_When_loadingLogs")
```
1. Capture all network requests via `page.on("request", ...)`.
2. Navigate to `/logs` and wait for rows to appear.
3. Assert: no request was made to any URL path that did not exist before this feature
   (validate against a known-good list of existing endpoints; new `repeated levels` field
   is on existing `GetLogs` endpoint — that is acceptable).

---

## Security Tests

**File**: `web-app/src/lib/logs/__tests__/logParser.test.ts` (units) +
`tests/e2e/log-viewer-security.spec.ts` (browser-level)

#### T-SEC-001 · SEC — OSC Hyperlink XSS (unit; see also T-UNIT-016)
```
describe("renderAnsi — OSC XSS")
  it("T-SEC-001: renderAnsi_should_notProduceAnchorWithJavascriptHref")
```
Input: `"\x1b]8;;javascript:void(document.cookie='stolen=1')\x07Click\x1b]8;;\x07"`.
Parse the output as a DOM fragment (use `new DOMParser().parseFromString(..., 'text/html')`).
Assert: no `<a>` elements exist in the fragment.
Assert: no element has an attribute whose value starts with `"javascript:"`.

#### T-SEC-002 · SEC — HTML Injection via Log Content (E2E browser)
**File**: `tests/e2e/log-viewer-security.spec.ts`
```
  it("T-SEC-002: logViewer_should_notExecuteScript_When_logLineContainsXssPayload")
```
1. Mock the `GetLogs` RPC to return a log entry whose message is:
   `'<img src=x onerror="window.__xss_fired=true"> normal text'`.
2. Load the log viewer.
3. Evaluate `window.__xss_fired` in the page context.
4. Assert: value is `undefined` or `false` (XSS did not fire).

#### T-SEC-003 · FR-4, NFR-2 — WCAG AA Contrast (automated)
**File**: `tests/e2e/log-viewer-a11y.spec.ts`
```
  it("T-SEC-003: logViewer_should_passWCAGAAContrast_When_levelBadgesRendered")
```
1. Load log entries with all 5 levels (ERROR, WARN, INFO, DEBUG, TRACE).
2. Run Axe Core (`@axe-core/playwright`) on the log container.
3. Assert: no violations of rule `color-contrast`.
4. Explicitly check the level badge foreground/background pairs against WCAG AA ratio ≥ 4.5:1
   (use `page.evaluate` to call `getComputedStyle` on badge elements and verify colors).

Reference values (from plan.md §S3.2):
- ERROR badge: `#B91C1C` on `#FFF` → ratio ≈ 5.9 (passes)
- WARN badge: dark text `#1A1A1A` on `#F59E0B` → ratio ≈ 7.6 (passes)
- INFO badge: `#1D4ED8` on `#FFF` → ratio ≈ 5.9 (passes)
- DEBUG badge: `#6B7280` on `#FFF` → ratio ≈ 4.6 (passes, borderline — verify)

---

## Performance Tests (Manual Baseline)

These tests are not automated in CI but must be manually verified before each milestone merge.

**File**: `web-app/src/lib/logs/__tests__/logParser.perf.ts` (standalone, not part of jest suite)

#### T-PERF-001 · NFR-1 — Virtual Scroll DOM Count
**Method**: DevTools / Playwright `evaluate`
1. Load the log viewer with 10,000 synthetic entries in a 800px-tall container.
2. In DevTools (or via `page.evaluate`), count `document.querySelectorAll('[data-testid^="log-row"]').length`.
3. **Pass criteria**: ≤ 100 rows in DOM (Virtuoso default overscan renders ~30-50 rows).
4. **Fail threshold**: > 500 rows (indicates virtualization is broken).

#### T-PERF-002 · NFR-1 — Filter Latency (10k rows)
**Method**: Playwright `page.evaluate` with `performance.now()`
1. Inject 10,000 log entries into the component state (via mock or direct state injection).
2. Measure `performance.now()` before and after typing a query that matches ~500 entries.
3. **Pass criteria**: < 100 ms elapsed.
4. **Fail threshold**: > 500 ms.

#### T-PERF-003 · NFR-1 — Live Tail Frame Rate
**Method**: Chrome DevTools Performance tab / manual observation
1. Enable live tail with mock API returning 50 new entries every 2 seconds.
2. Monitor frame rate in DevTools for 30 seconds.
3. **Pass criteria**: sustained ≥ 55 fps; no jank frames > 50ms during append.
4. **Fail threshold**: < 30 fps or visible jank on scroll during live tail.

#### T-PERF-004 · NFR-1 — Scroll FPS (iOS Safari)
**Method**: Manual on physical iPhone (or BrowserStack)
Viewport: 390 × 844 (iPhone 14).
1. Load 2,000 entries, scroll rapidly up and down for 10 seconds.
2. **Pass criteria**: no visible blank row flashes; scroll feels fluid (subjective ≥ 90% smooth).
3. **Fail indicator**: white/blank rows visible during fast scroll → increase `overscan`.

---

## Coverage Summary

### Test Counts by Type

| Layer | Count |
|---|---|
| Unit tests (`logParser.ts`) | 12 |
| Unit tests (`useLogViewer.ts`) | 9 |
| Component tests (RTL) | 20 |
| E2E tests (Playwright) | 10 |
| Security tests (unit + E2E) | 3 |
| Performance tests (manual baseline) | 4 |
| **Total automated** | **54** |

### Requirements Coverage

| Requirement | Covered by | Status |
|---|---|---|
| FR-1 Live tail | T-UNIT-001–004, T-COMP-001–002, T-E2E-001–002 | Covered |
| FR-2 Horizontal scroll / no wrap | T-COMP-003–004, T-E2E-003 | Covered |
| FR-3 Search & highlight | T-UNIT-005–007, T-COMP-005–006B, T-E2E-004 | Covered |
| FR-4 Log level coloring | T-UNIT-008–010, T-COMP-007–008, T-SEC-003 | Covered |
| FR-5 Expandable row / JSON | T-UNIT-011–012, T-COMP-009–011, T-E2E-005 | Covered |
| FR-6 Mobile UX / touch targets | T-COMP-012–013, T-E2E-006–007 | Covered |
| FR-7 Level filter chips | T-UNIT-013–014, T-COMP-014–015, T-E2E-008 | Covered |
| NFR-1 Virtual scroll perf | T-COMP-016, T-UNIT-PERF-001, T-PERF-001–004 | Covered |
| NFR-2 Keyboard / accessibility | T-COMP-017–018, T-COMP-017B, T-E2E-009, T-SEC-003 | Covered |
| NFR-3 No new endpoints | T-E2E-010 | Covered |
| SEC XSS / ANSI OSC | T-UNIT-015–017, T-SEC-001–002 | Covered |

**Coverage: 11 / 11 requirements (100%)** including all 7 FRs, all 3 NFRs, and the security requirement.

### Test File Summary

| File | Layer | Tests |
|---|---|---|
| `web-app/src/lib/logs/__tests__/logParser.test.ts` | Unit | 12 |
| `web-app/src/lib/hooks/__tests__/useLogViewer.test.ts` | Unit | 9 |
| `web-app/src/components/logs/__tests__/LogRow.test.tsx` | Component | 6 |
| `web-app/src/components/logs/__tests__/JumpToLatestButton.test.tsx` | Component | 2 |
| `web-app/src/components/logs/__tests__/LevelFilterChips.test.tsx` | Component | 3 |
| `web-app/src/components/logs/__tests__/LogViewerToolbar.test.tsx` | Component | 2 |
| `web-app/src/components/logs/__tests__/ExpandedLogDetail.test.tsx` | Component | 3 |
| `web-app/src/components/logs/__tests__/VirtualLogList.test.tsx` | Component | 1 |
| `web-app/src/components/logs/__tests__/LogViewer.test.tsx` | Component | 3 |
| `tests/e2e/log-viewer.spec.ts` | E2E | 9 |
| `tests/e2e/log-viewer-security.spec.ts` | Security (E2E) | 1 |
| `tests/e2e/log-viewer-a11y.spec.ts` | Security/A11y | 1 |
| Manual perf baseline | Manual | 4 |
