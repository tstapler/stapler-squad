import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "../../styles/theme.css";

export const root = style({
  width: "100%",
});

export const item = style({
  borderBottom: `1px solid ${vars.color.borderColor}`,
  selectors: {
    "&:last-child": {
      borderBottom: "none",
    },
  },
});

// Header wraps Accordion.Header/Trigger; sized to meet the >=44x44px touch
// target requirement (Story 1.1.1 acceptance criteria).
export const header = style({
  display: "flex",
  width: "100%",
  minHeight: "44px",
  alignItems: "center",
  justifyContent: "space-between",
  gap: vars.space["2"],
  padding: `${vars.space["3"]} ${vars.space["2"]}`,
  background: "transparent",
  border: "none",
  cursor: "pointer",
  color: vars.color.textPrimary,
  fontSize: vars.fontSize.base,
  fontWeight: vars.fontWeight.semibold,
  textAlign: "left",
  selectors: {
    "&:hover": {
      background: vars.color.hoverBackground,
    },
    "&:focus-visible": {
      outline: `2px solid ${vars.color.primary}`,
      outlineOffset: "-2px",
    },
  },
});

export const chevron = style({
  display: "inline-flex",
  alignItems: "center",
  justifyContent: "center",
  flexShrink: 0,
  transition: "transform 0.15s ease",
  color: vars.color.textMuted,
  selectors: {
    [`${header}[data-state="open"] &`]: {
      transform: "rotate(90deg)",
    },
  },
});

const slideDown = keyframes({
  from: { height: "0" },
  to: { height: "var(--radix-accordion-content-height)" },
});

const slideUp = keyframes({
  from: { height: "var(--radix-accordion-content-height)" },
  to: { height: "0" },
});

export const content = style({
  overflow: "hidden",
  selectors: {
    '&[data-state="open"]': {
      animation: `${slideDown} 0.15s ease-out`,
    },
    '&[data-state="closed"]': {
      animation: `${slideUp} 0.15s ease-out`,
    },
  },
});

export const contentInner = style({
  padding: `0 ${vars.space["2"]} ${vars.space["3"]}`,
});
