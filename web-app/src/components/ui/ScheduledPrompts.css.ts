import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const sectionHead = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space[2],
});

export const confirmBox = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[2],
  padding: "8px 0",
});

export const inlineMessage = style({
  margin: 0,
  fontSize: "0.875rem",
  color: vars.color.textPrimary,
  overflowWrap: "anywhere",
});

export const errorMessage = style({
  margin: 0,
  fontSize: "0.875rem",
  color: vars.color.error,
  overflowWrap: "anywhere",
});

export const answerField = style({
  width: "100%",
  minHeight: "88px",
  padding: "8px",
  borderRadius: "8px",
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  font: "inherit",
  fontSize: "16px", // UX-AC44: below 16px iOS Safari zooms the page on focus
  boxSizing: "border-box",
  selectors: { "&:focus-visible": { outline: `2px solid ${vars.color.primary}`, outlineOffset: "2px" } },
});

/** The Remind me time picker: a single-line field, still a 44px target (UX-AC42). */
export const pickField = style([answerField, { minHeight: "44px" }]);

export const fieldLabel = style({ fontSize: "0.8125rem", color: vars.color.textSecondary });

export const pausedBanner = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space[2],
  padding: "10px 12px",
  border: `1px solid ${vars.color.warning}`,
  borderRadius: "8px",
  background: vars.color.cardBackground,
});

export const menuList = style({
  listStyle: "none",
  margin: 0,
  padding: 0,
  display: "flex",
  flexDirection: "column",
  gap: vars.space[1],
});

export const menuItem = style({ width: "100%", justifyContent: "flex-start" });
