"use client";

import type { Ref } from "react";
import { COPY } from "@/lib/copy/notificationCopy";
import { actionButton, menu } from "./ActionRow.css";

interface MoreMenuProps {
  open: boolean;
  blocked: boolean;
  minHeight?: number;
  onToggle: () => void;
  onCreateRule: () => void;
  buttonRef: Ref<HTMLButtonElement>;
  firstItemRef: Ref<HTMLButtonElement>;
}

/** Desktop-only menu holding Create rule..., kept out of the primary row (ux.md 2.1). */
export function MoreMenu({ open, blocked, minHeight, onToggle, onCreateRule, buttonRef, firstItemRef }: MoreMenuProps) {
  const toggle = (
    // analytics-exempt
    <button
      ref={buttonRef}
      type="button"
      className={actionButton}
      data-testid="action-more"
      style={{ minHeight }}
      aria-haspopup="menu"
      aria-expanded={open}
      aria-disabled={blocked ? true : undefined}
      onClick={() => !blocked && onToggle()}
      onKeyDown={(e) => {
        if (e.key === "ArrowDown" && !blocked && !open) {
          e.preventDefault();
          onToggle();
        }
      }}
      >
      {COPY.more}
    </button>
  );
  const createRule = (
    // analytics-exempt
    <button ref={firstItemRef} type="button" role="menuitem" className={actionButton} data-testid="action-create-rule" onClick={onCreateRule}>
      {COPY.createRuleMenu}
    </button>
  );
  return (
    <>
      {toggle}
      {open && (
        <div role="menu" className={menu} aria-label={COPY.more} data-testid="more-menu">
          {createRule}
        </div>
      )}
    </>
  );
}
