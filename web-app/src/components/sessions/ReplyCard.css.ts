import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// Sits directly below the read-only banner, above the terminal (RP-1). Sized from
// --viewport-height so it stays above the soft keyboard (RP-9).
export const card = style({
  flex: "0 0 auto",
  maxHeight: "calc(var(--viewport-height, 100dvh) * 0.5)",
  overflowY: "auto",
  display: "flex",
  flexDirection: "column",
  gap: vars.space[2],
  padding: `${vars.space[3]} ${vars.space[3]}`,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const heading = style({
  margin: 0,
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.semibold,
});

export const prompt = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  whiteSpace: "pre-wrap",
  overflowWrap: "anywhere",
});

export const disclosure = style({
  alignSelf: "flex-start",
  minHeight: "44px",
  padding: "0 12px",
  background: "transparent",
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  cursor: "pointer",
});

export const options = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[2],
});

export const option = style({
  minHeight: "44px",
  padding: "10px 14px",
  textAlign: "left",
  fontSize: vars.fontSize.sm,
  background: vars.color.modalBackground,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  cursor: "pointer",
  selectors: {
    "&:disabled": { opacity: 0.6, cursor: "not-allowed" },
    "&[aria-pressed='true']": { borderColor: vars.color.primary, fontWeight: vars.fontWeight.semibold },
  },
});

export const status = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
});

export const footer = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const actionButton = style({
  alignSelf: "flex-start",
  minHeight: "44px",
  padding: "0 16px",
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: "6px",
  cursor: "pointer",
});
