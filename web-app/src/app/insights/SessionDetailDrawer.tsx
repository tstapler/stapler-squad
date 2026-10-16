// +feature: insights-dashboard
"use client";

import { useEffect, useRef } from "react";
import { createPortal } from "react-dom";
import Link from "next/link";
import type { SessionTokenSummary } from "@/gen/session/v1/insights_pb";
import type { BacklogIndexEntry } from "@/lib/hooks/useBacklogService";
import {
  overlay,
  drawer,
  drawerHeader,
  drawerTitle,
  sessionIdChip,
  closeButton,
  srOnly,
  openFullPageLink,
} from "./SessionDetailDrawer.css";
import { shortId } from "./insightsFormatters";
import { useSessionTurnTimeline } from "@/lib/hooks/useInsightsService";
import { SessionDetailContent } from "./SessionDetailContent";

interface Props {
  session: SessionTokenSummary | null;
  onClose: () => void;
  backlogEntry?: BacklogIndexEntry;
}

const FOCUSABLE_SELECTOR =
  'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])';

export function SessionDetailDrawer({ session, onClose, backlogEntry }: Props) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const closeButtonRef = useRef<HTMLButtonElement>(null);
  const previouslyFocusedRef = useRef<Element | null>(null);

  // Escape closes; Tab/Shift+Tab cycles within the dialog rather than
  // escaping to the rest of the page (this drawer is a plain positioned
  // <div role="dialog">, not a native <dialog>.showModal(), so nothing traps
  // focus without this — mirrors useDialogFocusTrap.ts's Tab-handling,
  // inlined here since that hook's mount-only initial-focus effect doesn't
  // fit this component's always-mounted/session-prop-driven open+close).
  useEffect(() => {
    if (!session) return;
    function handleKey(e: KeyboardEvent) {
      if (e.key === "Escape") {
        onClose();
        return;
      }
      if (e.key === "Tab" && dialogRef.current) {
        const focusable = Array.from(dialogRef.current.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR));
        if (focusable.length === 0) return;
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    }
    document.addEventListener("keydown", handleKey);
    return () => document.removeEventListener("keydown", handleKey);
  }, [session, onClose]);

  // Focus management (Epic 1.4, Story 1.4.2): move focus to the close button
  // on open, restore it to whatever had focus before opening on close. This
  // was entirely missing before — role="dialog"/aria-modal alone give a
  // screen reader no cue that focus should move.
  useEffect(() => {
    if (session) {
      previouslyFocusedRef.current = document.activeElement;
      closeButtonRef.current?.focus();
    } else if (previouslyFocusedRef.current instanceof HTMLElement) {
      previouslyFocusedRef.current.focus();
      previouslyFocusedRef.current = null;
    }
  }, [session]);

  const { turns } = useSessionTurnTimeline(session?.conversationId);

  if (!session || typeof document === "undefined") return null;

  const displayId = session.sessionId || session.conversationId;
  // Never a bare `?sessionId=` — an orphan session has an empty sessionId,
  // so this must fall back to conversationId (Epic 1.4, Story 1.4.4). Query
  // param, not a `/insights/session/[sessionId]` path segment, so cold
  // navigation resolves under `output: "export"` (see
  // src/app/insights/session-detail/page.tsx).
  const fullPageHref = `/insights/session-detail?sessionId=${encodeURIComponent(
    session.sessionId || session.conversationId
  )}`;

  const content = (
    <>
      <div className={overlay} onClick={onClose} aria-hidden="true" />
      <div
        ref={dialogRef}
        className={drawer}
        role="dialog"
        aria-modal="true"
        aria-label="Session details"
        aria-describedby="session-detail-description"
      >
        <div id="session-detail-description" className={srOnly}>
          Session token usage details including cost, model, tools used, and skill activations.
        </div>
        <div className={drawerHeader}>
          <div className={drawerTitle}>
            <span className={sessionIdChip}>{shortId(displayId)}</span>
            Session Details
          </div>
          <Link href={fullPageHref} className={openFullPageLink}>
            Open full page ↗
          </Link>
          <button
            type="button"
            ref={closeButtonRef}
            className={closeButton}
            onClick={onClose}
            aria-label="Close session details"
          >
            ×
          </button>
        </div>

        <SessionDetailContent session={session} backlogEntry={backlogEntry} turns={turns} />
      </div>
    </>
  );

  return createPortal(content, document.body);
}
