import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const provenance = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: vars.space["3"],
  margin: 0,
  padding: vars.space["3"],
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  "@media": {
    "(max-width: 640px)": { flexDirection: "column", alignItems: "stretch", gap: vars.space["2"] },
  },
});

export const provenanceItem = style({
  display: "flex",
  gap: vars.space["2"],
  minWidth: 0,
  overflowWrap: "anywhere",
});

export const provenanceLabel = style({ color: vars.color.textMuted, margin: 0 });

export const provenanceValue = style({ margin: 0 });

export const sourceBadge = style({
  padding: "0.125rem 0.5rem",
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.primary}`,
  color: vars.color.primary,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
});

export const updateBadge = style({
  display: "inline-flex",
  alignItems: "center",
  padding: "0.125rem 0.5rem",
  borderRadius: vars.radii.md,
  minHeight: "44px",
  maxWidth: "100%",
  overflow: "hidden",
  textOverflow: "ellipsis",
  background: vars.color.primary,
  color: vars.color.onPrimaryFill,
  fontSize: vars.fontSize.sm,
  textDecoration: "none",
  whiteSpace: "nowrap",
  selectors: { "&:hover": { background: vars.color.primaryHover } },
});

export const page = style({
  maxWidth: "860px",
  margin: "0 auto",
  padding: "2rem 1rem",
  height: "100%",
  overflowY: "auto",
  overflowX: "hidden",
  boxSizing: "border-box",
  "@media": {
    "(max-width: 640px)": { padding: "1rem 0.75rem" },
  },
});

export const title = style({
  color: vars.color.textPrimary,
  fontSize: "1.5rem",
  fontWeight: 700,
  margin: `0 0 ${vars.space["4"]}`,
  "@media": {
    "(max-width: 640px)": { fontSize: "1.25rem" },
  },
});

export const release = style({
  marginTop: vars.space["4"],
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  background: vars.color.cardBackground,
});

export const releaseCurrent = style({
  borderColor: vars.color.primary,
});

export const releaseHeader = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: vars.space["2"],
  width: "100%",
  padding: vars.space["3"],
  background: "transparent",
  border: "none",
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.medium,
  textAlign: "left",
  cursor: "pointer",
  minHeight: "44px",
});

export const releaseDate = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  fontWeight: 400,
});

export const hereChip = style({
  padding: "0.0625rem 0.5rem",
  borderRadius: vars.radii.md,
  background: vars.color.primary,
  color: vars.color.onPrimaryFill,
  fontSize: vars.fontSize.sm,
});

export const releaseBody = style({
  padding: `0 ${vars.space["3"]} ${vars.space["3"]}`,
  color: vars.color.textSecondary,
});

export const sectionTitle = style({
  margin: `${vars.space["3"]} 0 ${vars.space["1"]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textPrimary,
});

export const entryList = style({
  margin: 0,
  paddingLeft: "1.25rem",
  overflowWrap: "anywhere",
});

export const scope = style({
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textPrimary,
});

export const muted = style({
  color: vars.color.textMuted,
});

export const releaseHeading = style({ margin: 0 });
