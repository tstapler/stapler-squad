import { createHash } from "node:crypto";
import { copyFileSync, existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { FONT_URL_PREFIX } from "../config";
import { walkFiles } from "../stats";

const HASH_LEN = 8;
const REWRITE_EXT = /\.(?:css|html|js)$/;

/**
 * Bun inlines woff2 as data: URIs (194 KB of base64 in CSS), so fonts.css points at /_next/static/media/<name>
 * and stays external; this copies the files there. The names get a content hash because Go serves everything
 * under /_next/ as immutable for a year, and an unhashed font would never refresh.
 */
export function copyHashedFonts(fontsDir: string, tmpDir: string): Map<string, string> {
  const mapping = new Map<string, string>();
  if (!existsSync(fontsDir)) return mapping;
  const mediaDir = path.join(tmpDir, FONT_URL_PREFIX);
  mkdirSync(mediaDir, { recursive: true });
  for (const file of walkFiles(fontsDir).filter((f) => f.endsWith(".woff2"))) {
    const base = path.basename(file, ".woff2");
    if (mapping.has(`${base}.woff2`)) throw new Error(`two fonts named ${base}.woff2 under ${fontsDir}; names must be unique`);
    const bytes = readFileSync(file);
    const hashed = `${base}.${createHash("sha256").update(bytes).digest("hex").slice(0, HASH_LEN)}.woff2`;
    copyFileSync(file, path.join(mediaDir, hashed));
    mapping.set(`${base}.woff2`, hashed);
  }
  return mapping;
}

/** Rewrites `/_next/static/media/<name>.woff2` to the hashed name in every emitted CSS, HTML and JS file. */
export function rewriteFontUrls(tmpDir: string, mapping: Map<string, string>): void {
  if (mapping.size === 0) return;
  for (const file of walkFiles(tmpDir).filter((f) => REWRITE_EXT.test(f))) {
    const text = readFileSync(file, "utf8");
    let next = text;
    for (const [plain, hashed] of mapping) next = next.split(`${FONT_URL_PREFIX}${plain}`).join(`${FONT_URL_PREFIX}${hashed}`);
    if (next !== text) writeFileSync(file, next);
  }
}
