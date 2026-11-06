"use client";

import { useState, type CSSProperties, type Ref } from "react";
import { useEngagement } from "@/lib/hooks/useEngagedCard";
import { getTiming } from "@/lib/hooks/interactionTiming";
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
  /** One of an answer or decision group: a digit key moves focus to the Nth of these. */
  option?: boolean;
}

/** The row's button: `aria-disabled` rather than `disabled`, so the reason stays reachable. */
export function ActionButton({ id, label, onClick, variant = "", disabled, reasonId, minHeight, style, buttonRef, option }: ActionButtonProps) {
  const tracker = useEngagement();
  const [mountedAt] = useState(() => Date.now());
  // A control that just rendered or moved ignores activation, so a tap aimed at its
  // previous occupant cannot trigger it (design/ux.md 7.1, UX-AC62).
  const settled = () => (tracker ? tracker.isLayoutSettled(mountedAt) : Date.now() - mountedAt >= getTiming().settleMs);
  return (
    // analytics-exempt
    <button
      ref={buttonRef}
      type="button"
      className={`${actionButton} ${variant}`}
      data-testid={`action-${id}`}
      aria-disabled={disabled ? true : undefined}
      aria-describedby={disabled ? reasonId : undefined}
      data-action-option={option ? "" : undefined}
      style={{ minHeight, ...style }}
      onClick={() => {
        if (!disabled && settled()) onClick();
      }}
    >
      {label}
    </button>
  );
}
