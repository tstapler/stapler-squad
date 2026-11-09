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

/** Where scripts/build-ghostty-wasm.sh installs our wasm (gitignored; under build/ so the Makefile tracks it). Relative to web-app/, the build's cwd. */
export const OWN_WASM_PATH = path.resolve("build", "ghostty-own", "ghostty-vt.wasm");

/** Our build when present, else ghostty-web's stock wasm; either way it ships as one hashed asset. */
export function chooseGhosttyWasm(stockWasm: string, ownWasm: string = OWN_WASM_PATH): string {
  return existsSync(ownWasm) ? ownWasm : stockWasm;
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
