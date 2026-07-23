import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme-contract.css";

export const pill = recipe({
  base: {
    display: "inline-flex",
    alignItems: "center",
    gap: vars.space[1],
    padding: `${vars.space[1]} ${vars.space[2]}`,
    borderRadius: vars.radii.full,
    fontSize: vars.fontSize.sm,
    fontWeight: vars.fontWeight.medium,
    whiteSpace: "nowrap",
  },
  variants: {
    state: {
      shipped: { color: vars.color.success, background: vars.color.successBg },
      ready_to_merge: { color: vars.color.success, background: vars.color.successBg },
      draft: { color: vars.color.textSecondary, background: vars.color.surfaceMuted },
      conflicted: { color: vars.color.errorText, background: vars.color.errorBg },
      changes_requested: { color: vars.color.warningText, background: vars.color.warningBg },
      ci_failing: { color: vars.color.errorText, background: vars.color.errorBg },
      ci_pending: { color: vars.color.warningText, background: vars.color.warningBg },
      closed_unshipped: { color: vars.color.textMuted, background: vars.color.surfaceMuted },
      snapshot_unavailable: { color: vars.color.warningText, background: vars.color.warningBg },
      no_pr: { color: vars.color.textMuted, background: vars.color.surfaceMuted },
    },
  },
  defaultVariants: { state: "no_pr" },
});
