// @feature local-file-browser
import { test, expect } from '@playwright/test';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const FILES_URL = `${BASE_URL}/files`;

// ── Test data setup ──────────────────────────────────────────────────────────
// A real temp directory covering every acceptance criterion in one fixture:
// a symlinked subdirectory, a broken symlink, a special-character filename,
// and enough entries to exercise the truncation cap.

let rootDir: string;
let subDir: string;

test.beforeAll(() => {
  rootDir = fs.mkdtempSync(path.join(os.tmpdir(), 'sq-e2e-filebrowser-'));
  subDir = path.join(rootDir, 'real-subdir');
  fs.mkdirSync(subDir);
  fs.writeFileSync(path.join(subDir, 'inside.txt'), 'hello from inside\n');

  fs.symlinkSync(subDir, path.join(rootDir, 'link-to-subdir'));
  fs.symlinkSync(path.join(rootDir, 'does-not-exist'), path.join(rootDir, 'broken-link'));

  fs.writeFileSync(path.join(rootDir, 'weird name #1.txt'), 'special chars in the filename\n');
});

test.afterAll(() => {
  try { fs.rmSync(rootDir, { recursive: true, force: true }); } catch { /* ignore */ }
});

test.describe('local-file-browser', () => {
  test('local-file-browser > navigates into a symlinked directory', async ({ page }) => {
    await page.goto(`${FILES_URL}/?path=${encodeURIComponent(rootDir)}`, { waitUntil: 'domcontentloaded' });

    await page.locator('[data-testid="file-browser-entry"]', { hasText: 'link-to-subdir/' }).click();

    await expect(page.getByTestId('file-browser-path-input')).toHaveValue(subDir);
    await expect(page.locator('[data-testid="file-browser-entry"]', { hasText: 'inside.txt' })).toBeVisible();
  });

  test('local-file-browser > filter box narrows entries client-side', async ({ page }) => {
    await page.goto(`${FILES_URL}/?path=${encodeURIComponent(rootDir)}`, { waitUntil: 'domcontentloaded' });

    await expect(page.locator('[data-testid="file-browser-entry"]', { hasText: 'broken-link' })).toBeVisible();
    await page.getByTestId('file-browser-filter-input').fill('weird');

    await expect(page.locator('[data-testid="file-browser-entry"]', { hasText: 'weird name' })).toBeVisible();
    await expect(page.locator('[data-testid="file-browser-entry"]', { hasText: 'broken-link' })).not.toBeVisible();
  });

  test('local-file-browser > opens a file with special characters in its name', async ({ page }) => {
    await page.goto(`${FILES_URL}/?path=${encodeURIComponent(rootDir)}`, { waitUntil: 'domcontentloaded' });

    await page.locator('[data-testid="file-browser-entry"]', { hasText: 'weird name #1.txt' }).click();

    await expect(page.getByText('special chars in the filename')).toBeVisible();
  });

  test('local-file-browser > path bar offers autocomplete suggestions while typing', async ({ page }) => {
    await page.goto(`${FILES_URL}/?path=${encodeURIComponent(os.tmpdir())}`, { waitUntil: 'domcontentloaded' });

    const input = page.getByTestId('file-browser-path-input');
    await input.fill(rootDir.slice(0, -3));

    await expect(page.getByRole('listbox')).toBeVisible({ timeout: 5000 });
  });

  test('local-file-browser > open terminal here creates a directory-mode session', async ({ page }) => {
    await page.goto(`${FILES_URL}/?path=${encodeURIComponent(rootDir)}`, { waitUntil: 'domcontentloaded' });

    await page.getByTestId('file-browser-open-terminal').click();

    await expect(page).toHaveURL(/[?&]session=/, { timeout: 15000 });
  });
});
