"use client";

import { useEffect, useState } from "react";
import { WindowedFile } from "@/lib/files/windowedFile";
import { useWindowedState } from "@/lib/files/useWindowedState";
import type { BufferSource } from "@/lib/files/bufferReducer";
import { useFileTransport } from "@/lib/hooks/useFileTransport";
import { CodeEditor, type CodeEditorHandle } from "../CodeEditor";
import { AccessibleView } from "./AccessibleView";
import { LargeFileStates } from "./LargeFileStates";
import type { ReadViewHandle } from "./readViewHandle";

type Windowed = Extract<BufferSource, { kind: "windowed" }>;

export interface WindowedFileViewProps {
  sessionId: string;
  path: string;
  source: Windowed;
  language: string;
  /** The per-device Accessible view: pages come from the same windowed file through `read(lines)`. */
  accessible?: boolean;
  dirty?: boolean;
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
export function WindowedFileView({ sessionId, path, source, language, accessible = false, dirty = false, reloadKey, onReady, onDemoted }: WindowedFileViewProps) {
  const transport = useFileTransport();
  const [current, setCurrent] = useState<{ file: WindowedFile; id: number } | null>(null);
  useEffect(() => {
    const f = new WindowedFile(transport, { sessionId, path }, source.size);
    setCurrent({ file: f, id: ++instances });
    return () => { f.dispose(); setCurrent(null); };
  }, [transport, sessionId, path, source.size, reloadKey]);
  const file = current?.file ?? null;
  const state = useWindowedState(file);
  // An open that fails has no window to show; the full read is the always-working floor, so fall back like a demotion.
  const fallBack = state.demoted || state.openFailed;
  useEffect(() => { if (fallBack) onDemoted?.(); }, [fallBack, onDemoted]);

  if (!file) return null;
  if (accessible) {
    return <AccessibleView path={path} windowed={file} dirty={dirty} saveDisabledReason="saving is not available yet" onReady={onReady} />;
  }
  return (
    <>
      <LargeFileStates file={file} state={state} />
      <WindowedEditor key={current?.id} file={file} path={path} language={language} startLine={source.windowStartLine} onReady={onReady} />
    </>
  );
}

function WindowedEditor({ file, path, language, startLine, onReady }: { file: WindowedFile; path: string; language: string; startLine: number; onReady?: (handle: ReadViewHandle) => void }) {
  useEffect(() => () => file.detach(), [file]);
  return (
    <CodeEditor
      content=""
      language={language}
      path={path}
      mode="Read"
      windowed
      onReady={(editor) => {
        void file.attach(editor, language, startLine);
        onReady?.(bridgeHandle(editor, file));
      }}
    />
  );
}
