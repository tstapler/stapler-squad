"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { PRComment } from "@/gen/session/v1/types_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import { formatRelativeTime } from "@/lib/utils/datetime";
import * as styles from "./VcsWidgetComments.css";

interface VcsWidgetCommentsProps {
  owner: string;
  repo: string;
  prNumber: number;
  /**
   * `GetPRComments` is keyed by session ID, not owner/repo/prNumber (see
   * `GetPRCommentsRequest`) — required for the fetch. owner/repo/prNumber
   * are used to build each comment's "View on GitHub" link.
   */
  sessionId: string;
}

type LoadState = "idle" | "loading" | "loaded" | "error";

/**
 * PR comments section, collapsed by default. Fetches lazily on first expand,
 * triggered by the child mounting (CollapsibleGroup unmounts/remounts
 * `Accordion.Content` on collapse/expand — see Collapsible.tsx) since grouped
 * mode makes `onExpandedChange` inert.
 */
export function VcsWidgetComments({ owner, repo, prNumber, sessionId }: VcsWidgetCommentsProps) {
  const [comments, setComments] = useState<PRComment[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("idle");
  const fetchedRef = useRef(false);
  // Guards against a setState after unmount: queue navigation remounts this
  // component (keyed by sessionId in VcsWidget.tsx) while a fetch from the
  // previous session may still be in flight.
  const unmountedRef = useRef(false);
  useEffect(() => {
    return () => {
      unmountedRef.current = true;
    };
  }, []);

  const fetchComments = useCallback(() => {
    if (fetchedRef.current) return;
    fetchedRef.current = true;
    setLoadState("loading");
    const client = createClient(SessionService, getConnectTransport());
    client
      .getPRComments({ id: sessionId })
      .then((response) => {
        if (unmountedRef.current) return;
        setComments(response.comments ?? []);
        setLoadState("loaded");
      })
      .catch((err) => {
        if (unmountedRef.current) return;
        console.error("[VcsWidgetComments] failed to load PR comments", err);
        setLoadState("error");
      });
  }, [sessionId]);

  return (
    <CollapsibleSection sectionKey="pr-comments" title="Comments" defaultExpanded={false}>
      <VcsWidgetCommentsBody
        owner={owner}
        repo={repo}
        prNumber={prNumber}
        comments={comments}
        loadState={loadState}
        onMount={fetchComments}
      />
    </CollapsibleSection>
  );
}

interface VcsWidgetCommentsBodyProps {
  owner: string;
  repo: string;
  prNumber: number;
  comments: PRComment[];
  loadState: LoadState;
  onMount: () => void;
}

function VcsWidgetCommentsBody({
  owner,
  repo,
  prNumber,
  comments,
  loadState,
  onMount,
}: VcsWidgetCommentsBodyProps) {
  // Fires once per mount of this subtree, which only mounts while the
  // section is expanded — see the module doc comment above for why.
  useEffect(() => {
    onMount();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  if (loadState === "idle" || loadState === "loading") {
    return <p className={styles.status}>Loading…</p>;
  }
  if (loadState === "error") {
    return <p className={styles.status}>Failed to load comments</p>;
  }
  if (comments.length === 0) {
    return <p className={styles.status}>No comments yet.</p>;
  }

  return (
    <ul className={styles.list}>
      {comments.map((comment) => (
        <li key={comment.id.toString()} className={styles.comment}>
          <div className={styles.commentMeta}>
            <span className={styles.author}>{comment.author}</span>
            {comment.createdAt && (
              <span className={styles.timestamp}>
                {formatRelativeTime(timestampDate(comment.createdAt).getTime())}
              </span>
            )}
          </div>
          {/* Plain JSX text interpolation — auto-escaped, XSS-safe (mirrors
              VcsWidgetReviewFeedback's body rendering). */}
          <p className={styles.body}>{comment.body}</p>
          <a
            href={`https://github.com/${owner}/${repo}/pull/${prNumber}#${
              comment.isReview ? "discussion_r" : "issuecomment-"
            }${comment.id}`}
            target="_blank"
            rel="noreferrer"
            className={styles.viewLink}
          >
            View on GitHub ↗
          </a>
        </li>
      ))}
    </ul>
  );
}
