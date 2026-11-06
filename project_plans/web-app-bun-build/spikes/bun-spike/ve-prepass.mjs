// Node prepass: evaluate every *.css.ts with VE's compile() and cache the transformed JS.
import { compile, processVanillaFile } from "@vanilla-extract/integration";
import { globSync } from "node:fs";
import { writeFileSync, mkdirSync } from "node:fs";
const t0 = performance.now();
const files = globSync("src/**/*.css.ts").map((f) => process.cwd() + "/" + f);
const out = {}; let fail = 0;
const conc = 8; let i = 0;
async function worker() {
  while (i < files.length) {
    const f = files[i++];
    try {
      const r = await compile({ filePath: f, cwd: process.cwd() });
      out[f] = await processVanillaFile({ source: r.source, filePath: f, identOption: "short" });
    } catch (e) { fail++; console.log("FAIL", f.split("/src/")[1], String(e.message).slice(0, 120)); }
  }
}
await Promise.all(Array.from({ length: conc }, worker));
mkdirSync(".ve", { recursive: true });
writeFileSync(".ve/map.json", JSON.stringify(out));
console.log(`prepass: ${files.length} files, ${fail} failed, ${((performance.now() - t0) / 1000).toFixed(1)}s`);
