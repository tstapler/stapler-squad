/** Token range within a tokenized text (offsets into the text, `to` exclusive). */
export interface TokenSpan {
  from: number;
  to: number;
  className: string;
}

export interface Highlighter {
  /** `range.text` is the text of the lines in `range`. */
  tokenize(range: { startLine: number; endLine: number; text: string }, languageId: string): Promise<TokenSpan[]>;
}
