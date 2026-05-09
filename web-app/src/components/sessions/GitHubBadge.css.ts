import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const badge = style({
  display: "inline-flex",
  alignItems: "center",
  gap: 4,
  padding: "4px 10px",
  borderRadius: 12,
  fontSize: 12,
  fontWeight: 600,
  textDecoration: "none",
  transition: "all 0.2s ease",
  cursor: "pointer",
  border: "1px solid transparent",
  selectors: {
    "&:hover": {
      transform: "translateY(-1px)",
      boxShadow: "0 2px 4px rgba(0, 0, 0, 0.1)",
    },
    "&:active": {
      transform: "translateY(0)",
    },
    "&:focus": {
      outline: "2px solid #0969da",
      outlineOffset: 2,
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      fontSize: 11,
      padding: "3px 8px",
    },
  },
});

export const compact = style({
  padding: "4px 8px",
  fontSize: 11,
});

export const prBadge = style({
  background: "#8250df",
  color: "#ffffff",
  borderColor: "#6e40c9",
  selectors: {
    "&:hover": {
      background: "#6e40c9",
      boxShadow: "0 2px 6px rgba(130, 80, 223, 0.3)",
    },
  },
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#8957e5",
      color: "#ffffff",
      borderColor: "#7741d9",
    },
  },
});

export const repoBadge = style({
  background: "#f6f8fa",
  color: "#24292f",
  borderColor: "#d0d7de",
  selectors: {
    "&:hover": {
      background: "#eaeef2",
      borderColor: "#afb8c1",
      boxShadow: "0 2px 6px rgba(0, 0, 0, 0.1)",
    },
  },
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#21262d",
      color: "#c9d1d9",
      borderColor: "#30363d",
    },
  },
});

export const prBadgeBlocking = style({
  background: vars.color.error,
  color: "#ffffff",
  borderColor: vars.color.error,
  selectors: {
    "&:hover": {
      opacity: 0.9,
      boxShadow: "0 2px 6px rgba(0, 0, 0, 0.2)",
    },
  },
});

export const prBadgeReady = style({
  background: vars.color.success,
  color: "#ffffff",
  borderColor: vars.color.success,
  selectors: {
    "&:hover": {
      opacity: 0.9,
      boxShadow: "0 2px 6px rgba(0, 0, 0, 0.2)",
    },
  },
});

export const prBadgePending = style({
  background: vars.color.warning,
  color: "#1a1a1a",
  borderColor: vars.color.warning,
  selectors: {
    "&:hover": {
      opacity: 0.9,
      boxShadow: "0 2px 6px rgba(0, 0, 0, 0.2)",
    },
  },
});

export const prBadgeDraft = style({
  background: vars.color.textMuted,
  color: "#ffffff",
  borderColor: vars.color.textMuted,
  selectors: {
    "&:hover": {
      opacity: 0.9,
      boxShadow: "0 2px 6px rgba(0, 0, 0, 0.15)",
    },
  },
});

export const prBadgeComplete = style({
  background: vars.color.textSecondary,
  color: "#ffffff",
  borderColor: vars.color.textSecondary,
  opacity: 0.8,
  selectors: {
    "&:hover": {
      opacity: 1,
    },
  },
});

export const prBadgeError = style({
  background: vars.color.error,
  color: "#ffffff",
  borderColor: vars.color.error,
  selectors: {
    "&:hover": {
      opacity: 0.9,
      boxShadow: "0 2px 6px rgba(0, 0, 0, 0.2)",
    },
  },
});

export const prBadgeUnknown = style({
  background: vars.color.surfaceSubtle,
  color: vars.color.textSecondary,
  borderColor: vars.color.borderColor,
  selectors: {
    "&:hover": {
      opacity: 0.9,
      boxShadow: "0 2px 6px rgba(0, 0, 0, 0.1)",
    },
  },
});

export const priorityLabel = style({
  fontSize: 10,
  fontWeight: 700,
  letterSpacing: "0.03em",
  textTransform: "uppercase",
  opacity: 0.9,
  paddingLeft: 2,
  borderLeft: "1px solid rgba(255, 255, 255, 0.4)",
  marginLeft: 2,
});

export const icon = style({
  width: 14,
  height: 14,
  flexShrink: 0,
  "@media": {
    "screen and (max-width: 768px)": {
      width: 12,
      height: 12,
    },
  },
});

export const text = style({
  whiteSpace: "nowrap",
  overflow: "hidden",
  textOverflow: "ellipsis",
  maxWidth: 200,
  "@media": {
    "screen and (max-width: 768px)": {
      maxWidth: 150,
    },
  },
});
