import { style, styleVariants } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
});

export const sectionTitle = style({
  margin: 0,
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const sectionDescription = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const grid = style({
  display: "grid",
  gridTemplateColumns: "repeat(auto-fill, minmax(160px, 1fr))",
  gap: vars.space["3"],
  "@media": {
    "(max-width: 640px)": {
      gridTemplateColumns: "repeat(2, 1fr)",
    },
  },
});

export const themeButton = style({
  position: "relative",
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: vars.space["3"],
  border: `2px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  background: vars.color.cardBackground,
  cursor: "pointer",
  textAlign: "left",
  transition: "border-color 0.15s ease, box-shadow 0.15s ease",
  selectors: {
    "&:hover": {
      borderColor: vars.color.borderHover,
      boxShadow: `0 0 0 1px ${vars.color.glowSecondary}`,
    },
    "&:focus-visible": {
      outline: `2px solid ${vars.color.primary}`,
      outlineOffset: "2px",
    },
  },
});

export const themeButtonActive = style({
  borderColor: vars.color.primary,
  boxShadow: `0 0 0 1px ${vars.color.glowPrimary}, inset 0 0 0 1px ${vars.color.primary}`,
});

export const previewSwatch = styleVariants({
  matrix: {
    height: "48px",
    borderRadius: vars.radii.md,
    background: "linear-gradient(135deg, #000000 0%, #0a1200 50%, #003300 100%)",
    border: "1px solid #003300",
  },
  cyberpunk77: {
    height: "48px",
    borderRadius: vars.radii.md,
    background: "linear-gradient(135deg, #0d0d1a 0%, #12122a 50%, #1a1a3e 100%)",
    border: "1px solid #ff2d78",
  },
  wh40k: {
    height: "48px",
    borderRadius: vars.radii.md,
    background: "linear-gradient(135deg, #0c0a08 0%, #1a1510 50%, #3d3020 100%)",
    border: "1px solid #c0a020",
  },
  clean: {
    height: "48px",
    borderRadius: vars.radii.md,
    background: "linear-gradient(135deg, #0f0f11 0%, #1a1a1f 50%, #22222a 100%)",
    border: "1px solid #7c3aed",
  },
  light: {
    height: "48px",
    borderRadius: vars.radii.md,
    background: "linear-gradient(135deg, #ffffff 0%, #f9f9f9 50%, #f0f0f0 100%)",
    border: "1px solid #e0e0e0",
  },
  dark: {
    height: "48px",
    borderRadius: vars.radii.md,
    background: "linear-gradient(135deg, #0a0a0a 0%, #1a1a1a 50%, #2a2a2a 100%)",
    border: "1px solid #333333",
  },
});

export const themeName = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const themeDescription = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const activeCheckmark = style({
  position: "absolute",
  top: vars.space["2"],
  right: vars.space["2"],
  width: "20px",
  height: "20px",
  borderRadius: vars.radii.full,
  background: vars.color.primary,
  color: vars.color.primaryText,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  fontSize: "10px",
  fontWeight: vars.fontWeight.bold,
});
