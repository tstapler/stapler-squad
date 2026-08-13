import { style } from "@vanilla-extract/css";
import { recipe, type RecipeVariants } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme.css";

export const inputWrapper = recipe({
  base: {
    display: "flex",
    flexDirection: "column",
    gap: vars.space["1"],
  },
});

export const input = recipe({
  base: {
    display: "block",
    width: "100%",
    background: vars.color.inputBackground,
    color: vars.color.inputText,
    border: `1px solid ${vars.color.inputBorder}`,
    borderRadius: vars.radii.md,
    outline: "none",
    transition: "border-color 0.15s",
    selectors: {
      "&::placeholder": { color: vars.color.placeholderColor },
      "&:focus": {
        borderColor: vars.color.inputFocusBorder,
        boxShadow: "0 0 0 3px rgba(0,112,243,0.15)",
      },
      "&:disabled": {
        opacity: "0.5",
        cursor: "not-allowed",
        background: vars.color.hoverBackground,
      },
    },
  },
  variants: {
    size: {
      sm: {
        padding: `${vars.space["1"]} ${vars.space["2"]}`,
        fontSize: vars.fontSize.sm,
        minHeight: "36px",
      },
      md: {
        padding: `${vars.space["2"]} ${vars.space["3"]}`,
        fontSize: vars.fontSize.base,
        minHeight: "44px",
      },
      lg: {
        padding: `${vars.space["3"]} ${vars.space["4"]}`,
        fontSize: vars.fontSize.base,
        minHeight: "52px",
      },
    },
    state: {
      default: {},
      error: {
        borderColor: vars.color.error,
        selectors: {
          "&:focus": {
            borderColor: vars.color.error,
            boxShadow: "0 0 0 3px rgba(239,68,68,0.15)",
          },
        },
      },
      disabled: {
        opacity: "0.5",
        cursor: "not-allowed",
      },
    },
  },
  defaultVariants: {
    size: "md",
    state: "default",
  },
});

export const inputLabel = style({
  fontSize: vars.fontSize.sm,
  fontWeight: "500",
  color: vars.color.textSecondary,
  display: "block",
});

export const inputError = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.error,
});

export type InputVariants = RecipeVariants<typeof input>;
