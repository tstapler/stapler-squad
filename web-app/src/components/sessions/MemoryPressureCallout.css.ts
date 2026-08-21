import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const callout = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.75rem",
  padding: "1rem",
  background: vars.color.warningBg,
  border: `1px solid ${vars.color.warning}`,
  borderRadius: "0.5rem",
  marginBottom: "1rem",
});

export const header = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  gap: "0.5rem",
});

export const title = style({
  color: vars.color.warningText,
  fontWeight: 600,
  fontSize: "0.875rem",
});

export const dismissAll = style({
  background: "none",
  border: "none",
  color: vars.color.warningText,
  cursor: "pointer",
  fontSize: "0.75rem",
  padding: "0.125rem 0.25rem",
  opacity: 0.8,
  selectors: {
    "&:hover": {
      opacity: 1,
    },
  },
});

export const sessionList = style({
  display: "flex",
  flexDirection: "column",
  gap: "0.5rem",
});

export const sessionRow = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  gap: "0.5rem",
});

export const sessionName = style({
  color: vars.color.warningText,
  fontSize: "0.8125rem",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  flex: 1,
});

export const savings = style({
  color: vars.color.warning,
  fontSize: "0.75rem",
  fontWeight: 600,
  flexShrink: 0,
});

export const hibernateBtn = style({
  background: vars.color.warning,
  color: vars.color.textPrimary,
  border: "none",
  borderRadius: "0.25rem",
  padding: "0.125rem 0.5rem",
  fontSize: "0.75rem",
  fontWeight: 600,
  cursor: "pointer",
  flexShrink: 0,
  selectors: {
    "&:hover": {
      opacity: 0.9,
    },
  },
});

export const bulkAction = style({
  display: "flex",
  justifyContent: "flex-end",
});

export const hibernateAllBtn = style({
  background: vars.color.warning,
  color: vars.color.textPrimary,
  border: "none",
  borderRadius: "0.375rem",
  padding: "0.375rem 0.75rem",
  fontSize: "0.8125rem",
  fontWeight: 600,
  cursor: "pointer",
  selectors: {
    "&:hover": {
      opacity: 0.9,
    },
  },
});

export const pressureHighlight = style({
  borderLeft: `3px solid ${vars.color.warning}`,
});
