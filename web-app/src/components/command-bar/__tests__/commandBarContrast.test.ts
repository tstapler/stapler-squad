/**
 * AC10 guard: every text/background token pair the command bar renders (see
 * CommandBar.css.ts) meets WCAG AA (4.5:1) in every theme. Reads the literal theme
 * values, like src/styles/__tests__/overlayContrastTokens.test.ts, so it needs no browser.
 */
import * as fs from "fs";
import * as path from "path";

const THEMES_SRC = fs.readFileSync(path.resolve(__dirname, "../../../styles/theme.css.ts"), "utf-8");
const THEME_NAMES = ["lightTheme", "darkTheme", "matrixTheme", "cyberpunk77Theme", "wh40kTheme", "cleanTheme"];
const AA_TEXT = 4.5;

function themeBlock(name: string): string {
  const start = THEMES_SRC.indexOf(`export const ${name} = createTheme(`);
  if (start < 0) throw new Error(`theme ${name} not found`);
  const next = THEMES_SRC.indexOf("export const ", start + 10);
  return THEMES_SRC.slice(start, next < 0 ? undefined : next);
}

function token(block: string, key: string): string {
  const match = block.match(new RegExp(`\\b${key}: "(#[0-9a-fA-F]{6})"`));
  if (!match) throw new Error(`token ${key} not found as a 6-digit hex`);
  return match[1];
}

function luminance(hex: string): number {
  const [r, g, b] = [1, 3, 5].map((i) => parseInt(hex.slice(i, i + 2), 16) / 255);
  const lin = (v: number) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4);
  return 0.2126 * lin(r) + 0.7152 * lin(g) + 0.0722 * lin(b);
}

function contrast(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

// [text token, background token, where it is used in CommandBar.css.ts]
const PAIRS: Array<[string, string, string]> = [
  ["textPrimary", "modalBackground", "row label on dialog"],
  ["textPrimary", "hoverBackground", "row label on selected row"],
  ["textSecondary", "modalBackground", "breadcrumb, group label, help, empty state, icon buttons"],
  ["textSecondary", "hoverBackground", "group label on selected row"],
  ["inputText", "inputBackground", "search input"],
  ["textPrimary", "surfaceSubtle", "key caps"],
];

describe("command bar text tokens meet WCAG AA in every theme", () => {
  for (const theme of THEME_NAMES) {
    for (const [text, bg, usage] of PAIRS) {
      it(`${theme}: ${text} on ${bg} (${usage})`, () => {
        const block = themeBlock(theme);
        expect(contrast(token(block, text), token(block, bg))).toBeGreaterThanOrEqual(AA_TEXT);
      });
    }
  }
});
