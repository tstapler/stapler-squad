import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const formWrapper = style({
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  padding: vars.space["4"],
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
});

export const formTitle = style({
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  marginBottom: vars.space["2"],
});

export const modeToggle = style({
  display: "flex",
  gap: "0",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  overflow: "hidden",
  alignSelf: "flex-start",
});

export const modeBtn = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  fontSize: vars.fontSize.sm,
  border: "none",
  background: "transparent",
  color: vars.color.textSecondary,
  cursor: "pointer",
  fontWeight: vars.fontWeight.medium,
  transition: "background 0.1s, color 0.1s",
  ":hover": { background: vars.color.hoverBackground },
});

export const modeBtnActive = style({
  background: vars.color.primary,
  color: vars.color.textInverse,
  ":hover": { background: vars.color.primaryHover },
});

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
  borderTop: `1px solid ${vars.color.borderSubtle}`,
  paddingTop: vars.space["3"],
});

export const sectionTitle = style({
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  textTransform: "uppercase",
  letterSpacing: "0.06em",
  color: vars.color.textMuted,
  marginBottom: vars.space["1"],
});

export const formGrid = style({
  display: "grid",
  gridTemplateColumns: "1fr 1fr",
  gap: vars.space["3"],
  "@media": {
    "(max-width: 600px)": { gridTemplateColumns: "1fr" },
  },
});

export const formGridFull = style({
  gridColumn: "1 / -1",
});

export const fieldLabel = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
});

export const fieldInput = style({
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  fontSize: vars.fontSize.sm,
  fontFamily: vars.font.mono,
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "1px",
  },
});

export const fieldSelect = style({
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  ":focus": {
    borderColor: vars.color.inputFocusBorder,
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "1px",
  },
});

export const decisionSegment = style({
  display: "flex",
  gap: vars.space["2"],
  flexWrap: "wrap",
});

export const decisionBtn = style({
  flex: 1,
  minWidth: "100px",
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  background: "transparent",
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  transition: "all 0.1s",
  ":hover": { background: vars.color.hoverBackground },
});

export const decisionAllow = style({
  borderColor: vars.color.success,
  color: vars.color.success,
  background: vars.color.successBg,
});

export const decisionDeny = style({
  borderColor: vars.color.error,
  color: vars.color.error,
  background: vars.color.errorBg,
});

export const decisionEscalate = style({
  borderColor: vars.color.warning,
  color: vars.color.warning,
  background: vars.color.warningBg,
});

export const checkboxRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  cursor: "pointer",
});

export const priorityHint = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  marginTop: vars.space["1"],
});

export const priorityWarning = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.warning,
  background: vars.color.warningBg,
  borderRadius: vars.radii.sm,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  marginTop: vars.space["1"],
});

export const errorBanner = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.errorText,
  background: vars.color.errorBg,
  borderRadius: vars.radii.md,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
});

export const actions = style({
  display: "flex",
  gap: vars.space["2"],
  paddingTop: vars.space["2"],
});

export const saveBtn = style({
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  background: vars.color.primary,
  color: vars.color.textInverse,
  border: "none",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  cursor: "pointer",
  ":hover": { background: vars.color.primaryHover },
  ":disabled": { opacity: 0.5, cursor: "not-allowed" },
});

export const cancelBtn = style({
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  background: "transparent",
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  ":hover": { background: vars.color.hoverBackground },
});

export const advancedToggle = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  background: "none",
  border: "none",
  cursor: "pointer",
  padding: 0,
  textDecoration: "underline",
  alignSelf: "flex-start",
  ":hover": { color: vars.color.textPrimary },
});

export const radioGroup = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const radioRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  cursor: "pointer",
});

export const pythonSection = style({
  border: `1px solid ${vars.color.borderSubtle}`,
  borderRadius: vars.radii.md,
  padding: vars.space["3"],
  background: vars.color.surfaceSubtle,
});

export const pythonSectionTitle = style({
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textMuted,
  marginBottom: vars.space["2"],
  textTransform: "uppercase",
  letterSpacing: "0.05em",
});

export const checkboxGrid = style({
  display: "grid",
  gridTemplateColumns: "1fr 1fr",
  gap: vars.space["2"],
});
