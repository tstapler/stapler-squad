import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const page = style({
  minHeight: "var(--viewport-height, 100dvh)",
  display: "flex",
  flexDirection: "column",
  backgroundColor: vars.color.background,
  color: vars.color.textPrimary,
});

export const main = style({
  flex: 1,
  padding: "2rem",
  maxWidth: "1400px",
  width: "100%",
  margin: "0 auto",
  "@media": {
    "screen and (max-width: 900px)": {
      padding: "1rem",
      // Ensure content clears the bottom nav on mobile
      paddingBottom: "calc(var(--bottom-nav-height, 56px) + max(env(safe-area-inset-bottom, 0px), 0px) + 1rem)",
    },
  },
});

export const loading = style({
  padding: "2rem",
  textAlign: "center",
  color: vars.color.textMuted,
});

export const error = style({
  color: "#ff4444",
  padding: "1rem",
  backgroundColor: "rgba(255, 68, 68, 0.1)",
  borderRadius: "8px",
  margin: "1rem 0",
});

export const modal = style({
  position: "fixed",
  top: 0,
  left: 0,
  right: 0,
  bottom: 0,
  background: "rgba(0, 0, 0, 0.7)",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  zIndex: 1000,
  padding: "1rem",
  paddingTop: "calc(var(--header-height) + 0.5rem)",
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "0",
      paddingTop: "var(--header-height)",
      // Stretch instead of center — modal fills from header down, no gap above terminal
      alignItems: "stretch",
      justifyContent: "flex-start",
    },
  },
});

export const modalContent = style({
  background: vars.color.cardBackground,
  borderRadius: "12px",
  maxWidth: "1200px",
  width: "100%",
  height: "60vh",
  maxHeight: "60vh",
  overflow: "hidden",
  display: "flex",
  flexDirection: "column",
  boxShadow: "0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04)",
  "@media": {
    "screen and (max-width: 768px)": {
      maxHeight: "calc(var(--viewport-height, 100dvh) - var(--header-height))",
      height: "calc(var(--viewport-height, 100dvh) - var(--header-height))",
      borderRadius: "0",
      maxWidth: "100%",
    },
  },
});

export const modalContentFullscreen = style({
  maxWidth: "98vw",
  width: "98vw",
  maxHeight: "calc(100dvh - var(--header-height) - 1.5rem)",
  height: "calc(100dvh - var(--header-height) - 1.5rem)",
  borderRadius: "8px",
  "@media": {
    "screen and (max-width: 768px)": {
      maxHeight: "calc(var(--viewport-height, 100dvh) - var(--header-height))",
      height: "calc(var(--viewport-height, 100dvh) - var(--header-height))",
      // aspect-ratio constraint defeats portrait layout on mobile; disable it
      aspectRatio: "auto",
    },
  },
});

export const modalHeader = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  padding: "1.5rem",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "1rem",
    },
  },
});

export const closeButton = style({
  background: "transparent",
  border: "none",
  fontSize: "1.5rem",
  cursor: "pointer",
  color: vars.color.textMuted,
  padding: "0.5rem",
  lineHeight: "1",
  transition: "color 0.2s",
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
    },
  },
});

export const modalBody = style({
  padding: "1.5rem",
  overflowY: "auto",
  "@media": {
    "screen and (max-width: 768px)": {
      padding: "1rem",
    },
  },
});

export const placeholder = style({
  marginTop: "2rem",
  padding: "2rem",
  textAlign: "center",
  color: vars.color.textMuted,
  fontStyle: "italic",
});

export const helpButton = style({
  position: "fixed",
  bottom: "2rem",
  right: "2rem",
  width: "3rem",
  height: "3rem",
  borderRadius: "50%",
  background: vars.color.primary,
  color: "white",
  border: "none",
  fontSize: "1.5rem",
  fontWeight: 600,
  cursor: "pointer",
  boxShadow: "0 4px 6px -1px rgba(0, 0, 0, 0.1), 0 2px 4px -1px rgba(0, 0, 0, 0.06)",
  transition: "transform 0.2s, box-shadow 0.2s",
  zIndex: 100,
  selectors: {
    "&:hover": {
      transform: "scale(1.1)",
      boxShadow: "0 10px 15px -3px rgba(0, 0, 0, 0.1), 0 4px 6px -2px rgba(0, 0, 0, 0.05)",
    },
    "&:active": {
      transform: "scale(1.05)",
    },
  },
  "@media": {
    "screen and (max-width: 768px)": {
      bottom: "1rem",
      right: "1rem",
      width: "2.5rem",
      height: "2.5rem",
      fontSize: "1.25rem",
    },
  },
});
