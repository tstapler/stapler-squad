import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const pulse = keyframes({ "0%, 100%": { opacity: 0.5 }, "50%": { opacity: 1 } });

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[2],
  padding: `${vars.space[2]} 16px`,
});

export const groupLabel = style({
  fontSize: "0.75rem",
  fontWeight: 600,
  letterSpacing: "0.3px",
  textTransform: "uppercase",
  color: vars.color.textSecondary,
  margin: 0,
});

export const rowList = style({
  listStyle: "none",
  margin: 0,
  padding: 0,
  display: "flex",
  flexDirection: "column",
  gap: vars.space[2],
});

export const row = style({
  display: "flex",
  flexDirection: "column",
  gap: "6px",
  padding: "12px",
  border: `1px solid ${vars.color.borderColor}`,
  borderLeft: `3px solid ${vars.color.error}`,
  borderRadius: "8px",
  background: vars.color.cardBackground,
  minWidth: 0,
});

export const rowNeedsHuman = style({
  borderLeftColor: vars.color.warning,
});

export const rowHead = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
  flexWrap: "wrap",
  minWidth: 0,
});

export const rowTitle = style({
  color: vars.color.textPrimary,
  fontWeight: 600,
  fontSize: "0.9375rem",
  overflowWrap: "anywhere",
});

export const statusLabel = style({
  fontSize: "0.625rem",
  fontWeight: 700,
  textTransform: "uppercase",
  letterSpacing: "0.3px",
  padding: "2px 6px",
  borderRadius: "3px",
  color: "white",
  backgroundColor: vars.color.error,
  // Darkens the status colour so white text stays >= 4.5:1.
  backgroundImage: "linear-gradient(rgba(0, 0, 0, 0.4), rgba(0, 0, 0, 0.4))",
  whiteSpace: "nowrap",
});

export const statusLabelNeedsHuman = style({
  backgroundColor: vars.color.warning,
  backgroundImage: "linear-gradient(rgba(0, 0, 0, 0.55), rgba(0, 0, 0, 0.55))",
});

export const rowMeta = style({
  fontSize: "0.8125rem",
  color: vars.color.textSecondary,
});

export const rowMessage = style({
  fontSize: "0.875rem",
  color: vars.color.textPrimary,
  margin: 0,
  overflowWrap: "anywhere",
});

export const rowActions = style({
  display: "flex",
  gap: vars.space[2],
  flexWrap: "wrap",
});

export const action = style({
  minWidth: "44px",
  minHeight: "44px",
  padding: "0 14px",
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  borderRadius: "8px",
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
  fontWeight: 500,
  textDecoration: "none",
  cursor: "pointer",
  selectors: {
    "&:hover": { backgroundColor: vars.color.hoverBackground },
    "&:focus-visible": { outline: `2px solid ${vars.color.primary}`, outlineOffset: "2px" },
  },
});

export const summary = style({
  display: "flex",
  flexDirection: "column",
  gap: "4px",
  paddingTop: vars.space[2],
  borderTop: `1px solid ${vars.color.borderColor}`,
  fontSize: "0.875rem",
  color: vars.color.textPrimary,
});

export const footer = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space[2],
  fontSize: "0.8125rem",
  color: vars.color.textSecondary,
});

export const stateBox = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  gap: vars.space[2],
  padding: "24px 16px",
  textAlign: "center",
  color: vars.color.textPrimary,
});

export const staleLine = style({
  fontSize: "0.8125rem",
  color: vars.color.textSecondary,
  margin: 0,
});

export const skeleton = style({
  height: "72px",
  borderRadius: "8px",
  background: vars.color.surfaceMuted,
  animation: `${pulse} 1.4s ease-in-out infinite`,
  "@media": { "(prefers-reduced-motion: reduce)": { animation: "none" } },
});

export const successIcon = style({ color: vars.color.successText, fontSize: "1.5rem" });

export const segments = style({
  display: "flex",
  gap: vars.space[1],
  padding: `0 16px`,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  flexShrink: 0,
});

export const segment = style({
  minHeight: "44px",
  padding: "0 14px",
  background: "transparent",
  border: "none",
  borderBottom: "2px solid transparent",
  color: vars.color.textSecondary,
  fontSize: "0.875rem",
  fontWeight: 500,
  cursor: "pointer",
  selectors: {
    "&[aria-selected='true']": { color: vars.color.textPrimary, borderBottomColor: vars.color.primary },
    "&:focus-visible": { outline: `2px solid ${vars.color.primary}`, outlineOffset: "-2px" },
  },
});
