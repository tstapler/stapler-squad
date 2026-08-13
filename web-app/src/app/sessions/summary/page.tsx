"use client";
// +feature: session-summary-standalone-route

import { Suspense } from "react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { usePageView } from "@/lib/analytics/usePageView";
import { SessionSummaryPanel } from "@/components/sessions/SessionSummaryPanel";
import * as styles from "./page.css";

/**
 * Durable standalone route for a session's summary (Epic 3.3, requirements.md
 * FR-3 / AC-3 / AC-7): retrievable via a stable `sessionId` even after the
 * live `Session` has been deleted and the server has restarted. Deliberately
 * has no dependency on the Redux sessions list (`useAppSelector(selectAllSessions)`)
 * — SessionSummaryPanel fetches directly via `useSessionSummary`, which calls
 * `GetSessionSummary` and needs nothing but the sessionId.
 *
 * Uses a `?sessionId=` query param rather than a `/[sessionId]/` path segment:
 * this app builds with `output: "export"` (static export embedded in the Go
 * binary, served with an SPA fallback — see server/middleware/static.go), and
 * session IDs aren't known at build time, so a dynamic path segment has no
 * pre-renderable params under static export. A query param needs no
 * per-value pre-rendering at all.
 */
function SessionSummaryPageInner() {
  usePageView();
  const searchParams = useSearchParams();
  const sessionId = searchParams.get("sessionId");

  return (
    <main id="main-content" className={styles.page}>
      <Link href="/" className={styles.backLink}>
        ← Back
      </Link>
      <h1 className={styles.title}>Session Summary</h1>
      {sessionId && <SessionSummaryPanel sessionId={sessionId} />}
    </main>
  );
}

export default function SessionSummaryPage() {
  return (
    <Suspense>
      <SessionSummaryPageInner />
    </Suspense>
  );
}
