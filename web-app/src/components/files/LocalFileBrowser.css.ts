import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme-contract.css";

export const page = style({
  display: "flex",
  flexDirection: "column",
  height: "100%",
  overflow: "hidden",
});

export const pathBar = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: vars.space["3"],
  borderBottom: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  flexShrink: 0,
});

export const pathForm = style({
  display: "flex",
  gap: vars.space["2"],
  alignItems: "center",
});

export const pathInput = style({
  flex: 1,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  padding: "6px 10px",
  fontSize: vars.fontSize.sm,
  outline: "none",
  fontFamily: vars.font.mono,
  selectors: {
    "&:focus": {
      borderColor: vars.color.inputFocusBorder,
    },
  },
});

export const pathButton = style({
  background: vars.color.primary,
  color: vars.color.primaryText,
  border: "none",
  borderRadius: vars.radii.sm,
  padding: "6px 14px",
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  flexShrink: 0,
  selectors: {
    "&:hover": {
      background: vars.color.primaryHover,
    },
    "&:active": {
      background: vars.color.primaryActive,
    },
  },
});

export const upButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  background: "none",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  color: vars.color.textMuted,
  cursor: "pointer",
  padding: "5px 8px",
  flexShrink: 0,
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
    "&:disabled": {
      opacity: 0.4,
      cursor: "not-allowed",
    },
  },
});

export const breadcrumbs = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: 2,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const crumbGroup = style({
  display: "flex",
  alignItems: "center",
  gap: 2,
});

export const breadcrumbSep = style({
  color: vars.color.textDisabled,
  display: "flex",
  alignItems: "center",
});

export const breadcrumbLink = style({
  background: "none",
  border: "none",
  color: vars.color.textMuted,
  cursor: "pointer",
  padding: "2px 4px",
  borderRadius: vars.radii.sm,
  fontSize: "inherit",
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
      background: vars.color.hoverBackground,
    },
  },
});

export const content = style({
  display: "flex",
  flex: 1,
  overflow: "hidden",
});

export const sidebar = style({
  width: 280,
  borderRight: `1px solid ${vars.color.borderColor}`,
  overflowY: "auto",
  flexShrink: 0,
  display: "flex",
  flexDirection: "column",
});

export const sidebarEmpty = style({
  padding: vars.space["4"],
  color: vars.color.textMuted,
  fontStyle: "italic",
  textAlign: "center",
  fontSize: vars.fontSize.sm,
});

export const fileEntry = recipe({
  base: {
    width: "100%",
    display: "flex",
    alignItems: "center",
    gap: 8,
    padding: "6px 12px",
    background: "none",
    border: "none",
    cursor: "pointer",
    textAlign: "left",
    selectors: {
      "&:hover": {
        background: vars.color.hoverBackground,
      },
    },
  },
  variants: {
    active: {
      true: {
        background: vars.color.accentBg,
        selectors: {
          "&:hover": {
            background: vars.color.accentHover,
          },
        },
      },
      false: {},
    },
  },
  defaultVariants: {
    active: false,
  },
});

export const fileIcon = style({
  color: vars.color.textMuted,
  flexShrink: 0,
  display: "flex",
  alignItems: "center",
});

export const fileName = style({
  flex: 1,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
});

export const fileSize = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.xs,
  flexShrink: 0,
});

export const viewer = style({
  flex: 1,
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
});

export const viewerEmpty = style({
  flex: 1,
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  gap: vars.space["2"],
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});

export const viewerWrapper = style({
  flex: 1,
  display: "flex",
  flexDirection: "column",
  overflow: "hidden",
});

export const viewerToolbar = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: "4px 12px",
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  background: vars.color.cardBackground,
  flexShrink: 0,
});

export const viewerLabel = style({
  flex: 1,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const externalButton = style({
  display: "flex",
  alignItems: "center",
  gap: 4,
  background: "none",
  border: `1px solid ${vars.color.borderColor}`,
  color: vars.color.textSecondary,
  borderRadius: vars.radii.sm,
  padding: "3px 8px",
  cursor: "pointer",
  fontSize: vars.fontSize.xs,
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
});

export const viewerFrame = style({
  flex: 1,
  width: "100%",
  border: "none",
  background: vars.color.background,
});

export const viewerImage = style({
  maxWidth: "100%",
  maxHeight: "100%",
  objectFit: "contain",
  margin: "auto",
  display: "block",
  padding: vars.space["4"],
});

export const viewerVideo = style({
  maxWidth: "100%",
  maxHeight: "100%",
  margin: "auto",
  display: "block",
});

export const viewerText = style({
  flex: 1,
  overflow: "auto",
  padding: vars.space["4"],
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  background: vars.color.background,
  margin: 0,
  whiteSpace: "pre",
});

export const viewerHint = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});
