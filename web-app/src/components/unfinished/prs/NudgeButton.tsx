// +feature: up-next-tabs
"use client";

import { useEffect, useId, useRef, useState } from "react";
import Link from "next/link";
import { NudgeOutcome } from "@/gen/session/v1/github_user_pb";
import { LinkedSessionStatus, type UserPR } from "@/gen/session/v1/types_pb";
import { prAttention } from "@/lib/unfinished/prAttention";
import { emitTabEvent } from "@/lib/unfinished/tabStats";
import { useNudgePR, type NudgeClient, type NudgeState } from "@/lib/hooks/useNudgePR";
import * as styles from "./NudgeButton.css";

export const NUDGE_HINT =
  "Sends this session a message listing the failing checks, unresolved review threads and merge conflict for this PR, as links. Comment text is not included.";
const PAUSED_TEXT = "Session paused. Open it to resume";
const NOT_IDLE_TEXT = "Session isn't idle, so it can't take a request. Open it to continue.";
const BUSY_FALLBACK_TEXT = "Session can't take a request right now. Open it to continue.";
export const NOT_IDLE_HINT =
  "None of this PR's sessions are confirmed idle, so a request could interrupt them. Open the session to continue.";
export const SLOW_SEND_MS = 3_000;

export interface NudgeSession {
  sessionId: string;
  status: LinkedSessionStatus;
  /** Server-confirmed idle and able to take a request; false when unknown. */
  steerReady: boolean;
}

interface NudgeButtonProps {
  pr: UserPR;
  /** Server-linked sessions only (never the legacy `sessionIds` fallback), most recently active first. */
  sessions: NudgeSession[];
  nudgeable: boolean;
  client?: NudgeClient;
  /** Called on `NOTHING_TO_FIX` so the owner can refresh its data. */
  onNothingToFix?: () => void;
}

interface Message {
  text: string;
  role: "status" | "alert";
  openSessionId?: string;
}

/** Why a session cannot take a request, or undefined when it can. */
function blockedReason(status: LinkedSessionStatus): string | undefined {
  if (status === LinkedSessionStatus.PAUSED) return PAUSED_TEXT;
  if (status === LinkedSessionStatus.STOPPED) return "Session stopped. Open it to restart it.";
  return undefined;
}

/** Copy for each outcome; the `never` default fails the build when the proto gains an outcome. */
export function outcomeMessage(outcome: NudgeOutcome, detail: string, sessionId: string): Message {
  const status = (text: string, withLink = false): Message => ({
    text,
    role: "status",
    openSessionId: withLink ? sessionId : undefined,
  });
  switch (outcome) {
    case NudgeOutcome.DELIVERED:
      return status(`Request sent to ${sessionId}`, true);
    case NudgeOutcome.BUSY:
      return status(detail || BUSY_FALLBACK_TEXT, true);
    case NudgeOutcome.PAUSED:
      return status(detail || PAUSED_TEXT, true);
    case NudgeOutcome.DUPLICATE:
      return status("Already requested in the last minute", true);
    case NudgeOutcome.NOTHING_TO_FIX:
      return status("Nothing to fix right now");
    case NudgeOutcome.SESSION_NOT_LINKED:
      return status("Session no longer linked");
    case NudgeOutcome.PR_NOT_FOUND:
      return status("PR not found (closed or moved?)");
    case NudgeOutcome.UNSPECIFIED:
      return status("Unexpected response. Try again.");
    default: {
      const unhandled: never = outcome;
      return status(`Unexpected response (${String(unhandled)})`);
    }
  }
}

export function stateMessage(state: NudgeState): Message | undefined {
  if (state.status === "done") return outcomeMessage(state.outcome, state.detail, state.sessionId);
  if (state.status === "error") return { text: state.message, role: "alert" };
  return undefined;
}

function fixNoun(pr: UserPR): string {
  const a = prAttention(pr);
  const reasons = [a.failingChecks > 0, a.unresolvedThreads > 0, a.mergeConflict].filter(Boolean).length;
  if (reasons > 1) return `PR #${pr.number}`;
  if (a.failingChecks > 0) return `CI on PR #${pr.number}`;
  if (a.unresolvedThreads > 0) return `comments on PR #${pr.number}`;
  return `conflicts on PR #${pr.number}`;
}

export function NudgeButton({ pr, sessions, nudgeable, client, onNothingToFix }: NudgeButtonProps) {
  const { state, nudge, dismiss } = useNudgePR({ client });
  const [selected, setSelected] = useState<string | undefined>();
  const [blockedIds, setBlockedIds] = useState<ReadonlyMap<string, string>>(new Map());
  const block = (id: string, reason: string) => setBlockedIds((prev) => new Map(prev).set(id, reason));
  const [slow, setSlow] = useState(false);
  const [hiddenByNothingToFix, setHiddenByNothingToFix] = useState(false);
  const [landingNote, setLandingNote] = useState(false);
  const hadFocus = useRef(false);
  const regionRef = useRef<HTMLDivElement>(null);
  const hintId = useId();
  const selectId = useId();

  // A poll hands over a fresh `pr` object each time; only a change in what needs attention may unhide.
  const attention = prAttention(pr);
  const attentionSignature = `${pr.checkConclusion}|${attention.failingChecks}|${attention.unresolvedThreads}|${attention.mergeConflict}`;
  useEffect(() => setHiddenByNothingToFix(false), [attentionSignature]);
  const pending = state.status === "pending";
  useEffect(() => {
    if (!pending) {
      setSlow(false);
      return;
    }
    const t = setTimeout(() => setSlow(true), SLOW_SEND_MS);
    return () => clearTimeout(t);
  }, [pending]);

  // A send in flight or finished keeps its session ready even if the next poll reports it busy.
  const isReady = (s: NudgeSession) => s.steerReady || (state.status !== "idle" && state.sessionId === s.sessionId);
  const isBlocked = (s: NudgeSession) =>
    blockedIds.has(s.sessionId) || blockedReason(s.status) !== undefined || !isReady(s);
  const reasonFor = (s: NudgeSession) =>
    blockedReason(s.status) ?? blockedIds.get(s.sessionId) ?? (isReady(s) ? PAUSED_TEXT : NOT_IDLE_TEXT);
  const runnable = sessions.filter((s) => !isBlocked(s));
  const target = runnable.find((s) => s.sessionId === selected) ?? runnable[0];
  const label = target ?? sessions[0];
  const openTarget = sessions.find((s) => s.sessionId === selected) ?? sessions[0];
  const showControls = nudgeable && sessions.length > 0 && !hiddenByNothingToFix;

  // The focused button is removed when the PR stops being nudgeable: keep focus in the card.
  useEffect(() => {
    if (showControls || !hadFocus.current) return;
    hadFocus.current = false;
    setLandingNote(true);
    regionRef.current?.focus();
  }, [showControls]);
  useEffect(() => {
    if (showControls) setLandingNote(false);
  }, [showControls]);

  const forTarget = state.status !== "idle" && target && state.sessionId === target.sessionId;
  const outcome = state.status === "done" ? state.outcome : undefined;
  const holdsOff =
    forTarget && (outcome === NudgeOutcome.DELIVERED || outcome === NudgeOutcome.DUPLICATE);
  const disabled =
    pending || holdsOff || !target || outcome === NudgeOutcome.PR_NOT_FOUND;

  let visibleText = label ? `Ask ${label.sessionId} to fix` : "";
  let accessibleName: string | undefined = label ? `${visibleText} ${fixNoun(pr)}` : undefined;
  if (pending) {
    visibleText = slow ? "Still sending..." : "Sending...";
    accessibleName = undefined;
  } else if (forTarget && outcome === NudgeOutcome.DELIVERED) {
    visibleText = "Sent";
    accessibleName = undefined;
  } else if (forTarget && outcome === NudgeOutcome.DUPLICATE) {
    visibleText = "Already requested in the last minute";
    accessibleName = undefined;
  }

  const onClick = async () => {
    if (disabled || !target) return;
    const result = await nudge(pr, target.sessionId);
    if (result.status === "error" && result.kind === "failed_precondition") {
      block(result.sessionId, result.message);
    } else if (result.status === "done") {
      if (result.outcome === NudgeOutcome.PAUSED) {
        block(result.sessionId, result.detail || PAUSED_TEXT);
      } else if (result.outcome === NudgeOutcome.SESSION_NOT_LINKED) {
        block(result.sessionId, "No longer linked");
      } else if (result.outcome === NudgeOutcome.NOTHING_TO_FIX) {
        setHiddenByNothingToFix(true);
        onNothingToFix?.();
      }
    }
  };

  const onSelect = (sessionId: string) => {
    dismiss();
    setSelected(sessionId);
  };

  const message = stateMessage(state) ?? (landingNote ? ({ text: "Nothing to fix right now", role: "status" } as Message) : undefined);
  const statusMessage = message?.role === "status" ? message : undefined;
  const alertMessage = message?.role === "alert" ? message : undefined;
  const openSessionLink = (id: string) => (
    <Link
      href={`/?session=${encodeURIComponent(id)}`}
      className={styles.openSessionLink}
      aria-label={`Open session ${id}`}
      onClick={() => emitTabEvent({ type: "openSession" })}
    >
      Open session
    </Link>
  );
  const blockedList = sessions.filter(isBlocked);
  const allBlocked = showControls && runnable.length === 0;
  const allNotIdle = allBlocked && sessions.every((s) => !isReady(s));

  return (
    <div className={styles.nudge}>
      {showControls && (
        <>
          <div className={styles.controls}>
            {sessions.length > 1 && (
              <label className={styles.selectLabel} htmlFor={selectId}>
                Session
                <select
                  id={selectId}
                  className={styles.select}
                  value={target?.sessionId ?? ""}
                  onChange={(e) => onSelect(e.target.value)}
                >
                  {sessions.map((s) => (
                    <option key={s.sessionId} value={s.sessionId} disabled={isBlocked(s)}>
                      {isBlocked(s) ? `${s.sessionId} (${reasonFor(s)})` : s.sessionId}
                    </option>
                  ))}
                </select>
              </label>
            )}
            {allNotIdle && (
              <Link
                href={`/?session=${encodeURIComponent(openTarget.sessionId)}`}
                className={styles.primaryOpenLink}
                aria-label={`Open session ${openTarget.sessionId}`}
                data-testid={`pr-open-session-${pr.number}`}
                onClick={() => emitTabEvent({ type: "openSession" })}
              >
                Open session
              </Link>
            )}
            <button
              type="button"
              className={allNotIdle ? styles.askButtonSecondary : styles.askButton}
              aria-label={accessibleName}
              aria-disabled={disabled || undefined}
              aria-busy={pending || undefined}
              aria-describedby={hintId}
              data-testid={`pr-nudge-${pr.number}`}
              onFocus={() => {
                hadFocus.current = true;
              }}
              onBlur={() => {
                hadFocus.current = false;
              }}
              onClick={onClick}
            >
              {visibleText}
            </button>
          </div>
          <p id={hintId} className={styles.hint}>
            {allNotIdle ? NOT_IDLE_HINT : NUDGE_HINT}
          </p>
          {blockedList.map((s) => (
            <p key={s.sessionId} className={styles.hint} data-testid="nudge-blocked-reason">
              {s.sessionId}: {reasonFor(s)}{" "}
              {allBlocked && !allNotIdle && blockedList.length === 1 && openSessionLink(s.sessionId)}
            </p>
          ))}
        </>
      )}
      <div
        ref={regionRef}
        role="status"
        tabIndex={-1}
        className={styles.statusRegion}
        data-testid={`pr-nudge-status-${pr.number}`}
      >
        {statusMessage && (
          <>
            <span>{statusMessage.text}</span>
            {statusMessage.openSessionId && openSessionLink(statusMessage.openSessionId)}
          </>
        )}
      </div>
      <div role="alert" className={styles.alertRegion}>
        {alertMessage && <span>{alertMessage.text}</span>}
      </div>
    </div>
  );
}
