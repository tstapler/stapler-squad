"use client";

import { useEffect, useRef, useCallback, useImperativeHandle, forwardRef, useState } from "react";
import { createPortal } from "react-dom";
import { useTerminalGestures } from "@/lib/hooks/useTerminalGestures";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import type { WebglAddon } from "@xterm/addon-webgl";
import { CanvasAddon } from "@xterm/addon-canvas";
import { SearchAddon } from "@xterm/addon-search";
import { SerializeAddon } from "@xterm/addon-serialize";
import "@xterm/xterm/css/xterm.css";
import * as styles from "./XtermTerminal.css";
import { TerminalContextMenu } from "./TerminalContextMenu";
import { loadTerminalConfig, darkTerminalTheme, lightTerminalTheme, type TerminalConfig } from "@/lib/config/terminalConfig";
import { dimensionsEqual, isFiniteResizeDimensions, type ResizeDimensions } from "@/lib/terminal/types";
import { getCellDimensions } from "@/lib/terminal/cellDimensions";
import { isMouseTracking } from "@/lib/terminal/mouseTracking";

const DEFAULT_SCROLLBACK_SIZE = 5000;

/**
 * Fixed cadence (ms) of the decoupled resize sampler. See
 * project_plans/terminal-resize-fit-loop/decisions/ADR-002-decoupled-sampler-tick-semantics.md
 *
 * Exported so tests import the real cadence instead of redeclaring a local
 * copy that could silently desync from the implementation.
 */
export const SAMPLE_INTERVAL_MS = 50;

/**
 * Bounded give-up threshold (~1s of sampling) for sustained oscillation. See
 * project_plans/terminal-resize-fit-loop/decisions/ADR-002-decoupled-sampler-tick-semantics.md
 *
 * Exported so tests import the real threshold instead of redeclaring a local
 * copy that could silently desync from the implementation.
 */
export const MAX_SAMPLES = 20;

/**
 * Tolerance (px) and consecutive-sample threshold for the WebGL actual-vs-
 * expected pixels-per-column mismatch tracker (AC5). Provisional values, not
 * yet validated against a real fractionally-scaled display — jsdom cannot
 * reproduce real WebGL glyph-width mismatch magnitude, especially under
 * fractional OS display scaling (Windows 125%/150%, macOS non-integer zoom),
 * which is exactly the condition that produces this mismatch in the first
 * place. Chosen as a reasonable starting point from requirements.md's own
 * "warns above a 1px tolerance" precedent, not measured data. See Task 5.2
 * step 7 in project_plans/terminal-resize-fit-loop/implementation/plan.md
 * for the real-device validation/tuning step.
 */
const MISMATCH_TOLERANCE_PX = 1;
const MISMATCH_THRESHOLD = 3;

export interface ShouldScheduleFitResult {
  schedule: boolean;
  nextPending: ResizeDimensions | null;
}

/**
 * Pure Reading-A dead-band decision: a fit() should only be scheduled once a
 * proposed candidate matches the immediately preceding sampled candidate
 * exactly (not merely "differs from applied"). See ADR-002 §2 for the full
 * derivation of why Reading A is correct and Reading B is not.
 */
export function shouldScheduleFit(
  proposed: ResizeDimensions | undefined,
  applied: ResizeDimensions,
  pending: ResizeDimensions | null
): ShouldScheduleFitResult {
  if (!proposed) return { schedule: false, nextPending: null };
  if (dimensionsEqual(proposed, applied)) {
    return { schedule: false, nextPending: null };
  }
  if (pending && dimensionsEqual(pending, proposed)) {
    return { schedule: true, nextPending: null };
  }
  return { schedule: false, nextPending: proposed };
}

export interface CellMismatchInputs {
  actualPxPerCol: number;
  expectedPxPerCol: number;
}

/**
 * Impure extraction of the raw actual-vs-expected pixels-per-column inputs
 * from xterm.js internals and DOM measurement (AC5). Returns null when the
 * renderer hasn't measured cell dimensions yet. Deliberately does no
 * Number.isFinite guarding here — `terminal.cols === 0` simply produces
 * `Infinity` and is passed through; `isSustainedMismatch()` is the sole
 * guard boundary (architecture-review.md Concern 2).
 */
export function extractCellMismatchInputs(
  terminal: Terminal,
  containerEl: HTMLElement
): CellMismatchInputs | null {
  const dims = (terminal as any)._core?._renderService?.dimensions;
  if (!dims?.css?.cell?.width) return null;
  return {
    actualPxPerCol: containerEl.getBoundingClientRect().width / terminal.cols,
    expectedPxPerCol: dims.css.cell.width,
  };
}

/**
 * Pure, Number.isFinite-guarded mismatch decision (AC5). Returns false
 * unless both inputs are finite — guards against the `terminal.cols === 0`
 * / hidden-tab `Infinity` case (pitfalls §4) using `Number.isFinite`, not
 * `Number.isNaN` (per AC5's explicit wording; `Number.isNaN(Infinity)` is
 * `false`, which would incorrectly admit the sample).
 */
export function isSustainedMismatch(
  actualPxPerCol: number,
  expectedPxPerCol: number,
  tolerance: number
): boolean {
  if (!Number.isFinite(actualPxPerCol) || !Number.isFinite(expectedPxPerCol)) {
    return false;
  }
  return Math.abs(actualPxPerCol - expectedPxPerCol) > tolerance;
}

export interface XtermTerminalProps {
  /**
   * Callback when user types in terminal
   */
  onData?: (data: string) => void;

  /**
   * Callback when terminal is resized
   */
  onResize?: (cols: number, rows: number) => void;

  /**
   * Terminal theme (overrides config if provided)
   */
  theme?: "light" | "dark";

  /**
   * Font size in pixels (overrides config if provided)
   */
  fontSize?: number;

  /**
   * Scrollback buffer size in lines (overrides config if provided)
   */
  scrollback?: number;

  /**
   * Use terminal configuration from localStorage
   * If true, theme/fontSize/scrollback props are ignored unless explicitly provided
   */
  useConfig?: boolean;
}

export interface XtermTerminalHandle {
  terminal: Terminal | null;
  /** SerializeAddon instance for buffer serialization (used by TerminalStreamManager for scrollback prepend). */
  serializeAddon: SerializeAddon | null;
  write: (data: string) => void;
  writeln: (data: string) => void;
  clear: () => void;
  focus: () => void;
  fit: () => void;
  /**
   * Directly set the terminal's grid size in cols/rows, bypassing FitAddon's pixel-based
   * measurement. Used to synchronously match the terminal buffer to a pre-calculated size
   * (from cached cell metrics) before the initial capture-pane snapshot streams in — otherwise
   * ANSI cursor-positioning sequences targeting rows beyond the xterm.js default (80x24) are
   * silently dropped, leaving those rows unpainted until a later resize forces a full repaint.
   */
  resize: (cols: number, rows: number) => void;
  search: (term: string) => boolean;
  searchNext: (term: string) => boolean;
  searchPrevious: (term: string) => boolean;
}

/**
 * XtermTerminal - React wrapper for xterm.js terminal emulator
 *
 * Features:
 * - Canvas-based rendering (10-100x faster than DOM)
 * - WebGL acceleration (2x faster than canvas)
 * - Automatic resizing with FitAddon
 * - Clickable web links
 * - Search functionality
 * - Mouse event reporting (drag-to-select, clicks, etc.)
 * - Professional terminal UX
 */
export const XtermTerminal = forwardRef<XtermTerminalHandle, XtermTerminalProps>(({
  onData,
  onResize,
  theme: themeProp,
  fontSize: fontSizeProp,
  scrollback: scrollbackProp,
  useConfig = false,
}, ref) => {
  // Load configuration
  const config = useConfig ? loadTerminalConfig() : null;

  // Use props or config values
  const theme = themeProp ?? config?.theme ?? "dark";
  const fontSize = fontSizeProp ?? config?.fontSize ?? 14;
  const scrollback = scrollbackProp ?? config?.scrollbackLines ?? 0;
  // Mouse tracking mode is set at runtime by PTY escape sequences and read via terminal.modes.mouseTrackingMode.
  // It is not configurable via prop — the 'mouseTracking' ITerminalOptions field does not exist in xterm.js 6.
  const fontFamily = config?.fontFamily ?? 'Menlo, Monaco, "Courier New", monospace';
  const cursorStyle = config?.cursorStyle ?? "block";
  const cursorBlink = config?.cursorBlink ?? true;

  const terminalRef = useRef<Terminal | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const searchAddonRef = useRef<SearchAddon | null>(null);
  const webglAddonRef = useRef<WebglAddon | null>(null);
  const serializeAddonRef = useRef<SerializeAddon | null>(null);
  const lastSizeRef = useRef<{ cols: number; rows: number } | null>(null);

  // Refs for floating Copy button and toast — avoid React re-renders on 60fps selection changes
  const copyButtonRef = useRef<HTMLButtonElement>(null);
  const toastRef = useRef<HTMLDivElement>(null);
  // Draggable selection handles for mobile — DOM-mutated at 60fps alongside the copy button
  const startHandleRef = useRef<HTMLDivElement>(null);
  const endHandleRef = useRef<HTMLDivElement>(null);
  // Custom left-side scrollbar track and thumb
  const scrollTrackRef = useRef<HTMLDivElement>(null);
  const scrollThumbRef = useRef<HTMLDivElement>(null);

  // Context menu uses useState (shown at most once per right-click — not a hot path)
  const [contextMenuState, setContextMenuState] = useState<{ x: number; y: number } | null>(null);

  // Store callbacks in refs to avoid recreating terminal on callback changes
  const onDataRef = useRef(onData);
  const onResizeRef = useRef(onResize);

  useEffect(() => {
    onDataRef.current = onData;
    onResizeRef.current = onResize;
  }, [onData, onResize]);

  // Unified mobile gesture state machine (R4.3).
  // Replaces the conflicting useTouchScroll + useMobileTerminalGestures hooks:
  // having both register touchmove caused double-scroll and prevented selection.
  // Pass terminalRef (the RefObject itself, not .current) so gesture handlers always
  // read the live terminal instance — at render time terminalRef.current is null since
  // the terminal is created inside an effect (Bug 1 fix).
  useTerminalGestures({
    containerRef,
    terminalRef,
    onSendData: useCallback((data: string) => onDataRef.current?.(data), []),
  });

  // Show the "Copied/Copy failed" toast via DOM mutation (no re-render).
  // Defined as useCallback with empty deps — only reads stable refs.
  const showToast = useCallback((status: 'copied' | 'failed') => {
    const toast = toastRef.current;
    if (!toast) return;
    toast.textContent = status === 'copied' ? 'Copied' : 'Copy failed';
    // Remove animation class first to allow restart; force reflow before re-adding.
    toast.classList.remove(styles.copiedToastVisible);
    toast.style.display = 'block';
    void toast.offsetHeight;
    toast.classList.add(styles.copiedToastVisible);
    setTimeout(() => {
      if (toastRef.current) {
        toastRef.current.style.display = 'none';
        toastRef.current.classList.remove(styles.copiedToastVisible);
      }
    }, 1500);
  }, []);

  // execCommand fallback for browsers that deny clipboard API
  const execCommandCopy = useCallback((text: string): boolean => {
    const el = document.createElement('textarea');
    el.value = text;
    document.body.appendChild(el);
    el.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(el);
    return ok;
  }, []);

  // "Copy all" scrollback handler — extracts plain text from xterm buffer (no ANSI codes).
  // Uses onPointerDown (not onClick) to stay in a synchronous user-gesture context for
  // iOS Safari, which only allows clipboard writes inside unbroken gesture handlers.
  const handleCopyScrollbackPointerDown = useCallback((e: React.PointerEvent) => {
    e.preventDefault();
    const terminal = terminalRef.current;
    if (!terminal) return;

    const buffer = terminal.buffer.active;
    const lineStrings: string[] = [];
    for (let i = 0; i < buffer.length; i++) {
      const line = buffer.getLine(i);
      if (line) {
        // translateToString(true) trims trailing whitespace — gives clean plain text
        lineStrings.push(line.translateToString(true));
      }
    }
    const text = lineStrings.join('\n').trimEnd();
    if (!text) return;

    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(text)
        .then(() => showToast('copied'))
        .catch(() => showToast(execCommandCopy(text) ? 'copied' : 'failed'));
    } else {
      showToast(execCommandCopy(text) ? 'copied' : 'failed');
    }
  }, [showToast, execCommandCopy]);

  // Floating Copy button pointer-down handler — synchronous user gesture path (iOS safe)
  const handleCopyButtonPointerDown = useCallback((e: React.PointerEvent) => {
    const terminal = terminalRef.current;
    if (!terminal) return;
    const text = terminal.getSelection();
    if (!text) return;

    if (copyButtonRef.current) {
      copyButtonRef.current.style.display = 'none';
    }

    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(text).then(() => {
        showToast('copied');
      }).catch(() => {
        const ok = execCommandCopy(text);
        showToast(ok ? 'copied' : 'failed');
      });
    } else {
      const ok = execCommandCopy(text);
      showToast(ok ? 'copied' : 'failed');
    }
    e.preventDefault();
  }, [showToast, execCommandCopy]);

  // Context menu action handlers
  const handleMenuCopy = useCallback(() => {
    const terminal = terminalRef.current;
    if (!terminal) return;
    const text = terminal.getSelection();
    if (!text) return;
    if (navigator.clipboard?.writeText) {
      navigator.clipboard.writeText(text)
        .then(() => showToast('copied'))
        .catch(() => {
          const ok = execCommandCopy(text);
          showToast(ok ? 'copied' : 'failed');
        });
    } else {
      const ok = execCommandCopy(text);
      showToast(ok ? 'copied' : 'failed');
    }
    setContextMenuState(null);
  }, [showToast, execCommandCopy]);

  const handleMenuSelectAll = useCallback(() => {
    terminalRef.current?.selectAll();
    setContextMenuState(null);
  }, []);

  const handleMenuPaste = useCallback(() => {
    navigator.clipboard.readText().then((text) => {
      onDataRef.current?.(text);
    }).catch(() => {
      // Clipboard read permission denied — silently ignore
    });
    setContextMenuState(null);
  }, []);

  const handleContextMenuDismiss = useCallback(() => {
    setContextMenuState(null);
  }, []);

  // Sync the custom left-side scrollbar with the terminal's current viewport.
  // Called from onScroll, onResize, and after the initial fit — all via direct
  // DOM mutation so there's no React re-render overhead on every scroll event.
  const updateScrollbar = useCallback((term: Terminal) => {
    const track = scrollTrackRef.current;
    const thumb = scrollThumbRef.current;
    if (!track || !thumb) return;
    const buf = term.buffer.active;
    const totalLines = buf.length;
    const visibleLines = term.rows;
    if (totalLines <= visibleLines) {
      track.style.display = 'none';
      return;
    }
    track.style.display = 'block';
    const trackHeight = track.clientHeight;
    if (trackHeight === 0) return;
    const thumbHeight = Math.max(20, trackHeight * (visibleLines / totalLines));
    const maxScrollLines = totalLines - visibleLines;
    const progress = maxScrollLines > 0 ? buf.viewportY / maxScrollLines : 0;
    const thumbTop = progress * (trackHeight - thumbHeight);
    thumb.style.height = `${thumbHeight}px`;
    thumb.style.transform = `translateY(${thumbTop}px)`;
  }, []);

  // Initialize terminal on mount
  useEffect(() => {
    // SSR guard
    if (typeof window === 'undefined') {
      console.warn('[XtermTerminal] SSR detected, terminal requires client-side rendering');
      return;
    }

    if (!containerRef.current || terminalRef.current) return;

    // Create terminal instance with configuration
    // Note: mouseTracking is NOT set here — it is not a valid ITerminalOptions field in xterm.js 6.
    // Mouse tracking mode is set at runtime by PTY escape sequences and read via terminal.modes.mouseTrackingMode.
    const terminal = new Terminal({
      cursorBlink,
      cursorStyle,
      fontSize,
      fontFamily,
      theme: getTheme(theme),
      scrollback: scrollback && scrollback > 0 ? scrollback : DEFAULT_SCROLLBACK_SIZE,
      allowProposedApi: true, // Required for some addons
      rightClickSelectsWord: true, // Right-click selects the word under cursor
    });

    // WebGL mismatch tracker + one-directional Canvas fallback (AC5). See
    // project_plans/terminal-resize-fit-loop/decisions/ADR-001-add-xterm-addon-canvas-dependency.md
    let webglMismatchCount = 0;
    let webglFallbackTriggered = false;
    // RAF handle for the post-fallback fit() scheduled inside
    // triggerCanvasFallback()'s success path, so it can be cancelled on
    // unmount if the component tears down before the RAF fires.
    let postFallbackRafId: number | null = null;

    const triggerCanvasFallback = () => {
      if (webglFallbackTriggered || cancelled) return; // one-directional latch, never re-arms (pitfalls §4)
      webglFallbackTriggered = true;
      console.warn('[XtermTerminal] WebGL cell-measurement mismatch exceeded threshold, falling back to canvas renderer');

      // @xterm/addon-webgl resolved to 0.18.0 (confirmed in package-lock.json /
      // node_modules). This postdates the historical WebglAddon.dispose()
      // no-op bug (xterm.js #2254, fixed via #2548, a 2019-era fix long since
      // released). The GPU-memory-leak-on-dispose fix (#3889, fixed via
      // #3890) is also merged upstream, but a lightweight web search could not
      // definitively pin the exact release/version boundary where #3890
      // landed relative to 0.18.0 — noting that explicitly rather than
      // asserting an unverified claim (Task 3.0.2).
      webglAddonRef.current?.dispose();
      webglAddonRef.current = null;

      try {
        terminal.loadAddon(new CanvasAddon());
        // Wait one RAF frame after the addon swap before fitting, per the
        // historical xterm.js #1416 crash precedent (measuring against a
        // not-yet-initialized renderer).
        postFallbackRafId = requestAnimationFrame(() => {
          postFallbackRafId = null;
          const proposed = fitAddonRef.current?.proposeDimensions();
          if (fitAddonRef.current && isFiniteResizeDimensions(proposed)) {
            fitAddonRef.current.fit();
          } else {
            console.warn('[XtermTerminal] Skipped post-fallback fit: proposed dimensions not finite');
          }
        });
      } catch (err) {
        // adversarial-review.md Blocker: CanvasAddon construction must be
        // guarded, mirroring the WebglAddon try/catch above. If this also
        // throws, the latch stays tripped (no retry) and xterm.js's built-in
        // DOM renderer is left active automatically — no explicit fallback
        // code path is needed (confirmed by build-vs-buy.md research).
        console.error("[XtermTerminal] Canvas renderer also failed to load; falling back to xterm's built-in DOM renderer", err);
      }
    };

    // Single check-and-maybe-trigger-fallback sequence, shared by both the
    // startup double-RAF check and the sampler's per-confirmed-resize check
    // (previously duplicated with inconsistent guard placement). `inputs` is
    // null when the renderer hasn't measured cell dimensions yet -- that's
    // "no sample", not a clean sample, so it neither increments nor resets
    // the counter. A genuinely clean (non-mismatching) sample resets the
    // counter to 0, making MISMATCH_THRESHOLD a true consecutive-sample
    // threshold rather than a cumulative one (AC5's "sustained" mismatch).
    const recordMismatchSample = (inputs: CellMismatchInputs | null) => {
      if (webglFallbackTriggered || !inputs) return;
      if (isSustainedMismatch(inputs.actualPxPerCol, inputs.expectedPxPerCol, MISMATCH_TOLERANCE_PX)) {
        console.error(`[XtermTerminal] ⚠️ SIZING MISMATCH! Container width doesn't match cell width calculation`);
        webglMismatchCount++;
        if (webglMismatchCount >= MISMATCH_THRESHOLD) {
          triggerCanvasFallback();
        }
      } else {
        webglMismatchCount = 0;
      }
    };

    // Dev-only manual trigger (Task 3.2.1a): lets a human visually confirm
    // the Canvas tier renders correctly without waiting for the mismatch
    // heuristic or a real WebGL context loss (jsdom cannot exercise either
    // path for real, so this is otherwise unverifiable pre-ship).
    //
    // Scoped to this instance's container DOM node (not `window`) so that
    // multiple concurrently mounted XtermTerminal instances -- the normal
    // case in this multi-session app -- each get their own independent
    // trigger instead of clobbering one another's global hook on mount/
    // unmount (Task 5.2 step 7 manual multi-instance verification).
    if (typeof window !== "undefined" && localStorage.getItem("debug-terminal") === "true" && containerRef.current) {
      (containerRef.current as any).__staplerSquadForceCanvasFallback = () => triggerCanvasFallback();
    }

    // Create and load addons
    const fitAddon = new FitAddon();
    const webLinksAddon = new WebLinksAddon();
    const searchAddon = new SearchAddon();
    const serializeAddon = new SerializeAddon();

    terminal.loadAddon(fitAddon);
    terminal.loadAddon(webLinksAddon);
    terminal.loadAddon(searchAddon);
    terminal.loadAddon(serializeAddon);

    // xterm.js issue #2033 — guard WebGL before loading on Android/mobile.
    // Loaded dynamically (rather than the always-on static import this
    // replaced) so platforms lacking WebGL2RenderingContext never pull the
    // addon-webgl module at all. webglAddonRef is still populated (instead of
    // a locally-scoped const) so triggerCanvasFallback()'s dispose() call and
    // the unmount cleanup effect below can reach the live instance, and
    // onContextLoss routes through triggerCanvasFallback() so a real context
    // loss is treated identically to the AC5 mismatch-tracker fallback path.
    // Guards against the async import below resolving after this effect has
    // already been cleaned up (e.g. a fast session-switch unmount) — without
    // this, loadAddon() would be called on an already-disposed terminal and
    // webglAddonRef.current would be left pointing at an orphaned, undisposed
    // WebGL addon that leaks its context. Set to true in the cleanup below.
    let cancelled = false;
    (async () => {
      if (typeof WebGL2RenderingContext !== 'undefined') {
        try {
          const { WebglAddon } = await import('@xterm/addon-webgl');
          if (cancelled) return;
          webglAddonRef.current = new WebglAddon();
          terminal.loadAddon(webglAddonRef.current);
          webglAddonRef.current.onContextLoss(() => {
            console.warn('[XtermTerminal] WebGL context lost, falling back to canvas renderer');
            triggerCanvasFallback();
          });
          console.log("[XtermTerminal] WebGL renderer enabled");
        } catch (e) {
          console.warn("[XtermTerminal] WebGL failed to load:", e);
        }
      } else {
        console.log("[XtermTerminal] WebGL2 unavailable (Android?), using canvas renderer");
      }
    })();

    // Open terminal in container with error boundary
    try {
      terminal.open(containerRef.current);

      // Attach contextmenu listener after terminal.open() — terminal.element is non-null after open().
      // Guard against null (can occur in test environments where open() is mocked as no-op).
      const termElement = terminal.element;
      const handleContextMenu = (e: MouseEvent) => {
        e.preventDefault(); // always suppress browser native menu
        if (isMouseTracking(terminal)) return; // let PTY handle right-click via VT sequences
        setContextMenuState({ x: e.clientX, y: e.clientY });
      };
      if (termElement) {
        termElement.addEventListener('contextmenu', handleContextMenu);
      }

      // Custom key handler — a single handler for all shortcuts (attaching multiple replaces).
      // All key intercepts must be here. No cleanup needed; tied to terminal instance lifecycle.
      // iOS note: navigator.clipboard.writeText may fail in keydown because keydown events may
      // not carry the same user-gesture trust as pointer events. iOS users don't have Ctrl+C
      // hardware — they use the floating Copy button (onPointerDown path) instead.
      terminal.attachCustomKeyEventHandler?.((event: KeyboardEvent): boolean => {
        if (event.type !== 'keydown') return true;

        const isCopyShortcut = (event.ctrlKey || event.metaKey) && event.key === 'c';
        const isSelectAllShortcut = (event.ctrlKey || event.metaKey) && event.key === 'a';

        if (isCopyShortcut && terminal.getSelection().length > 0) {
          const text = terminal.getSelection();
          const toast = toastRef.current;
          const showToastInHandler = (status: 'copied' | 'failed') => {
            if (!toast) return;
            toast.textContent = status === 'copied' ? 'Copied' : 'Copy failed';
            toast.classList.remove(styles.copiedToastVisible);
            toast.style.display = 'block';
            void toast.offsetHeight;
            toast.classList.add(styles.copiedToastVisible);
            setTimeout(() => {
              if (toastRef.current) {
                toastRef.current.style.display = 'none';
                toastRef.current.classList.remove(styles.copiedToastVisible);
              }
            }, 1500);
          };
          const execFallback = (t: string): boolean => {
            const el = document.createElement('textarea');
            el.value = t;
            document.body.appendChild(el);
            el.select();
            const ok = document.execCommand('copy');
            document.body.removeChild(el);
            return ok;
          };
          if (navigator.clipboard?.writeText) {
            navigator.clipboard.writeText(text)
              .then(() => showToastInHandler('copied'))
              .catch(() => {
                const ok = execFallback(text);
                showToastInHandler(ok ? 'copied' : 'failed');
              });
          } else {
            const ok = execFallback(text);
            showToastInHandler(ok ? 'copied' : 'failed');
          }
          terminal.clearSelection();
          return false;
        }

        if (isSelectAllShortcut) {
          terminal.selectAll();
          return false;
        }

        return true;
      });

      // CRITICAL: Wait for browser to complete layout before fitting
      // Use requestAnimationFrame to ensure DOM is rendered and measurements are accurate
      requestAnimationFrame(() => {
        requestAnimationFrame(() => {
          // Double RAF ensures layout is stable before FitAddon measures dimensions
          const containerEl = containerRef.current;
          if (containerEl) {
            const rect = containerEl.getBoundingClientRect();
            console.log(`[XtermTerminal] Container size before fit: ${rect.width}px × ${rect.height}px`);
          }

          // Log what FitAddon will see
          const proposedDims = fitAddon.proposeDimensions();
          console.log(`[XtermTerminal] Proposed dimensions:`, proposedDims);

          // Check if cell dimensions are available (via private API for debugging)
          const dims = (terminal as any)._core?._renderService?.dimensions;
          if (dims?.css?.cell) {
            console.log(`[XtermTerminal] Cell dimensions: ${dims.css.cell.width}px × ${dims.css.cell.height}px`);
          } else {
            console.warn(`[XtermTerminal] Cell dimensions not available yet!`);
          }

          fitAddon.fit();
          updateScrollbar(terminal);

          console.log(`[XtermTerminal] Initial fit complete: ${terminal.cols} cols × ${terminal.rows} rows`);

          // Calculate actual pixels per column for verification (AC5 mismatch tracker)
          if (containerEl) {
            const mismatchInputs = extractCellMismatchInputs(terminal, containerEl);
            if (mismatchInputs) {
              console.log(`[XtermTerminal] Actual pixels per column: ${mismatchInputs.actualPxPerCol.toFixed(2)}px`);
              console.log(`[XtermTerminal] Expected pixels per column: ${mismatchInputs.expectedPxPerCol.toFixed(2)}px`);
            }
            recordMismatchSample(mismatchInputs);
          }
          // Secondary delayed fit() removed (R1.3): the double-rAF above provides sufficient layout
          // stability; the extra setTimeout caused a second terminal.onResize, triggering a duplicate
          // server resize RPC and second capture-pane cycle (double-resize corruption on mount).
        });
      });

      // Cleanup includes contextmenu listener removal
      const cleanupContextMenu = () => {
        if (termElement) termElement.removeEventListener('contextmenu', handleContextMenu);
      };

      // Setup event handlers using refs to avoid recreating terminal
      const dataDisposable = terminal.onData((data) => {
        onDataRef.current?.(data);
      });

      // Show floating Copy button on selection change via direct DOM mutation (no setState).
      // onSelectionChange fires at up to 60fps during mouse drag — setState here would cause
      // a re-render storm. Direct ref mutation costs ~0.01ms vs ~3ms for React reconcile.
      // true once on mount — pointer:coarse means a touch-primary device
      const isTouchPrimary = typeof window !== 'undefined' && window.matchMedia('(pointer: coarse)').matches;

      const selectionDisposable = terminal.onSelectionChange(() => {
        const btn = copyButtonRef.current;
        if (!btn) return;
        const text = terminal.getSelection();
        if (text && text.length > 0) {
          const pos = terminal.getSelectionPosition();
          if (pos && terminal.element) {
            const rect = terminal.element.getBoundingClientRect();
            const { cellH, cellW } = getCellDimensions(terminal);
            btn.style.left = `${rect.left + pos.end.x * cellW}px`;
            btn.style.top = `${rect.top + pos.end.y * cellH - 40}px`;
            btn.style.display = 'block';

            // Draggable selection handles — mobile only
            if (isTouchPrimary) {
              const sh = startHandleRef.current;
              const eh = endHandleRef.current;
              if (sh && eh) {
                // Start handle: left edge of first selected character, below the row
                sh.style.left = `${rect.left + pos.start.x * cellW}px`;
                sh.style.top = `${rect.top + (pos.start.y + 1) * cellH}px`;
                sh.style.display = 'block';
                // End handle: right edge of last selected character, below the row
                eh.style.left = `${rect.left + (pos.end.x + 1) * cellW}px`;
                eh.style.top = `${rect.top + (pos.end.y + 1) * cellH}px`;
                eh.style.display = 'block';
              }
            }
          }
        } else {
          btn.style.display = 'none';
          startHandleRef.current && (startHandleRef.current.style.display = 'none');
          endHandleRef.current && (endHandleRef.current.style.display = 'none');
        }
      });

      const resizeDisposable = terminal.onResize(({ cols, rows }) => {
        // Only trigger callback if size actually changed
        const lastSize = lastSizeRef.current;
        if (!lastSize || lastSize.cols !== cols || lastSize.rows !== rows) {
          lastSizeRef.current = { cols, rows };
          onResizeRef.current?.(cols, rows);
        }
        updateScrollbar(terminal);
      });

      const scrollDisposable = terminal.onScroll?.(() => updateScrollbar(terminal));
      // onWriteParsed fires after each chunk of data is rendered. When the user has
      // scrolled up into history and new output arrives, onScroll doesn't fire (the
      // viewport position didn't change) but the buffer grew, so thumb proportions
      // go stale. Updating here keeps the thumb correctly sized while streaming.
      const writeParsedDisposable = terminal.onWriteParsed?.(() => updateScrollbar(terminal));

      // CRITICAL: Store refs BEFORE triggering callbacks
      // This ensures terminalRef is available when parent component calls getTerminal()
      terminalRef.current = terminal;
      fitAddonRef.current = fitAddon;
      searchAddonRef.current = searchAddon;
      serializeAddonRef.current = serializeAddon;

      // Deliberately no synchronous initial onResize() call here: it used to fire
      // onResize(terminal.cols, terminal.rows) with xterm's construction-time defaults
      // (80x24) before fitAddon.fit() ever ran, corrupting TerminalOutput's dimension
      // cache with the wrong size (see XtermTerminalBug.test.tsx "Bug 1" and R-series fix
      // "prevent premature resize from corrupting dimension cache"). The double-RAF
      // initial fit() below — and the decoupled sampler / ResizeObserver after it — are
      // solely responsible for telling the parent the real size via terminal.onResize.

      // ---- Mobile selection handle drag logic ----
      // Each handle listens for touchstart, then registers a document-level
      // touchmove handler that updates the xterm selection in real time.
      // The anchor (fixed endpoint) is captured at touchstart and held throughout
      // the drag so the opposite handle stays in place.
      const handleCleanupFns: (() => void)[] = [];
      if (isTouchPrimary) {
        const attachHandleDrag = (el: HTMLDivElement | null, handle: 'start' | 'end') => {
          if (!el) return;
          // { startCol, startRow, endCol, endRow } in xterm viewport coords
          let anchor: { sc: number; sr: number; ec: number; er: number } | null = null;

          const onTouchMove = (e: TouchEvent) => {
            if (!anchor || !terminal.element) return;
            const touch = e.touches[0];
            if (!touch) return;
            e.preventDefault();
            const rect = terminal.element.getBoundingClientRect();
            const { cellH, cellW } = getCellDimensions(terminal);
            const col = Math.max(0, Math.min(terminal.cols - 1, Math.floor((touch.clientX - rect.left) / cellW)));
            const row = Math.max(0, Math.min(terminal.rows - 1, Math.floor((touch.clientY - rect.top) / cellH)));

            if (handle === 'end') {
              // Keep start fixed, extend/shrink the end
              const len = Math.max(1, (row - anchor.sr) * terminal.cols + (col - anchor.sc));
              terminal.select(anchor.sc, anchor.sr, len);
            } else {
              // Keep end fixed, extend/shrink the start
              const len = Math.max(1, (anchor.er - row) * terminal.cols + (anchor.ec - col));
              terminal.select(col, row, len);
            }
          };

          const onTouchEnd = () => {
            anchor = null;
            document.removeEventListener('touchmove', onTouchMove);
            document.removeEventListener('touchend', onTouchEnd);
            document.removeEventListener('touchcancel', onTouchEnd);
          };

          const onTouchStart = (e: TouchEvent) => {
            e.preventDefault();
            e.stopPropagation();
            const pos = terminal.getSelectionPosition();
            if (!pos) return;
            anchor = { sc: pos.start.x, sr: pos.start.y, ec: pos.end.x, er: pos.end.y };
            document.addEventListener('touchmove', onTouchMove, { passive: false });
            document.addEventListener('touchend', onTouchEnd, { passive: true });
            document.addEventListener('touchcancel', onTouchEnd, { passive: true });
          };

          el.addEventListener('touchstart', onTouchStart, { passive: false });
          handleCleanupFns.push(() => {
            el.removeEventListener('touchstart', onTouchStart);
            document.removeEventListener('touchmove', onTouchMove);
            document.removeEventListener('touchend', onTouchEnd);
            document.removeEventListener('touchcancel', onTouchEnd);
          });
        };

        attachHandleDrag(startHandleRef.current, 'start');
        attachHandleDrag(endHandleRef.current, 'end');
      }

      // ---- Custom scrollbar track click (jump to position) ----
      // A tap on the track itself (not the thumb) scrolls to that proportional position.
      const trackEl = scrollTrackRef.current;
      if (trackEl) {
        const onTrackClick = (e: MouseEvent) => {
          if (e.target !== trackEl) return; // thumb click handled separately
          const trackHeight = trackEl.clientHeight;
          if (trackHeight <= 0) return;
          const relY = e.clientY - trackEl.getBoundingClientRect().top;
          const buf = terminal.buffer.active;
          const maxScrollLines = buf.length - terminal.rows;
          if (maxScrollLines <= 0) return;
          const targetViewportY = Math.max(0, Math.min(maxScrollLines,
            Math.round((relY / trackHeight) * maxScrollLines)));
          terminal.scrollLines(targetViewportY - buf.viewportY);
        };
        const onTrackTouchEnd = (e: TouchEvent) => {
          if (e.target !== trackEl) return;
          const touch = e.changedTouches[0];
          if (!touch) return;
          const trackHeight = trackEl.clientHeight;
          if (trackHeight <= 0) return;
          const relY = touch.clientY - trackEl.getBoundingClientRect().top;
          const buf = terminal.buffer.active;
          const maxScrollLines = buf.length - terminal.rows;
          if (maxScrollLines <= 0) return;
          const targetViewportY = Math.max(0, Math.min(maxScrollLines,
            Math.round((relY / trackHeight) * maxScrollLines)));
          terminal.scrollLines(targetViewportY - buf.viewportY);
        };
        trackEl.addEventListener('click', onTrackClick);
        trackEl.addEventListener('touchend', onTrackTouchEnd, { passive: true });
        handleCleanupFns.push(() => {
          trackEl.removeEventListener('click', onTrackClick);
          trackEl.removeEventListener('touchend', onTrackTouchEnd);
        });
      }

      // ---- Custom scrollbar thumb drag (mouse + touch, all devices) ----
      // Dragging the left-side thumb scrolls the terminal without touching the
      // right-side window scrollbar area.
      const thumbEl = scrollThumbRef.current;
      if (thumbEl) {
        let dragStartY = 0;
        let dragStartViewportY = 0;
        let isDragging = false;

        const performDrag = (clientY: number) => {
          if (!isDragging) return;
          const track = scrollTrackRef.current;
          if (!track) return;
          const trackHeight = track.clientHeight;
          const buf = terminal.buffer.active;
          const totalLines = buf.length;
          const visibleLines = terminal.rows;
          const maxScrollLines = totalLines - visibleLines;
          if (maxScrollLines <= 0 || trackHeight <= 0) return;
          const thumbHeight = Math.max(20, trackHeight * (visibleLines / totalLines));
          const scrollableTrack = Math.max(1, trackHeight - thumbHeight);
          const dy = clientY - dragStartY;
          const targetViewportY = Math.max(0, Math.min(maxScrollLines,
            dragStartViewportY + Math.round((dy / scrollableTrack) * maxScrollLines)));
          const delta = targetViewportY - buf.viewportY;
          if (delta !== 0) terminal.scrollLines(delta);
        };

        const endDrag = () => {
          if (!isDragging) return;
          isDragging = false;
          document.removeEventListener('mousemove', onMouseMove);
          document.removeEventListener('mouseup', endDrag);
          document.removeEventListener('touchmove', onScrollThumbTouchMove);
          document.removeEventListener('touchend', endDrag);
          document.removeEventListener('touchcancel', endDrag);
        };

        const onMouseMove = (e: MouseEvent) => performDrag(e.clientY);

        const onScrollThumbTouchMove = (e: TouchEvent) => {
          e.preventDefault();
          const t = e.touches[0];
          if (t) performDrag(t.clientY);
        };

        const onMouseDown = (e: MouseEvent) => {
          e.preventDefault();
          isDragging = true;
          dragStartY = e.clientY;
          dragStartViewportY = terminal.buffer.active.viewportY;
          document.addEventListener('mousemove', onMouseMove);
          document.addEventListener('mouseup', endDrag);
        };

        const onScrollThumbTouchStart = (e: TouchEvent) => {
          e.preventDefault();
          e.stopPropagation();
          const t = e.touches[0];
          if (!t) return;
          isDragging = true;
          dragStartY = t.clientY;
          dragStartViewportY = terminal.buffer.active.viewportY;
          document.addEventListener('touchmove', onScrollThumbTouchMove, { passive: false });
          document.addEventListener('touchend', endDrag, { passive: true });
          document.addEventListener('touchcancel', endDrag, { passive: true });
        };

        thumbEl.addEventListener('mousedown', onMouseDown);
        thumbEl.addEventListener('touchstart', onScrollThumbTouchStart, { passive: false });

        handleCleanupFns.push(() => {
          thumbEl.removeEventListener('mousedown', onMouseDown);
          thumbEl.removeEventListener('touchstart', onScrollThumbTouchStart);
          document.removeEventListener('mousemove', onMouseMove);
          document.removeEventListener('mouseup', endDrag);
          document.removeEventListener('touchmove', onScrollThumbTouchMove);
          document.removeEventListener('touchend', endDrag);
          document.removeEventListener('touchcancel', endDrag);
        });
      }

      // Setup ResizeObserver for automatic fitting
      // Track container size to avoid unnecessary fit() calls
      let lastContainerSize = { width: 0, height: 0 };
      let resizeTimeout: NodeJS.Timeout | null = null;

      // Decoupled resize sampler (ADR-002): a fixed-cadence re-sampling loop,
      // started by the debounce below but never reset by further
      // ResizeObserver deliveries once running. See ADR-002 for the full
      // algorithm and rationale.
      let samplerActive = false;
      let sampleTimeout: NodeJS.Timeout | null = null;
      let sampleCount = 0;
      let pendingProposedDims: ResizeDimensions | null = null;

      const stopSampler = () => {
        samplerActive = false;
        pendingProposedDims = null;
        sampleCount = 0;
        if (sampleTimeout) {
          clearTimeout(sampleTimeout);
          sampleTimeout = null;
        }
      };

      const sampleTick = () => {
        if (!fitAddonRef.current || !terminalRef.current) {
          stopSampler();
          return;
        }

        const proposed = fitAddonRef.current.proposeDimensions();
        const applied: ResizeDimensions = {
          cols: terminalRef.current.cols,
          rows: terminalRef.current.rows,
        };
        const result = shouldScheduleFit(proposed, applied, pendingProposedDims);

        if (result.schedule) {
          fitAddonRef.current.fit();
          console.log(`[XtermTerminal] Sampler confirmed resize, fit applied: ${terminalRef.current.cols} cols × ${terminalRef.current.rows} rows`);

          // Sync lastContainerSize to the post-fit DOM dimensions so the next
          // ResizeObserver entry (triggered by fit() resizing xterm.js internals)
          // is filtered out, breaking the scrollbar-appearance oscillation loop.
          if (containerRef.current) {
            const r = containerRef.current.getBoundingClientRect();
            lastContainerSize = { width: r.width, height: r.height };
          }

          // AC5: accumulate mismatch across confirmed resize events, not just
          // a single startup check (architecture research point 2).
          if (containerRef.current) {
            const mismatchInputs = extractCellMismatchInputs(terminalRef.current, containerRef.current);
            recordMismatchSample(mismatchInputs);
          }

          stopSampler();
          return;
        }

        if (result.nextPending === null) {
          // At rest (proposed equals applied) or proposeDimensions() returned undefined.
          stopSampler();
          return;
        }

        pendingProposedDims = result.nextPending;
        sampleCount++;

        if (sampleCount >= MAX_SAMPLES) {
          console.warn('[XtermTerminal] Resize did not converge after 20 samples; giving up');
          // Full reset (not a partial abandon): give-up must not leave the
          // sampler permanently inert, since startSamplerIfNeeded() is a
          // no-op whenever samplerActive is already true. See ADR-002.
          stopSampler();
          return;
        }

        sampleTimeout = setTimeout(sampleTick, SAMPLE_INTERVAL_MS);
      };

      const startSamplerIfNeeded = () => {
        if (samplerActive) return;
        samplerActive = true;
        sampleCount = 0;
        pendingProposedDims = null;
        sampleTick();
      };

      const resizeObserver = new ResizeObserver((entries: ResizeObserverEntry[]) => {
        if (!fitAddonRef.current || !terminalRef.current) return;

        const entry = entries[0];
        if (!entry) return;

        // Get current container size
        const { width, height } = entry.contentRect;

        // Only fit if size actually changed (avoid sub-pixel changes)
        const widthChanged = Math.abs(width - lastContainerSize.width) > 1;
        const heightChanged = Math.abs(height - lastContainerSize.height) > 1;

        if ((widthChanged || heightChanged) && width > 0 && height > 0) {
          lastContainerSize = { width, height };

          console.log(`[XtermTerminal] Container resized to ${width}px × ${height}px (before fit)`);
          console.log(`[XtermTerminal] Terminal dimensions BEFORE fit: ${terminalRef.current.cols} cols × ${terminalRef.current.rows} rows`);

          // Flat 150ms debounce (R1.2): ensures tmux has processed the previous SIGWINCH before
          // FitAddon measures container and fires terminal.onResize. The previous adaptive debounce
          // (10ms for first 3 resizes) fired before a single animation frame, causing the
          // ResizeObserver to trigger fit and server resize before tmux could stabilize.
          const debounceDelay = 150;

          // Clear any pending resize timeout
          if (resizeTimeout) {
            clearTimeout(resizeTimeout);
          }

          // Schedule the decoupled sampler start (ADR-002) after the debounce settles.
          // No extra rAF guard here (unlike the startup double-rAF fit): a stale first
          // proposeDimensions() read just produces a "pending" sample that won't match
          // the next tick SAMPLE_INTERVAL_MS later, self-correcting within one extra
          // 50ms tick rather than committing a wrong fit(). The debounce only decides
          // *when to start* the sampler for a burst of RO deliveries; once started, the
          // sampler's own tick chain is never reset by further RO deliveries (ADR-002).
          resizeTimeout = setTimeout(() => {
            startSamplerIfNeeded();
            resizeTimeout = null;
          }, debounceDelay);
        } else if ((widthChanged || heightChanged) && (width === 0 || height === 0)) {
          console.log(`[XtermTerminal] Skipping fit: container collapsed to zero-size (${width}px × ${height}px)`);
        }
      });

      resizeObserver.observe(containerRef.current!);

      // Cleanup
      return () => {
        cancelled = true;
        if (resizeTimeout) {
          clearTimeout(resizeTimeout);
        }
        stopSampler();
        if (postFallbackRafId !== null) {
          cancelAnimationFrame(postFallbackRafId);
          postFallbackRafId = null;
        }
        cleanupContextMenu();
        resizeObserver.disconnect();
        dataDisposable?.dispose();
        selectionDisposable?.dispose();
        resizeDisposable?.dispose();
        scrollDisposable?.dispose();
        writeParsedDisposable?.dispose();
        handleCleanupFns.forEach(fn => fn());
        terminal.dispose();
        terminalRef.current = null;
        fitAddonRef.current = null;
        searchAddonRef.current = null;
        serializeAddonRef.current = null;
        webglAddonRef.current = null;
        if (containerRef.current) {
          delete (containerRef.current as any).__staplerSquadForceCanvasFallback;
        }
      };
    } catch (error) {
      console.error('[XtermTerminal] Terminal initialization failed:', error);
      // Notify parent via resize callback with error indicator (0x0 dimensions)
      if (onResizeRef.current) {
        // Signal error by passing 0x0 dimensions
        // Parent can detect this and show error message
        console.error('[XtermTerminal] Notifying parent of initialization failure');
      }
      return; // Stop initialization
    }
    // Only recreate terminal if scrollback changes (requires full recreation)
    // Other options can be updated dynamically below
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [scrollback]);

  // Update theme dynamically (no terminal recreation needed)
  useEffect(() => {
    if (terminalRef.current) {
      terminalRef.current.options.theme = getTheme(theme);
      terminalRef.current.refresh(0, terminalRef.current.rows - 1);
    }
  }, [theme]);

  // Detect system color scheme changes and update terminal theme accordingly
  // This provides automatic theme switching when no explicit theme prop is given
  useEffect(() => {
    if (typeof window === "undefined" || themeProp !== undefined) return;
    if (typeof window.matchMedia !== "function") return;

    const mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
    const handleChange = (e: MediaQueryListEvent) => {
      const newTheme = e.matches ? "dark" : "light";
      if (terminalRef.current) {
        terminalRef.current.options.theme = getTheme(newTheme);
        terminalRef.current.refresh(0, terminalRef.current.rows - 1);
      }
    };

    mediaQuery.addEventListener("change", handleChange);
    return () => mediaQuery.removeEventListener("change", handleChange);
  }, [themeProp]);

  // Update font size dynamically (no terminal recreation needed)
  useEffect(() => {
    if (terminalRef.current && terminalRef.current.options.fontSize !== fontSize) {
      terminalRef.current.options.fontSize = fontSize;
      // Defer fit to avoid synchronous resize events
      setTimeout(() => fitAddonRef.current?.fit(), 0);
    }
  }, [fontSize]);

  // Update font family dynamically (no terminal recreation needed)
  useEffect(() => {
    if (terminalRef.current && terminalRef.current.options.fontFamily !== fontFamily) {
      terminalRef.current.options.fontFamily = fontFamily;
      // Defer fit to avoid synchronous resize events
      setTimeout(() => fitAddonRef.current?.fit(), 0);
    }
  }, [fontFamily]);

  // Update cursor options dynamically (no terminal recreation needed)
  useEffect(() => {
    if (terminalRef.current) {
      terminalRef.current.options.cursorStyle = cursorStyle;
      terminalRef.current.options.cursorBlink = cursorBlink;
    }
  }, [cursorStyle, cursorBlink]);

  // Expose terminal methods via ref
  // CRITICAL: Use getter for terminal property to return current ref value
  useImperativeHandle(ref, () => ({
    get terminal() {
      return terminalRef.current;
    },
    get serializeAddon() {
      return serializeAddonRef.current;
    },
    write: (data: string) => {
      terminalRef.current?.write(data);
    },
    writeln: (data: string) => {
      terminalRef.current?.writeln(data);
    },
    clear: () => {
      terminalRef.current?.clear();
    },
    focus: () => {
      terminalRef.current?.focus();
    },
    fit: () => {
      fitAddonRef.current?.fit();
    },
    resize: (cols: number, rows: number) => {
      terminalRef.current?.resize(cols, rows);
    },
    search: (term: string): boolean => {
      if (!searchAddonRef.current) return false;
      return searchAddonRef.current.findNext(term);
    },
    searchNext: (term: string): boolean => {
      if (!searchAddonRef.current) return false;
      return searchAddonRef.current.findNext(term);
    },
    searchPrevious: (term: string): boolean => {
      if (!searchAddonRef.current) return false;
      return searchAddonRef.current.findPrevious(term);
    },
  }), []);

  return (
    <div className={styles.container} data-context="terminal">
      <div ref={containerRef} className={styles.terminal} />
      {/* Custom left-side scrollbar — stays out of the right-side window-scrollbar zone.
          Only visible when scrollback content exists (controlled via updateScrollbar). */}
      <div ref={scrollTrackRef} className={styles.scrollTrack} style={{ display: 'none' }}>
        <div ref={scrollThumbRef} className={styles.scrollThumb} />
      </div>
      {/* Persistent "Copy all" button — copies full scrollback as plain text.
          Always visible so mobile users don't need to struggle with text selection.
          onPointerDown keeps clipboard write inside a synchronous gesture (iOS safe). */}
      <button
        aria-label="Copy terminal scrollback to clipboard"
        className={styles.scrollbackCopyButton}
        onPointerDown={handleCopyScrollbackPointerDown}
      >
        📋 Copy all
      </button>
      {/* Floating Copy button and toast — always in DOM (hidden by default) so no
          mount/unmount during selection drag. Position set via ref DOM mutation.
          onPointerDown used (not onClick) because iOS Safari only allows clipboard
          writes inside synchronous user gesture handlers. */}
      {typeof document !== 'undefined' && createPortal(
        <>
          <button
            ref={copyButtonRef}
            aria-label="Copy selected text"
            className={styles.floatingCopyButton}
            style={{ display: 'none' }}
            onPointerDown={handleCopyButtonPointerDown}
          >
            Copy
          </button>
          <div
            ref={toastRef}
            className={styles.copiedToast}
            aria-live="polite"
            style={{ display: 'none' }}
          />
          {/* Mobile selection handles — draggable circles at selection start/end.
              Touch events attached natively in useEffect (not React handlers)
              so they can call e.preventDefault() and stopPropagation reliably. */}
          <div
            ref={startHandleRef}
            className={styles.selectionHandle}
            aria-hidden="true"
            style={{ display: 'none' }}
          />
          <div
            ref={endHandleRef}
            className={styles.selectionHandle}
            aria-hidden="true"
            style={{ display: 'none' }}
          />
        </>,
        document.body
      )}
      {contextMenuState && (
        <TerminalContextMenu
          x={contextMenuState.x}
          y={contextMenuState.y}
          hasSelection={(terminalRef.current?.getSelection()?.length ?? 0) > 0}
          onCopy={handleMenuCopy}
          onSelectAll={handleMenuSelectAll}
          onPaste={handleMenuPaste}
          onDismiss={handleContextMenuDismiss}
        />
      )}
    </div>
  );
});

XtermTerminal.displayName = "XtermTerminal";

/**
 * Get xterm.js theme configuration using named theme exports
 */
function getTheme(theme: "light" | "dark") {
  return theme === "light" ? lightTerminalTheme : darkTerminalTheme;
}
