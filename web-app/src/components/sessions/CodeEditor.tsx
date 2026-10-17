"use client";

import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef } from "react";
import type { GutterMarkType } from "@/lib/utils/parseDiff";
import type {
  BufferInit, BufferState, EditorMode, EngineFeatures, EngineHandle, ModeConfig, ScrollSnapshot,
} from "./files/engine/EditorEngine";
import { codeEditor } from "./CodeEditor.css";

export type { EditorMode, BufferInit, BufferState, ScrollSnapshot };

export interface CodeEditorHandle {
  readonly features: EngineFeatures;
  createBufferState(init: Omit<BufferInit, "config">): Promise<BufferState>;
  getState(): BufferState;
  setBufferState(state: BufferState): void;
  getText(): string;
  getCursor(): { line: number; col: number };
  /** Inserts at the cursor like typing; false (and no change) while read-only. */
  insertText(text: string): boolean;
  gotoLine(line: number): void;
  scrollSnapshot(): ScrollSnapshot;
  restoreScroll(snapshot: ScrollSnapshot): void;
  focus(): void;
}

export interface CodeEditorProps {
  content: string;
  language: string;
  path?: string;
  mode?: EditorMode;
  wrapLines?: boolean;
  gutterMarks?: Map<number, GutterMarkType>;
  onReady?: (handle: CodeEditorHandle) => void;
}

function modeConfig(p: Pick<CodeEditorProps, "mode" | "wrapLines" | "path">): ModeConfig {
  return { mode: p.mode ?? "Read", wrap: p.wrapLines, ariaLabel: `File: ${p.path ?? ""}` };
}

/** React facade over the EditorEngine port; it never touches CodeMirror directly. */
export const CodeEditor = forwardRef<CodeEditorHandle, CodeEditorProps>(function CodeEditor(props, ref) {
  const { content, language, path, mode = "Read", wrapLines, gutterMarks, onReady } = props;
  const hostRef = useRef<HTMLDivElement>(null);
  const engineRef = useRef<EngineHandle | null>(null);
  const live = useRef(props);
  live.current = props;

  // The engine mounts asynchronously (lazy chunk), so callers hold a stable delegate.
  const delegate = useMemo<CodeEditorHandle>(() => {
    const e = () => {
      if (!engineRef.current) throw new Error("CodeEditor: engine not ready (wait for onReady)");
      return engineRef.current;
    };
    return {
      get features() { return e().features; },
      createBufferState: (init) => e().createBuffer({ ...init, config: modeConfig(live.current) }),
      getState: () => e().getBuffer(),
      setBufferState: (s) => e().setBuffer(s),
      getText: () => e().getText(),
      getCursor: () => e().getCursor(),
      insertText: (t) => e().insertText(t),
      gotoLine: (n) => e().reveal(n),
      scrollSnapshot: () => e().scrollSnapshot(),
      restoreScroll: (s) => e().restoreScroll(s),
      focus: () => e().focus(),
    };
  }, []);
  useImperativeHandle(ref, () => delegate, [delegate]);

  // Mount once; later prop changes go through mode/buffer updates, never a rebuild.
  useEffect(() => {
    let cancelled = false;
    let mounted: EngineHandle | null = null;
    (async () => {
      const { CmEngine } = await import("./files/engine/CmEngine");
      if (cancelled || !hostRef.current) return;
      const engine = new CmEngine();
      const cur = live.current;
      const config = modeConfig(cur);
      const buffer = await engine.createBuffer({
        doc: cur.content, language: cur.language, gutterMarks: cur.gutterMarks, config,
      });
      if (cancelled || !hostRef.current) return;
      mounted = engine.mount(hostRef.current, { ...config, buffer });
      engineRef.current = mounted;
      mounted.setMode(modeConfig(live.current)); // props may have moved while the chunk loaded
      live.current.onReady?.(delegate);
    })();
    return () => {
      cancelled = true;
      mounted?.destroy();
      engineRef.current = null;
    };
  }, [delegate]);

  useEffect(() => {
    engineRef.current?.setMode(modeConfig({ mode, wrapLines, path }));
  }, [mode, wrapLines, path]);

  // A read-only buffer follows its source; an editable one never has its doc replaced from props.
  const seeded = useRef({ content, language, gutterMarks });
  useEffect(() => {
    const prev = seeded.current;
    seeded.current = { content, language, gutterMarks };
    const engine = engineRef.current;
    if (!engine || mode === "Edit") return;
    if (prev.content === content && prev.language === language && prev.gutterMarks === gutterMarks) return;
    void engine
      .createBuffer({ doc: content, language, gutterMarks, config: modeConfig({ mode, wrapLines, path }) })
      .then((b) => engine.setBuffer(b));
  }, [content, language, gutterMarks, mode, wrapLines, path]);

  return <div ref={hostRef} className={codeEditor} />;
});
