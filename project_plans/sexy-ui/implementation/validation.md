# Validation Plan: sexy-ui

## Traceability Matrix

| REQ | Acceptance Criterion | Test Type | Test ID | Test Description |
|-----|---------------------|-----------|---------|-----------------|
| REQ-1 | No `#00ff00`, `#00cc00`, or matrix-green hex in source | unit | UT-001 | Source scan: grep theme files for banned color values |
| REQ-1 | `globals.css` bridge vars updated to slate palette | unit | UT-002 | Source scan: assert `--background` is `#0f1117`, `--primary` is `#6366f1` |
| REQ-1 | `cleanTheme` is default on first visit (no localStorage) | e2e | E2E-001 | Navigate to `/` with empty localStorage; assert `background-color` is `rgb(15, 17, 23)` |
| REQ-1 | Accent color is indigo/violet on interactive elements | e2e | E2E-002 | Assert primary button computed `background-color` is `rgb(99, 102, 241)` |
| REQ-1 | Terminal foreground is neutral, not green | e2e | E2E-003 | Assert `--terminal-foreground` computed value is not `rgb(0, 255, 0)` on `/` |
| REQ-1 | FOUC script fallback is `cleanTheme`, no one-frame matrix flash | unit | UT-003 | Source scan: assert `layout.tsx` FOUC script contains `m['clean']` not `m['matrix']` as fallback |
| REQ-1 | No matrix-green remaining | visual-regression | VR-001 | Screenshot diff `session-list-empty.png` under `visual-clean` project |
| REQ-1 | WCAG AA contrast — textMuted on background | accessibility | A11Y-001 | Axe Core on `/` route: 0 critical/serious violations |
| REQ-1 | New `statusDot.*` contract tokens compile in all six themes | unit | UT-004 | Source scan: assert all six `createTheme` calls in `theme.css.ts` contain `statusDot` key |
| REQ-1 | New `transition.*` contract tokens compile in all six themes | unit | UT-005 | Source scan: assert all six `createTheme` calls in `theme.css.ts` contain `transition` key |
| REQ-1 | No hardcoded hex in `SessionDetailView.tsx` inline styles | unit | UT-006 | Source scan: `SessionDetailView.tsx` contains no `style={{ color: 'var(--color-success` inline style |
| REQ-2 | Each session row is 36–40px tall maximum | unit | UT-007 | Source scan: `SessionRow.css.ts` `row` style has `height: "38px"` (or 36–40 range) |
| REQ-2 | `data-testid="session-row"` present on root `<li>` | unit | UT-008 | RTL render `SessionRow`; assert `getByTestId('session-row')` is in document |
| REQ-2 | Single-line row: all metadata on one line, no wrap | e2e | E2E-004 | At 1280×800, assert no `session-row` element has `scrollHeight > offsetHeight` |
| REQ-2 | Hover reveals action icons without layout shift | e2e | E2E-005 | Hover over first `session-row`; assert pause/delete buttons become visible; assert row bounding box height unchanged |
| REQ-2 | 15+ sessions visible at 1080p without scrolling | e2e | E2E-006 | Viewport 1920×1080; create 15 sessions; assert 15 `session-row` elements within viewport without scroll |
| REQ-2 | Group headers are 24px, no heavy dividers | unit | UT-009 | Source scan: `groupHeader` style in `SessionRow.css.ts` has `height: "24px"` and no `border-bottom` property |
| REQ-2 | Status dot colors from `statusDot.*` tokens | unit | UT-010 | RTL render `SessionRow` with `status="running"`; assert status dot element has `data-status="running"` |
| REQ-2 | `prefers-reduced-motion` suppresses pulse animation | unit | UT-011 | Source scan: `SessionRow.css.ts` contains `@media (prefers-reduced-motion: no-preference)` wrapping animation |
| REQ-2 | Compact row is default in `SessionList` | unit | UT-012 | Source scan: `SessionList.tsx` has `viewMode` prop defaulting to `"row"` |
| REQ-2 | `touch-targets.spec.ts` locator updated | e2e | E2E-007 | `touch-targets.spec.ts` uses `[data-testid="session-row"]` (not `[class*="sessionCard"]`) |
| REQ-3 | Single `/settings` route renders with four tabs | e2e | E2E-008 | Navigate to `/settings`; assert tabs General, Config Files, Appearance, Keyboard Shortcuts all visible |
| REQ-3 | `/config` redirects to `/settings?tab=config-files` | e2e | E2E-009 | Navigate to `/config`; assert final URL contains `/settings` and no 404 |
| REQ-3 | `/settings/defaults` redirects to `/settings` | e2e | E2E-010 | Navigate to `/settings/defaults`; assert final URL is `/settings` |
| REQ-3 | Deep-link `?tab=` param selects the correct tab | e2e | E2E-011 | Navigate to `/settings?tab=config-files`; assert Config Files tab panel is active |
| REQ-3 | Tab keyboard navigation works (Left/Right arrows) | e2e | E2E-012 | Focus first tab; press ArrowRight; assert next tab receives focus |
| REQ-3 | No duplicate settings across pages | unit | UT-013 | Source scan: `ThemePicker` rendered only in one location (Appearance tab) — assert no duplicate import sites in route files |
| REQ-4 | Onboarding shown on first visit (no localStorage flag) | unit | UT-014 | RTL render `useOnboarding` hook with empty localStorage; assert `showOnboarding` becomes `true` after `useEffect` fires |
| REQ-4 | Onboarding NOT shown when `stapler-squad:onboarded` is set | unit | UT-015 | RTL render `useOnboarding` with `localStorage.setItem('stapler-squad:onboarded','true')`; assert `showOnboarding` stays `false` |
| REQ-4 | Modal has 4 steps | unit | UT-016 | RTL render `OnboardingModal` open; assert step 1 visible; click Next 3 times; assert step 4 content visible |
| REQ-4 | Skip button present on every step sets the flag and closes | unit | UT-017 | RTL render `OnboardingModal`; click Skip on step 1; assert `onClose` called; assert `localStorage.getItem('stapler-squad:onboarded')` is `'true'` |
| REQ-4 | "Don't show again" checkbox pre-checked; "Get started" sets flag | unit | UT-018 | RTL advance to step 4; assert checkbox checked by default; click "Get started"; assert `localStorage.getItem('stapler-squad:onboarded')` is `'true'` |
| REQ-4 | No SSR hydration mismatch (`useState(false)` + `useEffect`) | unit | UT-019 | Source scan: `useOnboarding.ts` uses `useState(false)` initial value and reads localStorage only inside `useEffect`; assert no direct `localStorage` access outside `useEffect` |
| REQ-4 | Re-triggerable from Settings | e2e | E2E-013 | Navigate to `/settings`; click "Show onboarding tour again"; assert `OnboardingModal` becomes visible |
| REQ-4 | Onboarding first-run | e2e | E2E-014 | Fresh page load with no localStorage; assert `[role="dialog"]` with "One place for all your AI coding sessions" headline becomes visible |
| REQ-5 | `?` key (outside text input) opens cheatsheet | e2e | E2E-015 | Press `?` on `/` not focused in an input; assert `KeyboardShortcutOverlay` dialog becomes visible |
| REQ-5 | Escape closes cheatsheet | e2e | E2E-016 | Open cheatsheet; press Escape; assert overlay is hidden |
| REQ-5 | `⌘?` also opens cheatsheet | e2e | E2E-017 | Press `Meta+Shift+/` on `/`; assert `KeyboardShortcutOverlay` dialog becomes visible |
| REQ-5 | Shortcut list grouped by context in Settings tab | e2e | E2E-018 | Navigate to `/settings?tab=keyboard-shortcuts`; assert headings for Global, Session List, Terminal, Omnibar contexts |
| REQ-5 | Shortcut data comes from single source of truth (`shortcutRegistry`) | unit | UT-020 | Source scan: `KeyboardShortcutOverlay` and Settings Keyboard Shortcuts tab both import from `shortcutRegistry.ts`; assert no duplicate hardcoded shortcut arrays |
| REQ-5 | `omnibar` context added to `ShortcutContext` union | unit | UT-021 | Source scan: `shortcutRegistry.ts` `ShortcutContext` union includes `"omnibar"` |
| REQ-6 | `/help` route returns 200 and renders content | e2e | E2E-019 | Navigate to `/help`; assert HTTP 200 (no error page); assert sidebar nav list visible |
| REQ-6 | Client-side search filters nav in real time | e2e | E2E-020 | Type "omnibar" in search input on `/help`; assert sidebar shows only matching entries; assert no network request fired during typing |
| REQ-6 | At least 6 markdown docs present and render | e2e | E2E-021 | Assert sidebar nav on `/help` has at least 6 items; click each; assert article pane content is non-empty |
| REQ-6 | Docs accessible from Settings | e2e | E2E-022 | Navigate to `/settings`; click "View documentation" link; assert URL becomes `/help` |
| REQ-6 | Docs accessible from onboarding "Learn more" | unit | UT-022 | RTL render `OnboardingModal` on step 1; assert "Learn more" link has `href` containing `/help` |
| REQ-6 | `docLoader.ts` parses title from markdown heading | unit | UT-023 | Call `loadDocs()` with mock markdown content `# My Title\n...`; assert returned `DocEntry.title` is `"My Title"` |
| REQ-6 | Fuse index search returns matching docs | unit | UT-024 | Call `buildFuseIndex(mockDocs)`; call `fuse.search("omnibar")`; assert result contains the omnibar doc entry |
| WCAG AA | Axe Core: main page zero critical/serious violations after palette change | accessibility | A11Y-001 | Axe Core on `/` (existing `accessibility.spec.ts` IT-5.1 — must pass with new palette) |
| WCAG AA | Axe Core: `/settings` zero violations | accessibility | A11Y-002 | Axe Core on `/settings` route |
| WCAG AA | Axe Core: `/help` zero violations | accessibility | A11Y-003 | Axe Core on `/help` route |
| WCAG AA | Axe Core: onboarding modal zero violations | accessibility | A11Y-004 | Open onboarding modal; run Axe with modal open |
| WCAG AA | `textDisabled` not used for informational text | unit | UT-025 | Source scan: grep all `.css.ts` files for `textDisabled`; manually verify each usage is on a genuinely disabled element |
| CSS lint | No undefined CSS variable references | css-lint | CSS-001 | `make lint:css` passes with exit code 0 |
| VR | Visual baseline: `session-list-empty.png` updated | visual-regression | VR-001 | `visual-regression.spec.ts` `session list empty state` passes under `visual-clean` project |
| VR | Visual baseline: `omnibar-open.png` updated | visual-regression | VR-002 | `visual-regression.spec.ts` `omnibar open` passes under `visual-clean` project |

---

## Unit Tests (Jest)

All unit tests run with: `cd web-app && npx jest --no-coverage`

### UT-001: No matrix-green hex in theme files
**File**: `web-app/src/styles/__tests__/themeTokens.test.ts` (new file)
**Asserts**: Read `theme.css.ts` and `globals.css` as strings; assert neither contains `#00ff00`, `#00cc00`, `#00ff33`, or the literal string `matrix-green`. Also assert `cleanTheme` object in `theme.css.ts` does not contain the old `#0f0f11` background value (replaced by `#0f1117`).
```ts
describe('Theme token hygiene', () => {
  it('UT-001: no matrix-green hex in theme.css.ts', () => {
    const src = fs.readFileSync(THEME_FILE, 'utf-8');
    expect(src).not.toMatch(/#00ff00|#00cc00|#00ff33/i);
  });
  it('UT-001: no matrix-green hex in globals.css', () => {
    const src = fs.readFileSync(GLOBALS_FILE, 'utf-8');
    expect(src).not.toMatch(/#00ff00|#00cc00|#00ff33/i);
  });
});
```

### UT-002: `globals.css` palette values
**File**: `web-app/src/styles/__tests__/themeTokens.test.ts` (same file as UT-001)
**Asserts**: Parse `globals.css`; assert `--background` maps to `#0f1117`, `--primary` maps to `#6366f1`, `--text-primary` maps to `#e2e8f0`, `--border-color` maps to `#1e293b`.

### UT-003: FOUC script fallback is `clean`
**File**: `web-app/src/styles/__tests__/themeTokens.test.ts`
**Asserts**: Read `web-app/src/app/layout.tsx` as string; assert it contains `m['clean']` as the fallback in the FOUC script block; assert it does NOT contain `m['matrix']` as the fallback (the literal `'matrix'` as a theme map key lookup in the fallback expression).

### UT-004: All six themes have `statusDot` keys
**File**: `web-app/src/styles/__tests__/themeTokens.test.ts`
**Asserts**: Read `theme.css.ts`; count occurrences of `statusDot:`; assert count is exactly 6 (one per `createTheme` call: `lightTheme`, `darkTheme`, `matrixTheme`, `cyberpunk77Theme`, `wh40kTheme`, `cleanTheme`).

### UT-005: All six themes have `transition` keys
**File**: `web-app/src/styles/__tests__/themeTokens.test.ts`
**Asserts**: Same pattern as UT-004 but for `transition:` top-level key; assert count is 6.

### UT-006: No inline `color: var(--color-success` in `SessionDetailView.tsx`
**File**: `web-app/src/styles/__tests__/themeTokens.test.ts`
**Asserts**: Read `web-app/src/components/sessions/SessionDetailView.tsx`; assert it does NOT contain the string `style={{ color: 'var(--color-success`; assert it does NOT contain `style={{ color: "var(--color-success`.

### UT-007: `SessionRow` CSS height is in 36–40px range
**File**: `web-app/src/components/sessions/__tests__/SessionRow.css.test.ts` (new file)
**Asserts**: Read `SessionRow.css.ts` as a string; assert it contains `height: "38px"` (or a value matching `/height:\s*["']3[6-9]px|40px["']/`). Assert it does NOT contain `height: "auto"` on the row style.

### UT-008: `SessionRow` renders `data-testid="session-row"` on root `<li>`
**File**: `web-app/src/components/sessions/__tests__/SessionRow.test.tsx` (new file)
**Asserts**: RTL render `<SessionRow session={mockSession} onSelect={jest.fn()} />` with a `mockSession` stub (status: "running"). Assert `screen.getByTestId('session-row')` is in the document. Assert it is an `<li>` element.

### UT-009: `groupHeader` style has 24px height and no `border-bottom`
**File**: `web-app/src/components/sessions/__tests__/SessionRow.css.test.ts`
**Asserts**: Read `SessionRow.css.ts` as string; extract the `groupHeader` style block by searching for the identifier; assert it contains `height: "24px"`; assert the block does NOT contain `borderBottom` or `border-bottom`.

### UT-010: Status dot receives correct `data-status` attribute
**File**: `web-app/src/components/sessions/__tests__/SessionRow.test.tsx`
**Asserts**: RTL render `SessionRow` with `session.status = "running"`; find the status dot span (query by its unique class or by being within the `session-row` li and having no text content); assert `data-status` attribute is `"running"`. Repeat for `"paused"` and `"idle"`.

### UT-011: `prefers-reduced-motion` guard in `SessionRow.css.ts`
**File**: `web-app/src/components/sessions/__tests__/SessionRow.css.test.ts`
**Asserts**: Read `SessionRow.css.ts` as string; assert it contains `prefers-reduced-motion: no-preference`; assert the `animationName` or keyframe reference appears only inside or after this media query check (not at the top level of a `style()` call that would always apply the animation).

### UT-012: `SessionList` defaults `viewMode` to `"row"`
**File**: `web-app/src/components/sessions/__tests__/SessionList.viewmode.test.tsx` (new file)
**Asserts**: RTL render `<SessionList sessions={[mockSession]} />` without explicit `viewMode`; assert `screen.getByTestId('session-row')` is in the document (not a card). Also assert rendering `viewMode="card"` renders a card component instead.

### UT-013: `ThemePicker` not duplicated across route pages
**File**: `web-app/src/styles/__tests__/themeTokens.test.ts`
**Asserts**: Use `glob('web-app/src/app/**/page.tsx')` to find all page files; read each; count how many contain `ThemePicker` import; assert count is exactly 1 (only the Appearance tab).

### UT-014: `useOnboarding` shows modal when flag absent
**File**: `web-app/src/components/onboarding/__tests__/useOnboarding.test.ts` (new file)
**Asserts**: Mock `localStorage.getItem` to return `null`. Render `useOnboarding` hook via `renderHook`. Advance fake timers by 1000ms. Assert `result.current.showOnboarding` is `true`.

### UT-015: `useOnboarding` does not show modal when flag set
**File**: `web-app/src/components/onboarding/__tests__/useOnboarding.test.ts`
**Asserts**: Mock `localStorage.getItem('stapler-squad:onboarded')` to return `'true'`. Render hook. Advance timers. Assert `showOnboarding` is `false`.

### UT-016: `OnboardingModal` has 4 steps
**File**: `web-app/src/components/onboarding/__tests__/OnboardingModal.test.tsx` (new file)
**Asserts**: RTL render `<OnboardingModal isOpen={true} onClose={jest.fn()} />`. Assert step 1 headline "One place for all your AI coding sessions" is visible. Click "Next" button. Assert step 2 content visible. Click "Next" twice more. Assert step 4 content (Key shortcuts list) is visible. Assert "Next" button is no longer present on step 4 (replaced by "Get started").

### UT-017: Skip button sets flag and calls `onClose`
**File**: `web-app/src/components/onboarding/__tests__/OnboardingModal.test.tsx`
**Asserts**: RTL render modal open; click the Skip button; assert mock `onClose` was called once; assert `localStorage.getItem('stapler-squad:onboarded')` is `'true'` after click. Test for each of step 1, 2, 3, 4 independently (Skip always visible).

### UT-018: "Don't show again" pre-checked; "Get started" sets flag
**File**: `web-app/src/components/onboarding/__tests__/OnboardingModal.test.tsx`
**Asserts**: Advance to step 4; assert the "Don't show this again" checkbox has `checked` attribute; click "Get started"; assert `localStorage.getItem('stapler-squad:onboarded')` is `'true'`.

### UT-019: `useOnboarding` reads localStorage only in `useEffect`
**File**: `web-app/src/components/onboarding/__tests__/useOnboarding.test.ts`
**Asserts** (source-level): Read `useOnboarding.ts` as string; assert `useState(false)` is the initial state (not `useState(() => localStorage...)`); assert `localStorage` only appears inside a line within an `useEffect` callback block. Also assert the file does NOT contain `typeof window !== 'undefined'` (banned pattern per plan).

### UT-020: Single source of truth for shortcut data
**File**: `web-app/src/lib/shortcuts/__tests__/shortcutRegistry.test.ts` (new file)
**Asserts**: Source scan: `KeyboardShortcutOverlay.tsx` imports from `shortcutRegistry`; Settings Keyboard Shortcuts tab component also imports from `shortcutRegistry`. Assert neither file defines a hardcoded array literal of shortcut objects independent of the registry (grep for `{ key: "`, count occurrences — any array literal with 3+ keyboard shortcut objects is a violation).

### UT-021: `omnibar` context in `ShortcutContext` union
**File**: `web-app/src/lib/shortcuts/__tests__/shortcutRegistry.test.ts`
**Asserts**: Read `shortcutRegistry.ts`; assert it contains the string `"omnibar"` in the `ShortcutContext` type definition.

### UT-022: Onboarding step 1 "Learn more" links to `/help`
**File**: `web-app/src/components/onboarding/__tests__/OnboardingModal.test.tsx`
**Asserts**: RTL render modal open on step 1; find element with text "Learn more"; assert its `href` attribute ends with `/help`.

### UT-023: `docLoader` parses title from `# Heading` line
**File**: `web-app/src/lib/docs/__tests__/docLoader.test.ts` (new file)
**Asserts**: Call `loadDocs()` mocked with `{ 'test.md': '# My Title\n\nBody text.' }`; assert returned array contains `{ title: 'My Title', slug: 'test', content: '# My Title\n\nBody text.' }`. Also test `<!-- title: Explicit Title -->` frontmatter comment parsing if that pattern is used.

### UT-024: Fuse index returns correct search results
**File**: `web-app/src/lib/docs/__tests__/docLoader.test.ts`
**Asserts**: Call `buildFuseIndex([{ slug: 'omnibar', title: 'Omnibar usage', content: 'Press Cmd+K to open the omnibar...' }, { slug: 'config', title: 'Configuration', content: 'Edit config.json' }])`; call `fuse.search('omnibar')`; assert result length >= 1 and first result has `item.slug === 'omnibar'`. Call `fuse.search('configjson')` — assert result does not return the omnibar doc first.

### UT-025: `textDisabled` not used for informational text
**File**: `web-app/src/styles/__tests__/themeTokens.test.ts`
**Asserts**: Glob all `*.css.ts` files; for each file containing `textDisabled`, extract the surrounding style block context; assert it appears only inside style blocks whose name or parent context includes words like `disabled`, `inactive`, `placeholder`, or applies to elements with `disabled` in the selector. This is a structured source-level assertion, not a visual check.

---

## E2E Tests (Playwright)

All E2E tests run against `http://localhost:8544`. Start test server with:
```
STAPLER_SQUAD_USE_CONTROL_MODE=false STAPLER_SQUAD_INSTANCE=e2e-local ./stapler-squad --tmux-keep-server &
```

### E2E-001: Default theme is `clean` (no localStorage)
**File**: `tests/e2e/theme-palette.spec.ts` (new file)
**Feature tag**: `// @feature ui:theme, session:list`
```ts
test('clean theme applied by default with no localStorage', async ({ page }) => {
  await page.addInitScript(() => { localStorage.clear(); });
  await page.goto('/');
  await page.waitForLoadState('domcontentloaded');
  const bg = await page.evaluate(() =>
    window.getComputedStyle(document.documentElement).getPropertyValue('--background').trim()
  );
  // #0f1117 in computed CSS is returned as the hex value from CSS vars
  expect(bg).toBe('#0f1117');
});
```

### E2E-002: Primary button uses indigo accent
**File**: `tests/e2e/theme-palette.spec.ts`
**Asserts**: Find the primary action button (new session or omnibar trigger); compute its `background-color`; assert it is `rgb(99, 102, 241)` (the indigo `#6366f1`).

### E2E-003: Terminal foreground is not matrix green
**File**: `tests/e2e/theme-palette.spec.ts`
**Asserts**: Navigate to `/`; read computed `--terminal-foreground` CSS variable from `:root`; assert it is NOT `rgb(0, 255, 0)` and NOT `#00ff00`.

### E2E-004: Session rows are single-line (no wrapping)
**File**: `tests/e2e/session-row-density.spec.ts` (new file)
**Feature tag**: `// @feature ui:session-list`
**Asserts**: At viewport 1280×800 with at least one session present; for each `[data-testid="session-row"]` element, evaluate `el.scrollHeight <= el.offsetHeight` (single line, no overflow). Assert all rows pass.

### E2E-005: Session row hover reveals actions without layout shift
**File**: `tests/e2e/session-row-density.spec.ts`
**Asserts**: Get bounding box of first `session-row` before hover; hover over the element; get bounding box after hover; assert `height` is identical before and after (no layout shift). Assert at least one of `[data-testid="session-pause-btn"]`, `[data-testid="session-resume-btn"]` or `[data-testid="session-delete-btn"]` becomes visible after hover (opacity check, not display change).

### E2E-006: 15+ session rows visible at 1080p without scrolling
**File**: `tests/e2e/session-row-density.spec.ts`
**Asserts**: At viewport 1920×1080; assert at minimum that 15 `[data-testid="session-row"]` elements have bounding boxes fully within viewport `y < 1080`. (Note: this test is conditional on the test environment having 15+ sessions; mark as `test.skip` with a descriptive reason if the test server has fewer than 15 sessions loaded.)

### E2E-007: `touch-targets.spec.ts` locator uses `data-testid="session-row"`
**File**: `tests/e2e/touch-targets.spec.ts` (modification to existing file)
**Asserts**: The existing file's locator `page.locator('[class*="sessionCard"]')` at line 91 is replaced with `page.locator('[data-testid="session-row"]')`. This is a test file modification, not a new spec — verified by running `npx playwright test touch-targets.spec.ts`.

### E2E-008: `/settings` renders four tabs
**File**: `tests/e2e/settings-consolidation.spec.ts` (new file)
**Feature tag**: `// @feature settings:unified`
**Asserts**: Navigate to `/settings`; assert elements with roles `tab` and names General, Config Files, Appearance, and Keyboard Shortcuts are all visible.

### E2E-009: `/config` redirects to `/settings`
**File**: `tests/e2e/settings-consolidation.spec.ts`
**Asserts**: Navigate to `/config`; await navigation to settle; assert `page.url()` contains `/settings`; assert no 404 error element is visible.

### E2E-010: `/settings/defaults` redirects to `/settings`
**File**: `tests/e2e/settings-consolidation.spec.ts`
**Asserts**: Navigate to `/settings/defaults`; await navigation; assert final URL is exactly `http://localhost:8544/settings`.

### E2E-011: Deep-link `?tab=config-files` activates Config Files tab
**File**: `tests/e2e/settings-consolidation.spec.ts`
**Asserts**: Navigate to `/settings?tab=config-files`; assert the Config Files tab panel is the active (aria-selected) tab; assert content within it (e.g. Monaco editor or CLAUDE.md section heading) is visible.

### E2E-012: Settings tab keyboard navigation (ArrowRight)
**File**: `tests/e2e/settings-consolidation.spec.ts`
**Asserts**: Navigate to `/settings`; click the General tab to focus it; press `ArrowRight`; assert the Config Files tab now has `aria-selected="true"` or is focused. Press ArrowRight again; assert Appearance tab is focused.

### E2E-013: "Show onboarding tour again" re-triggers modal
**File**: `tests/e2e/onboarding.spec.ts` (new file)
**Feature tag**: `// @feature ui:onboarding`
**Asserts**: Set `localStorage.setItem('stapler-squad:onboarded','true')`; navigate to `/settings`; click button with text "Show onboarding tour again"; assert `[role="dialog"]` with onboarding content becomes visible.

### E2E-014: First-run onboarding modal shown on fresh visit
**File**: `tests/e2e/onboarding.spec.ts`
**Asserts**: Clear localStorage; navigate to `/`; wait up to 2000ms; assert `[role="dialog"]` is visible with text "One place for all your AI coding sessions".

### E2E-015: `?` key opens keyboard shortcut cheatsheet
**File**: `tests/e2e/keyboard-shortcuts.spec.ts` (new file)
**Feature tag**: `// @feature ui:keyboard-shortcuts`
**Asserts**: Navigate to `/`; click on a non-input area of the page to ensure body has focus; press `?`; assert `KeyboardShortcutOverlay` dialog is visible (query by `[role="dialog"]` containing a shortcut-related heading).

### E2E-016: Escape closes cheatsheet
**File**: `tests/e2e/keyboard-shortcuts.spec.ts`
**Asserts**: Open cheatsheet via `?`; assert visible; press `Escape`; assert `[role="dialog"]` is no longer visible.

### E2E-017: `Meta+Shift+/` opens cheatsheet
**File**: `tests/e2e/keyboard-shortcuts.spec.ts`
**Asserts**: Navigate to `/`; press `Meta+Shift+/` (macOS `⌘?`); assert cheatsheet dialog visible.

### E2E-018: Keyboard Shortcuts tab shows grouped contexts
**File**: `tests/e2e/keyboard-shortcuts.spec.ts`
**Asserts**: Navigate to `/settings?tab=keyboard-shortcuts`; assert the tab panel is active; assert heading elements for at least Global, Session List, and Terminal contexts are present.

### E2E-019: `/help` route loads and renders sidebar
**File**: `tests/e2e/docs-hub.spec.ts` (new file)
**Feature tag**: `// @feature ui:docs-hub`
**Asserts**: Navigate to `/help`; assert HTTP response is not 404; assert a `<nav>` or list element with documentation entry links is visible; assert the article pane area is non-empty.

### E2E-020: Client-side search filters sidebar without network request
**File**: `tests/e2e/docs-hub.spec.ts`
**Asserts**: Navigate to `/help`; note initial nav link count; start capturing network requests; type "omnibar" in the search input; assert no new XHR/fetch requests were made after typing started; assert nav link count decreased (filtered); assert at least one nav link contains "omnibar" (case-insensitive).

### E2E-021: All 6 docs render content
**File**: `tests/e2e/docs-hub.spec.ts`
**Asserts**: Navigate to `/help`; query all sidebar nav links; assert count >= 6; for each link, click it and assert the article pane heading is non-empty (at minimum an `<h1>` or `<h2>` is visible in the pane).

### E2E-022: Settings "View documentation" link navigates to `/help`
**File**: `tests/e2e/docs-hub.spec.ts`
**Asserts**: Navigate to `/settings`; find link/button with text "View documentation"; click it; assert `page.url()` ends with `/help`.

---

## Visual Regression Tests

### VR-001: Session list empty state — clean theme
**File**: `tests/e2e/visual-regression.spec.ts` (existing file, existing test)
**Baseline**: `tests/e2e/tests/snapshots/chromium/visual-regression.spec.ts/session-list-empty.png`
**Command**: `npx playwright test visual-regression.spec.ts --project=visual-clean`
**Asserts**: After Epic 1 lands, regenerate baseline with `--update-snapshots`. CI compares subsequent runs against this baseline with `maxDiffPixelRatio: 0.01`. No green (`rgb(0, 255, 0)`) pixels should be present in the diff.
**Baseline must be regenerated**: Yes — Epic 1 changes `cleanTheme` colors. Run `--update-snapshots --project=visual-clean` immediately after Epic 1 merges, before any other epic's PR is opened.

### VR-002: Omnibar open — clean theme
**File**: `tests/e2e/visual-regression.spec.ts` (existing file, existing test)
**Baseline**: `tests/e2e/tests/snapshots/chromium/visual-regression.spec.ts/omnibar-open.png`
**Command**: `npx playwright test visual-regression.spec.ts --project=visual-clean`
**Asserts**: Same as VR-001. The omnibar modal should render with the indigo border/focus ring (`#6366f1`), not the old violet-purple. Regenerate alongside VR-001 after Epic 1.
**Baseline must be regenerated**: Yes.

---

## Accessibility Tests

### A11Y-001: Main page — WCAG AA after palette change
**File**: `tests/e2e/accessibility.spec.ts` (existing file, `IT-5.1` test)
**Asserts**: Run Axe Core on `/` with the `cleanTheme` active (default after Epic 1). Assert 0 critical or serious violations. This subsumes the WCAG AA contrast requirement: if `#64748b` on `#0f1117` fails 4.5:1, Axe will report a `color-contrast` violation.

### A11Y-002: `/settings` page — WCAG AA
**File**: `tests/e2e/accessibility.spec.ts` (add new `describe` block to existing file)
**Asserts**: Navigate to `/settings`; run Axe Core excluding terminal elements; assert 0 critical/serious violations. Tab panel switching must not introduce focus traps.

### A11Y-003: `/help` page — WCAG AA
**File**: `tests/e2e/accessibility.spec.ts` (add to existing file)
**Asserts**: Navigate to `/help`; run Axe Core; assert 0 critical/serious violations. The markdown-rendered article content (especially headings hierarchy and link contrast) is in scope.

### A11Y-004: Onboarding modal — WCAG AA
**File**: `tests/e2e/accessibility.spec.ts` (add to existing file)
**Asserts**: Navigate to `/`; wait for onboarding modal to open (set localStorage clear first); run Axe Core with the modal open; assert 0 critical/serious violations. Modal must have `role="dialog"` and `aria-labelledby` pointing to the step headline.

---

## CSS Lint Gate

### CSS-001: `make lint:css` passes with exit code 0
**Command**: `make lint:css`
**When to run**: After every epic that touches `.css.ts` files or `globals.css`.
**Fails on**: Any CSS variable reference (e.g., `var(--color-bg)`) that is not defined in `globals.css`. The plan introduces new bridge variables — confirm each is added to `globals.css` before being referenced in any `.module.css` file.
**New variables requiring addition before use**:
- No new CSS Module variables are introduced by this plan (new components use vanilla-extract `.css.ts` only).
- If any `.module.css` file is edited to reference a new `--` variable, it must be added to `globals.css` first.
**Verification**: `make lint:css` must return exit code 0 as part of `make ci`.

---

## Manual Verification Checklist

These items require human visual or interaction judgment that automated tests cannot fully cover:

- [ ] **REQ-1 visual feel**: Load the app in Chrome. Confirm the overall palette matches the Linear/Vercel aesthetic — slate dark background, neutral text hierarchy, indigo accent. No green anywhere. Terminal foreground is light gray or white.
- [ ] **REQ-1 accent consistency**: Focus a text input; confirm the focus ring is indigo (`#818cf8`), not green or purple. Click a primary button; confirm background is `#6366f1`.
- [ ] **REQ-2 density at 1080p**: Open the app at 1920×1080 with 15+ sessions loaded. Confirm all sessions are visible without any vertical scrollbar.
- [ ] **REQ-2 hover action strip**: Slowly hover over a session row. Confirm the pause/delete icons fade in smoothly without any column jitter or height change. Confirm elapsed time column fades out as actions fade in.
- [ ] **REQ-2 group header appearance**: Enable a grouping strategy (e.g., Category). Confirm group headers are visually slim (24px), uppercase, muted — no heavy divider line below them.
- [ ] **REQ-3 settings navigation flow**: Click Config in the sidebar; confirm redirect to `/settings?tab=config-files`. Click Settings; confirm you land on the General tab. Confirm all four tabs cycle correctly with keyboard arrows.
- [ ] **REQ-4 onboarding flow**: Open incognito; load app. Confirm modal appears after ~800ms. Click through all 4 steps. Confirm Skip on step 2 dismisses and does not show again on reload. Reset via Settings; confirm modal reappears.
- [ ] **REQ-4 illustrations**: Confirm ASCII diagram in step 1 renders in monospace and is legible (not broken by markdown rendering).
- [ ] **REQ-5 cheatsheet legibility**: Press `?`; confirm all shortcut groups are visible, `<Kbd>` components render as styled key badges, Escape closes the overlay.
- [ ] **REQ-6 docs search UX**: Type a partial word (e.g., "tmux") in the `/help` search; confirm results filter immediately; confirm clicking a result scrolls the article pane and displays relevant content.
- [ ] **REQ-1 reduced motion**: Enable "Reduce Motion" in OS accessibility settings; reload app. Confirm status dot does not pulse. Confirm omnibar open/close has no animation.
- [ ] **FOUC check**: Hard-reload the app with DevTools Network throttled to "Slow 3G". Confirm no visible flash of green/matrix theme before the clean theme renders.
- [ ] **`textDisabled` audit**: Search codebase for `textDisabled`; for each usage confirm it only styles genuinely disabled form controls (not labels, metadata, or informational text).

---

## Coverage Summary

- **Total requirements**: 6 (REQ-1 through REQ-6)
- **Requirements fully covered**: 6 (all REQ-1–6 have at least one automated test of every acceptance criterion plus a manual check)
- **Requirements partially covered**: 0
- **Test cases by type**:
  - **unit (Jest)**: 25 (UT-001 through UT-025)
  - **e2e (Playwright)**: 22 (E2E-001 through E2E-022)
  - **visual-regression**: 2 (VR-001, VR-002)
  - **accessibility (Axe Core)**: 4 (A11Y-001 through A11Y-004)
  - **css-lint**: 1 (CSS-001)
  - **Total**: 54

### New test files to create

| File | Tests |
|------|-------|
| `web-app/src/styles/__tests__/themeTokens.test.ts` | UT-001, UT-002, UT-003, UT-004, UT-005, UT-006, UT-013, UT-025 |
| `web-app/src/components/sessions/__tests__/SessionRow.css.test.ts` | UT-007, UT-009, UT-011 |
| `web-app/src/components/sessions/__tests__/SessionRow.test.tsx` | UT-008, UT-010 |
| `web-app/src/components/sessions/__tests__/SessionList.viewmode.test.tsx` | UT-012 |
| `web-app/src/components/onboarding/__tests__/useOnboarding.test.ts` | UT-014, UT-015, UT-019 |
| `web-app/src/components/onboarding/__tests__/OnboardingModal.test.tsx` | UT-016, UT-017, UT-018, UT-022 |
| `web-app/src/lib/shortcuts/__tests__/shortcutRegistry.test.ts` | UT-020, UT-021 |
| `web-app/src/lib/docs/__tests__/docLoader.test.ts` | UT-023, UT-024 |
| `tests/e2e/theme-palette.spec.ts` | E2E-001, E2E-002, E2E-003 |
| `tests/e2e/session-row-density.spec.ts` | E2E-004, E2E-005, E2E-006 |
| `tests/e2e/settings-consolidation.spec.ts` | E2E-008, E2E-009, E2E-010, E2E-011, E2E-012 |
| `tests/e2e/onboarding.spec.ts` | E2E-013, E2E-014 |
| `tests/e2e/keyboard-shortcuts.spec.ts` | E2E-015, E2E-016, E2E-017, E2E-018 |
| `tests/e2e/docs-hub.spec.ts` | E2E-019, E2E-020, E2E-021, E2E-022 |

### Modified existing test files

| File | Change |
|------|--------|
| `tests/e2e/touch-targets.spec.ts` | Line 91 locator → `[data-testid="session-row"]` (E2E-007) |
| `tests/e2e/accessibility.spec.ts` | Add A11Y-002, A11Y-003, A11Y-004 describe blocks |
| `tests/e2e/visual-regression.spec.ts` | No code change — regenerate baselines with `--update-snapshots --project=visual-clean` |
