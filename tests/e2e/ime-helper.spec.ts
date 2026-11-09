// @feature mobile-input:ime-helper
// Spike 0.1.2: proves helpers/ime.ts drives a real Chromium textarea. Self-contained (page.setContent),
// needs no stapler-squad server. Events are REAL Chromium events from CDP, not Gboard recordings.
import { test, expect, devices } from '@playwright/test';
import { imeBackspace, imeCommit, imeCompose } from './helpers/ime';

test.use({ ...devices['Pixel 7'] });

test.describe('ime helper (CDP)', () => {
  test.beforeEach(async ({ page }) => {
    await page.setContent('<textarea aria-label="ime probe"></textarea>');
    await page.getByRole('textbox', { name: 'ime probe' }).focus();
    await page.evaluate(() => {
      const w = window as unknown as { log: string[] };
      w.log = [];
      const t = document.querySelector('textarea') as HTMLTextAreaElement;
      for (const ev of ['keydown', 'beforeinput', 'input', 'compositionstart', 'compositionend']) {
        t.addEventListener(ev, (e) => w.log.push(ev + ((e as InputEvent).inputType ? ':' + (e as InputEvent).inputType : '')));
      }
    });
  });

  test('composed text reaches the textarea as insertCompositionText then commits', async ({ page }) => {
    const box = page.getByRole('textbox', { name: 'ime probe' });
    await imeCompose(page, 'hel');
    await imeCompose(page, 'hello');
    await expect(box).toHaveValue('hello');
    await imeCommit(page, 'hello');
    await expect(box).toHaveValue('hello');
    const log = await page.evaluate(() => (window as unknown as { log: string[] }).log);
    expect(log[0]).toBe('compositionstart');
    expect(log).toContain('beforeinput:insertCompositionText');
    expect(log[log.length - 1]).toBe('compositionend');
  });

  test('backspace keycode-229 on an empty field emits keydown without beforeinput', async ({ page }) => {
    await imeBackspace(page, 'keycode-229');
    const log = await page.evaluate(() => (window as unknown as { log: string[] }).log);
    expect(log).toEqual(['keydown']);
  });
});
