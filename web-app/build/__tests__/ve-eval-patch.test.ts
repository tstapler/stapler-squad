import { copyFileSync, mkdirSync, mkdtempSync, symlinkSync } from "node:fs";
import { createRequire } from "node:module";
import os from "node:os";
import path from "node:path";
import { runInBun } from "./helpers/run-in-bun";

// Pristine eval.js next to the real require-like, so the probe fails on Bun's
// Module._load behaviour and not merely on a missing dependency.
function unpatchedEvalDir(): string {
  const dir = mkdtempSync(path.join(os.tmpdir(), "eval-unpatched-"));
  copyFileSync(path.resolve(__dirname, "fixtures/eval-unpatched/eval.js"), path.join(dir, "eval.js"));
  const fromIntegration = createRequire(require.resolve("@vanilla-extract/integration/package.json"));
  const evalPkg = fromIntegration.resolve("eval/package.json");
  const requireLike = path.dirname(createRequire(evalPkg).resolve("require-like/package.json"));
  mkdirSync(path.join(dir, "node_modules"));
  symlinkSync(requireLike, path.join(dir, "node_modules/require-like"));
  return dir;
}

describe("vanilla-extract eval patch (ADR-008)", () => {
  it("fails the probe on an unpatched eval (asserts status, not error text)", () => {
    const { status, json } = runInBun("ve-eval-patch.bun.ts", ["--eval-dir", unpatchedEvalDir()]);
    expect(status).not.toBe(0);
    expect((json as { ok: boolean }).ok).toBe(false);
  });

  it("passes the probe on the installed, patched eval", () => {
    const { status, json } = runInBun("ve-eval-patch.bun.ts");
    expect(status).toBe(0);
    expect((json as { ok: boolean }).ok).toBe(true);
  });

  it("compiles a .css.ts with a style and a themed custom property under Bun", () => {
    const { status, json } = runInBun("ve-canary.bun.ts");
    const res = json as { ok: boolean; source: string; css: string };
    expect(status).toBe(0);
    expect(res.ok).toBe(true);
    expect(res.source).toMatch(/canary/);
    expect(res.css).toMatch(/--canary-color.*#123456/);
  });
});
