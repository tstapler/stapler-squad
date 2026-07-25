import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import { breakpoints, zIndex } from "@/styles/theme-contract.css";

export const overlay = style({
  position: "fixed",
  inset: 0,
  background: vars.color.overlayBackground,
  display: "flex",
  alignItems: "stretch",
  justifyContent: "center",
  zIndex: zIndex.modal,
  padding: "3rem 1rem 1rem",
  "@media": {
    [`(max-width: ${breakpoints.md})`]: {
      padding: 0,
    },
  },
});

export const modal = style({
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.lg,
  display: "flex",
  flexDirection: "column",
  width: "min(900px, 95vw)",
  maxHeight: "calc(var(--viewport-height, 100dvh) - 4rem)",
  overflow: "hidden",
  "@media": {
    [`(max-width: ${breakpoints.md})`]: {
      position: "fixed",
      inset: 0,
      width: "100%",
      maxHeight: "var(--viewport-height, 100dvh)",
      borderRadius: 0,
      border: "none",
    },
  },
});

export const header = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: `${vars.space["3"]} ${vars.space["4"]}`,
  borderBottom: `1px solid ${vars.color.modalBorder}`,
  flexShrink: 0,
});

export const title = style({
  fontSize: vars.fontSize.base,
  fontWeight: 600,
  color: vars.color.textPrimary,
  margin: 0,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const subtitle = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  margin: 0,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const closeButton = style({
  background: "transparent",
  border: "none",
  color: vars.color.textMuted,
  fontSize: "1.25rem",
  cursor: "pointer",
  padding: vars.space["1"],
  lineHeight: 1,
  borderRadius: vars.radii.sm,
  flexShrink: 0,
  ":hover": {
    color: vars.color.textPrimary,
    background: vars.color.hoverBackground,
  },
});

export const body = style({
  flex: 1,
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
});
