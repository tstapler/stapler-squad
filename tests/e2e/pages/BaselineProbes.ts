import { expect, type Page } from "@playwright/test";
import { AppShellPage } from "./AppShellPage";

const BASE_URL = process.env.TEST_SERVER_URL || "http://localhost:8544";

export interface NotFoundBaseline {
  status: number;
  /** Pathname after client-side redirects settle ("/" means the sessions page took over). */
  finalPath: string;
  title: string;
}

/** Loads a path that matches no route and reports what the build does with it (Story 7.7a). */
export async function measureNotFound(page: Page, missing = "/nonexistent"): Promise<NotFoundBaseline> {
  const response = await page.goto(`${BASE_URL}${missing}`, { waitUntil: "domcontentloaded" });
  await expect(page.getByRole("main").first()).toBeVisible();
  // A client redirect, if the build does one, lands within a few seconds; none means the URL is kept.
  await page.waitForURL((url) => url.pathname !== missing, { timeout: 5000 }).catch(() => undefined);
  return { status: response?.status() ?? 0, finalPath: new URL(page.url()).pathname, title: await page.title() };
}

export interface ScrollPolicyBaseline {
  /** scrollTop of the long page's scroll container before navigating. */
  scrolledTo: number;
  /** scrollTop of the new page's scroll container after navigating forward (0 means the top). */
  forwardNavScrollTop: number;
  /** scrollTop of the long page's scroll container after Back (the restored value, or 0). */
  backScrollTop: number;
  /** Whether a hash-only navigation (#main-content) kept the same document. */
  hashNavKeepsDocument: boolean;
}

const LONG_PAGE = "/settings/features";
const SCROLL_TARGET = 1200;
const STABLE_READS = 2;

/** Reads scrollTop until two consecutive polls agree, so a value taken mid-effect isn't mistaken for the settled one. */
async function pollStableScrollTop(shell: AppShellPage): Promise<number> {
  let last: number | undefined;
  let stableCount = 0;
  return expect
    .poll(async () => {
      const value = await shell.scrollTop();
      stableCount = value === last ? stableCount + 1 : 1;
      last = value;
      return stableCount >= STABLE_READS ? value : undefined;
    })
    .not.toBeUndefined()
    .then(() => last!);
}

/** Forward nav, Back and hash nav against the long settings page (Story 7.12 baseline). */
export async function measureScrollPolicy(page: Page): Promise<ScrollPolicyBaseline> {
  const shell = new AppShellPage(page);
  await page.setViewportSize({ width: 1400, height: 900 });
  await shell.skipOnboarding();
  await shell.goto(LONG_PAGE);
  await expect(shell.drawerNav).toBeVisible();
  await expect(page.getByRole("main").last()).toBeVisible();
  await expect.poll(() => shell.scrollRange()).toBeGreaterThan(SCROLL_TARGET);
  await shell.scrollTo(SCROLL_TARGET);
  const scrolledTo = await shell.scrollTop();

  await shell.navLink(/history/i).click({ force: true });
  await expect(page).toHaveURL(/\/history/);
  await expect(shell.main).toBeVisible();
  // The forward-reset/POP-restore effect runs in a post-commit useEffect, a render tick after the
  // route change itself commits, so a read taken immediately after the URL changes can observe the
  // pre-effect value; poll for it to settle instead of reading once.
  const forwardNavScrollTop = await pollStableScrollTop(shell);

  await page.goBack();
  await expect(page).toHaveURL(/\/settings\/features/);
  await expect(page.getByRole("main").last()).toBeVisible();
  const backScrollTop = await pollStableScrollTop(shell);

  await shell.goto("/history");
  await expect(shell.drawerNav).toBeVisible();
  await page.evaluate(() => {
    (window as unknown as { __hashMarker: string }).__hashMarker = "same-document";
  });
  await shell.tabToSkipLink();
  await shell.activateSkipLink();
  const hashNavKeepsDocument = await page.evaluate(
    () => (window as unknown as { __hashMarker?: string }).__hashMarker === "same-document",
  );
  return { scrolledTo, forwardNavScrollTop, backScrollTop, hashNavKeepsDocument };
}
