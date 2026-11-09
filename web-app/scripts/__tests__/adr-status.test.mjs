// Run by `bun run test:codemods` (node --test). The web-app-bun-build ADRs are decided; none may sit at Proposed.
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const decisions = path.join(path.dirname(fileURLToPath(import.meta.url)), "../../../project_plans/web-app-bun-build/decisions");

/** True when a decision file's Status line still says Proposed. */
export function isProposed(text) {
  return /^Status:\s*Proposed/m.test(text);
}

test("no web-app-bun-build ADR still says Status: Proposed", () => {
  const files = readdirSync(decisions).filter((n) => /^ADR-\d+.*\.md$/.test(n));
  assert.ok(files.length >= 8, "expected ADR-001..008");
  const proposed = files.filter((n) => isProposed(readFileSync(path.join(decisions, n), "utf8")));
  assert.deepEqual(proposed, []);
});

test("the check can fail: a Proposed status is detected, an Accepted one is not", () => {
  assert.equal(isProposed("# ADR\n\nStatus: Proposed — pending\n"), true);
  assert.equal(isProposed("# ADR\n\nStatus: Accepted\n"), false);
});
