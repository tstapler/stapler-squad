import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const dialog = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[4],
});

export const heading = style({
  margin: 0,
  fontSize: vars.fontSize.lg,
  fontWeight: 700,
  color: vars.color.textPrimary,
});

export const fieldGroup = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.3rem",
});

export const label = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 500,
  color: vars.color.textSecondary,
});

export const textarea = style({
  padding: `${vars.space[2]} ${vars.space[3]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  outline: "none",
  width: "100%",
  boxSizing: "border-box",
  resize: "vertical",
  minHeight: "120px",
  fontFamily: "inherit",
  ":focus": { borderColor: vars.color.inputFocusBorder },
});

export const dropZone = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  gap: vars.space[2],
  padding: vars.space[4],
  border: `2px dashed ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.sm,
  textAlign: "center",
});

export const dropZoneActive = style({
  borderColor: vars.color.primary,
  background: vars.color.hoverBackground,
});

export const thumbs = style({
  display: "grid",
  gridTemplateColumns: "repeat(auto-fill, minmax(96px, 1fr))",
  gap: vars.space[2],
  listStyle: "none",
  margin: 0,
  padding: 0,
});

export const thumb = style({
  position: "relative",
  display: "flex",
  flexDirection: "column",
  gap: "0.25rem",
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
});

export const thumbImage = style({
  width: "100%",
  height: "80px",
  objectFit: "cover",
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.hoverBackground,
});

export const thumbDim = style({ opacity: 0.5 });

export const thumbName = style({
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const thumbError = style({ color: vars.color.error });

// 44px square: the minimum comfortable touch target.
export const removeButton = style({
  position: "absolute",
  top: 0,
  right: 0,
  width: "44px",
  height: "44px",
  display: "flex",
  alignItems: "flex-start",
  justifyContent: "flex-end",
  padding: "4px",
  background: "transparent",
  border: "none",
  cursor: "pointer",
});

export const removeGlyph = style({
  width: "22px",
  height: "22px",
  lineHeight: "22px",
  borderRadius: "50%",
  background: vars.color.textPrimary,
  color: vars.color.textInverse,
  fontSize: vars.fontSize.sm,
  textAlign: "center",
});

export const errorText = style({
  color: vars.color.error,
  fontSize: vars.fontSize.sm,
  margin: 0,
});

export const hint = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  margin: 0,
});

export const actions = style({
  display: "flex",
  justifyContent: "flex-end",
  gap: vars.space[3],
});

const buttonBase = {
  minHeight: "44px",
  padding: `${vars.space[2]} ${vars.space[4]}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
} as const;

export const secondaryButton = style({
  ...buttonBase,
  background: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  fontWeight: 500,
  ":hover": { background: vars.color.hoverBackground, color: vars.color.textPrimary },
  ":disabled": { opacity: 0.6, cursor: "not-allowed" },
});

export const primaryButton = style({
  ...buttonBase,
  background: vars.color.primary,
  color: vars.color.textInverse,
  border: "none",
  fontWeight: 600,
  ":hover": { background: vars.color.primaryHover },
  ":disabled": { opacity: 0.6, cursor: "not-allowed" },
});

export const hiddenInput = style({ display: "none" });
