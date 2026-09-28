"use client";

/**
 * Terminal Jank Elimination, Story 3 (Task 3.1) — Terminal Instance Pool.
 *
 * Keeps up to `maxSize` xterm.js `Terminal` instances (via `XtermTerminal`)
 * alive across a `TerminalOutput` remount, so switching which session a pane
 * shows no longer tears down and recreates the terminal's DOM/WebGL context.
 *
 * ## Why this exists (the actual jank source)
 *
 * `PaneSplitRenderer.tsx` keys its `<SessionDetail>` on the pane id AND the
 * assigned session id (see that file's `SessionDetail` render call) --
 * switching which session a pane displays fully unmounts and remounts the
 * whole `SessionDetail` -> `SessionDetailView` -> `TerminalOutput` subtree,
 * including `TerminalOutput`'s own local `<XtermTerminal>` instance.
 * `SessionDetailView`'s `pooledSessionIds` (kept-alive `TerminalOutput`
 * instances via CSS `visibility`) only helps for *within-view* tab switches;
 * it can't help across a pane's session reassignment because the whole view
 * — pool state included — is destroyed with it. `TerminalPoolProvider` lives
 * above that remount boundary (in `app/Providers.tsx`), so it survives it.
 *
 * ## Scope decision (see PR body for the full writeup)
 *
 * The doc's Task 3.1 sketch has each pool entry own a `useTerminalStream`
 * connection (WebSocket) in addition to the xterm.js `Terminal`. This
 * implementation deliberately does NOT move the WebSocket connection,
 * `TerminalStreamManager`, or `TerminalOutput`'s resync/flow-control/
 * scrollback-paging state into the pool — `TerminalOutput.tsx` is ~2500
 * lines of tightly-coupled state built on those living in one component
 * instance's lifetime, and re-plumbing all of it was judged too high-risk
 * to do safely in one pass. Instead, the pool owns exactly the thing whose
 * teardown/recreation is actually visible and expensive: the xterm.js
 * `Terminal` + its WebGL context + its DOM. `TerminalOutput` still owns a
 * fresh `useTerminalStream` connection per mount (fast, thanks to Story 2's
 * quiescence detector + snapshot cache) and simply reconnects it against the
 * SAME, already-populated Terminal instance -- old content stays visible
 * instantly instead of a blank terminal, and `TerminalStreamManager.write()`
 * already knows how to clear+replace on a full-pane snapshot (see
 * `ANSI_SNAPSHOT_PREFIX` in TerminalStreamManager.ts), which is exactly what
 * a fresh connect's initial snapshot is.
 *
 * ## Docking mechanism
 *
 * Each entry gets one persistent, detached host `<div>` created once
 * (`entry.hostEl`) holding a `createPortal(<XtermTerminal/>, entry.hostEl)`.
 * "Docking" a session to a consumer's anchor element is a plain
 * `anchor.appendChild(entry.hostEl)` -- moving an already-attached DOM node
 * via appendChild does not destroy/recreate it (no WebGL context loss, no
 * xterm.js re-init), unlike removing then reinserting. Undocked entries live
 * in a small always-attached "graveyard" container (`TerminalPool.css.ts`)
 * so xterm.js always has a real, laid-out container to measure. This avoids
 * both `position: fixed` + bounding-rect-tracking (what the doc's "portal
 * into document.body" sketch implies) and React's `createPortal` container
 * prop changing across renders (an unreliable way to reparent state --
 * `react-reverse-portal` exists specifically because of this gotcha).
 */

import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  Suspense,
  lazy,
} from "react";
import { createPortal } from "react-dom";
import type {
  XtermTerminalHandle,
  XtermTerminalProps,
} from "@/components/sessions/XtermTerminal";
import * as styles from "./TerminalPool.css";

const XtermTerminal = lazy(() =>
  import("@/components/sessions/XtermTerminal").then((m) => ({ default: m.XtermTerminal }))
) as React.ForwardRefExoticComponent<XtermTerminalProps & React.RefAttributes<XtermTerminalHandle>>;

export const DEFAULT_TERMINAL_POOL_MAX_SIZE = 8;

// Pooled terminals keep their buffer alive across switches, so scrollback is
// meaningful -- always 5000 regardless of what any individual consumer might
// otherwise request (Task 3.3). Never overridden per-entry.
const POOLED_SCROLLBACK = 5000;

// Fit-on-show retry cap (Bug 2, see usePooledTerminal's docking effect) --
// bounds the poll to ~5s at 60fps before giving up and logging a warning,
// rather than polling forever if a pane genuinely never lays out.
const FIT_ON_SHOW_MAX_ATTEMPTS = 300;

/** Per-entry state the pool owns for the lifetime of a pooled session's Terminal. */
interface PoolEntry {
  sessionId: string;
  /** Detached host node holding this entry's <XtermTerminal>. Identity never changes -- see module doc comment. */
  hostEl: HTMLDivElement;
  handleRef: React.RefObject<XtermTerminalHandle | null>;
  /**
   * True once this entry has ever received real content (TerminalOutput's
   * TerminalStreamManager.setOnFirstOutput fires). A consumer mounting
   * against an already-warm entry can skip its own loading overlay --
   * see usePooledTerminal's `warmRef` return and TerminalOutput.tsx's
   * `isLoadingInitialContent` initializer (Task 3.4's "no loading overlay,
   * no spinner" acceptance criterion for a warm switch). Never reset to
   * false once true, including across a consumer remount -- the pool entry
   * (and its Terminal's content) outlives any one consumer mount.
   */
  warmRef: React.RefObject<boolean>;
  /** Number of live consumers (usePooledTerminal callers) currently mounted for this session. Entries with a zero count are eligible for LRU eviction -- see pin()/unpin(). */
  pinCount: number;
  /** True while hostEl is appended to a live consumer's dock anchor rather than parked in the graveyard. Gates backend resize forwarding (Task 3.5 / Bug 7). */
  docked: boolean;
  lastAccessed: number;
  // Ref-mirrors for the currently-attached consumer's callbacks, updated by
  // usePooledTerminal on every render (same ref-mirror idiom TerminalOutput.tsx
  // already uses throughout). Stable wrapper functions below close over these
  // so <XtermTerminal>'s own props never need to change identity.
  onDataRef: React.RefObject<((data: string) => void) | null>;
  onResizeRef: React.RefObject<((cols: number, rows: number) => void) | null>;
  isAltScreenActiveRef: React.RefObject<(() => boolean) | null>;
  onAltScreenScrollUpRef: React.RefObject<((lines: number) => void) | null>;
  // Stable wrappers passed as <XtermTerminal> props, created once per entry.
  stableOnData: (data: string) => void;
  stableOnResize: (cols: number, rows: number) => void;
  stableIsAltScreenActive: () => boolean;
  stableOnAltScreenScrollUp: (lines: number) => void;
}

interface TerminalPoolContextValue {
  maxSize: number;
  getOrCreateEntry: (sessionId: string) => PoolEntry;
  /** Call from an effect (never during render) after getOrCreateEntry -- see useEntryLifecycle's doc comment. */
  registerEntry: () => void;
  pin: (sessionId: string) => void;
  unpin: (sessionId: string) => void;
  dock: (sessionId: string, anchor: HTMLElement) => void;
  undock: (sessionId: string) => void;
}

const TerminalPoolContext = createContext<TerminalPoolContextValue | null>(null);

// dockedIds lives in its OWN context, separate from TerminalPoolContextValue
// above. Every dock()/undock() call changes it, and TerminalPoolContextValue
// is a dependency of usePooledTerminal's docking effect -- folding dockedIds
// into that same object previously meant every dock/undock changed `pool`'s
// identity, re-triggering that effect, which called dock/undock again: an
// infinite update loop (caught by this file's own tests -- "Maximum update
// depth exceeded"). Splitting it out keeps TerminalPoolContextValue's
// function identities stable across dock/undock, while still letting a
// caller that genuinely wants introspection (tests, debugging) subscribe to
// dockedIds without destabilizing anything.
const DockedIdsContext = createContext<ReadonlySet<string>>(new Set());

/** sessionIds currently docked (visible) -- for tests/introspection; not needed for normal pool usage (see DockedIdsContext's doc comment for why it's separate from useTerminalPool()). */
export function useTerminalPoolDockedIds(): ReadonlySet<string> {
  return useContext(DockedIdsContext);
}

function createEntry(sessionId: string): PoolEntry {
  const hostEl = document.createElement("div");
  hostEl.className = styles.host;
  hostEl.dataset.pooledSessionId = sessionId;

  const onDataRef: React.RefObject<((data: string) => void) | null> = { current: null };
  const onResizeRef: React.RefObject<((cols: number, rows: number) => void) | null> = { current: null };
  const isAltScreenActiveRef: React.RefObject<(() => boolean) | null> = { current: null };
  const onAltScreenScrollUpRef: React.RefObject<((lines: number) => void) | null> = { current: null };

  return {
    sessionId,
    hostEl,
    handleRef: { current: null },
    warmRef: { current: false },
    pinCount: 0,
    docked: false,
    lastAccessed: Date.now(),
    onDataRef,
    onResizeRef,
    isAltScreenActiveRef,
    onAltScreenScrollUpRef,
    stableOnData: (data) => onDataRef.current?.(data),
    // Task 3.5 / Bug 7 — onResizeRef is only ever populated (non-null) while
    // the entry is docked for a foreground consumer, see usePooledTerminal;
    // a resize firing on a backgrounded/parked entry is a plain no-op here.
    stableOnResize: (cols, rows) => onResizeRef.current?.(cols, rows),
    stableIsAltScreenActive: () => isAltScreenActiveRef.current?.() ?? false,
    stableOnAltScreenScrollUp: (lines) => onAltScreenScrollUpRef.current?.(lines),
  };
}

export interface TerminalPoolProviderProps {
  children: React.ReactNode;
  maxSize?: number;
}

/**
 * Entry creation/eviction, factored out of usePoolManager purely to keep
 * function bodies short.
 *
 * `getOrCreateEntry` is called synchronously during a *consumer's* render
 * (usePooledTerminal needs the entry's stable `handleRef` available on the
 * very first render it returns). It therefore touches only `entriesRef` (a
 * plain ref, safe to mutate during any component's render) and never calls
 * `setEntryVersion` directly -- doing so triggered React's "Cannot update a
 * component while rendering a different component" warning (updating
 * TerminalPoolProvider's state from inside a child's render is unsupported,
 * unlike a component updating its own state during its own render).
 * `registerEntry`, called from `usePooledTerminal`'s effect (i.e. after
 * render commits, a fully supported time to update an ancestor's state), is
 * what actually bumps `entryVersion` so the portal list picks up the new
 * entry. A brand-new entry that gets docked before that effect fires simply
 * renders its `<XtermTerminal>` a tick later -- both effects flush in the
 * same pass, so there's no visible gap.
 */
function useEntryLifecycle(maxSize: number) {
  const entriesRef = useRef<Map<string, PoolEntry>>(new Map());
  // Pure re-render trigger -- entriesRef.current (a Map) is the source of
  // truth; this only exists so the portal list re-renders when entries are
  // added or evicted. Do not read from this for entry data.
  const [entryVersion, setEntryVersion] = useState(0);

  const evictLRU = useCallback((): boolean => {
    let victim: PoolEntry | null = null;
    for (const entry of entriesRef.current.values()) {
      if (entry.pinCount > 0 || entry.docked) continue; // never evict a live consumer (Bug: "evicted while still displayed")
      if (!victim || entry.lastAccessed < victim.lastAccessed) victim = entry;
    }
    if (!victim) return false; // pool is "full" of pinned/docked entries -- allowed to exceed maxSize rather than evict something on-screen
    entriesRef.current.delete(victim.sessionId);
    victim.hostEl.remove();
    return true;
  }, []);

  const getOrCreateEntry = useCallback((sessionId: string): PoolEntry => {
    const existing = entriesRef.current.get(sessionId);
    if (existing) {
      existing.lastAccessed = Date.now();
      return existing;
    }
    if (entriesRef.current.size >= maxSize) {
      evictLRU();
    }
    const entry = createEntry(sessionId);
    entriesRef.current.set(sessionId, entry);
    return entry;
  }, [maxSize, evictLRU]);

  // See this function's doc comment -- call only from an effect.
  const registerEntry = useCallback(() => {
    setEntryVersion((v) => v + 1);
  }, []);

  return { entriesRef, entryVersion, getOrCreateEntry, registerEntry };
}

/** pin/unpin/dock/undock, factored out of usePoolManager purely to keep function bodies short. */
function useDockingActions(entriesRef: React.RefObject<Map<string, PoolEntry>>, graveyardRef: React.RefObject<HTMLDivElement | null>) {
  const [dockedIds, setDockedIds] = useState<ReadonlySet<string>>(new Set());

  const pin = useCallback((sessionId: string) => {
    const entry = entriesRef.current.get(sessionId);
    if (entry) entry.pinCount += 1;
  }, [entriesRef]);

  const unpin = useCallback((sessionId: string) => {
    const entry = entriesRef.current.get(sessionId);
    if (entry) entry.pinCount = Math.max(0, entry.pinCount - 1);
  }, [entriesRef]);

  const dock = useCallback((sessionId: string, anchor: HTMLElement) => {
    const entry = entriesRef.current.get(sessionId);
    if (!entry) return;
    entry.lastAccessed = Date.now();
    entry.docked = true;
    // appendChild on an already-attached node moves it -- no DOM
    // destroy/recreate, no WebGL context loss (module doc comment).
    if (entry.hostEl.parentElement !== anchor) {
      anchor.appendChild(entry.hostEl);
    }
    setDockedIds((prev) => (prev.has(sessionId) ? prev : new Set(prev).add(sessionId)));
  }, [entriesRef]);

  const undock = useCallback((sessionId: string) => {
    const entry = entriesRef.current.get(sessionId);
    if (!entry) return;
    entry.docked = false;
    if (graveyardRef.current && entry.hostEl.parentElement !== graveyardRef.current) {
      graveyardRef.current.appendChild(entry.hostEl);
    }
    setDockedIds((prev) => {
      if (!prev.has(sessionId)) return prev;
      const next = new Set(prev);
      next.delete(sessionId);
      return next;
    });
  }, [entriesRef, graveyardRef]);

  return { pin, unpin, dock, undock, dockedIds };
}

/** Composes entry lifecycle + docking into the full pool API, factored out of TerminalPoolProvider purely to keep that component's own body short. */
function usePoolManager(maxSize: number, graveyardRef: React.RefObject<HTMLDivElement | null>) {
  const { entriesRef, entryVersion, getOrCreateEntry, registerEntry } = useEntryLifecycle(maxSize);
  const { pin, unpin, dock, undock, dockedIds } = useDockingActions(entriesRef, graveyardRef);

  // dockedIds is deliberately NOT a dependency here -- see DockedIdsContext's
  // doc comment for why folding it into this memo caused an infinite loop.
  const contextValue = useMemo<TerminalPoolContextValue>(() => ({
    maxSize, getOrCreateEntry, registerEntry, pin, unpin, dock, undock,
  }), [maxSize, getOrCreateEntry, registerEntry, pin, unpin, dock, undock]);

  return { entriesRef, entryVersion, contextValue, dockedIds };
}

export function TerminalPoolProvider({ children, maxSize = DEFAULT_TERMINAL_POOL_MAX_SIZE }: TerminalPoolProviderProps) {
  const graveyardRef = useRef<HTMLDivElement | null>(null);
  const { entriesRef, entryVersion, contextValue, dockedIds } = usePoolManager(maxSize, graveyardRef);

  // Preload xterm.js eagerly (mirrors TerminalOutput.tsx's prior per-mount
  // preload, now hoisted to the pool since it's the sole mount point).
  useEffect(() => {
    void import("@/components/sessions/XtermTerminal");
  }, []);

  const portals = useMemo(() => {
    return Array.from(entriesRef.current.values()).map((entry) =>
      createPortal(
        <Suspense fallback={null} key={entry.sessionId}>
          <XtermTerminal
            ref={entry.handleRef}
            onData={entry.stableOnData}
            onResize={entry.stableOnResize}
            theme="dark"
            fontSize={14}
            scrollback={POOLED_SCROLLBACK}
            isAltScreenActive={entry.stableIsAltScreenActive}
            onAltScreenScrollUp={entry.stableOnAltScreenScrollUp}
          />
        </Suspense>,
        entry.hostEl
      )
    );
    // entryVersion (not entriesRef, a ref) is the real dependency -- see its declaration.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [entryVersion]);

  return (
    <TerminalPoolContext.Provider value={contextValue}>
      <DockedIdsContext.Provider value={dockedIds}>
        {children}
        <div ref={graveyardRef} className={styles.graveyard} data-testid="terminal-pool-graveyard" aria-hidden="true" />
        {portals}
      </DockedIdsContext.Provider>
    </TerminalPoolContext.Provider>
  );
}

function useTerminalPoolContext(): TerminalPoolContextValue {
  const ctx = useContext(TerminalPoolContext);
  if (!ctx) {
    throw new Error("useTerminalPool/usePooledTerminal must be used within a TerminalPoolProvider");
  }
  return ctx;
}

/** Low-level pool API (Task 3.1's PoolEntry/TerminalPoolAPI sketch), for callers that need direct control rather than the usePooledTerminal convenience hook below. */
export function useTerminalPool(): TerminalPoolContextValue {
  return useTerminalPoolContext();
}

export interface UsePooledTerminalResult {
  /** Drop-in replacement for a locally-owned `useRef<XtermTerminalHandle | null>(null)` -- persists across this consumer's remounts as long as the pool keeps the entry alive. */
  xtermRef: React.RefObject<XtermTerminalHandle | null>;
  /** See PoolEntry.warmRef's doc comment. Consumers read `.current` once, synchronously, to decide their own initial loading-state -- never subscribe to it. */
  warmRef: React.RefObject<boolean>;
}

/**
 * Retries `fit()`+`focus()` on `entry` until `anchor` reports a real,
 * non-zero size AND `entry.handleRef` is populated (see usePooledTerminal's
 * docking effect for why neither is guaranteed on the frame docking
 * happens). Returns a cleanup function that cancels any pending retry.
 */
function fitOnShow(entry: PoolEntry, anchor: HTMLElement, sessionId: string): () => void {
  let settled = false;
  let rafId: number | null = null;
  let attempts = 0;

  const tryFit = () => {
    rafId = null;
    if (settled) return;
    const handle = entry.handleRef.current;
    const { width, height } = anchor.getBoundingClientRect();
    if (handle && width > 0 && height > 0) {
      settled = true;
      handle.fit();
      handle.focus();
      observer.disconnect();
      return;
    }
    attempts += 1;
    if (attempts >= FIT_ON_SHOW_MAX_ATTEMPTS) {
      console.warn(`[TerminalPool] Gave up waiting to fit ${sessionId} after ${attempts} frames (handle ready: ${!!handle}, size: ${width}x${height})`);
      return;
    }
    rafId = requestAnimationFrame(tryFit);
  };

  // Anchor size changing (including its first real layout, which fires once
  // even if it never changes again) is exactly the signal that a
  // previously-failed fit attempt might now succeed.
  const observer = new ResizeObserver(() => {
    if (rafId === null) rafId = requestAnimationFrame(tryFit);
  });
  observer.observe(anchor);
  rafId = requestAnimationFrame(tryFit);

  return () => {
    settled = true;
    if (rafId !== null) cancelAnimationFrame(rafId);
    observer.disconnect();
  };
}

/**
 * Consumer-facing hook (Task 3.4), part 1 of 2 -- see `usePooledTerminalCallbacks`
 * below for the other half. Split in two because a caller like
 * `TerminalOutput.tsx` needs `xtermRef` available before its own
 * onData/onResize/etc. callbacks exist (they're defined later in that
 * component and close over state declared in between) -- exactly the
 * temporal-dead-zone problem TerminalOutput.tsx's own ref-mirror callbacks
 * (e.g. `handleAppScrollbackFrameRef`) already solve the same way.
 *
 * Gets-or-creates this session's pool entry, pins it for the caller's mount
 * lifetime, and docks/undocks it against `dockRef` as `isVisible` changes,
 * including fit-on-show + focus (Task 3.5). Returns a stable `xtermRef`
 * usable exactly like the `useRef<XtermTerminalHandle | null>(null)` it
 * replaces, plus `warmRef` for the caller's own loading-state decision.
 */
export function usePooledTerminal(
  sessionId: string,
  dockRef: React.RefObject<HTMLDivElement | null>,
  isVisible: boolean
): UsePooledTerminalResult {
  const pool = useTerminalPoolContext();

  const entryRef = useRef<PoolEntry | null>(null);
  if (!entryRef.current || entryRef.current.sessionId !== sessionId) {
    entryRef.current = pool.getOrCreateEntry(sessionId);
  }
  const entry = entryRef.current;

  // Ensures the provider mounts (or keeps mounted) this entry's
  // <XtermTerminal> portal -- deferred to an effect rather than done inside
  // getOrCreateEntry's synchronous render-time call; see useEntryLifecycle's
  // doc comment for why.
  useEffect(() => {
    pool.registerEntry();
  }, [pool, sessionId]);

  // Pin for this consumer's mount lifetime (keeps the entry out of LRU
  // eviction while something is actually displaying it, even backgrounded --
  // SessionDetailView's own inner tab pool can background this consumer
  // without unmounting it).
  useEffect(() => {
    pool.pin(sessionId);
    return () => pool.unpin(sessionId);
  }, [pool, sessionId]);

  // Dock/undock + fit-on-show + focus (Task 3.5).
  //
  // Bug 2 (docs/tasks/terminal-jank.md "FitAddon on visibility:hidden
  // Terminals") — this used to fit() inside a single requestAnimationFrame,
  // assuming both (a) the anchor's CSS layout had already settled to its
  // final size and (b) the lazily-loaded <XtermTerminal> (see the `lazy(...)`
  // import above) had already mounted and populated `entry.handleRef`.
  // Neither is guaranteed on every dock: a window-switch's freshly-mounted
  // pane can still be mid-layout a frame later, and a cold pool entry's
  // XtermTerminal chunk may not have resolved yet. When the guess was wrong,
  // fit() ran against a stale/zero size (or was a no-op on a null ref) and
  // nothing re-fit it afterward, leaving a blank terminal until the user
  // clicked the manual "Resize" button. fitOnShow() polls (bounded) instead
  // of guessing one frame ahead.
  useEffect(() => {
    if (!isVisible) {
      pool.undock(sessionId);
      return;
    }
    const anchor = dockRef.current;
    if (!anchor) return;
    pool.dock(sessionId, anchor);
    const cancelFit = fitOnShow(entry, anchor, sessionId);
    return () => {
      cancelFit();
      // Also fires on unmount (not just isVisible->false) -- without this an
      // unmounted consumer leaves its entry stuck `docked: true`, permanently
      // exempt from evictLRU.
      pool.undock(sessionId);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- entry/dockRef are stable identities; re-run only on session/visibility change
  }, [pool, sessionId, isVisible]);

  return { xtermRef: entry.handleRef, warmRef: entry.warmRef };
}

export interface PooledTerminalCallbacks {
  onData: (data: string) => void;
  onResize: (cols: number, rows: number) => void;
  isAltScreenActive: () => boolean;
  onAltScreenScrollUp: (lines: number) => void;
}

/**
 * Consumer-facing hook (Task 3.4), part 2 of 2 -- call after
 * `usePooledTerminal` and after the callbacks in `callbacks` are defined
 * (see that hook's doc comment for why this is split out). Registers this
 * render's callback closures onto the entry's ref-mirrors so
 * `<XtermTerminal>` (mounted once, inside TerminalPoolProvider, never
 * re-rendered for this) always dispatches to the current consumer.
 */
export function usePooledTerminalCallbacks(
  sessionId: string,
  isVisible: boolean,
  { onData, onResize, isAltScreenActive, onAltScreenScrollUp }: PooledTerminalCallbacks
): void {
  const pool = useTerminalPoolContext();
  // Entry must already exist -- usePooledTerminal (called earlier in the
  // same render, per its own doc comment) created it. getOrCreateEntry is a
  // cheap idempotent lookup here, not a real creation.
  const entry = pool.getOrCreateEntry(sessionId);

  // Task 3.6 rapid-switch guard: always point the entry's callback refs at
  // THIS render's closures. A write/resize that arrives after this consumer
  // unmounts can only reach a ref the cleanup below has already nulled out
  // (guarded so it only clears its OWN callback, never a still-live
  // successor's that took over first).
  useEffect(() => {
    entry.onDataRef.current = onData;
    // Bug 7 / Task 3.5 — never forward resizes to a backgrounded consumer's
    // backend resize() call, even if a stray ResizeObserver tick fires on a
    // parked (graveyard) or momentarily-hidden entry.
    entry.onResizeRef.current = isVisible ? onResize : null;
    entry.isAltScreenActiveRef.current = isAltScreenActive;
    entry.onAltScreenScrollUpRef.current = onAltScreenScrollUp;
    return () => {
      if (entry.onDataRef.current === onData) entry.onDataRef.current = null;
      if (entry.onResizeRef.current === onResize) entry.onResizeRef.current = null;
      if (entry.isAltScreenActiveRef.current === isAltScreenActive) entry.isAltScreenActiveRef.current = null;
      if (entry.onAltScreenScrollUpRef.current === onAltScreenScrollUp) entry.onAltScreenScrollUpRef.current = null;
    };
  }, [entry, isVisible, onData, onResize, isAltScreenActive, onAltScreenScrollUp]);
}
