"use client";

/** What both renderers do to host the layer, in one place: the sibling container, the stable host and the inert renderer textarea. */
import { useEffect, useRef, type RefObject } from "react";
import { useMobileInputLayerActive } from "@/lib/terminal/useMobileInputLayerActive";
import { suppressHelperTextarea } from "./rendererInputGuards";
import type { SendResult } from "./types";
import {
  useMobileInput,
  type MobileInputHandle,
  type MobileInputHost,
  type MobileInputRendererKind,
  type MobileInputRendererProps,
} from "./useMobileInput";

/** Stand-in host while the layer is off; never reached because the hook is disabled. */
const INERT_HOST: MobileInputHost = {
  onData: () => "dropped",
  getModifiers: () => ({ ctrl: false, alt: false, shift: false }),
  consumeModifiers: () => undefined,
  isReadOnly: () => true,
};

export interface RendererMobileInputArgs {
  renderer: MobileInputRendererKind;
  /** Set by the pool only when the consumer has the layer active. */
  mobileInput: MobileInputRendererProps | undefined;
  /** The renderer's latest `onData` prop: layer bytes go here directly, bypassing its own mouse-report filter. */
  onDataRef: RefObject<((data: string) => SendResult | void) | undefined>;
  /** The element handed to the renderer's open(); also the visible tap target. */
  containerRef: RefObject<HTMLElement | null>;
  getHelperTextarea(): HTMLTextAreaElement | null | undefined;
  bracketedPasteEnabled(): boolean;
  /** Flips true once an asynchronously opened renderer has created its textarea. */
  rendererReady?: boolean;
}

export interface RendererMobileInput {
  handle: MobileInputHandle;
  layerActive: boolean;
  /** Render `<div ref={layerContainerRef} data-mobile-input-host />` beside the renderer's element when `layerActive`. */
  layerContainerRef: RefObject<HTMLDivElement | null>;
}

export function useRendererMobileInput(args: RendererMobileInputArgs): RendererMobileInput {
  const { renderer, mobileInput, onDataRef, containerRef, rendererReady } = args;
  const layerActive = useMobileInputLayerActive() && mobileInput !== undefined;
  const layerContainerRef = useRef<HTMLDivElement>(null);
  const argsRef = useRef(args);
  argsRef.current = args;
  const handleRef = useRef<MobileInputHandle | null>(null);
  const host = mobileInput ? { ...mobileInput.host, onData: (bytes: Parameters<MobileInputHost["onData"]>[0]) => onDataRef.current?.(bytes) ?? "dropped" } : INERT_HOST;
  const visible = layerActive && (mobileInput?.visible ?? false);

  const handle = useMobileInput({
    enabled: layerActive,
    visible: mobileInput?.visible ?? false,
    sessionId: mobileInput?.sessionId ?? "",
    isConnected: mobileInput?.isConnected ?? false,
    reuseKey: mobileInput?.reuseKey,
    renderer,
    getMount: () => ({ container: layerContainerRef.current, rendererElement: containerRef.current, surface: containerRef.current }),
    host,
    bracketedPasteEnabled: () => argsRef.current.bracketedPasteEnabled(),
    onAttached: () => suppressHelperTextarea(argsRef.current.getHelperTextarea()),
  });

  handleRef.current = handle;

  // An asynchronously opened renderer may create its own textarea after the layer attached.
  // Only while a controller exists: an unattached layer must leave the renderer's own input working.
  useEffect(() => {
    if (visible && rendererReady && handleRef.current?.controller != null) return suppressHelperTextarea(argsRef.current.getHelperTextarea());
  }, [visible, rendererReady]);

  return { handle, layerActive, layerContainerRef };
}
