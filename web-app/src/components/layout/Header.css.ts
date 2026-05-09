import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const pulse = keyframes({
  "0%, 100%": { transform: "scale(1)" },
  "50%": { transform: "scale(1.1)" },
});

export const header = style({
  background: vars.color.cardBackground,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  padding: `0 ${vars.space["4"]}`,
  position: "sticky",
  top: 0,
  zIndex: 1100,
  backdropFilter: "blur(8px)",
  backgroundColor: "rgba(26, 26, 26, 0.95)",
  isolation: "isolate",
  // Header always uses a dark backdrop regardless of OS color scheme.
  // Override text tokens so all children get sufficient contrast.
  vars: {
    [vars.color.textPrimary]: "#ededed",
    [vars.color.textSecondary]: "#b4b4b4",
  },

  "@media": {
    // Hide below 900px — BottomNav takes over for mobile + foldable range
    "(max-width: 899px)": {
      display: "none",
    },
    // Collapse to dark surface between 900–1099px (nav items overflow at narrow desktop)
    "(min-width: 900px) and (max-width: 1099px)": {
      backdropFilter: "blur(8px)",
    },
  },
});

export const container = style({
  maxWidth: "1400px",
  margin: "0 auto",
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  height: "var(--header-height)",
  gap: vars.space["8"],

  "@media": {
    "(max-width: 768px)": {
      height: "var(--header-height)",
      gap: vars.space["3"],
      position: "relative",
    },
  },
});

export const branding = style({
  display: "flex",
  alignItems: "baseline",
  gap: vars.space["3"],
});

export const title = style({
  fontSize: vars.fontSize.xl,
  fontWeight: "600",
  margin: 0,
  color: vars.color.textPrimary,

  "@media": {
    "(max-width: 768px)": {
      fontSize: vars.fontSize.lg,
    },
  },
});

export const subtitle = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  fontWeight: "400",

  "@media": {
    "(max-width: 768px)": {
      display: "none",
    },
  },
});

export const nav = style({
  display: "flex",
  gap: vars.space["2"],
  flex: 1,
  justifyContent: "center",

  "@media": {
    // At narrow desktop (900–1099px) and below, collapse to dropdown opened by hamburger
    "(max-width: 1099px)": {
      display: "none",
      position: "absolute",
      top: "100%",
      left: `-${vars.space["4"]}`,
      right: `-${vars.space["4"]}`,
      flexDirection: "column",
      gap: "0",
      backgroundColor: "rgba(20, 20, 20, 0.98)",
      borderBottom: `1px solid ${vars.color.borderColor}`,
      padding: `${vars.space["2"]} 0`,
      zIndex: 1000,
      backdropFilter: "blur(8px)",
      boxShadow: "0 4px 12px rgba(0, 0, 0, 0.4)",
    },
  },
});

export const navOpen = style({
  "@media": {
    "(max-width: 1099px)": {
      display: "flex",
    },
  },
});

export const navLink = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: "500",
  color: vars.color.textSecondary,
  textDecoration: "none",
  transition: "all 0.2s ease",
  position: "relative",

  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
      background: vars.color.hoverBackground,
    },
  },

  "@media": {
    // Full-width row style inside collapsed dropdown (900–1099px)
    "(max-width: 1099px)": {
      padding: `${vars.space["3"]} ${vars.space["4"]}`,
      borderRadius: "0",
      fontSize: vars.fontSize.base,
      borderBottom: `1px solid ${vars.color.borderColor}`,
      minHeight: "44px",
    },
  },
});

export const active = style({
  // Use textPrimary (overridden to #ededed inside the dark header) instead of
  // vars.color.primary (#0070f3) which fails WCAG AA on the dark header backdrop.
  // The blue underline still signals the active state visually.
  color: vars.color.textPrimary,
  fontWeight: 700,

  selectors: {
    "&::after": {
      content: '""',
      position: "absolute",
      bottom: 0,
      left: vars.space["2"],
      right: vars.space["2"],
      height: "2px",
      background: vars.color.primary,
      borderRadius: "2px 2px 0 0",
    },
  },
});

export const actions = style({
  display: "flex",
  gap: vars.space["3"],
  alignItems: "center",
});

export const newSessionButton = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.sm,
  fontWeight: "600",
  color: "white",
  background: vars.color.primary,
  textDecoration: "none",
  transition: "all 0.2s ease",
  border: "none",
  cursor: "pointer",

  selectors: {
    "&:hover": {
      background: vars.color.primaryHover,
      transform: "translateY(-1px)",
      boxShadow: "0 4px 8px rgba(0, 102, 204, 0.3)",
    },
    "&:active": {
      transform: "translateY(0)",
    },
  },

  "@media": {
    "(max-width: 768px)": {
      minWidth: "44px",
      minHeight: "44px",
      padding: vars.space["2"],
      justifyContent: "center",
    },
  },
});

export const newSessionIcon = style({
  fontSize: vars.fontSize.xl,
  fontWeight: "400",
  lineHeight: "1",
});

export const newSessionLabel = style({
  display: "inline",

  "@media": {
    "(max-width: 768px)": {
      display: "none",
    },
  },
});

const iconButtonBase = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "2rem",
  height: "2rem",
  borderRadius: "50%",
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
  color: vars.color.textSecondary,
  cursor: "pointer",
  transition: "all 0.2s ease",

  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
      background: vars.color.hoverBackground,
      borderColor: vars.color.borderHover,
    },
    "&:active": {
      transform: "scale(0.95)",
    },
  },

  "@media": {
    "(max-width: 768px)": {
      width: "44px",
      height: "44px",
    },
  },
});

export const debugButton = style([iconButtonBase, {
  fontSize: vars.fontSize.base,

  selectors: {
    "&:hover": {
      transform: "scale(1.05)",
    },
  },
}]);

export const helpButton = style([iconButtonBase, {
  fontSize: vars.fontSize.base,
  fontWeight: "600",
}]);

export const notificationButton = style([iconButtonBase, {
  fontSize: vars.fontSize.lg,
  position: "relative",

  selectors: {
    "&:hover": {
      transform: "scale(1.05)",
    },
  },
}]);

export const notificationBadge = style({
  position: "absolute",
  top: "-4px",
  right: "-4px",
  minWidth: "1.25rem",
  height: "1.25rem",
  padding: `0 ${vars.space["1"]}`,
  background: vars.color.error,
  color: "white",
  fontSize: vars.fontSize.xs,
  fontWeight: "600",
  borderRadius: "10px",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  border: `2px solid ${vars.color.cardBackground}`,
  animation: `${pulse} 2s ease-in-out infinite`,
});

export const hamburger = style({
  display: "none",
  flexDirection: "column",
  justifyContent: "center",
  alignItems: "center",
  gap: "5px",
  minWidth: "44px",
  minHeight: "44px",
  padding: "10px",
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  cursor: "pointer",
  order: -1,
  flexShrink: 0,

  "@media": {
    // Show hamburger at narrow desktop (900–1099px) where nav overflows
    "(max-width: 1099px)": {
      display: "flex",
    },
  },
});

export const hamburgerLine = style({
  display: "block",
  width: "20px",
  height: "2px",
  backgroundColor: vars.color.textSecondary,
  borderRadius: "2px",
  transition: "transform 0.2s ease, opacity 0.2s ease",
  transformOrigin: "center",
});

export const hamburgerLineOpen1 = style({
  transform: "translateY(7px) rotate(45deg)",
});

export const hamburgerLineOpen2 = style({
  opacity: 0,
});

export const hamburgerLineOpen3 = style({
  transform: "translateY(-7px) rotate(-45deg)",
});
