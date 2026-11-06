import { getSourceFromVirtualCssFile } from "@vanilla-extract/integration";
import { readFileSync } from "node:fs";
const a = JSON.parse(readFileSync(".ve/map-node.json", "utf8")), b = JSON.parse(readFileSync(".ve/map-bun.json", "utf8"));
const re = /["'](?:[^"']*\.vanilla\.css\?source=[^"']+)["']/g;
async function norm(s) { const parts = [...s.matchAll(re)].map((m) => m[0].slice(1, -1)); const css = []; for (const p of parts) css.push((await getSourceFromVirtualCssFile(p)).source); return { js: s.replace(re, '"@CSS@"'), css }; }
let same = 0, jsDiff = 0, cssDiff = 0;
for (const f of Object.keys(a)) { const x = await norm(a[f]), y = await norm(b[f]); if (x.js !== y.js) jsDiff++; else if (JSON.stringify(x.css) !== JSON.stringify(y.css)) cssDiff++; else same++; }
console.log({ same, jsDiff, cssDiff });
