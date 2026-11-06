/**
 * Foreground/background `vars.color` token names for PR chips and the tab badge. Shared by
 * `PRCard.css.ts`/`UpNextTabs.css.ts` and `chipContrast.test.ts`, so the contrast test checks the
 * pairs that actually render.
 */
export interface ChipTokenPair {
  fg: "textSecondary" | "successText" | "warningText" | "errorText";
  bg: "surfaceSubtle" | "successBg" | "warningBg" | "errorBg";
}

export const CHIP_TOKEN_PAIRS = {
  neutral: { fg: "textSecondary", bg: "surfaceSubtle" },
  draft: { fg: "textSecondary", bg: "surfaceSubtle" },
  success: { fg: "successText", bg: "successBg" },
  warning: { fg: "warningText", bg: "warningBg" },
  error: { fg: "errorText", bg: "errorBg" },
  tabBadge: { fg: "errorText", bg: "errorBg" },
} as const satisfies Record<string, ChipTokenPair>;
