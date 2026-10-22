#!/usr/bin/env node
// Diffs two `eslint -f json` reports as (file, rule id, severity) multisets, to prove a config
// change (here: eslintrc -> flat) did not move any finding. Usage:
//   node scripts/eslint-report-diff.mjs <before.json> <after.json> [--max-percent 2]
// Exit 0 when the diff is empty, or every differing line falls in a named class and the total
// is within --max-percent of the baseline; exit 1 otherwise.
import { readFileSync } from "node:fs";
import path from "node:path";

const CLASSES = {
  "config-newly-linted": (line) => /(^|\/)[^/]*\.config\.[cm]?[jt]s$/.test(line.file) || /(^|\/)\.[^/]*rc\.[cm]?js$/.test(line.file),
  "parser-difference": (line) => line.rule === null,
  "rule-renamed": (line) => line.rule !== null && !line.rule.includes("/") && line.side === "after" && RENAMED.has(line.rule),
};
const RENAMED = new Set();

export function multiset(report, root) {
  const counts = new Map();
  for (const f of report) {
    const file = path.relative(root, f.filePath);
    for (const m of f.messages) {
      const key = JSON.stringify([file, m.ruleId, m.severity]);
      counts.set(key, (counts.get(key) ?? 0) + 1);
    }
  }
  return counts;
}

export function diff(before, after) {
  const lines = [];
  for (const key of new Set([...before.keys(), ...after.keys()])) {
    const delta = (after.get(key) ?? 0) - (before.get(key) ?? 0);
    if (delta === 0) continue;
    const [file, rule, severity] = JSON.parse(key);
    lines.push({ file, rule, severity, delta, side: delta > 0 ? "after" : "before" });
  }
  return lines;
}

export function classify(lines) {
  const classified = { unclassified: [] };
  for (const name of Object.keys(CLASSES)) classified[name] = [];
  for (const line of lines) {
    const name = Object.keys(CLASSES).find((n) => CLASSES[n](line));
    classified[name ?? "unclassified"].push(line);
  }
  return classified;
}

function main(argv) {
  const [beforePath, afterPath] = argv.filter((a) => !a.startsWith("--") && !/^\d+(\.\d+)?$/.test(a));
  const pct = argv.includes("--max-percent") ? Number(argv[argv.indexOf("--max-percent") + 1]) : 2;
  const root = process.cwd();
  const before = multiset(JSON.parse(readFileSync(beforePath, "utf8")), root);
  const after = multiset(JSON.parse(readFileSync(afterPath, "utf8")), root);
  const baseline = [...before.values()].reduce((a, b) => a + b, 0);
  const lines = diff(before, after);
  const total = lines.reduce((a, l) => a + Math.abs(l.delta), 0);
  const classified = classify(lines);
  console.log(JSON.stringify({ baseline, differing: total, byClass: Object.fromEntries(Object.entries(classified).map(([k, v]) => [k, v.length])) }));
  for (const l of lines) console.log(`${l.delta > 0 ? "+" : "-"}${Math.abs(l.delta)} ${l.file} ${l.rule} sev=${l.severity}`);
  const ok = total === 0 || (classified.unclassified.length === 0 && total <= (baseline * pct) / 100);
  process.exit(ok ? 0 : 1);
}

if (import.meta.url === `file://${process.argv[1]}`) main(process.argv.slice(2));
