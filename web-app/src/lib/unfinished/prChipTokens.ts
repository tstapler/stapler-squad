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
  // Session PR chip states (session-multi-pr-tracking S1/S10). Shape and text carry the state too (WCAG 1.4.1).
  prOpen: { fg: "successText", bg: "successBg" },
  prMerged: { fg: "textSecondary", bg: "surfaceSubtle" },
  prClosed: { fg: "errorText", bg: "errorBg" },
  prChecking: { fg: "textSecondary", bg: "surfaceSubtle" },
  prStale: { fg: "warningText", bg: "warningBg" },
} as const satisfies Record<string, ChipTokenPair>;
