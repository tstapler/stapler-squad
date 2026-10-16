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

// Neutral by default; red is reserved for "needs you" (notificationBadgeAttention).
// The nav-colored ring separates it from the bell; absolute, so the hit area is unchanged.
export const notificationBadge = style({
  position: "absolute",
  top: "-7px",
  right: "-10px",
  background: vars.color.textMuted,
  // Darkened so white text keeps >= 4.5:1 on the dark theme's lighter muted grey.
  backgroundImage: "linear-gradient(rgba(0, 0, 0, 0.3), rgba(0, 0, 0, 0.3))",
  color: vars.color.primaryText,
  fontSize: "11px",
  fontWeight: "700",
  lineHeight: "1",
  minWidth: "18px",
  height: "18px",
  borderRadius: "9px",
  boxShadow: `0 0 0 2px ${vars.color.background}`,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  padding: "0 4px",
});

export const notificationBadgeAttention = style([
  notificationBadge,
  { background: vars.color.error, backgroundImage: "none" },
]);

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

export const moreSheetScrollable = style({
  overflowY: "auto",
  // Use 100dvh so it doesn't overflow behind iOS Safari's collapsible address bar
  maxHeight: "calc(100dvh - var(--bottom-nav-height, 72px) - env(safe-area-inset-bottom, 0px))",
  WebkitOverflowScrolling: "touch",
});

export const moreSheetSection = style({
  // Semantic grouping container
});

export const moreSheetSectionHeader = style({
  display: "block",
  padding: `${vars.space["3"]} ${vars.space["6"]} ${vars.space["1"]}`,
  fontSize: vars.fontSize.xs,
  fontWeight: vars.fontWeight.bold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  userSelect: "none",
});

export const moreSheetUtilitySection = style({
  borderTop: `1px solid ${vars.color.borderColor}`,
  marginTop: vars.space["1"],
});
