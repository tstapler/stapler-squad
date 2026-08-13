import { style, keyframes } from "@vanilla-extract/css";
import { vars } from "@/styles/theme.css";

const shimmerFrames = keyframes({
  "0%": { backgroundPosition: "200% 0" },
  "100%": { backgroundPosition: "-200% 0" },
});

export const skeleton = style({
  background: `linear-gradient(90deg, ${vars.color.borderSubtle} 25%, ${vars.color.borderMuted} 50%, ${vars.color.borderSubtle} 75%)`,
  backgroundSize: "200% 100%",
  animation: `${shimmerFrames} 1.5s infinite`,
  "@media": {
    "(prefers-color-scheme: dark)": {
      background: `linear-gradient(90deg, ${vars.color.surfaceMuted} 25%, ${vars.color.borderMuted} 50%, ${vars.color.surfaceMuted} 75%)`,
      backgroundSize: "200% 100%",
    },
  },
});

export const rectangular = style({
  borderRadius: "4px",
});

export const circular = style({
  borderRadius: "50%",
});

export const text = style({
  borderRadius: "4px",
  height: "1em",
});
