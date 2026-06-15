import { FEATURE_CATALOG } from '../../web-app/src/lib/features';
// Features: one-off-session — mapped from @feature annotation
const _features = [
  FEATURE_CATALOG['session-create'],
] as const;
import { test, expect } from '@playwright/test';

const BASE_URL = 'http://localhost:8544';

test.describe('one-off session creation', () => {
  test('shows one-off option in creation panel', async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });

    // Open omnibar / new session panel
    await page.getByText('New Session').click();

    // The One-off radio option must be visible
    await expect(page.getByRole('radio', { name: /one.off/i })).toBeVisible({ timeout: 5000 });
  });

  test('hides path input when one-off is selected', async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });
    await page.getByText('New Session').click();

    // Select the One-off option
    await page.getByRole('radio', { name: /one.off/i }).click();

    // Path / directory input must not be visible
    await expect(page.getByPlaceholder(/path|directory|repo/i)).not.toBeVisible();

    // Session name input must still be visible and required
    await expect(page.getByLabel('Session Name')).toBeVisible();
  });

  test('creation panel stays visible while typing session name', async ({ page }) => {
    // Regression: typing in the Session Name field used to trigger the detection
    // useEffect (sessionName was in its dep array), which reset mode to discovery
    // when the main input was empty, collapsing the creation panel mid-typing.
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });
    await page.getByText('New Session').click();

    await page.getByRole('radio', { name: /one.off/i }).click();

    const nameInput = page.getByLabel('Session Name');
    await expect(nameInput).toBeVisible({ timeout: 5000 });

    // Type character-by-character — each keystroke previously triggered the bug
    await nameInput.pressSequentially('my-one-off-session', { delay: 30 });

    // Creation panel (radio group) must still be present after typing the full name
    await expect(page.getByRole('radio', { name: /one.off/i })).toBeVisible();
    await expect(nameInput).toHaveValue('my-one-off-session');

    // Submit button must be enabled (session name is all that's required for one-off)
    await expect(page.getByRole('button', { name: /create|start/i })).toBeEnabled();
  });

  test('creates session with one_off flag', async ({ page }) => {
    await page.goto(BASE_URL, { waitUntil: 'domcontentloaded', timeout: 10000 });
    await page.getByText('New Session').click();

    // Select One-off and fill title via the labelled Session Name field
    await page.getByRole('radio', { name: /one.off/i }).click();
    await page.getByLabel('Session Name').fill('e2e-one-off-test');

    // Intercept the CreateSession RPC to verify one_off is set
    const requestPromise = page.waitForRequest(
      (req) => req.url().includes('CreateSession') && req.method() === 'POST'
    );

    await page.getByRole('button', { name: /create|start/i }).click();

    const request = await requestPromise;
    const body = request.postDataJSON();
    expect(body).toMatchObject({ oneOff: true });
    expect(body.path ?? '').toBe('');
  });
});
