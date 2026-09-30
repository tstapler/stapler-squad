import { style } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

// 44x44px meets the minimum touch target (AC9/AC27 -- non-drag fallback must be directly
// tappable on mobile, no hover-reveal) even though the icon itself is small; padding does the
// sizing work rather than a large icon.
export const menuTrigger = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  flexShrink: 0,
  width: "44px",
  height: "44px",
  padding: 0,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  cursor: "pointer",
  transition: "background 0.15s ease",
  selectors: {
    "&:hover": { background: vars.color.hoverBackground },
  },
});

// Reuses the shared dropdown z-index slot (theme-contract.css.ts) -- no new slot needed.
export const menu = style({
  position: "fixed",
  minWidth: "160px",
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  boxShadow: vars.shadow.md,
  zIndex: zIndex.dropdown,
  padding: vars.space["1"],
  display: "flex",
  flexDirection: "column",
});

export const menuItem = style({
  display: "flex",
  alignItems: "center",
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  border: "none",
  borderRadius: vars.radii.md,
  background: "transparent",
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
  textAlign: "left",
  cursor: "pointer",
  width: "100%",
  transition: "background 0.15s ease",
  selectors: {
    "&:hover": { background: vars.color.hoverBackground },
  },
});

export const menuEmpty = style({
  margin: 0,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});
