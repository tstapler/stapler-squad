import assert from "node:assert/strict";
import { test } from "node:test";
import { buildBaseline, classify } from "./classify-e2e.mjs";

const att = (...statuses) => statuses.map((status, retry) => ({ retry, status }));
const report = (results) => ({ suites: [{ title: "a.spec.ts", file: "a.spec.ts", specs: [{ title: "t", file: "a.spec.ts", tests: [{ projectName: "chromium", results }] }] }] });

test("first-fail-then-pass is flaky, not pass", () => {
  assert.equal(classify([[...att("failed", "passed")], att("passed"), att("passed")]), "flaky");
});
test("passed on attempt 0 in all runs is pass", () => {
  assert.equal(classify([att("passed"), att("passed"), att("passed")]), "pass");
});
test("failed in all runs after the retry is fail", () => {
  assert.equal(classify([att("failed", "failed"), att("failed", "timedOut"), att("failed", "failed")]), "fail");
});
test("differing final outcomes are flaky", () => {
  assert.equal(classify([att("passed"), att("failed", "failed"), att("passed")]), "flaky");
});
test("skipped in every run is skipped", () => {
  assert.equal(classify([att("skipped"), att("skipped"), att("skipped")]), "skipped");
});
test("buildBaseline keeps every attempt of every run", () => {
  const b = buildBaseline([report(att("failed", "passed")), report(att("passed")), report(att("passed"))], { x: 1 });
  assert.equal(b.specs.length, 1);
  assert.equal(b.specs[0].class, "flaky");
  assert.deepEqual(b.specs[0].runs[0].attempts, [{ retry: 0, status: "failed" }, { retry: 1, status: "passed" }]);
  assert.equal(b.meta.counts.flaky, 1);
});
