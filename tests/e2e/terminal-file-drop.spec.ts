// @feature terminal:file-drop
import { test, expect, type Page } from '@playwright/test';
import { SessionClient } from './helpers/session-client';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

async function openSessionTerminal(page: Page, sessionId: string) {
  await page.goto(`${BASE_URL}/?session=${sessionId}`, { waitUntil: 'domcontentloaded', timeout: 10000 });
  await expect(page.getByTestId('toolbar-toggle')).toBeVisible({ timeout: 8000 });
  return page.getByTestId('terminal-drop-zone');
}

/** Dispatches a synthetic drag event carrying the given transfer types/files on the drop zone. */
async function dispatchDrag(page: Page, type: 'dragenter' | 'dragleave' | 'drop', transferTypes: 'files' | 'pane') {
  await page.getByTestId('terminal-drop-zone').evaluate((el, { type, transferTypes }) => {
    const dt = new DataTransfer();
    if (transferTypes === 'files') dt.items.add(new File(['hello'], 'dropped.txt', { type: 'text/plain' }));
    else dt.setData('text/plain', 'pane-id');
    el.dispatchEvent(new DragEvent(type, { dataTransfer: dt, bubbles: true, cancelable: true }));
  }, { type, transferTypes });
}

test.describe('terminal:file-drop', () => {
  let client: SessionClient;
  let sessionId: string;

  test.beforeEach(async () => {
    client = new SessionClient(BASE_URL);
    sessionId = (await client.createIdleSession(`e2e-file-drop-${Date.now()}`, '/tmp')).id;
  });

  test('shows the drop overlay while a file is dragged over and hides it on leave', async ({ page }) => {
    await openSessionTerminal(page, sessionId);

    await dispatchDrag(page, 'dragenter', 'files');
    await expect(page.getByTestId('drop-zone-overlay')).toBeVisible();

    await dispatchDrag(page, 'dragleave', 'files');
    await expect(page.getByTestId('drop-zone-overlay')).toHaveCount(0);
  });

  test('ignores pane-reorder drags (no Files in dataTransfer)', async ({ page }) => {
    await openSessionTerminal(page, sessionId);

    await dispatchDrag(page, 'dragenter', 'pane');
    await expect(page.getByTestId('drop-zone-overlay')).toHaveCount(0);
  });

  test('dropping a file uploads it via /v1/upload-image and clears the overlay', async ({ page }) => {
    await openSessionTerminal(page, sessionId);

    const upload = page.waitForRequest((r) => r.url().includes('/v1/upload-image') && r.method() === 'POST');
    await dispatchDrag(page, 'dragenter', 'files');
    await dispatchDrag(page, 'drop', 'files');

    await upload;
    await expect(page.getByTestId('drop-zone-overlay')).toHaveCount(0);
  });
});
