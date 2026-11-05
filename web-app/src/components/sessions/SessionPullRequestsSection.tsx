"use client";

import { useRef, useState } from "react";
import type { Session } from "@/gen/session/v1/types_pb";
import { PullRequestList } from "@/components/shared/PullRequestList";
import { useSessionServiceContext } from "@/lib/contexts/SessionServiceContext";
import { useSessionMultiPR } from "@/lib/features/sessionMultiPr";
import { useSessionPullRequests } from "@/lib/hooks/useSessionPullRequests";
import { legacyPullRequest } from "@/lib/pr/legacyPullRequest";
import { CreatePullRequestModal } from "./CreatePullRequestModal";
import * as styles from "./SessionDetail.css";
import * as s from "./SessionPullRequestsSection.css";

/** Flag on: the list, its RPC hooks and the Create PR modal. Split out so the flag-off path needs none of them. */
function MultiPullRequestSection({ session }: { session: Session }) {
  const { pullRequests, holds, actions } = useSessionPullRequests(session);
  const { draftPullRequest, createPullRequest } = useSessionServiceContext();
  const [createOpen, setCreateOpen] = useState(false);
  const createTrigger = useRef<HTMLElement | null>(null);
  return (
    <div className={`${styles.infoItem} ${s.wide}`}>
      <PullRequestList
        pullRequests={pullRequests}
        holds={holds}
        actions={actions}
        onCreatePullRequest={() => setCreateOpen(true)}
        fallbackUrl={legacyPullRequest(session)?.url}
      />
      <CreatePullRequestModal
        session={session}
        isOpen={createOpen}
        onClose={() => setCreateOpen(false)}
        draftPullRequest={draftPullRequest}
        createPullRequest={createPullRequest}
        triggerRef={createTrigger}
      />
    </div>
  );
}

/**
 * The detail "Pull requests" section. With `session_multi_pr` reader:ui on it is the full list; off, it is
 * the legacy one-line PR link built from the single-PR scalars (rollback is instant).
 */
export function SessionPullRequestsSection({ session }: { session: Session }) {
  const multi = useSessionMultiPR();
  if (multi) return <MultiPullRequestSection session={session} />;

  const legacy = legacyPullRequest(session);
  if (!legacy?.url) return null;
  return (
    <div className={styles.infoItem}>
      <span className={styles.infoLabel}>GitHub PR:</span>
      <span className={styles.infoValue}>
        <a href={legacy.url} target="_blank" rel="noopener noreferrer" className={s.legacyLink}>{/* analytics-exempt */}
          #{legacy.number} {legacy.state && `(${legacy.state})`}
          {legacy.isDraft && " [Draft]"}
        </a>
      </span>
    </div>
  );
}
