import { useEffect, useRef, type KeyboardEvent } from "react";

/** Menu-button behavior: focus first item on open, close on outside mousedown/Tab, arrows cycle, Esc closes and refocuses the trigger. */
export function usePopupMenu(open: boolean, setOpen: (open: boolean) => void) {
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    menuRef.current?.querySelector<HTMLElement>('[role="menuitem"]')?.focus();
    const away = (e: Event) => { if (!menuRef.current?.contains(e.target as Node) && !triggerRef.current?.contains(e.target as Node)) setOpen(false); };
    document.addEventListener("mousedown", away);
    return () => document.removeEventListener("mousedown", away);
  }, [open, setOpen]);

  const onKeyDown = (e: KeyboardEvent) => {
    const entries = Array.from(menuRef.current?.querySelectorAll<HTMLElement>('[role="menuitem"]') ?? []);
    const at = entries.indexOf(document.activeElement as HTMLElement);
    if (e.key === "Escape") { e.stopPropagation(); setOpen(false); triggerRef.current?.focus(); }
    else if (e.key === "ArrowDown") { e.preventDefault(); entries[(at + 1) % entries.length]?.focus(); }
    else if (e.key === "ArrowUp") { e.preventDefault(); entries[(at - 1 + entries.length) % entries.length]?.focus(); }
    else if (e.key === "Tab") setOpen(false);
  };

  return { triggerRef, menuRef, onKeyDown };
}
