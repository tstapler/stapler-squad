import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const wrapper = style({
  marginTop: vars.space["4"],
  borderTop: `1px solid ${vars.color.borderSubtle}`,
  paddingTop: vars.space["4"],
});

export const heading = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textSecondary,
  marginBottom: vars.space["2"],
});

export const grid = style({
  display: "grid",
  gridTemplateColumns: "1fr 1fr",
  gap: vars.space["3"],
  "@media": {
    "(max-width: 600px)": { gridTemplateColumns: "1fr" },
  },
});

export const column = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const colHeading = style({
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  marginBottom: vars.space["1"],
});

export const matchHead = style({
  color: vars.color.success,
});

export const noMatchHead = style({
  color: vars.color.error,
});

export const exampleRow = style({
  fontSize: vars.fontSize.xs,
  fontFamily: vars.font.mono,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  background: vars.color.surfaceSubtle,
  color: vars.color.textPrimary,
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
});

export const empty = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  fontStyle: "italic",
});

export const coverageBar = style({
  marginBottom: vars.space["3"],
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const coverageLabel = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  display: "flex",
  justifyContent: "space-between",
});

export const coverageTrack = style({
  height: 6,
  borderRadius: vars.radii.full,
  background: vars.color.surfaceSubtle,
  overflow: "hidden",
});

export const coverageFill = style({
  height: "100%",
  background: vars.color.success,
  borderRadius: vars.radii.full,
  transition: "width 0.2s ease",
});

export const suggestionsRow = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space["1"],
  alignItems: "center",
  marginBottom: vars.space["3"],
});

export const suggestionLabel = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  flexShrink: 0,
});

export const suggestionChip = style({
  fontSize: vars.fontSize.xs,
  fontFamily: vars.font.mono,
  padding: `2px ${vars.space["2"]}`,
  borderRadius: vars.radii.full,
  background: vars.color.surfaceSubtle,
  border: `1px solid ${vars.color.borderSubtle}`,
  color: vars.color.textSecondary,
  cursor: "pointer",
  lineHeight: 1.4,
  selectors: {
    "&:hover": {
      background: vars.color.surfaceMuted,
      borderColor: vars.color.borderHover,
      color: vars.color.textPrimary,
    },
  },
});

export const notice = style({
  marginTop: vars.space["2"],
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  background: vars.color.surfaceMuted,
  borderRadius: vars.radii.sm,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
});
