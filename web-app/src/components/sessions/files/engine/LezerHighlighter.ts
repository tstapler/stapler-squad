import { highlightTree, tagHighlighter, tags } from "@lezer/highlight";
import type { LanguageSupport } from "@codemirror/language";
import type { Highlighter, TokenSpan } from "./Highlighter";
import { loadCodemirrorLang } from "./languages";
import { TOKEN_CLASS } from "@/lib/highlight/tokenClasses";

// Class names (TOKEN_CLASS) are styled by DiffRenderer.css.ts (syntaxScope).
const classes = tagHighlighter([
  { tag: [tags.keyword, tags.controlKeyword, tags.operatorKeyword, tags.modifier], class: TOKEN_CLASS.keyword },
  { tag: [tags.string, tags.special(tags.string)], class: TOKEN_CLASS.string },
  { tag: [tags.comment, tags.lineComment, tags.blockComment], class: TOKEN_CLASS.comment },
  { tag: [tags.number, tags.bool, tags.null], class: TOKEN_CLASS.number },
  { tag: [tags.function(tags.variableName), tags.function(tags.propertyName)], class: TOKEN_CLASS.function },
  { tag: [tags.className, tags.typeName], class: TOKEN_CLASS.type },
  { tag: [tags.tagName, tags.attributeName], class: TOKEN_CLASS.tag },
  { tag: tags.invalid, class: TOKEN_CLASS.invalid },
]);

/**
 * Tokenizes with the same Lezer grammars the full editor loads; unknown languages yield no spans.
 * Callers tokenize hunk fragments, so constructs that start outside the fragment (block comments,
 * template strings) can be mis-coloured.
 */
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
