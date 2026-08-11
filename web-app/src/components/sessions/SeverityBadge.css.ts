import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "../../styles/theme.css";

export const badge = recipe({
  base: {
    display: "inline-flex",
    alignItems: "center",
    gap: vars.space[1],
    // ponytail: padding stays sub-token-grain (the space scale starts at 4px, too coarse
    // for a tight badge) — matches StatusBadge.css.ts's identical "3px 10px", not a new
    // one-off value.
    padding: "3px 10px",
    borderRadius: vars.radii.lg,
    fontSize: vars.fontSize.xs,
    fontWeight: 600,
    whiteSpace: "nowrap",
  },
  variants: {
    level: {
      critical: {
        background: vars.color.criticalBg,
        color: vars.color.criticalText,
        border: `1px solid ${vars.color.critical}`,
      },
      high: {
        background: vars.color.errorBg,
        color: vars.color.errorText,
      },
      medium: {
        background: vars.color.warningBg,
        color: vars.color.warningText,
      },
      low: {
        background: vars.color.successBg,
        color: vars.color.successText,
      },
      unknown: {
        background: vars.color.surfaceMuted,
        color: vars.color.textMuted,
      },
    },
    compact: {
      true: { padding: "2px 6px" },
      false: {},
    },
  },
  defaultVariants: { level: "unknown", compact: false },
});

export const icon = style({
  fontSize: vars.fontSize.sm,
  lineHeight: 1,
});
