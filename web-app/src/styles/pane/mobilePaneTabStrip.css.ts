import { style, keyframes } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

const slideUp = keyframes({ from: { transform: "translateY(100%)" }, to: { transform: "translateY(0)" } });

export const mobileTabStrip = style({
  display: "flex",
  borderTop: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  flexShrink: 0,
  minHeight: "44px",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `0 ${vars.space["2"]}`,
  paddingBottom: "env(safe-area-inset-bottom, 0px)",
});

export const mobileSwitcherButton = style({
  flex: 1,
  minWidth: 0,
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  height: "36px",
  padding: `0 ${vars.space["3"]}`,
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.xs,
  fontFamily: vars.font.mono,
  cursor: "pointer",
  textAlign: "left",
});

export const mobileSwitcherLabel = style({ flex: 1, minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" });
export const mobileSwitcherCount = style({ flexShrink: 0, color: vars.color.textMuted });

export const mobileAddPaneButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  height: "36px",
  width: "36px",
  padding: 0,
  background: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  cursor: "pointer",
  fontSize: "18px",
  color: vars.color.textMuted,
  flexShrink: 0,
  selectors: { "&:hover": { background: vars.color.hoverBackground, color: vars.color.textPrimary } },
});

export const sheetBackdrop = style({
  position: "fixed",
  inset: 0,
  zIndex: zIndex.dialog,
  background: "rgba(0,0,0,0.45)",
  display: "flex",
  alignItems: "flex-end",
});

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
  "@media": { "(prefers-reduced-motion: reduce)": { animation: "none" } },
});

export const sheetHeader = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["2"]} ${vars.space["2"]} ${vars.space["3"]}`,
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const sheetSearch = style({
  flex: 1,
  minWidth: 0,
  height: "40px",
  padding: `0 ${vars.space["3"]}`,
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  color: vars.color.textPrimary,
  // 16px stops iOS Safari zooming the page when the field is focused.
  fontSize: "16px",
  ":focus-visible": { outline: `2px solid ${vars.color.inputFocusBorder}`, outlineOffset: "-2px" },
});

export const sheetClose = style({
  width: "44px",
  height: "44px",
  border: "none",
  background: "transparent",
  color: vars.color.textSecondary,
  cursor: "pointer",
  fontSize: "18px",
});

export const sheetList = style({ listStyle: "none", margin: 0, padding: 0, overflowY: "auto" });

export const sheetRow = style({
  display: "flex",
  flexDirection: "column",
  gap: "2px",
  width: "100%",
  minHeight: "52px",
  padding: "8px 16px",
  border: "none",
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
  background: "transparent",
  color: vars.color.textPrimary,
  font: "inherit",
  textAlign: "left",
  cursor: "pointer",
  selectors: {
    "&[aria-selected='true']": { background: vars.color.hoverBackground, boxShadow: `inset 3px 0 0 ${vars.color.primary}` },
  },
});

export const sheetRowTitle = style({
  fontSize: "15px",
  // Long titles (a pasted paragraph) stay at two lines so the list remains scannable.
  display: "-webkit-box",
  WebkitLineClamp: 2,
  WebkitBoxOrient: "vertical",
  overflow: "hidden",
  wordBreak: "break-word",
});

export const sheetRowMeta = style({
  fontSize: "12px",
  color: vars.color.textMuted,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const sheetEmpty = style({ padding: "24px 16px", color: vars.color.textSecondary });
