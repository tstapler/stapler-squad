import { runInBun } from "./helpers/run-in-bun";

describe("build test project", () => {
  it("runs Bun-side scripts under Bun, not Node", () => {
    const { status, json } = runInBun("runtime.bun.ts");
    expect(status).toBe(0);
    expect(json).toEqual({ runtime: true });
  });

  it("fails, rather than skips, when bun is not on PATH", () => {
    const savedPath = process.env.PATH;
    process.env.PATH = "";
    try {
      expect(() => runInBun("runtime.bun.ts")).toThrow(/bun not found.*make ensure-tools/);
    } finally {
      process.env.PATH = savedPath;
    }
  });

  it("itself runs under Node", () => {
    expect(typeof (globalThis as { Bun?: unknown }).Bun).toBe("undefined");
  });
});
