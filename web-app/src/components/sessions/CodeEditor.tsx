"use client";

import { forwardRef, useEffect, useImperativeHandle, useMemo, useRef } from "react";
import type { GutterMarkType } from "@/lib/utils/parseDiff";
import { degradeProfile } from "@/lib/files/degradeProfile";
import { useFontPx } from "@/lib/files/fontSize";
import { useCoarsePointer } from "@/lib/hooks/useCoarsePointer";
import type {
  BufferInit, BufferState, EditorMode, EngineFeatures, EngineHandle, HitRange, ModeConfig, ScrollSnapshot,
} from "./files/engine/EditorEngine";
import { codeEditor, codeEditorRoot, degradeNotice } from "./CodeEditor.css";

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
  /** Marks a search hit until the cursor moves; null clears it. */
  highlightRange(range: HitRange | null): void;
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

interface Device { coarse: boolean; fontPx: number }

/** `doc` is the text the buffer is created from; the large-file profile is decided per buffer, not per prop. */
function modeConfig(p: Pick<CodeEditorProps, "mode" | "wrapLines" | "path">, device: Device, doc: string): ModeConfig {
  const mode = p.mode ?? "Read";
  const degrade = degradeProfile({ doc, pointerCoarse: device.coarse });
  return {
    mode,
    wrap: p.wrapLines && degrade.wrap,
    ariaLabel: `File: ${p.path ?? ""}`,
    degrade,
    fontPx: device.fontPx,
    noVirtualKeyboard: device.coarse && mode === "Read",
  };
}

/** React facade over the EditorEngine port; it never touches CodeMirror directly. */
export const CodeEditor = forwardRef<CodeEditorHandle, CodeEditorProps>(function CodeEditor(props, ref) {
  const { content, language, path, mode = "Read", wrapLines, gutterMarks, onReady } = props;
  const hostRef = useRef<HTMLDivElement>(null);
  const engineRef = useRef<EngineHandle | null>(null);
  const coarse = useCoarsePointer();
  const fontPx = useFontPx(coarse);
  const live = useRef({ props, device: { coarse, fontPx } });
  live.current = { props, device: { coarse, fontPx } };
  const notice = useMemo(() => degradeProfile({ doc: content, pointerCoarse: coarse }).notice, [content, coarse]);

  // The engine mounts asynchronously (lazy chunk), so callers hold a stable delegate.
  const delegate = useMemo<CodeEditorHandle>(() => {
    const e = () => {
      if (!engineRef.current) throw new Error("CodeEditor: engine not ready (wait for onReady)");
      return engineRef.current;
    };
    return {
      get features() { return e().features; },
      createBufferState: (init) => e().createBuffer({ ...init, config: modeConfig(live.current.props, live.current.device, init.doc) }),
      getState: () => e().getBuffer(),
      setBufferState: (s) => e().setBuffer(s),
      getText: () => e().getText(),
      getCursor: () => e().getCursor(),
      insertText: (t) => e().insertText(t),
      gotoLine: (n) => e().reveal(n),
      highlightRange: (r) => e().highlight(r),
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
      const { props: cur, device } = live.current;
      const config = modeConfig(cur, device, cur.content);
      const buffer = await engine.createBuffer({
        doc: cur.content, language: cur.language, gutterMarks: cur.gutterMarks, config,
      });
      if (cancelled || !hostRef.current) return;
      mounted = engine.mount(hostRef.current, { ...config, buffer });
      engineRef.current = mounted;
      mounted.setMode(modeConfig(live.current.props, live.current.device, live.current.props.content)); // props may have moved while the chunk loaded
      live.current.props.onReady?.(delegate);
    })();
    return () => {
      cancelled = true;
      mounted?.destroy();
      engineRef.current = null;
    };
  }, [delegate]);

  useEffect(() => {
    engineRef.current?.setMode(modeConfig({ mode, wrapLines, path }, { coarse, fontPx }, content));
    // The doc only decides wrapping here; a new document is handled by the buffer effect below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode, wrapLines, path, coarse, fontPx]);

  // A read-only buffer follows its source; an editable one never has its doc replaced from props.
  const seeded = useRef({ content, language, gutterMarks });
  useEffect(() => {
    const prev = seeded.current;
    seeded.current = { content, language, gutterMarks };
    const engine = engineRef.current;
    if (!engine || mode === "Edit") return;
    if (prev.content === content && prev.language === language && prev.gutterMarks === gutterMarks) return;
    void engine
      .createBuffer({ doc: content, language, gutterMarks, config: modeConfig({ mode, wrapLines, path }, { coarse, fontPx }, content) })
      .then((b) => engine.setBuffer(b));
  }, [content, language, gutterMarks, mode, wrapLines, path, coarse, fontPx]);

  return (
    <div className={codeEditorRoot}>
      {notice && <div className={degradeNotice} role="note" data-testid="degrade-notice">{notice}</div>}
      <div ref={hostRef} className={codeEditor} />
    </div>
  );
});
