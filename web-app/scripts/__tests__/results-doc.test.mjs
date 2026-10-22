// Run by `bun run test:codemods` (node --test). Story 8.3 (plan validation A4, REQ-S6): parses
// docs/explanation/bun-migration-results.md and fails the build if it is incomplete, rather than trusting
// whoever wrote it to remember every row by hand.
//
// A row is a markdown table line whose first cell is a backtick-quoted ID, e.g. "| `M1` | ... |"; the
// remaining cells are [metric, value, status, note]. Two independent checks:
//   1. Every footprint row (F1-F6) and the binary-size-delta row (D3) named by Story 8.3's own AC text must
//      be present, and the metrics table's row count ("M*" ids) must equal requirements.md's own Success
//      Metrics bullet count -- so a metric added to requirements.md later, with no matching row added here,
//      is caught instead of silently drifting.
//   2. Every row must carry either a measured number (in its Value cell) or a stated reason (in its Note
//      cell) -- a row with neither (an empty placeholder) fails. This is what lets a genuinely-unmeasured
//      row (Story 9.2's "after" footprint, not yet applied) stay honest ("not yet measurable, pending Story
//      9.2") instead of either being fabricated or silently blank.
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const root = path.join(path.dirname(fileURLToPath(import.meta.url)), "../../..");
const DOC_PATH = path.join(root, "docs/explanation/bun-migration-results.md");
const REQUIREMENTS_PATH = path.join(root, "project_plans/web-app-bun-build/requirements.md");

// Named directly by Story 8.3's AC text ("footprint rows... direct dependency count... installed package
// count... du -sk... bun install... cold and warm"; "the unminified cutover JS bytes"; "binary size delta").
const REQUIRED_IDS = ["F1", "F2", "F3", "F4", "F5", "F6", "D3"];

const HAS_NUMBER = /\d/;
const isReasonStated = (note) => note.replace(/[-—\s]/g, "").length >= 8;

/** Parses backtick-ID pipe-table rows. Returns a Map id -> [metric, value, status, note]. */
export function parseRows(text) {
  const rows = new Map();
  for (const line of text.split("\n")) {
    const m = line.match(/^\|\s*`([A-Za-z0-9]+)`\s*\|(.*)\|\s*$/);
    if (!m) continue;
    rows.set(m[1], m[2].split("|").map((c) => c.trim()));
  }
  return rows;
}

function successMetricsBulletCount(requirementsText) {
  const section = requirementsText.match(/## Success Metrics\n([\s\S]*?)\n## /)?.[1] ?? "";
  return section.split("\n").filter((l) => l.trim().startsWith("-")).length;
}

/** Returns a list of problem strings; empty means the doc is complete. */
export function checkResultsDoc(docText, requirementsText) {
  const rows = parseRows(docText);
  const problems = [];

  const metricIds = [...rows.keys()].filter((id) => /^M\d+$/.test(id));
  const wantMetrics = successMetricsBulletCount(requirementsText);
  if (wantMetrics > 0 && metricIds.length !== wantMetrics) {
    problems.push(
      `requirements.md's Success Metrics section has ${wantMetrics} bullets but the results doc has ${metricIds.length} "M*" rows -- add or remove a row to match`,
    );
  }

  for (const id of REQUIRED_IDS) {
    if (!rows.has(id)) problems.push(`missing required row \`${id}\``);
  }

  for (const [id, cells] of rows) {
    const [, value = "", , note = ""] = cells;
    if (!HAS_NUMBER.test(value) && !isReasonStated(note)) {
      problems.push(`row \`${id}\` has neither a measured number (Value cell) nor a stated reason (Note cell)`);
    }
  }
  return problems;
}

const JOIN_SEP = "; ";
const ONE_BULLET_REQS = "## Success Metrics\n- one metric\n## Next section\n";

test("the shipped results doc is complete", () => {
  const problems = checkResultsDoc(readFileSync(DOC_PATH, "utf8"), readFileSync(REQUIREMENTS_PATH, "utf8"));
  assert.deepEqual(problems, []);
});

// Task 8.3c: a seeded failing fixture, so the checker itself is proven to fail on real gaps.
test("fixture: a missing footprint row is reported", () => {
  const fixture = "| `M1` | Build time | 1.2s | PASS | measured |\n";
  const problems = checkResultsDoc(fixture, ONE_BULLET_REQS);
  assert.ok(problems.some((p) => p.includes("`F1`")), problems.join(JOIN_SEP));
});

test("fixture: a row with no number and no reason is reported", () => {
  const fixture = "| `M1` | Build time |  | PASS |  |\n";
  const problems = checkResultsDoc(fixture, ONE_BULLET_REQS);
  assert.ok(problems.some((p) => p.includes("`M1` has neither")), problems.join(JOIN_SEP));
});

test("fixture: an unmeasured row with a stated reason is not reported as numberless", () => {
  const fixture = "| `M1` | Build time | not yet measurable | PENDING | pending Story 9.2 dependency removal |\n";
  const problems = checkResultsDoc(fixture, ONE_BULLET_REQS);
  assert.ok(!problems.some((p) => p.includes("`M1` has neither")), problems.join(JOIN_SEP));
});

test("fixture: a metrics-table row count that drifts from requirements.md is reported", () => {
  const fixture = "| `M1` | Build time | 1.2s | PASS | measured |\n";
  const problems = checkResultsDoc(fixture, "## Success Metrics\n- one\n- two\n## Next section\n");
  assert.ok(problems.some((p) => p.includes("2 bullets but the results doc has 1")), problems.join(JOIN_SEP));
});
