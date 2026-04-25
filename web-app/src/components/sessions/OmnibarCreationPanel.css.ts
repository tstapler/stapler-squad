import { style } from "@vanilla-extract/css";
import { vars } from "../../styles/theme-contract.css";

export const pathDisplay = style({
  padding: `${vars.space[2]} ${vars.space[4]}`,
  background: vars.color.surfaceSubtle,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  fontFamily: "monospace",
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const radioGroup = style({
  display: "flex",
  gap: vars.space[1],
  flexWrap: "wrap",
});

export const radioBtn = style({
  padding: `${vars.space[1]} ${vars.space[3]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: 500,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
  color: vars.color.textSecondary,
  cursor: "pointer",
  transition: "all 0.1s ease",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.borderHover,
    },
    "&:focus-visible": {
      outline: `2px solid ${vars.color.primary}`,
      outlineOffset: "2px",
    },
  },
});

export const radioBtnActive = style({
  background: vars.color.primary,
  color: vars.color.primaryText,
  borderColor: vars.color.primary,
  selectors: {
    "&:hover": {
      background: vars.color.primaryHover,
      borderColor: vars.color.primaryHover,
    },
  },
});

// ─── Image attachment styles ──────────────────────────────────────────────────

export const attachArea = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space[2],
  flexWrap: "wrap",
  marginTop: vars.space[2],
});

export const attachButton = style({
  padding: `${vars.space[1]} ${vars.space[3]}`,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderColor}`,
  background: "transparent",
  color: vars.color.textSecondary,
  cursor: "pointer",
  transition: "all 0.1s ease",
  selectors: {
    "&:hover:not(:disabled)": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.borderHover,
    },
    "&:disabled": {
      opacity: "0.5",
      cursor: "not-allowed",
    },
  },
});

export const attachLimit = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const attachError = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.error,
});

export const thumbnailRow = style({
  display: "flex",
  gap: vars.space[2],
  flexWrap: "wrap",
  marginTop: vars.space[2],
});

export const thumbnail = style({
  position: "relative",
  width: "64px",
  height: "64px",
  borderRadius: vars.radii.sm,
  overflow: "hidden",
  border: `1px solid ${vars.color.borderSubtle}`,
});

export const thumbnailImg = style({
  width: "100%",
  height: "100%",
  objectFit: "cover",
  display: "block",
});

export const thumbnailRemove = style({
  position: "absolute",
  top: "2px",
  right: "2px",
  width: "18px",
  height: "18px",
  borderRadius: vars.radii.full,
  background: "rgba(0,0,0,0.6)",
  color: "#fff",
  border: "none",
  cursor: "pointer",
  fontSize: "12px",
  lineHeight: "18px",
  textAlign: "center",
  padding: "0",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  selectors: {
    "&:hover": {
      background: vars.color.error,
    },
  },
});

// ─────────────────────────────────────────────────────────────────────────────

export const advancedSection = style({
  overflow: "hidden",
  maxHeight: 0,
  transition: "max-height 0.25s ease-out",
});

export const advancedSectionOpen = style({
  maxHeight: "600px",
  transition: "max-height 0.3s ease-in",
});

// "Create new repository" opt-in notice — appears when the typed path doesn't
// exist on disk. Default state is neutral/informational; once the user opts in
// the card switches to a primary-tinted "active" variant so they can tell at a
// glance that the action is now armed.
export const createRepoNotice = style({
  margin: `${vars.space[2]} ${vars.space[4]} 0`,
  padding: `${vars.space[3]} ${vars.space[4]}`,
  display: "flex",
  flexDirection: "column",
  gap: vars.space[2],
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.surfaceSubtle,
  transition: "background 0.15s ease, border-color 0.15s ease",
});

export const createRepoNoticeActive = style({
  background: vars.color.accentBg,
  borderColor: vars.color.primary,
});

export const createRepoNoticeRow = style({
  display: "flex",
  alignItems: "flex-start",
  gap: vars.space[3],
});

export const createRepoNoticeIcon = style({
  flexShrink: 0,
  width: "1.5rem",
  height: "1.5rem",
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  borderRadius: vars.radii.full,
  background: vars.color.primary,
  color: vars.color.primaryText,
  fontSize: vars.fontSize.sm,
  fontWeight: 700,
  lineHeight: 1,
});

export const createRepoNoticeBody = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[1],
  minWidth: 0,
});

export const createRepoNoticeTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: 600,
  color: vars.color.textPrimary,
});

export const createRepoNoticeDesc = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  lineHeight: 1.4,
});

export const createRepoNoticeBlocked = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.errorText,
  marginTop: vars.space[1],
});

export const createRepoNoticeIconError = style({
  background: vars.color.error,
});
