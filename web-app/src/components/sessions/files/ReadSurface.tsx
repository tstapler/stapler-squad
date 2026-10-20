"use client";

import { useMemo } from "react";
import { bucketBytes } from "@/lib/files/analytics/events";
import { degradeProfile } from "@/lib/files/degradeProfile";
import { useCoarsePointer } from "@/lib/hooks/useCoarsePointer";
import { textPageSource } from "@/lib/files/pageSource";
import type { GutterMarkType } from "@/lib/utils/parseDiff";
import { CodeEditor } from "../CodeEditor";
import { AccessibleView } from "./AccessibleView";
import type { ReadViewHandle } from "./readViewHandle";
import { useOpenLatency } from "./useOpenLatency";

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
  const coarse = useCoarsePointer();
  const latency = useOpenLatency();
  const ready = (handle: ReadViewHandle) => {
    const degraded = degradeProfile({ doc: content, pointerCoarse: coarse }).notice !== null;
    latency.finish({ size: bucketBytes(content.length), degraded: degraded ? "true" : "false", tier: "t0" });
    onReady?.(handle);
  };
  if (source) {
    return (
      <AccessibleView
        path={path}
        source={source}
        dirty={dirty}
        saveDisabledReason="saving is not available yet"
        onReady={ready}
      />
    );
  }
  return <CodeEditor content={content} language={language} path={path} mode="Read" wrapLines={wrapLines} gutterMarks={gutterMarks} onReady={ready} />;
}
