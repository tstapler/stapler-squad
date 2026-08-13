"use client";
// +feature: ui:memory-pressure-callout

import { useState, useCallback } from "react";
import { Session, SessionStatus } from "@/gen/session/v1/types_pb";
import { useSystemMemory } from "@/lib/contexts/SystemMemoryContext";
import * as styles from "./MemoryPressureCallout.css";

interface MemoryPressureCalloutProps {
  sessions: Session[];
  onHibernate: (sessionId: string) => void;
}

function getMB(session: Session): number {
  return Number(session.estimatedSavingsMb ?? 0n);
}

function isActive(session: Session): boolean {
  return (session.status as number) === (SessionStatus.ACTIVE as number);
}

/**
 * Shows the top-3 active sessions by estimated memory savings when the system
 * is under memory pressure. Includes a bulk "Hibernate all recommended" action.
 * Dismissible per-session via sessionStorage.
 */
export function MemoryPressureCallout({ sessions, onHibernate }: MemoryPressureCalloutProps) {
  const { isUnderPressure } = useSystemMemory();
  const [dismissed, setDismissed] = useState<Set<string>>(() => {
    try {
      const raw = sessionStorage.getItem("memory-pressure-dismissed");
      return raw ? new Set(JSON.parse(raw) as string[]) : new Set();
    } catch {
      return new Set();
    }
  });

  const dismiss = useCallback((id: string) => {
    setDismissed((prev) => {
      const next = new Set(prev);
      next.add(id);
      try {
        sessionStorage.setItem("memory-pressure-dismissed", JSON.stringify([...next]));
      } catch { /* storage may be unavailable */ }
      return next;
    });
  }, []);

  if (!isUnderPressure) return null;

  const candidates = sessions
    .filter((s) => isActive(s) && !dismissed.has(s.id) && getMB(s) > 0)
    .sort((a, b) => getMB(b) - getMB(a))
    .slice(0, 3);

  if (candidates.length === 0) return null;

  const handleHibernateAll = () => {
    for (const s of candidates) {
      onHibernate(s.id);
      dismiss(s.id);
    }
  };

  return (
    <div className={styles.callout} role="alert" aria-live="polite">
      <div className={styles.header}>
        <span className={styles.title}>
          ⚠ Memory pressure — idle sessions are consuming RAM
        </span>
      </div>

      <div className={styles.sessionList}>
        {candidates.map((s) => {
          const mb = getMB(s);
          return (
            <div key={s.id} className={styles.sessionRow}>
              <span className={styles.sessionName} title={s.title}>{s.title}</span>
              {mb > 0 && (
                <span className={styles.savings}>saves ~{mb} MB</span>
              )}
              <button
                className={styles.hibernateBtn}
                onClick={() => { onHibernate(s.id); dismiss(s.id); }}
                title={`Hibernate ${s.title}`}
              >
                Hibernate
              </button>
              <button
                className={styles.dismissAll}
                onClick={() => dismiss(s.id)}
                aria-label={`Dismiss recommendation for ${s.title}`}
                title="Dismiss"
              >
                ✕
              </button>
            </div>
          );
        })}
      </div>

      <div className={styles.bulkAction}>
        <button className={styles.hibernateAllBtn} onClick={handleHibernateAll}>
          Hibernate all recommended
        </button>
      </div>
    </div>
  );
}
