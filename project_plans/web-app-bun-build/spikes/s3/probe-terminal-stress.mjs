// Repro for the /test/terminal-stress `ReferenceError: __commonJS is not defined` crash (see spike-results.md's
// S3 section). Reproduces on both minified and unminified Bun builds: page-<hash>.js's import statement omits
// `__commonJS`, which its inlined `require_xterm2 = __commonJS(...)` (the @xterm/xterm/css/xterm.css side-effect
// wrapper) needs, even though every other chunk that calls __commonJS imports it correctly.
// Run from web-app/:  node ../project_plans/web-app-bun-build/spikes/s3/probe-terminal-stress.mjs <bunBase>
import { launch } from "./pw.mjs";

const base = process.argv[2];
const b = await launch();
const p = await b.newPage();
const errs = [];
p.on("pageerror", (e) => errs.push(String(e).slice(0, 400)));
p.on("console", (m) => m.type() === "error" && /ReferenceError|React Router caught/.test(m.text()) && errs.push("console: " + m.text().slice(0, 300)));
await p.goto(base + "/test/terminal-stress/", { waitUntil: "load" });
await p.waitForTimeout(2000);
console.log("h1:", await p.evaluate(() => document.querySelector("h1")?.textContent ?? null));
console.log("errors:", JSON.stringify(errs, null, 1));
await b.close();
process.exit(errs.length > 0 ? 1 : 0);
