import { style } from "@vanilla-extract/css";
import { recipe } from "@vanilla-extract/recipes";
import { vars } from "@/styles/theme-contract.css";

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
      style: { background: vars.color.logError, color: vars.color.logOnDark, borderColor: vars.color.logError },
    },
    {
      variants: { level: "WARN", isActive: true },
      style: { background: vars.color.logWarn, color: vars.color.logOnAmber, borderColor: vars.color.logWarn },
    },
    {
      variants: { level: "INFO", isActive: true },
      style: { background: vars.color.logInfo, color: vars.color.logOnDark, borderColor: vars.color.logInfo },
    },
    {
      variants: { level: "DEBUG", isActive: true },
      style: { background: vars.color.logDebug, color: vars.color.logOnDark, borderColor: vars.color.logDebug },
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
