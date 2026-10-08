// +feature: unfinished-github-prs
"use client";

import { useId, useState } from "react";
import Link from "next/link";
import {
  LinkedSessionStatus,
  type LinkedSession,
  type UserPR,
} from "@/gen/session/v1/types_pb";
import { prAttention, type PRAttention } from "@/lib/unfinished/prAttention";
import { prKey } from "@/lib/unfinished/prOrdering";
import type { NudgeClient } from "@/lib/hooks/useNudgePR";
import { NudgeButton } from "./NudgeButton";
import * as styles from "./PRCard.css";

const cx = (...names: string[]) => names.join(" ");
const EXTERNAL = { target: "_blank", rel: "noopener noreferrer" } as const;

function ciChip(pr: UserPR, a: PRAttention): React.ReactNode {
  if (pr.isDraft) return null;
  if (a.failingChecks > 0) return null; // rendered by FailingChecks
  if (a.checksFailingUnlisted) {
    return (
      <span className={styles.chipError} data-testid="ci-failing-unlisted">
        CI failing (checks not listed)
      </span>
    );
  }
  const conclusion = pr.checkConclusion;
  if (conclusion === "success" || conclusion === "completed") {
    return <span className={styles.chipSuccess}>✓ CI passing</span>;
  }
  if (conclusion === "pending" || conclusion === "in_progress" || conclusion === "queued") {
    return <span className={styles.chipNeutral}>CI pending</span>;
  }
  return null;
}

function FailingChecks({ pr, count }: { pr: UserPR; count: number }) {
  const [open, setOpen] = useState(false);
  const listId = useId();
  const capped = pr.failingChecks.length >= 10;
  return (
    <>
      <button
        type="button"
        className={cx(styles.chipError, styles.chipInteractive)}
        aria-expanded={open}
        aria-controls={listId}
        onClick={() => setOpen((v) => !v)}
        data-testid="failing-checks-toggle"
      >
        CI: {count} failing
      </button>
      {open && (
        <ul id={listId} className={styles.checkList} data-testid="failing-checks-list">
          {pr.failingChecks.map((c) => (
            <li key={`${c.name}|${c.url}`}>
              {c.url ? (
                <a className={styles.checkLink} href={c.url} {...EXTERNAL}>
                  {c.name}
                </a>
              ) : (
                <span className={styles.checkLink}>{c.name}</span>
              )}
            </li>
          ))}
          {capped && (
            <li>
              <a className={styles.checkLink} href={`${pr.htmlUrl}/checks`} {...EXTERNAL}>
                and more on GitHub
              </a>
            </li>
          )}
        </ul>
      )}
    </>
  );
}

function ThreadsChip({ pr, a }: { pr: UserPR; a: PRAttention }) {
  const unknown = a.threadsUnknown || (!pr.detailsLoaded && a.unresolvedThreads === 0);
  if (unknown) return <span className={styles.chipNeutral}>? threads</span>;
  if (a.unresolvedThreads === 0) return null;
  return (
    <a
      className={cx(styles.chipWarning, styles.chipInteractive)}
      href={pr.htmlUrl}
      {...EXTERNAL}
    >
      {a.unresolvedThreads}
      {pr.unresolvedThreadsTruncated ? "+" : ""} unresolved
    </a>
  );
}

function sessionStatusText(status: LinkedSessionStatus): string {
  switch (status) {
    case LinkedSessionStatus.RUNNING:
      return "running";
    case LinkedSessionStatus.PAUSED:
      return "paused";
    case LinkedSessionStatus.STOPPED:
      return "stopped";
    default:
      return "status unknown";
  }
}

const lastActive = (s: LinkedSession) => Number(s.lastActiveAt?.seconds ?? 0n);

/** Server-linked sessions, most recently active first; falls back to bare `sessionIds`. */
function linkedSessionsOf(pr: UserPR): Pick<LinkedSession, "sessionId" | "status" | "lastActiveAt" | "steerReady">[] {
  if (pr.linkedSessions.length > 0) {
    return [...pr.linkedSessions].sort((x, y) => lastActive(y) - lastActive(x));
  }
  return pr.sessionIds.map((sessionId) => ({
    sessionId,
    status: LinkedSessionStatus.UNSPECIFIED,
    lastActiveAt: undefined,
    steerReady: false,
  }));
}

interface PRCardProps {
  pr: UserPR;
  /** True when the list spans more than one host or account, so cards must disambiguate. */
  showHostAccount?: boolean;
  /** Test seam for the nudge RPC; defaults to the shared Connect client. */
  nudgeClient?: NudgeClient;
  /** Called when the server reports nothing left to fix, so the owner can refetch PR data. */
  onNothingToFix?: () => void;
}

export function PRCard({ pr, showHostAccount = false, nudgeClient, onNothingToFix }: PRCardProps) {
  const a = prAttention(pr);
  const sessions = linkedSessionsOf(pr);
  const conflictChip = a.mergeConflict
    ? "Merge conflict"
    : a.conflictUnknown && !pr.isDraft && !a.needsAttention
      ? "? conflict"
      : null;

  return (
    <div className={styles.prCard} data-testid="github-pr-card" data-pr-key={prKey(pr)}>
      <div className={styles.prHeader}>
        <a
          className={styles.prTitle}
          href={pr.htmlUrl}
          target="_blank"
          rel="noreferrer"
          aria-label={`PR #${pr.number}: ${pr.title}`}
        >
          {pr.title}
        </a>
        <div className={styles.chips}>
          {pr.isDraft && <span className={styles.chipDraft}>Draft</span>}
          {!pr.isDraft && a.failingChecks > 0 && <FailingChecks pr={pr} count={a.failingChecks} />}
          {ciChip(pr, a)}
          {a.changesRequested && <span className={styles.chipError}>Changes requested</span>}
          {pr.approvedCount > 0 && !a.changesRequested && (
            <span className={styles.chipSuccess}>{pr.approvedCount} approved</span>
          )}
          {!pr.isDraft && <ThreadsChip pr={pr} a={a} />}
          {conflictChip && (
            <span className={a.mergeConflict ? styles.chipError : styles.chipNeutral}>
              {conflictChip}
            </span>
          )}
        </div>
      </div>
      <div className={styles.prMeta}>
        <span className={styles.prRepo}>
          {pr.owner}/{pr.repo} #{pr.number}
        </span>
        {showHostAccount && (
          <span className={styles.hostAccountLabel} data-testid="pr-host-account">
            {pr.host || "github.com"} - {pr.accountLogin}
          </span>
        )}
        <span className={styles.prBranch}>
          {pr.headRef} → {pr.baseRef}
        </span>
        {pr.localWorktreePath && (
          <span className={styles.worktreeLink} title={pr.localWorktreePath}>
            {pr.localWorktreePath.split("/").slice(-2).join("/")}
          </span>
        )}
      </div>
      {a.needsAttention && !a.nudgeable && a.changesRequested && (
        <p className={styles.changesRequestedNote}>
          A reviewer asked for changes. Open the PR on GitHub to read them; no automatic request is
          available.
        </p>
      )}
      {sessions.length > 0 && (
        <ul className={styles.sessionList} aria-label="Linked sessions">
          {sessions.map((s, i) => (
            <li key={s.sessionId} className={styles.sessionRow}>
              <span className={styles.sessionName}>{s.sessionId}</span>
              <span className={styles.sessionStatus}>
                ({sessionStatusText(s.status)}
                {i === 0 ? ", default" : ""})
              </span>
              <Link
                href={`/?session=${encodeURIComponent(s.sessionId)}`}
                className={styles.openSessionButton}
                aria-label={`Open session ${s.sessionId}`}
                data-testid="open-session-link"
              >
                Open session
              </Link>
            </li>
          ))}
        </ul>
      )}
      {pr.linkedSessions.length > 0 && (
        <NudgeButton pr={pr} sessions={sessions} nudgeable={a.nudgeable} client={nudgeClient} onNothingToFix={onNothingToFix} />
      )}
      <div className={styles.prActions}>
        {a.needsAttention && !a.nudgeable && a.changesRequested && (
          <a
            className={styles.createSessionButton}
            href={pr.htmlUrl}
            {...EXTERNAL}
            data-testid="open-pr-on-github"
          >
            Open PR on GitHub
          </a>
        )}
        {sessions.length === 0 && (
          <>
            <span className={styles.noSessionText}>No session on this branch</span>
            <Link
              href={`/?pr=${encodeURIComponent(pr.htmlUrl)}`}
              className={styles.createSessionButton}
              data-testid="create-session-button"
            >
              + Session
            </Link>
          </>
        )}
      </div>
    </div>
  );
}
