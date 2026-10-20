// THROWAWAY (Task 0.1.4c): the surface the Playwright spec and the owner page read through window.__spike.api.
export interface SpikeApi {
  lineHeight(): number;
  topAbs(): number;
  topLineClientTop(): number;
  scrollToAbs(n: number): void;
  loadAboveViewport(linesAbove: number): { ok: boolean; before: { top: number; y: number } };
  cycles(n: number): void;
  type(text: string): void;
  placeCaretInViewport(): void;
  undoDepth(): number;
  stats(): { loadedChars: number; segs: number; heap: number; dirtySegs: number };
  composeProbe(): { dispatchedWhileComposing: boolean };
}

export function spikeApi(api: SpikeApi): void {
  (window as unknown as { __spike: { api: SpikeApi } }).__spike = { api };
}

/** Client top of the first rendered line that intersects the scroller: a DOM fact, not a CodeMirror estimate. */
export function domTopLineClientTop(view: { scrollDOM: HTMLElement }): number {
  const r = view.scrollDOM.getBoundingClientRect();
  for (const el of Array.from(view.scrollDOM.querySelectorAll(".cm-line"))) {
    const b = el.getBoundingClientRect();
    if (b.bottom > r.top) return b.top;
  }
  return NaN;
}
