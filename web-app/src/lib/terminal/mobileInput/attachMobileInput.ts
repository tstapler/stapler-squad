/**
 * DOM adapter for the mobile input layer: a hidden textarea, mounted as a sibling of (never inside)
 * the renderer's element, feeds real DOM events through the pure reducer into `onData`.
 * Typed text never reaches console, `onAnomaly` or diagnostics: only anomaly classes and sequence ids do.
 */
import { DeferredIndicator } from "./deferredIndicator";
import { InputEngine } from "./inputEngine";
import { createInputElement } from "./inputElement";
import { TextareaSource } from "./inputSource";
import type { Modifiers } from "./keymap";
import { supportsBeforeInput } from "./normalize";
import { physicalKeyBytes } from "./physicalKeys";
import { DEFAULT_POLICY, DEFERRED_INDICATOR_MS, resetValue, WATCHDOG_TICK_MS, type MobileInputPolicy } from "./policy";
import { shouldAttachMobileInput } from "./platform";
import { bindTapTarget } from "./tapTarget";
import type {
  DeliveryReport,
  ImeAnomaly,
  InvalidateReason,
  MobileInputClock,
  MobileInputDiagnostic,
  MobileInputEvent,
  OnData,
} from "./types";

export interface AttachOptions {
  onData: OnData;
  getModifiers(): Modifiers;
  /** Clears the armed modifiers; called once per character that consumed them. */
  consumeModifiers(): void;
  isReadOnly(): boolean;
  /** Anomaly class only; never carries typed data. */
  onAnomaly(kind: ImeAnomaly): void;
  /** Content-free trace for the diagnostics ring. */
  onDiagnostic?(entry: MobileInputDiagnostic): void;
  /** Subscribes the layer to queue discards (PR-B); returns the unsubscribe function. */
  onQueueDiscard?(listener: (report: DeliveryReport) => void): () => void;
  /** Grapheme count held back by a deferred commit once it has waited 300 ms; null when cleared. */
  onDeferredWaiting?(waiting: number | null): void;
  /** DECSET 2004 state of the active renderer; defaults to off. */
  bracketedPasteEnabled?(): boolean;
  policy?: MobileInputPolicy;
  clock?: MobileInputClock;
  /** The element handed to the renderer's `open()`. The container must be outside it. */
  rendererElement?: HTMLElement;
  /** Id of the polite status region the textarea is described by. */
  describedBy?: string;
}

export interface MobileInputController {
  readonly element: HTMLTextAreaElement;
  /** True from attach until dispose. */
  readonly active: boolean;
  /** True while the layer textarea is `document.activeElement`. */
  readonly focused: boolean;
  /** Focuses the textarea synchronously: call it inside the user gesture. */
  focus(): void;
  blur(): void;
  /** Tap-to-focus on the visible terminal surface; returns the unbind function. */
  bindTapTarget(surface: HTMLElement): () => void;
  /** The host says the diff base can no longer be trusted (session switch, reconnect, ...). */
  invalidate(reason: InvalidateReason): void;
  reportQueueDiscard(report: DeliveryReport): void;
  /** Re-reads `isReadOnly()` into the textarea's `inputmode`. */
  syncReadOnly(): void;
  dispose(): void;
}

const defaultClock = (): MobileInputClock => ({
  now: () => performance.now(),
  setTimeout: (fn, ms) => window.setTimeout(fn, ms),
  clearTimeout: (id) => window.clearTimeout(id),
});

const isStaleKind = (e: MobileInputEvent): boolean =>
  e.kind === "compositionupdate" ||
  e.kind === "input" ||
  (e.kind === "beforeinput" && (e.inputType === "insertCompositionText" || e.inputType === "insertReplacementText"));

/** Returns null (zero listeners, zero nodes) off the Android touch class or without `beforeinput`. */
export function attachMobileInput(container: HTMLElement, opts: AttachOptions): MobileInputController | null {
  const doc = container.ownerDocument;
  if (!shouldAttachMobileInput() || !supportsBeforeInput(doc)) return null;
  if (opts.rendererElement?.contains(container)) {
    // Inside the renderer's element the textarea's beforeinput would bubble into its blanket preventDefault.
    console.error("[mobileInput] container must be outside the renderer element");
    return null;
  }

  const clock = opts.clock ?? defaultClock();
  const policy = opts.policy ?? DEFAULT_POLICY;
  const source = new TextareaSource(createInputElement(doc, { describedBy: opts.describedBy }), clock);
  const el = source.element;

  let disposed = false;
  let browserComposing = false;
  // After a mid-composition reset the browser keeps sending updates for text already flushed.
  let ignoreStale = false;
  let watchdog: number | null = null;
  let readOnlyApplied: boolean | null = null;
  const loggedKinds = new Set<ImeAnomaly>();
  const tapUnbinds = new Set<() => void>();

  const report = (kind: ImeAnomaly) => {
    try {
      opts.onAnomaly(kind);
      opts.onDiagnostic?.({ event: "anomaly", kind });
      if (kind === "stuck_composition") opts.onDiagnostic?.({ event: "watchdog" });
    } catch {
      /* a failing observer must not break typing */
    }
    if (kind !== "duplicate_suppressed" && !loggedKinds.has(kind)) {
      loggedKinds.add(kind);
      console.error(`[mobileInput] anomaly: ${kind}`);
    }
  };

  const engine = new InputEngine({
    policy,
    deps: {
      getModifiers: opts.getModifiers,
      consumeModifiers: opts.consumeModifiers,
      bracketedPasteEnabled: opts.bracketedPasteEnabled ?? (() => false),
    },
    onData: opts.onData,
    isReadOnly: opts.isReadOnly,
    onAnomaly: report,
    onDiagnostic: opts.onDiagnostic,
  });

  const indicator = new DeferredIndicator(clock, DEFERRED_INDICATOR_MS, (n) => opts.onDeferredWaiting?.(n));

  const syncReadOnly = () => {
    const ro = opts.isReadOnly();
    if (ro === readOnlyApplied) return;
    readOnlyApplied = ro;
    if (ro) el.setAttribute("inputmode", "none");
    else el.removeAttribute("inputmode");
  };

  const syncTimers = () => {
    const composing = engine.state.kind === "composing";
    if (composing && watchdog === null) watchdog = clock.setTimeout(onWatchdog, WATCHDOG_TICK_MS);
    if (!composing && watchdog !== null) {
      clock.clearTimeout(watchdog);
      watchdog = null;
    }
    indicator.update(engine.deferredWaiting);
  };

  function onWatchdog() {
    watchdog = null;
    if (disposed) return;
    run({ kind: "tick", t: clock.now() }, null);
  }

  /** Reduce, execute, then re-sync timers. Exceptions never escape into the browser's event loop. */
  function run(event: MobileInputEvent, raw: Event | null): boolean {
    let prevent = false;
    try {
      const outcome = engine.dispatch(event);
      prevent = outcome.preventDefault;
      if (raw instanceof KeyboardEvent && !prevent && event.kind === "keydown") prevent = mapPhysicalKey(raw);
      if (outcome.resetTextarea) {
        if (browserComposing && event.kind !== "compositionend") ignoreStale = true;
        source.reset(resetValue(policy));
      }
    } catch {
      report("unrecognized_sequence");
    }
    syncTimers();
    return prevent;
  }

  function mapPhysicalKey(e: KeyboardEvent): boolean {
    if (e.keyCode === 229 || e.isComposing || engine.state.kind === "composing") return false;
    const bytes = physicalKeyBytes(e);
    if (bytes === null) return false;
    engine.emitRaw(bytes);
    return true;
  }

  const handle = (event: MobileInputEvent, raw: Event | null): boolean => {
    if (disposed) return false;
    syncReadOnly();
    if (event.kind === "compositionstart") {
      browserComposing = true;
      ignoreStale = false;
    } else if (event.kind === "compositionend") {
      browserComposing = false;
      ignoreStale = false;
    } else if (event.kind === "blur") {
      browserComposing = false;
    } else if (ignoreStale && isStaleKind(event)) {
      return false;
    }
    return run(event, raw);
  };

  syncReadOnly();
  container.appendChild(el);
  source.start(handle);
  const unsubscribeQueue = opts.onQueueDiscard?.((r) => controller.reportQueueDiscard(r));

  const controller: MobileInputController = {
    element: el,
    get active() {
      return !disposed;
    },
    get focused() {
      return !disposed && doc.activeElement === el;
    },
    focus() {
      if (!disposed) el.focus({ preventScroll: true });
    },
    blur() {
      if (!disposed) el.blur();
    },
    bindTapTarget(surface) {
      if (disposed || surface === el || el.contains(surface)) return () => undefined;
      const unbind = bindTapTarget(surface, () => controller.focus(), clock);
      tapUnbinds.add(unbind);
      return () => {
        unbind();
        tapUnbinds.delete(unbind);
      };
    },
    invalidate(reason) {
      if (disposed) return;
      engine.invalidate(reason);
      syncTimers();
    },
    reportQueueDiscard(r) {
      if (disposed) return;
      engine.reportQueueDiscard(r);
      syncTimers();
    },
    syncReadOnly,
    dispose() {
      if (disposed) return;
      disposed = true;
      source.dispose();
      tapUnbinds.forEach((f) => f());
      tapUnbinds.clear();
      unsubscribeQueue?.();
      if (watchdog !== null) clock.clearTimeout(watchdog);
      indicator.dispose();
      el.remove();
    },
  };
  return controller;
}
