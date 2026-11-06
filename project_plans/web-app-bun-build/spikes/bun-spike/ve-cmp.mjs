import { compile, processVanillaFile } from "@vanilla-extract/integration";
import { globSync, writeFileSync } from "node:fs";
const rt = process.versions.bun ? "bun" : "node";
const t0 = performance.now();
const files = globSync("src/**/*.css.ts").map((f) => process.cwd() + "/" + f).sort();
const out = {}; let i = 0, fail = 0;
async function w() { while (i < files.length) { const f = files[i++]; try {
  const r = await compile({ filePath: f, cwd: process.cwd(), identOption: "short" });
  out[f] = await processVanillaFile({ source: r.source, filePath: f, identOption: "short" }); } catch (e) { fail++; } } }
await Promise.all(Array.from({ length: 8 }, w));
writeFileSync(`.ve/map-${rt}.json`, JSON.stringify(out));
console.log(rt, files.length, "files", fail, "failed", ((performance.now() - t0) / 1000).toFixed(1) + "s");
