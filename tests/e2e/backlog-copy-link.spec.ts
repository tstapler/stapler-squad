// @feature backlog:item-detail

import { test, expect } from '@playwright/test';
import { createBacklogItemDirect, enableBacklogFeatureFlag, disableBacklogFeatureFlag } from './pages/BacklogMutations';

/**
 * Story 5.1 / Surface 1 (project_plans/backlog-deep-linking): the "Copy
 * Link" affordance on the backlog item detail panel
 * (web-app/src/components/backlog/BacklogItemDetail.tsx). Locators follow
 * the actual rendered contract there:
 *   - `data-testid="copy-item-link-button"` for the button. The button's
 *     *accessible name* comes from an `aria-label` ("Copy shareable link" /
 *     "Copied link to clipboard") which does NOT contain the contiguous
 *     substring "copy link" that validation.md's suggested locator
 *     (`getByRole('button', {name: /copy link/i})`) assumes — so this file
 *     uses the testid instead. The button's *visible text* does cycle
 *     "Copy Link" -> "✓ Copied" -> "Copy Link" as validation.md describes.
 *   - `data-testid="copy-status-announcement"` (`role="status"`,
 *     `aria-live="polite"`) for the AC6 screen-reader announcement.
 */

const REVERT_TIMEOUT = 1500;

test.describe('Backlog item Copy Link', () => {
  test.beforeAll(async ({ request }) => {
    await enableBacklogFeatureFlag(request);
  });

  test.afterAll(async ({ request }) => {
    await disableBacklogFeatureFlag(request);
  });

  test.beforeEach(async ({ context, page }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await page.addInitScript(() => {
      localStorage.setItem('stapler-squad:backlog-onboarded', 'true');
    });
  });

  test('copyLink_should_CopyUrlInOneClickAndRevertLabel_When_Clicked', async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, { title: `e2e-copy-link-${Date.now()}` });
    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });

    const button = page.getByTestId('copy-item-link-button');
    await expect(button).toBeVisible();
    await expect(button).toHaveText('Copy Link');

    await button.click();

    const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
    expect(clipboardText).toMatch(/^ssq:\/\//);

    await expect(button).toHaveText('✓ Copied');
    // Revert after the 1500ms timer — poll via toHaveText, no waitForTimeout.
    await expect(button).toHaveText('Copy Link', { timeout: REVERT_TIMEOUT + 2000 });
  });

  test('copyLink_should_ProduceWellFormedURLWithNoEncodingArtifacts_When_Copied', async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, { title: `e2e-copy-link-wellformed-${Date.now()}` });
    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });

    await page.getByTestId('copy-item-link-button').click();
    const clipboardText = await page.evaluate(() => navigator.clipboard.readText());

    // Must parse as a well-formed URL with no double-encoding artifacts.
    expect(() => new URL(clipboardText)).not.toThrow();
    expect(clipboardText).not.toContain('%25'); // double-encoded '%'
    expect(clipboardText).not.toMatch(/%[0-9A-Fa-f]{2}.*%[0-9A-Fa-f]{2}/); // no stray percent-encoding at all
    expect(clipboardText).toContain(`/backlog/v1/${itemId}`);
  });

  test('copyLink_should_ShowCopyFailedState_When_ClipboardWriteRejected', async ({ context, page, request }) => {
    // GAP (Surface 1 AC4): BacklogItemDetail.tsx's handleCopy has no
    // failure branch — copyToClipboard()'s `ok === false` case is a silent
    // no-op (setCopiedField is only ever called inside the `.then((ok) => {
    // if (!ok ...) return; ...})` guard). There is no "Copy failed" text,
    // state, or console warning anywhere in the component. This test
    // documents the gap rather than asserting a UX that doesn't exist —
    // see BacklogItemDetail.tsx:161-171.
    test.fail(true, 'Copy Link has no distinct "Copy failed" state on clipboard-write failure (Gap: BacklogItemDetail.tsx handleCopy silently no-ops when copyToClipboard() resolves false) — see project_plans/backlog-deep-linking/implementation/validation.md AC4');

    await context.grantPermissions([]); // revoke clipboard permissions granted in beforeEach
    const itemId = await createBacklogItemDirect(request, { title: `e2e-copy-link-fail-${Date.now()}` });
    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });

    const consoleWarnings: string[] = [];
    page.on('console', (msg) => {
      if (msg.type() === 'warning') consoleWarnings.push(msg.text());
    });

    // Force both clipboard write paths to fail so copyToClipboard()
    // resolves false regardless of which branch it takes.
    await page.addInitScript(() => {
      Object.defineProperty(navigator, 'clipboard', {
        value: { writeText: () => Promise.reject(new Error('denied')) },
        configurable: true,
      });
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      (document as any).execCommand = () => false;
    });
    await page.reload({ waitUntil: 'domcontentloaded' });

    const button = page.getByTestId('copy-item-link-button');
    await button.click();

    await expect(button).toHaveText(/copy failed/i);
    expect(consoleWarnings.length).toBeGreaterThan(0);
  });

  test('copyLink_should_RemainFocusedAfterActivation_When_TriggeredViaKeyboard', async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, { title: `e2e-copy-link-kbd-${Date.now()}` });
    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });

    const button = page.getByTestId('copy-item-link-button');
    await button.focus();
    await expect(button).toBeFocused();

    await page.keyboard.press('Enter');

    await expect(button).toBeFocused();
    await expect(button).toHaveText('✓ Copied');
    await expect(button).toHaveText('Copy Link', { timeout: REVERT_TIMEOUT + 2000 });
  });

  test('copyLink_should_AnnounceViaAriaLivePolite_When_ActivatedViaKeyboard', async ({ page, request }) => {
    const itemId = await createBacklogItemDirect(request, { title: `e2e-copy-link-announce-${Date.now()}` });
    await page.goto(`/backlog?item=${itemId}`, { waitUntil: 'domcontentloaded' });

    const liveRegion = page.getByTestId('copy-status-announcement');
    await expect(liveRegion).toHaveAttribute('aria-live', 'polite');

    const button = page.getByTestId('copy-item-link-button');
    await button.focus();
    await page.keyboard.press('Enter');

    await expect(liveRegion).toHaveText('Link copied to clipboard');
  });

  test('copyLink_should_ProduceWorkingUUIDForm_When_ItemHasNoPublicIdYet', async ({ page, request }) => {
    // GAP (Surface 1 AC8): no seeding mechanism (createBacklogItemDirect,
    // any other BacklogMutations.ts helper, or any /api/debug/backlog/*
    // handler) can create a backlog item without a public_id —
    // CreateBacklogItem unconditionally mints one. There is no way to set
    // up this pre-backfill scenario truthfully via any exposed mechanism,
    // so this AC cannot be exercised end-to-end today.
    test.skip(true, 'No available seeding mechanism creates a backlog item without a public_id — CreateBacklogItem always mints one (checked BacklogMutations.ts and all /api/debug/backlog/* handlers). Cannot truthfully set up this pre-backfill scenario.');
  });
});
