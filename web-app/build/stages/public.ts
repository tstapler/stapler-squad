import { cpSync, existsSync, readdirSync } from "node:fs";
import path from "node:path";
import type { BuildContext } from "./context";
import type { Stage } from "./runner";

/** Copies public/ to the output root with unhashed names (service workers and the manifest need stable URLs). */
export const publicFiles: Stage<BuildContext> = {
  name: "public",
  run(ctx) {
    if (!existsSync(ctx.publicDir)) return;
    for (const name of readdirSync(ctx.publicDir)) {
      if (existsSync(path.join(ctx.tmpDir, name))) throw new Error(`public/${name} collides with a built output of the same name`);
    }
    cpSync(ctx.publicDir, ctx.tmpDir, { recursive: true });
  },
};
