// @feature session-list
import { test, expect } from '@playwright/test';
import * as fs from 'fs';
import * as path from 'path';

// CI encoding of gate S2's overlay and CSS-dependant criteria (project_plans/web-app-bun-build/research/spike-results.md,
// section S2-overlay and S2-csstoken), per Story 5.6's AC. Run only under WEB_BUILD=bun (playwright.dev-mode.config.ts's
// global-setup spawns the frontend via scripts/dev-stack/launch.ts's frontendSpawnPlan, which reads WEB_BUILD): the CI
// step and Story 5.11's web-build-smoke-bun job set it before invoking this config; run by hand the same way:
//   WEB_BUILD=bun npx playwright test --config=tests/e2e/playwright.dev-mode.config.ts tests/e2e/dev-mode-hmr.spec.ts
//
// Edits a real source file and restores it even on failure — never leaves the worktree dirty.
const TARGET = path.resolve(__dirname, '../../web-app/src/app/test/terminal-stress/page.tsx');
const THEME_FILE = path.resolve(__dirname, '../../web-app/src/styles/theme.css.ts');

async function withEdit<T>(file: string, mutate: (s: string) => string, fn: () => Promise<T>): Promise<T> {
  const original = fs.readFileSync(file, 'utf8');
  try {
    fs.writeFileSync(file, mutate(original));
    return await fn();
  } finally {
    fs.writeFileSync(file, original);
  }
}

test.describe('dev-mode-hmr', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/test/terminal-stress', { waitUntil: 'load' });
    await page.waitForSelector('[data-testid="preset-selector"]', { timeout: 15000 });
  });

  test('a syntax error shows the dev-server overlay naming the file, and it clears when fixed', async ({ page }) => {
    await withEdit(TARGET, (s) => s.replace("'use client';", "'use client\nconst __broken = ("), async () => {
      await expect
        .poll(async () => page.evaluate(() => document.querySelector('bun-hmr')?.shadowRoot?.textContent ?? ''), { timeout: 5000 })
        .toContain('terminal-stress');
    });
    // Reverted by withEdit's finally; the overlay element stays in the DOM (its last error text persists in the
    // shadow root) but is hidden via style.display, so "cleared" means absent OR display:none, not empty text.
    await expect
      .poll(
        () =>
          page.evaluate(() => {
            const el = document.querySelector('bun-hmr') as HTMLElement | null;
            return !el || el.style.display === 'none';
          }),
        { timeout: 8000 },
      )
      .toBe(true);
  });

  test('a render-time throw is caught by the ErrorBoundary, and the app recovers after reload', async ({ page }) => {
    await withEdit(
      TARGET,
      (s) => s.replace('return (\n    <div', 'if (true) { throw new Error("s2-runtime-canary"); }\n  return (\n    <div'),
      async () => {
        await expect(page.getByText(/something went wrong/i)).toBeVisible({ timeout: 5000 });
      },
    );
    await page.reload({ waitUntil: 'load' });
    await expect(page.locator('[data-testid="preset-selector"]')).toBeVisible({ timeout: 10000 });
  });

  test('editing a shared token file (theme.css.ts) updates a dependent element within 2s, no manual reload', async ({ page }) => {
    // Encodes gate S2's literal AC (spike-results.md, section S2-csstoken): "the computed colour on a dependent
    // element changes within 2s with no manual reload". A standalone `bun build/dev.ts` (the S2 measurement)
    // showed this with zero full-page reloads at all; through this harness's full dev-stack (real backend, real
    // React Router) one spontaneous `load` event was observed around the edit whose exact trigger is not yet
    // isolated (not this app's own reload code — see dev-server.ts/dev.ts, which only reload via its SSE hub,
    // never created when HMR is on) — recorded as an annotation, not failed, since the AC itself only requires
    // no *manual* reload and the 2s budget, both of which hold regardless.
    await page.goto('/settings', { waitUntil: 'load' });
    await page.waitForSelector('h1', { timeout: 15000 });
    // The dev server's own first-connect HMR handshake can briefly destroy the evaluate context right after
    // the initial load fires — retry; it settles well before gate S2's 2s dependant-update budget starts.
    const sample = async (): Promise<string> => {
      for (let attempt = 0; attempt < 10; attempt++) {
        try {
          return await page.evaluate(() => getComputedStyle(document.querySelector('h1')!).color.trim());
        } catch {
          await new Promise((r) => setTimeout(r, 300));
        }
      }
      throw new Error('sample(): <h1> unreachable after retries');
    };
    const before = await sample();
    let reloadCount = 0;
    page.on('load', () => {
      reloadCount++;
    });
    await withEdit(THEME_FILE, (s) => s.replace('textPrimary: "#e2e8f0",', 'textPrimary: "#ff00ff",'), async () => {
      await expect.poll(sample, { timeout: 2000 }).not.toBe(before);
    });
    test.info().annotations.push({ type: 'reload-count', description: String(reloadCount) });
  });
});
