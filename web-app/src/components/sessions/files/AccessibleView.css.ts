import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const region = style({ display: "flex", flexDirection: "column", minHeight: 0 });

export const pager = style({
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: "8px",
  padding: "4px 8px",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  position: "sticky",
  top: 0,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
});

export const control = style({
  minWidth: "44px",
  minHeight: "44px",
  font: "inherit",
  padding: "0 12px",
  cursor: "pointer",
  ":focus-visible": { outline: `2px solid ${vars.color.inputFocusBorder}`, outlineOffset: "2px" },
  selectors: { "&[aria-disabled='true'], &:disabled": { opacity: 0.5, cursor: "default" } },
});

export const gotoInput = style({ minHeight: "44px", width: "7em", font: "inherit", padding: "0 8px", marginLeft: "6px" });

export const pageLabel = style({ fontVariantNumeric: "tabular-nums" });

export const unsaved = style({ fontWeight: 600 });

export const page = style({
  margin: 0,
  padding: "8px",
  fontFamily: vars.font.mono,
  fontSize: "14px",
  lineHeight: 1.5,
  whiteSpace: "pre-wrap",
  overflowWrap: "anywhere",
  ":focus-visible": { outline: `2px solid ${vars.color.inputFocusBorder}`, outlineOffset: "-2px" },
});

export const line = style({
  display: "block",
  selectors: { "&[data-hit='true']": { background: vars.color.hoverBackground } },
  ":focus-visible": { outline: `2px solid ${vars.color.inputFocusBorder}` },
});

export const skipLink = style({
  position: "absolute",
  left: "8px",
  top: "4px",
  zIndex: 5,
  minHeight: "44px",
  padding: "0 12px",
  font: "inherit",
  transform: "translateY(-200%)",
  ":focus": { transform: "none" },
});
