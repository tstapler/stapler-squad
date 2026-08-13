// @feature backlog:item-form, upload:backlog-attachment
import { test, expect } from '@playwright/test';
import { BacklogPage } from './pages/BacklogPage';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

// Minimal valid 1x1 PNG (magic bytes + IHDR/IDAT/IEND), used to exercise the
// real upload endpoint's magic-byte validation rather than a fake extension.
const PNG_BASE64 =
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=';

test.describe('Backlog Rich Text', () => {
  test.beforeAll(async ({ request }) => {
    await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
      headers: { 'Content-Type': 'application/json' },
      data: { name: 'backlog', enabled: true },
    });
  });

  test.afterAll(async ({ request }) => {
    await request.post(`${BASE_URL}/api/session.v1.SessionService/UpdateFeatureFlag`, {
      headers: { 'Content-Type': 'application/json' },
      data: { name: 'backlog', enabled: false },
    });
  });

  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      localStorage.setItem('stapler-squad:backlog-onboarded', 'true');
    });
    await page.goto(`${BASE_URL}/backlog`, { waitUntil: 'domcontentloaded' });
    await page.waitForSelector('[data-testid="backlog-page"]', { timeout: 15000 });
  });

  test('e2e:backlog-rich-text-preview - Write/Preview toggle renders markdown', async ({ page }) => {
    const backlogPage = new BacklogPage(page);
    await backlogPage.openNewItemForm();

    const description = page.locator('[data-testid="backlog-description-input"]');
    await description.fill('**bold text** and a [link](https://example.com)');

    await page.locator('[data-testid="backlog-description-tab-preview"]').click();
    const preview = page.locator('[data-testid="backlog-description-preview"]');
    await expect(preview.locator('strong')).toHaveText('bold text');
    await expect(preview.locator('a')).toHaveAttribute('href', 'https://example.com');

    await page.locator('[data-testid="backlog-description-tab-write"]').click();
    await expect(description).toHaveValue('**bold text** and a [link](https://example.com)');
  });

  test('e2e:backlog-rich-text-upload - Attaching an image inserts markdown and persists after reload', async ({ page }) => {
    const backlogPage = new BacklogPage(page);
    const itemTitle = `rich-text-upload-${Date.now()}`;

    await backlogPage.openNewItemForm();
    await page.locator('[data-testid="backlog-title-input"]').fill(itemTitle);

    const fileInput = page.locator('[data-testid="backlog-attach-image-input"]');
    await fileInput.setInputFiles({
      name: 'screenshot.png',
      mimeType: 'image/png',
      buffer: Buffer.from(PNG_BASE64, 'base64'),
    });

    const description = page.locator('[data-testid="backlog-description-input"]');
    await expect(description).toHaveValue(/\.png/, { timeout: 10000 });

    await page.locator('[data-testid="backlog-description-tab-preview"]').click();
    await expect(page.locator('[data-testid="backlog-description-preview"] img')).toBeVisible();
    await page.locator('[data-testid="backlog-description-tab-write"]').click();

    await page.locator('[data-testid="backlog-form-submit"]').click();
    await page.waitForSelector('[data-testid="backlog-form-modal"]', { state: 'hidden', timeout: 5000 });

    const itemRow = backlogPage.getTableRows().filter({ hasText: itemTitle });
    await expect(itemRow.first()).toBeVisible();
    await backlogPage.openItemDetail(itemTitle);

    // Relies on Description defaulting expanded (backlog-description-prominence) —
    // CollapsibleSection removes collapsed content from the DOM entirely, so this
    // assertion would fail with no header click if that default ever reverts.
    const rendered = page.locator('[data-testid="backlog-description-rendered"]');
    await expect(rendered.locator('img')).toBeVisible();

    // Persists after reload — not just an in-memory form state artifact.
    await page.reload({ waitUntil: 'domcontentloaded' });
    await backlogPage.openItemDetail(itemTitle);
    await expect(page.locator('[data-testid="backlog-description-rendered"] img')).toBeVisible();
  });

  test('e2e:backlog-rich-text-mobile-fallback - Image attach works without drag-and-drop on a mobile viewport', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    const backlogPage = new BacklogPage(page);
    await backlogPage.openNewItemForm();

    // The file input fallback must be present and directly usable (no drag events).
    const fileInput = page.locator('[data-testid="backlog-attach-image-input"]');
    await expect(fileInput).toBeAttached();
    await fileInput.setInputFiles({
      name: 'mobile-shot.png',
      mimeType: 'image/png',
      buffer: Buffer.from(PNG_BASE64, 'base64'),
    });

    const description = page.locator('[data-testid="backlog-description-input"]');
    await expect(description).toHaveValue(/\.png/, { timeout: 10000 });
  });
});
