import { style, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const bar = style({
  display: "flex",
  alignItems: "stretch",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  position: "relative",
  flexShrink: 0,
});

export const tabList = style({
  display: "flex",
  flex: 1,
  minWidth: 0,
  overflowX: "auto",
  scrollbarWidth: "thin",
});

export const tab = style({
  display: "flex",
  alignItems: "center",
  gap: "4px",
  padding: "0 4px 0 10px",
  minHeight: "32px",
  maxWidth: "200px",
  flexShrink: 0,
  cursor: "pointer",
  color: vars.color.textSecondary,
  borderRight: `1px solid ${vars.color.borderSubtle}`,
  fontSize: "13px",
  whiteSpace: "nowrap",
  selectors: {
    "&[aria-selected='true']": {
      background: vars.color.background,
      color: vars.color.textPrimary,
      boxShadow: `inset 0 -2px 0 ${vars.color.primary}`,
    },
    "&:hover": { background: vars.color.hoverBackground },
  },
  ":focus-visible": { outline: `2px solid ${vars.color.inputFocusBorder}`, outlineOffset: "-2px" },
});

export const tabLabel = style({ overflow: "hidden", textOverflow: "ellipsis" });
globalStyle(`${tab}[data-preview='true'] ${tabLabel}`, { fontStyle: "italic" });

export const pinGlyph = style({ fontSize: "10px", opacity: 0.7 });

// 3:1 against both the tab and bar backgrounds in light and dark (tokens, not hex).
export const dirtyDot = style({
  width: "8px",
  height: "8px",
  borderRadius: "50%",
  background: vars.color.warningText,
  flexShrink: 0,
});

export const closeButton = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  width: "24px",
  height: "24px",
  border: "none",
  borderRadius: vars.radii.sm,
  background: "transparent",
  color: "inherit",
  cursor: "pointer",
  lineHeight: 1,
  ":hover": { background: vars.color.accentHover },
  "@media": { "(pointer: coarse)": { width: "44px", height: "44px" } },
});

export const overflowButton = style({
  border: "none",
  borderLeft: `1px solid ${vars.color.borderSubtle}`,
  background: "transparent",
  color: vars.color.textSecondary,
  padding: "0 10px",
  cursor: "pointer",
  minWidth: "32px",
  "@media": { "(pointer: coarse)": { minWidth: "44px", minHeight: "44px" } },
});

const floating = {
  position: "absolute" as const,
  zIndex: 20,
  background: vars.color.modalBackground,
  border: `1px solid ${vars.color.modalBorder}`,
  borderRadius: vars.radii.md,
  boxShadow: "0 4px 16px rgba(0,0,0,0.25)",
  padding: "4px",
  minWidth: "180px",
};

export const menu = style({ ...floating, top: "100%", left: "8px" });
export const tabListDialog = style({ ...floating, top: "100%", right: "0", maxHeight: "50vh", overflowY: "auto", minWidth: "260px" });

export const menuItem = style({
  display: "block",
  width: "100%",
  textAlign: "left",
  padding: "6px 10px",
  border: "none",
  background: "transparent",
  color: vars.color.textPrimary,
  cursor: "pointer",
  fontSize: "13px",
  borderRadius: vars.radii.sm,
  ":hover": { background: vars.color.hoverBackground },
  ":focus-visible": { outline: `2px solid ${vars.color.inputFocusBorder}` },
  "@media": { "(pointer: coarse)": { minHeight: "44px" } },
});

export const listRow = style({ display: "flex", alignItems: "center", gap: "4px" });

export const toast = style({
  position: "absolute",
  top: "100%",
  left: "8px",
  right: "8px",
  zIndex: 15,
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

export const toastAction = style({
  border: "none",
  background: "transparent",
  color: "inherit",
  textDecoration: "underline",
  cursor: "pointer",
  fontSize: "13px",
  "@media": { "(pointer: coarse)": { minHeight: "44px", minWidth: "44px" } },
});

export const srOnly = style({
  position: "absolute",
  width: "1px",
  height: "1px",
  overflow: "hidden",
  clip: "rect(0 0 0 0)",
  whiteSpace: "nowrap",
});
