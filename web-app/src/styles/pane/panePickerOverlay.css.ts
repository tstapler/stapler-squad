import { style } from "@vanilla-extract/css";
import { keyframes } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

const fadeIn = keyframes({
  from: { opacity: 0 },
  to: { opacity: 1 },
});

const slideUp = keyframes({
  from: { opacity: 0, transform: "translateY(8px)" },
  to: { opacity: 1, transform: "translateY(0)" },
});

export const pickerOverlay = style({
  position: "absolute",
  inset: 0,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  background: "rgba(0,0,0,0.55)",
  zIndex: 20,
  cursor: "pointer",
  animation: `${fadeIn} 80ms ease`,
});

export const pickerActionBar = style({
  position: "absolute",
  bottom: 0,
  left: 0,
  right: 0,
  zIndex: 25,
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  background: "rgba(10, 10, 10, 0.88)",
  backdropFilter: "blur(12px)",
  borderTop: "1px solid rgba(255, 255, 255, 0.08)",
  animation: `${slideUp} 120ms ease`,
});

export const pickerActionButton = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["1"],
  padding: `5px ${vars.space["3"]}`,
  borderRadius: vars.radii.md,
  border: "1px solid rgba(255, 255, 255, 0.15)",
  background: "rgba(255, 255, 255, 0.06)",
  color: "rgba(255, 255, 255, 0.9)",
  fontSize: vars.fontSize.sm,
  fontFamily: vars.font.sans,
  cursor: "pointer",
  transition: "background 100ms, border-color 100ms",
  whiteSpace: "nowrap",
  selectors: {
    "&:hover": {
      background: "rgba(255, 255, 255, 0.14)",
      borderColor: "rgba(255, 255, 255, 0.3)",
    },
  },
});

export const pickerActionKbd = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minWidth: "18px",
  height: "18px",
  padding: "0 3px",
  borderRadius: "3px",
  background: "rgba(255, 255, 255, 0.12)",
  border: "1px solid rgba(255, 255, 255, 0.2)",
  fontSize: "11px",
  fontFamily: vars.font.mono,
  fontWeight: vars.fontWeight.bold,
  color: "rgba(255, 255, 255, 0.7)",
  lineHeight: 1,
  marginLeft: vars.space["1"],
});

// Mobile-only bottom sheet shown in place of the desktop picker overlay.
// z-indices are sourced from the central zIndex map in theme-contract.css.ts;
// update that map (not these files) when adding new stacking layers.
export const mobilePickerBackdrop = style({
  position: "fixed",
  inset: 0,
  zIndex: zIndex.mobilePickerBackdrop,
  background: "rgba(0,0,0,0.5)",
});

export const mobilePickerSheet = style({
  position: "fixed",
  left: 0,
  right: 0,
  bottom: "calc(var(--bottom-nav-height, 64px) + var(--mobile-pane-tab-strip-height, 0px))",
  zIndex: zIndex.mobilePickerSheet,
  background: vars.color.background,
  borderTop: `1px solid ${vars.color.borderColor}`,
  borderRadius: `${vars.radii.lg} ${vars.radii.lg} 0 0`,
  padding: `${vars.space["4"]} 0`,
  animation: `${slideUp} 180ms ease`,
});

export const mobilePickerSheetTitle = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.bold,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
  padding: `0 ${vars.space["4"]} ${vars.space["3"]}`,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  marginBottom: vars.space["2"],
});

export const mobilePickerPaneItem = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  width: "100%",
  padding: `${vars.space["3"]} ${vars.space["4"]}`,
  background: "transparent",
  border: "none",
  textAlign: "left",
  cursor: "pointer",
  fontSize: vars.fontSize.base,
  color: vars.color.textPrimary,
  transition: "background 100ms",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
    },
    "&:active": {
      background: vars.color.hoverBackground,
    },
  },
});

export const mobilePickerPaneLabel = style({
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  padding: `1px ${vars.space["1"]}`,
  background: vars.color.hoverBackground,
  borderRadius: vars.radii.sm,
  flexShrink: 0,
});

export const mobilePickerCancelButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "100%",
  padding: `${vars.space["3"]} ${vars.space["4"]}`,
  background: "transparent",
  border: "none",
  cursor: "pointer",
  fontSize: vars.fontSize.base,
  color: vars.color.textMuted,
  borderTop: `1px solid ${vars.color.borderColor}`,
  marginTop: vars.space["2"],
  transition: "background 100ms",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
    },
  },
});

export const pickerLabel = style({
  width: "56px",
  height: "56px",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  borderRadius: vars.radii.lg,
  background: vars.color.primary,
  color: vars.color.primaryText,
  fontSize: "28px",
  fontWeight: vars.fontWeight.bold,
  fontFamily: vars.font.mono,
  boxShadow: vars.shadow.lg,
  pointerEvents: "none",
  userSelect: "none",
});
