import path from "node:path";
import { runInBun } from "../../__tests__/helpers/run-in-bun";

type Result = { ok: true; text: string } | { ok: false; message: string };
function bundle(entry: string): Result {
  const r = runInBun(path.resolve(__dirname, "bun", "build-seam.bun.ts"), [entry]);
  expect(r.status).toBe(0);
  return r.json as Result;
}

describe("build-seam plugin (under Bun)", () => {
  it("aliases the nav and lazy seams to the router and bun implementations, and resolves .js imports of .ts sources", () => {
    const r = bundle("entry-ok.ts");
    expect(r).toMatchObject({ ok: true });
    const text = (r as { text: string }).text;
    expect(text).toContain("ROUTER_IMPL_MARKER");
    expect(text).toContain("BUN_LAZY_MARKER");
    expect(text).toContain("GEN_X_MARKER");
  });

  it("fails the build naming the importer when next/navigation is imported", () => {
    const r = bundle("entry-ban.ts");
    expect(r).toMatchObject({ ok: false });
    expect((r as { message: string }).message).toContain("forbidden import of next/navigation");
    expect((r as { message: string }).message).toContain("entry-ban.ts");
  });

  it("also refuses next when a dependency module imports it", () => {
    const r = bundle("entry-ban-dep.ts");
    expect(r).toMatchObject({ ok: false });
    expect((r as { message: string }).message).toContain("vendor-dep.js");
  });
});
