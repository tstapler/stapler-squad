import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "../../styles/theme.css";

export const badge = recipe({
  base: {
    display: "inline-flex",
    alignItems: "center",
    gap: "4px",
    padding: "3px 10px",
    borderRadius: "12px",
    fontSize: "0.75rem",
    fontWeight: 600,
    whiteSpace: "nowrap",
  },
  variants: {
    level: {
      critical: {
        background: vars.color.criticalBg,
        color: vars.color.criticalText,
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
  fontSize: "0.8125rem",
  lineHeight: 1,
});
