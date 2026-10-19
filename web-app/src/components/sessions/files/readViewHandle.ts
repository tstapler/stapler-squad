import type { CodeEditorHandle } from "../CodeEditor";

/** What the editor bridge needs from whatever shows the file read-only: CodeMirror or the Accessible view. */
export type ReadViewHandle = Pick<
  CodeEditorHandle,
  "getText" | "getCursor" | "gotoLine" | "highlightRange" | "scrollSnapshot" | "restoreScroll" | "focus"
>;
