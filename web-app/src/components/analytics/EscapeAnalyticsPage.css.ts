import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const page = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["6"],
  padding: vars.space["6"],
  maxWidth: "1200px",
  margin: "0 auto",
  width: "100%",
  "@media": {
    "screen and (max-width: 768px)": {
      padding: vars.space["4"],
      gap: vars.space["4"],
    },
  },
});

export const header = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const title = style({
  fontSize: vars.fontSize.xl,
  fontWeight: vars.fontWeight.bold,
  color: vars.color.textPrimary,
  margin: 0,
});

export const subtitle = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  margin: 0,
});

// Tab toggle ("Per-Session" / "All Sessions") — same visual language (underline
// on the active tab) as SessionDetail.css.ts's `tabs`/`tab`/`active` trio, but
// this page owns its own token references per .claude/rules/css-architecture.md.
export const tablist = style({
  display: "flex",
  gap: vars.space["2"],
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const tab = style({
  display: "flex",
  alignItems: "center",
  padding: `${vars.space["2"]} ${vars.space["4"]}`,
  border: "none",
  background: "transparent",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  cursor: "pointer",
  borderBottom: "2px solid transparent",
  transition: "color 0.2s, border-color 0.2s",
  selectors: {
    "&:hover": {
      color: vars.color.textPrimary,
    },
    '&[aria-selected="true"]': {
      color: vars.color.primary,
      borderBottomColor: vars.color.primary,
      fontWeight: vars.fontWeight.semibold,
    },
  },
  ":focus-visible": {
    outline: `2px solid ${vars.color.inputFocusBorder}`,
    outlineOffset: "2px",
  },
});

export const tabpanel = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["4"],
});

export const sessionSelectorRow = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
  flexWrap: "wrap",
});

export const selectorLabel = style({
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  color: vars.color.textSecondary,
  flexShrink: 0,
});

export const sessionSelect = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.inputBorder}`,
  backgroundColor: vars.color.inputBackground,
  color: vars.color.inputText,
  fontSize: vars.fontSize.sm,
  minWidth: "240px",
  ":focus": {
    outline: "none",
    borderColor: vars.color.inputFocusBorder,
  },
});

export const grid = style({
  display: "grid",
  gridTemplateColumns: "1fr 1fr",
  gap: vars.space["4"],
  "@media": {
    "screen and (max-width: 900px)": {
      gridTemplateColumns: "1fr",
    },
  },
});

export const card = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
  padding: vars.space["4"],
  borderRadius: vars.radii.lg,
  border: `1px solid ${vars.color.borderColor}`,
  backgroundColor: vars.color.cardBackground,
});

export const cardTitle = style({
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.semibold,
  color: vars.color.textPrimary,
  margin: 0,
});

export const fullWidthCard = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
  padding: vars.space["4"],
  borderRadius: vars.radii.lg,
  border: `1px solid ${vars.color.borderColor}`,
  backgroundColor: vars.color.cardBackground,
});

export const filterRow = style({
  display: "flex",
  gap: vars.space["3"],
  alignItems: "center",
  flexWrap: "wrap",
});

export const filterInput = style({
  padding: `${vars.space["2"]} ${vars.space["3"]}`,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.inputBorder}`,
  backgroundColor: vars.color.inputBackground,
  color: vars.color.inputText,
  fontSize: vars.fontSize.sm,
  ":focus": {
    outline: "none",
    borderColor: vars.color.inputFocusBorder,
  },
});

export const filterLabel = style({
  fontSize: vars.fontSize.sm,
  color: vars.color.textSecondary,
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
});

export const errorBanner = style({
  padding: vars.space["3"],
  borderRadius: vars.radii.md,
  backgroundColor: vars.color.errorBg,
  color: vars.color.errorText,
  fontSize: vars.fontSize.sm,
  border: `1px solid ${vars.color.error}`,
});

export const loadingText = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  padding: vars.space["4"],
  textAlign: "center",
});

export const noSessionMessage = style({
  color: vars.color.textMuted,
  fontSize: vars.fontSize.sm,
  padding: vars.space["8"],
  textAlign: "center",
});
