import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import { zIndex } from "@/styles/theme-contract.css";
import { MIN_TOUCH_TARGET } from "@/lib/unfinished/prTouchTokens";

const NARROW = "screen and (max-width: 480px)";

export const backdrop = style({
  position: "fixed",
  inset: 0,
  zIndex: zIndex.dialog,
  background: vars.color.overlayBackground,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  padding: vars.space["4"],
  "@media": { [NARROW]: { alignItems: "flex-end", padding: 0 } },
});

export const sheet = style({
  width: "min(480px, 100%)",
  maxHeight: "var(--viewport-height, 100dvh)",
  overflowY: "auto",
  background: vars.color.modalBackground,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.lg,
  padding: vars.space["4"],
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
  overflowWrap: "anywhere",
  "@media": {
    [NARROW]: {
      borderRadius: `${vars.radii.lg} ${vars.radii.lg} 0 0`,
      paddingBottom: "calc(16px + env(safe-area-inset-bottom, 0px))",
    },
  },
});

export const form = style({ display: "flex", flexDirection: "column", gap: vars.space["3"], margin: 0 });

export const title = style({ margin: 0, fontSize: vars.fontSize.lg, fontWeight: 600 });
export const body = style({ margin: 0, fontSize: vars.fontSize.sm, color: vars.color.textSecondary });

export const field = style({ display: "flex", flexDirection: "column", gap: vars.space["1"], fontSize: vars.fontSize.sm });

export const input = style({
  minHeight: MIN_TOUCH_TARGET,
  padding: `0 ${vars.space["2"]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: "16px",
  ":focus-visible": { outline: `2px solid ${vars.color.inputFocusBorder}`, outlineOffset: "2px" },
});

export const errorBox = style({
  margin: 0,
  padding: vars.space["2"],
  borderRadius: vars.radii.md,
  background: vars.color.errorBg,
  color: vars.color.errorText,
  fontSize: vars.fontSize.sm,
});

export const actions = style({
  display: "flex",
  gap: vars.space["2"],
  justifyContent: "flex-end",
  flexWrap: "wrap",
  "@media": { [NARROW]: { flexDirection: "column-reverse" } },
});

export const button = style({
  minHeight: MIN_TOUCH_TARGET,
  minWidth: MIN_TOUCH_TARGET,
  padding: `0 ${vars.space["4"]}`,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.surfaceSubtle,
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  cursor: "pointer",
  textDecoration: "none",
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  ":disabled": { opacity: 0.6, cursor: "not-allowed" },
  ":focus-visible": { outline: `2px solid ${vars.color.inputFocusBorder}`, outlineOffset: "2px" },
});

export const primaryButton = style({
  background: vars.color.primary,
  color: vars.color.onPrimaryFill,
  borderColor: vars.color.primaryDark,
});

export const dangerButton = style({
  background: vars.color.error,
  color: vars.color.onErrorFill,
  borderColor: vars.color.errorDark,
});
