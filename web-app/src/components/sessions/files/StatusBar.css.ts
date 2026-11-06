import { keyframes, style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const bar = style({
  display: "flex",
  alignItems: "center",
  gap: "12px",
  padding: "2px 10px",
  minHeight: "26px",
  fontSize: "12px",
  borderTop: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  color: vars.color.textSecondary,
  "@media": { "(max-width: 767px)": { overflowX: "auto", whiteSpace: "nowrap" } },
});

export const segment = style({ fontVariantNumeric: "tabular-nums", flexShrink: 0 });

export const chip = style({
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: vars.radii.sm,
  background: "transparent",
  color: "inherit",
  font: "inherit",
  padding: "0 6px",
  cursor: "pointer",
  flexShrink: 0,
  "@media": { "(pointer: coarse)": { minHeight: "44px" } },
});

export const popover = style({
  position: "absolute",
  bottom: "100%",
  left: "8px",
  maxWidth: "320px",
  padding: "8px 10px",
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.md,
  color: vars.color.textPrimary,
  boxShadow: "0 4px 16px rgba(0,0,0,0.25)",
  whiteSpace: "normal",
});

export const region = style({ marginLeft: "auto", display: "flex", alignItems: "center", gap: "6px", minWidth: 0 });
export const errorText = style({ color: vars.color.errorText });
export const dirtyDot = style({ fontWeight: 600 });

const fade = keyframes({ from: { opacity: 0 }, to: { opacity: 1 } });

export const toastBox = style({
  position: "fixed",
  left: "50%",
  transform: "translateX(-50%)",
  bottom: "calc(112px + env(safe-area-inset-bottom, 0px))",
  zIndex: 40,
  display: "none",
  alignItems: "center",
  gap: "8px",
  padding: "8px 12px",
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.md,
  color: vars.color.textPrimary,
  boxShadow: "0 4px 16px rgba(0,0,0,0.25)",
  selectors: { "&[data-motion='fade']": { animation: `${fade} 150ms ease-out` } },
  "@media": { "(max-width: 767px)": { display: "flex" } },
});

export const toastButton = style({
  border: "none",
  background: "transparent",
  color: "inherit",
  font: "inherit",
  textDecoration: "underline",
  cursor: "pointer",
  minWidth: "44px",
  minHeight: "44px",
});
