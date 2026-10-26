// @feature window:switch, a11y:window-switch
/**
 * Window-switch focus, announcement, tabpanel and highlight (Story 4.1.4, UX criteria 1-5, 7, 8, 14, 33).
 * Runs with `window_keep_alive` OFF (the only state that exists today); re-run under the flag once Story 4.1.2
 * lands. Criterion 15 (leader 4 s idle) uses `page.clock`; the Task 1.1.3e spike for it has not run in CI
 * (research/ci-spikes.md), so it also keeps its Jest fake-timer twin in `useWindowShortcuts.test.ts`.
 * Criterion 32 is automated here as a keyboard-only proxy; the VoiceOver pass stays manual.
 */
import { test, expect, Page } from '@playwright/test';
import { WindowTabStripPage } from './pages/WindowTabStripPage';

async function seedThreeWindows(page: Page): Promise<WindowTabStripPage> {
  await page.goto('/');
  const strip = new WindowTabStripPage(page);
  await strip.waitForLoaded();
  await strip.createWindow();
  await strip.createWindow();
  await expect(strip.tabs).toHaveCount(3);
  await strip.renameTab('Window 2', 'api');
  await strip.switchToTab('Window 1');
  await expect(strip.getActiveTab()).toHaveAttribute('title', 'Window 1');
  return strip;
}

const activeElementIsTab = (page: Page, name: string) =>
  page.evaluate((n) => {
    const el = document.activeElement;
    return el?.getAttribute('role') === 'tab' && el.getAttribute('aria-label') === n;
  }, name);

const activeElementInsidePanel = (page: Page) =>
  page.evaluate(() => {
    const panel = document.querySelector('[role="tabpanel"]');
    const el = document.activeElement;
    return !!panel && !!el && panel.contains(el);
  });

test.beforeEach(async ({ context }) => {
  await context.addInitScript(() => localStorage.setItem('stapler-squad:onboarded', 'true'));
});

test.describe('window switch: highlight, focus, announcement, tabpanel', () => {
  test('leaderSwitch_should_SetAriaSelectedInSameFrame_When_AltWThen2', async ({ page }) => {
    const strip = await seedThreeWindows(page);
    await page.keyboard.press('Alt+w');
    await page.keyboard.press('2');
    // One animation frame after the keydown, before any URL-driven content commit is awaited.
    await page.evaluate(() => new Promise<void>((resolve) => requestAnimationFrame(() => resolve())));
    await expect(strip.getTab('api')).toHaveAttribute('aria-selected', 'true');
  });

  test('switch_should_ReachTargetInOneClick_And_TwoKeysForLeader_When_ClickedOrKeyed', async ({ page }) => {
    const strip = await seedThreeWindows(page);
    await strip.switchToTab('Window 3');
    await expect(strip.getActiveTab()).toHaveAttribute('title', 'Window 3');
    await page.keyboard.press('Alt+w');
    await page.keyboard.press('1');
    await expect(strip.getActiveTab()).toHaveAttribute('title', 'Window 1');
  });

  test('activeTab_should_ExposeAriaSelectedAndNoAriaCurrent_When_WindowSelected', async ({ page }) => {
    await seedThreeWindows(page);
    await expect(page.locator('[role="tab"][aria-selected="true"]')).toHaveCount(1);
    await expect(page.locator('[role="tablist"] [aria-current]')).toHaveCount(0);
  });

  test('tab_should_ControlTabpanelWithBackReference_When_FlagOff', async ({ page }) => {
    const strip = await seedThreeWindows(page);
    const tab = strip.getActiveTab();
    const controls = await tab.getAttribute('aria-controls');
    expect(controls).toBeTruthy();
    const panel = page.locator(`[id="${controls}"]`);
    await expect(panel).toHaveAttribute('role', 'tabpanel');
    await expect(panel).toHaveAttribute('aria-labelledby', (await tab.getAttribute('id')) as string);
  });

  test('windowSwitch_should_AnnounceOnceInSingleLiveRegion_When_NoPathnameChange', async ({ page }) => {
    const strip = await seedThreeWindows(page);
    // RouteAnnouncer only exists in the react-router build (WEB_BUILD=bun); the Next build has no polite route announcer.
    const region = page.getByTestId('route-announcer');
    test.skip((await region.count()) === 0, 'RouteAnnouncer is not mounted in the Next build');
    await strip.switchToTab('api');
    await expect(region).toHaveText('Window 2 of 3: api');
    await expect(page.getByTestId('route-announcer')).toHaveCount(1);
    await strip.switchToTab('Window 1');
    await strip.switchToTab('api');
    await expect(region).toHaveText('Window 2 of 3: api');
  });

  test('tablistKeys_should_KeepFocusOnTargetTab_When_ArrowHomeEndEnterOrSpacePressed', async ({ page }) => {
    const strip = await seedThreeWindows(page);
    await strip.getTab('Window 1').focus();
    await page.keyboard.press('ArrowRight');
    expect(await activeElementIsTab(page, 'api')).toBe(true);
    await page.keyboard.press('End');
    expect(await activeElementIsTab(page, 'Window 3')).toBe(true);
    await page.keyboard.press('Home');
    expect(await activeElementIsTab(page, 'Window 1')).toBe(true);
    await strip.getTab('api').focus();
    await page.keyboard.press('Enter');
    await expect(strip.getActiveTab()).toHaveAttribute('title', 'api');
    expect(await activeElementIsTab(page, 'api')).toBe(true);
    await page.keyboard.press('Space');
    expect(await activeElementIsTab(page, 'api')).toBe(true);
    await page.keyboard.press('Tab');
    expect(await activeElementInsidePanel(page)).toBe(true);
  });

  test('focus_should_LandInTargetPaneOrTabpanel_When_MouseClickOrLeader', async ({ page }) => {
    const strip = await seedThreeWindows(page);
    await strip.switchToTab('api');
    await expect.poll(() => activeElementInsidePanel(page)).toBe(true);
    await page.keyboard.press('Alt+w');
    await page.keyboard.press('3');
    await expect(strip.getActiveTab()).toHaveAttribute('title', 'Window 3');
    await expect.poll(() => activeElementInsidePanel(page)).toBe(true);
  });

  test('keyboardOnlyFlow_should_AnnounceOrShowFocusAtEachStep_When_SwitchToWindow3AndType', async ({ page }) => {
    const strip = await seedThreeWindows(page);
    const region = page.getByTestId('route-announcer');
    const hasAnnouncer = (await region.count()) > 0;

    // Step 1: focus the tablist by keyboard; focus is evident on a tab.
    await strip.getActiveTab().focus();
    expect(await page.evaluate(() => document.activeElement?.getAttribute('role'))).toBe('tab');

    // Step 2: leader switch to window 3; the selected state and (where mounted) the announcement follow.
    await page.keyboard.press('Alt+w');
    await page.keyboard.press('3');
    await expect(strip.getActiveTab()).toHaveAttribute('title', 'Window 3');
    await expect(strip.getTab('Window 3')).toHaveAttribute('aria-selected', 'true');
    if (hasAnnouncer) await expect(region).toHaveText('Window 3 of 3: Window 3');

    // Step 3: focus lands inside the tabpanel, so typing reaches that window's content, not the tab strip.
    await expect.poll(() => activeElementInsidePanel(page)).toBe(true);
    await page.keyboard.type('ls');
    expect(await activeElementIsTab(page, 'Window 3')).toBe(false);
    await expect(strip.getActiveTab()).toHaveAttribute('title', 'Window 3');
  });

  test('leaderKey_should_Disarm_When_4sIdleBeforeDigit', async ({ page }) => {
    // Installed before navigation so the leader timer runs on the fake clock; time still flows until fastForward.
    await page.clock.install();
    const strip = await seedThreeWindows(page);
    await expect(strip.getActiveTab()).toHaveAttribute('title', 'Window 1');
    await page.keyboard.press('Alt+w');
    await page.clock.fastForward(4000);
    await page.keyboard.press('2');
    await expect(strip.getActiveTab()).toHaveAttribute('title', 'Window 1');
    // Control: the same sequence without idle does switch, so the disarm assertion above is not vacuous.
    await page.keyboard.press('Alt+w');
    await page.keyboard.press('2');
    await expect(strip.getActiveTab()).toHaveAttribute('title', 'api');
  });

  for (const reducedMotion of ['reduce', 'no-preference'] as const) {
    test(`switch_should_RunNoTransitionOrAnimation_When_ReducedMotion_${reducedMotion}`, async ({ page }) => {
      await page.emulateMedia({ reducedMotion });
      const strip = await seedThreeWindows(page);
      await strip.switchToTab('api');
      const result = await page.evaluate(() => {
        const durations = [...document.querySelectorAll('[role="tab"], [role="tabpanel"]')].map(
          (el) => getComputedStyle(el).transitionDuration,
        );
        return { animations: document.getAnimations().length, durations };
      });
      expect(result.animations).toBe(0);
      for (const d of result.durations) expect(d.split(',').every((part) => parseFloat(part) === 0)).toBe(true);
    });
  }
});

test.describe('window switch on a coarse pointer', () => {
  test.use({ hasTouch: true, isMobile: true });

  test('touchSwitch_should_NotFocusTerminalAndMeet44pxTargets_When_TapOnCoarsePointer', async ({ page }) => {
    const strip = await seedThreeWindows(page);
    await strip.getTab('Window 3').tap();
    await expect(strip.getActiveTab()).toHaveAttribute('title', 'Window 3');
    expect(await activeElementIsTab(page, 'Window 3')).toBe(true);
    await expect(page.locator('.xterm-helper-textarea:focus')).toHaveCount(0);
    for (const tab of await strip.tabs.all()) {
      const box = await tab.boundingBox();
      expect(box?.width).toBeGreaterThanOrEqual(44);
      expect(box?.height).toBeGreaterThanOrEqual(44);
    }
  });
});
