// +feature: notification-background-activity
"use client";

import Link from "next/link";
import { formatRelativeTime } from "@/lib/utils/datetime";
import type { ActivityRow } from "@/lib/utils/backgroundActivity";
import { action, failureStrip, failureStripItem } from "./BackgroundActivity.css";

export const STRIP_NAMED_MAX = 3;

interface Props {
  failures: readonly ActivityRow[];
  hrefFor: (row: ActivityRow) => string;
  onShow: (row: ActivityRow) => void;
  onOpen: (row: ActivityRow) => void;
}

const verb = (row: ActivityRow) => (row.state === "exception_needs_human" ? "needs attention" : "failed");

/** Names each new failure (at most 3, then a count) so it is never only a number in an "N new" pill. */
export function BackgroundFailureStrip({ failures, hrefFor, onShow, onOpen }: Props) {
  if (failures.length === 0) return null;
  const named = failures.slice(0, STRIP_NAMED_MAX);
  const more = failures.length - named.length;
  return (
    <ul className={failureStrip} data-testid="background-failure-strip" aria-label="New background failures">
      {named.map((r) => (
        <li key={r.key} className={failureStripItem} data-testid="background-failure-strip-item">
          <span>
            {r.sessionTitle} {verb(r)}
            {r.timestampMs > 0 ? ` ${formatRelativeTime(r.timestampMs)}` : ""}
          </span>
          {
            // analytics-exempt
            <button type="button" className={action} onClick={() => onShow(r)}>
              Show
            </button>
          }
          <Link href={hrefFor(r)} className={action} onClick={() => onOpen(r)}>
            Open
          </Link>
        </li>
      ))}
      {more > 0 && <li className={failureStripItem}>{more} more {more === 1 ? "failure" : "failures"}</li>}
    </ul>
  );
}
