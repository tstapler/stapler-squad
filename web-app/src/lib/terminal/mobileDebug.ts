/**
 * Local-only mobile terminal debug log. Off unless
 * localStorage['debug-terminal-mobile'] === 'true'; when off every method is a
 * no-op (no buffering, no console output, no stats). Nothing is sent anywhere.
 * Read it from a phone via window.__termDebug.dump() / .stats().
 */

export const DEBUG_FLAG_KEY = "debug-terminal-mobile";
export const MAX_DEBUG_ENTRIES = 500;
export const MISROUTE_OVERRIDE_WINDOW_MS = 10_000;
export const MISROUTE_TOOLBAR_WINDOW_MS = 5_000;

export interface DebugEntry {
  t: number;
  type: string;
  data: unknown;
}

export interface RouteDecision {
  target: string;
  source: string;
  unverified: boolean;
}

export interface MobileDebugStats {
  routeDecisions: number;
  overrideChanges: number;
  misroutes: { overrideAfterDrag: number; toolbarKeyAfterDrag: number };
}

export interface TermDebugApi {
  dump(): string;
  stats(): MobileDebugStats | null;
}

export interface MobileDebugOptions {
  /** Monotonic-enough millisecond clock; injected in tests. */
  now?: () => number;
  /** Flag reader, called on every log so toggling needs no reload. */
  isEnabled?: () => boolean;
  /** Receives each entry when enabled; defaults to console.debug. */
  sink?: (entry: DebugEntry) => void;
  /** Object that receives __termDebug; defaults to window when present. */
  target?: { __termDebug?: TermDebugApi };
}

const AUTO_SOURCE = "auto";

function readLocalStorageFlag(): boolean {
  try {
    return typeof localStorage !== "undefined" && localStorage.getItem(DEBUG_FLAG_KEY) === "true";
  } catch {
    return false; // storage can throw in private mode / sandboxed iframes
  }
}

export class MobileDebugLog {
  private readonly now: () => number;
  private readonly isEnabled: () => boolean;
  private readonly sink: (entry: DebugEntry) => void;
  private readonly target?: { __termDebug?: TermDebugApi };

  private readonly ring: (DebugEntry | undefined)[] = new Array(MAX_DEBUG_ENTRIES);
  private head = 0; // next write slot
  private size = 0;

  private lastAutoDragAt: number | null = null;
  private routeDecisions = 0;
  private overrideChanges = 0;
  private overrideAfterDrag = 0;
  private toolbarKeyAfterDrag = 0;

  constructor(options: MobileDebugOptions = {}) {
    this.now = options.now ?? Date.now;
    this.isEnabled = options.isEnabled ?? readLocalStorageFlag;
    this.sink = options.sink ?? ((entry) => console.debug("[term-debug]", entry.type, entry.data));
    this.target = options.target ?? (typeof window !== "undefined" ? (window as unknown as { __termDebug?: TermDebugApi }) : undefined);
  }

  /** Registers window.__termDebug. Safe to call repeatedly. */
  install(): void {
    if (!this.target) return;
    this.target.__termDebug = {
      dump: () => this.dump(),
      stats: () => this.stats(),
    };
  }

  /** Current flag value, for callers that cache it to skip building log payloads while off. */
  enabled(): boolean {
    return this.isEnabled();
  }

  log(type: string, data: unknown): void {
    if (!this.isEnabled()) return;
    const entry: DebugEntry = { t: this.now(), type, data };
    this.ring[this.head] = entry;
    this.head = (this.head + 1) % MAX_DEBUG_ENTRIES;
    if (this.size < MAX_DEBUG_ENTRIES) this.size++;
    this.sink(entry);
  }

  /** JSON array of buffered entries, oldest first. */
  dump(): string {
    const start = this.size < MAX_DEBUG_ENTRIES ? 0 : this.head;
    const entries: DebugEntry[] = [];
    for (let i = 0; i < this.size; i++) {
      entries.push(this.ring[(start + i) % MAX_DEBUG_ENTRIES] as DebugEntry);
    }
    return JSON.stringify(entries);
  }

  /** Counts since page load, or null when the debug flag is off. */
  stats(): MobileDebugStats | null {
    if (!this.isEnabled()) return null;
    return {
      routeDecisions: this.routeDecisions,
      overrideChanges: this.overrideChanges,
      misroutes: {
        overrideAfterDrag: this.overrideAfterDrag,
        toolbarKeyAfterDrag: this.toolbarKeyAfterDrag,
      },
    };
  }

  routeDecision(decision: RouteDecision): void {
    if (!this.isEnabled()) return;
    this.routeDecisions++;
    this.lastAutoDragAt = decision.source === AUTO_SOURCE ? this.now() : null;
    this.log("route-decision", decision);
  }

  overrideChange(from: string, to: string): void {
    if (!this.isEnabled()) return;
    this.overrideChanges++;
    this.log("override-change", { from, to });
    if (this.withinWindow(MISROUTE_OVERRIDE_WINDOW_MS)) {
      this.overrideAfterDrag++;
      this.log("misroute-proxy", { kind: "override-after-drag" });
    }
  }

  toolbarKey(key: string): void {
    if (!this.isEnabled()) return;
    if (this.withinWindow(MISROUTE_TOOLBAR_WINDOW_MS)) {
      this.toolbarKeyAfterDrag++;
      this.log("misroute-proxy", { kind: "toolbar-key-after-drag" });
    }
  }

  private withinWindow(windowMs: number): boolean {
    return this.lastAutoDragAt !== null && this.now() - this.lastAutoDragAt <= windowMs;
  }
}

/** Shared instance for app code; registered on window at first import in the browser. */
export const mobileDebug = new MobileDebugLog();
mobileDebug.install();
