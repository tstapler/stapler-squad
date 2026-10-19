"use client";

import type { CSSProperties, Ref } from "react";
import { actionButton } from "./ActionRow.css";

interface ActionButtonProps {
  id: string;
  label: string;
  onClick: () => void;
  variant?: string;
  /** Rendered `aria-disabled` (stays focusable) and described by `reasonId`. */
  disabled?: boolean;
  reasonId?: string;
  minHeight?: number;
  style?: CSSProperties;
  buttonRef?: Ref<HTMLButtonElement>;
}

/** The row's button: `aria-disabled` rather than `disabled`, so the reason stays reachable. */
export function ActionButton({ id, label, onClick, variant = "", disabled, reasonId, minHeight, style, buttonRef }: ActionButtonProps) {
  return (
    // analytics-exempt
    <button
      ref={buttonRef}
      type="button"
      className={`${actionButton} ${variant}`}
      data-testid={`action-${id}`}
      aria-disabled={disabled ? true : undefined}
      aria-describedby={disabled ? reasonId : undefined}
      style={{ minHeight, ...style }}
      onClick={() => {
        if (!disabled) onClick();
      }}
    >
      {label}
    </button>
  );
}
