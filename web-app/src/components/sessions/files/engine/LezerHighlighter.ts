import { highlightTree, tagHighlighter, tags } from "@lezer/highlight";
import type { LanguageSupport } from "@codemirror/language";
import type { Highlighter, TokenSpan } from "./Highlighter";
import { loadCodemirrorLang } from "./languages";

// Class names are styled by DiffRenderer.css.ts (syntaxScope); keep the two in sync.
const classes = tagHighlighter([
  { tag: [tags.keyword, tags.controlKeyword, tags.operatorKeyword, tags.modifier], class: "ssq-tok-keyword" },
  { tag: [tags.string, tags.special(tags.string)], class: "ssq-tok-string" },
  { tag: [tags.comment, tags.lineComment, tags.blockComment], class: "ssq-tok-comment" },
  { tag: [tags.number, tags.bool, tags.null], class: "ssq-tok-number" },
  { tag: [tags.function(tags.variableName), tags.function(tags.propertyName)], class: "ssq-tok-function" },
  { tag: [tags.className, tags.typeName], class: "ssq-tok-type" },
  { tag: [tags.tagName, tags.attributeName], class: "ssq-tok-tag" },
  { tag: tags.invalid, class: "ssq-tok-invalid" },
]);

/** Tokenizes with the same Lezer grammars the full editor loads; unknown languages yield no spans. */
export class LezerHighlighter implements Highlighter {
  async tokenize(range: { startLine: number; endLine: number; text: string }, languageId: string): Promise<TokenSpan[]> {
    if (!range.text) return [];
    const ext = await loadCodemirrorLang(languageId);
    const language = (ext as LanguageSupport | null)?.language;
    if (!language) return [];

    const spans: TokenSpan[] = [];
    highlightTree(language.parser.parse(range.text), classes, (from, to, className) => {
      spans.push({ from, to, className });
    });
    return spans;
  }
}
