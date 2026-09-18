"use client";

import { useCallback, useEffect, useId, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { MoveRight } from "lucide-react";
import type { Session } from "@/gen/session/v1/types_pb";
import { BOARD_COLUMNS, type BoardColumnKey } from "@/lib/board/columns";
import { legalBoardTransitions } from "@/lib/board/transitions";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import { menuTrigger, menu, menuItem, menuEmpty } from "./MoveToMenu.css";

export interface MoveToMenuProps {
  session: Session;
  /** The column this card currently renders in. */
  currentColumn: BoardColumnKey;
  /**
   * Invoked with the chosen target column when the user selects a menu item. The caller
   * (SessionBoard, via BoardCard) wires this to the same `attemptColumnMove` a completed drag
   * would call, so drag and menu selection always converge on one outcome.
   */
  onMove: (toColumn: BoardColumnKey) => void;
}

function columnLabel(key: BoardColumnKey): string {
  return BOARD_COLUMNS.find((c) => c.key === key)?.label ?? key;
}

/**
 * Target columns offered for `currentColumn`. Mirrors legalBoardTransitions, plus the
 * needs_review -> running special case: that move resolves the pending approval rather than
 * writing a raw status (see attemptColumnMove in SessionBoard.tsx), so it has no entry in
 * legalBoardTransitions itself and has to be added back in here.
 */
function menuTargets(currentColumn: BoardColumnKey): BoardColumnKey[] {
  if (currentColumn === "needs_review") return ["running"];
  return legalBoardTransitions[currentColumn] ?? [];
}

/**
 * Non-drag "Move to..." fallback every BoardCard exposes (WCAG SC 2.5.7 -- dragging must not
 * be the only way to perform a board move). A small trigger + menu, modeled on
 * SessionActionsOverflow's portal-based overflow menu.
 */
export function MoveToMenu({ session, currentColumn, onMove }: MoveToMenuProps) {
  const [isOpen, setIsOpen] = useState(false);
  const [menuPos, setMenuPos] = useState({ top: 0, left: 0 });
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);
  const menuId = useId();

  const targets = menuTargets(currentColumn);

  useFocusTrap(menuRef, isOpen, triggerRef);

  const close = useCallback(() => setIsOpen(false), []);

  const openMenu = useCallback(() => {
    const rect = triggerRef.current?.getBoundingClientRect();
    if (rect) {
      setMenuPos({ top: rect.bottom + 4, left: rect.left });
    }
    setIsOpen(true);
  }, []);

  useEffect(() => {
    if (!isOpen) return;
    const handler = (e: MouseEvent) => {
      const target = e.target as Node;
      if (triggerRef.current?.contains(target) || menuRef.current?.contains(target)) return;
      close();
    };
    document.addEventListener("mousedown", handler);
    return () => document.removeEventListener("mousedown", handler);
  }, [isOpen, close]);

  const handleSelect = useCallback(
    (toColumn: BoardColumnKey) => {
      close();
      onMove(toColumn);
    },
    [close, onMove]
  );

  const handleMenuKeyDown = useCallback((e: React.KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Escape") {
      close();
      return;
    }
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      const items = Array.from(
        menuRef.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? []
      );
      if (items.length === 0) return;
      const currentIndex = items.indexOf(document.activeElement as HTMLElement);
      const nextIndex =
        e.key === "ArrowDown"
          ? (currentIndex + 1) % items.length
          : (currentIndex - 1 + items.length) % items.length;
      items[nextIndex]?.focus();
    }
  }, [close]);

  return (
    <>
      <button
        type="button"
        ref={triggerRef}
        id={`board-card-move-trigger-${session.id}`}
        className={menuTrigger}
        aria-haspopup="menu"
        aria-expanded={isOpen}
        aria-controls={menuId}
        aria-label={`Move ${session.title} to another column`}
        data-testid="move-to-menu-trigger"
        onClick={(e) => {
          e.stopPropagation();
          if (isOpen) {
            close();
          } else {
            openMenu();
          }
        }}
      >
        <MoveRight size={16} aria-hidden="true" />
      </button>
      {isOpen &&
        createPortal(
          <div
            ref={menuRef}
            id={menuId}
            role="menu"
            aria-label={`Move ${session.title} to`}
            className={menu}
            style={{ top: menuPos.top, left: menuPos.left }}
            data-testid="move-to-menu"
            onClick={(e) => e.stopPropagation()}
            onKeyDown={handleMenuKeyDown}
          >
            {targets.length === 0 ? (
              <p className={menuEmpty} data-testid="move-to-menu-empty">
                No moves available
              </p>
            ) : (
              targets.map((target) => (
                <button
                  key={target}
                  type="button"
                  role="menuitem"
                  className={menuItem}
                  onClick={(e) => {
                    e.stopPropagation();
                    handleSelect(target);
                  }}
                >
                  {columnLabel(target)}
                </button>
              ))
            )}
          </div>,
          document.body
        )}
    </>
  );
}
