import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme-contract.css";

export const overlay = style({
  position: "fixed",
  inset: 0,
  background: "rgba(0, 0, 0, 0.5)",
  display: "flex",
  alignItems: "center",
  justifyContent: "center",
  zIndex: 1000,
});

export const dialog = style({
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  borderRadius: 8,
  padding: "1.5rem",
  maxWidth: 420,
  width: "90%",
});

export const dialogActions = style({
  display: "flex",
  justifyContent: "flex-end",
  gap: "0.75rem",
  marginTop: "1rem",
});

export const warningNotice = style({
  background: "rgba(234, 179, 8, 0.12)",
  color: vars.color.textPrimary,
  padding: "8px 12px",
  borderRadius: 6,
});
