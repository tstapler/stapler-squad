// @feature session:fanout, session:fanout-pick, parallel-fanout
/**
 * Parallel fan-out lifecycle: a 2-variant group renders side by side in the comparison view,
 * picking a winner calls PickFanoutWinner, and the other variant shows as archived.
 *
 * The shared test-mode instance has no agent/git fixture to run two real variants, so the group's
 * members, diffs and the pick are fulfilled by route stubs (same approach as background-activity.spec.ts).
 * The real merge + loser-archive path is covered by Go tests against real git worktrees
 * (server/services/session_service_fanout_test.go); the cap and unknown-group errors hit the real server here.
 */
import { test, expect, type Page } from '@playwright/test';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const GROUP = 'e2e-group';
const RPC = '**/api/session.v1.SessionService';

interface Member {
  id: string;
  title: string;
  tags: string[];
  status: string;
  archivedAt?: string;
}

const member = (n: number): Member => ({
  id: `fanout-${n}`,
  title: `Task [variant ${n}]`,
  tags: [`fanout:${GROUP}`, `fanout-variant:${n}`],
  status: 'SESSION_STATUS_PAUSED',
});

async function stubGroup(page: Page, members: Member[]): Promise<{ picks: string[] }> {
  const picks: string[] = [];
  await page.route(`${RPC}/ListSessions`, (route) =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ sessions: members }) }),
  );
  await page.route(`${RPC}/GetSessionDiff`, (route) =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ diffStats: { added: 1, removed: 0, content: '+hello' } }) }),
  );
  await page.route(`${RPC}/PickFanoutWinner`, (route) => {
    const { winnerSessionId } = route.request().postDataJSON() as { winnerSessionId: string };
    picks.push(winnerSessionId);
    for (const m of members) {
      if (m.id !== winnerSessionId) m.archivedAt = new Date().toISOString();
    }
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ baseBranch: 'main', archivedSessionIds: members.filter((m) => m.id !== winnerSessionId).map((m) => m.id) }),
    });
  });
  return { picks };
}

test.describe('Parallel fan-out lifecycle', () => {
  test('comparison view renders both variants, pick winner archives the other', async ({ page }) => {
    const members = [member(1), member(2)];
    const { picks } = await stubGroup(page, members);

    await page.goto(`${BASE_URL}/fanout?groupId=${GROUP}`, { waitUntil: 'domcontentloaded' });

    await expect(page.getByTestId('fanout-column-fanout-1')).toBeVisible();
    await expect(page.getByTestId('fanout-column-fanout-2')).toBeVisible();
    await expect(page.getByTestId('fanout-status-fanout-1')).toHaveText(/paused/i);

    await page.getByTestId('fanout-pick-fanout-1').click();
    await page.getByTestId('fanout-confirm-fanout-1').click();

    await expect(page.getByTestId('fanout-picked')).toBeVisible();
    expect(picks).toEqual(['fanout-1']);
    await expect(page.getByTestId('fanout-status-fanout-2')).toHaveText(/archived/i, { timeout: 15000 });
  });

  test('server rejects more than 5 variants with a structured error', async ({ request }) => {
    const res = await request.post(`${BASE_URL}/api/session.v1.SessionService/FanOutSession`, {
      data: { base: { title: 'cap', path: '/tmp' }, count: 6 },
    });
    expect(res.status()).toBe(400);
    expect((await res.json()).code).toBe('invalid_argument');
  });

  test('server returns a structured not_found for an unknown fan-out group', async ({ request }) => {
    const res = await request.post(`${BASE_URL}/api/session.v1.SessionService/PickFanoutWinner`, {
      data: { groupId: 'no-such-group', winnerSessionId: 'x' },
    });
    expect(res.status()).toBe(404);
    expect((await res.json()).code).toBe('not_found');
  });
});
