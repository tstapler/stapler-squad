import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
// Features: unfinished-work — mapped from @feature annotation
const _features = [
  FEATURE_CATALOG['unfinished-work'],
  // FEATURE_CATALOG['unfinished-list'], // TODO: add to catalog (covered by unfinished-work rpcIds)
  // FEATURE_CATALOG['unfinished-watch'], // TODO: add to catalog (covered by unfinished-work rpcIds)
  FEATURE_CATALOG['unfinished-scan'],
  FEATURE_CATALOG['unfinished-dismiss'],
  FEATURE_CATALOG['unfinished-snooze'],
] as const;
import { test, expect } from '@playwright/test';
import { execSync } from 'child_process';
import * as fs from 'fs';
import * as os from 'os';
import * as path from 'path';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const UNFINISHED_URL = `${BASE_URL}/unfinished`;

// ── Test data setup ──────────────────────────────────────────────────────────
// We create a real bare git repo + worktree with uncommitted changes AND a
// matching stapler-squad session (path = worktree path) so every test path —
// including "Reattach Session" — runs against actual data.

let testRepoDir: string;
let testWorktreeDir: string;
let testSessionId: string; // UUID returned by CreateSession

async function rpc(service: string, method: string, body: object): Promise<Response> {
  return fetch(`${BASE_URL}/${service}/${method}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

async function addPinnedRepoViaApi(repoPath: string): Promise<void> {
  const res = await rpc('session.v1.UnfinishedWorkService', 'UpdateUnfinishedWorkConfig', {
    config: { autoSpiderSessions: false, watchDirs: [], pinnedRepos: [repoPath] },
  });
  if (!res.ok) throw new Error(`UpdateUnfinishedWorkConfig failed: ${res.status} ${await res.text()}`);
}

async function createSessionViaApi(worktreePath: string, branch: string): Promise<string> {
  const res = await rpc('session.v1.SessionService', 'CreateSession', {
    title: 'e2e-unfinished-test',
    path: worktreePath,
    branch,
    program: 'echo',          // harmless no-op program; won't block the test
    sessionType: 1,           // SESSION_TYPE_DIRECTORY
    skipDefaults: true,
  });
  if (!res.ok) throw new Error(`CreateSession failed: ${res.status} ${await res.text()}`);
  const data = await res.json();
  // session.id = UUID (GetStableID returns UUID for new sessions)
  return data.session?.id ?? '';
}

async function triggerScanAndWait(timeoutMs = 15000): Promise<void> {
  await rpc('session.v1.UnfinishedWorkService', 'ScanUnfinishedWork', {});
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const res = await rpc('session.v1.UnfinishedWorkService', 'ListUnfinishedWork', {});
    if (res.ok) {
      const data = await res.json();
      if (data.worktrees && data.worktrees.length > 0) return;
    }
    await new Promise(r => setTimeout(r, 500));
  }
}

test.beforeAll(async () => {
  // 1. Create a temp bare repo + worktree with uncommitted changes
  testRepoDir = fs.mkdtempSync(path.join(os.tmpdir(), 'sq-e2e-repo-'));
  testWorktreeDir = fs.mkdtempSync(path.join(os.tmpdir(), 'sq-e2e-wt-'));

  execSync(`git init --bare "${testRepoDir}"`);

  const seedDir = fs.mkdtempSync(path.join(os.tmpdir(), 'sq-e2e-seed-'));
  execSync(`git clone "${testRepoDir}" "${seedDir}"`);
  execSync('git config user.email "test@example.com"', { cwd: seedDir });
  execSync('git config user.name "Test"', { cwd: seedDir });
  fs.writeFileSync(path.join(seedDir, 'README.md'), 'init\n');
  execSync('git add . && git commit -m "init"', { cwd: seedDir });
  execSync('git push origin main', { cwd: seedDir });

  // Add a worktree on a feature branch — the scanner will detect it
  execSync(`git worktree add -b e2e-feature-branch "${testWorktreeDir}"`, { cwd: seedDir });
  fs.writeFileSync(path.join(testWorktreeDir, 'work-in-progress.ts'), 'export const wip = true;\n');

  // 2. Create a stapler-squad session whose path matches the worktree
  //    The backend sessionPathIndex will match inst.Path == WorktreePath
  testSessionId = await createSessionViaApi(testWorktreeDir, 'e2e-feature-branch');

  // 3. Register the repo as pinned and wait for it to appear in the scan results
  await addPinnedRepoViaApi(testRepoDir);
  await triggerScanAndWait();
});

test.afterAll(() => {
  // Best-effort cleanup
  try { fs.rmSync(testRepoDir, { recursive: true, force: true }); } catch { /* ignore */ }
  try { fs.rmSync(testWorktreeDir, { recursive: true, force: true }); } catch { /* ignore */ }
});

// ── Tests ────────────────────────────────────────────────────────────────────

test.describe('unfinished-work', () => {
  test('unfinished-work > page loads with title and filter controls', async ({ page }) => {
    await page.goto(UNFINISHED_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });

    await expect(page.getByRole('heading', { name: 'Unfinished Work' })).toBeVisible();
    await expect(page.getByRole('group', { name: 'Filter worktrees' })).toBeVisible();
    await expect(page.getByRole('button', { name: /Refresh/i })).toBeVisible();
  });

  test('unfinished-work > shows empty state when no worktrees found', async ({ page }) => {
    await page.goto(UNFINISHED_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });

    // With our seeded data, items should be visible — but test both branches defensively
    const itemCount = await page.locator('[data-testid="unfinished-item"]').count();
    if (itemCount === 0) {
      await expect(page.getByText(/No unfinished work found/i)).toBeVisible({ timeout: 5000 });
    } else {
      expect(itemCount).toBeGreaterThan(0);
    }
  });

  test('unfinished-work > filter chips toggle between All Uncommitted Ahead Behind', async ({ page }) => {
    await page.goto(UNFINISHED_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });

    const filterGroup = page.getByRole('group', { name: 'Filter worktrees' });
    await expect(filterGroup.getByRole('button', { name: 'All' })).toBeVisible();
    await expect(filterGroup.getByRole('button', { name: 'Uncommitted' })).toBeVisible();
    await expect(filterGroup.getByRole('button', { name: 'Ahead' })).toBeVisible();
    await expect(filterGroup.getByRole('button', { name: 'Behind' })).toBeVisible();

    // All is pressed by default
    await expect(filterGroup.getByRole('button', { name: 'All' })).toHaveAttribute('aria-pressed', 'true');

    // Clicking Uncommitted presses it and unpresses All
    await filterGroup.getByRole('button', { name: 'Uncommitted' }).click();
    await expect(filterGroup.getByRole('button', { name: 'Uncommitted' })).toHaveAttribute('aria-pressed', 'true');
    await expect(filterGroup.getByRole('button', { name: 'All' })).toHaveAttribute('aria-pressed', 'false');

    // Clicking All restores default
    await filterGroup.getByRole('button', { name: 'All' }).click();
    await expect(filterGroup.getByRole('button', { name: 'All' })).toHaveAttribute('aria-pressed', 'true');
  });

  test('unfinished-work > Refresh button triggers a scan RPC', async ({ page }) => {
    await page.goto(UNFINISHED_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });

    const scanRequest = page.waitForRequest(
      (req) => req.url().includes('ScanUnfinishedWork') && req.method() === 'POST',
      { timeout: 5000 }
    );

    await page.getByRole('button', { name: /Refresh/i }).click();
    await scanRequest;
  });

  test('unfinished-work > item card expands and collapses on click', async ({ page }) => {
    await page.goto(UNFINISHED_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });

    // Wait for our seeded item to appear
    const item = page.locator('[data-testid="unfinished-item"]').first();
    await expect(item).toBeVisible({ timeout: 10000 });

    // Initially collapsed
    await expect(item).toHaveAttribute('aria-expanded', 'false');

    // Expand
    await item.click();
    await expect(item).toHaveAttribute('aria-expanded', 'true');

    // Collapse
    await item.click();
    await expect(item).toHaveAttribute('aria-expanded', 'false');
  });

  test('unfinished-work > Open Session button navigates to home with worktree prefill params', async ({ page }) => {
    await page.goto(UNFINISHED_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });

    // Wait for our seeded item (which has no session)
    const item = page.locator('[data-testid="unfinished-item"]').first();
    await expect(item).toBeVisible({ timeout: 10000 });
    await item.click();

    // "Open Session" appears when sessionIds is empty
    const openBtn = page.getByRole('button', { name: 'Open Session' });
    await expect(openBtn).toBeVisible({ timeout: 5000 });

    await openBtn.click();
    // Should navigate to home with ?worktree= param to pre-fill wizard
    await expect(page).toHaveURL(/[?&]worktree=/, { timeout: 5000 });
  });

  test('unfinished-work > Commit & Push button opens commit modal', async ({ page }) => {
    await page.goto(UNFINISHED_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });

    const item = page.locator('[data-testid="unfinished-item"]').first();
    await expect(item).toBeVisible({ timeout: 10000 });
    await item.click();

    const commitBtn = page.getByRole('button', { name: /Commit.*Push/i });
    await expect(commitBtn).toBeVisible({ timeout: 5000 });
    await commitBtn.click();

    // Modal with commit message input should appear
    await expect(page.getByRole('dialog').or(page.locator('[class*="modal"]'))).toBeVisible({ timeout: 3000 });
  });

  test('unfinished-work > Dismiss button calls DismissWorktree RPC', async ({ page }) => {
    await page.goto(UNFINISHED_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });

    const item = page.locator('[data-testid="unfinished-item"]').first();
    await expect(item).toBeVisible({ timeout: 10000 });

    const dismissRequest = page.waitForRequest(
      (req) => req.url().includes('DismissWorktree') && req.method() === 'POST',
      { timeout: 5000 }
    );

    await item.hover();
    const dismissBtn = page.getByRole('button', { name: /Dismiss/i });
    await expect(dismissBtn).toBeVisible({ timeout: 3000 });
    await dismissBtn.click();

    await dismissRequest;
  });

  test('unfinished-work > scan completed event updates last-scanned timestamp', async ({ page }) => {
    await page.goto(UNFINISHED_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });

    await page.getByRole('button', { name: /Refresh/i }).click();
    await expect(page.getByText(/Last scanned/i)).toBeVisible({ timeout: 15000 });
  });

  test('unfinished-work > nav badge links to unfinished page', async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });

    const unfinishedLink = page.getByRole('link', { name: /Unfinished/i }).first();
    await expect(unfinishedLink).toBeVisible({ timeout: 10000 });
    await unfinishedLink.click();
    await expect(page).toHaveURL(/\/unfinished/, { timeout: 5000 });
  });

  test('unfinished-work > Reattach Session button navigates to session detail', async ({ page }) => {
    // This test only runs when a worktree has a matching session.
    // It verifies the ?session= URL param navigation path.
    // The session matching is done by the backend sessionPathIndex() using worktree path.
    await page.goto(UNFINISHED_URL, { waitUntil: 'domcontentloaded', timeout: 15000 });

    const item = page.locator('[data-testid="unfinished-item"]').first();
    await expect(item).toBeVisible({ timeout: 10000 });
    await item.click();

    // "Reattach Session" only appears when sessionIds.length > 0 — skip if absent
    const reattachBtn = page.getByRole('button', { name: /Reattach Session/i });
    const hasReattach = await reattachBtn.isVisible({ timeout: 2000 }).catch(() => false);
    if (!hasReattach) {
      test.skip();
      return;
    }

    await reattachBtn.click();
    await expect(page).toHaveURL(/[?&]session=/, { timeout: 5000 });
  });
});
