import { recipe, type RecipeVariants } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme.css";

export const button = recipe({
  base: {
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    gap: vars.space["2"],
    fontWeight: "500",
    cursor: "pointer",
    border: "1px solid transparent",
    borderRadius: vars.radii.md,
    transition: "background 0.15s, border-color 0.15s, color 0.15s",
    textDecoration: "none",
    userSelect: "none",
    outline: "none",
    minHeight: "44px",
    selectors: {
      "&:focus-visible": {
        outline: `2px solid ${vars.color.primary}`,
        outlineOffset: "2px",
      },
      "&:disabled": {
        opacity: "0.5",
        cursor: "not-allowed",
        pointerEvents: "none",
      },
    },
  },

  variants: {
    intent: {
      primary: {
        background: vars.color.primary,
        color: vars.color.primaryText,
        selectors: {
          "&:hover": {
            background: vars.color.primaryHover,
          },
        },
      },
      secondary: {
        background: vars.color.hoverBackground,
        color: vars.color.textPrimary,
        borderColor: vars.color.borderColor,
        selectors: {
          "&:hover": {
            borderColor: vars.color.borderHover,
          },
        },
      },
      danger: {
        background: vars.color.error,
        color: vars.color.textInverse,
        selectors: {
          "&:hover": {
            background: vars.color.errorDark,
          },
        },
      },
      ghost: {
        background: "transparent",
        color: vars.color.textSecondary,
        selectors: {
          "&:hover": {
            background: vars.color.hoverBackground,
            color: vars.color.textPrimary,
          },
        },
      },
    },

    size: {
      sm: {
        padding: `${vars.space["1"]} ${vars.space["2"]}`,
        fontSize: vars.fontSize.sm,
        minHeight: "36px",
      },
      md: {
        padding: `${vars.space["2"]} ${vars.space["4"]}`,
        fontSize: vars.fontSize.base,
        minHeight: "44px",
      },
      lg: {
        padding: `${vars.space["3"]} ${vars.space["6"]}`,
        fontSize: vars.fontSize.lg,
        minHeight: "56px",
      },
    },
  },

  defaultVariants: {
    intent: "primary",
    size: "md",
  },
});

export type ButtonVariants = RecipeVariants<typeof button>;
