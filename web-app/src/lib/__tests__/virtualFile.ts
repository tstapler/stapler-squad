// Deterministic fake file source for the lazy-document spike page and the Story 1.6.4 tests (moved here from
// app/spike-lazydoc, Task 1.6.4a, so tests need not import from app/). No storage: each line is
// regenerated from (seed, index), so a 1,000,000-line / ~50 MB file costs nothing until fetched.

export const VIRTUAL_LINE_COUNT = 1_000_000;
const FILLER = "abcdefghijklmnopqrstuvwxyz0123456789 _-=+(){}[];:,.";

function hash32(n: number): number {
  let h = (n ^ 0x9e3779b9) >>> 0;
  h = Math.imul(h ^ (h >>> 16), 0x85ebca6b);
  h = Math.imul(h ^ (h >>> 13), 0xc2b2ae35);
  return (h ^ (h >>> 16)) >>> 0;
}

/** Line text for 0-based index `i`: "L<i+1> " plus 14..74 filler chars, ~50 bytes on average. */
export function lineText(i: number, seed = 1): string {
  const h = hash32(i * 2654435761 + seed);
  const len = 14 + (h % 61);
  let out = `L${i + 1} `;
  let x = h;
  for (let k = 0; k < len; k++) {
    x = hash32(x + k);
    out += FILLER[x % FILLER.length];
  }
  return out;
}

export class VirtualDoc {
  constructor(readonly lineCount = VIRTUAL_LINE_COUNT, readonly seed = 1) {}

  fetch(start: number, count: number): string[] {
    const end = Math.min(this.lineCount, start + count);
    const out: string[] = [];
    for (let i = start; i < end; i++) out.push(lineText(i, this.seed));
    return out;
  }
}

/** mulberry32: small seeded PRNG so every spike and oracle run is reproducible. */
export function prng(seed: number): () => number {
  let a = seed >>> 0;
  return () => {
    a = (a + 0x6d2b79f5) >>> 0;
    let t = a;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}
