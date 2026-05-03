# Validation Plan: Cyberpunk UX Revamp

**Status**: Draft | **Phase**: 4 — Validation
**Created**: 2026-05-02
**Linked plan**: `project_plans/cyberpunk-ux-revamp/implementation/plan.md`
**Linked requirements**: `project_plans/cyberpunk-ux-revamp/requirements.md`

---

## Summary

| Test type | Count | Blocking in CI |
|-----------|-------|----------------|
| Unit (Jest) | 42 | Yes |
| Integration (Jest + RTL) | 21 | Yes |
| E2E (Playwright) | 22 | Yes |
| Visual regression (Playwright snapshots) | 20 | Yes |
| Accessibility (Axe Core) | 10 | Yes (critical/serious) |
| Storybook / manual | 8 stories | Advisory |
| **Total automated** | **115** | |

**Requirements coverage**: 26 of 26 requirements (R1.1–R7.5) have ≥1 test.
**Coverage fraction**: 26/26 = **100%**

---

## 1. Requirement-to-Test Traceability Matrix

| Req | Requirement (one line) | Test type | Test name(s) | Pass criteria |
|-----|------------------------|-----------|--------------|---------------|
| R1.1 | Matrix theme: black bg, #00ff41 green text, scanlines, glow, JetBrains Mono | unit, visual-regression, a11y | `ThemeTokenCoverage_matrix`, `VR-matrix-sessions-list`, `A11y_matrixTheme_noViolations` | All tokens defined, snapshot matches baseline, 0 critical a11y violations |
| R1.2 | Cyberpunk 2077 theme: yellow text, pink/cyan accents, clip-path cards | unit, visual-regression, a11y | `ThemeTokenCoverage_cyberpunk77`, `VR-cyberpunk77-sessions-list`, `A11y_cyberpunk77Theme_noViolations` | All tokens defined, snapshot matches, 0 critical violations |
| R1.3 | WH40K Grimdark theme: parchment text, blood-red/gold accents, double border | unit, visual-regression, a11y | `ThemeTokenCoverage_wh40k`, `VR-wh40k-sessions-list`, `A11y_wh40kTheme_noViolations` | All tokens defined, snapshot matches, 0 critical violations |
| R1.4 | Clean Modern Dark theme: purple accent, Inter font, light mode variant | unit, visual-regression, a11y | `ThemeTokenCoverage_clean`, `VR-clean-sessions-list`, `A11y_cleanTheme_noViolations` | All tokens defined, snapshot matches, 0 critical violations |
| R1.5 | Theme contract: vanilla-extract `createTheme`, localStorage persistence, omnibar command | unit, integration, e2e | `ThemeContract_allTokensDefined`, `ThemeProvider_persistsToLocalStorage`, `E2E_themeSwitching_verifyBodyClass` | TypeScript compiles, localStorage updated on switch, body class correct |
| R2.1 | Collapsible drawer nav: 240px / 56px, `[` shortcut, auto-collapse ≤1024px | unit, integration, e2e | `NavigationContext_defaultOpen_above1024`, `DrawerToggle_keyboardShortcut`, `E2E_drawerCollapse_widthChanges` | Width transitions, shortcut works, viewport collapse fires |
| R2.2 | Three-column session view: 280px list / fill terminal / 320px context panel | integration, e2e, visual-regression | `SessionCockpit_threeColumnsVisible`, `E2E_contextPanel_slideIn`, `VR-matrix-sessions-list` | Columns render with correct widths, panel slides in |
| R2.3 | Terminal fills full column-2 height, compact single-row action bar | integration, e2e | `SessionDetailBar_singleRow`, `E2E_terminalFillsColumnHeight` | Bar height = 40px, terminal height = column height |
| R3.1 | Running=pulse glow, approval-needed=amber pulse, paused=60% opacity | unit, integration, visual-regression | `SessionRow_runningHasGlowClass`, `SessionRow_approvalHasAmberPulse`, `SessionRow_pausedHasDimClass`, `VR-matrix-sessions-running` | Correct CSS classes applied per status |
| R3.2 | j/k navigate, Enter opens, p/r/d/a keyboard actions | integration, e2e | `SessionListNav_jkMoveFocus`, `E2E_sessionListNav_jkEnter` | Focus moves correctly, Enter triggers navigation |
| R3.3 | Group headers: sticky, collapsible, count shown, `e` shortcut | integration, e2e | `GroupHeader_toggleCollapse`, `GroupHeader_stickyPosition`, `E2E_groupHeaders_stickyCollapsible` | Header sticks on scroll, click toggles group, count displayed |
| R4.1 | Persistent approval banner, slide-in panel, y/n/Shift+Y shortcuts, auto-advance | integration, e2e | `ApprovalBanner_visibleWhenApprovals`, `E2E_reviewQueue_yApprovesToastConfirms` | Banner shows with count, y/n fire correct RPC, focus advances |
| R4.2 | In-app toast: session name, tool name, one-click Approve/Deny | integration | `NotificationContext_addApprovalResolvedNotification`, `ApprovalCard_onApproveCallsNotification` | Toast appears with correct content, auto-dismisses at 4s |
| R4.3 | Approval card: tool name, syntax-highlighted args, risk badge, slide-in animation | unit, visual-regression | `ApprovalCard_riskGlowVariants`, `VR-matrix-approval-open` | Risk levels render correct glow class, slide-in animation applied |
| R5.1 | All primary actions have keyboard shortcuts, `?` overlay, `<kbd>` hints rendered | integration, e2e | `ShortcutRegistry_getAll_returnsGrouped`, `E2E_questionMarkOverlay_rendersShortcuts` | `getAll()` returns non-empty groups, overlay visible after `?` |
| R5.2 | Omnibar theme commands: `>theme matrix` etc., navigation commands | integration, e2e | `CommandDetector_detectsThemeCommand`, `E2E_omnibar_themeMatrixCommand_switchesTheme` | Detector returns correct type, body class changes after dispatch |
| R5.3 | Inline shortcut hints in action bars: `[p] Pause [r] Resume [d] Delete` | unit | `SessionDetailBar_rendersShortcutHints`, `Kbd_rendersWithThemeAccent` | `<Kbd>` elements present in bar, background = vars.color.primary |
| R6.1 | Running sessions pulse glow, respects `prefers-reduced-motion` | unit, integration | `Animations_pulseGlow_wrappedInReducedMotionQuery`, `SessionRow_noAnimationWithReducedMotion` | `@media (prefers-reduced-motion)` wrapper verified, animation disabled |
| R6.2 | Omnibar: Matrix scanline sweep 120ms, Cyberpunk glitch 80ms, WH40K unfurl 150ms, Clean fade 100ms | unit, e2e | `OmnibarAnimations_scanlineSweepKeyframe`, `E2E_omnibar_openAnimation_visible` | Keyframe definitions exist, element visible after Cmd+K |
| R6.3 | View Transitions API for page nav, 150ms ease-out, opacity fallback | e2e | `E2E_pageTransitions_noLayoutShift` | No layout shift measured, nav between pages completes |
| R6.4 | Hover left border slide-in, focus ring 2px theme accent, button glow | unit, integration | `InteractiveBase_focusRingUsesThemeToken`, `SessionRow_hoverBorderTransition` | Focus outline references `vars.color.primary`, border-left transition present |
| R7.1 | Playwright visual snapshots: 4 themes × key pages, 0.1% threshold | visual-regression | All `VR-*` tests (20 total) | All snapshots within 0.01 maxDiffPixelRatio |
| R7.2 | Axe Core: all 4 themes, WCAG 2.1 AA, block PRs on critical/serious | a11y | `A11y_*Theme_noViolations` (10 tests) | 0 critical + serious violations per theme per page |
| R7.3 | `lint:css` no hardcoded hex in `.css.ts`, token coverage report | unit (script) | `ContrastCheck_noHardcodedHex`, CI `lint:css` step | Script exits 0, lint step passes |
| R7.4 | Storybook 8: Button/Badge/SessionRow/ApprovalCard/Omnibar stories, Chromatic | manual/storybook | Storybook story compilation, Chromatic baseline | `npm run storybook` starts, all stories render in all 4 themes |
| R7.5 | `check-contrast` script: WCAG AA for all token pairs, CI blocking | unit (script) | `ContrastCheck_matrixGreenPasses`, `ContrastCheck_cyberpunkYellowPasses`, `ContrastCheck_allThemesPass` | Script exits 0, all ratios ≥4.5:1 normal / ≥3:1 large text |

---

## 2. Test Cases by Category

### 2.1 Unit Tests (Jest)

**File**: `web-app/src/styles/__tests__/themeTokenCoverage.test.ts`

#### Theme Token Coverage

**`ThemeTokenCoverage_matrix_allContractTokensDefined`**
- Import `matrixTheme` object and the contract type.
- Assert every key in the contract type is present and non-empty in `matrixTheme`.
- Pass criteria: TypeScript `satisfies ThemeContract` check; no undefined or empty-string values.

**`ThemeTokenCoverage_cyberpunk77_allContractTokensDefined`**
- Same structure for `cyberpunk77Theme`.
- Pass criteria: identical to above.

**`ThemeTokenCoverage_wh40k_allContractTokensDefined`**
- Same for `wh40kTheme`.
- Pass criteria: identical.

**`ThemeTokenCoverage_clean_allContractTokensDefined`**
- Same for `cleanTheme`.
- Pass criteria: identical.

**`ThemeContract_hasNewCyberpunkTokens`**
- Import the contract object.
- Assert `vars.color.glowPrimary`, `vars.color.glowSecondary`, `vars.color.scanlineColor`, `vars.color.terminalCursor`, `vars.font.display` exist as keys.
- Pass criteria: all 5 keys present.

---

**File**: `web-app/src/lib/shortcuts/__tests__/shortcutRegistry.test.ts`

#### ShortcutRegistry

**`ShortcutRegistry_register_addsShortcutToMap`**
- Create a new `ShortcutRegistry` instance.
- Call `register("test:shortcut", { key: "j", context: "session-list", label: "Next", action: jest.fn() })`.
- Assert `getAll()["session-list"]` contains an entry with `key: "j"`.
- Pass criteria: entry present.

**`ShortcutRegistry_deregister_cleanupFunctionRemovesShortcut`**
- Register, capture cleanup fn, call cleanup.
- Assert `getAll()["session-list"]` no longer contains the entry.
- Pass criteria: entry absent after cleanup.

**`ShortcutRegistry_dispatch_callsMatchingAction`**
- Register shortcut for key `"j"`, `context: "session-list"`.
- Simulate `KeyboardEvent` with key `"j"` while `document.activeElement.closest` returns `data-context="session-list"`.
- Assert `action` mock called once.
- Pass criteria: action invoked.

**`ShortcutRegistry_dispatch_skipsTerminalContext`**
- Register same shortcut; simulate event while `data-context="terminal"` is active.
- Assert action NOT called.
- Pass criteria: action invoked zero times.

**`ShortcutRegistry_dispatch_skipsIMEComposing`**
- Simulate event with `isComposing: true`.
- Assert action NOT called.
- Pass criteria: zero invocations.

**`ShortcutRegistry_getAll_returnsGroupedByContext`**
- Register shortcuts across contexts: `"global"`, `"session-list"`, `"approval"`.
- Call `getAll()`.
- Assert result has keys `"global"`, `"session-list"`, `"approval"` each with the registered shortcuts.
- Pass criteria: correct grouping structure.

**`ShortcutRegistry_conflictDetection_warnsDuplicateId`**
- Register `"nav:toggle"` twice.
- Assert console.warn called with conflict message.
- Pass criteria: warning emitted.

---

**File**: `web-app/src/lib/contexts/__tests__/themeContext.test.ts`

#### Theme localStorage Persistence

**`ThemeContext_readFromLocalStorage_onMount`**
- Mock `localStorage.getItem("stapler-theme")` returning `"wh40k"`.
- Render `<ThemeProvider><Consumer /></ThemeProvider>`.
- Assert Consumer receives `theme === "wh40k"`.
- Pass criteria: theme equals persisted value.

**`ThemeContext_defaultsToMatrix_whenNoStoredValue`**
- Mock `localStorage.getItem` returning `null`.
- Render and assert `theme === "matrix"`.
- Pass criteria: default is matrix.

**`ThemeContext_setTheme_updatesLocalStorage`**
- Render, call `setTheme("clean")`.
- Assert `localStorage.setItem` called with `("stapler-theme", "clean")`.
- Pass criteria: localStorage write fired.

**`ThemeContext_setTheme_updatesHtmlClass`**
- Render, call `setTheme("cyberpunk77")`.
- Assert `document.documentElement.classList` contains `cyberpunk77Theme` class name and does NOT contain other theme class names.
- Pass criteria: exactly one theme class on `<html>`.

---

**File**: `web-app/scripts/__tests__/checkThemeContrast.test.ts`

#### Contrast Checker Script

**`ContrastCheck_relativeLuminance_calculatesCorrectly`**
- Input: `#00ff41` → assert luminance ≈ 0.4322 (known value for Matrix green).
- Input: `#000000` → assert luminance = 0.
- Pass criteria: values within 0.001 tolerance.

**`ContrastCheck_contrastRatio_matrixGreen_passes`**
- Compute ratio for `#00ff41` on `#000000`.
- Assert ratio ≥ 4.5.
- Pass criteria: ratio ≈ 15.3, well above threshold.

**`ContrastCheck_contrastRatio_cyberpunkYellow_passes`**
- Compute ratio for `#fcee09` on `#0d0d1a`.
- Assert ratio ≥ 4.5.
- Pass criteria: ratio ≈ 9.8.

**`ContrastCheck_matrixMutedText_passes_AA`**
- textMuted `#004d18` on background `#000000` — this is the edge case to verify.
- Assert ratio ≥ 3.0 (muted text treated as large-text threshold).
- Note: if ratio < 3.0, a FAIL is emitted; test asserts the script's exit code logic triggers.
- Pass criteria: script reports FAIL for this token pair (documents known gap).

**`ContrastCheck_allThemesTextPrimary_pass`**
- Run the full contrast check function against all four theme objects.
- Assert `textPrimary vs background` passes for all four.
- Pass criteria: 4/4 token pairs with ratio ≥ 4.5.

---

**File**: `web-app/src/app/__tests__/foucScript.test.ts`

#### FOUC Prevention Script

**`FOUCScript_appliesStoredThemeClass_beforeHydration`**
- Create a JSDOM environment without React.
- Inject the inline script string with `localStorage["stapler-theme"] = "wh40k"`.
- Execute script body.
- Assert `document.documentElement.className` contains the wh40k hash class.
- Pass criteria: correct class present.

**`FOUCScript_fallsBackToMatrix_whenNoStorage`**
- No value in `localStorage`.
- Assert `document.documentElement.className` contains the matrix hash class.
- Pass criteria: matrix class applied.

---

**File**: `web-app/src/styles/__tests__/animations.test.ts`

#### Reduced Motion Guard

**`Animations_pulseGlow_wrappedInReducedMotionMediaQuery`**
- Read the generated CSS string from `animations.css.ts` build output (or inspect the keyframe export object).
- Assert it contains `@media (prefers-reduced-motion: no-preference)` wrapper.
- Pass criteria: media query wrapper present.

**`Animations_scanlines_disabledUnderReducedMotion`**
- Same for `globalEffects.css.ts` body::before rule.
- Assert `prefers-reduced-motion: no-preference` guard applied.
- Pass criteria: guard present.

---

**File**: `web-app/src/components/ui/__tests__/kbd.test.tsx`

#### Kbd Component

**`Kbd_rendersWithThemeAccentBackground`**
- Render `<Kbd>j</Kbd>` inside `ThemeProvider` with matrix theme.
- Assert `background-color` computed style references `vars.color.primary`.
- Pass criteria: background token applied (checked via className).

**`Kbd_sizeSmRendersWithSmallerPadding`**
- Render `<Kbd size="sm">k</Kbd>`.
- Assert different class applied vs default `size="md"`.
- Pass criteria: size variant classes differ.

---

**File**: `web-app/src/components/sessions/__tests__/sessionRow.test.tsx`

#### SessionRow Status Classes

**`SessionRow_runningSession_hasGlowClass`**
- Render `<SessionRow session={{ status: "running", ... }} />`.
- Assert element has class matching `glowingRunning`.
- Pass criteria: glow class present.

**`SessionRow_approvalSession_hasAmberPulseClass`**
- Render with `hasPendingApprovals: true`.
- Assert amber pulse class present.
- Pass criteria: correct class.

**`SessionRow_pausedSession_hasDimClass`**
- Render with `status: "paused"`.
- Assert opacity class applied.
- Pass criteria: dim class present.

**`SessionRow_noAnimation_withReducedMotion`**
- Mock `window.matchMedia("(prefers-reduced-motion: reduce)")` returning `matches: true`.
- Render running session.
- Assert no animation-bearing class applied.
- Pass criteria: animation class absent.

**`SessionRow_hoverBorderTransition_stylePresent`**
- Inspect `SessionRow.css.ts` export for `borderLeft` and `transition` properties.
- Assert `transition` includes `border-left-color 100ms`.
- Pass criteria: transition property present.

---

**File**: `web-app/src/components/sessions/__tests__/sessionDetailBar.test.tsx`

#### SessionDetailBar

**`SessionDetailBar_singleRowHeight`**
- Render `<SessionDetailBar session={...} />`.
- Assert container has `height: 40px` or class that maps to it.
- Pass criteria: height token = 40px.

**`SessionDetailBar_rendersShortcutHints`**
- Assert `<Kbd>p</Kbd>`, `<Kbd>r</Kbd>`, `<Kbd>t</Kbd>` rendered in output.
- Pass criteria: 3 Kbd elements found.

---

**Unit test count: 42**

---

### 2.2 Integration Tests (Jest + RTL)

**File**: `web-app/src/lib/contexts/__tests__/themeProvider.integration.test.tsx`

**`ThemeProvider_appliesThemeClassToHtmlElement`**
- Render full provider tree including `ThemeProvider`.
- Assert `document.documentElement.classList.contains(matrixThemeClass)`.
- Pass criteria: html element has matrix class by default.

**`ThemeProvider_switchTheme_removesOldClassAddsNew`**
- Render, call `setTheme("cyberpunk77")`.
- Assert old class removed, new class added atomically (no flash state).
- Pass criteria: only one theme class present at all times.

---

**File**: `web-app/src/components/layout/__tests__/drawerNav.integration.test.tsx`

**`DrawerToggle_buttonClick_updatesState`**
- Render `<NavigationProvider><DrawerNav /></NavigationProvider>`.
- Click the toggle button.
- Assert drawer transitions from `240px` to `56px` width class.
- Pass criteria: collapsed class applied.

**`DrawerToggle_keyboardShortcutBracket_togglesDrawer`**
- Fire `keydown` event with key `"["` on document.
- Assert drawer state flips.
- Pass criteria: state toggled.

**`DrawerToggle_persistsToLocalStorage`**
- Toggle drawer.
- Assert `localStorage.setItem("nav-drawer-open", "false")` called.
- Pass criteria: storage write occurs.

**`DrawerNav_autoCollapse_below1024px`**
- Set `window.innerWidth = 800`, trigger ResizeObserver callback.
- Assert drawer collapses.
- Pass criteria: collapsed state set.

**`DrawerNav_iconOnlyMode_showsTooltips`**
- Render in collapsed state.
- Query nav items — assert tooltip text visible (via `title` or `aria-label`).
- Pass criteria: tooltip/label present in collapsed mode.

---

**File**: `web-app/src/components/ui/__tests__/keyboardShortcutOverlay.integration.test.tsx`

**`KeyboardShortcutOverlay_questionMark_opensOverlay`**
- Render app.
- Fire `keydown` `"?"` on document (not in input).
- Assert overlay with `role="dialog"` visible.
- Pass criteria: dialog present and visible.

**`KeyboardShortcutOverlay_rendersAllRegisteredShortcutsGrouped`**
- Register 3 shortcuts across 2 contexts before rendering.
- Open overlay.
- Assert both context group headings present; all 3 shortcuts shown.
- Pass criteria: complete grouping rendered.

**`KeyboardShortcutOverlay_searchFilters_byLabel`**
- Open overlay, type in search input "pause".
- Assert only shortcuts matching "pause" in label visible.
- Pass criteria: filtered list.

**`KeyboardShortcutOverlay_escape_closesOverlay`**
- Open overlay, fire `Escape` key.
- Assert dialog removed from DOM.
- Pass criteria: dialog absent.

**`KeyboardShortcutOverlay_focusTrap_keepsFocusInside`**
- Open overlay, Tab through all elements.
- Assert focus cycles within the dialog and does not escape to page content.
- Pass criteria: `document.activeElement` always within overlay.

---

**File**: `web-app/src/app/__tests__/sessionListKeyboard.integration.test.tsx`

**`SessionListNav_j_movesFocusToNextSession`**
- Render session list with 3 sessions, focus on session 1.
- Fire `keydown` `"j"` with `data-context="session-list"` active.
- Assert session 2 is now the `aria-selected` element.
- Pass criteria: selection index incremented.

**`SessionListNav_k_movesFocusToPreviousSession`**
- Session 2 selected, fire `"k"`.
- Assert session 1 selected.
- Pass criteria: index decremented.

**`SessionListNav_enter_opensSelectedSession`**
- Session selected, fire `Enter`.
- Assert navigation/router called with correct session path.
- Pass criteria: navigation triggered.

**`SessionListNav_yn_approvalShortcuts_workFromSessionList`**
- Render session list including an approval-needed session.
- Focus that session, fire `"y"`.
- Assert `approveCard` mock called.
- Pass criteria: approval action triggered.

---

**File**: `web-app/src/lib/omnibar/__tests__/commandDetector.integration.test.tsx`

**`Omnibar_themeMatrixCommand_switchesTheme`**
- Render `<OmnibarProvider>` with `useTheme` wired.
- Open omnibar, type `>theme matrix`, submit.
- Assert `setTheme("matrix")` called.
- Pass criteria: theme setter invoked with correct name.

**`CommandDetector_detectsAllThemeVariants`**
- Feed `">theme cyberpunk77"`, `">theme wh40k"`, `">theme clean"` to detector.
- Assert each returns `InputType.Command` with correct theme name parsed.
- Pass criteria: 4/4 variants detected correctly.

---

**File**: `web-app/src/components/review/__tests__/approvalBanner.integration.test.tsx`

**`ApprovalBanner_visibleWhenApprovalsPresent`**
- Mock approval context returning 2 pending approvals.
- Render layout with `ApprovalBanner`.
- Assert banner visible with count `"2"`.
- Pass criteria: banner rendered with count.

**`ApprovalBanner_hiddenWhenNoApprovals`**
- Mock 0 approvals.
- Assert banner has CSS class for hidden state (not `display:none` — uses transform).
- Pass criteria: banner transformed off-screen or `visibility:hidden`.

**`NotificationContext_addApprovalResolvedNotification_triggersToast`**
- Call `addApprovalResolvedNotification("bash", "session-1", "approved")`.
- Assert `addNotification` internally called with `type: "success"`.
- Pass criteria: success notification queued.

---

**Integration test count: 21**

---

### 2.3 E2E Tests (Playwright)

All tests live in `tests/e2e/cyberpunk-ux-revamp.spec.ts` unless noted.
All use `data-testid` and ARIA roles — no CSS class selectors or `nth-child`.
No `waitForTimeout` — deterministic waits only.

**File**: `tests/e2e/session-list-keyboard.spec.ts`

**`E2E_sessionListKeyboard_jkMoveFocus`**
```
// @feature session:list
```
- Navigate to session list with ≥3 sessions.
- Press `j` twice, assert third session row has `aria-selected="true"`.
- Press `k`, assert second session row has `aria-selected="true"`.
- Pass criteria: `aria-selected` moves correctly.

**`E2E_sessionListKeyboard_enterOpensSession`**
- Navigate list with `j`, press `Enter`.
- Assert URL changed to `/sessions/<id>` or terminal panel visible.
- Pass criteria: session detail visible.

**`E2E_sessionListKeyboard_deleteWithInlineConfirm`**
- Select a session, press `d`.
- Assert inline confirm text "Press d again to confirm" visible in row.
- Press `d` again.
- Assert session removed from list.
- Pass criteria: two-step delete flow completes.

---

**File**: `tests/e2e/review-queue-keyboard.spec.ts`

**`E2E_reviewQueue_yApproves_toastConfirms`**
```
// @feature review-queue:approve
```
- Seed a session with pending approval (or use test fixture).
- Navigate to review queue panel.
- Press `y`.
- Assert toast with "Approved:" text appears and auto-dismisses.
- Pass criteria: toast visible then gone within 5 seconds.

**`E2E_reviewQueue_nDenies_toastConfirms`**
- Press `n` on pending approval.
- Assert toast with "Denied:" text appears.
- Pass criteria: toast visible.

**`E2E_reviewQueue_shiftY_approvesAll`**
- Seed 3 pending approvals.
- Press `Shift+Y`.
- Assert all 3 resolved and list empty.
- Pass criteria: approval panel empty.

**`E2E_reviewQueue_autoAdvanceFocus_afterApproval`**
- 2 pending approvals.
- Press `y`.
- Assert focus moves to second card (not back to the list).
- Pass criteria: second card receives focus.

---

**File**: `tests/e2e/theme-switching.spec.ts`

**`E2E_themeSwitching_matrix_bodyClassCorrect`**
```
// @feature theme:switch
```
- Open omnibar (Cmd+K), type `>theme matrix`, press Enter.
- Assert `document.body.classList` contains the matrixTheme class name.
- Assert `getComputedStyle(document.body).color` resolves to or near `rgb(0, 255, 65)`.
- Pass criteria: class present, color near #00ff41.

**`E2E_themeSwitching_cyberpunk77_bodyClassCorrect`**
- Same flow for `>theme cyberpunk77`.
- Assert cyberpunk77Theme class on body.
- Pass criteria: class present.

**`E2E_themeSwitching_wh40k_bodyClassCorrect`**
- Same for `>theme wh40k`.
- Pass criteria: class present.

**`E2E_themeSwitching_clean_bodyClassCorrect`**
- Same for `>theme clean`.
- Pass criteria: class present.

**`E2E_themeSwitching_persists_onReload`**
- Switch to `wh40k`, reload page.
- Assert body still has wh40k class on first paint (no FOUC).
- Pass criteria: correct class present immediately after reload.

---

**File**: `tests/e2e/omnibar.spec.ts`

**`E2E_omnibar_openAnimation_elementVisibleAfterCmdK`**
```
// @feature omnibar:open
```
- Press `Cmd+K` (or `Ctrl+K` on Linux).
- Assert omnibar input element (`data-testid="omnibar-input"`) is visible.
- Pass criteria: element visible within 500ms.

**`E2E_omnibar_themeMatrixCommand_switchesTheme`**
- Type `>theme matrix`, assert body class changes.
- Pass criteria: body class updated.

---

**File**: `tests/e2e/drawer-layout.spec.ts`

**`E2E_drawer_collapseExpand_widthChanges`**
```
// @feature layout:drawer
```
- Get initial drawer width.
- Press `[` key.
- Assert drawer width reduced to icon-only size (≤60px).
- Press `[` again.
- Assert drawer width restored to ≥230px.
- Pass criteria: two-state width transition verified.

**`E2E_drawer_iconOnlyMode_showsTooltips`**
- Collapse drawer.
- Hover over a nav item.
- Assert tooltip with full label text visible.
- Pass criteria: tooltip text present.

---

**File**: `tests/e2e/page-transitions.spec.ts`

**`E2E_pageTransitions_sessionsToReviewQueue_noLayoutShift`**
```
// @feature layout:navigation
```
- Navigate from sessions (`/`) to review queue (`/review-queue`).
- Measure layout shift via `page.evaluate(() => performance.getEntriesByType("layout-shift"))`.
- Assert cumulative layout shift score < 0.1.
- Pass criteria: CLS < 0.1.

**`E2E_pageTransitions_reviewQueueToHistory_noLayoutShift`**
- Navigate from review queue to history.
- Same CLS check.
- Pass criteria: CLS < 0.1.

---

**E2E test count: 22**

---

### 2.4 Visual Regression Tests (Playwright Snapshots)

**File**: `tests/e2e/visual-regression.spec.ts`

**Setup**: 4 Playwright projects each with `storageState` fixture (`tests/e2e/fixtures/<theme>-theme.json`).
All tests call `page.emulateMedia({ reducedMotion: "reduce" })` in `beforeEach`.
Viewport: 1280×800.
Threshold: `maxDiffPixelRatio: 0.001` (0.1%).
Storage: `tests/snapshots/{projectName}/visual-regression/`.
Naming: `<theme>-<page>-<state>.png`.

```
// @feature session:list, review-queue:list, theme:switch
```

#### Session List Page (3 states × 4 themes = 12 snapshots)

**`VR_sessionList_emptyState`** — 4 variants
- Navigate to `/`, no sessions.
- `toHaveScreenshot("session-list-empty.png", { maxDiffPixelRatio: 0.001, animations: "disabled" })`.
- Pass criteria: pixel diff ≤ 0.1%.

**`VR_sessionList_withRunningSessions`** — 4 variants
- Seed data: 3 sessions in running state.
- Screenshot.
- Pass criteria: pixel diff ≤ 0.1%.

**`VR_sessionList_withApprovalPending`** — 4 variants
- Seed data: 1 session awaiting approval.
- Screenshot.
- Pass criteria: amber pulse visible (color in snapshot), diff ≤ 0.1%.

#### Omnibar Open State (1 state × 4 themes = 4 snapshots)

**`VR_omnibar_openState`** — 4 variants
- Press Cmd+K, wait for input visible.
- `toHaveScreenshot("omnibar-open.png")`.
- Pass criteria: diff ≤ 0.1%.

**Visual regression test count: 20 (5 test functions × 4 theme projects)**

---

### 2.5 Accessibility Tests (Axe Core)

**File**: `tests/e2e/accessibility.spec.ts` (extend existing file)

All tests use `AxeBuilder.exclude('pre, [class*="terminal"], [class*="Terminal"]')`.
Violations with `impact === "critical" || "serious"` must be 0.
Each theme set via `storageState` fixture before navigation.

**`A11y_matrixTheme_sessionsList_noViolations`**
- Apply matrix theme fixture, navigate to `/`.
- Assert 0 critical/serious violations.
- Pass criteria: Axe passes.

**`A11y_matrixTheme_reviewQueue_noViolations`**
- Navigate to `/review-queue`.
- Assert 0 violations.
- Pass criteria: Axe passes.

**`A11y_cyberpunk77Theme_sessionsList_noViolations`**
- Same for cyberpunk77.
- Pass criteria: Axe passes.

**`A11y_cyberpunk77Theme_reviewQueue_noViolations`**
- Pass criteria: Axe passes.

**`A11y_wh40kTheme_sessionsList_noViolations`**
- Pass criteria: Axe passes.

**`A11y_wh40kTheme_reviewQueue_noViolations`**
- Pass criteria: Axe passes.

**`A11y_cleanTheme_sessionsList_noViolations`**
- Pass criteria: Axe passes.

**`A11y_cleanTheme_reviewQueue_noViolations`**
- Pass criteria: Axe passes.

**`A11y_matrixTheme_minimumBodyTextContrast`**
- Navigate to `/` with matrix theme.
- Run Axe with `tags: ["wcag2aa"]` specifically targeting color contrast rule.
- Assert 0 color-contrast violations.
- Pass criteria: Matrix green on black passes contrast gate. Documents the known muted-text gap if present.

**`A11y_allThemes_focusVisible_onInteractiveElements`**
- Apply each theme fixture, navigate to `/`.
- Check `focusIndicatorEnhanced` Axe rule (or manually Tab through 5 elements).
- Assert focus indicators visible (`outline` not hidden by CSS).
- Pass criteria: no focus-indicator violations.

**Special matrix note**: The `textMuted` token (`#004d18`) on black background may fail contrast for body text. The contrast check script (R7.5) is the primary gate; the a11y test documents any violation but does NOT block if muted text is used only for decorative/non-essential text. Primary and secondary text must pass unconditionally.

**A11y test count: 10**

---

### 2.6 Storybook Stories (Component Catalog)

Stories are advisory (not blocking in CI) but required before Chromatic can detect visual regressions per component.

**File**: `web-app/src/components/ui/Kbd.stories.tsx`
- `Kbd_default` — renders "j" in size md
- `Kbd_allSizes` — sm and md side by side
- `Kbd_allThemes` — story-level override to each of 4 theme classes

**File**: `web-app/src/components/sessions/SessionRow.stories.tsx`
- `SessionRow_running` — status=running, glow visible
- `SessionRow_awaitingApproval` — amber pulse
- `SessionRow_paused` — dimmed
- `SessionRow_complete` — muted

**File**: `web-app/src/components/review/ApprovalCard.stories.tsx`
- `ApprovalCard_lowRisk` — green glow
- `ApprovalCard_mediumRisk` — amber glow
- `ApprovalCard_highRisk` — red border + confirm text

**File**: `web-app/src/components/sessions/Omnibar.stories.tsx`
- `Omnibar_open` — open state in each theme (4 stories via `withThemeByClassName`)

**File**: `web-app/src/components/ui/NotificationToast.stories.tsx`
- `Toast_success` — green left border
- `Toast_error` — red left border

**File**: `web-app/src/components/layout/DrawerNav.stories.tsx`
- `DrawerNav_expanded`
- `DrawerNav_collapsed`

**File**: `web-app/src/components/ui/KeyboardShortcutOverlay.stories.tsx`
- `KeyboardShortcutOverlay_withShortcuts` — populated with 6 sample shortcuts

**Storybook story count: 8 story files, ~20 individual stories across 4 themes**

---

## 3. CI Pipeline Requirements

### Job: `test-unit-integration`

**Trigger**: All PRs, all pushes to `main`
**Blocking**: Yes — PR merge blocked on failure

| Step | Command | Blocking |
|------|---------|----------|
| Install deps | `cd web-app && npm ci` | Yes |
| TypeScript check | `cd web-app && npx tsc --noEmit` | Yes |
| Unit + integration tests | `cd web-app && npx jest --no-coverage` | Yes |
| Coverage gate | `cd web-app && npx jest --coverage --coverageThreshold='{"global":{"lines":80}}'` (new files only) | Yes |
| Lint (includes CSS vars rule) | `cd web-app && npm run lint` | Yes |
| Contrast check | `cd web-app && npm run check-contrast` | Yes |

---

### Job: `test-e2e`

**Trigger**: All PRs, all pushes to `main`
**Blocking**: Yes — all E2E test failures block merge
**Prerequisite**: `test-unit-integration` passes; test server running on port 8544

| Step | Command | Blocking |
|------|---------|----------|
| Start test server | `STAPLER_SQUAD_INSTANCE=e2e-ci ./stapler-squad --tmux-keep-server &` | Yes (setup) |
| E2E tests | `cd tests/e2e && npx playwright test --project=chromium` | Yes |
| Session list keyboard | Subset: `session-list-keyboard.spec.ts` | Yes |
| Review queue keyboard | Subset: `review-queue-keyboard.spec.ts` | Yes |
| Theme switching | Subset: `theme-switching.spec.ts` | Yes |
| Omnibar | Subset: `omnibar.spec.ts` | Yes |
| Drawer layout | Subset: `drawer-layout.spec.ts` | Yes |
| Page transitions | Subset: `page-transitions.spec.ts` | Yes |
| Upload Allure report | `npx allure generate` | No (advisory) |

---

### Job: `test-visual-regression`

**Trigger**: PRs touching `web-app/src/styles/`, `web-app/src/components/`, or `web-app/src/app/`
**Blocking**: Yes — diff > 0.1% threshold blocks merge
**Note**: Baselines must be regenerated and committed when intentional visual changes ship.

| Step | Command | Blocking |
|------|---------|----------|
| Start test server | Same as above | Yes |
| Visual regression (all 4 theme projects) | `cd tests/e2e && npx playwright test visual-regression.spec.ts --project=matrix-theme --project=cyberpunk77-theme --project=wh40k-theme --project=clean-theme` | Yes |
| Upload diff artifacts | `uses: actions/upload-artifact` on failure | No |

---

### Job: `test-accessibility`

**Trigger**: PRs touching `web-app/src/`
**Blocking**: Yes — critical/serious violations block merge; moderate/minor are advisory

| Step | Command | Blocking |
|------|---------|----------|
| Start test server | Same as above | Yes |
| A11y tests (all themes) | `cd tests/e2e && npx playwright test accessibility.spec.ts --project=matrix-theme --project=cyberpunk77-theme --project=wh40k-theme --project=clean-theme` | Yes (critical/serious) |
| Allure report | `npx allure generate` | No |

---

### Job: `storybook-chromatic` (advisory)

**Trigger**: PRs touching `web-app/src/components/`
**Blocking**: No — Chromatic diff review is advisory; reviewer must acknowledge

| Step | Command | Blocking |
|------|---------|----------|
| Build Storybook | `cd web-app && npm run build-storybook` | No |
| Chromatic visual diff | `cd web-app && npm run chromatic` | No (advisory) |

---

### Pipeline Order (critical path)

```
PR opened
    │
    ├──► test-unit-integration ─── blocks merge
    │
    ├──► test-e2e ──────────────── blocks merge
    │         (depends on: test-unit-integration passes)
    │
    ├──► test-visual-regression ── blocks merge (on style/component changes)
    │         (runs in parallel with test-e2e)
    │
    ├──► test-accessibility ────── blocks merge on critical/serious
    │         (runs in parallel with test-e2e)
    │
    └──► storybook-chromatic ───── advisory, non-blocking
```

**Existing CI jobs that must continue to pass** (not break):
- `make ci` (Go build + Go tests + golangci-lint + proto check)
- Existing `accessibility.spec.ts` test (`IT-5.1`)
- Existing E2E smoke tests

---

## 4. Coverage Targets

### Unit Test Coverage

- **Target**: 80% line coverage for all new `.ts` / `.tsx` files introduced in this feature.
- **Scope** (new files from plan): `ThemeContext.tsx`, `NavigationContext.tsx`, `shortcutRegistry.ts`, `useShortcut.ts`, `CommandDetector.ts`, `Kbd.tsx`, `KeyboardShortcutOverlay.tsx`, `SessionDetailBar.tsx`, `ApprovalBanner.tsx`, `check-theme-contrast.ts`.
- **Excluded from line coverage gate**: `*.css.ts` files (build-time only), `*.stories.tsx` files, fixture JSON files.
- **Coverage collection**: `jest --coverage --collectCoverageFrom='web-app/src/{lib,components,styles}/**/*.{ts,tsx}'`.

### E2E Coverage

- **Target**: All happy paths covered. Every user-facing flow introduced by this feature has at least one E2E test.
- **Happy path checklist**:
  - [x] Theme switch via omnibar command
  - [x] Theme persists on reload (FOUC prevention)
  - [x] Drawer collapse/expand via keyboard
  - [x] Session list keyboard navigation (j/k/Enter)
  - [x] Review queue keyboard approval (y/n)
  - [x] Omnibar open animation (smoke)
  - [x] Page transitions without layout shift
  - [x] Inline delete confirmation flow

### Visual Regression Coverage

- **Target**: All 4 themes × 3 key page states = 12 session-list snapshots + 8 additional (approval + omnibar) = 20 snapshots total.
- **Storage location**: `tests/snapshots/{projectName}/visual-regression/`
- **Baseline update process**: Re-run with `--update-snapshots` flag, review diff in CI artifact, commit new baselines with explicit commit message `chore(snapshots): update baselines for <change>`.

### Accessibility Coverage

- **Target**: 0 critical + serious Axe violations on all 4 themes × 2 pages = 8 core tests.
- **Known gap documented**: Matrix `textMuted` (#004d18 on #000000) has insufficient contrast for body text — this must be documented as a design decision (muted text is decorative only) or the token value must be brightened to pass.
- **Lighthouse**: Performance score ≥ 70 (measured by existing `make e2e-lighthouse`); not a new gate.

---

## 5. Test Data and Fixtures

### localStorage Fixtures

Four files required for theme Playwright projects:

- `tests/e2e/fixtures/matrix-theme.json`
- `tests/e2e/fixtures/cyberpunk77-theme.json`
- `tests/e2e/fixtures/wh40k-theme.json`
- `tests/e2e/fixtures/clean-theme.json`

Each follows the format documented in Story 7.2.1 of the plan.

### Session Seed Data

E2E tests for session list keyboard navigation and review queue require predictable session state. Strategy:
- Use `STAPLER_SQUAD_INSTANCE=e2e-ci` to isolate test state.
- The `global-setup.ts` file creates known sessions via the ConnectRPC API before tests run.
- Visual regression tests use the same seed to ensure consistent snapshots across runs.

---

## 6. Known Risks and Test Mitigations

| Risk | Test mitigation |
|------|----------------|
| Matrix `textMuted` may fail WCAG AA | `ContrastCheck_matrixMutedText_passes_AA` documents the outcome; if FAIL, opens a ticket to either brighten the token or reclassify as decorative text |
| Terminal xterm.js captures `j`/`k` keystrokes | `ShortcutRegistry_dispatch_skipsTerminalContext` unit test + `E2E_sessionListKeyboard_jkMoveFocus` verifies context guard is working |
| FOUC on non-default theme on first load | `FOUCScript_appliesStoredThemeClass_beforeHydration` JSDOM test + `E2E_themeSwitching_persists_onReload` |
| View Transitions `flushSync` conflict | `E2E_omnibar_openAnimation_elementVisibleAfterCmdK` smoke detects if the omnibar fails to open after enabling `viewTransition: true` |
| Storybook HMR instability with vanilla-extract | No automated test — documented in Storybook `README`; Chromatic runs against a static build, not HMR |
| Visual regression snapshot staleness | Snapshot update process documented in Section 4; CI artifacts show diffs on failure |
