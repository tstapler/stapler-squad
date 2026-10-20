/**
 * Pure logic for the Next-vs-Bun computed-style parity harness (Story 7.9). No browser here, so
 * the self-test (parity-diff-selftest.spec.ts) can prove the harness fails on a seeded difference.
 */

export interface StyleSnapshot {
  /** Resolved custom properties of :root, name -> value. */
  rootVars: Record<string, string>;
  /** Computed style sample, keyed by `selector|state|property`. */
  styles: Record<string, string>;
}

export interface Difference {
  route: string;
  /** Matrix cell (viewport, theme, walk) the difference was seen in; informational, not matched by the allow-list. */
  cell?: string;
  selector: string;
  property: string;
  a: string;
  b: string;
}

export interface AllowEntry {
  route: string;
  selector: string;
  property: string;
  reason: string;
}

export const ROOT_SELECTOR = ':root';

/** A reason must be a URL or a quoted CSS rule, so every allowed difference is traceable. */
const REASON_PATTERN = /^(https?:\/\/\S+|"[^"]*\{[^}]*\}[^"]*")$/;

export function validateAllowList(entries: unknown): string[] {
  if (!Array.isArray(entries)) return ['allow-list must be a JSON array'];
  const problems: string[] = [];
  entries.forEach((entry: Partial<AllowEntry>, index) => {
    for (const key of ['route', 'selector', 'property', 'reason'] as const) {
      if (typeof entry?.[key] !== 'string' || entry[key] === '') {
        problems.push(`entry ${index}: missing ${key}`);
      }
    }
    if (typeof entry?.reason === 'string' && entry.reason !== '' && !REASON_PATTERN.test(entry.reason)) {
      problems.push(`entry ${index}: reason must be a URL or a quoted CSS rule, got ${JSON.stringify(entry.reason)}`);
    }
  });
  return problems;
}

/** Differences between two snapshots of the same route/viewport/theme cell. */
export function diffSnapshots(route: string, a: StyleSnapshot, b: StyleSnapshot, cell?: string): Difference[] {
  const out: Difference[] = [];
  const names = new Set([...Object.keys(a.rootVars), ...Object.keys(b.rootVars)]);
  for (const name of names) {
    if (a.rootVars[name] !== b.rootVars[name]) {
      out.push({ route, cell, selector: ROOT_SELECTOR, property: name, a: a.rootVars[name] ?? '', b: b.rootVars[name] ?? '' });
    }
  }
  const keys = new Set([...Object.keys(a.styles), ...Object.keys(b.styles)]);
  for (const key of keys) {
    if (a.styles[key] !== b.styles[key]) {
      const [selector, state, property] = key.split('|');
      out.push({ route, cell, selector: state === 'base' ? selector : `${selector}:${state}`, property, a: a.styles[key] ?? '', b: b.styles[key] ?? '' });
    }
  }
  return out;
}

export function isAllowed(diff: Difference, allow: AllowEntry[]): boolean {
  return allow.some(
    (e) =>
      (e.route === '*' || e.route === diff.route) &&
      (e.selector === '*' || e.selector === diff.selector) &&
      e.property === diff.property,
  );
}

export function unallowedDifferences(diffs: Difference[], allow: AllowEntry[]): Difference[] {
  return diffs.filter((d) => !isAllowed(d, allow));
}

/** Deterministic PRNG (mulberry32) so a failing navigation ordering can be replayed from its seed. */
export function seededRandom(seed: number): () => number {
  let state = seed >>> 0;
  return () => {
    state = (state + 0x6d2b79f5) >>> 0;
    let t = state;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

export function seededShuffle<T>(items: readonly T[], seed: number): T[] {
  const random = seededRandom(seed);
  const copy = [...items];
  for (let i = copy.length - 1; i > 0; i--) {
    const j = Math.floor(random() * (i + 1));
    [copy[i], copy[j]] = [copy[j], copy[i]];
  }
  return copy;
}
