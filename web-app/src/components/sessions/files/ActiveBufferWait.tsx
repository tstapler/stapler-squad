"use client";

import { useEffect, type ReactNode, type RefObject } from "react";
import type { FileTreeHandle } from "../FileTree";
import type { FilesV2 } from "@/lib/hooks/useFilesV2";
import { useFilesAnalytics } from "@/lib/files/analytics/filesAnalytics";
import { PENDING_MS, WaitState, usePendingDelay } from "./PendingState";

const baseName = (path: string) => path.slice(path.lastIndexOf("/") + 1);

/**
 * The open wait of ux.md 8.5 for the active buffer: after 150 ms a pending panel ("Opening b.go..."),
 * on failure a message with Retry and Close tab (not found also offers Reveal in tree). The viewer stays
 * mounted underneath and is only hidden, so its own load is never restarted.
 */
export function useActiveBufferWait(v2: FilesV2 | undefined, fileTreeRef: RefObject<FileTreeHandle | null>): { node: ReactNode; hideViewer: boolean } {
  const analytics = useFilesAnalytics({ component: "FileContentViewer" });
  const active = v2?.buffers.active ?? null;
  const loading = active?.status === "loading";
  const showPending = usePendingDelay(loading, PENDING_MS);
  const failed = active?.status === "failed" ? active : null;
  const failureKey = failed ? `${failed.path}:${failed.reason}` : null;

  // One event per failure shown, enum reason only.
  useEffect(() => {
    if (failed) analytics.emit("files.open.failed", { reason: failed.reason });
    // failureKey identifies the failure; `failed` itself changes identity on unrelated state updates.
  }, [failureKey]); // eslint-disable-line react-hooks/exhaustive-deps

  if (!v2 || !active) return { node: null, hideViewer: false };
  const name = baseName(active.path);
  if (failed) {
    const { buffers, bridge } = v2;
    const actions = [
      { label: "Retry", onClick: () => { buffers.close(failed.path); bridge.open(failed.path); } },
      { label: "Close tab", onClick: () => buffers.close(failed.path) },
      ...(failed.reason === "not_found" ? [{ label: "Reveal in tree", onClick: () => fileTreeRef.current?.revealPath(failed.path) }] : []),
    ];
    return { node: <WaitState kind="open" name={name} failure={{ reason: failed.reason, actions }} />, hideViewer: true };
  }
  if (showPending) return { node: <WaitState kind="open" name={name} />, hideViewer: true };
  return { node: null, hideViewer: false };
}
