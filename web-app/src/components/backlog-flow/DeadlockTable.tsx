"use client";

import { useState } from "react";
import { AppLink } from "@/components/ui/AppLink";
import { EstimatedValue } from "@/components/ui/EstimatedValue";
import { srOnly } from "@/components/ui/LiveRegion.css";
import type { FlowDeadlockPage, FlowDeadlockRow } from "@/gen/session/v1/backlog_flow_pb";
import * as styles from "./flow.css";

/**
 * Owner decision (2026-11-05): the heading stays "Review-fail streaks (heuristic)" until the
 * Story 6.1.1 hand-labeled hit rate reaches 70%. Flip this one constant to rename it; no key
 * or `estimated` tag changes.
 */
export const DEADLOCK_PROMOTED = false;

const ESTIMATED_TITLE = "Heuristic ranking of review-fail streaks, not a confirmed deadlock (see Data notes)";
const HEADING_PROMOTED = "Deadlock candidates";
const HEADING_HEURISTIC = "Review-fail streaks (heuristic)";
const DEFAULT_VISIBLE = 10;
const SHORT_ID_LENGTH = 8;
const OUTCOME_WORDS: Record<string, string> = { F: "fail", R: "partial", U: "unverifiable", P: "pass" };

interface DeadlockTableProps {
  page: FlowDeadlockPage;
  /** The server's weak-filter sentence, shown beside the list. */
  weakFilter: string;
  /** True when the section lists the heuristic code in `estimated[]`. */
  estimated: boolean;
  promoted?: boolean;
}

const spellSequence = (sequence: string): string => [...sequence].map((letter) => OUTCOME_WORDS[letter] ?? "unknown").join(", ");

function criterionText(row: FlowDeadlockRow): string {
  return row.hasRepeatedCriterion ? `yes (c${row.repeatedCriterion})` : "no";
}

function Row({ row, showClaimed }: { row: FlowDeadlockRow; showClaimed: boolean }) {
  return (
    <tr>
      <th scope="row" className={styles.cell}>
        {row.title} <span className={styles.muted}>{row.itemId.slice(0, SHORT_ID_LENGTH)}</span>
      </th>
      <td className={styles.cell}>
        <span aria-label={spellSequence(row.sequence)}>{row.sequence}</span>
      </td>
      <td className={styles.cell}>{row.resolution}</td>
      <td className={styles.cell}>{row.runLength}</td>
      <td className={styles.cell}>{criterionText(row)}</td>
      <td className={styles.cell}>{row.diffUnchangedPairs > 0 ? `yes (${row.diffUnchangedPairs})` : "no"}</td>
      {showClaimed && <td className={styles.cell}>{row.agentClaimedDone ? "yes" : "no"}</td>}
      <td className={styles.cell}>
        <AppLink href={`?item=${row.itemId}`} className={styles.link}>
          Open item<span className={srOnly}> {row.title}</span>
        </AppLink>
      </td>
    </tr>
  );
}

const COLUMNS = ["Item", "Sequence", "Resolution", "Run", "Same criterion", "Diff unchanged"] as const;

/** Top review-fail streaks ranked by run length; a weak filter, so named for what it can prove. */
export function DeadlockTable({ page, weakFilter, estimated, promoted = DEADLOCK_PROMOTED }: DeadlockTableProps) {
  const [showAll, setShowAll] = useState(false);
  const [showClaimed, setShowClaimed] = useState(false);
  const heading = promoted ? HEADING_PROMOTED : HEADING_HEURISTIC;
  const ranked = [...page.items].sort((a, b) => b.runLength - a.runLength);
  const visible = showAll ? ranked : ranked.slice(0, DEFAULT_VISIBLE);
  const total = Math.max(page.totalCount, ranked.length);

  return (
    <div className={styles.block}>
      <h3 className={styles.subheading}>{heading}</h3>
      {ranked.length === 0 ? (
        <p>No review-fail streaks in this window</p>
      ) : (
        <>
          <p className={styles.muted}>
            {estimated ? <EstimatedValue title={ESTIMATED_TITLE}>ranked by run length</EstimatedValue> : "ranked by run length"}
            {" "}- weak filter: {weakFilter}.
          </p>
          <label className={styles.secondary}>
            <input type="checkbox" checked={showClaimed} onChange={(event) => setShowClaimed(event.target.checked)} /> Show agent-claimed-done (info only)
          </label>
          <div className={styles.scrollRegion} role="region" aria-label={heading} tabIndex={0}>
            <table className={styles.table}>
              <caption className={styles.caption}>{heading}, longest run first</caption>
              <thead>
                <tr>
                  {COLUMNS.map((column) => (
                    <th key={column} scope="col" className={styles.cell}>
                      {column}
                    </th>
                  ))}
                  {showClaimed && (
                    <th scope="col" className={styles.cell}>
                      Agent claimed done
                    </th>
                  )}
                  <th scope="col" className={styles.cell}>
                    Action
                  </th>
                </tr>
              </thead>
              <tbody>
                {visible.map((row) => (
                  <Row key={row.itemId} row={row} showClaimed={showClaimed} />
                ))}
              </tbody>
            </table>
          </div>
          <div className={styles.row}>
            <span className={styles.muted}>
              Showing {visible.length} of {total}
            </span>
            {!showAll && ranked.length > DEFAULT_VISIBLE && (
              <button type="button" className={styles.textButton} onClick={() => setShowAll(true)}>
                Show all
              </button>
            )}
          </div>
        </>
      )}
    </div>
  );
}
