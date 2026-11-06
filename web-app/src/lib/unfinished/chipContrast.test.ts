import * as fs from "fs";
import * as path from "path";
import { CHIP_TOKEN_PAIRS } from "./prChipTokens";

const THEME_SOURCE = fs.readFileSync(path.resolve(__dirname, "../../styles/theme.css.ts"), "utf-8");

/** Maps theme export name to its `color` tokens, parsed from the createTheme source text. */
function parseThemeColors(source: string): Record<string, Record<string, string>> {
  const themes: Record<string, Record<string, string>> = {};
  const starts = [...source.matchAll(/export const (\w+Theme) = createTheme\(vars, \{/g)];
  starts.forEach((m, i) => {
    const body = source.slice(m.index!, starts[i + 1]?.index ?? source.length);
    const colors: Record<string, string> = {};
    for (const t of body.matchAll(/\b(\w+): "(#[0-9a-fA-F]{6})"/g)) colors[t[1]] = t[2];
    themes[m[1]] = colors;
  });
  return themes;
}

function luminance(hex: string): number {
  const [r, g, b] = [1, 3, 5].map((i) => {
    const c = parseInt(hex.slice(i, i + 2), 16) / 255;
    return c <= 0.03928 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

export function contrastRatio(fg: string, bg: string): number {
  const [hi, lo] = [luminance(fg), luminance(bg)].sort((a, b) => b - a);
  return (hi + 0.05) / (lo + 0.05);
}

describe("chipContrast", () => {
  const themes = parseThemeColors(THEME_SOURCE);

  it("parses every theme", () => {
    expect(Object.keys(themes).sort()).toEqual(
      ["cleanTheme", "cyberpunk77Theme", "darkTheme", "lightTheme", "matrixTheme", "wh40kTheme"].sort()
    );
  });

  it("computes known WCAG ratios", () => {
    expect(contrastRatio("#000000", "#ffffff")).toBeCloseTo(21, 5);
    expect(contrastRatio("#767676", "#ffffff")).toBeGreaterThan(4.5);
  });

  it("chipContrast_should_BeAtLeast4Point5_When_EveryThemeChipAndBadgeTokenPair", () => {
    const failures: string[] = [];
    let checked = 0;
    for (const [theme, colors] of Object.entries(themes)) {
      for (const [name, { fg, bg }] of Object.entries(CHIP_TOKEN_PAIRS)) {
        expect(colors[fg]).toBeDefined();
        expect(colors[bg]).toBeDefined();
        checked++;
        const ratio = contrastRatio(colors[fg], colors[bg]);
        if (ratio < 4.5) failures.push(`${theme} ${name} ${fg} on ${bg} = ${ratio.toFixed(2)}`);
      }
    }
    expect(checked).toBe(6 * Object.keys(CHIP_TOKEN_PAIRS).length);
    expect(failures).toEqual([]);
  });
});
