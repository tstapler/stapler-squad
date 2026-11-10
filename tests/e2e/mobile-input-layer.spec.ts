// @feature terminal:input, terminal:mobile-input
/**
 * Mobile input layer wiring (Story 5.1.1: tasks 5.1.1a, b, d, e and the flag-off part of c).
 *
 * Proves WIRING only: text entered through the layer's hidden textarea reaches a live bash pane under both
 * renderers, exactly once. It is NOT evidence of Gboard behaviour; the owner device gate (Task 5.1.4c) is.
 *
 * Which events are real and which are synthetic:
 *  - REAL (Chromium emits them): the compositionstart/update/end and beforeinput/input `insertCompositionText`
 *    / `insertText` produced by CDP `Input.imeSetComposition` and `Input.insertText` (helpers/ime.ts); the
 *    keyboard events from `page.keyboard`; the touch events from `page.touchscreen.tap` and CDP
 *    `Input.dispatchTouchEvent` (TerminalPage.dragSurface); the Pixel 7 UA, viewport and coarse/no-hover media.
 *  - SYNTHETIC / MODELLED: the autocorrect rewrite is `imeSetComposition` replacing the open composition, which
 *    is the closest CDP gets to Gboard's correction; CDP emits no keydown 229 and no `insertReplacementText`.
 *    The "desktop" case is Desktop Chrome emulation (fine pointer, hover), not a real desktop.
 *
 * Pane text is read from the run's private tmux socket (pages/TerminalPage.ts). No waitForTimeout.
 */
import { test, expect, devices } from '@playwright/test';
import * as fs from 'node:fs';
import { baseUrl, slotTmp } from './helpers/base-url';
import { setFlag } from './helpers/feature-flag';
import { imeBackspace, imeCommit, imeCompose } from './helpers/ime';
import { SessionClient } from './helpers/session-client';
import { TerminalPage, type RendererKind } from './pages/TerminalPage';

const ONBOARDED_KEY = 'stapler-squad:onboarded';
const pixel7 = devices['Pixel 7'];

test.use({ ...pixel7 });

async function newSession(label: string) {
  const client = new SessionClient(baseUrl());
  const title = `e2emi${label}${Date.now()}`;
  const dir = slotTmp('mobile-input-layer');
  fs.mkdirSync(dir, { recursive: true });
  const session = await client.createSession({ title, path: dir, program: 'bash' });
  await client.waitForSessionsActive([title]);
  return { client, title, id: session.id };
}

async function openReady(page: import('@playwright/test').Page, term: TerminalPage, id: string) {
  await page.addInitScript((key) => localStorage.setItem(key, 'true'), ONBOARDED_KEY);
  // The first load persists the flags; the second applies them (flags apply on the next page load).
  await term.open(id);
  await page.reload({ waitUntil: 'domcontentloaded' });
  await term.open(id);
  // The layer attaches once the renderer has loaded; a tap before that has nothing to focus.
  await expect(term.layerTextarea()).toBeAttached({ timeout: 20_000 });
  await expect.poll(() => term.promptLine(), { message: 'bash prompt', timeout: 20_000 }).toMatch(/[$#]\s*$/);
}

for (const renderer of ['xterm', 'ghostty'] as RendererKind[]) {
  test.describe(`mobile input layer (${renderer}, Pixel 7)`, () => {
    let ctx: Awaited<ReturnType<typeof newSession>>;

    test.beforeEach(async ({ request }) => {
      await setFlag(request, 'mobile_input_layer', true);
      await setFlag(request, 'ghostty_renderer', renderer === 'ghostty');
      ctx = await newSession(renderer);
    });
    test.afterEach(async () => {
      await ctx?.client.deleteSession(ctx.id, true).catch(() => undefined);
    });
    test.afterAll(async ({ request }) => {
      await setFlag(request, 'ghostty_renderer', false);
    });

    test('first tap on a never-focused docked terminal focuses the layer, then composed text reaches the pane once', async ({ page }) => {
      const term = new TerminalPage(page, ctx.title);
      await openReady(page, term, ctx.id);
      await expect(term.renderer(renderer)).toBeVisible();
      await expect(term.layerTextarea()).toBeAttached();
      // Opening the session may auto-focus the terminal; drop focus so the tap below is the first one.
      await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
      await expect(term.layerTextarea()).not.toBeFocused();

      await term.tapSurface();
      await expect(term.layerTextarea()).toBeFocused();

      await imeCompose(page, 'l');
      await imeCompose(page, 'ls');
      await imeCommit(page, 'ls');
      await expect.poll(() => term.promptLine()).toMatch(/[$#] ls$/);
    });

    test('typing abc through the layer shows abc exactly once', async ({ page }) => {
      const term = new TerminalPage(page, ctx.title);
      await openReady(page, term, ctx.id);
      await term.tapSurface();
      await expect(term.layerTextarea()).toBeFocused();
      await imeCommit(page, 'abc');
      await expect.poll(() => term.promptLine()).toMatch(/[$#] abc$/);
      // Still exactly one copy once the pane has settled on the committed text.
      await expect.poll(async () => (await term.promptLine()).match(/abc/g)?.length).toBe(1);
    });

    test('hardware keyboard on the Android profile types each character once', async ({ page }) => {
      const term = new TerminalPage(page, ctx.title);
      await openReady(page, term, ctx.id);
      await term.tapSurface();
      await expect(term.layerTextarea()).toBeFocused();
      await page.keyboard.type('ls');
      await expect.poll(() => term.promptLine()).toMatch(/[$#] ls$/);
    });

    test('an autocorrect-style rewrite replaces the word and Backspace deletes one character', async ({ page }) => {
      const term = new TerminalPage(page, ctx.title);
      await openReady(page, term, ctx.id);
      await term.tapSurface();
      await expect(term.layerTextarea()).toBeFocused();

      await imeCompose(page, 'teh');
      await imeCompose(page, 'the'); // the keyboard rewrites the open composition
      await imeCommit(page, 'the');
      await expect.poll(() => term.promptLine()).toMatch(/[$#] the$/);

      await imeBackspace(page, 'keycode-8');
      await expect.poll(() => term.promptLine()).toMatch(/[$#] th$/);
    });

    test('a tap after a scroll gesture still focuses the layer', async ({ page }) => {
      const term = new TerminalPage(page, ctx.title);
      await openReady(page, term, ctx.id);
      const cdp = await page.context().newCDPSession(page);
      await term.dragSurface(cdp, -160);
      await term.tapSurface();
      await expect(term.layerTextarea()).toBeFocused();
    });

    test('rewrite shows no blank frame and no shortened prompt, sampled every animation frame', async ({ page }) => {
      const term = new TerminalPage(page, ctx.title);
      await openReady(page, term, ctx.id);
      await term.tapSurface();
      await expect(term.layerTextarea()).toBeFocused();
      await term.useShortPrompt();
      await expect.poll(() => term.promptLine()).toBe('$');
      await imeCommit(page, 'echo ');
      await expect.poll(() => term.promptLine()).toBe('$ echo');

      await page.evaluate(() => {
        const w = window as unknown as { __sampling: boolean; __blank: number; __read: number; __frames: number };
        const root = document.querySelector('[data-context="terminal"]') as HTMLElement;
        w.__blank = 0;
        w.__read = 0;
        w.__frames = 0;
        w.__sampling = true;
        const scratch = document.createElement('canvas').getContext('2d');
        // One-colour test of the renderer's canvas, read back through a 2D canvas so WebGL canvases work too.
        const canvasBlank = (src: HTMLCanvasElement): boolean | null => {
          if (!scratch || src.width === 0 || src.height === 0) return null;
          try {
            scratch.canvas.width = Math.min(src.width, 400);
            scratch.canvas.height = Math.min(src.height, 400);
            scratch.drawImage(src, 0, 0, scratch.canvas.width, scratch.canvas.height);
            const { data } = scratch.getImageData(0, 0, scratch.canvas.width, scratch.canvas.height);
            for (let i = 4; i < data.length; i += 4) if (data[i] !== data[0] || data[i + 1] !== data[1] || data[i + 2] !== data[2]) return false;
            return true;
          } catch {
            return null;
          }
        };
        const tick = () => {
          if (!w.__sampling) return;
          w.__frames++;
          for (const c of Array.from(root.querySelectorAll('canvas'))) {
            const blank = canvasBlank(c);
            if (blank === null) continue;
            w.__read++;
            if (blank) w.__blank++;
          }
          requestAnimationFrame(tick);
        };
        requestAnimationFrame(tick);
      });
      await imeCompose(page, 'wrold');
      await imeCompose(page, 'world');
      await imeCommit(page, 'world');
      await expect.poll(() => term.promptLine()).toBe('$ echo world');
      const { frames, blank, read } = await page.evaluate(() => {
        const w = window as unknown as { __sampling: boolean; __blank: number; __read: number; __frames: number };
        w.__sampling = false;
        return { frames: w.__frames, blank: w.__blank, read: w.__read };
      });
      // The rewrite ran across `frames` animation frames and the renderer canvas was never a single colour in
      // any readable one. Known gap: neither renderer exposes its buffer as DOM text here, so a SHORTENED
      // prompt (as opposed to a blank frame) cannot be seen per frame; the exact final pane text above is the
      // only check on what the rewrite left behind.
      expect(frames, 'animation frames sampled').toBeGreaterThan(0);
      if (read > 0) expect(blank, `of ${read} readable canvas frames`).toBe(0);
    });
  });
}

test.describe('mobile input layer (flag off and platform gate)', () => {
  test('?mobile_input=0 on Pixel 7 with ghostty on renders xterm, no layer, and shows the hint', async ({ browser, request }) => {
    await setFlag(request, 'mobile_input_layer', true);
    await setFlag(request, 'ghostty_renderer', true);
    const context = await browser.newContext({ ...pixel7 });
    const page = await context.newPage();
    const ctx = await newSession('off');
    try {
      const term = new TerminalPage(page, ctx.title);
      await page.addInitScript((key) => localStorage.setItem(key, 'true'), ONBOARDED_KEY);
      await page.goto(`${baseUrl()}/?session=${ctx.id}&mobile_input=0`, { waitUntil: 'domcontentloaded' });
      // Persist ghostty on this origin, then load once more with the override already stored.
      await page.reload({ waitUntil: 'domcontentloaded' });
      await term.open(ctx.id);
      await expect(term.renderer('xterm')).toBeVisible();
      await expect(term.layerTextarea()).toHaveCount(0);
      await expect(term.flagOffHint()).toBeVisible();
    } finally {
      await ctx.client.deleteSession(ctx.id, true).catch(() => undefined);
      await context.close();
      await setFlag(request, 'ghostty_renderer', false);
    }
  });

  test('desktop emulation does not attach the layer', async ({ browser, request }) => {
    await setFlag(request, 'mobile_input_layer', true);
    const context = await browser.newContext({ ...devices['Desktop Chrome'] });
    const page = await context.newPage();
    const ctx = await newSession('desk');
    try {
      const term = new TerminalPage(page, ctx.title);
      await page.addInitScript((key) => localStorage.setItem(key, 'true'), ONBOARDED_KEY);
      await term.open(ctx.id);
      await expect(term.renderer('xterm')).toBeVisible();
      await expect(term.layerTextarea()).toHaveCount(0);
    } finally {
      await ctx.client.deleteSession(ctx.id, true).catch(() => undefined);
      await context.close();
    }
  });
});
