// Android soft keyboards (Gboard) never send per-key keydown for letters: every letter is a keydown with
// keyCode 229 / key "Unidentified", and the text arrives as composition + `input` events on xterm's helper
// textarea. xterm.js only emits a composed word at compositionend (space, punctuation, picking a
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
 * Attach to the element that contains xterm's helper textarea (`terminal.element`). Listens in the
 * capture phase so it runs, and can stop propagation, before xterm's own textarea listeners.
 * Returns a disposer.
 */
export function attachAndroidImeInput(root: HTMLElement, send: (data: string) => void): () => void {
  let composing = false;
  let compositionStart = 0; // textarea.value.length when the composition began
  let sent = ""; // composition text already forwarded

  const textarea = (e: Event): HTMLTextAreaElement | null =>
    e.target instanceof HTMLTextAreaElement ? e.target : null;

  const flushComposition = (ta: HTMLTextAreaElement) => {
    const current = ta.value.slice(compositionStart);
    const delta = compositionDelta(sent, current);
    sent = current;
    if (delta) send(delta);
  };

  const swallow = (e: Event) => e.stopImmediatePropagation();

  const onKeyDown = (e: Event) => {
    if (!textarea(e) || (e as KeyboardEvent).keyCode !== IME_KEYCODE) return;
    // xterm would diff the textarea on a 229 keydown and double-send what the input listener forwards.
    swallow(e);
  };

  const onCompositionStart = (e: Event) => {
    const ta = textarea(e);
    if (!ta) return;
    composing = true;
    compositionStart = ta.value.length;
    sent = "";
    swallow(e);
  };

  const onCompositionUpdate = (e: Event) => {
    if (textarea(e)) swallow(e);
  };

  const onCompositionEnd = (e: Event) => {
    const ta = textarea(e);
    if (!ta) return;
    if (composing) flushComposition(ta);
    composing = false;
    sent = "";
    swallow(e);
  };

  const onInput = (e: Event) => {
    const ta = textarea(e);
    if (!ta) return;
    const { inputType, data } = e as InputEvent;
    if (composing && inputType === "insertCompositionText") {
      flushComposition(ta);
      swallow(e);
    } else if (!composing && inputType === "insertText" && data) {
      send(data);
      swallow(e);
    } else if (!composing && inputType === "deleteContentBackward") {
      send(DEL);
      swallow(e);
    }
  };

  const listeners: Array<[string, (e: Event) => void]> = [
    ["keydown", onKeyDown],
    ["compositionstart", onCompositionStart],
    ["compositionupdate", onCompositionUpdate],
    ["compositionend", onCompositionEnd],
    ["input", onInput],
  ];
  for (const [type, fn] of listeners) root.addEventListener(type, fn, true);
  return () => {
    for (const [type, fn] of listeners) root.removeEventListener(type, fn, true);
  };
}
