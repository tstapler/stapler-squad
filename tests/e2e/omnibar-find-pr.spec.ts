// @feature omnibar:detect
import { test, expect } from '@playwright/test';
import { OmnibarFindPrPage, PR_SEARCH_URL } from './pages/OmnibarFindPrPage';
import { ToastDeck } from './pages/NotificationPanel';

test.describe('omnibar find sessions for this PR (desktop)', () => {
  test('omnibar should offer Find sessions for acme/app#12 and select via keyboard', async ({ page }) => {
    const omnibar = new OmnibarFindPrPage(page);
    await omnibar.mockLookup([
      { sessionId: 'u1', title: 'first', source: 'session_pr' },
      { sessionId: 'u2', title: 'second', source: 'artifact' },
    ]);
    await omnibar.open();
    await omnibar.paste();
    await expect(omnibar.action).toHaveAccessibleName(/Find sessions for acme\/app#12/);

    // ArrowDown from the input reaches the action and Enter runs it; Enter on the
    // action must not also submit the session-creation form.
    await omnibar.input.press('ArrowDown');
    await expect(omnibar.action).toBeFocused();
    await omnibar.action.press('Enter');

    // Several matches go to the artifacts list; the page itself lands in wave 4, so only the target is asserted.
    await page.waitForURL(PR_SEARCH_URL, { timeout: 10000 });
    expect(omnibar.lookupRequests).toHaveLength(1);
    expect(JSON.parse(omnibar.lookupRequests[0]).pr).toMatchObject({ owner: 'acme', repo: 'app', number: 12 });
  });

  test('omnibar should show inline error and keep other actions when lookup fails', async ({ page }) => {
    const omnibar = new OmnibarFindPrPage(page);
    await omnibar.mockLookup(null);
    await omnibar.open();
    await omnibar.paste();
    await omnibar.action.click();

    await expect(omnibar.error).toHaveText(/Couldn't look up sessions for this PR/);
    // The omnibar stays open and its other actions still work.
    await expect(page.getByTestId('omnibar')).toBeVisible();
    await expect(omnibar.input).toBeEditable();
    await expect(omnibar.retry).toBeVisible();

    // Retry succeeds once the lookup recovers.
    await page.unroute('**/api/session.v1.SessionService/FindSessionsByPullRequest');
    await omnibar.mockLookup([]);
    await omnibar.retry.click();
    await page.waitForURL(PR_SEARCH_URL, { timeout: 10000 });
  });

  test('omnibar should close on Escape while the lookup error is shown', async ({ page }) => {
    const omnibar = new OmnibarFindPrPage(page);
    await omnibar.mockLookup(null);
    await omnibar.open();
    await omnibar.paste();
    await omnibar.action.click();
    await expect(omnibar.error).toBeVisible();
    // Existing omnibar behavior: the first Escape leaves creation mode (the action and its error go
    // away with it), the second closes the dialog.
    await omnibar.input.press('Escape');
    await expect(omnibar.error).toBeHidden();
    await omnibar.input.press('Escape');
    await expect(page.getByTestId('omnibar')).toBeHidden();
  });

  test('single match should open the session with a View all action and the Tracked label', async ({ page }) => {
    const omnibar = new OmnibarFindPrPage(page);
    await omnibar.mockLookup([{ sessionId: 'only-one', title: 'only one', source: 'session_pr' }]);
    await omnibar.open();
    await omnibar.paste();
    await omnibar.action.click();

    await expect(page).toHaveURL(/session=only-one/, { timeout: 10000 });
    const toast = new ToastDeck(page).toasts.filter({ hasText: 'Opened the session tracking acme/app#12 (Tracked)' });
    await expect(toast).toBeVisible();
    await toast.getByRole('button', { name: 'View all' }).click();
    await page.waitForURL(PR_SEARCH_URL, { timeout: 10000 });
  });
});
