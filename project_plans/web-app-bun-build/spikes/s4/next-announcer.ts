// Task 0.5d + the Next side of the S4 guard-parity and scroll-container measurements.
// Runs against the REAL built Next app embedded in a ManualInstance (default http://localhost:62871; never the live instance port).
// Usage: bun next-announcer.ts   -> writes next-results.json
import { chromium, type Page } from "playwright-core";
import { writeFileSync } from "node:fs";
import { ROUTE_META } from "../../../../web-app/src/app/route-meta";

const BASE = process.env.S4_NEXT_BASE ?? "http://localhost:62871";
if (BASE.includes(":" + ["85", "43"].join(""))) throw new Error("refusing to touch the live instance");
const CHROME = `${process.env.HOME}/.cache/ms-playwright/chromium-1208/chrome-linux64/chrome`;
const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));
const browser = await chromium.launch({ executablePath: CHROME, args: ["--no-sandbox"] });
const out: Record<string, unknown> = { base: BASE, chromium: browser.version() };

// ---- announcer ----
{
  const page = await browser.newPage();
  await page.goto(`${BASE}/`);
  await page.waitForLoadState("networkidle");
  const read = () =>
    page.evaluate(() => {
      const host = document.querySelector("next-route-announcer");
      const el = host?.shadowRoot?.querySelector("#__next-route-announcer__") ?? document.getElementById("__next-route-announcer__");
      return {
        hostTag: host?.tagName ?? null,
        shadow: !!host?.shadowRoot,
        ariaLive: el?.getAttribute("aria-live") ?? null,
        role: el?.getAttribute("role") ?? null,
        text: el?.textContent ?? null,
        outerHTML: el?.outerHTML ?? null,
      };
    });
  const initial = await read();
  const events: unknown[] = [];
  await page.evaluate(() => {
    const host = document.querySelector("next-route-announcer");
    const el = host?.shadowRoot?.querySelector("#__next-route-announcer__");
    (window as any).__ann = [];
    if (el) new MutationObserver(() => (window as any).__ann.push({ text: el.textContent, title: document.title })).observe(el, { childList: true, characterData: true, subtree: true });
  });
  await page.evaluate(() => (window as any).next.router.push("/history"));
  await page.waitForURL(/\/history\/?$/);
  await wait(1500);
  const afterNav = await read();
  events.push(...(await page.evaluate(() => (window as any).__ann)));
  // grep the client source Next shipped in the built output
  const scripts = await page.evaluate(() => performance.getEntriesByType("resource").map((r) => r.name).filter((n) => n.endsWith(".js")));
  const hits: unknown[] = [];
  for (const url of [...new Set(scripts)]) {
    const body = await (await fetch(url)).text();
    const i = body.indexOf("__next-route-announcer__");
    if (i >= 0) hits.push({ url: url.replace(BASE, ""), snippet: body.slice(Math.max(0, i - 260), i + 160) });
  }
  out.announcer = { initial, afterClientNavToHistory: afterNav, mutationEvents: events, chunksContainingId: hits.length, hits: hits.slice(0, 2) };
  await page.close();
}

// ---- real scroll container ----
async function scrollInfo(page: Page, inject: boolean) {
  return page.evaluate((tall) => {
    if (tall) {
      const d = document.createElement("div");
      d.id = "__tall";
      d.style.cssText = "height:6000px;flex:none";
      (document.getElementById("main-content") ?? document.body).appendChild(d);
    }
    const vh = window.innerHeight;
    const se = document.scrollingElement!;
    const scrollers: string[] = [];
    document.querySelectorAll<HTMLElement>("body *").forEach((e) => {
      const cs = getComputedStyle(e);
      if (/(auto|scroll)/.test(cs.overflowY) && e.scrollHeight > e.clientHeight + 1 && e.clientHeight > vh * 0.3)
        scrollers.push(`${e.tagName.toLowerCase()}${e.id ? "#" + e.id : ""}${e.getAttribute("data-testid") ? `[data-testid=${e.getAttribute("data-testid")}]` : ""}.${String(e.className).split(" ")[0]}`);
    });
    const main = document.getElementById("main-content")!;
    const before = window.scrollY;
    window.scrollTo(0, 400);
    const winMoved = window.scrollY - before;
    window.scrollTo(0, 0);
    return {
      windowScrollable: se.scrollHeight - vh,
      windowMovedBy400: winMoved,
      htmlOverflow: getComputedStyle(document.documentElement).overflowY,
      bodyOverflow: getComputedStyle(document.body).overflowY,
      bodyHeight: Math.round(document.body.getBoundingClientRect().height),
      viewportH: vh,
      mainOverflow: getComputedStyle(main).overflowY,
      mainScrollable: main.scrollHeight - main.clientHeight,
      overflowingScrollers: scrollers.slice(0, 4),
    };
  }, inject);
}
{
  const rows: unknown[] = [];
  for (const viewport of [{ width: 1280, height: 800 }, { width: 390, height: 844 }]) {
    const ctx = await browser.newContext({ viewport });
    const page = await ctx.newPage();
    for (const path of Object.keys(ROUTE_META)) {
      await page.goto(`${BASE}${path}`);
      await page.waitForLoadState("networkidle").catch(() => {});
      await wait(300);
      const natural = await scrollInfo(page, false);
      const withTall = await scrollInfo(page, true);
      rows.push({ vp: `${viewport.width}x${viewport.height}`, path, finalPath: new URL(page.url()).pathname, natural, withTall });
    }
    await ctx.close();
  }
  out.scrollRows = rows;
}

// ---- Next-side FeatureGuard behaviour ----
{
  const guards = Object.entries(ROUTE_META).filter(([, m]) => m.guard).map(([p, m]) => ({ path: p, flag: m.guard! }));
  const rows: unknown[] = [];
  for (const g of guards) {
    for (const enabled of [true, false]) {
      for (const via of ["client-nav", "cold-load"] as const) {
        const ctx = await browser.newContext();
        const page = await ctx.newPage();
        let calls = 0;
        await page.route("**/GetFeatureFlags", async (route) => {
          calls++;
          const res = await route.fetch();
          const json = await res.json();
          const f = json.flags.find((x: any) => x.name === g.flag);
          if (f) f.enabled = enabled;
          else json.flags.push({ name: g.flag, enabled });
          await wait(800);
          await route.fulfill({ response: res, json });
        });
        const probe = (p: Page) => p.evaluate(() => ({ path: location.pathname, mainText: (document.getElementById("main-content")?.innerText ?? "").trim().length, len: history.length }));
        let len0: number;
        let early: unknown;
        if (via === "client-nav") {
          // land on /history with flags resolved (fast), then navigate client-side; first load pays the delay once
          await page.goto(`${BASE}/history`);
          await page.waitForLoadState("networkidle");
          len0 = (await probe(page)).len;
          calls = 0;
          await page.evaluate((to) => (window as any).next.router.push(to), g.path);
          await wait(250);
          early = await probe(page);
        } else {
          await page.goto(`${BASE}${g.path}`, { waitUntil: "commit" });
          len0 = -1;
          await wait(450);
          early = await probe(page).catch((e) => String(e));
        }
        await wait(1800);
        const late = await probe(page);
        rows.push({ guardPath: g.path, flag: g.flag, enabled, via, early, late, historyLengthDeltaFromBefore: len0 >= 0 ? late.len - len0 : null, getFeatureFlagsCalls: calls });
        await ctx.close();
      }
    }
  }
  out.guardRows = rows;
}

await browser.close();
writeFileSync(`${import.meta.dir}/next-results.json`, JSON.stringify(out, null, 2));
console.log("done", Object.keys(out));
