/** Structural subset of the proto FlagInfo; the generated type satisfies it. */
export interface FlagOption {
  readonly name: string;
  readonly short: string;
  readonly takesValue: boolean;
  readonly description: string;
  readonly aliases: readonly string[];
}

export interface FlagToken {
  start: number;
  end: number;
  text: string;
}

const isSpace = (ch: string) => /\s/.test(ch);

/**
 * The whitespace-delimited token around `caret`, or null unless it starts with
 * `-`, is not the `--` terminator, and is not inside quotes or after `--`.
 */
export function tokenAtCaret(value: string, caret: number): FlagToken | null {
  const c = Math.max(0, Math.min(caret, value.length));
  let start = c;
  while (start > 0 && !isSpace(value[start - 1])) start--;
  let end = c;
  while (end < value.length && !isSpace(value[end])) end++;
  const text = value.slice(start, end);
  if (!text.startsWith("-") || text === "--") return null;

  const before = value.slice(0, start);
  if (before.split(/\s+/).includes("--")) return null;
  const quotes = (before.match(/["']/g) ?? []).length;
  if (quotes % 2 === 1) return null;
  return { start, end, text };
}

/** Replaces `token` with `flag.name` plus one trailing space (never `=`). */
export function applyCompletion(
  value: string,
  token: FlagToken,
  flag: Pick<FlagOption, "name">,
): { value: string; caret: number } {
  const rest = value.slice(token.end);
  const insert = flag.name + (rest.startsWith(" ") ? "" : " ");
  const head = value.slice(0, token.start) + insert;
  return { value: head + rest, caret: head.length + (rest.startsWith(" ") ? 1 : 0) };
}

/** Prefix matches first, then substring matches, over name, short and aliases. */
export function filterFlags<T extends FlagOption>(flags: readonly T[], text: string): T[] {
  const q = text.toLowerCase();
  if (!q) return [];
  const names = (f: T) => [f.name, f.short, ...f.aliases].filter(Boolean).map((n) => n.toLowerCase());
  const prefix: T[] = [];
  const substring: T[] = [];
  for (const f of flags) {
    const ns = names(f);
    if (ns.some((n) => n.startsWith(q))) prefix.push(f);
    else if (ns.some((n) => n.includes(q))) substring.push(f);
  }
  return [...prefix, ...substring];
}
