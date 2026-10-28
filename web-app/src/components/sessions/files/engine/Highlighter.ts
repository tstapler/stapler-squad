// Port so a non-CodeMirror engine can reuse Lezer parsers for syntax highlighting.
// Canonical definition lives in lib/highlight so shared/ components can use it without importing sessions/.
export type { Highlighter, TokenSpan } from "@/lib/highlight/types";
