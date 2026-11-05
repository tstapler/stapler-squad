import { style, keyframes, globalStyle } from "@vanilla-extract/css";
import { vars, breakpoints, zIndex } from "../../styles/theme-contract.css";
import { MIN_TOUCH_TARGET } from "@/lib/unfinished/prTouchTokens";

const focusRing = { outline: `2px solid ${vars.color.inputFocusBorder}`, outlineOffset: "2px" } as const;
const mobileOnly = `screen and (max-width: ${parseInt(breakpoints.md, 10) - 1}px)`;

export const page = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
  maxWidth: "1200px",
  margin: "0 auto",
  padding: vars.space["4"],
  minWidth: 0,
  color: vars.color.textPrimary,
  "@media": { [mobileOnly]: { padding: vars.space["3"] } },
});

export const heading = style({ margin: 0, fontSize: vars.fontSize.xl, fontWeight: 600, selectors: { "&:focus": { outline: "none" } } });
export const muted = style({ margin: 0, fontSize: vars.fontSize.sm, color: vars.color.textSecondary });

export const searchForm = style({ display: "flex", flexWrap: "wrap", gap: vars.space["2"], alignItems: "flex-start" });
export const searchField = style({ display: "flex", flexDirection: "column", gap: vars.space["1"], flex: "1 1 240px", minWidth: 0 });

const fieldBase = {
  minHeight: MIN_TOUCH_TARGET,
  padding: `0 ${vars.space["3"]}`,
  background: vars.color.inputBackground,
  color: vars.color.inputText,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.base,
  minWidth: 0,
  selectors: { "&:focus-visible": focusRing },
} as const;
export const input = style({ ...fieldBase, width: "100%", boxSizing: "border-box" });
export const select = style({ ...fieldBase });

export const button = style({
  minHeight: MIN_TOUCH_TARGET,
  minWidth: MIN_TOUCH_TARGET,
  padding: `0 ${vars.space["3"]}`,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderStrong}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  selectors: { "&:focus-visible": focusRing, "&:hover": { background: vars.color.hoverBackground } },
});
export const primaryButton = style({ background: vars.color.primary, color: vars.color.onPrimaryFill, borderColor: vars.color.primary });

export const hint = style({ margin: 0, fontSize: vars.fontSize.xs, color: vars.color.textSecondary });
export const invalid = style({ margin: 0, fontSize: vars.fontSize.xs, color: vars.color.errorText });

export const filterBar = style({ display: "flex", flexWrap: "wrap", gap: vars.space["3"], alignItems: "flex-end" });
export const filterField = style({ display: "flex", flexDirection: "column", gap: vars.space["1"], fontSize: vars.fontSize.xs, color: vars.color.textSecondary });

export const sheetBackdrop = style({
  position: "fixed",
  inset: 0,
  background: vars.color.overlayBackground,
  display: "flex",
  alignItems: "flex-end",
  zIndex: zIndex.dialog,
});
export const sheet = style({
  width: "100%",
  boxSizing: "border-box",
  maxHeight: "80vh",
  overflowY: "auto",
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
  padding: vars.space["4"],
  background: vars.color.modalBackground,
  color: vars.color.textPrimary,
  borderTopLeftRadius: vars.radii.lg,
  borderTopRightRadius: vars.radii.lg,
});
export const sheetActions = style({ display: "flex", gap: vars.space["2"], justifyContent: "flex-end" });

export const statusLine = style({ display: "flex", alignItems: "center", gap: vars.space["2"], fontSize: vars.fontSize.sm, color: vars.color.textSecondary });
const spin = keyframes({ to: { transform: "rotate(360deg)" } });
export const spinner = style({
  width: "14px",
  height: "14px",
  borderRadius: "50%",
  border: `2px solid ${vars.color.borderStrong}`,
  borderTopColor: vars.color.primary,
  animation: `${spin} 0.8s linear infinite`,
  "@media": { "(prefers-reduced-motion: reduce)": { animation: "none" } },
});
export const dimmed = style({ opacity: 0.55 });

export const notice = style({
  margin: 0,
  padding: vars.space["2"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  background: vars.color.surfaceSubtle,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
});
export const errorBox = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: vars.space["2"],
  padding: vars.space["3"],
  color: vars.color.errorText,
  background: vars.color.errorBg,
  border: `1px solid ${vars.color.error}`,
  borderRadius: vars.radii.sm,
});
export const empty = style({ display: "flex", flexDirection: "column", alignItems: "flex-start", gap: vars.space["2"], padding: vars.space["4"] });

export const table = style({ width: "100%", borderCollapse: "collapse", fontSize: vars.fontSize.sm });
export const th = style({
  textAlign: "left",
  padding: vars.space["2"],
  color: vars.color.textSecondary,
  fontWeight: 600,
  fontSize: vars.fontSize.xs,
  borderBottom: `1px solid ${vars.color.borderStrong}`,
});
export const td = style({ padding: vars.space["2"], verticalAlign: "top", borderBottom: `1px solid ${vars.color.borderColor}`, overflowWrap: "anywhere" });

export const cards = style({ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: vars.space["2"] });
export const card = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
  padding: vars.space["3"],
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  minWidth: 0,
});
export const kind = style({ fontSize: vars.fontSize.xs, color: vars.color.textSecondary, textTransform: "uppercase", letterSpacing: "0.04em" });
export const mono = style({ fontFamily: vars.font.mono, overflowWrap: "anywhere" });
export const tag = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.full,
  padding: "0 6px",
  marginLeft: vars.space["1"],
});
export const sessionLink = style({
  display: "inline-flex",
  alignItems: "center",
  minHeight: MIN_TOUCH_TARGET,
  color: vars.color.textPrimary,
  textDecoration: "underline",
  overflowWrap: "anywhere",
  selectors: { "&:focus-visible": focusRing },
});
export const loadMore = style({ alignSelf: "flex-start" });

// The shared PR chip is a compact badge; on a touch card its tap area still has to reach the minimum target.
globalStyle(`${card} [data-testid="pr-chip"], ${card} [data-testid="pr-more-button"]`, {
  minHeight: MIN_TOUCH_TARGET,
  display: "inline-flex",
  alignItems: "center",
});
