// @feature session:create
import { test, expect } from '@playwright/test';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

test.describe('omnibar modal scroll', () => {
  test('Create Session button stays reachable on a short viewport with Advanced Options expanded', async ({ page }) => {
    // Short viewport reproduces the reported bug: modal grows past the fold and
    // the footer becomes unreachable without an internal scroll region.
    await page.setViewportSize({ width: 1024, height: 700 });
    // Pre-seed the "onboarded" flag: on a fresh browser context the first-visit
    // OnboardingModal appears ~800ms after mount (useOnboarding.ts) and, given
    // this test's several sequential interactions, can pop up mid-test and
    // block the omnibar. Unrelated to the fix under test — same
    // localStorage-seeding technique this repo already uses for theme state
    // (see theme-background.spec.ts).
    await page.addInitScript(() => {
      localStorage.setItem('stapler-squad:onboarded', 'true');
    });
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });
    // Wait for the app to hydrate (global keydown listener attaches in a
    // useEffect) before sending the shortcut — otherwise the keypress can
    // race React's mount and be dropped.
    await expect(page.getByRole('link', { name: 'Sessions' })).toBeVisible({ timeout: 10000 });
    await page.keyboard.press('Control+Shift+K');
    await expect(page.getByRole('radiogroup', { name: 'Session type' })).toBeVisible({ timeout: 5000 });

    await page.getByRole('radio', { name: 'Directory' }).click();
    await page.locator('input[aria-label="Session source input"]').fill('/tmp');

    await page.getByText('Advanced Options').click();

    // The Advanced Options section animates open via a `max-height` transition
    // (OmnibarCreationPanel.css.ts `advancedSection` -> `advancedSectionOpen`,
    // 0 -> 600px). Its fields are always present in the DOM (clipped, not
    // unmounted, while collapsed), so waiting on generic visibility isn't
    // enough — `toBeInViewport()` uses IntersectionObserver, which correctly
    // accounts for the ancestor's `overflow: hidden` clip, so it only resolves
    // once the "Program" field (the first field revealed by the expansion) is
    // actually intersecting the viewport, i.e. the section has expanded enough
    // to render it. This avoids asserting the Create button's position while
    // the modal's height is still shifting mid-transition.
    await expect(page.getByLabel('Program')).toBeInViewport();

    const createButton = page.getByRole('button', { name: 'Create Session' });
    await expect(createButton).toBeEnabled({ timeout: 3000 });
    // No scrollIntoView is called — this only passes if the button is already
    // within the visible viewport, i.e. reachable without page-level scroll.
    await expect(createButton).toBeInViewport();
  });
});
