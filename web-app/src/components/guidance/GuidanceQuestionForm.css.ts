import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

// design/ux.md 3.1: the mobile layout is everything below 900px.
const narrow = "screen and (max-width: 899px)";

export const form = style({ display: "flex", flexDirection: "column", gap: vars.space["3"] });

export const group = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  border: "none",
  margin: 0,
  padding: 0,
  minWidth: 0,
});

export const groupLabel = style({
  margin: 0,
  padding: 0,
  fontSize: vars.fontSize.base,
  fontWeight: 600,
  color: vars.color.textPrimary,
  overflowWrap: "anywhere",
});

export const optionList = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space["2"],
  "@media": { [narrow]: { flexDirection: "column", flexWrap: "nowrap" } },
});

export const choiceRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  minHeight: "44px",
  fontSize: vars.fontSize.sm,
  color: vars.color.textPrimary,
  overflowWrap: "anywhere",
});

export const choiceInput = style({ width: "20px", height: "20px", flexShrink: 0 });

export const description = style({ fontSize: vars.fontSize.xs, color: vars.color.textSecondary });

export const fieldError = style({ margin: 0, fontSize: vars.fontSize.sm, color: vars.color.errorText });

export const helper = style({ margin: 0, fontSize: vars.fontSize.sm, color: vars.color.textSecondary });

export const selectField = style({
  fontSize: "16px", // stops iOS Safari zooming the page on focus
  minHeight: "44px",
  padding: "0.5rem",
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  maxWidth: "100%",
});

export const otherRow = style({ display: "flex", gap: vars.space["2"], alignItems: "center" });

export const otherField = style({
  flex: 1,
  minWidth: 0,
  fontSize: "16px",
  minHeight: "44px",
  padding: "0.5rem",
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
});

export const submitRow = style({
  display: "flex",
  flexWrap: "wrap",
  gap: vars.space["2"],
  alignItems: "center",
  "@media": { [narrow]: { position: "sticky", bottom: 0, padding: "8px 0", background: vars.color.cardBackground } },
});

export const deliveryLine = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
  margin: 0,
});

export const deliveryText = style({ margin: 0, fontSize: vars.fontSize.sm, color: vars.color.textSecondary, overflowWrap: "anywhere" });

export const chip = style({
  alignSelf: "flex-start",
  fontSize: vars.fontSize.xs,
  fontWeight: 600,
  padding: `2px ${vars.space["2"]}`,
  borderRadius: vars.radii.sm,
  background: vars.color.warningBg,
  color: vars.color.warningText,
  border: `1px solid ${vars.color.warning}`,
  "@media": { "(forced-colors: active)": { borderColor: "CanvasText" } },
});

export const textOption = style({ margin: 0, fontSize: vars.fontSize.sm, color: vars.color.textPrimary, overflowWrap: "anywhere" });
export const textList = style({ margin: 0, paddingLeft: vars.space["4"], display: "flex", flexDirection: "column", gap: vars.space["1"] });

export const toggleButton = style({
  selectors: {
    '&[aria-pressed="true"]': { background: vars.color.primary, color: vars.color.background, borderColor: vars.color.primary },
  },
  "@media": {
    // A fill is not a signal in forced-colors mode; the system highlight is.
    "(forced-colors: active)": { selectors: { '&[aria-pressed="true"]': { background: "Highlight", color: "HighlightText", borderColor: "Highlight" } } },
  },
});
