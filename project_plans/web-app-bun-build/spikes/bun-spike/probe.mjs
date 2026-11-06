import { compile, processVanillaFile } from "@vanilla-extract/integration";
const f = process.cwd() + "/src/components/ui/ErrorState.css.ts";
try {
  const t=performance.now();
  const r = await compile({ filePath: f, cwd: process.cwd() });
  const out = await processVanillaFile({ source: r.source, filePath: f, identOption: "short" });
  console.log(process.versions.bun ? "bun" : "node", "OK", r.watchFiles.length, out.length, (performance.now()-t).toFixed(0)+"ms");
} catch (e) { console.log(process.versions.bun ? "bun" : "node", "ERR", String(e.message).slice(0,200)); }
