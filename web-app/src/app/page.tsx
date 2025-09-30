"use client";

import { SessionList } from "@/components/sessions/SessionList";
import { useSessionService } from "@/lib/hooks/useSessionService";
import styles from "./page.module.css";

export default function Home() {
  const {
    sessions,
    loading,
    error,
    deleteSession,
    pauseSession,
    resumeSession,
  } = useSessionService({
    baseUrl: "http://localhost:8543",
    autoWatch: true,
  });

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <h1 className={styles.title}>Claude Squad Sessions</h1>
      </header>
      <main className={styles.main}>
        {loading && <div>Loading sessions...</div>}
        {error && <div className={styles.error}>Error: {error.message}</div>}
        {!loading && !error && (
          <SessionList
            sessions={sessions}
            onDeleteSession={deleteSession}
            onPauseSession={pauseSession}
            onResumeSession={resumeSession}
          />
        )}
      </main>
    </div>
  );
}
