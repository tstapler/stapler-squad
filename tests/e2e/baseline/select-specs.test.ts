import * as fs from "fs";
import * as path from "path";
import { ALL_RULES, SAFETY_NET_SPECS, SUBSET_RULES, WEB_LEAF_GROUPS, expandGlobs, select, shardCountFor, summarize } from "../scripts/select-specs";

const specs = [
  "smoke.spec.ts", "route-smoke.spec.ts", "backlog.spec.ts", "backlog-flow.spec.ts", "insights.spec.ts",
  "escape-analytics-global-view.spec.ts", "files-tabs.spec.ts", "alias.spec.ts", "terminal-flickering.spec.ts", "terminal-stress/log-flood.spec.ts",
];

describe("select-specs", () => {
  test("SS1: an unknown file selects everything (fail-safe)", () => {
    const s = select(["session/some_new_thing.go"], specs);
    expect(s.mode).toBe("all");
    expect(s.reasons[0]).toContain("fail-safe");
  });

  test("SS2: one shared-surface file beats any number of subset files", () => {
    const s = select(["server/analytics/db.go", "web-app/src/lib/terminal/pool.ts", "tests/e2e/insights.spec.ts"], specs);
    expect(s.mode).toBe("all");
  });

  test("SS3: a leaf-area change selects its specs plus the safety net, nothing else", () => {
    const s = select(["web-app/src/app/analytics/escape/page.tsx"], specs);
    expect(s).toMatchObject({ mode: "subset", specs: ["escape-analytics-global-view.spec.ts", "insights.spec.ts", "route-smoke.spec.ts", "smoke.spec.ts"] });
  });

  test("SS4: only non-runtime files select none", () => {
    const s = select(["docs/how-to/x.md", "server/foo_test.go", "web-app/src/lib/x.test.ts", "web-app/build/__tests__/y.test.ts", ".github/workflows/lint.yml", "tests/e2e/shard-timings.json"], specs);
    expect(s.mode).toBe("none");
  });

  test("SS5: a Jest test under a shared directory is still only a test", () => {
    expect(select(["web-app/src/lib/terminal/__tests__/pool.test.ts"], specs).mode).toBe("none");
  });

  test("SS6: a changed spec selects itself; shared e2e helpers select all", () => {
    expect(select(["tests/e2e/alias.spec.ts"], specs)).toMatchObject({ mode: "subset", specs: expect.arrayContaining(["alias.spec.ts"]) });
    expect(select(["tests/e2e/pages/SessionsPage.ts"], specs).mode).toBe("all");
    expect(select(["tests/e2e/playwright.config.ts"], specs).mode).toBe("all");
  });

  test("SS7: the CI pipeline and build inputs select all", () => {
    for (const f of [".github/workflows/build.yml", ".github/actions/setup-bun/action.yml", "go.mod", "proto/session/v1/session.proto", "session/streamhub/hub.go", "web-app/bun.lock"]) {
      expect(select([f], specs).mode).toBe("all");
    }
  });

  test("SS8: every subset glob and the safety net match at least one real spec file", () => {
    const real = fs.readdirSync(path.join(__dirname, "..")).filter((f) => f.endsWith(".spec.ts"));
    for (const r of SUBSET_RULES) expect({ rule: r.name, hits: expandGlobs(r.specs, real).length > 0 }).toEqual({ rule: r.name, hits: true });
    expect(expandGlobs(SAFETY_NET_SPECS, real).sort()).toEqual([...SAFETY_NET_SPECS].sort());
  });

  test("SS9: no file can be both a shared surface and a subset leaf (the leaf would be dead)", () => {
    const samples = ["web-app/src/components/files/Tree.tsx", "web-app/src/app/analytics/escape/page.tsx", "web-app/src/app/notifications/page.tsx"];
    for (const f of samples) expect(ALL_RULES.some((r) => r.re.test(f))).toBe(false);
    for (const f of samples) expect(SUBSET_RULES.some((r) => r.re.test(f))).toBe(true);
  });

  test("SS12: nothing imports a subset leaf from outside its group (else the leaf is really a shared component)", () => {
    const src = path.join(__dirname, "../../../web-app/src");
    const files: string[] = [];
    const walk = (d: string) => {
      for (const e of fs.readdirSync(d, { withFileTypes: true })) {
        const full = path.join(d, e.name);
        if (e.isDirectory()) walk(full);
        else if (/\.tsx?$/.test(e.name) && !/\.test\.|\.stories\./.test(e.name) && !full.includes("__tests__")) files.push(full);
      }
    };
    walk(src);
    for (const group of WEB_LEAF_GROUPS) {
      const importers: string[] = [];
      for (const f of files) {
        const rel = path.relative(src, f);
        if (group.some((g) => rel.startsWith(g + "/"))) continue;
        const text = fs.readFileSync(f, "utf8");
        if (group.some((g) => new RegExp(`from ['"](@/${g}|(\\.\\./)+${g.split("/")[1]})(/|['"])`).test(text))) importers.push(rel);
      }
      expect({ group, importers }).toEqual({ group, importers: [] });
    }
  });

  test("SS13: an empty change list is fail-safe, CHANGELOG.md is a build input, and dropped shared components select all", () => {
    expect(select(["server/analytics/db.go"], specs).mode).toBe("all");
    expect(select([""], specs).mode).toBe("all");
    expect(select(["CHANGELOG.md"], specs).mode).toBe("all");
    expect(select(["web-app/src/components/analytics/PageViewTracker.tsx"], specs).mode).toBe("all");
    expect(select(["web-app/src/components/backlog/BacklogStreamContext.tsx"], specs).mode).toBe("all");
  });

  test("SS10: shard count follows recorded seconds, never exceeds files or the cap", () => {
    expect(shardCountFor(["a.spec.ts"], { "a.spec.ts": 10 }, 5)).toBe(1);
    expect(shardCountFor(["a", "b", "c", "d", "e", "f"], Object.fromEntries(["a", "b", "c", "d", "e", "f"].map((k) => [k, 3000])), 5)).toBe(5);
    expect(shardCountFor(["a", "b"], { a: 9000, b: 9000 }, 5)).toBe(2);
  });

  test("SS11: the summary names the decision and every reason", () => {
    const md = summarize(select(["web-app/src/app/analytics/escape/page.tsx"], specs), specs.length);
    expect(md).toContain("SUBSET");
    expect(md).toContain("web-app/src/app/analytics/escape/page.tsx: analytics");
  });
});
