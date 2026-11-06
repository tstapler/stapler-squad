import { style, keyframes } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

const slideUp = keyframes({ from: { transform: "translateY(100%)" }, to: { transform: "translateY(0)" } });

export const backdrop = style({ position: "fixed", inset: 0, zIndex: zIndex.dialog, background: "rgba(0,0,0,0.45)", display: "flex", alignItems: "flex-end" });

export const sheet = style({
  width: "100%",
  maxHeight: "80dvh",
  display: "flex",
  flexDirection: "column",
  background: vars.color.modalBackground,
  borderTop: `1px solid ${vars.color.modalBorder}`,
  borderRadius: `${vars.radii.md} ${vars.radii.md} 0 0`,
  paddingBottom: "env(safe-area-inset-bottom, 0px)",
  animation: `${slideUp} 180ms ease-out`,
  "@media": { "(prefers-reduced-motion: reduce)": { animation: "none", transition: "none" } },
});

export const sheetHeader = style({ display: "flex", alignItems: "center", justifyContent: "space-between", padding: "0 8px 0 16px", borderBottom: `1px solid ${vars.color.borderColor}` });
export const heading = style({ margin: 0, fontSize: "15px", fontWeight: 600, color: vars.color.textPrimary });
export const list = style({ listStyle: "none", margin: 0, padding: 0, overflowY: "auto" });
export const row = style({ position: "relative", display: "flex", alignItems: "stretch", borderBottom: `1px solid ${vars.color.borderSubtle}` });
export const rowActive = style({ background: vars.color.hoverBackground });
export const rowMain = style({
  flex: 1,
  minWidth: 0,
  display: "flex",
  flexDirection: "column",
  justifyContent: "center",
  textAlign: "left",
  gap: "2px",
  padding: "6px 12px",
  border: "none",
  background: "transparent",
  color: vars.color.textPrimary,
  font: "inherit",
  cursor: "pointer",
  touchAction: "pan-y",
});
export const rowTitle = style({ display: "flex", alignItems: "center", gap: "6px", fontSize: "15px", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" });
export const rowPath = style({ fontSize: "12px", color: vars.color.textMuted, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", direction: "rtl", textAlign: "left" });
export const stateWord = style({ fontSize: "12px", color: vars.color.textSecondary });
export const dirtyMark = style({ width: "8px", height: "8px", borderRadius: "50%", background: vars.color.warningText, flexShrink: 0 });

export const iconButton = style({
  width: "44px",
  minHeight: "44px",
  border: "none",
  background: "transparent",
  color: vars.color.textSecondary,
  cursor: "pointer",
  fontSize: "16px",
  selectors: { "&[data-highlight='true']": { outline: `2px solid ${vars.color.primary}`, outlineOffset: "-4px", color: vars.color.textPrimary } },
  ":focus-visible": { outline: `2px solid ${vars.color.inputFocusBorder}`, outlineOffset: "-2px" },
});

export const notice = style({ padding: "10px 16px", fontSize: "13px", background: vars.color.warningBg, color: vars.color.warningText, borderBottom: `1px solid ${vars.color.warning}` });
export const empty = style({ padding: "24px 16px", display: "flex", flexDirection: "column", gap: "12px", alignItems: "flex-start", color: vars.color.textSecondary });
export const textButton = style({ minHeight: "44px", padding: "0 16px", border: `1px solid ${vars.color.borderMuted}`, borderRadius: vars.radii.sm, background: "transparent", color: vars.color.textPrimary, font: "inherit", cursor: "pointer" });

export const rowMenu = style({
  position: "absolute",
  right: "48px",
  top: "4px",
  zIndex: zIndex.raised,
  display: "flex",
  flexDirection: "column",
  minWidth: "180px",
  padding: "4px",
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.md,
  boxShadow: "0 4px 16px rgba(0,0,0,0.25)",
});
export const menuItem = style({
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
  selectors: { "&[disabled]": { opacity: 0.45, cursor: "default" } },
});
