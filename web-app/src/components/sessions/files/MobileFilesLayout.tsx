"use client";

import { useEffect, useMemo, useState, type ReactNode } from "react";
import { canGoBack, canGoForward, type BufferNotice } from "@/lib/files/bufferReducer";
import { bucketTabs } from "@/lib/files/analytics/events";
import { useFilesAnalytics } from "@/lib/files/analytics/filesAnalytics";
import { stepFontPx } from "@/lib/files/fontSize";
import { useCoarsePointer } from "@/lib/hooks/useCoarsePointer";
import type { FilesV2 } from "@/lib/hooks/useFilesV2";
import type { PaneState, PaneVia } from "@/lib/hooks/useMobilePane";
import { useViewport } from "@/components/providers/ViewportProvider";
import { MobileBottomBar, MobileTopBar, type MenuItem } from "./MobileBars";
import { QuickfixChip, type QuickfixView } from "./SearchResults";
import { TabSwitcherSheet } from "./TabSwitcherSheet";
import { baseName, isDirty } from "./tabLabels";
import { useTabActions } from "./useTabActions";
import { floatingStack, stackHost, toast, toastButton, srOnly } from "./MobileFilesLayout.css";

const TOAST_MS = 8_000;

export interface MobileFilesLayoutProps {
  /** The mobile layout was chosen (`chooseFilesLayout`); otherwise only `children` render. */
  active: boolean;
  v2?: FilesV2;
  pane: PaneState;
  /** Pane the editor was opened from, for the back arrow's name. */
  origin: PaneState;
  containerWidth: number;
  onPane: (to: PaneState, via: PaneVia) => void;
  onBack: () => void;
  onQuickOpen: () => void;
  onSearch: () => void;
  quickfix?: QuickfixView | null;
  /** Extra overflow-menu entries (font stepper, and later vim and keyboard choices). */
  overflowItems?: MenuItem[];
  /** The panes; they stay at one position in the tree so a layout change never remounts the editor. */
  children: ReactNode;
}

function noticeText(n: BufferNotice | null): string | null {
  if (!n) return null;
  return n.kind === "evicted" ? `Closed ${baseName(n.path)} to stay within ${n.cap} tabs` : "Too many tabs open - close one";
}

/** Top bar, floating controls and bottom action bar around the three panes (ux.md 7.1). */
export function MobileFilesLayout(p: MobileFilesLayoutProps) {
  const { v2 } = p;
  const on = p.active && !!v2;
  const { isVirtualKeyboardOpen } = useViewport();
  const coarse = useCoarsePointer();
  const analytics = useFilesAnalytics({ component: "MobileFilesLayout" });
  const [sheetOpen, setSheetOpen] = useState(false);
  const [shown, setShown] = useState<BufferNotice | null>(null);
  const notice = v2?.buffers.state.notice ?? null;

  // The reducer clears its notice on the next action; the layout keeps what it showed until dismissed or timed out.
  useEffect(() => {
    if (!notice) return;
    setShown(notice);
    if (notice.kind === "refused") setSheetOpen(true);
  }, [notice]);
  useEffect(() => {
    if (shown?.kind !== "evicted") return;
    const t = setTimeout(() => setShown(null), TOAST_MS);
    return () => clearTimeout(t);
  }, [shown]);

  const tabActions = useTabActions(v2);
  const overflow = useMemo<MenuItem[]>(() => {
    if (!v2) return [];
    const active = v2.buffers.active;
    return [
      { label: "Outline", run: () => v2.bridge.setOverlay("outline"), disabled: !active },
      { label: v2.accessible.on ? "Switch to code editor" : "Switch to accessible view", run: () => v2.accessible.toggle("overflow"), disabled: !active },
      { label: "Copy path", run: () => active && tabActions.copyPath(active.path), disabled: !active },
      { label: "Text larger (A+)", run: () => void stepFontPx(1, coarse) },
      { label: "Text smaller (A-)", run: () => void stepFontPx(-1, coarse) },
      ...(p.overflowItems ?? []),
    ];
  }, [v2, p.overflowItems, tabActions, coarse]);

  if (!on || !v2) return <>{null}{p.children}{null}</>;

  const { buffers, bridge } = v2;
  const { state } = buffers;
  const active = buffers.active;
  const emitClose = (path: string, method: "button" | "context_menu" | "close_others") => {
    const b = state.buffers.find((x) => x.path === path);
    analytics.emit("files.tab.close", { dirty: b && isDirty(b) ? "true" : "false", method, open_tabs: bucketTabs(state.buffers.length) });
  };
  const closeFromSheet = (path: string, method: "button" | "context_menu") => {
    emitClose(path, method);
    buffers.close(path);
  };
  const closeOthersFromSheet = (keep: string) => {
    for (const b of state.buffers) if (b.path !== keep) emitClose(b.path, "close_others");
    tabActions.closeOthers(keep);
  };
  const selectFromSheet = (path: string) => {
    analytics.emit("files.tab.switch", { via: "sheet" });
    bridge.activate(path);
    setSheetOpen(false);
    // The sheet's own history entry pops first (async); only then may the editor pane push its entry.
    if (p.pane !== "editor") window.addEventListener("popstate", () => p.onPane("editor", "bottom_bar"), { once: true });
  };
  const message = noticeText(shown);

  return (
    <>
      <MobileTopBar
        pane={p.pane}
        origin={p.origin}
        path={active?.path ?? null}
        dirty={!!active && isDirty(active)}
        tabCount={state.buffers.length}
        overflow={overflow}
        onBack={p.onBack}
        onTabs={() => setSheetOpen(true)}
      />
      {p.children}
      {!isVirtualKeyboardOpen && (
        <>
          <div className={stackHost}>
            <div className={floatingStack} data-testid="floating-controls">
              {p.quickfix && p.pane === "editor" && <QuickfixChip view={p.quickfix} stacked />}
            </div>
            {shown?.kind === "evicted" && (
              <div className={toast} data-testid="files-tab-toast" role="group" aria-label="Tab closed">
                <span>{message}</span>
                <button type="button" className={toastButton} onClick={() => { tabActions.reopen(); setShown(null); }}>Undo</button>
                <button type="button" className={toastButton} aria-label="Dismiss" onClick={() => setShown(null)}>×</button>
              </div>
            )}
          </div>
          <MobileBottomBar
            pane={p.pane}
            width={p.containerWidth}
            hasFile={!!active}
            canBack={canGoBack(state)}
            canForward={canGoForward(state)}
            onFiles={() => p.onPane("files", "bottom_bar")}
            onSearch={p.onSearch}
            onQuickOpen={p.onQuickOpen}
            onGoToLine={() => bridge.setOverlay("goto")}
            onBack={() => bridge.back("button")}
            onForward={() => bridge.forward("button")}
          />
        </>
      )}
      <div className={srOnly} role="status" aria-live="polite">{message}</div>
      {sheetOpen && (
        <TabSwitcherSheet
          buffers={state.buffers}
          activeIndex={state.activeIndex}
          refusal={shown?.kind === "refused" ? message : null}
          onSelect={selectFromSheet}
          onClose={closeFromSheet}
          onPin={buffers.pin}
          onUnpin={buffers.unpin}
          onCloseOthers={closeOthersFromSheet}
          onCopyPath={tabActions.copyPath}
          onDismiss={() => setSheetOpen(false)}
          onGoToFiles={() => { setSheetOpen(false); p.onPane("files", "bottom_bar"); }}
        />
      )}
    </>
  );
}
