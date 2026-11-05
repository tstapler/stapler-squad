import { keyframes, style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import { zIndex } from "@/styles/theme-contract.css";
import { MIN_TOUCH_TARGET } from "@/lib/unfinished/prTouchTokens";

const NARROW = "screen and (max-width: 480px)";
const focusRing = { outline: `2px solid ${vars.color.inputFocusBorder}`, outlineOffset: "2px" };

export const section = style({ display: "flex", flexDirection: "column", gap: vars.space["2"], minWidth: 0, scrollMarginTop: vars.space["4"] });

export const header = style({ display: "flex", alignItems: "center", flexWrap: "wrap", gap: vars.space["2"], justifyContent: "space-between" });
export const heading = style({
  margin: 0,
  fontSize: vars.fontSize.base,
  fontWeight: 600,
  color: vars.color.textPrimary,
  ":focus-visible": focusRing,
});
export const headerActions = style({ display: "flex", alignItems: "center", gap: vars.space["2"], position: "relative" });
export const hint = style({ margin: 0, fontSize: vars.fontSize.xs, color: vars.color.textSecondary, overflowWrap: "anywhere" });

export const list = style({ listStyle: "none", margin: 0, padding: 0, display: "flex", flexDirection: "column", gap: vars.space["2"] });

export const row = style({
  position: "relative",
  display: "grid",
  gridTemplateColumns: "minmax(0, 1fr) auto",
  gap: vars.space["1"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  background: vars.color.cardBackground,
  overflowWrap: "anywhere",
  "@media": { [NARROW]: { gridTemplateColumns: "minmax(0, 1fr) auto" } },
});
export const rowActive = style({ borderColor: vars.color.primary });
export const rowTerminal = style({ background: vars.color.surfaceSubtle, color: vars.color.textSecondary });

export const rowMain = style({ display: "flex", flexDirection: "column", gap: vars.space["1"], minWidth: 0 });
export const rowLine = style({ display: "flex", alignItems: "center", flexWrap: "wrap", gap: vars.space["2"], minWidth: 0, fontSize: vars.fontSize.sm });
export const rowMeta = style({ fontSize: vars.fontSize.xs, color: vars.color.textSecondary, display: "flex", flexWrap: "wrap", gap: vars.space["2"] });
export const glyph = style({ width: 14, height: 14, flexShrink: 0 });

export const prTitleText = style({ fontSize: vars.fontSize.sm, color: vars.color.textPrimary, overflowWrap: "anywhere", minWidth: 0 });

export const prLink = style({
  color: vars.color.textPrimary,
  fontWeight: 600,
  textDecoration: "underline",
  minHeight: MIN_TOUCH_TARGET,
  display: "inline-flex",
  alignItems: "center",
  ":focus-visible": focusRing,
  "@media": { "(min-width: 481px)": { minHeight: "24px" } },
});

const pill = {
  display: "inline-flex",
  alignItems: "center",
  padding: `0 ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  lineHeight: 1.6,
} as const;
export const roleChip = style({ ...pill, background: vars.color.surfaceSubtle, color: vars.color.textSecondary, border: `1px solid ${vars.color.borderColor}` });
export const activeChip = style({ ...pill, background: vars.color.accentBg, color: vars.color.accentText });
export const errorChip = style({ ...pill, background: vars.color.warningBg, color: vars.color.warningText, whiteSpace: "normal" });

export const holdLine = style({ margin: 0, fontSize: vars.fontSize.xs, color: vars.color.textSecondary, overflowWrap: "anywhere" });
export const holdWarning = style({ color: vars.color.warningText, background: vars.color.warningBg, padding: vars.space["1"], borderRadius: vars.radii.sm });

export const rowActions = style({ display: "flex", alignItems: "flex-start", gap: vars.space["1"], position: "relative", flexWrap: "wrap", justifyContent: "flex-end" });
export const inlineActions = style({ display: "flex", gap: vars.space["1"], "@media": { [NARROW]: { display: "none" } } });

export const button = style({
  minHeight: MIN_TOUCH_TARGET,
  minWidth: MIN_TOUCH_TARGET,
  padding: `0 ${vars.space["3"]}`,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.surfaceSubtle,
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  ":disabled": { opacity: 0.6, cursor: "not-allowed" },
  ":focus-visible": focusRing,
  "@media": { "(min-width: 481px)": { minHeight: "32px" } },
});
export const disclosure = style([button, { alignSelf: "flex-start", "@media": { [NARROW]: { alignSelf: "stretch" } } }]);

export const menu = style({
  position: "absolute",
  top: "100%",
  right: 0,
  zIndex: zIndex.dropdown,
  minWidth: 200,
  maxWidth: "calc(100vw - 32px)",
  display: "flex",
  flexDirection: "column",
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.md,
  padding: vars.space["1"],
});
export const menuItem = style({
  minHeight: MIN_TOUCH_TARGET,
  padding: `0 ${vars.space["3"]}`,
  display: "flex",
  alignItems: "center",
  textAlign: "left",
  background: "transparent",
  color: vars.color.textPrimary,
  border: 0,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  textDecoration: "none",
  ":hover": { background: vars.color.hoverBackground },
  ":focus-visible": focusRing,
});

export const empty = style({ display: "flex", flexDirection: "column", gap: vars.space["2"], alignItems: "flex-start", fontSize: vars.fontSize.sm });
export const alertBox = style({ padding: vars.space["2"], borderRadius: vars.radii.md, background: vars.color.errorBg, color: vars.color.errorText, fontSize: vars.fontSize.sm, display: "flex", flexDirection: "column", gap: vars.space["2"], alignItems: "flex-start" });

const pulse = keyframes({ "0%, 100%": { opacity: 1 }, "50%": { opacity: 0.5 } });

export const skeleton = style({
  height: 56,
  borderRadius: vars.radii.md,
  background: vars.color.surfaceSubtle,
  animation: `${pulse} 1.4s ease-in-out infinite`,
  "@media": { "(prefers-reduced-motion: reduce)": { animation: "none" } },
});

export const visuallyHidden = style({
  position: "absolute",
  width: 1,
  height: 1,
  margin: -1,
  padding: 0,
  overflow: "hidden",
  clip: "rect(0 0 0 0)",
  whiteSpace: "nowrap",
  border: 0,
});

/** Above the bottom navigation and the safe area; dialogs are closed by the time it shows. */
export const toast = style({
  position: "fixed",
  left: vars.space["4"],
  right: vars.space["4"],
  bottom: "calc(var(--bottom-nav-height, 72px) + env(safe-area-inset-bottom, 0px) + 16px)",
  zIndex: zIndex.toast,
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space["3"],
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  borderRadius: vars.radii.lg,
  background: vars.color.modalBackground,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.modalBorder}`,
  "@media": { "(min-width: 481px)": { left: "auto", maxWidth: 420 } },
});
export const toastButton = style([button, { fontWeight: 700 }]);
