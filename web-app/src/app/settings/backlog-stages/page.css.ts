import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";
import {
  actionRowBase,
  badge,
  container,
  description,
  empty,
  errorMessageBase,
  formOverlay,
  headerRow,
  list,
  listItem,
  listItemDisabled,
  listItemInfo,
  listItemMeta,
  listItemName,
  listItemNameRow,
  listItemSlug,
  newBtn,
  smallBtn,
  title,
  toggle,
  toggleOn,
} from "@/styles/settingsListPage.css";

export {
  badge,
  container,
  description,
  empty,
  formOverlay,
  headerRow,
  list,
  listItem,
  listItemDisabled,
  listItemInfo,
  listItemMeta,
  listItemName,
  listItemNameRow,
  listItemSlug,
  newBtn,
  smallBtn,
  title,
  toggle,
  toggleOn,
};

export const actionRow = style([actionRowBase, { alignItems: "center" }]);

export const errorMessage = style([
  errorMessageBase,
  {
    display: "flex",
    alignItems: "center",
    justifyContent: "space-between",
    gap: vars.space["3"],
  },
]);

export const rowErrorMessage = style({
  color: vars.color.errorText,
  fontSize: vars.fontSize.xs,
  marginTop: vars.space["1"],
});

export const retryBtn = style({
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  border: `1px solid ${vars.color.error}`,
  background: "transparent",
  color: vars.color.errorText,
  flexShrink: 0,
});

// ---------------------------------------------------------------------------
// Engine-mode banner (Story 1.3.2). Full-width block; text wraps; actions
// stack under the text below 640px and sit inline at the right above it.
// ---------------------------------------------------------------------------

const bannerBase = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  overflowWrap: "anywhere",
  "@media": {
    "(min-width: 640px)": { flexDirection: "row", alignItems: "center", justifyContent: "space-between" },
  },
});

export const bannerNeutral = style([bannerBase, { background: vars.color.surfaceMuted, border: `1px solid ${vars.color.borderMuted}` }]);
export const bannerSuccess = style([bannerBase, { background: vars.color.successBg, border: `1px solid ${vars.color.success}` }]);
export const bannerWarning = style([bannerBase, { background: vars.color.warningBg, border: `1px solid ${vars.color.warning}` }]);

export const bannerBody = style({ display: "flex", flexDirection: "column", gap: vars.space["1"], minWidth: 0 });
export const bannerTextRow = style({ display: "flex", gap: vars.space["2"], alignItems: "flex-start" });
export const bannerIcon = style({ flexShrink: 0 });
export const bannerReview = style({ fontSize: vars.fontSize.xs, color: vars.color.textPrimary });
export const bannerReviewOverdue = style({ fontWeight: 600 });

export const bannerActions = style({ display: "flex", flexWrap: "wrap", gap: vars.space["2"], flexShrink: 0 });

export const bannerBtn = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  minHeight: "44px",
  padding: `${vars.space["1"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  cursor: "pointer",
  border: `1px solid ${vars.color.borderStrong}`,
  background: "transparent",
  color: vars.color.textPrimary,
  textDecoration: "underline",
});

// Visually hidden but still announced: the live region is empty until a change.
export const visuallyHidden = style({
  position: "absolute",
  width: "1px",
  height: "1px",
  margin: "-1px",
  padding: 0,
  overflow: "hidden",
  clip: "rect(0 0 0 0)",
  whiteSpace: "nowrap",
  border: 0,
});

export const saveStatus = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.successText,
  background: vars.color.successBg,
  borderRadius: vars.radii.sm,
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  selectors: { "&:empty": { display: "none" } },
});
