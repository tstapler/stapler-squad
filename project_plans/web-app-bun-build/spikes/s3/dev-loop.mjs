// Gate S3, Task 0.4f: `next dev` baseline. Cold start to first 200 on `/`, then n edits each of (a) a visible string in
// /history and (b) a vanilla-extract token in theme.css.ts, timed from file write to the DOM/computed-style change.
// Run from web-app/:  node ../project_plans/web-app-bun-build/spikes/s3/dev-loop.mjs <port 62873-62880> <backendPort> <out.json> [n=5]
// Edits are reverted in `finally`; check `git status` after a crash.
import { spawn } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { launch } from "./pw.mjs";

const [port, backend, outFile, nArg] = process.argv.slice(2);
const N = Number(nArg ?? 5);
if (Number(port) < 62873 || Number(port) > 62880) throw new Error("port outside the manual block");
const base = `http://localhost:${port}`;
const TEXT_FILE = "src/app/history/page.tsx";
const CSS_FILE = "src/styles/theme.css.ts";

const median = (a) => [...a].sort((x, y) => x - y)[Math.floor(a.length / 2)];
const env = { ...process.env, PORT: port, NEXT_PUBLIC_API_URL: `http://localhost:${backend}/api` };
delete env.npm_config_user_agent;

const t0 = Date.now();
const dev = spawn("bun", ["run", "dev"], { env, stdio: "ignore", detached: true });
const original = { text: readFileSync(TEXT_FILE, "utf8"), css: readFileSync(CSS_FILE, "utf8") };
const result = { n: N };
let browser;
try {
  for (let ok = false; !ok; ) {
    await new Promise((r) => setTimeout(r, 300));
    ok = await fetch(base + "/").then((r) => r.status === 200, () => false);
    if (Date.now() - t0 > 180000) throw new Error("dev server did not answer 200 in 180 s");
  }
  result.coldStartToFirst200Ms = Date.now() - t0;
  browser = await launch();
  const page = await (await browser.newContext({ viewport: { width: 1280, height: 800 } })).newPage();
  await page.goto(base + "/history", { waitUntil: "load", timeout: 120000 });
  await page.waitForSelector("h1", { timeout: 120000 });
  await page.waitForTimeout(3000); // let the first compile settle

  const h1Marker = (m) => page.waitForFunction((x) => document.querySelector("h1")?.textContent?.includes(x), m, { timeout: 60000, polling: 10 });
  const textTimes = [];
  const h1 = original.text.match(/(Claude History Browser)/)?.[1];
  if (!h1) throw new Error("h1 anchor not found in history page");
  for (let i = 0; i < N; i++) {
    const marker = `S3x${i}`;
    const t = Date.now();
    writeFileSync(TEXT_FILE, original.text.replace(h1, `${h1} ${marker}`));
    await h1Marker(marker);
    textTimes.push(Date.now() - t);
    writeFileSync(TEXT_FILE, original.text);
    await h1Marker("Claude History Browser").catch(() => {});
    await page.waitForTimeout(500);
  }
  result.textEditToDomMs = { runs: textTimes, median: median(textTimes), min: Math.min(...textTimes), max: Math.max(...textTimes) };

  // the legacy globals.css owns body's colour, so watch for any element painting the new token value instead
  const anyElementWith = (rgb) => page.evaluate((c) => [...document.querySelectorAll("*")].some((e) => getComputedStyle(e).backgroundColor === c), rgb).catch(() => false);
  const cleanBlock = original.css.indexOf("export const cleanTheme");
  const bgMatch = original.css.slice(cleanBlock).match(/cardBackground:\s*"(#[0-9a-fA-F]{3,8})"/);
  if (!bgMatch) throw new Error("clean theme cardBackground token not found");
  const cssTimes = [];
  for (let i = 0; i < N; i++) {
    const colour = i % 2 === 0 ? "#123456" : "#654321";
    const rgb = i % 2 === 0 ? "rgb(18, 52, 86)" : "rgb(101, 67, 33)";
    const t = Date.now();
    writeFileSync(CSS_FILE, original.css.slice(0, cleanBlock) + original.css.slice(cleanBlock).replace(bgMatch[0], `cardBackground: "${colour}"`));
    let done = null;
    while (done === null && Date.now() - t < 60000) {
      if (await anyElementWith(rgb)) done = Date.now() - t;
      else await new Promise((r) => setTimeout(r, 10));
    }
    cssTimes.push(done);
    await page.waitForTimeout(500);
  }
  result.cssTokenEditToComputedStyleMs = { runs: cssTimes, median: median(cssTimes.filter((x) => x !== null)), timeouts: cssTimes.filter((x) => x === null).length };
} finally {
  writeFileSync(TEXT_FILE, original.text);
  writeFileSync(CSS_FILE, original.css);
  await browser?.close();
  try {
    process.kill(-dev.pid);
  } catch {
    /* already gone */
  }
}
writeFileSync(outFile, JSON.stringify(result, null, 1));
console.log(JSON.stringify(result));
