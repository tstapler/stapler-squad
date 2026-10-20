import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const banner = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: "8px",
  padding: "6px 10px",
  fontSize: "13px",
  background: vars.color.warningBg,
  color: vars.color.warningText,
  borderBottom: `1px solid ${vars.color.warning}`,
});

export const bannerAction = style({
  border: "none",
  background: "transparent",
  color: "inherit",
  font: "inherit",
  textDecoration: "underline",
  cursor: "pointer",
  padding: "0 2px",
  "@media": { "(pointer: coarse)": { minHeight: "44px", minWidth: "44px" } },
});
