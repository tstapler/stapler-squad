import { EditorView } from "@codemirror/view";
import type { Extension } from "@codemirror/state";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { tags } from "@lezer/highlight";
import { vars } from "@/styles/theme.css";

// Editor chrome sourced from the theme-invariant terminal token family, matching FilesTab.css.ts's container.
const chrome = EditorView.theme(
  {
    "&": {
      color: vars.color.terminalForeground,
      backgroundColor: vars.color.terminalBackground,
      height: "100%",
    },
    ".cm-content": { caretColor: vars.color.terminalCursor },
    ".cm-cursor, .cm-dropCursor": { borderLeftColor: vars.color.terminalCursor },
    "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, .cm-content ::selection": {
      backgroundColor: `${vars.color.terminalHoverBg} !important`,
    },
    ".cm-activeLine": { backgroundColor: vars.color.terminalHoverBg },
    ".cm-activeLineGutter": { backgroundColor: vars.color.terminalHoverBg },
    ".cm-gutters": {
      backgroundColor: vars.color.terminalBackground,
      color: vars.color.terminalTextMuted,
      borderRight: `1px solid ${vars.color.terminalBorder}`,
    },
    ".cm-lineNumbers .cm-gutterElement": { color: vars.color.terminalTextMuted },
  },
  { dark: true },
);

// Syntax-token colors reuse the app's semantic tokens so they track the accent/status palette.
const tokens = HighlightStyle.define([
  { tag: [tags.keyword, tags.controlKeyword, tags.operatorKeyword, tags.modifier], color: vars.color.primary },
  { tag: [tags.string, tags.special(tags.string)], color: vars.color.success },
  { tag: [tags.comment, tags.lineComment, tags.blockComment], color: vars.color.terminalTextMuted, fontStyle: "italic" },
  { tag: [tags.number, tags.bool, tags.null], color: vars.color.warning },
  { tag: [tags.function(tags.variableName), tags.function(tags.propertyName)], color: vars.color.accentText },
  { tag: [tags.definition(tags.variableName), tags.definition(tags.propertyName)], color: vars.color.terminalForeground },
  { tag: [tags.className, tags.typeName], color: vars.color.primary },
  { tag: [tags.tagName, tags.attributeName], color: vars.color.gitModified },
  { tag: tags.invalid, color: vars.color.error },
]);

export const cmTheme: Extension = [chrome, syntaxHighlighting(tokens)];
