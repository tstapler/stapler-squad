"use client";

/**
 * Chrome for the mobile input layer (Story 1.4.1e shell). TerminalOutput calls this once and renders
 * `fragment` in one slot; later slices (chip, compose bar) extend this hook rather than TerminalOutput.
 * It owns the live regions, the flag-off and paste-flattened hints, the DeferredCommit indicator and
 * the `?inputdebug=1` panel, and lays them out under `chromeBudget`. On desktop and iOS it renders nothing.
 */
import { useEffect, useId, useMemo, useRef, useState, type ComponentProps, type ReactElement } from "react";
import { GHOSTTY_RENDERER_FLAG, MOBILE_INPUT_LAYER_FLAG, preloadFlagValue } from "@/lib/features/flagStore";
import type { Announcer } from "@/lib/terminal/mobileInput/announce";
import { chromeBudget, type ChromeBudget } from "@/lib/terminal/mobileInput/chromeBudget";
import type { Modifiers } from "@/lib/terminal/mobileInput/keymap";
import { shouldAttachMobileInput } from "@/lib/terminal/mobileInput/platform";
import type { OnData } from "@/lib/terminal/mobileInput/types";
import type { MobileInputHost } from "@/lib/terminal/mobileInput/useMobileInput";
import { resolveRenderer } from "@/lib/terminal/rendererPolicy";
import { MinimalInputDiagnostics } from "./MinimalInputDiagnostics";
import { stackItem, touchButton } from "./MobileInputChrome.css";
import { useMobileInputLiveRegions } from "./MobileInputLiveRegions";

export const FLAG_OFF_HINT =
  "Phone keyboard support is off, so typing may not work well. You can turn it on in the feature flags panel.";
export const PASTE_FLATTENED_HINT = "Multi-line paste was joined into one line so it would not run each line.";
export const READ_ONLY_TEXT = "Read-only session";
const PASTE_HINT_MS = 6000;
const READ_ONLY_HINT_INTERVAL_MS = 10_000;

/** TerminalOutput's side of the layer: where bytes go and the sticky modifiers they consume. */
export interface MobileInputSource {
  onData: OnData;
  getModifiers(): Modifiers;
  consumeModifiers(): void;
}

export interface UseMobileInputChromeOptions {
  streamState: string;
  readOnly: boolean;
  source: MobileInputSource;
  /** Height of the extra-keys toolbar, 0 when it is hidden. */
  toolbarPx: number;
  /** Terminal cell height; 0 or absent means unknown, which keeps the layout comfortable. */
  getCellHeightPx?: () => number;
}

export interface MobileInputChromeCallbacks {
  /** Id of the polite region the textarea is described by. */
  describedBy: string;
  onPasteFlattened(): void;
  onDeferredWaiting(waiting: number | null): void;
  /** The layer textarea took focus in a read-only session. */
  onReadOnlyTap(): void;
}

export interface MobileInputChrome {
  /** Null off the Android class, so the slot costs nothing on desktop and iOS. */
  fragment: ReactElement | null;
  callbacks: MobileInputChromeCallbacks;
  budget: ChromeBudget | null;
  announcer: Announcer;
  /** The host the pool hands the renderer; undefined unless the layer is active, so nothing else changes. */
  host: MobileInputHost | undefined;
}

const deferredText = (n: number) => `Typing... (${n} ${n === 1 ? "character" : "characters"} waiting)`;

export function useMobileInputChrome({ streamState, readOnly, source, toolbarPx, getCellHeightPx }: UseMobileInputChromeOptions): MobileInputChrome {
  // Fixed for the page's lifetime, like the renderer choice.
  const [attachable] = useState(() => shouldAttachMobileInput());
  const [flagOn] = useState(() => preloadFlagValue(MOBILE_INPUT_LAYER_FLAG) === true);
  const [renderer] = useState(() =>
    resolveRenderer({
      ghosttyFlag: preloadFlagValue(GHOSTTY_RENDERER_FLAG) === true,
      mobileLayerFlag: flagOn,
      attachable,
    }),
  );
  const viewportHeightPx = useVisualViewportHeight(attachable);
  const regionId = useId();
  const { announcer, regions } = useMobileInputLiveRegions(regionId);

  const [flagHintDismissed, setFlagHintDismissed] = useState(false);
  const [pasteHint, setPasteHint] = useState(false);
  const [waiting, setWaiting] = useState<number | null>(null);
  const pasteTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastReadOnlyHint = useRef<number | null>(null);
  const readOnlySeq = useRef(0);
  const deferredAnnounced = useRef(false);
  const readOnlyRef = useRef(readOnly);
  readOnlyRef.current = readOnly;

  const showFlagOffHint = attachable && !flagOn && !flagHintDismissed;
  useEffect(() => {
    if (!attachable || flagOn) return;
    console.info("[mobileInput] layer is off on this device; typing may not work well");
  }, [attachable, flagOn]);
  useEffect(() => {
    if (showFlagOffHint) announcer.announce("flag-off-hint", FLAG_OFF_HINT, "flag-off");
  }, [showFlagOffHint, announcer]);

  useEffect(() => () => {
    if (pasteTimer.current) clearTimeout(pasteTimer.current);
  }, []);

  const callbacks = useMemo<MobileInputChromeCallbacks>(
    () => ({
      describedBy: regionId,
      onPasteFlattened() {
        if (pasteTimer.current) clearTimeout(pasteTimer.current);
        pasteTimer.current = setTimeout(() => {
          pasteTimer.current = null;
          setPasteHint(false);
        }, PASTE_HINT_MS);
        setPasteHint(true);
        announcer.announce("paste-flattened-hint", PASTE_FLATTENED_HINT, "paste-flattened");
      },
      onDeferredWaiting(n) {
        setWaiting(n);
        if (n !== null) {
          deferredAnnounced.current = true;
          announcer.announce("deferred-commit", deferredText(n), `deferred:${n}`);
        } else if (deferredAnnounced.current) {
          deferredAnnounced.current = false;
          announcer.clear("polite");
        }
      },
      onReadOnlyTap() {
        if (!readOnlyRef.current) return;
        const now = Date.now();
        if (lastReadOnlyHint.current !== null && now - lastReadOnlyHint.current < READ_ONLY_HINT_INTERVAL_MS) return;
        lastReadOnlyHint.current = now;
        announcer.announce("read-only-hint", READ_ONLY_TEXT, `read-only:${++readOnlySeq.current}`);
      },
    }),
    [announcer, regionId],
  );

  const budget = useMemo(
    () =>
      attachable
        ? chromeBudget({
            viewportHeightPx,
            cellHeightPx: getCellHeightPx?.() ?? 0,
            toolbarPx,
            composeOpen: false,
            chipVisible: false,
          })
        : null,
    // getCellHeightPx reads live layout; the viewport height is what changes when the keyboard moves.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [attachable, viewportHeightPx, toolbarPx],
  );

  const sourceRef = useRef(source);
  sourceRef.current = source;
  const layerActive = attachable && flagOn;
  const host = useMemo<MobileInputHost | undefined>(
    () =>
      layerActive
        ? {
            onData: (bytes, seq) => sourceRef.current.onData(bytes, seq),
            getModifiers: () => sourceRef.current.getModifiers(),
            consumeModifiers: () => sourceRef.current.consumeModifiers(),
            isReadOnly: () => readOnlyRef.current,
            onDeferredWaiting: callbacks.onDeferredWaiting,
            onPasteFlattened: callbacks.onPasteFlattened,
            onReadOnlyTap: callbacks.onReadOnlyTap,
            describedBy: callbacks.describedBy,
          }
        : undefined,
    [layerActive, callbacks],
  );

  const fragment = useMemo<ReactElement | null>(
    () =>
      attachable ? (
        <ChromeView
          budget={budget}
          regions={regions}
          flagOffHint={showFlagOffHint}
          onDismissFlagOffHint={() => setFlagHintDismissed(true)}
          pasteHint={pasteHint}
          waiting={waiting}
          diagnosticsHeader={{ renderer, flag: flagOn ? "on" : "off", streamState, keyboardHint: "soft keyboard" }}
        />
      ) : null,
    [attachable, budget, regions, showFlagOffHint, pasteHint, waiting, renderer, flagOn, streamState],
  );

  return { fragment, callbacks, budget, announcer, host };
}

interface ChromeViewProps {
  budget: ChromeBudget | null;
  regions: ReactElement;
  flagOffHint: boolean;
  onDismissFlagOffHint(): void;
  pasteHint: boolean;
  waiting: number | null;
  diagnosticsHeader: ComponentProps<typeof MinimalInputDiagnostics>["header"];
}

function ChromeView({ budget, regions, flagOffHint, onDismissFlagOffHint, pasteHint, waiting, diagnosticsHeader }: ChromeViewProps) {
  // Hints wait, rather than render, when the compose bar had to collapse to keep the terminal usable.
  const hintsQueued = budget?.queueHints ?? false;
  return (
    <div className={stackItem} data-testid="mobile-input-chrome" data-density={budget?.density ?? "comfortable"}>
      {regions}
      {!hintsQueued && flagOffHint && (
        <div className={stackItem} data-testid="mobile-input-flag-off-hint">
          <span>{FLAG_OFF_HINT}</span>
          <button type="button" className={touchButton} onClick={onDismissFlagOffHint} aria-label="Dismiss keyboard support hint">
            {/* analytics-exempt */}
            Dismiss
          </button>
        </div>
      )}
      {!hintsQueued && pasteHint && (
        <div className={stackItem} data-testid="mobile-input-paste-hint">
          {PASTE_FLATTENED_HINT}
        </div>
      )}
      {waiting !== null && (
        <div className={stackItem} data-testid="mobile-input-deferred-indicator">
          {deferredText(waiting)}
        </div>
      )}
      <MinimalInputDiagnostics header={diagnosticsHeader} />
    </div>
  );
}

/** Visual viewport height (null when unavailable), tracked only where the layer's chrome exists. */
function useVisualViewportHeight(enabled: boolean): number | null {
  const [height, setHeight] = useState<number | null>(() => (enabled ? window.visualViewport?.height ?? null : null));
  useEffect(() => {
    const vv = enabled ? window.visualViewport : null;
    if (!vv) return;
    const update = () => setHeight(vv.height);
    vv.addEventListener("resize", update);
    return () => vv.removeEventListener("resize", update);
  }, [enabled]);
  return height;
}
