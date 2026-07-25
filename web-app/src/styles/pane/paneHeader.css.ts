import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const paneHeader = style({
  minHeight: "32px",
  display: "flex",
  flexWrap: "wrap",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `${vars.space["1"]} ${vars.space["2"]}`,
  background: vars.color.cardBackground,
  borderBottom: `1px solid ${vars.color.borderColor}`,
  flexShrink: 0,
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
  userSelect: "none",
  cursor: "default",
});

export const paneTitle = style({
  flex: 1,
  minWidth: 0,
  overflow: "hidden",
  textOverflow: "ellipsis",
  whiteSpace: "nowrap",
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.xs,
});

// Groups all action buttons so they wrap together as a unit when the pane is too narrow
// to show both the session name and the buttons on one line.
export const paneHeaderActions = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  flexShrink: 0,
});

export const paneHeaderButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "20px",
  height: "20px",
  padding: 0,
  background: "transparent",
  border: "none",
  borderRadius: vars.radii.sm,
  color: vars.color.textMuted,
  cursor: "pointer",
  fontSize: vars.fontSize.xs,
  flexShrink: 0,
  transition: "background 100ms, color 100ms",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
});

export const paneCloseButton = style([
  paneHeaderButton,
  {
    selectors: {
      "&:hover": {
        background: vars.color.errorBg,
        color: vars.color.error,
      },
    },
  },
]);
