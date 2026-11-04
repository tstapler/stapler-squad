import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import { zIndex } from "@/styles/theme-contract.css";

export const previewFrame = style({
  display: "flex",
  width: "100%",
  height: "100%",
  gap: "2px",
  minWidth: 0,
  minHeight: 0,
});

export const previewLeaf = style({
  flex: 1,
  minWidth: 0,
  minHeight: 0,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  padding: "2px 4px",
  background: vars.color.hoverBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.xs,
  overflow: "hidden",
});

export const previewLeafFocused = style({
  borderColor: vars.color.primary,
});

export const previewLeafEmpty = style({
  color: vars.color.textMuted,
  fontStyle: "italic",
});

export const previewLeafLabel = style({
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  textAlign: "center",
});

export const card = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: vars.space["2"],
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  color: vars.color.textPrimary,
  cursor: "pointer",
  textAlign: "left",
  font: "inherit",
  selectors: {
    "&:hover, &:focus-visible": { borderColor: vars.color.primary },
  },
});

export const cardActive = style({
  borderColor: vars.color.primary,
});

export const cardTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const cardPreview = style({
  aspectRatio: "16 / 10",
  width: "100%",
});

export const popover = style({
  position: "absolute",
  top: "100%",
  left: 0,
  zIndex: zIndex.modal,
  display: "flex",
  gap: vars.space["2"],
  padding: vars.space["2"],
  maxWidth: "100vw",
  overflowX: "auto",
  background: vars.color.background,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  boxShadow: "0 8px 24px rgba(0, 0, 0, 0.35)",
});

export const popoverCard = style({
  width: "180px",
  flexShrink: 0,
});

export const overlay = style({
  position: "fixed",
  inset: 0,
  zIndex: zIndex.modal,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  background: "rgba(0, 0, 0, 0.6)",
});

export const overlayPanel = style({
  width: "min(1100px, 92vw)",
  maxHeight: "86vh",
  overflowY: "auto",
  padding: vars.space["4"],
  background: vars.color.background,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  outline: "none",
});

export const overlayGrid = style({
  display: "grid",
  gridTemplateColumns: "repeat(auto-fill, minmax(260px, 1fr))",
  gap: vars.space["3"],
});

export const overlayHint = style({
  marginBottom: vars.space["3"],
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
});
