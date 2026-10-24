// Plan Task 7.10e: maps every RoutePath in web-app/src/app/route-meta.ts's ROUTE_META to the
// specs that visited it (via next-e2e-baseline.json's `routesVisited`, Task 0.1e -- which
// includes tests/e2e/route-smoke.spec.ts, Story 7.14, since it is a real spec in that baseline
// run like any other) and fails if a route's only coverage is baseline-`fail`, baseline-`flaky`
// or listed in bun-expected-diff.json (pre-mortem failure mode 6: a route must not be able to
// hide behind a spec the Bun gate excludes).
//
// ROUTE_META keys are extracted by regex from the source file rather than imported as a TS
// module: scripts/ has no tsconfig path-alias resolution for web-app's "@/*" (web-app's own
// `@/lib/nav/paths` import, pulled in transitively), and ROUTE_META's key set is a flat object
// literal of `"/path": {` lines, which a regex extracts exactly as reliably as a real import
// for this read-only purpose.
//
// Usage:
//   ts-node scripts/route-coverage-manifest.ts \
//     --baseline tests/e2e/baseline/next-e2e-baseline.json \
//     --expected-diff tests/e2e/baseline/bun-expected-diff.json \
//     --route-meta web-app/src/app/route-meta.ts \
//     [--out manifest.json]
import * as fs from "fs";

export interface BaselineSpecEntryWithRoutes {
  spec: string;
  project: string;
  title: string;
  class: "pass" | "flaky" | "fail" | "skipped";
  routesVisited?: string[] | null;
}

export interface BaselineFile {
  meta?: Record<string, unknown>;
  specs: BaselineSpecEntryWithRoutes[];
}

export interface ExpectedDiffEntry {
  spec: string;
  test: string;
  issue?: string;
}

export interface ExpectedDiffFile {
  entries: ExpectedDiffEntry[];
}

export interface RouteCoverage {
  route: string;
  /** Every (spec, title) that visited this route, with why it does or doesn't count as real coverage. */
  visitors: { spec: string; title: string; class: string; disqualified: boolean; reason?: string }[];
  covered: boolean;
}

export interface Manifest {
  routeCount: number;
  uncoveredRoutes: string[];
  routes: RouteCoverage[];
}

/** Extracts the top-level string keys of `export const ROUTE_META = { ... }`. Not a general parser -- relies on ROUTE_META being a flat object literal of `"/path": {` lines, which it is today. */
export function extractRoutePaths(routeMetaSource: string): string[] {
  const marker = "export const ROUTE_META";
  const start = routeMetaSource.indexOf(marker);
  if (start === -1) throw new Error(`"${marker}" not found -- route-meta.ts's shape changed; update this extractor`);
  const openBrace = routeMetaSource.indexOf("{", start);
  const closeBrace = routeMetaSource.indexOf("\n};", openBrace);
  if (openBrace === -1 || closeBrace === -1) throw new Error("could not find ROUTE_META's object literal bounds");
  const body = routeMetaSource.slice(openBrace, closeBrace);
  const keys: string[] = [];
  const re = /^\s*"(\/[^"]*)":\s*\{/gm;
  let m: RegExpExecArray | null;
  while ((m = re.exec(body))) keys.push(m[1]);
  return keys;
}

/** "/foo/" visited -> "/foo" for matching against ROUTE_META keys (root "/" is left alone). */
function normalizeRoute(pathname: string): string {
  return pathname.length > 1 && pathname.endsWith("/") ? pathname.slice(0, -1) : pathname;
}

export function buildManifest(routes: string[], baseline: BaselineFile, expectedDiff: ExpectedDiffFile): Manifest {
  const expectedDiffKeys = new Set(expectedDiff.entries.map((e) => `${e.spec} :: ${e.test}`));
  const byRoute = new Map<string, RouteCoverage["visitors"]>();
  for (const route of routes) byRoute.set(route, []);

  for (const s of baseline.specs) {
    for (const visited of s.routesVisited ?? []) {
      const route = normalizeRoute(visited);
      if (!byRoute.has(route)) continue; // a visited path outside ROUTE_META (e.g. /api/..., a redirect target) -- not this manifest's concern
      const listed = expectedDiffKeys.has(`${s.spec} :: ${s.title}`);
      const disqualified = s.class === "fail" || s.class === "flaky" || s.class === "skipped" || listed;
      const reason = listed ? "listed in bun-expected-diff.json" : s.class !== "pass" ? `baseline class '${s.class}'` : undefined;
      byRoute.get(route)!.push({ spec: s.spec, title: s.title, class: s.class, disqualified, reason });
    }
  }

  const routeCoverages: RouteCoverage[] = routes.map((route) => {
    const visitors = byRoute.get(route) ?? [];
    const covered = visitors.some((v) => !v.disqualified);
    return { route, visitors, covered };
  });

  return {
    routeCount: routes.length,
    uncoveredRoutes: routeCoverages.filter((r) => !r.covered).map((r) => r.route),
    routes: routeCoverages,
  };
}

function parseArgs(argv: string[]): Record<string, string> {
  const out: Record<string, string> = {};
  for (let i = 0; i < argv.length; i += 2) {
    if (argv[i].startsWith("--")) out[argv[i].slice(2)] = argv[i + 1];
  }
  return out;
}

function main(): void {
  const args = parseArgs(process.argv.slice(2));
  const routeMetaSource = fs.readFileSync(args["route-meta"], "utf8");
  const routes = extractRoutePaths(routeMetaSource);
  const baseline: BaselineFile = JSON.parse(fs.readFileSync(args["baseline"], "utf8"));
  const expectedDiff: ExpectedDiffFile = JSON.parse(fs.readFileSync(args["expected-diff"], "utf8"));

  const manifest = buildManifest(routes, baseline, expectedDiff);
  if (args.out) fs.writeFileSync(args.out, JSON.stringify(manifest, null, 1) + "\n");

  if (manifest.uncoveredRoutes.length > 0) {
    console.error(`${manifest.uncoveredRoutes.length}/${manifest.routeCount} routes have no non-excluded covering spec:`);
    for (const r of manifest.uncoveredRoutes) console.error(`  ${r}`);
    process.exitCode = 1;
  } else {
    console.log(`All ${manifest.routeCount} routes have at least one non-excluded covering spec.`);
  }
}

if (require.main === module) {
  main();
}
