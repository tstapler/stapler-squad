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

export const notice = style({
  marginTop: vars.space["2"],
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  background: vars.color.surfaceMuted,
  borderRadius: vars.radii.sm,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
});
