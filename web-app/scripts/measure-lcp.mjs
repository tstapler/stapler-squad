#!/usr/bin/env node
// Cold-load LCP A/B for the ghostty_renderer flag: node scripts/measure-lcp.mjs <baseUrl> [runs]
// Seeds the persisted flag and blocks GetFeatureFlags so the server cannot overwrite it.
import { chromium } from "@playwright/test";

const DEFAULT_RUNS = 5;
const CLICK_PX = 5;
const base = process.argv[2] ?? "http://localhost:62873";
const runs = Number(process.argv[3] ?? DEFAULT_RUNS);

async function lcpOnce(browser, ghostty) {
  const ctx = await browser.newContext();
  await ctx.addInitScript((on) => {
    if (on) localStorage.setItem("ssq:flag:ghostty_renderer", "1");
    else localStorage.removeItem("ssq:flag:ghostty_renderer");
  }, ghostty);
  await ctx.route(/GetFeatureFlags/, (r) => r.abort());
  const page = await ctx.newPage();
  const cdp = await ctx.newCDPSession(page);
  await cdp.send("Network.setCacheDisabled", { cacheDisabled: true });
  await page.addInitScript(() => {
    window.__lcp = 0;
    new PerformanceObserver((l) => {
      for (const e of l.getEntries()) window.__lcp = e.startTime;
    }).observe({ type: "largest-contentful-paint", buffered: true });
  });
  await page.goto(base + "/", { waitUntil: "load" });
  await page.waitForTimeout(2500);
  await page.mouse.click(CLICK_PX, CLICK_PX); // finalizes LCP
  const lcp = await page.evaluate(() => window.__lcp);
  await ctx.close();
  return Math.round(lcp);
}

const browser = await chromium.launch({ executablePath: process.env.CHROMIUM_PATH || undefined });
const median = (a) => [...a].sort((x, y) => x - y)[Math.floor(a.length / 2)];
const out = {};
for (const [name, on] of [["xterm", false], ["ghostty", true]]) {
  const v = [];
  for (let i = 0; i < runs; i++) v.push(await lcpOnce(browser, on));
  out[name] = { runs: v, median: median(v) };
}
await browser.close();
console.log(JSON.stringify(out));
console.log(`delta (ghostty - xterm) median: ${out.ghostty.median - out.xterm.median} ms (budget: 200 ms)`);
