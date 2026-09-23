import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

/**
 * Shared collapsible-section chrome for the Unfinished tab's queue sections
 * (BacklogQueueSection, GitHubPRsSection).
 */
export const section = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const sectionHeader = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  cursor: "pointer",
  userSelect: "none",
  outline: "none",
  ":hover": {
    background: vars.color.hoverBackground,
  },
  ":focus-visible": {
    boxShadow: `0 0 0 2px ${vars.color.inputFocusBorder}`,
  },
});

export const chevron = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  transition: "transform 0.15s",
  display: "inline-block",
});

export const chevronExpanded = style({
  transform: "rotate(90deg)",
});

export const sectionTitle = style({
  fontWeight: 600,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
});

/** Base badge style — compose with `style([badgeBase, {...}])` for per-site tweaks. */
export const badgeBase = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  background: vars.color.surfaceSubtle,
  borderRadius: vars.radii.full,
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
});
