import { check, defaultEvalDir } from "../../scripts/verify-eval-patch.mjs";
import type { Stage } from "./runner";
import type { BuildContext } from "./context";

/** First stage: a drifted eval patch otherwise surfaces later as an opaque "setAdapter of undefined" (ADR-008). */
export const guard: Stage<BuildContext> = {
  name: "guard",
  run() {
    const problem = check(defaultEvalDir());
    if (problem) throw new Error(problem);
  },
};
