// Post-processes the HTML Bun emits for the index.html entry (layout A', gate S1).
import { buildThemeBootScript, type ThemeMap } from "../src/styles/theme-boot-script";
import { FONT_PRELOAD_PLACEHOLDER, FONT_URL_PREFIX, THEME_BOOT_PLACEHOLDER } from "./config";

/** Injects the boot script at the placeholder and gives <html> the clean theme class, like layout.tsx does. */
export function injectThemeBoot(html: string, map: ThemeMap): string {
  const parts = html.split(THEME_BOOT_PLACEHOLDER);
  if (parts.length !== 2) throw new Error(`expected exactly one ${THEME_BOOT_PLACEHOLDER} in the emitted HTML, found ${parts.length - 1}`);
  const withScript = `${parts[0]}<script>${buildThemeBootScript(map)}</script>${parts[1]}`;
  if (!/<html\b[^>]*>/.test(withScript)) throw new Error("emitted HTML has no <html> tag");
  return withScript.replace(/<html\b([^>]*)>/, (_m, attrs: string) => `<html${attrs.replace(/\sclass="[^"]*"/, "")} class="${map.clean}">`);
}

/** Bun cannot resolve an external font href in the HTML entry, so the preload link is added after bundling. */
export function injectFontPreload(html: string, hashedFiles: string[]): string {
  const parts = html.split(FONT_PRELOAD_PLACEHOLDER);
  if (parts.length !== 2) throw new Error(`expected exactly one ${FONT_PRELOAD_PLACEHOLDER} in the emitted HTML, found ${parts.length - 1}`);
  const links = hashedFiles.map((f) => `<link rel="preload" as="font" type="font/woff2" crossorigin href="${FONT_URL_PREFIX}${f}">`).join("");
  return `${parts[0]}${links}${parts[1]}`;
}
