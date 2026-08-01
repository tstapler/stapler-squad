// @feature session:create, repo-path-picker-parity
import { test, expect } from '@playwright/test';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';

async function openInCreationMode(page: import('@playwright/test').Page) {
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });
  const radiogroup = page.getByRole('radiogroup', { name: 'Session type' });
  // Ctrl+Shift+K opens directly in creation mode (OmnibarContext.tsx), but the
  // panel briefly resets to discovery mode shortly after opening on this test
  // server (an unrelated, pre-existing race — Omnibar.tsx re-runs its
  // input-detection effect once aliases/workflows finish their per-open
  // refetch, and since the search input is untouched/empty, that effect's
  // `else` branch unconditionally dispatches reset_to_discovery). Retrying the
  // shortcut re-enters creation mode; this loop is a test-side workaround, not
  // a claim that the underlying race is fixed.
  for (let attempt = 0; attempt < 8; attempt++) {
    await page.keyboard.press('Control+Shift+K');
    try {
      await expect(radiogroup).toBeVisible({ timeout: 1000 });
      return;
    } catch {
      // retry
    }
  }
  await expect(radiogroup).toBeVisible({ timeout: 5000 });
}

// "New Project" and "Existing branch" are both ADVANCED_TYPES in
// OmnibarCreationPanel.tsx (SessionTypeRadioGroup), collapsed behind a
// "▾ More" toggle by default since the panel's default sessionType
// ("new_worktree") is a primary type. Expand it before selecting either mode.
async function selectSessionType(page: import('@playwright/test').Page, label: string) {
  const radio = page.getByRole('radio', { name: label, exact: true });
  if (!(await radio.isVisible().catch(() => false))) {
    await page.getByRole('button', { name: /More/ }).click();
  }
  await expect(radio).toBeVisible({ timeout: 3000 });
  await radio.click();
}

// ── Test data setup ──────────────────────────────────────────────────────────
// A single real session, seeded once via a direct RPC call (bypassing the UI,
// per the createSessionViaApi pattern in unfinished-work.spec.ts), whose path
// becomes the history entry both RepoPathInput fields under test should surface.
// Because Playwright is configured `workers: 1` / `fullyParallel: false`
// (tests/e2e/playwright.config.ts), no other test can create a session with a
// later `updatedAt` between this beforeAll and the tests that depend on it —
// this session is therefore always the single most-recent entry in
// `selectActiveSessionsSortedByUpdatedAt`, guaranteeing it lands inside
// `useSessionRepoPaths`' MAX_HISTORY=5 window regardless of run order or how
// many other sessions already exist in this test server instance.

let seededPath: string;

async function rpc(service: string, method: string, body: object): Promise<Response> {
  return fetch(`${BASE_URL}/api/${service}/${method}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

async function createSessionViaApi(dirPath: string, title: string): Promise<string> {
  const res = await rpc('session.v1.SessionService', 'CreateSession', {
    title,
    path: dirPath,
    program: 'echo', // harmless no-op; won't block the test
    sessionType: 1, // SESSION_TYPE_DIRECTORY
    skipDefaults: true,
  });
  if (!res.ok) throw new Error(`CreateSession failed: ${res.status} ${await res.text()}`);
  const data = await res.json();
  return data.session?.id ?? '';
}

test.beforeAll(async () => {
  seededPath = fs.mkdtempSync(path.join(os.tmpdir(), 'sq-e2e-rpp-'));
  await createSessionViaApi(seededPath, `e2e-rpp-seed-${Date.now()}`);
});

test.afterAll(() => {
  try {
    fs.rmSync(seededPath, { recursive: true, force: true });
  } catch {
    /* ignore */
  }
});

// Shape returned by the page.evaluate() geometry probe shared by the 390x844
// viewport tests. Deliberately not Playwright's boundingBox() — boundingBox()
// reports an element's own box regardless of visual clipping by an
// `overflow: hidden` ancestor (the Omnibar's modal panel), which would produce
// a false-positive pass on exactly the clipping bug this check exists to
// catch. getBoundingClientRect() computed in-page reflects actual rendered
// (and potentially clipped) geometry.
interface DropdownGeometry {
  dropdown?: { top: number; right: number; bottom: number; left: number; width: number; height: number };
  modal?: { top: number; right: number; bottom: number; left: number; width: number; height: number };
  rowRects: { top: number; right: number; bottom: number; left: number; width: number; height: number }[];
}

async function readDropdownGeometry(page: import('@playwright/test').Page): Promise<DropdownGeometry> {
  return page.evaluate(() => {
    // The Omnibar's outer overlay carries role="dialog" (full-viewport backdrop,
    // no overflow clipping of its own); its immediate child is the actual modal
    // panel (`overflow: hidden`) that can clip an absolutely-positioned dropdown.
    // Structural lookup (firstElementChild) avoids depending on a CSS class name.
    const overlayEl = document.querySelector('[role="dialog"][aria-labelledby="omnibar-title"]');
    const modalEl = overlayEl?.firstElementChild as HTMLElement | null;
    const dropdownEl = document.querySelector('[data-testid="path-completion-dropdown"]');
    const rows = dropdownEl ? Array.from(dropdownEl.querySelectorAll('[role="option"]')) : [];
    return {
      dropdown: dropdownEl?.getBoundingClientRect().toJSON(),
      modal: modalEl?.getBoundingClientRect().toJSON(),
      rowRects: rows.map((r) => r.getBoundingClientRect().toJSON()),
    };
  });
}

test.describe('repo path picker parity — Parent Directory field', () => {
  test('T-E2E-RPP-001: history suggestion appears and selecting it fills the field in 2 actions', async ({ page }) => {
    await openInCreationMode(page);
    await selectSessionType(page, 'New Project');

    const parentDir = page.getByLabel('Parent Directory *');
    await parentDir.focus(); // action 1

    const listbox = page.getByRole('listbox');
    await expect(listbox).toBeVisible();
    const option = page.getByRole('option', { name: seededPath });
    await expect(option).toBeVisible();

    await option.click(); // action 2

    await expect(parentDir).toHaveValue(seededPath);
    await expect(listbox).not.toBeVisible();
  });

  test('T-E2E-RPP-003: typing a brand-new path is preserved verbatim, no dropdown override', async ({ page }) => {
    await openInCreationMode(page);
    await selectSessionType(page, 'New Project');

    const parentDir = page.getByLabel('Parent Directory *');
    const typed = `/tmp/brand-new-e2e-parent-${Date.now()}`;
    await parentDir.fill(typed);

    await expect(parentDir).toHaveValue(typed);
    await expect(page.getByRole('listbox')).not.toBeVisible();
  });

  test('T-E2E-RPP-005: Escape with dropdown open closes only the dropdown', async ({ page }) => {
    await openInCreationMode(page);
    await selectSessionType(page, 'New Project');

    const parentDir = page.getByLabel('Parent Directory *');
    await parentDir.focus();
    await expect(page.getByRole('listbox')).toBeVisible();

    await page.keyboard.press('Escape');

    await expect(page.getByRole('listbox')).not.toBeVisible();
    // Panel didn't reset: mode is still selected, panel still open.
    await expect(page.getByRole('radio', { name: 'New Project' })).toHaveAttribute('aria-checked', 'true');
    await expect(page.getByRole('radiogroup', { name: 'Session type' })).toBeVisible();
  });

  test('T-E2E-RPP-006: second Escape (dropdown already closed) falls through to the pre-existing reset-to-discovery behavior', async ({ page }) => {
    await openInCreationMode(page);
    await selectSessionType(page, 'New Project');

    const parentDir = page.getByLabel('Parent Directory *');
    await parentDir.focus();
    await expect(page.getByRole('listbox')).toBeVisible();

    await page.keyboard.press('Escape'); // 1st: closes dropdown only
    await expect(page.getByRole('listbox')).not.toBeVisible();

    await page.keyboard.press('Escape'); // 2nd: dropdown already closed -> resets panel to discovery

    await expect(page.getByRole('radiogroup', { name: 'Session type' })).not.toBeVisible();
  });

  test('T-E2E-RPP-007: dropdown fits at 390x844 with no overflow/clip, rows clear 24x24 tap target', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await openInCreationMode(page);
    await selectSessionType(page, 'New Project');

    await page.getByLabel('Parent Directory *').focus();
    await expect(page.getByRole('listbox')).toBeVisible();

    const rects = await readDropdownGeometry(page);

    expect(rects.dropdown).toBeTruthy();
    expect(rects.modal).toBeTruthy();
    expect(rects.rowRects.length).toBeGreaterThan(0);

    // No horizontal overflow at 390px viewport width.
    expect(rects.dropdown!.right).toBeLessThanOrEqual(390);
    // No vertical clipping by the modal panel's own overflow:hidden.
    expect(rects.dropdown!.bottom).toBeLessThanOrEqual(rects.modal!.bottom);

    // WCAG 2.2 AA SC 2.5.8 minimum tap target size (UX-AC-12 / R5).
    for (const row of rects.rowRects) {
      expect(row.height).toBeGreaterThanOrEqual(24);
      expect(row.width).toBeGreaterThanOrEqual(24);
    }
  });
});

test.describe('repo path picker parity — Existing Worktree Path fallback field', () => {
  test('T-E2E-RPP-002: history suggestion appears and selecting it fills the field in 2 actions', async ({ page }) => {
    await openInCreationMode(page);
    await selectSessionType(page, 'Existing branch');

    // No repo path has been typed into the omnibar's own source input yet, so
    // worktree auto-discovery is never enabled (useWorktreeSuggestions requires
    // a non-empty repoPathForWorktrees) — the fallback RepoPathInput renders
    // immediately, matching the existing "shows worktree path input when
    // existing worktree is selected" test's assumption in
    // session-create-existing-worktree.spec.ts.
    const worktreeInput = page.getByLabel('Existing Worktree Path *');
    await expect(worktreeInput).toBeVisible();

    await worktreeInput.focus(); // action 1

    const listbox = page.getByRole('listbox');
    await expect(listbox).toBeVisible();
    const option = page.getByRole('option', { name: seededPath });
    await expect(option).toBeVisible();

    await option.click(); // action 2

    await expect(worktreeInput).toHaveValue(seededPath);
    await expect(listbox).not.toBeVisible();
  });

  test('T-E2E-RPP-004: typing a brand-new path is preserved verbatim, no dropdown override', async ({ page }) => {
    await openInCreationMode(page);
    await selectSessionType(page, 'Existing branch');

    const worktreeInput = page.getByLabel('Existing Worktree Path *');
    const typed = `/tmp/brand-new-e2e-worktree-${Date.now()}`;
    await worktreeInput.fill(typed);

    await expect(worktreeInput).toHaveValue(typed);
    await expect(page.getByRole('listbox')).not.toBeVisible();
  });

  test('T-E2E-RPP-008: dropdown fits at 390x844 with no overflow/clip, rows clear 24x24 tap target', async ({ page }) => {
    await page.setViewportSize({ width: 390, height: 844 });
    await openInCreationMode(page);
    await selectSessionType(page, 'Existing branch');

    const worktreeInput = page.getByLabel('Existing Worktree Path *');
    await expect(worktreeInput).toBeVisible();
    await worktreeInput.focus();
    await expect(page.getByRole('listbox')).toBeVisible();

    const rects = await readDropdownGeometry(page);

    expect(rects.dropdown).toBeTruthy();
    expect(rects.modal).toBeTruthy();
    expect(rects.rowRects.length).toBeGreaterThan(0);

    expect(rects.dropdown!.right).toBeLessThanOrEqual(390);
    expect(rects.dropdown!.bottom).toBeLessThanOrEqual(rects.modal!.bottom);

    for (const row of rects.rowRects) {
      expect(row.height).toBeGreaterThanOrEqual(24);
      expect(row.width).toBeGreaterThanOrEqual(24);
    }
  });
});
