// Port so a non-CodeMirror engine can reuse Lezer parsers for syntax highlighting.
import type { LineRange } from "./EditorEngine";

export interface TokenSpan {
  /** Offsets into the range text. */
  from: number;
  to: number;
  className: string;
}

export interface Highlighter {
  /** `range.text` is the text of the lines in `range`. */
  tokenize(range: LineRange & { text: string }, languageId: string): Promise<TokenSpan[]>;
}
