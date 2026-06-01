// +feature: insights-dashboard
import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";
import { zIndex } from "@/styles/theme-contract.css";

const slideIn = keyframes({
  from: { transform: "translateX(100%)" },
  to: { transform: "translateX(0)" },
});

export const overlay = style({
  position: "fixed",
  inset: 0,
  background: "rgba(0,0,0,0.4)",
  zIndex: zIndex.slideOver - 1,
});

export const drawer = style({
  position: "fixed",
  top: 0,
  right: 0,
  height: "100vh",
  width: "min(480px, 90vw)",
  overflowY: "auto",
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRight: "none",
  zIndex: zIndex.slideOver,
  animation: `${slideIn} 0.2s ease-out`,
  display: "flex",
  flexDirection: "column",
  gap: 0,
});

export const drawerHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: `${vars.space[4]} ${vars.space[4]}`,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  flexShrink: 0,
});

export const drawerTitle = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
});

export const sessionIdChip = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  background: vars.color.surfaceSubtle,
  padding: `1px ${vars.space[2]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderSubtle}`,
});

export const closeButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "28px",
  height: "28px",
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderSubtle}`,
  background: "transparent",
  color: vars.color.textMuted,
  cursor: "pointer",
  fontSize: vars.fontSize.base,
  flexShrink: 0,
  ":hover": {
    background: vars.color.hoverBackground,
    color: vars.color.textPrimary,
  },
});

export const section = style({
  padding: `${vars.space[4]} ${vars.space[4]}`,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  display: "flex",
  flexDirection: "column",
  gap: vars.space[3],
  ":last-child": {
    borderBottom: "none",
  },
});

export const sectionTitle = style({
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  margin: 0,
});

export const metaGrid = style({
  display: "grid",
  gridTemplateColumns: "auto 1fr",
  gap: `${vars.space[1]} ${vars.space[3]}`,
  alignItems: "baseline",
});

export const metaLabel = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  whiteSpace: "nowrap",
});

export const metaValue = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  wordBreak: "break-all",
});

export const toolsTable = style({
  width: "100%",
  borderCollapse: "collapse",
  fontSize: vars.fontSize.sm,
});

export const toolsTh = style({
  textAlign: "left",
  padding: `${vars.space[1]} ${vars.space[2]}`,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  fontWeight: vars.fontWeight.medium,
});

export const toolsThRight = style([toolsTh, { textAlign: "right" }]);

export const toolsTd = style({
  padding: `${vars.space[2]} ${vars.space[2]}`,
  color: vars.color.textPrimary,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  fontSize: vars.fontSize.xs,
});

export const toolsTdRight = style([toolsTd, { textAlign: "right" }]);

export const skillList = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space[1],
});

export const skillBadge = style({
  display: "inline-block",
  padding: `2px ${vars.space[2]}`,
  background: vars.color.accentBg,
  color: vars.color.textSecondary,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.xs,
  border: `1px solid ${vars.color.borderSubtle}`,
});

export const emptyState = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  fontStyle: "italic",
});
