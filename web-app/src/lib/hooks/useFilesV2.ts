"use client";

import { useCallback, useMemo } from "react";
import { useBufferFreshness, type StaleAction } from "./useBufferFreshness";
import { useEditorBridge } from "./useEditorBridge";
import { useOpenBuffers, type UseOpenBuffersOptions } from "./useOpenBuffers";

const RECENT_LIMIT = 8;

/** Everything the buffered (files:viewer-v2) Files tab needs, composed once so FilesTab stays a thin root. */
export function useFilesV2(sessionId: string, sessionRunning: boolean, options: UseOpenBuffersOptions) {
  const buffers = useOpenBuffers(sessionId, options);
  const bridge = useEditorBridge(buffers);
  const freshness = useBufferFreshness({ active: buffers.active, enabled: true, sessionRunning, onAdoptToken: buffers.adoptToken });
  const { active, close } = buffers;
  const { stale, resolve } = freshness;
  const { back, forward, setOverlay, reload } = bridge;

  const shortcuts = useMemo(() => ({
    onBack: () => back("keyboard"),
    onForward: () => forward("keyboard"),
    onGoToLine: () => setOverlay("goto"),
    onCloseTab: () => { if (active) close(active.path); },
  }), [back, forward, setOverlay, active, close]);

  const onStaleAction = useCallback((action: StaleAction) => {
    if (action === "reload" && stale) reload(stale.token);
    resolve(action);
  }, [stale, reload, resolve]);

  const recentPaths = useMemo(() => buffers.mru.slice(0, RECENT_LIMIT), [buffers.mru]);
  return { buffers, bridge, stale, sessionRunning, shortcuts, recentPaths, onStaleAction };
}

export type FilesV2 = ReturnType<typeof useFilesV2>;
