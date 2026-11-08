/**
 * @jest-environment node
 */
import { isGhosttyWasmManifest, loadOwnGhostty, resolveOwnGhosttyWasmUrl } from "../ghosttyWasm";

const SHA = "0123456789abcdef".repeat(4);
const manifest = { file: `ghostty-vt.${SHA.slice(0, 12)}.wasm`, sha256: SHA, rev: "r", patches: "p", zig: "0.15.2" };

const respondWith = (ok: boolean, body: () => Promise<unknown>) => async () => ({ ok, json: body });

describe("isGhosttyWasmManifest", () => {
  it("accepts a well-formed manifest", () => {
    expect(isGhosttyWasmManifest(manifest)).toBe(true);
  });

  it.each([
    ["path traversal", { ...manifest, file: "../ghostty-vt.0123456789ab.wasm" }],
    ["absolute URL", { ...manifest, file: "https://evil.example/ghostty-vt.0123456789ab.wasm" }],
    ["sha not matching file name", { ...manifest, sha256: "f".repeat(64) }],
    ["non-object", "ghostty-vt.json"],
    ["null", null],
  ])("rejects %s", (_name, value) => {
    expect(isGhosttyWasmManifest(value)).toBe(false);
  });
});

describe("resolveOwnGhosttyWasmUrl", () => {
  it("returns the content-addressed URL for a valid manifest", async () => {
    const got = await resolveOwnGhosttyWasmUrl(respondWith(true, async () => manifest));
    expect(got?.url).toBe(`/${manifest.file}`);
  });

  it("returns null on 404", async () => {
    expect(await resolveOwnGhosttyWasmUrl(respondWith(false, async () => manifest))).toBeNull();
  });

  it("returns null when the SPA fallback serves HTML (json() rejects)", async () => {
    const html = respondWith(true, async () => {
      throw new SyntaxError("Unexpected token <");
    });
    expect(await resolveOwnGhosttyWasmUrl(html)).toBeNull();
  });

  it("returns null when the network fails", async () => {
    expect(
      await resolveOwnGhosttyWasmUrl(async () => {
        throw new TypeError("offline");
      }),
    ).toBeNull();
  });
});

describe("loadOwnGhostty", () => {
  const manifestFetch = respondWith(true, async () => manifest);

  it("loads our wasm via Ghostty.load", async () => {
    const instance = { id: "own" };
    const load = jest.fn().mockResolvedValue(instance);
    const got = await loadOwnGhostty({ load } as never, manifestFetch);
    expect(load).toHaveBeenCalledWith(`/${manifest.file}`);
    expect(got).toBe(instance);
  });

  it("does not call Ghostty.load when no wasm was built", async () => {
    const load = jest.fn();
    expect(await loadOwnGhostty({ load } as never, respondWith(false, async () => ({})))).toBeNull();
    expect(load).not.toHaveBeenCalled();
  });

  it("falls back (null) when the wasm fails to instantiate", async () => {
    const warn = jest.spyOn(console, "warn").mockImplementation(() => undefined);
    const load = jest.fn().mockRejectedValue(new Error("CompileError"));
    expect(await loadOwnGhostty({ load } as never, manifestFetch)).toBeNull();
    warn.mockRestore();
  });
});
