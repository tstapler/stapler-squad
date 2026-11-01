import { style } from "@vanilla-extract/css";
import { vars, zIndex, breakpoints } from "@/styles/theme.css";

export const controls = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  marginBottom: vars.space["2"],
});

// 44px min touch target (mobile UX convention — see SessionsSection.css.ts's sessionSteerBtn).
export const jumpButton = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["2"],
  minHeight: 44,
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  cursor: "pointer",
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  ":hover": {
    borderColor: vars.color.primary,
    color: vars.color.primary,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});

export const roleMenuTrigger = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  flexShrink: 0,
  width: 44,
  height: 44,
  padding: 0,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  background: vars.color.cardBackground,
  color: vars.color.textSecondary,
  cursor: "pointer",
  ":hover": {
    background: vars.color.hoverBackground,
    color: vars.color.textPrimary,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});

export const roleMenu = style({
  position: "fixed",
  minWidth: "200px",
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  boxShadow: vars.shadow.md,
  zIndex: zIndex.dropdown,
  padding: vars.space["1"],
  display: "flex",
  flexDirection: "column",
});

export const roleMenuItem = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "flex-start",
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  border: "none",
  borderRadius: vars.radii.md,
  background: "transparent",
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
  textAlign: "left",
  cursor: "pointer",
  width: "100%",
  ":hover": {
    background: vars.color.hoverBackground,
  },
});

export const roleMenuItemRole = style({
  fontWeight: vars.fontWeight.medium,
});

export const roleMenuItemDate = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

// Loading/error state shown while the mobile modal's session is still being fetched.
export const modalStatus = style({
  position: "fixed",
  inset: 0,
  zIndex: zIndex.modal,
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  gap: vars.space["3"],
  background: vars.color.background,
  color: vars.color.textPrimary,
});

export const panel = style({
  display: "flex",
  flexDirection: "column",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  overflow: "hidden",
  marginBottom: vars.space["3"],
  // Scopes SessionDetail's own `calc(var(--viewport-height) - var(--header-height))`
  // container height (SessionDetail.css.ts) down to a fixed in-page panel size instead
  // of the full-viewport height it's designed for everywhere else (main page, peek
  // modal) — these two custom properties cascade to it unchanged, no code change
  // needed there.
  vars: {
    "--viewport-height": "min(60vh, 520px)",
    "--header-height": "0px",
  } as Record<string, string>,
  "@media": {
    [`screen and (max-width: ${breakpoints.sm})`]: {
      vars: {
        "--viewport-height": "min(70vh, 420px)",
      } as Record<string, string>,
    },
  },
});

export const panelLoading = style({
  padding: vars.space["4"],
  textAlign: "center",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});

export const panelError = style({
  padding: vars.space["4"],
  textAlign: "center",
  color: vars.color.error,
  fontSize: vars.fontSize.sm,
});
