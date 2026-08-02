import { style } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme-contract.css";

// Task 4.2.1.2 — a drop is a warning-severity event, not a hard error, so
// this reuses the warning token trio rather than a bespoke color.
export const badge = style({
  position: "absolute",
  top: 8,
  left: "50%",
  transform: "translateX(-50%)",
  zIndex: zIndex.floatingTerminalUI,
  display: "inline-flex",
  alignItems: "center",
  gap: 6,
  padding: "4px 10px",
  borderRadius: 12,
  fontSize: 12,
  fontWeight: 600,
  background: vars.color.warningBg,
  color: vars.color.warningText,
  border: `1px solid ${vars.color.warning}`,
  boxShadow: "0 2px 6px rgba(0, 0, 0, 0.15)",
  pointerEvents: "none",
  whiteSpace: "nowrap",
});

export const icon = style({
  width: 14,
  height: 14,
  flexShrink: 0,
});

export const text = style({
  whiteSpace: "nowrap",
});
