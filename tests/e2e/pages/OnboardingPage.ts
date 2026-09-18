import { Page } from '@playwright/test';

/**
 * Dismisses the first-run onboarding modal if it's present. The modal mounts
 * ~800ms after page load (useOnboarding.ts's setTimeout), so this must be a
 * real click-and-catch, not an isVisible() guard — Locator.isVisible({timeout})
 * is a deprecated no-op that checks the DOM synchronously and never polls,
 * which would silently skip the click before the modal exists. .click({timeout})
 * does poll, so the bare click+catch below is what actually works.
 */
export async function dismissOnboardingIfPresent(page: Page): Promise<void> {
  await page
    .getByRole('button', { name: 'Skip onboarding' })
    .click({ timeout: 5000 })
    .catch(() => {});
}
