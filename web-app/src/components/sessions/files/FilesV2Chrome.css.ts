import { style } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

export const chrome = style({ position: "relative", flexShrink: 0 });

export const navRow = style({
  display: "flex",
  alignItems: "center",
  gap: "4px",
  padding: "2px 8px",
  borderBottom: `1px solid ${vars.color.borderColor}`,
  background: vars.color.cardBackground,
});

export const navButton = style({
  border: "none",
  background: "transparent",
  color: vars.color.textSecondary,
  borderRadius: vars.radii.sm,
  padding: "2px 8px",
  cursor: "pointer",
  fontSize: "13px",
  ":hover": { background: vars.color.hoverBackground },
  selectors: { "&[aria-disabled='true']": { opacity: 0.4, cursor: "default" } },
  "@media": { "(pointer: coarse)": { minHeight: "44px", minWidth: "44px" } },
});

export const spacer = style({ flex: 1 });

export const tabPanel = style({ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" });
