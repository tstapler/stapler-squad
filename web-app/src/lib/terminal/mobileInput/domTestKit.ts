/** Test-only helpers for the jsdom adapter suites: injected clock, platform env, real DOM events. */
import type { ReplayFixture } from "./fixtureSchema";
import { ModelTerminal } from "./replayHarness";
import type { MobileInputClock, MobileInputEvent, SendResult } from "./types";

export class ManualClock implements MobileInputClock {
  private t = 0;
  private nextId = 1;
  private timers = new Map<number, { at: number; fn: () => void }>();

  now(): number {
    return this.t;
  }
  setTimeout(fn: () => void, ms: number): number {
    const id = this.nextId++;
    this.timers.set(id, { at: this.t + ms, fn });
    return id;
  }
  clearTimeout(id: number): void {
    this.timers.delete(id);
  }
  get pending(): number {
    return this.timers.size;
  }
  /** Moves time forward (never back), firing due timers in order. */
  advanceTo(target: number): void {
    for (;;) {
      const due = [...this.timers.entries()].filter(([, v]) => v.at <= target).sort((a, b) => a[1].at - b[1].at)[0];
      if (!due) break;
      this.timers.delete(due[0]);
      this.t = Math.max(this.t, due[1].at);
      due[1].fn();
    }
    this.t = Math.max(this.t, target);
  }
  advance(ms: number): void {
    this.advanceTo(this.t + ms);
  }
}

const ANDROID_UA = "Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36 Chrome/120 Mobile Safari/537.36";
const originals = {
  ua: Object.getOwnPropertyDescriptor(navigator, "userAgent"),
  uaData: Object.getOwnPropertyDescriptor(navigator, "userAgentData"),
  matchMedia: window.matchMedia,
};

export function setEnv(opts: { ua?: string; coarse: boolean; hover: boolean }): void {
  Object.defineProperty(navigator, "userAgent", { value: opts.ua ?? ANDROID_UA, configurable: true });
  Object.defineProperty(navigator, "userAgentData", { value: undefined, configurable: true });
  window.matchMedia = ((q: string) => ({
    matches: q === "(pointer: coarse)" ? opts.coarse : q === "(hover: hover)" ? opts.hover : false,
  })) as unknown as typeof window.matchMedia;
}

export const setAndroidEnv = () => setEnv({ coarse: true, hover: false });

export function restoreEnv(): void {
  for (const [name, d] of [
    ["userAgent", originals.ua],
    ["userAgentData", originals.uaData],
  ] as const) {
    if (d) Object.defineProperty(navigator, name, d);
    else delete (navigator as unknown as Record<string, unknown>)[name];
  }
  window.matchMedia = originals.matchMedia;
}

/** A sink with a link that can be down, drop a single unit, or silently lose one (half-open socket). */
export class LinkSink {
  linkUp = true;
  dropNext = 0;
  loseNext = 0;
  /** Every unit handed over with a non-dropped result, lost ones included. */
  handed: string[] = [];
  dropped: string[] = [];
  readonly term = new ModelTerminal();

  readonly onData = (bytes: string): SendResult => {
    if (!this.linkUp || this.dropNext > 0) {
      if (this.dropNext > 0) this.dropNext--;
      this.dropped.push(bytes);
      return "dropped";
    }
    this.handed.push(bytes);
    if (this.loseNext > 0) this.loseNext--;
    else this.term.apply(bytes);
    return "sent";
  };
  get bytes(): string {
    return this.handed.join("");
  }
}

export function key(target: EventTarget, type: "keydown" | "keyup", init: KeyboardEventInit & { keyCode?: number }): KeyboardEvent {
  const e = new KeyboardEvent(type, { bubbles: true, cancelable: true, ...init });
  target.dispatchEvent(e);
  return e;
}

export function beforeinput(el: HTMLTextAreaElement, inputType: string, data: string | null, value?: string): InputEvent {
  if (value !== undefined) setValue(el, value);
  const e = new InputEvent("beforeinput", { bubbles: true, cancelable: true, inputType, data });
  el.dispatchEvent(e);
  return e;
}

export function inputEvent(el: HTMLTextAreaElement, value: string): void {
  setValue(el, value);
  el.dispatchEvent(new InputEvent("input", { bubbles: true, inputType: "insertText" }));
}

export function composition(el: HTMLTextAreaElement, type: "compositionstart" | "compositionupdate" | "compositionend", data = ""): void {
  el.dispatchEvent(new CompositionEvent(type, { bubbles: true, data }));
}

export function setValue(el: HTMLTextAreaElement, value: string, selStart = value.length, selEnd = selStart): void {
  el.value = value;
  el.setSelectionRange(Math.min(selStart, value.length), Math.min(selEnd, value.length));
}

/** Plays one fixture step as a real DOM event (boundary steps go through the supplied callbacks). */
export function dispatchStep(
  el: HTMLTextAreaElement,
  step: ReplayFixture["events"][number],
  clock: ManualClock,
  boundary: (kind: "linkDown" | "linkUp" | "sessionSwitch" | "dropNext" | "loseNext") => void,
): void {
  const e = step as MobileInputEvent | { kind: "linkDown" | "linkUp" | "sessionSwitch" | "dropNext" | "loseNext" };
  if ("t" in e && e.kind !== "tick") clock.advanceTo(e.t);
  switch (e.kind) {
    case "linkDown":
    case "linkUp":
    case "sessionSwitch":
    case "dropNext":
    case "loseNext":
      boundary(e.kind);
      return;
    case "keydown":
      key(el, "keydown", { key: e.key, keyCode: e.keyCode, isComposing: e.isComposing });
      return;
    case "beforeinput":
      setValue(el, e.value, e.selStart, e.selEnd);
      beforeinput(el, e.inputType, e.data);
      return;
    case "input":
      setValue(el, e.value, e.selStart, e.selEnd);
      el.dispatchEvent(new InputEvent("input", { bubbles: true }));
      return;
    case "compositionstart":
    case "compositionupdate":
    case "compositionend":
      composition(el, e.kind, "data" in e ? e.data : "");
      return;
    case "selectionchange":
      setValue(el, el.value.padEnd(e.selEnd, "​"), e.selStart, e.selEnd);
      document.dispatchEvent(new Event("selectionchange"));
      return;
    case "blur":
      el.dispatchEvent(new FocusEvent("blur"));
      return;
    case "tick":
      // The watchdog runs on a coarse cadence: give it one full interval past the fixture's tick time.
      clock.advanceTo(e.t + 500);
      return;
  }
}
