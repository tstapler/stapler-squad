// Per-route cold-load JS/CSS actually fetched by Chromium (all 44 RoutePath values), for one base URL.
// Run from web-app/:  node ../project_plans/web-app-bun-build/spikes/s3/route-loads.mjs <baseUrl> <routes.json> > loads.json
// first-load JS = every .js response up to networkidle at 1280x800; sizes are the uncompressed body bytes plus gzip -9 of the body.
import { readFileSync } from "node:fs";
import { gzipSync } from "node:zlib";
import { launch } from "./pw.mjs";

const [base, routesFile] = process.argv.slice(2);
const routes = JSON.parse(readFileSync(routesFile, "utf8"));
const CONCURRENCY = 6;
const browser = await launch();

async function loadRoute(route) {
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  const page = await ctx.newPage();
  const js = new Map();
  const css = new Map();
  const errors = [];
  page.on("pageerror", (e) => errors.push(String(e).slice(0, 160)));
  page.on("response", async (r) => {
    const u = new URL(r.url());
    const kind = u.pathname.endsWith(".js") ? js : u.pathname.endsWith(".css") ? css : null;
    if (!kind) return;
    try {
      const body = await r.body();
      kind.set(u.pathname, { raw: body.length, gzip: gzipSync(body, { level: 9 }).length });
    } catch {
      /* body unavailable (redirect or aborted) */
    }
  });
  await page.goto(base + (route === "/" ? "/" : route + "/"), { waitUntil: "networkidle" });
  const state = await page.evaluate(() => ({
    path: location.pathname,
    title: document.title,
    mainChars: (document.querySelector("#main-content")?.textContent ?? document.body.textContent).trim().length,
  }));
  await ctx.close();
  const sum = (m, k) => [...m.values()].reduce((n, v) => n + v[k], 0);
  return { route, ...state, jsFiles: js.size, jsRaw: sum(js, "raw"), jsGzip: sum(js, "gzip"), cssFiles: [...css.keys()], cssRaw: sum(css, "raw"), errors };
}

const results = [];
const queue = [...routes];
await Promise.all(
  Array.from({ length: CONCURRENCY }, async () => {
    for (let r = queue.shift(); r !== undefined; r = queue.shift()) results.push(await loadRoute(r));
  }),
);
await browser.close();
results.sort((a, b) => routes.indexOf(a.route) - routes.indexOf(b.route));
console.log(JSON.stringify(results, null, 1));
