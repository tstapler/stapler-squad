"use client";

import { useEffect, useRef, useCallback, useImperativeHandle, forwardRef, useState } from "react";
import { useTerminalGestures } from "@/lib/hooks/useTerminalGestures";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { SearchAddon } from "@xterm/addon-search";
import { SerializeAddon } from "@xterm/addon-serialize";
import "@xterm/xterm/css/xterm.css";
import * as styles from "./XtermTerminal.css";
import { loadTerminalConfig, darkTerminalTheme, lightTerminalTheme, type TerminalConfig } from "@/lib/config/terminalConfig";
import { getCellDimensions } from "@/lib/terminal/cellDimensions";

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

  // Floating Copy button state — shown when xterm has a non-empty selection (R3.1)
  const [copyButtonPos, setCopyButtonPos] = useState<{ x: number; y: number } | null>(null);
  const [showCopiedToast, setShowCopiedToast] = useState<'copied' | 'failed' | null>(null);

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

    // Setup event handlers using refs to avoid recreating terminal
    const dataDisposable = terminal.onData((data) => {
      onDataRef.current?.(data);
    });

    // Task 3.2.1 — Show floating Copy button on selection change (R3.1).
    // onSelectionChange is NOT a user gesture in iOS Safari, so we only set state here.
    // The clipboard write happens in the button's onPointerDown (synchronous user gesture).
    const selectionDisposable = terminal.onSelectionChange(() => {
      const text = terminal.getSelection();
      if (text && text.length > 0) {
        const pos = terminal.getSelectionPosition();
        if (pos && terminal.element) {
          const rect = terminal.element.getBoundingClientRect();
          const { cellH, cellW } = getCellDimensions(terminal);
          setCopyButtonPos({
            x: rect.left + pos.end.x * cellW,
            y: rect.top + pos.end.y * cellH - 40, // 40px above selection end
          });
        }
      } else {
        setCopyButtonPos(null);
      }
    });

    const resizeDisposable = terminal.onResize(({ cols, rows }) => {
      // Only trigger callback if size actually changed
      const lastSize = lastSizeRef.current;
      if (!lastSize || lastSize.cols !== cols || lastSize.rows !== rows) {
        lastSizeRef.current = { cols, rows };
        onResizeRef.current?.(cols, rows);
      }
    });

    // CRITICAL: Store refs BEFORE triggering callbacks
    // This ensures terminalRef is available when parent component calls getTerminal()
    terminalRef.current = terminal;
    fitAddonRef.current = fitAddon;
    searchAddonRef.current = searchAddon;
    serializeAddonRef.current = serializeAddon;

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
              console.log(`[XtermTerminal] Terminal dimensions AFTER fit: ${terminalRef.current?.cols} cols × ${terminalRef.current?.rows} rows`);
            });
          });
          resizeTimeout = null;
        }, debounceDelay);
      } else if ((widthChanged || heightChanged) && (width === 0 || height === 0)) {
        console.log(`[XtermTerminal] Skipping fit: container collapsed to zero-size (${width}px × ${height}px)`);
      }
    });

    resizeObserver.observe(containerRef.current);

    // Cleanup
    return () => {
      if (resizeTimeout) {
        clearTimeout(resizeTimeout);
      }
      resizeObserver.disconnect();
      dataDisposable.dispose();
      selectionDisposable.dispose();
      resizeDisposable.dispose();
      terminal.dispose();
      terminalRef.current = null;
      fitAddonRef.current = null;
      searchAddonRef.current = null;
      serializeAddonRef.current = null;
    };
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
      {/* Task 3.2.2 — Floating Copy button (R3.2).
          Rendered in a fixed-position button so it appears above the terminal.
          onPointerDown is used (not onClick) because iOS Safari only allows clipboard
          writes inside synchronous user gesture handlers (ADR-013). */}
      {copyButtonPos && (
        <button
          aria-label="Copy selected text"
          className={styles.floatingCopyButton}
          style={{ left: copyButtonPos.x, top: copyButtonPos.y }}
          onPointerDown={(e) => {
            const terminal = terminalRef.current;
            if (!terminal) return;
            const text = terminal.getSelection(); // synchronous within user gesture — iOS safe
            const tryExecCommandCopy = () => {
              const el = document.createElement('textarea');
              el.value = text;
              document.body.appendChild(el);
              el.select();
              const ok = document.execCommand('copy');
              document.body.removeChild(el);
              return ok;
            };
            if (navigator.clipboard?.writeText) {
              navigator.clipboard.writeText(text).then(() => {
                setCopyButtonPos(null);
                setShowCopiedToast('copied');
                setTimeout(() => setShowCopiedToast(null), 1500);
              }).catch(() => {
                // Clipboard API denied — fall back to execCommand
                const ok = tryExecCommandCopy();
                setCopyButtonPos(null);
                setShowCopiedToast(ok ? 'copied' : 'failed');
                setTimeout(() => setShowCopiedToast(null), 1500);
              });
              e.preventDefault();
              return;
            }
            const ok = tryExecCommandCopy();
            setCopyButtonPos(null);
            setShowCopiedToast(ok ? 'copied' : 'failed');
            setTimeout(() => setShowCopiedToast(null), 1500);
            e.preventDefault();
          }}
        >
          Copy
        </button>
      )}
      {showCopiedToast && (
        <div className={styles.copiedToast} aria-live="polite">
          {showCopiedToast === 'copied' ? 'Copied' : 'Copy failed'}
        </div>
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

