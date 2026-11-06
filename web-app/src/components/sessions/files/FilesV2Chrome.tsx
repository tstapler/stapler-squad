"use client";

import type { RefObject } from "react";
import type { FileTreeHandle } from "../FileTree";
import { canGoBack, canGoForward } from "@/lib/files/bufferReducer";
import type { FilesV2 } from "@/lib/hooks/useFilesV2";
import { useCoarsePointer } from "@/lib/hooks/useCoarsePointer";
import { GoToLineDialog } from "./GoToLineDialog";
import { OpenTabsBar } from "./OpenTabsBar";
import { OutlinePanel } from "./OutlinePanel";
import { BannerStack } from "./BannerStack";
import { staleBannerSpec } from "./StaleBanner";
import { useTabActions } from "./useTabActions";
import { reloadFailedBanner } from "./PendingState";
import { chrome, navRow, navButton, spacer } from "./FilesV2Chrome.css";

interface FilesV2ChromeProps {
  v2: FilesV2;
  fileTreeRef: RefObject<FileTreeHandle | null>;
  panelId: string;
  /** Mobile layout: the tab sheet and bottom bar replace the strip and the nav row. */
  mobile?: boolean;
  /** Desktop layout only: toolbar launchers for the chords a soft keyboard does not have. */
  onQuickOpen?: () => void;
  onSearch?: () => void;
}

/** Tab strip, back/forward/outline/go-to-line row, stale banner and their overlays. */
export function FilesV2Chrome({ v2, fileTreeRef, panelId, mobile = false, onQuickOpen, onSearch }: FilesV2ChromeProps) {
  const { buffers, bridge, stale, sessionRunning } = v2;
  const { state } = buffers;
  const back = canGoBack(state);
  const forward = canGoForward(state);
  const { closeOthers, copyPath, reopen } = useTabActions(v2);
  // Touch sizing follows the pointer until the resolved InputProfile (ADR-033) lands; 44px touch, 24px with a hardware keyboard.
  const target = useCoarsePointer() ? 44 : 24;
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
  if (bridge.beyond) {
    banners.push({
      id: `beyond:${bridge.beyond.path}`, kind: "beyond_loaded", seq: 2, body: `Line ${bridge.beyond.line} is beyond the loaded portion`, onDismiss: bridge.dismissBeyond,
    });
  }
  const jumpToSymbol = (line: number) => {
    if (buffers.active) bridge.jumpTo(buffers.active.path, line, { source: "outline" });
    bridge.setOverlay(null);
  };

  return (
    <div className={chrome}>
      {!mobile && <OpenTabsBar
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
        onCopyPath={copyPath}
        onReveal={(path) => fileTreeRef.current?.revealPath(path)}
        onUndoEvict={reopen}
      />}
      {!mobile && <div className={navRow}>
        <button type="button" className={navButton} aria-label="Back" aria-disabled={!back} title={back ? "Back (Alt+[)" : "No earlier location"} onClick={() => bridge.back("button")}>←</button>
        <button type="button" className={navButton} aria-label="Forward" aria-disabled={!forward} title={forward ? "Forward (Alt+])" : "No later location"} onClick={() => bridge.forward("button")}>→</button>
        <span className={spacer} />
        {onQuickOpen && <button type="button" className={navButton} style={{ minHeight: target, minWidth: target }} aria-haspopup="dialog" title="Quick open (Ctrl+P)" onClick={onQuickOpen}>Quick open</button>}
        {onSearch && <button type="button" className={navButton} style={{ minHeight: target, minWidth: target }} title="Search (Ctrl+Shift+F)" onClick={onSearch}>Search</button>}
        <button type="button" className={navButton} style={{ minHeight: target, minWidth: target }} aria-pressed={v2.accessible.on} disabled={!buffers.active} title="Accessible view: plain paged text for screen readers" onClick={() => v2.accessible.toggle("toolbar")}>Accessible view</button>
        <button type="button" className={navButton} style={{ minHeight: target, minWidth: target }} aria-haspopup="dialog" disabled={!buffers.active} onClick={() => bridge.setOverlay("outline")}>Outline</button>
        <button type="button" className={navButton} style={{ minHeight: target, minWidth: target }} aria-haspopup="dialog" disabled={!buffers.active} title="Go to line (Ctrl+G)" onClick={() => bridge.setOverlay("goto")}>Go to line</button>
      </div>}
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
