/**
 * WCAG AA contrast ratio checker for all 4 themes.
 * Run: npm run check-contrast
 */

interface ThemeColors {
  textPrimary: string;
  textSecondary: string;
  textMuted: string;
  background: string;
  cardBackground: string;
  primary: string;
  primaryText: string;
}

const themes: Record<string, ThemeColors> = {
  matrix: {
    textPrimary: "#00ff41",
    textSecondary: "#00cc33",
    textMuted: "#00b32d", // was #004d18 in original — fixed for WCAG AA
    background: "#000000",
    cardBackground: "#0a0a0a",
    primary: "#00ff41",
    primaryText: "#000000",
  },
  cyberpunk77: {
    textPrimary: "#fcee09",
    textSecondary: "#c8be08",
    textMuted: "#aaaa00",
    background: "#0d0d1a",
    cardBackground: "#12122a",
    primary: "#cc245f",
    primaryText: "#ffffff",
  },
  wh40k: {
    textPrimary: "#c8b89a",
    textSecondary: "#a89878",
    textMuted: "#a08870",
    background: "#0c0a08",
    cardBackground: "#1a1510",
    primary: "#c0a020",
    primaryText: "#0c0a08",
  },
  clean: {
    textPrimary: "#ededed",
    textSecondary: "#b4b4b4",
    textMuted: "#8a8a8a",
    background: "#0f0f11",
    cardBackground: "#1a1a1f",
    primary: "#7c3aed",
    primaryText: "#ffffff",
  },
};

// WCAG relative luminance
function hexToRgb(hex: string): [number, number, number] {
  const r = parseInt(hex.slice(1, 3), 16);
  const g = parseInt(hex.slice(3, 5), 16);
  const b = parseInt(hex.slice(5, 7), 16);
  return [r, g, b];
}

function linearize(c: number): number {
  const s = c / 255;
  return s <= 0.04045 ? s / 12.92 : Math.pow((s + 0.055) / 1.055, 2.4);
}

function luminance(hex: string): number {
  const [r, g, b] = hexToRgb(hex);
  return 0.2126 * linearize(r) + 0.7152 * linearize(g) + 0.0722 * linearize(b);
}

function contrast(fg: string, bg: string): number {
  const l1 = luminance(fg);
  const l2 = luminance(bg);
  const lighter = Math.max(l1, l2);
  const darker = Math.min(l1, l2);
  return (lighter + 0.05) / (darker + 0.05);
}

const WCAG_AA_NORMAL = 4.5;
const WCAG_AA_LARGE = 3.0;

let failures = 0;

console.log("\nWCAG AA Contrast Check\n" + "=".repeat(60));

for (const [themeName, colors] of Object.entries(themes)) {
  console.log(`\n[${themeName.toUpperCase()}]`);
  const pairs: Array<[string, string, string, number]> = [
    ["textPrimary", "background", colors.textPrimary + " on " + colors.background, contrast(colors.textPrimary, colors.background)],
    ["textSecondary", "background", colors.textSecondary + " on " + colors.background, contrast(colors.textSecondary, colors.background)],
    ["textMuted", "cardBackground", colors.textMuted + " on " + colors.cardBackground, contrast(colors.textMuted, colors.cardBackground)],
    ["primaryText", "primary", colors.primaryText + " on " + colors.primary, contrast(colors.primaryText, colors.primary)],
  ];
  for (const [fgName, bgName, desc, ratio] of pairs) {
    const pass = ratio >= WCAG_AA_NORMAL;
    const passLarge = ratio >= WCAG_AA_LARGE;
    const status = pass ? "PASS" : passLarge ? "LARGE-ONLY" : "FAIL";
    console.log(`  [${status}]  ${fgName}/${bgName}: ${ratio.toFixed(2)}:1  (${desc})`);
    if (!pass) failures++;
  }
}

console.log("\n" + "=".repeat(60));
if (failures > 0) {
  console.log(`\nFAIL: ${failures} contrast pair(s) failed WCAG AA. Fix before merging.\n`);
  process.exit(1);
} else {
  console.log("\nPASS: All contrast pairs pass WCAG AA.\n");
}
