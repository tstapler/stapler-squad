// Task 7.10e: a route whose only covering spec is excluded from the Bun gate (baseline-`fail`,
// baseline-`flaky`, or listed in bun-expected-diff.json) must fail this manifest's check
// (pre-mortem failure mode 6).
import * as fs from "fs";
import * as path from "path";
import { buildManifest, extractRoutePaths, type BaselineFile, type ExpectedDiffFile } from "../../../scripts/route-coverage-manifest";

describe("extractRoutePaths", () => {
  test("matches the real ROUTE_META key count (45 today; plan text says 42 -- stale, reconciled here)", () => {
    const src = fs.readFileSync(path.join(__dirname, "../../../web-app/src/app/route-meta.ts"), "utf8");
    const routes = extractRoutePaths(src);
    expect(routes.length).toBe(45);
    expect(routes).toContain("/backlog/board");
  });
});

function spec(file: string, title: string, cls: "pass" | "flaky" | "fail" | "skipped", routesVisited: string[]) {
  return { spec: file, project: "chromium", title, class: cls, routesVisited };
}

describe("buildManifest", () => {
  test("a route visited only by a baseline-fail spec is uncovered", () => {
    const baseline: BaselineFile = { specs: [spec("a.spec.ts", "a test", "fail", ["/history"])] };
    const expectedDiff: ExpectedDiffFile = { entries: [] };
    const manifest = buildManifest(["/history"], baseline, expectedDiff);
    expect(manifest.uncoveredRoutes).toEqual(["/history"]);
  });

  test("a route visited only by a baseline-flaky spec is uncovered", () => {
    const baseline: BaselineFile = { specs: [spec("a.spec.ts", "a test", "flaky", ["/history"])] };
    const expectedDiff: ExpectedDiffFile = { entries: [] };
    const manifest = buildManifest(["/history"], baseline, expectedDiff);
    expect(manifest.uncoveredRoutes).toEqual(["/history"]);
  });

  test("a route visited only by a spec listed in bun-expected-diff.json is uncovered even if baseline class is pass", () => {
    const baseline: BaselineFile = { specs: [spec("a.spec.ts", "a test", "pass", ["/history"])] };
    const expectedDiff: ExpectedDiffFile = { entries: [{ spec: "a.spec.ts", test: "a test", issue: "https://x/1" }] };
    const manifest = buildManifest(["/history"], baseline, expectedDiff);
    expect(manifest.uncoveredRoutes).toEqual(["/history"]);
  });

  test("a route with at least one non-excluded (pass, unlisted) covering spec is covered even if another visiting spec is excluded", () => {
    const baseline: BaselineFile = {
      specs: [spec("a.spec.ts", "a test", "fail", ["/history"]), spec("route-smoke.spec.ts", "loads /history cleanly", "pass", ["/history"])],
    };
    const expectedDiff: ExpectedDiffFile = { entries: [] };
    const manifest = buildManifest(["/history"], baseline, expectedDiff);
    expect(manifest.uncoveredRoutes).toEqual([]);
    expect(manifest.routes[0].covered).toBe(true);
  });

  test("a route with no visiting spec at all is uncovered", () => {
    const baseline: BaselineFile = { specs: [] };
    const expectedDiff: ExpectedDiffFile = { entries: [] };
    const manifest = buildManifest(["/orphan"], baseline, expectedDiff);
    expect(manifest.uncoveredRoutes).toEqual(["/orphan"]);
  });

  test("a trailing-slash visit normalizes to match the bare ROUTE_META key", () => {
    const baseline: BaselineFile = { specs: [spec("route-smoke.spec.ts", "loads /history/ cleanly", "pass", ["/history/"])] };
    const expectedDiff: ExpectedDiffFile = { entries: [] };
    const manifest = buildManifest(["/history"], baseline, expectedDiff);
    expect(manifest.uncoveredRoutes).toEqual([]);
  });
});
