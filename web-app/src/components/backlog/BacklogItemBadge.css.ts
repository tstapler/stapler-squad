import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const badge = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["1"],
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  maxWidth: "260px",
  overflow: "hidden",
  borderRadius: vars.radii.sm,
  padding: `2px ${vars.space["2"]}`,
  border: `1px solid transparent`,
  whiteSpace: "nowrap",
  verticalAlign: "middle",
  lineHeight: "1.4",
});

export const statusChip = style({
  display: "inline-flex",
  alignItems: "center",
  borderRadius: vars.radii.sm,
  padding: `1px ${vars.space["1"]}`,
  fontSize: "10px",
  fontWeight: vars.fontWeight.semibold,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  flexShrink: 0,
});

export {
  statusIdea,
  statusReady,
  statusInProgress,
  statusReview,
  statusDone,
  statusArchived,
  statusRefining,
} from "@/styles/statusBadgeVariants.css";

export const acCount = style({
  color: vars.color.textSecondary,
  flexShrink: 0,
});

export const itemTitle = style({
  color: vars.color.textPrimary,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  flex: 1,
  minWidth: 0,
});
