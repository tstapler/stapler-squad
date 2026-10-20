/**
 * Fulfils the hidden-session reads the Background segment makes (ListSessions{hiddenOnly} and GetSession),
 * because the shared test-mode instance cannot create `Hidden: true` sessions (see hidden-session-view.spec.ts).
 * Everything else, including a main-list ListSessions, reaches the real server.
 */
import { expect, type APIRequestContext, type Page } from '@playwright/test';
import { sendNotification } from './NotificationPanel';

export interface HiddenFixture {
  id: string;
  title: string;
  status: string;
  updatedAt?: string;
  backgroundOutcome?: 'completed' | 'failed' | 'unknown';
}

export const hidden = (id: string, status = 'SESSION_STATUS_ACTIVE', updatedAt = new Date().toISOString()): HiddenFixture => ({
  id,
  title: id,
  status,
  updatedAt,
});

export interface Calls {
  hiddenPolls: number;
  mainListIncludesHidden: number;
}

/** Serves ListSessions{hiddenOnly} from `current()`; everything else is the real server. */
export async function fakeHiddenSessions(page: Page, current: () => HiddenFixture[] | 'error'): Promise<Calls> {
  const calls: Calls = { hiddenPolls: 0, mainListIncludesHidden: 0 };
  await page.route('**/api/session.v1.SessionService/ListSessions', async (route) => {
    const body = (route.request().postDataJSON() ?? {}) as { hiddenOnly?: boolean; includeHidden?: boolean };
    if (!body.hiddenOnly) {
      if (body.includeHidden) calls.mainListIncludesHidden += 1;
      return route.continue();
    }
    calls.hiddenPolls += 1;
    const result = current();
    if (result === 'error') {
      return route.fulfill({ status: 503, contentType: 'application/json', body: JSON.stringify({ code: 'unavailable', message: 'down' }) });
    }
    const sessions = result.map((s) => ({ ...s, program: 'claude', path: '/tmp/e2e-background', branch: 'main', hidden: true }));
    return route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ sessions }) });
  });
  await page.route('**/api/session.v1.SessionService/GetSession', async (route) => {
    const { id } = (route.request().postDataJSON() ?? {}) as { id?: string };
    const session = current() === 'error' ? undefined : (current() as HiddenFixture[]).find((s) => s.id === id);
    if (!session) {
      return route.fulfill({ status: 404, contentType: 'application/json', body: JSON.stringify({ code: 'not_found', message: 'gone' }) });
    }
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ session: { ...session, program: 'claude', path: '/tmp/e2e-background', branch: 'main', hidden: true } }),
    });
  });
  return calls;
}

async function storedFor(request: APIRequestContext, baseUrl: string, sessionId: string): Promise<number> {
  const response = await request.post(`${baseUrl}/api/session.v1.SessionService/GetNotificationHistory`, {
    headers: { 'Content-Type': 'application/json' },
    data: { limit: 500 },
  });
  const body = (await response.json()) as { notifications?: { sessionId?: string }[] };
  return (body.notifications ?? []).filter((n) => n.sessionId === sessionId).length;
}

/** A real ERROR notification for the session, waited on until the history subscriber has stored it. */
export async function seedFailureAt(request: APIRequestContext, baseUrl: string, sessionId: string, title = 'Review failed'): Promise<void> {
  await sendNotification(request, baseUrl, { sessionId, type: 'ERROR', title, message: '3 tests failing' });
  // The subscriber coalesces for ~500ms before persisting.
  await expect.poll(() => storedFor(request, baseUrl, sessionId), { timeout: 10_000 }).toBeGreaterThan(0);
}
