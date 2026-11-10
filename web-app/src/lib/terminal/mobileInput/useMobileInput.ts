"use client";

/**
 * The one hook both renderers use to host the mobile input layer (Story 1.4.1). It attaches while the
 * terminal is visible/docked (not on focus, so the tap target exists before the first tap), hands focus to
 * a single owner among docked terminals, and tells the layer when its diff base can no longer be trusted.
 */
import { useCallback, useEffect, useRef } from "react";
import { attachMobileInput, defaultClock, type MobileInputController } from "./attachMobileInput";
import { inputDiagnostics } from "./diagnostics";
import type { Modifiers } from "./keymap";
import { HIDDEN_INVALIDATE_MS } from "./policy";
import { createMobileInputTelemetry } from "./telemetry";
import type { ImeAnomaly, MobileInputClock, MobileInputDiagnostic, OnData } from "./types";

export type MobileInputRendererKind = "xterm" | "ghostty";

/** What the owning TerminalOutput supplies; the renderer reads it through a stable delegate. */
export interface MobileInputHost {
  onData: OnData;
  getModifiers(): Modifiers;
  consumeModifiers(): void;
  isReadOnly(): boolean;
  onAnomaly?(kind: ImeAnomaly): void;
  onDiagnostic?(entry: MobileInputDiagnostic): void;
  onDeferredWaiting?(waiting: number | null): void;
  onPasteFlattened?(): void;
  /** The layer textarea took focus while the session is read-only. */
  onReadOnlyTap?(): void;
  /** Id of the polite live region the textarea is described by. */
  describedBy?: string;
}

/** What the pool hands a renderer so it can host the layer; absent when the layer is not active. */
export interface MobileInputRendererProps {
  /** Stable delegate to the owning consumer (TerminalOutput). */
  host: MobileInputHost;
  sessionId: string;
  isConnected: boolean;
  /** Docked and visible. */
  visible: boolean;
  /** Changes when the pooled renderer is handed to another consumer. */
  reuseKey: number;
}

export interface MobileInputMount {
  /** Parent for the layer textarea: a sibling of `rendererElement`, never inside it. */
  container: HTMLElement | null;
  /** The element handed to the renderer's `open()`. */
  rendererElement: HTMLElement | null;
  /** The visible terminal surface that takes the tap. */
  surface: HTMLElement | null;
}

export interface UseMobileInputOptions {
  /** Layer flag AND the Android-class gate (`useMobileInputLayerActive`). */
  enabled: boolean;
  /** Visible/docked. A hidden or undocked terminal disposes its layer. */
  visible: boolean;
  sessionId: string;
  isConnected: boolean;
  /** Changes when the pooled renderer is handed to another consumer. */
  reuseKey?: unknown;
  renderer: MobileInputRendererKind;
  getMount(): MobileInputMount;
  host: MobileInputHost;
  bracketedPasteEnabled?: () => boolean;
  /** Renderer-specific setup after attach (inert renderer textarea); returns its undo. */
  onAttached?(controller: MobileInputController): (() => void) | void;
  clock?: MobileInputClock;
  /** Wall-clock source for the hidden-too-long bound. */
  now?: () => number;
}

export interface MobileInputHandle {
  /** Focuses the layer textarea; false when no layer is attached, so the caller falls back to the renderer. */
  focus(): boolean;
  blur(): boolean;
  readonly controller: MobileInputController | null;
}

/** Docked layers: one owns focus, the rest keep `inputmode="none"` so only the owner raises the keyboard. */
const group = new Set<MobileInputController>();

function claimFocus(owner: MobileInputController): void {
  group.forEach((c) => c.setFocusOwner(c === owner));
}

function registerInGroup(controller: MobileInputController): () => void {
  group.add(controller);
  const el = controller.element;
  const onFocusIn = () => claimFocus(controller);
  const onFocusOut = (e: FocusEvent) => {
    const next = e.relatedTarget;
    const staysInLayer = next instanceof Node && [...group].some((c) => c.element === next);
    if (!staysInLayer) group.forEach((c) => c.setFocusOwner(true));
  };
  el.addEventListener("focusin", onFocusIn);
  el.addEventListener("focusout", onFocusOut);
  return () => {
    el.removeEventListener("focusin", onFocusIn);
    el.removeEventListener("focusout", onFocusOut);
    group.delete(controller);
    group.forEach((c) => c.setFocusOwner(true));
  };
}

export function useMobileInput(options: UseMobileInputOptions): MobileInputHandle {
  const optsRef = useRef(options);
  optsRef.current = options;
  const controllerRef = useRef<MobileInputController | null>(null);
  const prev = useRef({ sessionId: options.sessionId, isConnected: options.isConnected, reuseKey: options.reuseKey });

  const { enabled, visible } = options;
  useEffect(() => {
    if (!enabled || !visible) return;
    const live = () => optsRef.current;
    const mount = live().getMount();
    if (!mount.container) return;
    const now = () => (live().now ?? Date.now)();
    const clock = live().clock ?? defaultClock();
    const telemetry = createMobileInputTelemetry({ renderer: live().renderer, clock });
    const controller = attachMobileInput(mount.container, {
      onData: (bytes, seq) => {
        inputDiagnostics.pushInput("emit", bytes, now());
        return live().host.onData(bytes, seq);
      },
      getModifiers: () => live().host.getModifiers(),
      consumeModifiers: () => live().host.consumeModifiers(),
      isReadOnly: () => live().host.isReadOnly(),
      onAnomaly: (kind) => {
        telemetry.onAnomaly(kind);
        live().host.onAnomaly?.(kind);
      },
      onDiagnostic: (entry) => {
        telemetry.onDiagnostic(entry);
        live().host.onDiagnostic?.(entry);
      },
      onDeferredWaiting: (n) => live().host.onDeferredWaiting?.(n),
      onPasteFlattened: () => live().host.onPasteFlattened?.(),
      bracketedPasteEnabled: () => live().bracketedPasteEnabled?.() ?? false,
      onWillFocus: () => {
        if (controllerRef.current) claimFocus(controllerRef.current);
      },
      clock,
      rendererElement: mount.rendererElement ?? undefined,
      describedBy: live().host.describedBy,
    });
    if (!controller) return;
    controllerRef.current = controller;
    // A fresh layer has nothing to invalidate: the boundary effect below compares against this baseline.
    prev.current = { sessionId: live().sessionId, isConnected: live().isConnected, reuseKey: live().reuseKey };
    const unbindTap = mount.surface ? controller.bindTapTarget(mount.surface) : () => undefined;
    const unregister = registerInGroup(controller);
    const undoRenderer = live().onAttached?.(controller);
    const onFocus = () => {
      if (live().host.isReadOnly()) live().host.onReadOnlyTap?.();
    };
    controller.element.addEventListener("focus", onFocus);

    let hiddenAt: number | null = null;
    const onVisibility = () => {
      if (document.visibilityState === "hidden") {
        hiddenAt = now();
        return;
      }
      if (hiddenAt !== null && now() - hiddenAt > HIDDEN_INVALIDATE_MS) controller.invalidate("visibility");
      hiddenAt = null;
    };
    document.addEventListener("visibilitychange", onVisibility);

    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      controller.element.removeEventListener("focus", onFocus);
      undoRenderer?.();
      unregister();
      unbindTap();
      controller.dispose();
      telemetry.dispose();
      controllerRef.current = null;
    };
  }, [enabled, visible]);

  // Boundary triggers: each one means the text the layer sent may no longer be where it thinks.
  useEffect(() => {
    const was = prev.current;
    prev.current = { sessionId: options.sessionId, isConnected: options.isConnected, reuseKey: options.reuseKey };
    const c = controllerRef.current;
    if (!c) return;
    if (was.sessionId !== options.sessionId) c.invalidate("session");
    if (was.isConnected !== options.isConnected) c.invalidate("transport");
    if (!Object.is(was.reuseKey, options.reuseKey)) c.invalidate("pool_reuse");
  }, [options.sessionId, options.isConnected, options.reuseKey]);

  return {
    focus: useCallback(() => {
      const c = controllerRef.current;
      if (!c) return false;
      c.focus();
      return true;
    }, []),
    blur: useCallback(() => {
      const c = controllerRef.current;
      if (!c) return false;
      c.blur();
      return true;
    }, []),
    get controller() {
      return controllerRef.current;
    },
  };
}
