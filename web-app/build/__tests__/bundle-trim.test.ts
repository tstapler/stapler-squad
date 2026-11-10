import { createHash } from "node:crypto";
import { mkdtempSync, existsSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { chooseGhosttyWasm, externalizeGhosttyWasm, readWasmPin, type WasmPin } from "../plugins/ghostty-wasm";
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

describe("chooseGhosttyWasm", () => {
  const stock = "/stock/ghostty-vt.wasm";
  const sha = (text: string) => createHash("sha256").update(text).digest("hex");
  const ownFile = (text: string) => {
    const own = path.join(mkdtempSync(path.join(os.tmpdir(), "own-wasm-")), "ghostty-vt.wasm");
    writeFileSync(own, text);
    return own;
  };
  const pin = (sha256: string | null, extra: Partial<WasmPin> = {}): WasmPin => ({ sha256, require: false, allowUnpinned: false, ...extra });

  it("falls back to the stock wasm when ours is absent", () => {
    expect(chooseGhosttyWasm(stock, "/nonexistent/ghostty-vt.wasm", pin(sha("x")))).toBe(stock);
  });

  it("ships ours only when its sha256 equals the pinned one", () => {
    const own = ownFile("x");
    expect(chooseGhosttyWasm(stock, own, pin(sha("x")))).toBe(own);
  });

  it("falls back loudly on a sha256 mismatch or an unrecorded pin", () => {
    const warn = jest.spyOn(console, "warn").mockImplementation(() => {});
    const own = ownFile("x");
    expect(chooseGhosttyWasm(stock, own, pin(sha("other")))).toBe(stock);
    expect(chooseGhosttyWasm(stock, own, pin(null))).toBe(stock);
    expect(warn).toHaveBeenCalledTimes(2);
    warn.mockRestore();
  });

  it("trusts an unrecorded pin only when explicitly allowed", () => {
    const own = ownFile("x");
    expect(chooseGhosttyWasm(stock, own, pin(null, { allowUnpinned: true }))).toBe(own);
  });

  it("throws instead of falling back when own wasm is required", () => {
    expect(() => chooseGhosttyWasm(stock, ownFile("x"), pin(sha("other"), { require: true }))).toThrow(/NOT using our own wasm/);
    expect(() => chooseGhosttyWasm(stock, "/nonexistent/ghostty-vt.wasm", pin(sha("x"), { require: true }))).toThrow(/does not exist/);
  });
});

describe("readWasmPin", () => {
  it("finds the recorded sha256 for the repo's current pin", () => {
    const { sha256 } = readWasmPin(path.resolve(__dirname, "..", "..", ".."), {});
    expect(sha256).toMatch(/^[0-9a-f]{64}$/);
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
