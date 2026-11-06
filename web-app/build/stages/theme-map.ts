import { buildThemeMap } from "../theme-map";
import type { BuildContext } from "./context";
import type { Stage } from "./runner";

export const themeMap: Stage<BuildContext> = {
  name: "theme-map",
  async run(ctx) {
    ctx.themeMap = await buildThemeMap(ctx.ve, ctx.themeFile);
  },
};
