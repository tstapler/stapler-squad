"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { useSessionSummary } from "@/lib/hooks/useSessionSummary";
import { SessionSummaryStatus } from "@/gen/session/v1/types_pb";
import type { SessionSummaryProto } from "@/gen/session/v1/session_summary_pb";
import { formatDate } from "@/lib/utils/timestamp";
import * as markdownStyles from "@/components/backlog/markdownBody.css";
import { srOnly } from "@/components/ui/LiveRegion.css";
import * as styles from "./SessionSummaryPanel.css";

export interface SessionSummaryPanelProps {
  sessionId: string;
}

// Plain-language lead sentence per `error_stage`, per design/ux.md surface (d) —
// never the raw enum string or a stack trace in the primary error text.
const ERROR_STAGE_COPY: Record<string, string> = {
  decisions: "Failed while computing approval decisions.",
  diff: "Failed while computing the diff summary.",
  persist: "Failed while saving the generated summary.",
  "restart-interrupted": "Generation was interrupted, possibly by a server restart.",
};

function stageSentence(stage: string): string {
  return ERROR_STAGE_COPY[stage] ?? "Something went wrong while generating this summary.";
}

function isGenerating(status: SessionSummaryStatus): boolean {
  return (
    status === SessionSummaryStatus.PENDING ||
    status === SessionSummaryStatus.GENERATING ||
    status === SessionSummaryStatus.UNSPECIFIED
  );
}

type Phase = "loading" | "empty" | "ready" | "error" | "error-stale";

function computePhase(data: SessionSummaryProto | null, neverResolved: boolean): Phase {
  if (neverResolved) return "empty";
  if (data === null) return "loading";
  if (isGenerating(data.status)) return "loading";
  if (data.status === SessionSummaryStatus.ERROR) {
    return data.markdown ? "error-stale" : "error";
  }
  return "ready";
}

// ---------------------------------------------------------------------------
// Skeleton spec — design/ux.md surface (b): 5 heading bars + 12 content bars
// = 17 total `data-testid="summary-skeleton-block"` elements.
// ---------------------------------------------------------------------------

type SkeletonBlockKind = "heading" | "pill" | "line" | "lineWidth60" | "lineWidth40";

interface SkeletonBlockSpec {
  kind: SkeletonBlockKind;
}

interface SkeletonSectionSpec {
  title: string;
  blocks: SkeletonBlockSpec[];
}

const SKELETON_SECTIONS: SkeletonSectionSpec[] = [
  {
    title: "Decisions",
    blocks: [
      { kind: "heading" },
      { kind: "pill" },
      { kind: "pill" },
      { kind: "pill" },
      { kind: "pill" },
      { kind: "pill" },
    ],
  },
  {
    title: "What Was Done",
    blocks: [{ kind: "heading" }, { kind: "line" }, { kind: "line" }, { kind: "lineWidth60" }],
  },
  {
    title: "Changes",
    blocks: [{ kind: "heading" }, { kind: "line" }, { kind: "lineWidth40" }],
  },
  {
    title: "Timeline",
    blocks: [{ kind: "heading" }, { kind: "line" }],
  },
  {
    title: "Token Usage",
    blocks: [{ kind: "heading" }, { kind: "line" }],
  },
];

function skeletonShapeClass(kind: SkeletonBlockKind): string {
  switch (kind) {
    case "heading":
      return styles.skeletonHeading;
    case "pill":
      return styles.skeletonPill;
    case "lineWidth60":
      return styles.skeletonLineWidth60;
    case "lineWidth40":
      return styles.skeletonLineWidth40;
    case "line":
    default:
      return styles.skeletonLine;
  }
}

/**
 * Reads `prefers-reduced-motion` at the JS layer (not just via a CSS media
 * query) so the shimmer-vs-static class choice is directly assertable in
 * tests via `window.matchMedia` mocking (UX-AC-14, Task 3.1.2c).
 */
function usePrefersReducedMotion(): boolean {
  const [reduced, setReduced] = useState(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") return false;
    return window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  });

  useEffect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") return;
    const mql = window.matchMedia("(prefers-reduced-motion: reduce)");
    const handler = () => setReduced(mql.matches);
    handler();
    mql.addEventListener?.("change", handler);
    return () => mql.removeEventListener?.("change", handler);
  }, []);

  return reduced;
}

function SummarySkeleton() {
  const reducedMotion = usePrefersReducedMotion();
  const blockClass = reducedMotion ? styles.skeletonBlockReducedMotion : styles.skeletonBlock;

  return (
    <>
      {SKELETON_SECTIONS.map((section) => (
        <div className={styles.skeletonSection} key={section.title}>
          {section.blocks.map((block, i) => (
            <div
              key={`${section.title}-${i}`}
              data-testid="summary-skeleton-block"
              className={`${blockClass} ${skeletonShapeClass(block.kind)}`}
            />
          ))}
        </div>
      ))}
    </>
  );
}

// ---------------------------------------------------------------------------
// Decisions-at-a-glance card — built from structured `data.decisions` proto
// fields, not parsed from markdown, so it's unmissable near the top rather
// than buried in the flowing document (research/ux.md §5).
// ---------------------------------------------------------------------------

function DecisionsGlanceCard({ decisions }: { decisions?: SessionSummaryProto["decisions"] }) {
  const autoApproved = decisions?.autoApproved ?? 0;
  const manuallyApproved = decisions?.manuallyApproved ?? 0;
  const denied = decisions?.denied ?? 0;
  const reviewQueueResolved = decisions?.reviewQueueResolved ?? 0;
  const stillOpen = decisions?.stillOpen ?? 0;
  const total = autoApproved + manuallyApproved + denied + reviewQueueResolved + stillOpen;

  return (
    <div className={styles.glanceCard} data-testid="summary-decisions-glance">
      <div className={styles.glanceTitle}>Decisions at a glance</div>
      {total === 0 ? (
        <div className={styles.glanceEmptyText}>
          No approval requests occurred during this session.
        </div>
      ) : (
        <div className={styles.glancePills}>
          <span className={styles.glancePill}>✓ {autoApproved} auto-approved</span>
          <span className={styles.glancePill}>✓ {manuallyApproved} manually approved</span>
          <span className={styles.glancePill}>✕ {denied} denied</span>
          <span className={styles.glancePill}>◔ {reviewQueueResolved} review-resolved</span>
          <span className={styles.glancePill}>● {stillOpen} still open</span>
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Main component
// ---------------------------------------------------------------------------

export function SessionSummaryPanel({ sessionId }: SessionSummaryPanelProps) {
  const { data, neverResolved, regenerate, copy } = useSessionSummary(sessionId);

  const phase = computePhase(data, neverResolved);

  const [liveMessage, setLiveMessage] = useState("");
  const [regenerating, setRegenerating] = useState(false);
  const regeneratingRef = useRef(false);
  const [copyState, setCopyState] = useState<"idle" | "success" | "failure">("idle");
  const copyRevertTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const prevPhaseRef = useRef<Phase | null>(null);

  const setRegeneratingBoth = useCallback((value: boolean) => {
    regeneratingRef.current = value;
    setRegenerating(value);
  }, []);

  // Announce state transitions through the single shared aria-live region,
  // per design/ux.md's exact-strings table (surface (e)).
  useEffect(() => {
    if (prevPhaseRef.current === phase) return;
    prevPhaseRef.current = phase;

    switch (phase) {
      case "loading":
        setLiveMessage("Generating summary…");
        break;
      case "ready":
        setLiveMessage(regeneratingRef.current ? "Summary regenerated." : "Summary ready.");
        setRegeneratingBoth(false);
        break;
      case "error":
        setLiveMessage(`Summary generation failed: ${stageSentence(data?.errorStage ?? "")}`);
        setRegeneratingBoth(false);
        break;
      case "error-stale":
        setLiveMessage(
          "Showing the previous summary. Regeneration failed — see the banner for details.",
        );
        setRegeneratingBoth(false);
        break;
      case "empty":
        setLiveMessage("No summary available for this session.");
        break;
    }
    // data is read for its current-render value only when phase transitions to
    // "error" — intentionally not a dependency so we don't re-run per poll tick.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [phase, setRegeneratingBoth]);

  useEffect(() => {
    return () => {
      if (copyRevertTimeoutRef.current) clearTimeout(copyRevertTimeoutRef.current);
    };
  }, []);

  const handleCopy = useCallback(async () => {
    const ok = await copy();
    if (ok) {
      setCopyState("success");
      setLiveMessage("Summary copied to clipboard.");
      if (copyRevertTimeoutRef.current) clearTimeout(copyRevertTimeoutRef.current);
      copyRevertTimeoutRef.current = setTimeout(() => setCopyState("idle"), 1500);
    } else {
      setCopyState("failure");
      setLiveMessage("Copy failed. Select the text and copy manually.");
    }
  }, [copy]);

  const handleRegenerate = useCallback(async () => {
    setRegeneratingBoth(true);
    setLiveMessage("Regenerating summary…");
    await regenerate();
  }, [regenerate, setRegeneratingBoth]);

  if (phase === "empty") {
    return (
      <div className={styles.container} data-testid="session-summary-panel">
        <div className={styles.emptyState} data-testid="summary-empty-state">
          <div className={styles.emptyStateHeading}>No summary available for this session.</div>
        </div>
        <div role="status" aria-live="polite" aria-atomic="true" className={srOnly}>
          {liveMessage}
        </div>
      </div>
    );
  }

  if (phase === "loading") {
    return (
      <div
        className={styles.container}
        data-testid="session-summary-panel"
        role="region"
        aria-busy="true"
        aria-label="Session summary"
      >
        <div className={styles.header}>
          <div className={styles.titleGroup}>
            <span className={styles.title}>Session Summary</span>
            <span className={styles.statusText} aria-hidden="true">
              ⟳ Generating…
            </span>
          </div>
          <div className={styles.toolbar}>
            <button
              type="button"
              className={styles.copyButton}
              aria-label="Copy summary as Markdown"
              disabled
            >
              📋 Copy as Markdown
            </button>
          </div>
        </div>
        <SummarySkeleton />
        <div role="status" aria-live="polite" aria-atomic="true" className={srOnly}>
          {liveMessage}
        </div>
      </div>
    );
  }

  // phase is "ready" | "error" | "error-stale" here — data is guaranteed non-null.
  const summary = data as SessionSummaryProto;
  const isStale = phase === "error-stale";
  const isBareError = phase === "error";

  return (
    <div className={styles.container} data-testid="session-summary-panel" role="region" aria-label="Session summary">
      <div className={styles.header}>
        <div className={styles.titleGroup}>
          <span className={styles.title}>
            Session Summary{summary.sessionTitle ? `: ${summary.sessionTitle}` : ""}
          </span>
          {!isBareError && (
            <span className={styles.statusText}>
              {isStale ? "⚠ Regeneration failed" : `✓ Ready · generated ${formatDate(summary.generatedAt)}`}
            </span>
          )}
        </div>
        {!isBareError && (
          <div className={styles.toolbar}>
            <button
              type="button"
              className={styles.copyButton}
              aria-label="Copy summary as Markdown"
              onClick={handleCopy}
            >
              {copyState === "success" ? "✓ Copied" : "📋 Copy as Markdown"}
            </button>
          </div>
        )}
      </div>

      {copyState === "failure" && (
        <div className={styles.copyFailureText} data-testid="summary-copy-failure">
          Copy failed — select the text below and copy manually.
        </div>
      )}

      {isBareError && (
        <div className={styles.errorCard} data-testid="summary-error-card">
          <div className={styles.errorLead}>
            <span aria-hidden="true">⚠</span>
            <span>Couldn&apos;t finish generating this summary.</span>
          </div>
          <div className={styles.errorStageText}>{stageSentence(summary.errorStage)}</div>
          <div className={styles.errorTimestamp}>
            Last attempt: {formatDate(summary.generatedAt)}
          </div>
          <button
            type="button"
            className={styles.primaryButton}
            onClick={handleRegenerate}
            disabled={regenerating}
          >
            {regenerating ? "⟳ Regenerating…" : "↻ Regenerate"}
          </button>
          {summary.errorMessage && (
            <details className={styles.errorDetails}>
              <summary>Details</summary>
              <div className={styles.errorDetailsBody}>{summary.errorMessage}</div>
            </details>
          )}
        </div>
      )}

      {isStale && (
        <div className={styles.staleBanner} data-testid="summary-stale-banner">
          <div className={styles.staleBannerText}>
            Showing the summary from the last successful generation, dated{" "}
            {formatDate(summary.generatedAt)} — regeneration failed, see error above.
          </div>
          <div className={styles.staleBannerActions}>
            <button
              type="button"
              className={styles.primaryButton}
              onClick={handleRegenerate}
              disabled={regenerating}
              data-testid="summary-try-again"
            >
              {regenerating ? "⟳ Regenerating…" : "↻ Try again"}
            </button>
            {summary.errorMessage && (
              <details className={styles.errorDetails}>
                <summary>Details</summary>
                <div className={styles.errorDetailsBody}>{summary.errorMessage}</div>
              </details>
            )}
          </div>
        </div>
      )}

      {(phase === "ready" || isStale) && (
        <>
          <DecisionsGlanceCard decisions={summary.decisions} />
          <div className={markdownStyles.markdownBody} data-testid="summary-markdown-body">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{summary.markdown}</ReactMarkdown>
          </div>
        </>
      )}

      <div role="status" aria-live="polite" aria-atomic="true" className={srOnly}>
        {liveMessage}
      </div>
    </div>
  );
}
