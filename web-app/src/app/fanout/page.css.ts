import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const page = style({ display: "flex", flexDirection: "column", gap: 12, padding: 16, height: "100%", minHeight: 0 });
export const header = style({ display: "flex", alignItems: "center", gap: 12, justifyContent: "space-between" });
export const grid = style({
  display: "grid",
  gridAutoFlow: "column",
  gridAutoColumns: "minmax(320px, 1fr)",
  gap: 12,
  flex: 1,
  minHeight: 0,
  overflowX: "auto",
});
export const column = style({
  display: "flex",
  flexDirection: "column",
  gap: 8,
  minHeight: 0,
  border: `1px solid ${vars.color.borderColor}`,
  borderRadius: 8,
  padding: 8,
});
export const columnHeader = style({ display: "flex", alignItems: "center", justifyContent: "space-between", gap: 8 });
export const diffPane = style({ flex: 1, minHeight: 0, overflow: "auto" });
export const badge = style({ fontSize: 12, padding: "2px 8px", borderRadius: 999, border: `1px solid ${vars.color.borderColor}` });
export const errorText = style({ color: vars.color.error });
