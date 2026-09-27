import type { FlagOption } from "./flagTokens";

interface Token {
  text: string;
  quoted: boolean;
}

/** Splits on whitespace, honouring single/double quotes; quoted tokens are values, never flags. */
function tokenize(input: string): Token[] {
  const tokens: Token[] = [];
  let text = "";
  let inToken = false;
  let quoted = false;
  let quote = "";
  const flush = () => {
    if (inToken) tokens.push({ text, quoted });
    text = "";
    inToken = false;
    quoted = false;
  };
  for (const ch of input) {
    if (quote) {
      if (ch === quote) quote = "";
      else text += ch;
    } else if (ch === '"' || ch === "'") {
      quote = ch;
      quoted = true;
      inToken = true;
    } else if (/\s/.test(ch)) {
      flush();
    } else {
      text += ch;
      inToken = true;
    }
  }
  flush();
  return tokens;
}

/**
 * Long flags in `input` that `flags` (a probe's parsed --help) does not list.
 * Precision-first: bundled or unknown single-dash tokens, values, `--x=value`
 * names that are known, `--no-<known>` and everything after `--` are never
 * reported. Returns [] when `flags` is empty.
 */
export function validateFlags(input: string, flags: readonly FlagOption[]): string[] {
  if (flags.length === 0) return [];
  const byName = new Map<string, FlagOption>();
  for (const f of flags) {
    for (const n of [f.name, f.short, ...f.aliases]) if (n) byName.set(n, f);
  }

  const unknown: string[] = [];
  let expectsValue = false;
  for (const { text, quoted } of tokenize(input)) {
    if (text === "--" && !quoted) break;
    if (expectsValue) {
      expectsValue = false;
      continue;
    }
    if (quoted || !text.startsWith("-") || text === "-") continue;

    const eq = text.indexOf("=");
    const name = eq === -1 ? text : text.slice(0, eq);
    const known = byName.get(name);
    if (known) {
      expectsValue = known.takesValue && eq === -1;
    } else if (name.startsWith("--no-") && byName.has(`--${name.slice(5)}`)) {
      continue;
    } else if (name.startsWith("--") && !unknown.includes(name)) {
      unknown.push(name);
    }
  }
  return unknown;
}
