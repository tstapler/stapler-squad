// Dev-server twin of the inline boot script build/html.ts injects. Module scripts run after parsing, so
// dev can flash the default theme for a frame before this applies the stored one (docs/how-to/web-dev-server.md).
import { buildThemeBootScript, THEME_NAMES, type ThemeMap, type ThemeName } from "@/styles/theme-boot-script";
import * as themes from "@/styles/theme.css";

const map = Object.fromEntries(THEME_NAMES.map((name) => [name, themes[`${name}Theme` as keyof typeof themes]])) as ThemeMap;

// Same script as production so the localStorage keys and custom-theme handling cannot drift.
new Function(buildThemeBootScript(map))();

export type { ThemeName };
