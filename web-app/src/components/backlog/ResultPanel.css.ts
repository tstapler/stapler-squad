import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  minWidth: 0,
  padding: vars.space["3"],
  border: `1px solid ${vars.color.borderSubtle}`,
  borderRadius: vars.radii.md,
  background: vars.color.cardBackground,
  fontSize: vars.fontSize.sm,
  overflowWrap: "anywhere",
});

export const header = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space["2"],
});

export const title = style({ margin: 0, fontSize: vars.fontSize.base, fontWeight: 600 });

export const meta = style({ display: "flex", flexWrap: "wrap", gap: vars.space["2"], color: vars.color.textMuted });

export const badge = style({
  fontSize: vars.fontSize.xs,
  padding: `2px ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  background: vars.color.hoverBackground,
  color: vars.color.textMuted,
  whiteSpace: "nowrap",
});

export const badgePass = style([badge, { background: vars.color.successBg, color: vars.color.successText }]);
export const badgeFail = style([badge, { background: vars.color.errorBg, color: vars.color.errorText }]);
export const badgeWarn = style([badge, { background: vars.color.warningBg, color: vars.color.warningText }]);

export const sectionLabel = style({ margin: 0, fontSize: vars.fontSize.xs, fontWeight: 600, color: vars.color.textMuted });

export const list = style({ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column" });

export const row = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "baseline",
  gap: vars.space["2"],
  padding: `${vars.space["1"]} 0`,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  selectors: { "&:last-child": { borderBottom: "none" } },
});

export const rowText = style({ flex: "1 1 12rem", minWidth: 0 });

export const evidence = style({ flexBasis: "100%", color: vars.color.textMuted, fontSize: vars.fontSize.xs });

export const mutedText = style({ color: vars.color.textMuted, margin: 0 });

export const disclosure = style({ fontSize: vars.fontSize.xs });

export const disclosureSummary = style({
  cursor: "pointer",
  minHeight: "44px",
  display: "inline-flex",
  alignItems: "center",
  color: vars.color.textMuted,
});

export const rawJson = style({
  margin: 0,
  padding: vars.space["2"],
  maxWidth: "100%",
  overflowX: "auto",
  fontFamily: vars.font.mono,
  background: vars.color.surfaceMuted,
  borderRadius: vars.radii.sm,
});

const pulse = keyframes({ "0%, 100%": { opacity: 0.4 }, "50%": { opacity: 1 } });

export const skeletonLine = style({
  height: "1em",
  borderRadius: vars.radii.sm,
  background: vars.color.hoverBackground,
  animation: `${pulse} 1.4s ease-in-out infinite`,
  "@media": { "(prefers-reduced-motion: reduce)": { animation: "none" } },
});
