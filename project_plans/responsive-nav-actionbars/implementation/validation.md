# Validation Plan: Responsive Nav & Action Bars

Status: Ready
Created: 2026-04-13
Plan: `docs/tasks/responsive-nav-actionbars.md`
Requirements: `project_plans/responsive-nav-actionbars/requirements.md`

---

## Coverage Matrix

Every requirement maps to at least one test case below.

| Requirement | Test Cases | Type |
|-------------|-----------|------|
| REQ-1: Header renders 320–2560px without overflow | TC-1.1, TC-1.2, TC-1.3 | Visual/Manual |
| REQ-2: All controls accessible at any viewport width | TC-2.1, TC-2.2, TC-2.3, TC-2.4 | Manual |
| REQ-3: No layout jumps on dynamic resize | TC-3.1, TC-3.2 | Manual |
| REQ-4: ≤768px touch targets ≥44px | TC-4.1, TC-4.2 | Manual/DevTools |
| REQ-5: No new component APIs unless ActionBar insufficient | TC-5.1 | Code review |
| REQ-6: No overflow/clipping at 800–1100px dead zone | TC-1.2, TC-6.1 | Visual/Manual |
| REQ-7: WorkspaceSwitcher no overflow at any width | TC-7.1, TC-7.2 | Manual |
| REQ-8: ActionBar used consistently | TC-5.1, TC-8.1 | Code review/Build |

---

## Test Cases

### Story 1: Header + WorkspaceSwitcher

#### TC-1.1: Header full-width range — no overflow

**Requirement:** REQ-1
**Task:** 1.1
**Type:** Manual / DevTools
**Breakpoints to test:** 320, 375, 480, 640, 768, 800, 900, 1024, 1100, 1280, 1440, 1920, 2560

**Steps:**
1. Open `http://localhost:8543` in Chrome with DevTools open
2. Temporarily comment out `overflow-x: hidden` on `body` in DevTools Styles panel to expose real overflow
3. For each breakpoint, set responsive mode to that width
4. Verify: no horizontal scrollbar, no element clipping, no element overlap

**Pass criteria:**
- At all widths: no element overflows the viewport
- At 900px: header actions (WorkspaceSwitcher + buttons) and nav links do not overlap
- At 768px: hamburger menu present, desktop nav hidden
- At 1440px+: full desktop layout, no regression from current

**Fail evidence:** Screenshot showing overlap or horizontal scrollbar appearing when body overflow-x is revealed

---

#### TC-1.2: Header 800–1100px dead zone eliminated

**Requirement:** REQ-6, REQ-1
**Task:** 1.1
**Type:** Manual
**Critical breakpoints:** 768, 800, 900, 1000, 1024, 1100

**Steps:**
1. Set DevTools to 900px width
2. Verify "New Session" button shows only the `+` icon (no "New Session" text)
3. Verify all header action buttons (WorkspaceSwitcher, +, approval badge, notification, debug, help) are visible without overlap
4. Verify nav links are visible and not pushed off-screen
5. Drag window width between 768px and 1024px continuously and observe no sudden layout jumps

**Pass criteria:**
- At 900px: `newSessionLabel` text hidden, `+` icon visible
- At 1024px: compact mode engaged (smaller gaps, no "Session Manager" subtitle)
- At 1024px → 1025px: single visual transition point (not two separate transitions)
- Header height change (4rem → 3.5rem) coincides with compact breakpoint at 1024px

---

#### TC-1.3: Hamburger menu not broken by overflow guard

**Requirement:** KI-1 mitigation
**Task:** 1.1
**Type:** Manual
**Viewport:** 375px

**Steps:**
1. Set DevTools to 375px
2. Click the hamburger menu icon
3. Verify mobile nav dropdown appears and is fully visible (not clipped)
4. Click a nav link in the dropdown
5. Click outside to close

**Pass criteria:**
- Mobile nav dropdown appears below header, unclipped
- All nav links visible and clickable
- Clicking outside dismisses the nav

---

#### TC-2.1: All header action buttons reachable at 375px

**Requirement:** REQ-2, REQ-4
**Task:** 1.1, 1.2
**Type:** DevTools element inspector
**Viewport:** 375px

**Steps:**
1. Open DevTools at 375px
2. Inspect each interactive element in the header: WorkspaceSwitcher trigger, hamburger button, any visible action buttons
3. In DevTools → Computed styles → verify `height` and `width` are ≥44px (or use accessibility audit)

**Pass criteria:**
- WorkspaceSwitcher trigger: ≥44px height
- Hamburger button: ≥44px height and width
- All interactive header elements meet `--min-touch-target: 44px`

---

#### TC-7.1: WorkspaceSwitcher dropdown stays within viewport at 320px

**Requirement:** REQ-7, KI-5 mitigation
**Task:** 1.2
**Type:** Manual
**Viewports:** 320px, 375px, 414px

**Steps:**
1. Set DevTools to 320px
2. Click WorkspaceSwitcher trigger button
3. Observe dropdown position
4. Verify dropdown left edge ≥ 0 (not off-screen left)
5. Verify dropdown right edge ≤ viewport width (not off-screen right)
6. Repeat at 375px and 414px

**Pass criteria:**
- Dropdown visible within viewport at all three widths
- Dropdown content readable (no horizontal scroll required to see workspace names)
- Dropdown closes on outside click

---

#### TC-7.2: WorkspaceSwitcher trigger compressed at 768px

**Requirement:** REQ-7
**Task:** 1.2
**Type:** DevTools
**Viewport:** 375px

**Steps:**
1. Inspect WorkspaceSwitcher `.trigger` element at 375px
2. Verify `max-width` is 120px (not 160px)
3. Verify the text inside trigger truncates correctly (ellipsis, not overflow)
4. Count the number of visible header buttons: WorkspaceSwitcher + icon buttons should all be present

**Pass criteria:**
- Trigger `max-width` ≤ 120px at 375px
- No icon button hidden or pushed off-screen

---

#### TC-3.1: Header height change is single-point at 1024px

**Requirement:** REQ-3, KI-2 mitigation
**Task:** 1.3
**Type:** Manual
**Viewport:** Drag across 1024px

**Steps:**
1. Open browser at 1100px width
2. Open DevTools → Computed styles on a content element that uses `padding-top: var(--header-height)` (the main layout wrapper)
3. Slowly drag window narrower from 1100px to 900px
4. Observe: content position should change exactly once at 1024px

**Pass criteria:**
- `--header-height` is 4rem from 1025px and above
- `--header-height` is 3.5rem from 1024px and below
- No 8px shift visible when crossing 768px (the old breakpoint)

---

### Story 2: ActionBar Consistency

#### TC-5.1: ActionBar `compact` prop — unit test

**Requirement:** REQ-5, REQ-8
**Task:** 2.1
**Type:** Unit test (React Testing Library or manual DOM check)
**Command:** `go test ./ui/... -run TestActionBar` (if Go tests exist) or browser visual test

**Steps:**
1. Render `<ActionBar compact>` in isolation
2. At viewport 900px: inspect computed `gap` — should be smaller than default (< 1rem)
3. At viewport 600px: inspect: `overflow-x` should be `auto`, `flex-wrap` should be `nowrap`
4. Render `<ActionBar>` without compact: verify no change at 900px or 600px (no regression)
5. Render `<ActionBar scroll>`: verify scroll activates at 640px (not 768px) — existing behavior preserved

**Pass criteria:**
- `compact` prop produces smaller gap at 1024px breakpoint
- `compact` prop produces `overflow-x: auto` at 768px
- Props `scroll` and `compact` do not interfere with each other
- No visual change to `<ActionBar>` with no props

---

#### TC-8.1: ActionBar applied to SessionList and HistoryFilterBar — build check

**Requirement:** REQ-8
**Task:** 2.2, 2.3
**Type:** Build / TypeScript
**Command:** `make quick-check`

**Steps:**
1. After completing Tasks 2.2 and 2.3, run `make quick-check`
2. Verify build passes (no TypeScript errors)
3. Verify lint passes (CSS vars exist in globals.css)

**Pass criteria:** `make quick-check` exits 0

---

#### TC-2.2: SessionList filter bar responsive behavior

**Requirement:** REQ-2, REQ-4
**Task:** 2.2
**Type:** Manual
**Viewports:** 1440px, 900px, 768px, 375px

**Steps at 1440px:**
1. Open Sessions page
2. Verify filter bar shows all controls (Group by, Status, Tag dropdowns + search)
3. Verify layout identical to before (regression check)

**Steps at 900px:**
1. Verify filter bar visible, controls reachable with tighter gaps
2. No horizontal overflow

**Steps at 768px:**
1. Tap the filter toggle button
2. Verify filter panel appears below search bar (stacked vertically)
3. Verify each control meets 44px touch target
4. Tap filter toggle again to hide panel

**Steps at 375px:**
1. Expand filter panel
2. Drag horizontally within the filter panel — verify horizontal scroll if controls don't fit
3. Verify no control is unreachable

**Pass criteria:**
- Filter panel visible at all widths
- Mobile toggle (show/hide) still functional (KI-3 mitigation verified)
- No `display: contents` pattern in rendered DOM (inspect element)
- All controls meet 44px touch target at 375px

---

#### TC-2.3: HistoryFilterBar responsive behavior

**Requirement:** REQ-2
**Task:** 2.3
**Type:** Manual
**Viewports:** 1440px, 768px, 375px

**Steps at 1440px:**
1. Open History page
2. Verify filter bar identical to current (regression check)

**Steps at 768px:**
1. Verify all filter selects visible and reachable
2. Verify bar scrolls horizontally if items don't fit

**Steps at 375px:**
1. Verify selects meet 44px height
2. Sort order button meets 44px height and width

**Pass criteria:**
- No overflow at any width
- Touch targets ≥44px at 375px

---

#### TC-4.1: All filter controls meet 44px touch target on mobile

**Requirement:** REQ-4
**Task:** 2.2, 2.3
**Type:** DevTools accessibility audit
**Viewport:** 375px

**Steps:**
1. Open Chrome DevTools → Lighthouse → Accessibility audit (or manual inspect)
2. Check SessionList filter selects, buttons
3. Check HistoryFilterBar selects, sort button

**Pass criteria:** All interactive elements height and width ≥ 44px

---

### Story 3: Logs Dropdown Viewport Guards

#### TC-2.4: All logs toolbar controls accessible at 375px

**Requirement:** REQ-2
**Task:** 3.1, 3.2
**Type:** Manual
**Viewport:** 375px

**Steps:**
1. Navigate to Logs page at 375px
2. Verify toolbar scrolls horizontally (ActionBar compact scroll mode)
3. Open ExportButton dropdown: verify within viewport bounds
4. Open TimeRangePicker dropdown: verify within viewport bounds
5. Open MultiSelect dropdown: verify within viewport bounds
6. Click LiveTailToggle: verify toggle button meets 44px, any dropdown within viewport

**Pass criteria:**
- All dropdowns stay within viewport (no off-screen overflow)
- Toolbar scrollable, all buttons reachable by scrolling

---

#### TC-6.1: ExportButton dropdown bounded at 375px

**Requirement:** KI-6 mitigation, REQ-2
**Task:** 3.1
**Type:** Manual + DevTools
**Viewport:** 375px, 320px

**Steps:**
1. Open Logs page at 375px
2. Click Export button
3. In DevTools, inspect `.dropdown` element
4. Verify `getBoundingClientRect().right ≤ window.innerWidth`
5. Verify `getBoundingClientRect().left ≥ 0`
6. Repeat at 320px

**Pass criteria:** Dropdown fully within `[0, viewport.width]` at both widths

---

#### TC-6.2: TimeRangePicker, MultiSelect, LiveTailToggle dropdowns bounded at 375px

**Requirement:** KI-6 mitigation, REQ-2
**Task:** 3.1
**Type:** Manual
**Viewport:** 375px

Same procedure as TC-6.1 for each component.

**Pass criteria:** All three dropdown bounding rects within viewport

---

#### TC-3.2: Logs page ActionBar — no regression at 1440px

**Requirement:** REQ-2 (no regression)
**Task:** 3.2
**Type:** Manual / screenshot comparison
**Viewport:** 1440px

**Steps:**
1. Screenshot logs page toolbar at 1440px before changes
2. After Task 3.2: screenshot again
3. Compare: pixel-identical or intentionally equivalent

**Pass criteria:** No visual change on desktop after ActionBar wrapping

---

### Build & Lint (all tasks)

#### TC-BUILD-1: Build passes after each task

**Type:** Automated
**Command:** `make quick-check`

Run after completing each task:
- After Task 1.1 alone
- After Task 1.2 alone
- After Task 1.3 alone
- After Task 2.1
- After Task 2.2 + 2.3
- After Task 3.1 alone
- After Task 3.2

**Pass criteria:** `make quick-check` exits 0 at every checkpoint

---

#### TC-BUILD-2: CSS lint passes — no undefined vars

**Type:** Automated
**Command:** `make lint` or `npm run lint:css`

Verify no `var(--undefined-var)` introduced in any `.module.css` change.

**Pass criteria:** CSS linter exits 0

---

#### TC-BUILD-3: Dark mode not broken

**Requirement:** CSS constraints (use defined CSS variables only)
**Type:** Manual
**Viewport:** 1440px, 375px

**Steps:**
1. Toggle dark mode (if applicable)
2. Verify Header, SessionList filters, HistoryFilterBar, Logs toolbar all use correct CSS variables
3. No hardcoded hex colors introduced

**Pass criteria:** Components render correctly in both light and dark mode; no hardcoded color values in changed CSS files

---

### Regression

#### TC-REG-1: Desktop layout unchanged at 1440px

**Type:** Manual visual regression
**Viewports:** 1440px, 1920px
**Pages:** Sessions, History, Logs, Header

For each page, verify layout is pixel-identical (or intentionally equivalent) to pre-change state.

---

#### TC-REG-2: Keyboard navigation preserved

**Requirement:** REQ-2 (fully accessible)
**Type:** Manual
**Viewport:** 1440px

**Steps:**
1. Tab through Header: logo → nav links → WorkspaceSwitcher → action buttons
2. Tab through SessionList: search → filter toggle → filter controls (if visible)
3. Tab through HistoryFilterBar: all selects and sort button
4. Tab through Logs toolbar: LiveTailToggle → TimeRangePicker → Export → MultiSelect

**Pass criteria:** All interactive elements reachable via keyboard, focus ring visible, no keyboard trap

---

#### TC-REG-3: Existing `<ActionBar scroll>` behavior unchanged

**Requirement:** KI-4 mitigation
**Task:** 2.1
**Type:** Manual
**Viewport:** 600px

Find any existing usage of `<ActionBar scroll>` in the codebase. Verify the scroll behavior at 640px is unchanged after adding the `compact` prop.

**Pass criteria:** Pre-existing `scroll` behavior fires at 640px, not affected by `compact` changes

---

## Test Execution Order

```
Phase 1 — Build gates (run after every change):
  TC-BUILD-1 → TC-BUILD-2

Phase 2 — Story 1 verification (after Tasks 1.1, 1.2, 1.3):
  TC-1.1 → TC-1.2 → TC-1.3 → TC-7.1 → TC-7.2 → TC-3.1

Phase 3 — Story 2 verification (after Tasks 2.1, 2.2, 2.3):
  TC-5.1 → TC-8.1 → TC-2.2 → TC-2.3 → TC-4.1

Phase 4 — Story 3 verification (after Tasks 3.1, 3.2):
  TC-2.4 → TC-6.1 → TC-6.2 → TC-3.2

Phase 5 — Final regression (all tasks done):
  TC-REG-1 → TC-REG-2 → TC-REG-3 → TC-BUILD-3
```

---

## Definition of Done

All of the following must be true before this feature is complete:

- [ ] `make quick-check` passes (build + tests + lint)
- [ ] TC-1.1: Header renders correctly at 320, 375, 480, 640, 768, 800, 900, 1024, 1280, 1440px
- [ ] TC-1.2: 800–1100px dead zone eliminated (no overlap between nav and actions at 900px)
- [ ] TC-1.3: Hamburger menu unbroken after overflow guard added
- [ ] TC-7.1: WorkspaceSwitcher dropdown within viewport at 320px
- [ ] TC-3.1: Single header height transition point at 1024px (no 768px jump)
- [ ] TC-5.1: ActionBar `compact` prop behavior verified, no regression on `scroll`
- [ ] TC-2.2: SessionList mobile filter toggle still works at 375px
- [ ] TC-2.3: HistoryFilterBar selects ≥44px height at 375px
- [ ] TC-6.1, TC-6.2: All logs dropdowns within viewport at 375px
- [ ] TC-REG-1: Desktop layout unchanged at 1440px
- [ ] TC-REG-2: Keyboard navigation intact
- [ ] TC-BUILD-3: Dark mode unchanged

---

## Known Gaps

- **No automated visual regression tests**: The project has no snapshot or screenshot diffing setup. All visual checks are manual DevTools procedures. If this feature is revisited, adding playwright visual snapshots at key breakpoints would make regression detection automatic.
- **No automated touch target test**: Lighthouse accessibility audit is manual. A custom Jest + Testing Library test that checks rendered element dimensions would be more reliable but requires jsdom to compute layout (difficult).
- **iOS Safari not tested in CI**: iOS Safari 15.4 is the minimum browser target but cannot be automated without a real device or XCUITest. The CSS patterns used (`min()`, `max()`, `overflow-x: auto`, `flex-wrap`) are all supported in iOS Safari 15.4 per MDN, but should be verified on a real device or simulator before ship.
