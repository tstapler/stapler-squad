import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// In normal flow (not a floating overlay) so a mobile keyboard cannot cover it.
export const listbox = style({
  listStyle: "none",
  margin: `${vars.space["1"]} 0 0`,
  padding: 0,
  maxHeight: "min(40vh, 352px)",
  overflowY: "auto",
  backgroundColor: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: "4px",
});

export const option = style({
  minHeight: "44px",
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  display: "flex",
  flexDirection: "column",
  justifyContent: "center",
  cursor: "pointer",
  color: vars.color.inputText,
  fontSize: "0.875rem",
  selectors: {
    '&[aria-selected="true"]': { backgroundColor: vars.color.hoverBackground },
  },
});

export const optionName = style({ fontFamily: "monospace" });

export const optionValue = style({ color: vars.color.textSecondary });

export const optionDescription = style({
  color: vars.color.textSecondary,
  fontSize: "0.8125rem",
});
