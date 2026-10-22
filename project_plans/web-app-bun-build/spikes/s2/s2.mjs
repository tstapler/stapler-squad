// Gate S2 driver: the real app under `bun web-app/build/dev.ts`. Usage: node s2.mjs <routes|fastrefresh|tokens|overlay|public|cors|memory>
// Env: S2_URL (page origin, default http://localhost:62875), S2_API (backend origin), S2_WEB_APP (worktree web-app dir),
// S2_PW (dir holding playwright-core, default tests/e2e/node_modules). Output is JSON on stdout.
import { createRequire } from "node:module";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";

const webApp = process.env.S2_WEB_APP ?? path.resolve(import.meta.dirname, "../../../../web-app");
const pwDir = process.env.S2_PW ?? path.resolve(webApp, "../tests/e2e/node_modules/playwright-core");
const { chromium } = createRequire(import.meta.url)(pwDir);
const URL0 = process.env.S2_URL ?? "http://localhost:62875";
const API = process.env.S2_API ?? "http://127.0.0.1:62876";
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

function routePaths() {
  const src = readFileSync(path.join(webApp, "src/app/routes.tsx"), "utf8");
  return [...src.matchAll(/^\s+"(\/[^"]*)":\s*\{/gm)].map((m) => m[1]);
}

async function open(url, { viewport = { width: 1280, height: 800 } } = {}) {
  const args = ["--use-gl=swiftshader", "--disable-gpu-sandbox"];
  if (process.env.S2_RESOLVE_LOCALHOST) args.push(`--host-resolver-rules=MAP localhost ${process.env.S2_RESOLVE_LOCALHOST}`);
  const browser = await chromium.launch({ args });
  const ctx = await browser.newContext({ viewport });
  await ctx.addInitScript(() => {
    localStorage.setItem("stapler-squad:backlog-onboarded", "true");
  });
  const page = await ctx.newPage();
  const logs = { errors: [], pageErrors: [], failed: [] };
  page.on("console", (m) => m.type() === "error" && logs.errors.push(`${m.text()} @ ${m.location().url}`));
  page.on("pageerror", (e) => logs.pageErrors.push(String(e)));
  page.on("requestfailed", (r) => logs.failed.push(`${r.url()} ${r.failure()?.errorText}`));
  if (url) await page.goto(url, { waitUntil: "load" });
  return { browser, page, logs };
}

async function routes() {
  const { browser, page, logs } = await open();
  const results = [];
  for (const p of routePaths()) {
    logs.errors.length = logs.pageErrors.length = logs.failed.length = 0;
    let status = "ok";
    try {
      const resp = await page.goto(URL0 + p, { waitUntil: "load" });
      await page.waitForSelector("h1, main, [role=main], #main-content", { timeout: 15000 });
      await sleep(600);
      if (!resp || resp.status() >= 400) status = `http ${resp?.status()}`;
    } catch (e) {
      status = `fail: ${String(e).split("\n")[0]}`;
    }
    const landmark = await page.evaluate(() => !!document.querySelector("h1, main, [role=main]")).catch(() => false);
    results.push({ path: p, final: new URL(page.url()).pathname, status, landmark, errors: [...logs.errors], pageErrors: [...logs.pageErrors], failed: [...logs.failed] });
  }
  await browser.close();
  const bad = results.filter((r) => r.status !== "ok" || !r.landmark || r.errors.length || r.pageErrors.length);
  console.log(JSON.stringify({ total: results.length, ok: results.length - bad.length, bad, results }, null, 1));
}

const cmds = { routes };
const cmd = process.argv[2];
if (!cmds[cmd]) throw new Error(`unknown command ${cmd}`);
await cmds[cmd]();
