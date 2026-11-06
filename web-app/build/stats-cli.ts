// Prints Markdown tables for stats.json files (CI appends the output to $GITHUB_STEP_SUMMARY).
import { readFileSync } from "node:fs";
import path from "node:path";
import { statsToMarkdown, type BuildStats } from "./stats";

for (const file of process.argv.slice(2)) {
  const stats = JSON.parse(readFileSync(file, "utf8")) as BuildStats;
  process.stdout.write(`${statsToMarkdown(stats, `Bun web build: ${path.basename(file)}`)}\n`);
}
