// Differential CSS census (plan Story 5.10): compares the CSS of a Next build and a Bun build of one commit.
//   bun build/oracle/census-cli.ts --next <next out dir> --bun <bun out dir>
// Exit 1 on an unexplained difference in (a) the custom-property map of any theme class or (b) the
// last-written value of any (selector, property) both builds define: reordering that changes the cascade (Bun bug
// #45730) shows up there, while counts stay equal. Wholesale module reordering is reported but not failed.
// Static, not browser-resolved: both builds keep the same VE class names and var() references, so a value that
// differs after canonicalising notation is a real difference. Pure Node apart from the CLI block.
import { readFileSync } from "node:fs";
import path from "node:path";

export interface Rule {
  /** Enclosing at-rule preludes, outermost first (e.g. "@media (min-width:1px)"). */
  at: string[];
  selectors: string[];
  decls: [string, string][];
}

const WS = /\s+/g;

/** Names a CSS minifier substitutes for an equal rgb value; expanded so both notations compare equal. */
const NAMED_COLORS: Record<string, string> = Object.fromEntries(
  "black:000000 silver:c0c0c0 gray:808080 white:ffffff maroon:800000 red:ff0000 purple:800080 green:008000 lime:00ff00 olive:808000 yellow:ffff00 navy:000080 blue:0000ff teal:008080 aqua:00ffff cyan:00ffff fuchsia:ff00ff magenta:ff00ff orange:ffa500 gold:ffd700 pink:ffc0cb brown:a52a2a tan:d2b48c indigo:4b0082 violet:ee82ee khaki:f0e68c coral:ff7f50 salmon:fa8072 orchid:da70d6 peru:cd853f plum:dda0dd sienna:a0522d tomato:ff6347 wheat:f5deb3 snow:fffafa ivory:fffff0 beige:f5f5dc bisque:ffe4c4 azure:f0ffff crimson:dc143c linen:faf0e6"
    .split(" ")
    .map((p) => p.split(":") as [string, string]),
);

/** Splits on `sep` at nesting depth 0, ignoring strings and parentheses. */
function splitTop(text: string, sep: string): string[] {
  const out: string[] = [];
  let depth = 0;
  let quote = "";
  let cur = "";
  for (const ch of text) {
    if (quote) {
      if (ch === quote) quote = "";
    } else if (ch === '"' || ch === "'") quote = ch;
    else if (ch === "(" || ch === "[") depth++;
    else if (ch === ")" || ch === "]") depth--;
    else if (ch === sep && depth === 0) {
      out.push(cur);
      cur = "";
      continue;
    }
    cur += ch;
  }
  out.push(cur);
  return out;
}

function parseDecls(body: string): [string, string][] {
  return splitTop(body, ";")
    .map((d) => d.trim())
    .filter(Boolean)
    .flatMap((d): [string, string][] => {
      const i = d.indexOf(":");
      return i === -1 ? [] : [[d.slice(0, i).trim().toLowerCase(), d.slice(i + 1).trim()]];
    });
}

/** Finds the `}` matching the `{` at `open`, skipping strings. */
function closeOf(css: string, open: number): number {
  let depth = 0;
  let quote = "";
  for (let i = open; i < css.length; i++) {
    const ch = css[i];
    if (quote) {
      if (ch === "\\") i++;
      else if (ch === quote) quote = "";
    } else if (ch === '"' || ch === "'") quote = ch;
    else if (ch === "{") depth++;
    else if (ch === "}" && --depth === 0) return i;
  }
  throw new Error("unbalanced braces in CSS");
}

export function parseRules(rawCss: string, at: string[] = []): Rule[] {
  const css = rawCss.replace(/\/\*[\s\S]*?\*\//g, "");
  const rules: Rule[] = [];
  let i = 0;
  while (i < css.length) {
    const open = css.indexOf("{", i);
    const semi = css.indexOf(";", i);
    if (open === -1) break;
    if (semi !== -1 && semi < open && css.slice(i, semi).trim().startsWith("@")) {
      i = semi + 1; // @import / @charset
      continue;
    }
    const prelude = css.slice(i, open).trim().replace(WS, " ");
    const close = closeOf(css, open);
    const body = css.slice(open + 1, close);
    if (prelude.startsWith("@")) {
      const nested = /^@(media|supports|layer|container|scope)\b/.test(prelude);
      if (nested) rules.push(...parseRules(body, [...at, prelude]));
      else rules.push({ at: [...at, prelude], selectors: [], decls: parseDecls(body.includes("{") ? "" : body) });
    } else {
      rules.push({ at, selectors: splitTop(prelude, ",").map((s) => s.trim().replace(WS, " ")), decls: parseDecls(body) });
    }
    i = close + 1;
  }
  return rules;
}

function hexToRgba(hex: string): string {
  const h = hex.length <= 5 ? [...hex].map((c) => c + c).join("") : hex;
  const [r, g, b, a = "ff"] = h.match(/../g) as string[];
  return `rgba(${parseInt(r, 16)},${parseInt(g, 16)},${parseInt(b, 16)},${+(parseInt(a, 16) / 255).toFixed(2)})`;
}

/** Canonicalises notation the CSS minifier rewrites (hex length, rgb/rgba, leading zeros, ms vs s). */
export function canonicalValue(value: string): string {
  return value
    .trim()
    .toLowerCase()
    .replace(WS, " ")
    .replace(/(^|[\s,(])([a-z]+)(?=$|[\s,)])/g, (m, pre: string, name: string) => (NAMED_COLORS[name] ? `${pre}#${NAMED_COLORS[name]}` : m))
    .replace(/#([0-9a-f]{8}|[0-9a-f]{6}|[0-9a-f]{4}|[0-9a-f]{3})\b/g, (_m, h: string) => hexToRgba(h))
    .replace(/rgba?\(\s*([^)]*)\)/g, (_m, args: string) => {
      const [r, g, b, a = "1"] = args.split(/[\s,/]+/).filter(Boolean);
      return `rgba(${r},${g},${b},${+Number(a.endsWith("%") ? parseFloat(a) / 100 : a).toFixed(2)})`;
    })
    .replace(/\s*,\s*/g, ",")
    .replace(/(\d*\.?\d+)ms\b/g, (_m, n: string) => `${+(parseFloat(n) / 1000).toFixed(6)}s`)
    .replace(/(^|[^\d.])0+(\.\d)/g, "$1$2")
    .replace(/["']/g, '"');
}

export function themeMaps(rules: Rule[], classes: Record<string, string>): Record<string, Record<string, string>> {
  const out: Record<string, Record<string, string>> = {};
  for (const [name, cls] of Object.entries(classes)) {
    const props: Record<string, string> = {};
    for (const r of rules) {
      if (r.at.length > 0 || !r.selectors.includes(`.${cls}`)) continue;
      for (const [k, v] of r.decls) if (k.startsWith("--")) props[k] = canonicalValue(v);
    }
    out[name] = props;
  }
  return out;
}

/** Flattened `at | selector` keys in source order; merged selector lists expand to their members. */
export function selectorSequence(rules: Rule[]): string[] {
  return rules.flatMap((r) => r.selectors.map((s) => `${r.at.join(" > ")} | ${s}`));
}

/** Elements of `bun` (restricted to those also in `next`) that are out of order relative to `next`. */
export function reorderedSelectors(next: string[], bun: string[]): string[] {
  const nextIndex = new Map<string, number>();
  next.forEach((s, i) => nextIndex.has(s) || nextIndex.set(s, i));
  const seen = new Set<string>();
  const common = bun.filter((s) => nextIndex.has(s) && !seen.has(s) && seen.add(s));
  // Longest increasing subsequence of Next indices along Bun order; everything outside it moved.
  const tails: number[] = [];
  const prev: number[] = [];
  const tailAt: number[] = [];
  common.forEach((s, i) => {
    const v = nextIndex.get(s) as number;
    let lo = 0;
    let hi = tails.length;
    while (lo < hi) {
      const mid = (lo + hi) >> 1;
      if (tails[mid] < v) lo = mid + 1;
      else hi = mid;
    }
    tails[lo] = v;
    tailAt[lo] = i;
    prev[i] = lo > 0 ? tailAt[lo - 1] : -1;
  });
  const keep = new Set<number>();
  for (let k = tailAt[tails.length - 1]; k !== undefined && k >= 0; k = prev[k]) keep.add(k);
  return common.filter((_s, i) => !keep.has(i));
}

export interface CensusReport {
  themeDiffs: string[];
  /** (selector, property) pairs written more than once whose winning value differs between builds: a cascade change. */
  cascadeDiffs: string[];
  /** Shared selectors whose relative order differs. Informational: Bun orders CSS by its import graph, Next by its own. */
  movedSelectors: number;
  counts: { next: number; bun: number; commonSelectors: number };
}

/** key `at | selector | property` to the winning (last) value and how often it is written. */
export function winningValues(rules: Rule[]): Map<string, { value: string; writes: number }> {
  const out = new Map<string, { value: string; writes: number }>();
  for (const r of rules) {
    for (const sel of r.selectors) {
      for (const [prop, value] of r.decls) {
        const key = `${r.at.join(" > ")} | ${sel} | ${prop}`;
        out.set(key, { value: canonicalValue(value), writes: (out.get(key)?.writes ?? 0) + 1 });
      }
    }
  }
  return out;
}

export function compareCensus(nextCss: string, bunCss: string, classes: Record<string, string>): CensusReport {
  const nextRules = parseRules(nextCss);
  const bunRules = parseRules(bunCss);
  const nextMaps = themeMaps(nextRules, classes);
  const bunMaps = themeMaps(bunRules, classes);
  const themeDiffs: string[] = [];
  for (const name of Object.keys(classes)) {
    for (const k of new Set([...Object.keys(nextMaps[name]), ...Object.keys(bunMaps[name])])) {
      if (nextMaps[name][k] !== bunMaps[name][k]) themeDiffs.push(`${name} ${k}: next=${nextMaps[name][k]} bun=${bunMaps[name][k]}`);
    }
  }
  const nextWin = winningValues(nextRules);
  const cascadeDiffs: string[] = [];
  for (const [key, bun] of winningValues(bunRules)) {
    const next = nextWin.get(key);
    // A property written once per build cannot depend on source order, so only repeated writes can differ by reordering.
    if (next && (next.writes > 1 || bun.writes > 1) && next.value !== bun.value) cascadeDiffs.push(`${key}: next=${next.value} bun=${bun.value}`);
  }
  const nextSeq = selectorSequence(nextRules);
  const bunSeq = selectorSequence(bunRules);
  const nextSet = new Set(nextSeq);
  return {
    themeDiffs,
    cascadeDiffs,
    movedSelectors: reorderedSelectors(nextSeq, bunSeq).length,
    counts: { next: nextRules.length, bun: bunRules.length, commonSelectors: bunSeq.filter((s) => nextSet.has(s)).length },
  };
}

const LINK_HREF = /<link[^>]*\brel=["']stylesheet["'][^>]*\bhref=["']([^"']+)["']|<link[^>]*\bhref=["']([^"']+\.css)["'][^>]*\brel=["']stylesheet["']/g;

/** The render-blocking stylesheets linked from index.html, in document order: the shared, non-lazy CSS. */
export function sharedCss(outDir: string): string {
  const html = readFileSync(path.join(outDir, "index.html"), "utf8");
  const hrefs = [...html.matchAll(LINK_HREF)].map((m) => m[1] ?? m[2]);
  if (hrefs.length === 0) throw new Error(`no stylesheet links in ${outDir}/index.html`);
  return hrefs.map((h) => readFileSync(path.join(outDir, h.split("?")[0]), "utf8")).join("\n");
}

export function themeClassesOf(outDir: string): Record<string, string> {
  const m = /var m=(\{[^}]*\});/.exec(readFileSync(path.join(outDir, "index.html"), "utf8"));
  if (!m) throw new Error(`no theme boot script in ${outDir}/index.html (pass the Bun build as --bun)`);
  return JSON.parse(m[1]) as Record<string, string>;
}
