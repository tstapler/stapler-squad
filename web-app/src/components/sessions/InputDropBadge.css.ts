import { style, keyframes } from "@vanilla-extract/css";
import { vars, zIndex } from "@/styles/theme.css";

// Modeled on XtermTerminal.css.ts's `copiedToast` — same fixed-position,
// bottom-center, floating-terminal-UI stacking slot (design/ux.md §2.1).
const fadeIn = keyframes({
  "0%": { opacity: 0, transform: "translateX(-50%) translateY(4px)" },
  "100%": { opacity: 1, transform: "translateX(-50%) translateY(0)" },
});

export const badge = style({
  position: "fixed",
  bottom: "80px",
  left: "50%",
  transform: "translateX(-50%)",
  zIndex: zIndex.floatingTerminalUI,
  padding: `${vars.space[1]} ${vars.space[3]}`,
  background: vars.color.warningBg,
  color: vars.color.warningText,
  borderRadius: vars.radii.full,
  fontSize: vars.fontSize.sm,
  fontWeight: vars.fontWeight.medium,
  boxShadow: "0 2px 8px rgba(0,0,0,0.3)",
  // Never a click/drag target (design/ux.md §2.1, UX-AC-10) — clicks pass
  // straight through to the terminal underneath.
  pointerEvents: "none",
  animation: `${fadeIn} 150ms ease-out`,
});
