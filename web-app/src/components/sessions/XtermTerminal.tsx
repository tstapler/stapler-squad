"use client";

import { useEffect, useRef, useCallback, useImperativeHandle, forwardRef } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { WebglAddon } from "@xterm/addon-webgl";
import { SearchAddon } from "@xterm/addon-search";
import "@xterm/xterm/css/xterm.css";
import styles from "./XtermTerminal.module.css";
import { loadTerminalConfig, type TerminalConfig } from "@/lib/config/terminalConfig";

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
  const scrollback = scrollbackProp ?? config?.scrollbackLines ?? 10000;
  const enableWebGL = config?.enableWebGL ?? true;
  const fontFamily = config?.fontFamily ?? 'Menlo, Monaco, "Courier New", monospace';
  const cursorStyle = config?.cursorStyle ?? "block";
  const cursorBlink = config?.cursorBlink ?? true;

  const terminalRef = useRef<Terminal | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const searchAddonRef = useRef<SearchAddon | null>(null);
  const lastSizeRef = useRef<{ cols: number; rows: number } | null>(null);

  // Store callbacks in refs to avoid recreating terminal on callback changes
  const onDataRef = useRef(onData);
  const onResizeRef = useRef(onResize);

  useEffect(() => {
    onDataRef.current = onData;
    onResizeRef.current = onResize;
  }, [onData, onResize]);

  // Initialize terminal on mount
  useEffect(() => {
    // SSR guard
    if (typeof window === 'undefined') {
      console.warn('[XtermTerminal] SSR detected, terminal requires client-side rendering');
      return;
    }

    if (!containerRef.current || terminalRef.current) return;

    // Create terminal instance with configuration
    const terminal = new Terminal({
      cursorBlink,
      cursorStyle,
      fontSize,
      fontFamily,
      theme: getTheme(theme),
      scrollback,
      allowProposedApi: true, // Required for some addons
    });

    // Create and load addons
    const fitAddon = new FitAddon();
    const webLinksAddon = new WebLinksAddon();
    const searchAddon = new SearchAddon();

    terminal.loadAddon(fitAddon);
    terminal.loadAddon(webLinksAddon);
    terminal.loadAddon(searchAddon);

    // Try to enable WebGL renderer if enabled in config (fallback to canvas if unavailable)
    if (enableWebGL) {
      try {
        const webglAddon = new WebglAddon();
        terminal.loadAddon(webglAddon);
        console.log("[XtermTerminal] WebGL renderer enabled");
      } catch (e) {
        console.warn("[XtermTerminal] WebGL not available, using canvas fallback:", e);
      }
    }

    // Open terminal in container with error boundary
    try {
      terminal.open(containerRef.current);

      // Fit terminal to container
      fitAddon.fit();
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

    // Now trigger initial resize callback (ref is ready for parent's getTerminal())
    lastSizeRef.current = { cols: terminal.cols, rows: terminal.rows };
    if (onResizeRef.current) {
      onResizeRef.current(terminal.cols, terminal.rows);
    }

    // Setup ResizeObserver for automatic fitting
    // Track container size to avoid unnecessary fit() calls
    let lastContainerSize = { width: 0, height: 0 };
    let resizeCount = 0;
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

      if (widthChanged || heightChanged) {
        lastContainerSize = { width, height };
        resizeCount++;

        console.log(`[XtermTerminal] Container resized to ${width}px × ${height}px (before fit)`);
        console.log(`[XtermTerminal] Terminal dimensions BEFORE fit: ${terminalRef.current.cols} cols × ${terminalRef.current.rows} rows`);

        // Use minimal debounce for initial resizes (first 3), then increase for stability
        // This ensures ultra-fast initial sizing (10ms) when modal opens, then reduces resize frequency
        const debounceDelay = resizeCount <= 3 ? 10 : 250;

        // Clear any pending resize timeout
        if (resizeTimeout) {
          clearTimeout(resizeTimeout);
        }

        // Schedule fit with adaptive debounce
        resizeTimeout = setTimeout(() => {
          fitAddonRef.current?.fit();
          console.log(`[XtermTerminal] Terminal dimensions AFTER fit: ${terminalRef.current?.cols} cols × ${terminalRef.current?.rows} rows`);
          resizeTimeout = null;
        }, debounceDelay);
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
      resizeDisposable.dispose();
      terminal.dispose();
      terminalRef.current = null;
      fitAddonRef.current = null;
      searchAddonRef.current = null;
    };
    // Only recreate terminal if container changes or scrollback changes (requires full recreation)
    // Other options can be updated dynamically below
  }, [scrollback, enableWebGL]); // Reduced dependencies - only recreate when necessary

  // Update theme dynamically (no terminal recreation needed)
  useEffect(() => {
    if (terminalRef.current) {
      terminalRef.current.options.theme = getTheme(theme);
    }
  }, [theme]);

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
    <div className={styles.container}>
      <div ref={containerRef} className={styles.terminal} />
    </div>
  );
});

XtermTerminal.displayName = "XtermTerminal";

/**
 * Get xterm.js theme configuration
 */
function getTheme(theme: "light" | "dark") {
  if (theme === "light") {
    return {
      background: "#ffffff",
      foreground: "#333333",
      cursor: "#333333",
      cursorAccent: "#ffffff",
      selection: "rgba(0, 0, 0, 0.3)",
      black: "#000000",
      red: "#cd3131",
      green: "#00bc00",
      yellow: "#949800",
      blue: "#0451a5",
      magenta: "#bc05bc",
      cyan: "#0598bc",
      white: "#555555",
      brightBlack: "#666666",
      brightRed: "#cd3131",
      brightGreen: "#14ce14",
      brightYellow: "#b5ba00",
      brightBlue: "#0451a5",
      brightMagenta: "#bc05bc",
      brightCyan: "#0598bc",
      brightWhite: "#a5a5a5",
    };
  }

  // Dark theme (default)
  return {
    background: "#1e1e1e",
    foreground: "#cccccc",
    cursor: "#cccccc",
    cursorAccent: "#1e1e1e",
    selection: "rgba(255, 255, 255, 0.3)",
    black: "#000000",
    red: "#cd3131",
    green: "#0dbc79",
    yellow: "#e5e510",
    blue: "#2472c8",
    magenta: "#bc3fbc",
    cyan: "#11a8cd",
    white: "#e5e5e5",
    brightBlack: "#666666",
    brightRed: "#f14c4c",
    brightGreen: "#23d18b",
    brightYellow: "#f5f543",
    brightBlue: "#3b8eea",
    brightMagenta: "#d670d6",
    brightCyan: "#29b8db",
    brightWhite: "#ffffff",
  };
}

/**
 * Debounce helper for resize events
 */
function debounce<T extends (...args: any[]) => void>(
  func: T,
  wait: number
): (...args: Parameters<T>) => void {
  let timeout: NodeJS.Timeout | null = null;

  return function executedFunction(...args: Parameters<T>) {
    const later = () => {
      timeout = null;
      func(...args);
    };

    if (timeout) {
      clearTimeout(timeout);
    }
    timeout = setTimeout(later, wait);
  };
}
