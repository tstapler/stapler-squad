import * as fs from "fs";
import { countListed, failRows, grepInvertFor, summaryFor, BaselineRow } from "../scripts/baseline-skip";

const rows: BaselineRow[] = [
  { spec: "a.spec.ts", project: "chromium", title: "Suite > does (x) [y]", class: "fail" },
  { spec: "a.spec.ts", project: "chromium-dom", title: "plain.title", class: "fail" },
  { spec: "b.spec.ts", project: "chromium", title: "ok", class: "pass" },
  { spec: "b.spec.ts", project: "chromium", title: "wobbly", class: "flaky" },
];

// Playwright greps "<project> <file> <describes> <title>" (titlePath joined by spaces, leading root is empty).
const hit = (re: string, s: string) => new RegExp(re, "i").test(s);

describe("baseline-skip", () => {
  test("BS1: only class fail rows are skipped", () => {
    expect(failRows(rows).map((r) => r.title)).toEqual(["Suite > does (x) [y]", "plain.title"]);
  });

  test("BS2: regex matches the skipped tests' title paths and nothing else", () => {
    const re = grepInvertFor(rows);
    expect(hit(re, " chromium a.spec.ts Suite does (x) [y]")).toBe(true);
    expect(hit(re, " chromium-dom a.spec.ts plain.title")).toBe(true);
    expect(hit(re, " chromium a.spec.ts plain.title")).toBe(false); // other project keeps running
    expect(hit(re, " chromium b.spec.ts ok")).toBe(false);
    expect(hit(re, " chromium b.spec.ts wobbly")).toBe(false);
    expect(hit(re, " chromium a.spec.ts Suite does (x) [y] extra")).toBe(false);
  });

  test("BS3: an empty fail set matches no test", () => {
    expect(hit(grepInvertFor([rows[2]]), " chromium b.spec.ts ok")).toBe(false);
  });

  test("BS4: summary counts the skipped tests and names the files", () => {
    const s = summaryFor(rows);
    expect(s).toContain("skipped on this PR run: 2");
    expect(s).toContain("- a.spec.ts: 2");
  });

  test("BS5: a spec file in the keep set is not skipped (PR repairs a baseline-fail test)", () => {
    const keep = new Set(["a.spec.ts"]);
    expect(failRows(rows, keep)).toEqual([]);
    expect(hit(grepInvertFor(rows, keep), " chromium a.spec.ts plain.title")).toBe(false);
  });

  test("BS6: a real title containing ' > ' still matches", () => {
    const r: BaselineRow[] = [{ spec: "u.spec.ts", project: "chromium", title: "unfinished-work > page loads", class: "fail" }];
    expect(hit(grepInvertFor(r), " chromium u.spec.ts unfinished-work > page loads")).toBe(true);
    expect(hit(grepInvertFor(r), " chromium u.spec.ts unfinished-work page loads")).toBe(true);
  });

  test("BS7: summary warns when the baseline is stale and reports kept specs", () => {
    const meta = { capturedAt: "2026-10-29T00:00:00Z" };
    const s = summaryFor(rows, { meta, now: new Date("2026-11-16T00:00:00Z"), keep: new Set(["a.spec.ts"]), removed: 0 });
    expect(s).toContain("18 days ago");
    expect(s).toContain("WARNING");
    expect(s).toContain("Not skipped because this PR changes their spec file: 2 rows");
    expect(summaryFor(rows, { meta, now: new Date("2026-11-01T00:00:00Z") })).not.toContain("WARNING");
  });

  test("BS9: the regex for the committed baseline stays well under the 128 KB argv limit", () => {
    const real = JSON.parse(fs.readFileSync(`${__dirname}/next-e2e-baseline.json`, "utf8")).specs as BaselineRow[];
    expect(Buffer.byteLength(grepInvertFor(real))).toBeLessThan(90_000);
  });

  test("BS8: countListed counts tests across nested suites", () => {
    expect(countListed({ suites: [{ specs: [{ tests: [{}, {}] }], suites: [{ specs: [{ tests: [{}] }] }] }] })).toBe(3);
  });
});
