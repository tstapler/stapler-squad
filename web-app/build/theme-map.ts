// Extracts the theme name-to-class map through the build's own VE compiler, so the boot script and the
// bundled CSS share identifiers (a second evaluation could disagree under another identOption).
import { THEME_NAMES, type ThemeMap, type ThemeName } from "../src/styles/theme-boot-script";
import type { VeCompiler } from "./plugins/vanilla-extract";

export async function buildThemeMap(ve: VeCompiler, themeFile: string): Promise<ThemeMap> {
  const exports = await ve.classExports(themeFile);
  const map = {} as Record<ThemeName, string>;
  for (const name of THEME_NAMES) {
    const cls = exports[`${name}Theme`];
    if (!cls) throw new Error(`${themeFile} does not export ${name}Theme (found: ${Object.keys(exports).join(", ") || "none"})`);
    map[name] = cls;
  }
  return map;
}
