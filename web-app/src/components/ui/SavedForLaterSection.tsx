"use client";
// +feature: session:guidance-structured

import Link from "next/link";
import { useId, useState } from "react";
import type { PlainGuidanceRequest } from "@/lib/api/guidanceApi";
import { QUESTION_FORM_COPY as COPY, COPY as ROW_COPY } from "@/lib/copy/notificationCopy";
import {
  autoHandledBadge,
  autoHandledChevron,
  autoHandledChevronOpen,
  autoHandledHeader,
  autoHandledHeaderLeft,
  autoHandledItem,
  autoHandledList,
  autoHandledSection,
} from "./NotificationPanel.css";

export interface SavedForLaterSectionProps {
  rows: PlainGuidanceRequest[];
  /** Session title by uuid, from the history the panel already holds. */
  sessionNames?: Readonly<Record<string, string>>;
  onNavigate?: () => void;
}

const sessionOf = (r: PlainGuidanceRequest) => r.sessionUuid || r.askerSessionUuid || "";

/**
 * Answers waiting for a stopped or archived session to resume (design/ux.md 3.4): a read-only,
 * collapsed-by-default disclosure at the foot of the list. It is not counted in the bell badge;
 * Open session is the only way to resume (there is no discard in v1).
 */
export function SavedForLaterSection({ rows, sessionNames = {}, onNavigate }: SavedForLaterSectionProps) {
  const [open, setOpen] = useState(false);
  const listId = useId();
  if (rows.length === 0) return null;
  return (
    <div className={autoHandledSection} data-testid="saved-for-later">
      <button type="button" className={autoHandledHeader} aria-expanded={open} aria-controls={listId} onClick={() => setOpen((v) => !v)}>
        <span className={autoHandledHeaderLeft}>
          {COPY.savedForLater(rows.length)}
          <span className={autoHandledBadge} aria-hidden="true">
            {rows.length}
          </span>
        </span>
        <span className={`${autoHandledChevron} ${open ? autoHandledChevronOpen : ""}`} aria-hidden="true">
          ▼
        </span>
      </button>
      {open && (
        <ul id={listId} className={autoHandledList} data-testid="saved-for-later-list">
          {rows.map((r) => {
            const uuid = sessionOf(r);
            const archived = r.deliveryRoute === "read_only";
            return (
              <li key={r.id} className={autoHandledItem} data-testid={`saved-for-later-${r.id}`}>
                <span>
                  <strong>{sessionNames[uuid] || uuid.slice(0, 8) || "Session"}</strong> ({archived ? COPY.savedArchived : COPY.savedStopped}): {r.questionText} - {r.answer}
                </span>
                {uuid && (
                  <Link href={`/?session=${encodeURIComponent(uuid)}`} onClick={onNavigate}>
                    {ROW_COPY.openSession}
                  </Link>
                )}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
