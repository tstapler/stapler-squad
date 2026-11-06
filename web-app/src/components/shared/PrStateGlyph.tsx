import type { PrRowState } from "@/lib/pr/pullRequests";

const PATHS: Record<PrRowState, string> = {
  // hollow circle with a dot
  open: "M8 1.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13Zm0 1.5a5 5 0 1 1 0 10A5 5 0 0 1 8 3Zm0 3a2 2 0 1 0 0 4 2 2 0 0 0 0-4Z",
  // dashed circle
  draft: "M8 1.5a6.5 6.5 0 0 1 2.5.5l-.6 1.4A5 5 0 0 0 8 3V1.5ZM12 3.4a6.5 6.5 0 0 1 1.9 2.2l-1.4.7A5 5 0 0 0 11 4.8l1-1.4Zm2.3 4.1a6.5 6.5 0 0 1-.4 2.9l-1.4-.6a5 5 0 0 0 .3-2.3h1.5ZM13 12a6.5 6.5 0 0 1-2.2 1.8l-.7-1.4a5 5 0 0 0 1.6-1.3l1.3.9Zm-4.5 2.4a6.5 6.5 0 0 1-2.9-.4l.6-1.4a5 5 0 0 0 2.3.3v1.5ZM4 13a6.5 6.5 0 0 1-1.9-2.2l1.4-.7a5 5 0 0 0 1.5 1.1L4 13Zm-2.3-4.1a6.5 6.5 0 0 1 .4-2.9l1.4.6a5 5 0 0 0-.3 2.3H1.7ZM3 4a6.5 6.5 0 0 1 2.2-1.8l.7 1.4A5 5 0 0 0 4.3 4.9L3 4Z",
  // two merging arrows
  merged: "M3.5 2a2 2 0 0 1 .75 3.85v4.3a2 2 0 1 1-1.5 0v-4.3A2 2 0 0 1 3.5 2Zm9 6a2 2 0 0 1-1.7 1.98 4.5 4.5 0 0 1-3.8 2.5v-1.5a3 3 0 0 0 2.7-1.5 2 2 0 1 1 2.8-1.48ZM3.5 3.5a.5.5 0 1 0 0 1 .5.5 0 0 0 0-1Zm0 9a.5.5 0 1 0 0 1 .5.5 0 0 0 0-1Zm8-5a.5.5 0 1 0 0 1 .5.5 0 0 0 0-1Z",
  // circle with an x
  closed: "M8 1.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13Zm0 1.5a5 5 0 1 1 0 10A5 5 0 0 1 8 3Zm-2.1 2.9L8 8l2.1-2.1 1.05 1.05L9.05 9.05l2.1 2.1-1.05 1.05L8 10.1l-2.1 2.1-1.05-1.05 2.1-2.1-2.1-2.1L5.9 5.9Z",
  // circle with a clock hand
  unknown: "M8 1.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13Zm0 1.5a5 5 0 1 1 0 10A5 5 0 0 1 8 3Zm-.75 1.75h1.5v3.2l2 1.2-.75 1.3-2.75-1.65v-4.05Z",
  // unverified terminal reuses the closed glyph (ux.md vocabulary)
  unverified: "",
};

/** State glyph with a distinct shape per state, so colour never carries the state alone. */
export function PrStateGlyph({ state, className }: { state: PrRowState; className?: string }) {
  const shape = state === "unverified" ? "closed" : state;
  return (
    <svg className={className} viewBox="0 0 16 16" fill="currentColor" aria-hidden="true" focusable="false" data-glyph={shape}>
      <path d={PATHS[shape]} />
    </svg>
  );
}
