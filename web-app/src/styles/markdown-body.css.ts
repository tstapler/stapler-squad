import { globalStyle, style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// Shared GFM body styles — used by the help page and the file viewer.
export const markdownBody = style({
  maxWidth: "720px",
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.base,
  lineHeight: 1.7,
});

// Heading styles inside markdown body
globalStyle(`${markdownBody} h1`, {
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.xl,
  fontWeight: vars.fontWeight.bold,
  marginBottom: vars.space["4"],
  marginTop: 0,
  paddingBottom: vars.space["2"],
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

globalStyle(`${markdownBody} h2`, {
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.semibold,
  marginTop: vars.space["8"],
  marginBottom: vars.space["3"],
});

globalStyle(`${markdownBody} h3`, {
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.semibold,
  marginTop: vars.space["6"],
  marginBottom: vars.space["2"],
});

globalStyle(`${markdownBody} p`, {
  marginBottom: vars.space["4"],
});

globalStyle(`${markdownBody} a`, {
  color: vars.color.primary,
  textDecoration: "underline",
});

globalStyle(`${markdownBody} a:hover`, {
  color: vars.color.primaryHover,
});

globalStyle(`${markdownBody} code`, {
  fontFamily: vars.font.mono,
  fontSize: "0.875em",
  background: vars.color.cardBackground,
  padding: "0.15em 0.35em",
  borderRadius: vars.radii.sm,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderSubtle}`,
});

globalStyle(`${markdownBody} pre`, {
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.md,
  padding: vars.space["4"],
  overflowX: "auto",
  marginBottom: vars.space["4"],
});

globalStyle(`${markdownBody} pre code`, {
  background: "transparent",
  border: "none",
  padding: 0,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
});

globalStyle(`${markdownBody} ul, ${markdownBody} ol`, {
  paddingLeft: vars.space["6"],
  marginBottom: vars.space["4"],
});

globalStyle(`${markdownBody} li`, {
  marginBottom: vars.space["1"],
});

globalStyle(`${markdownBody} table`, {
  width: "100%",
  borderCollapse: "collapse",
  marginBottom: vars.space["4"],
  fontSize: vars.fontSize.sm,
});

globalStyle(`${markdownBody} th`, {
  color: vars.color.textPrimary,
  fontWeight: vars.fontWeight.semibold,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  border: `1px solid ${vars.color.borderColor}`,
  borderBottomWidth: "2px",
  textAlign: "left",
});

globalStyle(`${markdownBody} td`, {
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  border: `1px solid ${vars.color.borderColor}`,
});

globalStyle(`${markdownBody} blockquote`, {
  borderLeft: `3px solid ${vars.color.primary}`,
  paddingLeft: vars.space["4"],
  color: vars.color.textMuted,
  marginLeft: 0,
  marginBottom: vars.space["4"],
});
