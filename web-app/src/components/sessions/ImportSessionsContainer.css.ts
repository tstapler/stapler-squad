import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: 16,
  padding: 24,
  maxWidth: 960,
  margin: "0 auto",
});

export const errorBanner = style({
  background: vars.color.errorBg,
  color: vars.color.errorText,
  border: `1px solid ${vars.color.error}`,
  borderRadius: 8,
  padding: "10px 14px",
  fontSize: 14,
});
