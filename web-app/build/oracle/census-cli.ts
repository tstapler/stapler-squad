import path from "node:path";
import { compareCensus, sharedCss, themeClassesOf } from "./css-census";

function dirFlag(name: string): string {
  const i = process.argv.indexOf(name);
  if (i === -1 || !process.argv[i + 1]) throw new Error(`missing ${name} <dir>`);
  return path.resolve(process.argv[i + 1]);
}

const bunDir = dirFlag("--bun");
const report = compareCensus(sharedCss(dirFlag("--next")), sharedCss(bunDir), themeClassesOf(bunDir));
console.log(JSON.stringify({ ...report, themeDiffs: report.themeDiffs.slice(0, 20), cascadeDiffs: report.cascadeDiffs.slice(0, 20) }, null, 2));
process.exit(report.themeDiffs.length === 0 && report.cascadeDiffs.length === 0 ? 0 : 1);
