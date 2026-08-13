import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme.css";

export const kbd = recipe({
  base: {
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    fontFamily: vars.font.mono,
    lineHeight: 1,
    background: vars.color.primary,
    color: vars.color.primaryText,
    border: `1px solid ${vars.color.borderStrong}`,
    borderRadius: vars.radii.sm,
    userSelect: "none",
  },
  variants: {
    size: {
      sm: {
        fontSize: vars.fontSize.xs,
        padding: "2px 5px",
        minWidth: "1.25rem",
      },
      md: {
        fontSize: vars.fontSize.sm,
        padding: "3px 7px",
        minWidth: "1.5rem",
      },
    },
  },
  defaultVariants: {
    size: "md",
  },
});
