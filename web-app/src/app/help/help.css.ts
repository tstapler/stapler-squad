import { globalStyle, style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme.css";

export const pageRoot = style({
  display: "flex",
  height: "100%",
  gap: vars.space["4"],
  minHeight: 0,
});

export const sidebar = style({
  width: "240px",
  flexShrink: 0,
  overflowY: "auto",
  padding: vars.space["4"],
  borderRight: `1px solid ${vars.color.borderColor}`,
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const searchInput = style({
  width: "100%",
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  marginBottom: vars.space["3"],
  background: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.md,
  color: vars.color.inputText,
  fontSize: vars.fontSize.sm,
  outline: "none",
  transition: vars.transition.fast,
  selectors: {
    "&:focus": {
      borderColor: vars.color.inputFocusBorder,
    },
    "&::placeholder": {
      color: vars.color.placeholderColor,
    },
  },
});

export const sidebarLink = recipe({
  base: {
    display: "block",
    padding: `${vars.space["1"]} ${vars.space["2"]}`,
    borderRadius: vars.radii.sm,
    fontSize: vars.fontSize.sm,
    color: vars.color.textSecondary,
    cursor: "pointer",
    transition: vars.transition.fast,
    textDecoration: "none",
    background: "transparent",
    border: "none",
    width: "100%",
    textAlign: "left",
    selectors: {
      "&:hover": {
        background: vars.color.hoverBackground,
        color: vars.color.textPrimary,
      },
    },
  },
  variants: {
    active: {
      true: {
        color: vars.color.primary,
        background: vars.color.accentBg,
        fontWeight: vars.fontWeight.medium,
        selectors: {
          "&:hover": {
            background: vars.color.accentHover,
            color: vars.color.primary,
          },
        },
      },
    },
  },
  defaultVariants: {
    active: false,
  },
});

export const articlePane = style({
  flex: 1,
  overflowY: "auto",
  padding: vars.space["6"],
  minWidth: 0,
});

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
  borderBottom: `2px solid ${vars.color.borderColor}`,
  textAlign: "left",
});

globalStyle(`${markdownBody} td`, {
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
});

globalStyle(`${markdownBody} blockquote`, {
  borderLeft: `3px solid ${vars.color.primary}`,
  paddingLeft: vars.space["4"],
  color: vars.color.textMuted,
  marginLeft: 0,
  marginBottom: vars.space["4"],
});

export const loadingContainer = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  height: "200px",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});
