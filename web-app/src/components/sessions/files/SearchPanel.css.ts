import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const panel = style({ display: "flex", flexDirection: "column", flex: 1, minHeight: 0, minWidth: 0 });

export const header = style({ display: "flex", gap: "2px", padding: "4px 6px", borderBottom: `1px solid ${vars.color.borderColor}` });
export const headerTab = style({
  border: "none",
  background: "transparent",
  color: vars.color.textSecondary,
  font: "inherit",
  fontSize: "13px",
  padding: "4px 10px",
  cursor: "pointer",
  borderBottom: "2px solid transparent",
  selectors: { "&[aria-selected='true']": { color: vars.color.textPrimary, borderBottomColor: vars.color.primary } },
  "@media": { "(pointer: coarse)": { minHeight: "44px" } },
});

export const form = style({ display: "flex", flexDirection: "column", gap: "6px", padding: "8px" });
export const queryRow = style({ display: "flex", gap: "4px", alignItems: "center" });
export const queryInput = style({
  flex: 1,
  minWidth: 0,
  padding: "5px 8px",
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  font: "inherit",
  fontSize: "13px",
  "@media": { "(pointer: coarse)": { minHeight: "44px" } },
});
export const toggleRow = style({ display: "flex", gap: "4px", alignItems: "center", flexWrap: "wrap" });
export const toggle = style({
  border: `1px solid ${vars.color.borderMuted}`,
  borderRadius: vars.radii.sm,
  background: "transparent",
  color: vars.color.textPrimary,
  font: "inherit",
  fontSize: "12px",
  fontFamily: vars.font.mono,
  minWidth: "28px",
  padding: "2px 6px",
  cursor: "pointer",
  selectors: { "&[aria-pressed='true']": { background: vars.color.accentBg, borderColor: vars.color.primary, color: vars.color.accentText } },
  "@media": { "(pointer: coarse)": { minHeight: "44px", minWidth: "44px" } },
});
export const textButton = style({
  border: "none",
  background: "transparent",
  color: vars.color.textPrimary,
  font: "inherit",
  fontSize: "12px",
  textDecoration: "underline",
  cursor: "pointer",
  padding: "2px 4px",
  "@media": { "(pointer: coarse)": { minHeight: "44px", minWidth: "44px" } },
});
export const filters = style({ display: "flex", flexDirection: "column", gap: "4px" });
export const fieldLabel = style({ fontSize: "11px", color: vars.color.textMuted });
export const checkLabel = style({ display: "flex", gap: "6px", alignItems: "center", fontSize: "12px", color: vars.color.textSecondary });

export const status = style({ padding: "4px 10px", fontSize: "12px", color: vars.color.textSecondary, display: "flex", gap: "8px", alignItems: "center", flexWrap: "wrap" });
export const inlineError = style({ padding: "2px 10px", fontSize: "12px", color: vars.color.errorText });
export const note = style({ padding: "4px 10px", fontSize: "12px", color: vars.color.textMuted });

export const helper = style({ padding: "12px 10px", fontSize: "13px", color: vars.color.textSecondary, display: "flex", flexDirection: "column", gap: "6px" });
export const emptyMessage = style({ padding: "12px 10px", fontSize: "13px", color: vars.color.textSecondary, display: "flex", flexDirection: "column", gap: "8px", alignItems: "flex-start" });

export const tree = style({ flex: 1, minHeight: 0, overflowY: "auto", position: "relative", outline: "none" });
export const treeDimmed = style({ opacity: 0.55 });
export const treeInner = style({ position: "relative", width: "100%" });
export const rowBase = style({
  position: "absolute",
  top: 0,
  left: 0,
  width: "100%",
  boxSizing: "border-box",
  display: "flex",
  gap: "8px",
  alignItems: "baseline",
  padding: "2px 10px",
  fontSize: "12px",
  cursor: "pointer",
  color: vars.color.textPrimary,
  overflow: "hidden",
  selectors: {
    "&[aria-selected='true']": { background: vars.color.accentBg, outline: `1px solid ${vars.color.primary}`, outlineOffset: "-1px" },
    "&:hover": { background: vars.color.hoverBackground },
  },
  "@media": { "(pointer: coarse)": { alignItems: "center", whiteSpace: "normal" } },
});
export const groupRow = style({ fontWeight: 600, background: vars.color.surfaceSubtle, alignItems: "center" });
export const groupPath = style({ flex: 1, minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" });
export const count = style({ color: vars.color.textMuted, fontWeight: 400, fontVariantNumeric: "tabular-nums" });
export const lineNo = style({ color: vars.color.textMuted, fontVariantNumeric: "tabular-nums", minWidth: "3ch", textAlign: "right", flexShrink: 0 });
export const hitText = style({ flex: 1, minWidth: 0, fontFamily: vars.font.mono, whiteSpace: "pre", overflow: "hidden", textOverflow: "ellipsis" });
export const mark = style({ background: vars.color.warningBg, color: vars.color.warningText, borderRadius: "2px", padding: "0 1px" });
export const dirtyNote = style({ color: vars.color.warningText, fontSize: "11px", flexShrink: 0 });

export const chip = style({
  display: "flex",
  alignItems: "center",
  gap: "6px",
  padding: "2px 10px",
  fontSize: "12px",
  borderTop: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  color: vars.color.textSecondary,
  "@media": {
    "(pointer: coarse)": {
      position: "fixed",
      right: "12px",
      bottom: "calc(64px + env(safe-area-inset-bottom, 0px))",
      zIndex: 35,
      border: `1px solid ${vars.color.borderMuted}`,
      borderRadius: vars.radii.md,
      boxShadow: "0 2px 10px rgba(0,0,0,0.25)",
    },
  },
});
export const chipButton = style({
  border: "none",
  background: "transparent",
  color: "inherit",
  font: "inherit",
  cursor: "pointer",
  padding: "0 6px",
  "@media": { "(pointer: coarse)": { minWidth: "44px", minHeight: "44px" } },
});
