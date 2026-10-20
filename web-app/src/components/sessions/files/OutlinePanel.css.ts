import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// Dropdown on desktop, bottom sheet on narrow screens.
export const panel = style({
  position: "absolute",
  top: "100%",
  right: "8px",
  zIndex: 30,
  width: "320px",
  maxHeight: "60vh",
  display: "flex",
  flexDirection: "column",
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.md,
  boxShadow: "0 4px 16px rgba(0,0,0,0.25)",
  "@media": {
    "(max-width: 767px)": {
      position: "fixed",
      top: "auto",
      right: 0,
      bottom: "var(--keyboard-height, 0px)",
      left: 0,
      width: "auto",
      maxHeight: "calc(var(--viewport-height, 100dvh) * 0.7)",
      borderRadius: `${vars.radii.lg} ${vars.radii.lg} 0 0`,
    },
  },
});

export const header = style({ display: "flex", alignItems: "center", gap: "8px", padding: "8px" });
export const heading = style({ fontWeight: 600, fontSize: "14px", flexShrink: 0 });
export const filter = style({
  flex: 1,
  minWidth: 0,
  padding: "4px 8px",
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  // 16px stops iOS Safari zooming the page on focus.
  "@media": { "(pointer: coarse)": { minHeight: "44px", fontSize: "16px" } },
});
export const list = style({ overflowY: "auto", margin: 0, padding: "0 4px 8px", listStyle: "none" });
export const option = style({
  display: "flex",
  gap: "8px",
  alignItems: "baseline",
  padding: "6px 8px",
  cursor: "pointer",
  borderRadius: vars.radii.sm,
  color: vars.color.textPrimary,
  selectors: { "&[aria-selected='true']": { background: vars.color.accentBg, outline: `1px solid ${vars.color.primary}` } },
  "@media": { "(pointer: coarse)": { minHeight: "44px", alignItems: "center" } },
});
export const symbolKind = style({ fontSize: "12px", color: vars.color.textMuted });
export const symbolLine = style({ marginLeft: "auto", fontVariantNumeric: "tabular-nums", color: vars.color.textMuted });
export const message = style({ padding: "12px", color: vars.color.textSecondary, fontSize: "13px" });
export const actions = style({ display: "flex", gap: "8px", padding: "0 12px 12px" });
export const textButton = style({
  border: `1px solid ${vars.color.borderMuted}`,
  background: "transparent",
  color: vars.color.textPrimary,
  borderRadius: vars.radii.sm,
  padding: "4px 10px",
  cursor: "pointer",
  "@media": { "(pointer: coarse)": { minHeight: "44px", minWidth: "44px" } },
});
