// Gate S2 HMR/overlay/memory probes against a REAL `bun web-app/build/dev.ts`. Usage: node hmr.mjs <fastrefresh|csstoken|overlay|memory>
// Env: S2_URL (default http://localhost:62875), S2_WEB_APP (worktree web-app dir), S2_PW (playwright-core dir).
// Every probe edits a real source file and restores it in a finally block, even on throw.
import { createRequire } from "node:module";
import { readFileSync, writeFileSync } from "node:fs";
import path from "node:path";

const webApp = process.env.S2_WEB_APP ?? path.resolve(import.meta.dirname, "../../../../web-app");
const pwDir = process.env.S2_PW ?? path.resolve(webApp, "../tests/e2e/node_modules/playwright-core");
const { chromium } = createRequire(import.meta.url)(pwDir);
const URL0 = process.env.S2_URL ?? "http://localhost:62875";
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function withEdit(file, mutate, fn) {
  const abs = path.join(webApp, file);
  const original = readFileSync(abs, "utf8");
  try {
    writeFileSync(abs, mutate(original));
    return await fn();
  } finally {
    writeFileSync(abs, original);
  }
}

async function openBrowser() {
  const browser = await chromium.launch({ args: ["--use-gl=swiftshader", "--disable-gpu-sandbox"] });
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 800 } });
  await ctx.addInitScript(() => localStorage.setItem("stapler-squad:backlog-onboarded", "true"));
  const page = await ctx.newPage();
  let loads = 0;
  const consoleMsgs = [];
  page.on("load", () => loads++);
  page.on("console", (m) => consoleMsgs.push(`${m.type()}: ${m.text()}`));
  page.on("pageerror", (e) => consoleMsgs.push(`pageerror: ${e}`));
  return { browser, page, loads: () => loads, consoleMsgs };
}

/** Never throws: wraps a probe step and returns { ok, error, bodySnippet } on failure for real diagnostics. */
async function safely(page, fn) {
  try {
    return await fn();
  } catch (e) {
    const bodySnippet = await page.evaluate(() => document.body?.innerText?.slice(0, 500)).catch(() => null);
    return { ok: false, error: String(e).split("\n")[0], bodySnippet };
  }
}

async function fastrefresh() {
  const { browser, page, loads, consoleMsgs } = await openBrowser();
  const results = {};
  try {
    // 1. Session list: "show archived" toggle (a real piece of component state in SessionList.tsx).
    results.sessionList = await safely(page, async () => {
      await page.goto(URL0 + "/", { waitUntil: "load" });
      await page.waitForSelector('[aria-label^="Filters"]', { timeout: 15000 });
      // The toggle lives in a collapsible panel (CSS-hidden until opened, even at desktop width in this viewport).
      await page.click('[aria-label^="Filters"]');
      await page.waitForSelector('[data-testid="show-archived-toggle"]', { state: "visible", timeout: 5000 });
      await page.click('[data-testid="show-archived-toggle"]');
      const before = loads();
      return withEdit("src/components/sessions/SessionList.tsx", (s) => s + "\n// s2-hmr-marker\n", async () => {
        await sleep(2000);
        return {
          ok: true,
          checked: await page.isChecked('[data-testid="show-archived-toggle"]', { timeout: 5000 }).catch(() => null),
          reloaded: loads() !== before,
        };
      });
    });

    // 2. Omnibar: open with Ctrl+K, type into the source input.
    results.omnibar = await safely(page, async () => {
      await page.keyboard.press("Control+k");
      const input = page.getByRole("combobox", { name: "Session source input" });
      await input.waitFor({ timeout: 5000 });
      await input.fill("s2-hmr-canary");
      const before = loads();
      const r = await withEdit("src/components/sessions/Omnibar.tsx", (s) => s + "\n// s2-hmr-marker\n", async () => {
        await sleep(2000);
        return { ok: true, value: await input.inputValue().catch(() => null), reloaded: loads() !== before };
      });
      await page.keyboard.press("Escape");
      return r;
    });

    // 3. Terminal-adjacent page (renders a real @xterm/xterm Terminal instance, same lib as the session terminal pane):
    // select a non-default preset, a real piece of component state in this page.tsx.
    results.terminalStress = await safely(page, async () => {
      await page.goto(URL0 + "/test/terminal-stress", { waitUntil: "load" });
      await page.waitForSelector('[data-testid="preset-selector"]', { timeout: 15000 });
      const options = await page.locator('[data-testid="preset-selector"] option').allTextContents();
      await page.selectOption('[data-testid="preset-selector"]', { index: 1 });
      const chosen = await page.$eval('[data-testid="preset-selector"]', (el) => el.value);
      const before = loads();
      return withEdit("src/app/test/terminal-stress/page.tsx", (s) => s + "\n// s2-hmr-marker\n", async () => {
        await sleep(2000);
        return { ok: true, value: await page.$eval('[data-testid="preset-selector"]', (el) => el.value).catch(() => null), chosen, reloaded: loads() !== before, options };
      });
    });
  } finally {
    await browser.close();
  }
  console.log(JSON.stringify({ results, consoleTail: consoleMsgs.slice(-30) }, null, 1));
}

// The plan's AC names theme-contract.css.ts, but that file is only `createThemeContract` placeholders (no
// literal values — see its own header comment); the dependent literal colour per theme lives in theme.css.ts
// (also what build/config.ts's `themeFile` points at). We test both: the contract file for "does an edit to
// it even propagate" (it has no visible colour to check), and theme.css.ts for the AC's actual colour check.
async function csstoken() {
  const { browser, page, loads } = await openBrowser();
  try {
    await page.goto(URL0 + "/", { waitUntil: "load" });
    // `.page` (src/app/page.css.ts) is the only DOM element observed to set `color: vars.color.textPrimary` directly
    // (globals.css's `body { color: var(--foreground) }` is a plain hardcoded var, not a vanilla-extract dependant).
    await page.waitForSelector(".page, [class*='page_']", { timeout: 15000 });
    const sample = () => page.evaluate(() => getComputedStyle(document.querySelector("[class*='page_']") ?? document.body).color.trim());
    const before = await sample();
    const beforeLoads = loads();

    const contractFileTriggersSomething = await withEdit("src/styles/theme-contract.css.ts", (s) => s + "\n// s2-hmr-marker\n", async () => {
      await sleep(1500);
      return { reloaded: loads() !== beforeLoads };
    });

    const beforeLoads2 = loads();
    const themeFile = await withEdit("src/styles/theme.css.ts", (s) => s.replace('textPrimary: "#e2e8f0",', 'textPrimary: "#ff00ff",'), async () => {
      for (let i = 0; i < 20; i++) {
        await sleep(100);
        const v = await sample().catch(() => before);
        if (v !== before) return { value: v, waitedMs: (i + 1) * 100, reloaded: loads() !== beforeLoads2 };
      }
      return { value: await sample().catch(() => before), waitedMs: 2000, reloaded: loads() !== beforeLoads2 };
    });
    console.log(JSON.stringify({ before, contractFileTriggersSomething, themeFile }, null, 1));
  } finally {
    await browser.close();
  }
}

async function overlay() {
  const { browser, page } = await openBrowser();
  const file = "src/app/test/terminal-stress/page.tsx";
  try {
    await page.goto(URL0 + "/test/terminal-stress", { waitUntil: "load" });
    await page.waitForSelector('[data-testid="preset-selector"]', { timeout: 15000 });

    // Type error / syntax error: an unterminated string breaks the bundler itself.
    const typeErr = await withEdit(file, (s) => s.replace("'use client';", "'use client\nconst __broken = ("), async () => {
      await sleep(1200);
      const shown = await page.evaluate(() => document.querySelector("bun-hmr")?.shadowRoot?.textContent ?? null);
      return { overlayShown: !!shown, mentionsFile: shown?.includes("terminal-stress") ?? false, snippet: shown?.slice(0, 300) };
    });
    await sleep(1200);
    const clearedAfterFix = await page.evaluate(() => {
      const el = document.querySelector("bun-hmr");
      return !el || el.style.display === "none" || !el.shadowRoot?.textContent;
    });

    // Runtime throw: a render-time throw caught by ErrorBoundary (component-level), not a Bun overlay.
    const runtimeErr = await withEdit(file, (s) => s.replace("return (\n    <div", 'if (true) { throw new Error("s2-runtime-canary"); }\n  return (\n    <div'), async () => {
      await sleep(1200);
      const body = await page.evaluate(() => document.body.innerText);
      return { boundaryShown: body.includes("s2-runtime-canary") || /error|something went wrong/i.test(body), snippet: body.slice(0, 400) };
    });
    await page.reload({ waitUntil: "load" });
    await sleep(500);
    const recoveredAfterFix = await page.waitForSelector('[data-testid="preset-selector"]', { timeout: 10000 }).then(() => true).catch(() => false);

    console.log(JSON.stringify({ typeErr, clearedAfterFix, runtimeErr, recoveredAfterFix }, null, 1));
  } finally {
    await browser.close();
  }
}

async function memory() {
  const { browser, page } = await openBrowser();
  const file = "src/app/test/terminal-stress/page.tsx";
  const original = readFileSync(path.join(webApp, file), "utf8");
  const rssOf = async (pid) => {
    try {
      const status = readFileSync(`/proc/${pid}/status`, "utf8");
      return Number(/VmRSS:\s*(\d+)/.exec(status)?.[1] ?? 0) * 1024;
    } catch {
      return null;
    }
  };
  const pid = Number(process.env.S2_DEV_PID);
  try {
    await page.goto(URL0 + "/test/terminal-stress", { waitUntil: "load" });
    await page.waitForSelector('[data-testid="preset-selector"]', { timeout: 15000 });
    const before = pid ? await rssOf(pid) : null;
    let marker = 0;
    for (let i = 0; i < 200; i++) {
      marker++;
      writeFileSync(path.join(webApp, file), original + `\n// s2-mem-${marker}\n`);
      await sleep(60);
    }
    await sleep(1500);
    const after = pid ? await rssOf(pid) : null;
    const stillAlive = pid ? (await rssOf(pid)) !== null : null;
    console.log(JSON.stringify({ edits: marker, beforeRssBytes: before, afterRssBytes: after, stillAlive, pid: pid || null }, null, 1));
  } finally {
    writeFileSync(path.join(webApp, file), original);
    await browser.close();
  }
}

const cmds = { fastrefresh, csstoken, overlay, memory };
const cmd = process.argv[2];
if (!cmds[cmd]) throw new Error(`unknown command ${cmd}; use one of ${Object.keys(cmds).join(", ")}`);
await cmds[cmd]();
