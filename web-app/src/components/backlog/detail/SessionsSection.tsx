"use client";
// +feature: backlog:session-steer

import { useEffect, useRef, useState } from "react";
import type { BacklogItem, LinkedSession, PipelineMode } from "@/lib/hooks/useBacklogService";
import { CollapsibleSection, CollapsibleGroup } from "@/components/ui/Collapsible";
import { classifySessionKind, isSteerable, type SessionKind } from "@/lib/backlog/sessionKind";
import { resolvePipelineModeDisplay } from "@/lib/backlog/pipelineModeDisplay";
import { formatDate } from "@/lib/backlog/formatDate";
import { useShowMore } from "@/lib/hooks/useShowMore";
import { getErrorMessage } from "@/lib/utils/connectError";
import { SessionMonitor } from "../SessionMonitor";
import { SessionDiagnosticPanel } from "./SessionDiagnosticPanel";
import * as styles from "../BacklogItemDetail.css";
import * as sectionStyles from "./SessionsSection.css";

// Partial (not a full Record<SessionKind, ...>) because "work"/"review" are
// Real Sessions rendered via the plain <a> branch below and never look this
// map up — only the 3 Synthetic Session kinds need an icon here.
const SYNTHETIC_KIND_ICON: Partial<Record<SessionKind, string>> = {
  headless_diagnostic: "🩺",
  blocked_guardrail: "🚫",
  manual_review_marker: "✍️",
};

export interface SessionsSectionProps {
  item: BacklogItem;
  pipelineModes: PipelineMode[];
  latestWorkSession: LinkedSession | undefined;
  deletingSessionId: string | null;
  defaultExpanded: boolean;
  onDeleteSession: (session: LinkedSession) => void;
  /**
   * Steers a live work/review session via the widened UpdateSession RPC
   * (Epic 2.1). Rejects on failure so the inline composer (Task 2.2.2c) can
   * keep itself open and surface the error instead of closing optimistically.
   */
  onSteerSession: (session: LinkedSession, message: string) => Promise<void>;
  steeringSessionId: string | null;
}

const SHOW_MORE_CAP = 5;

/**
 * Linked-sessions list, total cost, and the active-session monitor —
 * extracted verbatim from BacklogItemDetail.tsx (Story 3.1.4, Task 3.1.4c).
 * Default-expanded (primary operational info, per requirements' emphasis
 * on session inspectability). Caps its default rendering to the 5 most
 * recent sessions via `useShowMore` (Task 3.1.4c2, Blocker C fix) — a
 * single already-expanded section could otherwise reproduce the
 * "everything visible, nothing prioritized" problem this project exists
 * to fix, one level down, for heavily-cycled items.
 */
export function SessionsSection({
  item,
  pipelineModes,
  latestWorkSession,
  deletingSessionId,
  defaultExpanded,
  onDeleteSession,
  onSteerSession,
  steeringSessionId,
}: SessionsSectionProps) {
  const { visible, hasMore, remaining, showAll } = useShowMore(
    item.id,
    "sessions",
    item.linkedSessions,
    SHOW_MORE_CAP
  );

  // Steer composer state (Story 2.2.2). Mirrors TriageDiffSection's
  // openIndex/draft/toggleRefs shape (Gap 1's same inline-disclosure
  // pattern) — keyed by sessionId rather than array index since sessions
  // aren't positionally stable across a "show more" expansion.
  //
  // steerDrafts is keyed per-sessionId (Record<string, string>), not a
  // single shared string — matching TriageDiffSection's answerDrafts
  // (Record<number, string>) for the same "multiple independent inline
  // forms" problem. A single shared draft let an unsent draft typed for
  // session A survive into session B's composer when the operator switched
  // Steer targets without sending, risking the draft being sent to the
  // wrong session (code review finding, PR #457).
  const [openSteerFor, setOpenSteerFor] = useState<string | null>(null);
  const [steerDrafts, setSteerDrafts] = useState<Record<string, string>>({});
  const [steerError, setSteerError] = useState<string | null>(null);
  const steerToggleRefs = useRef<Record<string, HTMLButtonElement | null>>({});
  const steerInputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (openSteerFor !== null) {
      steerInputRef.current?.focus();
    }
  }, [openSteerFor]);

  const clearSteerDraft = (sessionId: string) => {
    setSteerDrafts((prev) => {
      if (!(sessionId in prev)) return prev;
      const next = { ...prev };
      delete next[sessionId];
      return next;
    });
  };

  const handleSteerCancel = () => {
    const sessionId = openSteerFor;
    const toggle = sessionId ? steerToggleRefs.current[sessionId] : null;
    setOpenSteerFor(null);
    if (sessionId) clearSteerDraft(sessionId);
    setSteerError(null);
    toggle?.focus();
  };

  const handleSteerSubmit = async (s: LinkedSession) => {
    const message = (steerDrafts[s.sessionId] ?? "").trim();
    if (!message) return;
    // Belt-and-suspenders re-check (pre-mortem failure #2, P2): the Send
    // button's disabled state already re-derives isSteerable(s) live on
    // every render, but a stale click event could still slip through if the
    // session ended between render and click — don't make the network call.
    if (!isSteerable(s)) {
      setSteerError("Session has ended — steering is unavailable.");
      return;
    }
    setSteerError(null);
    try {
      await onSteerSession(s, message);
      clearSteerDraft(s.sessionId);
      setOpenSteerFor(null);
      steerToggleRefs.current[s.sessionId]?.focus();
    } catch (err) {
      // Keep the composer open and surface the error inline — don't close
      // optimistically on a failed RPC call.
      setSteerError(getErrorMessage(err, "Failed to steer session."));
    }
  };

  if (item.linkedSessions.length === 0) return null;

  const statusToRole: Record<string, string> = {
    idea: "triage",
    in_progress: "work",
    review: "review",
  };
  const expectedRole = statusToRole[item.status];
  // For the "work" role, source from the same current-work-session value as
  // every other call site (Story 1.1.2, D3) instead of re-scanning
  // independently; other roles keep their own lookup since "current work
  // session" isn't what they're asking for.
  const active =
    expectedRole === "work"
      ? latestWorkSession && !latestWorkSession.endedAt
        ? latestWorkSession
        : undefined
      : [...item.linkedSessions]
          .reverse()
          .find(
            (s) =>
              !s.endedAt &&
              s.role === expectedRole &&
              (classifySessionKind(s) === "work" || classifySessionKind(s) === "review")
          );

  return (
    <CollapsibleSection
      sectionKey="sessions"
      title={`Sessions (${item.linkedSessions.length})`}
      defaultExpanded={defaultExpanded}
    >
      <div className={styles.section}>
        {/* Story 4.1.4: synthetic-session rows (Collapsible + SessionDiagnosticPanel)
            get their own local CollapsibleGroup, deliberately NOT the page-level
            CollapsibleGroup BacklogItemDetail.tsx wraps every top-level sibling
            section in (Task 3.1.4i). SessionsSection's own "sessions"
            CollapsibleSection is a controlled Accordion.Item in that outer group
            (value/onValueChange driven by BacklogItemDetail's sectionExpandEntries,
            which only knows about the fixed top-level section-key set) — nesting
            per-row Items directly into that same shared Accordion.Root would (a)
            immediately be forced closed again by the outer group's controlled
            `value` prop, since a row's sectionKey is never a member of that fixed
            set, and (b) merge dozens of ephemeral row headers into the page-level
            Home/End/Arrow nav loop ADR-027 scoped to top-level siblings only. A
            fresh, uncontrolled CollapsibleGroup here mounts its own Accordion.Root,
            isolating the rows' open/closed state and giving them their own
            row-to-row keyboard nav without disturbing the outer group. */}
        <CollapsibleGroup>
          <div className={styles.sessionList} role="list" aria-label="Linked sessions">
            {visible.map((s) => {
              // A session without endedAt that isn't in the active phase for
              // this item's current status is a stale/orphaned record — label
              // it ended.
              const isOrphan = !s.endedAt && s.role !== statusToRole[item.status];
              const pipelineDisplay = resolvePipelineModeDisplay(s, pipelineModes);
              const kind = classifySessionKind(s);
              const isSynthetic = kind !== "work" && kind !== "review";
              return (
                <div key={s.entityId ?? s.sessionId} className={styles.sessionRow} role="listitem">
                  <div className={styles.sessionRowMain}>
                    {isSynthetic ? (
                      <div className={sectionStyles.diagnosticRowWrapper}>
                        <CollapsibleSection
                          sectionKey={`session-${s.entityId ?? s.sessionId}`}
                          defaultExpanded={false}
                          title={
                            <span className={sectionStyles.diagnosticRowTitle}>
                              <span aria-hidden="true">{SYNTHETIC_KIND_ICON[kind] ?? "🔍"}</span>
                              <span className={styles.sessionId} title={s.sessionId}>
                                {s.sessionId}
                              </span>
                              <span className={styles.sessionRole}>{s.role}</span>
                              {s.startedAt && (
                                <span className={styles.sessionDate}>{formatDate(s.startedAt)}</span>
                              )}
                              {s.estimatedCostUsd > 0 && (
                                <span className={styles.sessionCost} title="Estimated session cost">
                                  ${s.estimatedCostUsd.toFixed(4)}
                                </span>
                              )}
                            </span>
                          }
                        >
                          <SessionDiagnosticPanel session={s} item={item} />
                        </CollapsibleSection>
                      </div>
                    ) : (
                      <a className={styles.sessionLink} href={`/?session=${s.sessionId}`} title="Open in terminal">
                        <span className={styles.sessionId} title={s.sessionId}>
                          {s.sessionId}
                        </span>
                        <span className={styles.sessionRole}>{s.role}</span>
                        {s.worktreeBranch && (
                          <span className={styles.branchBadge} title="Git branch for this work session">
                            {s.worktreeBranch}
                          </span>
                        )}
                        {s.startedAt && <span className={styles.sessionDate}>{formatDate(s.startedAt)}</span>}
                        {s.estimatedCostUsd > 0 && (
                          <span className={styles.sessionCost} title="Estimated session cost">
                            ${s.estimatedCostUsd.toFixed(4)}
                          </span>
                        )}
                        {isOrphan && <span className={styles.sessionEndedBadge}>ended</span>}
                      </a>
                    )}
                    {/* Steer control (Story 2.2.2, ADR-002): never rendered for a
                        synthetic row (headless triage/review, blocked-guardrail,
                        manual-review-marker) — those rows are already a collapsed
                        diagnostic panel, not an action surface. For an ended
                        work/review row it renders disabled+reason instead of being
                        absent, since "this used to be steerable" is real state
                        information. */}
                    {!isSynthetic && (
                      <button
                        type="button"
                        ref={(el) => {
                          steerToggleRefs.current[s.sessionId] = el;
                        }}
                        className={sectionStyles.sessionSteerBtn}
                        disabled={!isSteerable(s) || steeringSessionId === s.sessionId}
                        aria-disabled={!isSteerable(s)}
                        aria-expanded={openSteerFor === s.sessionId}
                        aria-controls={`session-steer-composer-${s.sessionId}`}
                        title={!isSteerable(s) && s.endedAt ? "Session has ended — steering is unavailable" : undefined}
                        aria-label={`Steer session ${s.sessionId}`}
                        data-testid={`session-steer-toggle-${s.sessionId}`}
                        onClick={(e) => {
                          e.preventDefault();
                          setSteerError(null);
                          setOpenSteerFor(s.sessionId);
                        }}
                      >
                        Steer
                      </button>
                    )}
                    <button
                      className={styles.sessionDeleteBtn}
                      disabled={deletingSessionId === s.sessionId}
                      aria-label="Delete session"
                      onClick={(e) => {
                        e.preventDefault();
                        onDeleteSession(s);
                      }}
                    >
                      {deletingSessionId === s.sessionId ? "…" : "Delete"}
                    </button>
                  </div>
                  {!isSynthetic && openSteerFor === s.sessionId && (
                    <div
                      id={`session-steer-composer-${s.sessionId}`}
                      data-testid={`session-steer-composer-${s.sessionId}`}
                      className={sectionStyles.steerComposer}
                      role="form"
                      aria-label={`Steer session ${s.sessionId}`}
                    >
                      <input
                        ref={steerInputRef}
                        type="text"
                        className={sectionStyles.steerInput}
                        value={steerDrafts[s.sessionId] ?? ""}
                        onChange={(e) =>
                          setSteerDrafts((prev) => ({ ...prev, [s.sessionId]: e.target.value }))
                        }
                        onKeyDown={(e) => {
                          if (e.key === "Enter") void handleSteerSubmit(s);
                          if (e.key === "Escape") handleSteerCancel();
                        }}
                        placeholder="Steering message…"
                        data-testid={`session-steer-input-${s.sessionId}`}
                        disabled={steeringSessionId === s.sessionId}
                      />
                      <button
                        type="button"
                        className={sectionStyles.steerSubmitButton}
                        onClick={() => void handleSteerSubmit(s)}
                        // Re-derives isSteerable(s) from the current s prop on every
                        // render rather than closing over the value from when the
                        // composer was opened — a session that ends while the
                        // composer is open must disable Send without requiring
                        // close/reopen (pre-mortem failure #2, P2).
                        disabled={
                          steeringSessionId === s.sessionId ||
                          !(steerDrafts[s.sessionId] ?? "").trim() ||
                          !isSteerable(s)
                        }
                        aria-busy={steeringSessionId === s.sessionId}
                        data-testid={`session-steer-submit-${s.sessionId}`}
                      >
                        {steeringSessionId === s.sessionId ? "Sending…" : "Send"}
                      </button>
                      <button
                        type="button"
                        className={sectionStyles.steerCancelButton}
                        onClick={handleSteerCancel}
                        data-testid={`session-steer-cancel-${s.sessionId}`}
                      >
                        Cancel
                      </button>
                      {steerError && (
                        <span className={sectionStyles.steerError} role="alert">
                          {steerError}
                        </span>
                      )}
                    </div>
                  )}
                  <div className={styles.pipelineGroup} role="group" aria-label="Pipeline">
                    <span className={styles.pipelineLabel}>Pipeline:</span>{" "}
                    {pipelineDisplay.kind === "unrecognized" ? (
                      <span className={styles.pipelineValue}>
                        {`custom (unrecognized mode: '${pipelineDisplay.slug}')`}
                      </span>
                    ) : (
                      <>
                        <span className={styles.pipelineValue}>{pipelineDisplay.name}</span>
                        {pipelineDisplay.drifted && (
                          <>
                            {" "}
                            <span className={styles.pipelineDriftBadge}>(content since changed)</span>
                          </>
                        )}
                      </>
                    )}
                  </div>
                  {/* Synthetic rows' reviewVerdict is shown inside the
                      collapsed SessionDiagnosticPanel above (BlockedNotice /
                      GateVerdictBox readOnly) — showing it again here,
                      always-visible, would both duplicate it and defeat the
                      progressive-disclosure default-collapsed intent. */}
                  {!isSynthetic &&
                    s.reviewVerdict &&
                    (s.reviewVerdict.summary || (s.reviewVerdict.perCriterion?.length ?? 0) > 0) && (
                      <div className={styles.verdictDetail} aria-label="Review verdict detail">
                        {s.reviewVerdict.summary && (
                          <span className={styles.verdictSummary}>
                            <strong>{s.reviewVerdict.overallOutcome}:</strong> {s.reviewVerdict.summary}
                          </span>
                        )}
                        {s.reviewVerdict.perCriterion?.map((c) => (
                          <div key={c.criterionIndex} className={styles.verdictCriterion}>
                            <span>
                              #{c.criterionIndex} {c.outcome}
                            </span>
                            {c.evidence && <span>— {c.evidence}</span>}
                          </div>
                        ))}
                      </div>
                    )}
                </div>
              );
            })}
          </div>
        </CollapsibleGroup>

        {hasMore && (
          <button
            type="button"
            className={sectionStyles.showMoreButton}
            onClick={showAll}
            data-testid="sessions-show-more"
          >
            Show {remaining} more
          </button>
        )}

        {item.totalEstimatedCostUsd > 0 && (
          <p className={styles.sessionTotalCost}>
            Total estimated cost: <strong>${item.totalEstimatedCostUsd.toFixed(4)}</strong>
          </p>
        )}

        {/* Session monitor for the most recent active session. A session is
            only considered active if the item is in the matching lifecycle
            phase — prevents ghost "RUNNING" tiles for sessions that died
            without setting endedAt. */}
        {active && <SessionMonitor sessionId={active.sessionId} sessionRole={active.role} isRunning={true} />}
      </div>
    </CollapsibleSection>
  );
}
