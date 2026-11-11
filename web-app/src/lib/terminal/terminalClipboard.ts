/**
 * The one definition of terminal copy/paste behavior, shared by the xterm and ghostty renderers.
 *
 * Both renderers adapt to `TerminalClipboardSurface` and call the same `ClipboardActions`, so the
 * rules cannot drift apart again (ghostty shipped without Ctrl+C-copies, a Copy button, a menu and
 * bracketed paste, and auto-copied on select). Presentation (button placement, selection handles,
 * the menu component) stays renderer-specific; what each action does does not.
 *
 * Contract:
 *  - Selecting never copies. Copying takes an explicit action: the Copy button, the menu, Copy all,
 *    or Ctrl/Cmd+C (+Shift) while text is selected.
 *  - Ctrl/Cmd+C with a selection copies, clears it and is consumed (the app never sees ^C);
 *    with no selection it passes through and interrupts as usual.
 *  - Menu Paste reports when the browser blocks clipboard reads (the user can still press Ctrl/Cmd+V).
 *  - Paste goes through the renderer's own `paste`, which wraps in bracketed-paste markers when the
 *    app asked for them, so a multi-line paste is not submitted line by line.
 *  - The menu is not offered while the app tracks the mouse (the app owns right-click).
 */

export interface TerminalClipboardSurface {
  getSelection(): string;
  clearSelection(): void;
  selectAll(): void;
  /** Sends text as a paste: bracketed when the app enabled it. */
  paste(text: string): void;
  /** Scrollback plus screen as plain text. */
  plainText(): string;
  hasMouseTracking(): boolean;
}

export interface ClipboardEnv {
  copy(text: string): Promise<boolean>;
  readText(): Promise<string>;
  /** Called with the outcome of every copy, whatever triggered it. */
  report(ok: boolean): void;
}

type KeyLike = Pick<KeyboardEvent, "type" | "key" | "ctrlKey" | "metaKey" | "altKey" | "preventDefault">;

/** `blocked`: the browser refused or cannot read the clipboard (permission prompt dismissed, or no secure context). */
export type PasteResult = "pasted" | "empty" | "blocked";

export interface ClipboardActions {
  /** Copies the selection; false when nothing is selected. */
  copySelection(): boolean;
  /** Copies the whole buffer; false when it is empty. */
  copyAll(): boolean;
  /** Menu Paste. Ctrl/Cmd+V never needs this: the browser hands the renderer the text directly. */
  paste(): Promise<PasteResult>;
  /** True when the key was a copy and was consumed; the caller maps that to its renderer's key-handler polarity. */
  interceptCopyKey(event: KeyLike): boolean;
  canOpenMenu(): boolean;
}

export function createClipboardActions(getSurface: () => TerminalClipboardSurface | null, env: ClipboardEnv): ClipboardActions {
  const copyText = (text: string): boolean => {
    if (!text) return false;
    void env.copy(text).then(env.report);
    return true;
  };

  return {
    copySelection: () => copyText(getSurface()?.getSelection() ?? ""),
    copyAll: () => copyText(getSurface()?.plainText() ?? ""),
    async paste() {
      let text: string;
      try {
        text = await env.readText();
      } catch {
        return "blocked";
      }
      if (!text) return "empty";
      getSurface()?.paste(text);
      return "pasted";
    },
    interceptCopyKey(event) {
      const surface = getSurface();
      const isCopy = event.type === "keydown" && (event.ctrlKey || event.metaKey) && !event.altKey && event.key.toLowerCase() === "c";
      if (!isCopy || !surface?.getSelection()) return false;
      event.preventDefault();
      const copied = copyText(surface.getSelection());
      surface.clearSelection();
      return copied;
    },
    canOpenMenu: () => !getSurface()?.hasMouseTracking(),
  };
}

interface BufferLike {
  buffer: { active: { length: number; getLine(y: number): { translateToString(trim: boolean): string } | undefined } };
}

/** Plain text of a buffer, trailing blanks trimmed; both renderers expose this xterm-shaped buffer. */
export function bufferPlainText(term: BufferLike): string {
  const { active } = term.buffer;
  const lines: string[] = [];
  for (let y = 0; y < active.length; y++) {
    const line = active.getLine(y);
    if (line) lines.push(line.translateToString(true));
  }
  return lines.join("\n").trimEnd();
}
