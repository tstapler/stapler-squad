// CI entry: bun build/compare-stats-cli.ts <current stats.json> <baseline dir> [budgets.json]
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import { compareToBaseline, findStatsFile, type GrowthTolerances } from "./compare-stats";

const [currentFile, baselineDir, budgetsFile = path.resolve(__dirname, "budgets.json")] = process.argv.slice(2);
if (!currentFile || !existsSync(currentFile)) {
  console.error(`growth gate: current stats file ${currentFile} is missing (did the build step fail?)`);
  process.exit(1);
}
const baselineFile = baselineDir ? findStatsFile(baselineDir, path.basename(currentFile)) : undefined;
if (!baselineFile) {
  console.log(`growth gate skipped: no baseline ${path.basename(currentFile)} under ${baselineDir ?? "(none given)"}; absolute ceilings still apply`);
  process.exit(0);
}
const tolerances = (JSON.parse(readFileSync(budgetsFile, "utf8")) as { growthToleranceBytes: GrowthTolerances }).growthToleranceBytes;
const METRICS = ["jsBytes", "initialJsBytes", "largestChunkBytes", "largestLazyChunkBytes"] as const;
if (!METRICS.every((m) => typeof tolerances?.[m] === "number")) {
  console.error(`growth gate: ${budgetsFile} growthToleranceBytes must define ${METRICS.join(", ")}`);
  process.exit(1);
}
const errors = compareToBaseline(JSON.parse(readFileSync(currentFile, "utf8")), JSON.parse(readFileSync(baselineFile, "utf8")), tolerances);
if (errors.length > 0) {
  console.error(errors.join("\n"));
  process.exit(1);
}
console.log(`growth gate passed against ${baselineFile}`);
