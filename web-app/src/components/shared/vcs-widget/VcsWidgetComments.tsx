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
 * PR comments section, collapsed by default. Fetches lazily on first expand
 * only — never on mount. Because this section renders inside VcsWidget's
 * shared CollapsibleGroup, `CollapsibleSection`'s `onExpandedChange` prop is
 * inert there (grouped mode only speaks through the group's own
 * value/onValueChange — see Collapsible.tsx), so the fetch trigger instead
 * lives in a `useEffect` on the section's children: the group unmounts
 * `Accordion.Content` on collapse and remounts it on expand, so mounting is
 * a reliable "just expanded" signal. The `fetchedRef` guard lives on this
 * always-mounted wrapper (not the child that gets unmounted) so a
 * collapse/re-expand cycle does not refetch.
 */
export function VcsWidgetComments({ owner, repo, prNumber, sessionId }: VcsWidgetCommentsProps) {
  const [comments, setComments] = useState<PRComment[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("idle");
  const fetchedRef = useRef(false);

  const fetchComments = useCallback(() => {
    if (fetchedRef.current) return;
    fetchedRef.current = true;
    setLoadState("loading");
    const client = createClient(SessionService, getConnectTransport());
    client
      .getPRComments({ id: sessionId })
      .then((response) => {
        setComments(response.comments ?? []);
        setLoadState("loaded");
      })
      .catch((err) => {
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
        <li key={comment.id} className={styles.comment}>
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
