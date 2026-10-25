import path from "node:path";
import { runInBun } from "../../__tests__/helpers/run-in-bun";

function run<T>(scenario: string): T {
  const r = runInBun(path.resolve(__dirname, "bun", "vanilla-extract.bun.ts"), [scenario]);
  expect(r.status).toBe(0);
  return r.json as T;
}

describe("VeCompiler (under Bun)", () => {
  it("returns the class map and virtual CSS for style, createTheme, globalStyle and keyframes, in both ident modes", () => {
    const out = run<Record<"short" | "debug", { exports: Record<string, string>; css: string }>>("output");
    for (const mode of ["short", "debug"] as const) {
      expect(Object.keys(out[mode].exports).sort()).toEqual(["box", "cleanTheme", "cyberpunk77Theme", "darkTheme", "lightTheme", "matrixTheme", "wh40kTheme"]);
      expect(out[mode].css).toContain("@keyframes");
      expect(out[mode].css).toContain("body {");
      expect(out[mode].css).toContain(`.${out[mode].exports.cleanTheme} {`);
    }
    expect(out.debug.exports.box).toMatch(/^theme_box__/);
    expect(out.short.exports.box).not.toMatch(/theme_/);
  });

  it("re-compiles only when the content hash of the file or its dependency closure changes", () => {
    const { steps } = run<{ steps: { name: string; compiles: number; css: string }[] }>("invalidate");
    const by = Object.fromEntries(steps.map((s) => [s.name, s]));
    expect(by.first.compiles).toBe(1);
    expect(by.unchanged.compiles).toBe(1);
    expect(by.touched.compiles).toBe(1);
    expect(by["edited-dependency"].compiles).toBe(2);
    expect(by.first.css).toContain("#111111");
    expect(by["edited-dependency"].css).toContain("#222222");
  });

  it("evicts least-recently-used entries past the bound", () => {
    expect(run("lru")).toEqual({ cacheSize: 2, hitLatest: true, evictedOldestRecompiled: true });
  });

  it("throws an error naming the file and caches nothing for it", () => {
    const r = run<{ err: string; cacheSize: number; bad: string }>("throws");
    expect(r.err).toContain(r.bad);
    expect(r.err).toContain("kaboom");
    expect(r.cacheSize).toBe(0);
  });

  it("loads the Story 3.4 canary through the plugin with non-empty class names and CSS", () => {
    const r = run<{ exports: Record<string, string>; css: string }>("canary");
    expect(r.exports.themeClass).toBeTruthy();
    expect(r.exports.box).toBeTruthy();
    expect(r.css).toContain("#123456");
  });
});
