import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const pulse = keyframes({
  "0%, 100%": { opacity: 1 },
  "50%": { opacity: 0.7 },
});

export const cardExpired = style({
  opacity: 0.6,
  borderLeftColor: vars.color.borderColor,
});

export const card = style({
  background: vars.color.background,
  border: `1px solid ${vars.color.borderColor}`,
  borderLeft: "3px solid #f59e0b",
  borderRadius: vars.radii.lg,
  padding: vars.space["4"],
  transition: "all 0.2s ease",
  selectors: {
    "&:hover": {
      borderColor: vars.color.borderHover,
      boxShadow: "0 2px 8px rgba(0, 0, 0, 0.1)",
    },
  },
  "@media": {
    "(max-width: 640px)": {
      padding: vars.space["3"],
    },
  },
});

export const header = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  marginBottom: vars.space["3"],
  "@media": {
    "(max-width: 640px)": {
      flexWrap: "wrap",
      gap: vars.space["2"],
      marginBottom: vars.space["2"],
    },
  },
});

export const toolName = style({
  display: "flex",
  alignItems: "center",
  gap: "6px",
  fontSize: "15px",
  fontWeight: 700,
  color: vars.color.textPrimary,
  "@media": {
    "(max-width: 640px)": {
      fontSize: vars.fontSize.base,
    },
  },
});

export const toolIcon = style({
  fontSize: "16px",
  flexShrink: 0,
});

export const countdown = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  fontVariantNumeric: "tabular-nums",
  padding: `2px ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  whiteSpace: "nowrap",
});

export const countdownNormal = style({
  background: "#dbeafe",
  color: "#1e40af",
  border: "1px solid #93c5fd",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#1e3a8a",
      color: "#dbeafe",
      borderColor: "#1e40af",
    },
  },
});

export const countdownWarning = style({
  background: "#fef3c7",
  color: "#92400e",
  border: "1px solid #fcd34d",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#78350f",
      color: "#fef3c7",
      borderColor: "#92400e",
    },
  },
});

export const countdownUrgent = style({
  background: "#fee2e2",
  color: "#991b1b",
  border: "1px solid #fca5a5",
  animationName: pulse,
  animationDuration: "1.5s",
  animationTimingFunction: "ease-in-out",
  animationIterationCount: "infinite",
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: "#7f1d1d",
      color: "#fecaca",
      borderColor: "#991b1b",
    },
  },
});

export const body = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  marginBottom: vars.space["3"],
});

export const detail = style({
  display: "flex",
  gap: vars.space["2"],
  alignItems: "baseline",
  fontSize: "13px",
  "@media": {
    "(max-width: 640px)": {
      flexDirection: "column",
      gap: "2px",
    },
  },
});

export const detailLabel = style({
  color: vars.color.textSecondary,
  fontWeight: 500,
  minWidth: "70px",
  flexShrink: 0,
  "@media": {
    "(max-width: 640px)": {
      minWidth: "unset",
    },
  },
});

export const detailValue = style({
  color: vars.color.textPrimary,
  fontFamily: "monospace",
  fontSize: vars.fontSize.sm,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  flex: 1,
});

export const toolInputPreview = style({
  background: vars.color.surfaceSubtle,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: `${vars.space["2"]} 12px`,
  fontFamily: "monospace",
  fontSize: vars.fontSize.sm,
  lineHeight: 1.5,
  color: vars.color.textPrimary,
  overflowX: "auto",
  whiteSpace: "pre-wrap",
  wordBreak: "break-all",
  maxHeight: "80px",
  overflowY: "auto",
  "@media": {
    "(max-width: 640px)": {
      maxHeight: "60px",
      fontSize: "11px",
    },
  },
});

export const detailsToggle = style({
  marginBottom: vars.space["2"],
});

export const detailsButton = style({
  background: "none",
  border: "none",
  padding: `2px 0`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  cursor: "pointer",
  transition: "color 0.15s",
  selectors: {
    "&:hover": { color: vars.color.textPrimary },
  },
});

export const fullDetails = style({
  margin: `6px 0 0`,
  padding: `${vars.space["2"]} 10px`,
  background: vars.color.surfaceSubtle,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  fontFamily: "monospace",
  fontSize: vars.fontSize.xs,
  lineHeight: 1.5,
  color: vars.color.textPrimary,
  maxHeight: "8em",
  overflowY: "auto",
  wordBreak: "break-all",
  whiteSpace: "pre-wrap",
});

export const actions = style({
  display: "flex",
  gap: vars.space["2"],
  paddingTop: vars.space["3"],
  borderTop: `1px solid ${vars.color.borderColor}`,
  "@media": {
    "(max-width: 640px)": {
      flexDirection: "column",
      gap: "6px",
      paddingTop: "10px",
    },
  },
});

export const approveButton = style({
  flex: 1,
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  border: "none",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.base,
  fontWeight: 600,
  cursor: "pointer",
  transition: "all 0.15s ease",
  background: "#22c55e",
  color: "white",
  selectors: {
    "&:hover": {
      background: "#16a34a",
      transform: "translateY(-1px)",
      boxShadow: "0 4px 8px rgba(0, 0, 0, 0.2)",
    },
  },
  "@media": {
    "(max-width: 640px)": {
      padding: `10px 12px`,
    },
  },
});

export const denyButton = style({
  flex: 1,
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  border: "none",
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.base,
  fontWeight: 600,
  cursor: "pointer",
  transition: "all 0.15s ease",
  background: "#ef4444",
  color: "white",
  selectors: {
    "&:hover": {
      background: "#dc2626",
      transform: "translateY(-1px)",
      boxShadow: "0 4px 8px rgba(0, 0, 0, 0.2)",
    },
  },
  "@media": {
    "(max-width: 640px)": {
      padding: `10px 12px`,
    },
  },
});

export const dismissButton = style({
  flex: 1,
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.base,
  fontWeight: 600,
  cursor: "pointer",
  transition: "all 0.15s ease",
  background: vars.color.surfaceSubtle,
  color: vars.color.textSecondary,
  selectors: {
    "&:hover": {
      background: vars.color.borderColor,
      color: vars.color.textPrimary,
    },
  },
});
