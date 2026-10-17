"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import type { CodeEditorHandle, ScrollSnapshot } from "@/components/sessions/CodeEditor";
import { detectLanguage } from "@/components/sessions/FileContentViewer";
import { useFilesAnalytics } from "@/lib/files/analytics/filesAnalytics";
import { clampLine } from "@/lib/files/deepLink";
import { buildOutline, type OutlineResult } from "@/lib/files/outline";
import { invalidateFileContent } from "./useFileService";
import type { OpenSource, useOpenBuffers } from "./useOpenBuffers";

type OpenBuffers = ReturnType<typeof useOpenBuffers>;
type NavVia = "button" | "keyboard" | "long_press_menu";
export type Overlay = "goto" | "outline" | null;

const depthBucket = (n: number) => (n <= 1 ? "1" : n <= 5 ? "2-5" : "6+") as "1" | "2-5" | "6+";

/**
 * Glue between the buffer reducer and the live editor: applies the reducer's pending cursor reveal
 * once the right editor is on screen, records the cursor before leaving a buffer, and owns go-to-line,
 * jumplist moves, outline text and reload-in-place.
 */
export function useEditorBridge(buffers: OpenBuffers) {
  const analytics = useFilesAnalytics({ component: "FileContentViewer" });
  const [editor, setEditor] = useState<{ handle: CodeEditorHandle; path: string } | null>(null);
  const [reloadKey, setReloadKey] = useState(0);
  const [overlay, setOverlay] = useState<Overlay>(null);
  const [beyond, setBeyond] = useState<{ path: string; line: number } | null>(null);
  const keepScroll = useRef<{ path: string; snapshot: ScrollSnapshot } | null>(null);
  const { state, active, revealed, recordViewState } = buffers;

  const onEditorReady = useCallback((handle: CodeEditorHandle, path: string) => setEditor({ handle, path }), []);

  const { pendingReveal } = state;
  useEffect(() => {
    if (!pendingReveal || !editor || editor.path !== pendingReveal.path) return;
    try {
      // The engine clamps; a hit past the loaded text (truncated read) says so instead of landing silently.
      const lines = editor.handle.getText().split("\n").length;
      setBeyond(pendingReveal.line > lines ? { path: pendingReveal.path, line: pendingReveal.line } : null);
      editor.handle.gotoLine(pendingReveal.line);
      const keep = keepScroll.current;
      if (keep?.path === pendingReveal.path) {
        editor.handle.restoreScroll(keep.snapshot);
        keepScroll.current = null;
      }
    } catch {
      return; // the engine was torn down; the next onReady retries
    }
    revealed(pendingReveal.seq);
  }, [pendingReveal, editor, revealed]);

  /** Saves the cursor of the buffer on screen so leaving it and coming back lands on the same line. */
  const capture = useCallback(() => {
    if (!editor || !active || editor.path !== active.path) return;
    try {
      const { line, col } = editor.handle.getCursor();
      recordViewState(active.path, { ...active.viewState, line, col });
    } catch {
      // engine gone: keep the last recorded position
    }
  }, [editor, active, recordViewState]);

  const open = useCallback((path: string, opts: { pinned?: boolean; source?: OpenSource } = {}) => {
    capture();
    buffers.open(path, opts);
  }, [capture, buffers]);
  const activate = useCallback((path: string) => { capture(); buffers.activate(path); }, [capture, buffers]);
  const jumpTo = useCallback((path: string, line: number, opts: Parameters<OpenBuffers["jump"]>[2] = {}) => {
    capture();
    buffers.jump(path, line, opts);
  }, [capture, buffers]);

  const move = useCallback((delta: -1 | 1, via: NavVia) => {
    const behind = state.navIndex;
    const depth = delta < 0 ? behind : state.nav.length - 1 - behind;
    if (depth <= 0) return;
    capture();
    analytics.emit(delta < 0 ? "files.nav.back" : "files.nav.forward", { via, depth: depthBucket(depth) });
    if (delta < 0) buffers.navBack();
    else buffers.navForward();
  }, [state.navIndex, state.nav.length, capture, analytics, buffers]);
  const back = useCallback((via: NavVia) => move(-1, via), [move]);
  const forward = useCallback((via: NavVia) => move(1, via), [move]);

  const goToLine = useCallback((requested: number, via: "palette" | "status_bar" | "vim") => {
    if (!editor || !active || editor.path !== active.path || !Number.isFinite(requested)) return;
    const { line, clamped } = clampLine(requested, editor.handle.getText().split("\n").length);
    analytics.emit("files.nav.jump_line", { via, clamped: String(clamped) as "true" | "false" });
    jumpTo(active.path, line);
  }, [editor, active, analytics, jumpTo]);

  /** Fetches the file again in place: same cursor line and scroll once the new text is on screen. */
  const reload = useCallback((newToken: string) => {
    if (!active) return;
    const here = editor?.path === active.path ? editor.handle : null;
    let cursor = { line: active.viewState.line, col: active.viewState.col };
    try {
      if (here) {
        cursor = here.getCursor();
        keepScroll.current = { path: active.path, snapshot: here.scrollSnapshot() };
      }
    } catch {
      keepScroll.current = null;
    }
    invalidateFileContent(active.sessionId, active.path);
    setEditor(null);
    setReloadKey((k) => k + 1);
    buffers.adoptToken(active.path, newToken);
    buffers.jump(active.path, cursor.line, { col: cursor.col, chain: "step" });
  }, [active, editor, buffers]);

  const outline = useCallback((): OutlineResult => {
    if (!editor || !active || editor.path !== active.path) return { kind: "unavailable", reason: "error" };
    try {
      const text = editor.handle.getText();
      return buildOutline(text, detectLanguage(active.path), { size: text.length });
    } catch {
      return { kind: "unavailable", reason: "error" };
    }
  }, [editor, active]);

  /** Cursor and line endings of the buffer on screen, or null while its editor is not ready. */
  const inspect = useCallback((): { line: number; col: number; eol: "LF" | "CRLF" | "mixed" | null } | null => {
    if (!editor || !active || editor.path !== active.path) return null;
    try {
      const { line, col } = editor.handle.getCursor();
      return { line, col, eol: null };
    } catch {
      return null;
    }
  }, [editor, active]);
  const eol = useCallback((): "LF" | "CRLF" | "mixed" | null => {
    if (!editor || !active || editor.path !== active.path) return null;
    try {
      const text = editor.handle.getText();
      const crlf = (text.match(/\r\n/g) ?? []).length;
      if (crlf === 0) return "LF";
      return crlf === (text.match(/\n/g) ?? []).length ? "CRLF" : "mixed";
    } catch {
      return null;
    }
  }, [editor, active]);

  const text = useCallback((): string | null => {
    if (!editor || !active || editor.path !== active.path) return null;
    try { return editor.handle.getText(); } catch { return null; }
  }, [editor, active]);

  const focusEditor = useCallback(() => {
    if (!editor || !active || editor.path !== active.path) return;
    try { editor.handle.focus(); } catch { /* engine torn down */ }
  }, [editor, active]);

  const dismissBeyond = useCallback(() => setBeyond(null), []);

  return { onEditorReady, focusEditor, beyond: beyond && active && beyond.path === active.path ? beyond : null, dismissBeyond, inspect, eol, text, reloadKey, overlay, setOverlay, open, activate, jumpTo, back, forward, goToLine, reload, outline };
}
