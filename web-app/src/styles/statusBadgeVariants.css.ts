import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

/**
 * Shared backlog-item status color variants — compose onto a badge's own
 * layout style (e.g. `style([statusIdea, layoutStyle])`). Used by
 * BacklogItemBadge, BacklogItemDetail, and the backlog page's item-form modal.
 */
export const statusIdea = style({
  background: vars.color.surfaceMuted,
  color: vars.color.textMuted,
  border: `1px solid ${vars.color.borderMuted}`,
});

export const statusReady = style({
  background: vars.statusBadge.inputBg,
  color: vars.statusBadge.inputFg,
  border: `1px solid ${vars.statusBadge.inputBorder}`,
});

export const statusInProgress = style({
  background: vars.statusBadge.uncommittedBg,
  color: vars.statusBadge.uncommittedFg,
  border: `1px solid ${vars.statusBadge.uncommittedBorder}`,
});

export const statusReview = style({
  background: vars.statusBadge.approvalBg,
  color: vars.statusBadge.approvalFg,
  border: `1px solid ${vars.statusBadge.approvalBorder}`,
});

export const statusDone = style({
  background: vars.statusBadge.completeBg,
  color: vars.statusBadge.completeFg,
  border: `1px solid ${vars.statusBadge.completeBorder}`,
});

export const statusArchived = style({
  background: vars.color.surfaceMuted,
  color: vars.color.textDisabled,
  border: `1px solid ${vars.color.borderMuted}`,
});

export const statusRefining = style({
  background: vars.color.warningBg,
  color: vars.color.warningText,
  border: `1px solid ${vars.color.warning}`,
});
