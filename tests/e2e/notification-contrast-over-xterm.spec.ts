// @feature notification-tray, accessibility
/**
 * MC-1 (plan Task 3.7g, ux.md TC-7 / XA-3): WCAG contrast of the toast deck and the tray
 * text, icons and icon-only controls when they sit over the xterm canvas. Axe cannot see
 * canvas pixels, so this samples the rendered pixels itself: every overlay element's ink is
 * made transparent, one screenshot is taken, and each text/icon rect is scored against the
 * worst background pixel behind it (text >= 4.5:1, large text and icons >= 3:1).
 * Writes the raw measurements to MC1_OUT (default /tmp/mc1-contrast) for the report.
 */
import fs from 'fs';
import path from 'path';
import { test, expect, type Page } from '@playwright/test';
import { NotificationTray, ToastDeck, TRAY_V2_FLAG, sendNotification, setFeatureFlag } from './pages/NotificationPanel';
import { SessionClient } from './helpers/session-client';
import { profiles } from './helpers/viewport-profiles';

const BASE_URL = process.env.TEST_SERVER_URL || 'http://localhost:8544';
const OUT = process.env.MC1_OUT || '/tmp/mc1-contrast';
const MODES = ['brightfill', 'whitebg', 'brightwhitebg', 'yellowbg', 'syntax'] as const;
const THEMES = (process.env.MC1_THEMES || 'light,dark,clean,matrix,cyberpunk77,wh40k').split(',');
const ROOT_SELECTORS = [
  '[data-testid="toast-stack"]',
  '[data-testid="notification-tray"]',
  '[data-testid="tray-handle"]',
  '[data-testid="tray-entry"]',
];
const ROOTS = ROOT_SELECTORS.join(', ');

export interface Measurement {
  surface: string;
  kind: 'text' | 'icon' | 'icon-control-boundary';
  sample: string;
  required: number;
  worst: number;
  fg: string;
  worstBg: string;
  /** Page position (CSS px) of the worst pixel, for locating a failure in the saved screenshots. */
  at: string;
}

/** Runs in the page: scores each overlay element against the ink-less screenshot. */
async function score(page: Page, pngBase64: string): Promise<Measurement[]> {
  return page.evaluate(
    async ({ png, roots }) => {
      const dpr = window.devicePixelRatio;
      const bitmap = await createImageBitmap(await (await fetch(`data:image/png;base64,${png}`)).blob());
      const canvas = new OffscreenCanvas(bitmap.width, bitmap.height);
      const ctx = canvas.getContext('2d', { willReadFrequently: true })!;
      ctx.drawImage(bitmap, 0, 0);

      const parse = (c: string): [number, number, number, number] | null => {
        const m = c.match(/rgba?\(([^)]+)\)/) || c.match(/color\(srgb ([^)]+)\)/);
        if (!m) return null;
        const p = m[1].split(/[ ,/]+/).filter(Boolean).map(Number);
        if (c.startsWith('color(')) return [p[0] * 255, p[1] * 255, p[2] * 255, p.length > 3 ? p[3] : 1];
        return [p[0], p[1], p[2], p.length > 3 ? p[3] : 1];
      };
      const lin = (v: number) => {
        const s = v / 255;
        return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
      };
      const lum = (r: number, g: number, b: number) => 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
      const ratio = (a: number, b: number) => (Math.max(a, b) + 0.05) / (Math.min(a, b) + 0.05);
      const effectiveOpacity = (el: Element) => {
        let o = 1;
        for (let n: Element | null = el; n; n = n.parentElement) o *= parseFloat(getComputedStyle(n).opacity);
        return o;
      };
      const surfaceOf = (el: Element) => {
        const chain: string[] = [];
        for (let n: Element | null = el; n; n = n.parentElement) {
          const id = n.getAttribute('data-testid');
          if (id) chain.push(id);
        }
        return chain.length ? chain.slice(0, 2).reverse().join(' > ') : el.tagName.toLowerCase();
      };

      const pixelsIn = (r: { x: number; y: number; w: number; h: number }) => {
        const x0 = Math.max(0, Math.floor(r.x * dpr));
        const y0 = Math.max(0, Math.floor(r.y * dpr));
        const x1 = Math.min(bitmap.width, Math.ceil((r.x + r.w) * dpr));
        const y1 = Math.min(bitmap.height, Math.ceil((r.y + r.h) * dpr));
        if (x1 <= x0 || y1 <= y0) return null;
        const img = ctx.getImageData(x0, y0, x1 - x0, y1 - y0);
        return Object.assign(img.data, { originX: x0, originY: y0, width: x1 - x0 });
      };
      type Pixels = Uint8ClampedArray & { originX: number; originY: number; width: number };
      const worstAgainst = (data: Pixels, fg: [number, number, number, number], opacity: number) => {
        const a = fg[3] * opacity;
        let worst = Infinity;
        let worstBg = '';
        let at = '';
        for (let i = 0; i < data.length; i += 4) {
          const r = fg[0] * a + data[i] * (1 - a);
          const g = fg[1] * a + data[i + 1] * (1 - a);
          const b = fg[2] * a + data[i + 2] * (1 - a);
          const c = ratio(lum(r, g, b), lum(data[i], data[i + 1], data[i + 2]));
          if (c < worst) {
            worst = c;
            worstBg = `rgb(${data[i]},${data[i + 1]},${data[i + 2]})`;
            const px = i / 4;
            at = `${Math.round((data.originX + (px % data.width)) / dpr)},${Math.round((data.originY + Math.floor(px / data.width)) / dpr)}`;
          }
        }
        return { worst, worstBg, at };
      };
      // Skips elements scrolled out of view or covered by another surface (e.g. the tray's sticky footer).
      const onTop = (el: Element, b: { x: number; y: number; width: number; height: number }) => {
        const cx = b.x + b.width / 2;
        const cy = b.y + b.height / 2;
        if (cx < 0 || cy < 0 || cx > window.innerWidth || cy > window.innerHeight) return false;
        const top = document.elementFromPoint(cx, cy);
        return !!top && (el.contains(top) || top.contains(el));
      };
      const visible = (el: Element) =>
        (el as HTMLElement).checkVisibility?.({ checkOpacity: true, checkVisibilityCSS: true }) ?? true;

      const out: Measurement[] = [];
      const seen = new Set<Element>();
      for (const root of Array.from(document.querySelectorAll(roots))) {
        for (const el of [root, ...Array.from(root.querySelectorAll('*'))]) {
          if (seen.has(el) || !visible(el)) continue;
          seen.add(el);
          const cs = getComputedStyle(el);
          const opacity = effectiveOpacity(el);
          const ownText = Array.from(el.childNodes).filter((n) => n.nodeType === 3 && (n.textContent || '').trim());
          if (ownText.length) {
            const range = document.createRange();
            range.setStartBefore(ownText[0]);
            range.setEndAfter(ownText[ownText.length - 1]);
            const tb = range.getBoundingClientRect();
            const eb = el.getBoundingClientRect();
            // Only the painted part is scored: an ellipsized label spills past its box and a
            // max-height card clips what overflows it.
            let [left, top, right, bottom] = [Math.max(tb.left, eb.left), Math.max(tb.top, eb.top), Math.min(tb.right, eb.right), Math.min(tb.bottom, eb.bottom)];
            for (let a = el.parentElement; a; a = a.parentElement) {
              const acs = getComputedStyle(a);
              if (acs.overflowX === 'visible' && acs.overflowY === 'visible') continue;
              // Content is clipped at the padding box, inside the border.
              const ab = a.getBoundingClientRect();
              const [pl, pt] = [ab.left + a.clientLeft, ab.top + a.clientTop];
              [left, top, right, bottom] = [Math.max(left, pl), Math.max(top, pt), Math.min(right, pl + a.clientWidth), Math.min(bottom, pt + a.clientHeight)];
            }
            const b = { x: left, y: top, width: right - left, height: bottom - top };
            const label = ownText.map((n) => n.textContent!.trim()).join(' ');
            const emojiOnly = /^[\p{Extended_Pictographic}\uFE0F\s]+$/u.test(label);
            const data = pixelsIn({ x: b.x, y: b.y, w: b.width, h: b.height });
            const fg = parse(cs.color);
            if (data && fg && b.width > 0 && b.height > 0 && !emojiOnly && onTop(el, b)) {
              const px = parseFloat(cs.fontSize);
              const large = px >= 24 || (px >= 18.66 && parseInt(cs.fontWeight, 10) >= 700);
              const { worst, worstBg, at } = worstAgainst(data, fg, opacity);
              out.push({
                surface: surfaceOf(el),
                kind: 'text',
                sample: label.slice(0, 40),
                required: large ? 3 : 4.5,
                worst,
                fg: cs.color,
                worstBg,
                at,
              });
            }
          }
          if (el instanceof SVGSVGElement) {
            const b = el.getBoundingClientRect();
            const data = pixelsIn({ x: b.x, y: b.y, w: b.width, h: b.height });
            const paint = cs.stroke !== 'none' ? cs.stroke : cs.fill !== 'none' ? cs.fill : cs.color;
            const fg = parse(paint);
            if (data && fg && b.width > 0 && onTop(el, b)) {
              const { worst, worstBg, at } = worstAgainst(data, fg, opacity);
              out.push({
                surface: surfaceOf(el),
                kind: 'icon',
                sample: el.getAttribute('aria-label') || el.parentElement?.getAttribute('aria-label') || 'svg',
                required: 3,
                worst,
                fg: paint,
                worstBg,
                at,
              });
            }
          }
          // Icon-only controls have no text label, so their boundary is what identifies them (WCAG 1.4.11).
          if (el instanceof HTMLButtonElement && !(el.textContent || '').trim()) {
            const b = el.getBoundingClientRect();
            const fill = parse(cs.backgroundColor);
            const border = parseFloat(cs.borderTopWidth) > 0 ? parse(cs.borderTopColor) : null;
            const edge = border && border[3] > 0 ? border : fill && fill[3] > 0 ? fill : null;
            const ringPx = 3;
            const ring = pixelsIn({ x: b.x - ringPx, y: b.y - ringPx, w: b.width + ringPx * 2, h: ringPx });
            if (edge && ring && b.width > 0 && onTop(el, b)) {
              const { worst, worstBg, at } = worstAgainst(ring, edge, opacity);
              out.push({
                surface: surfaceOf(el),
                kind: 'icon-control-boundary',
                sample: el.getAttribute('aria-label') || 'button',
                required: 3,
                worst,
                fg: `${border && border[3] > 0 ? 'border' : 'fill'} ${cs.backgroundColor}`,
                worstBg,
                at,
              });
            }
          }
        }
      }
      return out;
    },
    { png: pngBase64, roots: ROOTS },
  );
}

/** Waits until every overlay card is fully opaque and stopped moving, so no half-entered card is scored. */
async function overlaySettled(page: Page): Promise<void> {
  let last = '';
  await expect
    .poll(
      async () => {
        const frame = await page.evaluate(() => {
          const els = Array.from(document.querySelectorAll('[data-testid="toast"], [data-testid="notification-tray"]'));
          return JSON.stringify(
            els.map((el) => {
              const r = el.getBoundingClientRect();
              return [getComputedStyle(el).opacity, Math.round(r.x), Math.round(r.y), Math.round(r.width), Math.round(r.height)];
            }),
          );
        });
        const stable = frame === last && !/"0\.\d+"/.test(frame);
        last = frame;
        return stable;
      },
      { intervals: [150], timeout: 10_000 },
    )
    .toBe(true);
}

async function measure(page: Page, label: string, into: Measurement[], shotName: string): Promise<void> {
  await overlaySettled(page);
  await page.addStyleTag({ content: '*, *::before, *::after { transition: none !important; animation: none !important; }' });
  const everything = ROOT_SELECTORS.flatMap((r) => [r, `${r} *`]).join(', ');
  const svgs = ROOT_SELECTORS.flatMap((r) => [`${r} svg`, `${r} svg *`]).join(', ');
  const hide = await page.addStyleTag({
    content: `${everything} { color: transparent !important; -webkit-text-fill-color: transparent !important; text-shadow: none !important; }
      ${svgs} { stroke: transparent !important; fill: transparent !important; }`,
  });
  const png = (await page.screenshot({ type: 'png' })).toString('base64');
  await hide.evaluate((el) => (el as Element).remove());
  const rows = await score(page, png);
  into.push(...rows.map((r) => ({ ...r, surface: `${label}: ${r.surface}` })));
  fs.mkdirSync(OUT, { recursive: true });
  await page.screenshot({ path: path.join(OUT, `${shotName}.png`) });
}

type ToastKind = 'approval' | 'error' | 'info' | 'warning';

const KIND_NOTIFICATION: Record<ToastKind, { type: 'APPROVAL_NEEDED' | 'ERROR' | 'CUSTOM' | 'WARNING'; title: string; message: string }> = {
  approval: { type: 'APPROVAL_NEEDED', title: 'Approve command?', message: 'Bash: rm -rf ./build && git push --force-with-lease origin main' },
  error: { type: 'ERROR', title: 'Build failed', message: 'make: *** [build] Error 2' },
  info: { type: 'CUSTOM', title: 'Session finished', message: 'Task completed in 42s' },
  warning: { type: 'WARNING', title: 'Disk almost full', message: '92% used' },
};

/** Seeds the given kinds in order; the first send is retried until the live stream is attached. */
async function seedDeck(page: Page, request: Parameters<typeof sendNotification>[0], prefix: string, kinds: ToastKind[]): Promise<ToastDeck> {
  const deck = new ToastDeck(page);
  const send = (kind: ToastKind) => {
    const n = KIND_NOTIFICATION[kind];
    const metadata = kind === 'approval' ? { approval_id: `${prefix}-approval-id`, tool_name: 'Bash' } : undefined;
    return sendNotification(request, BASE_URL, { sessionId: `${prefix}-${kind}`, metadata, ...n });
  };
  await expect(async () => {
    await send(kinds[0]);
    await expect(deck.toasts.first()).toBeVisible({ timeout: 1_500 });
  }).toPass({ timeout: 20_000 });
  for (const kind of kinds.slice(1)) await send(kind);
  await expect(deck.toasts).toHaveCount(Math.min(kinds.length, 3));
  return deck;
}

/** Waits until two consecutive screenshots of the terminal area are identical (output has stopped). */
async function terminalSettled(page: Page): Promise<void> {
  const input = page.getByRole('textbox', { name: 'Terminal input' });
  await expect(input).toBeAttached({ timeout: 20_000 });
  let last = '';
  await expect
    .poll(
      async () => {
        const clip = await input.evaluate((el) => {
          const r = (el.closest('.xterm') ?? el).getBoundingClientRect();
          return { x: r.x, y: r.y, width: r.width, height: r.height };
        });
        if (clip.width < 50 || clip.height < 50) return false;
        const shot = (await page.screenshot({ clip })).toString('base64');
        const same = shot === last;
        last = shot;
        return same;
      },
      { intervals: [400], timeout: 30_000 },
    )
    .toBe(true);
}

for (const vpKey of ['V1', 'V2'] as const) {
  test.describe(`contrast over xterm ${profiles[vpKey].name}`, () => {
    test.use(profiles[vpKey].use);
    test.setTimeout(240_000);
    const client = new SessionClient(BASE_URL);
    const sessions = new Map<string, string>();

    test.beforeAll(async () => {
      for (const mode of MODES) {
        const s = await client.createSession({
          title: `mc1-${mode}-${vpKey}-${Date.now()}`,
          path: '/tmp',
          program: `sh ${path.join(__dirname, 'fixtures', 'mc1-terminal-content.sh')} ${mode}`,
        });
        sessions.set(mode, s.id);
      }
    });
    test.afterAll(async () => {
      for (const id of sessions.values()) await client.deleteSession(id, true).catch(() => undefined);
    });
    test.beforeEach(async ({ request }) => {
      await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, true);
    });
    test.afterEach(async ({ request }) => {
      await setFeatureFlag(request, BASE_URL, TRAY_V2_FLAG, false);
    });

    for (const theme of THEMES) {
      test(`${theme}: toast deck and tray text, icons and icon controls meet AA over every terminal content mode`, async ({ page, request }) => {
        await page.addInitScript(
          ([name]) => {
            localStorage.setItem('stapler-theme', name);
            localStorage.setItem('stapler-squad:onboarded', 'true');
          },
          [theme],
        );
        const rows: Measurement[] = [];
        // The phone deck shows one card at a time, so each toast kind gets its own round there.
        const rounds: ToastKind[][] =
          vpKey === 'V1' ? [['approval', 'error', 'info', 'warning']] : [['approval'], ['error'], ['info'], ['warning']];
        for (const mode of MODES) {
          for (const [round, kinds] of rounds.entries()) {
            await page.goto(`${BASE_URL}/?session=${sessions.get(mode)}`, { waitUntil: 'domcontentloaded' });
            await terminalSettled(page);
            const deck = await seedDeck(page, request, `mc1-${vpKey}-${theme}-${mode}-${round}-${Date.now()}`, kinds);
            const shot = `${vpKey}-${theme}-${mode}-${kinds.join('+')}`;
            await measure(page, `${mode} deck(${kinds.join('+')})`, rows, `${shot}-deck`);
            if (round > 0) continue;

            const tray = new NotificationTray(page);
            await tray.handle.or(deck.chip).or(tray.entryOpen).first().click();
            await tray.expectOpen();
            await expect
              .poll(() =>
                tray.tray.evaluate((el) => {
                  const m = new DOMMatrix(getComputedStyle(el).transform);
                  return Math.abs(m.m41) + Math.abs(m.m42) < 0.5;
                }),
              )
              .toBe(true);
            await measure(page, `${mode} tray`, rows, `${shot}-tray`);
            await tray.dismissWhatChanged();
            await measure(page, `${mode} tray`, rows, `${shot}-tray-2`);
          }
        }
        fs.mkdirSync(OUT, { recursive: true });
        fs.writeFileSync(path.join(OUT, `${vpKey}-${theme}.json`), JSON.stringify(rows, null, 1));
        expect(rows.length).toBeGreaterThan(10);
        const failing = rows.filter((r) => r.worst < r.required);
        expect(
          failing.map((r) => `${r.surface} [${r.kind}] "${r.sample}" ${r.worst.toFixed(2)} < ${r.required} (fg ${r.fg} on ${r.worstBg} at ${r.at})`),
        ).toEqual([]);
      });
    }
  });
}
