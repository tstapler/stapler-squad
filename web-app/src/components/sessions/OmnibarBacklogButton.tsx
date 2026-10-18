"use client";

import type { KeyboardEvent } from "react";
import { button as buttonClass, buttonSecondary } from "./Omnibar.css";

interface OmnibarBacklogButtonProps {
  onClick: () => void;
  disabled?: boolean;
  className?: string;
}

export function OmnibarBacklogButton({ onClick, disabled = false, className }: OmnibarBacklogButtonProps) {
  // The modal-wide handleKeyDown would otherwise also run handleSubmit on Enter.
  const handleKeyDown = (e: KeyboardEvent<HTMLButtonElement>) => {
    if ((e.key === "Enter" && !e.metaKey && !e.ctrlKey) || e.key === " ") {
      e.stopPropagation();
    }
  };

  return (
    <button
      type="button"
      data-testid="omnibar-create-backlog-button"
      className={className ?? `${buttonClass} ${buttonSecondary}`}
      onClick={onClick}
      onKeyDown={handleKeyDown}
      disabled={disabled}
    >
      Create Backlog Item
    </button>
  );
}
