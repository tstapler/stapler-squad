import { style, globalStyle } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

const bar = {
  display: "flex",
  alignItems: "center",
  flexShrink: 0,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
} as const;

export const topBar = style({ ...bar, minHeight: "44px", borderBottom: `1px solid ${vars.color.borderColor}`, position: "relative" });

export const bottomBar = style({
  ...bar,
  minHeight: "48px",
  borderTop: `1px solid ${vars.color.borderColor}`,
  paddingBottom: "env(safe-area-inset-bottom, 0px)",
  position: "relative",
});

export const barButton = style({
  minWidth: "44px",
  minHeight: "44px",
  border: "none",
  background: "transparent",
  color: "inherit",
  font: "inherit",
  fontSize: "13px",
  cursor: "pointer",
  display: "inline-flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  gap: "1px",
  padding: "0 4px",
  ":focus-visible": { outline: `2px solid ${vars.color.inputFocusBorder}`, outlineOffset: "-2px" },
  selectors: {
    "&[aria-disabled='true']": { opacity: 0.4, cursor: "default" },
    "&[aria-current='true']": { color: vars.color.primary },
  },
});

export const bottomItem = style({ flex: 1, minWidth: 0 });
export const split = style({ flex: 1, display: "flex", minWidth: 0 });
export const barLabel = style({ fontSize: "11px" });

export const title = style({
  flex: 1,
  minWidth: 0,
  display: "flex",
  alignItems: "center",
  gap: "6px",
  padding: "0 4px",
  fontSize: "15px",
  fontWeight: 600,
});

// Keep the tail of a long path: render right-to-left so the ellipsis lands on the left.
export const fileName = style({ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", direction: "rtl", textAlign: "left", minWidth: 0 });
export const dirtyDot = style({ width: "8px", height: "8px", borderRadius: "50%", background: vars.color.warningText, flexShrink: 0 });

export const popover = style({
  position: "absolute",
  zIndex: zIndex.dropdown,
  minWidth: "200px",
  display: "flex",
  flexDirection: "column",
  padding: "4px",
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.md,
  boxShadow: "0 4px 16px rgba(0,0,0,0.25)",
  selectors: { "&[data-from='top']": { top: "100%", right: "4px" }, "&[data-from='bottom']": { bottom: "100%", right: "4px" } },
});
export const popoverItem = style({
  minHeight: "44px",
  textAlign: "left",
  padding: "0 12px",
  border: "none",
  background: "transparent",
  color: vars.color.textPrimary,
  font: "inherit",
  cursor: "pointer",
  borderRadius: vars.radii.sm,
  ":hover": { background: vars.color.hoverBackground },
  selectors: { "&[aria-disabled='true']": { opacity: 0.45, cursor: "default" } },
});

// Right edge, above the bottom bar. Column-reverse puts the first child lowest, so the chip registers first and the FAB (2.4.2b) stacks above it.
export const floatingStack = style({
  position: "absolute",
  right: "8px",
  bottom: "8px",
  zIndex: zIndex.raised,
  display: "flex",
  flexDirection: "column-reverse",
  alignItems: "flex-end",
  gap: "8px",
  pointerEvents: "none",
});
globalStyle(`${floatingStack} > *`, { pointerEvents: "auto" });
export const stackHost = style({ position: "relative", flexShrink: 0, height: 0 });

export const toast = style({
  position: "absolute",
  left: "8px",
  right: "8px",
  bottom: "8px",
  zIndex: zIndex.raised,
  display: "flex",
  alignItems: "center",
  gap: "8px",
  padding: "6px 10px",
  fontSize: "13px",
  background: vars.color.warningBg,
  color: vars.color.warningText,
  border: `1px solid ${vars.color.warning}`,
  borderRadius: vars.radii.md,
});
export const toastButton = style({
  minWidth: "44px",
  minHeight: "44px",
  border: "none",
  background: "transparent",
  color: "inherit",
  font: "inherit",
  textDecoration: "underline",
  cursor: "pointer",
});
export const srOnly = style({ position: "absolute", width: "1px", height: "1px", overflow: "hidden", clip: "rect(0 0 0 0)", whiteSpace: "nowrap" });
