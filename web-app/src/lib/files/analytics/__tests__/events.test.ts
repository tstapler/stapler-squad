import { readFileSync } from "fs";
import { resolve } from "path";
import {
  FILES_EVENTS, STAMP_KEYS, bucketBytes, bucketOps, bucketPayload, bucketQueryLen, bucketResults,
  extClass, searchOpts, slugOf, type FilesEventName,
} from "../events";
import { LABEL_VALUE_SHAPE, MAX_LABELS } from "../../__tests__/fixtures/privacyHarness";

const ADR = resolve(__dirname, "../../../../../../project_plans/files-viewer-revamp/decisions/ADR-035-files-analytics-taxonomy-and-privacy.md");
type Def = { category: string; labels: Record<string, readonly string[]> };
const registry = FILES_EVENTS as unknown as Record<string, Def>;

/** event name -> { category, label key -> literal value set (undefined when the ADR names only a bucket) } */
function parseAdr() {
  const rows = new Map<string, { category: string; labels: Map<string, string[] | undefined> }>();
  for (const line of readFileSync(ADR, "utf8").split("\n")) {
    const cells = line.split("|").map((c) => c.trim());
    if (cells.length < 5 || !cells[1].startsWith("`files.")) continue;
    const labels = new Map<string, string[] | undefined>();
    for (const m of cells[3].matchAll(/`([a-z_]+)`(?:\/`([a-z_]+)`)?(?:\s*\{([^}]*)\})?/g)) {
      const set = m[3] && !m[3].includes(" ") ? m[3].split(",") : undefined;
      labels.set(m[1], set);
      if (m[2]) labels.set(m[2], set);
    }
    for (const name of cells[1].match(/files\.[a-z0-9_.]+/g) ?? []) {
      rows.set(name, { category: cells[2], labels });
    }
  }
  return rows;
}

describe("events registry", () => {
  it("events_should_FitTheServerGuard_When_StampsAreCounted", () => {
    for (const [name, def] of Object.entries(registry)) {
      const stamps = name.startsWith("files.server.") ? 0 : STAMP_KEYS.length;
      expect(Object.keys(def.labels).length + stamps).toBeLessThanOrEqual(MAX_LABELS);
      for (const set of Object.values(def.labels)) {
        for (const v of set) expect(v).toMatch(LABEL_VALUE_SHAPE);
      }
    }
  });

  it("events_should_HaveUniqueShapeValidSlugs_ForEveryDroppableEvent", () => {
    const slugs = Object.keys(registry).filter((n) => !n.startsWith("files.server.")).map((n) => slugOf(n));
    expect(new Set(slugs).size).toBe(slugs.length);
    slugs.forEach((s) => expect(s).toMatch(LABEL_VALUE_SHAPE));
    expect(registry["files.meta.dropped"].labels.event).toEqual(slugs.filter((s) => s !== "meta_dropped"));
  });

  it("adr035Table_should_MatchEventsRegistry_When_Parsed", () => {
    const adr = parseAdr();
    expect([...adr.keys()].sort()).toEqual(Object.keys(registry).sort());
    for (const [name, row] of adr) {
      const def = registry[name];
      expect(row.category).toBe(def.category);
      expect([...row.labels.keys()].sort()).toEqual(Object.keys(def.labels).sort());
      for (const [key, set] of row.labels) {
        if (set) expect([name, key, def.labels[key]]).toEqual([name, key, set]);
      }
    }
  });

  it("transportExtension_should_DeclareTheClosedEnumsOfAdr036Section16", () => {
    const sets = (name: FilesEventName, key: string) => registry[name].labels[key];
    expect(sets("files.transport.select", "transport")).toEqual(["full", "connect_unary", "connect_stream", "ws_stream", "http_range"]);
    expect(sets("files.transport.fallback", "fallback_reason")).toEqual(["network", "unavailable", "deadline", "malformed", "capability", "version"]);
    expect(sets("files.read.window", "payload")).toEqual(["lt1k", "1k-10k", "10k-100k", "100k-1m", "ge1m"]);
    expect(sets("files.server.patch", "ops")).toEqual(["1", "2-9", "10-99", "100-999", "1000+"]);
    expect(sets("files.server.patch", "tier")).toEqual(["t0", "t1", "t2"]);
    expect(Object.keys(registry)).toEqual(expect.arrayContaining([
      "files.transport.select", "files.transport.fallback", "files.read.window",
      "files.edit.patch_rejected", "files.edit.preflight", "files.server.patch",
    ]));
  });
});

describe("bucket functions and classes", () => {
  it("buckets_should_ReturnMembersOfTheirDeclaredSets_When_GivenEdgeValues", () => {
    expect(bucketQueryLen(2)).toBe("2-3");
    expect(bucketQueryLen(33)).toBe("33+");
    expect(bucketResults(0)).toBe("0");
    expect(bucketResults(1000)).toBe("1000+");
    expect(bucketBytes(1_048_576)).toBe("gt1m");
    expect(bucketPayload(1_048_576)).toBe("ge1m");
    expect(bucketPayload(500)).toBe("lt1k");
    expect(bucketOps(1)).toBe("1");
    expect(bucketOps(10)).toBe("10-99");
    expect(registry["files.server.patch"].labels.ops).toContain(bucketOps(5000));
  });

  it("extClass_should_ReturnOnlyAClass_When_GivenAPath", () => {
    expect(extClass("secret/dir/Canary.go")).toBe("code");
    expect(extClass("a/b/README.md")).toBe("doc");
    expect(extClass("Makefile")).toBe("none");
    expect(extClass(".gitignore")).toBe("none");
    expect(extClass("weird.zzz")).toBe("other");
  });

  it("searchOpts_should_PackSixBooleansIntoAMemberOfTheSet", () => {
    const opts = searchOpts({ regex: true, caseSensitive: false, wholeWord: true, globs: false, ignored: false, large: true });
    expect(opts).toBe("r1c0w1g0i0l1");
    expect(registry["files.search.run"].labels.opts).toContain(opts);
    expect(registry["files.search.run"].labels.opts).toHaveLength(64);
  });
});
