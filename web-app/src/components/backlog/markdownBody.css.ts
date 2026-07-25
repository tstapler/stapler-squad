import { globalStyle, style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const markdownBody = style({
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.sm,
  lineHeight: 1.6,
});

globalStyle(`${markdownBody} p`, {
  margin: `0 0 ${vars.space["2"]} 0`,
});

globalStyle(`${markdownBody} p:last-child`, {
  marginBottom: 0,
});

globalStyle(`${markdownBody} a`, {
  color: vars.color.primary,
  textDecoration: "underline",
});

globalStyle(`${markdownBody} img`, {
  maxWidth: "100%",
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderSubtle}`,
});

globalStyle(`${markdownBody} code`, {
  fontFamily: vars.font.mono,
  fontSize: "0.875em",
  background: vars.color.cardBackground,
  padding: "0.15em 0.35em",
  borderRadius: vars.radii.sm,
  border: `1px solid ${vars.color.borderSubtle}`,
});

globalStyle(`${markdownBody} pre`, {
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: vars.space["3"],
  overflowX: "auto",
});

globalStyle(`${markdownBody} pre code`, {
  background: "transparent",
  border: "none",
  padding: 0,
});

globalStyle(`${markdownBody} ul, ${markdownBody} ol`, {
  paddingLeft: vars.space["6"],
  marginBottom: vars.space["2"],
});

globalStyle(`${markdownBody} blockquote`, {
  borderLeft: `3px solid ${vars.color.primary}`,
  paddingLeft: vars.space["3"],
  color: vars.color.textMuted,
  marginLeft: 0,
});
