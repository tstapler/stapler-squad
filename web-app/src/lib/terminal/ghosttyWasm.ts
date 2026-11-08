import type { Ghostty } from "ghostty-web";

/**
 * Loads our own ghostty-vt.wasm (scripts/build-ghostty-wasm.sh) when the build produced one,
 * and returns null otherwise so the caller keeps ghostty-web's embedded stock wasm.
 *
 * The build writes `ghostty-vt.json` plus a content-addressed `ghostty-vt.<sha12>.wasm` into
 * web-app/public. The manifest is fetched revalidating (`no-cache`) and names the wasm file,
 * so a new build busts the cache without a bundler define. Both names are public-allowlisted.
 */
const MANIFEST_URL = "/ghostty-vt.json";
const WASM_FILE = /^ghostty-vt\.[0-9a-f]{12}\.wasm$/;

export interface GhosttyWasmManifest {
  file: string;
  sha256: string;
  rev: string;
  patches: string;
  zig: string;
}

type FetchLike = (url: string, init?: RequestInit) => Promise<Pick<Response, "ok" | "json">>;

/** True only for a manifest whose `file` is a bare content-addressed wasm name (no path tricks). */
export function isGhosttyWasmManifest(value: unknown): value is GhosttyWasmManifest {
  if (typeof value !== "object" || value === null) return false;
  const m = value as Record<string, unknown>;
  return (
    typeof m.file === "string" &&
    WASM_FILE.test(m.file) &&
    typeof m.sha256 === "string" &&
    m.sha256.startsWith(m.file.slice("ghostty-vt.".length, -".wasm".length))
  );
}

/** Resolves the served wasm URL, or null when no own-build wasm was shipped (404, SPA fallback HTML, bad manifest). */
export async function resolveOwnGhosttyWasmUrl(fetchImpl: FetchLike = fetch): Promise<{ url: string; manifest: GhosttyWasmManifest } | null> {
  try {
    const res = await fetchImpl(MANIFEST_URL, { cache: "no-cache" });
    if (!res.ok) return null;
    const manifest: unknown = await res.json(); // the SPA fallback serves HTML for unknown paths; json() rejects
    return isGhosttyWasmManifest(manifest) ? { url: `/${manifest.file}`, manifest } : null;
  } catch {
    return null;
  }
}

/** Loads our wasm through ghostty-web's `Ghostty.load`; null means "use the stock wasm". */
export async function loadOwnGhostty(
  GhosttyClass: Pick<typeof Ghostty, "load">,
  fetchImpl: FetchLike = fetch,
): Promise<Ghostty | null> {
  const resolved = await resolveOwnGhosttyWasmUrl(fetchImpl);
  if (!resolved) return null;
  try {
    return await GhosttyClass.load(resolved.url);
  } catch (err) {
    console.warn("[ghostty] own wasm failed to load; falling back to the stock wasm", err);
    return null;
  }
}
