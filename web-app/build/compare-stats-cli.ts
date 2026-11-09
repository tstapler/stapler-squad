// CI entry: bun build/compare-stats-cli.ts <current stats.json> <baseline dir> [budgets.json]
import { readFileSync } from "node:fs";
import path from "node:path";
import { compareToBaseline, findStatsFile, type GrowthTolerances } from "./compare-stats";

const [currentFile, baselineDir, budgetsFile = path.resolve(__dirname, "budgets.json")] = process.argv.slice(2);
const baselineFile = baselineDir ? findStatsFile(baselineDir, path.basename(currentFile)) : undefined;
if (!baselineFile) {
  console.log(`growth gate skipped: no baseline ${path.basename(currentFile)} under ${baselineDir ?? "(none given)"}; absolute ceilings still apply`);
  process.exit(0);
}
const tolerances = (JSON.parse(readFileSync(budgetsFile, "utf8")) as { growthToleranceBytes: GrowthTolerances }).growthToleranceBytes;
const errors = compareToBaseline(JSON.parse(readFileSync(currentFile, "utf8")), JSON.parse(readFileSync(baselineFile, "utf8")), tolerances);
if (errors.length > 0) {
  console.error(errors.join("\n"));
  process.exit(1);
}
console.log(`growth gate passed against ${baselineFile}`);
