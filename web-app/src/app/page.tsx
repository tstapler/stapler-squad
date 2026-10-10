"use client";
// +feature: session-list session-search session-filter session-groupby session-list-collapse-groups

import React, { useState, useEffect, useRef, Suspense, useCallback, useMemo } from "react";
import { useSearchParams, useRouter } from "next/navigation";
import { usePushClickHandoff } from "@/lib/hooks/usePushNotifications";
import { Session } from "@/gen/session/v1/types_pb";
import { SessionListSkeleton } from "@/components/sessions/SessionListSkeleton";
import { SessionDetailTab } from "@/components/sessions/SessionDetail";

const VALID_TABS = ["terminal", "diff", "vcs", "logs", "info"] as const;
function isValidTab(tab: string | null): tab is SessionDetailTab {
  return tab !== null && (VALID_TABS as readonly string[]).includes(tab);
}
import { ResumeSessionModal } from "@/components/sessions/ResumeSessionModal";
import { useSessionServiceContext } from "@/lib/contexts/SessionServiceContext";
import { useKeyboard } from "@/lib/hooks/useKeyboard";
import { useFocusTrap } from "@/lib/hooks/useFocusTrap";
import { useOmnibar } from "@/lib/contexts/OmnibarContext";
import { PaneTilingContainer } from "@/components/pane/PaneTilingContainer";
import type { PaneAction } from "@/lib/pane/paneTypes";
import { useWindowManager } from "@/lib/window/useWindowManager";
import { useWindowUrlSync } from "@/lib/window/useWindowUrlSync";
import { useWindowShortcuts } from "@/lib/window/useWindowShortcuts";
import type { WindowId } from "@/lib/window/windowTypes";
import { WindowTabStrip, type WindowTabStripHandle } from "@/components/window/WindowTabStrip";
import { CockpitActionsProvider } from "@/lib/contexts/CockpitActionsContext";
import { SessionViewModeProvider } from "@/lib/contexts/SessionViewModeContext";
import { useSessionViewMode } from "@/lib/hooks/useSessionViewMode";
import { usePageView } from "@/lib/analytics/usePageView";
import { useAnalytics } from "@/lib/contexts/AnalyticsContext";
import { useNotifications } from "@/lib/contexts/NotificationContext";
import { findSessionById as findSessionByIdIn } from "./findSessionById";
import { SessionUnavailableCard } from "@/components/sessions/SessionUnavailableCard";
import { useNotificationRecord } from "@/lib/hooks/useNotificationRecord";
import {
  classifyGetSessionFailure,
  parseSessionDeepLink,
  shouldResolveViaGetSession,
  type SessionDeepLinkParams,
  type SessionLookupOutcome,
} from "./sessionDeepLink";
import { useOmnibarQueryParamLaunch } from "./useOmnibarQueryParamLaunch";
import { DeleteSessionConfirmModal } from "./DeleteSessionConfirmModal";
import { useResumeSessionFlow } from "./useResumeSessionFlow";
import * as styles from "./page.css";

// A hidden session is not in the watch stream, so an open one is polled for status changes.
const FALLBACK_SESSION_REFRESH_MS = 5_000;
const MAX_FALLBACK_SESSIONS = 5;

function HomeContent() {
  usePageView();
  const { track } = useAnalytics();
  const { addNotification, notificationHistory } = useNotifications();
  const searchParams = useSearchParams();
  const router = useRouter();
  usePushClickHandoff(useCallback((url: string) => router.push(url), [router]));
  const { openInCreationMode, openOmnibar } = useOmnibar();
  const [selectedSession, setSelectedSession] = useState<Session | null>(null);
  const [activeTab, setActiveTab] = useState<SessionDetailTab>("info");
  const [deleteConfirmTarget, setDeleteConfirmTarget] = useState<Session | null>(null);
  const [pendingSessionId, setPendingSessionId] = useState<string | null>(null);
  // j/k keyboard navigation index within the session list
  const [focusedSessionIndex, setFocusedSessionIndex] = useState<number>(-1);

  // List/Board view mode — lifted here (rather than called again deep in the pane
  // tree) so the 'b' shortcut below and SessionListPaneBody's render both observe
  // the same state instead of two independent useSessionViewMode() instances.
  const [viewMode, setViewMode] = useSessionViewMode();

  // Tiling: tracks the most-recently-clicked session to route to the focused pane.
  // Using a counter-based key so that clicking the same session again still triggers.
  const [externalAssignCounter, setExternalAssignCounter] = useState(0);
  const [externalAssignSession, setExternalAssignSession] = useState<{ sessionId: string; tab: SessionDetailTab; forceNewPane?: boolean } | null>(null);

  // Focus management: modal containers (tabIndex={-1}) and trigger element refs
  const sessionDetailRef = useRef<HTMLDivElement>(null);
  const sessionTriggerRef = useRef<HTMLElement | null>(null);
  const deleteDialogRef = useRef<HTMLDivElement>(null);
  const lastFocusBeforeDelete = useRef<HTMLElement | null>(null);
  const windowTabStripRef = useRef<WindowTabStripHandle>(null);

  // Tracks the last URL params that were routed to a pane. Prevents the URL-watching
  // effect from re-triggering pane assignment on every sessions stream update (which
  // changes the `sessions` dependency but not the URL itself).
  const lastUrlRoutedRef = useRef<{ sessionId: string | null; tab: string | null }>({ sessionId: null, tab: null });

  // Guards the hidden-session fallback fetch (below) so it fires once per
  // sessionId instead of refiring on every `sessions` stream update.
  const hiddenSessionFallbackRef = useRef<string | null>(null);

  // Sessions resolved through getSession because the live list does not carry them
  // (hidden sessions). They are handed to the pane tree only, never to the session list.
  const [fallbackSessions, setFallbackSessions] = useState<Record<string, Session>>({});
  // Set when the deep link's session could not be opened: gone (not_found) or a
  // retryable lookup failure (failed). Rendered as a card, never a 404 or blank screen.
  const [lookup, setLookup] = useState<{
    sessionId: string;
    notificationId: string | null;
    status: Exclude<SessionLookupOutcome, "found">;
  } | null>(null);

  // Focus detail panel when session opens; return focus on close
  useEffect(() => {
    if (selectedSession) {
      sessionDetailRef.current?.focus();
    } else if (sessionTriggerRef.current) {
      sessionTriggerRef.current.focus();
      sessionTriggerRef.current = null;
    }
  }, [selectedSession]);

  // Trap focus inside delete confirmation dialog; return focus on close
  useFocusTrap(deleteDialogRef, !!deleteConfirmTarget, lastFocusBeforeDelete);

  const {
    sessions,
    loading,
    error,
    deleteSession,
    pauseSession,
    resumeSession,
    renameSession,
    restartSession,
    pinSession,
    unpinSession,
    retrySession,
    clearConversationState,
    createCheckpoint,
    listCheckpoints,
    forkSession,
    listSessions,
    updateSession,
    getSession,
    hasLoadedOnce,
    error: sessionsError,
  } = useSessionServiceContext();

  const paneSessions = useMemo(() => {
    const extras = Object.values(fallbackSessions).filter((f) => !sessions.some((s) => s.id === f.id));
    return extras.length > 0 ? [...sessions, ...extras] : sessions;
  }, [sessions, fallbackSessions]);

  const {
    resumeTarget,
    resumeTriggerRef,
    handleResumeRequest,
    handleDirectResume,
    handleResumeConfirm,
    handleResumeCancel,
  } = useResumeSessionFlow(resumeSession, track);

  // Multi-window layer (Epic 2.2): useWindowManager owns the windows array/persistence;
  // useWindowUrlSync resolves which window this tab is showing from `?window=`. Neither
  // hook has a notion of "the active window" on its own (ADR-001) — currentWindow is
  // derived here by looking up currentWindowId in the windows array.
  // createWindow/closeWindow/renameWindow have no UI to call them from yet — WindowTabStrip
  // (Task 2.2.2b) is deferred to Epic 3.1, where it doesn't exist yet.
  const { windows, isRestored: isWindowsRestored, dispatchPane, createWindow, closeWindow, renameWindow } =
    useWindowManager(paneSessions);
  const { currentWindowId, switchToWindow } = useWindowUrlSync(windows, isWindowsRestored);
  const currentWindow = windows.find((w) => w.id === currentWindowId) ?? windows[0];
  const paneDispatch = useCallback(
    (action: PaneAction) => dispatchPane(currentWindow.id, action),
    [dispatchPane, currentWindow.id]
  );
  // Bridges useWindowShortcuts' "," leader follow-up (which only knows a
  // window id) to WindowTabStrip's imperative beginEdit(id, name) — the same
  // inline editor double-click/F2 already open, not a separate native prompt.
  const handleWindowRenameRequest = useCallback(
    (id: WindowId) => {
      const target = windows.find((w) => w.id === id);
      if (target) windowTabStripRef.current?.beginEdit(id, target.name);
    },
    [windows]
  );
  useWindowShortcuts(windows, currentWindow.id, switchToWindow, handleWindowRenameRequest);
  // Helper function to find a session by ID with fuzzy matching for external sessions
  const findSessionById = useCallback(
    (sessionId: string): Session | undefined => findSessionByIdIn(sessions, sessionId),
    [sessions]
  );

  // Update URL with session and tab parameters
  const updateUrl = useCallback((sessionId: string | null, tab: SessionDetailTab | null) => {
    const params = new URLSearchParams();
    if (sessionId) {
      params.set("session", sessionId);
      if (tab && tab !== "info") {
        params.set("tab", tab);
      }
    }
    const query = params.toString();
    router.replace(query ? `/?${query}` : "/", { scroll: false });
  }, [router]);

  // Handle pending session navigation from notification click
  useEffect(() => {
    if (pendingSessionId && sessions.length > 0) {
      const session = findSessionById(pendingSessionId);
      if (session) {
        setSelectedSession(session);
        setActiveTab("terminal");
        updateUrl(session.id, "terminal");
      } else {
        console.warn(`[Notification] Session not found: ${pendingSessionId}`);
      }
      setPendingSessionId(null);
    }
  }, [pendingSessionId, sessions, findSessionById, updateUrl]);

  // Routes to a resolved session — shared by the in-list (findSessionById)
  // and hidden-session (getSession fallback) paths below so both apply the
  // exact same tab/pane wiring.
  const routeToResolvedSession = useCallback(
    (session: Session, sessionId: string, tabParam: string | null, newPaneParam: string | null) => {
      setSelectedSession(session);
      const resolvedTab = isValidTab(tabParam) ? tabParam : "terminal";
      setActiveTab(resolvedTab);
      // Only route to pane when URL params actually changed. The `sessions` dependency
      // causes this effect to re-run on every stream update; without this guard the
      // picker would re-appear after every session status change.
      if (lastUrlRoutedRef.current.sessionId !== sessionId || lastUrlRoutedRef.current.tab !== tabParam) {
        lastUrlRoutedRef.current = { sessionId, tab: tabParam };
        setExternalAssignCounter((c) => c + 1);
        setExternalAssignSession({
          sessionId: session.id,
          tab: resolvedTab,
          forceNewPane: newPaneParam === "true",
        });
      }
      // Clean up newPane param from URL after consuming it
      if (newPaneParam === "true") {
        const params = new URLSearchParams();
        params.set("session", sessionId);
        if (tabParam) params.set("tab", tabParam);
        router.replace(`/?${params.toString()}`, { scroll: false });
      }
    },
    [router]
  );

  // Not in the locally-fetched list — e.g. a Diagnose & Nudge diagnostic
  // session, dispatched with hidden=true so it doesn't clutter the main
  // session list (server/services/session_service.go's
  // SpawnDiagnosticSession). ListSessions excludes hidden sessions by
  // default and this component never passes includeHidden, so such a
  // session can never appear in `sessions` — without this fallback, its
  // "View diagnostic session" deep link (StuckItemDetail.tsx) would silently
  // no-op. GetSession, unlike ListSessions, doesn't filter on Hidden, so a
  // direct-by-UUID lookup still resolves it. Guarded by
  // hiddenSessionFallbackRef so it fires once per sessionId, not on every
  // `sessions` stream update.
  const fetchHiddenSessionFallback = useCallback(
    (link: SessionDeepLinkParams, force = false) => {
      const { sessionId } = link;
      if (!sessionId) return;
      if (!force && hiddenSessionFallbackRef.current === sessionId) return;
      hiddenSessionFallbackRef.current = sessionId;
      let failure: SessionLookupOutcome = "not_found";
      getSession(sessionId, {
        onFailure: (err) => {
          failure = classifyGetSessionFailure(err);
        },
      }).then((fetched) => {
        if (!fetched) {
          setLookup({
            sessionId,
            notificationId: link.notificationId,
            status: failure === "failed" ? "failed" : "not_found",
          });
          return;
        }
        setLookup(null);
        setFallbackSessions((prev) => {
          const kept = Object.entries(prev).filter(([id]) => id !== fetched.id).slice(-(MAX_FALLBACK_SESSIONS - 1));
          return { ...Object.fromEntries(kept), [fetched.id]: fetched };
        });
        routeToResolvedSession(fetched, sessionId, link.tab, link.newPane);
      });
    },
    [getSession, routeToResolvedSession]
  );

  // Handle direct session selection from URL. A cold deep link arrives with an empty
  // list: the fallback runs once the list request has settled, not on `sessions.length`.
  useEffect(() => {
    const link = parseSessionDeepLink(searchParams);
    if (!link.sessionId) {
      hiddenSessionFallbackRef.current = null;
      setLookup((prev) => (prev ? null : prev));
      return;
    }

    const session = findSessionById(link.sessionId);
    if (session) {
      routeToResolvedSession(session, link.sessionId, link.tab, link.newPane);
      return;
    }
    const listSettled = hasLoadedOnce === true || sessions.length > 0 || sessionsError != null;
    if (!shouldResolveViaGetSession({ sessionId: link.sessionId, foundInList: false, listSettled })) return;
    fetchHiddenSessionFallback(link);
  }, [searchParams, sessions, hasLoadedOnce, sessionsError, findSessionById, routeToResolvedSession, fetchHiddenSessionFallback]);

  // Keep a fallback session's status current: the watch stream never carries hidden sessions.
  const openFallbackIds = useMemo(() => Object.keys(fallbackSessions).join(","), [fallbackSessions]);
  useEffect(() => {
    const ids = openFallbackIds ? openFallbackIds.split(",") : [];
    if (ids.length === 0) return;
    const timer = setInterval(() => {
      for (const id of ids) {
        void getSession(id, { onFailure: () => {} }).then((fresh) => {
          if (fresh) setFallbackSessions((prev) => (prev[id] ? { ...prev, [id]: fresh } : prev));
        });
      }
    }, FALLBACK_SESSION_REFRESH_MS);
    return () => clearInterval(timer);
  }, [openFallbackIds, getSession]);

  const lookupNotification = useNotificationRecord(
    notificationHistory,
    lookup?.notificationId ?? null,
    lookup?.sessionId ?? null,
    lookup?.status === "not_found",
  );

  // Detect ?new=true, ?pr=<url>, ?duplicate=<id>, or ?worktree=<path>&branch=<branch> query params
  useOmnibarQueryParamLaunch(searchParams, router, openOmnibar, track, getSession);

  // Close session and clear URL query parameter
  const closeSession = () => {
    setSelectedSession(null);
    setActiveTab("info");
    lastUrlRoutedRef.current = { sessionId: null, tab: null };
    updateUrl(null, null);
  };

  // Handle session deletion
  const handleDeleteSession = async (sessionId: string) => {
    if (selectedSession?.id === sessionId) {
      closeSession();
      await new Promise(resolve => setTimeout(resolve, 100));
    }
    track({ name: "session_deleted", category: "user_action" });
    await deleteSession(sessionId);
  };

  // Handle new workspace on same project
  const handleNewWorkspaceSession = (sessionId: string) => {
    track({ name: "session_new_workspace_initiated", category: "user_action" });
    getSession(sessionId).then((session) => {
      openOmnibar(session?.repoRoot);
    }).catch(() => {
      openOmnibar();
    });
  };

  const handleCloneSession = useCallback((_sessionId: string) => {
    openInCreationMode();
  }, [openInCreationMode]);

  const handleNewSession = () => {
    openInCreationMode();
  };

  const handleUpdateTags = async (sessionId: string, tags: string[]) => {
    if (tags.length > 0) {
      track({ name: "session_tags_updated", category: "user_action" });
      await updateSession(sessionId, { tags });
    }
  };

  const handleSetRateLimitEnabled = useCallback(async (sessionId: string, enabled: boolean): Promise<void> => {
    track({ name: "session_rate_limit_updated", category: "user_action" });
    await updateSession(sessionId, { rateLimitEnabled: enabled });
  }, [updateSession, track]);

  const handleToggleAutonomousMode = useCallback(async (sessionId: string, enabled: boolean): Promise<void> => {
    track({ name: "session_autonomous_mode_updated", category: "user_action" });
    try {
      await updateSession(sessionId, { autonomousMode: enabled });
    } catch (err) {
      console.error("[page] toggleAutonomousMode failed:", err);
    }
  }, [updateSession, track]);

  const handleTogglePinned = useCallback(async (sessionId: string, pinned: boolean): Promise<void> => {
    track({ name: "session_pinned_updated", category: "user_action" });
    await (pinned ? pinSession(sessionId) : unpinSession(sessionId));
  }, [pinSession, unpinSession, track]);

  const handleToggleAutoApprove = useCallback(async (sessionId: string, enabled: boolean): Promise<void> => {
    track({ name: "session_auto_approve_updated", category: "user_action" });
    try {
      await updateSession(sessionId, { autoApprove: enabled });
    } catch (err) {
      console.error("[page] toggleAutoApprove failed:", err);
    }
  }, [updateSession, track]);

  const handleSteerAutonomousSession = useCallback(async (sessionId: string, message: string): Promise<boolean> => {
    track({ name: "session_autonomous_steer", category: "user_action" });
    const result = await updateSession(sessionId, { steerMessage: message });
    if (result === null) {
      addNotification({
        message: "Failed to send steering message — the session may not be running.",
        notificationType: "error",
        sessionId,
        sessionName: "",
      });
      return false;
    }
    return true;
  }, [updateSession, track, addNotification]);

  const handleSessionClick = (session: Session) => {
    sessionTriggerRef.current = document.activeElement as HTMLElement;
    if (typeof performance !== "undefined") {
      performance.mark("session:click");
    }
    setSelectedSession(session);
    setActiveTab("info");
    // Pre-mark the URL state so the URL-watching effect skips re-routing when
    // searchParams updates in response to this updateUrl call. "info" tab is not
    // added to the URL (filtered out), so the stored tabParam is null.
    lastUrlRoutedRef.current = { sessionId: session.id, tab: null };
    updateUrl(session.id, "info");
    // Also route the session to the currently-focused tiling pane
    setExternalAssignCounter((c) => c + 1);
    setExternalAssignSession({ sessionId: session.id, tab: "info" });
  };

  const handleTabChange = (tab: SessionDetailTab) => {
    setActiveTab(tab);
    if (selectedSession) {
      // Pre-mark the URL state before the URL changes so the URL-watching effect
      // does not re-route the session to a pane (which would show the picker).
      // "info" is not written to the URL, so its tabParam is null.
      lastUrlRoutedRef.current = { sessionId: selectedSession.id, tab: tab !== "info" ? tab : null };
      updateUrl(selectedSession.id, tab);
    }
  };

  // Story 3.2 — j/k keyboard navigation in session list
  // When no session is open, j/k move the focus index; Enter opens the focused session.
  // When a session is open, p/r/d act on the currently-open session.
  useKeyboard({
    // '?' is handled exclusively by CockpitShell's useShortcut to avoid dual-listener collision
    Escape: () => {
      if (deleteConfirmTarget) {
        setDeleteConfirmTarget(null);
      } else if (resumeTarget) {
        handleResumeCancel();
      } else if (selectedSession) {
        closeSession();
      }
    },
    "R": () => {
      if (!loading) {
        track({ name: "sessions_refreshed", category: "user_action" });
        listSessions();
      }
    },
    // j/k navigation (only when no modal is open)
    "j": () => {
      if (deleteConfirmTarget || resumeTarget) return;
      setFocusedSessionIndex(prev =>
        sessions.length === 0 ? -1 : Math.min(prev + 1, sessions.length - 1)
      );
    },
    // List/Board view toggle. useKeyboard's default ignoreElements (INPUT/TEXTAREA/SELECT)
    // already covers the instant-search box, so this never fires while typing "b" into it.
    "b": () => {
      if (deleteConfirmTarget || resumeTarget) return;
      setViewMode(viewMode === "board" ? "list" : "board");
    },
    "k": () => {
      if (deleteConfirmTarget || resumeTarget) return;
      setFocusedSessionIndex(prev =>
        sessions.length === 0 ? -1 : Math.max(prev - 1, 0)
      );
    },
    Enter: () => {
      if (deleteConfirmTarget || resumeTarget) return;
      if (!selectedSession && focusedSessionIndex >= 0 && sessions[focusedSessionIndex]) {
        handleSessionClick(sessions[focusedSessionIndex]);
      }
    },
    // p/r/d act on the open session
    "p": () => {
      if (selectedSession && !deleteConfirmTarget) {
        track({ name: "session_paused", category: "user_action" });
        pauseSession(selectedSession.id);
      }
    },
    "r": () => {
      if (selectedSession && !deleteConfirmTarget) {
        handleResumeRequest(selectedSession);
      }
    },
    "d": () => {
      if (selectedSession && !deleteConfirmTarget) {
        // document.activeElement is unreliable here: the effect above focuses
        // sessionDetailRef (the cockpit container) whenever selectedSession is
        // set, so by the time this fires, activeElement is that container, not
        // the row that was actually clicked. sessionTriggerRef still holds the
        // real opener; fall back to activeElement only for the deep-link case
        // where a session was selected without a click (no captured trigger).
        lastFocusBeforeDelete.current = sessionTriggerRef.current ?? (document.activeElement as HTMLElement);
        setDeleteConfirmTarget(selectedSession);
      }
    },
    // t — jump to terminal tab
    "t": () => {
      if (selectedSession && !deleteConfirmTarget) {
        handleTabChange("terminal");
      }
    },
  });

  const cockpitActions = useMemo(() => ({
    onSessionClick: handleSessionClick,
    onDeleteSession: handleDeleteSession,
    onPauseSession: pauseSession,
    onResumeSession: handleResumeRequest,
    onDirectResumeSession: handleDirectResume,
    onCloneSession: handleCloneSession,
    onNewWorkspaceSession: handleNewWorkspaceSession,
    onRenameSession: renameSession,
    onRestartSession: restartSession,
    onRetryNowSession: retrySession,
    onUpdateTags: handleUpdateTags,
    onNewSession: handleNewSession,
    onCreateCheckpoint: createCheckpoint,
    onListCheckpoints: listCheckpoints,
    onForkFromCheckpoint: forkSession,
    onSetRateLimitEnabled: handleSetRateLimitEnabled,
    onToggleAutonomousMode: handleToggleAutonomousMode,
    onTogglePinned: handleTogglePinned,
    onToggleAutoApprove: handleToggleAutoApprove,
    onSteerAutonomousSession: handleSteerAutonomousSession,
    onClearConversationState: clearConversationState,
    onListSessions: listSessions,
  }), [
    handleSessionClick, handleDeleteSession, pauseSession, handleResumeRequest,
    handleDirectResume, handleCloneSession, handleNewWorkspaceSession, renameSession,
    restartSession, retrySession, handleUpdateTags, handleNewSession, createCheckpoint,
    listCheckpoints, forkSession, handleSetRateLimitEnabled,
    handleToggleAutonomousMode, handleTogglePinned, handleToggleAutoApprove, handleSteerAutonomousSession, clearConversationState, listSessions,
  ]);

  return (
    <div className={styles.page}>
      {/* Unified tiling cockpit — session list and detail panels are both pane views */}
      <CockpitActionsProvider value={cockpitActions}>
        <SessionViewModeProvider value={{ viewMode, setViewMode }}>
          <WindowTabStrip
            ref={windowTabStripRef}
            windows={windows}
            currentWindowId={currentWindow.id}
            onSwitch={switchToWindow}
            onCreate={() => switchToWindow(createWindow())}
            onClose={(id) => {
              const next = closeWindow(id);
              if (id === currentWindow.id && next) switchToWindow(next);
            }}
            onRename={renameWindow}
          />
          {lookup ? (
            <SessionUnavailableCard
              variant={lookup.status === "failed" ? "failed" : "unavailable"}
              sessionLabel={lookup.sessionId.slice(0, 8)}
              notification={lookup.status === "not_found" ? lookupNotification : null}
              onOpenNotifications={() => router.push("/notifications")}
              onGoToSessions={() => {
                setLookup(null);
                updateUrl(null, null);
              }}
              onRetry={() => {
                const link = parseSessionDeepLink(searchParams);
                setLookup(null);
                fetchHiddenSessionFallback(link, true);
              }}
            />
          ) : (
            <div
              ref={sessionDetailRef}
              className={styles.cockpitContainer}
              tabIndex={-1}
              role="region"
              aria-label="Session cockpit"
              data-context="cockpit"
            >
              <PaneTilingContainer
                sessions={paneSessions}
                paneState={currentWindow.paneState}
                dispatch={paneDispatch}
                externalSessionAssign={externalAssignSession ? {
                  ...externalAssignSession,
                  version: externalAssignCounter,
                } : null}
              />
            </div>
          )}
        </SessionViewModeProvider>
      </CockpitActionsProvider>

      {/* Resume session modal */}
      {resumeTarget && (
        <ResumeSessionModal
          key={resumeTarget.id}
          session={resumeTarget}
          sessions={sessions}
          onConfirm={handleResumeConfirm}
          onCancel={handleResumeCancel}
          triggerRef={resumeTriggerRef}
        />
      )}

      {/* Delete confirmation modal (triggered by 'd' keyboard shortcut) */}
      {deleteConfirmTarget && (
        <DeleteSessionConfirmModal
          target={deleteConfirmTarget}
          dialogRef={deleteDialogRef}
          onCancel={() => setDeleteConfirmTarget(null)}
          onConfirm={handleDeleteSession}
        />
      )}
    </div>
  );
}

export default function Home() {
  return (
    <Suspense fallback={<SessionListSkeleton count={4} />}>
      <HomeContent />
    </Suspense>
  );
}
