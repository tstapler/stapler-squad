import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const slideIn = keyframes({
  from: { transform: "translateX(100%)" },
  to: { transform: "translateX(0)" },
});

/** Fixed right-side panel anchored to viewport — non-modal (no backdrop) */
export const drawer = style({
  position: "fixed",
  top: "var(--header-height, 60px)",
  right: 0,
  bottom: 0,
  width: 360,
  maxWidth: "100vw",
  background: vars.color.background,
  borderLeft: `1px solid ${vars.color.borderColor}`,
  boxShadow: "-4px 0 16px rgba(0,0,0,0.12)",
  zIndex: 900,
  display: "flex",
  flexDirection: "column",
  animation: `${slideIn} 200ms ease-out`,
  overflowY: "auto",
});

export const header = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: "12px 16px",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  position: "sticky",
  top: 0,
  zIndex: 1,
});

export const title = style({
  fontSize: "0.9rem",
  fontWeight: 700,
  color: vars.color.textPrimary,
  margin: 0,
});

export const closeButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  fontSize: "1.25rem",
  lineHeight: 1,
  color: vars.color.textSecondary,
  padding: "4px 6px",
  borderRadius: 4,
  ":hover": { color: vars.color.textPrimary },
  ":focus-visible": {
    outline: `2px solid ${vars.color.primary}`,
    outlineOffset: 2,
  },
});

export const list = style({
  flex: 1,
  padding: "8px 12px",
  display: "flex",
  flexDirection: "column",
  gap: 8,
});

export const empty = style({
  padding: "32px 16px",
  textAlign: "center",
  color: vars.color.textSecondary,
  fontSize: "0.85rem",
  fontStyle: "italic",
});

/** Visually-hidden live region for expiry announcements */
export const announcer = style({
  position: "absolute",
  width: 1,
  height: 1,
  padding: 0,
  margin: -1,
  overflow: "hidden",
  clip: "rect(0,0,0,0)",
  whiteSpace: "nowrap",
  border: 0,
});
