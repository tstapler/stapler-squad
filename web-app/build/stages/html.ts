import { readFileSync, renameSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import { PRELOAD_FONTS } from "../config";
import { injectFontPreload, injectThemeBoot } from "../html";
import type { BuildContext } from "./context";
import type { Stage } from "./runner";

/** Layout A': Bun writes the HTML entry under _next/static/chunks/; the shell must live at the root for SPA fallback. */
export const html: Stage<BuildContext> = {
  name: "html",
  run(ctx) {
    const emitted = ctx.emitted.filter((o) => o.path.endsWith(".html"));
    if (emitted.length !== 1) throw new Error(`expected exactly one emitted .html, got ${emitted.length}: ${emitted.map((o) => o.path).join(", ")}`);
    if (!ctx.themeMap) throw new Error("theme-map stage did not run");
    const from = emitted[0].path;
    const to = path.join(ctx.tmpDir, "index.html");
    const preload = PRELOAD_FONTS.flatMap((f) => ctx.fontFiles.get(f) ?? []);
    writeFileSync(from, injectFontPreload(injectThemeBoot(readFileSync(from, "utf8"), ctx.themeMap), preload));
    rmSync(to, { force: true });
    renameSync(from, to);
  },
};
