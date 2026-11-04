import assert from "node:assert/strict";
import { writeFileSync, rmSync } from "node:fs";
import { test } from "node:test";
import { buildBaseline, classify, mergeRoutesVisited, loadBuildCoupledSpecs } from "./classify-e2e.mjs";

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

test("mergeRoutesVisited merges JSONL route records by spec file, sorted and deduped", () => {
  const jsonl = "/tmp/classify-e2e-test-routes.jsonl";
  writeFileSync(jsonl, [
    JSON.stringify({ specFile: "a.spec.ts", routes: ["/b", "/a"] }),
    JSON.stringify({ specFile: "a.spec.ts", routes: ["/a", "/c"] }),
  ].join("\n") + "\n");
  const specs = [{ spec: "a.spec.ts", title: "t1", routesVisited: null }, { spec: "other.spec.ts", title: "t2", routesVisited: null }];
  mergeRoutesVisited(specs, jsonl);
  assert.deepEqual(specs[0].routesVisited, ["/a", "/b", "/c"]);
  assert.equal(specs[1].routesVisited, null);
  rmSync(jsonl);
});

test("mergeRoutesVisited is a no-op when the JSONL path is missing or absent", () => {
  const specs = [{ spec: "a.spec.ts", title: "t1", routesVisited: null }];
  mergeRoutesVisited(specs, undefined);
  mergeRoutesVisited(specs, "/tmp/does-not-exist-classify-e2e.jsonl");
  assert.equal(specs[0].routesVisited, null);
});

test("loadBuildCoupledSpecs reads the committed build-coupled-specs.json inventory", () => {
  const list = loadBuildCoupledSpecs(new URL("../../../../tests/e2e/baseline/build-coupled-specs.json", import.meta.url).pathname);
  assert.ok(list.length >= 8, `expected at least 8 build-coupled specs, got ${list.length}`);
  assert.ok(list.some((s) => s.spec === "jules-status-badge.spec.ts"));
});
