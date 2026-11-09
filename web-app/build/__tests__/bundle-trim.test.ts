import { mkdtempSync, existsSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { externalizeGhosttyWasm } from "../plugins/ghostty-wasm";
import { pruneUnreferencedCss } from "../stages/prune-css";

describe("externalizeGhosttyWasm", () => {
  const inlined = 'const B = new URL("data:application/wasm;base64,AGFzbQEAAAA=", self.location);';

  it("swaps the base64 data URL for an imported asset URL", () => {
    const out = externalizeGhosttyWasm(inlined, "/x/ghostty-vt.wasm");
    expect(out).not.toContain("base64");
    expect(out).toContain('import __ghosttyVtWasmUrl from "/x/ghostty-vt.wasm";');
    expect(out).toContain("new URL(__ghosttyVtWasmUrl, self.location)");
  });

  it("fails loudly when a ghostty-web release no longer inlines the WASM", () => {
    expect(() => externalizeGhosttyWasm("const B = 1;", "/x/ghostty-vt.wasm")).toThrow(/ghostty-wasm plugin/);
  });
});

describe("pruneUnreferencedCss", () => {
  it("deletes only CSS files that no emitted html or js names", () => {
    const dir = mkdtempSync(path.join(os.tmpdir(), "prune-css-"));
    const f = (name: string, text = "x") => {
      const p = path.join(dir, name);
      writeFileSync(p, text);
      return { path: p };
    };
    const emitted = [f("index.html", '<link href="/c/index-1.css">'), f("a.js", 'load("lazy-2.css")'), f("index-1.css"), f("lazy-2.css"), f("page-3.css")];
    const pruned = pruneUnreferencedCss(emitted);
    expect(pruned.map((p) => path.basename(p))).toEqual(["page-3.css"]);
    expect(existsSync(emitted[2].path) && existsSync(emitted[3].path)).toBe(true);
    expect(existsSync(emitted[4].path)).toBe(false);
  });
});
