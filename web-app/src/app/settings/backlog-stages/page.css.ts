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
