/**
 * MC-1 guard (notification toast deck + tray): the solid-fill text tokens meet WCAG AA against
 * their fill in every theme, and terminal chrome pinned inside the pane stays below the
 * tray and toast layers. The rendered-pixel check lives in
 * tests/e2e/notification-contrast-over-xterm.spec.ts; this keeps the token pairs and the
 * z-index ladder from regressing without a browser.
 */
import * as fs from "fs";
import * as path from "path";

const STYLES = path.resolve(__dirname, "..");
const THEMES_SRC = fs.readFileSync(path.join(STYLES, "theme.css.ts"), "utf-8");
const CONTRACT_SRC = fs.readFileSync(path.join(STYLES, "theme-contract.css.ts"), "utf-8");

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
  if (!match) throw new Error(`token ${key} not found`);
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

describe("solid-fill text tokens meet WCAG AA on their fill", () => {
  const pairs: Array<[string, string]> = [
    ["onPrimaryFill", "primary"],
    ["onSuccessFill", "success"],
    ["onErrorFill", "error"],
  ];
  for (const theme of THEME_NAMES) {
    for (const [text, fill] of pairs) {
      it(`${theme}: ${text} on ${fill}`, () => {
        const block = themeBlock(theme);
        expect(contrast(token(block, text), token(block, fill))).toBeGreaterThanOrEqual(AA_TEXT);
      });
    }
  }
});

describe("terminal pane chrome stays under the tray and toast layers", () => {
  const level = (name: string) => {
    const match = CONTRACT_SRC.match(new RegExp(`\\b${name}: (\\d+)`));
    if (!match) throw new Error(`zIndex.${name} not found`);
    return Number(match[1]);
  };

  it("terminalPaneChrome is below slideOver and toast", () => {
    expect(level("terminalPaneChrome")).toBeLessThan(level("slideOver"));
    expect(level("terminalPaneChrome")).toBeLessThan(level("toast"));
  });

  const absoluteChrome: Array<[string, string]> = [
    ["components/sessions/XtermTerminal.css.ts", "scrollTrack"],
    ["components/sessions/XtermTerminal.css.ts", "scrollbackCopyButton"],
    ["components/sessions/JumpToLatestButton.css.ts", "button"],
    ["components/sessions/ScrollingPanel.css.ts", "panel"],
  ];
  for (const [file, exportName] of absoluteChrome) {
    it(`${exportName} in ${path.basename(file)} uses terminalPaneChrome`, () => {
      const src = fs.readFileSync(path.resolve(STYLES, "..", file), "utf-8");
      const start = src.indexOf(`export const ${exportName} = style(`);
      expect(start).toBeGreaterThanOrEqual(0);
      const end = src.indexOf("\nexport const ", start + 10);
      const body = src.slice(start, end < 0 ? undefined : end);
      expect(body).toContain("zIndex.terminalPaneChrome");
      expect(body).not.toContain("zIndex.floatingTerminalUI");
    });
  }
});
