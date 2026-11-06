import { mkdirSync, rmSync } from "node:fs";
import type { BuildContext } from "./context";
import type { Stage } from "./runner";

/** Builds go to a PID-scoped temp dir, so concurrent builds never share state and `out/` stays intact until swap. */
export const clean: Stage<BuildContext> = {
  name: "clean",
  run(ctx) {
    rmSync(ctx.tmpDir, { recursive: true, force: true });
    mkdirSync(ctx.tmpDir, { recursive: true });
  },
};
