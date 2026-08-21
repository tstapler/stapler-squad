import { style, keyframes, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const pulse = keyframes({
  "0%, 100%": { opacity: 1 },
  "50%": { opacity: 0.5 },
});

const spin = keyframes({
  to: { transform: "rotate(360deg)" },
});

const slideIn = keyframes({
  from: { opacity: 0, transform: "translateY(-6px)" },
  to: { opacity: 1, transform: "translateY(0)" },
});

export const wrapper = style({
  position: "relative",
});

export const trigger = style({
  display: "flex",
  alignItems: "center",
  gap: "0.375rem",
  padding: "0.375rem 0.625rem",
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
  color: vars.color.textSecondary,
  fontSize: "0.8125rem",
  fontWeight: "500",
  cursor: "pointer",
  transition: "all 0.2s ease",
  whiteSpace: "nowrap",
  maxWidth: "160px",
  overflow: "hidden",

  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
      background: vars.color.hoverBackground,
      borderColor: vars.color.borderHover,
    },
  },
});

export const triggerName = style({
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const chevron = style({
  flexShrink: 0,
  transition: "transform 0.2s ease",
});

export const chevronOpen = style({
  transform: "rotate(180deg)",
});

export const switching = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
  padding: "0.375rem 0.75rem",
  fontSize: "0.8125rem",
  color: vars.color.textSecondary,
  animation: `${pulse} 1.5s ease-in-out infinite`,
});

export const dropdown = style({
  position: "absolute",
  top: "calc(100% + 0.375rem)",
  right: 0,
  minWidth: "220px",
  maxWidth: "320px",
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: "0.5rem",
  boxShadow: "0 8px 24px rgba(0, 0, 0, 0.4)",
  zIndex: 1000,
  overflow: "hidden",
});

export const dropdownHeader = style({
  padding: "0.5rem 0.75rem",
  fontSize: "0.6875rem",
  fontWeight: "600",
  textTransform: "uppercase",
  letterSpacing: "0.06em",
  color: vars.color.textMuted,
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const dropdownList = style({
  listStyle: "none",
  margin: 0,
  padding: "0.25rem 0",
  maxHeight: "320px",
  overflowY: "auto",
});

// Declare these before workspaceItem so they can be referenced in selectors
export const workspaceItemCurrent = style({
  background: vars.color.primaryActive,
});

export const workspaceItemLoading = style({
  opacity: 0.7,
});

export const mergeButton = style({
  flexShrink: 0,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "2.5rem",
  height: "1.5rem",
  marginRight: "0.5rem",
  padding: "0 0.5rem",
  borderRadius: "0.25rem",
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
  color: vars.color.textSecondary,
  fontSize: "0.6875rem",
  fontWeight: "500",
  cursor: "pointer",
  opacity: 0,
  transition: "opacity 0.15s ease, color 0.15s ease, border-color 0.15s ease, background 0.15s ease",

  selectors: {
    "&:hover:not(:disabled)": {
      color: vars.color.textPrimary,
      borderColor: vars.color.borderHover,
      background: vars.color.hoverBackground,
    },
    "&:disabled": {
      cursor: "default",
      opacity: 0.4,
    },
  },
});

export const workspaceItem = style({
  display: "flex",
  alignItems: "center",
  width: "100%",
  transition: "background 0.15s ease",

  selectors: {
    [`&:not(.${workspaceItemCurrent}):not(.${workspaceItemLoading}):hover`]: {
      background: vars.color.hoverBackground,
    },
  },
});

globalStyle(
  `.${workspaceItem}:not(.${workspaceItemCurrent}):not(.${workspaceItemLoading}):hover .${mergeButton}`,
  { opacity: 1 },
);

export const workspaceItemMain = style({
  display: "flex",
  alignItems: "center",
  gap: "0.625rem",
  flex: 1,
  minWidth: 0,
  padding: "0.5rem 0.375rem 0.5rem 0.75rem",
  background: "transparent",
  border: "none",
  cursor: "pointer",
  textAlign: "left",

  selectors: {
    [`${workspaceItemCurrent} &`]: {
      cursor: "default",
    },
  },
});

export const workspaceIcon = style({
  flexShrink: 0,
  width: "1.25rem",
  height: "1.25rem",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  color: vars.color.textSecondary,
});

export const workspaceIconCurrent = style({
  color: vars.color.primary,
});

export const workspaceInfo = style({
  flex: 1,
  minWidth: 0,
});

export const workspaceName = style({
  fontSize: "0.875rem",
  fontWeight: "500",
  color: vars.color.textPrimary,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const workspaceNameCurrent = style({
  color: vars.color.primary,
});

export const workspacePath = style({
  fontSize: "0.6875rem",
  color: vars.color.textMuted,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  marginTop: "0.0625rem",
});

export const workspaceMeta = style({
  flexShrink: 0,
  fontSize: "0.6875rem",
  color: vars.color.textMuted,
});

export const spinner = style({
  width: "0.875rem",
  height: "0.875rem",
  border: `2px solid ${vars.color.borderColor}`,
  borderTopColor: vars.color.primary,
  borderRadius: "50%",
  animation: `${spin} 0.6s linear infinite`,
});

export const errorBanner = style({
  padding: "0.5rem 0.75rem",
  fontSize: "0.75rem",
  color: vars.color.error,
  borderTop: `1px solid ${vars.color.borderColor}`,
});

export const mergeToast = style({
  position: "fixed",
  top: "calc(var(--header-height, 56px) + 0.75rem)",
  right: "1rem",
  zIndex: 2000,
  padding: "0.5rem 0.875rem",
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderLeft: `3px solid ${vars.color.primary}`,
  borderRadius: "0.375rem",
  color: vars.color.textPrimary,
  fontSize: "0.8125rem",
  boxShadow: "0 4px 12px rgba(0, 0, 0, 0.4)",
  animation: `${slideIn} 0.2s ease`,
});
