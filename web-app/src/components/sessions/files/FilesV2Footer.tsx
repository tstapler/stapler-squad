"use client";

import { detectLanguage } from "../FileContentViewer";
import type { FilesV2 } from "@/lib/hooks/useFilesV2";
import { FeedbackToast } from "./FeedbackToast";
import { StatusBar } from "./StatusBar";
import { QuickfixChip, type QuickfixView } from "./SearchResults";

const languageName = (id: string) => (id === "text" ? "Plain Text" : id.charAt(0).toUpperCase() + id.slice(1));

/** Status bar and the mobile toast for the active buffer. */
export function FilesV2Footer({ v2, quickfix }: { v2: FilesV2; quickfix?: QuickfixView | null }) {
  const { buffers, status, feedback } = v2;
  const active = buffers.active;
  if (!active) return null;
  const loaded = active.status === "loaded" ? active : null;
  return (
    <>
      {quickfix && <QuickfixChip view={quickfix} />}
      <StatusBar
        line={status.line}
        col={status.col}
        eol={status.eol}
        encoding={loaded ? "UTF-8" : null}
        language={languageName(detectLanguage(active.path))}
        readOnly={loaded?.readOnly ?? "none"}
        dirty={!!loaded?.dirty}
        message={feedback.message}
        onDismissMessage={feedback.dismiss}
        onPause={feedback.pause}
        onResume={feedback.resume}
      />
      <FeedbackToast toast={feedback.toast} onDismiss={feedback.dismiss} onPause={feedback.pause} onResume={feedback.resume} />
    </>
  );
}
