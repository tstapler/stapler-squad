// Diagnostics for terminal soft-keyboard input: a ring buffer of the last input-path events on the
// terminal root, shipped to the server log through the client-log shipper (console.warn, tag
// "[ime-trace]") when the page is Android or has ?imeTrace=1. Lets us read the exact event sequence
// a phone produced instead of inferring it.

const RING_SIZE = 50;
const FLUSH_DELAY_MS = 800;
const EVENT_TYPES = [
  "keydown", "keyup", "compositionstart", "compositionupdate", "compositionend",
  "beforeinput", "input", "focusin", "focusout",
] as const;

export function imeTraceEnabled(ua: string = navigator.userAgent, search: string = location.search): boolean {
  return /Android/i.test(ua) || new URLSearchParams(search).has("imeTrace");
}

function describe(e: Event): string {
  const t = e.target instanceof Element ? e.target.tagName.toLowerCase() : "?";
  const k = e as KeyboardEvent;
  const i = e as InputEvent;
  const c = e as CompositionEvent;
  switch (e.type) {
    case "keydown":
    case "keyup":
      return `${e.type} key=${k.key} code=${k.keyCode} comp=${k.isComposing} t=${t}`;
    case "beforeinput":
    case "input":
      return `${e.type} type=${i.inputType} data=${JSON.stringify(i.data)} comp=${i.isComposing} cancel=${e.cancelable} t=${t}`;
    case "focusin":
    case "focusout":
      return `${e.type} t=${t}`;
    default:
      return `${e.type} data=${JSON.stringify(c.data)} t=${t}`;
  }
}

export interface ImeTrace {
  /** Record a non-DOM step (onData length, shim send, socket state). */
  note: (label: string) => void;
  dispose: () => void;
}

/** Call BEFORE attachAndroidImeInput so these capture listeners run first and see events it swallows. */
export function attachImeTrace(root: HTMLElement, renderer: string): ImeTrace {
  const ring: string[] = [];
  let timer: ReturnType<typeof setTimeout> | undefined;
  const start = performance.now();
  const script = document.querySelector<HTMLScriptElement>('script[src*="index-"]')?.src ?? "unknown";

  const flush = () => {
    timer = undefined;
    if (ring.length === 0) return;
    const lines = ring.splice(0);
    console.warn(`[ime-trace] renderer=${renderer} bundle=${script} ${JSON.stringify(lines)}`);
  };
  const note = (label: string) => {
    ring.push(`+${Math.round(performance.now() - start)} ${label}`);
    if (ring.length > RING_SIZE) ring.shift();
    if (timer === undefined) timer = setTimeout(flush, FLUSH_DELAY_MS);
  };
  const onEvent = (e: Event) => note(describe(e));
  for (const type of EVENT_TYPES) root.addEventListener(type, onEvent, { capture: true, passive: true });
  note(`attach active=${document.activeElement?.tagName ?? "none"}`);
  return {
    note,
    dispose: () => {
      for (const type of EVENT_TYPES) root.removeEventListener(type, onEvent, true);
      if (timer !== undefined) clearTimeout(timer);
      flush();
    },
  };
}
