"use client";

import Link from "next/link";
import { useParams } from "next/navigation";
import { usePageView } from "@/lib/analytics/usePageView";
import { SessionSummaryPanel } from "@/components/sessions/SessionSummaryPanel";
import * as styles from "./page.css";

/**
 * Durable standalone route for a session's summary (Epic 3.3, requirements.md
 * FR-3 / AC-3 / AC-7): retrievable via a stable `sessionId` URL even after the
 * live `Session` has been deleted and the server has restarted. Deliberately
 * has no dependency on the Redux sessions list (`useAppSelector(selectAllSessions)`)
 * — SessionSummaryPanel fetches directly via `useSessionSummary`, which calls
 * `GetSessionSummary` and needs nothing but the sessionId.
 */
export default function SessionSummaryPage() {
  usePageView();
  const params = useParams<{ sessionId: string }>();
  const sessionId = Array.isArray(params.sessionId) ? params.sessionId[0] : params.sessionId;

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
