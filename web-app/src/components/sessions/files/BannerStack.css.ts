import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const stack = style({ display: "flex", flexDirection: "column" });

export const collapsedRow = style({
  display: "flex",
  alignItems: "center",
  gap: "4px",
  padding: "0 8px",
  fontSize: "13px",
  background: vars.color.warningBg,
  color: vars.color.warningText,
  borderBottom: `1px solid ${vars.color.warning}`,
});

export const collapsedText = style({
  flex: 1,
  minWidth: 0,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
});

export const moreButton = style({
  alignSelf: "flex-start",
  border: "none",
  background: "transparent",
  color: vars.color.accentText,
  font: "inherit",
  fontSize: "12px",
  textDecoration: "underline",
  cursor: "pointer",
  padding: "2px 10px",
});
