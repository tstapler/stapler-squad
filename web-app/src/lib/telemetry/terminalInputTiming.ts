/** What the browser knows about one terminal input frame at the moment it is handed to the socket. */
export interface InputSendSample {
  /** Time from the keystroke entering the send queue to the socket send. */
  queuedMs: number;
  /** Bytes the socket had not yet put on the wire when this frame was sent. */
  bufferedBytes: number;
}

export interface InputSendTimer {
  /** A terminal input message entered the send queue. */
  markQueued(): void;
  /** The oldest queued input message was passed to the socket. */
  markSent(bufferedBytes: number): void;
  /** Forget every queued message: the queue they sat in was closed and will never send them. */
  reset(): void;
  subscribe(listener: (s: InputSendSample) => void): () => void;
  pending(): number;
}

/** Bounds the queue if sends never happen (a socket that never opens). */
const MAX_PENDING = 256;

/**
 * Pairs each queued input message with its send, FIFO. The transport iterates messages in
 * order, so the nth send belongs to the nth queued input; frames with nothing queued (resize,
 * credit, handshake) are not input and are ignored.
 */
export function createInputSendTimer(now: () => number = () => performance.now()): InputSendTimer {
  const queued: number[] = [];
  const listeners = new Set<(s: InputSendSample) => void>();
  return {
    markQueued() {
      if (queued.length >= MAX_PENDING) queued.shift();
      queued.push(now());
    },
    markSent(bufferedBytes) {
      const at = queued.shift();
      if (at === undefined) return;
      const sample = { queuedMs: Math.max(0, now() - at), bufferedBytes };
      listeners.forEach((l) => l(sample));
    },
    reset() {
      queued.length = 0;
    },
    subscribe(listener) {
      listeners.add(listener);
      return () => void listeners.delete(listener);
    },
    pending: () => queued.length,
  };
}

export interface KeyedInputSendTimers {
  markQueued(key: string): void;
  markSent(key: string, bufferedBytes: number): void;
  reset(key: string): void;
  subscribe(key: string, listener: (s: InputSendSample) => void): () => void;
}

/**
 * One FIFO per terminal key (the session id), so a reset or send on one terminal never pairs with
 * or wipes another's queue. An entry is dropped once it has no listeners and nothing queued.
 */
export function createKeyedInputSendTimers(now?: () => number): KeyedInputSendTimers {
  const timers = new Map<string, { timer: InputSendTimer; listeners: number }>();
  const entry = (key: string) => {
    let e = timers.get(key);
    if (!e) {
      e = { timer: createInputSendTimer(now), listeners: 0 };
      timers.set(key, e);
    }
    return e;
  };
  const prune = (key: string) => {
    const e = timers.get(key);
    if (e && e.listeners === 0 && e.timer.pending() === 0) timers.delete(key);
  };
  return {
    markQueued: (key) => entry(key).timer.markQueued(),
    markSent(key, bufferedBytes) {
      timers.get(key)?.timer.markSent(bufferedBytes);
      prune(key);
    },
    reset(key) {
      timers.get(key)?.timer.reset();
      prune(key);
    },
    subscribe(key, listener) {
      const e = entry(key);
      e.listeners++;
      const off = e.timer.subscribe(listener);
      return () => {
        off();
        e.listeners--;
        prune(key);
      };
    },
  };
}

/** Shared by the terminal hook (queue side) and the WebSocket transport (send side). */
export const terminalInputTimer = createKeyedInputSendTimers();
