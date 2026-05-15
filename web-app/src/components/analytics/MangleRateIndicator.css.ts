import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: vars.space["4"],
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
  backgroundColor: vars.color.cardBackground,
});

export const rateRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
});

export const rateValue = recipe({
  base: {
    fontSize: vars.fontSize.xl,
    fontWeight: vars.fontWeight.bold,
    fontVariantNumeric: "tabular-nums",
    lineHeight: 1,
  },
  variants: {
    severity: {
      good: { color: vars.color.success },
      warning: { color: vars.color.warning },
      error: { color: vars.color.error },
    },
  },
  defaultVariants: { severity: "good" },
});

export const rateBadge = recipe({
  base: {
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
    padding: `${vars.space["1"]} ${vars.space["2"]}`,
    borderRadius: vars.radii.full,
    fontSize: vars.fontSize.xs,
    fontWeight: vars.fontWeight.semibold,
  },
  variants: {
    severity: {
      good: {
        backgroundColor: vars.color.successBg,
        color: vars.color.success,
      },
      warning: {
        backgroundColor: vars.color.warningBg,
        color: vars.color.warningText,
      },
      error: {
        backgroundColor: vars.color.errorBg,
        color: vars.color.errorText,
      },
    },
  },
  defaultVariants: { severity: "good" },
});

export const subtitle = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
});

export const countsRow = style({
  display: "flex",
  gap: vars.space["4"],
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
});

export const countItem = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["1"],
});

export const countValue = style({
  fontSize: vars.fontSize.lg,
  fontWeight: vars.fontWeight.semibold,
  fontVariantNumeric: "tabular-nums",
  color: vars.color.textPrimary,
});

export const countLabel = style({
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
  textTransform: "uppercase",
  letterSpacing: "0.05em",
});
