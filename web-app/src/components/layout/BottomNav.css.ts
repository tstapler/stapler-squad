import { style } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

export const nav = style({
  position: "fixed",
  bottom: 0,
  left: 0,
  right: 0,
  display: "flex",
  background: vars.color.background,
  borderTop: `1px solid ${vars.color.borderColor}`,
  zIndex: zIndex.bottomNav,
  paddingBottom: "max(env(safe-area-inset-bottom, 0px), 8px)",

  // Only show below 900px (mobile + foldable range)
  "@media": {
    "(min-width: 900px)": {
      display: "none",
    },
  },

  // Left-handed mode: reverse item order so primary actions (New, More) move to left thumb zone.
  selectors: {
    "&[data-left-handed]": {
      flexDirection: "row-reverse",
    },
  },
});

export const navItem = style({
  flex: 1,
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  minHeight: "64px",
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  background: "transparent",
  border: "none",
  cursor: "pointer",
  padding: `${vars.space["2"]} ${vars.space["1"]}`,
  transition: "color 0.15s, background 0.15s",
  textDecoration: "none",
  gap: vars.space["1"],

  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
      background: vars.color.hoverBackground,
    },
  },
});

export const navItemActive = style({
  color: vars.color.primary,

  selectors: {
    "&:hover": {
      color: vars.color.primaryHover,
    },
  },
});

export const navItemIcon = style({
  fontSize: vars.fontSize.lg,
  lineHeight: "1",
});

export const navItemLabel = style({
  fontSize: vars.fontSize.xs,
  fontWeight: "500",
});

export const newSessionButton = style({
  flex: 1,
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  minHeight: "64px",
  fontSize: vars.fontSize.xs,
  color: vars.color.primary,
  background: "transparent",
  border: "none",
  cursor: "pointer",
  padding: `${vars.space["2"]} ${vars.space["1"]}`,
  gap: vars.space["1"],

  selectors: {
    "&:hover": {
      color: vars.color.primaryHover,
      background: vars.color.hoverBackground,
    },
    "&:active": {
      color: vars.color.primaryActive,
    },
  },
});

export const newSessionButtonInner = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "32px",
  height: "32px",
  borderRadius: "50%",
  background: vars.color.primary,
  color: vars.color.primaryText,
  fontSize: "22px",
  lineHeight: "1",
  fontWeight: "300",
});

export const notificationButton = style({});

export const notificationIconWrap = style({
  position: "relative",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  fontSize: vars.fontSize.lg,
  lineHeight: "1",
});

export const notificationBadge = style({
  position: "absolute",
  top: "-4px",
  right: "-6px",
  background: vars.color.error,
  color: vars.color.primaryText,
  fontSize: "10px",
  fontWeight: "700",
  lineHeight: "1",
  minWidth: "16px",
  height: "16px",
  borderRadius: "8px",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  padding: "0 3px",
});

// More menu sheet — slides up from just above the bottom nav
export const moreBackdrop = style({
  position: "fixed",
  inset: 0,
  zIndex: zIndex.bottomNavMoreBackdrop,
  background: "transparent",

  "@media": {
    "(min-width: 900px)": {
      display: "none",
    },
  },
});

export const moreSheet = style({
  position: "fixed",
  left: 0,
  right: 0,
  bottom: "var(--bottom-nav-height, 72px)" as string,
  zIndex: zIndex.bottomNavMoreSheet,
  background: vars.color.background,
  borderTop: `1px solid ${vars.color.borderColor}`,
  borderRadius: `${vars.radii.lg} ${vars.radii.lg} 0 0`,
  transform: "translateY(100%)",
  transition: "transform 0.22s ease",
  paddingBottom: "var(--safe-area-bottom, 0px)",

  "@media": {
    "(min-width: 900px)": {
      display: "none",
    },
  },
});

export const moreSheetOpen = style({
  transform: "translateY(0)",
});

export const moreSheetItem = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  padding: `${vars.space["4"]} ${vars.space["6"]}`,
  color: vars.color.textPrimary,
  textDecoration: "none",
  fontSize: vars.fontSize.base,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  transition: "background 0.12s",

  selectors: {
    "&:last-child": {
      borderBottom: "none",
    },
    "&:hover": {
      background: vars.color.hoverBackground,
    },
  },
});

export const moreSheetItemActive = style({
  color: vars.color.primary,
  fontWeight: "600",
});

export const moreSheetItemIcon = style({
  fontSize: vars.fontSize.lg,
  width: "24px",
  textAlign: "center",
  flexShrink: 0,
});
