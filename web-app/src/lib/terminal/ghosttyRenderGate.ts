/**
 * Pauses ghostty-web's per-terminal requestAnimationFrame render loop while a terminal cannot be seen.
 *
 * ghostty-web@0.4.0 re-arms `requestAnimationFrame` forever from `startRenderLoop()`, with no hidden or
 * off-screen check. With the pool keeping a dozen terminals alive (parked off-screen in the graveyard), a
 * Chrome trace showed ~10 render callbacks per frame (4856 callbacks / 485 frames) and about 10% of
 * main-thread time spent there. The WASM keeps its dirty-row flags until a render consumes them, so
 * pausing loses nothing: the first render after resume repaints whatever changed.
 *
 * `animationFrameId` and `startRenderLoop` are private in ghostty-web's typings; the narrow cast below is
 * the only place that touches them, and a version bump that renames them degrades to "never pauses".
 */

interface PausableGhosttyTerminal {
  animationFrameId?: number;
  isOpen?: boolean;
  isDisposed?: boolean;
  startRenderLoop?: () => void;
}

type ObserverFactory = typeof IntersectionObserver | undefined;

/** Cancels the pending frame so the loop stops re-arming. Returns true if a running loop was paused. */
export function pauseRenderLoop(term: object): boolean {
  const t = term as PausableGhosttyTerminal;
  if (t.animationFrameId === undefined) return false;
  cancelAnimationFrame(t.animationFrameId);
  t.animationFrameId = undefined;
  return true;
}

/** Restarts a loop paused by `pauseRenderLoop`; its first render runs synchronously. No-op if already running or disposed. */
export function resumeRenderLoop(term: object): void {
  const t = term as PausableGhosttyTerminal;
  if (t.animationFrameId !== undefined || t.isDisposed || t.isOpen === false) return;
  t.startRenderLoop?.();
}

/**
 * Runs the terminal's render loop only while `container` intersects the viewport and the document is visible.
 * Returns a disposer that detaches the observers and leaves the loop running.
 */
export function attachRenderGate(
  term: object,
  container: Element,
  doc: Document = document,
  Observer: ObserverFactory = typeof IntersectionObserver === "undefined" ? undefined : IntersectionObserver,
): () => void {
  if (!Observer) return () => {};
  let intersecting = true;
  let paused = false;

  const apply = () => {
    const shouldRun = intersecting && doc.visibilityState !== "hidden";
    if (shouldRun && paused) {
      paused = false;
      resumeRenderLoop(term);
    } else if (!shouldRun && !paused) {
      paused = pauseRenderLoop(term);
    }
  };

  const observer = new Observer((entries) => {
    const last = entries[entries.length - 1];
    if (!last) return;
    intersecting = last.isIntersecting;
    apply();
  });
  observer.observe(container);
  doc.addEventListener("visibilitychange", apply);

  return () => {
    observer.disconnect();
    doc.removeEventListener("visibilitychange", apply);
    if (paused) resumeRenderLoop(term);
  };
}
