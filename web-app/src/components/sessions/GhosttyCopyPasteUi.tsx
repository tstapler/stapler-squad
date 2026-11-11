"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";
import type { Terminal as GhosttyWebTerminal } from "ghostty-web";
import { disableGhosttyAutoCopy } from "@/lib/terminal/ghosttyConfirmCopy";
import type { ClipboardActions } from "@/lib/terminal/terminalClipboard";
import { TerminalContextMenu } from "./TerminalContextMenu";
import * as styles from "./XtermTerminal.css";

const TOAST_MS = 1500;
const BUTTON_RISE_PX = 40;

export type CopyToast = "copied" | "failed" | "paste-blocked" | null;
const TOAST_TEXT = { copied: "Copied", failed: "Copy failed", "paste-blocked": "Clipboard blocked: press Ctrl+V to paste" } as const;

/** A toast that clears itself; shared with the Ctrl+C handler so every copy path reports the same way. */
export function useCopyToast() {
  const [toast, setToast] = useState<CopyToast>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const show = useCallback((status: boolean | "paste-blocked") => {
    setToast(status === "paste-blocked" ? status : status ? "copied" : "failed");
    clearTimeout(timer.current);
    timer.current = setTimeout(() => setToast(null), TOAST_MS);
  }, []);
  useEffect(() => () => clearTimeout(timer.current), []);
  return { toast, show };
}

interface Props {
  terminal: GhosttyWebTerminal | null;
  actions: ClipboardActions;
  toast: CopyToast;
  showToast: (status: boolean | "paste-blocked") => void;
}

/** Floating Copy button, right-click/long-press menu and "Copy all", matching what XtermTerminal offers. */
export function GhosttyCopyPasteUi({ terminal, actions, toast, showToast }: Props) {
  const [button, setButton] = useState<{ left: number; top: number } | null>(null);
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null);

  useEffect(() => {
    if (!terminal) return;
    disableGhosttyAutoCopy(terminal);

    const syncButton = () => {
      const pos = terminal.getSelection() ? terminal.getSelectionPosition() : undefined;
      const canvas = terminal.element?.querySelector("canvas");
      if (!pos || !canvas) return setButton(null);
      const rect = canvas.getBoundingClientRect();
      const left = rect.left + ((pos.end.x + 1) / terminal.cols) * rect.width;
      const top = rect.top + (pos.end.y / terminal.rows) * rect.height - BUTTON_RISE_PX;
      setButton({ left, top: Math.max(4, top) });
    };
    const onContextMenu = (e: MouseEvent) => {
      e.preventDefault();
      e.stopPropagation();
      if (!actions.canOpenMenu()) return;
      setMenu({ x: e.clientX, y: e.clientY });
    };

    const selection = terminal.onSelectionChange(syncButton);
    // Capture on an ancestor so it runs before ghostty-web's own contextmenu handler on the canvas.
    terminal.element?.addEventListener("contextmenu", onContextMenu, true);
    return () => {
      selection.dispose();
      terminal.element?.removeEventListener("contextmenu", onContextMenu, true);
    };
  }, [terminal, actions]);

  const copySelection = () => {
    actions.copySelection();
    setButton(null);
  };

  return (
    <>
      <button aria-label="Copy terminal scrollback to clipboard" className={styles.scrollbackCopyButton} onPointerDown={(e) => (e.preventDefault(), actions.copyAll())}>
        📋 Copy all
      </button>
      {typeof document !== "undefined" &&
        createPortal(
          <>
            {button && (
              <button
                aria-label="Copy selected text"
                className={styles.floatingCopyButton}
                style={{ left: button.left, top: button.top }}
                onPointerDown={(e) => (e.preventDefault(), copySelection())}
              >
                Copy
              </button>
            )}
            {toast && (
              <div className={`${styles.copiedToast} ${styles.copiedToastVisible}`} aria-live="polite">
                {TOAST_TEXT[toast]}
              </div>
            )}
          </>,
          document.body,
        )}
      {menu && (
        <TerminalContextMenu
          x={menu.x}
          y={menu.y}
          hasSelection={(terminal?.getSelection().length ?? 0) > 0}
          onCopy={copySelection}
          onSelectAll={() => terminal?.selectAll()}
          onPaste={() => void actions.paste().then((r) => r === "blocked" && showToast("paste-blocked"))}
          onDismiss={() => setMenu(null)}
        />
      )}
    </>
  );
}
