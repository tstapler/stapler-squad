/**
 * Next-vs-Bun computed-style parity harness (Story 7.9). Usage (run from tests/e2e):
 *   bun tools/parity-diff.ts --a http://localhost:A --b http://localhost:B [--bun-run] [--quick] [--walk-only] [--allow file] [--out diff.json]
 *
 * Matrix: 5 routes x 2 viewports x 6 theme classes, comparing computed styles (plus :hover and
 * :focus-visible on the first 5 focusable elements) and the resolved :root custom-property map.
 * `--bun-run` (b is a Bun build) additionally runs all RouteMeta routes cold at 1 viewport x 2
 * themes, and the in-session navigation-sequence walk (3 fixed + 3 seeded orderings, no reload),
 * comparing each walk's final route with its cold load on b and with a.
 * Exit status is non-zero on any difference not covered by parity-allow.json, whose entries each
 * need {route, selector, property, reason} with reason a URL or a quoted CSS rule.
 */
import * as fs from 'fs';
import * as path from 'path';
import { chromium, type Browser, type BrowserContext } from '@playwright/test';
import { ROUTE_META } from '../../../web-app/src/app/route-meta.ts';
import { collectSnapshot, primePage, THEME_NAMES } from './parity-collect.ts';
import { clickNav, navRoutes, walkOrders, FIXED_ORDER_COUNT } from './nav-walk.ts';
import {
  diffSnapshots,
  unallowedDifferences,
  validateAllowList,
  type AllowEntry,
  type Difference,
  type StyleSnapshot,
} from './parity-core.ts';

const ROUTES = ['/', '/history', '/settings', '/review-queue', '/rules'];
const VIEWPORTS = [
  { width: 1280, height: 800 },
  { width: 390, height: 844 },
];
const COLD_THEMES = ['clean', 'matrix'];
const ALLOW_FILE = path.join(import.meta.dirname, 'parity-allow.json');
const DEFAULT_SEED = 20261019;

interface Args {
  a: string;
  b: string;
  bunRun: boolean;
  /** Debug: only the navigation-sequence walk. */
  walkOnly: boolean;
  /** Self-test mode: one route, one viewport, one theme. */
  quick: boolean;
  out?: string;
  allowFile: string;
  seed: number;
}

function parseArgs(argv: string[]): Args {
  const value = (flag: string) => {
    const i = argv.indexOf(flag);
    return i >= 0 ? argv[i + 1] : undefined;
  };
  const a = value('--a');
  const b = value('--b');
  if (!a || !b) throw new Error('usage: parity-diff.ts --a <baseUrl> --b <baseUrl> [--bun-run] [--out file] [--seed n]');
  return { a, b, bunRun: argv.includes('--bun-run'), quick: argv.includes('--quick'), walkOnly: argv.includes('--walk-only'), out: value('--out'), allowFile: value('--allow') ?? ALLOW_FILE, seed: Number(value('--seed') ?? DEFAULT_SEED) };
}

const QUIET_MS = 1500;
const SETTLE_TIMEOUT_MS = 10000;
const FREEZE_MOTION_CSS = '*,*::before,*::after{animation:none!important;transition:none!important;caret-color:transparent!important}';

/** Waits for fonts and a DOM that has stopped mutating, and freezes animation so samples are repeatable. */
async function settle(page: import('@playwright/test').Page): Promise<void> {
  await page.getByRole('main').first().waitFor();
  await page.addStyleTag({ content: FREEZE_MOTION_CSS });
  await page.evaluate(
    ({ quiet, timeout }) =>
      document.fonts.ready.then(
        () =>
          new Promise<void>((resolve) => {
            let timer = setTimeout(done, quiet);
            const observer = new MutationObserver(() => {
              clearTimeout(timer);
              timer = setTimeout(done, quiet);
            });
            const hard = setTimeout(done, timeout);
            function done() {
              observer.disconnect();
              clearTimeout(hard);
              resolve();
            }
            observer.observe(document.body, { subtree: true, childList: true, attributes: true, characterData: true });
          }),
      ),
    { quiet: QUIET_MS, timeout: SETTLE_TIMEOUT_MS },
  );
}

async function coldSnapshot(browser: Browser, base: string, route: string, theme: string, viewport: { width: number; height: number }): Promise<StyleSnapshot> {
  const context = await browser.newContext({ viewport });
  try {
    const page = await context.newPage();
    await primePage(page, theme);
    await page.goto(`${base}${route}`, { waitUntil: 'load' });
    await settle(page);
    return await collectSnapshot(page);
  } finally {
    await context.close();
  }
}

const diffKey = (d: Difference) => `${d.route}|${d.cell}|${d.selector}|${d.property}|${d.a}|${d.b}`;

/** A difference caused by late-loading data shows up once; one caused by CSS shows up every time. */
async function confirmed(measure: () => Promise<Difference[]>): Promise<Difference[]> {
  const first = await measure();
  if (first.length === 0) return first;
  const again = new Set((await measure()).map(diffKey));
  return first.filter((d) => again.has(diffKey(d)));
}

async function matrixDiffs(browser: Browser, args: Args): Promise<Difference[]> {
  const diffs: Difference[] = [];
  for (const route of args.quick ? ROUTES.slice(0, 1) : ROUTES) {
    for (const viewport of args.quick ? VIEWPORTS.slice(0, 1) : VIEWPORTS) {
      for (const theme of args.quick ? THEME_NAMES.slice(3, 4) : THEME_NAMES) {
        diffs.push(
          ...(await confirmed(async () => {
            const [a, b] = await Promise.all([coldSnapshot(browser, args.a, route, theme, viewport), coldSnapshot(browser, args.b, route, theme, viewport)]);
            return diffSnapshots(route, a, b, `@${viewport.width}x${viewport.height} [${theme}]`);
          })),
        );
      }
    }
  }
  return diffs;
}

async function coldRouteDiffs(browser: Browser, args: Args): Promise<Difference[]> {
  const diffs: Difference[] = [];
  for (const route of Object.keys(ROUTE_META)) {
    for (const theme of COLD_THEMES) {
      diffs.push(
        ...(await confirmed(async () => {
          const [a, b] = await Promise.all([coldSnapshot(browser, args.a, route, theme, VIEWPORTS[0]), coldSnapshot(browser, args.b, route, theme, VIEWPORTS[0])]);
          return diffSnapshots(route, a, b, `cold [${theme}]`);
        })),
      );
    }
  }
  return diffs;
}

async function walkFinal(context: BrowserContext, base: string, routes: string[]): Promise<{ snapshot: StyleSnapshot; finalPath: string }> {
  const page = await context.newPage();
  await page.goto(`${base}${routes[0]}`, { waitUntil: 'load' });
  await page.evaluate(() => {
    (window as unknown as { __walk: boolean }).__walk = true;
  });
  for (const route of routes.slice(1)) await clickNav(page, route);
  if (!(await page.evaluate(() => (window as unknown as { __walk?: boolean }).__walk))) throw new Error('navigation walk reloaded the document');
  // A cold load has no pointer or focus on the last clicked link; clear both so only CSS can differ.
  const { width, height } = VIEWPORTS[0];
  await page.mouse.move(width - 1, height - 1);
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());
  await settle(page);
  const snapshot = await collectSnapshot(page);
  const finalPath = new URL(page.url()).pathname.replace(/(.)\/$/, '$1');
  await page.close();
  return { snapshot, finalPath };
}

async function walkDiffs(browser: Browser, args: Args): Promise<Difference[]> {
  const probeContext = await browser.newContext({ viewport: VIEWPORTS[0] });
  const probe = await probeContext.newPage();
  await primePage(probe, 'clean');
  await probe.goto(`${args.b}/`, { waitUntil: 'load' });
  const routes = await navRoutes(probe);
  await probeContext.close();

  const diffs: Difference[] = [];
  console.log(`navigation walk over ${routes.length} routes, seed ${args.seed}`);
  for (const order of walkOrders(routes, args.seed)) {
    diffs.push(
      ...(await confirmed(async () => {
        const walkContext = await browser.newContext({ viewport: VIEWPORTS[0] });
        await primePage(walkContext, 'clean');
        const { snapshot: walked, finalPath: last } = await walkFinal(walkContext, args.b, order.routes);
        await walkContext.close();
        const cold = await coldSnapshot(browser, args.b, last, 'clean', VIEWPORTS[0]);
        const next = await coldSnapshot(browser, args.a, last, 'clean', VIEWPORTS[0]);
        return [
          ...diffSnapshots(last, walked, cold, `walk ${order.label} vs cold b`),
          ...diffSnapshots(last, walked, next, `walk ${order.label} vs a`),
        ];
      })),
    );
  }
  console.log(`${FIXED_ORDER_COUNT} fixed + seeded orderings done`);
  return diffs;
}

async function main(): Promise<number> {
  const args = parseArgs(process.argv.slice(2));
  const allow = JSON.parse(fs.readFileSync(args.allowFile, 'utf8')) as AllowEntry[];
  const problems = validateAllowList(allow);
  if (problems.length) {
    console.error(`${args.allowFile} is invalid:\n${problems.join('\n')}`);
    return 2;
  }
  const browser = await chromium.launch();
  try {
    const diffs = args.walkOnly ? [] : [...(await matrixDiffs(browser, args))];
    if (args.bunRun && !args.walkOnly) diffs.push(...(await coldRouteDiffs(browser, args)));
    if (args.bunRun || args.walkOnly) diffs.push(...(await walkDiffs(browser, args)));
    const unallowed = unallowedDifferences(diffs, allow);
    const report = { a: args.a, b: args.b, total: diffs.length, allowed: diffs.length - unallowed.length, unallowed };
    if (args.out) fs.writeFileSync(args.out, `${JSON.stringify(report, null, 2)}\n`);
    console.log(JSON.stringify({ total: report.total, allowed: report.allowed, unallowed: unallowed.length }));
    return unallowed.length === 0 ? 0 : 1;
  } finally {
    await browser.close();
  }
}

main().then((code) => process.exit(code), (err) => {
  console.error(err);
  process.exit(2);
});
