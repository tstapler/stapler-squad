import { readFileSync, rmSync } from "node:fs";
import path from "node:path";

/**
 * Bun emits one CSS file per lazy page chunk, but nothing loads them: index.html links only the entry CSS,
 * which already holds every rule (no JS names the others). Deletes each emitted .css whose file name no
 * emitted .html/.js mentions, and returns the deleted paths.
 */
export function pruneUnreferencedCss(emitted: { path: string }[]): string[] {
  const referrers = emitted.filter((o) => /\.(?:html|js)$/.test(o.path)).map((o) => readFileSync(o.path, "utf8"));
  const pruned: string[] = [];
  for (const { path: file } of emitted) {
    if (!file.endsWith(".css")) continue;
    const name = path.basename(file);
    if (referrers.some((text) => text.includes(name))) continue;
    rmSync(file, { force: true });
    pruned.push(file);
  }
  return pruned;
}
