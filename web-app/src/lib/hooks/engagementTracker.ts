import { getTiming } from "./interactionTiming";

/** none: collapsed. pre: expanded by the program on panel open (never engages). operator: opened by the operator. */
export type Expansion = "none" | "pre" | "operator";

interface Card {
  focus: boolean;
  pointer: boolean;
  touchUntil: number;
  subStep: boolean;
  hold: boolean;
  resolved: boolean;
  expansion: Expansion;
  /** The operator opened or Tabbed onto it. */
  armed: boolean;
  /** The operator has toggled it, so the pre-expanded default no longer applies. */
  touched: boolean;
}

const blank = (): Card => ({
  focus: false,
  pointer: false,
  touchUntil: 0,
  subStep: false,
  hold: false,
  resolved: false,
  expansion: "none",
  armed: false,
  touched: false,
});

/**
 * Framework-free core of `design/ux.md` 7.1: which cards are engaged, whether the Needs-you
 * list is frozen, and whether a card is digit-armed. `frozen` is true while any card is engaged
 * and for `graceMs` after the last stops being so.
 */
export class EngagementTracker {
  private cards = new Map<string, Card>();
  private listeners = new Set<() => void>();
  private frozen = false;
  private releaseTimer: ReturnType<typeof setTimeout> | null = null;
  private touchTimers = new Map<string, ReturnType<typeof setTimeout>>();
  private lastProgrammaticAt = Number.NEGATIVE_INFINITY;
  private lastLayoutChangeAt = Number.NEGATIVE_INFINITY;
  private programmaticDepth = 0;
  private digitDepth = 0;

  subscribe = (fn: () => void): (() => void) => {
    this.listeners.add(fn);
    return () => this.listeners.delete(fn);
  };

  isFrozen = (): boolean => this.frozen;

  private card(id: string): Card {
    let c = this.cards.get(id);
    if (!c) {
      c = blank();
      this.cards.set(id, c);
    }
    return c;
  }

  private patch(id: string, p: Partial<Card>): void {
    Object.assign(this.card(id), p);
    this.recompute();
  }

  /** Engaged by the 7.1 definition: a resolved card is held only by pointer and touch. */
  isEngaged(id: string, now = Date.now()): boolean {
    const c = this.cards.get(id);
    if (!c) return false;
    if (c.pointer || c.touchUntil > now) return true;
    if (c.resolved) return false;
    return c.focus || c.subStep || c.hold || c.expansion === "operator";
  }

  private anyEngaged(): boolean {
    const now = Date.now();
    for (const id of this.cards.keys()) if (this.isEngaged(id, now)) return true;
    return false;
  }

  private recompute(): void {
    if (this.anyEngaged()) {
      if (this.releaseTimer) clearTimeout(this.releaseTimer);
      this.releaseTimer = null;
      if (!this.frozen) this.setFrozen(true);
      return;
    }
    if (this.frozen && !this.releaseTimer) {
      this.releaseTimer = setTimeout(() => {
        this.releaseTimer = null;
        if (this.anyEngaged()) return;
        this.setFrozen(false);
      }, getTiming().graceMs);
    }
  }

  private setFrozen(v: boolean): void {
    this.frozen = v;
    this.listeners.forEach((fn) => fn());
  }

  /** The operator's focus landed in the card. Program-placed focus never engages. */
  focusIn(id: string): void {
    if (this.programmaticDepth > 0) return;
    const c = this.card(id);
    c.focus = true;
    // A digit's own focus move is the operator's keystroke: it engages but arms nothing new.
    if (this.digitDepth === 0) c.armed = true;
    this.recompute();
  }

  focusOut(id: string): void {
    if (this.cards.has(id)) this.patch(id, { focus: false });
  }

  pointer(id: string, over: boolean): void {
    this.patch(id, { pointer: over });
  }

  touch(id: string): void {
    const hold = getTiming().touchHoldMs;
    this.card(id).touchUntil = Date.now() + hold;
    const prev = this.touchTimers.get(id);
    if (prev) clearTimeout(prev);
    this.touchTimers.set(
      id,
      setTimeout(() => {
        this.touchTimers.delete(id);
        this.recompute();
      }, hold),
    );
    this.recompute();
  }

  setSubStep = (id: string, v: boolean) => this.patch(id, { subStep: v });
  setHold = (id: string, v: boolean) => this.patch(id, { hold: v });
  setResolved = (id: string, v: boolean) => this.patch(id, { resolved: v });

  expansion(id: string): Expansion {
    return this.cards.get(id)?.expansion ?? "none";
  }

  /** The program's default: expand the first item on open. Ignored once the operator touched it. */
  preExpand(id: string): void {
    const c = this.card(id);
    if (!c.touched && c.expansion === "none") this.patch(id, { expansion: "pre" });
  }

  /** Enter, Space or a tap on the header. Pre-expanded collapses; the next toggle opens and arms it. */
  toggleExpanded(id: string): Expansion {
    const c = this.card(id);
    c.touched = true;
    if (c.expansion === "none") {
      c.expansion = "operator";
      c.armed = true;
    } else {
      c.expansion = "none";
    }
    this.recompute();
    return c.expansion;
  }

  collapse(id: string): void {
    if (this.cards.has(id)) this.patch(id, { expansion: "none", touched: true });
  }

  /** A row was inserted, removed or reordered, or focus moved by the program. */
  noteListChange(): void {
    this.lastProgrammaticAt = Date.now();
    this.lastLayoutChangeAt = this.lastProgrammaticAt;
  }

  /** Moves focus without engaging: the program's focus never counts. */
  focusProgrammatically(el: HTMLElement | null | undefined): void {
    if (!el) return;
    this.lastProgrammaticAt = Date.now();
    this.programmaticDepth += 1;
    try {
      el.focus({ preventScroll: true });
    } finally {
      this.programmaticDepth -= 1;
    }
  }

  /** A digit key's focus move: engages the card, restarts no clock, disarms nothing. */
  focusByDigit(el: HTMLElement | null | undefined): void {
    if (!el) return;
    this.digitDepth += 1;
    try {
      el.focus({ preventScroll: true });
    } finally {
      this.digitDepth -= 1;
    }
  }

  isDigitArmed(id: string, now = Date.now()): boolean {
    const c = this.cards.get(id);
    if (!c || c.resolved || !c.armed) return false;
    const held = c.focus || c.touchUntil > now || c.expansion === "operator";
    return held && now - this.lastProgrammaticAt >= getTiming().graceMs;
  }

  /** A control mounted or moved at `since` ignores activation until the layout has settled. */
  isLayoutSettled(since: number, now = Date.now()): boolean {
    return now - Math.max(since, this.lastLayoutChangeAt) >= getTiming().settleMs;
  }

  /** The card's row left the list. */
  forget(id: string): void {
    if (!this.cards.delete(id)) return;
    const t = this.touchTimers.get(id);
    if (t) clearTimeout(t);
    this.touchTimers.delete(id);
    this.recompute();
  }

  /** The panel closed: nothing is engaged any more and the list unfreezes at once. */
  reset(): void {
    this.cards.clear();
    this.touchTimers.forEach((t) => clearTimeout(t));
    this.touchTimers.clear();
    if (this.releaseTimer) clearTimeout(this.releaseTimer);
    this.releaseTimer = null;
    if (this.frozen) this.setFrozen(false);
  }
}
