/**
 * Ratchet: no all-property transitions in vanilla-extract styles. `transition: all` makes every
 * style invalidation (theme class swap, container resize on reload) start a transition on every
 * matching element, each emitting transitionrun/start/cancel events that bubble through React's
 * root listener. List the properties that actually change instead.
 */
import * as fs from "fs";
import * as path from "path";

const SRC = path.resolve(__dirname, "../..");

// Files (relative to src/) allowed to keep an all-property transition. Only ever shrink this.
const ALLOWLIST = new Set<string>([]);

const ALL_TRANSITION = /transition(?:Property)?\s*:\s*["'`]\s*all\b/;

function cssTsFiles(dir: string): string[] {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) return entry.name === "node_modules" ? [] : cssTsFiles(full);
    return entry.name.endsWith(".css.ts") ? [full] : [];
  });
}

describe("no all-property transitions", () => {
  it("has no new `transition: all` in *.css.ts", () => {
    const offenders = cssTsFiles(SRC)
      .map((file) => path.relative(SRC, file))
      .filter((rel) => !ALLOWLIST.has(rel))
      .filter((rel) =>
        fs
          .readFileSync(path.join(SRC, rel), "utf-8")
          .split("\n")
          .some((line) => ALL_TRANSITION.test(line)),
      );
    expect(offenders).toEqual([]);
  });

  it("matches the shapes it is meant to catch", () => {
    expect(ALL_TRANSITION.test('  transition: "all 0.2s ease",')).toBe(true);
    expect(ALL_TRANSITION.test('  transitionProperty: "all",')).toBe(true);
    expect(ALL_TRANSITION.test('  transition: "background-color 0.2s ease",')).toBe(false);
  });
});
