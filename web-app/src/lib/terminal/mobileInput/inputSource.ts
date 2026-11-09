/**
 * The adapter's single seam to the browser's text-input events (ADR-006). `TextareaSource` is the
 * primary source; an EditContext source can implement `InputSource` later without touching the reducer.
 */
import {
  normalizeBeforeInput,
  normalizeComposition,
  normalizeInput,
  normalizeKeydown,
  normalizeSelection,
} from "./normalize";
import type { MobileInputClock, MobileInputEvent } from "./types";

/** Returns true when the raw event must be `preventDefault`ed. */
export type SourceHandler = (event: MobileInputEvent, raw: Event | null) => boolean;

export interface InputSource {
  readonly element: HTMLTextAreaElement;
  start(handler: SourceHandler): void;
  /** Puts the field back to its reset value; the only place the layer writes to the textarea. */
  reset(value: string): void;
  dispose(): void;
}

export class TextareaSource implements InputSource {
  private teardown: Array<() => void> = [];

  constructor(
    readonly element: HTMLTextAreaElement,
    private readonly clock: MobileInputClock,
  ) {}

  start(handler: SourceHandler): void {
    const el = this.element;
    const doc = el.ownerDocument;
    const on = <E extends Event>(target: EventTarget, type: string, fn: (e: E) => MobileInputEvent | null, prevent = false) => {
      const listener = (raw: Event) => {
        const ev = fn(raw as E);
        if (ev && handler(ev, raw) && prevent) raw.preventDefault();
      };
      target.addEventListener(type, listener);
      this.teardown.push(() => target.removeEventListener(type, listener));
    };
    const now = () => this.clock.now();

    on<KeyboardEvent>(el, "keydown", (e) => normalizeKeydown(e, now()), true);
    on<InputEvent>(el, "beforeinput", (e) => normalizeBeforeInput(e, el, now()));
    on<Event>(el, "input", () => normalizeInput(el, now()));
    on<CompositionEvent>(el, "compositionstart", (e) => normalizeComposition("compositionstart", e, now()));
    on<CompositionEvent>(el, "compositionupdate", (e) => normalizeComposition("compositionupdate", e, now()));
    on<CompositionEvent>(el, "compositionend", (e) => normalizeComposition("compositionend", e, now()));
    on<Event>(el, "blur", () => ({ kind: "blur", t: now() }));
    on<Event>(doc, "selectionchange", () => (doc.activeElement === el ? normalizeSelection(el, now()) : null));
  }

  reset(value: string): void {
    this.element.value = value;
    this.element.setSelectionRange(value.length, value.length);
  }

  dispose(): void {
    this.teardown.forEach((f) => f());
    this.teardown = [];
  }
}
