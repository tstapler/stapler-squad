import { style } from "@vanilla-extract/css";
import { vars } from "../../styles/theme.css";

export const wrapper = style({
  display: "inline-flex",
  alignItems: "center",
  gap: "4px",
  padding: "3px 10px",
  borderRadius: "12px",
  fontSize: "0.75rem",
  fontWeight: 600,
  whiteSpace: "nowrap",
  background: vars.color.warningBg,
  color: vars.color.warningText,
  border: `1px solid ${vars.color.warning}`,
});

export const icon = style({
  fontSize: "0.8125rem",
  lineHeight: 1,
});
