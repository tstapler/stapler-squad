"use client";

import { createContext, useContext, useLayoutEffect, useMemo, useRef, useState } from "react";
import type { ReactNode, RefObject } from "react";
import { createPortal } from "react-dom";
import { WindowKeepAliveHost, type WindowRenderContext } from "@/components/pane/WindowKeepAliveHost";
import { usePathname } from "@/lib/nav";
import type { PaneAction } from "@/lib/pane/paneTypes";
import type { WindowFocusMemory } from "./useWindowFocusMemory";
import { useWindowsController, type WindowsController } from "./useWindowsController";
import type { NamedWindow, WindowId } from "./windowTypes";

const HOME_PATH = "/";

/** What the Sessions page publishes each commit so the host can keep rendering after the page unmounts. */
export interface WindowHostInputs {
  windows: NamedWindow[];
  currentWindowId: WindowId;
  focusMemory: WindowFocusMemory;
  panelRef?: RefObject<HTMLDivElement | null>;
  externalAssign: unknown;
  dispatchPane: (windowId: WindowId, action: PaneAction) => void;
  renderWindow: (window: NamedWindow, ctx: WindowRenderContext<unknown>) => ReactNode;
  renderConnecting?: (window: NamedWindow, ctx: { active: boolean }) => ReactNode;
  /** Re-establishes the page-owned context providers the portaled windows read from. */
  wrap: (host: ReactNode) => ReactNode;
}

interface WindowsContextValue {
  controller: WindowsController;
  publish: (inputs: WindowHostInputs) => void;
  /** Long-lived element the host portals into; the page parks it in its own tree while mounted. */
  slotElement: HTMLElement | null;
}

const WindowsContext = createContext<WindowsContextValue | null>(null);

/** Null outside a `WindowsProvider` (isolated page tests); the page then uses its own controller. */
export function useWindowsContext(): WindowsContextValue | null {
  return useContext(WindowsContext);
}

const noopDispatch = () => {};

/**
 * Layout-level owner of the window controller and the keep-alive host, so retained windows outlive the
 * Sessions page across `/` -> `/backlog` -> `/`. Only mounts the host when `window_keep_alive` is on.
 */
export function WindowsProvider({ children }: { children: ReactNode }) {
  const routeActive = usePathname() === HOME_PATH;
  const [published, setPublished] = useState<WindowHostInputs | null>(null);
  const [slotElement] = useState<HTMLElement | null>(() => {
    if (typeof document === "undefined") return null;
    const el = document.createElement("div");
    el.style.display = "contents";
    el.setAttribute("data-testid", "window-host-slot");
    return el;
  });

  const { keepAlive, paneDispatchFor, detailRefFor, activeDetailRef, focusActiveDetail, syncCurrentWindow } =
    useWindowsController({
      currentWindowId: published?.currentWindowId ?? "",
      dispatchPane: published?.dispatchPane ?? noopDispatch,
      routeActive,
    });
  // Stable across published updates: the page reads this context and must not re-render per publish.
  const controller = useMemo<WindowsController>(
    () => ({ keepAlive, paneDispatchFor, detailRefFor, activeDetailRef, focusActiveDetail, syncCurrentWindow }),
    [keepAlive, paneDispatchFor, detailRefFor, activeDetailRef, focusActiveDetail, syncCurrentWindow],
  );
  const value = useMemo<WindowsContextValue>(
    () => ({ controller, publish: setPublished, slotElement }),
    [controller, slotElement],
  );

  return (
    <WindowsContext.Provider value={value}>
      {children}
      {keepAlive && published && slotElement
        ? createPortal(
            published.wrap(
              <WindowKeepAliveHost
                windows={published.windows}
                currentWindowId={published.currentWindowId}
                keepAlive
                routeActive={routeActive}
                focusMemory={published.focusMemory}
                panelRef={published.panelRef}
                externalAssign={published.externalAssign}
                renderWindow={published.renderWindow}
                renderConnecting={published.renderConnecting}
              />,
            ),
            slotElement,
          )
        : null}
    </WindowsContext.Provider>
  );
}

/**
 * Rendered by the page where the windows belong. Parks the provider's slot element here (same DOM
 * position and geometry as an inline host) and publishes the page's current inputs.
 */
export function WindowHostSlot({ inputs }: { inputs: WindowHostInputs }) {
  const ctx = useWindowsContext();
  const placeholder = useRef<HTMLDivElement>(null);
  const slot = ctx?.slotElement ?? null;

  useLayoutEffect(() => {
    const parent = placeholder.current;
    if (!parent || !slot) return;
    parent.appendChild(slot);
    return () => {
      slot.remove(); // detach, don't destroy: the portal target must outlive this page
    };
  }, [slot]);

  const publish = ctx?.publish;
  const controller = ctx?.controller;
  useLayoutEffect(() => {
    controller?.syncCurrentWindow(inputs.currentWindowId);
    publish?.(inputs);
  });

  return <div ref={placeholder} style={{ display: "contents" }} data-testid="window-host-placeholder" />;
}
