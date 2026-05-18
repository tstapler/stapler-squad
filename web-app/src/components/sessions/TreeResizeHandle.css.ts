import { style } from "@vanilla-extract/css";
import { vars, breakpoints } from "../../styles/theme.css";

export const hitTarget = style({
  width: "8px",
  marginLeft: "-4px",
  marginRight: "-4px",
  flexShrink: 0,
  cursor: "col-resize",
  background: "transparent",
  transition: "background 0.15s",
  position: "relative",
  zIndex: 1,
  "::after": {
    content: '""',
    position: "absolute",
    top: 0,
    bottom: 0,
    left: "50%",
    transform: "translateX(-50%)",
    width: "2px",
    background: vars.color.borderColor,
    opacity: 0,
    transition: "opacity 0.15s",
  },
  selectors: {
    "&:hover::after": {
      opacity: 1,
    },
    "&:active::after": {
      opacity: 1,
    },
  },
  "@media": {
    [`(max-width: ${breakpoints.md})`]: {
      display: "none",
    },
  },
});
