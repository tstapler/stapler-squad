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
      background: "rgba(59, 130, 246, 0.05)",
    },
    "&:focus": {
      outline: "none",
      borderColor: vars.color.primary,
      boxShadow: "0 0 0 2px rgba(59, 130, 246, 0.2)",
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
      color: "rgba(255, 255, 255, 0.9)",
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
      color: "rgba(255, 255, 255, 0.9)",
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
      color: "rgba(255, 255, 255, 0.9)",
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
      color: "rgba(255, 255, 255, 0.9)",
    },
  },
});

export const entryMessages = style({
  selectors: {
    [`${selected} &`]: {
      color: "rgba(255, 255, 255, 0.9)",
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
      color: "rgba(255, 255, 255, 0.9)",
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
      color: "rgba(255, 255, 255, 0.9)",
    },
  },
});

export const entryDirty = style({
  marginLeft: "6px",
  fontSize: "12px",
  color: vars.color.warning,

  selectors: {
    [`${selected} &`]: {
      color: "#fde68a",
    },
  },
});
