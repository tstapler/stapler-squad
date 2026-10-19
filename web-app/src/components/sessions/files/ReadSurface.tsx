"use client";

import { useMemo } from "react";
import { textPageSource } from "@/lib/files/pageSource";
import type { GutterMarkType } from "@/lib/utils/parseDiff";
import { CodeEditor } from "../CodeEditor";
import { AccessibleView } from "./AccessibleView";
import type { ReadViewHandle } from "./readViewHandle";

interface ReadSurfaceProps {
  accessible: boolean;
  dirty: boolean;
  content: string;
  language: string;
  path: string;
  wrapLines: boolean;
  gutterMarks?: Map<number, GutterMarkType>;
  onReady?: (handle: ReadViewHandle) => void;
}

/** The file body in Read mode: CodeMirror, or the plain paged Accessible view when the device chose it. */
export function ReadSurface({ accessible, dirty, content, language, path, wrapLines, gutterMarks, onReady }: ReadSurfaceProps) {
  const source = useMemo(() => (accessible ? textPageSource(content) : null), [accessible, content]);
  if (source) {
    return (
      <AccessibleView
        path={path}
        source={source}
        dirty={dirty}
        saveDisabledReason="saving is not available yet"
        onReady={onReady}
      />
    );
  }
  return <CodeEditor content={content} language={language} path={path} mode="Read" wrapLines={wrapLines} gutterMarks={gutterMarks} onReady={onReady} />;
}
