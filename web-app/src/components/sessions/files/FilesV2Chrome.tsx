"use client";

import type { RefObject } from "react";
import type { FileTreeHandle } from "../FileTree";
import { canGoBack, canGoForward } from "@/lib/files/bufferReducer";
import type { FilesV2 } from "@/lib/hooks/useFilesV2";
import { GoToLineDialog } from "./GoToLineDialog";
import { OpenTabsBar } from "./OpenTabsBar";
import { OutlinePanel } from "./OutlinePanel";
import { BannerStack } from "./BannerStack";
import { staleBannerSpec } from "./StaleBanner";
import { reloadFailedBanner } from "./PendingState";
import { chrome, navRow, navButton, spacer } from "./FilesV2Chrome.css";

interface FilesV2ChromeProps {
  v2: FilesV2;
  fileTreeRef: RefObject<FileTreeHandle | null>;
  panelId: string;
}

/** Tab strip, back/forward/outline/go-to-line row, stale banner and their overlays. */
export function FilesV2Chrome({ v2, fileTreeRef, panelId }: FilesV2ChromeProps) {
  const { buffers, bridge, stale, sessionRunning } = v2;
  const { state } = buffers;
  const back = canGoBack(state);
  const forward = canGoForward(state);
  const reopen = () => {
    const top = state.closed[state.closed.length - 1];
    buffers.reopenClosed();
    if (top) v2.feedback.announceSuccess("reopened", top.path.slice(top.path.lastIndexOf("/") + 1));
  };
  const active = buffers.active;
  const banners = [
    ...(stale ? [staleBannerSpec(stale, sessionRunning, v2.onStaleAction)] : []),
    ...(v2.reloadFailed && active?.status === "loaded" && active.path === v2.reloadFailed.path
      ? [reloadFailedBanner({
          path: active.path, reason: "rpc_error", dirty: active.dirty, seq: v2.reloadFailed.seq,
          onRetry: v2.reloadFromDisk,
          onCopy: () => { const text = bridge.text(); if (text !== null) void navigator.clipboard?.writeText(text); },
          onKeep: v2.clearReloadFailed,
        })]
      : []),
  ];
  const closeOthers = (keep: string) => state.buffers.forEach((b) => { if (b.path !== keep) buffers.close(b.path); });
  const jumpToSymbol = (line: number) => {
    if (buffers.active) bridge.jumpTo(buffers.active.path, line, { source: "outline" });
    bridge.setOverlay(null);
  };

  return (
    <div className={chrome}>
      <OpenTabsBar
        buffers={state.buffers}
        activeIndex={state.activeIndex}
        notice={state.notice}
        panelId={panelId}
        onActivate={bridge.activate}
        onClose={(path) => buffers.close(path)}
        onPin={buffers.pin}
        onUnpin={buffers.unpin}
        onMove={buffers.move}
        onCloseOthers={closeOthers}
        onCopyPath={(path) => void navigator.clipboard?.writeText(path)}
        onReveal={(path) => fileTreeRef.current?.revealPath(path)}
        onUndoEvict={reopen}
      />
      <div className={navRow}>
        <button type="button" className={navButton} aria-label="Back" aria-disabled={!back} title={back ? "Back (Alt+[)" : "No earlier location"} onClick={() => bridge.back("button")}>←</button>
        <button type="button" className={navButton} aria-label="Forward" aria-disabled={!forward} title={forward ? "Forward (Alt+])" : "No later location"} onClick={() => bridge.forward("button")}>→</button>
        <span className={spacer} />
        <button type="button" className={navButton} aria-haspopup="dialog" disabled={!buffers.active} onClick={() => bridge.setOverlay("outline")}>Outline</button>
        <button type="button" className={navButton} aria-haspopup="dialog" disabled={!buffers.active} title="Go to line (Ctrl+G)" onClick={() => bridge.setOverlay("goto")}>Go to line</button>
      </div>
      <BannerStack banners={banners} />
      {bridge.overlay === "outline" && (
        <OutlinePanel result={bridge.outline()} onSelect={(s) => jumpToSymbol(s.line)} onClose={() => bridge.setOverlay(null)} onGoToLine={() => bridge.setOverlay("goto")} />
      )}
      {bridge.overlay === "goto" && (
        <GoToLineDialog
          onSubmit={(line) => { bridge.setOverlay(null); bridge.goToLine(line, "palette"); }}
          onClose={() => bridge.setOverlay(null)}
        />
      )}
    </div>
  );
}
