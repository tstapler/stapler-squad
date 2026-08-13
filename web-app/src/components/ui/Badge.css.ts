import { recipe, type RecipeVariants } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme.css";

export const badge = recipe({
  base: {
    display: "inline-flex",
    alignItems: "center",
    gap: vars.space["1"],
    fontWeight: "500",
    borderRadius: vars.radii.full,
    whiteSpace: "nowrap",
  },

  variants: {
    intent: {
      default: {
        background: vars.color.hoverBackground,
        color: vars.color.textSecondary,
      },
      success: {
        background: vars.color.successBg,
        color: vars.color.success,
      },
      warning: {
        background: vars.color.warningBg,
        color: vars.color.warning,
      },
      error: {
        background: vars.color.errorBg,
        color: vars.color.errorText,
      },
      primary: {
        background: vars.color.primary,
        color: vars.color.primaryText,
      },
    },

    size: {
      sm: {
        padding: `${vars.space["0"]} ${vars.space["2"]}`,
        fontSize: vars.fontSize.xs,
        height: "20px",
      },
      md: {
        padding: `${vars.space["1"]} ${vars.space["3"]}`,
        fontSize: vars.fontSize.sm,
        height: "24px",
      },
    },
  },

  defaultVariants: {
    intent: "default",
    size: "md",
  },
});

export type BadgeVariants = RecipeVariants<typeof badge>;
