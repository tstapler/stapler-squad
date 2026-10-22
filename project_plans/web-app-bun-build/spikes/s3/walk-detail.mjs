// Diagnostic companion to parity-diff.mjs's `walk` mode: an untruncated diff of one ordering's final route
// against its own cold load, on the same Bun build. parity-diff.mjs's `vsBunCold.final.vsBunColdDiffs` count
// undercounts this for reasons not fully diagnosed (see spike-results.md's S3 section); this script was how the
// pointer-events-stuck-after-navigation finding was actually confirmed.
// Run from web-app/:  node ../project_plans/web-app-bun-build/spikes/s3/walk-detail.mjs <bunBase> <order.json>
import { readFileSync } from "node:fs";
import { launch } from "./pw.mjs";

const bunBase = process.argv[2];
const order = JSON.parse(readFileSync(process.argv[3], "utf8"));
const browser = await launch();

const pageHelpers = () => {
  const props = (el) => {
    const cs = getComputedStyle(el);
    const out = {};
    for (let i = 0; i < cs.length; i++) if (!cs[i].startsWith("--")) out[cs[i]] = cs.getPropertyValue(cs[i]);
    return out;
  };
  window.__ssq = {
    snap() {
      const els = {};
      const seen = {};
      for (const el of [...document.querySelectorAll("[data-testid]")].slice(0, 400)) {
        const id = el.getAttribute("data-testid").replace(/[0-9a-f]{8}(?:-[0-9a-f]{4}){0,4}/g, "ID");
        seen[id] = (seen[id] ?? 0) + 1;
        els[`${id}#${seen[id]}`] = props(el);
      }
      return els;
    },
  };
};

// Suppress the first-run onboarding tour (see parity-diff.mjs's newPage): otherwise the walk's own
// elapsed time crosses useOnboarding.ts's 800ms timer, opening a real Radix Dialog that legitimately
// locks `body` with `pointer-events: none` for as long as it stays open and unexplored-dismissed.
const suppressOnboarding = (page) => page.addInitScript(() => localStorage.setItem("stapler-squad:onboarded", "true"));

const page = await (await browser.newContext({ viewport: { width: 1280, height: 800 } })).newPage();
await suppressOnboarding(page);
await page.goto(bunBase + "/", { waitUntil: "load" });
await page.evaluate(pageHelpers);
for (const route of order) {
  await page.evaluate((p) => {
    history.pushState(null, "", p === "/" ? "/" : p + "/"); // "/" + "/" is a protocol-relative URL, not a path
    window.dispatchEvent(new PopStateEvent("popstate", { state: null }));
  }, route);
  await page.waitForTimeout(120);
}
await page.waitForTimeout(300);
const walked = await page.evaluate(() => window.__ssq.snap());
await page.context().close();

const page2 = await (await browser.newContext({ viewport: { width: 1280, height: 800 } })).newPage();
await suppressOnboarding(page2);
const finalRoute = order[order.length - 1];
await page2.goto(bunBase + (finalRoute === "/" ? "/" : finalRoute + "/"), { waitUntil: "load" });
await page2.evaluate(pageHelpers);
await page2.waitForTimeout(300);
const cold = await page2.evaluate(() => window.__ssq.snap());
await page2.context().close();
await browser.close();

console.log("final route:", finalRoute);
for (const key of new Set([...Object.keys(walked), ...Object.keys(cold)])) {
  if (!walked[key] || !cold[key]) {
    console.log("STRUCT", key, walked[key] ? "onlyWalked" : "onlyCold");
    continue;
  }
  for (const p of new Set([...Object.keys(walked[key]), ...Object.keys(cold[key])])) {
    if (walked[key][p] !== cold[key][p]) console.log("PROP", key, p, "cold=", cold[key][p], "walked=", walked[key][p]);
  }
}
