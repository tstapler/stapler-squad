import { MAX_REGION } from "./policy";
import type { CompositionRegion, RewriteEdit, SentText } from "./types";

const segmenter = new Intl.Segmenter(undefined, { granularity: "grapheme" });

/** User-perceived characters; the unit for every DEL count (never UTF-16 units). */
export function graphemes(text: string): string[] {
  return Array.from(segmenter.segment(text), (s) => s.segment);
}

/**
 * Edit that turns what this layer already sent into the new composition region:
 * keep the common grapheme prefix, DEL the rest of `sent`, then insert the new tail.
 * Backspaces never exceed graphemes(sent), and are 0 when `sent` is empty.
 */
export function computeRewrite(
  sent: SentText,
  region: CompositionRegion,
  maxRegion: number = MAX_REGION,
): RewriteEdit {
  const next = graphemes(region);
  if (next.length > maxRegion || /[\r\n]/.test(region)) {
    return { kind: "deferred", anomaly: "rewrite_bound_exceeded" };
  }
  const prev = graphemes(sent);
  let prefix = 0;
  while (prefix < prev.length && prefix < next.length && prev[prefix] === next[prefix]) prefix++;
  return { kind: "edit", backspaces: prev.length - prefix, insert: next.slice(prefix).join("") };
}
