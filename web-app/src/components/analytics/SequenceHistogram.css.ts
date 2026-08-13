import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  width: "100%",
});

export const emptyState = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  padding: vars.space["4"],
  textAlign: "center",
});

export const row = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  minHeight: "28px",
});

export const label = style({
  width: "160px",
  flexShrink: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  fontFamily: vars.font.mono,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const barTrack = style({
  flex: 1,
  height: "20px",
  backgroundColor: vars.color.surfaceMuted,
  borderRadius: vars.radii.sm,
  overflow: "hidden",
  display: "flex",
  position: "relative",
});

export const barFill = style({
  height: "100%",
  backgroundColor: vars.color.primary,
  borderRadius: vars.radii.sm,
  display: "flex",
  alignItems: "center",
  justifyContent: "flex-end",
  minWidth: "2px",
  position: "relative",
  transition: "width 0.2s ease",
});

export const barMangledFill = style({
  height: "100%",
  backgroundColor: vars.color.error,
  borderRadius: vars.radii.sm,
  minWidth: "2px",
  transition: "width 0.2s ease",
});

export const countLabel = style({
  width: "80px",
  flexShrink: 0,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  textAlign: "right",
  fontVariantNumeric: "tabular-nums",
});

export const legend = style({
  display: "flex",
  gap: vars.space["4"],
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  marginTop: vars.space["2"],
});

export const legendItem = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["1"],
});

export const legendDot = style({
  width: "10px",
  height: "10px",
  borderRadius: vars.radii.sm,
  backgroundColor: vars.color.primary,
  flexShrink: 0,
});

export const legendDotMangled = style({
  width: "10px",
  height: "10px",
  borderRadius: vars.radii.sm,
  backgroundColor: vars.color.error,
  flexShrink: 0,
});
