export type ShortcutContext = "global" | "session-list" | "approval" | "terminal" | "cockpit" | "omnibar";

export interface Shortcut {
  /** Key string as reported by KeyboardEvent.key (e.g. "k", "Enter", "[", "?") */
  key: string;
  /** Optional modifier keys required */
  modifiers?: {
    meta?: boolean;
    ctrl?: boolean;
    shift?: boolean;
    alt?: boolean;
  };
  /** Human-readable label shown in the ? overlay */
  label: string;
  /** Context in which this shortcut fires */
  context: ShortcutContext;
  /** The action to execute */
  action: () => void;
}

/**
 * ShortcutRegistry — centralized keyboard shortcut manager.
 *
 * - Single document.addEventListener("keydown") for all shortcuts.
 * - Context-sensitive: detects active context via data-context attribute
 *   on the focused element's ancestor chain.
 * - IME composition events are skipped for single-character shortcuts.
 * - Conflict detection: warns (does not throw) on duplicate id registration.
 */
export class ShortcutRegistry {
  private shortcuts = new Map<string, Shortcut>();
  private bound: (e: KeyboardEvent) => void;

  constructor() {
    this.bound = this.dispatch.bind(this);
    if (typeof document !== "undefined") {
      document.addEventListener("keydown", this.bound);
    }
  }

  /**
   * Register a shortcut. Returns a cleanup function that deregisters it.
   */
  register(id: string, shortcut: Shortcut): () => void {
    if (this.shortcuts.has(id)) {
      console.warn(`[ShortcutRegistry] Duplicate shortcut id: "${id}" — overwriting.`);
    }
    this.shortcuts.set(id, shortcut);
    return () => {
      this.shortcuts.delete(id);
    };
  }

  /**
   * Get all registered shortcuts grouped by context.
   * Used by the ? overlay to render the shortcut reference.
   */
  getAll(): Record<ShortcutContext, Shortcut[]> {
    const result: Record<ShortcutContext, Shortcut[]> = {
      global: [],
      "session-list": [],
      approval: [],
      terminal: [],
      cockpit: [],
      omnibar: [],
    };
    for (const shortcut of this.shortcuts.values()) {
      result[shortcut.context].push(shortcut);
    }
    return result;
  }

  /**
   * Detect the active context by walking up the DOM from document.activeElement
   * looking for a [data-context] attribute.
   */
  private getActiveContext(): ShortcutContext {
    if (typeof document === "undefined") return "global";
    let el: Element | null = document.activeElement;
    while (el) {
      const ctx = el.getAttribute("data-context");
      if (ctx) return ctx as ShortcutContext;
      el = el.parentElement;
    }
    return "global";
  }

  private dispatch(event: KeyboardEvent): void {
    // Skip IME composition for single-character shortcuts
    if (event.isComposing) return;

    // Skip if target is an input/textarea/select/contenteditable
    // (unless the shortcut explicitly uses a modifier)
    const hasModifier = event.metaKey || event.ctrlKey || event.altKey;
    if (!hasModifier && isInputElement(event.target as Element)) return;

    const activeContext = this.getActiveContext();

    for (const shortcut of this.shortcuts.values()) {
      // Context check: global shortcuts fire everywhere (except inside terminal unless
      // the shortcut is terminal-scoped). Terminal-scoped shortcuts only fire in terminal.
      if (shortcut.context === "terminal" && activeContext !== "terminal") continue;
      if (activeContext === "terminal" && shortcut.context !== "terminal" && shortcut.context !== "global" && shortcut.context !== "cockpit") continue;
      if (shortcut.context !== "global" && shortcut.context !== activeContext) continue;

      if (!keyMatches(event, shortcut)) continue;

      event.preventDefault();
      shortcut.action();
      return; // first match wins
    }
  }

  /** Tear down the global listener (useful in tests). */
  destroy(): void {
    if (typeof document !== "undefined") {
      document.removeEventListener("keydown", this.bound);
    }
  }
}

function keyMatches(event: KeyboardEvent, shortcut: Shortcut): boolean {
  if (event.key !== shortcut.key) return false;
  const m = shortcut.modifiers ?? {};
  if (!!m.meta !== event.metaKey) return false;
  if (!!m.ctrl !== event.ctrlKey) return false;
  if (!!m.shift !== event.shiftKey) return false;
  if (!!m.alt !== event.altKey) return false;
  return true;
}

function isInputElement(el: Element | null): boolean {
  if (!el) return false;
  const tag = el.tagName.toLowerCase();
  if (tag === "input" || tag === "textarea" || tag === "select") return true;
  if (el instanceof HTMLElement && el.isContentEditable) return true;
  return false;
}

/** Singleton registry — import this in components and hooks. */
export const registry = new ShortcutRegistry();
