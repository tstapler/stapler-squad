"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  trayButton,
  trayMenu,
  trayMenuCaption,
  trayMenuDivider,
  trayMenuItem,
} from "./NotificationPanel.css";

export interface TrayMenuItem {
  key: string;
  label: string;
  caption?: string;
  /** Rendered before the label (an icon for the destructive item). */
  icon?: ReactNode;
  /** Visible reason a disabled item cannot run ("Offline", "Nothing to clear"). */
  disabledReason?: string;
  onSelect: () => void;
}

/** Items are grouped; a divider separates groups (undoable above irreversible above view-only). */
export interface TrayMenuProps {
  groups: TrayMenuItem[][];
  /** Offline and similar state names the reason on every disabled item. */
  triggerLabel?: string;
}

/** "..." overflow menu: arrow keys move, Esc closes and returns focus to the trigger (TY-6, TM-6). */
export function TrayOverflowMenu({ groups, triggerLabel = "More notification actions" }: TrayMenuProps) {
  const [open, setOpen] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  const items = () => Array.from(menuRef.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? []);

  useEffect(() => {
    if (open) items()[0]?.focus();
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (e: PointerEvent) => {
      const target = e.target as Node;
      if (!menuRef.current?.contains(target) && !triggerRef.current?.contains(target)) setOpen(false);
    };
    document.addEventListener("pointerdown", onPointerDown);
    return () => document.removeEventListener("pointerdown", onPointerDown);
  }, [open]);

  const close = (restoreFocus: boolean) => {
    setOpen(false);
    if (restoreFocus) triggerRef.current?.focus();
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    const list = items();
    const index = list.indexOf(document.activeElement as HTMLElement);
    const move = (next: number) => {
      e.preventDefault();
      list[(next + list.length) % list.length]?.focus();
    };
    if (e.key === "ArrowDown") move(index + 1);
    else if (e.key === "ArrowUp") move(index - 1);
    else if (e.key === "Home") move(0);
    else if (e.key === "End") move(list.length - 1);
    else if (e.key === "Escape") {
      // The tray's own Esc handler must not also close the tray.
      e.preventDefault();
      e.stopPropagation();
      close(true);
    } else if (e.key === "Tab") setOpen(false);
  };

  return (
    <div style={{ position: "relative" }}>
      <button
        ref={triggerRef}
        type="button"
        className={trayButton}
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={triggerLabel}
        data-testid="tray-overflow"
        onClick={() => setOpen((v) => !v)}
      >
        <span aria-hidden="true">...</span>
      </button>
      {open && (
        <div ref={menuRef} role="menu" className={trayMenu} data-testid="tray-overflow-menu" onKeyDown={onKeyDown}>
          {groups.map((group, groupIndex) => (
            <div key={groupIndex} role="none">
              {groupIndex > 0 && <div className={trayMenuDivider} role="separator" />}
              {group.map((item) => {
                const disabled = item.disabledReason !== undefined;
                return (
                  <button
                    key={item.key}
                    type="button"
                    role="menuitem"
                    className={trayMenuItem}
                    aria-disabled={disabled || undefined}
                    data-testid={`tray-menu-${item.key}`}
                    onClick={() => {
                      if (disabled) return;
                      // The focused item unmounts with the menu; a target that opens its own UI re-focuses over this.
                      close(true);
                      item.onSelect();
                    }}
                  >
                    <span>
                      {item.icon}
                      {item.label}
                    </span>
                    {(item.disabledReason ?? item.caption) && (
                      <span className={trayMenuCaption}>{item.disabledReason ?? item.caption}</span>
                    )}
                  </button>
                );
              })}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}
