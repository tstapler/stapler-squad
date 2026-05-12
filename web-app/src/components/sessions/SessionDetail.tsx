"use client";

import { useEffect } from "react";
import { Session } from "@/gen/session/v1/types_pb";
import { useSessionActions } from "@/lib/hooks/useSessionActions";
import { SessionVcsProvider } from "@/lib/contexts/SessionVcsContext";
import { prefetchVcsStatus } from "@/lib/hooks/useVcsStatus";
import { getApiBaseUrl } from "@/lib/config";
import { useAppSelector } from "@/lib/store";
import { selectAllSessions } from "@/lib/store/sessionsSlice";
import { SessionDetailView } from "./SessionDetailView";

export type SessionDetailTab = "terminal" | "diff" | "vcs" | "logs" | "info" | "files";

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
        backlogItemId={backlogItemId}
      />
    </SessionVcsProvider>
  );
}
