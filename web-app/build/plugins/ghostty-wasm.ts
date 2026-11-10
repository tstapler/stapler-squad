import { createHash } from "node:crypto";
import { existsSync, readFileSync } from "node:fs";
import path from "node:path";
import type { BunPlugin } from "bun";

const GHOSTTY_ENTRY = /[\\/]ghostty-web[\\/]dist[\\/]ghostty-web\.js$/;
/** ghostty-web 0.4.0 inlines ghostty-vt.wasm as a base64 data: URL (~564 KB of the lazy chunk). */
const EMBEDDED_WASM = /new URL\("data:application\/wasm;base64,[A-Za-z0-9+/=]+", self\.location\)/;

/** Rewrites the inlined WASM into a hashed file under _next/static/media so it ships as a binary instead of base64. */
export function externalizeGhosttyWasm(source: string, wasmPath: string): string {
  if (!EMBEDDED_WASM.test(source)) {
    throw new Error("ghostty-wasm plugin: the embedded WASM data URL was not found in ghostty-web; update build/plugins/ghostty-wasm.ts for the new release");
  }
  const url = `new URL(__ghosttyVtWasmUrl, self.location)`;
  return `import __ghosttyVtWasmUrl from ${JSON.stringify(wasmPath)}; ${source.replace(EMBEDDED_WASM, url)}`;
}

/** Where scripts/build-ghostty-wasm.sh installs our wasm (gitignored; under build/ so the Makefile tracks it). */
export const OWN_WASM_PATH = path.resolve(__dirname, "..", "ghostty-own", "ghostty-vt.wasm");

const REPO_ROOT = path.resolve(__dirname, "..", "..", "..");

/** What the own wasm must hash to, and how strictly the build treats a wasm that does not. */
export interface WasmPin {
  /** Recorded sha256 for the pin in MODULE.bazel; null when scripts/ghostty-wasm.sha256 has no line for it. */
  sha256: string | null;
  /** GHOSTTY_WASM_REQUIRE_OWN=1 (release/CI): fail the build instead of falling back to the stock wasm. */
  require: boolean;
  /** GHOSTTY_WASM_ALLOW_UNPINNED=1: trust an own wasm with no recorded sha256 (testing a new pin locally). */
  allowUnpinned: boolean;
}

/** Reads the recorded sha256 for the current pin ("<rev12>-zig<version>", same key as scripts/build-ghostty-wasm.sh). */
export function readWasmPin(root: string = REPO_ROOT, env: NodeJS.ProcessEnv = process.env): WasmPin {
  const module = readFileSync(path.join(root, "MODULE.bazel"), "utf8");
  const rev = /^GHOSTTY_REV = "([0-9a-f]{40})"$/m.exec(module)?.[1];
  const zig = /^ZIG_VERSION = "([^"]+)"$/m.exec(module)?.[1];
  let sha256: string | null = null;
  if (rev && zig) {
    const key = `${rev.slice(0, 12)}-zig${zig}`;
    const record = readFileSync(path.join(root, "scripts", "ghostty-wasm.sha256"), "utf8");
    sha256 = record.split("\n").map((l) => l.trim().split(/\s+/)).find((f) => f[0] === key)?.[1] ?? null;
  }
  return { sha256, require: env.GHOSTTY_WASM_REQUIRE_OWN === "1", allowUnpinned: env.GHOSTTY_WASM_ALLOW_UNPINNED === "1" };
}

function rejectOwnWasm(reason: string, pin: WasmPin, stockWasm: string): string {
  const msg = `ghostty-wasm plugin: NOT using our own wasm: ${reason}`;
  if (pin.require) throw new Error(`${msg} (GHOSTTY_WASM_REQUIRE_OWN=1)`);
  console.warn(`\n!!! ${msg}\n!!! Shipping ghostty-web's STOCK wasm (no DECSTR / page-zeroing fixes). Run scripts/fetch-ghostty-wasm.sh.\n`);
  return stockWasm;
}

/**
 * Our build when present and equal to the recorded sha256, else ghostty-web's stock wasm; either way it ships
 * as one hashed asset. Fail-closed: a wasm that does not match the pin (stale install after a pin bump, a
 * corrupt or hand-placed file) is never shipped as ours.
 */
export function chooseGhosttyWasm(stockWasm: string, ownWasm: string = OWN_WASM_PATH, pin: WasmPin = readWasmPin()): string {
  if (!existsSync(ownWasm)) {
    if (pin.require) return rejectOwnWasm(`${ownWasm} does not exist`, pin, stockWasm);
    console.warn(`ghostty-wasm plugin: ${ownWasm} not found; shipping ghostty-web's STOCK wasm (run scripts/fetch-ghostty-wasm.sh)`);
    return stockWasm;
  }
  const actual = createHash("sha256").update(readFileSync(ownWasm)).digest("hex");
  if (pin.sha256 === null && !pin.allowUnpinned) return rejectOwnWasm("scripts/ghostty-wasm.sha256 has no recorded sha256 for the pin", pin, stockWasm);
  if (pin.sha256 !== null && actual !== pin.sha256) return rejectOwnWasm(`sha256 ${actual} != pinned ${pin.sha256}`, pin, stockWasm);
  console.info(`ghostty-wasm plugin: shipping own build wasm (${ownWasm}, sha256 ${actual})`);
  return ownWasm;
}

export function ghosttyWasmPlugin(): BunPlugin {
  return {
    name: "ghostty-wasm",
    setup: (b) => {
      b.onLoad({ filter: GHOSTTY_ENTRY }, ({ path: p }) => ({
        contents: externalizeGhosttyWasm(readFileSync(p, "utf8"), chooseGhosttyWasm(path.join(path.dirname(p), "ghostty-vt.wasm"))),
        loader: "js",
      }));
    },
  };
}
