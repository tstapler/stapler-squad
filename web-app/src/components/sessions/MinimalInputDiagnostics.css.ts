import { style } from "@vanilla-extract/css";
import { vars } from "../../styles/theme-contract.css";

export const panel = style({
  flexShrink: 0,
  boxSizing: "border-box",
  width: "100%",
  maxWidth: 640,
  maxHeight: "40vh",
  overflowY: "auto",
  padding: 12,
  background: vars.color.cardBackground,
  color: vars.color.textPrimary,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 8,
  fontSize: 14,
});

export const header = style({
  display: "flex",
  alignItems: "center",
  justifyContent: "space-between",
  gap: 8,
});

export const title = style({ margin: 0, fontSize: 16 });

export const meta = style({
  margin: "8px 0",
  display: "grid",
  gridTemplateColumns: "auto 1fr",
  gap: "2px 12px",
  wordBreak: "break-word",
});

export const events = style({
  margin: "8px 0",
  padding: 0,
  listStyle: "none",
  fontFamily: "monospace",
  fontSize: 13,
});

export const eventRow = style({
  display: "flex",
  gap: 12,
  padding: "2px 0",
  borderBottom: `1px solid ${vars.color.borderSubtle}`,
});

export const actions = style({ display: "flex", flexWrap: "wrap", gap: 8 });

export const reportBox = style({
  boxSizing: "border-box",
  width: "100%",
  minHeight: 96,
  font: "12px monospace",
  background: vars.color.inputBackground,
  color: vars.color.inputText,
});
