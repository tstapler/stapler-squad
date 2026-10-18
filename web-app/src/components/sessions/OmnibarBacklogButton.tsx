"use client";

import type { KeyboardEvent } from "react";

interface OmnibarBacklogButtonProps {
  onClick: () => void;
  disabled?: boolean;
  className: string;
}

export function OmnibarBacklogButton({ onClick, disabled = false, className }: OmnibarBacklogButtonProps) {
  // The modal-wide handleKeyDown would otherwise also run handleSubmit on Enter (a focused
  // button still fires click, so Ctrl/Cmd+Enter would run both actions).
  const handleKeyDown = (e: KeyboardEvent<HTMLButtonElement>) => {
    if (e.key === "Enter") e.stopPropagation();
  };

  return (
    <button
      type="button"
      data-testid="omnibar-create-backlog-button"
      className={className}
      onClick={onClick}
      onKeyDown={handleKeyDown}
      disabled={disabled}
    >
      Create Backlog Item
    </button>
  );
}
