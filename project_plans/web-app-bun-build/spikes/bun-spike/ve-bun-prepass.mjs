import { resolve } from "node:path";
const { compile, processVanillaFile } = await import("@vanilla-extract/integration");
import { globSync } from "node:fs";
import { readFileSync } from "node:fs";
const t0 = performance.now();
const files = globSync("src/**/*.css.ts").map((f) => process.cwd() + "/" + f);
const ref = JSON.parse(readFileSync(".ve/map.json", "utf8"));
let fail = 0, same = 0, diff = 0, i = 0; const errs = {};
async function worker() { while (i < files.length) { const f = files[i++];
  try { const r = await compile({ filePath: f, cwd: process.cwd(), identOption: "short" });
    const out = await processVanillaFile({ source: r.source, filePath: f, identOption: "short" });
    out === ref[f] ? same++ : diff++;
  } catch (e) { fail++; const k = String(e.message).slice(0, 90); errs[k] = (errs[k] || 0) + 1; } } }
await Promise.all(Array.from({ length: 8 }, worker));
console.log(`bun prepass: ${files.length} files, ${fail} failed, identical-to-node ${same}, differing ${diff}, ${((performance.now() - t0) / 1000).toFixed(1)}s`, errs);
