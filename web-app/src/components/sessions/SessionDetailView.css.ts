import { style } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

/** Used for diff added lines and approved review counts. */
export const diffAdded = style({
  color: vars.color.success,
});

/** Full-area overlay displayed when the current session is paused.
 *  Sits above the terminal pool (which stays mounted for keep-alive).
 *  Uses position:absolute inside the existing position:relative terminal container.
 *  No createPortal needed — the overlay is intentionally local to the terminal area.
 */
export const pausedOverlay = style({
  position: "absolute",
  inset: 0,
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  gap: vars.space["4"],
  background: vars.color.overlayBackground,
  zIndex: zIndex.raised,
  borderRadius: vars.radii.md,
});

export const pausedOverlayIcon = style({
  fontSize: vars.fontSize.xl,
  color: vars.color.warningText,
});

export const pausedOverlayTitle = style({
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  margin: 0,
});

export const pausedOverlayReason = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  margin: 0,
  textAlign: "center",
  maxWidth: "360px",
});

export const pausedOverlayButton = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["6"]}`,
  background: vars.color.primary,
  color: vars.color.primaryText,
  borderRadius: vars.radii.md,
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.semibold,
  border: "none",
  cursor: "pointer",
  marginTop: vars.space["2"],
  ":hover": {
    background: vars.color.primaryHover,
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: "2px",
  },
});
