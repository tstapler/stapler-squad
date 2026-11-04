export type ShortcutContext = "global" | "session-list" | "approval" | "terminal" | "cockpit" | "omnibar" | "diff";

export interface Shortcut {
  /**
   * Key string as reported by KeyboardEvent.key (e.g. "k", "Enter", "[", "?").
   * Omit for sequence-only actions that have no direct chord (reached through
   * the leader-key command bar).
   */
  key?: string;
  /**
   * Also match KeyboardEvent.code (e.g. "KeyW"); either `key` or `code` may
   * match. Needed where a modifier changes `key` (macOS Option+letter yields a
   * symbol, not the letter).
   */
  code?: string;
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
  /**
   * Leader-key sequence reaching this action in the command bar, e.g. ["w", "1"].
   * Keys are single printable characters; "j" and "k" are reserved for list navigation.
   */
  sequence?: string[];
  /** Group heading for the first key of `sequence` (e.g. "Windows"). */
  group?: string;
  /** Hide from the command bar, getAll() and the shortcuts reference (internal follow-ups). */
  hidden?: boolean;
}

/** One node of the which-key tree: a key at some level, either a leaf action or a branch. */
export interface ShortcutTreeNode {
  key: string;
  label: string;
  /** Present on leaves. */
  shortcut?: Shortcut;
  children: ShortcutTreeNode[];
}

export type ShortcutTree = ShortcutTreeNode[];

/** Keys the command bar reserves for list navigation; never valid in a sequence. */
export const RESERVED_SEQUENCE_KEYS: ReadonlySet<string> = new Set(["j", "k", "?"]);

/** A palette entry: a registered action plus its display label for the sequence. */
export interface PaletteEntry {
  id: string;
  shortcut: Shortcut;
}

/** Produces sequenced shortcuts on demand when the bar navigates under `prefix`. */
export type LazyProvider = () => Array<{ id: string; shortcut: Shortcut }>;

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
  private lazyProviders = new Map<string, { prefix: string[]; provider: LazyProvider; cleanups: Array<() => void> }>();
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
    if (shortcut.sequence) {
      if (shortcut.sequence.length === 0 || shortcut.sequence.some((k) => k.length !== 1 || RESERVED_SEQUENCE_KEYS.has(k))) {
        console.warn(`[ShortcutRegistry] Invalid sequence for "${id}": ${JSON.stringify(shortcut.sequence)} — ignoring sequence.`);
        shortcut = { ...shortcut, sequence: undefined };
      }
    }
    if (this.shortcuts.has(id)) {
      console.warn(`[ShortcutRegistry] Duplicate shortcut id: "${id}" — overwriting.`);
    }
    this.shortcuts.set(id, shortcut);
    // Owner-token cleanup: a stale registration (e.g. a hidden window unmounting) must not delete a later overwrite.
    return () => {
      if (this.shortcuts.get(id) === shortcut) this.shortcuts.delete(id);
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
      diff: [],
    };
    for (const shortcut of this.shortcuts.values()) {
      if (shortcut.hidden) continue;
      result[shortcut.context].push(shortcut);
    }
    return result;
  }

  /**
   * Every visible action, for the command bar's fuzzy palette: includes actions
   * with no sequence (they show their direct chord). Stable registration order.
   */
  getPalette(): PaletteEntry[] {
    const out: PaletteEntry[] = [];
    for (const [id, shortcut] of this.shortcuts) {
      if (!shortcut.hidden) out.push({ id, shortcut });
    }
    return out;
  }

  /**
   * Group-nested which-key structure built from `sequence` fields. A node is
   * either a leaf (the sequence ends there) or a branch; if one registration's
   * sequence is a prefix of another's, the longer one wins the key and the
   * shorter is dropped (warned) so a key never both runs and descends.
   */
  getTree(): ShortcutTree {
    const root: ShortcutTreeNode = { key: "", label: "", children: [] };
    // Longest sequences first so a branch always beats a leaf that is its prefix,
    // independent of React effect registration order.
    const entries = [...this.shortcuts.entries()]
      .flatMap(([id, sc]) => (sc.sequence && !sc.hidden ? [{ id, shortcut: sc, seq: sc.sequence }] : []))
      .sort((a, b) => b.seq.length - a.seq.length);
    for (const { id, shortcut, seq } of entries) {
      let node = root;
      let skipped = false;
      for (let i = 0; i < seq.length; i++) {
        const key = seq[i];
        const isLast = i === seq.length - 1;
        let child = node.children.find((c) => c.key === key);
        if (child?.shortcut) {
          skipped = true; // would extend (or duplicate) a leaf
          break;
        }
        if (!child) {
          child = { key, label: i === 0 ? shortcut.group ?? key : key, children: [] };
          node.children.push(child);
        } else if (i === 0 && shortcut.group && child.label === child.key) {
          child.label = shortcut.group;
        }
        if (isLast) {
          if (child.children.length > 0) {
            skipped = true; // a longer sequence already owns this key as a branch
            break;
          }
          child.shortcut = shortcut;
          child.label = shortcut.label;
        }
        node = child;
      }
      if (skipped) console.warn(`[ShortcutRegistry] Sequence ${JSON.stringify(seq)} of "${id}" collides with another binding — skipped.`);
    }
    return root.children;
  }

  /**
   * Register a provider whose shortcuts exist only while the command bar is under
   * `prefix` (called by resolveLazy). Keeps per-item actions (e.g. reply options
   * 1..N) off the registry at mount.
   */
  registerLazy(id: string, prefix: string[], provider: LazyProvider): () => void {
    const entry = { prefix, provider, cleanups: [] as Array<() => void> };
    this.lazyProviders.set(id, entry);
    return () => {
      entry.cleanups.forEach((c) => c());
      this.lazyProviders.delete(id);
    };
  }

  /** Drop every lazily provided shortcut (the bar closed). */
  releaseLazy(): void {
    for (const entry of this.lazyProviders.values()) {
      entry.cleanups.forEach((c) => c());
      entry.cleanups = [];
    }
  }

  /**
   * (Re)build lazily provided shortcuts whose prefix is compatible with `path`
   * (the path is a prefix of theirs, or theirs of it). With an empty path every
   * provider resolves, which the fuzzy palette needs. Returns when done; old
   * provided entries are replaced.
   */
  resolveLazy(path: string[]): void {
    for (const entry of this.lazyProviders.values()) {
      const n = Math.min(path.length, entry.prefix.length);
      const compatible = path.slice(0, n).every((k, i) => k === entry.prefix[i]);
      if (!compatible) continue;
      entry.cleanups.forEach((c) => c());
      entry.cleanups = entry.provider().map(({ id, shortcut }) => this.register(id, shortcut));
    }
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
    if (!hasModifier && isInputElement(event.target)) return;

    const activeContext = this.getActiveContext();

    for (const shortcut of this.shortcuts.values()) {
      if (shortcut.key === undefined && shortcut.code === undefined) continue;
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
  const keyOk = shortcut.key !== undefined && event.key === shortcut.key;
  const codeOk = shortcut.code !== undefined && event.code === shortcut.code;
  if (!keyOk && !codeOk) return false;
  const m = shortcut.modifiers ?? {};
  if (!!m.meta !== event.metaKey) return false;
  if (!!m.ctrl !== event.ctrlKey) return false;
  if (!!m.shift !== event.shiftKey) return false;
  if (!!m.alt !== event.altKey) return false;
  return true;
}

export function isInputElement(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return false;
  const el = target;
  const tag = el.tagName.toLowerCase();
  if (tag === "input" || tag === "textarea" || tag === "select") return true;
  if (el instanceof HTMLElement && el.isContentEditable) return true;
  return false;
}

/** Singleton registry — import this in components and hooks. */
export const registry = new ShortcutRegistry();
