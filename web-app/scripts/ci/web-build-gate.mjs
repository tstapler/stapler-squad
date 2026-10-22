// Aggregate required check for the dual web build (plan decision 10, Story 5.11).
// Env: NEEDS_JSON = toJSON(needs); MEMBERS = vars.WEB_BUILD_GATE_MEMBERS (comma list of job ids; empty = default).
// A member's failure or cancellation fails the gate; success and skipped (irrelevant PR) pass; non-members only report.
import { appendFileSync } from "node:fs";

export const DEFAULT_MEMBERS = ["detect-changes", "web-build-smoke-next", "e2e-next"];
const FAILING = new Set(["failure", "cancelled"]);
const PASSING = new Set(["success", "skipped"]);

export function evaluate(needs, membersEnv) {
  const explicit = (membersEnv ?? "").split(",").map((s) => s.trim()).filter(Boolean);
  const isDefault = explicit.length === 0;
  // The default may name jobs a later PR creates (e2e-next); an explicit list must not (typo guard).
  const members = isDefault ? DEFAULT_MEMBERS.filter((m) => m in needs) : explicit;
  const failures = [];
  const lines = [];
  for (const name of members) {
    if (!(name in needs)) {
      failures.push(`${name}: listed in WEB_BUILD_GATE_MEMBERS but is not a job in needs`);
      continue;
    }
    const result = needs[name]?.result;
    if (FAILING.has(result) || !PASSING.has(result)) failures.push(`${name}: ${result ?? "no result"}`);
    lines.push(`| ${name} | ${result} | member |`);
  }
  for (const name of Object.keys(needs).filter((n) => !members.includes(n))) lines.push(`| ${name} | ${needs[name]?.result} | not a member (informational) |`);
  return { failures, lines };
}

function main() {
  const needs = JSON.parse(process.env.NEEDS_JSON ?? "{}");
  const { failures, lines } = evaluate(needs, process.env.MEMBERS);
  const summary = ["### web-build-gate", "", "| job | result | role |", "| --- | --- | --- |", ...lines, "", failures.length ? `FAILED:\n${failures.map((f) => `- ${f}`).join("\n")}` : "All members green or skipped."].join("\n");
  if (process.env.GITHUB_STEP_SUMMARY) appendFileSync(process.env.GITHUB_STEP_SUMMARY, `${summary}\n`);
  console.log(summary);
  process.exit(failures.length ? 1 : 0);
}

if (process.argv[1] && import.meta.url === new URL(`file://${process.argv[1]}`).href) main();
