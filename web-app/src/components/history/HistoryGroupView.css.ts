import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const loadingContainer = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  padding: "60px 20px",
  textAlign: "center",
});

export const loadingTitle = style({
  fontSize: "18px",
  fontWeight: "600",
  margin: "20px 0 10px",
  color: vars.color.textPrimary,
});

export const emptyStateContainer = style({
  display: "flex",
  flexDirection: "column",
  alignItems: "center",
  justifyContent: "center",
  padding: "60px 20px",
  textAlign: "center",
  background: vars.color.cardBackground,
  border: `2px dashed ${vars.color.borderColor}`,
  borderRadius: "8px",
});

export const emptyStateIcon = style({
  fontSize: "48px",
  marginBottom: "16px",
});

export const emptyStateTitle = style({
  fontSize: "18px",
  fontWeight: "600",
  color: vars.color.textPrimary,
  margin: "0 0 8px 0",
});

export const linkButton = style({
  background: "none",
  border: "none",
  color: vars.color.primary,
  cursor: "pointer",
  fontSize: "inherit",
  padding: 0,
  textDecoration: "underline",

  selectors: {
    "&:hover": {
      color: vars.color.primaryHover,
    },
  },
});

export const entryCards = style({
  display: "flex",
  flexDirection: "column",
  gap: "16px",
  maxHeight: "calc(var(--viewport-height, 100dvh) - 320px)",
  overflowY: "auto",
  paddingRight: "8px",

  "@media": {
    "(max-width: 1024px)": {
      maxHeight: "400px",
    },
  },
});

export const categoryGroup = style({
  marginBottom: "8px",
});

export const categoryTitle = style({
  fontSize: "13px",
  fontWeight: "600",
  color: vars.color.textSecondary,
  textTransform: "uppercase",
  letterSpacing: "0.5px",
  margin: "0 0 10px 0",
  paddingBottom: "6px",
  borderBottom: `1px solid ${vars.color.borderColor}`,
});

export const categoryContent = style({
  display: "flex",
  flexDirection: "column",
  gap: "8px",
});
