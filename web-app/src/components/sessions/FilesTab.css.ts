import { style, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  height: "100%",
  overflow: "hidden",
  background: vars.color.terminalBackground,
});

export const treePane = style({
  display: "flex",
  flexDirection: "column",
  flexShrink: 0,
  borderRight: `1px solid ${vars.color.borderColor}`,
  overflow: "hidden",
});

export const treePaneCollapsed = style({
  width: "0 !important",
  overflow: "hidden",
  borderRight: "none",
  minWidth: "0 !important",
});

export const mobilePaneHidden = style({
  "@media": {
    "(max-width: 767px)": {
      display: "none !important",
    },
  },
});

export const mobilePaneVisible = style({
  "@media": {
    "(max-width: 767px)": {
      display: "flex !important",
      flex: 1,
      width: "100%",
      maxWidth: "none",
    },
  },
});

export const mobileBackButton = style({
  display: "none",
  "@media": {
    "(max-width: 767px)": {
      display: "flex",
      alignItems: "center",
      gap: 4,
      padding: "8px 12px",
      background: "transparent",
      border: "none",
      cursor: "pointer",
      color: vars.color.primary,
      fontSize: 13,
      borderBottom: `1px solid ${vars.color.borderColor}`,
    },
  },
});

export const contentPane = style({
  flex: 1,
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
});

export const toolbar = style({
  display: "flex",
  alignItems: "center",
  gap: 6,
  padding: "5px 8px",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  background: vars.color.terminalTabsBg,
  flexShrink: 0,
});

export const searchInput = style({
  flex: 1,
  minWidth: 0,
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 4,
  color: vars.color.terminalForeground,
  fontSize: 12,
  padding: "3px 8px",
  outline: "none",
  selectors: {
    "&:focus": {
      borderColor: vars.color.primary,
    },
    "&::placeholder": {
      color: vars.color.textMuted,
    },
  },
});

export const toolbarLabel = style({
  display: "flex",
  alignItems: "center",
  gap: 3,
  fontSize: 11,
  color: vars.color.textMuted,
  cursor: "pointer",
  flexShrink: 0,
  whiteSpace: "nowrap",
  selectors: {
    "&:hover": {
      color: vars.color.terminalForeground,
    },
  },
});

globalStyle(`${toolbarLabel} input[type='checkbox']`, {
  cursor: "pointer",
  accentColor: vars.color.primary,
});

export const toolbarButton = style({
  flexShrink: 0,
  padding: "2px 7px",
  fontSize: 13,
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 3,
  color: vars.color.textMuted,
  cursor: "pointer",
  lineHeight: 1.4,
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.terminalHoverBg,
      color: vars.color.terminalForeground,
    },
    "&:disabled": {
      opacity: 0.5,
      cursor: "default",
    },
  },
});

export const searchCount = style({
  flexShrink: 0,
  fontSize: 11,
  color: vars.color.textMuted,
  whiteSpace: "nowrap",
});

export const treeWrapper = style({
  flex: 1,
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
});
