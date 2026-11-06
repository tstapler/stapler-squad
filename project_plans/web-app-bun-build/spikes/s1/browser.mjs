// Gate S1 browser probes (throwaway). Run from the repo root:
//   node browser.mjs login  <baseUrl>               unauthenticated /login request list (TLS errors ignored)
//   node browser.mjs census <baseUrl> <out.json>    resolved custom-property map per theme class + rule counts
import { createRequire } from "node:module";
import { writeFileSync } from "node:fs";
import { resolve } from "node:path";

const require = createRequire(resolve("tests/e2e/package.json"));
const { chromium } = require("@playwright/test");

const [mode, base, outFile] = process.argv.slice(2);
const browser = await chromium.launch();
const ctx = await browser.newContext({ ignoreHTTPSErrors: true });
const page = await ctx.newPage();
const consoleErrors = [];
page.on("console", (m) => m.type() === "error" && consoleErrors.push(m.text().slice(0, 200)));
page.on("pageerror", (e) => consoleErrors.push("pageerror: " + String(e).slice(0, 200)));

if (mode === "login") {
  const reqs = [];
  page.on("response", (r) => reqs.push({ url: new URL(r.url()).pathname, status: r.status(), type: r.request().resourceType(), ct: (r.headers()["content-type"] ?? "").split(";")[0] }));
  const resp = await page.goto(base + "/login", { waitUntil: "load" });
  await page.waitForTimeout(4000);
  const exempt = ["/auth/", "/login", "/health", "/_next/", "/favicon"];
  const outside = reqs.filter((r) => !exempt.some((p) => r.url.startsWith(p)));
  const byType = {};
  for (const r of reqs) byType[r.type + ":" + r.status] = (byType[r.type + ":" + r.status] ?? 0) + 1;
  const outsideSummary = {};
  for (const r of outside) outsideSummary[r.url + " -> " + r.status] = (outsideSummary[r.url + " -> " + r.status] ?? 0) + 1;
  console.log(JSON.stringify({ base, loginStatus: resp.status(), finalUrl: new URL(page.url()).pathname, requestCount: reqs.length, byTypeStatus: byType, requestsOutsideExemptPrefixes: outsideSummary, consoleErrors, processNotDefined: consoleErrors.some((e) => /process is not defined/.test(e)) }, null, 1));
}

if (mode === "census") {
  await page.goto(base + "/", { waitUntil: "load" });
  await page.waitForTimeout(4000);
  const result = await page.evaluate(async () => {
    let themes = window.__themes;
    if (!themes) {
      const m = document.documentElement.outerHTML.match(/var m=(\{[^}]*\})/) ?? (await (await fetch("/")).text()).match(/var m=(\{[^}]*\})/);
      themes = m ? JSON.parse(m[1]) : null;
    }
    const countRules = (rules) => {
      let n = 0;
      for (const r of rules) n += r.cssRules ? 1 + countRules(r.cssRules) : 1;
      return n;
    };
    let totalRules = 0;
    const names = new Set();
    const walk = (rules) => {
      for (const r of rules) {
        if (r.style) for (const p of r.style) if (p.startsWith("--")) names.add(p);
        if (r.cssRules) walk(r.cssRules);
      }
    };
    for (const s of document.styleSheets) {
      try {
        totalRules += countRules(s.cssRules);
        walk(s.cssRules);
      } catch {}
    }
    const root = document.documentElement;
    const per = {};
    for (const [t, cls] of Object.entries(themes)) {
      for (const c of Object.values(themes)) for (const x of c.split(" ")) root.classList.remove(x);
      for (const x of cls.split(" ")) root.classList.add(x);
      const cs = getComputedStyle(root);
      const props = {};
      const probe = document.createElement("span");
      document.body.appendChild(probe);
      // Bun's CSS minifier rewrites colour notation (#00cc33 -> #0c3); compare resolved colours, not source text.
      const canon = (v) => {
        for (const prop of ["color", "box-shadow", "transition"]) {
          if (!v || !CSS.supports(prop, v)) continue;
          probe.style.cssText = "";
          probe.style.setProperty(prop, v);
          return prop + ":" + getComputedStyle(probe).getPropertyValue(prop);
        }
        return v.replace(/\s*,\s*/g, ",").replace(/["']/g, "'").replace(/\s+/g, " ");
      };
      for (const n of names) props[n] = canon(cs.getPropertyValue(n).trim());
      probe.remove();
      const themeRules = [];
      const find = (rules) => {
        for (const r of rules) {
          if (r.selectorText && r.selectorText.includes("." + cls.split(" ")[0])) themeRules.push(r.selectorText);
          if (r.cssRules) find(r.cssRules);
        }
      };
      for (const s of document.styleSheets) {
        try {
          find(s.cssRules);
        } catch {}
      }
      per[t] = { cls, propCount: Object.keys(props).length, nonEmpty: Object.values(props).filter(Boolean).length, rulesMentioningClass: themeRules.length, props };
    }
    return { rootChildren: document.getElementById("root")?.childElementCount ?? null, bodyTextLength: document.body.innerText.length, totalRules, sheetCount: document.styleSheets.length, customPropertyNames: names.size, per };
  });
  writeFileSync(outFile, JSON.stringify({ base, consoleErrors, ...result }));
  console.log(JSON.stringify({ base, rootChildren: result.rootChildren, bodyTextLength: result.bodyTextLength, totalRules: result.totalRules, sheetCount: result.sheetCount, customPropertyNames: result.customPropertyNames, consoleErrors, perTheme: Object.fromEntries(Object.entries(result.per).map(([t, v]) => [t, { cls: v.cls, nonEmpty: v.nonEmpty, rulesMentioningClass: v.rulesMentioningClass }])) }));
}
await browser.close();
