import { spawnSync } from "node:child_process";
import path from "node:path";

const REPO_ROOT = path.resolve(__dirname, "../../..");

// `git check-ignore -q` exits 0 when the path IS ignored, 1 when it is not.
function isIgnored(p: string): boolean {
  const res = spawnSync("git", ["check-ignore", "-q", p], { cwd: REPO_ROOT });
  if (res.status !== 0 && res.status !== 1) {
    throw new Error(`git check-ignore failed: ${res.stderr}`);
  }
  return res.status === 0;
}

describe(".gitignore keeps web-app/build/ tracked", () => {
  it.each([
    "web-app/build/build.ts",
    "web-app/build/__tests__/runner-smoke.test.ts",
    "web-app/build/__tests__/bun/runtime.bun.ts",
    "web-app/build/plugins/p.ts",
  ])("does not ignore %s", (p) => {
    expect(isIgnored(p)).toBe(false);
  });

  it("still ignores other build/ directories", () => {
    expect(isIgnored("other/build/z")).toBe(true);
  });
});
