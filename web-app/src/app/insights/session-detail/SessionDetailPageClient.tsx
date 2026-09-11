// +feature: insights-session-detail-route
"use client";

import { useEffect, useRef } from "react";
import Link from "next/link";
import { useSessionDetail, useSessionTurnTimeline } from "@/lib/hooks/useInsightsService";
import { useBacklogSessionIndex } from "@/lib/hooks/useBacklogService";
import { SessionDetailContent } from "../SessionDetailContent";
import { shortId } from "../insightsFormatters";
import { main, backLink, heading } from "./page.css";

interface Props {
  sessionId: string;
}

/**
 * Client component for the deep-linkable /insights/session-detail?sessionId=
 * route (Epic 1.4, Story 1.4.3). Fetches independently of any dashboard
 * state — a cold direct navigation (bookmark, refresh, shared link) must
 * render correctly with no parent InsightsDashboard mounted.
 */
export function SessionDetailPageClient({ sessionId }: Props) {
  const { summary, loading, error } = useSessionDetail(sessionId);
  const { turns } = useSessionTurnTimeline(summary?.conversationId);
  const { index: backlogIndex } = useBacklogSessionIndex();

  const headingRef = useRef<HTMLHeadingElement>(null);

  // Route mount moves focus to the heading (Epic 1.4, Story 1.4.3d) — a
  // route gets no free role="dialog" AT signal the way the modal does, so
  // this is the only cue a screen reader gets that content changed.
  useEffect(() => {
    headingRef.current?.focus();
  }, []);

  const notFound = !loading && !error && !summary;

  return (
    <main id="main-content" className={main}>
      <Link href="/insights" className={backLink}>
        ← Back to dashboard
      </Link>
      <h1 ref={headingRef} tabIndex={-1} className={heading}>
        Session Details{summary ? `: ${shortId(summary.sessionId || summary.conversationId)}` : ""}
      </h1>

      {loading && <p role="status">Loading session…</p>}

      {!loading && error && (
        <div role="alert">
          <p>Couldn&apos;t load session: {error}</p>
        </div>
      )}

      {notFound && (
        <div data-testid="session-not-found">
          <p>Session not found.</p>
        </div>
      )}

      {!loading && !error && summary && (
        <SessionDetailContent
          session={summary}
          backlogEntry={summary.sessionId ? backlogIndex.get(summary.sessionId) : undefined}
          turns={turns}
        />
      )}
    </main>
  );
}
