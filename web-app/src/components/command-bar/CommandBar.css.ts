import { style, keyframes } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

import { ROW_HEIGHT_PX, LIST_MAX_HEIGHT_PX } from "./commandBarState";

const fadeIn = keyframes({ from: { opacity: 0, transform: "translateY(-8px)" }, to: { opacity: 1, transform: "none" } });

export const backdrop = style({
  position: "fixed",
  inset: 0,
  background: vars.color.overlayBackground,
  zIndex: zIndex.dialog,
  display: "flex",
  alignItems: "flex-start",
  justifyContent: "center",
  padding: `12vh ${vars.space[4]} ${vars.space[4]}`,
});

export const dialog = style({
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.lg,
  boxShadow: vars.shadow.lg,
  width: "100%",
  maxWidth: "560px",
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
  animation: `${fadeIn} 120ms ease-out`,
  "@media": {
    "(prefers-reduced-motion: reduce)": { animation: "none" },
  },
});

export const header = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
  padding: vars.space[2],
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const searchInput = style({
  flex: 1,
  minWidth: 0,
  minHeight: `${ROW_HEIGHT_PX}px`,
  background: vars.color.inputBackground,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: `0 ${vars.space[3]}`,
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.base,
  ":focus-visible": { outline: `2px solid ${vars.color.primary}`, outlineOffset: "1px" },
});

export const iconButton = style({
  minWidth: `${ROW_HEIGHT_PX}px`,
  minHeight: `${ROW_HEIGHT_PX}px`,
  background: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  cursor: "pointer",
  fontFamily: vars.font.mono,
  ":hover": { background: vars.color.hoverBackground, color: vars.color.textPrimary },
  ":focus-visible": { outline: `2px solid ${vars.color.primary}`, outlineOffset: "1px" },
});

export const breadcrumb = style({
  padding: `${vars.space[2]} ${vars.space[3]}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  fontFamily: vars.font.mono,
});

export const list = style({
  listStyle: "none",
  margin: 0,
  padding: 0,
  maxHeight: `${LIST_MAX_HEIGHT_PX}px`,
  overflowY: "auto",
});

export const row = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[3],
  height: `${ROW_HEIGHT_PX}px`,
  minHeight: `${ROW_HEIGHT_PX}px`,
  padding: `0 ${vars.space[3]}`,
  color: vars.color.textPrimary,
  cursor: "pointer",
  boxSizing: "border-box",
  selectors: {
    '&[aria-selected="true"]': { background: vars.color.hoverBackground, boxShadow: `inset 3px 0 0 ${vars.color.primary}` },
  },
});

export const rowLabel = style({ flex: 1, minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" });
export const rowGroup = style({ fontSize: vars.fontSize.sm, color: vars.color.textSecondary });
export const keys = style({ display: "flex", gap: vars.space[1], fontFamily: vars.font.mono });

export const keyCap = style({
  minWidth: "1.75em",
  textAlign: "center",
  padding: "2px 6px",
  border: `1px solid ${vars.color.borderStrong}`,
  borderRadius: vars.radii.sm,
  background: vars.color.surfaceSubtle,
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
});

export const empty = style({ padding: vars.space[4], color: vars.color.textSecondary, textAlign: "center" });

export const help = style({
  padding: vars.space[3],
  borderTop: `1px solid ${vars.color.borderColor}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  lineHeight: 1.6,
});

export const srOnly = style({
  position: "absolute",
  width: "1px",
  height: "1px",
  overflow: "hidden",
  clip: "rect(0 0 0 0)",
  whiteSpace: "nowrap",
});
