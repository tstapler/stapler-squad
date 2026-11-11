// @feature website-demos
import type { Page } from '@playwright/test';

/** The dashboard's session list is pane/window based; its rows carry this test id (there are no cards). */
export const ROW = '[data-testid="session-row"]';
export const SEARCH = 'input[aria-label="Search sessions"]';
// The aria-label carries a "(keyboard: G)" suffix, so match the prefix.
export const GROUP_BY = 'select[aria-label^="Group sessions by"]';

/** The grouping, status and approval filters sit behind the Filters toggle. */
export async function openFilters(page: Page): Promise<void> {
  await page.getByRole('button', { name: /^Filters/ }).click();
  await page.locator(GROUP_BY).waitFor();
}

export async function groupBy(page: Page, value: 'tag' | 'branch'): Promise<void> {
  await openFilters(page);
  await page.locator(GROUP_BY).selectOption(value);
}

/** On a phone the window shows one pane at a time; the session list is the "Sessions" pane tab. */
export const SESSIONS_PANE_TAB = 'role=tab[name="Sessions"]';

export async function showSessionList(page: Page): Promise<void> {
  await page.locator(SESSIONS_PANE_TAB).tap();
  await page.locator(ROW).first().waitFor();
}

/** The terminal tab paints "Initializing..." until its stream connects; capture only the live view. */
export async function terminalConnected(page: Page): Promise<void> {
  await page.getByText('Connected', { exact: true }).first().waitFor({ timeout: 30_000 });
}
