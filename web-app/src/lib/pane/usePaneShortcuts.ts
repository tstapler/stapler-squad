"use client";

import { useCallback, RefObject } from "react";
import { useShortcut } from "@/lib/shortcuts/useShortcut";
import type { PaneState } from "./paneTypes";
import { getAdjacentLeaf } from "./paneUtils";
import type { PaneAction } from "./paneTypes";

/**
 * usePaneShortcuts — registers all 12 pane keyboard shortcuts.
 *
 * All shortcuts use context: "cockpit" so they fire even when a terminal
 * pane is focused (see the cockpit context guard in shortcutRegistry.ts).
 *
 * Note on Ctrl+-: browser zoom-out on Windows/Linux. event.preventDefault()
 * in ShortcutRegistry's dispatch loop reliably blocks zoom in Chrome/Firefox.
 * On macOS, zoom is Cmd+-, so Ctrl+- is safe. If Safari proves unreliable,
 * use Ctrl+Shift+H as a fallback.
 *
 * Note on Ctrl+W: browser close-tab. preventDefault() blocks it on Chrome/Firefox
 * when the page's keydown handler fires first. The close button (✕) in each pane
 * header is the primary reliable close path.
 */
export function usePaneShortcuts(
  state: PaneState,
  dispatch: React.Dispatch<PaneAction>,
  containerRef: RefObject<HTMLElement | null>
): void {
  // ── Split ──────────────────────────────────────────────────────────────────
  const splitVertical = useCallback(() => {
    dispatch({ type: "SPLIT_PANE", paneId: state.focusedPaneId, direction: "vertical" });
  }, [dispatch, state.focusedPaneId]);

  const splitHorizontal = useCallback(() => {
    dispatch({ type: "SPLIT_PANE", paneId: state.focusedPaneId, direction: "horizontal" });
  }, [dispatch, state.focusedPaneId]);

  // ── Close ──────────────────────────────────────────────────────────────────
  const closePane = useCallback(() => {
    dispatch({ type: "CLOSE_PANE", paneId: state.focusedPaneId });
  }, [dispatch, state.focusedPaneId]);

  // ── Focus navigation ───────────────────────────────────────────────────────
  const focusRight = useCallback(() => {
    const adj = getAdjacentLeaf(state.root, state.focusedPaneId, "ArrowRight");
    if (adj) dispatch({ type: "FOCUS_PANE", paneId: adj.id });
  }, [dispatch, state.root, state.focusedPaneId]);

  const focusLeft = useCallback(() => {
    const adj = getAdjacentLeaf(state.root, state.focusedPaneId, "ArrowLeft");
    if (adj) dispatch({ type: "FOCUS_PANE", paneId: adj.id });
  }, [dispatch, state.root, state.focusedPaneId]);

  const focusUp = useCallback(() => {
    const adj = getAdjacentLeaf(state.root, state.focusedPaneId, "ArrowUp");
    if (adj) dispatch({ type: "FOCUS_PANE", paneId: adj.id });
  }, [dispatch, state.root, state.focusedPaneId]);

  const focusDown = useCallback(() => {
    const adj = getAdjacentLeaf(state.root, state.focusedPaneId, "ArrowDown");
    if (adj) dispatch({ type: "FOCUS_PANE", paneId: adj.id });
  }, [dispatch, state.root, state.focusedPaneId]);

  // ── Resize nudge ───────────────────────────────────────────────────────────
  const nudgeRight = useCallback(() => {
    const containerSizePx = containerRef.current?.getBoundingClientRect().width ?? 400;
    dispatch({
      type: "NUDGE_RESIZE",
      paneId: state.focusedPaneId,
      direction: "ArrowRight",
      amountPx: 20,
      containerSizePx,
    });
  }, [dispatch, state.focusedPaneId, containerRef]);

  const nudgeLeft = useCallback(() => {
    const containerSizePx = containerRef.current?.getBoundingClientRect().width ?? 400;
    dispatch({
      type: "NUDGE_RESIZE",
      paneId: state.focusedPaneId,
      direction: "ArrowLeft",
      amountPx: 20,
      containerSizePx,
    });
  }, [dispatch, state.focusedPaneId, containerRef]);

  const nudgeUp = useCallback(() => {
    const containerSizePx = containerRef.current?.getBoundingClientRect().height ?? 400;
    dispatch({
      type: "NUDGE_RESIZE",
      paneId: state.focusedPaneId,
      direction: "ArrowUp",
      amountPx: 20,
      containerSizePx,
    });
  }, [dispatch, state.focusedPaneId, containerRef]);

  const nudgeDown = useCallback(() => {
    const containerSizePx = containerRef.current?.getBoundingClientRect().height ?? 400;
    dispatch({
      type: "NUDGE_RESIZE",
      paneId: state.focusedPaneId,
      direction: "ArrowDown",
      amountPx: 20,
      containerSizePx,
    });
  }, [dispatch, state.focusedPaneId, containerRef]);

  // ── Swap ───────────────────────────────────────────────────────────────────
  const swapRight = useCallback(() => {
    const adj = getAdjacentLeaf(state.root, state.focusedPaneId, "ArrowRight");
    if (adj) dispatch({ type: "SWAP_PANES", paneId: state.focusedPaneId, targetPaneId: adj.id });
  }, [dispatch, state.root, state.focusedPaneId]);

  const swapLeft = useCallback(() => {
    const adj = getAdjacentLeaf(state.root, state.focusedPaneId, "ArrowLeft");
    if (adj) dispatch({ type: "SWAP_PANES", paneId: state.focusedPaneId, targetPaneId: adj.id });
  }, [dispatch, state.root, state.focusedPaneId]);

  const swapUp = useCallback(() => {
    const adj = getAdjacentLeaf(state.root, state.focusedPaneId, "ArrowUp");
    if (adj) dispatch({ type: "SWAP_PANES", paneId: state.focusedPaneId, targetPaneId: adj.id });
  }, [dispatch, state.root, state.focusedPaneId]);

  const swapDown = useCallback(() => {
    const adj = getAdjacentLeaf(state.root, state.focusedPaneId, "ArrowDown");
    if (adj) dispatch({ type: "SWAP_PANES", paneId: state.focusedPaneId, targetPaneId: adj.id });
  }, [dispatch, state.root, state.focusedPaneId]);

  // ── Zoom ───────────────────────────────────────────────────────────────────
  const zoomPane = useCallback(() => {
    dispatch({ type: "ZOOM_PANE", paneId: state.focusedPaneId });
  }, [dispatch, state.focusedPaneId]);

  // ── Register all shortcuts ─────────────────────────────────────────────────
  useShortcut("cockpit.split-vertical", {
    key: "\\",
    modifiers: { ctrl: true },
    label: "Split pane vertically",
    context: "cockpit",
    action: splitVertical,
  });

  useShortcut("cockpit.split-horizontal", {
    key: "-",
    modifiers: { ctrl: true },
    label: "Split pane horizontally",
    context: "cockpit",
    action: splitHorizontal,
  });

  useShortcut("cockpit.close-pane", {
    key: "w",
    modifiers: { ctrl: true },
    label: "Close focused pane",
    context: "cockpit",
    action: closePane,
  });

  useShortcut("cockpit.focus-right", {
    key: "ArrowRight",
    modifiers: { ctrl: true },
    label: "Focus pane right",
    context: "cockpit",
    action: focusRight,
  });

  useShortcut("cockpit.focus-left", {
    key: "ArrowLeft",
    modifiers: { ctrl: true },
    label: "Focus pane left",
    context: "cockpit",
    action: focusLeft,
  });

  useShortcut("cockpit.focus-up", {
    key: "ArrowUp",
    modifiers: { ctrl: true },
    label: "Focus pane up",
    context: "cockpit",
    action: focusUp,
  });

  useShortcut("cockpit.focus-down", {
    key: "ArrowDown",
    modifiers: { ctrl: true },
    label: "Focus pane down",
    context: "cockpit",
    action: focusDown,
  });

  useShortcut("cockpit.resize-right", {
    key: "ArrowRight",
    modifiers: { ctrl: true, alt: true },
    label: "Resize right",
    context: "cockpit",
    action: nudgeRight,
  });

  useShortcut("cockpit.resize-left", {
    key: "ArrowLeft",
    modifiers: { ctrl: true, alt: true },
    label: "Resize left",
    context: "cockpit",
    action: nudgeLeft,
  });

  useShortcut("cockpit.resize-up", {
    key: "ArrowUp",
    modifiers: { ctrl: true, alt: true },
    label: "Resize up",
    context: "cockpit",
    action: nudgeUp,
  });

  useShortcut("cockpit.resize-down", {
    key: "ArrowDown",
    modifiers: { ctrl: true, alt: true },
    label: "Resize down",
    context: "cockpit",
    action: nudgeDown,
  });

  useShortcut("cockpit.zoom-pane", {
    key: "z",
    modifiers: { ctrl: true },
    label: "Zoom/unzoom focused pane",
    context: "cockpit",
    action: zoomPane,
  });

  useShortcut("cockpit.swap-right", {
    key: "ArrowRight",
    modifiers: { ctrl: true, shift: true },
    label: "Swap pane right",
    context: "cockpit",
    action: swapRight,
  });

  useShortcut("cockpit.swap-left", {
    key: "ArrowLeft",
    modifiers: { ctrl: true, shift: true },
    label: "Swap pane left",
    context: "cockpit",
    action: swapLeft,
  });

  useShortcut("cockpit.swap-up", {
    key: "ArrowUp",
    modifiers: { ctrl: true, shift: true },
    label: "Swap pane up",
    context: "cockpit",
    action: swapUp,
  });

  useShortcut("cockpit.swap-down", {
    key: "ArrowDown",
    modifiers: { ctrl: true, shift: true },
    label: "Swap pane down",
    context: "cockpit",
    action: swapDown,
  });
}
