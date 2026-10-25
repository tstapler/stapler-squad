// Identify the universal inner scroller seen at 390x844 in next-results.json (class _8c7jyjh): its ancestors and sizes.
import { chromium } from "playwright-core";
const CHROME = `${process.env.HOME}/.cache/ms-playwright/chromium-1208/chrome-linux64/chrome`;
const b = await chromium.launch({ executablePath: CHROME, args: ["--no-sandbox"] });
const page = await (await b.newContext({ viewport: { width: 390, height: 844 } })).newPage();
await page.goto(`${process.env.S4_NEXT_BASE ?? "http://localhost:62871"}/history`);
await page.waitForLoadState("networkidle");
console.log(
  JSON.stringify(
    await page.evaluate(() => {
      const el = document.querySelector<HTMLElement>("div._8c7jyjh");
      const chain: string[] = [];
      for (let e: HTMLElement | null = el; e && e !== document.body; e = e.parentElement)
        chain.push(`${e.tagName.toLowerCase()}${e.id ? "#" + e.id : ""}${e.dataset.testid ? `[data-testid=${e.dataset.testid}]` : ""} overflowY=${getComputedStyle(e).overflowY} h=${Math.round(e.clientHeight)} sh=${e.scrollHeight}`);
      return { found: !!el, chain, containsMain: !!el?.querySelector("#main-content"), insideMain: !!el?.closest("#main-content") };
    }),
    null,
    1,
  ),
);
await b.close();
