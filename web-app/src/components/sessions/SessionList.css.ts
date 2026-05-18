import { style, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  width: "100%",
  // Story 2.2.2: max-width removed — the session list now lives in a fixed 280px
  // column; constraining width further would waste space.
  padding: vars.space["4"],
  // Fill paneBody (flex column) and scroll when content overflows
  flex: 1,
  minHeight: 0,
  overflowY: "auto",
  "@media": {
    "(max-width: 768px)": {
      padding: vars.space["3"],
    },
  },
});

export const header = style({
  marginBottom: vars.space["6"],
});

export const headerTop = style({
  display: "flex",
  flexWrap: "wrap",
  justifyContent: "space-between",
  alignItems: "center",
  marginBottom: vars.space["4"],
  gap: vars.space["2"],
});

export const title = style({
  margin: 0,
  fontSize: "1.5rem",
  fontWeight: 700,
  color: vars.color.textPrimary,
  flexShrink: 0,
});

export const headerActions = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["3"],
});

export const selectModeButton = style({
  padding: `10px 20px`,
  borderRadius: vars.radii.md,
  fontSize: "0.875rem",
  fontWeight: 600,
  background: vars.color.cardBackground,
  border: `1px solid ${vars.color.borderColor}`,
  color: vars.color.textPrimary,
  cursor: "pointer",
  transition: "all 0.2s ease",
  whiteSpace: "nowrap",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.primary,
    },
  },
});

export const selectModeButtonActive = style({
  background: vars.color.primary,
  color: "white",
  borderColor: vars.color.primary,
});

export const filters = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["2"],
});

export const filterTopRow = style({
  display: "flex",
  gap: vars.space["2"],
  alignItems: "center",
  width: "100%",
});

export const filterToggle = style({
  display: "flex",
  alignItems: "center",
  gap: "6px",
  padding: `6px 12px`,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.lg,
  background: vars.color.inputBackground,
  color: vars.color.textPrimary,
  fontSize: "0.8125rem",
  fontWeight: 500,
  cursor: "pointer",
  whiteSpace: "nowrap",
  transition: "border-color 0.2s ease",
  flexShrink: 0,
  selectors: {
    "&:hover": { borderColor: vars.color.inputFocusBorder },
  },
});

export const filterToggleActive = style({
  borderColor: vars.color.primary,
  color: vars.color.primary,
});

export const filterActiveDot = style({
  display: "inline-block",
  width: "8px",
  height: "8px",
  background: vars.color.primary,
  borderRadius: vars.radii.full,
  flexShrink: 0,
});

export const filterControls = style({
  display: "none",
  flexDirection: "row",
  flexWrap: "wrap",
  gap: vars.space["2"],
  width: "100%",
});

export const filterControlsOpen = style({
  display: "flex",
});

export const searchInput = style({
  flex: 1,
  minWidth: 0,
  padding: `10px ${vars.space["4"]}`,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.lg,
  background: vars.color.inputBackground,
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
  transition: "border-color 0.2s ease",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.inputFocusBorder,
    },
  },
  "@media": {
    "(max-width: 768px)": {
      minWidth: "unset",
      flex: 1,
    },
  },
});

export const select = style({
  padding: `10px ${vars.space["4"]}`,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.lg,
  background: vars.color.inputBackground,
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
  cursor: "pointer",
  transition: "border-color 0.2s ease",
  selectors: {
    "&:focus": {
      outline: "none",
      borderColor: vars.color.inputFocusBorder,
    },
  },
  "@media": {
    "(max-width: 768px)": {
      width: "100%",
      minHeight: "44px",
    },
  },
});

export const sortDirButton = style({
  padding: `10px 14px`,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.lg,
  background: vars.color.inputBackground,
  color: vars.color.textPrimary,
  fontSize: "1rem",
  lineHeight: 1,
  cursor: "pointer",
  transition: "border-color 0.2s ease",
  whiteSpace: "nowrap",
  flexShrink: 0,
  selectors: {
    "&:hover": { borderColor: vars.color.inputFocusBorder },
  },
  "@media": {
    "(max-width: 768px)": {
      width: "100%",
      minHeight: "44px",
    },
  },
});

export const checkboxLabel = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `10px ${vars.space["4"]}`,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.lg,
  background: vars.color.inputBackground,
  cursor: "pointer",
  fontSize: "0.875rem",
  color: vars.color.textPrimary,
  transition: "border-color 0.2s ease",
  selectors: {
    "&:hover": { borderColor: vars.color.inputFocusBorder },
  },
  "@media": {
    "(max-width: 768px)": {
      width: "100%",
      minHeight: "44px",
    },
  },
});

globalStyle(`${checkboxLabel} input[type='checkbox']`, { cursor: "pointer" });

export const sessionList = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["6"],
});

export const categoryGroup = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const categoryTitle = style({
  margin: 0,
  padding: `${vars.space["2"]} 12px`,
  fontSize: "1rem",
  fontWeight: 600,
  color: vars.color.textPrimary,
  // Story 4.3: sticky group headers — stick to top of scrolling column
  position: "sticky",
  top: 0,
  zIndex: 10,
  background: vars.color.surfaceSubtle,
  borderLeft: `4px solid ${vars.color.primary}`,
  borderRadius: vars.radii.sm,
  // Subtle glow on group header accent border
  boxShadow: `inset 4px 0 0 ${vars.color.glowSecondary}`,
  cursor: "pointer",
  userSelect: "none",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
    },
  },
  transition: "background 0.15s ease",
});

export const categoryContent = style({
  display: "flex",
  flexDirection: "column",
  gap: vars.space["3"],
});

export const empty = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  padding: "48px 24px",
  textAlign: "center",
  color: vars.color.textSecondary,
});

globalStyle(`${empty} p`, { margin: `0 0 ${vars.space["4"]} 0`, fontSize: "1rem" });

export const clearButton = style({
  padding: `10px 24px`,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: vars.radii.lg,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
  fontWeight: 500,
  cursor: "pointer",
  transition: "all 0.2s ease",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
      borderColor: vars.color.borderHover,
    },
  },
});

export const emptyActions = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  gap: vars.space["4"],
  marginTop: vars.space["2"],
});

export const emptyHint = style({
  margin: 0,
  fontSize: "0.9375rem",
  color: vars.color.textSecondary,
});

export const newSessionHeaderButton = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  width: "32px",
  height: "32px",
  borderRadius: vars.radii.md,
  fontSize: "1.25rem",
  fontWeight: 400,
  lineHeight: 1,
  background: vars.color.primary,
  color: vars.color.textInverse,
  border: "none",
  cursor: "pointer",
  flexShrink: 0,
  transition: "background 0.15s, opacity 0.15s",
  selectors: {
    "&:hover": { background: vars.color.primaryHover, opacity: 0.9 },
    "&:active": { background: vars.color.primaryActive },
  },
});

export const newSessionButtonLarge = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `14px 28px`,
  borderRadius: vars.radii.lg,
  fontSize: "1rem",
  fontWeight: 600,
  color: vars.color.primaryText,
  background: vars.color.primary,
  textDecoration: "none",
  transition: "all 0.2s ease",
  boxShadow: `0 2px 4px ${vars.color.glowSecondary}`,
  border: "none",
  cursor: "pointer",
  selectors: {
    "&:hover": {
      background: vars.color.primaryHover,
      transform: "translateY(-2px)",
      boxShadow: `0 6px 12px ${vars.color.glowPrimary}`,
    },
    "&:active": { transform: "translateY(0)" },
  },
});

export const newSessionIcon = style({
  fontSize: "1.5rem",
  fontWeight: 400,
  lineHeight: 1,
});
