// Probe: which stylesheets load per route cold, and do lazy-chunk CSS files get linked at all?
// Run from web-app/:  node ../project_plans/web-app-bun-build/spikes/s3/probe-css.mjs <baseUrl> <route...>
import { launch } from "./pw.mjs";

const [base, ...routes] = process.argv.slice(2);
const browser = await launch();
const out = [];
for (const route of routes) {
  const page = await (await browser.newContext({ viewport: { width: 1280, height: 800 } })).newPage();
  const errors = [];
  page.on("pageerror", (e) => errors.push(String(e).slice(0, 160)));
  const cssReqs = [];
  page.on("response", (r) => r.url().endsWith(".css") && cssReqs.push(new URL(r.url()).pathname.split("/").pop()));
  await page.goto(base + route, { waitUntil: "networkidle" });
  const info = await page.evaluate(() => ({
    sheets: document.styleSheets.length,
    links: [...document.querySelectorAll("link[rel=stylesheet]")].map((l) => l.getAttribute("href")?.split("/").pop()),
    h1: document.querySelector("h1")?.textContent?.slice(0, 40) ?? null,
    bodyText: document.body.innerText.length,
  }));
  out.push({ route, cssReqs, ...info, errors });
  await page.context().close();
}
await browser.close();
console.log(JSON.stringify(out, null, 1));
