import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// ux.md: the mobile layout is everything below 900px.
const narrow = "screen and (max-width: 899px)";

export const root = style({
  display: "flex",
  flexDirection: "column",
  gap: "8px",
  width: "100%",
});

export const buttons = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: "8px",
  "@media": {
    [narrow]: { flexDirection: "column", alignItems: "stretch", flexWrap: "nowrap" },
  },
});

export const actionButton = style({
  minHeight: "32px",
  padding: "0.25rem 0.75rem",
  fontSize: "0.8125rem",
  fontWeight: 600,
  borderRadius: "6px",
  cursor: "pointer",
  background: "transparent",
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderColor}`,
  selectors: {
    '&[aria-disabled="true"]': { opacity: 0.55, cursor: "not-allowed" },
  },
  "@media": {
    [narrow]: { width: "100%", minHeight: "44px", fontSize: "1rem" },
    // Opacity is not a signal in forced-colors mode; use the system disabled colour.
    "(forced-colors: active)": {
      selectors: { '&[aria-disabled="true"]': { color: "GrayText", borderColor: "GrayText", opacity: 1 } },
    },
  },
});

export const approveVariant = style({ color: vars.color.success, borderColor: vars.color.success });
export const denyVariant = style({ color: vars.color.error, borderColor: vars.color.error });
export const secondaryVariant = style({ color: vars.color.primary, borderColor: vars.color.primary });

export const subStep = style({
  display: "flex",
  flexDirection: "column",
  gap: "8px",
});

export const field = style({
  width: "100%",
  boxSizing: "border-box",
  // 16px stops iOS Safari zooming the page on focus.
  fontSize: "16px",
  minHeight: "44px",
  padding: "0.5rem",
  borderRadius: "6px",
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
});

export const status = style({
  margin: 0,
  fontSize: "0.8125rem",
  color: vars.color.textSecondary,
  overflowWrap: "anywhere",
});

export const commandRegion = style({
  display: "flex",
  flexDirection: "column",
  gap: "4px",
});

export const commandText = style({
  margin: 0,
  fontFamily: vars.font.mono,
  fontSize: "0.8125rem",
  whiteSpace: "pre-wrap",
  overflowWrap: "anywhere",
  background: vars.color.surfaceMuted,
  padding: "0.5rem",
  borderRadius: "6px",
});

export const commandScroll = style({
  maxHeight: "12em",
  overflowY: "auto",
  selectors: {
    "&:focus-visible": { outline: `2px solid ${vars.color.primary}`, outlineOffset: "2px" },
  },
});

export const commandMeta = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  fontSize: "0.75rem",
  color: vars.color.textSecondary,
});

export const menu = style({
  display: "flex",
  flexDirection: "column",
  gap: "4px",
  padding: "4px",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "6px",
  background: vars.color.cardBackground,
});

export const stepActions = style({
  display: "flex",
  flexWrap: "wrap",
  gap: "8px",
});
