/** Turns real DOM events into the reducer's `MobileInputEvent`. Reads values; never mutates the textarea. */
import type { MobileInputEvent } from "./types";

type Of<K extends MobileInputEvent["kind"]> = Extract<MobileInputEvent, { kind: K }>;

/** `beforeinput` is the layer's primary signal; without it the layer must not attach. */
export function supportsBeforeInput(doc: Document): boolean {
  return "onbeforeinput" in doc.createElement("textarea");
}

export function normalizeKeydown(e: KeyboardEvent, t: number): Of<"keydown"> {
  return { kind: "keydown", key: e.key, keyCode: e.keyCode, isComposing: e.isComposing, t };
}

export function normalizeBeforeInput(e: InputEvent, el: HTMLTextAreaElement, t: number): Of<"beforeinput"> {
  // Textareas usually carry the pasted text in `data`; fall back to dataTransfer when they do not.
  const data = e.data ?? (e.inputType === "insertFromPaste" ? (e.dataTransfer?.getData("text/plain") ?? null) : null);
  return {
    kind: "beforeinput",
    inputType: e.inputType,
    data,
    value: el.value,
    selStart: el.selectionStart,
    selEnd: el.selectionEnd,
    t,
  };
}

export function normalizeInput(el: HTMLTextAreaElement, t: number): Of<"input"> {
  return { kind: "input", value: el.value, selStart: el.selectionStart, selEnd: el.selectionEnd, t };
}

export function normalizeComposition(
  kind: "compositionstart" | "compositionupdate" | "compositionend",
  e: CompositionEvent,
  t: number,
): MobileInputEvent {
  return kind === "compositionstart" ? { kind, t } : { kind, data: e.data ?? "", t };
}

export function normalizeSelection(el: HTMLTextAreaElement, t: number): Of<"selectionchange"> {
  return { kind: "selectionchange", selStart: el.selectionStart, selEnd: el.selectionEnd, t };
}
