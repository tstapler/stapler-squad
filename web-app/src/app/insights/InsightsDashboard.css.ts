// +feature: insights-dashboard
import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

const pulse = keyframes({
  "0%, 100%": { opacity: 1 },
  "50%": { opacity: 0.5 },
});

export const page = style({
  padding: `${vars.space[6]} ${vars.space[4]}`,
  maxWidth: "1200px",
  margin: "0 auto",
  display: "flex",
  flexDirection: "column",
  gap: vars.space[6],
});

export const pageHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  flexWrap: "wrap",
  gap: vars.space[2],
});

export const title = style({
  fontSize: vars.fontSize.xl,
  fontWeight: vars.fontWeight.bold,
  color: vars.color.textPrimary,
  margin: 0,
});

export const subtitle = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  marginTop: vars.space[1],
});

export const liveIndicator = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[1],
  fontSize: vars.fontSize.xs,
  color: vars.color.success,
  padding: `${vars.space[1]} ${vars.space[2]}`,
  borderRadius: vars.radii.full,
  background: vars.color.successBg,
  border: `1px solid ${vars.color.success}`,
});

export const liveDot = style({
  width: "6px",
  height: "6px",
  borderRadius: vars.radii.full,
  background: vars.color.success,
  animation: `${pulse} 1.5s ease-in-out infinite`,
});

export const loadingBanner = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
  padding: `${vars.space[2]} ${vars.space[3]}`,
  background: vars.color.accentBg,
  border: `1px solid ${vars.color.borderSubtle}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const spinner = style({
  width: "14px",
  height: "14px",
  border: `2px solid ${vars.color.borderSubtle}`,
  borderTopColor: vars.color.primary,
  borderRadius: vars.radii.full,
  animation: `${keyframes({
    to: { transform: "rotate(360deg)" },
  })} 0.8s linear infinite`,
  flexShrink: 0,
});

export const errorBox = style({
  padding: `${vars.space[4]} ${vars.space[4]}`,
  background: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: vars.radii.md,
  color: vars.color.errorText,
  fontSize: vars.fontSize.sm,
});

export const emptyState = style({
  padding: `${vars.space[12]} ${vars.space[4]}`,
  textAlign: "center",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.base,
});

export const grid2 = style({
  display: "grid",
  gridTemplateColumns: "repeat(auto-fit, minmax(280px, 1fr))",
  gap: vars.space[4],
});

export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[3],
});

export const sectionTitle = style({
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  margin: 0,
});
