import { existsSync, renameSync, rmSync } from "node:fs";
import type { BuildContext } from "./context";
import type { Stage } from "./runner";

const ATTEMPTS = 3;

/**
 * Two builds finishing together can both find `out/` missing; the loser moves the winner's output aside and retries.
 * If every attempt fails the first previous output is put back, so a failed swap never leaves `out/` empty.
 */
export const swap: Stage<BuildContext> = {
  name: "swap",
  run(ctx) {
    const aside: string[] = [];
    for (let attempt = 1; ; attempt++) {
      if (existsSync(ctx.outDir)) {
        const old = `${ctx.tmpDir}.old${attempt}`;
        rmSync(old, { recursive: true, force: true });
        renameSync(ctx.outDir, old);
        aside.push(old);
      }
      try {
        renameSync(ctx.tmpDir, ctx.outDir);
        break;
      } catch (e) {
        if (attempt < ATTEMPTS) continue;
        if (aside.length > 0 && !existsSync(ctx.outDir)) renameSync(aside.shift() as string, ctx.outDir);
        throw e;
      }
    }
    for (const old of aside) rmSync(old, { recursive: true, force: true });
  },
};
