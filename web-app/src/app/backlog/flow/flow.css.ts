import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const page = style({
  display: "flex",
  flexDirection: "column",
  height: "100%",
  overflow: "hidden",
});

export const scroller = style({
  flex: 1,
  overflowY: "auto",
  padding: `${vars.space["4"]} ${vars.space["6"]}`,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
});

export const header = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const title = style({
  margin: 0,
  fontSize: vars.fontSize.xl,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const controls = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "flex-start",
  gap: vars.space["6"],
});

export const facts = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: vars.space["3"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const textButton = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textPrimary,
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  cursor: "pointer",
  selectors: {
    '&[aria-disabled="true"]': { cursor: "progress", color: vars.color.textMuted },
  },
  ":hover": { background: vars.color.hoverBackground },
});

// Plain primary fails 4.5:1 on cardBackground in the light theme (4.32:1); blending toward the text color fixes it in every theme.
const linkColor = `color-mix(in srgb, ${vars.color.primary} 85%, ${vars.color.textPrimary})`;

export const linkButton = style({
  display: "inline-flex",
  alignItems: "center",
  minHeight: "24px",
  padding: 0,
  fontSize: vars.fontSize.sm,
  color: linkColor,
  background: "none",
  border: "none",
  textDecoration: "underline",
  cursor: "pointer",
});

export const jumpList = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: vars.space["3"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const jumpLink = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "24px",
  minHeight: "24px",
  color: linkColor,
});

export const notice = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.warningText,
  background: vars.color.warningBg,
  borderRadius: vars.radii.sm,
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
});

export const hint = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
});

export const staleBanner = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  fontSize: vars.fontSize.sm,
  color: vars.color.warningText,
  background: vars.color.warningBg,
  borderRadius: vars.radii.sm,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
});

export const staleLabel = style({
  fontWeight: vars.fontWeight.semibold,
});

export const dimmed = style({
  opacity: 0.55,
});

export const sections = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["6"],
});

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const sectionHeading = style({
  margin: 0,
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  ":focus": { outline: `2px solid ${vars.color.primary}`, outlineOffset: "2px" },
});

export const sectionSummary = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const sectionError = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  fontSize: vars.fontSize.sm,
  color: vars.color.errorText,
  background: vars.color.errorBg,
  borderRadius: vars.radii.sm,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
});

export const skeletonRows = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const stateBlock = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "flex-start",
  gap: vars.space["3"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
});

export const stateActions = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space["2"],
});
