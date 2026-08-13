import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const entryCard = style({
  padding: "14px 16px",
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "8px",
  cursor: "pointer",
  transition: "all 0.15s ease",

  selectors: {
    "&:hover": {
      borderColor: vars.color.primary,
      background: vars.color.accentBg,
    },
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
      boxShadow: `0 0 0 2px ${vars.color.glowPrimary}`,
    },
  },
});

export const selected = style({
  backgroundColor: vars.color.primary,
  borderColor: vars.color.primary,
});

export const entryHeader = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "baseline",
  marginBottom: "6px",
  gap: "12px",
});

export const entryName = style({
  fontWeight: "600",
  fontSize: "15px",
  color: vars.color.textPrimary,
  flex: 1,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",

  selectors: {
    [`${selected} &`]: {
      color: vars.color.textInverse,
    },
  },
});

export const entryTime = style({
  fontSize: "12px",
  color: vars.color.textMuted,
  whiteSpace: "nowrap",
  flexShrink: 0,

  selectors: {
    [`${selected} &`]: {
      color: vars.color.textInverse,
    },
  },
});

export const entryMeta = style({
  fontSize: "13px",
  color: vars.color.textSecondary,
  marginBottom: "4px",
  display: "flex",
  alignItems: "center",
  flexWrap: "wrap",

  selectors: {
    [`${selected} &`]: {
      color: vars.color.textInverse,
    },
  },
});

export const entryModel = style({
  fontWeight: "500",
});

export const entryDivider = style({
  margin: "0 8px",
  color: vars.color.textMuted,

  selectors: {
    [`${selected} &`]: {
      color: vars.color.textInverse,
    },
  },
});

export const entryMessages = style({
  selectors: {
    [`${selected} &`]: {
      color: vars.color.textInverse,
    },
  },
});

export const entryProject = style({
  fontSize: "12px",
  color: vars.color.textMuted,
  marginTop: "6px",
  wordBreak: "break-all",
  fontFamily: "monospace",

  selectors: {
    [`${selected} &`]: {
      color: vars.color.textInverse,
    },
  },
});

export const entryBranch = style({
  fontSize: "12px",
  fontFamily: "monospace",
  color: vars.color.primary,
  fontWeight: "500",

  selectors: {
    [`${selected} &`]: {
      color: vars.color.textInverse,
    },
  },
});

export const entryDirty = style({
  marginLeft: "6px",
  fontSize: "12px",
  color: vars.color.warning,

  selectors: {
    [`${selected} &`]: {
      color: vars.color.warningText,
    },
  },
});

export const headerRight = style({
  display: "flex",
  alignItems: "center",
  gap: "8px",
  flexShrink: 0,
});

export const statusPill = style({
  display: "inline-flex",
  alignItems: "center",
  gap: "4px",
  fontSize: "11px",
  fontWeight: "600",
  padding: "1px 6px",
  borderRadius: vars.radii.full,
});

export const status_running = style({
  background: vars.color.successBg,
  color: vars.color.success,
});

export const status_paused = style({
  background: vars.color.warningBg,
  color: vars.color.warning,
});

export const status_creating = style({
  background: vars.color.accentBg,
  color: vars.color.primary,
});

export const status_stopped = style({
  background: vars.color.hoverBackground ?? "rgba(0,0,0,0.06)",
  color: vars.color.textMuted,
});

export const statusDot = style({
  width: "6px",
  height: "6px",
  borderRadius: vars.radii.full,
  background: "currentColor",
  animation: "pulse 2s ease-in-out infinite",
});

export const expandRow = style({
  marginTop: "6px",
});

export const expandButton = style({
  background: "none",
  border: "none",
  padding: "0",
  cursor: "pointer",
  fontSize: "11px",
  color: vars.color.textMuted,

  selectors: {
    "&:hover": {
      color: vars.color.primary,
    },
    [`${selected} &`]: {
      color: vars.color.textInverse,
    },
  },
});
