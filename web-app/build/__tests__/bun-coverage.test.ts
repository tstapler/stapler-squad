import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { findBunCoverageViolations, SourceFile } from "./helpers/bun-coverage";

const BUILD_DIR = path.resolve(__dirname, "../..");
// Keep this file out of its own scan: its fixtures deliberately mention bare script names.
const SELF = path.basename(__filename);

function walk(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    const p = path.join(dir, e.name);
    return e.isDirectory() ? (e.name === "node_modules" || e.name === "fixtures" ? [] : walk(p)) : [p];
  });
}

function load(files: string[]): SourceFile[] {
  return files.map((p) => ({ path: path.relative(BUILD_DIR, p), content: readFileSync(p, "utf8") }));
}

describe("every *.bun.ts is driven through runInBun", () => {
  const all = walk(BUILD_DIR);
  const scripts = load(all.filter((p) => p.endsWith(".bun.ts") && p.includes(`${path.sep}__tests__${path.sep}`)));
  const tests = load(all.filter((p) => p.endsWith(".test.ts") && path.basename(p) !== SELF));

  it("holds for the real tree", () => {
    expect(findBunCoverageViolations(scripts, tests)).toEqual([]);
  });

  it("flags an orphan script", () => {
    const v = findBunCoverageViolations([{ path: "x/__tests__/bun/orphan.bun.ts", content: "" }], []);
    expect(v).toEqual([expect.stringContaining("orphan.bun.ts")]);
  });

  it("flags a test that mentions a script without importing run-in-bun", () => {
    const sneaky = { path: "x/__tests__/sneaky.test.ts", content: 'spawn("bun", ["sneaky.bun.ts"])' };
    const v = findBunCoverageViolations([{ path: "x/__tests__/bun/sneaky.bun.ts", content: "" }], [sneaky]);
    expect(v).toHaveLength(2);
  });

  it("accepts a script driven through runInBun", () => {
    const good = { path: "x/__tests__/good.test.ts", content: 'import { runInBun } from "./helpers/run-in-bun";\nrunInBun("good.bun.ts");' };
    expect(findBunCoverageViolations([{ path: "x/__tests__/bun/good.bun.ts", content: "" }], [good])).toEqual([]);
  });
});
