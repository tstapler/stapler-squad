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

/** Keystrokes that turn `previous` into `next`: DELs for the changed tail, then the new tail (by code point). */
export function compositionDelta(previous: string, next: string): string {
  const prev = Array.from(previous);
  const nxt = Array.from(next);
  let prefix = 0;
  const max = Math.min(prev.length, nxt.length);
  while (prefix < max && prev[prefix] === nxt[prefix]) prefix++;
  return DEL.repeat(prev.length - prefix) + nxt.slice(prefix).join("");
}

const trailingWord = (s: string) => /\S*$/.exec(s)?.[0] ?? "";
const dropLastCodePoint = (s: string) => Array.from(s).slice(0, -1).join("");
// Only the IME target (helper textarea or the contenteditable root) is ours to intercept.
const isImeTarget = (root: HTMLElement, e: Event) => e.target === root || e.target instanceof HTMLTextAreaElement;

/**
 * Attach to the element that holds the IME target: `terminal.element` for xterm.js (contains the helper
 * textarea) or ghostty-web's contenteditable root. Works from event data, using textarea.value only to
 * confirm a re-composition. Listens in the capture phase so it runs, and can stop propagation, before the
 * renderer's own listeners; it stops only events targeting the IME element. Nothing else in web-app
 * listens for these events on the terminal (checked: only bubble-phase window/document shortcut hooks,
 * which cannot match a 229/"Unidentified" key). `paste` receives multi-character and pasted text so it
 * takes the renderer's bracketed-paste path. Returns a disposer.
 */
export function attachAndroidImeInput(
  root: HTMLElement,
  send: (data: string) => void,
  paste: (text: string) => void = (text) => send(text.replace(/\r?\n/g, "\r")),
): () => void {
  let composing = false;
  let sent = ""; // text of the active composition already forwarded
  let base = ""; // terminal-side word text that the active composition sits after
  let word = ""; // trailing non-space word as the terminal currently has it (outside any composition)
  let started = false; // first compositionupdate of the active composition seen

  const sendTyped = (data: string) => {
    word = trailingWord(word + data);
    send(data);
  };

  const flushComposition = (text: string) => {
    const delta = compositionDelta(sent, text);
    sent = text;
    if (delta) send(delta);
  };

  // Gboard re-composes an already-sent word (backspace into it, suggestion tap): the first update then
  // carries the whole existing word, which is already on the terminal, so treat it as already sent.
  const claimExistingWord = (e: Event, text: string) => {
    started = true;
    const ta = e.target instanceof HTMLTextAreaElement ? e.target : undefined;
    const recomposing = word !== "" && text.startsWith(word) && (!ta || ta.value.endsWith(word));
    base = recomposing ? "" : word;
    sent = recomposing ? word : "";
  };

  const onKeyDown = (e: Event) => {
    if ((e as KeyboardEvent).keyCode !== IME_KEYCODE) {
      word = ""; // Enter/Tab/arrows go through xterm; the word under the cursor is no longer ours to track
      return;
    }
    // The renderer would otherwise diff/reinterpret a 229 keydown and double-send.
    if (isImeTarget(root, e)) e.stopImmediatePropagation();
  };

  const onCompositionStart = (e: Event) => {
    if (!isImeTarget(root, e)) return;
    composing = true;
    started = false;
    sent = "";
    base = word;
    e.stopImmediatePropagation();
  };

  const onCompositionUpdate = (e: Event) => {
    if (!isImeTarget(root, e)) return;
    if (composing) {
      const text = (e as CompositionEvent).data ?? sent;
      if (!started) claimExistingWord(e, text);
      flushComposition(text);
    }
    e.stopImmediatePropagation();
  };

  const onCompositionEnd = (e: Event) => {
    if (!isImeTarget(root, e)) return;
    if (composing) {
      const text = (e as CompositionEvent).data ?? sent;
      if (!started) claimExistingWord(e, text);
      flushComposition(text);
      word = trailingWord(base + text);
    }
    composing = false;
    sent = "";
    e.stopImmediatePropagation();
    // ghostty-web's root is contenteditable and accumulates the composed text nodes.
    if (!(e.target instanceof HTMLTextAreaElement)) {
      for (const node of Array.from(root.childNodes)) if (node.nodeType === Node.TEXT_NODE) root.removeChild(node);
    }
  };

  const onBeforeInput = (e: Event) => {
    if (!isImeTarget(root, e)) return;
    const ie = e as InputEvent;
    const { inputType } = ie;
    if (composing) return;
    const text = ie.data ?? ie.dataTransfer?.getData("text/plain") ?? "";
    if (inputType === "insertFromPaste") {
      if (!text) return;
      paste(text);
      word = "";
    } else if (inputType === "insertReplacementText") {
      // Spellcheck/suggestion replacing the word under the cursor: same correction as autocorrect.
      if (!text) return;
      const delta = compositionDelta(word, text);
      word = trailingWord(text);
      if (delta) send(delta);
    } else if (inputType === "insertText" && text) {
      if (Array.from(text).length > 1 || /[\r\n]/.test(text)) {
        paste(text);
        word = "";
      } else sendTyped(text);
    } else if (inputType === "deleteContentBackward") {
      word = dropLastCodePoint(word);
      send(DEL);
    } else return;
    e.preventDefault();
    e.stopImmediatePropagation();
  };

  // xterm sends insertText from `input` once keydown was swallowed (keyDownSeen is false), so swallow it too.
  const onInput = (e: Event) => {
    if (!isImeTarget(root, e)) return;
    const { inputType } = e as InputEvent;
    if (
      inputType === "insertText" || inputType === "insertCompositionText" || inputType === "deleteContentBackward" ||
      inputType === "insertFromPaste" || inputType === "insertReplacementText"
    ) {
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
