import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  display: "flex",
  flexDirection: "column",
  height: "var(--viewport-height, 100dvh)",
  overflow: "hidden",
  padding: "1.5rem",
  backgroundColor: vars.color.background,
  color: vars.color.textPrimary,
  fontFamily: vars.font.mono,
});

export const header = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  marginBottom: "1.5rem",
});

export const headerActions = style({
  display: "flex",
  alignItems: "center",
  gap: "1rem",
});

export const timezone = style({
  padding: "0.5rem 1rem",
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.sm,
  color: vars.color.textMuted,
  cursor: "help",
});

export const refreshButton = style({
  padding: "0.5rem 1rem",
  backgroundColor: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  color: vars.color.textPrimary,
  cursor: "pointer",
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.base,
  transition: "background-color 0.2s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
    },
  },
});

export const filters = style({
  display: "flex",
  gap: "1rem",
  marginBottom: "1.5rem",
  padding: "1rem",
  backgroundColor: vars.color.cardBackground,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
  flexWrap: "wrap",
});

export const filterGroup = style({
  display: "flex",
  alignItems: "center",
  gap: "0.5rem",
});

export const select = style({
  padding: "0.5rem",
  backgroundColor: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  color: vars.color.inputText,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.base,
  cursor: "pointer",
  fontFamily: vars.font.mono,
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.inputFocusBorder,
    },
  },
});

export const viewTab = style({
  padding: "0.4rem 0.9rem",
  backgroundColor: "transparent",
  border: `1px solid ${vars.color.borderColor}`,
  color: vars.color.textMuted,
  cursor: "pointer",
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.base,
  fontFamily: vars.font.mono,
  transition: "background-color 0.15s, color 0.15s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.hoverBackground,
      color: vars.color.textPrimary,
    },
  },
});

export const viewTabActive = style([
  viewTab,
  {
    backgroundColor: vars.color.primary,
    borderColor: vars.color.primary,
    color: vars.color.background,
    selectors: {
      "&:hover": {
        backgroundColor: vars.color.primaryHover,
      },
    },
  },
]);

export const error = style({
  padding: "2rem",
  textAlign: "center",
  backgroundColor: vars.color.cardBackground,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.error}`,
  color: vars.color.error,
});

export const logsContainer = style({
  flex: 1,
  // overflow: hidden required so react-virtuoso's ResizeObserver can measure
  // the scroll container height. The VirtualLogList manages its own overflow.
  overflow: "hidden",
  minHeight: 0,
  display: "flex",
  flexDirection: "column",
  backgroundColor: vars.color.background,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
});

export const footer = style({
  marginTop: "1rem",
  padding: "0.75rem 1rem",
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  backgroundColor: vars.color.cardBackground,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
  fontSize: vars.fontSize.xs,
  color: vars.color.textMuted,
});

export const shortcuts = style({
  display: "flex",
  gap: "0.75rem",
  alignItems: "center",
  fontSize: vars.fontSize.xs,
  color: vars.color.textSecondary,
});

// viewPane toggles the Table view's visibility while switching to Patterns —
// "contents" (not "none") when shown, so LogViewer's own layout box doesn't
// gain an extra wrapper in the flex/grid flow; see page.tsx's view-mode toggle.
export const viewPane = style({
  display: "contents",
  selectors: {
    '&[data-hidden="true"]': {
      display: "none",
    },
  },
});
