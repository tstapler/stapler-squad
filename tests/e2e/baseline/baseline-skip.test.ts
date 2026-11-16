import { failRows, grepInvertFor, summaryFor, BaselineRow } from "../scripts/baseline-skip";

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
});
