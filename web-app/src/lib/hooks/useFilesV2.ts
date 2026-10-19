"use client";

import { useCallback, useMemo, useState } from "react";
import type { PaletteCommand } from "@/components/sessions/QuickOpenPalette";
import { useAccessibleView } from "./useAccessibleView";
import { useBufferFreshness, type StaleAction } from "./useBufferFreshness";
import { useEditorBridge } from "./useEditorBridge";
import { useFileTransport } from "./useFileTransport";
import { useFilesFeedback } from "./useFilesFeedback";
import { useOpenBuffers, type UseOpenBuffersOptions } from "./useOpenBuffers";
import { useStatusModel } from "./useStatusModel";

const RECENT_LIMIT = 8;
const baseName = (path: string) => path.slice(path.lastIndexOf("/") + 1);

/** Everything the buffered (files:viewer-v2) Files tab needs, composed once so FilesTab stays a thin root. */
export function useFilesV2(sessionId: string, sessionRunning: boolean, options: UseOpenBuffersOptions) {
  const buffers = useOpenBuffers(sessionId, options);
  const feedback = useFilesFeedback();
  const bridge = useEditorBridge(buffers, { announce: feedback.announceInfo });
  const freshness = useBufferFreshness({ active: buffers.active, enabled: true, sessionRunning, onAdoptToken: buffers.adoptToken });
  const { active, close } = buffers;
  const { stale, resolve } = freshness;
  const { back, forward, setOverlay, reload, swapView, text } = bridge;
  const accessible = useAccessibleView({ textLength: useCallback(() => text()?.length ?? 0, [text]), swapView });
  const transport = useFileTransport();
  const status = useStatusModel(bridge);
  const { announceSuccess, announceProgress, announceError } = feedback;

  const shortcuts = useMemo(() => ({
    onBack: () => back("keyboard"),
    onForward: () => forward("keyboard"),
    onGoToLine: () => setOverlay("goto"),
    onCloseTab: () => { if (active) close(active.path); },
  }), [back, forward, setOverlay, active, close]);

  const onStaleAction = useCallback((action: StaleAction) => {
    if (action === "reload" && stale) {
      reload(stale.token);
      announceSuccess("reloaded", baseName(stale.path));
    }
    resolve(action);
  }, [stale, reload, resolve, announceSuccess]);

  // The palette's "Reload from disk": stat first so the adopted token is the one the new text carries.
  // A failure never touches the buffer; it raises a banner with a way out (ux.md 8.5).
  const [reloadFailed, setReloadFailed] = useState<{ path: string; seq: number } | null>(null);
  const reloadFromDisk = useCallback(() => {
    if (!active || active.status !== "loaded") return;
    const { sessionId, path } = active;
    announceProgress("Checking the file on disk...");
    setReloadFailed(null);
    transport.stat({ sessionId, path }).then(
      (r) => { reload(r.token); announceSuccess("reloaded", baseName(path)); },
      () => { announceError("Could not check the file: network error"); setReloadFailed({ path, seq: Date.now() }); },
    );
  }, [active, transport, reload, announceSuccess, announceProgress, announceError]);
  const dirty = active?.status === "loaded" && active.dirty;
  const paletteCommands = useMemo<PaletteCommand[]>(() => [
    { id: "goto_line", label: "Go to line", shortcut: "Ctrl+G", disabledReason: active ? undefined : "no file open", run: () => setOverlay("goto") },
    { id: "toggle_accessible_view", label: accessible.on ? "Switch to code editor" : "Switch to accessible view", disabledReason: active ? undefined : "no file open", run: () => accessible.toggle("palette") },
    { id: "reload", label: "Reload from disk", disabledReason: active?.status === "loaded" ? undefined : "no file open", run: reloadFromDisk },
    { id: "save", label: "Save", shortcut: "Ctrl+S", disabledReason: dirty ? "saving is not available yet" : "no changes", run: () => undefined },
  ], [active, dirty, setOverlay, reloadFromDisk, accessible]);

  const viewerSource = active?.status === "loaded" ? active.source : undefined;
  const recentPaths = useMemo(() => buffers.mru.slice(0, RECENT_LIMIT), [buffers.mru]);
  return { buffers, bridge, viewerSource, accessible, stale, sessionRunning, shortcuts, recentPaths, onStaleAction, feedback, status, paletteCommands, reloadFailed, reloadFromDisk, clearReloadFailed: () => setReloadFailed(null) };
}

export type FilesV2 = ReturnType<typeof useFilesV2>;
