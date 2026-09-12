"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import { SessionService } from "@/gen/session/v1/session_pb";
import type { PRComment } from "@/gen/session/v1/types_pb";
import { getConnectTransport } from "@/lib/api/transport";
import { CollapsibleSection } from "@/components/ui/Collapsible";
import { useAnalytics } from "@/lib/contexts/AnalyticsContext";
import { formatRelativeTime } from "@/lib/utils/datetime";
import { getGitHubRateLimitMessage } from "@/lib/vcs/githubRateLimit";
import * as styles from "./VcsWidgetComments.css";

interface VcsWidgetCommentsProps {
  owner: string;
  repo: string;
  prNumber: number;
  /**
   * GitHub Enterprise host owning owner/repo, e.g. "github.netflix.net".
   * Empty/undefined means github.com — mirrors GitHubBadge's `host` prop.
   */
  host?: string;
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
export function VcsWidgetComments({ owner, repo, prNumber, host, sessionId }: VcsWidgetCommentsProps) {
  const [comments, setComments] = useState<PRComment[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("idle");
  // Captured (not discarded) so the failure body can classify it via
  // getGitHubRateLimitMessage instead of always showing the fixed generic
  // string — see design/ux.md State A/B.
  const [loadError, setLoadError] = useState<unknown>(null);
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
        setLoadError(err);
        setLoadState("error");
      });
  }, [sessionId]);

  // Retry: today's only escape hatch is collapsing/re-expanding the section
  // (which remounts this subtree); this makes that reset explicit and gives
  // the error state a real, visible action (design/ux.md's "No dead ends"
  // acceptance criterion).
  const retryFetchComments = useCallback(() => {
    fetchedRef.current = false;
    fetchComments();
  }, [fetchComments]);

  return (
    <CollapsibleSection sectionKey="pr-comments" title="Comments" defaultExpanded={false}>
      <VcsWidgetCommentsBody
        owner={owner}
        repo={repo}
        prNumber={prNumber}
        host={host}
        comments={comments}
        loadState={loadState}
        loadError={loadError}
        onMount={fetchComments}
        onRetry={retryFetchComments}
      />
    </CollapsibleSection>
  );
}

interface VcsWidgetCommentsBodyProps {
  owner: string;
  repo: string;
  prNumber: number;
  host?: string;
  comments: PRComment[];
  loadState: LoadState;
  loadError: unknown;
  onMount: () => void;
  onRetry: () => void;
}

function VcsWidgetCommentsBody({
  owner,
  repo,
  prNumber,
  host,
  comments,
  loadState,
  loadError,
  onMount,
  onRetry,
}: VcsWidgetCommentsBodyProps) {
  const { track } = useAnalytics();
  // Fires once per mount of this subtree, which only mounts while the
  // section is expanded — see the module doc comment above for why.
  useEffect(() => {
    onMount();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const handleRetry = () => {
    track({ name: "toolbar_button_click", category: "user_action", component: "VcsWidgetComments", labels: { button: "retry" } });
    onRetry();
  };

  if (loadState === "idle" || loadState === "loading") {
    return <p className={styles.status}>Loading…</p>;
  }
  if (loadState === "error") {
    return (
      <div className={styles.status} role="status" aria-live="polite">
        <p style={{ margin: 0 }}>{getGitHubRateLimitMessage(loadError, "Failed to load comments", { autoRetries: false })}</p>
        <button type="button" onClick={handleRetry} style={{ marginTop: 4 }}>
          Retry
        </button>
      </div>
    );
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
            href={`https://${host || "github.com"}/${owner}/${repo}/pull/${prNumber}#${
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
