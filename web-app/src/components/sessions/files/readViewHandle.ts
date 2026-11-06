import type { CodeEditorHandle } from "../CodeEditor";
import type { WindowedFile } from "@/lib/files/windowedFile";

/** What the editor bridge needs from whatever shows the file read-only: CodeMirror or the Accessible view. */
export type ReadViewHandle = Pick<
  CodeEditorHandle,
  "getText" | "getCursor" | "gotoLine" | "highlightRange" | "scrollSnapshot" | "restoreScroll" | "focus"
> & {
  /** Windowed buffers only: the file's real line count (the loaded text is a fraction) and the live window state. */
  windowed?: WindowedFile;
};
