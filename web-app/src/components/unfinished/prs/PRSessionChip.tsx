"use client";

import { useEffect, useId, useRef, useState } from "react";
import { Link } from "@/lib/nav";
import { routes } from "@/lib/routes";
import type { Session } from "@/gen/session/v1/types_pb";
import * as styles from "./PRSessionChip.css";

const sessionLabel = (s: Session) => s.title || s.id;

function SessionLink({ session, className, label, children }: { session: Session; className: string; label?: string; children: React.ReactNode }) {
  return (
    <Link href={routes.sessionDetail(session.id)} className={className} aria-label={label}>
      {children}
    </Link>
  );
}

/**
 * Muted chip on an Up Next PR card naming the sessions that track the PR (ux.md S6): one session reads
 * "session: name"; two or more read "N sessions" and open a menu of links (Esc closes, focus returns).
 */
export function PRSessionChip({ sessions }: { sessions: readonly Session[] }) {
  const [open, setOpen] = useState(false);
  const wrapRef = useRef<HTMLSpanElement>(null);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const menuId = useId();

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (wrapRef.current && !wrapRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    return () => document.removeEventListener("mousedown", onDown);
  }, [open]);

  if (sessions.length === 0) return null;
  if (sessions.length === 1) {
    return (
      <span className={styles.wrap} data-testid="pr-session-chip">
        <SessionLink session={sessions[0]} className={styles.chip}>session: {sessionLabel(sessions[0])}</SessionLink>
      </span>
    );
  }

  return (
    <span
      ref={wrapRef}
      className={styles.wrap}
      data-testid="pr-session-chip"
      onKeyDown={(e) => {
        if (e.key !== "Escape" || !open) return;
        e.stopPropagation();
        setOpen(false);
        triggerRef.current?.focus();
      }}
    >
      <button
        ref={triggerRef}
        type="button"
        className={styles.chip}
        aria-expanded={open}
        aria-controls={menuId}
        onClick={() => setOpen((v) => !v)}
        data-testid="pr-session-chip-menu-button"
      >{/* analytics-exempt */}
        {sessions.length} sessions
      </button>
      {open && (
        <ul id={menuId} className={styles.menu} aria-label="Sessions tracking this PR" data-testid="pr-session-chip-menu">
          {sessions.map((s) => (
            <li key={s.id}>
              <SessionLink session={s} className={styles.menuLink} label={`Open session ${sessionLabel(s)}`}>{sessionLabel(s)}</SessionLink>
            </li>
          ))}
        </ul>
      )}
    </span>
  );
}
