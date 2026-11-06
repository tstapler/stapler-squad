/**
 * One body MutationObserver + ResizeObserver + window resize listener shared by
 * every layout subscriber. A burst of triggers schedules a single frame, and each
 * subscriber's `measure` runs at most once in it (so N mounted views cost N rect
 * reads per frame, not N observers each reading on every body mutation).
 * Nothing is scheduled unless a trigger fires, so a stable layout costs 0.
 */
export interface BodyMeasureSubscriber {
  /** Elements whose own resize should trigger a measure (body is always observed). */
  elements: readonly Element[];
  measure: () => void;
}

export interface SharedBodyMeasurer {
  /** Measures once synchronously, then on every coalesced trigger. Returns unsubscribe. */
  subscribe(subscriber: BodyMeasureSubscriber): () => void;
  subscriberCount(): number;
}

export function createSharedBodyMeasurer(): SharedBodyMeasurer {
  const subscribers = new Set<BodyMeasureSubscriber>();
  const observedRefs = new Map<Element, number>();
  let frame = 0;
  let resizeObserver: ResizeObserver | null = null;
  let mutationObserver: MutationObserver | null = null;

  const runFrame = () => {
    frame = 0;
    subscribers.forEach((s) => s.measure());
  };
  const schedule = () => {
    if (frame === 0) frame = requestAnimationFrame(runFrame);
  };

  const connect = () => {
    resizeObserver = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(schedule);
    resizeObserver?.observe(document.body);
    observedRefs.set(document.body, 1); // permanent ref so a subscriber listing body can't unobserve it
    mutationObserver = new MutationObserver(schedule);
    mutationObserver.observe(document.body, { childList: true, subtree: true });
    window.addEventListener("resize", schedule);
  };
  const disconnect = () => {
    if (frame !== 0) cancelAnimationFrame(frame);
    frame = 0;
    resizeObserver?.disconnect();
    observedRefs.clear();
    mutationObserver?.disconnect();
    resizeObserver = null;
    mutationObserver = null;
    window.removeEventListener("resize", schedule);
  };

  return {
    subscribe(subscriber) {
      if (subscribers.size === 0) connect();
      subscribers.add(subscriber);
      subscriber.elements.forEach((el) => {
        const n = observedRefs.get(el) ?? 0;
        observedRefs.set(el, n + 1);
        if (n === 0) resizeObserver?.observe(el);
      });
      subscriber.measure();
      return () => {
        if (!subscribers.delete(subscriber)) return;
        subscriber.elements.forEach((el) => {
          const n = (observedRefs.get(el) ?? 1) - 1;
          if (n > 0) {
            observedRefs.set(el, n);
          } else {
            observedRefs.delete(el);
            resizeObserver?.unobserve(el);
          }
        });
        if (subscribers.size === 0) disconnect();
      };
    },
    subscriberCount: () => subscribers.size,
  };
}

export const sharedBodyMeasurer = createSharedBodyMeasurer();
