import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const summaryLine = style({ display: "flex", alignItems: "center", flexWrap: "wrap", gap: vars.space["2"], justifyContent: "space-between" });
export const summaryText = style({ margin: 0, fontSize: vars.fontSize.sm, fontWeight: 600, color: vars.color.textPrimary, overflowWrap: "anywhere" });
export const progress = style({
  appearance: "none",
  width: "100%",
  height: "8px",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  background: vars.color.surfaceSubtle,
  accentColor: vars.color.primary,
  selectors: {
    "&::-webkit-progress-bar": { background: vars.color.surfaceSubtle, borderRadius: vars.radii.sm },
    "&::-webkit-progress-value": { background: vars.color.primary, borderRadius: vars.radii.sm },
    "&::-moz-progress-bar": { background: vars.color.primary, borderRadius: vars.radii.sm },
  },
});
export const ruleLine = style({ margin: 0, fontSize: vars.fontSize.xs, color: vars.color.textSecondary, overflowWrap: "anywhere" });
export const holds = style({ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: vars.space["1"] });
export const stack = style({ display: "flex", flexDirection: "column", gap: vars.space["2"], minWidth: 0 });
