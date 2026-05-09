import { style, globalStyle } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const container = style({
  width: "100%",
  maxWidth: "1200px",
  margin: "0 auto",
  padding: vars.space["6"],
  "@media": {
    "(max-width: 768px)": {
      padding: vars.space["4"],
    },
  },
});

export const header = style({
  marginBottom: vars.space["6"],
});

export const headerTop = style({
  display: "flex",
  justifyContent: "space-between",
  alignItems: "center",
  marginBottom: vars.space["4"],
  gap: vars.space["4"],
});

export const title = style({
  margin: 0,
  fontSize: "1.5rem",
  fontWeight: 700,
  color: vars.color.textPrimary,
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
  gap: vars.space["3"],
  flexWrap: "wrap",
  alignItems: "center",
  "@media": {
    "(max-width: 768px)": {
      flexDirection: "column",
      alignItems: "stretch",
      gap: vars.space["2"],
    },
  },
});

export const filterTopRow = style({
  // On desktop, children participate in the parent flex
  display: "contents",
  "@media": {
    "(max-width: 768px)": {
      display: "flex",
      gap: vars.space["2"],
      width: "100%",
    },
  },
});

export const filterToggle = style({
  display: "none",
  alignItems: "center",
  gap: "6px",
  padding: `10px ${vars.space["4"]}`,
  border: `1px solid ${vars.color.inputBorder}`,
  borderRadius: vars.radii.lg,
  background: vars.color.inputBackground,
  color: vars.color.textPrimary,
  fontSize: "0.875rem",
  fontWeight: 500,
  cursor: "pointer",
  minHeight: "44px",
  whiteSpace: "nowrap",
  transition: "border-color 0.2s ease",
  selectors: {
    "&:hover": { borderColor: vars.color.inputFocusBorder },
  },
  "@media": {
    "(max-width: 768px)": {
      display: "flex",
    },
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
  display: "contents",
  "@media": {
    "(max-width: 768px)": {
      display: "none",
      flexDirection: "column",
      gap: vars.space["2"],
      width: "100%",
    },
  },
});

export const filterControlsOpen = style({
  "@media": {
    "(max-width: 768px)": {
      display: "flex",
    },
  },
});

export const searchInput = style({
  flex: 1,
  minWidth: "250px",
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
  background: vars.color.surfaceSubtle,
  borderLeft: `4px solid ${vars.color.primary}`,
  borderRadius: vars.radii.sm,
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

export const newSessionButtonLarge = style({
  display: "flex",
  alignItems: "center",
  gap: vars.space["2"],
  padding: `14px 28px`,
  borderRadius: vars.radii.lg,
  fontSize: "1rem",
  fontWeight: 600,
  color: "white",
  background: vars.color.primary,
  textDecoration: "none",
  transition: "all 0.2s ease",
  boxShadow: "0 2px 4px rgba(0, 102, 204, 0.2)",
  border: "none",
  cursor: "pointer",
  selectors: {
    "&:hover": {
      background: vars.color.primaryHover,
      transform: "translateY(-2px)",
      boxShadow: "0 6px 12px rgba(0, 102, 204, 0.3)",
    },
    "&:active": { transform: "translateY(0)" },
  },
});

export const newSessionIcon = style({
  fontSize: "1.5rem",
  fontWeight: 400,
  lineHeight: 1,
});
