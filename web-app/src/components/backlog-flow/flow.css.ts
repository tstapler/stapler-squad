import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const plainButton = {
  padding: 0,
  font: "inherit",
  background: "none",
  border: "none",
  cursor: "pointer",
  minHeight: "24px",
  minWidth: "24px",
} as const;

export const drillButton = style({
  ...plainButton,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.primary,
  textDecoration: "underline",
});

export const sortButton = style({
  ...plainButton,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const tileRow = style({
  display: "grid",
  gridTemplateColumns: "repeat(auto-fit, minmax(180px, 1fr))",
  gap: vars.space["3"],
});

export const tile = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  padding: vars.space["3"],
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
});

export const tileLabel = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const tileValue = style({
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const secondary = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const muted = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
});

export const lowSample = style({
  color: vars.color.textMuted,
  fontWeight: vars.fontWeight.normal,
});

export const link = style({
  color: vars.color.primary,
  fontSize: vars.fontSize.sm,
});

export const funnelList = style({
  margin: 0,
  padding: 0,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const funnelStage = style({
  listStyle: "none",
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
});

export const funnelBarTrack = style({
  height: "10px",
  background: vars.color.surfaceMuted,
  borderRadius: vars.radii.sm,
});

export const funnelBar = style({
  height: "100%",
  background: vars.color.primary,
  borderRadius: vars.radii.sm,
});

export const row = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: vars.space["3"],
});

export const backEdge = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space["2"],
  paddingLeft: vars.space["4"],
  color: vars.color.textSecondary,
});

export const table = style({
  width: "100%",
  borderCollapse: "collapse",
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
});

export const cell = style({
  textAlign: "left",
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  verticalAlign: "top",
});

export const caption = style({
  textAlign: "left",
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  paddingBottom: vars.space["1"],
});

export const scrollRegion = style({
  overflowX: "auto",
  ":focus-visible": { outline: `2px solid ${vars.color.primary}`, outlineOffset: "2px" },
});

export const block = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const subheading = style({
  margin: 0,
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const banner = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  fontSize: vars.fontSize.sm,
  color: vars.color.warningText,
  background: vars.color.warningBg,
  borderRadius: vars.radii.sm,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
});

export const chip = style({
  fontSize: vars.fontSize.xs,
  padding: `0 ${vars.space["2"]}`,
  border: `1px solid ${vars.color.borderStrong}`,
  borderRadius: vars.radii.sm,
  color: vars.color.textSecondary,
});

export const dialogContent = style({
  selectors: { "&&": { maxWidth: "720px" } },
});

export const dialogTitle = style({
  selectors: { "&:focus": { outline: "none" }, "&:focus-visible": { outline: `2px solid ${vars.color.primary}`, outlineOffset: "2px" } },
});

export const dialogMeta = style({
  margin: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
});

export const dialogRows = style({
  listStyle: "none",
  margin: 0,
  padding: 0,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const dialogClose = style({
  position: "absolute",
  top: vars.space["4"],
  right: vars.space["4"],
  minWidth: "32px",
  minHeight: "32px",
  fontSize: vars.fontSize.lg,
  color: vars.color.textMuted,
  background: "transparent",
  border: "none",
  cursor: "pointer",
});

export const textButton = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  cursor: "pointer",
  minHeight: "24px",
});
