"use client";

import { useEffect, useState } from "react";
import { WindowedFile } from "@/lib/files/windowedFile";
import { useWindowedState } from "@/lib/files/useWindowedState";
import type { BufferSource } from "@/lib/files/bufferReducer";
import { useFileTransport } from "@/lib/hooks/useFileTransport";
import { CodeEditor, type CodeEditorHandle } from "../CodeEditor";
import { LargeFileStates } from "./LargeFileStates";
import type { ReadViewHandle } from "./readViewHandle";

type Windowed = Extract<BufferSource, { kind: "windowed" }>;

export interface WindowedFileViewProps {
  sessionId: string;
  path: string;
  source: Windowed;
  language: string;
  /** Changes when the file must be read again (stale-banner Reload). */
  reloadKey?: number;
  onReady?: (handle: ReadViewHandle) => void;
  /** Range reads are gone (transport demotion): the caller falls back to the truncated whole-file read. */
  onDemoted?: () => void;
}

let instances = 0;

/** The editor handle the bridge sees: absolute line numbers, jumps that load their window first. */
function bridgeHandle(editor: CodeEditorHandle, file: WindowedFile): ReadViewHandle {
  return {
    getText: () => editor.getText(),
    getCursor: () => editor.getCursor(),
    gotoLine: (n) => { void file.goTo(n); },
    highlightRange: (r) => { void file.settled().then(() => editor.highlightRange(r)); },
    scrollSnapshot: () => editor.scrollSnapshot(),
    restoreScroll: (s) => editor.restoreScroll(s),
    focus: () => editor.focus(),
    windowed: file,
  };
}

/** A windowed (T1/T2) file in Read mode: CodeMirror over a lazy document that loads line windows on demand. */
export function WindowedFileView({ sessionId, path, source, language, reloadKey, onReady, onDemoted }: WindowedFileViewProps) {
  const transport = useFileTransport();
  const [current, setCurrent] = useState<{ file: WindowedFile; id: number } | null>(null);
  useEffect(() => {
    const f = new WindowedFile(transport, { sessionId, path }, source.size);
    setCurrent({ file: f, id: ++instances });
    return () => { f.dispose(); setCurrent(null); };
  }, [transport, sessionId, path, source.size, reloadKey]);
  const file = current?.file ?? null;
  const state = useWindowedState(file);
  useEffect(() => { if (state.demoted) onDemoted?.(); }, [state.demoted, onDemoted]);

  if (!file) return null;
  return (
    <>
      <LargeFileStates file={file} state={state} />
      <CodeEditor
        key={current?.id}
        content=""
        language={language}
        path={path}
        mode="Read"
        onReady={(editor) => {
          void file.attach(editor, language, source.windowStartLine);
          onReady?.(bridgeHandle(editor, file));
        }}
      />
    </>
  );
}
