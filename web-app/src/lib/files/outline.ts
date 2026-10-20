// Symbol outline without LSP or a parser: per-language declaration maps over lines. The editor's
// syntax tree is unreachable here (ADR-036 keeps @lezer/* inside the engine adapter), and
// declarations at line starts are all the outline lists.

export type OutlineKind = "function" | "method" | "type" | "class" | "heading" | "impl";
export interface OutlineSymbol { name: string; line: number; kind: OutlineKind; depth: number }
export type OutlineResult =
  | { kind: "symbols"; symbols: OutlineSymbol[] }
  | { kind: "unavailable"; reason: "large" | "unsupported" | "error" };

/** Stand-in for the highlight-off threshold until Story 1.5.2 defines it. */
export const OUTLINE_MAX_BYTES = 1024 * 1024;

type Hit = { name: string; kind: OutlineKind; depth?: number };
type LineRule = (line: string) => Hit | null;
type Scanner = (lines: readonly string[]) => OutlineSymbol[];

/** Tries each regex in order; the first match wins and its builder names the symbol. */
function rules(...table: [RegExp, (m: RegExpMatchArray) => Hit][]): LineRule {
  return (line) => {
    for (const [re, build] of table) {
      const m = line.match(re);
      if (m) return build(m);
    }
    return null;
  };
}

const perLine = (rule: LineRule): Scanner => (lines) =>
  lines.flatMap((text, i) => {
    const hit = rule(text);
    return hit ? [{ name: hit.name, line: i + 1, kind: hit.kind, depth: hit.depth ?? 0 }] : [];
  });

const ID = "[A-Za-z_$][\\w$]*";
const indentDepth = (indent: string, unit = 4) => Math.floor(indent.replace(/\t/g, "    ").length / unit);

const go = perLine(rules(
  [new RegExp(`^func\\s+\\(\\s*(?:\\w+\\s+)?\\*?(${ID})(?:\\[[^\\]]*\\])?\\s*\\)\\s*(${ID})`), (m) => ({ name: `${m[1]}.${m[2]}`, kind: "method" })],
  [new RegExp(`^func\\s+(${ID})`), (m) => ({ name: m[1], kind: "function" })],
  [new RegExp(`^type\\s+(${ID})`), (m) => ({ name: m[1], kind: "type" })],
));

const script = perLine(rules(
  [new RegExp(`^(?:export\\s+)?(?:default\\s+)?(?:async\\s+)?function\\*?\\s+(${ID})`), (m) => ({ name: m[1], kind: "function" })],
  [new RegExp(`^(?:export\\s+)?(?:default\\s+)?(?:abstract\\s+)?class\\s+(${ID})`), (m) => ({ name: m[1], kind: "class" })],
  [new RegExp(`^(?:export\\s+)?(?:interface|enum)\\s+(${ID})`), (m) => ({ name: m[1], kind: "type" })],
  [new RegExp(`^(?:export\\s+)?type\\s+(${ID})\\b`), (m) => ({ name: m[1], kind: "type" })],
  [new RegExp(`^(?:export\\s+)?const\\s+(${ID})\\s*(?::[^=]+)?=\\s*(?:async\\s*)?(?:\\([^)]*\\)|${ID})\\s*(?::[^=]+)?=>`), (m) => ({ name: m[1], kind: "function" })],
));

const python = perLine(rules(
  [new RegExp(`^(\\s*)(?:async\\s+)?def\\s+(${ID})`), (m) => ({ name: m[2], kind: m[1] ? "method" : "function", depth: indentDepth(m[1]) })],
  [new RegExp(`^(\\s*)class\\s+(${ID})`), (m) => ({ name: m[2], kind: "class", depth: indentDepth(m[1]) })],
));

const VIS = "(?:pub(?:\\([^)]*\\))?\\s+)?";
const rust = perLine(rules(
  [new RegExp(`^(\\s*)${VIS}(?:const\\s+)?(?:async\\s+)?(?:unsafe\\s+)?fn\\s+(${ID})`), (m) => ({ name: m[2], kind: "function", depth: indentDepth(m[1]) })],
  [new RegExp(`^(\\s*)${VIS}(?:struct|enum|trait|type|union)\\s+(${ID})`), (m) => ({ name: m[2], kind: "type", depth: indentDepth(m[1]) })],
  [new RegExp(`^(\\s*)impl(?:<[^>]*>)?\\s+(?:[\\w:<>]+\\s+for\\s+)?(${ID})`), (m) => ({ name: `impl ${m[2]}`, kind: "impl", depth: indentDepth(m[1]) })],
));

const MOD = "(?:(?:public|private|protected|static|final|abstract|synchronized|default)\\s+)";
const java = perLine(rules(
  [new RegExp(`^(\\s*)${MOD}*(?:class|interface|enum|record)\\s+(${ID})`), (m) => ({ name: m[2], kind: "class", depth: indentDepth(m[1]) })],
  [new RegExp(`^(\\s+)${MOD}+[\\w<>\\[\\], ?]+\\s+(${ID})\\s*\\(`), (m) => ({ name: m[2], kind: "method", depth: indentDepth(m[1]) })],
));

const markdown: Scanner = (lines) => {
  let fenced = false;
  return lines.flatMap((text, i) => {
    if (/^\s*(```|~~~)/.test(text)) {
      fenced = !fenced;
      return [];
    }
    const m = !fenced && text.match(/^(#{1,6})\s+(.+?)\s*#*\s*$/);
    return m ? [{ name: m[2], line: i + 1, kind: "heading" as const, depth: m[1].length - 1 }] : [];
  });
};

const SCANNERS: Record<string, Scanner> = {
  go, typescript: script, tsx: script, javascript: script, jsx: script, python, rust, java, markdown,
};

export const outlineLanguages = (): string[] => Object.keys(SCANNERS);

/** Never throws: any failure is reported as `unavailable`. */
export function buildOutline(text: string, language: string, opts: { size?: number } = {}): OutlineResult {
  const scan = SCANNERS[language];
  if (!scan) return { kind: "unavailable", reason: "unsupported" };
  if ((opts.size ?? text?.length ?? 0) > OUTLINE_MAX_BYTES) return { kind: "unavailable", reason: "large" };
  try {
    return { kind: "symbols", symbols: scan(text.split("\n")) };
  } catch {
    return { kind: "unavailable", reason: "error" };
  }
}
