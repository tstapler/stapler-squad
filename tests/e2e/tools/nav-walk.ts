/**
 * In-session navigation-sequence walk (Story 7.9b): navigates between routes through the nav links
 * without a reload, so CSS that is only injected when a route's chunk loads (T4 ordering bugs)
 * shows up as a difference between the final route and the same route loaded cold.
 */
import type { BrowserContext, Page } from '@playwright/test';
import { seededShuffle } from './parity-core.ts';

const STAY_TIMEOUT_MS = 5000;
export const FIXED_ORDER_COUNT = 3;
export const SEEDED_ORDER_COUNT = 3;

export interface WalkOrder {
  label: string;
  routes: string[];
}

/** 3 fixed orderings (forward, reverse, outside-in) then 3 shuffles derived from `seed`. */
export function walkOrders(routes: readonly string[], seed: number): WalkOrder[] {
  const forward = [...routes];
  const reverse = [...routes].reverse();
  const outsideIn: string[] = [];
  for (let lo = 0, hi = routes.length - 1; lo <= hi; lo++, hi--) {
    outsideIn.push(routes[lo]);
    if (hi !== lo) outsideIn.push(routes[hi]);
  }
  const orders: WalkOrder[] = [
    { label: 'fixed:forward', routes: forward },
    { label: 'fixed:reverse', routes: reverse },
    { label: 'fixed:outside-in', routes: outsideIn },
  ];
  for (let i = 0; i < SEEDED_ORDER_COUNT; i++) {
    orders.push({ label: `seeded:${seed + i}`, routes: seededShuffle(routes, seed + i) });
  }
  return orders;
}

const normalise = (href: string) => new URL(href, 'http://x').pathname.replace(/\/$/, '') || '/';

/** Routes that have a drawer-nav link on this build, in nav order. */
export async function navRoutes(page: Page): Promise<string[]> {
  const hrefs = await page
    .getByTestId('drawer-nav')
    .getByRole('link')
    .evaluateAll((links) => links.map((l) => (l as HTMLAnchorElement).getAttribute('href') ?? ''));
  return [...new Set(hrefs.map(normalise))];
}

/** Click the nav link for `route` (client-side navigation) and wait for the URL to settle. */
export async function clickNav(page: Page, route: string): Promise<void> {
  const links = page.getByTestId('drawer-nav').getByRole('link', { includeHidden: true });
  const hrefs = await links.evaluateAll((all) => all.map((l) => (l as HTMLAnchorElement).getAttribute('href') ?? ''));
  const index = hrefs.findIndex((h) => normalise(h) === route);
  if (index < 0) throw new Error(`no drawer link for ${route}`);
  const before = normalise(new URL(page.url()).pathname);
  if (before === route) return; // an earlier redirect already landed here; a click would not navigate
  await links.nth(index).click({ force: true });
  // Client navigation fires no load event, and a few routes redirect, so wait for the path to change.
  // A link that redirects straight back to the current page (for example a flag-guarded route) never
  // changes the path, so a timeout here means "stayed", not failure.
  await page.waitForURL((url) => normalise(url.pathname) !== before, { waitUntil: 'commit', timeout: STAY_TIMEOUT_MS }).catch(() => undefined);
  await page.evaluate(() => new Promise<void>((r) => requestAnimationFrame(() => requestAnimationFrame(() => r()))));
}

/** Stamps a window marker so the caller can prove no reload happened during the walk. */
export async function walk(context: BrowserContext, baseUrl: string, order: WalkOrder): Promise<Page> {
  const page = await context.newPage();
  await page.goto(`${baseUrl}${order.routes[0]}`, { waitUntil: 'load' });
  await page.evaluate(() => {
    (window as unknown as { __walkMarker: string }).__walkMarker = 'alive';
  });
  for (const route of order.routes.slice(1)) {
    await clickNav(page, route);
  }
  const marker = await page.evaluate(() => (window as unknown as { __walkMarker?: string }).__walkMarker);
  if (marker !== 'alive') throw new Error(`walk ${order.label} reloaded the document`);
  return page;
}
