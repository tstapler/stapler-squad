"use client";

import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from "react";
import type * as GhosttyModule from "ghostty-web";
import type { FitAddon, Terminal as GhosttyWebTerminal } from "ghostty-web";
import type { Terminal as XtermWebTerminal } from "@xterm/xterm";
import type { XtermTerminalHandle, XtermTerminalProps } from "./XtermTerminal";
import * as styles from "./XtermTerminal.css";
import { loadTerminalConfig, darkTerminalTheme, lightTerminalTheme } from "@/lib/config/terminalConfig";
import { asXtermCompatible, FRESH_TERMINAL_SCRUB, ghosttyScrollbackBytes, serializeGhosttyBuffer, toITerminal } from "@/lib/terminal/ghosttyAdapter";
import { attachRenderGate } from "@/lib/terminal/ghosttyRenderGate";
import { createMouseWheelForwarder, type WheelTerminal } from "@/lib/terminal/ghosttyWheelForward";
import { findInBuffer, type SearchState } from "@/lib/terminal/ghosttySearch";
import { attachTerminalImeSupport } from "@/lib/terminal/terminalImeSupport";
import type { RefitOptions } from "@/lib/terminal/postFitRepaint";
import type { MobileInputHandle } from "@/lib/terminal/mobileInput/useMobileInput";
import { useRendererMobileInput } from "@/lib/terminal/mobileInput/useRendererMobileInput";

/** Adapts a ghostty-web terminal to what the wheel forwarder reads; `renderer` is private in its typings. */
function ghosttyWheelTerminal(term: XtermWebTerminal | GhosttyWebTerminal): WheelTerminal {
  const t = term as unknown as GhosttyWebTerminal & { renderer?: { getMetrics?: () => { width: number; height: number } } };
  return {
    get cols() {
      return t.cols;
    },
    get rows() {
      return t.rows;
    },
    hasMouseTracking: () => t.hasMouseTracking(),
    cellSize: () => t.renderer?.getMetrics?.(),
    gridRect: () => t.element?.querySelector("canvas")?.getBoundingClientRect(),
  };
}

const DEFAULT_SCROLLBACK_SIZE = 5000;
const DEFAULT_FONT_SIZE = 14;

/**
 * ghostty-web (Ghostty's VT parser as WASM, drawn on a canvas) behind the same ref contract as
 * `XtermTerminal`, so the pool can swap renderers on the `ghostty_renderer` flag alone.
 *
 * The library is imported dynamically: its ~0.7 MB bundle (WASM inlined) stays out of the
 * initial chunk, and the module-level `init()` promise makes every instance share one WASM load.
 * Not ported from XtermTerminal: touch gestures, the WebGL slot budget, and the resize sampler
 * (ghostty-web's FitAddon observes the container itself).
 */
let ghosttyReady: Promise<typeof GhosttyModule> | null = null;
export function loadGhostty(): Promise<typeof GhosttyModule> {
  ghosttyReady ??= import("ghostty-web")
    .then(async (mod) => {
      await mod.init();
      return mod;
    })
    .catch((err) => {
      ghosttyReady = null; // a transient chunk/WASM fetch failure must not poison every later mount
      throw err;
    });
  return ghosttyReady;
}

export const GhosttyTerminal = forwardRef<XtermTerminalHandle, XtermTerminalProps>(({
  onData,
  onResize,
  theme: themeProp,
  fontSize: fontSizeProp,
  scrollback: scrollbackProp,
  useConfig = false,
  windowActive = true,
  mobileInput,
}, ref) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<GhosttyWebTerminal | null>(null);
  const compatRef = useRef<GhosttyWebTerminal | null>(null);
  const fitRef = useRef<FitAddon | null>(null);
  // Writes that arrive while the WASM is still loading; flushed in order once the terminal opens.
  const pendingWritesRef = useRef<string[]>([]);
  const searchStateRef = useRef<SearchState>({ term: "", row: -1, col: -1 });
  const layerRef = useRef<MobileInputHandle | null>(null);
  const [terminalReady, setTerminalReady] = useState(false);
  const onDataRef = useRef(onData);
  const onResizeRef = useRef(onResize);
  const windowActiveRef = useRef(windowActive);
  onDataRef.current = onData;
  onResizeRef.current = onResize;
  windowActiveRef.current = windowActive;

  const config = useConfig ? loadTerminalConfig() : null;
  const themeName = themeProp ?? config?.theme ?? "dark";
  const fontSize = fontSizeProp ?? config?.fontSize ?? DEFAULT_FONT_SIZE;
  const scrollback = scrollbackProp ?? config?.scrollbackLines ?? DEFAULT_SCROLLBACK_SIZE;
  const fontFamily = config?.fontFamily ?? 'Menlo, Monaco, "Courier New", monospace';
  const cursorStyle = config?.cursorStyle ?? "block";
  const cursorBlink = config?.cursorBlink ?? true;

  // Every PTY write goes through the facade so the query responder sees it (handle.write and handle.terminal.write alike).
  const compatTerminal = (): GhosttyWebTerminal | null => {
    const t = terminalRef.current;
    if (!t) return null;
    compatRef.current ??= asXtermCompatible(t, (reply) => onDataRef.current?.(reply));
    return compatRef.current;
  };

  // Created once per mount; later prop changes would need a remount, as the pool already keys on session.
  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;
    let disposed = false;
    let term: GhosttyWebTerminal | null = null;
    let disposeIme: (() => void) | undefined;
    let detachRenderGate: (() => void) | null = null;

    void loadGhostty().then(({ Terminal, FitAddon }) => {
      if (disposed) return;
      term = new Terminal({
        fontSize,
        fontFamily,
        scrollback: ghosttyScrollbackBytes(scrollback),
        cursorStyle,
        cursorBlink,
        theme: themeName === "light" ? lightTerminalTheme : darkTerminalTheme,
      });
      const fit = new FitAddon();
      term.loadAddon(fit);
      term.open(container);
      const ime = term.element
        ? attachTerminalImeSupport(term.element, "ghostty", (data) => onDataRef.current?.(data), (text) => term?.paste(text))
        : undefined;
      disposeIme = () => ime?.dispose();
      term.onData((data) => {
        ime?.traceData(data);
        onDataRef.current?.(data);
      });
      term.attachCustomWheelEventHandler(
        createMouseWheelForwarder(ghosttyWheelTerminal(term), (data) => onDataRef.current?.(data)),
      );
      term.onResize(({ cols, rows }) => onResizeRef.current?.(cols, rows));
      terminalRef.current = term;
      fitRef.current = fit;
      setTerminalReady(true);
      const surface = compatTerminal();
      surface?.write(FRESH_TERMINAL_SCRUB);
      for (const chunk of pendingWritesRef.current.splice(0)) surface?.write(chunk);
      fit.fit();
      fit.observeResize();
      detachRenderGate = attachRenderGate(term, container);
      onResizeRef.current?.(term.cols, term.rows);
    }).catch((err) => {
      pendingWritesRef.current.length = 0;
      console.error("[GhosttyTerminal] failed to load ghostty-web; disable the ghostty_renderer flag to fall back to xterm.js", err);
    });

    return () => {
      disposed = true;
      disposeIme?.();
      detachRenderGate?.();
      terminalRef.current = null;
      compatRef.current = null;
      fitRef.current?.dispose?.();
      fitRef.current = null;
      term?.dispose();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps -- see comment above
  }, []);

  // A hidden (keep-alive) pane measures 0x0; refit once when it becomes visible again.
  useEffect(() => {
    if (windowActive) fitRef.current?.fit();
  }, [windowActive]);

  useImperativeHandle(ref, () => {
    const searchFrom = (term: string, backwards: boolean): boolean => {
      const t = terminalRef.current;
      if (!t || !term) return false;
      const hit = findInBuffer(t, term, searchStateRef.current, backwards);
      if (!hit) return false;
      searchStateRef.current = { term, row: hit.row, col: hit.col };
      t.scrollToLine(Math.max(0, hit.row - Math.floor(t.rows / 2)));
      t.select(hit.col, hit.row, term.length);
      return true;
    };
    return {
      get terminal() {
        // ghostty-web mirrors the xterm.js Terminal surface TerminalOutput touches.
        return compatTerminal() as unknown as XtermWebTerminal | null;
      },
      get serializeAddon() {
        const t = terminalRef.current;
        return t ? ({ serialize: () => serializeGhosttyBuffer(t) } as unknown as XtermTerminalHandle["serializeAddon"]) : null;
      },
      write: (data: string) => {
        const surface = compatTerminal();
        if (surface) surface.write(data);
        else pendingWritesRef.current.push(data);
      },
      writeln: (data: string) => {
        const surface = compatTerminal();
        if (surface) surface.write(`${data}\r\n`);
        else pendingWritesRef.current.push(`${data}\r\n`);
      },
      clear: () => {
        const t = terminalRef.current;
        if (t) toITerminal(t).clear();
        else pendingWritesRef.current.length = 0;
      },
      focus: () => {
        if (!layerRef.current?.focus()) terminalRef.current?.focus();
      },
      fit: () => fitRef.current?.fit(),
      resize: (cols: number, rows: number) => terminalRef.current?.resize(cols, rows),
      refit: (opts?: RefitOptions) => {
        fitRef.current?.fit();
        const t = terminalRef.current;
        if (t) opts?.onFitted?.({ cols: t.cols, rows: t.rows });
      },
      search: (term: string) => {
        searchStateRef.current = { term: "", row: -1, col: -1 };
        return searchFrom(term, false);
      },
      searchNext: (term: string) => searchFrom(term, false),
      searchPrevious: (term: string) => searchFrom(term, true),
    };
  }, []);

  // The layer textarea lives beside, never inside, the element handed to open(): that element carries
  // ghostty's blanket beforeinput preventDefault and keydown handlers. Layer bytes skip the terminal's own onData.
  const layer = useRendererMobileInput({
    renderer: "ghostty",
    mobileInput,
    onDataRef,
    containerRef,
    getHelperTextarea: () => terminalRef.current?.textarea,
    bracketedPasteEnabled: () => terminalRef.current?.hasBracketedPaste() ?? false,
    rendererReady: terminalReady,
  });
  layerRef.current = layer.handle;

  return (
    <div className={styles.container} data-context="terminal" data-renderer="ghostty">
      <div ref={containerRef} className={styles.terminal} />
      {layer.layerActive && <div ref={layer.layerContainerRef} data-mobile-input-host />}
    </div>
  );
});

GhosttyTerminal.displayName = "GhosttyTerminal";
