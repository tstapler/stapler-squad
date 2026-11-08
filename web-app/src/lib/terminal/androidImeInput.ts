// Android soft keyboards (Gboard) never send per-key keydown for letters: every letter is a keydown with
// keyCode 229 / key "Unidentified", and the text arrives as composition + `input` events on xterm's helper
// textarea (or ghostty-web's contenteditable root). xterm.js only emits a composed word at compositionend (space, punctuation, picking a
// suggestion), so typing letters looks dead until then. This streams each composition change to the
// terminal as it happens and keeps xterm's own composition/diff paths out of the way (no double send).
// Real keys (Enter, Tab, Backspace with a real keyCode) are untouched and still go through xterm.

const IME_KEYCODE = 229;
const DEL = "\x7f";

export function isAndroidUserAgent(ua: string = typeof navigator === "undefined" ? "" : navigator.userAgent): boolean {
  return /Android/i.test(ua);
}

/** Keystrokes that turn `previous` into `next`: DELs for the changed tail, then the new tail. */
export function compositionDelta(previous: string, next: string): string {
  let prefix = 0;
  const max = Math.min(previous.length, next.length);
  while (prefix < max && previous[prefix] === next[prefix]) prefix++;
  const removed = [...previous.slice(prefix)].length;
  return DEL.repeat(removed) + next.slice(prefix);
}

/**
 * Attach to the element that holds the IME target: `terminal.element` for xterm.js (contains the helper
 * textarea) or ghostty-web's contenteditable root. Works from event data only, never textarea.value.
 * Listens in the capture phase so it runs, and can stop propagation, before the renderer's own listeners.
 * Returns a disposer.
 */
export function attachAndroidImeInput(root: HTMLElement, send: (data: string) => void): () => void {
  let composing = false;
  let sent = ""; // composition text already forwarded

  const flushComposition = (text: string) => {
    const delta = compositionDelta(sent, text);
    sent = text;
    if (delta) send(delta);
  };

  const onKeyDown = (e: Event) => {
    // The renderer would otherwise diff/reinterpret a 229 keydown and double-send.
    if ((e as KeyboardEvent).keyCode === IME_KEYCODE) e.stopImmediatePropagation();
  };

  const onCompositionStart = (e: Event) => {
    composing = true;
    sent = "";
    e.stopImmediatePropagation();
  };

  const onCompositionUpdate = (e: Event) => {
    if (composing) flushComposition((e as CompositionEvent).data ?? sent);
    e.stopImmediatePropagation();
  };

  const onCompositionEnd = (e: Event) => {
    if (composing) flushComposition((e as CompositionEvent).data ?? sent);
    composing = false;
    sent = "";
    e.stopImmediatePropagation();
    // ghostty-web's root is contenteditable and accumulates the composed text nodes.
    if (!(e.target instanceof HTMLTextAreaElement)) {
      for (const node of Array.from(root.childNodes)) if (node.nodeType === Node.TEXT_NODE) root.removeChild(node);
    }
  };

  const onBeforeInput = (e: Event) => {
    const { inputType, data } = e as InputEvent;
    if (composing) return;
    if (inputType === "insertText" && data) send(data);
    else if (inputType === "deleteContentBackward") send(DEL);
    else return;
    e.preventDefault();
    e.stopImmediatePropagation();
  };

  // xterm sends insertText from `input` once keydown was swallowed (keyDownSeen is false), so swallow it too.
  const onInput = (e: Event) => {
    const { inputType } = e as InputEvent;
    if (inputType === "insertText" || inputType === "insertCompositionText" || inputType === "deleteContentBackward") {
      e.stopImmediatePropagation();
    }
  };

  const listeners: Array<[string, (e: Event) => void]> = [
    ["keydown", onKeyDown],
    ["compositionstart", onCompositionStart],
    ["compositionupdate", onCompositionUpdate],
    ["compositionend", onCompositionEnd],
    ["beforeinput", onBeforeInput],
    ["input", onInput],
  ];
  for (const [type, fn] of listeners) root.addEventListener(type, fn, true);
  return () => {
    for (const [type, fn] of listeners) root.removeEventListener(type, fn, true);
  };
}
