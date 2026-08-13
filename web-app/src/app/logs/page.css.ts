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

export const searchInput = style({
  padding: "0.5rem",
  backgroundColor: vars.color.inputBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  color: vars.color.inputText,
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.base,
  minWidth: "300px",
  fontFamily: vars.font.mono,
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.inputFocusBorder,
    },
  },
});

export const searchButton = style({
  padding: "0.5rem 1rem",
  backgroundColor: vars.color.hoverBackground,
  border: `1px solid ${vars.color.inputBorder}`,
  color: vars.color.textPrimary,
  cursor: "pointer",
  borderRadius: vars.radii.sm,
  fontSize: vars.fontSize.base,
  transition: "background-color 0.2s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.panelBgSecondary,
    },
  },
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

export const loading = style({
  padding: "2rem",
  textAlign: "center",
  backgroundColor: vars.color.cardBackground,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
});

export const error = style({
  padding: "2rem",
  textAlign: "center",
  backgroundColor: vars.color.cardBackground,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.error}`,
  color: vars.color.error,
});

export const noLogs = style({
  padding: "3rem 2rem",
  textAlign: "center",
  backgroundColor: vars.color.cardBackground,
  borderRadius: vars.radii.md,
  border: `1px solid ${vars.color.borderColor}`,
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

export const logsTable = style({
  width: "100%",
  borderCollapse: "collapse",
  fontSize: vars.fontSize.xs,
});

export const timestampColumn = style({
  width: "200px",
});

export const levelColumn = style({
  width: "100px",
});

export const sourceColumn = style({
  width: "200px",
});

export const messageColumn = style({
  flex: 1,
});

export const logRow = style({
  borderBottom: `1px solid ${vars.color.cardBackground}`,
  transition: "background-color 0.1s",
  selectors: {
    "&:hover": {
      backgroundColor: vars.color.cardBackground,
    },
  },
});

export const logRowExpanded = style({
  backgroundColor: vars.color.cardBackground,
});

export const timestamp = style({
  padding: "0.75rem",
  color: vars.color.textMuted,
  whiteSpace: "nowrap",
});

export const level = style({
  padding: "0.75rem",
  fontWeight: 600,
  textTransform: "uppercase",
  whiteSpace: "nowrap",
});

export const levelDebug = style({ color: vars.color.textMuted });
export const levelInfo = style({ color: vars.color.primary });
export const levelWarning = style({ color: vars.color.warning });
export const levelError = style({ color: vars.color.error });
export const levelFatal = style({ color: vars.color.errorDark, fontWeight: 700 });

export const source = style({
  padding: "0.75rem",
  color: vars.color.textSecondary,
  fontFamily: vars.font.mono,
  fontSize: vars.fontSize.xs,
});

export const message = style({
  padding: "0.75rem",
  color: vars.color.textPrimary,
  wordBreak: "break-word",
  lineHeight: "1.4",
});

export const loadingMore = style({
  padding: "1.5rem",
  textAlign: "center",
  color: vars.color.textMuted,
  fontSize: vars.fontSize.base,
  backgroundColor: vars.color.cardBackground,
  borderTop: `1px solid ${vars.color.borderColor}`,
});

export const endOfLogs = style({
  padding: "1.5rem",
  textAlign: "center",
  color: vars.color.textSecondary,
  fontSize: vars.fontSize.xs,
  backgroundColor: vars.color.background,
  borderTop: `1px solid ${vars.color.borderColor}`,
  fontStyle: "italic",
});

export const searchWrapper = style({
  position: "relative",
  display: "flex",
  alignItems: "center",
});

export const clearSearch = style({
  position: "absolute",
  right: "8px",
  background: "none",
  border: "none",
  color: vars.color.textSecondary,
  cursor: "pointer",
  fontSize: "1.2rem",
  lineHeight: "1",
  padding: "0.25rem",
  transition: "color 0.15s",
  selectors: {
    "&:hover": {
      color: vars.color.error,
    },
  },
});

export const expandColumn = style({
  width: "40px",
  textAlign: "center",
});

export const expandCell = style({
  textAlign: "center",
  padding: "0.5rem",
});

export const expandButton = style({
  background: "none",
  border: "none",
  color: vars.color.textSecondary,
  cursor: "pointer",
  fontSize: vars.fontSize.xs,
  padding: "0.25rem",
  transition: "color 0.15s",
  selectors: {
    "&:hover": {
      color: vars.color.primary,
    },
  },
});

export const actionsColumn = style({
  width: "50px",
  textAlign: "center",
});

export const actionsCell = style({
  textAlign: "center",
  padding: "0.5rem",
});

export const actionButton = style({
  background: "none",
  border: "none",
  cursor: "pointer",
  fontSize: vars.fontSize.base,
  padding: "0.25rem",
  opacity: 0.5,
  transition: "opacity 0.15s",
  selectors: {
    "&:hover": {
      opacity: 1,
    },
  },
});

export const clickable = style({
  cursor: "pointer",
  position: "relative",
  selectors: {
    "&:hover": {
      textDecoration: "underline",
    },
  },
});

export const filterIcon = style({
  fontSize: "0.7rem",
  marginLeft: "0.25rem",
  opacity: 0,
  transition: "opacity 0.15s",
});

export const expandedRow = style({});

export const logDetail = style({
  padding: "1rem 1.5rem",
});

export const logDetailSection = style({
  marginBottom: "0.75rem",
  display: "flex",
  gap: "0.75rem",
});

export const logDetailMessage = style({
  margin: 0,
  padding: "0.75rem",
  backgroundColor: vars.color.background,
  borderRadius: vars.radii.sm,
  whiteSpace: "pre-wrap",
  wordBreak: "break-word",
  fontSize: vars.fontSize.xs,
  lineHeight: "1.5",
  overflowX: "auto",
  maxHeight: "300px",
  overflowY: "auto",
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

export const liveTailStatus = style({
  color: vars.color.primary,
  fontWeight: 500,
});

export const searchHistoryWrapper = style({
  minWidth: "300px",
});

export const densityCompact = style({});
export const densityComfortable = style({});
export const densitySpacious = style({});
