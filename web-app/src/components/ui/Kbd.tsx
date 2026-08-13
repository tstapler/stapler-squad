import { kbd } from "./Kbd.css";

interface KbdProps {
  children: React.ReactNode;
  size?: "sm" | "md";
}

/**
 * Styled keyboard key component.
 * Uses theme tokens (vars.color.primary, vars.font.mono) so it adapts
 * automatically to any active vanilla-extract theme.
 */
export function Kbd({ children, size = "md" }: KbdProps) {
  return <kbd className={kbd({ size })}>{children}</kbd>;
}
