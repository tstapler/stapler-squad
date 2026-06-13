"use client";

import { useEffect, useRef, useCallback, useImperativeHandle, forwardRef, useState } from "react";
import { createPortal } from "react-dom";
import { useTerminalGestures } from "@/lib/hooks/useTerminalGestures";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { SearchAddon } from "@xterm/addon-search";
import { SerializeAddon } from "@xterm/addon-serialize";
import "@xterm/xterm/css/xterm.css";
import * as styles from "./XtermTerminal.css";
import { TerminalContextMenu } from "./TerminalContextMenu";
import { loadTerminalConfig, darkTerminalTheme, lightTerminalTheme, type TerminalConfig } from "@/lib/config/terminalConfig";
import { getCellDimensions } from "@/lib/terminal/cellDimensions";
import { isMouseTracking } from "@/lib/terminal/mouseTracking";

const DEFAULT_SCROLLBACK_SIZE = 5000;

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

    // Create and load addons
    const fitAddon = new FitAddon();
    const webLinksAddon = new WebLinksAddon();
    const searchAddon = new SearchAddon();
    const serializeAddon = new SerializeAddon();

    terminal.loadAddon(fitAddon);
    terminal.loadAddon(webLinksAddon);
    terminal.loadAddon(searchAddon);
    terminal.loadAddon(serializeAddon);

    // xterm.js issue #2033 — guard WebGL before loading on Android/mobile
    (async () => {
      if (typeof WebGL2RenderingContext !== 'undefined') {
        try {
          const { WebglAddon } = await import('@xterm/addon-webgl');
          const webglAddon = new WebglAddon();
          webglAddon.onContextLoss(() => {
            console.warn('[XtermTerminal] WebGL context lost, falling back to canvas renderer');
            webglAddon.dispose();
          });
          terminal.loadAddon(webglAddon);
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

      // Attach contextmenu listener after terminal.open() — terminal.element is guaranteed non-null here.
      // Capture the element reference for cleanup to ensure add/remove operate on the same node.
      const termElement = terminal.element!;

      const handleContextMenu = (e: MouseEvent) => {
        e.preventDefault(); // always suppress browser native menu
        if (isMouseTracking(terminal)) return; // let PTY handle right-click via VT sequences
        setContextMenuState({ x: e.clientX, y: e.clientY });
      };
      termElement.addEventListener('contextmenu', handleContextMenu);

      // Custom key handler — a single handler for all shortcuts (attaching multiple replaces).
      // All key intercepts must be here. No cleanup needed; tied to terminal instance lifecycle.
      // iOS note: navigator.clipboard.writeText may fail in keydown because keydown events may
      // not carry the same user-gesture trust as pointer events. iOS users don't have Ctrl+C
      // hardware — they use the floating Copy button (onPointerDown path) instead.
      terminal.attachCustomKeyEventHandler((event: KeyboardEvent): boolean => {
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

          // Calculate actual pixels per column for verification
          if (containerEl && terminal.cols > 0) {
            const actualPixelsPerCol = containerEl.getBoundingClientRect().width / terminal.cols;
            console.log(`[XtermTerminal] Actual pixels per column: ${actualPixelsPerCol.toFixed(2)}px`);
            if (dims?.css?.cell) {
              console.log(`[XtermTerminal] Expected pixels per column: ${dims.css.cell.width.toFixed(2)}px`);
              if (Math.abs(actualPixelsPerCol - dims.css.cell.width) > 1) {
                console.error(`[XtermTerminal] ⚠️ SIZING MISMATCH! Container width doesn't match cell width calculation`);
              }
            }
          }
          // Secondary delayed fit() removed (R1.3): the double-rAF above provides sufficient layout
          // stability; the extra setTimeout caused a second terminal.onResize, triggering a duplicate
          // server resize RPC and second capture-pane cycle (double-resize corruption on mount).
        });
      });

      // Cleanup includes contextmenu listener removal
      const cleanupContextMenu = () => {
        termElement.removeEventListener('contextmenu', handleContextMenu);
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

      const scrollDisposable = terminal.onScroll(() => updateScrollbar(terminal));
      // onWriteParsed fires after each chunk of data is rendered. When the user has
      // scrolled up into history and new output arrives, onScroll doesn't fire (the
      // viewport position didn't change) but the buffer grew, so thumb proportions
      // go stale. Updating here keeps the thumb correctly sized while streaming.
      const writeParsedDisposable = terminal.onWriteParsed(() => updateScrollbar(terminal));

      // CRITICAL: Store refs BEFORE triggering callbacks
      // This ensures terminalRef is available when parent component calls getTerminal()
      terminalRef.current = terminal;
      fitAddonRef.current = fitAddon;
      searchAddonRef.current = searchAddon;
      serializeAddonRef.current = serializeAddon;

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

          // Schedule fit with adaptive debounce.
          // Double rAF ensures DOM reflow is complete before measuring on iOS Safari —
          // a single rAF is insufficient because the browser may batch it with the resize
          // event, leaving stale dimensions. See xterm.js issue #3895.
          resizeTimeout = setTimeout(() => {
            requestAnimationFrame(() => {
              requestAnimationFrame(() => {
                fitAddonRef.current?.fit();
                // Sync lastContainerSize to the post-fit DOM dimensions so the next
                // ResizeObserver entry (triggered by fit() resizing xterm.js internals)
                // is filtered out, breaking the scrollbar-appearance oscillation loop.
                if (containerRef.current) {
                  const r = containerRef.current.getBoundingClientRect();
                  lastContainerSize = { width: r.width, height: r.height };
                }
                console.log(`[XtermTerminal] Terminal dimensions AFTER fit: ${terminalRef.current?.cols} cols × ${terminalRef.current?.rows} rows`);
              });
            });
            resizeTimeout = null;
          }, debounceDelay);
        } else if ((widthChanged || heightChanged) && (width === 0 || height === 0)) {
          console.log(`[XtermTerminal] Skipping fit: container collapsed to zero-size (${width}px × ${height}px)`);
        }
      });

      resizeObserver.observe(containerRef.current!);

      // Cleanup
      return () => {
        if (resizeTimeout) {
          clearTimeout(resizeTimeout);
        }
        cleanupContextMenu();
        resizeObserver.disconnect();
        dataDisposable.dispose();
        selectionDisposable.dispose();
        resizeDisposable.dispose();
        scrollDisposable.dispose();
        writeParsedDisposable.dispose();
        handleCleanupFns.forEach(fn => fn());
        terminal.dispose();
        terminalRef.current = null;
        fitAddonRef.current = null;
        searchAddonRef.current = null;
        serializeAddonRef.current = null;
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
