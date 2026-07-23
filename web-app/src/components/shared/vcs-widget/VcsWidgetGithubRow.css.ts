import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const container = style({
  display: "flex",
  alignItems: "center",
  flexWrap: "wrap",
  gap: vars.space[2],
  fontSize: vars.fontSize.sm,
});

export const prLink = style({
  display: "inline-flex",
  alignItems: "center",
  gap: vars.space[1],
  color: vars.color.primary,
  textDecoration: "none",
  selectors: {
    "&:hover": { textDecoration: "underline" },
  },
});

export const draftBadge = style({
  color: vars.color.textSecondary,
  fontWeight: vars.fontWeight.medium,
});

export const reviewCounts = style({
  display: "inline-flex",
  gap: vars.space[2],
});

export const approved = style({
  display: "inline-flex",
  alignItems: "center",
  gap: 2,
  color: vars.color.success,
});

export const changesRequested = style({
  display: "inline-flex",
  alignItems: "center",
  gap: 2,
  color: vars.color.errorText,
});

export const ciSuccess = style({ color: vars.color.success });
export const ciFailure = style({ color: vars.color.errorText });
export const ciPending = style({ color: vars.color.textSecondary });

export const captureFailed = style({ color: vars.color.warningText });
