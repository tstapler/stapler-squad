# Validation Plan: ux-overhaul

**Feature**: Mobile friction fixes, design system foundation, UX best-practices alignment
**Date**: 2026-04-26
**Status**: Ready
**Links**: [requirements.md](../requirements.md) | [plan.md](plan.md)

---

## Coverage Summary

| Layer | Count |
|---|---|
| Unit tests (React Testing Library / Jest) | 14 |
| Playwright E2E (automated, desktop Chrome) | 22 |
| Visual regression (Playwright screenshot) | 6 |
| Manual device checks (iOS Safari / Pixel Fold) | 18 |
| **Total test cases** | **60** |

**Milestone 1 requirements coverage**: 7 / 7 success criteria covered (100%)
**Milestone 2 requirements coverage**: 6 / 6 success criteria covered at approach level (100%)

---

## Milestone 1 — Detailed Test Cases

### SC-1: Terminal toolbar compact/expanded toggle — one tap, state persists in localStorage

**Requirement**: A single persistent toggle switches between compact and expanded states; toggle is always reachable with one tap; state is remembered in `localStorage`.

#### Unit Tests (Jest + React Testing Library)

**U-1.1** — Toggle renders and is always visible
- Component: `TerminalOutput`
- Covers: toggle button renders in both `toolbarExpanded=true` and `toolbarExpanded=false` states
- Acceptance: `getByRole('button', { name: /collapse toolbar|expand toolbar/i })` is present in both renders; no DOM exceptions

**U-1.2** — Toolbar items hidden when compact, visible when expanded
- Component: `TerminalOutput`
- Covers: conditional rendering of `.toolbarActions` container
- Acceptance: when `toolbarExpanded=false`, toolbar action buttons are not in the DOM; when `true` they are

**U-1.3** — Toggle writes to localStorage
- Component: `TerminalOutput`
- Covers: `useEffect` → `localStorage.setItem('stapler-squad-toolbar-expanded', ...)`
- Acceptance: after clicking the toggle button, `localStorage.getItem('stapler-squad-toolbar-expanded')` equals `'false'`; clicking again returns `'true'`

**U-1.4** — Toggle initialises from localStorage
- Component: `TerminalOutput`
- Covers: `useState` lazy initializer reading `'stapler-squad-toolbar-expanded'`
- Acceptance: mounting the component with `localStorage` pre-set to `'false'` renders in compact mode without any user interaction

#### Playwright E2E Tests (spec file: `tests/e2e/terminal-toolbar-toggle.spec.ts`)

**E-1.1** — Toggle button is reachable in one interaction from session view
```
// @feature ui:terminal-toolbar-toggle
```
- Navigate to a session; locate `[data-testid="toolbar-toggle"]`
- `await expect(toggleBtn).toBeVisible()`
- Click; assert `[data-testid="toolbar-actions"]` is not visible
- Acceptance: passes in Chromium desktop; no `waitForTimeout`

**E-1.2** — State survives page reload
- Click toolbar toggle to collapse; reload; assert toolbar still collapsed
- Acceptance: `[data-testid="toolbar-actions"]` absent after reload; `localStorage['stapler-squad-toolbar-expanded']` equals `'false'`

**E-1.3** — Desktop media query: toolbar always visible at ≥1024px
- Set viewport to `1280×800`; navigate to session
- Acceptance: `[data-testid="toolbar-actions"]` visible even when localStorage is `'false'`; toggle button not visible (CSS `display:none`)

#### Manual Device Check

**M-1.1** — One-tap reachability on iPhone
- Device: iPhone (iOS Safari, portrait)
- Steps: navigate to any session; tap the toggle; confirm toolbar collapses in single tap
- Close Safari tab, reopen app; confirm compact state persists
- Pass: toggle requires exactly one tap; state survives background/foreground cycle
- Fail: requires long-press, double-tap, or precision targeting under 44px

---

### SC-2: Session name always visible in session view header on mobile with keyboard open

**Requirement**: Session title is displayed in the SessionDetail header as a sticky element; visible even when the iOS virtual keyboard is open; no content shift when keyboard appears.

#### Unit Tests

**U-2.1** — SessionDetail header renders session title
- Component: `SessionDetail`
- Covers: `<h2>` or equivalent with session title text; `position:sticky` CSS class applied
- Acceptance: `getByRole('heading', { name: session.title })` found; heading element has `flex-shrink: 0` in computed style (via jsdom inline style mock)

**U-2.2** — Title truncates with ellipsis and carries `title` attribute
- Component: `SessionDetail`
- Covers: long title (>60 chars) renders with `title` attr equal to full title
- Acceptance: `heading.title` equals full session name; visual truncation class is applied

#### Playwright E2E Tests (spec file: `tests/e2e/session-header-visibility.spec.ts`)

**E-2.1** — Session title visible in header after navigation
```
// @feature ui:session-header
```
- Navigate to session detail view; assert `[data-testid="session-header-title"]` is visible
- Acceptance: locator is visible; `textContent` matches session title

**E-2.2** — Header does not scroll away (sticky)
- Open session with long terminal output; scroll terminal to bottom
- Acceptance: `[data-testid="session-header-title"]` remains visible (`isVisible()` returns `true` after scroll)

**E-2.3** — Status badge visible alongside title
- Navigate to a running session
- Acceptance: `[data-testid="session-status-badge"]` is visible in the header and contains expected status text

#### Manual Device Check

**M-2.1** — Title visible with keyboard open (iOS Safari)
- Device: iPhone (iOS Safari)
- Steps: open session; tap terminal input to open keyboard; verify session name in header is still visible above keyboard
- Pass: header title visible without scrolling when keyboard is open
- Fail: header scrolls under keyboard or is hidden

---

### SC-3: Session actions (delete, pause/resume, rename/retag, worktree) accessible from session view

**Requirement**: All session actions accessible from within the session view — no need to return to sessions list.

#### Unit Tests

**U-3.1** — Action sheet renders all expected actions
- Component: `SessionDetail` with `actionSheetOpen=true`
- Covers: Pause/Resume, Rename, Edit Tags, Switch Workspace, Delete buttons all present
- Acceptance: `getByRole('button', { name: /pause|resume/i })`, `getByRole('button', { name: /rename/i })`, `getByRole('button', { name: /edit tags/i })`, `getByRole('button', { name: /delete/i })` all found; Delete has error/destructive color class

**U-3.2** — Pause/Resume label conditional on session status
- Component: `SessionDetail`
- Covers: when `session.status === 'PAUSED'`, button text is "Resume"; otherwise "Pause"
- Acceptance: two renders with different status produce correct label

**U-3.3** — External sessions hide Pause/Resume
- Component: `SessionDetail`
- Covers: `instanceType === EXTERNAL` → no Pause/Resume button in sheet
- Acceptance: `queryByRole('button', { name: /pause|resume/i })` is null when external

**U-3.4** — Delete confirmation dialog for running sessions
- Component: `SessionDetail`
- Covers: clicking Delete on a running session shows confirmation modal
- Acceptance: delete button click sets `showDeleteConfirm=true`; confirmation modal text matches "running"

#### Playwright E2E Tests (spec file: `tests/e2e/session-actions.spec.ts`)

**E-3.1** — ⋯ button opens action sheet
```
// @feature ui:session-actions-sheet, session:delete, session:update
```
- Navigate to session detail; click `[data-testid="more-actions-button"]`
- Acceptance: `[data-testid="action-sheet"]` visible; contains at minimum Delete, Rename, Edit Tags buttons

**E-3.2** — Action sheet dismisses on outside click
- Open sheet; click overlay/backdrop
- Acceptance: `[data-testid="action-sheet"]` is not visible after click; no JS errors

**E-3.3** — Pause action closes sheet and reflects new status
- Navigate to a running session; open action sheet; click Pause
- Acceptance: sheet closes; session status badge updates to "Paused" (poll with `waitForSelector`)

**E-3.4** — Rename action: empty title rejected
- Open sheet → Rename; clear input; attempt save
- Acceptance: inline validation error visible; session title unchanged

**E-3.5** — Delete flow: confirmation for running session
- Open sheet → Delete on a running session
- Acceptance: confirmation dialog visible (`[data-testid="delete-confirm-dialog"]`); Cancel closes it without deleting

#### Manual Device Check

**M-3.1** — Action sheet renders as bottom sheet on iOS
- Device: iPhone (iOS Safari)
- Steps: open session; tap ⋯; verify sheet slides up from bottom
- Pass: sheet uses bottom-sheet presentation; all actions tappable; sheet dismisses on backdrop tap
- Fail: sheet renders as centered modal on mobile; actions clipped by screen edge

**M-3.2** — Rename modal: no iOS zoom on input focus
- Device: iPhone (iOS Safari)
- Steps: open session → ⋯ → Rename; tap the rename input
- Pass: no viewport zoom when input is focused; font size ≥16px visually
- Fail: viewport zooms in on input tap

---

### SC-4: Bottom navbar does not occlude content

**Requirement**: Page content has correct `padding-bottom` / `env(safe-area-inset-bottom)` to clear the navbar; no content hidden under the nav.

#### Unit Tests

**U-4.1** — BottomNav applies safe-area padding-bottom
- Component: `BottomNav`
- Covers: CSS class/style includes `max(env(safe-area-inset-bottom, 0px), 8px)` pattern
- Acceptance: rendered element has a class whose CSS definition includes `env(safe-area-inset-bottom)` (snapshot test on the CSS-in-TS output)

**U-4.2** — Session list page has padding-bottom that references `--bottom-nav-height`
- Component: session list page wrapper
- Covers: CSS module class applied to page content container
- Acceptance: snapshot includes `padding-bottom` referencing `--bottom-nav-height` variable

#### Playwright E2E Tests (spec file: `tests/e2e/bottom-nav-clearance.spec.ts`)

**E-4.1** — Last item in session list is not occluded
```
// @feature ui:bottom-nav-clearance
```
- Load session list with enough sessions to scroll; scroll to bottom
- Acceptance: last `[data-testid="session-card"]` bounding box bottom is less than `BottomNav` bounding box top (no overlap); computed with `getBoundingClientRect` via `page.evaluate`

**E-4.2** — `--bottom-nav-height` CSS variable is defined
- Navigate to home; evaluate `getComputedStyle(document.documentElement).getPropertyValue('--bottom-nav-height')`
- Acceptance: value is a non-empty string parseable as px (e.g., `' 56px'`)

#### Manual Device Check

**M-4.1** — Bottom nav does not hide last session card (iPhone, Safari)
- Device: iPhone (iOS Safari, portrait)
- Steps: populate session list; scroll to very bottom; verify last session card fully visible above nav bar
- Pass: last card not clipped; action buttons on card fully tappable
- Fail: any part of the last card overlaps the bottom nav

**M-4.2** — Safe-area inset respected (iPhone with home indicator)
- Device: iPhone (Face ID model, home indicator visible)
- Steps: observe bottom nav; verify nav content does not overlap the home indicator swipe zone
- Pass: nav items sit above the home indicator area
- Fail: nav items overlap the home indicator

---

### SC-5: All interactive elements in terminal/session view ≥44px touch target

**Requirement**: All interactive elements in the terminal and session view meet 44px minimum touch target; no accidental out-of-session taps.

#### Unit Tests

**U-5.1** — Mobile keyboard button styles include min touch target
- Component: `TerminalOutput` (mobile keyboard row)
- Covers: `.mobileKey` CSS class has `min-height: var(--min-touch-target, 44px)` and `min-width: var(--min-touch-target, 44px)`
- Acceptance: rendered buttons have computed `minHeight` / `minWidth` ≥ `44px` via style snapshot

**U-5.2** — `--min-touch-target` CSS variable is defined
- Covers: `globals.css` root definition
- Acceptance: CSS file contains `--min-touch-target: 44px` in `:root` block (static file assertion)

#### Playwright E2E Tests (spec file: `tests/e2e/touch-targets.spec.ts`)

**E-5.1** — Toolbar toggle button is ≥44px in both dimensions
```
// @feature ui:touch-targets
```
- Navigate to session; evaluate `[data-testid="toolbar-toggle"]` bounding box
- Acceptance: `width >= 44` and `height >= 44`

**E-5.2** — ⋯ actions button is ≥44px in both dimensions
- Navigate to session; evaluate `[data-testid="more-actions-button"]` bounding box
- Acceptance: `width >= 44` and `height >= 44`

**E-5.3** — All buttons in session detail header are ≥44px
- Navigate to session; query all `button` descendants of `[data-testid="session-header"]`; evaluate each bounding box
- Acceptance: every button has `width >= 44` and `height >= 44`

**E-5.4** — Mobile keyboard buttons ≥44px (mobile viewport emulation)
- Set viewport to `390×844` (iPhone 14 Pro); navigate to session; evaluate `.mobileKey` or `[data-testid="mobile-key"]` bounding boxes
- Acceptance: all mobile key buttons ≥44px in height

#### Manual Device Check

**M-5.1** — One-handed tap accuracy on terminal toolbar (iPhone)
- Device: iPhone (non-dominant hand one-handed hold)
- Steps: use thumb to tap each toolbar button; note any mis-taps
- Pass: zero mis-taps on toggle, ⋯, and top-row action buttons across 10 trials
- Fail: any button requires precision tap or causes accidental adjacent tap

---

### SC-6: iOS virtual keyboard — layout uses dvh/visualViewport; terminal/input remain visible

**Requirement**: Layout uses `dvh`/`visualViewport` so terminal and input remain visible when keyboard is open; zero occurrences of bare `100vh` in layout CSS.

#### Unit Tests

**U-6.1** — ViewportProvider writes `--viewport-height` from visualViewport
- Component: `ViewportProvider`
- Covers: `window.visualViewport` resize event → `document.documentElement.style.setProperty('--viewport-height', ...)`
- Acceptance: mock `visualViewport` with `height=600`; fire resize event; assert CSS var set to `'600px'`

**U-6.2** — SessionDetail container uses `var(--viewport-height)` not `100vh`
- Static: scan `SessionDetail.module.css` (or `.css.ts`) for literal `100vh`
- Acceptance: zero occurrences of `100vh` in `SessionDetail` CSS file

**U-6.3** — No `100vh` in globals.css after migration
- Static: scan `web-app/src/app/globals.css` for literal `100vh`
- Acceptance: zero occurrences (replaced by `var(--viewport-height)` or `100dvh` in the root definition)

#### Playwright E2E Tests (spec file: `tests/e2e/keyboard-layout.spec.ts`)

**E-6.1** — `--viewport-height` CSS variable is set on document root
```
// @feature ui:keyboard-aware-layout
```
- Load app; evaluate `document.documentElement.style.getPropertyValue('--viewport-height')` or `getComputedStyle(document.documentElement).getPropertyValue('--viewport-height')`
- Acceptance: non-empty value set by ViewportProvider

**E-6.2** — Terminal container height responds to `--viewport-height` reduction
- Simulate keyboard open: `await page.evaluate(() => { document.documentElement.style.setProperty('--viewport-height', '400px'); })`
- Assert `[data-testid="session-terminal-container"]` computed height ≤ 400px
- Acceptance: terminal shrinks proportionally; no overflow or scroll escape

**E-6.3** — Zero bare `100vh` in page layout CSS (static check via test)
- `page.evaluate` fetches all loaded stylesheets and checks text for `100vh` outside `:root` blocks
- Acceptance: zero matches

#### Manual Device Check

**M-6.1** — Terminal visible with keyboard open (iPhone)
- Device: iPhone (iOS Safari)
- Steps: open a session; tap the terminal input to bring up keyboard; verify terminal area and input row are fully visible without scrolling
- Pass: both terminal output and the input row visible simultaneously with keyboard open
- Fail: terminal input scrolled off-screen or hidden behind keyboard

**M-6.2** — Keyboard open/close does not cause layout jump
- Device: iPhone (iOS Safari)
- Steps: open a session; quickly open and dismiss keyboard 3×; observe layout
- Pass: no visible layout jump or content flash when keyboard transitions
- Fail: any visible reflow, white flash, or content shift

---

### SC-7: Safe-area insets applied — notch and home indicator never clip content

**Requirement**: `env(safe-area-inset-*)` applied so notch and home indicator never clip content.

#### Unit Tests

**U-7.1** — Safe-area CSS variables defined in globals.css
- Static: scan `web-app/src/app/globals.css` for `--safe-area-top`, `--safe-area-bottom`, `--safe-area-left`, `--safe-area-right`
- Acceptance: all four defined in `:root` using `env(safe-area-inset-*, 0px)` pattern

**U-7.2** — BottomNav CSS includes `env(safe-area-inset-bottom)` in padding-bottom
- Static: scan `BottomNav.css.ts` (or `.module.css`) for `env(safe-area-inset-bottom`
- Acceptance: pattern found; uses `max(env(safe-area-inset-bottom, 0px), 8px)` form

**U-7.3** — Terminal container CSS includes lateral safe-area padding
- Static: scan `TerminalOutput.module.css` for `var(--safe-area-left)` and `var(--safe-area-right)`
- Acceptance: both present in `.terminalContainer` rule

#### Playwright E2E Tests (spec file: `tests/e2e/safe-area-insets.spec.ts`)

**E-7.1** — Safe-area CSS variables are defined at root level
```
// @feature ui:safe-area-insets
```
- Evaluate: `getComputedStyle(document.documentElement).getPropertyValue('--safe-area-bottom')`
- Acceptance: non-null value returned (may be `'0px'` in desktop Chrome — that is correct)

**E-7.2** — `viewport-fit=cover` is set in viewport meta tag
- Evaluate: `document.querySelector('meta[name="viewport"]').content`
- Acceptance: content string includes `viewport-fit=cover`

#### Manual Device Check

**M-7.1** — Notch does not clip header (iPhone with notch, landscape)
- Device: iPhone (notch model, landscape orientation)
- Steps: navigate to session list and session detail in landscape; verify header content not behind notch
- Pass: all header text and buttons visible; no clipping
- Fail: any header element partially obscured by notch

**M-7.2** — Home indicator area not occupied by content (iPhone Face ID)
- Device: iPhone (Face ID, portrait)
- Steps: observe bottom nav; verify nav items end above the home indicator region
- Pass: safe-area padding visibly clears the home indicator zone
- Fail: nav items extend into home indicator zone

**M-7.3** — Landscape terminal not clipped by side notch (iPhone)
- Device: iPhone (notch model, landscape, Dynamic Island model acceptable)
- Steps: open terminal session in landscape; rotate device
- Pass: terminal text starts after the notch/island inset on both sides
- Fail: terminal text or keyboard buttons hidden under notch

---

## Milestone 2 — High-Level Test Approach

### SC-M2-1: Full vanilla-extract migration; zero new CSS Module files

**Approach**:
- **CI static check**: `grep -r '\.module\.css' web-app/src/ --include="*.tsx" --include="*.ts"` — count must not increase from the baseline at Milestone 2 start
- **Per-file visual regression**: for each of the 10 CSS Module files migrated (Epic 2.3), capture before/after Playwright screenshots at 1280×800 and 390×844 viewports; diff with `pixelmatch` tolerance ≤0.1%
- **Unit test**: each new `.css.ts` file is imported in a Jest test to verify TypeScript compiles without error (no undefined token references)

### SC-M2-2: Shared UI primitive library (Button, Modal, Card, Input, ActionBar, Badge)

**Approach**:
- **Unit tests per primitive** (React Testing Library): render in all variant/size combinations; assert expected class names are applied; assert ARIA roles and labels
- **Visual regression**: Playwright screenshot tests for each primitive in each variant at both desktop and `390×844` mobile viewport; stored as baseline images in `tests/e2e/visual-baselines/`
- **Accessibility**: run `axe-core` via `@axe-core/playwright` on a Storybook/catalogue page that renders all primitives; zero critical/serious violations required

### SC-M2-3: Complete design token contract via `createTheme`

**Approach**:
- **TypeScript compile check**: `tsc --noEmit` must pass with zero errors in `web-app/src/styles/theme.css.ts` — catch any undefined token references at build time
- **Unit test**: enumerate all keys in `vars` export; assert no key has value `undefined` at runtime (jest snapshot of `vars` shape)
- **No-hardcoded-values lint rule**: add a custom ESLint rule (or grep check in CI) that flags `background: '#...` or `color: 'rgb...'` in `.css.ts` files

### SC-M2-4: Consistent dark/light mode coverage

**Approach**:
- **Visual regression**: Playwright screenshot tests with `colorScheme: 'dark'` and `colorScheme: 'light'` forced via `page.emulateMedia`; all primitives must differ visually between modes (diff > 0 pixels)
- **Manual check**: load app in iOS Safari dark mode and light mode; verify no white-on-white or black-on-black text; check all 6 primitive types

### SC-M2-5: All new primitives ship with unit + visual regression tests

**Approach**:
- **PR gate**: CI pipeline includes a check that any new component file in `web-app/src/components/ui/` must have a corresponding `*.test.tsx` (unit) and a `*.visual.spec.ts` (Playwright)
- **Coverage threshold**: Jest coverage for `components/ui/` must stay ≥80% branch coverage

### SC-M2-6: App fully usable on Pixel 9 Pro Fold (inner and outer screens, all touch targets ≥44dp)

**Approach**:
- **Automated viewport emulation**: add Playwright project for `Galaxy Fold` device (`device: 'Galaxy Fold'` for outer screen; custom `344×882` viewport for inner screen); run existing E2E suite against both viewports
- **Touch target check**: extend E2E-5.x touch target tests to run in fold viewports
- **Manual device check**: full manual walkthrough on Pixel 9 Pro Fold physical device — inner screen (2176×1812), outer screen (1080×2092); verify all session actions, toolbar toggle, and bottom nav clearance on both screens

---

## New Spec Files to Create

| File | Feature Annotation | Tests Defined Above |
|---|---|---|
| `tests/e2e/terminal-toolbar-toggle.spec.ts` | `// @feature ui:terminal-toolbar-toggle` | E-1.1, E-1.2, E-1.3 |
| `tests/e2e/session-header-visibility.spec.ts` | `// @feature ui:session-header` | E-2.1, E-2.2, E-2.3 |
| `tests/e2e/session-actions.spec.ts` | `// @feature ui:session-actions-sheet, session:delete, session:update` | E-3.1, E-3.2, E-3.3, E-3.4, E-3.5 |
| `tests/e2e/bottom-nav-clearance.spec.ts` | `// @feature ui:bottom-nav-clearance` | E-4.1, E-4.2 |
| `tests/e2e/touch-targets.spec.ts` | `// @feature ui:touch-targets` | E-5.1, E-5.2, E-5.3, E-5.4 |
| `tests/e2e/keyboard-layout.spec.ts` | `// @feature ui:keyboard-aware-layout` | E-6.1, E-6.2, E-6.3 |
| `tests/e2e/safe-area-insets.spec.ts` | `// @feature ui:safe-area-insets` | E-7.1, E-7.2 |

Unit tests go in colocated `*.test.tsx` files alongside the component under test in `web-app/src/components/sessions/` and `web-app/src/components/layout/`.

---

## Page Object Extensions

Add to `tests/e2e/pages/SessionsPage.ts` or create `tests/e2e/pages/SessionDetailPage.ts`:

```ts
// SessionDetailPage.ts
export class SessionDetailPage {
  readonly headerTitle: Locator;         // [data-testid="session-header-title"]
  readonly statusBadge: Locator;         // [data-testid="session-status-badge"]
  readonly moreActionsButton: Locator;   // [data-testid="more-actions-button"]
  readonly actionSheet: Locator;         // [data-testid="action-sheet"]
  readonly toolbarToggle: Locator;       // [data-testid="toolbar-toggle"]
  readonly toolbarActions: Locator;      // [data-testid="toolbar-actions"]
  readonly terminalContainer: Locator;  // [data-testid="session-terminal-container"]
  readonly deleteConfirmDialog: Locator; // [data-testid="delete-confirm-dialog"]
}
```

**Required `data-testid` attributes** (must be added during implementation):
- `session-header` — SessionDetail header wrapper
- `session-header-title` — `<h2>` session title
- `session-status-badge` — status badge element
- `more-actions-button` — ⋯ trigger button
- `action-sheet` — bottom sheet modal content wrapper
- `delete-confirm-dialog` — delete confirmation modal
- `toolbar-toggle` — compact/expand toggle button
- `toolbar-actions` — container for collapsible toolbar buttons
- `session-terminal-container` — outer terminal div
- `mobile-key` — each mobile keyboard shortcut button

---

## Manual Device Check Consolidated Checklist

| ID | Criterion | Device | Pass Condition |
|---|---|---|---|
| M-1.1 | Toolbar toggle one tap | iPhone iOS Safari | Single tap collapses/expands; state survives background |
| M-2.1 | Title visible with keyboard | iPhone iOS Safari | Header title visible while keyboard open |
| M-3.1 | Action sheet bottom-sheet on mobile | iPhone iOS Safari | Sheet slides from bottom; all actions tappable |
| M-3.2 | Rename no iOS zoom | iPhone iOS Safari | No viewport zoom on rename input focus |
| M-4.1 | Last card above nav | iPhone iOS Safari | Last session card not clipped by nav |
| M-4.2 | Home indicator cleared | iPhone Face ID | Nav items above home indicator zone |
| M-5.1 | One-handed tap accuracy | iPhone one-handed | Zero mis-taps across 10 trials |
| M-6.1 | Terminal visible with keyboard | iPhone iOS Safari | Terminal + input both visible when keyboard open |
| M-6.2 | No layout jump on keyboard toggle | iPhone iOS Safari | No visible reflow across 3 keyboard cycles |
| M-7.1 | No notch clipping landscape | iPhone notch, landscape | Header fully visible in landscape |
| M-7.2 | Home indicator not occupied | iPhone Face ID portrait | Safe-area padding visually clears indicator |
| M-7.3 | Landscape notch safe-area | iPhone notch landscape | Terminal text clear of notch on both sides |
| M-M2.1 | Dark mode coverage (all primitives) | iPhone, dark mode | No white-on-white or black-on-black text |
| M-M2.2 | Light mode coverage (all primitives) | iPhone, light mode | All primitives legible in light mode |
| M-M2.3 | Fold outer screen usable | Pixel 9 Pro Fold outer | All session actions accessible on outer screen |
| M-M2.4 | Fold inner screen usable | Pixel 9 Pro Fold inner | Full app functional on inner unfolded screen |
| M-M2.5 | Fold touch targets ≥44dp | Pixel 9 Pro Fold | All interactive elements ≥44dp on both screens |
| M-M2.6 | Fold bottom nav clearance | Pixel 9 Pro Fold inner | No content behind bottom nav on either screen |

---

## Constraints and Conventions Enforced

- **No `waitForTimeout`**: all E2E tests use `waitForSelector`, `expect(locator).toBeVisible()`, `page.waitForLoadState()`, or `waitForFunction`
- **Locators**: `data-testid` or ARIA roles only; no CSS class selectors, no nth-child
- **`@feature` annotation**: every spec file starts with `// @feature <id>` on line 1
- **Page Object Model**: all new page helpers in `tests/e2e/pages/`
- **Device checks are manual**: iOS Safari specifics (visualViewport behaviour, safe-area rendering, keyboard interaction) cannot be reliably reproduced in Chromium headless; all iOS Safari criteria are manual

---

## Traceability Matrix

| Success Criterion | Unit | E2E | Visual | Manual | Total |
|---|---|---|---|---|---|
| SC-1: Toolbar toggle + localStorage | U-1.1, U-1.2, U-1.3, U-1.4 | E-1.1, E-1.2, E-1.3 | — | M-1.1 | 8 |
| SC-2: Session name always visible | U-2.1, U-2.2 | E-2.1, E-2.2, E-2.3 | — | M-2.1 | 6 |
| SC-3: Session actions in session view | U-3.1, U-3.2, U-3.3, U-3.4 | E-3.1, E-3.2, E-3.3, E-3.4, E-3.5 | — | M-3.1, M-3.2 | 11 |
| SC-4: Bottom navbar clearance | U-4.1, U-4.2 | E-4.1, E-4.2 | — | M-4.1, M-4.2 | 6 |
| SC-5: 44px touch targets | U-5.1, U-5.2 | E-5.1, E-5.2, E-5.3, E-5.4 | — | M-5.1 | 7 |
| SC-6: dvh/visualViewport keyboard layout | U-6.1, U-6.2, U-6.3 | E-6.1, E-6.2, E-6.3 | — | M-6.1, M-6.2 | 8 |
| SC-7: Safe-area insets | U-7.1, U-7.2, U-7.3 | E-7.1, E-7.2 | — | M-7.1, M-7.2, M-7.3 | 8 |
| SC-M2-1: vanilla-extract migration | CI static | — | 6 VR | — | 6+ |
| SC-M2-2: Primitive library | per-primitive unit | per-primitive E2E | per-primitive VR | M-M2.1, M-M2.2 | approach |
| SC-M2-3: Token contract | tsc + unit | — | — | — | approach |
| SC-M2-4: Dark/light coverage | — | emulateMedia | VR | M-M2.1, M-M2.2 | approach |
| SC-M2-5: Primitives ship with tests | PR gate | — | — | — | approach |
| SC-M2-6: Pixel 9 Pro Fold | — | fold viewports | — | M-M2.3–6 | approach |
| **Totals** | **14** | **22** | **6** | **18** | **60** |
