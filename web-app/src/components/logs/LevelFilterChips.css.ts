import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";

export const chipRow = style({
  display: "flex",
  gap: 4,
  overflowX: "auto",
  flexWrap: "nowrap",
  scrollbarWidth: "none",
  WebkitOverflowScrolling: "touch",
  selectors: {
    "&::-webkit-scrollbar": {
      display: "none",
    },
  },
});

export const chip = recipe({
  base: {
    border: "1px solid rgba(255,255,255,0.15)",
    borderRadius: 12,
    padding: "2px 10px",
    minHeight: 44,
    fontSize: 11,
    fontWeight: 600,
    cursor: "pointer",
    background: "transparent",
    color: "inherit",
    whiteSpace: "nowrap",
    flexShrink: 0,
    display: "inline-flex",
    alignItems: "center",
    justifyContent: "center",
  },
  variants: {
    level: {
      ALL: {},
      ERROR: {},
      WARN: {},
      INFO: {},
      DEBUG: {},
    },
    isActive: {
      true: {},
      false: {},
    },
  },
  compoundVariants: [
    {
      variants: { level: "ERROR", isActive: true },
      style: { background: "#B91C1C", color: "#fff", borderColor: "#B91C1C" },
    },
    {
      variants: { level: "WARN", isActive: true },
      style: { background: "#D97706", color: "#1A1A1A", borderColor: "#D97706" },
    },
    {
      variants: { level: "INFO", isActive: true },
      style: { background: "#1D4ED8", color: "#fff", borderColor: "#1D4ED8" },
    },
    {
      variants: { level: "DEBUG", isActive: true },
      style: { background: "#6B7280", color: "#fff", borderColor: "#6B7280" },
    },
    {
      variants: { level: "ALL", isActive: true },
      style: {
        background: "rgba(255,255,255,0.15)",
        borderColor: "rgba(255,255,255,0.3)",
      },
    },
  ],
  defaultVariants: { level: "ALL", isActive: false },
});
