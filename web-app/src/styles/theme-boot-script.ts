// Pure string builder for the first-paint theme script. Imports nothing from theme.css.ts so the Bun
// build can feed it a map extracted through its own vanilla-extract plugin instead of evaluating VE twice.
export const THEME_NAMES = ["matrix", "cyberpunk77", "wh40k", "clean", "light", "dark"] as const;
export type ThemeName = (typeof THEME_NAMES)[number];
export type ThemeMap = Record<ThemeName, string>;

/** Inline, synchronous: runs in <head> before first paint so ThemeBoot classes always have CSS behind them. */
export function buildThemeBootScript(map: ThemeMap): string {
  const themeMapJson = JSON.stringify(map);
  // Only theme classes are removed, so other classes on <html> survive theme switching.
  const allThemeClasses = Object.values(map).join(" ");
  return `(function(){try{var m=${themeMapJson};var t=localStorage.getItem('stapler-theme');var cls=t&&m[t]?m[t]:m['clean'];if(t&&t.indexOf('custom:')===0){var c=JSON.parse(localStorage.getItem('stapler-theme-custom')||'null');if(c&&c.id===t&&typeof c.cls==='string'&&/^\\S+$/.test(c.cls)&&c.props){cls=c.cls;Object.keys(c.props).forEach(function(k){document.documentElement.style.setProperty(k,c.props[k]);});}}var themeClasses=${JSON.stringify(allThemeClasses)}.split(' ');themeClasses.forEach(function(c){if(c)document.documentElement.classList.remove(c);});document.documentElement.classList.add(cls);}catch(e){try{document.documentElement.classList.add(${JSON.stringify(map.clean)});}catch(e2){}}})();`;
}
