// Gate S1: compare two census JSON files from browser.mjs (Next vs Bun). Usage: node census-compare.mjs next.json bun.json
import { readFileSync } from "node:fs";

const [nextFile, bunFile] = process.argv.slice(2);
const a = JSON.parse(readFileSync(nextFile, "utf8"));
const b = JSON.parse(readFileSync(bunFile, "utf8"));
const out = { next: { totalRules: a.totalRules, names: a.customPropertyNames }, bun: { totalRules: b.totalRules, names: b.customPropertyNames }, themes: {} };
let allEqual = true;
for (const t of Object.keys(a.per)) {
  const pa = a.per[t].props;
  const pb = b.per[t].props;
  const common = Object.keys(pa).filter((k) => k in pb);
  const diff = common.filter((k) => pa[k] !== pb[k]);
  const onlyNext = Object.keys(pa).filter((k) => !(k in pb) && pa[k] !== "");
  const onlyBun = Object.keys(pb).filter((k) => !(k in pa) && pb[k] !== "");
  if (diff.length || onlyNext.length || onlyBun.length) allEqual = false;
  out.themes[t] = { classNext: a.per[t].cls, classBun: b.per[t].cls, common: common.length, valueDiffs: diff.length, diffSample: diff.slice(0, 5).map((k) => [k, pa[k], pb[k]]), onlyNextNonEmpty: onlyNext.length, onlyNextSample: onlyNext.slice(0, 5), onlyBunNonEmpty: onlyBun.length, onlyBunSample: onlyBun.slice(0, 5) };
}
console.log(JSON.stringify({ allEqual, ...out }, null, 1));
