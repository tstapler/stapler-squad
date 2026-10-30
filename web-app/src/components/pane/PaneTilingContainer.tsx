"use client";

import { useRef, useEffect, useState, useCallback, type Dispatch } from "react";
import { createPortal } from "react-dom";
import { Columns2, Rows2, LayoutList, X, Maximize2, ArrowRight } from "lucide-react";
import type { Session } from "@/gen/session/v1/types_pb";
import { usePaneShortcuts } from "@/lib/pane/usePaneShortcuts";
import { getAllLeaves } from "@/lib/pane/paneReducer";
import type { PaneState, PaneAction, SplitDirection } from "@/lib/pane/paneTypes";
import { PaneSplitRenderer } from "./PaneSplitRenderer";
import { usePanePickerKeys } from "./usePanePickerKeys";
import { PaneContext } from "./PaneContext";
import { useViewport } from "@/components/providers/ViewportProvider";
import { SessionPeekModal } from "@/components/sessions/SessionPeekModal";
import { useBacklogSessionIndex } from "@/lib/hooks/useBacklogService";
import {
  pickerActionBar,
  pickerActionButton,
  pickerActionKbd,
  mobilePickerBackdrop,
  mobilePickerSheet,
  mobilePickerSheetTitle,
  mobilePickerPaneItem,
  mobilePickerPaneLabel,
  mobilePickerCancelButton,
} from "@/styles/pane/panePickerOverlay.css";

interface PaneTilingContainerProps {
  sessions: Session[];
  paneState: PaneState;
  dispatch: Dispatch<PaneAction>;
  /**
   * When set, the session with this id is assigned to the currently focused pane.
   * The `version` field must change each time an assignment should fire (even for
   * the same session), so callers should increment a counter.
   */
  externalSessionAssign?: {
    sessionId: string;
    tab?: "terminal" | "diff" | "vcs" | "logs" | "info" | "files" | "browser" | "artifacts" | "summary";
    forceNewPane?: boolean;
    version: number;
  } | null;
  /** False when this container's window is retained but hidden (keep-alive); gates keys, focus and external assign. */
  windowActive?: boolean;
}

/**
 * PaneTilingContainer — top-level tiling layout component.
 *
 * Holds the pane reducer state, wires keyboard shortcuts, persists layout,
 * and renders the recursive pane tree via PaneSplitRenderer.
 */
export function PaneTilingContainer({
  sessions,
  paneState,
  dispatch,
  externalSessionAssign,
  windowActive = true,
}: PaneTilingContainerProps) {
  const state = paneState;
  const containerRef = useRef<HTMLDivElement>(null);
  const prevVersionRef = useRef<number | null>(null);

  const [pickerPendingSession, setPickerPendingSession] = useState<Session | null>(null);
  const pickerBarRef = useRef<HTMLDivElement>(null);
  const [peekSession, setPeekSession] = useState<Session | null>(null);
  // Mobile-only: bottom sheet picker state (replaces overlay when on narrow screens)
  const [mobilePickerSession, setMobilePickerSession] = useState<Session | null>(null);
  const [mobilePickerPanes, setMobilePickerPanes] = useState<{ id: string; label: string; letter: string }[]>([]);
  const mobilePickerSheetRef = useRef<HTMLDivElement>(null);
  const { isMobile, isFoldable } = useViewport();
  const isNarrow = isMobile || isFoldable;
  const { index: backlogIndex } = useBacklogSessionIndex();

  const cancelPicker = useCallback(() => {
    setPickerPendingSession(null);
    setMobilePickerSession(null);
    setMobilePickerPanes([]);
  }, []);

  const triggerPickerForceNew = useCallback(
    (session: Session, tab?: string) => {
      const resolvedTab = (tab ?? "terminal") as "terminal" | "diff" | "vcs" | "logs" | "info" | "files";

      // Choose split direction that keeps terminal width >= 120 columns.
      // At ~8px per character, 120 cols ≈ 960px per pane → need container >= 1920px for vertical split.
      // If the container is narrower, use horizontal split (stacked) to preserve full width.
      const containerWidth = containerRef.current?.getBoundingClientRect().width ?? 0;
      const direction: SplitDirection = containerWidth / 2 >= 960 ? "vertical" : "horizontal";

      dispatch({
        type: "SPLIT_AND_ASSIGN_SESSION",
        paneId: state.focusedPaneId,
        sessionId: session.id,
        tab: resolvedTab,
        direction,
      });
    },
    [state.focusedPaneId, dispatch, containerRef],
  );

  const triggerPicker = useCallback(
    (session: Session, tab?: string) => {
      const resolvedTab = (tab ?? "terminal") as "terminal" | "diff" | "vcs" | "logs" | "info" | "files";

      const allLeaves = getAllLeaves(state.root);
      const eligiblePanes = allLeaves.filter((l) => l.viewKind !== "session-list");

      if (eligiblePanes.length === 0) {
        // Auto-split: reducer creates a new detail pane beside the session-list pane.
        dispatch({ type: "ASSIGN_SESSION", paneId: state.focusedPaneId, sessionId: session.id });
      } else if (eligiblePanes.length === 1 && eligiblePanes[0].sessionId === session.id) {
        // Already showing this session: just focus, so ASSIGN_SESSION doesn't reset the active tab.
        dispatch({ type: "FOCUS_PANE", paneId: eligiblePanes[0].id });
      } else if (eligiblePanes.length === 1) {
        // Single detail pane: assign directly. If focused pane is the detail pane,
        // eligiblePanes[0] === focusedLeaf, so the intent shortcut still fires — just
        // via this branch rather than the removed early-return block.
        dispatch({ type: "ASSIGN_SESSION", paneId: eligiblePanes[0].id, sessionId: session.id });
        dispatch({ type: "ASSIGN_TAB", paneId: eligiblePanes[0].id, tab: resolvedTab });
        // On mobile, vertical splits show only the focused pane. Move focus to the detail
        // pane so the user sees the session they just opened instead of the session list.
        dispatch({ type: "FOCUS_PANE", paneId: eligiblePanes[0].id });
      } else if (isNarrow) {
        // On mobile, only one pane is visible at a time — the desktop overlay (which
        // expects the user to click on a visible pane) doesn't work. Use a bottom sheet
        // that lists all eligible panes by name instead.
        const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ";
        setMobilePickerSession(session);
        setMobilePickerPanes(
          eligiblePanes.map((p, i) => {
            const sess = sessions.find((s) => s.id === p.sessionId);
            return {
              id: p.id,
              label: sess ? sess.title : (p.viewKind === "session-list" ? "Sessions" : "Empty pane"),
              letter: letters[i] ?? String(i + 1),
            };
          })
        );
      } else {
        // 2+ eligible panes on desktop: show the picker overlay so the user can click
        // on the pane they want.
        setPickerPendingSession(session);
      }
    },
    [state.root, state.focusedPaneId, dispatch, isNarrow, sessions],
  );

  // Register keyboard shortcuts
  usePaneShortcuts(state, dispatch, containerRef, windowActive);

  const openPeek = useCallback((session: Session) => setPeekSession(session), []);
  const findOpenPane = (sessionId: string | undefined) =>
    sessionId
      ? getAllLeaves(state.root).find((l) => l.viewKind !== "session-list" && l.sessionId === sessionId) ?? null
      : null;
  const alreadyOpenLeaf = findOpenPane(pickerPendingSession?.id);
  const alreadyOpenLeafMobile = findOpenPane(mobilePickerSession?.id);
  usePanePickerKeys(windowActive, {
    pickerPendingSession, state, dispatch, cancelPicker, openPeek,
    alreadyOpenPaneId: alreadyOpenLeaf?.id ?? null,
  });

  // Focus the mobile sheet when it opens so keyboard and screen reader users can interact.
  useEffect(() => {
    if (windowActive && mobilePickerSession) {
      mobilePickerSheetRef.current?.focus();
    }
  }, [windowActive, mobilePickerSession]);

  // Focus the first button in the picker action bar when it appears, so keyboard
  // and screen reader users can immediately interact without hunting for focus.
  useEffect(() => {
    if (windowActive && pickerPendingSession && pickerBarRef.current) {
      const firstBtn = pickerBarRef.current.querySelector<HTMLButtonElement>("button");
      firstBtn?.focus();
    }
  }, [windowActive, pickerPendingSession]);

  // When externalSessionAssign fires (omnibar, URL nav, keyboard), route through
  // triggerPicker so the user gets the same pane-picker UX as session-list clicks.
  useEffect(() => {
    if (!windowActive || !externalSessionAssign) return;
    if (externalSessionAssign.version === prevVersionRef.current) return;
    prevVersionRef.current = externalSessionAssign.version;

    const session = sessions.find((s) => s.id === externalSessionAssign.sessionId);
    if (!session) return;
    if (externalSessionAssign.forceNewPane) {
      triggerPickerForceNew(session, externalSessionAssign.tab);
    } else {
      triggerPicker(session, externalSessionAssign.tab);
    }
  }, [windowActive, externalSessionAssign, sessions, triggerPicker, triggerPickerForceNew]);

  const allLeaves = pickerPendingSession ? getAllLeaves(state.root) : [];
  const listPane = allLeaves.find((l) => l.viewKind === "session-list") ?? null;

  return (
    <PaneContext.Provider value={{ state, dispatch, sessions, pickerPendingSession, triggerPicker, triggerPickerForceNew, cancelPicker, backlogIndex }}>
      <div
        ref={containerRef}
        style={{ display: "flex", flex: 1, overflow: "hidden", minHeight: 0, position: "relative" }}
        data-context="cockpit"
      >
        <PaneSplitRenderer
          state={state}
          dispatch={dispatch}
          sessions={sessions}
          windowActive={windowActive}
        />
        {/* Aria-live region: always in DOM, announces picker appearance to screen readers */}
        <div
          role="status"
          aria-live="assertive"
          aria-atomic="true"
          style={{ position: "absolute", width: 1, height: 1, overflow: "hidden", clip: "rect(0,0,0,0)" }}
        >
          {pickerPendingSession
            ? `Choose where to open "${pickerPendingSession.title}": ${alreadyOpenLeaf ? "Open existing pane (O), " : ""}Side by side (V), Top/Bottom (H), Replace list (M), or Cancel (Esc)`
            : ""}
        </div>
        {pickerPendingSession && (
          <div
            ref={pickerBarRef}
            className={pickerActionBar}
            role="toolbar"
            aria-label={`Open ${pickerPendingSession.title}`}
          >
            {alreadyOpenLeaf && (
              <button
                className={pickerActionButton}
                data-testid="picker-open-existing"
                onClick={() => {
                  dispatch({ type: "FOCUS_PANE", paneId: alreadyOpenLeaf.id });
                  cancelPicker();
                }}
                title="Switch to the pane already showing this session"
              >
                {/* analytics-exempt */}
                <ArrowRight size={14} />
                Open existing pane
                <span className={pickerActionKbd}>O</span>
              </button>
            )}
            <button
              className={pickerActionButton}
              onClick={() => {
                dispatch({ type: "SPLIT_AND_ASSIGN_SESSION", paneId: state.focusedPaneId, sessionId: pickerPendingSession.id, tab: "terminal", direction: "vertical" });
                cancelPicker();
              }}
              title="Open in a new pane side by side"
            >
              {/* analytics-exempt */}
              <Columns2 size={14} />
              Side by side
              <span className={pickerActionKbd}>V</span>
            </button>
            <button
              className={pickerActionButton}
              onClick={() => {
                dispatch({ type: "SPLIT_AND_ASSIGN_SESSION", paneId: state.focusedPaneId, sessionId: pickerPendingSession.id, tab: "terminal", direction: "horizontal" });
                cancelPicker();
              }}
              title="Open in a new pane stacked top/bottom"
            >
              {/* analytics-exempt */}
              <Rows2 size={14} />
              Top / Bottom
              <span className={pickerActionKbd}>H</span>
            </button>
            {listPane && (
              <button
                className={pickerActionButton}
                onClick={() => {
                  dispatch({ type: "SET_PANE_VIEW", paneId: listPane.id, viewKind: "session-detail" });
                  dispatch({ type: "ASSIGN_SESSION", paneId: listPane.id, sessionId: pickerPendingSession.id });
                  cancelPicker();
                }}
                title="Replace the session list pane with this session"
              >
                {/* analytics-exempt */}
                <LayoutList size={14} />
                Replace list
              </button>
            )}
            <button
              className={pickerActionButton}
              onClick={() => {
                setPeekSession(pickerPendingSession);
                cancelPicker();
              }}
              title="Open in a floating modal without changing pane layout"
            >
              {/* analytics-exempt */}
              <Maximize2 size={14} />
              Open in Modal
              <span className={pickerActionKbd}>M</span>
            </button>
            <button
              className={pickerActionButton}
              onClick={cancelPicker}
              title="Cancel (Esc)"
            >
              {/* analytics-exempt */}
              <X size={14} />
              Cancel
              <span className={pickerActionKbd}>Esc</span>
            </button>
          </div>
        )}
        {peekSession && (
          <SessionPeekModal
            session={peekSession}
            onClose={() => setPeekSession(null)}
          />
        )}
      </div>

      {/* Mobile bottom sheet — rendered into document.body via portal to escape any
          ancestor transform/filter that would break fixed positioning */}
      {mobilePickerSession && createPortal(
        <>
          <div
            className={mobilePickerBackdrop}
            onClick={cancelPicker}
            aria-hidden="true"
          />
          <div
            ref={mobilePickerSheetRef}
            className={mobilePickerSheet}
            role="dialog"
            aria-modal="true"
            aria-labelledby="mobile-pane-picker-title"
            tabIndex={-1}
            onKeyDown={(e) => { if (e.key === "Escape") cancelPicker(); }}
          >
            <div id="mobile-pane-picker-title" className={mobilePickerSheetTitle}>Open session in…</div>
            {alreadyOpenLeafMobile && (
              <button
                className={mobilePickerPaneItem}
                data-testid="mobile-picker-open-existing"
                onClick={() => {
                  dispatch({ type: "FOCUS_PANE", paneId: alreadyOpenLeafMobile.id });
                  cancelPicker();
                }}
              >
                {/* analytics-exempt */}
                <span className={mobilePickerPaneLabel}>→</span>
                Open existing pane
              </button>
            )}
            {/* A split is just another full-screen page on narrow screens, so offer a
                fresh pane instead of forcing the session into an existing one. */}
            <button
              className={mobilePickerPaneItem}
              data-testid="mobile-picker-new-pane"
              onClick={() => {
                dispatch({
                  type: "SPLIT_AND_ASSIGN_SESSION",
                  paneId: state.focusedPaneId,
                  sessionId: mobilePickerSession.id,
                  tab: "terminal",
                  direction: "horizontal",
                });
                cancelPicker();
              }}
            >
              {/* analytics-exempt */}
              <span className={mobilePickerPaneLabel}>+</span>
              New full-screen pane
            </button>
            {mobilePickerPanes.map((p) => (
              <button
                key={p.id}
                className={mobilePickerPaneItem}
                onClick={() => {
                  dispatch({ type: "ASSIGN_SESSION", paneId: p.id, sessionId: mobilePickerSession.id });
                  dispatch({ type: "ASSIGN_TAB", paneId: p.id, tab: "terminal" });
                  dispatch({ type: "FOCUS_PANE", paneId: p.id });
                  cancelPicker();
                }}
              >
                {/* analytics-exempt */}
                <span className={mobilePickerPaneLabel}>{p.letter}</span>
                {p.label}
              </button>
            ))}
            <button className={mobilePickerCancelButton} onClick={cancelPicker}>
              {/* analytics-exempt */}
              Cancel
            </button>
          </div>
        </>,
        document.body
      )}
    </PaneContext.Provider>
  );
}
