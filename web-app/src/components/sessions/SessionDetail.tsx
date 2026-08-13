"use client";

import { useEffect } from "react";
import dynamic from "next/dynamic";
import { Session } from "@/gen/session/v1/types_pb";
import { useSessionActions } from "@/lib/hooks/useSessionActions";
import { SessionVcsProvider } from "@/lib/contexts/SessionVcsContext";
import { prefetchVcsStatus } from "@/lib/hooks/useVcsStatus";
import { getApiBaseUrl } from "@/lib/config";
import { useAppSelector } from "@/lib/store";
import { selectAllSessions } from "@/lib/store/sessionsSlice";

// Dynamically import SessionDetailView (and its heavy transitive deps: CodeMirror,
// XtermTerminal, syntax-highlight packs, WASM) so they are NOT in the initial bundle.
// This defers ~2MB of JS parse/eval until the user actually opens a session.
const SessionDetailView = dynamic(
  () => import("./SessionDetailView").then((m) => ({ default: m.SessionDetailView })),
  {
    ssr: false,
    loading: () => (
      <div style={{ padding: "20px", textAlign: "center", color: "var(--text-secondary)" }}>
        Loading...
      </div>
    ),
  }
);

export type SessionDetailTab = "terminal" | "diff" | "vcs" | "logs" | "info" | "files" | "browser" | "artifacts" | "summary";

interface SessionDetailProps {
  session: Session;
  onClose: () => void;
  onFullscreenChange?: (isFullscreen: boolean) => void;
  onTabChange?: (tab: SessionDetailTab) => void;
  initialTab?: SessionDetailTab;
  /** When true, hides the title header and tab strip — caller (e.g. PaneHeader) owns those. */
  embedded?: boolean;
  onNext?: () => void;
  onPrevious?: () => void;
  showNavigation?: boolean;
  onApprovalResolved?: () => void;
  onDismissFromQueue?: () => void;
  queuePosition?: number;
  queueTotal?: number;
  /** Name of the next session in queue — shown as a label under the → button. */
  nextSessionName?: string;
  /** Name of the previous session in queue — shown as a label under the ← button. */
  previousSessionName?: string;
  /** Called when user clicks the back-in-history button. */
  onBack?: () => void;
  /** Whether back navigation is available. */
  canGoBack?: boolean;
  /** Backlog item ID to display in right-side panel. If provided, shows BacklogItemPanel. */
  backlogItemId?: string;
}

export function SessionDetail({
  session,
  onClose,
  onFullscreenChange,
  onTabChange,
  initialTab = "info",
  embedded = false,
  onNext,
  onPrevious,
  showNavigation = false,
  onApprovalResolved,
  onDismissFromQueue,
  queuePosition,
  queueTotal,
  nextSessionName,
  previousSessionName,
  onBack,
  canGoBack,
  backlogItemId,
}: SessionDetailProps) {
  const actions = useSessionActions(session.id);
  const allSessions = useAppSelector(selectAllSessions);

  // Prefetch VCS data as soon as a session is selected so tabs load instantly.
  useEffect(() => {
    const baseUrl = getApiBaseUrl();
    prefetchVcsStatus(session.id, baseUrl);
  }, [session.id]);

  return (
    <SessionVcsProvider sessionId={session.id} baseUrl={getApiBaseUrl()}>
      <SessionDetailView
        session={session}
        allSessions={allSessions}
        actions={actions}
        onClose={onClose}
        onFullscreenChange={onFullscreenChange}
        onTabChange={onTabChange}
        initialTab={initialTab}
        embedded={embedded}
        onNext={onNext}
        onPrevious={onPrevious}
        showNavigation={showNavigation}
        onApprovalResolved={onApprovalResolved}
        onDismissFromQueue={onDismissFromQueue}
        queuePosition={queuePosition}
        queueTotal={queueTotal}
        nextSessionName={nextSessionName}
        previousSessionName={previousSessionName}
        onBack={onBack}
        canGoBack={canGoBack}
        backlogItemId={backlogItemId}
      />
    </SessionVcsProvider>
  );
}
