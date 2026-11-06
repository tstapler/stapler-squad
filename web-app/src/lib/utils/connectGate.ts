/**
 * Process-wide limit on terminal stream connects in flight (open until the first message), so
 * N panes re-attaching after a mux drop or server restart do not all hit the server at once.
 * Foreground panes jump the queue.
 */
export const CONNECT_GATE_WIDTH = 4;

export type ReleaseConnectSlot = () => void;

interface Waiter {
  grant: (release: ReleaseConnectSlot) => void;
}

let active = 0;
const priorityQueue: Waiter[] = [];
const normalQueue: Waiter[] = [];

function makeRelease(): ReleaseConnectSlot {
  let released = false;
  return () => {
    if (released) return;
    released = true;
    active--;
    drain();
  };
}

function drain(): void {
  while (active < CONNECT_GATE_WIDTH) {
    const next = priorityQueue.shift() ?? normalQueue.shift();
    if (!next) return;
    active++;
    next.grant(makeRelease());
  }
}

/** Takes a slot immediately when one is free, otherwise null. */
export function tryAcquireConnectSlot(): ReleaseConnectSlot | null {
  if (active >= CONNECT_GATE_WIDTH) return null;
  active++;
  return makeRelease();
}

export interface QueuedConnectSlot {
  /** Resolves with the release function, or null if cancelled first. */
  promise: Promise<ReleaseConnectSlot | null>;
  cancel: () => void;
}

/** Queues for a slot; call after tryAcquireConnectSlot returned null. */
export function queueForConnectSlot(priority: boolean): QueuedConnectSlot {
  const queue = priority ? priorityQueue : normalQueue;
  let waiter: Waiter | undefined;
  let settle: (release: ReleaseConnectSlot | null) => void = () => {};
  const promise = new Promise<ReleaseConnectSlot | null>((resolve) => {
    settle = resolve;
    waiter = { grant: resolve };
  });
  if (waiter) queue.push(waiter);
  const cancel = () => {
    const i = queue.indexOf(waiter as Waiter);
    if (i !== -1) {
      queue.splice(i, 1);
      settle(null);
    }
  };
  return { promise, cancel };
}

/** Test seam: forget every slot and waiter. */
export function resetConnectGateForTests(): void {
  active = 0;
  priorityQueue.length = 0;
  normalQueue.length = 0;
}
