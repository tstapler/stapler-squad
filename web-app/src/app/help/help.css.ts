import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme.css";
import { markdownBody } from "@/styles/markdown-body.css";

export { markdownBody };

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

export const loadingContainer = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  height: "200px",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
});
